#!/bin/sh
# Smoke test of bruh (spec section 20). See tests/smoke/README.md.
#
# Starts a real bigm in a scratch ledger. bigm starts one clanker, and the
# clanker starts two clerks. The driver checks each signal with a timeout,
# prints one PASS, FAIL, or SKIP line for each step, and stops every
# background session that it started.
#
# Usage: BRUH_TRUSTED_REPO=<repo> sh tests/smoke/run.sh [--dry-run]
set -u
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../.." && pwd)
# shellcheck source=tests/lib.sh
. "$here/../lib.sh"

usage() { echo "usage: BRUH_TRUSTED_REPO=<repo> sh tests/smoke/run.sh [--dry-run]"; }
for a in "$@"; do
	case $a in
	--dry-run) DRY=1 ;;
	-h | --help)
		usage
		exit 0
		;;
	*)
		usage >&2
		exit 2
		;;
	esac
done

for v in "${SMOKE_STEP_MINUTES:-30}" "${SMOKE_SWEEP_MINUTES:-15}" "${SMOKE_IDLE_MINUTES:-0}"; do
	case $v in '' | *[!0-9]*) die "SMOKE_*_MINUTES values must be whole numbers" ;; esac
done
step_min=${SMOKE_STEP_MINUTES:-30}
sweep_min=${SMOKE_SWEEP_MINUTES:-15}
idle_min=${SMOKE_IDLE_MINUTES:-0}

# BRUH_TEST_PLUGIN lets tests/test.sh point the driver at a fixture plugin.
plugin=${BRUH_TEST_PLUGIN:-$root/plugins/bruh}
run_id=${SMOKE_RUN:-$(random_id)}
valid_run_id "$run_id" || die "SMOKE_RUN must be 1 to 12 lowercase letters and digits"
project=smoke-$run_id
clanker=clanker-$project
greet=clerk-$project-greet
gate=clerk-$project-gate
trusted=${BRUH_TRUSTED_REPO:-}
# A relative path would resolve against the repository in `git -C`, and
# session_launch refuses a relative folder.
if [ -n "$trusted" ] && t=$(abs_dir "$trusted"); then trusted=$t; fi
if [ "$DRY" = 1 ] && [ -z "$trusted" ]; then trusted='<trusted repo>'; fi
base=$trusted/.bruh-test/$project
proj=$base/project
ledger=$base/ledger
evidence=${SMOKE_EVIDENCE:-${TMPDIR:-/tmp}/bruh-smoke-$run_id}
ag=$evidence/agents.json
base_real=$base

# The stand-in caller of the MCP server. The driver acts as bigm only.
BRUH_DATA=$(data_dir)
BRUH_PLUGIN_ROOT=$plugin
BRUH_ROLE_KEY=bigm
BRUH_TEST_MCP=$evidence/bruh-mcp
data=$BRUH_DATA

fails=0
started=0
detail=
remote_run=
remote_dispatch=
remote_worktree=
remote_reply_id=

mark() {
	v=$(printf '%s' "$1" | tr - _)
	eval "ok_$v=1"
}
ok() {
	v=$(printf '%s' "$1" | tr - _)
	eval "[ \"\${ok_$v:-0}\" = 1 ]"
}
fail() {
	fails=$((fails + 1))
	result FAIL "$@"
}
# need prints a SKIP line for step $1 and returns 1 when a step it needs failed.
need() {
	name=$1
	shift
	for s in "$@"; do
		if ! ok "$s"; then
			result SKIP "$name" "needs $s"
			return 1
		fi
	done
}
# verify waits up to $2 minutes for the check command and prints the result of step $1.
verify() {
	name=$1
	min=$2
	shift 2
	detail=
	if [ "$DRY" = 1 ]; then
		printf '+ check (up to %s minutes): %s\n' "$min" "$*" >&2
		mark "$name"
		result DRY "$name"
		return 0
	fi
	if wait_for "$min" "$@"; then
		mark "$name"
		result PASS "$name" "$detail"
	else
		fail "$name" "${detail:-no signal within $min minutes}"
		return 1
	fi
}

# write_file writes standard input to the file $1, or only names it in dry-run mode.
write_file() {
	if [ "$DRY" = 1 ]; then
		printf '+ write %s\n' "$1" >&2
		cat >/dev/null
	else
		cat >"$1"
	fi
}

# ev prints the path of the evidence file $1, or /dev/null in dry-run mode.
ev() { if [ "$DRY" = 1 ]; then echo /dev/null; else echo "$evidence/$1"; fi; }

refresh() { agents_json "$(ev agents.json)"; }
# sess prints the jq expression $2 of the session named $1 in the run folder.
sess() {
	[ "$DRY" = 1 ] && return 0
	jq -r --arg n "$1" --arg p "$base_real" \
		--arg w "$trusted_real/.claude/worktrees/" --arg k "clerk-$project-" \
		"[.[] | select(.name == \$n and ((.cwd // \"\") as \$c | \$c == \$p or (\$c | startswith(\$p + \"/\")) or ((\$n | startswith(\$k)) and (\$c | startswith(\$w)))))][0] | $2 // empty" "$ag"
}
has_live() {
	refresh
	[ -n "$(sess "$1" .pid)" ]
}
messages() { cat "$1"/*.json "$1"/read/*.json 2>/dev/null; }

# --- checks -----------------------------------------------------------------

deliver_seen() {
	id=$(sess "$greet" .id)
	[ -n "$id" ] && claude logs "$id" 2>/dev/null | grep -qF 'bruh:deliver'
}
p1_opened() {
	for f in "$data"/questions/Q-*.json; do
		[ -f "$f" ] || continue
		if jq -e --arg k "$greet" '.priority == "P1" and ([.. | strings] | index($k))' "$f" >/dev/null 2>&1; then
			q_id=$(basename "$f" .json)
			detail="question $q_id"
			return 0
		fi
	done
	return 1
}
p1_at_bigm() {
	messages "$data/mail/bigm" | jq -se --arg h "P1 $q_id:" 'any(.[]; .header | startswith($h))' >/dev/null
}
answer_same_run() {
	[ -f "$data/answers/$greet/$q_id.answer" ] || return 1
	messages "$data/mail/$clanker" | jq -se --arg f "$greet" 'any(.[]; .from == $f and (.header | startswith("DONE:")))' >/dev/null || return 1
	id=$(sess "$greet" .id)
	if claude logs "$id" 2>/dev/null | grep -qF resumeFromRunId; then
		detail="the answer came through a relaunch with resumeFromRunId, not in the same run"
		return 1
	fi
	# The greeting word of the task branch or a clerk worktree must be in the answer.
	answer=$(jq -r '.text // empty' "$data/answers/$greet/$q_id.answer" | tr '[:upper:]' '[:lower:]')
	word=
	for b in $(git -C "$trusted" for-each-ref --format='%(refname:short)' refs/heads | grep -vxF -f "$evidence/branches-before.txt"); do
		word=$(git -C "$trusted" show "refs/heads/$b:GREETING.txt" 2>/dev/null | head -1 | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
		[ -z "$word" ] || break
	done
	if [ -z "$word" ]; then
		f=$(find "$proj" -name GREETING.txt -type f 2>/dev/null | head -1)
		[ -z "$f" ] || word=$(head -1 "$f" | tr -d '[:space:]' | tr '[:upper:]' '[:lower:]')
	fi
	if [ -z "$word" ]; then
		detail="no GREETING.txt on a new branch or in the project"
		return 1
	fi
	case $answer in
	*"$word"*) detail="answer file, DONE, and GREETING.txt with the answered word '$word'" ;;
	*)
		detail="GREETING.txt has '$word', which is not in the answer"
		return 1
		;;
	esac
}
prompt_waiting() {
	refresh
	[ "$(sess "$gate" .waitingFor)" = "permission prompt" ]
}
p0_raised() {
	needle="claude attach $gate_id"
	for f in "$data"/questions/Q-*.json; do
		[ -f "$f" ] || continue
		if jq -e --arg s "$needle" '.priority == "P0" and ([.. | strings] | any(contains($s)))' "$f" >/dev/null 2>&1; then
			detail="P0 question $(basename "$f" .json)"
			return 0
		fi
	done
	if messages "$data/mail/bigm" | jq -se --arg s "$needle" 'any(.[]; (.header | startswith("P0 ")) and ((.header + " " + .body) | contains($s)))' >/dev/null 2>&1; then
		detail="P0 message to bigm"
		return 0
	fi
	if grep -rlF --exclude-dir=.git "$needle" "$ledger" >/dev/null 2>&1; then
		detail="ledger file"
		return 0
	fi
	bigm_id=$(sess bigm .id)
	if [ -n "$bigm_id" ] && claude logs "$bigm_id" 2>/dev/null | grep -F "$needle" | grep -q P0; then
		detail="bigm output"
		return 0
	fi
	return 1
}
both_clerks() {
	has_live "$greet" || return 1
	[ -n "$(sess "$gate" .pid)" ] || return 1
	detail="$greet and $gate"
}
clanker_gone() { ! has_live "$clanker"; }
# clanker_resumed passes when the real smoke bigm, not the driver, sent the
# idle check message and brought the stopped clanker back.
clanker_resumed() {
	refresh
	[ -n "$(sess "$clanker" .pid)" ] || return 1
	now_sid=$(sess "$clanker" .sessionId)
	if [ "$now_sid" != "$clanker_sid" ]; then
		detail="new session ID $now_sid, expected $clanker_sid"
		return 1
	fi
	cat "$data/mail/$clanker"/read/*.json 2>/dev/null |
		jq -se --arg h "DONE: smoke idle check $idle_nonce" 'any(.[]; .from == "bigm" and .header == $h)' >/dev/null || return 1
	bigm_log=$(claude logs "$(sess bigm .id)" 2>/dev/null)
	how=
	case $bigm_log in *session_resume*) how=session_resume ;; esac
	if [ -z "$how" ] && [ "$idle_min" -eq 0 ]; then
		case $bigm_log in *respawn*) how=respawn ;; esac
	fi
	if [ -z "$how" ]; then
		detail="the clanker is back, but the output of bigm shows no session_resume"
		return 1
	fi
	detail="bigm brought the clanker back with $how; same session ID; message read"
}
# remote_messages prints the Orca inbox messages of the remote run as JSON lines.
remote_messages() {
	orca orchestration inbox --limit 200 --json 2>/dev/null |
		jq -c --arg r "$remote_run" '.result.messages[]? | select($r == "" or .run_id == $r)'
}
# remote_asked answers the P1 of the remote clanker with a fresh nonce once, and
# passes when a message carries the rot13 form of that nonce. Only a session
# that got the reply and ran the command of its start message can make it.
remote_asked() {
	if [ -z "$remote_reply_id" ]; then
		remote_reply_id=$(remote_messages | jq -r --arg q "$remote_q" \
			'select(((.subject // "") + " " + (.body // "")) | contains($q)) | select((.body // "") | contains("In this worktree") | not) | .id' | head -1)
		[ -n "$remote_reply_id" ] || return 1
		orca orchestration reply --id "$remote_reply_id" --body "$remote_nonce" >/dev/null 2>&1 || {
			remote_reply_id=
			return 1
		}
	fi
	remote_messages | jq -e --arg n "$remote_ack" 'select((.body // "") | contains($n))' >/dev/null || return 1
	detail="round trip through Orca; the rot13 nonce came back"
}

# --- cleanup ----------------------------------------------------------------

cleanup() {
	trap - EXIT HUP INT TERM
	echo "cleanup"
	if [ "$DRY" = 1 ]; then
		show claude stop '<each session in the run folder or in the clerk worktrees of the trusted repository>'
		show git -C "$trusted" worktree remove --force --force '<each clerk worktree whose branch this run created>'
		show rm -rf "$base"
		show git -C "$trusted" worktree prune
		show git -C "$trusted" branch -D '<each branch that this run created>'
		show rm -rf "$data/mail/bigm" "$data/roles/bigm.json" '<the other data files of this run>'
		if [ -n "$remote_worktree" ]; then
			show orca worktree rm --environment "$SMOKE_ORCA_ENV" --worktree "name:$remote_worktree" --force
		fi
		return 0
	fi
	[ "$started" = 1 ] || return 0
	# Every session in the run folder belongs to the run: bigm, clerk-ledger, the
	# clanker, the task clerks, a merger clerk, and the -p sessions of the driver.
	if ! refresh; then
		fail cleanup "claude agents --json --all failed; stop the sessions of this run by hand"
	fi
	ids="$(own_sessions "$ag" "$base_real" "") $(own_sessions "$ag" "$trusted_real/.claude/worktrees" "clerk-$project-" "$clanker")"
	# shellcheck disable=SC2086 # ids is a list of short session IDs
	keys=$(jq -r --args '.[] | select(.id as $i | $ARGS.positional | index($i)) | .name // empty' $ids <"$ag" 2>/dev/null)
	for id in $ids; do
		name=$(jq -r --arg i "$id" '.[] | select(.id == $i) | .name' "$ag")
		claude logs "$id" >"$evidence/logs-$name.txt" 2>&1 || true
	done
	# shellcheck disable=SC2086 # ids is a list of short session IDs
	if ! stop_sessions $ids; then
		fail cleanup "a session of this run still has a process: $ids"
	fi
	if [ -n "$remote_dispatch" ]; then
		orca orchestration worker-stop --dispatch "$remote_dispatch" >/dev/null 2>&1 || true
	fi
	if [ -n "$remote_worktree" ]; then
		orca worktree rm --environment "$SMOKE_ORCA_ENV" --worktree "name:$remote_worktree" --force >/dev/null 2>&1 ||
			fail cleanup "remove the remote Orca worktree $remote_worktree by hand"
	fi
	# The clerk worktrees under .claude/worktrees/ of the trusted repository whose
	# branch this run created.
	git -C "$trusted" worktree list --porcelain | awk '/^worktree /{w=substr($0,10)} /^branch refs\/heads\//{print w "\t" substr($0,19)}' |
		while IFS='	' read -r wt br; do
			case $wt in "$trusted_real"/.claude/worktrees/*) ;; *) continue ;; esac
			grep -qxF "$br" "$evidence/branches-before.txt" 2>/dev/null && continue
			git -C "$trusted" worktree remove --force --force "$wt" >/dev/null 2>&1 || fail cleanup "remove the clerk worktree $wt by hand"
		done
	rmdir "$trusted_real/.claude/worktrees" "$trusted_real/.claude" 2>/dev/null || true
	if ! remove_scratch "$trusted" "$base" "$evidence" "$project-"; then
		fail cleanup "no list of the branches before the run; no branch deleted"
	fi
	# shellcheck disable=SC2086 # keys is a list of role keys
	remove_role_data "$data" bigm clerk-ledger "$clanker" "$greet" "$gate" $keys
	grep -lF "$project" "$data"/questions/Q-*.json 2>/dev/null | while read -r f; do rm -f "$f"; done
	echo "evidence: $evidence"
	if [ "$fails" -gt 0 ]; then exit 1; fi
}

# --- preflight --------------------------------------------------------------

echo "bruh smoke test, run $run_id"
if [ "$DRY" = 1 ]; then
	result DRY preflight
else
	reason=$(preflight_common) || die "preflight: $reason"
	for f in agents/bigm.md agents/clanker.md agents/clerk.md workflows/deliver.js ledger-template/mode.md; do
		[ -f "$plugin/$f" ] || die "preflight: $plugin/$f does not exist"
	done
	mkdir -p "$evidence"
	RESULTS=$evidence/results.txt
	: >"$RESULTS"
	refresh || die "preflight: claude agents --json --all failed"
	# bigm and clerk-ledger have fixed role keys. A nudge goes to a session name,
	# so a live session with a name of this run would get the messages of the test.
	live=$(live_conflicts "$ag" bigm clerk-ledger "$clanker" "clerk-$project-" | words)
	[ -z "$live" ] || die "preflight: live sessions have names that this run uses: $live. Stop them first."
	for p in mail/bigm roles/bigm.json handoffs/bigm.md reports/bigm.jsonl answers/bigm \
		mail/clerk-ledger roles/clerk-ledger.json handoffs/clerk-ledger.md reports/clerk-ledger.jsonl; do
		[ ! -e "$data/$p" ] || die "preflight: $data/$p exists. It is the state of another run. Move it away first."
	done
	result PASS preflight "Claude Code $(claude --version | awk '{print $1}')"
fi
trap cleanup EXIT
trap 'exit 129' HUP
trap 'exit 130' INT
trap 'exit 143' TERM

# --- setup ------------------------------------------------------------------

run mkdir -p "$base"
[ "$DRY" = 1 ] || record_branches "$trusted" "$evidence" || die "setup: cannot read the branches of $trusted"
started=1
build_mcp "$plugin" "$BRUH_TEST_MCP"
add_scratch_worktree "$trusted" "$proj" "bruh-$project"
add_scratch_worktree "$trusted" "$ledger" "bruh-$project-ledger"
if [ "$DRY" = 1 ]; then base_real=$base; else base_real=$(cd "$base" && pwd -P); fi
# EnterWorktree of a clerk puts its worktree under .claude/worktrees/ of the main
# repository, which is the trusted repository, not the run folder.
if [ "$DRY" = 1 ]; then trusted_real=$trusted; else trusted_real=$(cd "$trusted" && pwd -P); fi

write_file "$proj/TASK.md" <<'EOF'
# Smoke test tasks

This project is a smoke test of bruh. Divide the work into the two tasks below. Start one clerk for each task, with the task IDs `greet` and `gate`. Each clerk runs `/bruh:deliver` for its task.

There is no remote. Deliver each task as a local branch. Do not push.

## Task greet

Create the file `GREETING.txt` with one line: a greeting word.

The greeting word is an open decision of the owner. Before the workflow writes the file, it asks this P1 question through the question path of the `deliver` workflow: "Which greeting word goes into GREETING.txt: hello or hi?" It uses the answer.

- Gate command: `test -s GREETING.txt`
- Acceptance: `GREETING.txt` exists and contains the answered word.

## Task gate

Create the file `GATE.txt` with the line `ok`.

- Gate command: `./gate.sh`
- Acceptance: `./gate.sh` exits 0.
EOF
write_file "$proj/gate.sh" <<'EOF'
#!/bin/sh
test "$(cat GATE.txt 2>/dev/null)" = ok
EOF
run chmod +x "$proj/gate.sh"
run mkdir -p "$proj/.claude" "$ledger/.claude"
# The ask rules force a permission prompt for the gate command, also in auto mode.
write_file "$proj/.claude/settings.json" <<'EOF'
{
  "permissions": {
    "allow": ["Workflow(bruh:deliver)", "mcp__plugin_bruh_bruh"],
    "ask": ["Bash(./gate.sh)", "Bash(./gate.sh:*)", "Bash(sh gate.sh:*)", "Bash(sh ./gate.sh:*)", "Bash(bash gate.sh:*)", "Bash(bash ./gate.sh:*)"],
    "deny": ["Bash(git push:*)"]
  }
}
EOF
[ "$DRY" = 1 ] || add_push_block "$proj/.claude/settings.json" "$BRUH_TRUSTED_REPO" || die "setup: cannot block pushes in the project settings"
commit_all "$proj" "Add the smoke test tasks"

run cp -R "$plugin/ledger-template/." "$ledger/"
# init replaces the placeholder priorities.md of the template with the default list.
run cp "$plugin/defaults/priorities.md" "$ledger/priorities.md"
if [ "$DRY" = 1 ]; then
	show sed 's/^mode: .*/mode: autonomous/' "$ledger/mode.md"
else
	sed 's/^mode: .*/mode: autonomous/' "$ledger/mode.md" >"$ledger/mode.md.new" && mv "$ledger/mode.md.new" "$ledger/mode.md"
	grep -q '^mode: autonomous$' "$ledger/mode.md" || die "setup: mode.md of the ledger template has no 'mode:' line"
fi
# The agent key and the role key are start settings of bigm that /bruh:init
# writes into the ledger (spec 3.4).
write_file "$ledger/.claude/settings.json" <<'EOF'
{
  "agent": "bruh:bigm",
  "env": {"BRUH_ROLE_KEY": "bigm"},
  "permissions": {
    "allow": ["mcp__plugin_bruh_bruh", "SendMessage"],
    "deny": ["Bash(git push:*)"]
  }
}
EOF
[ "$DRY" = 1 ] || add_push_block "$ledger/.claude/settings.json" "$BRUH_TRUSTED_REPO" || die "setup: cannot block pushes in the ledger settings"
commit_all "$ledger" "Create the smoke test ledger"

# --- steps ------------------------------------------------------------------

# A plain claude in the ledger starts as bigm through the agent key of the
# ledger settings. With no tool and no MCP server, the probe only answers. The
# step does not probe the role key: when it is missing, each bruh tool call of
# bigm fails with "BRUH_ROLE_KEY is not set".
plain=$(run_in "$ledger" claude -p --strict-mcp-config --tools "" --max-turns 1 --plugin-dir "$plugin" \
	"Do not use any tool and do not follow any start-of-turn procedure. Reply with exactly one line: the role name that your system instructions give you, or NONE if they give you no role name.")
if [ "$DRY" = 1 ]; then
	result DRY plain-start
elif [ "$plain" = bigm ]; then
	result PASS plain-start "a plain claude in the ledger answered bigm"
else
	fail plain-start "a plain claude in the ledger answered '$(printf '%s' "$plain" | head -1)', not bigm"
fi

bigm_settings=$(mcp_call role_settings_write '{"role_key":"bigm"}' | jq -r '.path // empty')
if [ "$DRY" = 1 ]; then
	bigm_settings='<data>/roles/bigm.json'
elif [ -z "$bigm_settings" ]; then
	die "setup: role_settings_write for bigm failed"
fi
idle_nonce=idle$(random_id)
prompt="bruh smoke test, run $run_id. The ledger in this folder is in autonomous mode. Start one clanker with the role key $clanker for the project $project in the folder $proj. The work of the project is in TASK.md in that folder. Later in this test, a session sends you a message that starts with SMOKE-IDLE. When it arrives, send the clanker a message with the header 'DONE: smoke idle check <the word after SMOKE-IDLE>' and the body 'No answer is necessary.'. Follow your procedure for messages to a local session."
run_in "$ledger" env CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --agent bruh:bigm --name bigm \
	--permission-mode auto --settings "$bigm_settings" --plugin-dir "$plugin" "$prompt" >/dev/null
verify bigm 2 has_live bigm

need clanker bigm && verify clanker "$step_min" has_live "$clanker"
need clerk clanker && verify clerk "$step_min" both_clerks
need deliver clerk && verify deliver "$step_min" deliver_seen
q_id='Q-<id>'
need p1-sent deliver && verify p1-sent "$step_min" p1_opened
need p1-bigm p1-sent && verify p1-bigm "$step_min" p1_at_bigm
need answer-same-run p1-bigm && verify answer-same-run "$step_min" answer_same_run

gate_id='<gate clerk id>'
if need p0-prompt clerk; then
	if [ "$DRY" = 1 ] || wait_for "$step_min" prompt_waiting; then
		[ "$DRY" = 1 ] || gate_id=$(sess "$gate" .id)
		verify p0-prompt $((sweep_min + 5)) p0_raised
	else
		fail p0-prompt "the gate clerk never waited on a permission prompt"
	fi
fi

idle_resume() {
	refresh
	clanker_sid=$(sess "$clanker" .sessionId)
	clanker_id=$(sess "$clanker" .id)
	if [ "$idle_min" -gt 0 ]; then
		if ! wait_for "$idle_min" clanker_gone; then
			fail idle-resume "the clanker still had a process after $idle_min minutes"
			return 1
		fi
	elif ! stop_sessions "${clanker_id:-<clanker id>}"; then
		fail idle-resume "claude stop did not stop the clanker"
		return 1
	fi
	# A short -p session nudges bigm. bigm, not the driver, sends the message to
	# the clanker and must bring it back by itself. The nudge loads only the user
	# settings, so the agent key of the ledger settings does not make it a bigm.
	# It has no Bash, because it does not get the push block of the ledger settings.
	run_in "$ledger" claude -p --model haiku --permission-mode auto --setting-sources user --allowedTools SendMessage --disallowedTools Bash \
		"Use the SendMessage tool once to send this exact text to the session named bigm, then stop: SMOKE-IDLE $idle_nonce" \
		>"$(ev idle-nudge.txt)" || true
	verify idle-resume "$step_min" clanker_resumed
}
need idle-resume clanker && idle_resume

remote_q="P1 Q-smoke-remote-1: smoke remote $run_id"
remote_nonce=x$(random_id)
remote_ack=$(printf '%s' "$remote_nonce" | tr 'abcdefghijklmnopqrstuvwxyz' 'nopqrstuvwxyzabcdefghijklm')
if [ -z "${SMOKE_ORCA_ENV:-}" ] || [ -z "${SMOKE_ORCA_REPO:-}" ]; then
	result SKIP remote "SMOKE_ORCA_ENV or SMOKE_ORCA_REPO is not set, so no Orca environment is paired for this run"
elif [ "$DRY" != 1 ] && ! orca environment show --environment "$SMOKE_ORCA_ENV" >/dev/null 2>&1; then
	result SKIP remote "the Orca environment $SMOKE_ORCA_ENV is not paired"
elif ! run orca orchestration run-create --objective "bruh smoke test $run_id" --json >"$(ev orca-run.json)"; then
	fail remote "orca orchestration run-create failed; run the driver in an Orca terminal"
else
	remote_run=$(jq -r '[.. | strings | select(test("^run_[0-9a-f]+$"))] | first // empty' "$(ev orca-run.json)" 2>/dev/null)
	task="1. Run: orca orchestration ask --question '$remote_q' --timeout-ms 600000. The answer is one word. 2. Run: printf '%s' '<the answer word>' | tr 'abcdefghijklmnopqrstuvwxyz' 'nopqrstuvwxyzabcdefghijklm'. 3. Run: orca orchestration send --subject 'DONE: smoke remote $run_id' --body '<the output of step 2>'. Then stop."
	spec="bruh smoke test, run $run_id. In this worktree, start a bruh clanker with this command and wait until it ends: claude -p --agent bruh:clanker --permission-mode auto \"$task\""
	remote_worktree=bruh-smoke-$run_id
	run orca orchestration worker-start --on "$SMOKE_ORCA_ENV" --worktree new-top-level --name "$remote_worktree" \
		--repo "$SMOKE_ORCA_REPO" --setup skip --agent claude --spec "$spec" --json >"$(ev orca-worker.json)" || true
	remote_dispatch=$(jq -r '[.. | strings | select(test("^ctx_[0-9a-f]+$"))] | first // empty' "$(ev orca-worker.json)" 2>/dev/null)
	verify remote "$step_min" remote_asked
fi

if [ "$fails" -gt 0 ]; then exit 1; fi
