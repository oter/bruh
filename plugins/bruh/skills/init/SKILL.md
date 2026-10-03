---
name: init
description: Set up bruh for this user in 12 steps. Finds the ledger, asks the mode and the settings, picks the projects under the root, runs the learn step (one read-only learner for each project) and the confirm of each learned value, shows the diff of each file that it will write and writes only after an explicit yes, then runs the trust step. A second run changes the projects or the settings. Use when the user runs /bruh:init.
disable-model-invocation: true
---

# bruh init

You set up bruh for the owner. You ask questions, the learners read the projects, and then the bruh MCP server writes the files. You do not write or edit a file yourself.

## Rules

- Write nothing before the owner says yes to the diff (step 11). Only `init_apply` writes.
- Never run a command of a repository: no `git`, `task`, `make`, or another command in a repository folder. `learn_scan` reads the git facts.
- Read no file of a repository, except to check a learner line (step 9). You can read the ledger files and `~/.claude/settings.json`.
- Each step starts with one line that tells the owner what the answer controls.
- Ask each question with fixed answers as an `AskUserQuestion` select, with the default first and the label "(default)". A select has 2 to 4 options, and one screen has at most 4 questions. Put more options on more questions or more screens. Ask a path, a host, or a key as text.
- Do not guess an answer. When an answer is not clear, ask again.

## Before step 1

Call `bruh_info` (`mcp__plugin_bruh_bruh__bruh_info`). Keep its `data_dir` for step 4. When it returns a `role_key` that is not empty, stop. Tell the owner to run `/bruh:init` again from a session with no role key: `claude --setting-sources user` in the ledger folder, or a session in a folder outside each clone of the ledger. A plain `claude` in the ledger folder is bigm, and `learn_scan` refuses a caller with a role key.

## Steps

### 1. Ledger

Controls: the ledger repository where bruh keeps its state. bruh creates each ledger file that does not exist yet.

Find the ledger folders with Glob: `mode.md` at depth 1 to 4 below `~/workspace` (`*/mode.md` to `*/*/*/*/mode.md`), and `mode.md` in the current folder. Keep each folder that has a `.git` folder. Show a select of the found folders plus "another path". When you find none, or the owner selects "another path", ask the absolute path as text. The path is the `ledger` input of each `learn_scan` call and the `ledger_path` of the answers.

When the ledger has `learn/tree.json`, this is a second run: go to "Second run". Else go to step 2.

### 2. Mode

Controls: whether bigm asks the owner (`human`) or decides alone and reports (`autonomous`).

Select: `human` (default), `autonomous`.

### 3. P1 batches and review rounds

Controls: how often bigm sends the P1 questions to the owner, how many go in one batch, and how many review rounds the `deliver` workflow runs before it stops.

One select each, the default first:

- P1 batch interval in minutes: `60` (default), `30`, `120`, `240`.
- P1 batch size: `5` (default), `3`, `10`, `20`.
- Review-round cap: `2` (default), `1`, `3`, `4`.

### 4. Defaults for the rest

Controls: the auto-compact window of each session, the status line tap that gives each role its context use, and the chat channels of bigm.

Select: "Defaults for the rest" (default), "customize". "Defaults for the rest" leaves out `auto_compact_window`, `wrap_statusline`, and `channels`: `init_plan` keeps each value that the settings already have, and sets only the missing ones. "Customize" asks:

- Auto-compact window in tokens, from 100000 to 1000000: `550000` (default, 55 percent of a 1M window), `400000`, `750000`, `1000000`.
- Status line tap: `yes` (default), `no`. Before you ask, read `statusLine.command` in `~/.claude/settings.json` with Read. Show the current value, or "none", and the new value `'<data_dir>/bin/statusline-tap.sh' '<current value>'`. Tell the owner: without the tap, no role gets the handoff message at the handoff threshold, because the message reads the context use from the tap files. The roles then write a handoff only when a message tells them to.
- Channels: `none` (default), `telegram`, `slack`, `telegram and slack`.

### 5. Root

Controls: the folder under which `learn_scan` looks for the repositories of the owner.

Select: `~/workspace` (default), "another path". For "another path", ask the absolute path as text. Use the absolute path as `root`. `depth` is 4 and `exclude` is `["archive"]` unless the owner asks for other values.

### 6. Hosts

Controls: the code host of each repository, so that the pick can show its activity and bruh knows which API to call.

Call `learn_scan` (`mcp__plugin_bruh_bruh__learn_scan`) with `ledger` and `root` (and `depth` and `exclude` when they are not the defaults).

- For each entry of `hosts_without_kind`: a select of the kind: `github`, `gitlab`, `gitea`, "other: no calls". Put each kind other than "other: no calls" into `host_kinds`.
- For each entry of `aliases_without_host`: ask the real host as text, or "skip". A typed host must match `[a-z0-9.-]+` (write it in lower case). Put the alias and the typed host into `host_aliases`. When the owner skips, the pick shows the activity as "unknown", and the learner of the project proposes the host (step 9).

### 7. Pick

Controls: the repositories that bruh knows as projects. bigm and the clankers work only in these.

Call `learn_scan` again with `ledger`, `root`, `host_kinds`, and `host_aliases`: the pairs of `host_aliases` of the first call plus the hosts that the owner typed. When it returns a new entry in `hosts_without_kind`, ask its kind as in step 6.

Run `date -u` to get the time now. A repository is active when its `activity.at` is within 30 days of that time.

- Screen 1: the groups (the `group` of the repositories) and the single repositories (`group` is `""`), each with a count, for example "shop-team: 12 repos, 4 active in 30 days". Use `multiSelect`. At most 4 questions of 4 options on a screen; more than 16 items go on more screens.
- For each selected group: select "active in 30 days" (default), "all", or "pick". "Pick" opens screens of the repositories of the group, most active first, with `multiSelect`.
- On a remote machine, the owner selects no project. Then skip steps 8 to 10, and send no `projects` key: init writes no `learn/` file.

### 8. Projects

Controls: which repositories form one project, the main repository of each project, and the project key in the role keys.

Start from the `projects` of `learn_scan` that have a picked repository, with only the picked repositories.

- For each group, for each proposed project: select "accept" (default), "split", "join", "skip".
  - "Split": ask which repositories go into a new project. Each new project gets the key of the key rule below.
  - "Join": ask the project that gets the repositories of this project. A joined project takes the group of its main repository.
  - "Skip": the project is not selected.
- For each project with more than one repository: select its main repository. The clanker of the project works in the folder of the main repository.
- The key rule: the project name in lower case, each run of characters that are not an ASCII letter or digit becomes one `-`, and no `-` at the start or the end. When two groups have a project with the same name, the key gets the group name as a prefix (`group-a-infra`). When the key is empty or longer than 40 characters, ask the key as text. Each key is unique.
- When a join merges two projects that have a stored purpose, select the purpose that stays.

### 9. Learn

Controls: the proposals of the index: the purpose, the links to other projects, the doc pointers, and the host of each alias with no host.

Start one subagent `bruh:learner` for each project of step 8 with the Agent tool, at most 8 at the same time. This is not a workflow. The prompt of each learner has:

- the root;
- the key of the project and the paths of its repositories, relative to the root;
- the keys of the other selected projects, each with the paths of its repositories;
- the aliases of the project that step 6 left with no host (can be empty).

A learner can show a permission prompt to read a folder outside the working folder of this session. The owner answers it.

Collect only these closed lines, and ignore each other line:

- `PURPOSE: <text>`
- `LINK <project key>: <repo>/<file>:<line>`
- `DOC: <repo>/<path>`
- `HOST <alias>: <host>`

Parse rules:

- `<repo>` is the longest repository path of the input that the text starts with, followed by `/`.
- Split a `LINK` value at the last `:` into the file and the line.
- A path has no `..` part and does not start with `/`.
- The target key of a `LINK` is a key of the input, and not the key of the own project.

Refuse each line with one of these problems, and do not propose it:

- a value with a line break or the text ` | `;
- a second `PURPOSE` line of the same learner;
- a `PURPOSE` of more than 120 characters;
- a file that does not exist in the repository (check with Read of `<root>/<repo>/<file>`);
- a `LINK` line that is not from 1 to the last line of the file (check with Read);
- a `HOST` line for an alias that is not in the input;
- a host that does not match `[a-z0-9.-]+`.

Write nothing.

### 10. Confirm

Controls: the values that go into the index. Only a value that the owner accepts or types goes into it.

For each project, show as text: its purpose, its links with their evidence (`<repo>/<file>:<line>`), its doc pointers, and the host of each alias. Then select "accept all" (default), "one by one", "skip all".

- "One by one": for each value, select "accept" (default), "edit", "skip". "Edit" takes the value as text.
- An accepted proposal has the source `agent`. An edited or typed value has the source `owner`. A skipped value is not in the index.
- At "learn again" (second run), show the stored value next to the proposal. A stored value with the source `owner` stays unless the owner selects the new one. Put each stored value that stays into the fills, with its stored source.
- When a fill adds a host whose kind is not known (not `github.com`, not `gitlab.com`, and not in `host_kinds`), select its kind as in step 6, and put it into `host_kinds`.

The accepted values go into `fills` of the project:

- `{"field": "purpose", "value": "<text>", "source": "<source>"}`
- `{"field": "link", "value": "<project key>", "source": "<source>", "repo": "<repo>", "file": "<file>", "line": <line>}`
- `{"field": "doc", "value": "<path>", "source": "<source>", "repo": "<repo>"}`
- `{"field": "host", "value": "<host>", "source": "<source>", "repo": "<repo>"}`

A `HOST <alias>` value becomes one `host` fill for each repository of the project whose remote uses that alias (the `alias` of the repository in the `learn_scan` output). A host that the owner typed at step 6 is in `host_aliases`, and also gets one `host` fill with the source `owner` for each repository with that alias.

### 11. Diff

Controls: nothing changes on disk before your yes.

Call `init_plan` (`mcp__plugin_bruh_bruh__init_plan`) with `answers`, one object with: `ledger_path`, `mode`, `p1_batch_minutes`, `p1_batch_size`, `review_round_cap`, `auto_compact_window`, `wrap_statusline`, `channels` (a list), `root`, `depth`, `exclude`, `host_kinds` (each kind that `learn_scan` found plus each kind of the owner), `host_aliases` (each pair of `learn_scan` plus each host that the owner typed), and `projects` (a list of `{"key", "repos", "main", "fills"}`, with the paths relative to the root). Leave out a key to use its default.

When `init_plan` returns an error, tell the owner the error, and ask the question of that answer again.

- Show `outside_diff` in full, in one code block with the language `diff`. It has the user settings and `<ledger>/.claude/settings.json`. Do not shorten it.
- Show `ledger_table` as a table with the columns: project key, purpose, repositories, links, docs.
- Select "apply" (default), "show all", "cancel". "Show all" shows `ledger_diff` in full in one `diff` code block, then asks again. "Cancel" stops, and nothing is written.

Only after "apply", call `init_apply` (`mcp__plugin_bruh_bruh__init_apply`) with the `plan_id` and the `diff_sha256` of the result. Claude Code asks the owner to approve the call. The plan exists only in this session. When `init_apply` says that a file changed after `init_plan`, call `init_plan` again with the same answers, and show the new diff. Tell the owner each path of `applied`, and each path of `kept` (a project file that stays because it has open items).

### 12. Trust

Controls: whether the background sessions of bruh can start in each folder. `claude --bg` does not start in a folder that is not trusted.

Show the `trust` list of the `init_plan` result: the ledger, then each selected repository. Mark each repository that has `.claude/settings.json` or `.mcp.json`: the trust dialog turns on their hooks and MCP servers. Then select "now" (default), "at the first work".

- "Now": for each folder, with a counter (`3 of 12`), tell the owner to run `claude` in that folder, accept the trust dialog, and quit with `/exit`. A folder that is already trusted opens with no dialog. In the ledger folder, the session starts as bigm: accept the dialog, then always quit with `/exit`.
- "At the first work": bigm sends a P0 with the folder when a clanker cannot start there, and a clanker does the same for a clerk.

Never read or write the trust flags of `~/.claude.json`.

## Second run

Ask first: "change projects" (default), "change settings". The mode and the P1 settings are not asked on a second run: bigm changes them. Both branches call `init_plan` (step 11), which also merges `<ledger>/.claude/settings.json`, and then run step 12.

- "Change projects": read `learn/tree.json` and each `learn/projects/<key>.json` of the ledger. Show the selected projects with their repositories. For each project, select "keep" (default), "remove", "learn again"; then select "add a project" or "done".
  - "Add": run steps 6 and 7 with the stored `root`, `depth`, and `exclude`, and show only the repositories that no project has.
  - An added project and a project to learn again go through steps 8 to 10.
  - The answers carry the full new `projects` list, and the stored `root`, `depth`, `exclude`, `host_kinds`, and `host_aliases` with the new pairs. A kept project has its stored `key`, `repos`, and `main`, and no `fills` key: `init_plan` then keeps its stored JSON. A removed project is not in the list.
- "Change settings": ask the questions of step 4 again. Each select shows the current value first, with the label "(current)". Read the current values in `~/.claude/settings.json` with Read: `autoCompactWindow`; the tap is current when `statusLine.command` runs `statusline-tap.sh`; Slack is current when the allow rules have `mcp__plugin_bruh_slack__post_question`; Telegram is current when `enabledPlugins["telegram@claude-plugins-official"]` is true. Send `ledger_path` and the answers of step 4, and no `projects` key: the index does not change.

## After apply

Show only the notes that match the answers:

- The `launch_command` of the `init_plan` result, in a code block. It starts bigm.
- A plain `claude` in the ledger folder also starts bigm, with no `auto` mode, no name `bigm`, and no channel. After a plain start, run `/rename bigm`. When a channel is set up, start bigm only with the `launch_command`.
- Only with `telegram`: install `telegram@claude-plugins-official`, run `/telegram:configure <token>`, pair your account, and set the policy to `allowlist`.
- Only with `slack`: run `/plugin configure bruh` and set the Slack bot token, the channel ID, and your Slack user ID. `plugins/bruh/channels/slack/README.md` tells how to make the Slack app.
- Only when init wrote the tap: before you uninstall the bruh plugin, set `statusLine.command` back to the previous command (the second quoted part of the new value). The uninstall deletes the plugin data folder and the tap in it.
- Only for the host kinds of the selected repositories. bruh sends a token only to its own host.
  - GitHub: log in with `gh auth login`. bruh uses `GITHUB_TOKEN` or `gh auth token` for `api.github.com` and the hosts in `BRUH_GITHUB_HOSTS`.
  - GitLab: log in with `glab auth login --hostname <host>`. bruh calls GitLab only through `glab`.
  - Gitea: set `BRUH_GITEA_TOKEN_<HOST>`, or log in with `tea login add`.
- bigm commits the paths of `applied` at its next turn.
- Merge grants, delegated P1 classes, and remote machines are not init questions. Tell them to bigm, and bigm records them.

## Non-interactive form

In a container, or in a script, run the same init without questions:

```bash
go run -C <plugin root>/mcp . init --answers <answers file>
```

The answers file is one JSON object with the keys of step 11, with the `fills` as they are: no learner runs, and `init_plan` checks the fills with its value rules. A container has no install dialog, so the file can also have the plugin options `user_name`, `handoff_percent`, and `max_busy_clerks`.

A `BRUH_INIT_<KEY>` variable sets one key and has priority over the file, for example `BRUH_INIT_ROOT=/home/me/workspace` or `BRUH_INIT_CHANNELS='["telegram"]'`. The values of `user_name`, `ledger_path`, `mode`, and `root` are text. The other values are JSON. The command prints the diff, writes the files, prints `applied: <path>` for each write and `deleted: <path>` for each deletion, and prints the bigm start command. `BRUH_DATA` sets the plugin data folder (default `~/.claude/plugins/data/bruh-oter`), and `BRUH_SETTINGS_FILE` sets the user settings file (default `~/.claude/settings.json`).
