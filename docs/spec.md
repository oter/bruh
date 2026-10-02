# bruh specification

Status: version 0.6 is a draft for the review of the owner (2026-10-02). The owner approved version 0.5 on 2026-09-30. Section 25 lists the changes from version 0.5, and section 24 the changes from version 0.4.

This specification is the input for the implementation plan. The decision log is in [design.md](design.md). The verified facts and the field lessons are in [knowledge.md](knowledge.md). The process is shown as diagrams in [flow.md](flow.md). Each decision in this file has one tag:

- "Owner decision <date>": the owner made the decision.
- "Owner decision <date> (delegated ruling)": the owner gave the decision to a subagent, and the subagent made it.
- "Owner decision <date> (delegated to the agent)": the owner told the agent to decide, and the agent made the decision.
- "Agent-derived, accepted 2026-09-30": the agent proposed it, and the owner accepted all such items as a set on 2026-09-30, to test them in practice.
- "Agent-derived, needs owner decision": the agent proposed it, and the owner has not decided it.
- "Verify": a fact that the implementation must prove with a test before it depends on it.

The field evidence comes from two sources: the setup of the owner (a 4-day run of a coordinator, lane sessions, clerk subagents, and review workflows), and a retrospective of the same run by a second engineer. Section 23 lists the changes from version 0.3.

## 1. Purpose and scope

bruh is a Claude Code plugin. It lets one person run work in many projects on many machines through a hierarchy of Claude Code sessions. The person talks to one session, bigm. bigm keeps the status of all work and answers questions about it. It shows the person only the questions that need the person. Owner decision 2026-09-27.

- bruh runs in two modes: with a human, and fully autonomous (section 11). Owner decision 2026-09-29.
- bruh runs the same way on a local machine and in a container (section 10.2). Owner decision 2026-09-30.
- Version 0.1 contains all parts of this specification. Nothing moves to a later version to cut scope. Owner decision 2026-09-30.
- bruh learns the projects of the owner before the owner asks for work (section 8.5). Owner decision 2026-10-02.

## 2. Principles

These rules apply to every role. Each is agent-derived, accepted 2026-09-30, and comes from the field evidence, except where tagged.

1. **A status is true only when it was just read from its source.** A report or a ledger row that says merged, deployed, live, down, or out of quota carries its source read: the command or API call, the value it returned, and a UTC time. bigm reads the source again before it tells the owner, or it shows the claim as "unverified".
2. **Mechanical stops, not notes.** A rule that must never break is a deny rule, a hook, or a sandbox rule, not a sentence in a prompt or a memory note. A Bash deny rule matches only the command text, so another form of the same program gets past it. A deny rule is a speed bump. Where a real stop is necessary, bruh uses the sandbox. When the same lesson is recorded a second time, bigm raises a P1 that proposes a mechanical stop for it.
3. **Plugin code writes the files.** All bruh files are in the plugin data folder, and plugin code writes them (section 10). Owner decision 2026-09-29. The files of the learn step are in the ledger, and plugin code writes them too (section 8.5). Owner decision 2026-10-02.
4. **Timestamps come from code.** A time in the ledger, a handoff, or a message comes from `date -u` or from the MCP server, never from the model.
5. **Rules are kept word for word.** An owner rule is stored with its words, its date, and its source. A rule with no source is a recommendation, not a rule.
6. **A refusal is escalated, never handed on.** A permission prompt or a classifier refusal goes to the owner. No other session runs the refused command.
7. **The owner terminal is for the owner.** No role types into the input of the owner.

## 3. Roles

| Role | Lifetime | Started by | Does hands-on work |
|---|---|---|---|
| bigm | Long-lived | The owner | No |
| Clanker | Long-lived | bigm | No |
| Clerk | One task | A clanker | No |
| Workflow | One run | A clerk | Yes |

Owner decision 2026-09-27: the four roles, their names, the spawn chain, the lifetimes, and the hands-on column for clankers, clerks, and workflows. The hands-on value for bigm is agent-derived, accepted 2026-09-30. The setup of the owner supports it: its coordinator had the rule "no hands".

Each role runs as a plugin agent: `--agent bruh:bigm`, `--agent bruh:clanker`, `--agent bruh:clerk`. The role instructions are then the system prompt, which reloads after compaction. In the setup of the owner, the "no hands" rule faded after compaction because it was only in the conversation. Agent-derived, accepted 2026-09-30. Verify: `--agent` accepts the namespaced name of a plugin agent.

### 3.1 Launch settings of every role

Every role starts with these flags. Agent-derived, accepted 2026-09-30, except where tagged.

- `--permission-mode auto`. A plugin agent ignores `permissionMode` in its definition, so the mode is set at launch. All sessions of the setup of the owner ran in `auto` mode, and no message was held. Owner decision 2026-09-30.
- `--settings <file>`, a settings file for the role that the parent writes through the MCP server. It holds `env.BRUH_ROLE_KEY` (section 3.2), the deny rules of section 13, and the tool account variables of section 4.1. Since version 0.6, the file of a clanker also holds read-only allow rules for the other repositories of its project (section 8.5; agent-derived, needs owner decision). The `--settings` flag carries through the restarts of a background session, but variables exported in the shell do not reach a background session.

### 3.2 Role key

Each role has a stable role key: `bigm`, `clanker-<project>`, `clerk-<project>-<task>`, and `clerk-ledger` for ledger pushes. bigm assigns the clanker keys, and a clanker assigns the clerk keys. The ledger maps each role key to the current session ID and session name. Agent-derived, accepted 2026-09-30.

- Handoffs, mailboxes, leases, and ledger rows use the role key, not the session name and not the session ID. In the setup of the owner, a reboot renamed every session, and `/clear` changed a session ID.
- Hooks and the MCP server read the role key from `BRUH_ROLE_KEY`. When the variable is not set, a bruh hook does nothing. So the manual sessions of the owner are not affected.
- After a restart, the parent reads `claude agents --json --all` and updates the map.

### 3.3 Models

- All roles run on models with a 1M context window, so that the auto-compact window of section 7 gives 55 percent. Agent-derived, accepted 2026-09-30.
- A role never changes to a different model on its own. A model change is a P0. The setup of the owner had the rule "never substitute the model; on a usage limit, pause and report". Agent-derived, accepted 2026-09-30.
- The agent definition of each role sets its model and effort. Workflow agents get per-stage defaults: reviewers at low effort, implementers at high effort. Agent-derived, accepted 2026-09-30.

### 3.4 bigm

- The owner starts bigm in the folder of the private ledger repository:

  ```bash
  claude --agent bruh:bigm --name bigm --permission-mode auto \
    --settings <plugin data folder>/roles/bigm.json \
    --channels plugin:telegram@claude-plugins-official
  ```

  For remote work, bigm runs in an Orca terminal. A channel runs only when the session starts with `--channels`. Agent-derived, accepted 2026-09-30.
- bigm starts a clanker for each project that has work. It starts as many clankers as necessary and does not rotate them. Owner decision 2026-09-27. The concurrency caps of section 3.7 apply. Agent-derived, accepted 2026-09-30.
- bigm is the only writer of the ledger, except the files of the learn step (section 8.5), which plugin code writes. bigm commits after each change. The setup of the owner confirmed that one writer kept the ledger consistent. Agent-derived, accepted 2026-09-30. The exception for the learn step: owner decision 2026-10-02. bigm is also the only role that commits the files of the learn step: agent-derived, needs owner decision.
- bigm runs the sweep of section 9.1 and reconciles all sessions at each sweep and at the start of each turn. Agent-derived, accepted 2026-09-30.
- bigm answers status questions from the ledger. It reads the source again for each claim of principle 1. Agent-derived, accepted 2026-09-30.
- bigm keeps an "Owed to owner" list. Each ask of the owner, and each item that bigm owes the owner, goes on the list before bigm acts or relays. In the field run, two such items were lost. Agent-derived, accepted 2026-09-30.
- bigm shows P0 and P1 questions to the owner in its terminal, and through a channel (section 12). Owner decision 2026-09-27 (terminal) and 2026-09-29 (channels).
- bigm keeps the lease table of the clankers (section 8.4). Owner decision 2026-09-29.
- bigm stays an interactive session. It is never sent to the background, because a background session moves into a worktree before it edits files. Agent-derived, accepted 2026-09-30.

### 3.5 Clanker

- A clanker works in one project folder and knows the full picture of that project. It does no hands-on work. Owner decision 2026-09-27.
- A clanker starts from the stored knowledge of its project and of the linked projects (section 8.5). It reads the code for its task, and the gates that the knowledge marks `unknown` or `convention`. Owner decision 2026-10-02 (delegated ruling): the clanker reads these gates. The rest is agent-derived, needs owner decision.
- A clanker writes no file. Its handoff and its reports go through the bruh MCP server. Agent-derived, accepted 2026-09-30.
- A clanker divides the work into tasks and starts one clerk for each task. Owner decision 2026-09-27.
- The start message of a clerk contains: the task, the acceptance criteria, the project context that the task needs, the role key of the clanker, the text of `priorities.md` and `rules.md`, the base SHA, the files that the task will touch, and the known overlaps with other tasks. Agent-derived, accepted 2026-09-30.
- A clanker starts each clerk from the main checkout of the project, so that each clerk gets its own worktree. Agent-derived, accepted 2026-09-30. Since version 0.6, a project can have more than one repository, so the clerk starts from the main checkout of the repository that its task changes (section 8.5). Agent-derived, needs owner decision.
- A clanker answers P2 questions, and P1 questions of the delegated classes of `priorities.md`. It sends the other P1 questions and all P0 questions to bigm. Owner decision 2026-09-27 (P2) and 2026-09-29 (delegated classes).
- A clanker sends each answer to the clerk that asked, as an `ANSWER`, and writes a copy to its report file. Routine status goes only to the report file. In the setup of the owner, one coordinator sent about 469 messages to the owner session (an estimate), and only about 36 were decision asks. Agent-derived, accepted 2026-09-30.
- A clanker keeps the lease table of its clerks (section 8.4). Owner decision 2026-09-29.

### 3.6 Clerk

- A clerk owns one task. It starts workflows. The workflows do the work. Owner decision 2026-09-27.
- Clerks do all pushes. Owner decision 2026-09-30. A clerk pushes the branch of its task. The ledger clerk `clerk-ledger` pushes the ledger after bigm commits. Agent-derived, accepted 2026-09-30: the ledger clerk.
- A clerk sends its questions to its clanker, each with the P-level that the clerk thinks is correct. Agent-derived, accepted 2026-09-30.
- An overlap with a file that is not on the list of the start message means stop, and send a P2 to the clanker. On a shared instruction file, a clerk edits only its own lines. Agent-derived, accepted 2026-09-30.
- A clerk delivers a branch or a pull request. It merges only under a merge grant (section 8.3). Owner decision 2026-09-29.
- A clerk reports the result with evidence: the branch or the pull request, the base SHA and the head SHA, the test counts of section 6.3, and the review findings. Agent-derived, accepted 2026-09-30.
- A clerk stops when its task is done. The next task gets a new clerk. Owner decision 2026-09-27.

### 3.7 Concurrency

Agent-derived, accepted 2026-09-30.

- The plugin option `max_busy_clerks` caps the clerks that work at the same time. Default 8. In the setup of the owner, the operator judged that the setup became unstable at about 8 busy sessions or about 30 workflow agents. This was a judgment, not a measurement, so the default is a starting point.
- Each workflow run sets `CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` through the role settings file. Default 16, the Claude Code default.
- A clanker that reaches a cap queues the next task and records it in its report file.
- A clerk removes its worktree when the clanker accepts its result. In the setup of the owner, review worktrees filled 99.8 GiB of disk.

## 4. Transport

### 4.1 Local sessions

Owner decision 2026-09-27: local sessions talk through Claude Code itself, not through Orca. Owner decision 2026-09-30: local messages use the durable mailbox of the MCP server plus a `SendMessage` nudge. The details below are agent-derived, accepted 2026-09-30.

- The setup of the owner used `claude --bg` only twice. Its sessions were tabs that a person opened. So this mechanism needs a load test. Verify: 8 background sessions that exchange messages for one hour.
- Launch: the parent writes the start message to the mailbox and the role settings file, then runs this command in the project folder:

  ```bash
  CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --agent bruh:<role> \
    --name <role key> --permission-mode auto \
    --settings <plugin data folder>/roles/<role key>.json \
    "Read your start message with mail_read."
  ```

  The message body never goes on the command line. Verify: a session started this way from the Bash tool of another session appears in `claude agents --json`.
- Accounts: a background session uses the Claude account of the supervisor. Setting `CLAUDE_CONFIG_DIR` starts a separate supervisor with its own sessions, which the parent cannot see, so bruh never sets it. Tool accounts, for example `CODEX_HOME`, are pinned in the role settings file.
- Liveness: the supervisor stops an idle background session after about an hour. Before each nudge, the sender checks `pid` in `claude agents --json`. If the process is gone, the sender resumes the session with `claude --resume <session ID> --bg "Read your mailbox with mail_read."`. Verify: the resume continues under the same session ID.
- State: `waiting` is a `status` value that means between turns, not stuck. Only `waitingFor` equal to `permission prompt`, or a `state` of `failed` or `stopped`, is a problem. In the setup of the owner, a coordinator misread `waiting` as stuck. Verify: the values of `status` and `waitingFor`.
- Background roles do not show in Orca, because the Orca hook skips sessions that have `CLAUDE_JOB_DIR`. `claude agents` shows them.
- Workspace trust: `claude --bg` from a script fails in a folder that is not trusted. Each repository root is its own trust boundary, also inside a trusted folder (knowledge.md, "Workspace trust for test repositories"), and the docs give no way to trust a folder before its first interactive session. The trust step of the init skill (section 16) covers the selected repositories. Owner decision 2026-10-02.

### 4.2 Remote sessions

Owner decision 2026-09-27: remote sessions talk through the Orca remote runtime. Owner decision 2026-09-30: remote messages use Orca `send` and `ask`. The details below are agent-derived, accepted 2026-09-30.

- The mailbox and the report files are on one machine. So all traffic between bigm and a remote clanker goes through Orca: questions with `ask`, answers with `reply`, reports and `DONE` with `send`. Orca keeps the messages durable.
- On the remote machine: Claude Code, the bruh plugin, the init skill, and `orca serve` with `--pairing-address` set to an address on a private network.
- On the bigm machine: `orca environment add` pairs the runtime once. bigm runs in an Orca terminal and creates one Orca run with `orca orchestration run-create`. Verify: the orchestration commands work only from an Orca terminal.
- Launch: `orca orchestration worker-start --on <environment> --worktree new-top-level --repo <selector> --agent claude --spec "<short start message>"`. The start message tells the remote session to start `claude --agent bruh:clanker` with its role settings. Verify: the `--worktree` and `--repo` values for a remote environment, and how a remote session loads the plugin agent and its settings.
- Receive loop: bigm runs `orca orchestration check --wait` as a background command, and restarts it after a resume (section 9.1). Verify: it wakes bigm.
- Questions: the remote clanker sends P0 and P1 with `orca orchestration ask --timeout-ms <n>` and asks again with the same message ID after a timeout. Verify: the `--timeout-ms` option and the retry with the same message ID.
- The remote clanker starts its clerks on its own machine with the local mechanism of section 4.1, and uses the local mailbox of its own machine for them.

## 5. Messages

Agent-derived, accepted 2026-09-30.

- A local message goes to two places: the mailbox of the receiver, and a live nudge. The MCP tool `mail_post` writes the mailbox in the plugin data folder, keyed by role key. The nudge is a `SendMessage` with only the header line. The receiver reads the full message with `mail_read`. In the setup of the owner, 4 messages were missed, and 7 nudges that were typed into terminals were swallowed.
- The header line is one of these. `<id>` is a question ID, `Q-<number>`, unique in the ledger.
  - `P0 <id>: <subject>`, `P1 <id>: <subject>`, `P2 <id>: <subject>`
  - `ANSWER <id>: <subject>`
  - `REC <id>: <subject>`: a recommendation, not an owner answer.
  - `RULE <rule id>: <subject>`: a new standing rule for all sessions (section 9.3).
  - `DONE: <subject>`
- Only these headers are sent as messages. Routine status goes to the report file of the sender, which bigm reads at each sweep.
- An `ANSWER` from the owner quotes the words of the owner, with the date and the question ID.
- A question records its age and the work that it blocks.
- A retry adds an attempt counter to the nudge. In the setup of the owner, a repeated identical message was dropped as a duplicate. The layer that dropped it is not known. The receiver ignores a question ID that it already answered.
- Verify: how `SendMessage` behaves when the receiver is busy.

## 6. Workflows

### 6.1 General

- A workflow is a script for the Claude Code `Workflow` tool. Owner decision 2026-09-27.
- The plugin ships its workflows in `plugins/bruh/workflows/`. They run as `/bruh:<name>`. Agent-derived, accepted 2026-09-30. Verify: the Workflow tool accepts a plugin workflow, because it refuses a `scriptPath` outside the working directory.
- The clerk starts a workflow with a slash command in its start message. In the setup of the owner, a workflow needed the typed opt-in of the owner in each session, and a relayed opt-in did not count. Verify: a clerk started by a script can launch `/bruh:deliver`, and the allow rule `Workflow(bruh:deliver)` matches a namespaced plugin workflow. Agent-derived, accepted 2026-09-30.
- The script cannot write files. Agents and MCP tools do all writes.
- No agent inside a workflow posts outside the project, for example comments on a pull request. In the setup of the owner, an in-workflow post agent refused twice because it saw the latest chat message of the owner. Posting is clerk work and is outward-facing. Agent-derived, accepted 2026-09-30.

### 6.2 Questions during a run

- A workflow can get input while it runs. Owner decision 2026-09-27.
- An agent with a question sends it to `main`, which is the clerk, with a section 5 header, then calls the MCP tool `answer_wait`. The clerk answers it, or routes it through its clanker to bigm. The clerk writes the answer with `answer_write`. The two probes of 2026-09-27 proved this path in the same run (knowledge.md, "Probe results"). Owner decision 2026-09-29: plugin code writes the answer file. The tool names are agent-derived, accepted 2026-09-30.
- An MCP tool call aborts when the server sends no response and no progress notification for the idle window. So `answer_wait` sends a progress notification each minute, and the `.mcp.json` of the plugin sets a per-server `timeout` of 24 hours. `answer_wait` takes a deadline from `args` and returns `pending` when the deadline passes. Agent-derived, accepted 2026-09-30. Verify: the idle window for a stdio server, and that progress notifications keep the call alive.
- Fallback after `pending`: the workflow returns the question and ends. After the answer, the clerk launches it again with `resumeFromRunId`. The answer goes only into the prompts of the agents after the question, so the earlier agents return cached results. An agent whose prompt contains changed data runs again. Agent-derived, accepted 2026-09-30.

### 6.3 The `deliver` workflow

Agent-derived, accepted 2026-09-30, except where tagged. `deliver` adapts two scripts of the setup of the owner: `implement-tickets.js` for stages 1 and 2, and `review-and-fix.js` for stages 3 and 4.

Inputs in `args`: the task, the acceptance criteria, the base SHA, the gate commands of the project, the house rules file, the guides index of the project, the deliberate choices that reviewers must not flag, the answer folder and deadline, and the review-round cap.

1. **Plan.** Read the task, the code, and the guides of the project. Pin the base SHA. Write a plan with written STOP conditions for each step. Send each open question through section 6.2.
2. **Implement.** Implement the plan. Record deviations from the plan, and stop at a conflict instead of guessing.
3. **Review.** Three checks in parallel, each with the house rules in its prompt:
   - an adversarial reviewer that tries to refute the change;
   - an independent checker that verifies invariants against the code;
   - build, test, and lint with the gate commands. This check records how many tests ran, passed, failed, and were skipped. A skipped test in a required suite is a finding.

   Each review diffs against the pinned base SHA, never against `origin/main`. Findings are deduplicated by `file:line`. One refuter checks each finding. A finding that the refuter does not confirm counts as refuted, and a refuted finding stays refuted.
4. **Fix.** Fix the confirmed findings in batches by area, one batch at a time, because parallel fixers in one tree collide. Then review again.

- The review-round cap is a runtime setting with default 2. If findings are left after the last round, the run stops and the clerk raises a P1. Owner decision 2026-09-27 (two rounds) and 2026-09-29 (runtime setting).
- A failed step that costs money or cannot be undone is never retried automatically.
- The init skill adds the allow rule `Workflow(bruh:deliver)`.

### 6.4 The implement skillset

Owner decision 2026-09-30: the plugin includes the bruh-implement skillset of the setup of the owner, as the skill `/bruh:implement` and its workflows. The owner used it for every code change and every review in the field run. The details below are agent-derived, needs owner decision, except where tagged.

- The skill is the procedure of rule zero: the orchestrating session never edits code. Each change is a brief to an implementer agent and a review by a separate agent with the review guides, in a fix loop. For a settled spec with many changes, the pipeline is: review guides, function-sized TDD tickets, execution waves, lanes, a whole-branch gate, and a multi-lens review whose findings are refuted before they count.
- Workflows: `/bruh:tickets` (ticket breakdown), `/bruh:implement-tickets` (waves and lanes), `/bruh:review-and-fix` (review, refute, and fix of the own change), and `/bruh:review-only` (review and refute of a change that bruh did not write). Each takes its paths in `args`.
- Scripts: `scripts/lane.sh` (a private lane copy of the tree for a ticket, its patch, and its merge back) and `scripts/post-findings.sh` (posts a saved review result on the pull request or merge request).
- No workflow posts outside the project (section 6.1). A workflow returns its findings. A session saves the result with the MCP tool `result_save` and posts it with `post-findings.sh` after the owner approved the post (an `ANSWER` of bigm with the header `ANSWER Q-<n>: post <owner/repo>#<number> at <head SHA> approved`, or a yes in the terminal of a manual session), or under a post grant in `grants.md`. A post goes out under an account of the owner, so it is on the never-without-the-owner list (section 13). Each posted body starts and ends with the structural marker line of section 9.2.
- In a role session, `/bruh:implement` runs inside a clerk. In a manual session of the owner, the owner is the orchestrator and answers each ask in the terminal.
- The published files contain no project names, account names, host names, or ticket names of the setup of the owner.

## 7. Context budget and compaction

Owner decision 2026-09-27: every role stays below about 55 percent of its context window. The mechanism is design.md, open question 1, option 2. The configuration is in user settings. The setup of the owner supports the budget: the owner corrected sessions at 900k tokens and above. The details below are agent-derived, accepted 2026-09-30.

- Auto-compact window: the init skill writes `autoCompactWindow` into `~/.claude/settings.json`. The default is 550000, which is 55 percent of a 1M window.
- Status line tap: the init skill copies `statusline-tap.sh` into the plugin data folder and sets `statusLine.command` to the absolute path of the copy, followed by the previous status line command. The tap writes `context_window.used_percentage` and `rate_limits.five_hour.used_percentage` to `<plugin data folder>/context/<session_id>`. It writes nothing for a `null` value. Then it runs the previous command with the same input. Verify: the field `rate_limits.five_hour.used_percentage`.
- Verify: the status line runs in a background session that has no attached terminal. If it does not, the handoff message hook reads the token usage of the last assistant message from `transcript_path` instead. Verify: that usage field.
- Handoff message: a `PostToolUse` hook reads the context value. It exits at once when the input contains `agent_id`. At or above the plugin option `handoff_percent` (default 50), it tells the agent to update its handoff. It sends the message once for each crossing.
- Handoff file: `${CLAUDE_PLUGIN_DATA}/handoffs/<role key>.md`, written by the MCP tool `handoff_write`. Owner decision 2026-09-29: the plugin data folder, written by plugin code. The rules below are agent-derived, accepted 2026-09-30.
  - Each call replaces the whole file with one current state. History goes to `handoffs/<role key>.history.md`, which the pickup hook never injects.
  - Sections: role and role key, standing owner rules (word for word, with tags), goal, state, decisions, waiting on the owner, waiting on others (each with the exact IDs), next steps, files to read.
  - The MCP server stamps each write with system UTC time. It refuses a write that has only file pointers and no items, and a write that names a subagent ID or a `/tmp` path.
  - The file stays below 8,000 characters. The largest handoff of the setup of the owner was 9,851 characters, so the MCP server refuses a longer file and says so.
- Pickup: a `SessionStart` hook with the matchers `compact`, `clear`, and `resume` reads `BRUH_ROLE_KEY` and returns the handoff file of that role key. Its first line says: "The compaction summary is not a source. Rules come only from this handoff and from rules.md." It returns nothing when no file exists or when `BRUH_ROLE_KEY` is not set.
- After a pickup, the session reads the live source again for each pending item before it acts on it.
- In the setup of the owner, the handoffs were manual, and the 15 automatic compactions had none. The handoff message is the fix.

## 8. Ledger

Owner decision 2026-09-27: the ledger is Markdown in a separate private repository, and it describes all other repositories. The layout below is agent-derived, accepted 2026-09-30.

```text
README.md
mode.md               human or autonomous, and the runtime settings (section 11)
priorities.md         P-level definitions, delegated P1 classes, and the never-without-the-owner list
rules.md              standing owner rules, word for word, with dates
grants.md             merge grants (section 8.3)
questions.md          open P1 questions, oldest first, with age and what they block
owed.md               items owed to the owner, and asks of the owner
leases.md             the lease table of the clankers (section 8.4)
projects/<project>.md one file for each project
learn/tree.json       the hierarchy (section 8.5)
learn/projects/<key>.json  the knowledge of each project (section 8.5)
```

Version 0.6 adds `learn/`: owner decision 2026-10-02.

### 8.1 Project file

Agent-derived, accepted 2026-09-30.

Sections: Summary; In progress; Merged; Live; Decisions; Sessions (role key, session ID, session name, machine, state, compaction count); Identities (the account or identity of each credential, checked before its first write); Shared resources and clerk leases (section 8.4); Waiting on others.

- Each row has: owner, task, expected deliverable, state, next check (UTC), link, and the source read of principle 1. The setup of the owner used these columns, with the rules "a dispatch is a row" and "a report is an update".
- An owner action is a row too.
- Merged and Live are separate. bigm rebuilds In progress, Merged, and Live from git and the code host after each merge and at each sweep, and stamps each row with its own "as of" time. Agent-derived, accepted 2026-09-30. Since version 0.6, Merged and Live hold only the items since the last status report (section 8.6), so a rebuild reads only that window. Agent-derived, needs owner decision.
- Version 0.6 removes the section "Questions and answers". Owner decision 2026-10-02 (delegated to the agent). The answer is in the body of the commit that closes the question, with the words of the owner, the date, and the source, and in the `ANSWER` message. An answer that stays binding also goes into "Decisions" (section 8.6). Agent-derived, needs owner decision.
- The init skill makes the project file of each selected project, in its diff (section 16). Before version 0.6, bigm made it at the first work. Agent-derived, needs owner decision.

### 8.2 Commits and pushes

- bigm commits after each change to the ledger. Agent-derived, accepted 2026-09-30.
- Clerks do all pushes. Owner decision 2026-09-30. The ledger clerk pushes the ledger after each commit of bigm. Agent-derived, accepted 2026-09-30.

### 8.3 Merges

Owner decision 2026-09-29: every merge is a P1 to the owner, plus optional merge grants for each repository.

- A merge grant names one merger role key and its conditions, for example "CI green and all review rounds passed". The owner gives it explicitly, and bigm records it in `grants.md`. A grant is an answer that the owner gives in advance, so the never-without-the-owner list still holds. Owner decision 2026-09-29.
- One merger for each repository at a time. Agent-derived, accepted 2026-09-30.
- A merge is confirmed by reading the code host API, never by an exit code. In the setup of the owner, a runner exit code of 0 on a failed pipeline was reported as merged. The merger reports each merge after the confirmation. Agent-derived, accepted 2026-09-30.
- The merge train of the setup of the owner becomes a plugin script that does not depend on one code host. Agent-derived, accepted 2026-09-30.

### 8.4 Shared resources and leases

Owner decision 2026-09-29: leases are hierarchical. bigm keeps the lease table of the clankers, and each clanker keeps the lease table of its clerks. The details below are agent-derived, accepted 2026-09-30.

- A shared resource (a test database, a staging environment, a paid API, a runner) has a capacity and a list of lease holders.
- The MCP server keeps the tables with the tools `lease_request`, `lease_grant`, and `lease_release`. bigm grants a clanker a lease, and the clanker grants a sub-lease to one of its clerks.
- A `PreToolUse` hook refuses a command on a shared resource when the role key of the session holds no lease. It reads the role key from `BRUH_ROLE_KEY` and matches the command by structure (exact command patterns), never by the meaning of words. Like every deny rule (principle 2), it is a speed bump: a command that starts the resource by itself, for example a test that starts a database, gets past it. The field breach of the setup of the owner was of that kind. Verify: the hook fires for the tool calls of workflow agents inside a clerk session.

### 8.5 Learned projects

Owner decision 2026-10-02: bruh learns the projects of the owner before the owner asks for work. The init skill runs the learn step (section 16), and bigm keeps the result current (section 9.1). The decision log, with the rejected options, is in design.md, "Learn step and onboarding".

Hierarchy. Owner decision 2026-10-02: the hierarchy has the folder tree (root, group folder, project, repository), projects with more than one repository, and links between projects. Machines are not part of it.

- Projects on a remote machine (section 4.2) are not learned in version 0.6. bigm learns about them from the first work request, as in version 0.5. Owner decision 2026-10-02.
- The root is a folder that the owner selects, default `~/workspace`. Owner decision 2026-10-02.
- The scan looks for repositories to a depth that is a runtime setting, default 4, because repositories of the owner are at depth 4. A group folder is a folder between the root and a repository that is not a repository itself. A nested group folder is one item of the pick, shown with its path, for example `gamble/platform-reference`. Agent-derived, needs owner decision.
- The scan proposes the projects and the links from structure only. It never classifies text by its meaning. The owner confirms each proposal. Owner decision 2026-10-02. The rules below are agent-derived, needs owner decision:
  - Projects: two repositories of one group are one proposed project when the name of one is the full name of the other, followed by `-` or `.` (`wishmateai` and `wishmateai-app`). Each other repository is one project.
  - Links: a link goes from a repository to a scanned repository when a manifest names that repository exactly: a module path in `go.mod` or a URL in `.gitmodules` that is equal to the remote of the scanned repository, a package name in `package.json` or `pubspec.yaml` that is equal to the package name of the scanned repository, or a Dockerfile `FROM` image whose last path part without the tag is equal to the name of the scanned repository.
- The project key is the proposed project name in lower case. Each run of characters that are not an ASCII letter or an ASCII digit becomes one `-`, and a `-` at the start or at the end is removed, so that the key matches the project part of a role key (section 3.2). When two groups have a project with the same name, the key gets the group name as a prefix (`hydra-ai-infra`). A key has at most 40 characters. A role key has at most 64 characters (`mcp/env.go`), so with a key of 40 characters a clanker gives a task name of at most 17 characters. When the rule gives an empty key or a key of more than 40 characters, the confirm asks the owner for the key. Agent-derived, needs owner decision.
- A project with more than one repository has one main repository, which the owner can change at the confirm. The clanker works in the folder of the main repository. A clerk starts from the main checkout of the repository that its task changes, and a task changes one repository only: the clanker divides a change of two repositories into two tasks. Agent-derived, needs owner decision.

Knowledge. Owner decision 2026-10-02: for each project, bruh stores the repositories and their code hosts, and the stack and the gates. It stores no live status. bigm reads the open pull requests, the issues, and the pipelines from the code host when the owner asks or when work needs it (principle 1).

- For each repository: the path, the remotes, the host, the host kind, the API root (`api_url` of `repos_set`), and the default branch. Agent-derived, needs owner decision. The details:
  - The scan removes the user information (a user name or a token) from each remote URL before it stores or shows the URL.
  - The API root is `https://api.github.com` for `github.com`, `https://<host>/api/v3` for another GitHub host, `https://<host>/api/v1` for Gitea, and `https://<host>/api/v4` for GitLab.
  - The default branch comes from `.git/refs/remotes/<remote>/HEAD` or from `.git/packed-refs`. When neither has it, the scan reads it from the code host, or records `unknown`. `.git/HEAD` is the branch that is checked out, not the default branch.
- Owner decision 2026-10-02 (delegated ruling): the Go scan finds the stack and the gates, with no model call and no workflow. The sources, in this order: the task names of `Taskfile.yml` or `Taskfile.yaml`, the target names of a Makefile, the `scripts` of `package.json`, and a convention from `go.mod` or `pubspec.yaml`. Exact names map to gates: `build`, `test`, `lint`, `check`. The CI system comes from the CI paths `.github/workflows/`, `.gitlab-ci.yml`, and `.gitea/workflows/`. Each gate records its source (`declared`, `convention`, or `unknown`) and its file and line.
- The scan reads files only, and only the top level of each repository. It never runs `task`, `make`, or another command of a repository, because these can run code of the repository. A clanker reads the `includes:` of a Taskfile and the parts of a monorepo. Owner decision 2026-10-02 (delegated ruling).
- The details below are agent-derived, needs owner decision:
  - A gate value is the call, for example `task test`, `make test`, or `npm run test`, not the body of the task.
  - The MCP server uses the Go standard library only, so it has no YAML parser. In a Taskfile, the scan reads only the names of the keys under `tasks:`, line by line. A file that it cannot read in this way gives the source `unknown`.
  - The scan never runs `git` in a repository, because the git configuration of a repository can run commands. It reads `.git/config`, `.git/refs/`, and `.git/packed-refs` as files.
  - The scan skips a `.git` file (a linked worktree), `.claude/` folders, a repository inside a repository, the ledger, and the folders that the owner excludes, default `archive/`.

Code hosts and activity.

- The pick shows the last activity of each repository on its code host: GitLab `last_activity_at`, GitHub `pushed_at`, Gitea `updated_at`. The last commit of the local clone is not the activity: on 2026-10-02 the local clone of an active GitLab repository showed a commit that was two months old. Owner decision 2026-10-02.
- bruh reads GitLab through `glab`. Owner decision 2026-10-02.
- GitLab gets the same support as GitHub and Gitea: the pick, the status on demand, `repos_set`, the watcher, and the merge train, all through `glab`. Owner decision 2026-10-02. Posts on GitLab already work through `scripts/post-findings.sh`. The contract below is agent-derived, needs owner decision:
  - `glab` has no command that prints a token. So bruh makes each GitLab call with `glab api --hostname <host> <path>`, and glab sends the login of the owner.
  - A GitLab path can have subgroups, for example `group/sub/repo`. So for `gitlab`, the repository grammar of `repos_set`, of the merge grants, and of the merge approvals accepts two or more parts. The API path uses the full path, URL-encoded.
  - The watcher reads the merge requests, the notes, and the pipelines of the head SHA, and maps them to the events of section 9.2.
  - The merge train merges with `PUT projects/<id>/merge_requests/<iid>/merge` and the confirmed head SHA in `sha`, with the methods `merge` and `squash`. It confirms the merge by reading the state `merged` of the merge request (section 8.3).
  - Verify: each of these `glab api` calls on a GitLab repository of the owner, the reads first.
- The rules below are agent-derived, needs owner decision:
  - The host of a remote with an SSH host alias (for example `gitlab.com-work`) is the `hostname` that `ssh -G -- <alias>` prints. This command reads the SSH configuration of the owner and makes no connection. The alias comes from the git configuration of a repository, so the scan accepts only the characters `A-Z`, `a-z`, `0-9`, `.`, and `-`. When the printed `hostname` is the alias itself, init asks the owner for the host of the alias (section 16, step 6).
  - The host kind comes, in this order, from an exact list (`github.com` is `github`, `gitlab.com` is `gitlab`), from the hosts of the logged-in CLIs (`gh auth status --json hosts`, `glab auth status --all`, `tea logins list`), and from the `BRUH_GITEA_TOKEN_<HOST>` variables. Init asks the kind of each other host before the pick (section 16), so that the pick can show its activity.
  - bruh reads GitHub through `gh` and Gitea through `tea` or a `BRUH_GITEA_TOKEN_<HOST>` variable. It calls a host only when it has a working login for that host. For each other host, it makes no call and shows "no login".
  - It runs each CLI with a working folder outside the repositories, so that no configuration of a repository applies.
  - Each call has a timeout of 10 seconds, and at most 8 calls run at the same time. A failed call (no access, not found, a rate limit, a timeout) shows the activity as "unknown" and does not stop the scan.
  - Verify: the `tea` command that reads `updated_at` of a repository, and the option `--all` of `glab auth status`.

Storage. Owner decision 2026-10-02: the result is JSON in the ledger, `learn/tree.json` and `learn/projects/<key>.json`. Plugin code writes these files, and bigm commits them. The rules below are agent-derived, needs owner decision:

- Plugin code writes a file only when a value changes, so a sweep with no change makes no commit.
- The JSON holds no time and no activity. So `init_plan` and `init_apply` write the same bytes for the same answers and the same files.
- The files that the init skill writes, bigm commits at its next turn with `git add -A`.
- When the owner removes a project, plugin code deletes its JSON, and git keeps the old version (section 8.6). The project file goes when it has no open item.
- bruh writes no Markdown view of the tree. bigm shows the tree from the JSON when the owner asks. Owner decision 2026-10-02.

Use.

- bigm reads the code host again for each status claim (principle 1). Owner decision 2026-10-02. bigm answers the other questions about the projects from the JSON. Agent-derived, needs owner decision.
- The rules below are agent-derived, needs owner decision:
  - When bigm starts a clanker, it puts the JSON of the project and of the linked projects into the start message, so that the clanker reads no ledger file.
  - bigm takes the input of `repos_set` (the repositories, the host kinds, and the API roots) from the JSON. The init skill does not call `repos_set`, so the watcher polls only the repositories of the projects that have a clanker.
  - The role settings of the clanker get a read-only allow rule for each other repository of its project. So `role_settings_write` gets an input `allow` (section 10.1). Verify: a `Read` allow rule covers the tools Read, Grep, and Glob. A read of another repository with Bash stays with the classifier of `auto` mode.

Refresh. Owner decision 2026-10-02: the refresh depends on the cost, and it runs at each sweep with no model call. The details are agent-derived, needs owner decision:

- At each sweep, bigm calls `learn_refresh`. The tool reads the remotes and the default branch again from the files in `.git` (see "Knowledge"), and calculates one hash for each project over the names and the contents of the gate files that exist. A new, changed, or removed gate file makes the scan read the gates of that project again.
- A repository that is no longer at its path gets the state `missing`, and bigm sends one P1 for each missing repository.
- A second run of `/bruh:init` changes the projects (section 16).

Tool rules. Agent-derived, needs owner decision:

- `learn_scan` refuses a caller that has a role key, so only the init skill in a session of the owner calls it, never bigm. It takes the root from the init answers.
- `learn_refresh` refuses each caller except bigm.

Verify:

- The scan of the root takes less than 2 seconds when it makes no code host call.
- A fixture Makefile with `$(shell touch x)` does not make the file `x`, and a fixture repository with `core.fsmonitor` in its `.git/config` runs nothing.
- The MCP server reads the root without a permission prompt.
- The output of the scan on the repositories of the owner matches a list made by hand.

### 8.6 Ledger size

Owner decision 2026-10-02 (delegated to the agent): the owner said "i want to avoid situation that every new interaction with bigm will be polluting the .md files, constantly growing ... decide how to overcome that". The decision: the Markdown files of the ledger hold only the current state, and git is the history.

- Each Markdown file of the ledger shows only open or live items. When an item closes, bigm deletes its row in the same commit. Version 0.5 kept these rows with a closed state. Items that close: a task that is done, a merge or a deployment that a status report showed (section 9.4), a question that has an answer, an item of `owed.md` that the owner got, a decision that a newer decision replaces, a lease that ended, an item of "Waiting on others" that arrived, and the session of a retired role.
- A retired role is a clerk whose result the clanker accepted, or a clanker that bigm stopped for good. A session in the state `failed` or `stopped` keeps its row, because the row maps the role key to the session ID for a resume (sections 3.2 and 15).
- The commit that deletes a row names the item in the closed form `close <kind>: <subject>`. `<kind>` is `task`, `merge`, `live`, `question`, `owed`, `decision`, `lease`, `waiting`, `session`, `rule`, or `grant`. The subject of a question starts with its ID, `Q-<n>`. The body of the commit that closes a question quotes the words of the owner, with the date and the source. So `git log --grep` finds each closed item, and nothing is lost.
- Before bigm shows a question, it checks `git log --grep "close question Q-<n>:"`. A question ID that was already answered is not shown again (section 5).
- `rules.md` and `grants.md` hold the rules and the grants that apply now. When the owner retires a rule or withdraws a grant, bigm deletes it with `close rule` or `close grant`, and the commit body quotes the words of the owner. Premise changed: the ledger template said "Do not delete a rule; the owner retires it with a new rule".
- Size check: at each sweep, `learn_refresh` also returns each Markdown file of the ledger that has more lines than `ledger_max_lines` in `mode.md`. bigm then deletes the closed rows of that file. When the file still has more lines, bigm sends one P1 to the owner with the file and its line count. No commit is refused, so bigm never stops. Version 0.6 has no `pre-commit` hook: a refused commit stops bigm, because bigm commits before it acts.
- The details of this section, except the first paragraph, are agent-derived, needs owner decision. `ledger_max_lines` is a runtime setting with the default 300.

## 9. Loops

### 9.1 Sweep

Agent-derived, accepted 2026-09-30.

- bigm runs the sweep as a recurring `CronCreate` task every 15 minutes, and a message wakes it sooner. `CronCreate` tasks are restored on resume. A recurring task expires after 7 days, so bigm creates it again every 6 days.
- The sweep reads all report files, reconciles `claude agents --json --all` and Orca `worker-list`, reads the source for each row past its next check, sends the P1 batch when it is due, and updates the ledger.
- A `Monitor`, a background command, and a self-paced `/loop` are not restored on resume. So on a resume, bigm starts the watcher (section 9.2) and the Orca receive loop again. In the setup of the owner, the sweep loop died on a usage limit, and nobody restarted it.
- Verify: `CronCreate` tasks run in an interactive bigm while the owner is away from the terminal.
- At each sweep, bigm calls `learn_refresh` (section 8.5) and commits a change of the files of the learn step. Owner decision 2026-10-02: the refresh at each sweep. Agent-derived, needs owner decision: the tool and the commit.

### 9.2 Watcher

Agent-derived, accepted 2026-09-30.

- A plugin script, run by a background `Monitor` in bigm, watches the code host for events of the projects: new pushes, replies, red pipelines, merges. It writes each event to the report file of the project. In the setup of the owner, sessions reviewed a change, left comments, and then did nothing more until a watcher was added.
- An agent marks each post it makes with a structural marker, so that the watcher separates agent posts from human posts by structure, not by wording.

### 9.3 Rules

- A new owner rule goes into `rules.md` word for word, and bigm broadcasts it with a `RULE` header to all running sessions. In the setup of the owner, memory notes reached only the sessions that started after the note. Agent-derived, accepted 2026-09-30.

### 9.4 Status report

Agent-derived, accepted 2026-09-30.

- bigm reports status in three parts: "Ready for you", "Waiting on you", "In progress". Commands that the owner must run are in code blocks, never "see above". The setup of the owner used this form.
- The status cadence is a runtime setting in `mode.md`: report only on change, or always.

## 10. Files, the MCP server, and environments

### 10.1 Files and the MCP server

Owner decision 2026-09-29: bruh keeps all its files in the plugin data folder `${CLAUDE_PLUGIN_DATA}`, and plugin code writes them. Claude Code guards `~/.claude` against writes by Claude, but not against writes by plugin code. The only files outside the plugin data folder are the settings keys that the init skill writes after the owner approves the diff, and the private ledger repository.

The plugin ships a stdio MCP server written in Go with the standard library only. Claude Code starts it with `go run -C ${CLAUDE_PLUGIN_ROOT}/mcp .` and `GOTOOLCHAIN=local`, so the repository holds no binaries. Owner decision 2026-09-30. Its tools: `mail_post`, `mail_read`, `handoff_write`, `handoff_read`, `answer_write`, `answer_wait`, `report_write`, `role_settings_write`, `lease_request`, `lease_grant`, `lease_release`. It reads the role key of its caller from `BRUH_ROLE_KEY`. Agent-derived, accepted 2026-09-30. Verify: an MCP server started by a session gets the `env` values of its `--settings` file. Version 0.6 adds the tools `learn_scan` and `learn_refresh` (section 8.5; agent-derived, needs owner decision), the field `options` of `question_open` (section 14.2; owner decision 2026-10-02), and the input `allow` of `role_settings_write` (section 8.5; agent-derived, needs owner decision).

### 10.2 Local and container

Owner decision 2026-09-30: the skillset runs the same way on a local machine and in a container. For autonomous work, the container image is `ghcr.io/oter/autonomous-agents/agent`. The image is infrastructure and is outside this repository. The details below are agent-derived, accepted 2026-09-30.

- The plugin supports macOS and Linux. Scripts use POSIX shell and portable commands.
- In a container, the plugin data folder must be on a volume, or the handoffs, the mailboxes, and the leases die with the container.
- In a container, the init skill runs in its non-interactive form (section 16). The answers then give the selected projects with the keys `root` and `projects`. Agent-derived, needs owner decision.
- In a container, bigm has no Orca terminal. A remote clanker in a container is reached through `orca serve` in the container.

## 11. Modes

Owner decision 2026-09-29: bruh runs with a human or fully autonomous. In autonomous mode, bigm is the main brain and acts on its own. The switch is recorded in the ledger repository.

- The switch is `mode.md` with one line `mode: human` or `mode: autonomous`, the date and the reason of the last change, and the runtime settings of this file. bigm reads it at the start of each turn and passes the mode to each clanker. Agent-derived, accepted 2026-09-30.
- Human mode: P0 and P1 go to the owner. Agent-derived, accepted 2026-09-30.
- Autonomous mode: bigm decides P1 questions itself and records each decision in the ledger with the tag "bigm decision <date>" and its reasons. The owner can reverse each one. The global owner rule "write the options ranked, set needs-info and stop" does not apply to bigm in autonomous mode. Owner decision 2026-09-30.
- The never-without-the-owner list (section 13) is a hard stop in both modes. A P0 in autonomous mode goes to the owner through the channel, and bigm continues other work. Owner decision 2026-09-29 (hard stop). The rest is agent-derived, accepted 2026-09-30.

## 12. Channels

Owner decision 2026-09-29: P0 and P1 questions can go through chat channels. bruh supports several. Slack and Telegram come first. The details below are agent-derived, accepted 2026-09-30.

- A channel runs only when bigm starts with `--channels`.
- Telegram uses the official channel plugin. A question is one bot message, and the owner answers with a reply to it.
- Slack is a custom channel in bruh, `plugins/bruh/channels/slack/`. During the channels research preview, a custom channel loads only with `--dangerously-load-development-channels`. A question is one thread.
- A channel shows the options of a question (section 14.2) as numbered text. Owner decision 2026-10-02.
- A channel that declares permission relay lets the owner approve a permission prompt of bigm from the chat. It does not reach the prompts of other sessions, so a P0 for a prompt of a clanker or a clerk still carries `claude attach <id>`. Only the owner is on the sender allowlist of a channel. Verify: the permission relay and the sender allowlist of each channel.

## 13. Never without the owner

Agent-derived, accepted 2026-09-30, except where tagged. The list is data in `priorities.md`, shipped as a default and copied by the init skill. It is a hard stop in both modes. Owner decision 2026-09-29.

- An irreversible or outward-facing action: publish, deploy, delete, send, merge (except under a merge grant), post a review result on a pull request (except under a post grant, section 6.4). A post grant, like a merge grant, is an answer that the owner gives in advance for one repository and one role key. Agent-derived, needs owner decision: the post grant.
- Deletes and cleanup of anything that is not a temporary file of the session. In the setup of the owner, a coordinator deleted 116 volumes against its own memory note.
- A model change (section 3.3).
- A write under the personal identity of the owner, or with a credential whose identity nobody checked. The identity goes into the project file. Owner decision 2026-09-29.
- Moving or copying a credential, or switching an account. A read-only token read inside a process is allowed. Owner decision 2026-09-29.
- Scope growth past the task.

Each item that matches an exact command pattern (for example `docker volume rm`) becomes a deny rule in the role settings files, not in user settings, so the manual sessions of the owner are not blocked. These deny rules are speed bumps (principle 2). The other items stay P0 in `priorities.md`.

## 14. Questions and priorities

### 14.1 Priorities

Owner decision 2026-09-27: the definitions below. The owner expects to change them. They are data.

- P0: work is blocked and only the owner can unblock it. Examples: security, data loss, money, an item of section 13, a permission block, a classifier refusal, a session waiting on a prompt.
- P1: an owner decision. Examples: an open question, an "X versus Y" choice, a premise that changed under an owner decision, a scope change, a merge.
- P2: the clanker answers from the project context (code, docs, ADRs, the ledger).

Owner decision 2026-09-27: the plugin ships the default `priorities.md`, and bigm passes it to each clanker. Agent-derived, accepted 2026-09-30: the init skill copies the default to the ledger, and a clanker reads it again before it routes a question.

Agent-derived, accepted 2026-09-30: the section 13 items, classifier refusals, and prompt waits in the P0 examples, and merges in the P1 examples.

Owner decision 2026-09-29: `priorities.md` lists the delegated P1 classes that a clanker may answer. Since version 0.6, the init skill does not ask for them: the owner tells bigm, and bigm writes the class into `priorities.md`. Agent-derived, needs owner decision.

### 14.2 Routing

- The clanker answers P2 and delegated P1 classes, and logs each answer. It sends P0 and the other P1 questions to bigm. Owner decision 2026-09-27 and 2026-09-29.
- The clanker decides the final P-level. Agent-derived, accepted 2026-09-30.
- A clanker never answers a P1 question outside the delegated classes. It can add a `REC`. Agent-derived, accepted 2026-09-30.
- bigm shows a P0 at once, at the top of its next reply and through the channel. Owner decision 2026-09-27.
- A P0 for a permission prompt carries the command `claude attach <id>`. A P0 for a classifier refusal carries the exact command and the refusal category, and gives the owner two options: run it, or add a scoped allow rule. Agent-derived, accepted 2026-09-30.
- bigm queues P1 questions and shows them as a batch. Owner decision 2026-09-27. The interval and the maximum batch size are runtime settings in `mode.md`, asked by the init skill and changeable at any time. Owner decision 2026-09-29. Agent-derived, accepted 2026-09-30: the defaults are an interval of 60 minutes and at most 5 items; a batch goes out early when 5 items are queued; an empty queue sends nothing; a P0 never waits for a batch.
- Owner decision 2026-10-02: `question_open` takes an optional field `options`: 2 to 4 options, each with a label and a description. bigm shows each P1 question that has options as a select (`AskUserQuestion`), at most 4 questions on one screen, and more screens when the batch has more. bigm shows each question without options as text. This replaces "bigm shows the items of a batch one for each message" of version 0.5.
- The question file of the asker keeps the options. `question_open` returns the body of the message with the options in it, and the asker sends that body. The options travel in the body, one line for each option in the closed form `OPTION <n>: <label> | <description>`, because bigm cannot read the question file of another role and a remote clanker is on another machine. A clanker or a clerk that relays a question copies these lines word for word. bigm reads the options only from these lines. Agent-derived, needs owner decision.
- Owner decision 2026-10-02: bigm shows the selects only in a reply to a message of the owner. At another time, bigm shows the batch as text and tells the owner to reply to answer it with selects. An open select holds the turn of bigm, so the sweep and a new P0 would wait. Verify: an open select holds the turn of bigm.
- A `REC` that recommends an option has the subject `OPTION <k>`. bigm puts that option first, and its label says "(Recommended)". Agent-derived, needs owner decision.
- bigm sends the answer to the session that asked, as an `ANSWER`, and records the question and the answer in the ledger. Agent-derived, accepted 2026-09-30. Since version 0.6, the record is the commit that closes the question (section 8.6). Agent-derived, needs owner decision.

## 15. Failure handling

Agent-derived, accepted 2026-09-30, except where tagged.

- **A reboot comes first.** After a shutdown, a background session shows as `failed` for up to 48 hours, and as `stopped` after that. An attach or a reply restarts it. So bigm checks for a reboot before it applies the rules below: when many sessions changed to `failed` or `stopped` at the same time, bigm reconciles the ledger from `claude agents --json --all`, updates the role key map, and resumes the long-lived roles with `claude --resume <session ID> --bg`.
- **A session waits on a prompt:** `waitingFor` equal to `permission prompt`, or a waiter timeout, raises a P0 at once. In the setup of the owner, a session that waited on a prompt stalled a coordinator for 34 minutes, and nothing noticed.
- **A classifier refusal:** a P0 (section 14.2).
- **A usage limit:** not a crash. The session stops and reports. bigm does not change the model. bigm probes the account before it claims that the account is out of quota, and sends a P1 so that the owner chooses the account. Owner decision 2026-09-29. A workflow in a background session does not pause at a usage limit: its agents fail. After the limit resets, the clerk launches the workflow again with `resumeFromRunId` and the same `args`. The agents that completed return cached results, and the failed agents and the agents after them run again. In the setup of the owner, a usage limit stopped all sessions for about 8 hours, and 23 workflows resumed this way.
- **A session in state `failed` or `stopped`** without a reboot, that did not finish its task: the parent restarts it once with `claude respawn <id>`. If it fails again, the parent raises a P0. Verify: `claude respawn`.
- **A burst refusal:** a sender that gets a refusal waits and sends again with the next attempt counter.

## 16. Init skill

Owner decision 2026-09-27: the plugin has an init skill that asks the user questions. Owner decision 2026-10-02: the init skill also runs the learn step (section 8.5). The owner gave this feedback after the first run of version 0.5: the 13 questions did not tell what each answer controls, there was no step to see and select the projects, and the questions were not interactive. Agent-derived, needs owner decision: the answer to the feedback, which is one line for each question that tells what its answer controls, and a select (`AskUserQuestion`) with the default first for each question with fixed answers.

Before init: the `userConfig` dialog of the plugin asks the plugin options when the owner installs the plugin, among them `user_name`, `handoff_percent`, and `max_busy_clerks`. The owner changes them in `/config`. The init skill does not ask them, and `init_plan` does not write them. Owner decision 2026-10-02. Premise changed: the owner decision of 2026-09-27 made "how should bruh address you?" the first init question. The install dialog now asks it, and the option is still `user_name`. The first run of version 0.5 showed the problem: the install dialog had set `handoff_percent` to 55 and `max_busy_clerks` to 30, and init asked again and replaced both. The title of each option starts with "bruh: ", for example "bruh: Your name", and the keys do not change. Owner decision 2026-10-02. Verify: a row of `/config` shows the title of the option. If it shows the key, the keys get the prefix `bruh_`, and init moves the saved values (agent-derived, needs owner decision).

`/bruh:init` has these steps. Each step is agent-derived, needs owner decision, except where tagged.

1. Ledger: a select of the ledger folders that the skill finds (a repository with `mode.md`) in the current folder and below the default root, plus another path. When the skill finds none, it asks for the path as text. The skill creates the layout of section 8 for each file that does not exist.
2. Mode: human or autonomous. Agent-derived, accepted 2026-09-30.
3. P1 batch interval, P1 batch size, and review-round cap: one select each, with the default first. Owner decision 2026-10-02, which keeps the owner decision of 2026-09-29 that init asks the batch interval and size.
4. "Defaults for the rest" or "customize". "Customize" asks the auto-compact window, the status line tap (the skill shows the current and the new value), and the channels. "Defaults" keeps each value that the settings already have, and sets only the missing ones.
5. Root folder: `~/workspace`, or another path. Owner decision 2026-10-02.
6. Host kinds: the kind of each host that section 8.5 cannot find, so that the pick can show its activity.
7. Pick (section 8.5). Screen 1 shows the groups and the single repositories, each with a count, for example "12 repos, 4 active in 30 days". For each selected group, the owner selects "active in 30 days", "all", or "pick". Only "pick" opens screens of repositories, most active first, with at most 4 questions of 4 options on a screen. Owner decision 2026-10-02. More than 16 items go on more screens: agent-derived, needs owner decision. Verify: `AskUserQuestion` takes 1 to 4 questions with 2 to 4 options each (the docs page agent-sdk/user-input says so).
8. Confirm. For each group, the proposed projects and links, with the selects "accept", "split", "join", and "skip". "Join" adds the repositories of the project to another project. Then the main repository of each project with more than one repository, and the key of each project for which the rule of section 8.5 gives no key.
9. Diff. Nothing is written before the yes of the owner. The diff contains each write of init: the settings, the role settings, the ledger layout, the files of the learn step, and the project file of each selected project. The skill shows the full diff of the files outside the ledger, and a table for the ledger part: the project key, the repositories, the stack, and the gates with their source. When the owner selects "show all", it shows the full diff of the ledger part. One `diff_sha256` covers all files. Owner decision 2026-10-02 (the table and "show all").
10. Trust. The skill lists each selected repository, and shows for each one whether it has `.claude/settings.json` or `.mcp.json`, because the trust dialog turns on the hooks and the MCP servers of the repository. The owner selects "now" or "at the first work". "Now": the skill guides the owner through one interactive `claude` for each repository, with a counter, and a repository that is already trusted opens with no dialog. "At the first work": bigm sends a P0 with the folder when a clanker cannot start there. Owner decision 2026-10-02 (the trust step, "now" and "at the first work"). The same P0 for a clerk: agent-derived, needs owner decision. The skill does not read or write the trust flags in `~/.claude.json`, because they are not documented.

A second run of the skill asks first: "change projects" or "change settings". "Change projects" shows the selected projects and asks "add" or "remove" with selects. A removed project loses its JSON, and its project file goes when it has no open item (section 8.6). "Change settings" asks the settings outside the ledger again, and each select shows the current value first with the label "(current)". The mode and the P1 settings in `mode.md` change only through bigm (section 11). Each item of this paragraph is agent-derived, needs owner decision. Verify: a select of `AskUserQuestion` has no option that is selected at the start.

These are no longer init questions: merge grants, delegated P1 classes, and remote machines. The owner tells bigm, and bigm records each one: a merge grant in `grants.md` (section 8.3), a delegated class in `priorities.md` (section 14.1), and a remote machine on the new line `remote_environments:` of `mode.md`. Agent-derived, needs owner decision.

The skill adds the allow rules `Workflow(bruh:deliver)` and the allow rules of the MCP tools to user settings, and writes the defaults of the role settings files (section 3.1). Agent-derived, accepted 2026-09-30. After the apply, it shows only the notes that match the answers: for example, no Telegram note when the answer has no channel. Agent-derived, needs owner decision.

Non-interactive form: for a container, the init skill reads the same answers from a file or from environment variables, prints the diff, and writes the files. The learn answers have the keys `root`, `depth`, `exclude`, `host_kinds` (a host and its kind), and `projects`: a list in which each project has `key`, `repos` (the paths), `main` (the path of the main repository), and `links` (the keys of the linked projects). The interactive init sends the same keys to `init_plan`. A container has no install dialog, so the keys `user_name`, `handoff_percent`, and `max_busy_clerks` set the plugin options there. Agent-derived, needs owner decision.

## 17. Plugin layout

Owner decision 2026-09-27: `oter/bruh` is a plugin marketplace with one plugin, `bruh`, installed at user scope. The layout is agent-derived, accepted 2026-09-30.

```text
.claude-plugin/marketplace.json
plugins/bruh/.claude-plugin/plugin.json
plugins/bruh/.mcp.json
plugins/bruh/agents/bigm.md
plugins/bruh/agents/clanker.md
plugins/bruh/agents/clerk.md
plugins/bruh/skills/init/SKILL.md
plugins/bruh/hooks/hooks.json
plugins/bruh/mcp/go.mod
plugins/bruh/mcp/*.go
plugins/bruh/channels/slack/
plugins/bruh/scripts/statusline-tap.sh
plugins/bruh/scripts/handoff-nudge.sh
plugins/bruh/scripts/handoff-inject.sh
plugins/bruh/scripts/lease-guard.sh
plugins/bruh/scripts/watcher.sh
plugins/bruh/scripts/merge-train.sh
plugins/bruh/workflows/deliver.js
plugins/bruh/workflows/tickets.js
plugins/bruh/workflows/implement-tickets.js
plugins/bruh/workflows/review-and-fix.js
plugins/bruh/workflows/review-only.js
plugins/bruh/skills/implement/SKILL.md
plugins/bruh/skills/implement/references/
plugins/bruh/scripts/lane.sh
plugins/bruh/scripts/post-findings.sh
plugins/bruh/defaults/priorities.md
plugins/bruh/defaults/house-rules.md
plugins/bruh/defaults/role-settings.json
```

## 18. README install instructions

Owner decision 2026-09-27: the README gives clear install instructions for each part. The list is agent-derived, accepted 2026-09-30.

1. The plugin: `claude plugin marketplace add oter/bruh`, then `claude plugin install bruh@bruh`. The install dialog asks the plugin options, among them the name, the handoff threshold, and the busy clerk cap.
2. The ledger: create a private repository before the init skill runs.
3. The init skill: `/bruh:init`, or the non-interactive form in a container. It asks for the ledger, the settings, and the projects, and it ends with the trust step.

   Version 0.6 changes items 1 to 3: the install dialog and the trust step are owner decisions of 2026-10-02 (section 16); the ledger before the init skill is agent-derived, needs owner decision.
4. Channels: Telegram or Slack.
5. A remote machine: Claude Code, the plugin, the init skill, `orca serve` on a private network address, and `orca environment add` on the bigm machine.
6. A container: the image, the volume for the plugin data folder, and the non-interactive init.
7. Start: the bigm command of section 3.4.
8. Requirements: the minimum Claude Code version, Go 1.26 or later (for the MCP server), `jq` (for the hook scripts), and Orca for remote work. A warning that plugins that inject text at session start cost context in every role.

## 19. Day-0 maturity

Owner decision 2026-09-27: the repository is mature on day 0, proven by what it contains. A badge shows only a fact that CI or GitHub proves. The list is agent-derived, accepted 2026-09-30.

- CI on each push and pull request: `gofmt`, `go vet`, `go test -race`, Markdown lint, link check, `claude plugin validate`, shellcheck, and the tests of section 20. Verify: `claude plugin validate` runs in CI without a login.
- CI runs the tests on macOS and on Linux (section 10.2).
- Branch protection on `main`, with the CI checks required.
- Semantic version tags with GitHub releases, and `CHANGELOG.md` in the Keep a Changelog format. The first release is `v0.1.0`.
- `LICENSE` with Apache-2.0 (the owner delegated the choice on 2026-09-29), `SECURITY.md`, `CONTRIBUTING.md`, issue templates, and a pull request template.
- Dependabot for GitHub Actions versions. OpenSSF Scorecard workflow and badge.
- CodeQL default setup on each push and pull request, Go fuzz tests of the parsers and the policy checks, and the CI tools pinned by hash. Agent-derived, needs owner decision (the owner asked to handle the Scorecard alerts on 2026-10-01).
- Badges: CI status, latest release, license, OpenSSF Scorecard.
- Repository topics: `claude-code`, `claude-code-plugin`, `multi-agent`, `orchestration`, `agent-workflows`.
- A banner image in `docs/assets/`.

## 20. Testing

Agent-derived, accepted 2026-09-30.

- Script tests with `bats`, on macOS and Linux:
  - the tap writes both percents, writes nothing for `null`, and passes the input on unchanged;
  - the handoff message fires once for each crossing, exits for `agent_id`, and does nothing without `BRUH_ROLE_KEY`;
  - the pickup hook returns the handoff of the role key with its first line, also after `/clear`, and nothing when no file exists;
  - the lease guard refuses a command without a lease and allows it with one.
- MCP server tests: `handoff_write` replaces the file, refuses more than 8,000 characters, refuses a pointer-only write, refuses a subagent ID or a `/tmp` path, and stamps UTC time; `answer_wait` returns an answer that `answer_write` writes, sends progress notifications, and returns `pending` at its deadline; `mail_post` and `mail_read` keep order; the lease tools keep the hierarchy.
- `claude plugin validate` on the marketplace and the plugin.
- Learn tests, in Go with fixture folders (section 8.5). Agent-derived, needs owner decision.
  - The scan finds each repository, and skips a `.git` file, `.claude/` folders, a repository inside a repository, the ledger, and an excluded folder.
  - It removes the user information from each remote URL.
  - It proposes projects from a shared name prefix, and links from each manifest kind.
  - Each project key matches section 3.2, gets the group prefix on a collision, and has at most 40 characters.
  - Exact task and target names map to gates, with the source and the line.
  - It never runs a command of a repository.
  - It calls no code host that has no login (fake `gh`, `glab`, and `tea`).
  - `learn_refresh` writes no file when only a check time changes, and a changed gate file makes the scan read the gates of that project again.
- GitLab host tests with a fake `glab`, in the same way that `ghBin` fakes `gh`. Agent-derived, needs owner decision.
- `question_open` keeps the options, and refuses fewer than 2 or more than 4. Agent-derived, needs owner decision.
- The non-interactive init with the keys `root` and `projects` writes the same files as the interactive init. Agent-derived, needs owner decision.
- A load test of section 4.1: 8 background sessions for one hour.
- A smoke test in a temporary repository before each release. bigm starts one clanker, and the clanker starts one clerk. The clerk runs `/bruh:deliver` on a small task. A workflow agent sends a P1, the question reaches bigm, and the answer reaches the agent in the same run. A clerk that waits on a permission prompt raises a P0 within one sweep. A clanker that was idle for more than an hour resumes when it gets a message. A remote clanker sends a P1 through Orca. Since version 0.6: the init skill with a scratch repository that has a `Taskfile.yml` ends with its test command in `learn/projects/<key>.json` (agent-derived, needs owner decision). The result goes into the release notes.
- Each "Verify" item gets a test or a smoke test step before the implementation depends on it.
- Acceptance of version 0.6: release a new version, install it again, and the owner runs the onboarding and judges it. Owner decision 2026-10-02. The release and the push each need an explicit go of the owner.

## 21. Out of scope for version 0.1

Agent-derived, accepted 2026-09-30.

- Native Windows.
- Per-agent identities and several Claude accounts. Owner decision 2026-09-29: later.

## 22. Open questions for the owner

All numbered questions of version 0.3 are decided. Their answers are in design.md, "Answers to the spec open questions", and in the sections above.

1. The ledger clerk (section 3.6): the reading of "clerks do all pushes" for the ledger.
2. Each item in this file tagged "Agent-derived, accepted 2026-09-30".
3. Each item in this file tagged "Agent-derived, needs owner decision". Version 0.6 has them in sections 3.4, 3.5, 8.1, 8.5, 8.6, 9.1, 10.1, 10.2, 14, 16, 18, and 20.
4. The watcher and section 9.2. Section 9.2 says that the watcher writes each event to the report file of the project, but the code writes all events to one file, `reports/watcher.jsonl` (found in the review of 2026-10-02). Options, ranked: (1) change section 9.2 to match the code; (2) change the watcher to write the report file of each project.

## 23. Changes from version 0.3

- Launch settings for every role: `--permission-mode auto`, and a role settings file with `BRUH_ROLE_KEY`, deny rules, and tool account variables (section 3.1). This fixes the blocker that no hook could find the role key.
- Remote traffic goes only through Orca, because the mailbox is local (section 4.2). This fixes the second blocker.
- Answers go to the clerk that asked, with a copy in the report file (section 3.5).
- The sweep uses `CronCreate`, and bigm restarts the watcher and the Orca receive loop after a resume (section 9.1).
- `answer_wait` sends progress notifications and has a deadline (section 6.2).
- Deny rules are speed bumps, and they go into the role settings files, not user settings (principle 2, section 13).
- A background session uses the account of the supervisor (section 4.1).
- A reboot check comes before the failure rules, and a usage limit makes background workflow agents fail, not pause (section 15).
- The channel flag at launch, and permission relay only for bigm (section 12).
- The owner answers of 2026-09-29 and 2026-09-30: delegated P1 classes, review cap as a setting, merge grants, credential rules, identities later, hierarchical leases, hard stop in both modes, usage limit P1, mailbox transport, Apache-2.0, clerks push, `auto` mode, autonomous bigm decides P1, local and container, all parts in version 0.1.
- Field claims corrected: `claude --bg` was used twice, the concurrency numbers were a judgment, the message count was an estimate, the swallowed nudges were terminal sends, and the layer that dropped a duplicate is not known.

## 24. Changes from version 0.4

- Owner decision 2026-09-30: the implement skillset of the setup of the owner is part of the plugin (section 6.4). The review of pull requests that bruh did not write moves from section 21 into version 0.1, as `/bruh:review-only`.
- Section 13 gets the post grant, the counterpart of the merge grant for posts of review results.
- Premise changed: section 21 excluded `review-only`. Dependent decisions re-decided: the plugin layout of section 17 lists the new files; the posting rule of section 6.1 stays and now also covers the review workflows, so the posting step of the setup of the owner moves out of the workflows into a session step with the owner yes or a post grant.

## 25. Changes from version 0.5

The owner ran `/bruh:init` for the first time on 2026-10-02. The changes come from the feedback of the owner, the decisions L1 to L35 of design.md ("Learn step and onboarding"), and reviews by subagents: four on the decisions (an adversarial reviewer, a checker of the claims against the code, a walk through the journey of the owner, and a search of the Claude Code docs), and two rounds of an adversarial reviewer and a checker on this specification. Each dependent that a "Premise changed" line names carries its own tag in its section.

- Owner decision 2026-10-02: bruh learns the projects before the owner asks for work (sections 1 and 8.5). The init skill scans a root folder, the owner selects the projects with selects, and the Go scan stores the hierarchy, the repositories and their code hosts, and the stack and the gates as JSON in the ledger.
- Owner decision 2026-10-02 (delegated ruling): the Go scan finds the gates from exact task and target names. There is no learn workflow, so no role other than a clerk starts a workflow (section 3).
- Owner decision 2026-10-02: bruh stores no live status. bigm reads it from the code host on demand (section 8.5).
- Owner decision 2026-10-02: bruh supports GitLab through `glab` in the same way as GitHub and Gitea: the pick, the status on demand, `repos_set`, the watcher, and the merge train (section 8.5).
- Owner decision 2026-10-02, delegated to the agent: the Markdown files of the ledger hold only the current state, git is the history, and a check at each sweep finds a file that is too long (section 8.6). Premise changed: bigm kept each row with a closed state, and the project file kept each question and answer. Dependent decisions re-decided: a closed row is deleted in its commit, the section "Questions and answers" goes (section 8.1), a removed project loses its JSON (section 8.5), the record of an answer is its commit (section 14.2), and a retired rule or grant is deleted (section 8.6).
- Owner decisions 2026-10-02, after the review of version 0.6: no Markdown view of the tree in the ledger; projects on a remote machine are not learned in version 0.6; the init diff shows a table for the ledger part, and the full ledger diff on request (sections 8.5 and 16).
- The init skill has new steps (section 16): the pick and the trust step are owner decisions of 2026-10-02; the purpose line for each question and the selects are agent-derived, needs owner decision. It does not ask the three plugin options of the install dialog, and it ends with a trust step (sections 4.1 and 16).
- Owner decision 2026-10-02: P1 questions can have options, and bigm shows them as selects in a reply to a message of the owner (section 14.2). A channel shows them as numbered text (section 12).
- Premise changed: bigm learned about a project only from the first work request. Dependent decisions re-decided: the project file is made by the init skill, not at the first work (section 8.1); a clanker starts from the stored knowledge (section 3.5); bigm takes the input of `repos_set` from the stored knowledge (section 8.5); the sweep calls `learn_refresh` (section 9.1).
- Premise changed: bigm was the only writer of the ledger. Dependent decisions re-decided: plugin code writes the files of the learn step, and bigm stays the only role that commits (principle 3, section 3.4).
- Premise changed: init wrote `user_name`, `handoff_percent`, and `max_busy_clerks` into `pluginConfigs`. Dependent decisions re-decided: the install dialog asks them, `init_plan` does not write them, and the non-interactive form sets them in a container (section 16).
- Premise changed: the trust list of init had only the ledger. Dependent decisions re-decided: the trust step covers the selected repositories, and the P0 at the first work covers a clanker and a clerk (sections 4.1 and 16).
- Premise changed: init asked 13 questions, one for each message. Dependent decisions re-decided: merge grants, delegated P1 classes, and remote machines go to bigm, which records them in `grants.md`, `priorities.md`, and `mode.md` (sections 14.1 and 16); the README asks for the ledger before the init skill (section 18).
- Premise changed: bigm showed the P1 items one for each message. Dependent decision re-decided: at most 4 questions with options on one select screen, and the text form at other times (section 14.2).
- Corrected in the decision log: the watcher is its own process that polls every 60 seconds, not a step of the sweep; bruh already uses `glab` for posts on GitLab; the plugin has 6 options, not 3; many repositories of the owner are on `gitlab.com`: a count of the first remotes of the local clones on 2026-10-02 found 55, of which 37 use an SSH host alias.
