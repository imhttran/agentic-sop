# PLAN/IMPLEMENT/REVIEW Workflow Measurement

## ID

PREJEV017-S2

## Objective

Measure a PLAN/IMPLEMENT/REVIEW workflow from the existing per-task artifacts and
capture the full metric set: total time, agent time (plan + implement),
validation time, review time, agent calls, validation runs, and fix cycles, with
the validation `build`/`test`/`lint` breakdown where the runner reported it.
Scope is limited to `internal/perf` and `internal/cli`. Do not discover unrelated
subsystems.

This is a measurement task: the workflow and its instrumentation already exist.
Existing correct behavior must be validated, not rewritten. Tool calls are
recorded only where the existing output exposes them — perf exposes none.

## Requirements

- Follow the three-step loop: verify existing evidence first, implement only what is missing, then validate independently.
- Inspect existing coverage before adding anything. The workflow's measurement is covered by `TestPerformanceBenchmarkFixture` and `TestReportShowsPerformance` in `internal/cli/perf_test.go`, and the rendering by `TestWriteTaskRendersStagesAndCounts` in `internal/perf/perf_test.go`.
- Read the captured evidence from `.agent-sdlc/runs/PREJEV016/metrics.json` (a completed plan → implement → validate → review task) and from the per-task rendering; do not fabricate a value that no artifact contains.
- Run the focused validation and confirm it passes: `go test ./internal/cli/ -run 'Performance|Report'` and `go test ./internal/perf/...`.
- Run the broader validation: `go test ./...`.
- If a required metric is absent from the existing artifacts, record it as not recorded rather than inventing it, and finish with no change (`changes_expected=false`).

## Acceptance Criteria

- a task completes the PLAN/IMPLEMENT/REVIEW lifecycle with plan, implement, validation, and review stages recorded in `stages_ms`.
- every required metric is captured from the existing artifacts, not fabricated.
- agent time and validation time are separable, and the validation breakdown is present where the runner reported it.
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
