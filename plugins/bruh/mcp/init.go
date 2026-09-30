package main

import (
	"bytes"
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// pluginID is the ID of the marketplace install (spec 18): the key of pluginConfigs.
const pluginID = "bruh@bruh"

var (
	planIDRE  = regexp.MustCompile(`^[a-z2-7]{16}$`)
	repoRE    = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
	envNameRE = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
)

type MergeGrant struct {
	Repo       string `json:"repo"`
	Merger     string `json:"merger"`
	Conditions string `json:"conditions"`
}

// InitAnswers are the answers of interfaces section 5. Zero values take the defaults.
type InitAnswers struct {
	UserName           string       `json:"user_name"`
	LedgerPath         string       `json:"ledger_path"`
	Mode               string       `json:"mode"`
	P1BatchMinutes     int          `json:"p1_batch_minutes"`
	P1BatchSize        int          `json:"p1_batch_size"`
	ReviewRoundCap     int          `json:"review_round_cap"`
	AutoCompactWindow  int          `json:"auto_compact_window"`
	HandoffPercent     int          `json:"handoff_percent"`
	MaxBusyClerks      int          `json:"max_busy_clerks"`
	WrapStatusline     *bool        `json:"wrap_statusline"`
	Channels           []string     `json:"channels"`
	DelegatedP1Classes []string     `json:"delegated_p1_classes"`
	MergeGrants        []MergeGrant `json:"merge_grants"`
	RemoteEnvironments []string     `json:"remote_environments"`
}

func oneLine(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, "\r\n") && len(s) <= 500
}

// normalize sets the defaults and checks every answer.
func (a *InitAnswers) normalize() error {
	a.Mode = cmp.Or(a.Mode, "human")
	a.P1BatchMinutes = cmp.Or(a.P1BatchMinutes, 60)
	a.P1BatchSize = cmp.Or(a.P1BatchSize, 5)
	a.ReviewRoundCap = cmp.Or(a.ReviewRoundCap, 2)
	a.AutoCompactWindow = cmp.Or(a.AutoCompactWindow, 550000)
	a.HandoffPercent = cmp.Or(a.HandoffPercent, 50)
	a.MaxBusyClerks = cmp.Or(a.MaxBusyClerks, 8)
	if a.WrapStatusline == nil {
		a.WrapStatusline = new(true)
	}
	var errs []error
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	check(oneLine(a.UserName), "user_name is required (one line)")
	check(filepath.IsAbs(a.LedgerPath), "ledger_path must be an absolute path: %q", a.LedgerPath)
	check(a.Mode == "human" || a.Mode == "autonomous", "mode must be human or autonomous: %q", a.Mode)
	check(a.P1BatchMinutes > 0 && a.P1BatchSize > 0 && a.ReviewRoundCap > 0, "p1_batch_minutes, p1_batch_size, and review_round_cap must be 1 or more")
	check(a.AutoCompactWindow >= 100000 && a.AutoCompactWindow <= 1000000, "auto_compact_window must be 100000 to 1000000: %d", a.AutoCompactWindow)
	check(a.HandoffPercent >= 1 && a.HandoffPercent <= 99, "handoff_percent must be 1 to 99: %d", a.HandoffPercent)
	check(a.MaxBusyClerks >= 1, "max_busy_clerks must be 1 or more: %d", a.MaxBusyClerks)
	for i, ch := range a.Channels {
		check((ch == "telegram" || ch == "slack") && !slices.Contains(a.Channels[:i], ch), "channels: %q is not telegram or slack, or is repeated", ch)
	}
	for _, c := range a.DelegatedP1Classes {
		check(oneLine(c), "delegated_p1_classes: each class is one line: %q", c)
	}
	for _, g := range a.MergeGrants {
		k, err := ParseRoleKey(g.Merger)
		check(repoRE.MatchString(g.Repo), "merge_grants: repo must be owner/name: %q", g.Repo)
		check(err == nil && k.Role == "clerk", "merge_grants: merger must be a clerk role key: %q", g.Merger)
		check(oneLine(g.Conditions), "merge_grants: conditions are one line: %q", g.Conditions)
	}
	for _, e := range a.RemoteEnvironments {
		check(envNameRE.MatchString(e), "remote_environments: invalid name %q", e)
	}
	return errors.Join(errs...)
}

type plannedFile struct {
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	Before  string `json:"before"` // SHA-256 of the current content, or "absent"
	Content string `json:"content"`
}

type initPlan struct {
	ID    string        `json:"id"`
	At    string        `json:"at"`
	Files []plannedFile `json:"files"`
}

func fileHash(path string) (string, []byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "absent", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), data, nil
}

// shq quotes s as one sh word.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// mcpAllowRules lists the allow rules of the bruh MCP tools. init_apply is left out, so an
// agent cannot write user settings without the user.
func mcpAllowRules(slack bool) []string {
	var rules []string
	for _, t := range AllTools() {
		if t.Name != "init_apply" {
			rules = append(rules, "mcp__plugin_bruh_bruh__"+t.Name)
		}
	}
	if slack {
		rules = append(rules, "mcp__plugin_bruh_slack__post_question", "mcp__plugin_bruh_slack__reply")
	}
	return rules
}

// planSettings returns the new content of the user settings file.
func planSettings(old []byte, a InitAnswers, tap string) ([]byte, error) {
	top := newObject()
	if len(bytes.TrimSpace(old)) > 0 {
		v, err := parseOrdered(old)
		if err != nil {
			return nil, fmt.Errorf("settings file: %w", err)
		}
		o, ok := v.(*object)
		if !ok {
			return nil, errors.New("settings file is not a JSON object")
		}
		top = o
	}
	top.set("autoCompactWindow", json.Number(strconv.Itoa(a.AutoCompactWindow)))
	if *a.WrapStatusline {
		sl := top.child("statusLine")
		prev, _ := sl.vals["command"].(string)
		cmd := shq(tap)
		switch {
		case prev == cmd || strings.HasPrefix(prev, cmd+" "):
			cmd = prev // already wrapped
		case prev != "":
			cmd += " " + shq(prev)
		}
		sl.set("type", "command")
		sl.set("command", cmd)
	}
	perms := top.child("permissions")
	allow, _ := perms.vals["allow"].([]any)
	for _, r := range append([]string{"Workflow(bruh:deliver)"}, mcpAllowRules(slices.Contains(a.Channels, "slack"))...) {
		if !slices.Contains(allow, any(r)) {
			allow = append(allow, r)
		}
	}
	perms.set("allow", allow)
	opts := top.child("pluginConfigs").child(pluginID).child("options")
	opts.set("user_name", a.UserName)
	opts.set("handoff_percent", json.Number(strconv.Itoa(a.HandoffPercent)))
	opts.set("max_busy_clerks", json.Number(strconv.Itoa(a.MaxBusyClerks)))
	return encodeOrdered(top), nil
}

// fillLedgerFile applies the fill rules of ledger-template/README.md to one template file.
func fillLedgerFile(rel string, content []byte, a InitAnswers, pluginRoot, now string) ([]byte, error) {
	switch rel {
	case "mode.md":
		values := map[string]string{
			"mode": a.Mode, "p1_batch_minutes": strconv.Itoa(a.P1BatchMinutes), "p1_batch_size": strconv.Itoa(a.P1BatchSize),
			"review_round_cap": strconv.Itoa(a.ReviewRoundCap), "changed": now, "reason": "init",
		}
		lines := strings.Split(string(content), "\n")
		for i, l := range lines {
			if k, _, ok := strings.Cut(l, ": "); ok {
				if v, ok := values[k]; ok {
					lines[i] = k + ": " + v
				}
			}
		}
		return []byte(strings.Join(lines, "\n")), nil
	case "priorities.md":
		def, err := os.ReadFile(filepath.Join(pluginRoot, "defaults", "priorities.md"))
		if errors.Is(err, fs.ErrNotExist) {
			def = content
		} else if err != nil {
			return nil, err
		}
		if len(a.DelegatedP1Classes) == 0 {
			return def, nil
		}
		var items strings.Builder
		for _, c := range a.DelegatedP1Classes {
			items.WriteString("- " + c + "\n")
		}
		text := string(def)
		const head = "\n## Delegated P1 classes\n"
		i := strings.Index(text, head)
		if i < 0 {
			return nil, errors.New("default priorities.md has no section \"Delegated P1 classes\"")
		}
		body := i + len(head)
		end := len(text)
		if j := strings.Index(text[body:], "\n## "); j >= 0 {
			end = body + j + 1
		}
		section := strings.TrimRight(text[body:end], "\n") + "\n\n" + items.String()
		if end < len(text) {
			section += "\n"
		}
		return []byte(text[:body] + section + text[end:]), nil
	case "grants.md":
		text := strings.TrimRight(string(content), "\n") + "\n"
		cell := func(s string) string { return strings.ReplaceAll(s, "|", `\|`) }
		for _, g := range a.MergeGrants {
			text += fmt.Sprintf("| %s | %s | %s | %s | %s | init |\n", cell(g.Repo), g.Merger, cell(g.Conditions), cell(g.Conditions), now)
		}
		return []byte(text), nil
	}
	return content, nil
}

// planInit computes every file that init would write. It writes nothing.
func planInit(env Env, a InitAnswers) ([]plannedFile, error) {
	if env.DataDir == "" {
		return nil, errors.New("BRUH_DATA is not set")
	}
	data, err := filepath.Abs(env.DataDir)
	if err != nil {
		return nil, err
	}
	root, err := filepath.Abs(env.PluginRoot)
	if err != nil {
		return nil, err
	}
	var files []plannedFile
	add := func(path string, mode uint32, content []byte) error {
		// Write through a symbolic link, for example a settings file of a dotfiles repository.
		if fi, err := os.Lstat(path); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			if path, err = filepath.EvalSymlinks(path); err != nil {
				return err
			}
		}
		before, old, err := fileHash(path)
		if err != nil {
			return err
		}
		if before != "absent" && bytes.Equal(old, content) {
			return nil
		}
		files = append(files, plannedFile{Path: path, Mode: mode, Before: before, Content: string(content)})
		return nil
	}
	tap := filepath.Join(data, "bin", "statusline-tap.sh")
	_, old, err := fileHash(env.SettingsFile)
	if err != nil {
		return nil, err
	}
	settings, err := planSettings(old, a, tap)
	if err != nil {
		return nil, err
	}
	if err := add(env.SettingsFile, 0o600, settings); err != nil {
		return nil, err
	}
	if *a.WrapStatusline {
		src, err := os.ReadFile(filepath.Join(root, "scripts", "statusline-tap.sh"))
		if err != nil {
			return nil, err
		}
		if err := add(tap, 0o755, src); err != nil {
			return nil, err
		}
	}
	bigm := filepath.Join(data, "roles", "bigm.json")
	if _, err := os.Stat(bigm); errors.Is(err, fs.ErrNotExist) {
		content, err := roleSettings(root, "bigm", nil, nil)
		if err != nil {
			return nil, err
		}
		if err := add(bigm, 0o600, content); err != nil {
			return nil, err
		}
	}
	now := env.Now().UTC().Format("2006-01-02T15:04:05Z") // the time format of the ledger
	tmpl := filepath.Join(root, "ledger-template")
	err = filepath.WalkDir(tmpl, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(tmpl, p)
		if err != nil {
			return err
		}
		dest := filepath.Join(a.LedgerPath, rel)
		if _, err := os.Lstat(dest); err == nil {
			return nil // never change an existing ledger file
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if content, err = fillLedgerFile(filepath.ToSlash(rel), content, a, root, now); err != nil {
			return err
		}
		return add(dest, 0o644, content)
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return files, nil
}

func planDiff(files []plannedFile) (string, error) {
	var b strings.Builder
	for _, f := range files {
		from := f.Path
		var old []byte
		if f.Before == "absent" {
			from = "/dev/null"
		} else {
			var err error
			if old, err = os.ReadFile(f.Path); err != nil {
				return "", err
			}
		}
		b.WriteString(unifiedDiff(from, f.Path, string(old), f.Content))
	}
	return b.String(), nil
}

func launchCommand(data string, a InitAnswers) string {
	cmd := "cd " + shq(a.LedgerPath) + " && claude --agent bruh:bigm --name bigm --permission-mode auto --settings " + shq(filepath.Join(data, "roles", "bigm.json"))
	if slices.Contains(a.Channels, "telegram") {
		cmd += " --channels plugin:telegram@claude-plugins-official"
	}
	if slices.Contains(a.Channels, "slack") {
		cmd += " --dangerously-load-development-channels plugin:bruh@bruh"
	}
	return cmd
}

// initPlanRun plans init, stores the plan, and returns the tool result.
func initPlanRun(env Env, a InitAnswers) (map[string]any, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	files, err := planInit(env, a)
	if err != nil {
		return nil, err
	}
	diff, err := planDiff(files)
	if err != nil {
		return nil, err
	}
	plan := initPlan{ID: strings.ToLower(rand.Text()[:16]), At: env.Stamp(), Files: files}
	dir, err := env.Dir("init", "plans")
	if err != nil {
		return nil, err
	}
	raw, _ := json.MarshalIndent(plan, "", "  ")
	if err := atomicWrite(filepath.Join(dir, plan.ID+".json"), raw); err != nil {
		return nil, err
	}
	data, _ := filepath.Abs(env.DataDir)
	paths := []string{}
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return map[string]any{
		"plan_id":        plan.ID,
		"diff":           diff,
		"files":          paths,
		"launch_command": launchCommand(data, a),
		"trust":          []string{a.LedgerPath},
	}, nil
}

// initApplyRun writes the files of a plan. It refuses the whole plan when a target changed.
func initApplyRun(env Env, id string) ([]string, error) {
	if !planIDRE.MatchString(id) {
		return nil, fmt.Errorf("invalid plan_id: %q", id)
	}
	if env.DataDir == "" {
		return nil, errors.New("BRUH_DATA is not set")
	}
	file := filepath.Join(env.DataDir, "init", "plans", id+".json")
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("no plan %s; run init_plan again: %w", id, err)
	}
	var plan initPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return nil, err
	}
	for _, f := range plan.Files {
		now, _, err := fileHash(f.Path)
		if err != nil {
			return nil, err
		}
		if now != f.Before {
			return nil, fmt.Errorf("%s changed after init_plan; nothing was written; run init_plan again", f.Path)
		}
	}
	applied := []string{}
	for _, f := range plan.Files {
		dirMode := os.FileMode(0o700)
		if f.Mode == 0o644 {
			dirMode = 0o755
		}
		if err := os.MkdirAll(filepath.Dir(f.Path), dirMode); err != nil {
			return applied, err
		}
		if err := atomicWrite(f.Path, []byte(f.Content)); err != nil {
			return applied, err
		}
		if err := os.Chmod(f.Path, os.FileMode(f.Mode)); err != nil {
			return applied, err
		}
		applied = append(applied, f.Path)
	}
	return applied, os.Remove(file)
}

func initTools() []Tool {
	str, num := stringSchema(), map[string]any{"type": "integer"}
	list := map[string]any{"type": "array", "items": str}
	return []Tool{
		{
			Name:        "init_plan",
			Description: "Plan /bruh:init: compute the diff of every file init would write (user settings, the status line tap, the bigm role settings, the ledger layout). Writes only the plan. Show the diff to the user and wait for an explicit yes before init_apply.",
			InputSchema: objectSchema(map[string]any{"answers": objectSchema(map[string]any{
				"user_name": str, "ledger_path": str, "mode": map[string]any{"type": "string", "enum": []string{"human", "autonomous"}},
				"p1_batch_minutes": num, "p1_batch_size": num, "review_round_cap": num, "auto_compact_window": num,
				"handoff_percent": num, "max_busy_clerks": num, "wrap_statusline": map[string]any{"type": "boolean"},
				"channels": list, "delegated_p1_classes": list, "remote_environments": list,
				"merge_grants": map[string]any{"type": "array", "items": objectSchema(map[string]any{"repo": str, "merger": str, "conditions": str}, "repo", "merger", "conditions")},
			}, "user_name", "ledger_path")}, "answers"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Answers json.RawMessage `json:"answers"`
				}](raw)
				if err != nil {
					return nil, err
				}
				answers, err := parseAnswers(a.Answers)
				if err != nil {
					return nil, err
				}
				return initPlanRun(c.Env, answers)
			},
		},
		{
			Name:        "init_apply",
			Description: "Apply a plan of init_plan. Call it only after the user said yes to the diff. Refuses a plan whose targets changed.",
			InputSchema: objectSchema(map[string]any{"plan_id": str}, "plan_id"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					PlanID string `json:"plan_id"`
				}](raw)
				if err != nil {
					return nil, err
				}
				applied, err := initApplyRun(c.Env, a.PlanID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"applied": applied}, nil
			},
		},
	}
}

// parseAnswers decodes answers and refuses unknown keys, so a typo is not silently a default.
func parseAnswers(raw []byte) (InitAnswers, error) {
	var a InitAnswers
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = []byte(`{}`)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&a); err != nil {
		return a, fmt.Errorf("answers: %w", err)
	}
	return a, nil
}
