# PLAN --- EV-CONTEXT-RUN: Context Engine Live-Path Evaluation

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** completed read-only Phase 8 Context Engine integration audit (2026-10-07)\
**Supersedes:** none — this plan does **not** reopen Phase 8 (closed) or POST8-001 (historical)

## Project

EV-CONTEXT-RUN — Context Engine Live-Path Evaluation

## Summary

Decide, on measurable evidence, whether the existing Phase 8 retrieval facilities
(structural repository index, `internal/repoindex`, CTX-002; deterministic BM25
retrieval, `internal/retrieval`, CTX-003) should be wired into the **live `sop run`
context-building path**, which today supplies only changed-file, plan, task,
execution, recovery, and (opt-in) decision-memory evidence. The audit established
that the retrieval facilities exist, are tested, and are provider/model-neutral, but
are reachable only through the operator commands `sop index`, `sop retrieve`, and
`sop gate retrieve|vector` — never from `sop run`.

The work is staged behind two evidence gates so that no production behavior changes
ahead of evidence:

```text
E1  offline, model-free evaluation (existing capabilities; no production change)
        |
        +-- NO-GO / HOLD --> stop (keep the baseline) ; recorded as evidence
        |
        v  GO
E2  live A/B evaluation (requires human approval + a minimal, opt-in, gated seam)
        |
        +-- NO-GO / HOLD --> keep the baseline
        |
        v  GO --> a SEPARATE, later plan authorizes any production change
```

This plan ends at the **evaluation decision**. It authorizes **no** production
change: the retrieval seam needed for E2 is explicitly gated behind a human
architecture decision (see "Unresolved Architecture Decision").

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `agentic-sop` builds and tests with the repository Go toolchain
  (`go build ./...` and `go test ./internal/context/... ./internal/repoindex/...
./internal/retrieval/... ./internal/retrievalgate/... ./internal/vectoreval/...`
  all pass on HEAD `388ea56`).
- Owner: operator-supplied development environment
- Location: Go executable available on PATH

### Existing retrieval/index/gate facilities — EXISTS

- Evidence: `internal/repoindex` (CTX-002), `internal/retrieval` (CTX-003),
  `internal/retrievalgate` (CTX-004), `internal/context` (CTX-001), `internal/prompt`
  (CTX-005), `internal/normalize` (CTX-006), with passing unit tests.
- Owner: `agentic-sop`
- Location: `internal/repoindex`, `internal/retrieval`, `internal/retrievalgate`,
  `internal/context`, `internal/prompt`, `internal/normalize`

### A configured model/provider for the live arm (E2 only) — UNKNOWN

- Evidence: the live arm must run the existing `sop run` workflow against a
  configured provider/model; no provider is selected or named by this plan.
- Owner: operator-supplied development environment
- Location: resolved by existing configuration/environment (unchanged)

## Goal

Produce a deterministic, reproducible, provider/model-neutral evaluation that
answers, with objective GO / HOLD / NO-GO criteria:

> Does supplying already-available structural + BM25 retrieval evidence to the
> `sop run` context materially improve retrieval quality, context efficiency, task
> completion, or execution cost, without regressing correctness, latency, or the
> existing governance boundaries?

## Non-Goals

- Do not implement the retrieval seam on the live path in this plan (that is the
  subject of the "Unresolved Architecture Decision" and a later plan).
- Do not reopen Phase 8 or POST8-001, and do not modify the archived Phase 8 evidence.
- Do not implement the persistent cross-run verification cache
  (`VERIFCACHE-PERSISTENCE`); it remains deferred.
- Do not evaluate Graphify; it remains deferred and unscheduled (see "Deferrals").
- Do not introduce or use token budgets, or any token-based routing/budget policy.
- Do not add third-party dependencies.
- Do not change lifecycle, approval, retry, replan, budget, verification,
  external-completion, or plan-lifecycle semantics.
- Do not change provider/model selection, escalation, or routing.
- Do not add provider-, model-, or task-domain-specific behavior.
- Do not modify unrelated workstreams (for example the active CONV plan) or their
  evidence.

## Architecture Invariant

The evaluation is expressed entirely over existing, provider-neutral facilities. The
E1 arm is model-free and in-process. The E2 arm reuses the existing `sop run`
lifecycle and the existing canonical context contract; it adds **no** authority, no
budget, and no provider/model dependency.

```text
repoindex.Build (CTX-002)
        |
        v
retrieval.New(...).Search (CTX-003)   <-- deterministic BM25, model-free
        |
        v
retrievalgate.Measure (CTX-004)       <-- precision@k / recall@k / MRR / bytes
        |
        +--> E1: offline evaluation over a fixed corpus (no production change)
        |
        v  (only if E1 = GO, and only with human approval)
sopctx.Context (CTX-001) -> prompt.Compile (CTX-005) -> agent -> ParseOutcome (CTX-006)
        |
        +--> E2: live A/B over the corpus (gated, opt-in, off by default)
```

The invariant must hold identically for every provider and model, and must not name
any provider, model, or task domain.

## Basis (authoritative, from the completed audit)

The completed read-only audit (2026-10-07) established:

- **Live context path** — `internal/cli/run.go` `runStages` builds the IMPLEMENT
  context via `implementContext` (`run.go:1787`) → `sopctx.FromInputs`
  (`internal/context/context.go:305`), then compiles the model input via
  `prompt.Compile` (`run.go:618`). The context carries: task/plan
  (`SourceTask`), changed-file paths only (`SourceRepository`), execution
  (`SourceExecution`), recovery (`SourceRecovery`), and opt-in decision memory
  (`SourceMemory`, gated by `cfg.ContextEfficiency.DecisionMemory`, `run.go:1826`).
- **Retrieval is not on the path** — `internal/repoindex` and `internal/retrieval`
  are imported only by `internal/cli/index.go`, `internal/cli/retrieve.go`, and
  (transitively) `internal/cli/gate.go`; `internal/cli/run.go` does not import them.
  `sopctx.Inputs` (`context.go:279`) has no retrieval input; the only retrieval-aware
  context builder is the Phase 9 `internal/orchestration/context_routing.go:49`
  (`Retrieved`), reachable only via `sop orchestrate`.
- **Retrieval gate is hermetic** — `sop gate retrieve` (`internal/cli/gate.go:44`)
  evaluates `retrievalgate.Evaluate(retrievalgate.Corpus(), ...)` over a **built-in,
  machine-independent corpus** (`internal/retrievalgate/gate.go:195`), not the live
  repository. It reports PASS/FAIL on precision@k/MRR and explicitly lists
  task-success/latency/tool-call signals as **not measured** (`gate.go:85`).
- **Context observability exists** — the run trace records the context summary
  (`internal/cli/trace.go:173` → `runtrace.ContextInfo`, `internal/runtrace/trace.go:97`):
  items, files, bytes, truncated, per-source counts.
- **Cost observability exists (partial)** — `internal/perf` records stage timings
  and counts (`run.go:1492` `timedValidation`, `perf.Recorder`), written to
  `metrics.json`.
- **Neutrality is enforced** — `internal/archtest` guards the Phase 8 core packages
  (`archtest/arch_test.go:9`, `guard_test.go`) and the pipeline
  (`neutrality_test.go`); all pass.

## Live Path (current, authoritative)

| Context source                                                   | Populated by `sop run`? | Evidence                                        |
| ---------------------------------------------------------------- | ----------------------- | ----------------------------------------------- |
| `SourceTask` (task + plan)                                       | Yes                     | `implementContext` `run.go:1829` → `FromInputs` |
| `SourceRepository` (changed-file paths only)                     | Yes                     | `run.go:1816-1819`, `context.go:338-351`        |
| `SourceExecution` (provider/model/class)                         | Yes                     | `run.go:1788-1794`, `executionContextText`      |
| `SourceRecovery` (prior attempt + validation failure)            | Yes                     | `run.go:1796-1814`                              |
| `SourceMemory` (decision memory)                                 | Opt-in                  | `run.go:1825-1828` gate                         |
| Retrieval / index evidence (`SourceRepository` from CTX-002/003) | **No**                  | no import; no `sopctx.Inputs` field             |

## Baseline Methodology (existing behavior, unchanged)

The baseline is the **current** behavior: the compiled IMPLEMENT context built from
`implementContext` with **no** retrieval evidence. It is measured two ways:

1. **Static baseline (E1).** For each corpus case, the baseline repository evidence
   is the unranked, stable-path-ordered candidate set (the same definition
   `retrievalgate.baselineOrder`, `gate.go:152`), and the baseline context carries
   only changed-file paths. This is deterministic and model-free.
2. **Live baseline (E2).** The existing `sop run` lifecycle with the retrieval gate
   **off** (the default), over the pinned corpus, capturing the trace context
   summary, stage timings, and terminal stage.

The baseline must be reproducible: pinned corpus task files, pinned repository state
(git ref), unchanged configuration, and unchanged budgets.

## Evaluation Methodology (existing capabilities)

### E1 — Offline, model-free (no production change)

For each corpus case, compute over the **live repository's** structural index and
BM25 ranking, using existing library calls only:

- `repoindex.Build(repoindex.Options{Root, Head, Dirty})` (`internal/repoindex/index.go`);
- `retrieval.New(retrieval.CandidatesFromIndex(idx)).Search(...)` (`internal/cli/retrieve.go:29`);
- `retrievalgate.Measure(ids, relevant, k, candidates)` (`gate.go:112`) for
  precision@k, recall@k, MRR, and context bytes.

Compare the retrieval-ranked candidate list against the unranked baseline over the
corpus. This is the live-repository analogue of `sop gate retrieve` and reuses its
exact metric rather than a weaker second implementation.

### E2 — Live A/B (requires approval + a gated seam)

Only after E1 = GO and a human architecture decision: run the corpus through the
existing `sop run` lifecycle with the retrieval evidence supplied to the context as a
new `Retrieved` input to `sopctx.Inputs`/`implementContext`, behind an opt-in gate
that defaults off (mirroring `context.decision_memory`). Capture per-run:
`trace.Context` summary, stage timings, terminal stage, and outcome. The seam, its
gate, and any spec/trace change are **not** authored by this plan.

## Metrics and Instrumentation Status

| Metric (required by the objective)                                 | Source today                                   | Status                                     |
| ------------------------------------------------------------------ | ---------------------------------------------- | ------------------------------------------ |
| Context relevance / retrieval quality (precision@k, recall@k, MRR) | `retrievalgate.Measure`                        | AVAILABLE                                  |
| Context size (items, files, bytes, truncated, sources)             | `runtrace.ContextInfo` (`trace.go:97`)         | AVAILABLE                                  |
| Task completion / failure rate                                     | task status + `trace.Termination.Stage`        | AVAILABLE                                  |
| Execution latency                                                  | `internal/perf` stage timings (`metrics.json`) | AVAILABLE                                  |
| Retrieval overhead (index build time, search time)                 | none on the live path                          | MISSING (see EV-004)                       |
| Input token consumption                                            | provider-dependent; not normalized             | MISSING / UNAVAILABLE (informational only) |

**Token-consumption rule (binding, from Phase 8 §26).** Token counts are recorded
**informationally at most**, when the provider reports them, and **never** as a
budget, routing input, or acceptance authority. A metric must have reliable
deterministic ownership before SOP uses it as an execution policy. Where no reliable
count exists, the value is recorded `UNAVAILABLE`, never invented.

## EV-001 — Live-Path Baseline and State Verification

Record the exact repository/lifecycle state and the current live context-building
path, so the evaluation baseline is unambiguous.

### Authoritative Inputs

- `internal/cli/run.go` (`implementContext`, `runStages`)
- `internal/context/context.go` (`Inputs`, `FromInputs`)
- `internal/prompt/compile.go` (`Compile`)
- `internal/cli/trace.go` (`traceContext`), `internal/runtrace/trace.go`
- `.agent-sdlc/plan.meta.json` (active plan state)

### Mutation Targets

- `docs/reports/ev-context-run/EV-001-live-path-baseline.md` — the only file this task
  writes; no production change.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/ev-context-run/EV-001-live-path-baseline.md` — the HEAD/branch/working-tree
  state, the active SOP plan id and lifecycle, and the exact live context-building path
  with file/line anchors, confirming retrieval is absent from it.

### Acceptance Criteria

- HEAD, branch, working tree, and active plan state are recorded verbatim.
- The live context path (`runStages` → `implementContext` → `FromInputs` → `prompt.Compile`)
  is recorded with file/line anchors.
- It is demonstrated (by import/reference) that `internal/repoindex` and
  `internal/retrieval` are absent from `internal/cli/run.go`'s path.
- No production change is made.

### Execution Contract

1. Verify `git` state and `.agent-sdlc/plan.meta.json`.
2. Read `run.go` (`runStages`, `implementContext`), `context.go`, `compile.go`.
3. Produce the baseline report.
4. Finish.

Expected first action: verify the repository state.

### Production-Change Scope

None

## EV-002 — Deterministic Evaluation Corpus and Baseline Protocol

Define a fixed, representative corpus and the pinned baseline protocol, so the
comparison is deterministic and not optimized against a single repository task.

### Authoritative Inputs

- `internal/retrievalgate/gate.go` (`Corpus`, `Candidates`, `Measure`, `baselineOrder`)
- `internal/repoindex/index.go`, `internal/retrieval/retrieval.go`
- `docs/history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md` §14 (evaluation corpus)

### Mutation Targets

- `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md` — design and
  corpus only; no production change.

### Dependencies

- EV-001

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md` — the corpus
  (one case per required category: single-file, multi-file, existing-test discovery,
  interface implementation, configuration change, documentation+code, bug fix,
  cross-package dependency, replan after failure), each with a pinned task file, a
  pinned repository ref, the labelled relevant IDs, and the exact reproducible baseline
  command set.

### Acceptance Criteria

- The corpus covers every category listed in the Phase 8 plan §14.
- Each case pins its task file and repository ref, so runs are reproducible.
- The baseline protocol (unranked stable-path order; changed-file-only context) is
  stated and is model-free.
- No production change is made.

### Execution Contract

1. Read `retrievalgate` corpus/metric and `repoindex`/`retrieval` APIs.
2. Define the corpus and the pinned baseline protocol.
3. Produce the report.
4. Finish.

Expected first action: read `internal/retrievalgate/gate.go`.

### Production-Change Scope

None

## EV-003 — Retrieval-Enabled Evaluation Using Existing Capabilities

Execute the **E1 offline, model-free** evaluation over the EV-002 corpus using the
existing `repoindex`, `retrieval`, and `retrievalgate` facilities, and record the
comparison against the baseline.

### Authoritative Inputs

- `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md`
- `internal/repoindex/index.go`, `internal/retrieval/retrieval.go`
- `internal/retrievalgate/gate.go` (`Measure`)
- `internal/cli/retrieve.go` (the existing live-index + BM25 usage precedent)

### Mutation Targets

- `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md` — the evaluation report.
- A test-only, model-free evaluation harness (if the existing packages do not already
  expose one) — a test file only; **no** production behavior change, no new dependency.

### Dependencies

- EV-002

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md` — per-case and aggregate
  precision@k, recall@k, MRR, and context bytes for baseline vs retrieval, plus the
  measured retrieval overhead (index build time and search time) for the live corpus.

### Acceptance Criteria

- The evaluation is model-free and deterministic: identical inputs yield an identical
  report.
- Every result exposes source, score, reason, and rank (CTX-003 contract).
- Ranking is unaffected by filesystem iteration order.
- The report states explicitly which required metrics are measured (retrieval quality,
  context size, retrieval overhead) and which are not (task success, latency, tokens).
- No production behavior change is made and no new dependency is added.

### Execution Contract

1. Read the EV-002 corpus protocol.
2. Build the index and run BM25 over the corpus; measure with `retrievalgate.Measure`.
3. Time index build and search separately.
4. Produce the report.
5. Finish.

Expected first action: read `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md`.

### Production-Change Scope

None (test-only harness at most; no runtime path is changed)

## EV-004 — Instrumentation Gap Assessment

Enumerate, from the implementation, what SOP can and cannot measure today, and
specify the **smallest** additions that would make the full comparison valid —
without implementing them.

### Authoritative Inputs

- `internal/perf` (stage timings, counts)
- `internal/runtrace/trace.go` (`ContextInfo`, `Termination`), `internal/cli/trace.go`
- `internal/cli/run.go` (`runStages` measurements)
- The metrics table in this plan

### Mutation Targets

- `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md` — the gap register.

### Dependencies

- EV-002
- EV-003

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md` — a table of each
  required metric marked AVAILABLE or MISSING, with the exact file/line anchor, the
  smallest proposed addition for each MISSING metric, and a note on which additions
  would change production (and therefore require the human decision) versus which are
  test-only.

### Acceptance Criteria

- Every metric in this plan's metrics table is classified with evidence.
- Retrieval-overhead instrumentation and token-usage capture are each addressed, the
  latter explicitly as informational-only and never policy.
- The report distinguishes production-affecting additions from test-only additions.
- No production change is made.

### Execution Contract

1. Read `internal/perf`, `internal/runtrace`, and the `run.go` measurement sites.
2. Classify each metric and propose the smallest addition.
3. Produce the register.
4. Finish.

Expected first action: read `internal/perf`.

### Production-Change Scope

None

## EV-005 — Acceptance Criteria, Decision Gate, and Approval Prerequisites

Fix the objective GO / HOLD / NO-GO criteria and the stop conditions against the
measured baseline, and enumerate the approvals required to proceed to E2.

### Authoritative Inputs

- `docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md`
- `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md`
- `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md`
- `docs/history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md` §13–15 (gate, corpus, success gate)

### Mutation Targets

- `docs/reports/ev-context-run/EV-005-acceptance-criteria.md` — the decision record.

### Dependencies

- EV-003
- EV-004

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/ev-context-run/EV-005-acceptance-criteria.md` — the GO/HOLD/NO-GO
  thresholds fixed against the measured baseline, the stop conditions, the decision
  procedure, and the prerequisite list requiring human approval.

### Acceptance Criteria

- The E1 decision rule is objective and uses only measured signals (precision@k, MRR,
  recall@k, context bytes, retrieval overhead).
- The E2 decision rule is objective and names the measured signals it may use (task
  completion, first-attempt success, iterations/tool-calls, latency, context size) and
  explicitly excludes tokens from authority.
- Stop conditions are stated and fail-closed.
- The prerequisite approvals for E2 are enumerated.
- No production change and no plan activation are performed.

### Execution Contract

1. Read the EV-002/003/004 reports.
2. Fix thresholds and the decision procedure.
3. Produce the decision record.
4. Finish.

Expected first action: read `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md`.

### Production-Change Scope

None

## Baseline / Evaluation Acceptance Criteria (GO / HOLD / NO-GO)

Thresholds are **fixed in EV-005 against the measured baseline**; the values below are
the decision _shape_, not pre-committed constants.

### Gate E1 — offline, model-free

- **GO** — retrieval evidence improves precision@k **and/or** MRR over the baseline on
  the corpus, with no recall@k regression, no context-byte regression beyond the
  configured bound, and measured retrieval overhead within the bound fixed in EV-005.
- **HOLD** — mixed or inconclusive results (for example gains on some categories
  only); expand or refine the corpus and re-run. Do not proceed to E2.
- **NO-GO** — no measurable ranking improvement, or a regression in recall or context
  size, or overhead above the bound. Keep the baseline; record and stop.

### Gate E2 — live A/B (only after E1 = GO and approval)

- **GO** — the treatment improves task completion / first-attempt success and/or
  reduces iterations / tool calls, with no correctness regression, no lifecycle/
  approval/verification change, no budget increase, and no latency regression beyond
  the bound fixed in EV-005, across the minimum number of runs fixed in EV-005, and
  the change is feature-gated and off by default.
- **HOLD** — inconclusive (variance exceeds the pre-fixed bound); add runs; do not
  authorize a production change.
- **NO-GO** — no improvement, or any correctness/latency/budget regression. Keep the
  baseline; record and stop.

### Stop Conditions (fail-closed)

Stop and report **HOLD or NO-GO** — never proceed — if any of the following occurs:

- any correctness, lifecycle, approval, retry, replan, budget, or verification
  semantics would change;
- any provider/model-specific behavior, or a non-neutral dependency, would be
  introduced;
- the model-free arm is non-deterministic, or a required metric cannot be measured;
- a token count would be used as budget, routing, or acceptance authority;
- a required prerequisite approval is absent.

## Prerequisites Requiring Human Approval

1. **E2 authorization.** A human architecture decision (see "Unresolved Architecture
   Decision") to add the minimal, opt-in retrieval seam to the live context path.
2. **Model-budget authorization.** Approval to consume model budget by running the
   corpus through the live `sop run` lifecycle (E2). `sop run` never commits, pushes,
   or merges; runs remain local and bounded.
3. **Spec/trace authorization.** Approval for any change to
   `docs/specs/AGENT-PROVIDER.md` or the run-trace schema that E2 would require.
4. **Token-metric authorization.** Confirmation that token consumption is recorded
   informationally only, never as policy.
5. **Corpus authorization.** Confirmation that the EV-002 corpus and its pinned refs
   are acceptable for repeated local runs.

E1 (EV-001…EV-005) requires no approval to _author and run_ as a model-free,
read-only evaluation; only the E1→E2 transition and any production change require the
approvals above.

## Unresolved Architecture Decision

E2 requires a production seam (a `Retrieved` input to
`sopctx.Inputs`/`implementContext`, behind an opt-in gate) that does not exist today.
Before any E2 work, the human must choose:

- **A. Authorize a minimal, opt-in, gated retrieval seam**, evaluated only after E1 =
  GO, with a separate implementation plan; or
- **B. Do not wire retrieval into the live path**, accepting that CTX-002/CTX-003
  remain operator-command capabilities, in which case E2 is **NOT_REQUIRED** and this
  plan concludes at the E1 gate.

EV-001…EV-005 remain valuable under either choice: they produce the first
**live-repository** evaluation of the Phase 8 retrieval facilities (the existing
`sop gate retrieve` corpus is hermetic) and a precise instrumentation register.

## Deferrals

- **`VERIFCACHE-PERSISTENCE`** — the persistent cross-run verification cache remains
  deferred (unchanged). `sop run` continues to use the in-memory `sessionValidation`
  reuse; `internal/verifcache` remains reachable only as `sop validate --cache`. Not
  in scope here. See `docs/plans/BACKLOG.md`.
- **Graphify evaluation** — reserved as the **last, unscheduled** candidate at the
  tail of the backlog, after the existing deferred entries. No Graphify artifact
  exists in this repository today; it is **not** evaluated, scheduled, or
  implemented by this plan.

## Safety Invariants

This plan must not:

- change any production runtime behavior (E1 is model-free; E2 is gated and opt-in);
- authorize mutations, bypass human approvals, or change commit/push authorization;
- alter task approval boundaries, retry, replan, budget, or verification semantics;
- change provider/model selection, escalation, or routing;
- weaken fail-closed behavior or silently reset retries;
- introduce token-based budgets or routing;
- add third-party dependencies;
- name or depend on any provider, model, or task domain in the evaluation logic;
- modify Phase 8 / POST8-001 evidence, or the active CONV plan and its evidence.

## Dependency Graph

```text
EV-001 (live-path baseline, read-only)
    |
EV-002 (corpus + baseline protocol, read-only design)
    |
EV-003 (E1 offline evaluation, model-free, test-only)
    |
EV-004 (instrumentation gap assessment, read-only)
    |
EV-005 (acceptance criteria + decision gate + approvals, read-only)
    |
    +-- E1 = GO  --> [human decision A] --> SEPARATE plan authorizes E2 (live A/B)
    +-- E1 = HOLD/NO-GO --> stop; keep the baseline; record as evidence

Deferred (unchanged, not scheduled here):
    - VERIFCACHE-PERSISTENCE (persistent cross-run verification cache)
    - Graphify evaluation (last, unscheduled, tail of backlog)
```
