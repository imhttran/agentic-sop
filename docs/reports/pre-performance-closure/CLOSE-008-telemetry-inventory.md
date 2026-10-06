# CLOSE-008 — Inventory Performance Telemetry

- Task: CLOSE-008 (Inventory Performance Telemetry)
- Plan: `docs/plans/PLAN-Pre-Performance-Closure.md` (section `CLOSE-008 — Inventory Performance Telemetry`)
- Classification: evidence-only closure task. This report is the single intentional repository mutation.
- Dependencies: CLOSE-003 (SOP deterministic baseline) and CLOSE-004 (controller deterministic baseline), both present under `docs/reports/pre-performance-closure/`.

This report inventories the authoritative performance telemetry the pre-performance
baseline will consume, and classifies each listed data point as AVAILABLE, PARTIAL,
MISSING or NOT APPLICABLE with an existing source path, artifact or schema element
per row. It introduces no new metrics store, no new schema field, no new log and no
new instrumentation architecture. Every value is either present in an existing
artifact (with unit and observation scope) or written as UNAVAILABLE/MISSING with
the reason. No value is inferred or estimated.

---

## 1. Authoritative telemetry contract (schema)

Source of truth: `internal/perf/perf.go` (package doc: "SOP's single performance
model", "diagnostic metadata only — timing never influences a workflow decision").
Reference documentation: `docs/reference/PERFORMANCE.md`. Where wording differs,
the code is authoritative.

| Contract element | Identifier (source path) | Unit | Scope |
| --- | --- | --- | --- |
| Stage durations | `internal/perf/perf.go` `Task.StagesMS map[string]int64` (`json:"stages_ms"`) | milliseconds | per task |
| Stage names | `internal/perf/perf.go` constants `StagePlan`, `StageImplement`, `StageValidation`, `StageReview`, `StageFix` | — | per task |
| Validation breakdown | `internal/perf/perf.go` `Task.ValidationMS` + `CategoryBuild`/`CategoryTest`/`CategoryLint`; recorded by `Recorder.AddValidation` | milliseconds | per task (sums to `stages_ms[validation]`) |
| Task total | `internal/perf/perf.go` `Task.TotalMS` (`json:"total_ms"`) | milliseconds | per task wall clock |
| Operation counts | `internal/perf/perf.go` `Counts` (`agent_calls`, `agent_calls_avoided`, `validation_runs`, `validation_reused`, `review_runs`, `review_reused`, `fix_cycles`, `plan_repairs`) | count | per task / per run |
| Category projection | `internal/perf/perf.go` `Run.CategoryMS() (agent, validation, review)` | milliseconds | per run (plan aggregate) |
| Task rendering | `internal/perf/perf.go` `WriteTask(w, Task)` | text | per task |
| Run rendering | `internal/perf/perf.go` `WriteRun(w, Run)` | text | per run |
| JEV sub-record | `internal/perf/perf.go` `JEV` (`invocations`, `total_ms`, `provider`, `model`, `tool_calls`, `findings`, `blocking_findings`) + `WriteJEV` | ms / count / name | JEV only, per task and aggregated per run |
| Recorder API | `internal/perf/perf.go` `NewRecorder`/`WithClock`/`Measure`/`Add`/`AddValidation`/`AgentCall`/`ValidationRun`/`ReviewRun`/`FixCycle`/`PlanRepair`/`JEVInvocation` | — | per task lifecycle |

**Base contract (extend later, do not replace).** The base contract to extend in a
future phase is exactly: `stages_ms` (`plan`, `implement`, `validation` with the
`build`/`test`/`lint` breakdown, `review`, `fix`), `validation_ms`, `total_ms`,
`counts` (`agent_calls`, `agent_calls_avoided`, `validation_runs`,
`validation_reused`, `review_runs`, `review_reused`, `fix_cycles`, `plan_repairs`),
`CategoryMS`, `WriteTask`, `WriteRun`, plus the `JEV` sub-record. `internal/perf` is
the single authoritative metrics representation; no competing metrics store exists
in this repository or in this plan.

---

## 2. Generation path

Source: `internal/cli/run.go`.

- `executeLifecycle` calls `runStages` (`internal/cli/run.go`), which creates the
  recorder: `rec := perf.NewRecorder(rn.State().ID)`.
- Stage timers wrap existing operations with `rec.Measure(perf.StagePlan)`,
  `rec.Measure(perf.StageImplement)`, `rec.Measure(perf.StageReview)`,
  `rec.Measure(perf.StageFix)` in `runStages`, and `rec.Measure(perf.StageValidation)`
  in `timedValidation` (`internal/cli/run.go`).
- Counts are incremented at the same call sites: `rec.AgentCall()` (PLAN,
  IMPLEMENT, FIX; and in the `OnRepair` callback for plan repairs),
  `rec.AgentCallAvoided()` (verify-first pass), `rec.ValidationRun()` /
  `rec.ValidationReused()` (in `timedValidation` / `sessionValidation`),
  `rec.ReviewRun()` / `rec.ReviewReused()` (review path in `runStages`),
  `rec.FixCycle()` (fix loop), `rec.PlanRepair()`.
- `timedValidation` is the single place a suite is measured, so counts and timings
  cannot drift from where validation runs; it also records per-category durations
  via `rec.AddValidation(validationCategory(r.Category), r.Duration)`.
- The recorder is snapshotted in a deferred function in `runStages`:
  `res.perf = rec.Task()` then `writeMetrics(rn, res.perf)`.

**Generation path is a wrapper, not a state machine.** `docs/reference/PERFORMANCE.md`
states timing "wraps existing operations in `runStages` (`internal/cli/run.go`); it
does not add a parallel state machine or change any lifecycle transition", matching
`Measure`'s contract in `internal/perf/perf.go` ("wraps an existing operation without
changing its control flow").

---

## 3. Persistence and scope

| Artifact | Written by (source path) | Scope | Read by |
| --- | --- | --- | --- |
| `.agent-sdlc/runs/<task-id>/metrics.json` | `internal/cli/run.go` `writeMetrics(rn, perf.Task)` (called from the `runStages` deferred snapshot) | per task | external readers; not read back for decisions |
| `performance` field in `.agent-sdlc/runs/<task-id>/report.json` | `internal/cli/run.go` `runReportDoc.Performance perf.Task` (`json:"performance"`), written by `writeRunJSON(rn, "report.json", ...)` in `executeLifecycle` | per task | `internal/cli/report.go` (`doc.Performance`, `perf.WriteTask`) |
| `.agent-sdlc/runs/<plan-id>/metrics.json` | `internal/perf/perf.go` `Run` + `Run.Add`/`Run.Finish`; `WriteRun` renders it (plan aggregate a full `sop run` leaves behind, per `docs/reference/PERFORMANCE.md`) | per run / plan aggregate | `internal/cli/report.go` `loadRunMetrics` → `perf.WriteRun` |

Observed artifacts confirming the shape (inspected):

- `.agent-sdlc/runs/CLOSE-007/metrics.json` — per-task record; fields `id`,
  `total_ms`, `stages_ms` (`implement`, `plan`, `validation`), `validation_ms`
  (`build`, `lint`, `test`), `counts` (`agent_calls`, `agent_calls_avoided`,
  `validation_runs`, `validation_reused`, `review_runs`, `review_reused`,
  `fix_cycles`, `plan_repairs`). This is the base contract exactly as declared in
  `internal/perf/perf.go`.
- `.agent-sdlc/runs/CLOSE-008/` — present for this task; at inventory time it held
  `activity.jsonl`, `model-selection.json`, `plan.md`, `state.json`, `task.md`.
  `metrics.json` / `report.json` are written by the deferred snapshot and the
  lifecycle completion respectively, so they appear once the run completes.

A missing or older artifact is handled gracefully: `internal/cli/report.go`
`writePerformance` prints "No timing was recorded for this run." rather than
inventing values.

---

## 4. Readers

| Reader | Source path | What it reads |
| --- | --- | --- |
| `sop report` performance section | `internal/cli/report.go` (`writePerformance`, `loadRunMetrics`, `perf.WriteRun`, `perf.WriteTask`) | plan aggregate `.agent-sdlc/runs/<plan-id>/metrics.json` first (via `activePlanID`), else the `performance` field of the task `report.json` |
| `sop run` concise per-task line | `internal/cli/run.go` `emitRunSummary` (`res.perf.Line()`), rendered by `perf.Task.Line` | in-memory `perf.Task` snapshot |
| Drive/scheduler projection | `internal/cli/drive.go` (imports `internal/perf`) | the same contract for scheduling/drive reporting |
| JEV projection | `internal/cli/jev.go` (imports `internal/perf`); JEV artifact consumed by `internal/cli/jev_artifact.go` | the `JEV` sub-record |
| Controller projection | external controller repository (`internal/sopclient/performance.go` per `docs/plans/PLAN-Pre-Performance-Closure.md` line 56 and `docs/reports/pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md`). This file is **not present in this repository** — the controller consumes the same per-task/plan metrics contract read-only. | per-task/plan metrics contract (read-only) |

Note: `internal/sopclient/performance.go` is a controller-repository path cited by the
plan; it does not exist in this repository (verified by search: only plan/report
references, no file). It is recorded as a read-only projection of the same contract,
not as a second store.

---

## 5. Stages / operations / validation carried

- Stages: `plan`, `implement`, `validation`, `review`, `fix` (`internal/perf/perf.go`
  stage constants; recorded only when the stage actually ran).
- Validation breakdown: `build`, `test`, `lint` (`CategoryBuild`/`CategoryTest`/
  `CategoryLint`; `validationCategory` in `internal/cli/run.go` maps runner
  categories to these).
- Operations counted: agent calls (PLAN/IMPLEMENT/FIX), agent calls avoided
  (verify-first), validation runs/reused, review runs/reused, fix cycles, plan
  repairs (`Counts` in `internal/perf/perf.go`).
- JEV: invocations, duration, provider/model, tool calls, findings, blocking
  findings (`JEV` in `internal/perf/perf.go`).
- Run aggregate: `started_at`, `total_ms`, `tasks`, `counts`, `jev` (`Run` in
  `internal/perf/perf.go`), plus the derived `CategoryMS()` split.

---

## 6. Model-provider / token / retry / tool information (what exists vs UNAVAILABLE)

- **Agent-call counts are lifecycle operation counts, not provider generation
  calls.** `internal/perf/perf.go` documents `Counts.AgentCalls` as implementation-
  agent calls (PLAN, IMPLEMENT, FIX) with REVIEW counted separately;
  `docs/reference/PERFORMANCE.md` states an agent-call count "is a lifecycle
  operation count, not the number of provider generation requests or provider
  retries".
- **Stage durations are not exact LLM time.** `docs/reference/PERFORMANCE.md`:
  "Agent-stage duration includes harness and tool overhead; it is not exact LLM
  time." No per-call provider duration exists in `internal/perf/perf.go`.
- **The only provider/model field is JEV-specific.** `JEV.Provider` / `JEV.Model`
  (`internal/perf/perf.go`, `json:"provider,omitempty"`/`"model,omitempty"`) apply
  to JEV only and are empty when the analyzer did not name them (not an error).
  They do not prove provider/model generation counts for PLAN/IMPLEMENT/FIX/REVIEW.
- **Tool calls are counted only inside the JEV sub-record** (`JEV.ToolCalls`). No
  general per-stage tool-call counter exists.
- **Retries/escalation:** the failure classification and autonomy decision are
  persisted (`classification.json`, `report.json` `classification` field) and
  attempt records exist for Phase 5 escalation, but `internal/perf/perf.go` carries
  no retry/escalation counter; `plan_repairs`/`fix_cycles` are the only repair-ish
  counts and are lifecycle counts, not provider retries.
- **Tokens:** no input/output token field exists anywhere in `internal/perf/perf.go`.
  `docs/reference/PERFORMANCE.md` explicitly lists input/output tokens as not
  recorded.
- **UNAVAILABLE rule.** True model-call counts and exact LLM generation time are
  UNAVAILABLE because no existing artifact proves them. They are never inferred
  from stage-level agent duration, from text size, or from diff size.

---

## 7. Classification matrix

Legend: **A** = AVAILABLE, **P** = PARTIAL, **M** = MISSING (present measurable
lifetime does the work but does not record it), **N/A** = NOT APPLICABLE
(future-phase architecture absent today). Every row cites an existing source path,
artifact or schema element.

| # | Data point | Status | Source / artifact / schema evidence | Unit | Observation scope |
| --- | --- | --- | --- | --- | --- |
| 1 | Prompt compilation | N/A | No compiler exists. `docs/plans/BACKLOG.md` defers the Prompt Compiler; `docs/reference/PERFORMANCE.md` does not record it; `internal/perf/perf.go` has no field. Future-phase architecture absent. | — | — |
| 2 | Context construction | M | SOP builds a fix/implement input (e.g. `fixContext` in `internal/cli/run.go`) but records no construction metric: `internal/perf/perf.go` has no field; `docs/reference/PERFORMANCE.md` lists context sizes as not recorded. | — | present measurable gap |
| 3 | Cache lookup | P | Two caches exist: validation/review session cache (`internal/cli/session.go`, used by `sessionValidation`/`cachedReview` in `internal/cli/run.go`) and the JEV record. The *reuse counts* are recorded (`validation_reused`, `review_reused` in `Counts`, `internal/perf/perf.go`), but no lookup-attempt count or lookup duration exists. | count | per-task/per-run, cached path only |
| 4 | Cache hit-miss | P | Hits are recorded as `validation_reused` / `review_reused` (`internal/perf/perf.go` `Counts`; incremented by `rec.ValidationReused()`/`rec.ReviewReused()` in `internal/cli/run.go`). Misses are implicit (runs recorded via `validation_runs`/`review_runs`), not an explicit miss counter. | count | per-task/per-run |
| 5 | Indexing | N/A | No structural index exists; `docs/plans/BACKLOG.md` defers the structural index. No field in `internal/perf/perf.go`. | — | — |
| 6 | RAG retrieval | N/A | No retrieval subsystem exists; `docs/plans/BACKLOG.md` defers BM25/vector RAG. No field in `internal/perf/perf.go`. | — | — |
| 7 | Result count | N/A | No retrieval, so there is no result set to count; no field in `internal/perf/perf.go`. | — | — |
| 8 | Context size | M | Not recorded. `docs/reference/PERFORMANCE.md` explicitly lists context sizes as not recorded; `internal/perf/perf.go` has no size field. Present lifecycle builds context (`fixContext`, request `Input`) but does not measure it. | — | present measurable gap |
| 9 | Model execution | P | Lifecycle operation counts exist (`agent_calls`, `review_runs`, `Counts`, `internal/perf/perf.go`) and stage durations exist (`stages_ms`, per stage). But these are NOT provider generation calls and NOT exact LLM time (`docs/reference/PERFORMANCE.md`). | count / ms | PLAN/IMPLEMENT/FIX/REVIEW stages, includes harness+tool overhead |
| 10 | Input/output tokens | M | Not recorded. `docs/reference/PERFORMANCE.md` lists input/output tokens as not recorded; `internal/perf/perf.go` has no token field. True token counts are UNAVAILABLE and never inferred from text size. | — | present measurable gap |
| 11 | Tool execution | P | The only existing per-task tool-call counter is `JEV.ToolCalls` (`internal/perf/perf.go`), scoped to JEV only. No general per-stage tool counter or per-tool timing exists (`docs/reference/PERFORMANCE.md`). Existing audit/trace artifacts (e.g. `.agent-sdlc/runs/<task-id>/activity.jsonl`; `.agent-sdlc/runs/prompts/prompt-<id>/`) may supply *observed* tool activity where enabled, as-is, without new instrumentation. | count | JEV-only in the metrics schema; observed-only via enabled audit/trace artifacts |
| 12 | Normalization | N/A | No Response Normalizer exists; `docs/plans/BACKLOG.md` defers it. No field in `internal/perf/perf.go`. | — | — |
| 13 | Validation | A | `stages_ms["validation"]` and `validation_ms{build,test,lint}` (`internal/perf/perf.go` `Task.ValidationMS`, `CategoryBuild/CategoryTest/CategoryLint`); counts `validation_runs`/`validation_reused`; generated by `timedValidation`/`sessionValidation` in `internal/cli/run.go`. Observed in `.agent-sdlc/runs/CLOSE-007/metrics.json`. | ms / count | per task, per category |
| 14 | Retry / escalation | P | `fix_cycles` and `plan_repairs` (`Counts`, `internal/perf/perf.go`) record lifecycle repair counts; escalation exists as attempt/RoutingSourceEscalation evidence (`internal/cli/run.go`, `report.json`) but is not a `perf` field, and provider retries are not counted. | count | per task (lifecycle repairs); attempt records for escalation |
| 15 | Selected model | P | `JEV.Provider`/`JEV.Model` (`internal/perf/perf.go`) record provider/model for JEV only. For routing, `model-selection.json` and `report.json` `model_selection`/`routing` (`internal/cli/run.go`) record the selected class/provider/model — but these are run artifacts, not `perf` metrics. | name | JEV (perf) / per run (routing artifacts) |
| 16 | Model class | P | The selected class is recorded in routing evidence (`model-selection.json`, `report.json` `routing`/`model_selection` via `internal/cli/run.go`), but there is no `model_class` field in `internal/perf/perf.go`. | name | per run (routing artifacts, not perf) |

---

## 8. Future-phase NOT APPLICABLE (separated from present gaps)

These are NOT APPLICABLE today because the deferred architecture does not exist
(`docs/plans/BACKLOG.md` deferral list; no field in `internal/perf/perf.go`):

| Data point | Reason it is N/A today | Deferred owner (evidence) |
| --- | --- | --- |
| Prompt compilation | Prompt Compiler not implemented | `docs/plans/BACKLOG.md`; `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Future telemetry data points" |
| Indexing | Structural index not implemented | `docs/plans/BACKLOG.md` |
| RAG retrieval | BM25/vector RAG not implemented | `docs/plans/BACKLOG.md` |
| Result count | No retrieval result set exists | `docs/plans/BACKLOG.md` |
| Normalization | Response Normalizer not implemented | `docs/plans/BACKLOG.md` |

These will become AVAILABLE/PARTIAL only once the corresponding subsystem is built;
they are not measurable gaps in the current lifecycle.

---

## 9. Present measurable gaps (separated from future-phase N/A)

These are things the current lifecycle *does* execute but does not record. Each
states what would be needed to measure it — recorded as gap input, not as work
started here.

| Data point | Why it is a present gap | What would be needed to measure it (future plan input) |
| --- | --- | --- |
| Tool execution | Only JEV `tool_calls` is counted; no general per-stage tool counter or timing (`internal/perf/perf.go`; `docs/reference/PERFORMANCE.md`). | A new counter/timer in `internal/perf` (extended base contract) incremented at existing call sites; or derived from existing enabled audit/trace artifacts as-is. |
| Input/output tokens | No token field anywhere (`internal/perf/perf.go`; `docs/reference/PERFORMANCE.md`). Never inferred from text size. | A provider-reported token count field; unavailable until a provider surface supplies it. |
| Context size | Context is built but not measured (`fixContext` in `internal/cli/run.go`; `docs/reference/PERFORMANCE.md`). | A recorded size (bytes/tokens) field on the request/recorder; not inferred from text length. |
| Model execution (exact) | Stage durations include harness+tool overhead and are not exact LLM time (`docs/reference/PERFORMANCE.md`). | A per-provider-call duration recorded at the provider boundary, distinct from `stages_ms`. |
| Retry / escalation (provider) | Only lifecycle `fix_cycles`/`plan_repairs` exist; no provider retry counter. | A provider-retry counter distinct from lifecycle repair counts. |

True model-call counts and exact LLM generation time are **UNAVAILABLE**: no existing
artifact proves them, and they are never derived from stage-level agent duration.

---

## 10. Non-inference and UNAVAILABLE rules (applied)

1. Exact LLM time is never inferred from `stages_ms[plan|implement|fix]`; those
   stages include orchestration and tool overhead (`docs/reference/PERFORMANCE.md`).
2. True model-call counts are never equated with `agent_calls`/`review_runs`;
   those are lifecycle operation counts (`internal/perf/perf.go` `Counts`).
3. Token counts are never inferred from text size or diff size; there is no token
   field and no such inference appears in this report.
4. Every classification cites an existing source path, artifact or schema element;
   no value is asserted without evidence, and unmeasurable values read UNAVAILABLE
   or MISSING with the reason.
5. No competing metrics store and no new instrumentation architecture is
   introduced: this report adds no schema field, no artifact, no log and no store,
   and cites existing audit/trace logs only as-is.

---

## 11. Deterministic validation self-check

| Acceptance criterion | Report section(s) satisfying it |
| --- | --- |
| Authoritative schema, generation, persistence and readers recorded (`internal/perf`, per-task `metrics.json`, report.json `performance`, plan aggregate `metrics.json`) | §1, §2, §3, §4 |
| Each listed data point classified with evidence; future-phase N/A separated from present gaps | §7 (all 16 rows), §8 (N/A), §9 (present gaps) |
| Exact LLM time not inferred from stage-level agent duration; true model-call counts/LLM time marked UNAVAILABLE | §6, §10, and rows 9/10 in §7 |
| No competing metrics store / no new instrumentation; audit/trace used as-is | §7 row 11, §10 rule 5, and the preamble |
| `internal/perf` scope (`stages_ms`, `validation_ms`, `total_ms`, counts, `CategoryMS`, `WriteTask`, `WriteRun`) recorded as base contract to extend later | §1 ("Base contract") and §5 |

Check results:

1. Every classification cites an existing source path, artifact or schema element.
   PASS — each §7 row cites `internal/perf/perf.go`, `internal/cli/run.go`,
   `docs/reference/PERFORMANCE.md`, `docs/plans/BACKLOG.md`, or a specific
   `.agent-sdlc/runs/...` artifact.
2. No inferred value (no LLM time from stage duration; no tokens from text size).
   PASS — §10 rules 1–4; rows 9 and 10 mark exact values UNAVAILABLE/MISSING.
3. No competing metrics store or new instrumentation architecture. PASS — §10
   rule 5; the report introduces no field/artifact/log/store.
4. Base contract recorded as extend-later. PASS — §1, §5.
5. Future-phase N/A separated from present gaps. PASS — §8 vs §9.

Citation existence checks (inspected at check time):

- `internal/perf/perf.go` — exists (read in full).
- `internal/cli/run.go` — exists (read; `runStages`, `timedValidation`,
  `sessionValidation`, `writeMetrics` present).
- `internal/cli/report.go` — exists (read; `writePerformance`,
  `loadRunMetrics` present).
- `docs/reference/PERFORMANCE.md` — exists (read).
- `docs/plans/PLAN-Pre-Performance-Closure.md` — exists (referenced).
- `.agent-sdlc/runs/CLOSE-007/metrics.json` — exists on disk (read).
- `.agent-sdlc/runs/CLOSE-008/` — exists on disk; at inventory time held
  `activity.jsonl`, `model-selection.json`, `plan.md`, `state.json`, `task.md`.
- `internal/sopclient/performance.go` — NOT present in this repository (external
  controller path cited by the plan); recorded as a read-only projection, not as a
  local artifact.

Sole intentional mutation: this report file. No production code, configuration or
unrelated user change was touched; pre-existing working-tree changes are preserved.

Deterministic presence/nonemptiness proof for this artifact:

```bash
test -s docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md && echo PRESENT
go build ./... && go test ./... && go vet ./...
```

Observed result: the report file exists and is nonempty after this write.
