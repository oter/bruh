---
name: init
description: Set up bruh for this user. Asks the 13 setup questions one at a time, shows the diff of every file that it will write, and writes the files only after an explicit yes. Use when the user runs /bruh:init.
disable-model-invocation: true
---

# bruh init

You set up bruh for the user. You ask questions, then the bruh MCP server writes the files. You do not write or edit a settings file yourself.

## Rules

- Ask one question in each message. Wait for the answer before you ask the next question.
- Show the default of each question. An empty answer, or "default", takes the default.
- Do not guess an answer. If an answer is not clear, ask again.
- Write nothing before the user says yes to the diff. The words "yes", "apply", or "go" are a yes. Any other answer is not a yes.

## Questions

1. How should bruh address you? (required)
2. Where is the ledger repository? Give the absolute path of a local clone of a private repository. bruh creates the ledger layout in it for each file that does not exist yet. (required)
3. Mode: `human` or `autonomous`? (default `human`)
4. P1 batch interval in minutes, and P1 batch size. (defaults 60 and 5)
5. Review-round cap of the `deliver` workflow. (default 2)
6. Auto-compact window in tokens, from 100000 to 1000000. (default 550000, which is 55 percent of a 1M window)
7. Handoff threshold in percent of the context window. (default 50)
8. Concurrency cap: the maximum number of busy clerks. (default 8)
9. Wrap the current status line with the bruh tap? (default yes) Before you ask, read `statusLine.command` in `~/.claude/settings.json` with the Read tool. Show the current value, or "none". Show the new value: `'<data folder>/bin/statusline-tap.sh' '<current value>'`. Get the data folder from the `bruh_info` tool.
10. Channels: `telegram`, `slack`, both, or none. (default none)
11. Delegated P1 classes: the P1 question classes that a clanker can answer. One class for each line. (optional, default none)
12. Merge grants: for each repository, the repository (`owner/name`), the merger role key (a clerk key, for example `clerk-my-app-merge`), and the conditions, for example "CI green and all review rounds passed". (optional, default none)
13. Remote machines: the names of the paired Orca environments that bigm can use. (optional, default none)

## Plan and apply

1. Call the tool `init_plan` (`mcp__plugin_bruh_bruh__init_plan`) with `answers`, one object with these keys: `user_name`, `ledger_path`, `mode`, `p1_batch_minutes`, `p1_batch_size`, `review_round_cap`, `auto_compact_window`, `handoff_percent`, `max_busy_clerks`, `wrap_statusline` (true or false), `channels` (list), `delegated_p1_classes` (list), `merge_grants` (list of objects with `repo`, `merger`, and `conditions`), and `remote_environments` (list). Leave out a key to use its default.
2. If `init_plan` returns an error, tell the user the error and ask the question of that answer again.
3. Show the full `diff` of the result in one code block with the language `diff`. Do not shorten it. Then ask: "Apply these changes?"
4. Only after a yes, call the tool `init_apply` (`mcp__plugin_bruh_bruh__init_apply`) with the `plan_id` and the `diff_sha256` of the result. Claude Code then asks the user to approve the call, in every permission mode. The plan exists only in this session, so call `init_plan` and `init_apply` in the same session.
5. If `init_apply` says that a file changed after `init_plan`, call `init_plan` again with the same answers, and show the new diff.
6. Tell the user the list of files that `init_apply` wrote.

## After apply

Tell the user these items:

- Workspace trust: `claude --bg` does not start in a folder that is not trusted. Open `claude` one time in an interactive session, and accept the trust dialog, in each of these folders: the ledger folder (the `trust` list of the `init_plan` result), and each project folder where a clanker or a clerk will work.
- The plugin options (`user_name`, `handoff_percent`, `max_busy_clerks`) are in `pluginConfigs` of `~/.claude/settings.json`. `/config` shows them.
- Start bigm with the `launch_command` of the `init_plan` result. Show it in a code block.
- Before you uninstall the bruh plugin, set `statusLine.command` back to the previous command (the second quoted part of the new value). The uninstall deletes the plugin data folder and the tap in it.
- The watcher and the merge train send a token only to its own host: `GITHUB_TOKEN` or `gh auth token` to `api.github.com` and the hosts in `BRUH_GITHUB_HOSTS`, and `BRUH_GITEA_TOKEN_<HOST>` to a Gitea host.
- For Telegram: install `telegram@claude-plugins-official`, run `/telegram:configure <token>`, pair your account, and set the policy to `allowlist`.
- For Slack: run `/plugin configure bruh` and set the Slack bot token, the channel ID, and your Slack user ID. `plugins/bruh/channels/slack/README.md` tells how to make the Slack app.

## Non-interactive form

In a container, or in a script, run the same init without questions:

```bash
go run -C <plugin root>/mcp . init --answers <answers file>
```

The answers file is one JSON object with the keys above. A `BRUH_INIT_<KEY>` variable sets one key and has priority over the file, for example `BRUH_INIT_USER_NAME=Sam` or `BRUH_INIT_CHANNELS='["telegram"]'`. The values of `user_name`, `ledger_path`, and `mode` are text. The other values are JSON. The command prints the diff, writes the files, and prints the bigm start command. `BRUH_DATA` sets the plugin data folder (default `~/.claude/plugins/data/bruh-bruh`), and `BRUH_SETTINGS_FILE` sets the user settings file (default `~/.claude/settings.json`).
