# TEL-003 — Decision Adapter Telemetry Integration

**Status:** IMPLEMENTED — Stage 1 (evidence producer in `sop-decision-adapters`) and
Stage 2 (read-only consumer in `agentic-sop/internal/runmetrics`) are complete and
verified. Nothing is committed or pushed.

This report records the implemented TEL-003 evidence contract in
[PLAN-Decision-Telemetry-Extension.md](../../plans/PLAN-Decision-Telemetry-Extension.md).
It is non-normative evidence; the boundary rules are normative in
[DECISION-TELEMETRY.md](../../specs/DECISION-TELEMETRY.md).

## 1. Evidence contract (Stage 1, producer)

The adapter-side shadow harness (`internal/shadow/shadow.go`) already computed
agreement, disagreement, invalid-choice, indeterminate, timeout/failure, latency
percentiles, and a confidence histogram **in memory**. TEL-003 adds a versioned,
offline serialization of those recordings — no new evaluation engine.

- **Artifact name:** `shadow-evidence.json`, schema version `1`
  (`shadow.EvidenceSchemaVersion`).
- **Producer surface:** the library (`shadow.ProduceEvidence`, `(*Recorder).Evidence`,
  `shadow.MarshalEvidence`, `shadow.WriteEvidence`, `shadow.ReadEvidence`) and the
  operator CLI `sop-decision-adapter shadow -in <run-sheet.json> [-out <evidence.json>]`.
- **Reuse, not duplication.** A single classification path
  (`(*Recorder).recordOutcome`) is shared by live observation (`evaluateSource`) and
  offline replay (`(*Recorder).RecordSupplied`). No `Observe` call and no model call
  is made by the producer: it replays recorded observations, so its output is
  deterministic.
- **Fields persisted per observation:** `case_id`, `source` (provenance),
  `comparable`, `outcome`, `primary`, `shadow`, `reference`, `confidence` (nullable),
  `latency_ms`.

Schema:

```json
{
  "schema_version": 1,
  "cases": [
    {
      "case_id": "c1",
      "source": "clef",
      "comparable": true,
      "outcome": "shadow_agreement",
      "primary": "a",
      "shadow": "a",
      "reference": "a",
      "confidence": 0.9,
      "latency_ms": 10
    }
  ]
}
```

The producer validates its input and fails closed (unique sources and case IDs,
declared observation sources, well-formed questions). An unknown schema version is
rejected by `ReadEvidence`, and the consumer treats it as present-but-UNAVAILABLE.

## 2. Consumer (Stage 2, `agentic-sop/internal/runmetrics`)

The aggregator reads `<run>/shadow-evidence.json` read-only through the existing
reader (`readEvidence`) and emits these decision dimensions, appended after the RM-002
metrics. Denominators mirror the adapter harness exactly, so the offline metrics
reproduce the harness definitions.

| Metric                    | Numerator                                                 | Denominator                                       | Coverage                            |
| ------------------------- | --------------------------------------------------------- | ------------------------------------------------- | ----------------------------------- |
| `adapter_invocation`      | recorded observations                                     | recorded observations                             | runs whose evidence parsed / corpus |
| `adapter_agreement`       | comparable observations whose choice equals the reference | comparable observations that recorded a reference | same                                |
| `adapter_confidence`      | comparable observations that recorded a confidence        | comparable observations                           | same                                |
| `adapter_latency`         | summed latency (value = p50, breakdown p50/p95)           | comparable observations                           | same                                |
| `adapter_invalid_choice`  | comparable observations with `shadow_invalid_choice`      | comparable observations                           | same                                |
| `adapter_indeterminate`   | comparable observations with `shadow_indeterminate`       | comparable observations                           | same                                |
| `adapter_timeout_failure` | comparable observations with timeout/error/malformed      | comparable observations                           | same                                |

Rules carried from the harness and the telemetry contract:

- A **comparable** observation is one the source actually evaluated; a
  `skipped_not_comparable` record is excluded from every denominator.
- **Agreement** counts only a comparison that recorded a reference; a recorded
  no-reference agreement is not counted as agreement. Agreement is evidence, not
  proof of correctness.
- **Confidence** is coverage of recorded confidences, bucketed into fixed `[0,1]`
  ranges; a missing confidence is not a zero.
- **Latency** uses the nearest-rank p50/p95 (the harness's definition); it is
  diagnostic only.
- A run without evidence — or with an unknown schema version — yields `UNAVAILABLE`
  and is excluded from the denominator, never a zero observation.

## 3. Available vs unavailable

**Available (measured when an evidence artifact is present):** adapter invocation,
agreement (overall), confidence coverage, latency p50/p95, invalid-choice,
indeterminate, timeout/failure.

**Unavailable (no recorded source, never fabricated):**

- **Agreement split by reference provenance.** The artifact records the reference
  _value_ but not whether it came from a deterministic expectation or a recorded SOP
  outcome, so the plan's two separate agreement metrics collapse into one
  `adapter_agreement` over all references. Separating them would require extending
  the evidence schema (a future change).
- **Routing decision confidence** (`decision_confidence`) — unchanged: a routing
  decision's confidence is not persisted in any run artifact.
- **Per-adapter Nimble-only agreement** is not surfaced as a separate metric; the
  per-source agreement/disagreement counts are in the `adapter_agreement` breakdown.

## 4. Files changed

**Stage 1 — `sop-decision-adapters`:**

- `internal/shadow/shadow.go` — extracted the shared `recordOutcome`; added
  `(*Recorder).RecordSupplied`; tagged `Reference` for a stable run-sheet contract.
- `internal/shadow/evidence.go` (new) — the versioned evidence schema and
  read/write helpers.
- `internal/shadow/runsheet.go` (new) — the offline run-sheet and `ProduceEvidence`.
- `internal/shadow/evidence_test.go` (new) — determinism, outcomes, comparability,
  malformed/version rejection, fail-closed, JSON shape.
- `cmd/sop-decision-adapter/main.go` — the `shadow` subcommand.

**Stage 2 — `agentic-sop`:**

- `internal/runmetrics/runmetrics.go` — read `shadow-evidence.json`.
- `internal/runmetrics/decision.go` — the adapter metrics and helpers.
- `internal/runmetrics/decision_test.go` — adapter-metric and unknown-version tests.
- `internal/runmetrics/testdata/decisions/.../shadow-evidence.json` (new) and
  `internal/runmetrics/testdata/adapterbad/...` (new) fixtures.
- Goldens regenerated (additive).

## 5. Validation

All commands run and observed green:

- `sop-decision-adapters`: `gofmt -l .` clean; `go build ./...`; `go vet ./...`;
  `go test ./...`; `go test -race ./internal/shadow/ ./decision/`.
- CLI smoke: `sop-decision-adapter shadow -in sheet.json` emitted a
  `schema_version: 1` artifact with the expected observations.
- `agentic-sop`: `gofmt -l .` clean; `go build ./...`; `go vet ./...`;
  `go test ./internal/runmetrics/ ./internal/archtest/`; `go test -race ./internal/runmetrics/`.

No production decision authority, routing, or approval behavior changed; the
aggregator remains unwired (guarded by `internal/runmetrics/unwired_test.go`), and no
live shadow execution or model call occurs in `agentic-sop`.

## 6. Remaining gaps

- Agreement is not split by reference provenance (deterministic vs recorded SOP
  outcome) — the evidence schema would need a reference-source field.
- No consumer CLI/`sop report` surface (unwired guard; opt-in command not approved).
- Per-adapter confidence distribution is aggregate-only.
- Julia/Nimble evidence is exercised only through offline replay, not a live runtime.
