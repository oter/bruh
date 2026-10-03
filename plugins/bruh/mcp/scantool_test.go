package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// noLogin is a fake gh, glab, or tea that reports no login.
const noLogin = "exit 1"

// scanFakes points ghBin, glabBin, and teaBin at fakes that run cliBody, and sshBin at a fake
// that runs sshBody, in which "$3" is the alias, for the length of the test. It returns their
// logs by name. No BRUH_GITEA_TOKEN_ variable names git.example.org.
func scanFakes(t *testing.T, cliBody, sshBody string) map[string]string {
	t.Helper()
	t.Setenv("BRUH_GITEA_TOKEN_"+envHostKey("git.example.org"), "")
	return map[string]string{
		"gh":   fakeCLI(t, &ghBin, cliBody),
		"glab": fakeCLI(t, &glabBin, cliBody),
		"tea":  fakeCLI(t, &teaBin, cliBody),
		"ssh":  fakeCLI(t, &sshBin, sshBody),
	}
}

// echoHostname is a fake ssh that prints the alias as its hostname: no alias resolves.
const echoHostname = `printf 'hostname %s\n' "$3"`

// checkNoCalls reports an error for each fake whose log is not empty.
func checkNoCalls(t *testing.T, logs map[string]string) {
	t.Helper()
	for name, log := range logs {
		if data, _ := os.ReadFile(log); len(data) != 0 {
			t.Errorf("%s log = %q, want empty", name, data)
		}
	}
}

// scanArgs is the input of learn_scan with ledger and root, plus extra.
func scanArgs(ledger, root string, extra map[string]any) map[string]any {
	args := map[string]any{"ledger": ledger, "root": root}
	maps.Copy(args, extra)
	return args
}

func TestLearnScanRefusesRoleKey(t *testing.T) {
	logs := scanFakes(t, noLogin, echoHostname)
	root := t.TempDir()
	writeRepo(t, root, projectKey(t.Name()), originConfig("https://gitlab.com/team/"+projectKey(t.Name())+".git"), "origin", "")
	_, err := call(t, testEnv(t, "bigm"), "learn_scan", scanArgs(t.TempDir(), root, nil))
	if err == nil || !strings.Contains(err.Error(), "no role key") {
		t.Fatalf("learn_scan as bigm: err = %v, want an error that contains %q", err, "no role key")
	}
	checkNoCalls(t, logs)
}

func TestLearnScanRejectsBadInput(t *testing.T) {
	logs := scanFakes(t, noLogin, echoHostname)
	root := t.TempDir()
	writeRepo(t, root, projectKey(t.Name()), originConfig("https://git.example.org/team/"+projectKey(t.Name())), "origin", "")
	ledger := t.TempDir()
	tests := []struct {
		name string
		args map[string]any
	}{
		{"unknown key", scanArgs(ledger, root, map[string]any{"unknown": 1})},
		{"no ledger", map[string]any{"root": root}},
		{"relative root", scanArgs(ledger, "workspace", nil)},
		{"depth 9", scanArgs(ledger, root, map[string]any{"depth": 9})},
		{"alias with ;", scanArgs(ledger, root, map[string]any{"host_aliases": map[string]string{"code;alias": "gitlab.com"}})},
		{"kind bitbucket", scanArgs(ledger, root, map[string]any{"host_kinds": map[string]string{"git.example.org": "bitbucket"}})},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := call(t, testEnv(t, ""), "learn_scan", tt.args); err == nil {
				t.Errorf("learn_scan(%v): err = nil, want an error", tt.args)
			}
		})
	}
	checkNoCalls(t, logs)
}

// treeState returns the size, the mode, and the modification time of each file and folder
// under each of dirs.
func treeState(t *testing.T, dirs ...string) map[string]string {
	t.Helper()
	state := map[string]string{}
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			state[path] = fmt.Sprintf("%d %v %v", info.Size(), info.Mode(), info.ModTime())
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return state
}

func TestLearnScanWritesNothing(t *testing.T) {
	scanFakes(t, noLogin, `case "$3" in *-work) echo 'hostname gitlab.com' ;; *) printf 'hostname %s\n' "$3" ;; esac`)
	env := testEnv(t, "")
	root := t.TempDir()
	name := projectKey(t.Name())
	ledger := filepath.Join(root, name+"-ledger")
	writeRepo(t, root, name+"-ledger", "", "origin", "")
	writeRepo(t, root, name, originConfig("https://gitlab.com/team/"+name+".git"), "origin", "ref: refs/remotes/origin/main\n")
	writeRepo(t, root, name+"-app", originConfig("git@gitlab.com-work:team/"+name+"-app.git"), "origin", "")
	writeRepo(t, root, "group/"+name+"-db", originConfig("git@db-alias:team/"+name+"-db.git"), "origin", "")
	writeRepo(t, root, name+"-docs", originConfig("https://git.example.org/team/"+name+"-docs"), "origin", "")

	before := treeState(t, env.DataDir, root)
	if _, err := call(t, env, "learn_scan", scanArgs(ledger, root, nil)); err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	if after := treeState(t, env.DataDir, root); !maps.Equal(after, before) {
		t.Errorf("learn_scan changed the data folder or the root:\nbefore %q\nafter  %q", before, after)
	}
}

func TestLearnScanOutput(t *testing.T) {
	scanFakes(t, noLogin, `case "$3" in gitlab.com-work) echo 'hostname gitlab.com' ;; *) printf 'hostname %s\n' "$3" ;; esac`)
	root := t.TempDir()
	writeRepo(t, root, "shop", originConfig("https://gitlab.com/team/shop.git"), "origin", "ref: refs/remotes/origin/main\n")
	writeRepo(t, root, "shop-app", originConfig("git@gitlab.com-work:team/shop-app.git"), "origin", "")
	writeRepo(t, root, "group-a/auth", originConfig("git@auth-alias:team/auth.git"), "origin", "")
	writeRepo(t, root, "infra", originConfig("https://git.example.org/team/infra"), "origin", "")
	writeRepo(t, root, "ledger", "", "origin", "")

	got, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(filepath.Join(root, "ledger"), root, nil))
	if err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	const wantJSON = `{
		"repos": [
			{"path": "group-a/auth", "remotes": [{"name": "origin", "url": "git@auth-alias:team/auth.git"}], "remote": "origin",
			 "host": {"value": null, "source": "git"}, "kind": "unknown", "api_url": "", "host_path": "team/auth",
			 "default_branch": "unknown", "group": "group-a", "alias": "auth-alias", "activity": {"at": "", "state": "no login"}},
			{"path": "infra", "remotes": [{"name": "origin", "url": "https://git.example.org/team/infra"}], "remote": "origin",
			 "host": {"value": "git.example.org", "source": "git"}, "kind": "unknown", "api_url": "", "host_path": "team/infra",
			 "default_branch": "unknown", "group": "", "alias": "", "activity": {"at": "", "state": "no login"}},
			{"path": "shop", "remotes": [{"name": "origin", "url": "https://gitlab.com/team/shop.git"}], "remote": "origin",
			 "host": {"value": "gitlab.com", "source": "git"}, "kind": "gitlab", "api_url": "https://gitlab.com/api/v4", "host_path": "team/shop",
			 "default_branch": "main", "group": "", "alias": "", "activity": {"at": "", "state": "no login"}},
			{"path": "shop-app", "remotes": [{"name": "origin", "url": "git@gitlab.com-work:team/shop-app.git"}], "remote": "origin",
			 "host": {"value": "gitlab.com", "source": "git"}, "kind": "gitlab", "api_url": "https://gitlab.com/api/v4", "host_path": "team/shop-app",
			 "default_branch": "unknown", "group": "", "alias": "gitlab.com-work", "activity": {"at": "", "state": "no login"}}
		],
		"projects": [
			{"key": "auth", "repos": ["group-a/auth"], "main": "group-a/auth"},
			{"key": "infra", "repos": ["infra"], "main": "infra"},
			{"key": "shop", "repos": ["shop", "shop-app"], "main": "shop"}
		],
		"host_aliases": {"gitlab.com-work": "gitlab.com"},
		"hosts_without_kind": ["git.example.org"],
		"aliases_without_host": ["auth-alias"]
	}`
	var want any
	if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("learn_scan = %s\nwant %s", jsonText(t, got), jsonText(t, want))
	}
}

func TestLearnScanOutputKeys(t *testing.T) {
	scanFakes(t, noLogin, echoHostname)
	root := t.TempDir()
	name := projectKey(t.Name())
	writeRepo(t, root, name, originConfig("https://gitlab.com/team/"+name+".git"), "origin", "")

	got, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(t.TempDir(), root, nil))
	if err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	out, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("learn_scan = %#v, want an object", got)
	}
	if keys, want := slices.Sorted(maps.Keys(out)), []string{"aliases_without_host", "host_aliases", "hosts_without_kind", "projects", "repos"}; !slices.Equal(keys, want) {
		t.Errorf("learn_scan keys = %q, want %q", keys, want)
	}
	// No alias and a host with a kind: the lists are [] and the map is {}, never null.
	for _, k := range []string{"aliases_without_host", "hosts_without_kind", "projects", "repos"} {
		if _, ok := out[k].([]any); !ok {
			t.Errorf("learn_scan %s = %#v, want a list", k, out[k])
		}
	}
	if m, ok := out["host_aliases"].(map[string]any); !ok || len(m) != 0 {
		t.Errorf("learn_scan host_aliases = %#v, want {}", out["host_aliases"])
	}
	repos, _ := out["repos"].([]any)
	if len(repos) != 1 {
		t.Fatalf("learn_scan repos = %#v, want one repository", out["repos"])
	}
	repo, ok := repos[0].(map[string]any)
	if !ok {
		t.Fatalf("learn_scan repos[0] = %#v, want an object", repos[0])
	}
	want := []string{"activity", "alias", "api_url", "default_branch", "group", "host", "host_path", "kind", "path", "remote", "remotes"}
	if keys := slices.Sorted(maps.Keys(repo)); !slices.Equal(keys, want) {
		t.Errorf("learn_scan repos[0] keys = %q, want %q", keys, want)
	}
}

func TestLearnScanUnder2s(t *testing.T) {
	scanFakes(t, noLogin, echoHostname)
	root := t.TempDir()
	name := projectKey(t.Name())
	for i := range 60 {
		rel := fmt.Sprintf("%s-%02d", name, i)
		writeRepo(t, root, rel, originConfig("https://git.example.org/team/"+rel), "origin", "")
	}

	start := time.Now()
	got, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(t.TempDir(), root, nil))
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	if repos, _ := got.(map[string]any)["repos"].([]any); len(repos) != 60 {
		t.Errorf("learn_scan found %d repositories, want 60", len(repos))
	}
	if elapsed >= 2*time.Second {
		t.Errorf("learn_scan of 60 repositories took %v, want less than 2s", elapsed)
	}
}

func TestLearnScanCallsNoHostWithoutLogin(t *testing.T) {
	// The CLIs report no login but answer each other call with an activity time.
	logs := scanFakes(t,
		`case "$1" in auth|logins) exit 1 ;; esac; echo '{"pushed_at":"2026-10-01T12:00:00Z","last_activity_at":"2026-10-01T12:00:00Z"}'`,
		`case "$3" in *-work) echo 'hostname gitlab.com' ;; *) printf 'hostname %s\n' "$3" ;; esac`)
	root := t.TempDir()
	name := projectKey(t.Name())
	writeRepo(t, root, name+"-github", originConfig("https://github.com/team/"+name+"-github.git"), "origin", "")
	writeRepo(t, root, name+"-gitlab", originConfig("https://gitlab.com/team/"+name+"-gitlab.git"), "origin", "")
	writeRepo(t, root, name+"-work", originConfig("git@gitlab.com-work:team/"+name+"-work.git"), "origin", "")

	got, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(t.TempDir(), root, nil))
	if err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	for name, want := range map[string]string{"gh": "auth status --json hosts\n", "glab": "auth status --all\n"} {
		if data, _ := os.ReadFile(logs[name]); string(data) != want {
			t.Errorf("%s log = %q, want %q", name, data, want)
		}
	}
	repos, _ := got.(map[string]any)["repos"].([]any)
	if len(repos) != 3 {
		t.Fatalf("learn_scan repos = %s, want 3 repositories", jsonText(t, repos))
	}
	for _, r := range repos {
		if a := r.(map[string]any)["activity"]; !reflect.DeepEqual(a, map[string]any{"at": "", "state": "no login"}) {
			t.Errorf("learn_scan activity of %v = %v, want no login", r.(map[string]any)["path"], a)
		}
	}
}

func TestScanRunsNothing(t *testing.T) {
	logs := scanFakes(t, noLogin, echoHostname)
	// Fake git, make, and task first on PATH; each one makes a marker file in the markers folder.
	fake, markers := t.TempDir(), t.TempDir()
	for _, cmd := range []string{"git", "make", "task"} {
		script := fmt.Sprintf("#!/bin/sh\ntouch %s\n", shq(filepath.Join(markers, cmd)))
		if err := os.WriteFile(filepath.Join(fake, cmd), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", fake+":"+os.Getenv("PATH"))
	root := t.TempDir()
	name := projectKey(t.Name())
	config := "[core]\n\tfsmonitor = touch y\n" + originConfig("git@"+name+"-alias:team/"+name+".git")
	writeRepo(t, root, name, config, "origin", "")

	if _, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(t.TempDir(), root, nil)); err != nil {
		t.Fatalf("learn_scan: %v", err)
	}
	if data, _ := os.ReadFile(logs["ssh"]); len(data) == 0 {
		t.Errorf("ssh log is empty, want a lookup of the alias %s-alias", name)
	}
	for _, dir := range []string{root, fake, markers} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.Name() == "y" || dir == markers && path != markers {
				t.Errorf("learn_scan ran a command: %s exists", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat("y"); err == nil {
		t.Error("learn_scan ran a command: y exists in the test folder")
	}
}

// findKeys reports an error for each object key of v, at any level, that is in keys.
func findKeys(t *testing.T, v any, keys ...string) {
	t.Helper()
	switch v := v.(type) {
	case map[string]any:
		for k, sub := range v {
			if slices.Contains(keys, k) {
				t.Errorf("learn_scan output has the key %q", k)
			}
			findKeys(t, sub, keys...)
		}
	case []any:
		for _, sub := range v {
			findKeys(t, sub, keys...)
		}
	}
}

func TestScanReadsNoWorkingTreeFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads each file, whatever its mode")
	}
	scanFakes(t, noLogin, echoHostname)
	root := t.TempDir()
	name := projectKey(t.Name())
	writeRepo(t, root, name, originConfig("https://gitlab.com/team/"+name+".git"), "origin", "ref: refs/remotes/origin/main\n")
	files := map[string]string{
		"README.md":    "# " + name + "\n",
		"Taskfile.yml": "version: '3'\ntasks:\n  test:\n    cmds: [go test ./...]\n",
		"go.mod":       "module example.com/" + name + "\n\ngo 1.26\n",
		"package.json": `{"name": "` + name + `", "scripts": {"test": "jest"}}` + "\n",
	}
	for file, text := range files {
		if err := os.WriteFile(filepath.Join(root, name, file), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	scan := func() string {
		t.Helper()
		got, err := call(t, testEnv(t, ""), "learn_scan", scanArgs(t.TempDir(), root, nil))
		if err != nil {
			t.Fatalf("learn_scan: %v", err)
		}
		findKeys(t, got, "stack", "gates", "ci", "links")
		return jsonText(t, got)
	}

	readable := scan()
	for file := range files {
		path := filepath.Join(root, name, file)
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(path, 0o644) })
	}
	if unreadable := scan(); unreadable != readable {
		t.Errorf("learn_scan with unreadable working tree files = %s\nwant %s", unreadable, readable)
	}
}
