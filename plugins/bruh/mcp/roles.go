package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
)

func rolesTools() []Tool {
	return []Tool{
		{
			Name:        "role_settings_write",
			Description: "Write the --settings file of a role: BRUH_ROLE_KEY, extra env values (tool accounts), and deny rules. Returns the absolute path.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role_key": map[string]any{"type": "string"},
					"env":      map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"deny":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"role_key"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					RoleKey string            `json:"role_key"`
					Env     map[string]string `json:"env"`
					Deny    []string          `json:"deny"`
				}](raw)
				if err != nil {
					return nil, err
				}
				key, err := checkKey(a.RoleKey, "role key")
				if err != nil {
					return nil, err
				}
				if _, ok := a.Env["BRUH_ROLE_KEY"]; ok {
					return nil, errors.New("env must not set BRUH_ROLE_KEY; role_key sets it")
				}
				data, err := os.ReadFile(filepath.Join(c.Env.PluginRoot, "defaults", "role-settings.json"))
				if err != nil {
					return nil, err
				}
				var settings map[string]any
				if err := json.Unmarshal(data, &settings); err != nil {
					return nil, err
				}
				env, _ := settings["env"].(map[string]any)
				if env == nil {
					env = map[string]any{}
				}
				for k, v := range a.Env {
					env[k] = v
				}
				env["BRUH_ROLE_KEY"] = key
				settings["env"] = env
				perms, _ := settings["permissions"].(map[string]any)
				if perms == nil {
					perms = map[string]any{}
				}
				var deny []string
				if old, ok := perms["deny"].([]any); ok {
					for _, d := range old {
						if s, ok := d.(string); ok {
							deny = append(deny, s)
						}
					}
				}
				for _, d := range a.Deny {
					if !slices.Contains(deny, d) {
						deny = append(deny, d)
					}
				}
				perms["deny"] = deny
				settings["permissions"] = perms
				dir, err := c.Env.Dir("roles")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, key+".json")
				out, _ := json.MarshalIndent(settings, "", "  ")
				if err := atomicWrite(file, out); err != nil {
					return nil, err
				}
				abs, err := filepath.Abs(file)
				return map[string]string{"path": abs}, err
			},
		},
	}
}
