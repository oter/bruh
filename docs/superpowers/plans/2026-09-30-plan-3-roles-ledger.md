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
2. `mode.md` has the key `status_cadence` with the values `on-change` and `always`. Spec 9.4 names the setting but not the key. Agent-derived, needs owner decision. Options, ranked: (a) `status_cadence: on-change | always`, one `key: value` line like the other runtime settings; (b) `status_report: change | every-sweep`; (c) a boolean `status_always: false`.
3. The structural check is a `node --test` file, not a `bats` test. Node 22 is already in CI for `claude plugin validate`.
4. A clerk that finds its clanker not running cannot call `session_resume`, because interfaces section 2 allows it only for the parent. Controller ruling, fix round 2: the clerk keeps its mail in the mailbox and writes the event `clanker-<project> not running; mail pending`; only a P0 question goes straight to bigm; bigm resumes each idle clanker that has unread mail or such an event, at each turn and each sweep.
5. The merger role key `clerk-<project>-merge` is stable, so that a grant can name it. Controller ruling, fix round 3: the merger clerk is not long-lived, because the owner decision of 2026-09-27 in spec 3.6 wins ("A clerk stops when its task is done. The next task gets a new clerk."). Each merge is one task and gets a new session under the stable key. This keeps spec 3.6 (see decision 11).

## Decisions

Each decision below is agent-derived, needs owner decision, unless it says "controller ruling, fix round 1": the controller ruled it on 2026-09-30, and it is still agent-derived, needs owner decision. The first option of each list is the chosen one.

1. **Model of each role.** Options, ranked: (a) `opus[1m]` for all three roles: the roles route, judge, and decide, and the `[1m]` alias selects the 1M window also on providers where `opus` alone gives 200K (model-config.md, "Model aliases" and "Extended context"); (b) plain `opus`, which has the 1M window on the Anthropic API for Opus 4.7 and later, but not on every provider; (c) `opus[1m]` for bigm and the clanker, `sonnet[1m]` for the clerk, to cost less; (d) `inherit`, which follows the session model and can be a 200K model.
2. **Effort of each role.** Options, ranked: (a) bigm `high`, clanker `high`, clerk `medium`: the clerk does little reasoning of its own, and its workflow agents set their own effort (spec 3.3); (b) `high` for all three roles; (c) no `effort` field, so each role inherits the session effort.
3. **Tool limits.** Options, ranked: (a) only the clanker gets `disallowedTools: Edit, Write, NotebookEdit` (spec 3.5 "writes no file"); bigm writes the ledger, and a limit on the clerk also removes the tools of its workflow agents, because subagents inherit the tool pool of the main session; (b) also a `tools` allowlist for bigm that keeps `Edit` and `Write` for the ledger only (not possible: a tool limit has no path scope); (c) no limits, only the prose rule. A Bash command can still write a file, so each limit is a speed bump (spec 2, principle 2).
4. **Reboot threshold.** Options, ranked: (a) at least two sessions and at least half of the known sessions changed to `failed` or `stopped` between two reconciles; (b) all long-lived sessions changed at once; (c) any two sessions changed at once.
5. **The ledger clerk** (spec 22, open question 1). Controller ruling, fix round 1: its own start procedure, no `EnterWorktree`, and it pushes the current branch of the ledger checkout after each commit of bigm. Options, ranked: (a) one long-lived `clerk-ledger` in the ledger checkout, nudged with `DONE: ledger commit <short SHA>` after each commit; (b) a new `clerk-ledger` session for each push; (c) bigm pushes the ledger itself (against "clerks do all pushes", owner decision 2026-09-30).
6. **Where a clerk reads a question body.** Options, ranked: (a) from `<data_dir>/questions/<id>.json`, because the interfaces file has no `question_read` tool; (b) a new MCP tool `question_read` (a contract change for lane go); (c) the workflow agent sends the body in the `SendMessage` too (against spec 5, "the nudge is ... only the header line").
7. **The sweep record.** Options, ranked: (a) bigm keeps the ID and the UTC creation time of its sweep task in the "State" section of its handoff, because `CronList` does not show the creation time; (b) a line in `mode.md` (it holds owner settings, not bigm state); (c) delete and create the task again at each start of bigm, so its age is never more than the time since the last start.
8. **Last P1 batch time.** Options, ranked: (a) the line `last_batch: <UTC>` at the top of `questions.md`; (b) a line in `mode.md`; (c) the handoff of bigm only (lost when bigm writes no handoff before a crash).
9. **Start message header.** Controller ruling, fix round 1, and interfaces section 4a: `START: <subject>`. Options, ranked: (a) `START: <subject>`; (b) `DONE: start message for <key>` (the first version; a clerk can read it as an acceptance).
10. **Acceptance of a clerk result.** Controller ruling, fix round 1. Options, ranked: (a) only a message from the clanker with the header `DONE: result accepted for <task>` and the first body line `accepted: <head SHA>`, where the SHA is the delivered head SHA; (b) any `DONE` message about the task from the clanker; (c) an `ANSWER` to a question "accept?" that the clerk opens.
11. **Merges and grants.** Controller ruling, fix rounds 1 and 3. Options, ranked: (a) a stable merger role key `clerk-<project>-merge` for each project that a grant names; each merge is one task: the clanker starts a new session under the key for each merge, only when no live session has the key, with one `START: merge <owner/repo>#<n>` request and a cover (the grant, or the owner `ANSWER`); the session runs `merge-train.sh <repo> <pr>` with only its pull request numbers, confirms through the code host API, reports, and stops. This option keeps the owner decision of 2026-09-27 in spec 3.6; (b) a long-lived merger clerk under the same key (the fix round 1 version; it conflicts with spec 3.6); (c) each task clerk merges its own pull request (a grant cannot name a task clerk, because its key does not exist when the owner gives the grant); (d) the clanker merges (against "clerks do all pushes").
12. **Busy-clerk cap.** Controller ruling, fix round 1. Options, ranked: (a) count the task clerks whose `state` is not `done`, `failed`, or `stopped`, because a clerk that waits for its workflow run is between turns but still works; (b) count `status` equal to `busy` (the first version; it never binds while clerks wait for workflows); (c) count the open workflow runs from the report files.
13. **Liveness before a nudge from a clerk.** Controller ruling, fix round 2 (M5, N4), within the contract (see deviation 4). Options, ranked: (a) the clerk always writes the mailbox; for a receiver that is not running it writes the event `<role key> not running; mail pending`, and only a P0 question also goes to bigm; bigm resumes idle clankers with work at each turn and sweep, with no page to the owner; (b) the clerk sends a P0 to bigm for each idle clanker (the fix round 1 version; it paged the owner about once an hour); (c) the clerk calls `session_resume` (refused by the contract: parent only).
14. **Remote clanker start.** Controller ruling, fix round 1, and interfaces section 4a. Options, ranked: (a) the Orca spec text has the launch steps (run `go run -C <plugin root>/mcp . role-settings clanker-<project>`, then start the clanker) and the full start message; the remote clanker reports through `orca orchestration send`; (b) bigm sends the start message after the launch with `orca orchestration send`; (c) the remote init skill writes a start message into the remote mailbox.
15. **Telegram sends.** Controller ruling, fix round 1. Options, ranked: (a) the `reply` tool of the official Telegram channel plugin with the `chat_id` of the latest inbound message of the owner; without an inbound message, the P0 stays in the terminal and on `owed.md`; (b) a `chat_id` from the init answers (not in interfaces section 5); (c) terminal only.
16. **Rule broadcast.** Options, ranked: (a) bigm sends a `RULE` to the clankers and to `clerk-ledger`, each clanker sends it to its clerks, and each role ignores a rule ID that it already applied; (b) bigm sends it to every session of `session_list` (the first version; each clerk got it two times).
17. **Mode changes.** Options, ranked: (a) when the mode changes, bigm sends `DONE: mode is now <mode>` with the text of `mode.md` to each clanker; (b) a `RULE` message (a mode is not an owner rule); (c) the clankers read `mode.md` themselves (a remote clanker cannot).
18. **A missing hard-stop list.** Options, ranked: (a) when `priorities.md` of the ledger has no section "Never without the owner", bigm starts no work and raises a P0; (b) bigm copies the default of the plugin itself (bigm then writes a file that the init skill owns); (c) no check.
19. **Retry after a failed run.** Controller ruling, fix round 2 (N3). Options, ranked: (a) the clerk schedules its own retry with a one-shot `CronCreate` at the reported reset time, else in 15 minutes, relaunches with `resumeFromRunId` and the stored `args`, at most 3 retries per task, and after the third sends a P0 through its clanker; (b) the clanker nudges the clerk after the reset; (c) bigm nudges the clerks at the sweep from the `five_hour_percentage` files.
20. **The receive path of a remote clanker.** Controller ruling, fix round 2 (M3, m11). Options, ranked: (a) the remote clanker runs its own `orca orchestration check --wait` loop as a background command, restarts it after each batch and each resume, and acts on work, `RULE`, and `DONE: mode is now <mode>`; (b) bigm puts every later message into a new Orca dispatch; (c) the remote clanker polls `orca orchestration check` at each turn only.
21. **Mode changes at a local clanker.** Controller ruling, fix round 2 (m11). Options, ranked: (a) the clanker reads the mode from `DONE: mode is now <mode>` and puts it in the start message of each new clerk; running clerks keep their start message; (b) the clanker also sends the mode to its running clerks; (c) the clanker reads `mode.md` itself.
22. **The ledger clerk check of a commit.** Options, ranked: (a) `git merge-base --is-ancestor <short SHA> HEAD`, so that two commits of bigm before one push pass; (b) `git log -1` starts with the short SHA (the first version; it failed on back-to-back commits).
23. **The reserved task name `merge`.** Options, ranked: (a) the clanker never gives a task the name `merge`, so that `clerk-<project>-merge` is always the merger clerk; (b) a merger key outside the clerk grammar, for example `merger-<project>` (a change of interfaces section 1).
24. **The source of a merge cover.** Options, ranked: (a) the merger clerk reads the grant row in `grants.md` or the question row of the project file of the ledger, from the ledger path of the request; on a remote machine, the Orca reply of bigm that the request names; (b) the body of the request only.
25. **The ledger clerk start message.** Options, ranked: (a) it has the text of `priorities.md` and `rules.md`, like every other start message; (b) no priorities text, and a rule exception for the ledger clerk.

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
| The official Telegram channel plugin has the tool `reply` with the required arguments `chat_id` and `text` and the optional `reply_to` (a message ID); it refuses a chat that is not on its allowlist; it returns the ID of the sent message; an inbound message arrives as a `<channel source="telegram" chat_id="..." message_id="...">` block | Read the source of the installed plugin (`server.ts` of `telegram` 0.0.7) | Telegram channel plugin of `claude-plugins-official` |
| The mailbox refuses a header that is not in the grammar of spec 5 | Read `mail.go` (`headerRE`) | `plugins/bruh/mcp/mail.go` |
| `role_settings_write` merges the default deny rules of `defaults/role-settings.json` | Read `roles.go` | `plugins/bruh/mcp/roles.go` |

No live probe was necessary for this plan.

## Self-review

- **Spec coverage:** 2 (all roles, principles), 3.3 to 3.7 (frontmatter, procedures), 4.1 and 4.2 (bigm and clanker), 5 (headers, attempt counter), 7 (handoff protocol in each agent), 8 (ledger template, bigm), 9 (bigm), 11 and 12 (bigm), 13 and 14 (priorities.md, routing in clanker and bigm), 15 (bigm, clanker, clerk).
- **Placeholders:** the ledger template has default values only; the init tool replaces `key:` values and `priorities.md`, as the template README documents.
- **Review Focus:** each item has a subtest in `agents_test.mjs`.

## Final review fixes (2026-09-30)

The final whole-branch review found 1 blocker, 7 major, and 17 minor findings. The rulings of the controller for the findings of this plan are below. Each is agent-derived, needs owner decision. The first option of each list is the one that is built.

1. **M1, rule broadcast and sender checks.** Premise changed: `mail_post` now accepts a `RULE` only from bigm (plan 2, final review fix 2), so decision 16 (a) ("each clanker sends it to its clerks") cannot work. Decision 16 becomes: bigm sends each `RULE` to each running local clanker, each running local clerk (task and merger clerks), and `clerk-ledger`. A remote clanker gets the rule through Orca and relays it to its clerks with the header `DONE: rule R-<n>: <subject>`; a clerk accepts that form only from its own clanker. A clanker and a clerk apply a `RULE` only when its `from` is `bigm`, and a clerk writes an `ANSWER` only when its `from` is its clanker or `bigm`. Options, ranked: (a) as built; (b) `mail_post` also accepts a `RULE` from `Parent(receiver)`, so the clanker forwards it as before on every machine; (c) remote clerks get new rules only in the start message of the next clerk.
2. **M2, the merge approval header.** bigm sends a yes to a merge question with exactly `ANSWER Q-<n>: merge <owner/repo>#<pr>[,#<pr>...] approved` and a no with `ANSWER Q-<n>: merge <owner/repo>#<pr> refused`, and puts the words of the owner in the body. The clanker and the merger clerk treat only the approval form that names the pull request as a cover. Decision 11 (a) stays; its cover is now this header. Options, ranked: (a) the header form, as built, because the merge gate reads it (plan 5, final review fix 1); (b) a first body line `approve: <owner/repo>#<n>`; (c) free text, as before.
3. **M3, remote merges.** The merger clerk of every project, local or remote, runs on the machine of bigm, because a merge is a call of the code host API and needs no checkout. A local clanker starts its merger clerk as before. A remote clanker does not: it sends `P1 Q-<n>: merge <owner/repo>#<n>?` to bigm with `orca orchestration ask`, with the grant and the source reads of its conditions when a grant covers it. bigm finds the cover (a grant row, or the answer of the owner), writes the role settings, posts the approval `ANSWER` and the `START: merge ...` request to `clerk-<project>-merge`, and calls `session_launch` in the ledger folder. The merger clerk started by bigm writes its result with `report_write` and sends no `DONE` (a `DONE` from a clerk to bigm is refused by `mail_post`); bigm reads it, checks the merge at the code host, and sends `DONE: merged ...` to the remote clanker through Orca. Premise changed: decision 24 (a) named "the Orca reply of bigm" as the cover source on a remote machine; the merger now always runs where the ledger is, so decision 24 becomes: the source is always `grants.md` or the project file of the local ledger. Options, ranked: (a) as built; (b) the remote clanker starts a remote merger that accepts an approval relayed by the clanker on a machine marked remote (option 1 of the review); (c) remote merges stay a manual owner action.
4. **M4, question IDs across machines.** bigm keys a question by the pair (asker role key, question ID), never by the ID alone, because the `question_open` counter is local to each machine. The ledger records a question of a remote clanker as `<orca environment>/Q-<n>`; message headers keep the plain `Q-<n>` of the asker, so the header grammar does not change. Options, ranked: (a) the pair and the ledger prefix, as built (option 3 of the review, the ruling of the controller); (b) bigm maps each remote question to a new local ID with `question_open` and keeps a map (option 1 of the review); (c) a machine part in the ID, `Q-<env>-<n>`, a change of the header grammar and the spec.
