# Plan 6: Verification and release preparation

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. The lane that owns this plan implements it itself, so a step can describe the code precisely instead of pasting all of it.

**Goal:** Make the repository mature on day 0 (spec 19), give the install guide of spec 18, and give the smoke test and the load test of spec 20 and spec 4.1 as runbooks with driver scripts. Prepare the `v0.1.0` release. The owner runs the outward-facing steps.

**Architecture:** Documentation and repository files at the root and in `.github/`. One original SVG banner in `docs/assets/`. Two POSIX `sh` drivers, `tests/smoke/run.sh` and `tests/load/run.sh`, share `tests/lib.sh`. A driver talks to the bruh MCP server in two ways: through the Claude Code sessions that it starts, and directly as a stand-in caller. For the stand-in calls, the driver builds the server once into its scratch folder and sends one JSON-RPC `tools/call` line on standard input, with `BRUH_DATA`, `BRUH_ROLE_KEY`, and `BRUH_PLUGIN_ROOT` set. So plugin code writes every bruh file, also in a test. `tests/test.sh` tests the functions of `tests/lib.sh` and the `--dry-run` output of both drivers without a Claude session.

**Tech Stack:** Markdown, GitHub Actions YAML, GitHub issue forms, SVG, POSIX `sh`, `jq`, `git`, Go 1.26 (to build the MCP server for the stand-in calls), Claude Code 2.1.284, Orca (optional remote step).

**Spec:** [../../spec.md](../../spec.md) version 0.4, sections 4.1, 18, 19, and 20. Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md). Index: [2026-09-30-bruh-v0.1-index.md](2026-09-30-bruh-v0.1-index.md).

## Global Constraints

- Lane `repo` owns `README.md`, `CHANGELOG.md`, `SECURITY.md`, `CONTRIBUTING.md`, `.github/` except `ci.yml`, `docs/assets/`, `tests/`. This plan does not edit `.github/workflows/ci.yml`.
- Names come from the interfaces file: agents `bruh:bigm`, `bruh:clanker`, `bruh:clerk`; workflow `/bruh:deliver`; MCP tools `mcp__plugin_bruh_bruh__<tool>`; role keys `bigm`, `clanker-<project>`, `clerk-<project>-<task>`; the launch command of interfaces section 2.
- A badge shows only a fact that CI or GitHub proves (spec 19, owner decision 2026-09-27).
- The README makes no claim that is not true of the repository.
- Scripts: POSIX `sh` and `jq`, portable to macOS and Linux, shellcheck-clean.
- Public repository: no personal names, no email addresses, no host names, no machine paths in any committed file.
- Documentation: ASD-STE100 Simplified Technical English.
- A driver stops every background session that it started, and only those.
- The outward-facing release steps (branch protection, topics, private vulnerability reporting, tag, release) need the explicit go of the owner. This plan writes them as commands and does not run them.

## Review Focus

1. **A live session has a name that the smoke run uses** (the real `bigm` or `clerk-ledger` of the owner, whose role keys are fixed). A `SendMessage` nudge from the smoke roles would reach it. Expected: the smoke driver refuses to start and names the session, and its cleanup stops every session in the run folder, `clerk-ledger` included. Pinned in Task 5 by `tests/test.sh` (`live_conflicts` and `own_sessions` on fixtures of `claude agents --json --all`).
2. **The trusted repository has a remote.** A smoke clerk could push its task branch to it (clerks do all pushes). Expected: the drivers refuse a `BRUH_TRUSTED_REPO` that has a remote, or that is not a git repository. Pinned in Task 5 (`check_trusted_repo` on a temporary repository with and without a remote).
3. **Cleanup stops a session that the run did not start.** Expected: cleanup selects sessions by name and by a `cwd` under the scratch folders of the run. Pinned in Task 5 (`own_sessions` on a fixture with a same-named session in another folder).
4. **A message is counted as delivered when it was never read.** Expected: the load test counts a message as missed when it is still in the unread mailbox folder and was posted before the grace cutoff. Pinned in Task 5 (`count_missed` on fixture mailboxes).
5. **Claude Code is older than the version the probes ran on.** Expected: preflight fails with the found and the required version. Pinned in Task 5 (`version_ge`).

Also pinned: the `--dry-run` output of both drivers contains the documented launch commands with the interface names (Task 6 and Task 7).

---

### Task 1: Plan file

**Files:**

- Create: `docs/superpowers/plans/2026-09-30-plan-6-verification-release.md`

- [x] **Step 1:** Write this file. Run markdownlint on it. Commit.

### Task 2: Community and repository files

**Files:**

- Create: `CHANGELOG.md`, `SECURITY.md`, `CONTRIBUTING.md`
- Create: `.github/ISSUE_TEMPLATE/bug.yml`, `.github/ISSUE_TEMPLATE/feature.yml`, `.github/ISSUE_TEMPLATE/config.yml`
- Create: `.github/pull_request_template.md`, `.github/dependabot.yml`, `.github/CODEOWNERS`
- Create: `.github/workflows/scorecard.yml`

**Interfaces:**

- Produces: the Scorecard workflow `scorecard.yml` (job `analysis`). The badge of Task 4 reads its published result.

- [x] **Step 1: Check first.** Run `ruby -e 'ARGV.each { |f| YAML.load_file(f) }' .github/**/*.yml` and `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest`. Expected before the files exist: no file to check.
- [x] **Step 2: Write the files.**
  - `CHANGELOG.md`: Keep a Changelog 1.1.0 format, Semantic Versioning. An `Unreleased` section that lists the v0.1 contents by spec part (roles, MCP server tools, hooks, init skill, `deliver` workflow, loops, channels, remote transport, tests, repository files). A `0.1.0` section is added at release time with the date (release checklist step 5).
  - `SECURITY.md`: supported versions (latest release), report through GitHub private vulnerability reporting (`https://github.com/oter/bruh/security/advisories/new`), what to include, the threat model in short: deny rules are speed bumps (spec principle 2), the sandbox is the real stop, what bruh writes and where (the plugin data folder, the settings keys of the init skill after a yes, the ledger repository), no telemetry, messages are plain text on the local socket.
  - `CONTRIBUTING.md`: the local checks (the commands of lane-common step 4 plus `sh tests/test.sh`), the ASD-STE100 rule for documentation, the commit style (plain English, imperative, no type prefix), docs and code in one pull request, no binaries, the public-repository rule, and how to run the smoke test before a release.
  - Issue forms: `bug.yml` (versions of Claude Code, bruh, Go, OS; role; steps; expected; actual; evidence with `claude agents --json --all` output), `feature.yml` (problem, proposal, spec section). `config.yml`: `blank_issues_enabled: false`, a contact link to the security advisory page.
  - `pull_request_template.md`: summary, spec section, checklist of the local checks, docs and code in the same pull request, test counts (ran, passed, failed, skipped).
  - `dependabot.yml`: version 2, `github-actions` at `/`, `gomod` at `/plugins/bruh/mcp`, weekly.
  - `CODEOWNERS`: `* @oter`.
  - `scorecard.yml`: the official workflow of `ossf/scorecard` (`.github/workflows/scorecard-analysis.yml`), with the same pinned SHAs, `permissions: read-all` at the top, `security-events: write` and `id-token: write` on the job, `persist-credentials: false`, `publish_results: true`.
- [x] **Step 3: Run the checks.** YAML parse, actionlint, markdownlint. Expected: no error.
- [x] **Step 4: Commit.**

### Task 3: Banner

**Files:**

- Create: `docs/assets/banner.svg`

- [x] **Step 1:** Write a hand-made SVG, 1280 by 320, `viewBox` set, `role="img"`, a `<title>` and a `<desc>`. The art shows the hierarchy: one node (bigm) with lines to clanker nodes, then clerk nodes, then small workflow dots. It uses its own filled background (a dark rounded panel), so it reads the same on the light and the dark GitHub theme. Text uses only the generic families `ui-monospace, Menlo, Consolas, monospace` and `system-ui, sans-serif`, so it renders offline without a web font.
- [x] **Step 2:** Check: `xmllint --noout docs/assets/banner.svg` passes, and `rsvg-convert -w 1280 docs/assets/banner.svg -o /tmp/bruh-lane-repo-banner.png` writes a PNG. Look at the PNG.
- [x] **Step 3:** Commit.

### Task 4: README

**Files:**

- Modify: `README.md`

- [x] **Step 1:** Rewrite the README in this order: banner; badges (CI status of `ci.yml`, latest release, license, OpenSSF Scorecard); what bruh is; the role table of spec 3; a Mermaid overview and a link to `docs/flow.md`; the eight install parts of spec 18 as numbered steps with exact commands; requirements; the session-start warning; links to the spec, flow, design, and knowledge files; license. Remove the status "The design is in progress".
- [x] **Step 2:** Every command in the README comes from the spec, the interfaces file, a Claude Code docs page, or `orca --help`. Each one is listed in "Verified facts".
- [x] **Step 3:** Run markdownlint. Commit.

### Task 5: Shared test library and its tests

**Files:**

- Create: `tests/lib.sh`, `tests/test.sh`

**Interfaces:**

- Produces (`tests/lib.sh`, sourced by both drivers):
  - `version_ge <found> <required>`: exit 0 when `found` is at least `required` (dotted numbers).
  - `live_named <agents json file> <name>`: prints the `id` of each entry with that `name` and a `pid`.
  - `own_sessions <agents json file> <cwd prefix> <name prefix>...`: prints the `id` of each entry whose `cwd` starts with the prefix and whose `name` starts with one of the name prefixes.
  - `check_trusted_repo <path>`: exit 0 when the path is a git repository with no remote; else prints the reason and exits 1.
  - `count_missed <mailbox folder> <cutoff UTC>`: prints the count of messages in the unread folder (not in `read/`) with `at` before the cutoff.
  - `mcp_call <tool> <arguments json>`: sends one `tools/call` to the server binary `$BRUH_TEST_MCP` with `BRUH_DATA`, `BRUH_ROLE_KEY`, and `BRUH_PLUGIN_ROOT` from the caller; prints the result text; exits 1 on `isError` or a JSON-RPC error. In dry-run mode it prints the request line and prints `{}`.
  - `run <command>...` and `run_in <folder> <command>...`: run a command, or print it after a `+` sign in dry-run mode.
  - `add_scratch_worktree <trusted repo> <path> <branch>`: `git worktree add --detach`, then an orphan branch with no files.
  - `data_dir`: prints `${CLAUDE_CODE_PLUGIN_CACHE_DIR:-$HOME/.claude/plugins}/data/bruh-inline`, or `$BRUH_TEST_DATA` when set.
- Test command: `sh tests/test.sh`.

- [x] **Step 1: Write the failing test.** `tests/test.sh` sources `tests/lib.sh` and checks each function with fixtures in a `mktemp -d` folder: `version_ge 2.1.284 2.1.284` is true, `version_ge 2.1.283 2.1.284` is false, `version_ge 2.2.0 2.1.284` is true; `live_named` finds the live `bigm` and not a stopped one; `own_sessions` skips the same-named session in another folder; `check_trusted_repo` refuses a missing folder, a folder that is not a repository, and a repository with a remote, and accepts one without a remote; `count_missed` counts one old unread message and skips a new unread one and an old read one. It prints `ok <n>` or `not ok <n>` for each check and exits 1 when one fails.
- [x] **Step 2:** Run `sh tests/test.sh`. Expected: FAIL, because `tests/lib.sh` does not exist.
- [x] **Step 3:** Write `tests/lib.sh`.
- [x] **Step 4:** Run `sh tests/test.sh`. Expected: all `ok`. Run shellcheck. Commit.

### Task 6: Smoke test (spec 20)

**Files:**

- Create: `tests/smoke/README.md`, `tests/smoke/run.sh`
- Modify: `tests/test.sh` (dry-run checks)

**Interfaces:**

- Consumes: `session_launch`, `session_resume`, `role_settings_write`, `mail_post`, `question_open` result files `questions/Q-<n>.json`, answer files `answers/<role key>/Q-<n>.answer`, mailboxes `mail/<role key>/` and `mail/<role key>/read/`, the agents `bruh:bigm`, `bruh:clanker`, `bruh:clerk`, the workflow `/bruh:deliver`, and `plugins/bruh/ledger-template/` (lane roles).
- Environment: `BRUH_TRUSTED_REPO` (required, a local git repository with no remote that the user trusted in an interactive session), `SMOKE_STEP_MINUTES` (default 30), `SMOKE_SWEEP_MINUTES` (default 15), `SMOKE_IDLE_MINUTES` (default 0), `SMOKE_ORCA_ENV` and `SMOKE_ORCA_REPO` (optional), `BRUH_TEST_DATA` (optional).

The driver does these steps and prints one `PASS`, `FAIL`, or `SKIP` line for each:

| Step | Signal that the driver checks |
|---|---|
| preflight | tools present; Claude Code at least 2.1.284; `BRUH_TRUSTED_REPO` passes `check_trusted_repo`; no live session with a name of the run (`bigm`, `clerk-ledger`, `clanker-smoke-<run>`, `clerk-smoke-<run>-*`); no `bigm` or `clerk-ledger` state in the data folder; the agents, the workflow, and the ledger template exist |
| bigm | a session named `bigm` with a `pid`, `cwd` in the scratch ledger |
| clanker | a session named `clanker-smoke-<run>` (bigm started it with `session_launch`) |
| clerk | sessions named `clerk-smoke-<run>-greet` and `clerk-smoke-<run>-gate` |
| deliver | `claude logs <greet clerk id>` contains `bruh:deliver` |
| p1-sent | a file `questions/Q-<n>.json` with `priority` `P1` that names the greet clerk key |
| p1-bigm | a message with header `P1 Q-<n>:` in `mail/bigm/` or `mail/bigm/read/` |
| answer-same-run | the file `answers/<greet clerk key>/Q-<n>.answer` exists, a `DONE:` message from the greet clerk is in the clanker mailbox, `claude logs` of the greet clerk does not contain `resumeFromRunId`, and the word in `GREETING.txt` is in the answer |
| p0-prompt | the gate clerk shows `waitingFor` `permission prompt`; then, within `SMOKE_SWEEP_MINUTES` plus 5, the text `claude attach <gate clerk id>` is in a P0 question file, a P0 message to bigm, a ledger file, or `claude logs` of bigm |
| idle-resume | after the clanker process is gone (`claude stop`, or the supervisor idle stop when `SMOKE_IDLE_MINUTES` is set), a `claude -p` session sends `SMOKE-IDLE <word>` to bigm; the real smoke bigm sends `DONE: smoke idle check <word>` to the clanker; the clanker gets a `pid` under the same `sessionId`, the message from bigm is in `read/`, and `claude logs` of bigm shows `session_resume` (or `respawn` after `claude stop`) |
| remote | skipped with a `SKIP` line when `SMOKE_ORCA_ENV` or `SMOKE_ORCA_REPO` is not set or `orca environment show` does not find it; else a worker (worktree `bruh-smoke-<run>`, `--setup skip`) starts a `claude -p --agent bruh:clanker` that asks `P1 Q-1: smoke remote <run>` with `orca orchestration ask`; the driver finds the question in `orca orchestration inbox --json`, replies with a new random word, and passes when a message carries the rot13 form of the word; cleanup removes the remote worktree with `orca worktree rm` |

- [x] **Step 1: Write the failing test.** Add to `tests/test.sh`: `BRUH_TRUSTED_REPO=<fixture> sh tests/smoke/run.sh --dry-run` exits 0, and its output contains `claude --bg --agent bruh:bigm --name bigm --permission-mode auto`, `--settings`, `--plugin-dir`, `"name":"role_settings_write"`, the `SMOKE-IDLE` nudge to bigm, no `"name":"session_resume"` call of the driver, and `SKIP remote`.
- [x] **Step 2:** Run `sh tests/test.sh`. Expected: FAIL, because `tests/smoke/run.sh` does not exist.
- [x] **Step 3:** Write `tests/smoke/run.sh`: preflight, setup (build the MCP server into the scratch folder; add the project and the ledger as linked worktrees of `BRUH_TRUSTED_REPO` on orphan branches; write `TASK.md`, `gate.sh`, and `.claude/settings.json` with the allow rules `Workflow(bruh:deliver)` and `mcp__plugin_bruh_bruh`, the ask rule `Bash(./gate.sh)`, and the deny rule `Bash(git push:*)`; copy the ledger template and set `mode: autonomous`; commit both with a fixed test identity), the steps of the table, and a cleanup trap for `EXIT`, `HUP`, `INT`, and `TERM` that saves the evidence, stops every session in the run folder, confirms that no own session has a `pid`, removes the worktrees and the branches that the run created, and removes the data files of the run keys.
- [x] **Step 4:** Write `tests/smoke/README.md`: purpose, the one-time setup of the trusted repository, the command, the environment variables, the table of steps, how to read the result, and what goes into the release notes.
- [x] **Step 5:** Run `sh tests/test.sh` and shellcheck. Expected: PASS. Commit.

### Task 7: Load test (spec 4.1)

**Files:**

- Create: `tests/load/README.md`, `tests/load/run.sh`
- Modify: `tests/test.sh` (dry-run checks)

**Interfaces:**

- Flags: `--sessions <n>` (default 8), `--minutes <n>` (default 60), `--interval <minutes>` (default 2), `--model <name>` (default `haiku`), `--dry-run`.
- Role keys: hub `clanker-load-<run>`, sessions `clerk-load-<run>-s<i>`.

- [x] **Step 1: Write the failing test.** Add to `tests/test.sh`: `sh tests/load/run.sh --dry-run --sessions 2 --minutes 1` exits 0, and its output contains two launch commands `claude --bg --name clerk-load-<run>-s1` and `-s2`, `"name":"mail_post"`, and `SendMessage`.
- [x] **Step 2:** Run `sh tests/test.sh`. Expected: FAIL.
- [x] **Step 3:** Write `tests/load/run.sh`. The hub (a stand-in caller) writes the role settings of each session with `role_settings_write` and puts a start message in each mailbox with `mail_post`. The start message tells session `i` to create a recurring `CronCreate` task every `--interval` minutes that posts one message to session `i+1` (a ring) with `mail_post` and header `P2 Q-<i>: load tick`, then sends the header and the returned mail ID as a `SendMessage` nudge to that session by name, and to call `mail_read` when a nudge arrives. The driver launches each session with the launch command of spec 4.1 without `--agent`, in a linked worktree of `BRUH_TRUSTED_REPO`. Each minute it reads `claude agents --json --all`, and for a session with no `pid` and unread mail older than 2 minutes it runs `claude --resume <sessionId> --bg "Read your mailbox with mail_read."` and records the resume and whether the `sessionId` stayed the same. At the end it prints, for each session, the posted, read, and missed counts, the resumes, and one `PASS` or `FAIL` line: PASS when no message is missed and each resume kept its session ID. Cleanup is the same as in the smoke test.
- [x] **Step 4:** Write `tests/load/README.md`.
- [x] **Step 5:** Run `sh tests/test.sh` and shellcheck. Commit.

### Task 8: Checks and lane report

- [x] **Step 1:** Run every check of lane-common step 4 that applies, plus `sh tests/test.sh`, YAML parse, actionlint, and `rsvg-convert`. Fix what fails.
- [x] **Step 2:** Write `.superpowers/lane-report.md`.

## Release checklist (needs the explicit go of the owner)

Each step is outward-facing. Nobody runs a step before the owner says go for that step. Run them in this order.

1. **Private vulnerability reporting, before the v0.1 lanes merge into `main`.** `SECURITY.md` and the issue form `config.yml` point to it, and `blank_issues_enabled: false` removes the other route, so it must work when those files reach the public `main`. On 2026-09-30 it was off (`gh api repos/oter/bruh/private-vulnerability-reporting` returned `{"enabled":false}`).

   ```bash
   gh api -X PUT repos/oter/bruh/private-vulnerability-reporting
   ```

2. **Smoke test** after the merge. Run `tests/smoke/run.sh` on the release candidate (see [tests/smoke/README.md](../../../tests/smoke/README.md)). Save the output as `smoke-result.txt`. A `FAIL` line stops the release. A `SKIP remote` line needs the go of the owner to continue, because the remote signal of spec 20 is then not proven.

3. **Repository topics:**

   ```bash
   gh repo edit oter/bruh --add-topic claude-code --add-topic claude-code-plugin \
     --add-topic multi-agent --add-topic orchestration --add-topic agent-workflows
   ```

4. **Branch protection on `main`** with the CI checks required. The check names are the job names of `ci.yml` as GitHub shows them. Read them from the last run on `main` first, because other lanes add jobs:

   ```bash
   gh run list --repo oter/bruh --workflow ci.yml --branch main --limit 1 --json databaseId --jq '.[0].databaseId'
   gh run view <run id> --repo oter/bruh --json jobs --jq '.jobs[].name'
   ```

   Then put each name in `contexts` (the example has the job names of `ci.yml` today). The settings are agent-derived, needs owner decision: `strict: true` (the branch must be up to date), `required_linear_history: true`, `enforce_admins: false` (the owner can still merge in an emergency), no required reviews (one maintainer), no force pushes, no deletion. Options, ranked: (a) these settings; (b) the same with `enforce_admins: true`; (c) a ruleset instead of classic branch protection.

   ```bash
   gh api -X PUT repos/oter/bruh/branches/main/protection --input - <<'EOF'
   {
     "required_status_checks": {
       "strict": true,
       "contexts": ["go (ubuntu-latest)", "go (macos-latest)", "lint", "plugin-validate"]
     },
     "enforce_admins": false,
     "required_pull_request_reviews": null,
     "restrictions": null,
     "allow_force_pushes": false,
     "allow_deletions": false,
     "required_linear_history": true
   }
   EOF
   ```

5. **Changelog and version.** In a pull request: rename `## [Unreleased]` in `CHANGELOG.md` to `## [0.1.0] - <UTC date>`, add a new empty `## [Unreleased]` above it, update the compare links, and set `"version": "0.1.0"` in `plugins/bruh/.claude-plugin/plugin.json`. Merge it after CI is green.
6. **Tag:**

   ```bash
   git switch main && git pull --ff-only
   git tag -a v0.1.0 -m "bruh v0.1.0"
   git push origin v0.1.0
   ```

7. **GitHub release** with the smoke test result in the notes:

   ```bash
   { sed -n '/^## \[0.1.0\]/,/^## \[/p' CHANGELOG.md | sed '$d'; printf '\n## Smoke test\n\n```text\n'; cat smoke-result.txt; printf '```\n'; } > release-notes.md
   gh release create v0.1.0 --repo oter/bruh --title "bruh v0.1.0" --notes-file release-notes.md --verify-tag
   ```

8. **Check the badges.** After the release and after the first Scorecard run on `main`, open the README on GitHub and confirm that each badge shows a value.

## Deviations from the specification

Each item below is tagged. Each option list is ranked, the first option is the one that is built.

1. **The smoke test starts bigm as a background session.** Agent-derived, needs owner decision. Spec 3.4 says that bigm stays an interactive session. A script cannot own an interactive terminal, so the driver starts bigm with `claude --bg --agent bruh:bigm --name bigm` in the scratch ledger. The reason in spec 3.4 (a background session moves into a worktree before it edits files) does not apply, because the scratch ledger is already a linked worktree. The smoke test does not use `--channels`. Options: (a) background bigm; (b) the person starts bigm in a terminal with the spec 3.4 command and the driver checks everything else; (c) a bigm stand-in with no sweep, which cannot test the P0 step.
2. **The ledger is in autonomous mode.** Agent-derived, needs owner decision. bigm decides the P1 question itself (spec 11), so no person must answer. The human-mode path is not in the smoke test. Options: (a) autonomous mode; (b) human mode, and the driver prints the question and waits for the person to answer in `claude attach` of bigm.
3. **The idle-resume step simulates the idle stop with `claude stop`** by default. Agent-derived, needs owner decision. `claude stop` sets the state `stopped`, and bigm treats `stopped` as a failure (`claude respawn`), not as an idle stop (`session_resume`). So in the default mode the step accepts `respawn` or `session_resume` in the output of bigm. `SMOKE_IDLE_MINUTES=65` waits for the real idle stop and accepts only `session_resume`. In both modes the real smoke bigm, not the driver, sends the message: a `claude -p` session sends `SMOKE-IDLE <word>` to bigm, and the start prompt of bigm says what to do with it. Options: (a) as built; (b) the real idle wait by default, which adds more than one hour to each release; (c) a new `claude` command to end the process without the state `stopped`, if Claude Code adds one.
4. **Allow rules come from the scratch project, not from the init skill.** Agent-derived, needs owner decision. The driver writes `Workflow(bruh:deliver)`, `mcp__plugin_bruh_bruh`, and (in the ledger) `SendMessage` into `.claude/settings.json` of the scratch project and the scratch ledger. Options: (a) project settings; (b) a preflight check that the user settings have the allow rules of `/bruh:init`.
5. **The permission prompt comes from ask rules.** Agent-derived, needs owner decision. The ask rules cover `./gate.sh`, `sh gate.sh`, and `bash gate.sh`. Another form of the command gets past them (principle 2), and then the step fails with the reason. Options: (a) ask rules; (b) a command on a protected path, which goes to the classifier in `auto` mode and so does not always prompt.
6. **The smoke clanker starts two clerks.** Agent-derived, needs owner decision. Spec 20 says that the clanker starts one clerk. The test has the tasks `greet` (the P1 question) and `gate` (the permission prompt), because a clerk that waits on a prompt cannot also finish the P1 round trip. Each clerk makes its own worktree with `EnterWorktree` (lane roles `clerk.md`). Options: (a) two clerks; (b) one clerk that asks the P1 first and hits the prompt last, which makes the steps depend on the order inside one workflow run.
7. **The load test sessions run without `--agent`,** on `--model haiku` by default. Agent-derived, needs owner decision. The test measures the transport of spec 4.1, not the role instructions. The driver, not the sender, does the `pid` check and the resume. PASS needs zero missed messages and a message for each session in each window of 5 minutes (or 2 intervals) after a warm-up window. Options: (a) as built; (b) `--agent bruh:clerk` sessions with the `pid` check in the start message, which tests the role text and the transport together.
8. **The remote smoke step uses a `claude -p` clanker in the Orca worker terminal** and does not start a remote clerk. Agent-derived, needs owner decision. The driver must run in an Orca terminal (spec 4.2 Verify item). PASS needs the rot13 form of a word that the driver sends only in its Orca reply, so the text of the worker spec cannot match. It does not prove that the answering session is a bruh clanker. The driver cannot check that `SMOKE_ORCA_REPO` on the remote machine has no git remote; the runbook says to use one with no remote, and the remote clanker only asks and sends. Options: (a) as built; (b) a full remote chain with a remote clerk, which needs the init skill on the remote machine first.
9. **`tests/test.sh` is not in CI in this branch.** Agent-derived, needs owner decision. This lane does not edit `ci.yml`. The controller adds a step `sh tests/test.sh` to the `go` matrix job (macOS and Linux) at the merge. Options: (a) the `go` matrix job; (b) a separate `shell-tests` job on both systems; (c) the `lint` job on Linux only, which does not test macOS.
10. **The drivers act as a stand-in caller of the MCP server.** Agent-derived, needs owner decision. The smoke driver calls `role_settings_write` as `bigm` before bigm starts; the load driver calls `role_settings_write` and `mail_post` as a hub clanker. A binary that the driver builds into its evidence folder gets each call on standard input. Options: (a) direct calls, because they are exact and cost no tokens; (b) `claude -p` stand-in sessions that call the tools.
11. **The scratch ledger is a copy of the ledger template, not a ledger of `/bruh:init`.** Agent-derived, needs owner decision. The non-interactive init also writes user settings, which a test must not change. So the fill rules of `ledger-template/README.md` are not applied. Options: (a) a copy; (b) `init_plan` and `init_apply` with a test answer file, when init gets an option to write no user settings.
12. **Cleanup deletes each new branch that does not contain the HEAD commit of the trusted repository.** Agent-derived, needs owner decision. The task branches of the clerks have names that the driver does not know. They start from the orphan commit of the scratch project, so they do not contain that HEAD commit. Options: (a) as built; (b) delete only `bruh-<project>*` branches and leave the task branches.
13. **Only the latest release gets security fixes** (`SECURITY.md`). Agent-derived, needs owner decision. Options: (a) latest release only; (b) the latest two minor versions.
14. **Dependabot also updates the Go module** (`gomod` at `plugins/bruh/mcp`). Agent-derived, needs owner decision. Spec 19 names only GitHub Actions. The module has no `require` lines today, so the entry creates no pull request until one is added. Options: (a) GitHub Actions and `gomod`; (b) GitHub Actions only, as spec 19 says.
15. **Blank issues are off** (`blank_issues_enabled: false`). Agent-derived, needs owner decision. Each report then has the version and evidence fields. Options: (a) off; (b) on.
16. **A remote machine gets its own empty ledger path for init** (README step 5). Agent-derived, needs owner decision. Init requires a ledger path, and only bigm writes the real ledger. Options: (a) an empty local folder; (b) an init option for a remote machine with no ledger (lane go).
17. **The container volume holds the whole `~/.claude` folder** (README step 6). Agent-derived, needs owner decision (controller ruling of fix round 1). It keeps the plugin install, the user settings, the login, and the plugin data. Options: (a) the whole folder; (b) two volumes, one for the plugin data folder and one for the rest.
18. **The README has no Slack steps yet.** Agent-derived, needs owner decision (controller ruling of fix round 1). Lane go builds the Slack channel, and the controller writes that section at the merge. Options: (a) no Slack steps in this branch; (b) Slack steps now, which could describe a channel that does not exist.

## Assumptions about the other lanes

The drivers depend on names and shapes that lanes go and roles build at the same time. Check each one when the lanes are merged, then run the smoke test.

1. A question file `questions/Q-<n>.json` has a top-level field `priority`, and one of its string fields is the role key of the asker (interfaces section 2 says "with the asker").
2. A clerk sends `DONE: <subject>` to its clanker with `mail_post` when its task is done (spec 5).
3. A P0 for a permission prompt contains the text `claude attach <short id>` (spec 14.2), in a P0 question file, a P0 message to bigm, a ledger file, or the output of bigm.
4. `plugins/bruh/ledger-template/mode.md` has a line that starts with `mode:` and a space (spec 11).
5. The agent files are `plugins/bruh/agents/{bigm,clanker,clerk}.md`, and the workflow is `plugins/bruh/workflows/deliver.js` (spec 17).
6. `session_resume` finds the session ID and the folder of the role key by itself, so the caller gives only `role_key` and `prompt` (interfaces section 2).
7. The README step 2 command `GOTOOLCHAIN=local go run -C <plugin root>/mcp . init --answers <file>` works from a shell with no `BRUH_DATA` (interfaces section 5). Lane go decides how the command finds the plugin data folder.
8. The JSON output of `orca orchestration worker-start --json` contains the dispatch ID as a string `ctx_<hex>`, and `run-create --json` the run ID as `run_<hex>` (the forms that `orca orchestration inbox --json` shows). Not verified; the remote step uses them to filter the inbox and to stop the worker.
9. bigm starts `clerk-ledger` in the ledger folder (lane roles `bigm.md`), so cleanup finds it by its folder.

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| `claude --bg` in a fresh `git init` repository inside a trusted folder fails with "Workspace not trusted". A linked worktree of an already-trusted repository works: `git -C <trusted repo> worktree add --detach <path> HEAD`, then `claude --bg` in `<path>` started and finished normally. | Controller probe, 2026-09-30 | Probe (no machine paths recorded) |
| `${CLAUDE_PLUGIN_DATA}` is `~/.claude/plugins/data/<id>/`, where `<id>` is the plugin identifier with each character other than a letter, digit, `_`, or `-` replaced by `-`. | Docs | <https://code.claude.com/docs/en/plugins-reference.md>, "Environment variables" |
| A plugin loaded with `--plugin-dir` has the id `<name>@inline`. So its data folder is `data/bruh-inline/`, and an installed `bruh@bruh` uses `data/bruh-bruh/`. | Docs | <https://code.claude.com/docs/en/plugins/loading.md>, "Find where a plugin came from" |
| An enabled `--plugin-dir` plugin replaces a same-named installed marketplace plugin for that session. | Docs | plugins/loading.md, "Name conflicts" |
| The plugins root is `~/.claude/plugins` unless `CLAUDE_CODE_PLUGIN_CACHE_DIR` is set; an installed plugin is in `cache/<marketplace>/<plugin>/<version>/`. | Docs | plugins/loading.md, "Find plugins on disk" |
| `claude agents --json` fields: `id` (short ID for `attach`, `logs`, `stop`, `respawn`), `state`, `pid` and `status` only while the process lives, `waitingFor` (`permission prompt`, `input needed`, `sandbox request`, `worker request`, `dialog open`), `sessionId` (full UUID for `--resume`), `name`, `cwd`, `kind`. | Docs | <https://code.claude.com/docs/en/agent-view.md> |
| `claude --resume <sessionId> --bg` continues under the same ID, or starts a copy under a new ID and prints a `note:` line. | Docs | agent-view.md |
| An explicit ask rule forces a permission prompt also in `auto` mode. | Docs | <https://code.claude.com/docs/en/permission-modes.md>, auto mode |
| A background session started with `--bg` is refused until the trust dialog was accepted in an interactive session. | Docs | permission-modes.md |
| The receiving Claude reads a cross-session message between tool calls; an idle session starts a new turn. The receiver drops identical repeats that arrive in a short window. A session in `auto` mode accepts a message from a session that does not bypass permission prompts. A `claude -p` session binds an inbox socket too. | Docs | <https://code.claude.com/docs/en/cross-session-messaging.md> |
| The only documented part of the socket protocol is the auth line, so a shell script cannot send a `SendMessage` nudge itself. The drivers send nudges only from sessions. | Docs | cross-session-messaging.md, "The session's inbox socket" |
| The bruh MCP server answers a `tools/call` line without an `initialize` first, and exits when standard input closes. | Code read | `plugins/bruh/mcp/rpc.go`, `Serve` and `handle` |
| `role_settings_write` accepts callers `bigm` and `clanker-*` and returns `{"path":"<absolute path>"}`. | Code read | `plugins/bruh/mcp/roles.go` |
| `mail_read` moves each message file to `mail/<role key>/read/`; a message has `id`, `from`, `to`, `header`, `body`, `at`. | Code read | `plugins/bruh/mcp/mail.go` |
| A header must match `^(P[012] Q-\d+\|ANSWER Q-\d+\|REC Q-\d+\|RULE R-\d+\|DONE\|START): \S.{0,199}$` in lane go (`START` added there). The drivers use only `P2` and `DONE` headers, which both versions accept. | Code read | `plugins/bruh/mcp/mail.go`, `headerRE`, in this branch and in lane go |
| Telegram channel setup: `/plugin install telegram@claude-plugins-official`, `/telegram:configure <token>`, start with `--channels plugin:telegram@claude-plugins-official`, `/telegram:access pair <code>`, `/telegram:access policy allowlist`. The official channel plugins need Bun. | Docs | <https://code.claude.com/docs/en/channels.md> |
| A channel that is not on the allowlist loads with `--dangerously-load-development-channels plugin:<name>@<marketplace>`. | Docs | channels.md, "Research preview" |
| `orca serve [--port <port>] [--pairing-address <host>]` and `orca environment add --name <name> --pairing-code <code>`; `orca orchestration ask --question <text> --timeout-ms <n>`; `orca orchestration check --wait --timeout-ms <n>`; `orca orchestration run-create --objective <text>`; `orca orchestration worker-start --spec <text> --on <environment> --worktree new-top-level --repo <selector> --agent claude`; `orca orchestration worker-stop --dispatch <id>`; `orca environment show --environment <selector>`. | `--help` of Orca 1.4.216 | Command output |
| The official Scorecard workflow pins `actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1` (v7.0.1), `ossf/scorecard-action@2d1146689b8cda280b9bc96326124645441f03bc` (v2.4.4, the latest release), `actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` (v7.0.1), `github/codeql-action/upload-sarif@2892aa5e19bbd11bc0cff5427e3b750a04d9e3c2` (v4.38.2). | `git ls-remote` of each action repository matched each SHA to its tag | <https://github.com/ossf/scorecard/blob/main/.github/workflows/scorecard-analysis.yml> |
| The Scorecard badge is `https://api.scorecard.dev/projects/github.com/<owner>/<repo>/badge`; it redirects to a shields.io image (HTTP 302, then 200), also before the first Scorecard run. The release badge answers 200 with "no releases" before the first release. | `curl` of each badge URL | <https://github.com/ossf/scorecard-action> |
| `tests/test.sh` passes on macOS in `sh`, `dash`, and `bash --posix`, and on Linux (`debian:stable-slim` with `git` and `jq`). The functions use only portable flags: `sort -C`, `date -u -r` with a GNU `date -d` fallback, `grep -F -x -f`. | Test runs | `sh tests/test.sh`, `docker run debian:stable-slim` |
| `mcp_call` works against the real server: `mail_post` returns `{"at","id"}`, `role_settings_write` returns `{"path"}`, and an invalid header returns `isError` and exit 1. | A run against the server built from this branch, with a scratch data folder | Command output |
| No live Claude session was started for this plan. The drivers cannot run end to end before lanes go and roles are merged. | Not applicable | Not applicable |
| `orca orchestration inbox --json` returns `{"result":{"messages":[{"id","run_id","from_handle","to_handle","subject","body",...}]}}`; a message ID has the form `msg_<hex>`, a run ID `run_<hex>`, a dispatch ID `ctx_<hex>`. | A read of the local inbox | Command output, Orca 1.4.216 |
| `orca serve` has the options `--port`, `--pairing-address`, `--no-pairing`, `--mobile-pairing`, `--project-root`, and no option for the listen address. `--pairing-address` "changes only the client-advertised address". | `orca serve --help` | Command output, Orca 1.4.216 |
| `orca worktree rm --worktree <selector> [--force]` removes a worktree from Orca and git and tries to delete its branch; `worker-stop` never deletes the worktree; `worker-start` has `--name` and `--setup skip`. | `--help` of each command | Command output, Orca 1.4.216 |
| The image `ghcr.io/oter/autonomous-agents/agent` has the tag `2026-09-06` and no `latest` tag. | Review of fix round 1 (ghcr `tags/list`) | Reviewer evidence |
| `go run -C <dir> .` runs the program in `<dir>`, so a relative file argument does not resolve against the folder of the caller. | Review of fix round 1 (scratch module) | Reviewer evidence |
| The local toolchain: Claude Code 2.1.284, Orca 1.4.216, git 2.54. | `claude --version`, `orca --version`, `git --version` | Command output |

## Final review fixes (2026-09-30)

The final whole-branch review found 1 blocker, 7 major, and 17 minor findings. The rulings of the controller for the findings of this plan are below.

1. **M1, the load test sessions are pairs.** Premise changed: `mail_post` now accepts only the edges of the role tree (plan 2, final review fix 2), so the ring of sibling clerks of deviation 10 is refused. The load driver starts `--sessions` (an even number) sessions in pairs: `clanker-load-<run>-p<k>` and its clerk `clerk-load-<run>-p<k>-s`, and each session sends to its partner. The driver writes the role settings of each key as its parent, and the start messages as bigm. The semantics stay: each session gets a message each interval, the driver resumes a session with no process and counts missed messages. `tests/test.sh` replays each MCP call of the dry runs of both drivers against the real server, so a refused call fails the test. Agent-derived, needs owner decision. Options, ranked: (a) pairs, as built; (b) a star: one clanker session and its clerks, where the clanker answers each clerk (the clanker gets `n - 1` times the load); (c) keep the ring and let the driver post every message as bigm (it then does not test the sessions as senders).
2. **M3, the ledger of a remote machine.** Premise changed: the final review said option (b) of deviation 16 (an init option for a remote machine) is needed for remote merges. The merger clerk of a remote project now runs on the machine of bigm (plan 3, final review fix 3), so a remote machine needs no ledger, and deviation 16 stays (a), an empty local folder. Agent-derived, needs owner decision. Options, ranked: (a) an empty local folder, as built; (b) an init option for a remote machine with no ledger.
3. **m7, SECURITY.md.** The threat model says that roles are not isolated: any Bash command of any session can set `BRUH_ROLE_KEY` and call the scripts or the MCP server as another role, so the role checks of the MCP tools and the merge gate are speed bumps like deny rules. The list of files now names every file of the data folder and the `pluginConfigs` write to user settings, and it says that an init plan stays in the memory of the server. Agent-derived, needs owner decision (ruling of the controller, 2026-09-30). Options, ranked: (a) document the limit, as built; (b) bind each role to a secret per session that the server checks (a design change, not v0.1).
