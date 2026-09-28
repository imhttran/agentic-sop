# Baseline Evidence Inventory and Environment

## ID

PREJEV017-S1

## Objective

Confirm the repository is healthy at the captured revision and inventory the
existing measurement surface the pre-JEV performance baseline draws on:
`internal/perf` (stages, `Counts`, `Task`, `Run`, `CategoryMS`, `WriteTask`,
`WriteRun`), the recorder wiring in `internal/cli` (`run.go`, `drive.go`,
`report.go`), and the already-recorded run artifacts under `.agent-sdlc/runs/`
(per-task `metrics.json`, the run aggregates, and `sop report`). Scope is limited
to those packages and artifacts. Do not discover unrelated subsystems.

This is a verification task: the measurement surface already exists and is already
tested. Existing correct behavior must be validated, not rewritten, and no
production code is changed.

## Requirements

- Follow the three-step loop: verify existing evidence first, implement only what is missing, then validate independently.
- Inspect the existing measurement surface before adding anything. It is covered by `TestRecorderMeasuresWithInjectedClock`, `TestRunAggregatesTasksAndCategories`, `TestWriteTaskRendersStagesAndCounts`, and `TestHumanMS` in `internal/perf/perf_test.go`.
- Read the captured artifacts read-only (for example `cat .agent-sdlc/runs/PREJEV016/metrics.json`); do not run a workflow to produce new evidence.
- Run the focused validation and confirm it passes: `go test ./internal/perf/...` and `go test ./internal/cli/...`.
- Run the broader validation: `go test ./...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.

## Acceptance Criteria

- the captured revision and working-tree state are recorded.
- the existing instrumentation surface is inventoried from the repository, and no new field or metric is claimed.
- the focused and broader validation commands pass.
- completion is recorded as no change (`changes_expected=false`).

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not add instrumentation, a metric, a test, or production code; this task inventories and verifies only.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Preserve existing unrelated working-tree changes.

## Dependencies

- none

## Execution

- verify-first
