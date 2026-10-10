package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// projectJSON builds the index file of one project of the init answers: the git facts of each
// repository (facts with no CLI and no ssh call, state "present"), then the fills.
func projectJSON(root string, p answerProject, aliases, kinds map[string]string) (projectFile, error) {
	fc := newFactCtx(root, os.TempDir(), aliases, kinds)
	out := projectFile{Key: p.Key, Main: p.Main}
	for _, rel := range p.Repos {
		r, _, _, err := fc.facts(rel)
		if err != nil {
			return projectFile{}, fmt.Errorf("repository %s: %w", rel, err)
		}
		out.Repos = append(out.Repos, r)
	}
	for _, f := range p.Fills {
		switch f.Field {
		case "purpose":
			out.Purpose = &sourced{Value: f.Value, Source: f.Source}
		case "link":
			out.Links = append(out.Links, link{Project: f.Value, Repo: f.Repo, File: f.File, Line: f.Line, Source: f.Source})
		case "doc":
			out.Docs = append(out.Docs, doc{Repo: f.Repo, Path: f.Value, Source: f.Source})
		case "host":
			// checkFills checked that f.Repo is a repository of the project.
			i := slices.IndexFunc(out.Repos, func(r indexRepo) bool { return r.Path == f.Repo })
			if i < 0 {
				continue
			}
			r := &out.Repos[i]
			r.Host = hostValue{Value: &f.Value, Source: f.Source}
			r.Kind = hostKind(strings.ToLower(f.Value), fc.kinds, nil)
			r.APIURL = apiRoot(f.Value, r.Kind)
		}
	}
	return out, nil
}

// learnPlan is the part of the init plan that the projects of the answers give.
type learnPlan struct {
	Write    map[string][]byte // absolute path -> content: learn/tree.json, learn/projects/<key>.json, projects/<key>.md
	Delete   []string          // absolute paths: the JSON of each removed project, and its project file with no data row
	Kept     []string          // absolute paths: the project file of each removed project that has a data row
	Projects []projectFile     // the index of each project of the answers, as planned or as stored
}

// planLearn plans the index files and the project files for the projects of the answers. With no
// projects key in the answers, it plans nothing (G27). It writes nothing.
func planLearn(pluginRoot string, a InitAnswers) (learnPlan, error) {
	var lp learnPlan
	if a.Projects == nil {
		return lp, nil
	}
	lp.Write = map[string][]byte{}
	learnDir := filepath.Join(a.LedgerPath, "learn")
	jsonPath := func(key string) string { return filepath.Join(learnDir, "projects", key+".json") }
	mdPath := func(key string) string { return filepath.Join(a.LedgerPath, "projects", key+".md") }
	tmpl, err := projectTemplateFile(pluginRoot)
	if err != nil {
		return learnPlan{}, err
	}
	tree := treeFile{Root: a.Root, Depth: a.Depth, Exclude: slices.Clone(a.Exclude), HostAliases: a.HostAliases, HostKinds: a.HostKinds}
	for _, p := range a.Projects {
		tree.Projects = append(tree.Projects, treeProject{Key: p.Key, Group: projectGroup(p.Main)})
		pf, keep, err := storedUnchanged(jsonPath(p.Key), p)
		if err != nil {
			return learnPlan{}, err
		}
		if !keep {
			if pf, err = projectJSON(a.Root, p, a.HostAliases, a.HostKinds); err != nil {
				return learnPlan{}, fmt.Errorf("project %s: %w", p.Key, err)
			}
			data, err := encodeLearn(&pf)
			if err != nil {
				return learnPlan{}, err
			}
			lp.Write[jsonPath(p.Key)] = data
		}
		lp.Projects = append(lp.Projects, pf)
		if _, err := os.Lstat(mdPath(p.Key)); errors.Is(err, fs.ErrNotExist) {
			lp.Write[mdPath(p.Key)] = projectMD(tmpl, p.Key)
		} else if err != nil {
			return learnPlan{}, err
		}
	}
	data, err := encodeLearn(&tree)
	if err != nil {
		return learnPlan{}, err
	}
	lp.Write[filepath.Join(learnDir, "tree.json")] = data

	var stored treeFile
	if _, err := readStoredLearn(filepath.Join(learnDir, "tree.json"), &stored); err != nil {
		return learnPlan{}, err
	}
	for _, sp := range stored.Projects {
		if slices.ContainsFunc(a.Projects, func(p answerProject) bool { return p.Key == sp.Key }) {
			continue
		}
		// The key builds the paths to delete, so a bad key of the stored file is an error.
		if !projectRE.MatchString(sp.Key) {
			return learnPlan{}, fmt.Errorf("learn/tree.json: project key %q does not match %s", sp.Key, projectRE)
		}
		if _, err := os.Lstat(jsonPath(sp.Key)); err == nil {
			lp.Delete = append(lp.Delete, jsonPath(sp.Key))
		} else if !errors.Is(err, fs.ErrNotExist) {
			return learnPlan{}, err
		}
		md, err := os.ReadFile(mdPath(sp.Key))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			return learnPlan{}, err
		case hasDataRow(string(md)):
			lp.Kept = append(lp.Kept, mdPath(sp.Key))
		default:
			lp.Delete = append(lp.Delete, mdPath(sp.Key))
		}
	}
	slices.Sort(lp.Delete)
	slices.Sort(lp.Kept)
	return lp, nil
}

// projectGroup is the group of a project: the folder of its main repository (spec 423), or ""
// at the root.
func projectGroup(main string) string {
	if g := path.Dir(main); g != "." {
		return g
	}
	return ""
}

// projectTemplateFile reads the template of the project files of the ledger.
func projectTemplateFile(pluginRoot string) ([]byte, error) {
	return os.ReadFile(filepath.Join(pluginRoot, "ledger-template", "projects", "_template.md"))
}

// projectMD is the project file projects/<key>.md that the template tmpl gives.
func projectMD(tmpl []byte, key string) []byte {
	return []byte(strings.ReplaceAll(string(tmpl), "<project>", key))
}

// storedUnchanged returns the stored index file of p when the answers do not change it (G63): p
// has no fills key, and the key, the repositories as a set, and the main repository equal the
// stored file.
func storedUnchanged(file string, p answerProject) (projectFile, bool, error) {
	if p.Fills != nil {
		return projectFile{}, false, nil
	}
	var old projectFile
	found, err := readStoredLearn(file, &old)
	if !found || err != nil {
		return projectFile{}, false, err
	}
	repos := make([]string, 0, len(old.Repos))
	for _, r := range old.Repos {
		repos = append(repos, r.Path)
	}
	slices.Sort(repos)
	want := slices.Sorted(slices.Values(p.Repos))
	return old, old.Key == p.Key && old.Main == p.Main && slices.Equal(repos, want), nil
}

// readStoredLearn decodes the learn file at path into v and refuses unknown fields. A missing file
// is not an error: found is false.
func readStoredLearn(file string, v any) (found bool, err error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return true, fmt.Errorf("%s: %w", file, err)
	}
	return true, nil
}

// hasDataRow reports whether a table of the Markdown text has a data row: a "|" line after the
// separator line ("|---|...") of the table (G26).
func hasDataRow(text string) bool {
	inTable := false
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		switch {
		case !strings.HasPrefix(line, "|"):
			inTable = false
		case inTable:
			return true
		case strings.Contains(line, "-") && strings.Trim(line, "|-: ") == "":
			inTable = true
		}
	}
	return false
}
