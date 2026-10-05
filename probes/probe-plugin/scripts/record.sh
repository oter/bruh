#!/bin/sh
# Records one hook event for the bruh probes. Output folder: $PROBE_OUT.
# hooks.tsv gets one short line for each event; hooks.jsonl gets the full hook input.
set -eu
out="${PROBE_OUT:-/tmp/bruh-probes}"
mkdir -p "$out"
input=$(cat)
event=$(printf '%s' "$input" | jq -r '.hook_event_name // "unknown"')
agent_id=$(printf '%s' "$input" | jq -r '.agent_id // ""')
printf '%s\t%s\t%s\t%s\n' "$event" "${BRUH_ROLE_KEY:-}" "$agent_id" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$out/hooks.tsv"
printf '%s' "$input" | jq -c . >> "$out/hooks.jsonl"
exit 0
