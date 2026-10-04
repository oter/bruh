# bruh specification

Status: version 0.6, for the build (2026-10-03). The owner approved version 0.5 on 2026-09-30. On 2026-10-03, the owner accepted all earlier agent-derived items of version 0.6 and made twelve decisions (design.md, L38 to L49). Section 25 lists the changes from version 0.5, and section 24 the changes from version 0.4.

This specification is the input for the implementation plan. The decision log is in [design.md](design.md). The verified facts and the field lessons are in [knowledge.md](knowledge.md). The process is shown as diagrams in [flow.md](flow.md). Each decision in this file has one tag:

- "Owner decision <date>": the owner made the decision.
- "Owner decision <date> (delegated ruling)": the owner gave the decision to a subagent, and the subagent made it.
- "Owner decision <date> (delegated to the agent)": the owner told the agent to decide, and the agent made the decision.
- "Agent-derived, accepted 2026-09-30": the agent proposed it, and the owner accepted all such items as a set on 2026-09-30, to test them in practice.
- "Agent-derived, accepted 2026-10-03": the agent proposed it, and the owner accepted all such items of version 0.6 as a set on 2026-10-03 ("Accept all, build now").
- "Agent-derived, needs owner decision": the agent proposed it, and the owner has not decided it. The items of 2026-10-03 with this tag are built as written, and the owner reviews them during the onboarding run (owner decision 2026-10-03, section 22).
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
8. **A defect of bruh gets the offer of a bug report.** When a session finds a defect of bruh itself, the session that talks to the owner offers the owner a bug report with the final text of the issue. It files the issue only after a yes of the owner for that text. A vulnerability gets a private report, never a public issue. A clanker or a clerk sends a notice to its parent, and bigm makes the offer. The steps are in `defaults/bug-reports.md`. Owner decision 2026-10-03: the offer. Agent-derived, needs owner decision: the rest (design.md, L49).

## 3. Roles

| Role | Lifetime | Started by | Does hands-on work |
|---|---|---|---|
| bigm | Long-lived | The owner | No |
| Clanker | Long-lived | bigm | No |
| Clerk | One task | A clanker | No |
| Workflow | One run | A clerk | Yes |

Owner decision 2026-09-27: the four roles, their names, the spawn chain, the lifetimes, and the hands-on column for clankers, clerks, and workflows. The hands-on value for bigm is agent-derived, accepted 2026-09-30. The setup of the owner supports it: its coordinator had the rule "no hands".

Premise changed: the role table of 2026-09-27 has four roles, and only a clerk starts a workflow. Version 0.6 adds the subagent `bruh:learner` (section 8.5). Dependent decisions re-decided: the learner is not a role, because the init session starts it with the Agent tool for one read of one project, it has no role key of its own, and it does no hands-on work; the table, the spawn chain, and the rule that only a clerk starts a workflow stay. Agent-derived, needs owner decision.

Each role runs as a plugin agent: `--agent bruh:bigm`, `--agent bruh:clanker`, `--agent bruh:clerk`. The role instructions are then the system prompt, which reloads after compaction. In the setup of the owner, the "no hands" rule faded after compaction because it was only in the conversation. Agent-derived, accepted 2026-09-30. Verify: `--agent` accepts the namespaced name of a plugin agent.

### 3.1 Launch settings of every role

Every role starts with these flags, except bigm (section 3.4). Agent-derived, accepted 2026-09-30, except where tagged. The exception for bigm: agent-derived, accepted 2026-10-03.

- `--permission-mode auto`. A plugin agent ignores `permissionMode` in its definition, so the mode is set at launch. All sessions of the setup of the owner ran in `auto` mode, and no message was held. Owner decision 2026-09-30.
- `--settings <file>`, a settings file for the role that the parent writes through the MCP server. It holds `env.BRUH_ROLE_KEY` (section 3.2), the deny rules of section 13, and the tool account variables of section 4.1. Since version 0.6, the file of a clanker also holds read-only allow rules for the other repositories of its project (section 8.5; agent-derived, accepted 2026-10-03). The `--settings` flag carries through the restarts of a background session, but variables exported in the shell do not reach a background session.
- Since version 0.6, the start settings of bigm are `<ledger>/.claude/settings.json` (section 3.4). Owner decision 2026-10-02. The file holds the same `env.BRUH_ROLE_KEY`, env values, and deny rules, so bigm starts with no `--settings` flag. Agent-derived, accepted 2026-10-03. A plain `claude` start of bigm runs in the default permission mode, not in `auto` mode (section 3.4). Agent-derived, accepted 2026-10-03.

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

- The owner starts bigm in the folder of the private ledger repository, in one of two ways. Owner decision 2026-10-02.
  - The full command:

    ```bash
    claude --agent bruh:bigm --name bigm --permission-mode auto \
      --channels plugin:telegram@claude-plugins-official
    ```

  - Plain `claude`. The key `agent` of the ledger settings file makes the session bigm. This session runs in the default permission mode, not in `auto` mode, and has no name `bigm` and no channel: `auto` mode does not take effect from project settings, and no settings key sets the session name or loads a channel. The full command adds them.
  - After a plain start, the owner runs `/rename bigm`. The clankers and the clerks nudge bigm with `SendMessage` to the session name `bigm`, so without the name a message waits for the next sweep. Agent-derived, accepted 2026-10-03.
  - When Slack or Telegram is set up, the owner starts bigm only with the full command. A plain start reads the channel messages and drops them. The Slack server of bruh polls Slack and moves its state forward whenever `BRUH_ROLE_KEY` is `bigm`, also without the development flag, and Claude Code drops the channel notifications of a session that did not load the channel. So the answers of the owner are lost. The Telegram server of each session that loads the Telegram plugin takes the bot (README step 4). Agent-derived, accepted 2026-10-03.

  For remote work, bigm runs in an Orca terminal. A channel runs only when the session starts with `--channels`. Agent-derived, accepted 2026-09-30.
- The start settings of bigm are `<ledger>/.claude/settings.json`, committed to the ledger repository. Owner decision 2026-10-02. The file has the key `agent` with the value `bruh:bigm`, the `env` values (`BRUH_ROLE_KEY=bigm` and the env of `defaults/role-settings.json`), and the deny rules of `defaults/role-settings.json`. It replaces `<plugin data folder>/roles/bigm.json`, so the full command has no `--settings`. Init writes the file (section 16). Agent-derived, accepted 2026-10-03.
  - Each session that starts in the ledger folder or in another clone of the ledger with no `--agent` and no `--setting-sources user` is bigm. Today only the machine of bigm has the ledger, because a remote clanker does not clone it. Agent-derived, accepted 2026-10-03.
  - On the machine of bigm, a second `claude` in the ledger folder is a second bigm. It reads the mailbox of bigm with `mail_read`, starts a second sweep and a second watcher, and writes the handoff of bigm. So the owner runs one bigm at a time, and opens each other session in the ledger folder with `claude --setting-sources user`. Agent-derived, accepted 2026-10-03. The owner also runs `/bruh:init` from such a session, because `learn_scan` refuses a caller with a role key (section 16). Agent-derived, needs owner decision.
  - On a remote machine, init writes the same file into the stand-in ledger folder (README step 5). A plain `claude` in that folder is a bigm on the remote machine, so the owner does not start `claude` there. Agent-derived, accepted 2026-10-03.
  - The ledger clerk and a merger clerk start in the ledger folder with `--agent bruh:clerk` and their own `--settings`. Both flags rank above the project settings, so each stays a clerk with its own role key. Agent-derived, accepted 2026-10-03.
  - An agent name that does not exist gives a normal session with no warning (probe of 2026-10-02 with Claude Code 2.1.284). So the release smoke test checks the plain start (section 20). A missing role key is not silent: each bruh tool call fails with "BRUH_ROLE_KEY is not set", so the smoke test does not probe it. Agent-derived, accepted 2026-10-03.
- bigm starts a clanker for each project that has work. It starts as many clankers as necessary and does not rotate them. Owner decision 2026-09-27. The concurrency caps of section 3.7 apply. Agent-derived, accepted 2026-09-30.
- bigm is the only writer of the ledger, except the files of the learn step (section 8.5), which plugin code writes, and `.claude/settings.json`, which init writes (section 16). bigm commits after each change. The setup of the owner confirmed that one writer kept the ledger consistent. Agent-derived, accepted 2026-09-30. The exception for the learn step: owner decision 2026-10-02. bigm is also the only role that commits the files of the learn step: agent-derived, accepted 2026-10-03. The exception for `.claude/settings.json`: agent-derived, accepted 2026-10-03.
- bigm runs the sweep of section 9.1 and reconciles all sessions at each sweep and at the start of each turn. Agent-derived, accepted 2026-09-30.
- bigm answers status questions from the ledger. It reads the source again for each claim of principle 1. Agent-derived, accepted 2026-09-30.
- bigm keeps an "Owed to owner" list. Each ask of the owner, and each item that bigm owes the owner, goes on the list before bigm acts or relays. In the field run, two such items were lost. Agent-derived, accepted 2026-09-30.
- bigm shows P0 and P1 questions to the owner in its terminal, and through a channel (section 12). Owner decision 2026-09-27 (terminal) and 2026-09-29 (channels).
- bigm keeps the lease table of the clankers (section 8.4). Owner decision 2026-09-29.
- bigm stays an interactive session. It is never sent to the background, because a background session moves into a worktree before it edits files. Agent-derived, accepted 2026-09-30.

### 3.5 Clanker

- A clanker works in one project folder and knows the full picture of that project. It does no hands-on work. Owner decision 2026-09-27.
- A clanker starts from the index of its project and of the linked projects (section 8.5), and reads the code for its task. Agent-derived, accepted 2026-10-03.
- At the start of work, a clanker learns how to build, test, and run the project from the repository itself, through the doc pointers of the index, and gives its clerks the gate commands in their start message. Nothing of it is stored in the ledger. Owner decision 2026-10-03.
- Premise changed: the delegated ruling of 2026-10-02 had the clanker read only the gates that the stored knowledge marked `unknown` or `convention`. The ledger now stores no gate (section 8.5). Dependent decision re-decided: the clanker learns each gate from the repository at the start of work (owner decision 2026-10-03, above), and the re-learn of the first form of 2026-10-03 goes. Agent-derived, needs owner decision.
- A clanker writes no file. Its handoff and its reports go through the bruh MCP server. Agent-derived, accepted 2026-09-30.
- A clanker divides the work into tasks and starts one clerk for each task. Owner decision 2026-09-27.
- The start message of a clerk contains: the task, the acceptance criteria, the project context that the task needs, the role key of the clanker, the text of `priorities.md` and `rules.md`, the base SHA, the files that the task will touch, and the known overlaps with other tasks. Agent-derived, accepted 2026-09-30. Since version 0.6, it also contains the gate commands that the clanker learned from the repository. Owner decision 2026-10-03.
- A clanker starts each clerk from the main checkout of the project, so that each clerk gets its own worktree. Agent-derived, accepted 2026-09-30. Since version 0.6, a project can have more than one repository, so the clerk starts from the main checkout of the repository that its task changes (section 8.5). Agent-derived, accepted 2026-10-03.
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
- The header line is one of these. `<id>` is a question ID, `Q-<project>-<host>-<n>` (below).
  - `P0 <id>: <subject>`, `P1 <id>: <subject>`, `P2 <id>: <subject>`
  - `ANSWER <id>: <subject>`
  - `REC <id>: <subject>`: a recommendation, not an owner answer.
  - `RULE <rule id>: <subject>`: a new standing rule for all sessions (section 9.3).
  - `DONE: <subject>`
  - `START: <subject>`: the start message of a role (plan index, item 5). Agent-derived, needs owner decision.
- Question ID. Owner decision 2026-10-03: a question ID is `Q-<project>-<host>-<n>`, and `<host>` is the host name of the machine of the asker. The owner accepted that a renamed machine, or a new install on the same host, can give an ID again. The rules below are agent-derived, needs owner decision.
  - `<project>` is the project part of the role key of the asker (section 3.2): `<p>` for `clanker-<p>` and for `clerk-<p>-<task>`, `bigm` for `bigm`, and `ledger` for `clerk-ledger`. A workflow agent asks with the role key of its clerk.
  - `<host>` is the host name of the machine up to the first `.`, in lower case. Each run of characters that are not `a` to `z` or `0` to `9` becomes one `-`, and a `-` at the start or at the end is removed. When the result is empty, `<host>` is `host`.
  - `<n>` is the question counter of the machine (`mcp/question.go`), as in version 0.5. It starts at 1.
  - An ID matches `^Q-[a-z0-9]+(-[a-z0-9]+)+-[0-9]+$`. Each check of a header, of an answer, and of an approval uses this pattern, so it refuses an ID of version 0.5, which has no project and no host.
  - Limits: two IDs can still be equal. A project key and a host can contain `-`, so the project `a-b` on the host `c` and the project `a` on the host `b-c` both give `Q-a-b-c-1`. Two machines whose host names have the same part before the first `.` give the same `<host>`. bruh uses each ID only as a whole, and does not split it back into its parts.
  - Example: the clerk `clerk-shop-login` on the host `Dev-Mac.local` sends `P1 Q-shop-dev-mac-7: merge the login fix?`.
- Premise changed: the counter of each machine gave `Q-<n>`, so a remote clanker and bigm could give the same ID, and a new install started again at `Q-1` (section 22 of the draft of 2026-10-02, item 5). Dependent decisions re-decided, each agent-derived, needs owner decision:
  - "Unique in the ledger" becomes: unique across the machines, except after a renamed machine or a new install on the same host, and except the limits above.
  - The receiver ignored each question ID that it already answered. This changes with the item "A reused question ID" (below).
  - The header of a post approval (section 6.4) and the subject of the commit that closes a question (section 8.6) carry the new ID. The commit subject keeps the role key of the asker, because the ID names the project but not the task of a clerk.
  - A question counts as answered when bigm has its answer file, not by the history in git (section 8.6). This stays, because an ID can still repeat after a new install.
  - bigm keys a question by the pair of the role key of the asker and the ID, never by the ID alone (plan 3, ruling M4). This stays. The record `<orca environment>/Q-<n>` of a question of a remote clanker goes, because the ID now names the host. Premise changed: ruling M4 added the Orca environment because the counter of each machine gave the same IDs.
- A reused question ID. Owner decision 2026-10-03: when bigm gets a question whose ID it already answered, and the subject is different, the question is new. bigm tells the asker to open it again, which gives it a new ID. bigm drops no question without a message. The owner selected "Compare subject, re-ask".
  - The details below are agent-derived, needs owner decision.
  - Only bigm makes this check. It keys each question by the pair of the role key of the asker and the ID, for open and for answered questions.
  - bigm compares the subjects after it removes a trailing space and `(attempt <n>)` (section 15).
  - The same pair and the same subject is a repeat. For an answered question, bigm sends the stored `ANSWER` again. A repeat of an open question stays one open question. bigm never ignores a repeat without a reply.
  - For a repeat of an open question, bigm adds no row and sends `DONE: <id> is still open` to the asker. A remote clanker that asks again with `orca orchestration ask --resume` gets the earlier reply from Orca, so bigm sends nothing more to it. Agent-derived, needs owner decision.
  - The same pair and another subject is a new question. bigm sends `ANSWER <id>: reask` with the body "open the question again with a new ID". A clanker and a clerk read the subject `reask` as "open the question again", never as the answer.
  - A workflow agent waits on the answer file of the old ID. So its clerk opens the question again under the role key of the clerk, and when the `ANSWER` of the new ID arrives, the clerk writes that answer to the new ID and to the old ID with `answer_write`. Agent-derived, needs owner decision.
  - `answer_write` gets the input `subject` and stores it in the answer file. bigm calls `answer_write` for each answer that it sends, so that it can compare and send the answer again.
  - `answer_write` also gets the optional input `asker`, the role key of the asker, and stores it in the answer file. The name of an answer file of bigm has only the ID, so bigm compares the pair of the asker and the ID with the stored `asker`. Agent-derived, needs owner decision.
  - Each new ID moves the counter of the asker by one, so after a new install of a remote machine, a question can need more than one new ID.
- Upgrade from version 0.5: the checks refuse an ID of version 0.5, so an open question with such an ID cannot get an `ANSWER`. bigm asks each asker to open the question again, which gives it a new ID, and closes the old row. Agent-derived, needs owner decision.
  - bigm sends `DONE: reask Q-<n> - <subject>` to the asker of each open row with an ID of version 0.5, because the checks refuse `ANSWER Q-<n>: reask`. A clanker and a clerk read this message as a `reask` of the question `Q-<n>`. Agent-derived, needs owner decision.
- Only these headers are sent as messages. Routine status goes to the report file of the sender, which bigm reads at each sweep.
- An `ANSWER` from the owner quotes the words of the owner, with the date and the question ID.
- A question records its age and the work that it blocks.
- A retry adds an attempt counter to the nudge. In the setup of the owner, a repeated identical message was dropped as a duplicate. The layer that dropped it is not known. The receiver ignores a question ID that it already answered. Since version 0.6, bigm sends the stored answer again for a repeat with the same subject (above). Agent-derived, needs owner decision. A question with the same ID and another subject is new (owner decision 2026-10-03, above).
- Verify: how `SendMessage` behaves when the receiver is busy.

## 6. Workflows

### 6.1 General

- A workflow is a script for the Claude Code `Workflow` tool. Owner decision 2026-09-27.
- The plugin ships its workflows in `plugins/bruh/workflows/`. They run as `/bruh:<name>`. Agent-derived, accepted 2026-09-30. Verify: the Workflow tool accepts a plugin workflow, because it refuses a `scriptPath` outside the working directory.
- The clerk starts a workflow with a slash command in its start message. In the setup of the owner, a workflow needed the typed opt-in of the owner in each session, and a relayed opt-in did not count. Verify: a clerk started by a script can launch `/bruh:deliver`, and the allow rule `Workflow(bruh:deliver)` matches a namespaced plugin workflow. Agent-derived, accepted 2026-09-30.
- The script cannot write files. Agents and MCP tools do all writes.
- No agent inside a workflow posts outside the project, for example comments on a pull request. In the setup of the owner, an in-workflow post agent refused twice because it saw the latest chat message of the owner. Posting is clerk work and is outward-facing. Agent-derived, accepted 2026-09-30. A bug report on the bruh repository is not a post of project work: the session that got the yes of the owner files it (section 2, principle 8). Agent-derived, needs owner decision.

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

Owner decision 2026-09-30: the plugin includes the bruh-implement skillset of the setup of the owner, as the skill `/bruh:implement` and its workflows. The owner used it for every code change and every review in the field run. The details below are agent-derived, accepted 2026-10-03, except where tagged.

- The skill is the procedure of rule zero: the orchestrating session never edits code. Each change is a brief to an implementer agent and a review by a separate agent with the review guides, in a fix loop. For a settled spec with many changes, the pipeline is: review guides, function-sized TDD tickets, execution waves, lanes, a whole-branch gate, and a multi-lens review whose findings are refuted before they count.
- Workflows: `/bruh:tickets` (ticket breakdown), `/bruh:implement-tickets` (waves and lanes), `/bruh:review-and-fix` (review, refute, and fix of the own change), and `/bruh:review-only` (review and refute of a change that bruh did not write). Each takes its paths in `args`.
- Scripts: `scripts/lane.sh` (a private lane copy of the tree for a ticket, its patch, and its merge back) and `scripts/post-findings.sh` (posts a saved review result on the pull request or merge request).
- No workflow posts outside the project (section 6.1). A workflow returns its findings. A session saves the result with the MCP tool `result_save` and posts it with `post-findings.sh` after the owner approved the post (an `ANSWER` of bigm with the header `ANSWER <id>: post <owner/repo>#<number> at <head SHA> approved`, where `<id>` is the question ID of section 5, or a yes in the terminal of a manual session), or under a post grant in `grants.md`. A post goes out under an account of the owner, so it is on the never-without-the-owner list (section 13). Each posted body starts and ends with the structural marker line of section 9.2.
- In a role session, `/bruh:implement` runs inside a clerk. In a manual session of the owner, the owner is the orchestrator and answers each ask in the terminal.
- The published files contain no project names, account names, host names, or ticket names of the setup of the owner.
- `post-findings.sh` stops with the exit code 1 when `jq` cannot parse an index file of the ledger, so that it does not skip the identity check of an SSH host alias (section 8.5). Agent-derived, needs owner decision.

## 7. Context budget and compaction

Owner decision 2026-09-27: every role stays below about 55 percent of its context window. The mechanism is design.md, open question 1, option 2. The configuration is in user settings. The setup of the owner supports the budget: the owner corrected sessions at 900k tokens and above. The details below are agent-derived, accepted 2026-09-30.

- Auto-compact window: the init skill writes `autoCompactWindow` into `~/.claude/settings.json`. The default is 550000, which is 55 percent of a 1M window.
- Status line tap: the init skill copies `statusline-tap.sh` into the plugin data folder and sets `statusLine.command` to the absolute path of the copy, followed by the previous status line command. The tap writes `context_window.used_percentage` and `rate_limits.five_hour.used_percentage` to `<plugin data folder>/context/<session_id>`. It writes nothing for a `null` value. Then it runs the previous command with the same input. Owner decision 2026-10-03: before init wraps the previous command, it removes each tap that init wrote in another bruh data folder (`<plugins root>/data/bruh-<id>/bin/statusline-tap.sh`), so a new install does not wrap the old tap again (design.md, L48). The match rule is agent-derived, needs owner decision. Verify: the field `rate_limits.five_hour.used_percentage`.
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
learn/projects/<key>.json  the index of each project (section 8.5)
.claude/settings.json the start settings of bigm (section 3.4)
```

Version 0.6 adds `learn/`: owner decision 2026-10-02. Version 0.6 adds `.claude/settings.json`: owner decision 2026-10-02.

### 8.1 Project file

Agent-derived, accepted 2026-09-30.

Sections: Summary; In progress; Merged; Live; Decisions; Sessions (role key, session ID, session name, machine, state, compaction count); Identities (the account or identity of each credential, checked before its first write); Shared resources and clerk leases (section 8.4); Waiting on others.

- Each row has: owner, task, expected deliverable, state, next check (UTC), link, and the source read of principle 1. The setup of the owner used these columns, with the rules "a dispatch is a row" and "a report is an update".
- An owner action is a row too.
- Merged and Live are separate. bigm rebuilds In progress, Merged, and Live from git and the code host after each merge and at each sweep, and stamps each row with its own "as of" time. Agent-derived, accepted 2026-09-30. Since version 0.6, bigm adds a row to Merged or Live when it finds a new merge or deployment, and does not rebuild these two sections from the history. A row goes after bigm has shown it in a reply to a message of the owner (section 8.6). Agent-derived, accepted 2026-10-03.
- Version 0.6 removes the section "Questions and answers". Owner decision 2026-10-02 (delegated to the agent). The answer is in the body of the commit that closes the question, with the words of the owner, the date, and the source, and in the `ANSWER` message. An answer that stays binding also goes into "Decisions" (section 8.6). Agent-derived, accepted 2026-10-03.
- The init skill makes the project file of each selected project, in its diff (section 16). Before version 0.6, bigm made it at the first work. Agent-derived, accepted 2026-10-03.

### 8.2 Commits and pushes

- bigm commits after each change to the ledger. Agent-derived, accepted 2026-09-30.
- Clerks do all pushes. Owner decision 2026-09-30. The ledger clerk pushes the ledger after each commit of bigm. Agent-derived, accepted 2026-09-30.

### 8.3 Merges

Owner decision 2026-09-29: every merge is a P1 to the owner, plus optional merge grants for each repository.

- A merge grant names one merger role key and its conditions, for example "CI green and all review rounds passed". The owner gives it explicitly, and bigm records it in `grants.md`. A grant is an answer that the owner gives in advance, so the never-without-the-owner list still holds. Owner decision 2026-09-29.
- One merger for each repository at a time. Agent-derived, accepted 2026-09-30.
- A merge is confirmed by reading the code host API, never by an exit code. In the setup of the owner, a runner exit code of 0 on a failed pipeline was reported as merged. The merger reports each merge after the confirmation. Agent-derived, accepted 2026-09-30.
- The merge train of the setup of the owner becomes a plugin script that does not depend on one code host. Agent-derived, accepted 2026-09-30.
- A merger clerk reads the cover of a merge at its source: the grant row in `grants.md`, or the answer file `<plugin data folder>/answers/bigm/<question ID>.answer`, which bigm writes for each answer that it sends (section 5). Agent-derived, needs owner decision.
- When the index file of the project exists but plugin code cannot read or decode it, the merge train stops with an error. It does not skip the identity check of an SSH host alias (section 8.5). Agent-derived, needs owner decision.

### 8.4 Shared resources and leases

Owner decision 2026-09-29: leases are hierarchical. bigm keeps the lease table of the clankers, and each clanker keeps the lease table of its clerks. The details below are agent-derived, accepted 2026-09-30.

- A shared resource (a test database, a staging environment, a paid API, a runner) has a capacity and a list of lease holders.
- The MCP server keeps the tables with the tools `lease_request`, `lease_grant`, and `lease_release`. bigm grants a clanker a lease, and the clanker grants a sub-lease to one of its clerks.
- A `PreToolUse` hook refuses a command on a shared resource when the role key of the session holds no lease. It reads the role key from `BRUH_ROLE_KEY` and matches the command by structure (exact command patterns), never by the meaning of words. Like every deny rule (principle 2), it is a speed bump: a command that starts the resource by itself, for example a test that starts a database, gets past it. The field breach of the setup of the owner was of that kind. Verify: the hook fires for the tool calls of workflow agents inside a clerk session.

### 8.5 Learned projects

Owner decision 2026-10-02: bruh learns the projects of the owner before the owner asks for work. The init skill runs the learn step (section 16), and bigm keeps the git facts current with the refresh (section 9.1). The decision log, with the rejected options, is in design.md, "Learn step and onboarding".

Owner decision 2026-10-03: the ledger is the index of the projects and the glue among them. It does not repeat how a repository or a service works, so bruh keeps no double knowledge. The owner said on 2026-10-03: "agent at init, BUT WE MUST NOT REPEAT HOW THE REPO/SERVICE WORKS. WE MUST NOT HAVE DOUBLE KNOWLEDGE. BUT THREAT bigm repo AS GLUE AND INDEX AMONG ALL THE PROJECTS".

Hierarchy. Owner decision 2026-10-02: the hierarchy has the folder tree (root, group folder, project, repository), projects with more than one repository, and links between projects. Machines are not part of it.

- Projects on a remote machine (section 4.2) are not learned in version 0.6. bigm learns about them from the first work request, as in version 0.5. Owner decision 2026-10-02.
- The root is a folder that the owner selects, default `~/workspace`. Owner decision 2026-10-02.
- The scan looks for repositories to a depth that is a runtime setting, default 4, because repositories of the owner are at depth 4. A group folder is a folder between the root and a repository that is not a repository itself. A nested group folder is one item of the pick, shown with its path, for example `group-c/reference`. Agent-derived, accepted 2026-10-03.
- The depth of a repository is the number of path elements of its folder relative to the root: `shop` has the depth 1, and `group-c/reference/shop` has the depth 3. `learn/tree.json` stores the depth setting. Agent-derived, needs owner decision.
- The Go scan never classifies text by its meaning, and the owner confirms each proposal. Owner decision 2026-10-02.
- Premise changed: the rule above was written for a scan with no model. Version 0.6 adds the learner, a model that reads files. Dependent decision re-decided: the rule holds for the Go scan, and the learner has its own eval set (section 20). Agent-derived, needs owner decision.
- The Go scan proposes the projects from the folder names only, because a folder name is structure, not a file of the working tree. Owner decision 2026-10-03. The rule is agent-derived, accepted 2026-10-03: two repositories of one group are one proposed project when the name of one is the full name of the other, followed by `-` or `.` (`shop` and `shop-app`). Each other repository is one project.
- Premise changed: the owner decision of 2026-10-02 had the scan propose the links from manifests in the working tree (`go.mod`, `.gitmodules`, `package.json`, `pubspec.yaml`, and the `FROM` lines of a Dockerfile). The Go scan now reads no file of the working tree. Dependent decision re-decided: the learner proposes each link with its evidence, and the owner confirms it ("Learner", below). Owner decision 2026-10-03.
- The project key is the proposed project name in lower case. Each run of characters that are not an ASCII letter or an ASCII digit becomes one `-`, and a `-` at the start or at the end is removed, so that the key matches the project part of a role key (section 3.2). When two groups have a project with the same name, the key gets the group name as a prefix (`group-a-infra`). A key has at most 40 characters. A role key has at most 64 characters (`mcp/env.go`), so with a key of 40 characters a clanker gives a task name of at most 17 characters. When the rule gives an empty key or a key of more than 40 characters, the confirm asks the owner for the key. Agent-derived, accepted 2026-10-03.
- For a nested group folder, the group prefix of a key is the last element of the group path (`reference-infra` for `group-c/reference`). When the keys still collide, the prefix is the full group path (`group-c-reference-infra`), and a project with no group folder keeps its key. Agent-derived, needs owner decision.
- A project with more than one repository has one main repository, which the owner can change at the confirm. The clanker works in the folder of the main repository. A clerk starts from the main checkout of the repository that its task changes, and a task changes one repository only: the clanker divides a change of two repositories into two tasks. Agent-derived, accepted 2026-10-03.

Index. Owner decision 2026-10-03: the index of a project has the key, a purpose of one line, the main repository, the repositories with their git facts, the links to other projects with the evidence file and line, and the doc pointers. A doc pointer is the path of a file in which a repository explains itself. The index has no stack, no gates, and no build, test, or run command. These stay in the repository, which is their single source. A clanker learns them from the repository at the start of work (section 3.5).

- Premise changed: the owner decision of 2026-10-02 said "for each project, bruh stores the repositories and their code hosts, and the stack and the gates". Dependent decisions re-decided, each agent-derived, needs owner decision:
  - The stored stack and gates go, with their sources `declared`, `convention`, and `unknown`, the hash of the gate files, the gaps, and the re-learn (`relearn`, `DONE: relearn`, its P1, and the tool `learn_set`).
  - The clanker learns the gates from the repository at the start of work, and gives them to its clerks (section 3.5).
  - The diff table of init shows the index (section 16, step 11), and the smoke test checks the index (section 20).
- Premise changed: the owner decision of 2026-10-02 (design.md, L3) said that the map of the docs and the rules is not part of the knowledge. Dependent decision re-decided: the index holds doc pointers, which are paths only, and no text of a doc. Owner decision 2026-10-03.
- bruh stores no live status. bigm reads the open pull requests, the issues, and the pipelines from the code host when the owner asks or when work needs it (principle 1). Owner decision 2026-10-02.

Git facts. Owner decision 2026-10-03: `learn_scan` (Go, with no model call and no workflow) reads only the folder tree and the `.git` folders: the remotes in `.git/config`, the file `.git/refs/remotes/<remote>/HEAD`, the host from the remote URL (and from `ssh -G` for an alias), the host kind, the API root, and the activity on the code host for the pick. It reads no file of the working tree: no Taskfile, no Makefile, no `package.json`, `go.mod`, `pubspec.yaml`, `.gitmodules`, or Dockerfile, and no CI file. The owner said on 2026-10-03: "BRUH YOU MUST NOT RELY ON THE REPO INTERNALS - taskfile, makefile. expect nothing from the repo. bruh, agents must be smart enough to decide without guidance on tasfile/makefile etc."

- Premise changed: the delegated ruling of 2026-10-02 (design.md, L21) had the Go scan find the stack and the gates in the Taskfile, the Makefile, `package.json`, `go.mod`, `pubspec.yaml`, and the CI paths, and the first form of the owner decision of 2026-10-03 (L40) kept these Go facts. Dependent decisions re-decided: the gate rules, the stack rules, the CI system, and the Taskfile reader of the Go scan go, so L21 is replaced in full. Agent-derived, needs owner decision.
- For each repository: the path, the remotes, the host, the host kind, the API root (`api_url` of `repos_set`), and the default branch. Agent-derived, accepted 2026-10-03. The details:
  - The scan removes a user name, a password, or a token from each URL with a scheme other than `ssh` before it stores or shows the URL. The SSH user of the scp form and of an `ssh://` URL (`git@`) stays, because it is the login name of SSH and no secret. A password of an `ssh://` URL does not stay. Agent-derived, needs owner decision.
  - The API root is `https://api.github.com` for `github.com`, `https://<host>/api/v3` for another GitHub host, `https://<host>/api/v1` for Gitea, and `https://<host>/api/v4` for GitLab.
  - The remote is `origin` when the repository has it, or else the first `[remote]` of `.git/config`.
  - The default branch comes from the file `.git/refs/remotes/<remote>/HEAD`. When the file does not exist, the scan records `unknown`, and the clanker finds the branch when it starts work. The scan makes no code host call for it, so the same files give the same JSON. `.git/HEAD` is the branch that is checked out, not the default branch.
  - A repository with no remote, or whose remote is a local path or a `file://` URL, stays in the index. Its host is `{"value": null, "source": "git"}`, its kind is `unknown`, its `api_url` and `host_path` are `""`, and its default branch comes from the rule above, which gives `unknown` when the repository has no remote. Agent-derived, needs owner decision.
- The scan never runs `task`, `make`, or another command of a repository, because these can run code of the repository. Owner decision 2026-10-02 (delegated ruling).
- The details below are agent-derived, accepted 2026-10-03:
  - The scan never runs `git` in a repository, because the git configuration of a repository can run commands. It reads `.git/config` and `.git/refs/` as files.
  - The scan skips a `.git` file (a linked worktree), `.claude/` folders, a repository inside a repository, the ledger, and the folders that the owner excludes, default `archive/`.
  - An exclude entry with no `/` is a folder name at each depth, and an entry with a `/` is a path relative to the root. The scan does not follow a symbolic link to a folder. Agent-derived, needs owner decision.

Learner. Owner decision 2026-10-03: at init, an agent reads every selected project, and proposes the purpose, the links, the doc pointers, and the host of each SSH host alias that did not resolve. The init session can run the agents in parallel with the Agent tool, which is not a workflow. The owner confirms each value at the confirm of init (section 16, step 10). The contract below is agent-derived, needs owner decision.

- The agent is the plugin agent `bruh:learner` (section 17). Its frontmatter gives it only the tools Read, Grep, and Glob, so it reads files and runs no command of a repository (principle 2). Its frontmatter also sets the model of the roles (section 3.3) and the effort `medium`.
- Input, in its prompt: the root; the project key and the paths of its repositories, relative to the root; the keys of the other selected projects, each with the paths of its repositories; and the SSH host aliases of the project that step 6 of init left with no host.
- Output: only these closed lines. The init skill ignores each other line.
  - `PURPOSE: <text>`: one line that tells what the project is, at most 120 characters. A second `PURPOSE` line is refused.
  - `LINK <project key>: <repo>/<file>:<line>`: the project uses the other project, and this line of this file shows it. `<project key>` is a key of the input.
  - `DOC: <repo>/<path>`: a file in which the repository explains itself, for example `README.md` or `CLAUDE.md`.
  - `HOST <alias>: <host>`: the real host of an alias of the input.
- Parse rules: `<repo>` is a repository path of the input, followed by `/`, and the longest path that matches wins. A `LINK` line is split at the last `:` into the file and the line. A path is relative: it has no `..` part and does not start with `/`. The target key of a `LINK` is a key of the input, and not the key of the own project. A value that has a line break or the text ` | ` is refused. The init skill refuses a file that does not exist in the repository, a line that is not from 1 to the last line of the file, and a host that does not match `[a-z0-9.-]+`. A refused line is not proposed.
- The learner proposes no link, doc pointer, or host without evidence in the files. The purpose has no evidence line, so the owner checks it at the confirm. The non-interactive form runs no learner (section 16).
- Verify: the subagent `bruh:learner` has only the tools Read, Grep, and Glob. Verify: the subagent reads a selected repository outside the working folder of the init session, or shows its permission prompt to the owner.

Code hosts and activity.

- The pick shows the last activity of each repository on its code host: GitLab `last_activity_at`, GitHub `pushed_at`, Gitea `updated_at`. The last commit of the local clone is not the activity: on 2026-10-02 the local clone of an active GitLab repository showed a commit that was two months old. Owner decision 2026-10-02.
- bruh reads GitLab through `glab`. Owner decision 2026-10-02.
- GitLab gets the same support as GitHub and Gitea: the pick, the status on demand, `repos_set`, the watcher, and the merge train, all through `glab`. Owner decision 2026-10-02. Posts on GitLab already work through `scripts/post-findings.sh`. The contract below is agent-derived, accepted 2026-10-03:
  - bruh makes each GitLab call with `glab api --hostname <host> <path>`. glab keeps the login of the owner and refreshes its OAuth token, so no token passes through bruh. `scripts/post-findings.sh` gets the same `--hostname`.
  - A GitLab path can have subgroups, for example `group/sub/repo`. So for `gitlab`, the repository grammar of `repos_set`, of the merge grants, of the post grants, and of the merge approvals accepts two or more parts. The API path uses the full path, URL-encoded. A post grant names the host, not only the kind `gitlab`.
  - The host column of a post grant holds the host name, for example `gitlab.com`, `github.com`, or the name of a self-hosted server. `--hostname` of `post-findings.sh` has the default `gitlab.com` for `gitlab` and `github.com` for `github`, so a post grant row that names only the kind does not match. Agent-derived, needs owner decision.
  - The watcher reads the branches (`repository/branches`) for pushes, the notes of each open merge request (`merge_requests/<iid>/notes`), and the latest pipeline of the head SHA (`pipelines?sha=<sha>`). The pipeline state `success` is green, `failed` is red, and each other state is not green.
  - Each pipeline state other than `success` and `failed` (for example `canceled`, `skipped`, or `manual`) is pending: the watcher sends no red event for it. The merge train waits until `--wait-minutes` ends, and then skips the merge request with the reason `checks_timeout`. Agent-derived, needs owner decision.
  - The merge train reads the merge request state `opened` as open, and needs `detailed_merge_status` equal to `mergeable`. It merges with `PUT merge_requests/<iid>/merge` with the confirmed head SHA in `sha`, and `squash=true` only when the grant says squash. The merge method is a setting of the GitLab project. It confirms the merge by reading the state `merged` (section 8.3).
  - The merge train merges at `mergeable`, and waits at `checking`, `unchecked`, `preparing`, `ci_still_running`, and `approvals_syncing`, as at the GitHub value `unknown`. At each other value of `detailed_merge_status`, it skips the merge request with the reason `detailed_merge_status_<value>`. When the response has no `detailed_merge_status`, it skips the merge request with the reason `detailed_merge_status_missing`. Agent-derived, needs owner decision.
  - The `merge_method` of a GitLab repository in `repos_set` is `merge` (the default) or `squash`. bigm passes `squash` only when the merge grant says squash, and the merge train then sends `"squash": true`. Agent-derived, needs owner decision.
  - A remote with an SSH host alias can use another account than the login of `glab`. So the identity of each such repository is not checked, and the merge train and the posts refuse it until the owner confirms the account in "Identities" of the project file (section 13).
  - A repository uses an SSH host alias when the host part of the URL of its chosen remote is not equal to `host.value` of its index, or when `host.value` is `null`. A confirmed account is a data row of the table under "Identities" whose first cell is the alias and whose second cell is not empty, and with no index file of the project, bruh makes no identity check. When `<plugin data folder>/init/config.json` exists but cannot be read or has no `ledger_path`, the merge train and the posts refuse (fail closed). Agent-derived, needs owner decision.
  - Only a role session makes this identity check. A manual session of the owner posts with `--yes` after the owner said yes, and `post-findings.sh` makes no identity check there. Agent-derived, needs owner decision.
  - Verify: each of these `glab api` calls on a GitLab repository of the owner, the reads first, and the merge fields against the GitLab API docs.
- The rules below are agent-derived, accepted 2026-10-03:
  - The host of a remote with an SSH host alias (for example `gitlab.com-work`) is the `hostname` that `ssh -G -- <alias>` prints. This command reads the SSH configuration of the owner and makes no connection. The alias comes from the git configuration of a repository, so the scan accepts only the characters `A-Z`, `a-z`, `0-9`, `.`, and `-`. `ssh -G` prints the host in lower case, so the scan compares without case. Only `learn_scan` runs `ssh -G`. The init skill puts each alias and its host from the output of `learn_scan` into `host_aliases`, and `init_plan` and `learn_refresh` use only that map and the stored host (agent-derived, needs owner decision). When the printed `hostname` is the alias itself, init asks the owner for the host (section 16, step 6). When the owner skips it, the learner proposes the host ("Learner", above), and the confirm asks its kind (section 16, step 10). Agent-derived, needs owner decision. `ssh -G` can run the `Match exec` commands of the SSH configuration of the owner.
  - The host kind comes, in this order, from an exact list (`github.com` is `github`, `gitlab.com` is `gitlab`), from the hosts of the logged-in CLIs (`gh auth status --json hosts`, `glab auth status --all`, `tea logins list`), and from the `BRUH_GITEA_TOKEN_<HOST>` variables. Init asks the kind of each other host before the pick (section 16), so that the pick can show its activity.
  - bruh reads only the host names from the output of these CLIs: the keys of `hosts` of `gh auth status --json hosts`, each line of `glab auth status --all` that has no indent and only a host name, and the host of each `url` of `tea logins list --output json`. Each of these calls has the timeout of 10 seconds. Agent-derived, needs owner decision.
  - bruh reads GitHub through `gh` and Gitea through `tea` or a `BRUH_GITEA_TOKEN_<HOST>` variable. It calls a host only when it has a working login for that host. For each other host, it makes no call and shows "no login".
  - bruh reads `updated_at` of a Gitea repository over HTTPS, only with the `BRUH_GITEA_TOKEN_<HOST>` variable. A `tea` login gives the kind of its host, but a Gitea host with only a `tea` login shows its activity as "no login". Agent-derived, needs owner decision.
  - It runs each CLI with a working folder outside the repositories, so that no configuration of a repository applies.
  - Each call has a timeout of 10 seconds, and at most 8 calls run at the same time. A failed call (no access, not found, a rate limit, a timeout) shows the activity as "unknown" and does not stop the scan.
  - Verify: the `tea` command that reads `updated_at` of a repository.
  - `init_plan` runs no CLI login check. The init skill puts the kind of each host into the answer `host_kinds`, also each kind that `learn_scan` found, so `init_plan` writes the same bytes for the same answers. Agent-derived, needs owner decision.

Schema. The files and the tools of the learn step use this closed schema. Agent-derived, needs owner decision.

- Each value of the index has a source: `git` (read from a `.git` folder or from `ssh -G`), `agent` (the learner proposed it, and the owner accepted it), or `owner` (the owner typed it, or edited a proposal).
- Plugin code writes the keys in the order shown, and sorts each list: the projects by key, the repositories by path, the links by project key, repository, file, and line, and the doc pointers by repository and path. A path is relative to the root. The files hold no time and no activity.
- Plugin code writes each JSON file with an indent of two spaces, and does not escape `&`, `<`, and `>` (the Go encoder with `SetEscapeHTML(false)`). Agent-derived, needs owner decision.
- `learn/tree.json`:

  ```json
  {
    "root": "/home/me/workspace",
    "depth": 4,
    "exclude": ["archive"],
    "host_aliases": {"gitlab.com-work": "gitlab.com"},
    "host_kinds": {"git.example.org": "gitea"},
    "projects": [{"key": "shop", "group": ""}]
  }
  ```

  `group` is the path of the group folder, or `""`. A project that a join made from repositories of two groups takes the group of its main repository. `host_aliases` maps an alias to its host, and `host_kinds` maps a host to `github`, `gitlab`, or `gitea`.
- `learn/projects/<key>.json`:

  ```json
  {
    "key": "shop",
    "purpose": {"value": "Online shop: web client and Go API", "source": "agent"},
    "main": "shop",
    "repos": [
      {
        "path": "shop",
        "remotes": [{"name": "origin", "url": "git@gitlab.com:team/shop.git"}],
        "remote": "origin",
        "host": {"value": "gitlab.com", "source": "git"},
        "kind": "gitlab",
        "api_url": "https://gitlab.com/api/v4",
        "host_path": "team/shop",
        "default_branch": "main",
        "state": "present"
      }
    ],
    "links": [{"project": "auth", "repo": "shop", "file": "go.mod", "line": 5, "source": "agent"}],
    "docs": [{"repo": "shop", "path": "README.md", "source": "agent", "gone": false}]
  }
  ```

  `purpose` is `null` when nobody gave one. `host` has the value `null` and the source `git` when the alias of the repository has no host, because the owner skipped it and no learner gave one. `kind` is `github`, `gitlab`, `gitea`, or `unknown`, and `api_url` is `""` when `kind` is `unknown`. `host_path` is the path of the repository on its code host. `default_branch` is a branch name or `unknown`. `state` is `present` or `missing`. `line` is an integer from 1. `repo` in a link or a doc pointer is the path of a repository of the project. `gone` is `true` while the file of a doc pointer does not exist.
- `learn_scan` input: `ledger` (the absolute path of the ledger, from init step 1), `root`, `depth`, `exclude`, `host_aliases`, and `host_kinds`. Output: `repos`, a list of the repository records above without `state`, each with `group`, `alias` (the SSH host alias, or `""`), and `activity` (`{"at": "<UTC time or empty>", "state": "ok | no login | unknown"}`); `projects`, a list of `{"key", "repos", "main"}`; `host_aliases`, each alias with the host that `ssh -G` printed; `hosts_without_kind`; and `aliases_without_host`. The activity is only in this output, never in a file.
- `learn_scan` always reads the activity of each repository on a host with a working login. The init skill calls it at step 6 and again at step 7 (section 16), so each call can take up to 10 seconds for each group of 8 repositories on such a host. Agent-derived, needs owner decision.
- `learn_refresh` input: none. It reads the path of the ledger from `ledger_path` in `<plugin data folder>/init/config.json`, which init writes, and refuses with a clear error when the file or the key does not exist. Output: `{"written": [<ledger paths>], "missing": [<repository paths>], "gone_docs": [{"project", "repo", "path"}], "long_files": [<ledger paths>]}`. `missing` and `gone_docs` list only the repositories and the doc pointers that changed state at this refresh. `long_files` is the size check of section 8.6.
- When `learn/tree.json` does not exist, for example on a remote machine, `learn_refresh` changes no project file and lists no repository and no doc pointer. It still returns `long_files`. Agent-derived, needs owner decision.
- The init answers give each project as `{"key", "repos", "main", "fills"}`. Each fill has `field` (`purpose`, `link`, `doc`, or `host`), `value`, `source` (`agent` or `owner`), and, except for `purpose`, `repo`. A `link` fill also has `file` and `line`. The value of a `link` is a project key, of a `doc` a path in the repository, and of a `host` a host of the grammar `[a-z0-9.-]+`.
- `init_plan` checks the fills with the value rules: the field grammar, a line from 1, a `repo` of the project, a relative path with no `..` part, a value with no line break and no ` | `, at most one `purpose` for each project and one `host` for each repository, no fill twice, and a `link` target that is a key of the answers and not the key of the own project. It checks that the file of a `doc` fill exists with a stat, and reads no content of a file of the working tree. The init skill checks the `LINK` lines ("Learner", above).
- `init_plan` keeps each stored project JSON that the answers do not change.
- A project of the answers does not change its stored JSON when it has no `fills` key, and its `key`, its `repos` (as a set), and its `main` are equal to the stored file. `init_plan` then keeps the stored file byte for byte, with its `gone` flags and its `owner` values, and builds each other project JSON again from the git facts and its `fills`. Agent-derived, needs owner decision.

Storage. Owner decision 2026-10-02: the result is JSON in the ledger, `learn/tree.json` and `learn/projects/<key>.json`. Plugin code writes these files, and bigm commits them. The rules below are agent-derived, accepted 2026-10-03:

- Plugin code writes a file only when a value changes, so a sweep with no change makes no commit.
- The JSON holds no time and no activity. So `init_plan` and `init_apply` write the same bytes for the same answers and the same files.
- `init_apply` returns the paths that it wrote, and bigm commits these paths at its next turn. The scan settings (`root`, `depth`, `exclude`, `host_aliases`) are in `learn/tree.json`, so that the refresh and a second run use them.
- The init session is not bigm, so bigm does not see the result of `init_apply`. At the start of each turn, bigm commits each change of the ledger that it did not make, which `git status --porcelain` shows. Agent-derived, needs owner decision.
- When the owner removes a project, plugin code deletes its JSON, and git keeps the old version (section 8.6). The project file goes when it has no open item.
- bruh writes no Markdown view of the tree. bigm shows the tree from the JSON when the owner asks. Owner decision 2026-10-02.

Use.

- bigm reads the code host again for each status claim (principle 1). Owner decision 2026-10-02. bigm answers the other questions about the projects from the index. Agent-derived, accepted 2026-10-03.
- bigm routes a work request to a project with the purpose lines and the links, and reads the pointed docs only when it must decide. Agent-derived, needs owner decision.
- The rules below are agent-derived, accepted 2026-10-03:
  - When bigm starts a clanker, it puts the JSON of the project and of the linked projects into the start message, so that the clanker reads no ledger file.
  - bigm takes the input of `repos_set` (the repositories, the host kinds, and the API roots) from the JSON. The init skill does not call `repos_set`, so the watcher polls only the repositories of the projects that have a clanker.
  - The role settings of the clanker get a read-only allow rule for each other repository of its project. So `role_settings_write` gets an input `allow` (section 10.1). It accepts only the form `Read(//<absolute path>/**)` for a repository of the project in the JSON, and only from bigm for a clanker key. A path with one `/` is relative to the settings file, so the rule needs `//`. Verify: a `Read` allow rule covers the tools Read, Grep, and Glob. A read of another repository with Bash stays with the classifier of `auto` mode.
  - An absolute path starts with `/`, so the rule text is `Read(/` + the absolute path + `/**)`, for example `Read(//home/me/workspace/shop/**)`. Agent-derived, needs owner decision.
  - The `allow` check of `role_settings_write` reads the path of the ledger from `ledger_path` in `<plugin data folder>/init/config.json`, and refuses with a clear error when the file or the key does not exist. Agent-derived, needs owner decision.
- Limit: bigm, and a clanker for a linked project, read the pointed docs of another repository through a Read prompt or the classifier of `auto` mode. bruh adds no allow rule for them. Agent-derived, needs owner decision.

Refresh. Owner decision 2026-10-02: the refresh depends on the cost, and it runs at each sweep with no model call.

- At each sweep, bigm calls `learn_refresh`. The tool reads the remotes and the default branch again from the files in `.git` (see "Git facts"), and keeps a stored default branch when the file does not exist. Agent-derived, accepted 2026-10-03.
- A repository that is no longer at its path gets the state `missing`, and bigm sends one P1 for each missing repository. Agent-derived, accepted 2026-10-03. The refresh lists a repository only when it becomes missing, so bigm sends the P1 once. Agent-derived, needs owner decision.
- The rules below are agent-derived, needs owner decision:
  - The refresh keeps a host with the source `agent` or `owner` while the remote URL of the repository does not change.
  - It never deletes a doc pointer. It sets `gone: true` on a doc pointer whose file does not exist, sets `gone: false` when the file is back, and names each change in `gone_docs`, so that bigm names it in the commit. It checks the file with a stat, and does not read it. It skips the doc check of a missing repository.
  - It does not check the evidence of a link. The evidence is the record of the confirm.
  - Premise changed: L6, L14, and L21 had the refresh read the gates again when a gate file changed, and the first form of 2026-10-03 marked `relearn`. Dependent decisions re-decided: the gate hash, `relearn`, and the re-learn go. The index changes rarely, and the owner updates it with a second run of `/bruh:init`: "change projects" runs the learner for the projects that change (section 16).

Tool rules. Agent-derived, accepted 2026-10-03:

- `learn_scan` refuses a caller that has a role key, so only the init skill in a session of the owner calls it, never bigm. It takes the root and the path of the ledger from the init answers.
- A session in the ledger folder has the role key `bigm` (section 3.4). So the init skill checks the role key with `bruh_info` at the start of each run, and stops when the session has one (section 16). The non-interactive form and `init_plan` compute the git facts with the scan code of the MCP server, not through the tool `learn_scan`, so the role key rule of `learn_scan` does not apply to them. Agent-derived, needs owner decision.
- `learn_refresh` refuses each caller except bigm.

Verify:

- The scan of the root takes less than 2 seconds when it makes no code host call.
- A fixture repository with `core.fsmonitor` in its `.git/config` runs nothing.
- The MCP server reads the root without a permission prompt.
- The output of the scan on the repositories of the owner matches a list made by hand.

### 8.6 Ledger size

Owner decision 2026-10-02 (delegated to the agent): the owner said "i want to avoid situation that every new interaction with bigm will be polluting the .md files, constantly growing ... decide how to overcome that". The decision: the Markdown files of the ledger hold only the current state, and git is the history.

- Each Markdown file of the ledger shows only open or live items. When an item closes, bigm deletes its row in the same commit. Version 0.5 kept these rows with a closed state. Items that close: a task that is done, a merge or a deployment that bigm showed in a reply to a message of the owner, a question that has an answer, an item of `owed.md` that the owner got, a decision that a newer decision replaces, a lease that ended, an item of "Waiting on others" that arrived, and the session of a retired role.
- A retired role is a clerk whose result the clanker accepted or whose task the clanker gave up, or a clanker that bigm stopped for good. A session in the state `failed` or `stopped` keeps its row, because the row maps the role key to the session ID for a resume (sections 3.2 and 15).
- The commit that deletes a row names the item in the closed form `close <kind>: <subject>`. `<kind>` is `task`, `merge`, `live`, `question`, `owed`, `decision`, `lease`, `waiting`, `session`, `rule`, or `grant`. The subject of a question is `<role key> <id> - <subject>`, with the question ID of section 5. The body of the commit that closes a question quotes the words of the owner, with the date and the source. In autonomous mode, the body has the decision of bigm and its reasons (section 11). So `git log --grep` finds each closed item, and nothing is lost.
- A question counts as answered when the plugin data folder of bigm has its answer file. bigm does not show such a question again (section 5). Since version 0.6, a question with the same ID and another subject is a new question, and bigm asks the asker to open it again (section 5). Owner decision 2026-10-03. A remote clanker sends a question again with `orca orchestration ask --resume` and the same message ID, and Orca returns the earlier reply. The history in git is not used for this check, because the question counter is in the plugin data folder, and a new install starts it again at 1.
- The init skill updates the text above the first table of each ledger file, and the file `projects/_template.md`, to the text of the plugin version, and keeps each table row. The change is in the diff. So a ledger of version 0.5 gets the rules of this section. Premise changed: the init skill of version 0.5 never changed a ledger file that existed.
- `rules.md` and `grants.md` hold the rules and the grants that apply now. When the owner retires a rule or withdraws a grant, bigm deletes it with `close rule` or `close grant`, and the commit body quotes the words of the owner. Premise changed: the ledger template said "Do not delete a rule; the owner retires it with a new rule".
- Size check: at each sweep, `learn_refresh` also returns each Markdown file of the ledger that has more lines than `ledger_max_lines` in `mode.md`. bigm first deletes each row of a closed item that it missed. When the file still has more lines, bigm sends one P1 for that file, and no other P1 for it until the file is below the cap again. The P1 has the options "raise the cap", "I close items", and "keep it". No commit is refused, so bigm never stops. Version 0.6 has no `pre-commit` hook: a refused commit stops bigm, because bigm commits before it acts.
- The size check counts each `*.md` file under the ledger, except the files under `.git/` and `learn/`. The number of lines of a file is the number of `\n` characters, plus 1 when the file does not end with `\n`. Agent-derived, needs owner decision.
- The details of this section, except the first paragraph, are agent-derived, accepted 2026-10-03. `ledger_max_lines` is a runtime setting with the default 300.

## 9. Loops

### 9.1 Sweep

Agent-derived, accepted 2026-09-30.

- bigm runs the sweep as a recurring `CronCreate` task every 15 minutes, and a message wakes it sooner. `CronCreate` tasks are restored on resume. A recurring task expires after 7 days, so bigm creates it again every 6 days.
- The sweep reads all report files, reconciles `claude agents --json --all` and Orca `worker-list`, reads the source for each row past its next check, sends the P1 batch when it is due, and updates the ledger.
- A `Monitor`, a background command, and a self-paced `/loop` are not restored on resume. So on a resume, bigm starts the watcher (section 9.2) and the Orca receive loop again. In the setup of the owner, the sweep loop died on a usage limit, and nobody restarted it.
- Verify: `CronCreate` tasks run in an interactive bigm while the owner is away from the terminal.
- At each sweep, bigm calls `learn_refresh` (section 8.5) and commits a change of the files of the learn step. Owner decision 2026-10-02: the refresh at each sweep. Agent-derived, accepted 2026-10-03: the tool and the commit.
- The refresh makes no model call and starts no agent (owner decision 2026-10-02). The commit of a refresh names each doc pointer that changed its `gone` state (section 8.5). Agent-derived, needs owner decision.

### 9.2 Watcher

Agent-derived, accepted 2026-09-30.

- A plugin script, run by a background `Monitor` in bigm, watches the code host for events of the projects: new pushes, replies, red pipelines, merges. It writes each event to the report file of the project of the event, not to one shared file. Owner decision 2026-10-03. In the setup of the owner, sessions reviewed a change, left comments, and then did nothing more until a watcher was added.
- The details below are agent-derived, needs owner decision:
  - The report file of a project is `<plugin data folder>/reports/clanker-<project>.jsonl`, the report file of its clanker. `<project>` is the project of the repository in `repos_set`. `mcp/watch.go` writes the one file `reports/watcher.jsonl` today, so the code changes.
  - `repos_set` requires `project` for each repository, and checks it with the project grammar of a role key (`projectRE`, section 3.2). The watcher skips a stored entry with no `project` and logs it, and bigm calls `repos_set` again for that repository.
  - The watcher and `report_write` append whole lines with one write each to a file that is open with `O_APPEND`, so no lock is necessary.
  - One rule for the events: bigm reads each event from the line of the `Monitor`. When the clanker of the project must act, bigm relays the event to it: with `mail_post` for a local clanker, and with Orca `send` for a remote clanker (section 4.2). The relay has the header `DONE: event <project>: <subject>`. At each sweep, bigm reads the report files with `since` only to find lines that the `Monitor` missed (section 9.1). `report_read` has no role key `watcher`.
  - When bigm retires the clanker of a project, it removes the repositories of that project from `repos_set`, so that the watcher stops its polls of them.
- An agent marks each post it makes with a structural marker, so that the watcher separates agent posts from human posts by structure, not by wording.
- The merge train refuses a stored entry of `repos_set` with no `project`, with the same log text as the watcher, so that no merge runs without a project for the identity check. Agent-derived, needs owner decision.
- `repos_set` keys each entry by `repo` only, and refuses the same `repo` with another `api_url`, so bigm removes the old entry first. Agent-derived, needs owner decision.
- On GitLab, the watcher skips each note with `system: true`, and reads no review events, because GitLab gives none. Agent-derived, needs owner decision.

### 9.3 Rules

- A new owner rule goes into `rules.md` word for word, and bigm broadcasts it with a `RULE` header to all running sessions. In the setup of the owner, memory notes reached only the sessions that started after the note. Agent-derived, accepted 2026-09-30.

### 9.4 Status report

Agent-derived, accepted 2026-09-30.

- bigm reports status in three parts: "Ready for you", "Waiting on you", "In progress". Commands that the owner must run are in code blocks, never "see above". The setup of the owner used this form.
- The status cadence is a runtime setting in `mode.md`: report only on change, or always.

### 9.5 Monitors

Status: Design for owner review, not built.

Owner decision 2026-10-04: a role starts a monitor on demand for each external state that its work depends on, and the mechanism is general, for each developer who installs bruh. The examples of the owner (a push, a task tracker, team work in another system) show the range only. bruh does not hard-code them. The words of the owner are in design.md, L50.

The verified facts for this section are in knowledge.md, "Monitors, background tasks, and event delivery". Each part below gives the documented options first, ranked, and a custom design last. Each decision bullet carries its tag. The open choices M1 to M9 are in part k. They are not decided, and section 22, item 5, lists them.

Terms:

- Monitor: one subscription. It has a source, a subscriber, a project, a reason, and an expiry.
- Source: the description of one external state, as data (part b).
- Source key: the canonical identity of a source. Monitors with the same source key share one poll.
- Subscriber: the role key that gets the events of the monitor. The subscriber owns the monitor.
- Poller: the process that polls the sources and writes the events.
- Wake: the step that makes the session of a subscriber read its mailbox.
- Event: one change of a source, as one JSON line.

#### a. Who starts a monitor

- Each role can start a monitor: bigm, a clanker, or a clerk. The subscriber is the caller. Agent-derived, needs owner decision.
- The general trigger: the next step of the role waits on a state outside its session, and that state can change without an action of the role. Examples, as categories only: the result of checks after a push, a reply of a person in another system, the state of a task that the role took from a tracker, and the progress of work of others in a shared system. Agent-derived, needs owner decision.
- A role does not wait with `sleep`, and does not ask the same source again in a loop. Each such wait becomes a monitor. Agent-derived, needs owner decision.
- The start goes through a new MCP tool, `monitor_start`. It takes the source, the project, the reason, and `until`, and returns the monitor ID, the source key, the expiry, and `shared` (true when another monitor already polls the same source key). Agent-derived, needs owner decision.
- `monitor_start` runs the first poll at once and returns its result as the baseline, with the source read of principle 1 (the call, the value, and the UTC time). After the baseline, events are changes only. So a change that happens before the first periodic poll is not lost. Agent-derived, needs owner decision.
- A workflow agent does not start a monitor, because a monitor outlives the run. It asks its clerk, and the clerk starts the monitor. Agent-derived, needs owner decision.

#### b. What a monitor watches

- Plugin code stores each monitor in `<plugin data folder>/monitors.json` (principle 3). The poller reads the file again before each poll, as the watcher of section 9.2 reads `repos.json` today. Agent-derived, needs owner decision.
- A source has a closed schema. An unknown key is an error. Agent-derived, needs owner decision. A source has `kind` and exactly one object of that kind:
  - `codehost`: a repository of a code host kind of section 8.5, with `repo`, `host`, and `api_url`, and optional `number` (a pull request or a merge request) or `ref` (a branch). The events are the events of section 9.2: push, red checks, comment, review, and merge.
  - `command`: a read-only CLI that prints JSON. `argv` is an array, with no shell. The CLI keeps its own login.
  - `http`: a GET of an `https` URL that returns JSON.
- A `command` or `http` source also has `items` (a JSON pointer, RFC 6901, to an array, or `""` for the root), `id` (a pointer into each item), `version` (a pointer to a field that changes when the item changes, for example an update time or a state), and the optional `title` (a pointer to a text for the subject). The poller emits `new` for a new `id`, `changed` for a new `version`, and `gone` for an `id` that is not there any more. These rules use structure and exact values only, never the meaning of words (the owner rule against regex classifiers). Agent-derived, needs owner decision.
- Each monitor also has `project`, `reason` (one line), `until` (part d), and the optional `interval_seconds` (part g). Agent-derived, needs owner decision.
- The source key is `codehost:<host>:<repo>` with the number or the ref, `command:` with the SHA-256 of the `argv` elements joined with a NUL byte, or `http:` with the SHA-256 of the URL and the pointers. Agent-derived, needs owner decision.
- Examples use placeholders only: `owner/repo`, `https://api.example.com/v1/items?project=demo`, and `["tracker-cli", "list", "--json"]` for an issue tracker CLI. bruh names no tracker and no code host product as a built-in case. Agent-derived, needs owner decision.
- A system that has only an MCP server, with no CLI and no HTTP API that the poller can call, is open choice M5 (part k). Not decided.
- A `command` source runs a program outside the Bash permission rules, because the poller starts it and not the Bash tool. Its approval is open choice M6 (part k). Not decided.

#### c. How events reach the role

- Durable first, as section 5 says for messages. Agent-derived, needs owner decision. Options, ranked:
  1. The poller writes each event to the mailbox of each local subscriber, with the existing header `DONE: event <project>: <subject>` and the event line as the body. It also writes the event to the report file of the project, `reports/clanker-<project>.jsonl` (owner decision 2026-10-03, section 9.2), so that bigm sees it at the sweep. (recommendation)
  2. The poller writes only to the report file of the project, and bigm relays each event, as today (section 9.2).
  3. A new event file for each role, with a new read tool.
- The recommendation keeps one inbox for each role and one read tool (`mail_read`). The rule of bigm for idle roles (resume a role that has unread mail at each sweep) then covers events too. The poller writes the mail with the sender `bigm`, because it polls for bigm (open choice M2), and the mailbox allows a post from `bigm` to each role (`mail.go`). Agent-derived, needs owner decision.
- The event adds no new header. The body is one JSON line with `at`, `monitor` (the ID), `key`, `type`, `subject`, the optional `ref`, `number`, and `url`, and `source` (`call`, `value`, `at`, principle 1). Agent-derived, needs owner decision.
- For a remote subscriber, the poller writes the event to the report file of the project and prints it, and bigm relays it through Orca with the same header, as section 9.2 says today. Agent-derived, needs owner decision.
- The poller prints a line for bigm only for a remote subscriber, for an `error` event, and for a monitor of bigm itself, so that events of other roles cost no context of bigm. Agent-derived, needs owner decision.
- The wake of the subscriber session is open choice M1. A channel (section 12) is not an option for the wake: it is a research preview, and a custom channel needs the development flag with a warning dialog at each launch of each role. Agent-derived, needs owner decision.

#### d. Lifetime, expiry, and stop

- `until` is required. It is a UTC time. When the caller gives none, the default is `monitor_default_hours` of `mode.md`, and `monitor_start` refuses a time past `monitor_max_hours`. Both are runtime settings (section 11). Their default values are open choice M9. Agent-derived, needs owner decision.
- A standing monitor has no `until`: the codehost monitors of a clanker (part i) end when bigm retires the clanker. Agent-derived, needs owner decision.
- At `until`, the poller stops the polls of that monitor and sends one `expired` event to the subscriber, so that the role can start it again when its work still needs it. This follows the notice of the `Monitor` tool at its deadline. Agent-derived, needs owner decision.
- The subscriber stops its monitor with the new MCP tool `monitor_stop` when its work no longer waits on the source. The parent of the subscriber and bigm can also stop it. Agent-derived, needs owner decision.
- At each sweep, bigm calls `monitor_stop` for each monitor whose subscriber is a retired role (section 8.6). Agent-derived, needs owner decision.
- A monitor of a clerk at the end of its task is open choice M4 (part k). Not decided.

#### e. Re-arm after a resume or a compaction

- The monitors are data in the plugin data folder, and the cursor of each source key is in `watch/state.json`, as today. So the poller keeps its polls while a subscriber session is down, and the events wait in the mailbox. No event is lost. Agent-derived, needs owner decision.
- The handoff of each role lists the ID of each active monitor in "Waiting on others" (section 7). After a pickup, the role calls the new MCP tool `monitor_list` as the live source before it acts (section 7). Agent-derived, needs owner decision.
- The re-arm of the wake depends on M1: option 1 re-arms through the `SessionStart` hook with the matchers `resume` and `compact`, option 2 starts again with each session, option 3 re-arms from the handoff, and Claude Code restores the task of option 4 on a resume. Agent-derived, needs owner decision.

#### f. Credentials and identity

Owner decision 2026-09-29 (section 13): a read-only token read inside a process is allowed, and bruh never moves or copies a credential.

- `codehost`: the token rules of today (`hostToken` in `codehost.go`), and `glab api` for that host kind, so that the CLI keeps the login. The use of `glab api`: owner decision 2026-10-02 (design.md, L18). The use of these rules for monitors: agent-derived, needs owner decision.
- `command`: bruh passes no token. The CLI uses its own login. Agent-derived, needs owner decision.
- `http`: the poller reads a token only from the environment variable `BRUH_TOKEN_<HOST>`, where `<HOST>` is the host name with the rule of `envHostKey`. It sends the token only to that exact host, over `https`, and follows no redirect to another host. `monitor_start` refuses a URL with user information, and a URL with a query parameter whose name is in an exact list (`token`, `access_token`, `private_token`, `api_key`, `key`), so that no token is stored in `monitors.json`. Agent-derived, needs owner decision.
- A monitor makes no write. So the identity check before the first write (section 13) does not apply. `monitor_list` shows the name of the credential of each source (the variable name or the CLI), never its value. Agent-derived, needs owner decision.

#### g. Rate limits and shared polling

- One poll for each source key, shared by all its subscribers. Each subscriber gets each event. Agent-derived, needs owner decision.
- A minimum interval for each kind: the codehost interval of `repos.json` (default 60 seconds, at least 10, as today), and at least 60 seconds for `command` and `http`. `monitor_start` raises a smaller `interval_seconds` to the minimum. Agent-derived, needs owner decision.
- Back-off: on HTTP 429 or 503, the poller waits for `Retry-After` when the response has it. Otherwise, after each failed poll, it doubles the interval of that source key, up to 30 minutes, and goes back to the normal interval after the next good poll. A failed `command` (an exit code other than 0, or output that is not JSON) counts as a failed poll. Agent-derived, needs owner decision.
- One `error` event for each new error text of a source key, as the watcher does today. Agent-derived, needs owner decision.
- `monitor_max_active` of `mode.md` caps the source keys that the machine polls. `monitor_start` refuses a new source key past the cap with a clear error, and the role sends a P2 to its parent. The default is open choice M9. Agent-derived, needs owner decision.
- A `command` runs with a time limit of 30 seconds and at most 1 MiB of output. An `http` response has the same size limit. Agent-derived, needs owner decision.
- A source on a paid API is a shared resource (section 8.4). `monitor_start` refuses a source whose host is the name of a resource of `lease_define` when the caller holds no lease of that resource. This is a check in code (principle 2), not a note. Agent-derived, needs owner decision.

#### h. How the ledger shows the active monitors

Open choice M7 (part k). Not decided. The recommendation reads the monitors on demand with `monitor_list`, because the owner decision of 2026-10-02 says that bruh stores no live status (design.md, L24), and the ledger holds only the current state (section 8.6).

#### i. How the watcher of section 9.2 fits

- The watcher of today runs in a `Monitor` tool watch of bigm (section 9.2). A watch of the `Monitor` tool has a deadline of at most 30 minutes, and 5 minutes when Claude passes no `timeout_ms` (knowledge.md). The watcher then stops, and a live event waits until bigm starts it again. bigm finds the missed lines with `report_read` at the next sweep (section 9.1), up to 15 minutes later. This is a likely defect of bruh (design.md, L50, F1). The choice of M2 fixes it. Agent-derived finding. Verify: a probe of bigm shows when the watch of the watcher ends.
- The fate of the watcher is open choice M8 (part k). Not decided. The recommendation folds it in: the watcher becomes the poller, with the source kind `codehost`, and each repository of `repos_set` becomes a standing monitor of the clanker of its project. So the rule of L15 holds: a project with no clanker gets no events. The merge train keeps `repos.json` for its configuration.

#### j. Test plan

- Go tests of the poller, with the standard library only: a fake HTTP server (`net/http/httptest`), and fake CLIs, as `ghBin` and `glabBin` fake `gh` and `glab` today. Agent-derived, needs owner decision.
  - Two subscribers of one source key cause one poll, and each subscriber gets each event.
  - The first poll is the baseline and returns the source read. Then `new`, `changed`, and `gone` follow the pointers, and no rule reads the meaning of a text.
  - A restart of the poller continues from the cursor and emits no event twice.
  - At `until`, the polls stop and one `expired` event goes to the subscriber. `monitor_start` refuses a time past `monitor_max_hours`.
  - `monitor_stop` of a retired subscriber stops its polls, and a shared source key stays while it has another subscriber.
  - On 429 with `Retry-After`, the next poll waits for that time. On other failures, the interval doubles up to 30 minutes, and one `error` event goes out for each new error text.
  - An `http` token goes only to its own host, never after a redirect to another host, and `monitor_start` refuses a URL with user information or a query parameter of the exact list.
  - A `command` source that is not approved (M6) is refused, and an `argv` runs with no shell.
  - `monitor_start` refuses a new source key past `monitor_max_active`, and a source on a lease resource without a lease.
  - Each local subscriber gets the event in its mailbox with the header `DONE: event <project>: <subject>`, and the report file of the project gets the same line.
- Script tests in `tests/test.sh` for the wake of M1: one waiter for each role key, and no waiter when `BRUH_ROLE_KEY` is not set. Agent-derived, needs owner decision.
- Smoke test steps (section 20): a clerk starts a monitor on a fake source, a change of the source wakes the idle clerk, and after a resume of the clerk the next change wakes it again. Agent-derived, needs owner decision.
- Each "Verify" fact of knowledge.md that the selected options depend on gets a test or a smoke test step before the build depends on it (section 20). Agent-derived, needs owner decision.

#### k. Open choices

These choices are not decided. Each option list is ranked, and one option of each list is tagged "(recommendation)". Only the owner decides them. Section 22, item 5, lists them.

M1. The wake of a subscriber session. Options, ranked:

1. An `asyncRewake` hook (documented). A plugin `Stop` hook, and a `SessionStart` hook with the matchers `startup`, `resume`, and `compact`, start a waiter with `asyncRewake: true`. The waiter exits with code 2 when the mailbox of its role key has a new message, so Claude wakes at once, also when the session is idle, and calls `mail_read`. The waiter does nothing when `BRUH_ROLE_KEY` is not set. A new waiter replaces the old waiter of the same role key, because Claude Code does not deduplicate async hooks. Plugin hooks run in each session type. Claude Code enforces `timeout` on this hook, so the waiter exits with code 2 and the line "no new mail" before its timeout, which costs one short turn for each timeout period. Verify: the largest `timeout`, and the idle stop of a background session with a running waiter. (recommendation: it wakes an idle session at once in each session type, re-arms at the end of each turn and after each resume and compaction, and costs a turn only for an event or a timeout.)
2. A plugin monitor (documented) that tails the mailbox of the role key of its session, for the whole session, with no deadline. Plugin monitors start only in interactive sessions, so a role that runs with `claude --bg` can have none (Verify). A running monitor counts as working, so the supervisor never stops the idle process of the role. The monitor also starts in each manual session of the owner, so it exits at once when `BRUH_ROLE_KEY` is not set (Verify: the monitor process gets that variable).
3. The `Monitor` tool (documented). The role starts a watch of its mailbox, and starts it again at each deadline notice and after each pickup. The Bash rules and the classifier apply to it. A watch lasts at most 30 minutes, so each role spends one turn each 30 minutes for the re-arm, and a role that misses a notice has no wake until its next turn.
4. A `CronCreate` task (documented) that reads the mailbox each N minutes. Claude Code restores it on a resume. It fires only when the session is idle, at a granularity of 1 minute, up to half of the interval late, with one turn for each fire also when no event came, and it expires after 7 days.

M2. The host of the poller. Options, ranked:

1. A plugin monitor (documented). `monitors/monitors.json` of the plugin starts `scripts/watcher.sh` in each interactive session. The script polls only when `BRUH_ROLE_KEY` is `bigm`, and else exits at once. bigm stays an interactive session (section 3.4), so plugin monitors start there. The monitor runs for the whole session, with no deadline, and each line that it prints reaches bigm as a notification, as the line of a `Monitor` tool watch does today. A lock file in the plugin data folder keeps one poller for each machine, also when the owner starts a second bigm (section 3.4). Verify: the monitor process gets `BRUH_ROLE_KEY` from the start settings of bigm. (recommendation: it has no deadline and needs no re-arm, so it fixes the defect of part i with the least change.)
2. The `Monitor` tool in bigm, as today, and bigm starts it again at each deadline notice. This costs one turn of bigm each 30 minutes, and each missed notice leaves a gap until the next sweep.
3. A loop in the bruh MCP server of the session of bigm (custom). The Slack server of bruh already polls only when the role key is `bigm` (section 3.4). It needs no Claude Code feature, but it is new code with the same lock as option 1.
4. A service of the operating system (custom). It polls also when bigm is down, but it needs install and uninstall steps outside the plugin, for each supported operating system.

M3. The topology of the polls. Options, ranked:

1. One poller on the machine of bigm for all subscribers. bigm relays each event of a remote subscriber through Orca, as section 9.2 says today. (recommendation: one cursor file, one rate limit for each source key, and no lock among sessions.)
2. A poller in the session of each subscriber, with a shared cursor and a shared result for each source key under a file lock. The polls continue when bigm is down, but each session runs a poller, and the lock rules are new.

M4. A monitor of a clerk at the end of its task. Options, ranked:

1. Hand over to the clanker. When the clanker accepts the result of the clerk or gives up the task, each monitor of the clerk becomes a monitor of the clanker, and the clanker stops each monitor that its work does not need. (recommendation: work can still wait on an external state after the clerk ends, for example a review of a delivered pull request.)
2. Stop with the clerk. At the accept, the clanker stops each monitor of the clerk, and starts a new monitor when it needs one.
3. The clerk lists its monitors in its result, and the clanker decides for each one at the accept.

M5. A system that has only an MCP server. Options, ranked:

1. A poll by the role (documented). The subscriber session runs a `CronCreate` task that calls the MCP tool of that system, and reports each change with a new MCP tool `monitor_report`, which writes the event in the form of part c. (recommendation: the MCP server keeps its own login, so no credential moves, and only these systems cost turns.)
2. Sources with a CLI or an HTTP API only. A role reads a system that has only an MCP server on demand.
3. The poller starts the MCP server of the source itself and calls its tool (custom). This needs the command and the credentials of the server outside Claude Code, which can move a credential (section 13), and it does not work for a remote MCP server whose login is in Claude Code.

M6. The approval of a `command` source. Options, ranked:

1. A command grant. The owner approves a CLI with its fixed first arguments, and bigm records it in `grants.md`, as a merge grant or a post grant (sections 8.3 and 13). `monitor_start` accepts a `command` source only when its `argv` starts with the `argv` of a grant, by exact match. (recommendation: a check in code (principle 2), and the owner sees each program that runs outside the permission rules.)
2. A `command` source runs only in a `Monitor` tool watch of the subscriber session, so the Bash rules and the classifier apply. Such a source has no shared poll, and it has the deadline of 30 minutes.
3. No `command` kind. Only `codehost` and `http` sources.

M7. How the ledger shows the active monitors. Options, ranked:

1. On demand only. `monitor_list` returns the active monitors, and bigm shows them under "In progress" of the status report (section 9.4) when the owner asks. The ledger stores nothing. (recommendation: bruh stores no live status (owner decision 2026-10-02, L24), the ledger holds only the current state (section 8.6), and `monitors.json` is already the durable record.)
2. A table "Monitors" in each project file, which bigm updates at each sweep. It shows in git, but each start and stop adds a commit, and the copy can be out of date.
3. A separate ledger file, `monitors.md`. One place, with the same commits as option 2, and one more file for the size check.

M8. The watcher of section 9.2. Options, ranked:

1. Fold it in. The watcher becomes the poller with the kind `codehost`, and `repos_set` makes the standing monitors of the clanker of each project. (recommendation: one poller, one cursor file, and one set of rules.)
2. Keep both. The watcher stays as today, and the monitors have a second poller. Two cursor files and two code paths, and a repository can get two polls.
3. Replace it. No standing monitor: each role starts a monitor for each pull request or branch that it needs. A push of a person to a project with a clanker then gives no event, which changes L15.

M9. The defaults of the runtime settings `monitor_default_hours`, `monitor_max_hours`, and `monitor_max_active`. Options, ranked:

1. 24, 168, and 20. (recommendation: one day by default, a maximum equal to the 7-day expiry of a `CronCreate` task, and a small cap, because a `codehost` poll makes several API calls each minute.)
2. 8, 72, and 10.
3. 72, 336, and 50.

## 10. Files, the MCP server, and environments

### 10.1 Files and the MCP server

Owner decision 2026-09-29: bruh keeps all its files in the plugin data folder `${CLAUDE_PLUGIN_DATA}`, and plugin code writes them. Claude Code guards `~/.claude` against writes by Claude, but not against writes by plugin code. The only files outside the plugin data folder are the settings keys that the init skill writes after the owner approves the diff, and the private ledger repository.

The plugin ships a stdio MCP server written in Go with the standard library only. Claude Code starts it with `go run -C ${CLAUDE_PLUGIN_ROOT}/mcp .` and `GOTOOLCHAIN=local`, so the repository holds no binaries. Owner decision 2026-09-30. Its tools: `mail_post`, `mail_read`, `handoff_write`, `handoff_read`, `answer_write`, `answer_wait`, `report_write`, `role_settings_write`, `lease_request`, `lease_grant`, `lease_release`. It reads the role key of its caller from `BRUH_ROLE_KEY`. Agent-derived, accepted 2026-09-30. Verify: an MCP server started by a session gets the `env` values of its `--settings` file. Version 0.6 adds the tools `learn_scan` and `learn_refresh` (section 8.5; agent-derived, accepted 2026-10-03), the field `options` of `question_open` (section 14.2; owner decision 2026-10-02), and the input `allow` of `role_settings_write` (section 8.5; agent-derived, accepted 2026-10-03).

The input and the output of `learn_scan` and `learn_refresh` are in section 8.5, "Schema". Agent-derived, needs owner decision.

### 10.2 Local and container

Owner decision 2026-09-30: the skillset runs the same way on a local machine and in a container. For autonomous work, the container image is `ghcr.io/oter/autonomous-agents/agent`. The image is infrastructure and is outside this repository. The details below are agent-derived, accepted 2026-09-30.

- The plugin supports macOS and Linux. Scripts use POSIX shell and portable commands.
- In a container, the plugin data folder must be on a volume, or the handoffs, the mailboxes, and the leases die with the container.
- In a container, the init skill runs in its non-interactive form (section 16). The answers then give the selected projects with the keys `root` and `projects`. Agent-derived, accepted 2026-10-03.
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
- bigm writes the numbered options into the text that it sends to the channel, one line `<n>. <label>: <description>` for each `OPTION` line, so the Slack server of bruh does not change. Agent-derived, needs owner decision.
- A channel that declares permission relay lets the owner approve a permission prompt of bigm from the chat. It does not reach the prompts of other sessions, so a P0 for a prompt of a clanker or a clerk still carries `claude attach <id>`. Only the owner is on the sender allowlist of a channel. Verify: the permission relay and the sender allowlist of each channel.

## 13. Never without the owner

Agent-derived, accepted 2026-09-30, except where tagged. The list is data in `priorities.md`, shipped as a default and copied by the init skill. It is a hard stop in both modes. Owner decision 2026-09-29.

- An irreversible or outward-facing action: publish, deploy, delete, send, merge (except under a merge grant), post a review result on a pull request (except under a post grant, section 6.4). A post grant, like a merge grant, is an answer that the owner gives in advance for one repository and one role key. Agent-derived, accepted 2026-10-03: the post grant.
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

Owner decision 2026-09-29: `priorities.md` lists the delegated P1 classes that a clanker may answer. Since version 0.6, the init skill does not ask for them: the owner tells bigm, and bigm writes the class into `priorities.md`. Agent-derived, accepted 2026-10-03.

### 14.2 Routing

- The clanker answers P2 and delegated P1 classes, and logs each answer. It sends P0 and the other P1 questions to bigm. Owner decision 2026-09-27 and 2026-09-29.
- The clanker decides the final P-level. Agent-derived, accepted 2026-09-30.
- A clanker never answers a P1 question outside the delegated classes. It can add a `REC`. Agent-derived, accepted 2026-09-30.
- bigm shows a P0 at once, at the top of its next reply and through the channel. Owner decision 2026-09-27.
- A P0 for a permission prompt carries the command `claude attach <id>`. A P0 for a classifier refusal carries the exact command and the refusal category, and gives the owner two options: run it, or add a scoped allow rule. Agent-derived, accepted 2026-09-30.
- bigm queues P1 questions and shows them as a batch. Owner decision 2026-09-27. The interval and the maximum batch size are runtime settings in `mode.md`, asked by the init skill and changeable at any time. Owner decision 2026-09-29. Agent-derived, accepted 2026-09-30: the defaults are an interval of 60 minutes and at most 5 items; a batch goes out early when 5 items are queued; an empty queue sends nothing; a P0 never waits for a batch.
- Owner decision 2026-10-02: `question_open` takes an optional field `options`: 2 to 4 options, each with a label and a description. bigm shows each P1 question that has options as a select (`AskUserQuestion`), at most 4 questions on one screen, and more screens when the batch has more. bigm shows each question without options as text. This replaces "bigm shows the items of a batch one for each message" of version 0.5.
- The question file of the asker keeps the options. `question_open` returns the body of the message with the options in it, and the asker sends that body. The options travel in the body, one line for each option in the closed form `OPTION <n>: <label> | <description>`, because bigm cannot read the question file of another role and a remote clanker is on another machine. A clanker or a clerk that relays a question copies these lines word for word. bigm reads the options only from these lines. Agent-derived, accepted 2026-10-03.
- The label of an option has 1 to 60 characters on one line, with no `|`. The description has 1 to 200 characters on one line. Agent-derived, needs owner decision.
- The question prompt of each workflow tells an agent to pass `options` to `question_open` when the question has fixed answers, and to return the body that `question_open` returned. Agent-derived, needs owner decision.
- Owner decision 2026-10-02: bigm shows the selects only in a reply to a message of the owner. At another time, bigm shows the batch as text and tells the owner to reply to answer it with selects. An open select holds the turn of bigm, so the sweep and a new P0 would wait. Verify: an open select holds the turn of bigm.
- A `REC` that recommends an option has the subject `OPTION <k>`. bigm puts that option first, and its label says "(Recommended)". Agent-derived, accepted 2026-10-03.
- bigm sends the answer to the session that asked, as an `ANSWER`, and records the question and the answer in the ledger. Agent-derived, accepted 2026-09-30. Since version 0.6, the record is the commit that closes the question (section 8.6). Agent-derived, accepted 2026-10-03.

## 15. Failure handling

Agent-derived, accepted 2026-09-30, except where tagged.

- **A reboot comes first.** After a shutdown, a background session shows as `failed` for up to 48 hours, and as `stopped` after that. An attach or a reply restarts it. So bigm checks for a reboot before it applies the rules below: when many sessions changed to `failed` or `stopped` at the same time, bigm reconciles the ledger from `claude agents --json --all`, updates the role key map, and resumes the long-lived roles with `claude --resume <session ID> --bg`.
- **A session waits on a prompt:** `waitingFor` equal to `permission prompt`, or a waiter timeout, raises a P0 at once. In the setup of the owner, a session that waited on a prompt stalled a coordinator for 34 minutes, and nothing noticed.
- **A classifier refusal:** a P0 (section 14.2).
- **A usage limit:** not a crash. The session stops and reports. bigm does not change the model. bigm probes the account before it claims that the account is out of quota, and sends a P1 so that the owner chooses the account. Owner decision 2026-09-29. A workflow in a background session does not pause at a usage limit: its agents fail. After the limit resets, the clerk launches the workflow again with `resumeFromRunId` and the same `args`. The agents that completed return cached results, and the failed agents and the agents after them run again. In the setup of the owner, a usage limit stopped all sessions for about 8 hours, and 23 workflows resumed this way.
- **A session in state `failed` or `stopped`** without a reboot, that did not finish its task: the parent restarts it once with `claude respawn <id>`. If it fails again, the parent raises a P0. Verify: `claude respawn`.
- **A burst refusal:** a sender that gets a refusal waits and sends again with the next attempt counter.

## 16. Init skill

Owner decision 2026-09-27: the plugin has an init skill that asks the user questions. Owner decision 2026-10-02: the init skill also runs the learn step (section 8.5). The owner gave this feedback after the first run of version 0.5: the 13 questions did not tell what each answer controls, there was no step to see and select the projects, and the questions were not interactive. Agent-derived, accepted 2026-10-03: the answer to the feedback, which is one line for each question that tells what its answer controls, and a select (`AskUserQuestion`) with the default first for each question with fixed answers.

Before init: the `userConfig` dialog of the plugin asks the plugin options when the owner installs the plugin, among them `user_name`, `handoff_percent`, and `max_busy_clerks`. The owner changes them in `/config`. The init skill does not ask them, and `init_plan` does not write them. Owner decision 2026-10-02. Premise changed: the owner decision of 2026-09-27 made "how should bruh address you?" the first init question. The install dialog now asks it, and the option is still `user_name`. The first run of version 0.5 showed the problem: the install dialog had set `handoff_percent` to 55 and `max_busy_clerks` to 30, and init asked again and replaced both. The title of each option starts with "bruh: ", for example "bruh: Your name", and the keys do not change. Owner decision 2026-10-02. Verify: a row of `/config` shows the title of the option. If it shows the key, the keys get the prefix `bruh_`, and init moves the saved values (agent-derived, accepted 2026-10-03).

`/bruh:init` has these steps. Each step is agent-derived, accepted 2026-10-03, except where tagged.

At the start of each run, before any question, the skill calls `bruh_info`. When it returns a role key, the skill stops. It tells the owner to run `/bruh:init` again from a session with no role key: `claude --setting-sources user` in the ledger folder, or a session in a folder outside each clone of the ledger. A plain `claude` in the ledger folder is bigm (section 3.4), and `learn_scan` refuses a caller with a role key (section 8.5). Agent-derived, needs owner decision.

1. Ledger: a select of the ledger folders that the skill finds (a repository with `mode.md`) in the current folder and below the default root, plus another path. When the skill finds none, it asks for the path as text. The skill creates the layout of section 8 for each file that does not exist. The skill finds the ledger folders with Glob (`mode.md` at a depth of 1 to 4 below the default root, and in the current folder), and keeps each folder that has a `.git` folder. Agent-derived, needs owner decision.
2. Mode: human or autonomous. Agent-derived, accepted 2026-09-30.
3. P1 batch interval, P1 batch size, and review-round cap: one select each, with the default first. Owner decision 2026-10-02, which keeps the owner decision of 2026-09-29 that init asks the batch interval and size.
4. "Defaults for the rest" or "customize". "Customize" asks the auto-compact window, the status line tap (the skill shows the current and the new value), and the channels. "Defaults" keeps each value that the settings already have, and sets only the missing ones.
5. Root folder: `~/workspace`, or another path. Owner decision 2026-10-02.
6. Hosts: for each host that section 8.5 cannot find, its kind, and for an SSH host alias that `ssh -G` does not resolve, its real host, so that the pick can show the activity. Since version 0.6, the owner can skip the host of an alias. The pick then shows its activity as "unknown", the skill gives the alias to the learner of its project (step 9), and the learner proposes the host. Agent-derived, needs owner decision. A host that the owner types goes into `host_aliases`, so that the refresh can use it, and also gives a `host` fill with the source `owner` for each repository with that alias. A host that comes only from `host_aliases` has the source `git`. Agent-derived, needs owner decision.
7. Pick (section 8.5). Screen 1 shows the groups and the single repositories, each with a count, for example "12 repos, 4 active in 30 days". For each selected group, the owner selects "active in 30 days", "all", or "pick". Only "pick" opens screens of repositories, most active first, with at most 4 questions of 4 options on a screen. Owner decision 2026-10-02. More than 16 items go on more screens: agent-derived, accepted 2026-10-03. Verify: `AskUserQuestion` takes 1 to 4 questions with 2 to 4 options each (the docs page agent-sdk/user-input says so). A repository is active when its `activity.at` of `learn_scan` is within 30 days of the time that `date -u` gives, and the skill counts the active repositories. Agent-derived, needs owner decision.
8. Projects. For each group, the proposed projects, with the selects "accept", "split", "join", and "skip". "Join" adds the repositories of the project to another project. Then the main repository of each project with more than one repository, and the key of each project for which the rule of section 8.5 gives an empty key or a key of more than 40 characters. When a join merges two projects that have a stored purpose, the owner selects the purpose that stays. Agent-derived, needs owner decision: this step before the learn step, and the purpose at a join.
9. Learn. An agent reads every selected project at init. Owner decision 2026-10-03. The skill runs one subagent `bruh:learner` for each project of step 8, with the input of section 8.5, "Learner", and at most 8 learners at the same time. It collects the closed lines of each learner, and refuses each line that breaks the parse rules. It writes nothing. Agent-derived, needs owner decision: the details of this step, and its place between the projects and the confirm.
10. Confirm. The owner confirms each value of step 9. Owner decision 2026-10-03. The details are agent-derived, needs owner decision. Each project shows its purpose, its links with their evidence, its doc pointers, and the host of each alias, as text, and the select "accept all", "one by one", or "skip all". "One by one" shows each value with the select "accept", "edit", or "skip", and "edit" takes the value as text. An accepted proposal has the source `agent`. An edited or typed value has the source `owner`. A skipped value is not in the index. At "learn again", the confirm shows the stored value next to the proposal, and a value with the source `owner` stays unless the owner selects the new one. The confirm asks the kind of each host that a fill adds, when section 8.5 cannot find the kind. The accepted values go into the init answers as the `fills` of the project. A `HOST <alias>` value gives one `host` fill for each repository of the project whose remote uses that alias. Agent-derived, needs owner decision.
11. Diff. Nothing is written before the yes of the owner. The diff contains each write of init: the settings, the start settings of bigm in the ledger (section 3.4), the ledger layout, the files of the learn step, and the project file of each selected project. The skill shows the full diff of the files outside the ledger, and a table for the ledger part: the project key, the purpose, the repositories, the links, and the docs. When the owner selects "show all", it shows the full diff of the ledger part. One `diff_sha256` covers all files. Owner decision 2026-10-02 (the table and "show all"). The skill shows the full diff of `<ledger>/.claude/settings.json` too, because it is a settings file (amends L32). Agent-derived, accepted 2026-10-03. Premise changed: the table of L32 had the stack and the gates with their source, and the index has neither (section 8.5). Dependent decision re-decided: the columns are the project key, the purpose, the repositories, the links, and the docs. Agent-derived, needs owner decision. The result of `init_plan` also has `outside_diff` (the files outside the ledger, and `<ledger>/.claude/settings.json`), `ledger_diff` (each other ledger file), `ledger_table`, `trust` (one object for the ledger and for each repository, with the flags `claude_settings` and `mcp`), and `kept`, and its `diff` is `outside_diff` followed by `ledger_diff`. Agent-derived, needs owner decision.
12. Trust. The skill lists each selected repository, and shows for each one whether it has `.claude/settings.json` or `.mcp.json`, because the trust dialog turns on the hooks and the MCP servers of the repository. The owner selects "now" or "at the first work". "Now": the skill guides the owner through one interactive `claude` for each repository, with a counter, and a repository that is already trusted opens with no dialog. "At the first work": bigm sends a P0 with the folder when a clanker cannot start there. Owner decision 2026-10-02 (the trust step, "now" and "at the first work"). The same P0 for a clerk: agent-derived, accepted 2026-10-03. The skill does not read or write the trust flags in `~/.claude.json`, because they are not documented.

A second run of the skill asks first: "change projects" or "change settings". "Change projects" shows the selected projects and asks "add" or "remove" with selects. A removed project loses its JSON, and its project file goes when it has no open item (section 8.6). "Change settings" asks the settings outside the ledger again, and each select shows the current value first with the label "(current)". The mode and the P1 settings in `mode.md` change only through bigm (section 3.4 and the rule in `mode.md`). Each item of this paragraph is agent-derived, accepted 2026-10-03.

"Change settings" sends no `projects` key, so the index does not change, and the answer `projects: []` removes each project. Agent-derived, needs owner decision. Init tests "no open item" by structure only: it deletes the project file of a removed project when no table of the file has a data row, and else keeps the file and lists it in `kept`. Agent-derived, needs owner decision. Slack is the current channel when the user settings allow `mcp__plugin_bruh_slack__post_question`, and Telegram when `enabledPlugins["telegram@claude-plugins-official"]` is `true`. Agent-derived, needs owner decision.

Both branches write or merge `<ledger>/.claude/settings.json` in the same way as the first run (section 3.4), so an existing install gets the file from either branch. Agent-derived, accepted 2026-10-03.

When a ledger file exists, init updates it with a rule for each file. `README.md` and `projects/_template.md` become the template, a `priorities.md` with no section "Never without the owner" becomes the default, `rules.md` gets the template text above its line `## Rules`, and each other file gets the template text with the values of its `key: value` lines and the data rows of its tables with the same header line (a table whose header line is not in the template goes to the end). Agent-derived, needs owner decision. Init does not change an existing `projects/<key>.md`, so a project file of version 0.5 keeps its section "Questions and answers". Agent-derived, needs owner decision.

Since version 0.6, "change projects" also has "learn again" for a selected project. An added project and a project to learn again go through steps 8 to 10. This is how the owner updates the index. `init_plan` keeps each stored project JSON that the answers do not change. Agent-derived, needs owner decision.

Verify: a select of `AskUserQuestion` has no option that is selected at the start.

These are no longer init questions: merge grants, delegated P1 classes, and remote machines. The owner tells bigm, and bigm records each one: a merge grant in `grants.md` (section 8.3), a delegated class in `priorities.md` (section 14.1), and a remote machine on the new line `remote_environments:` of `mode.md`. Agent-derived, accepted 2026-10-03. Both forms of the answers no longer have keys for these items, so an old answers file with one of them fails with the error "unknown field". In a container, the owner tells them to bigm through a channel. Agent-derived, needs owner decision.

The skill adds the allow rules `Workflow(bruh:deliver)` and the allow rules of the MCP tools to user settings. Agent-derived, accepted 2026-09-30. Since version 0.6, init writes the start settings of bigm into `<ledger>/.claude/settings.json` (section 3.4), and no longer writes `<plugin data folder>/roles/bigm.json`. When the file exists, init keeps each other key, sets `agent` and each env value of the defaults, and adds each default deny rule that the file does not have, after the existing rules. A file that is not a JSON object stops init with an error, and init does not replace it. So does a file whose `env` or `permissions` is not a JSON object, or whose `permissions.deny` is not a JSON array. An empty file counts as `{}`. An existing install runs `/bruh:init` again to write the file. Agent-derived, accepted 2026-10-03. After the apply, it shows only the notes that match the answers: for example, no Telegram note when the answer has no channel. Agent-derived, accepted 2026-10-03.

Non-interactive form: for a container, the init skill reads the same answers from a file or from environment variables, prints the diff, and writes the files. The learn answers have the keys `root`, `depth`, `exclude`, `host_kinds` (a host and its kind), `host_aliases` (an SSH host alias and its host), and `projects`: a list in which each project has `key`, `repos` (the paths), `main` (the path of the main repository), and `fills` (section 8.5). Version 0.5 had `links` in place of `fills`. The interactive init sends the same keys to `init_plan`. A container has no install dialog, so the keys `user_name`, `handoff_percent`, and `max_busy_clerks` set the plugin options there. Agent-derived, accepted 2026-10-03. The `fills` are agent-derived, needs owner decision. The non-interactive form runs no learner: it takes the `fills` of the answers as they are, and `init_plan` checks them with the value rules of section 8.5. The non-interactive form and `init_plan` compute the git facts with the scan code, not through the tool `learn_scan` (section 8.5). Agent-derived, needs owner decision. The MCP tool `init_plan` refuses the keys `user_name`, `handoff_percent`, and `max_busy_clerks` with an error, and the non-interactive form writes only the plugin options that the answers have. No answer key is necessary except `ledger_path`. Agent-derived, needs owner decision.

## 17. Plugin layout

Owner decision 2026-09-27: `oter/bruh` is a plugin marketplace with one plugin, `bruh`, installed at user scope. The layout is agent-derived, accepted 2026-09-30.

```text
.claude-plugin/marketplace.json
plugins/bruh/.claude-plugin/plugin.json
plugins/bruh/.mcp.json
plugins/bruh/agents/bigm.md
plugins/bruh/agents/clanker.md
plugins/bruh/agents/clerk.md
plugins/bruh/agents/learner.md
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
plugins/bruh/defaults/bug-reports.md
plugins/bruh/defaults/role-settings.json
```

Version 0.6 adds `agents/learner.md`, the read-only subagent of the learn step (section 8.5). Agent-derived, needs owner decision.

## 18. README install instructions

Owner decision 2026-09-27: the README gives clear install instructions for each part. The list is agent-derived, accepted 2026-09-30.

1. The plugin: `claude plugin marketplace add oter/bruh`, then `claude plugin install bruh@oter`. The install dialog asks the plugin options, among them the name, the handoff threshold, and the busy clerk cap. Owner decision 2026-10-03: the marketplace name is `oter`, so the plugin ID is `bruh@oter` (design.md, L47).
2. The ledger: create a private repository before the init skill runs.
3. The init skill: `/bruh:init`, or the non-interactive form in a container. It asks for the ledger, the settings, and the projects, and it ends with the trust step.

   Version 0.6 changes items 1 to 3: the install dialog and the trust step are owner decisions of 2026-10-02 (section 16); the ledger before the init skill is agent-derived, accepted 2026-10-03. The README says that the owner runs `/bruh:init`, also a second run, from a session with no role key: `claude --setting-sources user` in the ledger folder, or a session in another folder (section 16). Agent-derived, needs owner decision.
4. Channels: Telegram or Slack.
5. A remote machine: Claude Code, the plugin, the init skill, `orca serve` on a private network address, and `orca environment add` on the bigm machine.

   On a remote machine, the owner selects no project at the pick, and the init skill sends no `projects` key, so init writes no `learn/` file there. README step 5 says so. Agent-derived, needs owner decision.
6. A container: the image, the volume for the plugin data folder, and the non-interactive init.
7. Start: the bigm command of section 3.4.
8. Requirements: the minimum Claude Code version, Go 1.26 or later (for the MCP server), `jq` (for the hook scripts), and Orca for remote work. A warning that plugins that inject text at session start cost context in every role.

## 19. Day-0 maturity

Owner decision 2026-09-27: the repository is mature on day 0, proven by what it contains. A badge shows only a fact that CI or GitHub proves. The list is agent-derived, accepted 2026-09-30.

- CI on each push and pull request: `gofmt`, `go vet`, `go test -race`, Markdown lint, link check, `claude plugin validate`, shellcheck, and the tests of section 20. Verify: `claude plugin validate` runs in CI without a login.
- CI runs the tests on macOS and on Linux (section 10.2).
- Branch protection on `main`, with the CI checks required.
- Semantic version tags with GitHub releases, and `CHANGELOG.md` in the Keep a Changelog format. The first release is `v0.9.0`, and `plugin.json`, the bruh MCP server, and the Slack channel server report this version (owner decision 2026-10-03).
- `LICENSE` with Apache-2.0 (the owner delegated the choice on 2026-09-29), `SECURITY.md`, `CONTRIBUTING.md`, issue templates, and a pull request template.
- Dependabot for GitHub Actions versions. OpenSSF Scorecard workflow and badge.
- CodeQL default setup on each push and pull request, Go fuzz tests of the parsers and the policy checks, and the CI tools pinned by hash. Agent-derived, accepted 2026-10-03 (the owner asked to handle the Scorecard alerts on 2026-10-01).
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
- Learn tests, in Go with fixture folders (section 8.5). Agent-derived, accepted 2026-10-03.
  - The scan finds each repository, and skips a `.git` file, `.claude/` folders, a repository inside a repository, the ledger, and an excluded folder.
  - It removes a user name or a token from each remote URL with a scheme other than `ssh`, removes the password of an `ssh://` URL, and keeps the SSH user.
  - It proposes projects from a shared name prefix.
  - Each project key matches section 3.2, gets the group prefix on a collision, and has at most 40 characters.
  - It never runs a command of a repository.
  - It calls no code host that has no login (fake `gh`, `glab`, and `tea`).
  - `learn_refresh` writes no file when nothing changed, and a default branch that has no file keeps its stored value.
- Premise changed: the learn tests checked the links from manifests, the gates from task and target names, and the gate files of the refresh. The Go scan now reads git facts only (section 8.5). Dependent decisions re-decided: these checks go, and the index tests below replace them. Agent-derived, needs owner decision.
- Index tests, in Go with fixture folders (section 8.5). Agent-derived, needs owner decision.
  - The scan reads no file of the working tree: a fixture repository whose working tree files have no read permission gives the same result, and the output has no stack, gate, or link.
  - `init_plan` checks the fills with the value rules of section 8.5, writes the sources `agent` and `owner`, runs no CLI, and writes the same bytes for the same answers.
  - `learn_refresh` sets and clears `gone` on a doc pointer and names each change in `gone_docs`, skips the doc check of a missing repository, keeps a host with the source `agent` or `owner` while the remote URL does not change, lists a missing repository only once, runs no `ssh -G`, and refuses with a clear error when `init/config.json` has no `ledger_path`.
- Learner eval. The rule of the owner says that meaning needs a model with its own eval set. So the repository has fixture repositories, each with the expected `LINK` and `DOC` lines, and a run of `bruh:learner` on each fixture before each release. The eval passes when each expected `LINK` and `DOC` line is in the output. An extra `DOC` line is allowed. A `LINK` line that is not expected fails the eval. The result goes into the release notes. In the interactive init, the owner checks the purpose line at the confirm. Agent-derived, needs owner decision.
- The eval set is in `tests/eval/learner/<case>/`, and each case has a folder `root/`, a file `input.txt`, and a file `expected.txt`. The driver `tests/eval/run.sh` runs `claude -p --agent bruh:learner` on each case, does not run in CI, and prints the summary line `learner eval: <passed> of <cases>` for the release notes. Agent-derived, needs owner decision.
- Watcher test: each event goes to `reports/clanker-<project>.jsonl` of the project of its repository, and no file `reports/watcher.jsonl` is made. The test is agent-derived, needs owner decision.
- Question ID tests: `question_open` gives `Q-<project>-<host>-<n>` for each kind of role key, with the host rules of section 5, also for a host name that is empty after the rules. The header checks of `mail_post`, the `qidRE` of `answer.go`, the merge train, `post-findings.sh`, and the Slack channel accept the new form and refuse an ID of version 0.5. Agent-derived, needs owner decision.
- GitLab host tests with a fake `glab`, in the same way that `ghBin` fakes `gh`. Agent-derived, accepted 2026-10-03.
- `question_open` keeps the options, and refuses fewer than 2 or more than 4. Agent-derived, accepted 2026-10-03.
- The non-interactive init with the keys `root` and `projects` writes the same files as the interactive init. Agent-derived, accepted 2026-10-03.
- A load test of section 4.1: 8 background sessions for one hour.
- A smoke test in a temporary repository before each release. bigm starts one clanker, and the clanker starts one clerk. The clerk runs `/bruh:deliver` on a small task. A workflow agent sends a P1, the question reaches bigm, and the answer reaches the agent in the same run. A clerk that waits on a permission prompt raises a P0 within one sweep. A clanker that was idle for more than an hour resumes when it gets a message. A remote clanker sends a P1 through Orca. Since version 0.6: the init skill with a scratch repository that has a `README.md` ends with its index in `learn/projects/<key>.json`: the repository with its git facts, a purpose, and a doc pointer to the README. Premise changed: this step checked a test command in the JSON, and the index has no gates. Agent-derived, needs owner decision. Since version 0.6: a plain `claude -p` in the ledger, with no tool and no MCP server, answers with the role name `bigm`. This proves the agent key of the ledger settings file. A missing role key is not silent: each bruh tool call fails with "BRUH_ROLE_KEY is not set", so the smoke test does not probe it. Agent-derived, accepted 2026-10-03. The result goes into the release notes.
- A script cannot drive the interactive init skill, and the non-interactive form runs no learner. So the smoke step of the index runs the learner with `claude -p --agent bruh:learner`, turns its `PURPOSE` and `DOC` lines into fills, and runs the non-interactive `init --answers` with a scratch ledger, settings file, and data folder, and the Go test `TestCLIInitMatchesInteractive` shows that both forms write the same files. Agent-derived, needs owner decision.
- The scratch repository of that smoke step is a plain repository with its own `.git` folder, not a linked worktree, because init refuses a `.git` file, the scan skips a linked worktree, and a remote that a linked worktree adds goes into the trusted repository. Agent-derived, needs owner decision.
- Each "Verify" item gets a test or a smoke test step before the implementation depends on it.
- Acceptance of version 0.6: release a new version, install it again, and the owner runs the onboarding and judges it. Owner decision 2026-10-02. The release and the push each need an explicit go of the owner.

## 21. Out of scope for version 0.1

Agent-derived, accepted 2026-09-30.

- Native Windows.
- Per-agent identities and several Claude accounts. Owner decision 2026-09-29: later.

## 22. Open questions for the owner

All numbered questions of version 0.3 are decided. Their answers are in design.md, "Answers to the spec open questions", and in the sections above. The questions of the draft of 2026-10-02 about the watcher file and the question IDs are decided too (design.md, L38 and L39).

1. The ledger clerk (section 3.6): the reading of "clerks do all pushes" for the ledger.
2. Each item in this file tagged "Agent-derived, accepted 2026-09-30".
3. Each item in this file tagged "Agent-derived, needs owner decision". All of them are new on 2026-10-03 (design.md, L37, L38, L39, L42, L43, L44, L47, L48, and L49). The items of L49 are in sections 2 and 6.1. The build of 2026-10-03 adds more items with the same tag: the user information of a remote URL, the gaps of this specification that the build found, and five details of the build. They are in sections 3, 3.4, 3.5, 5, 6.4, 7, 8.3, 8.5, 8.6, 9.1, 9.2, 10.1, 12, 14.2, 16, 17, 18, and 20, and in the change list of section 25. The release version of section 19 is settled: owner decision 2026-10-03, the first release is `v0.9.0` (design.md, L46). Owner decision 2026-10-03: the other items are built as written, and the owner reviews them during the onboarding run (design.md, L45).
4. Inbound messages (design.md, open question 6). Without a `crossSessionInbound` value, a session that bypasses permission prompts holds a message from a session that does not. Options, as design.md lists them: run all roles in one permission mode, or the init skill sets `crossSessionInbound: accept` in user settings. `accept` delivers every message from any session of the same operating-system user.
5. Monitors ([section 9.5](#95-monitors)), design for owner review, not built. The owner request of 2026-10-04 is in design.md, L50. Each choice has ranked options and a recommendation in section 9.5, part k:
   - M1: the wake of a subscriber session: an `asyncRewake` hook, a plugin monitor, the `Monitor` tool, or a `CronCreate` task.
   - M2: the host of the poller: a plugin monitor of bigm, the `Monitor` tool in bigm, a loop in the bruh MCP server, or a service of the operating system.
   - M3: the topology of the polls: one poller on the machine of bigm, or a poller in each subscriber session.
   - M4: a monitor of a clerk at the end of its task: hand over to the clanker, stop with the clerk, or the clanker decides for each monitor.
   - M5: a system that has only an MCP server: a poll by the role, CLI and HTTP sources only, or the poller starts the MCP server.
   - M6: the approval of a `command` source: a command grant, the `Monitor` tool only, or no `command` kind.
   - M7: how the ledger shows the active monitors: on demand with `monitor_list`, a table in each project file, or a file `monitors.md`.
   - M8: the watcher of section 9.2: fold it in, keep both, or replace it.
   - M9: the defaults of `monitor_default_hours`, `monitor_max_hours`, and `monitor_max_active`.
   - Each other item of section 9.5 is tagged "Agent-derived, needs owner decision".

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

The owner ran `/bruh:init` for the first time on 2026-10-02. The changes come from the feedback of the owner, the decisions L1 to L49 of design.md ("Learn step and onboarding"), and reviews by subagents: four on the decisions (an adversarial reviewer, a checker of the claims against the code, a walk through the journey of the owner, and a search of the Claude Code docs), and two rounds of an adversarial reviewer and a checker on this specification. Each dependent that a "Premise changed" line names carries its own tag in its section.

- Owner decision 2026-10-02: bruh learns the projects before the owner asks for work (sections 1 and 8.5). The init skill scans a root folder, the owner selects the projects with selects, and init stores the hierarchy and the index of each project as JSON in the ledger. Since the owner decisions of 2026-10-03, the index has no stack and no gates.
- Premise changed: the delegated ruling of 2026-10-02 had the Go scan find the gates from exact task and target names. The owner decisions of 2026-10-03 below replace it in full: the Go scan reads git facts only. "No learn workflow" holds, so no role other than a clerk starts a workflow (section 3).
- Owner decision 2026-10-02: bruh stores no live status. bigm reads it from the code host on demand (section 8.5).
- Owner decision 2026-10-02: bruh supports GitLab through `glab` in the same way as GitHub and Gitea: the pick, the status on demand, `repos_set`, the watcher, and the merge train (section 8.5).
- Owner decision 2026-10-02 (delegated to the agent): the Markdown files of the ledger hold only the current state, git is the history, and a check at each sweep finds a file that is too long (section 8.6). Premise changed: bigm kept each row with a closed state, and the project file kept each question and answer. Dependent decisions re-decided: a closed row is deleted in its commit, the section "Questions and answers" goes (section 8.1), a removed project loses its JSON (section 8.5), the record of an answer is its commit (section 14.2), and a retired rule or grant is deleted (section 8.6).
- Owner decisions 2026-10-02, after the review of version 0.6: no Markdown view of the tree in the ledger; projects on a remote machine are not learned in version 0.6; the init diff shows a table for the ledger part, and the full ledger diff on request (sections 8.5 and 16).
- The init skill has new steps (section 16): the pick and the trust step are owner decisions of 2026-10-02; the purpose line for each question and the selects are agent-derived, accepted 2026-10-03. It does not ask the three plugin options of the install dialog, and it ends with a trust step (sections 4.1 and 16).
- Owner decision 2026-10-02: P1 questions can have options, and bigm shows them as selects in a reply to a message of the owner (section 14.2). A channel shows them as numbered text (section 12).
- Premise changed: bigm learned about a project only from the first work request. Dependent decisions re-decided: the project file is made by the init skill, not at the first work (section 8.1); a clanker starts from the index (section 3.5); bigm takes the input of `repos_set` from the index (section 8.5); the sweep calls `learn_refresh` (section 9.1).
- Premise changed: bigm was the only writer of the ledger. Dependent decisions re-decided: plugin code writes the files of the learn step, and bigm stays the only role that commits (principle 3, section 3.4).
- Premise changed: init wrote `user_name`, `handoff_percent`, and `max_busy_clerks` into `pluginConfigs`. Dependent decisions re-decided: the install dialog asks them, `init_plan` does not write them, and the non-interactive form sets them in a container (section 16).
- Premise changed: the trust list of init had only the ledger. Dependent decisions re-decided: the trust step covers the selected repositories, and the P0 at the first work covers a clanker and a clerk (sections 4.1 and 16).
- Premise changed: init asked 13 questions, one for each message. Dependent decisions re-decided: merge grants, delegated P1 classes, and remote machines go to bigm, which records them in `grants.md`, `priorities.md`, and `mode.md` (sections 14.1 and 16); the README asks for the ledger before the init skill (section 18).
- Premise changed: bigm showed the P1 items one for each message. Dependent decision re-decided: at most 4 questions with options on one select screen, and the text form at other times (section 14.2).
- Owner decision 2026-10-02: bigm starts in two ways in the ledger folder: `claude --agent bruh:bigm` or plain `claude` (section 3.4).
- Owner decision 2026-10-02: the start settings of bigm are `<ledger>/.claude/settings.json`, committed to the ledger repository (section 3.4). Agent-derived, accepted 2026-10-03: init writes that file and stops writing `<plugin data folder>/roles/bigm.json`; the merge rule for an existing file (section 16); the smoke check of the plain start (section 20).
- Premise changed: bigm started only with `--agent bruh:bigm` and `--settings <plugin data folder>/roles/bigm.json`. Dependent decisions re-decided, each agent-derived, accepted 2026-10-03:
  - Init writes the ledger settings file in place of `roles/bigm.json` (section 16). Both branches of a second init run write or merge it, so an existing install runs `/bruh:init` again (section 16).
  - Every role started with `--permission-mode auto` (owner decision 2026-09-30). A plain start of bigm runs in the default permission mode (sections 3.1 and 3.4).
  - bigm was the only writer of the ledger, except the files of the learn step. `.claude/settings.json` is a second exception, because init writes it (section 3.4).
  - Each session in the ledger folder with no `--agent` and no `--setting-sources user` starts as bigm. The ledger clerk and a merger clerk stay clerks, because their `--agent` and `--settings` flags rank above the file. The idle nudge of the smoke test loads only the user settings and has no Bash (`tests/smoke/run.sh`). The trust session of the init skill starts as bigm, and the owner always quits it with `/exit` after the trust dialog: the project `env` applies only after trust, so that session can have no role key. A manual session of the owner is a second bigm (a second `mail_read`, sweep, watcher, and handoff), so the owner runs one bigm at a time and opens each other session with `claude --setting-sources user`. On a remote machine, the stand-in ledger folder of README step 5 gets the same file (section 3.4).
  - Every clone of the ledger starts as bigm. Today only the machine of bigm has the ledger, because a remote clanker does not clone it (section 3.4).
  - A plain start has no session name and no channel. The owner runs `/rename bigm` after a plain start, and starts bigm only with the full command when Slack or Telegram is set up (section 3.4).
  - The start commands of section 3.4, the README, and the Slack README, and the `launch_command` of `init_plan`, have no `--settings`. The release smoke test checks the plain start (section 20).
- Corrected in the decision log: the watcher is its own process that polls every 60 seconds, not a step of the sweep; bruh already uses `glab` for posts on GitLab; the plugin has 6 options, not 3; many repositories of the owner are on `gitlab.com`: a count of the first remotes of the local clones on 2026-10-02 found 55, of which 37 use an SSH host alias.
- Owner decision 2026-10-03: the watcher writes each event to the report file of its project, not to one shared file (section 9.2). This decides item 4 of section 22 of the draft of 2026-10-02. Agent-derived, needs owner decision: the file name `reports/clanker-<project>.jsonl`, the required `project` of `repos_set`, the appends with no lock, one rule for the events (a relay from the `Monitor` line, and a read with `since` only for missed lines), and the end of the role key `watcher` of `report_read`.
- Owner decision 2026-10-03: a question ID is `Q-<project>-<host>-<n>`, with the host name of the machine (section 5). This decides item 5 of section 22 of the draft of 2026-10-02. Agent-derived, needs owner decision: the rules for `<project>` and `<host>`, and the pattern of the checks. Premise changed: the counter of each machine gave `Q-<n>`. Dependent decisions re-decided, each agent-derived, needs owner decision:
  - The ID is unique across the machines, except after a renamed machine or a new install on the same host, and except the limits of section 5: two IDs that split on another `-`, and two hosts with the same part before the first `.`.
  - The post approval of section 6.4 and the commit subject of section 8.6 carry the new ID. The commit subject keeps the role key, because the ID does not name the task of a clerk.
  - bigm still finds an answered question by its answer file, not by the history in git (section 8.6).
  - bigm still keys a question by the pair of the asker and the ID, and the record `<orca environment>/Q-<n>` of plan 3 (ruling M4) goes (section 5).
  - At the upgrade, bigm asks each asker of an open question of version 0.5 to open it again with a new ID (section 5).
- Owner decision 2026-10-03: a question whose ID bigm already answered, with another subject, is a new question, and bigm asks the asker to open it again with a new ID (section 5). The owner selected "Compare subject, re-ask". Premise changed: the receiver ignored each ID that it already answered. Dependent decisions re-decided: bigm sends the stored answer again for a repeat with the same subject, sends `ANSWER <id>: reask` for another subject, and `answer_write` stores the subject. Agent-derived, needs owner decision.
- Owner decision 2026-10-03: the Go scan reads only git facts: the folder tree and the `.git` folders, the host, the host kind, the API root, and the activity. It reads no file of the working tree (section 8.5).
- Owner decision 2026-10-03: the ledger is the index and the glue among the projects. The index of a project has the key, a purpose of one line, the main repository, the repositories, the links with their evidence, and the doc pointers. It has no stack, no gates, and no command. At init, the learner reads every selected project, and the owner confirms each value. A clanker learns the gates from the repository at the start of work, and gives them to its clerks (sections 3.5, 8.5, and 16).
- Owner decision 2026-10-03, first form: the Go scan reads the facts, and an agent fills the gaps at init (design.md, L40). Premise changed by the two decisions above: its gaps, its sources `declared` and `convention`, its gate hash, and its re-learn (with `relearn`, `DONE: relearn`, a P1, and `learn_set`) are not in the spec.
- Premise changed: the owner decision of 2026-10-02 said that bruh stores the stack and the gates, and the delegated ruling of 2026-10-02 had the Go scan find them in files of the working tree. Dependent decisions re-decided, each agent-derived, needs owner decision:
  - L21 is replaced in full: the Go scan has no gate, stack, CI, or link rule (section 8.5).
  - The links come from the learner with their evidence, not from manifests (section 8.5).
  - The clanker reads the gates from the repository at the start of work, not only the gates marked `unknown` or `convention` (section 3.5).
  - The refresh reads the git facts again, marks a missing repository, and marks a doc pointer whose file is gone. It has no gate hash and no re-learn. The owner updates the index with "change projects" of a second init run (sections 8.5, 9.1, and 16).
  - The diff table of init shows the project key, the purpose, the repositories, the links, and the docs (L32, section 16).
  - The smoke test checks the index of a scratch repository (section 20).
  - The learn tests of the gates go, and index tests and an eval set of the learner come (section 20).
  - No learn workflow: this holds. The learners run through the Agent tool, so only a clerk starts a workflow (section 3).
  - The refresh has no model call: this holds (section 9.1).
  - The scan runs no command of a repository: this holds, and the learner runs none either, because it has only Read, Grep, and Glob (section 8.5).
  - `init_plan` and `init_apply` write the same bytes for the same answers: this holds, because the fills are part of the answers, and `init_plan` runs no CLI (section 8.5).
- Agent-derived, needs owner decision: the closed schema of the learn files, of `learn_scan`, of `learn_refresh`, and of the fills, with the sources `git`, `agent`, and `owner` (section 8.5); the contract of `bruh:learner` (section 8.5); the eval set of the learner (section 20); the routing of a work request with the purpose lines and the links (section 8.5); the learner as a subagent that is not a role (section 3); the ledger path of the learn tools, the alias map from `learn_scan`, the `gone` flag, the split confirm with at most 8 learners at the same time, and the `START:` header (sections 5, 8.5, and 16).
- Agent-derived, needs owner decision: the init skill checks the role key with `bruh_info` at the start of each run, and stops when the session has one. A session in the ledger folder is bigm, and `learn_scan` refuses a caller with a role key (sections 3.4, 8.5, 16, and 18).
- Owner decision 2026-10-03: the owner accepted, as a set, every earlier item of version 0.6 tagged "Agent-derived, needs owner decision" ("Accept all, build now"). Each such item now has the tag "Agent-derived, accepted 2026-10-03".
- Owner decision 2026-10-03: the new items tagged "Agent-derived, needs owner decision" are built as written. They stay tagged and listed in section 22, item 3, and the owner reviews them during the onboarding run. The owner selected "Build as written".
- Owner decision 2026-10-03: the first release is `v0.9.0` (section 19).
- Owner decision 2026-10-03: the marketplace name is `oter`, and the plugin ID is `bruh@oter` (section 18).
- Owner decision 2026-10-03: init removes a bruh status line tap of another bruh data folder before it wraps the status line command again (section 7).
- Owner decision 2026-10-03: when a session finds a defect of bruh, the owner gets the offer of a bug report (section 2, principle 8).
