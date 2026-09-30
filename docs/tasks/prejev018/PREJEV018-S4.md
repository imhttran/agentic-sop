# PREJEV018-S4 — REVIEW Deterministic

Establish/confirm deterministic regression coverage for the REVIEW capability.

## Objective

Verify that REVIEW follows the inspect-to-synthesize lifecycle (read-only inspect,
bounded synthesis with tools disabled, structured review result) and that repeated
runs produce the same result with no unexplained nondeterminism.

## Dependencies

- PREJEV018-S1

## Requirements

- Inspect `internal/ollamaagent` (`review.go`) and its tests plus the deterministic
  `internal/e2e` harness; cross-check
  `docs/history/PREJEV012-REGRESSION-DECOMPOSITION.md`.
- Verify existing coverage first; add a test only where a real gap is demonstrated.
- Recorded evidence must confirm REVIEW stays read-only, inspection and synthesis
  are bounded, and phase-specific failure diagnostics exist.

## Acceptance Criteria

- REVIEW has deterministic regression coverage that can be re-run
- repeated REVIEW regression runs produce the same result with no unexplained nondeterminism
- REVIEW remains read-only; inspection and synthesis stay bounded; synthesis cannot execute tools
- REVIEW regression coverage passes against the reconciled Harness V2
- the focused and broader validation commands pass

## Validation

``` bash
go test ./internal/ollamaagent/ -run 'Review|REVIEW'
go test ./internal/ollamaagent/... ./internal/e2e/...
```

## Execution

- verify-first
