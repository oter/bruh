package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ledgerTimeLayout is the layout of `date -u +%Y-%m-%dT%H:%M:%SZ`, so a time of ledger_edit and
// a time that a human writes look the same (spec 8.7).
const ledgerTimeLayout = "2006-01-02T15:04:05Z"

var (
	setKeyRE = regexp.MustCompile(`^[a-z0-9_]+$`)
	ruleIDRE = regexp.MustCompile(`^R-[0-9]+$`)
	// ruleHeadRE matches the heading line of a rule section of rules.md, and ruleLogRE the subject
	// of a commit that added or retired a rule.
	ruleHeadRE = regexp.MustCompile(`^## R-([0-9]+):`)
	ruleLogRE  = regexp.MustCompile(`^(?:add|close) rule: R-([0-9]+) `)
)

// ledgerActions are the actions of ledger_edit. The last six are the text actions of task 51
// (owner rule R-19): bigm writes no ledger file by hand.
var ledgerActions = []string{"add", "update", "close", "set_key", "add_line", "close_line", "add_rule", "close_rule", "new_project", "commit"}

type ledgerEditArgs struct {
	Action     string            `json:"action"`
	File       string            `json:"file"`
	Table      string            `json:"table"`
	Match      map[string]string `json:"match"`
	Cells      map[string]string `json:"cells"`
	Kind       string            `json:"kind"`
	Subject    string            `json:"subject"`
	Words      string            `json:"words"`
	Source     string            `json:"source"`
	DecisionBy string            `json:"decision_by"`
	Text       string            `json:"text"`
	Rule       string            `json:"rule"`
	Paths      []string          `json:"paths"`
}

type ledgerEditResult struct {
	SHA string            `json:"sha"`
	Row map[string]string `json:"row"`
}

// matchError is the error of a match count other than 1: the count, and the match-column cell
// texts of up to 10 candidate rows (owner decision 2026-10-04).
type matchError struct {
	Count      int                 `json:"count"`
	Candidates []map[string]string `json:"candidates"`
}

func (e *matchError) Error() string {
	data, _ := json.Marshal(e)
	return fmt.Sprintf("%d rows match, need exactly 1; nothing written: %s", e.Count, data)
}

func ledgerEditTool() Tool {
	str := map[string]any{"type": "string"}
	return Tool{
		Name:        "ledger_edit",
		Description: "Change the ledger and commit: one row or one key line (add, update, close, set_key), one bullet line of a section such as Decisions (add_line, close_line), one rule section of rules.md (add_rule, close_rule), a new project file from the template (new_project), or the listed files that init_apply or learn_refresh wrote (commit). It commits only those files and writes the DONE mail to clerk-ledger, whose waiter wakes it: send no nudge. Only bigm calls it; bigm writes no ledger file by hand (R-19). It never pushes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":      map[string]any{"type": "string", "enum": ledgerActions},
				"file":        map[string]any{"type": "string", "description": "Each action except commit: path relative to the ledger folder, for example owed.md, rules.md, or projects/shop.md; new_project: projects/<key>.md, which must not exist"},
				"table":       map[string]any{"type": "string", "description": "Text of the heading above the table (optional when the file has one table), or above the bullet lines for add_line and close_line, for example Decisions"},
				"match":       map[string]any{"type": "object", "additionalProperties": str, "description": "update and close: header text to exact cell text; exactly one row must match"},
				"cells":       map[string]any{"type": "object", "additionalProperties": str, "description": "add: each header to its value; update: the changed cells; set_key: one key to its value. A time column (header ends with (UTC)) takes now, now+<duration>, none, or empty"},
				"kind":        map[string]any{"type": "string", "description": "add, update, close, add_line, close_line, commit: one word, for example task, question, decision, or learn"},
				"subject":     map[string]any{"type": "string", "description": "One line for the commit subject; for add_rule, the subject of the rule heading"},
				"words":       map[string]any{"type": "string", "description": "close, close_line, close_rule, add_rule: the words of the owner, word for word, or the decision and reasons of bigm"},
				"source":      map[string]any{"type": "string", "description": "close, close_line, close_rule, add_rule: where the words come from, one line"},
				"decision_by": map[string]any{"type": "string", "enum": []string{"owner", "bigm"}, "description": "close, close_line, close_rule"},
				"text":        map[string]any{"type": "string", "description": "add_line, close_line: the bullet text without \"- \", one line; commit: notes for the commit body, for example the gone_docs of learn_refresh"},
				"rule":        map[string]any{"type": "string", "description": "close_rule: the rule ID, for example R-2"},
				"paths":       map[string]any{"type": "array", "items": str, "description": "commit: the changed files, relative to the ledger folder or absolute inside it, as init_apply and learn_refresh return them"},
			},
			"required": []string{"action", "subject"},
		},
		Handler: func(c *Call, raw json.RawMessage) (any, error) {
			me, err := c.Env.Caller()
			if err != nil {
				return nil, err
			}
			if me != "bigm" {
				return nil, errors.New("only bigm calls ledger_edit")
			}
			a, err := decode[ledgerEditArgs](raw)
			if err != nil {
				return nil, err
			}
			return ledgerEdit(c.Env, a)
		},
	}
}

func hasControl(s string, allowed ...rune) bool {
	return strings.IndexFunc(s, func(r rune) bool { return unicode.IsControl(r) && !slices.Contains(allowed, r) }) >= 0
}

// ledgerLine is a subject, a kind, or a source: one line, not empty, with no control character.
func ledgerLine(s string) bool { return s != "" && !hasControl(s) }

// checkLedgerArgs checks the inputs before any file read.
func checkLedgerArgs(a ledgerEditArgs) error {
	if !slices.Contains(ledgerActions, a.Action) {
		return fmt.Errorf("unknown action %q: use one of %s", a.Action, strings.Join(ledgerActions, ", "))
	}
	if !ledgerLine(a.Subject) {
		return errors.New("subject must be one line with no control character")
	}
	needKind := !slices.Contains([]string{"set_key", "add_rule", "close_rule", "new_project"}, a.Action)
	if needKind && (!ledgerLine(a.Kind) || strings.Contains(a.Kind, " ")) {
		return fmt.Errorf("kind must be one word, got %q", a.Kind)
	}
	switch a.Action {
	case "add_line", "close_line":
		if !ledgerLine(a.Text) || strings.Trim(a.Text, " \t") != a.Text {
			return fmt.Errorf("%s needs text: one line with no control character and no space or tab at either end", a.Action)
		}
	case "close_rule":
		if !ruleIDRE.MatchString(a.Rule) {
			return fmt.Errorf("close_rule needs rule, for example R-2, got %q", a.Rule)
		}
	case "commit":
		if len(a.Paths) == 0 {
			return errors.New("commit needs paths")
		}
		if hasControl(a.Text, '\n', '\t') {
			return errors.New("text has a control character other than a newline and a tab")
		}
	}
	for what, m := range map[string]map[string]string{"cells": a.Cells, "match": a.Match} {
		for k, v := range m {
			if hasControl(k) || hasControl(v) || strings.Trim(v, " \t") != v {
				return fmt.Errorf("%s[%q] = %q: a value is one line with no control character and no space or tab at either end", what, k, v)
			}
		}
	}
	switch a.Action {
	case "set_key":
		if len(a.Cells) != 1 {
			return errors.New("set_key needs exactly one pair in cells: the key and its value")
		}
		for k := range a.Cells {
			if !setKeyRE.MatchString(k) {
				return fmt.Errorf("invalid key %q: use [a-z0-9_]+", k)
			}
		}
	case "add", "update":
		if len(a.Cells) == 0 {
			return fmt.Errorf("%s needs cells", a.Action)
		}
	}
	if (a.Action == "update" || a.Action == "close") && len(a.Match) == 0 {
		return fmt.Errorf("%s needs match", a.Action)
	}
	if quotesWords(a.Action) {
		if a.Words == "" || hasControl(a.Words, '\n', '\t') {
			return fmt.Errorf("%s needs words, with no control character other than a newline and a tab", a.Action)
		}
		if !ledgerLine(a.Source) {
			return fmt.Errorf("%s needs source, one line", a.Action)
		}
		if a.Action != "add_rule" && a.DecisionBy != "owner" && a.DecisionBy != "bigm" {
			return fmt.Errorf("%s needs decision_by: owner or bigm", a.Action)
		}
	}
	return nil
}

// quotesWords reports whether the commit body of action quotes words: each close, and add_rule.
func quotesWords(action string) bool {
	return strings.HasPrefix(action, "close") || action == "add_rule"
}

// ledgerFile checks the file rule and returns the absolute path: an existing regular *.md file
// inside the ledger folder, not under learn/, .git/, or .claude/.
func ledgerFile(ledger, file string) (string, error) {
	rel := filepath.Clean(file)
	first, _, _ := strings.Cut(filepath.ToSlash(rel), "/")
	path := filepath.Join(ledger, rel)
	if !relPath(file) || !strings.HasSuffix(rel, ".md") || slices.Contains([]string{"learn", ".git", ".claude"}, first) || !inside(ledger, path) {
		return "", fmt.Errorf("file %q: use an existing *.md file of the ledger folder, not under learn/, .git/, or .claude/", file)
	}
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return "", fmt.Errorf("file %q is not a regular file of the ledger", file)
	}
	return path, nil
}

// ledgerGit runs git in the ledger folder, with literal pathspecs, and returns its standard output.
func ledgerGit(ledger, stdin string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"--literal-pathspecs", "-C", ledger}, args...)...)
	cmd.Stdin = strings.NewReader(stdin)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(out)+stderr.String()))
	}
	return string(out), nil
}

// splitPipes splits a line at the pipes that are not escaped with a backslash.
func splitPipes(line string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(line); i++ {
		if line[i] == '|' && (i == 0 || line[i-1] != '\\') {
			parts = append(parts, line[start:i])
			start = i + 1
		}
	}
	return append(parts, line[start:])
}

// cellText is the cell-text rule (owner decision 2026-10-04): read \| as |, and trim only spaces
// and tabs. No backtick is trimmed, so tableCells is not used.
func cellText(raw string) string {
	return strings.ReplaceAll(strings.Trim(raw, " \t"), `\|`, "|")
}

// rowCells returns the cell texts of a table line. Text after the last pipe is a cell too.
func rowCells(line string) []string {
	parts := splitPipes(line)
	cells := parts[1 : len(parts)-1]
	if strings.Trim(parts[len(parts)-1], " \t") != "" {
		cells = parts[1:]
	}
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = cellText(c)
	}
	return out
}

func fmtCell(v string) string { return " " + strings.ReplaceAll(v, "|", `\|`) + " " }

// setRowCells replaces the raw cells of a row by column index, so each other cell keeps its
// bytes. A short row gets empty cells up to the column.
func setRowCells(line string, vals map[int]string) string {
	parts := splitPipes(line)
	if strings.Trim(parts[len(parts)-1], " \t") != "" {
		parts = append(parts, "") // close the row, so the text after the last pipe is a cell
	}
	for ci, v := range vals {
		for ci+2 > len(parts)-1 {
			parts = slices.Insert(parts, len(parts)-1, " ")
		}
		parts[ci+1] = fmtCell(v)
	}
	return strings.Join(parts, "|")
}

// timeToken turns now, now+<Go duration>, none, or empty into the value that ledger_edit writes
// (owner decision 2026-10-04). Each time comes from the server clock.
func timeToken(v string, now time.Time) (string, bool) {
	switch {
	case v == "" || v == "none":
		return v, true
	case v == "now":
		return now.Format(ledgerTimeLayout), true
	case strings.HasPrefix(v, "now+"):
		d, err := time.ParseDuration(v[len("now+"):])
		if err != nil {
			return "", false
		}
		return now.Add(d).Format(ledgerTimeLayout), true
	}
	return "", false
}

type editTable struct {
	header, rows int // the index of the header line, and the number of data rows
	cols         []string
}

// findTable returns the table under the heading name (the nearest line above the header that
// starts with "#"), or the one table of the file when name is empty.
func findTable(lines []string, name string) (editTable, error) {
	var found []editTable
	count := 0
	for i := 0; i < len(lines); i++ {
		if !isTableHeader(lines, i) {
			continue
		}
		count++
		heading := ""
		for j := i - 1; j >= 0; j-- {
			if strings.HasPrefix(lines[j], "#") {
				heading = strings.TrimSpace(strings.TrimLeft(lines[j], "#"))
				break
			}
		}
		t := editTable{header: i, rows: tableRows(lines, i+1)}
		if name == "" || heading == name {
			found = append(found, t)
		}
		i += 1 + t.rows
	}
	switch {
	case name == "" && count != 1:
		return editTable{}, fmt.Errorf("the file has %d tables; name one with table", count)
	case len(found) == 0:
		return editTable{}, fmt.Errorf("no table under the heading %q", name)
	case len(found) > 1:
		return editTable{}, fmt.Errorf("%d tables under the heading %q", len(found), name)
	}
	t := found[0]
	t.cols = rowCells(lines[t.header])
	for i, c := range t.cols {
		if c == "" || slices.Index(t.cols, c) != i {
			return editTable{}, fmt.Errorf("table %q does not parse: an empty or repeated header cell %q", name, c)
		}
	}
	return t, nil
}

func (t editTable) col(name string) (int, error) {
	i := slices.Index(t.cols, name)
	if i < 0 {
		return 0, fmt.Errorf("unknown column %q; the columns are %q", name, t.cols)
	}
	return i, nil
}

// rowMap maps each header (or only the headers of only, when it is not nil) to the cell text of
// a row. A missing cell of a short row reads as empty.
func (t editTable) rowMap(line string, only map[string]string) map[string]string {
	cells := rowCells(line)
	m := map[string]string{}
	for i, c := range t.cols {
		if _, ok := only[c]; only != nil && !ok {
			continue
		}
		m[c] = ""
		if i < len(cells) {
			m[c] = cells[i]
		}
	}
	return m
}

// lineEdit is the one line change of a call: change line at, insert a line before at, or delete
// line at. eolOf is the line whose line ending a new line takes.
type lineEdit struct {
	op        byte // 'c', 'i', or 'd'
	at, eolOf int
	text      string // the new line without its line ending, for 'c' and 'i'
	row       map[string]string
}

// planEdit finds the line that the action changes, in the lines without "\r".
func planEdit(lines []string, a ledgerEditArgs, now time.Time) (lineEdit, error) {
	if a.Action == "set_key" {
		var key, val string
		for key, val = range a.Cells {
		}
		if val == "now" || strings.HasPrefix(val, "now+") {
			v, ok := timeToken(val, now)
			if !ok {
				return lineEdit{}, fmt.Errorf("bad time token %q: use now or now+<duration>", val)
			}
			val = v
		}
		var hits []int
		fence := false
		for i, l := range lines {
			if t := strings.TrimLeft(l, " "); strings.HasPrefix(t, "```") || strings.HasPrefix(t, "~~~") {
				fence = !fence
			} else if !fence && strings.HasPrefix(l, key+": ") {
				hits = append(hits, i)
			}
		}
		if len(hits) != 1 {
			return lineEdit{}, fmt.Errorf("%d lines start with %q outside code blocks, need exactly 1; nothing written", len(hits), key+": ")
		}
		return lineEdit{op: 'c', at: hits[0], text: key + ": " + val, row: map[string]string{key: val}}, nil
	}
	t, err := findTable(lines, a.Table)
	if err != nil {
		return lineEdit{}, err
	}
	vals := map[int]string{}
	for c, v := range a.Cells {
		ci, err := t.col(c)
		if err != nil {
			return lineEdit{}, err
		}
		if strings.HasSuffix(c, "(UTC)") {
			tv, ok := timeToken(v, now)
			if !ok {
				return lineEdit{}, fmt.Errorf("time column %q takes now, now+<duration>, none, or empty, not %q", c, v)
			}
			v = tv
		}
		vals[ci] = v
	}
	first := t.header + 2
	if a.Action == "add" {
		for _, c := range t.cols {
			if _, ok := a.Cells[c]; !ok {
				return lineEdit{}, fmt.Errorf("add needs each column in cells; missing %q", c)
			}
		}
		text := "|"
		for i := range t.cols {
			text += fmtCell(vals[i]) + "|"
		}
		return lineEdit{op: 'i', at: first + t.rows, eolOf: t.header, text: text, row: t.rowMap(text, nil)}, nil
	}
	for c := range a.Match {
		if _, err := t.col(c); err != nil {
			return lineEdit{}, err
		}
	}
	var hits, all []int
	for i := first; i < first+t.rows; i++ {
		all = append(all, i)
		if maps.Equal(t.rowMap(lines[i], a.Match), a.Match) {
			hits = append(hits, i)
		}
	}
	if len(hits) != 1 {
		cand := hits
		if len(hits) == 0 {
			cand = all
		}
		e := &matchError{Count: len(hits), Candidates: []map[string]string{}}
		for _, i := range cand[:min(10, len(cand))] {
			e.Candidates = append(e.Candidates, t.rowMap(lines[i], a.Match))
		}
		return lineEdit{}, e
	}
	i := hits[0]
	if a.Action == "close" {
		return lineEdit{op: 'd', at: i, row: t.rowMap(lines[i], nil)}, nil
	}
	text := setRowCells(lines[i], vals)
	return lineEdit{op: 'c', at: i, text: text, row: t.rowMap(text, nil)}, nil
}

// applyEdit applies e to the raw lines of a file (split at "\n", each with its "\r" if any), so
// each other byte stays: the CRLF endings and a missing last newline too.
func applyEdit(raw []string, e lineEdit) []string {
	eol := func(i int) string {
		if strings.HasSuffix(raw[i], "\r") {
			return "\r"
		}
		return ""
	}
	switch e.op {
	case 'c':
		raw[e.at] = e.text + eol(e.at)
	case 'i':
		end := eol(e.eolOf)
		if e.at == len(raw) { // after the last line of a file with no last newline
			raw[e.at-1] += end
			return append(raw, e.text)
		}
		return slices.Insert(raw, e.at, e.text+end)
	case 'd':
		if e.at == len(raw)-1 && e.at > 0 { // the last line of a file with no last newline
			raw[e.at-1] = strings.TrimSuffix(raw[e.at-1], "\r")
		}
		return slices.Delete(raw, e.at, e.at+1)
	}
	return raw
}

// ledgerEdit runs one ledger_edit call after the caller check (spec 8.7, design.md L55).
func ledgerEdit(env Env, a ledgerEditArgs) (any, error) {
	if err := checkLedgerArgs(a); err != nil {
		return nil, err
	}
	ledger, err := ledgerPath(env)
	if err != nil {
		return nil, err
	}
	var path string
	switch a.Action {
	case "commit":
		a.Paths, err = commitPaths(ledger, a.Paths)
	case "new_project":
		path, err = newProjectFile(ledger, a.File)
	default:
		path, err = ledgerFile(ledger, a.File)
	}
	if err != nil {
		return nil, err
	}
	if path != "" {
		a.Paths = []string{filepath.ToSlash(filepath.Clean(a.File))}
	}
	rels := a.Paths
	now := env.Now().UTC()
	var res ledgerEditResult
	err = env.WithLock("ledger", func() error {
		var restore func() error
		var err error
		if a.Action == "commit" {
			restore, err = stageChanged(ledger, rels)
		} else {
			restore, res.Row, err = writeLedgerFile(env, ledger, path, rels[0], &a, now)
		}
		if err != nil {
			return err
		}
		msg := commitMessage(a, now)
		if _, err := ledgerGit(ledger, msg, append([]string{"commit", "--only", "--cleanup=verbatim", "-F", "-", "--"}, rels...)...); err != nil {
			return errors.Join(fmt.Errorf("commit failed; %s and its index entry restored", strings.Join(rels, ", ")), err, restore())
		}
		sha, err := ledgerGit(ledger, "", "rev-parse", "HEAD")
		if err != nil {
			return err
		}
		short, err := ledgerGit(ledger, "", "rev-parse", "--short", "HEAD")
		if err != nil {
			return err
		}
		sha, short = strings.TrimSpace(sha), strings.TrimSpace(short)
		header := "DONE: ledger commit " + short
		subject, _, _ := strings.Cut(msg, "\n")
		// The waiter of clerk-ledger (spec 9.5) wakes it on this mail, so the output has no nudge.
		if _, err := postMail(env, "bigm", "clerk-ledger", header, fmt.Sprintf("Commit: %s\nSubject: %s\nFile: %s\n", sha, subject, strings.Join(rels, " "))); err != nil {
			return fmt.Errorf("committed %s, but the DONE mail failed: %w; post %q to clerk-ledger with mail_post", sha, err, header)
		}
		res.SHA = sha
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// commitPaths checks the paths of a commit and returns them relative to the ledger folder, with /
// separators: each path is relative with no .., or absolute inside the ledger folder, as init_apply
// and learn_refresh return them, and not under .git/.
func commitPaths(ledger string, paths []string) ([]string, error) {
	var rels []string
	for _, p := range paths {
		abs := p
		if !filepath.IsAbs(p) {
			abs = filepath.Join(ledger, p)
		}
		rel, err := filepath.Rel(resolveExisting(ledger), resolveExisting(abs))
		rel = filepath.ToSlash(rel)
		first, _, _ := strings.Cut(rel, "/")
		if hasControl(p) || (!filepath.IsAbs(p) && !relPath(p)) || !inside(ledger, abs) || err != nil || first == ".git" {
			return nil, fmt.Errorf("path %q: use a path inside the ledger folder, not under .git/", p)
		}
		rels = append(rels, rel)
	}
	return rels, nil
}

// stageChanged stages the paths of a commit, after it checks that each one has a change (any
// output of git status, an untracked or a deleted file too). The restore unstages them.
func stageChanged(ledger string, rels []string) (func() error, error) {
	for _, r := range rels {
		out, err := ledgerGit(ledger, "", "status", "--porcelain", "--", r)
		if err != nil {
			return nil, err
		}
		if out == "" {
			return nil, fmt.Errorf("%s has no change to commit; nothing committed", r)
		}
	}
	restore := func() error {
		_, err := ledgerGit(ledger, "", append([]string{"reset", "-q", "--"}, rels...)...)
		return err
	}
	if _, err := ledgerGit(ledger, "", append([]string{"add", "-A", "--"}, rels...)...); err != nil {
		return nil, errors.Join(err, restore())
	}
	return restore, nil
}

// newProjectFile checks the file of new_project and returns its absolute path: projects/<key>.md,
// where <key> is a project key (projectRE, at most 40 characters, as init checks it).
func newProjectFile(ledger, file string) (string, error) {
	rel := filepath.ToSlash(filepath.Clean(file))
	dir, name, _ := strings.Cut(rel, "/")
	key, ok := strings.CutSuffix(name, ".md")
	p := filepath.Join(ledger, filepath.FromSlash(rel))
	if !relPath(file) || dir != "projects" || !ok || !projectRE.MatchString(key) || len(key) > 40 || !inside(ledger, p) {
		return "", fmt.Errorf("file %q: new_project makes projects/<key>.md, where <key> matches %s", file, projectRE)
	}
	return p, nil
}

// writeLedgerFile writes the change of one file action, after it checks that the file has no
// uncommitted change, and stages a new file. It returns the restore, which puts back the old bytes
// and the index entry, and the row of the output.
func writeLedgerFile(env Env, ledger, path, rel string, a *ledgerEditArgs, now time.Time) (func() error, map[string]string, error) {
	out, err := ledgerGit(ledger, "", "status", "--porcelain", "--", rel)
	if err != nil {
		return nil, nil, err
	}
	if out != "" {
		return nil, nil, fmt.Errorf("%s has uncommitted changes; nothing written: %s", rel, strings.TrimSpace(out))
	}
	reset := func() error {
		_, err := ledgerGit(ledger, "", "reset", "-q", "--", rel)
		return err
	}
	if a.Action == "new_project" {
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			return nil, nil, fmt.Errorf("%s exists; nothing written", rel)
		}
		tmpl, err := os.ReadFile(filepath.Join(env.PluginRoot, "ledger-template", "projects", "_template.md"))
		if err != nil {
			return nil, nil, err
		}
		key := strings.TrimSuffix(filepath.Base(path), ".md")
		// The same content as the project file that init makes (planLearn).
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, nil, err
		}
		restore := func() error { return errors.Join(reset(), os.Remove(path)) }
		if err := atomicWrite(path, []byte(strings.ReplaceAll(string(tmpl), "<project>", key))); err != nil {
			return nil, nil, err
		}
		if err := os.Chmod(path, 0o644); err != nil { // atomicWrite makes a 0600 file
			return nil, nil, errors.Join(err, restore())
		}
		if _, err := ledgerGit(ledger, "", "add", "--", rel); err != nil {
			return nil, nil, errors.Join(err, restore())
		}
		return restore, map[string]string{"project": key}, nil
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	old, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	raw := strings.Split(string(old), "\n")
	lines := make([]string, len(raw))
	for i, l := range raw {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	var content string
	var row map[string]string
	switch a.Action {
	case "add_rule":
		log, err := ledgerGit(ledger, "", "log", "--format=%s")
		if err != nil {
			return nil, nil, err
		}
		content, row = addRule(string(old), lines, log, a, now)
	case "close_rule":
		content, row, err = closeRule(raw, lines, a.Rule)
	case "add_line", "close_line":
		content, row, err = editLine(raw, lines, a)
	default:
		var e lineEdit
		e, err = planEdit(lines, *a, now)
		content, row = strings.Join(applyEdit(raw, e), "\n"), e.row
	}
	if err != nil {
		return nil, nil, err
	}
	restore := func() error {
		werr := atomicWrite(path, old)
		return errors.Join(werr, os.Chmod(path, fi.Mode().Perm()), reset())
	}
	if err := atomicWrite(path, []byte(content)); err != nil {
		return nil, nil, err
	}
	if err := os.Chmod(path, fi.Mode().Perm()); err != nil { // atomicWrite makes a 0600 file
		return nil, nil, errors.Join(err, restore())
	}
	return restore, row, nil
}

// addRule appends the next rule section to rules.md, in the form of the ledger template, after one
// empty line. The next ID is 1 more than the highest ID of a rule heading of the file and of a
// subject "add rule: R-<n> - ..." or "close rule: R-<n> - ..." of the ledger history, so a retired
// ID is not used again. The date and the tag come from the server clock. It sets a.Rule.
func addRule(old string, lines []string, log string, a *ledgerEditArgs, now time.Time) (string, map[string]string) {
	n := 0
	for _, l := range append(lines, strings.Split(log, "\n")...) {
		m := ruleHeadRE.FindStringSubmatch(l)
		if m == nil {
			m = ruleLogRE.FindStringSubmatch(l)
		}
		if m != nil {
			v, _ := strconv.Atoi(m[1])
			n = max(n, v)
		}
	}
	a.Rule = fmt.Sprintf("R-%d", n+1)
	if old != "" && !strings.HasSuffix(old, "\n") {
		old += "\n"
	}
	if old != "" && !strings.HasSuffix(old, "\n\n") {
		old += "\n"
	}
	old += fmt.Sprintf("## %s: %s\n\n- Words: \"%s\"\n- Date: %s\n- Source: %s\n- Tag: owner decision %s\n",
		a.Rule, a.Subject, a.Words, now.Format(ledgerTimeLayout), a.Source, now.Format(time.DateOnly))
	return old, map[string]string{"rule": a.Rule}
}

// isSectionHead reports whether l is a heading of level 1 or 2, the end of a rule section.
func isSectionHead(l string) bool { return strings.HasPrefix(l, "# ") || strings.HasPrefix(l, "## ") }

// closeRule deletes the section "## <rule>: ..." of rules.md up to the next heading of level 1 or
// 2. The last section goes with the empty lines above it.
func closeRule(raw, lines []string, rule string) (string, map[string]string, error) {
	var hits []int
	for i, l := range lines {
		if strings.HasPrefix(l, "## "+rule+":") {
			hits = append(hits, i)
		}
	}
	if len(hits) != 1 {
		return "", nil, fmt.Errorf("%d sections start with %q, need exactly 1; nothing written", len(hits), "## "+rule+":")
	}
	start, end := hits[0], len(raw)
	for i := start + 1; i < len(lines); i++ {
		if isSectionHead(lines[i]) {
			end = i
			break
		}
	}
	row := map[string]string{"rule": rule}
	if end == len(raw) {
		return strings.TrimRight(strings.Join(raw[:start], "\n"), "\r\n") + "\n", row, nil
	}
	return strings.Join(slices.Concat(raw[:start], raw[end:]), "\n"), row, nil
}

// editLine adds the bullet line "- <text>" after the last line of the section under the heading
// a.Table (up to the next heading), or deletes the one bullet line that equals it. An empty section
// gets an empty line, then the bullet.
func editLine(raw, lines []string, a *ledgerEditArgs) (string, map[string]string, error) {
	var heads []int
	for i, l := range lines {
		if strings.HasPrefix(l, "#") && strings.TrimSpace(strings.TrimLeft(l, "#")) == a.Table {
			heads = append(heads, i)
		}
	}
	if len(heads) != 1 {
		return "", nil, fmt.Errorf("%d headings %q, need exactly 1; nothing written", len(heads), a.Table)
	}
	h, end := heads[0], len(lines)
	for i := h + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "#") {
			end = i
			break
		}
	}
	item := "- " + a.Text
	row := map[string]string{"text": a.Text}
	if a.Action == "close_line" {
		var hits []int
		for i := h + 1; i < end; i++ {
			if lines[i] == item {
				hits = append(hits, i)
			}
		}
		if len(hits) != 1 {
			return "", nil, fmt.Errorf("%d lines %q under %q, need exactly 1; nothing written", len(hits), item, a.Table)
		}
		return strings.Join(applyEdit(raw, lineEdit{op: 'd', at: hits[0]}), "\n"), row, nil
	}
	last := h
	for i := h + 1; i < end; i++ {
		if strings.TrimSpace(lines[i]) != "" {
			last = i
		}
	}
	raw = applyEdit(raw, lineEdit{op: 'i', at: last + 1, eolOf: h, text: item})
	if last == h {
		raw = applyEdit(raw, lineEdit{op: 'i', at: h + 1, eolOf: h})
	}
	return strings.Join(raw, "\n"), row, nil
}

// commitMessage is the subject of spec 8.6 and the body of spec 8.7. The close body quotes the
// words as a > block (owner decision 2026-10-04); the body of add_rule does too.
func commitMessage(a ledgerEditArgs, now time.Time) string {
	stamp := now.Format(ledgerTimeLayout)
	if a.Action == "set_key" {
		for k := range a.Cells {
			return fmt.Sprintf("set %s: %s\n\nRecorded (UTC): %s\n", k, a.Subject, stamp)
		}
	}
	verb, kind, subject := a.Action, a.Kind, a.Subject
	switch a.Action {
	case "add_line", "close_line":
		verb, _, _ = strings.Cut(a.Action, "_")
	case "add_rule", "close_rule":
		verb, _, _ = strings.Cut(a.Action, "_")
		kind, subject = "rule", a.Rule+" - "+a.Subject
	case "new_project":
		verb, kind = "add", "project"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s\n\n", verb, kind, subject)
	if a.Action == "commit" {
		b.WriteString("Paths:\n")
		for _, p := range a.Paths {
			b.WriteString("- " + p + "\n")
		}
		if a.Text != "" {
			b.WriteString("Notes:\n" + strings.TrimRight(a.Text, "\n") + "\n")
		}
	}
	if !quotesWords(a.Action) {
		fmt.Fprintf(&b, "Recorded (UTC): %s\n", stamp)
		return b.String()
	}
	by := a.DecisionBy
	if a.Action == "add_rule" {
		by = "owner" // a rule is a standing rule of the owner
	}
	b.WriteString("Words:\n")
	for l := range strings.SplitSeq(a.Words, "\n") {
		if l == "" {
			b.WriteString(">\n")
		} else {
			b.WriteString("> " + l + "\n")
		}
	}
	fmt.Fprintf(&b, "Source: %s\nRecorded (UTC): %s\nTag: %s decision %s\n", a.Source, stamp, by, now.Format(time.DateOnly))
	return b.String()
}
