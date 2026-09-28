# Controller-Driven Workflow Measurement

## ID

PREJEV017-S3

## Objective

Measure a controller-driven workflow from the existing run aggregate
(`internal/perf.Run`): the aggregated tasks, the run-level counts (including
`agent_calls_avoided` and validation reuse), and the `CategoryMS` agent /
validation / review split that `perf.WriteRun` and `sop report` render. Scope is
limited to `internal/perf` and `internal/cli` (`drive.go`, `report.go`). Do not
discover unrelated subsystems.

This is a measurement task: a controller-driven `sop run` already writes the
aggregate. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing evidence first, implement only what is missing, then validate independently.
- Inspect existing coverage before adding anything. The controller aggregate and split are covered by `TestRunAggregatesTasksAndCategories` (`internal/perf/perf_test.go`), `TestPerformanceBenchmarkFixture`, `TestValidationReuseAcrossUnchangedVerifyFirstTasks`, and `TestValidationReuseInvalidatedByChangedInputs` (`internal/cli/perf_test.go`).
- Read the captured evidence from `.agent-sdlc/runs/plan-agent-harness-v2/metrics.json` and `.agent-sdlc/runs/plan-pre-jev-stabilization/metrics.json`; confirm the captured run aggregate agrees with the `sop report` rendering.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'Performance|Report|ValidationReuse'` and `go test ./internal/perf/...`.
- Run the broader validation: `go test ./...`.
- If every acceptance criterion is already covered, record the covering tests and finish with no change (`changes_expected=false`).

## Acceptance Criteria

- a controller-driven run writes a run-level aggregate that aggregates its tasks.
- run-level agent, validation, and review cost is distinguishable through the existing split.
- validation reuse is reported when it occurred, and a reused validation is not counted as a real run.
- the captured aggregate agrees with the `sop report` rendering.
- the focused and broader validation commands pass.
- completion is recorded as no change (`changes_expected=false`).

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not add a metric, a test, or production code, and do not run a real agent workflow to synthesize evidence.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Preserve existing unrelated working-tree changes.

## Dependencies

- PREJEV017-S1

## Execution

- verify-first
