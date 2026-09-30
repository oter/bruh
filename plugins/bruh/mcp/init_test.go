package main

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// initEnv returns an environment with a temporary plugin root that has the real scripts and
// defaults, the ledger template fixture (a copy of the template of lane roles), and the default
// priorities fixture. It also returns the ledger folder.
func initEnv(t *testing.T) (Env, string) {
	t.Helper()
	env := testEnv(t, "")
	root := t.TempDir()
	copyTree(t, "../scripts", filepath.Join(root, "scripts"))
	copyTree(t, "../defaults", filepath.Join(root, "defaults"))
	copyTree(t, "../.claude-plugin", filepath.Join(root, ".claude-plugin"))
	copyTree(t, "testdata/ledger-template", filepath.Join(root, "ledger-template"))
	b, err := os.ReadFile("testdata/priorities.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "defaults", "priorities.md"), b, 0o600); err != nil {
		t.Fatal(err)
	}
	env.PluginRoot = root
	return env, filepath.Join(t.TempDir(), "ledger")
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func answers(ledger string, extra map[string]any) map[string]any {
	a := map[string]any{"user_name": "Sam", "ledger_path": ledger}
	for k, v := range extra {
		a[k] = v
	}
	return a
}

func plan(t *testing.T, env Env, a map[string]any) map[string]any {
	t.Helper()
	out, err := call(t, env, "init_plan", map[string]any{"answers": a})
	if err != nil {
		t.Fatal(err)
	}
	return out.(map[string]any)
}

func apply(t *testing.T, env Env, p map[string]any) []string {
	t.Helper()
	out, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"]})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, v := range out.(map[string]any)["applied"].([]any) {
		paths = append(paths, v.(string))
	}
	return paths
}

func writeSettings(t *testing.T, env Env, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(env.SettingsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.SettingsFile, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readSettings(t *testing.T, env Env) map[string]any {
	t.Helper()
	var m map[string]any
	readJSON(t, env.SettingsFile, &m)
	return m
}

func TestInitPlanWritesNothingOutsidePlans(t *testing.T) {
	env, ledger := initEnv(t)
	writeSettings(t, env, `{"theme":"dark"}`)
	p := plan(t, env, answers(ledger, nil))
	if p["plan_id"] == "" || !strings.Contains(p["diff"].(string), "+  \"autoCompactWindow\": 550000,") {
		t.Fatalf("plan = %v", p)
	}
	entries, _ := os.ReadDir(env.DataDir)
	if len(entries) != 1 || entries[0].Name() != "init" {
		t.Fatalf("data folder = %v", entries)
	}
	if _, err := os.Stat(ledger); err == nil {
		t.Fatal("init_plan created the ledger")
	}
	if b, _ := os.ReadFile(env.SettingsFile); string(b) != `{"theme":"dark"}` {
		t.Fatalf("settings changed: %s", b)
	}
}

func TestInitApplyWritesPlannedContent(t *testing.T) {
	env, ledger := initEnv(t)
	writeSettings(t, env, `{"theme":"dark","permissions":{"allow":["Bash(ls)"]}}`)
	p := plan(t, env, answers(ledger, map[string]any{
		"mode": "autonomous", "p1_batch_minutes": 30, "channels": []string{"slack", "telegram"},
		"delegated_p1_classes": []string{"Dependency bumps", "Test-only changes"},
		"merge_grants":         []map[string]string{{"repo": "owner/repo", "merger": "clerk-repo-merge", "conditions": "CI green | two rounds"}},
	}))
	applied := apply(t, env, p)
	data, _ := filepath.Abs(env.DataDir)
	for _, want := range []string{env.SettingsFile, filepath.Join(data, "bin", "statusline-tap.sh"), filepath.Join(data, "roles", "bigm.json"), filepath.Join(ledger, "mode.md"), filepath.Join(ledger, "projects", "_template.md")} {
		if !slices.Contains(applied, want) {
			t.Errorf("not applied: %s (applied %v)", want, applied)
		}
	}
	s := readSettings(t, env)
	allow := s["permissions"].(map[string]any)["allow"].([]any)
	if s["theme"] != "dark" || s["autoCompactWindow"] != 550000.0 || allow[0] != "Bash(ls)" || !slices.Contains(allow, any("Workflow(bruh:deliver)")) ||
		!slices.Contains(allow, any("mcp__plugin_bruh_bruh__mail_post")) || slices.Contains(allow, any("mcp__plugin_bruh_bruh__init_apply")) ||
		!slices.Contains(allow, any("mcp__plugin_bruh_slack__post_question")) {
		t.Fatalf("settings = %v", s)
	}
	opts := s["pluginConfigs"].(map[string]any)["bruh@bruh"].(map[string]any)["options"].(map[string]any)
	if opts["user_name"] != "Sam" || opts["handoff_percent"] != 50.0 || opts["max_busy_clerks"] != 8.0 {
		t.Fatalf("options = %v", opts)
	}
	if fi, err := os.Stat(filepath.Join(data, "bin", "statusline-tap.sh")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("tap: %v %v", fi, err)
	}
	var bigm struct {
		Env map[string]string `json:"env"`
	}
	readJSON(t, filepath.Join(data, "roles", "bigm.json"), &bigm)
	if bigm.Env["BRUH_ROLE_KEY"] != "bigm" || bigm.Env["CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS"] != "16" {
		t.Fatalf("bigm env = %v", bigm.Env)
	}
	mode, _ := os.ReadFile(filepath.Join(ledger, "mode.md"))
	for _, want := range []string{"\nmode: autonomous\n", "\nreason: init\n", "\np1_batch_minutes: 30\n", "\np1_batch_size: 5\n", "\nreview_round_cap: 2\n", "\nstatus_cadence: on-change\n", "\nchanged: 20"} {
		if !strings.Contains(string(mode), want) {
			t.Errorf("mode.md has no %q:\n%s", want, mode)
		}
	}
	prio, _ := os.ReadFile(filepath.Join(ledger, "priorities.md"))
	if !strings.Contains(string(prio), "one item for each class. With no items, a clanker answers no P1 question.\n\n- Dependency bumps\n- Test-only changes\n\n## Never without the owner") {
		t.Fatalf("priorities.md:\n%s", prio)
	}
	grants, _ := os.ReadFile(filepath.Join(ledger, "grants.md"))
	if !strings.Contains(string(grants), "|---|\n| owner/repo | clerk-repo-merge | CI green \\| two rounds | CI green \\| two rounds | 20") || !strings.HasSuffix(string(grants), " | init |\n") {
		t.Fatalf("grants.md:\n%s", grants)
	}
	if !strings.Contains(p["launch_command"].(string), "--settings '"+filepath.Join(data, "roles", "bigm.json")+"' --channels plugin:telegram@claude-plugins-official --dangerously-load-development-channels plugin:bruh@bruh") {
		t.Fatalf("launch = %v", p["launch_command"])
	}
	if _, err := os.Stat(filepath.Join(data, "init", "plans", p["plan_id"].(string)+".json")); err == nil {
		t.Fatal("the plan was not removed")
	}
	// A second plan with the same answers changes nothing.
	p2 := plan(t, env, answers(ledger, map[string]any{"mode": "autonomous", "p1_batch_minutes": 30, "channels": []string{"slack", "telegram"}}))
	if p2["diff"] != "" {
		t.Fatalf("second diff:\n%s", p2["diff"])
	}
}

func TestInitApplyRefusesStalePlan(t *testing.T) {
	env, ledger := initEnv(t)
	writeSettings(t, env, `{}`)
	p := plan(t, env, answers(ledger, nil))
	writeSettings(t, env, `{"theme":"light"}`)
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"]})
	mustErr(t, err, "changed after init_plan")
	if b, _ := os.ReadFile(env.SettingsFile); string(b) != `{"theme":"light"}` {
		t.Fatalf("settings = %s", b)
	}
	if _, err := os.Stat(ledger); err == nil {
		t.Fatal("a stale plan wrote the ledger")
	}
	_, err = call(t, env, "init_apply", map[string]any{"plan_id": "../../x"})
	mustErr(t, err, "invalid plan_id")
}

func TestInitPlanWrapsStatusLine(t *testing.T) {
	env, ledger := initEnv(t)
	prev := `printf '%s' "it's $HOME" && jq -r .model.display_name`
	b, _ := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": prev, "padding": 2}})
	writeSettings(t, env, string(b))
	apply(t, env, plan(t, env, answers(ledger, nil)))
	sl := readSettings(t, env)["statusLine"].(map[string]any)
	data, _ := filepath.Abs(env.DataDir)
	tap := filepath.Join(data, "bin", "statusline-tap.sh")
	if sl["command"] != shq(tap)+" "+shq(prev) || sl["padding"] != 2.0 {
		t.Fatalf("statusLine = %v", sl)
	}
	// The wrapped command, run by a shell as Claude Code does, records the context and runs the previous command.
	cmd := exec.Command("sh", "-c", sl["command"].(string))
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=/example-home", "BRUH_ROLE_KEY=bigm"}
	cmd.Stdin = strings.NewReader(`{"session_id":"s-9","model":{"display_name":"Opus"},"context_window":{"used_percentage":12}}`)
	out, err := cmd.Output()
	if err != nil || string(out) != "it's /example-homeOpus\n" {
		t.Fatalf("out = %q, %v", out, err)
	}
	if m := readContext(t, data, "s-9"); m["used_percentage"] != 12.0 {
		t.Fatalf("context = %v", m)
	}
	// A second init does not wrap twice.
	p := plan(t, env, answers(ledger, nil))
	if p["diff"] != "" {
		t.Fatalf("second diff:\n%s", p["diff"])
	}
}

func TestInitPlanNoStatusLineAndNoWrap(t *testing.T) {
	env, ledger := initEnv(t)
	apply(t, env, plan(t, env, answers(ledger, nil)))
	data, _ := filepath.Abs(env.DataDir)
	if c := readSettings(t, env)["statusLine"].(map[string]any)["command"]; c != shq(filepath.Join(data, "bin", "statusline-tap.sh")) {
		t.Fatalf("command = %v", c)
	}
	env2, ledger2 := initEnv(t)
	writeSettings(t, env2, `{"statusLine":{"type":"command","command":"x"}}`)
	apply(t, env2, plan(t, env2, answers(ledger2, map[string]any{"wrap_statusline": false})))
	if c := readSettings(t, env2)["statusLine"].(map[string]any)["command"]; c != "x" {
		t.Fatalf("command = %v", c)
	}
	if _, err := os.Stat(filepath.Join(env2.DataDir, "bin")); err == nil {
		t.Fatal("tap copied without wrap")
	}
}

func TestInitPlanKeepsOtherKeys(t *testing.T) {
	env, ledger := initEnv(t)
	old := "{\n  \"z\": 1,\n  \"hooks\": {\"Stop\": []},\n  \"env\": {\"A\": \"x && y\"}\n}\n"
	writeSettings(t, env, old)
	apply(t, env, plan(t, env, answers(ledger, nil)))
	b, _ := os.ReadFile(env.SettingsFile)
	if !strings.HasPrefix(string(b), "{\n  \"z\": 1,\n  \"hooks\": {\n    \"Stop\": []\n  },\n  \"env\": {\n    \"A\": \"x && y\"\n  },\n  \"autoCompactWindow\": 550000,") {
		t.Fatalf("settings:\n%s", b)
	}
}

func TestInitKeepsSymlinkedSettings(t *testing.T) {
	env, ledger := initEnv(t)
	real := filepath.Join(t.TempDir(), "real.json")
	if err := os.WriteFile(real, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(env.SettingsFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, env.SettingsFile); err != nil {
		t.Fatal(err)
	}
	apply(t, env, plan(t, env, answers(ledger, nil)))
	if fi, err := os.Lstat(env.SettingsFile); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the symbolic link was replaced")
	}
	if b, _ := os.ReadFile(real); !strings.Contains(string(b), "autoCompactWindow") {
		t.Fatalf("real file = %s", b)
	}
}

func TestInitLedgerKeepsExistingFiles(t *testing.T) {
	env, ledger := initEnv(t)
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ledger, "README.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	applied := apply(t, env, plan(t, env, answers(ledger, nil)))
	if slices.Contains(applied, filepath.Join(ledger, "README.md")) || !slices.Contains(applied, filepath.Join(ledger, "rules.md")) {
		t.Fatalf("applied = %v", applied)
	}
	if b, _ := os.ReadFile(filepath.Join(ledger, "README.md")); string(b) != "mine\n" {
		t.Fatalf("README = %q", b)
	}
}

func TestInitPlanValidates(t *testing.T) {
	env, ledger := initEnv(t)
	for _, c := range []struct {
		a    map[string]any
		want string
	}{
		{map[string]any{"ledger_path": ledger}, "user_name"},
		{answers("rel/path", nil), "absolute"},
		{answers(ledger, map[string]any{"mode": "wild"}), "mode"},
		{answers(ledger, map[string]any{"auto_compact_window": 50}), "auto_compact_window"},
		{answers(ledger, map[string]any{"handoff_percent": 100}), "handoff_percent"},
		{answers(ledger, map[string]any{"channels": []string{"discord"}}), "channels"},
		{answers(ledger, map[string]any{"merge_grants": []map[string]string{{"repo": "x", "merger": "clanker-a", "conditions": ""}}}), "merge_grants"},
		{answers(ledger, map[string]any{"user_nmae": "typo"}), "unknown field"},
	} {
		_, err := call(t, env, "init_plan", map[string]any{"answers": c.a})
		mustErr(t, err, c.want)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "init")); err == nil {
		t.Fatal("a refused plan was stored")
	}
}
