# PREJEV018-S7 — Controller Aligned

Align the controller with SOP and make controller documentation match reality.

## Objective

Verify that `sop-controller` delegates orchestration to SOP, carries no duplicate
model/tool loop or second state machine, and that its documentation matches
observed controller behavior.

## Dependencies

- PREJEV018-S5
- PREJEV018-S6

## Requirements

- Inspect the controller's configuration/documentation and the architecture
  boundary in `docs/ARCHITECTURE.md`.
- Verify existing coverage first; record any documentation/behavior mismatch and
  its resolution in the readiness gate's remaining-issues register (do not silently
  edit either side).
- Confirm human approval gates are unaffected by controller delegation.

## Acceptance Criteria

- the controller delegates orchestration to SOP
- controller documentation matches actual controller behavior
- human approval gates are unaffected by controller delegation
- any controller documentation mismatch is recorded with its resolution
- the focused and broader validation commands pass

## Validation

``` bash
go vet ./... && go build ./...
```

## Execution

- verify-first
