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

- `question_open(priority, subject, body, blocks)` returns `{"id":"Q-<n>","header":"<priority> Q-<n>: <subject>"}`. It stores `questions/Q-<n>.json` and keeps the next number in `questions/next`, both under the lock `questions`. The header must match the mail header grammar of plan 1, so the subject is one line of at most 200 characters. `blocks` must not be empty. Agents read the question file directly, so its fields are a contract:

  | Field | Value |
  |---|---|
  | `id` | `Q-<n>` |
  | `priority` | `P0`, `P1`, or `P2` |
  | `subject` | one line, 1 to 200 characters |
  | `body` | text |
  | `blocks` | the work that waits for the answer |
  | `asker` | the role key of the caller (a workflow agent has the key of its clerk) |
  | `opened_at` | UTC time from the server, layout `2006-01-02T15:04:05.000Z` |

- `mail_post` also accepts the header `START: <subject>` (same length rule as `DONE:`). The parent writes each start message with it. Agent-derived, needs owner decision (request of lane roles through the controller, 2026-09-30). Options, ranked: 1. a `START:` header, as built; 2. the start message as a `DONE:`-style header with a fixed subject such as `START: start message`; 3. no header, and the first message of a mailbox is the start message by position.
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
  - Each file of `ledger-template/` (recursive) that does not exist in `ledger_path`, filled by the rules of `ledger-template/README.md` of lane roles: in `mode.md`, the value of each `key: value` line for `mode`, `p1_batch_minutes`, `p1_batch_size`, and `review_round_cap` becomes the answer, `changed` becomes the UTC time (`2006-01-02T15:04:05Z`), and `reason` becomes `init`; `priorities.md` becomes `defaults/priorities.md` with one `- <class>` item for each delegated P1 class at the end of the section "Delegated P1 classes"; `grants.md` gets one row `| <repo> | <merger> | <conditions> | <conditions> | <UTC time> | init |` for each merge grant (a `|` in a cell becomes `\|`). The other files are copied unchanged. The test fixture `mcp/testdata/ledger-template/` is a copy of the template of lane roles, and `mcp/testdata/priorities.md` is a copy of its default priorities.
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

- `role-settings <role key>`: writes `<data>/roles/<role key>.json` with the defaults and `BRUH_ROLE_KEY`, prints the absolute path, and refuses to overwrite an existing file. A remote clanker is started by an Orca worker session that has no `BRUH_ROLE_KEY`, so it cannot call `role_settings_write`. After fix round 1, it writes only clanker keys. Agent-derived, needs owner decision (request of lane roles through the controller, 2026-09-30). Options, ranked: 1. a CLI command for clanker keys only, as built; 2. bigm writes the file on its own machine and the Orca worker copies it; 3. the Orca worker session sets `BRUH_ROLE_KEY` and calls `role_settings_write`.

- [ ] **Step 1: Write the failing test** `TestCLIInit` that runs `runCLI([]string{"init","--answers",f}, env, stdout)` and checks the diff text and the written files, `TestCLIInitEnvAnswers`, and `TestCLIRoleSettings`.
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
- [ ] End-to-end check: in a scratch folder inside a trusted folder, `claude -p "Call the MCP tool mcp__plugin_bruh_bruh__bruh_info and print its JSON result exactly." --plugin-dir <repo>/plugins/bruh --model haiku --allowedTools=mcp__plugin_bruh_bruh__bruh_info` prints the plugin root and a data folder. This also proves that the plugin loads with the Slack channel server of plan 5 and no Slack option set. Result (2026-09-30): PASS, `{"data_dir":"<home>/.claude/plugins/data/bruh-inline","plugin_root":"<repo>/plugins/bruh","role_key":"","version":"0.1.0-dev"}`.

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
3. A merge grant from init fills the column "Owner words" of `grants.md` with the conditions text that the owner typed, and the column "Question ID" with `init`, because an init answer has no question ID. Options: 1. as built; 2. an extra answer field `words` for each grant; 3. leave both columns empty.
4. `session_resume` refuses a live session (a `pid` in the agents entry). A live session gets a `SendMessage` nudge instead. Options, ranked: 1. refuse, as built; 2. send the prompt as a `SendMessage` nudge instead; 3. resume anyway, which starts a copy (probe G1).
5. The plugin ID for `pluginConfigs` is `bruh@bruh` (the marketplace install of spec 18). A development load with `--plugin-dir` uses another ID, so the options set by init do not apply to it. Options, ranked: 1. `bruh@bruh` only, as built; 2. write the options for both `bruh@bruh` and `bruh@inline`; 3. an answer `plugin_id`.

6. Init answers refuse unknown keys (`user_nmae` is an error, not a silent default). Options, ranked: 1. refuse, as built; 2. ignore unknown keys with a warning in the result; 3. ignore them silently.
7. `permissions.allow` of the Slack tools (`mcp__plugin_bruh_slack__post_question`, `mcp__plugin_bruh_slack__reply`) is added only when `channels` contains `slack`. Options, ranked: 1. only with Slack, as built; 2. always.

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
| A plugin loaded with `--plugin-dir` gets the plugin ID `bruh@inline` and the data folder `~/.claude/plugins/data/bruh-inline/`. So the `pluginConfigs["bruh@bruh"]` values that init writes apply only to the marketplace install. | End-to-end check of Task 9: `bruh_info` returned `data_dir` `<home>/.claude/plugins/data/bruh-inline` | — |
| The bruh MCP server and the Slack channel server both start when the Slack options are not set (the `${user_config.*}` values are unresolved): the debug log shows `MCP server "plugin:bruh:slack": Successfully connected`. | End-to-end check of Task 9, `claude -p --debug-file` | — |
| `/plugin configure <plugin>` opens the `userConfig` dialog; `claude plugin install --config key=value` sets an option. | Read the page | <https://code.claude.com/docs/en/plugins/cli-reference.md> |
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

## Fix round 1 (2026-09-30)

The adversarial review and the checker of lane go found 1 blocker, 8 major and 14 minor findings, and 1 checker FAIL. The changes of this plan are listed here. The report of the lane maps each finding to its commit.

- `init_plan` writes nothing. It keeps the plan in the memory of the server process (the same session calls both tools) and returns `diff_sha256`, the SHA-256 of its `diff`. `init_apply(plan_id, diff_sha256)` refuses an unknown plan, a wrong hash, and a target state that changed (it plans again at the time of `init_plan` and compares). It writes only the resolved settings file and files under the data folder and under `ledger_path`, and it writes the settings file last. `ledger_path` must be absolute, have no `..` element, and be in a folder that exists or can be created. A symbolic link in the ledger that points outside is refused. `init_apply` carries `_meta["anthropic/requiresUserInteraction"]: true`, so Claude Code asks a person for each call in every permission mode. Agent-derived, needs owner decision (ruling of the controller, 2026-09-30). Options, ranked: 1. memory plan, diff hash, closed list, and the user-interaction flag, as built; 2. the same without the flag, which relies on the allow list; 3. a plan file on disk with a hash in memory.
- init writes `<data>/init/config.json` = `{"ledger_path"}`. The merge train reads it (plan 5).
- The settings file keeps its indentation unit (tabs or any number of spaces).
- The init tests use the real `ledger-template/` and `defaults/priorities.md`; the copies in `testdata/` are gone.
- `WithLock` uses `flock` (macOS and Linux). The kernel releases the lock of a dead holder, so there is no stale lock to break.
- `question_open` creates `Q-<n>.json` with `O_EXCL`, before it moves `next`.
- `session_launch` checks for a live session and starts `claude` under a lock of the role key.
- The lease guard removes quoted strings, and a segment matches a pattern only when it equals it or continues with white space. bigm gets its own deny reason. `lease_define` refuses a pattern with white space at its start or end.

Rulings on findings that were not changed in code:

1. `autoCompactWindow` and the MCP allow rules stay in the user settings, because spec 7 and spec 16 name the user settings. Agent-derived, needs owner decision. Options, ranked: 1. user settings, as the spec says; 2. the allow rules in the role settings files, which also keeps them out of the manual sessions of the owner; 3. both.
2. An uninstall of the plugin deletes the data folder and with it the tap, so the status line command fails; the previous command is still the quoted argument. The init skill tells the user. Agent-derived, needs owner decision. Options, ranked: 1. document it, as built; 2. copy the tap to `~/.claude/bruh/` outside the plugin data folder, against spec 10.1; 3. a command that falls back to the previous command when the tap is missing.
3. The child `claude` of `session_launch` inherits the environment of the MCP server except `BRUH_ROLE_KEY`, including `CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`, and `CLAUDE_CODE_MESSAGING_SOCKET`. Probe G1 ran `claude --bg` from the Bash tool of a live session, which has the same variables, and the new session got its own ID and registered normally. So no variable is removed. Agent-derived, needs owner decision. Options, ranked: 1. keep them, as built; 2. remove the `CLAUDE_CODE_*` session variables and probe again.

## Final review fixes (2026-09-30)

The final whole-branch review found 1 blocker, 7 major, and 17 minor findings. The rulings of the controller for the findings of this plan are below. `.superpowers/final-fix-report.md` (git-ignored) maps each finding to its commit.

1. **B1, Telegram in role sessions.** `roleSettings` (used by `role_settings_write` and by the `role-settings` command) sets `"enabledPlugins": {"telegram@claude-plugins-official": false}` for every role key except `bigm`. Agent-derived, needs owner decision (ruling of the controller, 2026-09-30). Options, ranked: 1. turn the plugin off in the role settings of every key except `bigm`, as built; 2. install the plugin disabled at user scope and turn it on only in `roles/bigm.json`, which also protects the manual sessions of the owner; 3. document it only. Verify (smoke test of spec 20): a clanker started with this settings file does not start the Telegram server (bigm with `--channels`, then one role session; the log of bigm shows no `replacing stale poller`).

| Fact | How verified | Source |
|---|---|---|
| `enabledPlugins` turns single plugins on or off, keyed by `plugin-name@marketplace-name`, with the scope "Any file". A higher scope wins for each plugin: a project `true` beats a user `false`. | Read the page | <https://code.claude.com/docs/en/settings-reference.md> (`enabledPlugins`) |
| `--settings` "can set any key your user settings file can set", above the user, project, and local files and below managed settings. | Read the page | <https://code.claude.com/docs/en/settings.md> ("Change a setting for one session", "Settings precedence") |
2. **M1, sender policy of `mail_post`.** `mail_post` accepts a message only on an edge of the role tree: the receiver is `Parent(sender)`, or the sender is `Parent(receiver)`, or the sender is `bigm`. A clerk may also post a header that starts with `P0 ` to `bigm`. A `RULE` comes only from `bigm`. `START:` and `ANSWER` come only from `Parent(receiver)` or from `bigm` (so an `ANSWER` to a clerk comes from its clanker or bigm, and to a clanker only from bigm). The check uses `ParseRoleKey` and `Parent`, never a string prefix, and it is a speed bump like a deny rule (SECURITY.md). Agent-derived, needs owner decision (ruling of the controller, 2026-09-30). Options, ranked: 1. the tree edges and the header rules, as built; 2. only the header rules for `RULE`, `START`, and `ANSWER`, with free edges for questions and `DONE`; 3. no policy in the server, a `from` check in the agent text only.
3. **M3, bigm starts the merger clerk of a remote project.** `session_launch` and `session_resume` accept the caller when it is the parent of the key or `bigm`. `role_settings_write` follows the same rule, because `session_launch` needs the role settings file first; before, bigm could write only its own file and the files of its children. So bigm can start `clerk-<project>-merge` of a remote project on its own machine, in the ledger folder, where `grants.md`, the mailbox of the merger, and `repos.json` are. The merge gate does not change. Deviation 5 of this plan (bigm writes its own file) is now a case of this rule. Agent-derived, needs owner decision (ruling of the controller, 2026-09-30). Options, ranked: 1. parent or bigm for every key, as built; 2. parent, or bigm only for the keys `clerk-<project>-merge`, which is narrower; 3. the remote init copies `grants.md`, and the gate accepts an approval of the remote clanker (options 2 and 1 of the review).
