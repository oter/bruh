# Smoke test

The smoke test runs the full chain of bruh once, in a scratch repository, before each release (spec section 20). bigm starts one clanker, and the clanker starts two clerks. A workflow agent asks a P1 question, and the answer comes back in the same run. A clerk that waits on a permission prompt raises a P0. A stopped clanker resumes on a message. The result goes into the release notes.

`run.sh` is the driver. It starts real Claude Code sessions, so it uses your Claude account and your usage limits.

## One-time setup: a trusted repository

`claude --bg` refuses a folder that is not trusted. Trust is inherited by the subfolders of a trusted folder, but a new `git init` folder is a new trust boundary. A linked worktree of a trusted repository is trusted. So the driver makes each scratch folder a linked worktree of one repository that you trusted before.

Create this repository once. It must have no remote, because a test clerk pushes its branch. The driver refuses a repository with a remote.

```bash
mkdir -p <scratch folder>/bruh-smoke-home
cd <scratch folder>/bruh-smoke-home
git init
git commit --allow-empty -m "Home of the bruh smoke tests"
claude
```

Accept the trust dialog, then exit Claude Code. Do not use this repository for other work.

## Run

Run the driver from the root of the bruh checkout. It uses the plugin of the checkout (`--plugin-dir plugins/bruh`), not an installed copy.

```bash
BRUH_TRUSTED_REPO=<scratch folder>/bruh-smoke-home sh tests/smoke/run.sh
```

To see the commands without a run, add `--dry-run`. The dry run starts no session and writes no file.

```bash
BRUH_TRUSTED_REPO=<scratch folder>/bruh-smoke-home sh tests/smoke/run.sh --dry-run
```

Before the run:

- Stop your own bigm. The driver refuses to start when a live session named `bigm` runs, because the nudges of the test go to the name `bigm`.
- The driver refuses to start when the plugin data folder of the checkout (`~/.claude/plugins/data/bruh-inline/`) already has bigm state, such as `mail/bigm/` or `handoffs/bigm.md`.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `BRUH_TRUSTED_REPO` | none, required | The trusted repository of the one-time setup |
| `SMOKE_STEP_MINUTES` | 30 | The timeout of each step |
| `SMOKE_SWEEP_MINUTES` | 15 | The sweep interval of bigm. The P0 step waits this time plus 5 minutes. |
| `SMOKE_IDLE_MINUTES` | 0 | 0: the driver stops the clanker with `claude stop`. A value above 60, for example 65: the driver waits for the idle stop of the supervisor. |
| `SMOKE_ORCA_ENV` | none | A paired Orca environment for the remote step |
| `SMOKE_ORCA_REPO` | none | The Orca repository selector for the remote step, for example `name:<repository>` |
| `SMOKE_EVIDENCE` | `$TMPDIR/bruh-smoke-<run>` | The folder for the results and the session logs |
| `SMOKE_RUN` | random | The run ID. It is part of each role key and branch name. |
| `BRUH_TEST_DATA` | `~/.claude/plugins/data/bruh-inline` | The plugin data folder of the checkout |

## What the driver does

Setup:

1. It builds the MCP server of the checkout into the evidence folder. The driver uses it as a stand-in bigm caller: it sends one JSON-RPC `tools/call` line for each call. So plugin code writes all bruh files.
2. It adds two linked worktrees of `BRUH_TRUSTED_REPO` on new orphan branches: a project and a ledger.
3. The project gets `TASK.md` with two tasks (`greet` and `gate`), `gate.sh`, and `.claude/settings.json` with the allow rules `Workflow(bruh:deliver)` and `mcp__plugin_bruh_bruh`, ask rules for `./gate.sh`, and the deny rule `Bash(git push:*)`.
4. The ledger gets the ledger template of the plugin, with `mode: autonomous`, so that bigm decides the P1 question itself.
5. It writes the bigm role settings with `role_settings_write`, and starts bigm in the ledger:

   ```bash
   CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --agent bruh:bigm --name bigm \
     --permission-mode auto --settings <data>/roles/bigm.json --plugin-dir plugins/bruh "<start prompt>"
   ```

Steps:

| Step | PASS when |
|---|---|
| `preflight` | the tools are present, Claude Code is 2.1.284 or later, the trusted repository has a commit and no remote, and no other bigm runs |
| `bigm` | a session named `bigm` has a process within 2 minutes |
| `clanker` | bigm started a session named `clanker-smoke-<run>` |
| `clerk` | the clanker started `clerk-smoke-<run>-greet` and `clerk-smoke-<run>-gate` |
| `deliver` | `claude logs` of the greet clerk contains `bruh:deliver` |
| `p1-sent` | a file `questions/Q-<n>.json` with priority `P1` names the greet clerk |
| `p1-bigm` | a message with the header `P1 Q-<n>:` is in the mailbox of bigm |
| `answer-same-run` | the answer file of the question exists, the greet clerk sent `DONE:` to the clanker, and its log does not contain `resumeFromRunId` |
| `p0-prompt` | the gate clerk waits on a permission prompt, and within one sweep the text `claude attach <gate clerk id>` is in a P0 question, a P0 message to bigm, a ledger file, or the output of bigm |
| `idle-resume` | after the clanker process stops, the stand-in bigm posts a message and calls `session_resume`; the clanker gets a process under the same session ID and reads the message |
| `remote` | `SKIP` when `SMOKE_ORCA_ENV` or `SMOKE_ORCA_REPO` is not set, or when the environment is not paired. Else a worker on the environment asks `P1 Q-1: smoke remote <run>` with `orca orchestration ask`, and the question is in `orca orchestration inbox`. Run the driver in an Orca terminal for this step. |

A step that needs a failed step prints `SKIP <step> - needs <step>`.

Cleanup (also after a failure or Ctrl+C):

1. It saves `claude logs` of each session of the run to the evidence folder.
2. It stops each session whose name belongs to the run and whose folder is in the run folder, and checks that none has a process.
3. It removes the scratch worktrees and the branches that the run created in `BRUH_TRUSTED_REPO`.
4. It removes the bruh data files of the role keys of the run.

## Read the result

The driver prints one line for each step, and writes the same lines to `results.txt` in the evidence folder:

```text
PASS preflight - Claude Code 2.1.284
PASS bigm
PASS clanker
...
SKIP remote - SMOKE_ORCA_ENV or SMOKE_ORCA_REPO is not set, so no Orca environment is paired for this run
```

The exit code is 0 when no step failed. A `FAIL` line stops the release. Read the session logs in the evidence folder to find the cause.

## Release notes

Put `results.txt` into the release notes of the release (see the release checklist in [plan 6](../../docs/superpowers/plans/2026-09-30-plan-6-verification-release.md)). Give the Claude Code version, and say which steps were skipped and why.

## Limits

- bigm runs as a background session, not in a terminal, because a script cannot own a terminal. The scratch ledger is a linked worktree, so bigm does not move into a new worktree.
- The test uses autonomous mode. The path where you answer in the bigm terminal is not in the test.
- The allow rules come from the scratch project, not from `/bruh:init`.
- The two clerks share the scratch project worktree.
