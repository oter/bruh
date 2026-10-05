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
				Type        string   `json:"type"`
				Command     string   `json:"command"`
				Args        []string `json:"args"`
				AsyncRewake bool     `json:"asyncRewake"`
				Timeout     int      `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	readJSON(t, "../hooks/hooks.json", &h)
	// Each group: the matcher, the script, and whether it is the waiter of spec 9.5 (M1), which
	// runs with asyncRewake and a timeout above its own limit of 3300 seconds.
	type group struct {
		matcher, script string
		waiter          bool
	}
	want := map[string][]group{
		"PostToolUse":      {{"", "handoff-nudge.sh", false}},
		"SessionStart":     {{"compact|clear|resume", "handoff-inject.sh", false}, {"startup|resume|compact", "wake.sh", true}},
		"Stop":             {{"", "wake.sh", true}, {"", "refusal-stop.sh", false}},
		"PreToolUse":       {{"Bash", "lease-guard.sh", false}, {"", "refusal-stop.sh", false}},
		"PermissionDenied": {{"", "refusal-stop.sh", false}},
	}
	if len(h.Hooks) != len(want) {
		t.Fatalf("events = %v", h.Hooks)
	}
	for ev, ws := range want {
		groups := h.Hooks[ev]
		if len(groups) != len(ws) {
			t.Fatalf("%s = %+v", ev, groups)
		}
		for i, w := range ws {
			if groups[i].Matcher != w.matcher || len(groups[i].Hooks) != 1 {
				t.Fatalf("%s group %d = %+v", ev, i, groups[i])
			}
			hk := groups[i].Hooks[0]
			if hk.Type != "command" || hk.Command != "sh" || len(hk.Args) != 1 || hk.Args[0] != "${CLAUDE_PLUGIN_ROOT}/scripts/"+w.script ||
				hk.AsyncRewake != w.waiter || (w.waiter && hk.Timeout != 3600) {
				t.Fatalf("%s hook = %+v", ev, hk)
			}
			if _, err := os.Stat("../scripts/" + w.script); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// refusal runs refusal-stop.sh with the hook input in and returns its output.
func refusal(t *testing.T, data string, in map[string]any, env ...string) string {
	t.Helper()
	b, _ := json.Marshal(in)
	out, code := runScript(t, "refusal-stop.sh", string(b), append([]string{"CLAUDE_PLUGIN_DATA=" + data}, env...)...)
	if code != 0 {
		t.Fatalf("refusal-stop.sh with input %s: exit %d, want 0; output %q", b, code, out)
	}
	return out
}

func preTool(sid, tool, command, cwd string) map[string]any {
	return map[string]any{"hook_event_name": "PreToolUse", "session_id": sid, "cwd": cwd, "tool_name": tool, "tool_input": map[string]string{"command": command}}
}

// denyReason returns the reason of a PreToolUse deny, or "" when out allows the call.
func denyReason(t *testing.T, out string) string {
	t.Helper()
	if out == "" {
		return ""
	}
	var m struct {
		H struct {
			Event    string `json:"hookEventName"`
			Decision string `json:"permissionDecision"`
			Reason   string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &m); err != nil || m.H.Event != "PreToolUse" || m.H.Decision != "deny" || m.H.Reason == "" {
		t.Fatalf("output = %q", out)
	}
	return m.H.Reason
}

// holdFiles returns the hold records of the data folder.
func holdFiles(t *testing.T, data string) []map[string]any {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(data, "holds", "H-*.json"))
	var out []map[string]any
	for _, f := range files {
		var m map[string]any
		readJSON(t, f, &m)
		out = append(out, m)
	}
	return out
}

const refusalKey = "BRUH_ROLE_KEY=clerk-a-1"

func denyClassifier(t *testing.T, data string) string {
	t.Helper()
	refusal(t, data, map[string]any{
		"hook_event_name": "PermissionDenied", "session_id": "S", "agent_id": "A", "tool_name": "Bash",
		"tool_input": map[string]string{"command": "gh pr merge 7"}, "tool_use_id": "tu-1",
		"reason": "[Merge Without Review]",
	}, refusalKey)
	holds := holdFiles(t, data)
	if len(holds) != 1 {
		t.Fatalf("holds = %v", holds)
	}
	return holds[0]["id"].(string)
}

func TestRefusalHoldStopsTheSession(t *testing.T) {
	data := t.TempDir()
	id := denyClassifier(t, data)
	h := holdFiles(t, data)[0]
	if h["session_id"] != "S" || h["role_key"] != "clerk-a-1" || h["tool_name"] != "Bash" || h["denial_source"] != "permission_denied" ||
		h["denial_reason"] != "[Merge Without Review]" || h["question_id"] != "" || len(h) != 9 {
		t.Fatalf("hold = %v", h)
	}
	if _, err := time.Parse(stampLayout, h["at"].(string)); err != nil || !holdRE.MatchString(id) {
		t.Fatalf("at = %v, id = %q", h["at"], id)
	}
	// Another command of S, also from a subagent and with another tool: denied with the hold ID.
	for _, in := range []map[string]any{preTool("S", "Bash", "gh api -X PUT repos/o/r/pulls/7/merge", ""), preTool("S", "Write", "", "")} {
		in["agent_id"] = "B"
		if r := denyReason(t, refusal(t, data, in, refusalKey)); !strings.Contains(r, id) || !strings.Contains(r, "question_open") {
			t.Fatalf("reason = %q", r)
		}
	}
	for _, tool := range []string{"mcp__plugin_bruh_bruh__question_open", "mcp__plugin_bruh_bruh__answer_wait", "mcp__plugin_bruh_bruh__mail_post",
		"mcp__plugin_bruh_bruh__mail_read", "SendMessage", "ToolSearch", "StructuredOutput"} {
		if out := refusal(t, data, preTool("S", tool, "", ""), refusalKey); out != "" {
			t.Fatalf("%s denied: %q", tool, out)
		}
	}
	if out := refusal(t, data, preTool("S2", "Bash", "gh pr merge 7", ""), refusalKey); out != "" {
		t.Fatalf("other session denied: %q", out)
	}
	// answer_write: while a hold exists, only bigm may call it, also for the linked P0.
	answerWrite := func(qid any) map[string]any {
		in := preTool("S", "mcp__plugin_bruh_bruh__answer_write", "", "")
		in["tool_input"] = map[string]any{"question_id": qid, "text": "t"}
		return in
	}
	if denyReason(t, refusal(t, data, answerWrite("Q-x-1"), refusalKey)) == "" {
		t.Fatal("answer_write of a held clerk allowed before the P0")
	}
	if out := refusal(t, data, answerWrite("Q-x-1"), "BRUH_ROLE_KEY=bigm"); out != "" {
		t.Fatalf("answer_write of bigm denied: %q", out)
	}
	env := testEnv(t, "clerk-a-1")
	env.DataDir = data
	q, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": id})
	if err != nil {
		t.Fatal(err)
	}
	if denyReason(t, refusal(t, data, answerWrite(q.(map[string]any)["id"]), refusalKey)) == "" {
		t.Fatal("answer_write of the linked P0 by the held clerk allowed")
	}
}

func TestRefusalClearByBigm(t *testing.T) {
	data := t.TempDir()
	id := denyClassifier(t, data)
	env := testEnv(t, "clerk-a-1")
	env.DataDir = data
	q, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": id})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, as(env, "bigm"), "answer_write", map[string]any{"question_id": q.(map[string]any)["id"], "text": "run it. Owner, 2026-10-04."}); err != nil {
		t.Fatal(err)
	}
	if out := refusal(t, data, preTool("S", "Bash", "ls", ""), refusalKey); out != "" {
		t.Fatalf("denied after the clear: %q", out)
	}
}

func TestRefusalNoRoleKey(t *testing.T) {
	data := t.TempDir()
	in := map[string]any{"hook_event_name": "PermissionDenied", "session_id": "S", "tool_name": "Bash", "reason": "[Merge Without Review]"}
	if out := refusal(t, data, in); out != "" {
		t.Fatalf("output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(data, "holds")); err == nil {
		t.Fatal("hold written without BRUH_ROLE_KEY")
	}
	denyClassifier(t, data)
	if out := refusal(t, data, preTool("S", "Bash", "ls", "")); out != "" {
		t.Fatalf("denied without BRUH_ROLE_KEY: %q", out)
	}
}

func TestRefusalStopBlock(t *testing.T) {
	data := t.TempDir()
	stop := map[string]any{"hook_event_name": "Stop", "session_id": "S", "stop_hook_active": false}
	if out := refusal(t, data, stop, refusalKey); out != "" {
		t.Fatalf("block without a hold: %q", out)
	}
	id := denyClassifier(t, data)
	var m struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(refusal(t, data, stop, refusalKey)), &m); err != nil || m.Decision != "block" || !strings.Contains(m.Reason, id) {
		t.Fatalf("refusal-stop.sh Stop output = %+v (unmarshal err %v), want decision \"block\" with reason containing %q", m, err, id)
	}
	if out := refusal(t, data, map[string]any{"hook_event_name": "Stop", "session_id": "S", "stop_hook_active": true}, refusalKey); out != "" {
		t.Fatalf("block with stop_hook_active: %q", out)
	}
	if out := refusal(t, data, map[string]any{"hook_event_name": "Stop", "session_id": "S2"}, refusalKey); out != "" {
		t.Fatalf("block of another session: %q", out)
	}
	env := testEnv(t, "clerk-a-1")
	env.DataDir = data
	if _, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": id}); err != nil {
		t.Fatal(err)
	}
	if out := refusal(t, data, stop, refusalKey); out != "" {
		t.Fatalf("block after the P0: %q", out)
	}
}

// TestStopHooksTogether runs both Stop groups of hooks.json on one data folder: the hold Stop
// block of refusal-stop.sh and the waiter wake.sh (spec 9.5, M1). Each does its own job with
// and without a hold.
func TestStopHooksTogether(t *testing.T) {
	stop := map[string]any{"hook_event_name": "Stop", "session_id": "S", "stop_hook_active": false}
	in, _ := json.Marshal(stop)
	for _, held := range []bool{true, false} {
		data := t.TempDir()
		id := ""
		if held {
			id = denyClassifier(t, data)
		}
		wake := func() (int, string) {
			_, code := runScript(t, "wake.sh", string(in), "CLAUDE_PLUGIN_DATA="+data, refusalKey, "BRUH_WAKE_POLL=1", "BRUH_WAKE_SECONDS=0")
			seen, _ := os.ReadFile(filepath.Join(data, "wake", "clerk-a-1.seen"))
			return code, string(seen)
		}
		box := filepath.Join(data, "mail", "clerk-a-1")
		if err := os.MkdirAll(box, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(box, "1.json"), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
		if code, seen := wake(); code != 2 || seen != "1.json\n" {
			t.Fatalf("held=%v: new mail: exit %d, seen %q", held, code, seen)
		}
		if code, seen := wake(); code != 2 || seen != "1.json\n" {
			t.Fatalf("held=%v: no new mail: exit %d, seen %q", held, code, seen)
		}
		out := refusal(t, data, stop, refusalKey)
		if !held {
			if out != "" {
				t.Fatalf("block without a hold: %q", out)
			}
			continue
		}
		var m struct{ Decision, Reason string }
		if err := json.Unmarshal([]byte(out), &m); err != nil || m.Decision != "block" || !strings.Contains(m.Reason, id) {
			t.Fatalf("refusal-stop.sh Stop output = %+v (unmarshal err %v), want decision \"block\" with reason containing %q", m, err, id)
		}
	}
}

// linkedWorktree makes a repository and a linked worktree of it, and returns both paths.
func linkedWorktree(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	wt := filepath.Join(t.TempDir(), "wt")
	for _, args := range [][]string{
		{"init", "-q", repo},
		{"-C", repo, "-c", "user.name=t", "-c", "user.email=t", "commit", "-q", "--allow-empty", "-m", "x"},
		{"-C", repo, "worktree", "add", "-q", wt},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	return repo, wt
}

func TestGitShapeGuard(t *testing.T) {
	repo, wt := linkedWorktree(t)
	shapes := []string{
		"docker run --rm img $(git ls-files '*.sh')",
		"git ls-files '*.sh' > f; xargs lint < f",
		"git ls-files | xargs lint",
		`node -e 'const a={"c":"git diff"}; console.log(a)'`,
		"mkdir d && cat <<EOF > d/s.py\nprint('git status')\nEOF\ncp d/s.py e.py\npython3 e.py",
		"echo `git rev-parse HEAD`",
		"git log | head",
		"git status; ls",
		`git commit -m "$(cat msg)"`,
		"git diff 2>&1 | head",
		"git status\ngit log",
		`git commit -m "a" && ls`,
		`git log \' ; ls ; echo \'`,
		`git log \" ; ls ; echo \"`,
		"git diff <(git show HEAD:a) b",
	}
	for _, cmd := range shapes {
		data := t.TempDir()
		r := denyReason(t, refusal(t, data, preTool("S", "Bash", cmd, wt), refusalKey))
		holds := holdFiles(t, data)
		if len(holds) != 1 || !strings.Contains(r, holds[0]["id"].(string)) || !strings.Contains(r, "no compound commands with git") ||
			holds[0]["denial_source"] != "bruh-git-shape" {
			t.Errorf("%q: reason = %q, holds = %v", cmd, r, holds)
			continue
		}
		if out := refusal(t, data, preTool("S", "Bash", cmd, repo), refusalKey); denyReason(t, out) == "" || !strings.Contains(out, "hold") {
			t.Errorf("%q: the hold does not reach the next call", cmd)
		}
		// Outside a linked worktree the guard denies nothing.
		other := t.TempDir()
		if out := refusal(t, other, preTool("S", "Bash", cmd, repo), refusalKey); out != "" {
			t.Errorf("%q in the main checkout: %q", cmd, out)
		}
		if out := refusal(t, other, preTool("S", "Bash", cmd, t.TempDir()), refusalKey); out != "" {
			t.Errorf("%q outside a repository: %q", cmd, out)
		}
	}
	data := t.TempDir()
	for _, cmd := range []string{
		"git status", "git -C . log --oneline -5", "echo a; echo b", "ls | wc -l", "legit-tool; x", "cat .gitignore | wc -l",
		// One plain git command: separators only inside quotes or in an fd redirect.
		"git diff A B 2>&1", "git log --format='%h|%s'", `git commit -m "a; b"`, `git commit -m "say \"x; y\""`,
		"git log --format='$(x)'", "git status\n",
	} {
		if out := refusal(t, data, preTool("S", "Bash", cmd, wt), refusalKey); out != "" {
			t.Errorf("%q denied: %q", cmd, out)
		}
	}
	if out := refusal(t, data, preTool("S", "Write", "git a; b", wt), refusalKey); out != "" || len(holdFiles(t, data)) != 0 {
		t.Fatalf("Write denied: %q", out)
	}
}

// TestMonitorsJSON checks the plugin monitor of M2 against the strict entry keys of the docs
// (manifest-reference.md, "monitors"), because claude plugin validate does not read the default
// file monitors/monitors.json.
func TestMonitorsJSON(t *testing.T) {
	var entries []map[string]string
	readJSON(t, "../monitors/monitors.json", &entries)
	if len(entries) != 1 {
		t.Fatalf("entries = %v", entries)
	}
	e := entries[0]
	for k := range e {
		if k != "name" && k != "command" && k != "description" && k != "when" {
			t.Errorf("unknown key %q: the plugin would not load", k)
		}
	}
	if e["name"] != "bruh-poller" || e["description"] == "" || e["when"] != "always" ||
		e["command"] != `sh "${CLAUDE_PLUGIN_ROOT}/scripts/watcher.sh" --data "${CLAUDE_PLUGIN_DATA}"` {
		t.Errorf("entry = %v", e)
	}
}

func TestLeaseGuardMatchesWholeWords(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, nil)
	key := "BRUH_ROLE_KEY=clerk-a-1"
	for _, cmd := range []string{
		"psql -h test-db2 -c 1",
		"make integration-lint",
		`git commit -m "wip; make integration tests faster"`,
		`echo "x|make integration"`,
		`echo 'a; psql -h test-db'`,
		`printf "%s \" ; make integration" x`,
	} {
		if denied(t, guard(t, data, cmd, key)) {
			t.Errorf("denied %q", cmd)
		}
	}
	for _, cmd := range []string{"psql -h test-db", "psql -h test-db\t-c 1", `git commit -m "x" && make integration`, "make integration FOO=1"} {
		if !denied(t, guard(t, data, cmd, key)) {
			t.Errorf("allowed %q", cmd)
		}
	}
}

func TestLeaseGuardTellsBigm(t *testing.T) {
	data := t.TempDir()
	leaseState(t, data, nil)
	out := guard(t, data, "psql -h test-db", "BRUH_ROLE_KEY=bigm")
	if !denied(t, out) || !strings.Contains(out, "bigm does not run commands") || strings.Contains(out, "lease_request") {
		t.Fatalf("output = %q", out)
	}
}
