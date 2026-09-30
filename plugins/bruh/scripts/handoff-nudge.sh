#!/bin/sh
# PostToolUse hook. When the context use of a bruh role session reaches the
# handoff threshold, tell the agent once to update its handoff. The marker file
# <plugin data>/context/<session_id>.nudged is removed when the use falls below
# the threshold again (after a compaction), which arms the next crossing.
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
sid=$(jq -r 'if has("agent_id") then empty else .session_id // empty end' 2> /dev/null)
case "$sid" in '' | *[!A-Za-z0-9_-]*) exit 0 ;; esac
ctx="$CLAUDE_PLUGIN_DATA/context/$sid.json"
marker="$CLAUDE_PLUGIN_DATA/context/$sid.nudged"
[ -f "$ctx" ] || exit 0
out=$(jq -c --arg limit "${CLAUDE_PLUGIN_OPTION_HANDOFF_PERCENT:-50}" --arg key "$BRUH_ROLE_KEY" '
	(($limit | tonumber?) // 50) as $t
	| if .used_percentage == null then empty
	elif .used_percentage < $t then "under"
	else {hookSpecificOutput: {hookEventName: "PostToolUse", additionalContext:
		"bruh: the context use of this session is \(.used_percentage) percent, at or above the handoff threshold of \($t) percent. The role instructions of \($key) require a handoff_write call with the full current state now, before the next step."}}
	end' "$ctx" 2> /dev/null) || exit 0
case "$out" in
	'') ;;
	'"under"') rm -f "$marker" ;;
	*)
		if [ ! -e "$marker" ] && : > "$marker"; then
			printf '%s\n' "$out"
		fi
		;;
esac
exit 0
