# Knowledge

Verified facts about Claude Code and Orca that the design depends on. Each fact has a source. Verify a fact again before you change a decision that depends on it.

Verified on 2026-09-27 with Claude Code 2.1.280 and Orca 1.4.209.

## Context window and compaction

- `autoCompactWindow` (settings), `--autocompact` (CLI flag), and `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (environment variable) set how full the context gets before automatic compaction. The environment variable has the highest precedence, then the flag, then the setting. Source: <https://code.claude.com/docs/en/model-config.md>, "Set the auto-compact window".
- The value is a token count from 100K to 1M, not a percent. Claude Code caps it at the context window of the model. A value of 550000 gives 55 percent on a 1M model only. On a 200K model the cap makes the value equal to the full window. Source: same section.
- Without a setting, a 1M model compacts at about 967K tokens. Source: same page, "Sonnet 5 context window".
- `SessionStart` fires again after compaction, with `source` equal to `compact`. Its hook can return `hookSpecificOutput.additionalContext`. Source: <https://code.claude.com/docs/en/hooks.md>, SessionStart input and decision control.
- `PreCompact` has the matchers `manual` and `auto`. It can block a proactive automatic compaction. Claude Code discards its `systemMessage`, so it cannot instruct the agent. Source: hooks.md, PreCompact.
- `PostCompact` receives `compact_summary`. It has no decision control. Source: hooks.md, PostCompact.
- A `PostToolUse` hook can return `hookSpecificOutput.additionalContext` to give Claude a message after a tool call. Source: hooks.md, PostToolUse.
- Hook inputs do not contain context usage. Every hook input contains `session_id` and `transcript_path`. Source: hooks.md, common input fields.
- `SessionStart` matcher values are `startup`, `resume`, `clear`, `compact`, and `fork`. Source: hooks.md, matcher table.
- Plugin hooks also run inside subagents. `PostToolUse` then fires for the tool calls of the subagent, and the input contains `agent_id`. The main conversation has no `agent_id`. Source: hooks.md.
- Claude Code caps `additionalContext` at 10,000 characters. For a longer value, Claude gets a file path and a preview of the first 2,000 characters. Source: hooks.md.
- The status line input contains `context_window.used_percentage` and `session_id`. The `session_id` is stable for the life of a session. Source: <https://code.claude.com/docs/en/statusline.md>.
- `context_window.used_percentage` can be `null` early in a session. `context_window.current_usage` is `null` after `/compact` until the next API call. Source: statusline.md.
- The status line runs on events with a 300 ms debounce. It can go quiet while the session is idle. `refreshInterval` adds a timer. Source: statusline.md, "When it updates".
- No documented mechanism lets one session start compaction in another session.

## Cross-session messaging

- Cross-session messaging does not use TCP. Source for all items in this section: <https://code.claude.com/docs/en/cross-session-messaging.md>.
- On one machine, each session binds a Unix domain socket (a named pipe on Windows). Claude Code exports its path as `CLAUDE_CODE_MESSAGING_SOCKET` and restricts it to the operating-system user.
- To a session on another machine, a message goes through Anthropic servers over Remote Control. Both ends need a sign-in with a Claude account and Remote Control. Without Remote Control on the sender, the message has no reply address.
- Messages are plain text.
- Before a send on the same machine, Claude Code checks that the process that holds the target socket is the target session. A forwarded socket fails with `connected endpoint is not the expected process`. Source: <https://code.claude.com/docs/en/errors.md>, "Refusing to send a cross-session message".
- Only the auth line of the socket protocol is documented: `{"type":"auth","token":"<CLAUDE_CODE_MESSAGING_TOKEN>"}`. The message line format is not documented.
- `crossSessionInbound` (`accept`, `hold`, `refuse`) controls inbound messages. Without a value, a session that bypasses permission prompts holds a message from a session that does not bypass them.
- `isolatePeerMachines: true` requires approval before a message leaves the machine.
- A message from another session never counts as consent of the user. It cannot answer a pending permission prompt.
- A session answers to the name set with `--name` or `/rename`. When a live session on the same machine already has the name, Claude Code gives the new session a variant of the name.
- Only a session that binds an inbox socket can receive messages.

## Orca

- Orca orchestration has runs, tasks, dispatches, blocking `ask` and `reply`, `worker_done`, decision gates, and nested workers with a depth limit. Source: `orca skills get orchestration`.
- `orca orchestration worker-start` has no option to pass CLI flags such as `--autocompact` to the agent. It accepts `--model` and `--effort`. Source: `orca orchestration worker-start --help`.
- Orca exposes no context usage field. Source: `orca agent-context --json`.
- `orca terminal send --text <text> --enter` types text into a terminal. Source: `orca agent-context --json`.
- `orca serve` starts a headless Orca runtime on a WebSocket endpoint. `--pairing-address` sets the advertised address, for example a LAN, VPN, or SSH-forward address. `orca environment add --name <name> --pairing-code <code>` saves the runtime, and `worker-start --on <environment>` starts a supervised worker on it. Source: `orca serve --help`, `orca environment add --help`.
- The executable of the Orca CLI is selected in this order: the value of `ORCA_CLI_COMMAND` when it is set; `orca-dev` in a development checkout whose session has `ORCA_DEV_REPO_ROOT`; `orca-ide` on Linux outside an Orca terminal; else `orca`. On Linux, a bare `orca` outside an Orca terminal is normally the GNOME Orca screen reader (`/usr/bin/orca`), which starts speech. When the selected executable cannot run, report its error and stop, and do not try another executable. Source: the installed `orca-cli` and `orchestration` skill files, and `orca skills get orca-cli`, Orca 1.4.218, 2026-10-04.
- `orca --version` prints the version, for example `1.4.218`. `orca status --json` shows the readiness and does not start the app: the top-level field `ok` (`true`), `result.app.running`, `result.app.pid`, `result.runtime.state` (`ready`), `result.runtime.reachable`, `result.runtime.appVersion`, and `result.runtime.capabilities`. `orca open` starts the app. Most other commands need a running runtime. Source: `orca --help`, `orca status --help`, `orca open --help`, and `orca status --json`, Orca 1.4.218, 2026-10-04.
- `orca terminal create [--worktree <selector>] [--title <name>] [--command <text>] [--focus] --json` makes a terminal tab in an Orca worktree. On macOS and Linux, the command is typed into the login shell of the terminal. Without `--focus`, the tab does not take the focus. When the app cannot show the tab, Orca keeps a background handle. `orca terminal rename` changes the title. Source: `orca terminal create --help` and `orca terminal rename --help`, Orca 1.4.218, 2026-10-04.
- A worktree selector is `id:<repoId>::<path>`, `path:<path>`, `name:<name>`, `branch:<branch>`, `identity:<identity>`, or `active`. An Orca worktree belongs to a repository that `orca repo add --path <path>` registered. Source: `orca --help` and `orca repo add --help`, Orca 1.4.218, 2026-10-04. Verify: `terminal create --worktree path:<folder>` for a folder of a repository that Orca does not know.
- Each repository of `orca repo list --json` has the field `externalWorktreeVisibility` (`show` or `hide`). `orca worktree list --json` lists a linked worktree that `git worktree add` made outside Orca, when the repository has `show`. The `repo` commands of the CLI have no option to change the field. Source: `orca repo list --json`, `orca worktree list --json`, and the list of `repo` commands of `orca --help`, Orca 1.4.218, 2026-10-04.
- `orca terminal list --json` lists only the terminals that Orca manages. Each row has `handle`, `title`, `worktreeId`, `worktreePath`, `connected`, `agentIdentity`, and `lastOutputAt`. `orca terminal show --json` also has `agentWait`, which names an agent that waits on a prompt that only a human can answer. A handle is valid for one runtime: after an Orca restart, or the error `terminal_handle_stale`, list the terminals again. Source: `orca terminal list --json`, `orca terminal show --json`, and `orca skills get orca-cli`, Orca 1.4.218, 2026-10-04.
- `orca terminal close --worktree <selector> --all` stops each terminal process of the workspace and removes its tabs and its agent resume records. The help says: "Use workspace Sleep when the terminals and agent sessions should resume later." Source: `orca terminal close --help`, Orca 1.4.218, 2026-10-04.
- `orca worktree create --agent <id> --prompt <text>` starts the agent with the launcher that Orca has configured. It has no option to pass other arguments to the agent. For other arguments, use `terminal create --command`. Source: `orca skills get orca-cli`, Orca 1.4.218, 2026-10-04.
- `orca worktree rm` also tries to delete the local branch of the worktree. Orca keeps a branch whose changes it cannot prove are merged. Source: `orca worktree rm --help`, Orca 1.4.218, 2026-10-04.
- Orca adds hook commands for the Claude Code events to the user settings. Each one runs `~/.orca/agent-hooks/claude-hook.sh`. The script exits at once when `CLAUDE_JOB_DIR` is set, and it sends nothing when `ORCA_AGENT_HOOK_PORT`, `ORCA_AGENT_HOOK_TOKEN`, or `ORCA_PANE_KEY` is not set. So Orca gets no hook event from a background session. Source: a read of the user settings and of the installed hook script, Orca 1.4.218, 2026-10-04. This confirms the line "the Orca hook skips sessions that have `CLAUDE_JOB_DIR`" of spec section 4.1.
- A clerk that a clanker started with `session_launch`, in a bigm that runs in an Orca terminal, had the `ORCA_*` variables of the terminal of bigm, for example `ORCA_TERMINAL_HANDLE` with the handle of a live terminal of bigm. So an `ORCA_*` variable does not prove that a session runs in its own Orca terminal. Source: the names of the environment variables of the clerk session, compared with `orca terminal list --json`, Orca 1.4.218, Claude Code 2.1.284, 2026-10-04. Verify: whether the variables come from the dispatching shell or from the environment of the supervisor. The first case does not agree with the item "From the dispatching shell it keeps only `PATH` and the cloud provider variables" of the section "Launch, loops, and limits".
- `orca search` searches the agent sessions that one Orca host indexed. It works only when a human turned it on under Settings, Agent Session History, and the CLI cannot turn it on. `orca search --index-status --json` gives `enabled`. Source: `orca search --help` and `orca skills get orca-cli`, Orca 1.4.218, 2026-10-04. Verify: whether the index has the transcripts of background sessions.

## Claude Code plugins

- A plugin can ship skills, agents, commands, hooks, MCP servers, and `userConfig` values that Claude Code prompts for when the plugin is enabled. Hook commands can reference `${CLAUDE_PLUGIN_ROOT}`. Source: <https://code.claude.com/docs/en/plugins-reference.md>.
- A plugin `settings.json` applies only the keys `agent` and `subagentStatusLine`. Claude Code drops all other keys at load. A plugin cannot set `autoCompactWindow` or `statusLine`. Source: plugins-reference.md, `settings`.
- A plugin ships Workflow scripts in a `workflows/` directory at the plugin root, or in the paths of the `workflows` manifest field. Plugin workflows are namespaced: a script with `meta.name` `release-audit` in the plugin `acme-tools` runs as `/acme-tools:release-audit`. Source: <https://code.claude.com/docs/en/workflows.md>, "Distribute a workflow in a plugin".
- `${CLAUDE_PLUGIN_DATA}` resolves to `~/.claude/plugins/data/<id>/`. Claude Code creates it on first reference and keeps it across plugin updates. Claude Code deletes it when you uninstall the plugin from the last place, unless you pass `--keep-data`. Hook commands can reference it. A status line command cannot, because it is not a hook. Source: plugins-reference.md, "Environment variables".
- A marketplace is a repository with `.claude-plugin/marketplace.json`. Install from GitHub with `claude plugin marketplace add <owner>/<repo>`, then `claude plugin install <plugin>@<marketplace>`. Inside a session, `/plugin marketplace add` and `/plugin install` do the same. Source: <https://code.claude.com/docs/en/plugin-marketplaces.md>.
- `${user_config.KEY}` is substituted in skill and agent content, MCP and LSP server config, and exec-form hook `args`. Only non-sensitive values are substituted in skill and agent content. Hook processes get every option as `CLAUDE_PLUGIN_OPTION_<KEY>`. Non-sensitive values are saved under `pluginConfigs` in the user `settings.json`. Source: plugins-reference.md, "User configuration".
- A plugin option of the type `string` can have `options`, a fixed list of values that `/config` shows as a picker. A plugin with `options` does not load in Claude Code before v2.1.271. Source: <https://code.claude.com/docs/en/plugins-reference.md>, "Limit a field to fixed options", verified 2026-10-04.

## Background sessions (agent view)

Source for all items in this section: <https://code.claude.com/docs/en/agent-view.md>.

- `claude --bg "<prompt>"` (long form `--background`) starts a background session in the current directory. `--name <name>` sets its display name. `--agent <name>` runs a defined subagent as the main agent of the session. `--bg` cannot be combined with `-p`.
- In a directory that is not trusted, a script gets the error `Workspace not trusted` and no session starts.
- `claude --resume <session-id> --bg "<prompt>"` continues an existing conversation in the background. On v2.1.257 or later it continues under the same ID when it can.
- The supervisor stops the process of a session that is finished or waits for a message and has no attached terminal for about an hour. The conversation stays on disk and resumes on the next attach or reply. Only the agent view can pin a session (`Ctrl+T`).
- The supervisor restarts a process that exits unexpectedly.
- Each background session has `CLAUDE_JOB_DIR` set to `~/.claude/jobs/<id>`. The docs suggest `$CLAUDE_JOB_DIR/tmp` for files of the session.
- A supervisor process hosts background sessions. They keep working after the terminal closes.
- `claude agents --json --all` is the supported way to read session state from a script. Fields: `cwd`, `kind`, `startedAt`, `id`, `state` (`working`, `blocked`, `done`, `failed`, `stopped`), `pid`, `status` (`busy`, `waiting`, `idle`), `waitingFor`, `sessionId`, `name`. The files under `~/.claude/jobs/` are not a stable interface.
- `claude attach <id>`, `claude logs <id>`, `claude stop <id>`, and `claude respawn <id>` manage a session from the shell.
- Before a background session edits files, Claude moves it into its own git worktree under `.claude/worktrees/`, unless it already runs in a linked worktree.
- Only the main conversation of a session can ask a session on the same machine for a notice when that session goes idle or exits. Source: cross-session-messaging.md.
- The supervisor stops a process only when the session is finished or waits for the next message, and is unattached for about an hour. A session that is working, waits on a permission prompt or another dialog, or is attached keeps its process. Attaching to a stopped session starts a new process from the saved conversation. Detaching (`←`, `Ctrl+Z`, `/exit`, or a double `Ctrl+C` or `Ctrl+D`) never stops a background session. Source: agent-view.md, "The supervisor process", verified 2026-10-04.
- Two processes cannot write to the same transcript. When the conversation of a stopped session is open in another live process, Claude Code does not start the process of the session, and opening the row shows `Can't open — this session is running in another terminal` (since v2.1.248). Source: agent-view.md, "Opening a session says the conversation is already open", verified 2026-10-04. Verify: what `claude attach <id>` does when another terminal is already attached to the same session.
- A background session does not move into a worktree when it already runs in a linked git worktree, when the file is in a linked worktree, when the folder is not a git repository and no `WorktreeCreate` hook is set, or when the write is outside the working directory. The project setting `worktree.bgIsolation: "none"` turns the move off (since v2.1.143). Source: agent-view.md, verified 2026-10-04.
- `/status` shows the row `Session kind`: `background job · attached`, `background job · unattended`, or `interactive`. Source: agent-view.md, verified 2026-10-04.
- `claude agents --cwd <path>` shows only the sessions started under the path, also a session that moved into a worktree under it. Source: agent-view.md and `claude agents --help`, Claude Code 2.1.284, verified 2026-10-04.
- `claude agents --json` lists the live interactive sessions too. The help says "Print active sessions (interactive and background)". On 2026-10-04 with Claude Code 2.1.284, the output had 10 entries with `kind: "interactive"`, each with only `cwd`, `kind`, `name`, `pid`, `sessionId`, `startedAt`, and `status`: no `id` and no `state`. agent-view.md says that the interactive sessions in other terminals do not show in the list of agent view until they go to the background. Source: `claude agents --help` and `claude agents --json`, Claude Code 2.1.284, 2026-10-04. Verify: whether an interactive entry has `waitingFor` during a permission prompt, and whether it stays in `--all` after its process exits.
- `claude attach <id>`, `claude logs <id>`, `claude stop <id>`, and `claude rm <id>` take the short ID that `claude --bg` prints. `claude rm <id>` deletes a background session, and its worktree when that is safe. `claude respawn --all` restarts all background sessions. Source: `claude --help`, Claude Code 2.1.284, 2026-10-04.
- `--name` sets the display name of the session, which shows in `/resume` and in the terminal title. Source: <https://code.claude.com/docs/en/cli-reference.md>, verified 2026-10-04.
- An attached session always shows in full screen mode, because a background session has no terminal scrollback. Source: agent-view.md, verified 2026-10-04.
- Verify: the MCP server of a background session gets the `PATH` of the session, and a `claude --bg` that the MCP server starts keeps the `PATH` of the MCP server. The docs say only that a background session keeps `PATH` from the dispatching shell (section "Launch, loops, and limits"). The step `orca-absent` of the smoke test (spec section 4.3) checks it.

## Permissions and launch

- Writes to protected paths are never approved automatically, except in `bypassPermissions` mode. The mode decides: `default` and `acceptEdits` prompt, `auto` sends the write to the classifier, `dontAsk` denies. Allow rules do not pre-approve these writes. Protected directories include `.claude`, except `.claude/worktrees`, and `.git`. Source: <https://code.claude.com/docs/en/permission-modes.md>, "Protected paths".
- The allow rule `Workflow` approves every workflow launch. `Workflow(<name>)` approves one saved workflow. Without a rule, Claude Code asks before a workflow runs. Source: workflows.md, "Approve the plan before it runs".
- `--settings <file-or-json>` overrides settings keys for one session. `--agent <name>` sets the agent of the session. Source: <https://code.claude.com/docs/en/cli-reference.md>.
- After compaction, the system prompt reloads, and Claude Code injects the body of each skill that the session invoked again. Source: <https://code.claude.com/docs/en/context-window.md>.
- Claude Code sets `CLAUDE_CODE_CHILD_SESSION=1` in the processes that its Bash tool starts. `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1` forces the transcript and the `claude agents` registration for a `claude` started from inside another session. Source: <https://code.claude.com/docs/en/env-vars.md>.
- The Workflow script API has `agent()`, `pipeline()`, `parallel()`, `log()`, `phase()`, `workflow()`, the `args` global, and `budget`. The script has no input call and no filesystem access. Source: the bundled `/workflow-authoring` skill.
- A relaunch with `resumeFromRunId` returns the cached results of the completed `agent()` calls whose prompt and options did not change. Source: the bundled `/workflow-authoring` skill, "Resume".
- A subagent that runs in the background keeps these built-in tools: `Read`, `Grep`, `Glob`, `LSP`, `Bash`, `PowerShell`, `Edit`, `Write`, `NotebookEdit`, `WebFetch`, `WebSearch`, `TodoWrite`, `Skill`, `ToolSearch`, `EnterWorktree`, `ExitWorktree`, `Monitor`, `TaskStop`, `SendMessage`, and `Artifact`. No subagent gets `AskUserQuestion` or `Workflow`. A background subagent does not keep `ListAgents`. Source: <https://code.claude.com/docs/en/sub-agents.md>, "Available tools".
- A subagent with `SendMessage` gets a roster of `main` and the other named agents of the session as valid `to` values. Source: sub-agents.md.

## Probe results (2026-09-27, Claude Code 2.1.280)

Probe: a workflow with one agent. The agent sent a question to `main` with `SendMessage`, then waited with `sleep 20` in a loop. The main session answered with `SendMessage` to the agent ID.

- The question reached the main session during the run. `SendMessage` to `main` from a workflow agent works.
- The answer did not reach the running agent. `SendMessage` to the agent ID returned "Resuming agent" and started a copy of the agent from its transcript, outside the workflow. The copy got the answer and reported to the main session with `SubagentHandback`. The agent in the workflow waited 12 times, did not get the answer, and returned `timeout` to the script.
- Result: a workflow agent can ask. A reply by `SendMessage` cannot reach it in the same run.

Probe 2: the same workflow shape, but the agent waited for an answer file with a Bash loop (`[ -s <file> ]`, `sleep 5`, at most 100 seconds for each call). The main session wrote the file with a temporary name and then renamed it.

- The agent read the answer from the file during the same run and returned `{"got_answer":true,"answer":"<secret word>","bash_calls":1}` to the script. The round trip took 24 seconds.
- Result: a workflow agent can ask a question and get the answer in the same run when the answer comes back through a file.

## Field lessons (2026-09-29)

Lessons from a retrospective of a 4-day multi-agent Claude Code run (a coordinator, leads, and workers). An adversarial verifier checked each lesson against the retrospective text. The retrospective itself is private.

- A status is true only when it was just read from the system that owns it.
- A rule that keeps coming back needs a mechanical stop, not another note.
- A session waiting on a prompt must wake its parent, and a waiter timeout is an alarm.
- Report how many tests ran and how many were skipped, and treat an unexpected skip as a failure.
- A compaction summary is not a source for rules.
- A resumed session re-checks each pending item against its source.
- Rewrite the handoff as one current state. Do not append to it.
- Put the items themselves in a handoff, not pointers to files.
- Timestamps come from code, not from the model.
- Handoffs are keyed by role name, not session ID.
- Write an incoming request to durable state as soon as it arrives.
- Every shared resource needs a queue or a gate.
- Only one merger acts at a time, and a merge is confirmed through the forge API, not an exit code.
- A refused action is escalated, never handed to another session.
- Send each message to a durable mailbox and as a live nudge.
- Pass message bodies through files, not shell arguments.
- Never type into the owner's input terminal.
- Pin tool-account variables when launching a session, and probe an account before claiming it is out of quota.
- Pin review diffs to a base SHA.
- Separate "merged" from "live".
- Rebuild tracker state from git.
- Broadcast new rules live, because memory notes are read only at start.
- Name the known file overlaps up front, and treat any other overlap as a reason to stop.
- Check which identity a token acts as before its first write.
- Never move a person's credential.
- Give risky steps narrow tools and written STOP conditions, and never retry a paid failure blindly.
- Tag a recommendation separately from a ruling.

## Launch, loops, and limits (verified 2026-09-30)

- Configuration flags of the launch carry through to a background session: `--settings`, `--mcp-config`, `--strict-mcp-config`, `--setting-sources`, `--add-dir`, `--plugin-dir`, `--fallback-model`. Source: agent-view.md, "What carries over when you background".
- A background session reads settings from its folder and the carried flags. From the dispatching shell it keeps only `PATH` and the cloud provider variables. Source: agent-view.md, "Settings and provider".
- With `CLAUDE_CONFIG_DIR` set, the supervisor runs as a separate instance with its own sessions. Source: agent-view.md, "Where state is stored".
- Plugin subagents ignore the `hooks`, `mcpServers`, and `permissionMode` frontmatter fields. Source: <https://code.claude.com/docs/en/sub-agents.md>.
- After a shutdown, a background session shows as failed within 48 hours and as stopped after 48 hours. An attach or a reply restarts it. Source: agent-view.md, troubleshooting.
- `ScheduleWakeup` reschedules the next iteration of a self-paced `/loop`. `CronCreate` tasks are restored on `--resume` or `--continue` if they did not expire. A recurring task expires 7 days after creation. Source: <https://code.claude.com/docs/en/scheduled-tasks.md> and <https://code.claude.com/docs/en/tools-reference.md>.
- A workflow run pauses at a usage limit only in an interactive session with a claude.ai subscription and `autoContinueAtUsageLimit` on. In a background session, the agents that hit the limit fail. Source: workflows.md, "When a run hits your usage limit".
- `CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` sets how many agents one workflow run executes at once, from 1 to 256. The default is 16. Source: <https://code.claude.com/docs/en/env-vars.md>.
- An MCP tool call that gets no response and no progress notification for the idle window aborts. A per-server `timeout` in `.mcp.json` is a hard wall-clock limit, and a value of at least 1000 is also a floor for the idle timeout. Without it, the wall-clock default is about 28 hours. Source: <https://code.claude.com/docs/en/mcp.md>.
- A Bash permission rule matches the command text after Claude Code splits compound commands and strips wrappers. It does not match the same program invoked in a different form. Source: <https://code.claude.com/docs/en/permissions.md>.
- No channel runs until a user opts it in for the session with `--channels`. Source: channels.md.

## Probe results (plan 1, 2026-09-29, Claude Code 2.1.284)

`probes/run.sh` runs P1 to P3; set `PROBE_WORKDIR` to point it at an existing folder instead of a fresh `mktemp -d`. P4 to P7 were run by hand as `probes/README.md` describes, each as `claude --bg` in a scratch folder inside a trusted folder, none attached to after it started.

Fact, confirmed both ways: `claude --bg` refuses to start in a folder whose trust dialog nobody has accepted interactively (a first pass at P2 to P7, each in a fresh, never-trusted folder, failed this way before the session did anything — `claude --help` documents no flag for it, and `-p` only skips the dialog for that one run, it does not persist trust). Workspace trust is inherited by a subfolder of an already-trusted folder, so a scratch folder inside a folder the user trusted in an interactive session works without touching `~/.claude.json`. The one catch: `git init` inside that scratch folder makes it a repository root of its own, a new trust boundary that blocks the inheritance again — so `probes/run.sh` now runs `git init` only for its own fresh `mktemp` folder, never for a caller-supplied `PROBE_WORKDIR`. With a trusted-and-uninitialized scratch folder, P1 through P7 all reached and passed the behavior each is meant to test.

| ID | Result | Evidence |
|---|---|---|
| P1 | PASS | `claude -p --plugin-dir probes/probe-plugin --agent probe:probe-role "who are you"` printed `PROBE-ROLE-LOADED`. |
| P2 | PASS | `PROBE_WORKDIR=<scratch folder inside a trusted folder> sh probes/run.sh` started `claude --bg --name probe-p2 ...`; `claude agents --json --all` showed a session named `probe-p2` at that `cwd` with `state: "done"`. |
| P3 | PASS | Same run as P2: `/tmp/bruh-probes/hooks.tsv` gained a `PreToolUse` line carrying `clerk-probe-p3`, the `BRUH_ROLE_KEY` from the `--settings` file. |
| P4 | PASS | `claude --bg --name probe-p4`, `--settings` setting `statusLine.command` to append `date -u` to `/tmp/bruh-probes/statusline.log`, given a 3-minute task, in a scratch folder inside a trusted folder, not attached. After 90 seconds the log had 7 lines while `claude agents --json --all` showed the session `status: "busy", state: "working"` — it grew with no terminal attached. |
| P5 | PASS | `claude --bg --plugin-dir probes/probe-plugin --name probe-p5 "Use a subagent to run: echo sub"`, same scratch setup. `/tmp/bruh-probes/hooks.tsv` gained two `PreToolUse` lines sharing one non-empty `agent_id` (the subagent's tool calls), alongside the main session's own `SessionStart` and `PreToolUse` lines, which carry no `agent_id`. |
| P6 | PASS | `probes/probe-plugin/workflows/hello.js` (one `agent()` call, `meta.name: 'hello'`) plus the allow rule `Workflow(probe:hello)` in `--settings`; `claude --bg --plugin-dir probes/probe-plugin --name probe-p6 "/probe:hello"`, same scratch setup. `claude logs <id>` shows the workflow ran with no approval prompt and the agent returned `"HELLO-WORKFLOW-OK"`. |
| P7 | PASS | `claude --bg --name probe-p7` asked to make a recurring `CronCreate` task appending `date -u` to `/tmp/bruh-probes/cron.log`, same scratch setup; nobody typed into the session for 5 minutes. The log gained 5 lines, about one a minute, while `claude agents --json --all` showed the session idle throughout. |
| P8 | PASS (2026-09-30) | In a scratch folder inside a trusted folder (no `git init`), with a settings file `{"env":{"BRUH_ROLE_KEY":"clerk-probe-p8"}}`: `claude -p --plugin-dir plugins/bruh --settings <that file> --permission-mode auto "Call the bruh MCP tool answer_wait with question_id Q-8 and deadline_seconds 600. Print its result exactly."` compiled the server with `go run` on first use, then printed `{"status":"pending"}` and exited 0 after 649 seconds, with no idle-timeout error. |
| E2E | PASS (2026-09-30) | In a scratch folder inside a trusted folder (no `git init`), with a settings file `{"env":{"BRUH_ROLE_KEY":"clanker-e2e"}}`: `claude -p --plugin-dir <repo>/plugins/bruh --settings <that file> --permission-mode auto "Call the bruh MCP tool report_write with kind status and text e2e-ok. Then call report_read with role_key clanker-e2e and print the text of the first line."` compiled the server with `go run` on first use, then printed "The first line's text is `e2e-ok`. Both calls worked", confirming `report_write` and `report_read` round-tripped through the MCP server with `BRUH_ROLE_KEY` reaching it from `--settings`. |

Every background session above (`probe-p2`, `probe-p4`, `probe-p5`, `probe-p6`, `probe-p7`) was stopped with `claude stop <id>` and confirmed absent a `pid` in `claude agents --json --all` afterward.

## Workspace trust for test repositories (verified 2026-09-30, Claude Code 2.1.284)

- `claude --bg` in a new repository made with `git init` inside a trusted folder fails with `Workspace not trusted`, also when a parent folder of it is trusted.
- `claude --bg` in a linked worktree of a trusted repository (`git worktree add`) starts and finishes normally. So a test that needs a temporary git repository makes it a linked worktree of a trusted repository.
- `claude --bg --plugin-dir <plugin> --agent <plugin>:<agent>` prints `warning: no agent named '<plugin>:<agent>' — spawning with default template`, because the launcher checks the name before it loads the plugin. The session loads the plugin agent all the same: its header shows `@<plugin>:<agent>`, and it follows the agent text. Verified 2026-09-30 with `bruh:bigm`. The warning is harmless.
- `auto` permission mode is not available for Haiku 4.5. A session on that model falls back to manual mode.

## Live test results of v0.1 (2026-09-30, Claude Code 2.1.284)

- Smoke test (`tests/smoke/run.sh`, run 3 on the final tree): 10 steps PASS, the remote step SKIP (no Orca environment paired). bigm started the clanker with `session_launch`, the clanker started two clerks, a clerk started `/bruh:deliver` from a start message, a workflow agent asked a P1, the answer reached the agent in the same run, bigm raised a P0 with `claude attach <id>` for a clerk that waited on a permission prompt, and bigm restarted a stopped clanker under the same session ID.
- The earlier runs found four defects, each fixed before run 3: the scratch ledger had no default `priorities.md` (bigm stopped at its hard stop, as the procedure says), the watcher looked in the data folder of an installed plugin, the driver lost clerks that `EnterWorktree` moved under `.claude/worktrees/` of the trusted repository, and the cleanup left their branches.
- `EnterWorktree` of a session in a linked worktree makes its worktree under `.claude/worktrees/` of the main repository, on a new branch `worktree-<name>` from the HEAD of the main repository.
- bigm raised a P0 when the deny rule `Bash(git push:*)` refused the push of the ledger clerk, so a refusal is escalated, not retried (principle 6).
- Load test (`tests/load/run.sh`, 8 background sessions on Haiku 4.5, 60 minutes, one message every 2 minutes): 244 messages, 0 missed, each session got a message in each 5-minute window, 0 resumes needed. This passes the Verify item of spec section 4.1 for the transport. The sessions ran in manual mode, because Haiku 4.5 has no auto mode.
