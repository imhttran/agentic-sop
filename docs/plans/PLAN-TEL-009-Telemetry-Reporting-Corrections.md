# PLAN — TEL-009: Telemetry Reporting Corrections

**Status:** IMPLEMENTED (uncommitted). Reporting and evidence changes only; no
routing, JEV, approval, or execution behavior changed.

**Baseline:** the completed read-only audit across 219 runs under
`.agent-sdlc/runs/` (model selections: 74 recorded — `env` 66, `cli` 8; classes
`large` 72, `medium` 2; 145 runs without a recorded selection; `quality.jev` never
enabled; no `attempts/` records; no `shadow-evidence.json`).

## Corrections

### 1. `decision_escalation_rate` — model escalation only

Previously the numerator was `routing.json source == "escalation"` **OR** a
`NEEDS_HUMAN`/`approval.json` human approval — i.e. it duplicated
`human_approval_frequency` and mislabeled approvals as escalations (12/156). Now it
counts **model-class escalation only**: `routing.json source == "escalation"` or an
escalated retry under `attempts/` (`attempt >= 2`). The denominator is the runs that
recorded escalation-relevant evidence (a routing decision or attempt records); a run
with neither is `UNAVAILABLE`. Human approvals are reported solely by
`human_approval_frequency` (`internal/runmetrics/decision.go`,
`internal/runmetrics/runmetrics.go`).

### 2. `decision_override_rate` — reconciled provenance and documented denominator

A manual override is recorded in **two** places and the metric now reads both:
`routing.json source == "manual_override"` (the routing decision) and
`model-selection.json source == "cli"` (the model layer). This reconciles the corpus
mismatch (9 `manual_override` routing records vs 8 `cli` selections — e.g. a run whose
routing says `manual_override` but whose selection says `env`). The denominator is
documented in the metric note: runs that recorded a routing source or a selection
source.

### 3. `retry_efficiency` — rate, not a bare count

It previously reported a run **count** whose numerator equalled its denominator
(e.g. `67` over `67`) under a name that reads as a rate. It now uses the shared
`rate()` helper: a **percentage** (`%`) of eligible runs (those carrying retry
evidence) that recorded a retry signal (`internal/runmetrics/aggregate.go`).

### 4. Versioned model-selection evidence

`model-selection.json` now carries `schema_version: 1`
(`internal/run/model_selection.go`, written by `writeModelSelectionArtifact`). A
**historical** artifact with no `schema_version` is accepted as the initial version;
an **unknown** version is present but `UNAVAILABLE`, never interpreted
(`internal/runmetrics/runmetrics.go`). The producer fails closed on an unsupported
version. The persisted field set is unchanged apart from the added version, so
historical records stay readable.

### 5. Configured model usage vs observed router selections

Documented here: with `config.yaml` `agent.model = deepseek-v4.1-flash:cloud`, the
**default (unrouted) agent** runs the large-class cloud model, so the 145 runs without
a recorded selection still ran large. Automatic routing was **never observed** (0
runs with `source == "router"`); every recorded selection was explicit (`env` default
class / `--model-class`). SMALL/MEDIUM are reachable only via an explicit selection,
a configured default, or the router (which additionally requires JEV evidence before
selecting SMALL), and none were selected here.

## Files changed

- `internal/runmetrics/runmetrics.go` — version-aware model-selection read; structured
  `attempts/` evidence (escalation).
- `internal/runmetrics/decision.go` — corrected `decision_escalation_rate` and
  `decision_override_rate`; `sourceManualOverride`.
- `internal/runmetrics/aggregate.go` — `retry_efficiency` becomes a rate.
- `internal/run/model_selection.go` (new) + `internal/cli/run.go` — versioned
  model-selection artifact.
- Tests/fixtures: `internal/runmetrics/tel009_test.go`,
  `internal/run/model_selection_test.go`, new `testdata/{escalation,override,mselver}`;
  goldens regenerated.
- Docs: this plan; `docs/specs/DECISION-TELEMETRY.md`; `docs/README.md`.

## Validation

- `gofmt -l .` clean; `go build ./...`; `go vet ./...`.
- `go test ./internal/runmetrics/ ./internal/run/ ./internal/cli/ ./internal/archtest/`.

## Remaining gaps

- Agreement provenance split (adapter deterministic vs recorded SOP outcome) remains
  unavailable (unchanged from TEL-003).
- `decision_override_rate` coverage stays bounded by the runs that recorded provenance
  (the 145 silent runs cannot be evaluated and are `UNAVAILABLE`, not zero).
- `model_tier_distribution` percentages can sum below 100% when a routed run records
  an unknown class (unchanged).
