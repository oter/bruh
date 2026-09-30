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

# lane.sh: a private copy of the tree for one ticket, its patch, and the merge back
lane="$here/../plugins/bruh/scripts/lane.sh"
git init -q "$tmp/ln"
printf 'a\n' >"$tmp/ln/keep.txt" && printf 'b\n' >"$tmp/ln/gone.txt" && printf 'c\n' >"$tmp/ln/edit.txt"
mkdir -p "$tmp/ln/.scratch" "$tmp/ln/node_modules/.bin" && printf 's\n' >"$tmp/ln/.scratch/spec.md" && printf 't\n' >"$tmp/ln/node_modules/.bin/tsc"
git -C "$tmp/ln" add keep.txt gone.txt edit.txt && git -C "$tmp/ln" -c user.name=t -c user.email=t@example.com commit -q -m base
printf 'dirty\n' >"$tmp/ln/keep.txt" # the shared tree is dirty on purpose
lanes="$tmp/lanes"
lp=$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start t-01-a 2>/dev/null)
check "lane start prints the lane folder" eq "$lp" "$lanes/t-01-a"
check "lane start copies the dirty tree" eq "$(cat "$lp/keep.txt" 2>/dev/null)" dirty
check "lane start skips the top-level .scratch" not test -e "$lp/.scratch"
check "lane start keeps node_modules/.bin" test -e "$lp/node_modules/.bin/tsc"
check "a new lane shows no change" eq "$(git -C "$lp" diff --cached refs/lane/base 2>/dev/null)" ""
printf 'C\n' >"$lp/edit.txt" && rm "$lp/gone.txt" && printf 'n\n' >"$lp/new.txt"
out=$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" patch t-01-a 2>&1)
check "lane patch counts the files of the ticket" contains "$out" "(3 files)"
check "lane apply succeeds" env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" apply t-01-a
check "lane apply brings a changed file" eq "$(cat "$tmp/ln/edit.txt")" C
check "lane apply brings a new file" test -f "$tmp/ln/new.txt"
check "lane apply brings a deletion" not test -e "$tmp/ln/gone.txt"
check "lane apply keeps the dirty work of the shared tree" eq "$(cat "$tmp/ln/keep.txt")" dirty
check "lane apply leaves the index of the shared tree clean" eq "$(git -C "$tmp/ln" diff --cached --name-only)" ""
# shellcheck disable=SC2016 # the inner shell expands $1, $2, and $3
check "lane clean removes the lane and the patch" sh -c 'ROOT="$1" LANES="$2" sh "$3" clean t-01-a >/dev/null && ! test -e "$2/t-01-a" && ! test -e "$2/t-01-a.patch"' _ "$tmp/ln" "$lanes" "$lane"
check "lane refuses a ticket ID with a slash" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start ../x
check "lane refuses a ticket ID that starts with a dot" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start .x
check "lane refuses a ROOT that is not the top of a work tree" not env ROOT="$tmp/ln/node_modules" LANES="$lanes" sh "$lane" start t-02-b
check "lane refuses an unknown command" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" merge t-02-b
check "lane apply of a missing patch says so" contains "$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" apply t-09-z 2>&1)" "empty patch"
# A linked worktree has a .git pointer file; the lane must not share its index.
git -C "$tmp/ln" -c user.name=t -c user.email=t@example.com commit -qam work
git -C "$tmp/ln" worktree add -q "$tmp/lnw" -b lnw
lp=$(ROOT="$tmp/lnw" LANES="$lanes" sh "$lane" start t-02-b 2>/dev/null)
rm "$lp/keep.txt"
ROOT="$tmp/lnw" LANES="$lanes" sh "$lane" patch t-02-b >/dev/null 2>&1
check "a lane of a linked worktree leaves the index of the worktree clean" eq "$(git -C "$tmp/lnw" status --porcelain)" ""
# shellcheck disable=SC2016 # the inner shell expands $1, $2, and $3
check "a lane of a linked worktree applies a deletion" sh -c 'ROOT="$1" LANES="$2" sh "$3" apply t-02-b >/dev/null && ! test -e "$1/keep.txt"' _ "$tmp/lnw" "$lanes" "$lane"

# post-findings.sh with a fake glab and a fake gh on PATH
post="$here/../plugins/bruh/scripts/post-findings.sh"
mkdir -p "$tmp/fakebin"
cat >"$tmp/fakebin/glab" <<'FAKE'
#!/bin/sh
# Fake glab and gh: records each call, stores each POST body, and serves them back.
tool=${0##*/}
printf '%s %s\n' "$tool" "$*" >>"$FAKE_DIR/calls"
method=GET input='' path=''
while [ $# -gt 0 ]; do
	case $1 in
	-X) method=$2; shift ;;
	--input | -H) [ "$1" = --input ] && input=$2; shift ;;
	api | --paginate) ;;
	*) path=$1 ;;
	esac
	shift
done
if [ "$method" = POST ]; then
	if [ -n "${FAKE_REJECT_INLINE:-}" ] && jq -e 'has("position") or has("path")' "$input" >/dev/null; then
		if [ "$tool" = glab ]; then echo 'glab: 400 Bad Request (HTTP 400)' >&2; else echo 'gh: Validation Failed (HTTP 422)' >&2; fi
		exit 1
	fi
	jq -c . "$input" >>"$FAKE_DIR/posted.jsonl"
	id=$(wc -l <"$FAKE_DIR/posted.jsonl" | tr -d ' ')
	printf '{"id":%s,"notes":[{"id":%s}],"html_url":"https://example.com/c/%s"}\n' "$id" "$id" "$id"
	exit 0
fi
case $path in
*/discussions | */comments)
	# FAKE_FAIL_PAGE2: print the first page, then fail on the second, as --paginate does.
	if [ -n "${FAKE_FAIL_PAGE2:-}" ]; then
		jq -s '[.[] | {notes: [{body}], body}] | .[:1]' "$FAKE_DIR/posted.jsonl"
		echo "$tool: HTTP 502 on page 2" >&2
		exit 1
	fi
	;;
esac
case $path in
*/discussions) jq -s '[.[] | {notes: [{body}]}]' "$FAKE_DIR/posted.jsonl" ;;
*/pulls/*/comments) jq -s '[.[] | select(has("path")) | {body}]' "$FAKE_DIR/posted.jsonl" ;;
*/issues/*/comments) jq -s '[.[] | select(has("path") | not) | {body}]' "$FAKE_DIR/posted.jsonl" ;;
*) jq -n --arg h "$FAKE_HEAD" '{web_url: "https://example.com/mr/7", html_url: "https://example.com/pr/7",
	diff_refs: {base_sha: "1111111111111111111111111111111111111111", start_sha: "1111111111111111111111111111111111111111", head_sha: $h},
	head: {sha: $h}}' ;;
esac
FAKE
cp "$tmp/fakebin/glab" "$tmp/fakebin/gh"
chmod +x "$tmp/fakebin/glab" "$tmp/fakebin/gh"
RH=cccccccccccccccccccccccccccccccccccccccc
cat >"$tmp/result.json" <<JSON
{"status":"done","workflow":"review-only","base":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","head_sha":"$RH",
 "summaries":[{"key":"correctness","summary":"two bugs"},{"key":"security","summary":"clean"}],
 "confirmed":[{"file":"src/a.go","line":3,"lens":"correctness","rule":"guide R1","severity":"bug","problem":"nil map \$(touch $tmp/pwned)","fix":"make it","round":1,"reason":"shown"},
              {"file":"src/b.go","line":9,"lens":"security","rule":"asvs 6.1","severity":"security","problem":"timing","fix":"compare in constant time","round":1,"reason":"shown"}],
 "refuted":[{"file":"src/c.go","line":1,"lens":"correctness","rule":"r","severity":"nit","problem":"p","fix":"f","round":1,"reason":"taste"}],
 "deviations":[]}
JSON
# pf <fake folder> <args>: runs post-findings.sh with the fakes; FAKE_* and BRUH_* come from the caller.
pf() {
	d=$1
	shift
	mkdir -p "$d" && touch "$d/posted.jsonl" "$d/calls"
	FAKE_DIR=$d FAKE_HEAD=${FAKE_HEAD:-$RH} PATH="$tmp/fakebin:$PATH" sh "$post" "$@"
}
posts() { grep -c . "$1/posted.jsonl"; }
# Each posted body starts and ends with the marker line and says "Agent review".
marked() {
	jq -e --arg m "$2" '.body | split("\n") as $l | $l[0] == $m and $l[-1] == $m and contains("Agent review")' "$1/posted.jsonl" >/dev/null &&
		[ "$(jq -s --arg m "$2" '[.[] | .body | split("\n") | select(.[0] != $m or .[-1] != $m)] | length' "$1/posted.jsonl")" = 0 ]
}
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf1" gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "post-findings refuses without --yes or a post grant" contains "$out" "exit 3"
check "post-findings refusal in a manual session names --yes" contains "$out" "pass --yes only after the owner said yes"
check "post-findings refusal posts nothing" eq "$(posts "$tmp/pf1")" 0
check "post-findings refusal reads nothing from the code host" eq "$(grep -c . "$tmp/pf1/calls")" 0
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf1" --dry-run gitlab group/app 7 "$tmp/result.json" 2>&1)
check "post-findings dry run needs no --yes and shows each body" contains "$out" "would post inline src/a.go:3"
check "post-findings dry run posts nothing" eq "$(posts "$tmp/pf1")" 0
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf1" --yes gitlab group/app 7 "$tmp/result.json" 2>&1)
check "post-findings posts the summary and each confirmed finding" eq "$(posts "$tmp/pf1")" 3
check "post-findings reports the counts" contains "$out" "2 inline, 0 general, 0 failed, 0 already posted"
check "post-findings marks each body with the owner marker" marked "$tmp/pf1" '<!-- bruh:owner -->'
check "post-findings posts inline findings on the head of the merge request" eq "$(jq -s '[.[] | select(.position.head_sha == "'"$RH"'" and .position.new_line == 3)] | length' "$tmp/pf1/posted.jsonl")" 1
check "post-findings summary counts the confirmed and refuted findings" contains "$(jq -r 'select(.position == null) | .body' "$tmp/pf1/posted.jsonl")" "2 confirmed (1 security, 1 bug), 1 refuted"
check "post-findings never runs finding text in a shell" not test -e "$tmp/pwned"
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf1" --yes gitlab group/app 7 "$tmp/result.json" 2>&1)
check "post-findings rerun posts nothing new" eq "$(posts "$tmp/pf1")" 3
check "post-findings rerun reports each body as already posted" contains "$out" "0 inline, 0 general, 0 failed, 2 already posted"
# A role session needs the approval ANSWER of bigm in its mailbox, or a post grant; --yes is refused.
mkdir -p "$tmp/pfbox/mail/clerk-app-t1/read"
jq -n '{id: "1", from: "clanker-app", to: "clerk-app-t1", header: "ANSWER Q-4: post group/app#7 approved", body: "yes"}' >"$tmp/pfbox/mail/clerk-app-t1/1.json"
jq -n '{id: "2", from: "bigm", to: "clerk-app-t1", header: "ANSWER Q-5: post group/app#8 approved", body: "yes"}' >"$tmp/pfbox/mail/clerk-app-t1/2.json"
jq -n '{id: "3", from: "bigm", to: "clerk-app-t1", header: "ANSWER Q-6: post group/app#7 refused", body: "no"}' >"$tmp/pfbox/mail/clerk-app-t1/3.json"
out=$(BRUH_ROLE_KEY=clerk-app-t1 pf "$tmp/pf2" --yes gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "post-findings refuses --yes in a role session" contains "$out" "exit 3"
for q in Q-4 Q-5 Q-6 Q-9; do
	out=$(BRUH_ROLE_KEY=clerk-app-t1 pf "$tmp/pf2" --data "$tmp/pfbox" --answer "$q" gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
	check "post-findings refuses $q: not an approval of bigm for this post" contains "$out" "exit 3"
done
check "a refused post posts nothing" eq "$(posts "$tmp/pf2")" 0
jq -n '{id: "4", from: "bigm", to: "clerk-app-t1", header: "ANSWER Q-7: post group/app#7 approved", body: "owner: yes"}' >"$tmp/pfbox/mail/clerk-app-t1/read/4.json"
out=$(BRUH_ROLE_KEY=clerk-app-t1 pf "$tmp/pf2" --data "$tmp/pfbox" --answer Q-7 gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "the approval ANSWER of bigm for this post allows it" contains "$out" "exit 0"
check "post-findings marks each body with the role key" marked "$tmp/pf2" '<!-- bruh:clerk-app-t1 -->'
check "post-findings refuses a bad question ID" not pf "$tmp/pf2" --answer 'Q-1 x' gitlab group/app 7 "$tmp/result.json"
# A post grant row lets the named role key post without --yes; a merge grant row does not.
mkdir -p "$tmp/pfdata/init" "$tmp/pfledger"
jq -n --arg l "$tmp/pfledger" '{ledger_path: $l}' >"$tmp/pfdata/init/config.json"
cat >"$tmp/pfledger/grants.md" <<'MD'
# Grants

## Merge grants

| Repository | Merger role key | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|
| group/app | clerk-app-t1 | green CI | "merge it" | 2026-09-30T10:00:00Z | Q-1 |

## Post grants

| Poster role key | Host | Repository | Conditions | Owner words | Date (UTC) | Question ID |
|---|---|---|---|---|---|---|
| `clerk-app-t2` | gitlab | `group/app` | review results | "post reviews" | 2026-09-30T10:00:00Z | Q-2 |
MD
out=$(BRUH_ROLE_KEY=clerk-app-t1 pf "$tmp/pf3" --data "$tmp/pfdata" gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "a merge grant row is not a post grant" contains "$out" "exit 3"
out=$(BRUH_ROLE_KEY=clerk-app-t2 pf "$tmp/pf3" --data "$tmp/pfdata" gitlab group/other 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "a post grant covers only its repository" contains "$out" "exit 3"
out=$(BRUH_ROLE_KEY=clerk-app-t2 pf "$tmp/pf3" --data "$tmp/pfdata" github group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "a post grant covers only its host" contains "$out" "exit 3"
out=$(BRUH_ROLE_KEY=clerk-app-t2 pf "$tmp/pf3" --data "$tmp/pfdata" gitlab group/app 7 "$tmp/result.json" 2>&1; echo "exit $?")
check "a post grant row for the repository and the role key allows the post" contains "$out" "exit 0"
check "the post grant posts each body" eq "$(posts "$tmp/pf3")" 3
out=$(unset BRUH_ROLE_KEY; FAKE_REJECT_INLINE=1 pf "$tmp/pf4" --yes gitlab group/app 7 "$tmp/result.json" 2>&1)
check "a rejected inline position becomes a general comment" contains "$out" "0 inline, 2 general, 0 failed"
# shellcheck disable=SC2016 # literal backticks
check "a general comment names file:line" contains "$(jq -r .body "$tmp/pf4/posted.jsonl")" '`src/a.go:3`'
out=$(unset BRUH_ROLE_KEY; FAKE_HEAD=dddddddddddddddddddddddddddddddddddddddd pf "$tmp/pf5" --yes gitlab group/app 7 "$tmp/result.json" 2>&1)
check "a moved head posts every finding as a general comment" contains "$out" "0 inline, 2 general"
check "a moved head is named in the summary" contains "$(jq -r .body "$tmp/pf5/posted.jsonl")" "has moved to \`dddddddd\`"
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf6" --yes github owner/app 7 "$tmp/result.json" 2>&1)
check "post-findings posts on GitHub" contains "$out" "2 inline, 0 general, 0 failed"
check "GitHub inline comments name the commit, the path, and the line" eq "$(jq -s '[.[] | select(.commit_id == "'"$RH"'" and .path == "src/b.go" and .line == 9 and .side == "RIGHT")] | length' "$tmp/pf6/posted.jsonl")" 1
check "GitHub posts go to the pull request" contains "$(cat "$tmp/pf6/calls")" "repos/owner/app/pulls/7/comments"
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf6" --yes github owner/app 7 "$tmp/result.json" 2>&1)
check "GitHub rerun posts nothing new" eq "$(posts "$tmp/pf6")" 3
jq '.status = "stopped"' "$tmp/result.json" >"$tmp/stopped.json"
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf7" --yes gitlab group/app 7 "$tmp/stopped.json" 2>&1; echo "exit $?")
check "post-findings refuses a stopped result" contains "$out" "exit 2"
check "post-findings refuses a bad repository" not pf "$tmp/pf7" --yes github 'owner/app;x' 7 "$tmp/result.json"
check "post-findings refuses a repository with a newline" not pf "$tmp/pf7" --yes github "owner/app
../../orgs/x" 7 "$tmp/result.json"
check "a refused repository reaches no code host" eq "$(grep -c . "$tmp/pf7/calls")" 0
# A failed read of any page of the existing comments stops before the first post.
for h in gitlab github; do
	r=group/app
	[ "$h" = github ] && r=owner/app
	(unset BRUH_ROLE_KEY; pf "$tmp/pf9$h" --yes "$h" "$r" 7 "$tmp/result.json" >/dev/null 2>&1)
	out=$(unset BRUH_ROLE_KEY; FAKE_FAIL_PAGE2=1 pf "$tmp/pf9$h" --yes "$h" "$r" 7 "$tmp/result.json" 2>&1; echo "exit $?")
	check "a failed page 2 of the $h comments exits non-zero" not contains "$out" "exit 0"
	check "a failed page 2 of the $h comments posts nothing" eq "$(posts "$tmp/pf9$h")" 3
done
check "post-findings refuses an unknown host" not pf "$tmp/pf7" --yes gitea owner/app 7 "$tmp/result.json"
jq '.workflow = "review-and-fix" | .status = "findings_left" | .confirmed[0].state = "fixed" | .confirmed[1].state = "open" | .confirmed[1].round = 2' "$tmp/result.json" >"$tmp/fixed.json"
out=$(unset BRUH_ROLE_KEY; pf "$tmp/pf8" --yes gitlab group/app 7 "$tmp/fixed.json" 2>&1)
check "a finding of a later round is a general comment" contains "$out" "1 inline, 1 general"
check "review-and-fix gets a note of the fixes" contains "$(jq -r .body "$tmp/pf8/posted.jsonl")" "1 of 2 findings fixed"
check "the note of the fixes is marked" marked "$tmp/pf8" '<!-- bruh:owner -->'

echo "$n tests, $fails failed"
[ "$fails" -eq 0 ]
