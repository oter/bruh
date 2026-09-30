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
check "smoke dry run stops own sessions in cleanup" contains "$out" "claude stop '<each session in the run folder>'"
rout=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 SMOKE_ORCA_ENV=env1 SMOKE_ORCA_REPO=name:r sh "$here/smoke/run.sh" --dry-run 2>&1)
check "smoke remote step starts a worker with no setup hooks" contains "$rout" "--setup skip --agent claude --spec"
check "smoke remote step names the remote worktree" contains "$rout" "--name bruh-smoke-dry1"
check "smoke remote step asks for the rot13 of the reply" contains "$rout" "tr 'abcdefghijklmnopqrstuvwxyz' 'nopqrstuvwxyzabcdefghijklm'"
check "smoke remote step removes the remote worktree in cleanup" contains "$rout" "orca worktree rm --environment env1 --worktree name:bruh-smoke-dry1 --force"
check "smoke refuses an unknown flag" not sh "$here/smoke/run.sh" --bogus

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

# Load driver dry run
out=$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 2>&1)
check "load dry run exits 0" eq "$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 >/dev/null 2>&1; echo $?)" 0
check "load dry run launches session 1" contains "$out" "claude --bg --name clerk-load-dry2-s1 --permission-mode auto --settings"
check "load dry run launches session 2" contains "$out" "claude --bg --name clerk-load-dry2-s2 --permission-mode auto --settings"
check "load dry run launches no third session" not contains "$out" "clerk-load-dry2-s3 --permission-mode"
check "load dry run uses the model flag" contains "$out" "--model haiku"
check "load dry run posts start messages" contains "$out" '"name":"mail_post","arguments":{"to":"clerk-load-dry2-s1"'
check "load dry run tells sessions to nudge" contains "$out" "SendMessage"
check "load dry run closes the ring" contains "$out" "The next session in the ring is clerk-load-dry2-s1."
check "load dry run defaults to 8 sessions" contains "$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry3 sh "$here/load/run.sh" --dry-run 2>&1)" "clerk-load-dry3-s8 --permission-mode"
check "load refuses --sessions 1" not sh "$here/load/run.sh" --dry-run --sessions 1
check "load refuses a word for --minutes" not sh "$here/load/run.sh" --dry-run --minutes ten

echo "$n tests, $fails failed"
[ "$fails" -eq 0 ]
