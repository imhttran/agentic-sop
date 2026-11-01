# S5V — Verify delivery readiness

## Status

VERIFY

## Objective

Run the final affected checks for delivery: the full suite, race detection on the
isolated target, the macOS isolation boundary (filesystem, secrets, network), the
AISEC suite, the live Promptfoo adapter invocation, and the operating guide.

## Acceptance Criteria

- The full repository suite passes.
- The isolated target passes under the race detector.
- The macOS isolation boundary denies ordinary workspace access, inherited
  secrets, and network access.
- The six deterministic AISEC checks pass.
- One real pinned-Promptfoo invocation succeeds through the adapter.
- The operating guide is tracked and documents configuration, reporting, exit
  behavior, outbound behavior, isolation, and platform limitations.

## Acceptance Checks

- go test ./... -count=1
- go test -race ./internal/evaltarget/ -count=1
- go test ./internal/evaltarget/ -run TestAISEC -count=1
- go test ./internal/evaltarget/ -run TestDarwinIsolation -count=1
- go test ./internal/security -run TestPromptfooOperatingGuide -count=1
- go test -tags=promptfoo_live ./internal/security -run TestPromptfooPinnedBinaryVersion -count=1

## Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 4
- criterion 4: check 3
- criterion 5: check 6
- criterion 6: check 5

## Execution

verify-first
