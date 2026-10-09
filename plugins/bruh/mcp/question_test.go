package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func openQ(t *testing.T, env Env, subject string) (map[string]any, error) {
	t.Helper()
	out, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": subject, "body": "which one?", "blocks": "stage 2"})
	if err != nil {
		return nil, err
	}
	return out.(map[string]any), nil
}

func TestQuestionOpenNumbersInOrder(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	for i, want := range []string{"Q-a-testhost-1", "Q-a-testhost-2"} {
		q, err := openQ(t, env, "merge the fix?")
		if err != nil {
			t.Fatal(err)
		}
		if q["id"] != want || q["header"] != "P1 "+want+": merge the fix?" {
			t.Fatalf("call %d: %v", i, q)
		}
	}
	data, err := os.ReadFile(filepath.Join(env.DataDir, "questions", "Q-a-testhost-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	var q Question
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	if q.Asker != "clerk-a-1" || q.Blocks != "stage 2" || !strings.HasSuffix(q.OpenedAt, "Z") {
		t.Fatalf("question = %+v", q)
	}
}

func TestQuestionIDProjectPart(t *testing.T) {
	for caller, want := range map[string]string{
		"bigm":            "Q-bigm-testhost-1",
		"clerk-ledger":    "Q-ledger-testhost-1",
		"clanker-my-app":  "Q-my-app-testhost-1",
		"clerk-my-app-t1": "Q-my-app-testhost-1",
	} {
		env := testEnv(t, caller)
		q, err := openQ(t, env, "s")
		if err != nil {
			t.Errorf("question_open as %s: %v", caller, err)
			continue
		}
		if q["id"] != want {
			t.Errorf("question_open as %s: id = %v, want %s", caller, q["id"], want)
		}
		if _, err := os.Stat(filepath.Join(env.DataDir, "questions", want+".json")); err != nil {
			t.Errorf("question_open as %s: %v", caller, err)
		}
	}
	if got := questionID("clerk-shop-merge", "dev-mac", 12); got != "Q-shop-dev-mac-12" {
		t.Errorf("questionID(clerk-shop-merge, dev-mac, 12) = %q, want Q-shop-dev-mac-12", got)
	}
}

func TestQuestionOpenRefusesBadInput(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	for _, args := range []map[string]any{
		{"priority": "P3", "subject": "s", "body": "b", "blocks": "x"},
		{"priority": "P1", "subject": "", "body": "b", "blocks": "x"},
		{"priority": "P1", "subject": "two\nlines", "body": "b", "blocks": "x"},
		{"priority": "P1", "subject": strings.Repeat("s", 201), "body": "b", "blocks": "x"},
		{"priority": "P1", "subject": "s", "body": "b", "blocks": ""},
	} {
		if _, err := call(t, env, "question_open", args); err == nil {
			t.Errorf("args %v: no error", args)
		}
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "questions", "next")); err == nil {
		t.Fatal("a refused question used a number")
	}
}

func TestQuestionOpenParallelIDsAreUnique(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	var mu sync.Mutex
	ids := map[string]bool{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			q, err := openQ(t, env, "s")
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			ids[q["id"].(string)] = true
			mu.Unlock()
		})
	}
	wg.Wait()
	if len(ids) != 20 {
		t.Fatalf("%d unique IDs", len(ids))
	}
}

func TestBruhInfo(t *testing.T) {
	env := testEnv(t, "")
	out, err := call(t, env, "bruh_info", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	info := out.(map[string]any)
	root, _ := filepath.Abs("..")
	if info["plugin_root"] != root || info["data_dir"] != env.DataDir || info["role_key"] != "" || info["version"] == "" {
		t.Fatalf("info = %v", info)
	}
}

func TestLeaseDefinePatterns(t *testing.T) {
	bigm := testEnv(t, "bigm")
	def := func(args map[string]any) error {
		_, err := call(t, bigm, "lease_define", args)
		return err
	}
	if err := def(map[string]any{"resource": "db", "capacity": 1, "patterns": []string{"psql -h test-db", "make integration"}}); err != nil {
		t.Fatal(err)
	}
	if err := def(map[string]any{"resource": "db", "capacity": 2}); err != nil {
		t.Fatal(err)
	}
	mustErr(t, def(map[string]any{"resource": "db", "capacity": 1, "patterns": []string{"a\nb"}}), "pattern")
	mustErr(t, def(map[string]any{"resource": "db", "capacity": 1, "patterns": []string{" "}}), "pattern")
	mustErr(t, def(map[string]any{"resource": "db", "capacity": 1, "patterns": []string{"  psql"}}), "pattern")
	st, _ := call(t, bigm, "lease_list", map[string]any{})
	r := st.(map[string]any)["resources"].(map[string]any)["db"].(map[string]any)
	if r["capacity"] != 2.0 || len(r["patterns"].([]any)) != 2 {
		t.Fatalf("resource = %v", r)
	}
}

func TestQuestionOpenKeepsOptions(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	opts := []Option{{Label: "A", Description: "first"}, {Label: "B", Description: "second"}}
	out, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": "s", "body": "which one?", "blocks": "stage 2", "options": opts})
	if err != nil {
		t.Fatal(err)
	}
	r := out.(map[string]any)
	if want := "which one?\n\nOPTION 1: A | first\nOPTION 2: B | second"; r["body"] != want {
		t.Errorf("body = %q, want %q", r["body"], want)
	}
	data, err := os.ReadFile(filepath.Join(env.DataDir, "questions", "Q-a-testhost-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var q Question
	if err := json.Unmarshal(data, &q); err != nil {
		t.Fatal(err)
	}
	if q.Body != "which one?" || !slices.Equal(q.Options, opts) {
		t.Errorf("stored body = %q, options = %v, want %q, %v", q.Body, q.Options, "which one?", opts)
	}
}

func TestQuestionOpenRefusesBadOptions(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	two := func(label, desc string) []Option {
		return []Option{{Label: label, Description: desc}, {Label: "B", Description: "second"}}
	}
	five := make([]Option, 5)
	for i := range five {
		five[i] = Option{Label: strconv.Itoa(i + 1), Description: "d"}
	}
	for name, opts := range map[string][]Option{
		"one option":         {{Label: "A", Description: "first"}},
		"five options":       five,
		"label with bar":     two("A|B", "first"),
		"label with newline": two("A\nB", "first"),
		"empty label":        two("", "first"),
		"label of 61":        two(strings.Repeat("l", 61), "first"),
		"description of 201": two("A", strings.Repeat("d", 201)),
	} {
		if _, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": "s", "body": "b", "blocks": "x", "options": opts}); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if files, _ := filepath.Glob(filepath.Join(env.DataDir, "questions", "*.json")); len(files) > 0 {
		t.Errorf("refused questions wrote %v", files)
	}
}

func TestQuestionOpenWithoutOptionsReturnsBody(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	q, err := openQ(t, env, "s")
	if err != nil {
		t.Fatal(err)
	}
	if q["body"] != "which one?" {
		t.Errorf("body = %q, want %q", q["body"], "which one?")
	}
	data, err := os.ReadFile(filepath.Join(env.DataDir, "questions", "Q-a-testhost-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]any
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored["options"]; ok {
		t.Errorf("stored question has options: %v", stored["options"])
	}
}

// writeHold writes a hold record as scripts/refusal-stop.sh does.
func writeHold(t *testing.T, data, id, roleKey, qid string, toolInput any) {
	t.Helper()
	b, _ := json.Marshal(map[string]any{
		"id": id, "session_id": "S", "role_key": roleKey, "tool_name": "Bash", "tool_input": toolInput,
		"denial_source": "permission_denied", "denial_reason": "[Merge Without Review]", "at": "2026-10-04T20:00:00.000Z", "question_id": qid,
	})
	if err := os.MkdirAll(filepath.Join(data, "holds"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(data, "holds", id+".json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestQuestionOpenWithHold(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	writeHold(t, env.DataDir, "H-1", "clerk-a-1", "", map[string]string{"command": "gh pr merge 7 --squash"})
	out, err := call(t, env, "question_open", map[string]any{
		"priority": "P2", "subject": "refused", "blocks": "merge",
		"body": "COMMAND: echo harmless\nCATEGORY: none", "hold": "H-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	q := out.(map[string]any)
	body := q["body"].(string)
	suffix := "\n\nCOMMAND: gh pr merge 7 --squash\nCATEGORY: permission_denied [Merge Without Review]"
	if !strings.HasPrefix(q["header"].(string), "P0 ") || !strings.HasSuffix(body, suffix) {
		t.Fatalf("question_open(hold H-1) = %v, want header prefix \"P0 \" and body suffix %q", q, suffix)
	}
	var h map[string]any
	readJSON(t, filepath.Join(env.DataDir, "holds", "H-1.json"), &h)
	if h["question_id"] != q["id"] || h["session_id"] != "S" {
		t.Fatalf("hold H-1 = %v, want question_id %v and session_id S", h, q["id"])
	}
	var stored Question
	readJSON(t, filepath.Join(env.DataDir, "questions", q["id"].(string)+".json"), &stored)
	if stored.Priority != "P0" || stored.Body != body {
		t.Fatalf("stored question = %+v, want Priority P0 and Body %q", stored, body)
	}
	// One P0 for each hold.
	if _, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "again", "body": "b", "blocks": "x", "hold": "H-1"}); err == nil || !strings.Contains(err.Error(), q["id"].(string)) {
		t.Fatalf("second question_open(hold H-1) err = %v, want an error naming %s", err, q["id"])
	}
}

// mailbox returns the unread messages in the mailbox of key.
func mailbox(t *testing.T, data, key string) []Message {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(data, "mail", key, "*.json"))
	var msgs []Message
	for _, f := range files {
		var m Message
		readJSON(t, f, &m)
		msgs = append(msgs, m)
	}
	return msgs
}

// Task 41 (issue #42): a P0 with a hold goes from question_open straight to the mailbox of the
// parent of the caller and of bigm, once each, with no nudge of the asker. A P0 with no hold
// posts nothing.
func TestQuestionOpenHoldRelaysP0(t *testing.T) {
	for caller, targets := range map[string][]string{
		"clerk-a-1":    {"clanker-a", "bigm"},
		"clanker-a":    {"bigm"},
		"clerk-ledger": {"bigm"},
		"bigm":         nil,
	} {
		env := testEnv(t, caller)
		writeHold(t, env.DataDir, "H-1", caller, "", map[string]string{"command": "git -C /main status"})
		out, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": "H-1"})
		if err != nil {
			t.Fatalf("%s: %v", caller, err)
		}
		q := out.(map[string]any)
		if _, ok := q["relay_error"]; ok {
			t.Errorf("%s: relay_error = %v", caller, q["relay_error"])
		}
		for _, to := range targets {
			msgs := mailbox(t, env.DataDir, to)
			if len(msgs) != 1 || msgs[0].From != caller || msgs[0].Header != q["header"] || msgs[0].Body != q["body"] {
				t.Errorf("%s: mailbox of %s = %+v, want one message from %s with header %q and body %q", caller, to, msgs, caller, q["header"], q["body"])
			}
		}
		if caller == "bigm" {
			if msgs := mailbox(t, env.DataDir, "bigm"); len(msgs) != 0 {
				t.Errorf("bigm: own mailbox = %+v, want none", msgs)
			}
		}
		var stored map[string]any
		readJSON(t, filepath.Join(env.DataDir, "questions", q["id"].(string)+".json"), &stored)
		if stored["hold"] != "H-1" {
			t.Errorf("%s: stored hold = %v, want H-1", caller, stored["hold"])
		}
	}
	env := testEnv(t, "clerk-a-1")
	if _, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "no hold", "body": "b", "blocks": "x"}); err != nil {
		t.Fatal(err)
	}
	if msgs := append(mailbox(t, env.DataDir, "clanker-a"), mailbox(t, env.DataDir, "bigm")...); len(msgs) != 0 {
		t.Fatalf("P0 with no hold posted %+v, want nothing", msgs)
	}
}

func TestQuestionOpenHoldOtherTool(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	writeHold(t, env.DataDir, "H-2", "clerk-a-1", "", map[string]string{"file_path": "/x/y.sh"})
	out, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": "refused", "body": "b", "blocks": "x", "hold": "H-2"})
	if err != nil {
		t.Fatal(err)
	}
	want := "\nCOMMAND: {\"file_path\":\"/x/y.sh\"}\n"
	if body := out.(map[string]any)["body"].(string); !strings.Contains(body, want) {
		t.Fatalf("question_open(hold H-2) body = %q, want it to contain %q", body, want)
	}
}

func TestQuestionOpenHoldRefusals(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	writeHold(t, env.DataDir, "H-3", "clerk-b-1", "", map[string]string{"command": "x"})
	for _, hold := range []string{"H-3", "../H-3", "H-missing", "X-1"} {
		if _, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "s", "body": "b", "blocks": "x", "hold": hold}); err == nil {
			t.Errorf("question_open(hold %q) err = nil, want error", hold)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(env.DataDir, "questions")); len(entries) != 0 {
		t.Fatalf("questions/ entries after refused holds = %v, want none", entries)
	}
}

// openIDs returns the IDs that question_list returns to bigm.
func openIDs(t *testing.T, env Env) []string {
	t.Helper()
	out, err := call(t, as(env, "bigm"), "question_list", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, q := range out.(map[string]any)["questions"].([]any) {
		ids = append(ids, q.(map[string]any)["id"].(string))
	}
	return ids
}

func exists(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Stat(path)
	return err == nil
}

// Task 24: the answer_write of a clanker to a delegated P1 closes it for bigm. Task 41: the
// answer_wait of the clerk gets the answer of the clanker or of bigm, and its own copy first.
func TestDelegatedAnswerClosesQuestion(t *testing.T) {
	clerk := testEnv(t, "clerk-shop-x")
	q, err := openQ(t, clerk, "which port?")
	if err != nil {
		t.Fatal(err)
	}
	p0, err := call(t, clerk, "question_open", map[string]any{"priority": "P0", "subject": "still open", "body": "b", "blocks": "x"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, clerk, "question_open", map[string]any{"priority": "P2", "subject": "a P2", "body": "b", "blocks": "x"}); err != nil {
		t.Fatal(err)
	}
	id, p0id := q["id"].(string), p0.(map[string]any)["id"].(string)
	if got := openIDs(t, clerk); !slices.Equal(got, []string{id, p0id}) {
		t.Fatalf("question_list before the answer = %v, want [%s %s]", got, id, p0id)
	}
	if _, err := call(t, as(clerk, "clanker-shop"), "answer_write", map[string]any{"question_id": id, "text": "8080", "asker": "clerk-shop-x"}); err != nil {
		t.Fatal(err)
	}
	if !exists(t, filepath.Join(clerk.DataDir, "answers", "clanker-shop", id+".answer")) {
		t.Fatalf("no answer file of clanker-shop for %s", id)
	}
	if got := openIDs(t, clerk); !slices.Equal(got, []string{p0id}) {
		t.Fatalf("question_list after the delegated answer = %v, want [%s]", got, p0id)
	}
	wait := func(qid, want string) map[string]any {
		t.Helper()
		out, err := call(t, clerk, "answer_wait", map[string]any{"question_id": qid, "deadline_seconds": 0})
		if err != nil {
			t.Fatal(err)
		}
		m := out.(map[string]any)
		if m["status"] != "answered" || m["text"] != want {
			t.Fatalf("answer_wait(%s) of the clerk = %v, want answered with text %q", qid, m, want)
		}
		return m
	}
	wait(id, "8080")
	if _, err := call(t, as(clerk, "bigm"), "answer_write", map[string]any{"question_id": p0id, "text": "go. Owner.", "subject": "still open", "asker": "clerk-shop-x"}); err != nil {
		t.Fatal(err)
	}
	if m := wait(p0id, "go. Owner."); m["subject"] != "still open" || m["asker"] != "clerk-shop-x" {
		t.Fatalf("answer_wait(%s) of the clerk = %v, want subject and asker of bigm", p0id, m)
	}
	if _, err := call(t, clerk, "answer_write", map[string]any{"question_id": id, "text": "8081"}); err != nil {
		t.Fatal(err)
	}
	wait(id, "8081")
}

// Task 24: a question that a newer one replaces closes with the answer of the newer one.
func TestReplacedQuestionClosesWithItsReplacement(t *testing.T) {
	clerk := testEnv(t, "clerk-a-1")
	writeHold(t, clerk.DataDir, "H-1", "clerk-a-1", "", map[string]string{"command": "gh pr merge 7"})
	q1, err := call(t, clerk, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": "H-1"})
	if err != nil {
		t.Fatal(err)
	}
	id1 := q1.(map[string]any)["id"].(string)
	q2, err := call(t, clerk, "question_open", map[string]any{"priority": "P0", "subject": "refused (attempt 2)", "body": "b", "blocks": "x", "replaces": id1})
	if err != nil {
		t.Fatal(err)
	}
	id2 := q2.(map[string]any)["id"].(string)
	if _, err := call(t, as(clerk, "bigm"), "answer_write", map[string]any{"question_id": id2, "text": "ok. Owner.", "asker": "clerk-a-1"}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{id1, id2} {
		if !exists(t, filepath.Join(clerk.DataDir, "answers", "bigm", id+".answer")) {
			t.Errorf("no answer file of bigm for %s", id)
		}
	}
	if exists(t, filepath.Join(clerk.DataDir, "holds", "H-1.json")) {
		t.Error("hold H-1 of the replaced P0 is still there")
	}
	if got := openIDs(t, clerk); len(got) != 0 {
		t.Fatalf("question_list = %v, want none", got)
	}
	// An answer that the caller wrote already for a replaced question stays as it is.
	q3, err := openQ(t, clerk, "old")
	if err != nil {
		t.Fatal(err)
	}
	id3 := q3["id"].(string)
	if _, err := call(t, as(clerk, "bigm"), "answer_write", map[string]any{"question_id": id3, "text": "first"}); err != nil {
		t.Fatal(err)
	}
	q4, err := call(t, clerk, "question_open", map[string]any{"priority": "P1", "subject": "new", "body": "b", "blocks": "x", "replaces": id3})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, as(clerk, "bigm"), "answer_write", map[string]any{"question_id": q4.(map[string]any)["id"], "text": "second"}); err != nil {
		t.Fatal(err)
	}
	var a answer
	readJSON(t, filepath.Join(clerk.DataDir, "answers", "bigm", id3+".answer"), &a)
	if a.Text != "first" {
		t.Errorf("answer of %s = %q, want first", id3, a.Text)
	}
}

// Task 24: a repeat of the same refusal replaces the open P0 of the first hold.
func TestHoldRepeatReplacesOpenP0(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	open := func(hold string) Question {
		t.Helper()
		out, err := call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "b", "blocks": "x", "hold": hold})
		if err != nil {
			t.Fatal(err)
		}
		var q Question
		readJSON(t, filepath.Join(env.DataDir, "questions", out.(map[string]any)["id"].(string)+".json"), &q)
		return q
	}
	writeHold(t, env.DataDir, "H-1", "clerk-a-1", "", map[string]string{"command": "gh pr merge 7"})
	writeHold(t, env.DataDir, "H-2", "clerk-a-1", "", map[string]string{"command": "gh pr merge 7"})
	writeHold(t, env.DataDir, "H-3", "clerk-a-1", "", map[string]string{"command": "gh pr merge 8"})
	q1, q2, q3 := open("H-1"), open("H-2"), open("H-3")
	if q1.Replaces != "" || q2.Replaces != q1.ID || q3.Replaces != "" {
		t.Fatalf("replaces = %q, %q, %q, want \"\", %s, \"\"", q1.Replaces, q2.Replaces, q3.Replaces, q1.ID)
	}
}

func TestQuestionOpenRefusesBadReplaces(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	other, err := openQ(t, as(env, "clerk-b-1"), "of b")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{other["id"].(string), "Q-a-testhost-99", "../x"} {
		if _, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": "s", "body": "b", "blocks": "x", "replaces": id}); err == nil {
			t.Errorf("question_open(replaces %q) err = nil, want error", id)
		}
	}
	// A hand-edited loop of replaces ends.
	q1, err := openQ(t, env, "one")
	if err != nil {
		t.Fatal(err)
	}
	q2, err := call(t, env, "question_open", map[string]any{"priority": "P1", "subject": "two", "body": "b", "blocks": "x", "replaces": q1["id"]})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(env.DataDir, "questions", q1["id"].(string)+".json")
	var q Question
	readJSON(t, file, &q)
	q.Replaces = q2.(map[string]any)["id"].(string)
	b, _ := json.Marshal(q)
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, as(env, "bigm"), "answer_write", map[string]any{"question_id": q1["id"], "text": "x"}); err != nil {
		t.Fatal(err)
	}
}

// Task 29: question_open replaces {now} in the body with its opened_at time. The COMMAND line of
// a hold keeps its text word for word.
func TestQuestionOpenFillsNow(t *testing.T) {
	env := fixedNow(testEnv(t, "clerk-a-1"))
	out, err := call(t, env, "question_open", map[string]any{
		"priority": "P1", "subject": "pick", "body": "asked {now}", "blocks": "x",
		"options": []map[string]string{{"label": "a", "description": "one"}, {"label": "b", "description": "two"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	q := out.(map[string]any)
	if body, want := q["body"].(string), "asked "+fixedStamp+"\n"; !strings.HasPrefix(body, want) {
		t.Errorf("question_open body = %q, want prefix %q", body, want)
	}
	var stored Question
	readJSON(t, filepath.Join(env.DataDir, "questions", q["id"].(string)+".json"), &stored)
	if stored.Body != "asked "+fixedStamp || stored.OpenedAt != fixedStamp {
		t.Errorf("stored question = %+v, want Body %q and OpenedAt %s", stored, "asked "+fixedStamp, fixedStamp)
	}
	writeHold(t, env.DataDir, "H-1", "clerk-a-1", "", map[string]string{"command": "echo {now}"})
	out, err = call(t, env, "question_open", map[string]any{"priority": "P0", "subject": "refused", "body": "at {now}", "blocks": "x", "hold": "H-1"})
	if err != nil {
		t.Fatal(err)
	}
	if body, want := out.(map[string]any)["body"].(string), "at "+fixedStamp+"\n\nCOMMAND: echo {now}\n"; !strings.HasPrefix(body, want) {
		t.Errorf("question_open(hold) body = %q, want prefix %q", body, want)
	}
}
