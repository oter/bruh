# Plan 3: Roles and the ledger

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task by task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the three role agents (`bruh:bigm`, `bruh:clanker`, `bruh:clerk`) as complete operating procedures, the default `priorities.md` and `house-rules.md`, and the ledger template, with a structural check in CI that keeps the agent text in step with the MCP tool contract.

**Architecture:** Each role is a plugin agent file. Its body is the system prompt of the session, so it reloads after compaction (spec 3). The body is the full procedure of the role: it names each MCP tool of the interfaces file exactly, and it names the files and headers of spec 5 and 8. The defaults are data files that the init skill (plan 2, lane go) copies. The ledger template is a folder that the init tool copies verbatim. A `node --test` file checks the frontmatter of each agent, checks that each MCP tool name in the agent text is in the interfaces list, and checks that each role names the tools that its procedure needs.

**Tech Stack:** Markdown (ASD-STE100 Simplified Technical English), YAML frontmatter, Node.js 22 (`node --test`, standard library only), GitHub Actions.

**Spec:** [../../spec.md](../../spec.md) version 0.4, sections 2, 3, 4, 5, 8, 9, 11, 12, 13, 14, 15. Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md).

## Global Constraints

- Agent frontmatter fields come from sub-agents.md, "Frontmatter reference": `name` and `description` are required; `model`, `effort`, `tools`, `disallowedTools` are optional. A plugin agent ignores `hooks`, `mcpServers`, and `permissionMode`, so the agents do not set them.
- Every role runs on a 1M-context model (spec 3.3). The model value is `opus[1m]` (model-config.md, "Model aliases").
- The MCP tool names are the names of interfaces section 2, with the session prefix `mcp__plugin_bruh_bruh__`. The agent text uses the short names in backticks.
- The agents find the plugin scripts through `bruh_info` (`plugin_root`), as the interfaces file says.
- The owner is addressed as `${user_config.user_name}`. Claude Code substitutes non-sensitive `${user_config.KEY}` values in agent content (plugins-reference.md, "User configuration").
- Timestamps in the ledger, in messages, and in handoffs come from `date -u +%Y-%m-%dT%H:%M:%SZ` or from the MCP server (spec 2, principle 4).
- Message headers are only those of spec 5, and they match the `headerRE` of `mail.go`: `P0 Q-<n>:`, `P1 Q-<n>:`, `P2 Q-<n>:`, `ANSWER Q-<n>:`, `REC Q-<n>:`, `RULE R-<n>:`, `DONE:`.
- Project documentation is ASD-STE100 Simplified Technical English. No personal names, host names, or machine paths.

## Review Focus

1. **A role reads a tool name that the MCP server does not have** (a typo, or a tool renamed in the contract). Expected: CI fails and names the file and the tool. Test in Task 1: `agents_test.mjs`, "every MCP tool name in the agent text exists".
2. **An agent file with frontmatter that Claude Code does not load** (a missing `name`, a colon in `name`, a model without a 1M window). Expected: CI fails. Test in Task 1: "frontmatter is valid".
3. **A role procedure that forgets a required step** (for example bigm without `session_list`, or the clerk without `answer_write`). Expected: CI fails and names the role and the missing tool. Test in Task 1: "each role names the tools of its procedure".
4. **A clanker that writes a file** (spec 3.5). Expected: the clanker agent has `disallowedTools` with `Edit`, `Write`, and `NotebookEdit`, a mechanical stop (spec 2, principle 2). Test in Task 1: "the clanker cannot write files".
5. **A ledger template that the init tool cannot fill** (a missing file of spec 8, or a `mode.md` without a runtime settings key). Expected: CI fails. Test in Task 1: "the ledger template has the layout of spec 8".

---

### Task 1: The structural check

**Files:**

- Create: `plugins/bruh/agents/agents_test.mjs`
- Modify: `.github/workflows/ci.yml` (one job `node`)

**Interfaces:**

- Consumes: interfaces section 2 (the tool names in the line "Existing tools (plan 1)" and in the first column of the "New tools" table), the headers of spec 5.
- Produces: `node --test plugins/bruh/agents/agents_test.mjs`.

- [ ] **Step 1: Write the failing test.** The test reads the interfaces file and collects the tool names: the backticked names in the line that starts with "Existing tools", and the backticked first cell of each row of the "New tools" table. For each `agents/*.md` it parses the frontmatter (lines between the first two `---` lines, `key: value` pairs) and checks: `name` equals the file name without `.md` and has no colon; `description` is not empty; `model` matches `^(opus|sonnet|fable)\[1m\]$`; `effort` is one of `low`, `medium`, `high`, `xhigh`, `max`. It collects each backticked token of the body that matches `^(mail|handoff|answer|report|role_settings|lease|question|session|init|bruh)_[a-z_]+$` and is not an argument name (`session_id`, `question_id`), and checks that each is a known tool. It checks a required tool list for each role. It checks that `clanker.md` has `disallowedTools` with `Edit`, `Write`, and `NotebookEdit`. It checks that each header in the text that starts with `P0 Q-`, `ANSWER Q-`, and so on matches the header grammar. It checks the files of the ledger template and the keys of `mode.md`.
- [ ] **Step 2: Run the test to verify that it fails.** Run: `node --test plugins/bruh/agents/agents_test.mjs`. Expected: FAIL, because `agents/bigm.md` does not exist.
- [ ] **Step 3: Add the CI job** `node` (ubuntu-latest and macos-latest, `actions/setup-node@v4` with Node 22) that runs `node --test plugins/bruh/agents/agents_test.mjs plugins/bruh/workflows/deliver.test.mjs`. Plan 4 adds the second file; until then, the job runs only the first file.
- [ ] **Step 4: Commit** `Add the structural check of the role agents`.

### Task 2: Defaults: priorities and house rules

**Files:**

- Create: `plugins/bruh/defaults/priorities.md`
- Create: `plugins/bruh/defaults/house-rules.md`

**Interfaces:**

- Produces: `priorities.md` with the sections `P0`, `P1`, `P2` (spec 14.1, owner decision 2026-09-27), `Delegated P1 classes` (spec 14.1, owner decision 2026-09-29; empty by default, the init skill writes the answer `delegated_p1_classes` as one list item each), `Never without the owner` (spec 13), and `Deny rules` (the exact command patterns that `defaults/role-settings.json` carries).
- Produces: `house-rules.md`, the text that the clanker passes as `args.house_rules` to `/bruh:deliver`. Derived from spec 2, spec 6.3, and the field lessons of knowledge.md.

- [ ] **Step 1:** Extend the test: `priorities.md` has the five section headings; `house-rules.md` exists and names the base SHA rule and the skipped-test rule.
- [ ] **Step 2:** Run the test. Expected: FAIL.
- [ ] **Step 3:** Write both files.
- [ ] **Step 4:** Run the test. Expected: the defaults subtests pass.
- [ ] **Step 5: Commit** `Add the default priorities and house rules`.

### Task 3: Ledger template

**Files:**

- Create: `plugins/bruh/ledger-template/README.md`, `mode.md`, `priorities.md`, `rules.md`, `grants.md`, `questions.md`, `owed.md`, `leases.md`, `projects/_template.md`

**Interfaces:**

- Produces: the layout of spec 8. `mode.md` holds one `key: value` line for each of `mode` (`human`), `changed`, `reason`, `p1_batch_minutes` (60), `p1_batch_size` (5), `review_round_cap` (2), `status_cadence` (`on-change`). The init tool copies the folder verbatim and then replaces the value of a `key:` line with the init answer of the same key (interfaces section 5). `priorities.md` is a note; the init tool replaces it with `defaults/priorities.md`. The project template has the sections of spec 8.1.

- [ ] **Step 1:** Extend the test: every file of the layout exists, `mode.md` has each key, and `projects/_template.md` has each section of spec 8.1.
- [ ] **Step 2:** Run the test. Expected: FAIL.
- [ ] **Step 3:** Write the files.
- [ ] **Step 4:** Run the test. Expected: the template subtests pass.
- [ ] **Step 5: Commit** `Add the ledger template`.

### Task 4: The clerk agent

**Files:**

- Create: `plugins/bruh/agents/clerk.md`

**Interfaces:**

- Frontmatter: `name: clerk`, `model: opus[1m]`, `effort: medium`. No `tools` list, because workflow agents inherit the tool pool of the main session (sub-agents.md, "Available tools"), and a `deliver` implementer needs `Edit` and `Write`.
- Procedure: start (`mail_read`, `bruh_info`, check the base SHA, make its own worktree), launch `/bruh:deliver` with the `args` of interfaces section 4, the question path of spec 6.2 (`answer_write`, relaunch with `resumeFromRunId` and `args.answers`), the usage-limit rule (spec 15), overlaps (spec 3.6), the push, the evidence report (`report_write` with a source), `DONE` to the clanker, the merge under a grant (`scripts/merge-train.sh`), the removal of the worktree, the stop. The ledger clerk procedure (role key `clerk-ledger`). The handoff protocol (spec 7).

- [ ] **Step 1:** Run the test. Expected: FAIL for `clerk.md`.
- [ ] **Step 2:** Write `clerk.md`.
- [ ] **Step 3:** Run the test. Expected: the clerk subtests pass.
- [ ] **Step 4: Commit** `Add the clerk agent`.

### Task 5: The clanker agent

**Files:**

- Create: `plugins/bruh/agents/clanker.md`

**Interfaces:**

- Frontmatter: `name: clanker`, `model: opus[1m]`, `effort: high`, `disallowedTools: Edit, Write, NotebookEdit`.
- Procedure: start message, tasks, the clerk start message of spec 3.5, `role_settings_write`, `mail_post`, `session_launch`, the caps of spec 3.7, routing of spec 14.2 (`priorities.md` read again, final P-level, delegated classes, `REC`), `ANSWER` to the clerk that asked with a copy through `report_write`, leases (`lease_request`, `lease_grant`, `lease_release`, `lease_list`), liveness of its clerks (`session_list`, `session_resume`), acceptance of a result, merges, the remote form (Orca `ask`, `send`), the handoff protocol.

- [ ] **Step 1:** Run the test. Expected: FAIL for `clanker.md`.
- [ ] **Step 2:** Write `clanker.md`.
- [ ] **Step 3:** Run the test. Expected: the clanker subtests pass.
- [ ] **Step 4: Commit** `Add the clanker agent`.

### Task 6: The bigm agent

**Files:**

- Create: `plugins/bruh/agents/bigm.md`

**Interfaces:**

- Frontmatter: `name: bigm`, `model: opus[1m]`, `effort: high`. No tool limits: bigm writes the ledger, and it needs `CronCreate`, `Monitor`, and `SendMessage`.
- Procedure: the start of each turn (`mode.md`, `session_list`, `mail_read`, the reboot check), the sweep (`CronCreate` every 15 minutes, created again every 6 days), the watcher `Monitor` and the Orca receive loop after a resume, the owed list, questions (P0 at once, P1 batches from `mode.md`), human and autonomous mode, the never-without-the-owner hard stop, rules and `RULE`, the status report of spec 9.4, the failure rules of spec 15, merge grants, leases, remote clankers through Orca (spec 4.2), the ledger (only writer, a commit after each change, `clerk-ledger` pushes), the handoff protocol.

- [ ] **Step 1:** Run the test. Expected: FAIL for `bigm.md`.
- [ ] **Step 2:** Write `bigm.md`.
- [ ] **Step 3:** Run the full test and every check of lane-common.md. Expected: all pass.
- [ ] **Step 4: Commit** `Add the bigm agent`.

---

## Deviations from the specification

1. The agent files use `bruh_info` to find the plugin scripts, as the interfaces file says. The docs say that Claude Code also substitutes `${CLAUDE_PLUGIN_ROOT}` in agent content (plugins-reference.md, "Where each variable resolves"). The interfaces file says the opposite ("agent text gets no plugin path"). The agents follow the contract. Reported to the controller.
2. `mode.md` has the key `status_cadence` with the values `on-change` and `always`. Spec 9.4 names the setting but not the key. Agent-derived, needs owner decision.
3. The structural check is a `node --test` file, not a `bats` test. Node 22 is already in CI for `claude plugin validate`.

## Decisions (agent-derived, needs owner decision)

1. **Model of each role.** Options, ranked: (a) `opus[1m]` for all three roles (chosen: the roles route, judge, and decide, and the value selects the 1M window on every provider); (b) `opus[1m]` for bigm and the clanker, `sonnet[1m]` for the clerk, to cost less; (c) `inherit`, which follows the model of the session and can be a 200K model.
2. **Effort of each role.** Chosen: bigm `high`, clanker `high`, clerk `medium`. The clerk does little reasoning of its own; its workflow agents set their own effort (spec 3.3).
3. **Tool limits.** The clanker gets `disallowedTools: Edit, Write, NotebookEdit` (spec 3.5 "writes no file"). bigm and the clerk get no limits: bigm writes the ledger, and a limit on the clerk also limits its workflow agents, because subagents inherit the tool pool of the main session. A Bash command can still write a file, so the limit is a speed bump (spec 2, principle 2).
4. **Reboot threshold.** bigm treats a change to `failed` or `stopped` of at least two sessions and at least half of the known sessions between two reconciles as a reboot.
5. **The ledger clerk** (spec 22, open question 1). bigm keeps one long-lived `clerk-ledger` session in the ledger folder. After each commit, bigm sends it `DONE: ledger commit <short SHA>` (a `DONE` header, because spec 5 allows no other header for this). The ledger clerk pushes and writes the result with `report_write`.
6. **Where a clerk reads a question body.** The workflow agent sends only the header to `main`. The clerk reads the body from `<data_dir>/questions/<id>.json` (`data_dir` from `bruh_info`), because the interfaces file has no `question_read` tool.
7. **The sweep record.** bigm keeps the ID and the UTC creation time of its sweep task in its handoff (section "State"), because `CronList` does not show the creation time.
8. **Last P1 batch time.** bigm keeps the line `last_batch: <UTC>` at the top of `questions.md`.

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| Agent frontmatter fields: `name` (required, no colon), `description` (required), `tools`, `disallowedTools`, `model`, `effort`, and others | Read the page | <https://code.claude.com/docs/en/sub-agents.md>, "Frontmatter reference" |
| `model` accepts `sonnet`, `opus`, `haiku`, `fable`, a full model ID ("the same values as the `--model` flag"), or `inherit` | Read the page | sub-agents.md, "Choose a model" |
| `opus[1m]` and `sonnet[1m]` are model aliases that select the 1M context window | Read the page | <https://code.claude.com/docs/en/model-config.md>, "Model aliases" |
| `effort` values: `low`, `medium`, `high`, `xhigh`, `max` | Read the page | sub-agents.md, "Frontmatter reference" |
| With `--agent`, the main thread takes on the tool restrictions and the model of the agent, and the body replaces the system prompt | Read the page | sub-agents.md, "Invoke subagents explicitly" |
| Subagents inherit the built-in and MCP tools of the main conversation | Read the page | sub-agents.md, "Available tools" |
| `${user_config.KEY}` is substituted in agent content (non-sensitive values only) | Read the page | <https://code.claude.com/docs/en/plugins-reference.md>, "User configuration" |
| `${CLAUDE_PLUGIN_ROOT}` resolves anywhere in the Markdown body of agent content | Read the page | plugins-reference.md, "Where each variable resolves" |
| A plugin loads agent `.md` files from `agents/` and workflow `.js` files from `workflows/`, so a `.mjs` test file in these folders is not a component | Read the page | plugins-reference.md, "File locations reference" |
| `CronCreate` takes a 5-field cron expression; a recurring task expires after 7 days; tasks are restored on `--resume`; `Monitor` and background Bash tasks are not | Read the page | <https://code.claude.com/docs/en/scheduled-tasks.md> |
| `SendMessage` addresses a local session by its name; the receiver drops identical repeats within a short window | Read the page | <https://code.claude.com/docs/en/cross-session-messaging.md> |
| Orca: `ask --resume <message_id>` asks again after a timeout; `check --wait --timeout-ms <n>`; `reply --id <msg_id> --body <text>`; `send --to <address> --subject <text> --body <text> --type <type>`; `worker-list`; `run-create --objective <text>` | `orca orchestration <command> --help` | Orca CLI |
| The mailbox refuses a header that is not in the grammar of spec 5 | Read `mail.go` (`headerRE`) | `plugins/bruh/mcp/mail.go` |
| `role_settings_write` merges the default deny rules of `defaults/role-settings.json` | Read `roles.go` | `plugins/bruh/mcp/roles.go` |

No live probe was necessary for this plan.

## Self-review

- **Spec coverage:** 2 (all roles, principles), 3.3 to 3.7 (frontmatter, procedures), 4.1 and 4.2 (bigm and clanker), 5 (headers, attempt counter), 7 (handoff protocol in each agent), 8 (ledger template, bigm), 9 (bigm), 11 and 12 (bigm), 13 and 14 (priorities.md, routing in clanker and bigm), 15 (bigm, clanker, clerk).
- **Placeholders:** the ledger template has default values only; the init tool replaces `key:` values and `priorities.md`, as the template README documents.
- **Review Focus:** each item has a subtest in `agents_test.mjs`.
