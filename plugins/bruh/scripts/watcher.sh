#!/bin/sh
# Runs the bruh watcher: polls the code hosts of <plugin data>/repos.json and prints
# one JSON line for each event. Each event also goes to the report file of its project,
# reports/clanker-<project>.jsonl. bigm runs it in a Monitor. Pass --data <folder> when
# the plugin data folder is not the default.
GOTOOLCHAIN=local exec go run -C "$(dirname -- "$0")/../mcp" . watch "$@"
