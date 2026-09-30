package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func trainLines(t *testing.T, out *bytes.Buffer) []trainResult {
	t.Helper()
	var rs []trainResult
	for line := range strings.SplitSeq(strings.TrimSpace(out.String()), "\n") {
		var r trainResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		if r.Source.Call == "" || r.At == "" {
			t.Fatalf("no source: %+v", r)
		}
		rs = append(rs, r)
	}
	return rs
}

func runTrain(t *testing.T, f *fakeForge, r repoConfig, wait time.Duration, numbers ...int) ([]trainResult, bool) {
	t.Helper()
	h, _ := newHost(r)
	var out bytes.Buffer
	ok := mergeTrain(context.Background(), testEnv(t, ""), h, r, numbers, wait, 5*time.Millisecond, &out)
	return trainLines(t, &out), ok
}

func TestMergeTrainMergesInOrder(t *testing.T) {
	for _, kind := range []string{"github", "gitea"} {
		f, r := newFakeForge(t, kind)
		for _, n := range []int{3, 1, 2} {
			sha := strings.Repeat(string(rune('a'+n)), 40)
			f.addPull(n, sha)
			f.green(sha)
		}
		rs, ok := runTrain(t, f, r, time.Second, 3, 1, 2)
		if !ok || len(rs) != 3 {
			t.Fatalf("%s: ok %v, lines %+v", kind, ok, rs)
		}
		if len(f.merges) != 3 || f.merges[0]["number"] != 3 || f.merges[1]["number"] != 1 || f.merges[2]["number"] != 2 {
			t.Fatalf("%s merges = %v", kind, f.merges)
		}
		if rs[0].MergeSHA == "" || !strings.Contains(rs[0].Source.Call, "GET ") || !strings.HasSuffix(rs[0].Source.Call, "/pulls/3") {
			t.Fatalf("%s confirmation = %+v", kind, rs[0])
		}
	}
}

func TestMergeTrainStopsAtFirstSkip(t *testing.T) {
	f, r := newFakeForge(t, "github")
	f.addPull(4, "d4")
	f.pulls[4].Draft = true
	f.addPull(5, "e5")
	f.pulls[5].Mergeable = new(false)
	f.addPull(2, "g2")
	f.green("g2")
	for _, first := range []int{4, 5} {
		rs, ok := runTrain(t, f, r, time.Second, first, 2)
		if ok || len(rs) != 1 || rs[0].Result != "skipped" {
			t.Fatalf("first %d: ok %v, lines %+v", first, ok, rs)
		}
	}
	if len(f.merges) != 0 {
		t.Fatalf("merged after a skip: %v", f.merges)
	}
}

func TestMergeTrainRequiredChecks(t *testing.T) {
	f, r := newFakeForge(t, "github")
	f.addPull(1, "s1")
	f.green("s1")
	f.pulls[1].MergeableState = "blocked"
	rs, _ := runTrain(t, f, r, time.Second, 1)
	if rs[0].Reason != "mergeable_state_blocked" || len(f.merges) != 0 {
		t.Fatalf("lines = %+v, merges %v", rs, f.merges)
	}
	f.pulls[1].MergeableState = "unknown"
	go func() {
		time.Sleep(30 * time.Millisecond)
		f.mu.Lock()
		f.pulls[1].MergeableState = "clean"
		f.mu.Unlock()
	}()
	if rs, _ := runTrain(t, f, r, 5*time.Second, 1); rs[0].Result != "merged" {
		t.Fatalf("lines = %+v", rs)
	}
}

func TestMergeTrainChecks(t *testing.T) {
	f, r := newFakeForge(t, "gitea")
	f.addPull(1, "p1")
	f.statuses["p1"] = fakeStatus{"pending", 1}
	f.addPull(2, "r2")
	f.statuses["r2"] = fakeStatus{"failure", 1}
	f.addPull(3, "n3")
	go func() {
		time.Sleep(30 * time.Millisecond)
		f.green("p1")
	}()
	var got []string
	for _, n := range []int{1, 2, 3} {
		rs, _ := runTrain(t, f, r, 5*time.Second, n)
		got = append(got, rs[0].Result+" "+rs[0].Reason)
	}
	if got[0] != "merged " || got[1] != "skipped checks_failure" || got[2] != "skipped checks_none" {
		t.Fatalf("lines = %q", got)
	}
	f.addPull(4, "p4")
	f.statuses["p4"] = fakeStatus{"pending", 1}
	rs, _ := runTrain(t, f, r, 20*time.Millisecond, 4)
	if rs[0].Reason != "checks_timeout" {
		t.Fatalf("lines = %+v", rs)
	}
	if len(f.merges) != 1 {
		t.Fatalf("merges = %v", f.merges)
	}
}

func TestMergeTrainConfirmsByRead(t *testing.T) {
	f, r := newFakeForge(t, "github")
	f.mergeNoop = true
	for _, n := range []int{1, 2} {
		f.addPull(n, "s")
	}
	f.green("s")
	rs, ok := runTrain(t, f, r, time.Second, 1, 2)
	if ok || len(rs) != 1 || rs[0].Result != "not_confirmed" || rs[0].Source.Value != "merged: false" {
		t.Fatalf("ok %v, lines %+v", ok, rs)
	}
	f.mergeNoop, f.mergeFail = false, 409
	rs, _ = runTrain(t, f, r, time.Second, 2)
	if rs[0].Result != "skipped" || !strings.Contains(rs[0].Reason, "409") {
		t.Fatalf("lines = %+v", rs)
	}
}

// gateEnv returns an environment of the merger clerk of the fake repository, with repos.json and
// a ledger whose grants.md has rows.
func gateEnv(t *testing.T, r repoConfig, grants string) Env {
	t.Helper()
	env := testEnv(t, "clerk-repo-merge")
	b, _ := json.Marshal(reposConfig{Repos: []repoConfig{r}})
	ledger := t.TempDir()
	c, _ := json.Marshal(initConfig{LedgerPath: ledger})
	dir, _ := env.Dir("init")
	for f, data := range map[string][]byte{
		filepath.Join(env.DataDir, "repos.json"): b,
		filepath.Join(dir, "config.json"):        c,
		filepath.Join(ledger, "grants.md"):       []byte("# Merge grants\n\n| Repository | Merger role key | Conditions | Owner words | Date (UTC) | Question ID |\n|---|---|---|---|---|---|\n" + grants),
	} {
		if err := os.WriteFile(f, data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return env
}

func TestMergeTrainCLI(t *testing.T) {
	f, r := newFakeForge(t, "github")
	f.addPull(9, "s9")
	f.green("s9")
	env := gateEnv(t, r, "| owner/repo | clerk-repo-merge | CI green | CI green | 2026-09-30T10:00:00Z | init |\n")
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"merge-train", "--data", env.DataDir, "owner/repo", "9"}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s %s", code, out.String(), errOut.String())
	}
	if rs := trainLines(t, &out); rs[0].Result != "merged" {
		t.Fatalf("lines = %+v", rs)
	}
	for _, args := range [][]string{{"merge-train", "owner/repo"}, {"merge-train", "owner/other", "1"}, {"merge-train", "owner/repo", "x"}} {
		if code := runCLI(append(args[:1:1], append([]string{"--data", env.DataDir}, args[1:]...)...), env, &out, &errOut); code == 0 {
			t.Errorf("%v: exit 0", args)
		}
	}
}

func TestMergeGate(t *testing.T) {
	_, r := newFakeForge(t, "github")
	grant := "| `owner/repo` | `clerk-repo-merge` | CI green | CI green | 2026-09-30T10:00:00Z | init |\n"
	env := gateEnv(t, r, grant)
	if err := mergeGate(env, r, ""); err != nil {
		t.Fatalf("grant: %v", err)
	}
	for _, key := range []string{"", "clerk-repo-t1", "clanker-repo", "clerk-other-merge"} {
		mustErr(t, mergeGate(as(env, key), r, ""), "merger clerk")
	}
	env = gateEnv(t, r, "| owner/repo | clerk-repo-other | CI green | x | y | init |\n| owner/other | clerk-repo-merge | x | x | y | init |\n")
	mustErr(t, mergeGate(env, r, ""), "no merge grant")
	// A P1 answer of bigm in the mailbox opens the gate; an answer from another role does not.
	mustErr(t, mergeGate(env, r, "Q-4"), "no ANSWER Q-4")
	if _, err := call(t, as(env, "clanker-repo"), "mail_post", map[string]any{"to": "clerk-repo-merge", "header": "ANSWER Q-4: merge it", "body": "yes"}); err != nil {
		t.Fatal(err)
	}
	mustErr(t, mergeGate(env, r, "Q-4"), "no ANSWER Q-4")
	if _, err := call(t, as(env, "bigm"), "mail_post", map[string]any{"to": "clerk-repo-merge", "header": "ANSWER Q-4: merge it", "body": "owner: yes"}); err != nil {
		t.Fatal(err)
	}
	if err := mergeGate(env, r, "Q-4"); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "mail_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := mergeGate(env, r, "Q-4"); err != nil {
		t.Fatalf("after mail_read: %v", err)
	}
	mustErr(t, mergeGate(env, r, "Q-40"), "no ANSWER Q-40")
}

func TestLaunchScripts(t *testing.T) {
	for _, script := range []string{"merge-train.sh", "watcher.sh"} {
		// With a bad flag, the program prints its usage, so the script found and built the module.
		out, err := exec.Command("sh", "../scripts/"+script, "--no-such-flag").CombinedOutput()
		if err == nil || !strings.Contains(string(out), "flag provided but not defined: -no-such-flag") {
			t.Fatalf("%s: %v\n%s", script, err, out)
		}
	}
}
