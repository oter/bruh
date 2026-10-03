package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzParseRoleKey checks that every accepted key prints back as itself, stays in
// the length limit, and has a parent that is a valid key too.
func FuzzParseRoleKey(f *testing.F) {
	for _, s := range []string{"bigm", "clerk-ledger", "clanker-my-app", "clerk-my-app-t1", "clerk-my-app-merge", "clerk-my", "clanker-", "clerk--x", "Clanker-a", strings.Repeat("a", 65)} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		k, err := ParseRoleKey(s)
		if err != nil {
			return
		}
		if got := k.String(); got != s {
			t.Fatalf("ParseRoleKey(%q).String() = %q", s, got)
		}
		if len(s) > 64 {
			t.Fatalf("accepted a key of %d bytes: %q", len(s), s)
		}
		if p := k.Parent(); p != "" {
			if _, err := ParseRoleKey(p); err != nil {
				t.Fatalf("the parent %q of %q is not a valid key: %v", p, s, err)
			}
		} else if k.Role != "bigm" {
			t.Fatalf("%q has no parent", s)
		}
	})
}

// FuzzMailAllowed checks the sender policy of mail_post: only bigm sends a RULE, and
// every other accepted message is on an edge of the role tree or a P0 of a clerk to bigm.
func FuzzMailAllowed(f *testing.F) {
	for _, c := range [][3]string{
		{"bigm", "clanker-a", "RULE R-1: x"},
		{"clanker-a", "clerk-a-1", "ANSWER Q-a-host-1: x"},
		{"clerk-a-1", "clanker-a", "P1 Q-a-host-2: x"},
		{"clerk-a-1", "bigm", "P0 Q-a-host-3: x"},
		{"clerk-a-1", "clanker-b", "RULE R-99: x"},
		{"clerk-a-1", "clerk-b-1", "ANSWER Q-a-host-5: x"},
	} {
		f.Add(c[0], c[1], c[2])
	}
	f.Fuzz(func(t *testing.T, from, to, header string) {
		fk, errFrom := ParseRoleKey(from)
		tk, errTo := ParseRoleKey(to)
		if errFrom != nil || errTo != nil {
			return
		}
		if mailAllowed(from, to, header) != nil {
			return
		}
		if strings.HasPrefix(header, "RULE ") && from != "bigm" {
			t.Fatalf("%s may post a RULE to %s", from, to)
		}
		edge := from == "bigm" || tk.Parent() == from || fk.Parent() == to ||
			(fk.Role == "clerk" && to == "bigm" && strings.HasPrefix(header, "P0 "))
		if !edge {
			t.Fatalf("%s may post %q to %s off the role tree", from, header, to)
		}
	})
}

// FuzzValidateHandoff checks that an accepted handoff is not empty and stays below
// the size cap.
func FuzzValidateHandoff(f *testing.F) {
	f.Add("## Goal\n- ship plan 2\n")
	f.Add("- see docs/spec.md\n")
	f.Add("- wait for a0123456789abcdef\n")
	f.Add("- notes in /tmp/x\n")
	f.Fuzz(func(t *testing.T, text string) {
		if validateHandoff(text) != nil {
			return
		}
		if text == "" {
			t.Fatal("accepted an empty handoff")
		}
		if n := utf8.RuneCountInString(text); n > handoffMax {
			t.Fatalf("accepted a handoff of %d characters", n)
		}
	})
}

// FuzzCheckFills checks that checkFills does not panic and that an accepted fill value
// follows the value rules of spec 8.5.
func FuzzCheckFills(f *testing.F) {
	root := f.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "shop"), 0o755); err != nil {
		f.Fatalf("os.MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "shop", "README.md"), []byte("doc\n"), 0o644); err != nil {
		f.Fatalf("os.WriteFile() error: %v", err)
	}
	for _, c := range []fill{
		{Field: "purpose", Value: "Online shop", Source: "agent"},
		{Field: "link", Value: "auth", Source: "agent", Repo: "shop", File: "go.mod", Line: 5},
		{Field: "doc", Value: "README.md", Source: "owner", Repo: "shop"},
		{Field: "host", Value: "gitlab.com", Source: "owner", Repo: "shop-app"},
		{Field: "purpose", Value: "line one\nline two", Source: "agent"},
		{Field: "purpose", Value: "shop | auth", Source: "agent"},
		{Field: "doc", Value: "../shop-app/README.md", Source: "owner", Repo: "shop"},
		{Field: "host", Value: "GitLab.com", Source: "owner", Repo: "shop-app"},
	} {
		f.Add(c.Field, c.Value, c.Source, c.Repo, c.File, c.Line)
	}
	hostPattern := regexp.MustCompile(`^[a-z0-9.-]+$`)
	f.Fuzz(func(t *testing.T, field, value, source, repo, file string, line int) {
		fl := fill{Field: field, Value: value, Source: source, Repo: repo, File: file, Line: line}
		project := answerProject{Key: "shop", Repos: []string{"shop", "shop-app"}, Main: "shop", Fills: []fill{fl}}
		if checkFills(root, project, []string{"shop", "auth"}) != nil {
			return
		}
		if strings.ContainsAny(value, "\r\n") || strings.Contains(value, " | ") {
			t.Fatalf("accepted %+v with a line break or \" | \"", fl)
		}
		switch field {
		case "host":
			if !hostPattern.MatchString(value) {
				t.Fatalf("accepted the host %q", value)
			}
		case "purpose":
			if n := utf8.RuneCountInString(value); n > 120 {
				t.Fatalf("accepted a purpose of %d characters", n)
			}
		case "link":
			if value != "auth" {
				t.Fatalf("accepted the link %q", value)
			}
		}
	})
}

// FuzzGitConfigRemotes checks that readRemotes does not panic on any .git/config, that
// each remote has a name, and that no http or https URL keeps user information (C1).
func FuzzGitConfigRemotes(f *testing.F) {
	f.Add("[remote \"origin\"]\n\turl = https://oauth2:token@gitlab.com/g/r.git\n")
	f.Add("# remotes\n[remote \"origin\"]\n\turl = git@gitlab.com:g/r.git\n; mirror\n[remote \"backup\"]\n\turl=https://github.com/g/r.git\n")
	f.Add("[core]\n\tfsmonitor = touch y\n")
	f.Add("")
	f.Add("[remote \"x\n\turl = https://u@h/x\n")
	f.Add("[remote \"q\"]\n\turl = \"http://u:p@h/x\"\n")
	f.Add("[remote \"\"]\n\turl = https://h/x\n")
	f.Fuzz(func(t *testing.T, config string) {
		repo := t.TempDir()
		if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatalf("os.Mkdir() error: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, ".git", "config"), []byte(config), 0o644); err != nil {
			t.Fatalf("os.WriteFile() error: %v", err)
		}
		rs, err := readRemotes(repo)
		if err != nil {
			t.Fatalf("readRemotes(%q) error: %v", config, err)
		}
		for _, r := range rs {
			if r.Name == "" {
				t.Errorf("readRemotes(%q) returned a remote with no name: %+v", config, r)
			}
			lower := strings.ToLower(r.URL)
			if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
				continue
			}
			u, err := url.Parse(r.URL)
			if err != nil {
				t.Fatalf("url.Parse(%q) error: %v", r.URL, err)
			}
			if u.User != nil {
				t.Errorf("readRemotes(%q) kept the user information of %q", config, r.URL)
			}
		}
	})
}

// FuzzQuestionID checks that each ID that qidRE accepts starts with Q-, has only
// [a-z0-9-] after it, and ends with a dash and digits (decision D2).
func FuzzQuestionID(f *testing.F) {
	for _, s := range []string{"Q-a-host-1", "Q-7", "Q-my-app-mac-12", "Q--x-1", "Q-a-1", "Q-A-host-1"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !qidRE.MatchString(s) {
			return
		}
		rest, ok := strings.CutPrefix(s, "Q-")
		if !ok {
			t.Fatalf("qidRE accepted %q with no Q- prefix", s)
		}
		if strings.Trim(rest, "abcdefghijklmnopqrstuvwxyz0123456789-") != "" {
			t.Fatalf("qidRE accepted %q with a character outside [a-z0-9-]", s)
		}
		i := strings.LastIndexByte(rest, '-')
		if i < 0 || i == len(rest)-1 || strings.Trim(rest[i+1:], "0123456789") != "" {
			t.Fatalf("qidRE accepted %q with no -<digits> at the end", s)
		}
	})
}

// FuzzAllowRule checks that checkAllowRules does not panic and accepts a rule only when it is
// Read(//<path>/**) with <path> the root of learn/tree.json joined with shop or shop-app,
// without its first slash (build spec A15.1, G33).
func FuzzAllowRule(f *testing.F) {
	env := Env{DataDir: f.TempDir()}
	ledger := f.TempDir()
	root := filepath.Join(f.TempDir(), "root")
	cfg, err := json.Marshal(initConfig{LedgerPath: ledger})
	if err != nil {
		f.Fatalf("json.Marshal(initConfig) error: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(env.DataDir, "init"), 0o700); err != nil {
		f.Fatalf("os.MkdirAll() error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(env.DataDir, "init", "config.json"), cfg, 0o600); err != nil {
		f.Fatalf("os.WriteFile(config.json) error: %v", err)
	}
	tree := &treeFile{Root: root, Projects: []treeProject{{Key: "shop"}}}
	if _, err := writeLearnFile(filepath.Join(ledger, "learn", "tree.json"), tree); err != nil {
		f.Fatalf("writeLearnFile(tree.json) error: %v", err)
	}
	proj := &projectFile{Key: "shop", Main: "shop", Repos: []indexRepo{{Path: "shop"}, {Path: "shop-app"}}}
	if _, err := writeLearnFile(filepath.Join(ledger, "learn", "projects", "shop.json"), proj); err != nil {
		f.Fatalf("writeLearnFile(shop.json) error: %v", err)
	}
	rel := strings.TrimPrefix(root, "/")
	for _, s := range []string{
		"Read(//" + rel + "/shop/**)",
		"Read(//" + rel + "/shop-app/**)",
		"Read(//x/**)",
		"Write(//" + rel + "/shop/**)",
		"Read(/" + rel + "/shop/**)",
		"Read(//" + rel + "/shop/../x/**)",
		"Read(//" + rel + "/shop/*)",
		"",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, rule string) {
		if checkAllowRules(env, "shop", []string{rule}) != nil {
			return
		}
		path, ok := strings.CutPrefix(rule, "Read(//")
		if !ok {
			t.Fatalf("checkAllowRules accepted %q with no Read(// prefix", rule)
		}
		path, ok = strings.CutSuffix(path, "/**)")
		if !ok {
			t.Fatalf("checkAllowRules accepted %q with no /**) suffix", rule)
		}
		if path != rel+"/shop" && path != rel+"/shop-app" {
			t.Fatalf("checkAllowRules accepted %q for the path %q, want %q or %q", rule, path, rel+"/shop", rel+"/shop-app")
		}
	})
}
