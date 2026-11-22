#!/usr/bin/env bash
# docs-only.sh — decide whether a change touches only documentation that no check reads.
#
# Reads changed repository paths on stdin, one per line, and prints "true" when every
# path is under docs/, is not Go source or a Go module file, and no tracked non-Markdown
# file outside docs/ names it. Tests and scripts that read a doc name it (for example
# ../../docs/plans/PLAN-*.md, or an evidence JSON's file name in a filepath.Join), so
# such a doc counts as code. A mention does not count on a comment line, or when the
# line names a different docs/ path. Prints "false" otherwise, and for an empty list.
# An error exits non-zero; CI then runs everything.
# shortcut: a doc read through a computed name (no literal file name outside docs/) is
# missed, and a shared name (docs/README.md) always counts as code; switch to an
# explicit manifest of the docs that checks read if either starts to matter.
#
# Usage: git diff --no-renames --name-only BASE HEAD | scripts/checks/docs-only.sh
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."

no() { echo "docs-only: $*" >&2; echo false; exit 0; }

n=0
while IFS= read -r path; do
  [ -n "$path" ] || continue
  n=$((n + 1))
  case "$path" in docs/*) ;; *) no "$path is outside docs/" ;; esac
  case "$path" in *.go|*/go.mod|*/go.sum) no "$path is Go source" ;; esac
  rc=0
  # The self-check names docs as test cases; it reads none of them.
  hits="$(git grep -nF -- "${path##*/}" -- ':!docs/' ':!*.md' ':!scripts/checks/test-docs-only.sh')" || rc=$?
  [ "$rc" -le 1 ] || { echo "docs-only: git grep failed ($rc)" >&2; exit 2; }
  hit="$(printf '%s\n' "$hits" | awk -v p="$path" '
    $0 == "" { next }
    { line = $0; sub(/^[^:]*:[0-9]+:/, "", line) }
    line ~ /^[ \t]*(\/\/|#|\*)/ { next }
    index(line, "docs/") && !index(line, p) { next }
    { print; exit }')"
  [ -z "$hit" ] || no "$path is named by ${hit%%:*}"
done
[ "$n" -gt 0 ] || no "no changed files"
echo true
