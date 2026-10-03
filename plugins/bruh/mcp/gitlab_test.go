package main

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

// glProject is the API path prefix of group/sub/repo, URL-encoded.
const glProject = "projects/group%2Fsub%2Frepo/"

// newGitLabHost returns the client of group/sub/repo on gitlab.example.com.
func newGitLabHost(t *testing.T) codeHost {
	t.Helper()
	h, err := newHost(repoConfig{Repo: "group/sub/repo", Host: "gitlab", APIURL: "https://gitlab.example.com/api/v4", Project: "repo", MergeMethod: "merge"})
	if err != nil {
		t.Fatalf("newHost: %v", err)
	}
	return h
}

// logLines returns the lines of a fake CLI log.
func logLines(t *testing.T, logFile string) []string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func TestGitLabCallsUseHostnameAndEncodedPath(t *testing.T) {
	branches := glProject + "repository/branches?per_page=100"
	log := fakeGlab(t, map[string]string{
		branches: `[{"name":"main","commit":{"id":"m1"}},{"name":"feat","commit":{"id":"f1"}}]`,
	})
	h := newGitLabHost(t)

	got, err := h.Branches(t.Context())
	if want := map[string]string{"main": "m1", "feat": "f1"}; err != nil || !maps.Equal(got, want) {
		t.Fatalf("Branches() = %v, %v; want %v", got, err, want)
	}
	lines := logLines(t, log)
	if len(lines) != 1 || !strings.Contains(lines[0], "--hostname gitlab.example.com") || !strings.Contains(lines[0], branches) {
		t.Errorf("glab log = %q; want --hostname gitlab.example.com and %s", lines, branches)
	}
	if got, want := h.LastCall(), "glab api --hostname gitlab.example.com "+branches; got != want {
		t.Errorf("LastCall() = %q, want %q", got, want)
	}
	if _, err := h.Pull(t.Context(), 1); err == nil || !strings.Contains(err.Error(), "404 Not Found") {
		t.Errorf("Pull(1) with no fixture: err = %v, want an error with 404 Not Found", err)
	}
}

func TestGitLabChecksStates(t *testing.T) {
	pipelines := func(sha string) string {
		return glProject + "pipelines?sha=" + sha + "&order_by=id&sort=desc&per_page=1"
	}
	fakeGlab(t, map[string]string{
		pipelines("s1"): `[{"id":11,"sha":"s1","status":"success"}]`,
		pipelines("s2"): `[{"id":12,"sha":"s2","status":"failed"}]`,
		pipelines("s3"): `[{"id":13,"sha":"s3","status":"running"}]`,
		pipelines("s4"): `[{"id":14,"sha":"s4","status":"canceled"}]`,
		pipelines("s5"): `[]`,
	})
	h := newGitLabHost(t)
	for _, tc := range []struct{ sha, status, want string }{
		{"s1", "success", "success"},
		{"s2", "failed", "failure"},
		{"s3", "running", "pending"},
		{"s4", "canceled", "pending"},
		{"s5", "no pipeline", "none"},
	} {
		if got, err := h.Checks(t.Context(), tc.sha); err != nil || got != tc.want {
			t.Errorf("Checks(%s) with %s = %q, %v; want %q", tc.sha, tc.status, got, err, tc.want)
		}
	}
}

func TestGitLabPullMapping(t *testing.T) {
	web := "https://gitlab.example.com/group/sub/repo/-/merge_requests/"
	fakeGlab(t, map[string]string{
		glProject + "merge_requests/5": `{"iid":5,"state":"opened","draft":true,"sha":"h5","merge_commit_sha":null,"squash_commit_sha":null,` +
			`"web_url":"` + web + `5","updated_at":"2026-10-01T10:00:00Z","detailed_merge_status":"not_approved"}`,
		glProject + "merge_requests/6": `{"iid":6,"state":"merged","draft":false,"sha":"h6","merge_commit_sha":null,"squash_commit_sha":"q6",` +
			`"web_url":"` + web + `6","updated_at":"2026-10-02T10:00:00Z","detailed_merge_status":"not_open"}`,
		glProject + "merge_requests?state=merged&order_by=updated_at&sort=desc&per_page=30": `[` +
			`{"iid":9,"state":"merged","sha":"h9","merge_commit_sha":"m9","squash_commit_sha":null,"web_url":"` + web + `9"},` +
			`{"iid":8,"state":"merged","sha":"h8","merge_commit_sha":null,"squash_commit_sha":"q8","web_url":"` + web + `8"}]`,
	})
	h := newGitLabHost(t)

	type pullView struct {
		State, SHA, MergeSHA, URL, UpdatedAt, DetailedMergeStatus string
		Merged, Draft                                             bool
	}
	view := func(p hostPull) pullView {
		return pullView{p.State, p.Head.SHA, p.MergeSHA, p.URL, p.UpdatedAt, p.DetailedMergeStatus, p.Merged, p.Draft}
	}
	for n, want := range map[int]pullView{
		5: {State: "open", SHA: "h5", URL: web + "5", UpdatedAt: "2026-10-01T10:00:00Z", DetailedMergeStatus: "not_approved", Draft: true},
		6: {State: "merged", SHA: "h6", MergeSHA: "q6", URL: web + "6", UpdatedAt: "2026-10-02T10:00:00Z", DetailedMergeStatus: "not_open", Merged: true},
	} {
		p, err := h.Pull(t.Context(), n)
		if got := view(p); err != nil || got != want {
			t.Errorf("Pull(%d) = %+v, %v; want %+v", n, got, err, want)
		}
	}

	type mergedView struct {
		Number   int
		Merged   bool
		MergeSHA string
	}
	ps, err := h.MergedPulls(t.Context())
	if err != nil {
		t.Fatalf("MergedPulls() error: %v", err)
	}
	var got []mergedView
	for _, p := range ps {
		got = append(got, mergedView{p.Number, p.Merged, p.MergeSHA})
	}
	if want := []mergedView{{9, true, "m9"}, {8, true, "q8"}}; !slices.Equal(got, want) {
		t.Errorf("MergedPulls() = %+v, want %+v", got, want)
	}
}

func TestGitLabMergeBody(t *testing.T) {
	merge := glProject + "merge_requests/7/merge"
	for _, tc := range []struct{ method, stdin string }{
		{"merge", `stdin: {"sha":"h7"}`},
		{"squash", `stdin: {"sha":"h7","squash":true}`},
	} {
		t.Run(tc.method, func(t *testing.T) {
			log := fakeGlab(t, map[string]string{merge: `{"iid":7,"state":"merged"}`})
			if err := newGitLabHost(t).Merge(t.Context(), 7, "h7", tc.method); err != nil {
				t.Fatalf("Merge(7, h7, %s) error: %v", tc.method, err)
			}
			lines := logLines(t, log)
			if len(lines) != 2 {
				t.Fatalf("glab log = %q; want the arguments and the stdin line", lines)
			}
			for _, part := range []string{"-X PUT", "--input -", "Content-Type: application/json", merge} {
				if !strings.Contains(lines[0], part) {
					t.Errorf("glab arguments = %q; want %q in them", lines[0], part)
				}
			}
			if lines[1] != tc.stdin {
				t.Errorf("glab stdin line = %q, want %q", lines[1], tc.stdin)
			}
		})
	}
}

func TestGitLabSkipsSystemNotes(t *testing.T) {
	web := "https://gitlab.example.com/group/sub/repo/-/merge_requests/"
	notes := func(iid string) string {
		return glProject + "merge_requests/" + iid + "/notes?sort=asc&order_by=updated_at&per_page=100"
	}
	body := t.Name() + ": please rename the flag"
	log := fakeGlab(t, map[string]string{
		glProject + "merge_requests?state=opened&order_by=updated_at&sort=desc&per_page=30": `[` +
			`{"iid":3,"state":"opened","updated_at":"2026-10-02T09:00:00Z","web_url":"` + web + `3"},` +
			`{"iid":2,"state":"opened","updated_at":"2026-09-30T09:00:00Z","web_url":"` + web + `2"}]`,
		notes("3"): `[` +
			`{"id":31,"body":"added 1 commit","system":true,"author":{"username":"bot"},"updated_at":"2026-10-02T08:00:00Z"},` +
			`{"id":32,"body":"` + body + `","system":false,"author":{"username":"alice"},"updated_at":"2026-10-02T09:00:00Z"}]`,
	})
	h := newGitLabHost(t)

	cs, err := h.IssueComments(t.Context(), "2026-10-01T00:00:00Z")
	if err != nil {
		t.Fatalf("IssueComments() error: %v", err)
	}
	type commentView struct {
		ID                          int64
		Body, UpdatedAt, Login, URL string
		Number                      int
	}
	var got []commentView
	for _, c := range cs {
		got = append(got, commentView{c.ID, c.Body, c.UpdatedAt, c.User.Login, c.URL, c.Number()})
	}
	want := []commentView{{32, body, "2026-10-02T09:00:00Z", "alice", web + "3#note_32", 3}}
	if !slices.Equal(got, want) {
		t.Errorf("IssueComments() = %+v, want %+v", got, want)
	}
	for _, line := range logLines(t, log) {
		if strings.Contains(line, "merge_requests/2/notes") {
			t.Errorf("glab log has %q; the notes of a merge request updated before since must not be read", line)
		}
	}
}
