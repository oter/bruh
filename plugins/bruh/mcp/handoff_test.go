package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const goodHandoff = "## Goal\n- Ship the login fix\n## State\n- Tests pass on branch fix-login\n"

func TestHandoffReplacesAndKeepsHistory(t *testing.T) {
	env := testEnv(t, "clanker-a")
	if _, err := call(t, env, "handoff_write", map[string]any{"text": goodHandoff}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "handoff_write", map[string]any{"text": strings.Replace(goodHandoff, "login", "logout", 1)}); err != nil {
		t.Fatal(err)
	}
	now, _ := call(t, env, "handoff_read", map[string]any{})
	text := now.(string)
	if !strings.Contains(text, "logout") || strings.Contains(text, "Ship the login fix") {
		t.Fatalf("current = %q", text)
	}
	if !regexp.MustCompile(`^Updated: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z`).MatchString(text) {
		t.Fatalf("no stamp: %q", text)
	}
	hist, err := os.ReadFile(filepath.Join(env.DataDir, "handoffs", "clanker-a.history.md"))
	if err != nil || !strings.Contains(string(hist), "login fix") {
		t.Fatalf("history = %q, %v", hist, err)
	}
}

func TestHandoffRefusals(t *testing.T) {
	env := testEnv(t, "clanker-a")
	if _, err := call(t, env, "handoff_write", map[string]any{"text": goodHandoff}); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ text, want string }{
		{strings.Repeat("- item\n", 1400), "over 8000 characters"},
		{"## Files\n- `docs/plan.md`\n- src/app.js\n", "only file pointers"},
		{"## State\n- Waiting on agent a55bfb00ce43c38de\n", "subagent ID"},
		{"## State\n- Notes in /tmp/notes.md\n", "/tmp path"},
	}
	for _, c := range cases {
		if _, err := call(t, env, "handoff_write", map[string]any{"text": c.text}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, err = %v", c.want, err)
		}
	}
	now, _ := call(t, env, "handoff_read", map[string]any{})
	if !strings.Contains(now.(string), "Ship the login fix") {
		t.Fatal("a refused write changed the file")
	}
}

func TestHandoffReadEmpty(t *testing.T) {
	got, err := call(t, testEnv(t, "bigm"), "handoff_read", map[string]any{"role_key": "clanker-zzz"})
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}
