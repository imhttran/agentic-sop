#!/usr/bin/env bash
#
# ETOE-005 — disposable dogfood fixture teardown.
#
# Removes ONLY the fixture root created by scripts/etoe-005-fixture-setup.sh.
# It never touches the agentic-sop checkout or its .agent-sdlc state.
#
# Usage:
#   scripts/etoe-005-fixture-teardown.sh [FIXTURE_ROOT]
#
# Environment:
#   ETOE005_FIXTURE_ROOT  overrides FIXTURE_ROOT (default: ${TMPDIR:-/tmp}/etoe-005-fixture)
#
# Reports whether removal succeeded. Idempotent: a missing root is reported and
# exits zero.
#
# Safety: the target must be a strict descendant of the disposable base (default
# ${TMPDIR:-/tmp}); anything else — a production checkout, a home directory, or
# a single-segment path such as /opt — is refused. Both the base and the target
# are fully symlink-resolved (pwd -P) before comparison, so a symlinked /tmp
# (e.g. /tmp -> /private/tmp on macOS) cannot defeat the guard. The descendant
# check compares whole path components against the physically resolved root, so
# it is unaffected by case-insensitive filesystems (macOS default) and does not
# false-reject legitimate names containing a literal '..' substring.

set -euo pipefail

BASE_ROOT="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P || echo /tmp)"

FIXTURE_ROOT="${1:-${ETOE005_FIXTURE_ROOT:-${BASE_ROOT}/etoe-005-fixture}}"

# Fully resolve the fixture root: walk up to the nearest existing ancestor and
# resolve it physically, then re-attach the remaining components. A trailing
# separator is stripped so it cannot defeat the descendant comparison.
_resolve() {
  local p="$1" rest=""
  # Strip trailing separators (except a lone '/').
  while [ "${p%/}" != "$p" ] && [ "$p" != "/" ]; do
    p="${p%/}"
  done
  while [ ! -d "$p" ]; do
    rest="/$(basename "$p")${rest}"
    p="$(dirname "$p")"
    case "$p" in /) break ;; esac
  done
  printf '%s%s' "$(cd "$p" && pwd -P)" "$rest"
}
FIXTURE_ROOT="$(_resolve "$FIXTURE_ROOT")"

# Helper: is PATH a strict descendant of ROOT? Compares only whole path
# components (never a bare substring), so a legitimate path such as /tmp/..foo
# is not rejected, and the check does not depend on case-insensitive shell
# patterns. Both arguments are already physically resolved.
is_descendant() { # is_descendant PATH ROOT
  local path="$1" root="$2"
  [ -n "$root" ] || return 1
  [ "$path" = "$root" ] && return 1
  case "$root" in
    /) [ -n "$path" ] && [ "$path" != "/" ] && return 0 || return 1 ;;
  esac
  case "$path" in
    "$root"/*) return 0 ;;
    *) return 1 ;;
  esac
}

if ! is_descendant "$FIXTURE_ROOT" "$BASE_ROOT"; then
  if [ "$FIXTURE_ROOT" = "$BASE_ROOT" ]; then
    echo "teardown: refusing to remove the disposable base root itself: $FIXTURE_ROOT" >&2
  else
    echo "teardown: refusing unsafe path outside the disposable base root ($BASE_ROOT): $FIXTURE_ROOT" >&2
    echo "teardown: set ETOE005_FIXTURE_ROOT to a path under $BASE_ROOT" >&2
  fi
  exit 1
fi

if [ ! -e "$FIXTURE_ROOT" ]; then
  echo "teardown: fixture root already absent: $FIXTURE_ROOT"
  exit 0
fi

rm -rf "$FIXTURE_ROOT"

if [ -e "$FIXTURE_ROOT" ]; then
  echo "teardown: FAILED to remove $FIXTURE_ROOT" >&2
  exit 1
fi

echo "teardown: removed $FIXTURE_ROOT"