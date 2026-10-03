#!/bin/sh
# Posts a saved result of /bruh:review-only or /bruh:review-and-fix on a merge
# request (GitLab, glab) or a pull request (GitHub, gh): one summary note, one
# comment for each confirmed finding (inline on its line when the host takes the
# position, else a general comment that names file:line), and for review-and-fix
# one note of the fixes. A body that is already on the pull request is not posted
# again, so a rerun after a partial failure posts only what is missing.
#
#   post-findings.sh [--dry-run] [--yes] [--answer Q-<id>] [--data <folder>] [--hostname <host>] gitlab|github <repo> <number> <result.json>
#
# A post is outward-facing and goes out under an account of the owner (spec 13).
# Without --dry-run it needs a cover, checked by structure only:
# - no BRUH_ROLE_KEY (a manual session of the owner): --yes, after the owner said yes;
# - a role session: --answer Q-<id>, with a message from bigm in the mailbox of the
#   caller whose header is exactly "ANSWER Q-<id>: post <repo>#<number> at <sha> approved",
#   where <sha> (7 to 40 hex) is a prefix of the head_sha of the result, or
#   a row of the section "Post grants" of grants.md for BRUH_ROLE_KEY, the host name
#   (--hostname <host>, default gitlab.com or github.com), and the repository.
#   --yes is refused in a role session.
# It finds the mailbox and grants.md (through ledger_path in <data>/init/config.json)
# in the data folder, as the merge train does. This is a speed bump (principle 2):
# a session with Bash can still post by other means.
#
# A failed read of the comments that are on the pull request (any page) stops the
# script before the first post, so a rerun never posts a body twice.
#
# Each body starts and ends with the marker line <!-- bruh:<role key> -->
# (<!-- bruh:owner --> without BRUH_ROLE_KEY): the watcher reads the last line.
# Each payload is built whole by jq and sent with --input, so the text of a
# finding never passes through the shell, and glab gets no bracketed field names.
set -eu

usage='usage: post-findings.sh [--dry-run] [--yes] [--answer Q-<id>] [--data <folder>] [--hostname <host>] gitlab|github <repo> <number> <result.json>'
err() { printf 'post-findings.sh: %s\n' "$*" >&2; }
bad() {
	err "$*"
	exit 2
}

dry=0
yes=0
answer=
data=${BRUH_DATA:-}
hostname=
while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) dry=1 ;;
	--yes) yes=1 ;;
	--answer)
		[ $# -ge 2 ] || bad "$usage"
		answer=$2
		shift
		;;
	--data)
		[ $# -ge 2 ] || bad "$usage"
		data=$2
		shift
		;;
	--hostname)
		[ $# -ge 2 ] || bad "$usage"
		hostname=$2
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
gitlab) re='^([0-9]+|[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)+)$' cli=glab dflt=gitlab.com ;;
github) re='^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$' cli=gh dflt=github.com ;;
*) bad "$usage" ;;
esac
hostname=${hostname:-$dflt}
case $hostname in *"
"*) bad "bad host name: a newline" ;; esac
printf '%s\n' "$hostname" | grep -Eqx '[A-Za-z0-9.-]+' || bad "bad host name: $hostname"
case $repo in *"
"*) bad "bad repository for $host: a newline" ;; esac
printf '%s\n' "$repo" | grep -Eqx "$re" || bad "bad repository for $host: $repo"
if [ -n "$answer" ]; then
	# The ERE copy of qidPattern in plugins/bruh/mcp/env.go (spec 5, decision D2).
	printf '%s\n' "$answer" | grep -Eqx 'Q-[a-z0-9]+(-[a-z0-9]+)+-[0-9]+' || bad "bad question ID: $answer"
fi
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

# config_ledger prints the ledger path of the init config of the data folder.
config_ledger() { jq -r '.ledger_path // empty' "$data/init/config.json" 2>/dev/null; }

# has_grant exits 0 when the "Post grants" section of grants.md has a row
# | <role key> | <host name> | <repository> | ... for BRUH_ROLE_KEY, the host name, and the repository.
has_grant() {
	[ -n "$data" ] || return 1
	ledger=$(config_ledger) || return 1
	[ -n "$ledger" ] && [ -f "$ledger/grants.md" ] || return 1
	awk -v key="$BRUH_ROLE_KEY" -v host="$hostname" -v repo="$repo" '
		function cell(s) { gsub(/^[ \t`]+|[ \t`]+$/, "", s); return s }
		/^## / { post = ($0 ~ /^## Post grants[ \t]*$/); next }
		post && /^\|/ { split($0, c, "|"); if (cell(c[2]) == key && cell(c[3]) == host && cell(c[4]) == repo) found = 1 }
		END { exit !found }' "$ledger/grants.md"
}

# has_answer exits 0 when the mailbox of BRUH_ROLE_KEY (read or not) holds a message
# from bigm with the exact approval header of this post, for the reviewed head: an
# approval of an earlier review of the same pull request does not cover this one.
has_answer() {
	[ -n "$data" ] && [ -n "$answer" ] || return 1
	head_sha=$(jq -r '.head_sha // ""' "$file")
	printf '%s\n' "$head_sha" | grep -Eqx '[0-9a-f]{40}' || return 1
	want="ANSWER $answer: post $repo#$num at "
	for m in "$data/mail/$BRUH_ROLE_KEY"/*.json "$data/mail/$BRUH_ROLE_KEY"/read/*.json; do
		[ -f "$m" ] || continue
		if jq -e --arg p "$want" --arg head "$head_sha" '
			.from == "bigm" and (.header | startswith($p) and endswith(" approved"))
			and (.header[($p | length):-(" approved" | length)] as $sha
				| ($sha | test("^[0-9a-f]{7,40}$")) and ($head | startswith($sha)))' "$m" >/dev/null 2>&1; then return 0; fi
	done
	return 1
}

refuse() {
	err "refused: a post is outward-facing and goes out under an account of the owner. $*"
	exit 3
}

# check_identity refuses when the chosen remote of the repository, in the first
# repository of a learn/projects/*.json index file of the ledger with this
# host_path and host name, uses an SSH host alias (the host part of its scp or
# ssh:// URL differs from the host name; an https URL has none), and no row of
# "## Identities" of projects/<key>.md names the alias with a non-empty account
# (spec 8.5, G10). No ledger or no index file: no check.
check_identity() {
	ledger=$(config_ledger) || return 0
	[ -n "$ledger" ] || return 0
	set -- "$ledger"/learn/projects/*.json
	[ -f "$1" ] || return 0
	# The key is the file name, and neither it nor the alias holds a "/".
	found=$(jq -rn --arg repo "$repo" --arg h "$hostname" '
		first(inputs | (input_filename | sub(".*/"; "") | sub("\\.json$"; "")) as $k
			| .repos[]? | select(.host_path == $repo and .host.value == $h) | [$k, .])
		| .[0] as $k | .[1] as $r
		| ([$r.remotes[]? | select(.name == $r.remote) | .url | strings][0] // "") as $u
		| if $u | startswith("ssh://") then $u | capture("^ssh://([^/]*@)?(?<h>[^/:@]+)").h
			elif $u | contains("://") then empty
			else $u | capture("^(?<p>[^:/]+):").p | sub(".*@"; "") end
		| ascii_downcase | select(. != ($h | ascii_downcase))
		| "\($k)/\(.)"' "$@") || {
		err "could not read the index files in $ledger/learn/projects"
		exit 1
	}
	[ -n "$found" ] || return 0
	pkey=${found%%/*}
	halias=${found#*/}
	awk -v alias="$halias" '
		function cell(s) { gsub(/^[ \t`]+|[ \t`]+$/, "", s); return s }
		/^#/ { ids = ($0 ~ /^## Identities[ \t]*$/); next }
		ids && /^\|/ { split($0, c, "|"); if (cell(c[2]) == alias && cell(c[3]) != "") found = 1 }
		END { exit !found }' "$ledger/projects/$pkey.md" 2>/dev/null && return 0
	err "refused: the remote of $repo uses the SSH host alias $halias; the owner confirms the account in \"Identities\" of projects/$pkey.md"
	exit 3
}
if [ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "$data" ]; then check_identity; fi

if [ "$dry" = 0 ]; then
	if [ -z "${BRUH_ROLE_KEY:-}" ]; then
		[ "$yes" = 1 ] || refuse "In a manual session, pass --yes only after the owner said yes to this post."
	else
		[ "$yes" = 0 ] || refuse "--yes works only in a manual session of the owner; a role session needs --answer Q-<id> or a post grant."
		if ! has_answer && ! has_grant; then
			refuse "Ask bigm for the ANSWER \"ANSWER Q-<id>: post $repo#$num at <head SHA> approved\" and pass --answer Q-<id>, or ask the owner for a row in the section \"Post grants\" of grants.md for $key, $hostname, and $repo."
		fi
	fi
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

# fetch <file> <command>: runs one read into its own file. Any failure (also of a
# later page of --paginate) stops the script before the first post.
fetch() {
	out=$1
	shift
	if ! "$@" >"$out" 2>"$tmp/read-err.txt"; then
		cat "$tmp/read-err.txt" >&2
		err "could not read the comments of $repo#$num ($*); posted nothing"
		exit 1
	fi
	jq -e . "$out" >/dev/null 2>&1 || {
		err "the comments of $repo#$num are not JSON ($*); posted nothing"
		exit 1
	}
}

# Read the pull request and the bodies that are on it already.
if [ "$host" = gitlab ]; then
	api="projects/$(jq -rn --arg p "$repo" '$p | @uri')/merge_requests/$num"
	glab api --hostname "$hostname" "$api" >"$tmp/pr.json"
	jq -e '.diff_refs.head_sha' "$tmp/pr.json" >/dev/null || {
		err "$api has no diff_refs yet; retry when GitLab has computed the diff"
		exit 1
	}
	web_url=$(jq -r '.web_url' "$tmp/pr.json")
	head=$(jq -r '.diff_refs.head_sha' "$tmp/pr.json")
	fetch "$tmp/page-discussions.json" glab api --hostname "$hostname" --paginate "$api/discussions"
	jq -s '[.[][] | .notes[]? | .body]' "$tmp/page-discussions.json" >"$tmp/existing.json"
else
	api="repos/$repo"
	gh api --hostname "$hostname" "$api/pulls/$num" >"$tmp/pr.json"
	web_url=$(jq -r '.html_url' "$tmp/pr.json")
	head=$(jq -r '.head.sha' "$tmp/pr.json")
	fetch "$tmp/page-issue.json" gh api --hostname "$hostname" --paginate "$api/issues/$num/comments"
	fetch "$tmp/page-pull.json" gh api --hostname "$hostname" --paginate "$api/pulls/$num/comments"
	jq -s '[.[][] | .body]' "$tmp/page-issue.json" "$tmp/page-pull.json" >"$tmp/existing.json"
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
	if "$cli" api --hostname "$hostname" -X POST "$1" -H 'Content-Type: application/json' --input "$2" >"$tmp/resp.json" 2>"$tmp/err.txt"; then
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
