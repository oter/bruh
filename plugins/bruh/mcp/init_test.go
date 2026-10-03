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
	"time"
)

// initEnv returns an environment with a temporary plugin root that has copies of the real
// scripts, defaults, manifest, and ledger template of the plugin. It also returns the ledger folder.
func initEnv(t *testing.T) (Env, string) {
	t.Helper()
	env := testEnv(t, "")
	root := t.TempDir()
	for _, d := range []string{"scripts", "defaults", ".claude-plugin", "ledger-template"} {
		copyTree(t, filepath.Join("..", d), filepath.Join(root, d))
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
	a := map[string]any{"ledger_path": ledger}
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
	out, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": p["diff_sha256"]})
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
	if entries, _ := os.ReadDir(env.DataDir); len(entries) != 0 {
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
	}))
	applied := apply(t, env, p)
	data, _ := filepath.Abs(env.DataDir)
	for _, want := range []string{env.SettingsFile, filepath.Join(data, "bin", "statusline-tap.sh"), filepath.Join(ledger, ".claude", "settings.json"), filepath.Join(ledger, "mode.md"), filepath.Join(ledger, "projects", "_template.md")} {
		if !slices.Contains(applied, want) {
			t.Errorf("not applied: %s (applied %v)", want, applied)
		}
	}
	// The ledger settings file replaces roles/bigm.json as the start settings of bigm.
	if _, err := os.Stat(filepath.Join(data, "roles", "bigm.json")); err == nil {
		t.Error("init_apply wrote roles/bigm.json, want no such file")
	}
	s := readSettings(t, env)
	allow := s["permissions"].(map[string]any)["allow"].([]any)
	if s["theme"] != "dark" || s["autoCompactWindow"] != 550000.0 || allow[0] != "Bash(ls)" || !slices.Contains(allow, any("Workflow(bruh:deliver)")) ||
		!slices.Contains(allow, any("mcp__plugin_bruh_bruh__mail_post")) || slices.Contains(allow, any("mcp__plugin_bruh_bruh__init_apply")) ||
		!slices.Contains(allow, any("mcp__plugin_bruh_slack__post_question")) {
		t.Fatalf("settings = %v", s)
	}
	// The settings that init writes allow each implement workflow and result_save (fix round 2).
	for _, want := range []string{"Workflow(bruh:deliver)", "Workflow(bruh:tickets)", "Workflow(bruh:implement-tickets)",
		"Workflow(bruh:review-and-fix)", "Workflow(bruh:review-only)", "mcp__plugin_bruh_bruh__result_save"} {
		if !slices.Contains(allow, any(want)) {
			t.Errorf("the written settings do not allow %s", want)
		}
	}
	// The plugin options belong to the install dialog and /config (spec 16): init_plan does not write them.
	if pc, ok := s["pluginConfigs"]; ok {
		t.Errorf("init_apply: settings pluginConfigs = %v, want no pluginConfigs key", pc)
	}
	if fi, err := os.Stat(filepath.Join(data, "bin", "statusline-tap.sh")); err != nil || fi.Mode().Perm() != 0o755 {
		t.Fatalf("tap: %v %v", fi, err)
	}
	ls := ledgerSettings(t, ledger)
	lenv, _ := ls["env"].(map[string]any)
	lperms, _ := ls["permissions"].(map[string]any)
	ldeny, _ := lperms["deny"].([]any)
	if ls["agent"] != "bruh:bigm" || lenv["BRUH_ROLE_KEY"] != "bigm" || lenv["CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS"] != "16" || !slices.Equal(ldeny, defaultDeny(t, env)) {
		t.Fatalf("init_apply: ledger settings = %v, want agent bruh:bigm, BRUH_ROLE_KEY bigm, CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS 16, deny %v", ls, defaultDeny(t, env))
	}
	if fi, err := os.Stat(filepath.Join(ledger, ".claude", "settings.json")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("init_apply: ledger settings file = %v (%v), want mode 0644", fi, err)
	}
	mode, _ := os.ReadFile(filepath.Join(ledger, "mode.md"))
	for _, want := range []string{"\nmode: autonomous\n", "\nreason: init\n", "\np1_batch_minutes: 30\n", "\np1_batch_size: 5\n", "\nreview_round_cap: 2\n", "\nstatus_cadence: on-change\n", "\nchanged: 20"} {
		if !strings.Contains(string(mode), want) {
			t.Errorf("mode.md has no %q:\n%s", want, mode)
		}
	}
	// Init asks no delegated P1 classes and no merge grants (spec 16): both files are copies.
	for rel, src := range map[string]string{"priorities.md": "defaults/priorities.md", "grants.md": "ledger-template/grants.md"} {
		got, _ := os.ReadFile(filepath.Join(ledger, rel))
		want, err := os.ReadFile(filepath.Join(env.PluginRoot, src))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("init_apply: ledger %s = %q, want %s unchanged %q", rel, got, src, want)
		}
	}
	wantLaunch := "cd " + shq(ledger) + " && claude --agent bruh:bigm --name bigm --permission-mode auto --channels plugin:telegram@claude-plugins-official --dangerously-load-development-channels plugin:bruh@bruh"
	if lc := p["launch_command"].(string); lc != wantLaunch {
		t.Fatalf("init_plan launch_command = %q, want %q", lc, wantLaunch)
	}
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": p["diff_sha256"]})
	mustErr(t, err, "no plan")
	var cfg initConfig
	readJSON(t, filepath.Join(data, "init", "config.json"), &cfg)
	if cfg.LedgerPath != ledger {
		t.Fatalf("config = %+v", cfg)
	}
	// A second plan with the same answers changes nothing.
	p2 := plan(t, env, answers(ledger, map[string]any{"mode": "autonomous", "p1_batch_minutes": 30, "channels": []string{"slack", "telegram"}}))
	if p2["diff"] != "" {
		t.Fatalf("second diff:\n%s", p2["diff"])
	}
}

// ledgerSettings reads the start settings of bigm in the ledger folder.
func ledgerSettings(t *testing.T, ledger string) map[string]any {
	t.Helper()
	var m map[string]any
	readJSON(t, filepath.Join(ledger, ".claude", "settings.json"), &m)
	return m
}

// defaultDeny returns the deny rules of defaults/role-settings.json.
func defaultDeny(t *testing.T, env Env) []any {
	t.Helper()
	var d struct {
		Permissions struct {
			Deny []any `json:"deny"`
		} `json:"permissions"`
	}
	readJSON(t, filepath.Join(env.PluginRoot, "defaults", "role-settings.json"), &d)
	return d.Permissions.Deny
}

func TestInitMergesLedgerSettings(t *testing.T) {
	env, ledger := initEnv(t)
	deny := defaultDeny(t, env)
	file := filepath.Join(ledger, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal(map[string]any{"agent": "other", "env": map[string]any{"A": "1"},
		"permissions": map[string]any{"allow": []any{"Read(x)"}, "deny": []any{"Bash(rm:*)", deny[1]}}})
	if err := os.WriteFile(file, old, 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, env, plan(t, env, answers(ledger, nil)))
	s := ledgerSettings(t, ledger)
	lenv, _ := s["env"].(map[string]any)
	perms, _ := s["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	gotDeny, _ := perms["deny"].([]any)
	// The existing rules keep their order; a default rule that is there already is not added again.
	want := append([]any{"Bash(rm:*)", deny[1], deny[0]}, deny[2:]...)
	if s["agent"] != "bruh:bigm" || lenv["A"] != "1" || lenv["BRUH_ROLE_KEY"] != "bigm" || lenv["CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS"] != "16" ||
		!slices.Equal(allow, []any{"Read(x)"}) || !slices.Equal(gotDeny, want) {
		t.Fatalf("init_apply over %s: ledger settings = %v, want agent bruh:bigm, env A 1 plus the defaults, allow [Read(x)], deny %v", old, s, want)
	}
	// A second init changes nothing.
	if d := plan(t, env, answers(ledger, nil))["diff"]; d != "" {
		t.Fatalf("second init_plan: diff = %q, want empty", d)
	}
}

func TestInitEmptyLedgerSettings(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(ledger, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, env, plan(t, env, answers(ledger, nil)))
	s := ledgerSettings(t, ledger)
	lenv, _ := s["env"].(map[string]any)
	perms, _ := s["permissions"].(map[string]any)
	deny, _ := perms["deny"].([]any)
	if s["agent"] != "bruh:bigm" || lenv["BRUH_ROLE_KEY"] != "bigm" || !slices.Equal(deny, defaultDeny(t, env)) {
		t.Fatalf("init_apply over an empty file: ledger settings = %v, want agent bruh:bigm, BRUH_ROLE_KEY bigm, deny %v", s, defaultDeny(t, env))
	}
}

func TestInitRefusesBadLedgerSettings(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(ledger, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ name, text string }{
		{name: "invalid JSON", text: "{"},
		{name: "not an object", text: "[]"},
		{name: "env is not an object", text: `{"env":"x"}`},
		{name: "permissions is not an object", text: `{"permissions":[]}`},
		{name: "deny is not an array", text: `{"permissions":{"deny":{}}}`},
	} {
		t.Run(c.name, func(t *testing.T) {
			if err := os.WriteFile(file, []byte(c.text), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := call(t, env, "init_plan", map[string]any{"answers": answers(ledger, nil)})
			if err == nil || !strings.Contains(err.Error(), "ledger settings file") {
				t.Errorf("init_plan with the ledger settings %s: error = %v, want an error about the ledger settings file", c.text, err)
			}
			if b, _ := os.ReadFile(file); string(b) != c.text {
				t.Errorf("init_plan with the ledger settings %s: file = %s, want it unchanged", c.text, b)
			}
		})
	}
}

func TestInitApplyRefusesStalePlan(t *testing.T) {
	env, ledger := initEnv(t)
	writeSettings(t, env, `{}`)
	p := plan(t, env, answers(ledger, nil))
	writeSettings(t, env, `{"theme":"light"}`)
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": p["diff_sha256"]})
	mustErr(t, err, "changed after init_plan")
	if b, _ := os.ReadFile(env.SettingsFile); string(b) != `{"theme":"light"}` {
		t.Fatalf("settings = %s", b)
	}
	if _, err := os.Stat(ledger); err == nil {
		t.Fatal("a stale plan wrote the ledger")
	}
	_, err = call(t, env, "init_apply", map[string]any{"plan_id": "../../x", "diff_sha256": ""})
	mustErr(t, err, "no plan")
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
		{answers("rel/path", nil), "absolute"},
		{answers(ledger, map[string]any{"mode": "wild"}), "mode"},
		{answers(ledger, map[string]any{"auto_compact_window": 50}), "auto_compact_window"},
		{answers(ledger, map[string]any{"handoff_percent": 100}), "handoff_percent"},
		{answers(ledger, map[string]any{"channels": []string{"discord"}}), "channels"},
		// Spec 16: merge grants, delegated P1 classes, and remote machines are no longer init answers.
		{answers(ledger, map[string]any{"merge_grants": []any{}}), "unknown field"},
		{answers(ledger, map[string]any{"delegated_p1_classes": []any{}}), "unknown field"},
		{answers(ledger, map[string]any{"remote_environments": []any{}}), "unknown field"},
		{answers(ledger, map[string]any{"user_nmae": "typo"}), "unknown field"},
	} {
		_, err := call(t, env, "init_plan", map[string]any{"answers": c.a})
		mustErr(t, err, c.want)
	}
	if _, err := os.Stat(filepath.Join(env.DataDir, "init")); err == nil {
		t.Fatal("a refused plan was stored")
	}
}

// pluginOptionsErr is the refusal of init_plan for an answer that is a plugin option (spec 16).
const pluginOptionsErr = "user_name, handoff_percent, and max_busy_clerks are plugin options: the install dialog asks them, and /config changes them; init_plan does not write them"

func TestInitPlanDoesNotWritePluginOptions(t *testing.T) {
	env, ledger := initEnv(t)
	const configs = `{"bruh@bruh":{"options":{"handoff_percent":55,"max_busy_clerks":30}}}`
	writeSettings(t, env, `{"theme":"dark","pluginConfigs":`+configs+`}`)
	apply(t, env, plan(t, env, answers(ledger, nil)))
	got, err := json.Marshal(readSettings(t, env)["pluginConfigs"])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != configs {
		t.Errorf("init_plan and init_apply: pluginConfigs = %s, want %s unchanged", got, configs)
	}
}

func TestInitPlanRefusesPluginOptionKeys(t *testing.T) {
	for key, val := range map[string]any{"user_name": "Sam", "handoff_percent": 40, "max_busy_clerks": 8} {
		t.Run(key, func(t *testing.T) {
			env, ledger := initEnv(t)
			writeSettings(t, env, `{"theme":"dark"}`)
			out, err := call(t, env, "init_plan", map[string]any{"answers": answers(ledger, map[string]any{key: val})})
			if out != nil || err == nil || err.Error() != pluginOptionsErr {
				t.Fatalf("init_plan with %s: planned %t, error %v, want no plan and error %q", key, out != nil, err, pluginOptionsErr)
			}
			if b, _ := os.ReadFile(env.SettingsFile); string(b) != `{"theme":"dark"}` {
				t.Errorf("init_plan with %s: settings = %s, want them unchanged", key, b)
			}
			if _, err := os.Stat(filepath.Join(env.DataDir, "init")); err == nil {
				t.Errorf("init_plan with %s: a refused plan was stored", key)
			}
			if _, err := os.Stat(ledger); err == nil {
				t.Errorf("init_plan with %s: the ledger was created", key)
			}
		})
	}
}

func TestInitApplyIgnoresPlanFiles(t *testing.T) {
	env, _ := initEnv(t)
	outside := filepath.Join(t.TempDir(), "authorized_keys")
	forged := `{"id":"aaaaaaaaaaaaaaaa","files":[{"path":"` + outside + `","mode":420,"before":"absent","content":"ssh-ed25519 ATTACKER\n"}]}`
	dir, _ := env.Dir("init", "plans")
	if err := os.WriteFile(filepath.Join(dir, "aaaaaaaaaaaaaaaa.json"), []byte(forged), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": "aaaaaaaaaaaaaaaa", "diff_sha256": ""})
	mustErr(t, err, "no plan")
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("a forged plan file was applied")
	}
}

func TestInitApplyNeedsTheShownDiff(t *testing.T) {
	env, ledger := initEnv(t)
	p := plan(t, env, answers(ledger, nil))
	if p["diff_sha256"] != sha256Hex(p["diff"].(string)) {
		t.Fatalf("diff_sha256 = %v", p["diff_sha256"])
	}
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": sha256Hex("other")})
	mustErr(t, err, "diff_sha256")
	if _, err := os.Stat(env.SettingsFile); err == nil {
		t.Fatal("written with a wrong diff hash")
	}
	// A template file that appears after init_plan changes the target state.
	if err := os.WriteFile(filepath.Join(env.PluginRoot, "ledger-template", "extra.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": p["diff_sha256"]})
	mustErr(t, err, "changed after init_plan")
}

func TestInitTargetsAreAClosedList(t *testing.T) {
	env, ledger := initEnv(t)
	for _, bad := range []string{"relative/ledger", ledger + "/../x", "/dev/null/ledger"} {
		_, err := call(t, env, "init_plan", map[string]any{"answers": answers(bad, nil)})
		mustErr(t, err, "ledger_path")
	}
	// A symbolic link inside the ledger that points outside is refused.
	outside := t.TempDir()
	if err := os.MkdirAll(ledger, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(ledger, "projects")); err != nil {
		t.Fatal(err)
	}
	_, err := call(t, env, "init_plan", map[string]any{"answers": answers(ledger, nil)})
	mustErr(t, err, "refuses to write")
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("outside = %v", entries)
	}
}

func TestInitApplyRequiresUserInteraction(t *testing.T) {
	c := startRPC(t, testEnv(t, "bigm"), AllTools())
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	for _, tl := range c.next(t)["result"].(map[string]any)["tools"].([]any) {
		m := tl.(map[string]any)
		meta, _ := m["_meta"].(map[string]any)
		if (m["name"] == "init_apply") != (meta["anthropic/requiresUserInteraction"] == true) {
			t.Errorf("%s _meta = %v", m["name"], meta)
		}
	}
}

func TestInitApplyWritesSettingsLast(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores folder modes")
	}
	env, ledger := initEnv(t)
	writeSettings(t, env, `{}`)
	p := plan(t, env, answers(ledger, nil))
	if err := os.Chmod(env.DataDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(env.DataDir, 0o700) })
	_, err := call(t, env, "init_apply", map[string]any{"plan_id": p["plan_id"], "diff_sha256": p["diff_sha256"]})
	if err == nil {
		t.Fatal("no error")
	}
	if b, _ := os.ReadFile(env.SettingsFile); string(b) != `{}` {
		t.Fatalf("settings were written before the tap: %s", b)
	}
}

func TestInitKeepsIndentation(t *testing.T) {
	for _, indent := range []string{"\t", "    "} {
		env, ledger := initEnv(t)
		old := "{\n" + indent + "\"theme\": \"dark\",\n" + indent + "\"env\": {\n" + indent + indent + "\"A\": \"1\"\n" + indent + "}\n}\n"
		writeSettings(t, env, old)
		d := plan(t, env, answers(ledger, nil))["diff"].(string)
		d = d[:strings.Index(d, "\n--- /dev/null")]
		if strings.Contains(d, "-"+indent+"\"theme\"") || strings.Contains(d, "-"+indent+indent+"\"A\"") || !strings.Contains(d, "+"+indent+"\"autoCompactWindow\": 550000") {
			t.Fatalf("indent %q diff:\n%s", indent, d)
		}
	}
}

func TestInitApplyAcrossASecond(t *testing.T) {
	env, ledger := initEnv(t)
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	env.Now = func() time.Time { return now }
	p := plan(t, env, answers(ledger, nil))
	now = now.Add(2 * time.Second)
	apply(t, env, p)
	if b, _ := os.ReadFile(filepath.Join(ledger, "mode.md")); !strings.Contains(string(b), "changed: 2026-09-30T10:00:00Z") {
		t.Fatalf("mode.md:\n%s", b)
	}
}

// Spec 16 step 4: the defaults keep each value that the settings already have, and set only the missing ones.
func TestInitDefaultsKeepExistingValues(t *testing.T) {
	// The key after autoCompactWindow keeps its line free of a new comma, so a kept value leaves the line unchanged.
	const existing = "{\n  \"autoCompactWindow\": 400000,\n  \"theme\": \"dark\"\n}\n"
	for _, c := range []struct {
		name     string
		settings string
		answer   map[string]any
		want     float64
	}{
		{"existing value and no answer", existing, nil, 400000},
		{"no key and no answer", "{\n  \"theme\": \"dark\"\n}\n", nil, 550000},
		{"existing value and an answer", existing, map[string]any{"auto_compact_window": 600000}, 600000},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, ledger := initEnv(t)
			writeSettings(t, env, c.settings)
			p := plan(t, env, answers(ledger, c.answer))
			if c.settings == existing && c.answer == nil {
				for line := range strings.Lines(p["diff"].(string)) {
					if (strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-")) && strings.Contains(line, `"autoCompactWindow"`) {
						t.Errorf("init_plan with settings %q and no answer: diff line %q, want no change of autoCompactWindow", c.settings, line)
					}
				}
			}
			apply(t, env, p)
			if got := readSettings(t, env)["autoCompactWindow"]; got != c.want {
				t.Errorf("init_apply with settings %q and answers %v: autoCompactWindow = %v, want %v", c.settings, c.answer, got, c.want)
			}
		})
	}
}

// repoRoot returns a root folder with a repository (a folder with a .git folder) for each path.
func repoRoot(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		if err := os.MkdirAll(filepath.Join(root, p, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func project(key, main string, repos ...string) map[string]any {
	return map[string]any{"key": key, "repos": repos, "main": main}
}

func TestInitPlanValidatesProjects(t *testing.T) {
	root := repoRoot(t, "app", "api", "web")
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "plain"), 0o755); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a", 41)
	badFill := project("app", "app", "app")
	badFill["fills"] = []map[string]any{{"field": "link", "value": "nope", "source": "agent", "repo": "app", "file": "README.md", "line": 1}}
	for _, c := range []struct {
		name  string
		extra map[string]any
		want  []string // each text is in the error
	}{
		{"key with capitals", map[string]any{"projects": []any{project("My_App", "app", "app")}}, []string{"My_App"}},
		{"key of 41 characters", map[string]any{"projects": []any{project(long, "app", "app")}}, []string{long}},
		{"key twice", map[string]any{"projects": []any{project("dup", "app", "app"), project("dup", "api", "api")}}, []string{"dup"}},
		{"parent path", map[string]any{"projects": []any{project("parent", "../x", "../x")}}, []string{"parent", "../x"}},
		{"absolute path", map[string]any{"projects": []any{project("rooted", "/abs", "/abs")}}, []string{"rooted", "/abs"}},
		{"repository is a file", map[string]any{"projects": []any{project("text", "notes.txt", "notes.txt")}}, []string{"text", "notes.txt"}},
		{"repository without .git", map[string]any{"projects": []any{project("nogit", "plain", "plain")}}, []string{"nogit", "plain"}},
		{"repository in two projects", map[string]any{"projects": []any{project("one", "app", "app"), project("two", "api", "api", "app")}}, []string{"two", "app"}},
		{"main not in repos", map[string]any{"projects": []any{project("app", "api", "app")}}, []string{"app", "api"}},
		{"link to an unknown key", map[string]any{"projects": []any{badFill}}, []string{"app", "nope"}},
		{"relative root", map[string]any{"root": "rel/root"}, []string{"root", "rel/root"}},
		{"depth 9", map[string]any{"depth": 9}, []string{"depth", "9"}},
		{"empty exclude entry", map[string]any{"exclude": []string{"archive", ""}}, []string{"exclude"}},
		{"unknown host kind", map[string]any{"host_kinds": map[string]string{"git.example.com": "bitbucket"}}, []string{"host_kinds", "bitbucket"}},
		{"host alias with ;", map[string]any{"host_aliases": map[string]string{"bad;alias": "git.example.com"}}, []string{"host_aliases", "bad;alias"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			env, ledger := initEnv(t)
			a := answers(ledger, map[string]any{"root": root})
			for k, v := range c.extra {
				a[k] = v
			}
			out, err := call(t, env, "init_plan", map[string]any{"answers": a})
			if out != nil || err == nil {
				t.Fatalf("init_plan with %v: planned %t, error %v, want no plan and an error", c.extra, out != nil, err)
			}
			if strings.Contains(err.Error(), "unknown field") {
				t.Errorf("init_plan with %v: error %q, want the new keys accepted and the check named", c.extra, err)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("init_plan with %v: error %q does not name %q", c.extra, err, w)
				}
			}
		})
	}
	t.Run("two valid projects", func(t *testing.T) {
		env, ledger := initEnv(t)
		api := project("api", "api", "api", "web")
		api["fills"] = []map[string]any{{"field": "link", "value": "app", "source": "agent", "repo": "api", "file": "README.md", "line": 1}}
		a := answers(ledger, map[string]any{
			"root": root, "depth": 2, "exclude": []string{"archive"},
			"host_kinds":   map[string]string{"git.example.com": "gitea"},
			"host_aliases": map[string]string{"work": "git.example.com"},
			"projects":     []any{project("app", "app", "app"), api},
		})
		if p := plan(t, env, a); p["plan_id"] == "" {
			t.Errorf("init_plan with %v: plan = %v, want a plan ID", a, p)
		}
	})
}

func TestInitPlanRefusesLedgerRepo(t *testing.T) {
	env, _ := initEnv(t)
	// The root is a symbolic link to the folder of the ledger, so only resolved paths are equal.
	real := repoRoot(t, "notes", "app")
	root := filepath.Join(t.TempDir(), "root")
	if err := os.Symlink(real, root); err != nil {
		t.Fatal(err)
	}
	a := answers(filepath.Join(real, "notes"), map[string]any{"root": root, "projects": []any{project("app", "app", "app", "notes")}})
	out, err := call(t, env, "init_plan", map[string]any{"answers": a})
	if out != nil || err == nil || !strings.Contains(err.Error(), "notes") || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("init_plan with %v: planned %t, error %v, want no plan and an error that names the ledger repository notes", a, out != nil, err)
	}
}
