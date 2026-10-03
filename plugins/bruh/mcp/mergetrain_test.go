package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	if err := mergeGate(env, r, "", []int{9}); err != nil {
		t.Fatalf("grant: %v", err)
	}
	for _, key := range []string{"", "clerk-repo-t1", "clanker-repo", "clerk-other-merge"} {
		mustErr(t, mergeGate(as(env, key), r, "", []int{9}), "merger clerk")
	}
	env = gateEnv(t, r, "| owner/repo | clerk-repo-other | CI green | x | y | init |\n| owner/other | clerk-repo-merge | x | x | y | init |\n")
	mustErr(t, mergeGate(env, r, "", []int{9}), "no merge grant")
	// An approval of bigm in the mailbox opens the gate; the same header from another role does not.
	mustErr(t, mergeGate(env, r, "Q-repo-host-4", []int{9}), "no ANSWER Q-repo-host-4")
	post := func(from, header string) {
		t.Helper()
		if _, err := call(t, as(env, from), "mail_post", map[string]any{"to": "clerk-repo-merge", "header": header, "body": "owner: yes"}); err != nil {
			t.Fatal(err)
		}
	}
	post("clanker-repo", "ANSWER Q-repo-host-4: merge owner/repo#9 approved")
	mustErr(t, mergeGate(env, r, "Q-repo-host-4", []int{9}), "no ANSWER Q-repo-host-4")
	post("bigm", "ANSWER Q-repo-host-4: merge owner/repo#9,#11 approved")
	if err := mergeGate(env, r, "Q-repo-host-4", []int{9}); err != nil {
		t.Fatal(err)
	}
	if err := mergeGate(env, r, "Q-repo-host-4", []int{11, 9}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "mail_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	if err := mergeGate(env, r, "Q-repo-host-4", []int{9}); err != nil {
		t.Fatalf("after mail_read: %v", err)
	}
	mustErr(t, mergeGate(env, r, "Q-repo-host-40", []int{9}), "no ANSWER Q-repo-host-40")
	// A GitLab path with subgroups matches the first cell of its grant row as is (A13.3).
	gitlab := r
	gitlab.Host, gitlab.Repo = "gitlab", "group/sub/app"
	env = gateEnv(t, gitlab, "| group/sub/app | clerk-repo-merge | CI green | CI green | 2026-09-30T10:00:00Z | init |\n")
	if err := mergeGate(env, gitlab, "", []int{9}); err != nil {
		t.Fatalf("GitLab grant: %v", err)
	}
}

// TestMergeGateRefusesUncheckedAlias pins the identity rule of spec 8.5 (A13.2, G10): when the
// chosen remote of the repository in the index uses an SSH host alias, the gate needs a confirmed
// account for that alias in "Identities" of the project file before it checks the grant.
func TestMergeGateRefusesUncheckedAlias(t *testing.T) {
	r := repoConfig{Repo: "owner/repo", Host: "gitlab", Project: "repo", MergeMethod: "merge"}
	const (
		alias    = "git@gitlab.com-work:owner/repo.git"
		aliasErr = `the remote of owner/repo uses the SSH host alias gitlab.com-work, so its account is not checked; the owner confirms the account in "Identities" of projects/repo.md (a row for gitlab.com-work)`
		head     = "# repo\n\n## Identities\n\n| Alias | Account | Confirmed |\n|---|---|---|\n"
	)
	gitlabCom := "gitlab.com"
	for _, c := range []struct {
		name       string
		url        string
		host       *string
		identities string // the project file; "" writes none
		noIndex    bool
		wantErr    string // "" means no error
	}{
		{"alias, no row", alias, &gitlabCom, head, false, aliasErr},
		{"alias, no project file", alias, &gitlabCom, "", false, aliasErr},
		{"alias, empty identity", alias, &gitlabCom, head + "| `gitlab.com-work` |  | 2026-10-01 |\n", false, aliasErr},
		{"alias, confirmed", alias, &gitlabCom, head + "| `gitlab.com-work` | work-account | 2026-10-01 |\n", false, ""},
		{"alias, host null", alias, nil, head, false, aliasErr},
		{"https remote", "https://gitlab.com/owner/repo.git", &gitlabCom, head, false, ""},
		{"no index file", alias, &gitlabCom, head, true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := gateEnv(t, r, "| owner/repo | clerk-repo-merge | CI green | CI green | 2026-09-30T10:00:00Z | init |\n")
			ledger, err := ledgerPath(env)
			if err != nil {
				t.Fatal(err)
			}
			if !c.noIndex {
				pf := &projectFile{Key: "repo", Main: "repo", Repos: []indexRepo{{
					Path:     "repo",
					Remotes:  []remote{{Name: "origin", URL: c.url}},
					Remote:   "origin",
					Host:     hostValue{Value: c.host, Source: "git"},
					Kind:     "gitlab",
					HostPath: "owner/repo",
					State:    "ok",
				}}}
				if _, err := writeLearnFile(filepath.Join(ledger, "learn", "projects", "repo.json"), pf); err != nil {
					t.Fatal(err)
				}
			}
			if c.identities != "" {
				if err := os.MkdirAll(filepath.Join(ledger, "projects"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(ledger, "projects", "repo.md"), []byte(c.identities), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			err = mergeGate(env, r, "", []int{9})
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("mergeGate(%s) = %v, want nil", c.name, err)
				}
				return
			}
			if err == nil || err.Error() != c.wantErr {
				t.Fatalf("mergeGate(%s) = %v, want %q", c.name, err, c.wantErr)
			}
		})
	}
}

// TestMergeTrainRefusesEntryWithoutProject pins G2: a repos.json entry of version 0.5 with no
// project gets no merge, and the train calls no code host.
func TestMergeTrainRefusesEntryWithoutProject(t *testing.T) {
	f, r := newFakeForge(t, "github")
	f.addPull(9, "s9")
	f.green("s9")
	env := gateEnv(t, r, "| owner/repo | clerk-repo-merge | CI green | CI green | 2026-09-30T10:00:00Z | init |\n")
	repos := `{"repos":[{"repo":"owner/repo","host":"github","api_url":"` + r.APIURL + `","merge_method":"squash"}]}`
	if err := os.WriteFile(filepath.Join(env.DataDir, "repos.json"), []byte(repos), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code := runCLI([]string{"merge-train", "--data", env.DataDir, "owner/repo", "9"}, env, &out, &errOut)
	if want := "owner/repo: no project; bigm calls repos_set with project"; code == 0 || !strings.Contains(errOut.String(), want) {
		t.Fatalf("runCLI(merge-train owner/repo 9) = exit %d, stderr %q; want non-zero exit and stderr with %q", code, errOut.String(), want)
	}
	if len(f.calls) != 0 {
		t.Fatalf("code host calls = %v", f.calls)
	}
}

// Final review M2: the ANSWER must approve these exact pull requests of this repository.
func TestMergeGateNeedsApprovalOfEachPull(t *testing.T) {
	_, r := newFakeForge(t, "github")
	env := gateEnv(t, r, "")
	gitlab := r
	gitlab.Host, gitlab.Repo = "gitlab", "group/sub/app" // a GitLab path with a subgroup (A13.4)
	for i, c := range []struct {
		repo    repoConfig
		header  string
		numbers []int
		ok      bool
	}{
		{r, "merge owner/repo#12 approved", []int{12}, true},
		{r, "merge owner/repo#3,#12 approved", []int{3, 12}, true},
		{r, "merge owner/repo#12 refused", []int{12}, false},                // a no
		{r, "merge owner/repo#3? No, do not merge.", []int{12}, false},      // the probe of the review
		{r, "merge owner/repo#3 approved", []int{12}, false},                // another pull request
		{r, "merge owner/repo#12 approved", []int{12, 13}, false},           // one number of the train is not named
		{r, "merge owner/other#12 approved", []int{12}, false},              // another repository
		{r, "merge owner/repo#12 approved, also #13", []int{12, 13}, false}, // not the closed grammar
		{r, "merge owner/repo#012 approved", []int{12}, false},
		{gitlab, "merge group/sub/app#12 approved", []int{12}, true},
	} {
		qid := fmt.Sprintf("Q-repo-host-%d", i+1)
		if _, err := call(t, as(env, "bigm"), "mail_post", map[string]any{"to": "clerk-repo-merge", "header": "ANSWER " + qid + ": " + c.header, "body": "owner words"}); err != nil {
			t.Fatal(err)
		}
		if err := mergeGate(env, c.repo, qid, c.numbers); (err == nil) != c.ok {
			t.Errorf("%q with %v: err = %v, want ok = %v", c.header, c.numbers, err, c.ok)
		}
	}
	// The old ID form Q-<n> is refused (spec 5): the mailbox does not take it, and the gate does not read it.
	if _, err := call(t, as(env, "bigm"), "mail_post", map[string]any{"to": "clerk-repo-merge", "header": "ANSWER Q-4: merge owner/repo#9 approved", "body": "owner words"}); err == nil {
		t.Error("mail_post took the old ID ANSWER Q-4")
	}
	if err := mergeGate(env, r, "Q-4", []int{9}); err == nil {
		t.Error("mergeGate opened for the old ID Q-4")
	}
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

// TestMergeTrainGitLab pins the G8 rule of the merge train on GitLab (spec 8.5, A13.1): it merges
// only on detailed_merge_status mergeable, waits on checking, skips each other value, and confirms
// the merge by a read of state merged.
func TestMergeTrainGitLab(t *testing.T) {
	const head = "0123456789abcdef0123456789abcdef01234567"
	r := repoConfig{Repo: "group/sub/shop", Host: "gitlab", APIURL: "https://gitlab.example.com/api/v4", Project: "shop", MergeMethod: "merge"}
	for _, c := range []struct {
		name          string
		first, then   string // detailed_merge_status of the first two reads, and of each later read
		result        string
		reason, value string
		puts          int
		pipelineReads int
	}{
		{"mergeable", "mergeable", "mergeable", "merged", "", "merged: true", 1, 1},
		{"not approved", "not_approved", "not_approved", "skipped", "detailed_merge_status_not_approved", "detailed_merge_status: not_approved", 0, 1},
		// The first read is the one before the checks; the second, after a green pipeline, still
		// says checking, so the train must read the pipeline again before it merges.
		{"checking then mergeable", "checking", "mergeable", "merged", "", "merged: true", 1, 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			// The merge call logs its stdin and marks the merge request merged; a read of the merge
			// request counts itself in the file reads.
			log := fakeCLI(t, &glabBin, `dir=$(dirname "$log")
case "$4" in
projects/group%2Fsub%2Fshop/merge_requests/7/merge)
	case "$*" in *"-X PUT"*) ;; *) echo 'not a PUT' >&2; exit 1 ;; esac
	echo "stdin: $(cat)" >> "$log"
	echo PUT >> "$dir/puts"
	: > "$dir/merged"
	echo '{}' ;;
projects/group%2Fsub%2Fshop/merge_requests/7)
	n=$(( $(cat "$dir/reads" 2>/dev/null || echo 0) + 1 ))
	echo "$n" > "$dir/reads"
	if [ -e "$dir/merged" ]; then
		echo '{"iid":7,"state":"merged","sha":"`+head+`","merge_commit_sha":"m7"}'
		exit 0
	fi
	status=`+c.then+`
	[ "$n" -gt 2 ] || status=`+c.first+`
	echo '{"iid":7,"state":"opened","draft":false,"sha":"`+head+`","detailed_merge_status":"'"$status"'"}' ;;
projects/group%2Fsub%2Fshop/pipelines\?*)
	echo pipeline >> "$dir/pipelines"
	echo '[{"status":"success"}]' ;;
*) echo '404 Not Found' >&2; exit 1 ;;
esac`)
			dir := filepath.Dir(log)
			rs, ok := runTrain(t, nil, r, 5*time.Second, 7)
			if len(rs) != 1 || rs[0].Result != c.result || rs[0].Reason != c.reason || rs[0].Source.Value != c.value || ok != (c.result == "merged") {
				t.Fatalf("ok %v, lines %+v", ok, rs)
			}
			count := func(name string) int {
				data, _ := os.ReadFile(filepath.Join(dir, name))
				return strings.Count(string(data), "\n")
			}
			if got := count("puts"); got != c.puts {
				t.Fatalf("PUT calls = %d, want %d", got, c.puts)
			}
			if got := count("pipelines"); got != c.pipelineReads {
				t.Errorf("pipeline reads = %d, want %d", got, c.pipelineReads)
			}
			data, _ := os.ReadFile(log)
			if stdin := `stdin: {"sha":"` + head + `"}`; c.puts == 1 && !strings.Contains(string(data), stdin+"\n") {
				t.Errorf("log %q has no %q", data, stdin)
			}
			if c.result == "merged" && (rs[0].MergeSHA != "m7" || !strings.HasSuffix(rs[0].Source.Call, "/merge_requests/7")) {
				t.Errorf("confirmation = %+v", rs[0])
			}
		})
	}
}
