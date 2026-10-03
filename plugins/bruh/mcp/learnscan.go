package main

import (
	"cmp"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// The git facts scan core. Each function of this file reads only folder entries and files
// inside .git folders. It runs no git, task, make, or other command of a repository, and it
// reads no file of the working tree (D5).

// repoDir is one repository that walkRepos found.
type repoDir struct {
	Path  string // relative to the root, with / separators
	Group string // the path of the parent folder relative to the root, or ""
}

// walkRepos returns the repositories under root with a depth from 1 to depth, sorted by Path.
// A repository is a folder that has a .git folder. ledger is the ledger folder, which it skips.
func walkRepos(root string, depth int, exclude []string, ledger string) []repoDir {
	ledger = resolvePath(ledger)
	var repos []repoDir
	var walk func(rel string, level int)
	walk = func(rel string, level int) {
		entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return // an unreadable folder is skipped
		}
		for _, e := range entries {
			if !e.IsDir() { // a symbolic link to a folder is not a folder here
				continue
			}
			child := path.Join(rel, e.Name())
			dir := filepath.Join(root, filepath.FromSlash(child))
			if e.Name() == ".claude" || excluded(child, e.Name(), exclude) || resolvePath(dir) == ledger {
				continue
			}
			info, err := os.Lstat(filepath.Join(dir, ".git"))
			switch {
			case err == nil && info.IsDir():
				repos = append(repos, repoDir{Path: child, Group: rel})
			case err == nil:
				// A .git file: a linked worktree, skipped.
			case level < depth:
				walk(child, level+1)
			}
		}
	}
	if depth >= 1 {
		walk("", 1)
	}
	slices.SortFunc(repos, func(a, b repoDir) int { return strings.Compare(a.Path, b.Path) })
	return repos
}

// resolvePath returns p with its symbolic links resolved, or p cleaned when that fails.
func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// excluded reports whether the folder rel (relative to the root, / separators) with the
// name name matches an exclude entry: an entry without / matches a folder name at any
// depth, an entry with / matches a path relative to the root.
func excluded(rel, name string, exclude []string) bool {
	for _, e := range exclude {
		if strings.Contains(e, "/") {
			if path.Clean(e) == rel {
				return true
			}
		} else if e == name {
			return true
		}
	}
	return false
}

// splitRemoteURL parses a git remote URL. clean is the URL without user information for the
// http and https forms (conflict C1); the scp form and ssh:// keep their SSH user. host is the
// host part in lower case, without user and port. hostPath is the path on the host without a
// leading "/" and without a trailing ".git". sshForm is true for the scp form and for ssh://.
// A local path or a file:// URL gives an empty host and hostPath.
func splitRemoteURL(raw string) (clean, host, hostPath string, sshForm bool) {
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil {
			return "", "", "", false // the user information of a broken URL cannot be removed
		}
		clean = raw
		if u.Scheme == "http" || u.Scheme == "https" {
			u.User = nil
			clean = u.String()
		}
		if u.Scheme == "file" || u.Hostname() == "" {
			return clean, "", "", false
		}
		return clean, strings.ToLower(u.Hostname()), trimRepoPath(u.Path), u.Scheme == "ssh"
	}
	// The scp form [user@]host:path: a colon with no "/" before it, as git reads it.
	hostPart, p, ok := strings.Cut(raw, ":")
	if !ok || hostPart == "" || strings.Contains(hostPart, "/") {
		return raw, "", "", false // a local path
	}
	if i := strings.LastIndex(hostPart, "@"); i >= 0 {
		hostPart = hostPart[i+1:]
	}
	return raw, strings.ToLower(hostPart), trimRepoPath(p), true
}

// trimRepoPath removes one leading "/" and one trailing ".git" from p.
func trimRepoPath(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(p, "/"), ".git")
}

// readRemotes reads the [remote "<name>"] sections of <repo>/.git/config as a file with a
// closed line grammar, and returns them in file order. Each URL is the clean URL of
// splitRemoteURL. A missing .git/config gives no remotes and no error.
func readRemotes(repo string) ([]remote, error) {
	data, err := os.ReadFile(filepath.Join(repo, ".git", "config"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// ponytail: a closed line grammar; it does not handle inline comments, include files, or
	// line continuations, and a value keeps a "#" in it. A full git config parser if needed.
	var rs []remote
	name, inRemote, haveURL := "", false, false
	for line := range strings.Lines(string(data)) {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || line[0] == '#' || line[0] == ';':
		case line[0] == '[':
			n, ok := strings.CutPrefix(line, `[remote "`)
			n, ok2 := strings.CutSuffix(n, `"]`)
			name, inRemote, haveURL = n, ok && ok2, false
		case inRemote && !haveURL:
			key, value, ok := strings.Cut(line, "=")
			if !ok || !strings.EqualFold(strings.TrimSpace(key), "url") {
				continue
			}
			value = strings.TrimSpace(value)
			if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
				value = value[1 : len(value)-1]
			}
			clean, _, _, _ := splitRemoteURL(value)
			rs = append(rs, remote{Name: name, URL: clean})
			haveURL = true
		}
	}
	return rs, nil
}

// chooseRemote returns "origin" when rs has it, else the name of the first remote, else "".
func chooseRemote(rs []remote) string {
	if slices.ContainsFunc(rs, func(r remote) bool { return r.Name == "origin" }) {
		return "origin"
	}
	if len(rs) > 0 {
		return rs[0].Name
	}
	return ""
}

// defaultBranch reads <repo>/.git/refs/remotes/<remote>/HEAD. The content
// "ref: refs/remotes/<remote>/<branch>" gives <branch>; a missing or other file gives
// "unknown". It never reads .git/HEAD.
func defaultBranch(repo, remote string) string {
	if remote == "" || slices.Contains(strings.Split(remote, "/"), "..") {
		return "unknown" // the remote name must not leave refs/remotes
	}
	data, err := os.ReadFile(filepath.Join(repo, ".git", "refs", "remotes", filepath.FromSlash(remote), "HEAD"))
	if err != nil {
		return "unknown"
	}
	branch, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "ref: refs/remotes/"+remote+"/")
	if !ok || branch == "" {
		return "unknown"
	}
	return branch
}

// projectProposal is one project that learn_scan proposes.
type projectProposal struct {
	Key   string   `json:"key"`
	Repos []string `json:"repos"`
	Main  string   `json:"main"`
	Group string   `json:"-"`
}

// projectKey turns a project name into a key: lower case, each run of characters that are not
// [a-z0-9] becomes one "-", and a "-" at the start or the end is removed.
func projectKey(name string) string {
	return strings.Trim(nonSlugRE.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// proposeProjects groups the repositories of each group into projects by name, and gives each
// project a key. The result is sorted by Key.
func proposeProjects(repos []repoDir) []projectProposal {
	// Each repository joins the project of the shortest name P of its group that its name
	// equals or starts with P- or P. (a folder name rule, structure only).
	sorted := slices.Clone(repos)
	slices.SortFunc(sorted, func(a, b repoDir) int {
		return cmp.Or(cmp.Compare(len(path.Base(a.Path)), len(path.Base(b.Path))), strings.Compare(a.Path, b.Path))
	})
	var projects []*projectProposal
	var names []string // the project name P of each entry of projects
	for _, r := range sorted {
		name := path.Base(r.Path)
		var p *projectProposal
		for i, q := range projects {
			if q.Group == r.Group && (strings.HasPrefix(name, names[i]+"-") || strings.HasPrefix(name, names[i]+".")) {
				p = q // projects is in the order of name length, so the first match is the shortest
				break
			}
		}
		if p == nil {
			p = &projectProposal{Key: projectKey(name), Main: r.Path, Group: r.Group}
			projects = append(projects, p)
			names = append(names, name)
		}
		p.Repos = append(p.Repos, r.Path)
	}

	// Collisions across groups: first the last element of the group path as a prefix, then the
	// whole group path (G15). A project of the group "" keeps its key. The prefix is normalized
	// with the key, so a group name with no [a-z0-9] adds no "-".
	base := make([]string, len(projects))
	for i, p := range projects {
		base[i] = p.Key
	}
	for _, prefix := range []func(group string) string{path.Base, func(g string) string { return g }} {
		count := map[string]int{}
		for _, p := range projects {
			count[p.Key]++
		}
		for i, p := range projects {
			if count[p.Key] > 1 && p.Group != "" && base[i] != "" { // an empty key goes to the owner
				p.Key = projectKey(prefix(p.Group) + "-" + base[i])
			}
		}
	}

	out := make([]projectProposal, len(projects))
	for i, p := range projects {
		slices.SortFunc(p.Repos, strings.Compare)
		out[i] = *p
	}
	slices.SortFunc(out, func(a, b projectProposal) int { return strings.Compare(a.Key, b.Key) })
	return out
}
