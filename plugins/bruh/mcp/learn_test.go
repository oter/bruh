package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// stableProject builds the same project index each time; reversed reverses each input list.
func stableProject(reversed bool) *projectFile {
	host := "gitlab.com"
	p := &projectFile{
		Key:     "shop",
		Purpose: &sourced{Value: "shop & <auth> > stable", Source: "owner"},
		Main:    "shop",
		Repos: []indexRepo{
			{
				Path: "shop",
				Remotes: []remote{
					{Name: "origin", URL: "git@gitlab.com:group-a/shop.git"},
					{Name: "backup", URL: "git@gitlab.com:group-a/shop-backup.git"},
				},
				Remote:        "origin",
				Host:          hostValue{Value: &host, Source: "git"},
				Kind:          "gitlab",
				APIURL:        "https://gitlab.com/api/v4",
				HostPath:      "group-a/shop",
				DefaultBranch: "main",
				State:         "present",
			},
			{
				Path:          "group-a/shop-app",
				Remotes:       []remote{{Name: "origin", URL: "git@gitlab.com-stable:group-a/shop-app.git"}},
				Remote:        "origin",
				Host:          hostValue{Source: "git"},
				Kind:          "unknown",
				HostPath:      "group-a/shop-app",
				DefaultBranch: "unknown",
				State:         "missing",
			},
		},
		Links: []link{
			{Project: "auth", Repo: "shop", File: "go.mod", Line: 12, Source: "agent"},
			{Project: "auth", Repo: "shop", File: "go.mod", Line: 5, Source: "agent"},
			{Project: "auth", Repo: "group-a/shop-app", File: "go.mod", Line: 3, Source: "owner"},
		},
		Docs: []doc{
			{Repo: "shop", Path: "README.md", Source: "agent"},
			{Repo: "shop", Path: "docs/stable.md", Source: "owner", Gone: true},
			{Repo: "group-a/shop-app", Path: "README.md", Source: "agent"},
		},
	}
	if reversed {
		slices.Reverse(p.Repos)
		slices.Reverse(p.Repos[0].Remotes)
		slices.Reverse(p.Repos[1].Remotes)
		slices.Reverse(p.Links)
		slices.Reverse(p.Docs)
	}
	return p
}

// stableTree builds the same tree index each time; reversed reverses each input list.
func stableTree(reversed bool) *treeFile {
	t := &treeFile{
		Root:        "/stable/workspace",
		Depth:       4,
		Exclude:     []string{"archive", "stable"},
		HostAliases: map[string]string{"gitlab.com-stable": "gitlab.com", "github.com-stable": "github.com"},
		HostKinds:   map[string]string{"git.stable.example": "gitea", "gitlab.stable.example": "gitlab"},
		Projects: []treeProject{
			{Key: "auth", Group: ""},
			{Key: "shop", Group: "group-a"},
			{Key: "shop-app", Group: "group-a"},
		},
	}
	if reversed {
		slices.Reverse(t.Exclude)
		slices.Reverse(t.Projects)
	}
	return t
}

// mustEncodeLearn returns encodeLearn(v) and fails the test on an error.
func mustEncodeLearn(t *testing.T, v any) string {
	t.Helper()
	b, err := encodeLearn(v)
	if err != nil {
		t.Fatalf("encodeLearn(%T) error: %v", v, err)
	}
	return string(b)
}

// checkKeyOrder reports an error when the keys do not appear in out in this order.
func checkKeyOrder(t *testing.T, out string, keys ...string) {
	t.Helper()
	prev := -1
	for _, k := range keys {
		i := strings.Index(out, `"`+k+`":`)
		if i < 0 {
			t.Errorf("encodeLearn output has no key %q:\n%s", k, out)
			return
		}
		if i < prev {
			t.Errorf("encodeLearn output has key %q out of the order %q:\n%s", k, keys, out)
			return
		}
		prev = i
	}
}

func TestLearnJSONIsStable(t *testing.T) {
	project := mustEncodeLearn(t, stableProject(false))
	if got := mustEncodeLearn(t, stableProject(true)); got != project {
		t.Errorf("encodeLearn(project) depends on the list order:\n%s\nwant:\n%s", got, project)
	}
	tree := mustEncodeLearn(t, stableTree(false))
	if got := mustEncodeLearn(t, stableTree(true)); got != tree {
		t.Errorf("encodeLearn(tree) depends on the list order:\n%s\nwant:\n%s", got, tree)
	}

	for name, out := range map[string]string{"project": project, "tree": tree} {
		if !strings.HasSuffix(out, "}\n") || strings.HasSuffix(out, "\n\n") {
			t.Errorf("encodeLearn(%s) = %q, want an end of exactly one \"\\n\"", name, out)
		}
		for _, k := range []string{"at", "time", "activity", "stack", "gates", "relearn", "gate_hash"} {
			if strings.Contains(out, `"`+k+`":`) {
				t.Errorf("encodeLearn(%s) has the key %q:\n%s", name, k, out)
			}
		}
	}

	checkKeyOrder(t, project, "key", "purpose", "main", "repos", "links", "docs")
	repos := project[strings.Index(project, `"repos":`):]
	checkKeyOrder(t, repos, "path", "remotes", "remote", "host", "kind", "api_url", "host_path", "default_branch", "state")
	checkKeyOrder(t, tree, "root", "depth", "exclude", "host_aliases", "host_kinds", "projects")

	if !strings.Contains(project, `"value": "shop & <auth> > stable"`) {
		t.Errorf("encodeLearn(project) escapes the purpose:\n%s", project)
	}
	for _, esc := range []string{`\u0026`, `\u003c`, `\u003e`} {
		if strings.Contains(project, esc) {
			t.Errorf("encodeLearn(project) has %s:\n%s", esc, project)
		}
	}

	// A nil purpose, a nil host value, and nil lists and maps.
	empty := mustEncodeLearn(t, &projectFile{
		Key:   "shop",
		Main:  "shop",
		Repos: []indexRepo{{Path: "shop", Remote: "origin", Host: hostValue{Source: "git"}, Kind: "unknown", DefaultBranch: "unknown", State: "present"}},
	})
	for _, want := range []string{`"purpose": null`, `"host": {`, `"remotes": []`, `"links": []`, `"docs": []`} {
		if !strings.Contains(empty, want) {
			t.Errorf("encodeLearn(nil fields) has no %s:\n%s", want, empty)
		}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(empty)); err != nil {
		t.Fatalf("json.Compact(encodeLearn(nil fields)) error: %v", err)
	}
	if want := `"host":{"value":null,"source":"git"}`; !strings.Contains(compact.String(), want) {
		t.Errorf("encodeLearn(nil host value) = %s, want %s in it", compact.String(), want)
	}
	if got := mustEncodeLearn(t, &projectFile{
		Key:   "shop",
		Main:  "shop",
		Repos: []indexRepo{{Path: "shop", Remotes: []remote{}, Remote: "origin", Host: hostValue{Source: "git"}, Kind: "unknown", DefaultBranch: "unknown", State: "present"}},
		Links: []link{},
		Docs:  []doc{},
	}); got != empty {
		t.Errorf("encodeLearn(empty project lists) = %s, want the bytes of nil lists %s", got, empty)
	}

	emptyTree := mustEncodeLearn(t, &treeFile{Root: "/stable", Depth: 4})
	for _, want := range []string{`"exclude": []`, `"host_aliases": {}`, `"host_kinds": {}`, `"projects": []`} {
		if !strings.Contains(emptyTree, want) {
			t.Errorf("encodeLearn(nil tree fields) has no %s:\n%s", want, emptyTree)
		}
	}
	if got := mustEncodeLearn(t, &treeFile{
		Root: "/stable", Depth: 4, Exclude: []string{}, HostAliases: map[string]string{}, HostKinds: map[string]string{}, Projects: []treeProject{},
	}); got != emptyTree {
		t.Errorf("encodeLearn(empty tree lists) = %s, want the bytes of nil lists %s", got, emptyTree)
	}

	if b, err := encodeLearn("x"); err == nil {
		t.Errorf(`encodeLearn("x") = %q, want an error`, b)
	}
}

// The two examples of docs/spec.md section 8.5 "Schema".
const (
	specTreeExample = `{
  "root": "/home/me/workspace",
  "depth": 4,
  "exclude": ["archive"],
  "host_aliases": {"gitlab.com-work": "gitlab.com"},
  "host_kinds": {"git.example.org": "gitea"},
  "projects": [{"key": "shop", "group": ""}]
}`
	specProjectExample = `{
  "key": "shop",
  "purpose": {"value": "Online shop: web client and Go API", "source": "agent"},
  "main": "shop",
  "repos": [
    {
      "path": "shop",
      "remotes": [{"name": "origin", "url": "git@gitlab.com:team/shop.git"}],
      "remote": "origin",
      "host": {"value": "gitlab.com", "source": "git"},
      "kind": "gitlab",
      "api_url": "https://gitlab.com/api/v4",
      "host_path": "team/shop",
      "default_branch": "main",
      "state": "present"
    }
  ],
  "links": [{"project": "auth", "repo": "shop", "file": "go.mod", "line": 5, "source": "agent"}],
  "docs": [{"repo": "shop", "path": "README.md", "source": "agent", "gone": false}]
}`
)

// decodeStrict decodes data into v and fails the test on an unknown field.
func decodeStrict(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		t.Fatalf("decode %T error: %v\n%s", v, err, data)
	}
}

// roundTrip decodes example into a T, encodes it with encodeLearn, and checks that
// the decoded result has the same value.
func roundTrip[T any](t *testing.T, example string) {
	t.Helper()
	var want T
	decodeStrict(t, []byte(example), &want)
	b, err := encodeLearn(&want)
	if err != nil {
		t.Fatalf("encodeLearn(%T) error: %v", &want, err)
	}
	var got T
	decodeStrict(t, b, &got)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("encodeLearn(%T) round trip = %+v, want %+v", &want, got, want)
	}
}

func TestLearnJSONMatchesSpecExample(t *testing.T) {
	t.Run("tree", func(t *testing.T) { roundTrip[treeFile](t, specTreeExample) })
	t.Run("project", func(t *testing.T) { roundTrip[projectFile](t, specProjectExample) })
}

// checkMode reports an error when the permission bits of path are not want.
func checkMode(t *testing.T, path string, want fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) error: %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Errorf("mode of %q = %v, want %v", path, got, want)
	}
}

// checkFileBytes reports an error when the file at path does not hold want.
func checkFileBytes(t *testing.T, path, want string) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile(%q) error: %v", path, err)
	}
	if string(got) != want {
		t.Errorf("file %q = %s, want %s", path, got, want)
	}
}

func TestWriteLearnFileOnlyOnChange(t *testing.T) {
	root := t.TempDir()
	learnDir := filepath.Join(root, "learn")
	projectsDir := filepath.Join(learnDir, "projects")
	path := filepath.Join(projectsDir, strings.ToLower(t.Name())+".json")

	// First call: the folders and the file do not exist yet.
	changed, err := writeLearnFile(path, stableTree(false))
	if err != nil || !changed {
		t.Fatalf("writeLearnFile(new path) = %v, %v, want true, nil", changed, err)
	}
	first := mustEncodeLearn(t, stableTree(false))
	checkFileBytes(t, path, first)
	checkMode(t, path, 0o644)
	checkMode(t, learnDir, 0o755)
	checkMode(t, projectsDir, 0o755)

	// Second call with the same value: no write, so the modification time stays.
	old := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("os.Chtimes(%q) error: %v", path, err)
	}
	changed, err = writeLearnFile(path, stableTree(true))
	if err != nil || changed {
		t.Errorf("writeLearnFile(same value) = %v, %v, want false, nil", changed, err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("os.Stat(%q) error: %v", path, err)
	}
	if !info.ModTime().Equal(old) {
		t.Errorf("modification time after writeLearnFile(same value) = %v, want %v", info.ModTime(), old)
	}
	checkFileBytes(t, path, first)

	// A changed value: a write of the new bytes.
	next := stableTree(false)
	next.Depth = 6
	changed, err = writeLearnFile(path, next)
	if err != nil || !changed {
		t.Errorf("writeLearnFile(changed value) = %v, %v, want true, nil", changed, err)
	}
	second := mustEncodeLearn(t, next)
	checkFileBytes(t, path, second)
	checkMode(t, path, 0o644)

	// An encode error: the error, and no write to an existing or a new file.
	if changed, err := writeLearnFile(path, "x"); err == nil || changed {
		t.Errorf(`writeLearnFile(existing path, "x") = %v, %v, want false and an error`, changed, err)
	}
	checkFileBytes(t, path, second)
	fresh := filepath.Join(projectsDir, "fresh.json")
	if changed, err := writeLearnFile(fresh, "x"); err == nil || changed {
		t.Errorf(`writeLearnFile(new path, "x") = %v, %v, want false and an error`, changed, err)
	}
	if _, err := os.Stat(fresh); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("os.Stat(%q) after an encode error = %v, want fs.ErrNotExist", fresh, err)
	}
}

func TestLedgerPath(t *testing.T) {
	const wantErr = "no ledger path in <data>/init/config.json; run /bruh:init"
	ledger := t.TempDir()
	good, err := json.Marshal(initConfig{LedgerPath: ledger})
	if err != nil {
		t.Fatalf("json.Marshal(initConfig) error: %v", err)
	}
	tests := []struct {
		name    string
		config  []byte // nil: no file <data>/init/config.json
		want    string
		wantErr bool
	}{
		{name: "no file", wantErr: true},
		{name: "empty object", config: []byte("{}"), wantErr: true},
		{name: "empty ledger_path", config: []byte(`{"ledger_path": ""}`), wantErr: true},
		{name: "not JSON", config: []byte("not json"), wantErr: true},
		{name: "ledger_path set", config: good, want: ledger},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testEnv(t, "bigm")
			if tt.config != nil {
				dir := filepath.Join(env.DataDir, "init")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatalf("os.MkdirAll(%q) error: %v", dir, err)
				}
				if err := os.WriteFile(filepath.Join(dir, "config.json"), tt.config, 0o600); err != nil {
					t.Fatalf("os.WriteFile(config.json) error: %v", err)
				}
			}
			got, err := ledgerPath(env)
			if tt.wantErr {
				if err == nil || err.Error() != wantErr {
					t.Errorf("ledgerPath() = %q, %v, want error %q", got, err, wantErr)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Errorf("ledgerPath() = %q, %v, want %q, nil", got, err, tt.want)
			}
		})
	}
}

func TestCheckFills(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"shop/docs", "shop-app"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatalf("os.MkdirAll(%q) error: %v", dir, err)
		}
	}
	for _, file := range []string{"shop/README.md", "shop/secret.md", "shop-app/README.md"} {
		if err := os.WriteFile(filepath.Join(root, file), []byte("doc\n"), 0o644); err != nil {
			t.Fatalf("os.WriteFile(%q) error: %v", file, err)
		}
	}
	secret := filepath.Join(root, "shop", "secret.md")
	if err := os.Chmod(secret, 0o000); err != nil {
		t.Fatalf("os.Chmod(%q) error: %v", secret, err)
	}

	keys := []string{"shop", "auth"}
	purpose := fill{Field: "purpose", Value: "Online shop", Source: "agent"}
	link := fill{Field: "link", Value: "auth", Source: "agent", Repo: "shop", File: "go.mod", Line: 5}
	docFill := fill{Field: "doc", Value: "README.md", Source: "owner", Repo: "shop"}
	host := fill{Field: "host", Value: "gitlab.com", Source: "owner", Repo: "shop-app"}
	// with returns f with the changes of edit applied.
	with := func(f fill, edit func(*fill)) fill {
		edit(&f)
		return f
	}

	tests := []struct {
		name     string
		fills    []fill
		want     string // a substring of the error; "" means no error
		skipRoot bool   // skip when the test runs as root, because root reads each file
	}{
		{name: "valid purpose of 120 characters", fills: []fill{with(purpose, func(f *fill) { f.Value = strings.Repeat("p", 120) })}},
		{name: "valid link", fills: []fill{link}},
		{name: "valid doc", fills: []fill{docFill}},
		{name: "valid host", fills: []fill{host}},
		{name: "doc fill whose file has mode 000 passes", fills: []fill{with(docFill, func(f *fill) { f.Value = "secret.md" })}, skipRoot: true},

		{name: "bad field", fills: []fill{with(docFill, func(f *fill) { f.Field = "stack" })}, want: "stack"},
		{name: "bad source", fills: []fill{with(purpose, func(f *fill) { f.Source = "git" })}, want: "Online shop"},
		{name: "missing repo", fills: []fill{with(docFill, func(f *fill) { f.Repo = "" })}, want: "README.md"},
		{name: "purpose with a repo", fills: []fill{with(purpose, func(f *fill) { f.Repo = "shop" })}, want: "Online shop"},
		{name: "repo outside the project", fills: []fill{with(host, func(f *fill) { f.Repo = "auth" })}, want: "gitlab.com"},
		{name: "link to an unknown key", fills: []fill{with(link, func(f *fill) { f.Value = "billing" })}, want: "billing"},
		{name: "link to the own key", fills: []fill{with(link, func(f *fill) { f.Value = "shop" })}, want: "link"},
		{name: "link file with ..", fills: []fill{with(link, func(f *fill) { f.File = "../auth/go.mod" })}, want: "auth"},
		{name: "link file that starts with /", fills: []fill{with(link, func(f *fill) { f.File = "/go.mod" })}, want: "auth"},
		{name: "link line 0", fills: []fill{with(link, func(f *fill) { f.Line = 0 })}, want: "auth"},
		{name: "doc path with ..", fills: []fill{with(docFill, func(f *fill) { f.Value = "../shop-app/README.md" })}, want: "../shop-app/README.md"},
		{name: "doc path that starts with /", fills: []fill{with(docFill, func(f *fill) { f.Value = "/README.md" })}, want: "/README.md"},
		{name: "doc file that does not exist", fills: []fill{with(docFill, func(f *fill) { f.Value = "MISSING.md" })}, want: "MISSING.md"},
		{name: "doc value that is a folder", fills: []fill{with(docFill, func(f *fill) { f.Value = "docs" })}, want: "docs"},
		{name: "bad host", fills: []fill{with(host, func(f *fill) { f.Value = "GitLab.com" })}, want: "GitLab.com"},
		{name: "purpose of 121 characters", fills: []fill{with(purpose, func(f *fill) { f.Value = strings.Repeat("p", 121) })}, want: strings.Repeat("p", 121)},
		{name: "value with a line feed", fills: []fill{with(purpose, func(f *fill) { f.Value = "line one\nline two" })}, want: "line one"},
		{name: "value with a carriage return", fills: []fill{with(purpose, func(f *fill) { f.Value = "line one\rline two" })}, want: "line one"},
		{name: "value with a pipe", fills: []fill{with(purpose, func(f *fill) { f.Value = "shop | auth" })}, want: "shop | auth"},
		{name: "two purpose fills", fills: []fill{purpose, with(purpose, func(f *fill) { f.Value = "Second purpose" })}, want: "purpose"},
		{name: "two host fills for one repository", fills: []fill{host, with(host, func(f *fill) { f.Value = "github.com" })}, want: "host"},
		{name: "a fill twice", fills: []fill{docFill, docFill}, want: "README.md"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipRoot && os.Geteuid() == 0 {
				t.Skip("root reads each file")
			}
			project := answerProject{Key: "shop", Repos: []string{"shop", "shop-app"}, Main: "shop", Fills: tt.fills}
			err := checkFills(root, project, keys)
			if tt.want == "" {
				if err != nil {
					t.Errorf("checkFills(%+v) = %v, want nil", tt.fills, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("checkFills(%+v) = nil, want an error with %q", tt.fills, tt.want)
			}
			for _, sub := range []string{project.Key, tt.want} {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("checkFills(%+v) = %q, want %q in it", tt.fills, err, sub)
				}
			}
		})
	}
}
