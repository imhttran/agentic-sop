# Resume Continues Valid Work

## ID

PREJEV016-S1

## Objective

Verify that `sop resume` continues valid interrupted work: it resolves the single
in-flight task (or the named task id), reports the next legal action through the
shared status-to-action decision, persists the reconciliation, and continues from
the reconciled status instead of restarting or duplicating work. Scope is limited
to `internal/resume` and `internal/cli` (`resume.go`). Do not discover unrelated
subsystems.

This is a verification task: the behavior is already implemented. Existing correct
behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestResolveActionsForEveryStatus`, `TestRecoverExistingBranch`, `TestRecoverExistingPR`, `TestResolvePersistsRecovery`, `TestRecoverySaveFailureIsAtomic`, `TestResolveActive`, `TestRunResumeReportsCreateBranchForReadyTask`, `TestRunResumeReportsNextAction`, `TestRunResumeRecoversExistingBranch`, `TestRunResumeNothingToResume`, `TestRunResumeConsistencyError`, and `TestRunResumeIsIdempotent`.
- Run the focused validation and confirm it passes: `go test ./internal/resume/...` and `go test ./internal/cli/ -run 'Resume'`.
- Run the broader validation: `go test ./...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.

## Acceptance Criteria

- resume resolves the single in-flight task and reports the next legal action.
- a persisted recovery means a subsequent resume continues from the advanced status.
- resume with no in-flight task reports nothing to resume without mutating state.
- resume reconciles an existing branch or pull request instead of recreating it.
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
