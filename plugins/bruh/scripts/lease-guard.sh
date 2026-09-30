#!/bin/sh
# PreToolUse hook for Bash. Denies a command that starts with a command prefix
# (patterns) of a shared resource when the role key holds no live lease on it.
# It matches by structure only: quoted strings are removed, the rest is split at
# ; & | and newlines, and a segment matches when it equals a prefix or continues
# with white space after it. A deny rule like this is a speed bump.
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
state="$CLAUDE_PLUGIN_DATA/leases/state.json"
[ -f "$state" ] || exit 0
jq -c --slurpfile st "$state" --arg me "$BRUH_ROLE_KEY" --arg now "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)" '
	([39] | implode) as $sq
	| [(.tool_input.command // "")
		| gsub("\"([^\"\\\\]|\\\\.)*\""; "Q") | gsub($sq + "[^" + $sq + "]*" + $sq; "Q")
		| splits("[;&|\n]") | sub("^\\s+"; "")] as $segs
	| [($st[0].resources // {}) | to_entries[]
		| select(any(.value.patterns[]?; . as $p | any($segs[]; . == $p or startswith($p + " ") or startswith($p + "\t"))))
		| select(any(.value.grants[]?; .holder == $me and .until > $now) | not)
		| .key] as $missing
	| select($missing | length > 0)
	| {hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason:
		(if $me == "bigm"
		then "bruh lease guard: bigm does not run commands on the shared resources \($missing | join(", ")). A clerk with a lease runs them."
		else "bruh lease guard: \($me) holds no lease on \($missing | join(", ")). Ask your grantor with lease_request and wait for lease_grant." end)}}' 2> /dev/null
exit 0
