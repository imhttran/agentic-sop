# PREJEV018-S3 — IMPLEMENT Deterministic

Establish/confirm deterministic regression coverage for the IMPLEMENT capability.

## Objective

Verify that a productive IMPLEMENT returns before the hard ceiling, mutation-aware
completion enables deterministic finalization, and the emitted outcome stays
SOP-compatible — producing the same result on repeated runs with no unexplained
nondeterminism.

## Dependencies

- PREJEV018-S1

## Requirements

- Inspect `internal/ollamaagent` (`implement.go`, `outcome.go`) and its harness
  tests plus the deterministic `internal/e2e` harness.
- Verify existing coverage first; add a test only where a real gap is demonstrated.
- Recorded evidence must show stable, repeatable results, including
  `validation runs > 0` for an end-to-end implementation.

## Acceptance Criteria

- IMPLEMENT has deterministic regression coverage that can be re-run
- repeated IMPLEMENT regression runs produce the same result with no unexplained nondeterminism
- mutation-aware completion yields deterministic finalization and an SOP-compatible outcome
- IMPLEMENT regression coverage passes against the reconciled Harness V2
- the focused and broader validation commands pass

## Validation

``` bash
go test ./internal/ollamaagent/ -run 'Implement|IMPLEMENT|Outcome|Reconcile'
go test ./internal/ollamaagent/... ./internal/e2e/...
```

## Execution

- verify-first
