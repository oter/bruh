# Changelog

All notable changes to this project are recorded in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

The first release, `v0.1.0`, contains all parts of the [specification](docs/spec.md) version 0.4.

### Added

- Plugin marketplace `bruh` with one plugin, `bruh` (spec 17).
- Role agents `bruh:bigm`, `bruh:clanker`, and `bruh:clerk`, with the launch settings of spec 3.1 and the role keys of spec 3.2.
- The bruh MCP server in Go (standard library only), started with `go run`. Tools: `mail_post`, `mail_read`, `handoff_write`, `handoff_read`, `answer_write`, `answer_wait`, `report_write`, `report_read`, `role_settings_write`, `lease_define`, `lease_request`, `lease_grant`, `lease_release`, `lease_list`, `question_open`, `session_launch`, `session_resume`, `session_list`, `init_plan`, `init_apply`, `repos_set`, and `bruh_info`.
- Hook scripts: the status line tap, the handoff message, the handoff pickup after compaction, and the lease guard (spec 7 and 8.4).
- The init skill `/bruh:init`, with an interactive form and a non-interactive form for a container (spec 16).
- The `deliver` workflow `/bruh:deliver`: plan, implement, review with three checks, and fix, with questions during a run (spec 6).
- The sweep, the code host watcher, and the merge train (spec 8.3 and 9).
- The remote transport through the Orca remote runtime (spec 4.2).
- Channels: Telegram through the official channel plugin, and a Slack channel of bruh that polls the Slack Web API (spec 12).
- Default `priorities.md`, `house-rules.md`, role settings, and a ledger template (spec 8, 13, and 14).
- Tests: Go tests for the MCP server and the hook scripts, a smoke test runbook and driver (`tests/smoke/`), and a load test runbook and driver (`tests/load/`) (spec 20).
- Repository files: CI (the Go checks, the shell tests, and the Node tests on macOS and Linux; lint, the link check, and plugin validation on Linux), OpenSSF Scorecard, Dependabot, issue forms, a pull request template, `SECURITY.md`, `CONTRIBUTING.md`, and the Apache-2.0 license (spec 19).

[Unreleased]: https://github.com/oter/bruh/commits/main
