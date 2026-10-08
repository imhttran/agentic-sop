# PLAN — Run Metrics and Observability

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** completed read-only assessment (2026-10-08) of the recorded run evidence
under `.agent-sdlc/runs/` and the `CLOSE-008` telemetry inventory.\
**Plan id:** `plan-run-metrics-and-observability`

## Project

Run Metrics and Observability

## Summary

Make the repository's own execution history observable: a deterministic, **offline**,
**model-free** aggregation over the run artifacts SOP already persists
(`.agent-sdlc/runs/<TASK-ID>/`), reported across seven metric dimensions — agent
success/failure rate, routing and finding-severity distribution, fix-loop
convergence, retry efficiency, human-approval frequency, execution latency, and
token usage. The aggregation reuses the existing readers in `internal/run` and
`internal/perf`; it adds **no new dependency**, **no provider-specific logic**, and
**no production runtime change**. Every value carries an explicit denominator and a
coverage percentage, and an **absent artifact is distinguished from a zero-valued
measurement** — a run that recorded no `metrics.json` is *unmeasured*, not zero.
Token usage is reported `UNAVAILABLE` whenever no provider reports it, and is
**informational only, never policy**.

The work proceeds through five stages: a run-artifact inventory and schema census
(RM-001), the metric register and classification (RM-002), the offline aggregator
implemented test-first as a pure library (RM-003), reproducibility/coverage/neutrality
verification (RM-004), and a decision record on whether to surface the aggregate as
an opt-in command (RM-005). RM-001, RM-002, RM-004, and RM-005 are read-only;
RM-003 adds a new internal library package and its tests and **does not wire any CLI
command**.

## Capabilities

### Go build/test/race toolchain — EXISTS

- **Evidence:** `.github/workflows/ci.yml` runs gofmt, `go vet ./...`, `go build ./...`, `go test ./...`, and `go test -race ./...`; the local Go toolchain builds the repository.
- **Owner:** operator-supplied development environment
- **Location:** Go executable on PATH

### Recorded run artifacts — EXISTS

- **Evidence:** 164 run directories under `.agent-sdlc/runs/` (census 2026-10-08) carrying 132 `report.json`, 146 `metrics.json`, 50 `model-selection.json`, 31 `classification.json`, 7 `approval.json`, 60 `attempt.txt`, 15 `continuations.txt`, and 39 `trace.json`.
- **Owner:** SOP lifecycle (existing)
- **Location:** `.agent-sdlc/runs/<TASK-ID>/`

### Run-artifact readers — EXISTS

- **Evidence:** `internal/run` (`run.go`, `approval.go`, `attempt.go`, `routing_artifact.go`, `external_completion.go`) and `internal/perf/perf.go` already read and type the persisted artifacts.
- **Owner:** agentic-sop
- **Location:** `internal/run`, `internal/perf`

### Provider-reported token counts — MISSING

- **Evidence:** `internal/context/context.go:10-11`, `internal/prompt/compile.go:15-16`, and `internal/orchestration/budget.go:23-25` document that no provider-independent token accounting exists; `CLOSE-008` §6 classifies tokens as not recorded.
- **Owner:** model provider (external)
- **Gap:** no reliable, provider-independent token count is persisted.
- **Resolution:** recorded as `UNAVAILABLE` wherever no count exists; informational only and never an input to budget, routing, or acceptance. No stage requires this capability.

## Goal

Produce, from the already-recorded run artifacts and with no model calls, a
deterministic and reproducible aggregate of the seven metric dimensions, each with
an explicit denominator and coverage percentage, so future decisions about context,
routing, retries, and convergence rest on evidence rather than on inspection of
individual runs.

## Non-Goals

- Implement `sop metrics` or `sop report --all` (deferred to the RM-005 decision; not implemented here).
- Add any new instrumentation to the live execution path, or change any existing artifact schema.
- Count tokens as policy, budget, routing, or acceptance authority.
- Change routing, retry, replan, budget, verification, approval, or commit semantics.
- Aggregate EV, CONV, RSH, or H001–H004 evidence in a way that overwrites or reinterprets it.

## Architecture Invariant

The aggregator is a **reader**, not an authority. It observes recorded artifacts and
emits measurements; it never drives a lifecycle decision, never mutates repository or
SOP state, and never calls a model. Identical inputs must yield byte-identical output.
An absent artifact is `UNAVAILABLE` with a coverage impact, never a zero. Provider and
model names, when present, are data to be counted, never logic to be special-cased.

## Basis (authoritative, from the completed assessment)

- Run evidence census (2026-10-08): 164 run directories, artifact counts as listed above.
- `docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md` — the authoritative per-metric AVAILABLE / PARTIAL / MISSING classification and the non-inference / `UNAVAILABLE` rules.
- `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md` §3–§4 — the retrieval-overhead and token rules.
- `internal/perf/perf.go` — `Task.StagesMS` / `Task.TotalMS` / `Counts` (`fix_cycles`, `plan_repairs`, `validation_runs`, `review_runs`, …).
- `internal/run/*.go` — typed readers for approvals, attempts, routing artifacts, external completion.
- `docs/specs/HUMAN-APPROVAL.md`, `docs/specs/RECOVERY.md`, `docs/specs/MODEL-ROUTING.md` — the semantics each metric reports on.

## Metric Register (the seven dimensions)

The register fixed by this plan. Status legend: **AVAILABLE** (artifact present and
populated), **PARTIAL** (present but not for every run / no dedicated field),
**MISSING / UNAVAILABLE** (the lifecycle does not record it).

| # | Dimension | Status | Primary source |
| --- | --- | --- | --- |
| 1 | Agent success / failure rate | AVAILABLE | `report.json` `decision`; run-trace `Termination` |
| 2 | Routing / finding-severity distribution | PARTIAL | `model-selection.json` (routing opt-in); `review.json` severities |
| 3 | Fix-loop convergence | AVAILABLE | `metrics.json` `counts.fix_cycles` / `plan_repairs`; `report.json` `fix_cycles` |
| 4 | Retry efficiency | PARTIAL | `attempt.txt`, `continuations.txt`, `classification.json` |
| 5 | Human-approval frequency | AVAILABLE | `report.json` `decision=NEEDS_HUMAN`; `approval.json` |
| 6 | Execution latency | AVAILABLE | `metrics.json` `total_ms` / `stages_ms` / `validation_ms` |
| 7 | Token usage | MISSING / UNAVAILABLE | none; informational only, never policy |

## Baseline / Evaluation Acceptance Criteria (GO / HOLD)

The aggregation is accepted (**GO**) only when **all** hold:

- every emitted metric states its **denominator** (the number of runs eligible for it) and a **coverage percentage** (runs carrying the artifact ÷ eligible runs);
- a run missing an artifact is counted as `UNAVAILABLE` and excluded from the denominator of that metric, never folded in as `0`;
- two runs over the same inputs produce **identical** aggregate output (reproducibility);
- `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race ./...` pass and `gofmt -l .` is clean;
- token usage is reported `UNAVAILABLE` where not recorded and appears in **no** gate, budget, or routing field;
- no file under `internal/cli/` and no production runtime path is modified by RM-001…RM-004.

**HOLD** when any required metric cannot be established without inference or the
coverage denominator is ambiguous; **NO-GO** when a metric would require inventing a
value, changing an artifact schema, or using a token count as authority. On either,
keep the baseline, record it, and stop.

## Prerequisites Requiring Human Approval

1. **RM-005 surface authorization.** Any eventual opt-in `sop metrics` / `sop report --all` command is production-affecting and requires a human decision; RM-005 only records the decision.
2. **Commit authorization.** RM-001…RM-005 are not committed and the plan is not closed without separate human approval.
3. **Token-metric authorization.** Continued confirmation that token consumption is informational only, never policy.

RM-001…RM-004 require no approval to author and run: they are read-only or add a pure
library plus tests, are model-free, and change no production runtime path.

## Deferrals

- **`VERIFCACHE-PERSISTENCE`** — the persistent cross-run verification cache remains deferred (unchanged). See `docs/plans/BACKLOG.md`.
- **E2 (live retrieval A/B)** — remains gated behind the EV plan's Option A / Option B human decision; untouched here.
- **Graphify evaluation** — remains the last, unscheduled candidate at the tail of the backlog; not evaluated, scheduled, or implemented by this plan.
- **Local-only validation configuration** — tracked as a separate backlog item, not a task here (a config-only change under the git-ignored `.agent-sdlc/` cannot be a governed task).

## Safety Invariants

This plan must not:

- change any production runtime behavior or add instrumentation to a live path;
- add a third-party dependency, or any provider/model/task-domain-specific logic;
- use a token count as budget, routing, or acceptance authority;
- alter approval, retry, replan, budget, verification, or commit semantics;
- overwrite or reinterpret EV, CONV, RSH, or H001–H004 evidence;
- write `.agent-sdlc` state by hand, or bypass any human approval boundary.

## Dependency Graph

```text
RM-001 (run-artifact inventory, read-only)
    |
RM-002 (metric register + classification, read-only)
    |
RM-003 (offline aggregator, pure library + tests, model-free)
    |
RM-004 (reproducibility / coverage / neutrality verification, read-only)
    |
RM-005 (decision record: surface opt-in command? — read-only)
    |
    +-- GO  --> [human decision] --> SEPARATE plan wires `sop metrics` / `sop report --all`
    +-- HOLD/NO-GO --> keep the baseline; record; stop
```

## RM-001 — Run-Artifact Inventory and Schema Census

Enumerate the recorded run evidence and fix the deterministic aggregation contract:
which artifact carries which field, for how many runs, so every later metric has an
explicit, verifiable denominator.

### Authoritative Inputs

- `.agent-sdlc/runs/<TASK-ID>/` (the 164 recorded run directories and their artifacts)
- `internal/run/*.go`, `internal/perf/perf.go` (the existing artifact readers)
- `docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md`

### Mutation Targets

- `docs/reports/run-metrics/RM-001-artifact-inventory.md` — the only file this task writes; no production change.

### Dependencies

None

### Requires

- Go build/test/race toolchain
- Recorded run artifacts

### Deliverables

- `docs/reports/run-metrics/RM-001-artifact-inventory.md` — a per-artifact census table (artifact name, runs carrying it, runs missing it, coverage percentage of the 164 directories), the field list each artifact exposes, and the aggregation contract (eligible runs and denominator per planned metric).

### Acceptance Criteria

- The census lists every artifact type present under `.agent-sdlc/runs/` with its exact run count and coverage percentage.
- Each of the seven dimensions names the artifact(s) and field(s) it will read, or is explicitly marked as having no recorded source.
- The contract states that an absent artifact is `UNAVAILABLE`, distinct from a recorded zero value, and defines the eligible-run denominator per metric.
- No production file is created, modified, or deleted.

### Execution Contract

1. Inventory `.agent-sdlc/runs/` by artifact type.
2. Read `internal/run` and `internal/perf` to confirm the field each artifact exposes.
3. Produce the census and the aggregation contract.
4. Finish.

Expected first action: count artifacts under `.agent-sdlc/runs/`.

### Production-Change Scope

None

## RM-002 — Metric Register and Classification

Fix the seven metric dimensions with their source field, classification, denominator,
and coverage rule, reusing the `CLOSE-008` classification so the register introduces no
new metric and omits none the assessment named.

### Authoritative Inputs

- `docs/reports/run-metrics/RM-001-artifact-inventory.md`
- `docs/reports/pre-performance-closure/CLOSE-008-telemetry-inventory.md`
- `docs/reports/ev-context-run/EV-004-instrumentation-gaps.md`
- `internal/perf/perf.go`, `internal/run/routing_artifact.go`, `internal/run/approval.go`

### Mutation Targets

- `docs/reports/run-metrics/RM-002-metric-register.md` — the only file this task writes; no production change.

### Dependencies

- RM-001

### Requires

- Go build/test/race toolchain
- Recorded run artifacts
- Run-artifact readers

### Deliverables

- `docs/reports/run-metrics/RM-002-metric-register.md` — the register: for each of the seven dimensions, the source field, the AVAILABLE / PARTIAL / MISSING classification with an anchor, the denominator, the coverage rule, and the exact zero-vs-`UNAVAILABLE` handling; plus the binding token rule.

### Acceptance Criteria

- All seven dimensions are present, each with a source field (or an explicit "no recorded source") and a classification carried from `CLOSE-008`.
- Every dimension states its denominator and coverage rule; an unreported value is `UNAVAILABLE`, never inferred from text, byte, or diff size.
- Token usage is classified MISSING / `UNAVAILABLE` and is excluded from every gate, budget, and routing use.
- No production file is created, modified, or deleted.

### Execution Contract

1. Read RM-001, `CLOSE-008`, and `EV-004`.
2. Fix the register and the classification.
3. Produce the report.
4. Finish.

Expected first action: read `docs/reports/run-metrics/RM-001-artifact-inventory.md`.

### Production-Change Scope

None

## RM-003 — Offline Aggregator (Pure Library and Tests)

Implement the deterministic, model-free aggregation as a pure internal library plus
tests and golden fixtures. It reads recorded run artifacts through the existing reader
shapes, emits the RM-002 metrics with denominators and coverage, and is wired to **no**
CLI command and **no** production runtime path.

### Authoritative Inputs

- `docs/reports/run-metrics/RM-002-metric-register.md`
- `internal/run/*.go`, `internal/perf/perf.go` (reused readers)
- `internal/retrievalgate/*_test.go` (the EV-003 test-only harness precedent)

### Mutation Targets

- `internal/runmetrics/` — a new, pure library package (aggregation logic)
- `internal/runmetrics/testdata/` — golden fixtures (synthetic run directories)
- `internal/runmetrics/*_test.go` — unit, golden, and reproducibility tests
- `docs/reports/run-metrics/RM-003-aggregation-results.md` — the measured aggregate over the live `.agent-sdlc/runs/` evidence

### Dependencies

- RM-002

### Requires

- Go build/test/race toolchain
- Recorded run artifacts
- Run-artifact readers

### Deliverables

- `internal/runmetrics/` — a deterministic aggregator with no new dependency and no provider/model-specific logic.
- `internal/runmetrics/testdata/` — fixtures covering present, absent, and zero-valued artifacts.
- `internal/runmetrics/*_test.go` — tests asserting the goldens, the zero-vs-`UNAVAILABLE` distinction, coverage percentages, and reproducibility.
- `docs/reports/run-metrics/RM-003-aggregation-results.md` — the aggregate over the live run evidence, with per-metric denominators and coverage.

### Acceptance Criteria

- The aggregator reads only recorded artifacts and makes no model call and no network call.
- Absent and zero-valued artifacts are handled distinctly: absent is `UNAVAILABLE` and reduces coverage, zero is a recorded measurement.
- Each emitted metric includes its denominator and coverage percentage.
- Golden fixtures pass and two runs over the same inputs produce identical output.
- No file under `internal/cli/` is modified and no production runtime path imports the new library; the package adds no `go.mod` requirement.
- Token usage is emitted as `UNAVAILABLE` when not recorded and is absent from any gate/budget/routing field.

### Execution Contract

1. Read RM-002 and the reused readers.
2. Implement the pure aggregator and its tests with golden fixtures.
3. Produce the aggregate over the live run evidence.
4. Finish.

Expected first action: read `docs/reports/run-metrics/RM-002-metric-register.md`.

### Production-Change Scope

Adds a new internal library package and tests only; no existing runtime path is modified and no CLI command is wired.

## RM-004 — Reproducibility, Coverage, and Neutrality Verification

Independently verify that the aggregate is deterministic, that coverage reconciles to
the stated denominators, that absence is never read as zero, and that nothing in the
production path changed.

### Authoritative Inputs

- `internal/runmetrics/` and its tests and fixtures
- `docs/reports/run-metrics/RM-003-aggregation-results.md`
- `docs/reports/run-metrics/RM-002-metric-register.md`

### Mutation Targets

- `docs/reports/run-metrics/RM-004-verification.md` — the only file this task writes; no production change.

### Dependencies

- RM-003

### Requires

- Go build/test/race toolchain
- Run-artifact readers

### Deliverables

- `docs/reports/run-metrics/RM-004-verification.md` — the re-run identity check, the coverage reconciliation, the zero-vs-`UNAVAILABLE` demonstration, the no-production-path-change assertion, and the provider/model-neutrality assertion, with the exact commands and results.

### Acceptance Criteria

- A re-run of the aggregation over identical inputs yields byte-identical output (recorded).
- Every metric's coverage percentage reconciles to the RM-001 census denominators.
- A fixture with a missing artifact is proven `UNAVAILABLE` (not `0`); a fixture with a recorded `0` is proven `0`.
- It is demonstrated by import/symbol search that no production path references `internal/runmetrics`.
- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, and `go test -race ./...` all pass; `git diff --check` is clean.
- No production file is created, modified, or deleted by this task.

### Execution Contract

1. Re-run the aggregation and compare outputs.
2. Reconcile coverage to the census denominators.
3. Search for any production import of the new library.
4. Produce the verification report.
5. Finish.

Expected first action: re-run the aggregation test and diff its output.

### Production-Change Scope

None

## RM-005 — Decision Record: Offline Aggregate vs Opt-in Command

Record a GO / HOLD decision on whether to surface the verified aggregate as an opt-in
`sop metrics` / `sop report --all` command under a separate, evidence-gated plan. Do
not implement the surface here.

### Authoritative Inputs

- `docs/reports/run-metrics/RM-004-verification.md`
- `docs/reports/run-metrics/RM-003-aggregation-results.md`
- `docs/reports/ev-context-run/EV-005-acceptance-criteria.md` (the Option A / B decision precedent)

### Mutation Targets

- `docs/reports/run-metrics/RM-005-decision.md` — the only file this task writes; no production change.

### Dependencies

- RM-004

### Requires

- Go build/test/race toolchain

### Deliverables

- `docs/reports/run-metrics/RM-005-decision.md` — the GO / HOLD / NO-GO decision on surfacing an opt-in command, its evidence, the fail-closed stop conditions, and the prerequisite approvals a future surface plan would need.

### Acceptance Criteria

- The decision names the chosen option and cites the RM-004 evidence.
- No production surface is implemented; the report states that explicitly.
- The stop conditions mirror the EV fail-closed discipline (no token authority, no provider/model-specific behavior, no lifecycle change).
- Prerequisite approvals for a future surface plan are enumerated, or the option to leave the aggregate offline-only is stated.

### Execution Contract

1. Read the RM-004 verification and the RM-003 results.
2. Decide and record GO / HOLD / NO-GO for a future surface.
3. Produce the decision record.
4. Finish.

Expected first action: read `docs/reports/run-metrics/RM-004-verification.md`.

### Production-Change Scope

None
