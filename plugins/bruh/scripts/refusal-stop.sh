#!/bin/sh
# Hook for PermissionDenied, PreToolUse (all tools), and Stop: a refusal stops the session
# (spec 15.1). PermissionDenied writes a hold record. PreToolUse denies every call of a session
# with a hold, except the escalation tools, and denies a compound Bash command with the word
# git in a linked worktree (the git-shape guard), which writes a hold too. Stop blocks the end
# of the turn while a hold of the session has no P0. It reads only the event name and fields of
# the hook input, never the meaning of a text. answer_write of the linked P0 removes the hold.
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
		[ -f "$f" ] && jq -r --arg s "$sid" "select(.session_id == \$s and ($1)) | .id" "$f" 2> /dev/null
	done | head -n 1
}

deny() {
	jq -nc --arg r "$1" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $r}}'
}

case $(field .hook_event_name) in
PermissionDenied)
	write_hold "$(field .denial_source)" "$(field .denial_reason)" > /dev/null
	;;
PreToolUse)
	tool=$(field .tool_name)
	hold=$(first_hold true)
	if [ -n "$hold" ]; then
		case $tool in
		mcp__plugin_bruh_bruh__question_open | mcp__plugin_bruh_bruh__answer_wait | mcp__plugin_bruh_bruh__mail_post | \
			mcp__plugin_bruh_bruh__mail_read | SendMessage | ToolSearch | StructuredOutput) exit 0 ;;
		mcp__plugin_bruh_bruh__answer_write) [ "$BRUH_ROLE_KEY" = bigm ] && exit 0 ;;
		esac
		deny "bruh refusal stop: hold $hold holds this session after a refusal. Open a P0 with question_open and the field hold = $hold, then wait for the answer with answer_wait. Do not run another form of the refused command."
		exit 0
	fi
	[ "$tool" = Bash ] || exit 0
	# Closed token check: the word git (also inside quotes) plus a separator, a substitution,
	# or a heredoc.
	printf '%s' "$input" | jq -e '(.tool_input.command // "")
		| test("(^|[^A-Za-z0-9_-])git($|[^A-Za-z0-9_-])") and test("[;&|\n`]|\\$\\(|<<")' > /dev/null 2>&1 || exit 0
	cwd=$(field .cwd)
	dirs=$(git -C "${cwd:-.}" rev-parse --path-format=absolute --git-dir --git-common-dir 2> /dev/null) || exit 0
	[ "$(printf '%s\n' "$dirs" | sed -n 1p)" != "$(printf '%s\n' "$dirs" | sed -n 2p)" ] || exit 0
	rule="In a worktree, no compound commands with git. Data goes through tool inputs."
	hold=$(write_hold bruh-git-shape "bruh git-shape guard: $rule")
	deny "bruh git-shape guard: $rule This command has the word git and a separator, a substitution, or a heredoc. Hold $hold now holds this session: open a P0 with question_open and the field hold = $hold, then wait for the answer with answer_wait. Do not run another form."
	;;
Stop)
	[ "$(field .stop_hook_active)" = true ] && exit 0
	hold=$(first_hold '.question_id == ""')
	[ -n "$hold" ] || exit 0
	jq -nc --arg r "open the P0 for hold $hold with question_open first" '{decision: "block", reason: $r}'
	;;
esac
exit 0
