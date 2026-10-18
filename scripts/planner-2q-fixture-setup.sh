#!/usr/bin/env bash
#
# Planner two-question live demo — disposable fixture setup.
#
# Creates a disposable SOP project OUTSIDE any production checkout for the live
# two-question planner demo with a model-driven executor
# (docs/testing/PLANNER-TWO-QUESTION-LIVE.md). The project holds one task whose
# two implementation details (ordering, then duplicate handling) must be
# clarified with the planner, and an operator-owned acceptance test that checks
# normalized.txt against whatever choices the planner made.
#
# Usage:
#   scripts/planner-2q-fixture-setup.sh [FIXTURE_ROOT] [SOP_BINARY]
#
# Environment:
#   PLANNER2Q_FIXTURE_ROOT   overrides FIXTURE_ROOT (default: ${TMPDIR:-/tmp}/planner-2q-live)
#   PLANNER2Q_SOP_BINARY     overrides SOP_BINARY   (default: sop on PATH)
#   PLANNER2Q_MODEL          executor model         (default: deepseek-v4.1-flash:cloud)
#   PLANNER2Q_FORCE_RECREATE set to 1 to remove and recreate an existing fixture root
#
# The SOP binary must include the decision-history fix (imhttran/agentic-sop#68);
# otherwise the final round sees only the latest planner decision.
#
# Safety: the fixture root must be a strict descendant of ${TMPDIR:-/tmp} and must
# not be inside this checkout. Logs belong outside the fixture root so they never
# enter the task's change set.

set -euo pipefail

BASE_ROOT="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P || echo /tmp)"
FIXTURE_ROOT="${1:-${PLANNER2Q_FIXTURE_ROOT:-${BASE_ROOT}/planner-2q-live}}"
SOP_BINARY="${2:-${PLANNER2Q_SOP_BINARY:-sop}}"
MODEL="${PLANNER2Q_MODEL:-deepseek-v4.1-flash:cloud}"

SELF_DIR="$(cd "$(dirname "$0")" && pwd -P)"
# The checkout this script runs from, if any. A copy run from outside a git
# checkout (for example from $TMPDIR) has none, and the in-checkout guard is skipped.
CHECKOUT_ROOT="$(git -C "$SELF_DIR" rev-parse --show-toplevel 2>/dev/null || true)"
if [ -n "$CHECKOUT_ROOT" ]; then
  CHECKOUT_ROOT="$(cd "$CHECKOUT_ROOT" && pwd -P)"
fi

# Resolve the fixture root physically through its nearest existing ancestor.
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

case "$FIXTURE_ROOT" in
  "$BASE_ROOT"/*) : ;;
  *) echo "setup: fixture root must be under $BASE_ROOT: $FIXTURE_ROOT" >&2; exit 1 ;;
esac
if [ -n "$CHECKOUT_ROOT" ]; then
  case "$FIXTURE_ROOT/" in
    "$CHECKOUT_ROOT"/*) echo "setup: refusing a fixture root inside the checkout: $FIXTURE_ROOT" >&2; exit 1 ;;
  esac
fi

if [ -e "$FIXTURE_ROOT" ]; then
  if [ "${PLANNER2Q_FORCE_RECREATE:-0}" = "1" ]; then
    echo "setup: removing existing fixture root $FIXTURE_ROOT (PLANNER2Q_FORCE_RECREATE=1)"
    rm -rf "$FIXTURE_ROOT"
  else
    echo "setup: fixture root already exists: $FIXTURE_ROOT" >&2
    echo "setup: set PLANNER2Q_FORCE_RECREATE=1 to recreate it" >&2
    exit 1
  fi
fi

command -v "$SOP_BINARY" >/dev/null 2>&1 || { echo "setup: sop binary not found: $SOP_BINARY" >&2; exit 1; }
SOP_PATH="$(command -v "$SOP_BINARY")"
command -v git >/dev/null 2>&1 || { echo "setup: git is required" >&2; exit 1; }
command -v go >/dev/null 2>&1 || { echo "setup: go is required for the acceptance check" >&2; exit 1; }

echo "setup: fixture root = $FIXTURE_ROOT"
echo "setup: sop binary   = $SOP_PATH"
echo "setup: model        = $MODEL"

mkdir -p "$FIXTURE_ROOT"
cd "$FIXTURE_ROOT"
git init -q -b main

# 1. Initialize the SOP project, then replace the config with the fixture's own.
"$SOP_PATH" init
CONFIG=".agent-sdlc/config.yaml"
[ -f "$CONFIG" ] || { echo "setup: sop init did not create $CONFIG" >&2; exit 1; }

cat > "$CONFIG" <<EOF
version: 1

project:
  name: planner-2q-live
  integration_branch: main

agent:
  harness: tool
  provider: ollama
  model: ${MODEL}

planner:
  enabled: true
  mode: external
  round_limit: 3

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

# 2. Go module and the operator-owned acceptance test. The test accepts either
#    answer to each question: it checks normalized.txt against input.txt under
#    the choices recorded in choices.txt.
cat > go.mod <<'EOF'
module planner2qfixture

go 1.21
EOF

cat > doc.go <<'EOF'
// Package normalize holds the acceptance check for the planner two-question demo.
package normalize
EOF

printf 'banana\napple\ncherry\napple\nbanana\n' > input.txt

cat > normalize_test.go <<'EOF'
package normalize

import (
	"os"
	"sort"
	"strings"
	"testing"
)

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func choice(t *testing.T, line, key string) bool {
	t.Helper()
	switch line {
	case key + "=true":
		return true
	case key + "=false":
		return false
	}
	t.Fatalf("choices.txt line %q, want %s=true or %s=false", line, key, key)
	return false
}

func TestNormalized(t *testing.T) {
	choices := readLines(t, "choices.txt")
	if len(choices) != 2 {
		t.Fatalf("choices.txt has %d lines, want 2 (sort=..., dedupe=...)", len(choices))
	}
	sorted := choice(t, choices[0], "sort")
	dedupe := choice(t, choices[1], "dedupe")

	want := readLines(t, "input.txt")
	if dedupe {
		seen := map[string]bool{}
		kept := want[:0:0]
		for _, l := range want {
			if !seen[l] {
				seen[l] = true
				kept = append(kept, l)
			}
		}
		want = kept
	}
	if sorted {
		sort.Strings(want)
	}

	got := readLines(t, "normalized.txt")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("normalized.txt = %q, want %q (sort=%t dedupe=%t)", got, want, sorted, dedupe)
	}
}
EOF

# 3. The plan. The task names the needs_planner outcome and when to use it. A
#    softer wording ("clarify with the planner") let the model decide both choices
#    itself in the first live attempt, and an "operator preference" framing made
#    the earlier planner-real attempt return needs_human. This demo tests SOP's
#    multi-round mechanics, not whether a model notices ambiguity unprompted.
cat > PLAN.md <<'EOF'
# Implementation Plan

## Project

Planner two-question live demo

## Summary

Normalize a word list. Two implementation choices are decided by the planner, one at a time, during implementation.

## T001 — Normalize the word list

Write normalized.txt from input.txt, and record the choices used in choices.txt.

Two implementation choices are not yours to make. The planner decides them, one at a time, through the `needs_planner` outcome (not `needs_human`):

1. Ordering: sort the lines alphabetically (`sort=true`), or preserve the input order (`sort=false`).
2. Duplicates: remove repeated lines, keeping the first occurrence (`dedupe=true`), or keep them (`dedupe=false`).

During implementation:

- If your input has no planner decision about ordering, return `needs_planner` asking only the ordering question, with the two options above. Write no files.
- If your input has the ordering decision but none about duplicates, return `needs_planner` asking only the duplicates question, with the two options above. Write no files.
- Only when your input contains both planner decisions, write choices.txt and normalized.txt.

Never choose either value yourself, and never ask both questions at once. The planner's decisions arrive in your input as `Planner decision:` lines.

choices.txt has exactly two lines: `sort=true` or `sort=false`, then `dedupe=true` or `dedupe=false`.

This is a single implementation stage; do not split the questions into separate stages.

### Dependencies

None

### Deliverables

- normalized.txt
- choices.txt

### Acceptance Criteria

- normalized.txt matches input.txt under the choices recorded in choices.txt

### Acceptance Checks

- `go test ./...`

### Acceptance Mapping

- criterion 1: check 1
EOF

# 4. Validate the config, prove the acceptance test fails before the task runs,
#    and commit the starting state.
"$SOP_PATH" status >/dev/null || { echo "setup: config failed to load (sop status)" >&2; exit 1; }
if go test ./... >/dev/null 2>&1; then
  echo "setup: the acceptance test passed before the task ran; the fixture is wrong" >&2
  exit 1
fi
git add -A
git -c user.name=planner-2q -c user.email=planner-2q@localhost commit -q -m "planner two-question fixture"

echo "setup: ready. Next steps are in docs/testing/PLANNER-TWO-QUESTION-LIVE.md:"
echo "  cd $FIXTURE_ROOT && $SOP_PATH run PLAN.md"
