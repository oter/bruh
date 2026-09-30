#!/bin/sh
# Runs the bruh watcher: polls the code hosts of <plugin data>/repos.json and prints
# one JSON line for each event. bigm runs it in a Monitor. Pass --data <folder> when
# the plugin data folder is not the default.
GOTOOLCHAIN=local exec go run -C "$(dirname -- "$0")/../mcp" . watch "$@"
