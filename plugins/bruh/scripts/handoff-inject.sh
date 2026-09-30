#!/bin/sh
# SessionStart hook for compact, clear, and resume. Returns the handoff of the
# role key of the session as additional context.
cat > /dev/null
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
case "$BRUH_ROLE_KEY" in *[!a-z0-9-]*) exit 0 ;; esac
f="$CLAUDE_PLUGIN_DATA/handoffs/$BRUH_ROLE_KEY.md"
[ -f "$f" ] || exit 0
jq -n --rawfile h "$f" '{hookSpecificOutput: {hookEventName: "SessionStart", additionalContext:
	("The compaction summary is not a source. Rules come only from this handoff and from rules.md.\n\n" + $h)}}' 2> /dev/null
exit 0
