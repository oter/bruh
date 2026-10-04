package main

import (
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
	later, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clanker-a", "since": at})
	if len(later.([]any)) != 1 {
		t.Fatalf("later = %v", later)
	}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "gossip", "text": "x"}); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("err = %v", err)
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
	// an idle starter.
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
