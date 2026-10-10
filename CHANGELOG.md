# Changelog

All notable changes to this project are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.12.3] - 2026-10-10

### Added

- The review step of `/bruh:deliver` has a simplicity reviewer next to the adversarial reviewer and the invariant checker. It finds unrequested abstractions, code for a speculative need, a re-implemented standard library function or existing helper, an unneeded dependency, scaffolding for later, and a diff that is longer than the problem needs. Each finding names a concrete simpler replacement and goes through the same refuter and fix loop. What the task or a deliberate choice asks for is never a finding (task 45).
- A `simplicity` example lens for `/bruh:review-and-fix` and `/bruh:review-only` in `skills/implement/references/example-args.md`, and its guide `skills/implement/references/simplicity.md` with the ladder and the rules. The idea comes from the ponytail plugin; bruh does not need it installed (task 45).
- YAGNI and KISS rows under "Design principles" in `skills/implement/references/guide-sources.md` (task 45).
- New `ledger_edit` actions, so bigm writes no ledger file by hand (owner rule R-19, task 51): `add_line` and `close_line` add or delete one bullet line of a section such as "Decisions"; `add_rule` adds the next `R-<n>` section to `rules.md` with the date and the tag from the server clock, and `close_rule` deletes one; `new_project` makes `projects/<key>.md` from the template; `commit` commits the listed files that `init_apply` or `learn_refresh` wrote.
- A `PreToolUse` hook in the bigm start settings `<ledger>/.claude/settings.json` blocks each file write of bigm by hand: the Edit, Write, and NotebookEdit tools, and a shell write (a `>`, `>>`, `>|`, or `&>` redirect to a file, `tee`, `cp`, `mv`, or `sed -i`, also with a heredoc). It acts only when `BRUH_ROLE_KEY` is `bigm`, so `clerk-ledger` keeps its writes (task 51).

### Changed

- bigm.md has no hand commit: each ledger change of bigm goes through `ledger_edit`. Run `/bruh:init` again: it adds the write hook to an existing ledger settings file (task 51).
- `mail_post` appends the full current text of `priorities.md` and `rules.md` of the ledger to every `START` message, so no start message carries only a pointer to the rules (R-17). A START is refused when the ledger is configured but a file cannot be read. On a machine with no ledger, the output says `ledger_text` `none`, and the sender pastes the text from its own start message.

### Fixed

- A refusal P0 is no longer silent while its asker is held (#42): `question_open` with `hold` posts the P0 itself to the mailbox of the parent of the asker and of bigm, and their waiters wake them, so it needs no `SendMessage` nudge. The question file keeps the hold ID in the new field `hold`. On a post error, the output has `relay_error` and the asker sends the nudge itself.
- `answer_wait` returns the answer that any role wrote for the question ID with `answer_write` (bigm, the clanker, or the asker), not only the copy of the caller, so a workflow agent no longer stays pending until its clerk writes its own copy.
- The watcher sends an `error` event for a source only on its 3rd failed poll in a row, once for each outage, and one `recovered` event when the source works again. A single failed poll (a reset connection, a DNS miss) no longer wakes the role (task 52).
- The gates step of `/bruh:deliver` counts a Go test gate whose output has no test count (`ok <pkg>` or `ok <pkg> (cached)`) with a second plain run with `-count=1 -v`, so a passing Go gate no longer reports 0 tests ran. The gate list of the project does not change (task 49).
- At the round cap of `/bruh:deliver`, a gate finding that its refuter refuted stays refuted; only a real gate failure (exit not 0, a failed or skipped test) stays open (task 49).
- A clerk agent no longer freezes on a chained git command: in a clerk session, the new `PreToolUse` hook `git-chain.sh` denies a Bash command that has the word git together with `;`, `&&`, `||`, `|`, or `cd`, with a text that says to split the command, run each part alone, and go on. The deny sets no hold and opens no question. The gates step of `/bruh:deliver` gets the head SHA from the step that made the last commit and runs no git, and the implement prompt lists the gates (task 41 part 2, Q-207).
- The guard hold of `refusal-stop.sh` and `git-chain.sh` count git only as a command word, so a path such as `.claude/worktrees/no-git-freeze` no longer turns a guard refusal of a no-git command into a hold or a deny. `clanker.md` says that a task name and its branch never contain git (task 41b).

## [0.12.2] - 2026-10-09

### Added

- Each open question on the `/bruh-board` pane has a text box, `write your own answer`, under its buttons; a question without options gets only the text box. Enter sends the text of the owner to bigm word for word, and text in the box goes with a later press of a button of that question (task 42).

### Changed

- A press of an answer button on `/bruh-board` no longer submits a prompt into the session. It puts one quiet mail into the mailbox of bigm, `ANSWER Q-<id>: <label>` (or `ANSWER Q-<id>: own words` for the text box), with the body lines `QUESTION:`, `PICK:`, and `TEXT:`, and the waiter of bigm wakes it. bigm records the answer with `answer_write` and relays it as before. A toast says whether the mail was sent (task 42).
- `mail_post` refuses every `ANSWER` to `bigm`, also from bigm, so an `ANSWER` in the mailbox of bigm comes only from the board (task 42).

### Fixed

- The watcher reads a private Gitea repository through `tea api --login <login>` when `tea logins list` has a login for the host of `api_url`, so a host with a tea login no longer gets 403 "Only signed in user is allowed to call APIs.". No token passes through bruh. Without such a login, `BRUH_GITEA_TOKEN_<HOST>` works as before, else the call has no token.

## [0.12.1] - 2026-10-08

### Added

- Every question to the owner shows on the `/bruh-board` pane with its buttons (owner rule R-15). bigm opens each own ask with `question_open` first, and a local copy of each question of a remote clanker. A new plugin hook `owner-ask.sh` (`PreToolUse`, matcher `AskUserQuestion`) blocks a select of bigm unless the text of each of its questions names the `Q-<id>` of an open P0 or P1 question file with no answer file. Other roles pass.

### Changed

- The test suites are trimmed and grouped (task 33 test audit, [#41](https://github.com/oter/bruh/pull/41)): tests that checked nothing new are cut or folded into the tests that keep their checks, row tests are merged into tables that report every failing row, and five untested branches now have a test (the `handoff_percent` range, the slash rule of `lane.sh`, the empty default branch name, the empty-list guard of `remove_scratch`, and the tool names of the agents against the MCP server). Go `plugins/bruh/mcp` top-level tests 264 -> 241, node tests 168 -> 132, `tests/test.sh` 171 -> 91, all passing with 0 skipped. No production code changed.
- The `/bruh-board` pane draws what waits on the owner last (owner rule R-16): the open questions and their answer buttons sit below the clankers, the clerks, and the Done group in `cards`, `buckets`, and `pipeline`. `buckets` shows every open question under one WAITS ON YOU bar at the bottom, and no question inside the row of its clerk any more.
- bigm reports status in the order "Ready for you", "In progress", "Waiting on you", and shows a new P0 at the end of its next reply, not at the top (owner rule R-16).

## [0.12.0] - 2026-10-07

### Added

- The `/bruh-board` command opens a live pane. It shows one short line for each open P0 and P1 question, and one collapsed line for each clanker with a spinner and its task count. Enter on a clanker expands it to its clerks (task, spinner, and state), and Enter on a clerk expands it to its name, last done, and next step. The lines have no hotkeys: Tab and the arrows move the focus, and Esc gives the keys back to the prompt. The expanded state stays in the store of the mod, its only write.
- The `/bruh-board` pane has three designs from the same data: `cards` (bordered cards, the default), `buckets` (sections by state with soft coloured bars, a question inside the row of its clerk), and `pipeline` (a strip for each clerk across the deliver phases). Pick one in `/config` with `bruh: Board design` (the plugin option `board_design`); the open pane changes at once, with no restart.
- The `/bruh-board` pane has a Done group at the bottom: one collapsed line with the count of the done and stopped clerks and clankers. Enter expands it to one gray line for each; nothing is deleted.
- The `pipeline` design reads the top-level `phase` field of the report lines of a clerk (`plan`, `implement`, `review`, `fix`, `merge`, or `done`) and fills the strip up to the latest one. Without a phase line it shows `phase not reported`.
- The `/bruh-board` pane shows answer buttons for each open question: the options of the question, or `ok` and `hold` for a refusal P0. A press sends `Q-<id>: <label>` as a prompt to bigm.
- `report_write` takes an optional `phase` (`plan`, `implement`, `review`, `fix`, `merge`, or `done`) and stores it as the top-level field `phase` of the report line; any other value is an error. The agents of `/bruh:deliver` write `plan`, `implement`, `review`, `fix`, and `done` at each step, and `/bruh:review-and-fix` writes `review` and `fix`.

### Changed

- `/bruh:deliver` takes a guides folder in `args.guides` (an absolute path with `INDEX.md`): its two reviewers and its refuters must cite a guide rule, a house rule, or a failing scenario for each finding. The clerk builds the folder for each task from `guide-sources.md`, which has a new Design principles section (Go Proverbs, Effective Go, DRY, SOLID), into `/tmp/<clerk key>-guides/`.
- Init adds six deny rules to the start settings of bigm in `<ledger>/.claude/settings.json`: `Agent(claude-code-guide)`, `Agent(general-purpose)`, `Agent(Explore)`, `Agent(Plan)`, `WebFetch`, and `WebSearch`. bigm gets each fact from a clanker or its scout. Run `/bruh:init` again to add the rules.
- bigm gets a notice in its mailbox when a role writes a status or result report line, with a maximum of one unread notice for each role. bigm gives the owner a short update every 5 minutes while work runs.
- The waiter does not wake an idle session at its timeout any more. It waits up to 7 days for new mail and exits in silence at its limit, so only new mail wakes the session (hook timeout 604800 seconds).
- After a workflow run stops with `CONFLICT:`, the clerk starts a new run without `resumeFromRunId`, because a resume replays the cached stop. When the hold guard denies its next call with a hold ID, it opens a P0 with the `hold` field of `question_open`, not a P2 conflict question.
- bigm runs only bruh skills: init adds a `PreToolUse` hook for the `Skill` tool to `<ledger>/.claude/settings.json` that blocks each other skill, and bigm sends each ask of the owner that needs a skill, research, or project work to a clanker. Run `/bruh:init` again to get the hook.
- A scout clerk may clone a public repository and download a file for research, into `/tmp/<scout key>-*` only: `role_settings_write` adds two allow rules for the exact forms `git clone https://<url> /tmp/<scout key>-<name>` and `curl -fsSL -o /tmp/<scout key>-<name> https://<url>`, plus guard deny rules for options, quotes, variables, `..`, file URLs, redirects, and home folders. A scout still never pushes or commits.
- `answer_write`, `mail_post`, and `question_open` replace `{now}` in the text or body with the server time, so bigm, the clankers, and the clerks run no `date -u` for these calls.
- From a worktree, a clerk and a clanker never run `git -C <main checkout>`: they run git inside their own worktree, or read other refs through `origin/<branch>`. The `root` of a clerk's implement or review workflow is a worktree, never the main checkout.

### Removed

- The house rule "In a worktree, no compound commands with git. Data goes through tool inputs." (item 10 of `defaults/house-rules.md`). Item 10 keeps the lane text for file reads, writes, and searches.
- The lease guard hook, the post script of review results, and the shell wrapper of the poller. The plugin monitor `bruh-poller` runs the Go `watch` command, which polls only in bigm. A clerk posts a review result with `gh pr review` or `glab mr note create` after the yes of the owner or under a post grant.
- The merge train (`scripts/merge-train.sh` and the `merge-train` command), the merger clerk `clerk-<project>-merge`, and `merge_method` of `repos_set`. After an owner approval (a recorded `ANSWER` or a merge grant), the clanker of the project merges with `gh pr merge <n> --repo <owner/repo> --merge --match-head-commit <sha>` (`glab` or `tea` for other hosts). Then it reads the state merged and the merge commit from the code host API. A `repos.json` file that has `merge_method` still loads.

### Fixed

- The board and bigm count a question as open only while no role folder has its answer. A clanker records each delegated or relayed answer with `answer_write`, and a replaced duplicate closes with the answer of its replacement (new `replaces` input of `question_open`; a repeat of the same refusal links its older P0 itself). bigm counts the open questions with the new tool `question_list`. Run `/bruh:init` again so that the user settings allow it.
- A held scout clerk no longer deadlocks: it has no `question_open`, so the hold guard tells it to send `DONE: scout <subject> refused` to its clanker with `mail_post`, and the `Stop` hook lets it stop.
- A Claude Code worktree guard refusal of a git command now sets a hold, like a classifier refusal: a `PostToolUseFailure` hook of `refusal-stop.sh` writes it, so `ExitWorktree` and another form of the refused command are denied until the answer. A guard refusal of a command with no git still sets no hold.
- A role reads its mail only through `mail_read`: every role settings file (bigm start settings, clanker, clerk, scout) denies `Read` (which also covers Grep, Glob, and `cat`-style Bash reads) and Bash commands that name `<data>/mail`, and `session_list` gives each role an `unread_mail` count for the idle check of bigm. The rules are a speed bump, not a sandbox. Run `/bruh:init` again, and write the other role settings again, to get them.
- `/bruh:deliver` and `/bruh:review-and-fix` close a finding by its stable ID (`F<n>`), not by file and line, so a fix that moves the line of its finding no longer leaves it open and the run ends `done`.
- A finished one-shot role is not woken any more: the clanker runs `claude stop <id>` after it accepts a clerk result or reads the `DONE` of a scout, a clerk ends its turn after `task closed` and its `DONE: <task> closed` mail, a scout after its `DONE`, and the waiter exits at once when the last report line of its role is `task closed`.
- bigm also loads the global skills of the owner (the names under `~/.claude/skills` that have a `SKILL.md`, such as `bro`); the `Skill` hook still blocks each skill of another plugin. Run `/bruh:init` again, which replaces the old `Skill` hook.
- A codehost monitor with `number` sends one `checks` event when the checks of the pull request head finish, success or failure, with a count of the results (GitHub only), so a clerk that waits on CI gets an event.
- A no-verdict denial of the auto mode classifier (`reason` `Classifier unavailable`, or one that starts with `Auto mode could not evaluate this action`) sets no hold any more, so the single retry that Claude Code allows is not denied. A classifier refusal still sets a hold.

## [0.11.1] - 2026-10-05

### Changed

- A refusal does not stop bigm. After a refusal, the hook denies only the exact refused call of bigm, and bigm continues all other work. bigm must open the P0 before its turn ends. A held clanker or clerk stays blocked until bigm records the answer of the owner.
- `ledger_edit` does not return a nudge. The waiter of `clerk-ledger` wakes the clerk when the DONE mail arrives, so bigm does not send a `SendMessage` nudge to `clerk-ledger`. This is also true after a hand commit and its `mail_post`.

### Removed

- The git-shape guard. The hook does not deny a compound Bash command with the word git in a linked worktree, and it does not write a hold for it. The Claude Code worktree guard, permission modes, and classifier stay.

### Fixed

- The learner eval does not inherit BRUH_ROLE_KEY. Before, a learner eval that started in a role session did not stop.
- A second poller waits for the poller lock and starts to poll when the first poller stops. Before, the second poller stopped at once, and the machine had no poller after the first bigm stopped.

## [0.11.0] - 2026-10-05

### Added

- When a session finds a defect of bruh itself, the owner gets the offer of a bug report with the final text of the issue. The session files the issue only after the owner says yes. A vulnerability gets a private report. A clanker or a clerk sends a notice to its parent, and bigm makes the offer.
- Scout clerks. bigm does not read the sources of a project itself: for a status question, it sends `DONE: info request <subject>` to the clanker of the project. The clanker starts a short-lived, read-only scout clerk `clerk-<project>-scout<n>`, checks its claims, and replies with `DONE: info <subject>`. bigm answers from the reply and reads the source of each status claim again. The MCP server adds the read-only deny rules to each scout settings file, also for `git fetch` and the `git -C <path>` form, refuses a scout report line without its source read, uses each scout key once, and never resumes a scout.
- A refusal stops the session. After a classifier refusal, or a deny of the new git-shape guard, a hold denies every other tool call of the session until the owner answers the P0. Only the escalation tools stay open. `question_open` with `hold` opens that P0 with the exact command and the refusal category, and `answer_write` of the P0 clears the hold, by bigm or by a clanker that relays the answer. In a session with a hold, only bigm may call `answer_write`. In a linked worktree, the git-shape guard denies a compound Bash command that names git; one plain git command with separators only inside quotes or in `2>&1` passes. New house rules: no compound commands with git in a worktree, and run settings go straight into the tool call.
- The routing eval (`tests/eval/run.sh routing`): a run of bigm on fixed work requests that checks the commitment line and that bigm asks no question. Run it before each release.
- The MCP tool `ledger_edit`: bigm adds, updates, or closes a table row, or sets a `key: value` line, of a ledger file in one call. The tool commits only that file, writes the DONE mail to `clerk-ledger`, and returns the nudge. It never pushes. Run `/bruh:init` again to allow the tool.
- On-demand monitors (spec 9.5): the MCP tools `monitor_start`, `monitor_stop`, `monitor_list`, and `monitor_report`. A local role starts a monitor on a code host repository, on a read-only CLI that prints JSON, or on an MCP tool that it polls itself, and each change reaches its mailbox as `DONE: event <project>: <subject>`. A scout clerk cannot call `monitor_start`, `monitor_stop`, or `monitor_report`.
- The waiter `scripts/wake.sh`: a `Stop` hook and a `SessionStart` hook with `asyncRewake` wake an idle role when its mailbox gets new mail.
- The plugin monitor `bruh-poller` (`monitors/monitors.json`) runs the poller in bigm, with a lock for one poller on each machine.
- The ledger file `monitors.md`, the table "Command grants" of `grants.md` (a `command` source runs only under a grant of its exact `argv` prefix), and the settings `monitor_default_hours`, `monitor_max_hours`, and `monitor_max_active` of `mode.md`.
- Local sessions in Orca: when the Orca app runs, `session_launch` and `session_resume` open an Orca tab titled with the role key that runs `claude attach <short ID>`. The new tool `session_tab_close` closes the tab of a retired role. The plugin option `orca_local` (`auto` or `off`, default `auto`) turns it off, and the init trust step offers to add the repositories to Orca. Without Orca nothing changes (spec 4.3).

### Changed

- The watcher is the poller of the monitors, in a plugin monitor of bigm instead of a `Monitor` tool watch. Each repository of `repos_set` is a standing monitor of the clanker of its project, and a local clanker gets the events in its mailbox with no relay of bigm. bigm relays only the events of remote clankers. Run `/bruh:init` again after the update: it adds the allow rules of the new tools and the new ledger parts.
- bigm and each clanker route work to a lane themselves and state the commitment, for example `I send <work> to clanker-<project> as task <n>.` They do not ask the owner which agent, clanker, or clerk does the work. The owner observes and can say no. Owner decisions, such as a merge or a scope change, still go to the owner.
- bigm tells the owner a change of the ledger in plain words, with no commit SHA.

### Fixed

- `session_launch` and `session_resume` find the session ID when `claude --bg` prints it with ANSI color codes. The error texts show the output without escape codes.
- `learn_refresh` lists in `long_files` only the Markdown files at the top of the ledger folder and in `projects/`. Notes in another folder, such as `research/`, are not ledger state and no longer show up.
- The test row `TestOrcaNotReady/timeout` uses a 2 s timeout, so that it is stable under `go test -race`.

## [0.10.0] - 2026-10-03

### Changed

- The marketplace name is `oter`. Install with `claude plugin install bruh@oter`.

### Fixed

- Init removes a bruh status line tap of another plugin data folder before it wraps the status line command again.

## [0.9.0] - 2026-10-03

This is the first release of bruh. It contains all parts of the [specification](docs/spec.md) version 0.6.

### Added

- Go fuzz tests of the role key parser, the sender policy of `mail_post`, and the handoff check. CodeQL scans each push and pull request. CI installs Claude Code from a lockfile with hashes (OpenSSF Scorecard).
- Plugin marketplace `bruh` with one plugin, `bruh` (spec 17).
- Role agents `bruh:bigm`, `bruh:clanker`, and `bruh:clerk`, with the launch settings of spec 3.1 and the role keys of spec 3.2.
- The bruh MCP server in Go (standard library only), started with `go run`. Tools: `mail_post`, `mail_read`, `handoff_write`, `handoff_read`, `answer_write`, `answer_wait`, `report_write`, `report_read`, `role_settings_write`, `lease_define`, `lease_request`, `lease_grant`, `lease_release`, `lease_list`, `question_open`, `session_launch`, `session_resume`, `session_list`, `init_plan`, `init_apply`, `repos_set`, `result_save`, and `bruh_info`.
- Hook scripts: the status line tap, the handoff message, the handoff pickup after compaction, and the lease guard (spec 7 and 8.4).
- The init skill `/bruh:init`, with an interactive form and a non-interactive form for a container (spec 16).
- The `deliver` workflow `/bruh:deliver`: plan, implement, review with three checks, and fix, with questions during a run (spec 6).
- The implement skillset (spec 6.4): the skill `/bruh:implement` with its references, the workflows `/bruh:tickets`, `/bruh:implement-tickets`, `/bruh:review-and-fix`, and `/bruh:review-only`, and the scripts `lane.sh` (a private copy of the tree for each ticket) and `post-findings.sh` (posts a saved review result on a GitLab merge request or a GitHub pull request, only with the yes of the owner or under a post grant). The ledger template `grants.md` has a "Post grants" section. The MCP tool `result_save` saves a workflow result in the data folder for `post-findings.sh`, and the watcher counts the marker `<!-- bruh:owner -->` as an agent post.
- The sweep, the code host watcher, and the merge train (spec 8.3 and 9).
- The remote transport through the Orca remote runtime (spec 4.2).
- Channels: Telegram through the official channel plugin, and a Slack channel of bruh that polls the Slack Web API (spec 12).
- Default `priorities.md`, `house-rules.md`, role settings, and a ledger template (spec 8, 13, and 14).
- Tests: Go tests for the MCP server and the hook scripts, Node tests for the control flow of the workflows, shell tests for `lane.sh` and `post-findings.sh` with a fake `glab` and `gh`, a smoke test runbook and driver (`tests/smoke/`), and a load test runbook and driver (`tests/load/`) (spec 20).
- Repository files: CI (the Go checks, the shell tests, and the Node tests on macOS and Linux; lint, the link check, and plugin validation on Linux), OpenSSF Scorecard, Dependabot, issue forms, a pull request template, `SECURITY.md`, `CONTRIBUTING.md`, and the Apache-2.0 license (spec 19).
- The index of the ledger: `learn/tree.json` and `learn/projects/<key>.json`. The MCP tools `learn_scan` and `learn_refresh` write and refresh the index (spec 8.5).
- The plugin agent `bruh:learner` and its eval set in `tests/eval/` (spec 8.5 and 20).
- GitLab support through `glab`: the client, `repos_set`, the watcher, the merge train, and `post-findings.sh --hostname`.
- P1 options in `question_open`.
- The inputs `subject` and `asker` of `answer_write`.
- The input `allow` of `role_settings_write`.
- The init steps: ledger, hosts, pick, learn, confirm, diff with the ledger table, and trust (spec 16).
- The ledger size check.

### Changed

- A question ID has the form `Q-<project>-<host>-<n>`. When an asker uses an ID again with another subject, bigm asks again with `ANSWER Q-<id>: reask`.
- The watcher writes each event to the report file of the clanker of its project.
- Init updates the template text of an existing ledger and keeps its rows.
- A post grant names the host name.
- The titles of the plugin options start with "bruh: ".
- `repos_set` requires `project`.

### Removed

- The init answer keys `delegated_p1_classes`, `merge_grants`, and `remote_environments`. bigm records these values.
- `init_plan` does not write the plugin options (`pluginConfigs`).
- The report file `reports/watcher.jsonl` and the role key `watcher` of `report_read`.

[Unreleased]: https://github.com/oter/bruh/compare/v0.12.3...HEAD
[0.12.3]: https://github.com/oter/bruh/compare/v0.12.2...v0.12.3
[0.12.2]: https://github.com/oter/bruh/compare/v0.12.1...v0.12.2
[0.12.1]: https://github.com/oter/bruh/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/oter/bruh/compare/v0.11.1...v0.12.0
[0.11.1]: https://github.com/oter/bruh/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/oter/bruh/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/oter/bruh/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/oter/bruh/releases/tag/v0.9.0
