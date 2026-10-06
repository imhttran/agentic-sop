# PERFORMANCE-BASELINE — Pre-Performance Baseline (CLOSE-010)

- Task: CLOSE-010 (Report `docs/reports/PERFORMANCE-BASELINE.md`)
- Plan: `docs/plans/PLAN-Pre-Performance-Closure.md`
- Raw measurements (authoritative input): `docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`
- CLOSE-009 write-up: `docs/reports/pre-performance-closure/CLOSE-009-performance-baseline.md`
- Telemetry contract: `docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md`
- Frozen fixtures: `docs/reports/pre-performance-closure/workloads/{A,B,C,D}/`
- Pinned build reference: `docs/reports/pre-performance-closure/workloads/build-pin.md`

This document is the **pre-performance baseline**. It is positioned **before any
future architecture change**: any future performance work is compared against the
numbers published here, and only numbers that trace to a CLOSE-009 raw measurement
and run id are publishable. Nothing in this report is inferred, estimated, or
derived from narration.

> **Retained CLOSE-009 boundary language.** These are real-provider governed
task-sized integration fixtures. They are **not** actual controller latency and
they do **not** replace the real controller named-plan execution or the CLOSE-006
controller HTTP human flow. Synthetic fixture results are never presented as
controller performance data.

---

## 1. Scope and provenance conventions

### 1.1 Provenance convention (every published number)

Every published number in this report MUST carry, and is traceable through:

- **source artifact** — the path (from the CLOSE-009 `metric_contract`) that produced the value;
- **unit** — the recorded unit (e.g. `ms`, `count`, `name`, `bytes_or_tokens`);
- **observation scope** — the recorded scope (e.g. `per task`, `per run`, `per provider call`, `lifecycle operation count`).

A number is only publishable when it traces to a **CLOSE-009 raw measurement and a
run id**. When the CLOSE-009 raw artifact records a metric as `UNAVAILABLE`, or when
no run record exists for it, the value cell reads `UNAVAILABLE` with the recorded
reason. **No inferred metric may appear.**

### 1.2 Boundary

- Workloads A and B exercise the full governed SOP named-plan IMPLEMENT/FIX lifecycle
  (`sop run` named plan) with deterministic checks (`gofmt -l .` empty, `go vet ./...`,
  `go test ./...`, `go build ./...`), review, and applicable approval gates.
- Workload C exercises the planner contract `sop prompt --capability plan --file workload-C.md --json`.
- Workload D exercises the review contract `sop prompt --capability review --file workload-D.md --json`.
- No new performance subsystem, cache, index, or RAG is introduced. Timing is
  diagnostic metadata only and never drives a workflow decision.

## 2. Baseline metadata

### 2.1 Pinned build (revision and binary hash)

From `docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`
`pinned_build`:

| Field | Value | Unit | Observation scope | Source artifact |
| --- | --- | --- | --- | --- |
| Build reference | `docs/reports/pre-performance-closure/workloads/build-pin.md` | path | per baseline | raw JSON `pinned_build.reference` |
| Revision | `UNAVAILABLE` | — | per baseline | raw JSON `pinned_build.revision` |
| Revision reason | "The reproducible SOP build revision aligned under CLOSE-005 was not resolvable from repository inspection on this run." | — | per baseline | raw JSON `pinned_build.revision_reason` |
| Binary hash | `UNAVAILABLE` | — | per baseline | raw JSON `pinned_build.binary_hash` |
| Binary hash reason | "No pinned binary hash was observed for the measurement build on this run." | — | per baseline | raw JSON `pinned_build.hash_reason` |

`workloads/build-pin.md` was read directly and independently records the same
`UNAVAILABLE` revision and binary hash (Status: UNAVAILABLE). No pinned build revision
or binary hash exists to publish.

### 2.2 Repository SHAs / dirty state

The raw CLOSE-009 artifact contains **no** repository SHA or git dirty-state field, so
there is **no run-time measurement SHA** for this baseline. The values below are a
**publication-time observation** made by CLOSE-010 and are **not** CLOSE-009 run-time
evidence.

| Field | Value | Unit | Observation scope | Source |
| --- | --- | --- | --- | --- |
| HEAD SHA | `8224505b4b69236ee41fdb6f5f5a9deddf65a3d8` | git SHA | publication-time observation (not run-time measurement) | `git rev-parse HEAD` observed by CLOSE-010 |
| Dirty state | see §6 preservation note — working tree contains user-owned unrelated changes not part of this task | state | publication-time observation (not run-time measurement) | `git status`, observed by CLOSE-010 |
| Run-time repo SHA / dirty state | `UNAVAILABLE` | — | run-time measurement | no such field in raw JSON or CLOSE-009 write-up |

### 2.3 Configuration

| Field | Value | Unit | Observation scope | Source artifact |
| --- | --- | --- | --- | --- |
| Operator-configured provider mapping | "Preserved as-is from operator configuration (.env / .env.example / integrations/)." | — | operator configuration | raw JSON `providers.mapping` |
| Manual class chosen | `false` | boolean | operator configuration | raw JSON `providers.manual_class_chosen` |
| Reroute/escalation outside policy | `false` | boolean | operator configuration | raw JSON `providers.reroute_or_escalation_outside_policy` |
| Provider availability observation | `UNAVAILABLE` | — | operator configuration | raw JSON `providers.availability_observation` |
| Deterministic-check config (Workload A) | `A/config.yaml` | path | per workload fixture | raw JSON `workloads.A.frozen_hashes.config` -> `workloads/A/HASHES.md` |

### 2.4 Binary

Binary hash and revision are `UNAVAILABLE` (§2.1). No pinned `sop` binary exists to
publish for the measurement build.

### 2.5 Provider / models / routing

From raw JSON `providers` and `metric_contract`:

| Field | Value | Unit | Observation scope | Source artifact |
| --- | --- | --- | --- | --- |
| Operator configured | `true` | boolean | operator configuration | raw JSON `providers.operator_configured` |
| Concrete provider/model/class name | `UNAVAILABLE` | name | per run | not recorded in raw JSON `providers` (no concrete value) |
| Provider retries (provider-level) | `UNAVAILABLE` | count | per provider | raw JSON `metric_contract.provider_retries` (reason: "No provider retry counter; fix_cycles/plan_repairs are lifecycle counts.") |
| Provider/model/class (per run) | `PARTIAL` | name | per run (routing artifacts, not perf) | raw JSON `metric_contract.provider_model_class` -> `.agent-sdlc/runs/<id>/model-selection.json` |

The metric `provider_model_class` is recorded `PARTIAL` (routing artifacts, not
performance). No concrete provider/model/class name is publishable because no run
record exists (see §2.9).

### 2.6 Workload input hashes (frozen)

Quoted only where actually present in the repository at publication time.

| Workload | Artifact | Path | Unit | Observation scope | Value / status |
| --- | --- | --- | --- | --- | --- |
| A | source, test_input, module, config, task, criteria | `docs/reports/pre-performance-closure/workloads/A/HASHES.md` | SHA-256 | per frozen fixture | `UNAVAILABLE` (reason recorded in `A/HASHES.md`: "hash tool not available in this environment") |
| A | fixture_patch | `n/a` | — | per frozen fixture | `n/a` (not applicable for A) |
| B | source, criteria, test_input, config, fixture_patch | `docs/reports/pre-performance-closure/workloads/B/HASHES.md` | SHA-256 | per frozen fixture | `UNAVAILABLE` (the `B/` fixture directory and hash values were not observable from the repository listing at publication time) |
| C | input, b_source_refs | `docs/reports/pre-performance-closure/workloads/C/HASHES.md` | SHA-256 | per frozen fixture | `UNAVAILABLE` (the `C/` fixture directory and hash values were not observable from the repository listing at publication time) |
| D | input, fixture_patch | `docs/reports/pre-performance-closure/workloads/D/HASHES.md` | SHA-256 | per frozen fixture | `UNAVAILABLE` (the `D/` fixture directory and hash values were not observable from the repository listing at publication time) |
| Build pin | Revision, Binary hash | `docs/reports/pre-performance-closure/workloads/build-pin.md` | — | per baseline | `UNAVAILABLE` (read directly; Status: UNAVAILABLE) |

No frozen hash value is re-derived, estimated, or copied from CLOSE-009 prose.

### 2.7 Platform

| Field | Value | Unit | Observation scope | Source |
| --- | --- | --- | --- | --- |
| Platform | `UNAVAILABLE` | — | per run / per baseline | no platform field is recorded in the raw CLOSE-009 artifact; platform not observed |

Platform is `UNAVAILABLE`; it is not guessed.

### 2.8 Repetitions

| Workload | Repetitions | Unit | Observation scope | Source artifact |
| --- | --- | --- | --- | --- |
| A | 3 | count | per workload definition (not observed runs) | raw JSON `workloads.A.repetitions` |
| B | 3 | count | per workload definition (not observed runs) | raw JSON `workloads.B.repetitions` |
| C | 3 | count | per workload definition (not observed runs) | raw JSON `workloads.C.repetitions` |
| D | 3 | count | per workload definition (not observed runs) | raw JSON `workloads.D.repetitions` |

The value `3` is the number of repetitions **defined** per workload in the raw JSON
workload definitions. It is **not** a count of observed runs (each workload's `runs`
array is empty — see §2.9).

### 2.9 Run ids and artifact paths

| Workload | Run ids | Artifact paths | capture_status | Capture status reason | Source artifact |
| --- | --- | --- | --- | --- | --- |
| A | none (`runs: []`) | none recorded | `UNAVAILABLE` | "Real-provider repetitions on the pinned build could not be observed from repository inspection this run; measurement values are recorded UNAVAILABLE rather than estimated." | raw JSON `workloads.A.runs`, `capture_status` |
| B | none (`runs: []`) | none recorded | `UNAVAILABLE` | same | raw JSON `workloads.B.runs`, `capture_status` |
| C | none (`runs: []`) | none recorded | `UNAVAILABLE` | same | raw JSON `workloads.C.runs`, `capture_status` |
| D | none (`runs: []`) | none recorded | `UNAVAILABLE` | same | raw JSON `workloads.D.runs`, `capture_status` |

**No run records exist for workloads A/B/C/D at publication time.** The raw JSON sets
`workloads.{A,B,C,D}.runs = []` and `capture_status = "UNAVAILABLE"`. Consequently no
per-run measurement values, run ids, or per-run artifact paths are publishable. No run
id table is fabricated.

Contract-declared source-artifact paths of the form `.agent-sdlc/runs/<id>/metrics.json`
and `.agent-sdlc/runs/<id>/model-selection.json` appear in `metric_contract` as
**annotations only**. No concrete run id exists for this baseline, so those artifacts
are **uninstantiated** and are never read or cited as existing sources.

## 3. Raw measurements (per metric contract)

The raw measurements are the CLOSE-009 raw JSON `metric_contract`. Values are
publishable only where a run record exists; no run records exist (§2.9), so every
value cell is `UNAVAILABLE`, with the contract's own availability/reason retained.

| Metric | Source artifact | Unit | Observation scope | Availability | Published value |
| --- | --- | --- | --- | --- | --- |
| stage_durations | `.agent-sdlc/runs/<id>/metrics.json#stages_ms` | ms | per task | AVAILABLE | `UNAVAILABLE` (no run record) |
| validation_breakdown | `.agent-sdlc/runs/<id>/metrics.json#validation_ms` | ms | per task | AVAILABLE | `UNAVAILABLE` (no run record) |
| task_total_wall_time | `.agent-sdlc/runs/<id>/metrics.json#total_ms` | ms | per task | AVAILABLE | `UNAVAILABLE` (no run record) |
| stage_agent_calls | `.agent-sdlc/runs/<id>/metrics.json#counts.agent_calls` | count | lifecycle operation count (NOT individual provider/model generation calls) | AVAILABLE | `UNAVAILABLE` (no run record) |
| validation_runs | `.agent-sdlc/runs/<id>/metrics.json#counts.validation_runs` | count | per task | AVAILABLE | `UNAVAILABLE` (no run record) |
| review_runs | `.agent-sdlc/runs/<id>/metrics.json#counts.review_runs` | count | per task | AVAILABLE | `UNAVAILABLE` (no run record) |
| fix_cycles | `.agent-sdlc/runs/<id>/metrics.json#counts.fix_cycles` | count | per task (lifecycle repairs, not provider retries) | AVAILABLE | `UNAVAILABLE` (no run record) |
| plan_repairs | `.agent-sdlc/runs/<id>/metrics.json#counts.plan_repairs` | count | per task (lifecycle repairs, not provider retries) | AVAILABLE | `UNAVAILABLE` (no run record) |
| provider_model_generation_calls | — | count | per provider call | UNAVAILABLE | `UNAVAILABLE` (reason: "Not recorded; agent_calls is a lifecycle count, not provider generation calls.") |
| exact_llm_generation_time | — | ms | per provider call | UNAVAILABLE | `UNAVAILABLE` (reason: "Stage durations include orchestration/tool overhead and are not exact LLM time.") |
| input_output_tokens | — | count | per call | UNAVAILABLE | `UNAVAILABLE` (reason: "No token field exists in telemetry; never inferred from text size.") |
| tool_calls | `.agent-sdlc/runs/<id>/metrics.json#jev.tool_calls` | count | JEV only | PARTIAL | `UNAVAILABLE` (JEV-only; no general per-stage tool-call counter; no run record) |
| provider_retries | — | count | per provider | UNAVAILABLE | `UNAVAILABLE` (reason: "No provider retry counter; fix_cycles/plan_repairs are lifecycle counts.") |
| files_repeated_reads | — | count | per task | UNAVAILABLE | `UNAVAILABLE` (reason: "Not recorded.") |
| context_size | — | bytes_or_tokens | per request | UNAVAILABLE | `UNAVAILABLE` (reason: "Context is built but not measured; never inferred from text length.") |
| provider_model_class | `.agent-sdlc/runs/<id>/model-selection.json` | name | per run (routing artifacts, not perf) | PARTIAL | `UNAVAILABLE` (no run record) |

### 3.1 Median / range

Median and range are computable **only** where actual per-repetition values exist. No
per-repetition values exist (all `runs` arrays are empty), so:

| Statistic | Value | Computation basis (run ids) | Source artifact |
| --- | --- | --- | --- |
| Median (all metrics) | `UNAVAILABLE` | none — no run ids exist (`runs: []`) | raw JSON `workloads.*.runs` |
| Range (all metrics) | `UNAVAILABLE` | none — no run ids exist (`runs: []`) | raw JSON `workloads.*.runs` |

## 4. Workload comparison table (observed-only cells)

A cell is filled only where a run id and source artifact exist. No run records exist,
so every listed metric reads `UNAVAILABLE`, with its unit and observation scope.

| Metric | Unit | Observation scope | A | B | C | D |
| --- | --- | --- | --- | --- | --- | --- |
| Wall time (`task_total_wall_time`) | ms | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Model time (exact LLM time) | ms | per provider call | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Tool time | ms | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Validation time (`validation_breakdown`) | ms | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Tokens (`input_output_tokens`) | count | per call | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Calls (`stage_agent_calls`, lifecycle count) | count | lifecycle operation count | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Retries (`provider_retries`) | count | per provider | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Largest context (`context_size`) | bytes_or_tokens | per request | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Slowest stages (from `stage_durations`) | ms | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Repeated reads (`files_repeated_reads`) | count | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Largest outputs | — | per task | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |
| Retry-heavy operations | count | lifecycle repairs | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` | `UNAVAILABLE` |

**No run-traceable measurement was available to compare.** No fabricated or
representative numbers are published.

### 4.1 Metric semantics

- **Calls:** `stage_agent_calls` is a **lifecycle operation count**, not a count of
  individual provider/model generation calls. True provider/model generation calls are
  `UNAVAILABLE`.
- **Model time:** stage durations include orchestration and tool overhead; they are
  **not** exact LLM generation time. Exact LLM time is `UNAVAILABLE`.
- **Human-wait time** is never added to or reported as generation time. No human-wait
  time is published in this baseline.
- Largest context, repeated reads, tokens, retries, and call counts are marked
  `UNAVAILABLE` per their recorded availability and are **never inferred** from text
  size or output length.

## 5. Coverage / gaps and UNAVAILABLE register

| Metric | Availability | Gap reason |
| --- | --- | --- |
| stage_durations | AVAILABLE (contract) / UNAVAILABLE (published) | Contract-sourceable, but no run record exists to publish a value. |
| validation_breakdown | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists. |
| task_total_wall_time | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists. |
| stage_agent_calls | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists; lifecycle count, not model calls. |
| validation_runs | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists. |
| review_runs | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists. |
| fix_cycles | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists; lifecycle repairs, not provider retries. |
| plan_repairs | AVAILABLE (contract) / UNAVAILABLE (published) | No run record exists; lifecycle repairs, not provider retries. |
| provider_model_generation_calls | UNAVAILABLE | "Not recorded; agent_calls is a lifecycle count, not provider generation calls." |
| exact_llm_generation_time | UNAVAILABLE | "Stage durations include orchestration/tool overhead and are not exact LLM time." |
| input_output_tokens | UNAVAILABLE | "No token field exists in telemetry; never inferred from text size." |
| tool_calls | PARTIAL | "JEV-only; no general per-stage tool-call counter." |
| provider_retries | UNAVAILABLE | "No provider retry counter; fix_cycles/plan_repairs are lifecycle counts." |
| files_repeated_reads | UNAVAILABLE | "Not recorded." |
| context_size | UNAVAILABLE | "Context is built but not measured; never inferred from text length." |
| provider_model_class | PARTIAL | Routing artifacts, not `perf`. |
| capture_status (all workloads) | UNAVAILABLE | "Real-provider repetitions on the pinned build could not be observed from repository inspection this run; measurement values are recorded UNAVAILABLE rather than estimated." |
| pinned_build.revision | UNAVAILABLE | "The reproducible SOP build revision aligned under CLOSE-005 was not resolvable from repository inspection on this run." |
| pinned_build.binary_hash | UNAVAILABLE | "No pinned binary hash was observed for the measurement build on this run." |
| platform | UNAVAILABLE | No platform field recorded; not observed. |
| repo SHA / dirty state (run-time) | UNAVAILABLE | No run-time SHA/dirty-state field exists in raw JSON or CLOSE-009 write-up. |
| first_run_vs_repeat_run cache cold/warm | UNAVAILABLE | "Warm/cold state was not observed; labelled UNAVAILABLE." |

### 5.1 Availability / provisional NEEDS_HUMAN (as recorded)

| Field | Value | Source artifact |
| --- | --- | --- |
| provisional_needs_human.recorded | `false` | raw JSON `provisional_needs_human.recorded` |
| provisional_needs_human.condition | "Only if externally unavailable models prevent capture AND deterministic correctness is otherwise sound." | raw JSON `provisional_needs_human.condition` |

This mirrors the raw JSON without asserting that capture happened.

## 6. Preservation statement

- The CLOSE-009 raw JSON
  (`docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`) is
  preserved **unmodified** and **not overwritten**. It was read only.
- The CLOSE-009 write-up
  (`docs/reports/pre-performance-closure/CLOSE-009-performance-baseline.md`) is
  preserved unmodified and not overwritten.
- The frozen `workloads/` directory (including `A/HASHES.md` and `build-pin.md`) is
  **unmodified**. CLOSE-010 only read fixture/hash files; it never edits `workloads/`.
- CLOSE-010 created/updated only `docs/reports/PERFORMANCE-BASELINE.md`. Pre-existing
  working-tree changes unrelated to this task are user-owned and were neither reverted
  nor discarded.

## 7. Baseline positioning and comparability

- This report is the **pre-performance baseline**, positioned **before any future
  architecture change**. Future performance work is compared against it.
- The baseline is comparable **only** for identical inputs, configuration, and source.
  If the frozen hashes change, prior numbers are **no longer comparable** (retained
  from CLOSE-009 §9). Any intentional remediation must be recorded and evidence
  refreshed.
- Reproducibility requires a resolvable pinned revision and binary hash; both are
  `UNAVAILABLE` today (§2.1), so a future run must first reconcile them.

## 8. Deterministic validation checklist

Testable assertions a reviewer can re-run against the CLOSE-009 raw JSON
(`docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`):

1. **Traceability.** Every numeric value published in this report traces to a CLOSE-009
   raw measurement and run id. Check: this report publishes **no** numeric value
   other than the frozen `repetitions: 3` (a workload definition, §2.8) and the
   HEAD SHA observation (§2.2, scoped as publication-time, not run-time). Confirm all
   per-metric cells read `UNAVAILABLE`.
2. **No inferred metrics.** Confirm no cell contains a value absent from the raw JSON;
   in particular no tokens, context size, repeated reads, provider retries, or model
   time is published (raw JSON `metric_contract` records them `UNAVAILABLE`).
3. **Human-wait vs generation time.** Confirm no human-wait time is added to or
   reported as generation time, and true model-call counts / LLM time are marked
   `UNAVAILABLE` (§3, §4.1).
4. **Lifecycle vs model calls.** Confirm `stage_agent_calls` is described as a
   lifecycle operation count, not provider/model generation calls (§3, §4.1).
5. **Run records.** Confirm `workloads.{A,B,C,D}.runs == []` and
   `capture_status == "UNAVAILABLE"` in the raw JSON; confirm this report states no
   run-traceable measurement was available (§2.9, §4).
6. **Median/range basis.** Confirm median/range are `UNAVAILABLE` with the stated
   computation basis (no run ids) (§3.1).
7. **Input preservation.** Confirm the raw JSON, CLOSE-009 write-up, and frozen
   `workloads/` are byte-unchanged after CLOSE-010 (no overwrite); CLOSE-010 output is
   only this file (§6).

```bash
test -s docs/reports/PERFORMANCE-BASELINE.md && echo PRESENT
test -s docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json && echo RAW_PRESERVED
go build ./... && go test ./... && go vet ./...
```
