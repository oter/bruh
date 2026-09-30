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
