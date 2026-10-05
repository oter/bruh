#!/bin/sh
# Hook for PermissionDenied, PreToolUse (all tools), and Stop: a refusal stops the session
# (spec 15.1). PermissionDenied writes a hold record. For a role other than bigm, PreToolUse
# denies every call of a session with a hold, except the escalation tools. For bigm, it denies
# only the exact refused call, so bigm keeps working. Stop blocks the end of the turn while a
# hold of the session has no P0. It reads only the event name and fields of the hook input,
# never the meaning of a text. bigm's answer_write of the linked P0 removes the hold.
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
holds="$CLAUDE_PLUGIN_DATA/holds"
input=$(cat)
field() { printf '%s' "$input" | jq -r "$1 // \"\"" 2> /dev/null; }
sid=$(field .session_id)
[ -n "$sid" ] || exit 0

# write_hold <denial_source> <denial_reason>: writes the hold record and prints its ID.
write_hold() {
	id="H-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	mkdir -p "$holds" &&
		printf '%s' "$input" | jq -c --arg id "$id" --arg key "$BRUH_ROLE_KEY" --arg src "$1" --arg why "$2" \
			--arg at "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)" \
			'{id: $id, session_id, role_key: $key, tool_name, tool_input, denial_source: $src, denial_reason: $why, at: $at, question_id: ""}' \
			> "$holds/$id.json.tmp" && mv "$holds/$id.json.tmp" "$holds/$id.json" && printf '%s' "$id"
}

# first_hold <jq filter>: the ID of the first hold of this session that matches the filter.
first_hold() {
	for f in "$holds"/H-*.json; do
		[ -f "$f" ] && jq -r --arg s "$sid" --argjson in "$input" "select(.session_id == \$s and ($1)) | .id" "$f" 2> /dev/null
	done | head -n 1
}

deny() {
	jq -nc --arg r "$1" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $r}}'
}

case $(field .hook_event_name) in
PermissionDenied)
	write_hold permission_denied "$(field .reason)" > /dev/null
	;;
PreToolUse)
	# A refusal never freezes bigm: deny only the exact refused call (same tool_name and
	# tool_input), until the answer_write of its P0 removes the hold.
	if [ "$BRUH_ROLE_KEY" = bigm ]; then
		hold=$(first_hold '.tool_name == $in.tool_name and .tool_input == $in.tool_input')
		[ -n "$hold" ] && deny "bruh refusal stop: hold $hold: the owner has not answered the refusal of this exact call. Do not run it again in any form. If its P0 is not open, open it with question_open and the field hold = $hold. Go on with your other work."
		exit 0
	fi
	hold=$(first_hold true)
	[ -n "$hold" ] || exit 0
	case $(field .tool_name) in
	mcp__plugin_bruh_bruh__question_open | mcp__plugin_bruh_bruh__answer_wait | mcp__plugin_bruh_bruh__mail_post | \
		mcp__plugin_bruh_bruh__mail_read | SendMessage | ToolSearch | StructuredOutput) exit 0 ;;
	esac
	deny "bruh refusal stop: hold $hold holds this session after a refusal. Open a P0 with question_open and the field hold = $hold, then wait for the answer: a subagent or a workflow agent waits with answer_wait; the main session waits for the ANSWER with mail_read. Do not run another form of the refused command."
	;;
Stop)
	[ "$(field .stop_hook_active)" = true ] && exit 0
	hold=$(first_hold '.question_id == ""')
	[ -n "$hold" ] || exit 0
	jq -nc --arg r "open the P0 for hold $hold with question_open first" '{decision: "block", reason: $r}'
	;;
esac
exit 0
