#!/bin/sh
# Load test of the local transport of bruh (spec section 4.1). See tests/load/README.md.
#
# Starts <sessions> background sessions in a ring. Each session posts a message
# to the next session every <interval> minutes with mail_post and sends a
# SendMessage nudge. The driver resumes a session whose process is gone, counts
# the missed messages, and stops every session that it started.
#
# Usage: BRUH_TRUSTED_REPO=<repo> sh tests/load/run.sh [--sessions 8] [--minutes 60]
#          [--interval 2] [--model haiku] [--dry-run]
set -u
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
# shellcheck source=tests/lib.sh
. "$here/../lib.sh"

usage() {
	echo "usage: BRUH_TRUSTED_REPO=<repo> sh tests/load/run.sh [--sessions 8] [--minutes 60] [--interval 2] [--model haiku] [--dry-run]"
}
sessions=8
minutes=60
interval=2
model=haiku
while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) DRY=1 ;;
	--sessions | --minutes | --interval | --model)
		[ $# -ge 2 ] || die "$1 needs a value"
		case $1 in
		--sessions) sessions=$2 ;;
		--minutes) minutes=$2 ;;
		--interval) interval=$2 ;;
		--model) model=$2 ;;
		esac
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 2
		;;
	esac
	shift
done
for v in "$sessions" "$minutes" "$interval"; do
	case $v in '' | *[!0-9]*) die "--sessions, --minutes, and --interval take whole numbers" ;; esac
done
[ "$sessions" -ge 2 ] || die "--sessions must be 2 or more"
[ "$interval" -ge 1 ] && [ "$interval" -le 59 ] || die "--interval must be 1 to 59 minutes"
[ "$minutes" -ge 1 ] || die "--minutes must be 1 or more"
case $model in '' | *[!A-Za-z0-9._-]*) die "--model takes a model name such as haiku" ;; esac

plugin=$root/plugins/bruh
run_id=${LOAD_RUN:-$(random_id)}
project=load-$run_id
hub=clanker-$project
trusted=${BRUH_TRUSTED_REPO:-}
if [ "$DRY" = 1 ] && [ -z "$trusted" ]; then trusted='<trusted repo>'; fi
base=$trusted/.bruh-test/$project
work=$base/work
base_real=$base
evidence=${LOAD_EVIDENCE:-${TMPDIR:-/tmp}/bruh-load-$run_id}
ag=$evidence/agents.json
grace_seconds=300

# The stand-in caller of the MCP server. The driver acts as the hub clanker, the
# parent of the session role keys.
BRUH_DATA=$(data_dir)
BRUH_PLUGIN_ROOT=$plugin
BRUH_ROLE_KEY=$hub
BRUH_TEST_MCP=$evidence/bruh-mcp
data=$BRUH_DATA

key() { echo "clerk-$project-s$1"; }
keys() {
	i=1
	while [ "$i" -le "$sessions" ]; do
		key "$i"
		i=$((i + 1))
	done
}
fails=0
started=0

write_file() {
	if [ "$DRY" = 1 ]; then
		printf '+ write %s\n' "$1" >&2
		cat >/dev/null
	else
		cat >"$1"
	fi
}

cleanup() {
	trap - EXIT INT TERM
	echo "cleanup"
	if [ "$DRY" = 1 ]; then
		show claude stop '<each session of this run>'
		show rm -rf "$base"
		show git -C "$trusted" worktree prune
		show git -C "$trusted" branch -D "bruh-$project"
		return 0
	fi
	[ "$started" = 1 ] || return 0
	agents_json "$ag"
	ids=$(own_sessions "$ag" "$base_real" "clerk-$project-")
	for id in $ids; do
		name=$(jq -r --arg i "$id" '.[] | select(.id == $i) | .name' "$ag")
		claude logs "$id" >"$evidence/logs-$name.txt" 2>&1 || true
	done
	# shellcheck disable=SC2086 # ids is a list of short session IDs
	if ! stop_sessions $ids; then
		fails=$((fails + 1))
		result FAIL cleanup "a session of this run still has a process: $ids"
	fi
	if ! remove_scratch "$trusted" "$base" "$evidence"; then
		fails=$((fails + 1))
		result FAIL cleanup "no list of the branches before the run; no branch deleted"
	fi
	# shellcheck disable=SC2046 # keys prints one role key for each line
	remove_role_data "$data" "$hub" $(keys)
	echo "evidence: $evidence"
	if [ "$fails" -gt 0 ]; then exit 1; fi
}

# --- preflight and setup ----------------------------------------------------

echo "bruh load test, run $run_id: $sessions sessions, $minutes minutes, a message every $interval minutes, model $model"
if [ "$DRY" = 1 ]; then
	result DRY preflight
else
	reason=$(preflight_common) || die "preflight: $reason"
	mkdir -p "$evidence"
	RESULTS=$evidence/results.txt
	: >"$RESULTS"
	result PASS preflight "Claude Code $(claude --version | awk '{print $1}')"
fi
trap cleanup EXIT
trap 'exit 130' INT TERM

run mkdir -p "$base"
[ "$DRY" = 1 ] || git -C "$trusted" for-each-ref --format='%(refname:short)' refs/heads >"$evidence/branches-before.txt"
started=1
build_mcp "$plugin" "$BRUH_TEST_MCP"
add_scratch_worktree "$trusted" "$work" "bruh-$project"
[ "$DRY" = 1 ] || base_real=$(cd "$base" && pwd -P)
run mkdir -p "$work/.claude"
write_file "$work/.claude/settings.json" <<'EOF'
{
  "permissions": {
    "allow": ["mcp__plugin_bruh_bruh", "SendMessage", "ListAgents", "CronCreate"],
    "deny": ["Bash", "Edit", "Write", "NotebookEdit", "WebFetch", "WebSearch"]
  }
}
EOF
commit_all "$work" "Add the load test settings"

i=1
while [ "$i" -le "$sessions" ]; do
	me=$(key "$i")
	next=$(key $((i % sessions + 1)))
	settings=$(mcp_call role_settings_write "$(jq -cn --arg k "$me" '{role_key: $k}')" | jq -r '.path // empty')
	[ -n "$settings" ] || settings="<data>/roles/$me.json"
	body="You are session s$i of a bruh load test, run $run_id. Your role key is $me. The next session in the ring is $next.
1. Now, create one recurring task with the CronCreate tool, with the cron expression */$interval * * * * and this prompt: Load tick. Call the bruh MCP tool mail_post with to $next, header \"P2 Q-$i: load tick from s$i\", and body \"tick\". Then send one SendMessage to the session named $next with the text \"P2 Q-$i: load tick from s$i\" followed by a space and the id that mail_post returned.
2. Each time a message from another session arrives, call mail_read once. Do not answer the message.
3. Do no other work."
	mcp_call mail_post "$(jq -cn --arg to "$me" --arg b "$body" '{to: $to, header: "P2 Q-0: load test start message", body: $b}')" >/dev/null ||
		die "setup: mail_post to $me failed"
	run_in "$work" env CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --name "$me" --permission-mode auto \
		--settings "$settings" --plugin-dir "$plugin" --model "$model" "Read your start message with mail_read." >/dev/null
	i=$((i + 1))
done

# --- monitor ----------------------------------------------------------------

# Each minute: resume a session that has no process and has unread mail older
# than 2 minutes, and record whether the resume kept the session ID.
if [ "$DRY" = 1 ]; then
	printf '+ monitor each minute for %s minutes: resume with claude --resume <sessionId> --bg "Read your mailbox with mail_read." a session with no pid and unread mail older than 2 minutes\n' "$minutes" >&2
else
	: >"$evidence/resumes.tsv"
	end=$(($(date -u +%s) + minutes * 60))
	while [ "$(date -u +%s)" -lt "$end" ]; do
		sleep 60
		agents_json "$ag"
		for me in $(keys); do
			entry=$(jq -c --arg n "$me" --arg p "$base_real" \
				'[.[] | select(.name == $n and ((.cwd // "") as $c | $c == $p or ($c | startswith($p + "/"))))] | sort_by(.startedAt) | last // empty' "$ag")
			[ -n "$entry" ] || continue
			[ "$(printf '%s' "$entry" | jq -r '.pid // empty')" = "" ] || continue
			[ "$(count_missed "$data/mail/$me" "$(utc_ago 120)")" -gt 0 ] || continue
			sid=$(printf '%s' "$entry" | jq -r '.sessionId // empty')
			[ -n "$sid" ] || continue
			(cd "$work" && CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --resume "$sid" --bg "Read your mailbox with mail_read." >/dev/null 2>&1)
			sleep 20
			agents_json "$ag"
			new=$(jq -r --arg n "$me" '[.[] | select(.name == $n and .pid != null)] | last | .sessionId // empty' "$ag")
			printf '%s\t%s\t%s\t%s\n' "$(utc_now)" "$me" "$sid" "$new" >>"$evidence/resumes.tsv"
		done
	done
fi

# --- result -----------------------------------------------------------------

if [ "$DRY" = 1 ]; then
	result DRY load
	exit 0
fi
cutoff=$(utc_ago "$grace_seconds")
total_missed=0
problems=
for me in $(keys); do
	all=$(count_all "$data/mail/$me")
	read_n=$(cat "$data/mail/$me"/read/*.json 2>/dev/null | jq -s 'length')
	missed=$(count_missed "$data/mail/$me" "$cutoff")
	resumes=$(awk -F '\t' -v k="$me" '$2 == k' "$evidence/resumes.tsv" | wc -l | tr -d ' ')
	changed=$(awk -F '\t' -v k="$me" '$2 == k && $3 != $4' "$evidence/resumes.tsv" | wc -l | tr -d ' ')
	echo "$me: messages $all, read $read_n, missed $missed, resumes $resumes, resumes with a new session ID $changed" | tee -a "$RESULTS"
	total_missed=$((total_missed + missed))
	[ "$all" -ge 2 ] || problems="$problems $me:no-traffic"
	[ "$changed" -eq 0 ] || problems="$problems $me:new-session-id"
done
if [ "$total_missed" -eq 0 ] && [ -z "$problems" ]; then
	result PASS load "no missed message, each resume kept its session ID"
else
	fails=$((fails + 1))
	result FAIL load "missed $total_missed;$problems"
fi
