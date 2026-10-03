#!/bin/sh
# Learner eval (spec section 20). See tests/eval/README.md.
#
# Runs bruh:learner of this checkout on each case of tests/eval/learner/ and
# checks its LINK and DOC lines against expected.txt of the case.
#
# Usage: sh tests/eval/run.sh [--dry-run | --compare <expected> <output>]
set -eu
here=$(cd "$(dirname "$0")" && pwd)
plugin=$(cd "$here/../.." && pwd)/plugins/bruh
secs=${EVAL_TIMEOUT:-600}

usage() {
	echo "usage: sh tests/eval/run.sh [--dry-run | --compare <expected> <output>]" >&2
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

dry=0
case ${1:-} in
'') [ $# -eq 0 ] || usage ;;
--dry-run)
	[ $# -eq 1 ] || usage
	dry=1
	;;
--compare)
	[ $# -eq 3 ] || usage
	if compare "$2" "$3"; then exit 0; fi
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
for c in "$here"/learner/*/; do
	c=${c%/}
	name=${c##*/}
	cases=$((cases + 1))
	if [ "$dry" = 1 ]; then
		input=$(sed 's|<root>|<tmp>|g' "$c/input.txt")
		printf '%s: cd <tmp> && timeout %s claude -p --agent bruh:learner --plugin-dir %s "%s"\n' \
			"$name" "$secs" "$plugin" "$input"
		continue
	fi
	# A random folder name, so that the name of the case gives no hint to the learner.
	dir=$(mktemp -d "$work/XXXXXX")
	dir=$(cd "$dir" && pwd -P)
	cp -R "$c/root/." "$dir/"
	input=$(sed "s|<root>|$(printf '%s' "$dir" | sed 's/[&|\\]/\\&/g')|g" "$c/input.txt")
	out=$work/$name.out
	if (cd "$dir" && timeout "$secs" claude -p --agent bruh:learner --plugin-dir "$plugin" "$input") >"$out" 2>"$work/$name.err"; then
		if diff=$(compare "$c/expected.txt" "$out"); then
			echo "PASS $name"
			passed=$((passed + 1))
		else
			echo "FAIL $name"
			printf '%s\n' "$diff"
		fi
	else
		rc=$?
		echo "FAIL $name"
		echo "claude exited $rc (124 is a timeout of $secs seconds):"
		cat "$work/$name.err"
	fi
	rm -rf "$dir"
done

if [ "$dry" = 1 ]; then exit 0; fi
echo "learner eval: $passed of $cases"
[ "$passed" = "$cases" ]
