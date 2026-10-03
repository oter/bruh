package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	// body can also use "$log", the log file; fakeGlab appends its stdin line there.
	script := filepath.Join(dir, "cli")
	if err := os.WriteFile(script, fmt.Appendf(nil, "#!/bin/sh\nlog=%s\necho \"$*\" >> \"$log\"\n%s\n", shq(logFile), body), 0o700); err != nil {
		t.Fatal(err)
	}
	old := *bin
	*bin = script
	t.Cleanup(func() { *bin = old })
	return logFile
}

// fakeGlab points glabBin at a fake glab. It logs its arguments as fakeCLI does, then the line
// "stdin: <text>" when an argument is "--input" followed by "-", and prints
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
	return fakeCLI(t, &glabBin, `prev=
path=
for a in "$@"; do
	if [ "$prev" = --input ] && [ "$a" = - ]; then echo "stdin: $(cat)" >> "$log"; fi
	case $a in projects/*) [ -n "$path" ] || path=$a ;; esac
	prev=$a
done
case $path in
`+cases.String()+`*) echo '404 Not Found' >&2; exit 1 ;;
esac`)
}

func TestFakeHelpers(t *testing.T) {
	bin := "old-bin"
	oldGlab := glabBin
	t.Run("fakes", func(t *testing.T) {
		log := fakeCLI(t, &bin, "echo hi")
		out, err := exec.CommandContext(t.Context(), bin, "a", "b c").Output()
		if err != nil || string(out) != "hi\n" {
			t.Fatalf("fakeCLI out = %q, %v", out, err)
		}
		if data, _ := os.ReadFile(log); string(data) != "a b c\n" {
			t.Fatalf("fakeCLI log = %q", data)
		}

		log = fakeGlab(t, map[string]string{"projects/g%2Fr": `{"x":1}`})
		glab := func(stdin string, args ...string) (string, string, error) {
			cmd := exec.CommandContext(t.Context(), glabBin, args...)
			cmd.Stdin = strings.NewReader(stdin)
			var stdout, stderr strings.Builder
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			return stdout.String(), stderr.String(), err
		}
		if out, _, err := glab("", "api", "--hostname", "h", "projects/g%2Fr"); err != nil || out != `{"x":1}` {
			t.Fatalf("fixture out = %q, %v", out, err)
		}
		if _, _, err := glab(`{"sha":"s"}`, "api", "--hostname", "h", "--input", "-", "projects/g%2Fr"); err != nil {
			t.Fatal(err)
		}
		_, errOut, err := glab("", "api", "--hostname", "h", "projects/none")
		if exit, ok := errors.AsType[*exec.ExitError](err); !ok || exit.ExitCode() != 1 || errOut != "404 Not Found\n" {
			t.Fatalf("unknown path: err = %v, stderr = %q", err, errOut)
		}
		want := "api --hostname h projects/g%2Fr\n" +
			"api --hostname h --input - projects/g%2Fr\n" +
			`stdin: {"sha":"s"}` + "\n" +
			"api --hostname h projects/none\n"
		if data, _ := os.ReadFile(log); string(data) != want {
			t.Fatalf("fakeGlab log = %q, want %q", data, want)
		}
	})
	if bin != "old-bin" || glabBin != oldGlab {
		t.Fatalf("after the test: bin = %q, glabBin = %q", bin, glabBin)
	}
}
