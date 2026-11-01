# EV-004 — Instrumentation Gap Assessment

Read-only gap register for the EV-CONTEXT-RUN evaluation. It enumerates, from the
implementation, every metric required by the plan's "Metrics and Instrumentation
Status" table and classifies each as **AVAILABLE** or **MISSING**, with exact
file/line anchors, the smallest proposed addition for each MISSING metric, and a
per-row classification of each proposed addition as **production-affecting**
(requires the human decision) or **test-only**. **No production change is made.**

## 0. Source of the metrics table and inspected revision

**Metrics table source.** The authoritative enumeration this report classifies against
is the section **"Metrics and Instrumentation Status"** of
`docs/plans/PLAN-EV-Context-Run-Evaluation.md` (the active plan
`plan-ev-context-run-evaluation`; source sha256
`e6f8a3c67d4ea5d10835de8c48260976c1bb85512763eeb959000a0b3bae5865`, recorded in
`docs/reports/ev-context-run/EV-001-live-path-baseline.md` §2). That table has exactly
**six** metric rows, reproduced verbatim in §1 below. This report adds no metric that
is not in that table and omits none.

**Inspected revision.** The anchors below were derived at HEAD
`388ea56aef4b8e2976c73dc0e211069c695b620d` (branch `main`), the pinned ref recorded in
`EV-001` §1 and `EV-002` §3.2. File/line anchors are stable at this revision; drift is
handled by re-running this assessment, not by guessing.

**Read-only guarantee.** This task modifies no source, configuration, or policy file
and no `.agent-sdlc` state. The only artifact added is this report. See §6.

## 1. Classification table (one row per plan metric)

The six rows below are exactly the six metrics of the plan's metrics table, in the
plan's order, with the plan's own Status column carried into the verdict.

| # | Metric (plan wording) | Verdict | Exact anchor (AVAILABLE) / nearest anchor (MISSING) | Smallest proposed addition | Production-affecting or test-only |
| --- | --- | --- | --- | --- | --- |
| 1 | Context relevance / retrieval quality (precision@k, recall@k, MRR) | **AVAILABLE** | `internal/retrievalgate/gate.go:112` — `Measure(ids, relevant, k, candidates)` returns `Precision`, `Recall`, `MRR`, `Bytes` | none | — (already available) |
| 2 | Context size (items, files, bytes, truncated, sources) | **AVAILABLE** | `internal/runtrace/trace.go:97` — `ContextInfo` (`Items`, `Files`, `Bytes`, `Truncated`, `Sources`); populated at `internal/cli/trace.go:173` (`traceContext`) | none | — (already available) |
| 3 | Task completion / failure rate | **AVAILABLE** | `internal/runtrace/trace.go:180` — `Termination` (`Stage`, `Kind`, `Disposition`, `Action`, `HumanRequired`); populated at `internal/cli/trace.go:154` (`traceTermination`); stage/exit codes at `internal/runtrace/trace.go:184` (`Verification`) via `internal/cli/trace.go:143` (`traceVerification`) | none | — (already available) |
| 4 | Execution latency | **AVAILABLE** | `internal/perf/perf.go:118` — `Recorder.Measure(stage)`; stage totals persisted in `Task.StagesMS`/`Task.TotalMS` (`perf.go:90`). Live call sites: `internal/cli/run.go:1492` (`timedValidation`) and the `perf.NewRecorder`/`Measure` uses in `runStages` | none (index build/search latency is metric 5) | — (already available) |
| 5 | Retrieval overhead (index build time, search time) | **MISSING** | No timing on the retrieval/index call paths in the live path. Nearest anchors: `internal/repoindex/index.go` (`Build`) and `internal/retrieval/retrieval.go` (`Search`) are imported only by `internal/cli/index.go` and `internal/cli/retrieve.go`, never by `internal/cli/run.go`. `perf` has no `StageRetrieve`/`StageIndex` constant (`internal/perf/perf.go:20-26`). See §3. | Add two `perf` stage constants (`index`, `retrieve`) and wrap the existing `repoindex.Build` / `retrieval.Search` call sites with `Recorder.Measure(...)` in a **test-only** harness; only if retrieval is ever wired to the live path would a production `Measure` wrap at the `implementContext` site be needed. | **Test-only** as proposed (§3). A production wrap at the live context site is **production-affecting** and requires the human decision. |
| 6 | Input token consumption | **MISSING / UNAVAILABLE** | No token accounting anywhere in the pipeline. Anchors for the deliberate absence: `internal/context/context.go:10-11` (limits are bytes, "reliable provider-independent token accounting does not exist"), `internal/prompt/compile.go:15-16` (same rule), `internal/orchestration/budget.go:23-25` (no token/cost field), `internal/orchadopt/orchadopt.go:86` ("provider cost and token counts (no reliable provider-independent accounting)"). See §4. | Record the provider-reported count **informationally only** if/when a provider reports it; no new SOP-owned counter is proposed. | **Test-only / informational** — must never feed policy (§4). Any production field would be **production-affecting**. |

Metric count check: the plan's table has 6 rows; this table has 6 rows, 1:1 by name and order. No metric is added or omitted.

## 2. Evidence for the AVAILABLE rows

Each AVAILABLE anchor was reopened at the inspected revision and resolves to code that
genuinely produces the signal:

- **Row 1 — `retrievalgate.Measure`.** `internal/retrievalgate/gate.go:112` defines the
  metric entry point used by both the hermetic gate (`sop gate retrieve`) and the E1
  evaluation (EV-003 reuses it verbatim). It computes precision@k, recall@k, MRR, and
  context `Bytes` from a candidate list and a labelled relevant set — the exact metric
  the plan names for "Context relevance / retrieval quality".
- **Row 2 — `runtrace.ContextInfo`.** `internal/runtrace/trace.go:97` defines
  `ContextInfo{Items, Files, Bytes, Truncated, Sources}`, and
  `internal/cli/trace.go:173` (`traceContext`) populates it from `sopctx.Context`
  (`Items()`, `Files()`, `Bytes()`, `Truncated()`, `Sources()`). All five sub-signals
  the plan lists (items, files, bytes, truncated, sources) are present; nothing is
  missing.
- **Row 3 — task completion / failure rate.** `internal/runtrace/trace.go:180`
  (`Termination`) carries the terminal `Stage`, `Kind`, `Disposition`, `Action`, and
  `HumanRequired`; `internal/runtrace/trace.go:184` (`Verification`) carries each
  check's `Command`, `Status`, `ExitCode`, `DurationMS`. `internal/cli/trace.go:154`
  (`traceTermination`) and `internal/cli/trace.go:143` (`traceVerification`) populate
  them from the run's existing evidence. Completion/failure is therefore observable
  without new instrumentation.
- **Row 4 — execution latency.** `internal/perf/perf.go:118` (`Recorder.Measure`)
  times a stage with the monotonic clock and stores it in `StagesMS`; `Task.TotalMS`
  (`perf.go:90`) is the task wall-clock total; `Run.TotalMS` (`perf.go:130`) is the run
  total. `internal/cli/run.go:1492` (`timedValidation`) is the live validation call
  site. These are persisted to `metrics.json` and are diagnostic-only (never read back
  to drive a decision), which satisfies the plan's latency metric without changing
  behavior.

## 3. Retrieval-overhead instrumentation (dedicated subsection)

**Verdict: MISSING on the live path.**

**What exists.** Retrieval and indexing are real, testable facilities:
`repoindex.Build` (`internal/repoindex/index.go`) and the deterministic BM25
`retrieval.Search` (`internal/retrieval/retrieval.go`), plus
`retrievalgate.Measure` (`internal/retrievalgate/gate.go:112`) for quality. But no code
path emits **timing or counters** for them, and `internal/perf/perf.go:20-26` defines
only five stage constants (`plan`, `implement`, `validation`, `review`, `fix`) — there
is no `index` or `retrieve` stage. EV-003 records the overhead as manually-invoked
timing ("time index build and search separately"); that is an offline measurement, not
live instrumentation, so the plan's "index build time, search time" metric has no
production owner today.

**Reachability anchor (why nothing can measure it live).** `internal/cli/run.go`
imports neither `internal/repoindex` nor `internal/retrieval` (EV-001 §4), so
`implementContext` never builds an index or searches. There is therefore no live call
site that could be timed; the metric is MISSING by construction, not by omission in an
existing code path.

**Smallest proposed addition (test-only).** Add two stage constants,
`StageIndex = "index"` and `StageRetrieve = "retrieve"`, to `internal/perf/perf.go` and
use the existing `Recorder.Measure(StageIndex)` / `Recorder.Measure(StageRetrieve)`
around the `repoindex.Build` and `retrieval.Search` calls **inside the test-only E1
harness**. This requires no change to any live call site and no new dependency: it
reuses the existing `perf.Recorder`, which already measures any string stage name.

**Production-affecting variant (requires the human decision).** Only if retrieval were
ever wired into the live context path (the E2 seam that the plan explicitly defers)
would a production `Measure` wrap be needed at the `implementContext` site in
`internal/cli/run.go`. That variant is **production-affecting** and is not proposed for
execution here; it is gated behind the plan's "Unresolved Architecture Decision".

## 4. Token-usage capture (dedicated subsection)

**Verdict: MISSING / UNAVAILABLE — and it must remain informational-only and never
policy.**

**What exists.** Nothing in SOP counts tokens. This is a deliberate design choice, not
an oversight, and it is documented at the exact anchors below:

- `internal/context/context.go:10-11`: context limits "are sizes and files — never in
  tokens: reliable provider-independent token accounting does not exist, so a token
  budget would be authority SOP cannot justify."
- `internal/prompt/compile.go:15-16`: "Limits are expressed in bytes, never tokens:
  reliable provider-independent token accounting does not exist, so a token budget
  would be authority SOP cannot justify."
- `internal/orchestration/budget.go:23-25`: the orchestration budget "introduces NO
  token budget and NO provider-specific budget ... There is no field (and no
  dependency) naming tokens, cost ...".
- `internal/orchadopt/orchadopt.go:86`: token counts are listed among the things that
  have "no reliable provider-independent accounting".

The plan's own metrics table already classifies this row as `MISSING / UNAVAILABLE
(informational only)`, and its "Token-consumption rule (binding, from Phase 8 §26)"
states that token counts are recorded **informationally at most**, when the provider
reports them, and **never** as a budget, routing input, or acceptance authority.

**Smallest proposed addition (test-only / informational).** None as SOP-owned policy
code. The smallest faithful step is to record a **provider-reported** count, when one
is available, into a test-only observation sink for the E2 report — labelled
`UNAVAILABLE` when no reliable count exists, and never persisted into any gate, budget,
or routing field. No new SOP counter, no new dependency, and no new `perf`/`runtrace`
field is required by this plan stage.

**Binding constraint.** Where no reliable count exists the value is recorded
`UNAVAILABLE`, never invented. Token usage is **informational-only and never policy**;
using it as budget, routing, or acceptance authority is one of the plan's fail-closed
stop conditions ("a token count would be used as budget, routing, or acceptance
authority"). Any production field that captured tokens would be **production-affecting**
and would require the human decision, and is not proposed here.

## 5. Production-affecting vs test-only: the two groups

| Group | Proposed additions in this report | Execution requires |
| --- | --- | --- |
| **Test-only** | (a) two `perf` stage constants + `Measure` wraps inside the E1 harness for index/search overhead (§3); (b) provider-reported token count recorded into a test-only observation sink, `UNAVAILABLE` when absent (§4) | no human decision; no runtime behavior change; no new dependency |
| **Production-affecting** (requires the human decision) | (c) a production `Measure` wrap at the live `implementContext` site — only relevant if retrieval is ever wired to the live path (the deferred E2 seam); (d) any production field capturing token usage | the plan's "Prerequisites Requiring Human Approval" (E2 authorization, spec/trace authorization, token-metric authorization) and the "Unresolved Architecture Decision" |

Nothing in the **production-affecting** group is proposed for execution by this stage.
It is listed only so a reader of a MISSING row knows whether executing its addition would
require the human decision: for rows 5 and 6 the test-only additions above are sufficient
to make the comparison valid without any production change.

## 6. No production change is made

This report is a gap register, not an implementation. It contains **only proposals**;
none is applied. Specifically:

- No source file under `internal/` is created, modified, or deleted.
- No configuration or policy file is modified, and no `.agent-sdlc` state is touched.
- `internal/perf/perf.go` is **not** edited to add `StageIndex`/`StageRetrieve`; that
  addition is described, not implemented.
- No token-counting code is added; token usage remains observational-at-most and never
  policy.
- The only artifact added by this task is
  `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md`.

**No production change is made.** Pre-existing user-owned working-tree changes recorded
in EV-001 §1 are preserved unchanged.
