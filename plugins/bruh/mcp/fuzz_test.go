package main

import (
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
