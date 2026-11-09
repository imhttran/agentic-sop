#!/usr/bin/env bash
# pre-push.sh — the fast gates a push must pass, in one place.
#
# Why: CI evaluates a pull request *merged into its base branch*, not the branch
# alone. A relative documentation link can resolve on the branch and still be broken
# in the merge result, which makes the failure invisible locally (PRs #295 and #296
# both failed this way). This script checks the merge result before the push exists.
#
# Usage: scripts/checks/pre-push.sh [options]
#   --base REF   base branch to merge against (default: origin/main)
#   --fix        rewrite unformatted Go files with gofmt -w instead of failing
#   --no-merge   check the working tree instead of the merge with --base
#   -h, --help   show this help
#
# Gates:
#   1. formatting     gofmt -l .            (the same gate CI's gofmt step runs)
#   2. documentation  check-doc-links.sh    (evaluated on the merge with --base)
#
# The merge result is materialized with `git archive` into a temporary directory.
# Nothing in the working tree, the index, or the repository state is modified, and
# the temporary directory is removed on exit. When the base ref cannot be resolved
# (a shallow CI checkout, for example) the working tree is checked instead and the
# reason is printed, so the gate never silently disappears.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
root="$(cd "$script_dir/../.." && pwd)"

base="origin/main"
fix=0
merge=1

usage() {
  sed -n '2,/^set -euo/p' "$0" | sed 's/^# \{0,1\}//; $d'
}

while [ "$#" -gt 0 ]; do
  case "$1" in
    --base)
      [ "$#" -ge 2 ] || { echo "pre-push: --base needs a value" >&2; exit 2; }
      base="$2"; shift 2 ;;
    --fix) fix=1; shift ;;
    --no-merge) merge=0; shift ;;
    -h|--help) usage; exit 0 ;;
    *)
      echo "pre-push: unknown argument: $1 (see --help)" >&2
      exit 2 ;;
  esac
done

cd "$root"

status=0

# --- 1. Formatting ----------------------------------------------------------
if ! command -v gofmt >/dev/null 2>&1; then
  echo "pre-push: gofmt is not on PATH" >&2
  exit 2
fi

unformatted="$(gofmt -l .)"
if [ -n "$unformatted" ] && [ "$fix" = "1" ]; then
  echo "pre-push: formatting with gofmt -w:"
  printf '%s\n' "$unformatted"
  while IFS= read -r file; do
    [ -n "$file" ] || continue
    gofmt -w "$file"
  done <<EOF
$unformatted
EOF
  unformatted="$(gofmt -l .)"
fi
if [ -n "$unformatted" ]; then
  echo "pre-push: these files are not gofmt-formatted:"
  printf '%s\n' "$unformatted"
  echo "pre-push: run 'make check-push-fix' (or scripts/checks/pre-push.sh --fix) and commit the result." >&2
  status=1
fi

# --- 2. Documentation links -------------------------------------------------
link_scope="the working tree"

if [ "$merge" = "1" ]; then
  if ! git rev-parse --verify --quiet "$base^{commit}" >/dev/null 2>&1; then
    echo "pre-push: base '$base' is unavailable; checking the working tree instead" >&2
    merge=0
  fi
fi

tree=""
if [ "$merge" = "1" ]; then
  if merge_out="$(git merge-tree --write-tree HEAD "$base" 2>&1)"; then
    candidate="$(printf '%s\n' "$merge_out" | head -n 1)"
    if git rev-parse --verify --quiet "$candidate^{tree}" >/dev/null 2>&1; then
      tree="$candidate"
    else
      echo "pre-push: git cannot produce a merge tree; checking the working tree instead" >&2
    fi
  else
    echo "pre-push: HEAD does not merge cleanly with $base:" >&2
    printf '%s\n' "$merge_out" >&2
    echo "pre-push: merge $base into the branch, resolve, and retry." >&2
    exit 1
  fi
fi

tmp=""
cleanup() {
  [ -n "$tmp" ] || return 0
  rm -rf -- "$tmp"
}
trap cleanup EXIT

if [ -n "$tree" ]; then
  tmp="$(mktemp -d)"
  git archive "$tree" | tar -x -C "$tmp"
  link_root="$tmp"
  link_scope="the merge of HEAD with $base"
else
  link_root="$root"
fi

if ! bash "$link_root/scripts/checks/check-doc-links.sh" "$link_root"; then
  status=1
fi

if [ "$status" -ne 0 ]; then
  echo "pre-push: FAILED (formatting; documentation links on $link_scope)" >&2
  exit 1
fi

echo "pre-push: OK (formatting; documentation links on $link_scope)"
