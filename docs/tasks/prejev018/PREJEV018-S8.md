# PREJEV018-S8 — Recovery Proven

Prove that recovery works without destructive state manipulation.

## Objective

Verify that recovery works end to end for the exercised scenarios (interrupted run
and failed stage recovery) and requires no deletion of `.agent-sdlc`, `state.db`,
or run history, and no `git reset --hard` or `git clean`.

## Dependencies

- PREJEV018-S6
- PREJEV018-S7

## Requirements

- Inspect `docs/history/PREJEV016-RECOVERY-DECOMPOSITION.md`, `internal/cli/recovery_test.go`,
  and `internal/resume`.
- Verify existing coverage first; add a test only where a real gap is demonstrated.
- Confirm recovery preserves pre-existing working-tree changes and human approval
  gates and never destructively manipulates SOP state.

## Acceptance Criteria

- recovery works end to end for the exercised scenarios
- recovery requires no destructive state manipulation
- recovery preserves human approval gates and SOP state
- the focused and broader validation commands pass

## Validation

``` bash
go test ./internal/cli/ -run 'PreservesWorkingTree|Resume|Retry|Active'
go test ./internal/resume/...
```

## Execution

- verify-first
