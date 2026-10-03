package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// goneDoc is a doc pointer whose gone flag changed at this refresh.
type goneDoc struct {
	Project string `json:"project"`
	Repo    string `json:"repo"`
	Path    string `json:"path"`
}

// refreshResult is the result of learn_refresh.
type refreshResult struct {
	Written   []string  `json:"written"`
	Missing   []string  `json:"missing"`
	GoneDocs  []goneDoc `json:"gone_docs"`
	LongFiles []string  `json:"long_files"`
}

// learnRefresh reads the git facts of each repository of the index again and writes each
// project file whose bytes change. Only bigm calls it. It makes no model call, no code host
// call, and no ssh call.
func learnRefresh(env Env) (refreshResult, error) {
	res := refreshResult{Written: []string{}, Missing: []string{}, GoneDocs: []goneDoc{}, LongFiles: []string{}}
	me, err := env.Caller()
	if err != nil {
		return res, err
	}
	if me != "bigm" {
		return res, errors.New("only bigm calls learn_refresh")
	}
	ledger, err := ledgerPath(env)
	if err != nil {
		return res, err
	}
	var tree treeFile
	err = readLearnFile(filepath.Join(ledger, "learn", "tree.json"), &tree)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// Entry B2 of decisions.md: with no learn/tree.json (a remote machine), no project
		// file changes; long_files is still computed.
	case err != nil:
		return res, err
	default:
		if err := refreshProjects(ledger, &tree, &res); err != nil {
			return res, err
		}
	}
	if res.LongFiles, err = longFiles(ledger); err != nil {
		return res, err
	}
	return res, nil
}

// refreshProjects refreshes each project file of tree and fills written, missing, and
// gone_docs of res. It decodes each project file before it writes any.
func refreshProjects(ledger string, tree *treeFile, res *refreshResult) error {
	paths := make([]string, len(tree.Projects))
	projects := make([]projectFile, len(tree.Projects))
	for i, tp := range tree.Projects {
		if !projectRE.MatchString(tp.Key) {
			return fmt.Errorf("learn/tree.json: invalid project key %q", tp.Key)
		}
		paths[i] = filepath.Join(ledger, "learn", "projects", tp.Key+".json")
		if err := readLearnFile(paths[i], &projects[i]); err != nil {
			return err
		}
	}
	fc := factCtx{root: tree.Root, dir: os.TempDir(), aliases: tree.HostAliases, kinds: tree.HostKinds}
	for i := range projects {
		p := &projects[i]
		present := map[string]bool{}
		for j := range p.Repos {
			r := &p.Repos[j]
			repo := filepath.Join(tree.Root, filepath.FromSlash(r.Path))
			if info, err := os.Stat(filepath.Join(repo, ".git")); err != nil || !info.IsDir() {
				if r.State == "present" {
					res.Missing = append(res.Missing, r.Path)
				}
				r.State = "missing"
				continue
			}
			present[r.Path] = true
			nr, _, _, err := fc.facts(r.Path)
			if err != nil {
				return fmt.Errorf("repository %s: %w", r.Path, err)
			}
			if !headExists(repo, nr.Remote) {
				nr.DefaultBranch = r.DefaultBranch
			}
			if (r.Host.Source == "agent" || r.Host.Source == "owner") && remoteURL(*r) == remoteURL(nr) {
				nr.Host, nr.Kind, nr.APIURL, nr.HostPath = r.Host, r.Kind, r.APIURL, r.HostPath
			}
			*r = nr
		}
		for j := range p.Docs {
			d := &p.Docs[j]
			if !present[d.Repo] {
				continue
			}
			// The stat is the only access to the working tree; no read of the file.
			info, err := os.Stat(filepath.Join(tree.Root, filepath.FromSlash(d.Repo), filepath.FromSlash(d.Path)))
			if gone := err != nil || info.IsDir(); gone != d.Gone {
				d.Gone = gone
				res.GoneDocs = append(res.GoneDocs, goneDoc{Project: p.Key, Repo: d.Repo, Path: d.Path})
			}
		}
		changed, err := writeLearnFile(paths[i], p)
		if err != nil {
			return err
		}
		if changed {
			res.Written = append(res.Written, "learn/projects/"+p.Key+".json")
		}
	}
	return nil
}

// readLearnFile decodes the JSON file path into v and refuses unknown fields. An error names
// the file; a missing file gives an error that wraps fs.ErrNotExist.
func readLearnFile(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// headExists reports whether <repo>/.git/refs/remotes/<remote>/HEAD exists.
func headExists(repo, remote string) bool {
	if remote == "" || slices.Contains(strings.Split(remote, "/"), "..") {
		return false // the remote name must not leave refs/remotes
	}
	_, err := os.Stat(filepath.Join(repo, ".git", "refs", "remotes", filepath.FromSlash(remote), "HEAD"))
	return err == nil
}

// remoteURL returns the URL of the chosen remote of r, or "" when r has none.
func remoteURL(r indexRepo) string {
	if i := slices.IndexFunc(r.Remotes, func(x remote) bool { return x.Name == r.Remote }); i >= 0 {
		return r.Remotes[i].URL
	}
	return ""
}

// learnRefreshTool is the MCP tool learn_refresh.
func learnRefreshTool() Tool {
	return Tool{
		Name:        "learn_refresh",
		Description: "Refresh the index of the ledger (learn/) from the .git folders: remotes, default branch, missing repositories, and gone doc pointers; and list the Markdown files of the ledger over the line cap. Only bigm. No model, code host, or ssh call. Commit the written paths, name each gone_docs entry in the commit, and send one P1 for each missing repository.",
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(c *Call, _ json.RawMessage) (any, error) {
			return learnRefresh(c.Env)
		},
	}
}
