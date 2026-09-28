# Pre-Dirty Working Tree

## ID

PREJEV012-S7

## Objective

Verify that pre-existing user changes in the working tree are preserved and are never falsely attributed to the agent. Scope is limited to `internal/ollamaagent` (pre-dirty reconciliation) and `internal/e2e/harness` plus `internal/e2e/lifecycle` (the `Repo.Dirty`/`Repo.Untracked` fixtures). Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented from PREJEV009. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestPreDirtyNoInvocationChangeIsNotAttributed`, `TestPreDirtyWithInvocationChangeIsAttributed`, `TestPreDirtyStagedChangeNotAttributed`, `TestRepoCleanAndDirty`, `TestRepoUntrackedIsDirty`, and `TestImplementInvocationIsolation`.
- Run the focused validation and confirm it passes: `go test ./internal/ollamaagent/ -run 'PreDirty'`.
- Run the broader validation: `go test ./internal/e2e/... -run 'Repo|Isolation'`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more. The preservation case (G2) is covered by `TestPreDirtyContentPreservedAcrossInvocation`, and the cross-capability fixture (G3) by `TestPreDirtyRepoAcrossCapabilities`; confirm both pass.

## Acceptance Criteria

- a pre-existing modification and a pre-existing untracked file each leave the repository dirty before the invocation begins.
- a completed claim of `changes_expected=true` over a pre-dirty tree with no invocation change is reconciled to `false`.
- an invocation that does mutate over a pre-dirty tree is attributed, while pre-existing changes are not.
- pre-existing user changes are preserved, not deleted or reverted.
- invocation isolation still holds with a pre-dirty repository.
- the focused and broader validation commands pass.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations (`git reset --hard`, `git clean`); never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Keep all new tests deterministic and free of a live Ollama instance.

## Dependencies

- PREJEV012-S6

## Execution

- implement
