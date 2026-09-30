# Slack channel of bruh

This folder is a Claude Code channel: a stdio MCP server that connects the bigm session to one Slack channel. bigm posts each P0 and P1 question as one Slack message. The owner answers in the thread of that message. The answer comes into the bigm session with the question ID.

The server is part of the bruh plugin. Claude Code starts it in every session that loads the plugin, with `go run`. It sends and reads Slack messages only when all of these are true. In every other session it does nothing.

- `BRUH_ROLE_KEY` is `bigm`.
- The options `slack_bot_token`, `slack_channel_id`, and `slack_owner_user_id` of the plugin are set.

## Make the Slack app

1. Create a Slack app for your own workspace at <https://api.slack.com/apps>. Do not distribute it. Slack gives an internal app the Tier 3 rate limit (50 or more requests each minute) for `conversations.history` and `conversations.replies`. An app that is distributed outside the Slack Marketplace gets 1 request each minute.
2. Add these bot token scopes: `chat:write`, and `channels:history` for a public channel or `groups:history` for a private channel.
3. Install the app in the workspace. Copy the bot token (`xoxb-...`).
4. Create a channel for bruh and invite the app to it. Copy the channel ID.
5. Copy your own Slack member ID from your Slack profile.
6. In Claude Code, run `/plugin configure bruh` and set the three Slack options. The token is sensitive, so Claude Code keeps it in the secure credential store.

## Start bigm with the channel

During the channels research preview, a channel that is not on the Anthropic allowlist loads only with the development flag:

```bash
claude --agent bruh:bigm --name bigm --permission-mode auto \
  --settings <plugin data folder>/roles/bigm.json \
  --dangerously-load-development-channels plugin:bruh@bruh
```

## How it works

- The server polls the Slack Web API: `conversations.history` for new top-level messages, and `conversations.replies` for each open question thread. The poll interval is 20 seconds (`SLACK_POLL_SECONDS`, minimum 5). Socket Mode needs a WebSocket client, and bruh uses only the Go standard library, so the server polls instead.
- Only messages of the owner (`slack_owner_user_id`) reach the session. Messages of other people and of bots are dropped.
- The server polls a thread until the first message of the owner in it, or until the verdict of its permission prompt. A `reply` of bigm in a thread opens it again. A top-level message of the owner opens its thread too. The server polls the newest threads first. A question thread or a permission thread is dropped after 7 days, and another thread after one day. The state is in `<plugin data folder>/channels/slack/state.json`.
- Replies that the owner also sends to the channel, and messages with a file, reach the session.
- Permission relay: when bigm waits on a permission prompt, the server posts the tool, its description, and its input preview. The owner answers `yes <id>` or `no <id>`. The terminal prompt stays open, and the first answer wins. The relay reaches only the prompts of bigm, not the prompts of other sessions.

## Tools

| Tool | Arguments | Result |
|---|---|---|
| `post_question` | `question_id`, `header`, `body` | `{"thread_ts"}` |
| `reply` | `thread_ts`, `text` | `{"ts"}` |
