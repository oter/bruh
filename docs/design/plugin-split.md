# Plugin split: bruh and coordinator

Status: design confirmed by the owner (Q-251, 2026-10-09). This doc has no code. Later steps make the change.

The owner said on 2026-10-09 at 16:46Z: "bruh pllugin must be break down into two parts - bruh skills - my own skills for code review, for building, for breaking down the tickets, for communication - this is a bruh part. AND coordinator - jsut behavioural thing - the concept of coordinator (bigm), clanker, clerk. and the clerks may run tickets breakdown, building, making code review workflows."

## Decisions

1. Names: the plugin `bruh` holds the skills, and the plugin `coordinator` holds the roles. The agents become `coordinator:bigm`, `coordinator:clanker`, and `coordinator:clerk`. Owner decision 2026-10-09 (Q-237).
2. Two repos. `oter/bruh` stays (public, history, issues, marketplace `oter`) and holds the bruh plugin. A new public repo `oter/coordinator` holds the coordinator plugin. One commit copies its files. `gh issue transfer` moves the open coordinator issues. Owner decision 2026-10-09 (Q-238, Q-242, Q-243).
3. One marketplace `oter` in `oter/bruh` lists both plugins: bruh from `./plugins/bruh`, coordinator from the GitHub repo `oter/coordinator`. Owner decision 2026-10-09 (Q-244).
4. bruh holds the implement skill and its references (with `simplicity.md`), the workflows deliver, tickets, implement-tickets, review-and-fix, and review-only, `house-rules.md`, `lane.sh`, and the bro skill. coordinator holds the agents bigm, clanker, clerk, and learner, the MCP server, the hooks, the other scripts, the Slack channel, the ledger template, priorities, role settings, bug reports, monitors, and the init skill. Owner decision 2026-10-09 (Q-239).
5. The skills run without the coordinator. The coordinator calls a skill by name only (a START names `/bruh:implement` or `Workflow(bruh:deliver)`), so any skill plugin can plug in. The workflows find the coordinator tools with ToolSearch by short name (`question_open`, `answer_wait`, `report_write`, `result_save`) and use them when present. Without them, the workflows ask in chat and write the result to a file. Owner decision 2026-10-09 (Q-240).
6. Switch: a quiet switch after the open tasks finish. One migration step copies `~/.claude/plugins/data/bruh-oter` to `coordinator-oter`. The owner installs `coordinator@oter`, updates bruh, and restarts bigm. The ledger uses the `coordinator:` agent names. Owner decision 2026-10-09 (Q-245, Q-251).
7. Versions: coordinator 0.13.0 and bruh 0.13.0, with a plain update and no reinstall. The owner said: "b. bruh 0.13.0, the same as coordinator, with a plain update". This replaces "bruh 0.9.0" of the Q-245 answer. Owner decision 2026-10-09 (Q-252).

## Agent-derived

- a. Without the coordinator, the result file is `.bruh/results/<name>.json`, and git ignores it. Agent-derived, needs owner decision.
- b. The house rules ship with bruh, and deliver uses them by default. The coordinator stops pasting them into each START. Agent-derived, needs owner decision.
- c. bro moves into bruh, and a bruh SessionStart hook loads it. The bro line in the global `CLAUDE.md` of the owner then points to the plugin skill. The owner edits that file; no agent does. Agent-derived, needs owner decision.

## Changes to the spec

Later steps change these parts of [the spec](../spec.md). Each is a change of the spec, not a conflict:

- Section 17: the marketplace has two plugins, not one (decision 3).
- Sections 3, 3.4, and 20: the agent names change from `bruh:` to `coordinator:` (decision 1).
- Section 10.1: a project without the coordinator also has `.bruh/results/` (line a).
- Section 6.3: deliver uses the house rules of bruh by default (line b).
- Section 7: the status line tap matches `data/coordinator-<id>` (decision 6).
- Sections 16 and 18: init becomes `/coordinator:init`, and the install adds `coordinator@oter` (decisions 3, 4, and 6).
- Section 3.4: the Skill hook of the ledger also passes `coordinator:` skills, so `/coordinator:init` runs. Agent-derived, needs owner decision.

## Work plan

1. This doc.
2. Create `oter/coordinator` (after the owner approves the creation) and move the coordinator files.
3. The standalone fallback in the workflows, and the move of bro.
4. The migration step and the marketplace entry.
5. Release both at 0.13.0, then the quiet switch.

In-flight tasks 46 (#50), 48, and 41 part 2 land in `plugins/bruh` first and move with step 2. Each step is a PR. Merges wait for the owner.
