# RM-003 — Offline Aggregation Results

Aggregate produced by the pure `internal/runmetrics/` library over the live
recorded run evidence under `.agent-sdlc/runs/`. The library reads only the
recorded run artifacts through the existing `internal/run`, `internal/perf`, and
`internal/runtrace` reader shapes. It performs no model call and no network call
and never mutates a run directory.

This artifact is the library's real output, not a placeholder: every value below
was read from the recorded artifacts at the aggregation revision. Two runs over
the same inputs produce byte-identical output (`TestReproducible`, `TestGoldenOutput`).

## 1. Corpus and method

- **Corpus:** 168 run directories under `.agent-sdlc/runs/` at aggregation time.
  The corpus grows as runs are recorded (the audit census was 164 on 2026-10-08;
  it now includes the RM-001/RM-002/RM-003 runs themselves).
- **Numerator / denominator.** For a **rate** metric the numerator is the count of
  runs satisfying the condition and the denominator is the number of valid
  measurements; the value is the percentage rounded to one decimal. For a
  **magnitude** metric the numerator is the summed quantity and the denominator is
  the number of valid measurements. For a **count** metric the numerator is the
  count of runs and the denominator is the number of runs the count is measured
  over. In every case value and denominator are reported explicitly.
- **Coverage.** `coverage_pct = valid_measurements / total_runs` — the share of the
  corpus that carries a usable measurement. It is never inflated by absence.
- **Five quantities per metric.** `total_runs` (corpus), `artifacts_present` (runs
  carrying a source artifact), `valid_measurements` (runs yielding a usable
  value), `unavailable_measurements` (`total_runs − valid_measurements`), and the
  explicit `numerator`/`denominator`. An **absent** artifact is `UNAVAILABLE`; a
  **present but malformed** artifact is present but not a valid measurement; a
  **present zero** is a real measurement. The three states are never conflated
  (`TestMalformedArtifactsAreUnavailable`, `TestPartiallyPopulatedArtifact`,
  `TestMeasurementCategoryInvariants`).

## 2. Metric register results

| Metric                       | Status      |       Value | Unit   | Numerator | Denominator | Total runs | Artifacts present | Valid meas. | Unavailable | Coverage |
| ---------------------------- | ----------- | ----------: | ------ | --------: | ----------: | ---------: | ----------------: | ----------: | ----------: | -------: |
| `agent_success_failure_rate` | AVAILABLE   |        93.3 | %      |       126 |         135 |        168 |               135 |         135 |          33 |    80.4% |
| `routing_finding_severity`   | PARTIAL     |          53 | runs   |        53 |          53 |        168 |                53 |          53 |         115 |    31.5% |
| `fix_loop_convergence`       | AVAILABLE   |          84 | cycles |        84 |         150 |        168 |               150 |         150 |          18 |    89.3% |
| `retry_efficiency`           | PARTIAL     |          61 | runs   |        61 |          61 |        168 |                61 |          61 |         107 |    36.3% |
| `human_approval_frequency`   | AVAILABLE   |         6.7 | %      |         9 |         135 |        168 |               135 |         135 |          33 |    80.4% |
| `execution_latency`          | AVAILABLE   |    17000500 | ms     |  17000500 |         150 |        168 |               150 |         150 |          18 |    89.3% |
| `token_usage`                | UNAVAILABLE | UNAVAILABLE | tokens |         0 |           0 |        168 |                 0 |           0 |         168 |     0.0% |

Reading the table:

- **Agent success/failure rate** — 126 of 135 runs with a recorded terminal
  decision succeeded (93.3%); the decision is absent for 33 runs, which are
  `UNAVAILABLE` and excluded, never counted as failures.
- **Routing / finding-severity distribution** — routing is opt-in: only 53 runs
  (31.5%) carry `model-selection.json`, and every readable one records a selection.
  The informative figure for this PARTIAL metric is its coverage; the finding
  severity sub-signal is read from `review.json`.
- **Fix-loop convergence** — 84 fix cycles + plan repairs summed over 150 runs whose
  `metrics.json` parsed (mean ≈ 0.56 per measured run); 18 runs are unmeasured, not
  "converged in zero cycles".
- **Retry efficiency** — 61 runs (36.3%) recorded reusable escalation evidence
  (`attempt.txt` nonempty), and each recorded a positive retry signal; the remaining
  107 runs are `UNAVAILABLE`, never "0 retries". A `continuations.txt` of `0` is a
  recorded zero, not a retry.
- **Human-approval frequency** — 9 of 135 runs with a recorded decision required a
  human boundary (6.7%).
- **Execution latency** — 17,000,500 ms summed over 150 runs whose `metrics.json`
  parsed (mean ≈ 113 s/run); stage durations include harness and tool overhead.
- **Token usage** — `UNAVAILABLE`; no provider-independent count is persisted. It
  appears in no gate, budget, or routing field.

## 3. Artifact presence across the corpus

| Artifact               | State   | Present runs | Zero-valued runs |
| ---------------------- | ------- | -----------: | ---------------: |
| `report.json`          | present |          135 |                0 |
| `metrics.json`         | present |          150 |                0 |
| `review.json`          | present |          130 |                0 |
| `model-selection.json` | present |           53 |                0 |
| `trace.json`           | present |           42 |                0 |
| `classification.json`  | present |           32 |                0 |
| `attempt.txt`          | present |           61 |                0 |
| `continuations.txt`    | present |           15 |                0 |
| `approval.json`        | present |            8 |                0 |

## 4. UNAVAILABLE metrics

| Metric        | Status      | Reason                                                                                                                                                                                                                                                |
| ------------- | ----------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `token_usage` | UNAVAILABLE | No provider-independent token count is persisted (`internal/context/context.go`, `internal/prompt/compile.go`, `internal/orchestration/budget.go`; CLOSE-008 §6). Token usage is informational only and appears in no gate, budget, or routing field. |

> Values are `UNAVAILABLE` (not zero) whenever the underlying artifact was not
> recorded. Token usage is never inferred from text, byte, or diff size.

## 5. Scope note

The aggregate is diagnostic only. No file under `internal/cli/` references this
library (`TestNoCLIModification`) and no production package imports it
(`TestNoProductionImportOfRunmetrics`), so it is wired to no runtime path and drives
no decision.
