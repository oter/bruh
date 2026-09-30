#!/bin/sh
# Lane tooling for parallel ticket agents: one private copy of the tree for
# each ticket, and its delta patched back onto the shared working tree.
#
#   ROOT=<repo> [LANES=<folder>] [EXCLUDES="<paths>"] lane.sh start <ticket>   copy the tree, record the baseline, print the path
#   ROOT=<repo> [LANES=<folder>] lane.sh patch <ticket>   write the delta of the ticket to $LANES/<ticket>.patch
#   ROOT=<repo> [LANES=<folder>] lane.sh apply <ticket>   apply that patch onto the shared tree
#   ROOT=<repo> [LANES=<folder>] lane.sh clean <ticket>   remove the copy and the patch
#
# The copy gets its own `git init` with the objects and the HEAD of ROOT. A
# linked worktree has a .git pointer file, and a copied pointer shares the real
# index, so .git is never copied. `start` stages the baseline and records its
# tree as refs/lane/base. `patch` stages everything and diffs the index against
# that tree, so the delta holds the work of the agent whatever it staged itself,
# deletions included. The lane borrows the objects of ROOT through alternates:
# `git gc --prune=now` or `git prune` in ROOT while a lane is live can break it.
# `apply` is a plain `git apply`, not --3way: three-way needs a clean index, and
# the shared tree is dirty on purpose between milestone commits.
# Each path of EXCLUDES is anchored at the top of the tree, so `.bin/` does not
# drop node_modules/.bin.
set -eu
die() {
	printf 'lane.sh: %s\n' "$*" >&2
	exit 2
}
ROOT=${ROOT:-}
[ -n "$ROOT" ] || die "ROOT (the absolute path of the repository) must be set"
LANES=${LANES:-${TMPDIR:-/tmp}/bruh-lanes}
LANES=${LANES%/}
EXCLUDES=${EXCLUDES:-.scratch/}
[ $# -eq 2 ] || die "usage: lane.sh start|patch|apply|clean <ticket>"
cmd=$1
t=$2
case $t in
'' | .* | -* | *[!A-Za-z0-9._-]*) die "bad ticket ID: $t" ;;
esac
dir=$LANES/$t
case $cmd in
start)
	top=$(git -C "$ROOT" rev-parse --show-toplevel 2>/dev/null) || die "ROOT is not a git work tree: $ROOT"
	[ "$(cd "$top" && pwd -P)" = "$(cd "$ROOT" && pwd -P)" ] || die "ROOT is not the top level of a work tree: $ROOT"
	mkdir -p "$LANES"
	rm -rf "$dir"
	set -- --exclude /.git
	for e in $EXCLUDES; do set -- "$@" --exclude "/${e#/}"; done
	rsync -a "$@" "$ROOT/" "$dir/"
	git -C "$dir" init -q --object-format="$(git -C "$ROOT" rev-parse --show-object-format)"
	common=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir)
	echo "$common/objects" >"$dir/.git/objects/info/alternates"
	[ ! -f "$common/info/exclude" ] || cp "$common/info/exclude" "$dir/.git/info/exclude"
	git -C "$dir" update-ref HEAD "$(git -C "$ROOT" rev-parse HEAD)"
	git -C "$dir" add -A >/dev/null
	git -C "$dir" update-ref refs/lane/base "$(git -C "$dir" write-tree)"
	echo "$dir"
	;;
patch)
	[ -d "$dir/.git" ] || die "no lane for $t in $LANES"
	git -C "$dir" add -A >/dev/null
	git -C "$dir" diff --cached --binary refs/lane/base -- . ':(exclude).scratch' >"$dir.patch"
	echo "$dir.patch ($(grep -c '^diff --git' "$dir.patch" || true) files)"
	;;
apply)
	if [ ! -s "$dir.patch" ]; then
		echo "empty patch for $t"
		exit 0
	fi
	git -C "$ROOT" apply --whitespace=nowarn "$dir.patch"
	echo "applied $t"
	;;
clean)
	rm -rf "$dir" "$dir.patch"
	echo "cleaned $t"
	;;
*) die "unknown command: $cmd" ;;
esac
