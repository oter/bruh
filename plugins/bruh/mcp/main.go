package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const usage = `usage: go run -C <plugin root>/mcp . [command]

With no command, the program is the bruh MCP server (stdio).

Commands:
  init --answers <file>    non-interactive /bruh:init; BRUH_INIT_<KEY> variables override the file
  role-settings <role key> write <data>/roles/<role key>.json with the defaults (never overwrites)
  watch [--data <dir>] [--once]
                           poll the code hosts of <data>/repos.json; one JSON line for each event
  merge-train [--data <dir>] [--wait-minutes <n>] [--answer Q-<id>] <repo> <number>...
                           merge the pull requests in order, each only with green checks,
                           and confirm each merge by reading the code host API
`

func main() {
	if len(os.Args) < 2 {
		srv := NewServer("bruh", "0.11.1", AllTools())
		if err := srv.Serve(EnvFromOS(), os.Stdin, os.Stdout); err != nil {
			log.Fatal(err)
		}
		return
	}
	os.Exit(runCLI(os.Args[1:], cliEnv(), os.Stdout, os.Stderr))
}

// cliEnv is EnvFromOS with the defaults of a command run from a shell, where Claude Code
// sets neither BRUH_DATA nor BRUH_PLUGIN_ROOT. The default data folder is the one of the
// marketplace install: the plugin ID with each character other than [A-Za-z0-9_-] replaced by "-".
func cliEnv() Env {
	env := EnvFromOS()
	dataName := regexp.MustCompile(`[^A-Za-z0-9_-]`).ReplaceAllString(pluginID, "-")
	env.DataDir = cmp.Or(env.DataDir, filepath.Join(env.Home, ".claude", "plugins", "data", dataName))
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
		applied, err := initApplyRun(env, res["plan_id"].(string), res["diff_sha256"].(string))
		for _, f := range applied {
			verb := "applied:"
			if f.Delete {
				verb = "deleted:"
			}
			fmt.Fprintln(stdout, verb, f.Path)
		}
		if err != nil {
			return fail(err)
		}
		fmt.Fprintln(stdout, "start bigm with:", res["launch_command"])
		return 0
	case "watch":
		fs := flag.NewFlagSet("watch", flag.ContinueOnError)
		fs.SetOutput(stderr)
		data := fs.String("data", env.DataDir, "plugin data folder")
		once := fs.Bool("once", false, "poll once and exit")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		env.DataDir = *data
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if err := runWatch(ctx, env, stdout, *once); err != nil {
			return fail(err)
		}
		return 0
	case "merge-train":
		fs := flag.NewFlagSet("merge-train", flag.ContinueOnError)
		fs.SetOutput(stderr)
		data := fs.String("data", env.DataDir, "plugin data folder")
		waitMin := fs.Int("wait-minutes", 30, "how long to wait for pending checks of each pull request")
		answer := fs.String("answer", "", "question ID of a P1 answer of bigm that allows these merges, when grants.md has no grant")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if fs.NArg() < 2 {
			fmt.Fprint(stderr, usage)
			return 2
		}
		env.DataDir = *data
		var numbers []int
		for _, a := range fs.Args()[1:] {
			n, err := strconv.Atoi(a)
			if err != nil || n < 1 {
				return fail(fmt.Errorf("not a pull request number: %q", a))
			}
			numbers = append(numbers, n)
		}
		cfg, err := loadRepos(env.DataDir)
		if err != nil {
			return fail(err)
		}
		i := slices.IndexFunc(cfg.Repos, func(r repoConfig) bool { return r.Repo == fs.Arg(0) })
		if i < 0 {
			return fail(fmt.Errorf("%s is not in repos.json", fs.Arg(0)))
		}
		if err := mergeGate(env, cfg.Repos[i], *answer, numbers); err != nil {
			return fail(err)
		}
		h, err := newHost(cfg.Repos[i])
		if err != nil {
			return fail(err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		if !mergeTrain(ctx, env, h, cfg.Repos[i], numbers, time.Duration(*waitMin)*time.Minute, 20*time.Second, stdout) {
			return 1
		}
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
// values of user_name, ledger_path, mode, and root are text; the other values are JSON.
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
		if slices.Contains([]string{"user_name", "ledger_path", "mode", "root"}, key) {
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
	// Only a remote clanker needs this command; its clerks get their files from role_settings_write,
	// and bigm gets its file from init.
	if k.Role != "clanker" {
		return "", fmt.Errorf("role-settings writes only clanker keys, not %s", key)
	}
	content, err := roleSettings(env.PluginRoot, k.String(), nil, nil, nil)
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
