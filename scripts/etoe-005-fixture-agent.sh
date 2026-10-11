#!/usr/bin/env bash
#
# ETOE-005 — deterministic controlled agent provider for the dogfood fixture.
#
# This is the "explicitly controlled test provider" the baseline requires: it
# removes the external model from the loop so the fixture's lifecycle is
# deterministic and offline. It speaks SOP's command-agent contract
# (`internal/agent/command.go`): it reads a JSON Request on stdin and writes the
# raw agent output to stdout. SOP sends one Request per capability, so this
# script branches on the request's `capability`:
#
#   PLAN      -> a minimal valid plan JSON (the planner parses the reply)
#   REVIEW    -> a clean review JSON (review.engine: self)
#   IMPLEMENT / FIX / other mutating -> a structured outcome JSON, and for the
#              success/fail modes a REAL repository mutation (rewrite calc.go /
#              append a comment)
#
# Scenario is selected by MODE (arg 1), pinned per task by the run script:
#   success  implement `Add` (real mutation) -> validation must PASS
#   fail     trivial mutation; the run script enables an always-failing test,
#            so validation deterministically FAILS
#   gate     return a needs_human outcome -> SOP stops at the human boundary
#
# No network, no model service, no cloud. Deterministic.
set -euo pipefail

MODE="${1:-success}"

req="$(cat)"
cap="$(printf '%s' "$req" | python3 -c 'import json,sys
try: print(json.load(sys.stdin).get("capability",""))
except Exception: print("")')"

case "$cap" in
  PLAN)
    printf '%s' '{"project":"etoe-005-fixture","summary":"Deterministic ETOE-005 controlled-provider fixture task.","stages":[{"id":"S001","title":"Fixture task","objective":"Satisfy the deterministic fixture task.","dependencies":[],"deliverables":["fixture change"],"acceptance_criteria":["the fixture task completes"]}]}'
    ;;
  REVIEW)
    printf '%s' '{"summary":"clean","findings":[]}'
    ;;
  *)
    # IMPLEMENT / FIX / other mutating capabilities: mode-based behaviour.
    case "$MODE" in
      gate)
        printf '%s' '{"status":"needs_human","reason":"the requested fixture operation is destructive and irreversible, so it requires authorization","summary":"human gate"}'
        ;;
      fail)
        printf '\n// etoe-005 fixture (fail mode): intentional mutation\n' >> calc.go
        printf '%s' '{"status":"completed","reason":"introduced the intentional failure","summary":"fail-mode mutation","changes_expected":true}'
        ;;
      success|*)
        cat > calc.go <<'GO'
package fixture

// Add returns the sum of two integers.
func Add(a, b int) int {
	return a + b
}
GO
        printf '%s' '{"status":"completed","reason":"implemented Add","summary":"success-mode implementation","changes_expected":true}'
        ;;
    esac
    ;;
esac
