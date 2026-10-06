package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEnv(t *testing.T, role string) Env {
	t.Helper()
	home := t.TempDir()
	return Env{
		DataDir:          t.TempDir(),
		RoleKey:          role,
		Host:             "testhost",
		PluginRoot:       "..",
		Home:             home,
		SettingsFile:     filepath.Join(home, ".claude", "settings.json"),
		ClaudeBin:        filepath.Join(home, "no-claude-in-tests"),
		PollInterval:     10 * time.Millisecond,
		ProgressInterval: 30 * time.Millisecond,
		Now:              time.Now,
	}
}

// as returns a copy of env with another role key and the same data folder.
func as(env Env, role string) Env {
	env.RoleKey = role
	return env
}

// call runs one tool handler and returns its result decoded from JSON.
func call(t *testing.T, env Env, name string, args any) (any, error) {
	t.Helper()
	for _, tool := range AllTools() {
		if tool.Name != name {
			continue
		}
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		out, err := tool.Handler(&Call{Env: env}, raw)
		if err != nil {
			return nil, err
		}
		if s, ok := out.(string); ok {
			return s, nil
		}
		data, err := json.Marshal(out)
		if err != nil {
			t.Fatal(err)
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		return v, nil
	}
	t.Fatalf("unknown tool %s", name)
	return nil, nil
}

// fakeCLI points *bin at a sh script for the length of the test. The script appends one line
// with its arguments, joined by spaces, to the returned log file, then runs body: sh code in
// which "$@" are the arguments.
func fakeCLI(t *testing.T, bin *string, body string) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	logFile = filepath.Join(dir, "log")
	if err := os.WriteFile(logFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	// body can also use "$log", the log file.
	script := filepath.Join(dir, "cli")
	if err := os.WriteFile(script, fmt.Appendf(nil, "#!/bin/sh\nlog=%s\necho \"$*\" >> \"$log\"\n%s\n", shq(logFile), body), 0o700); err != nil {
		t.Fatal(err)
	}
	old := *bin
	*bin = script
	t.Cleanup(func() { *bin = old })
	return logFile
}

// fakeGlab points glabBin at a fake glab. It logs its arguments as fakeCLI does, and prints
// fixtures[<the first argument that starts with "projects/">]. A path with no fixture prints
// "404 Not Found" on stderr and exits 1.
func fakeGlab(t *testing.T, fixtures map[string]string) (logFile string) {
	t.Helper()
	dir := t.TempDir()
	var cases strings.Builder
	i := 0
	for path, text := range fixtures {
		file := filepath.Join(dir, fmt.Sprintf("fixture-%d", i))
		i++
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&cases, "%s) cat %s ;;\n", shq(path), shq(file))
	}
	return fakeCLI(t, &glabBin, `path=
for a in "$@"; do
	case $a in projects/*) [ -n "$path" ] || path=$a ;; esac
done
case $path in
`+cases.String()+`*) echo '404 Not Found' >&2; exit 1 ;;
esac`)
}
