#!/usr/bin/env bash
# Self-check for scripts/checks/docs-only.sh against real paths in this repository.
#
# Usage: scripts/checks/test-docs-only.sh
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
fail() { echo "FAIL: $*" >&2; exit 1; }

expect() {
  local want="$1" input="$2" got
  got="$(printf '%s' "$input" | bash "$root/scripts/checks/docs-only.sh" 2>/dev/null)"
  [ "$got" = "$want" ] || fail "want $want, got '$got' for: $(printf '%s' "$input" | tr '\n' ' ')"
}

new_doc="docs/reports/docs-only-self-check-never-named.md"
expect true  "$new_doc"$'\n'
expect true  "$new_doc"$'\n'"docs/plans/another-never-named-plan.md"$'\n'
expect false ""
expect false "README.md"$'\n'
expect false "internal/cli/cli.go"$'\n'
expect false "$new_doc"$'\n'"internal/cli/cli.go"$'\n'
expect false "docs/reports/pre-performance-closure/workloads/A/sum.go"$'\n'
# Read by internal/planner tests, so a change to it must run them.
expect false "docs/plans/PLAN-Security-Scanner-Addon.md"$'\n'
expect false "$new_doc"$'\n'"docs/history/plans/PLAN-Phase-3-OpenJEV.md"$'\n'
# Read through filepath.Join(..., "docs", ...) with the file name as a constant.
expect false "docs/reports/owasp-llm/llm02-c1-linux-campaign.json"$'\n'
expect false "docs/reports/evidence/JEV-WAVE2-DOGFOOD.json"$'\n'
# Named only in comments and Markdown, so editing it cannot change a check.
expect true  "docs/plans/PROPOSAL-SHARED-EXECUTION-EXCLUSION.md"$'\n'
echo "docs-only self-check: OK"
