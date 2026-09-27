# Coordinator design (draft)

Status: brainstorming. Nothing in this file is approved.

## What the owner said (2026-09-27)

- The coordinator tracks work: what is done and what is in progress.
- The coordinator answers questions from the owner.
- The coordinator collects questions from other sessions and gives them to the owner.
- The coordinator does not hold the full context. It spreads the context across clankers.
- A clanker works in one project folder. A clanker knows the full picture of its project. A clanker does no hands-on work. A clanker gives work to a clerk.
- A clerk owns one task. A clerk starts workflows. The workflows do the work.
- Spawn chain: the coordinator starts clankers, a clanker starts clerks, a clerk starts workflows.
- Answer quality degrades at 50 to 60 percent of the context window. No session may go above that level.
- A clanker rotates its clerks. When a clerk reaches the limit before its work is done, the clerk writes a handoff, compacts, reads the handoff, and continues.
- The coordinator starts as many clankers as necessary. It does not rotate clankers. A clanker that reaches the limit writes a handoff, compacts, reads the handoff, and continues.
- The coordinator must also coordinate work on remote machines.
- The finished tool can be a public GitHub repository named `bruh`. The role inside it keeps the name coordinator.
- The `bruh` skill must be installable into Claude Code.
- The `bruh` repository holds the skillset and the workflows.

## Decisions

- Role names: coordinator, clanker (was lead), clerk (was minion), workflow. Owner decision 2026-09-27.
- Session lifetime: the coordinator and the clankers are long-lived. A clerk lives for one task. Owner decision 2026-09-27, inferred from the compaction requirement and not yet confirmed.
- Context budget: every role stays below about 55 percent of its context window. Owner decision 2026-09-27.
- Compaction mechanism: option 2 of open question 1. Auto-compact at 55 percent, a live handoff file per session, a `SessionStart` hook with matcher `compact` that injects the handoff file, and a status line tap with a `PostToolUse` hook that tells the agent to write its handoff at 50 percent. Owner decision 2026-09-27.
- Configuration scope: user settings (`~/.claude/settings.json`) on each machine. The auto-compact window, the status line tap, and both hooks apply to every Claude Code session of the user, manual sessions included. Owner decision 2026-09-27.
- Packaging: `oter/bruh` is a Claude Code plugin marketplace with one plugin, `bruh`. Install with `claude plugin marketplace add oter/bruh` and `claude plugin install bruh@bruh`, at user scope. The plugin ships the skill and both hooks. A plugin cannot set `autoCompactWindow` or `statusLine`, so a setup step writes those two into user settings. Agent-derived, needs owner decision. This replaces hooks written by hand into user settings: a plugin can be turned off or removed in one step.
- Repositories: two. `oter/bruh` is public on GitHub and holds the tool. A separate private repository holds the ledger, because the ledger describes all other repositories. Owner decision 2026-09-27.
- `oter/bruh` on GitHub has no `.github/` directory, so no pipelines run. Agent-derived, needs owner decision.
- Remote transport: Orca remote runtime. `orca serve` on the remote machine on a private network, paired with `orca environment add`, workers started with `worker-start --on <environment>`. Owner decision 2026-09-27.

## Assumptions

- "Workflow" is the Claude Code Workflow tool. Agent-derived, needs owner decision.
- The transport between sessions is Orca orchestration (`orca skills get orchestration`). Orca orchestration has runs, tasks, dispatches, blocking ask and reply, and nested workers. Agent-derived, needs owner decision.
- The ledger of done and in-progress work is Markdown in a separate private repository. The repository is an owner decision 2026-09-27. The Markdown format is agent-derived, needs owner decision.
- `autonomous-agents` solves a different problem: "runs LLM CLI agents on a schedule or a webhook, one Docker container per execution". It is not the transport for this design. It can be a later executor for unattended work. Agent-derived, needs owner decision.

## Knowledge

The verified facts that these decisions depend on are in [knowledge.md](knowledge.md).

## Open questions

1. Compaction mechanism (decided 2026-09-27, option 2). Options, ranked:
   1. Documented only. Set the auto-compact window to 55 percent. Each session keeps a live handoff file and updates it at each checkpoint. A `SessionStart` hook with matcher `compact` injects the handoff file. No session monitors another session.
   2. Option 1, plus a status line tap. The status line writes `used_percentage` to a file per session. A `PostToolUse` hook reads the file and tells the agent to write its handoff at 50 percent.
   3. Custom supervisor. The status line tap from option 2. The supervisor reads the files and types `/compact` into the worker terminal with `orca terminal send`.
2. Configuration scope (decided 2026-09-27: user settings). Options were user settings, or `.claude/settings.local.json` in each project folder. The local file is ignored by git, so a new worktree does not have it.
3. Remote transport (decided 2026-09-27, option 1). Options, ranked:
   1. Orca remote runtime: `orca serve` on the remote machine, reachable over a private network, and `worker-start --on <environment>`. Traffic stays on that network. Questions and results use the same orchestration verbs as local workers.
   2. Claude Code Remote Control: `SendMessage` to a Remote Control session on the other machine. Traffic goes through Anthropic servers. Plain text only, with no task or dispatch tracking.
   3. Custom socket bridge over a private network (SSH or socat forwards a remote inbox socket). Not recommended:
      - `SendMessage` refuses the forwarded socket with `connected endpoint is not the expected process`. Source: `errors.md`, "Refusing to send a cross-session message".
      - Only the auth line of the socket protocol is documented. The message line format is not documented, so the bridge depends on an internal format.
      - A reply address is a local socket path on the sender machine, so replies need a second bridge.
      - A message from a process that is not a child of the session goes through inbound controls. A session that bypasses permission prompts holds it for approval unless `crossSessionInbound` is `accept`.
      - Security: the socket is protected by operating-system user permissions. A TCP forward on the network lets any peer that reaches the port send prompts to the session.
4. Public `bruh` repository and the ledger (decided 2026-09-27, option 1). The ledger records work in all projects. If the ledger lives in a public repository, that record is public. Options, ranked:
   1. Two repositories. `oter/bruh` is public and holds the tool: docs, hooks, status line tap, skills. A separate private repository holds the ledger.
   2. One public repository `bruh`. The ledger stays outside git, on the local disk only. No history and no backup for the ledger.
   3. One public repository `bruh` with the ledger inside. All project status is public.
