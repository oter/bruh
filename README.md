<p align="center">
  <img src="docs/assets/banner.svg" alt="bruh: coordinate Claude Code sessions across projects and machines" width="100%">
</p>

<p align="center">
  <a href="https://github.com/oter/bruh/actions/workflows/ci.yml"><img src="https://github.com/oter/bruh/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/oter/bruh/releases/latest"><img src="https://img.shields.io/github/v/release/oter/bruh" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/oter/bruh" alt="License"></a>
  <a href="https://scorecard.dev/viewer/?uri=github.com/oter/bruh"><img src="https://api.scorecard.dev/projects/github.com/oter/bruh/badge" alt="OpenSSF Scorecard"></a>
</p>

# bruh

bruh is a Claude Code plugin. It lets one person run work in many projects on many machines through a hierarchy of Claude Code sessions.

You talk to one session, bigm. bigm keeps the status of all work in a private ledger repository and answers your questions about it. It shows you only the questions that need you. The file `mode.md` of the ledger sets the mode: `human` (bigm asks you) or `autonomous` (bigm decides P1 questions itself and records each decision).

## Roles

| Role | Lifetime | Started by | Scope | Does hands-on work |
|---|---|---|---|---|
| bigm | Long-lived | You | All projects. Keeps the ledger and the "Owed to owner" list. | No |
| Clanker | Long-lived | bigm | One project. Knows the full picture of that project. Divides the work into tasks. | No |
| Clerk | One task | A clanker | One task. Starts workflows, pushes the branch, and reports with evidence. | No |
| Workflow | One run | A clerk | One step of a task: plan, implement, review, and fix. | Yes |

Each role runs as a plugin agent (`bruh:bigm`, `bruh:clanker`, `bruh:clerk`). The work runs in the `/bruh:deliver` workflow.

## How it works

```mermaid
flowchart LR
    owner([You]) <-->|"terminal, channel"| bigm
    bigm <-->|"mailbox + SendMessage"| c1["clanker-projecta"]
    c1 <-->|"mailbox + SendMessage"| k1["clerk-projecta-task1"]
    k1 -->|"/bruh:deliver"| w1[["workflow"]]
    bigm <-->|"Orca send, ask, reply"| c2["clanker-projectb<br/>remote machine"]
    bigm -->|commits| ledger[("ledger<br/>private repository")]
```

- A local message goes to the durable mailbox of the bruh MCP server. A `SendMessage` nudge carries only its header line.
- Each question has a priority. A clanker answers P2 questions from the project context. It also answers the P1 classes that you delegate to it in `priorities.md`. The other P1 questions (your decisions) and all P0 questions (blocks that only you can remove) go to bigm, and bigm shows them to you.
- On a model with a 1M context window, each role stays below about 55 percent of its context window. A hook tells the role to write its handoff before compaction, and another hook gives the handoff back after compaction.
- A remote clanker talks to bigm through the Orca remote runtime.

The full process is in [docs/flow.md](docs/flow.md) as diagrams.

## Install

Steps 4, 5, and 6 are optional. Create the ledger repository (step 3) before you answer question 2 of the init skill.

### 1. Install the plugin

```bash
claude plugin marketplace add oter/bruh
claude plugin install bruh@bruh
```

Install at user scope, so that all your projects get the plugin.

### 2. Run the init skill

In a Claude Code session, run:

```text
/bruh:init
```

The skill asks one question for each message: how bruh addresses you, the ledger path, the mode (human or autonomous), the P1 batch interval and size, the review-round cap, the auto-compact window, the handoff threshold, the concurrency cap, the status line tap, the channels, the delegated P1 classes, the merge grants, and the remote machines. Before it writes a settings file, it shows the diff and waits for your yes.

The skill lists the project folders that need workspace trust. Start `claude` once in each of these folders and accept the trust dialog. `claude --bg` refuses a folder that is not trusted.

Non-interactive form, for a container or a script: write the answers to a JSON file, then run the init command of the MCP server. `<plugin root>` is the folder of the installed plugin, `~/.claude/plugins/cache/bruh/bruh/<version>`. The MCP tool `bruh_info` also gives it.

```json
{
  "user_name": "<how bruh addresses you>",
  "ledger_path": "/path/to/ledger",
  "mode": "human",
  "p1_batch_minutes": 60,
  "p1_batch_size": 5,
  "review_round_cap": 2,
  "auto_compact_window": 550000,
  "handoff_percent": 50,
  "max_busy_clerks": 8,
  "wrap_statusline": true,
  "channels": [],
  "delegated_p1_classes": [],
  "merge_grants": [],
  "remote_environments": []
}
```

```bash
GOTOOLCHAIN=local go run -C <plugin root>/mcp . init --answers "$PWD/answers.json"
```

Give the answers file as an absolute path. `go run -C` runs the program in `<plugin root>/mcp`, so a relative path does not point to your file.

Only `user_name` and `ledger_path` are required. The other keys have the default values that the example shows. Instead of a file, you can set each key as an environment variable `BRUH_INIT_<KEY>`, for example `BRUH_INIT_USER_NAME`. The non-interactive form prints the diff instead of asking.

### 3. Create the ledger

The ledger is Markdown in a separate private repository. It describes all your other repositories. Create it, then give its path to the init skill (question 2):

```bash
gh repo create <owner>/<ledger repository> --private --clone
```

When the folder is empty, the init skill creates the layout: `README.md`, `mode.md`, `priorities.md`, `rules.md`, `grants.md`, `questions.md`, `owed.md`, `leases.md`, and `projects/`. bigm is the only writer of the ledger. The ledger clerk pushes it after each commit of bigm.

### 4. Set up a channel (optional)

A channel sends P0 and P1 questions to your phone. A channel runs only when bigm starts with `--channels`.

Telegram uses the official channel plugin, which needs [Bun](https://bun.sh). Create a bot with BotFather and copy its token. Then, in a Claude Code session:

```text
/plugin install telegram@claude-plugins-official
/telegram:configure <token>
```

Start bigm with `--channels plugin:telegram@claude-plugins-official` (step 7). Send a message to your bot, then pair your account and allow only yourself:

```text
/telegram:access pair <code>
/telegram:access policy allowlist
```

Slack is a channel of bruh itself. During the channels research preview, a channel that is not on the Anthropic allowlist loads only with a development flag.

1. Create a Slack app for your own workspace at <https://api.slack.com/apps>. Do not distribute it: Slack gives an app that is distributed outside the Slack Marketplace only 1 read request each minute.
2. Add the bot token scopes `chat:write`, and `channels:history` for a public channel or `groups:history` for a private channel. Install the app, and copy the bot token (`xoxb-...`).
3. Create a channel for bruh, invite the app to it, and copy the channel ID. Copy your own Slack member ID from your profile.
4. Run `/plugin configure bruh` and set `slack_bot_token`, `slack_channel_id`, and `slack_owner_user_id`. Only messages from `slack_owner_user_id` reach bigm.
5. Start bigm with `--dangerously-load-development-channels plugin:bruh@bruh` (step 7). When you also use Telegram, keep the `--channels` flag too.

The details are in [plugins/bruh/channels/slack/README.md](plugins/bruh/channels/slack/README.md).

The bot allows only one reader. Each session that loads the Telegram plugin starts its server, and that server takes the bot from the session before it, also without `--channels`. The role settings of bruh turn the plugin off for every role except bigm. Your own other Claude Code sessions load it too: while bigm runs, turn the plugin off in them, for example with `--settings '{"enabledPlugins": {"telegram@claude-plugins-official": false}}'`.

### 5. Add a remote machine (optional)

On the remote machine:

1. Install Claude Code, then do steps 1 and 2. The init skill asks for a ledger path. Give it an empty local folder that is not your ledger, for example `~/bruh-remote-ledger`. bigm writes only the ledger on its own machine.
2. Start the Orca runtime:

   ```bash
   orca serve --port <port> --pairing-address <private network address>
   ```

   `--pairing-address` sets only the address that Orca advertises to clients. It does not set the address that the runtime listens on, and `orca serve` has no option for that. The command prints the bound endpoint and the advertised endpoint. Make the port reachable only on a private network: allow it only on the VPN interface with the host firewall, or keep it closed and reach it through a tunnel, for example an SSH port forward, and give the tunnel address as `--pairing-address`.

   The command also prints the pairing status and a pairing code, a link that starts with `orca://pair`.

On the machine of bigm, pair the runtime once:

```bash
orca environment add --name <environment name> --pairing-code <pairing code>
```

Give the environment name to the init skill (question 13). For remote work, run bigm in an Orca terminal.

### 6. Run in a container (optional)

For autonomous work, the container image is `ghcr.io/oter/autonomous-agents/agent`. The image is outside this repository. It has no `latest` tag: pick a published tag of the image, and check that the image has the requirements of step 8.

Put the whole `~/.claude` folder of the container user on a volume. It holds the plugin install (step 1), the user settings that the init skill writes (the allow rules, `autoCompactWindow`, and the status line), the Claude login, and the plugin data folder with the handoffs, the mailboxes, and the leases. With a volume for only a part of it, a new container loses the rest, and bigm starts without the plugin or without its allow rules.

```bash
docker volume create bruh-claude
docker run -it -v bruh-claude:<home folder of the container user>/.claude \
  ghcr.io/oter/autonomous-agents/agent:<tag>
```

In the container, do step 1, then the non-interactive form of step 2. In a container, bigm has no Orca terminal. To reach a remote clanker in a container, run `orca serve` in that container (step 5).

### 7. Start bigm

Start bigm in the folder of your ledger repository:

```bash
claude --agent bruh:bigm --name bigm --permission-mode auto \
  --settings ~/.claude/plugins/data/bruh-bruh/roles/bigm.json \
  --channels plugin:telegram@claude-plugins-official
```

For Slack, add `--dangerously-load-development-channels plugin:bruh@bruh`. Without Telegram, remove the `--channels` line. `~/.claude/plugins/data/bruh-bruh/` is the plugin data folder of the plugin `bruh@bruh`. bigm stays an interactive session. Do not start it with `--bg`.

bigm starts a clanker for each project that has work. Tell bigm what to do. For each project, tell bigm its code host repositories (`owner/name` and the host, GitHub or Gitea). bigm records them with the `repos_set` tool of bruh, so that the watcher and the merge train know them.

### 8. Check the requirements

| Requirement | Why |
|---|---|
| Claude Code 2.1.284 or later | The version that the probes of this release ran on |
| A model with a 1M context window for each role | The auto-compact window of 550000 tokens is 55 percent only of a 1M window |
| Go 1.26 or later | Claude Code starts the bruh MCP server with `go run` |
| `jq` | The hook scripts |
| `git` | Worktrees, branches, and the ledger |
| Orca | Remote machines only |
| Bun | The Telegram channel plugin only |
| macOS or Linux | Native Windows is not supported |

Background sessions use the Claude account of the Claude Code supervisor. bruh never sets `CLAUDE_CONFIG_DIR`.

> [!WARNING]
> A plugin that adds text at session start (a `SessionStart` hook with `additionalContext`) adds that text to every role session: bigm, each clanker, and each clerk. bruh starts many sessions, so the cost is multiplied. Run `claude plugin list` and disable the plugins that the roles do not need.

## Documentation

- [Specification](docs/spec.md): what bruh does, and the tag of each decision.
- [Flow](docs/flow.md): the process as diagrams.
- [Design](docs/design.md): what the owner said, and the decision log.
- [Knowledge](docs/knowledge.md): the verified facts about Claude Code and Orca, and the probe results.
- [Smoke test](tests/smoke/README.md) and [load test](tests/load/README.md).
- [Changelog](CHANGELOG.md), [contributing](CONTRIBUTING.md), and [security policy](SECURITY.md).

## License

[Apache-2.0](LICENSE).
