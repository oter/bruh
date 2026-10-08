package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

// wantShopProject is learn/projects/shop.json of TestProjectJSONFromFills with all four fills.
const wantShopProject = `{
  "key": "shop",
  "purpose": {
    "value": "Online shop of the team",
    "source": "agent"
  },
  "main": "shop",
  "repos": [
    {
      "path": "shop",
      "remotes": [
        {
          "name": "origin",
          "url": "git@gitlab.com-work:team/shop.git"
        }
      ],
      "remote": "origin",
      "host": {
        "value": "gitlab.com",
        "source": "owner"
      },
      "kind": "gitlab",
      "api_url": "https://gitlab.com/api/v4",
      "host_path": "team/shop",
      "default_branch": "main",
      "state": "present"
    },
    {
      "path": "shop-app",
      "remotes": [
        {
          "name": "origin",
          "url": "https://gitlab.com/team/shop-app.git"
        }
      ],
      "remote": "origin",
      "host": {
        "value": "gitlab.com",
        "source": "git"
      },
      "kind": "gitlab",
      "api_url": "https://gitlab.com/api/v4",
      "host_path": "team/shop-app",
      "default_branch": "unknown",
      "state": "present"
    }
  ],
  "links": [
    {
      "project": "auth",
      "repo": "shop",
      "file": "go.mod",
      "line": 5,
      "source": "owner"
    }
  ],
  "docs": [
    {
      "repo": "shop",
      "path": "README.md",
      "source": "agent",
      "gone": false
    }
  ]
}
`

// noCLI points the four fakes at a log and returns a check that each log is empty: init_plan
// runs no CLI and no ssh (spec 398).
func noCLI(t *testing.T) func() {
	t.Helper()
	logs := map[string]string{}
	for name, bin := range map[string]*string{"ssh": &sshBin, "gh": &ghBin, "glab": &glabBin, "tea": &teaBin} {
		logs[name] = fakeCLI(t, bin, `printf 'hostname gitlab.com\n'`)
	}
	return func() {
		t.Helper()
		for name, log := range logs {
			if data, _ := os.ReadFile(log); len(data) != 0 {
				t.Errorf("%s log = %q, want empty", name, data)
			}
		}
	}
}

func TestProjectJSONFromFills(t *testing.T) {
	root := t.TempDir()
	writeRepo(t, root, "shop", originConfig("git@gitlab.com-work:team/shop.git"), "origin", "ref: refs/remotes/origin/main\n")
	writeRepo(t, root, "shop-app", originConfig("https://gitlab.com/team/shop-app.git"), "origin", "")
	fills := []fill{
		{Field: "purpose", Value: "Online shop of the team", Source: "agent"},
		{Field: "doc", Value: "README.md", Source: "agent", Repo: "shop"},
		{Field: "link", Value: "auth", Source: "owner", Repo: "shop", File: "go.mod", Line: 5},
	}
	hostFill := fill{Field: "host", Value: "gitlab.com", Source: "owner", Repo: "shop"}
	project := func(fills ...fill) answerProject {
		return answerProject{Key: "shop", Repos: []string{"shop", "shop-app"}, Main: "shop", Fills: fills}
	}

	t.Run("fills", func(t *testing.T) {
		checkLogs := noCLI(t)
		got, err := projectJSON(root, project(append(fills, hostFill)...), nil, nil)
		if err != nil {
			t.Fatalf("projectJSON: %v", err)
		}
		data, err := encodeLearn(&got)
		if err != nil {
			t.Fatalf("encodeLearn: %v", err)
		}
		if string(data) != wantShopProject {
			t.Errorf("projectJSON encoded =\n%s\nwant\n%s", data, wantShopProject)
		}
		checkLogs()
	})

	for _, tt := range []struct {
		name    string
		aliases map[string]string
		want    indexRepo // only Host, Kind, and APIURL are compared
	}{
		// The alias from host_aliases is checked end to end in TestInitPlanRunsNoCLI.
		{
			name: "alias without host",
			want: indexRepo{Host: hostValue{Value: nil, Source: "git"}, Kind: "unknown", APIURL: ""},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			checkLogs := noCLI(t)
			got, err := projectJSON(root, project(fills...), tt.aliases, nil)
			if err != nil {
				t.Fatalf("projectJSON: %v", err)
			}
			if got.Purpose == nil || got.Purpose.Source != "agent" {
				t.Errorf("projectJSON purpose = %+v, want the agent purpose", got.Purpose)
			}
			if len(got.Repos) != 2 || got.Repos[0].Path != "shop" {
				t.Fatalf("projectJSON repos = %s, want shop and shop-app", jsonText(t, got.Repos))
			}
			shop := got.Repos[0]
			gotHost := indexRepo{Host: shop.Host, Kind: shop.Kind, APIURL: shop.APIURL}
			if !reflect.DeepEqual(gotHost, tt.want) {
				t.Errorf("projectJSON shop host = %s, want %s", jsonText(t, gotHost), jsonText(t, tt.want))
			}
			if shop.HostPath != "team/shop" {
				t.Errorf("projectJSON shop host_path = %q, want %q", shop.HostPath, "team/shop")
			}
			checkLogs()
		})
	}

	t.Run("no purpose fill", func(t *testing.T) {
		got, err := projectJSON(root, project(fills[1:]...), nil, nil)
		if err != nil {
			t.Fatalf("projectJSON: %v", err)
		}
		if got.Purpose != nil {
			t.Errorf("projectJSON purpose = %+v, want nil", *got.Purpose)
		}
	})
}

// wantIndexShop is learn/projects/shop.json of TestInitPlanWritesIndex: the git facts of shop,
// then the fills of shopWithFills.
const wantIndexShop = `{
  "key": "shop",
  "purpose": {
    "value": "Online shop of the team",
    "source": "owner"
  },
  "main": "shop",
  "repos": [
    {
      "path": "shop",
      "remotes": [
        {
          "name": "origin",
          "url": "https://github.com/team/shop.git"
        }
      ],
      "remote": "origin",
      "host": {
        "value": "github.com",
        "source": "git"
      },
      "kind": "github",
      "api_url": "https://api.github.com",
      "host_path": "team/shop",
      "default_branch": "main",
      "state": "present"
    }
  ],
  "links": [
    {
      "project": "auth",
      "repo": "shop",
      "file": "go.mod",
      "line": 3,
      "source": "agent"
    }
  ],
  "docs": [
    {
      "repo": "shop",
      "path": "README.md",
      "source": "agent",
      "gone": false
    }
  ]
}
`

// storedShop is a stored learn/projects/shop.json that the git facts and no fill can give: an
// owner purpose, a doc pointer with gone true, and a default branch that the fixture does not have.
const storedShop = `{
  "key": "shop",
  "purpose": {
    "value": "Shop of the team, in the words of the owner",
    "source": "owner"
  },
  "main": "shop",
  "repos": [
    {
      "path": "shop",
      "remotes": [
        {
          "name": "origin",
          "url": "https://github.com/team/shop.git"
        }
      ],
      "remote": "origin",
      "host": {
        "value": "github.com",
        "source": "git"
      },
      "kind": "github",
      "api_url": "https://api.github.com",
      "host_path": "team/shop",
      "default_branch": "develop",
      "state": "present"
    },
    {
      "path": "shop-app",
      "remotes": [
        {
          "name": "origin",
          "url": "https://github.com/team/shop-app.git"
        }
      ],
      "remote": "origin",
      "host": {
        "value": "github.com",
        "source": "git"
      },
      "kind": "github",
      "api_url": "https://api.github.com",
      "host_path": "team/shop-app",
      "default_branch": "develop",
      "state": "present"
    }
  ],
  "links": [],
  "docs": [
    {
      "repo": "shop",
      "path": "docs/old.md",
      "source": "agent",
      "gone": true
    }
  ]
}
`

// learnRoot returns a root with the repositories shop (with a README.md), shop-app, and
// team/auth, each with the remote origin and refs/remotes/origin/HEAD.
func learnRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeRepo(t, root, "shop", originConfig("https://github.com/team/shop.git"), "origin", "ref: refs/remotes/origin/main\n")
	writeRepo(t, root, "shop-app", originConfig("https://github.com/team/shop-app.git"), "origin", "ref: refs/remotes/origin/main\n")
	writeRepo(t, root, "team/auth", originConfig("git@github.com:team/auth.git"), "origin", "ref: refs/remotes/origin/trunk\n")
	writeTestFile(t, filepath.Join(root, "shop", "README.md"), "# Shop\n")
	return root
}

// shopWithFills is the project shop of the answers with a purpose, a doc, and a link to auth.
func shopWithFills() map[string]any {
	p := project("shop", "shop", "shop")
	p["fills"] = []map[string]any{
		{"field": "purpose", "value": "Online shop of the team", "source": "owner"},
		{"field": "doc", "value": "README.md", "source": "agent", "repo": "shop"},
		{"field": "link", "value": "auth", "source": "agent", "repo": "shop", "file": "go.mod", "line": 3},
	}
	return p
}

// wantIndexTree is learn/tree.json of TestInitPlanWritesIndex for root.
func wantIndexTree(t *testing.T, root string) string {
	t.Helper()
	q, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	return `{
  "root": ` + string(q) + `,
  "depth": 4,
  "exclude": [
    "archive"
  ],
  "host_aliases": {},
  "host_kinds": {},
  "projects": [
    {
      "key": "auth",
      "group": "team"
    },
    {
      "key": "shop",
      "group": ""
    }
  ]
}
`
}

// projectTemplate is the project file of key that init makes from the ledger template.
func projectTemplate(t *testing.T, env Env, key string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(env.PluginRoot, "ledger-template", "projects", "_template.md"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "<project>", key)
}

// writeLearnTree stores learn/tree.json in ledger with root and the projects keys.
func writeLearnTree(t *testing.T, ledger, root string, keys ...string) {
	t.Helper()
	tree := treeFile{Root: root, Depth: 4, Exclude: []string{"archive"}}
	for _, k := range keys {
		tree.Projects = append(tree.Projects, treeProject{Key: k})
	}
	data, err := encodeLearn(&tree)
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(ledger, "learn", "tree.json"), string(data))
}

// writeProjectJSON stores learn/projects/<key>.json in ledger, with no repository.
func writeProjectJSON(t *testing.T, ledger, key string) string {
	t.Helper()
	data, err := encodeLearn(&projectFile{Key: key, Main: key})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(ledger, "learn", "projects", key+".json")
	writeTestFile(t, path, string(data))
	return path
}

// pathList returns the strings of a list of a tool result, for example files or kept.
func pathList(v any) []string {
	var out []string
	l, _ := v.([]any)
	for _, e := range l {
		s, _ := e.(string)
		out = append(out, s)
	}
	return out
}

func TestInitPlanWritesIndex(t *testing.T) {
	env, ledger := initEnv(t)
	root := learnRoot(t)
	a := answers(ledger, map[string]any{"root": root, "projects": []any{shopWithFills(), project("auth", "team/auth", "team/auth")}})
	p := plan(t, env, a)
	files := pathList(p["files"])
	for _, rel := range []string{"learn/tree.json", "learn/projects/shop.json", "learn/projects/auth.json", "projects/shop.md", "projects/auth.md"} {
		if path := filepath.Join(ledger, rel); !slices.Contains(files, path) {
			t.Errorf("init_plan with the projects shop and auth: files = %v, want %s", files, path)
		}
	}
	if kept, ok := p["kept"].([]any); !ok || len(kept) != 0 {
		t.Errorf("init_plan with no removed project: kept = %#v, want []", p["kept"])
	}
	apply(t, env, p)
	for rel, want := range map[string]string{
		"learn/projects/shop.json": wantIndexShop,
		"learn/tree.json":          wantIndexTree(t, root),
		"projects/shop.md":         projectTemplate(t, env, "shop"),
		"projects/auth.md":         projectTemplate(t, env, "auth"),
	} {
		got, err := os.ReadFile(filepath.Join(ledger, rel))
		if err != nil {
			t.Errorf("after init_apply: %v", err)
			continue
		}
		if string(got) != want {
			t.Errorf("after init_apply: %s =\n%s\nwant\n%s", rel, got, want)
		}
	}
}

func TestInitPlanSameBytesTwice(t *testing.T) {
	env, ledger := initEnv(t)
	root := learnRoot(t)
	a := answers(ledger, map[string]any{"root": root, "projects": []any{shopWithFills(), project("auth", "team/auth", "team/auth")}})
	first := plan(t, env, a)
	if path := filepath.Join(ledger, "learn", "projects", "shop.json"); !slices.Contains(pathList(first["files"]), path) {
		t.Fatalf("first init_plan: files = %v, want %s", first["files"], path)
	}
	apply(t, env, first)
	second := plan(t, env, a)
	if d := second["diff"].(string); d != "" {
		t.Errorf("init_plan after init_apply with the same answers: diff =\n%s\nwant empty", d)
	}
	if files := pathList(second["files"]); len(files) != 0 {
		t.Errorf("init_plan after init_apply with the same answers: files = %v, want none", files)
	}
}

func TestInitPlanKeepsUnchangedProjectJSON(t *testing.T) {
	root := learnRoot(t)
	emptyFills := project("shop", "shop", "shop", "shop-app")
	emptyFills["fills"] = []any{}
	for _, c := range []struct {
		name string
		shop map[string]any
		keep bool
	}{
		{"same key, repos, and main, no fills key", project("shop", "shop", "shop", "shop-app"), true},
		{"repos in another order", project("shop", "shop", "shop-app", "shop"), true},
		{"empty fills key", emptyFills, false},
		{"other repos", project("shop", "shop", "shop"), false},
		{"other main", project("shop", "shop-app", "shop", "shop-app"), false},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, ledger := initEnv(t)
			stored := filepath.Join(ledger, "learn", "projects", "shop.json")
			writeTestFile(t, stored, storedShop)
			writeLearnTree(t, ledger, root, "shop")
			a := answers(ledger, map[string]any{"root": root, "projects": []any{c.shop, project("auth", "team/auth", "team/auth")}})
			p := plan(t, env, a)
			files := pathList(p["files"])
			if auth := filepath.Join(ledger, "learn", "projects", "auth.json"); !slices.Contains(files, auth) {
				t.Fatalf("init_plan with the new project auth: files = %v, want %s", files, auth)
			}
			planned := slices.Contains(files, stored) || strings.Contains(p["diff"].(string), stored)
			if planned == c.keep {
				t.Errorf("init_plan with shop %v: %s in files or diff = %t, want %t", c.shop, stored, planned, !c.keep)
			}
			apply(t, env, p)
			got, err := os.ReadFile(stored)
			if err != nil {
				t.Fatal(err)
			}
			if c.keep && string(got) != storedShop {
				t.Errorf("after init_apply with shop %v: shop.json =\n%s\nwant the stored file\n%s", c.shop, got, storedShop)
			}
			if !c.keep && string(got) == storedShop {
				t.Errorf("after init_apply with shop %v: shop.json is the stored file, want it built again", c.shop)
			}
		})
	}
}

func TestInitPlanRemovesProject(t *testing.T) {
	env, ledger := initEnv(t)
	root := learnRoot(t)
	writeLearnTree(t, ledger, root, "shop", "auth", "docs")
	authJSON, docsJSON := writeProjectJSON(t, ledger, "auth"), writeProjectJSON(t, ledger, "docs")
	authMD, docsMD := filepath.Join(ledger, "projects", "auth.md"), filepath.Join(ledger, "projects", "docs.md")
	// One data row under the separator line of the first table, "In progress".
	sep := "|---|---|---|---|---|---|---|\n"
	row := "| Sam | Fix the login | merge request | open | 2026-10-04T09:00:00Z | https://git.example.com/team/auth/-/merge_requests/1 | mail |\n"
	busy := strings.Replace(projectTemplate(t, env, "auth"), sep, sep+row, 1)
	writeTestFile(t, authMD, busy)
	writeTestFile(t, docsMD, projectTemplate(t, env, "docs"))

	p := plan(t, env, answers(ledger, map[string]any{"root": root, "projects": []any{project("shop", "shop", "shop")}}))
	files, diff := pathList(p["files"]), p["diff"].(string)
	deleted := []string{authJSON, docsJSON, docsMD}
	for _, path := range deleted {
		if !slices.Contains(files, path) || !strings.Contains(diff, "--- "+path+"\n+++ /dev/null\n") {
			t.Errorf("init_plan without auth and docs: files = %v, diff has the deletion of %s = %t, want it in both", files, path, strings.Contains(diff, "--- "+path+"\n+++ /dev/null\n"))
		}
		// The diff of a deletion shows each old line as -<line>.
		old, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(strings.TrimSuffix(string(old), "\n"), "\n") {
			if !strings.Contains(diff, "\n-"+line+"\n") {
				t.Errorf("init_plan: the diff of the deletion of %s has no line %q", path, "-"+line)
			}
		}
	}
	if slices.Contains(files, authMD) || strings.Contains(diff, authMD) {
		t.Errorf("init_plan without auth: %s is in files or diff, want it kept (a data row)", authMD)
	}
	if kept := pathList(p["kept"]); !slices.Equal(kept, []string{authMD}) {
		t.Errorf("init_plan without auth and docs: kept = %v, want [%s]", kept, authMD)
	}
	applied := apply(t, env, p)
	for _, path := range deleted {
		if !slices.Contains(applied, path) {
			t.Errorf("init_apply: applied = %v, want the deleted %s", applied, path)
		}
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("after init_apply: Lstat(%s) error = %v, want the file deleted", path, err)
		}
	}
	if got, _ := os.ReadFile(authMD); string(got) != busy {
		t.Errorf("after init_apply: %s =\n%s\nwant it unchanged", authMD, got)
	}
}

func TestInitPlanWithoutProjectsKeepsLearn(t *testing.T) {
	env, ledger := initEnv(t)
	root := learnRoot(t)
	// A first init makes the ledger, so the next plan has no new template file under projects/.
	apply(t, env, plan(t, env, answers(ledger, nil)))
	writeLearnTree(t, ledger, root, "shop")
	shopJSON := writeProjectJSON(t, ledger, "shop")
	shopMD := filepath.Join(ledger, "projects", "shop.md")
	writeTestFile(t, shopMD, projectTemplate(t, env, "shop"))
	learnDir, projectsDir := filepath.Join(ledger, "learn")+string(filepath.Separator), filepath.Join(ledger, "projects")+string(filepath.Separator)

	t.Run("no projects key", func(t *testing.T) {
		p := plan(t, env, answers(ledger, map[string]any{"root": root}))
		for _, path := range pathList(p["files"]) {
			if strings.HasPrefix(path, learnDir) || strings.HasPrefix(path, projectsDir) {
				t.Errorf("init_plan with no projects key: files has %s, want no path under learn/ or projects/", path)
			}
		}
		if d := p["diff"].(string); strings.Contains(d, learnDir) || strings.Contains(d, projectsDir) {
			t.Errorf("init_plan with no projects key: diff =\n%s\nwant no path under learn/ or projects/", d)
		}
	})
	t.Run("empty projects", func(t *testing.T) {
		p := plan(t, env, answers(ledger, map[string]any{"root": root, "projects": []any{}}))
		files := pathList(p["files"])
		for _, path := range []string{shopJSON, shopMD} {
			if !slices.Contains(files, path) {
				t.Errorf("init_plan with projects []: files = %v, want the deletion of %s", files, path)
			}
		}
	})
}

// ledgerTree returns the content of each file under ledger, by its path relative to ledger.
func ledgerTree(t *testing.T, ledger string) map[string]string {
	t.Helper()
	tree := map[string]string{}
	err := filepath.WalkDir(ledger, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(ledger, p)
		tree[filepath.ToSlash(rel)] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return tree
}

func TestCLIInitMatchesInteractive(t *testing.T) {
	root := learnRoot(t)
	at := time.Date(2026, 10, 3, 9, 0, 0, 0, time.UTC)
	extra := func() map[string]any {
		return map[string]any{"root": root, "channels": []string{"telegram"}, "projects": []any{shopWithFills(), project("auth", "team/auth", "team/auth")}}
	}

	cliEnv, cliLedger := initEnv(t)
	cliEnv.Now = func() time.Time { return at }
	file := filepath.Join(t.TempDir(), "answers.json")
	b, err := json.Marshal(answers(cliLedger, extra()))
	if err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, file, string(b))
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"init", "--answers", file}, cliEnv, &out, &errOut); code != 0 {
		t.Fatalf("bruh init --answers %s: exit %d: %s", b, code, errOut.String())
	}

	toolEnv, toolLedger := initEnv(t)
	toolEnv.Now = func() time.Time { return at }
	apply(t, toolEnv, plan(t, toolEnv, answers(toolLedger, extra())))

	cliTree, toolTree := ledgerTree(t, cliLedger), ledgerTree(t, toolLedger)
	for _, rel := range []string{"learn/tree.json", "learn/projects/shop.json", "learn/projects/auth.json"} {
		if _, ok := toolTree[rel]; !ok {
			t.Errorf("init_plan and init_apply: the ledger has no %s", rel)
		}
	}
	rels := slices.Collect(maps.Keys(cliTree))
	for rel := range toolTree {
		if _, ok := cliTree[rel]; !ok {
			rels = append(rels, rel)
		}
	}
	slices.Sort(rels)
	for _, rel := range rels {
		c, inCLI := cliTree[rel]
		m, inTool := toolTree[rel]
		// The mail deny rules of the bigm start settings name the data folder, which differs.
		c = strings.ReplaceAll(c, cliEnv.DataDir, toolEnv.DataDir)
		if inCLI != inTool || c != m {
			t.Errorf("ledger file %s: the CLI wrote %t %q, init_apply wrote %t %q, want the same bytes", rel, inCLI, c, inTool, m)
		}
	}
}

func TestInitPlanRunsNoCLI(t *testing.T) {
	env, ledger := initEnv(t)
	root := t.TempDir()
	writeRepo(t, root, "shop", originConfig("git@gitlab.com-work:team/shop.git"), "origin", "ref: refs/remotes/origin/main\n")
	checkLogs := noCLI(t)
	a := answers(ledger, map[string]any{
		"root": root, "host_aliases": map[string]string{"gitlab.com-work": "gitlab.com"},
		"projects": []any{project("shop", "shop", "shop")},
	})
	apply(t, env, plan(t, env, a))
	checkLogs()
	var got projectFile
	data, err := os.ReadFile(filepath.Join(ledger, "learn", "projects", "shop.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Repos) != 1 || got.Repos[0].Host.Value == nil || *got.Repos[0].Host.Value != "gitlab.com" || got.Repos[0].Kind != "gitlab" || got.Repos[0].HostPath != "team/shop" || got.Repos[0].APIURL != "https://gitlab.com/api/v4" {
		t.Errorf("shop.json repos = %s, want the host gitlab.com of host_aliases, kind gitlab, api_url https://gitlab.com/api/v4, and host_path team/shop", jsonText(t, got.Repos))
	}
}
