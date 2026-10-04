package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// checkCommit checks the commit subject, the one file of the commit, the DONE mail, and the nudge.
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
	if n := out["nudge"].(map[string]any); n["to"] != "clerk-ledger" || n["header"] != header {
		t.Errorf("nudge = %v, want %s", n, header)
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
			"cells": map[string]string{"Owner": "t", "Item": "token " + token + ".","Due (UTC)": token}})
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
