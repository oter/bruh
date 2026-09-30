package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Question struct {
	ID       string `json:"id"`
	Priority string `json:"priority"`
	Subject  string `json:"subject"`
	Body     string `json:"body"`
	Blocks   string `json:"blocks"`
	Asker    string `json:"asker"`
	OpenedAt string `json:"opened_at"`
}

func questionTools() []Tool {
	return []Tool{
		{
			Name:        "question_open",
			Description: "Open a question and get its ID and header line. Send the header as the SendMessage nudge, then wait with answer_wait.",
			InputSchema: objectSchema(map[string]any{
				"priority": map[string]any{"type": "string", "enum": []string{"P0", "P1", "P2"}},
				"subject":  map[string]any{"type": "string", "description": "One line, at most 200 characters"},
				"body":     stringSchema(),
				"blocks":   map[string]any{"type": "string", "description": "The work that waits for the answer"},
			}, "priority", "subject", "body", "blocks"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ Priority, Subject, Body, Blocks string }](raw)
				if err != nil {
					return nil, err
				}
				if !slices.Contains([]string{"P0", "P1", "P2"}, a.Priority) {
					return nil, fmt.Errorf("invalid priority: %q", a.Priority)
				}
				if strings.TrimSpace(a.Body) == "" || strings.TrimSpace(a.Blocks) == "" {
					return nil, errors.New("body and blocks must not be empty")
				}
				var q Question
				err = c.Env.WithLock("questions", func() error {
					dir, err := c.Env.Dir("questions")
					if err != nil {
						return err
					}
					n := 1
					if data, err := os.ReadFile(filepath.Join(dir, "next")); err == nil {
						if n, err = strconv.Atoi(strings.TrimSpace(string(data))); err != nil || n < 1 {
							return fmt.Errorf("bad questions/next: %q", data)
						}
					} else if !errors.Is(err, fs.ErrNotExist) {
						return err
					}
					q = Question{Priority: a.Priority, Subject: a.Subject, Body: a.Body, Blocks: a.Blocks, Asker: me, OpenedAt: c.Env.Stamp()}
					if !headerRE.MatchString(q.Priority + " Q-1: " + q.Subject) {
						return fmt.Errorf("invalid subject: %q (one line, 1 to 200 characters)", a.Subject)
					}
					// Create the file first and never over an existing one: after a crash between the
					// two writes, the number in next is used already, and the loop skips it.
					for ; ; n++ {
						q.ID = fmt.Sprintf("Q-%d", n)
						data, _ := json.MarshalIndent(q, "", "  ")
						f, err := os.OpenFile(filepath.Join(dir, q.ID+".json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
						if errors.Is(err, fs.ErrExist) {
							continue
						}
						if err != nil {
							return err
						}
						_, werr := f.Write(data)
						if err := errors.Join(werr, f.Close()); err != nil {
							return err
						}
						break
					}
					return atomicWrite(filepath.Join(dir, "next"), []byte(strconv.Itoa(n+1)))
				})
				if err != nil {
					return nil, err
				}
				return map[string]string{"id": q.ID, "header": q.header()}, nil
			},
		},
		{
			Name:        "bruh_info",
			Description: "Return the plugin root, the data folder, the role key of this session, and the plugin version. Use the plugin root to find scripts/.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				root, err := filepath.Abs(c.Env.PluginRoot)
				if err != nil {
					return nil, err
				}
				data := c.Env.DataDir
				if data != "" {
					if data, err = filepath.Abs(data); err != nil {
						return nil, err
					}
				}
				var p struct {
					Version string `json:"version"`
				}
				if b, err := os.ReadFile(filepath.Join(root, ".claude-plugin", "plugin.json")); err == nil {
					_ = json.Unmarshal(b, &p)
				}
				return map[string]string{"plugin_root": root, "data_dir": data, "role_key": c.Env.RoleKey, "version": p.Version}, nil
			},
		},
	}
}

func (q Question) header() string { return q.Priority + " " + q.ID + ": " + q.Subject }
