package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const owedFixture = "# Owed\n\nText above.\n\n## Items\n\n| Owner | Item | Due (UTC) |\n|---|---|---|\n| max | `a` and `b` | none |\n| max | report | 2026-10-01T00:00:00Z |\n\nText below.\n"

// ledgerFixture is a temporary git ledger with files, and an env of bigm whose
// <data>/init/config.json points at it, with a fake clock at 2026-10-04T12:00:00Z. The global and
// system git config are off, so a gpgsign or hooksPath of the user does not apply. A missing git
// binary fails the test.
func ledgerFixture(t *testing.T, files map[string]string) (Env, string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "/dev/null")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	env := testEnv(t, "bigm")
	env.Now = func() time.Time { return time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC) }
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, dir, "init", "-q", "-b", "main")
	gitT(t, dir, "config", "user.name", "Test")
	gitT(t, dir, "config", "user.email", "test@example.com")
	gitT(t, dir, "add", "-A")
	gitT(t, dir, "commit", "-q", "-m", "init")
	if err := os.MkdirAll(filepath.Join(env.DataDir, "init"), 0o700); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(map[string]string{"ledger_path": dir})
	if err := os.WriteFile(filepath.Join(env.DataDir, "init", "config.json"), cfg, 0o600); err != nil {
		t.Fatal(err)
	}
	return env, dir
}

func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return string(out)
}

func readT(t *testing.T, dir, file string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// edit calls ledger_edit and returns the output as a map, or the error.
func edit(t *testing.T, env Env, args map[string]any) (map[string]any, error) {
	t.Helper()
	out, err := call(t, env, "ledger_edit", args)
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

// checkCommit checks the commit subject, the one file of the commit, and the DONE mail. The output
// has no nudge: the waiter of clerk-ledger wakes it (owner, 2026-10-05: "ledger tool must be automatic").
func checkCommit(t *testing.T, env Env, dir string, out map[string]any, subject, file string) {
	t.Helper()
	if got := gitT(t, dir, "log", "-1", "--format=%s"); got != subject+"\n" {
		t.Errorf("subject = %q, want %q", got, subject)
	}
	if got := gitT(t, dir, "show", "--name-only", "--format=", "HEAD"); got != file+"\n" {
		t.Errorf("files of the commit = %q, want %q", got, file)
	}
	sha := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	header := "DONE: ledger commit " + strings.TrimSpace(gitT(t, dir, "rev-parse", "--short", "HEAD"))
	if out["sha"] != sha {
		t.Errorf("sha = %v, want %s", out["sha"], sha)
	}
	if _, ok := out["nudge"]; ok {
		t.Errorf("output has a nudge: %v", out)
	}
	msgs, err := call(t, as(env, "clerk-ledger"), "mail_read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if m := msgs.([]any); len(m) != 1 || m[0].(map[string]any)["from"] != "bigm" || m[0].(map[string]any)["header"] != header ||
		!strings.Contains(m[0].(map[string]any)["body"].(string), sha) {
		t.Errorf("mail of clerk-ledger = %v, want one from bigm with %q", m, header)
	}
}

// checkNothing checks that a refused call changed no file and made no commit.
func checkNothing(t *testing.T, dir, head, file, want string) {
	t.Helper()
	if got := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD")); got != head {
		t.Errorf("HEAD moved: %s", got)
	}
	if got := readT(t, dir, file); got != want {
		t.Errorf("%s changed: %q", file, got)
	}
}

func TestLedgerEditRoundTrip(t *testing.T) {
	mode := "# Mode\n\nmode: human\nchanged: 2026-01-01T00:00:00Z\n\n```text\nchanged: example\n```\n"
	crlf := "# T\r\n\r\n| A | B |\r\n|---|---|\r\n| 1 | 2 |"
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture, "mode.md": mode, "crlf.md": crlf, "other.md": "x\n"})
	// Another dirty, staged file stays out of each commit and stays dirty.
	if err := os.WriteFile(filepath.Join(dir, "other.md"), []byte("y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", "other.md")

	out, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "table": "Items", "kind": "owed",
		"subject": "send the deck", "cells": map[string]string{"Owner": "bigm", "Item": "deck", "Due (UTC)": "now+30m"}})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(owedFixture, "00Z |\n", "00Z |\n| bigm | deck | 2026-10-04T12:30:00Z |\n", 1)
	if got := readT(t, dir, "owed.md"); got != want {
		t.Fatalf("after add:\n%q\nwant\n%q", got, want)
	}
	checkCommit(t, env, dir, out, "add owed: send the deck", "owed.md")
	if got := gitT(t, dir, "log", "-1", "--format=%B"); got != "add owed: send the deck\n\nRecorded (UTC): 2026-10-04T12:00:00Z\n\n" {
		t.Errorf("add body = %q", got)
	}
	if row := out["row"].(map[string]any); row["Item"] != "deck" || row["Due (UTC)"] != "2026-10-04T12:30:00Z" {
		t.Errorf("row = %v", row)
	}

	out, err = edit(t, env, map[string]any{"action": "update", "file": "owed.md", "kind": "owed", "subject": "report to ann",
		"table": "Items", "match": map[string]string{"Item": "report"}, "cells": map[string]string{"Owner": "ann"}})
	if err != nil {
		t.Fatal(err)
	}
	want = strings.Replace(want, "| max | report |", "| ann | report |", 1)
	if got := readT(t, dir, "owed.md"); got != want {
		t.Fatalf("after update:\n%q\nwant\n%q", got, want)
	}
	checkCommit(t, env, dir, out, "update owed: report to ann", "owed.md")

	out, err = edit(t, env, map[string]any{"action": "close", "file": "owed.md", "kind": "owed", "subject": "deck sent",
		"match": map[string]string{"Item": "deck"}, "words": "got it", "source": "terminal", "decision_by": "owner", "table": "Items"})
	if err != nil {
		t.Fatal(err)
	}
	want = strings.Replace(want, "| bigm | deck | 2026-10-04T12:30:00Z |\n", "", 1)
	if got := readT(t, dir, "owed.md"); got != want {
		t.Fatalf("after close:\n%q\nwant\n%q", got, want)
	}
	checkCommit(t, env, dir, out, "close owed: deck sent", "owed.md")
	if row := out["row"].(map[string]any); row["Item"] != "deck" {
		t.Errorf("close row = %v", row)
	}

	out, err = edit(t, env, map[string]any{"action": "set_key", "file": "mode.md", "subject": "mode changed", "cells": map[string]string{"changed": "now"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := readT(t, dir, "mode.md"); got != strings.Replace(mode, "changed: 2026-01-01T00:00:00Z", "changed: 2026-10-04T12:00:00Z", 1) {
		t.Fatalf("after set_key: %q", got)
	}
	checkCommit(t, env, dir, out, "set changed: mode changed", "mode.md")
	if row := out["row"].(map[string]any); row["changed"] != "2026-10-04T12:00:00Z" {
		t.Errorf("set_key row = %v", row)
	}

	// A CRLF file with no last newline keeps its line endings and its end.
	steps := []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"action": "add", "cells": map[string]string{"A": "3", "B": "4"}}, "# T\r\n\r\n| A | B |\r\n|---|---|\r\n| 1 | 2 |\r\n| 3 | 4 |"},
		{map[string]any{"action": "update", "match": map[string]string{"A": "1"}, "cells": map[string]string{"B": "5"}}, "# T\r\n\r\n| A | B |\r\n|---|---|\r\n| 1 | 5 |\r\n| 3 | 4 |"},
		{map[string]any{"action": "close", "match": map[string]string{"A": "3"}, "words": "done", "source": "terminal", "decision_by": "bigm"}, "# T\r\n\r\n| A | B |\r\n|---|---|\r\n| 1 | 5 |"},
	}
	for _, s := range steps {
		s.args["file"], s.args["kind"], s.args["subject"] = "crlf.md", "task", "crlf"
		out, err := edit(t, env, s.args)
		if err != nil {
			t.Fatal(err)
		}
		if got := readT(t, dir, "crlf.md"); got != s.want {
			t.Fatalf("%s: crlf.md = %q, want %q", s.args["action"], got, s.want)
		}
		checkCommit(t, env, dir, out, fmt.Sprintf("%s task: crlf", s.args["action"]), "crlf.md")
	}
	if got := gitT(t, dir, "status", "--porcelain", "--", "other.md"); got != "M  other.md\n" {
		t.Errorf("other.md status = %q, want it staged and out of the commits", got)
	}
}

func TestLedgerEditCellText(t *testing.T) {
	fixture := strings.Replace(owedFixture, "| max | `a` and `b` | none |\n", "", 1)
	env, dir := ledgerFixture(t, map[string]string{"owed.md": fixture})
	for _, c := range []struct {
		value, stored string
		ok            bool
	}{
		{"`a` and `b`", "| `a` and `b` |", true},
		{"`x`", "| `x` |", true},
		{"a | b", `| a \| b |`, true},
		{"x ", "", false},
		{"Grüße – 日本", "| Grüße – 日本 |", true},
	} {
		head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
		before := readT(t, dir, "owed.md")
		out, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "table": "Items", "kind": "owed", "subject": "s",
			"cells": map[string]string{"Owner": "max", "Item": c.value, "Due (UTC)": ""}})
		if !c.ok {
			if err == nil {
				t.Errorf("%q: add accepted", c.value)
			}
			checkNothing(t, dir, head, "owed.md", before)
			continue
		}
		if err != nil {
			t.Fatalf("%q: %v", c.value, err)
		}
		if !strings.Contains(readT(t, dir, "owed.md"), c.stored) {
			t.Errorf("%q: file has no %q", c.value, c.stored)
		}
		if out["row"].(map[string]any)["Item"] != c.value {
			t.Errorf("%q: add row = %v", c.value, out["row"])
		}
		// The backticks are part of the cell text: a match without them finds no row.
		if bare := strings.ReplaceAll(c.value, "`", ""); bare != c.value {
			if _, err := edit(t, env, map[string]any{"action": "update", "file": "owed.md", "table": "Items", "kind": "owed", "subject": "s",
				"match": map[string]string{"Item": bare}, "cells": map[string]string{"Owner": "ann"}}); err == nil {
				t.Errorf("%q: the match %q without the backticks was accepted", c.value, bare)
			}
		}
		out, err = edit(t, env, map[string]any{"action": "update", "file": "owed.md", "table": "Items", "kind": "owed", "subject": "s",
			"match": map[string]string{"Item": c.value}, "cells": map[string]string{"Owner": "ann"}})
		if err != nil || out["row"].(map[string]any)["Item"] != c.value {
			t.Fatalf("%q: update = %v, %v", c.value, out, err)
		}
		out, err = edit(t, env, map[string]any{"action": "close", "file": "owed.md", "table": "Items", "kind": "owed", "subject": "s",
			"match": map[string]string{"Item": c.value}, "words": "w", "source": "terminal", "decision_by": "owner"})
		if err != nil || out["row"].(map[string]any)["Item"] != c.value {
			t.Fatalf("%q: close = %v, %v", c.value, out, err)
		}
		if got := readT(t, dir, "owed.md"); got != fixture {
			t.Errorf("%q: after close the file is %q", c.value, got)
		}
	}
}

func TestLedgerEditCloseBody(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture})
	words := "line one\nsays \"hi\" and `code`\n# not a heading\ntrailing spaces  \n\n\tlast"
	if _, err := edit(t, env, map[string]any{"action": "close", "file": "owed.md", "kind": "question", "subject": "bigm Q-a-h-1 - merge?",
		"match": map[string]string{"Item": "report"}, "words": words, "source": "Q-a-h-1", "decision_by": "owner"}); err != nil {
		t.Fatal(err)
	}
	want := "close question: bigm Q-a-h-1 - merge?\n\nWords:\n> line one\n> says \"hi\" and `code`\n> # not a heading\n> trailing spaces  \n>\n> \tlast\n" +
		"Source: Q-a-h-1\nRecorded (UTC): 2026-10-04T12:00:00Z\nTag: owner decision 2026-10-04\n"
	// git log ends the %B of each commit with one more newline.
	if got := gitT(t, dir, "log", "-1", "--format=%B"); got != want+"\n" {
		t.Errorf("close body:\n%q\nwant\n%q", got, want+"\n")
	}
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	before := readT(t, dir, "owed.md")
	for _, w := range []string{"", "a\rb", "a\r\nb", "a\x1bb"} {
		if _, err := edit(t, env, map[string]any{"action": "close", "file": "owed.md", "kind": "owed", "subject": "s",
			"match": map[string]string{"Item": "`a` and `b`"}, "words": w, "source": "terminal", "decision_by": "owner"}); err == nil {
			t.Errorf("words %q accepted", w)
		}
	}
	checkNothing(t, dir, head, "owed.md", before)
}

func TestLedgerEditMatchError(t *testing.T) {
	var b strings.Builder
	b.WriteString("| Owner | Item |\n|---|---|\n")
	for i := range 12 {
		fmt.Fprintf(&b, "| same | i%d |\n", i+1)
	}
	b.WriteString("| pair | p |\n| pair | p |\n")
	env, dir := ledgerFixture(t, map[string]string{"t.md": b.String()})
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	for _, c := range []struct {
		owner      string
		count      int
		candidates []string // the Owner texts of the candidates
	}{
		{"nobody", 0, strings.Split(strings.Repeat("same ", 10), " ")[:10]},
		{"pair", 2, []string{"pair", "pair"}},
		{"same", 12, strings.Split(strings.Repeat("same ", 10), " ")[:10]},
	} {
		_, err := edit(t, env, map[string]any{"action": "update", "file": "t.md", "kind": "task", "subject": "s",
			"match": map[string]string{"Owner": c.owner}, "cells": map[string]string{"Item": "z"}})
		me, ok := errors.AsType[*matchError](err)
		if !ok {
			t.Fatalf("%s: err = %v, want a match error", c.owner, err)
		}
		var owners []string
		for _, cand := range me.Candidates {
			if len(cand) != 1 {
				t.Errorf("%s: candidate %v has other columns than the match column", c.owner, cand)
			}
			owners = append(owners, cand["Owner"])
		}
		if me.Count != c.count || strings.Join(owners, ",") != strings.Join(c.candidates, ",") {
			t.Errorf("%s: count %d, candidates %v; want %d, %v", c.owner, me.Count, owners, c.count, c.candidates)
		}
		if !strings.Contains(err.Error(), `"count":`) || !strings.Contains(err.Error(), `"candidates":`) {
			t.Errorf("%s: the error text has no JSON: %v", c.owner, err)
		}
	}
	checkNothing(t, dir, head, "t.md", b.String())
	if _, err := edit(t, env, map[string]any{"action": "update", "file": "t.md", "kind": "task", "subject": "s",
		"match": map[string]string{"Nope": "x"}, "cells": map[string]string{"Item": "z"}}); err == nil || !strings.Contains(err.Error(), "unknown column") {
		t.Errorf("unknown column: err = %v", err)
	}
}

func TestLedgerEditRefusesDirtyTarget(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture})
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	dirty := owedFixture + "hand edit\n"
	if err := os.WriteFile(filepath.Join(dir, "owed.md"), []byte(dirty), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "kind": "owed", "subject": "s",
		"cells": map[string]string{"Owner": "a", "Item": "b", "Due (UTC)": ""}})
	if err == nil || !strings.Contains(err.Error(), "uncommitted changes") {
		t.Fatalf("err = %v", err)
	}
	checkNothing(t, dir, head, "owed.md", dirty)
}

func TestLedgerEditCallerCheck(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture})
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	args := map[string]any{"action": "add", "file": "owed.md", "kind": "owed", "subject": "s",
		"cells": map[string]string{"Owner": "a", "Item": "b", "Due (UTC)": ""}}
	for role, want := range map[string]string{"clanker-x": "only bigm calls ledger_edit", "": "BRUH_ROLE_KEY is not set"} {
		if _, err := edit(t, as(env, role), args); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("role %q: err = %v", role, err)
		}
	}
	checkNothing(t, dir, head, "owed.md", owedFixture)
	if _, err := os.Stat(filepath.Join(env.DataDir, "mail")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("a refused caller made the mail folder: %v", err)
	}
}

func TestLedgerEditTimeTokens(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture, "mode.md": "changed: x\n"})
	for token, want := range map[string]string{"now": "2026-10-04T12:00:00Z", "now+30m": "2026-10-04T12:30:00Z", "none": "none", "": ""} {
		out, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "kind": "owed", "subject": "s",
			"cells": map[string]string{"Owner": "t", "Item": "token " + token + ".", "Due (UTC)": token}})
		if err != nil {
			t.Fatalf("%q: %v", token, err)
		}
		if got := out["row"].(map[string]any)["Due (UTC)"]; got != want {
			t.Errorf("%q: Due = %v, want %q", token, got, want)
		}
	}
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	before := readT(t, dir, "owed.md")
	for _, bad := range []string{"2026-10-04T12:00:00Z", "tomorrow", "now+x", "now-5m"} {
		if _, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "kind": "owed", "subject": "s",
			"cells": map[string]string{"Owner": "t", "Item": "bad", "Due (UTC)": bad}}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	checkNothing(t, dir, head, "owed.md", before)
	if _, err := edit(t, env, map[string]any{"action": "set_key", "file": "mode.md", "subject": "s", "cells": map[string]string{"changed": "now+1h"}}); err != nil {
		t.Fatal(err)
	}
	if got := readT(t, dir, "mode.md"); got != "changed: 2026-10-04T13:00:00Z\n" {
		t.Errorf("mode.md = %q", got)
	}
}

func TestLedgerEditRestoresOnFailedCommit(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture})
	if err := os.WriteFile(filepath.Join(dir, ".git", "hooks", "pre-commit"), []byte("#!/bin/sh\necho refused >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	_, err := edit(t, env, map[string]any{"action": "add", "file": "owed.md", "kind": "owed", "subject": "s",
		"cells": map[string]string{"Owner": "a", "Item": "b", "Due (UTC)": ""}})
	if err == nil || !strings.Contains(err.Error(), "refused") {
		t.Fatalf("err = %v, want the output of the hook", err)
	}
	checkNothing(t, dir, head, "owed.md", owedFixture)
	if got := gitT(t, dir, "status", "--porcelain"); got != "" {
		t.Errorf("status after the restore = %q", got)
	}
	if fi, err := os.Stat(filepath.Join(dir, "owed.md")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("mode after the restore = %v, %v", fi.Mode(), err)
	}
}

// ruleSection is one rule of rules.md in the form of the ledger template.
func ruleSection(n int, subject, words, date string) string {
	return fmt.Sprintf("## R-%d: %s\n\n- Words: \"%s\"\n- Date: %s\n- Source: terminal\n- Tag: owner decision %s\n", n, subject, words, date, date[:10])
}

// rulesFile is a rules.md with the rules 1 to n, separated by an empty line.
func rulesFile(n int) string {
	var s []string
	for i := range n {
		s = append(s, ruleSection(i+1, fmt.Sprintf("rule %d", i+1), fmt.Sprintf("words %d", i+1), "2026-10-03T10:00:00Z"))
	}
	return "# Rules\n\nText.\n\n## Rules\n\n" + strings.Join(s, "\n")
}

// at sets the fake clock of env to 2026-10-09T19:04:47Z, the time of owner rule R-19.
func at(env Env) Env {
	env.Now = func() time.Time { return time.Date(2026, 10, 9, 19, 4, 47, 0, time.UTC) }
	return env
}

// Task 51 (R-19): add_rule writes the next R-<n> section in the form of the other sections, with
// the date and the tag from the server clock.
func TestLedgerEditAddRule(t *testing.T) {
	tmpl, err := os.ReadFile(filepath.Join("..", "ledger-template", "rules.md"))
	if err != nil {
		t.Fatal(err)
	}
	words := "NO BRUH, YOU AS bign MUST NOT MODIFY FILES!"
	for _, c := range []struct {
		name, file string
		next       int
	}{
		{"template with no rule", string(tmpl), 1},
		{"two rules", rulesFile(2), 3},
		{"eighteen rules", rulesFile(18), 19},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, dir := ledgerFixture(t, map[string]string{"rules.md": c.file})
			env = at(env)
			out, err := edit(t, env, map[string]any{"action": "add_rule", "file": "rules.md", "subject": "bigm does not modify files",
				"words": words, "source": "terminal"})
			if err != nil {
				t.Fatal(err)
			}
			want := c.file + "\n" + ruleSection(c.next, "bigm does not modify files", words, "2026-10-09T19:04:47Z")
			if got := readT(t, dir, "rules.md"); got != want {
				t.Fatalf("rules.md:\n%s\nwant\n%s", got, want)
			}
			id := fmt.Sprintf("R-%d", c.next)
			checkCommit(t, env, dir, out, "add rule: "+id+" - bigm does not modify files", "rules.md")
			body := "add rule: " + id + " - bigm does not modify files\n\nWords:\n> " + words +
				"\nSource: terminal\nRecorded (UTC): 2026-10-09T19:04:47Z\nTag: owner decision 2026-10-09\n\n"
			if got := gitT(t, dir, "log", "-1", "--format=%B"); got != body {
				t.Errorf("body = %q, want %q", got, body)
			}
			if out["row"].(map[string]any)["rule"] != id {
				t.Errorf("row = %v, want rule %s", out["row"], id)
			}
		})
	}
	// Words go into the one-line "- Words:" bullet: a newline or a tab would break the section or
	// inject a fake "## R-<n>:" heading.
	env, dir := ledgerFixture(t, map[string]string{"rules.md": rulesFile(2)})
	env = at(env)
	headSHA := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	for _, w := range []string{"x\n## R-99: fake", "a\tb"} {
		if _, err := edit(t, env, map[string]any{"action": "add_rule", "file": "rules.md", "subject": "s", "words": w, "source": "terminal"}); err == nil {
			t.Errorf("add_rule accepted words %q", w)
		}
	}
	checkNothing(t, dir, headSHA, "rules.md", rulesFile(2))
}

// Task 51 (R-19): close_rule deletes one rule section, and the next add_rule skips each ID that a
// close commit retired.
func TestLedgerEditCloseRule(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"rules.md": rulesFile(3)})
	env = at(env)
	closeRule := func(rule string) (map[string]any, error) {
		return edit(t, env, map[string]any{"action": "close_rule", "file": "rules.md", "rule": rule, "subject": "old rule",
			"words": "drop it", "source": "terminal", "decision_by": "owner"})
	}
	out, err := closeRule("R-2")
	if err != nil {
		t.Fatal(err)
	}
	r1, r3 := ruleSection(1, "rule 1", "words 1", "2026-10-03T10:00:00Z"), ruleSection(3, "rule 3", "words 3", "2026-10-03T10:00:00Z")
	head := "# Rules\n\nText.\n\n## Rules\n\n"
	if got := readT(t, dir, "rules.md"); got != head+r1+"\n"+r3 {
		t.Fatalf("after close R-2:\n%s", got)
	}
	checkCommit(t, env, dir, out, "close rule: R-2 - old rule", "rules.md")
	if got := gitT(t, dir, "log", "-1", "--format=%B"); !strings.Contains(got, "Words:\n> drop it\nSource: terminal\n") {
		t.Errorf("body = %q", got)
	}
	// The last section goes with the empty line above it.
	if _, err := closeRule("R-3"); err != nil {
		t.Fatal(err)
	}
	if got := readT(t, dir, "rules.md"); got != head+r1 {
		t.Fatalf("after close R-3:\n%q", got)
	}
	gitT(t, dir, "commit", "-q", "--allow-empty", "-m", "close rule: R-7 - by hand")
	before := readT(t, dir, "rules.md")
	headSHA := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	for _, r := range []string{"R-9", "R-1x", ""} {
		if _, err := closeRule(r); err == nil {
			t.Errorf("close_rule %q accepted", r)
		}
	}
	checkNothing(t, dir, headSHA, "rules.md", before)
	if _, err := call(t, as(env, "clerk-ledger"), "mail_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	out, err = edit(t, env, map[string]any{"action": "add_rule", "file": "rules.md", "subject": "s", "words": "w", "source": "terminal"})
	if err != nil {
		t.Fatal(err)
	}
	if out["row"].(map[string]any)["rule"] != "R-8" {
		t.Errorf("next rule = %v, want R-8 after the retired R-7", out["row"])
	}
}

// Task 51 (R-19): new_project makes a project file from the template, add_line and close_line
// change one bullet of "Decisions", and the add of a "Sessions" row lands in "Sessions".
func TestLedgerEditProjectFile(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture})
	tmpl, err := os.ReadFile(filepath.Join("..", "ledger-template", "projects", "_template.md"))
	if err != nil {
		t.Fatal(err)
	}
	out, err := edit(t, env, map[string]any{"action": "new_project", "file": "projects/shop.md", "subject": "shop"})
	if err != nil {
		t.Fatal(err)
	}
	want := strings.ReplaceAll(string(tmpl), "<project>", "shop")
	if got := readT(t, dir, "projects/shop.md"); got != want {
		t.Fatalf("projects/shop.md:\n%s", got)
	}
	checkCommit(t, env, dir, out, "add project: shop", "projects/shop.md")
	if fi, err := os.Stat(filepath.Join(dir, "projects", "shop.md")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Errorf("mode = %v, %v; want 0644", fi.Mode(), err)
	}

	line := func(action, text string) (map[string]any, error) {
		return edit(t, env, map[string]any{"action": action, "file": "projects/shop.md", "table": "Decisions", "kind": "decision",
			"subject": text, "text": text, "words": "newer decision", "source": "Q-shop-h-1", "decision_by": "owner"})
	}
	steps := []struct{ action, text, decisions string }{
		{"add_line", "use postgres (owner decision 2026-10-09)", "## Decisions\n\n- use postgres (owner decision 2026-10-09)\n\n## Sessions"},
		{"add_line", "ship on friday", "## Decisions\n\n- use postgres (owner decision 2026-10-09)\n- ship on friday\n\n## Sessions"},
		{"close_line", "use postgres (owner decision 2026-10-09)", "## Decisions\n\n- ship on friday\n\n## Sessions"},
	}
	for _, s := range steps {
		out, err := line(s.action, s.text)
		if err != nil {
			t.Fatalf("%s %q: %v", s.action, s.text, err)
		}
		want = regexpDecisions.ReplaceAllLiteralString(want, s.decisions)
		if got := readT(t, dir, "projects/shop.md"); got != want {
			t.Fatalf("%s %q:\n%s\nwant\n%s", s.action, s.text, got, want)
		}
		verb, _, _ := strings.Cut(s.action, "_")
		checkCommit(t, env, dir, out, verb+" decision: "+s.text, "projects/shop.md")
	}
	out, err = edit(t, env, map[string]any{"action": "add", "file": "projects/shop.md", "table": "Sessions", "kind": "session", "subject": "clanker-shop",
		"cells": map[string]string{"Role key": "clanker-shop", "Session ID": "s1", "Session name": "clanker-shop", "Machine": "mac",
			"State": "running", "Compaction count": "0", "Source read": "session_list"}})
	if err != nil {
		t.Fatal(err)
	}
	row := "| clanker-shop | s1 | clanker-shop | mac | running | 0 | session_list |\n"
	sep := "| Role key | Session ID | Session name | Machine | State | Compaction count | Source read |\n|---|---|---|---|---|---|---|\n"
	if got := readT(t, dir, "projects/shop.md"); got != strings.Replace(want, sep, sep+row, 1) {
		t.Fatalf("after the Sessions add:\n%s", got)
	}
	checkCommit(t, env, dir, out, "add session: clanker-shop", "projects/shop.md")

	// Refusals write nothing: a close_line with 0 or 2 matches, a new_project of an existing file
	// or of a bad key, and a heading that is not there.
	if _, err := line("add_line", "ship on friday"); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, as(env, "clerk-ledger"), "mail_read", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	before := readT(t, dir, "projects/shop.md")
	for _, args := range []map[string]any{
		{"action": "close_line", "table": "Decisions", "text": "nothing like this"},
		{"action": "close_line", "table": "Decisions", "text": "ship on friday"},
		{"action": "add_line", "table": "Nope", "text": "x"},
		{"action": "add_line", "table": "Decisions", "text": " x"},
		{"action": "add_line", "table": "Decisions", "text": "a\nb"},
		{"action": "new_project"},
		{"action": "new_project", "file": "projects/Shop_1.md"},
		{"action": "new_project", "file": "projects/a/b.md"},
		{"action": "new_project", "file": "other.md"},
	} {
		if _, ok := args["file"]; !ok {
			args["file"] = "projects/shop.md"
		}
		args["kind"], args["subject"], args["words"], args["source"], args["decision_by"] = "decision", "s", "w", "terminal", "owner"
		if _, err := edit(t, env, args); err == nil {
			t.Errorf("%v accepted", args)
		}
	}
	checkNothing(t, dir, head, "projects/shop.md", before)
	for _, f := range []string{"projects/Shop_1.md", "projects/a/b.md", "other.md"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s was made: %v", f, err)
		}
	}
}

// regexpDecisions matches the "Decisions" section of a project file up to the next heading.
var regexpDecisions = regexp.MustCompile(`(?s)## Decisions\n.*?## Sessions`)

// Task 51 (R-19): commit commits the listed changed files of init and of the learn step (a change,
// a new file, and a deleted file), and nothing else.
func TestLedgerEditCommit(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture, "learn/tree.json": "{}\n", "projects/old.md": "# old\n", "other.md": "x\n"})
	writeFile(t, filepath.Join(dir, "learn", "tree.json"), "{\"projects\":[]}\n")
	writeFile(t, filepath.Join(dir, "learn", "projects", "shop.json"), "{}\n")
	writeFile(t, filepath.Join(dir, "other.md"), "y\n")
	if err := os.Remove(filepath.Join(dir, "projects", "old.md")); err != nil {
		t.Fatal(err)
	}
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	for _, paths := range [][]string{nil, {"owed.md"}, {"../x.json"}, {".git/config"}, {filepath.Join(t.TempDir(), "x")}, {"learn/tree.json", "owed.md"}} {
		if _, err := edit(t, env, map[string]any{"action": "commit", "paths": paths, "kind": "learn", "subject": "s"}); err == nil {
			t.Errorf("commit of %q accepted", paths)
		}
	}
	checkNothing(t, dir, head, "other.md", "y\n")
	if got := gitT(t, dir, "diff", "--cached", "--name-only"); got != "" {
		t.Errorf("a refused commit left staged files: %q", got)
	}
	out, err := edit(t, env, map[string]any{"action": "commit", "kind": "learn", "subject": "learn_refresh",
		"paths": []string{filepath.Join(dir, "learn", "tree.json"), "learn/projects/shop.json", "projects/old.md"},
		"text":  "gone_docs: shop api/README.md"})
	if err != nil {
		t.Fatal(err)
	}
	checkCommit(t, env, dir, out, "commit learn: learn_refresh", "learn/projects/shop.json\nlearn/tree.json\nprojects/old.md")
	want := "commit learn: learn_refresh\n\nPaths:\n- learn/tree.json\n- learn/projects/shop.json\n- projects/old.md\nNotes:\ngone_docs: shop api/README.md\nRecorded (UTC): 2026-10-04T12:00:00Z\n\n"
	if got := gitT(t, dir, "log", "-1", "--format=%B"); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
	if got := gitT(t, dir, "status", "--porcelain"); got != " M other.md\n" {
		t.Errorf("status = %q, want only other.md dirty", got)
	}
}

func TestLedgerEditFileRule(t *testing.T) {
	env, dir := ledgerFixture(t, map[string]string{"owed.md": owedFixture, "learn/x.md": owedFixture, ".claude/x.md": owedFixture})
	outside := filepath.Join(t.TempDir(), "out.md")
	if err := os.WriteFile(outside, []byte(owedFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "link.md")); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", "link.md")
	gitT(t, dir, "commit", "-q", "-m", "link")
	head := strings.TrimSpace(gitT(t, dir, "rev-parse", "HEAD"))
	for _, f := range []string{"../x.md", "learn/x.md", "./learn/x.md", ".claude/x.md", "missing.md", "link.md", dir + "/owed.md", "owed.txt"} {
		if _, err := edit(t, env, map[string]any{"action": "add", "file": f, "kind": "owed", "subject": "s",
			"cells": map[string]string{"Owner": "a", "Item": "b", "Due (UTC)": ""}}); err == nil {
			t.Errorf("%q accepted", f)
		}
	}
	checkNothing(t, dir, head, "owed.md", owedFixture)
	if got := readT(t, filepath.Dir(outside), "out.md"); got != owedFixture {
		t.Errorf("the file outside changed: %q", got)
	}
}
