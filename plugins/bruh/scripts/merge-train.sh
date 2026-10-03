#!/bin/sh
# Runs the bruh merge train: merge-train.sh [--data <folder>] [--wait-minutes <n>] [--answer Q-<id>] <owner/repo> <number>...
# Only the merger clerk clerk-<project>-merge with a grant in grants.md, or with a P1 answer of
# bigm in its mailbox (--answer), can run it.
# It merges the pull requests in order, each only with green checks, and confirms
# each merge by reading the code host API.
GOTOOLCHAIN=local exec go run -C "$(dirname -- "$0")/../mcp" . merge-train "$@"
