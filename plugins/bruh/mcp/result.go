package main

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
)

var resultNameRE = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

// resultTools holds result_save: a clerk saves the result of a workflow in the data
// folder (principle 3), and post-findings.sh reads the file from the returned path.
func resultTools() []Tool {
	return []Tool{{
		Name:        "result_save",
		Description: "Save the result object of a workflow run as <data>/results/<role key>/<name>.json (replaces the file) and return its path, for scripts/post-findings.sh.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":   map[string]any{"type": "string", "pattern": "^[a-z0-9-]{1,64}$"},
				"result": map[string]any{"type": "object"},
			},
			"required": []string{"name", "result"},
		},
		Handler: func(c *Call, raw json.RawMessage) (any, error) {
			me, err := c.Env.Caller()
			if err != nil {
				return nil, err
			}
			a, err := decode[struct {
				Name   string          `json:"name"`
				Result json.RawMessage `json:"result"`
			}](raw)
			if err != nil {
				return nil, err
			}
			if _, err := checkID(a.Name, resultNameRE, "result name"); err != nil {
				return nil, err
			}
			var obj map[string]any
			if len(a.Result) == 0 || json.Unmarshal(a.Result, &obj) != nil || obj == nil {
				return nil, errors.New("result must be a JSON object")
			}
			dir, err := c.Env.Dir("results", me)
			if err != nil {
				return nil, err
			}
			data, err := json.MarshalIndent(obj, "", "  ")
			if err != nil {
				return nil, err
			}
			path := filepath.Join(dir, a.Name+".json")
			if err := atomicWrite(path, append(data, '\n')); err != nil {
				return nil, err
			}
			return map[string]string{"path": path}, nil
		},
	}}
}
