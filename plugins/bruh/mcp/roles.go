package main

import (
	"encoding/json"
	"errors"
	"fmt"
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
				me, err := c.Env.Caller()
				if err != nil {
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
				target, err := ParseRoleKey(a.RoleKey)
				if err != nil {
					return nil, err
				}
				key := target.String()
				// bigm may write every key: its own file, and the merger clerk of a remote project
				// that runs on the machine of bigm.
				if target.Parent() != me && me != "bigm" {
					return nil, fmt.Errorf("%s cannot write the role settings of %s; only its parent %q or bigm can", me, key, target.Parent())
				}
				if _, ok := a.Env["BRUH_ROLE_KEY"]; ok {
					return nil, errors.New("env must not set BRUH_ROLE_KEY; role_key sets it")
				}
				// A child under another config folder runs under another supervisor, so session_list
				// and session_resume never find it (spec 4.1: bruh never sets it).
				if _, ok := a.Env["CLAUDE_CONFIG_DIR"]; ok {
					return nil, errors.New("env must not set CLAUDE_CONFIG_DIR; bruh never sets it (spec 4.1)")
				}
				out, err := roleSettings(c.Env.PluginRoot, key, a.Env, a.Deny)
				if err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("roles")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, key+".json")
				if err := atomicWrite(file, out); err != nil {
					return nil, err
				}
				abs, err := filepath.Abs(file)
				return map[string]string{"path": abs}, err
			},
		},
	}
}

const telegramPlugin = "telegram@claude-plugins-official"

// roleSettings builds a role settings file: the plugin defaults, the extra env values and
// deny rules, and BRUH_ROLE_KEY.
func roleSettings(pluginRoot, key string, extraEnv map[string]string, extraDeny []string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(pluginRoot, "defaults", "role-settings.json"))
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
	for k, v := range extraEnv {
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
	for _, d := range extraDeny {
		if !slices.Contains(deny, d) {
			deny = append(deny, d)
		}
	}
	perms["deny"] = deny
	settings["permissions"] = perms
	// The Telegram server of the official plugin polls the bot at start and ends any other
	// poller of the token, also in a session without --channels. Only bigm may hold the bot.
	if key != "bigm" {
		plugins, _ := settings["enabledPlugins"].(map[string]any)
		if plugins == nil {
			plugins = map[string]any{}
		}
		plugins[telegramPlugin] = false
		settings["enabledPlugins"] = plugins
	}
	out, err := json.MarshalIndent(settings, "", "  ")
	return append(out, '\n'), err
}
