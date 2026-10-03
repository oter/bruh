package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestScanFindsReposAndSkips(t *testing.T) {
	root := t.TempDir()
	mkdir := func(rel string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range []string{
		"shop/.git",                // depth 1: found
		"group-a/sub/auth/.git",    // depth 3: found
		"group-a/a/b/c/deep/.git",  // depth 5: below depth 4
		".claude/worktrees/x/.git", // under .claude
		"shop/vendor/inner/.git",   // a repository inside a repository
		t.Name() + "/.git",         // the ledger
		"archive/old/.git",         // excluded by name
		"group-a/skip/.git",        // excluded by path
		"worktree",                 // a linked worktree, its .git is a file below
	} {
		mkdir(rel)
	}
	if err := os.WriteFile(filepath.Join(root, "worktree", ".git"), []byte("gitdir: ../shop/.git/worktrees/w\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "shop"), filepath.Join(root, "link-to-shop")); err != nil {
		t.Fatal(err)
	}
	ledger := filepath.Join(root, t.Name()) + string(filepath.Separator) // not clean on purpose

	got := walkRepos(root, 4, []string{"archive", "group-a/skip"}, ledger)
	want := []repoDir{
		{Path: "group-a/sub/auth", Group: "group-a/sub"},
		{Path: "shop", Group: ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("walkRepos(root, 4, [archive group-a/skip], ledger) = %+v, want %+v", got, want)
	}
}

func TestRemotesDropUserInfo(t *testing.T) {
	tests := []struct {
		raw, clean, host, hostPath string
		sshForm                    bool
	}{
		{"https://oauth2:glpat-x@gitlab.com/g/s/r.git", "https://gitlab.com/g/s/r.git", "gitlab.com", "g/s/r", false},
		{"http://user@git.example.com/a/b", "http://git.example.com/a/b", "git.example.com", "a/b", false},
		{"https://github.com/owner/shop", "https://github.com/owner/shop", "github.com", "owner/shop", false},
		{"https://git.example.com:8443/team/shop.git", "https://git.example.com:8443/team/shop.git", "git.example.com", "team/shop", false},
		{"git@gitlab.com:team/shop.git", "git@gitlab.com:team/shop.git", "gitlab.com", "team/shop", true},
		{"git@gitlab.com-work:team/sub/shop.git", "git@gitlab.com-work:team/sub/shop.git", "gitlab.com-work", "team/sub/shop", true},
		{"gitlab.com-work:team/shop", "gitlab.com-work:team/shop", "gitlab.com-work", "team/shop", true},
		{"ssh://git@gitlab.com-work:2222/team/shop.git", "ssh://git@gitlab.com-work:2222/team/shop.git", "gitlab.com-work", "team/shop", true},
		{"ssh://Git.Example.com/team/shop", "ssh://Git.Example.com/team/shop", "git.example.com", "team/shop", true},
		{"/srv/git/shop.git", "/srv/git/shop.git", "", "", false},
		{"../shop", "../shop", "", "", false},
		{"file:///srv/git/shop.git", "file:///srv/git/shop.git", "", "", false},
	}
	for _, tt := range tests {
		clean, host, hostPath, sshForm := splitRemoteURL(tt.raw)
		if clean != tt.clean || host != tt.host || hostPath != tt.hostPath || sshForm != tt.sshForm {
			t.Errorf("splitRemoteURL(%q) = %q, %q, %q, %t, want %q, %q, %q, %t",
				tt.raw, clean, host, hostPath, sshForm, tt.clean, tt.host, tt.hostPath, tt.sshForm)
		}
		if strings.HasPrefix(tt.raw, "http") && strings.Contains(clean, "@") {
			t.Errorf("splitRemoteURL(%q) clean = %q, want no user information", tt.raw, clean)
		}
	}
}

// writeGitConfig makes a repository folder under t.TempDir() with config as .git/config.
func writeGitConfig(t *testing.T, config string) string {
	t.Helper()
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

func TestRemoteOriginFirst(t *testing.T) {
	tests := []struct {
		name, config string
		want         []remote
		chosen       string
	}{
		{
			name: "origin after upstream",
			config: "[remote \"upstream\"]\n\turl = https://git.example.com/team/shop.git\n" +
				"[remote \"origin\"]\n\turl = git@git.example.com:me/shop.git\n",
			want: []remote{
				{Name: "upstream", URL: "https://git.example.com/team/shop.git"},
				{Name: "origin", URL: "git@git.example.com:me/shop.git"},
			},
			chosen: "origin",
		},
		{
			name: "no origin",
			config: "[remote \"upstream\"]\n\turl = https://git.example.com/team/shop.git\n" +
				"[remote \"fork\"]\n\turl = https://git.example.com/me/shop.git\n",
			want: []remote{
				{Name: "upstream", URL: "https://git.example.com/team/shop.git"},
				{Name: "fork", URL: "https://git.example.com/me/shop.git"},
			},
			chosen: "upstream",
		},
		{
			name:   "no remotes",
			config: "[core]\n\tbare = false\n",
			chosen: "",
		},
		{
			name:   "empty remote name skipped",
			config: "[remote \"\"]\n\turl = https://h/x\n[remote \"origin\"]\n\turl = https://h/y\n",
			want:   []remote{{Name: "origin", URL: "https://h/y"}},
			chosen: "origin",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readRemotes(writeGitConfig(t, tt.config))
			if err != nil {
				t.Fatalf("readRemotes() error = %v, want nil", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("readRemotes() = %+v, want %+v", got, tt.want)
			}
			if chosen := chooseRemote(got); chosen != tt.chosen {
				t.Errorf("chooseRemote(%+v) = %q, want %q", got, chosen, tt.chosen)
			}
		})
	}
	if chosen := chooseRemote(nil); chosen != "" {
		t.Errorf("chooseRemote(nil) = %q, want \"\"", chosen)
	}
}

func TestReadRemotesGrammar(t *testing.T) {
	const config = `# a comment
; another comment
[core]
	fsmonitor = touch y
	url = https://git.example.com/core.git
[remote "tight"]
	url=https://git.example.com/tight.git
	url = https://git.example.com/second.git
	fetch = +refs/heads/*:refs/remotes/tight/*
[branch "main"]
	remote = tight
	url = https://git.example.com/branch.git
[remote "wide"]
	# url = https://git.example.com/hash.git
	; url = https://git.example.com/semicolon.git
	url   =   https://git.example.com/wide.git
[remote "upper"]
	URL = git@git.example.com:team/upper.git
[remote "push-only"]
	pushurl = https://git.example.com/push.git
[remote "quoted"]
	url = "https://git.example.com/quoted.git"
[remote "token"]
	url = https://oauth2:glpat-x@git.example.com/team/shop.git
`
	repo := writeGitConfig(t, config)
	got, err := readRemotes(repo)
	if err != nil {
		t.Fatalf("readRemotes() error = %v, want nil", err)
	}
	want := []remote{
		{Name: "tight", URL: "https://git.example.com/tight.git"},
		{Name: "wide", URL: "https://git.example.com/wide.git"},
		{Name: "upper", URL: "git@git.example.com:team/upper.git"},
		{Name: "quoted", URL: "https://git.example.com/quoted.git"},
		{Name: "token", URL: "https://git.example.com/team/shop.git"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("readRemotes() = %+v, want %+v", got, want)
	}
	for _, p := range []string{filepath.Join(repo, "y"), filepath.Join(repo, ".git", "y"), "y"} {
		if _, err := os.Lstat(p); err == nil {
			t.Errorf("file %s exists after readRemotes, want no command run", p)
		}
	}

	noConfig := t.TempDir()
	if err := os.Mkdir(filepath.Join(noConfig, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err = readRemotes(noConfig)
	if err != nil || len(got) != 0 {
		t.Errorf("readRemotes(repo without .git/config) = %+v, %v, want no remotes and nil", got, err)
	}
}

func TestDefaultBranchFromRemoteHEAD(t *testing.T) {
	tests := []struct {
		name    string
		remote  string
		missing bool   // no file at .git/refs/remotes/<remote>/HEAD
		content string // the contents of that file
		want    string
	}{
		{name: "main with newline", remote: "origin", content: "ref: refs/remotes/origin/main\n", want: "main"},
		{name: "branch with slash", remote: "origin", content: "ref: refs/remotes/origin/release/v1", want: "release/v1"},
		{name: "missing file", remote: "origin", missing: true, want: "unknown"},
		{name: "sha", remote: "origin", content: "0123456789abcdef0123456789abcdef01234567\n", want: "unknown"},
		{name: "local branch ref", remote: "origin", content: "ref: refs/heads/main\n", want: "unknown"},
		{name: "other remote", remote: "origin", content: "ref: refs/remotes/upstream/main\n", want: "unknown"},
		{name: "empty file", remote: "origin", content: "", want: "unknown"},
		// The file exists and names the remote, so only the check of the remote name refuses it.
		{name: "empty remote", remote: "", content: "ref: refs/remotes//main\n", want: "unknown"},
		{name: "dot-dot remote", remote: "../remotes/origin", content: "ref: refs/remotes/../remotes/origin/main\n", want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := t.TempDir()
			gitDir := filepath.Join(repo, ".git")
			if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes"), 0o755); err != nil {
				t.Fatal(err)
			}
			// .git/HEAD names another branch; defaultBranch must not read it.
			if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/feature\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if !tt.missing {
				file := filepath.Join(gitDir, "refs", "remotes", filepath.FromSlash(tt.remote), "HEAD")
				if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte(tt.content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if got := defaultBranch(repo, tt.remote); got != tt.want {
				t.Errorf("defaultBranch(repo, %q) with %q = %q, want %q", tt.remote, tt.content, got, tt.want)
			}
		})
	}
}

func TestProposeProjectsByPrefix(t *testing.T) {
	tests := []struct {
		name  string
		repos []repoDir
		want  []projectProposal
	}{
		{
			name: "prefix joins one group",
			repos: []repoDir{
				{Path: "shop", Group: ""},
				{Path: "shop-app", Group: ""},
				{Path: "shop.web", Group: ""},
			},
			want: []projectProposal{
				{Key: "shop", Repos: []string{"shop", "shop-app", "shop.web"}, Main: "shop", Group: ""},
			},
		},
		{
			name: "no common repository name",
			repos: []repoDir{
				{Path: "foo-a", Group: ""},
				{Path: "foo-b", Group: ""},
			},
			want: []projectProposal{
				{Key: "foo-a", Repos: []string{"foo-a"}, Main: "foo-a", Group: ""},
				{Key: "foo-b", Repos: []string{"foo-b"}, Main: "foo-b", Group: ""},
			},
		},
		{
			name: "no join across groups",
			repos: []repoDir{
				{Path: "group-a/shop", Group: "group-a"},
				{Path: "group-b/shop-app", Group: "group-b"},
			},
			want: []projectProposal{
				{Key: "shop", Repos: []string{"group-a/shop"}, Main: "group-a/shop", Group: "group-a"},
				{Key: "shop-app", Repos: []string{"group-b/shop-app"}, Main: "group-b/shop-app", Group: "group-b"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := proposeProjects(tt.repos); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("proposeProjects(%+v) = %+v, want %+v", tt.repos, got, tt.want)
			}
		})
	}
}

func TestProjectKeyRule(t *testing.T) {
	long := strings.Repeat("a", 50)
	for _, tt := range []struct{ name, want string }{
		{"My_App", "my-app"},
		{"--x--", "x"},
		{"a..b", "a-b"},
		{"___", ""},
		{long, long}, // longer than 40 characters: returned as it is
	} {
		if got := projectKey(tt.name); got != tt.want {
			t.Errorf("projectKey(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}

	collisions := []struct {
		name  string
		repos []repoDir
		want  []projectProposal
	}{
		{
			name: "last element of the group path",
			repos: []repoDir{
				{Path: "group-a/infra", Group: "group-a"},
				{Path: "group-b/infra", Group: "group-b"},
			},
			want: []projectProposal{
				{Key: "group-a-infra", Repos: []string{"group-a/infra"}, Main: "group-a/infra", Group: "group-a"},
				{Key: "group-b-infra", Repos: []string{"group-b/infra"}, Main: "group-b/infra", Group: "group-b"},
			},
		},
		{
			name: "whole group path",
			repos: []repoDir{
				{Path: "x/group-a/infra", Group: "x/group-a"},
				{Path: "y/group-a/infra", Group: "y/group-a"},
			},
			want: []projectProposal{
				{Key: "x-group-a-infra", Repos: []string{"x/group-a/infra"}, Main: "x/group-a/infra", Group: "x/group-a"},
				{Key: "y-group-a-infra", Repos: []string{"y/group-a/infra"}, Main: "y/group-a/infra", Group: "y/group-a"},
			},
		},
		{
			name: "root group keeps its key",
			repos: []repoDir{
				{Path: "group-a/infra", Group: "group-a"},
				{Path: "infra", Group: ""},
			},
			want: []projectProposal{
				{Key: "group-a-infra", Repos: []string{"group-a/infra"}, Main: "group-a/infra", Group: "group-a"},
				{Key: "infra", Repos: []string{"infra"}, Main: "infra", Group: ""},
			},
		},
		{
			name:  "long key returned as it is",
			repos: []repoDir{{Path: long, Group: ""}},
			want:  []projectProposal{{Key: long, Repos: []string{long}, Main: long, Group: ""}},
		},
	}
	for _, tt := range collisions {
		t.Run(tt.name, func(t *testing.T) {
			if got := proposeProjects(tt.repos); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("proposeProjects(%+v) = %+v, want %+v", tt.repos, got, tt.want)
			}
		})
	}

	for _, name := range []string{
		"My_App", "--x--", "a..b", "Shop.Web", "über-tool", "foo  bar", "A1-B2_c3", "x", "9lives",
		"_lead", "trail_", "UPPER", "dots.and-dashes_and spaces", strings.Repeat("ab_", 13) + "z",
	} {
		key := projectKey(name)
		if len(key) >= 1 && len(key) <= 40 && !projectRE.MatchString(key) {
			t.Errorf("projectKey(%q) = %q, want a match of %s", name, key, projectRE)
		}
	}
}

func TestSSHArgsEndOptions(t *testing.T) {
	log := fakeCLI(t, &sshBin, `pwd -P >> "$log"
printf 'user git\nhostname GitLab.com\nport 22\n'`)
	dir := t.TempDir()
	host, ok := sshHostname("gitlab.com-work", dir)
	if host != "gitlab.com" || !ok {
		t.Errorf("sshHostname(%q) = %q, %v, want %q, true", "gitlab.com-work", host, ok, "gitlab.com")
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := "-G -- gitlab.com-work\n" + realDir + "\n"
	if data, _ := os.ReadFile(log); string(data) != want {
		t.Errorf("ssh log = %q, want %q", data, want)
	}
}

func TestSSHAliasBadCharsNoCall(t *testing.T) {
	log := fakeCLI(t, &sshBin, `printf 'hostname x\n'`)
	dir := t.TempDir()
	for _, alias := range []string{"x;touch z", "a b", "-oProxyCommand=x", ""} {
		if host, ok := sshHostname(alias, dir); ok {
			t.Errorf("sshHostname(%q) = %q, true, want false", alias, host)
		}
	}
	if data, _ := os.ReadFile(log); len(data) != 0 {
		t.Errorf("ssh log = %q, want empty", data)
	}
	if _, err := os.Stat(filepath.Join(dir, "z")); err == nil {
		t.Error("file z exists in dir")
	}
}

func TestSSHHostnameFails(t *testing.T) {
	dir := t.TempDir()
	t.Run("exit 1", func(t *testing.T) {
		fakeCLI(t, &sshBin, `printf 'hostname gitlab.com\n'; exit 1`)
		if host, ok := sshHostname("gitlab.com-work", dir); ok {
			t.Errorf("sshHostname = %q, true, want false", host)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		old := cliTimeout
		cliTimeout = 200 * time.Millisecond
		t.Cleanup(func() { cliTimeout = old })
		fakeCLI(t, &sshBin, `sleep 5; printf 'hostname gitlab.com\n'`)
		start := time.Now()
		host, ok := sshHostname("gitlab.com-work", dir)
		if ok {
			t.Errorf("sshHostname = %q, true, want false", host)
		}
		if d := time.Since(start); d > 2*time.Second {
			t.Errorf("sshHostname took %v, want at most 2s", d)
		}
	})
}

func TestLoginHosts(t *testing.T) {
	// The outputs follow the shapes that the CLIs print (probed for G38), with neutral hosts.
	// glab prints its status on stderr; gh and tea print JSON on stdout. ghe.example.com is
	// also a glab host and gitlab.example.com also a tea host: the order gh, glab, tea wins.
	const (
		ghOut   = `{"hosts":{"github.com":[{"state":"success","active":true,"host":"github.com","login":"u","tokenSource":"keyring"}],"GHE.example.com":[{"state":"success","active":true,"host":"GHE.example.com","login":"u"}]}}`
		glabErr = "gitlab.com\n  ✓ Logged in to gitlab.com as u (keyring)\n  ✓ Token: **************************\n  example.net\nghe.example.com\n  x ghe.example.com: API call failed\nlocalhost\nFailed to read config\n"
		glabOut = "GitLab.Example.com\n  ✓ Logged in to gitlab.example.com as u\n"
		teaOut  = `[{"default":true,"name":"a","ssh_host":"","URL":"https://Git.Example.Org","user":"u"},{"default":false,"name":"b","ssh_host":"","url":"https://gitlab.example.com/sub/","user":"u"}]`
	)
	emit := func(stdout, stderr string) string {
		return "cat <<'EOF'\n" + stdout + "\nEOF\ncat >&2 <<'EOF'\n" + stderr + "\nEOF"
	}
	ok := map[string]string{"gh": emit(ghOut, ""), "glab": emit(glabOut, glabErr), "tea": emit(teaOut, "")}
	exit1 := func(body string) string { return body + "\nexit 1" }
	slow := func(body string) string { return "sleep 10\n" + body }
	args := map[string]string{"gh": "auth status --json hosts", "glab": "auth status --all", "tea": "logins list --output json"}
	bins := map[string]*string{"gh": &ghBin, "glab": &glabBin, "tea": &teaBin}

	dir := t.TempDir()
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	full := map[string]string{
		"github.com": "gh", "ghe.example.com": "gh",
		"gitlab.com": "glab", "gitlab.example.com": "glab",
		"git.example.org": "tea",
	}
	for _, tc := range []struct {
		name    string
		bodies  map[string]string // the fake body of a CLI; a CLI not in it prints its output
		timeout bool
		want    map[string]string
	}{
		{name: "all work", want: full},
		{
			name:   "gh exits 1",
			bodies: map[string]string{"gh": exit1(ok["gh"])},
			want:   map[string]string{"ghe.example.com": "glab", "gitlab.com": "glab", "gitlab.example.com": "glab", "git.example.org": "tea"},
		},
		{
			name:   "glab exits 1",
			bodies: map[string]string{"glab": exit1(ok["glab"])},
			want:   map[string]string{"github.com": "gh", "ghe.example.com": "gh", "gitlab.example.com": "tea", "git.example.org": "tea"},
		},
		{
			name:   "tea exits 1",
			bodies: map[string]string{"tea": exit1(ok["tea"])},
			want:   map[string]string{"github.com": "gh", "ghe.example.com": "gh", "gitlab.com": "glab", "gitlab.example.com": "glab"},
		},
		// cliTimeout also cuts a quick fake under -race, so the other two CLIs fail at once here.
		{
			name:    "gh times out",
			bodies:  map[string]string{"gh": slow(ok["gh"]), "glab": "exit 1", "tea": "exit 1"},
			timeout: true,
			want:    map[string]string{},
		},
		{
			name:    "glab times out",
			bodies:  map[string]string{"gh": "exit 1", "glab": slow(ok["glab"]), "tea": "exit 1"},
			timeout: true,
			want:    map[string]string{},
		},
		{
			name:    "tea times out",
			bodies:  map[string]string{"gh": "exit 1", "glab": "exit 1", "tea": slow(ok["tea"])},
			timeout: true,
			want:    map[string]string{},
		},
		{
			name:   "all fail",
			bodies: map[string]string{"gh": exit1(ok["gh"]), "glab": exit1(ok["glab"]), "tea": exit1(ok["tea"])},
			want:   map[string]string{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.timeout {
				old := cliTimeout
				cliTimeout = 200 * time.Millisecond
				t.Cleanup(func() { cliTimeout = old })
			}
			logs := map[string]string{}
			for cli, bin := range bins {
				body, found := tc.bodies[cli]
				if !found {
					body = ok[cli]
				}
				logs[cli] = fakeCLI(t, bin, "pwd -P >> \"$log\"\n"+body)
			}
			start := time.Now()
			got := loginHosts(dir)
			if d := time.Since(start); tc.timeout && d > 4*time.Second {
				t.Errorf("loginHosts took %v, want at most 4s", d)
			}
			if got == nil {
				t.Error("loginHosts = nil, want a map")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("loginHosts = %v, want %v", got, tc.want)
			}
			if tc.timeout {
				return // a fake cut by the 200 ms timeout can stop before it logs
			}
			for cli, log := range logs {
				want := args[cli] + "\n" + realDir + "\n"
				if data, _ := os.ReadFile(log); string(data) != want {
					t.Errorf("%s log = %q, want %q", cli, data, want)
				}
			}
		})
	}
}

// hostKindFixture sets the fakes of gh (ghe.example.com), glab (gitlab.example.com), and tea
// (git.example.org), and the token variable of git.example.net. It returns the hostKinds map
// of the hostKind tests and the log files of the three fakes.
func hostKindFixture(t *testing.T) (hostKinds map[string]string, logs []string) {
	t.Helper()
	logs = []string{
		fakeCLI(t, &ghBin, `printf '%s\n' '{"hosts":{"ghe.example.com":[{"state":"success","active":true}]}}'`),
		fakeCLI(t, &glabBin, `printf 'gitlab.example.com\n  ✓ Logged in to gitlab.example.com as u\n' >&2`),
		fakeCLI(t, &teaBin, `printf '%s\n' '[{"name":"a","url":"https://git.example.org"}]'`),
	}
	t.Setenv("BRUH_GITEA_TOKEN_GIT_EXAMPLE_NET", "x")
	hostKinds = map[string]string{"gitlab.com": "gitea", "ghe.example.com": "gitlab", "other.example.com": "gitea"}
	return hostKinds, logs
}

func TestHostKindOrder(t *testing.T) {
	hostKinds, _ := hostKindFixture(t)
	logins := loginHosts(t.TempDir())
	for host, want := range map[string]string{
		"github.com":         "github", // the exact list wins over hostKinds
		"gitlab.com":         "gitlab",
		"ghe.example.com":    "github", // a CLI wins over hostKinds
		"gitlab.example.com": "gitlab",
		"git.example.org":    "gitea",
		"git.example.net":    "gitea", // the token variable
		"other.example.com":  "gitea", // hostKinds
		"none.example.com":   "unknown",
	} {
		if got := hostKind(host, hostKinds, logins); got != want {
			t.Errorf("hostKind(%q, hostKinds, logins) = %q, want %q", host, got, want)
		}
	}
}

func TestHostKindWithoutCLIs(t *testing.T) {
	hostKinds, logs := hostKindFixture(t)
	for host, want := range map[string]string{
		"github.com":         "github",
		"gitlab.com":         "gitlab",
		"ghe.example.com":    "gitlab", // from hostKinds
		"gitlab.example.com": "unknown",
		"git.example.org":    "unknown",
		"git.example.net":    "unknown", // the token variable counts only with logins
		"other.example.com":  "gitea",
		"none.example.com":   "unknown",
	} {
		if got := hostKind(host, hostKinds, nil); got != want {
			t.Errorf("hostKind(%q, hostKinds, nil) = %q, want %q", host, got, want)
		}
	}
	for _, log := range logs {
		if data, _ := os.ReadFile(log); len(data) != 0 {
			t.Errorf("fake log %s = %q, want empty", log, data)
		}
	}
}

func TestAPIRoots(t *testing.T) {
	for _, tc := range []struct{ host, kind, want string }{
		{"github.com", "github", "https://api.github.com"},
		{"ghe.example.com", "github", "https://ghe.example.com/api/v3"},
		{"git.example.org", "gitea", "https://git.example.org/api/v1"},
		{"gitlab.com", "gitlab", "https://gitlab.com/api/v4"},
		{"x.example.com", "unknown", ""},
	} {
		if got := apiRoot(tc.host, tc.kind); got != tc.want {
			t.Errorf("apiRoot(%q, %q) = %q, want %q", tc.host, tc.kind, got, tc.want)
		}
	}
}

// writeRepo makes the repository root/rel with config as .git/config (no file when config is
// "") and, when head is not "", head as .git/refs/remotes/<remote>/HEAD.
func writeRepo(t *testing.T, root, rel, config, remote, head string) {
	t.Helper()
	gitDir := filepath.Join(root, filepath.FromSlash(rel), ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "remotes", remote), 0o755); err != nil {
		t.Fatal(err)
	}
	if config != "" {
		if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if head != "" {
		if err := os.WriteFile(filepath.Join(gitDir, "refs", "remotes", remote, "HEAD"), []byte(head), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// originConfig is a .git/config with the one remote origin at url.
func originConfig(url string) string {
	return "[core]\n\tbare = false\n[remote \"origin\"]\n\turl = " + url + "\n\tfetch = +refs/heads/*:refs/remotes/origin/*\n"
}

func TestRepoFacts(t *testing.T) {
	git := func(host string) hostValue { return hostValue{Value: &host, Source: "git"} }
	null := hostValue{Value: nil, Source: "git"}
	tests := []struct {
		name   string
		config string // "" for no .git/config
		head   string // .git/refs/remotes/<want.Remote>/HEAD, "" for no file
		kinds  map[string]string
		want   indexRepo
	}{
		{
			name:   "gitlab.com",
			config: originConfig("https://gitlab.com/team/shop.git"),
			head:   "ref: refs/remotes/origin/main\n",
			want: indexRepo{
				Remotes: []remote{{"origin", "https://gitlab.com/team/shop.git"}}, Remote: "origin",
				Host: git("gitlab.com"), Kind: "gitlab", APIURL: "https://gitlab.com/api/v4",
				HostPath: "team/shop", DefaultBranch: "main",
			},
		},
		{
			name:   "gitea from host_kinds",
			config: originConfig("https://git.example.org/team/auth"),
			head:   "ref: refs/remotes/origin/trunk\n",
			kinds:  map[string]string{"git.example.org": "gitea"},
			want: indexRepo{
				Remotes: []remote{{"origin", "https://git.example.org/team/auth"}}, Remote: "origin",
				Host: git("git.example.org"), Kind: "gitea", APIURL: "https://git.example.org/api/v1",
				HostPath: "team/auth", DefaultBranch: "trunk",
			},
		},
		{
			// The scp form with a host of the exact list: no alias and no ssh call.
			name:   "github.com scp form",
			config: originConfig("git@github.com:team/app.git"),
			want: indexRepo{
				Remotes: []remote{{"origin", "git@github.com:team/app.git"}}, Remote: "origin",
				Host: git("github.com"), Kind: "github", APIURL: "https://api.github.com",
				HostPath: "team/app", DefaultBranch: "unknown",
			},
		},
		{
			// Remotes is sorted by name; Remote is chosen in the file order.
			name: "no origin",
			config: "[remote \"zeta\"]\n\turl = https://gitlab.com/team/z.git\n" +
				"[remote \"alpha\"]\n\turl = https://git.example.org/team/a.git\n",
			head: "ref: refs/remotes/zeta/dev\n",
			want: indexRepo{
				Remotes: []remote{{"alpha", "https://git.example.org/team/a.git"}, {"zeta", "https://gitlab.com/team/z.git"}},
				Remote:  "zeta", Host: git("gitlab.com"), Kind: "gitlab", APIURL: "https://gitlab.com/api/v4",
				HostPath: "team/z", DefaultBranch: "dev",
			},
		},
		// Entry B1 of decisions.md: the null and unknown values of the schema.
		{
			name:   "no remote",
			config: "[core]\n\tbare = false\n",
			want:   indexRepo{Remotes: []remote{}, Host: null, Kind: "unknown", DefaultBranch: "unknown"},
		},
		{
			name: "no config",
			want: indexRepo{Remotes: []remote{}, Host: null, Kind: "unknown", DefaultBranch: "unknown"},
		},
		{
			name:   "local path remote",
			config: originConfig("../upstream/shop.git"),
			want: indexRepo{
				Remotes: []remote{{"origin", "../upstream/shop.git"}}, Remote: "origin",
				Host: null, Kind: "unknown", DefaultBranch: "unknown",
			},
		},
		{
			name:   "file remote",
			config: originConfig("file:///srv/git/shop.git"),
			want: indexRepo{
				Remotes: []remote{{"origin", "file:///srv/git/shop.git"}}, Remote: "origin",
				Host: null, Kind: "unknown", DefaultBranch: "unknown",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := fakeCLI(t, &sshBin, `printf 'hostname gitlab.com\n'`)
			root := t.TempDir()
			writeRepo(t, root, "team/shop", tt.config, cmp.Or(tt.want.Remote, "origin"), tt.head)
			f := factCtx{root: root, dir: t.TempDir(), kinds: tt.kinds, ssh: true}
			r, alias, sshHost, err := f.facts("team/shop")
			if err != nil {
				t.Fatalf("facts: %v", err)
			}
			want := tt.want
			want.Path, want.State = "team/shop", "present"
			if !reflect.DeepEqual(r, want) {
				t.Errorf("facts = %s, want %s", jsonText(t, r), jsonText(t, want))
			}
			if alias != "" || sshHost != "" {
				t.Errorf("facts alias, sshHost = %q, %q, want empty", alias, sshHost)
			}
			if data, _ := os.ReadFile(log); len(data) != 0 {
				t.Errorf("ssh log = %q, want empty", data)
			}
		})
	}
}

// jsonText returns v as JSON, for the messages of the facts tests.
func jsonText(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSSHAliasResolves(t *testing.T) {
	for _, url := range []string{"git@gitlab.com-work:team/shop.git", "ssh://git@gitlab.com-work/team/shop.git"} {
		t.Run(url, func(t *testing.T) {
			log := fakeCLI(t, &sshBin, `printf 'user git\nhostname gitlab.com\nport 22\n'`)
			root := t.TempDir()
			writeRepo(t, root, "shop", originConfig(url), "origin", "ref: refs/remotes/origin/main\n")
			// ssh -G wins over host_aliases.
			f := factCtx{root: root, dir: t.TempDir(), aliases: map[string]string{"gitlab.com-work": "git.example.org"}, ssh: true}
			r, alias, sshHost, err := f.facts("shop")
			if err != nil {
				t.Fatalf("facts: %v", err)
			}
			host := "gitlab.com"
			want := indexRepo{
				Path: "shop", Remotes: []remote{{"origin", url}}, Remote: "origin",
				Host: hostValue{Value: &host, Source: "git"}, Kind: "gitlab", APIURL: "https://gitlab.com/api/v4",
				HostPath: "team/shop", DefaultBranch: "main", State: "present",
			}
			if !reflect.DeepEqual(r, want) {
				t.Errorf("facts = %s, want %s", jsonText(t, r), jsonText(t, want))
			}
			if alias != "gitlab.com-work" || sshHost != "gitlab.com" {
				t.Errorf("facts alias, sshHost = %q, %q, want %q, %q", alias, sshHost, "gitlab.com-work", "gitlab.com")
			}
			if data, _ := os.ReadFile(log); string(data) != "-G -- gitlab.com-work\n" {
				t.Errorf("ssh log = %q, want %q", data, "-G -- gitlab.com-work\n")
			}
		})
	}
}

func TestSSHAliasUnresolved(t *testing.T) {
	const (
		echoAlias = `printf 'hostname gitlab.com-work\n'`
		call      = "-G -- gitlab.com-work\n"
		workURL   = "git@gitlab.com-work:team/shop.git"
	)
	named := map[string]string{"gitlab.com-work": "gitlab.com"}
	tests := []struct {
		name     string
		body     string // the fake ssh
		url      string
		aliases  map[string]string
		kinds    map[string]string
		ssh      bool
		wantHost *string
		wantKind string
		wantAPI  string
		wantPath string
		wantLog  string
		alias    string
	}{
		{name: "alias in host_aliases", body: echoAlias, url: workURL, aliases: named, ssh: true,
			wantHost: new("gitlab.com"), wantKind: "gitlab", wantAPI: "https://gitlab.com/api/v4", wantPath: "team/shop", wantLog: call, alias: "gitlab.com-work"},
		{name: "ssh fails", body: "printf 'hostname gitlab.com\\n'; exit 1", url: workURL, aliases: named, ssh: true,
			wantHost: new("gitlab.com"), wantKind: "gitlab", wantAPI: "https://gitlab.com/api/v4", wantPath: "team/shop", wantLog: call, alias: "gitlab.com-work"},
		{name: "alias not in host_aliases", body: echoAlias, url: workURL, ssh: true,
			wantKind: "unknown", wantPath: "team/shop", wantLog: call, alias: "gitlab.com-work"},
		{name: "ssh off, alias in host_aliases", body: echoAlias, url: workURL, aliases: named,
			wantHost: new("gitlab.com"), wantKind: "gitlab", wantAPI: "https://gitlab.com/api/v4", wantPath: "team/shop", alias: "gitlab.com-work"},
		{name: "ssh off, alias not in host_aliases", body: echoAlias, url: workURL,
			wantKind: "unknown", wantPath: "team/shop", alias: "gitlab.com-work"},
		// Build spec A3.1: an alias with other characters is the host, and its kind is unknown
		// even when host_kinds names it.
		{name: "bad characters", body: echoAlias, url: "git@x;y:team/r.git", kinds: map[string]string{"x;y": "gitea"}, ssh: true,
			wantHost: new("x;y"), wantKind: "unknown", wantPath: "team/r", alias: "x;y"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := fakeCLI(t, &sshBin, tt.body)
			root := t.TempDir()
			writeRepo(t, root, "shop", originConfig(tt.url), "origin", "ref: refs/remotes/origin/main\n")
			f := factCtx{root: root, dir: t.TempDir(), aliases: tt.aliases, kinds: tt.kinds, ssh: tt.ssh}
			r, alias, sshHost, err := f.facts("shop")
			if err != nil {
				t.Fatalf("facts: %v", err)
			}
			want := indexRepo{
				Path: "shop", Remotes: []remote{{"origin", tt.url}}, Remote: "origin",
				Host: hostValue{Value: tt.wantHost, Source: "git"}, Kind: tt.wantKind, APIURL: tt.wantAPI,
				HostPath: tt.wantPath, DefaultBranch: "main", State: "present",
			}
			if !reflect.DeepEqual(r, want) {
				t.Errorf("facts = %s, want %s", jsonText(t, r), jsonText(t, want))
			}
			if alias != tt.alias || sshHost != "" {
				t.Errorf("facts alias, sshHost = %q, %q, want %q, \"\"", alias, sshHost, tt.alias)
			}
			if data, _ := os.ReadFile(log); string(data) != tt.wantLog {
				t.Errorf("ssh log = %q, want %q", data, tt.wantLog)
			}
		})
	}
}

// activityRepo returns a repository of the activity tests; an empty host gives the null host.
func activityRepo(host, kind, hostPath, apiURL string) indexRepo {
	r := indexRepo{Path: hostPath, Kind: kind, APIURL: apiURL, HostPath: hostPath, Host: hostValue{Source: "git"}, State: "present"}
	if host != "" {
		r.Host.Value = &host
	}
	return r
}

func TestActivityCallsOnlyLoggedInHosts(t *testing.T) {
	// Each fake answers 2026-10-01T12:00:00+02:00 by default; the result is in UTC.
	const at = "2026-10-01T12:00:00+02:00"
	okAt := activityResult{At: "2026-10-01T10:00:00Z", State: "ok"}
	noLogin := activityResult{At: "", State: "no login"}
	unknown := activityResult{At: "", State: "unknown"}
	ghOK := `printf '%s\n' '{"pushed_at":"` + at + `"}'`
	glabOK := `printf '%s\n' '{"last_activity_at":"` + at + `"}'`

	var mu sync.Mutex
	var requests []string // "<path> <Authorization header>" of each request to the Gitea server
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Path+" "+r.Header.Get("Authorization"))
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/repos/team/shop":
			fmt.Fprint(w, `{"updated_at":"`+at+`"}`)
		case "/api/v1/repos/team/broken":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := hostHTTP
	hostHTTP = srv.Client() // trusts the test certificate of the server
	t.Cleanup(func() { hostHTTP = old })
	api := srv.URL + "/api/v1"
	t.Setenv("BRUH_GITEA_TOKEN_GIT_EXAMPLE_NET", "tok-net")
	t.Setenv("BRUH_GITEA_TOKEN_GIT_EXAMPLE_ORG", "") // git.example.org has only a tea login

	for _, tc := range []struct {
		name      string
		repos     []indexRepo
		logins    map[string]string
		gh, glab  string // the fake bodies; "" prints the default time
		timeout   bool   // cliTimeout is 200 milliseconds
		want      []activityResult
		ghCalls   []string // the exact arguments of each gh call
		glabCalls []string // the end of the arguments of each glab call
		requests  []string
	}{
		{
			name:    "github with a gh login",
			repos:   []indexRepo{activityRepo("github.com", "github", "owner/shop", "https://api.github.com")},
			logins:  map[string]string{"github.com": "gh"},
			want:    []activityResult{okAt},
			ghCalls: []string{"api --hostname github.com repos/owner/shop"},
		},
		{
			name:      "gitlab with a glab login",
			repos:     []indexRepo{activityRepo("gitlab.example.com", "gitlab", "group/sub/shop", "https://gitlab.example.com/api/v4")},
			logins:    map[string]string{"gitlab.example.com": "glab"},
			want:      []activityResult{okAt},
			glabCalls: []string{"projects/group%2Fsub%2Fshop"},
		},
		{
			name: "no login",
			repos: []indexRepo{
				activityRepo("ghe.example.com", "github", "owner/shop", "https://ghe.example.com/api/v3"),
				activityRepo("gitlab.com", "gitlab", "group/shop", "https://gitlab.com/api/v4"),
				activityRepo("", "unknown", "", ""),
				activityRepo("code.example.com", "unknown", "owner/shop", ""),
			},
			logins: map[string]string{"code.example.com": "gh", "github.com": "gh", "gitlab.example.com": "glab"},
			want:   []activityResult{noLogin, noLogin, noLogin, noLogin},
		},
		{
			name: "gitea with a token, and gitea with only a tea login",
			repos: []indexRepo{
				activityRepo("git.example.net", "gitea", "team/shop", api),
				activityRepo("git.example.org", "gitea", "team/shop", api),
			},
			logins:   map[string]string{"git.example.org": "tea"},
			want:     []activityResult{okAt, noLogin},
			requests: []string{"/api/v1/repos/team/shop token tok-net"},
		},
		{
			name:    "gh exits 1",
			repos:   []indexRepo{activityRepo("github.com", "github", "owner/shop", "https://api.github.com")},
			logins:  map[string]string{"github.com": "gh"},
			gh:      ghOK + "\nexit 1",
			want:    []activityResult{unknown},
			ghCalls: []string{"api --hostname github.com repos/owner/shop"},
		},
		{
			name:    "glab times out",
			repos:   []indexRepo{activityRepo("gitlab.example.com", "gitlab", "group/shop", "https://gitlab.example.com/api/v4")},
			logins:  map[string]string{"gitlab.example.com": "glab"},
			glab:    "sleep 5\n" + glabOK,
			timeout: true,
			want:    []activityResult{unknown},
		},
		{
			name: "bad answers do not stop the other calls",
			repos: []indexRepo{
				activityRepo("github.com", "github", "owner/nojson", "https://api.github.com"),
				activityRepo("github.com", "github", "owner/empty", "https://api.github.com"),
				activityRepo("github.com", "github", "owner/badtime", "https://api.github.com"),
				activityRepo("git.example.net", "gitea", "team/broken", api),
				activityRepo("github.com", "github", "owner/shop", "https://api.github.com"),
			},
			logins: map[string]string{"github.com": "gh"},
			gh: `case $4 in
repos/owner/nojson) echo 'not json' ;;
repos/owner/empty) printf '%s\n' '{"pushed_at":""}' ;;
repos/owner/badtime) printf '%s\n' '{"pushed_at":"yesterday"}' ;;
*) ` + ghOK + ` ;;
esac`,
			want: []activityResult{unknown, unknown, unknown, unknown, okAt},
			ghCalls: []string{
				"api --hostname github.com repos/owner/nojson",
				"api --hostname github.com repos/owner/empty",
				"api --hostname github.com repos/owner/badtime",
				"api --hostname github.com repos/owner/shop",
			},
			requests: []string{"/api/v1/repos/team/broken token tok-net"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.timeout {
				old := cliTimeout
				cliTimeout = 200 * time.Millisecond
				t.Cleanup(func() { cliTimeout = old })
			}
			gh, glab := cmp.Or(tc.gh, ghOK), cmp.Or(tc.glab, glabOK)
			ghLog, glabLog := fakeCLI(t, &ghBin, gh), fakeCLI(t, &glabBin, glab)
			mu.Lock()
			requests = nil
			mu.Unlock()

			start := time.Now()
			got := activity(tc.repos, tc.logins, t.TempDir())
			if d := time.Since(start); d > 4*time.Second {
				t.Errorf("activity took %v, want at most 4s", d)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("activity = %+v, want %+v", got, tc.want)
			}
			mu.Lock()
			if !slices.Equal(requests, tc.requests) {
				t.Errorf("Gitea requests = %q, want %q", requests, tc.requests)
			}
			mu.Unlock()
			if tc.timeout {
				return // a fake cut by the 200 ms timeout can stop before it logs
			}
			calls := func(log string) []string { // the lines of a log, sorted: the calls run in parallel
				if lines := logLines(t, log); lines[0] != "" {
					return slices.Sorted(slices.Values(lines))
				}
				return nil
			}
			ghWant := slices.Sorted(slices.Values(tc.ghCalls))
			if ghGot := calls(ghLog); !slices.Equal(ghGot, ghWant) {
				t.Errorf("gh calls = %q, want %q", ghGot, ghWant)
			}
			glabGot := calls(glabLog)
			if len(glabGot) != len(tc.glabCalls) {
				t.Fatalf("glab calls = %q, want %d calls that end with %q", glabGot, len(tc.glabCalls), tc.glabCalls)
			}
			for i, end := range tc.glabCalls {
				if !strings.HasSuffix(glabGot[i], end) {
					t.Errorf("glab call = %q, want an end of %q", glabGot[i], end)
				}
			}
		})
	}
}

func TestActivityCap8(t *testing.T) {
	shared := t.TempDir()
	counts := filepath.Join(t.TempDir(), "counts")
	fakeCLI(t, &ghBin, `touch `+shq(shared)+`/$$
ls `+shq(shared)+` | wc -l >> `+shq(counts)+`
sleep 0.2
rm `+shq(shared)+`/$$
printf '%s\n' '{"pushed_at":"2026-10-01T10:00:00Z"}'`)
	var repos []indexRepo
	for i := range 20 {
		repos = append(repos, activityRepo("github.com", "github", fmt.Sprintf("owner/shop-%d", i), "https://api.github.com"))
	}

	got := activity(repos, map[string]string{"github.com": "gh"}, t.TempDir())

	for i, r := range got {
		if r != (activityResult{At: "2026-10-01T10:00:00Z", State: "ok"}) {
			t.Errorf("activity[%d] = %+v, want ok at 2026-10-01T10:00:00Z", i, r)
		}
	}
	if len(got) != len(repos) {
		t.Errorf("activity gave %d results, want %d", len(got), len(repos))
	}
	lines := logLines(t, counts) // a missing log fails here
	if len(lines) != len(repos) {
		t.Errorf("count log has %d lines, want %d", len(lines), len(repos))
	}
	most := 0
	for _, l := range lines {
		n, err := strconv.Atoi(strings.TrimSpace(l))
		if err != nil {
			t.Fatalf("count log line %q: %v", l, err)
		}
		most = max(most, n)
	}
	if most > 8 {
		t.Errorf("at most %d calls ran at the same time, want at most 8", most)
	}
}
