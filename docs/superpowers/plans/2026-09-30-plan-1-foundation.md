# Plan 1: Foundation, probes, and the bruh MCP server

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Before you edit any Go file, run the `modern-go-guidelines:use-modern-go` skill `list` command for that file and follow its guidelines.

**Goal:** Create the plugin skeleton, prove the "Verify" items that later plans depend on, and build the bruh MCP server that does every file write of bruh.

**Architecture:** One Claude Code plugin marketplace repository with one plugin, `bruh`. The MCP server is a Go program that uses only the standard library. Claude Code starts it with `go run -C ${CLAUDE_PLUGIN_ROOT}/mcp .`, so the repository contains no binaries. The server speaks JSON-RPC 2.0 over stdio (one JSON message for each line). It writes only under `BRUH_DATA` (the plugin data folder) and identifies its caller by `BRUH_ROLE_KEY`. All tool handlers get an `Env` value instead of reading process variables, so tests run many callers in one process.

**Tech Stack:** Go 1.26 or later (standard library only), JSON, POSIX shell, GitHub Actions.

**Spec:** [../../spec.md](../../spec.md) version 0.4. Index: [2026-09-30-bruh-v0.1-index.md](2026-09-30-bruh-v0.1-index.md).

## Global Constraints

- All bruh files are in the plugin data folder; plugin code writes them (spec 2, principle 3; spec 10.1).
- Timestamps come from code, in the fixed layout `2006-01-02T15:04:05.000Z` (UTC, millisecond precision). The fixed layout makes string comparison equal to time comparison (spec 2, principle 4).
- Role keys: `bigm`, `clanker-<project>`, `clerk-<project>-<task>`, `clerk-ledger` (spec 3.2). Pattern `^[a-z0-9][a-z0-9-]{0,63}$`.
- Handoff text of the caller is at most 8,000 characters (spec 7).
- Message headers: `P0 Q-<n>:`, `P1 Q-<n>:`, `P2 Q-<n>:`, `ANSWER Q-<n>:`, `REC Q-<n>:`, `RULE R-<n>:`, `DONE:` (spec 5).
- The MCP server uses only the Go standard library. `go.mod` has no `require` lines. `GOTOOLCHAIN=local`, so `go run` never downloads a toolchain.
- Runs on macOS and Linux (spec 10.2). CI tests both.
- License Apache-2.0 (spec 19).
- Public repository: no personal names, no private repository names, no host names, no `/Users` paths in any committed file.
- Git identity for this repository: `Max <4347218+oter@users.noreply.github.com>` (already set as repository-local config).

## Review Focus

1. **Two sessions write the lease state at the same time.** Expected: no lost update. Test in Task 7: 20 parallel `lease_grant` calls keep an exact count.
2. **The caller has no `BRUH_ROLE_KEY`** (a manual session of the owner loads the plugin). Expected: each tool returns an error that says `BRUH_ROLE_KEY is not set` and writes nothing. Test in Task 3, run against every tool.
3. **A role key or question ID with path characters** (`../x`, `a/b`). Expected: refused before any file operation. Tests in Tasks 4 and 5.
4. **A handoff over 8,000 characters or with only file pointers.** Expected: refused with the reason, and the old file stays. Test in Task 4.
5. **`answer_wait` waits for hours.** Expected: progress notifications keep the call alive, and the deadline returns `pending`. Test in Task 5 with short intervals, and probe P8.

---

### Task 1: Repository skeleton, license, and CI

**Files:**

- Create: `.claude-plugin/marketplace.json`
- Create: `plugins/bruh/.claude-plugin/plugin.json`
- Create: `plugins/bruh/.mcp.json`
- Create: `plugins/bruh/defaults/role-settings.json`
- Create: `plugins/bruh/mcp/go.mod`
- Create: `plugins/bruh/mcp/main.go`
- Create: `plugins/bruh/mcp/skeleton_test.go`
- Create: `LICENSE`, `.gitignore`, `.markdownlint.json`
- Create: `.github/workflows/ci.yml`

**Interfaces:**

- Produces: plugin name `bruh`, marketplace name `bruh`, MCP server name `bruh`. The server env: `BRUH_DATA=${CLAUDE_PLUGIN_DATA}`, `BRUH_PLUGIN_ROOT=${CLAUDE_PLUGIN_ROOT}`, `GOTOOLCHAIN=local`. Test command: `cd plugins/bruh/mcp && go test ./...`.

- [ ] **Step 1: Write the failing test**

`plugins/bruh/mcp/go.mod`:

```text
module github.com/oter/bruh/mcp

go 1.26
```

`plugins/bruh/mcp/skeleton_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"
)

func readJSON(t *testing.T, rel string, v any) {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(rel))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatalf("%s: %v", rel, err)
	}
}

func TestMarketplaceListsBruh(t *testing.T) {
	var m struct {
		Name    string `json:"name"`
		Plugins []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"plugins"`
	}
	readJSON(t, "../../../.claude-plugin/marketplace.json", &m)
	if m.Name != "bruh" || len(m.Plugins) != 1 || m.Plugins[0].Name != "bruh" || m.Plugins[0].Source != "./plugins/bruh" {
		t.Fatalf("marketplace = %+v", m)
	}
}

func TestPluginManifest(t *testing.T) {
	var p struct {
		Name    string `json:"name"`
		License string `json:"license"`
	}
	readJSON(t, "../.claude-plugin/plugin.json", &p)
	if p.Name != "bruh" || p.License != "Apache-2.0" {
		t.Fatalf("plugin = %+v", p)
	}
}

func TestMCPConfig(t *testing.T) {
	var c struct {
		MCPServers map[string]struct {
			Command string            `json:"command"`
			Args    []string          `json:"args"`
			Env     map[string]string `json:"env"`
			Timeout int               `json:"timeout"`
		} `json:"mcpServers"`
	}
	readJSON(t, "../.mcp.json", &c)
	s, ok := c.MCPServers["bruh"]
	if !ok {
		t.Fatal("no bruh server")
	}
	if s.Command != "go" || !slices.Equal(s.Args, []string{"run", "-C", "${CLAUDE_PLUGIN_ROOT}/mcp", "."}) {
		t.Fatalf("server = %+v", s)
	}
	if s.Env["BRUH_DATA"] != "${CLAUDE_PLUGIN_DATA}" || s.Env["BRUH_PLUGIN_ROOT"] != "${CLAUDE_PLUGIN_ROOT}" || s.Env["GOTOOLCHAIN"] != "local" {
		t.Fatalf("env = %v", s.Env)
	}
	if s.Timeout != 86400000 {
		t.Fatalf("timeout = %d", s.Timeout)
	}
}

func TestLicenseIsApache(t *testing.T) {
	data, err := os.ReadFile("../../../LICENSE")
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`Apache License\s+Version 2\.0, January 2004`).Match(data) {
		t.Fatal("LICENSE is not Apache-2.0")
	}
}
```

`plugins/bruh/mcp/main.go` (a stub so that the package compiles; Task 3 replaces it):

```go
package main

func main() {}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: FAIL with `no such file or directory` for `marketplace.json`.

- [ ] **Step 3: Write the files**

`.claude-plugin/marketplace.json`:

```json
{
  "name": "bruh",
  "owner": { "name": "oter" },
  "plugins": [
    {
      "name": "bruh",
      "source": "./plugins/bruh",
      "description": "Coordinate Claude Code sessions: bigm, clankers, clerks, and workflows."
    }
  ]
}
```

`plugins/bruh/.claude-plugin/plugin.json`:

```json
{
  "name": "bruh",
  "version": "0.1.0-dev",
  "description": "Coordinate Claude Code sessions across projects and machines: bigm, clankers, clerks, and workflows.",
  "license": "Apache-2.0",
  "repository": "https://github.com/oter/bruh"
}
```

`plugins/bruh/.mcp.json`:

```json
{
  "mcpServers": {
    "bruh": {
      "command": "go",
      "args": ["run", "-C", "${CLAUDE_PLUGIN_ROOT}/mcp", "."],
      "env": {
        "BRUH_DATA": "${CLAUDE_PLUGIN_DATA}",
        "BRUH_PLUGIN_ROOT": "${CLAUDE_PLUGIN_ROOT}",
        "GOTOOLCHAIN": "local"
      },
      "timeout": 86400000
    }
  }
}
```

`plugins/bruh/defaults/role-settings.json` (the deny rules of spec 13 that match exact command patterns; Plan 2 extends the list):

```json
{
  "permissions": {
    "deny": [
      "Bash(docker volume rm:*)",
      "Bash(docker volume prune:*)",
      "Bash(git push --force:*)",
      "Bash(git push -f:*)"
    ]
  }
}
```

`.gitignore`:

```text
*.tmp
.remember/
```

`.markdownlint.json` (long lines and inline HTML in Mermaid labels are allowed; if the first CI run reports other findings in the existing docs, fix the docs, not the rules):

```json
{ "MD013": false, "MD033": false, "MD024": { "siblings_only": true } }
```

`LICENSE`: download the official text, do not type it:

```bash
curl -sfL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE
```

`.github/workflows/ci.yml`:

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
permissions:
  contents: read
jobs:
  go:
    strategy:
      matrix:
        os: [ubuntu-latest, macos-latest]
    runs-on: ${{ matrix.os }}
    defaults:
      run:
        working-directory: plugins/bruh/mcp
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: plugins/bruh/mcp/go.mod
      - run: test -z "$(gofmt -l .)"
      - run: go vet ./...
      - run: go test -race ./...
  lint:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: shellcheck
        run: |
          files=$(git ls-files '*.sh')
          if [ -n "$files" ]; then shellcheck $files; fi
      - uses: DavidAnson/markdownlint-cli2-action@v19
        with:
          globs: '**/*.md'
      - uses: lycheeverse/lychee-action@v2
        with:
          args: --no-progress './**/*.md'
  plugin-validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-node@v4
        with:
          node-version: 22
      - run: npm install -g @anthropic-ai/claude-code
      - run: claude plugin validate ./plugins/bruh
      - run: claude plugin validate .
```

- [ ] **Step 4: Run the test to verify that it passes**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: `ok  github.com/oter/bruh/mcp`.

- [ ] **Step 5: Validate the plugin locally**

Run: `claude plugin validate ./plugins/bruh && claude plugin validate .`
Expected: exit code 0 for both.

- [ ] **Step 6: Commit**

```bash
git add .claude-plugin plugins LICENSE .gitignore .markdownlint.json .github
git commit -m "Add the plugin skeleton, Apache-2.0 license, and CI"
```

---

### Task 2: Probes for the Verify items

Each probe answers one "Verify" item that Plans 2 to 5 depend on. A probe prints `PASS` or `FAIL` and the evidence. The results go into `docs/knowledge.md`, section "Probe results (plan 1)". A probe that fails does not stop this plan; it changes a later plan.

**Files:**

- Create: `probes/README.md`
- Create: `probes/probe-plugin/.claude-plugin/plugin.json`
- Create: `probes/probe-plugin/agents/probe-role.md`
- Create: `probes/probe-plugin/hooks/hooks.json`
- Create: `probes/probe-plugin/scripts/record.sh`
- Create: `probes/run.sh`
- Modify: `docs/knowledge.md`

**Interfaces:**

- Produces: the results P1 to P8 in `docs/knowledge.md`. Plans 2 to 5 read them.

| ID | Spec item | Question |
|---|---|---|
| P1 | 3 | Does `--agent probe:probe-role` load a plugin agent? |
| P2 | 4.1 | Does a `claude --bg` session started from a script with `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1` appear in `claude agents --json`? |
| P3 | 3.1, 10.1 | Do `env` values of a `--settings` file reach hook processes of a background session? |
| P4 | 7 | Does the status line command run in a background session with no attached terminal? |
| P5 | 8.4 | Does a plugin `PreToolUse` hook fire for the tool calls of a subagent in a background session, with `agent_id` in the input? |
| P6 | 6.1 | Can a background session launch a plugin workflow by slash command with the allow rule `Workflow(<plugin>:<name>)`? |
| P7 | 9.1 | Does a recurring `CronCreate` task fire in an interactive session that nobody types into? |
| P8 | 6.2 | Does a stdio MCP tool call that sends a progress notification each 60 seconds stay alive for 10 minutes? |

- [ ] **Step 1: Write the probe plugin**

`probes/probe-plugin/.claude-plugin/plugin.json`:

```json
{ "name": "probe", "version": "0.0.1", "description": "Probes for bruh Verify items. Not shipped." }
```

`probes/probe-plugin/agents/probe-role.md`:

```markdown
---
name: probe-role
description: Probe agent for bruh. Reports its own identity.
---
You are the probe role. When asked "who are you", answer exactly: PROBE-ROLE-LOADED.
```

`probes/probe-plugin/scripts/record.sh`:

```sh
#!/bin/sh
# Records one hook event for the bruh probes. Output folder: $PROBE_OUT.
set -eu
out="${PROBE_OUT:-/tmp/bruh-probes}"
mkdir -p "$out"
input=$(cat)
event=$(printf '%s' "$input" | jq -r '.hook_event_name // "unknown"')
agent_id=$(printf '%s' "$input" | jq -r '.agent_id // ""')
printf '%s\t%s\t%s\t%s\n' "$event" "${BRUH_ROLE_KEY:-}" "$agent_id" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$out/hooks.tsv"
exit 0
```

`probes/probe-plugin/hooks/hooks.json`:

```json
{
  "hooks": {
    "SessionStart": [{ "hooks": [{ "type": "command", "command": "sh \"${CLAUDE_PLUGIN_ROOT}/scripts/record.sh\"" }] }],
    "PreToolUse": [{ "hooks": [{ "type": "command", "command": "sh \"${CLAUDE_PLUGIN_ROOT}/scripts/record.sh\"" }] }]
  }
}
```

- [ ] **Step 2: Write the probe runner**

`probes/run.sh`:

```sh
#!/bin/sh
# Runs the bruh probes P1 to P3. P4 to P8 are manual; see probes/README.md.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
export PROBE_OUT="${PROBE_OUT:-/tmp/bruh-probes}"
rm -rf "$PROBE_OUT"; mkdir -p "$PROBE_OUT"
work=$(mktemp -d); cd "$work"; git init -q .
settings="$PROBE_OUT/role.json"
printf '{"env":{"BRUH_ROLE_KEY":"clerk-probe-p3"}}' > "$settings"

echo "P1: plugin agent by namespaced name"
if claude -p --plugin-dir "$here/probe-plugin" --agent probe:probe-role "who are you" | grep -q PROBE-ROLE-LOADED; then echo "P1 PASS"; else echo "P1 FAIL"; fi

echo "P2 and P3: background session registration and settings env"
CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --plugin-dir "$here/probe-plugin" \
  --name probe-p2 --permission-mode auto --settings "$settings" "Run the shell command: echo hello" > "$PROBE_OUT/bg.txt" 2>&1 || true
sleep 60
if claude agents --json --all | jq -e '.[] | select(.name=="probe-p2")' >/dev/null; then echo "P2 PASS"; else echo "P2 FAIL"; fi
if grep -q 'clerk-probe-p3' "$PROBE_OUT/hooks.tsv" 2>/dev/null; then echo "P3 PASS (hooks get BRUH_ROLE_KEY)"; else echo "P3 FAIL"; fi
id=$(claude agents --json --all | jq -r '.[] | select(.name=="probe-p2") | .id' | head -1)
[ -n "$id" ] && claude stop "$id" >/dev/null 2>&1 || true
echo "Evidence: $PROBE_OUT"
```

`probes/README.md`:

```markdown
# Probes

`sh probes/run.sh` runs P1 to P3 and prints PASS or FAIL.

- P4 status line in a background session: in a scratch folder, set a status line command that appends `date -u` to `/tmp/bruh-probes/statusline.log`, start a `claude --bg` session with a 3-minute task, and do not attach. PASS if the log grows while no terminal is attached.
- P5 hook in a subagent of a background session: start `claude --bg --plugin-dir probes/probe-plugin` with the prompt "Use a subagent to run: echo sub". PASS if `/tmp/bruh-probes/hooks.tsv` has a `PreToolUse` line with a non-empty `agent_id`.
- P6 plugin workflow from a background session: add a one-agent workflow `probes/probe-plugin/workflows/hello.js` with `meta.name` `hello`, add the allow rule `Workflow(probe:hello)` to the settings file, and start `claude --bg` with the prompt `/probe:hello`. PASS if `claude logs <id>` shows the workflow result without an approval prompt.
- P7 CronCreate while idle: in an interactive session, ask Claude to create a recurring CronCreate task every minute that appends `date -u` to `/tmp/bruh-probes/cron.log`. Do not type for 5 minutes. PASS if the log has 4 or more lines.
- P8 long MCP call: after Task 5, call `answer_wait` with a deadline of 600 seconds from a session. PASS if the call returns `pending` after 600 seconds and not an idle-timeout error.
```

- [ ] **Step 3: Run the probes**

Run: `sh probes/run.sh`
Expected: three lines, `P1 ...`, `P2 ...`, `P3 ...`, each PASS or FAIL. Then run P4 to P7 as `probes/README.md` says. Run P8 after Task 5.

- [ ] **Step 4: Record the results**

Append to `docs/knowledge.md` (fill the date from `date -u +%F` and the version from `claude --version`):

```markdown
## Probe results (plan 1, <date>, Claude Code <version>)

| ID | Result | Evidence |
|---|---|---|
| P1 | PASS or FAIL | the output line |
```

Add one row for each probe with the real output. For each FAIL, add one line under the table: the spec section that changes, and how. Do not change the spec in this plan.

- [ ] **Step 5: Commit**

```bash
git add probes docs/knowledge.md
git commit -m "Add the probes for the Verify items and record their results"
```

---

### Task 3: MCP server core (Env, store, and JSON-RPC over stdio)

**Files:**

- Create: `plugins/bruh/mcp/env.go`
- Create: `plugins/bruh/mcp/rpc.go`
- Create: `plugins/bruh/mcp/tools.go`
- Modify: `plugins/bruh/mcp/main.go`
- Create: `plugins/bruh/mcp/helpers_test.go`
- Create: `plugins/bruh/mcp/rpc_test.go`

**Interfaces:**

- Produces:
  - `type Env struct { DataDir, RoleKey, PluginRoot string; PollInterval, ProgressInterval time.Duration; Now func() time.Time }` and `EnvFromOS() Env`.
  - `(Env) Caller() (string, error)`, `(Env) Dir(parts ...string) (string, error)`, `(Env) Stamp() string`, `(Env) WithLock(name string, fn func() error) error`.
  - `checkKey(s, what string) (string, error)`, `checkID(s string, re *regexp.Regexp, what string) (string, error)`, `atomicWrite(file string, data []byte) error`, `const stampLayout = "2006-01-02T15:04:05.000Z"`.
  - `type Tool struct { Name, Description string; InputSchema map[string]any; Handler func(c *Call, args json.RawMessage) (any, error) }`.
  - `type Call struct { Env Env; progress func(n int, msg string) }` with `(*Call) Progress(n int, msg string)`.
  - `NewServer(name, version string, tools []Tool) *Server` and `(*Server) Serve(env Env, in io.Reader, out io.Writer) error`.
  - `AllTools() []Tool` in `tools.go`. Tasks 4 to 7 add their tools to it.
  - `decode[T any](raw json.RawMessage) (T, error)`.
  - Test helpers: `testEnv(t, role) Env`, `as(env, role) Env`, `call(t, env, name, args) (any, error)`, which returns the result decoded from JSON.
- Rule for Tasks 4 to 7: each handler calls `c.Env.Caller()` before it touches the file system.

- [ ] **Step 1: Write the test helpers and the failing tests**

`plugins/bruh/mcp/helpers_test.go`:

```go
package main

import (
	"encoding/json"
	"testing"
	"time"
)

func testEnv(t *testing.T, role string) Env {
	t.Helper()
	return Env{
		DataDir:          t.TempDir(),
		RoleKey:          role,
		PluginRoot:       "..",
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
```

`plugins/bruh/mcp/rpc_test.go`:

```go
package main

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

type rpcClient struct {
	in  *io.PipeWriter
	out *bufio.Scanner
}

func startRPC(t *testing.T, env Env, tools []Tool) *rpcClient {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	srv := NewServer("bruh", "test", tools)
	go func() { _ = srv.Serve(env, inR, outW); outW.Close() }()
	t.Cleanup(func() { inW.Close() })
	return &rpcClient{in: inW, out: bufio.NewScanner(outR)}
}

func (c *rpcClient) send(t *testing.T, msg string) {
	t.Helper()
	if _, err := io.WriteString(c.in, msg+"\n"); err != nil {
		t.Fatal(err)
	}
}

func (c *rpcClient) next(t *testing.T) map[string]any {
	t.Helper()
	if !c.out.Scan() {
		t.Fatal("no response")
	}
	var m map[string]any
	if err := json.Unmarshal(c.out.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestInitializeAndList(t *testing.T) {
	c := startRPC(t, testEnv(t, "bigm"), AllTools())
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	init := c.next(t)
	info := init["result"].(map[string]any)["serverInfo"].(map[string]any)
	if info["name"] != "bruh" {
		t.Fatalf("init = %v", init)
	}
	c.send(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	list := c.next(t)
	if _, ok := list["result"].(map[string]any)["tools"].([]any); !ok {
		t.Fatalf("list = %v", list)
	}
}

func TestUnknownTool(t *testing.T) {
	c := startRPC(t, testEnv(t, "bigm"), AllTools())
	c.send(t, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"no_such_tool","arguments":{}}}`)
	resp := c.next(t)
	if !strings.Contains(resp["error"].(map[string]any)["message"].(string), "unknown tool") {
		t.Fatalf("resp = %v", resp)
	}
}

func TestToolErrorAndProgress(t *testing.T) {
	tools := []Tool{{
		Name:        "slow",
		InputSchema: map[string]any{"type": "object"},
		Handler: func(c *Call, _ json.RawMessage) (any, error) {
			c.Progress(1, "working")
			return nil, io.ErrUnexpectedEOF
		},
	}}
	c := startRPC(t, testEnv(t, "bigm"), tools)
	c.send(t, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"slow","arguments":{},"_meta":{"progressToken":"tok"}}}`)
	progress := c.next(t)
	if progress["method"] != "notifications/progress" || progress["params"].(map[string]any)["progressToken"] != "tok" {
		t.Fatalf("progress = %v", progress)
	}
	result := c.next(t)["result"].(map[string]any)
	if result["isError"] != true {
		t.Fatalf("result = %v", result)
	}
}

func TestMissingRoleKeyWritesNothing(t *testing.T) {
	env := testEnv(t, "")
	for _, tool := range AllTools() {
		_, err := tool.Handler(&Call{Env: env}, json.RawMessage(`{}`))
		if err == nil || !strings.Contains(err.Error(), "BRUH_ROLE_KEY is not set") {
			t.Errorf("%s: err = %v", tool.Name, err)
		}
	}
	entries, err := os.ReadDir(env.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("data folder has %d entries", len(entries))
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: build FAIL: `undefined: Env`, `undefined: NewServer`, `undefined: AllTools`.

- [ ] **Step 3: Write `env.go`**

```go
package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

const stampLayout = "2006-01-02T15:04:05.000Z"

var keyRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// Env is everything a tool handler needs from its process.
type Env struct {
	DataDir          string
	RoleKey          string
	PluginRoot       string
	PollInterval     time.Duration
	ProgressInterval time.Duration
	Now              func() time.Time
}

func EnvFromOS() Env {
	return Env{
		DataDir:          os.Getenv("BRUH_DATA"),
		RoleKey:          os.Getenv("BRUH_ROLE_KEY"),
		PluginRoot:       os.Getenv("BRUH_PLUGIN_ROOT"),
		PollInterval:     durationEnv("BRUH_POLL_MS", 2*time.Second),
		ProgressInterval: durationEnv("BRUH_PROGRESS_MS", time.Minute),
		Now:              time.Now,
	}
}

func durationEnv(name string, def time.Duration) time.Duration {
	ms, err := strconv.Atoi(os.Getenv(name))
	if err != nil || ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// Caller returns the role key of the calling session.
func (e Env) Caller() (string, error) {
	if e.RoleKey == "" {
		return "", errors.New("BRUH_ROLE_KEY is not set")
	}
	return checkKey(e.RoleKey, "role key")
}

func checkKey(s, what string) (string, error) {
	return checkID(s, keyRE, what)
}

func checkID(s string, re *regexp.Regexp, what string) (string, error) {
	if !re.MatchString(s) {
		return "", fmt.Errorf("invalid %s: %q", what, s)
	}
	return s, nil
}

// Dir returns a folder under the data folder and creates it with mode 0700.
func (e Env) Dir(parts ...string) (string, error) {
	if e.DataDir == "" {
		return "", errors.New("BRUH_DATA is not set")
	}
	p := filepath.Join(append([]string{e.DataDir}, parts...)...)
	return p, os.MkdirAll(p, 0o700)
}

// Stamp is the current UTC time in the fixed layout.
func (e Env) Stamp() string {
	return e.Now().UTC().Format(stampLayout)
}

// atomicWrite writes a temporary file in the same folder and renames it.
func atomicWrite(file string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(file), filepath.Base(file)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, file)
}

// WithLock runs fn while it holds a lock folder. Creating a folder is atomic.
func (e Env) WithLock(name string, fn func() error) error {
	locks, err := e.Dir("locks")
	if err != nil {
		return err
	}
	lock := filepath.Join(locks, name+".lock")
	deadline := time.Now().Add(10 * time.Second)
	for {
		err := os.Mkdir(lock, 0o700)
		if err == nil {
			break
		}
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		if fi, serr := os.Stat(lock); serr == nil && time.Since(fi.ModTime()) > 30*time.Second {
			os.RemoveAll(lock)
			continue
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("lock busy: %s", name)
		}
		time.Sleep(25 * time.Millisecond)
	}
	defer os.RemoveAll(lock)
	return fn()
}
```

- [ ] **Step 4: Write `rpc.go`**

```go
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"encoding/json"
	"io"
	"sync"
)

type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(c *Call, args json.RawMessage) (any, error)
}

// Call is the context of one tool call.
type Call struct {
	Env      Env
	progress func(n int, msg string)
}

// Progress sends a progress notification when the request asked for it.
func (c *Call) Progress(n int, msg string) {
	if c.progress != nil {
		c.progress(n, msg)
	}
}

func decode[T any](raw json.RawMessage) (T, error) {
	var v T
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	err := json.Unmarshal(raw, &v)
	return v, err
}

type Server struct {
	name, version string
	tools         []Tool
	byName        map[string]Tool
	mu            sync.Mutex
	enc           *json.Encoder
}

func NewServer(name, version string, tools []Tool) *Server {
	s := &Server{name: name, version: version, tools: tools, byName: map[string]Tool{}}
	for _, t := range tools {
		s.byName[t.Name] = t
	}
	return s
}

type request struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

func (s *Server) send(msg any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.enc.Encode(msg)
}

func (s *Server) result(id json.RawMessage, result any) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *Server) fail(id json.RawMessage, code int, msg string) {
	s.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": msg}})
}

func textResult(text string, isError bool) map[string]any {
	r := map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	if isError {
		r["isError"] = true
	}
	return r
}

// Serve reads one JSON-RPC message for each line and answers each request in its own goroutine.
func (s *Server) Serve(env Env, in io.Reader, out io.Writer) error {
	s.enc = json.NewEncoder(out)
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var wg sync.WaitGroup
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			s.fail(json.RawMessage("null"), -32700, "parse error")
			continue
		}
		if len(req.ID) == 0 || string(req.ID) == "null" {
			continue // a notification from the client
		}
		wg.Go(func() { s.handle(env, req) })
	}
	wg.Wait()
	return sc.Err()
}

func (s *Server) handle(env Env, req request) {
	switch req.Method {
	case "initialize":
		p, _ := decode[struct {
			ProtocolVersion string `json:"protocolVersion"`
		}](req.Params)
		s.result(req.ID, map[string]any{
			"protocolVersion": cmp.Or(p.ProtocolVersion, "2025-06-18"),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": s.name, "version": s.version},
		})
	case "ping":
		s.result(req.ID, map[string]any{})
	case "tools/list":
		list := make([]map[string]any, 0, len(s.tools))
		for _, t := range s.tools {
			list = append(list, map[string]any{"name": t.Name, "description": t.Description, "inputSchema": t.InputSchema})
		}
		s.result(req.ID, map[string]any{"tools": list})
	case "tools/call":
		p, err := decode[struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
			Meta      struct {
				ProgressToken any `json:"progressToken"`
			} `json:"_meta"`
		}](req.Params)
		if err != nil {
			s.fail(req.ID, -32602, err.Error())
			return
		}
		tool, ok := s.byName[p.Name]
		if !ok {
			s.fail(req.ID, -32602, "unknown tool: "+p.Name)
			return
		}
		c := &Call{Env: env}
		if tok := p.Meta.ProgressToken; tok != nil {
			c.progress = func(n int, msg string) {
				s.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress",
					"params": map[string]any{"progressToken": tok, "progress": n, "message": msg}})
			}
		}
		out, err := tool.Handler(c, p.Arguments)
		if err != nil {
			s.result(req.ID, textResult(err.Error(), true))
			return
		}
		if text, ok := out.(string); ok {
			s.result(req.ID, textResult(text, false))
			return
		}
		data, err := json.Marshal(out)
		if err != nil {
			s.result(req.ID, textResult(err.Error(), true))
			return
		}
		s.result(req.ID, textResult(string(data), false))
	default:
		s.fail(req.ID, -32601, "method not found: "+req.Method)
	}
}
```

- [ ] **Step 5: Write `tools.go` and `main.go`**

`plugins/bruh/mcp/tools.go`:

```go
package main

// AllTools lists every tool of the server. Tasks 4 to 7 add their tools here.
func AllTools() []Tool {
	var tools []Tool
	return tools
}
```

`plugins/bruh/mcp/main.go`:

```go
package main

import (
	"log"
	"os"
)

func main() {
	srv := NewServer("bruh", "0.1.0-dev", AllTools())
	if err := srv.Serve(EnvFromOS(), os.Stdin, os.Stdout); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run: `cd plugins/bruh/mcp && gofmt -l . && go vet ./... && go test -race ./...`
Expected: no `gofmt` output, then `ok`. `TestMissingRoleKeyWritesNothing` passes with zero tools; Tasks 4 to 7 give it real work.

- [ ] **Step 7: Commit**

```bash
git add plugins/bruh/mcp
git commit -m "Add the MCP server core: Env, file store, and JSON-RPC over stdio"
```

---

### Task 4: Mailbox and handoff tools

**Files:**

- Create: `plugins/bruh/mcp/mail.go`
- Create: `plugins/bruh/mcp/handoff.go`
- Modify: `plugins/bruh/mcp/tools.go`
- Create: `plugins/bruh/mcp/mail_test.go`
- Create: `plugins/bruh/mcp/handoff_test.go`

**Interfaces:**

- Consumes: `Env`, `checkKey`, `atomicWrite`, `decode` from Task 3.
- Produces:
  - `mailTools() []Tool`: `mail_post({to, header, body})` → `{"id", "at"}`, file `mail/<to>/<id>.json` = `Message`; `mail_read({})` → `[]Message` for the caller, oldest first, moved to `mail/<caller>/read/`.
  - `type Message struct { ID, From, To, Header, Body, At string }` with JSON names `id`, `from`, `to`, `header`, `body`, `at`.
  - `headerRE` with the pattern `^(P[012] Q-\d+|ANSWER Q-\d+|REC Q-\d+|RULE R-\d+|DONE): \S.{0,199}$`.
  - `handoffTools() []Tool`: `handoff_write({text})` → `{"at", "chars"}`, file `handoffs/<caller>.md`, the old version appended to `handoffs/<caller>.history.md`; `handoff_read({role_key})` → text or `""`; an empty `role_key` means the caller.
  - `const handoffMax = 8000`, `validateHandoff(text string) error`.

- [ ] **Step 1: Write the failing tests**

`plugins/bruh/mcp/mail_test.go`:

```go
package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestMailKeepsOrderAndArchives(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	for n := range 3 {
		if _, err := call(t, env, "mail_post", map[string]any{
			"to": "clanker-a", "header": fmt.Sprintf("P2 Q-%d: question %d", n+1, n+1), "body": "b",
		}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := call(t, as(env, "clanker-a"), "mail_read", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	msgs := got.([]any)
	if len(msgs) != 3 {
		t.Fatalf("got %d messages", len(msgs))
	}
	for i, m := range msgs {
		if h := m.(map[string]any)["header"]; h != fmt.Sprintf("P2 Q-%d: question %d", i+1, i+1) {
			t.Fatalf("message %d header %v", i, h)
		}
	}
	if from := msgs[0].(map[string]any)["from"]; from != "clerk-a-1" {
		t.Fatalf("from = %v", from)
	}
	again, _ := call(t, as(env, "clanker-a"), "mail_read", map[string]any{})
	if len(again.([]any)) != 0 {
		t.Fatal("read messages were not archived")
	}
}

func TestMailRefusesBadInput(t *testing.T) {
	env := testEnv(t, "bigm")
	if _, err := call(t, env, "mail_post", map[string]any{"to": "clanker-a", "header": "STATUS: x", "body": ""}); err == nil || !strings.Contains(err.Error(), "invalid header") {
		t.Fatalf("err = %v", err)
	}
	if _, err := call(t, env, "mail_post", map[string]any{"to": "../x", "header": "DONE: x", "body": ""}); err == nil || !strings.Contains(err.Error(), "invalid role key") {
		t.Fatalf("err = %v", err)
	}
}
```

`plugins/bruh/mcp/handoff_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const goodHandoff = "## Goal\n- Ship the login fix\n## State\n- Tests pass on branch fix-login\n"

func TestHandoffReplacesAndKeepsHistory(t *testing.T) {
	env := testEnv(t, "clanker-a")
	if _, err := call(t, env, "handoff_write", map[string]any{"text": goodHandoff}); err != nil {
		t.Fatal(err)
	}
	if _, err := call(t, env, "handoff_write", map[string]any{"text": strings.Replace(goodHandoff, "login", "logout", 1)}); err != nil {
		t.Fatal(err)
	}
	now, _ := call(t, env, "handoff_read", map[string]any{})
	text := now.(string)
	if !strings.Contains(text, "logout") || strings.Contains(text, "Ship the login fix") {
		t.Fatalf("current = %q", text)
	}
	if !regexp.MustCompile(`^Updated: \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z`).MatchString(text) {
		t.Fatalf("no stamp: %q", text)
	}
	hist, err := os.ReadFile(filepath.Join(env.DataDir, "handoffs", "clanker-a.history.md"))
	if err != nil || !strings.Contains(string(hist), "login fix") {
		t.Fatalf("history = %q, %v", hist, err)
	}
}

func TestHandoffRefusals(t *testing.T) {
	env := testEnv(t, "clanker-a")
	if _, err := call(t, env, "handoff_write", map[string]any{"text": goodHandoff}); err != nil {
		t.Fatal(err)
	}
	cases := []struct{ text, want string }{
		{strings.Repeat("- item\n", 1400), "over 8000 characters"},
		{"## Files\n- `docs/plan.md`\n- src/app.js\n", "only file pointers"},
		{"## State\n- Waiting on agent a55bfb00ce43c38de\n", "subagent ID"},
		{"## State\n- Notes in /tmp/notes.md\n", "/tmp path"},
	}
	for _, c := range cases {
		if _, err := call(t, env, "handoff_write", map[string]any{"text": c.text}); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, err = %v", c.want, err)
		}
	}
	now, _ := call(t, env, "handoff_read", map[string]any{})
	if !strings.Contains(now.(string), "Ship the login fix") {
		t.Fatal("a refused write changed the file")
	}
}

func TestHandoffReadEmpty(t *testing.T) {
	got, err := call(t, testEnv(t, "bigm"), "handoff_read", map[string]any{"role_key": "clanker-zzz"})
	if err != nil || got != "" {
		t.Fatalf("got %q, %v", got, err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: FAIL with `unknown tool mail_post`.

- [ ] **Step 3: Write `mail.go`**

```go
package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

var headerRE = regexp.MustCompile(`^(P[012] Q-\d+|ANSWER Q-\d+|REC Q-\d+|RULE R-\d+|DONE): \S.{0,199}$`)

var mailSeq atomic.Int64

type Message struct {
	ID     string `json:"id"`
	From   string `json:"from"`
	To     string `json:"to"`
	Header string `json:"header"`
	Body   string `json:"body"`
	At     string `json:"at"`
}

func mailTools() []Tool {
	return []Tool{
		{
			Name:        "mail_post",
			Description: "Put a message in the durable mailbox of a role. Send only the header line as the SendMessage nudge.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"to":     map[string]any{"type": "string", "description": "Role key of the receiver"},
					"header": map[string]any{"type": "string", "description": `One header line, for example "P1 Q-7: merge the login fix?"`},
					"body":   map[string]any{"type": "string"},
				},
				"required": []string{"to", "header", "body"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				from, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ To, Header, Body string }](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.To, "role key"); err != nil {
					return nil, err
				}
				if !headerRE.MatchString(a.Header) {
					return nil, fmt.Errorf("invalid header: %q", a.Header)
				}
				box, err := c.Env.Dir("mail", a.To)
				if err != nil {
					return nil, err
				}
				msg := Message{
					ID:     fmt.Sprintf("%015d-%06d-%s", time.Now().UnixMilli(), mailSeq.Add(1), strings.ToLower(rand.Text()[:8])),
					From:   from,
					To:     a.To,
					Header: a.Header,
					Body:   a.Body,
					At:     c.Env.Stamp(),
				}
				data, _ := json.Marshal(msg)
				if err := atomicWrite(filepath.Join(box, msg.ID+".json"), data); err != nil {
					return nil, err
				}
				return map[string]string{"id": msg.ID, "at": msg.At}, nil
			},
		},
		{
			Name:        "mail_read",
			Description: "Read and archive all new messages for the calling role, oldest first.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				box, err := c.Env.Dir("mail", me)
				if err != nil {
					return nil, err
				}
				readDir, err := c.Env.Dir("mail", me, "read")
				if err != nil {
					return nil, err
				}
				entries, err := os.ReadDir(box) // sorted by file name
				if err != nil {
					return nil, err
				}
				msgs := []Message{}
				for _, e := range entries {
					if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
						continue
					}
					data, err := os.ReadFile(filepath.Join(box, e.Name()))
					if err != nil {
						return nil, err
					}
					var m Message
					if err := json.Unmarshal(data, &m); err != nil {
						return nil, errors.Join(fmt.Errorf("bad message %s", e.Name()), err)
					}
					if err := os.Rename(filepath.Join(box, e.Name()), filepath.Join(readDir, e.Name())); err != nil {
						return nil, err
					}
					msgs = append(msgs, m)
				}
				return msgs, nil
			},
		},
	}
}
```

- [ ] **Step 4: Write `handoff.go`**

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"
)

const handoffMax = 8000

var (
	itemRE     = regexp.MustCompile(`^\s*(?:[-*]|\d+\.)\s+(.*)$`)
	pointerRE  = regexp.MustCompile("^`?[\\w./~-]+`?$")
	subagentRE = regexp.MustCompile(`\ba[0-9a-f]{16}\b`)
	tmpRE      = regexp.MustCompile("(^|[\\s`'\"(])/(?:private/)?tmp/")
)

func validateHandoff(text string) error {
	if text == "" {
		return errors.New("handoff is empty")
	}
	if n := utf8.RuneCountInString(text); n > handoffMax {
		return fmt.Errorf("handoff is %d characters, over 8000 characters", n)
	}
	if subagentRE.MatchString(text) {
		return errors.New("handoff names a subagent ID; a new session cannot reach it")
	}
	if tmpRE.MatchString(text) {
		return errors.New("handoff names a /tmp path; a reboot deletes it")
	}
	items, pointers := 0, 0
	for line := range strings.SplitSeq(text, "\n") {
		m := itemRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		items++
		if pointerRE.MatchString(strings.TrimSpace(m[1])) {
			pointers++
		}
	}
	if items == 0 || items == pointers {
		return errors.New("handoff has only file pointers; write the items themselves")
	}
	return nil
}

func handoffTools() []Tool {
	return []Tool{
		{
			Name:        "handoff_write",
			Description: "Replace the handoff of the calling role with one current state. Sections: role, standing owner rules word for word, goal, state, decisions, waiting on the owner, waiting on others, next steps, files to read.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"text": map[string]any{"type": "string"}},
				"required":   []string{"text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct{ Text string }](raw)
				if err != nil {
					return nil, err
				}
				if err := validateHandoff(a.Text); err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("handoffs")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, me+".md")
				if old, err := os.ReadFile(file); err == nil {
					h, err := os.OpenFile(filepath.Join(dir, me+".history.md"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
					if err != nil {
						return nil, err
					}
					_, werr := fmt.Fprintf(h, "%s\n\n---\n\n", old)
					if err := errors.Join(werr, h.Close()); err != nil {
						return nil, err
					}
				} else if !errors.Is(err, fs.ErrNotExist) {
					return nil, err
				}
				at := c.Env.Stamp()
				stamped := "Updated: " + at + "\n\n" + a.Text
				if err := atomicWrite(file, []byte(stamped)); err != nil {
					return nil, err
				}
				return map[string]any{"at": at, "chars": utf8.RuneCountInString(stamped)}, nil
			},
		},
		{
			Name:        "handoff_read",
			Description: "Read the current handoff of a role. Empty when none exists.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"role_key": map[string]any{"type": "string"}}},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
				}](raw)
				if err != nil {
					return nil, err
				}
				key := me
				if a.RoleKey != "" {
					if key, err = checkKey(a.RoleKey, "role key"); err != nil {
						return nil, err
					}
				}
				dir, err := c.Env.Dir("handoffs")
				if err != nil {
					return nil, err
				}
				data, err := os.ReadFile(filepath.Join(dir, key+".md"))
				if errors.Is(err, fs.ErrNotExist) {
					return "", nil
				}
				return string(data), err
			},
		},
	}
}
```

- [ ] **Step 5: Register the tools**

`plugins/bruh/mcp/tools.go`:

```go
package main

// AllTools lists every tool of the server.
func AllTools() []Tool {
	var tools []Tool
	tools = append(tools, mailTools()...)
	tools = append(tools, handoffTools()...)
	return tools
}
```

- [ ] **Step 6: Run the tests to verify that they pass**

Run: `cd plugins/bruh/mcp && gofmt -l . && go vet ./... && go test -race ./...`
Expected: `ok`. `TestMissingRoleKeyWritesNothing` now checks four tools.

- [ ] **Step 7: Commit**

```bash
git add plugins/bruh/mcp
git commit -m "Add the mailbox and handoff tools of the MCP server"
```

---

### Task 5: Answer tools with progress and deadline

**Files:**

- Create: `plugins/bruh/mcp/answer.go`
- Modify: `plugins/bruh/mcp/tools.go`
- Create: `plugins/bruh/mcp/answer_test.go`

**Interfaces:**

- Consumes: `Env`, `checkID`, `atomicWrite`, `decode`, `(*Call).Progress`.
- Produces `answerTools() []Tool`:
  - `answer_write({question_id, text})` → `{"at"}`. File `answers/<caller>/<question_id>.answer` = `{"text", "at"}`. `question_id` matches `^Q-\d+$`.
  - `answer_wait({question_id, deadline_seconds})` → `{"status":"answered","text","at"}` or `{"status":"pending"}`. It sends a progress notification at the start and every `Env.ProgressInterval`, and polls every `Env.PollInterval`.

The caller of both tools is the clerk session: a workflow agent runs inside the clerk session, so its MCP server has the role key of the clerk.

- [ ] **Step 1: Write the failing test**

`plugins/bruh/mcp/answer_test.go`:

```go
package main

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAnswerWaitGetsAnswerWithProgress(t *testing.T) {
	env := testEnv(t, "clerk-a-1")
	var wait func(*Call, json.RawMessage) (any, error)
	for _, tool := range AllTools() {
		if tool.Name == "answer_wait" {
			wait = tool.Handler
		}
	}
	if wait == nil {
		t.Fatal("no answer_wait tool")
	}
	var mu sync.Mutex
	progress := 0
	c := &Call{Env: env, progress: func(int, string) { mu.Lock(); progress++; mu.Unlock() }}
	done := make(chan any, 1)
	go func() {
		out, err := wait(c, json.RawMessage(`{"question_id":"Q-7","deadline_seconds":5}`))
		if err != nil {
			t.Error(err)
		}
		done <- out
	}()
	time.Sleep(150 * time.Millisecond)
	if _, err := call(t, env, "answer_write", map[string]any{"question_id": "Q-7", "text": "Use option B. Owner, 2026-09-30."}); err != nil {
		t.Fatal(err)
	}
	out := (<-done).(map[string]string)
	if out["status"] != "answered" || out["text"] != "Use option B. Owner, 2026-09-30." {
		t.Fatalf("out = %v", out)
	}
	mu.Lock()
	defer mu.Unlock()
	if progress < 2 {
		t.Fatalf("progress = %d", progress)
	}
}

func TestAnswerWaitPendingAtDeadline(t *testing.T) {
	out, err := call(t, testEnv(t, "clerk-a-1"), "answer_wait", map[string]any{"question_id": "Q-8", "deadline_seconds": 0.2})
	if err != nil || out.(map[string]any)["status"] != "pending" {
		t.Fatalf("out = %v, %v", out, err)
	}
}

func TestAnswerRefusesBadID(t *testing.T) {
	_, err := call(t, testEnv(t, "clerk-a-1"), "answer_write", map[string]any{"question_id": "../Q-1", "text": "x"})
	if err == nil || !strings.Contains(err.Error(), "invalid question ID") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: FAIL with `no answer_wait tool`.

- [ ] **Step 3: Write `answer.go`**

```go
package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

var qidRE = regexp.MustCompile(`^Q-\d+$`)

type answer struct {
	Text string `json:"text"`
	At   string `json:"at"`
}

func answerFile(env Env, qid string) (string, error) {
	me, err := env.Caller()
	if err != nil {
		return "", err
	}
	if _, err := checkID(qid, qidRE, "question ID"); err != nil {
		return "", err
	}
	dir, err := env.Dir("answers", me)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, qid+".answer"), nil
}

func answerTools() []Tool {
	return []Tool{
		{
			Name:        "answer_write",
			Description: "Write the answer to a question of a workflow agent of this clerk.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question_id": map[string]any{"type": "string"},
					"text":        map[string]any{"type": "string"},
				},
				"required": []string{"question_id", "text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					QuestionID string `json:"question_id"`
					Text       string `json:"text"`
				}](raw)
				if err != nil {
					return nil, err
				}
				file, err := answerFile(c.Env, a.QuestionID)
				if err != nil {
					return nil, err
				}
				at := c.Env.Stamp()
				data, _ := json.Marshal(answer{Text: a.Text, At: at})
				return map[string]string{"at": at}, atomicWrite(file, data)
			},
		},
		{
			Name:        "answer_wait",
			Description: `Wait for the answer to a question. Returns {"status":"answered","text"} or {"status":"pending"} at the deadline.`,
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question_id":      map[string]any{"type": "string"},
					"deadline_seconds": map[string]any{"type": "number"},
				},
				"required": []string{"question_id", "deadline_seconds"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					QuestionID      string  `json:"question_id"`
					DeadlineSeconds float64 `json:"deadline_seconds"`
				}](raw)
				if err != nil {
					return nil, err
				}
				file, err := answerFile(c.Env, a.QuestionID)
				if err != nil {
					return nil, err
				}
				end := time.Now().Add(time.Duration(a.DeadlineSeconds * float64(time.Second)))
				n := 1
				c.Progress(n, "waiting for "+a.QuestionID)
				lastProgress := time.Now()
				for {
					data, err := os.ReadFile(file)
					if err == nil {
						var ans answer
						if err := json.Unmarshal(data, &ans); err != nil {
							return nil, err
						}
						return map[string]string{"status": "answered", "text": ans.Text, "at": ans.At}, nil
					}
					if !errors.Is(err, fs.ErrNotExist) {
						return nil, err
					}
					if !time.Now().Before(end) {
						return map[string]string{"status": "pending"}, nil
					}
					if time.Since(lastProgress) >= c.Env.ProgressInterval {
						n++
						c.Progress(n, "waiting for "+a.QuestionID)
						lastProgress = time.Now()
					}
					time.Sleep(min(c.Env.PollInterval, max(time.Millisecond, time.Until(end))))
				}
			},
		},
	}
}
```

- [ ] **Step 4: Register the tools**

In `plugins/bruh/mcp/tools.go`, add `tools = append(tools, answerTools()...)` before `return tools`.

- [ ] **Step 5: Run the tests to verify that they pass**

Run: `cd plugins/bruh/mcp && gofmt -l . && go vet ./... && go test -race ./...`
Expected: `ok`.

- [ ] **Step 6: Run probe P8**

Follow `probes/README.md`, P8, and add its row to the probe table in `docs/knowledge.md`.

- [ ] **Step 7: Commit**

```bash
git add plugins/bruh/mcp docs/knowledge.md
git commit -m "Add answer_write and answer_wait with progress and a deadline"
```

---

### Task 6: Report and role settings tools

**Files:**

- Create: `plugins/bruh/mcp/report.go`
- Create: `plugins/bruh/mcp/roles.go`
- Modify: `plugins/bruh/mcp/tools.go`
- Create: `plugins/bruh/mcp/report_test.go`
- Create: `plugins/bruh/mcp/roles_test.go`

**Interfaces:**

- Consumes: `Env`, `checkKey`, `atomicWrite`, `decode`, `plugins/bruh/defaults/role-settings.json` (read from `Env.PluginRoot`).
- Produces:
  - `reportTools() []Tool`: `report_write({kind, text, source})` → `{"at"}`. `kind` is one of `status`, `answer`, `event`, `result`. `source` is optional: `{call, value, at}` (spec principle 1). Appends one JSON line to `reports/<caller>.jsonl`. `report_read({role_key, since})` → the lines of that role with `at` after `since` (optional).
  - `type ReportLine struct { At, From, Kind, Text string; Source *Source }` with JSON names `at`, `from`, `kind`, `text`, `source` (`omitempty`).
  - `rolesTools() []Tool`: `role_settings_write({role_key, env, deny})` → `{"path"}`. Writes `roles/<role_key>.json` = the defaults, plus `env.BRUH_ROLE_KEY = role_key`, plus extra `env` values, plus extra `deny` rules. The path is absolute, for `--settings`.

- [ ] **Step 1: Write the failing tests**

`plugins/bruh/mcp/report_test.go`:

```go
package main

import (
	"strings"
	"testing"
	"time"
)

func TestReportWriteAndRead(t *testing.T) {
	env := testEnv(t, "clanker-a")
	first, err := call(t, env, "report_write", map[string]any{"kind": "status", "text": "task 1 started"})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	at := first.(map[string]any)["at"].(string)
	if _, err := call(t, env, "report_write", map[string]any{"kind": "result", "text": "merged",
		"source": map[string]any{"call": "gh pr view 3 --json state", "value": "MERGED", "at": at}}); err != nil {
		t.Fatal(err)
	}
	all, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clanker-a"})
	lines := all.([]any)
	if len(lines) != 2 || lines[1].(map[string]any)["source"].(map[string]any)["value"] != "MERGED" {
		t.Fatalf("lines = %v", lines)
	}
	later, _ := call(t, as(env, "bigm"), "report_read", map[string]any{"role_key": "clanker-a", "since": at})
	if len(later.([]any)) != 1 {
		t.Fatalf("later = %v", later)
	}
	if _, err := call(t, env, "report_write", map[string]any{"kind": "gossip", "text": "x"}); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Fatalf("err = %v", err)
	}
}
```

`plugins/bruh/mcp/roles_test.go`:

```go
package main

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestRoleSettingsWrite(t *testing.T) {
	env := testEnv(t, "bigm")
	out, err := call(t, env, "role_settings_write", map[string]any{
		"role_key": "clanker-a",
		"env":      map[string]string{"CODEX_HOME": "/opt/codex"},
		"deny":     []string{"Bash(rm -rf /:*)"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(out.(map[string]any)["path"].(string))
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Env         map[string]string `json:"env"`
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &s); err != nil {
		t.Fatal(err)
	}
	if s.Env["BRUH_ROLE_KEY"] != "clanker-a" || s.Env["CODEX_HOME"] != "/opt/codex" {
		t.Fatalf("env = %v", s.Env)
	}
	if !slices.Contains(s.Permissions.Deny, "Bash(docker volume rm:*)") || !slices.Contains(s.Permissions.Deny, "Bash(rm -rf /:*)") {
		t.Fatalf("deny = %v", s.Permissions.Deny)
	}
	_, err = call(t, env, "role_settings_write", map[string]any{"role_key": "x", "env": map[string]string{"BRUH_ROLE_KEY": "bigm"}})
	if err == nil || !strings.Contains(err.Error(), "BRUH_ROLE_KEY") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify that they fail**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: FAIL with `unknown tool report_write`.

- [ ] **Step 3: Write `report.go`**

```go
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

var reportKinds = []string{"status", "answer", "event", "result"}

type Source struct {
	Call  string `json:"call"`
	Value string `json:"value"`
	At    string `json:"at"`
}

type ReportLine struct {
	At     string  `json:"at"`
	From   string  `json:"from"`
	Kind   string  `json:"kind"`
	Text   string  `json:"text"`
	Source *Source `json:"source,omitempty"`
}

func reportTools() []Tool {
	return []Tool{
		{
			Name:        "report_write",
			Description: "Append one line to the report file of the calling role. A status claim (merged, deployed, live, down, out of quota) carries source: {call, value, at}.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": reportKinds},
					"text": map[string]any{"type": "string"},
					"source": map[string]any{"type": "object", "properties": map[string]any{
						"call": map[string]any{"type": "string"}, "value": map[string]any{"type": "string"}, "at": map[string]any{"type": "string"},
					}},
				},
				"required": []string{"kind", "text"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Kind   string  `json:"kind"`
					Text   string  `json:"text"`
					Source *Source `json:"source"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if !slices.Contains(reportKinds, a.Kind) {
					return nil, fmt.Errorf("invalid kind: %q", a.Kind)
				}
				dir, err := c.Env.Dir("reports")
				if err != nil {
					return nil, err
				}
				line := ReportLine{At: c.Env.Stamp(), From: me, Kind: a.Kind, Text: a.Text, Source: a.Source}
				data, _ := json.Marshal(line)
				f, err := os.OpenFile(filepath.Join(dir, me+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
				if err != nil {
					return nil, err
				}
				_, werr := f.Write(append(data, '\n'))
				return map[string]string{"at": line.At}, errors.Join(werr, f.Close())
			},
		},
		{
			Name:        "report_read",
			Description: "Read the report lines of a role, optionally only after a UTC time.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"role_key": map[string]any{"type": "string"},
					"since":    map[string]any{"type": "string"},
				},
				"required": []string{"role_key"},
			},
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				a, err := decode[struct {
					RoleKey string `json:"role_key"`
					Since   string `json:"since"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.RoleKey, "role key"); err != nil {
					return nil, err
				}
				dir, err := c.Env.Dir("reports")
				if err != nil {
					return nil, err
				}
				f, err := os.Open(filepath.Join(dir, a.RoleKey+".jsonl"))
				if errors.Is(err, fs.ErrNotExist) {
					return []ReportLine{}, nil
				}
				if err != nil {
					return nil, err
				}
				defer f.Close()
				lines := []ReportLine{}
				sc := bufio.NewScanner(f)
				sc.Buffer(make([]byte, 1<<20), 16<<20)
				for sc.Scan() {
					var l ReportLine
					if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
						return nil, err
					}
					if a.Since == "" || l.At > a.Since {
						lines = append(lines, l)
					}
				}
				return lines, sc.Err()
			},
		},
	}
}
```

- [ ] **Step 4: Write `roles.go`**

```go
package main

import (
	"encoding/json"
	"errors"
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
				if _, err := c.Env.Caller(); err != nil {
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
				key, err := checkKey(a.RoleKey, "role key")
				if err != nil {
					return nil, err
				}
				if _, ok := a.Env["BRUH_ROLE_KEY"]; ok {
					return nil, errors.New("env must not set BRUH_ROLE_KEY; role_key sets it")
				}
				data, err := os.ReadFile(filepath.Join(c.Env.PluginRoot, "defaults", "role-settings.json"))
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
				for k, v := range a.Env {
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
				for _, d := range a.Deny {
					if !slices.Contains(deny, d) {
						deny = append(deny, d)
					}
				}
				perms["deny"] = deny
				settings["permissions"] = perms
				dir, err := c.Env.Dir("roles")
				if err != nil {
					return nil, err
				}
				file := filepath.Join(dir, key+".json")
				out, _ := json.MarshalIndent(settings, "", "  ")
				if err := atomicWrite(file, out); err != nil {
					return nil, err
				}
				abs, err := filepath.Abs(file)
				return map[string]string{"path": abs}, err
			},
		},
	}
}
```

`maps.Copy(env, a.Env)` does not compile here, because `env` is `map[string]any` and `a.Env` is `map[string]string`.

- [ ] **Step 5: Register the tools**

In `plugins/bruh/mcp/tools.go`, add `tools = append(tools, reportTools()...)` and `tools = append(tools, rolesTools()...)` before `return tools`.

- [ ] **Step 6: Run the tests to verify that they pass**

Run: `cd plugins/bruh/mcp && gofmt -l . && go vet ./... && go test -race ./...`
Expected: `ok`.

- [ ] **Step 7: Commit**

```bash
git add plugins/bruh/mcp
git commit -m "Add the report and role settings tools"
```

---

### Task 7: Lease tools with a lock

**Files:**

- Create: `plugins/bruh/mcp/lease.go`
- Modify: `plugins/bruh/mcp/tools.go`
- Create: `plugins/bruh/mcp/lease_test.go`

**Interfaces:**

- Consumes: `Env`, `(Env).WithLock`, `checkKey`, `checkID`, `atomicWrite`, `decode`, `stampLayout`.
- Produces `leaseTools() []Tool` and the state file `leases/state.json`, which `lease-guard.sh` (Plan 2) reads with `jq`:

  ```json
  {
    "resources": {
      "staging": {
        "capacity": 1,
        "grants": [
          { "holder": "clanker-a", "grantor": "bigm", "until": "2026-09-30T14:00:00.000Z" },
          { "holder": "clerk-a-3", "grantor": "clanker-a", "until": "2026-09-30T13:30:00.000Z" }
        ]
      }
    },
    "requests": [ { "resource": "staging", "requester": "clanker-b", "at": "2026-09-30T12:00:00.000Z" } ]
  }
  ```

  - `lease_define({resource, capacity})`: only `bigm`.
  - `lease_request({resource})`: adds a request; the grantor sees it in `lease_list`.
  - `lease_grant({resource, to, minutes})`:
    - caller `bigm`: `to` starts with `clanker-`; active grants of `bigm` for the resource are below `capacity`.
    - caller `clanker-<p>`: `to` starts with `clerk-<p>-`; the caller holds an active grant from `bigm`; the caller has no other active sub-grant for the resource; the sub-grant ends no later than the grant of the caller.
    - any other caller: refused.
  - `lease_release({resource, holder})`: the caller is the holder or its grantor. Releasing a clanker grant also releases its sub-grants.
  - `lease_list({})`: the state without expired grants.

- [ ] **Step 1: Write the failing test**

`plugins/bruh/mcp/lease_test.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func mustErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("want error %q, got %v", want, err)
	}
}

func TestLeaseHierarchy(t *testing.T) {
	bigm := testEnv(t, "bigm")
	ca, cb := as(bigm, "clanker-a"), as(bigm, "clanker-b")
	g := func(env Env, to string) error {
		_, err := call(t, env, "lease_grant", map[string]any{"resource": "staging", "to": to, "minutes": 30})
		return err
	}
	_, err := call(t, ca, "lease_define", map[string]any{"resource": "staging", "capacity": 1})
	mustErr(t, err, "only bigm")
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "staging", "capacity": 1}); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(ca, "clerk-a-1"), "holds no grant")
	if err := g(bigm, "clanker-a"); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(bigm, "clanker-b"), "capacity")
	mustErr(t, g(ca, "clerk-b-1"), "own clerks")
	if err := g(ca, "clerk-a-1"); err != nil {
		t.Fatal(err)
	}
	mustErr(t, g(ca, "clerk-a-2"), "one sub-grant")
	_, err = call(t, cb, "lease_release", map[string]any{"resource": "staging", "holder": "clanker-a"})
	mustErr(t, err, "holder or its grantor")
	if _, err := call(t, bigm, "lease_release", map[string]any{"resource": "staging", "holder": "clanker-a"}); err != nil {
		t.Fatal(err)
	}
	state, _ := call(t, bigm, "lease_list", map[string]any{})
	grants := state.(map[string]any)["resources"].(map[string]any)["staging"].(map[string]any)["grants"].([]any)
	if len(grants) != 0 {
		t.Fatalf("grants = %v", grants)
	}
}

func TestLeaseParallelGrantsKeepCount(t *testing.T) {
	bigm := testEnv(t, "bigm")
	if _, err := call(t, bigm, "lease_define", map[string]any{"resource": "runner", "capacity": 10}); err != nil {
		t.Fatal(err)
	}
	var ok atomic.Int64
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Go(func() {
			if _, err := call(t, bigm, "lease_grant", map[string]any{"resource": "runner", "to": fmt.Sprintf("clanker-p%d", i), "minutes": 5}); err == nil {
				ok.Add(1)
			}
		})
	}
	wg.Wait()
	if ok.Load() != 10 {
		t.Fatalf("granted %d, want 10", ok.Load())
	}
	data, err := os.ReadFile(filepath.Join(bigm.DataDir, "leases", "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var st struct {
		Resources map[string]struct {
			Grants []any `json:"grants"`
		} `json:"resources"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatal(err)
	}
	if n := len(st.Resources["runner"].Grants); n != 10 {
		t.Fatalf("state has %d grants", n)
	}
}
```

- [ ] **Step 2: Run the test to verify that it fails**

Run: `cd plugins/bruh/mcp && go test ./...`
Expected: FAIL with `unknown tool lease_define`.

- [ ] **Step 3: Write `lease.go`**

```go
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

var resourceRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type Grant struct {
	Holder  string `json:"holder"`
	Grantor string `json:"grantor"`
	Until   string `json:"until"`
}

type Resource struct {
	Capacity int     `json:"capacity"`
	Grants   []Grant `json:"grants"`
}

type LeaseRequest struct {
	Resource  string `json:"resource"`
	Requester string `json:"requester"`
	At        string `json:"at"`
}

type LeaseState struct {
	Resources map[string]*Resource `json:"resources"`
	Requests  []LeaseRequest       `json:"requests"`
}

func leaseFile(env Env) (string, error) {
	dir, err := env.Dir("leases")
	return filepath.Join(dir, "state.json"), err
}

func loadLeases(env Env) (*LeaseState, error) {
	st := &LeaseState{Resources: map[string]*Resource{}, Requests: []LeaseRequest{}}
	file, err := leaseFile(env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(file)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal(data, st); err != nil {
			return nil, err
		}
	}
	now := env.Stamp()
	for _, r := range st.Resources {
		r.Grants = slices.DeleteFunc(r.Grants, func(g Grant) bool { return g.Until <= now })
		if r.Grants == nil {
			r.Grants = []Grant{}
		}
	}
	return st, nil
}

// withLeases loads the state under the lock, runs fn, and saves the state when fn succeeds.
func withLeases(env Env, fn func(st *LeaseState) (any, error)) (any, error) {
	var out any
	err := env.WithLock("leases", func() error {
		st, err := loadLeases(env)
		if err != nil {
			return err
		}
		if out, err = fn(st); err != nil {
			return err
		}
		file, err := leaseFile(env)
		if err != nil {
			return err
		}
		data, _ := json.MarshalIndent(st, "", "  ")
		return atomicWrite(file, data)
	})
	return out, err
}

func resourceOf(st *LeaseState, name string) (*Resource, error) {
	if _, err := checkID(name, resourceRE, "resource"); err != nil {
		return nil, err
	}
	r, ok := st.Resources[name]
	if !ok {
		return nil, fmt.Errorf("unknown resource: %s", name)
	}
	return r, nil
}

func objectSchema(props map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func stringSchema() map[string]any { return map[string]any{"type": "string"} }

func leaseTools() []Tool {
	return []Tool{
		{
			Name:        "lease_define",
			Description: "Define a shared resource and its capacity. Only bigm.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema(), "capacity": map[string]any{"type": "integer", "minimum": 1}}, "resource", "capacity"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				if me != "bigm" {
					return nil, errors.New("only bigm defines resources")
				}
				a, err := decode[struct {
					Resource string `json:"resource"`
					Capacity int    `json:"capacity"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkID(a.Resource, resourceRE, "resource"); err != nil {
					return nil, err
				}
				if a.Capacity < 1 {
					return nil, errors.New("capacity must be 1 or more")
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					grants := []Grant{}
					if old, ok := st.Resources[a.Resource]; ok {
						grants = old.Grants
					}
					st.Resources[a.Resource] = &Resource{Capacity: a.Capacity, Grants: grants}
					return map[string]any{"resource": a.Resource, "capacity": a.Capacity}, nil
				})
			},
		},
		{
			Name:        "lease_request",
			Description: "Ask your grantor for a lease on a resource. A clanker asks bigm; a clerk asks its clanker.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema()}, "resource"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string `json:"resource"`
				}](raw)
				if err != nil {
					return nil, err
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					if _, err := resourceOf(st, a.Resource); err != nil {
						return nil, err
					}
					at := c.Env.Stamp()
					st.Requests = append(st.Requests, LeaseRequest{Resource: a.Resource, Requester: me, At: at})
					return map[string]string{"at": at}, nil
				})
			},
		},
		{
			Name:        "lease_grant",
			Description: "Grant a lease. bigm grants to clankers; a clanker grants one sub-lease at a time to its own clerks.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema(), "to": stringSchema(), "minutes": map[string]any{"type": "number", "minimum": 1}}, "resource", "to", "minutes"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string  `json:"resource"`
					To       string  `json:"to"`
					Minutes  float64 `json:"minutes"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.To, "role key"); err != nil {
					return nil, err
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					r, err := resourceOf(st, a.Resource)
					if err != nil {
						return nil, err
					}
					until := c.Env.Now().Add(time.Duration(a.Minutes * float64(time.Minute))).UTC().Format(stampLayout)
					switch {
					case me == "bigm":
						if !strings.HasPrefix(a.To, "clanker-") {
							return nil, errors.New("bigm grants only to clankers")
						}
						active := 0
						for _, g := range r.Grants {
							if g.Grantor == "bigm" {
								active++
							}
						}
						if active >= r.Capacity {
							return nil, fmt.Errorf("resource %s is at capacity %d", a.Resource, r.Capacity)
						}
					case strings.HasPrefix(me, "clanker-"):
						project := strings.TrimPrefix(me, "clanker-")
						if !strings.HasPrefix(a.To, "clerk-"+project+"-") {
							return nil, errors.New("a clanker grants only to its own clerks")
						}
						i := slices.IndexFunc(r.Grants, func(g Grant) bool { return g.Holder == me && g.Grantor == "bigm" })
						if i < 0 {
							return nil, fmt.Errorf("%s holds no grant on %s", me, a.Resource)
						}
						if slices.ContainsFunc(r.Grants, func(g Grant) bool { return g.Grantor == me }) {
							return nil, errors.New("a clanker has at most one sub-grant for each resource")
						}
						until = min(until, r.Grants[i].Until)
					default:
						return nil, fmt.Errorf("%s cannot grant leases", me)
					}
					r.Grants = append(r.Grants, Grant{Holder: a.To, Grantor: me, Until: until})
					st.Requests = slices.DeleteFunc(st.Requests, func(q LeaseRequest) bool {
						return q.Resource == a.Resource && q.Requester == a.To
					})
					return map[string]string{"resource": a.Resource, "holder": a.To, "until": until}, nil
				})
			},
		},
		{
			Name:        "lease_release",
			Description: "Release a lease. The holder or its grantor can release it. Releasing a clanker grant releases its sub-grants.",
			InputSchema: objectSchema(map[string]any{"resource": stringSchema(), "holder": stringSchema()}, "resource", "holder"),
			Handler: func(c *Call, raw json.RawMessage) (any, error) {
				me, err := c.Env.Caller()
				if err != nil {
					return nil, err
				}
				a, err := decode[struct {
					Resource string `json:"resource"`
					Holder   string `json:"holder"`
				}](raw)
				if err != nil {
					return nil, err
				}
				if _, err := checkKey(a.Holder, "role key"); err != nil {
					return nil, err
				}
				return withLeases(c.Env, func(st *LeaseState) (any, error) {
					r, err := resourceOf(st, a.Resource)
					if err != nil {
						return nil, err
					}
					i := slices.IndexFunc(r.Grants, func(g Grant) bool { return g.Holder == a.Holder })
					if i < 0 {
						return map[string]int{"released": 0}, nil
					}
					if me != a.Holder && me != r.Grants[i].Grantor {
						return nil, errors.New("only the holder or its grantor can release a lease")
					}
					before := len(r.Grants)
					r.Grants = slices.DeleteFunc(r.Grants, func(g Grant) bool { return g.Holder == a.Holder || g.Grantor == a.Holder })
					return map[string]int{"released": before - len(r.Grants)}, nil
				})
			},
		},
		{
			Name:        "lease_list",
			Description: "List resources, active grants, and open requests.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
			Handler: func(c *Call, _ json.RawMessage) (any, error) {
				if _, err := c.Env.Caller(); err != nil {
					return nil, err
				}
				return loadLeases(c.Env)
			},
		},
	}
}
```

- [ ] **Step 4: Register the tools**

The final `plugins/bruh/mcp/tools.go`:

```go
package main

// AllTools lists every tool of the server.
func AllTools() []Tool {
	var tools []Tool
	tools = append(tools, mailTools()...)
	tools = append(tools, handoffTools()...)
	tools = append(tools, answerTools()...)
	tools = append(tools, reportTools()...)
	tools = append(tools, rolesTools()...)
	tools = append(tools, leaseTools()...)
	return tools
}
```

- [ ] **Step 5: Run the tests to verify that they pass**

Run: `cd plugins/bruh/mcp && gofmt -l . && go vet ./... && go test -race -count=3 ./...`
Expected: `ok` on all three runs. `-count=3` repeats the parallel lease test.

- [ ] **Step 6: Commit**

```bash
git add plugins/bruh/mcp
git commit -m "Add hierarchical lease tools with a file lock"
```

---

### Task 8: End-to-end check in a real session

**Files:**

- Modify: `docs/knowledge.md` (one row in "Probe results (plan 1)")

- [ ] **Step 1: Load the plugin from the working tree**

Run, in a scratch folder (replace `<repo>` with the absolute path of the bruh working tree):

```bash
settings=$(mktemp); printf '{"env":{"BRUH_ROLE_KEY":"clanker-e2e"}}' > "$settings"
claude -p --plugin-dir <repo>/plugins/bruh --settings "$settings" --permission-mode auto \
  "Call the bruh MCP tool report_write with kind status and text e2e-ok. Then call report_read with role_key clanker-e2e and print the text of the first line."
```

Expected: the output contains `e2e-ok`. This proves that `go run -C` starts the server, that `BRUH_DATA` resolves to the plugin data folder, and that `BRUH_ROLE_KEY` from `--settings` reaches the MCP server (spec 10.1, Verify). The first start compiles the server and can take some seconds.

- [ ] **Step 2: Record the result**

Add a row `E2E | PASS or FAIL | <output line>` to the probe table in `docs/knowledge.md`.

- [ ] **Step 3: Commit**

```bash
git add docs/knowledge.md
git commit -m "Record the end-to-end check of the MCP server"
```

---

## Self-review

- **Spec coverage (this plan):** 10.1 MCP server and its tools (Tasks 3 to 7), 5 mailbox (Task 4), 6.2 answer path (Task 5), 7 handoff rules (Task 4), 8.4 lease tables (Task 7), 3.1 role settings file (Task 6), 19 license, CI, and plugin validation (Task 1), the Verify items of 3, 3.1, 4.1, 6.1, 6.2, 7, 8.4, 9.1, 10.1 (Tasks 2, 5, 8). The other spec sections are in Plans 2 to 6 of the index.
- **Placeholders:** none. Probe results come from real output in Task 2, Step 4.
- **Type consistency:** tool names, JSON field names, and `Env` methods match the Interfaces blocks. `leases/state.json` has the shape that Plan 2 reads. All times use `stampLayout`, so string comparison in `lease_*` and `report_read` is time comparison.
- **Review Focus:** each line has its test: concurrency (Task 7), missing role key (Task 3, checked for every tool), path characters (Tasks 4 and 5), handoff limits (Task 4), long waits (Task 5 and probe P8).
