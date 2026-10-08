#!/bin/sh
# Hook for PreToolUse (AskUserQuestion): each ask of bigm to the owner shows on the /bruh-board
# pane with its buttons (owner rule R-15, spec 9.4). A select of bigm passes only when the text of
# each of its questions names an open question that the board shows: a questions/<id>.json with
# the priority P0 or P1 and no answers/*/<id>.answer. It reads exact values only: the tokens of the
# question ID pattern (qidPattern of mcp/env.go, copied word for word) and the files. It never
# judges the meaning of a text. Exit 2 with the reason on stderr blocks the call; without the data
# folder, without jq, or without a questions array, it blocks too. Other roles pass: no subagent
# gets AskUserQuestion, and a clanker or a clerk asks through question_open.
[ "${BRUH_ROLE_KEY:-}" = bigm ] || exit 0
deny() {
	echo 'R-15: every owner ask goes on the bruh board first: open it with question_open (P0 or P1, with options), then name its open Q-<id> in the text of each AskUserQuestion question' >&2
	exit 2
}
data=${CLAUDE_PLUGIN_DATA:-}
[ -n "$data" ] || deny
# One line per question: "q" and the question IDs in its text. The "q" keeps a question with no ID
# as a line of its own.
ids=$(jq -r '.tool_input.questions | if type == "array" and length > 0 then .[]
	| "q " + ([.question // "" | tostring | scan("Q-[a-z0-9]+(?:-[a-z0-9]+)+-[0-9]+")] | join(" "))
	else error("no questions") end' 2> /dev/null) || deny

# is_open <id>: the board shows the question <id>.
is_open() {
	f="$data/questions/$1.json"
	[ -f "$f" ] || return 1
	case $(jq -r '.priority // ""' "$f" 2> /dev/null) in
	P0 | P1) ;;
	*) return 1 ;;
	esac
	for a in "$data"/answers/*/"$1.answer"; do
		[ -e "$a" ] && return 1
	done
	return 0
}

printf '%s\n' "$ids" | while IFS= read -r line; do
	ok=
	# shellcheck disable=SC2086 # the IDs hold no space and no glob character
	set -- $line
	shift
	for id in "$@"; do
		is_open "$id" && ok=1 && break
	done
	[ -n "$ok" ] || exit 1
done || deny
exit 0
