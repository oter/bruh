package main

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// sourced is a value of the index with its source: git, agent, or owner.
type sourced struct {
	Value  string `json:"value"`
	Source string `json:"source"`
}

// hostValue is the host of a repository. Value is nil when an SSH host alias has no host.
type hostValue struct {
	Value  *string `json:"value"`
	Source string  `json:"source"`
}

type remote struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type indexRepo struct {
	Path          string    `json:"path"`
	Remotes       []remote  `json:"remotes"`
	Remote        string    `json:"remote"`
	Host          hostValue `json:"host"`
	Kind          string    `json:"kind"`
	APIURL        string    `json:"api_url"`
	HostPath      string    `json:"host_path"`
	DefaultBranch string    `json:"default_branch"`
	State         string    `json:"state"`
}

type link struct {
	Project string `json:"project"`
	Repo    string `json:"repo"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Source  string `json:"source"`
}

type doc struct {
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Source string `json:"source"`
	Gone   bool   `json:"gone"`
}

type projectFile struct {
	Key     string      `json:"key"`
	Purpose *sourced    `json:"purpose"`
	Main    string      `json:"main"`
	Repos   []indexRepo `json:"repos"`
	Links   []link      `json:"links"`
	Docs    []doc       `json:"docs"`
}

// fill is one accepted value of the learn step for a project of the init answers.
type fill struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Source string `json:"source"`
	Repo   string `json:"repo,omitempty"`
	File   string `json:"file,omitempty"`
	Line   int    `json:"line,omitempty"`
}

// answerProject is one project of the init answers. Fills is nil when the answers have no
// fills key (gap G63 of the build spec).
type answerProject struct {
	Key   string   `json:"key"`
	Repos []string `json:"repos"`
	Main  string   `json:"main"`
	Fills []fill   `json:"fills"`
}

// checkFills checks the fills of one project of the init answers with the value rules of
// spec 8.5. root is the root of the answers, and keys are the project keys of the answers.
// The only access to the working tree is an os.Stat of the file of a doc fill.
func checkFills(root string, project answerProject, keys []string) error {
	var errs []error
	seen := map[fill]bool{}
	purposes := 0
	hosts := map[string]bool{}
	for _, f := range project.Fills {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("project %s: %s %q: %s", project.Key, f.Field, f.Value, fmt.Sprintf(format, args...)))
		}
		if seen[f] {
			bad("the fill is there twice")
			continue
		}
		seen[f] = true
		if !slices.Contains([]string{"purpose", "link", "doc", "host"}, f.Field) {
			bad("field is not purpose, link, doc, or host")
			continue
		}
		if f.Source != "agent" && f.Source != "owner" {
			bad("source %q is not agent or owner", f.Source)
		}
		if strings.ContainsAny(f.Value, "\r\n") || strings.Contains(f.Value, " | ") {
			bad("value has a line break or \" | \"")
		}
		if f.Field == "purpose" {
			if f.Repo != "" || f.File != "" || f.Line != 0 {
				bad("a purpose has no repo, file, or line")
			}
			if n := utf8.RuneCountInString(f.Value); n > 120 {
				bad("purpose has %d characters, more than 120", n)
			}
			if purposes++; purposes == 2 {
				bad("more than one purpose")
			}
			continue
		}
		repoOK := slices.Contains(project.Repos, f.Repo)
		if !repoOK {
			bad("repo %q is not a repository of the project", f.Repo)
		}
		switch f.Field {
		case "link":
			if f.Value == project.Key || !slices.Contains(keys, f.Value) {
				bad("value is not the key of another project")
			}
			if !relPath(f.File) {
				bad("file %q is not a relative path", f.File)
			}
			if f.Line < 1 {
				bad("line %d is less than 1", f.Line)
			}
		case "doc":
			if !relPath(f.Value) {
				bad("value is not a relative path")
			} else if repoOK {
				// The stat is the only access to the working tree; no read of the file.
				if info, err := os.Stat(filepath.Join(root, f.Repo, f.Value)); err != nil || info.IsDir() {
					bad("no file in repo %q", f.Repo)
				}
			}
		case "host":
			if !hostRE.MatchString(f.Value) {
				bad("value does not match %s", hostRE)
			}
			if hosts[f.Repo] {
				bad("more than one host for repo %q", f.Repo)
			}
			hosts[f.Repo] = true
		}
	}
	return errors.Join(errs...)
}

var hostRE = regexp.MustCompile(`^[a-z0-9.-]+$`)

// relPath reports whether p is a relative path with no ".." part that does not start with "/".
func relPath(p string) bool {
	return p != "" && !strings.HasPrefix(p, "/") && !slices.Contains(strings.Split(p, "/"), "..")
}

type treeProject struct {
	Key   string `json:"key"`
	Group string `json:"group"`
}

type treeFile struct {
	Root        string            `json:"root"`
	Depth       int               `json:"depth"`
	Exclude     []string          `json:"exclude"`
	HostAliases map[string]string `json:"host_aliases"`
	HostKinds   map[string]string `json:"host_kinds"`
	Projects    []treeProject     `json:"projects"`
}

// encodeLearn sorts the lists of v (a *treeFile or a *projectFile) and encodes it with a
// two-space indent and no HTML escape. The bytes end with one "\n".
func encodeLearn(v any) ([]byte, error) {
	switch f := v.(type) {
	case *treeFile:
		if f.Exclude == nil {
			f.Exclude = []string{}
		}
		if f.HostAliases == nil {
			f.HostAliases = map[string]string{}
		}
		if f.HostKinds == nil {
			f.HostKinds = map[string]string{}
		}
		if f.Projects == nil {
			f.Projects = []treeProject{}
		}
		slices.Sort(f.Exclude)
		slices.SortFunc(f.Projects, func(a, b treeProject) int { return cmp.Compare(a.Key, b.Key) })
	case *projectFile:
		if f.Repos == nil {
			f.Repos = []indexRepo{}
		}
		if f.Links == nil {
			f.Links = []link{}
		}
		if f.Docs == nil {
			f.Docs = []doc{}
		}
		for i := range f.Repos {
			r := &f.Repos[i]
			if r.Remotes == nil {
				r.Remotes = []remote{}
			}
			slices.SortFunc(r.Remotes, func(a, b remote) int { return cmp.Compare(a.Name, b.Name) })
		}
		slices.SortFunc(f.Repos, func(a, b indexRepo) int { return cmp.Compare(a.Path, b.Path) })
		slices.SortFunc(f.Links, func(a, b link) int {
			return cmp.Or(
				cmp.Compare(a.Project, b.Project),
				cmp.Compare(a.Repo, b.Repo),
				cmp.Compare(a.File, b.File),
				cmp.Compare(a.Line, b.Line),
			)
		})
		slices.SortFunc(f.Docs, func(a, b doc) int {
			return cmp.Or(cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.Path, b.Path))
		})
	default:
		return nil, fmt.Errorf("encodeLearn: unsupported type %T", v)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeLearnFile encodes v with encodeLearn and writes it to path only when the bytes differ
// from the bytes on disk. It creates the missing folders with mode 0755 and writes the file
// with mode 0644 through atomicWrite. It reports whether it wrote the file.
func writeLearnFile(path string, v any) (changed bool, err error) {
	data, err := encodeLearn(v)
	if err != nil {
		return false, err
	}
	old, err := os.ReadFile(path)
	if err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	if err := atomicWrite(path, data); err != nil {
		return false, err
	}
	if err := os.Chmod(path, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// errNoLedger is the error of ledgerPath when <data>/init/config.json does not exist.
var errNoLedger = errors.New("no ledger path in <data>/init/config.json; run /bruh:init")

// ledgerPath returns ledger_path of <data>/init/config.json, which init writes. Only a missing file
// returns errNoLedger. init always writes ledger_path, so a file that cannot be read, is not JSON,
// or has no ledger_path is damaged and returns another error.
func ledgerPath(env Env) (string, error) {
	raw, err := os.ReadFile(filepath.Join(env.DataDir, "init", "config.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", errNoLedger
	}
	if err != nil {
		return "", fmt.Errorf("read <data>/init/config.json: %w", err)
	}
	var cfg initConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", fmt.Errorf("<data>/init/config.json: %w", err)
	}
	if cfg.LedgerPath == "" {
		return "", errors.New("<data>/init/config.json has no ledger_path; run /bruh:init")
	}
	return cfg.LedgerPath, nil
}

// longFiles returns the ledger-relative paths, sorted, of the *.md files under ledger (except
// under .git/ and learn/) that have more lines than the cap: the value of the line
// "ledger_max_lines: <n>" of mode.md, or 300 when the line is missing or bad. The number of
// lines is the number of "\n", plus 1 when the file does not end with "\n".
func longFiles(ledger string) ([]string, error) {
	ledger = filepath.Clean(ledger)
	limit := 300
	mode, err := os.ReadFile(filepath.Join(ledger, "mode.md"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for line := range strings.Lines(string(mode)) {
		v, ok := strings.CutPrefix(strings.TrimRight(line, "\r\n"), "ledger_max_lines: ")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
		break
	}
	long := []string{}
	err = filepath.WalkDir(ledger, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if filepath.Dir(path) == ledger && (d.Name() == ".git" || d.Name() == "learn") {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := bytes.Count(data, []byte("\n"))
		if !bytes.HasSuffix(data, []byte("\n")) {
			lines++
		}
		if lines > limit {
			rel, err := filepath.Rel(ledger, path)
			if err != nil {
				return err
			}
			long = append(long, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.Sort(long)
	return long, nil
}
