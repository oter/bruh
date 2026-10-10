package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
)

// learnAddInput is the closed input of learn_add: one project.
type learnAddInput struct {
	Repos []string `json:"repos"`
	Main  string   `json:"main"`
	Key   string   `json:"key"`
}

// learnAddResult is the result of learn_add: the ledger-relative paths that it wrote, sorted.
type learnAddResult struct {
	Written []string `json:"written"`
}

// learnAdd adds one project to the index of the ledger: learn/projects/<key>.json with the git
// facts of its repositories (the code of init, with no CLI and no ssh call), its entry in
// learn/tree.json, and projects/<key>.md from the template when that file is missing. It never
// changes a project that is in the index. Only bigm calls it.
func learnAdd(env Env, in learnAddInput) (learnAddResult, error) {
	res := learnAddResult{Written: []string{}}
	me, err := env.Caller()
	if err != nil {
		return res, err
	}
	if me != "bigm" {
		return res, errors.New("only bigm calls learn_add")
	}
	ledger, err := ledgerPath(env)
	if err != nil {
		return res, err
	}
	if len(in.Repos) == 0 {
		return res, errors.New("repos must name 1 or more repositories")
	}
	if in.Main == "" && len(in.Repos) == 1 {
		in.Main = in.Repos[0]
	}
	p := answerProject{Key: cmp.Or(in.Key, projectKey(path.Base(in.Main))), Repos: in.Repos, Main: in.Main}
	// One add at a time: each add reads and writes learn/tree.json.
	err = env.WithLock("learn-add", func() error {
		var err error
		res.Written, err = addProject(env.PluginRoot, ledger, p)
		return err
	})
	return res, err
}

// addProject checks p against the index of ledger and writes its files. It writes nothing when
// a check fails.
func addProject(pluginRoot, ledger string, p answerProject) ([]string, error) {
	learnDir := filepath.Join(ledger, "learn")
	jsonPath := func(key string) string { return filepath.Join(learnDir, "projects", key+".json") }
	var tree treeFile
	err := readLearnFile(filepath.Join(learnDir, "tree.json"), &tree)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, errors.New("no learn/tree.json; the owner runs /bruh:init first")
	}
	if err != nil {
		return nil, err
	}
	if slices.ContainsFunc(tree.Projects, func(tp treeProject) bool { return tp.Key == p.Key }) {
		return nil, fmt.Errorf("project %q is already in learn/tree.json; learn_add does not change it", p.Key)
	}
	a := InitAnswers{LedgerPath: ledger, Root: tree.Root, Projects: []answerProject{p}}
	if err := a.checkProjects(); err != nil {
		return nil, err
	}
	for _, tp := range tree.Projects {
		if !projectRE.MatchString(tp.Key) {
			return nil, fmt.Errorf("learn/tree.json: invalid project key %q", tp.Key)
		}
		var old projectFile
		if err := readLearnFile(jsonPath(tp.Key), &old); err != nil {
			return nil, err
		}
		for _, r := range old.Repos {
			if slices.Contains(p.Repos, r.Path) {
				return nil, fmt.Errorf("repository %q is already in project %q", r.Path, tp.Key)
			}
		}
	}
	pf, err := projectJSON(tree.Root, p, tree.HostAliases, tree.HostKinds)
	if err != nil {
		return nil, fmt.Errorf("project %s: %w", p.Key, err)
	}
	tmpl, err := projectTemplateFile(pluginRoot)
	if err != nil {
		return nil, err
	}

	// learn/tree.json comes last: a failed add leaves no tree entry, and a retry writes again.
	var written []string
	md := filepath.Join(ledger, "projects", p.Key+".md")
	if _, err := os.Lstat(md); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(filepath.Dir(md), 0o755); err != nil {
			return nil, err
		}
		if err := atomicWrite(md, projectMD(tmpl, p.Key)); err != nil {
			return nil, err
		}
		if err := os.Chmod(md, 0o644); err != nil {
			return nil, err
		}
		written = append(written, "projects/"+p.Key+".md")
	} else if err != nil {
		return nil, err
	}
	if _, err := writeLearnFile(jsonPath(p.Key), &pf); err != nil {
		return nil, err
	}
	tree.Projects = append(tree.Projects, treeProject{Key: p.Key, Group: projectGroup(p.Main)})
	if _, err := writeLearnFile(filepath.Join(learnDir, "tree.json"), &tree); err != nil {
		return nil, err
	}
	written = append(written, "learn/projects/"+p.Key+".json", "learn/tree.json")
	slices.Sort(written)
	return written, nil
}

// learnAddTool is the MCP tool learn_add.
func learnAddTool() Tool {
	str := stringSchema()
	return Tool{
		Name:        "learn_add",
		Description: "Add one project to the index of the ledger (learn/): the git facts of its repositories, its entry in learn/tree.json, and projects/<key>.md when it is missing. Only bigm, when the owner asks to add a project. repos are paths relative to root of learn/tree.json; main defaults to the one repository; key defaults to the key of the name of main. Refuses a key or a repository that is in the index. purpose, links, and docs stay empty. No model, code host, or ssh call. Commit the written paths.",
		InputSchema: objectSchema(map[string]any{
			"repos": map[string]any{"type": "array", "items": str, "minItems": 1},
			"main":  str,
			"key":   str,
		}, "repos"),
		Handler: func(c *Call, raw json.RawMessage) (any, error) {
			if len(bytes.TrimSpace(raw)) == 0 {
				raw = json.RawMessage(`{}`)
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			dec.DisallowUnknownFields() // the input is closed
			var in learnAddInput
			if err := dec.Decode(&in); err != nil {
				return nil, fmt.Errorf("learn_add input: %w", err)
			}
			return learnAdd(c.Env, in)
		},
	}
}
