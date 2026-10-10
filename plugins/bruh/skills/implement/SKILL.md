---
name: implement
description: "The bruh procedure for code changes and reviews. The orchestrating session does not edit code: an implementer agent writes from a brief, and a separate reviewer agent checks with the review guides, in a fix loop. For a settled spec with many changes: function-sized TDD tickets, waves, parallel lanes, a whole-branch gate, and a multi-lens review that refutes its own findings. Use when the user runs /bruh:implement, asks to implement a settled spec with tickets and waves, or asks for a bruh review of a pull request or a merge request, and when a bruh start message names /bruh:implement."
argument-hint: "[spec path, or pull request]"
---

# bruh implement

This skill turns one settled design into one reviewed change. Many small agents do the work, not one large agent. The design must be decided before you start: a spec file with no open questions. If the spec has open questions, interview the owner first and write the spec. This skill starts after that.

The shape, from start to end:

```text
spec -> guides -> tickets (reviewed) -> waves -> implement lanes -> gate -> commit -> pull request
                                                                                        |
green <- pipeline fix <- commit <- post <- review (lenses -> refute -> fix -> gate) <----+
```

Each arrow is a workflow of this plugin or a short manual step. Each workflow takes its paths in `args`, so you do not edit a script to use it again:

| Workflow | Use |
|---|---|
| `/bruh:tickets` | Break a settled spec into tickets and waves (phase 2) |
| `/bruh:implement-tickets` | Implement the tickets, wave by wave (phase 3) |
| `/bruh:review-and-fix` | Review, refute, and fix an own change (phase 4) |
| `/bruh:review-only` | Review and refute a change of another author (phase 4) |

The script is `${CLAUDE_PLUGIN_ROOT}/scripts/lane.sh`. Example `args` are in `references/example-args.md`.

## Who is the orchestrator

- In a bruh role session, a clerk runs this skill. The clerk is the orchestrator. It sends each question to its clanker, and it posts only as "Post a review" says.
- In a manual session of the owner, the session is the orchestrator, and the owner answers each question in the terminal.
- The Workflow tool asks for an opt-in before a workflow runs. The init skill adds the allow rule `Workflow(bruh:<name>)` for each workflow of this skill. In a role session, a refusal of the Workflow tool is a P0 to the clanker. Do not try another form.
- Each workflow can ask a question while it runs (spec 6.2): an agent opens it with `question_open`, sends its header to the session, and waits with `answer_wait` up to `args.deadline_seconds` (default 3600). When the deadline passes, the run returns `status: question`. Answer it, add the answer to `args.answers` (`{"Q-<id>": "<answer>"}`), and relaunch with `resumeFromRunId`: the agents before the question return their cached results. For a new run (no `resumeFromRunId`), also pass the answers as `args.decided`: each agent prompt then has them as "Decisions already made". A resume keeps the `decided` of its run unchanged, so that the cached prompts stay the same.

## Rule zero: the orchestrator does not touch the code

The orchestrating session runs this skill. It does not do the work. Each edit to a repository, also a small one, is a brief to an implementer agent and a review by a separate agent with the guides, then a fix loop. This rule applies to a one-line fix, a comment, a task runner task, a CI job, and an infrastructure variable. The orchestrator writes only its own notes in the scratch folder, its memory, and its handoff.

The reason: the context of the orchestrator is the scarce resource, and the orchestrator is the judge. Code that the orchestrator writes fills the context that must hold the full picture. The same eyes that wrote the code then give it a lighter review. "Small enough to do inline" is exactly the decision that the orchestrator must not make.

In practice: for one change, start one implementer agent at high effort with the brief of "Single-change brief". Then start one reviewer agent at low effort with the review prompt and the guides. Loop on the findings, at most three times. For more than a few changes, use the ticket pipeline. The orchestrator can read files to write a brief. The orchestrator does not use `Write` or `Edit` on a path of the repository.

## Phase 0: Ground rules

Write these rules into the spec, or into a `RULES` block. Each agent prompt carries them. The workflows of this plugin put them into each prompt.

- Agents never run `git commit`, `git push`, `git stash`, `git checkout`, `git switch`, `git reset`, or `git rebase`. The owner or the clerk makes one commit for each milestone.
- Agents do not post outside the project: no comments on pull requests, merge requests, or issues. A session posts a saved result later (see "Post a review").
- Agents edit only the files that their ticket lists. Each other change goes into the `deviations` field of the report.
- The ticket wins on detail, and the spec wins on intent. An agent reports a conflict. It does not guess.
- A scratch folder (`.scratch/<feature>/`, ignored by git) holds the spec, the tickets, the guides, and the wave lists. Agents edit only the `Status:` line of a ticket there.
- The verify commands of the acceptance criteria of a ticket are the only pass or fail signal.

## Phase 1: Review guides

Put a copy of each style guide and security guide that the reviewers apply into `.scratch/<feature>/guides/`: one file for each guide, with its source URL on the first line. Then write `guides/INDEX.md`: a table from file globs to guide files, and a list of the accepted deviations that reviewers must not flag. A reviewer reads only the rows that match its diff.

`references/guide-sources.md` tells where to get each guide, for each technology. Get each guide fresh. Do not trust the memory of a model for a style guide.

The guides are mandatory for each reviewing agent. Each review prompt of this skill carries the rule: read the guide files that match the diff before you read the diff, cite the guide and the rule for each finding, and treat a review without the guides as invalid. A ticket with an empty "Review guides" section fails its review until the section has rows. If a technology of the diff has no row in `guide-sources.md`, stop and add a row before the review. If a skill for the language of the diff is installed, the reviewer loads it too.

## Phase 2: Tickets

Run `/bruh:tickets` with `args = {root, spec, issues, guides, rules}`. `rules` is the text of `references/ticket-template.md` plus the rules of the project: the lane names, the files that force `Parallel-safe: no`, and the order of work in each lane. Optional: `ground` (a list of paths that the writer reads to ground each file path), `amendments` (spec decisions of the loop, for the fixers from round 3), and `round_cap` (default 5). One writer drafts, one reviewer checks against the spec and the rules, and one fixer applies the findings, up to the round cap. The two rules that are most important:

- **Function-sized.** One function, one type, one configuration file, or one path of an API spec. A new agent completes it in one session. No vertical slices.
- **TDD pairs.** A test ticket writes the test and a stub that panics (for example `panic("not implemented")`), so the package compiles and the test is red. The implementation ticket, blocked by the test ticket, makes the test green without a change to the test.

The writer also writes `issues/INDEX.md` with the execution waves. A wave is a set of tickets that are not blocked, that are marked parallel-safe, and whose file lists do not overlap. A ticket that touches the module file, generated code, the task runner file, CI configuration, the root spec file, or the main entry point is a wave of one. The review loop usually converges in three to five rounds. Fix the last one or two contradictions with one more implementer agent, not with a sixth round.

Check before the run: each ticket that edits a file names a file that exists, and each new module that a ticket needs is allowed by the MODULES rule of `/bruh:implement-tickets`, so reviewers do not flag the change of the module file.

## Phase 3: Implement the tickets

Run `/bruh:implement-tickets` with `args = {root, spec, guides, issues, lane, waves, gates}`. `lane` is `${CLAUDE_PLUGIN_ROOT}/scripts/lane.sh`. `waves` is a list of lists of ticket file names, in order. `gates` is the list of the gate commands of the project (format check, vet, build, lint, tidy and then `git status --short` of the module files, the full tests, the coverage floor). Optional: `test_gates` (the gates of `gates` that are test suites; each must run at least one test), `fix_cap` (default 3), `deadline_seconds`, `answers`, and `decided`. For each ticket:

1. **Implement** (high effort). In the shared tree for a wave of one. In a private lane copy (`lane.sh start <id>`) for a wave of more than one, and for a leftover ticket of `lane_tickets`. A lane is an `rsync` copy with the baseline staged, so `git diff refs/lane/base` in the lane is exactly the work of this ticket. The lanes of a run are under `${TMPDIR:-/tmp}/bruh-lanes/<hash of root and spec>`. `start` keeps an existing lane only when it was made for the same repository, the same commit, and the same run; else it makes the lane again. `apply` applies a patch once.
2. **Review** (low effort). The reviewer runs the verify commands itself, checks the signatures byte for byte against the ticket, applies the MUST rules of the guides, and checks the layers and the spec. On a pass, it changes the ticket to `Status: done`.
3. **Fix loop**, at most `fix_cap` rounds. The findings of a failed review go to a fixer at high effort, then the reviewer checks again.
4. **Merge the lanes** back in wave order with `lane.sh patch`, `apply`, and `clean`. A rejected hunk stops the run. A failed lane stays for a look.
5. **Gate** at the end: the commands of `gates` over the whole branch, with the rules of `/bruh:deliver`: each gate exits 0, a failed or a skipped test is a finding, each gate of `gates` has exactly one result, and each gate of `test_gates` runs tests.

`lane.sh apply` is a plain `git apply`, not `--3way`: a three-way apply needs a clean index, and the shared tree is dirty on purpose.

The result has `status` (`done`, `question`, `findings_left`, or `stopped`), each ticket with its state, the gate result with the test counts, and `deviations`. A wave with an open question is not merged, so after `question` a relaunch with `resumeFromRunId` is safe. After a `FAILED:` stop, relaunch as a new run with `waves` = the `remaining_waves` of the result (the tickets that are not merged) and `lane_tickets` = its `lane_tickets`. When the gate agent died, the result has `gate_only` = true: relaunch with `waves` = `[]` and `gate_only` = `true`. Do not use `resumeFromRunId` then: a merge step of an applied wave runs again and fails (see the lessons).

## Phase 4: Commit, rebase, and review

1. Run the gate once more yourself, then stage all files and make one signed commit. In a role session, the clerk commits.
2. Rebase onto the target branch. Generate generated code again; do not resolve it by hand. After the rebase, run the vet command on each test package: the automatic merge of git can drop imports and helper functions silently when both sides edited the same file.
3. Push and open the pull request, so the review has a place to go.
4. Run `/bruh:review-and-fix` with `args = {root, base, head, spec, guides, lenses, deliberate, gates}`. `base` is the pinned base SHA and `head` is the committed SHA of `root` (40 hex each; never a moving ref such as `origin/main`). A first agent checks that HEAD of `root` is `head` (without `head`, it takes HEAD), that the tree is clean, and that `base` is an ancestor; else the run stops. The `head_sha` of the result is the checked value. Each lens is `{key, prompt}`: one reviewer with a slice of the files and a set of guides. Split a large diff by layer, so that no reviewer gets 20,000 lines. Optional: `house_rules` (text), `test_gates`, `round_cap` (default 2; in a role session, the review-round cap of `mode.md`), `deadline_seconds`, `answers`, and `decided`. The workflow removes duplicate findings by `file:line` and gives each finding its own refuter. A refuter confirms a finding only when the problem is real and the fix is correct and proportionate. The gate commands run in each round. The gate rules are those of `/bruh:deliver`. The confirmed findings go to fixers, one area (two folders) at a time, in the shared tree. After each batch, a separate agent checks each fix that the fixer claims against the diff; only a checked fix is `fixed`. Then the review runs again, up to the round cap. The fixes stay in the working tree.
5. Save the result, and post it (see "Post a review").
6. Commit the fixes, push, and watch the pipeline until it is green. Fix CI in the same branch.

The result has `status` (`done`, `findings_left`, or `stopped`), the lens summaries, the confirmed findings (each `fixed` or `open`), and the refuted findings. A dead agent stops the run with a `FAILED:` line. It never gives `done`.

Expect the refuters to remove about one third of the raw findings. Expect the other findings to include real bugs that the reviews of single tickets could not see, because each ticket review saw one ticket: lost updates across two store calls, expiry fields that live longer than the state that they guard, and placeholder values that nothing checks.

For a change of another author, run `/bruh:review-only` with `args = {root, base, head, spec, guides, lenses, deliberate}`. `root` is a detached, clean worktree at the head of the pull request, `base` is the base SHA of the pull request, and `head` is its head SHA. The same first check runs. It reviews and refutes. It never edits the branch of the author.

## Post a review

A workflow never posts (spec 6.1). A post goes out under an account of the owner, so it is on the list "Never without the owner". Each post needs a cover:

- In a manual session of the owner (no `BRUH_ROLE_KEY`): the owner said yes to this post in the terminal.
- In a role session: the message of bigm `ANSWER Q-<id>: post <owner/repo>#<number> at <head SHA> approved` in the mailbox of the caller, for the `head_sha` of the result, or a row of the section "Post grants" of `grants.md` for the role key, the host, and the repository. A clerk follows "Posts" of its agent file.

1. Save the result of the workflow. Use the MCP tool `result_save` (`name`, for example `review-only-42`, and the result object); it returns the path. It works in a manual session too: without `BRUH_ROLE_KEY`, it saves under `<data>/results/owner/`. Never write a result file yourself (principle 3).
2. Write the body with the Write tool to a temporary file: the summary (the lenses, the confirmed and the refuted counts), then each confirmed finding with its `file:line`. For `/bruh:review-and-fix`, add what is fixed and what is open. End the file with the marker line `<!-- bruh:<role key> -->` (`<!-- bruh:owner -->` in a manual session). The watcher uses the marker to tell agent posts from human posts. Show the body to the owner (in a role session, put it in the question to the clanker).
3. With the cover in place, post it with the plain CLI of the code host:

   ```bash
   gh pr review <pull request number> --repo <owner/repo> --comment --body-file <file>
   glab mr note create <merge request number> --repo <group/project> < <file>
   ```

Do not post a result with `status` `stopped` or `question`: that review is not complete.

## Phase 5: Report

Each status message to the owner is a table: item, state, link. Each pull request, pipeline, and job has its full URL. A manual deploy job is the click of the owner.

When the run found a defect of bruh itself, a clerk sends the notice of rule 10 of its agent text. A manual session of the owner follows `${CLAUDE_PLUGIN_ROOT}/defaults/bug-reports.md`.

## Lessons of this skill

Read `references/lessons.md` before the first run. The short list:

- **A resume replays stale steps.** A relaunch with `resumeFromRunId` returns the cached results of unchanged agents, but a merge step whose lanes are already applied runs again and fails. Relaunch with only the remaining waves in `args`, as a new run.
- **Parallel lanes collide on package-level names.** Ten agents that write tests in one package declare the same constant twice. The gate finds it. Keep one shared test helper file, and tell the test tickets to use it.
- **Set the test timeout, and watch expensive hashes under the race detector.** The default test timeout of Go is ten minutes, and a suite can reach it. Make an expensive constant (for example the bcrypt cost) a variable that the test packages lower in `TestMain`.
- **Measure coverage over the module path, not `./...`.** In CI, the module cache can be in the project folder, and `./...` then adds it to the denominator.
- **An emulator harness must fail in CI, not skip.** A `TestMain` that exits 0 without Docker makes CI measure nothing.
- **Keep the full suite under one minute.** Then the gate after each fix batch costs almost nothing.
- **glab rejects bracketed field names in a JSON body.** `-f 'position[new_line]=...'` fails. Build each payload whole with `jq` and send it with `--input`.

## Single-change brief

The minimum that a one-off implementer gets, so that it needs no memory of the conversation:

```text
The repository and the branch; the files to create or edit (absolute paths); what exists
after the change that did not exist before, with the exact signatures; what must not
change; the verify commands (build, vet, lint, the tests to run); the guides to read
first; the ground rules of phase 0. Return: the files changed, the verify output, and
the deviations.
```

The reviewer gets the same brief, the diff command against the pinned base SHA, and the rule of phase 1 that the guides are mandatory.

## When not to use the full pipeline

- A spec with open questions: interview the owner first.
- A change that one agent can hold in its context: skip the ticket breakdown and the lanes. Rule zero still applies: one implementer agent and one reviewer agent.
- The ticket breakdown pays back from about thirty tickets.
