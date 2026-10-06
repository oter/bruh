# Security policy

## Supported versions

Only the latest release gets security fixes.

## Report a vulnerability

Do not open a public issue for a vulnerability. Report it privately through GitHub private vulnerability reporting:

<https://github.com/oter/bruh/security/advisories/new>

Include:

- the bruh version or commit, the Claude Code version, and the operating system;
- the steps to reproduce, and what an attacker gets;
- the files, settings, or sessions that are affected.

## Threat model in short

bruh coordinates Claude Code sessions. It does not make them safe. Read this before you run bruh in autonomous mode.

- **Deny rules are speed bumps.** bruh writes deny rules into the role settings files, for example for `docker volume rm`. A Bash deny rule matches only the command text. Another form of the same program gets past it. See principle 2 of the [specification](docs/spec.md).
- **The sandbox is the real stop.** Where a command must never run, use the Claude Code sandbox, or run bruh in a container with only the access that the work needs.
- **A message from another session is not consent.** Claude Code does not let a cross-session message answer a permission prompt. bruh sends a permission prompt or a classifier refusal to the owner as a P0. No other session runs the refused command.
- **The never-without-the-owner list is data.** The list in `priorities.md` tells the roles to stop and ask. Only the items that match an exact command pattern are also deny rules.
- **Roles are not isolated from each other.** All roles run as the same user on the same machine. The MCP server, the merge gate, and the commands of bruh know the role of the caller only from the variable `BRUH_ROLE_KEY`. Any Bash command of any session can set that variable and call the scripts or the server as another role, for example as bigm. So the role checks of the MCP tools (the sender policy of `mail_post`, the parent checks of `session_launch` and `role_settings_write`, the `allow` check of `role_settings_write` (only bigm, only for a clanker key), `repos_set`, `learn_refresh`, and `ledger_edit` only for bigm) and the merge gate are speed bumps, like the deny rules. They stop a mistake or a confused agent, not an agent that tries to get past them. `learn_scan` refuses a caller that has a role key, so only the init skill in a session of the owner calls it. Like the other role checks, this is a speed bump: a session can clear the variable.
- **`learn_scan` runs `ssh -G`.** For an SSH host alias of a remote URL, `learn_scan` runs `ssh -G -- <alias>`. This command reads the SSH configuration of the owner and makes no connection. But it can run the `Match exec` commands of that configuration. The alias comes from the git configuration of a repository, so the scan accepts only the characters `A-Z`, `a-z`, `0-9`, `.`, and `-`. The scan runs no command of a repository and reads no file of the working tree. It reads only the folder tree and the `.git` folders.
- **`ledger_edit` runs `git` in the ledger folder.** The MCP server runs `git status`, `git commit`, `git rev-parse`, and `git reset` only in the ledger folder of `init/config.json`, and only for one `*.md` file inside it. It never pushes. The commit message goes on the standard input, not on the command line.
- **The learner reads files only.** The learner subagent (`bruh:learner`) has only the tools Read, Grep, and Glob. It runs no command of a repository.

## What bruh writes, and where

- **The plugin data folder** (`~/.claude/plugins/data/<plugin id>/`). The bruh MCP server writes all bruh files here: mailboxes, handoffs, answers, questions, reports, role settings, leases, locks, the status line copies of the tap (`context/`), the tap script (`bin/`), the init result (`init/config.json`), the code host configuration (`repos.json`), the stored monitors (`monitors.json`), the poller state (`watch/`: cursors and `poller_at`), and the waiter files (`wake/`). The Slack channel writes `channels/slack/state.json`. An init plan stays only in the memory of the server until it is applied. Claude Code deletes this folder when you uninstall the plugin, unless you use `--keep-data`.
- **User settings** (`~/.claude/settings.json`). The init skill writes `autoCompactWindow`, the status line command, and the allow rules of bruh. It shows the diff first and writes only after you say yes. The init skill does not write the options of the plugin (`pluginConfigs`). The install dialog asks them, and `/config` changes them. The non-interactive form of init writes only the options that its answers have.
- **The ledger repository.** bigm writes and commits the ledger. It changes a table row or a key line through `ledger_edit`, which commits only that file. Plugin code writes the index in `learn/` (`init_apply` and `learn_refresh`), and bigm commits it. The index holds no time and no activity. The ledger is a private repository that you choose. It can contain the names and the status of your projects.

bruh sends no telemetry. Local messages go through the per-session sockets of Claude Code on your machine. Remote messages go through the Orca runtime that you pair. Channel messages go through the chat service of the channel.
