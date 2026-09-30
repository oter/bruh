package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const usage = `usage: go run -C <plugin root>/mcp . [command]

With no command, the program is the bruh MCP server (stdio).

Commands:
  init --answers <file>    non-interactive /bruh:init; BRUH_INIT_<KEY> variables override the file
  role-settings <role key> write <data>/roles/<role key>.json with the defaults (never overwrites)
`

func main() {
	if len(os.Args) < 2 {
		srv := NewServer("bruh", "0.1.0-dev", AllTools())
		if err := srv.Serve(EnvFromOS(), os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	os.Exit(runCLI(os.Args[1:], cliEnv(), os.Stdout, os.Stderr))
}

// cliEnv is EnvFromOS with the defaults of a command run from a shell, where Claude Code
// sets neither BRUH_DATA nor BRUH_PLUGIN_ROOT.
func cliEnv() Env {
	env := EnvFromOS()
	env.DataDir = cmp.Or(env.DataDir, filepath.Join(env.Home, ".claude", "plugins", "data", "bruh-bruh"))
	if env.PluginRoot == "" {
		wd, _ := os.Getwd() // go run -C <plugin root>/mcp runs in mcp/
		env.PluginRoot = filepath.Dir(wd)
	}
	return env
}

func runCLI(args []string, env Env, stdout, stderr io.Writer) int {
	fail := func(err error) int {
		fmt.Fprintln(stderr, "bruh:", err)
		return 1
	}
	switch args[0] {
	case "init":
		fs := flag.NewFlagSet("init", flag.ContinueOnError)
		fs.SetOutput(stderr)
		file := fs.String("answers", "", "JSON file with the init answers")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		a, err := cliAnswers(*file, os.Environ())
		if err != nil {
			return fail(err)
		}
		res, err := initPlanRun(env, a)
		if err != nil {
			return fail(err)
		}
		fmt.Fprint(stdout, res["diff"])
		applied, err := initApplyRun(env, res["plan_id"].(string))
		for _, p := range applied {
			fmt.Fprintln(stdout, "applied:", p)
		}
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, "start bigm with:", res["launch_command"])
		return 0
	case "role-settings":
		if len(args) != 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		path, err := writeNewRoleSettings(env, args[1])
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, path)
		return 0
	}
	fmt.Fprint(stderr, usage)
	return 2
}

// cliAnswers reads the answers file (optional) and applies BRUH_INIT_<KEY> variables. The
// values of user_name, ledger_path, and mode are text; the other values are JSON.
func cliAnswers(file string, environ []string) (InitAnswers, error) {
	m := map[string]any{}
	if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return InitAnswers{}, err
		}
		if err := json.Unmarshal(data, &m); err != nil {
			return InitAnswers{}, fmt.Errorf("%s: %w", file, err)
		}
	}
	for _, kv := range environ {
		name, val, _ := strings.Cut(kv, "=")
		key, ok := strings.CutPrefix(name, "BRUH_INIT_")
		if !ok {
			continue
		}
		key = strings.ToLower(key)
		if slices.Contains([]string{"user_name", "ledger_path", "mode"}, key) {
			m[key] = val
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(val), &v); err != nil {
			return InitAnswers{}, fmt.Errorf("%s is not JSON: %w", name, err)
		}
		m[key] = v
	}
	raw, _ := json.Marshal(m)
	return parseAnswers(raw)
}

// writeNewRoleSettings writes the default role settings of key. A remote clanker has no
// BRUH_ROLE_KEY yet, so its Orca worker session runs this command instead of role_settings_write.
func writeNewRoleSettings(env Env, key string) (string, error) {
	k, err := ParseRoleKey(key)
	if err != nil {
		return "", err
	}
	content, err := roleSettings(env.PluginRoot, k.String(), nil, nil)
	if err != nil {
		return "", err
	}
	dir, err := env.Dir("roles")
	if err != nil {
		return "", err
	}
	path, err := filepath.Abs(filepath.Join(dir, k.String()+".json"))
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return "", fmt.Errorf("%s exists; it is not overwritten", path)
	}
	if err != nil {
		return "", err
	}
	_, werr := f.Write(content)
	return path, errors.Join(werr, f.Close())
}
