package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// No test reaches a real Orca: a test that wants Orca calls fakeOrca.
func init() {
	orcaLookPath = func(string) (string, error) { return "", exec.ErrNotFound }
}

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

// orcaStatus is the output of orca status --json; the arguments replace the ready values.
func orcaStatus(ok, running, reachable bool, state, version string) string {
	return fmt.Sprintf(`{"ok":%t,"result":{"app":{"running":%t,"pid":1},"runtime":{"state":%q,"reachable":%t,"appVersion":%q}}}`, ok, running, state, reachable, version)
}

var orcaReadyStatus = orcaStatus(true, true, true, "ready", orcaMinVersion)

// orcaList is the output of orca terminal list --json with one terminal for each title; the
// handle of the terminal at index n is term_<n>.
func orcaList(titles ...string) string {
	rows := []map[string]any{}
	for i, title := range titles {
		rows = append(rows, map[string]any{"handle": fmt.Sprintf("term_%d", i), "title": title, "worktreePath": "/x"})
	}
	data, _ := json.Marshal(map[string]any{"ok": true, "result": map[string]any{"terminals": rows, "totalCount": len(rows)}})
	return string(data)
}

// printJSON is sh code that prints s.
func printJSON(s string) string { return "printf '%s\\n' " + shq(s) }

// fakeOrca makes orcaLookPath find a fake orca for each name. status is sh code for the status
// command; terminal list prints list; terminal create prints the handle term_new, or fails
// when failCreate is set, or prints $ORCA_CREATE when it is set; terminal close fails for the
// handle $ORCA_CLOSE_FAIL. The log has one line for each call.
func fakeOrca(t *testing.T, status, list string, failCreate bool) (bin, logFile string) {
	t.Helper()
	t.Setenv("ORCA_CLI_COMMAND", "")
	t.Setenv("ORCA_CREATE", "")
	t.Setenv("ORCA_CLOSE_FAIL", "")
	create := printJSON(`{"ok":true,"result":{"terminal":{"handle":"term_new","title":"x"}}}`)
	if failCreate {
		create = printJSON(`{"ok":false,"error":{"code":"selector_not_found"}}`) + "; exit 1"
	}
	logFile = fakeCLI(t, &bin, `case "$1 $2" in
status*) `+status+` ;;
"terminal list") `+printJSON(list)+` ;;
"terminal create") [ -n "$ORCA_CREATE" ] && { printf '%s\n' "$ORCA_CREATE"; exit 0; }; `+create+` ;;
"terminal close") [ "$4" = "$ORCA_CLOSE_FAIL" ] && exit 1; `+printJSON(`{"ok":true,"result":{"closed":true}}`)+` ;;
*) exit 9 ;;
esac`)
	old := orcaLookPath
	orcaLookPath = func(string) (string, error) { return bin, nil }
	t.Cleanup(func() { orcaLookPath = old })
	return bin, logFile
}

// orcaLog returns the logged orca calls, one line for each.
func orcaLog(t *testing.T, logFile string) []string {
	t.Helper()
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	return slices.DeleteFunc(strings.Split(string(data), "\n"), func(s string) bool { return s == "" })
}

// launchClanker starts clanker-my-app in a new folder through a fake claude, checks that the
// claude argv is the one of TestSessionLaunchArgv, and returns the result and the folder.
func launchClanker(t *testing.T, env Env) (map[string]any, string) {
	t.Helper()
	settings := writeRoleSettings(t, env, "clanker-my-app")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	log := fakeClaude(t, &env, "7c5dcf5d", []map[string]any{
		{"id": "7c5dcf5d", "kind": "background", "name": "clanker-my-app", "sessionId": "7c5dcf5d-aaaa", "state": "working", "startedAt": 1},
	})
	out, err := call(t, env, "session_launch", map[string]any{"agent": "clanker", "role_key": "clanker-my-app", "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	res := out.(map[string]any)
	if res["session_id"] != "7c5dcf5d-aaaa" || res["state"] != "working" {
		t.Fatalf("result = %v", res)
	}
	var launch []string
	for _, c := range calls(t, log) {
		if c[3] == "--bg" {
			launch = c
		}
	}
	root, _ := filepath.Abs("..")
	want := []string{cwd, "FORCE=1", "ROLE=", "--bg", "--agent", "bruh:clanker", "--name", "clanker-my-app", "--permission-mode", "auto",
		"--settings", settings, "--plugin-dir", root, "Read your start message with mail_read."}
	if !slices.Equal(launch, want) {
		t.Fatalf("launch =\n%q\nwant\n%q", launch, want)
	}
	return res, cwd
}

func TestOrcaOff(t *testing.T) {
	_, log := fakeOrca(t, printJSON(orcaReadyStatus), orcaList("clanker-my-app"), false)
	env := testEnv(t, "bigm")
	writeSettings(t, env, `{"pluginConfigs":{"bruh@oter":{"options":{"orca_local":"off"}}}}`)
	res, _ := launchClanker(t, env)
	if _, ok := res["orca"]; ok || res["orca_error"] != nil {
		t.Fatalf("off: result = %v, want no orca fields", res)
	}
	out, err := call(t, env, "session_tab_close", map[string]any{"role_key": "clanker-my-app"})
	if err != nil || out.(map[string]any)["closed"] != 0.0 {
		t.Fatalf("off: session_tab_close = %v, %v", out, err)
	}
	if got := orcaLog(t, log); len(got) != 0 {
		t.Fatalf("off: orca calls = %q, want none", got)
	}
}

func TestOrcaNotFound(t *testing.T) {
	res, _ := launchClanker(t, testEnv(t, "bigm"))
	if _, ok := res["orca"]; ok || res["orca_error"] != nil {
		t.Fatalf("no orca: result = %v, want no orca fields", res)
	}
}

func TestOrcaViewerAtLaunch(t *testing.T) {
	_, log := fakeOrca(t, printJSON(orcaReadyStatus), orcaList("clanker-my-app-2", "clanker-my-app", "Clanker-my-app"), false)
	res, cwd := launchClanker(t, testEnv(t, "bigm"))
	if res["orca"] != "term_new" || res["orca_error"] != nil {
		t.Fatalf("result = %v", res)
	}
	want := []string{
		"status --json",
		"terminal list --json",
		"terminal close --terminal term_1 --tab --json",
		"terminal create --worktree path:" + cwd + " --title clanker-my-app --command claude attach 7c5dcf5d --json",
	}
	if got := orcaLog(t, log); !slices.Equal(got, want) {
		t.Fatalf("orca calls =\n%q\nwant\n%q", got, want)
	}
}

// Each readiness condition of the detection fails alone, with the ready values in the other fields.
func TestOrcaNotReady(t *testing.T) {
	old := orcaTimeout
	t.Cleanup(func() { orcaTimeout = old })
	for _, c := range []struct{ name, status, want string }{
		{"ok false", printJSON(orcaStatus(false, true, true, "ready", orcaMinVersion)), "ok is not true"},
		{"app not running", printJSON(orcaStatus(true, false, true, "ready", orcaMinVersion)), "result.app.running"},
		{"unreachable", printJSON(orcaStatus(true, true, false, "ready", orcaMinVersion)), "result.runtime.reachable"},
		{"starting", printJSON(orcaStatus(true, true, true, "starting", orcaMinVersion)), `"starting"`},
		{"old version", printJSON(orcaStatus(true, true, true, "ready", "1.4.217")), `"1.4.217" is not 1.4.218`},
		{"bad version", printJSON(orcaStatus(true, true, true, "ready", "1.4")), `"1.4" is not`},
		{"not JSON", "echo hello", "orca status: invalid character"},
		{"exit 1", "echo '{}'; exit 1", "exit status 1"},
		{"timeout", "exec sleep 5", "orca status: signal: killed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			// Only the timeout row gets a short timeout: a loaded machine can take longer to run sh.
			orcaTimeout = old
			if c.name == "timeout" {
				orcaTimeout = 300 * time.Millisecond
			}
			_, log := fakeOrca(t, c.status, orcaList(), false)
			res, _ := launchClanker(t, testEnv(t, "bigm"))
			if e, _ := res["orca_error"].(string); !strings.Contains(e, c.want) || res["orca"] != nil {
				t.Fatalf("result = %v, want orca_error with %q", res, c.want)
			}
			if got := orcaLog(t, log); !slices.Equal(got, []string{"status --json"}) {
				t.Fatalf("orca calls = %q, want only status", got)
			}
		})
	}
	if v := parseVersion("1.10.0"); slices.Compare(v, parseVersion(orcaMinVersion)) <= 0 {
		t.Fatalf("1.10.0 = %v is not newer than %s", v, orcaMinVersion)
	}
}

func TestOrcaCreateFails(t *testing.T) {
	fakeOrca(t, printJSON(orcaReadyStatus), orcaList(), true)
	res, _ := launchClanker(t, testEnv(t, "bigm"))
	if e, _ := res["orca_error"].(string); !strings.Contains(e, "orca terminal create") || !strings.Contains(e, "selector_not_found") || res["orca"] != nil {
		t.Fatalf("result = %v, want orca_error of the create", res)
	}
}

// A create that answers ok without a handle is an orca_error, not a tab.
func TestOrcaCreateNoHandle(t *testing.T) {
	for _, out := range []string{`{"ok":true,"result":{}}`, `{"ok":true,"result":"term_new"}`, `{"ok":true}`} {
		fakeOrca(t, printJSON(orcaReadyStatus), orcaList(), false)
		t.Setenv("ORCA_CREATE", out)
		res, _ := launchClanker(t, testEnv(t, "bigm"))
		if e, _ := res["orca_error"].(string); !strings.Contains(e, "orca terminal create") || res["orca"] != nil {
			t.Errorf("create %s: result = %v, want orca_error of the create", out, res)
		}
	}
}

// session_resume keeps an open tab with the exact title, and else opens one in the start folder
// of the clerk, not in the worktree that it moved into.
func TestOrcaViewerAtResume(t *testing.T) {
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	clerkDir := filepath.Join(cwd, ".claude", "worktrees", "t1")
	if err := os.MkdirAll(clerkDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name, list, handle string
		want               []string
	}{
		{"tab open", orcaList("clerk-a-1"), "term_0", []string{"status --json", "terminal list --json"}},
		{"no tab", orcaList("clerk-a-1x", "clerk-a-10"), "term_new", []string{"status --json", "terminal list --json",
			"terminal create --worktree path:" + cwd + " --title clerk-a-1 --command claude attach 5e55a000 --json"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, log := fakeOrca(t, printJSON(orcaReadyStatus), c.list, false)
			env := testEnv(t, "clanker-a")
			claudeLog := fakeClaude(t, &env, "5e55a000", []map[string]any{
				{"id": "5e55a000", "kind": "background", "name": "clerk-a-1", "sessionId": "5e55a000-full", "state": "stopped", "cwd": clerkDir, "startedAt": 5},
			})
			out, err := call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
			if err != nil {
				t.Fatal(err)
			}
			if res := out.(map[string]any); res["orca"] != c.handle || res["orca_error"] != nil {
				t.Fatalf("result = %v", res)
			}
			if got := orcaLog(t, log); !slices.Equal(got, c.want) {
				t.Fatalf("orca calls =\n%q\nwant\n%q", got, c.want)
			}
			for _, cl := range calls(t, claudeLog) {
				if cl[3] == "--resume" && !slices.Equal(cl[3:], []string{"--resume", "5e55a000-full", "--bg", "Read your mailbox with mail_read."}) {
					t.Fatalf("resume = %q", cl)
				}
			}
		})
	}
}

// On Linux a bare orca is the screen reader: the detection never looks it up or runs it.
func TestOrcaLinux(t *testing.T) {
	bin, log := fakeOrca(t, printJSON(orcaReadyStatus), orcaList(), false)
	old := goos
	t.Cleanup(func() { goos = old })
	var looked []string
	orcaLookPath = func(name string) (string, error) {
		looked = append(looked, name)
		return bin, nil
	}
	env := testEnv(t, "bigm")
	for _, c := range []struct{ goos, cli, want string }{
		{"linux", "", "orca-ide"},
		{"linux", "/opt/orca/bin/orca-ide", "/opt/orca/bin/orca-ide"},
		{"linux", "/usr/bin/orca", ""},
		{"linux", "orca", ""},
		{"linux", "orca-dev", ""},
		{"darwin", "", "orca"},
		{"darwin", "/usr/local/bin/orca-dev", ""},
	} {
		goos, looked = c.goos, nil
		t.Setenv("ORCA_CLI_COMMAND", c.cli)
		got := orcaBin(env)
		if c.want == "" && (got != "" || looked != nil) {
			t.Errorf("%s, ORCA_CLI_COMMAND=%q: bin = %q, looked up %q, want none", c.goos, c.cli, got, looked)
		}
		if c.want != "" && (got != bin || !slices.Equal(looked, []string{c.want})) {
			t.Errorf("%s, ORCA_CLI_COMMAND=%q: bin = %q, looked up %q, want %q", c.goos, c.cli, got, looked, c.want)
		}
	}
	if got := orcaLog(t, log); len(got) != 0 {
		t.Fatalf("orca calls = %q", got)
	}
}

func TestSessionTabClose(t *testing.T) {
	_, log := fakeOrca(t, printJSON(orcaReadyStatus), orcaList("clerk-a-1", "clerk-a-10", "clerk-a-1"), false)
	env := testEnv(t, "clanker-a")
	_, err := call(t, as(env, "clanker-b"), "session_tab_close", map[string]any{"role_key": "clerk-a-1"})
	mustErr(t, err, "only its parent")
	if got := orcaLog(t, log); len(got) != 0 {
		t.Fatalf("refused call: orca calls = %q", got)
	}
	out, err := call(t, env, "session_tab_close", map[string]any{"role_key": "clerk-a-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res := out.(map[string]any); res["closed"] != 2.0 || res["role_key"] != "clerk-a-1" || res["orca_error"] != nil {
		t.Fatalf("result = %v", res)
	}
	want := []string{"terminal list --json", "terminal close --terminal term_0 --tab --json", "terminal close --terminal term_2 --tab --json"}
	if got := orcaLog(t, log); !slices.Equal(got, want) {
		t.Fatalf("orca calls =\n%q\nwant\n%q", got, want)
	}
}

// A close that fails partway reports the tabs that it closed before the error.
func TestSessionTabClosePartial(t *testing.T) {
	_, log := fakeOrca(t, printJSON(orcaReadyStatus), orcaList("clerk-a-1", "clerk-a-10", "clerk-a-1"), false)
	t.Setenv("ORCA_CLOSE_FAIL", "term_2")
	out, err := call(t, testEnv(t, "clanker-a"), "session_tab_close", map[string]any{"role_key": "clerk-a-1"})
	if err != nil {
		t.Fatal(err)
	}
	if res := out.(map[string]any); res["closed"] != 1.0 || res["orca_error"] == nil {
		t.Fatalf("result = %v, want closed 1 and orca_error", res)
	}
	if got := orcaLog(t, log); len(got) != 3 {
		t.Fatalf("orca calls = %q, want list and two closes", got)
	}
}

// R-1: the clanker of the project starts its scout clerk, as it starts any clerk. bigm is not
// the parent of a scout, so it neither starts nor resumes one, and no one resumes a scout: a
// stopped scout gets a new one.
func TestSessionLaunchScoutByClanker(t *testing.T) {
	env := testEnv(t, "clanker-app")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	fakeClaude(t, &env, "5c0a7000", []map[string]any{
		{"id": "5c0a7000", "kind": "background", "name": "clerk-app-scout1", "sessionId": "5c0a7000-full", "state": "done", "cwd": cwd, "startedAt": 1},
	})
	if _, err := call(t, env, "role_settings_write", map[string]any{"role_key": "clerk-app-scout1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-app-scout1", "cwd": cwd}); err != nil {
		t.Errorf("clanker-app launches clerk-app-scout1: %v", err)
	}
	_, err := call(t, env, "session_resume", map[string]any{"role_key": "clerk-app-scout1"})
	mustErr(t, err, "a scout is never resumed")
	for _, caller := range []string{"bigm", "clanker-other", "clerk-app-x", "clerk-app-scout2", "clerk-ledger"} {
		_, err := call(t, as(env, caller), "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-app-scout1", "cwd": cwd})
		mustErr(t, err, "only its parent")
		_, err = call(t, as(env, caller), "session_resume", map[string]any{"role_key": "clerk-app-scout1"})
		mustErr(t, err, "only its parent")
	}
}

// colorClaude is fakeClaude with the output of Claude Code 2.1.284: the short ID in SGR color codes
// and dim hint lines after it.
func colorClaude(t *testing.T, env *Env, id string, agents []map[string]any) {
	t.Helper()
	fakeClaude(t, env, "\x1b[36m"+id+"\x1b[39m", agents)
	f, err := os.OpenFile(env.ClaudeBin, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("echo \"\x1b[2m  claude agents to see the session\x1b[22m\"\necho \"\x1b[2m  claude stop " + id + "\x1b[22m\"\n"); err != nil {
		t.Fatal(err)
	}
}

func noEscape(t *testing.T, err error) {
	t.Helper()
	if strings.Contains(err.Error(), "\x1b") {
		t.Fatalf("error has escape codes: %q", err)
	}
}

func TestSessionLaunchANSIOutput(t *testing.T) {
	env := testEnv(t, "clanker-bruh")
	writeRoleSettings(t, env, "clerk-bruh-scoutrole")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	colorClaude(t, &env, "b087c156", []map[string]any{
		{"id": "b087c156", "kind": "background", "name": "clerk-bruh-scoutrole", "sessionId": "b087c156-full", "state": "working", "startedAt": 1},
	})
	out, err := call(t, env, "session_launch", map[string]any{"agent": "clerk", "role_key": "clerk-bruh-scoutrole", "cwd": cwd})
	if err != nil {
		t.Fatal(err)
	}
	if out.(map[string]any)["session_id"] != "b087c156-full" {
		t.Fatalf("result = %v", out)
	}
}

func TestSessionResumeANSIOutput(t *testing.T) {
	env := testEnv(t, "clanker-a")
	cwd, _ := filepath.EvalSymlinks(t.TempDir())
	agents := []map[string]any{
		{"id": "5e55a000", "kind": "background", "name": "clerk-a-1", "sessionId": "5e55a000-full", "state": "stopped", "cwd": cwd, "startedAt": 5},
	}
	colorClaude(t, &env, "5e55a000", agents)
	out, err := call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	if err != nil {
		t.Fatal(err)
	}
	if out.(map[string]any)["session_id"] != "5e55a000-full" {
		t.Fatalf("result = %v", out)
	}

	colorClaude(t, &env, "c0b1e000", agents)
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	mustErr(t, err, "started a copy c0b1e000 instead of resuming 5e55a000")
	noEscape(t, err)

	colorClaude(t, &env, "", agents)
	_, err = call(t, env, "session_resume", map[string]any{"role_key": "clerk-a-1"})
	mustErr(t, err, "printed no session ID: Starting background service…\nbackgrounded ·  · x\n")
	noEscape(t, err)
}
