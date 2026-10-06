package main

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestReportWriteAndRead(t *testing.T) {
	env := testEnv(t, "clanker-a")
	first, err := call(t, env, "report_write", map[string]any{"kind": "status", "text": "task 1 started"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	at := first.(map[string]any)["at"].(string)
	if _, err := call(t, env, "report_write", map[string]any{"kind": "result", "text": "merged",
		"source": map[string]any{"call": "gh pr view 3 --json state", "value": "MERGED", "at": at}}); err != nil {
		t.Fatal(err)
	}
	all, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clanker-a"})
	lines := all.([]any)
	if len(lines) != 2 || lines[1].(map[string]any)["source"].(map[string]any)["value"] != "MERGED" {
		t.Fatalf("lines = %v", lines)
	}
	// since is inclusive: the line at since and the later line.
	later, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clanker-a", "since": at})
	if len(later.([]any)) != 2 {
		t.Fatalf("later = %v", later)
	}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "gossip", "text": "x"}); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("err = %v", err)
	}
}

// Task 17: a status or result line of a role other than bigm pushes a "DONE: report <role key>"
// notice to bigm, with at most one unread notice for each role.
func TestReportWritePushesNoticeToBigm(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	// A fixed clock: the lines of one step have the same millisecond at, the worst case of since.
	now := time.Date(2026, 10, 5, 17, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	bigm := as(env, "bigm")
	write := func(e Env, kind, text string) {
		t.Helper()
		if _, err := call(t, e, "report_write", map[string]any{"kind": kind, "text": text}); err != nil {
			t.Fatal(err)
		}
	}
	read := func() []map[string]any {
		t.Helper()
		got, err := call(t, bigm, "mail_read", map[string]any{})
		if err != nil {
			t.Fatal(err)
		}
		var msgs []map[string]any
		for _, m := range got.([]any) {
			msgs = append(msgs, m.(map[string]any))
		}
		return msgs
	}
	bodyLine := func(m map[string]any) ReportLine {
		t.Helper()
		var l ReportLine
		if err := json.Unmarshal([]byte(m["body"].(string)), &l); err != nil {
			t.Fatalf("body %q is not a report line: %v", m["body"], err)
		}
		return l
	}

	// (a) One status line gives one notice; its body is the report line.
	write(env, "status", "task 1 started")
	msgs := read()
	if len(msgs) != 1 || msgs[0]["header"] != "DONE: report clerk-a-1" || msgs[0]["from"] != "clerk-a-1" {
		t.Fatalf("bigm mail = %v, want one DONE: report clerk-a-1", msgs)
	}
	if l := bodyLine(msgs[0]); l.Text != "task 1 started" || l.Kind != "status" || l.From != "clerk-a-1" {
		t.Fatalf("body line = %+v", l)
	}

	// (b) The cap: three lines in one millisecond with no read between give one notice, and
	// report_read with since = the at of its body line returns the body line and the two later
	// lines, which have the same at.
	now = now.Add(time.Millisecond)
	write(env, "status", "plan done")
	write(env, "result", "PR open")
	write(env, "status", "review")
	// (c) The cap is per role: clanker-a gets its own notice while clerk-a-1 has one pending.
	write(as(env, "clanker-a"), "status", "wave 1")
	msgs = read()
	from := map[string]int{}
	for _, m := range msgs {
		from[m["from"].(string)]++
	}
	if len(msgs) != 2 || from["clerk-a-1"] != 1 || from["clanker-a"] != 1 {
		t.Fatalf("bigm mail = %v, want one notice from clerk-a-1 and one from clanker-a", msgs)
	}
	var first ReportLine
	for _, m := range msgs {
		if m["from"] == "clerk-a-1" {
			first = bodyLine(m)
		}
	}
	if first.Text != "plan done" {
		t.Fatalf("clerk-a-1 notice body = %+v, want the first line after the read", first)
	}
	later, err := call(t, bigm, "report_read", map[string]any{"role_key": "clerk-a-1", "since": first.At})
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, l := range later.([]any) {
		texts = append(texts, l.(map[string]any)["text"].(string))
	}
	if want := []string{"plan done", "PR open", "review"}; !slices.Equal(texts, want) {
		t.Fatalf("report_read since %s = %v, want %v", first.At, texts, want)
	}

	// (d) After bigm read the notice, a new line gives a new notice.
	write(env, "result", "merged")
	if msgs = read(); len(msgs) != 1 || bodyLine(msgs[0]).Text != "merged" {
		t.Fatalf("bigm mail after a read = %v, want one new notice", msgs)
	}

	// (e) No notice for an event or an answer line, for a line of bigm, or for a refused line.
	write(env, "event", "clanker-a not running; mail pending")
	write(env, "answer", "yes")
	write(bigm, "status", "sweep done")
	if _, err := call(t, as(env, "clerk-a-scout1"), "report_write", map[string]any{"kind": "status", "text": "no source"}); err == nil {
		t.Fatal("scout line with no source: want a refusal")
	}
	if msgs = read(); len(msgs) != 0 {
		t.Fatalf("bigm mail = %v, want no notice", msgs)
	}

	// (f) The report file format does not change: each line has only at, from, kind, and text.
	data, err := os.ReadFile(filepath.Join(env.DataDir, "reports", "clerk-a-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatal(err)
		}
		keys := slices.Sorted(maps.Keys(m))
		if !slices.Equal(keys, []string{"at", "from", "kind", "text"}) {
			t.Errorf("report line keys = %v, want at, from, kind, text: %s", keys, raw)
		}
	}
}

// R-1: each report line of a scout clerk carries its source read, with a UTC time in RFC 3339.
func TestReportWriteScoutNeedsSource(t *testing.T) {
	env := testEnv(t, "clerk-a-scout1")
	src := map[string]any{"call": "gh pr view 3 --json state", "value": "MERGED", "at": "2026-10-04T10:12:17Z"}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "result", "text": "owner/a#3 is merged", "source": src}); err != nil {
		t.Fatalf("scout line with a source: %v", err)
	}
	// A read that finds nothing often prints nothing: an empty value is valid output.
	empty := map[string]any{"call": "git -C /repo log --grep=fix main..main", "value": "", "at": "2026-10-04T10:12:17Z"}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "result", "text": "not found: fix commit", "source": empty}); err != nil {
		t.Fatalf("scout line with an empty value: %v", err)
	}
	// The event line of the mail procedure (clerk.md) has no source, so the bigm sweep can resume
	// an idle clanker.
	if _, err := call(t, env, "report_write", map[string]any{"kind": "event", "text": "clanker-a not running; mail pending"}); err != nil {
		t.Fatalf("scout mail-pending event: %v", err)
	}
	for name, args := range map[string]map[string]any{
		"no source":   {"kind": "result"},
		"no call":     {"kind": "result", "source": map[string]any{"value": "MERGED", "at": "2026-10-04T10:12:17Z"}},
		"no value":    {"kind": "result", "source": map[string]any{"call": "gh pr view 3", "at": "2026-10-04T10:12:17Z"}},
		"no time":     {"kind": "result", "source": map[string]any{"call": "gh pr view 3", "value": "MERGED"}},
		"plain time":  {"kind": "result", "source": map[string]any{"call": "gh pr view 3", "value": "MERGED", "at": "this morning"}},
		"other event": {"kind": "event"},
		"bad key":     {"kind": "event", "text": "Clanker A not running; mail pending"},
		"not event":   {"kind": "status", "text": "clanker-a not running; mail pending"},
	} {
		if args["text"] == nil {
			args["text"] = "owner/a#3 is merged"
		}
		_, err := call(t, env, "report_write", args)
		if err == nil || !strings.Contains(err.Error(), "a scout report line needs source") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
	lines, _ := call(t, env, "report_read", map[string]any{"role_key": "clerk-a-scout1"})
	if n := len(lines.([]any)); n != 3 {
		t.Fatalf("report lines = %d, want 3: a refused line must not reach the file", n)
	}
	if v := lines.([]any)[1].(map[string]any)["source"].(map[string]any)["value"]; v != "" {
		t.Errorf("empty value stored as %q", v)
	}
	// A task clerk still writes a line with no source.
	if _, err := call(t, as(env, "clerk-a-1"), "report_write", map[string]any{"kind": "status", "text": "started"}); err != nil {
		t.Fatalf("task clerk line with no source: %v", err)
	}
}

// Task 30: the optional phase is stored as a top-level field of the line; any other value is an
// error and writes no line.
func TestReportWritePhase(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	if _, err := call(t, env, "report_write", map[string]any{"kind": "event", "text": "phase review", "phase": "review"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "event", "text": "no phase"}); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"deploy", ""} {
		if _, err := call(t, env, "report_write", map[string]any{"kind": "event", "text": "x", "phase": bad}); err == nil || !strings.Contains(err.Error(), "invalid phase") {
			t.Fatalf("phase %q: err = %v", bad, err)
		}
	}
	dir, err := env.Dir("reports")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "clerk-a-1.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	raw := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(raw) != 2 {
		t.Fatalf("lines = %q, want 2: a bad phase writes no line", raw)
	}
	var first, second map[string]any
	if json.Unmarshal([]byte(raw[0]), &first) != nil || json.Unmarshal([]byte(raw[1]), &second) != nil {
		t.Fatalf("lines = %q", raw)
	}
	if first["phase"] != "review" {
		t.Fatalf("first line = %v, want top-level phase review", first)
	}
	if _, ok := second["phase"]; ok {
		t.Fatalf("second line = %v, want no phase key", second)
	}
	got, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clerk-a-1"})
	if l := got.([]any); l[0].(map[string]any)["phase"] != "review" {
		t.Fatalf("report_read = %v", l)
	}
}
