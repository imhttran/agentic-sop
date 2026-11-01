# ETOE-002 — Core workflow contract audit

Core workflow contract audit: state transitions, validation/review/FIX/retry,
approval refusal, closure, and trace/metrics. Read-only.

## Deliverables
- `docs/reports/end-to-end-reliability/ETOE-002-workflow-contract-matrix.md` — requirement-to-test matrix with missing coverage identified.

## Acceptance criteria
- A requirement-to-test matrix maps each core workflow transition and each approval-refusal path to existing test coverage.
- Every requirement with no covering test is listed explicitly as a gap.
- The matrix distinguishes behavior verified by unit tests from behavior requiring a real disposable run.
