package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResultSaveWritesTheResultOfTheCaller(t *testing.T) {
	env := testEnv(t, "clerk-app-t1")
	result := map[string]any{"status": "done", "workflow": "review-only", "confirmed": []any{}}
	out, err := call(t, env, "result_save", map[string]any{"name": "review-7", "result": result})
	if err != nil {
		t.Fatal(err)
	}
	path := out.(map[string]any)["path"].(string)
	if want := filepath.Join(env.DataDir, "results", "clerk-app-t1", "review-7.json"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil || got["workflow"] != "review-only" {
		t.Fatalf("file = %s, err = %v", raw, err)
	}
	// A second save replaces the file and leaves no temporary file.
	if _, err := call(t, env, "result_save", map[string]any{"name": "review-7", "result": map[string]any{"status": "stopped"}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("folder has %d entries, want 1", len(entries))
	}
	raw, _ = os.ReadFile(path)
	if !strings.Contains(string(raw), `"stopped"`) {
		t.Fatalf("file = %s", raw)
	}
}

func TestResultSaveRefusesBadInput(t *testing.T) {
	env := testEnv(t, "clerk-app-t1")
	for _, args := range []map[string]any{
		{"name": "../x", "result": map[string]any{}},
		{"name": "Review", "result": map[string]any{}},
		{"name": "", "result": map[string]any{}},
		{"name": strings.Repeat("a", 65), "result": map[string]any{}},
		{"name": "ok", "result": "not an object"},
		{"name": "ok"},
	} {
		if _, err := call(t, env, "result_save", args); err == nil {
			t.Errorf("result_save(%v) = nil error", args)
		}
	}
	if entries, _ := os.ReadDir(env.DataDir); len(entries) != 0 {
		t.Fatalf("a refused save wrote %d entries", len(entries))
	}
}

// A manual session of the owner has no role key: its results go to results/owner/,
// so they are in the data folder too (principle 3).
func TestResultSaveWithoutRoleKeyUsesOwner(t *testing.T) {
	env := testEnv(t, "")
	out, err := call(t, env, "result_save", map[string]any{"name": "review-3", "result": map[string]any{"status": "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(env.DataDir, "results", "owner", "review-3.json"); out.(map[string]any)["path"] != want {
		t.Fatalf("path = %v, want %s", out, want)
	}
}
