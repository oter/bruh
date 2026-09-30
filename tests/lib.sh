# shellcheck shell=sh
# Shared functions of the bruh smoke test and load test drivers.
# Source this file. POSIX sh and jq only.

BRUH_MIN_CLAUDE=2.1.284
DRY=${DRY:-0}

# show prints a command the way a shell reads it, after a "+" sign, on stderr.
show() {
	out=
	for a in "$@"; do
		case $a in
		'' | *[[:space:]]* | *'"'*) a="'$a'" ;;
		esac
		out="$out${out:+ }$a"
	done
	printf '+ %s\n' "$out" >&2
}

# run runs a command, or only prints it in dry-run mode.
run() {
	if [ "$DRY" = 1 ]; then show "$@"; else "$@"; fi
}

# run_in runs a command in a folder, or only prints it in dry-run mode.
run_in() {
	dir=$1
	shift
	if [ "$DRY" = 1 ]; then
		printf '+ (cd %s)\n' "$dir" >&2
		show "$@"
	else
		(cd "$dir" && "$@")
	fi
}

# result prints one PASS, FAIL, or SKIP line and appends it to $RESULTS.
result() {
	line="$1 $2${3:+ - $3}"
	printf '%s\n' "$line"
	if [ -n "${RESULTS:-}" ]; then printf '%s\n' "$line" >>"$RESULTS"; fi
}

# words joins the lines of standard input with spaces.
words() { tr '\n' ' ' | sed 's/ $//'; }

die() {
	printf 'error: %s\n' "$*" >&2
	exit 2
}

utc_now() { date -u +%Y-%m-%dT%H:%M:%S.000Z; }

# utc_ago prints the UTC time <seconds> before now, in the layout of the MCP server.
utc_ago() {
	t=$(($(date -u +%s) - $1))
	date -u -r "$t" +%Y-%m-%dT%H:%M:%S.000Z 2>/dev/null || date -u -d "@$t" +%Y-%m-%dT%H:%M:%S.000Z
}

# version_ge exits 0 when the dotted version $1 is at least $2.
version_ge() {
	printf '%s\n%s\n' "$2" "$1" | sort -t. -k1,1n -k2,2n -k3,3n -C
}

# live_named prints the id of each session in the agents JSON file $1 that has
# the name $2 and a live process.
live_named() {
	jq -r --arg n "$2" '.[] | select(.name == $n and .pid != null) | .id' "$1"
}

# live_conflicts prints "<name> (<id>)" for each session in the agents JSON file
# $1 that has a live process and a name that starts with one of the other
# arguments. A driver refuses to start when a name that it will use is live.
live_conflicts() {
	file=$1
	shift
	jq -r --args '
		.[]
		| select(.pid != null)
		| select((.name // "") as $n | any($ARGS.positional[]; . as $x | $n | startswith($x)))
		| "\(.name) (\(.id // "no id"))"' "$@" <"$file"
}

# valid_run_id exits 0 when $1 is a run ID: 1 to 12 lowercase letters and digits.
# A run ID is part of role keys and of a folder that the cleanup removes.
valid_run_id() {
	case $1 in
	'' | *[!abcdefghijklmnopqrstuvwxyz0123456789]*) return 1 ;;
	esac
	[ ${#1} -le 12 ]
}

# abs_dir prints the absolute physical path of the folder $1.
abs_dir() { (cd "$1" 2>/dev/null && pwd -P); }

# own_sessions prints the id of each session in the agents JSON file $1 whose
# cwd is the folder $2 or is below it, and whose name starts with one of the
# other arguments.
own_sessions() {
	file=$1
	prefix=$2
	shift 2
	jq -r --arg p "$prefix" --args '
		.[]
		| select(.id != null)
		| select((.cwd // "") as $c | $c == $p or ($c | startswith($p + "/")))
		| select((.name // "") as $n | any($ARGS.positional[]; . as $x | $n | startswith($x)))
		| .id' "$@" <"$file"
}

# check_trusted_repo exits 0 when $1 is a git repository with a commit and no
# remote. Else it prints the reason.
check_trusted_repo() {
	if [ -z "$1" ]; then
		echo "BRUH_TRUSTED_REPO is not set"
		return 1
	fi
	if [ ! -d "$1" ]; then
		echo "$1 is not a folder"
		return 1
	fi
	if ! git -C "$1" rev-parse --show-toplevel >/dev/null 2>&1; then
		echo "$1 is not a git repository"
		return 1
	fi
	if ! git -C "$1" rev-parse --verify -q HEAD >/dev/null; then
		echo "$1 has no commit; make one commit first"
		return 1
	fi
	if [ -n "$(git -C "$1" remote)" ] && [ "${SMOKE_ALLOW_REMOTE:-0}" != 1 ]; then
		echo "$1 has a git remote; a test clerk could push to it; use a local repository with no remote, or set SMOKE_ALLOW_REMOTE=1"
		return 1
	fi
}

# push_block_env prints a JSON object of GIT_CONFIG_* variables that set the
# push URL of every remote of the repository $1 to a path that does not exist,
# so that no git push of a session with these variables reaches a remote.
push_block_env() {
	git -C "$1" remote | jq -R . | jq -s '
		to_entries
		| map({("GIT_CONFIG_KEY_\(.key)"): "remote.\(.value).pushurl",
		       ("GIT_CONFIG_VALUE_\(.key)"): "/nonexistent/bruh-smoke-no-push"})
		| add // {}
		| . + {GIT_CONFIG_COUNT: ((length / 2) | tostring)}'
}

# add_push_block adds the push_block_env variables of the repository $2 to the
# "env" object of the settings file $1.
add_push_block() {
	_env=$(push_block_env "$2") || return 1
	jq --argjson e "$_env" '.env = ((.env // {}) + $e)' "$1" >"$1.new" && mv "$1.new" "$1"
}

# count_missed prints the count of unread messages in the mailbox folder $1
# (not in read/) that were posted before the UTC time $2.
count_missed() {
	cat "$1"/*.json 2>/dev/null | jq -s --arg c "$2" '[.[] | select(.at < $c)] | length'
}

# count_all prints the count of messages in the mailbox folder $1, read or not.
count_all() {
	cat "$1"/*.json "$1"/read/*.json 2>/dev/null | jq -s 'length'
}

# data_dir prints the plugin data folder of bruh when Claude Code loads it with
# --plugin-dir (plugin id bruh@inline).
data_dir() {
	printf '%s\n' "${BRUH_TEST_DATA:-${CLAUDE_CODE_PLUGIN_CACHE_DIR:-$HOME/.claude/plugins}/data/bruh-inline}"
}

# mcp_call sends one tools/call request to the MCP server binary $BRUH_TEST_MCP
# as the role $BRUH_ROLE_KEY, and prints the text of the result. It exits 1 when
# the tool returns an error. In dry-run mode it prints the request and prints {}.
mcp_call() {
	req=$(jq -cn --arg t "$1" --argjson a "$2" '{jsonrpc:"2.0",id:1,method:"tools/call",params:{name:$t,arguments:$a}}')
	if [ "$DRY" = 1 ]; then
		printf '+ mcp as %s: %s\n' "${BRUH_ROLE_KEY:-}" "$req" >&2
		echo '{}'
		return 0
	fi
	resp=$(printf '%s\n' "$req" | BRUH_DATA="$BRUH_DATA" BRUH_ROLE_KEY="$BRUH_ROLE_KEY" \
		BRUH_PLUGIN_ROOT="$BRUH_PLUGIN_ROOT" "$BRUH_TEST_MCP")
	if ! printf '%s\n' "$resp" | jq -e 'select(.id == 1) | .error == null and (.result.isError // false) == false' >/dev/null; then
		printf 'error: %s failed: %s\n' "$1" "$resp" >&2
		return 1
	fi
	printf '%s\n' "$resp" | jq -r 'select(.id == 1) | .result.content[0].text'
}

# build_mcp builds the MCP server of the plugin $1 into the file $2.
build_mcp() {
	run env GOTOOLCHAIN=local go build -C "$1/mcp" -o "$2" .
}

# agents_json writes the output of `claude agents --json --all` to the file $1.
agents_json() {
	if [ "$DRY" = 1 ]; then
		echo '[]' >"$1"
	else
		claude agents --json --all >"$1" && jq -e 'type == "array"' "$1" >/dev/null
	fi
}

# add_scratch_worktree adds a linked worktree of the trusted repository $1 at
# $2, on a new orphan branch $3 with no files. A linked worktree of a trusted
# repository is trusted, and a new `git init` folder is not.
add_scratch_worktree() {
	run git -C "$1" worktree add -q --detach "$2" HEAD
	run git -C "$2" checkout -q --orphan "$3"
	run git -C "$2" rm -rfq --ignore-unmatch .
}

# commit_all commits every file of the worktree $1 with a fixed test identity.
commit_all() {
	run git -C "$1" add -A
	run git -C "$1" -c user.name=bruh-test -c user.email=bruh-test@example.com commit -q -m "$2"
}

# wait_for polls a command every 10 seconds until it succeeds or $1 minutes pass.
wait_for() {
	minutes=$1
	shift
	if [ "$DRY" = 1 ]; then
		printf '+ wait up to %s minutes for: %s\n' "$minutes" "$*" >&2
		return 0
	fi
	end=$(($(date -u +%s) + minutes * 60))
	while :; do
		if "$@"; then return 0; fi
		[ "$(date -u +%s)" -lt "$end" ] || return 1
		sleep 10
	done
}

# stop_sessions stops each session id given, and returns 1 when one still has a
# live process after 60 seconds.
stop_sessions() {
	if [ "$DRY" = 1 ]; then
		for id in "$@"; do show claude stop "$id"; done
		return 0
	fi
	for id in "$@"; do
		claude stop "$id" >/dev/null 2>&1 || true
	done
	[ $# -gt 0 ] || return 0
	i=0
	while [ $i -lt 6 ]; do
		claude agents --json --all >"${TMPDIR:-/tmp}/bruh-test-agents.$$" 2>/dev/null || return 1
		live=$(jq -r --args '[.[] | select(.pid != null and (.id as $i | $ARGS.positional | index($i)))] | length' "$@" <"${TMPDIR:-/tmp}/bruh-test-agents.$$")
		rm -f "${TMPDIR:-/tmp}/bruh-test-agents.$$"
		[ "$live" = 0 ] && return 0
		i=$((i + 1))
		sleep 10
	done
	return 1
}

# random_id prints 6 random lowercase letters and digits.
random_id() {
	LC_ALL=C tr -dc 'a-z0-9' </dev/urandom | head -c 6
}

# preflight_common checks the tools, the Claude Code version, and the trusted
# repository. It prints the reason of a failure.
preflight_common() {
	for t in claude jq git go; do
		if ! command -v "$t" >/dev/null 2>&1; then
			echo "missing tool: $t"
			return 1
		fi
	done
	v=$(claude --version | awk '{print $1}')
	if ! version_ge "$v" "$BRUH_MIN_CLAUDE"; then
		echo "Claude Code $v is older than $BRUH_MIN_CLAUDE"
		return 1
	fi
	check_trusted_repo "${BRUH_TRUSTED_REPO:-}"
}

# record_branches writes the branches and the HEAD commit of the trusted
# repository $1 into the evidence folder $2, before a run creates anything.
record_branches() {
	git -C "$1" for-each-ref --format='%(refname:short)' refs/heads >"$2/branches-before.txt" &&
		git -C "$1" rev-parse HEAD >"$2/trusted-head.txt"
}

# remove_scratch removes the scratch folder $2 of the trusted repository $1 and
# the branches that the run created. $3 is the evidence folder of
# record_branches. A branch of the run is new and does not contain the HEAD
# commit of the trusted repository, because each run branch starts from an
# orphan commit. A new branch that contains that commit was made by somebody
# else, and it stays. It returns 1 when the records are missing; then it
# deletes no branch.
remove_scratch() {
	rm -rf "$2"
	git -C "$1" worktree prune
	git -C "$1" for-each-ref --format='%(refname:short)' refs/heads >"$3/branches-after.txt"
	[ -s "$3/branches-before.txt" ] && [ -s "$3/trusted-head.txt" ] || return 1
	head=$(cat "$3/trusted-head.txt")
	grep -vxF -f "$3/branches-before.txt" "$3/branches-after.txt" | while read -r b; do
		if ! git -C "$1" merge-base --is-ancestor "$head" "refs/heads/$b" 2>/dev/null; then
			git -C "$1" branch -D -q "$b" || true
		fi
	done
}

# remove_role_data removes the bruh data files of each role key after the data folder $1.
remove_role_data() {
	d=$1
	shift
	for key in "$@"; do
		rm -rf "$d/mail/$key" "$d/answers/$key" "$d/roles/$key.json" \
			"$d/handoffs/$key.md" "$d/handoffs/$key.history.md" "$d/reports/$key.jsonl"
	done
}

# window_gaps prints the count of windows of $4 seconds, from the epoch time $2
# to the epoch time $3, in which the mailbox folder $1 (read or not) got no
# message whose header contains "load tick". A last window that is shorter
# than $4 is not counted.
window_gaps() {
	cat "$1"/*.json "$1"/read/*.json 2>/dev/null | jq -s --argjson a "$2" --argjson b "$3" --argjson w "$4" '
		[.[] | select(.header | contains("load tick")) | .at | sub("\\.[0-9]+Z$"; "Z") | fromdateiso8601] as $t
		| [range($a; $b - $w + 1; $w) as $s | select([$t[] | select(. >= $s and . < $s + $w)] | length == 0)]
		| length'
}
