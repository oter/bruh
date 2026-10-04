package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// scoutTaskRE matches the task part of a scout clerk key clerk-<project>-scout<n>. A bare
// "scout" is a scout too, so that no task clerk gets a scout name without the scout stops.
var scoutTaskRE = regexp.MustCompile(`^scout[0-9]*$`)

// isScout reports whether k is a scout clerk: a short-lived, read-only clerk that bigm or the
// clanker of the project starts to gather facts (owner rule R-1, spec 3.6.1).
func isScout(k RoleKey) bool { return k.Role == "clerk" && scoutTaskRE.MatchString(k.Task) }

// bigmMayStart reports whether bigm may act for k without being its parent (role settings,
// launch, and resume): the keys of bigmActsFor, and a scout clerk of any project (R-1).
// session_resume still refuses every scout key.
func bigmMayStart(k RoleKey) bool { return bigmActsFor(k) || isScout(k) }

const bruhTool = "mcp__plugin_bruh_bruh__"

// scoutDeny are the deny rules that role_settings_write adds to each scout settings file, so
// that its starter cannot leave them out (spec principle 2). The tool rules remove the tools;
// the Bash rules match only the command text, so they are speed bumps (spec 3.6.1).
var scoutDeny = []string{
	"Edit", "Write", "NotebookEdit", "Workflow", "EnterWorktree",
	bruhTool + "session_launch", bruhTool + "session_resume", bruhTool + "role_settings_write",
	bruhTool + "lease_define", bruhTool + "lease_request", bruhTool + "lease_grant", bruhTool + "lease_release",
	bruhTool + "answer_write", bruhTool + "question_open", bruhTool + "repos_set", bruhTool + "result_save",
	bruhTool + "init_plan", bruhTool + "init_apply", bruhTool + "learn_refresh", bruhTool + "learn_scan",
	"Bash(git push:*)", "Bash(git commit:*)", "Bash(git add:*)", "Bash(git rm:*)", "Bash(git mv:*)",
	"Bash(git merge:*)", "Bash(git rebase:*)", "Bash(git reset:*)", "Bash(git checkout:*)", "Bash(git switch:*)",
	"Bash(git restore:*)", "Bash(git stash:*)", "Bash(git tag:*)", "Bash(git worktree:*)", "Bash(git clean:*)",
	"Bash(git pull:*)", "Bash(git apply:*)", "Bash(git cherry-pick:*)", "Bash(git revert:*)",
	"Bash(rm:*)", "Bash(mv:*)", "Bash(cp:*)", "Bash(mkdir:*)", "Bash(touch:*)", "Bash(tee:*)", "Bash(chmod:*)", "Bash(ln:*)",
	"Bash(claude:*)", "Bash(go run:*)", "Bash(sh:*)", "Bash(bash:*)",
	"Bash(gh pr merge:*)", "Bash(gh pr create:*)", "Bash(gh pr comment:*)", "Bash(gh pr review:*)",
	"Bash(gh pr close:*)", "Bash(gh pr edit:*)", "Bash(gh pr ready:*)",
	"Bash(gh issue create:*)", "Bash(gh issue comment:*)", "Bash(gh issue close:*)", "Bash(gh issue edit:*)",
	"Bash(gh release create:*)",
	"Bash(gh api * -X *)", "Bash(gh api * --method *)", "Bash(gh api * -f *)", "Bash(gh api * -F *)",
	"Bash(gh api * --field *)", "Bash(gh api * --raw-field *)", "Bash(gh api * --input *)",
	"Bash(glab mr merge:*)", "Bash(glab mr create:*)", "Bash(glab mr note:*)", "Bash(glab mr close:*)",
	"Bash(glab issue create:*)", "Bash(glab issue note:*)", "Bash(glab api * -X *)", "Bash(glab api * --method *)",
	"Bash(tea pr merge:*)", "Bash(tea pr create:*)", "Bash(tea comment:*)",
}

// nextScoutKey returns the scout key of project whose n is one more than the highest n of the
// scout settings files in dir (a bare "scout" counts as 0).
func nextScoutKey(dir, project string) string {
	prefix := "clerk-" + project + "-scout"
	high := 0
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), prefix)
		if !ok {
			continue
		}
		digits, ok := strings.CutSuffix(rest, ".json")
		if n, err := strconv.Atoi(digits); ok && err == nil && n > high {
			high = n
		}
	}
	return prefix + strconv.Itoa(high+1)
}

func rolesTools() []Tool {
	return []Tool{
		{
			Name:        "role_settings_write",
			Description: "Write the --settings file of a role: BRUH_ROLE_KEY, extra env values (tool accounts), deny rules, and allow rules. Only bigm passes allow, and only for a clanker key or a scout key: each rule is Read(//<path>/**) for a repository of the project in learn/projects/<project>.json. A scout key clerk-<project>-scout<n> always gets the scout deny rules and is written once. Returns the absolute path.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role_key": map[string]any{"type": "string"},
					"env":      map[string]any{"type": "object", "additionalProperties": map[string]any{"type": "string"}},
					"deny":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"allow":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
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
					Allow   []string          `json:"allow"`
				}](raw)
				if err != nil {
					return nil, err
				}
				target, err := ParseRoleKey(a.RoleKey)
				if err != nil {
					return nil, err
				}
				key := target.String()
				if target.Parent() != me && (me != "bigm" || !bigmMayStart(target)) {
					return nil, fmt.Errorf("%s cannot write the role settings of %s; only its parent %q can (bigm too for its own key, a merger clerk, and a scout clerk)", me, key, target.Parent())
				}
				if _, ok := a.Env["BRUH_ROLE_KEY"]; ok {
					return nil, errors.New("env must not set BRUH_ROLE_KEY; role_key sets it")
				}
				// A child under another config folder runs under another supervisor, so session_list
				// and session_resume never find it (spec 4.1: bruh never sets it).
				if _, ok := a.Env["CLAUDE_CONFIG_DIR"]; ok {
					return nil, errors.New("env must not set CLAUDE_CONFIG_DIR; bruh never sets it (spec 4.1)")
				}
				if len(a.Allow) > 0 {
					if me != "bigm" || (target.Role != "clanker" && !isScout(target)) {
						return nil, errors.New("allow is only for a clanker key or a scout key, written by bigm")
					}
					if err := checkAllowRules(c.Env, target.Project, a.Allow); err != nil {
						return nil, err
					}
				}
				deny := a.Deny
				if isScout(target) {
					deny = append(slices.Clone(a.Deny), scoutDeny...)
				}
				out, err := roleSettings(c.Env.PluginRoot, key, a.Env, deny, a.Allow)
				if err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("roles")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, key+".json")
				if isScout(target) {
					// One question, one scout: a used key keeps its old mailbox and report lines, and
					// the exclusive create stops bigm and the clanker from sharing one key.
					if err := writeNew(file, out); errors.Is(err, os.ErrExist) {
						return nil, fmt.Errorf("the role settings of %s exist: a scout key is used once; use %s", key, nextScoutKey(dir, target.Project))
					} else if err != nil {
						return nil, err
					}
				} else if err := atomicWrite(file, out); err != nil {
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
// deny rules, the allow rules when extraAllow is not empty, and BRUH_ROLE_KEY.
func roleSettings(pluginRoot, key string, extraEnv map[string]string, extraDeny, extraAllow []string) ([]byte, error) {
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
	if len(extraAllow) > 0 {
		perms["allow"] = extraAllow
	}
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

// checkAllowRules checks the allow rules of role_settings_write for the clanker of project: each
// rule is exactly "Read(/" + <root of learn/tree.json joined with the path of a repository of
// learn/projects/<project>.json> + "/**)". It reads the ledger path with ledgerPath.
func checkAllowRules(env Env, project string, rules []string) error {
	ledger, err := ledgerPath(env)
	if err != nil {
		return err
	}
	var tree treeFile
	if err := readLearn(filepath.Join(ledger, "learn", "tree.json"), &tree); err != nil {
		return err
	}
	var proj projectFile
	if err := readLearn(filepath.Join(ledger, "learn", "projects", project+".json"), &proj); err != nil {
		return err
	}
	allowed := make(map[string]bool, len(proj.Repos))
	for _, r := range proj.Repos {
		allowed["Read(/"+filepath.Join(tree.Root, r.Path)+"/**)"] = true
	}
	for _, rule := range rules {
		if !allowed[rule] {
			return fmt.Errorf("allow rule %q is not Read(//<path>/**) for a repository of project %s in learn/projects/%s.json", rule, project, project)
		}
	}
	return nil
}

// readLearn decodes the JSON file at path into v.
func readLearn(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// writeNew writes data to a new file, and fails with os.ErrExist when file exists.
func writeNew(file string, data []byte) error {
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Close())
}
