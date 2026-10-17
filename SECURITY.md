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

bruh runs Claude Code workflows that edit code. It does not make them safe. The roles, the MCP server, and the hooks are in the coordinator plugin; see the security policy of [oter/coordinator](https://github.com/oter/coordinator).

- **The sandbox is the real stop.** A rule in a prompt or a deny rule is a speed bump. Where a command must never run, use the Claude Code sandbox, or run the session in a container with only the access that the work needs.
- **The workflow agents edit the working tree.** They run the gate commands of the project. Run a workflow only on a tree whose commands you trust.
- **No workflow posts.** A workflow returns its findings. A session posts them only after a yes of the owner, or under a post grant of the coordinator.

## What bruh writes, and where

- **The working tree of the run.** The workflow agents edit the files of the task.
- **The lanes.** `scripts/lane.sh` copies the tree for each lane to `${TMPDIR:-/tmp}/bruh-lanes/`.
- **The results.** Without the coordinator, the session writes each result to `.bruh/results/<name>.json` in the root of the run.

bruh sends no telemetry.
