# PREJEV018-S9 — Both Projects Dogfooded

Dogfood both repositories until both build and test successfully using the
reconciled harness.

## Objective

Record that both `agentic-sop` and `sop-controller` build and test successfully
under the reconciled Harness V2 and SOP-owned validation, using representative SOP
dogfood workflows.

## Dependencies

- PREJEV018-S8

## Requirements

- Run, for each repository, the final-validation commands from the plan.
- Record per-repository build and test results (and representative dogfood runs).
- Verify existing coverage first; change nothing when the results already hold.
- Do not fabricate CI/PR/merge results.

## Acceptance Criteria

- both repositories build successfully
- both repositories test successfully
- dogfooding uses the reconciled Harness V2 and SOP-owned validation
- the recorded results cover both repositories

## Validation

``` bash
# per repository
gofmt -l .
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

## Execution

- verify-first
