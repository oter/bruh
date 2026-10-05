#!/bin/sh
# The waiter of spec 9.5 (M1). A Stop hook and a SessionStart hook (startup, resume,
# compact) run it with asyncRewake. It waits for new mail in the mailbox of the role
# key of the session and exits 2, which wakes the session, also when it is idle.
# A newer waiter of the same role key replaces it: the pid file names the newest
# waiter, and each other waiter exits 0 in silence. Before the hook timeout, it exits
# 2 with "no new mail". It does nothing when BRUH_ROLE_KEY is not set.
cat > /dev/null
[ -n "${BRUH_ROLE_KEY:-}" ] && [ -n "${CLAUDE_PLUGIN_DATA:-}" ] || exit 0
key=$BRUH_ROLE_KEY
case "$key" in *[!a-z0-9-]*) exit 0 ;; esac
poll=${BRUH_WAKE_POLL:-2}
limit=${BRUH_WAKE_SECONDS:-3300}
case "$poll" in '' | 0 | *[!0-9]*) poll=2 ;; esac
case "$limit" in '' | *[!0-9]*) limit=3300 ;; esac
box="$CLAUDE_PLUGIN_DATA/mail/$key"
dir="$CLAUDE_PLUGIN_DATA/wake"
mkdir -p "$box" "$dir" || exit 0
pidf="$dir/$key.pid"
seen="$dir/$key.seen"
echo "$$" > "$pidf"
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
	if [ "$waited" -ge "$limit" ]; then
		echo "bruh: no new mail for $key." >&2
		exit 2
	fi
	sleep "$poll"
	waited=$((waited + poll))
done
