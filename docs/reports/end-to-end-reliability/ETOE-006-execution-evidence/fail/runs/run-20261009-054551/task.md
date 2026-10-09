# FIX-FAIL

Introduce the intentional failure so the deterministic quality gate reports FAIL.

The enabling step (renaming `broken_test.go.disabled` to `broken_test.go`) is
performed by scripts/etoe-005-fixture-run.sh before this task runs. The test
`TestBroken` always fails, so `sop validate` (go test) fails deterministically.
