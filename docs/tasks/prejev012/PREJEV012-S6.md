# Repository Change Reconciliation

## ID

PREJEV012-S6

## Objective

Verify repository-change detection and reconciliation in the Ollama agent harness: changed, unchanged, and restored files, plus the rule that observed repository reality overrides the model's `changes_expected` claim. Scope is limited to `internal/ollamaagent` (reconcile/outcome) and `internal/toolharness` (`WorkingTreeFingerprint`/`ChangedSince`). Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented from PREJEV009. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestObservedRepositoryChangeMutationEvidenceWins`, `TestObservedRepositoryChangeNoEvidenceNoBaselineUnknown`, `TestChangedSinceReportsBaselineDivergence`, `TestReconcileOutcomeOverrideBothDirections`, `TestReconcileOutcomeFingerprintFailureKeepsModelClaim`, `TestReconcileOutcomeGroundsChangesExpected`, `TestOutcomeWireShapeIsSOPCompatible`, `TestWorkingTreeChanged`, and `TestRestoreFileRecoversCommittedContent`.
- Run the focused validation and confirm it passes: `go test ./internal/ollamaagent/ -run 'Reconcile|ObservedRepositoryChange|ChangedSince|MutationEvidence|PreDirty|OutcomeWire'`.
- Run the broader validation: `go test ./internal/ollamaagent/... ./internal/toolharness/...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more. The restored-file case (G1) is already covered by `TestChangedSinceIgnoresRestoredFile`; confirm it passes alongside the others.

## Acceptance Criteria

- changed, unchanged, and restored repository states are each covered by a passing test.
- a successful controlled mutation is a positive change signal even when the working tree already had changes.
- a model `changes_expected=true` claim over an unchanged tree is reconciled to `false`, and `changes_expected=false` over a changed tree is reconciled to `true`.
- the emitted outcome wire shape stays SOP-compatible (`status`/`summary`/`reason`/`changes_expected` only).
- the focused and broader validation commands pass.
- completion is recorded either as no change (`changes_expected=false`) or as a single focused test that closes a demonstrated gap.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Keep all new tests deterministic and free of a live Ollama instance.

## Dependencies

- none

## Execution

- implement
