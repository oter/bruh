package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// fakeClaude writes a fake claude script. It logs each call (folder, two variables, one argument
// for each line), prints the agents fixture for "agents", and prints a --bg result with id.
func fakeClaude(t *testing.T, env *Env, id string, agents []map[string]any) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "log")
	fixture := filepath.Join(dir, "agents.json")
	data, _ := json.Marshal(agents)
	if err := os.WriteFile(fixture, data, 0o600); err != nil {
		t.Fatal(err)
	}
	script := fmt.Sprintf(`#!/bin/sh
{ echo ---; pwd -P; echo "FORCE=${CLAUDE_CODE_FORCE_SESSION_PERSISTENCE:-}"; echo "ROLE=${BRUH_ROLE_KEY:-}"; for a in "$@"; do echo "$a"; done; } >> '%s'
if [ "$1" = agents ]; then cat '%s'; exit 0; fi
echo "Starting background service…"
echo "backgrounded · %s · x"
`, logFile, fixture, id)
	env.ClaudeBin = filepath.Join(dir, "claude")
	if err := os.WriteFile(env.ClaudeBin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return logFile
}

// calls returns the logged calls, each as its lines.
func calls(t *testing.T, logFile string) [][]string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for part := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "---\n") {
		if part != "" {
			out = append(out, strings.Split(strings.TrimSuffix(part, "\n"), "\n"))
		}
	}
	return out
}

func writeRoleSettings(t *testing.T, env Env, key string) string {
	t.Helper()
	dir, _ := env.Dir("roles")
	f := filepath.Join(dir, key+".json")
	if err := os.WriteFile(f, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(f)
	return abs
}

func TestSessionLaunchArgv(t *testing.T) {
	t.Setenv("BRUH_ROLE_KEY", "bigm")
	env := testEnv(t, "bigm")
	settings := writeRoleSettings(t, env, "clanker-my-app")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	root, _ := filepath.Abs("..")
	log := fakeClaude(t, &env, "7c5dcf5d", []map[string]any{
		{"id": "7c5dcf5d", "kind": "background", "name": "clanker-my-app", "sessionId": "7c5dcf5d-aaaa", "state": "working", "startedAt": 1},
	})
	out, err := call(t, env, "session_launch", map[string]any{"agent": "clanker", "role_key": "clanker-my-app", "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(map[string]any)
	if res["session_id"] != "7c5dcf5d-aaaa" || res["name"] != "clanker-my-app" || res["state"] != "working" || res["role_key"] != "clanker-my-app" {
		t.Fatalf("result = %v", res)
	}
	var launch []string
	for _, c := range calls(t, log) {
		if c[3] == "--bg" {
			launch = c
		}
	}
	want := []string{cwd, "FORCE=1", "ROLE=", "--bg", "--agent", "bruh:clanker", "--name", "clanker-my-app", "--permission-mode", "auto",
		"--settings", settings, "--plugin-dir", root, "Read your start message with mail_read."}
	if !slices.Equal(launch, want) {
		t.Fatalf("launch =\n%q\nwant\n%q", launch, want)
	}

	// An installed plugin root gets no --plugin-dir.
	env.PluginRoot = filepath.Join(env.Home, ".claude", "plugins", "cache", "oter", "bruh", "0.10.0")
	if err := os.MkdirAll(env.PluginRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	log = fakeClaude(t, &env, "7c5dcf5d", []map[string]any{{"id": "7c5dcf5d", "kind": "background", "name": "clanker-my-app", "sessionId": "s", "state": "working", "startedAt": 1}})
	if _, err := call(t, env, "session_launch", map[string]any{"agent": "clanker", "role_key": "clanker-my-app", "cwd": cwd, "prompt": "Go."}); err != nil {
		t.Fatal(err)
	}
	for _, c := range calls(t, log) {
		if c[3] == "--bg" && (slices.Contains(c, "--plugin-dir") || c[len(c)-1] != "Go.") {
			t.Fatalf("installed launch = %q", c)
		}
	}
}

func TestSessionLaunchRefusals(t *testing.T) {
	env := testEnv(t, "clanker-a")
	fakeClaude(t, &env, "1", []map[string]any{{"id": "9", "kind": "background", "name": "clerk-a-1", "pid": 1, "startedAt": 1}})
	cwd := t.TempDir()
	writeRoleSettings(t, env, "clerk-a-1")
	for _, c := range []struct {
		args map[string]any
		want string
	}{
		{map[string]any{"agent": "clerk", "role_key": "clerk-b-1", "cwd": cwd}, "only its parent"},
		{map[string]any{"agent": "clerk", "role_key": "clerk-ab-1", "cwd": cwd}, "only its parent"},
		{map[string]any{"agent": "clanker", "role_key": "clerk-a-2", "cwd": cwd}, "does not fit"},
		{map[string]any{"agent": "clerk", "role_key": "clerk-a-2", "cwd": "rel/dir"}, "absolute folder"},
		{map[string]any{"agent": "clerk", "role_key": "clerk-a-2", "cwd": cwd}, "no role settings"},
		{map[string]any{"agent": "clerk", "role_key": "clerk-a-1", "cwd": cwd}, "live session"},
		{map[string]any{"agent": "clerk", "role_key": "clerk-a-1", "cwd": cwd, "prompt": "--help"}, "must not start"},
	} {
		_, err := call(t, env, "session_launch", c.args)
		mustErr(t, err, c.want)
	}
	_, err := call(t, as(env, "clerk-a-1"), "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-a-2", "cwd": cwd})
	mustErr(t, err, "only its parent")
}

func TestSessionResumeArgv(t *testing.T) {
	env := testEnv(t, "clanker-a")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	agents := []map[string]any{
		{"id": "0ld00000", "kind": "background", "name": "clerk-a-1", "sessionId": "old", "state": "stopped", "cwd": cwd, "startedAt": 1},
		{"id": "5e55a000", "kind": "background", "name": "clerk-a-1", "sessionId": "5e55a000-full", "state": "stopped", "cwd": cwd, "startedAt": 5},
		{"kind": "interactive", "name": "clerk-a-1", "sessionId": "other", "cwd": cwd, "startedAt": 9, "pid": 3},
	}
	log := fakeClaude(t, &env, "5e55a000", agents)
	out, err := call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.(map[string]any)["session_id"] != "5e55a000-full" {
		t.Fatalf("result = %v", out)
	}
	var resume []string
	for _, c := range calls(t, log) {
		if c[3] == "--resume" {
			resume = c
		}
	}
	want := []string{cwd, "FORCE=1", "ROLE=", "--resume", "5e55a000-full", "--bg", "Read your mailbox with mail_read."}
	if !slices.Equal(resume, want) {
		t.Fatalf("resume =\n%q\nwant\n%q", resume, want)
	}

	fakeClaude(t, &env, "c0b1e000", agents)
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	mustErr(t, err, "started a copy")

	agents[1]["pid"] = 7
	fakeClaude(t, &env, "5e55a000", agents)
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	mustErr(t, err, "is live")
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-9"})
	mustErr(t, err, "no background session")
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-b-1"})
	mustErr(t, err, "only its parent")
}

func TestSessionList(t *testing.T) {
	env := testEnv(t, "bigm")
	fakeClaude(t, &env, "1", []map[string]any{
		{"name": "bigm", "kind": "interactive"}, {"name": "clanker-a"}, {"name": "Draft plan review"}, {"name": "platform-26"}, {},
	})
	out, err := call(t, env, "session_list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	list := out.([]any)
	if len(list) != 2 || list[1].(map[string]any)["role_key"] != "clanker-a" {
		t.Fatalf("list = %v", list)
	}
}

// Final review M3: bigm may start and resume the merger clerk of any project, so that it runs the
// merger of a remote project on its own machine. Every other caller must be the parent of the key,
// and bigm is not the parent of a task clerk.
func TestSessionLaunchAndResumeByBigm(t *testing.T) {
	env := testEnv(t, "bigm")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	fakeClaude(t, &env, "3e4ce000", []map[string]any{
		{"id": "3e4ce000", "kind": "background", "name": "clerk-remote-app-merge", "sessionId": "3e4ce000-full", "state": "done", "cwd": cwd, "startedAt": 1},
	})
	if _, err := call(t, env, "role_settings_write", map[string]any{"role_key": "clerk-remote-app-merge"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-remote-app-merge", "cwd": cwd}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "session_resume", map[string]any{"role_key": "clerk-remote-app-merge"}); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, env, "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-remote-app-t1", "cwd": cwd})
	mustErr(t, err, "only its parent")
	for _, caller := range []string{"clanker-other", "clerk-remote-app-x", "clerk-ledger"} {
		_, err := call(t, as(env, caller), "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-remote-app-merge", "cwd": cwd})
		mustErr(t, err, "only its parent")
		_, err = call(t, as(env, caller), "session_resume", map[string]any{"role_key": "clerk-remote-app-merge"})
		mustErr(t, err, "only its parent")
	}
}
