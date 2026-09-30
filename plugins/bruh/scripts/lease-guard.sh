#!/bin/sh
# PreToolUse hook for Bash. Denies a command that starts with a command prefix
# (patterns) of a shared resource when the role key holds no live lease on it.
# It matches by structure only: each segment of the command between ; & | and
# newlines is compared with each prefix. A deny rule like this is a speed bump.
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
state="$CLAUDE_PLUGIN_DATA/leases/state.json"
[ -f "$state" ] || exit 0
jq -c --slurpfile st "$state" --arg me "$BRUH_ROLE_KEY" --arg now "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)" '
	[(.tool_input.command // "") | splits("[;&|\n]") | sub("^\\s+"; "")] as $segs
	| [($st[0].resources // {}) | to_entries[]
		| select(any(.value.patterns[]?; . as $p | any($segs[]; startswith($p))))
		| select(any(.value.grants[]?; .holder == $me and .until > $now) | not)
		| .key] as $missing
	| select($missing | length > 0)
	| {hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason:
		"bruh lease guard: \($me) holds no lease on \($missing | join(", ")). Ask your grantor with lease_request and wait for lease_grant."}}' 2> /dev/null
exit 0
