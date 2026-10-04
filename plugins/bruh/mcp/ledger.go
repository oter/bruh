package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
)

// ledgerTimeLayout is the layout of `date -u +%Y-%m-%dT%H:%M:%SZ`, so a time of ledger_edit and
// a time that a human writes look the same (spec 8.7).
const ledgerTimeLayout = "2006-01-02T15:04:05Z"

var setKeyRE = regexp.MustCompile(`^[a-z0-9_]+$`)

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
}

type ledgerNudge struct {
	To     string `json:"to"`
	Header string `json:"header"`
}

type ledgerEditResult struct {
	SHA   string            `json:"sha"`
	Row   map[string]string `json:"row"`
	Nudge ledgerNudge       `json:"nudge"`
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
		Description: "Change one row or one key line of a ledger file, commit only that file, and write the DONE mail to clerk-ledger. Then send nudge.header to nudge.to with SendMessage. Only bigm calls it. It never pushes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action":      map[string]any{"type": "string", "enum": []string{"add", "update", "close", "set_key"}},
				"file":        map[string]any{"type": "string", "description": "Path relative to the ledger folder, for example owed.md or projects/shop.md"},
				"table":       map[string]any{"type": "string", "description": "Text of the heading above the table; optional when the file has one table"},
				"match":       map[string]any{"type": "object", "additionalProperties": str, "description": "update and close: header text to exact cell text; exactly one row must match"},
				"cells":       map[string]any{"type": "object", "additionalProperties": str, "description": "add: each header to its value; update: the changed cells; set_key: one key to its value. A time column (header ends with (UTC)) takes now, now+<duration>, none, or empty"},
				"kind":        map[string]any{"type": "string", "description": "add, update, close: one word, for example task or question"},
				"subject":     map[string]any{"type": "string", "description": "One line for the commit subject"},
				"words":       map[string]any{"type": "string", "description": "close: the words of the owner, word for word, or the decision and reasons of bigm"},
				"source":      map[string]any{"type": "string", "description": "close: where the words come from, one line"},
				"decision_by": map[string]any{"type": "string", "enum": []string{"owner", "bigm"}},
			},
			"required": []string{"action", "file", "subject"},
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
	if !slices.Contains([]string{"add", "update", "close", "set_key"}, a.Action) {
		return fmt.Errorf("unknown action %q: use add, update, close, or set_key", a.Action)
	}
	if !ledgerLine(a.Subject) {
		return errors.New("subject must be one line with no control character")
	}
	if a.Action != "set_key" && (!ledgerLine(a.Kind) || strings.Contains(a.Kind, " ")) {
		return fmt.Errorf("kind must be one word, got %q", a.Kind)
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
	if a.Action == "close" {
		if a.Words == "" || hasControl(a.Words, '\n', '\t') {
			return errors.New("close needs words, with no control character other than a newline and a tab")
		}
		if !ledgerLine(a.Source) {
			return errors.New("close needs source, one line")
		}
		if a.DecisionBy != "owner" && a.DecisionBy != "bigm" {
			return errors.New("close needs decision_by: owner or bigm")
		}
	}
	return nil
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

// ledgerEdit runs one ledger_edit call after the caller check (spec 8.7, design.md L51).
func ledgerEdit(env Env, a ledgerEditArgs) (any, error) {
	if err := checkLedgerArgs(a); err != nil {
		return nil, err
	}
	ledger, err := ledgerPath(env)
	if err != nil {
		return nil, err
	}
	path, err := ledgerFile(ledger, a.File)
	if err != nil {
		return nil, err
	}
	rel := filepath.ToSlash(filepath.Clean(a.File))
	now := env.Now().UTC()
	var res ledgerEditResult
	err = env.WithLock("ledger", func() error {
		out, err := ledgerGit(ledger, "", "status", "--porcelain", "--", rel)
		if err != nil {
			return err
		}
		if out != "" {
			return fmt.Errorf("%s has uncommitted changes; nothing written: %s", rel, strings.TrimSpace(out))
		}
		fi, err := os.Stat(path)
		if err != nil {
			return err
		}
		old, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		raw := strings.Split(string(old), "\n")
		lines := make([]string, len(raw))
		for i, l := range raw {
			lines[i] = strings.TrimSuffix(l, "\r")
		}
		e, err := planEdit(lines, a, now)
		if err != nil {
			return err
		}
		restore := func(cause error) error {
			werr := atomicWrite(path, old)
			_, rerr := ledgerGit(ledger, "", "reset", "-q", "--", rel)
			return errors.Join(fmt.Errorf("commit failed; %s and its index entry restored", rel), cause, werr, os.Chmod(path, fi.Mode().Perm()), rerr)
		}
		if err := atomicWrite(path, []byte(strings.Join(applyEdit(raw, e), "\n"))); err != nil {
			return err
		}
		if err := os.Chmod(path, fi.Mode().Perm()); err != nil { // atomicWrite makes a 0600 file
			return restore(err)
		}
		msg := commitMessage(a, now)
		if _, err := ledgerGit(ledger, msg, "commit", "--only", "--cleanup=verbatim", "-F", "-", "--", rel); err != nil {
			return restore(err)
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
		if _, err := postMail(env, "bigm", "clerk-ledger", header, fmt.Sprintf("Commit: %s\nSubject: %s\nFile: %s\n", sha, subject, rel)); err != nil {
			return fmt.Errorf("committed %s, but the DONE mail failed: %w; post %q to clerk-ledger with mail_post", sha, err, header)
		}
		res = ledgerEditResult{SHA: sha, Row: e.row, Nudge: ledgerNudge{To: "clerk-ledger", Header: header}}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// commitMessage is the subject of spec 8.6 and the body of spec 8.7. The close body quotes the
// words as a > block (owner decision 2026-10-04).
func commitMessage(a ledgerEditArgs, now time.Time) string {
	stamp := now.Format(ledgerTimeLayout)
	if a.Action == "set_key" {
		for k := range a.Cells {
			return fmt.Sprintf("set %s: %s\n\nRecorded (UTC): %s\n", k, a.Subject, stamp)
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s: %s\n\n", a.Action, a.Kind, a.Subject)
	if a.Action != "close" {
		fmt.Fprintf(&b, "Recorded (UTC): %s\n", stamp)
		return b.String()
	}
	b.WriteString("Words:\n")
	for l := range strings.SplitSeq(a.Words, "\n") {
		if l == "" {
			b.WriteString(">\n")
		} else {
			b.WriteString("> " + l + "\n")
		}
	}
	fmt.Fprintf(&b, "Source: %s\nRecorded (UTC): %s\nTag: %s decision %s\n", a.Source, stamp, a.DecisionBy, now.Format(time.DateOnly))
	return b.String()
}
