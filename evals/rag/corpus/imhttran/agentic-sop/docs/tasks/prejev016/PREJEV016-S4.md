# Named-Plan Provenance

## ID

PREJEV016-S4

## Objective

Verify that named plans cannot silently mix graphs: a run bound to a named plan
cannot silently resume against a different plan graph, and a provenance mismatch
produces a focused failure. Scope is limited to `internal/cli` (`run.go` plan
resolution) and `internal/planflow`. Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented. Existing correct
behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestRunNamedPlan`, `TestRunNamedPlanResolvesToDocs`, `TestRunNamedPlanAmbiguous`, `TestRunNamedPlanMissing`, `TestRunDifferentPlanStops`, `TestRunChangedPlanStops`, `TestPrepareRebuildsWhenSourceChanges`, and `TestPrepareReconcilesChangedPlanWithTasks`.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'NamedPlan|DifferentPlan|ChangedPlan'` and `go test ./internal/planflow/...`.
- Run the broader validation: `go test ./...`.
- Record that `resume` and `retry` operate only on the persisted store and never read or rewrite plan provenance, so they cannot rebind or mix a plan graph; do not add a test for behavior that structurally cannot occur.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.

## Acceptance Criteria

- a named plan resolves to its own graph.
- a run against a different active plan stops with a focused `different plan is already active` failure.
- a changed plan source stops with a focused `plan changed` failure.
- an ambiguous or missing named-plan selection fails clearly rather than mixing graphs.
- resume and retry cannot rebind or mix the plan graph (they do not touch plan provenance).
- the focused and broader validation commands pass.
- completion is recorded as no change (`changes_expected=false`) unless a real gap is demonstrated.

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Keep all new tests deterministic and free of network access.

## Dependencies

- none

## Execution

- verify-first
