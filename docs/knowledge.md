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

## Claude Code plugins

- A plugin can ship skills, agents, commands, hooks, MCP servers, and `userConfig` values that Claude Code prompts for when the plugin is enabled. Hook commands can reference `${CLAUDE_PLUGIN_ROOT}`. Source: <https://code.claude.com/docs/en/plugins-reference.md>.
- A plugin `settings.json` applies only the keys `agent` and `subagentStatusLine`. Claude Code drops all other keys at load. A plugin cannot set `autoCompactWindow` or `statusLine`. Source: plugins-reference.md, `settings`.
- A plugin ships Workflow scripts in a `workflows/` directory at the plugin root, or in the paths of the `workflows` manifest field. Plugin workflows are namespaced: a script with `meta.name` `release-audit` in the plugin `acme-tools` runs as `/acme-tools:release-audit`. Source: <https://code.claude.com/docs/en/workflows.md>, "Distribute a workflow in a plugin".
- `${CLAUDE_PLUGIN_DATA}` resolves to `~/.claude/plugins/data/<id>/`. Claude Code creates it on first reference and keeps it across plugin updates. Claude Code deletes it when you uninstall the plugin from the last place, unless you pass `--keep-data`. Hook commands can reference it. A status line command cannot, because it is not a hook. Source: plugins-reference.md, "Environment variables".
- A marketplace is a repository with `.claude-plugin/marketplace.json`. Install from GitHub with `claude plugin marketplace add <owner>/<repo>`, then `claude plugin install <plugin>@<marketplace>`. Inside a session, `/plugin marketplace add` and `/plugin install` do the same. Source: <https://code.claude.com/docs/en/plugin-marketplaces.md>.
- `${user_config.KEY}` is substituted in skill and agent content, MCP and LSP server config, and exec-form hook `args`. Only non-sensitive values are substituted in skill and agent content. Hook processes get every option as `CLAUDE_PLUGIN_OPTION_<KEY>`. Non-sensitive values are saved under `pluginConfigs` in the user `settings.json`. Source: plugins-reference.md, "User configuration".

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

## Monitors, background tasks, and event delivery (verified 2026-10-04)

Read on 2026-10-04 (date from `date -u`) for the design of spec section 9.5. A fact that the page does not prove is tagged "Verify".

Monitor tool:

- Each watch that Claude starts with the `Monitor` tool has a deadline: 5 minutes by default, at most 30 minutes, and at most 10 minutes in a `-p` run. At the deadline the watch ends, and Claude gets one notice, so that it can start the watch again. Source: <https://code.claude.com/docs/en/tools-reference.md>, "Monitor tool".
- A `Monitor` command uses the same permission rules as Bash. In auto mode, Claude Code sets aside allow rules that name `Monitor`, and the classifier reviews each `Monitor` command as it reviews a Bash command. Source: tools-reference.md, "Monitor tool".
- When a subagent that started monitors stops, its monitors stop with it. Source: tools-reference.md, "Monitor tool".
- The `Monitor` tool is not available on Amazon Bedrock, Google Cloud's Agent Platform, or Microsoft Foundry, or when `DISABLE_TELEMETRY` or `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC` is set. Source: tools-reference.md, "Monitor tool".

Plugin monitors:

- A plugin declares monitors in `monitors/monitors.json`, or in the manifest key `experimental.monitors`. Each entry has `name`, `command`, `description`, and the optional `when`. With `when` equal to `"always"` (the default), the monitor starts at session start and on plugin reload. With `"on-skill-invoke:<skill>"`, it starts the first time that skill runs. An unknown key in an entry is an error, and the plugin does not load. Source: <https://code.claude.com/docs/en/plugins/manifest-reference.md>, "monitors".
- A plugin monitor is a shell command that runs in the background for the whole session. What it prints reaches Claude as notifications. The command runs in the working directory where the session started. Source: <https://code.claude.com/docs/en/plugins/components.md>, "Monitors".
- Plugin monitors start only in an interactive session, never with `-p`, and not where the `Monitor` tool is unavailable. Source: components.md, "Monitors". Verify: a `claude --bg` session counts as interactive for plugin monitors. `/status` shows the session kind of such a session as `background job`, not `interactive` (agent-view.md, below).
- A monitor command gets the path variables and `${ENV_VAR}` from the environment, but never `${user_config.*}`, and the process does not get `CLAUDE_PLUGIN_OPTION_<KEY>`. In a monitor command, `${CLAUDE_PLUGIN_ROOT}` and `${CLAUDE_PLUGIN_DATA}` resolve inline, but Claude Code does not export them to the process. Source: components.md, "Monitors", and manifest-reference.md, "Where each variable resolves". Verify: the monitor process gets the `env` values of the `--settings` file and of the project settings, for example `BRUH_ROLE_KEY`.
- When the owner disables a plugin during a session, its running monitors continue until the session ends. Source: components.md, "Monitors".
- The docs do not say if the permission rules or the auto mode classifier apply to the command of a plugin monitor. Verify.

Background commands and background sessions:

- A background Bash command that the main conversation started continues after a final response, until it exits, is stopped, or reaches its time limit. A command of a foreground subagent stops when that subagent ends. Source: tools-reference.md, "When a background command stops".
- A session that runs unattended (a `-p` run, an Agent SDK application, a CI job, or a cloud session) gives background Bash commands a time limit: 30 minutes, or the `timeout` that Claude passes, at most 2 hours. A local session in a terminal has no time limit. Source: tools-reference.md, "Time limit for background commands". Verify: a `claude --bg` session with no attached terminal (`background job · unattended`) gets this time limit.
- `/status` shows the session kind `background job · attached` or `background job · unattended` in a background session, and `interactive` in each other session. Source: <https://code.claude.com/docs/en/agent-view.md>.
- A background session can start subagents, monitors, and background commands. When a session moves to the background, Claude Code stops a running monitor. Source: agent-view.md.
- A running subagent, workflow, or monitor counts as working, so the supervisor does not stop the process of that session. The supervisor stops a session that is finished or waits for a message and has no attached terminal for about an hour. Source: agent-view.md.
- When the process of a background session stops or restarts, its background shell commands, workflows, and background subagents carry over to the next process. Its running monitors stop with the process. Source: agent-view.md. Note: scheduled-tasks.md says that background Bash and monitor tasks are never restored on `--resume`. The two pages describe different events. Verify: a background command carries over when the supervisor stops an idle session and a message wakes it later.
- With `-p`, Claude Code ends a background Bash command about five seconds after the final result, and waits for a `Monitor` watch until it times out or for at most ten minutes. Source: <https://code.claude.com/docs/en/headless.md>, "Background tasks at exit".

Scheduled tasks:

- `CronCreate` tasks belong to one session. A session holds at most 50 tasks. The minimum interval is 1 minute. A task fires only while Claude Code runs and is idle, between turns, with no catch-up for missed fires. A recurring task fires up to half of its interval late (at most 30 minutes) because of jitter. A recurring task expires 7 days after creation. Source: <https://code.claude.com/docs/en/scheduled-tasks.md>.
- On `--resume` or `--continue`, Claude Code restores the `CronCreate` tasks that did not expire. A self-paced `/loop`, background Bash tasks, and monitor tasks are not restored. Moving a session to the background carries its `/loop` tasks over. Source: scheduled-tasks.md, "Limitations".
- The input of a `Stop` hook has the field `session_crons`, a list of the scheduled tasks of the session. Source: <https://code.claude.com/docs/en/hooks.md>, "Stop input".

Hooks:

- A command hook with `asyncRewake: true` runs in the background and wakes Claude when it exits with code 2. Its stderr, or its stdout when stderr is empty, reaches Claude as a system reminder. This wakes Claude at once, also when the session is idle. Source: hooks.md, "Common fields" and "Run hooks in the background".
- Claude Code does not enforce `timeout` on an `async` hook, but it does enforce `timeout` on an `asyncRewake` hook. The default `timeout` of a command hook is 600 seconds. Each firing of an async hook starts a separate process, with no deduplication. With `-p`, Claude Code kills a running async hook at teardown. Source: hooks.md. Verify: the largest `timeout` that Claude Code accepts for an `asyncRewake` hook, and whether a running `asyncRewake` hook counts as working for the idle stop of a background session.
- The output of a `FileChanged` hook cannot add context, wake an idle session, or start a turn. Source: hooks.md, "FileChanged".
- Plugin hooks run in all session types, also in background sessions. Source: hooks.md.

Channels and MCP notifications:

- A channel is an MCP server that declares the capability `claude/channel` and sends `notifications/claude/channel` with `content` and `meta`. The event reaches Claude as a `<channel>` tag. Events that arrive while Claude is busy reach it together on the next turn. Source: <https://code.claude.com/docs/en/channels-reference.md>.
- When the session did not load the server as a channel, Claude Code drops the events silently and returns no error. Source: channels-reference.md, "Notification format".
- Channels are a research preview. They need a login through claude.ai or a Console API key. A channel registers only when the session names it with `--channels`. A channel that is not on the approved allowlist loads only with `--dangerously-load-development-channels`, which shows a full-screen warning dialog. Source: <https://code.claude.com/docs/en/channels.md> and channels-reference.md. Verify: the warning dialog of the development flag in a `claude --bg` launch.
- The docs name `list_changed`, progress notifications, and channel notifications as the notifications of an MCP server. Only a channel puts text into the conversation. Source: <https://code.claude.com/docs/en/mcp.md>.
