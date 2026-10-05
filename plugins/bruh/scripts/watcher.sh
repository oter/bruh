#!/bin/sh
# Runs the bruh poller (spec 9.2 and 9.5): the plugin monitor bruh-poller
# (monitors/monitors.json) starts it in each interactive session, and it polls only
# in bigm. It polls the code hosts of <plugin data>/repos.json and the sources of the
# monitors, once for each source key, and delivers each event to the mailbox of each
# local subscriber and to the report file of its project. It prints a line, which
# reaches bigm as a notification, only for a remote subscriber and for an error.
# A lock in the data folder keeps one poller for each machine.
[ "${BRUH_ROLE_KEY:-}" = bigm ] || exit 0
GOTOOLCHAIN=local exec go run -C "$(dirname -- "$0")/../mcp" . watch "$@"
