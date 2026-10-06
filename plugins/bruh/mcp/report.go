package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

func isRFC3339(s string) bool {
	_, err := time.Parse(time.RFC3339, s)
	return err == nil
}

var reportKinds = []string{"status", "answer", "event", "result"}

// reportPhases are the values of the optional phase of a report line (task 30). The board reads
// the field, so the names are a contract.
var reportPhases = []string{"plan", "implement", "review", "fix", "merge", "done"}

// isMailPendingEvent reports whether a line is the event "<role key> not running; mail pending"
// of the mail procedure (clerk.md, "How to send a message"), with a valid role key.
func isMailPendingEvent(kind, text string) bool {
	key, ok := strings.CutSuffix(text, " not running; mail pending")
	_, err := ParseRoleKey(key)
	return kind == "event" && ok && err == nil
}

type Source struct {
	Call  string `json:"call"`
	Value string `json:"value"`
	At    string `json:"at"`
}

type ReportLine struct {
	At     string          `json:"at"`
	From   string          `json:"from"`
	Kind   string          `json:"kind"`
	Text   string          `json:"text"`
	Source *Source         `json:"source,omitempty"`
	Phase  string          `json:"phase,omitempty"`
	Event  json.RawMessage `json:"event,omitempty"` // watcher lines only
}

// notifyBigm pushes a status or result line of me to bigm as the mail "DONE: report <me>", with
// the line as body, so the wake.sh waiter of bigm wakes it (task 17). It writes no notice while
// one from me is still unread in the mailbox of bigm: at most one pending notice per role. bigm
// then reads the later lines with report_read and since = the at of the body line. since is
// inclusive, because a later line can have the same millisecond at; bigm skips the lines that it
// already has. The lock keeps two parallel calls of one role from writing two notices.
func notifyBigm(env Env, me string, line []byte) error {
	header := "DONE: report " + me
	return env.WithLock("report-notice-"+me, func() error {
		box, err := env.Dir("mail", "bigm")
		if err != nil {
			return err
		}
		files, err := filepath.Glob(filepath.Join(box, "*.json"))
		if err != nil {
			return err
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				continue // mail_read moved it
			}
			var m Message
			if json.Unmarshal(data, &m) == nil && m.From == me && m.Header == header {
				return nil
			}
		}
		_, err = writeMail(env, me, "bigm", header, string(line))
		return err
	})
}

func reportTools() []Tool {
	return []Tool{
		{
			Name:        "report_write",
			Description: "Append one line to the report file of the calling role. A status claim (merged, deployed, live, down, out of quota) carries source: {call, value, at}. Each line of a scout clerk needs the full source, with at in RFC 3339 and value present (it can be empty), except the event '<role key> not running; mail pending'. A status or result line of a role other than bigm also puts the notice 'DONE: report <role key>' (body: the line) in the mailbox of bigm, at most one unread notice per role. The optional phase (plan, implement, review, fix, merge, done) marks the step of the task that starts; it is stored as the top-level field phase of the line.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": reportKinds},
					"text": map[string]any{"type": "string"},
					"source": map[string]any{"type": "object", "properties": map[string]any{
						"call": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "at": map[string]any{"type": "string"},
					}},
					"phase": map[string]any{"type": "string", "enum": reportPhases},
				},
				"required": []string{"kind", "text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Kind   string `json:"kind"`
					Text   string `json:"text"`
					Source *struct {
						Call  string  `json:"call"`
						Value *string `json:"value"` // a pointer: present but empty is valid output
						At    string  `json:"at"`
					} `json:"source"`
					Phase *string `json:"phase"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if !slices.Contains(reportKinds, a.Kind) {
					return nil, fmt.Errorf("invalid kind: %q", a.Kind)
				}
				if a.Phase != nil && !slices.Contains(reportPhases, *a.Phase) {
					return nil, fmt.Errorf("invalid phase: %q", *a.Phase)
				}
				var src *Source
				if s := a.Source; s != nil {
					src = &Source{Call: s.Call, At: s.At}
					if s.Value != nil {
						src.Value = *s.Value
					}
				}
				// A scout reports only facts that it read (principle 1), so each of its lines has a
				// full source read with a UTC time (R-1, spec 3.6.1). The value can be empty: a read
				// that finds nothing often prints nothing. The one exception is the event line of
				// the mail procedure (clerk.md), which the bigm sweep reads to resume a receiver.
				if k, _ := ParseRoleKey(me); isScout(k) && !isMailPendingEvent(a.Kind, a.Text) {
					if s := a.Source; s == nil || s.Call == "" || s.Value == nil || !isRFC3339(s.At) {
						return nil, errors.New("a scout report line needs source {call, value, at}, with at in RFC 3339 from date -u; only the event line '<role key> not running; mail pending' has no source")
					}
				}
				dir, err := c.Env.Dir("reports")
				if err != nil {
					return nil, err
				}
				line := ReportLine{At: c.Env.Stamp(), From: me, Kind: a.Kind, Text: a.Text, Source: src}
				if a.Phase != nil {
					line.Phase = *a.Phase
				}
				data, _ := json.Marshal(line)
				f, err := os.OpenFile(filepath.Join(dir, me+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return nil, err
				}
				_, werr := f.Write(append(data, '\n'))
				if err := errors.Join(werr, f.Close()); err != nil {
					return nil, err
				}
				out := map[string]string{"at": line.At}
				// The line is written, so a notice error does not fail the call: a retry would
				// duplicate the line.
				if (a.Kind == "status" || a.Kind == "result") && me != "bigm" {
					if err := notifyBigm(c.Env, me, data); err != nil {
						out["notice_error"] = err.Error()
					}
				}
				return out, nil
			},
		},
		{
			Name:        "report_read",
			Description: "Read the report lines of a role, optionally only the lines at or after a UTC time (since is inclusive: two lines can have the same millisecond at).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role_key": map[string]any{"type": "string"},
					"since":    map[string]any{"type": "string"},
				},
				"required": []string{"role_key"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
					Since   string `json:"since"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.RoleKey, "role key"); err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("reports")
				if err != nil {
					return nil, err
				}
				f, err := os.Open(filepath.Join(dir, a.RoleKey+".jsonl"))
				if errors.Is(err, fs.ErrNotExist) {
					return []ReportLine{}, nil
				}
				if err != nil {
					return nil, err
				}
				defer f.Close()
				lines := []ReportLine{}
				sc := bufio.NewScanner(f)
				sc.Buffer(make([]byte, 1<<20), 16<<20)
				for sc.Scan() {
					var l ReportLine
					if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
						return nil, err
					}
					if a.Since == "" || l.At >= a.Since {
						lines = append(lines, l)
					}
				}
				return lines, sc.Err()
			},
		},
	}
}
