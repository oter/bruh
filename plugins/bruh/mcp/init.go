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
	"maps"
	"os"
	"path"
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
	repoRE = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)
	// hostNameRE matches a host and an SSH host alias of the answers host_kinds and host_aliases.
	hostNameRE = regexp.MustCompile(`^[A-Za-z0-9.-]+$`)
)

// InitAnswers are the answers of interfaces section 5. Zero values take the defaults. The plugin
// options UserName, HandoffPercent, and MaxBusyClerks have no default: nil is not in the answers.
type InitAnswers struct {
	UserName          *string  `json:"user_name"`
	LedgerPath        string   `json:"ledger_path"`
	Mode              string   `json:"mode"`
	P1BatchMinutes    int      `json:"p1_batch_minutes"`
	P1BatchSize       int      `json:"p1_batch_size"`
	ReviewRoundCap    int      `json:"review_round_cap"`
	AutoCompactWindow int      `json:"auto_compact_window"`
	HandoffPercent    *int     `json:"handoff_percent"`
	MaxBusyClerks     *int     `json:"max_busy_clerks"`
	WrapStatusline    *bool    `json:"wrap_statusline"`
	Channels          []string `json:"channels"`
	// The learn answers (spec 8.5). A nil Projects means that the answers have no projects
	// key, so the index does not change; an empty Projects removes every project (G27).
	Root        string            `json:"root"`
	Depth       int               `json:"depth"`
	Exclude     []string          `json:"exclude"`
	HostKinds   map[string]string `json:"host_kinds"`
	HostAliases map[string]string `json:"host_aliases"`
	Projects    []answerProject   `json:"projects"`
}

func oneLine(s string) bool {
	return strings.TrimSpace(s) != "" && !strings.ContainsAny(s, "\r\n") && len(s) <= 500
}

// normalize sets the defaults and checks every answer. home is the home folder of the user,
// the base of the default root.
func (a *InitAnswers) normalize(home string) error {
	a.Mode = cmp.Or(a.Mode, "human")
	a.Root = cmp.Or(a.Root, filepath.Join(home, "workspace"))
	a.Depth = cmp.Or(a.Depth, 4)
	if a.Exclude == nil {
		a.Exclude = []string{"archive"}
	}
	a.P1BatchMinutes = cmp.Or(a.P1BatchMinutes, 60)
	a.P1BatchSize = cmp.Or(a.P1BatchSize, 5)
	a.ReviewRoundCap = cmp.Or(a.ReviewRoundCap, 2)
	if a.WrapStatusline == nil {
		a.WrapStatusline = new(true)
	}
	var errs []error
	check := func(ok bool, format string, args ...any) {
		if !ok {
			errs = append(errs, fmt.Errorf(format, args...))
		}
	}
	if a.UserName != nil {
		check(oneLine(*a.UserName), "user_name must be one line: %q", *a.UserName)
	}
	check(ledgerPathOK(a.LedgerPath), "ledger_path must be a clean absolute path with no .., in a folder that exists or can be created: %q", a.LedgerPath)
	check(a.Mode == "human" || a.Mode == "autonomous", "mode must be human or autonomous: %q", a.Mode)
	check(a.P1BatchMinutes > 0 && a.P1BatchSize > 0 && a.ReviewRoundCap > 0, "p1_batch_minutes, p1_batch_size, and review_round_cap must be 1 or more")
	if a.AutoCompactWindow != 0 { // 0 is no answer: planSettings keeps the existing value
		check(a.AutoCompactWindow >= 100000 && a.AutoCompactWindow <= 1000000, "auto_compact_window must be 100000 to 1000000: %d", a.AutoCompactWindow)
	}
	if a.HandoffPercent != nil {
		check(*a.HandoffPercent >= 1 && *a.HandoffPercent <= 99, "handoff_percent must be 1 to 99: %d", *a.HandoffPercent)
	}
	if a.MaxBusyClerks != nil {
		check(*a.MaxBusyClerks >= 1, "max_busy_clerks must be 1 or more: %d", *a.MaxBusyClerks)
	}
	for i, ch := range a.Channels {
		check((ch == "telegram" || ch == "slack") && !slices.Contains(a.Channels[:i], ch), "channels: %q is not telegram or slack, or is repeated", ch)
	}
	rootOK := filepath.IsAbs(a.Root) && filepath.Clean(a.Root) == a.Root
	check(rootOK, "root must be a clean absolute path: %q", a.Root)
	check(a.Depth >= 1 && a.Depth <= 8, "depth must be 1 to 8: %d", a.Depth)
	for _, e := range a.Exclude {
		check(e != "", "exclude: an entry is empty")
	}
	for _, h := range slices.Sorted(maps.Keys(a.HostKinds)) {
		k := a.HostKinds[h]
		check(hostNameRE.MatchString(h), "host_kinds: host %q does not match %s", h, hostNameRE)
		check(k == "github" || k == "gitlab" || k == "gitea", "host_kinds: kind %q of host %q is not github, gitlab, or gitea", k, h)
	}
	for _, alias := range slices.Sorted(maps.Keys(a.HostAliases)) {
		h := a.HostAliases[alias]
		check(hostNameRE.MatchString(alias), "host_aliases: alias %q does not match %s", alias, hostNameRE)
		check(hostNameRE.MatchString(h), "host_aliases: host %q of alias %q does not match %s", h, alias, hostNameRE)
	}
	if rootOK { // the repository stats need an absolute root
		errs = append(errs, a.checkProjects())
	}
	return errors.Join(errs...)
}

// checkProjects checks the projects of the answers: each key, each repository, the main
// repository, and the fills (checkFills). It reads no file of the working tree; it only stats
// each repository folder and its .git folder, and the file of each doc fill.
func (a *InitAnswers) checkProjects() error {
	var errs []error
	keys := make([]string, 0, len(a.Projects))
	owner := map[string]string{} // repository -> key of its project
	ledger := resolveExisting(a.LedgerPath)
	for _, p := range a.Projects {
		bad := func(format string, args ...any) {
			errs = append(errs, fmt.Errorf("project %q: %s", p.Key, fmt.Sprintf(format, args...)))
		}
		if !projectRE.MatchString(p.Key) || len(p.Key) > 40 {
			bad("the key must match %s and have at most 40 characters", projectRE)
		}
		if slices.Contains(keys, p.Key) {
			bad("the key is there twice")
		}
		keys = append(keys, p.Key)
		for _, r := range p.Repos {
			if !relPath(r) || path.Clean(r) != r {
				bad("repository %q is not a clean relative path with no ..", r)
				continue
			}
			if k, ok := owner[r]; ok {
				bad("repository %q is also in project %q", r, k)
			}
			owner[r] = p.Key
			dir := filepath.Join(a.Root, filepath.FromSlash(r))
			// Only stats: the check reads no file of the working tree (D5).
			if info, err := os.Stat(dir); err != nil || !info.IsDir() {
				bad("repository %q is not a folder under root %q", r, a.Root)
				continue
			}
			// Lstat as in walkRepos: a .git that is a symbolic link is not a repository (G14).
			if info, err := os.Lstat(filepath.Join(dir, ".git")); err != nil || !info.IsDir() {
				bad("repository %q has no .git folder", r)
				continue
			}
			if resolveExisting(dir) == ledger {
				bad("repository %q is the ledger %q", r, a.LedgerPath)
			}
		}
		if !slices.Contains(p.Repos, p.Main) {
			bad("main %q is not one of its repos", p.Main)
		}
	}
	for _, p := range a.Projects {
		errs = append(errs, checkFills(a.Root, p, keys))
	}
	return errors.Join(errs...)
}

type plannedFile struct {
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	Before  string `json:"before"` // SHA-256 of the current content, or "absent"
	Content string `json:"content"`
	Delete  bool   `json:"delete"`
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

// ledgerPathOK accepts a clean absolute path with no .. element whose nearest existing ancestor is a folder.
func ledgerPathOK(p string) bool {
	if !filepath.IsAbs(p) || filepath.Clean(p) != p || slices.Contains(strings.Split(filepath.ToSlash(p), "/"), "..") {
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
// workflowAllowRules are the allow rules of the plugin workflows that the role sessions launch.
var workflowAllowRules = []string{
	"Workflow(bruh:deliver)", "Workflow(bruh:tickets)", "Workflow(bruh:implement-tickets)",
	"Workflow(bruh:review-and-fix)", "Workflow(bruh:review-only)",
}

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
	if _, ok := top.vals["autoCompactWindow"]; a.AutoCompactWindow != 0 || !ok {
		top.set("autoCompactWindow", json.Number(strconv.Itoa(cmp.Or(a.AutoCompactWindow, 550000))))
	}
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
	for _, r := range append(slices.Clone(workflowAllowRules), mcpAllowRules(slices.Contains(a.Channels, "slack"))...) {
		if !slices.Contains(allow, any(r)) {
			allow = append(allow, r)
		}
	}
	perms.set("allow", allow)
	// Only the CLI sets the plugin options; init_plan refuses them (spec 16).
	if a.UserName != nil || a.HandoffPercent != nil || a.MaxBusyClerks != nil {
		opts := top.child("pluginConfigs").child(pluginID).child("options")
		if a.UserName != nil {
			opts.set("user_name", *a.UserName)
		}
		if a.HandoffPercent != nil {
			opts.set("handoff_percent", json.Number(strconv.Itoa(*a.HandoffPercent)))
		}
		if a.MaxBusyClerks != nil {
			opts.set("max_busy_clerks", json.Number(strconv.Itoa(*a.MaxBusyClerks)))
		}
	}
	return encodeOrdered(top, indentOf(old)), nil
}

// planLedgerSettings returns the start settings of bigm in the ledger folder. It starts from the
// existing file (an empty file is {}), or from the bigm role settings when there is no file. It sets
// agent to bruh:bigm and each env key of the role settings, so it replaces the old values of these
// keys. It adds each deny rule of the role settings that the file does not have yet, and keeps every
// other key. A file whose env, permissions, or permissions.deny has another JSON type is an error,
// so init does not drop such a value.
func planLedgerSettings(exists bool, old, role []byte) ([]byte, error) {
	r, err := parseOrdered(role)
	if err != nil {
		return nil, err
	}
	def := r.(*object) // roleSettings always writes a JSON object
	src := role
	if exists {
		src = old
	}
	top := newObject()
	if len(bytes.TrimSpace(src)) > 0 {
		v, err := parseOrdered(src)
		if err != nil {
			return nil, fmt.Errorf("ledger settings file: %w", err)
		}
		o, ok := v.(*object)
		if !ok {
			return nil, errors.New("ledger settings file is not a JSON object")
		}
		top = o
	}
	for _, k := range []string{"env", "permissions"} {
		if v, ok := top.vals[k]; ok {
			if _, ok := v.(*object); !ok {
				return nil, fmt.Errorf("ledger settings file: %s is not a JSON object", k)
			}
		}
	}
	perms := top.child("permissions")
	deny, ok := perms.vals["deny"].([]any)
	if _, has := perms.vals["deny"]; has && !ok {
		return nil, errors.New("ledger settings file: permissions.deny is not a JSON array")
	}
	top.set("agent", "bruh:bigm")
	env, defEnv := top.child("env"), def.child("env")
	for _, k := range defEnv.keys {
		env.set(k, defEnv.vals[k])
	}
	defDeny, _ := def.child("permissions").vals["deny"].([]any)
	for _, d := range defDeny {
		if !slices.Contains(deny, d) {
			deny = append(deny, d)
		}
	}
	perms.set("deny", deny)
	return encodeOrdered(top, indentOf(src)), nil
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
		return def, nil
	}
	return content, nil
}

// mergeLedgerFile returns the new content of the existing ledger file rel (a path of
// ledger-template/, with / separators) by the update rules of gap G24: the template text with the
// values and the data rows of the existing file.
func mergeLedgerFile(rel string, tmpl, existing []byte, pluginRoot string) ([]byte, error) {
	switch rel {
	case "README.md", "projects/_template.md":
		return tmpl, nil
	case "priorities.md":
		if slices.Contains(strings.Split(string(existing), "\n"), "## Never without the owner") {
			return existing, nil
		}
		return fillLedgerFile(rel, tmpl, InitAnswers{}, pluginRoot, "")
	case "rules.md":
		t, e := strings.Split(string(tmpl), "\n"), strings.Split(string(existing), "\n")
		i, j := slices.Index(t, "## Rules"), slices.Index(e, "## Rules")
		if i < 0 || j < 0 {
			return existing, nil // no rule section to anchor on: keep the rules of the owner as they are
		}
		return []byte(strings.Join(append(t[:i:i], e[j:]...), "\n")), nil
	}
	return mergeKeysAndTables(tmpl, existing), nil
}

var (
	// keyLineRE matches the start of a "key: value" line of a ledger file.
	keyLineRE = regexp.MustCompile(`^[a-z0-9_]+: `)
	// tableSepRE matches the separator line of a Markdown table.
	tableSepRE = regexp.MustCompile(`^\|(\s*:?-+:?\s*\|)+\s*$`)
)

// mdTable is one Markdown table of a ledger file: the header line, the separator line, and
// the data rows.
type mdTable struct {
	header, sep string
	rows        []string
}

// isTableHeader reports whether lines[i] is the header line of a Markdown table.
func isTableHeader(lines []string, i int) bool {
	return strings.HasPrefix(lines[i], "|") && i+1 < len(lines) && tableSepRE.MatchString(lines[i+1])
}

// tableRows returns the number of data rows of the table whose separator line is lines[sep].
func tableRows(lines []string, sep int) int {
	n := 0
	for sep+1+n < len(lines) && strings.HasPrefix(lines[sep+1+n], "|") {
		n++
	}
	return n
}

// mergeKeysAndTables returns tmpl with the values of the key lines and the data rows of the tables
// of existing (gap G24). A table of existing whose header line is not in tmpl goes to the end.
func mergeKeysAndTables(tmpl, existing []byte) []byte {
	values := map[string]string{}
	var tables []mdTable
	rows := map[string][]string{}
	e := strings.Split(string(existing), "\n")
	for i := 0; i < len(e); i++ {
		if k := keyLineRE.FindString(e[i]); k != "" {
			if _, ok := values[k]; !ok {
				values[k] = e[i][len(k):]
			}
		} else if isTableHeader(e, i) {
			n := tableRows(e, i+1)
			tables = append(tables, mdTable{header: e[i], sep: e[i+1], rows: e[i+2 : i+2+n]})
			rows[e[i]] = append(rows[e[i]], e[i+2:i+2+n]...)
			i += 1 + n
		}
	}
	t := strings.Split(string(tmpl), "\n")
	inTmpl := map[string]bool{}
	var out []string
	for i := 0; i < len(t); i++ {
		if k := keyLineRE.FindString(t[i]); k != "" {
			if v, ok := values[k]; ok {
				out = append(out, k+v)
				continue
			}
		} else if isTableHeader(t, i) {
			inTmpl[t[i]] = true
			out = append(out, t[i], t[i+1])
			if r, ok := rows[t[i]]; ok {
				// The existing rows replace the rows of the template, so a second merge adds nothing.
				out = append(out, r...)
				i += 1 + tableRows(t, i+1)
			} else {
				i++
			}
			continue
		}
		out = append(out, t[i])
	}
	var extra []string
	for _, tb := range tables {
		if !inTmpl[tb.header] {
			extra = append(append(extra, "", tb.header, tb.sep), tb.rows...)
		}
	}
	if len(extra) > 0 {
		if out[len(out)-1] == "" {
			out = out[:len(out)-1]
		}
		out = append(append(out, extra...), "")
	}
	return []byte(strings.Join(out, "\n"))
}

// planInit computes the files that init writes or deletes at the time at (the ledger rows carry
// it), and the learn plan of the projects of the answers. It writes nothing.
func planInit(env Env, a InitAnswers, at time.Time) ([]plannedFile, learnPlan, error) {
	if env.DataDir == "" {
		return nil, learnPlan{}, errors.New("BRUH_DATA is not set")
	}
	data, err := filepath.Abs(env.DataDir)
	if err != nil {
		return nil, learnPlan{}, err
	}
	root, err := filepath.Abs(env.PluginRoot)
	if err != nil {
		return nil, learnPlan{}, err
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
		return nil, learnPlan{}, err
	}
	settings, err := planSettings(old, a, tap)
	if err != nil {
		return nil, learnPlan{}, err
	}
	if err := add(env.SettingsFile, 0o600, settings); err != nil {
		return nil, learnPlan{}, err
	}
	if *a.WrapStatusline {
		src, err := os.ReadFile(filepath.Join(root, "scripts", "statusline-tap.sh"))
		if err != nil {
			return nil, learnPlan{}, err
		}
		if err := add(tap, 0o755, src); err != nil {
			return nil, learnPlan{}, err
		}
	}
	role, err := roleSettings(root, "bigm", nil, nil, nil)
	if err != nil {
		return nil, learnPlan{}, fmt.Errorf("bigm role settings: %w", err)
	}
	start := filepath.Join(a.LedgerPath, ".claude", "settings.json")
	before, old, err := fileHash(start)
	if err != nil {
		return nil, learnPlan{}, err
	}
	content, err := planLedgerSettings(before != "absent", old, role)
	if err != nil {
		return nil, learnPlan{}, err
	}
	if err := add(start, 0o644, content); err != nil {
		return nil, learnPlan{}, err
	}
	cfg, _ := json.MarshalIndent(initConfig{LedgerPath: a.LedgerPath}, "", "  ")
	if err := add(filepath.Join(data, "init", "config.json"), 0o600, append(cfg, '\n')); err != nil {
		return nil, learnPlan{}, err
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
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		existing, err := os.ReadFile(dest)
		switch {
		case err == nil:
			content, err = mergeLedgerFile(rel, content, existing, root)
		case errors.Is(err, fs.ErrNotExist):
			content, err = fillLedgerFile(rel, content, a, root, now)
		}
		if err != nil {
			return err
		}
		return add(dest, 0o644, content)
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, learnPlan{}, err
	}
	lp, err := planLearn(root, a)
	if err != nil {
		return nil, learnPlan{}, err
	}
	for _, p := range slices.Sorted(maps.Keys(lp.Write)) {
		if err := add(p, 0o644, lp.Write[p]); err != nil {
			return nil, learnPlan{}, err
		}
	}
	for _, p := range lp.Delete {
		if !targetAllowed(env, a, p) {
			return nil, learnPlan{}, fmt.Errorf("init refuses to delete %s: it is not under the data folder or the ledger folder", p)
		}
		before, _, err := fileHash(p)
		if err != nil {
			return nil, learnPlan{}, err
		}
		if before != "absent" {
			files = append(files, plannedFile{Path: p, Before: before, Delete: true})
		}
	}
	return files, lp, nil
}

func planDiff(files []plannedFile) (string, error) {
	var b strings.Builder
	for _, f := range files {
		d, err := fileDiff(f)
		if err != nil {
			return "", err
		}
		b.WriteString(d)
	}
	return b.String(), nil
}

// fileDiff returns the unified diff of one planned file against the file on disk.
func fileDiff(f plannedFile) (string, error) {
	from, to := f.Path, f.Path
	var old []byte
	if f.Before == "absent" {
		from = "/dev/null"
	} else {
		var err error
		if old, err = os.ReadFile(f.Path); err != nil {
			return "", err
		}
	}
	if f.Delete {
		to = "/dev/null"
	}
	return unifiedDiff(from, to, string(old), f.Content), nil
}

// splitDiff returns the diff of the files outside the ledger plus <ledger>/.claude/settings.json,
// and the diff of all other files of the ledger.
func splitDiff(files []plannedFile, ledger string) (outside, inLedger string, err error) {
	settings := resolveExisting(filepath.Join(ledger, ".claude", "settings.json"))
	var out, in strings.Builder
	for _, f := range files {
		d, err := fileDiff(f)
		if err != nil {
			return "", "", err
		}
		if inside(ledger, f.Path) && resolveExisting(f.Path) != settings {
			in.WriteString(d)
		} else {
			out.WriteString(d)
		}
	}
	return out.String(), in.String(), nil
}

func launchCommand(a InitAnswers) string {
	cmd := "cd " + shq(a.LedgerPath) + " && claude --agent bruh:bigm --name bigm --permission-mode auto"
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

// tableRow is one row of the ledger table of init_plan.
type tableRow struct {
	Key     string   `json:"key"`
	Purpose string   `json:"purpose"`
	Repos   []string `json:"repos"`
	Links   []string `json:"links"`
	Docs    []string `json:"docs"`
}

// ledgerTable returns one row for each project, sorted by key: the purpose text or "", the
// repository paths, each link as "<project key> (<repo>/<file>:<line>)", and each doc pointer as
// "<repo>/<path>".
func ledgerTable(projects []projectFile) []tableRow {
	rows := make([]tableRow, 0, len(projects))
	for _, p := range projects {
		row := tableRow{Key: p.Key, Repos: []string{}, Links: []string{}, Docs: []string{}}
		if p.Purpose != nil {
			row.Purpose = p.Purpose.Value
		}
		for _, r := range p.Repos {
			row.Repos = append(row.Repos, r.Path)
		}
		for _, l := range p.Links {
			row.Links = append(row.Links, fmt.Sprintf("%s (%s/%s:%d)", l.Project, l.Repo, l.File, l.Line))
		}
		for _, d := range p.Docs {
			row.Docs = append(row.Docs, d.Repo+"/"+d.Path)
		}
		rows = append(rows, row)
	}
	slices.SortFunc(rows, func(a, b tableRow) int { return strings.Compare(a.Key, b.Key) })
	return rows
}

// trustEntry is one folder of the trust step of init (spec 16, step 12).
type trustEntry struct {
	Path           string `json:"path"`
	ClaudeSettings *bool  `json:"claude_settings,omitempty"`
	MCP            *bool  `json:"mcp,omitempty"`
}

// trustList returns the ledger, then the absolute path of each repository of the answers, in the
// order of the answers. For a repository, the two booleans say whether .claude/settings.json and
// .mcp.json exist (os.Stat, no read).
func trustList(a InitAnswers) []trustEntry {
	exists := func(name string) *bool {
		_, err := os.Stat(name)
		return new(err == nil)
	}
	list := []trustEntry{{Path: a.LedgerPath}}
	for _, p := range a.Projects {
		for _, r := range p.Repos {
			dir := filepath.Join(a.Root, filepath.FromSlash(r))
			list = append(list, trustEntry{
				Path:           dir,
				ClaudeSettings: exists(filepath.Join(dir, ".claude", "settings.json")),
				MCP:            exists(filepath.Join(dir, ".mcp.json")),
			})
		}
	}
	return list
}

// initPlanRun plans init, keeps the plan in memory, and returns the tool result. It writes nothing.
func initPlanRun(env Env, a InitAnswers) (map[string]any, error) {
	if err := a.normalize(env.Home); err != nil {
		return nil, err
	}
	at := env.Now()
	files, lp, err := planInit(env, a, at)
	if err != nil {
		return nil, err
	}
	outside, inLedger, err := splitDiff(files, a.LedgerPath)
	if err != nil {
		return nil, err
	}
	diff := outside + inLedger
	id := strings.ToLower(rand.Text()[:16])
	plans.Lock()
	plans.m[id] = &storedPlan{answers: a, at: at, files: files, diffSHA: sha256Hex(diff)}
	plans.Unlock()
	paths := []string{}
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	kept := lp.Kept
	if kept == nil {
		kept = []string{}
	}
	return map[string]any{
		"plan_id":        id,
		"diff":           diff,
		"outside_diff":   outside,
		"ledger_diff":    inLedger,
		"diff_sha256":    sha256Hex(diff),
		"files":          paths,
		"launch_command": launchCommand(a),
		"trust":          trustList(a),
		"kept":           kept,
		"ledger_table":   ledgerTable(lp.Projects),
	}, nil
}

// initApplyRun writes the files of a plan of this process. It refuses the plan when diffSHA is
// not the hash of the diff that init_plan returned, when a target changed since init_plan (it
// plans again and compares), or when a target is outside the closed list.
func initApplyRun(env Env, id, diffSHA string) ([]plannedFile, error) {
	plans.Lock()
	p := plans.m[id]
	plans.Unlock()
	if p == nil {
		return nil, fmt.Errorf("no plan %q in this session; run init_plan again", id)
	}
	if diffSHA != p.diffSHA {
		return nil, errors.New("diff_sha256 is not the hash of the diff of this plan; show the diff of init_plan to the user and pass its diff_sha256")
	}
	now, _, err := planInit(env, p.answers, p.at)
	if err != nil {
		return nil, err
	}
	outside, inLedger, err := splitDiff(now, p.answers.LedgerPath)
	if err != nil {
		return nil, err
	}
	if sha256Hex(outside+inLedger) != p.diffSHA || !slices.Equal(now, p.files) {
		return nil, errors.New("a target changed after init_plan; nothing was written; run init_plan again")
	}
	applied, err := writePlanned(env, p.answers, p.files)
	if err != nil {
		return applied, err
	}
	plans.Lock()
	delete(plans.m, id)
	plans.Unlock()
	return applied, nil
}

// writePlanned writes or deletes each planned file, the settings file last, and returns the files
// that it applied. It refuses a target outside the closed list of targetAllowed.
func writePlanned(env Env, a InitAnswers, files []plannedFile) ([]plannedFile, error) {
	// The settings file goes last, so a failed write never leaves statusLine pointing at a missing tap.
	settings := resolveExisting(env.SettingsFile)
	files = slices.Clone(files)
	slices.SortStableFunc(files, func(x, y plannedFile) int {
		return cmp.Compare(boolInt(resolveExisting(x.Path) == settings), boolInt(resolveExisting(y.Path) == settings))
	})
	applied := []plannedFile{}
	for _, f := range files {
		if !targetAllowed(env, a, f.Path) {
			return applied, fmt.Errorf("init refuses to write %s", f.Path)
		}
		if f.Delete {
			if err := os.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return applied, err
			}
			applied = append(applied, f)
			continue
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
		applied = append(applied, f)
	}
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
			Description: "Plan /bruh:init: compute the diff of every file init would write or delete (user settings, the status line tap, the ledger layout, the start settings of bigm in <ledger>/.claude/settings.json, the index files learn/tree.json and learn/projects/<key>.json, and the project files projects/<key>.md). Writes only the plan. The result splits the diff: outside_diff has the files outside the ledger plus <ledger>/.claude/settings.json, ledger_diff has all other ledger files, and diff is outside_diff + ledger_diff with its diff_sha256. Show outside_diff in full and ledger_table as a table; show ledger_diff in full on \"show all\". kept lists the project file of each removed project that has a data row; init does not delete these files. Wait for an explicit yes before init_apply.",
			InputSchema: objectSchema(map[string]any{"answers": objectSchema(map[string]any{
				"ledger_path": str, "mode": map[string]any{"type": "string", "enum": []string{"human", "autonomous"}},
				"p1_batch_minutes": num, "p1_batch_size": num, "review_round_cap": num, "auto_compact_window": num,
				"wrap_statusline": map[string]any{"type": "boolean"}, "channels": list,
				"root": str, "depth": num, "exclude": list,
				"host_kinds":   map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string", "enum": []string{"github", "gitlab", "gitea"}}},
				"host_aliases": map[string]any{"type": "object", "additionalProperties": str},
				"projects": map[string]any{"type": "array", "items": objectSchema(map[string]any{
					"key": str, "repos": list, "main": str,
					"fills": map[string]any{"type": "array", "items": objectSchema(map[string]any{
						"field": map[string]any{"type": "string", "enum": []string{"purpose", "link", "doc", "host"}},
						"value": str, "source": map[string]any{"type": "string", "enum": []string{"agent", "owner"}},
						"repo": str, "file": str, "line": num,
					}, "field", "value", "source")},
				}, "key", "repos", "main")},
			}, "ledger_path")}, "answers"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Answers json.RawMessage `json:"answers"`
				}](raw)
				if err != nil {
					return nil, err
				}
				// The install dialog asks the plugin options and /config changes them (spec 16).
				var keys map[string]json.RawMessage
				if json.Unmarshal(a.Answers, &keys) == nil {
					for _, k := range []string{"user_name", "handoff_percent", "max_busy_clerks"} {
						if _, ok := keys[k]; ok {
							return nil, errors.New("user_name, handoff_percent, and max_busy_clerks are plugin options: the install dialog asks them, and /config changes them; init_plan does not write them")
						}
					}
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
			Description: "Apply a plan of init_plan of this session: write and delete the planned files (among them the index files learn/tree.json and learn/projects/<key>.json and the project files projects/<key>.md). Call it only after the user said yes to the split diff (outside_diff, ledger_table, and ledger_diff on \"show all\"). Pass the diff_sha256 of the init_plan result, the hash of diff = outside_diff + ledger_diff. Refuses a plan whose targets changed. applied lists each written and each deleted path; bigm commits these paths.",
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
				paths := []string{}
				for _, f := range applied {
					paths = append(paths, f.Path)
				}
				return map[string]any{"applied": paths}, nil
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
