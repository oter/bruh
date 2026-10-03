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

If you cannot make a new trusted repository, you can use a trusted repository that has a remote. Set `SMOKE_ALLOW_REMOTE=1`. The driver then puts `GIT_CONFIG_*` variables into the `env` of the scratch project settings and the scratch ledger settings. These variables set the push URL of each remote to a path that does not exist, so a `git push` of a session of the run fails. The run branches are local branches of that repository until the cleanup deletes them.

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

- Stop your own bigm and your own `clerk-ledger`. These two role keys are fixed, and a nudge goes to a session name. The driver refuses to start when the name of a live session starts with `bigm`, `clerk-ledger`, `clanker-smoke-<run>`, or `clerk-smoke-<run>-`.
- Claude Code gives a session with no name the name `<folder name>-<hex>`. When the name of your ledger folder starts with `bigm`, each open session in that folder stops the run. Before the run, close or rename each session whose name starts with `bigm`.
- The driver refuses to start when the plugin data folder of the checkout (`~/.claude/plugins/data/bruh-inline/`) already has state of `bigm` or `clerk-ledger`, such as `mail/bigm/` or `handoffs/bigm.md`.

## Environment variables

| Variable | Default | Meaning |
|---|---|---|
| `BRUH_TRUSTED_REPO` | none, required | The trusted repository of the one-time setup |
| `SMOKE_STEP_MINUTES` | 30 | The timeout of each step |
| `SMOKE_SWEEP_MINUTES` | 15 | The time that the P0 step allows for one sweep, plus 5 minutes. The sweep of bigm runs every 15 minutes, and this value does not change it. Do not set a lower value. |
| `SMOKE_IDLE_MINUTES` | 0 | 0: the driver stops the clanker with `claude stop`. A value above 60, for example 65: the driver waits for the idle stop of the supervisor. |
| `SMOKE_ALLOW_REMOTE` | 0 | 1: accept a trusted repository that has a remote, and block pushes with `GIT_CONFIG_*` variables in the scratch settings |
| `SMOKE_RUN` | random | The run ID: 1 to 12 lowercase letters and digits. It is part of each role key, folder, and branch of the run. |
| `SMOKE_ORCA_ENV` | none | A paired Orca environment for the remote step |
| `SMOKE_ORCA_REPO` | none | The Orca repository selector on the remote machine for the remote step, for example `name:<repository>`. Use a repository with no remote. |
| `SMOKE_EVIDENCE` | `$TMPDIR/bruh-smoke-<run>` | The folder for the results and the session logs |
| `BRUH_TEST_DATA` | `~/.claude/plugins/data/bruh-inline` | The plugin data folder of the checkout |

## What the driver does

Setup:

1. It builds the MCP server of the checkout into the evidence folder. The driver uses it as a stand-in bigm caller: it sends one JSON-RPC `tools/call` line for each call. So plugin code writes all bruh files.
2. It adds two linked worktrees of `BRUH_TRUSTED_REPO` on new orphan branches: a project and a ledger.
3. The project gets `TASK.md` with two tasks (`greet` and `gate`), `gate.sh`, and `.claude/settings.json` with the allow rules `Workflow(bruh:deliver)` and `mcp__plugin_bruh_bruh`, ask rules for `./gate.sh`, and the deny rule `Bash(git push:*)`.
4. The ledger gets the ledger template of the plugin, with `mode: autonomous`, so that bigm decides the P1 question itself. Its `.claude/settings.json` has the key `"agent": "bruh:bigm"` and the env value `BRUH_ROLE_KEY=bigm`, as `/bruh:init` writes them.
5. It writes the bigm role settings with `role_settings_write`, and starts bigm in the ledger. The start prompt also tells bigm what to do when a session sends it `SMOKE-IDLE <word>` (the idle-resume step):

   ```bash
   CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --agent bruh:bigm --name bigm \
     --permission-mode auto --settings <data>/roles/bigm.json --plugin-dir plugins/bruh "<start prompt>"
   ```

Steps:

| Step | PASS when |
|---|---|
| `preflight` | the tools are present, Claude Code is 2.1.284 or later, the trusted repository has a commit and no remote, and no other bigm runs |
| `init-index` | the driver makes the plain repository `root/shop<run>` in the run folder, with a `README.md`, the remote `origin` on `example.com`, and `refs/remotes/origin/HEAD`. It runs `claude -p --agent bruh:learner` on it, turns the `PURPOSE:` and `DOC:` lines into fills, and runs the non-interactive `init --answers` with a scratch ledger, settings file, and data folder in the evidence folder. PASS when `learn/projects/shop<run>.json` of the scratch ledger has the remote `origin`, a default branch that is not `unknown`, a purpose with the source `agent`, and the doc pointer `README.md` |
| `plain-start` | a plain `claude -p` in the ledger, with no tool and no MCP server, answers exactly `bigm` when the driver asks for the role name of its system instructions. This proves that the agent key of the ledger settings starts bigm without `--agent`. The step does not probe the role key: when it is missing, each bruh tool call of bigm fails with "BRUH_ROLE_KEY is not set" |
| `bigm` | a session named `bigm` has a process within 2 minutes |
| `clanker` | bigm started a session named `clanker-smoke-<run>` |
| `clerk` | the clanker started `clerk-smoke-<run>-greet` and `clerk-smoke-<run>-gate` |
| `deliver` | `claude logs` of the greet clerk contains `bruh:deliver` |
| `p1-sent` | a file `questions/Q-<id>.json` with priority `P1` names the greet clerk |
| `p1-bigm` | a message with the header `P1 Q-<id>:` is in the mailbox of bigm |
| `answer-same-run` | the answer file of the question exists, the greet clerk sent `DONE:` to the clanker, its log does not contain `resumeFromRunId`, and the word in `GREETING.txt` (on a new branch or in a clerk worktree) is in the answer |
| `p0-prompt` | the gate clerk waits on a permission prompt, and within one sweep the text `claude attach <gate clerk id>` is in a P0 question, a P0 message to bigm, a ledger file, or the output of bigm |
| `idle-resume` | the driver stops the clanker, then a short `claude -p` session sends `SMOKE-IDLE <word>` to bigm. This session loads only the user settings, so the agent key of the ledger does not make it a bigm. It has no Bash, because it does not get the push block of the ledger settings. The real smoke bigm, not the driver, sends `DONE: smoke idle check <word>` to the clanker. PASS when the clanker has a process again under the same session ID, the message from bigm is in its `read/` folder, and the output of bigm shows `session_resume` (or `respawn`, when the driver stopped the clanker with `claude stop`) |
| `remote` | `SKIP` when `SMOKE_ORCA_ENV` or `SMOKE_ORCA_REPO` is not set, or when the environment is not paired. Else the driver starts a worker on the environment (worktree `bruh-smoke-<run>`, no setup hooks) that starts a `claude -p --agent bruh:clanker`. The clanker asks `P1 Q-smoke-remote-1: smoke remote <run>` with `orca orchestration ask`. The driver replies with a new random word. PASS when a message in `orca orchestration inbox` carries the rot13 form of that word, which only a session that got the reply can make. Run the driver in an Orca terminal for this step. |

A step that needs a failed step prints `SKIP <step> - needs <step>`.

Cleanup (also after a failure or Ctrl+C):

1. It saves `claude logs` of each session of the run to the evidence folder.
2. It stops each session whose folder is in the run folder, and checks that none has a process. These are bigm, `clerk-ledger`, the clanker, the task clerks, a merger clerk, and the `claude -p` sessions of the driver.
3. It removes the scratch worktrees. It deletes each new branch in `BRUH_TRUSTED_REPO` that does not contain the HEAD commit of that repository from before the run. A run branch starts from an orphan commit, so a branch that somebody made from HEAD during the run stays.
4. It removes the bruh data files of the role keys of the run, including `bigm` and `clerk-ledger`.
5. For the remote step, it stops the worker and removes the remote worktree with `orca worktree rm`.

## Read the result

The driver prints one line for each step, and writes the same lines to `results.txt` in the evidence folder:

```text
PASS preflight - Claude Code 2.1.284
PASS bigm
PASS clanker
...
SKIP remote - SMOKE_ORCA_ENV or SMOKE_ORCA_REPO is not set, so no Orca environment is paired for this run
```

The exit code is 0 when no step failed. A `FAIL` line stops the release. A `SKIP` line exits 0, but a release with `SKIP remote` needs the go of the owner. Read the session logs in the evidence folder to find the cause.

## Release notes

Put `results.txt` into the release notes of the release (see the release checklist in [plan 6](../../docs/superpowers/plans/2026-09-30-plan-6-verification-release.md)). Give the Claude Code version, and say which steps were skipped and why.

## Limits

- bigm runs as a background session, not in a terminal, because a script cannot own a terminal. The scratch ledger is a linked worktree, so bigm does not move into a new worktree.
- The test uses autonomous mode. The path where you answer in the bigm terminal is not in the test.
- The allow rules come from the scratch project, not from `/bruh:init`.
- Spec 20 has one clerk. The test has two clerks, because one task asks the P1 question and the other task waits on the permission prompt.
- Each clerk makes its own worktree with `EnterWorktree` under `.claude/worktrees/` of the scratch project.
- The idle stop is simulated with `claude stop` by default. `claude stop` sets the state `stopped`, which bigm treats as a failure and restarts with `claude respawn`, not with `session_resume`. Only `SMOKE_IDLE_MINUTES=65` tests the real idle stop and requires `session_resume`.
- The scratch ledger of the chain is a copy of the ledger template of the plugin, not a ledger that `/bruh:init` made. Only the `init-index` step uses the non-interactive init, with a scratch settings file, so that it does not change your user settings.
- The `deliver` step reads `claude logs`, which is not a stable interface. The text `bruh:deliver` can be in the log before the workflow runs. The later steps prove the run.
- The remote step cannot prove that the session that answers is a bruh clanker. The rot13 word proves only that a session on the remote machine got the reply through Orca.
