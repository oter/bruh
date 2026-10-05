# Changelog

All notable changes to this project are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- When a session finds a defect of bruh itself, the owner gets the offer of a bug report with the final text of the issue. The session files the issue only after the owner says yes. A vulnerability gets a private report. A clanker or a clerk sends a notice to its parent, and bigm makes the offer.
- Scout clerks. bigm does not read the sources of a project itself: for a status question, it sends `DONE: info request <subject>` to the clanker of the project. The clanker starts a short-lived, read-only scout clerk `clerk-<project>-scout<n>`, checks its claims, and replies with `DONE: info <subject>`. bigm answers from the reply and reads the source of each status claim again. The MCP server adds the read-only deny rules to each scout settings file, also for `git fetch` and the `git -C <path>` form, refuses a scout report line without its source read, uses each scout key once, and never resumes a scout.
- The routing eval (`tests/eval/run.sh routing`): a run of bigm on fixed work requests that checks the commitment line and that bigm asks no question. Run it before each release.
- The MCP tool `ledger_edit`: bigm adds, updates, or closes a table row, or sets a `key: value` line, of a ledger file in one call. The tool commits only that file, writes the DONE mail to `clerk-ledger`, and returns the nudge. It never pushes. Run `/bruh:init` again to allow the tool.
- On-demand monitors (spec 9.5): the MCP tools `monitor_start`, `monitor_stop`, `monitor_list`, and `monitor_report`. A local role starts a monitor on a code host repository, on a read-only CLI that prints JSON, or on an MCP tool that it polls itself, and each change reaches its mailbox as `DONE: event <project>: <subject>`.
- The waiter `scripts/wake.sh`: a `Stop` hook and a `SessionStart` hook with `asyncRewake` wake an idle role when its mailbox gets new mail.
- The plugin monitor `bruh-poller` (`monitors/monitors.json`) runs the poller in bigm, with a lock for one poller on each machine.
- The ledger file `monitors.md`, the table "Command grants" of `grants.md` (a `command` source runs only under a grant of its exact `argv` prefix), and the settings `monitor_default_hours`, `monitor_max_hours`, and `monitor_max_active` of `mode.md`.

### Changed

- The watcher is the poller of the monitors, in a plugin monitor of bigm instead of a `Monitor` tool watch. Each repository of `repos_set` is a standing monitor of the clanker of its project, and a local clanker gets the events in its mailbox with no relay of bigm. bigm relays only the events of remote clankers. Run `/bruh:init` again after the update: it adds the allow rules of the new tools and the new ledger parts.
- bigm and each clanker route work to a lane themselves and state the commitment, for example `I send <work> to clanker-<project> as task <n>.` They do not ask the owner which agent, clanker, or clerk does the work. The owner observes and can say no. Owner decisions, such as a merge or a scope change, still go to the owner.
- bigm tells the owner a change of the ledger in plain words, with no commit SHA.

### Fixed

- `session_launch` and `session_resume` find the session ID when `claude --bg` prints it with ANSI color codes. The error texts show the output without escape codes.
- `learn_refresh` lists in `long_files` only the Markdown files at the top of the ledger folder and in `projects/`. Notes in another folder, such as `research/`, are not ledger state and no longer show up.

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

[Unreleased]: https://github.com/oter/bruh/compare/v0.10.0...HEAD
[0.10.0]: https://github.com/oter/bruh/compare/v0.9.0...v0.10.0
[0.9.0]: https://github.com/oter/bruh/releases/tag/v0.9.0
