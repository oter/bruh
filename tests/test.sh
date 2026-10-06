#!/bin/sh
# Tests for tests/lib.sh and for the dry runs of the smoke, load, and learner eval drivers.
# No Claude session is started. Run: sh tests/test.sh
set -u
here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=tests/lib.sh disable=SC1091 # followed when tests/lib.sh is an input, as in CI
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
# refused <text> <command...>: the command exits non-zero and its output contains text.
refused() {
	want=$1
	shift
	if o=$("$@" 2>&1); then return 1; fi
	contains "$o" "$want"
}

# group <name> starts one check of several rows; row <label> <command...> runs one row of it;
# end_group counts the group as one check and prints each row that failed, so one failing row
# does not hide another.
group() {
	gname=$1
	gbad=""
}
row() {
	label=$1
	shift
	"$@" >/dev/null 2>&1 || gbad="$gbad# $gname: row failed: $label
"
}
end_group() {
	check "$gname" eq "$gbad" ""
	printf '%s' "$gbad"
}

group "version_ge compares versions"
row "equal" version_ge 2.1.284 2.1.284
row "older patch" not version_ge 2.1.283 2.1.284
row "newer minor" version_ge 2.2.0 2.1.284
row "newer patch with more digits" version_ge 2.1.1000 2.1.284
row "older major" not version_ge 1.9.999 2.1.284
end_group

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
group "own_sessions keeps the sessions of the run folder"
row "the run folder and the names" eq "$(own_sessions "$tmp/agents.json" /s/run clanker-smoke-abc clerk-smoke-abc- | words)" "a3 a5"
row "nothing for another prefix" eq "$(own_sessions "$tmp/agents.json" /nowhere bigm)" ""
row "an empty name prefix takes every session of the folder" eq "$(own_sessions "$tmp/agents.json" /s/run "" | words)" "a3 a5 a7"
end_group
cat >"$tmp/agents2.json" <<'JSON'
[
  {"id":"b1","name":"clerk-ledger","pid":11,"cwd":"/owner/ledger"},
  {"id":"b2","name":"clanker-smoke-abc","cwd":"/old"},
  {"id":"b3","name":"other","pid":12,"cwd":"/x"}
]
JSON
group "live_conflicts finds only live sessions with a run name"
row "the live clerk-ledger of the owner" eq "$(live_conflicts "$tmp/agents2.json" bigm clerk-ledger clanker-smoke-abc clerk-smoke-abc-)" "clerk-ledger (b1)"
row "not a stopped session with a run name" eq "$(live_conflicts "$tmp/agents2.json" clanker-smoke-abc)" ""
end_group

# valid_run_id guards the rm -rf paths of the drivers.
group "valid_run_id accepts only 1 to 12 small letters and digits"
row "letters and digits" valid_run_id abc123
row "a path" not valid_run_id 'a/../..'
row "capitals" not valid_run_id Abc
row "a hyphen" not valid_run_id a-b
row "an empty value" not valid_run_id ''
row "more than 12 characters" not valid_run_id abcdefghijklm
end_group
group "the drivers refuse a bad run ID"
row "smoke, SMOKE_RUN" not env SMOKE_RUN='a/../..' sh "$here/smoke/run.sh" --dry-run
row "load, LOAD_RUN" not env LOAD_RUN='a/../..' sh "$here/load/run.sh" --dry-run
end_group
mkdir -p "$tmp/rel/sub"
group "abs_dir resolves an existing folder only"
row "a relative folder" eq "$(cd "$tmp/rel" && abs_dir sub)" "$(cd "$tmp/rel/sub" && pwd -P)"
row "a missing folder fails" not abs_dir "$tmp/nothing-here"
end_group

# utc_ago
check "utc_ago has the layout of the MCP server" contains "$(utc_ago 60)" ".000Z"
lt() { [ "$1" != "$2" ] && [ "$(printf '%s\n%s\n' "$1" "$2" | sort | head -1)" = "$1" ]; }
group "utc_ago goes back the given seconds"
row "before now" lt "$(utc_ago 3600)" "$(utc_now)"
row "after two hours ago" lt "$(utc_ago 7200)" "$(utc_ago 3600)"
end_group

# check_trusted_repo
git init -q "$tmp/repo" && git -C "$tmp/repo" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
git init -q "$tmp/empty"
git init -q "$tmp/remote" && git -C "$tmp/remote" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
git -C "$tmp/remote" remote add origin https://example.com/owner/repo.git
mkdir -p "$tmp/plain"
check "check_trusted_repo accepts a repository with no remote" check_trusted_repo "$tmp/repo"
group "check_trusted_repo refuses an untrusted repository"
row "an empty value" not check_trusted_repo ""
row "a missing folder" not check_trusted_repo "$tmp/missing"
row "a folder that is not a repository" not check_trusted_repo "$tmp/plain"
row "a repository with no commit" not check_trusted_repo "$tmp/empty"
row "a repository with a remote" not check_trusted_repo "$tmp/remote"
row "the refusal names the remote problem" contains "$(check_trusted_repo "$tmp/remote")" "remote"
end_group
# shellcheck disable=SC2016 # the inner shell expands $1 and $2
check "check_trusted_repo accepts a remote with SMOKE_ALLOW_REMOTE=1" env SMOKE_ALLOW_REMOTE=1 sh -c '. "$1" && check_trusted_repo "$2"' _ "$here/lib.sh" "$tmp/remote"
printf '{"permissions":{}}' >"$tmp/push.json"
add_push_block "$tmp/push.json" "$tmp/remote"
# shellcheck disable=SC2016 # the inner shell expands $1 and $2
check "add_push_block sets the push URL of each remote to a missing path" \
	sh -c 'cd "$1" && env $(jq -r ".env | to_entries[] | \"\(.key)=\(.value)\"" "$2") git config remote.origin.pushurl | grep -qx /nonexistent/bruh-smoke-no-push' _ "$tmp/remote" "$tmp/push.json"

# count_missed
mkdir -p "$tmp/box/read"
printf '{"id":"1","header":"P2 Q-load-tick-1: old","at":"2026-09-30T10:00:00.000Z"}' >"$tmp/box/1.json"
printf '{"id":"2","header":"P2 Q-load-tick-1: new","at":"2026-09-30T10:59:00.000Z"}' >"$tmp/box/2.json"
printf '{"id":"3","header":"P2 Q-load-tick-1: read","at":"2026-09-30T09:00:00.000Z"}' >"$tmp/box/read/3.json"
group "count_missed counts old unread messages only"
row "an old unread message" eq "$(count_missed "$tmp/box" 2026-09-30T10:55:00.000Z)" 1
row "0 for a missing mailbox" eq "$(count_missed "$tmp/nobox" 2026-09-30T10:55:00.000Z)" 0
end_group
check "count_all counts unread and read" eq "$(count_all "$tmp/box")" 3

group "data_dir finds the data folder"
row "the inline plugin id" eq "$(unset BRUH_TEST_DATA; CLAUDE_CODE_PLUGIN_CACHE_DIR=/p data_dir)" /p/data/bruh-inline
row "BRUH_TEST_DATA" eq "$(BRUH_TEST_DATA=/d data_dir)" /d
end_group

# Smoke driver dry run
out=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 sh "$here/smoke/run.sh" --dry-run 2>&1)
check "smoke dry run exits 0" eq "$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 sh "$here/smoke/run.sh" --dry-run >/dev/null 2>&1; echo $?)" 0
group "smoke dry run prints each step"
row "starts bigm with the documented flags" contains "$out" "claude --bg --agent bruh:bigm --name bigm --permission-mode auto --settings"
row "loads the plugin of the checkout" contains "$out" "--plugin-dir $(cd "$here/.." && pwd)/plugins/bruh"
row "writes the bigm role settings" contains "$out" '"name":"role_settings_write","arguments":{"role_key":"bigm"}'
row "asks a plain claude in the ledger for its role name" contains "$out" "claude -p --strict-mcp-config --tools '' --max-turns 1 --plugin-dir"
row "stops the clanker to simulate the idle stop" contains "$out" "claude stop '<clanker id>'"
row "lets bigm, not the driver, message the clanker" contains "$out" "session named bigm, then stop: SMOKE-IDLE idle"
row "uses a linked worktree" contains "$out" "worktree add -q --detach"
row "skips the remote step" contains "$out" "SKIP remote"
row "builds the index of a scratch repository with init --answers" contains "$out" " init --answers "
row "builds the index of a scratch repository with the learner" contains "$out" "--agent bruh:learner"
row "runs init with a scratch settings file" contains "$out" "BRUH_SETTINGS_FILE="
end_group
# A variadic option of claude (<tools...>, <directories...>, <configs...>) reads
# every next word that is not an option, so a prompt right after its value
# becomes more values. Each printed claude command must follow the value of a
# variadic option with another option.
bad=$(printf '%s\n' "$out" | awk '/^\+ / { c = 0; for (i = 2; i <= NF; i++) { if ($i == "claude") c = 1; if (c && $i ~ /^--(allowedTools|allowed-tools|disallowedTools|disallowed-tools|tools|add-dir|mcp-config|betas|file)$/ && i + 2 <= NF && $(i + 2) !~ /^--/) print } }')
check "smoke dry run puts no prompt right after a variadic claude option" eq "$bad" ""
check "smoke dry run does not call session_resume itself" not contains "$out" '"name":"session_resume"'
group "the smoke and load drivers trap HUP"
row "smoke" grep -q "trap 'exit 129' HUP" "$here/smoke/run.sh"
row "load" grep -q "trap 'exit 129' HUP" "$here/load/run.sh"
end_group
rout=$(BRUH_TRUSTED_REPO="$tmp/repo" SMOKE_RUN=dry1 SMOKE_ORCA_ENV=env1 SMOKE_ORCA_REPO=name:r sh "$here/smoke/run.sh" --dry-run 2>&1)
group "smoke remote step prints each step"
row "starts a worker with no setup hooks" contains "$rout" "--setup skip --agent claude --spec"
row "names the remote worktree" contains "$rout" "--name bruh-smoke-dry1"
row "removes the remote worktree in cleanup" contains "$rout" "orca worktree rm --environment env1 --worktree name:bruh-smoke-dry1 --force"
end_group
check "smoke refuses an unknown flag" not sh "$here/smoke/run.sh" --bogus

# random_id ends also when SIGPIPE is ignored, as on a CI runner.
# perl ignores SIGPIPE, then execs sh; the alarm stops sh after 10 seconds with status 142.
rid_rc=0
rid=$(perl -e '$SIG{PIPE}="IGNORE"; alarm 10; exec "sh","-c",". \"$ARGV[0]\" && random_id"' "$here/lib.sh") || rid_rc=$?
check "random_id ends with SIGPIPE ignored" eq "$rid_rc" 0
group "random_id prints a run ID of 6 characters"
row "a valid run ID" valid_run_id "$rid"
row "6 characters" eq "${#rid}" 6
end_group
# remove_scratch deletes only the branches that the run created
git init -q "$tmp/rs" && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m init
mkdir -p "$tmp/rs-ev" "$tmp/rs/.bruh-test/x"
git -C "$tmp/rs" branch keep
record_branches "$tmp/rs" "$tmp/rs-ev"
git -C "$tmp/rs" branch owner-new
git -C "$tmp/rs" checkout -q --orphan new-of-run && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m orphan
git -C "$tmp/rs" checkout -q keep
remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev"
check "remove_scratch deletes the new orphan branch of the run" not git -C "$tmp/rs" rev-parse --verify -q refs/heads/new-of-run
group "remove_scratch keeps the branches that the run did not make"
row "a new branch that another person made from HEAD" git -C "$tmp/rs" rev-parse --verify -q refs/heads/owner-new
row "the old branches" git -C "$tmp/rs" rev-parse --verify -q refs/heads/keep
end_group
check "remove_scratch removes the scratch folder" not test -e "$tmp/rs/.bruh-test/x"
mkdir -p "$tmp/rs/.bruh-test/x"
git -C "$tmp/rs" branch worktree-smoke-r1-gate
git -C "$tmp/rs" branch worktree-smoke-r1-work
git -C "$tmp/rs" checkout -q worktree-smoke-r1-work && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m work && git -C "$tmp/rs" checkout -q keep
remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev" "smoke-r1-"
check "remove_scratch deletes an empty EnterWorktree branch with the run name" not git -C "$tmp/rs" rev-parse --verify -q refs/heads/worktree-smoke-r1-gate
group "remove_scratch with a run name keeps a branch with work or without the name"
row "a run-named branch that has commits" git -C "$tmp/rs" rev-parse --verify -q refs/heads/worktree-smoke-r1-work
row "a branch from HEAD without the run name" git -C "$tmp/rs" rev-parse --verify -q refs/heads/owner-new
end_group
# An orphan branch with a commit is a branch of a run unless the list of branches names it, so
# without the guard of an empty list remove_scratch would delete it.
git -C "$tmp/rs" checkout -q --orphan rs-orphan && git -C "$tmp/rs" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m orphan
git -C "$tmp/rs" checkout -q keep
: >"$tmp/rs-ev/branches-before.txt"
check "remove_scratch refuses an empty list of branches" not remove_scratch "$tmp/rs" "$tmp/rs/.bruh-test/x" "$tmp/rs-ev"
check "remove_scratch with an empty list keeps an orphan branch" git -C "$tmp/rs" rev-parse --verify -q refs/heads/rs-orphan

# add_scratch_worktree makes a linked worktree on an orphan branch with no files
printf 'x\n' >"$tmp/repo/file.txt" && git -C "$tmp/repo" add file.txt && git -C "$tmp/repo" -c user.name=t -c user.email=t@example.com commit -q -m file
record_branches "$tmp/repo" "$tmp/rs-ev"
add_scratch_worktree "$tmp/repo" "$tmp/repo/.bruh-test/w" bruh-test-w >/dev/null 2>&1
check "add_scratch_worktree makes a linked worktree" eq "$(git -C "$tmp/repo/.bruh-test/w" rev-parse --git-common-dir)" "$(cd "$tmp/repo/.git" && pwd -P)"
check "add_scratch_worktree checks out the orphan branch" eq "$(git -C "$tmp/repo/.bruh-test/w" symbolic-ref --short HEAD)" bruh-test-w
check "add_scratch_worktree leaves no file" not test -e "$tmp/repo/.bruh-test/w/file.txt"
printf 'y\n' >"$tmp/repo/.bruh-test/w/new.txt"
commit_all "$tmp/repo/.bruh-test/w" "scratch" >/dev/null 2>&1
group "remove_scratch removes the worktree and its scratch branch"
row "the scratch branch has a commit before" git -C "$tmp/repo" rev-parse --verify -q refs/heads/bruh-test-w
remove_scratch "$tmp/repo" "$tmp/repo/.bruh-test/w" "$tmp/rs-ev"
row "the scratch branch is gone after" not git -C "$tmp/repo" rev-parse --verify -q refs/heads/bruh-test-w
end_group
check "the main file stays" test -e "$tmp/repo/file.txt"

# window_gaps
mkdir -p "$tmp/wbox/read"
printf '{"header":"P2 Q-load-tick-1: load tick from s1","at":"2026-09-30T10:01:00.000Z"}' >"$tmp/wbox/read/a.json"
printf '{"header":"P2 Q-load-tick-1: load tick from s1","at":"2026-09-30T10:12:00.000Z"}' >"$tmp/wbox/b.json"
printf '{"header":"P2 Q-load-start-0: load test start message","at":"2026-09-30T10:06:00.000Z"}' >"$tmp/wbox/read/c.json"
t0=$(jq -n '"2026-09-30T10:00:00Z" | fromdateiso8601')
group "window_gaps counts the windows with no load tick"
row "the window with only a start message" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 900)) 300)" 1
row "0 when each window has a tick" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 300)) 300)" 0
row "a short last window is ignored" eq "$(window_gaps "$tmp/wbox" "$t0" $((t0 + 599)) 300)" 0
row "every window of an empty mailbox" eq "$(window_gaps "$tmp/none" "$t0" $((t0 + 900)) 300)" 3
end_group

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
--bg)
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
	{id: "tk1", name: "clerk-smoke-fk1-t1", pid: 3, cwd: ($p + "/.claude/worktrees/m")},
	{id: "own1", name: "clerk-ledger", pid: 4, cwd: "/owner/ledger"}]' >"$tmp/fake/agents-after.json"
echo '[]' >"$tmp/fake/agents.json"
FAKE_AGENTS_AFTER="$tmp/fake/agents-after.json" smoke_fake >"$tmp/fake/run.txt" 2>&1
stops=$(sort "$tmp/fake/stops.txt" 2>/dev/null | words)
check "smoke cleanup stops bigm, clerk-ledger, a task clerk, and a clerk in a worktree of the trusted repository" eq "$stops" "bg1 cl1 gt1 tk1"
group "smoke cleanup removes the data of the run"
row "the clerk-ledger data" not test -e "$tmp/sdata/mail/clerk-ledger"
row "the clerk-ledger role settings" not test -e "$tmp/sdata/roles/clerk-ledger.json"
row "the run folder" not test -e "$srun"
end_group

# Load driver dry run
out=$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 2>&1)
check "load dry run exits 0" eq "$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry2 sh "$here/load/run.sh" --dry-run --sessions 2 --minutes 1 >/dev/null 2>&1; echo $?)" 0
group "load dry run prints each step"
row "launches session 1" contains "$out" "claude --bg --name clanker-load-dry2-p1 --permission-mode auto --settings"
row "launches session 2" contains "$out" "claude --bg --name clerk-load-dry2-p1-s --permission-mode auto --settings"
row "uses the model flag" contains "$out" "--model haiku"
row "posts start messages" contains "$out" '"name":"mail_post","arguments":{"to":"clanker-load-dry2-p1"'
row "pairs each session with its partner" contains "$out" "Your partner session is clanker-load-dry2-p1."
end_group
check "load dry run launches no third session" not contains "$out" "load-dry2-p2"
check "load dry run writes the role settings of a clerk as its clanker" contains "$out" '+ mcp as clanker-load-dry2-p1: {"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"role_settings_write","arguments":{"role_key":"clerk-load-dry2-p1-s"}'
load8=$(BRUH_TRUSTED_REPO="$tmp/repo" LOAD_RUN=dry3 sh "$here/load/run.sh" --dry-run 2>&1)
group "load refuses a bad option"
row "--sessions 1" not sh "$here/load/run.sh" --dry-run --sessions 1
row "an odd --sessions" not sh "$here/load/run.sh" --dry-run --sessions 3
row "a word for --minutes" not sh "$here/load/run.sh" --dry-run --minutes ten
end_group

# Learner eval driver: dry run and the compare rule of spec 20
eout=$(sh "$here/eval/run.sh" --dry-run 2>&1)
check "learner eval dry run exits 0" eq "$(sh "$here/eval/run.sh" --dry-run >/dev/null 2>&1; echo $?)" 0
check "learner eval dry run prints one learner command for each case" eq "$(printf '%s\n' "$eout" | grep -c 'env -u BRUH_ROLE_KEY timeout [0-9]* claude -p --agent bruh:learner')" "$(find "$here/eval/learner" -mindepth 1 -maxdepth 1 -type d | wc -l | tr -d ' ')"
check "learner eval dry run loads the plugin of the checkout" contains "$eout" "--plugin-dir $(cd "$here/.." && pwd)/plugins/bruh"
mkdir -p "$tmp/eval"
printf 'LINK auth: shop/go.mod:5\nDOC: shop/README.md\n' >"$tmp/eval/expected.txt"
cp "$tmp/eval/expected.txt" "$tmp/eval/passes-the-expected-lines.txt"
printf 'LINK auth: shop/go.mod:5\nDOC: shop/README.md\nDOC: shop/CLAUDE.md\n' >"$tmp/eval/allows-an-extra-doc-line.txt"
printf 'PURPOSE: The shop service sells items.\nI read the files of shop.\nLINK auth: shop/go.mod:5\nDOC: shop/README.md\n' >"$tmp/eval/ignores-other-text.txt"
printf 'LINK auth: shop/go.mod:5\nLINK auth: shop/README.md:3\nDOC: shop/README.md\n' >"$tmp/eval/fails-an-extra-link-line.txt"
printf 'DOC: shop/README.md\n' >"$tmp/eval/fails-a-missing-line.txt"
group "learner eval compare follows the rule of spec 20"
row "passes the expected lines" sh "$here/eval/run.sh" --compare "$tmp/eval/expected.txt" "$tmp/eval/passes-the-expected-lines.txt"
row "allows an extra DOC line" sh "$here/eval/run.sh" --compare "$tmp/eval/expected.txt" "$tmp/eval/allows-an-extra-doc-line.txt"
row "ignores other text" sh "$here/eval/run.sh" --compare "$tmp/eval/expected.txt" "$tmp/eval/ignores-other-text.txt"
row "fails an extra LINK line" not sh "$here/eval/run.sh" --compare "$tmp/eval/expected.txt" "$tmp/eval/fails-an-extra-link-line.txt"
row "fails a missing line" not sh "$here/eval/run.sh" --compare "$tmp/eval/expected.txt" "$tmp/eval/fails-a-missing-line.txt"
end_group
check "learner eval expected files have only closed lines" not grep -Eqv '^$|^(LINK [a-z0-9-]+: [^ ]+:[0-9]+|DOC: [^ ]+)$' "$here"/eval/learner/*/expected.txt

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
group "the MCP server accepts each call of the drivers"
row "smoke" replay "$smoke_out"
row "load, 8 sessions" replay "$load8"
end_group
# Each tick of a load session posts to its partner as that session.
ticks() {
	pairs=$(printf '%s\n' "$1" | grep -o 'Your role key is [a-z0-9-]*\. Your partner session is [a-z0-9-]*\.' | sort -u)
	[ "$(printf '%s\n' "$pairs" | grep -c .)" -eq "$2" ] || return 1
	printf '%s\n' "$pairs" | while read -r _ _ _ _ me _ _ _ _ next; do
		DRY=0 BRUH_DATA="$tmp/replay" BRUH_ROLE_KEY=${me%.} BRUH_PLUGIN_ROOT="$here/../plugins/bruh" BRUH_TEST_MCP="$tmp/bruh-mcp" \
			mcp_call mail_post "$(jq -cn --arg to "${next%.}" '{to: $to, header: "P2 Q-load-tick-1: load tick", body: "tick"}')" >/dev/null || return 1
	done
}
check "the MCP server accepts each tick of the 8 load sessions" ticks "$load8" 8

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
check "lane start keeps an existing lane with its work" eq "$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start t-01-a 2>/dev/null && cat "$lp/edit.txt")" "$lp
C"
out=$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" patch t-01-a 2>&1)
check "lane patch counts the files of the ticket" contains "$out" "(3 files)"
ROOT="$tmp/ln" LANES="$lanes" sh "$lane" apply t-01-a >/dev/null 2>&1
group "lane apply brings each change of the ticket"
row "a changed file" eq "$(cat "$tmp/ln/edit.txt")" C
row "a new file" test -f "$tmp/ln/new.txt"
row "a deletion" not test -e "$tmp/ln/gone.txt"
end_group
check "lane apply keeps the dirty work of the shared tree" eq "$(cat "$tmp/ln/keep.txt")" dirty
check "lane apply leaves the index of the shared tree clean" eq "$(git -C "$tmp/ln" diff --cached --name-only)" ""
# shellcheck disable=SC2016 # the inner shell expands $1, $2, and $3
check "after clean, lane start makes a new copy" sh -c 'ROOT="$1" LANES="$2" sh "$3" start t-05-e >/dev/null && printf x >"$2/t-05-e/edit.txt" && ROOT="$1" LANES="$2" sh "$3" clean t-05-e >/dev/null && ROOT="$1" LANES="$2" sh "$3" start t-05-e >/dev/null && ! grep -qx x "$2/t-05-e/edit.txt"; r=$?; ROOT="$1" LANES="$2" sh "$3" clean t-05-e >/dev/null; exit $r' _ "$tmp/ln" "$lanes" "$lane"
# shellcheck disable=SC2016 # the inner shell expands $1, $2, and $3
check "lane clean removes the lane and the patch" sh -c 'ROOT="$1" LANES="$2" sh "$3" clean t-01-a >/dev/null && ! test -e "$2/t-01-a" && ! test -e "$2/t-01-a.patch"' _ "$tmp/ln" "$lanes" "$lane"
group "lane refuses a bad ticket ID or command"
row "../x, the rule of a leading dot" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start ../x
row ".x, the rule of a leading dot" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start .x
row "a/x, the rule of a character outside A-Za-z0-9._-" refused "bad ticket ID: a/x" env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start a/x
row "an unknown command" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" merge t-02-b
end_group
check "lane refuses a ROOT that is not the top of a work tree" not env ROOT="$tmp/ln/node_modules" LANES="$lanes" sh "$lane" start t-02-b
check "lane apply refuses a missing patch" not env ROOT="$tmp/ln" LANES="$lanes" sh "$lane" apply t-09-z
: >"$lanes/t-08-y.patch"
check "lane apply of an empty patch says so" contains "$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" apply t-08-y 2>&1)" "empty patch"
# A pattern of EXCLUDES is for rsync, not for the shell of the current folder.
printf 'x\n' >"$tmp/ln/gl1"
mkdir -p "$tmp/cwdx" && : >"$tmp/cwdx/glzz"
lp=$(cd "$tmp/cwdx" && ROOT="$tmp/ln" LANES="$lanes" EXCLUDES='gl*' sh "$lane" start t-04-d 2>/dev/null)
# shellcheck disable=SC2016 # the inner shell expands $1
check "the shell does not expand a pattern of EXCLUDES in the current folder" sh -c 'test -d "$1" && ! test -e "$1/gl1"' _ "$lp"
ROOT="$tmp/ln" LANES="$lanes" sh "$lane" clean t-04-d >/dev/null
rm -f "$tmp/ln/gl1"
# A nested repository: no nested .git in the lane, and its edits reach the patch.
git init -q "$tmp/ln/sub" && printf 's\n' >"$tmp/ln/sub/f.txt"
lp=$(ROOT="$tmp/ln" LANES="$lanes" sh "$lane" start t-03-c 2>/dev/null)
# shellcheck disable=SC2016 # the inner shell expands $1
check "lane start copies a nested repository without its .git" sh -c 'test -f "$1/sub/f.txt" && ! test -e "$1/sub/.git"' _ "$lp"
printf 'S\n' >"$lp/sub/f.txt"
ROOT="$tmp/ln" LANES="$lanes" sh "$lane" patch t-03-c >/dev/null 2>&1
check "an edit in a nested repository reaches the patch" grep -q 'sub/f.txt' "$lanes/t-03-c.patch"
ROOT="$tmp/ln" LANES="$lanes" sh "$lane" clean t-03-c >/dev/null
rm -rf "$tmp/ln/sub"
# A linked worktree has a .git pointer file; the lane must not share its index.
git -C "$tmp/ln" -c user.name=t -c user.email=t@example.com commit -qam work
git -C "$tmp/ln" worktree add -q "$tmp/lnw" -b lnw
lp=$(ROOT="$tmp/lnw" LANES="$lanes" sh "$lane" start t-02-b 2>/dev/null)
rm "$lp/keep.txt"
ROOT="$tmp/lnw" LANES="$lanes" sh "$lane" patch t-02-b >/dev/null 2>&1
check "a lane of a linked worktree leaves the index of the worktree clean" eq "$(git -C "$tmp/lnw" status --porcelain)" ""
# shellcheck disable=SC2016 # the inner shell expands $1, $2, and $3
check "a lane of a linked worktree applies a deletion" sh -c 'ROOT="$1" LANES="$2" sh "$3" apply t-02-b >/dev/null && ! test -e "$1/keep.txt"' _ "$tmp/lnw" "$lanes" "$lane"
# A kept lane must belong to the same ROOT, HEAD, and run (fix round 3, R1).
mkrepo() {
	git init -q "$1" && printf '%s\n' "$2" >"$1/g.txt" && git -C "$1" add g.txt &&
		git -C "$1" -c user.name=t -c user.email=t@example.com commit -q -m base
}
mkrepo "$tmp/ra" a && mkrepo "$tmp/rb" b
rl="$tmp/rlanes"
la=$(ROOT="$tmp/ra" LANES="$rl" sh "$lane" start mr1-01-schema 2>/dev/null)
printf 'edit of A\n' >"$la/g.txt"
lb=$(ROOT="$tmp/rb" LANES="$rl" sh "$lane" start mr1-01-schema 2>/dev/null)
check "a lane of another repository is not kept" eq "$(cat "$lb/g.txt")" b
ROOT="$tmp/rb" LANES="$rl" sh "$lane" patch mr1-01-schema >/dev/null 2>&1
ROOT="$tmp/rb" LANES="$rl" sh "$lane" apply mr1-01-schema >/dev/null 2>&1
printf 'edit of B\n' >"$lb/g.txt"
check "a lane of the same ROOT, HEAD, and run is kept" eq "$(ROOT="$tmp/rb" LANES="$rl" sh "$lane" start mr1-01-schema >/dev/null 2>&1; cat "$lb/g.txt")" "edit of B"
group "a lane of another run, HEAD, or base is not kept"
row "another run" eq "$(ROOT="$tmp/rb" LANES="$rl" LANE_RUN=other sh "$lane" start mr1-01-schema >/dev/null 2>&1; cat "$lb/g.txt")" b
printf 'edit of run other\n' >"$lb/g.txt"
git -C "$tmp/rb" -c user.name=t -c user.email=t@example.com commit -q --allow-empty -m next
row "another HEAD" eq "$(ROOT="$tmp/rb" LANES="$rl" LANE_RUN=other sh "$lane" start mr1-01-schema >/dev/null 2>&1; cat "$lb/g.txt")" b
printf 'edit again\n' >"$lb/g.txt"
git -C "$lb" update-ref -d refs/lane/base
row "no refs/lane/base" eq "$(ROOT="$tmp/rb" LANES="$rl" LANE_RUN=other sh "$lane" start mr1-01-schema >/dev/null 2>&1; cat "$lb/g.txt")" b
row "the remade lane has refs/lane/base again" git -C "$lb" rev-parse -q --verify refs/lane/base
end_group
# A patch is applied once. Four identical blocks give repeated context: plain git apply
# inserts the same line a second time at an offset, and lane.sh apply does not.
for _ in 1 2 3 4; do printf 'x\ny\nz\nw\n'; done >"$tmp/ra/g.txt" && git -C "$tmp/ra" -c user.name=t -c user.email=t@example.com commit -qam blocks
ROOT="$tmp/ra" LANES="$rl" sh "$lane" clean mr1-02-rep >/dev/null
lr=$(ROOT="$tmp/ra" LANES="$rl" sh "$lane" start mr1-02-rep 2>/dev/null)
awk 'NR == 7 { print "inserted" } { print }' "$lr/g.txt" >"$lr/g.new" && mv "$lr/g.new" "$lr/g.txt"
ROOT="$tmp/ra" LANES="$rl" sh "$lane" patch mr1-02-rep >/dev/null 2>&1
cp -R "$tmp/ra" "$tmp/ra-plain"
git -C "$tmp/ra-plain" apply "$rl/mr1-02-rep.patch" && git -C "$tmp/ra-plain" apply "$rl/mr1-02-rep.patch" 2>/dev/null
group "a patch applies once"
row "plain git apply of the patch twice inserts the line twice (the reproduction)" eq "$(grep -c inserted "$tmp/ra-plain/g.txt")" 2
ROOT="$tmp/ra" LANES="$rl" sh "$lane" apply mr1-02-rep >/dev/null 2>&1
ROOT="$tmp/ra" LANES="$rl" sh "$lane" apply mr1-02-rep >/dev/null 2>&1
row "lane.sh apply twice inserts the line once" eq "$(grep -c inserted "$tmp/ra/g.txt")" 1
end_group
printf 'more\n' >>"$lr/g.txt"
ROOT="$tmp/ra" LANES="$rl" sh "$lane" patch mr1-02-rep >/dev/null 2>&1
group "an applied lane refuses a changed patch"
row "the apply fails" not env ROOT="$tmp/ra" LANES="$rl" sh "$lane" apply mr1-02-rep
row "the refused patch changes nothing" eq "$(grep -c more "$tmp/ra/g.txt")" 0
end_group
# The root half of the lane meta: a clone at the same HEAD is another root.
git clone -q "$tmp/rb" "$tmp/rc"
ROOT="$tmp/rb" LANES="$rl" sh "$lane" clean mr1-03-root >/dev/null
lo=$(ROOT="$tmp/rb" LANES="$rl" sh "$lane" start mr1-03-root 2>/dev/null)
printf 'edit of rb\n' >"$lo/g.txt"
check "a lane of another root at the same HEAD is not kept" eq "$(ROOT="$tmp/rc" LANES="$rl" sh "$lane" start mr1-03-root >/dev/null 2>&1; cat "$lo/g.txt")" b

# The waiter of spec 9.5 (M1): scripts/wake.sh, in a temporary data folder.
wake="$here/../plugins/bruh/scripts/wake.sh"
wd="$tmp/wake"
mkdir -p "$wd"
out=$(unset BRUH_ROLE_KEY; CLAUDE_PLUGIN_DATA="$wd" sh "$wake" </dev/null 2>&1; echo "exit $?")
group "the waiter without BRUH_ROLE_KEY does nothing"
row "exits 0" eq "$out" "exit 0"
row "writes no pid file" not test -e "$wd/wake"
end_group
# wake_bg <key> <name>: starts a waiter in the background; its exit code goes to $tmp/<name>.code.
wake_bg() {
	(
		BRUH_ROLE_KEY=$1 CLAUDE_PLUGIN_DATA="$wd" BRUH_WAKE_POLL=1 BRUH_WAKE_SECONDS=30 sh "$wake" </dev/null 2>"$tmp/$2.err"
		echo $? >"$tmp/$2.code"
	) &
}
# wait_code <name>: waits up to 10 seconds for the exit code of a waiter.
wait_code() {
	i=0
	while [ ! -s "$tmp/$1.code" ] && [ "$i" -lt 100 ]; do
		sleep 0.1
		i=$((i + 1))
	done
	cat "$tmp/$1.code" 2>/dev/null
}
wake_bg clerk-app-t1 w1
sleep 1
wake_bg clerk-app-t1 w2
wake_bg clerk-app-t2 w3
check "a second waiter of the same role key replaces the first, which exits 0" eq "$(wait_code w1)" 0
sleep 1
mkdir -p "$wd/mail/clerk-app-t1"
echo '{}' >"$wd/mail/clerk-app-t1/0001-a.json"
check "the waiter exits 2 on new mail" eq "$(wait_code w2)" 2
check "the waiter tells the session to read its mail" contains "$(cat "$tmp/w2.err")" "new mail for clerk-app-t1. Call mail_read."
sleep 2
check "mail for another role key does not wake the waiter" not test -s "$tmp/w3.code"
echo '{}' >"$wd/mail/clerk-app-t2/0002-b.json"
check "the waiter of the other role key wakes on its own mail" eq "$(wait_code w3)" 2
out=$(BRUH_ROLE_KEY=clerk-app-t1 CLAUDE_PLUGIN_DATA="$wd" BRUH_WAKE_POLL=1 BRUH_WAKE_SECONDS=1 sh "$wake" </dev/null 2>&1; echo "exit $?")
check "mail that a waiter already reported does not wake the next one" not contains "$out" "Call mail_read"
check "the waiter exits 0 in silence at its limit, with no wake" eq "$out" "exit 0"
mkdir -p "$wd/reports" "$wd/mail/clerk-app-t4"
echo '{"at":"x","from":"clerk-app-t4","kind":"status","text":"task closed"}' >"$wd/reports/clerk-app-t4.jsonl"
echo '{}' >"$wd/mail/clerk-app-t4/0001-a.json"
out=$(BRUH_ROLE_KEY=clerk-app-t4 CLAUDE_PLUGIN_DATA="$wd" BRUH_WAKE_POLL=1 BRUH_WAKE_SECONDS=30 sh "$wake" </dev/null 2>&1; echo "exit $?")
check "the waiter of a closed task exits 0 at once, also with new mail" eq "$out" "exit 0"
echo '{"at":"y","from":"clerk-app-t4","kind":"status","text":"started"}' >>"$wd/reports/clerk-app-t4.jsonl"
out=$(BRUH_ROLE_KEY=clerk-app-t4 CLAUDE_PLUGIN_DATA="$wd" BRUH_WAKE_POLL=1 BRUH_WAKE_SECONDS=1 sh "$wake" </dev/null 2>&1; echo "exit $?")
group "a report line after task closed wakes the waiter again"
row "exit 2" contains "$out" "exit 2"
row "tells the session to call mail_read" contains "$out" "Call mail_read"
end_group

echo "$n tests, $fails failed"
[ "$fails" -eq 0 ]
