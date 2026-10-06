package main

import (
	"cmp"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	if ls["agent"] != "bruh:bigm" || lenv["BRUH_ROLE_KEY"] != "bigm" || lenv["CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS"] != "16" || !slices.Equal(ldeny, bigmDeny(t, env)) {
		t.Fatalf("init_apply: ledger settings = %v, want agent bruh:bigm, BRUH_ROLE_KEY bigm, CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS 16, deny %v", ls, bigmDeny(t, env))
	}
	if fi, err := os.Stat(filepath.Join(ledger, ".claude", "settings.json")); err != nil || fi.Mode().Perm() != 0o644 {
		t.Fatalf("init_apply: ledger settings file = %v (%v), want mode 0644", fi, err)
	}
	mode, _ := os.ReadFile(filepath.Join(ledger, "mode.md"))
	for _, want := range []string{"\nmode: autonomous\n", "\nreason: init\n", "\np1_batch_minutes: 30\n", "\np1_batch_size: 5\n", "\nreview_round_cap: 2\n", "\nstatus_cadence: on-change\n", "\nchanged: 20",
		"\nledger_max_lines: 300\n", "\nremote_environments: none\n"} {
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
	wantLaunch := "cd " + shq(ledger) + " && claude --agent bruh:bigm --name bigm --permission-mode auto --channels plugin:telegram@claude-plugins-official --dangerously-load-development-channels plugin:bruh@oter"
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

// bigmSix are the deny rules that only the bigm start settings add (owner rule R-1). The test names
// them literally, so a change of bigmDenyRules fails it.
var bigmSix = []any{"Agent(claude-code-guide)", "Agent(general-purpose)", "Agent(Explore)", "Agent(Plan)", "WebFetch", "WebSearch"}

// mailRules are the deny rules of every role settings file on the mail folder of env (task 31),
// written out literally from the absolute data folder, so a change of mailDeny fails the tests.
func mailRules(t *testing.T, env Env) []any {
	t.Helper()
	data, err := filepath.Abs(env.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	return []any{"Read(/" + data + "/mail)", "Read(/" + data + "/mail/**)", "Bash(*" + data + "/mail*)"}
}

// bigmDeny returns the deny rules of the bigm start settings: the default rules, bigmSix, then
// mailRules.
func bigmDeny(t *testing.T, env Env) []any {
	t.Helper()
	return append(append(defaultDeny(t, env), bigmSix...), mailRules(t, env)...)
}

func TestInitMergesLedgerSettings(t *testing.T) {
	env, ledger := initEnv(t)
	deny := defaultDeny(t, env)
	file := filepath.Join(ledger, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	old, _ := json.Marshal(map[string]any{"agent": "other", "env": map[string]any{"A": "1"},
		"permissions": map[string]any{"allow": []any{"Read(x)"}, "deny": []any{"Bash(rm:*)", "WebFetch", deny[1]}}})
	if err := os.WriteFile(file, old, 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, env, plan(t, env, answers(ledger, nil)))
	s := ledgerSettings(t, ledger)
	lenv, _ := s["env"].(map[string]any)
	perms, _ := s["permissions"].(map[string]any)
	allow, _ := perms["allow"].([]any)
	gotDeny, _ := perms["deny"].([]any)
	// The existing rules keep their order; a default or bigm rule that is there already (deny[1],
	// WebFetch) is not added again.
	want := append(append(append([]any{"Bash(rm:*)", "WebFetch", deny[1], deny[0]}, deny[2:]...),
		"Agent(claude-code-guide)", "Agent(general-purpose)", "Agent(Explore)", "Agent(Plan)", "WebSearch"), mailRules(t, env)...)
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
	if s["agent"] != "bruh:bigm" || lenv["BRUH_ROLE_KEY"] != "bigm" || !slices.Equal(deny, bigmDeny(t, env)) {
		t.Fatalf("init_apply over an empty file: ledger settings = %v, want agent bruh:bigm, BRUH_ROLE_KEY bigm, deny %v", s, bigmDeny(t, env))
	}
	if g := skillGroups(t, ledger); len(g) != 1 {
		t.Fatalf("init_apply over an empty file: Skill hook groups = %v, want one", g)
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
		{name: "hooks is not an object", text: `{"hooks":[]}`},
		{name: "PreToolUse is not an array", text: `{"hooks":{"PreToolUse":{}}}`},
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

// skillGroups returns the PreToolUse hook groups of the ledger settings with the matcher Skill.
func skillGroups(t *testing.T, ledger string) []map[string]any {
	t.Helper()
	hooks, _ := ledgerSettings(t, ledger)["hooks"].(map[string]any)
	pre, _ := hooks["PreToolUse"].([]any)
	var out []map[string]any
	for _, g := range pre {
		if g := g.(map[string]any); g["matcher"] == "Skill" {
			out = append(out, g)
		}
	}
	return out
}

// Task 27: the bigm start settings block each skill except bruh:* with a PreToolUse hook, and an
// existing file keeps its own hooks.
func TestInitBigmSkillHook(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal("jq is not on PATH: the Skill hook of bigm needs it")
	}
	env, ledger := initEnv(t)
	file := filepath.Join(ledger, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		t.Fatal(err)
	}
	own := `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"true"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true"}]}]}}`
	if err := os.WriteFile(file, []byte(own), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(t, env, plan(t, env, answers(ledger, nil)))
	hooks := ledgerSettings(t, ledger)["hooks"].(map[string]any)
	if pre := hooks["PreToolUse"].([]any); len(pre) != 2 || pre[0].(map[string]any)["matcher"] != "Bash" || hooks["Stop"] == nil {
		t.Fatalf("hooks = %v, want the own Stop and Bash groups kept and one more group", hooks)
	}
	groups := skillGroups(t, ledger)
	if len(groups) != 1 {
		t.Fatalf("Skill hook groups = %v, want one", groups)
	}
	h := groups[0]["hooks"].([]any)[0].(map[string]any)
	if h["type"] != "command" || h["command"] != bigmSkillHook {
		t.Fatalf("Skill hook = %v, want the command %q", h, bigmSkillHook)
	}
	if d := plan(t, env, answers(ledger, nil))["diff"]; d != "" {
		t.Fatalf("second init_plan: diff = %q, want empty", d)
	}
	// The written command blocks a skill of another plugin (exit 2) and lets a bruh skill pass.
	for _, c := range []struct {
		input string
		code  int
	}{
		{`{"tool_name":"Skill","tool_input":{"skill":"mattpocock-skills:grilling"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"bro"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"bruh:implement"}}`, 0},
	} {
		cmd := exec.Command("sh", "-c", h["command"].(string))
		cmd.Stdin = strings.NewReader(c.input)
		cmd.Env = append(os.Environ(), "HOME="+t.TempDir()) // no global skills, so bro is blocked
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			ee, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			code = ee.ExitCode()
		}
		if code != c.code || (code == 2) != strings.Contains(string(out), "send the ask to a clanker") {
			t.Errorf("Skill hook with %s: exit %d, output %q, want exit %d", c.input, code, out, c.code)
		}
	}
}

// runSkillHook runs the Skill hook command with HOME set to home and returns its exit code and output.
func runSkillHook(t *testing.T, command, home, input string) (int, string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdin = strings.NewReader(input)
	cmd.Env = append(os.Environ(), "HOME="+home)
	out, err := cmd.CombinedOutput()
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		return ee.ExitCode(), string(out)
	}
	return 0, string(out)
}

// Task 34: the Skill hook of bigm also passes the global skills of the owner, the names under
// $HOME/.claude/skills with a SKILL.md, read at run time; every other skill stays blocked.
func TestInitBigmSkillHookAllowsOwnerSkills(t *testing.T) {
	if _, err := exec.LookPath("jq"); err != nil {
		t.Fatal("jq is not on PATH: the Skill hook of bigm needs it")
	}
	env, ledger := initEnv(t)
	apply(t, env, plan(t, env, answers(ledger, nil)))
	groups := skillGroups(t, ledger)
	if len(groups) != 1 {
		t.Fatalf("Skill hook groups = %v, want one", groups)
	}
	command := groups[0]["hooks"].([]any)[0].(map[string]any)["command"].(string)
	home := t.TempDir()
	writeFile(t, filepath.Join(home, ".claude", "skills", "bro", "SKILL.md"), "---\nname: bro\n---\n")
	// A folder with no SKILL.md is not a skill.
	if err := os.MkdirAll(filepath.Join(home, ".claude", "skills", "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		input string
		code  int
	}{
		{`{"tool_name":"Skill","tool_input":{"skill":"bro"}}`, 0},
		{`{"tool_name":"Skill","tool_input":{"skill":"bruh:implement"}}`, 0},
		{`{"tool_name":"Skill","tool_input":{"skill":"mattpocock-skills:grilling"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"nothere"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"empty"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"../.claude/skills/bro"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"x/../bro"}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":"."}}`, 2},
		{`{"tool_name":"Skill","tool_input":{"skill":""}}`, 2},
		{`{"tool_name":"Skill","tool_input":{}}`, 2},
		{`{}`, 2},
		{`not json`, 2},
	} {
		code, out := runSkillHook(t, command, home, c.input)
		if code != c.code || (code == 2) != strings.Contains(out, "send the ask to a clanker") {
			t.Errorf("Skill hook with %s: exit %d, output %q, want exit %d", c.input, code, out, c.code)
		}
	}
	// Without jq the hook blocks, even a global skill of the owner.
	if code, _ := runSkillHook(t, "PATH=/nonexistent; "+command, home, `{"tool_input":{"skill":"bro"}}`); code != 2 {
		t.Errorf("Skill hook without jq: exit %d, want 2", code)
	}
}

// Task 34: init replaces the Skill hook group of an old command with the new group and keeps the
// own groups of the file.
func TestInitReplacesOldBigmSkillHook(t *testing.T) {
	env, ledger := initEnv(t)
	file := filepath.Join(ledger, ".claude", "settings.json")
	// The group as init wrote it before task 34: matcher first, then hooks.
	q, _ := json.Marshal(oldBigmSkillHooks[0])
	writeFile(t, file, `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"true"}]},`+
		`{"matcher":"Skill","hooks":[{"type":"command","command":`+string(q)+`}]},`+
		`{"matcher":"Skill","hooks":[{"type":"command","command":"true"}]}]}}`)
	apply(t, env, plan(t, env, answers(ledger, nil)))
	pre := ledgerSettings(t, ledger)["hooks"].(map[string]any)["PreToolUse"].([]any)
	var cmds []string
	for _, g := range pre {
		g := g.(map[string]any)
		cmds = append(cmds, g["matcher"].(string)+" "+g["hooks"].([]any)[0].(map[string]any)["command"].(string))
	}
	want := []string{"Bash true", "Skill true", "Skill " + bigmSkillHook}
	if !slices.Equal(cmds, want) {
		t.Fatalf("PreToolUse = %q, want %q", cmds, want)
	}
	if d := plan(t, env, answers(ledger, nil))["diff"]; d != "" {
		t.Fatalf("second init_plan: diff = %q, want empty", d)
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

// withValue returns text with the value of its line "key: ..." set to value.
func withValue(t *testing.T, text, key, value string) string {
	t.Helper()
	lines := strings.Split(text, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, key+": ") {
			lines[i] = key + ": " + value
			return strings.Join(lines, "\n")
		}
	}
	t.Fatalf("no line %q in:\n%s", key+": ", text)
	return ""
}

// withRow returns text with row after the separator line of the table with the header line header.
func withRow(t *testing.T, text, header, row string) string {
	t.Helper()
	before, after, ok := strings.Cut(text, header+"\n")
	sep, rest, ok2 := strings.Cut(after, "\n")
	if !ok || !ok2 {
		t.Fatalf("no table %q in:\n%s", header, text)
	}
	return before + header + "\n" + sep + "\n" + row + "\n" + rest
}

// TestInitUpdatesTemplateTextAndKeepsRows updates a ledger of version 0.5 (spec 8.6, G24, G25):
// each file gets the new template text and keeps its values, its data rows, and its rule sections.
func TestInitUpdatesTemplateTextAndKeepsRows(t *testing.T) {
	env, ledger := initEnv(t)
	fixtures := filepath.Join("testdata", "ledger-v05")
	copyTree(t, fixtures, ledger)
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	tmpl := func(rel string) string {
		return read(filepath.Join(env.PluginRoot, "ledger-template", filepath.FromSlash(rel)))
	}
	fixture := func(rel string) string { return read(filepath.Join(fixtures, filepath.FromSlash(rel))) }

	// The rules of G24: the template text before the line "## Rules", then the rule sections.
	head, _, ok := strings.Cut(tmpl("rules.md"), "\n## Rules\n")
	_, ruleSections, ok2 := strings.Cut(fixture("rules.md"), "\n## Rules\n")
	if !ok || !ok2 || !strings.Contains(ruleSections, "## R-1: ") {
		t.Fatal(`the template or the fixture rules.md has no line "## Rules" with a rule section after it`)
	}
	mode := tmpl("mode.md")
	for k, v := range map[string]string{
		"mode": "autonomous", "changed": "2026-09-30T12:00:00Z", "reason": "owner answer Q-2",
		"p1_batch_minutes": "45", "p1_batch_size": "5", "review_round_cap": "3", "status_cadence": "always",
	} {
		mode = withValue(t, mode, k, v)
	}
	grants := withRow(t, tmpl("grants.md"), "| Poster role key | Host | Repository | Conditions | Owner words | Date (UTC) | Question ID |",
		`| clerk-shop-post | gitlab | group/shop | review findings only | "post the findings" | 2026-09-29T09:00:00Z | Q-3 |`)
	grants = withRow(t, grants, "| Repository | Merger role key | Conditions | Owner words | Date (UTC) | Question ID |",
		`| group/shop | clerk-shop-merge | green pipeline | "merge when green" | 2026-09-29T09:05:00Z | Q-4 |`)
	want := map[string]string{
		"README.md":             tmpl("README.md"),
		"projects/_template.md": tmpl("projects/_template.md"),
		// The fixture is the placeholder of version 0.5: the first-run rule replaces it.
		"priorities.md": read(filepath.Join(env.PluginRoot, "defaults", "priorities.md")),
		"rules.md":      head + "\n## Rules\n" + ruleSections,
		"mode.md":       mode,
		"grants.md":     grants,
		"questions.md": withRow(t, withValue(t, tmpl("questions.md"), "last_batch", "2026-10-01T00:00:00Z"),
			"| ID | P-level | From (role key) | Subject | Asked (UTC) | Blocks | Channel message | State |",
			"| Q-1 | P1 | clanker-shop | Pick the cache size | 2026-10-01T00:00:00Z | shop deploy | none | open |"),
		"owed.md": withRow(t, tmpl("owed.md"), "| Item | Kind | Added (UTC) | Source | State |",
			"| Status of the shop build | owed | 2026-09-30T10:00:00Z | terminal | open |"),
		"leases.md": withRow(t, tmpl("leases.md"), "| Resource | Capacity | Holder (role key) | Until (UTC) | Source read |",
			"| shop-db | 1 | clanker-shop | 2026-10-01T02:00:00Z | lease_list 2026-10-01T00:00:00Z |"),
		// G25: a project file is not a template file, so init does not change it.
		"projects/shop.md": fixture("projects/shop.md"),
	}
	if strings.Contains(want["projects/_template.md"], "Questions and answers") || !strings.Contains(want["mode.md"], "\nledger_max_lines: 300\n") {
		t.Fatal("the ledger template is not the template of version 0.6")
	}

	p := plan(t, env, answers(ledger, nil))
	if diff := p["diff"].(string); !strings.Contains(diff, "\n+ledger_max_lines: 300\n") {
		t.Errorf("init_plan of a version 0.5 ledger: the diff has no line +ledger_max_lines: 300:\n%s", diff)
	}
	apply(t, env, p)
	for _, rel := range slices.Sorted(maps.Keys(want)) {
		if got := read(filepath.Join(ledger, filepath.FromSlash(rel))); got != want[rel] {
			t.Errorf("after init_apply, ledger %s =\n%s\nwant\n%s", rel, got, want[rel])
		}
	}
	if p2 := plan(t, env, answers(ledger, nil)); p2["diff"] != "" {
		t.Errorf("second init_plan: diff =\n%s\nwant empty", p2["diff"])
	}

	// Two rules that the fixtures do not reach. A table whose header line is not in the template
	// goes to the end, unchanged.
	extra := "| Old | Table |\n|---|---|\n| one | two |\n"
	got, err := mergeLedgerFile("owed.md", []byte(tmpl("owed.md")), []byte(fixture("owed.md")+"\n"+extra), env.PluginRoot)
	if err != nil || !strings.HasPrefix(string(got), want["owed.md"]) || !strings.HasSuffix(string(got), "\n\n"+extra) {
		t.Errorf("mergeLedgerFile(owed.md with an extra table) = %q, %v; want the merged file, a blank line, and the extra table %q", got, err, extra)
	}
	// A priorities.md with the line "## Never without the owner" is not a placeholder: it stays.
	prio := "# Priorities\n\n## Never without the owner\n\n- Delete a repository.\n"
	got, err = mergeLedgerFile("priorities.md", []byte(tmpl("priorities.md")), []byte(prio), env.PluginRoot)
	if err != nil || string(got) != prio {
		t.Errorf("mergeLedgerFile(priorities.md) = %q, %v; want %q unchanged", got, err, prio)
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
	const configs = `{"bruh@oter":{"options":{"handoff_percent":55,"max_busy_clerks":30}}}`
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

func writeTestFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPlanDiffShowsDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learn", "projects", "gone.json")
	old := "{\n  \"key\": \"gone\"\n}\n"
	writeTestFile(t, path, old)
	diff, err := planDiff([]plannedFile{{Path: path, Delete: true, Before: sha256Hex(old)}})
	if err != nil {
		t.Fatal(err)
	}
	if want := "--- " + path + "\n+++ /dev/null\n"; !strings.Contains(diff, want) {
		t.Errorf("planDiff of a deletion = %q, want the header %q", diff, want)
	}
	for _, line := range strings.Split(strings.TrimSuffix(old, "\n"), "\n") {
		if !strings.Contains(diff, "\n-"+line+"\n") {
			t.Errorf("planDiff of a deletion = %q, want the line %q", diff, "-"+line)
		}
	}
}

func TestWritePlannedDeletes(t *testing.T) {
	env, ledger := initEnv(t)
	a := InitAnswers{LedgerPath: ledger}
	t.Run("deletion inside the ledger", func(t *testing.T) {
		path := filepath.Join(ledger, "learn", "projects", "gone.json")
		writeTestFile(t, path, "{}\n")
		files := []plannedFile{{Path: path, Delete: true, Before: sha256Hex("{}\n")}}
		applied, err := writePlanned(env, a, files)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(applied, files) {
			t.Errorf("writePlanned(%v) = %v, want the deletion back with Delete true", files, applied)
		}
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Errorf("after writePlanned: Lstat(%s) error = %v, want the file removed", path, err)
		}
	})
	t.Run("deletion of a missing file", func(t *testing.T) {
		path := filepath.Join(ledger, "learn", "projects", "missing.json")
		files := []plannedFile{{Path: path, Delete: true, Before: sha256Hex("{}\n")}}
		if _, err := writePlanned(env, a, files); err != nil {
			t.Errorf("writePlanned(%v) error = %v, want none for an already missing file", files, err)
		}
	})
	t.Run("deletion outside the closed list", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "keep.json")
		writeTestFile(t, path, "{}\n")
		files := []plannedFile{{Path: path, Delete: true, Before: sha256Hex("{}\n")}}
		if _, err := writePlanned(env, a, files); err == nil {
			t.Errorf("writePlanned(%v) error = nil, want a refusal of a target outside the closed list", files)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("after the refusal: Stat(%s) error = %v, want the file kept", path, err)
		}
	})
	t.Run("write and settings file last", func(t *testing.T) {
		settings := plannedFile{Path: env.SettingsFile, Mode: 0o600, Before: "absent", Content: "{\"a\": 1}\n"}
		write := plannedFile{Path: filepath.Join(ledger, "learn", "tree.json"), Mode: 0o644, Before: "absent", Content: "{}\n"}
		applied, err := writePlanned(env, a, []plannedFile{settings, write})
		if err != nil {
			t.Fatal(err)
		}
		if want := []plannedFile{write, settings}; !slices.Equal(applied, want) {
			t.Errorf("writePlanned applied %v, want %v (the settings file last)", applied, want)
		}
		for _, f := range []plannedFile{write, settings} {
			info, err := os.Stat(f.Path)
			if err != nil {
				t.Fatal(err)
			}
			if b, _ := os.ReadFile(f.Path); string(b) != f.Content || info.Mode().Perm() != os.FileMode(f.Mode) {
				t.Errorf("%s = %q mode %v, want %q mode %v", f.Path, b, info.Mode().Perm(), f.Content, os.FileMode(f.Mode))
			}
		}
	})
}

// TestInitPlanLedgerTable checks the ledger_table of init_plan (L32): one row for each project,
// sorted by key, with only the keys key, purpose, repos, links, and docs.
func TestInitPlanLedgerTable(t *testing.T) {
	env, ledger := initEnv(t)
	root := learnRoot(t)
	shop := project("shop", "shop", "shop", "shop-app")
	shop["fills"] = []map[string]any{
		{"field": "purpose", "value": "Online shop of the team", "source": "owner"},
		{"field": "link", "value": "auth", "source": "agent", "repo": "shop", "file": "go.mod", "line": 5},
		{"field": "doc", "value": "README.md", "source": "agent", "repo": "shop"},
	}
	a := answers(ledger, map[string]any{"root": root, "projects": []any{shop, project("auth", "team/auth", "team/auth")}})
	p := plan(t, env, a)
	rows, ok := p["ledger_table"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("init_plan with the projects shop and auth: ledger_table = %#v, want two rows", p["ledger_table"])
	}
	wantKeys := []string{"docs", "key", "links", "purpose", "repos"}
	for i, r := range rows {
		row, _ := r.(map[string]any)
		if got := slices.Sorted(maps.Keys(row)); !slices.Equal(got, wantKeys) {
			t.Errorf("ledger_table row %d has the keys %v, want exactly %v (no stack, no gates)", i, got, wantKeys)
		}
	}
	// The rows are sorted by key: auth, then shop.
	auth, _ := rows[0].(map[string]any)
	if auth["key"] != "auth" || auth["purpose"] != "" || !slices.Equal(pathList(auth["repos"]), []string{"team/auth"}) {
		t.Errorf("ledger_table row 0 = %v, want the key auth, the purpose \"\", and the repos [team/auth]", auth)
	}
	want := map[string]any{
		"key":     "shop",
		"purpose": "Online shop of the team",
		"repos":   []any{"shop", "shop-app"},
		"links":   []any{"auth (shop/go.mod:5)"},
		"docs":    []any{"shop/README.md"},
	}
	if got := rows[1]; !reflect.DeepEqual(got, want) {
		t.Errorf("ledger_table row 1 = %#v, want %#v", got, want)
	}
}

// TestInitPlanSplitsDiff checks the split diff of init_plan (G53): outside_diff has the files
// outside the ledger and <ledger>/.claude/settings.json, ledger_diff has all other ledger files,
// diff is both, and init_apply takes the one hash of diff.
func TestInitPlanSplitsDiff(t *testing.T) {
	env, ledger := initEnv(t)
	key := strings.ToLower(t.Name())
	a := answers(ledger, map[string]any{"root": repoRoot(t, key), "projects": []any{project(key, key, key)}})
	p := plan(t, env, a)
	outside, ok := p["outside_diff"].(string)
	if !ok {
		t.Fatalf("init_plan with the project %s: outside_diff = %#v, want a string", key, p["outside_diff"])
	}
	inLedger, ok := p["ledger_diff"].(string)
	if !ok {
		t.Fatalf("init_plan with the project %s: ledger_diff = %#v, want a string", key, p["ledger_diff"])
	}
	claudeSettings := filepath.Join(ledger, ".claude", "settings.json")
	for _, path := range []string{env.SettingsFile, claudeSettings} {
		if !strings.Contains(outside, "+++ "+path+"\n") {
			t.Errorf("outside_diff = %q, want the file %s", outside, path)
		}
		if strings.Contains(inLedger, path) {
			t.Errorf("ledger_diff = %q, want no file %s (it is in outside_diff)", inLedger, path)
		}
	}
	if learn := filepath.Join(ledger, "learn") + string(filepath.Separator); strings.Contains(outside, learn) {
		t.Errorf("outside_diff = %q, want no path under %s", outside, learn)
	}
	for _, path := range []string{filepath.Join(ledger, "learn", "tree.json"), filepath.Join(ledger, "mode.md")} {
		if !strings.Contains(inLedger, "+++ "+path+"\n") {
			t.Errorf("ledger_diff = %q, want the file %s", inLedger, path)
		}
	}
	if got, want := p["diff"], outside+inLedger; got != want {
		t.Errorf("diff = %q, want outside_diff + ledger_diff = %q", got, want)
	}
	if got, want := p["diff_sha256"], sha256Hex(outside+inLedger); got != want {
		t.Errorf("diff_sha256 = %v, want sha256Hex(outside_diff + ledger_diff) = %s", got, want)
	}
	applied := apply(t, env, p)
	for _, path := range []string{env.SettingsFile, claudeSettings, filepath.Join(ledger, "learn", "tree.json")} {
		if !slices.Contains(applied, path) {
			t.Errorf("init_apply with diff_sha256 %v: applied %v, want %s among them", p["diff_sha256"], applied, path)
		}
	}
}

// TestInitPlanTrustList checks the trust list of init_plan (spec 16, step 12): the ledger with
// only its path, then each repository of the answers in their order, with its absolute path and
// whether .claude/settings.json and .mcp.json exist.
func TestInitPlanTrustList(t *testing.T) {
	t.Run("two projects", func(t *testing.T) {
		env, ledger := initEnv(t)
		root := repoRoot(t, "app", "api", "web")
		writeTestFile(t, filepath.Join(root, "app", ".claude", "settings.json"), "{}\n")
		writeTestFile(t, filepath.Join(root, "api", ".mcp.json"), "{}\n")
		a := answers(ledger, map[string]any{"root": root, "projects": []any{project("app", "app", "app"), project("api", "api", "api", "web")}})
		p := plan(t, env, a)
		want := []any{
			map[string]any{"path": ledger},
			map[string]any{"path": filepath.Join(root, "app"), "claude_settings": true, "mcp": false},
			map[string]any{"path": filepath.Join(root, "api"), "claude_settings": false, "mcp": true},
			map[string]any{"path": filepath.Join(root, "web"), "claude_settings": false, "mcp": false},
		}
		if got := p["trust"]; !reflect.DeepEqual(got, want) {
			t.Errorf("init_plan with the projects app and api: trust = %#v, want %#v", got, want)
		}
	})
	t.Run("no projects", func(t *testing.T) {
		env, ledger := initEnv(t)
		p := plan(t, env, answers(ledger, nil))
		want := []any{map[string]any{"path": ledger}}
		if got := p["trust"]; !reflect.DeepEqual(got, want) {
			t.Errorf("init_plan with no projects: trust = %#v, want %#v", got, want)
		}
	})
}

func TestPlanSettingsUnwrapsOldTap(t *testing.T) {
	const (
		tap   = "/h/.claude/plugins/data/bruh-oter/bin/statusline-tap.sh"
		old   = "/h/.claude/plugins/data/bruh-bruh/bin/statusline-tap.sh"
		other = "/h/.claude/plugins/data/bruh-inline/bin/statusline-tap.sh"
		orig  = `printf '%s' "it's $HOME" && jq -r .model.display_name`
	)
	const (
		cacheTap = "/p/data/bruh-oter/bin/statusline-tap.sh"
		cacheOld = "/p/data/bruh-bruh/bin/statusline-tap.sh"
		fooTap   = "/h/.claude/plugins/data/foo-bar/bin/statusline-tap.sh"
	)
	wrap := func(tap, inner string) string { return shq(tap) + " " + shq(inner) }
	for _, tc := range []struct {
		name, prev, want string
		tap              string // the tap of the current data folder; empty is tap
	}{
		{"no prev", "", shq(tap), ""},
		{"plain prev", "x", wrap(tap, "x"), ""},
		{"prev with quotes", orig, wrap(tap, orig), ""},
		{"current tap unchanged", wrap(tap, orig), wrap(tap, orig), ""},
		{"old tap alone", shq(old), shq(tap), ""},
		{"old tap", wrap(old, orig), wrap(tap, orig), ""},
		{"two old taps", wrap(old, wrap(other, orig)), wrap(tap, orig), ""},
		{"old tap around the current tap", wrap(old, wrap(tap, orig)), wrap(tap, orig), ""},
		{"tap path outside a data folder", wrap("/opt/bin/statusline-tap.sh", "x"), wrap(tap, wrap("/opt/bin/statusline-tap.sh", "x")), ""},
		{"old tap with an extra word", wrap(old, "x") + " y", wrap(tap, wrap(old, "x")+" y"), ""},
		{"current tap with an extra word unchanged", wrap(tap, "x") + " y", wrap(tap, "x") + " y", ""},
		{"current tap around an old tap", wrap(tap, wrap(old, orig)), wrap(tap, orig), ""},
		{"tap of another plugin", wrap(fooTap, "x"), wrap(tap, wrap(fooTap, "x")), ""},
		{"old tap in the plugin cache dir", wrap(cacheOld, orig), wrap(cacheTap, orig), cacheTap},
		{"tap of another plugin in the plugin cache dir", wrap("/p/data/foo-bar/bin/statusline-tap.sh", "x"), wrap(cacheTap, wrap("/p/data/foo-bar/bin/statusline-tap.sh", "x")), cacheTap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings := `{}`
			if tc.prev != "" {
				b, _ := json.Marshal(map[string]any{"statusLine": map[string]any{"type": "command", "command": tc.prev}})
				settings = string(b)
			}
			out, err := planSettings([]byte(settings), InitAnswers{WrapStatusline: new(true)}, cmp.Or(tc.tap, tap))
			if err != nil {
				t.Fatal(err)
			}
			var s struct {
				StatusLine struct {
					Command string `json:"command"`
				} `json:"statusLine"`
			}
			if err := json.Unmarshal(out, &s); err != nil {
				t.Fatal(err)
			}
			if s.StatusLine.Command != tc.want {
				t.Errorf("command = %s\nwant      %s", s.StatusLine.Command, tc.want)
			}
		})
	}
}
