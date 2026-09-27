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
- Hook inputs do not contain context usage. Source: hooks.md, common input fields.
- The status line input contains `context_window.used_percentage` and `session_id`. The `session_id` is stable for the life of a session. Source: https://code.claude.com/docs/en/statusline.md.
- The status line runs on events with a 300 ms debounce. It can go quiet while the session is idle. `refreshInterval` adds a timer. Source: statusline.md, "When it updates".
- No documented mechanism lets one session start compaction in another session.

## Cross-session messaging

- Cross-session messaging does not use TCP. Source for all items in this section: https://code.claude.com/docs/en/cross-session-messaging.md.
- On one machine, each session binds a Unix domain socket (a named pipe on Windows). Claude Code exports its path as `CLAUDE_CODE_MESSAGING_SOCKET` and restricts it to the operating-system user.
- To a session on another machine, a message goes through Anthropic servers over Remote Control. Both ends need a claude.ai sign-in and Remote Control. Without Remote Control on the sender, the message has no reply address.
- Messages are plain text.
- Before a send on the same machine, Claude Code checks that the process that holds the target socket is the target session. A forwarded socket fails with `connected endpoint is not the expected process`. Source: https://code.claude.com/docs/en/errors.md, "Refusing to send a cross-session message".
- Only the auth line of the socket protocol is documented: `{"type":"auth","token":"<CLAUDE_CODE_MESSAGING_TOKEN>"}`. The message line format is not documented.
- `crossSessionInbound` (`accept`, `hold`, `refuse`) controls inbound messages. Without a value, a session that bypasses permission prompts holds a message from a session that does not bypass them.
- `isolatePeerMachines: true` requires approval before a message leaves the machine.

## Orca

- Orca orchestration has runs, tasks, dispatches, blocking `ask` and `reply`, `worker_done`, decision gates, and nested workers with a depth limit. Source: `orca skills get orchestration`.
- `orca orchestration worker-start` has no option to pass CLI flags such as `--autocompact` to the agent. It accepts `--model` and `--effort`. Source: `orca orchestration worker-start --help`.
- Orca exposes no context usage field. Source: `orca agent-context --json`.
- `orca terminal send --text <text> --enter` types text into a terminal. Source: `orca agent-context --json`.
- `orca serve` starts a headless Orca runtime on a WebSocket endpoint. `--pairing-address` sets the advertised address, for example a LAN, Tailscale, or SSH-forward address, as the help text lists. `orca environment add --name <name> --pairing-code <code>` saves the runtime, and `worker-start --on <environment>` starts a supervised worker on it. Source: `orca serve --help`, `orca environment add --help`.

## Claude Code plugins

- A plugin can ship skills, agents, commands, hooks, MCP servers, and `userConfig` values that Claude Code prompts for when the plugin is enabled. Hook commands can reference `${CLAUDE_PLUGIN_ROOT}`. Source: https://code.claude.com/docs/en/plugins-reference.md.
- A plugin `settings.json` applies only the keys `agent` and `subagentStatusLine`. Claude Code drops all other keys at load. A plugin cannot set `autoCompactWindow` or `statusLine`. Source: plugins-reference.md, `settings`.
- A marketplace is a repository with `.claude-plugin/marketplace.json`. Install from GitHub with `claude plugin marketplace add <owner>/<repo>`, then `claude plugin install <plugin>@<marketplace>`. Inside a session, `/plugin marketplace add` and `/plugin install` do the same. Source: https://code.claude.com/docs/en/plugin-marketplaces.md.
