# Load test

The load test checks the local transport of bruh under load (spec section 4.1, Verify item): 8 background sessions exchange messages for one hour through the durable mailbox of the bruh MCP server and `SendMessage` nudges. The test records missed messages and resumes.

`run.sh` is the driver. It starts real Claude Code sessions, so it uses your Claude account and your usage limits. The default model is `haiku`, to keep the cost low.

## Setup

Use the trusted repository of the smoke test. See "One-time setup" in [../smoke/README.md](../smoke/README.md). It must have a commit and no remote.

## Run

Run the driver from the root of the bruh checkout:

```bash
BRUH_TRUSTED_REPO=<scratch folder>/bruh-smoke-home sh tests/load/run.sh
```

Flags:

| Flag | Default | Meaning |
|---|---|---|
| `--sessions <n>` | 8 | The count of background sessions, an even number, 2 or more |
| `--minutes <n>` | 60 | The duration of the test |
| `--interval <n>` | 2 | The minutes between two messages of one session, 1 to 59 |
| `--model <name>` | `haiku` | The model of the sessions |
| `--dry-run` | off | Print the commands. Start no session and write no file. |

For a short check before the full hour:

```bash
BRUH_TRUSTED_REPO=<scratch folder>/bruh-smoke-home sh tests/load/run.sh --sessions 2 --minutes 10
```

Environment variables: `LOAD_RUN` sets the run ID (1 to 12 lowercase letters and digits), `LOAD_EVIDENCE` sets the evidence folder (default `$TMPDIR/bruh-load-<run>`), and `BRUH_TEST_DATA` sets the plugin data folder (default `~/.claude/plugins/data/bruh-inline`).

## What the driver does

1. It builds the MCP server of the checkout and uses it as a stand-in caller. The sessions are pairs of a clanker and its clerk: session `2k-1` has the role key `clanker-load-<run>-p<k>`, and session `2k` has `clerk-load-<run>-p<k>-s`. `mail_post` allows only the edges of the role tree, so each session sends to its partner. The driver writes the role settings of each key as its parent (bigm for a clanker key, the clanker for a clerk key) and the start messages as bigm.
2. It adds a linked worktree of `BRUH_TRUSTED_REPO` on a new orphan branch, with a `.claude/settings.json` that allows the bruh MCP tools, `SendMessage`, `ListAgents`, and `CronCreate`, and denies `Bash`, `Edit`, and `Write`.
3. For each session, it writes the role settings with `role_settings_write` and a start message with `mail_post`, then starts the session with the launch command of spec 4.1, without `--agent`:

   ```bash
   CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --name <role key> --permission-mode auto \
     --settings <data>/roles/<role key>.json --plugin-dir plugins/bruh --model haiku \
     "Read your start message with mail_read."
   ```

4. The start message tells each session to create a recurring `CronCreate` task. Each tick posts one message to the partner session with `mail_post`, and sends the header and the mail ID as a `SendMessage` nudge to that session by name. The mail ID makes each nudge different, because a receiver drops identical repeats. When a nudge arrives, the session calls `mail_read`.
5. Each minute, the driver reads `claude agents --json --all`. For a session with no process and unread mail older than 2 minutes, it runs `claude --resume <sessionId> --bg "Read your mailbox with mail_read."` and records whether the session ID stayed the same.
6. At the end, it counts the messages of each mailbox, and stops and removes everything of the run, as the smoke test does.

## Read the result

The driver prints one line for each session and one result line:

```text
clanker-load-<run>-p1: messages 31, read 31, missed 0, windows of 5 minutes with no message 0, resumes 0, resumes with a new session ID 0
...
PASS load - no missed message, each session got a message in each window, no resume happened
```

- A message is missed when it is still unread at the end and was posted more than 5 minutes before the end.
- The run is divided into windows of 5 minutes, or of 2 intervals when the interval is more than 2.5 minutes. The first window is the warm-up. Each session must get at least one message in each later window.
- The result is `FAIL` when a message is missed, when a session has a window with no message (`gaps=<n>`), when a resume started a new session ID (`new-session-id`), or when the run is shorter than two windows (`too-short`).
- The result line says "no resume happened" when the driver did not resume a session.

The evidence folder keeps `results.txt`, `resumes.tsv`, and the log of each session.

## Limits

- The sessions run without a role agent. The test measures the transport, not the role instructions.
- Auto mode is not available for Haiku 4.5 (`docs/knowledge.md`), so with the default model the sessions fall back to manual mode. The allow rules of the work folder cover each tool that they use, so they get no prompt, but the test does not run under the permission mode of the production roles. Pass `--model` with a model that has auto mode to test that mode too.
- The driver, not the sending session, checks the process and resumes a session. In bruh, the sender does this check before each nudge.
