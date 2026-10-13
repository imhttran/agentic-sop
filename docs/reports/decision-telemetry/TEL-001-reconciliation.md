# TEL-001 — Telemetry Reconciliation and Common Metric Definitions

Status: read-only reconciliation and metric register. This report introduces no
production change of its own; the code it defines lives in `internal/runmetrics`
and is described in [PLAN-Decision-Telemetry-Extension.md](../../plans/PLAN-Decision-Telemetry-Extension.md).

This report covers **TEL-001** (reconcile existing telemetry) and **TEL-002**
(common metric definitions).

## 1. Reconciliation with existing telemetry (TEL-001)

Decision telemetry does not start a new collector. It extends the one the run-metrics
work already built.

| Existing artifact / code | What it already records | Reused for decision telemetry as |
| --- | --- | --- |
| `internal/perf` (`perf.Task`, `perf.Counts`, `perf.JEV`) | stage latency, operation counts, JEV invocation count (`JEV.Invocations`) | the JEV-invocation dimension (`jev_invocation`) |
| `internal/runmetrics` (RM-003 pure aggregator) | the seven RM-002 dimensions, `MetricResult` shape, UNAVAILABLE-vs-zero rule | the host of the decision dimensions (`Aggregate.Decisions`) |
| `model-selection.json` (`model.Selection`) | class, provider, model, locality, source, fallback, reason | model-tier distribution, override rate |
| `routing.json` (`run.RoutingArtifact`) | class, **source** (`policy`/`manual_override`/`default`/`escalation`), reasons, execution source | the escalation dimension |
| `report.json` decision | terminal `decision`/`verdict` (including `NEEDS_HUMAN`) | the escalation (human) sub-signal |
| shadow evaluation (`disabled → shadow → live`) | **documented, not implemented**: no runtime `shadow` mode exists, and no run artifact records an adapter recommendation | nothing — the agreement dimension is UNAVAILABLE |

RM-001–RM-005 status: RM-001 (artifact census) and RM-002 (seven-dimension register)
are the authoritative coverage source carried here. RM-003 is the shipped aggregator
(`internal/runmetrics`). RM-004 (verification) and RM-005 (aggregation-vs-command
decision) remain authoritative for the RM work; TEL-006 keeps the same
pure-library/opt-in-command separation but does **not** add a command (see §5).

**No duplicate schema.** The decision dimensions reuse `MetricResult` and its five
measurement quantities (total, eligible, artifacts-present, valid, unavailable) plus
the explicit numerator/denominator/coverage contract. A single additive field,
`Breakdown []BreakdownEntry`, carries a distribution (tier/source/fallback counts);
it is omitted for the RM-002 metrics, so their output is unchanged.

**No duplicate collector.** The reader is `internal/runmetrics.readEvidence`; the
new reads (model-selection source/fallback, routing.json source, JEV invocation)
extend it rather than adding a parallel reader.

## 2. Metric register (TEL-002)

Each dimension states its source artifact/field, its eligible-run denominator, its
numerator, its coverage, and its status. Status vocabulary is the RM-002 one:
AVAILABLE, PARTIAL, UNAVAILABLE.

### 2.1 Model-tier distribution — `model_tier_distribution` — PARTIAL

- **Source.** `model-selection.json` (`class`, `source`, `fallback`).
- **Present.** Runs carrying `model-selection.json`. **Valid.** Runs whose
  `model-selection.json` parsed and recorded a selection (`class`/`provider`/`model`/`source`).
- **Numerator.** The number of valid runs (a count); the tier/source/fallback
  breakdown is computed over those valid runs only.
- **Coverage.** Valid / corpus. Routing is opt-in, so RM-001 fixes the
  `model-selection.json` denominator at **50 runs (30.5% of 164)**.
- **Zero vs UNAVAILABLE.** A run without `model-selection.json` is UNAVAILABLE,
  never a "default" tier. An unknown class is excluded from the tier breakdown,
  never bucketed as another tier.

### 2.2 Override rate — `decision_override_rate` — PARTIAL

- **Source.** `model-selection.json` `source == "cli"` (an explicit `--model-class`
  override; `model.SourceCLI`).
- **Numerator.** Runs whose recorded source is `cli`. **Denominator.** Runs whose
  `model-selection.json` parsed and recorded a non-empty source.
- **Coverage.** Valid / corpus (bounded by the 50-run `model-selection.json`
  denominator). A run without a recorded source is UNAVAILABLE.

### 2.3 Escalation rate — `decision_escalation_rate` — PARTIAL

- **Sources.** `routing.json` `source == "escalation"` (a recovery class escalation)
  or `report.json` `decision == NEEDS_HUMAN` (a human escalation).
- **Numerator.** Runs recording either escalation. **Denominator.** Runs carrying
  `routing.json` or `report.json` and recording a routing source or a terminal
  decision.
- **Coverage.** Valid / corpus. `report.json` coverage is **132 runs (80.5%)** per
  RM-001; `routing.json` is not separately censused there, so it contributes only
  where present. Absence is UNAVAILABLE, never a recorded zero escalation.

### 2.4 JEV invocation — `jev_invocation` — AVAILABLE

- **Source.** `metrics.json` `JEV.Invocations` (`internal/perf`).
- **Numerator.** The summed recorded invocation count. **Denominator.** Runs whose
  `metrics.json` parsed. **Coverage.** Bounded by the **146-run (89.0%)**
  `metrics.json` denominator (RM-001).
- **Zero vs UNAVAILABLE.** A present `metrics.json` with no `jev` section is a
  recorded zero invocation; an absent `metrics.json` is UNAVAILABLE.

### 2.5 Adapter agreement — `adapter_agreement` — UNAVAILABLE

- **Source.** **None.** No run artifact records a decision-adapter (shadow)
  recommendation or its agreement with a deterministic expectation; shadow
  evaluation is a documented mode with no runtime implementation.
- **Value.** UNAVAILABLE for every run, numerator 0, denominator 0. It is never
  inferred and never treated as agreement or dissent. TEL-003 is recorded as
  pending until an adapter records comparable evidence (see §4).

### 2.6 Decision confidence — `decision_confidence` — UNAVAILABLE

- **Source.** **None.** A routing decision's confidence is not persisted: the
  routing artifact records the class, source, reasons, and typed signals, not a
  confidence.
- **Value.** UNAVAILABLE for every run; never inferred from a signal or a class.

### 2.7 Latency, review, approval, token dimensions

These are already the RM-002 dimensions and are not restated. Decision telemetry does
not add a second latency dimension: `execution_latency` remains the single latency
source, and `token_usage` remains UNAVAILABLE.

## 3. Comparability rules across providers (TEL-002)

- A decision metric reads recorded, provider-neutral fields only (`class`,
  `source`, `fallback`, `JEV.Invocations`); it contains no provider- or model-specific
  logic and matches source values (`cli`, `escalation`) by value, not by importing a
  model/provider package.
- Two runs are comparable for a decision metric only when their source artifact and
  field are present; an absent artifact is excluded from the denominator, never
  zero-filled, so coverage is honest across providers.
- UNAVAILABLE (unknown) and a recorded zero are never conflated: absence reduces
  coverage; a present explicit zero is a real measurement.

## 4. Missing data vs zero (TEL-002)

The decision dimensions inherit the RM-001 §5.1 rule verbatim: a metric value is
either a recorded number or UNAVAILABLE; UNAVAILABLE values are excluded from the
denominator. `adapter_agreement` and `decision_confidence` are UNAVAILABLE because
**no source exists**, which the register records explicitly rather than estimating.

## 5. Offline reporting (TEL-006) and no production change

- The aggregator stays a pure library. It is wired to no CLI command, guarded by
  `internal/runmetrics/unwired_test.go` (`TestNoProductionImportOfRunmetrics`,
  `TestNoCLIModification`). An opt-in `sop report` section is deferred, as the plan's
  constraint "reuse existing CLI infrastructure if an opt-in command is approved"
  requires a separate approval and would change those guards.
- JSON output is the existing `Aggregate` plus the additive `decisions` array; the
  human-readable summary is `runmetrics.RenderText`. Both are deterministic: two
  aggregations over identical inputs are byte-identical.
- No decision-path change: nothing in the aggregator is read by routing, approval, or
  execution; it is diagnostic only.
