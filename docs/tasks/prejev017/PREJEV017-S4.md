# Metric Availability, Bottlenecks, and Limitations

## ID

PREJEV017-S4

## Objective

Record which metrics the existing instrumentation actually exposes and which it
does not. Tool calls have no field in `perf.Counts`, so the baseline states them as
unavailable from the perf metrics rather than inventing a value; the missing
instrumentation is documented as a limitation, not implemented. Document each known
bottleneck with the metric and evidence that support it, and confirm the
non-blocking optimization work remains in `docs/plans/PLAN-SOP-Performance.md`. Scope is
limited to `internal/perf` and `internal/cli` (`report.go`). Do not discover
unrelated subsystems.

This is a reporting task: it documents availability and limitations. No metric,
field, or optimization is added.

## Requirements

- Follow the three-step loop: verify existing evidence first, implement only what is missing, then validate independently.
- Inspect existing coverage before adding anything. The schema and rendering are covered by `TestRecorderMeasuresWithInjectedClock`, `TestWriteTaskRendersStagesAndCounts` (`internal/perf/perf_test.go`), and `TestReportShowsPerformance` (`internal/cli/perf_test.go`).
- Verify from source that `internal/perf.Counts` has no tool-call field and that `perf.WriteRun` / `perf.WriteTask` never print one; state tool calls as unavailable instead of adding a field.
- Support each documented bottleneck with the capture that proves it — for example `implement` dominance (`.agent-sdlc/runs/PREJEV016/metrics.json`, `.agent-sdlc/runs/plan-agent-harness-v2/metrics.json`) and fix-loop cost (`.agent-sdlc/runs/PREJEV017/metrics.json`).
- Confirm optimization remains delegated to `docs/plans/PLAN-SOP-Performance.md` with nothing pulled forward.
- Run the focused validation and confirm it passes: `go test ./internal/perf/...` and `go test ./internal/cli/ -run 'Report|Performance'`.
- Run the broader validation: `go test ./...`.
- If every acceptance criterion is already covered, record it and finish with no change (`changes_expected=false`).

## Acceptance Criteria

- tool-call availability is stated from the existing instrumentation, and no new perf field is added.
- each documented bottleneck cites the captured metric and its source artifact.
- the missing instrumentation is recorded as a limitation, not implemented.
- optimization is delegated to `docs/plans/PLAN-SOP-Performance.md`, with nothing pulled forward.
- the focused and broader validation commands pass.
- completion is recorded as no change (`changes_expected=false`).

## Constraints

- Do not commit, push, or merge; do not run destructive git operations; never edit `.agent-sdlc/state.db`.
- Do not add an instrumentation field, an optimization, a test, or production code; this task documents and verifies only.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Preserve existing unrelated working-tree changes.

## Dependencies

- PREJEV017-S2
- PREJEV017-S3

## Execution

- verify-first
