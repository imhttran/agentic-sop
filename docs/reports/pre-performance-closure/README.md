# Pre-Performance Closure — Reports Index

**Type:** Descriptive index

Index of the evidence for the executed closure plan
[PLAN-Pre-Performance-Closure.md](../../plans/PLAN-Pre-Performance-Closure.md)
(complete: `CLOSE-001…CLOSE-011` are `LOCAL_DONE`). The single readiness verdict,
including the two explicit deferrals, is
[CLOSE-011-readiness.md](CLOSE-011-readiness.md).

## Final artifacts

| Artifact | Role |
| --- | --- |
| [CLOSE-001-baseline.md](CLOSE-001-baseline.md) | Source/config/toolchain baseline. |
| [CLOSE-003-sop-deterministic-baseline.md](CLOSE-003-sop-deterministic-baseline.md) | SOP deterministic baseline (all gates green). |
| [CLOSE-004-controller-deterministic-baseline.md](CLOSE-004-controller-deterministic-baseline.md) | Controller baseline (read-only). |
| [CLOSE-008-telemetry-inventory.md](CLOSE-008-telemetry-inventory.md) | Telemetry inventory. |
| [CLOSE-009-performance-baseline.md](CLOSE-009-performance-baseline.md) + [raw JSON](CLOSE-009-performance-baseline-raw.json) | Measurements (`UNAVAILABLE`), deferred. |
| [CLOSE-011-readiness.md](CLOSE-011-readiness.md) | Readiness verdict. |
| [workloads/](workloads/) | Frozen A/B/C/D fixtures and [build-pin.md](workloads/build-pin.md). |
| [../PERFORMANCE-BASELINE.md](../PERFORMANCE-BASELINE.md) | Published CLOSE-010 baseline. |

## Superseded interim reports (moved to history)

These recorded a point-in-time verdict from an earlier attempt and are superseded;
each carries a "Final status" banner. They now live under
[`../../history/pre-performance-closure/`](../../history/pre-performance-closure/).

| Artifact | Interim verdict (superseded) | Current state |
| --- | --- | --- |
| [CLOSE-002-status-reconciliation.md](../../history/pre-performance-closure/CLOSE-002-status-reconciliation.md) | twelve CLI/JEV failures `BLOCKED`; CLOSE-010 in backlog | Resolved — tests pass, baseline published. |
| [CLOSE-005-controller-work-verdicts.md](../../history/pre-performance-closure/CLOSE-005-controller-work-verdicts.md) | controller work `NEEDS_HUMAN`; gate not re-run | See [CLOSE-011](CLOSE-011-readiness.md) §2–§3. |
| [CLOSE-006-named-plan-dogfood.md](../../history/pre-performance-closure/CLOSE-006-named-plan-dogfood.md) | `NEEDS_HUMAN` (controller dogfood) | Deferred ([CLOSE-011](CLOSE-011-readiness.md) §0.2, §4). |
| [CLOSE-007-resume-idempotency.md](../../history/pre-performance-closure/CLOSE-007-resume-idempotency.md) | controller-root re-run `NEEDS_HUMAN` | Deferred ([CLOSE-011](CLOSE-011-readiness.md) §3). |
