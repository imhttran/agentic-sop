# Retry Requeues Legal Blocked Work

## ID

PREJEV016-S2

## Objective

Verify that retry requeues a task only when retry is legal for its blocked status,
preserves task history so the retry resumes the same task, and refuses completed,
merged, or non-blocked statuses. Scope is limited to `internal/cli` (`retry.go`)
and `internal/domain` (`Requeue`). Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented. Existing correct
behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestRunRetryRequeuesBlocked`, `TestRunRetryRejectsCompleted`, `TestRunRetryRejectsNotBlocked`, `TestRunRetryUnknownTask`, `TestRunRetryBudgetExhausted`, `TestRunRetryForceRaisesBudget`, `TestRunRetryAll`, `TestRunRetryAllNothingBlocked`, `TestRequeue`, `TestRequeueWithoutSpending`, and `TestRequeueBudgetExhausted`.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'Retry'` and `go test ./internal/domain/ -run 'Requeue|Transitions|Terminal'`.
- Run the broader validation: `go test ./...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.

## Acceptance Criteria

- retry requeues a retryable BLOCKED task to PLANNED and preserves its task history (same task id).
- retry refuses a completed, merged, or non-blocked task.
- retry reports an exhausted retry budget instead of requeueing silently; `--force` raises the budget deliberately.
- retry never requires deleting `state.db`.
- the focused and broader validation commands pass.
- completion is recorded either as no change (`changes_expected=false`) or as a single focused test that closes a demonstrated gap.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Keep all new tests deterministic and free of network access.

## Dependencies

- none

## Execution

- verify-first
