package main

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
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
