package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
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

// sshAliasRE matches an SSH alias that sshHostname passes to ssh.
var sshAliasRE = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)

// sshBin is the ssh binary; tests replace it.
var sshBin = "ssh"

// cliTimeout is the timeout of each CLI call of the scan; tests make it short.
var cliTimeout = 10 * time.Second

// sshHostname runs sshBin -G -- <alias> in dir, a folder outside the repositories, and returns
// the value of its hostname line in lower case. It runs nothing and returns false when the
// alias has a character outside A-Za-z0-9.-; it returns false when the command fails.
func sshHostname(alias, dir string) (string, bool) {
	if !sshAliasRE.MatchString(alias) {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, sshBin, "-G", "--", alias)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second // a child that keeps stdout open does not outlast the timeout
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	for line := range strings.Lines(string(out)) {
		if f := strings.Fields(line); len(f) >= 2 && f[0] == "hostname" {
			return strings.ToLower(f[1]), true
		}
	}
	return "", false
}

// teaBin is the tea binary of Gitea; tests replace it.
var teaBin = "tea"

// loginHosts reads once the hosts of the logged-in CLIs: host -> "gh", "glab", or "tea". Each
// CLI runs in dir with cliTimeout; a CLI that fails gives no hosts. The result is never nil.
func loginHosts(dir string) map[string]string {
	clis := []struct {
		name  string
		bin   *string
		args  []string
		parse func([]byte) ([]string, error)
	}{
		{"gh", &ghBin, []string{"auth", "status", "--json", "hosts"}, parseGHHosts},
		{"glab", &glabBin, []string{"auth", "status", "--all"}, parseGlabHosts},
		{"tea", &teaBin, []string{"logins", "list", "--output", "json"}, parseTeaHosts},
	}
	found := make([][]string, len(clis))
	var wg sync.WaitGroup
	for i, c := range clis {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, *c.bin, c.args...)
			cmd.Dir = dir
			cmd.WaitDelay = time.Second // a child that keeps the output open does not outlast the timeout
			run := cmd.Output
			if c.name == "glab" { // glab prints its status on stderr
				run = cmd.CombinedOutput
			}
			out, err := run()
			if err != nil {
				return
			}
			if hosts, err := c.parse(out); err == nil {
				found[i] = hosts
			}
		})
	}
	wg.Wait()
	logins := map[string]string{}
	for i, c := range clis { // the order gh, glab, tea: the first CLI that names a host wins
		for _, h := range found[i] {
			if _, ok := logins[h]; !ok {
				logins[h] = c.name
			}
		}
	}
	return logins
}

// parseGHHosts returns the keys of the object "hosts" of the JSON object of
// gh auth status --json hosts, in lower case.
func parseGHHosts(out []byte) ([]string, error) {
	var v struct {
		Hosts map[string]json.RawMessage `json:"hosts"`
	}
	if err := json.Unmarshal(out, &v); err != nil {
		return nil, err
	}
	var hosts []string
	for h := range v.Hosts {
		hosts = append(hosts, strings.ToLower(h))
	}
	return hosts, nil
}

// parseGlabHosts returns, in lower case, each line of glab auth status --all with no leading
// white space whose whole text matches sshAliasRE (the same closed grammar) and has a ".".
// Indented lines are ignored.
func parseGlabHosts(out []byte) ([]string, error) {
	var hosts []string
	for line := range strings.Lines(string(out)) {
		line = strings.TrimRight(line, "\r\n")
		if sshAliasRE.MatchString(line) && strings.Contains(line, ".") {
			hosts = append(hosts, strings.ToLower(line))
		}
	}
	return hosts, nil
}

// parseTeaHosts returns the host of the URL in the key "url" (any case of the key name) of
// each object of the JSON array of tea logins list --output json, in lower case.
func parseTeaHosts(out []byte) ([]string, error) {
	var logins []struct {
		URL string `json:"url"` // encoding/json matches the key name in any case
	}
	if err := json.Unmarshal(out, &logins); err != nil {
		return nil, err
	}
	var hosts []string
	for _, l := range logins {
		if u, err := url.Parse(l.URL); err == nil && u.Hostname() != "" {
			hosts = append(hosts, strings.ToLower(u.Hostname()))
		}
	}
	return hosts, nil
}

// hostKind returns github, gitlab, gitea, or unknown for host, in this order: the exact list
// (github.com, gitlab.com); when logins is not nil (only learn_scan), the hosts of the logged-in
// CLIs and the BRUH_GITEA_TOKEN_<HOST> variables; then hostKinds; else unknown.
func hostKind(host string, hostKinds, logins map[string]string) string {
	cliKinds := map[string]string{"gh": "github", "glab": "gitlab", "tea": "gitea"}
	switch {
	case host == "github.com":
		return "github"
	case host == "gitlab.com":
		return "gitlab"
	case logins != nil && cliKinds[logins[host]] != "":
		return cliKinds[logins[host]]
	case logins != nil && os.Getenv("BRUH_GITEA_TOKEN_"+envHostKey(host)) != "":
		return "gitea"
	}
	switch k := hostKinds[host]; k {
	case "github", "gitlab", "gitea":
		return k
	}
	return "unknown"
}

// apiRoot returns the API root of host: https://api.github.com for github.com,
// https://<host>/api/v3 for another GitHub host, https://<host>/api/v1 for Gitea,
// https://<host>/api/v4 for GitLab, and "" for unknown.
func apiRoot(host, kind string) string {
	switch {
	case host == "":
		return ""
	case kind == "github" && host == "github.com":
		return "https://api.github.com"
	case kind == "github":
		return "https://" + host + "/api/v3"
	case kind == "gitea":
		return "https://" + host + "/api/v1"
	case kind == "gitlab":
		return "https://" + host + "/api/v4"
	}
	return ""
}

// factCtx holds the inputs of one read of git facts.
type factCtx struct {
	root    string            // the absolute root
	dir     string            // the working folder of each CLI, outside the repositories
	aliases map[string]string // host_aliases: alias -> host
	kinds   map[string]string // host_kinds: host -> github, gitlab, or gitea
	logins  map[string]string // loginHosts; nil when no CLI may run (init_plan, the CLI init, learn_refresh)
	ssh     bool              // run ssh -G for an alias (only learn_scan)
}

// facts reads the git facts of the repository root/rel from its .git folder only: the remotes,
// the chosen remote, the host, the kind, api_url, host_path, and the default branch, with state
// "present". alias is the host part of the URL of the chosen remote when that URL has the scp
// form or ssh:// and its host is not github.com or gitlab.com, else "". sshHost is the host that
// ssh -G printed when it differs from the alias, else "".
func (f factCtx) facts(rel string) (r indexRepo, alias, sshHost string, err error) {
	repo := filepath.Join(f.root, filepath.FromSlash(rel))
	rs, err := readRemotes(repo)
	r = indexRepo{
		Path:    rel,
		Remotes: slices.SortedStableFunc(slices.Values(rs), func(a, b remote) int { return strings.Compare(a.Name, b.Name) }),
		Remote:  chooseRemote(rs),
		Host:    hostValue{Source: "git"},
		Kind:    "unknown",
		State:   "present",
	}
	if r.Remotes == nil {
		r.Remotes = []remote{}
	}
	r.DefaultBranch = defaultBranch(repo, r.Remote)
	// Entry B1 of decisions.md: no remote, a local path, or a file:// URL keeps the null host.
	i := slices.IndexFunc(rs, func(x remote) bool { return x.Name == r.Remote })
	if i < 0 {
		return r, "", "", err
	}
	_, host, hostPath, sshForm := splitRemoteURL(rs[i].URL)
	if host == "" {
		return r, "", "", err
	}
	r.HostPath = hostPath
	switch {
	case !sshForm || host == "github.com" || host == "gitlab.com":
		r.Host.Value = &host
	case f.ssh && !sshAliasRE.MatchString(host):
		r.Host.Value = &host // build spec A3.1: the alias is the host, and the kind stays unknown
		return r, host, "", err
	default:
		alias = host
		if f.ssh {
			if printed, ok := sshHostname(alias, f.dir); ok && !strings.EqualFold(printed, alias) {
				r.Host.Value, sshHost = &printed, printed
			}
		}
		if h, ok := f.aliases[alias]; ok && r.Host.Value == nil {
			r.Host.Value = &h
		}
	}
	if r.Host.Value != nil {
		r.Kind = hostKind(*r.Host.Value, f.kinds, f.logins)
		r.APIURL = apiRoot(*r.Host.Value, r.Kind)
	}
	return r, alias, sshHost, err
}

// activityResult is the last activity of one repository on its code host, for the pick.
type activityResult struct {
	At    string `json:"at"`    // RFC 3339 UTC, or ""
	State string `json:"state"` // ok, no login, or unknown
}

// activity reads the last activity of each repository, only for a host with a working login: a
// host of logins (loginHosts), or a Gitea host with a BRUH_GITEA_TOKEN_<HOST> variable. At most
// 8 calls run at the same time, each with cliTimeout, each CLI in dir. The result has the order
// of repos.
func activity(repos []indexRepo, logins map[string]string, dir string) []activityResult {
	out := make([]activityResult, len(repos))
	sem := make(chan struct{}, 8)
	var wg sync.WaitGroup
	for i, r := range repos {
		wg.Go(func() {
			sem <- struct{}{}
			defer func() { <-sem }()
			out[i] = repoActivity(r, logins, dir)
		})
	}
	wg.Wait()
	return out
}

// repoActivity reads the last activity of one repository by the rules of activity: no call and
// "no login" without a working login, "unknown" for each failed call.
func repoActivity(r indexRepo, logins map[string]string, dir string) activityResult {
	if r.Host.Value == nil {
		return activityResult{State: "no login"}
	}
	host := *r.Host.Value
	ctx, cancel := context.WithTimeout(context.Background(), cliTimeout)
	defer cancel()
	run := func(bin string, args ...string) (map[string]json.RawMessage, error) {
		cmd := exec.CommandContext(ctx, bin, args...)
		cmd.Dir = dir
		cmd.WaitDelay = time.Second // a child that keeps stdout open does not outlast the timeout
		out, err := cmd.Output()
		if err != nil {
			return nil, err
		}
		var v map[string]json.RawMessage
		return v, json.Unmarshal(out, &v)
	}
	var v map[string]json.RawMessage
	var field string
	var err error
	switch token := os.Getenv("BRUH_GITEA_TOKEN_" + envHostKey(host)); {
	case r.Kind == "github" && logins[host] == "gh":
		field = "pushed_at"
		v, err = run(ghBin, "api", "--hostname", host, "repos/"+r.HostPath)
	case r.Kind == "gitlab" && logins[host] == "glab":
		field = "last_activity_at"
		v, err = run(glabBin, "api", "--hostname", host, "projects/"+url.PathEscape(r.HostPath))
	case r.Kind == "gitea" && token != "":
		field = "updated_at"
		c := rest{base: r.APIURL, auth: "token " + token, hc: hostHTTP}
		err = c.do(ctx, "GET", "/repos/"+r.HostPath, nil, &v)
	default:
		return activityResult{State: "no login"}
	}
	var at string
	if err != nil || json.Unmarshal(v[field], &at) != nil {
		return activityResult{State: "unknown"}
	}
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return activityResult{State: "unknown"}
	}
	return activityResult{At: t.UTC().Format(time.RFC3339), State: "ok"}
}
