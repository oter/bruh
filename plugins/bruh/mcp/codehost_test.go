package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type fakeStatus struct {
	State string
	Total int
}

type fakeRun struct {
	Status     string
	Conclusion string // empty means null
}

// fakeForge is a small GitHub or Gitea API for one repository, owner/repo.
type fakeForge struct {
	kind       string // github or gitea
	mu         sync.Mutex
	branches   map[string]string
	comments   []hostComment
	review     []hostComment // GitHub pull request review comments
	pulls      map[int]*hostPull
	statuses   map[string]fakeStatus
	runs       map[string][]fakeRun
	mergeNoop  bool // the merge call answers 200 and merges nothing
	mergeFail  int  // HTTP status of the merge call, when not 0
	merges     []map[string]any
	auth       []string
	calls      []string
	lastSince  string
	mergeCount int
}

func newFakeForge(t *testing.T, kind string) (*fakeForge, repoConfig) {
	// The fake runs on 127.0.0.1; name it as a GitHub host and give both hosts a token, so no
	// test reaches the gh fallback.
	t.Setenv("BRUH_GITHUB_HOSTS", "127.0.0.1")
	t.Setenv("GITHUB_TOKEN", "tok")
	t.Setenv("BRUH_GITEA_TOKEN_127_0_0_1", "tok")
	f := &fakeForge{kind: kind, branches: map[string]string{}, pulls: map[int]*hostPull{}, statuses: map[string]fakeStatus{}, runs: map[string][]fakeRun{}}
	srv := httptest.NewTLSServer(f)
	t.Cleanup(srv.Close)
	old := hostHTTP
	hostHTTP = srv.Client() // trusts the test certificate of every httptest TLS server
	t.Cleanup(func() { hostHTTP = old })
	api := srv.URL
	if kind == "gitea" {
		api += "/api/v1"
	}
	return f, repoConfig{Repo: "owner/repo", Host: kind, APIURL: api, Project: "repo", MergeMethod: "squash"}
}

func (f *fakeForge) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.auth = append(f.auth, r.Header.Get("Authorization"))
	prefix := "/repos/owner/repo"
	if f.kind == "gitea" {
		prefix = "/api/v1" + prefix
	}
	p, ok := strings.CutPrefix(r.URL.Path, prefix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	f.calls = append(f.calls, r.Method+" "+p)
	send := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	parts := strings.Split(strings.Trim(p, "/"), "/")
	switch {
	case r.Method == "GET" && p == "/branches":
		var out []map[string]any
		for name, sha := range f.branches {
			c := map[string]string{"sha": sha}
			if f.kind == "gitea" {
				c = map[string]string{"id": sha}
			}
			out = append(out, map[string]any{"name": name, "commit": c})
		}
		send(out)
	case r.Method == "GET" && (p == "/issues/comments" || p == "/pulls/comments"):
		f.lastSince = r.URL.Query().Get("since")
		src := f.comments
		if p == "/pulls/comments" {
			src = f.review
		}
		out := []hostComment{}
		for _, c := range src {
			if c.UpdatedAt >= f.lastSince {
				out = append(out, c)
			}
		}
		send(out)
	case r.Method == "GET" && p == "/pulls":
		out := []hostPull{}
		for _, pr := range f.pulls {
			if pr.State == "closed" {
				out = append(out, *pr)
			}
		}
		send(out)
	case len(parts) == 2 && parts[0] == "pulls" && r.Method == "GET":
		n, _ := strconv.Atoi(parts[1])
		pr, ok := f.pulls[n]
		if !ok {
			http.NotFound(w, r)
			return
		}
		send(pr)
	case len(parts) == 3 && parts[0] == "pulls" && parts[2] == "merge":
		n, _ := strconv.Atoi(parts[1])
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		body["number"] = n
		f.merges = append(f.merges, body)
		if f.mergeFail != 0 {
			http.Error(w, `{"message":"Base branch was modified"}`, f.mergeFail)
			return
		}
		if pr := f.pulls[n]; pr != nil && !f.mergeNoop {
			f.mergeCount++
			pr.State, pr.Merged, pr.MergedAt, pr.MergeSHA = "closed", true, "2026-09-30T10:00:00Z", fmt.Sprintf("m%039d", f.mergeCount)
		}
		send(map[string]any{"merged": true, "message": "Pull Request successfully merged"})
	case len(parts) == 3 && parts[0] == "commits" && parts[2] == "status":
		st := f.statuses[parts[1]]
		send(map[string]any{"state": st.State, "total_count": st.Total})
	case len(parts) == 3 && parts[0] == "commits" && parts[2] == "check-runs":
		runs := []map[string]any{}
		for _, run := range f.runs[parts[1]] {
			m := map[string]any{"status": run.Status, "conclusion": nil}
			if run.Conclusion != "" {
				m["conclusion"] = run.Conclusion
			}
			runs = append(runs, m)
		}
		send(map[string]any{"total_count": len(runs), "check_runs": runs})
	default:
		http.NotFound(w, r)
	}
}

func (f *fakeForge) addPull(n int, sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pulls[n] = &hostPull{Number: n, State: "open", Mergeable: new(true), URL: fmt.Sprintf("https://example.com/owner/repo/pull/%d", n)}
	f.pulls[n].Head.SHA = sha
}

func (f *fakeForge) green(sha string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.kind == "github" {
		f.runs[sha] = []fakeRun{{"completed", "success"}}
	} else {
		f.statuses[sha] = fakeStatus{"success", 1}
	}
}

func TestLoadRepos(t *testing.T) {
	dir := t.TempDir()
	write := func(s string) error {
		if err := os.WriteFile(filepath.Join(dir, "repos.json"), []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := loadRepos(dir)
		return err
	}
	if err := write(`{"repos":[{"repo":"owner/app","host":"github"},{"repo":"o/x","host":"gitea","api_url":"https://git.example.com/api/v1/"}]}`); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadRepos(dir)
	gh, gt := cfg.Repos[0], cfg.Repos[1]
	if cfg.IntervalSeconds != 60 || gh.APIURL != "https://api.github.com" || gh.Project != "app" || gh.MergeMethod != "merge" ||
		gt.APIURL != "https://git.example.com/api/v1" {
		t.Fatalf("cfg = %+v", cfg)
	}
	for _, bad := range []string{`{"repos":[{"repo":"x","host":"github"}]}`, `{"repos":[{"repo":"o/x","host":"gitea"}]}`, `{"repos":[{"repo":"o/x","host":"gitlab"}]}`, `{"repo":[]}`,
		`{"repos":[{"repo":"o/x","host":"github","api_url":"http://attacker.example.com"}]}`,
		`{"repos":[{"repo":"o/x","host":"gitea","api_url":"https://u:p@git.example.com/api/v1"}]}`,
		`{"repos":[{"repo":"o/x","host":"github","token_env":"GITHUB_TOKEN"}]}`} {
		if err := write(bad); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

func TestCodeHostChecksGitHub(t *testing.T) {
	f, r := newFakeForge(t, "github")
	h, _ := newHost(r)
	f.statuses["a"] = fakeStatus{"pending", 0} // no statuses: the combined state is pending
	f.runs["a"] = []fakeRun{{"completed", "success"}, {"completed", "skipped"}}
	f.statuses["b"] = fakeStatus{"success", 2}
	f.runs["b"] = []fakeRun{{"in_progress", ""}}
	f.runs["c"] = []fakeRun{{"completed", "timed_out"}}
	f.statuses["d"] = fakeStatus{"failure", 1}
	f.statuses["e"] = fakeStatus{"pending", 1}
	for sha, want := range map[string]string{"a": "success", "b": "pending", "c": "failure", "d": "failure", "e": "pending", "none": "none"} {
		got, err := h.Checks(context.Background(), sha)
		if err != nil || got != want {
			t.Errorf("Checks(%s) = %q, %v; want %q", sha, got, err, want)
		}
	}
	if f.auth[0] != "Bearer tok" {
		t.Fatalf("auth = %q", f.auth[0])
	}
}

func TestCodeHostChecksGitea(t *testing.T) {
	f, r := newFakeForge(t, "gitea")
	h, _ := newHost(r)
	f.statuses["a"] = fakeStatus{"success", 1}
	f.statuses["b"] = fakeStatus{"pending", 2}
	f.statuses["c"] = fakeStatus{"error", 1}
	f.statuses["d"] = fakeStatus{"warning", 1}
	for sha, want := range map[string]string{"a": "success", "b": "pending", "c": "failure", "d": "failure", "none": "none"} {
		if got, _ := h.Checks(context.Background(), sha); got != want {
			t.Errorf("Checks(%s) = %q, want %q", sha, got, want)
		}
	}
	if f.auth[0] != "token tok" {
		t.Fatalf("auth = %q", f.auth[0])
	}
}

func TestCodeHostMergeBodies(t *testing.T) {
	for kind, want := range map[string]string{
		"github": `map[merge_method:squash number:7 sha:abc]`,
		"gitea":  `map[Do:squash head_commit_id:abc number:7]`,
	} {
		f, r := newFakeForge(t, kind)
		f.addPull(7, "abc")
		h, _ := newHost(r)
		if err := h.Merge(context.Background(), 7, "abc", "squash"); err != nil {
			t.Fatal(err)
		}
		if got := fmt.Sprint(f.merges[0]); got != want {
			t.Errorf("%s merge body = %s, want %s", kind, got, want)
		}
		wantCall := "PUT " + r.APIURL + "/repos/owner/repo/pulls/7/merge"
		if kind == "gitea" {
			wantCall = "POST " + r.APIURL + "/repos/owner/repo/pulls/7/merge"
		}
		if h.LastCall() != wantCall {
			t.Errorf("last call = %q, want %q", h.LastCall(), wantCall)
		}
		f.mergeFail = 405
		if err := h.Merge(context.Background(), 7, "abc", "squash"); err == nil || !strings.Contains(err.Error(), "405") {
			t.Errorf("%s: err = %v", kind, err)
		}
	}
}

func TestHostTokenGoesOnlyToItsHost(t *testing.T) {
	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$*\" = \"auth token --hostname $WANT\" ] && echo gh-token\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := ghBin
	ghBin = gh
	t.Cleanup(func() { ghBin = old })
	tok := func(host, api string) string { return hostToken(repoConfig{Host: host, APIURL: api}) }
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("BRUH_GITHUB_HOSTS", "ghe.example.com")
	t.Setenv("WANT", "github.com")
	if got := tok("github", "https://api.github.com"); got != "gh-token" {
		t.Fatalf("github.com fallback = %q", got)
	}
	t.Setenv("WANT", "ghe.example.com")
	if got := tok("github", "https://ghe.example.com/api/v3"); got != "gh-token" {
		t.Fatalf("enterprise fallback = %q", got)
	}
	t.Setenv("GITHUB_TOKEN", "env-token")
	t.Setenv("BRUH_GITEA_TOKEN_GIT_EXAMPLE_COM", "gitea-token")
	for _, c := range [][3]string{
		{"github", "https://api.github.com", "env-token"},
		{"github", "https://attacker.example.com", ""},
		{"github", "http://api.github.com", ""},
		{"gitea", "https://git.example.com/api/v1", "gitea-token"},
		{"gitea", "https://attacker.example.com/api/v1", ""},
		{"gitea", "https://api.github.com", ""},
	} {
		if got := tok(c[0], c[1]); got != c[2] {
			t.Errorf("%s %s: token %q, want %q", c[0], c[1], got, c[2])
		}
	}
}

func TestReposEntryElsewhereGetsNoToken(t *testing.T) {
	f, r := newFakeForge(t, "github")
	t.Setenv("BRUH_GITHUB_HOSTS", "")
	h, _ := newHost(r)
	if _, err := h.Branches(context.Background()); err != nil {
		t.Fatal(err)
	}
	if f.auth[0] != "" {
		t.Fatalf("a token went to %s: %q", r.APIURL, f.auth[0])
	}
}

func TestReposSet(t *testing.T) {
	env := testEnv(t, "bigm")
	set := func(env Env, args map[string]any) error {
		_, err := call(t, env, "repos_set", args)
		return err
	}
	if err := set(env, map[string]any{"repo": "owner/app", "host": "github", "project": "app"}); err != nil {
		t.Fatal(err)
	}
	if err := set(env, map[string]any{"repo": "o/x", "host": "gitea", "api_url": "https://git.example.com/api/v1", "interval_seconds": 30}); err != nil {
		t.Fatal(err)
	}
	if err := set(env, map[string]any{"repo": "owner/app", "host": "github", "merge_method": "squash"}); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadRepos(env.DataDir)
	if err != nil || len(cfg.Repos) != 2 || cfg.Repos[1].Repo != "owner/app" || cfg.Repos[1].MergeMethod != "squash" || cfg.IntervalSeconds != 30 {
		t.Fatalf("cfg = %+v, %v", cfg, err)
	}
	mustErr(t, set(env, map[string]any{"repo": "o/y", "host": "gitea"}), "api_url is required")
	mustErr(t, set(env, map[string]any{"repo": "o/y", "host": "github", "api_url": "http://attacker.example.com"}), "https")
	mustErr(t, set(as(env, "clanker-a"), map[string]any{"repo": "o/y", "host": "github"}), "only bigm")
	if err := set(env, map[string]any{"repo": "o/x", "remove": true}); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := loadRepos(env.DataDir); len(cfg.Repos) != 1 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "repos.json.check")); err == nil {
		t.Fatal("check file left")
	}
}
