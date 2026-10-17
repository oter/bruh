# bruh specification

Status: the specification of the bruh plugin 0.13.0 only: the implement skill, the five workflows, the house rules, the results, and the run without the coordinator. It is a copy of these sections of version 0.6, and each bullet keeps its tag. The plugin split ([docs/design/plugin-split.md](design/plugin-split.md)) moved the roles, the ledger, the MCP server, the hooks, and the init skill to the coordinator plugin.

The roles are specified in <https://github.com/oter/coordinator/blob/main/docs/spec.md>.

The section numbers are those of version 0.6, so that the references of the skill and the workflows stay correct. The other sections are in the coordinator spec. A reference to the principles (section 2) or to another section that is not in this file goes to the coordinator spec. Each decision in this file has one tag:

- "Owner decision <date>": the owner made the decision.
- "Agent-derived, accepted 2026-09-30": the agent proposed it, and the owner accepted all such items as a set on 2026-09-30, to test them in practice.
- "Agent-derived, accepted 2026-10-03": the agent proposed it, and the owner accepted all such items of version 0.6 as a set on 2026-10-03 ("Accept all, build now").
- "Agent-derived, needs owner decision": the agent proposed it, and the owner has not decided it.
- "bigm decision <date>" and "Clanker decision <date>": a role decided it under a rule of the owner.
- "Verify": a fact that the implementation must prove before it depends on it.

## 6. Workflows

### 6.1 General

- A workflow is a script for the Claude Code `Workflow` tool. Owner decision 2026-09-27.
- The plugin ships its workflows in `plugins/bruh/workflows/`. They run as `/bruh:<name>`. Agent-derived, accepted 2026-09-30. Verify: the Workflow tool accepts a plugin workflow, because it refuses a `scriptPath` outside the working directory.
- The clerk starts a workflow with a slash command in its start message. In the setup of the owner, a workflow needed the typed opt-in of the owner in each session, and a relayed opt-in did not count. Verify: a clerk started by a script can launch `/bruh:deliver`, and the allow rule `Workflow(bruh:deliver)` matches a namespaced plugin workflow. Agent-derived, accepted 2026-09-30.
- The script cannot write files. Agents and MCP tools do all writes.
- No agent inside a workflow posts outside the project, for example comments on a pull request. In the setup of the owner, an in-workflow post agent refused twice because it saw the latest chat message of the owner. Posting is clerk work and is outward-facing. Agent-derived, accepted 2026-09-30. A bug report on the bruh repository is not a post of project work: the session that got the yes of the owner files it (principle 8 of the [coordinator spec](https://github.com/oter/coordinator/blob/main/docs/spec.md)). Agent-derived, needs owner decision.

### 6.2 Questions during a run

- A workflow can get input while it runs. Owner decision 2026-09-27.
- An agent with a question sends it to `main`, which is the clerk, with a header of section 5 of the [coordinator spec](https://github.com/oter/coordinator/blob/main/docs/spec.md), then calls the MCP tool `answer_wait`. The clerk answers it, or routes it through its clanker to bigm. The clerk writes the answer with `answer_write`. The two probes of 2026-09-27 proved this path in the same run. Owner decision 2026-09-29: plugin code writes the answer file. The tool names are agent-derived, accepted 2026-09-30.
- Fallback after `pending`: the workflow returns the question and ends. After the answer, the clerk launches it again with `resumeFromRunId`. The answer goes only into the prompts of the agents after the question, so the earlier agents return cached results. An agent whose prompt contains changed data runs again. Agent-derived, accepted 2026-09-30.
- Exception: a stop with `CONFLICT:` is cached too, so a resume replays it. After the answer, the clerk starts a new run without `resumeFromRunId` (section 15.1.4 of the [coordinator spec](https://github.com/oter/coordinator/blob/main/docs/spec.md)). Agent-derived, needs owner decision.

### 6.3 The `deliver` workflow

Agent-derived, accepted 2026-09-30, except where tagged. `deliver` adapts two scripts of the setup of the owner: `implement-tickets.js` for stages 1 and 2, and `review-and-fix.js` for stages 3 and 4.

Inputs in `args`: the task, the acceptance criteria, the base SHA, the gate commands of the project, the house rules text (optional), the guides folder of the task (an absolute path with `INDEX.md`, or empty), the deliberate choices that reviewers must not flag, the answer folder and deadline, and the review-round cap.

- When the house rules text is missing or empty, `deliver`, `review-and-fix`, and `review-only` use the bundled text of `defaults/house-rules.md`. A passed text wins. Agent-derived, needs owner decision (plugin-split line b). Each of the three scripts embeds a copy of the file, because the Workflow runtime has no imports and no filesystem, and a test checks each copy byte for byte against the file. Clanker decision 2026-10-10 (Q-bruh-maksyms-macbook-pro-296).

1. **Plan.** Read the task, the code, and the guides of the project. Pin the base SHA. Write a plan with written STOP conditions for each step. Send each open question through section 6.2.
2. **Implement.** Implement the plan. Record deviations from the plan, and stop at a conflict instead of guessing.
3. **Review.** Four checks in parallel, each with the house rules in its prompt:
   - an adversarial reviewer that tries to refute the change;
   - an independent checker that verifies invariants against the code;
   - a simplicity reviewer that finds where the change is more complex than the task needs: an abstraction that the task did not ask for, code for a speculative need, a re-implemented standard library function or existing helper, an unneeded dependency, scaffolding for later, and a diff that is longer than the problem needs. Each finding has a file, a line, and a summary with a concrete simpler replacement, and goes through the same refuter and fix loop. Owner decision 2026-10-09 (Q-219);
   - build, test, and lint with the gate commands. This check records how many tests ran, passed, failed, and were skipped. A skipped test in a required suite is a finding. The gates check gets the head SHA from the implement or fix step that made the last commit, runs each gate alone as a single plain Bash call, and runs no git; the implement prompt lists the gates, and the implement and fix steps read the head SHA with a plain single `git rev-parse HEAD` call, never chained. Owner decision 2026-10-09 (Q-207). No gates script; each gate keeps its own ran, passed, failed, and skipped (bigm decision 2026-10-09 under R-4, Q-bruh-maksyms-macbook-pro-256). A Go test gate whose output has no test count (`go test` without `-v` prints only `ok <pkg>` or `ok <pkg> (cached)`) is counted with a second plain run with `-count=1 -v`; the gate list of the project does not change. Owner decision 2026-10-09 (task 49, bigm under R-4).

   Each reviewer and each refuter diffs against the pinned base SHA, never against `origin/main`. Findings are deduplicated by `file:line`. One refuter checks each finding. A finding that the refuter does not confirm counts as refuted, and a refuted finding stays refuted.
4. **Fix.** Fix the confirmed findings in batches by area, one batch at a time, because parallel fixers in one tree collide. Then review again.

- The review-round cap is a runtime setting with default 2. If findings are left after the last round, the run stops and the clerk raises a P1. Owner decision 2026-09-27 (two rounds) and 2026-09-29 (runtime setting).
- At the round cap, a gate finding that its refuter refuted stays refuted; a real gate failure (exit not 0, a failed or skipped test) stays open. Owner decision 2026-10-09 (task 49, bigm under R-4).
- At the cap the status stays `findings_left` while a gate is not clean, even when every gate finding is refuted: agent-derived, needs owner decision.
- A simplicity finding never overrides the acceptance criteria or a deliberate choice: something that the task asks for is not over-engineering. The reviewer prompt and `skills/implement/references/simplicity.md` carry this rule. Owner decision 2026-10-09 (Q-219).
- The simplicity reviewer puts the simpler replacement at the end of the summary and uses the same findings schema as the other reviewers, so the dedupe, the refuter, and the fixer stay unchanged: agent-derived, needs owner decision. Its findings come last in the dedupe order, so the adversarial reviewer and the invariant checker keep their summary for the same `file:line`: agent-derived, needs owner decision. The plan and implement prompts get no simplicity rule; only the review checks it: agent-derived, needs owner decision.
- The guides folder: when `args.guides` is an absolute folder, the three reviewers and every refuter get the rule "GUIDES ARE MANDATORY": read `INDEX.md` and the guides that it names for the files of the diff, and cite for each finding a guide file and rule, a house rule, or a concrete failing scenario; the accepted deviations of `INDEX.md` are not findings. The review lenses stay, and no other step gets the rule. Premise changed 2026-10-09 (task 45): "both reviewers" and "the 2 lenses" become the three reviewers, because the simplicity reviewer joined; it gets the rule like the other two. An empty `args.guides` gives the prompts of before. The clerk builds the folder for each task: it picks the guides from `skills/implement/references/guide-sources.md` by the file types of the task, fetches each one fresh into `/tmp/<clerk key>-guides/`, and writes `INDEX.md` with the file globs and the source URL of each guide. No guide file ships in the plugin. Owner decision 2026-10-06 (task 36, reuse the guides folder). A house rule as a citation, and a stop of the run when `args.guides` is not empty and not an absolute path: agent-derived, needs owner decision.
- A failed step that costs money or cannot be undone is never retried automatically.
- Each finding gets a stable ID (`F<n>`) when it first enters the findings of the run, and keeps it in later rounds. The fixer gets each finding as `[F<n>] file:line`, reports the IDs that it fixed, and the run closes the findings of the batch by ID. `file:line` stays for display and for the dedupe of new findings. `/bruh:review-and-fix` does the same, and its fix check reports by ID too. The result keeps its fields, with no ID. bigm decision 2026-10-06T17:14Z (task 32, R-4), after a run ended `findings_left` because a fix moved the line of its finding.

### 6.4 The implement skillset

Owner decision 2026-09-30: the plugin includes the bruh-implement skillset of the setup of the owner, as the skill `/bruh:implement` and its workflows. The owner used it for every code change and every review in the field run. The details below are agent-derived, accepted 2026-10-03, except where tagged.

- The skill is the procedure of rule zero: the orchestrating session never edits code. Each change is a brief to an implementer agent and a review by a separate agent with the review guides, in a fix loop. For a settled spec with many changes, the pipeline is: review guides, function-sized TDD tickets, execution waves, lanes, a whole-branch gate, and a multi-lens review whose findings are refuted before they count.
- Workflows: `/bruh:tickets` (ticket breakdown), `/bruh:implement-tickets` (waves and lanes), `/bruh:review-and-fix` (review, refute, and fix of the own change), and `/bruh:review-only` (review and refute of a change that bruh did not write). Each takes its paths in `args`.
- Scripts: `scripts/lane.sh` (a private lane copy of the tree for a ticket, its patch, and its merge back). Premise changed 2026-10-06 (task 20): the post script is removed. Owner decision 2026-10-06.
- No workflow posts outside the project (section 6.1). A workflow returns its findings. A session saves the result with the MCP tool `result_save` and posts it with plain `gh pr review` or `glab mr note create` after the owner approved the post (an `ANSWER` of bigm with the header `ANSWER <id>: post <owner/repo>#<number> at <head SHA> approved`, where `<id>` is the question ID of section 5 of the coordinator spec, or a yes in the terminal of a manual session), or under a post grant in `grants.md`. A post goes out under an account of the owner, so it is on the never-without-the-owner list (section 13 of the coordinator spec). Each posted body ends with the structural marker line of section 9.2 of the coordinator spec.
- The example lenses of `skills/implement/references/example-args.md` include a `simplicity` lens for `/bruh:review-and-fix` and `/bruh:review-only`. Its guide is `skills/implement/references/simplicity.md`: the ladder and the rules in bruh's own words, so that it works without the ponytail plugin, the source of the idea. `guide-sources.md` gets YAGNI and KISS rows under "Design principles". Owner decision 2026-10-09 (Q-219).
- In a role session, `/bruh:implement` runs inside a clerk. In a manual session of the owner, the owner is the orchestrator and answers each ask in the terminal.
- The published files contain no project names, account names, host names, or ticket names of the setup of the owner.
- The clerk checks the cover of a post itself: the row of "Post grants" in `grants.md`, or the `ANSWER` of bigm for the `head_sha` of the result, and the identity of an SSH host alias (section 8.5 of the coordinator spec). These checks are prose of the clerk, with no mechanical stop. bigm decision 2026-10-06 (Q-bruh-maksyms-macbook-pro-135), agent-derived, needs owner decision.

## 10. Files

### 10.1 Results without the coordinator

The skills and the workflows run without the coordinator (`docs/design/plugin-split.md`, task 55):

- The workflow agents find `question_open`, `answer_wait`, and `report_write` with ToolSearch by short name, so any plugin prefix works, and use them when they are there. The session finds `result_save` the same way. Owner decision 2026-10-09 (Q-240).
- Without `question_open`, the agent does not wait: it stops and returns the question with an empty id, its options ranked, and its recommendation. The run returns `status: question` with the id `chat:<step>` (`chat:<label>`, or `chat:fix <area> round <n>` for a fix step). The session asks it in chat, adds the answer to `args.answers` under that id, and relaunches with `resumeFromRunId` (section 6.2). Owner decision 2026-10-09 (Q-240): ask in chat. The id `chat:<step>` and the round in the id of a fix step: clanker decision 2026-10-10 (Q-bruh-maksyms-macbook-pro-297).
- Without `report_write`, the agent skips the phase line and goes on. Owner decision 2026-10-09 (Q-240).
- Without `result_save`, the session writes the result object with the Write tool to `.bruh/results/<name>.json` in the root of the run (`args.root`; for `deliver`, the working tree), and the project ignores `.bruh/` in its `.gitignore`. Principle 3 of the coordinator spec holds when the coordinator is there. Agent-derived, needs owner decision (plugin-split line a).

## 17. Plugin layout

Owner decision 2026-09-27: `oter/bruh` is a plugin marketplace with one plugin, `bruh`, installed at user scope. The layout is agent-derived, accepted 2026-09-30. Owner decision 2026-10-09 (Q-239): the plugin `bruh` keeps only the files below (plugin-split decision 4).

```text
.claude-plugin/marketplace.json
plugins/bruh/.claude-plugin/plugin.json
plugins/bruh/skills/implement/SKILL.md
plugins/bruh/skills/implement/references/
plugins/bruh/workflows/deliver.js
plugins/bruh/workflows/tickets.js
plugins/bruh/workflows/implement-tickets.js
plugins/bruh/workflows/review-and-fix.js
plugins/bruh/workflows/review-only.js
plugins/bruh/scripts/lane.sh
plugins/bruh/defaults/house-rules.md
```
