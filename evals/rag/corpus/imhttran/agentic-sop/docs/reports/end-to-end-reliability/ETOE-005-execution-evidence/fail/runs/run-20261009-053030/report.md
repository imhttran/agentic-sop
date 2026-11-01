# Run: FIX-FAIL

- Provider: `ollama`
- Review engine: `self`
- Stage: `WAITING_FOR_HUMAN`
- Fix cycles: 3/3
- Gate: `NEEDS_HUMAN`

## Task

# FIX-FAIL

Introduce the intentional failure so the deterministic quality gate reports FAIL.

The enabling step (renaming `broken_test.go.disabled` to `broken_test.go`) is
performed by scripts/etoe-005-fixture-run.sh before this task runs. The test
`TestBroken` always fails, so `sop validate` (go test) fails deterministically.

## Validation

- PASS BUILD `go build ./...`
- FAIL UNIT_TEST `go test ./...`

## Review

No findings.

## Gate

- tests failed or were not run
- fix-loop limit reached (3/3)

## Classification

- Kind: `AUTO_FIX_EXHAUSTED`
- Disposition: `NEEDS_HUMAN`
- Confidence: `HIGH`
- Reason: automatic fixes were exhausted (3/3) without resolving the failure: UNIT_TEST `go test ./...`: --- FAIL: TestBroken (0.00s) broken_test.go:7: intentional ETOE-005 validation failure --- FAIL: TestAdd (0.00s) calc_test.go:7: Add(2,3) = 0, want 5 FAIL FAIL etoe005fixture 0.169s FAIL

## Autonomy

- Autonomy: `BALANCED`
- Risk: `MEDIUM`
- Decision: `HUMAN_APPROVAL_REQUIRED`
- Human approval required
- Reason: automatic fixes were exhausted (3/3) without resolving the failure: UNIT_TEST `go test ./...`: --- FAIL: TestBroken (0.00s) broken_test.go:7: intentional ETOE-005 validation failure --- FAIL: TestAdd (0.00s) calc_test.go:7: Add(2,3) = 0, want 5 FAIL FAIL etoe005fixture 0.169s FAIL
