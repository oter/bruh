package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestRoleSettingsWrite(t *testing.T) {
	env := testEnv(t, "bigm")
	out, err := call(t, env, "role_settings_write", map[string]any{
		"role_key": "clanker-a",
		"env":      map[string]string{"CODEX_HOME": "/opt/codex"},
		"deny":     []string{"Bash(rm -rf /:*)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out.(map[string]any)["path"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Env         map[string]string `json:"env"`
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Env["BRUH_ROLE_KEY"] != "clanker-a" || s.Env["CODEX_HOME"] != "/opt/codex" {
		t.Fatalf("env = %v", s.Env)
	}
	if !slices.Contains(s.Permissions.Deny, "Bash(docker volume rm:*)") || !slices.Contains(s.Permissions.Deny, "Bash(rm -rf /:*)") {
		t.Fatalf("deny = %v", s.Permissions.Deny)
	}
	_, err = call(t, env, "role_settings_write", map[string]any{"role_key": "x", "env": map[string]string{"BRUH_ROLE_KEY": "bigm"}})
	if err == nil || !strings.Contains(err.Error(), "BRUH_ROLE_KEY") {
		t.Fatalf("err = %v", err)
	}
	_, err = call(t, as(env, "clerk-a-1"), "role_settings_write", map[string]any{"role_key": "clanker-a"})
	mustErr(t, err, "only bigm or a clanker writes role settings")
}
