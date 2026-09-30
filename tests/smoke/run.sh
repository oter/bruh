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

plugin=$root/plugins/bruh
run_id=${SMOKE_RUN:-$(random_id)}
project=smoke-$run_id
clanker=clanker-$project
greet=clerk-$project-greet
gate=clerk-$project-gate
trusted=${BRUH_TRUSTED_REPO:-}
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
		"[.[] | select(.name == \$n and ((.cwd // \"\") as \$c | \$c == \$p or (\$c | startswith(\$p + \"/\"))))][0] | $2 // empty" "$ag"
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
	detail="answer file and DONE present"
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
clanker_resumed() {
	refresh
	[ -n "$(sess "$clanker" .pid)" ] || return 1
	now_sid=$(sess "$clanker" .sessionId)
	if [ "$now_sid" != "$clanker_sid" ]; then
		detail="new session ID $now_sid, expected $clanker_sid"
		return 1
	fi
	[ -f "$data/mail/$clanker/read/$resume_msg.json" ] || return 1
	detail="same session ID, message read"
}
remote_asked() {
	orca orchestration inbox --json 2>/dev/null | grep -qF "$remote_q"
}

# --- cleanup ----------------------------------------------------------------

cleanup() {
	trap - EXIT INT TERM
	echo "cleanup"
	if [ "$DRY" = 1 ]; then
		show claude stop '<each session of this run>'
		show rm -rf "$base"
		show git -C "$trusted" worktree prune
		show git -C "$trusted" branch -D '<each branch that this run created>'
		show rm -rf "$data/mail/bigm" "$data/roles/bigm.json" '<the other data files of this run>'
		return 0
	fi
	[ "$started" = 1 ] || return 0
	refresh
	ids=$(own_sessions "$ag" "$base_real" bigm "$clanker" "clerk-$project-")
	for id in $ids; do
		name=$(jq -r --arg i "$id" '.[] | select(.id == $i) | .name' "$ag")
		claude logs "$id" >"$evidence/logs-$name.txt" 2>&1 || true
	done
	# shellcheck disable=SC2086 # ids is a list of short session IDs
	if ! stop_sessions $ids; then
		fail cleanup "a session of this run still has a process: $ids"
	fi
	if [ -n "${SMOKE_ORCA_DISPATCH:-}" ]; then
		orca orchestration worker-stop --dispatch "$SMOKE_ORCA_DISPATCH" >/dev/null 2>&1 || true
	fi
	rm -rf "$base"
	git -C "$trusted" worktree prune
	git -C "$trusted" for-each-ref --format='%(refname:short)' refs/heads >"$evidence/branches-after.txt"
	# Delete only the branches that this run created. An empty list of the
	# branches before the run would select every branch, so it stops here.
	if [ -s "$evidence/branches-before.txt" ]; then
		grep -vxF -f "$evidence/branches-before.txt" "$evidence/branches-after.txt" | while read -r b; do
			git -C "$trusted" branch -D -q "$b" || true
		done
	else
		fail cleanup "no list of the branches before the run; no branch deleted"
	fi
	for key in bigm "$clanker" "$greet" "$gate"; do
		rm -rf "$data/mail/$key" "$data/answers/$key" "$data/roles/$key.json" \
			"$data/handoffs/$key.md" "$data/handoffs/$key.history.md" "$data/reports/$key.jsonl"
	done
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
	refresh
	live=$(live_named "$ag" bigm)
	[ -z "$live" ] || die "preflight: a live session named bigm runs ($live). Stop it first, because the nudges of this test go to the name bigm."
	for p in mail/bigm roles/bigm.json handoffs/bigm.md reports/bigm.jsonl answers/bigm; do
		[ ! -e "$data/$p" ] || die "preflight: $data/$p exists. It is bigm state of another run. Move it away first."
	done
	result PASS preflight "Claude Code $(claude --version | awk '{print $1}')"
fi
trap cleanup EXIT
trap 'exit 130' INT TERM

# --- setup ------------------------------------------------------------------

run mkdir -p "$base"
[ "$DRY" = 1 ] || git -C "$trusted" for-each-ref --format='%(refname:short)' refs/heads >"$evidence/branches-before.txt"
started=1
build_mcp "$plugin" "$BRUH_TEST_MCP"
add_scratch_worktree "$trusted" "$proj" "bruh-$project"
add_scratch_worktree "$trusted" "$ledger" "bruh-$project-ledger"
if [ "$DRY" = 1 ]; then base_real=$base; else base_real=$(cd "$base" && pwd -P); fi

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
commit_all "$proj" "Add the smoke test tasks"

run cp -R "$plugin/ledger-template/." "$ledger/"
if [ "$DRY" = 1 ]; then
	show sed 's/^mode: .*/mode: autonomous/' "$ledger/mode.md"
else
	sed 's/^mode: .*/mode: autonomous/' "$ledger/mode.md" >"$ledger/mode.md.new" && mv "$ledger/mode.md.new" "$ledger/mode.md"
	grep -q '^mode: autonomous$' "$ledger/mode.md" || die "setup: mode.md of the ledger template has no 'mode:' line"
fi
write_file "$ledger/.claude/settings.json" <<'EOF'
{
  "permissions": {
    "allow": ["mcp__plugin_bruh_bruh"],
    "deny": ["Bash(git push:*)"]
  }
}
EOF
commit_all "$ledger" "Create the smoke test ledger"

# --- steps ------------------------------------------------------------------

bigm_settings=$(mcp_call role_settings_write '{"role_key":"bigm"}' | jq -r '.path // empty')
[ -n "$bigm_settings" ] || bigm_settings='<data>/roles/bigm.json'
prompt="bruh smoke test, run $run_id. The ledger in this folder is in autonomous mode. Start one clanker with the role key $clanker for the project $project in the folder $proj. The work of the project is in TASK.md in that folder."
run_in "$ledger" env CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --agent bruh:bigm --name bigm \
	--permission-mode auto --settings "$bigm_settings" --plugin-dir "$plugin" "$prompt" >/dev/null
verify bigm 2 has_live bigm

need clanker bigm && verify clanker "$step_min" has_live "$clanker"
need clerk clanker && verify clerk "$step_min" both_clerks
need deliver clerk && verify deliver "$step_min" deliver_seen
q_id='Q-<n>'
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
	elif [ -n "$clanker_id" ] || [ "$DRY" = 1 ]; then
		stop_sessions "${clanker_id:-<clanker id>}" || true
	fi
	resume_msg=$(mcp_call mail_post "$(jq -cn --arg to "$clanker" --arg r "$run_id" \
		'{to: $to, header: ("P2 Q-999999: smoke idle resume check " + $r), body: "No answer is necessary. This message checks that a stopped clanker resumes."}')" |
		jq -r '.id // empty')
	if ! (if [ "$DRY" != 1 ]; then cd "$ledger" || exit 1; fi
		mcp_call session_resume "$(jq -cn --arg k "$clanker" '{role_key: $k, prompt: "Read your mailbox with mail_read."}')" >/dev/null); then
		fail idle-resume "session_resume failed"
		return 1
	fi
	verify idle-resume "$step_min" clanker_resumed
}
need idle-resume clanker && idle_resume

remote_q="P1 Q-1: smoke remote $run_id"
if [ -z "${SMOKE_ORCA_ENV:-}" ] || [ -z "${SMOKE_ORCA_REPO:-}" ]; then
	result SKIP remote "SMOKE_ORCA_ENV or SMOKE_ORCA_REPO is not set, so no Orca environment is paired for this run"
elif [ "$DRY" != 1 ] && ! orca environment show --environment "$SMOKE_ORCA_ENV" >/dev/null 2>&1; then
	result SKIP remote "the Orca environment $SMOKE_ORCA_ENV is not paired"
elif ! run orca orchestration run-create --objective "bruh smoke test $run_id" --json >"$(ev orca-run.json)"; then
	fail remote "orca orchestration run-create failed; run the driver in an Orca terminal"
else
	spec="bruh smoke test, run $run_id. In this worktree, run this command and wait until it ends: claude -p --agent bruh:clanker --permission-mode auto \"Run this command and print its output: orca orchestration ask --question '$remote_q' --timeout-ms 300000\""
	run orca orchestration worker-start --on "$SMOKE_ORCA_ENV" --worktree new-top-level --repo "$SMOKE_ORCA_REPO" \
		--agent claude --spec "$spec" --json >"$(ev orca-worker.json)" || true
	SMOKE_ORCA_DISPATCH=$(jq -r '[.. | objects | (.dispatchId? // .dispatch_id? // empty)] | first // empty' "$(ev orca-worker.json)" 2>/dev/null)
	verify remote "$step_min" remote_asked
fi

if [ "$fails" -gt 0 ]; then exit 1; fi
