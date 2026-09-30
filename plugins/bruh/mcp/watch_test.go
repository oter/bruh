package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
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
			f.addPull(1, "old")
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
			f.addPull(2, "ccccccc3")
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

func TestWatchAppendsReport(t *testing.T) {
	f, w, out, cfg, hosts := watchSetup(t, "github")
	ctx := context.Background()
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	f.branches["main"] = "e1"
	if err := w.pollAll(ctx, cfg, hosts); err != nil {
		t.Fatal(err)
	}
	printed := out.String()
	b, err := os.ReadFile(filepath.Join(w.env.DataDir, "reports", "watcher.jsonl"))
	if err != nil || string(b) != printed {
		t.Fatalf("report = %q, printed %q, %v", b, printed, err)
	}
	lines, err := call(t, as(w.env, "bigm"), "report_read", map[string]any{"role_key": "watcher"})
	if err != nil {
		t.Fatal(err)
	}
	l := lines.([]any)[0].(map[string]any)
	if l["event"].(map[string]any)["type"] != "push" || !strings.Contains(l["source"].(map[string]any)["call"].(string), "/branches") {
		t.Fatalf("line = %v", l)
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
		f.addPull(7, "h7")
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
