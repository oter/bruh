package main

import (
	"cmp"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// refreshTime is the modification time of each file of a refresh fixture, so a write shows.
var refreshTime = time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)

// refreshFixture is a root with the repositories alpha and beta (each with a README.md), a
// ledger with learn/tree.json and the project file of key, and a data folder of bigm with
// init/config.json. The stored index is current: a refresh changes nothing.
type refreshFixture struct {
	env    Env
	root   string
	ledger string
	key    string
}

// refreshURL is the URL of origin of the fixture repository rel.
func refreshURL(rel string) string { return "https://gitlab.com/team/" + rel + ".git" }

// refreshRepo is the stored entry of the fixture repository rel, as its git facts give it.
func refreshRepo(rel string) indexRepo {
	host := "gitlab.com"
	return indexRepo{
		Path: rel, Remotes: []remote{{"origin", refreshURL(rel)}}, Remote: "origin",
		Host: hostValue{Value: &host, Source: "git"}, Kind: "gitlab", APIURL: "https://gitlab.com/api/v4",
		HostPath: "team/" + rel, DefaultBranch: "main", State: "present",
	}
}

// writeRefreshFile writes content to path, makes its folders, and sets its modification time
// to refreshTime.
func writeRefreshFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, refreshTime, refreshTime); err != nil {
		t.Fatal(err)
	}
}

func newRefreshFixture(t *testing.T) *refreshFixture {
	t.Helper()
	f := &refreshFixture{env: testEnv(t, "bigm"), root: t.TempDir(), ledger: t.TempDir(), key: projectKey(t.Name())}
	for _, rel := range []string{"alpha", "beta"} {
		f.writeRepo(t, rel, refreshURL(rel))
	}
	f.writeTree(t, nil, nil)
	f.writeProject(t, &projectFile{
		Key:     f.key,
		Purpose: &sourced{Value: "A test project.", Source: "owner"},
		Main:    "alpha",
		Repos:   []indexRepo{refreshRepo("alpha"), refreshRepo("beta")},
		// The refresh does not check a link: go.mod does not exist.
		Links: []link{{Project: "other", Repo: "alpha", File: "go.mod", Line: 3, Source: "agent"}},
		Docs: []doc{
			{Repo: "alpha", Path: "README.md", Source: "agent"},
			{Repo: "beta", Path: "README.md", Source: "owner"},
		},
	})
	cfg, err := json.Marshal(initConfig{LedgerPath: f.ledger})
	if err != nil {
		t.Fatal(err)
	}
	writeRefreshFile(t, filepath.Join(f.env.DataDir, "init", "config.json"), string(cfg))
	return f
}

// writeRepo makes (or remakes) the repository rel with url as origin, its HEAD file of origin
// with the branch main, and its README.md.
func (f *refreshFixture) writeRepo(t *testing.T, rel, url string) {
	t.Helper()
	writeRepo(t, f.root, rel, originConfig(url), "origin", "ref: refs/remotes/origin/main\n")
	writeRefreshFile(t, filepath.Join(f.root, rel, "README.md"), "# "+rel+"\n")
}

// writeTree writes learn/tree.json with the project of the fixture and host_aliases and
// host_kinds.
func (f *refreshFixture) writeTree(t *testing.T, aliases, kinds map[string]string) {
	t.Helper()
	tree := &treeFile{Root: f.root, Depth: 2, HostAliases: aliases, HostKinds: kinds, Projects: []treeProject{{Key: f.key}}}
	writeRefreshFile(t, filepath.Join(f.ledger, "learn", "tree.json"), mustEncodeLearn(t, tree))
}

// rel is the ledger-relative path of the project file.
func (f *refreshFixture) rel() string { return "learn/projects/" + f.key + ".json" }

func (f *refreshFixture) writeProject(t *testing.T, p *projectFile) {
	t.Helper()
	writeRefreshFile(t, filepath.Join(f.ledger, filepath.FromSlash(f.rel())), mustEncodeLearn(t, p))
}

func (f *refreshFixture) readProject(t *testing.T) projectFile {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.ledger, filepath.FromSlash(f.rel())))
	if err != nil {
		t.Fatal(err)
	}
	var p projectFile
	decodeStrict(t, data, &p)
	return p
}

// checkProject reports an error when the project file does not hold want.
func (f *refreshFixture) checkProject(t *testing.T, want projectFile) {
	t.Helper()
	if got := f.readProject(t); !reflect.DeepEqual(got, want) {
		t.Errorf("project file = %s, want %s", jsonText(t, got), jsonText(t, want))
	}
}

// snapshot returns the content and the modification time of each file of the ledger.
func (f *refreshFixture) snapshot(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(f.ledger, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files[path] = info.ModTime().String() + "\n" + string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// checkSnapshot reports an error when the ledger differs from before.
func (f *refreshFixture) checkSnapshot(t *testing.T, before map[string]string) {
	t.Helper()
	if after := f.snapshot(t); !maps.Equal(after, before) {
		t.Errorf("ledger changed:\nbefore %q\nafter  %q", before, after)
	}
}

// refresh calls learn_refresh as bigm, checks that the result has exactly the four keys, each
// a list (never null), and returns it.
func (f *refreshFixture) refresh(t *testing.T) refreshResult {
	t.Helper()
	out, err := call(t, as(f.env, "bigm"), "learn_refresh", map[string]any{})
	if err != nil {
		t.Fatalf("learn_refresh error: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("learn_refresh result = %#v, want an object", out)
	}
	if keys := slices.Sorted(maps.Keys(m)); !slices.Equal(keys, []string{"gone_docs", "long_files", "missing", "written"}) {
		t.Errorf("learn_refresh result keys = %q", keys)
	}
	for k, v := range m {
		if _, ok := v.([]any); !ok {
			t.Errorf("learn_refresh %s = %#v, want a list", k, v)
		}
	}
	var r refreshResult
	decodeStrict(t, []byte(jsonText(t, out)), &r)
	return r
}

// result returns a refreshResult with written and missing, no gone_docs, and no long_files.
func result(written, missing []string) refreshResult {
	return refreshResult{Written: written, Missing: missing, GoneDocs: []goneDoc{}, LongFiles: []string{}}
}

// noChange is the result of a refresh that changes nothing.
var noChange = result([]string{}, []string{})

func checkResult(t *testing.T, got, want refreshResult) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("learn_refresh = %s, want %s", jsonText(t, got), jsonText(t, want))
	}
}

func TestLearnRefreshRefusesNonBigm(t *testing.T) {
	tests := []struct{ role, want string }{
		{"", "BRUH_ROLE_KEY is not set"},
		{"clanker-shop", "only bigm calls learn_refresh"},
		{"clerk-shop-t1", "only bigm calls learn_refresh"},
	}
	for _, tt := range tests {
		t.Run(cmp.Or(tt.role, "no role key"), func(t *testing.T) {
			f := newRefreshFixture(t)
			// A refresh by bigm would write: a doc is gone and a repository is missing.
			if err := os.Remove(filepath.Join(f.root, "alpha", "README.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.RemoveAll(filepath.Join(f.root, "beta")); err != nil {
				t.Fatal(err)
			}
			before := f.snapshot(t)
			out, err := call(t, as(f.env, tt.role), "learn_refresh", map[string]any{})
			if err == nil || err.Error() != tt.want {
				t.Errorf("learn_refresh as %q = %v, %v, want error %q", tt.role, out, err, tt.want)
			}
			f.checkSnapshot(t, before)
		})
	}
}

func TestLearnRefreshNoChangeNoWrite(t *testing.T) {
	t.Run("current", func(t *testing.T) {
		f := newRefreshFixture(t)
		want := f.readProject(t)
		before := f.snapshot(t)
		for range 2 {
			checkResult(t, f.refresh(t), noChange)
			f.checkSnapshot(t, before)
		}
		f.checkProject(t, want)
	})
	t.Run("stale", func(t *testing.T) {
		f := newRefreshFixture(t)
		want := f.readProject(t)
		stale := f.readProject(t)
		stale.Repos[0].DefaultBranch = "dev"
		stale.Repos[1].Remotes = []remote{{"origin", "https://gitlab.com/team/old.git"}}
		stale.Repos[1].HostPath = "team/old"
		f.writeProject(t, &stale)

		checkResult(t, f.refresh(t), result([]string{f.rel()}, []string{}))
		f.checkProject(t, want)
		before := f.snapshot(t)
		checkResult(t, f.refresh(t), noChange)
		f.checkSnapshot(t, before)
	})
}

func TestLearnRefreshKeepsDefaultBranchWithoutFile(t *testing.T) {
	f := newRefreshFixture(t)
	if err := os.Remove(filepath.Join(f.root, "alpha", ".git", "refs", "remotes", "origin", "HEAD")); err != nil {
		t.Fatal(err)
	}
	want := f.readProject(t)
	before := f.snapshot(t)
	checkResult(t, f.refresh(t), noChange)
	f.checkSnapshot(t, before)
	f.checkProject(t, want)
	if got := f.readProject(t).Repos[0].DefaultBranch; got != "main" {
		t.Errorf("default_branch of alpha = %q, want %q", got, "main")
	}
}

// agentHost sets the host of p.Repos[0] to git.example.org, a Gitea host, with source.
func agentHost(p *projectFile, source string) {
	host := "git.example.org"
	r := &p.Repos[0]
	r.Host = hostValue{Value: &host, Source: source}
	r.Kind, r.APIURL = "gitea", "https://git.example.org/api/v1"
}

func TestLearnRefreshKeepsAgentHostWhileURLSame(t *testing.T) {
	const aliasURL = "git@code-work:team/alpha.git"
	for _, source := range []string{"agent", "owner"} {
		t.Run(source, func(t *testing.T) {
			f := newRefreshFixture(t)
			// The git facts give another host: host_aliases names the alias as gitlab.com.
			f.writeTree(t, map[string]string{"code-work": "gitlab.com"}, nil)
			f.writeRepo(t, "alpha", aliasURL)
			p := f.readProject(t)
			p.Repos[0].Remotes = []remote{{"origin", aliasURL}}
			agentHost(&p, source)
			f.writeProject(t, &p)

			before := f.snapshot(t)
			checkResult(t, f.refresh(t), noChange)
			f.checkSnapshot(t, before)
			f.checkProject(t, p)
		})
	}
}

func TestLearnRefreshRecomputesHostOnURLChange(t *testing.T) {
	null := hostValue{Value: nil, Source: "git"}
	git := func(host string) hostValue { return hostValue{Value: &host, Source: "git"} }
	tests := []struct {
		name    string
		source  string // the source of a stored host of git.example.org; "" keeps the git host
		url     string // the new URL of origin of alpha
		aliases map[string]string
		kinds   map[string]string
		want    indexRepo // Path, Remote, DefaultBranch, and State are the ones of alpha
	}{
		{name: "agent host, new URL", source: "agent", url: "https://gitlab.com/team/alpha-next.git",
			want: indexRepo{Host: git("gitlab.com"), Kind: "gitlab", APIURL: "https://gitlab.com/api/v4", HostPath: "team/alpha-next"}},
		{name: "owner host, new URL", source: "owner", url: "https://gitlab.com/team/alpha-next.git",
			want: indexRepo{Host: git("gitlab.com"), Kind: "gitlab", APIURL: "https://gitlab.com/api/v4", HostPath: "team/alpha-next"}},
		{name: "alias not in host_aliases", url: "git@code-work:team/alpha.git",
			want: indexRepo{Host: null, Kind: "unknown", HostPath: "team/alpha"}},
		{name: "agent host, alias not in host_aliases", source: "agent", url: "git@code-work:team/alpha.git",
			want: indexRepo{Host: null, Kind: "unknown", HostPath: "team/alpha"}},
		{name: "alias in host_aliases", url: "git@code-work:team/alpha.git",
			aliases: map[string]string{"code-work": "code.example.net"}, kinds: map[string]string{"code.example.net": "gitea"},
			want: indexRepo{Host: git("code.example.net"), Kind: "gitea", APIURL: "https://code.example.net/api/v1", HostPath: "team/alpha"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			f.writeTree(t, tt.aliases, tt.kinds)
			p := f.readProject(t)
			if tt.source != "" {
				agentHost(&p, tt.source)
				f.writeProject(t, &p)
			}
			f.writeRepo(t, "alpha", tt.url)

			checkResult(t, f.refresh(t), result([]string{f.rel()}, []string{}))
			want := tt.want
			want.Path, want.Remote, want.DefaultBranch, want.State = "alpha", "origin", "main", "present"
			want.Remotes = []remote{{"origin", tt.url}}
			p.Repos[0] = want
			f.checkProject(t, p)
		})
	}
}

func TestLearnRefreshSetsAndClearsGone(t *testing.T) {
	f := newRefreshFixture(t)
	readme := filepath.Join(f.root, "alpha", "README.md")
	p := f.readProject(t)
	entry := []goneDoc{{Project: f.key, Repo: "alpha", Path: "README.md"}}

	if err := os.Remove(readme); err != nil {
		t.Fatal(err)
	}
	checkResult(t, f.refresh(t), refreshResult{Written: []string{f.rel()}, Missing: []string{}, GoneDocs: entry, LongFiles: []string{}})
	p.Docs[0].Gone = true
	f.checkProject(t, p)

	before := f.snapshot(t)
	checkResult(t, f.refresh(t), noChange)
	f.checkSnapshot(t, before)
	f.checkProject(t, p)

	writeRefreshFile(t, readme, "# alpha\n")
	checkResult(t, f.refresh(t), refreshResult{Written: []string{f.rel()}, Missing: []string{}, GoneDocs: entry, LongFiles: []string{}})
	p.Docs[0].Gone = false
	f.checkProject(t, p)
}

func TestLearnRefreshSkipsDocsOfMissingRepo(t *testing.T) {
	tests := []struct {
		name   string
		remove []string // root-relative paths to remove
	}{
		{name: "folder removed", remove: []string{"beta"}},
		{name: "no .git folder", remove: []string{"beta/.git", "beta/README.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			p := f.readProject(t)
			for _, rel := range tt.remove {
				if err := os.RemoveAll(filepath.Join(f.root, filepath.FromSlash(rel))); err != nil {
					t.Fatal(err)
				}
			}
			checkResult(t, f.refresh(t), result([]string{f.rel()}, []string{"beta"}))
			p.Repos[1].State = "missing"
			f.checkProject(t, p) // the doc of beta keeps gone: false
		})
	}
}

func TestLearnRefreshRefusesWithoutLedgerPath(t *testing.T) {
	tests := []struct {
		name   string
		config string // "" for no init/config.json
	}{
		{name: "no init/config.json"},
		{name: "no ledger_path", config: "{}\n"},
		{name: "empty ledger_path", config: `{"ledger_path": ""}` + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			if err := os.Remove(filepath.Join(f.root, "alpha", "README.md")); err != nil {
				t.Fatal(err)
			}
			config := filepath.Join(f.env.DataDir, "init", "config.json")
			if tt.config == "" {
				if err := os.Remove(config); err != nil {
					t.Fatal(err)
				}
			} else {
				writeRefreshFile(t, config, tt.config)
			}
			_, wantErr := ledgerPath(f.env)
			if wantErr == nil {
				t.Fatal("ledgerPath() error = nil, want an error")
			}
			before := f.snapshot(t)
			out, err := call(t, as(f.env, "bigm"), "learn_refresh", map[string]any{})
			if err == nil || err.Error() != wantErr.Error() {
				t.Errorf("learn_refresh = %v, %v, want error %q", out, err, wantErr)
			}
			f.checkSnapshot(t, before)
		})
	}
}

func TestLearnRefreshMissingOnce(t *testing.T) {
	f := newRefreshFixture(t)
	p := f.readProject(t)
	if err := os.RemoveAll(filepath.Join(f.root, "beta")); err != nil {
		t.Fatal(err)
	}

	checkResult(t, f.refresh(t), result([]string{f.rel()}, []string{"beta"}))
	want := p // the other stored values stay
	want.Repos = slices.Clone(p.Repos)
	want.Repos[1].State = "missing"
	f.checkProject(t, want)

	before := f.snapshot(t)
	checkResult(t, f.refresh(t), noChange)
	f.checkSnapshot(t, before)
	f.checkProject(t, want)

	// The repository is back: present again, and not listed.
	f.writeRepo(t, "beta", refreshURL("beta"))
	checkResult(t, f.refresh(t), result([]string{f.rel()}, []string{}))
	f.checkProject(t, p)
}

func TestLearnRefreshRunsNoCLI(t *testing.T) {
	logs := map[string]string{
		"gh":   fakeCLI(t, &ghBin, "exit 1"),
		"glab": fakeCLI(t, &glabBin, "exit 1"),
		"ssh":  fakeCLI(t, &sshBin, "printf 'hostname gitlab.com\\n'"),
		"tea":  fakeCLI(t, &teaBin, "exit 1"),
	}
	f := newRefreshFixture(t)
	f.writeTree(t, map[string]string{"code-beta": "code.example.net"}, nil)
	// Two SSH alias remotes: one that host_aliases does not name, one that it names with a
	// host of no kind; and a gone doc.
	f.writeRepo(t, "alpha", "git@code-alpha:team/alpha.git")
	f.writeRepo(t, "beta", "ssh://git@code-beta/team/beta.git")
	if err := os.Remove(filepath.Join(f.root, "beta", "README.md")); err != nil {
		t.Fatal(err)
	}

	got := f.refresh(t)
	if !slices.Equal(got.Written, []string{f.rel()}) {
		t.Errorf("learn_refresh written = %q, want %q", got.Written, []string{f.rel()})
	}
	for name, log := range logs {
		if data, _ := os.ReadFile(log); len(data) != 0 {
			t.Errorf("%s log = %q, want empty", name, data)
		}
	}
}

func TestLearnRefreshLongFiles(t *testing.T) {
	lines := func(n int) string { return strings.Repeat("row\n", n) }
	capFive := map[string]string{
		"mode.md":       "mode: supervised\nledger_max_lines: 5\n",
		"ok.md":         lines(5),
		"projects/x.md": lines(6),
		"learn/x.md":    lines(6),
		".git/x.md":     lines(6),
	}
	tests := []struct {
		name      string
		files     map[string]string // ledger-relative path -> content
		noTree    bool              // remove learn/tree.json and change the repositories
		wantFiles []string
	}{
		{name: "cap from mode.md", files: capFive, wantFiles: []string{"projects/x.md"}},
		{name: "default cap", files: map[string]string{"ok.md": lines(300), "projects/x.md": lines(301)}, wantFiles: []string{"projects/x.md"}},
		{name: "no long file", files: map[string]string{"ok.md": lines(300)}, wantFiles: []string{}},
		// Entry B2 of decisions.md: with no learn/tree.json, no project file changes and no
		// repository or doc pointer is listed; long_files is still computed.
		{name: "no learn/tree.json", files: capFive, noTree: true, wantFiles: []string{"projects/x.md"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newRefreshFixture(t)
			for rel, content := range tt.files {
				writeRefreshFile(t, filepath.Join(f.ledger, filepath.FromSlash(rel)), content)
			}
			if tt.noTree {
				if err := os.Remove(filepath.Join(f.ledger, "learn", "tree.json")); err != nil {
					t.Fatal(err)
				}
				f.writeRepo(t, "alpha", "https://gitlab.com/team/alpha-next.git")
				if err := os.Remove(filepath.Join(f.root, "alpha", "README.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.RemoveAll(filepath.Join(f.root, "beta")); err != nil {
					t.Fatal(err)
				}
			}
			before := f.snapshot(t)
			want := noChange
			want.LongFiles = tt.wantFiles
			checkResult(t, f.refresh(t), want)
			f.checkSnapshot(t, before)
		})
	}
}
