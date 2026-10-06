#!/bin/sh
# The waiter of spec 9.5 (M1). A Stop hook and a SessionStart hook (startup, resume,
# compact) run it with asyncRewake. It waits for new mail in the mailbox of the role
# key of the session and exits 2, which wakes the session, also when it is idle.
# A newer waiter of the same role key replaces it: the pid file names the newest
# waiter, and each other waiter exits 0 in silence. At its limit (BRUH_WAKE_SECONDS,
# default 604500, below the hook timeout of 604800 seconds), it exits 0 in silence:
# only new mail wakes the session. It does nothing when BRUH_ROLE_KEY is not set.
# It exits 0 at once when the last report line of the role is the status "task closed".
cat > /dev/null
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
key=$BRUH_ROLE_KEY
case "$key" in *[!a-z0-9-]*) exit 0 ;; esac
poll=${BRUH_WAKE_POLL:-2}
limit=${BRUH_WAKE_SECONDS:-604500}
case "$poll" in '' | 0 | *[!0-9]*) poll=2 ;; esac
case "$limit" in '' | *[!0-9]*) limit=604500 ;; esac
box="$CLAUDE_PLUGIN_DATA/mail/$key"
dir="$CLAUDE_PLUGIN_DATA/wake"
mkdir -p "$box" "$dir" || exit 0
pidf="$dir/$key.pid"
seen="$dir/$key.seen"
echo "$$" > "$pidf"
# ponytail: a reused task key does not wake on mail until its new clerk writes a report line;
# a scout writes no "task closed" line, so its clanker stops it with claude stop.
tail -n 1 "$CLAUDE_PLUGIN_DATA/reports/$key.jsonl" 2> /dev/null | grep -Fq '"kind":"status","text":"task closed"' && exit 0
waited=0
while :; do
	[ "$(cat "$pidf" 2> /dev/null)" = "$$" ] || exit 0
	names=$(cd "$box" && for f in *.json; do [ -e "$f" ] && printf '%s\n' "$f"; done)
	for f in $names; do
		if ! grep -Fqx -- "$f" "$seen" 2> /dev/null; then
			printf '%s\n' "$names" > "$seen"
			echo "bruh: new mail for $key. Call mail_read." >&2
			exit 2
		fi
	done
	[ "$waited" -lt "$limit" ] || exit 0
	sleep "$poll"
	waited=$((waited + poll))
done
