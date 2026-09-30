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
	_, err = call(t, env, "role_settings_write", map[string]any{"role_key": "clanker-x", "env": map[string]string{"BRUH_ROLE_KEY": "bigm"}})
	if err == nil || !strings.Contains(err.Error(), "BRUH_ROLE_KEY") {
		t.Fatalf("err = %v", err)
	}
	// Final review m3: a child under another Claude Code config folder runs under another supervisor.
	_, err = call(t, env, "role_settings_write", map[string]any{"role_key": "clanker-x", "env": map[string]string{"CLAUDE_CONFIG_DIR": "/tmp/other"}})
	mustErr(t, err, "CLAUDE_CONFIG_DIR")
	_, err = call(t, as(env, "clerk-a-1"), "role_settings_write", map[string]any{"role_key": "clanker-a"})
	mustErr(t, err, "only its parent")
}

func TestRoleSettingsWriteParentOnly(t *testing.T) {
	env := testEnv(t, "bigm")
	w := func(caller, target string) error {
		_, err := call(t, as(env, caller), "role_settings_write", map[string]any{"role_key": target})
		return err
	}
	for _, c := range []struct {
		caller, target string
		ok             bool
	}{
		{"bigm", "clanker-a", true},
		{"bigm", "clerk-ledger", true},
		{"bigm", "bigm", true},
		{"bigm", "clerk-a-1", false},
		{"bigm", "clerk-a-merge", true},
		{"clanker-a", "clerk-a-1", true},
		{"clanker-a", "clerk-ab-1", false},
		{"clanker-a", "clerk-a-b-1", false},
		{"clanker-a", "clanker-b", false},
		{"clanker-a", "clanker-a", false},
		{"clerk-a-1", "clerk-a-2", false},
		{"clerk-ledger", "clerk-ledger", false},
	} {
		err := w(c.caller, c.target)
		if (err == nil) != c.ok {
			t.Errorf("%s writes %s: err = %v, want ok = %v", c.caller, c.target, err, c.ok)
		}
	}
}

// Final review B1: only bigm may load the Telegram plugin. Its server takes over the one
// getUpdates poller of the bot token, so any other role session would steal the bot from bigm.
func TestRoleSettingsDisableTelegramExceptBigm(t *testing.T) {
	env := testEnv(t, "bigm")
	telegram := func(path string) (bool, bool) {
		t.Helper()
		var s struct {
			EnabledPlugins map[string]bool `json:"enabledPlugins"`
		}
		readJSON(t, path, &s)
		v, ok := s.EnabledPlugins["telegram@claude-plugins-official"]
		return v, ok
	}
	for caller, key := range map[string]string{"bigm": "clanker-a", "clanker-a": "clerk-a-1"} {
		out, err := call(t, as(env, caller), "role_settings_write", map[string]any{"role_key": key})
		if err != nil {
			t.Fatal(err)
		}
		if v, ok := telegram(out.(map[string]any)["path"].(string)); !ok || v {
			t.Errorf("%s: telegram enabled = %v, set = %v; want false", key, v, ok)
		}
	}
	for _, key := range []string{"clerk-ledger", "bigm"} {
		out, err := call(t, env, "role_settings_write", map[string]any{"role_key": key})
		if err != nil {
			t.Fatal(err)
		}
		_, ok := telegram(out.(map[string]any)["path"].(string))
		if want := key != "bigm"; ok != want {
			t.Errorf("%s: telegram entry set = %v, want %v", key, ok, want)
		}
	}
	var out, errOut strings.Builder
	if code := runCLI([]string{"role-settings", "clanker-remote-b"}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if v, ok := telegram(strings.TrimSpace(out.String())); !ok || v {
		t.Errorf("role-settings CLI: telegram enabled = %v, set = %v; want false", v, ok)
	}
}
