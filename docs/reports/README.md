# Reports and Evidence

Point-in-time execution evidence. Reports record **what was observed** during a
run; they are non-normative and never override requirements, specifications, or
architecture. Current behavior is defined under [`../specs/`](../specs/) and
[`../requirements/`](../requirements/).

Authority and lifecycle state remain SOP's: the active plan is the one recorded
in `.agent-sdlc/plan.meta.json` (`sop status` reports it), not the report files
here.

## Contents

### `pre-performance-closure/`

Evidence for the Pre-Performance Closure work, `CLOSE-004`…`CLOSE-011`. The plan is
`COMPLETE` and archived, its document at
[`../history/plans/PLAN-Pre-Performance-Closure-Remaining.md`](../history/plans/PLAN-Pre-Performance-Closure-Remaining.md).

- `CLOSE-004-controller-deterministic-baseline.md`
- `CLOSE-005-controller-verification.md`
- `CLOSE-006-human-decision-dogfood.md`
- `CLOSE-007-resume-idempotency.md`
- `CLOSE-008-performance-telemetry-inventory.md`
- `CLOSE-009-performance-baseline-runs.md` (plus the operator-collected
  `CLOSE-009-operator-measurement-*.{md,json,jsonl}` raw evidence)
- `CLOSE-011-final-readiness.md`

**Kept operational:** the (archived) Pre-Performance Closure plan declares files in
this directory as its task deliverables, and SOP matches deliverables by path, so
this directory is not moved.

### `finish-pre-performance-closure/`

Earlier Pre-Performance Closure evidence: `FP-001`…`FP-009`,
`CLOSE-004-RERUN-READINESS.md`, and
[`READINESS.md`](finish-pre-performance-closure/READINESS.md). Historical; the
plan that produced it is archived under
[`../history/plans/PLAN-Finish-Pre-Performance-Closure.md`](../history/plans/PLAN-Finish-Pre-Performance-Closure.md).

### `PERFORMANCE-BASELINE.md`

The `CLOSE-010` performance baseline report, derived from the `CLOSE-009`
measurement evidence.

## Related

- Historical point-in-time artifacts also live under [`../history/`](../history/).
- Documentation index: [`../README.md`](../README.md).

### Developer Preview

- [`DP-001-real-sop-smoke.md`](DP-001-real-sop-smoke.md) — the Phase 1 real-`sop`
  integration smoke: discovery, project/task views, activity streaming, the
  approvals inbox, and the approve/decline/retry/reconcile delegations, verified
  against the real binary with a deterministic stub agent and a disposable project
  (no model calls; no production SOP state changes).
