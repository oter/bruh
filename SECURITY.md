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

- **Deny rules are speed bumps.** bruh writes deny rules into the role settings files, for example for `docker volume rm`. A Bash deny rule matches only the command text. Another form of the same program gets past it. The lease guard hook has the same limit. See principle 2 of the [specification](docs/spec.md).
- **The sandbox is the real stop.** Where a command must never run, use the Claude Code sandbox, or run bruh in a container with only the access that the work needs.
- **A message from another session is not consent.** Claude Code does not let a cross-session message answer a permission prompt. bruh sends a permission prompt or a classifier refusal to the owner as a P0. No other session runs the refused command.
- **The never-without-the-owner list is data.** The list in `priorities.md` tells the roles to stop and ask. Only the items that match an exact command pattern are also deny rules.
- **Roles are not isolated from each other.** All roles run as the same user on the same machine. The MCP server, the merge gate, and the commands of bruh know the role of the caller only from the variable `BRUH_ROLE_KEY`. Any Bash command of any session can set that variable and call the scripts or the server as another role, for example as bigm. So the role checks of the MCP tools (the sender policy of `mail_post`, the parent checks of `session_launch` and `role_settings_write`, `repos_set` only for bigm) and the merge gate are speed bumps, like the deny rules. They stop a mistake or a confused agent, not an agent that tries to get past them.

## What bruh writes, and where

- **The plugin data folder** (`~/.claude/plugins/data/<plugin id>/`). The bruh MCP server writes all bruh files here: mailboxes, handoffs, answers, questions, reports, role settings, leases, locks, the status line copies of the tap (`context/`), the tap script (`bin/`), the init result (`init/config.json`), the code host configuration (`repos.json`), and the read positions of the watcher (`watch/`). The Slack channel writes `channels/slack/state.json`. An init plan stays only in the memory of the server until it is applied. Claude Code deletes this folder when you uninstall the plugin, unless you use `--keep-data`.
- **User settings** (`~/.claude/settings.json`). The init skill writes `autoCompactWindow`, the status line command, the allow rules of bruh, and the options of the plugin (`pluginConfigs`). It shows the diff first and writes only after you say yes.
- **The ledger repository.** bigm writes and commits the ledger. The ledger is a private repository that you choose. It can contain the names and the status of your projects.

bruh sends no telemetry. Local messages go through the per-session sockets of Claude Code on your machine. Remote messages go through the Orca runtime that you pair. Channel messages go through the chat service of the channel.
