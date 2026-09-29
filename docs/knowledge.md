# Knowledge

Verified facts about Claude Code and Orca that the design depends on. Each fact has a source. Verify a fact again before you change a decision that depends on it.

Verified on 2026-09-27 with Claude Code 2.1.280 and Orca 1.4.209.

## Context window and compaction

- `autoCompactWindow` (settings), `--autocompact` (CLI flag), and `CLAUDE_CODE_AUTO_COMPACT_WINDOW` (environment variable) set how full the context gets before automatic compaction. The environment variable has the highest precedence, then the flag, then the setting. Source: https://code.claude.com/docs/en/model-config.md, "Set the auto-compact window".
- The value is a token count from 100K to 1M, not a percent. Claude Code caps it at the context window of the model. A value of 550000 gives 55 percent on a 1M model only. On a 200K model the cap makes the value equal to the full window. Source: same section.
- Without a setting, a 1M model compacts at about 967K tokens. Source: same page, "Sonnet 5 context window".
- `SessionStart` fires again after compaction, with `source` equal to `compact`. Its hook can return `hookSpecificOutput.additionalContext`. Source: https://code.claude.com/docs/en/hooks.md, SessionStart input and decision control.
- `PreCompact` has the matchers `manual` and `auto`. It can block a proactive automatic compaction. Claude Code discards its `systemMessage`, so it cannot instruct the agent. Source: hooks.md, PreCompact.
- `PostCompact` receives `compact_summary`. It has no decision control. Source: hooks.md, PostCompact.
- A `PostToolUse` hook can return `hookSpecificOutput.additionalContext` to give Claude a message after a tool call. Source: hooks.md, PostToolUse.
- Hook inputs do not contain context usage. Every hook input contains `session_id` and `transcript_path`. Source: hooks.md, common input fields.
- `SessionStart` matcher values are `startup`, `resume`, `clear`, `compact`, and `fork`. Source: hooks.md, matcher table.
- Plugin hooks also run inside subagents. `PostToolUse` then fires for the tool calls of the subagent, and the input contains `agent_id`. The main conversation has no `agent_id`. Source: hooks.md.
- Claude Code caps `additionalContext` at 10,000 characters. For a longer value, Claude gets a file path and a preview of the first 2,000 characters. Source: hooks.md.
- The status line input contains `context_window.used_percentage` and `session_id`. The `session_id` is stable for the life of a session. Source: https://code.claude.com/docs/en/statusline.md.
- `context_window.used_percentage` can be `null` early in a session. `context_window.current_usage` is `null` after `/compact` until the next API call. Source: statusline.md.
- The status line runs on events with a 300 ms debounce. It can go quiet while the session is idle. `refreshInterval` adds a timer. Source: statusline.md, "When it updates".
- No documented mechanism lets one session start compaction in another session.

## Cross-session messaging

- Cross-session messaging does not use TCP. Source for all items in this section: https://code.claude.com/docs/en/cross-session-messaging.md.
- On one machine, each session binds a Unix domain socket (a named pipe on Windows). Claude Code exports its path as `CLAUDE_CODE_MESSAGING_SOCKET` and restricts it to the operating-system user.
- To a session on another machine, a message goes through Anthropic servers over Remote Control. Both ends need a sign-in with a Claude account and Remote Control. Without Remote Control on the sender, the message has no reply address.
- Messages are plain text.
- Before a send on the same machine, Claude Code checks that the process that holds the target socket is the target session. A forwarded socket fails with `connected endpoint is not the expected process`. Source: https://code.claude.com/docs/en/errors.md, "Refusing to send a cross-session message".
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

## Claude Code plugins

- A plugin can ship skills, agents, commands, hooks, MCP servers, and `userConfig` values that Claude Code prompts for when the plugin is enabled. Hook commands can reference `${CLAUDE_PLUGIN_ROOT}`. Source: https://code.claude.com/docs/en/plugins-reference.md.
- A plugin `settings.json` applies only the keys `agent` and `subagentStatusLine`. Claude Code drops all other keys at load. A plugin cannot set `autoCompactWindow` or `statusLine`. Source: plugins-reference.md, `settings`.
- A plugin ships Workflow scripts in a `workflows/` directory at the plugin root, or in the paths of the `workflows` manifest field. Plugin workflows are namespaced: a script with `meta.name` `release-audit` in the plugin `acme-tools` runs as `/acme-tools:release-audit`. Source: https://code.claude.com/docs/en/workflows.md, "Distribute a workflow in a plugin".
- `${CLAUDE_PLUGIN_DATA}` resolves to `~/.claude/plugins/data/<id>/`. Claude Code creates it on first reference and keeps it across plugin updates. Claude Code deletes it when you uninstall the plugin from the last place, unless you pass `--keep-data`. Hook commands can reference it. A status line command cannot, because it is not a hook. Source: plugins-reference.md, "Environment variables".
- A marketplace is a repository with `.claude-plugin/marketplace.json`. Install from GitHub with `claude plugin marketplace add <owner>/<repo>`, then `claude plugin install <plugin>@<marketplace>`. Inside a session, `/plugin marketplace add` and `/plugin install` do the same. Source: https://code.claude.com/docs/en/plugin-marketplaces.md.
- `${user_config.KEY}` is substituted in skill and agent content, MCP and LSP server config, and exec-form hook `args`. Only non-sensitive values are substituted in skill and agent content. Hook processes get every option as `CLAUDE_PLUGIN_OPTION_<KEY>`. Non-sensitive values are saved under `pluginConfigs` in the user `settings.json`. Source: plugins-reference.md, "User configuration".

## Background sessions (agent view)

Source for all items in this section: https://code.claude.com/docs/en/agent-view.md.

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

## Permissions and launch

- Writes to protected paths are never approved automatically, except in `bypassPermissions` mode. The mode decides: `default` and `acceptEdits` prompt, `auto` sends the write to the classifier, `dontAsk` denies. Allow rules do not pre-approve these writes. Protected directories include `.claude`, except `.claude/worktrees`, and `.git`. Source: https://code.claude.com/docs/en/permission-modes.md, "Protected paths".
- The allow rule `Workflow` approves every workflow launch. `Workflow(<name>)` approves one saved workflow. Without a rule, Claude Code asks before a workflow runs. Source: workflows.md, "Approve the plan before it runs".
- `--settings <file-or-json>` overrides settings keys for one session. `--agent <name>` sets the agent of the session. Source: https://code.claude.com/docs/en/cli-reference.md.
- After compaction, the system prompt reloads, and Claude Code injects the body of each skill that the session invoked again. Source: https://code.claude.com/docs/en/context-window.md.
- Claude Code sets `CLAUDE_CODE_CHILD_SESSION=1` in the processes that its Bash tool starts. `CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1` forces the transcript and the `claude agents` registration for a `claude` started from inside another session. Source: https://code.claude.com/docs/en/env-vars.md.
- The Workflow script API has `agent()`, `pipeline()`, `parallel()`, `log()`, `phase()`, `workflow()`, the `args` global, and `budget`. The script has no input call and no filesystem access. Source: the bundled `/workflow-authoring` skill.
- A relaunch with `resumeFromRunId` returns the cached results of the completed `agent()` calls whose prompt and options did not change. Source: the bundled `/workflow-authoring` skill, "Resume".
- A subagent that runs in the background keeps these built-in tools: `Read`, `Grep`, `Glob`, `LSP`, `Bash`, `PowerShell`, `Edit`, `Write`, `NotebookEdit`, `WebFetch`, `WebSearch`, `TodoWrite`, `Skill`, `ToolSearch`, `EnterWorktree`, `ExitWorktree`, `Monitor`, `TaskStop`, `SendMessage`, and `Artifact`. No subagent gets `AskUserQuestion` or `Workflow`. A background subagent does not keep `ListAgents`. Source: https://code.claude.com/docs/en/sub-agents.md, "Available tools".
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
- Plugin subagents ignore the `hooks`, `mcpServers`, and `permissionMode` frontmatter fields. Source: https://code.claude.com/docs/en/sub-agents.md.
- After a shutdown, a background session shows as failed within 48 hours and as stopped after 48 hours. An attach or a reply restarts it. Source: agent-view.md, troubleshooting.
- `ScheduleWakeup` reschedules the next iteration of a self-paced `/loop`. `CronCreate` tasks are restored on `--resume` or `--continue` if they did not expire. A recurring task expires 7 days after creation. Source: https://code.claude.com/docs/en/scheduled-tasks.md and https://code.claude.com/docs/en/tools-reference.md.
- A workflow run pauses at a usage limit only in an interactive session with a claude.ai subscription and `autoContinueAtUsageLimit` on. In a background session, the agents that hit the limit fail. Source: workflows.md, "When a run hits your usage limit".
- `CLAUDE_CODE_WORKFLOW_MAX_CONCURRENT_AGENTS` sets how many agents one workflow run executes at once, from 1 to 256. The default is 16. Source: https://code.claude.com/docs/en/env-vars.md.
- An MCP tool call that gets no response and no progress notification for the idle window aborts. A per-server `timeout` in `.mcp.json` is a hard wall-clock limit, and a value of at least 1000 is also a floor for the idle timeout. Without it, the wall-clock default is about 28 hours. Source: https://code.claude.com/docs/en/mcp.md.
- A Bash permission rule matches the command text after Claude Code splits compound commands and strips wrappers. It does not match the same program invoked in a different form. Source: https://code.claude.com/docs/en/permissions.md.
- No channel runs until a user opts it in for the session with `--channels`. Source: channels.md.

## Probe results (plan 1, 2026-09-29, Claude Code 2.1.284)

`probes/run.sh` runs P1 to P3. P4 to P7 were run by hand as `probes/README.md` describes, each with `claude --bg` in its own scratch folder under `/tmp`, none attached to after it started. P8 needs the MCP server from Task 5.

| ID | Result | Evidence |
|---|---|---|
| P1 | PASS | `claude -p --plugin-dir probes/probe-plugin --agent probe:probe-role "who are you"` printed `PROBE-ROLE-LOADED`. |
| P2 | FAIL | Method: `sh probes/run.sh`. `claude --bg ... --settings <file> "Run the shell command: echo hello"` exited 1 with the same `Workspace not trusted` message as below. No session named `probe-p2` appeared in `claude agents --json --all`. |
| P3 | FAIL | Method: `sh probes/run.sh`. Same run as P2; the session that would carry `BRUH_ROLE_KEY` to the hook never started, so `/tmp/bruh-probes/hooks.tsv` has no `clerk-probe-p3` line. |
| P4 | FAIL | Method: `claude --bg` in a fresh `/tmp` scratch folder, with `--settings` pointing `statusLine.command` at a log append, and a 3-minute task, not attached. Exited 1 with the same `Workspace not trusted` message for that folder. The status line never ran. |
| P5 | FAIL | Method: `claude --bg --plugin-dir probes/probe-plugin` in a fresh `/tmp` scratch folder with the prompt "Use a subagent to run: echo sub", not attached. Exited 1 with the same `Workspace not trusted` message. `hooks.tsv` gained no new line. |
| P6 | FAIL | Method: `claude --bg --plugin-dir probes/probe-plugin` in a fresh `/tmp` scratch folder, `--settings` holding the allow rule `Workflow(probe:hello)`, prompt `/probe:hello`, not attached. Exited 1 with the same `Workspace not trusted` message. `probes/probe-plugin/workflows/hello.js` (one `agent()` call) was never invoked. |
| P7 | FAIL | Method: `claude --bg` in a fresh `/tmp` scratch folder with a prompt asking it to make a recurring `CronCreate` task, not attached. Exited 1 with the same `Workspace not trusted` message. Never reached the 5-minute idle wait. |
| P8 | pending (runs after Task 5) | — |

P2 through P7 all failed the same way, before the session did anything: `claude --bg` refuses to start in a folder whose trust dialog nobody has accepted interactively, even a folder just created with `git init`. This holds for a fresh `mktemp` folder (P2, P3) and for a purpose-made `/tmp` scratch folder (P4 to P7) alike. There is no flag for this: `claude --help` documents that `-p` (and any run whose stdout is not a TTY) skips the dialog for that one run only, and does not persist trust — confirmed by hand: running `claude -p` in a scratch folder, then `claude --bg` in the same folder, still gets the same refusal. The one documented persistent switch is `projects["<path>"].hasTrustDialogAccepted` in the user's own `~/.claude.json`, which this task does not write: it is global state on a machine running many other live sessions, outside `--settings`, and out of scope for a probe. So P2 to P7 did not reach the behavior each one is meant to test; they only reprove the known fact already in this file ("In a directory that is not trusted, a script gets the error `Workspace not trusted` and no session starts.") and in spec.md 4.1.

- P2 (spec 4.1): the Verify line "a session started this way from the Bash tool of another session appears in `claude agents --json`" stays open. No wording change; Plan 2 must not treat background-session registration as proven, and must re-run this probe from a folder a human has already trusted once (matches 4.1's own plan for the init skill to list folders that need trust).
- P3 (spec 3.1, 10.1): same open Verify for `BRUH_ROLE_KEY` reaching a hook process through `--settings`. Plan 2 and the Task 5 MCP server must re-check this once a trusted folder is available, before role keys are load-bearing.
- P4 (spec 7): the status-line-in-background-session Verify stays open. Until it is confirmed, Plan 2/3 should build the documented fallback first (the `PostToolUse` handoff hook reading token usage from `transcript_path`) rather than depend on the status-line tap for background sessions.
- P5 (spec 8.4): the Verify that a `PreToolUse` hook fires for a workflow/subagent's tool calls inside a background session, with `agent_id` set, stays open. The lease-guard hook plan must not assume subagent coverage until this is re-run.
- P6 (spec 6.1): the Verify that a script-started session can launch a plugin workflow by slash command under `Workflow(<plugin>:<name>)` stays open. The workflow-launch design (6.1, the `deliver` workflow) must not assume this is proven for a background session.
- P7 (spec 9.1): the Verify that a `CronCreate` task fires in an interactive session nobody is typing into stays open. The sweep loop (9.1) must not assume unattended cron firing works until this is re-run in a trusted folder.

