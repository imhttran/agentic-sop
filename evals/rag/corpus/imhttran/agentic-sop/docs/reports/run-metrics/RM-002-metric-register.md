# RM-002 — Metric Register and Classification

Status: read-only metric register. This report creates, modifies, or deletes no
production file. It is the sole write of the stage.

This register fixes the seven metric dimensions the Run Metrics and Observability
plan defines, each with (a) its source field — an artifact/field pair or an explicit
**no recorded source** — (b) the AVAILABLE / PARTIAL / MISSING classification
carried unchanged from the CLOSE-008 telemetry classification matrix, with an anchor
citing an existing source path, artifact, or schema element, (c) the eligible-run
denominator, (d) the coverage rule, and (e) the exact zero-vs-`UNAVAILABLE` handling.
It closes with the binding token rule.

RM-002 introduces no metric that the assessment did not already name and omits none
it named. The seven dimensions below are exactly the plan's
`PLAN-Run-Metrics-And-Observability.md` "Metric Register (the seven dimensions)"
rows, in the plan's order:

1. Agent success / failure rate
2. Routing / finding-severity distribution
3. Fix-loop convergence
4. Retry efficiency
5. Human-approval frequency
6. Execution latency
7. Token usage

The classification vocabulary is carried from
`docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md` §7
(**A** = AVAILABLE, **P** = PARTIAL, **M** = MISSING) and its non-inference rules in
§6/§10; the per-metric eligible-run denominators are carried from
`docs/reports/run-metrics/RM-001-artifact-inventory.md` §5.2, which is computed
against the 164 run directories (census 2026-10-08). The token rule is restated from
`docs/reports/ev-context-run/EV-004-instrumentation-gaps.md` §4 and CLOSE-008 §6.

---

## 0. Classification vocabulary and denominators (carried, not re-derived)

- **AVAILABLE** — the artifact is present and populated for the run; the metric is
  measurable without inference.
- **PARTIAL** — the signal is present but not for every run and/or has no dedicated
  field in the authoritative schema (`internal/perf`); it is read from a run artifact
  rather than a metrics field.
- **MISSING / UNAVAILABLE** — the lifecycle does not record it and no existing
  artifact proves it. It is never inferred.

Denominator source: `docs/reports/run-metrics/RM-001-artifact-inventory.md` §1 and
§5.2 fix the corpus at **164 run directories** and give the eligible-run denominator
per metric. No dimension below widens its denominator by zero-filling runs whose
artifact or field is `UNAVAILABLE`.

---

## 1. Register (the seven dimensions)

### Dimension 1 — Agent success / failure rate — AVAILABLE

- **Source field.** `report.json` review/report payload decision fields (`decision`,
  `verdict`, summary) read through `internal/run` (`run.go`, `report.json` shape),
  with the run-trace terminal classification (`Termination` in
  `internal/runtrace/trace.go`, populated by `traceTermination` in
  `internal/cli/trace.go`) as the failure-stage anchor. Anchor artifact/field:
  `.agent-sdlc/runs/<TASK-ID>/report.json` `decision`; `trace.json`
  `Termination.Stage` / `Disposition`.
- **Classification (carried from CLOSE-008).** **A — AVAILABLE.** CLOSE-008 §7 row 3
  (task completion / failure observability as evidenced by `runtrace.Termination`)
  and §7 row 9 (model execution lifecycle operation counts) both anchor the present
  lifecycle records that make success/failure observable without new instrumentation.
- **Denominator.** **132 runs** — the runs carrying `report.json`
  (RM-001 §2: 132 carrying / 32 missing / 80.5% of 164). Where a terminal trace
  (`trace.json`, 39 runs) is used as the secondary anchor, its denominator is **39**
  (RM-001 §2: 39 carrying / 23.8%). Each sub-signal states which denominator it uses.
- **Coverage rule.** Coverage is `runs carrying report.json / 164` = **80.5%** for the
  `report.json` decision signal, and `runs carrying trace.json / 164` = **23.8%** for
  the `Termination` signal. A run with neither artifact is `UNAVAILABLE` for this
  dimension and is excluded from the metric denominator; it is never counted as a
  failure or a success.
- **Zero vs `UNAVAILABLE`.** A present decision field explicitly recording a
  terminal outcome (for example a `FAILED` / `NEEDS_HUMAN` verdict) is a **recorded
  measurement**. An absent `report.json`/`trace.json` is **`UNAVAILABLE`** — the run's
  outcome is unknown, not a recorded zero and not a failure.

### Dimension 2 — Routing / finding-severity distribution — PARTIAL

- **Source field.** `model-selection.json` (and the routing evidence in
  `report.json` `routing`/`model_selection`; `internal/cli/run.go`,
  `internal/run/routing_artifact.go` `RoutingArtifact`/`RoutingSignals`) for the
  routing mix; `review.json` severities for the finding-severity distribution.
  Anchors: `.agent-sdlc/runs/<TASK-ID>/model-selection.json` (`class`, `provider`,
  `model`, `source`); `.agent-sdlc/runs/<TASK-ID>/review.json` finding severities.
- **Classification (carried from CLOSE-008).** **P — PARTIAL.** CLOSE-008 §7 row 15
  (Selected model) and row 16 (Model class) are PARTIAL: the selected
  class/provider/model is recorded in the routing artifacts, but there is no
  `model_class` field in the authoritative `internal/perf` schema, and routing is
  opt-in so it is recorded for only a subset of runs.
- **Denominator.** **50 runs** — the runs carrying `model-selection.json`
  (RM-001 §2 and §5.2: 50 carrying / 114 missing / 30.5% of 164). The
  finding-severity sub-signal uses the `review.json` denominator of **164 runs**
  (RM-001 §2: 164 carrying / 100.0%).
- **Coverage rule.** Routing-mix coverage is `50 / 164` = **30.5%**; finding-severity
  coverage is `164 / 164` = **100.0%** for `review.json` presence (a review artifact
  that carries no severities is still a present artifact but contributes no
  severity value, recorded per the zero-vs-`UNAVAILABLE` rule below). Runs without
  `model-selection.json` are `UNAVAILABLE` for the routing mix and are excluded from
  its denominator, never counted as a "default" routing choice; partial coverage is
  why this dimension is PARTIAL.
- **Zero vs `UNAVAILABLE`.** An absent `model-selection.json` is **`UNAVAILABLE`**
  for the routing mix (not a routing selection of `default`). A present
  `review.json` with a recorded severities object is a **recorded measurement**;
  a review artifact that records no severity field is `UNAVAILABLE` for that
  sub-signal, not a count of zero findings.

### Dimension 3 — Fix-loop convergence — AVAILABLE

- **Source field.** `metrics.json` `counts.fix_cycles` and `counts.plan_repairs`
  (`internal/perf/perf.go` `Counts`), with `report.json` `fix_cycles` as the
  run-level review/report anchor. Anchor artifact/field:
  `.agent-sdlc/runs/<TASK-ID>/metrics.json` `counts.fix_cycles` / `counts.plan_repairs`;
  `internal/perf/perf.go` `Counts.FixCycles` / `Counts.PlanRepairs`
  (`json:"fix_cycles"` / `json:"plan_repairs"`).
- **Classification (carried from CLOSE-008).** **A — AVAILABLE.** CLOSE-008 §7 row 14
  (Retry / escalation) records `fix_cycles` and `plan_repairs` as present lifecycle
  repair counts, and §1 lists them in the base `Counts` contract to extend later.
- **Denominator.** **146 runs** — the runs carrying `metrics.json`
  (RM-001 §2 and §5.2: 146 carrying / 18 missing / 89.0% of 164).
- **Coverage rule.** Coverage is `146 / 164` = **89.0%**. Runs without `metrics.json`
  are `UNAVAILABLE` for fix-loop convergence and are excluded from the denominator, so
  a run that recorded no metrics is *unmeasured*, not "converged in 0 cycles".
- **Zero vs `UNAVAILABLE`.** A present `metrics.json` whose `counts.fix_cycles`
  explicitly holds `0` is a **recorded zero** — a real measurement that a fix loop
  ran and converged without an iteration — and it does count toward the denominator
  and numerator. An absent `metrics.json` (or an absent `counts` field) is
  **`UNAVAILABLE`** and is never conflated with the recorded zero.

### Dimension 4 — Retry efficiency — PARTIAL

- **Source field.** `attempt.txt` (free-text attempt/iteration record,
  `internal/run/attempt.go`), `continuations.txt` (free-text continuation record),
  and `classification.json` (task classification used for routing and budget) as the
  escalation/attempt evidence (`RoutingSourceEscalation` evidence in
  `internal/run/routing_artifact.go`). Anchors:
  `.agent-sdlc/runs/<TASK-ID>/attempt.txt`; `.agent-sdlc/runs/<TASK-ID>/continuations.txt`;
  `.agent-sdlc/runs/<TASK-ID>/classification.json`.
- **Classification (carried from CLOSE-008).** **P — PARTIAL.** CLOSE-008 §7 row 14
  (Retry / escalation) is PARTIAL: escalation exists as attempt/RoutingSourceEscalation
  evidence, but `internal/perf` carries **no** retry/escalation counter and provider
  retries are not counted; `fix_cycles`/`plan_repairs` are lifecycle repair counts,
  not provider retries.
- **Denominator.** **60 runs** for the `attempt.txt` sub-signal (RM-001 §2: 60
  carrying / 36.6% of 164), **15 runs** for `continuations.txt` (RM-001 §2: 15
  carrying / 9.1%), and **31 runs** for `classification.json` (RM-001 §2 and §5.2: 31
  carrying / 18.9%). Each sub-signal uses its own denominator; none is widened.
- **Coverage rule.** Coverage is `attempt.txt 60 / 164 = 36.6%`,
  `continuations.txt 15 / 164 = 9.1%`, and `classification.json 31 / 164 = 18.9%`.
  A run missing the artifact that carries a sub-signal is `UNAVAILABLE` for that
  sub-signal and excluded from its denominator. The partial coverage across the three
  artifacts is why this dimension is PARTIAL, not full retry accounting.
- **Zero vs `UNAVAILABLE`.** An absent `attempt.txt`/`continuations.txt` is
  **`UNAVAILABLE`**, never a recorded "0 retries". Free-text attempt content is read
  as-is; no retry count is inferred from the text and no count is inferred from text
  length, byte size, or diff size. A present `classification.json` with a recorded
  class is a **recorded measurement**; a missing classification artifact is
  `UNAVAILABLE`.

### Dimension 5 — Human-approval frequency — AVAILABLE

- **Source field.** `report.json` `decision` (the `NEEDS_HUMAN` terminal decision)
  and `approval.json` current request head (`internal/run/approval.go`
  `ApprovalArtifactName` = `approval.json`, `ApprovalHistory` =
  `approval-history.json`). Anchors:
  `.agent-sdlc/runs/<TASK-ID>/approval.json` (`domain.ApprovalRequest`);
  `.agent-sdlc/runs/<TASK-ID>/report.json` `decision`.
- **Classification (carried from CLOSE-008).** **A — AVAILABLE.** The approval request
  and its decision history are SOP-owned lifecycle provenance persisted by
  `SaveApproval`/`AppendApprovalHistory` (`internal/run/approval.go`); the terminal
  `NEEDS_HUMAN` decision is recorded in the run report. CLOSE-008 §7 does not mark
  approval evidence MISSING; the persisted artifacts make the frequency observable
  without new instrumentation.
- **Denominator.** **7 runs** — the runs carrying `approval.json` (RM-001 §2: 7
  carrying / 157 missing / 4.3% of 164). The `report.json` `decision=NEEDS_HUMAN`
  sub-signal uses the **132-run** `report.json` denominator (RM-001 §2: 80.5%).
- **Coverage rule.** Approval-artifact coverage is `7 / 164` = **4.3%**; the
  `report.json` decision sub-signal coverage is `132 / 164` = **80.5%**. A run with no
  `approval.json` is `UNAVAILABLE` for the approval-artifact signal and excluded from
  its denominator; absence is not a recorded "no approval requested".
- **Zero vs `UNAVAILABLE`.** A present `approval.json` carrying a recorded request
  (or a present `report.json` with `decision=NEEDS_HUMAN`) is a **recorded
  measurement**. An absent approval artifact is **`UNAVAILABLE`** — the run's approval
  state is unknown, not a recorded zero — and a gate is never inferred from
  `BLOCKED`/`NEEDS_HUMAN` text that is not backed by the recorded artifact.

### Dimension 6 — Execution latency — AVAILABLE

- **Source field.** `metrics.json` `total_ms`, `stages_ms.*` (`plan`, `implement`,
  `validation`, `review`, `fix`), and `validation_ms.*` (`build`, `lint`, `test`),
  backed by `internal/perf/perf.go` `Task.TotalMS`, `Task.StagesMS`, `Task.ValidationMS`,
  and the `Recorder.Measure` reader (`internal/cli/run.go` `timedValidation`).
  Anchors: `.agent-sdlc/runs/<TASK-ID>/metrics.json` `total_ms`, `stages_ms`,
  `validation_ms`; `internal/perf/perf.go` `Task.StagesMS` / `Task.TotalMS` /
  `Task.ValidationMS`.
- **Classification (carried from CLOSE-008).** **A — AVAILABLE.** CLOSE-008 §7 row 13
  (Validation) is AVAILABLE and §1 records `stages_ms` / `validation_ms` / `total_ms`
  as the persisted base contract; EV-004 §1 row 4 anchors the same `internal/perf`
  latency signal (`Recorder.Measure`, `Task.StagesMS`/`Task.TotalMS`).
- **Denominator.** **146 runs** — the runs carrying `metrics.json` with the timing
  fields (RM-001 §2 and §5.2: 146 carrying / 18 missing / 89.0% of 164; the total-,
  per-stage-, validation-time, and validation/review run sub-signals all use the
  146-run denominator, RM-001 §5.2).
- **Coverage rule.** Coverage is `146 / 164` = **89.0%** per timing sub-signal. A run
  without `metrics.json` is `UNAVAILABLE` for latency and excluded from the
  denominator; latency is diagnostic only and never read back to drive a decision
  (`internal/perf` package doc).
- **Zero vs `UNAVAILABLE`.** A present `metrics.json` with a recorded `total_ms` /
  stage millisecond value is a **recorded measurement** (a stage that did not run is
  simply absent from `stages_ms`, per `internal/perf` — a stage is recorded only when
  its work ran). An absent `metrics.json` or an absent stage key is **`UNAVAILABLE`**
  for that sub-signal, not a recorded `0` ms. Exact LLM time is never inferred from
  `stages_ms[plan|implement|fix]`, which include harness and tool overhead
  (CLOSE-008 §10 rule 1).

### Dimension 7 — Token usage — MISSING / UNAVAILABLE

- **Source field.** **No recorded source.** There is no token field anywhere in
  `internal/perf/perf.go`, `metrics.json`, `report.json`, or any run artifact. The
  deliberate absence is anchored at `internal/context/context.go:10-11` (limits are
  sizes and files, never tokens — reliable provider-independent token accounting does
  not exist), `internal/prompt/compile.go:15-16` (limits in bytes, never tokens),
  `internal/orchestration/budget.go:23-25` (no token/cost field), and
  `internal/orchadopt/orchadopt.go` (token counts have no reliable
  provider-independent accounting); `docs/reference/PERFORMANCE.md` lists input/output
  tokens as not recorded.
- **Classification (carried from CLOSE-008).** **M — MISSING / UNAVAILABLE.**
  CLOSE-008 §7 row 10 (Input/output tokens) is MISSING ("True token counts are
  UNAVAILABLE and never inferred from text size"), and CLOSE-008 §6/§9/§10 state
  tokens are not recorded and are never inferred. EV-004 §4 classifies the token row
  **MISSING / UNAVAILABLE — informational only, never policy**.
- **Denominator.** **0 — `UNAVAILABLE`** (RM-001 §5.2: token usage reads nothing,
  denominator 0, coverage 0.0%). No run is eligible because no artifact records it.
- **Coverage rule.** Coverage is `0 / 164` = **0.0%**. Every run is `UNAVAILABLE` for
  token usage. The denominator is never widened by inference and never inflated from
  any proxy.
- **Zero vs `UNAVAILABLE`.** Token usage is **`UNAVAILABLE`** for every run; it is
  not a recorded zero and not a recorded value. Token counts are **never** inferred
  from text size, byte size, or diff size (CLOSE-008 §10 rule 3; EV-004 §4). If a
  provider ever reports a count, it is recorded **informationally at most** and, until
  such a count is persisted, the value stays `UNAVAILABLE`.

---

## 2. Dimension summary

| # | Dimension | Classification | Source field | Denominator | Coverage of 164 |
| --- | --- | --- | --- | ---: | ---: |
| 1 | Agent success / failure rate | AVAILABLE | `report.json` `decision`; `trace.json` `Termination` | 132 (report.json); 39 (trace.json) | 80.5%; 23.8% |
| 2 | Routing / finding-severity distribution | PARTIAL | `model-selection.json` (class/provider/model); `review.json` severities | 50 (routing); 164 (review) | 30.5%; 100.0% |
| 3 | Fix-loop convergence | AVAILABLE | `metrics.json` `counts.fix_cycles` / `counts.plan_repairs` | 146 | 89.0% |
| 4 | Retry efficiency | PARTIAL | `attempt.txt`; `continuations.txt`; `classification.json` | 60; 15; 31 | 36.6%; 9.1%; 18.9% |
| 5 | Human-approval frequency | AVAILABLE | `report.json` `decision=NEEDS_HUMAN`; `approval.json` | 7 (approval); 132 (report) | 4.3%; 80.5% |
| 6 | Execution latency | AVAILABLE | `metrics.json` `total_ms` / `stages_ms` / `validation_ms` | 146 | 89.0% |
| 7 | Token usage | MISSING / UNAVAILABLE | **no recorded source** | 0 — UNAVAILABLE | 0.0% |

No metric is added and none the assessment named is omitted: the table has exactly
the seven plan dimensions, in the plan's order, with the plan's status vocabulary
carried from CLOSE-008 §7.

Classification anchor map (carried from CLOSE-008 §7, `internal/perf` and the run
readers):

| Dimension | CLOSE-008 §7 anchor | Reader anchor |
| --- | --- | --- |
| 1 Agent success / failure rate | row 3 (task completion/failure) | `internal/runtrace/trace.go` `Termination`; `internal/cli/trace.go` `traceTermination`; `internal/run/run.go` report payload |
| 2 Routing / finding-severity | rows 15, 16 (selected model / model class) | `internal/run/routing_artifact.go` `RoutingArtifact`; `internal/cli/run.go` routing evidence; `review.json` |
| 3 Fix-loop convergence | row 14 (retry/escalation lifecycle counts) | `internal/perf/perf.go` `Counts.FixCycles` / `Counts.PlanRepairs` |
| 4 Retry efficiency | row 14 (attempt/escalation evidence) | `internal/run/attempt.go`; `classification.json`; `RoutingSourceEscalation` |
| 5 Human-approval frequency | approval provenance | `internal/run/approval.go` `Approval` / `ApprovalHistory`; `report.json` `decision` |
| 6 Execution latency | row 13 (validation) and §1 timing contract | `internal/perf/perf.go` `Task.StagesMS` / `Task.TotalMS` / `Task.ValidationMS` |
| 7 Token usage | row 10 (input/output tokens) | **none** |

---

## 3. UNAVAILABLE vs recorded zero (the exact handling)

Applied identically to every dimension above (RM-001 §5.1; CLOSE-008 §10 rules 3–5):

- **`UNAVAILABLE`** — the run does not carry the artifact, or the artifact does not
  expose the field. The metric is **unknown** for that run: it is **never** summed,
  averaged, or encoded as `0`, and the run is **excluded from the metric
  denominator** rather than folded in as a zero. An unreported value is **never**
  inferred from text, byte, or diff size.
- **Recorded zero** — the artifact is **present** and the field **explicitly** holds
  `0` (for example `counts.fix_cycles: 0`). This is a real measurement; it **does**
  count toward the metric denominator and numerator.

Rule: a metric value for a run is either a recorded number or `UNAVAILABLE`; the two
are never conflated. No dimension in this register widens its denominator by
treating an absent artifact or absent field as a zero-valued measurement.

---

## 4. Binding token rule

**Token usage is classified MISSING / `UNAVAILABLE` with no recorded source**
(dimension 7; CLOSE-008 §6/§7 row 10/§10; EV-004 §4) and is subject to this binding
rule:

1. **`UNAVAILABLE` everywhere.** Token usage is recorded as `UNAVAILABLE` wherever no
   reliable provider-independent count exists — which, at present, is every run. The
   denominator is `0`.
2. **Excluded from every gate, budget, and routing use.** Token usage is excluded
   from every gate, budget, and routing use. It is **never** an input to a gate, a
   budget, a routing decision, or an acceptance decision. Using a token count as
   budget, routing, or acceptance authority is a fail-closed stop condition
   (`docs/plans/PLAN-Run-Metrics-And-Observability.md`, EV-004 §4).
3. **Informational only.** If a provider ever reports a count, it is recorded
   informationally at most, never as policy. No SOP-owned token counter is proposed
   by this register, and no production token field is introduced.
4. **Never inferred.** Token counts are never inferred from text size, byte size, or
   diff size (CLOSE-008 §10 rule 3; EV-004 §4).

---

## 5. No production change

This register is a read-only report. It contains no implementation, no schema field,
no artifact, no log, and no store. Specifically:

- No source file under `internal/` is created, modified, or deleted.
- No configuration or policy file is modified, and no `.agent-sdlc` state is touched.
- The register introduces no new metric and omits none the assessment named; it only
  records the source field, classification, denominator, coverage rule, and
  zero-vs-`UNAVAILABLE` handling of the seven dimensions the plan already fixed.
- The only artifact written by this task is
  `docs/reports/run-metrics/RM-002-metric-register.md`. Pre-existing user-owned
  working-tree changes are preserved unchanged.

Deterministic presence/nonemptiness proof for this artifact:

```bash
test -s docs/reports/run-metrics/RM-002-metric-register.md && echo PRESENT
```

Expected/observed result: the register file exists and is nonempty after the write.
