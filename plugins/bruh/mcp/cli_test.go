package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInit(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(t.TempDir(), "answers.json")
	b, _ := json.Marshal(answers(ledger, map[string]any{"channels": []string{"telegram"}}))
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runCLI([]string{"init", "--answers", file}, env, &out, &errOut); code != 0 {
		t.Fatalf("exit %d: %s", code, errOut.String())
	}
	s := out.String()
	for _, want := range []string{"--- /dev/null\n+++ " + filepath.Join(ledger, "mode.md"), "+  \"autoCompactWindow\": 550000", "applied: " + env.SettingsFile, "start bigm with: cd '" + ledger + "'"} {
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
		`BRUH_INIT_CHANNELS=["slack"]`, "BRUH_INIT_WRAP_STATUSLINE=false", "OTHER=1"})
	if err != nil {
		t.Fatal(err)
	}
	if a.UserName != "Sam Lee" || a.HandoffPercent != 40 || a.Channels[0] != "slack" || *a.WrapStatusline {
		t.Fatalf("answers = %+v", a)
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
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	readJSON(t, path, &s)
	if s.Env["BRUH_ROLE_KEY"] != "clanker-remote-app" || len(s.Permissions.Deny) == 0 {
		t.Fatalf("settings = %+v", s)
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
	if code := runCLI([]string{"role-settings", "../x"}, env, &out, &errOut); code != 1 {
		t.Fatalf("bad key: exit %d", code)
	}
	if code := runCLI([]string{"nope"}, env, &out, &errOut); code != 2 {
		t.Fatalf("unknown command: exit %d", code)
	}
}
