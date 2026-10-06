package main

import (
	"strings"
	"testing"
)

func TestParseRoleKey(t *testing.T) {
	for _, tc := range []struct {
		key    string
		want   RoleKey
		parent string
	}{
		{"bigm", RoleKey{Role: "bigm"}, ""},
		{"clerk-ledger", RoleKey{Role: "ledger"}, "bigm"},
		{"clanker-my-app", RoleKey{Role: "clanker", Project: "my-app"}, "bigm"},
		{"clerk-my-app-t1", RoleKey{Role: "clerk", Project: "my-app", Task: "t1"}, "clanker-my-app"},
		{"clerk-eng123-fix", RoleKey{Role: "clerk", Project: "eng123", Task: "fix"}, "clanker-eng123"},
		{"clanker-ledger", RoleKey{Role: "clanker", Project: "ledger"}, "bigm"},
		{"clerk-ledger-t1", RoleKey{Role: "clerk", Project: "ledger", Task: "t1"}, "clanker-ledger"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			k, err := ParseRoleKey(tc.key)
			if err != nil || k != tc.want {
				t.Errorf("ParseRoleKey(%q) = %+v, %v; want %+v", tc.key, k, err, tc.want)
			}
			if k.String() != tc.key {
				t.Errorf("String() = %q, want %q", k.String(), tc.key)
			}
			if got := k.Parent(); got != tc.parent {
				t.Errorf("%s.Parent() = %q, want %q", tc.key, got, tc.parent)
			}
		})
	}
	bad := []string{"", "Bigm", "bigm2", "clanker-", "clanker--a", "clanker-a-", "clerk-a", "clerk--x", "clerk-a-b-", "clerk-a-B",
		"worker-a", "../x", "clanker-a/b", "clanker-" + strings.Repeat("a", 57)}
	for _, s := range bad {
		if k, err := ParseRoleKey(s); err == nil {
			t.Errorf("ParseRoleKey(%q) = %+v, want error", s, k)
		}
	}
}

// EnvFromOS sets Host to hostSlug(os.Hostname()) (env.go); each slug must match projectRE,
// because the host is a part of question IDs.
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
		got := hostSlug(name)
		if got != want {
			t.Errorf("hostSlug(%q) = %q, want %q", name, got, want)
		}
		if !projectRE.MatchString(got) {
			t.Errorf("hostSlug(%q) = %q, want a match of %s", name, got, projectRE)
		}
	}
}
