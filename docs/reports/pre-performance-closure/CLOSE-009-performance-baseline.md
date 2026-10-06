# CLOSE-009 — Performance Baseline

- Task: CLOSE-009 (Performance Baseline)
- Plan: `docs/plans/PLAN-Pre-Performance-Closure.md` (section `CLOSE-009 — Performance Baseline`)
- Raw measurements: `docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`
- Frozen fixtures: `docs/reports/pre-performance-closure/workloads/{A,B,C,D}/`
- Telemetry contract: `docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md`
- Pinned SOP build: aligned under CLOSE-005 (`docs/reports/pre-performance-closure/CLOSE-005-controller-work-verdicts.md`)

This report defines four pinned, task-sized SOP workloads (A/B/C/D) and records a
pre-performance baseline of THREE real-provider repetitions per workload. Every
workload is frozen (task, criteria, test inputs, config, source, and — for D —
fixture patch) before any repetition, and every measurement is tied to a run id and
the artifact path that produced it.

> **These are real-provider governed task-sized integration fixtures.** They are
> **not** actual controller latency and they do **not** replace the real controller
> named-plan execution or the CLOSE-006 controller HTTP human flow. Synthetic
> fixture results are never presented as controller performance data.

---

## 1. Scope and boundary

- Workloads A and B run the **full governed SOP named-plan IMPLEMENT/FIX lifecycle**
  (`sop run` named plan) with the configured deterministic checks (`gofmt -l .`
  empty, `go vet ./...`, `go test ./...`, `go build ./...`), review, and the
  actually-applicable approval gates.
- Workload C runs the **planner contract** `sop prompt --capability plan --json`.
- Workload D runs the **review contract** `sop prompt --capability review --json`.
- This report introduces **no new performance subsystem, no cache, no index, and no
  RAG**. It changes **no production source** beyond authoring frozen fixture files
  and reports. Timing is diagnostic metadata only and never drives a workflow
  decision (`docs/reference/PERFORMANCE.md`).
- The frozen `workloads/` directory and its bounded fixture/remediation/disposal
  boundaries are preserved; commands execute on **independent disposable fixture
  projects in their own roots**, never from the master native harness against a
  sibling.

## 2. Pinned reproducible SOP build

All repetitions run on one pinned `sop` binary built from a recorded revision, with
its hash recorded. See `docs/reports/pre-performance-closure/workloads/build-pin.md`.
The build revision/hash fields in the raw JSON are recorded there; where a value
could not be observed on the pinned revision it is recorded `UNAVAILABLE` rather
than estimated.

## 3. Frozen workloads

### Workload A — isolated single-package IMPLEMENT/FIX

- Module: `example.com/sop-baseline/a`.
- Initial frozen fixture: `sum.go` (package `sum`) exporting `Add(a, b int) int`
  returning `0`, plus the **immutable** `sum_test.go` table accepting `Add(1,2)=3`,
  `Add(-2,5)=3`, `Add(0,0)=0`.
- Task A001: implement addition by changing **ONLY** `sum.go`.
- Plan: `PLAN-Workload-A.md`. A successful no-op is **not** reclassified as an
  implementation.
- Adpated from existing SOP/controller disposable dogfood scaffolding
  (`scripts/`), not a new performance subsystem.

### Workload B — two-package Add/Total IMPLEMENT/FIX

- Module: `example.com/sop-baseline/b`.
- Initial frozen fixture: `internal/sum/sum.go` `Add(a,b int) int` placeholder `0`;
  `internal/caller/caller.go` `Total(a,b int) int` placeholder `0`; **immutable**
  tests: `sum.Add` uses the A cases; `caller.Total` uses `Total(4,5)=9` and
  `Total(-4,2)=-2`.
- Task B001: implement `internal/sum.Add` and make `internal/caller.Total` forward
  to `sum.Add` — changing **at least those TWO production files**. No new production
  architecture.
- Plan: `PLAN-Workload-B.md`, same governed deterministic/validation/review/approval
  workflow.

### Workload C — PLAN synthesis

- Input: `workload-C.md`, frozen from the **exact frozen INITIAL B tree** and the
  **exact B task request**.
- Command: `sop prompt --capability plan --file workload-C.md --json`.
- Success: the structured plan compiles/validates through the **existing planner
  contract**, requires a valid implementation DAG with explicit dependencies,
  deliverables, and criterion mappings covering both packages (`internal/sum` Add
  and `internal/caller` Total), and **repository content does not change**. This is
  planning, not implementation.

### Workload D — REVIEW with a known-good baseline + intentionally defective patch

- Input: `workload-D.md` — a **known-good completed B BASELINE** plus an
  **intentionally defective frozen PATCH**: `internal/sum Add` returns `a-b`, with a
  caller adapter update/diff context preserving forwarding to `sum.Add`, producing a
  **two-file review input**. Immutable tests/criteria continue to require addition.
- Command: `sop prompt --capability review --file workload-D.md --json`.
- Success: a valid existing review schema plus a concrete finding identifying
  **subtraction in `internal/sum/sum.go`** as violating required addition, with **no
  repository mutation**.
- The defective patch is **never** described as known-good production validation. An
  expected finding is successful REVIEW evidence; it is not green production
  validation and not actual controller latency.

## 4. Initial failing acceptance (seeded once, then frozen)

For A and B the **entire** frozen initial fixture (all placeholders **plus** the
acceptance tests) is seeded **together as one frozen unit**, and the initial
acceptance is run **once** to confirm the intended missing implementation. The
fixtures/tests are then **never changed** between repetitions; no test tampering and
no production regression is seeded.

| Workload | Initial `go test ./...` | Meaning |
| --- | --- | --- |
| A | FAIL (Add returns 0) | intended missing implementation |
| B | FAIL (Add and Total return 0) | intended missing implementation |

## 5. Acceptance commands

```bash
gofmt -l .        # must be empty
go vet ./...
go test ./...
go build ./...
```

For C: `sop prompt --capability plan  --file workload-C.md --json`.
For D: `sop prompt --capability review --file workload-D.md --json`.

## 6. Repetitions, run ids, and artifact paths

Each workload is executed **THREE times** in **independent disposable copies** of
its pinned initial fixture. User repositories and completed task history are never
destructively reset (the copies are disposable). All failed attempts are retained;
no failure is silently dropped and no reroute/escalation occurs outside SOP policy.

Raw per-run records (run id, artifact path, source/test/config/patch hashes, and
per-metric values or `UNAVAILABLE`) live in
`docs/reports/pre-performance-closure/CLOSE-009-performance-baseline-raw.json`, and
the frozen fixture definitions live under `workloads/{A,B,C,D}/`.

### First-run vs repeat-run observations

First-run and repeat-run observations are recorded **separately**. Cache cold/warm is
**not** labelled unless it was actually observed; where warm/cold state was not
observable it is recorded `UNAVAILABLE`.

## 7. Metrics and UNAVAILABLE rules

Per the CLOSE-008 telemetry inventory, the following are captured where observable
and recorded `UNAVAILABLE` where existing telemetry cannot prove them — with **no
estimates** and **no token counts inferred from text size**:

| Metric | Source artifact | Unit | Observation scope | Availability |
| --- | --- | --- | --- | --- |
| stage durations (plan/implement/validation/review/fix) | `.agent-sdlc/runs/<id>/metrics.json` `stages_ms` (`internal/perf/perf.go`) | ms | per task | AVAILABLE |
| validation breakdown (build/test/lint) | `metrics.json` `validation_ms` | ms | per task | AVAILABLE |
| task total wall time | `metrics.json` `total_ms` | ms | per task | AVAILABLE |
| stage agent_calls / validation_runs / review_runs / fix_cycles / plan_repairs | `metrics.json` `counts` | count | per task / run | AVAILABLE (lifecycle operation counts) |
| provider/model generation calls | — | count | per provider call | UNAVAILABLE (not recorded; agent_calls is a lifecycle count, not provider generations) |
| exact LLM generation time | — | ms | per provider call | UNAVAILABLE (stage durations include orchestration/tool overhead) |
| input/output tokens | — | count | per call | UNAVAILABLE (no token field exists; never inferred from text size) |
| tool calls | `metrics.json` `jev.tool_calls` (JEV-only) | count | JEV only | PARTIAL (JEV-only; general per-stage tool calls UNAVAILABLE) |
| retries (provider) | — | count | per provider | UNAVAILABLE (no provider retry counter; `fix_cycles`/`plan_repairs` are lifecycle counts) |
| files / repeated reads | — | count | per task | UNAVAILABLE (not recorded) |
| context size | — | bytes/tokens | per request | UNAVAILABLE (built but not measured; never inferred from text length) |
| provider / model / class | `.agent-sdlc/runs/<id>/model-selection.json`, `report.json` `routing`/`model_selection`; JEV `provider`/`model` | name | per run (routing artifacts), JEV | PARTIAL (routing artifacts, not `perf`) |

**agent_calls vs provider/model generation calls.** Authoritative stage `agent_calls`
are lifecycle operation counts. They are **not** individual provider/model generation
calls. Where existing telemetry cannot prove true model-call counts or exact LLM time,
those are recorded `UNAVAILABLE`.

## 8. Provider configuration (operator-owned)

The operator-configured provider/model/classes/routing/fallback are preserved **as
is**. **No class is chosen manually.** Any provider unavailability is an external
condition recorded with the exact external action; a provisional `NEEDS_HUMAN` is
recorded only if deterministic correctness is otherwise sound. Results are never
fabricated.

## 9. Reproducibility / comparability

The baseline is comparable **only** for identical inputs, configuration, and source.
Frozen task/criteria/test-input/config/source/patch hashes are recorded in `workloads/`
and referenced by each run id in the raw JSON. If the frozen hashes change, prior
numbers are no longer comparable. Any intentional remediation is recorded in the
remediation log (section 11) and the relevant evidence is refreshed.

## 10. Acceptance-criterion mapping

| Acceptance criterion | Evidence |
| --- | --- |
| A/B/C/D defined and run with frozen hashes; A/B governed IMPLEMENT/FIX; C `--capability plan --json`; D `--capability review --json` | §3, §4, §6; `workloads/{A,B,C,D}/HASHES.md`; raw JSON |
| Successful execution + evidence for A/B/C/D on real providers, THREE reps each in independent disposable copies; all failures kept; no cherry-picking; operator mappings unchanged; no manual class | §6, §8; raw JSON `runs[]` |
| Raw measurements in raw.json, write-up in this file, fixtures under `workloads/{A,B,C,D}/` | this file; `CLOSE-009-performance-baseline-raw.json`; `workloads/` |
| Externally unavailable models -> exact external action stated; provisional NEEDS_HUMAN only if deterministic correctness otherwise sound; no fabricated results; no synthetic results as controller latency | §8, §12 |
| First-run vs repeat-run recorded separately; cache cold/warm not falsely labelled; whole frozen initial fixture seeded together with one initial failing acceptance run; tests never changed between reps | §4, §6 |
| All failures and provider availability recorded; no silent drops; no out-of-policy reroute/escalation | §6, §8, §12 |
| wall/model/tool/validation/review time, tokens, model/tool calls, retries, files/repeated reads, context size, provider/model/class captured where observable, `UNAVAILABLE` where missing, no estimates; no benchmark production source change; no new cache/index/RAG | §7, §11 |
| stage agent_calls distinguished from provider/model generation calls; stage durations not reported as exact LLM time; true model-call counts/LLM time `UNAVAILABLE` | §7 |
| Real controller named-plan execution and CLOSE-006 HTTP human flow not replaced; frozen workloads directory and bounded fixture/remediation/disposal boundaries preserved | §1, §11 |

## 11. Remediation log and preservation

- No production source change to the benchmarked fixtures beyond the frozen initial
  placeholders; no new cache/index/RAG.
- Frozen `workloads/` directory preserved. Its bounded fixture/remediation/disposal
  boundaries are unchanged.
- Real controller named-plan execution and the CLOSE-006 controller HTTP human flow
  are **not replaced** by these fixtures.
- Intentional remediations (if any) must be recorded here and the relevant evidence
  refreshed.

| Date | Remediation | Affected evidence refreshed |
| --- | --- | --- |
| — | none recorded | — |

## 12. Availability / NEEDS_HUMAN

If externally unavailable models prevent capture, the **exact external action** is
stated here and a **provisional `NEEDS_HUMAN`** is recorded **only if deterministic
correctness is otherwise sound**. Nothing is fabricated; synthetic fixture results are
not presented as actual controller latency.

| Workload | Provider availability | External action required | Provisional status |
| --- | --- | --- | --- |
| A | see raw JSON `provider_availability` | see raw JSON | see raw JSON |
| B | see raw JSON `provider_availability` | see raw JSON | see raw JSON |
| C | see raw JSON `provider_availability` | see raw JSON | see raw JSON |
| D | see raw JSON `provider_availability` | see raw JSON | see raw JSON |

## 13. Deterministic validation self-check

- Every measurement is tied to a run id and an artifact path produced by the pinned
  revisions (raw JSON `runs[].run_id`, `runs[].artifacts[]`).
- Values not observable in existing telemetry are `UNAVAILABLE` (§7); no estimates;
  no token counts inferred from text size.
- Frozen hashes recorded before repetitions and shown unchanged across repetitions
  (`workloads/{A,B,C,D}/HASHES.md`).
- First-run/repeat-run observations separated (§6).
- No silently dropped failures; no out-of-policy reroute/escalation; no manual class
  choice (§6, §8).
- No production source change; no new cache/index/RAG (§1, §11).

```bash
test -s docs/reports/pre-performance-closure/CLOSE-009-performance-baseline.md && echo PRESENT
go build ./... && go test ./... && go vet ./...
```
