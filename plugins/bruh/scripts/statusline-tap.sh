#!/bin/sh
# bruh status line tap. init copies this file to <plugin data>/bin/ and sets
# statusLine.command to "'<copy>' '<previous command>'". In a session with
# BRUH_ROLE_KEY, the tap writes the context use to <plugin data>/context/<session_id>.json.
# Then it runs the previous command with the same input, which prints the status line.
data=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd) || exit 0
in=$(mktemp) || exit 0
trap 'rm -f "$in"' EXIT
cat > "$in"
if [ -n "${BRUH_ROLE_KEY:-}" ]; then
	sid=$(jq -r '.session_id // empty' < "$in" 2> /dev/null)
	case "$sid" in
		'' | *[!A-Za-z0-9_-]*) ;;
		*)
			out=$(jq -c --arg at "$(date -u +%Y-%m-%dT%H:%M:%S.000Z)" '
				select(.context_window.used_percentage != null)
				| {used_percentage: .context_window.used_percentage}
				+ (if .rate_limits.five_hour.used_percentage != null
					then {five_hour_percentage: .rate_limits.five_hour.used_percentage} else {} end)
				+ {at: $at}' < "$in" 2> /dev/null)
			if [ -n "$out" ] && mkdir -p "$data/context" && tmp=$(mktemp "$data/context/.tap.XXXXXX"); then
				printf '%s\n' "$out" > "$tmp" && mv "$tmp" "$data/context/$sid.json"
			fi
			;;
	esac
fi
if [ $# -gt 0 ]; then
	sh -c "$1" < "$in"
fi
