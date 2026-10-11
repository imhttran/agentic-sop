#!/usr/bin/env bash
#
# ETOE-005 — disposable dogfood fixture setup.
#
# Creates a disposable, `sop init`-initialized SOP project OUTSIDE any
# production checkout, containing exactly three tasks:
#   1. FIX-SUCCESS  — a success task  (expected: completed / LOCAL_DONE)
#   2. FIX-FAIL     — intentionally failing task (expected: validation FAIL, not LOCAL_DONE)
#   3. FIX-GATE     — a human-gated task (expected: a pending human approval)
#
# The fixture root is configurable and defaults to a temp location, so no
# mutation ever lands inside the agentic-sop checkout or its .agent-sdlc state.
#
# Usage:
#   scripts/etoe-005-fixture-setup.sh [FIXTURE_ROOT] [SOP_BINARY]
#
# Environment:
#   ETOE005_FIXTURE_ROOT  overrides FIXTURE_ROOT (default: ${TMPDIR:-/tmp}/etoe-005-fixture)
#   ETOE005_SOP_BINARY    overrides SOP_BINARY    (default: sop on PATH)
#
# Idempotency: re-running setup on an existing fixture root is rejected with a
# clear message unless ETOE005_FORCE_RECREATE=1 is set, in which case the
# existing root is removed and recreated.
#
# Safety: the fixture root must be a strict descendant of the disposable base
# (default ${TMPDIR:-/tmp}). Base and target are fully symlink-resolved (pwd -P)
# before comparison, so a symlinked /tmp (e.g. /tmp -> /private/tmp on macOS) or
# a symlinked checkout cannot defeat the in-checkout guard. The in-checkout
# guard compares the resolved fixture root against the resolved CHECKOUT ROOT
# (the parent of this script's directory), not merely the script directory, so
# a fixture root anywhere inside the production checkout is rejected. The
# script's own path is resolved through a symlink chain so an invoked-via-symlink
# copy still computes the real checkout root.
#
# Determinism: the fixture is driven by a controlled, offline agent
# (scripts/etoe-005-fixture-agent.sh) selected by the run script via
# SOP_AGENT_HARNESS=command / SOP_AGENT_PROVIDER=command / SOP_AGENT_COMMAND, so
# no external model service is required and the lifecycle is deterministic. The
# setup ships a STUB `Add` so the success scenario requires a real mutation.
#
# Config policy: `sop init` writes a complete, version-appropriate
# `.agent-sdlc/config.yaml`. This script INTENTIONALLY replaces that file with
# the fixture's own complete, deterministic configuration (see the heredoc
# below) so validation commands are byte-for-byte reproducible. The written file
# follows the documented schema in docs/reference/CONFIGURATION.md and is
# validated immediately by `sop status`.

set -euo pipefail

BASE_ROOT="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P || echo /tmp)"

FIXTURE_ROOT="${1:-${ETOE005_FIXTURE_ROOT:-${BASE_ROOT}/etoe-005-fixture}}"
SOP_BINARY="${2:-${ETOE005_SOP_BINARY:-sop}}"

# Resolve this script's real path (follow a symlink chain) so CHECKOUT_ROOT is
# correct even when the script is invoked through a symlink elsewhere.
self_path="$0"
while [ -h "$self_path" ]; do
  link="$(ls -ld -- "$self_path" | sed 's/.* -> //')"
  case "$link" in
    /*) self_path="$link" ;;
    *) self_path="$(dirname "$self_path")/$link" ;;
  esac
done
SELF_DIR="$(cd "$(dirname "$self_path")" && pwd -P)"
CHECKOUT_ROOT="$(cd "$SELF_DIR/.." && pwd -P)"

# Fully resolve the fixture root (nearest existing ancestor, physically).
_resolve() {
  local p="$1" rest=""
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

# Refuse a path that is not a strict descendant of the disposable base root.
if ! is_descendant "$FIXTURE_ROOT" "$BASE_ROOT"; then
  if [ "$FIXTURE_ROOT" = "$BASE_ROOT" ]; then
    echo "setup: refusing to use the disposable base root itself: $FIXTURE_ROOT" >&2
  else
    echo "setup: refusing fixture root outside the disposable base root ($BASE_ROOT): $FIXTURE_ROOT" >&2
    echo "setup: set ETOE005_FIXTURE_ROOT to a path under $BASE_ROOT" >&2
  fi
  exit 1
fi

# Refuse to write anywhere inside the agentic-sop production checkout.
if [ "$FIXTURE_ROOT" = "$CHECKOUT_ROOT" ] || is_descendant "$FIXTURE_ROOT" "$CHECKOUT_ROOT"; then
  echo "setup: refusing to use a fixture root inside the production checkout: $FIXTURE_ROOT" >&2
  exit 1
fi

if [ -e "$FIXTURE_ROOT" ]; then
  if [ "${ETOE005_FORCE_RECREATE:-0}" = "1" ]; then
    echo "setup: removing existing fixture root $FIXTURE_ROOT (ETOE005_FORCE_RECREATE=1)"
    rm -rf "$FIXTURE_ROOT"
  else
    echo "setup: fixture root already exists: $FIXTURE_ROOT" >&2
    echo "setup: set ETOE005_FORCE_RECREATE=1 to recreate it, or run teardown first" >&2
    exit 1
  fi
fi

command -v "$SOP_BINARY" >/dev/null 2>&1 || {
  echo "setup: sop binary not found: $SOP_BINARY" >&2
  exit 1
}
SOP_PATH="$(command -v "$SOP_BINARY")"

# Verify the CLI surfaces the fixture depends on BEFORE use. This probe uses
# `sop help` (which never executes a plan) and runs from the disposable base
# root, so a probe can never launch a run against a real project: a bare
# `sop run` with no arguments would execute the ACTIVE plan, which the fixture
# must never do.
help_out="$(cd "$BASE_ROOT" && "$SOP_PATH" help 2>&1 || true)"
case "$help_out" in
  *--task*) : ;;
  *) echo "setup: '$SOP_PATH' does not advertise a --task run surface" >&2
     echo "setup: use the source build: go build -o /tmp/sop-etoe005 ./cmd/sop" >&2
     exit 1 ;;
esac
command -v git >/dev/null 2>&1 || { echo "setup: git is required for change detection" >&2; exit 1; }

echo "setup: fixture root = $FIXTURE_ROOT"
echo "setup: sop binary   = $SOP_PATH"

mkdir -p "$FIXTURE_ROOT"/{tasks,docs}
cd "$FIXTURE_ROOT"

# 1. Initialize the disposable SOP project.
if ! "$SOP_PATH" init; then
  echo "setup: sop init failed (exit $?)" >&2
  exit 1
fi

# 1a. Assert initialization actually created the state database and config.
STATE_DB=".agent-sdlc/state.db"
CONFIG=".agent-sdlc/config.yaml"
for required in "$STATE_DB" "$CONFIG"; do
  if [ ! -f "$required" ]; then
    echo "setup: sop init did not create $required; initialization failed" >&2
    exit 1
  fi
done
echo "setup: sop init created $STATE_DB and $CONFIG"

# 2. Write the fixture's complete, deterministic configuration.
cat > "$CONFIG" <<'EOF'
version: 1

project:
  name: etoe-005-fixture
  integration_branch: main

validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - go vet ./...

review:
  engine: self
  delegation: false

quality:
  require_tests: true
  max_fix_cycles: 3
  fail_on:
    - critical
    - high

human:
  approval_before_commit: true

workflow:
  mode: local
EOF

# 3. Validate the config immediately (machine-checked shape).
if ! "$SOP_PATH" status >/dev/null 2>&1; then
  echo "setup: generated config failed to load (sop status); see $CONFIG" >&2
  "$SOP_PATH" status || true
  exit 1
fi
echo "setup: config validated by sop status"

# 4. Minimal Go module. `Add` is a STUB (returns 0) while calc_test expects 5,
#    so the success scenario requires a real implementation mutation.
cat > go.mod <<'EOF'
module etoe005fixture

go 1.21
EOF

cat > calc.go <<'EOF'
package fixture

// Add is an intentional STUB so the success task must implement it.
func Add(a, b int) int {
	return 0
}
EOF

cat > calc_test.go <<'EOF'
package fixture

import "testing"

func TestAdd(t *testing.T) {
	if got := Add(2, 3); got != 5 {
		t.Fatalf("Add(2,3) = %d, want 5", got)
	}
}
EOF

# 5. A deliberate, deterministic validation failure, shipped DISABLED.
cat > broken_test.go.disabled <<'EOF'
package fixture

import "testing"

// TestBroken is intentionally failing; the gate must report FAIL.
func TestBroken(t *testing.T) {
	t.Fatalf("intentional ETOE-005 validation failure")
}
EOF

# 6. Declare the three fixture tasks and their EXPECTED outcomes before execution.
cat > tasks/FIXTURE.md <<'EOF'
# ETOE-005 disposable fixture

Three tasks with expected outcomes declared before execution:

| Task        | Kind             | Expected outcome                          |
| ----------- | ---------------- | ----------------------------------------- |
| FIX-SUCCESS | success          | completed / LOCAL_DONE (validation PASS)  |
| FIX-FAIL    | intentional fail | validation FAIL; run does NOT complete    |
| FIX-GATE    | human-gated      | a pending human approval (needs_human)    |
EOF

cat > tasks/FIX-SUCCESS.md <<'EOF'
# FIX-SUCCESS

Implement `Add` in calc.go and keep the tests passing.

## Expected outcome (recorded before execution)

completed / LOCAL_DONE with validation PASS.
EOF

cat > tasks/FIX-FAIL.md <<'EOF'
# FIX-FAIL

Introduce the intentional failure so the deterministic quality gate reports FAIL.

The enabling step (renaming `broken_test.go.disabled` to `broken_test.go`) is
performed by scripts/etoe-005-fixture-run.sh before this task runs. The test
`TestBroken` always fails, so `sop validate` (go test) fails deterministically.

## Expected outcome (recorded before execution)

validation FAIL; the run does not reach LOCAL_DONE.
EOF

cat > tasks/FIX-GATE.md <<'EOF'
# FIX-GATE

Reach the human approval boundary. In this fixture the controlled provider
returns a `needs_human` outcome, so SOP stops at the approval boundary without
bypassing it.

## Expected outcome (recorded before execution)

a pending human approval request (needs_human) that only a human can resolve.
EOF

# A deterministic, plan-based human-gate fixture: SOP compiles this plan without
# an agent (rendered shape) and persists the task, so a genuine approval gate is
# recorded in state.db and visible via `sop approvals --json`. (A `--task` run
# stores no task, so it cannot record a queryable approval gate.)
cat > docs/PLAN.md <<'EOF'
# PLAN — ETOE-005 gate fixture

## Project

ETOE-005 gate fixture

## Summary

Deterministic human-gate scenario: the controlled provider returns a destructive,
authorization-required outcome, so SOP must stop at a genuine approval gate.

## FIX-003 — Human-gated fixture task

Reach the human approval boundary. The controlled provider declares a destructive,
authorization-required operation, so SOP records a pending human approval and
stops without approving it.

### Dependencies

None

### Acceptance Criteria

- SOP records a pending human approval for the task and does not approve it.
EOF

# 7. Initialize a git repository so SOP's working-tree change detection has a
#    baseline. The fixture is disposable and is never committed to or pushed.
git init -q
git config user.email "fixture@example.invalid"
git config user.name "ETOE-005 fixture"
git config commit.gpgsign false
git add -A
git commit -q -m "etoe-005 fixture baseline"

echo "setup: fixture created at $FIXTURE_ROOT"
echo "setup: git baseline: $(git rev-parse --short HEAD)"
echo "setup: sop init layout: $(ls -1 .agent-sdlc 2>/dev/null | tr '\n' ' ')"
echo "setup: tasks: $(ls -1 tasks 2>/dev/null | tr '\n' ' ')"
echo "setup: next: run scripts/etoe-005-fixture-run.sh to execute the three tasks"
