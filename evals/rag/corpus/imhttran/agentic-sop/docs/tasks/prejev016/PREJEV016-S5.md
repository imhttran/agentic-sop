# Safe Task Reset, If Available

## ID

PREJEV016-S5

## Objective

Determine whether a safe task-reset capability exists. If it exists, verify that it
resets only the intended task and preserves run history and `state.db`. If it does
not exist, record the requirement as not applicable and do not implement a reset
feature to satisfy this milestone. Scope is limited to `internal/cli`,
`internal/domain`, and `internal/store`. Do not discover unrelated subsystems.

Determination for this milestone: no safe task-reset capability exists. There is no
`reset` subcommand in the CLI dispatcher, and the only `Reset` symbols in the tree
are unrelated (`bytes.Buffer.Reset` inside `internal/toolharness` and
`FakeProvider.Reset` in the test harness). Recovery is instead served by `retry`
(requeue a BLOCKED task) and `resume` (reconcile an interrupted task), which are
non-destructive. This requirement is therefore recorded as not applicable; no reset
feature is added.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Confirm the determination above: there is no `reset` subcommand in `internal/cli/cli.go`, and no task-reset symbol in `internal/`.
- Run the focused validation and confirm it passes: `go test ./internal/cli/...` and `go test ./internal/store/...`.
- Run the broader validation: `go test ./...`.
- Record the requirement as not applicable and finish with no change (`changes_expected=false`); do not add a reset feature.

## Acceptance Criteria

- the presence or absence of a safe task-reset capability is determined and recorded.
- no safe reset exists in this milestone, so the requirement is recorded as not applicable.
- if a safe reset were present, it would affect only the intended task and preserve run history and `state.db`.
- safe reset is never used as a substitute for non-destructive recovery.
- the focused and broader validation commands pass.
- the task completes with no change (`changes_expected=false`); no reset feature is implemented.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not implement a new reset feature solely to satisfy this task.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Keep all new tests deterministic and free of network access.

## Dependencies

- none

## Execution

- verify-first
