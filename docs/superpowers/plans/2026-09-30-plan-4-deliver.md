# Plan 4: The `deliver` workflow

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task by task. Steps use checkbox (`- [ ]`) syntax for tracking. Before you edit the script, load the bundled `workflow-authoring` skill.

**Goal:** Ship `/bruh:deliver`, the Workflow script that a clerk runs for its task: plan, implement, review in three parallel checks, refute each finding, fix in sequential batches, and review again up to the round cap, with the question path of spec 6.2 that keeps the cached results on a relaunch.

**Architecture:** One plain JavaScript file, `plugins/bruh/workflows/deliver.js`, for the Claude Code Workflow tool. The script has no filesystem access and no clock. Agents do all reads and writes, and they call the bruh MCP tools. The script keeps the control flow: the state of each finding, the refuted set, the round count, and the result object of interfaces section 4. A `node --test` harness loads the script with stub globals (`agent`, `parallel`, `pipeline`, `phase`, `log`, `args`, `budget`) and checks the control flow without a model.

**Tech Stack:** JavaScript (Workflow script API), Node.js 22 (`node --test`, standard library only), GitHub Actions.

**Spec:** [../../spec.md](../../spec.md) version 0.4, section 6 (and 3.3, 15). Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md), section 4.

## Global Constraints

- `meta` is a pure literal with `name: 'deliver'`, so the workflow runs as `/bruh:deliver` (workflows.md, "Distribute a workflow in a plugin").
- No `Date.now()`, `Math.random()`, or argless `new Date()`: they throw in a Workflow script, because a relaunch must repeat the same `agent()` calls.
- Stage efforts (spec 3.3): plan, implement, and fix agents at `high`; reviewers, the gate check, and refuters at `low`.
- Every review diffs against `args.base_sha` with `git diff <base_sha> HEAD`, never against `origin/main` (spec 6.3).
- No agent posts outside the project (spec 6.1) and no agent pushes: pushes are clerk work (spec 3.6).
- The prompts of the agents before a question never contain an answer, so a relaunch with `resumeFromRunId` returns their cached results (spec 6.2).
- The result object has exactly the keys of interfaces section 4.

## Review Focus

1. **Two reviewers report the same `file:line`.** Expected: one finding and one refuter. Test: "dedup by file:line".
2. **A refuted finding comes back in the next round.** Expected: no second refuter; it stays `refuted`. Test: "refuted findings stay refuted".
3. **Findings stay after the last round.** Expected: `status: findings_left` after `round_cap` review rounds, with the open findings. Test: "round cap stops with findings_left".
4. **A question has no answer at the deadline, and the clerk relaunches after the answer.** Expected: `status: question` with the question; on the relaunch the earlier prompts are the same, and only the prompt after the question contains the answer. Test: "question path returns status question" and "answer reaches only prompts after the question".
5. **A required test is skipped.** Expected: a finding. Test: "a skipped required test becomes a finding".

---

### Task 1: The test harness and the first failing tests

**Files:**

- Create: `plugins/bruh/workflows/deliver.test.mjs`
- Modify: `.github/workflows/ci.yml` (the `node` job of plan 3 runs this file too)

**Interfaces:**

- Consumes: `plugins/bruh/workflows/deliver.js`.
- Harness: read the script text, replace `export const meta` with `const meta`, and build an `AsyncFunction` with the parameters `agent, parallel, pipeline, phase, log, args, budget`. The body can then use top-level `await` and `return`, as in the Workflow runtime. The `agent` stub records `{prompt, opts}` and answers from a scenario keyed by the first word of `opts.label` (`plan`, `implement`, `adversarial`, `invariants`, `gates`, `refute`, `fix`). `parallel` runs the thunks with `Promise.all` and maps a thrown thunk to `null`, as the runtime does.

- [ ] **Step 1:** Write the harness and the tests of the Review Focus, plus: `meta.name` is `deliver`; invalid `args` stop the run before any agent; reviewers and refuters run at low effort, implementers at high effort; a refuter that returns `null` counts as refuted; the same question pending again after its answer stops the run.
- [ ] **Step 2:** Run `node --test plugins/bruh/workflows/deliver.test.mjs`. Expected: FAIL, because `deliver.js` does not exist.
- [ ] **Step 3: Commit** `Add the test harness of the deliver workflow`.

### Task 2: The `deliver` script

**Files:**

- Create: `plugins/bruh/workflows/deliver.js`

**Interfaces:**

- `args`: interfaces section 4, plus the optional key `answers` (an object from question ID to answer text; see "Deviations").
- Result: interfaces section 4.
- Agent result schemas:
  - plan: `{plan, stop, reason, question?}`
  - implement: `{head_sha, deviations[], conflict, question?}`
  - adversarial and invariants: `{findings[{file, line, summary}]}`
  - gates: `{head_sha, tests{ran, passed, failed, skipped}, gates[{command, exit_code, ran, passed, failed, skipped, problems[{file, line, summary, kind}]}]}`; `kind` is `failed` or `skipped`
  - refute: `{confirmed, reason}`
  - fix: `{head_sha, fixed[{file, line}], deviations[], question?}`
  - `question`: `{id, header, body}`, set only when `answer_wait` returned `pending`

Control flow:

1. Check `args`: `task` not empty, `base_sha` 40 hex characters, `branch` not empty, `gates` a list. Otherwise return `stopped` with the reason in `deviations`.
2. **Plan** (phase `Plan`): one agent reads the task, the code, and the guides, checks that `base_sha` is a commit and an ancestor of `HEAD`, and writes a plan with STOP conditions for each step. `stop: true` returns `stopped`.
3. **Implement** (phase `Implement`): one agent implements the plan on `branch` and commits; it records deviations and returns `conflict` instead of a guess. A conflict returns `stopped`. The task text lists the files that the task can touch; an edit outside the list is a conflict (spec 3.6).
4. **Review** (phase `Review`): `parallel` of three agents with the house rules and the deliberate choices: the adversarial refuter of the change, the independent invariant checker, and the gate check. A missing result (an agent that failed, for example at a usage limit) returns `stopped`, because a missing check is not a pass. The script turns each failed or skipped test of the gate check into a finding, because each gate of `args.gates` is a required suite.
5. Deduplicate by `file:line`, first occurrence wins, and drop each key of the refuted set.
6. `parallel` of one refuter for each new finding. Only `confirmed: true` confirms. Each other finding goes into the refuted set and stays there.
7. No confirmed finding: return `done`. Round count equal to `round_cap` (default 2): return `findings_left`.
8. **Fix** (phase `Fix`): group the confirmed findings by area (the top-level folder of the file; gate findings without a file are the area `gates`), and run one fixer at a time. Then go to step 4.

Questions (plan, implement, fix): the prompt tells the agent to call `question_open`, send the returned `header` to `main` with `SendMessage`, and call `answer_wait` with `args.deadline_seconds`. When the agent returns a `question`, the script looks for `args.answers[<id>]`. With no answer, the script returns `status: question`. With an answer, the script runs the same step again, with the same prompt plus a section with the earlier questions and their answers. The prompt of the first attempt never changes, so a relaunch with `resumeFromRunId` returns its cached result, and only the new attempt runs live.

- [ ] **Step 1:** Write `deliver.js`.
- [ ] **Step 2:** Run `node --test plugins/bruh/workflows/deliver.test.mjs`. Expected: PASS.
- [ ] **Step 3:** Run `claude plugin validate ./plugins/bruh`. Expected: `Validation passed`.
- [ ] **Step 4: Commit** `Add the deliver workflow`.

---

## Deviations from the specification

1. **`args.answers`** (additive key; problem reported to the controller). Interfaces section 4 has no field that carries an answer into a relaunch. The Workflow runtime replays the cached result of each `agent()` call whose prompt did not change. The agent that asked returned `question` in the first run, so with the same `args` its cached result says `question` again, and the run can never continue. The clerk relaunches with the same `args` plus `answers: {"Q-<n>": "<answer text>"}`. No prompt before the question contains `answers`, so those agents stay cached (spec 6.2, "The answer goes only into the prompts of the agents after the question"). The clerk still writes each answer with `answer_write` first, so an agent that still waits gets it in the same run.
2. **`stopped` reason.** The result shape of interfaces section 4 has no reason field. The script puts the reason of a `stopped` result into `deviations` as a line that starts with `STOP:` or `CONFLICT:`.

## Decisions (agent-derived, needs owner decision)

1. **A review round** is one review and, when confirmed findings are left and the cap is not reached, one fix pass. With the default cap of 2, the run is: review, fix, review. Options, ranked: (a) this reading (chosen: the last action of a run is always a review, so the result reports what is in the tree); (b) review and fix in each round, then one extra review.
2. **A gate finding goes to a refuter too** (spec 6.3, "One refuter checks each finding"). The refuter of a gate finding runs the gate command again, and it refutes only when the new run shows no failure and no skip.
3. **A failed review agent stops the run** with `stopped`, and the clerk relaunches with `resumeFromRunId` (spec 15).
4. **`question` is `null`** in each result that is not `status: question`, so that every result has all keys of interfaces section 4.
5. **Finding states.** A finding is `open` when the last review confirmed it, `refuted` when a refuter did not confirm it, and `fixed` when a fixer fixed it or a later review did not confirm it again.
6. **An area** is the top-level folder of the file, so that one fixer gets all findings of one part of the tree.

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| Script API: `agent(prompt, {label, phase, schema, model, effort, isolation, agentType})`, `parallel` (a barrier; a thrown thunk gives `null`), `pipeline`, `phase`, `log`, `args`, `budget`, `workflow` | Loaded the bundled skill | `/workflow-authoring` |
| `agent()` with `schema` returns the validated object; it returns `null` when the agent dies | Loaded the bundled skill | `/workflow-authoring` |
| `meta` must be a pure literal with `name` and `description` | Loaded the bundled skill | `/workflow-authoring` |
| `Date.now()`, `Math.random()`, and argless `new Date()` throw in a script | Read the page | <https://code.claude.com/docs/en/workflows.md>, "Timestamps and randomness" |
| A relaunch returns the saved result of each completed agent; the first agent whose prompt differs runs again, and so does every agent after it; a failed agent and every agent after it run again | Read the page | workflows.md, "Resume after a pause" |
| A plugin workflow runs as `/<plugin>:<meta.name>`; plugin workflows are `.js` files in `workflows/` | Read the page | workflows.md, "Distribute a workflow in a plugin"; plugins-reference.md |
| Workflow agents reach the MCP tools of the session through `ToolSearch` | Loaded the bundled skill | `/workflow-authoring` |
| `SendMessage` to `main` from a workflow agent reaches the main session in the same run; the answer comes back through a file | Probes of 2026-09-27 | knowledge.md, "Probe results" |

Probe D1 (2026-09-30, Claude Code 2.1.284): in a scratch folder inside a trusted folder (no `git init`), with a settings file that allows `Workflow(bruh:deliver)`, `claude -p --plugin-dir <repo>/plugins/bruh --model haiku --settings <that file> --permission-mode auto 'Run the workflow /bruh:deliver with args {"task":"probe"} ...'` printed `{"status":"stopped","branch":"","base_sha":"","head_sha":"","tests":{"ran":0,"passed":0,"failed":0,"skipped":0},"findings":[],"question":null,"deviations":["STOP: args.base_sha is not a 40-character hex SHA; args.branch is empty; args.gates is not a list"]}`. This proves that the Workflow runtime parses `deliver.js`, that the plugin workflow runs as `/bruh:deliver`, that `args` arrives as an object, and that invalid `args` stop the run before any agent. The smoke test of plan 6 runs the question path end to end (spec 20).

## Self-review

- **Spec coverage:** 6.1 (no posts, no file writes by the script), 6.2 (question path and relaunch), 6.3 (all four stages, dedup, refuters, round cap, skipped tests, base SHA), 3.3 (efforts), 15 (a failed agent stops; relaunch with `resumeFromRunId`).
- **Review Focus:** each item has a test in `deliver.test.mjs`.
