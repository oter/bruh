#!/bin/sh
# Posts a saved result of /bruh:review-only or /bruh:review-and-fix on a merge
# request (GitLab, glab) or a pull request (GitHub, gh): one summary note, one
# comment for each confirmed finding (inline on its line when the host takes the
# position, else a general comment that names file:line), and for review-and-fix
# one note of the fixes. A body that is already on the pull request is not posted
# again, so a rerun after a partial failure posts only what is missing.
#
#   post-findings.sh [--dry-run] [--yes] [--data <folder>] gitlab|github <repo> <number> <result.json>
#
# A post is outward-facing and goes out under an account of the owner (spec 13).
# Without --dry-run the script refuses unless it has --yes (the owner said yes to
# this post) or the section "Post grants" of grants.md has a row for BRUH_ROLE_KEY
# and the repository. It finds grants.md through ledger_path in
# <data>/init/config.json, as the merge train does. This is a speed bump
# (principle 2): a session with Bash can still post by other means.
#
# Each body starts and ends with the marker line <!-- bruh:<role key> -->
# (<!-- bruh:owner --> without BRUH_ROLE_KEY): the watcher reads the last line.
# Each payload is built whole by jq and sent with --input, so the text of a
# finding never passes through the shell, and glab gets no bracketed field names.
set -eu

usage='usage: post-findings.sh [--dry-run] [--yes] [--data <folder>] gitlab|github <repo> <number> <result.json>'
err() { printf 'post-findings.sh: %s\n' "$*" >&2; }
bad() {
	err "$*"
	exit 2
}

dry=0
yes=0
data=${BRUH_DATA:-}
while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) dry=1 ;;
	--yes) yes=1 ;;
	--data)
		[ $# -ge 2 ] || bad "$usage"
		data=$2
		shift
		;;
	-*) bad "$usage" ;;
	*) break ;;
	esac
	shift
done
[ $# -eq 4 ] || bad "$usage"
host=$1
repo=$2
num=$3
file=$4
case $num in '' | *[!0-9]*) bad "$usage" ;; esac
case $host in
gitlab) re='^([0-9]+|[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+)$' cli=glab ;;
github) re='^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' cli=gh ;;
*) bad "$usage" ;;
esac
printf '%s\n' "$repo" | grep -Eqx "$re" || bad "bad repository for $host: $repo"
key=${BRUH_ROLE_KEY:-owner}
case $key in '' | *[!a-z0-9-]*) bad "bad BRUH_ROLE_KEY: $key" ;; esac
mark="<!-- bruh:$key -->"

[ -f "$file" ] || bad "no result file: $file"
jq -e '.status == "done" or .status == "findings_left"' "$file" >/dev/null 2>&1 ||
	bad "$file: the status is not done or findings_left; a stopped review is not complete, so it is not posted"
jq -e '(.workflow == "review-only" or .workflow == "review-and-fix")
	and (.summaries | type == "array") and (.refuted | type == "array") and (.confirmed | type == "array")
	and all(.confirmed[]; (.line | type == "number")
		and ([.file, .rule, .problem, .fix, .severity, .lens] | all(type == "string")))' "$file" >/dev/null 2>&1 ||
	bad "$file: needs workflow, summaries, refuted, and confirmed[] with file, line, lens, rule, severity, problem, fix"

# has_grant exits 0 when the "Post grants" section of grants.md has a row
# | <role key> | <repository> | ... for BRUH_ROLE_KEY and the repository.
has_grant() {
	[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "$data" ] || return 1
	ledger=$(jq -r '.ledger_path // empty' "$data/init/config.json" 2>/dev/null) || return 1
	[ -n "$ledger" ] && [ -f "$ledger/grants.md" ] || return 1
	awk -v key="$BRUH_ROLE_KEY" -v repo="$repo" '
		function cell(s) { gsub(/^[ \t`]+|[ \t`]+$/, "", s); return s }
		/^## / { post = ($0 ~ /^## Post grants[ \t]*$/); next }
		post && /^\|/ { split($0, c, "|"); if (cell(c[2]) == key && cell(c[3]) == repo) found = 1 }
		END { exit !found }' "$ledger/grants.md"
}
if [ "$dry" = 0 ] && [ "$yes" = 0 ] && ! has_grant; then
	err "refused: a post is outward-facing and goes out under an account of the owner."
	err "Pass --yes only after the owner said yes to this post, or ask the owner for a row in the section \"Post grants\" of grants.md for $key and $repo."
	exit 3
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# Read the pull request and the bodies that are on it already.
if [ "$host" = gitlab ]; then
	api="projects/$(jq -rn --arg p "$repo" '$p | @uri')/merge_requests/$num"
	glab api "$api" >"$tmp/pr.json"
	jq -e '.diff_refs.head_sha' "$tmp/pr.json" >/dev/null || {
		err "$api has no diff_refs yet; retry when GitLab has computed the diff"
		exit 1
	}
	web_url=$(jq -r '.web_url' "$tmp/pr.json")
	head=$(jq -r '.diff_refs.head_sha' "$tmp/pr.json")
	glab api --paginate "$api/discussions" | jq -s '[.[][] | .notes[]? | .body]' >"$tmp/existing.json"
else
	api="repos/$repo"
	gh api "$api/pulls/$num" >"$tmp/pr.json"
	web_url=$(jq -r '.html_url' "$tmp/pr.json")
	head=$(jq -r '.head.sha' "$tmp/pr.json")
	{
		gh api --paginate "$api/issues/$num/comments"
		gh api --paginate "$api/pulls/$num/comments"
	} | jq -s '[.[][] | .body]' >"$tmp/existing.json"
fi
reviewed=$(jq -r '.head_sha // ""' "$file")
stale=0
if [ -n "$reviewed" ] && [ "$reviewed" != "$head" ]; then stale=1; fi

# shellcheck disable=SC2016 # jq, not the shell, expands $m
DEFS='
def wrap($m): "\($m)\n" + . + "\n\($m)";
def trim: gsub("\\s+"; " ") | if length > 300 then .[:299] + "..." else . end;
def at: "`\(.file):\(.line)`";
def finding_body($m): "**Agent review** finding at \(at) [\(.severity)]\n\n\(.problem)\n\n**Fix:** \(.fix)\n\n<sub>\(.lens) - \(.rule)</sub>" | wrap($m);
'

# The summary note.
jq --arg m "$mark" --argjson stale "$stale" --arg head "$head" "$DEFS"'
	["security", "bug", "guideline", "nit"] as $order
	| [.confirmed[].severity] as $sev
	| ($order | map(. as $k | ($sev | map(select(. == $k)) | length) as $n | select($n > 0) | "\($n) \($k)") | join(", ")) as $counts
	| {body: ([
		"**Agent review** of `\(.head_sha // "" | .[:8])` (\(.workflow)): \(.confirmed | length) confirmed\(if $counts != "" then " (\($counts))" else "" end), \(.refuted | length) refuted by one refuter each. Lenses: \(.summaries | map(.key) | join(", ")).",
		(if .status == "findings_left" then "", "The review ended at its round cap with open findings." else empty end),
		(if $stale == 1 then "", "The pull request has moved to `\($head[:8])` since the review, so the findings are general comments that name the file and the line." else empty end),
		"",
		(.summaries[] | "- **\(.key):** \(.summary | trim)")
	] | join("\n") | wrap($m))}' "$file" >"$tmp/summary.json"

# The note of the fixes, for review-and-fix only.
jq --arg m "$mark" "$DEFS"'
	if .workflow == "review-and-fix" and (.confirmed | length) > 0 then
		([.confirmed[] | select(.state == "fixed")]) as $fixed
		| ([.confirmed[] | select(.state != "fixed")]) as $open
		| {body: ([
			"**Agent review** fixes: \($fixed | length) of \(.confirmed | length) findings fixed in the working tree. The next push has them.",
			(if ($fixed | length) > 0 then "", "Fixed: \($fixed | map(at) | join(", "))" else empty end),
			(if ($open | length) > 0 then "", ($open[] | "- Open \(at)") else empty end)
		] | join("\n") | wrap($m))}
	else empty end' "$file" >"$tmp/fixes.json"

posted() { jq -e --slurpfile p "$1" '$p[0].body as $b | any(.[]; . == $b)' "$tmp/existing.json" >/dev/null; }

# post <api path> <payload file>: exit 0 on success, 2 when the host rejects an
# inline position (GitLab 400, GitHub 422), and 1 on any other failure.
post() {
	if "$cli" api -X POST "$1" -H 'Content-Type: application/json' --input "$2" >"$tmp/resp.json" 2>"$tmp/err.txt"; then
		return 0
	fi
	if grep -Eq 'HTTP (400|422)' "$tmp/err.txt"; then return 2; fi
	return 1
}
url() {
	if [ "$host" = gitlab ]; then
		printf '%s#note_%s\n' "$web_url" "$(jq -r '.notes[0].id // .id' "$tmp/resp.json")"
	else
		jq -r '.html_url' "$tmp/resp.json"
	fi
}
general_path() { if [ "$host" = gitlab ]; then echo "$api/notes"; else echo "$api/issues/$num/comments"; fi; }
inline_path() { if [ "$host" = gitlab ]; then echo "$api/discussions"; else echo "$api/pulls/$num/comments"; fi; }

# note <payload file> <name>: posts one general note unless it is on the pull request.
note() {
	if posted "$1"; then
		echo "exists   $2"
	elif [ "$dry" = 1 ]; then
		echo "would post $2:"
		jq -r '.body' "$1" | sed 's/^/    /'
	elif post "$(general_path)" "$1"; then
		echo "posted   $2 $(url)"
	else
		cat "$tmp/err.txt" >&2
		err "the $2 failed"
		exit 1
	fi
}

note "$tmp/summary.json" "summary note"

inline=0 general=0 failed=0 existing=0
n=$(jq '.confirmed | length' "$file")
i=0
while [ "$i" -lt "$n" ]; do
	j=$i
	i=$((i + 1))
	jq --argjson i "$j" --arg m "$mark" "$DEFS"'.confirmed[$i] | {body: finding_body($m)}' "$file" >"$tmp/general.json"
	if [ "$host" = gitlab ]; then
		jq --argjson i "$j" --slurpfile g "$tmp/general.json" --slurpfile pr "$tmp/pr.json" '
			.confirmed[$i] as $f | $pr[0].diff_refs as $d
			| {body: $g[0].body, position: {position_type: "text", base_sha: $d.base_sha, start_sha: $d.start_sha,
				head_sha: $d.head_sha, old_path: $f.file, new_path: $f.file, new_line: $f.line}}' "$file" >"$tmp/inline.json"
	else
		jq --argjson i "$j" --slurpfile g "$tmp/general.json" --arg head "$head" '
			.confirmed[$i] as $f | {body: $g[0].body, commit_id: $head, path: $f.file, line: $f.line, side: "RIGHT"}' "$file" >"$tmp/inline.json"
	fi
	where=$(jq -r --argjson i "$j" '.confirmed[$i] | "\(.file):\(.line)"' "$file")
	if posted "$tmp/general.json"; then
		existing=$((existing + 1))
		echo "exists   $where"
		continue
	fi
	# Only a finding of the first round has a line in the reviewed head. A gate
	# finding has no line. A moved head makes every line stale.
	mode=inline
	if [ "$stale" = 1 ] || jq -e --argjson i "$j" '.confirmed[$i] | ((.round // 1) != 1) or (.file | startswith("gate: "))' "$file" >/dev/null; then
		mode=general
	fi
	if [ "$dry" = 1 ]; then
		echo "would post $mode $where:"
		jq -r '.body' "$tmp/general.json" | sed 's/^/    /'
		continue
	fi
	rc=0
	if [ "$mode" = inline ]; then
		post "$(inline_path)" "$tmp/inline.json" || rc=$?
		if [ "$rc" = 2 ]; then
			mode=general
			rc=0
		fi
	fi
	if [ "$mode" = general ]; then post "$(general_path)" "$tmp/general.json" || rc=$?; fi
	if [ "$rc" != 0 ]; then
		cat "$tmp/err.txt" >&2
		failed=$((failed + 1))
		echo "failed   $where"
		continue
	fi
	if [ "$mode" = inline ]; then inline=$((inline + 1)); else general=$((general + 1)); fi
	printf '%-8s %s %s\n' "$mode" "$where" "$(url)"
done

if [ -s "$tmp/fixes.json" ]; then note "$tmp/fixes.json" "note of the fixes"; fi

echo "$inline inline, $general general, $failed failed, $existing already posted"
[ "$failed" = 0 ]
