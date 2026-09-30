# Plan 5: Loops and integrations (code part)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Before you edit any Go file, run the `modern-go-guidelines:use-modern-go` skill `list` command and follow its guidelines.

**Goal:** Give bigm a watcher that reports code host events, give the merger clerk a merge train that confirms each merge through the code host API, and give bigm a Slack channel for P0 and P1 questions.

**Architecture:** The watcher and the merge train are subcommands of the Go program of plan 1 (`go run -C <plugin root>/mcp . watch` and `... merge-train`). They share one small interface, `codeHost`, with a GitHub and a Gitea implementation that use `net/http` only. The Slack channel is its own Go module in `plugins/bruh/channels/slack/`: a stdio MCP server that declares the `claude/channel` capability and polls the Slack Web API. The sweep (spec 9.1) and the Orca transport (spec 4.2) are role text of lane roles; this plan has no code for them. Telegram uses the official channel plugin; this plan only records its facts.

**Tech Stack:** Go 1.26 or later (standard library only), POSIX `sh`, GitHub REST API, Gitea REST API v1, Slack Web API.

**Spec:** [../../spec.md](../../spec.md) version 0.4, sections 8.3, 9.2, 12. Interfaces: [2026-09-30-v0.1-interfaces.md](2026-09-30-v0.1-interfaces.md) section 3.

## Global Constraints

- A status is true only when it was just read from its source (spec principle 1). A merge counts as done only when a `GET` of the pull request after the merge call says `merged: true`. The HTTP status of the merge call and the exit code are never the evidence.
- Each event line carries its source read: the API call, the value, and the UTC time.
- Tokens come from environment variables. For GitHub, `gh auth token` is the fallback. A token is never written to a file, a log line, or an event.
- Agent posts carry the structural marker `<!-- bruh:agent <role key> -->`. The watcher separates agent posts from human posts only by this marker, never by the words of a post.
- Tests use `httptest` servers. No test reaches the network.
- The Slack channel sends and polls only when `BRUH_ROLE_KEY` is `bigm` and the token, the channel, and the allowlist are set. In every other session it is an idle MCP server.

## Review Focus

1. **The merge call returns success, but the pull request is not merged** (a code host that answers 200 and does nothing, the failure of the setup of the owner). Expected: the train reports `not_confirmed` and stops. Test in Task 3 (`TestMergeTrainConfirmsByRead`).
2. **Checks are pending or missing.** Expected: the train waits for pending checks up to its limit and never merges a pull request with no checks or a red check. Test in Task 3 (`TestMergeTrainChecks`).
3. **A human reply looks like an agent post, or an agent post has no marker.** Expected: only the marker decides; a body without the marker is `human`. Test in Task 2 (`TestWatchSeparatesAgentPosts`).
4. **The first watcher run after a restart.** Expected: it records a baseline and emits no old events; events that happen between two polls are emitted once. Test in Task 2 (`TestWatchBaselineThenEvents`).
5. **A Slack message from a person who is not on the allowlist, or from the bot itself.** Expected: dropped, never sent to the session, and never read as a permission verdict. Test in Task 4 (`TestSlackGatesSender`).

---

### Task 1: Code host interface, configuration, and clients

**Files:**

- Create: `plugins/bruh/mcp/codehost.go`, `plugins/bruh/mcp/codehost_test.go`

**Interfaces:**

- Configuration file `<data>/repos.json`:

  ```json
  {
    "interval_seconds": 60,
    "repos": [
      {"repo": "owner/name", "host": "github", "project": "my-app"},
      {"repo": "owner/other", "host": "gitea", "api_url": "https://git.example.com/api/v1",
       "token_env": "GITEA_TOKEN", "project": "other", "merge_method": "squash"}
    ]
  }
  ```

  `host` is `github` or `gitea`. `api_url` defaults to `https://api.github.com` for GitHub and is required for Gitea. `token_env` defaults to `GITHUB_TOKEN` or `GITEA_TOKEN`. `merge_method` defaults to `merge`. `interval_seconds` defaults to 60, minimum 10. `project` defaults to the name part of `repo`.
- `codeHost` interface:

  ```go
  type codeHost interface {
      Branches(ctx context.Context) (map[string]string, error)        // branch name -> head SHA
      Comments(ctx context.Context, since string) ([]hostComment, error) // issue and pull request comments updated after since
      MergedPulls(ctx context.Context) ([]hostPull, error)             // recently closed pull requests that are merged
      Pull(ctx context.Context, n int) (hostPull, error)
      Checks(ctx context.Context, sha string) (string, error)          // "success", "pending", "failure", or "none"
      Merge(ctx context.Context, n int, sha, method string) error
  }
  ```

  Each method records the call it made (`method URL`) in `lastCall`, so an event can carry its source.
- GitHub: `GET /repos/{o}/{r}/branches`, `GET /repos/{o}/{r}/issues/comments?since=` and `GET /repos/{o}/{r}/pulls/comments?since=`, `GET /repos/{o}/{r}/pulls?state=closed&sort=updated&direction=desc`, `GET /repos/{o}/{r}/pulls/{n}`, `GET /repos/{o}/{r}/commits/{sha}/status` plus `GET /repos/{o}/{r}/commits/{sha}/check-runs`, `PUT /repos/{o}/{r}/pulls/{n}/merge` with `{"sha","merge_method"}`. Header `Authorization: Bearer <token>`.
- Gitea: `GET /repos/{o}/{r}/branches`, `GET /repos/{o}/{r}/issues/comments?since=`, `GET /repos/{o}/{r}/pulls?state=closed&sort=recentupdate`, `GET /repos/{o}/{r}/pulls/{n}`, `GET /repos/{o}/{r}/commits/{sha}/status`, `POST /repos/{o}/{r}/pulls/{n}/merge` with `{"Do","head_commit_id"}`. Header `Authorization: token <token>`.
- `Checks` on GitHub: `failure` when any check run is completed with a conclusion other than `success`, `neutral`, or `skipped`, or the combined status is `failure`; `pending` when any check run is not completed, or statuses exist and the combined state is `pending`; `none` when there are no check runs and no statuses; else `success`. (The combined status is `pending` when no status exists, so it counts only when `total_count` is above 0.) On Gitea: `none` for `total_count` 0, `success`, `pending`, or `failure` for `failure` and `error`.

- [ ] **Step 1: Write the failing tests** with an `httptest` server for each host: the paths, the headers, the request bodies of `Merge`, and each `Checks` case.
- [ ] **Step 2: Implement.** One `getJSON(ctx, url, v)` and one `sendJSON(ctx, method, url, body)` for each client, with a 30-second timeout and a response body limit of 8 MiB.
- [ ] **Step 3: Run** `go test -race ./...`. Expected: PASS.
- [ ] **Step 4: Commit** "Add the code host clients for GitHub and Gitea".

### Task 2: The watcher

**Files:**

- Create: `plugins/bruh/mcp/watch.go`, `plugins/bruh/mcp/watch_test.go`, `plugins/bruh/scripts/watcher.sh`
- Modify: `plugins/bruh/mcp/main.go`, `plugins/bruh/mcp/report.go` (`ReportLine.Event`, and `report_read` accepts the key `watcher`)

**Interfaces:**

- `watch [--data <dir>] [--once]`: reads `<data>/repos.json` and the state `<data>/watch/state.json`, polls each repository, prints one JSON line for each event, and appends the same line to `<data>/reports/watcher.jsonl`. Then it sleeps `interval_seconds` and polls again, until killed. `--once` polls once. The data folder defaults as in plan 2 Task 7, because the Bash tool and a `Monitor` get no `CLAUDE_PLUGIN_DATA`; bigm gets it from `bruh_info`.
- An event line is a report line of plan 1 with an `event` object: `{"at","from":"watcher","kind":"event","text":"<one line>","source":{"call","value","at"},"event":{"type","repo","project","ref","sha","number","url","author","by","role_key"}}`. `type` is `push`, `comment`, `red`, or `merge`. `by` is `agent` or `human` (comments only). `role_key` is the key from the marker.
- Events: `push` for a branch whose head SHA changed or that is new; `red` once for each branch head SHA whose `Checks` value is `failure` (checked only when the SHA is new or its last value was `pending`, to keep the API calls bounded); `comment` for each comment updated after the last poll; `merge` for each merged pull request not seen before.
- The first poll of a repository records the baseline and emits nothing.
- The agent marker: the regular expression `<!-- bruh:agent ([a-z0-9-]+) -->` on the comment body, and the key must pass `ParseRoleKey`.
- `scripts/watcher.sh`: `exec go run -C "<folder of the script>/../mcp" . watch "$@"` with `GOTOOLCHAIN=local`.

- [ ] **Step 1: Write the failing tests** with a fake GitHub server whose state the test changes between two `pollOnce` calls: `TestWatchBaselineThenEvents` (push, red, merge, comment, each once), `TestWatchSeparatesAgentPosts`, `TestWatchAppendsReport` (the line decodes as a `ReportLine` through `report_read` with key `watcher`), and `TestWatchGitea`.
- [ ] **Step 2: Implement** `pollOnce(ctx, env, cfg, state, out) error` and the loop.
- [ ] **Step 3: Run** `go test -race ./...` and shellcheck. Expected: PASS.
- [ ] **Step 4: Commit** "Add the watcher".

### Task 3: The merge train

**Files:**

- Create: `plugins/bruh/mcp/mergetrain.go`, `plugins/bruh/mcp/mergetrain_test.go`, `plugins/bruh/scripts/merge-train.sh`
- Modify: `plugins/bruh/mcp/main.go`

**Interfaces:**

- `merge-train [--data <dir>] [--wait-minutes <n>] <owner/repo> <number>...`: the queue is the list of pull request numbers, in order. For each number, one at a time:
  1. `Pull`: skip with `not_open` when it is not open, `draft` when it is a draft, `conflict` when `mergeable` is `false`.
  2. `Checks` of the head SHA: wait while `pending` (poll every 20 seconds, up to `--wait-minutes`, default 30); skip with `checks_failure`, `checks_none`, or `checks_timeout`.
  3. `Merge` with the head SHA, so the host refuses a head that moved.
  4. `Pull` again: `merged: true` gives `merged` with `merge_commit_sha`; anything else gives `not_confirmed`, and the train stops, because the state of the repository is not known.
- One JSON line for each pull request: `{"at","repo","number","result","reason","merge_commit_sha","source":{"call","value","at"}}`. The exit code is 0 when every pull request merged, 1 otherwise. The merger clerk reads the lines, not the exit code.
- `scripts/merge-train.sh`: `exec go run -C "<folder of the script>/../mcp" . merge-train "$@"`.

- [ ] **Step 1: Write the failing tests** with fake servers: `TestMergeTrainMergesInOrder`, `TestMergeTrainChecks` (pending then green, red, none, timeout with a short wait), `TestMergeTrainConfirmsByRead` (the fake answers 200 to the merge call and keeps `merged: false`), `TestMergeTrainGitea`.
- [ ] **Step 2: Implement.** The waits use `Env.PollInterval`, so tests run fast.
- [ ] **Step 3: Run** `go test -race ./...` and shellcheck. Expected: PASS.
- [ ] **Step 4: Commit** "Add the merge train".

### Task 4: The Slack channel

**Files:**

- Create: `plugins/bruh/channels/slack/go.mod`, `main.go`, `server.go`, `slack.go`, `slack_test.go`, `README.md`
- Modify: `plugins/bruh/.mcp.json` (server `slack`), `plugins/bruh/.claude-plugin/plugin.json` (`channels`), `plugins/bruh/mcp/skeleton_test.go`, `.github/workflows/ci.yml` (the module in the `go` job matrix)

**Interfaces:**

- `.mcp.json` server `slack`: `go run -C ${CLAUDE_PLUGIN_ROOT}/channels/slack .` with `GOTOOLCHAIN=local`, `BRUH_DATA=${CLAUDE_PLUGIN_DATA}`, `SLACK_BOT_TOKEN=${user_config.slack_bot_token}`, `SLACK_CHANNEL_ID=${user_config.slack_channel_id}`, `SLACK_ALLOWED_USERS=${user_config.slack_owner_user_id}`.
- `plugin.json` `channels`: `[{"server":"slack","displayName":"Slack","userConfig":{...}}]` with `slack_bot_token` (sensitive), `slack_channel_id`, and `slack_owner_user_id`.
- Capabilities: `experimental["claude/channel"]` = `{}`, `experimental["claude/channel/permission"]` = `{}` (only when active, because the allowlist gates the verdicts), `tools` = `{}`.
- Tools: `post_question(question_id, header, body)` posts one top-level message (`<header>`, the body, and the line `Answer in this thread.`) with `chat.postMessage` and returns `{"thread_ts"}`; `reply(thread_ts, text)` posts in a thread.
- Threads: `<data>/channels/slack/threads.json` maps each thread `ts` to the question ID (or the permission request ID), with the time of the last reply read. A thread is dropped after 7 days.
- Poll loop (every `SLACK_POLL_SECONDS`, default 20): `conversations.history` with `oldest` for new top-level messages and `conversations.replies` for each open thread. A message from a user not in `SLACK_ALLOWED_USERS`, or with `bot_id`, is dropped. A message that matches `^\s*(y|yes|n|no)\s+([a-km-z]{5})\s*$` (case-insensitive) becomes `notifications/claude/channel/permission` `{"request_id","behavior"}`. Every other message becomes `notifications/claude/channel` with `content` = the text and `meta` = `{"question_id","thread_ts","ts","user_id"}` (`question_id` only in a question thread).
- Permission relay: on `notifications/claude/channel/permission_request` it posts `bigm asks to run <tool_name>: <description>`, the `input_preview`, and `Reply "yes <id>" or "no <id>".`, and keeps the message as a thread.
- `SLACK_API_URL` (default `https://slack.com/api/`) points the tests at a fake server.

- [ ] **Step 1: Write the failing tests** with a fake Slack server and the server on pipes: `TestSlackInitialize` (capabilities), `TestSlackPostQuestion`, `TestSlackThreadReplyReachesSession`, `TestSlackGatesSender`, `TestSlackPermissionRelay` (request, post, `yes abcde` from the owner, verdict notification), `TestSlackIdleWithoutConfig` (no HTTP call, no tools that post).
- [ ] **Step 2: Implement.**
- [ ] **Step 3: Run** gofmt, `go vet`, and `go test -race ./...` in the module; `claude plugin validate ./plugins/bruh`. Expected: PASS.
- [ ] **Step 4: Commit** "Add the Slack channel".

### Task 5: Checks

- [ ] Run every check of the lane (both Go modules, shellcheck, markdownlint, both `claude plugin validate` commands). Fix what fails.

---

## Deviations from the specification

1. Spec 9.2 says the watcher writes each event to the report file of the project. The interfaces file (section 3) says `reports/watcher.jsonl`. This plan follows the interfaces file; each event carries `project`, so bigm can sort the events by project.
2. The merge train takes its queue as pull request numbers on the command line, not as a label or a file on the code host. The merger clerk knows which pull requests its grant covers. A label queue would let anyone who can set a label add a pull request to the train.
3. Slack uses Web API polling, not Socket Mode. Socket Mode needs a WebSocket client, which the Go standard library does not have, and the specification allows only the standard library. Polling needs no public URL. The cost is a delay of up to one poll interval (default 20 seconds).
4. The merge train does not update a pull request branch that is behind its base. That is a push, and clerks do all pushes (spec 3.6). The train skips it with the reason from the code host.

## Agent-derived decisions (need owner decision)

1. The agent marker is the HTML comment `<!-- bruh:agent <role key> -->`. GitHub and Gitea do not render HTML comments, so the marker is not visible to readers. Options, ranked: 1. the HTML comment, as built; 2. a visible footer line `-- posted by bruh <role key>`; 3. a separate bot account for agent posts (identities are out of scope for version 0.1, spec 21).
2. The code host configuration is the file `<data>/repos.json`, written by bigm through a tool of lane roles or by hand. The interfaces file does not name it. Options: 1. a data folder file, as built; 2. the ledger `projects/<project>.md` files.
3. The Slack channel is one of the MCP servers of the bruh plugin, so it starts (idle) in every session that loads the plugin. It posts and polls only in bigm. Options: 1. one plugin, as built; 2. a second plugin `bruh-slack` in the same marketplace.
4. The Slack app must be an internal app of the workspace of the owner. Slack limits `conversations.history` and `conversations.replies` to 1 request each minute for new apps that are distributed outside the Slack Marketplace, and gives internal apps Tier 3 (50 or more each minute).

## Telegram (no code; facts for the controller and lane roles)

| Fact | How verified | Source |
|---|---|---|
| The plugin is `telegram@claude-plugins-official`. It needs Bun. Install: `/plugin install telegram@claude-plugins-official` at user scope. | Read the page | <https://code.claude.com/docs/en/channels.md>, "Supported channels" |
| The bot token comes from BotFather. `/telegram:configure <token>` saves it in `~/.claude/channels/telegram/.env`; `TELEGRAM_BOT_TOKEN` in the shell also works. | Read the page | channels.md |
| Start: `claude --channels plugin:telegram@claude-plugins-official`. The bot answers only while the channel runs. | Read the page | channels.md |
| Sender allowlist: the owner sends any message to the bot, gets a pairing code, runs `/telegram:access pair <code>`, then `/telegram:access policy allowlist`. Other senders are dropped. | Read the page and the plugin README | channels.md, "Security"; <https://github.com/anthropics/claude-plugins-official/tree/main/external_plugins/telegram> |
| The plugin declares `claude/channel/permission`, so it relays the permission prompts of the session it runs in (bigm only). The owner answers `yes <id>` or `no <id>`. | Read the plugin source (`server.ts`) | same repository |
| The `reply` tool takes `chat_id`, `text`, and an optional `reply_to` message ID for native threading. It returns the sent message IDs. | Read the plugin README | same repository |
| An inbound message reaches the session with the meta `chat_id`, `message_id`, `user`, `user_id`, and `ts`. It carries no ID of the message that the owner replied to. So bigm cannot tell by structure which question a Telegram reply answers. Lane roles must let the owner start the answer with the question ID (`Q-<n>`), or send one question at a time. | Read the plugin source (`handleInbound` in `server.ts`) | same repository |
| The Bot API has no message history or search. | Read the plugin README | same repository |
| During the research preview, only the plugins of the Anthropic allowlist register with `--channels`. A custom channel (the Slack channel of bruh) needs `--dangerously-load-development-channels plugin:bruh@bruh`, and `--channels` does not extend that bypass. | Read the page | channels.md, "Research preview"; <https://code.claude.com/docs/en/channels-reference.md> |

## Verified facts

| Fact | How verified | Source |
|---|---|---|
| A channel is a stdio MCP server that declares `capabilities.experimental["claude/channel"]` = `{}` and sends `notifications/claude/channel` with `content` (string) and `meta` (string map; keys of letters, digits, and `_` only; other keys are dropped). | Read the page | channels-reference.md, "Server options", "Notification format" |
| Permission relay: the server declares `experimental["claude/channel/permission"]` = `{}`; Claude Code sends `notifications/claude/channel/permission_request` with `request_id` (five letters `a` to `z` without `l`), `tool_name`, `description`, and `input_preview`; the server answers `notifications/claude/channel/permission` with `request_id` and `behavior` (`allow` or `deny`). Relay goes only to servers opted in with `--channels` or the development flag. | Read the page | channels-reference.md, "Relay permission prompts" |
| A plugin `channels` entry is a strict object with `server` (required), `displayName`, and `userConfig`; its values substitute into `${user_config.KEY}` in the `env` of the server. | Read the page | <https://code.claude.com/docs/en/plugins-reference.md>, "Channels" |
| Slack `conversations.replies` and `conversations.history`: Tier 3 for internal apps; 1 request each minute and at most 15 objects for new commercially distributed apps outside the Marketplace (since 2025-05-29). | Read the page | <https://docs.slack.dev/reference/methods/conversations.replies/> |
| GitHub combined status is `pending` when no status exists; check runs are a separate API with `status` and `conclusion` (`success`, `failure`, `neutral`, `cancelled`, `skipped`, `timed_out`, `action_required`). | Read the pages | <https://docs.github.com/en/rest/commits/statuses>, <https://docs.github.com/en/rest/checks/runs> |
| GitHub `PUT /repos/{owner}/{repo}/pulls/{n}/merge` takes `sha` and `merge_method` (`merge`, `squash`, `rebase`); 405 when the merge cannot run, 409 when the head does not match `sha`. | Read the page | <https://docs.github.com/en/rest/pulls/pulls> |
| Gitea `POST /repos/{owner}/{repo}/pulls/{index}/merge` takes `Do` (`merge`, `rebase`, `rebase-merge`, `squash`, `fast-forward-only`, `manually-merged`) and `head_commit_id`; a pull request has `merged`, `merged_at`, `merge_commit_sha`, `mergeable`, `draft`, and `head.sha`; `GET /repos/{owner}/{repo}/commits/{ref}/status` returns `state` and `total_count`; `GET /repos/{owner}/{repo}/issues/comments` takes `since`. | Read the API description | <https://gitea.com/swagger.v1.json> (Gitea 1.27 development version) |

## Self-review

- **Spec coverage:** 8.3 merge confirmed by the API and a host-independent script (Task 3), 9.2 watcher and agent marker (Task 2), 12 Slack thread, allowlist, and permission relay (Task 4), Telegram facts (this file).
- **Verify items:** 12 permission relay and sender allowlist: documented for both channels; tested for Slack in Task 4.
