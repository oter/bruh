#!/bin/sh
# Hook for PermissionDenied, PostToolUseFailure (Bash, Monitor), PreToolUse (all tools), and Stop:
# a refusal stops the session (spec 15.1). PermissionDenied writes a hold record, except a
# no-verdict denial (spec 15.1.1). So does a
# PostToolUseFailure whose error has the two fixed fragments of the documented template of the
# Claude Code worktree guard (errors.md, "Command blocked by the worktree isolation checks") for a
# command with the word git; a guard refusal of a command with no git is a report event only
# (spec 15.1.6) and writes no hold. For a role other than bigm, PreToolUse
# denies every call of a session with a hold, except the escalation tools. For bigm, it denies
# only the exact refused call, so bigm keeps working. Stop blocks the end of the turn while a
# hold of the session has no P0. It reads only the event name and fields of the hook input,
# never the meaning of a text. bigm's answer_write of the linked P0 removes the hold. A scout
# clerk (clerk-<project>-scout<n>) cannot call question_open: its deny reason tells it to send
# DONE: scout <subject> refused to its clanker with mail_post, and Stop does not block it.
# question_open with the hold posts the P0 to the parent and bigm itself, so the deny reason
# names no nudge (task 41, issue #42).
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
holds="$CLAUDE_PLUGIN_DATA/holds"
input=$(cat)
field() { printf '%s' "$input" | jq -r "$1 // \"\"" 2> /dev/null; }
sid=$(field .session_id)
[ -n "$sid" ] || exit 0
# The same test as isScout in roles.go: the task after the last - is scout<n>.
scout=
printf '%s' "$BRUH_ROLE_KEY" | grep -Eq '^clerk-.+-scout[0-9]*$' && scout=1

# write_hold <denial_source> <denial_reason>: writes the hold record and prints its ID.
write_hold() {
	id="H-$(date -u +%Y%m%dT%H%M%SZ)-$$"
	mkdir -p "$holds" &&
		printf '%s' "$input" | jq -c --arg id "$id" --arg key "$BRUH_ROLE_KEY" --arg src "$1" --arg why "$2" \
			--arg at "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)" \
			'{id: $id, session_id, role_key: $key, tool_name, tool_input, denial_source: $src, denial_reason: $why, at: $at, question_id: ""}' \
			> "$holds/$id.json.tmp" && mv "$holds/$id.json.tmp" "$holds/$id.json" && printf '%s' "$id"
}

# first_hold <jq filter>: the ID of the first hold of this session that matches the filter. The
# hook input $in goes through stdin, never argv: a large tool_input (a Write of some MiB) in argv
# makes the exec of jq fail with E2BIG, and the hook would then allow the call.
first_hold() {
	for f in "$holds"/H-*.json; do
		[ -f "$f" ] && printf '%s' "$input" | jq -r --arg s "$sid" --slurpfile h "$f" \
			". as \$in | \$h[0] | select(.session_id == \$s and ($1)) | .id" 2> /dev/null
	done | head -n 1
}

deny() {
	jq -nc --arg r "$1" '{hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $r}}'
}

case $(field .hook_event_name) in
PermissionDenied)
	# A no-verdict denial is a failed check, not a refusal: its two reason forms are fixed vendor text
	# (hooks.md, "PermissionDenied input"). No hold, so the allowed single retry goes through (spec 15.1.1).
	printf '%s' "$input" | jq -e '.reason == "Classifier unavailable"
		or (.reason | type == "string" and startswith("Auto mode could not evaluate this action"))' > /dev/null 2>&1 ||
		write_hold permission_denied "$(field .reason)" > /dev/null
	;;
PostToolUseFailure)
	# The guard refusal reaches no permission event: the Bash tool throws it before the spawn.
	printf '%s' "$input" | jq -e '(.tool_name == "Bash" or .tool_name == "Monitor")
		and (.error | type == "string" and contains("is isolated in the worktree ") and contains("git operations must target its own worktree"))
		and (.tool_input.command // "" | test("\\bgit\\b"))' > /dev/null 2>&1 &&
		write_hold worktree_guard "$(field .error)" > /dev/null
	;;
PreToolUse)
	# A refusal never freezes bigm: deny only the exact refused call (same tool_name and
	# tool_input), until the answer_write of its P0 removes the hold.
	if [ "$BRUH_ROLE_KEY" = bigm ]; then
		# shellcheck disable=SC2016 # $in is a jq variable
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
	if [ -n "$scout" ]; then
		deny "bruh refusal stop: hold $hold holds this scout after a refusal. A scout cannot open a question: send DONE: scout <subject> refused with mail_post to your clanker, with the refused command and the refusal in the body, then stop."
		exit 0
	fi
	deny "bruh refusal stop: hold $hold holds this session after a refusal. Open a P0 with question_open and the field hold = $hold: the tool delivers it to your parent and bigm, so send no nudge (only when its output has relay_error). Then wait for the answer: a subagent or a workflow agent waits with answer_wait; the main session waits for the ANSWER with mail_read. Do not run another form of the refused command."
	;;
Stop)
	[ "$(field .stop_hook_active)" = true ] || [ -n "$scout" ] && exit 0
	hold=$(first_hold '.question_id == ""')
	[ -n "$hold" ] || exit 0
	jq -nc --arg r "open the P0 for hold $hold with question_open first" '{decision: "block", reason: $r}'
	;;
esac
exit 0
