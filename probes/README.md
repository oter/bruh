# Probes

`sh probes/run.sh` runs P1 to P3 and prints PASS or FAIL. Set `PROBE_WORKDIR` to use that folder instead of a fresh `mktemp -d`.

Background probes must run in a folder inside a folder that the user trusted in an interactive session, because `claude --bg` refuses untrusted folders.

- P4 status line in a background session: in a scratch folder, set a status line command that appends `date -u` to `/tmp/bruh-probes/statusline.log`, start a `claude --bg` session with a 3-minute task, and do not attach. PASS if the log grows while no terminal is attached.
- P5 hook in a subagent of a background session: start `claude --bg --plugin-dir probes/probe-plugin` with the prompt "Use a subagent to run: echo sub". PASS if `/tmp/bruh-probes/hooks.tsv` has a `PreToolUse` line with a non-empty `agent_id`.
- P6 plugin workflow from a background session: add a one-agent workflow `probes/probe-plugin/workflows/hello.js` with `meta.name` `hello`, add the allow rule `Workflow(probe:hello)` to the settings file, and start `claude --bg` with the prompt `/probe:hello`. PASS if `claude logs <id>` shows the workflow result without an approval prompt.
- P7 CronCreate while idle: in an interactive session, ask Claude to create a recurring CronCreate task every minute that appends `date -u` to `/tmp/bruh-probes/cron.log`. Do not type for 5 minutes. PASS if the log has 4 or more lines.
- P8 long MCP call: after Task 5, call `answer_wait` with a deadline of 600 seconds from a session. PASS if the call returns `pending` after 600 seconds and not an idle-timeout error.
