# bruh design (draft)

Status: brainstorming. A decision tagged "Owner decision" is approved. All other content is a draft.

## What the owner said (2026-09-27)

- bigm tracks work: what is done and what is in progress.
- bigm answers questions from the owner.
- bigm collects questions from other sessions and gives them to the owner.
- bigm does not hold the full context. It spreads the context across clankers.
- A clanker works in one project folder. A clanker knows the full picture of its project. A clanker does no hands-on work. A clanker gives work to a clerk.
- A clerk owns one task. A clerk starts workflows. The workflows do the work.
- Spawn chain: bigm starts clankers, a clanker starts clerks, a clerk starts workflows.
- Answer quality degrades at 50 to 60 percent of the context window. No session may go above that level.
- A clanker rotates its clerks. When a clerk reaches the limit before its work is done, the clerk writes a handoff, compacts, reads the handoff, and continues.
- bigm starts as many clankers as necessary. It does not rotate clankers. A clanker that reaches the limit writes a handoff, compacts, reads the handoff, and continues.
- bigm must also coordinate work on remote machines.
- The finished tool can be a public GitHub repository named `bruh`.
- The `bruh` skill must be installable into Claude Code.
- The `bruh` repository holds the skillset and the workflows.
- The top role is named bigm. It was the coordinator.
- Questions reach the owner in the bigm terminal.
- A clanker answers the questions that it can answer. Only P0 and P1 questions go to bigm.
- Local sessions talk through Claude Code itself. Remote sessions talk through Orca.
- Keep compaction in mind for the long-lived sessions.
- The README gives clear install instructions for each part.
- The skillset has its own init skill that asks the user questions. One question is how the skillset addresses the user.
- The owner wants pipelines, badges, artwork, topics, tags, and a repository that shows it is mature.

## Decisions

- Role names: bigm (was coordinator), clanker (was lead), clerk (was minion), workflow. Owner decision 2026-09-27.
- Question channel: questions reach the owner in the bigm terminal. Owner decision 2026-09-27.
- Workflow means the Claude Code `Workflow` tool. Owner decision 2026-09-27.
- Local transport: Claude Code itself, not Orca. Owner decision 2026-09-27. The mechanism is also an owner decision 2026-09-27 (the owner accepted it in chat): bigm or a clanker starts a session with `claude --bg --name <name>` in the project folder, reads state with `claude agents --json --all`, and sends and receives questions with `SendMessage`. A message is plain text, so it starts with its priority, for example `P1:`. A first draft used Orca for local sessions too; the owner rejected it the same day.
- Question routing: a clanker answers a question itself when it can. Only P0 and P1 questions go to bigm. Owner decision 2026-09-27.
- Question priorities: P0, P1, and P2 as defined in open question 5. Owner decision 2026-09-27. The owner expects to change them, so they are data, not skill text: one file that bigm reads and passes to each clanker in its task. Owner decision 2026-09-27 (the owner accepted it in chat): the file holds only the generic definitions, with no project data. The plugin ships the default definitions (public). The owner copy lives in the private ledger repository. The questions and their answers are project data and stay in the private ledger.
- Session lifetime: bigm and the clankers are long-lived and compact with the compaction mechanism below. A clerk lives for one task. Owner decision 2026-09-27.
- Context budget: every role stays below about 55 percent of its context window. Owner decision 2026-09-27.
- Compaction mechanism: option 2 of open question 1. Owner decision 2026-09-27. The parts: auto-compact at 55 percent; a live handoff file for each session; a `SessionStart` hook with matcher `compact` that injects the handoff file; a status line tap; a `PostToolUse` hook that tells the agent to write its handoff at 50 percent.
- Configuration scope: user settings (`~/.claude/settings.json`) on each machine. The auto-compact window, the status line tap, and both hooks apply to every Claude Code session of the user, manual sessions included. Owner decision 2026-09-27.
- Packaging: `oter/bruh` is a Claude Code plugin marketplace with one plugin, `bruh`, installed at user scope. The plugin ships the skills and both hooks. The README gives install instructions for each part. Owner decision 2026-09-27.
- Init skill: the plugin has an init skill that asks the user questions and writes the answers. It writes `autoCompactWindow` and the status line tap into user settings, because a plugin cannot set them. The first question is how the skillset addresses the user. Owner decision 2026-09-27. Owner decision 2026-09-27 (the owner accepted it in chat): the init skill stores the address as the plugin option `user_name` under `pluginConfigs`, and skill text uses `${user_config.user_name}`.
- Repositories: two. `oter/bruh` is public on GitHub and holds the tool. A separate private repository holds the ledger, because the ledger describes all other repositories. Owner decision 2026-09-27.
- Pipelines: `oter/bruh` gets GitHub Actions, because its home is GitHub. Owner decision 2026-09-27. Agent-derived, needs owner decision: the checks are Markdown lint, a link check, `claude plugin validate`, shellcheck for hook scripts, and tests for the status line tap and the hooks.
- Maturity: the goal is a repository that is mature on day 0, proven by what it contains, not by claims. Badges, artwork, topics, and semantic version tags with releases. A badge shows only a fact that CI or GitHub proves. Owner decision 2026-09-27.
- Ledger: Markdown in a separate private repository. Owner decision 2026-09-27.
- Remote transport: Orca remote runtime. `orca serve` on the remote machine on a private network, paired with `orca environment add`, workers started with `worker-start --on <environment>`. Owner decision 2026-09-27.

## Assumptions

- `autonomous-agents` solves a different problem: "runs LLM CLI agents on a schedule or a webhook, one Docker container per execution". It is not the transport for this design. It can be a later executor for unattended work. Agent-derived, needs owner decision.

## Owner decisions of 2026-09-29

- No pollution of the system: bruh keeps all its files in the plugin data folder `${CLAUDE_PLUGIN_DATA}` (`~/.claude/plugins/data/<plugin id>/`), which Claude Code deletes when the plugin is uninstalled. Owner decision 2026-09-29.
- Plugin code writes the files, not Claude: the plugin ships a small stdio MCP server, and Claude calls its tools. Claude Code guards `~/.claude` against writes by Claude, but not against writes by plugin code. Owner decision 2026-09-29 (option A).
- Dependents re-decided because of this premise. Each is agent-derived, needs owner decision:
  - Handoff files: were `$CLAUDE_JOB_DIR/tmp/handoff.md` and `handoffs/bigm.md` in the ledger. Now `${CLAUDE_PLUGIN_DATA}/handoffs/<role key>.md`, written by the MCP tool `handoff_write` (spec 0.3 moved the key from the session to the role key).
  - Answer files: were in `~/.local/state/bruh/answers/`. Now `${CLAUDE_PLUGIN_DATA}/answers/<clerk role key>/<question ID>.answer`, written by the MCP tool `answer_write`. A workflow agent waits with the MCP tool `answer_wait` instead of a Bash loop.
  - Context files of the status line tap: `${CLAUDE_PLUGIN_DATA}/context/<session_id>`, written by the tap script (plugin code). No change.
  - The only files outside the plugin data folder: the keys that the init skill writes into `~/.claude/settings.json` after the owner approves the diff, and the private ledger repository of the owner.

## Operating modes (owner, 2026-09-29)

- bruh runs in two modes: with a human, and fully autonomous. In autonomous mode, bigm is the main brain and acts on its own. Owner decision 2026-09-29.
- The mode switch is recorded in the repository that describes the behavior of the instance, which is the private ledger repository. Owner decision 2026-09-29.
- Agent-derived, needs owner decision: the switch is the file `mode.md` in the ledger repository, with one line `mode: human` or `mode: autonomous`, and the date and the reason of the last change. bigm reads it at the start of each turn. A clanker gets the mode in each message from bigm.
- Agent-derived, needs owner decision: in human mode, P0 and P1 go to the owner (the current design). In autonomous mode, bigm decides P1 questions itself, records each decision in the ledger with the tag "bigm decision <date>" and its reasons, and the owner can review and reverse it later.
- Decided later (spec open questions 8 and 14): in autonomous mode, what bigm does with a P0 question (security, data loss, money, an irreversible or outward-facing action). Options: bigm decides and acts; bigm parks the question and continues other work; bigm stops all work until the owner answers.
- Conflict to resolve: the global instructions of the owner say "In an autonomous run, write the options ranked, set the ticket needs-info and stop." That rule parks open questions. "bigm acts on its own" decides them. The owner must say which rule applies to bigm in autonomous mode.

## Answers to the spec open questions (owner, 2026-09-29)

- Q2: a clanker may answer P1 questions of delegated classes that the owner lists in `priorities.md`. It logs each answer, and the owner can reverse it. Owner decision 2026-09-29.
- Q3: the review-round cap is a runtime knob with default 2, set by the init skill. Owner decision 2026-09-29.
- Q5: read-only token reads inside a process are allowed. Moving or copying a credential is a P0. Owner decision 2026-09-29.
- Q6: per-agent identities come later. In version 0.1, a write under the personal identity of the owner is a P0. Owner decision 2026-09-29.
- Q8: the never-without-the-owner list is a hard stop in both modes. Owner decision 2026-09-29.
- Q9: on a usage limit, the session pauses and bigm sends a P1 so that the owner chooses the account. The model never changes. Support for several accounts comes later. Owner decision 2026-09-29.
- Q11: the license must allow modification, reuse, and commercial use. The owner delegated the choice to the agent on 2026-09-29. The agent chose Apache-2.0, because it adds an explicit patent grant to the permissions of MIT.
- Q4: every merge is a P1 to the owner, plus optional merge grants for each repository. A grant names one merger session and its conditions (for example: CI green and two review rounds passed). The owner gives it explicitly, bigm records it in the ledger, and the merger reports each merge after it confirms the merge through the code host API. A grant is an answer that the owner gives in advance, so the never-without-the-owner hard stop still holds. Owner decision 2026-09-29.
- Q7: leases are hierarchical. bigm keeps track of the leases of the clankers, and each clanker keeps track of the leases of its clerks. Owner decision 2026-09-29. The lease guard is in version 0.1 (Q16).
- Q10: local messages use the durable mailbox of the MCP server plus a `SendMessage` nudge. Remote messages use Orca `send` and `ask`. Owner decision 2026-09-30.
- Q12: clerks do all pushes. The ledger clerk `clerk-ledger` pushes the ledger after each commit of bigm. Owner decision 2026-09-30.
- MCP server language: Go, standard library only, started with `go run -C ${CLAUDE_PLUGIN_ROOT}/mcp .` and `GOTOOLCHAIN=local`, so no binaries are committed. The owner rejected Node (not on every machine) and `/tmp` for bruh files (a reboot wipes it, as in the setup of the owner). Owner decision 2026-09-30.
- Spec 0.4 approval: the owner accepted every item of spec 0.4 tagged agent-derived as a set, to test them in practice. Owner decision 2026-09-30.
- Q13: every role runs with `--permission-mode auto`. Owner decision 2026-09-30.
- Q14: in autonomous mode, bigm decides P1 questions itself and records each one as a reversible "bigm decision". The global rule "write the options ranked, set needs-info and stop" does not apply to bigm in autonomous mode. The never-without-the-owner list still stops it. Owner decision 2026-09-30.
- Q15: for autonomous work, the container image is `ghcr.io/oter/autonomous-agents/agent`. The image is infrastructure. The skillset must run both locally and in a container. Owner decision 2026-09-30.
- Q16: version 0.1 contains all parts. No scope cut. Owner decision 2026-09-30.
- Premise changed by Q16: the lease guard was proposed for version 0.2 (Q7 note). Now it is in version 0.1, as a speed bump, with a Verify item (spec 8.4).

## Channels and the container image (owner, 2026-09-29)

- P0 and P1 questions can go through a chat channel. bruh supports several channels. Slack and Telegram come first. Owner decision 2026-09-29.
- bruh is installed in the agentic Docker image. Owner decision 2026-09-29.
- Facts (<https://code.claude.com/docs/en/channels.md>): channels are a research preview. The official channel plugins are Telegram, Discord, and iMessage. A channel pushes chat messages into a running session that started with `--channels`. A channel can relay permission prompts, so a person can approve a prompt from the chat. Slack has no channel plugin: a Slack channel is a custom channel, and during the preview a custom channel loads only with `--dangerously-load-development-channels`. "Claude in Slack" starts cloud sessions from mentions and is a different product.
- Dependents re-decided because of the container premise. Each is agent-derived, needs owner decision:
  - Out of scope for version 0.1 was "`autonomous-agents` as an executor". Now the agentic image must contain bruh, so the install is in scope. Which image: spec open question 15, decided 2026-09-30.
  - The init skill asks questions in a chat. In an image build nobody answers, so init also needs a non-interactive form: the same answers from a file or environment variables.
  - A container filesystem can be ephemeral. The handoffs and answers in `${CLAUDE_PLUGIN_DATA}` die with the container, unless the plugin data folder is on a volume. The ledger survives because it is a git repository that bigm pushes.
  - bigm runs in an Orca terminal on the machine of the owner. In a container there is no Orca terminal, so remote clankers in containers need Orca `serve` in the container or another transport.
  - The Slack channel is custom code in bruh, and it needs the development flag while channels are a preview.

## Changes from the spec review (2026-09-27)

An adversarial review and an invariant check of the first spec draft found these problems. Each change below is agent-derived, needs owner decision.

- Roles as plugin agents: bigm, clanker, and clerk start with `--agent bruh:<role>`, because the system prompt reloads after compaction. The init skill stays a skill.
- Handoff location: the first draft used `${CLAUDE_PLUGIN_DATA}`, which is under the protected `.claude` directory, so every handoff write of an unattended role stalls or fails. A background role writes `$CLAUDE_JOB_DIR/tmp/handoff.md`. bigm writes `handoffs/bigm.md` in the ledger repository.
- Consent: a message from another session cannot approve a permission prompt. For a P0 that waits on a permission prompt, bigm gives the owner the command to attach to that session.
- Idle sessions: the supervisor stops an idle background session after about an hour, and it cannot receive messages then. Before a send, the sender checks `pid` in `claude agents --json` and resumes a stopped session with `claude --resume <session-id> --bg "<message>"`.
- Registration: a `claude` started from the Bash tool of another session can miss the `claude agents` registration. Every launch sets `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1`.
- One ledger writer: clankers write no files. A clanker sends P2 answers to bigm in its reports, and bigm records them.
- Merge ownership: a clerk delivers a branch or a pull request. A merge is P1 by default, set in `priorities.md`.
- Workflow input (corrected by the owner the same day): the review said that a workflow run takes no input, and the first fix split `plan` from `deliver`. The owner said that workflows can get input while they run. The docs confirm that each workflow agent keeps `SendMessage`. So `deliver` again contains the plan stage, and a workflow agent sends its questions to the clerk. The review round limit is two, an owner decision 2026-09-27.
- Handoff message in subagents: the `PostToolUse` hook exits when the input has `agent_id`, so subagent tool calls do not use up the message.

## Build of v0.1 (2026-09-30)

- Owner decision 2026-09-30: the owner accepted, as a set, the recommended option of each decision that plans 2 to 6 tagged "agent-derived, needs owner decision" (see the [plan index](superpowers/plans/2026-09-30-bruh-v0.1-index.md)). The owner said "go go go" to the push and the pull request of the build.

## Learn step and onboarding (owner, 2026-10-02)

The owner ran `/bruh:init` for the first time on 2026-10-02 and gave this feedback:

- F1: the plugin options in `/config` (`user_name`, `handoff_percent`, `max_busy_clerks`) do not show that they belong to bruh. The owner suggested a `bruh_` prefix. Open: see "Open items" below.
- F2: the 13 init questions do not tell what each answer controls.
- F3: the owner wants to answer questions with selects: the P1 questions of bigm and the init questions.
- F4: init has no step that finds the projects and lets the owner select the projects that bruh works on.

Premise changed: version 0.5 lets bigm learn about a project only from the first work request of the owner. The dependent decisions get a new decision in spec version 0.6: section 3.5 (a clanker knows the full picture of its project), section 8.1 (bigm makes the project file at the first work), section 16 (the init questions), the step "Work requests of the owner" of bigm (`repos_set` runs only when a clanker starts), step 3 of the start of the clanker (it reads the full project again each time), and the rules that bigm is the only writer of the ledger and that a clanker writes no file.

Decisions:

- L1: bruh learns the projects before the owner asks for work. A learn step makes the hierarchy and the knowledge. Owner decision 2026-10-02.
- L2: the hierarchy has three parts: the folder tree (workspace, group folder, project, repository), projects with more than one repository, and links between projects. Machines are not part of the hierarchy. Owner decision 2026-10-02.
- L3: the knowledge of each project has three parts: the repositories and their code hosts, the gates and the stack (language, build, test, and lint commands, CI), and the live status (open pull requests and issues, recent activity, deployments). The map of the docs and the rules is not part of the knowledge. Owner decision 2026-10-02.
- L4: init scans a root folder (default `~/workspace`), shows the tree of groups and repositories, and the owner selects the projects that bruh learns. A second run adds projects. Owner decision 2026-10-02.
- L5: the learn step proposes the projects of a group and the links between projects from structure only: a shared name prefix, dependency manifests, submodules, and container base images. It does not classify text by its meaning. The owner confirms or changes each proposal. Owner decision 2026-10-02.
- L6: the refresh depends on the cost. The MCP server refreshes the repositories, the code hosts, and the live status at each sweep of bigm, with no model call. The gates and the stack are learned again only when their source files change (for example a Makefile, a CI file, or a manifest), or when the owner runs the learn step again. Owner decision 2026-10-02.
- L7: approach A. Go code in the MCP server scans the root and proposes the tree. A `bruh:learn` workflow runs one agent for each confirmed project, which reads the gates and the stack and returns a fixed JSON shape. An MCP tool stores the result. bigm writes `workspace.md` and the knowledge section of each project file from the stored data, so bigm stays the only writer of the ledger. Owner decision 2026-10-02. Rejected: one learn clerk for each project (about 45 sessions, the busy clerk cap, and a trust dialog for each project folder), and bigm reads each project itself (context cost, no automatic refresh).
- L8: the learned data is JSON in the ledger: `learn/tree.json` for the hierarchy, and `learn/projects/<key>.json` for the knowledge of each project. MCP tools write these files, and bigm commits them. The data is versioned in git and stays after an uninstall of the plugin. Owner decision 2026-10-02. Rejected: JSON in the plugin data folder with a Markdown view in the ledger (an uninstall deletes the JSON).
  - Premise changed: bigm is no longer the only writer of the ledger. The MCP learn tools write the `learn/` folder. Re-decided (agent-derived, needs owner decision): bigm stays the only writer of each other ledger file, and the only role that commits. An MCP tool writes a file only when a value changes, not when only a check time changes, so that a sweep with no change makes no commit.
- L9: the project key is the proposed project name. When two groups have a project with the same name (for example `infra` in `hydra-ai/` and in `solostartups/`), the key gets the group name as a prefix (`hydra-ai-infra`). The owner can change each key when the owner confirms the tree. Agent-derived, needs owner decision.
- L10: the MCP tool `learn_scan` reads and writes nothing else: it finds each git repository under the root to a depth of 3, reads its remotes and its default branch, and removes the user information (a user name or a token) from each remote URL before it returns or stores the URL. It sets the host kind from an exact list of host names (`github.com` is `github`, `gitlab.com` is `gitlab`), and the owner sets the host kind of each other host when the owner confirms the tree. It proposes projects from a shared name prefix (`wishmateai` and `wishmateai-app`), and links from `go.mod`, `package.json`, `pubspec.yaml`, `.gitmodules`, and the `FROM` lines of a Dockerfile. Agent-derived, needs owner decision.
- L11: the owner picks the projects in two steps. Step 1 is a multi-select of the groups and the single repositories, with at most 4 options in a question and 4 questions on a screen (the limits of `AskUserQuestion`). Step 2 shows, for each selected group, the proposed projects and links as text, with a select "Accept" or "Edit". For "Edit", the owner types the change. Owner decision 2026-10-02. Rejected: a checklist file that the owner edits, and a numbered list in the chat.
- L12: init asks the 2 required questions first, then one select: "defaults for the rest" or "customize". Each question has one line that tells what the answer controls (F2), and each question with fixed answers is a select (F3). The merge grants, the delegated P1 classes, and the remote machines are not init questions: the owner adds them later in the ledger files. Agent-derived, needs owner decision.
- L13: the `bruh:learn` workflow runs inside `/bruh:init`, right after the pick, and the owner waits for it. The knowledge is complete when init ends. Owner decision 2026-10-02. Rejected: the workflow runs in bigm at its first start. Dependent items (agent-derived, needs owner decision):
  - The owner is present during init, so init writes no new read rule into the user settings. Verify: a workflow agent that an interactive session starts can read the root in `auto` mode, or shows its permission prompt to the owner. If neither is true, the init skill gets a read rule for the root that applies only while the skill runs.
  - bigm learns a stale project again at the sweep, so the role settings of bigm get the read-only allow rule `Read(//<root>/**)`. A global `additionalDirectories` entry is rejected, because it also allows writes to the root in every session.
  - The allow list that init writes gets `Workflow(bruh:learn)`.
- L14: refresh and use of the knowledge. Agent-derived, needs owner decision.
  - When the owner confirms the tree, init calls `repos_set` for each selected repository on a supported code host, and makes the project file of each selected project. Premise changed: `repos_set` ran only when bigm started a clanker, and the project file was made at the first work (spec 8.1). Both now happen at the confirm.
  - The watcher of spec 9.2 makes the live status. It already polls each repository of `repos_set`. It also reads the open pull requests, the open issues, and the latest release or deployment, and writes the live part of `learn/projects/<key>.json` only when a value changes. There is no second poller.
  - At each sweep, bigm calls `learn_refresh`. The tool reads the remotes and the default branch again from the local git data, and calculates the hashes of the cited gate files again. A changed hash marks the project as stale, and bigm runs `bruh:learn` for the stale projects only.
  - bigm writes `workspace.md` and the "Knowledge" section of each project file from the JSON, and commits.
  - A clanker starts from the JSON of its project and of the linked projects, and reads the code only for its task. Premise changed: step 3 of the start of the clanker read the full project each time.
- L15: for a project that has no clanker, the watcher only updates the live status. It sends events (pushes, replies, red pipelines, merges) to a report file only when the project has a clanker. Owner decision 2026-10-02. Rejected: events for each learned repository.
- L16 (F1): the title of each plugin option starts with "bruh: ", for example "bruh: Your name". The keys do not change. Owner decision 2026-10-02. Rejected: a `bruh_` prefix on the keys. Verify: a row of the `/config` panel shows the title of the option. If it shows the key, the decision falls back to the key prefix, and init moves the saved values to the new keys (agent-derived, needs owner decision).
- L17 (F3): `question_open` gets an optional `options` field: 2 to 4 options, each with a label and a description. bigm shows each P1 question that has options as a select with `AskUserQuestion`, and each question without options as text. `p1_batch_size` keeps its value. When a batch has more than 4 questions with options, bigm shows them on more than one screen, at most 4 questions on each screen. Owner decision 2026-10-02. Rejected: a maximum batch size of 4.
- L18: bruh reads GitLab through `glab api`, so that glab keeps the login and refreshes the OAuth token, in the same way that bruh uses `gh auth token` for GitHub. The owner said on 2026-10-02: "why you can read in gitlab??? glab logged in!". Owner decision 2026-10-02. The scope is the watcher and the live status only, with no merge train and no posts on GitLab: agent-derived, needs owner decision. Of the repositories of the owner, 15 are on `gitlab.com` and 1 is on a self-hosted GitLab.
- L19: the pick screen is interactive. It shows the repositories as multi-select questions, most active first. The activity is the last activity on the code host (GitLab `last_activity_at`, GitHub `pushed_at`, Gitea `updated_at`), not the last commit of the local clone: on 2026-10-02 the local clone of an active GitLab repository showed a commit date that was two months old. Owner words of 2026-10-02: "i asked you to prepare the list i can choose from" and "yo i expected that to be interactive!!!". Owner decision 2026-10-02. How this fits the two steps of L11 (groups first, then the projects of each group): agent-derived, needs owner decision.
- L20: the scan calls a code host only when the owner has a working login for that host (`gh auth status`, `glab auth status --hostname <host>`, or a `BRUH_GITEA_TOKEN_<HOST>` variable). For each other host, the scan makes no call and shows "no login" for the activity. On 2026-10-02 the agent called a self-hosted GitLab that the owner has no access to, and the owner did not ask for that call. Agent-derived, needs owner decision.

Open items (each is agent-derived, needs owner decision):

- The live status of a GitLab repository. `repos_set` accepts only `github` and `gitea`, and one repository of the owner is on GitLab.
- F1: a `bruh_` prefix on the option keys, or a "bruh: " prefix on the option titles. A key change also changes `${user_config.<key>}` in the agent files, the `CLAUDE_PLUGIN_OPTION_<KEY>` variables of the hooks, and the saved values in `pluginConfigs`.
- F3: `AskUserQuestion` takes at most 4 questions in one call. The default P1 batch size is 5.
- Verify: an agent of a workflow that bigm starts in the ledger folder can read a project folder outside the ledger folder without a permission prompt.

## Knowledge

The verified facts that these decisions depend on are in [knowledge.md](knowledge.md).

## Open questions

1. Compaction mechanism (decided 2026-09-27, option 2). Options, ranked:
   1. Documented only. Set the auto-compact window to 55 percent. Each session keeps a live handoff file and updates it at each checkpoint. A `SessionStart` hook with matcher `compact` injects the handoff file. No session monitors another session.
   2. Option 1, plus a status line tap. The status line writes `used_percentage` to a file per session. A `PostToolUse` hook reads the file and tells the agent to write its handoff at 50 percent.
   3. Custom supervisor. The status line tap from option 2. The supervisor reads the files and types `/compact` into the worker terminal with `orca terminal send`.
2. Configuration scope (decided 2026-09-27: user settings). Options were user settings, or `.claude/settings.local.json` in each project folder. The local file is ignored by git, so a new worktree does not have it.
3. Remote transport (decided 2026-09-27, option 1). Options, ranked:
   1. Orca remote runtime: `orca serve` on the remote machine, reachable over a private network, and `worker-start --on <environment>`. Traffic stays on that network. Questions and results use the Orca orchestration verbs `ask`, `reply`, and `worker_done`.
   2. Claude Code Remote Control: `SendMessage` to a Remote Control session on the other machine. Traffic goes through Anthropic servers. Plain text only, with no task or dispatch tracking.
   3. Custom socket bridge over a private network (SSH or socat forwards a remote inbox socket). Not recommended:
      - `SendMessage` refuses the forwarded socket with `connected endpoint is not the expected process`. Source: `errors.md`, "Refusing to send a cross-session message".
      - Only the auth line of the socket protocol is documented. The message line format is not documented, so the bridge depends on an internal format.
      - A reply address is a local socket path on the sender machine, so replies need a second bridge.
      - A message from a process that is not a child of the session goes through inbound controls. A session that bypasses permission prompts holds it for approval unless `crossSessionInbound` is `accept`.
      - Security: operating-system user permissions protect the socket. A TCP forward on the network lets any peer that reaches the port send prompts to the session.
4. Public `bruh` repository and the ledger (decided 2026-09-27, option 1). The ledger records work in all projects. If the ledger lives in a public repository, that record is public. Options, ranked:
   1. Two repositories. `oter/bruh` is public and holds the tool: docs, hooks, status line tap, skills. A separate private repository holds the ledger.
   2. One public repository `bruh`. The ledger stays outside git, on the local disk only. No history and no backup for the ledger.
   3. One public repository `bruh` with the ledger inside. All project status is public.
5. Question priorities (decided 2026-09-27: the definitions below, owner decision 2026-09-27; the owner expects to change them later):
   - P0: work is blocked and only the owner can unblock it. Examples: security, data loss, money, an irreversible or outward-facing action (publish, deploy, delete, send), a permission block. bigm shows a P0 question at once, at the top of its next reply.
   - P1: an owner decision. Examples: an open question, an "X versus Y" choice in a spec, a premise that changed under an owner decision, a scope change. bigm queues P1 questions and shows them as a batch.
   - P2: the clanker answers from the project context (code, docs, ADRs, the ledger) and records the answer. The owner sees it in the status, not as a question.
6. Inbound messages. Without a `crossSessionInbound` value, a session that bypasses permission prompts holds a message from a session that does not. If bigm, clankers, and clerks run in different permission modes, questions stall. Options: run all roles in one permission mode, or the init skill sets `crossSessionInbound: accept` in user settings. `accept` delivers every message from any session of the same operating-system user. Agent-derived, needs owner decision.
