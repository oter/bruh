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

// isScout reports whether k is a scout clerk: a short-lived, read-only clerk that the clanker
// of the project starts to gather facts (owner rule R-1, spec 3.6.1).
func isScout(k RoleKey) bool { return k.Role == "clerk" && scoutTaskRE.MatchString(k.Task) }

const bruhTool = "mcp__plugin_bruh_bruh__"

// scoutGitWrites are the git subcommands that write a repository (its objects, refs, index,
// config, or work tree), a remote, or new files. fetch and remote update write the
// remote-tracking refs of the checkout, so a scout uses git ls-remote instead. branch, remote,
// config, and symbolic-ref also have read forms; a scout uses git rev-parse, git for-each-ref,
// git ls-remote --get-url, and Read of .git/config instead (spec 3.6.1).
var scoutGitWrites = []string{
	"push", "commit", "add", "rm", "mv", "merge", "rebase", "reset", "checkout", "switch",
	"restore", "stash", "tag", "worktree", "clean", "pull", "fetch", "apply", "cherry-pick", "revert",
	"branch", "remote", "config", "update-ref", "symbolic-ref", "update-index", "read-tree",
	"submodule", "sparse-checkout", "bisect", "notes", "replace", "am", "init",
	"gc", "prune", "repack", "pack-refs", "maintenance", "filter-branch", "format-patch",
	"reflog expire", "reflog delete",
}

// scoutGitDeny returns three deny rules for each subcommand of scoutGitWrites: git <sub>, and
// git -C <path> <sub> with and without arguments, the form that the scout procedure prescribes
// (clerk.md, spec 3.6.1). A trailing " *" matches the bare command only when it is the only
// wildcard of the rule, so the -C form needs both rules.
// ponytail: a glob, so "git -C <path> log --grep push x" is denied too; a PreToolUse hook that
// parses the git options is the upgrade if a scout needs such a read.
func scoutGitDeny() []string {
	var out []string
	for _, sub := range scoutGitWrites {
		out = append(out, "Bash(git "+sub+":*)", "Bash(git -C * "+sub+")", "Bash(git -C * "+sub+" *)")
	}
	return out
}

// The two prefixes of the scout allow rules: a clone or a download into /tmp (task 28, owner
// 2026-10-06T16:31:45Z).
const (
	scoutClonePrefix = "git clone https://"
	scoutCurlPrefix  = "curl -fsSL -o /tmp/"
)

// scoutAllow returns the allow rules of the scout key: exactly one clone form and one download
// form, into /tmp/<key>-* only. role_settings_write adds them; the caller cannot pass allow for
// a scout.
// ponytail: glob rules match the command text, so they are speed bumps (spec 3.6.1). A PreToolUse
// hook that parses the destination, or sandbox filesystem.allowWrite (declined by the owner on
// 2026-10-04, spec 3.6.1 "Not built"), is the upgrade if a real stop is needed.
func scoutAllow(key string) []string {
	return []string{
		"Bash(" + scoutClonePrefix + "* /tmp/" + key + "-*)",
		"Bash(" + scoutCurlPrefix + key + "-* https://*)",
	}
}

// scoutGuardDeny returns the deny rules that keep the * of the scout allow rules from covering
// more than a URL and a /tmp name. Deny wins over allow, so each command with an allow prefix
// that holds one of these texts is denied: ".." climbs out of /tmp, "$" and "`" expand a
// variable (a token) or run a command, quotes and "\" hide an option, " -" is any option after
// the prefix (git clone -u or -c run a program, curl -T or -d upload a file), "file:/" is a
// local file URL, and ">" redirects the output into another file (a project folder too). A
// compound command needs no guard: Claude Code splits it at &&, ||, ;, |, &, and newlines, and
// each part must match an allow rule on its own. They match only commands that start with an
// allow prefix, so the other curl reads still go to the classifier.
func scoutGuardDeny() []string {
	var out []string
	for _, p := range []string{scoutClonePrefix, scoutCurlPrefix} {
		// "file:/", not "file:": a rule that ends in ":*" means a trailing " *".
		for _, x := range []string{"..", "$", "`", "'", `"`, `\`, " -", "file:/", ">"} {
			out = append(out, "Bash("+p+"*"+x+"*)")
		}
	}
	return out
}

// scoutDeny are the deny rules that role_settings_write adds to each scout settings file, so
// that its starter cannot leave them out (spec principle 2). The tool rules remove the tools;
// the Bash rules match only the command text, so they are speed bumps (spec 3.6.1).
var scoutDeny = append(append(scoutGitDeny(), scoutGuardDeny()...), []string{
	// A clone stays denied in the -C form, with an option before the URL, and into a home folder,
	// where the project folders and the credentials are; so does a download into a home folder.
	"Bash(git -C * clone)", "Bash(git -C * clone *)", "Bash(git clone -*)",
	"Bash(git clone * ~*)", "Bash(git clone * /Users/*)", "Bash(git clone * /home/*)",
	"Bash(curl * -o ~*)", "Bash(curl * -o /Users/*)", "Bash(curl * -o /home/*)",
	"Edit", "Write", "NotebookEdit", "Workflow", "EnterWorktree",
	bruhTool + "session_launch", bruhTool + "session_resume", bruhTool + "role_settings_write",
	bruhTool + "lease_define", bruhTool + "lease_request", bruhTool + "lease_grant", bruhTool + "lease_release",
	bruhTool + "answer_write", bruhTool + "question_open", bruhTool + "repos_set", bruhTool + "result_save",
	bruhTool + "init_plan", bruhTool + "init_apply", bruhTool + "learn_refresh", bruhTool + "learn_scan",
	bruhTool + "monitor_start", bruhTool + "monitor_stop", bruhTool + "monitor_report",
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
}...)

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
			Description: "Write the --settings file of a role: BRUH_ROLE_KEY, extra env values (tool accounts), deny rules, and allow rules. Only bigm passes allow, and only for a clanker key: each rule is Read(//<path>/**) for a repository of the project in learn/projects/<project>.json. A scout key clerk-<project>-scout<n> always gets the scout deny rules and two allow rules, a clone (git clone https://<url> /tmp/<key>-<name>) and a download (curl -fsSL -o /tmp/<key>-<name> https://<url>), and is written once. Returns the absolute path.",
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
				if target.Parent() != me && (me != "bigm" || !bigmActsFor(target)) {
					return nil, fmt.Errorf("%s cannot write the role settings of %s; only its parent %q can (bigm too for its own key)", me, key, target.Parent())
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
					if me != "bigm" || target.Role != "clanker" {
						return nil, errors.New("allow is only for a clanker key, written by bigm")
					}
					if err := checkAllowRules(c.Env, target.Project, a.Allow); err != nil {
						return nil, err
					}
				}
				deny, allow := a.Deny, a.Allow
				if isScout(target) {
					deny, allow = append(slices.Clone(a.Deny), scoutDeny...), scoutAllow(key)
				}
				out, err := roleSettings(c.Env, key, a.Env, deny, allow)
				if err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("roles")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, key+".json")
				if isScout(target) {
					// One question, one scout: a used key keeps its old mailbox and report lines, so a
					// new scout gets a new key and reads no old mail.
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

// mailDeny returns the deny rules that keep each role out of the mail folder of dataDir, so a
// role reads its mail only through mail_read (task 31). Per the permission docs, the Read rules
// (with // for an absolute path) also cover Grep, Glob, and the Bash file commands that Claude
// Code recognizes (cat, head, tail, sed, tee, < redirects); the Bash rule covers the other Bash
// reads that name the folder, such as grep, less, find, and ls. A Read deny also blocks Edit and
// Write there; no role writes mail with them (mail_post writes in the MCP server). The MCP server
// and the hook scripts read the folder as their own processes, which the rules do not gate.
// ponytail: a speed bump, not a sandbox: a relative or quoted path, a glob, or a script gets past
// it; the upgrade is the OS sandbox of Claude Code.
func mailDeny(dataDir string) ([]string, error) {
	if dataDir == "" {
		return nil, errors.New("BRUH_DATA is not set")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	m := filepath.Join(abs, "mail")
	return []string{"Read(/" + m + ")", "Read(/" + m + "/**)", "Bash(*" + m + "*)"}, nil
}

// roleSettings builds a role settings file: the plugin defaults, the extra deny rules, the mail
// deny rules (mailDeny), the extra env values, the allow rules when extraAllow is not empty, and
// BRUH_ROLE_KEY.
func roleSettings(e Env, key string, extraEnv map[string]string, extraDeny, extraAllow []string) ([]byte, error) {
	mail, err := mailDeny(e.DataDir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(e.PluginRoot, "defaults", "role-settings.json"))
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
	for _, d := range slices.Concat(extraDeny, mail) {
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
