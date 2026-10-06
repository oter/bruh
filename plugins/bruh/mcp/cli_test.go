package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCLIInit(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(t.TempDir(), "answers.json")
	root := repoRoot(t, "app")
	b, _ := json.Marshal(answers(ledger, map[string]any{"channels": []string{"telegram"}, "root": root, "projects": []any{project("app", "app", "app")}}))
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"init", "--answers", file}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	s := out.String()
	for _, want := range []string{"--- /dev/null\n+++ " + filepath.Join(ledger, "mode.md"), "+  \"autoCompactWindow\": 550000", "applied: " + env.SettingsFile,
		"applied: " + filepath.Join(ledger, "learn", "tree.json"), "start bigm with: cd '" + ledger + "'"} {
		if !strings.Contains(s, want) {
			t.Errorf("output has no %q:\n%s", want, s)
		}
	}
	if _, err := os.Stat(filepath.Join(ledger, "grants.md")); err != nil {
		t.Fatal(err)
	}
}

func TestCLIInitEnvAnswers(t *testing.T) {
	env, ledger := initEnv(t)
	a, err := cliAnswers("", []string{"BRUH_INIT_USER_NAME=Sam Lee", "BRUH_INIT_LEDGER_PATH=" + ledger, "BRUH_INIT_HANDOFF_PERCENT=40",
		`BRUH_INIT_CHANNELS=["slack"]`, "BRUH_INIT_WRAP_STATUSLINE=false", "BRUH_INIT_ROOT=/x", "OTHER=1"})
	if err != nil {
		t.Fatal(err)
	}
	// The CLI keeps user_name and root as text and sets the plugin options (spec 16).
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		UserName       string   `json:"user_name"`
		Root           string   `json:"root"`
		HandoffPercent int      `json:"handoff_percent"`
		Channels       []string `json:"channels"`
		WrapStatusline *bool    `json:"wrap_statusline"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.UserName != "Sam Lee" || got.Root != "/x" || got.HandoffPercent != 40 || len(got.Channels) != 1 || got.Channels[0] != "slack" || got.WrapStatusline == nil || *got.WrapStatusline {
		t.Fatalf("cliAnswers = %s, want user_name Sam Lee, root /x, handoff_percent 40, channels [slack], wrap_statusline false", b)
	}
	if _, err := cliAnswers("", []string{"BRUH_INIT_HANDOFF_PERCENT=forty"}); err == nil {
		t.Fatal("no error for a bad value")
	}
	if _, err := cliAnswers("", []string{"BRUH_INIT_NO_SUCH_KEY=1"}); err == nil {
		t.Fatal("no error for an unknown key")
	}
	var out, errOut bytes.Buffer
	t.Setenv("BRUH_INIT_USER_NAME", "Sam")
	t.Setenv("BRUH_INIT_LEDGER_PATH", ledger)
	if code := runCLI([]string{"init"}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
}

func TestCLIInitWritesGivenPluginOptions(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(t.TempDir(), "answers.json")
	b, _ := json.Marshal(map[string]any{"ledger_path": ledger, "handoff_percent": 40})
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"init", "--answers", file}, env, &out, &errOut); code != 0 {
		t.Fatalf("bruh init --answers %s: exit %d: %s", b, code, errOut.String())
	}
	var s struct {
		PluginConfigs map[string]struct {
			Options json.RawMessage `json:"options"`
		} `json:"pluginConfigs"`
	}
	readJSON(t, env.SettingsFile, &s)
	var opts any
	if err := json.Unmarshal(s.PluginConfigs[pluginID].Options, &opts); err != nil {
		t.Fatalf("bruh init --answers %s: pluginConfigs = %+v: %v", b, s.PluginConfigs, err)
	}
	got, _ := json.Marshal(opts)
	if want := `{"handoff_percent":40}`; string(got) != want {
		t.Errorf("bruh init --answers %s: options = %s, want %s", b, got, want)
	}
}

func TestCLIRoleSettings(t *testing.T) {
	env := testEnv(t, "")
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"role-settings", "clanker-remote-app"}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	path := strings.TrimSpace(out.String())
	var s struct {
		Env         map[string]string `json:"env"`
		Permissions struct {
			Deny []any `json:"deny"`
		} `json:"permissions"`
	}
	readJSON(t, path, &s)
	if s.Env["BRUH_ROLE_KEY"] != "clanker-remote-app" || len(s.Permissions.Deny) == 0 {
		t.Fatalf("settings = %+v", s)
	}
	for _, rule := range mailRules(t, env) { // task 31
		if !slices.Contains(s.Permissions.Deny, rule) {
			t.Errorf("deny = %v, want it to contain %q", s.Permissions.Deny, rule)
		}
	}
	if err := os.WriteFile(path, []byte(`{"mine":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	errOut.Reset()
	if code := runCLI([]string{"role-settings", "clanker-remote-app"}, env, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "not overwritten") {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	if b, _ := os.ReadFile(path); string(b) != `{"mine":true}` {
		t.Fatalf("overwritten: %s", b)
	}
	for _, key := range []string{"bigm", "clerk-remote-app-t1", "clerk-ledger"} {
		errOut.Reset()
		if code := runCLI([]string{"role-settings", key}, env, &out, &errOut); code != 1 || !strings.Contains(errOut.String(), "only clanker keys") {
			t.Fatalf("%s: exit %d, %s", key, code, errOut.String())
		}
	}
	if code := runCLI([]string{"role-settings", "../x"}, env, &out, &errOut); code != 1 {
		t.Fatalf("bad key: exit %d", code)
	}
	if code := runCLI([]string{"nope"}, env, &out, &errOut); code != 2 {
		t.Fatalf("unknown command: exit %d", code)
	}
}

func TestCLIEnvDefaultDataDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("BRUH_DATA", "")
	t.Setenv("BRUH_PLUGIN_ROOT", home)
	if pluginID != "bruh@oter" {
		t.Fatalf("pluginID = %q, want bruh@oter", pluginID)
	}
	if got, want := cliEnv().DataDir, filepath.Join(home, ".claude", "plugins", "data", "bruh-oter"); got != want {
		t.Errorf("DataDir = %q, want %q", got, want)
	}
}
