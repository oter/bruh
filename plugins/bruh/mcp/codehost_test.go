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
	t.Setenv("BRUH_TEST_TOKEN", "tok") // so no test reaches the gh fallback
	f := &fakeForge{kind: kind, branches: map[string]string{}, pulls: map[int]*hostPull{}, statuses: map[string]fakeStatus{}, runs: map[string][]fakeRun{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	api := srv.URL
	if kind == "gitea" {
		api += "/api/v1"
	}
	return f, repoConfig{Repo: "owner/repo", Host: kind, APIURL: api, TokenEnv: "BRUH_TEST_TOKEN", Project: "repo", MergeMethod: "squash"}
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
	if cfg.IntervalSeconds != 60 || gh.APIURL != "https://api.github.com" || gh.TokenEnv != "GITHUB_TOKEN" || gh.Project != "app" || gh.MergeMethod != "merge" ||
		gt.APIURL != "https://git.example.com/api/v1" || gt.TokenEnv != "GITEA_TOKEN" {
		t.Fatalf("cfg = %+v", cfg)
	}
	for _, bad := range []string{`{"repos":[{"repo":"x","host":"github"}]}`, `{"repos":[{"repo":"o/x","host":"gitea"}]}`, `{"repos":[{"repo":"o/x","host":"gitlab"}]}`, `{"repo":[]}`} {
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

func TestHostTokenFallback(t *testing.T) {
	gh := filepath.Join(t.TempDir(), "gh")
	if err := os.WriteFile(gh, []byte("#!/bin/sh\n[ \"$*\" = \"auth token\" ] && echo gh-token\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	old := ghBin
	ghBin = gh
	t.Cleanup(func() { ghBin = old })
	t.Setenv("BRUH_TEST_TOKEN", "")
	if got := hostToken(repoConfig{Host: "github", TokenEnv: "BRUH_TEST_TOKEN"}); got != "gh-token" {
		t.Fatalf("github token = %q", got)
	}
	if got := hostToken(repoConfig{Host: "gitea", TokenEnv: "BRUH_TEST_TOKEN"}); got != "" {
		t.Fatalf("gitea token = %q", got)
	}
	t.Setenv("BRUH_TEST_TOKEN", "env-token")
	if got := hostToken(repoConfig{Host: "github", TokenEnv: "BRUH_TEST_TOKEN"}); got != "env-token" {
		t.Fatalf("env token = %q", got)
	}
}
