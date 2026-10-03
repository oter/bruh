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
