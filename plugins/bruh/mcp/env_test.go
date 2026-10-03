package main

import (
	"os"
	"strings"
	"testing"
)

func TestParseRoleKey(t *testing.T) {
	good := map[string]RoleKey{
		"bigm":             {Role: "bigm"},
		"clerk-ledger":     {Role: "ledger"},
		"clanker-my-app":   {Role: "clanker", Project: "my-app"},
		"clerk-my-app-t1":  {Role: "clerk", Project: "my-app", Task: "t1"},
		"clerk-eng123-fix": {Role: "clerk", Project: "eng123", Task: "fix"},
		"clanker-ledger":   {Role: "clanker", Project: "ledger"},
		"clerk-ledger-t1":  {Role: "clerk", Project: "ledger", Task: "t1"},
	}
	for s, want := range good {
		k, err := ParseRoleKey(s)
		if err != nil || k != want {
			t.Errorf("ParseRoleKey(%q) = %+v, %v; want %+v", s, k, err, want)
		}
		if k.String() != s {
			t.Errorf("String() = %q, want %q", k.String(), s)
		}
	}
	bad := []string{"", "Bigm", "bigm2", "clanker-", "clanker--a", "clanker-a-", "clerk-a", "clerk--x", "clerk-a-b-", "clerk-a-B",
		"worker-a", "../x", "clanker-a/b", "clanker-" + strings.Repeat("a", 57)}
	for _, s := range bad {
		if k, err := ParseRoleKey(s); err == nil {
			t.Errorf("ParseRoleKey(%q) = %+v, want error", s, k)
		}
	}
}

func TestRoleKeyParent(t *testing.T) {
	for s, want := range map[string]string{
		"bigm": "", "clerk-ledger": "bigm", "clanker-my-app": "bigm", "clerk-my-app-t1": "clanker-my-app",
	} {
		k, err := ParseRoleKey(s)
		if err != nil {
			t.Fatal(err)
		}
		if got := k.Parent(); got != want {
			t.Errorf("%s.Parent() = %q, want %q", s, got, want)
		}
	}
}

func TestHostNormalize(t *testing.T) {
	for name, want := range map[string]string{
		"My_Mac.local":      "my-mac",
		"Dev-Mac.local":     "dev-mac", // the example of spec 5
		"web01.example.com": "web01",
		"--":                "host",
		"":                  "host",
		".local":            "host",
		"Über-Box":          "ber-box",
	} {
		if got := hostSlug(name); got != want {
			t.Errorf("hostSlug(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestEnvFromOSSetsHost(t *testing.T) {
	name, err := os.Hostname()
	if err != nil {
		t.Fatal(err)
	}
	got := EnvFromOS().Host
	if want := hostSlug(name); got != want {
		t.Errorf("EnvFromOS().Host = %q, want hostSlug(%q) = %q", got, name, want)
	}
	if !projectRE.MatchString(got) {
		t.Errorf("EnvFromOS().Host = %q, want a match of %s", got, projectRE)
	}
}
