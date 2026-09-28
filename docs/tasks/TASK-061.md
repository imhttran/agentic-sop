# T061 --- SOP Performance Measurement and Safe Reuse

> Measure where a run spends its time, expose verify-first savings, and safely
> reuse validation/review of an unchanged tree — without weakening any gate.

## Status

DONE (measurement core; some optimizations deliberately deferred, see below)

## Objective

Implement `docs/PLAN-SOP-Performance.md` measurement-first: add stage-level timing
and operation counts, persist them as diagnostic metadata, surface them in
`sop run` and `sop report`, measure the verify-first fast path, and reduce repeated
work only where SOP can prove the inputs are unchanged. Timing and reuse never
influence a workflow decision.

## Scope

- `internal/perf` (new): the single performance model — `Counts`, `Task`, `Run`,
  an injectable-clock `Recorder`, and the renderers used by `sop run` and
  `sop report`.
- `internal/cli/run.go`: `runStages` times `plan`/`implement`/`validation`/`review`/
  `fix` and counts agent calls, validations, reviews, fix cycles, and verify-first
  avoided calls. Metrics are written to `.agent-sdlc/runs/<id>/metrics.json` and to
  `report.json`'s `performance` field. `emitRunSummary` prints a concise line.
- `internal/cli/drive.go`: `driveGraph` aggregates the tasks it runs into a
  run-level record at `.agent-sdlc/runs/<plan-id>/metrics.json`.
- `internal/cli/report.go`: `sop report` shows the run summary when available,
  otherwise the task's record, otherwise a clear "unavailable" line.
- `internal/cli/session.go` (new): the per-run reuse cache for validation and
  review, keyed by input identity.
- `docs/PERFORMANCE.md` (new) and this task doc.

## Rules

- **Measurement is not control.** Timing never changes a stage, a gate, or task
  state; a missing metrics artifact is ignored.
- **Identity, not trust.** Validation reuse requires an unchanged command set _and_
  working-tree change; review reuse requires an unchanged task, diff, and engine.
  Only passing validation and clean review are cached; a miss is always safe.
- **verify-first is preserved and measured.** A passing verify-first task records
  one avoided agent call and still runs any required review/human gate.
- **No gate is weakened.** Parallel mutation, context/session reuse, targeted
  validation, and a persistent agent session are deferred with evidence rather than
  implemented unsafely.

## Deferred (with evidence, in docs/PERFORMANCE.md)

PERF008 targeted validation, PERF010 agent startup sessions, PERF011 intra-task
context reuse, PERF012 parallel mutation, and the real-provider half of PERF014.
Each has a recorded reason: no intermediate validation point exists to target,
sessions need a daemon the plan discourages, the command provider exposes no
session, and the local run shares one working tree/branch.

## Tests

`internal/perf`: deterministic timing with an injected clock, run aggregation and
categories, and the renderers. `internal/cli`: the deterministic benchmark fixture
(a verify-first task, an implementation task, a dependency) asserting operation
counts; validation reuse across unchanged verify-first tasks; the reuse cache
invalidated by a changed diff/command/task and never caching a failure or a
dirty review; and `sop report` surfacing performance.

## Acceptance Criteria

- [x] Stage-level duration metrics have one documented representation.
- [x] Agent/validation/review/fix counts and verify-first savings are available.
- [x] Metrics persist with run artifacts and never control task state.
- [x] `sop report` shows total time, categories, counts, and verify-first savings.
- [x] Validation/review reuse requires an unchanged input identity; a changed
      repository or config invalidates it.
- [x] Deterministic operation-count regression checks exist (no wall-clock
      assertions in the suite).
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T061): SOP performance measurement and safe reuse`

## Out of Scope

Weakening validation/review/human gates; parallel mutation; a performance daemon;
committing the plan documents.
