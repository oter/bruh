#!/bin/sh
# Evals of spec section 20. See tests/eval/README.md.
#
# learner (the default): runs bruh:learner of this checkout on each case of
# tests/eval/learner/ and checks its LINK and DOC lines against expected.txt of
# the case.
# routing: runs bruh:bigm of this checkout in a copy of the fixture ledger
# tests/eval/routing/ledger/ on each case of tests/eval/routing/, and checks its
# commitment line and its AskUserQuestion calls against expected.txt of the case.
#
# Usage: sh tests/eval/run.sh [learner | routing] [--dry-run | --compare <expected> <output>]
set -eu
here=$(cd "$(dirname "$0")" && pwd)
plugin=$(cd "$here/../.." && pwd)/plugins/bruh
secs=${EVAL_TIMEOUT:-600}

usage() {
	echo "usage: sh tests/eval/run.sh [learner | routing] [--dry-run | --compare <expected> <output>]" >&2
	exit 2
}

# compare prints the difference between the expected file $1 and the output
# file $2, and returns 1 when there is a difference. The rule of spec 20: each
# expected LINK and DOC line is in the output; an extra DOC line is allowed; a
# LINK line that is not expected fails.
compare() {
	got=$(grep -E '^(LINK |DOC:)' "$2" || true)
	bad=0
	while IFS= read -r line || [ -n "$line" ]; do
		[ -n "$line" ] || continue
		if ! printf '%s\n' "$got" | grep -Fxq -- "$line"; then
			echo "missing: $line"
			bad=1
		fi
	done <"$1"
	extra=$(printf '%s\n' "$got" | grep '^LINK ' | grep -vFx -f "$1" || true)
	if [ -n "$extra" ]; then
		printf '%s\n' "$extra" | sed 's/^/unexpected: /'
		bad=1
	fi
	return "$bad"
}

# The commitment line of bigm, "Work requests of the owner" of
# plugins/bruh/agents/bigm.md. agents_test.mjs checks that bigm.md has this form.
form='I send <work> to clanker-<project> as task <n>.'

# compare_routing checks the stream-json output $2 of a bigm run against the
# expected file $1, with exact rules only (spec 20):
#   ROUTE clanker-<p> <n>: a text line of bigm is the form above, with any text
#     for <work>, <p> for <project>, and <n> for <n>;
#   NOASK: no tool_use block has the name AskUserQuestion.
# Lines of the output that are not JSON are ignored.
compare_routing() {
	text=$(jq -rR 'fromjson? | if .type == "result" then .result // empty elif .type == "assistant" then .message.content[]? | select(.type == "text") | .text else empty end' "$2")
	asks=$(jq -R 'fromjson? | select(.type == "assistant") | .message.content[]? | select(.type == "tool_use" and .name == "AskUserQuestion") | .name' "$2" | grep -c . || true)
	bad=0
	while IFS= read -r line || [ -n "$line" ]; do
		case $line in
		'') ;;
		NOASK)
			if [ "$asks" -gt 0 ]; then
				echo "unexpected: $asks AskUserQuestion call(s)"
				bad=1
			fi
			;;
		*)
			if ! printf '%s\n' "$line" | grep -Eq '^ROUTE clanker-[a-z0-9-]+ [0-9]+$'; then
				echo "bad expected line: $line"
				bad=1
				continue
			fi
			rest=${line#ROUTE clanker-}
			if ! printf '%s' "$text" | jq -Rse --arg form "$form" --arg p "${rest% *}" --arg n "${rest##* }" \
				'($form | gsub("\\."; "\\.") | sub("<work>"; ".+") | sub("<project>"; $p) | sub("<n>"; $n)) as $re
				| split("\n") | any(test("^" + $re + "$"))' >/dev/null; then
				echo "missing: $line"
				bad=1
			fi
			;;
		esac
	done <"$1"
	return "$bad"
}

kind=learner
case ${1:-} in
learner | routing)
	kind=$1
	shift
	;;
esac

dry=0
case ${1:-} in
'') [ $# -eq 0 ] || usage ;;
--dry-run)
	[ $# -eq 1 ] || usage
	dry=1
	;;
--compare)
	[ $# -eq 3 ] || usage
	if [ "$kind" = routing ]; then
		if compare_routing "$2" "$3"; then exit 0; fi
	elif compare "$2" "$3"; then exit 0; fi
	exit 1
	;;
*) usage ;;
esac

if [ "$dry" = 0 ]; then
	work=$(mktemp -d)
	trap 'rm -rf "$work"' EXIT
fi
cases=0
passed=0
for c in "$here/$kind"/*/; do
	c=${c%/}
	name=${c##*/}
	# The shared fixture ledger of the routing eval is not a case.
	[ "$kind" = routing ] && [ "$name" = ledger ] && continue
	cases=$((cases + 1))
	if [ "$dry" = 1 ]; then
		if [ "$kind" = routing ]; then
			printf '%s: cd <tmp> && env -u BRUH_ROLE_KEY timeout %s claude -p --agent bruh:bigm --plugin-dir %s --tools Read,Grep,Glob,AskUserQuestion --strict-mcp-config --output-format stream-json --verbose "%s"\n' \
				"$name" "$secs" "$plugin" "$(cat "$c/input.txt")"
		else
			input=$(sed 's|<root>|<tmp>|g' "$c/input.txt")
			printf '%s: cd <tmp> && env -u BRUH_ROLE_KEY timeout %s claude -p --agent bruh:learner --plugin-dir %s "%s"\n' \
				"$name" "$secs" "$plugin" "$input"
		fi
		continue
	fi
	# A random folder name, so that the name of the case gives no hint to the agent.
	dir=$(mktemp -d "$work/XXXXXX")
	dir=$(cd "$dir" && pwd -P)
	out=$work/$name.out
	if [ "$kind" = routing ]; then
		# bigm gets only tools that read, no MCP server, and no role key, so the
		# run can write nothing. The hard-stop list comes from the defaults.
		cp -R "$here/routing/ledger/." "$dir/"
		cp "$plugin/defaults/priorities.md" "$dir/priorities.md"
		input=$(cat "$c/input.txt")
		if (cd "$dir" && env -u BRUH_ROLE_KEY timeout "$secs" claude -p --agent bruh:bigm --plugin-dir "$plugin" --tools Read,Grep,Glob,AskUserQuestion --strict-mcp-config --output-format stream-json --verbose "$input") >"$out" 2>"$work/$name.err"; then
			rc=0
		else
			rc=$?
		fi
	else
		cp -R "$c/root/." "$dir/"
		input=$(sed "s|<root>|$(printf '%s' "$dir" | sed 's/[&|\\]/\\&/g')|g" "$c/input.txt")
		if (cd "$dir" && env -u BRUH_ROLE_KEY timeout "$secs" claude -p --agent bruh:learner --plugin-dir "$plugin" "$input") >"$out" 2>"$work/$name.err"; then
			rc=0
		else
			rc=$?
		fi
	fi
	if [ "$rc" != 0 ]; then
		echo "FAIL $name"
		echo "claude exited $rc (124 is a timeout of $secs seconds):"
		cat "$work/$name.err"
	elif diff=$(if [ "$kind" = routing ]; then compare_routing "$c/expected.txt" "$out"; else compare "$c/expected.txt" "$out"; fi); then
		echo "PASS $name"
		passed=$((passed + 1))
	else
		echo "FAIL $name"
		printf '%s\n' "$diff"
	fi
	rm -rf "$dir"
done

if [ "$dry" = 1 ]; then exit 0; fi
echo "$kind eval: $passed of $cases"
[ "$passed" = "$cases" ]
