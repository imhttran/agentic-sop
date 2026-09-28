# Non-Destructive Recovery and Working-Tree Preservation

## ID

PREJEV016-S6

## Objective

Verify that recovery preserves pre-existing user changes, does not invoke
`git reset --hard` or `git clean`, and does not delete `.agent-sdlc`, `state.db`,
or run history. Scope is limited to `internal/cli` (the recovery commands `resume`,
`retry`, and the active-task recovery in `run`) and `internal/store`. Do not
discover unrelated subsystems.

This is the one sub-task with a confirmed coverage gap: the existing suite proves
each recovery behavior, but no test asserts that recovery leaves the working tree
and the persisted state intact.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything. The recovery commands are covered by `TestRunResumeRecoversExistingBranch`, `TestRunResumeNothingToResume`, `TestRunRetryRequeuesBlocked`, `TestRunRetryAll`, and `TestRunResumesInterruptedActiveTask`; the tool harness separately rejects destructive git as a tool command in `TestCommandPolicyRejectsDestructiveGitAndOthers`. None of these asserts working-tree or persisted-state preservation, which is the gap.
- Close the gap with the focused tests added in this milestone: `TestRetryPreservesWorkingTreeAndRunHistory`, `TestResumePreservesWorkingTreeAndRunHistory`, and `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory` (`internal/cli/recovery_test.go`). A recovery that ran `git reset --hard` would revert the tracked edit, one that ran `git clean` would delete the untracked file, and one that deleted `.agent-sdlc` would fail the state and run-history assertions.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'PreservesWorkingTree'`.
- Run the broader validation: `go test ./internal/cli/...` and `go test ./internal/resume/...`.
- Do not rewrite any correct recovery behavior to force a change; add only the missing coverage.

## Acceptance Criteria

- recovery preserves a pre-existing modified tracked file and an untracked file.
- recovery does not invoke `git reset --hard` or `git clean`.
- recovery does not delete the state database or run history.
- the focused and broader validation commands pass.
- the gap is closed by the named proving tests, and no correct behavior is rewritten.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not add a destructive recovery step, and do not use `git reset --hard` or `git clean` in a test fixture against the project repository.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Keep all new tests deterministic and free of network access.

## Dependencies

- PREJEV016-S1
- PREJEV016-S2
- PREJEV016-S3

## Execution

- implement
