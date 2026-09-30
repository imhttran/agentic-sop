# PREJEV017 — Capture Pre-JEV Performance Baseline (Decomposition)

This document restructures the umbrella milestone **PREJEV017** into focused,
independently executable sub-tasks. It is the updated PREJEV017 plan/task
decomposition; the umbrella stays, and S1–S5 become tasks that can each be
understood, verified, and completed on their own.

## Status

- **PREJEV017 remains the umbrella measurement/reporting milestone.** It completes
  only when S1–S5 have all been validated (see
  [Umbrella completion criteria](#umbrella-completion-criteria)).
- **PREJEV017 is a measurement and reporting milestone, not a code-change task.**
  It is captured from SOP's existing instrumentation and already-recorded run
  artifacts. It adds no instrumentation, no optimization, and no test merely to
  force a repository mutation.
- **The plan source now decomposes PREJEV017.**
  `docs/plans/PLAN-Pre-JEV-Stabilization.md` declares the umbrella plus PREJEV017-S1 …
  PREJEV017-S5, and `.agent-sdlc/plan.json` is regenerated from it so the machine
  plan matches.
- **S1–S5 are standalone task files** under `docs/tasks/prejev017/`, each runnable
  in isolation:

  ```bash
  sop run --task docs/tasks/prejev017/PREJEV017-S1.md
  sop run --task docs/tasks/prejev017/PREJEV017-S2.md
  sop run --task docs/tasks/prejev017/PREJEV017-S3.md
  sop run --task docs/tasks/prejev017/PREJEV017-S4.md
  sop run --task docs/tasks/prejev017/PREJEV017-S5.md
  ```

- **The baseline itself is recorded** in
  `docs/history/PREJEV017-PERFORMANCE-BASELINE.md`.

## Why this decomposition exists

The previous PREJEV017 IMPLEMENT invocation ran in `implement` mode. It collected
and documented the required evidence with no repository mutation, and because SOP
was waiting for a change it entered FIX; FIX then attempted unrelated test work and
exhausted its iteration budget:

- `.agent-sdlc/runs/PREJEV017/report.json` → `"decision": "FAIL"`,
  `stage: FAILED`, `validation_runs: 0`, `fix_cycles: 1`, `agent_calls: 3`.
- `.agent-sdlc/runs/PREJEV017/report.md` → the gate: "Ollama agent FIX did not
  complete after 24 iterations … last_action=\"write_file
  path=internal/cli/baseline_test.go\"".
- `.agent-sdlc/runs/PREJEV017/metrics.json` → `fix` = 56 535 ms of an 86 875 ms
  total: the fix loop, not the measurement, dominated the run.

The fix is **not** a larger agent or FIX budget. It is to make the milestone
**verify-first** and to state explicitly that a completed no-change outcome
(`changes_expected=false`) is a success, so an already-supported baseline never
needs an agent and never enters FIX. No agent or tool budget was increased.

## Design rules applied

Each task S1–S5 satisfies the decomposition requirements:

1. **One responsibility** — a single baseline area (see each task's _Objective_).
2. **One subsystem** — the relevant package(s) are named explicitly.
3. **Concrete acceptance criteria** — checkable statements, not prose.
4. **Explicit validation commands** — focused package tests first, broader
   regression checks after (see each task's _Requirements_ and this document's
   [validation matrix](#validation-matrix)).
5. **No unrelated discovery** — scope is limited to the named subsystem.
6. **Independently resumable and idempotent** — a task runs via `sop run --task`
   and writes to its own `.agent-sdlc/runs/<ID>/`; re-running is safe.
7. **No manufactured changes** — a requirement already supported by existing
   instrumentation and captured artifacts is _recorded_, not implemented. Covered
   requirements complete with no change (`changes_expected=false`).
8. **No duplicated orchestration** — tasks are task files; the SOP lifecycle
   (`PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`) is reused unchanged.

### Execution mode and the per-task loop

Every sub-task follows the same three-step loop:

1. **Verify existing evidence first** — inspect the instrumentation and the
   captured artifacts that already exist before changing anything.
2. **Implement only what is missing** — add a test or a document only where a real
   gap is demonstrated; otherwise change nothing.
3. **Validate independently** — SOP's own VALIDATE stage runs the deterministic
   validation; the agent's self-report is not the validation.

Execution mode is per-task:

- **S1–S4 run in `verify-first`.** Their behavior is already proven by the existing
  suite, so the deterministic validation passes, no agent is invoked, and each
  completes with `changes_expected=false`. This is the fast path that removes the
  budget-exhaustion failure mode.
- **S5 runs in `implement`.** It is the recording-and-closure task: it may need to
  refresh `docs/history/PREJEV017-PERFORMANCE-BASELINE.md`. Because SOP allows a completed
  no-change outcome, S5 still passes with `changes_expected=false` when the
  document already reflects the captured evidence; `verify-first` was deliberately
  *not* used for S5 because it short-circuits before the document can be refreshed.

The deterministic validation SOP runs is the project-configured validation
(`validation.build` / `test` / `lint` in `.agent-sdlc/config.yaml`), not the focused
command listed on each task. The focused commands narrow the area for a human or
agent; the passing or failing gate is the configured suite.

The measurement paths are read-only over artifacts, so no `-race` run is required;
the broader `go test -race ./...` remains available as an optional extra check.

## Metric availability and evidence map

The baseline draws only on what already exists. This table maps each required
metric to the existing source, and each verification area to the existing test that
already proves the underlying behavior.

| Metric / area                     | Existing source                                                                                     | Status      |
| --------------------------------- | --------------------------------------------------------------------------------------------------- | ----------- |
| total time                        | `perf.Task.total_ms` / `perf.Run.total_ms` in `metrics.json`                                        | available   |
| agent time                        | `perf.Run.CategoryMS()` agent = `plan` + `implement` + `fix`                                        | available   |
| validation time (+ breakdown)     | `stages_ms.validation`, `validation_ms` `build`/`test`/`lint`                                       | available   |
| review time                       | `stages_ms.review`                                                                                  | available   |
| agent calls                       | `perf.Counts.agent_calls` (and `agent_calls_avoided`)                                               | available   |
| tool calls                        | **none** — `perf.Counts` has no tool-call field; only the ollama `MaxToolCalls` bound               | unavailable |
| validation runs                   | `perf.Counts.validation_runs` / `validation_reused`                                                 | available   |
| fix cycles                        | `perf.Counts.fix_cycles`                                                                            | available   |
| PLAN/IMPLEMENT/REVIEW measurement | `TestPerformanceBenchmarkFixture`, `TestReportShowsPerformance` (`internal/cli/perf_test.go`)        | covered     |
| controller-driven measurement     | `TestValidationReuseAcrossUnchangedVerifyFirstTasks`, `TestPerformanceBenchmarkFixture`             | covered     |
| run split (`CategoryMS`)          | `TestRunAggregatesTasksAndCategories`, `TestWriteTaskRendersStagesAndCounts` (`internal/perf`)       | covered     |
| verify-first no-change completion | `TestVerifyFirstPassesWithoutAgent`, `TestRunImplementNoChangesStillValidates` (`internal/cli`)      | covered     |
| validation reuse accounting       | `TestValidationReuseAcrossUnchangedVerifyFirstTasks`, `TestValidationReuseInvalidatedByChangedInputs` | covered     |

## Coverage gaps

**No coverage gap requires new production code.** The measurement and reporting
behavior is already implemented and tested; PREJEV017 records the baseline and
confirms the instrumentation. Two documentation outputs are in scope:

| ID  | Area | Output                                                                       |
| --- | ---- | ---------------------------------------------------------------------------- |
| D1  | S5   | `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` — the recorded baseline             |
| D2  | S4   | the tool-call and missing-instrumentation limitations recorded in that file  |

Neither D1 nor D2 is a code change, and neither may be satisfied by adding
instrumentation, an optimization, or a test. A test is added only if a real defect
in existing performance behavior is demonstrated independently of the
documentation requirement.

## Task definitions

### PREJEV017-S1 — Baseline Evidence Inventory and Environment

- **Responsibility:** confirm the repository is healthy at the captured revision
  and inventory the existing measurement surface the baseline draws on.
- **Subsystem:** `internal/perf`, `internal/cli` (`run.go`, `drive.go`,
  `report.go`), `.agent-sdlc/runs/`.
- **Dependencies:** none.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/perf/...` then
  `go test ./internal/cli/...`.

### PREJEV017-S2 — PLAN/IMPLEMENT/REVIEW Workflow Measurement

- **Responsibility:** measure a PLAN/IMPLEMENT/REVIEW workflow from the existing
  per-task artifacts and capture the full metric set, with the validation
  `build`/`test`/`lint` breakdown where the runner reported it.
- **Subsystem:** `internal/perf`, `internal/cli`.
- **Dependencies:** S1.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/cli/ -run 'Performance|Report'` then
  `go test ./internal/perf/...`.

### PREJEV017-S3 — Controller-Driven Workflow Measurement

- **Responsibility:** measure a controller-driven workflow from the existing run
  aggregate, including the aggregated tasks, run-level counts, and the `CategoryMS`
  agent/validation/review split.
- **Subsystem:** `internal/perf` (`Run.CategoryMS`), `internal/cli` (`drive.go`,
  `report.go`).
- **Dependencies:** S1.
- **Execution:** `verify-first`.
- **Focused validation:**
  `go test ./internal/cli/ -run 'Performance|Report|ValidationReuse'` then
  `go test ./internal/perf/...`.

### PREJEV017-S4 — Metric Availability, Bottlenecks, and Limitations

- **Responsibility:** record which metrics the existing instrumentation actually
  exposes; report tool calls only where exposed and otherwise state them as
  unavailable; document each known bottleneck with its supporting metric; record
  the missing instrumentation as a limitation; confirm optimization remains in the
  Performance plan.
- **Subsystem:** `internal/perf`, `internal/cli` (`report.go`).
- **Dependencies:** S2, S3.
- **Execution:** `verify-first`.
- **Focused validation:** `go test ./internal/perf/...` then
  `go test ./internal/cli/ -run 'Report|Performance'`.

### PREJEV017-S5 — Baseline Recording and Closure

- **Responsibility:** confirm `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` records the
  captured baseline for both measured workflows, that every PREJEV017 acceptance
  criterion maps to captured evidence, and that each captured number matches an
  existing artifact; refresh the document only where it is stale, and add no test
  or code to satisfy the milestone.
- **Subsystem:** the whole suite.
- **Dependencies:** S1, S2, S3, S4.
- **Execution:** `implement` (may complete with `changes_expected=false` when the
  document already reflects the captured evidence).
- **Focused validation:** `go test ./internal/perf/...`,
  `go test ./internal/cli/...`, then the full validation matrix below.

## Validation matrix

Focused commands per task are above; the umbrella gate is:

```bash
go test ./internal/perf/...
go test ./internal/cli/...
go test ./...
go vet ./...
go build ./...
```

The measurement paths are read-only over artifacts, so no focused `-race` run is
required; `go test -race ./...` remains available as an optional extra check.

## Umbrella completion criteria

PREJEV017 is complete only when **all** of the following hold:

1. Every task PREJEV017-S1 … PREJEV017-S5 has run to a passing result
   (`Passed` / `LOCAL_DONE`) via `sop run --task …`, having verified existing
   evidence first, added only real gaps, and validated independently. S1–S4
   complete with `changes_expected=false`; S5 records or refreshes the baseline.
2. The baseline is supported by existing instrumentation and the captured run
   artifacts (see the [metric availability and evidence map](#metric-availability-and-evidence-map)),
   and every captured number matches an artifact.
3. `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` records both measured workflows and the
   full metric set, and states tool calls as unavailable from perf.
4. The validation matrix above passes.
5. No instrumentation, optimization, or test was added merely to force a
   repository change, and existing unrelated working-tree changes are preserved.

### Acceptance criteria → task

| Acceptance criterion                                                              | Task   |
| --------------------------------------------------------------------------------- | ------ |
| a PLAN/IMPLEMENT/REVIEW workflow is measured from existing run artifacts           | S2     |
| a controller-driven workflow is measured from existing run artifacts               | S3     |
| agent time and deterministic validation time are distinguishable                   | S2, S3 |
| known bottlenecks are documented with their supporting metric and evidence         | S4     |
| tool calls are reported only when instrumentation exposes them, else stated unavailable | S4 |
| missing instrumentation is documented as a limitation, not implemented             | S4     |
| optimization remains deferred to the Performance plan                              | S4     |
| existing unrelated working-tree changes are preserved                              | all    |
| successful verification may complete with `changes_expected=false`                 | all    |
| a no-change completed outcome does not trigger FIX merely for lack of a mutation   | all    |
| baseline recording and regression closure                                          | S5     |

## Workflow authority (unchanged)

This decomposition does not change SOP's workflow authority or lifecycle:

- Lifecycle remains `PLAN → IMPLEMENT → VALIDATE → REVIEW/FIX`.
- Human gates are unchanged; nothing is committed, pushed, or merged
  automatically.
- No destructive reset/cleanup; `.agent-sdlc/state.db` is never hand-edited.
- No regression task re-implements orchestration; each is a task file driven by
  the existing lifecycle.
- No CI/PR/merge result is fabricated.
- Perf data is diagnostic metadata only and never drives a workflow decision.
