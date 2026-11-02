# PREJEV018-S5 — Validation Ownership Proven

Prove that validation ownership is held by SOP rather than by capability agents.

## Objective

Verify that SOP owns and executes validation, retries, and gates — capability
agents do not self-validate or self-approve — and that human approval gates remain
intact.

## Dependencies

- PREJEV018-S2
- PREJEV018-S3
- PREJEV018-S4

## Requirements

- Inspect SOP's VALIDATE/REVIEW/FIX wiring in `internal/cli` and the gate/transition
  rules in `internal/domain`; cross-check the ownership boundary in
  `docs/architecture/OVERVIEW.md`.
- Verify existing coverage first; add a test only where a real gap is demonstrated.
- Confirm an agent's self-report is never the passing validation.

## Acceptance Criteria

- validation is demonstrably owned and executed by SOP
- no capability performs validation outside SOP ownership, and no agent self-approves
- human approval gates are shown to remain intact
- the focused and broader validation commands pass

## Validation

``` bash
go test ./internal/cli/...
go test ./internal/domain/...
```

## Execution

- verify-first
