package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	for i, want := range []string{"Q-1", "Q-2"} {
		q, err := openQ(t, env, "merge the fix?")
		if err != nil {
			t.Fatal(err)
		}
		if q["id"] != want || q["header"] != "P1 "+want+": merge the fix?" {
			t.Fatalf("call %d: %v", i, q)
		}
	}
	data, err := os.ReadFile(filepath.Join(env.DataDir, "questions", "Q-2.json"))
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
