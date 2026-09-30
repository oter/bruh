package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const handoffMax = 8000

var (
	itemRE     = regexp.MustCompile(`^\s*(?:[-*]|\d+\.)\s+(.*)$`)
	pointerRE  = regexp.MustCompile("^`?[\\w./~-]+`?$")
	subagentRE = regexp.MustCompile(`\ba[0-9a-f]{16}\b`)
	tmpRE      = regexp.MustCompile("(^|[\\s`'\"(])/(?:private/)?tmp/")
)

func validateHandoff(text string) error {
	if text == "" {
		return errors.New("handoff is empty")
	}
	if n := utf8.RuneCountInString(text); n > handoffMax {
		return fmt.Errorf("handoff is %d characters, over 8000 characters", n)
	}
	if subagentRE.MatchString(text) {
		return errors.New("handoff names a subagent ID; a new session cannot reach it")
	}
	if tmpRE.MatchString(text) {
		return errors.New("handoff names a /tmp path; a reboot deletes it")
	}
	items, pointers := 0, 0
	for line := range strings.SplitSeq(text, "\n") {
		m := itemRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		items++
		if pointerRE.MatchString(strings.TrimSpace(m[1])) {
			pointers++
		}
	}
	if items == 0 || items == pointers {
		return errors.New("handoff has only file pointers; write the items themselves")
	}
	return nil
}

func handoffTools() []Tool {
	return []Tool{
		{
			Name:        "handoff_write",
			Description: "Replace the handoff of the calling role with one current state. Sections: role, standing owner rules word for word, goal, state, decisions, waiting on the owner, waiting on others, next steps, files to read.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
				"required":   []string{"text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ Text string }](raw)
				if err != nil {
					return nil, err
				}
				if err := validateHandoff(a.Text); err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("handoffs")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, me+".md")
				if old, err := os.ReadFile(file); err == nil {
					h, err := os.OpenFile(filepath.Join(dir, me+".history.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
					if err != nil {
						return nil, err
					}
					_, werr := fmt.Fprintf(h, "%s\n\n---\n\n", old)
					if err := errors.Join(werr, h.Close()); err != nil {
						return nil, err
					}
				} else if !errors.Is(err, fs.ErrNotExist) {
					return nil, err
				}
				at := c.Env.Stamp()
				stamped := "Updated: " + at + "\n\n" + a.Text
				if err := atomicWrite(file, []byte(stamped)); err != nil {
					return nil, err
				}
				return map[string]any{"at": at, "chars": utf8.RuneCountInString(stamped)}, nil
			},
		},
		{
			Name:        "handoff_read",
			Description: "Read the current handoff of a role. Empty when none exists.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"role_key": map[string]any{"type": "string"}}},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
				}](raw)
				if err != nil {
					return nil, err
				}
				key := me
				if a.RoleKey != "" {
					if key, err = checkKey(a.RoleKey, "role key"); err != nil {
						return nil, err
					}
				}
				dir, err := c.Env.Dir("handoffs")
				if err != nil {
					return nil, err
				}
				data, err := os.ReadFile(filepath.Join(dir, key+".md"))
				if errors.Is(err, fs.ErrNotExist) {
					return "", nil
				}
				return string(data), err
			},
		},
	}
}
