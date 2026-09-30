#!/bin/sh
# Runs the bruh merge train: merge-train.sh [--data <folder>] [--wait-minutes <n>] <owner/repo> <number>...
# It merges the pull requests in order, each only with green checks, and confirms
# each merge by reading the code host API.
GOTOOLCHAIN=local exec go run -C "$(dirname -- "$0")/../mcp" . merge-train "$@"
