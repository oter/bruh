package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var shortIDRE = regexp.MustCompile(`backgrounded · ([0-9a-f]+)`)

// ansiRE matches the CSI escape sequences (SGR colors and others) in the output of claude --bg.
var ansiRE = regexp.MustCompile("\x1b\\[[0-?]*[ -/]*[@-~]")

// claudeCmd builds a claude command. BRUH_ROLE_KEY of this session never leaks into the child.
func claudeCmd(ctx context.Context, env Env, dir string, extra []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, env.ClaudeBin, args...)
	cmd.Dir = dir
	cmd.Env = append(slices.DeleteFunc(os.Environ(), func(s string) bool { return strings.HasPrefix(s, "BRUH_ROLE_KEY=") }), extra...)
	return cmd
}

// listAgents returns the entries of `claude agents --json --all`.
func listAgents(env Env) ([]map[string]any, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	out, err := claudeCmd(ctx, env, "", nil, "agents", "--json", "--all").Output()
	if err != nil {
		return nil, fmt.Errorf("claude agents --json --all: %w", err)
	}
	var entries []map[string]any
	if err := json.Unmarshal(out, &entries); err != nil {
		return nil, fmt.Errorf("claude agents --json --all: %w", err)
	}
	return entries, nil
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func isLive(e map[string]any) bool { return e["pid"] != nil }

// launchBackground runs check and then claude with --bg under the lock of the role key, so two
// parallel calls cannot start two sessions of one role. It returns the agents entry of the new session.
func launchBackground(env Env, key, dir string, args []string, wantID string, check func() error) (map[string]any, error) {
	var out []byte
	err := env.WithLock("session-"+key, func() error {
		if err := check(); err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		var err error
		out, err = claudeCmd(ctx, env, dir, []string{"CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1"}, args...).CombinedOutput()
		out = ansiRE.ReplaceAll(out, nil)
		if err != nil {
			return fmt.Errorf("claude %s: %w: %s", args[0], err, out)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	m := shortIDRE.FindSubmatch(out)
	if m == nil {
		return nil, fmt.Errorf("claude printed no session ID: %s", out)
	}
	id := string(m[1])
	if wantID != "" && id != wantID {
		return nil, fmt.Errorf("claude started a copy %s instead of resuming %s; stop the copy with claude stop %s: %s", id, wantID, id, out)
	}
	for range 20 {
		entries, err := listAgents(env)
		if err != nil {
			return nil, err
		}
		if i := slices.IndexFunc(entries, func(e map[string]any) bool { return str(e, "id") == id }); i >= 0 {
			return entries[i], nil
		}
		time.Sleep(env.PollInterval)
	}
	return nil, fmt.Errorf("session %s is not in claude agents --json --all", id)
}

func sessionResult(key string, e map[string]any) map[string]string {
	return map[string]string{"role_key": key, "session_id": str(e, "sessionId"), "name": str(e, "name"), "state": str(e, "state")}
}

// childKey checks that the caller is the parent of key or bigm, and returns the plugin agent of
// the key. bigm starts the merger clerk of a remote project on its own machine.
func childKey(env Env, key string) (RoleKey, string, error) {
	me, err := env.Caller()
	if err != nil {
		return RoleKey{}, "", err
	}
	k, err := ParseRoleKey(key)
	if err != nil {
		return RoleKey{}, "", err
	}
	if k.Parent() != me && (me != "bigm" || !bigmActsFor(k)) {
		return RoleKey{}, "", fmt.Errorf("%s cannot start %s; only its parent %q can (bigm too for a merger clerk)", me, key, k.Parent())
	}
	agent := map[string]string{"clanker": "clanker", "clerk": "clerk", "ledger": "clerk"}[k.Role]
	return k, agent, nil
}

// pluginDirArgs returns --plugin-dir for a plugin root outside ~/.claude/plugins/.
func pluginDirArgs(env Env) ([]string, error) {
	root, err := filepath.Abs(env.PluginRoot)
	if err != nil {
		return nil, err
	}
	installed := filepath.Join(env.Home, ".claude", "plugins") + string(filepath.Separator)
	if env.Home != "" && strings.HasPrefix(root+string(filepath.Separator), installed) {
		return nil, nil
	}
	return []string{"--plugin-dir", root}, nil
}

func checkPrompt(p string) error {
	if strings.HasPrefix(p, "-") {
		return errors.New("prompt must not start with -")
	}
	return nil
}

func sessionTools() []Tool {
	return []Tool{
		{
			Name:        "session_launch",
			Description: "Start a clanker or clerk as a background session. Only the parent of role_key, or bigm. Write its role settings and its start message (mail_post) first.",
			InputSchema: objectSchema(map[string]any{
				"agent":    map[string]any{"type": "string", "enum": []string{"clanker", "clerk"}},
				"role_key": stringSchema(),
				"cwd":      map[string]any{"type": "string", "description": "Absolute project folder; a clerk starts in the main checkout"},
				"prompt":   map[string]any{"type": "string", "description": "Default: Read your start message with mail_read."},
			}, "agent", "role_key", "cwd"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					Agent   string `json:"agent"`
					RoleKey string `json:"role_key"`
					Cwd     string `json:"cwd"`
					Prompt  string `json:"prompt"`
				}](raw)
				if err != nil {
					return nil, err
				}
				k, agent, err := childKey(c.Env, a.RoleKey)
				if err != nil {
					return nil, err
				}
				key := k.String()
				if a.Agent != agent {
					return nil, fmt.Errorf("agent %q does not fit role key %s; use %q", a.Agent, key, agent)
				}
				if fi, err := os.Stat(a.Cwd); !filepath.IsAbs(a.Cwd) || err != nil || !fi.IsDir() {
					return nil, fmt.Errorf("cwd must be an absolute folder: %q", a.Cwd)
				}
				prompt := cmp.Or(a.Prompt, "Read your start message with mail_read.")
				if err := checkPrompt(prompt); err != nil {
					return nil, err
				}
				if c.Env.DataDir == "" {
					return nil, errors.New("BRUH_DATA is not set")
				}
				settings, err := filepath.Abs(filepath.Join(c.Env.DataDir, "roles", key+".json"))
				if err != nil {
					return nil, err
				}
				if _, err := os.Stat(settings); err != nil {
					return nil, fmt.Errorf("no role settings for %s; write them with role_settings_write first", key)
				}
				pd, err := pluginDirArgs(c.Env)
				if err != nil {
					return nil, err
				}
				args := []string{"--bg", "--agent", "bruh:" + agent, "--name", key, "--permission-mode", "auto", "--settings", settings}
				args = append(append(args, pd...), prompt)
				e, err := launchBackground(c.Env, key, a.Cwd, args, "", func() error {
					entries, err := listAgents(c.Env)
					if err != nil {
						return err
					}
					if slices.ContainsFunc(entries, func(e map[string]any) bool { return str(e, "name") == key && isLive(e) }) {
						return fmt.Errorf("a live session is already named %s", key)
					}
					return nil
				})
				if err != nil {
					return nil, err
				}
				return sessionResult(key, e), nil
			},
		},
		{
			Name:        "session_resume",
			Description: "Wake a stopped background session of a role with a prompt. Only the parent of role_key, or bigm. A live session gets a SendMessage nudge instead. A scout clerk is never resumed.",
			InputSchema: objectSchema(map[string]any{
				"role_key": stringSchema(),
				"prompt":   map[string]any{"type": "string", "description": "Default: Read your mailbox with mail_read."},
			}, "role_key"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
					Prompt  string `json:"prompt"`
				}](raw)
				if err != nil {
					return nil, err
				}
				k, _, err := childKey(c.Env, a.RoleKey)
				if err != nil {
					return nil, err
				}
				key := k.String()
				// One question, one scout: a stopped scout gets a new scout, never a resume (R-1).
				if isScout(k) {
					return nil, fmt.Errorf("%s is a scout clerk: a scout is never resumed; start a new scout", key)
				}
				prompt := cmp.Or(a.Prompt, "Read your mailbox with mail_read.")
				if err := checkPrompt(prompt); err != nil {
					return nil, err
				}
				entries, err := listAgents(c.Env)
				if err != nil {
					return nil, err
				}
				var last map[string]any
				lastAt := -1.0
				for _, e := range entries {
					if t, _ := e["startedAt"].(float64); str(e, "name") == key && str(e, "kind") == "background" && t > lastAt {
						last, lastAt = e, t
					}
				}
				if last == nil {
					return nil, fmt.Errorf("no background session is named %s; start it with session_launch", key)
				}
				if isLive(last) {
					return nil, fmt.Errorf("session %s of %s is live; send it a nudge instead", str(last, "id"), key)
				}
				// No other flags: a background session keeps its saved options, and extra flags start a copy.
				e, err := launchBackground(c.Env, key, str(last, "cwd"), []string{"--resume", str(last, "sessionId"), "--bg", prompt}, str(last, "id"), func() error { return nil })
				if err != nil {
					return nil, err
				}
				return sessionResult(key, e), nil
			},
		},
		{
			Name:        "session_list",
			Description: "List the sessions of claude agents --json --all whose name is a role key, with role_key added.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				entries, err := listAgents(c.Env)
				if err != nil {
					return nil, err
				}
				out := []map[string]any{}
				for _, e := range entries {
					if _, err := ParseRoleKey(str(e, "name")); err == nil {
						e["role_key"] = str(e, "name")
						out = append(out, e)
					}
				}
				return out, nil
			},
		},
	}
}
