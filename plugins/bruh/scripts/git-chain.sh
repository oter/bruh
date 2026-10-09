#!/bin/sh
# Hook for PreToolUse (Bash), task 41 part 2, Q-207 OPTION 1: in a clerk session, deny a command
# with the word git (\bgit\b, the same test as the guard hold of refusal-stop.sh) together with ;,
# &&, ||, |, a line break, or cd. The Claude Code worktree guard refuses that shape, and its
# refusal holds the clerk; this deny comes first and only tells the agent to split the command.
# A PreToolUse deny is no refusal: refusal-stop.sh writes no hold for it (spec 15.1.6). It reads
# the command text as a closed grammar, never its meaning, and parses no shell: a quoted separator
# counts. Without jq it allows the call; the guard still stands.
case ${BRUH_ROLE_KEY:-} in
clerk-*) ;;
*) exit 0 ;;
esac
jq -c --arg r 'bruh git-chain: this Bash command has the word git together with ;, &&, ||, |, a line break, or cd, and the Claude Code worktree guard refuses that shape. This deny is not a refusal: do not escalate it and do not open a question. Split the command: run each part alone, as its own single plain Bash call (a git command alone, with no cd; use absolute paths), then go on with your work.' \
	'(.tool_input.command // "") as $c
	| select(($c | test("\\bgit\\b")) and ($c | test("[;|\n]|&&|\\bcd\\b")))
	| {hookSpecificOutput: {hookEventName: "PreToolUse", permissionDecision: "deny", permissionDecisionReason: $r}}' 2> /dev/null
exit 0
