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
	for name, source := range map[string]any{
		"no source":  nil,
		"no call":    map[string]any{"value": "MERGED", "at": "2026-10-04T10:12:17Z"},
		"no value":   map[string]any{"call": "gh pr view 3", "at": "2026-10-04T10:12:17Z"},
		"no time":    map[string]any{"call": "gh pr view 3", "value": "MERGED"},
		"plain time": map[string]any{"call": "gh pr view 3", "value": "MERGED", "at": "this morning"},
	} {
		args := map[string]any{"kind": "result", "text": "owner/a#3 is merged"}
		if source != nil {
			args["source"] = source
		}
		_, err := call(t, env, "report_write", args)
		if err == nil || !strings.Contains(err.Error(), "a scout report line needs source") {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
	lines, _ := call(t, env, "report_read", map[string]any{"role_key": "clerk-a-scout1"})
	if n := len(lines.([]any)); n != 1 {
		t.Fatalf("report lines = %d, want 1: a refused line must not reach the file", n)
	}
	// A task clerk still writes a line with no source.
	if _, err := call(t, as(env, "clerk-a-1"), "report_write", map[string]any{"kind": "status", "text": "started"}); err != nil {
		t.Fatalf("task clerk line with no source: %v", err)
	}
}
