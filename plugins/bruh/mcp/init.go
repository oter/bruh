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
	"sync"
	"time"
)

// pluginID is the ID of the marketplace install (spec 18): the key of pluginConfigs.
const pluginID = "bruh@bruh"

var (
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
	check(ledgerPathOK(a.LedgerPath), "ledger_path must be a clean absolute path with no .., in a folder that exists or can be created: %q", a.LedgerPath)
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

// storedPlan is a plan of init_plan. It lives only in the memory of this server process: the
// same session calls init_plan and init_apply, and a plan file on disk could be forged.
type storedPlan struct {
	answers InitAnswers
	at      time.Time
	files   []plannedFile
	diffSHA string
}

var plans = struct {
	sync.Mutex
	m map[string]*storedPlan
}{m: map[string]*storedPlan{}}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ledgerPathOK accepts an absolute path with no .. element whose nearest existing ancestor is a folder.
func ledgerPathOK(p string) bool {
	if !filepath.IsAbs(p) || slices.Contains(strings.Split(filepath.ToSlash(p), "/"), "..") {
		return false
	}
	for cur := filepath.Clean(p); ; cur = filepath.Dir(cur) {
		if fi, err := os.Stat(cur); err == nil {
			return fi.IsDir()
		}
		if filepath.Dir(cur) == cur {
			return false
		}
	}
}

// resolveExisting resolves the symbolic links of the longest existing prefix of p.
func resolveExisting(p string) string {
	var rest []string
	for cur := filepath.Clean(p); ; cur = filepath.Dir(cur) {
		if r, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(append([]string{r}, rest...)...)
		}
		if filepath.Dir(cur) == cur {
			return filepath.Clean(p)
		}
		rest = append([]string{filepath.Base(cur)}, rest...)
	}
}

func inside(base, p string) bool {
	rel, err := filepath.Rel(resolveExisting(base), resolveExisting(p))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// targetAllowed is the closed list of init targets: the settings file (after its symbolic link),
// files under the data folder, and files under the ledger folder.
func targetAllowed(env Env, a InitAnswers, p string) bool {
	return resolveExisting(p) == resolveExisting(env.SettingsFile) || inside(env.DataDir, p) || inside(a.LedgerPath, p)
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
	return encodeOrdered(top, indentOf(old)), nil
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
// planInit computes the files of init at the time at (the ledger rows carry it). It writes nothing.
func planInit(env Env, a InitAnswers, at time.Time) ([]plannedFile, error) {
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
		if !targetAllowed(env, a, path) {
			return fmt.Errorf("init refuses to write %s: it is not the settings file, and not under the data folder or the ledger folder", path)
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
	cfg, _ := json.MarshalIndent(initConfig{LedgerPath: a.LedgerPath}, "", "  ")
	if err := add(filepath.Join(data, "init", "config.json"), 0o600, append(cfg, '\n')); err != nil {
		return nil, err
	}
	now := at.UTC().Format("2006-01-02T15:04:05Z") // the time format of the ledger
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

// initConfig is <data>/init/config.json: the init answers that other commands read.
type initConfig struct {
	LedgerPath string `json:"ledger_path"`
}

// initPlanRun plans init, keeps the plan in memory, and returns the tool result. It writes nothing.
func initPlanRun(env Env, a InitAnswers) (map[string]any, error) {
	if err := a.normalize(); err != nil {
		return nil, err
	}
	at := env.Now()
	files, err := planInit(env, a, at)
	if err != nil {
		return nil, err
	}
	diff, err := planDiff(files)
	if err != nil {
		return nil, err
	}
	id := strings.ToLower(rand.Text()[:16])
	plans.Lock()
	plans.m[id] = &storedPlan{answers: a, at: at, files: files, diffSHA: sha256Hex(diff)}
	plans.Unlock()
	data, _ := filepath.Abs(env.DataDir)
	paths := []string{}
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	return map[string]any{
		"plan_id":        id,
		"diff":           diff,
		"diff_sha256":    sha256Hex(diff),
		"files":          paths,
		"launch_command": launchCommand(data, a),
		"trust":          []string{a.LedgerPath},
	}, nil
}

// initApplyRun writes the files of a plan of this process. It refuses the plan when diffSHA is
// not the hash of the diff that init_plan returned, when a target changed since init_plan (it
// plans again and compares), or when a target is outside the closed list.
func initApplyRun(env Env, id, diffSHA string) ([]string, error) {
	plans.Lock()
	p := plans.m[id]
	plans.Unlock()
	if p == nil {
		return nil, fmt.Errorf("no plan %q in this session; run init_plan again", id)
	}
	if diffSHA != p.diffSHA {
		return nil, errors.New("diff_sha256 is not the hash of the diff of this plan; show the diff of init_plan to the user and pass its diff_sha256")
	}
	now, err := planInit(env, p.answers, p.at)
	if err != nil {
		return nil, err
	}
	diff, err := planDiff(now)
	if err != nil {
		return nil, err
	}
	if sha256Hex(diff) != p.diffSHA || !slices.Equal(now, p.files) {
		return nil, errors.New("a target changed after init_plan; nothing was written; run init_plan again")
	}
	// The settings file goes last, so a failed write never leaves statusLine pointing at a missing tap.
	settings := resolveExisting(env.SettingsFile)
	files := slices.Clone(p.files)
	slices.SortStableFunc(files, func(x, y plannedFile) int {
		return cmp.Compare(boolInt(resolveExisting(x.Path) == settings), boolInt(resolveExisting(y.Path) == settings))
	})
	applied := []string{}
	for _, f := range files {
		if !targetAllowed(env, p.answers, f.Path) {
			return applied, fmt.Errorf("init refuses to write %s", f.Path)
		}
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
	plans.Lock()
	delete(plans.m, id)
	plans.Unlock()
	return applied, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
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
			Description: "Apply a plan of init_plan of this session. Call it only after the user said yes to the diff. Pass the diff_sha256 of the init_plan result. Refuses a plan whose targets changed.",
			InputSchema: objectSchema(map[string]any{"plan_id": str, "diff_sha256": str}, "plan_id", "diff_sha256"),
			// Claude Code prompts a person for every call, in every permission mode (mcp.md).
			Meta: map[string]any{"anthropic/requiresUserInteraction": true},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					PlanID     string `json:"plan_id"`
					DiffSHA256 string `json:"diff_sha256"`
				}](raw)
				if err != nil {
					return nil, err
				}
				applied, err := initApplyRun(c.Env, a.PlanID, a.DiffSHA256)
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
