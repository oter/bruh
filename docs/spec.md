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
2. **Mechanical stops, not notes.** A rule that must never break is a deny rule, a hook, or a sandbox rule, not a sentence in a prompt or a memory note. A Bash deny rule matches only the command text, so another form of the same program gets past it. A deny rule is a speed bump. Where a real stop is necessary, bruh uses the sandbox. When the same lesson is recorded a second time, bigm raises a P1 that proposes a mechanical stop for it. The refusal stop of section 15.1 is such a hook. Owner decision 2026-10-04.
3. **Plugin code writes the files.** All bruh files are in the plugin data folder, and plugin code writes them (section 10). Owner decision 2026-09-29. The files of the learn step are in the ledger, and plugin code writes them too (section 8.5). Owner decision 2026-10-02.
4. **Timestamps come from code.** A time in the ledger, a handoff, or a message comes from `date -u` or from the MCP server, never from the model.
5. **Rules are kept word for word.** An owner rule is stored with its words, its date, and its source. A rule with no source is a recommendation, not a rule.
6. **A refusal is escalated, never handed on.** A permission prompt, a classifier refusal, a worktree guard refusal, or a deny rule goes to the owner. No other session runs the refused command. A hold stops the session after a classifier refusal or a deny of the git-shape guard until the owner answers (section 15.1). Owner decision 2026-10-04.
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

Owner rule R-1 (2026-10-03) makes the scout clerk, a short-lived, read-only clerk (section 3.6.1). The owner said "you may spawn clerk scout/clerk scouts", so a scout is a kind of clerk, not a fifth role. Owner decision 2026-10-03. The table keeps its four roles and its spawn chain. Premise changed: the first build had bigm start scouts, as one more exception to "a clanker starts a clerk". The owner said on 2026-10-04 in the terminal, word for word: "okay maybe scout must be spawned by his lead? e.g. clanker? ". Dependent decision re-decided: the clanker of the project starts each scout, so a scout follows "a clanker starts a clerk" and is not an exception to the spawn chain (design.md, L52). Agent-derived, needs owner decision.

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
- The start settings of bigm are `<ledger>/.claude/settings.json`, committed to the ledger repository. Owner decision 2026-10-02. The file has the key `agent` with the value `bruh:bigm`, the `env` values (`BRUH_ROLE_KEY=bigm` and the env of `defaults/role-settings.json`), and the deny rules of `defaults/role-settings.json`. It replaces `<plugin data folder>/roles/bigm.json`, so the full command has no `--settings`. Init writes the file (section 16). Agent-derived, accepted 2026-10-03. The file also has the six deny rules of bigm: `Agent(claude-code-guide)`, `Agent(general-purpose)`, `Agent(Explore)`, `Agent(Plan)`, `WebFetch`, and `WebSearch`. They block the research subagents and the web tools, so bigm gets each fact through a clanker or its scout (owner rule R-1). Owner decision 2026-10-05, Q-bigm-maksyms-macbook-pro-99.
  - Each session that starts in the ledger folder or in another clone of the ledger with no `--agent` and no `--setting-sources user` is bigm. Today only the machine of bigm has the ledger, because a remote clanker does not clone it. Agent-derived, accepted 2026-10-03.
  - On the machine of bigm, a second `claude` in the ledger folder is a second bigm. It reads the mailbox of bigm with `mail_read`, starts a second sweep (the poller lock of section 9.5 stops a second poller), and writes the handoff of bigm. So the owner runs one bigm at a time, and opens each other session in the ledger folder with `claude --setting-sources user`. Agent-derived, accepted 2026-10-03. The owner also runs `/bruh:init` from such a session, because `learn_scan` refuses a caller with a role key (section 16). Agent-derived, needs owner decision.
  - On a remote machine, init writes the same file into the stand-in ledger folder (README step 5). A plain `claude` in that folder is a bigm on the remote machine, so the owner does not start `claude` there. Agent-derived, accepted 2026-10-03.
  - The ledger clerk and a merger clerk start in the ledger folder with `--agent bruh:clerk` and their own `--settings`. Both flags rank above the project settings, so each stays a clerk with its own role key. Agent-derived, accepted 2026-10-03.
  - An agent name that does not exist gives a normal session with no warning (probe of 2026-10-02 with Claude Code 2.1.284). So the release smoke test checks the plain start (section 20). A missing role key is not silent: each bruh tool call fails with "BRUH_ROLE_KEY is not set", so the smoke test does not probe it. Agent-derived, accepted 2026-10-03.
- bigm starts a clanker for each project that has work. It starts as many clankers as necessary and does not rotate them. Owner decision 2026-09-27. The concurrency caps of section 3.7 apply. Agent-derived, accepted 2026-09-30.
- bigm routes each work request and each bug report of the owner to a lane itself: the running clanker of the project, or a new clanker. It tells the owner its commitment, and the owner observes. bigm never asks the owner which role does the work, how to divide it, or whether to start it. The P1 classes of `priorities.md` and the items of section 13 still go to the owner. Owner decision 2026-10-04 (design.md, L50).
  - The commitment is one line in the closed form `I send <work> to clanker-<project> as task <n>.`, with no other text on the line. The Task cell of the row in the project file is `task <n>: <work>`. `<n>` is 1 more than the highest task number of the project, in its rows and in the subjects `close task: task <n>: <work>` of the history of its project file, and the first number is 1. bigm acts at once after the line and does not wait for a yes. The owner can say no: then bigm tells the clanker to drop the task and closes the row. Agent-derived, needs owner decision.
  - The fix of a defect of bruh (principle 8) is work too. When the ledger has a project for the repository of bruh, bigm routes the fix in the same way. The yes of the owner before the public bug report stays, because the issue is an outward post. Without such a project, the issue is the only deliverable. Work with no request, bug report, or defect notice behind it is scope growth (section 13), and stays with the owner. Agent-derived, needs owner decision.
- bigm is the only writer of the ledger, except the files of the learn step (section 8.5), which plugin code writes, and `.claude/settings.json`, which init writes (section 16). bigm commits after each change. The setup of the owner confirmed that one writer kept the ledger consistent. Agent-derived, accepted 2026-09-30. The exception for the learn step: owner decision 2026-10-02. bigm is also the only role that commits the files of the learn step: agent-derived, accepted 2026-10-03. The exception for `.claude/settings.json`: agent-derived, accepted 2026-10-03.
- bigm runs the sweep of section 9.1 and reconciles all sessions at each sweep and at the start of each turn. Agent-derived, accepted 2026-09-30.
- bigm answers status questions from the ledger. It reads the source again for each claim of principle 1. Agent-derived, accepted 2026-09-30.
  - Premise changed: bigm read the sources of a project itself to answer, and owner rule R-1 forbids this: bigm does not gather the info of a project by hand (section 3.6.1). Owner decision 2026-10-03. Dependent decision re-decided: bigm asks the clanker of the project with `DONE: info request <subject>`, and answers from its reply `DONE: info <subject>`, with the ledger rows as context. The clanker gets the facts from its scouts. For each claim of principle 1, bigm runs again only the call of the source of that claim, just before it tells the owner, and reads nothing else of the project. The ledger, the index, and the files of the plugin data folder stay the reads of bigm. When the project has no running clanker, bigm starts or resumes it first. bigm never starts a scout. Agent-derived, needs owner decision (design.md, L52).
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
- A clanker divides the work and starts its clerks itself. It never sends bigm a question about which clerk does a task, how to divide the work, or whether to start a task. Owner decision 2026-10-04 (design.md, L50). The dispatch record of the task in the report file is the commitment of the clanker, and a task that first needs an owner decision (a P1 outside the delegated classes, or an item of section 13) still goes to bigm as a question. Agent-derived, needs owner decision.
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

#### 3.6.1 Scout clerk

Owner rule R-1, 2026-10-03, in the terminal, after bigm read the git state, the code host, and the handoff docs of a project itself for a status question: "okay. bug report - you started mining info on your own. BUT you must not do that by hands, remember about clankers and clerks? for this specific thing you may spawn clerk scout/clerk scouts. i thin we have not covered that thing in ruling". Owner decision 2026-10-03: bigm does not gather the info of a project by hand, and scout clerks gather it. The owner said on 2026-10-04 in the terminal, word for word: "okay maybe scout must be spawned by his lead? e.g. clanker? ". So the clanker of the project starts each scout (design.md, L52). The items below are agent-derived, needs owner decision, except where tagged (design.md, L51 to L53).

- A scout is a short-lived, read-only clerk. It reads the sources of a project for one question, and reports each fact with its source read. A scout is a clerk, not a fifth role (section 3). Owner decision 2026-10-03.
- Who starts it: the clanker of the project, the lead of the scout. The owner said "okay maybe scout must be spawned by his lead? e.g. clanker? " (terminal, 2026-10-04). The clanker starts a scout for a read-only question of its own that needs no task, and for each `DONE: info request <subject>` of bigm. bigm never starts a scout. Agent-derived, needs owner decision.
- How bigm asks: bigm sends `DONE: info request <subject>` to the clanker of the project, with each question as one item of the body. The clanker replies with `DONE: info <subject>`, with each claim and its source as one item. Both use the header `DONE: <subject>` of section 5, on edges that `mail_post` already accepts: from bigm to anyone, and from a clanker to its parent bigm. To and from a remote clanker, they go through Orca (section 4.2). When the project has no running clanker, bigm starts or resumes it first, as for a work request (section 3.4). bigm has no fallback that starts a scout. Agent-derived, needs owner decision.
- Key: `clerk-<project>-scout<n>`, where `<n>` is one or more digits. The task names `scout` and `scout<n>` are reserved for scouts, as `merge` is for the merger clerk. The key gives `clanker-<project>` as the parent, and the rules of a clerk key apply: `role_settings_write` and `session_launch` accept only the parent. `session_resume` refuses every scout key: for the clanker because the key is a scout key, and for each other caller, bigm too, because the caller is not the parent.
- Each key is used once. `role_settings_write` creates the settings file of a scout key only when no file exists, and else refuses and names the next free key. So a new scout reads no old mail and no old report lines. The clanker takes `<n>` = one more than the highest `<n>` of the scout keys of the project in `session_list`.
- Folder: the main checkout of the repository of the question. For facts of the code host only, the main checkout of the main repository of the project. One scout reads one repository, and a question about more than one repository gets one scout for each repository. A scout gets no `allow`: `role_settings_write` accepts `allow` only for a clanker key, written by bigm.
- Reads and runs: the `Read` tool, one plain `grep -rn` or `find` command in Bash for a search (no pipe, no git word, no compound), and commands that change nothing: for example `git log`, `git show`, `git status`, `git diff`, `git rev-parse`, `git ls-remote`, the read commands of the code host tool, and `ls` or `cat`. A scout runs no build, test, gate, or script of the project, and no `git fetch` or `git remote update`, because these write the remote-tracking refs of the checkout. It uses `git ls-remote` for the state of a remote branch. bigm decision 2026-10-05 under R-4 (Q-bruh-maksyms-macbook-pro-103).
- Mechanical stops (principle 2). Owner decision 2026-10-04: the stop for Bash writes is option 1, "Auto + deny rules": `auto` mode, the scout deny rules, and the checks of `report_write` and `session_resume` (design.md, L53).
  - Deny rules that the MCP server adds to each scout settings file (`scoutDeny` in `mcp/roles.go`). The clanker cannot leave them out. The tool rules remove `Edit`, `Write`, `NotebookEdit`, `Workflow`, `EnterWorktree`, and the bruh tools that start or resume sessions or write role settings, leases, answers, questions, results, the repository list, and the init and learn files. These rules are stops for those tools.
  - Deny rules for the usual Bash forms of writes, pushes, merges, and posts, for example `Bash(git push:*)`, `Bash(git fetch:*)`, `Bash(rm:*)`, `Bash(gh pr merge:*)`, and `Bash(gh api * -X *)`. The git rules cover each subcommand of `scoutGitWrites` in `mcp/roles.go`: `push`, `commit`, `add`, `rm`, `mv`, `merge`, `rebase`, `reset`, `checkout`, `switch`, `restore`, `stash`, `tag`, `worktree`, `clean`, `pull`, `fetch`, `apply`, `cherry-pick`, `revert`, `branch`, `remote`, `config`, `update-ref`, `symbolic-ref`, `update-index`, `read-tree`, `submodule`, `sparse-checkout`, `bisect`, `notes`, `replace`, `am`, `init`, `clone`, `gc`, `prune`, `repack`, `pack-refs`, `maintenance`, `filter-branch`, `format-patch`, `reflog expire`, and `reflog delete`. Each subcommand has the rules `Bash(git <subcommand>:*)`, `Bash(git -C * <subcommand>)`, and `Bash(git -C * <subcommand> *)`, because a scout writes its git calls as `git -C <repository path> ...`. `branch`, `remote`, `config`, and `symbolic-ref` also have read forms, and the rules stop these reads too. For these facts, a scout uses `git rev-parse --abbrev-ref HEAD`, `git for-each-ref`, `git ls-remote --get-url <remote>`, and `Read` of `.git/config`. A git write that is not on the list has only the rule of "Reads and runs" as its stop. These rules also stop a read whose text has the subcommand as a word after `-C <path>`, for example `git -C <path> log --grep push x`. A Bash deny rule matches only the command text, so these rules are speed bumps.
  - A check in code: `report_write` refuses a line of a scout key without `source` with `call`, `value`, and `at`, and with `at` in RFC 3339. The `value` must be present and can be empty, because a read that finds nothing often prints nothing. The one line without a source is the event `<role key> not running; mail pending` of the mail procedure of a clerk, with a valid role key, which bigm reads at its sweep to resume the receiver.
  - A check in code: `session_resume` refuses a scout key.
  - Not built: a stop for every Bash write, with another permission mode or the sandbox for scouts. It changes the owner decision 2026-09-30 of section 3.1, and the owner chose option 1 on 2026-10-04.
- Report: the scout writes each claim as one `report_write` line with kind `result` and the source read of principle 1: `call` (the command or the API call, with absolute paths, so that it runs again from any folder), `value` (the value that it returned), and `at` (a UTC time from `date -u`). For a fact that it cannot read, it writes `not found: <question>`, with one line for each read that it tried. Then it sends `DONE: scout <subject>` to its clanker, on the edge to the parent. The body has the claims as items, not a pointer to the report file. The clanker reads the claims with `report_read` and the scout key, and checks each status claim at its source (section 3.5, principle 1). For a request of bigm, the clanker sends the claims to bigm as `DONE: info <subject>`. A scout has no edge to bigm. `question_open` is denied to it, so it has no question ID for a P0, and `mail_post` refuses each post of a scout to bigm (`mailAllowed` in `mcp/mail.go`). When `session_list` shows its clanker with `waitingFor` equal to `permission prompt`, the scout opens no question: it writes the event `clanker-<project> at permission prompt; mail pending` with the source read of its `session_list` call, sends the nudge, and stops. A nudge cannot approve a prompt. The bigm sweep raises the P0 for the prompt of the clanker (section 15). Agent-derived, needs owner decision (design.md, L54).
- End: after its `DONE`, the scout stops. One question gets one report, and a follow-up question gets a new scout. A scout is never resumed and never respawned. At a permission prompt or a classifier refusal, the scout stops at once, and when it can still act, it sends `DONE: scout <subject> refused` to its clanker with the command and the refusal. The clanker raises the P0 (principle 6), and no other session runs the refused command. A scout that failed or stopped without its `DONE` gets a new scout.
- Cap: a scout does not count against `max_busy_clerks`, like the merger clerk and the ledger clerk in the cap count of the clanker (section 3.7). A scout lives for one question and starts no workflow agents, because `Workflow` is denied. Owner decision 2026-10-04, "Scouts do not count" (design.md, L53).
- A project on a remote machine: the remote clanker starts the scout on its own machine, in a checkout there. It sends the claims to bigm with `orca orchestration send` and the subject `DONE: info <subject>`, because bigm cannot read the report file of a remote machine (section 4.2). When bigm cannot run the call of a status claim on its own machine, for example a path of the remote checkout, the claim is "unverified" (principle 1).

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

### 4.3 Local sessions in Orca

Status: built (CHANGELOG, Unreleased). The probes P1 and P6 did not run (see "Probes").

The owner said in the terminal on 2026-10-04 (2026-10-04T11:39:24Z): "yo, bruh yes, for local case - see if orca installed and make sure running sessions are visible in orca. but you also should support claude --bg". The owner looked for the running local roles in Orca and found no trace of them.

- When Orca is on the machine and runs, the running local role sessions are visible in Orca. bruh also supports `claude --bg`. Owner decision 2026-10-04.
- The local roles are the clankers, the task clerks, `clerk-ledger`, and the merger clerks. Without Orca, the local mechanism of section 4.1 stays exactly as it is today. Agent-derived, needs owner decision.
- Orca is optional. The plugin works for each user who installs it, with or without Orca. Nothing of the setup of one user goes into the plugin: no repository, no Orca setting, and no Orca launcher configuration. Agent-derived, needs owner decision.

The facts below explain why Orca showed no local role before this build. Each fact is in knowledge.md, sections "Orca" and "Background sessions (agent view)".

1. `session_launch` starts each local role with `claude --bg`, as section 4.1 says.
2. The Claude hook of Orca exits at once when `CLAUDE_JOB_DIR` is set. So Orca gets no event of a background session.
3. `orca terminal list` lists only the terminals that Orca manages. A background session has no Orca terminal.
4. An Orca terminal is in a worktree of a repository that Orca knows. Verify (probe P6): what `terminal create --worktree path:<folder>` does for a folder of a repository that Orca does not know.
5. A background session can have the `ORCA_*` variables of another Orca terminal, for example of the terminal of bigm. So an `ORCA_*` variable does not prove that a session runs in its own Orca terminal.

#### Owner decisions

The owner answered the questions Q-bruh-maksyms-macbook-pro-41, -42, -46, -48, -50, and -51 in the terminal on 2026-10-04 (2026-10-04T20:01:30Z): "go ahead. BUT MAKE SURE YOU ARE NOT OVERCOMPLICATING THINGS. the setup we develop must have some free choices fro agents, not too hard framing - this limits the prformance".

- O1 (Q-bruh-maksyms-macbook-pro-41): the plugin option `orca_local` is a string option with the values `auto` and `off`, default `auto`. Owner decision 2026-10-04.
- O2 (-42): the minimum version of the Orca app is the code constant `orcaMinVersion = "1.4.218"`. Owner decision 2026-10-04.
- O3 (-46): a local role starts with `claude --bg` as today, plus an Orca viewer tab that runs `claude attach <short ID>`. Owner decision 2026-10-04.
- O4 (-48): a viewer opens at each launch and each resume. The parent closes the tab of a retired role. Owner decision 2026-10-04.
- O5 (-50): a task clerk makes its worktree under `.claude/worktrees/` with `EnterWorktree`, as today. Owner decision 2026-10-04.
- O6 (-51): the trust step of the init skill runs `orca repo add` for each repository after a yes of the owner. Owner decision 2026-10-04.

#### Detection

- The MCP server detects Orca in code at each `session_launch`, `session_resume`, and `session_tab_close`, and keeps no cached result. No agent text runs it. Agent-derived, needs owner decision.
- It reads `orca_local` at each call from `pluginConfigs["bruh@oter"].options` of the user settings file (`BRUH_SETTINGS_FILE`, default `~/.claude/settings.json`), so a change in `/config` applies at once. Only `off` turns Orca off. No value, no file, or a file that does not parse counts as `auto`. Agent-derived, needs owner decision.
- It selects the executable: the value of `ORCA_CLI_COMMAND` when it is set, else `orca-ide` on Linux, else `orca`. It never runs a name whose last path part is `orca-dev`. On Linux it never runs a name whose last path part is `orca`, also from `ORCA_CLI_COMMAND`, because a bare `orca` is the GNOME screen reader. Then it looks the name up on `PATH`. Agent-derived, needs owner decision.
- With `orca_local` `off`, or with no executable, bruh runs no `orca` command, the `claude` arguments do not change, and the result has no field `orca` and no field `orca_error`. Agent-derived, needs owner decision.
- Orca is ready when `<orca> status --json` ends within 10 seconds with `ok` `true`, `result.app.running` `true`, `result.runtime.reachable` `true`, `result.runtime.state` `ready`, and `result.runtime.appVersion` at least `orcaMinVersion`. The version has three numbers, compared in order, and a version that does not parse fails. When a check fails, `orca_error` names the failed field, and no tab opens. Agent-derived, needs owner decision.
- bruh never runs `orca open`. The start of a desktop app is not part of the start of a session. Agent-derived, needs owner decision.

#### Viewer tab

- The launch of section 4.1 does not change: the same command, the same flags, the same lock of the role key, and the same check for a live session of the same name. Agent-derived, needs owner decision.
- After `claude agents --json --all` shows the new session, `session_launch` lists the Orca terminals and closes the whole tab of each one whose `title` is exactly the role key (`<orca> terminal close --terminal <handle> --tab --json`), because the old tab attaches to the short ID of an older session. Then it runs `<orca> terminal create --worktree path:<folder> --title <role key> --command "claude attach <short ID>" --json`. The folder is the `cwd` of the launch, cut before `/.claude/worktrees/`, so the tab of a role that starts in a worktree goes to its main checkout. Agent-derived, needs owner decision.
- `session_resume` opens a viewer in the same way when no Orca terminal has the role key as its exact `title`. It keeps a tab that exists. Its folder is the `cwd` of the last session, cut before `/.claude/worktrees/` in the same way, so the tab of a clerk goes to the main checkout, not to the worktree that the clerk moved into. Agent-derived, needs owner decision.
- The result of `session_launch` and `session_resume` gets the field `orca` with the handle of the tab, or the field `orca_error` with the reason. An Orca failure never fails the call and is never a P0, because the session runs. The MCP server does not try again. Agent-derived, needs owner decision.
- The command text of a tab is only `claude attach` and the short ID, which is hex (`shortIDRE`). A message body never goes into it (section 4.1). Agent-derived, needs owner decision.
- bruh keeps no handle file. A tab is found by its exact title in `<orca> terminal list --json`. `session_list` does not change and has no tab field. Agent-derived, needs owner decision.
- `session_tab_close` (`role_key`) closes the tab of a retired role: the whole tab of each Orca terminal whose title is exactly the role key. Only the parent of the key, or bigm, can call it. It never stops the session. Without Orca it runs nothing. It returns `role_key` and `closed` (the number of closed tabs), plus `orca_error` when an Orca call fails; `closed` then counts the tabs closed before the failure. Its tool description tells the parent when to call it: after it accepts the result of a clerk or gives up its task, after it stops a merger clerk, and after it stops a clanker for good. The agent files name it in three steps: the clanker calls it after it accepts the result of a clerk and after a merge, and bigm calls it after it stops a clanker for good. No agent step gives up a clerk's task, so only the tool description covers that case. An `orca_error` never blocks the retirement. Agent-derived, needs owner decision.

#### What stays as today

- The P0 of a permission prompt carries `claude attach <id>` (section 14.2). It names no tab: the tab has the role key as its title. Agent-derived, needs owner decision.
- An attached session keeps its process, so the supervisor does not stop a session while its viewer is open. This costs one live process for each role with an open tab. When the owner closes the tab, the attach ends, the session keeps running, and the idle stop applies again. Agent-derived, needs owner decision.
- The reboot check and the failure handling of section 15, `claude respawn`, and `claude stop` do not change. Verify (probe P4): whether Orca runs the command of a tab again when it restores the tab after an Orca restart or a reboot. A restored `claude attach <short ID>` would restart a `failed` session before the reboot check of bigm. Until the probe runs, the design assumes that Orca does not run the command again. Agent-derived, needs owner decision.
- The worktrees do not change (O5). Orca shows a worktree under `.claude/worktrees/` only when the user set the visibility of external worktrees to "show" for the repository. bruh does not change that setting. Agent-derived, needs owner decision.
- The user sees one tab for each running role in the Orca worktree of its start folder, with the role key as its title. The tab shows the live session in full screen. It shows no Orca agent state (fact 2). Agent view still lists each role. Agent-derived, needs owner decision.

#### Init

- The trust step (init skill, step 12) runs only when `orca_local` is not `off` and the Orca executable is found, with the same order and the same Linux rule as the detection. It runs `<orca> repo list --json`, shows each folder of the trust list that Orca does not know, and asks one select. Only after "add to Orca" does it run `<orca> repo add --path <folder> --json` for each folder. Without Orca it skips the step silently. Agent-derived, needs owner decision.

#### Probes

The owner gave a go for P1 and P6 in the terminal on 2026-10-04 (2026-10-04T20:05:11Z): "ok". The worktree guard of the build session refused the command that makes the scratch worktree, and bigm decided on 2026-10-04 (question Q-bruh-maksyms-macbook-pro-58, under R-4) to skip both probes. Each probe runs in a scratch linked worktree of a trusted repository, and the cleanup stops each session and closes each tab that the probe made.

- Verify, probe P1 (the viewer): `claude --bg --name probe-o1 "Reply ok, then wait."`, then `orca terminal create --worktree path:<folder that Orca knows> --title probe-o1 --command "claude attach <short ID>" --json`. Pass when `orca terminal list --json` has the tab with the exact title `probe-o1` after the attach starts, and after `orca terminal close --terminal <handle> --tab --json` the session still has a `pid` in `claude agents --json`. Risk until it runs: Orca shows the tabs of Claude Code sessions with titles such as `✳ bruh:bigm`. If `claude attach` replaces the title that `--title` set, the exact-title match breaks: `session_launch` does not close the old tab, `session_resume` opens a second tab, and `session_tab_close` closes nothing. Not run.
- Verify, probe P6 (the folder selector): `orca terminal create --worktree path:<folder> --title probe-o6 --command "true" --json` for a folder of a repository that Orca does not know. Record the result. Not run.

#### Tests

- Go unit tests of the MCP server with a fake `orca` executable that logs each call (`session_test.go`): the option `off` (no call), no executable, the viewer at launch with the exact arguments and the close of an old tab, each readiness condition alone (`ok`, the app, the runtime, the state, the version, output that is not JSON, an exit code, a timeout), a failed `terminal create`, the viewer at resume (an open tab, no tab, a near title), the executable order on Linux and macOS, and `session_tab_close`. The existing session tests run with no Orca and do not change. Agent-derived, needs owner decision.
- The smoke test of section 20 does not change. Agent-derived, needs owner decision.

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
grants.md             post grants, merge grants (section 8.3), and command grants (section 9.5)
questions.md          open P1 questions, oldest first, with age and what they block
owed.md               items owed to the owner, and asks of the owner
leases.md             the lease table of the clankers (section 8.4)
monitors.md           the active monitors, a copy of monitor_list (section 9.5)
projects/<project>.md one file for each project
learn/tree.json       the hierarchy (section 8.5)
learn/projects/<key>.json  the index of each project (section 8.5)
.claude/settings.json the start settings of bigm (section 3.4)
```

Version 0.6 adds `learn/`: owner decision 2026-10-02. Version 0.6 adds `.claude/settings.json`: owner decision 2026-10-02. `monitors.md`: owner decision 2026-10-04 (M7, section 9.5). The command grants of `grants.md`: owner decision 2026-10-04 (M6, section 9.5).

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
- bigm changes a table row or a `key: value` line with `ledger_edit`, which commits only that file and writes the DONE mail to `clerk-ledger` (section 8.7). A rule section of `rules.md`, the sections "Summary" and "Decisions" of a project file, and the files of init and of the learn step keep the hand commit. Agent-derived, needs owner decision.
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
- The size check counts each `*.md` file at the top of the ledger folder and each `projects/*.md` file. It counts no file in another folder, such as `research/`, `learn/`, or `.git/`, because those files are not ledger state. The number of lines of a file is the number of `\n` characters, plus 1 when the file does not end with `\n`. Agent-derived, needs owner decision.
- The details of this section, except the first paragraph, are agent-derived, accepted 2026-10-03. `ledger_max_lines` is a runtime setting with the default 300.

### 8.7 Ledger tools

Status: built (design.md, L55).

bigm edited the tables of the ledger with a hand-written `python3` heredoc, then ran `git add` and `git commit`, and then sent a DONE message to `clerk-ledger`. After the owner saw this, the owner said in the terminal (2026-10-04T10:15:21Z): "maybe for this have a mcp functionality, so no need to craft commands every time ?". bigm wrote the task in its START message (2026-10-04T10:15:29Z):

```text
Today bigm changes the Markdown tables of the ledger (projects/<project>.md, owed.md, questions.md, leases.md, grants.md, rules.md) by crafting a new edit command each time, then commits by hand and sends "DONE: ledger commit <sha>" to clerk-ledger. Design bruh MCP tools that do this in one call: add a row, update cells of a row, and close a row (delete it with the commit subject "close <kind>: <subject>" and a body with the owner's words). The commit and the push message to clerk-ledger should be part of the tool. Times come from the server clock. Keep bigm the only writer of the ledger (only role key bigm may call the tools). Keep the ledger plain Markdown that a human can read and edit by hand. ... the design goes to the owner for review before you apply it, open choices go to me as P1 with OPTION lines, docs and code ship in one PR, and the change must be general for any developer who installs bruh.
```

The owner asked for a design first. The decision log is in design.md, L55. The item that stays open is in section 22, item 6.

The owner answered the five open choices of the first design (owner answer, terminal, 2026-10-04T11:29:35Z, relayed by the clanker `clanker-bruh`): OC1 "One edit tool", OC2 "Nudge in output", OC3 "Refuse dirty target", OC4 "Rows and key lines", and OC5 "Tokens only". For OC1, the owner did not select the recommendation of the first design. Then the owner approved the fix plan of the review findings of the first design (owner approval of the fix plan, terminal, 2026-10-04T11:34:42Z, answer "Apply the plan"). That approval also settles the shape of the action `set_key`. At the build, the clanker `clanker-bruh` cut details of the design under the rule R-3 of the ledger of the owner ("do not overcomplicate bruh"). The cuts are not owner decisions (design.md, L55).

Each decision bullet ends with one tag from the list at the top of this file. A bullet that starts with "Fact:" is not a decision and has no tag.

#### Owner decisions

- OC1: the ledger has one tool, `ledger_edit`. Its input `action` is `add`, `update`, `close`, or `set_key`. `add` adds one row at the end of a table. `update` changes the named cells of one row, and each other cell keeps its bytes. `close` deletes one row. `set_key` sets the value of one `key: value` line. Rejected: "Edit and commit tool", "Three row tools" (the recommendation of the first design), and "One tool for each file" (custom). Owner decision 2026-10-04.
- OC2: `ledger_edit` writes the DONE mail to `clerk-ledger` and returns `nudge`. bigm sends the header of `nudge` with `SendMessage`, and calls `session_resume` when `clerk-ledger` has no `pid`. The MCP server cannot call `SendMessage`. Rejected: "Tool resumes the clerk", "Clerk watches its mailbox", and "Git hook" (custom). Owner decision 2026-10-04.
- Clerks do all pushes (section 8.2), so `ledger_edit` never pushes. Owner decision 2026-09-30.
- OC3: `ledger_edit` refuses a target file that has a change that is not committed, and commits only the target file. Each other changed file stays as it is. Rejected: "Refuse any dirty file" and "Add all". Owner decision 2026-10-04.
- OC4: rows and key lines. A rule section of `rules.md`, and the sections "Summary" and "Decisions" of a project file, keep the hand procedure (an edit, a commit, and the DONE mail). Rejected: "Rows only" and "Rows, key lines, and rules". Owner decision 2026-10-04.
- OC5: a time column accepts only `now`, `now+<duration>` (a Go duration, for example `now+30m`), `none`, or an empty cell. Each time comes from the server clock. Rejected: "Tokens or a stamp" and "Free text". Owner decision 2026-10-04.
- The rows of `monitors.md` use the same generic table path as each other ledger table, with no special case. Owner decision 2026-10-04.
- Only the role key `bigm` calls `ledger_edit`. Owner decision 2026-10-04. Fact: like the other role checks, it is a speed bump (SECURITY.md): a Bash command can set `BRUH_ROLE_KEY`. A hand edit of the owner stays allowed.
- One cell-text rule applies to each action: split a row at the pipes that are not escaped, read `\|` as `|`, and remove only the spaces and the tabs at the two ends of each cell. No backtick is removed: the cell text of `` `a` and `b` `` is `` `a` and `b` ``, as in the file. So `ledger_edit` does not use `tableCells`. Owner decision 2026-10-04.
- The output has the commit SHA, the cell texts of the row that the tool matched or wrote, and the nudge. With 0 matching rows, or with 2 or more, the tool writes nothing and returns the cell texts of the match columns of up to 10 candidate rows. Owner decision 2026-10-04.
- Each close needs `words`. The body of a close has the line `Words:`, and then the words as a Markdown quote block: each line of the words gets the prefix `>` and one space. The message goes on the standard input, and `git commit` runs with `--cleanup=verbatim`. The tool refuses empty `words`, and `words` with a control character other than a newline and a tab. Owner decision 2026-10-04.
- The tests have table-driven cell-text cases (`` `a` and `b` ``, `` `x` ``, `a | b` written as `a \| b`, a value with a space at an end, which is refused, and a non-ASCII text), each written with `add` and found again with `update` and `close`. A test reads a close back with `git log -1 --format=%B` and compares it byte for byte, for words with a newline, a double quote, a backtick, a line that starts with `#`, and spaces at the end of a line. Owner decision 2026-10-04.

#### What the code does

The code is `mcp/ledger.go`, and its tests are in `mcp/ledger_test.go`.

| Input | Actions | Meaning |
|---|---|---|
| `action` | all | `add`, `update`, `close`, or `set_key`. |
| `file` | all | The path of the ledger file, relative to the ledger folder. |
| `table` | add, update, close | The text of the heading above the table. Optional when the file has one table. |
| `match` | update, close | An object of header text to the exact cell text of the row. |
| `cells` | add, update, set_key | For an add, each header of the table to its value. For an update, the changed cells. An empty string is an empty cell. For `set_key`, exactly one pair: the key and its new value. |
| `kind` | add, update, close | One word, for example `task` or `question`. |
| `subject` | all | The subject of the commit, one line. |
| `words` | close | The words of the owner, word for word. In autonomous mode, the decision of bigm and its reasons (section 11). |
| `source` | close | Where the words come from, one line: `terminal`, the channel and its thread, or the question ID. |
| `decision_by` | close | `owner` or `bigm`. |

- Order of the checks: the caller check first, before any file read or lock. Then the inputs: `subject` is one line with no control character; `kind` is one word; each value of `cells` and `match` is one line with no control character and no space or tab at either end; an add and an update need `cells`, an update and a close need `match`; the key of `set_key` matches `[a-z0-9_]+`; a close needs `words`, `source`, and `decision_by`. An input that the action does not use is ignored. Agent-derived, needs owner decision.
- The file rule: `file` is an existing regular `*.md` file of the ledger folder (`ledger_path` of `<plugin data folder>/init/config.json`), not under `learn/`, `.git/`, or `.claude/`. An absolute path, a path with `..`, and a symbolic link are refused. The tool makes no new file. Agent-derived, needs owner decision.
- `table` is the text of the nearest line above the header line that starts with `#`, without the `#` signs and the spaces at its two ends. A table is found with `isTableHeader` and `tableRows`, as init finds it. Two tables under the same heading, an empty header cell, or two header cells with the same text are an error. A column is named by the cell text of its header cell. Agent-derived, needs owner decision.
- A row is written as `| <cell> | <cell> |`, with each `|` of a value written as `\|`. An add puts the new row after the last row of the table, with the line ending of the header line. An update replaces only the changed cells, so each other cell keeps its bytes. A missing cell of a short row reads as empty, and an update adds the cells up to the changed column. A close deletes the row line. Each other byte of the file stays the same: the line endings (LF or CRLF), and a missing last newline too. Agent-derived, needs owner decision.
- `match`: each value is compared byte for byte with the cell text. Exactly one data row must match each pair. With any other count, the error has `count` and `candidates` as JSON: the match-column cell texts of the first 10 data rows when no row matches, or of the first 10 matching rows. An unknown column is an error. Agent-derived, needs owner decision.
- `set_key`: exactly one line outside the fenced code blocks starts with `<key>:` and one space. The tool replaces only the text after it. With 0 lines, or with 2 or more, it writes nothing and returns the count. `now` and `now+<duration>` give the server time, and each other value is written as given. Agent-derived, needs owner decision.
- A column whose header text ends with `(UTC)` is a time column. The tool writes a time of the server clock (`Env.Now`) in the layout `2006-01-02T15:04:05Z`, the layout of `date -u +%Y-%m-%dT%H:%M:%SZ`. A time inside a free-text cell is not checked. Agent-derived, needs owner decision.
- The output is `sha` (the full SHA), `row`, and `nudge`. `row` maps each header to the cell text of the row as read back after the write. For a close, it is the deleted row. For `set_key`, it is the key and its value. `nudge` is `{to: "clerk-ledger", header: "DONE: ledger commit <short SHA>"}`. Agent-derived, needs owner decision.
- The tool holds the lock `ledger` (`Env.WithLock`) from the dirty check to the end of the mail. It runs `git` only in the ledger folder, with `--literal-pathspecs`: `status --porcelain -- <file>` (any output, also an untracked file, is a dirty target), then `commit --only --cleanup=verbatim -F - -- <file>` with the message on the standard input, then `rev-parse`. It writes the file with `atomicWrite` and keeps its file mode. Agent-derived, needs owner decision.
- The subjects are those of section 8.6: `add <kind>: <subject>`, `update <kind>: <subject>`, `close <kind>: <subject>`, and `set <key>: <subject>`. The body of an add, an update, and a `set_key` is the line `Recorded (UTC): <server time>`. The body of a close is below. An empty line of the words is the line `>`. `<date>` is the UTC date of the server clock, and the tag is `bigm decision <date>` when `decision_by` is `bigm`. Agent-derived, needs owner decision.

  ```text
  Words:
  > <line 1 of the words>
  > <line 2 of the words>
  Source: <source>
  Recorded (UTC): <server time>
  Tag: owner decision <date>
  ```

- When the commit fails, for example when a hook refuses it, the tool writes the old bytes back, runs `git reset -q -- <file>` (the target was clean, so its index entry was `HEAD`), and returns the output of `git`. Agent-derived, needs owner decision.
- After the commit, the tool writes the mail with the code of `mail_post`: from `bigm` to `clerk-ledger`, with the header `DONE: ledger commit <short SHA>`, and a body with the full SHA, the subject, and the file. When the mail fails, the commit stays, and the error has the SHA and tells bigm to post the DONE mail with `mail_post`. Agent-derived, needs owner decision.
- Each refusal writes nothing and makes no commit. The tool retries no step. Each call reads the file again, so a hand edit between two calls stays. Agent-derived, needs owner decision.
- The tests use a temporary git ledger with the global and system git config off, and fail when `git` is missing: one round trip of each action (with the exact subject, the mail, the nudge, a CRLF file with no last newline, and another staged file that stays out of the commit), the cell-text cases and the close body of the owner decisions above, the refused words, a match error with 0, 2, and 12 matching rows, a dirty target, the caller check, the time tokens, a failing `pre-commit` hook, and the file rule. Agent-derived, needs owner decision.
- The dependents change in the same pull request: agents/bigm.md, sections 8.2 and 10.1, SECURITY.md, README.md, the ledger template `README.md`, docs/flow.md, and CHANGELOG.md. An existing install runs `/bruh:init` again to allow the tool. Agent-derived, needs owner decision.

## 9. Loops

### 9.1 Sweep

Agent-derived, accepted 2026-09-30.

- bigm runs the sweep as a recurring `CronCreate` task every 15 minutes, and a message wakes it sooner. `CronCreate` tasks are restored on resume. A recurring task expires after 7 days, so bigm creates it again every 6 days.
- The sweep reads all report files, reconciles `claude agents --json --all` and Orca `worker-list`, reads the source for each row past its next check, sends the P1 batch when it is due, and updates the ledger.
- A `Monitor`, a background command, and a self-paced `/loop` are not restored on resume. So on a resume, bigm starts the Orca receive loop again. In the setup of the owner, the sweep loop died on a usage limit, and nobody restarted it. The poller of section 9.2 is a plugin monitor, which starts with the session, so bigm does not start it (owner decision 2026-10-04, M2, section 9.5).
- At each sweep, bigm calls `monitor_list`. When `poller_down` is true, it shows "poller down" under "Waiting on you" (section 9.4), with `/reload-plugins` as the step that starts the poller again. It calls `monitor_stop` for each monitor of a retired role, and rewrites the rows of `monitors.md` from `monitor_list` with `ledger_edit` (section 9.5, part h). Agent-derived, needs owner decision.
- Verify: `CronCreate` tasks run in an interactive bigm while the owner is away from the terminal.
- At each sweep, bigm calls `learn_refresh` (section 8.5) and commits a change of the files of the learn step. Owner decision 2026-10-02: the refresh at each sweep. Agent-derived, accepted 2026-10-03: the tool and the commit.
- The refresh makes no model call and starts no agent (owner decision 2026-10-02). The commit of a refresh names each doc pointer that changed its `gone` state (section 8.5). Agent-derived, needs owner decision.

### 9.2 Watcher

Agent-derived, accepted 2026-09-30.

- A plugin script watches the code host for events of the projects: new pushes, replies, red pipelines, merges. It writes each event to the report file of the project of the event, not to one shared file. Owner decision 2026-10-03. In the setup of the owner, sessions reviewed a change, left comments, and then did nothing more until a watcher was added.
- The watcher is the poller of section 9.5, with the source kind `codehost`, and each repository of `repos_set` is a standing monitor of the clanker of its project: owner decision 2026-10-04 (M8). The poller runs in the plugin monitor `bruh-poller` of bigm (`monitors/monitors.json` starts `scripts/watcher.sh`), not in a `Monitor` tool watch: owner decision 2026-10-04 (M2).
- The details below are agent-derived, needs owner decision:
  - The report file of a project is `<plugin data folder>/reports/clanker-<project>.jsonl`, the report file of its clanker. `<project>` is the project of the repository in `repos_set`.
  - `repos_set` requires `project` for each repository, and checks it with the project grammar of a role key (`projectRE`, section 3.2). The watcher skips a stored entry with no `project` and logs it, and bigm calls `repos_set` again for that repository.
  - The watcher and `report_write` append whole lines with one write each to a file that is open with `O_APPEND`, so no lock is necessary.
  - One rule for the events: the poller puts each event into the mailbox of each local subscriber, from `bigm`, with the header `DONE: event <project>: <subject>` and the event line in the body, so a local clanker gets the events of its standing monitors with no relay. The poller prints the event line, which reaches bigm as a notification of the plugin monitor, only for a remote subscriber and once for an `error` event. bigm relays each printed event of a remote clanker with Orca `send` and the same header (section 4.2). At each sweep, bigm reads the report files with `since` only to find events of remote clankers that it did not relay (section 9.1). `report_read` has no role key `watcher`.
  - When bigm retires the clanker of a project, it removes the repositories of that project from `repos_set`, so that the poller stops its polls of them and their standing monitors end.
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

Status: Built 2026-10-04.

Owner decision 2026-10-04: a role starts a monitor on demand for each external state that its work depends on, and the mechanism is general, for each developer who installs bruh. The examples of the owner (a push, a task tracker, team work in another system) show the range only. bruh does not hard-code them. The words of the owner are in design.md, L56.

The verified facts for this section are in knowledge.md, "Monitors, background tasks, and event delivery". The owner decided the choices M1 to M9 on 2026-10-04, and part k lists them. The rejected options and their reasons are in design.md, L56. Part k also lists the decisions of the build that are not owner decisions: the limit to local roles (bigm, Q-bruh-maksyms-macbook-pro-56), and the cuts of the clanker of the bruh project under RULE R-3. Each decision bullet carries its tag. Section 22, item 7, lists the agent-derived items of this section.

Terms:

- Monitor: one subscription. It has a source, a subscriber, a project, a reason, and an expiry (`until`).
- Source: the description of one external state, as data (part b).
- Source key: the canonical identity of a source. Monitors with the same source key share one poll and one cursor.
- Subscriber: the role key that gets the events of the monitor. The subscriber owns the monitor.
- Poller: the process that polls the sources and delivers the events (`scripts/watcher.sh`, `mcp/watch.go`).
- Waiter: the hook script that wakes the session of a subscriber on new mail (`scripts/wake.sh`).
- Event: one change of a source, as one JSON line.

#### a. Who starts a monitor

- Each local role can start a monitor: bigm, a clanker, or a clerk. The subscriber is the caller. Agent-derived, needs owner decision.
- The general trigger: the next step of the role waits on a state outside its session, and that state can change without an action of the role. Examples, as categories only: the result of checks after a push, a reply of a person in another system, the state of a task that the role took from a tracker, and the progress of work of others in a shared system. Agent-derived, needs owner decision.
- A role does not wait with `sleep`, and does not ask the same source again in a loop. Each such wait becomes a monitor. The agent files of bigm, the clanker, and the clerk carry this rule. It has no mechanical stop yet: a deny rule in the role settings is a follow-up. Agent-derived, needs owner decision.
- The MCP tool `monitor_start` takes `source`, `reason` (one line), the optional `project`, and the optional `hours`. The expiry is the clock of the MCP server plus `hours`, so the model writes no time. It returns `id` (`m-` and 8 random characters), `key`, `until`, `shared` (true when another active monitor, standing or stored, has the same source key), and `source`, the source read of principle 1 (`call`, `value`, `at`). Agent-derived, needs owner decision.
- The baseline: when the source key has no cursor yet, `monitor_start` polls once (a `codehost` source with the poll code of section 9.2, a `command` source with one run), stores the result as the cursor, and returns its source read. A key that has a cursor returns the last stored read. An `mcp` source has no baseline at the start: its first `monitor_report` is the baseline. After the baseline, events are changes only. `monitor_start` stores the monitor before it writes the cursor, so the poller does not prune the new cursor. Agent-derived, needs owner decision.
- `monitor_start` refuses a `codehost` or `command` source while `poller_down` is true (part e), with an error that says the poller runs only in the session of bigm on this machine. An `mcp` source is exempt, because its role polls it with `monitor_report`. Agent-derived, needs owner decision.
- The project: a clanker or a clerk starts a monitor only for its own project, which is the default. bigm passes `project`. A `codehost` source takes the project of its entry in `repos.json`. Agent-derived, needs owner decision.
- A workflow agent does not start a monitor, because a monitor outlives the run. It asks its clerk, and the clerk starts the monitor. Agent-derived, needs owner decision.
- A monitor of a clerk at the end of its task: owner decision 2026-10-04 (M4, option 3). The clerk lists its monitors in its result, and the clanker decides for each one at the accept.
- The form of M4: the result of the clerk (its `report_write` of the kind `result`) lists each monitor that `monitor_list` shows as active for the clerk: the monitor ID, the source key, the reason, and `until`. At the accept of the result, or when it gives up the task, the clanker stops each listed monitor with `monitor_stop`, or takes it over: it calls `monitor_start` with the same source, which shares the poll and the cursor of the same source key, and then stops the monitor of the clerk. Between the two calls, an event goes to both subscribers, so no event is lost. An `mcp` source has no shared poll, so the clanker starts its own poll for it. Agent-derived, needs owner decision.
- Monitors are for local roles only. A remote role starts no monitor, because the one poller runs on the machine of bigm (M3). `monitor_start` refuses a `codehost` or `command` source while `poller_down` is true, and that is always the case on a machine without the poller of bigm. An `mcp` source is not checked, so the rule of the agent file of the clanker (a remote clanker starts none, and tells its clerks so in their start messages) covers only that gap. The standing monitors of a remote clanker stay on the machine of bigm, and bigm relays their events through Orca, as section 9.2 says. bigm decision 2026-10-04 (Q-bruh-maksyms-macbook-pro-56, under RULE R-4). Agent-derived, needs owner decision.

#### b. What a monitor watches

- Plugin code stores each monitor in `<plugin data folder>/monitors.json` (principle 3), under a file lock. The poller reads the file again before each loop, as it reads `repos.json`. `monitors.json` is the source of truth for the poller and for each role. Agent-derived, needs owner decision.
- A source has a closed schema: `kind` and the keys of that kind only. An unknown key, or a key of another kind, is an error. There are three kinds (R-3 cut 1: no `http` kind; an HTTP API is polled through a granted CLI). Agent-derived, needs owner decision.
  - `codehost` `{repo, ref?, number?}`: a repository of `repos_set` with a project. `monitor_start` refuses a repository that is not there ("bigm calls repos_set"), and copies `host` and `api_url` from the entry; the caller cannot pass them. The events are the events of section 9.2: push, red checks, comment, review, and merge. With `ref` or `number`, the monitor gets only the events of that branch or that pull request, and each `error` and `expired` event. A `codehost` monitor gets events only while its repository is in `repos.json`.
  - `command` `{argv, items, id, version, title?}`: a read-only CLI that prints JSON. `argv` is an array, run with no shell, with a time limit of 30 seconds and at most 1 MiB of standard output. The CLI keeps its own login.
  - `mcp` `{server, tool, args?, items, id, version, title?}`: a tool of an MCP server in the session of the subscriber. The role polls it, not the poller (M5, below).
- `items` is a JSON pointer (RFC 6901) to an array, or `""` for the root. `id` and `version` are pointers into each item, and the optional `title` is a pointer to a text for the subject. A value is a string as it is, or else compact JSON. Each item needs a non-empty and unique ID; else the read is an error. Code emits `new` for a new ID, `changed` for a new version, and `gone` for an ID that is not there any more, and the subject is the title or else the ID. A result that is a JSON string (the text of an MCP tool result) is decoded once more. These rules use structure and exact values only, never the meaning of words (the owner rule against regex classifiers). Agent-derived, needs owner decision.
- The source key is `<host>:<repo>` for `codehost`, the state key of the watcher, so an upgrade needs no new baseline, and `ref` and `number` are filters, not parts of the key. It is `command:` with the first 16 hex characters of the SHA-256 of the `argv` and the pointers, and `mcp:<monitor ID>`, because a source of the kind `mcp` has no shared poll. Agent-derived, needs owner decision.
- Examples use placeholders only: `owner/repo` and `["tracker-cli", "list", "--json"]` for an issue tracker CLI. bruh names no tracker and no code host product as a built-in case. Agent-derived, needs owner decision.
- A system that has only an MCP server: owner decision 2026-10-04 (M5, option 1). The role polls it with a `CronCreate` task, and reports each result with the MCP tool `monitor_report`.
- The details of M5, each agent-derived, needs owner decision:
  - `monitor_start` records a source of the kind `mcp` in `monitors.json` with its expiry, so that `monitor_list` and `monitors.md` (part h) show it. The poller does not poll it.
  - The subscriber creates one recurring `CronCreate` task for the monitor. At each fire, the task calls the tool with `args`, and gives the result as it is to `monitor_report` with the monitor ID. Each fire costs one turn of the subscriber.
  - `monitor_report` applies the pointers of the source and keeps the cursor in `watch/state.json`. The first call is the baseline and returns `{baseline: true, items: <n>}`. Each later call returns the events and writes each event to the report file of the project. The subscriber has the events in the result of the call, so no mail goes to it. The source read of each event names the MCP server and the tool.
  - `monitor_report` refuses a caller that is not the subscriber, a monitor of another kind, and a monitor that is stopped or past its `until`.
  - At `monitor_stop`, or at the `expired` event, the subscriber deletes its task with `CronDelete`.
- A `command` source runs a program outside the Bash permission rules, because the poller starts it and not the Bash tool. Owner decision 2026-10-04 (M6, option 1): a command grant in `grants.md`, by exact `argv` prefix. The owner approves a CLI with its fixed first arguments, and bigm records it in `grants.md`, as a merge grant or a post grant (sections 8.3 and 13).
- The check of M6 in code: the first cell of each row of the section "Command grants" of `grants.md` is a JSON array of strings, the prefix; a row that does not parse never matches. A `command` source is accepted only when its first `argv` elements are equal, one by one and byte for byte, to a prefix. So the grant `["tracker-cli", "list"]` accepts `["tracker-cli", "list", "--json"]`, and refuses `["tracker-cli", "listx"]` and `["tracker-cli"]`. With no ledger, nothing is granted. `monitor_start` checks it, and the poller reads the grants again at each loop and stops a monitor whose grant is gone with an `expired` event with the cause `grant`. When `grants.md` cannot be read, the poller polls no `command` key in that loop. Agent-derived, needs owner decision.

#### c. How events reach the role

- Durable first, as section 5 says for messages. Each event is one report line (`from: watcher`, `kind: event`, the source read of principle 1, and the event), with the monitor ID and the source key of its subscriber. The poller appends the line once for each project to `reports/clanker-<project>.jsonl` (owner decision 2026-10-03, section 9.2), and puts it into the mailbox of each local subscriber, with the existing header `DONE: event <project>: <subject>` and the line as the body. One inbox for each role and one read tool (`mail_read`), so the rule of bigm for idle roles (resume a role that has unread mail at each sweep) covers events too. Agent-derived, needs owner decision.
- A local subscriber is bigm, or a role key that has a role settings file `<plugin data folder>/roles/<role key>.json` on this machine; a remote clanker writes its role settings on its own machine. The header is cleaned of control characters and cut to the 200 characters that the mailbox accepts. Agent-derived, needs owner decision.
- The poller writes the mail with the sender `bigm`, because it runs in the plugin monitor of bigm (owner decision 2026-10-04, M2, option 1), and the mailbox allows a post from `bigm` to each role (`mail.go`). Agent-derived, needs owner decision.
- The event adds no new header. Its fields: `type` (`push`, `red`, `comment`, `review`, `merge`, `new`, `changed`, `gone`, `error`, or `expired`), `monitor`, `key`, `project`, the optional `subject`, `item`, and `version` of an item, the fields of a code host event of section 9.2, and `cause` (`until` or `grant`) of an `expired` event. Agent-derived, needs owner decision.
- One poller on the machine of bigm serves all subscribers: owner decision 2026-10-04 (M3, option 1).
- The poller prints an event line, which reaches bigm as a notification of the plugin monitor, only for a remote subscriber (a remote clanker with its standing monitors, which bigm relays through Orca as section 9.2 says) and once for an `error` event, so that events of local roles cost no context of bigm. Agent-derived, needs owner decision.
- The wake of the subscriber session: owner decision 2026-10-04 (M1, option 1), an `asyncRewake` hook. Part e describes the waiter.

#### d. Lifetime, expiry, and stop

- Each monitor that a role starts has an expiry: `until` is the clock of the MCP server plus `hours`. When the caller gives no `hours`, it is `monitor_default_hours` of `mode.md`, and `monitor_start` refuses more than `monitor_max_hours`. Both are runtime settings of `mode.md` (section 11), read at each call, with the defaults when the ledger or the line is missing. Agent-derived, needs owner decision.
- The default values: `monitor_default_hours` 24, `monitor_max_hours` 168, and `monitor_max_active` 20 (part g). Owner decision 2026-10-04 (M9, option 1).
- A standing monitor has no `until`: the codehost monitors of a clanker (part i) end when bigm removes their repositories with `repos_set` and `remove: true`, and `monitor_stop` refuses them. Agent-derived, needs owner decision.
- At the first loop past `until`, the poller removes the monitor and sends one `expired` event with the cause `until` to the subscriber, so that the role can start it again when its work still needs it. The monitor is gone, so no second event follows. Agent-derived, needs owner decision.
- The subscriber stops its monitor with `monitor_stop` when its work no longer waits on the source. The parent of the subscriber and bigm can also stop it. Agent-derived, needs owner decision.
- At the end of each loop, the poller deletes the cursor of each source key that has no monitor left, so a shared key keeps its poll and its cursor while one monitor of it is left. Agent-derived, needs owner decision.
- At each sweep, bigm calls `monitor_stop` for each monitor whose subscriber is a retired role (section 8.6). It is the net for a monitor of a retired clerk that its result did not list (M4). Agent-derived, needs owner decision.

#### e. The waiter, and the re-arm after a resume or a compaction

- The monitors are data in the plugin data folder, and the cursor of each source key is in `watch/state.json`. So the poller keeps its polls while a subscriber session is down, and the events wait in the mailbox. No event is lost. Agent-derived, needs owner decision.
- The handoff of each role lists the ID of each active monitor in "Waiting on others" (section 7). After a pickup, the role calls `monitor_list` as the live source before it acts, and never reads `monitors.md`, which can be one sweep out of date (part h). Agent-derived, needs owner decision.
- The wake and its re-arm: owner decision 2026-10-04 (M1, option 1). A `SessionStart` hook with the matcher `startup|resume|compact` starts the waiter of the role key, and the `Stop` hook starts a new waiter at the end of each turn. Both are separate entries of `hooks/hooks.json` that run `scripts/wake.sh` with `asyncRewake: true` and `timeout: 3600`.
- The waiter in code: it exits 0 at once when `BRUH_ROLE_KEY` or `CLAUDE_PLUGIN_DATA` is not set, or when the role key is not a key. It writes its PID to `<plugin data folder>/wake/<role key>.pid`, and at each poll (every `BRUH_WAKE_POLL` seconds, 2 by default) it exits 0 in silence when that file names another waiter. So the newest waiter replaces the older one, one waiter for each role key, with no kill. New mail is a `*.json` file in `mail/<role key>/` whose name is not in `wake/<role key>.seen`. On new mail, the waiter writes the names of the mailbox to that file, prints `bruh: new mail for <role key>. Call mail_read.` on standard error, and exits 2, which wakes the session. After `BRUH_WAKE_SECONDS` (3300 by default), before the hook timeout, it prints `bruh: no new mail for <role key>.` and exits 2. Agent-derived, needs owner decision.
- The `Stop` hook also re-arms after the other kinds of session start (`clear`, and `fork`, which also fires when a conversation moves to the background), at the end of the first turn. So the matcher list stays as M1 says. Agent-derived, needs owner decision.
- A `CronCreate` task of a source of the kind `mcp` needs no re-arm: Claude Code restores it on `--resume` and `--continue` (knowledge.md). A recurring task expires after 7 days, which is equal to `monitor_max_hours`. Agent-derived, needs owner decision.
- The poller is a plugin monitor of bigm (M2). The docs say that a plugin monitor starts at session start and on `/reload-plugins`, and do not say if `--resume` or `--continue` starts it again. So `monitor_list` returns `poller_at`, the time of the last poll loop from `<plugin data folder>/watch/poller_at`, and `poller_down`, which code computes: the file is missing, or older than 3 times `interval_seconds` of `repos.json`. At each sweep, when `poller_down` is true, bigm shows "poller down" under "Waiting on you" of its status report (section 9.4), with `/reload-plugins` as the step that starts the poller again. This catches a poller that did not start again after a resume. Agent-derived, needs owner decision.

#### f. Credentials and identity

Owner decision 2026-09-29 (section 13): a read-only token read inside a process is allowed, and bruh never moves or copies a credential.

- `codehost`: the token rules of today (`hostToken` in `codehost.go`), and `glab api` for that host kind, so that the CLI keeps the login. The use of `glab api`: owner decision 2026-10-02 (design.md, L18). The use of these rules for monitors: agent-derived, needs owner decision.
- `command`: bruh passes no token. The CLI uses its own login. Agent-derived, needs owner decision.
- `mcp`: bruh passes no token. The MCP server keeps its own login in the session of the subscriber. Agent-derived, needs owner decision.
- A monitor makes no write. So the identity check before the first write (section 13) does not apply. Agent-derived, needs owner decision.

#### g. Rate limits and shared polling

- One poll for each source key, shared by all its subscribers. Each subscriber gets each event. Agent-derived, needs owner decision.
- The poller polls each `codehost` key at each loop, every `interval_seconds` of `repos.json` (default 60 seconds, at least 10, as section 9.2 does), and each `command` key at most once in `interval_seconds` or 60 seconds, whichever is longer. Agent-derived, needs owner decision.
- One `error` event for each new error text of a source key, as the watcher did before (R-3 cut 4: no `Retry-After` and no doubling of the interval). A failed `command` (an exit code other than 0, a run over the limits, or output that is not JSON) is an error. Agent-derived, needs owner decision.
- `monitor_max_active` of `mode.md` caps the active source keys of the machine, standing monitors and `mcp` keys included. `monitor_start` refuses a new source key past the cap with a clear error, and the role sends a P2 to its parent. Part d has the default. Agent-derived, needs owner decision.
- One poller for each machine (M2, M3): `scripts/watcher.sh` exits 0 at once unless `BRUH_ROLE_KEY` is `bigm`, and the poller takes a non-blocking lock on `<plugin data folder>/locks/poller.lock` for the life of its process. A second poller, for example of a second bigm, prints one line on standard error and waits: it tries the lock again every 5 seconds, and polls when it gets the lock, so it takes over when the first poller exits. Its wait ends when its session ends. With `--once`, it exits at once. Agent-derived, needs owner decision.

#### h. How the ledger shows the active monitors

Owner decision 2026-10-04 (M7, option 3): a separate ledger file, `monitors.md`.

- L24 (owner decision 2026-10-02: bruh stores no live status) holds for the state of the external systems: `monitors.md` holds the subscriptions, never a polled value, a cursor, or an event. Agent-derived, needs owner decision.
- `monitors.md` has one table, with one row for each active monitor: `| ID | Subscriber | Project | Source key | Reason | Until |`. The column is "Until", not "Until (UTC)", because its value is copied from `monitor_list` (a time from code, or `standing`), and `ledger_edit` accepts only `now` and `now+<duration>` in a column whose header ends with "(UTC)". Agent-derived, needs owner decision.
- Writer: bigm, because bigm is the only writer of the ledger (section 8). At each sweep, bigm rewrites the rows from `monitor_list` with `ledger_edit` (task 3): it adds each new monitor, updates each changed row, and deletes the row of each monitor that is not in the list. `monitor_list` wins each difference, so a row is at most one sweep out of date. The poller prints no lifecycle lines (R-3 cut 5). Agent-derived, needs owner decision.
- A deleted row uses the existing closed kind `waiting` of section 8.6, because a monitor is a wait on an outside state: `close waiting: <monitor ID> <source key>`. So the closed kind list of `ledger_edit` needs no new kind. Agent-derived, needs owner decision.
- The size check of section 8.6 counts `monitors.md`, because it counts each Markdown file of the ledger. Agent-derived, needs owner decision.

#### i. How the watcher of section 9.2 fits

- The poller runs in the plugin monitor of M2, which has no deadline, and not in a `Monitor` tool watch of bigm, which stopped after at most 30 minutes (design.md, L56, F1): owner decision 2026-10-04 (M2, option 1).
- The watcher: owner decision 2026-10-04 (M8, option 1), fold it in. The watcher is the poller, with the source kind `codehost`, and each repository of `repos_set` is a standing monitor of the clanker of its project.
- A standing monitor is derived from `repos.json` at each read and is never stored: its ID is `standing:<host>:<repo>`, its subscriber is `clanker-<project>`, and it has no `until`. Agent-derived, needs owner decision.
- So the rule of section 8.5 and L24 holds: bigm calls `repos_set` only when it starts a clanker, so a project with no clanker gets no polls and no events. The merge train keeps `repos.json` for its configuration. Agent-derived, needs owner decision.

#### j. Tests

- Go tests in `mcp/monitor_test.go`, with temporary data folders, a fake CLI that prints a JSON file and counts its runs, a fake clock, and a ledger fixture (R-3 cut 6). Agent-derived, needs owner decision.
  - A shared poll: two subscribers of one `command` source cause one baseline run and one run for each due loop, and each subscriber gets each event.
  - The baseline and the source read of `monitor_start`, then `new`, `changed`, and `gone` by nested JSON pointers (with the `~1` escape), with titles that mean nothing. A duplicate ID is one `error` event.
  - The expiry: one `expired` event with the cause `until`, the monitor leaves `monitor_list`, and the next loop sends no second event and runs no command. `hours` over `monitor_max_hours` is refused, and `monitor_default_hours` applies, both read from `mode.md`.
  - The command grant by exact `argv` prefix: `["tracker-cli", "list"]` accepts `[..., "--json"]` and refuses `listx`, a shorter `argv`, and a source with no grant or no ledger. A refused source does not run. A deleted grant row stops the monitor with the cause `grant`.
  - The mailbox delivery: a local subscriber gets `DONE: event <project>: <subject>` with the line as the body, the report file gets the same line, a long title with a newline and a control character gives a header that the mailbox accepts, and a remote subscriber gets no mail and a printed line.
  - The poller lock: while the first `runWatch` holds the lock, a second one polls nothing and does not return; after the release, it takes the lock within the retry interval and polls; it returns when its context ends; with `once`, it returns at once.
  - `monitor_report`: the first call is the baseline, then the events, with no mail; a caller that is not the subscriber is refused, and so is a call past `until`.
  - A standing monitor of `repos.json` is in `monitor_list` and `monitor_stop` refuses it; a `codehost` monitor with `ref` gets only the events of that branch; a repository outside `repos.json`, a key of another kind, and the kind `http` are refused.
- `TestHooksJSON` checks the two waiter entries, and `TestMonitorsJSON` checks the plugin monitor against the strict keys of the docs, because `claude plugin validate` does not read the default file `monitors/monitors.json`. The watch tests of section 9.2 run unchanged through the standing monitors. Agent-derived, needs owner decision.
- Script tests in `tests/test.sh`: the waiter exits 0 and writes no pid file without `BRUH_ROLE_KEY`; a second waiter of the same role key replaces the first, which exits 0; the waiter exits 2 on new mail of its role key and not on mail of another role key; mail that a waiter reported does not wake the next one; the waiter exits 2 with "no new mail" at its limit; `watcher.sh` exits 0 with no output and polls nothing without a role key and in a clanker. Agent-derived, needs owner decision.
- The smoke step, documented and not run (R-3 cut 6): running it starts `claude` sessions, so it needs a P1 to the owner first. Steps: in the smoke home, grant a fake CLI in `grants.md`; a clerk starts a `command` monitor on it and goes idle; a change of the file that the fake CLI prints wakes the clerk within one poll interval, and it calls `mail_read`; after `claude stop` of the clerk and `session_resume`, the next change wakes it again; after a resume of bigm, `monitor_list` shows `poller_down` false (the restart that the docs do not state, part e). Agent-derived, needs owner decision.

#### k. Decided choices

The owner decided each choice on 2026-10-04 (terminal, a select, 2026-10-04T11:29:35Z). The rejected options and their reasons are in design.md, L56.

- M1, the wake: an `asyncRewake` hook. A `Stop` hook and a `SessionStart` hook (`startup`, `resume`, `compact`) start a waiter for the role key, and the waiter exits 2 when the mailbox of the role gets new mail (part e). Owner decision 2026-10-04.
- M2, the host of the poller: a plugin monitor of bigm. `monitors/monitors.json` starts `scripts/watcher.sh`, which polls only when `BRUH_ROLE_KEY` is `bigm`, and a lock keeps one poller on each machine (part g). Owner decision 2026-10-04.
- M3, the topology: one poller on the machine of bigm for all subscribers (part c). Owner decision 2026-10-04.
- M4, a monitor of a clerk at the end of its task: the clerk lists its active monitors in its result, and the clanker decides for each one at the accept: it stops it, or takes it over with `monitor_start` on the same source and then `monitor_stop` (part a). Owner decision 2026-10-04.
- M5, a system that has only an MCP server: the role polls it with a `CronCreate` task, and gives each result to `monitor_report` (part b). Owner decision 2026-10-04.
- M6, the approval of a `command` source: a command grant in `grants.md`, matched by exact `argv` prefix (part b). Owner decision 2026-10-04.
- M7, the ledger view: a separate ledger file, `monitors.md` (part h). Owner decision 2026-10-04.
- M8, the watcher of section 9.2: it becomes the poller with the kind `codehost`, and each repository of `repos_set` is a standing monitor of the clanker of its project (part i). Owner decision 2026-10-04.
- M9, the defaults: `monitor_default_hours` 24, `monitor_max_hours` 168, and `monitor_max_active` 20, as runtime settings in `mode.md` (part d). Owner decision 2026-10-04.

Decisions of the build that are not owner decisions. Each is agent-derived, needs owner decision:

- Q-bruh-maksyms-macbook-pro-56, bigm decision 2026-10-04 under RULE R-4: monitors for local roles only (part a). The relay of section 9.2 for remote clankers keeps working.
- bigm added on 2026-10-04 to M6: keep the command grant check strict, because no lease coupling exists (part b).
- The cuts of the clanker of the bruh project under RULE R-3: (1) no `http` source kind; (2) no coupling of monitors and leases; (3) no remote subscriber path; (4) one `error` event for each new error text, with no `Retry-After` and no doubling of the interval; (5) bigm rewrites `monitors.md` from `monitor_list` at each sweep with `ledger_edit`, with no lifecycle lines from the poller; (6) the tests of part j, with one smoke step that is documented and not run.

## 10. Files, the MCP server, and environments

### 10.1 Files and the MCP server

Owner decision 2026-09-29: bruh keeps all its files in the plugin data folder `${CLAUDE_PLUGIN_DATA}`, and plugin code writes them. Claude Code guards `~/.claude` against writes by Claude, but not against writes by plugin code. The only files outside the plugin data folder are the settings keys that the init skill writes after the owner approves the diff, and the private ledger repository.

The plugin ships a stdio MCP server written in Go with the standard library only. Claude Code starts it with `go run -C ${CLAUDE_PLUGIN_ROOT}/mcp .` and `GOTOOLCHAIN=local`, so the repository holds no binaries. Owner decision 2026-09-30. Its tools: `mail_post`, `mail_read`, `handoff_write`, `handoff_read`, `answer_write`, `answer_wait`, `report_write`, `role_settings_write`, `lease_request`, `lease_grant`, `lease_release`. It reads the role key of its caller from `BRUH_ROLE_KEY`. Agent-derived, accepted 2026-09-30. Verify: an MCP server started by a session gets the `env` values of its `--settings` file. Version 0.6 adds the tools `learn_scan` and `learn_refresh` (section 8.5; agent-derived, accepted 2026-10-03), the field `options` of `question_open` (section 14.2; owner decision 2026-10-02), and the input `allow` of `role_settings_write` (section 8.5; agent-derived, accepted 2026-10-03).

The input and the output of `learn_scan` and `learn_refresh` are in section 8.5, "Schema". Agent-derived, needs owner decision.

The tool `ledger_edit` (section 8.7) changes one row or one key line of a ledger file and commits it. The MCP server runs `git` only in the ledger folder, and never pushes. Agent-derived, needs owner decision.

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

Each item that matches an exact command pattern (for example `docker volume rm`) becomes a deny rule in the role settings files, not in user settings, so the manual sessions of the owner are not blocked. These deny rules are speed bumps (principle 2). The other items stay P0 in `priorities.md`. A refusal by a deny rule is escalated as a P0 by text; no hook marks it, because the probe of section 15.1.1 did not run. Agent-derived, needs owner decision.

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
- A P0 for a permission prompt carries the command `claude attach <id>`. Agent-derived, accepted 2026-09-30.
- A P0 for a refusal carries the exact command and the refusal category. With a hold, `question_open` adds them as the `COMMAND` and `CATEGORY` lines from the hold record (section 15.1.4). It has no fixed options; the owner answers freely. This replaces the two options "run it, or add a scoped allow rule" of 2026-09-30. R-3 cut 2, agent-derived, needs owner decision.
- bigm queues P1 questions and shows them as a batch. Owner decision 2026-09-27. The interval and the maximum batch size are runtime settings in `mode.md`, asked by the init skill and changeable at any time. Owner decision 2026-09-29. Agent-derived, accepted 2026-09-30: the defaults are an interval of 60 minutes and at most 5 items; a batch goes out early when 5 items are queued; an empty queue sends nothing; a P0 never waits for a batch.
- Owner decision 2026-10-02: `question_open` takes an optional field `options`: 2 to 4 options, each with a label and a description. bigm shows each P1 question that has options as a select (`AskUserQuestion`), at most 4 questions on one screen, and more screens when the batch has more. bigm shows each question without options as text. This replaces "bigm shows the items of a batch one for each message" of version 0.5.
- The question file of the asker keeps the options. `question_open` returns the body of the message with the options in it, and the asker sends that body. The options travel in the body, one line for each option in the closed form `OPTION <n>: <label> | <description>`, because bigm cannot read the question file of another role and a remote clanker is on another machine. A clanker or a clerk that relays a question copies these lines word for word. bigm reads the options only from these lines. Agent-derived, accepted 2026-10-03.
- The label of an option has 1 to 60 characters on one line, with no `|`. The description has 1 to 200 characters on one line. Agent-derived, needs owner decision.
- The question prompt of each workflow tells an agent to pass `options` to `question_open` when the question has fixed answers, and to return the body that `question_open` returned. Agent-derived, needs owner decision.
- Owner decision 2026-10-02: bigm shows the selects only in a reply to a message of the owner. At another time, bigm shows the batch as text and tells the owner to reply to answer it with selects. An open select holds the turn of bigm, so the sweep and a new P0 would wait. Verify: an open select holds the turn of bigm.
- A `REC` that recommends an option has the subject `OPTION <k>`. bigm puts that option first, and its label says "(Recommended)". Agent-derived, accepted 2026-10-03.
- bigm sends the answer to the session that asked, as an `ANSWER`, and records the question and the answer in the ledger. Agent-derived, accepted 2026-09-30. Since version 0.6, the record is the commit that closes the question (section 8.6). Agent-derived, accepted 2026-10-03.
- A question about which role does a piece of work, how to divide it, or whether to start it is not a question for the owner and has no P-level. The role that routes the work decides it (sections 3.4 and 3.5). Owner decision 2026-10-04 (design.md, L50).
- The stop for this rule, under principle 2, ranked. Agent-derived, needs owner decision.
  1. A structural test in `plugins/bruh/agents/agents_test.mjs`: the rule sentences and the commitment form are in `bigm.md` and `clanker.md`, the eval uses the same form, and sections 3.4, 3.5, and 14.2 carry the owner decision. Built. It proves that the procedure has the rule, not that a model obeys it.
  2. The routing eval in `tests/eval/routing/` (section 20): a model run of bigm on fixed requests, with exact checks of the commitment line and of the `AskUserQuestion` calls. Built. It runs before each release, not in CI, because it starts a model session. A check of the meaning of the reply text would be a regex semantic classifier, which the owner forbids, so the eval checks only the closed line and the tool name.
  3. A closed list of question classes in `question_open` or `mail_post`. Not built. The defect of 2026-10-04 was chat text in the terminal of the owner, which goes through neither tool, so the check would not have stopped it. It also changes the tool contract of each role.

## 15. Failure handling

Agent-derived, accepted 2026-09-30, except where tagged.

- **A reboot comes first.** After a shutdown, a background session shows as `failed` for up to 48 hours, and as `stopped` after that. An attach or a reply restarts it. So bigm checks for a reboot before it applies the rules below: when many sessions changed to `failed` or `stopped` at the same time, bigm reconciles the ledger from `claude agents --json --all`, updates the role key map, and resumes the long-lived roles with `claude --resume <session ID> --bg`.
- **A session waits on a prompt:** `waitingFor` equal to `permission prompt`, or a waiter timeout, raises a P0 at once. In the setup of the owner, a session that waited on a prompt stalled a coordinator for 34 minutes, and nothing noticed.
- **A classifier refusal:** a P0 (section 14.2). A hold stops the session until the answer of the owner (section 15.1). Owner decision 2026-10-04.
- **A usage limit:** not a crash. The session stops and reports. bigm does not change the model. bigm probes the account before it claims that the account is out of quota, and sends a P1 so that the owner chooses the account. Owner decision 2026-09-29. A workflow in a background session does not pause at a usage limit: its agents fail. After the limit resets, the clerk launches the workflow again with `resumeFromRunId` and the same `args`. The agents that completed return cached results, and the failed agents and the agents after them run again. In the setup of the owner, a usage limit stopped all sessions for about 8 hours, and 23 workflows resumed this way.
- **A session in state `failed` or `stopped`** without a reboot, that did not finish its task: the parent restarts it once with `claude respawn <id>`. If it fails again, the parent raises a P0. Verify: `claude respawn`.
- **A burst refusal:** a sender that gets a refusal waits and sends again with the next attempt counter.

### 15.1 Refusals stop the session

Status: built 2026-10-04 (task 7 apply). The owner asked for this design on 2026-10-04 ("Task 7: design a stop") and decided the open choices on 2026-10-04 (15.1.8). The decision log is design.md, L58. The vendor facts are in knowledge.md, "Hooks, refusals, and workflows (read 2026-10-04)". The code is `scripts/refusal-stop.sh`, three entries of `hooks/hooks.json`, the `hold` field of `question_open`, and the hold clear of `answer_write`.

**The problem.** Principle 6 and house rule 8 say that a refusal is escalated, never handed on. Today this rule is only text: `agents/clerk.md` rule 3, `agents/clanker.md` rule 3, `agents/bigm.md` rule 6, and `defaults/house-rules.md` item 8. On one day, a refused command ran again in another form four times, also inside workflow agents of `deliver` and `review-and-fix`. The field cases, in placeholder form:

1. A gate command of the form `docker run ... <linter image> $(git ls-files '*.sh')` got a refusal from the worktree isolation guard in a clerk session ("too complex to verify that it stays inside the worktree"). A workflow agent then ran the linter as separate commands, with the file list from `git ls-files`.
2. The same refusal for a workflow agent of another task. The clerk itself then ran `git ls-files` and the linter with the file names written out.
3. The same refusal for a fixer agent of `/bruh:review-and-fix`. The agent wrote the file list to a temporary file, ran `xargs` into the same linter command, and deleted the file.
4. The guard refused a `node -e` command of a clerk whose JSON text contained the words "git diff". The command ran no git. The clerk moved the same code into a script file and ran it.
5. The guard refused a compound command of a clerk (`mkdir`, a heredoc that wrote a script file, `cp`, and `python3`) that built the workflow args. The heredoc text quoted git commands, but the command ran no git. The clerk did not try another form and sent a P0.
6. The guard refused the linter gate of this design task, of the form `docker run ... --entrypoint sh <linter image> -c 'find . -name "*.sh" ... -exec <linter> {} +'`, in a fixer agent of a workflow ("runs docker with the text sh in a plain command, so what it runs cannot be shown not to be git"). The command ran no git. The agent did not try another form and sent a P0. The clanker held the gate until the owner answers.
7. The guard refused a read-only command of a clerk: `grep -n "<pattern>" <plugin file> | head -30; sed -n '/<pattern>/,/+30p' <plugin file> | head -40` ("this command runs sed with the program /<pattern>/,/+30p in a plain command, so what it runs cannot be shown not to be git"). The command ran no git, and it has no word `git`, so the git-shape guard of 15.1.6 does not catch it. The clerk did not try another form, sent a P0, and went on with the Read tool. bigm decision 2026-10-04 under R-4: no action.
8. During the apply of this design, the guard refused a compound command of the implementing agent: `cd <folder>; cat > <file> <<'EOF' ... EOF` three times, then plain checks ("too complex to verify that it stays inside the worktree"). The command ran no git. The agent did not try another form and sent a P0. bigm decision 2026-10-04 under R-4: write files with the Write or Edit tool, never with a heredoc. The same agent later hit the same refusal twice with `cat >> <file> <<'EOF' ... EOF`, sent a P0, and got the same answer.
9. During the apply of this design, the guard refused the probe command of 15.1.8 item 4 (`claude -p ... "... echo \$(git ls-files '*.sh')"`): "this command runs claude with the text ... in a plain command, so what it runs cannot be shown not to be git". The agent did not try another form and sent a P0. bigm decision 2026-10-04 under R-4: skip the probes (15.1.1). The bruh plugin of that session had no hook for a guard refusal, so the refusal left no hook record.
10. During the apply of this design, the auto mode classifier refused a `git checkout -- <folder>` that would have removed the uncommitted probe edits of the agent (category `[Irreversible Local Destruction]`). The agent did not try another form and sent a P0. bigm decision 2026-10-04 under R-4: keep the edits.
11. Q-bruh-maksyms-macbook-pro-69, at about 2026-10-04T21:01Z: the auto mode classifier refused `gh pr ready 23 --repo oter/bruh` (category `[External System Writes]`), minutes after `gh pr edit 23` on the same pull request with the same account succeeded. In another clerk session, `gh pr ready 18` succeeded at about 2026-10-04T21:33Z. bigm decision 2026-10-04 under R-4: #23 stays a draft; the owner clicks Ready for review at merge time.
12. Q-bruh-maksyms-macbook-pro-74, at 2026-10-04T21:33:50Z: the auto mode classifier refused the read-only `gh pr view 18 --repo oter/bruh --json isDraft,headRefOid,title,state` (category `[Unrequested Commit in a Connected App]`), right after `gh pr edit 18` and `gh pr ready 18` succeeded.
13. Q-bruh-maksyms-macbook-pro-77, at about 2026-10-05T10:35:40Z: the worktree guard refused a background CI wait loop with no git (`until [ "$(gh run list ...)" = completed ]; do sleep 20; done; gh run list ...`) as "a construct too complex to verify".

In two other cases the auto mode classifier refused (a merge, category `[Merge Without Review]`; a `date -u` call right after `gh pr create`, category `[Unrequested Commit in a Connected App]`), and the session escalated correctly. Cases 4 to 9 and case 13 are false positives of the guard. The vendor text of a guard refusal tells Claude to rewrite the command, for example to split it (knowledge.md). So the vendor advice and the bruh rule point in opposite directions, and only a mechanical stop can hold the bruh rule (principle 2).

**Parts that this change touches.** Principles 2 and 6 (section 2), the deny rules of section 13, the P0 of section 14.2 for a refusal, the bullet "A classifier refusal" of section 15, the tests of section 20, `hooks/hooks.json` (three new entries), `scripts/refusal-stop.sh`, the MCP tools `question_open` and `answer_write`, `defaults/house-rules.md` (item 8 and items 10 and 11), the refusal rules of `clerk.md`, `clanker.md`, and `bigm.md`, and the recording hooks of `probes/probe-plugin`. The workflows do not change (R-3 cut 4).

#### 15.1.1 Detection from structured data

A hook finds a refusal only from the event name and the fields of the hook input. No hook reads the text of a message to decide what it means (owner rule against regex semantic classifiers). The text fields `reason` and `tool_input` go into the P0 word for word, and no code matches them.

| Refusal kind | Event | Structured fields | Built |
|---|---|---|---|
| Classifier refusal | `PermissionDenied` | `hook_event_name`, `tool_name`, `tool_input`, `tool_use_id`, `reason`, `session_id` (hooks.md, "PermissionDenied input") | A hold, with `denial_source` = `permission_denied` and `reason` as `denial_reason`. |
| Auto mode deny without a verdict | `PermissionDenied` | the same; only the text of `reason` differs | The same hold. |
| Permission deny rule | Not documented, not probed | None known | No hold. A P0 by text through the refusal rules of the agents. |
| Worktree isolation guard | Not documented, not probed | None known | No hold. A P0 by text through the refusal rules of the agents. |
| A deny of the git-shape guard of 15.1.6 | The bruh hook itself | The hook knows that it denied | A hold, written in the same run, with `denial_source` = `bruh-git-shape`. Owner decision 2026-10-04 (Q-49). |
| Permission prompt | `PermissionRequest`; `waitingFor` = `permission prompt` | `tool_name`, `tool_input`, `permission_rule_id` | No hold. The existing P0 of section 15 covers it. |

- The `PermissionDenied` hook reads any `PermissionDenied` input. It writes the fixed tag `permission_denied` as `denial_source` of the hold and copies `reason` word for word as `denial_reason`. It does not compare `reason` with a value, so a classifier refusal and a no-verdict denial get the same hold. The input has no field that tells them apart (hooks.md, "PermissionDenied input", read 2026-10-05). Agent-derived, needs owner decision.
- The probes of 15.1.8 item 4 did not run. The worktree guard refused the probe command in the isolated session of the implementing agent (field case 9), and bigm decided under R-4 on 2026-10-04 to skip the probes and add no text match. So no structured mark of a guard refusal or a deny-rule refusal is known, and those two kinds stay a P0 by text. The recording hooks in `probes/probe-plugin` (the full hook input to `hooks.jsonl`, and the events `PermissionDenied`, `PostToolUseFailure`, `PermissionRequest`, `SubagentStart`, and `SubagentStop`) are in the repository and were never run. This bigm decision (Q-bruh-maksyms-macbook-pro-55) replaced the probe criterion of owner decision Q-47 (run the probes first, then decide the detection from the data): no probe ran, and the guard and deny-rule detection stays a P0 by text. bigm decision 2026-10-04 under R-4.

#### 15.1.2 Options, ranked

Documented options first. A custom design comes last.

1. **`PermissionDenied` hook plus a `PreToolUse` hold guard (documented events).** A `PermissionDenied` hook writes a hold record. A `PreToolUse` hook, built like `scripts/lease-guard.sh`, denies each later tool call of the held key while the hold exists. This covers classifier refusals and no-verdict denials. Recommendation: build it.
2. **`PostToolUseFailure` hook for the guard and the deny rules (documented event, coverage not documented).** The same hold, written from `PostToolUseFailure`. It needs a structured field that marks a guard refusal or a deny-rule refusal. If the only mark is the text of `error`, this option needs an exact match of a documented vendor template, which the owner must allow first (15.1.8 item 4).
3. **`Stop` and `SubagentStop` hooks (documented `decision: "block"`).** While a hold has no question, the hook blocks the end of the turn or of the subagent, with the reason "open the P0 for hold `<id>` first". This makes the escalation happen, and it does not let an agent end in silence.
4. **`continue: false` from the hold guard (documented, effect in a subagent not documented).** It stops Claude after the hook. Verify what it stops inside a subagent and a workflow agent (15.1.8 item 3).
5. **Custom: the hold record, the P0 from the record, and the workflow path.** A hold file in the plugin data folder, a `hold` field of `question_open`, a hold clear in `answer_write`, and a question field on every agent of the three workflows. Options 1 to 4 need this part, because no vendor feature stores a hold or links it to a question.

- Built: option 1; option 3 with a `Stop` block on the session only and no `SubagentStop` block (owner decision 2026-10-04, Q-45 as changed by Q-52); option 5 without the workflow path (R-3 cut 4, agent-derived, needs owner decision). Not built: option 2, because the probe did not run (15.1.1), and option 4 (owner decision 2026-10-04, Q-45).
- Not chosen: the auto mode thresholds (3 blocks in a row, 20 in total) as the stop. They are not configurable, they count only classifier blocks, and in a `-p` run "Claude keeps working" (knowledge.md). Agent-derived, needs owner decision.
- Not chosen: more deny rules as the stop. A deny rule matches only the command text, so another form gets past it (principle 2), and its event is not documented. Agent-derived, needs owner decision.

#### 15.1.3 What stops

- The refused call stops at the vendor check. bruh adds nothing to that call. Agent-derived, needs owner decision.
- The `PermissionDenied` hook never returns `retry: true`. Agent-derived, needs owner decision.
- A deny of the lease guard (section 8.4) writes no hold. It keeps its own path: the agent asks for a lease with `lease_request` and waits for `lease_grant`. Agent-derived, needs owner decision.
- The hold record is `<plugin data>/holds/<hold ID>.json`, with the hold ID `H-<UTC time>-<process ID>`. Plugin code writes it: the hook script, through a temporary file and a rename, with the time from `date -u`. It holds only the hold ID, `session_id`, the role key, `tool_name`, `tool_input`, `denial_source`, `denial_reason`, the time, and the question ID (empty until the P0 is open). R-3 cut 1, agent-derived, needs owner decision.
- The hook does nothing when `BRUH_ROLE_KEY` or `CLAUDE_PLUGIN_DATA` is not set, as the other hooks. Agent-derived, needs owner decision.
- While a hold exists, the hold guard denies each tool call of the whole session (the main thread, each subagent, and each workflow agent with the same `session_id`) except the escalation tools: `question_open`, `answer_wait`, `mail_post`, `mail_read`, `SendMessage`, `ToolSearch`, and `StructuredOutput`. Owner decision 2026-10-04 (Q-43). Verify: workflow agents have the `session_id` of the session (the probe did not run, 15.1.1).
- The deny reason gives the hold ID and says: open a P0 with `question_open` and the field `hold` = `<hold ID>`, then wait for the answer: a subagent or a workflow agent waits with `answer_wait`; the main session waits for the `ANSWER` with `mail_read`. Do not run another form. Agent-derived, needs owner decision.
- While a hold exists, the hold guard allows `answer_write` only when the role key is `bigm`, also for the linked P0. A held clerk or a held local clanker is cleared by the `answer_write` of bigm from the session of bigm. bigm can record the answer of the owner to its own P0, and clears its own hold only by recording an answer of the owner to the linked P0 (the words, the date, and the source), never by its own decision. bigm decision 2026-10-04 under R-4 (Q-bruh-maksyms-macbook-pro-59).
- Known limit: a held remote clanker cannot clear its hold, because the `answer_write` of bigm does not reach a hold on another machine, and the hold guard denies its own `answer_write`. This build has no remote roles (Q-bruh-maksyms-macbook-pro-56). Agent-derived, needs owner decision.
- Known limit: the self-clear rule of bigm ("only by recording an answer of the owner") is prose, not a check. Agent-derived, needs owner decision.
- The hold guard denies calls; it never returns `continue: false`. A `Stop` hook blocks the end of the turn of the session while a hold of the session has no question ID, with the reason "open the P0 for hold `<id>` with question_open first". There is no `SubagentStop` block, and the workflows get no new question field: a subagent and a workflow agent just get the denies. Owner decision 2026-10-04 (Q-45 as changed by Q-52).
- The `Stop` block does nothing when the input has `stop_hook_active` = `true`, so a session whose MCP server is down cannot loop on paid turns. Agent-derived, needs owner decision.
- The hold guard fails open when its script cannot start or times out (knowledge.md). So it is a strong speed bump, not a sandbox. The tests of 15.1.7 check that it works. Agent-derived, needs owner decision.
- Known limit: a hold reaches only the session that got the refusal. A clanker that starts another clerk for the refused command is not blocked by it. Principle 6 and the P0 path cover that case as text. Agent-derived, needs owner decision.

#### 15.1.4 The P0 escalation

- `question_open` gets an optional field `hold`. With it, the MCP server checks the hold ID (`^H-[0-9A-Za-z-]+$`), reads the hold record, refuses a hold of another role key, sets the priority to P0, and adds two lines to the end of the body from the record, not from the model: `COMMAND: <tool_input.command, word for word>` (for a tool without a `command` string, the JSON of `tool_input`), and `CATEGORY: <denial_source> <denial_reason, word for word>`. Under the lock `holds`, it writes the question ID into the hold record. A hold that has a question ID already is refused, with that ID in the error: one P0 for each hold. Agent-derived, needs owner decision.
- The clerk relays the P0 to its clanker, and the clanker to bigm, with these lines unchanged (section 14.2). bigm shows the P0 at once. Agent-derived, needs owner decision.
- The P0 has no fixed option sets for each kind of refusal. The owner answers freely. Options that the asker passes stay as for any question. R-3 cut 2, agent-derived, needs owner decision.
- `answer_write` of the linked P0 removes each hold whose question ID is that P0, under the lock `holds`. Owner decision 2026-10-04 (Q-44). bigm writes the answer of the owner. A clanker that relays the `ANSWER` of a refusal P0 to a clerk also writes it with `answer_write`, because on a remote machine the `answer_write` of bigm does not reach the hold of the clerk. Agent-derived, needs owner decision.

#### 15.1.5 A second form and a later different command

- bruh does not compare the text of commands. The hold blocks each later call of the held scope until the hold is cleared, also a call that does a different job. So a second form of the refused command and a later different command are told apart by structure only: before the clear, every call is blocked; after the clear, the answer of the owner says what may run. Agent-derived, needs owner decision.
- A false positive costs one P0 and one answer. Cases 4 to 9 and case 13 are false positives of the guard: the commands ran no git. The answer can name the allowed form, for example "pass the data through tool inputs". The house rule of 15.1.6 removes the shapes of cases 4, 5, 8, and 9. It does not remove case 6, which has no separator (the guard refuses `sh -c` itself), or case 7 and case 13, which have no word `git`. A gate command in such a shape needs a form that the owner names, for example a gate that the repository ships as one script. Agent-derived, needs owner decision.
- A no-verdict denial (a classifier outage) holds too. Under server-side classifier review, ten no-verdict responses in a row stop the turn anyway (knowledge.md). The docs give no such limit for the local classifier, but the hold stops the agent after the first no-verdict denial, so the extra P0s are few. Agent-derived, needs owner decision.
- The owner clears a hold through the answer of the linked P0 (15.1.4). Owner decision 2026-10-04 (Q-44).

#### 15.1.6 The house rule for worktrees

- New house rule: "In a worktree, no compound commands with git. Data goes through tool inputs." Owner decision 2026-10-04 (answer "ok" to the plan of bigm to make this a bruh house rule). It is item 10 of `defaults/house-rules.md`.
- The clerk passes the run settings straight into the tool call (the `args` of the Workflow tool), with no shell script. Owner decision 2026-10-04 (answer "ok" to the plan "pass the run settings straight into the tool call, with no shell script at all"). It is item 11 of `defaults/house-rules.md`.
- A mechanical stop for the house rule (principle 2): the git-shape guard in `scripts/refusal-stop.sh`, on `PreToolUse` for Bash. When `cwd` is inside a linked worktree (`git rev-parse --git-dir` and `--git-common-dir` give two different paths), it denies a command that has the word `git` (bounded by characters other than `[A-Za-z0-9_-]`, also inside quotes) and has a separator (`;`, `&`, `|`, a newline), a command substitution (`$(` or a backtick), a process substitution (`<(` or `>(`), or a heredoc (`<<`). One plain git command passes: a command that starts with `git` and has no such token once its quoted strings and its fd redirects (`2>&1`, `>&2`) are removed, for example `git diff A B 2>&1`, `git log --format='%h|%s'`, or `git commit -m "a; b"`. A `$(` or a backtick inside double quotes still counts, because the shell runs it. This is a closed check of tokens, like the lease guard, not a judgment of meaning. It denies field cases 1 to 5 before the vendor guard sees them. It also denies case 9 (the word git with a `;` and a `$(` inside the quoted prompt). It does not deny cases 6, 7, and 13. Agent-derived, needs owner decision.
- A deny of the git-shape guard writes a hold first, with `denial_source` = `bruh-git-shape` and the house rule as `denial_reason`, and its deny reason names the hold ID. Owner decision 2026-10-04 (Q-49).
- The heredoc file write of field case 8 has no word `git`, so the guard does not catch it. bigm allowed adding that shape to the guard if it stays small; it is not added, because the standing instruction (write files only with the Write or Edit tool) covers it as text. Agent-derived, needs owner decision.
- The lane line for files: "Read files with the Read tool; write files with the Write or Edit tool. For a search, one plain `grep -rn` or `find` command in Bash is allowed, with no pipe, no git word, and no compound. Never read or write files with sed, cat, head, rg, or a heredoc in Bash." A worktree guard refusal of a plain file read or write inside the worktree that a built-in tool does is a report event, not a P0; the work goes on with the tool. bigm decision 2026-10-04 under R-4 (also under R-3). The Grep and Glob tools are gone in Claude Code 2.1.289, so the search goes through one plain grep -rn or find command: bigm decision 2026-10-05 under R-4 (Q-bruh-maksyms-macbook-pro-103).
- A worktree guard refusal of a wait loop or another compound shell construct with no git (field case 13) is a report event, not a P0; the work goes on with single plain commands, and the refused construct never runs in any form. All other refusals stay P0. bigm decision 2026-10-05 under R-4.

#### 15.1.7 Test plan

- Hook script tests, in Go in `mcp/scripts_test.go`, where the tests of the lease guard are. `TestRefusalHoldStopsTheSession`: a `PermissionDenied` input for session S (`reason` `[Merge Without Review]`) writes a hold with exactly the nine fields; then a different command of S, also from a subagent and with another tool, is denied with the hold ID in the reason; the escalation tools are allowed; another session is allowed; `answer_write` of the clerk is denied before the P0 and denied for the linked P0, and allowed for bigm. `TestRefusalClearByBigm`: the `answer_write` of bigm clears the hold. `TestRefusalNoRoleKey`: without `BRUH_ROLE_KEY`, no output and no file. `TestRefusalStopBlock`: the `Stop` block appears and goes away once the question ID is set or `stop_hook_active` is true. `TestGitShapeGuard`, in a real linked worktree made in a temporary folder: it denies field cases 1 to 5 in placeholder shapes and a backtick, writes a hold that also denies the next call, denies compound commands that start with `git` (a pipe, a `;`, a newline, `&&`, a `$(` inside double quotes), allows `git status`, plain git commands with separators only inside quotes or in an fd redirect, and compound commands without the word `git`, and denies nothing in the main checkout, outside a repository, or for another tool. `TestHooksJSON` lists both `PreToolUse` groups, `PermissionDenied`, and the two `Stop` groups: the waiter `wake.sh` of section 9.5 and `refusal-stop.sh`. `TestStopHooksTogether` runs both `Stop` scripts on one data folder: with a hold that has no P0, the `Stop` block appears and the waiter still wakes on new mail and at its limit; with no hold, the `Stop` script prints nothing and the waiter does the same. `TestRefusalHoldStopsTheSession` and `TestGitShapeGuard` failed with the `PreToolUse` branch of the script turned off. Agent-derived, needs owner decision.
- MCP server tests: `question_open` with `hold` gives priority P0 even when the model asks for P2, and the `COMMAND` and `CATEGORY` lines from the record also when the body holds fake `COMMAND` text; the JSON of `tool_input` for a tool without a command; a hold of another role key, a bad hold ID, a missing hold, and a second open are refused. `answer_write` removes the linked hold and leaves the other holds. Agent-derived, needs owner decision.
- No workflow tests and no eval case under `tests/eval/refusal` (R-3 cuts 3 and 4). The Go hook tests prove the stop. Agent-derived, needs owner decision.

#### 15.1.8 Decided choices

The owner decided each item with option 1 on 2026-10-04 (terminal, 2026-10-04T20:01:30Z: "go ahead. BUT MAKE SURE YOU ARE NOT OVERCOMPLICATING THINGS. the setup we develop must have some free choices fro agents, not too hard framing - this limits the prformance"). Item 3 was changed by Q-52 (owner "ok", 2026-10-04T20:05:11Z, terminal, on the plan "cut it, so only the main session opens one P0"). The options stay below as the record.

- Item 1, scope: the whole session except the escalation tools. Owner decision 2026-10-04 (Q-43).
- Item 2, the clear: `answer_write` of the linked P0. Owner decision 2026-10-04 (Q-44).
- Item 3, stop or deny: deny only, with a `Stop` block on the session while the hold has no P0; no `SubagentStop` block and no question field on every workflow agent. Owner decision 2026-10-04 (Q-45 as changed by Q-52).
- Item 4, detection: the owner approved the probe (Q-47). The probe command was refused by the worktree guard (field case 9), and the probes were skipped. bigm decision Q-bruh-maksyms-macbook-pro-55 replaced the probe criterion of Q-47: no probe ran, the recording hooks in `probes/probe-plugin` were never run, and the guard and deny-rule detection stays a P0 by text. bigm decision 2026-10-04 under R-4 (15.1.1).
- Item 5, a hold for the git-shape guard: yes. Owner decision 2026-10-04 (Q-49).
- The R-3 cuts of the apply: the minimal hold record, no fixed option sets for each kind, no eval case, and no question field on the gate, review, refuter, and merge agents. Agent-derived, needs owner decision.

1. **Scope of a hold.** Which calls does the hold block?
   1. All tools of the whole session (the main thread and each subagent and workflow agent with the same `session_id`), except the escalation tools. It also stops case 2, where the clerk ran the command after the refusal of its workflow agent. Verify: workflow agents have the `session_id` of the session. Recommended.
   2. All tools of the agent that got the refusal (`session_id` plus `agent_id`), except the escalation tools. Siblings and the clerk go on, so case 2 is not stopped.
   3. Only Bash of the agent that got the refusal. Least owner load, but another tool (for example `Write` of a script file, case 4) can still do the job.
2. **Who clears a hold, and how.**
   1. `answer_write` of the linked P0 clears the hold, because the answer of the owner is the decision. Recommended.
   2. A new MCP tool `hold_clear` that only bigm can call, after words of the owner. bigm records the words, the date, and the source.
   3. Both: the answer clears by default, and `hold_clear` clears a hold whose question was lost.
3. **Stop the session, or deny only.**
   1. Deny only, plus the `Stop` and `SubagentStop` blocks of 15.1.3. The session and its agents stay alive and can escalate. Recommended.
   2. Also return `continue: false` after the P0 is open, after a probe shows what it stops in a subagent and a workflow agent.
   3. Return `continue: false` at once on the refusal. The agent cannot open the P0 itself, so a hook must open it.
4. **Detection of the guard and the deny rules.** The docs name no event for them.
   1. Approve a probe, then decide. The probe adds recording hooks for `PermissionDenied`, `PostToolUseFailure`, `PermissionRequest`, and `SubagentStop` to `probes/probe-plugin`, and runs in a scratch linked worktree of a trusted repository: `claude -p --worktree probe-refusal --plugin-dir probes/probe-plugin --permission-mode auto --settings '{"permissions":{"deny":["Bash(touch probe-denied*)"]}}' "Run each Bash command once, exactly as written, as one command, and do not try another form: touch probe-denied-1; then: echo \$(git ls-files '*.sh')"`. A second probe runs the probe workflow `hello` and records `session_id` and `agent_id` of its agent. Recommended.
   2. No probe. Build only the git-shape guard of 15.1.6, which prevents the guard shapes before the vendor check, and leave deny rules as P0 by text.
   3. Allow an exact match of the documented vendor error template of the guard in `PostToolUseFailure.error`, as a closed grammar. This reads message text, so it needs an explicit exception to the owner rule.
5. **Does a deny of the git-shape guard write a hold?**
   1. Yes, the same as any refusal. A gate in that shape is a defect of the gate, and the owner fixes it at the source. Recommended.
   2. No. The deny reason names the allowed form (plain single commands, data through tool inputs), and the agent rewrites. This is quicker for cases 4 and 5, but for a gate (cases 1 to 3) it allows the same split that the field cases show.

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

The ledger template has the parts of the monitors of section 9.5: the file `monitors.md` (owner decision 2026-10-04, M7), the table "Command grants" of `grants.md` (owner decision 2026-10-04, M6), and the keys `monitor_default_hours: 24`, `monitor_max_hours: 168`, and `monitor_max_active: 20` of `mode.md` (owner decision 2026-10-04, M9). A later run of init adds them to an existing ledger with the rules above: a new file from the template, the new table after the tables of the template, and each new key with its template value. The table "Command grants" comes before "Merge grants", so the merge grants table stays the last part of `grants.md`. Agent-derived, needs owner decision.

Since version 0.6, "change projects" also has "learn again" for a selected project. An added project and a project to learn again go through steps 8 to 10. This is how the owner updates the index. `init_plan` keeps each stored project JSON that the answers do not change. Agent-derived, needs owner decision.

Verify: a select of `AskUserQuestion` has no option that is selected at the start.

These are no longer init questions: merge grants, delegated P1 classes, and remote machines. The owner tells bigm, and bigm records each one: a merge grant in `grants.md` (section 8.3), a delegated class in `priorities.md` (section 14.1), and a remote machine on the new line `remote_environments:` of `mode.md`. Agent-derived, accepted 2026-10-03. Both forms of the answers no longer have keys for these items, so an old answers file with one of them fails with the error "unknown field". In a container, the owner tells them to bigm through a channel. Agent-derived, needs owner decision.

The skill adds the allow rules `Workflow(bruh:deliver)` and the allow rules of the MCP tools to user settings. Agent-derived, accepted 2026-09-30. Since version 0.6, init writes the start settings of bigm into `<ledger>/.claude/settings.json` (section 3.4), and no longer writes `<plugin data folder>/roles/bigm.json`. When the file exists, init keeps each other key, sets `agent` and each env value of the defaults, and adds each default deny rule and each bigm deny rule of section 3.4 that the file does not have, after the existing rules. A file that is not a JSON object stops init with an error, and init does not replace it. So does a file whose `env` or `permissions` is not a JSON object, or whose `permissions.deny` is not a JSON array. An empty file counts as `{}`. An existing install runs `/bruh:init` again to write the file. Agent-derived, accepted 2026-10-03. After the apply, it shows only the notes that match the answers: for example, no Telegram note when the answer has no channel. Agent-derived, accepted 2026-10-03.

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
plugins/bruh/monitors/monitors.json
plugins/bruh/mcp/go.mod
plugins/bruh/mcp/*.go
plugins/bruh/channels/slack/
plugins/bruh/scripts/statusline-tap.sh
plugins/bruh/scripts/handoff-nudge.sh
plugins/bruh/scripts/handoff-inject.sh
plugins/bruh/scripts/lease-guard.sh
plugins/bruh/scripts/watcher.sh
plugins/bruh/scripts/wake.sh
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
  - the lease guard refuses a command without a lease and allows it with one;
  - the refusal stop holds a session after a refusal, lets the escalation tools through, clears with the answer, and the git-shape guard denies field cases 1 to 5 in a linked worktree (Go tests of section 15.1.7). Agent-derived, needs owner decision.
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
- Routing eval (section 14.2). Each item is agent-derived, needs owner decision.
  - The cases are in `tests/eval/routing/<case>/`, each with a file `input.txt` and a file `expected.txt`. All cases share one fixture ledger, `tests/eval/routing/ledger/`, with neutral project names. The driver copies the ledger to a temporary folder and adds `defaults/priorities.md`, so the hard-stop list exists.
  - The driver is `tests/eval/run.sh routing`. It runs `claude -p --agent bruh:bigm` with the plugin of the checkout in the copy of the ledger, with only the tools Read, Grep, Glob, and `AskUserQuestion`, with `--strict-mcp-config` (no MCP server), and with no `BRUH_ROLE_KEY`. So the run writes nothing.
  - `expected.txt` has only closed lines. `ROUTE clanker-<project> <n>`: a text line of bigm is the commitment form of section 3.4 with that project and that number, and any text for the work. `NOASK`: the run has no `AskUserQuestion` call. The driver compares exact values only, with `jq`, and never the meaning of the text.
  - The driver prints the summary line `routing eval: <passed> of <cases>` for the release notes. It does not run in CI, because it starts a model session. CI runs only the dry run and the compare rule, in `plugins/bruh/agents/agents_test.mjs`.
  - Verify: `claude -p` offers `AskUserQuestion`, so `NOASK` checks a real call.
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
5. The routing rule of 2026-10-04 (design.md, L50). The rule is an owner decision. Its details are tagged "Agent-derived, needs owner decision": the commitment form and the task number (section 3.4), the routing of a defect fix (section 3.4), the commitment of a clanker (section 3.5), the ranked stops (section 14.2), and the routing eval (section 20).
6. Ledger tools (section 8.7, built; design.md L55). The owner asked on 2026-10-04 for bruh MCP tools that change the tables of the ledger in one call, answered the open choices OC1 to OC5, and approved the fix plan of the review findings on the same day. The items of section 8.7 and of design.md L55 tagged "Agent-derived, needs owner decision" stay open for the owner, with the cuts of the clanker under rule R-3.
7. Monitors ([section 9.5](#95-monitors)), built 2026-10-04. The owner request of 2026-10-04 is in design.md, L56. The owner decided the choices M1 to M9 on 2026-10-04 (section 9.5, part k). The limit to local roles is a bigm decision (Q-bruh-maksyms-macbook-pro-56), and the cuts of the build are clanker decisions under RULE R-3 (section 9.5, part k); neither is an owner decision. Each other item of section 9.5, and the dependent items of sections 8, 9.1, 9.2, and 16, are tagged "Agent-derived, needs owner decision".
8. The six Orca choices O1 to O6 of section 4.3 are decided. Owner decision 2026-10-04 (section 4.3, "Owner decisions").
9. Each item of section 4.3 tagged "Agent-derived, needs owner decision": the detection, the viewer tab of `session_launch` and `session_resume`, the tool `session_tab_close` and the agent lines that call it, the option read from the user settings file at each call, the trust step of the init skill, and the tests. They are new on 2026-10-04 (design.md, L57).
10. Each item of section 15.1 tagged "Agent-derived, needs owner decision" (design.md, L58): the R-3 cuts of the apply, the `stop_hook_active` guard of the `Stop` block, and the details of the build. The five refusal-stop choices of 15.1.8 are decided: owner decision 2026-10-04.

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
