#!/bin/sh
# Tests for tests/lib.sh and for the dry runs of the smoke and load drivers.
# No Claude session is started. Run: sh tests/test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=tests/lib.sh
. "$here/lib.sh"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
n=0
fails=0

check() {
	n=$((n + 1))
	desc=$1
	shift
	if "$@" >/dev/null 2>&1; then
		echo "ok $n - $desc"
	else
		echo "not ok $n - $desc"
		fails=$((fails + 1))
	fi
}
not() { if "$@"; then return 1; fi; }
eq() { [ "$1" = "$2" ]; }
contains() { case $1 in *"$2"*) return 0 ;; esac; return 1; }

# version_ge
check "version_ge equal" version_ge 2.1.284 2.1.284
check "version_ge older patch" not version_ge 2.1.283 2.1.284
check "version_ge newer minor" version_ge 2.2.0 2.1.284
check "version_ge newer patch with more digits" version_ge 2.1.1000 2.1.284
check "version_ge older major" not version_ge 1.9.999 2.1.284

# live_named and own_sessions
cat >"$tmp/agents.json" <<'JSON'
[
  {"id":"a1","name":"bigm","pid":123,"cwd":"/x/owner","state":"working"},
  {"id":"a2","name":"bigm","cwd":"/x/old","state":"stopped"},
  {"id":"a3","name":"clanker-smoke-abc","pid":5,"cwd":"/s/run/project"},
  {"id":"a4","name":"clanker-smoke-abc","pid":6,"cwd":"/other/project"},
  {"id":"a5","name":"clerk-smoke-abc-greet","pid":7,"cwd":"/s/run/project/sub"},
  {"id":"a6","name":"bigm","pid":8,"cwd":"/s/run-other/ledger"},
  {"id":"a7","kind":"interactive","cwd":"/s/run"}
]
JSON
check "live_named finds only live sessions" eq "$(live_named "$tmp/agents.json" bigm | words)" "a1 a6"
check "own_sessions keeps the run folder and the names" \
	eq "$(own_sessions "$tmp/agents.json" /s/run clanker-smoke-abc clerk-smoke-abc- | words)" "a3 a5"
check "own_sessions finds nothing for another prefix" eq "$(own_sessions "$tmp/agents.json" /nowhere bigm)" ""
check "own_sessions with an empty name prefix takes every session of the folder" \
	eq "$(own_sessions "$tmp/agents.json" /s/run "" | words)" "a3 a5 a7"
cat >"$tmp/agents2.json" <<'JSON'
[
  {"id":"b1","name":"clerk-ledger","pid":11,"cwd":"/owner/ledger"},
  {"id":"b2","name":"clanker-smoke-abc","cwd":"/old"},
  {"id":"b3","name":"other","pid":12,"cwd":"/x"}
]
JSON
check "live_conflicts finds the live clerk-ledger of the owner" \
	eq "$(live_conflicts "$tmp/agents2.json" bigm clerk-ledger clanker-smoke-abc clerk-smoke-abc-)" "clerk-ledger (b1)"
check "live_conflicts skips a stopped session with a run name" \
	eq "$(live_conflicts "$tmp/agents2.json" clanker-smoke-abc)" ""

# valid_run_id and abs_dir
check "valid_run_id accepts letters and digits" valid_run_id abc123
check "valid_run_id refuses a path" not valid_run_id 'a/../..'
check "valid_run_id refuses capitals" not valid_run_id Abc
check "valid_run_id refuses a hyphen" not valid_run_id a-b
check "valid_run_id refuses an empty value" not valid_run_id ''
check "valid_run_id refuses more than 12 characters" not valid_run_id abcdefghijklm
check "smoke refuses a bad SMOKE_RUN" not env SMOKE_RUN='a/../..' sh "$here/smoke/run.sh" --dry-run
check "load refuses a bad LOAD_RUN" not env LOAD_RUN='a/../..' sh "$here/load/run.sh" --dry-run
mkdir -p "$tmp/rel/sub"
check "abs_dir resolves a relative folder" eq "$(cd "$tmp/rel" && abs_dir sub)" "$(cd "$tmp/rel/sub" && pwd -P)"
check "abs_dir fails for a missing folder" not abs_dir "$tmp/nothing-here"

# utc_ago
check "utc_ago has the layout of the MCP server" contains "$(utc_ago 60)" ".000Z"
lt() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort | head -1)" = "$1" ]; }
check "utc_ago is before now" lt "$(utc_ago 3600)" "$(utc_now)"
check "utc_ago is after two hours ago" lt "$(utc_ago 7200)" "$(utc_ago 3600)"

# check_trusted_repo
git init -q "$tmp/repo" && git -C "$tmp/repo" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
git init -q "$tmp/empty"
git init -q "$tmp/remote" && git -C "$tmp/remote" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
git -C "$tmp/remote" remote add origin https://example.com/owner/repo.git
mkdir -p "$tmp/plain"
check "check_trusted_repo accepts a repository with no remote" check_trusted_repo "$tmp/repo"
check "check_trusted_repo refuses an empty value" not check_trusted_repo ""
check "check_trusted_repo refuses a missing folder" not check_trusted_repo "$tmp/missing"
check "check_trusted_repo refuses a folder that is not a repository" not check_trusted_repo "$tmp/plain"
check "check_trusted_repo refuses a repository with no commit" not check_trusted_repo "$tmp/empty"
check "check_trusted_repo refuses a repository with a remote" not check_trusted_repo "$tmp/remote"
# shellcheck disable=SC2016 # the inner shell expands $1 and $2
check "check_trusted_repo accepts a remote with SMOKE_ALLOW_REMOTE=1" env SMOKE_ALLOW_REMOTE=1 sh -c '. "$1" && check_trusted_repo "$2"' _ "$here/lib.sh" "$tmp/remote"
printf '{"permissions":{}}' >"$tmp/push.json"
add_push_block "$tmp/push.json" "$tmp/remote"
# shellcheck disable=SC2016 # the inner shell expands $1 and $2
check "add_push_block sets the push URL of each remote to a missing path" \
	sh -c 'cd "$1" && env $(jq -r ".env | to_entries[] | \"\(.key)=\(.value)\"" "$2") git config remote.origin.pushurl | grep -qx /nonexistent/bruh-smoke-no-push' _ "$tmp/remote" "$tmp/push.json"
check "check_trusted_repo names the remote problem" contains "$(check_trusted_repo "$tmp/remote")" "remote"

# count_missed
mkdir -p "$tmp/box/read"
printf '{"id":"1","header":"P2 Q-1: old","at":"2026-09-30T10:00:00.000Z"}' >"$tmp/box/1.json"
printf '{"id":"2","header":"P2 Q-1: new","at":"2026-09-30T10:59:00.000Z"}' >"$tmp/box/2.json"
printf '{"id":"3","header":"P2 Q-1: read","at":"2026-09-30T09:00:00.000Z"}' >"$tmp/box/read/3.json"
check "count_missed counts an old unread message only" eq "$(count_missed "$tmp/box" 2026-09-30T10:55:00.000Z)" 1
check "count_missed is 0 for a missing mailbox" eq "$(count_missed "$tmp/nobox" 2026-09-30T10:55:00.000Z)" 0
check "count_all counts unread and read" eq "$(count_all "$tmp/box")" 3

# data_dir
check "data_dir uses the inline plugin id" eq "$(unset BRUH_TEST_DATA; CLAUDE_CODE_PLUGIN_CACHE_DIR=/p data_dir)" /p/data/bruh-inline
check "data_dir honors BRUH_TEST_DATA" eq "$(BRUH_TEST_DATA=/d data_dir)" /d

# mcp_call in dry-run mode prints the request and returns an empty object
out=$( (DRY=1 BRUH_ROLE_KEY=bigm mcp_call mail_post '{"to":"x","header":"DONE: y","body":"z"}') 2>&1)
check "mcp_call dry run prints the tools/call request" contains "$out" '"method":"tools/call","params":{"name":"mail_post"'

# Smoke driver dry run
out=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 sh "$here/smoke/run.sh" --dry-run 2>&1)
check "smoke dry run exits 0" eq "$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 sh "$here/smoke/run.sh" --dry-run >/dev/null 2>&1; echo $?)" 0
check "smoke dry run starts bigm with the documented flags" contains "$out" "claude --bg --agent bruh:bigm --name bigm --permission-mode auto --settings"
check "smoke dry run loads the plugin of the checkout" contains "$out" "--plugin-dir $(cd "$here/.." && pwd)/plugins/bruh"
check "smoke dry run writes the bigm role settings" contains "$out" '"name":"role_settings_write","arguments":{"role_key":"bigm"}'
check "smoke dry run stops the clanker to simulate the idle stop" contains "$out" "claude stop '<clanker id>'"
check "smoke dry run lets bigm, not the driver, message the clanker" contains "$out" "session named bigm, then stop: SMOKE-IDLE idle"
check "smoke dry run does not call session_resume itself" not contains "$out" '"name":"session_resume"'
check "smoke dry run traps HUP" grep -q "trap 'exit 129' HUP" "$here/smoke/run.sh"
check "load dry run traps HUP" grep -q "trap 'exit 129' HUP" "$here/load/run.sh"
check "smoke dry run uses a linked worktree" contains "$out" "worktree add -q --detach"
check "smoke dry run skips the remote step" contains "$out" "SKIP remote"
check "smoke dry run stops own sessions in cleanup" contains "$out" "claude stop '<each session in the run folder or in the clerk worktrees of the trusted repository>'"
rout=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 SMOKE_ORCA_ENV=env1 SMOKE_ORCA_REPO=name:r sh "$here/smoke/run.sh" --dry-run 2>&1)
check "smoke remote step starts a worker with no setup hooks" contains "$rout" "--setup skip --agent claude --spec"
check "smoke remote step names the remote worktree" contains "$rout" "--name bruh-smoke-dry1"
check "smoke remote step asks for the rot13 of the reply" contains "$rout" "tr 'abcdefghijklmnopqrstuvwxyz' 'nopqrstuvwxyzabcdefghijklm'"
check "smoke remote step removes the remote worktree in cleanup" contains "$rout" "orca worktree rm --environment env1 --worktree name:bruh-smoke-dry1 --force"
check "smoke refuses an unknown flag" not sh "$here/smoke/run.sh" --bogus

# random_id ends also when SIGPIPE is ignored, as on a CI runner.
# perl ignores SIGPIPE, then execs sh; the alarm stops sh after 10 seconds with status 142.
rid_rc=0
rid=$(perl -e '$SIG{PIPE}="IGNORE"; alarm 10; exec "sh","-c",". \"$ARGV[0]\" && random_id"' "$here/lib.sh") || rid_rc=$?
check "random_id ends with SIGPIPE ignored" eq "$rid_rc" 0
check "random_id prints a run ID" valid_run_id "$rid"
check "random_id has 6 characters" eq "${#rid}" 6
# remove_scratch deletes only the branches that the run created
git init -q "$tmp/rs" && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
mkdir -p "$tmp/rs-ev" "$tmp/rs/.bruh-test/x"
git -C "$tmp/rs" branch keep
record_branches "$tmp/rs" "$tmp/rs-ev"
git -C "$tmp/rs" branch owner-new
git -C "$tmp/rs" checkout -q --orphan new-of-run && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m orphan
git -C "$tmp/rs" checkout -q keep
check "remove_scratch succeeds" remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev"
check "remove_scratch deletes the new orphan branch of the run" not git -C "$tmp/rs" rev-parse --verify -q refs/heads/new-of-run
check "remove_scratch keeps a new branch that another person made from HEAD" git -C "$tmp/rs" rev-parse --verify -q refs/heads/owner-new
check "remove_scratch keeps the old branches" git -C "$tmp/rs" rev-parse --verify -q refs/heads/keep
check "remove_scratch removes the scratch folder" not test -e "$tmp/rs/.bruh-test/x"
mkdir -p "$tmp/rs/.bruh-test/x"
git -C "$tmp/rs" branch worktree-smoke-r1-gate
git -C "$tmp/rs" branch worktree-smoke-r1-work
git -C "$tmp/rs" checkout -q worktree-smoke-r1-work && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m work && git -C "$tmp/rs" checkout -q keep
remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev" "smoke-r1-"
check "remove_scratch deletes an empty EnterWorktree branch with the run name" not git -C "$tmp/rs" rev-parse --verify -q refs/heads/worktree-smoke-r1-gate
check "remove_scratch keeps a run-named branch that has commits" git -C "$tmp/rs" rev-parse --verify -q refs/heads/worktree-smoke-r1-work
check "remove_scratch keeps a branch from HEAD without the run name" git -C "$tmp/rs" rev-parse --verify -q refs/heads/owner-new
: >"$tmp/rs-ev/branches-before.txt"
check "remove_scratch refuses an empty list of branches" not remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev"
check "remove_scratch with an empty list deletes no branch" git -C "$tmp/rs" rev-parse --verify -q refs/heads/keep

# add_scratch_worktree makes a linked worktree on an orphan branch with no files
printf 'x\n' >"$tmp/repo/file.txt" && git -C "$tmp/repo" add file.txt && git -C "$tmp/repo" -c user.name=t -c user.email=t@example.com commit -q -m file
record_branches "$tmp/repo" "$tmp/rs-ev"
add_scratch_worktree "$tmp/repo" "$tmp/repo/.bruh-test/w" bruh-test-w >/dev/null 2>&1
check "add_scratch_worktree makes a linked worktree" eq "$(git -C "$tmp/repo/.bruh-test/w" rev-parse --git-common-dir)" "$(cd "$tmp/repo/.git" && pwd -P)"
check "add_scratch_worktree checks out the orphan branch" eq "$(git -C "$tmp/repo/.bruh-test/w" symbolic-ref --short HEAD)" bruh-test-w
check "add_scratch_worktree leaves no file" not test -e "$tmp/repo/.bruh-test/w/file.txt"
printf 'y\n' >"$tmp/repo/.bruh-test/w/new.txt"
commit_all "$tmp/repo/.bruh-test/w" "scratch" >/dev/null 2>&1
check "the scratch branch has a commit" git -C "$tmp/repo" rev-parse --verify -q refs/heads/bruh-test-w
check "remove_scratch removes the worktree and its branch" remove_scratch "$tmp/repo" "$tmp/repo/.bruh-test/w" "$tmp/rs-ev"
check "the scratch branch is gone" not git -C "$tmp/repo" rev-parse --verify -q refs/heads/bruh-test-w
check "the main file stays" test -e "$tmp/repo/file.txt"

# window_gaps
mkdir -p "$tmp/wbox/read"
printf '{"header":"P2 Q-1: load tick from s1","at":"2026-09-30T10:01:00.000Z"}' >"$tmp/wbox/read/a.json"
printf '{"header":"P2 Q-1: load tick from s1","at":"2026-09-30T10:12:00.000Z"}' >"$tmp/wbox/b.json"
printf '{"header":"P2 Q-0: load test start message","at":"2026-09-30T10:06:00.000Z"}' >"$tmp/wbox/read/c.json"
t0=$(jq -n '"2026-09-30T10:00:00Z" | fromdateiso8601')
check "window_gaps finds the window with only a start message" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 900)) 300)" 1
check "window_gaps is 0 when each window has a tick" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 300)) 300)" 0
check "window_gaps ignores a short last window" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 599)) 300)" 0
check "window_gaps counts every window of an empty mailbox" eq "$(window_gaps "$tmp/none" "$t0" $((t0 + 900)) 300)" 3

# Smoke preflight and cleanup with a fake claude on PATH. The fake prints the
# fixture $FAKE_AGENTS for `claude agents`, records `claude stop`, and, at the
# first `claude --bg`, switches to the fixture $FAKE_AGENTS_AFTER and creates
# clerk-ledger data, as bigm would.
mkdir -p "$tmp/fake/bin" "$tmp/fake/plugin/agents" "$tmp/fake/plugin/workflows" "$tmp/fake/plugin/ledger-template"
cat >"$tmp/fake/bin/claude" <<'SH'
#!/bin/sh
case $1 in
--version) echo "2.1.284 (Claude Code)" ;;
agents) cat "$FAKE_AGENTS" ;;
stop)
	echo "$2" >>"$FAKE_STOPS"
	jq --arg i "$2" 'map(if .id == $i then del(.pid) else . end)' "$FAKE_AGENTS" >"$FAKE_AGENTS.new" && mv "$FAKE_AGENTS.new" "$FAKE_AGENTS"
	;;
logs) ;;
*)
	if [ -n "${FAKE_AGENTS_AFTER:-}" ] && [ -f "$FAKE_AGENTS_AFTER" ]; then
		mv "$FAKE_AGENTS_AFTER" "$FAKE_AGENTS"
		mkdir -p "$BRUH_TEST_DATA/mail/clerk-ledger" && echo '{}' >"$BRUH_TEST_DATA/roles/clerk-ledger.json"
	fi
	;;
esac
SH
chmod +x "$tmp/fake/bin/claude"
for f in agents/bigm.md agents/clanker.md agents/clerk.md workflows/deliver.js; do : >"$tmp/fake/plugin/$f"; done
echo 'mode: human' >"$tmp/fake/plugin/ledger-template/mode.md"
ln -s "$(cd "$here/.." && pwd)/plugins/bruh/mcp" "$tmp/fake/plugin/mcp"
ln -s "$(cd "$here/.." && pwd)/plugins/bruh/defaults" "$tmp/fake/plugin/defaults"
git init -q "$tmp/strust" && git -C "$tmp/strust" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
srun=$(cd "$tmp/strust" && pwd -P)/.bruh-test/smoke-fk1
smoke_fake() {
	env PATH="$tmp/fake/bin:$PATH" BRUH_TEST_PLUGIN="$tmp/fake/plugin" BRUH_TEST_DATA="$tmp/sdata" \
		BRUH_TRUSTED_REPO="$tmp/strust" SMOKE_RUN=fk1 SMOKE_STEP_MINUTES=0 SMOKE_EVIDENCE="$tmp/sev" \
		FAKE_AGENTS="$tmp/fake/agents.json" FAKE_STOPS="$tmp/fake/stops.txt" sh "$here/smoke/run.sh"
}
cat >"$tmp/fake/agents.json" <<'JSON'
[{"id":"own1","name":"clerk-ledger","pid":41,"cwd":"/owner/ledger"}]
JSON
mkdir -p "$tmp/sdata/roles"
pout=$(smoke_fake 2>&1)
check "smoke preflight refuses a live clerk-ledger of the owner" contains "$pout" "clerk-ledger (own1)"
check "smoke preflight starts nothing when it refuses" not test -e "$tmp/fake/stops.txt"
jq -n --arg l "$srun/ledger" --arg p "$srun/project" --arg t "$(cd "$tmp/strust" && pwd -P)" '[
	{id: "gt1", name: "clerk-smoke-fk1-gate", pid: 5, cwd: ($t + "/.claude/worktrees/smoke-fk1-gate")},
	{id: "oth1", name: "clerk-other-x", pid: 6, cwd: ($t + "/.claude/worktrees/x")},
	{id: "bg1", name: "bigm", pid: 1, cwd: $l},
	{id: "cl1", name: "clerk-ledger", pid: 2, cwd: $l},
	{id: "mg1", name: "clerk-smoke-fk1-merge", pid: 3, cwd: ($p + "/.claude/worktrees/m")},
	{id: "own1", name: "clerk-ledger", pid: 4, cwd: "/owner/ledger"}]' >"$tmp/fake/agents-after.json"
echo '[]' >"$tmp/fake/agents.json"
FAKE_AGENTS_AFTER="$tmp/fake/agents-after.json" smoke_fake >"$tmp/fake/run.txt" 2>&1
stops=$(sort "$tmp/fake/stops.txt" 2>/dev/null | words)
check "smoke run reaches cleanup with the fake claude" contains "$(cat "$tmp/fake/run.txt")" "cleanup"
check "smoke cleanup stops bigm, clerk-ledger, a merger clerk, and a clerk in a worktree of the trusted repository" eq "$stops" "bg1 cl1 gt1 mg1"
check "smoke cleanup does not stop a clerk of another project in the trusted repository" not contains "$stops" oth1
check "smoke cleanup does not stop the clerk-ledger of the owner" not contains "$stops" own1
check "smoke cleanup removes the clerk-ledger data" not test -e "$tmp/sdata/mail/clerk-ledger"
check "smoke cleanup removes the clerk-ledger role settings" not test -e "$tmp/sdata/roles/clerk-ledger.json"
check "smoke cleanup removes the run folder" not test -e "$srun"

# Load driver dry run
out=$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 2>&1)
check "load dry run exits 0" eq "$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 >/dev/null 2>&1; echo $?)" 0
check "load dry run launches session 1" contains "$out" "claude --bg --name clanker-load-dry2-p1 --permission-mode auto --settings"
check "load dry run launches session 2" contains "$out" "claude --bg --name clerk-load-dry2-p1-s --permission-mode auto --settings"
check "load dry run launches no third session" not contains "$out" "load-dry2-p2"
check "load dry run uses the model flag" contains "$out" "--model haiku"
check "load dry run posts start messages" contains "$out" '"name":"mail_post","arguments":{"to":"clanker-load-dry2-p1"'
check "load dry run writes the role settings of a clerk as its clanker" contains "$out" '+ mcp as clanker-load-dry2-p1: {"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"role_settings_write","arguments":{"role_key":"clerk-load-dry2-p1-s"}'
check "load dry run tells sessions to nudge" contains "$out" "SendMessage"
check "load dry run pairs each session with its partner" contains "$out" "Your partner session is clanker-load-dry2-p1."
load8=$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry3 sh "$here/load/run.sh" --dry-run 2>&1)
check "load dry run defaults to 8 sessions" contains "$load8" "clerk-load-dry3-p4-s --permission-mode"
check "load refuses --sessions 1" not sh "$here/load/run.sh" --dry-run --sessions 1
check "load refuses an odd --sessions" not sh "$here/load/run.sh" --dry-run --sessions 3

# The MCP server accepts every call of the drivers: the sender policy of mail_post
# and the parent checks of role_settings_write (final review M1).
replay() {
	calls=$(printf '%s\n' "$1" | grep -c '^+ mcp as ')
	[ "$calls" -gt 0 ] || return 1
	printf '%s\n' "$1" | grep '^+ mcp as ' | while IFS= read -r line; do
		rest=${line#+ mcp as }
		req=${rest#*: }
		DRY=0 BRUH_DATA="$tmp/replay" BRUH_ROLE_KEY=${rest%%: *} BRUH_PLUGIN_ROOT="$here/../plugins/bruh" BRUH_TEST_MCP="$tmp/bruh-mcp" \
			mcp_call "$(printf '%s' "$req" | jq -r .params.name)" "$(printf '%s' "$req" | jq -c .params.arguments)" >/dev/null || return 1
	done
}
GOTOOLCHAIN=local go build -C "$here/../plugins/bruh/mcp" -o "$tmp/bruh-mcp" .
smoke_out=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry4 sh "$here/smoke/run.sh" --dry-run 2>&1)
check "the MCP server accepts each call of the smoke driver" replay "$smoke_out"
check "the MCP server accepts each call of the load driver" replay "$load8"
# Each tick of a load session posts to its partner as that session.
ticks() {
	pairs=$(printf '%s\n' "$1" | grep -o 'Your role key is [a-z0-9-]*\. Your partner session is [a-z0-9-]*\.' | sort -u)
	[ "$(printf '%s\n' "$pairs" | grep -c .)" -eq "$2" ] || return 1
	printf '%s\n' "$pairs" | while read -r _ _ _ _ me _ _ _ _ next; do
		DRY=0 BRUH_DATA="$tmp/replay" BRUH_ROLE_KEY=${me%.} BRUH_PLUGIN_ROOT="$here/../plugins/bruh" BRUH_TEST_MCP="$tmp/bruh-mcp" \
			mcp_call mail_post "$(jq -cn --arg to "${next%.}" '{to: $to, header: "P2 Q-1: load tick", body: "tick"}')" >/dev/null || return 1
	done
}
check "the MCP server accepts each tick of the 8 load sessions" ticks "$load8" 8
check "load refuses a word for --minutes" not sh "$here/load/run.sh" --dry-run --minutes ten

echo "$n tests, $fails failed"
[ "$fails" -eq 0 ]
