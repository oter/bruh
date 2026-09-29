#!/bin/sh
# Runs the bruh probes P1 to P3. P4 to P8 are manual; see probes/README.md.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
export PROBE_OUT="${PROBE_OUT:-/tmp/bruh-probes}"
rm -rf "$PROBE_OUT"; mkdir -p "$PROBE_OUT"
work=$(mktemp -d); cd "$work"; git init -q .
settings="$PROBE_OUT/role.json"
printf '{"env":{"BRUH_ROLE_KEY":"clerk-probe-p3"}}' > "$settings"

echo "P1: plugin agent by namespaced name"
if claude -p --plugin-dir "$here/probe-plugin" --agent probe:probe-role "who are you" | grep -q PROBE-ROLE-LOADED; then echo "P1 PASS"; else echo "P1 FAIL"; fi

echo "P2 and P3: background session registration and settings env"
CLAUDE_CODE_FORCE_SESSION_PERSISTENCE=1 claude --bg --plugin-dir "$here/probe-plugin" \
  --name probe-p2 --permission-mode auto --settings "$settings" "Run the shell command: echo hello" > "$PROBE_OUT/bg.txt" 2>&1 || true
sleep 60
if claude agents --json --all | jq -e '.[] | select(.name=="probe-p2")' >/dev/null; then echo "P2 PASS"; else echo "P2 FAIL"; fi
if grep -q 'clerk-probe-p3' "$PROBE_OUT/hooks.tsv" 2>/dev/null; then echo "P3 PASS (hooks get BRUH_ROLE_KEY)"; else echo "P3 FAIL"; fi
id=$(claude agents --json --all | jq -r '.[] | select(.name=="probe-p2") | .id' | head -1)
[ -n "$id" ] && claude stop "$id" >/dev/null 2>&1 || true
echo "Evidence: $PROBE_OUT"
