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
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"
)

var shortIDRE = regexp.MustCompile(`backgrounded · ([0-9a-f]+)`)

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
		if out, err = claudeCmd(ctx, env, dir, []string{"CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1"}, args...).CombinedOutput(); err != nil {
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

// orcaMinVersion is the oldest Orca app that the viewer tab is tested with (spec 4.3, O2).
const orcaMinVersion = "1.4.218"

// Tests replace these.
var (
	goos         = runtime.GOOS
	orcaLookPath = exec.LookPath
	orcaTimeout  = 10 * time.Second
)

// orcaBin returns the Orca CLI to use, or "" when bruh must run no orca command (spec 4.3): the
// plugin option orca_local is "off", the CLI is not on PATH, or the name is orca-dev. On Linux a
// bare orca is the GNOME screen reader, so bruh never runs it there.
func orcaBin(env Env) string {
	var s struct {
		PluginConfigs map[string]struct {
			Options struct {
				OrcaLocal string `json:"orca_local"`
			} `json:"options"`
		} `json:"pluginConfigs"`
	}
	// Read at each call, so a change in /config applies at once. No value means auto.
	if data, err := os.ReadFile(env.SettingsFile); err == nil && json.Unmarshal(data, &s) == nil && s.PluginConfigs[pluginID].Options.OrcaLocal == "off" {
		return ""
	}
	name := os.Getenv("ORCA_CLI_COMMAND")
	if name == "" {
		name = "orca"
		if goos == "linux" {
			name = "orca-ide"
		}
	}
	if base := filepath.Base(name); base == "orca-dev" || goos == "linux" && base == "orca" {
		return ""
	}
	p, err := orcaLookPath(name)
	if err != nil {
		return ""
	}
	return p
}

// orcaRun runs one orca command with --json and returns its result field.
func orcaRun(bin string, args ...string) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), orcaTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append(args, "--json")...)
	cmd.WaitDelay = time.Second
	out, err := cmd.Output()
	var r struct {
		OK     bool            `json:"ok"`
		Result json.RawMessage `json:"result"`
	}
	if err == nil {
		err = json.Unmarshal(out, &r)
	}
	if err == nil && !r.OK {
		err = errors.New("ok is not true")
	}
	if err != nil {
		return nil, fmt.Errorf("orca %s: %w: %.300s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return r.Result, nil
}

// parseVersion parses "x.y.z" into three numbers, or returns nil.
func parseVersion(v string) []int {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return nil
	}
	out := make([]int, 3)
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out[i] = n
	}
	return out
}

// orcaReady checks that the Orca app runs, that its runtime is ready, and its version.
func orcaReady(bin string) error {
	res, err := orcaRun(bin, "status")
	if err != nil {
		return err
	}
	var s struct {
		App struct {
			Running bool `json:"running"`
		} `json:"app"`
		Runtime struct {
			Reachable  bool   `json:"reachable"`
			State      string `json:"state"`
			AppVersion string `json:"appVersion"`
		} `json:"runtime"`
	}
	if err := json.Unmarshal(res, &s); err != nil {
		return fmt.Errorf("orca status: %w", err)
	}
	switch v := parseVersion(s.Runtime.AppVersion); {
	case !s.App.Running:
		return errors.New("orca status: result.app.running is not true")
	case !s.Runtime.Reachable:
		return errors.New("orca status: result.runtime.reachable is not true")
	case s.Runtime.State != "ready":
		return fmt.Errorf("orca status: result.runtime.state is %q, not ready", s.Runtime.State)
	case v == nil || slices.Compare(v, parseVersion(orcaMinVersion)) < 0:
		return fmt.Errorf("orca status: result.runtime.appVersion %q is not %s or later", s.Runtime.AppVersion, orcaMinVersion)
	}
	return nil
}

// orcaTabs returns the handles of the Orca terminals whose title is exactly title.
func orcaTabs(bin, title string) ([]string, error) {
	res, err := orcaRun(bin, "terminal", "list")
	if err != nil {
		return nil, err
	}
	var l struct {
		Terminals []struct {
			Handle string `json:"handle"`
			Title  string `json:"title"`
		} `json:"terminals"`
	}
	if err := json.Unmarshal(res, &l); err != nil {
		return nil, fmt.Errorf("orca terminal list: %w", err)
	}
	var out []string
	for _, t := range l.Terminals {
		if t.Title == title {
			out = append(out, t.Handle)
		}
	}
	return out, nil
}

// orcaClose closes the whole tab of each handle.
func orcaClose(bin string, handles []string) error {
	for _, h := range handles {
		if _, err := orcaRun(bin, "terminal", "close", "--terminal", h, "--tab"); err != nil {
			return err
		}
	}
	return nil
}

// orcaView shows the background session id in an Orca tab titled key, which runs claude attach
// (spec 4.3). replace closes an old tab of key first; else an existing tab is kept. It sets
// res["orca"] to the handle, or res["orca_error"]: an Orca failure never fails the session call.
func orcaView(env Env, res map[string]string, key, cwd, id string, replace bool) {
	bin := orcaBin(env)
	if bin == "" {
		return
	}
	handle, err := func() (string, error) {
		if err := orcaReady(bin); err != nil {
			return "", err
		}
		handles, err := orcaTabs(bin, key)
		if err != nil {
			return "", err
		}
		if !replace && len(handles) > 0 {
			return handles[0], nil
		}
		if err := orcaClose(bin, handles); err != nil {
			return "", err
		}
		// The tab goes to the start folder, not to the worktree that a clerk moved into.
		folder, _, _ := strings.Cut(cwd, string(filepath.Separator)+filepath.Join(".claude", "worktrees")+string(filepath.Separator))
		// id is hex (shortIDRE), so the command text needs no quoting.
		out, err := orcaRun(bin, "terminal", "create", "--worktree", "path:"+folder, "--title", key, "--command", "claude attach "+id)
		if err != nil {
			return "", err
		}
		var c struct {
			Handle   string `json:"handle"`
			Terminal struct {
				Handle string `json:"handle"`
			} `json:"terminal"`
		}
		_ = json.Unmarshal(out, &c)
		return cmp.Or(c.Terminal.Handle, c.Handle), nil
	}()
	if err != nil {
		res["orca_error"] = err.Error()
		return
	}
	res["orca"] = handle
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
				res := sessionResult(key, e)
				orcaView(c.Env, res, key, a.Cwd, str(e, "id"), true)
				return res, nil
			},
		},
		{
			Name:        "session_resume",
			Description: "Wake a stopped background session of a role with a prompt. Only the parent of role_key, or bigm. A live session gets a SendMessage nudge instead.",
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
				res := sessionResult(key, e)
				orcaView(c.Env, res, key, str(last, "cwd"), str(e, "id"), false)
				return res, nil
			},
		},
		{
			Name:        "session_tab_close",
			Description: "Close the Orca tab of a retired role (its title is role_key). The session keeps running. Only the parent of role_key, or bigm. Call it after you accept a clerk's result or give up its task, after you stop a merger clerk, and after you stop a clanker for good. Without Orca it does nothing.",
			InputSchema: objectSchema(map[string]any{"role_key": stringSchema()}, "role_key"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
				}](raw)
				if err != nil {
					return nil, err
				}
				k, _, err := childKey(c.Env, a.RoleKey)
				if err != nil {
					return nil, err
				}
				key := k.String()
				res := map[string]any{"role_key": key, "closed": 0}
				bin := orcaBin(c.Env)
				if bin == "" {
					return res, nil
				}
				handles, err := orcaTabs(bin, key)
				if err == nil {
					err = orcaClose(bin, handles)
				}
				if err != nil {
					res["orca_error"] = err.Error()
					return res, nil
				}
				res["closed"] = len(handles)
				return res, nil
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
