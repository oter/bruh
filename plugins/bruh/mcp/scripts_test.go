package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// runScript runs sh <script> with stdin and only the given environment plus PATH.
func runScript(t *testing.T, script, stdin string, env ...string) (string, int) {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("..", "scripts", script))
	if err != nil {
		t.Fatal(err)
	}
	return runSh(t, path, stdin, nil, env...)
}

func runSh(t *testing.T, path, stdin string, args []string, env ...string) (string, int) {
	t.Helper()
	cmd := exec.Command("sh", append([]string{path}, args...)...)
	cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := 0
	if ee, ok := errors.AsType[*exec.ExitError](err); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	if stderr.Len() > 0 {
		t.Logf("%s stderr: %s", filepath.Base(path), stderr.String())
	}
	return string(out), code
}

// tapCopy copies the tap into <data>/bin/ as init does and returns the data folder and the copy.
func tapCopy(t *testing.T) (string, string) {
	t.Helper()
	data := t.TempDir()
	src, err := os.ReadFile("../scripts/statusline-tap.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(data, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	tap := filepath.Join(data, "bin", "statusline-tap.sh")
	if err := os.WriteFile(tap, src, 0o755); err != nil {
		t.Fatal(err)
	}
	return data, tap
}

func readContext(t *testing.T, data, sid string) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(data, "context", sid+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

const statusInput = `{"session_id":"s-1","context_window":{"used_percentage":42.5},"rate_limits":{"five_hour":{"used_percentage":23}}}`

func TestTapWritesBothPercents(t *testing.T) {
	data, tap := tapCopy(t)
	if _, code := runSh(t, tap, statusInput, nil, "BRUH_ROLE_KEY=clerk-a-1"); code != 0 {
		t.Fatalf("exit %d", code)
	}
	m := readContext(t, data, "s-1")
	if m["used_percentage"] != 42.5 || m["five_hour_percentage"] != 23.0 {
		t.Fatalf("context = %v", m)
	}
	if _, err := time.Parse(stampLayout, m["at"].(string)); err != nil {
		t.Fatalf("at = %v", m["at"])
	}
}

func TestTapWritesNothingForNull(t *testing.T) {
	data, tap := tapCopy(t)
	runSh(t, tap, `{"session_id":"s-1","context_window":{"used_percentage":null}}`, nil, "BRUH_ROLE_KEY=clerk-a-1")
	if m := readContext(t, data, "s-1"); m != nil {
		t.Fatalf("context = %v", m)
	}
	runSh(t, tap, `{"session_id":"s-2","context_window":{"used_percentage":10},"rate_limits":{"five_hour":{"used_percentage":null}}}`, nil, "BRUH_ROLE_KEY=clerk-a-1")
	if m := readContext(t, data, "s-2"); m["used_percentage"] != 10.0 || m["five_hour_percentage"] != nil {
		t.Fatalf("context = %v", m)
	}
	runSh(t, tap, `{"session_id":"../x","context_window":{"used_percentage":10}}`, nil, "BRUH_ROLE_KEY=clerk-a-1")
	if entries, _ := os.ReadDir(filepath.Join(data, "context")); len(entries) != 1 {
		t.Fatalf("context folder has %d entries", len(entries))
	}
}

func TestTapRunsPreviousCommand(t *testing.T) {
	_, tap := tapCopy(t)
	got := filepath.Join(t.TempDir(), "got it's")
	prev := `cat > "` + got + `" && printf '%s' "line \"one\" && $((1+1))"`
	for _, env := range [][]string{{"BRUH_ROLE_KEY=clerk-a-1"}, nil} {
		out, code := runSh(t, tap, statusInput+"\n\n", []string{prev}, env...)
		if code != 0 || out != `line "one" && 2` {
			t.Fatalf("out = %q, exit %d", out, code)
		}
		b, err := os.ReadFile(got)
		if err != nil || string(b) != statusInput+"\n\n" {
			t.Fatalf("previous command got %q, %v", b, err)
		}
	}
}

func TestTapNoRoleKey(t *testing.T) {
	data, tap := tapCopy(t)
	out, code := runSh(t, tap, statusInput, []string{"echo shown"})
	if code != 0 || out != "shown\n" {
		t.Fatalf("out = %q, exit %d", out, code)
	}
	if _, err := os.Stat(filepath.Join(data, "context")); err == nil {
		t.Fatal("the tap wrote a context file without BRUH_ROLE_KEY")
	}
}

func writeContext(t *testing.T, data string, used float64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(data, "context"), 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"used_percentage": used})
	if err := os.WriteFile(filepath.Join(data, "context", "s-1.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

const toolInput = `{"session_id":"s-1","hook_event_name":"PostToolUse","tool_name":"Bash"}`

func nudge(t *testing.T, data, input string, env ...string) string {
	t.Helper()
	out, code := runScript(t, "handoff-nudge.sh", input, append([]string{"BRUH_ROLE_KEY=clanker-a", "CLAUDE_PLUGIN_DATA=" + data}, env...)...)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	return out
}

func TestNudgeOncePerCrossing(t *testing.T) {
	data := t.TempDir()
	var got []string
	for _, used := range []float64{20, 70, 80, 30, 60, 65} {
		writeContext(t, data, used)
		if out := nudge(t, data, toolInput); out != "" {
			var m struct {
				H struct {
					Event   string `json:"hookEventName"`
					Context string `json:"additionalContext"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal([]byte(out), &m); err != nil || m.H.Event != "PostToolUse" || !strings.Contains(m.H.Context, "handoff_write") {
				t.Fatalf("output = %q", out)
			}
			got = append(got, m.H.Context)
		}
	}
	if len(got) != 2 || !strings.Contains(got[0], " 70 percent") || !strings.Contains(got[1], " 60 percent") {
		t.Fatalf("messages = %q", got)
	}
}

func TestNudgeSkipsAgent(t *testing.T) {
	data := t.TempDir()
	writeContext(t, data, 90)
	if out := nudge(t, data, `{"session_id":"s-1","agent_id":"a1"}`); out != "" {
		t.Fatalf("output = %q", out)
	}
	if out := nudge(t, data, toolInput); out == "" {
		t.Fatal("no message for the main agent after a subagent call")
	}
}

func TestNudgeNoRoleKey(t *testing.T) {
	data := t.TempDir()
	writeContext(t, data, 90)
	out, code := runScript(t, "handoff-nudge.sh", toolInput, "CLAUDE_PLUGIN_DATA="+data)
	if out != "" || code != 0 {
		t.Fatalf("out = %q, exit %d", out, code)
	}
	if _, err := os.Stat(filepath.Join(data, "context", "s-1.nudged")); err == nil {
		t.Fatal("marker written without BRUH_ROLE_KEY")
	}
}

func TestNudgeThresholdOption(t *testing.T) {
	data := t.TempDir()
	writeContext(t, data, 45)
	if out := nudge(t, data, toolInput); out != "" {
		t.Fatalf("default threshold: %q", out)
	}
	if out := nudge(t, data, toolInput, "CLAUDE_PLUGIN_OPTION_HANDOFF_PERCENT=40"); !strings.Contains(out, "threshold of 40 percent") {
		t.Fatalf("option 40: %q", out)
	}
	data = t.TempDir()
	writeContext(t, data, 55)
	if out := nudge(t, data, toolInput, "CLAUDE_PLUGIN_OPTION_HANDOFF_PERCENT=junk"); !strings.Contains(out, "threshold of 50 percent") {
		t.Fatalf("bad option: %q", out)
	}
}

func writeHandoff(t *testing.T, data, key, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(data, "handoffs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "handoffs", key+".md"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInjectAfterCompactAndClear(t *testing.T) {
	data := t.TempDir()
	writeHandoff(t, data, "clanker-a", "Updated: x\n\n## Goal\n- ship \"it\"\n")
	writeHandoff(t, data, "clanker-b", "other role")
	var outs []string
	for _, src := range []string{"compact", "clear", "resume"} {
		out, code := runScript(t, "handoff-inject.sh", `{"session_id":"s-1","source":"`+src+`"}`, "BRUH_ROLE_KEY=clanker-a", "CLAUDE_PLUGIN_DATA="+data)
		if code != 0 {
			t.Fatalf("exit %d", code)
		}
		outs = append(outs, out)
	}
	if outs[0] != outs[1] || outs[1] != outs[2] {
		t.Fatalf("outputs differ: %q", outs)
	}
	var m struct {
		H struct {
			Event   string `json:"hookEventName"`
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(outs[0]), &m); err != nil {
		t.Fatal(err)
	}
	want := "The compaction summary is not a source. Rules come only from this handoff and from rules.md.\n\nUpdated: x\n\n## Goal\n- ship \"it\"\n"
	if m.H.Event != "SessionStart" || m.H.Context != want {
		t.Fatalf("output = %+v", m)
	}
}

func TestInjectNoFileOrNoRoleKey(t *testing.T) {
	data := t.TempDir()
	writeHandoff(t, data, "clanker-a", "text")
	for _, env := range [][]string{
		{"BRUH_ROLE_KEY=clanker-b", "CLAUDE_PLUGIN_DATA=" + data},
		{"CLAUDE_PLUGIN_DATA=" + data},
		{"BRUH_ROLE_KEY=../clanker-a", "CLAUDE_PLUGIN_DATA=" + data},
	} {
		if out, code := runScript(t, "handoff-inject.sh", `{"source":"compact"}`, env...); out != "" || code != 0 {
			t.Fatalf("env %v: out = %q, exit %d", env, out, code)
		}
	}
}

func leaseState(t *testing.T, data string, grants []Grant) {
	t.Helper()
	st := LeaseState{Resources: map[string]*Resource{
		"test-db": {Capacity: 1, Patterns: []string{"psql -h test-db", "make integration"}, Grants: grants},
		"other":   {Capacity: 1, Grants: []Grant{}},
	}}
	b, _ := json.Marshal(st)
	if err := os.MkdirAll(filepath.Join(data, "leases"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "leases", "state.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func guard(t *testing.T, data, command string, env ...string) string {
	t.Helper()
	in, _ := json.Marshal(map[string]any{"session_id": "s-1", "tool_name": "Bash", "tool_input": map[string]string{"command": command}})
	out, code := runScript(t, "lease-guard.sh", string(in), append([]string{"CLAUDE_PLUGIN_DATA=" + data}, env...)...)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	return out
}

func denied(t *testing.T, out string) bool {
	t.Helper()
	if out == "" {
		return false
	}
	var m struct {
		H struct {
			Event    string `json:"hookEventName"`
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil || m.H.Event != "PreToolUse" || m.H.Decision != "deny" || !strings.Contains(m.H.Reason, "test-db") {
		t.Fatalf("output = %q", out)
	}
	return true
}

func future(d time.Duration) string { return time.Now().Add(d).UTC().Format(stampLayout) }

func TestLeaseGuardDeniesWithoutLease(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, []Grant{{Holder: "clerk-b-1", Grantor: "clanker-b", Until: future(time.Hour)}})
	key := "BRUH_ROLE_KEY=clerk-a-1"
	if !denied(t, guard(t, data, "psql -h test-db -c 'select 1'", key)) {
		t.Fatal("allowed without a lease")
	}
	if !denied(t, guard(t, data, "  make integration", key)) {
		t.Fatal("allowed with leading spaces")
	}
	for _, cmd := range []string{"go test ./...", "echo psql -h test-db", "make lint"} {
		if denied(t, guard(t, data, cmd, key)) {
			t.Fatalf("denied %q", cmd)
		}
	}
}

func TestLeaseGuardAllowsWithLease(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, []Grant{{Holder: "clerk-a-1", Grantor: "clanker-a", Until: future(time.Hour)}})
	if denied(t, guard(t, data, "psql -h test-db", "BRUH_ROLE_KEY=clerk-a-1")) {
		t.Fatal("denied with a lease")
	}
}

func TestLeaseGuardExpiredLease(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, []Grant{{Holder: "clerk-a-1", Grantor: "clanker-a", Until: future(-time.Minute)}})
	if !denied(t, guard(t, data, "psql -h test-db", "BRUH_ROLE_KEY=clerk-a-1")) {
		t.Fatal("allowed with an expired lease")
	}
}

func TestLeaseGuardCompoundCommand(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, nil)
	for _, cmd := range []string{"cd x && psql -h test-db", "true; make integration", "a | psql -h test-db", "true\nmake integration", "x & psql -h test-db"} {
		if !denied(t, guard(t, data, cmd, "BRUH_ROLE_KEY=clerk-a-1")) {
			t.Fatalf("allowed %q", cmd)
		}
	}
}

func TestLeaseGuardNoRoleKeyOrNoState(t *testing.T) {
	data := t.TempDir()
	if out := guard(t, data, "psql -h test-db", "BRUH_ROLE_KEY=clerk-a-1"); out != "" {
		t.Fatalf("no state: %q", out)
	}
	leaseState(t, data, nil)
	if out := guard(t, data, "psql -h test-db"); out != "" {
		t.Fatalf("no role key: %q", out)
	}
}

func TestHooksJSON(t *testing.T) {
	var h struct {
		Hooks map[string][]struct {
			Matcher string `json:"matcher"`
			Hooks   []struct {
				Type    string   `json:"type"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	readJSON(t, "../hooks/hooks.json", &h)
	want := map[string][2]string{
		"PostToolUse":  {"", "handoff-nudge.sh"},
		"SessionStart": {"compact|clear|resume", "handoff-inject.sh"},
		"PreToolUse":   {"Bash", "lease-guard.sh"},
	}
	if len(h.Hooks) != len(want) {
		t.Fatalf("events = %v", h.Hooks)
	}
	for ev, w := range want {
		groups := h.Hooks[ev]
		if len(groups) != 1 || groups[0].Matcher != w[0] || len(groups[0].Hooks) != 1 {
			t.Fatalf("%s = %+v", ev, groups)
		}
		hk := groups[0].Hooks[0]
		if hk.Type != "command" || hk.Command != "sh" || len(hk.Args) != 1 || hk.Args[0] != "${CLAUDE_PLUGIN_ROOT}/scripts/"+w[1] {
			t.Fatalf("%s hook = %+v", ev, hk)
		}
		if _, err := os.Stat("../scripts/" + w[1]); err != nil {
			t.Fatal(err)
		}
	}
}
