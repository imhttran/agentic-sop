# PLAN — Decision Telemetry Extension

**Status:** IN PROGRESS — TEL-001, TEL-002, TEL-004, TEL-005 and the in-repo part of
TEL-006 are implemented on `feat/decision-telemetry`; TEL-003 (decision-adapter
metrics) and the `sop report` surface of TEL-006 are pending (see the status table
below). Verification (TEL-007) ran for the changed package.

**Primary repository:** agentic-sop
**Related repositories:** sop-decision-adapters, sop-controller

## Goal

Extend existing run metrics and shadow evaluation to measure model selection, JEV
usage, decision-adapter quality, and execution overhead without changing SOP's
decision authority.

## Tasks

### TEL-001 — Reconcile Existing Telemetry

- Review RM-001–RM-005 and existing implementation status.
- Reuse `internal/run`, `internal/perf`, and existing shadow evaluation.
- Identify missing metrics and coverage.
- Avoid duplicate schemas and collectors.

### TEL-002 — Common Metric Definitions

- Define invocation, agreement, escalation, latency, confidence, and model-tier metrics.
- Specify numerator, denominator, and coverage for each.
- Distinguish missing data from zero.
- Define comparability rules across providers.

### TEL-003 — Decision Adapter Metrics

- Reuse existing shadow evaluation.
- Include JEV, Nimble, Julia, and Clef where supported.
- Measure agreement with deterministic expectations and recorded SOP outcomes.
- Keep all shadow results observational.

### TEL-004 — Model Picker Distribution

- Aggregate SMALL, MEDIUM, and LARGE selections.
- Capture selected provider, model, fallback, and selection reason where recorded.
- Report percentages and coverage.
- Do not infer missing model selections.

### TEL-005 — Decision Effectiveness

- Measure recorded overrides, human escalations, and failures.
- Compare recommendations against outcomes only when semantically comparable.
- Do not treat agreement alone as proof of correctness.

### TEL-006 — Offline Reporting

- Extend the existing offline metrics aggregator.
- Provide deterministic JSON and human-readable summaries.
- Reuse existing CLI infrastructure if an opt-in command is approved.
- No model calls or runtime mutations.

### TEL-007 — Verification

- Test missing and malformed artifacts.
- Verify identical inputs produce identical output.
- Verify zero changes to routing, approval, and execution behavior.
- Run targeted tests, build, vet, and race tests for changed Go packages.

## Constraints

- No new telemetry service or database.
- No production decision-path changes.
- No new dependencies without justification.
- Preserve existing artifact compatibility.
- No provider-specific policy logic.
- No commit or push without separate authorization.

## Completion Criteria

A reproducible report shows model-tier usage, JEV invocation, adapter agreement,
latency, and escalation metrics with explicit coverage. Missing measurements remain
unavailable rather than estimated.

## Implementation status

The existing offline aggregator `internal/runmetrics` (the RM-003 pure library) is
extended, not replaced: the decision dimensions are emitted in a new `decisions`
section **after** the seven RM-002 metrics, which are byte-for-byte unchanged. The
aggregator remains unwired to any CLI command, as guarded by
`internal/runmetrics/unwired_test.go`.

| Task | Status | Where |
| --- | --- | --- |
| TEL-001 | Done | [TEL-001-reconciliation.md](../reports/decision-telemetry/TEL-001-reconciliation.md) |
| TEL-002 | Done | same report, §2 (the metric register) |
| TEL-003 | Partial — UNAVAILABLE | `adapter_agreement` has no recorded source in this repository; shadow evaluation has no runtime implementation, so it is reported UNAVAILABLE, never fabricated |
| TEL-004 | Done | `model_tier_distribution` (`internal/runmetrics/decision.go`) |
| TEL-005 | Done | `decision_override_rate`, `decision_escalation_rate` |
| TEL-006 | Partial — library only | `Aggregate.Decisions` + `RenderText`; no CLI command (unwired guard) |
| TEL-007 | Done | `internal/runmetrics/decision_test.go`; gofmt/vet/build/race green |
