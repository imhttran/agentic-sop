# Recovery Regression Closure

## ID

PREJEV016-S7

## Objective

Close out the recovery regression milestone: run the complete recovery suite plus
the broader standard suite, confirm every PREJEV016 acceptance criterion maps to at
least one passing verification, add only genuinely missing coverage, and complete
with no change where behavior is already covered. This is the closure task for the
umbrella PREJEV016 and runs in `implement` mode, because it may need to add tests.
Scope is the whole suite, with the recovery surfaces in `internal/resume` and
`internal/cli` first.

Do not discover or modify unrelated subsystems, and do not rewrite correct
behavior to force a change.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Run the focused validation and record the result: `go test ./internal/resume/...` and `go test ./internal/cli/...`.
- Run the full validation matrix: `go test ./...`, `go vet ./...`, `go build ./...`.
- Confirm the closure test for the working-tree gap passes: `TestRetryPreservesWorkingTreeAndRunHistory`, `TestResumePreservesWorkingTreeAndRunHistory`, and `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory`.
- Map every PREJEV016 acceptance criterion (resume, retry, interrupted active-task recovery, named-plan provenance, safe reset, working-tree preservation) to its proving test or record it as already covered.
- If no gap is confirmed, complete with no change (`changes_expected=false`).

## Acceptance Criteria

- every PREJEV016 acceptance criterion maps to at least one passing test.
- the full recovery suite, the standard suite, `go vet ./...`, and `go build ./...` pass.
- gaps are closed by named proving tests or recorded as already covered.
- no correct recovery behavior is rewritten and no out-of-scope feature is implemented.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Add only tests where a real coverage gap is demonstrated; otherwise make no change.
- Keep all tests deterministic and free of network access.

## Dependencies

- PREJEV016-S1
- PREJEV016-S2
- PREJEV016-S3
- PREJEV016-S4
- PREJEV016-S5
- PREJEV016-S6

## Execution

- implement
