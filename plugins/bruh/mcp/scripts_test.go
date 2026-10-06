package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	// runs with asyncRewake and a timeout above its own limit of 604500 seconds.
	type group struct {
		matcher, script string
		waiter          bool
	}
	want := map[string][]group{
		"PostToolUse":      {{"", "handoff-nudge.sh", false}},
		"SessionStart":     {{"compact|clear|resume", "handoff-inject.sh", false}, {"startup|resume|compact", "wake.sh", true}},
		"Stop":             {{"", "wake.sh", true}, {"", "refusal-stop.sh", false}},
		"PreToolUse":       {{"", "refusal-stop.sh", false}},
		"PermissionDenied": {{"", "refusal-stop.sh", false}},
		// Task D: a worktree guard refusal reaches only PostToolUseFailure.
		"PostToolUseFailure": {{"Bash|Monitor", "refusal-stop.sh", false}},
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
				hk.AsyncRewake != w.waiter || (w.waiter && hk.Timeout != 604800) {
				t.Fatalf("%s hook = %+v", ev, hk)
			}
			if _, err := os.Stat("../scripts/" + w.script); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestWakeLimitBelowHookTimeout: the default limit of wake.sh, and its fallback for a bad
// BRUH_WAKE_SECONDS, stay below the hook timeout of 604800 seconds that TestHooksJSON checks, so
// the waiter exits 0 in silence before Claude Code kills it (bigm decision 2026-10-06).
func TestWakeLimitBelowHookTimeout(t *testing.T) {
	b, err := os.ReadFile("../scripts/wake.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for prefix, end := range map[string]string{"limit=${BRUH_WAKE_SECONDS:-": "}", "*[!0-9]*) limit=": " "} {
		_, rest, ok := strings.Cut(s, prefix)
		digits, _, _ := strings.Cut(rest, end)
		n, err := strconv.Atoi(digits)
		if !ok || err != nil || n <= 0 || n >= 604800 {
			t.Fatalf("%q: limit %q, want a number below 604800", prefix, digits)
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
	// answer_write: the held clerk may not call it, also for the linked P0; bigm has no deny-all
	// hold, so its answer_write goes through.
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

// guardError is the worktree guard refusal of errors.md ("Command blocked by the worktree
// isolation checks"), as the error of PostToolUseFailure carries it.
func guardError(why string) string {
	return "This session is isolated in the worktree /w, but this command " + why + ". Refusing to run it — a worktree-isolated session's git operations must target its own worktree. Split it into plain, separate commands and run them from /w."
}

func guardFailure(command, err string) map[string]any {
	return map[string]any{"hook_event_name": "PostToolUseFailure", "session_id": "S", "tool_name": "Bash",
		"tool_input": map[string]string{"command": command}, "tool_use_id": "tu-1", "error": err}
}

// TestRefusalWorktreeGuardHolds (task D): a worktree guard refusal of a git command reaches the
// hooks only as PostToolUseFailure, and holds the session like a PermissionDenied: ExitWorktree and
// another form of the read are denied until the answer_write of bigm.
func TestRefusalWorktreeGuardHolds(t *testing.T) {
	data := t.TempDir()
	// The two report-only kinds of spec 15.1.6 and a plain failure write no hold.
	for _, in := range []map[string]any{
		guardFailure(`for o in 1 2; do dd if=$B bs=1 skip=$o count=1; done`, guardError("runs dd with a value computed at runtime inside a construct too complex to verify")),
		guardFailure("cat a > b", guardError("is too complex to verify that it stays inside the worktree")),
		guardFailure("git status", "Exit code 128\nfatal: not a git repository"),
	} {
		refusal(t, data, in, refusalKey)
	}
	if h := holdFiles(t, data); len(h) != 0 {
		t.Fatalf("holds = %v, want none", h)
	}

	refused := guardError("redirects git to the shared checkout via -C")
	refusal(t, data, guardFailure("git -C /main rev-parse --abbrev-ref HEAD", refused), refusalKey)
	holds := holdFiles(t, data)
	if len(holds) != 1 || holds[0]["denial_source"] != "worktree_guard" || holds[0]["denial_reason"] != refused || holds[0]["session_id"] != "S" {
		t.Fatalf("holds = %v", holds)
	}
	id := holds[0]["id"].(string)
	calls := []map[string]any{preTool("S", "ExitWorktree", "", ""), preTool("S", "Bash", "git rev-parse --abbrev-ref HEAD", "/main")}
	for _, in := range calls {
		if r := denyReason(t, refusal(t, data, in, refusalKey)); !strings.Contains(r, id) {
			t.Fatalf("%s: reason = %q, want the hold %s", in["tool_name"], r, id)
		}
	}
	env := testEnv(t, "clerk-a-1")
	env.DataDir = data
	q, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": id})
	if err != nil {
		t.Fatal(err)
	}
	if body := q.(map[string]any)["body"].(string); !strings.Contains(body, "CATEGORY: worktree_guard "+refused) {
		t.Fatalf("body = %q", body)
	}
	for _, in := range calls {
		if denyReason(t, refusal(t, data, in, refusalKey)) == "" {
			t.Fatalf("%s allowed before the answer", in["tool_name"])
		}
	}
	if _, err := call(t, as(env, "bigm"), "answer_write", map[string]any{"question_id": q.(map[string]any)["id"], "text": "ok. Owner, {now}."}); err != nil {
		t.Fatal(err)
	}
	for _, in := range calls {
		if out := refusal(t, data, in, refusalKey); out != "" {
			t.Fatalf("%s denied after the answer: %q", in["tool_name"], out)
		}
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

// TestRefusalHoldScoutTellsParent: a held scout cannot call question_open (scoutDeny), so the hold
// tells it to send DONE: scout <subject> refused to its clanker with mail_post and lets it stop
// (task 22, scout hold deadlock). A task clerk keeps the Stop block.
func TestRefusalHoldScoutTellsParent(t *testing.T) {
	data := t.TempDir()
	const scoutKey = "BRUH_ROLE_KEY=clerk-a-scout3"
	refusal(t, data, map[string]any{
		"hook_event_name": "PermissionDenied", "session_id": "S", "tool_name": "Bash",
		"tool_input": map[string]string{"command": "gh api repos/o/r"}, "reason": "[Scope Escalation]",
	}, scoutKey)
	holds := holdFiles(t, data)
	if len(holds) != 1 || holds[0]["role_key"] != "clerk-a-scout3" {
		t.Fatalf("holds = %v", holds)
	}
	id := holds[0]["id"].(string)
	reportWrite := preTool("S", "mcp__plugin_bruh_bruh__report_write", "", "")
	for _, in := range []map[string]any{preTool("S", "Bash", "ls", ""), reportWrite} {
		if r := denyReason(t, refusal(t, data, in, scoutKey)); !strings.Contains(r, id) || !strings.Contains(r, "mail_post") || !strings.Contains(r, "DONE: scout") {
			t.Fatalf("%s reason = %q", in["tool_name"], r)
		}
	}
	if out := refusal(t, data, preTool("S", "mcp__plugin_bruh_bruh__mail_post", "", ""), scoutKey); out != "" {
		t.Fatalf("mail_post of the held scout denied: %q", out)
	}
	env := testEnv(t, "clerk-a-scout3")
	env.DataDir = data
	if _, err := call(t, env, "mail_post", map[string]any{"to": "clanker-a", "header": "DONE: scout x refused", "body": "gh api repos/o/r: [Scope Escalation]"}); err != nil {
		t.Fatal(err)
	}
	stop := map[string]any{"hook_event_name": "Stop", "session_id": "S", "stop_hook_active": false}
	if out := refusal(t, data, stop, scoutKey); out != "" {
		t.Fatalf("Stop of the held scout blocked: %q", out)
	}
	// The same hold under a task clerk key still blocks the Stop.
	if out := refusal(t, data, stop, refusalKey); out == "" {
		t.Fatal("Stop of a held task clerk not blocked")
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
		if code, seen := wake(); code != 0 || seen != "1.json\n" {
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

// TestRefusalHoldSparesBigm: a refusal never freezes bigm (owner, 2026-10-05). Only the exact
// refused call stays denied; each other call goes through, and the Stop block stays until the P0.
func TestRefusalHoldSparesBigm(t *testing.T) {
	data := t.TempDir()
	const bigmKey = "BRUH_ROLE_KEY=bigm"
	refusal(t, data, map[string]any{
		"hook_event_name": "PermissionDenied", "session_id": "S", "tool_name": "Bash",
		"tool_input": map[string]string{"command": "gh pr merge 7"}, "reason": "[Merge Without Review]",
	}, bigmKey)
	holds := holdFiles(t, data)
	if len(holds) != 1 || holds[0]["role_key"] != "bigm" {
		t.Fatalf("holds = %v", holds)
	}
	id := holds[0]["id"].(string)
	answerWrite := preTool("S", "mcp__plugin_bruh_bruh__answer_write", "", "")
	answerWrite["tool_input"] = map[string]any{"question_id": "Q-x-1", "text": "t"}
	// Same command string in another field shape: not the exact call, so allowed.
	reshaped := preTool("S", "Bash", "", "")
	reshaped["tool_input"] = map[string]any{"command": "gh pr merge 7", "description": "merge"}
	for _, in := range []map[string]any{preTool("S", "Bash", "ls", ""), preTool("S", "Write", "", ""), answerWrite, reshaped} {
		if out := refusal(t, data, in, bigmKey); out != "" {
			t.Fatalf("%v denied: %q", in, out)
		}
	}
	exact := preTool("S", "Bash", "gh pr merge 7", "")
	if r := denyReason(t, refusal(t, data, exact, bigmKey)); !strings.Contains(r, id) || !strings.Contains(r, "other work") {
		t.Fatalf("reason = %q", r)
	}
	stop := map[string]any{"hook_event_name": "Stop", "session_id": "S", "stop_hook_active": false}
	var m struct{ Decision, Reason string }
	if err := json.Unmarshal([]byte(refusal(t, data, stop, bigmKey)), &m); err != nil || m.Decision != "block" || !strings.Contains(m.Reason, id) {
		t.Fatalf("Stop output = %+v (err %v), want a block with %q", m, err, id)
	}
	env := testEnv(t, "bigm")
	env.DataDir = data
	q, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": id})
	if err != nil {
		t.Fatal(err)
	}
	if out := refusal(t, data, stop, bigmKey); out != "" {
		t.Fatalf("block after the P0: %q", out)
	}
	if denyReason(t, refusal(t, data, exact, bigmKey)) == "" {
		t.Fatal("the exact call allowed before the answer")
	}
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": q.(map[string]any)["id"], "text": "run it. Owner, 2026-10-05."}); err != nil {
		t.Fatal(err)
	}
	if out := refusal(t, data, exact, bigmKey); out != "" {
		t.Fatalf("denied after the answer: %q", out)
	}
}

// TestRefusalHoldLargeInput: a tool_input above the argv limits (128 KiB on Linux, about 1 MiB
// on macOS) is still denied, for the deny-all hold of a clerk and for the exact call of bigm.
func TestRefusalHoldLargeInput(t *testing.T) {
	big := map[string]string{"file_path": "/tmp/x", "content": strings.Repeat("a", 2<<20)}
	write := func(event string) map[string]any {
		return map[string]any{"hook_event_name": event, "session_id": "S", "tool_name": "Write", "tool_input": big, "reason": "r"}
	}
	data := t.TempDir()
	denyClassifier(t, data)
	if denyReason(t, refusal(t, data, write("PreToolUse"), refusalKey)) == "" {
		t.Fatal("large Write of a held clerk allowed")
	}
	data = t.TempDir()
	refusal(t, data, write("PermissionDenied"), "BRUH_ROLE_KEY=bigm")
	if denyReason(t, refusal(t, data, write("PreToolUse"), "BRUH_ROLE_KEY=bigm")) == "" {
		t.Fatal("large held call of bigm allowed")
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
		e["command"] != `GOTOOLCHAIN=local go run -C "${CLAUDE_PLUGIN_ROOT}/mcp" . watch --data "${CLAUDE_PLUGIN_DATA}"` {
		t.Errorf("entry = %v", e)
	}
}
