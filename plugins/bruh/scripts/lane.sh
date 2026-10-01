#!/bin/sh
# Lane tooling for parallel ticket agents: one private copy of the tree for
# each ticket, and its delta patched back onto the shared working tree.
#
#   ROOT=<repo> [LANES=<folder>] [LANE_RUN=<key>] [EXCLUDES="<paths>"] lane.sh start <ticket>
#       copy the tree, record the baseline, print the path. An existing lane is kept
#       with its work only when it was made for the same ROOT, the same HEAD of ROOT,
#       and the same LANE_RUN; else it is wiped and made again.
#   ROOT=<repo> [LANES=<folder>] lane.sh patch <ticket>   write the delta of the ticket to $LANES/<ticket>.patch
#   ROOT=<repo> [LANES=<folder>] lane.sh apply <ticket>   apply that patch onto the shared tree, once
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
# drop node_modules/.bin. `.git` is excluded at every depth, so the files of a
# nested repository are plain files of the lane and their edits reach the patch.
# `apply` refuses a missing patch file: a merge that skipped `patch` must not look
# like an empty ticket, because `clean` then deletes the work. After an apply, the
# lane holds a marker with the checksum of the patch: the same patch is not applied
# a second time (plain `git apply` can match repeated context at an offset and
# apply it twice), and a changed patch of an applied lane is refused.
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
	# Keep a lane of this ROOT, this HEAD, and this run: the attempt after a
	# question, or a relaunch, goes on with the work of the earlier attempt. A lane
	# of another repository, another commit, or another run is wiped.
	meta=$(printf 'root=%s\nbase=%s\nrun=%s' "$(cd "$ROOT" && pwd -P)" "$(git -C "$ROOT" rev-parse HEAD)" "${LANE_RUN:-}")
	if [ -d "$dir/.git" ] && git -C "$dir" rev-parse -q --verify refs/lane/base >/dev/null &&
		[ "$(cat "$dir/.git/lane-meta" 2>/dev/null)" = "$meta" ]; then
		echo "$dir"
		exit 0
	fi
	mkdir -p "$LANES"
	rm -rf "$dir"
	set -f # EXCLUDES holds paths, not globs for this shell
	set -- --exclude .git
	for e in $EXCLUDES; do set -- "$@" --exclude "/${e#/}"; done
	set +f
	rsync -a "$@" "$ROOT/" "$dir/"
	git -C "$dir" init -q --object-format="$(git -C "$ROOT" rev-parse --show-object-format)"
	common=$(git -C "$ROOT" rev-parse --path-format=absolute --git-common-dir)
	echo "$common/objects" >"$dir/.git/objects/info/alternates"
	[ ! -f "$common/info/exclude" ] || cp "$common/info/exclude" "$dir/.git/info/exclude"
	git -C "$dir" update-ref HEAD "$(git -C "$ROOT" rev-parse HEAD)"
	git -C "$dir" add -A >/dev/null
	git -C "$dir" update-ref refs/lane/base "$(git -C "$dir" write-tree)"
	printf '%s' "$meta" >"$dir/.git/lane-meta"
	echo "$dir"
	;;
patch)
	[ -d "$dir/.git" ] || die "no lane for $t in $LANES"
	git -C "$dir" add -A >/dev/null
	git -C "$dir" diff --cached --binary refs/lane/base -- . ':(exclude).scratch' >"$dir.patch"
	echo "$dir.patch ($(grep -c '^diff --git' "$dir.patch" || true) files)"
	;;
apply)
	[ -f "$dir.patch" ] || die "no patch for $t in $LANES: run patch first"
	if [ ! -s "$dir.patch" ]; then
		echo "empty patch for $t"
		exit 0
	fi
	sum=$(cksum <"$dir.patch")
	if [ -f "$dir/.git/lane-applied" ]; then
		[ "$(cat "$dir/.git/lane-applied")" = "$sum" ] || die "$t was applied already with another patch; check the shared tree, then clean the lane"
		echo "already applied $t; skipped"
		exit 0
	fi
	git -C "$ROOT" apply --whitespace=nowarn "$dir.patch"
	[ ! -d "$dir/.git" ] || printf '%s' "$sum" >"$dir/.git/lane-applied"
	echo "applied $t"
	;;
clean)
	rm -rf "$dir" "$dir.patch"
	echo "cleaned $t"
	;;
*) die "unknown command: $cmd" ;;
esac
