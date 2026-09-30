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

// agentMark returns the role key of the marker on the last non-empty line of body, or "".
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
		if _, err := ParseRoleKey(m[1]); err != nil {
			return ""
		}
		return m[1]
	}
	return ""
}

type repoState struct {
	Branches    map[string]string `json:"branches"`
	Checks      map[string]string `json:"checks"`         // head SHA -> last Checks value
	Since       string            `json:"comments_since"` // RFC 3339, UTC
	SeenAtSince []int64           `json:"seen_at_since"`  // comment IDs updated exactly at Since
	Merged      []int             `json:"merged"`
	LastError   string            `json:"last_error,omitempty"`
}

type watchEvent struct {
	Type    string `json:"type"` // push, red, comment, merge, or error
	Repo    string `json:"repo"`
	Project string `json:"project"`
	Ref     string `json:"ref,omitempty"`
	SHA     string `json:"sha,omitempty"`
	Number  int    `json:"number,omitempty"`
	URL     string `json:"url,omitempty"`
	Author  string `json:"author,omitempty"`
	By      string `json:"by,omitempty"` // agent or human, for comments
	RoleKey string `json:"role_key,omitempty"`
}

type watcher struct {
	env Env
	out io.Writer
}

// emit prints one report line and appends it to reports/watcher.jsonl.
func (w *watcher) emit(ev watchEvent, text, call, value string) error {
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
	f, err := os.OpenFile(filepath.Join(dir, "watcher.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
		st.Branches, st.Checks, st.Since = branches, map[string]string{}, w.env.Now().UTC().Format(time.RFC3339)
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
		}
		if !changed && st.Checks[sha] != "pending" {
			continue
		}
		state, err := h.Checks(ctx, sha)
		if err != nil {
			return err
		}
		prev := st.Checks[sha]
		st.Checks[sha] = state
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

	comments, err := h.Comments(ctx, st.Since)
	if err != nil {
		return err
	}
	commentCall := h.LastCall()
	slices.SortStableFunc(comments, func(a, b hostComment) int { return parseTime(a.UpdatedAt).Compare(parseTime(b.UpdatedAt)) })
	since := parseTime(st.Since)
	for _, c := range comments {
		at := parseTime(c.UpdatedAt)
		if at.Before(since) || (at.Equal(since) && slices.Contains(st.SeenAtSince, c.ID)) {
			continue
		}
		ev := base
		ev.Type, ev.Number, ev.URL, ev.Author, ev.By = "comment", c.Number(), c.URL, c.User.Login, "human"
		who := "human"
		if key := agentMark(c.Body); key != "" {
			ev.By, ev.RoleKey, who = "agent", key, "agent "+key
		}
		if err := w.emit(ev, fmt.Sprintf("comment by %s (%s) on %s #%d", c.User.Login, who, r.Repo, ev.Number), commentCall, fmt.Sprint(c.ID)); err != nil {
			return err
		}
		if at.After(since) {
			since, st.SeenAtSince = at, nil
		}
		st.SeenAtSince = append(st.SeenAtSince, c.ID)
		st.Since = since.UTC().Format(time.RFC3339)
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
	cfg, err := loadRepos(env.DataDir)
	if err != nil {
		return err
	}
	var hosts []codeHost
	for _, r := range cfg.Repos {
		h, err := newHost(r)
		if err != nil {
			return err
		}
		hosts = append(hosts, h)
	}
	w := &watcher{env: env, out: out}
	for {
		if err := w.pollAll(ctx, cfg, hosts); err != nil {
			return err
		}
		if once {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Duration(cfg.IntervalSeconds) * time.Second):
		}
	}
}
