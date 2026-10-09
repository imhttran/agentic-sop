# Run: FIX-SUCCESS

- Provider: `ollama`
- Review engine: `self`
- Stage: `PASSED`
- Fix cycles: 0/3
- Gate: `PASS`

## Task

# FIX-SUCCESS

Implement `Add` in calc.go and keep the tests passing.

## Validation

- PASS BUILD `go build ./...`
- PASS UNIT_TEST `go test ./...`
- PASS LINT `go vet ./...`

## Review

clean

## Gate

- all required checks passed
