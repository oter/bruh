package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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

// TestRoleSettingsWriteAllow pins the allow input of role_settings_write (build spec A15, G33):
// only bigm may pass it, only for a clanker key, and each rule must be the read rule of a
// repository of the project in the index under the ledger of <data>/init/config.json.
func TestRoleSettingsWriteAllow(t *testing.T) {
	project := strings.ToLower(t.Name())
	clanker := "clanker-" + project
	repos := t.TempDir()
	alpha := filepath.Join(repos, "alpha")
	readRule := func(path string) string { return "Read(/" + path + "/**)" }
	// setup returns an env of bigm. Its data folder has init/config.json unless noConfig, and
	// the ledger of the config has the index of the project unless noIndex.
	setup := func(t *testing.T, noConfig, noIndex bool) Env {
		t.Helper()
		env := testEnv(t, "bigm")
		ledger := t.TempDir()
		if !noConfig {
			cfg, err := json.Marshal(initConfig{LedgerPath: ledger})
			if err != nil {
				t.Fatalf("json.Marshal(initConfig) error: %v", err)
			}
			dir := filepath.Join(env.DataDir, "init")
			if err := os.MkdirAll(dir, 0o700); err != nil {
				t.Fatalf("os.MkdirAll(%q) error: %v", dir, err)
			}
			if err := os.WriteFile(filepath.Join(dir, "config.json"), cfg, 0o600); err != nil {
				t.Fatalf("os.WriteFile(config.json) error: %v", err)
			}
		}
		if !noIndex {
			tree := &treeFile{Root: repos, Projects: []treeProject{{Key: project}}}
			if _, err := writeLearnFile(filepath.Join(ledger, "learn", "tree.json"), tree); err != nil {
				t.Fatalf("writeLearnFile(tree.json) error: %v", err)
			}
			proj := &projectFile{Key: project, Main: "alpha", Repos: []indexRepo{{Path: "alpha"}, {Path: "beta"}}}
			if _, err := writeLearnFile(filepath.Join(ledger, "learn", "projects", project+".json"), proj); err != nil {
				t.Fatalf("writeLearnFile(%s.json) error: %v", project, err)
			}
		}
		return env
	}

	t.Run("bigm writes a rule of a repository of the project", func(t *testing.T) {
		env := setup(t, false, false)
		rule := readRule(alpha)
		out, err := call(t, env, "role_settings_write", map[string]any{"role_key": clanker, "allow": []string{rule}})
		if err != nil {
			t.Fatalf("role_settings_write(%s, allow %q) error: %v", clanker, rule, err)
		}
		var s struct {
			Permissions struct {
				Allow []string `json:"allow"`
			} `json:"permissions"`
		}
		readJSON(t, out.(map[string]any)["path"].(string), &s)
		if !slices.Contains(s.Permissions.Allow, rule) {
			t.Errorf("role_settings_write(%s): permissions.allow = %q, want it to contain %q", clanker, s.Permissions.Allow, rule)
		}
	})

	t.Run("no allow writes no allow key", func(t *testing.T) {
		env := setup(t, false, false)
		out, err := call(t, env, "role_settings_write", map[string]any{"role_key": clanker})
		if err != nil {
			t.Fatalf("role_settings_write(%s) error: %v", clanker, err)
		}
		var s struct {
			Permissions map[string]any `json:"permissions"`
		}
		readJSON(t, out.(map[string]any)["path"].(string), &s)
		if v, ok := s.Permissions["allow"]; ok {
			t.Errorf("role_settings_write(%s) with no allow: permissions.allow = %v, want no key", clanker, v)
		}
	})

	ruleErr := func(rule string) string {
		return fmt.Sprintf("allow rule %q is not Read(//<path>/**) for a repository of project %s in learn/projects/%s.json", rule, project, project)
	}
	const roleErr = "allow is only for a clanker key, written by bigm"
	outside := readRule(filepath.Join(repos, "gamma"))
	write := "Write(/" + alpha + "/**)"
	oneLevel := "Read(/" + alpha + "/*)"
	relative := "Read(/alpha/**)"
	for _, tt := range []struct {
		name              string
		caller, target    string // "": bigm and the clanker of the project
		rule              string
		noConfig, noIndex bool
		wantErr           string // "": any error
	}{
		{name: "path outside the project", rule: outside, wantErr: ruleErr(outside)},
		{name: "write rule", rule: write, wantErr: ruleErr(write)},
		{name: "one-level glob", rule: oneLevel, wantErr: ruleErr(oneLevel)},
		{name: "relative path", rule: relative, wantErr: ruleErr(relative)},
		{name: "clanker writes for its clerk", caller: clanker, target: "clerk-" + project + "-1", rule: readRule(alpha), wantErr: roleErr},
		{name: "bigm writes for a clerk", target: "clerk-" + project + "-merge", rule: readRule(alpha), wantErr: roleErr},
		{name: "no index file", rule: readRule(alpha), noIndex: true},
		{name: "no init config", rule: readRule(alpha), noConfig: true, wantErr: "no ledger path in <data>/init/config.json; run /bruh:init"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			env := setup(t, tt.noConfig, tt.noIndex)
			caller, target := cmp.Or(tt.caller, "bigm"), cmp.Or(tt.target, clanker)
			_, err := call(t, as(env, caller), "role_settings_write", map[string]any{"role_key": target, "allow": []string{tt.rule}})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("%s writes %s with allow %q: error = %v, want %q", caller, target, tt.rule, err, cmp.Or(tt.wantErr, "an error"))
			}
			// A refused rule must not reach the disk.
			file := filepath.Join(env.DataDir, "roles", target+".json")
			if _, err := os.Stat(file); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s writes %s with allow %q: os.Stat(%q) error = %v, want fs.ErrNotExist", caller, target, tt.rule, file, err)
			}
		})
	}
}
