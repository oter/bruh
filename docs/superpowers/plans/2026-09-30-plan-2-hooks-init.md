# Plan 2: Hooks, status line tap, session tools, and the init skill

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Before you edit any Go file, run the `modern-go-guidelines:use-modern-go` skill `list` command and follow its guidelines.

**Goal:** Give every role a stable, checked role key, the hooks that keep the context budget and the leases, the MCP tools that start and resume role sessions, and the `/bruh:init` skill that writes the user settings after the user approves a diff.

**Architecture:** The Go MCP server of plan 1 gets a role key parser (`env.go`), seven new tools, and a command dispatcher in `main.go` (no argument: MCP server; `init`: non-interactive init; plan 5 adds `watch` and `merge-train`). The hook scripts are POSIX `sh` with `jq`. Claude Code runs them in exec form (`sh <script>`), so no file needs an executable bit. The status line tap is copied into the plugin data folder by init, because a status line command cannot use `${CLAUDE_PLUGIN_DATA}`. Go tests run every script with fixture input.

**Tech Stack:** Go 1.26 or later (standard library only), POSIX `sh`, `jq`, JSON.

**Spec:** [../../spec.md](../../spec.md) version 0.4, sections 3.2, 7, 8.4, 10.1, 13, 16, 20. Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md) sections 1, 2, 3, 5, 6.

## Global Constraints

- Role keys follow interfaces section 1. Every hierarchy check uses `ParseRoleKey` and `Parent`, never a string prefix.
- A hook script does nothing and exits 0 when `BRUH_ROLE_KEY` is not set. The status line tap still runs the previous status line command in that case, so the status line of a manual session does not change.
- A hook script never fails a tool call by accident: a missing file, a missing folder, or bad JSON gives exit 0 and no output.
- Scripts use POSIX `sh`, `jq`, `date -u`, `mktemp`, and `mv`. No GNU-only flags.
- Tests never touch `~/.claude/settings.json` or the real `claude` binary. `BRUH_SETTINGS_FILE` and `BRUH_CLAUDE_BIN` point them at test files. `Env.Home` is a temporary folder in every test.
- `init_apply` writes only the content that `init_plan` planned, each file atomically, and refuses a plan when a target changed after `init_plan`.
- Timestamps come from code (`stampLayout` in Go, `date -u` in scripts).

## Review Focus

1. **A clanker grants a lease to a clerk of another project whose name starts with the same text** (`clanker-my` to `clerk-my-app-t1`). Expected: refused. Test in Task 1 (`TestLeaseGrantUsesParent`).
2. **init wraps a status line command that contains quotes, `&&`, or `$`.** Expected: the previous command runs unchanged with the same input, and the settings file keeps its other keys in their order. Tests in Task 4 (`TestTapRunsPreviousCommand`) and Task 6 (`TestInitPlanWrapsStatusLine`, `TestOrderedJSONRoundTrip`).
3. **The user edits `settings.json` between `init_plan` and `init_apply`.** Expected: `init_apply` refuses the stale plan and writes nothing. Test in Task 6 (`TestInitApplyRefusesStalePlan`).
4. **The handoff message repeats on every tool call above the threshold.** Expected: once for each crossing, again only after the value fell below the threshold. Test in Task 4 (`TestNudgeOncePerCrossing`).
5. **`session_launch` starts a second session with a name that a live session has, or for a role key that the caller does not own.** Expected: refused before `claude` runs. Test in Task 3 (`TestSessionLaunchRefusals`).

---

### Task 1: Role key grammar and hierarchy checks

**Files:**

- Modify: `plugins/bruh/mcp/env.go`, `plugins/bruh/mcp/lease.go`, `plugins/bruh/mcp/roles.go`
- Create: `plugins/bruh/mcp/env_test.go`
- Modify: `plugins/bruh/mcp/lease_test.go`, `plugins/bruh/mcp/roles_test.go`

**Interfaces:**

- Produces: `RoleKey`, `ParseRoleKey`, `RoleKey.String`, `RoleKey.Parent` exactly as interfaces section 1. `checkKey` parses with `ParseRoleKey`, so every tool refuses a role key outside the grammar.
- `lease_request` stores the grantor (`Parent` of the requester) in each request: `{"resource","requester","grantor","at"}`. bigm has no grantor, so bigm cannot request.
- `lease_grant`: bigm grants only to a key with role `clanker`; a clanker grants only to a clerk whose `Parent` is the clanker.
- `role_settings_write`: the caller must be `Parent` of the target key, or the caller is `bigm` and the target is `bigm`.

- [ ] **Step 1: Write the failing tests.** `TestParseRoleKey` (table: `bigm`, `clerk-ledger`, `clanker-my-app`, `clerk-my-app-t1`, `clerk-eng123-fix`; refused: empty, `Bigm`, `clanker-`, `clerk-a`, `clerk--x`, `clerk-a-b-`, `worker-a`, 65 characters, `../x`). `TestRoleKeyParent` for each role. `TestLeaseGrantUsesParent`: bigm grants `staging` to `clanker-my`; `clanker-my` grants to `clerk-my-app-t1` gives "own clerks"; bigm grants to `clerk-my-t1` gives "only to clankers". `TestRoleSettingsWriteParentOnly`: bigm writes `clanker-a`, `clerk-ledger`, and `bigm`; bigm cannot write `clerk-a-1`; `clanker-a` writes `clerk-a-1` but not `clerk-ab-1` or `clanker-b`; `clerk-a-1` writes nothing.
- [ ] **Step 2: Run** `go test ./...`. Expected: FAIL (undefined `ParseRoleKey`).
- [ ] **Step 3: Implement.** `ParseRoleKey`: length 1 to 64; `bigm`; `clerk-ledger`; `clanker-<project>` with `<project>` matching `^[a-z0-9]+(-[a-z0-9]+)*$`; `clerk-<rest>` where the last hyphen of `<rest>` splits project and task, the task matching `^[a-z0-9]+$`. `checkKey(s, what)` calls `ParseRoleKey` and wraps its error with `what`. Replace the three `strings.HasPrefix` checks.
- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit** "Parse role keys and check the hierarchy with Parent".

### Task 2: question_open, bruh_info, and lease patterns

**Files:**

- Create: `plugins/bruh/mcp/question.go`, `plugins/bruh/mcp/question_test.go`
- Modify: `plugins/bruh/mcp/lease.go`, `plugins/bruh/mcp/tools.go`, `plugins/bruh/mcp/rpc_test.go`

**Interfaces:**

- `question_open(priority, subject, body, blocks)` returns `{"id":"Q-<n>","header":"<priority> Q-<n>: <subject>"}`. It stores `questions/Q-<n>.json` = `{"id","priority","subject","body","blocks","asker","at"}` and keeps the next number in `questions/next`, both under the lock `questions`. The header must match the mail header grammar of plan 1, so the subject is one line of at most 200 characters.
- `bruh_info()` returns `{"plugin_root","data_dir","role_key","version"}`. The paths are absolute. The version comes from `.claude-plugin/plugin.json`. It works without `BRUH_ROLE_KEY` (`role_key` is then empty) and writes nothing.
- `lease_define` takes an optional `patterns` (list of strings, each 1 to 200 characters, one line). When `patterns` is absent, a redefinition keeps the old patterns. `Resource` gets `"patterns"` (omitted when empty).

- [ ] **Step 1: Write the failing tests.** `TestQuestionOpenNumbersInOrder` (two calls give `Q-1` and `Q-2`, the file has the asker and a UTC stamp); `TestQuestionOpenRefusesBadInput` (priority `P3`, empty subject, subject with a newline); 20 parallel calls give 20 different IDs; `TestBruhInfo`; `TestLeaseDefinePatterns`. `TestMissingRoleKeyWritesNothing` gets an exemption list: `bruh_info`, `init_plan`, `init_apply` run in any session, and they must still write nothing for `{}`.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement** the tools and register them in `AllTools`.
- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit** "Add question_open, bruh_info, and lease patterns".

### Task 3: Session tools

**Files:**

- Create: `plugins/bruh/mcp/session.go`, `plugins/bruh/mcp/session_test.go`
- Modify: `plugins/bruh/mcp/env.go` (fields `ClaudeBin`, `Home`, `SettingsFile`), `plugins/bruh/mcp/tools.go`, `plugins/bruh/mcp/helpers_test.go`

**Interfaces:**

- `Env.ClaudeBin` is `BRUH_CLAUDE_BIN`, default `claude`. `Env.Home` is the home folder. `Env.SettingsFile` is `BRUH_SETTINGS_FILE`, default `<home>/.claude/settings.json`.
- `session_launch(agent, role_key, cwd, prompt)`: the caller is `Parent(role_key)`; `agent` is `clanker` for a clanker key and `clerk` for a clerk key or `clerk-ledger`; `cwd` is an absolute folder; `<data>/roles/<role key>.json` exists; no entry of `claude agents --json --all` has the name and a `pid`. Then it runs, in `cwd`, with `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1` added and `BRUH_ROLE_KEY` removed from the environment: `<claude> --bg --agent bruh:<agent> --name <role key> --permission-mode auto --settings <file> [--plugin-dir <plugin root>] <prompt>`. `--plugin-dir` is added when the absolute plugin root is not under `<home>/.claude/plugins/`. It reads the short ID from the output line `backgrounded · <id>` and returns `{"role_key","session_id","name","state"}` from the agents entry with that ID (it polls up to 20 times at `PollInterval`).
- `session_resume(role_key, prompt)`: the caller is `Parent(role_key)`; the newest background entry with the name gives the session ID; a live entry (with `pid`) is refused, because a live session gets a nudge instead. It runs `<claude> --resume <session ID> --bg <prompt>` with no other flags, because a background session keeps its saved options and extra flags start a copy (probe G1). When the output names another short ID (a copy), it returns an error with the output.
- `session_list()`: the agents entries whose `name` parses as a role key, each with `role_key` added.

- [ ] **Step 1: Write the failing tests** with a fake `claude` script in `t.TempDir()`: it appends each argument on its own line and the value of `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE` and `BRUH_ROLE_KEY` to a log file, prints `backgrounded · 7c5dcf5d · <name>` for `--bg`, and prints a fixture file for `agents --json --all`. `TestSessionLaunchArgv` checks the exact argument vector, the working folder, and the environment, and the `--plugin-dir` rule both ways. `TestSessionLaunchRefusals`: wrong parent, agent and key mismatch, relative `cwd`, missing role settings file, live name. `TestSessionResumeArgv` checks the exact argument vector and the refusal of a live session. `TestSessionList` filters names.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement** `session.go` with one helper `agents(env) ([]map[string]any, error)` and one helper `runClaude(env, dir, args...)`.
- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit** "Add session_launch, session_resume, and session_list".

### Task 4: Hook scripts and hooks.json

**Files:**

- Create: `plugins/bruh/scripts/statusline-tap.sh`, `plugins/bruh/scripts/handoff-nudge.sh`, `plugins/bruh/scripts/handoff-inject.sh`, `plugins/bruh/scripts/lease-guard.sh`, `plugins/bruh/hooks/hooks.json`
- Create: `plugins/bruh/mcp/scripts_test.go`

**Interfaces:**

- `hooks.json`: `PostToolUse` (no matcher, all tools) runs `sh ${CLAUDE_PLUGIN_ROOT}/scripts/handoff-nudge.sh`; `SessionStart` with matcher `compact|clear|resume` runs `handoff-inject.sh`; `PreToolUse` with matcher `Bash` runs `lease-guard.sh`. Exec form: `"command": "sh", "args": ["${CLAUDE_PLUGIN_ROOT}/scripts/<name>"]`.
- `statusline-tap.sh [<previous command>]`: the data folder is the parent of the folder of the script. With `BRUH_ROLE_KEY` set and a `session_id` of letters, digits, `-`, and `_`, it writes `<data>/context/<session_id>.json` = `{"used_percentage":<n>,"five_hour_percentage":<n>,"at":"<UTC>"}` through a temporary file and `mv`. It writes no file when `context_window.used_percentage` is `null` or absent, and omits `five_hour_percentage` when `rate_limits.five_hour.used_percentage` is `null` or absent. Then it runs `sh -c "$1"` with the saved input bytes on standard input, so the previous command gets the input unchanged and prints the status line.
- `handoff-nudge.sh`: exits at once for input with `agent_id`. It reads `<data>/context/<session_id>.json`. At or above `CLAUDE_PLUGIN_OPTION_HANDOFF_PERCENT` (default 50, and 50 when the value is not a number) and with no marker `<data>/context/<session_id>.nudged`, it creates the marker and prints `{"hookSpecificOutput":{"hookEventName":"PostToolUse","additionalContext":"..."}}`. Below the threshold it removes the marker, which arms the next crossing.
- `handoff-inject.sh`: prints `{"hookSpecificOutput":{"hookEventName":"SessionStart","additionalContext":"The compaction summary is not a source. Rules come only from this handoff and from rules.md.\n\n<file>"}}` for `<data>/handoffs/<role key>.md`, and nothing when the file does not exist.
- `lease-guard.sh`: splits `tool_input.command` into segments at `;`, `&`, `|`, and newlines, removes leading spaces, and denies when a segment starts with a `patterns` entry of a resource and the role key holds no grant of that resource with `until` later than now: `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"..."}}`.

- [ ] **Step 1: Write the failing tests.** A helper `runScript(t, script, stdin, env...)` runs `sh ../scripts/<script>` with only the given environment plus `PATH`, and returns standard output and the exit code. Tests, one for each spec section 20 script bullet: `TestTapWritesBothPercents`, `TestTapWritesNothingForNull`, `TestTapRunsPreviousCommand` (the previous command is `cat > <file>; printf 'line'` with quotes; the file equals the input byte for byte, and the output is `line`), `TestTapNoRoleKey` (no file, the previous command still runs), `TestNudgeOncePerCrossing` (70, 80 give one message; 30 re-arms; 60 gives a second message), `TestNudgeSkipsAgent`, `TestNudgeNoRoleKey`, `TestNudgeThresholdOption`, `TestInjectAfterCompactAndClear` (the same output for `source` `compact` and `clear`, and the first line), `TestInjectNoFile`, `TestInjectNoRoleKey`, `TestLeaseGuardDeniesWithoutLease`, `TestLeaseGuardAllowsWithLease`, `TestLeaseGuardExpiredLease`, `TestLeaseGuardCompoundCommand` (`cd x && psql -h test-db`), `TestLeaseGuardNoRoleKey`, and `TestHooksJSON` (events, matchers, and script paths exist).
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Write the scripts and `hooks.json`.**
- [ ] **Step 4: Run** `go test -race ./...` and shellcheck. Expected: PASS.
- [ ] **Step 5: Commit** "Add the hook scripts and the status line tap".

### Task 5: userConfig

**Files:**

- Modify: `plugins/bruh/.claude-plugin/plugin.json`, `plugins/bruh/mcp/skeleton_test.go`

**Interfaces:** `userConfig` with `user_name` (`string`), `handoff_percent` (`number`, default 50, `min` 1, `max` 99), `max_busy_clerks` (`number`, default 8, `min` 1). Each has `type`, `title`, and `description`. `user_name` is not `required`, so a non-interactive install does not stop at the dialog; init writes it.

- [ ] **Step 1: Write the failing test** `TestUserConfig`.
- [ ] **Step 2: Add the block. Run** `go test ./...` and `claude plugin validate ./plugins/bruh`. Expected: PASS.
- [ ] **Step 3: Commit** "Declare the plugin options as userConfig".

### Task 6: init_plan and init_apply

**Files:**

- Create: `plugins/bruh/mcp/ojson.go`, `plugins/bruh/mcp/diff.go`, `plugins/bruh/mcp/init.go`, `plugins/bruh/mcp/init_test.go`
- Modify: `plugins/bruh/mcp/roles.go` (extract `roleSettings(root, key, env, deny)`), `plugins/bruh/mcp/tools.go`, `plugins/bruh/defaults/role-settings.json`

**Interfaces:**

- `ojson.go`: `parseOrdered([]byte) (any, error)` and `encodeOrdered(any) []byte`. Objects keep their key order; numbers stay `json.Number`; the output has two-space indentation, no HTML escaping, and a final newline.
- `diff.go`: `unifiedDiff(path, old, new string) string`, a line diff (longest common subsequence) with `--- <path>`, `+++ <path>`, and `@@ -a,b +c,d @@` hunks with three lines of context. A new file diffs against empty text.
- `init_plan(answers)`: validates the answers of interfaces section 5 (`user_name` one line; `ledger_path` absolute; `mode` `human` or `autonomous`; numbers positive; `auto_compact_window` 100000 to 1000000; `handoff_percent` 1 to 99; `channels` only `telegram` and `slack`; each merge grant has `repo` `owner/name`, a clerk `merger` key, and `conditions`). It plans these files and writes the plan to `<data>/init/plans/<plan id>.json` (`{"id","at","files":[{"path","mode","before","content"}]}`, `before` is the SHA-256 of the current content or `absent`):
  - `Env.SettingsFile`: `autoCompactWindow`; `statusLine` (when `wrap_statusline`); `permissions.allow` gets `Workflow(bruh:deliver)` and `mcp__plugin_bruh_bruh__<tool>` for every tool except `init_apply`, plus the two Slack tools when `channels` has `slack`; `pluginConfigs["bruh@bruh"].options` gets `user_name`, `handoff_percent`, and `max_busy_clerks`. Other keys and their order stay.
  - `<data>/bin/statusline-tap.sh` (mode 0755), a copy of `scripts/statusline-tap.sh`, when `wrap_statusline`.
  - `<data>/roles/bigm.json` from `defaults/role-settings.json` with `BRUH_ROLE_KEY=bigm`, only when it does not exist.
  - Each file of `ledger-template/` (recursive) that does not exist in `ledger_path`, with `{{<answer key>}}` and `{{date}}` replaced. Lists become Markdown bullet lists, or `none`.
  - It returns `{"plan_id","diff","launch_command","trust"}`: the diff of all files, the bigm start command of spec 3.4 with the chosen channels, and the folders that need workspace trust (the ledger folder).
- The status line wrap: `statusLine.command` becomes `'<data>/bin/statusline-tap.sh' '<previous command>'` (each part single-quoted for `sh`). Other `statusLine` fields stay. A command that already starts with the quoted tap path stays as it is, so a second init does not wrap twice. No previous command gives the tap path alone.
- `init_apply(plan_id)`: reads the plan, checks every `before` hash against the current file, and refuses the whole plan with the stale path when one differs. Then it writes each file with `atomicWrite` (parent folders with mode 0700, except the ledger, 0755), sets the mode, deletes the plan, and returns `{"applied":[<paths>]}`.
- `defaults/role-settings.json` gets `env.CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` = `"16"` (spec 3.7).

- [ ] **Step 1: Write the failing tests** in a temporary plugin root (a copy of `scripts/`, `defaults/`, `.claude-plugin/`, and a fixture `ledger-template/` with `README.md` and `mode.md` containing `mode: {{mode}}`). `TestOrderedJSONRoundTrip` (order, `&&`, numbers like `1e3` stay), `TestUnifiedDiff`, `TestInitPlanWritesNothingOutsidePlans`, `TestInitPlanWrapsStatusLine` (a previous command with a single quote), `TestInitPlanWrapIsIdempotent`, `TestInitPlanKeepsOtherKeys`, `TestInitPlanValidates`, `TestInitApplyWritesPlannedContent`, `TestInitApplyRefusesStalePlan`, `TestInitLedgerKeepsExistingFiles`, and an end-to-end check that the wrapped command, run by `sh -c`, writes the context file and runs the previous command.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 5: Commit** "Add init_plan and init_apply".

### Task 7: Non-interactive init and command dispatch

**Files:**

- Modify: `plugins/bruh/mcp/main.go`, `plugins/bruh/mcp/init.go`
- Create: `plugins/bruh/mcp/cli_test.go`

**Interfaces:**

- `main.go`: no argument runs the MCP server. `init --answers <file>` runs the non-interactive init. Plan 5 adds `watch` and `merge-train`. An unknown command prints the usage and exits 2.
- `init`: reads the answers file (optional), then `BRUH_INIT_<KEY>` variables override single keys (`BRUH_INIT_USER_NAME=Sam`; a list or object value is JSON, for example `BRUH_INIT_CHANNELS='["telegram"]'`). The data folder is `BRUH_DATA`, default `<home>/.claude/plugins/data/bruh-bruh`. The plugin root is `BRUH_PLUGIN_ROOT`, default the parent of the working folder (`go run -C <plugin root>/mcp` sets it to `mcp/`). It prints the diff, applies the plan, prints the applied paths and the launch command, and exits 0.

- [ ] **Step 1: Write the failing test** `TestCLIInit` that runs `runCLI([]string{"init","--answers",f}, env, stdout)` and checks the diff text and the written files, and `TestCLIInitEnvAnswers`.
- [ ] **Step 2: Implement** `runCLI(args, env, out) int`.
- [ ] **Step 3: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 4: Commit** "Add the non-interactive init command".

### Task 8: The init skill

**Files:**

- Create: `plugins/bruh/skills/init/SKILL.md`
- Modify: `plugins/bruh/mcp/skeleton_test.go`

**Interfaces:** frontmatter `name: init`, `description`, `disable-model-invocation: true` (only the user starts init). The body: the 13 questions of spec 16, one for each message, with the defaults of interfaces section 5; question 9 shows the current `statusLine.command` (read from `~/.claude/settings.json`) and the new value; then `init_plan`, the diff in a code block, an explicit yes, `init_apply`; the folders that need workspace trust; the bigm start command; the non-interactive form.

- [ ] **Step 1: Write the failing test** `TestInitSkill`: the frontmatter, 13 numbered questions, and the names `init_plan` and `init_apply`.
- [ ] **Step 2: Write the skill.** Run the test, markdownlint, and `claude plugin validate ./plugins/bruh`.
- [ ] **Step 3: Commit** "Add the init skill".

### Task 9: Checks

- [ ] Run gofmt, `go vet`, `go test -race`, shellcheck, markdownlint, and both `claude plugin validate` commands. Fix what fails.
- [ ] End-to-end check: in a scratch folder inside a trusted folder, `claude -p --plugin-dir <repo>/plugins/bruh --model haiku "Call the bruh MCP tool bruh_info and print its result."` prints the plugin root and a data folder. This also proves that the plugin loads with the Slack channel server of plan 5 and no Slack option set.

---

## Deviations from the specification

1. Script tests are Go tests (`scripts_test.go`), not `bats` (index deviation 1).
2. `lease_request` refuses a request from bigm and stores the grantor, which the specification does not name. A request needs a grantor, and bigm has none.
3. The ledger layout is written for each missing file, not only when the folder is empty. A new private repository on a code host often has a `README.md`, and an "only when empty" rule would then write nothing. No existing file is changed.
4. `permissions.allow` lists each MCP tool by name and leaves out `init_apply`. A wildcard rule would let an agent write user settings without the user. Agent-derived, needs owner decision (options, ranked: 1. every tool except `init_apply`, as built; 2. the server wildcard `mcp__plugin_bruh_bruh__*`; 3. no MCP allow rules).
5. `role_settings_write` lets bigm write its own `bigm` file. The specification (3.4) starts bigm with that file, and `Parent("bigm")` is empty.

## Agent-derived decisions (need owner decision)

1. The previous status line command is kept inside `statusLine.command` as the quoted argument of the tap, not in a separate file. The user sees what is wrapped, one file holds the value, and unwrapping is one edit. Options, ranked: 1. an argument of the tap, as built; 2. a file `<data>/bin/statusline-previous`; 3. an environment variable in settings.
2. `user_name` is not `required` in `userConfig`, so a container install does not stop at the dialog. init writes the value. Options: 1. not required, as built; 2. required.
3. The ledger template uses `{{<answer key>}}` and `{{date}}` placeholders, which init replaces. This is a new cross-lane contract with lane roles. Options: 1. placeholders, as built; 2. init writes `mode.md`, `grants.md`, and the delegated classes itself and copies the other template files unchanged.
4. `session_resume` refuses a live session (a `pid` in the agents entry). A live session gets a `SendMessage` nudge instead.
5. The plugin ID for `pluginConfigs` is `bruh@bruh` (the marketplace install of spec 18). A development load with `--plugin-dir` uses another ID, so the options set by init do not apply to it.

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| `claude agents --json --all` entries have `cwd`, `kind`, `startedAt` (ms) always; `id` and `state` for background sessions; `pid` and `status` only while the process lives; `waitingFor` when `status` is `waiting`; `sessionId` and `name` when set. Interactive sessions are in the list too. | Ran the command (read-only), Claude Code 2.1.284; agent-view.md "List sessions as JSON" | <https://code.claude.com/docs/en/agent-view.md> |
| `claude --bg` prints `backgrounded · <short id> · <name>`, sometimes after `Starting background service…`. | Probe G1; agent-view.md | same |
| `claude --resume <session ID> --bg "<prompt>"` without other flags wakes a stopped background session under the same ID with its saved options (`--plugin-dir`, `--agent`, `--name`, `--settings`, `--model`, `--permission-mode`). With any of these flags, it starts a copy under a new ID with an automatic name. A resume while the stop is still in progress also starts a copy. | Probe G1 | — |
| A status line command in a background session gets `BRUH_ROLE_KEY` from the `env` of the `--settings` file. | Probe G1: the status line log had `clerk-probe-go1` | — |
| `--plugin-dir <dir> --agent <plugin>:<agent>` with `--bg` prints `warning: no agent named ...`, but the session starts with the plugin agent. | Probe G1: the session header showed `@probe:probe-role` | — |
| Plugin `hooks/hooks.json` has an optional `description` and a `hooks` object; exec form is `command` plus `args`, with no shell; `${CLAUDE_PLUGIN_ROOT}` is substituted in `command` and `args`. | Read the page | <https://code.claude.com/docs/en/hooks.md> |
| Matcher: only letters, digits, `_`, `-`, spaces, `,`, and `\|` is an exact string or list; `compact\|clear\|resume` is a list. | Read the page | hooks.md, "Matcher patterns" |
| `PreToolUse` denies with `hookSpecificOutput.permissionDecision` `deny` and `permissionDecisionReason` (shown to Claude). `PostToolUse` and `SessionStart` add text with `hookSpecificOutput.additionalContext` and `hookEventName`. | Read the page | hooks.md, decision control sections |
| Hook processes get `CLAUDE_PLUGIN_ROOT`, `CLAUDE_PLUGIN_DATA`, `CLAUDE_PROJECT_DIR`, and `CLAUDE_PLUGIN_OPTION_<KEY>`. Commands of the Bash tool get none of them. | Read the page | <https://code.claude.com/docs/en/plugins-reference.md>, "Where each variable resolves" |
| `userConfig` options are strict objects: `type` (`string`, `number`, `boolean`, `directory`, `file`), `title`, `description` required; `required`, `default`, `options`, `multiple`, `sensitive`, `min`, `max` optional. | Read the page; `claude plugin validate` | plugins-reference.md, "User configuration" |
| Non-sensitive option values are stored in user settings as `pluginConfigs["<plugin>@<marketplace>"].options.<key>`; project and local entries are ignored. | Read the page | <https://code.claude.com/docs/en/settings-reference.md>, `pluginConfigs` |
| `statusLine` is `{"type":"command","command":"...","padding","refreshInterval","hideVimModeIndicator"}`; the command runs in a shell. | Read the page | settings-reference.md, `statusLine`; statusline.md |
| `rate_limits.five_hour.used_percentage` exists (0 to 100) only for claude.ai Pro and Max subscribers and after the first API response; each window can be absent. `context_window.used_percentage` can be `null` early. | Read the page | <https://code.claude.com/docs/en/statusline.md>, "Available data" |
| `autoCompactWindow` is a token count from 100000 to 1000000. | Read the page | settings-reference.md, `autoCompactWindow` |
| An MCP allow rule can name one tool (`mcp__<server>__<tool>`) or all tools of a server (`mcp__<server>__*`). | Read the page | <https://code.claude.com/docs/en/permissions.md> |
| `disable-model-invocation: true` in skill frontmatter stops Claude from starting the skill on its own. | Read the page | <https://code.claude.com/docs/en/skills.md> |

### Probe G1 (2026-09-30, Claude Code 2.1.284)

In a scratch folder inside a trusted folder (no `git init`), with a settings file `{"env":{"BRUH_ROLE_KEY":"clerk-probe-go1"},"statusLine":{"type":"command","command":"<append $BRUH_ROLE_KEY and the time to a log file>"}}`:

1. `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --plugin-dir probes/probe-plugin --agent probe:probe-role --name probe-go1 --settings <file> --permission-mode auto --model haiku "<prompt>"` printed the `no agent named` warning and `backgrounded · <id> · probe-go1`. The status line log got `clerk-probe-go1` lines while no terminal was attached. (Auto mode is not available for Haiku, so the session fell back to manual mode and waited on a permission prompt.)
2. After `claude stop <id>`, `claude --resume <session ID> --bg` with the same flags and a second settings file printed `note: background session <id> keeps its own saved options, so the flags you passed started a copy as <new id>`.
3. `claude --resume <session ID> --bg "<prompt>"` without flags printed `note: woke session <id> with its saved options (--plugin-dir, --agent, --name, --settings, --model, --permission-mode)` and `backgrounded · <id> · probe-go1`. The agents entry had the same `sessionId`, and the status line log got `clerk-probe-go1` again.

Every session of the probe was stopped with `claude stop <id>`, and `claude agents --json --all` showed no `pid` for any of them afterward.

## Self-review

- **Spec coverage:** 3.2 role key (Task 1), 7 tap, handoff message, and pickup (Task 4), 8.4 lease guard and patterns (Tasks 2 and 4), 10.1 tools (Tasks 2, 3, 6), 13 deny rules in role settings (Task 6, defaults), 16 init skill and non-interactive form (Tasks 6 to 8), 20 script tests (Task 4).
- **Verify items:** 3.4 `--agent` with a plugin agent (probe G1), 4.1 resume under the same ID (probe G1), 7 status line in a background session and `rate_limits.five_hour.used_percentage` (probe P4 of plan 1, probe G1, statusline.md), 16 plugin options (`userConfig` plus `pluginConfigs`, documented).
