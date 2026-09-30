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
)

var reportKinds = []string{"status", "answer", "event", "result"}

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
	Event  json.RawMessage `json:"event,omitempty"` // watcher lines only
}

func reportTools() []Tool {
	return []Tool{
		{
			Name:        "report_write",
			Description: "Append one line to the report file of the calling role. A status claim (merged, deployed, live, down, out of quota) carries source: {call, value, at}.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": reportKinds},
					"text": map[string]any{"type": "string"},
					"source": map[string]any{"type": "object", "properties": map[string]any{
						"call": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "at": map[string]any{"type": "string"},
					}},
				},
				"required": []string{"kind", "text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Kind   string  `json:"kind"`
					Text   string  `json:"text"`
					Source *Source `json:"source"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if !slices.Contains(reportKinds, a.Kind) {
					return nil, fmt.Errorf("invalid kind: %q", a.Kind)
				}
				dir, err := c.Env.Dir("reports")
				if err != nil {
					return nil, err
				}
				line := ReportLine{At: c.Env.Stamp(), From: me, Kind: a.Kind, Text: a.Text, Source: a.Source}
				data, _ := json.Marshal(line)
				f, err := os.OpenFile(filepath.Join(dir, me+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return nil, err
				}
				_, werr := f.Write(append(data, '\n'))
				return map[string]string{"at": line.At}, errors.Join(werr, f.Close())
			},
		},
		{
			Name:        "report_read",
			Description: "Read the report lines of a role, optionally only after a UTC time. The role key watcher reads the code host events of the watcher.",
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
				if a.RoleKey != "watcher" {
					if _, err := checkKey(a.RoleKey, "role key"); err != nil {
						return nil, err
					}
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
					if a.Since == "" || l.At > a.Since {
						lines = append(lines, l)
					}
				}
				return lines, sc.Err()
			},
		},
	}
}
