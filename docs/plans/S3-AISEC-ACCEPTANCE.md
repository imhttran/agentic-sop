# S3V — Verify the AISEC suite against the current evaltarget

## Status

VERIFY

## Objective

Run the affected acceptance checks against the current
`internal/evaltarget/aisec.go` implementation and obtain a current lifecycle
verdict. This task changes no production behavior; it exists because the
original S3 `LOCAL_DONE` evidence predates `aisec.go`.

## Acceptance Criteria

- AISEC-001 through AISEC-006 exist with explicit deterministic assertions.
- The safe authorized case passes without a prohibited effect.
- AISEC-001, 003, 004, and 005 are detectable when the real controls are replaced
  by a permissive control.
- AISEC-002 is detected as missing execution evidence.
- The full evaltarget suite and its race run pass.

## Acceptance Checks

- go test ./internal/evaltarget/ -run 'TestAISEC' -count=1
- go test ./internal/evaltarget/ -count=1
- go test -race ./internal/evaltarget/ -count=1
- go test ./internal/evaltarget/ -run 'TestDarwinIsolation' -count=1

## Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 2
- criterion 5: check 3
- criterion 5: check 4

## Execution

verify-first
