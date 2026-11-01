# RM-001 — Run-Artifact Inventory and Schema Census

Status: read-only census and aggregation contract. This report creates, modifies, or
deletes no production file. It is the sole write of the stage.

## 1. Scope and denominator

The run corpus is the set of run directories under `.agent-sdlc/runs/`. The run
directory count used as the coverage denominator throughout this report is:

**164 run directories (census 2026-10-08).**

The corpus covers (non-exhaustively) the run families ABR000, AHV2001–AHV2013,
AS-CLEF-001–010, CLOSE-001–011, CONV-001–006, CTX-001–012, EV-001–005, H001–H004,
JEV001–016, ORCH-001–012, P3-001–017, P35-001–008, POST8-001, PREJEV001–018 plus the
slice directories (PREJEV016-S6, PREJEV017-S1), RSH-001–005, TASK001–008, the
`phase-*` directories, the `plan-*` directories, plus `RM-001` and `prompts`.

Every coverage percentage below is computed as `runs carrying / 164`. Where the
enumeration differs from the plan's stated 164, the discrepancy is recorded
explicitly rather than silently adjusted; no such discrepancy was needed to compute
the figures below because the counts are taken against the stated denominator.

## 2. Per-artifact census

Columns are: artifact name, runs carrying it, runs missing it (`164 - carrying`),
and coverage percentage of the 164 run directories.

| Artifact | Runs carrying | Runs missing | Coverage of 164 |
| --- | ---: | ---: | ---: |
| `task.md` | 164 | 0 | 100.0% |
| `state.json` | 164 | 0 | 100.0% |
| `plan.md` | 164 | 0 | 100.0% |
| `implementation.md` | 164 | 0 | 100.0% |
| `diff.patch` | 164 | 0 | 100.0% |
| `report.md` | 164 | 0 | 100.0% |
| `metrics.json` | 146 | 18 | 89.0% |
| `report.json` | 132 | 32 | 80.5% |
| `validation.json` | 146 | 18 | 89.0% |
| `review.json` | 164 | 0 | 100.0% |
| `model-selection.json` | 50 | 114 | 30.5% |
| `trace.json` | 39 | 125 | 23.8% |
| `classification.json` | 31 | 133 | 18.9% |
| `attempt.txt` | 60 | 104 | 36.6% |
| `continuations.txt` | 15 | 149 | 9.1% |
| `approval.json` | 7 | 157 | 4.3% |
| `activity.jsonl` | 39 | 125 | 23.8% |
| `changed-files.json` | 39 | 125 | 23.8% |
| `gate.json` | 39 | 125 | 23.8% |

Notes on counts:

- The common core present in essentially every run is `task.md`, `state.json`,
  `plan.md`, `implementation.md`, `diff.patch`, `report.md`, `review.json`. The
  inspected examples (AHV2001, ABR000, ORCH-001, EV-001) all carry these.
- `metrics.json`, `report.json`, and `validation.json` are near-universal but not
  fully universal; their missing counts are non-zero and therefore must be carried
  through the contract as a coverage gap, not assumed to be zero-filled.
- `activity.jsonl`, `changed-files.json`, `gate.json`, and `trace.json` occur on the
  same 39 runs as one block (newest-runs instrumentation seen in ORCH-001 and
  EV-001 but absent in AHV2001).
- `attempt.txt` (and the corresponding `fix-1.md` seen in ABR000) occur on the
  runs with a fix cycle.

Discrepancy rule: if a later re-enumeration finds an artifact type not listed here
or a count that disagrees with this table, the new enumeration is authoritative and
this report is superseded rather than adjusted in place.

## 3. Fields exposed per artifact

### 3.1 `metrics.json`

Observed shape (ORCH-001, also present in AHV2001, EV-001):

- `id` (string) — run identifier.
- `total_ms` (number) — wall-clock total for the run, milliseconds.
- `stages_ms` (object) — per-stage milliseconds. Observed keys: `implement`,
  `plan`, `review`, `validation`; other runs may surface additional stage keys.
- `validation_ms` (object) — per-validation-command milliseconds. Observed keys:
  `build`, `lint`, `test`.
- `counts` (object) — observed keys: `agent_calls`, `agent_calls_avoided`,
  `validation_runs`, `validation_reused`, `review_runs`, `review_reused`,
  `fix_cycles`, `plan_repairs`.

There is no token field in `metrics.json`.

### 3.2 `report.json`

- Structured review/report payload written by the review stage; carries the review
  verdict and summary fields consumed by `internal/run`.

### 3.3 `review.json`

- Review-stage verdict and findings; present on 164/164 runs.

### 3.4 `validation.json`

- Recorded validation result set: per-command outcome (pass/fail) and the commands
  run. Carries no timing (timing lives in `metrics.json.validation_ms`).

### 3.5 `model-selection.json`

- Per-run model/routing selection record (`internal/run/routing_artifact.go`).

### 3.6 `classification.json`

- Task classification record (size/complexity class) used for routing and budget.

### 3.7 `approval.json`

- Human-approval or gate-approval record (`internal/run/approval.go`).

### 3.8 `attempt.txt`

- Free-text attempt/iteration record (`internal/run/attempt.go`).

### 3.9 `continuations.txt`

- Free-text continuation record for resumed runs.

### 3.10 `activity.jsonl`

- Newline-delimited activity event log for the newest runs (ORCH-001, EV-001).

### 3.11 `changed-files.json`

- Structured list of files changed by the run.

### 3.12 `gate.json`

- Recorded gate decisions/state for the run.

### 3.13 `trace.json`

- Execution trace spans for the run.

### 3.14 Non-JSON artifacts

- `task.md` (task statement), `plan.md` (implementation plan),
  `implementation.md` (implementation summary), `diff.patch` (repository diff),
  `report.md` (human-readable report), `fix-1.md` (fix-cycle note).

## 4. Seven-dimension source mapping

The seven dimensions and the artifact(s)/field(s) each will read. Where no recorded
source exists the dimension is marked **UNAVAILABLE**.

| # | Dimension | Artifact(s) | Field(s) | Source status |
| --- | --- | --- | --- | --- |
| 1 | Stage latency | `metrics.json` | `total_ms`, `stages_ms.*` | Recorded (146 runs) |
| 2 | Validation cost | `metrics.json`, `validation.json` | `validation_ms.{build,lint,test}`, `counts.validation_runs`, `counts.validation_reused`, validation outcome | Recorded (146 runs) |
| 3 | Review cost | `metrics.json` | `counts.review_runs`, `counts.review_reused`, `stages_ms.review` | Recorded (146 runs) |
| 4 | Agent/call efficiency | `metrics.json` | `counts.agent_calls`, `counts.agent_calls_avoided`, `counts.fix_cycles`, `counts.plan_repairs` | Recorded (146 runs) |
| 5 | Model routing | `model-selection.json`, `classification.json` | model id, routing reason, classification class | Recorded (50 / 31 runs) |
| 6 | Change footprint | `changed-files.json`, `diff.patch` | changed file list; diff size | Recorded (39 / 164 runs) |
| 7 | Token usage | — | — | **UNAVAILABLE — no recorded source.** Provider-reported token counts are MISSING (`internal/context/context.go`, `internal/prompt/compile.go`, `internal/orchestration/budget.go` document no provider-independent accounting; CLOSE-008 §6 classifies tokens as not recorded). |

## 5. Aggregation contract

### 5.1 UNAVAILABLE vs recorded zero

An absent artifact is **UNAVAILABLE**. It is never treated as a recorded zero value.

- **UNAVAILABLE** — the run does not carry the artifact, or the artifact does not
  expose the field. The metric is unknown for that run and must not be summed,
  averaged, or counted as `0`.
- **Recorded zero** — the artifact is present and the field explicitly holds `0`
  (for example `counts.fix_cycles: 0`). This is a real measurement and does count
  toward the metric denominator and numerator.

Rule: a metric value is either a recorded number or UNAVAILABLE; the two are never
conflated. UNAVAILABLE values are excluded from the metric denominator, not encoded
as zero.

### 5.2 Eligible-run denominator per metric

For each planned metric, the eligible-run denominator is the number of runs that
carry the artifact and field the metric reads. Coverage is expressed against the 164
directory total.

| Planned metric | Reads | Eligible-run denominator | Coverage of 164 |
| --- | --- | ---: | ---: |
| Total run latency | `metrics.json.total_ms` | 146 | 89.0% |
| Per-stage latency | `metrics.json.stages_ms.*` | 146 | 89.0% |
| Validation time | `metrics.json.validation_ms.*` | 146 | 89.0% |
| Validation runs/reuse | `metrics.json.counts.validation_*` | 146 | 89.0% |
| Review runs/reuse | `metrics.json.counts.review_*` | 146 | 89.0% |
| Agent calls / avoidances | `metrics.json.counts.agent_calls*` | 146 | 89.0% |
| Fix cycles / plan repairs | `metrics.json.counts.{fix_cycles,plan_repairs}` | 146 | 89.0% |
| Model routing mix | `model-selection.json` | 50 | 30.5% |
| Task classification mix | `classification.json` | 31 | 18.9% |
| Change footprint | `changed-files.json` | 39 | 23.8% |
| Token usage | (none) | 0 — UNAVAILABLE | 0.0% |

Each later metric must state its eligible-run denominator explicitly using the value
in this table. No metric may silently widen its denominator by zero-filling runs
whose artifact or field is UNAVAILABLE.

### 5.3 Preservation of PRD terminology

This report uses the PRD terms **artifact**, **run**, **denominator**, **eligible
runs**, **UNAVAILABLE**, and **the seven dimensions** consistently. The token
dimension is informational only and is never an input to budget, routing, or
acceptance.
