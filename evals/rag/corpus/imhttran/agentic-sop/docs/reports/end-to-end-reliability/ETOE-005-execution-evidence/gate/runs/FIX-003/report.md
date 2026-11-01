# Run: FIX-003 — Human-gated fixture task

- Provider: `ollama`
- Review engine: `self`
- Stage: `WAITING_FOR_HUMAN`
- Fix cycles: 0/3
- Gate: `NEEDS_HUMAN`

## Task

# FIX-003 — Human-gated fixture task

Reach the human approval boundary. The controlled provider declares a destructive,
authorization-required operation, so SOP records a pending human approval and
stops without approving it.

## Acceptance criteria
- SOP records a pending human approval for the task and does not approve it.

## Validation

Validation checks configured but NOT RUN: the lifecycle stopped before validation.

- NOT RUN BUILD `go build ./...`
- NOT RUN UNIT_TEST `go test ./...`
- NOT RUN LINT `go vet ./...`

## Review

No findings.

## Gate

- the requested fixture operation is destructive and irreversible, so it requires authorization

## Classification

- Kind: `DESTRUCTIVE_OPERATION`
- Disposition: `NEEDS_HUMAN`
- Confidence: `HIGH`
- Reason: the agent reported a boundary that requires a human: the requested fixture operation is destructive and irreversible, so it requires authorization

## Autonomy

- Autonomy: `BALANCED`
- Risk: `IRREVERSIBLE`
- Decision: `HUMAN_APPROVAL_REQUIRED`
- Human approval required
- Reason: the agent reported a boundary that requires a human: the requested fixture operation is destructive and irreversible, so it requires authorization
