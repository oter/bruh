# Plan 7: The implement skillset

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task by task. Steps use checkbox (`- [ ]`) syntax for tracking. Before you edit a workflow script, load the bundled `workflow-authoring` skill.

**Goal:** Ship the skill `/bruh:implement` and its four workflows (`/bruh:tickets`, `/bruh:implement-tickets`, `/bruh:review-and-fix`, `/bruh:review-only`), and the scripts `lane.sh` and `post-findings.sh`. They port the implement skillset of the setup of the owner (spec 6.4) into the public plugin, without its private names, and without a post step inside a workflow (spec 6.1).

**Architecture:** The skill is Markdown: the procedure (rule zero, phases 0 to 5, the lessons, the single-change brief) and four reference files. Each workflow is one self-contained JavaScript file for the Workflow tool. The Workflow runtime has no imports, so each script copies the few helpers that it needs from `deliver.js`. A workflow returns its findings. It never posts. A session posts a saved result with `post-findings.sh`, after a yes of the owner (`--yes`) or under a post grant in `grants.md`. `lane.sh` makes a private copy of the tree for one ticket, writes the patch of the ticket, and applies it back onto the shared tree. Both scripts are POSIX `sh` with `jq`.

**Tech Stack:** JavaScript (Workflow script API), Node.js 22 (`node --test`), POSIX `sh`, `jq`, `rsync`, `git`, `glab` and `gh` (for posts), GitHub Actions.

**Spec:** [../../spec.md](../../spec.md) version 0.5, sections 2, 6.1 to 6.4, 9.2, 13, 17, and 24. Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md), sections 4 and 4a.

## Global Constraints

- `meta` of each workflow is a pure literal. `meta.name` is `tickets`, `implement-tickets`, `review-and-fix`, and `review-only`, so the workflows run as `/bruh:<name>` (workflows.md, "Distribute a workflow in a plugin").
- No `Date.now()`, `Math.random()`, or argless `new Date()` in a script.
- No agent inside a workflow posts outside the project (spec 6.1). No workflow has a Post phase. Each prompt carries the rule.
- Agents never run `git commit`, `git push`, `git stash`, `git checkout`, `git switch`, `git reset`, or `git rebase` (phase 0 of the source skill). Each prompt carries the rule. The owner or the clerk commits.
- A dead agent (an `agent()` call that returns `null`) stops the run with a `FAILED:` line in `deviations`. It never gives `done`, and a dead refuter is never a verdict (deliver.js, final review B1).
- A refuter that does not confirm a finding refutes it. A refuted finding stays refuted in later rounds.
- Invalid `args` stop the run before any agent, with a `STOP:` line.
- Stage efforts (spec 3.3): writers, implementers, and fixers at `high`; reviewers, refuters, merge, and gate agents at `low`. No script sets a model.
- Each path in `args` is absolute and has no single quote and no newline, because prompts quote it for the shell.
- The published files contain no project names, account names, host names, ticket names, GitLab project IDs, or personal paths of the setup of the owner (spec 6.4).

## Review Focus

1. **Two lenses report the same `file:line`.** Expected: one finding, one refuter, and the second problem appended. Test: "review-only: dedup by file:line, one refuter each".
2. **A refuter dies at a usage limit.** Expected: the run returns `stopped` with a `FAILED:` line, and the finding is not in `confirmed` or `refuted` as a verdict. Test: "review-only: a dead refuter stops the run" and "review-and-fix: a dead refuter stops the run, never done".
3. **A merge of parallel lanes applies in the wrong order or applies a failed lane.** Expected: the merge prompt lists the done tickets of the wave in wave order and names the failed lanes as untouched. Test: "implement-tickets: lanes merge in wave order and skip failed lanes".
4. **A post goes out without the yes of the owner.** Expected: `post-findings.sh` refuses without `--yes` and without a post grant row for the repository and the role key. A merge grant row does not count. Test: tests/test.sh "post-findings refuses without --yes or a post grant".
5. **A rerun after a partial failure posts the same comment twice.** Expected: a body that is already on the pull request is not posted again. Test: tests/test.sh "post-findings rerun posts nothing new".

---

### Task 1: The plan

**Files:** Create: `docs/superpowers/plans/2026-09-30-plan-7-implement-skillset.md`.

- [ ] **Step 1: Commit** `Add plan 7, the implement skillset`.

### Task 2: The workflow tests and the four workflows

**Files:**

- Create: `plugins/bruh/workflows/implement.test.mjs`, `tickets.js`, `implement-tickets.js`, `review-and-fix.js`, `review-only.js`
- Modify: `.github/workflows/ci.yml` (the `node` job runs the new test file)

**Interfaces:**

The harness of `deliver.test.mjs`: load a script, replace `export const meta` with `const meta`, and build an `AsyncFunction` with the parameters `agent, parallel, pipeline, phase, log, args, budget`. The `agent` stub answers by the first word of `opts.label`.

`/bruh:tickets` (`tickets.js`):

- `args`: `root`, `spec`, `issues`, `guides` (absolute paths), `rules` (the text of the ticket rules: `references/ticket-template.md` plus the project rules), `ground` (optional list of paths), `amendments` (optional text), `round_cap` (default 5).
- Flow: one writer (label `write`) writes the ticket files and `INDEX.md` with the waves. Then up to `round_cap` rounds: one reviewer (`review <n>`), and when it does not pass and the cap is not reached, one fixer (`fix <n>`).
- Result: `{status: done | findings_left | stopped, tickets: [...], notes, issues: [{ticket, problem, fix}], rounds, deviations}`. `issues` are the open issues of the last review.

`/bruh:implement-tickets` (`implement-tickets.js`):

- `args`: `root`, `spec`, `guides`, `issues`, `lane` (the absolute path of `lane.sh`), `waves` (a list of lists of ticket file names, in order), `gates` (a list of gate commands), `fix_cap` (default 3).
- Flow per wave: one implementer for each ticket (`impl <id>`), in the shared tree for a wave of one, and in a lane (`lane.sh start <id>`) for a wave of more than one. A reviewer (`review <id>`) runs the verify commands and flips `Status:` to `done` on a pass; a fixer (`fix <id>`) gets its findings, up to `fix_cap` fixes. After a wave with lanes, one merge agent (`merge <wave>`) runs `lane.sh patch`, `apply`, and `clean` for each done ticket, in wave order, and leaves the other lanes untouched. After the last wave, one gate agent (`gate`) runs `args.gates`.
- Result: `{status: done | findings_left | stopped, tickets: [{id, file, status: done | failed | conflict | error, workdir, rounds, findings, deviations}], gate: null | {ok, gates: [{command, exit_code, output_tail}]}, deviations}`.

`/bruh:review-and-fix` (`review-and-fix.js`):

- `args`: `root`, `base` (40 hex), `head` (40 hex, the commit of `root` at the start), `spec` (optional), `guides`, `lenses` (`[{key, prompt}]`), `deliberate` (list), `house_rules` (optional text), `gates` (list), `round_cap` (default 2).
- Flow per round: in parallel, one reviewer for each lens (`review <key>`) and one gate agent (`gate <n>`). Deduplicate the lens findings by `file:line`, drop refuted keys, and give each finding one refuter (`verify <file>:<line>`). A failed gate is an open finding without a refuter. No open finding: `done`. The round cap reached: `findings_left`. Otherwise fix the open findings by area, one fixer (`fix <area>`) at a time, and review again.
- Result: `{status, base, head_sha, summaries: [{key, summary}], confirmed: [finding], refuted: [finding], deviations}`. A finding is `{file, line, lens, rule, severity, problem, fix, round, state, reason}`. In `confirmed`, `state` is `fixed` or `open`.

`/bruh:review-only` (`review-only.js`):

- `args`: `root` (a detached, clean worktree at the head), `base`, `head` (40 hex), `spec` (optional), `guides`, `lenses`, `deliberate`, `house_rules` (optional).
- Flow: one review round and one refuter for each unique finding. No edits, no gate, no fix.
- Result: `{status: done | stopped, base, head_sha, summaries, confirmed, refuted, deviations}`. `confirmed` is sorted by severity: security, bug, guideline, nit.

- [ ] **Step 1:** Write `implement.test.mjs` with the Review Focus tests and: `meta.name` of each script and `meta` a literal; invalid `args` stop before any agent; every prompt has the git rule and the post rule and no prompt tells an agent to post; no script has a `Post` phase; efforts; round caps from `args` and their defaults; a dead agent of each stage stops the run; sequential fix batches; the waves stop after a failed wave.
- [ ] **Step 2:** Run `node --test plugins/bruh/workflows/implement.test.mjs`. Expected: FAIL, because the scripts do not exist.
- [ ] **Step 3:** Write the four scripts. Run the tests. Expected: PASS.
- [ ] **Step 4:** Add the file to the `node` job of `ci.yml`.
- [ ] **Step 5: Commit** `Add the tickets, implement-tickets, review-and-fix, and review-only workflows`.

### Task 3: `lane.sh` and `post-findings.sh`

**Files:**

- Create: `plugins/bruh/scripts/lane.sh`, `plugins/bruh/scripts/post-findings.sh`
- Modify: `tests/test.sh`

**Interfaces:**

- `ROOT=<repo> [LANES=<folder>] [EXCLUDES="<top-level paths>"] sh lane.sh start|patch|apply|clean <ticket id>`. `start` prints the path of the lane. The default `LANES` is `${TMPDIR:-/tmp}/bruh-lanes`. The default `EXCLUDES` is `.scratch/`. Each exclude is anchored at the top of the tree, so `.bin/` does not drop `node_modules/.bin` (lesson of the second run). A ticket ID is letters, digits, `.`, `_`, and `-`, and does not start with `.` or `-`.
- `sh post-findings.sh [--dry-run] [--yes] [--data <folder>] gitlab|github <repo> <number> <result.json>`. It reads a result of `/bruh:review-only` or `/bruh:review-and-fix`. It posts one summary note, one comment for each confirmed finding (inline on its line when it can, else a general comment that names `file:line`), and for a result with fix states, one note of the fixes. Each body starts with the marker line `<!-- bruh:<role key> -->` (`<!-- bruh:owner -->` when `BRUH_ROLE_KEY` is not set) and the words "Agent review", and it ends with the same marker line. A body already on the pull request is not posted again. Without `--dry-run`, it refuses (exit 3) unless it has `--yes` or a row of the section "Post grants" of `grants.md` names the repository and `BRUH_ROLE_KEY`. It finds `grants.md` through `ledger_path` in `<data>/init/config.json`, as `merge-train.sh` does. It refuses a result whose `status` is not `done` or `findings_left`.

- [ ] **Step 1:** Write the shell tests with a fake `glab` and a fake `gh` on `PATH`: lane start, patch, apply (new, changed, and deleted files; the index of the shared tree stays clean; a linked worktree works), clean, and a bad ticket ID; post-findings dry run, refusal, `--yes`, post grant, merge grant row that does not count, marker, idempotence, fallback to a general comment, a stale head, both hosts, and a `stopped` result. Run `sh tests/test.sh`. Expected: FAIL.
- [ ] **Step 2:** Write the scripts. Run the tests. Expected: PASS. Run shellcheck.
- [ ] **Step 3: Commit** `Add lane.sh and post-findings.sh`.

### Task 4: The skill, the references, and the agent text

**Files:**

- Create: `plugins/bruh/skills/implement/SKILL.md`, `references/lessons.md`, `references/ticket-template.md`, `references/example-args.md`, `references/guide-sources.md`
- Modify: `plugins/bruh/agents/clerk.md`, `plugins/bruh/ledger-template/grants.md`, `plugins/bruh/agents/agents_test.mjs`

- [ ] **Step 1:** Write the skill in ASD-STE100: rule zero, phases 0 to 5, the post step, the lessons, the single-change brief, and when not to use the full pipeline. The scripts are named with `${CLAUDE_PLUGIN_ROOT}/scripts/`.
- [ ] **Step 2:** Add to `clerk.md` the rule for the implement workflows and for posts. Add the "Post grants" table to `grants.md`. Add a structural test that the clerk names `post-findings.sh` and the post grant, and that `grants.md` has the section.
- [ ] **Step 3: Commit** `Add the implement skill and the post rule of the clerk`.

### Task 5: Docs, probe, leak scrub

**Files:** Modify: `README.md`, `CHANGELOG.md`, this plan.

- [ ] **Step 1:** README section "Implement and review", CHANGELOG Unreleased.
- [ ] **Step 2:** Live probe (below). Record it here.
- [ ] **Step 3:** Leak scrub of the diff. Run every check of the lane rules.
- [ ] **Step 4: Commit** `Document the implement skillset`.

## Deviations from the specification

1. **Marker position.** The controller brief says that each posted body starts with the marker line. Interfaces section 4a says that each post ends with it, and the watcher (`mcp/watch.go`, `agentMark`) reads only the last non-empty line. Each body of `post-findings.sh` has the marker as its first line and as its last line, so both hold. The contract is not changed.
2. **`<!-- bruh:owner -->`.** `owner` is not a role key, so the watcher counts a post with this marker as a human post (`ParseRoleKey` fails). This is correct for a post from a manual session of the owner, which is under the account of the owner. It is reported, not changed.
3. **GitLab.** The watcher and `repos_set` know `github` and `gitea`. `post-findings.sh` supports `gitlab` (as the source) and `github`. It does not support `gitea` (decision 5).
4. **No questions during a run in the four workflows.** Spec 6.2 gives workflows a question path, and `deliver` has it. The source workflows had none: the rule of the source is "a conflict is reported, not guessed", and the ticket pipeline starts only from a settled spec with no open questions. An agent of these workflows that needs a decision returns a `conflict`, and the run returns `stopped` with a `CONFLICT:` line (decision 3).

## Decisions

Each decision is agent-derived, needs owner decision. The first option of each list is the chosen one.

1. **The `description` of the skill.** The trigger of the owner was "ANY edit to a repo". In a public plugin, that loads the skill in every session of every user, for each edit. Options, ranked: (a) a narrow description: the skill loads when the user runs `/bruh:implement`, asks to implement a settled spec with tickets and waves, or asks for a bruh review of a pull request or merge request; a clerk can still load it with the Skill tool; (b) `disable-model-invocation: true`: only a typed `/bruh:implement` loads it, so a clerk that reads its start message from the mailbox cannot load it; (c) the owner trigger, "any edit to a repo".
2. **The review base.** Options, ranked: (a) `base` and `head` are 40-hex SHAs in `args`; reviews diff against the pinned base, never a moving ref (spec 6.3); (b) a ref such as `origin/main`, as the source default.
3. **Questions in the four workflows.** Options, ranked: (a) none; a conflict stops the run with `CONFLICT:` (Deviation 4); (b) copy the question path of `deliver` into each workflow.
4. **Gate failures in `review-and-fix`.** Options, ranked: (a) a gate command that does not exit 0 is an open finding of its round without a refuter, because an exit code is a source read; it goes to the fixer in the area `gates`; (b) a refuter runs the gate again, as in `deliver`.
5. **Code hosts of `post-findings.sh`.** Options, ranked: (a) GitLab through `glab` and GitHub through `gh`; (b) also Gitea, through its REST API with `curl` and a token (the Gitea API has issue comments and pull request reviews with inline comments, but no command-line tool that bruh already uses); (c) GitHub only.
6. **The post gate.** Options, ranked: (a) `--yes` (the owner said yes) or a row in the section "Post grants" of `grants.md` that names the repository and the role key; a speed bump (principle 2) in `sh`, in the style of the merge gate; (b) the gate in the Go MCP server, as `merge-train`; (c) no gate, only agent text.
7. **Inline comments after the first round.** The line numbers of a finding of round 2 or later of `review-and-fix` are in the fixed working tree, not in the pushed head. Options, ranked: (a) only findings of round 1 go inline; later findings are general comments that name `file:line`; (b) all inline.
8. **Ticket ID.** Options, ranked: (a) the file name of the ticket without `.md`, because it is unique and safe for a lane path; (b) the first two parts of the file name, as the source (`mr2-63`).
9. **Models.** Options, ranked: (a) no script sets a model; every agent inherits the model of the session (spec 3.3: a role never changes its model; the workflow-authoring guidance); stage efforts are fixed; (b) `impl` and `review` model overrides in `args`, as the source.
10. **Allow rules.** The init skill adds `Workflow(bruh:deliver)` only (spec 6.3). Options, ranked: (a) no new allow rule; the clerk runs an implement workflow only when its start message names it, and a refusal is a P0; (b) init adds `Workflow(bruh:<name>)` for the four workflows.
11. **Lane defaults.** Options, ranked: (a) `LANES` is `${TMPDIR:-/tmp}/bruh-lanes`, and `EXCLUDES` is `.scratch/`, anchored at the top of the tree; (b) the source defaults (`/tmp/lanes`, `.scratch/ .bin/ .reports/`, not anchored).

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| Script API: `agent(prompt, {label, phase, schema, model, effort, isolation, agentType})` returns `null` for a dead agent; `parallel` maps a thrown thunk to `null`; `meta` is a pure literal | Loaded the bundled skill | `/workflow-authoring` |
| A plugin workflow runs as `/<plugin>:<meta.name>`; `args` reaches the script as structured data | Read the page | <https://code.claude.com/docs/en/workflows.md>, "Distribute a workflow in a plugin", "Pass input to a saved workflow" |
| Claude Code substitutes `${CLAUDE_PLUGIN_ROOT}` in the Markdown content of a plugin skill | Read the page | <https://code.claude.com/docs/en/skills.md>, "Available string substitutions" |
| `disable-model-invocation: true` keeps the description out of context and stops Claude from loading the skill | Read the page | skills.md, "Control who invokes a skill" |
| The combined `description` and `when_to_use` is cut at 1,536 characters in the skill listing | Read the page | skills.md, "Frontmatter reference" |
| The watcher reads the marker only from the last non-empty line of a body, and accepts only a valid role key | Read the code | `plugins/bruh/mcp/watch.go`, `agentMark` |
| `merge-train` finds `grants.md` through `ledger_path` in `<data>/init/config.json` | Read the code | `plugins/bruh/mcp/mergetrain.go`, `mergeGate` |

## Probe

Pending (Task 5).
