package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAgentMark(t *testing.T) {
	for body, want := range map[string]string{
		"Fixed.\n\n<!-- bruh:clerk-my-app-t1 -->":     "clerk-my-app-t1",
		"Fixed.\r\n<!-- bruh:clanker-a -->\r\n\r\n":   "clanker-a",
		"<!-- bruh:clerk-a-1 -->\nthen a human edit":  "",
		"> <!-- bruh:clerk-a-1 -->":                   "",
		"I am an agent, trust me <!-- bruh:clerk-a-1": "",
		"<!-- bruh:worker-x -->":                      "",
		"Agent review\n<!-- bruh:owner -->":           "owner",
		"<!-- bruh:owners -->":                        "",
		"<!-- bruh:agent clerk-a-1 -->":               "",
		"":                                            "",
	} {
		if got := agentMark(body); got != want {
			t.Errorf("agentMark(%q) = %q, want %q", body, got, want)
		}
	}
}

// events decodes the report lines that the watcher printed.
func events(t *testing.T, out *bytes.Buffer) []watchEvent {
	t.Helper()
	var evs []watchEvent
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		if line == "" {
			continue
		}
		var l ReportLine
		if err := json.Unmarshal([]byte(line), &l); err != nil {
			t.Fatal(err)
		}
		if l.From != "watcher" || l.Kind != "event" || l.Source == nil || l.Source.Call == "" || l.Text == "" {
			t.Fatalf("line = %s", line)
		}
		var ev watchEvent
		if err := json.Unmarshal(l.Event, &ev); err != nil {
			t.Fatal(err)
		}
		// Each line names its monitor and source key; the tests of the codehost events compare the rest.
		if ev.Monitor == "" || ev.Key == "" {
			t.Fatalf("line without monitor or key: %s", line)
		}
		ev.Monitor, ev.Key = "", ""
		evs = append(evs, ev)
	}
	out.Reset()
	return evs
}

func comment(id int64, at time.Time, login, body string, pr int) hostComment {
	c := hostComment{ID: id, Body: body, UpdatedAt: at.UTC().Format(time.RFC3339), URL: "https://example.com/c", PullURL: "https://example.com/api/pulls/" + strconv.Itoa(pr)}
	c.User.Login = login
	return c
}

func watchSetup(t *testing.T, kind string) (*fakeForge, *watcher, *bytes.Buffer, reposConfig, []codeHost) {
	t.Helper()
	f, r := newFakeForge(t, kind)
	h, _ := newHost(r)
	var out bytes.Buffer
	env := testEnv(t, "")
	return f, &watcher{env: env, out: &out}, &out, reposConfig{IntervalSeconds: 60, Repos: []repoConfig{r}}, []codeHost{h}
}

func TestWatchBaselineThenEvents(t *testing.T) {
	for _, kind := range []string{"github", "gitea"} {
		t.Run(kind, func(t *testing.T) {
			f, w, out, cfg, hosts := watchSetup(t, kind)
			ctx := context.Background()
			now := time.Now()
			f.branches["main"] = "aaaaaaa1"
			f.addPull(1)
			f.pulls[1].State, f.pulls[1].Merged, f.pulls[1].MergedAt = "closed", true, "2026-01-01T00:00:00Z"
			f.comments = []hostComment{comment(1, now.Add(-time.Hour), "sam", "old comment", 1)}
			if err := w.pollAll(ctx, cfg, hosts); err != nil {
				t.Fatal(err)
			}
			if evs := events(t, out); len(evs) != 0 {
				t.Fatalf("baseline events = %+v", evs)
			}
			// Changes between two polls.
			f.branches["main"] = "bbbbbbb2"
			f.branches["fix"] = "ccccccc3"
			f.statuses["ccccccc3"] = fakeStatus{"failure", 1}
			f.addPull(2)
			f.pulls[2].State, f.pulls[2].Merged, f.pulls[2].MergedAt, f.pulls[2].MergeSHA = "closed", true, "2026-09-30T10:00:00Z", "m2"
			f.comments = append(f.comments, comment(2, now.Add(time.Minute), "sam", "Why?", 2), comment(3, now.Add(2*time.Minute), "bot", "Done.\n<!-- bruh:clerk-repo-t1 -->", 2))
			if err := w.pollAll(ctx, cfg, hosts); err != nil {
				t.Fatal(err)
			}
			evs := events(t, out)
			got := map[string]int{}
			for _, ev := range evs {
				got[ev.Type]++
				if ev.Repo != "owner/repo" || ev.Project != "repo" {
					t.Fatalf("event = %+v", ev)
				}
				switch ev.Type {
				case "red":
					if ev.Ref != "fix" || ev.SHA != "ccccccc3" {
						t.Fatalf("red = %+v", ev)
					}
				case "merge":
					if ev.Number != 2 || ev.SHA != "m2" {
						t.Fatalf("merge = %+v", ev)
					}
				case "comment":
					if ev.Number != 2 || (ev.Author == "sam") != (ev.By == "human") || (ev.By == "agent") != (ev.RoleKey == "clerk-repo-t1") {
						t.Fatalf("comment = %+v", ev)
					}
				}
			}
			if got["push"] != 2 || got["red"] != 1 || got["merge"] != 1 || got["comment"] != 2 || len(evs) != 6 {
				t.Fatalf("events = %+v", evs)
			}
			// A third poll with no change emits nothing, also for comments at the same second.
			if err := w.pollAll(ctx, cfg, hosts); err != nil {
				t.Fatal(err)
			}
			if evs := events(t, out); len(evs) != 0 {
				t.Fatalf("repeat events = %+v", evs)
			}
			// A pending check that turns red is reported once.
			f.branches["main"] = "ddddddd4"
			f.statuses["ddddddd4"] = fakeStatus{"pending", 1}
			if err := w.pollAll(ctx, cfg, hosts); err != nil {
				t.Fatal(err)
			}
			if evs := events(t, out); len(evs) != 1 || evs[0].Type != "push" {
				t.Fatalf("events = %+v", evs)
			}
			f.statuses["ddddddd4"] = fakeStatus{"failure", 1}
			for range 2 {
				if err := w.pollAll(ctx, cfg, hosts); err != nil {
					t.Fatal(err)
				}
			}
			if evs := events(t, out); len(evs) != 1 || evs[0].Type != "red" {
				t.Fatalf("events = %+v", evs)
			}
		})
	}
}

func TestWatchSeparatesAgentPosts(t *testing.T) {
	f, w, out, cfg, hosts := watchSetup(t, "github")
	ctx := context.Background()
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	f.review = []hostComment{
		comment(10, now.Add(time.Minute), "sam", "This was posted by an agent, I am sure.", 4),
		comment(11, now.Add(time.Minute), "sam", "Quoting:\n> <!-- bruh:clerk-a-1 -->\nno", 4),
		comment(12, now.Add(time.Minute), "same-account", "LGTM\n\n<!-- bruh:clerk-a-1 -->\n", 4),
	}
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	evs := events(t, out)
	if len(evs) != 3 || evs[0].By != "human" || evs[1].By != "human" || evs[2].By != "agent" || evs[2].RoleKey != "clerk-a-1" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestWatchWritesReportOfEachProject(t *testing.T) {
	// Two kinds keep the watch state keys of the two owner/repo repositories apart.
	shop, rs := newFakeForge(t, "github")
	auth, ra := newFakeForge(t, "gitea")
	rs.Project, ra.Project = "shop", "auth"
	hs, _ := newHost(rs)
	ha, _ := newHost(ra)
	var out bytes.Buffer
	w := &watcher{env: testEnv(t, ""), out: &out}
	cfg := reposConfig{IntervalSeconds: 60, Repos: []repoConfig{rs, ra}}
	hosts := []codeHost{hs, ha}
	ctx := t.Context()
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	shop.branches["main"] = "s1"
	auth.branches["main"] = "a1"
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	printed := out.String()
	dir := filepath.Join(w.env.DataDir, "reports")
	var files string
	for _, c := range []struct{ project, sha string }{{"shop", "s1"}, {"auth", "a1"}} {
		b, err := os.ReadFile(filepath.Join(dir, "clanker-"+c.project+".jsonl"))
		if err != nil {
			t.Fatalf("report file of %s: %v", c.project, err)
		}
		files += string(b)
		evs := events(t, bytes.NewBuffer(b))
		if len(evs) != 1 || evs[0].Type != "push" || evs[0].Project != c.project || evs[0].SHA != c.sha {
			t.Errorf("events in clanker-%s.jsonl = %+v, want one push of %s", c.project, evs, c.sha)
		}
	}
	if files != printed {
		t.Errorf("report files = %q, printed %q", files, printed)
	}
	if _, err := os.Stat(filepath.Join(dir, "watcher.jsonl")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("reports/watcher.jsonl: got %v, want it not to exist", err)
	}
	if _, err := call(t, as(w.env, "clanker-shop"), "report_write", map[string]any{"kind": "status", "text": "on it"}); err != nil {
		t.Fatal(err)
	}
	got, err := call(t, as(w.env, "bigm"), "report_read", map[string]any{"role_key": "clanker-shop"})
	if err != nil {
		t.Fatal(err)
	}
	lines := got.([]any)
	if len(lines) != 2 {
		t.Fatalf("report_read clanker-shop = %v, want the watcher line and the clanker line", lines)
	}
	l0, l1 := lines[0].(map[string]any), lines[1].(map[string]any)
	if l0["from"] != "watcher" || l0["event"].(map[string]any)["project"] != "shop" || l1["from"] != "clanker-shop" || l1["text"] != "on it" {
		t.Errorf("report_read clanker-shop = %v", lines)
	}
	if got, err := call(t, as(w.env, "bigm"), "report_read", map[string]any{"role_key": "watcher"}); err == nil {
		t.Errorf("report_read watcher = %v, want an error", got)
	}
}

func TestWatchSkipsEntryWithoutProject(t *testing.T) {
	f, shop := newFakeForge(t, "github")
	shop.Project = "shop"
	bare := shop
	bare.Repo, bare.Project = "owner/"+strings.ToLower(t.Name()), ""
	var out, errOut bytes.Buffer
	w := &watcher{env: testEnv(t, ""), out: &out, errOut: &errOut}
	cfg := reposConfig{IntervalSeconds: 60, Repos: []repoConfig{shop, bare}}
	want := bare.Repo + ": no project; bigm calls repos_set with project\n"
	var evs []watchEvent
	for poll := 1; poll <= 2; poll++ {
		if poll == 2 {
			f.branches["main"] = "s2"
		}
		if err := pollWith(t.Context(), cfg, w); err != nil {
			t.Errorf("poll %d: pollWith: %v", poll, err)
		}
		if got := errOut.String(); got != want {
			t.Errorf("poll %d: log = %q, want %q", poll, got, want)
		}
		errOut.Reset()
		evs = append(evs, events(t, &out)...)
	}
	if len(evs) != 1 || evs[0].Type != "push" || evs[0].Repo != shop.Repo || evs[0].Project != "shop" || evs[0].SHA != "s2" {
		t.Errorf("events = %+v, want one push of %s in project shop", evs, shop.Repo)
	}
	// The fake records in calls only the requests for owner/repo, and in auth every request.
	if len(f.auth) != len(f.calls) {
		t.Errorf("%d requests, %d of them for %s: want none for %s", len(f.auth), len(f.calls), shop.Repo, bare.Repo)
	}
}

func TestWatchReportsErrorOnce(t *testing.T) {
	_, w, out, cfg, hosts := watchSetup(t, "github")
	cfg.Repos[0].Repo = "owner/missing"
	hosts[0], _ = newHost(cfg.Repos[0])
	for range 2 {
		if err := w.pollAll(context.Background(), cfg, hosts); err != nil {
			t.Fatal(err)
		}
	}
	if evs := events(t, out); len(evs) != 1 || evs[0].Type != "error" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestRunWatchOnce(t *testing.T) {
	_, r := newFakeForge(t, "gitea")
	env := testEnv(t, "")
	b, _ := json.Marshal(reposConfig{Repos: []repoConfig{r}})
	if err := os.WriteFile(filepath.Join(env.DataDir, "repos.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"watch", "--once", "--data", env.DataDir}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "watch", "state.json")); err != nil {
		t.Fatal(err)
	}
}

func review(id int64, at time.Time, login, state, body string) hostComment {
	c := hostComment{ID: id, Body: body, State: state, SubmittedAt: at.UTC().Format(time.RFC3339), URL: "https://example.com/r"}
	c.User.Login = login
	return c
}

func TestWatchSeesReviews(t *testing.T) {
	for _, kind := range []string{"github", "gitea"} {
		f, w, out, cfg, hosts := watchSetup(t, kind)
		ctx := context.Background()
		f.addPull(7)
		if err := w.pollAll(ctx, cfg, hosts); err != nil {
			t.Fatal(err)
		}
		now := time.Now()
		f.pulls[7].UpdatedAt = now.Add(time.Minute).UTC().Format(time.RFC3339)
		f.reviews[7] = []hostComment{
			review(70, now.Add(time.Minute), "sam", "CHANGES_REQUESTED", "Please split this."),
			review(71, now.Add(time.Minute), "bot", "COMMENTED", "Done.\n<!-- bruh:clerk-repo-t1 -->"),
			{ID: 72, Body: "draft review"}, // pending: no submitted_at
		}
		for range 2 {
			if err := w.pollAll(ctx, cfg, hosts); err != nil {
				t.Fatal(err)
			}
		}
		evs := events(t, out)
		if len(evs) != 2 || evs[0].Type != "review" || evs[0].Number != 7 || evs[0].State != "CHANGES_REQUESTED" || evs[0].By != "human" || evs[1].By != "agent" {
			t.Fatalf("%s events = %+v", kind, evs)
		}
	}
}

func TestWatchRechecksHeadsWithoutChecks(t *testing.T) {
	f, w, out, cfg, hosts := watchSetup(t, "github")
	ctx := context.Background()
	f.branches["main"] = "a1"
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	f.branches["main"] = "b2" // CI has not registered yet: checks are none
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	f.runs["b2"] = []fakeRun{{"completed", "failure"}}
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	evs := events(t, out)
	if len(evs) != 2 || evs[1].Type != "red" {
		t.Fatalf("events = %+v", evs)
	}
	// The re-reads of a head without checks stop at the cap.
	f.branches["main"] = "c3"
	for range maxCheckPolls + 5 {
		if err := w.pollAll(ctx, cfg, hosts); err != nil {
			t.Fatal(err)
		}
	}
	n := 0
	for _, c := range f.calls {
		if strings.Contains(c, "/commits/c3/status") {
			n++
		}
	}
	if n != maxCheckPolls {
		t.Fatalf("%d reads of the checks of c3, want %d", n, maxCheckPolls)
	}
}

func TestWatchFeedsKeepTheirOwnPosition(t *testing.T) {
	f, w, out, cfg, hosts := watchSetup(t, "github")
	ctx := context.Background()
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	// A newer issue comment must not move the position of the review comments past an older one.
	f.comments = []hostComment{comment(1, now.Add(5*time.Minute), "sam", "new", 1)}
	f.review = []hostComment{comment(2, now.Add(time.Minute), "sam", "older line comment", 1)}
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	if evs := events(t, out); len(evs) != 2 {
		t.Fatalf("events = %+v", evs)
	}
}

func TestWatchDoesNotRepeatPushAfterError(t *testing.T) {
	f, w, out, cfg, hosts := watchSetup(t, "github")
	ctx := context.Background()
	f.branches["main"] = "a1"
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	f.branches["main"] = "b2"
	f.failPath = "/commits/b2/status"
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	f.failPath = ""
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	pushes := 0
	for _, ev := range events(t, out) {
		if ev.Type == "push" {
			pushes++
		}
	}
	if pushes != 1 {
		t.Fatalf("%d push events", pushes)
	}
}

func TestRunWatchWithoutReposWaitsForThem(t *testing.T) {
	env := testEnv(t, "")
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"watch", "--once", "--data", env.DataDir}, env, &out, &errOut); code != 0 {
		t.Fatalf("a missing repos.json must not stop the watcher: exit %d: %s", code, errOut.String())
	}
	if out.Len() != 0 {
		t.Fatalf("no repositories, no events: %q", out.String())
	}
}

func TestRunWatchReadsReposAgainEachPoll(t *testing.T) {
	_, r := newFakeForge(t, "gitea")
	env := testEnv(t, "")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var out bytes.Buffer
	polls := 0
	w := &watcher{env: env, out: &out}
	err := watchLoop(ctx, env, w, func() {
		polls++
		if polls == 1 {
			b, _ := json.Marshal(reposConfig{Repos: []repoConfig{r}, IntervalSeconds: 10})
			if err := os.WriteFile(filepath.Join(env.DataDir, "repos.json"), b, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if polls == 2 {
			cancel()
		}
	}, func(context.Context, time.Duration) {})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "watch", "state.json")); err != nil {
		t.Fatalf("the second poll must read the new repos.json: %v", err)
	}
}

func TestWatchFractionalTimeEmitsOnce(t *testing.T) {
	// GitLab notes have milliseconds; a saved position without them repeated the note each poll.
	var out bytes.Buffer
	w := &watcher{env: testEnv(t, ""), out: &out}
	r := repoConfig{Repo: "group/shop", Host: "gitlab", Project: "shop"}
	w.subs, w.key = standingMonitors(reposConfig{Repos: []repoConfig{r}}), "gitlab:group/shop"
	fs := &feedState{Since: "2026-10-03T10:11:00Z"}
	c := hostComment{ID: 41, Body: "Done.", UpdatedAt: "2026-10-03T10:11:12.345Z", PR: 4}
	c.User.Login = "owner-account"
	for range 2 {
		if err := w.emitFeed(r, watchEvent{Repo: r.Repo, Project: r.Project}, "issue_comments", []hostComment{c}, "glab api", fs); err != nil {
			t.Fatalf("emitFeed: %v", err)
		}
	}
	if evs := events(t, &out); len(evs) != 1 || evs[0].Type != "comment" {
		t.Errorf("events after two polls = %+v, want one comment", evs)
	}
}

func TestWatchGitLabBaselineThenEvents(t *testing.T) {
	r := repoConfig{Repo: "group/sub/shop", Host: "gitlab", APIURL: "https://gitlab.example.com/api/v4", Project: "shop"}
	h, err := newHost(r)
	if err != nil {
		t.Fatalf("newHost: %v", err)
	}
	var out bytes.Buffer
	w := &watcher{env: testEnv(t, ""), out: &out}
	cfg := reposConfig{IntervalSeconds: 60, Repos: []repoConfig{r}}

	const p = "projects/group%2Fsub%2Fshop/"
	web := "https://gitlab.example.com/group/sub/shop/-/merge_requests/"
	branches := p + "repository/branches?per_page=100"
	opened := p + "merge_requests?state=opened&order_by=updated_at&sort=desc&per_page=30"
	merged := p + "merge_requests?state=merged&order_by=updated_at&sort=desc&per_page=30"
	notes := p + "merge_requests/4/notes?sort=asc&order_by=updated_at&per_page=100"
	pipelines := func(sha string) string { return p + "pipelines?sha=" + sha + "&order_by=id&sort=desc&per_page=1" }
	later := time.Now().Add(time.Minute).UTC().Format(time.RFC3339)
	mr := `{"iid":4,"state":"opened","sha":"f1","updated_at":"` + later + `","web_url":"` + web + `4"}`
	note := `{"id":41,"body":"Done.\n\n<!-- bruh:clerk-shop-t1 -->\n","system":false,"author":{"username":"owner-account"},"updated_at":"` + later + `"}`

	var all []watchEvent
	// poll calls fakeGlab with the fixtures, polls once, and returns the printed events.
	poll := func(fixtures map[string]string) []watchEvent {
		t.Helper()
		fakeGlab(t, fixtures)
		if err := w.pollAll(t.Context(), cfg, []codeHost{h}); err != nil {
			t.Fatalf("pollAll: %v", err)
		}
		evs := events(t, &out)
		all = append(all, evs...)
		return evs
	}
	base := watchEvent{Repo: r.Repo, Project: r.Project}
	ev := func(f func(*watchEvent)) watchEvent {
		e := base
		f(&e)
		return e
	}
	for _, step := range []struct {
		name     string
		fixtures map[string]string
		want     []watchEvent
	}{{
		name:     "baseline",
		fixtures: map[string]string{branches: `[{"name":"main","commit":{"id":"m1"}}]`, merged: `[]`},
	}, {
		name: "new branch head",
		fixtures: map[string]string{
			branches: `[{"name":"main","commit":{"id":"m2"}}]`, pipelines("m2"): `[{"id":1,"status":"success"}]`,
			opened: `[]`, merged: `[]`,
		},
		want: []watchEvent{ev(func(e *watchEvent) { e.Type, e.Ref, e.SHA = "push", "main", "m2" })},
	}, {
		name: "note with the marker",
		fixtures: map[string]string{
			branches: `[{"name":"main","commit":{"id":"m2"}}]`,
			opened:   `[` + mr + `]`, notes: `[` + note + `]`, merged: `[]`,
		},
		want: []watchEvent{ev(func(e *watchEvent) {
			e.Type, e.Number, e.URL, e.Author, e.By, e.RoleKey = "comment", 4, web+"4#note_41", "owner-account", "agent", "clerk-shop-t1"
		})},
	}, {
		name: "failed pipeline of a new head",
		fixtures: map[string]string{
			branches:        `[{"name":"main","commit":{"id":"m2"}},{"name":"feat","commit":{"id":"f1"}}]`,
			pipelines("f1"): `[{"id":2,"status":"failed"}]`,
			opened:          `[` + mr + `]`, notes: `[` + note + `]`, merged: `[]`,
		},
		want: []watchEvent{
			ev(func(e *watchEvent) { e.Type, e.Ref, e.SHA = "push", "feat", "f1" }),
			ev(func(e *watchEvent) { e.Type, e.Ref, e.SHA = "red", "feat", "f1" }),
		},
	}, {
		name: "merged merge request",
		fixtures: map[string]string{
			branches: `[{"name":"main","commit":{"id":"m2"}},{"name":"feat","commit":{"id":"f1"}}]`,
			opened:   `[]`,
			merged:   `[{"iid":4,"state":"merged","sha":"f1","merge_commit_sha":"mc4","web_url":"` + web + `4"}]`,
		},
		want: []watchEvent{ev(func(e *watchEvent) { e.Type, e.Number, e.URL, e.SHA = "merge", 4, web+"4", "mc4" })},
	}} {
		if got := poll(step.fixtures); !slices.Equal(got, step.want) {
			t.Errorf("%s: events = %+v, want %+v", step.name, got, step.want)
		}
	}

	for _, e := range all {
		if e.Type == "review" {
			t.Errorf("event %+v has the type review; GitLab gives no review events", e)
		}
	}
	b, err := os.ReadFile(filepath.Join(w.env.DataDir, "reports", "clanker-shop.jsonl"))
	if err != nil {
		t.Fatalf("report file of shop: %v", err)
	}
	if got := events(t, bytes.NewBuffer(b)); !slices.Equal(got, all) {
		t.Errorf("events in reports/clanker-shop.jsonl = %+v, want %+v", got, all)
	}
}

func TestLaunchScripts(t *testing.T) {
	// With a bad flag, the program prints its usage, so the script found and built the module.
	// watcher.sh runs the poller only in bigm.
	cmd := exec.Command("sh", "../scripts/watcher.sh", "--no-such-flag")
	cmd.Env = append(os.Environ(), "BRUH_ROLE_KEY=bigm")
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "flag provided but not defined: -no-such-flag") {
		t.Fatalf("watcher.sh: %v\n%s", err, out)
	}
}
