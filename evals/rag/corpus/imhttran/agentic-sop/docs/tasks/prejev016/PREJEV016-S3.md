# Interrupted Active-Task Recovery

## ID

PREJEV016-S3

## Objective

Verify that `sop run` recovers an interrupted active task safely: exactly one
in-flight task is resumed rather than starting another, several in-flight tasks
refuse to proceed with a clear diagnostic naming them, and a scheduled-but-missing
or remote-parked active task is refused safely. Scope is limited to `internal/cli`
(`drive.go`, `run.go`). Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented. Existing correct
behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestRunSelectsPlannedTaskWhenNoActiveTask`, `TestRunResumesInterruptedActiveTask`, `TestRunResumesActiveTaskInsteadOfStartingAnother`, `TestRunCompletesActiveTaskThatAlreadyPassed`, `TestRunRejectsAmbiguousActiveTasks`, `TestRunRefusesRemoteParkedActiveTask`, and `TestRunGraphNeedsHumanRequeues`.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'Active|Interrupted|Resume'`.
- Run the broader validation: `go test ./internal/cli/ -run 'Run'` and `go test ./...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.

## Acceptance Criteria

- exactly one in-flight task is resumed instead of starting another.
- multiple in-flight tasks refuse to proceed with a clear diagnostic naming the tasks.
- a scheduled-but-missing or remote-parked active task is refused precisely rather than guessed.
- recovery of interrupted active work never requires editing or deleting `state.db`.
- the focused and broader validation commands pass.
- completion is recorded either as no change (`changes_expected=false`) or as a single focused test that closes a demonstrated gap.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Keep all new tests deterministic and free of network access.

## Dependencies

- PREJEV016-S1

## Execution

- verify-first
