#!/bin/sh
# Fail-closed isolation preflight for benchmark checkouts.
#
# Usage: isolate.sh <source-repo> <target-dir>
#
# Copies <source-repo> to <target-dir> only after verifying that <target-dir> is a
# separate, safe location. It aborts with a non-zero status BEFORE any write when a
# precondition fails, so a misconfigured target can never modify the source
# repository. On success it prints the absolute target path to stdout, which the
# caller must change into before any write or SOP execution:
#
#   dest=$(scripts/bench/isolate.sh "$SRC" "$DEST") && cd "$dest"
#
# The preflight verifies: the source is a git repository; the target's parent
# exists; the target is neither the source nor inside it; and the target does not
# already exist.
set -eu

if [ "$#" -ne 2 ]; then
	echo "isolate: usage: isolate.sh <source-repo> <target-dir>" >&2
	exit 2
fi

src=$1
dst=$2

if [ ! -d "$src/.git" ]; then
	echo "isolate: source is not a git repository: $src" >&2
	exit 1
fi

src_abs=$(cd "$src" && pwd -P)

dst_parent=$(dirname "$dst")
if [ ! -d "$dst_parent" ]; then
	echo "isolate: target parent does not exist: $dst_parent" >&2
	exit 1
fi

dst_parent_abs=$(cd "$dst_parent" && pwd -P)
dst_abs=$dst_parent_abs/$(basename "$dst")

case "$dst_abs" in
"$src_abs" | "$src_abs"/*)
	echo "isolate: target must be outside the source repository: $dst_abs" >&2
	exit 1
	;;
esac

if [ -e "$dst_abs" ]; then
	echo "isolate: target already exists: $dst_abs" >&2
	exit 1
fi

cp -R "$src_abs" "$dst_abs"

if [ ! -d "$dst_abs/.git" ]; then
	echo "isolate: copy is not a git repository: $dst_abs" >&2
	exit 1
fi

echo "$dst_abs"
