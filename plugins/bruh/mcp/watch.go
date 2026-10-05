package main

import (
	"cmp"
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
	"syscall"
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
	Since string  `json:"since"` // RFC 3339 with an optional fraction, UTC
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
	// Items is the cursor of a command or mcp source: item ID -> version. nil before the baseline.
	Items map[string]string `json:"items,omitzero"`
	// The last good read of the source key, which monitor_start returns for a shared key.
	LastCall  string `json:"last_call,omitempty"`
	LastValue string `json:"last_value,omitempty"`
	LastAt    string `json:"last_at,omitempty"`
}

type watchEvent struct {
	// push, red, comment, review, merge (codehost); new, changed, gone (command, mcp); error; expired
	Type    string `json:"type"`
	Monitor string `json:"monitor,omitempty"` // the monitor ID of the subscriber
	Key     string `json:"key,omitempty"`     // the source key
	Subject string `json:"subject,omitempty"` // new, changed, gone: the title or the ID of the item
	Item    string `json:"item,omitempty"`
	Version string `json:"version,omitempty"`
	Cause   string `json:"cause,omitempty"` // expired: until or grant
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
	env    Env
	out    io.Writer
	errOut io.Writer // the log of skipped entries; runWatch sets os.Stderr
	subs   []monitor // the monitors of the source key that is polled now
	key    string
	quiet  bool         // monitor_report: the report file only, no mail and no print
	sent   []watchEvent // the events emitted, for monitor_report
}

// emit delivers one event to each monitor of w.subs that wants it (spec 9.5): one report line
// with the monitor ID, appended once for each project to reports/clanker-<project>.jsonl; the
// line as mail from bigm with the header DONE: event <project>: <text> to each local subscriber;
// and the printed line, which reaches bigm as a notification of the plugin monitor, only for a
// remote subscriber (bigm relays it through Orca) and once for an error.
func (w *watcher) emit(ev watchEvent, text, call, value string) error {
	at := w.env.Stamp()
	ev.Key = w.key
	reported := map[string]bool{}
	printed := false
	for _, m := range w.subs {
		if !m.wants(ev) {
			continue
		}
		e := ev
		e.Monitor, e.Project = m.ID, cmp.Or(e.Project, m.Project)
		if !projectRE.MatchString(e.Project) {
			return fmt.Errorf("invalid project of %s: %q", cmp.Or(e.Repo, w.key), e.Project)
		}
		if len(reported) == 0 {
			w.sent = append(w.sent, e)
		}
		raw, _ := json.Marshal(e)
		line, _ := json.Marshal(ReportLine{At: at, From: "watcher", Kind: "event", Text: text, Source: &Source{Call: call, Value: value, At: at}, Event: raw})
		line = append(line, '\n')
		if !reported[e.Project] {
			reported[e.Project] = true
			if err := appendReport(w.env, e.Project, line); err != nil {
				return err
			}
		}
		if w.quiet {
			continue
		}
		local := isLocal(w.env, m.Subscriber)
		if local {
			if _, err := writeMail(w.env, "bigm", m.Subscriber, eventHeader(e.Project, text), string(line[:len(line)-1])); err != nil {
				return err
			}
		}
		if !local || (e.Type == "error" && !printed) {
			printed = true
			if _, err := w.out.Write(line); err != nil {
				return err
			}
		}
	}
	return nil
}

// appendReport appends one line to reports/clanker-<project>.jsonl in one write.
func appendReport(env Env, project string, line []byte) error {
	dir, err := env.Dir("reports")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "clanker-"+project+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
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
		fs.Since = since.UTC().Format(time.RFC3339Nano) // GitLab notes have milliseconds
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

// pollAll is one loop of the poller (spec 9.5). It stops the monitors that expired or lost
// their command grant, polls each codehost key of repos.json and each command key that is due,
// once for all subscribers of the key, saves each cursor, prunes the cursors of keys with no
// monitor, and writes watch/poller_at. An error of one key is an event (once for each new error
// text), and the other keys are still polled.
func (w *watcher) pollAll(ctx context.Context, cfg reposConfig, hosts []codeHost) error {
	file, err := watchFile(w.env)
	if err != nil {
		return err
	}
	grants, gerr := readCommandGrants(w.env)
	if gerr != nil {
		fmt.Fprintf(w.log(), "command grants: %v; command sources are not polled\n", gerr)
	}
	stored, err := w.stopMonitors(grants, gerr == nil)
	if err != nil {
		return err
	}
	groups := map[string][]monitor{}
	for _, m := range append(standingMonitors(cfg), stored...) {
		if m.Source.Kind != "mcp" {
			groups[m.key()] = append(groups[m.key()], m)
		}
	}
	state, err := loadWatchState(file)
	if err != nil {
		return err
	}
	poll := func(key string, call func(*repoState) error, errEv watchEvent, errText string, lastCall func() string) error {
		w.subs, w.key = groups[key], key
		st := state[key]
		if st == nil {
			st = &repoState{}
		}
		if err := call(st); err != nil {
			if err.Error() != st.LastError {
				st.LastError = err.Error()
				if eerr := w.emit(errEv, errText+": "+err.Error(), lastCall(), "error"); eerr != nil {
					return eerr
				}
			}
		} else {
			st.LastError = ""
		}
		return updateWatchState(w.env, func(s map[string]*repoState) error { s[key] = st; return nil })
	}
	for i, r := range cfg.Repos {
		h := hosts[i]
		err := poll(r.Host+":"+r.Repo, func(st *repoState) error {
			if err := w.pollRepo(ctx, r, h, st); err != nil {
				return err
			}
			st.LastCall, st.LastValue, st.LastAt = h.LastCall(), fmt.Sprintf("%d branches", len(st.Branches)), w.env.Stamp()
			return nil
		}, watchEvent{Type: "error", Repo: r.Repo, Project: r.Project}, "watcher error on "+r.Repo, h.LastCall)
		if err != nil {
			return err
		}
	}
	every := time.Duration(max(cfg.IntervalSeconds, 60)) * time.Second
	for _, key := range slices.Sorted(maps.Keys(groups)) {
		src := groups[key][0].Source
		if src.Kind != "command" || gerr != nil {
			continue
		}
		if st := state[key]; st != nil && st.LastAt != "" && w.env.Now().Sub(parseStamp(st.LastAt)) < every {
			continue
		}
		call := commandCall(src.Argv)
		err := poll(key, func(st *repoState) error {
			out, err := runCommand(ctx, src.Argv)
			if err != nil {
				return err
			}
			items, err := sourceItems(out, src)
			if err != nil {
				return err
			}
			return w.applyItems(st, items, call)
		}, watchEvent{Type: "error"}, "monitor error on "+key, func() string { return call })
		if err != nil {
			return err
		}
	}
	// Prune under the lock with a fresh read of the monitors, so that a cursor that
	// monitor_start wrote after the read above stays.
	err = updateWatchState(w.env, func(s map[string]*repoState) error {
		live, err := loadMonitors(w.env)
		if err != nil {
			return err
		}
		repos, err := loadReposOrEmpty(w.env.DataDir)
		if err != nil {
			return err
		}
		keep := map[string]bool{}
		for _, m := range slices.Concat(standingMonitors(cfg), standingMonitors(repos), live) {
			keep[m.key()] = true
		}
		maps.DeleteFunc(s, func(k string, _ *repoState) bool { return !keep[k] })
		return nil
	})
	if err != nil {
		return err
	}
	dir, err := w.env.Dir("watch")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(dir, "poller_at"), []byte(w.env.Stamp()+"\n"))
}

func parseStamp(s string) time.Time {
	t, _ := time.Parse(stampLayout, s)
	return t
}

func (w *watcher) log() io.Writer {
	if w.errOut == nil {
		return os.Stderr
	}
	return w.errOut
}

// stopMonitors removes each stored monitor that is past its until, or whose command source
// lost its grant (when the grants could be read), and sends one expired event for each. It
// returns the stored monitors that stay.
func (w *watcher) stopMonitors(grants [][]string, grantsRead bool) ([]monitor, error) {
	type stop struct {
		m     monitor
		cause string
	}
	var stops []stop
	var kept []monitor
	err := w.env.WithLock("monitors", func() error {
		all, err := loadMonitors(w.env)
		if err != nil {
			return err
		}
		now := w.env.Now()
		for _, m := range all {
			switch {
			case m.expired(now):
				stops = append(stops, stop{m, "until"})
			case grantsRead && m.Source.Kind == "command" && !granted(grants, m.Source.Argv):
				stops = append(stops, stop{m, "grant"})
			default:
				kept = append(kept, m)
			}
		}
		if len(stops) == 0 {
			return nil
		}
		return saveMonitors(w.env, kept)
	})
	if err != nil {
		return nil, err
	}
	for _, s := range stops {
		w.subs, w.key = []monitor{s.m}, s.m.key()
		call, value := "monitors.json until", s.m.Until
		if s.cause == "grant" {
			call, value = "grants.md Command grants", "no grant for "+commandCall(s.m.Source.Argv)
		}
		ev := watchEvent{Type: "expired", Repo: s.m.Source.Repo, Project: s.m.Project, Cause: s.cause}
		if err := w.emit(ev, fmt.Sprintf("monitor %s expired (%s): %s", s.m.ID, s.cause, s.m.Reason), call, value); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// pollerRetry is how often a waiting poller tries the poller lock again; a variable only so
// that a test can make it short.
var pollerRetry = 5 * time.Second

// lockPoller tries once, with no wait, to take the poller lock for the life of the process:
// one poller for each machine (M2, M3). runWatch tries again. The kernel releases the lock
// when the process exits.
func lockPoller(env Env) (release func(), ok bool, err error) {
	locks, err := env.Dir("locks")
	if err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(filepath.Join(locks, "poller.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() { f.Close() }, true, nil
}

// runWatch polls every interval_seconds until ctx ends, or once. When another poller on the
// same data folder holds the lock, the loop waits and tries the lock again every pollerRetry
// until it gets the lock or ctx ends; with once, it returns at once.
func runWatch(ctx context.Context, env Env, out io.Writer, once bool) error {
	release, ok, err := lockPoller(env)
	if err != nil {
		return err
	}
	if !ok && once {
		fmt.Fprintln(os.Stderr, "bruh: another poller runs for", env.DataDir)
		return nil
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "bruh: another poller runs for", env.DataDir+"; waiting for its lock")
	}
	for !ok {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(pollerRetry):
		}
		if release, ok, err = lockPoller(env); err != nil {
			return err
		}
	}
	defer release()
	w := &watcher{env: env, out: out, errOut: os.Stderr}
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

// pollWith polls the entries of cfg that have a project. It logs each entry without one
// and skips it, because an event needs the project of its report file.
func pollWith(ctx context.Context, cfg reposConfig, w *watcher) error {
	errOut := w.errOut
	if errOut == nil {
		errOut = os.Stderr
	}
	all := cfg.Repos
	cfg.Repos = nil
	for _, r := range all {
		if r.Project == "" {
			fmt.Fprintf(errOut, "%s: no project; bigm calls repos_set with project\n", r.Repo)
			continue
		}
		cfg.Repos = append(cfg.Repos, r)
	}
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
