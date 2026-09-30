package main

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func testEnv(t *testing.T, role string) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		DataDir:          t.TempDir(),
		RoleKey:          role,
		PluginRoot:       "..",
		Home:             home,
		SettingsFile:     filepath.Join(home, ".claude", "settings.json"),
		ClaudeBin:        filepath.Join(home, "no-claude-in-tests"),
		PollInterval:     10 * time.Millisecond,
		ProgressInterval: 30 * time.Millisecond,
		Now:              time.Now,
	}
}

// as returns a copy of env with another role key and the same data folder.
func as(env Env, role string) Env {
	env.RoleKey = role
	return env
}

// call runs one tool handler and returns its result decoded from JSON.
func call(t *testing.T, env Env, name string, args any) (any, error) {
	t.Helper()
	for _, tool := range AllTools() {
		if tool.Name != name {
			continue
		}
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		out, err := tool.Handler(&Call{Env: env}, raw)
		if err != nil {
			return nil, err
		}
		if s, ok := out.(string); ok {
			return s, nil
		}
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		return v, nil
	}
	t.Fatalf("unknown tool %s", name)
	return nil, nil
}
