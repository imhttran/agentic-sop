# PREJEV017 — Pre-JEV Performance Baseline

Captured: 2026-09-28. Scope: `agentic-sop` at `main` (HEAD `9ac8e44`,
"ollamaagent harness, JEV capability, and planning docs").

This is a **measurement and reporting** artifact. It is produced from SOP's
_existing_ instrumentation and already-recorded run artifacts — the `internal/perf`
recorder, its `metrics.json` files under `.agent-sdlc/runs/`, and the `sop report`
rendering. No new instrumentation, no test, and no optimization was added to
produce it. Timing metadata never drives a workflow decision, and nothing below
changes validation, review, plan provenance, or a human approval boundary.

## Measurement model (existing instrumentation only)

One package, `internal/perf`, is the single representation of perf data:

- **Stage durations** (`stages_ms`, monotonic milliseconds): `plan`, `implement`,
  `validation` (with a `build`/`test`/`lint` breakdown in `validation_ms`),
  `review`, and `fix`, plus the task's `total_ms`.
- **Counts** (`internal/perf.Counts`): agent calls, agent calls avoided
  (verify-first), validation runs, validation reused, review runs, review reused,
  and fix cycles. **There is no tool-call field.**
- **Run aggregate** (`internal/perf.Run`): `started_at`, `total_ms`, the
  aggregated `tasks`, and the summed counts. `Run.CategoryMS()` splits measured
  time into **agent** (`plan` + `implement` + `fix`), **validation**, and
  **review**.

It is persisted per task at `.agent-sdlc/runs/<task-id>/metrics.json` and per run
at `.agent-sdlc/runs/<plan-id>/metrics.json`, and rendered by `sop report`
(`perf.WriteTask` / `perf.WriteRun`) plus the per-task `performance:` line
(`Task.Line()`).

## Metric set captured

| Metric                       | Source in existing artifacts                                                 |
| ---------------------------- | ---------------------------------------------------------------------------- |
| total time                   | `Task.total_ms` / `Run.total_ms`                                             |
| agent time                   | `CategoryMS` agent = `plan` + `implement` + `fix` (per-task `stages_ms`)     |
| validation time              | `stages_ms.validation`, broken down by `validation_ms` `build`/`test`/`lint` |
| review time                  | `stages_ms.review`                                                           |
| agent calls                  | `Counts.agent_calls`                                                         |
| tool calls _where available_ | **not available** — `perf.Counts` has no tool-call field (see below)         |
| validation runs              | `Counts.validation_runs` (and `validation_reused`)                           |
| fix cycles                   | `Counts.fix_cycles`                                                          |

## Workflow A — PLAN/IMPLEMENT/REVIEW

Evidence: `.agent-sdlc/runs/PREJEV016/metrics.json` — one completed ordinary task
that ran plan → implement → validate → review.

Per-task `metrics.json` (verbatim):

```json
{
  "id": "PREJEV016",
  "total_ms": 36621,
  "stages_ms": {
    "implement": 24219,
    "plan": 6054,
    "review": 3806,
    "validation": 2507
  },
  "validation_ms": { "build": 487, "lint": 276, "test": 1743 },
  "counts": {
    "agent_calls": 2,
    "agent_calls_avoided": 0,
    "validation_runs": 1,
    "validation_reused": 0,
    "review_runs": 1,
    "review_reused": 0,
    "fix_cycles": 0
  }
}
```

Per-task rendering (`perf.WriteTask`):

```text
PLAN        6.1s
IMPLEMENT   24.2s
BUILD       487ms
TEST        1.7s
LINT        276ms
REVIEW      3.8s
----------------
TOTAL       36.6s

Agent calls: 2
Validation runs: 1
Review runs: 1
Fix cycles: 0
```

Captured metric set:

| Metric          | Value                                           |
| --------------- | ----------------------------------------------- |
| total time      | 36.6 s                                          |
| agent time      | 30.3 s (plan 6.1 s + implement 24.2 s)          |
| validation      | 2.5 s (build 487 ms + test 1.7 s + lint 276 ms) |
| review time     | 3.8 s                                           |
| agent calls     | 2                                               |
| tool calls      | unavailable from perf (see below)               |
| validation runs | 1                                               |
| fix cycles      | 0                                               |

Per-task summary line (`Task.Line()`):

```text
36.6s total (agent 30.3s, validation 2.5s, review 3.8s) | agent calls 2, validation runs 1, fix cycles 0
```

## Workflow B — controller-driven

A controller-driven workflow is `sop run` driving the task graph: the scheduler
folds each task's persisted record into a run-level aggregate
(`internal/cli/drive.go`), which `perf.WriteRun` and `sop report` render.

Evidence (primary): `.agent-sdlc/runs/plan-agent-harness-v2/metrics.json` — a real
two-task controller run, both tasks ordinary `implement` tasks.

Per-run rendering (`perf.WriteRun`):

```text
Performance

Tasks:       2
Total:       10m 9s

Agent:       8m 10s    (80%)
Validation:  2.0s      (0%)
Review:      1m 57s    (19%)

Agent calls:            4
Agent calls avoided:    0
Validation runs:        1
Validation reused:      0
Review runs:            1
Review reused:          0
Fix cycles:             0
```

Captured metric set:

| Metric          | Value                                            |
| --------------- | ------------------------------------------------ |
| total time      | 10m 9s (609 607 ms)                              |
| agent time      | 8m 10s (490 199 ms) — 80% of measured stage time |
| validation      | 2.0 s (2 027 ms) — 0% after rounding             |
| review time     | 1m 57s (117 334 ms) — 19%                        |
| agent calls     | 4                                                |
| tool calls      | unavailable from perf (see below)                |
| validation runs | 1 (0 reused)                                     |
| fix cycles      | 0                                                |

Evidence (secondary, pre-JEV stabilization): `.agent-sdlc/runs/plan-pre-jev-stabilization/metrics.json`
— a controller run whose recorded aggregate contains one task, the ordinary
`PREJEV013` task.

```text
Performance

Tasks:       1
Total:       23.9s

Agent:       23.9s    (100%)

Agent calls:            2
Agent calls avoided:    0
Validation runs:        0
Validation reused:      0
Review runs:            0
Review reused:          0
Fix cycles:             0
```

This matches `sop report` verbatim, so the captured run aggregate and its
rendering agree. It is included as a second controller run with a different
task-shape (a plan+implement task that reached no validation or review stage),
not as fix-loop evidence.

## Agent time versus deterministic validation time

The two are already separable with existing instrumentation, by two independent
signals that agree:

- `Run.CategoryMS()` splits measured time into agent (`plan` + `implement` +
  `fix`), validation, and review. In Workflow B (primary) that is 8m 10s agent
  versus 2.0s validation — deterministic validation is ~0.3% of measured time.
- Per-task, `validation_ms` carries the `build`/`test`/`lint` breakdown of the
  validation stage, so the deterministic cost is attributable per check, not just
  as a total (Workflow A: 487 ms + 1 743 ms + 276 ms).

Accounting convention (from the code, not invented here): `fix` counts as **agent**
time, `review` is reported **separately** from agent and validation, and the
percentages are taken over measured stage time only (`agent + validation + review`),
so an unrun stage does not dilute the split.

## Workflow C — controller-driven with a fix cycle

Evidence: `.agent-sdlc/runs/PREJEV017/metrics.json` — the failed first attempt at
this task, measured through the same lifecycle. It is the only observed fix cycle
in the captured set and shows the controller driving a task through
plan → implement → validate(reused) → review → fix.

Per-task `metrics.json` (verbatim):

```json
{
  "id": "PREJEV017",
  "total_ms": 86875,
  "stages_ms": {
    "fix": 56535,
    "implement": 15121,
    "plan": 11902,
    "review": 3282
  },
  "counts": {
    "agent_calls": 3,
    "agent_calls_avoided": 0,
    "validation_runs": 0,
    "validation_reused": 1,
    "review_runs": 1,
    "review_reused": 0,
    "fix_cycles": 1
  }
}
```

Captured metric set:

| Metric          | Value                                              |
| --------------- | -------------------------------------------------- |
| total time      | 86.9 s                                             |
| agent time      | 83.6 s (plan 11.9 s + implement 15.1 s + fix 56.5 s)|
| validation      | 0 s executed, 1 suite safely reused                |
| review time     | 3.3 s                                              |
| agent calls     | 3                                                  |
| tool calls      | unavailable from perf (see below)                  |
| validation runs | 0 executed (1 reused)                              |
| fix cycles      | 1                                                  |

## Tool calls — unavailable from perf

Tool calls are reported as **unavailable from the perf metrics**, and no perf field
was added to make them available:

- `internal/perf.Counts` has no tool-call field, and `perf.WriteRun` /
  `perf.WriteTask` therefore never print one.
- The only existing tool-call signal is the ollama harness's `MaxToolCalls` bound
  (visible in the harness, not surfaced through `internal/perf` or `sop report`).
  The command harness used for these measurements does not surface a tool-call
  count either.

Reporting a tool-call count would require a new `perf.Counts` field and new wiring;
that is instrumentation work and is out of scope here, so tool calls are recorded as
unavailable rather than invented.

## Known bottlenecks (metric + evidence)

Each bottleneck cites the captured metric and its artifact.

1. **IMPLEMENT dominates agent time.** Workflow A `implement` = 24 219 ms = 66% of
   the task's 36 586 ms measured and 80% of its 30 273 ms agent time. In the primary
   controller run, `AHV2011` `implement` = 224 524 ms = 82% of its 274 700 ms
   total, and `AHV2009` `implement` = 159 904 ms = 48% of its 334 902 ms.
   _Source: `PREJEV016`, `plan-agent-harness-v2`._
2. **PLAN is a non-trivial fixed cost per task.** `plan` = 6 054 ms (17% of the
   measured stage time) for `PREJEV016`, 50 173 ms (18% of total) for `AHV2011`,
   55 598 ms (17% of total) for `AHV2009`. _Source: same._
3. **REVIEW can rival IMPLEMENT on a change-heavy task.** `AHV2009` `review` =
   117 334 ms = 35% of its total and the single largest non-agent cost recorded;
   it is 19% of the controller run's measured time versus ~0% validation.
   _Source: `plan-agent-harness-v2`._
4. **FIX loops are the most expensive single stage when they run.** The failed
   `PREJEV017` attempt spent `fix` = 56 535 ms = 65% of its 86 875 ms total, the
   only fix cycle observed; it exhausted its iteration budget. _Source:
   `.agent-sdlc/runs/PREJEV017/metrics.json`._
5. **Deterministic validation is cheap, and free when safely reused.** `PREJEV016`
   validation = 2 507 ms = 7% of measured. The `PREJEV017` attempt recorded
   `validation_reused: 1` with `validation_runs: 0`, i.e. its suite was safely
   reused for unchanged inputs and cost no deterministic execution time.
   _Source: `PREJEV016`, `PREJEV017`._

## Missing instrumentation (documented, not implemented)

Recorded here as limitations; none is implemented by PREJEV017:

- **No tool-call counter** in `perf.Counts`; see the section above.
- **No stage breakdown beyond the duration** for a `fix` cycle, and no
  attempt/retry counter other than `fix_cycles`.
- **No `validation_ms` categories when validation is reused**, so a reused suite
  cannot be attributed per `build`/`test`/`lint` (e.g. `AHV2011`, `PREJEV017`).
- **No per-step timing inside PLAN**, so a slow plan is one number, not a profile.

## Optimization — deferred

Every optimization above is deferred to `docs/PLAN-SOP-Performance.md`; nothing is
pulled forward here. PREJEV017 measures and reports only. Non-blocking optimization
remains in the Performance plan (PERF001–PERF015) and is out of scope for
PREJEV017.

## Reproduction

The evidence is re-readable without running anything (read-only):

```bash
cat .agent-sdlc/runs/PREJEV016/metrics.json
cat .agent-sdlc/runs/plan-agent-harness-v2/metrics.json
cat .agent-sdlc/runs/plan-pre-jev-stabilization/metrics.json
cat .agent-sdlc/runs/PREJEV017/metrics.json
sop report
```

The measurement is also reproducible deterministically through the existing
fixtures (no network, no real agent): `TestPerformanceBenchmarkFixture`,
`TestValidationReuseAcrossUnchangedVerifyFirstTasks`,
`TestValidationReuseInvalidatedByChangedInputs`, and `TestReportShowsPerformance`
in `internal/cli/perf_test.go`, plus the `internal/perf` unit tests:

```bash
go test ./internal/perf/...
go test ./internal/cli/ -run 'Performance|Report|ValidationReuse'
```
