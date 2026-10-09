# AUTONOMY-001 — production integration: outcome verification and preventive safety

**Status:** VERIFIED (authorized scope). **Branch:** `feature/autonomy-001`. No commit/push.

## What was implemented

- **Production outcome verification (R1).** `internal/outcome` is now integrated into the
  governed lifecycle (`internal/cli/outcome.go` + a few additive lines in `internal/cli/run.go`).
  After the quality gate, SOP derives an objective-level verdict from the **authorized task
  specification** — never from a model completion claim. Determinants available on this branch:
  the gate's deterministic build/test/lint/review verdict, and the existence of each declared
  deliverable that names a repository-relative path. The verdict (VERIFIED / PARTIAL / HOLD) is
  written to `outcome.json`, rendered in `report.md`, printed as an `outcome:` summary line, and
  included in `report.json`. `PASS`/`LOCAL_DONE` is therefore never equated with verified
  completion. A task that declares no objective is unchanged (the step is a no-op).
- **Preventive safety (R2).** Reused the existing `toolharness` authorization and audit
  mechanism, proven by a new no-side-effect regression: protected-path writes, out-of-scope path
  escapes, and destructive commands are refused **before** effect, leave no file change, and are
  recorded in the audit.
- **Autonomous decisions (R3).** No change required; proven reuse of existing components —
  `internal/adaptiveroute` (evidence/cost-driven, smallest capable class, never downgrades,
  deterministic reasons), `internal/decision` (bounded, fail-closed on unknown choices), and
  `internal/autonomy` (risk-based, reversible low-risk auto-fix; escalation only for boundaries).

## Files

| File | Change |
|---|---|
| `internal/cli/outcome.go` | new — outcome adapter (`deriveOutcome`, artifact/summary/report writers) |
| `internal/cli/run.go` | +18/−3 — additive wiring: result field, artifact, report section, summary line |
| `internal/cli/outcome_integration_test.go` | new — R1 regressions (VERIFIED / missing-deliverable PARTIAL / free-text PARTIAL) |
| `internal/toolharness/preventive_safety_test.go` | new — R2 no-side-effect regression |

No new framework; no second engine; `harden-001` not merged; no production behavior change for
tasks without a declared objective.

## Validation

`gofmt -l .` clean · `go vet ./...` OK · `go build ./...` OK · `go test -count=1 ./...` all ok ·
`go test -race -count=1 ./...` all ok · `git diff --check` clean. Acceptance tests
(`internal/outcome`, `TestOutcomeIntegration*`, `TestAutonomous*`, `TestDeniedOperationsCauseNoSideEffect`)
green.

## Remaining (honest scope limit)

Generic per-criterion verification of **free-text** acceptance criteria is not implemented on
this branch (it needs operator-owned verifier bindings — the unmerged `harden-001`). Such criteria
are recorded but unverifiable, so the verdict is PARTIAL rather than VERIFIED. This is by design
and is the reason `PASS` is not treated as proof.
