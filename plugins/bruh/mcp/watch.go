package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// agentMarkRE is the structural marker that ends every post of an agent on a code host.
var agentMarkRE = regexp.MustCompile(`^<!-- bruh:([a-z0-9-]+) -->$`)

// agentMark returns the role key (or "owner") of the marker on the last non-empty line of body, or "".
// Only this structure decides; the words of a post never do.
func agentMark(body string) string {
	lines := splitLines(strings.ReplaceAll(body, "\r\n", "\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		l := strings.TrimSpace(lines[i])
		if l == "" {
			continue
		}
		m := agentMarkRE.FindStringSubmatch(l)
		if m == nil {
			return ""
		}
		// "owner" marks a post that an agent made from a manual session of the owner
		// (post-findings.sh without BRUH_ROLE_KEY); it is an agent post, not a human one.
		if _, err := ParseRoleKey(m[1]); err != nil && m[1] != "owner" {
			return ""
		}
		return m[1]
	}
	return ""
}

// feedState is the read position of one list of comments or reviews.
type feedState struct {
	Since string  `json:"since"` // RFC 3339, UTC
	Seen  []int64 `json:"seen"`  // IDs updated exactly at Since
}

// maxCheckPolls caps how often the watcher reads the checks of a head that is pending or has none
// yet (external CI often registers after the push). At 60 seconds it is about half an hour.
const maxCheckPolls = 30

type repoState struct {
	Branches   map[string]string     `json:"branches"`
	Checks     map[string]string     `json:"checks"`      // head SHA -> last Checks value
	CheckPolls map[string]int        `json:"check_polls"` // head SHA -> reads of its checks
	Feeds      map[string]*feedState `json:"feeds"`       // issue_comments, review_comments, reviews
	Merged     []int                 `json:"merged"`
	LastError  string                `json:"last_error,omitempty"`
}

type watchEvent struct {
	Type    string `json:"type"` // push, red, comment, review, merge, or error
	Repo    string `json:"repo"`
	Project string `json:"project"`
	Ref     string `json:"ref,omitempty"`
	SHA     string `json:"sha,omitempty"`
	Number  int    `json:"number,omitempty"`
	URL     string `json:"url,omitempty"`
	Author  string `json:"author,omitempty"`
	By      string `json:"by,omitempty"` // agent or human, for comments
	RoleKey string `json:"role_key,omitempty"`
	State   string `json:"review_state,omitempty"` // reviews
}

type watcher struct {
	env Env
	out io.Writer
}

// emit prints one report line and appends it to reports/clanker-<project>.jsonl, the report
// file of the project of the event, in one write.
func (w *watcher) emit(ev watchEvent, text, call, value string) error {
	if !projectRE.MatchString(ev.Project) {
		return fmt.Errorf("invalid project of %s: %q", ev.Repo, ev.Project)
	}
	at := w.env.Stamp()
	raw, _ := json.Marshal(ev)
	line, _ := json.Marshal(ReportLine{At: at, From: "watcher", Kind: "event", Text: text, Source: &Source{Call: call, Value: value, At: at}, Event: raw})
	line = append(line, '\n')
	if _, err := w.out.Write(line); err != nil {
		return err
	}
	dir, err := w.env.Dir("reports")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "clanker-"+ev.Project+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(line)
	return errors.Join(werr, f.Close())
}

func short(sha string) string { return sha[:min(len(sha), 7)] }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// pollRepo polls one repository. The first poll records the baseline and emits nothing.
func (w *watcher) pollRepo(ctx context.Context, r repoConfig, h codeHost, st *repoState) error {
	branches, err := h.Branches(ctx)
	if err != nil {
		return err
	}
	branchCall := h.LastCall()
	if st.Branches == nil {
		merged, err := h.MergedPulls(ctx)
		if err != nil {
			return err
		}
		now := w.env.Now().UTC().Format(time.RFC3339)
		st.Branches, st.Checks, st.CheckPolls = branches, map[string]string{}, map[string]int{}
		st.Feeds = map[string]*feedState{"issue_comments": {Since: now}, "review_comments": {Since: now}, "reviews": {Since: now}}
		for _, p := range merged {
			st.Merged = append(st.Merged, p.Number)
		}
		return nil
	}
	base := watchEvent{Repo: r.Repo, Project: r.Project}
	for _, name := range slices.Sorted(maps.Keys(branches)) {
		sha := branches[name]
		changed := st.Branches[name] != sha
		if changed {
			ev := base
			ev.Type, ev.Ref, ev.SHA = "push", name, sha
			if err := w.emit(ev, fmt.Sprintf("push %s %s %s", r.Repo, name, short(sha)), branchCall, sha); err != nil {
				return err
			}
			st.Branches[name] = sha // saved at once, so a later error does not emit the push again
		}
		prev, known := st.Checks[sha]
		if !changed && known && (prev != "pending" && prev != "none" || st.CheckPolls[sha] >= maxCheckPolls) {
			continue
		}
		if !changed && !known {
			continue // a head of the baseline
		}
		state, err := h.Checks(ctx, sha)
		if err != nil {
			return err
		}
		st.Checks[sha] = state
		st.CheckPolls[sha]++
		if state == "failure" && prev != "failure" {
			ev := base
			ev.Type, ev.Ref, ev.SHA = "red", name, sha
			if err := w.emit(ev, fmt.Sprintf("red checks %s %s %s", r.Repo, name, short(sha)), h.LastCall(), state); err != nil {
				return err
			}
		}
	}
	st.Branches = branches
	heads := slices.Collect(maps.Values(branches))
	maps.DeleteFunc(st.Checks, func(sha, _ string) bool { return !slices.Contains(heads, sha) })
	maps.DeleteFunc(st.CheckPolls, func(sha string, _ int) bool { return !slices.Contains(heads, sha) })

	for _, feed := range []struct {
		name  string
		fetch func(context.Context, string) ([]hostComment, error)
	}{{"issue_comments", h.IssueComments}, {"review_comments", h.ReviewComments}, {"reviews", h.Reviews}} {
		fs := st.Feeds[feed.name]
		items, err := feed.fetch(ctx, fs.Since)
		if err != nil {
			return err
		}
		if err := w.emitFeed(r, base, feed.name, items, h.LastCall(), fs); err != nil {
			return err
		}
	}

	merged, err := h.MergedPulls(ctx)
	if err != nil {
		return err
	}
	for _, p := range merged {
		if slices.Contains(st.Merged, p.Number) {
			continue
		}
		ev := base
		ev.Type, ev.Number, ev.URL, ev.SHA = "merge", p.Number, p.URL, p.MergeSHA
		if err := w.emit(ev, fmt.Sprintf("merged %s #%d", r.Repo, p.Number), h.LastCall(), p.MergedAt); err != nil {
			return err
		}
		st.Merged = append(st.Merged, p.Number)
	}
	st.Merged = st.Merged[max(0, len(st.Merged)-500):]
	return nil
}

// emitFeed emits the items of one feed that are newer than its read position, and moves it.
// Each feed has its own position, so a full page of one feed cannot hide items of another.
func (w *watcher) emitFeed(r repoConfig, base watchEvent, feed string, items []hostComment, call string, fs *feedState) error {
	slices.SortStableFunc(items, func(a, b hostComment) int { return parseTime(a.UpdatedAt).Compare(parseTime(b.UpdatedAt)) })
	since := parseTime(fs.Since)
	for _, c := range items {
		at := parseTime(c.UpdatedAt)
		if at.Before(since) || (at.Equal(since) && slices.Contains(fs.Seen, c.ID)) {
			continue
		}
		ev := base
		ev.Type, ev.Number, ev.URL, ev.Author, ev.By = "comment", c.Number(), c.URL, c.User.Login, "human"
		what := "comment"
		if feed == "reviews" {
			ev.Type, ev.State, what = "review", c.State, "review "+c.State
		}
		who := "human"
		if key := agentMark(c.Body); key != "" {
			ev.By, ev.RoleKey, who = "agent", key, "agent "+key
		}
		if err := w.emit(ev, fmt.Sprintf("%s by %s (%s) on %s #%d", what, c.User.Login, who, r.Repo, ev.Number), call, fmt.Sprint(c.ID)); err != nil {
			return err
		}
		if at.After(since) {
			since, fs.Seen = at, nil
		}
		fs.Seen = append(fs.Seen, c.ID)
		fs.Since = since.UTC().Format(time.RFC3339)
	}
	return nil
}

func loadWatchState(file string) (map[string]*repoState, error) {
	st := map[string]*repoState{}
	raw, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	return st, json.Unmarshal(raw, &st)
}

// pollAll polls each repository once and saves the state. An error of one repository is an
// event (once for each new error text), and the other repositories are still polled.
func (w *watcher) pollAll(ctx context.Context, cfg reposConfig, hosts []codeHost) error {
	dir, err := w.env.Dir("watch")
	if err != nil {
		return err
	}
	file := filepath.Join(dir, "state.json")
	state, err := loadWatchState(file)
	if err != nil {
		return err
	}
	for i, r := range cfg.Repos {
		key := r.Host + ":" + r.Repo
		st := state[key]
		if st == nil {
			st = &repoState{}
			state[key] = st
		}
		if err := w.pollRepo(ctx, r, hosts[i], st); err != nil {
			if err.Error() != st.LastError {
				st.LastError = err.Error()
				if eerr := w.emit(watchEvent{Type: "error", Repo: r.Repo, Project: r.Project}, "watcher error on "+r.Repo+": "+err.Error(), hosts[i].LastCall(), "error"); eerr != nil {
					return eerr
				}
			}
		} else {
			st.LastError = ""
		}
		raw, _ := json.MarshalIndent(state, "", "  ")
		if err := atomicWrite(file, raw); err != nil {
			return err
		}
	}
	return nil
}

// runWatch polls every interval_seconds until ctx ends, or once.
func runWatch(ctx context.Context, env Env, out io.Writer, once bool) error {
	w := &watcher{env: env, out: out}
	if once {
		return pollOnce(ctx, env, w)
	}
	return watchLoop(ctx, env, w, nil, func(ctx context.Context, d time.Duration) {
		select {
		case <-ctx.Done():
		case <-time.After(d):
		}
	})
}

// watchLoop reads repos.json again before each poll, so that repos_set takes effect
// without a restart, and a missing file means no repositories yet.
func watchLoop(ctx context.Context, env Env, w *watcher, afterPoll func(), sleep func(context.Context, time.Duration)) error {
	for ctx.Err() == nil {
		cfg, err := loadReposOrEmpty(env.DataDir)
		if err != nil {
			return err
		}
		if err := pollWith(ctx, cfg, w); err != nil {
			return err
		}
		if afterPoll != nil {
			afterPoll()
		}
		sleep(ctx, time.Duration(cfg.IntervalSeconds)*time.Second)
	}
	return nil
}

func pollOnce(ctx context.Context, env Env, w *watcher) error {
	cfg, err := loadReposOrEmpty(env.DataDir)
	if err != nil {
		return err
	}
	return pollWith(ctx, cfg, w)
}

func pollWith(ctx context.Context, cfg reposConfig, w *watcher) error {
	var hosts []codeHost
	for _, r := range cfg.Repos {
		h, err := newHost(r)
		if err != nil {
			return err
		}
		hosts = append(hosts, h)
	}
	return w.pollAll(ctx, cfg, hosts)
}

// loadReposOrEmpty is loadRepos, except that a missing repos.json is an empty configuration.
func loadReposOrEmpty(dataDir string) (reposConfig, error) {
	cfg, err := loadRepos(dataDir)
	if errors.Is(err, fs.ErrNotExist) {
		return reposConfig{IntervalSeconds: 60}, nil
	}
	return cfg, err
}
