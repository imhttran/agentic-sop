# Decision Telemetry Specification

**Type:** Normative specification

## Status

In progress. The implementation is being carried out on the
`feat/decision-telemetry` branch under
[PLAN-Decision-Telemetry-Extension.md](../plans/PLAN-Decision-Telemetry-Extension.md)
(TEL-001–TEL-007), which refines the exact metric set and their
numerator/denominator/coverage in
[TEL-001-reconciliation.md](../reports/decision-telemetry/TEL-001-reconciliation.md).
This specification remains the normative boundary contract; the plan and register
are its non-normative implementation detail.

## Purpose

SOP makes several bounded decisions during a governed run — which model class a
task routes to, whether an adaptation or a recovery escalation changed that class,
and how a human/autonomy gate was dispositioned. Today each decision is recorded
in its own place (the per-task `routing.json`, the run trace, the approval record),
but there is no single, aggregated view of _what SOP decided and where it ended up_
across a run.

Decision Telemetry is that diagnostic record: the aggregation of SOP's bounded
decisions and their resolved outcomes, reported alongside the existing
`internal/perf` metrics. It answers "what did SOP decide, from what evidence, and
how often" from evidence rather than guesswork, and it never changes a decision.

## Related Specifications

- [MODEL-ROUTING.md](MODEL-ROUTING.md) — the class abstraction, precedence, and the
  deterministic router whose per-task decision this specification records.
- [RECOVERY.md](RECOVERY.md) — bounded retries and escalation, whose from→to class
  choices this specification records.
- [HUMAN-APPROVAL.md](HUMAN-APPROVAL.md) — the human gates whose disposition this
  specification counts.
- [EXECUTION.md](EXECUTION.md) — the `sop run` lifecycle that produces the decisions.
- [PERFORMANCE.md](../reference/PERFORMANCE.md) — the single authoritative metrics
  representation (`internal/perf`) this specification extends.
- [SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) — the ownership model
  (JEV analyzes, SOP decides, the model runs bounded work).

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

## Definitions

**Decision** — a bounded choice SOP policy makes during a run: a model-routing
class resolution, an adaptive-routing adaptation, an availability-fallback
application, a recovery escalation, or a human/autonomy-gate disposition. A
decision is always produced by SOP policy, never by a model provider.

**Decision Telemetry** — the aggregated, diagnostic record of the decisions above
and their resolved outcomes, scoped to a task and rolled up to a run.

## Scope: the decisions recorded

Decision Telemetry MUST record, at minimum, the following decisions. Each is
already produced by existing code; this specification adds the aggregation, not a
new decision path.

1. **Model-routing class decision** (per task) — the resolved class
   (`small`/`medium`/`large`) and its `source`
   (`policy` | `manual_override` | `escalation`), matching the routing decision
   already persisted by `writeRoutingDecisionArtifact` in
   `internal/cli/routing.go`.
2. **Adaptive-routing adaptation** — when `adaptiveroute.Route` changes the
   deterministic baseline class, the baseline and adapted classes MUST both be
   recorded, so an adaptation is never confused with the baseline router's own
   output.
3. **Availability-fallback application** — when the local-first fallback supplies
   the executing model for a class, it MUST be recorded as an application of the
   class's configured fallback, leaving the routing _class_ unchanged. A `small`
   routing decision MUST NOT be recorded as `medium` merely because a `small`
   availability fallback executed.
4. **Recovery escalation** — when recovery escalates an attempt, the `from` and
   `to` classes and the attempt count MUST be recorded as escalation evidence,
   never as a router or operator decision.
5. **Gate disposition** — the outcome of a human/autonomy gate MUST be counted by
   disposition, so the rate of human-boundary decisions is observable.

## Where it is recorded

Decision Telemetry MUST extend `internal/perf`, the single representation of run
diagnostic data, and MUST NOT introduce a competing metrics store or a new
instrumentation architecture.

- Per task: a `decisions` object in `.agent-sdlc/runs/<task-id>/metrics.json`.
- Per run: the aggregated `decisions` object in `.agent-sdlc/runs/<plan-id>/metrics.json`
  and the `performance` field of the run's `report.json`.
- `sop report` SHOULD render a concise decision summary from the same aggregate,
  and MUST render nothing (or an explicit "not recorded") for a run that recorded
  no decisions or an older artifact that predates the field.

## Decision-adapter evidence (shadow evaluation)

SOP MUST NOT run a decision provider in production to gather telemetry. Adapter
comparisons are produced **offline and out of band** by
`sop-decision-adapters`, which records them in a versioned artifact that SOP reads
read-only.

- **Artifact.** `<run>/shadow-evidence.json`, schema version `1`. It carries one
  record per (case, source) with `case_id`, `source` (provenance), `comparable`,
  `outcome`, `primary`, `shadow`, `reference`, `confidence` (nullable), and
  `latency_ms`.
- **Producer.** `sop-decision-adapters` emits the artifact by replaying recorded
  observations through its existing shadow harness; it performs no model call in
  the replay path and is deterministic.
- **Consumer.** `internal/runmetrics` ingests the artifact read-only and emits the
  adapter dimensions (`adapter_invocation`, `adapter_agreement`, `adapter_confidence`,
  `adapter_latency`, `adapter_invalid_choice`, `adapter_indeterminate`,
  `adapter_timeout_failure`). The denominators mirror the harness: comparable
  observations (a `skipped_not_comparable` record is excluded), and agreement counts
  only comparisons that recorded a reference.
- **Unknown version.** An artifact at an unrecognized schema version is present but
  `UNAVAILABLE`, never interpreted. A missing artifact is `UNAVAILABLE` and
  contributes no observation.
- **Authority.** The evidence is diagnostic only: it MUST NOT be read by routing,
  approval, or execution, MUST NOT be inferred where unmeasured, and MUST NOT be
  treated as proof of correctness. A consumer is never wired into the primary path.

## Command surface

`sop report --decisions [--format text|json]` is the opt-in, read-only way to read
the aggregate. It MUST:

- take no run id and read only recorded artifacts under `.agent-sdlc/runs/`;
- make no model call, evaluation, retry, approval, or execution, and open no
  network connection;
- render deterministically — the human-readable summary, or the aggregate JSON;
- report `UNAVAILABLE` distinctly from a measured zero, and never infer a value.

The aggregator MUST stay off the production execution path: exactly one CLI file
(`report_decisions.go`) may import `internal/runmetrics`, and no run, execute,
routing, or approval file may. The seven RM-002 metrics and the decision dimensions
are unchanged in definition and order.

## Metric semantics (TEL-009)

- `decision_escalation_rate` measures **model-class escalation only**: a routing
  decision with source `escalation`, or an escalated retry recorded under `attempts/`
  (`attempt >= 2`). A human approval is NOT an escalation and is reported solely by
  `human_approval_frequency`.
- `decision_override_rate` reads a manual override from either provenance —
  `routing.json source == "manual_override"` or `model-selection.json source ==
"cli"` — over the runs that recorded a routing or selection source.
- `retry_efficiency` is a **rate** (a percentage of eligible runs), not a bare count.
- `model-selection.json` is versioned (`schema_version: 1`). A historical artifact with
  no version is the initial version; an unknown version is `UNAVAILABLE`, never
  interpreted.

## Invariants

These invariants are normative and MUST hold for any implementation.

- **Diagnostic only.** Decision Telemetry MUST NOT influence any decision,
  lifecycle transition, gate, approval, or artifact. Where telemetry and
  correctness conflict, correctness wins.
- **No second source of truth.** The in-memory decision remains authoritative and
  the persisted `routing.json` remains write-only diagnostic evidence. Telemetry
  MUST NOT be read back to drive, re-derive, or override any decision.
- **Extension, not a new subsystem.** The record MUST live in `internal/perf`.
  Introducing a parallel metrics store or a new instrumentation subsystem is
  prohibited.
- **Provider-neutral.** The record MUST contain no provider-specific logic and MUST
  be produced identically regardless of the configured provider or model.
- **No inference.** A value SOP did not measure MUST be reported as `UNAVAILABLE`
  and MUST NOT be estimated, derived from text size, or inferred from an unrelated
  signal.
- **Fail-soft.** A missing, older, or unparsable metrics artifact MUST NOT fail a
  run; the absence of decision telemetry is reported, not fabricated.

## Non-Goals

- Changing routing thresholds, escalation policy, or any decision rule. This
  specification observes decisions; it does not define them.
- Provider-reported input/output token counts, per-tool timings, or context sizes
  that the existing metrics contract does not record. These remain `UNAVAILABLE`.
- A user-facing analytics subsystem, dashboard, or cross-run persistent store
  beyond the existing run artifacts.

## Acceptance Criteria

The implementation satisfies this specification when:

- A governed run records per-task and per-run decision telemetry in
  `internal/perf`, covering routing source, resolved class, adaptation, fallback
  application, escalation, and gate disposition.
- Counts are exact and reproducible for a deterministic fixture; a run that makes
  no decision records no decision telemetry rather than a zero-inferred value.
- No decision, transition, gate, or approval changes as a result of recording
  telemetry, proven by the existing lifecycle and architecture tests remaining
  green.
- The record is provider-neutral and contains no provider-specific logic.
- `gofmt -l .` is empty and `go vet ./...`, `go test ./...`, and `go build ./...`
  pass.

## Open Questions

- Should the run-level aggregate be a pure reader over per-task artifacts (offline
  aggregation) or accumulated in-process by the run? The run-metrics design
  precedent (`PLAN-Run-Metrics-And-Observability.md`) separates a pure library from
  an opt-in command; this specification SHOULD follow the same separation.
- Should `sop report` grow a dedicated decision section, or fold the counts into
  the existing performance block? A dedicated section is preferred for auditability.
