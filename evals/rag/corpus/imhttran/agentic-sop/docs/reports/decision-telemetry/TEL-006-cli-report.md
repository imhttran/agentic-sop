# TEL-006 — Decision Telemetry CLI (`sop report --decisions`)

**Status:** IMPLEMENTED — the opt-in, read-only `sop report --decisions` command is
wired to the existing `internal/runmetrics` aggregator. Nothing is committed or
pushed.

This report records the TEL-006 CLI integration in
[PLAN-Decision-Telemetry-Extension.md](../../plans/PLAN-Decision-Telemetry-Extension.md).
The boundary rules are normative in
[DECISION-TELEMETRY.md](../../specs/DECISION-TELEMETRY.md).

## 1. Commands

```bash
sop report --decisions                 # deterministic, human-readable text
sop report --decisions --format json   # deterministic aggregate JSON
```

- `--format text` (default) and `--format json` are both accepted, as is the
  `--format=json` form.
- `--decisions` is project-wide: it aggregates every run under `.agent-sdlc/runs/`
  and takes no run id. Passing a run id with `--decisions`, or an unknown
  `--format`, exits with a usage error (code 2).
- The existing per-run `sop report [run-id] [--detailed]` behavior is unchanged.

## 2. What it renders

The command reuses `internal/runmetrics.AggregateProject` and the aggregator's text
renderer (`RenderDecisions`); JSON is the deterministic `Aggregate`. It includes:

- **Model-tier distribution** — `small`/`medium`/`large` counts and percentages,
  with `source` (router/cli/…) and `fallback` breakdowns.
- **Provider/model** — recorded per selection; the distribution is over the recorded
  class (the artifact holds the resolved provider/model per run).
- **JEV** — `jev_invocation` where `metrics.json` recorded it.
- **Adapter** — `adapter_invocation`, `adapter_agreement`, `adapter_confidence`,
  `adapter_latency` (p50/p95), `adapter_invalid_choice`, `adapter_indeterminate`,
  `adapter_timeout_failure`.
- **Overrides and escalations** — `decision_override_rate`, `decision_escalation_rate`.
- **Explicit denominators and coverage** on every metric.

`UNAVAILABLE` (no recorded source) is always distinct from a measured `0`.

### Sample output

```text
Decision telemetry (corpus 1 runs)

model_tier_distribution: 1 runs (coverage 100.0%, 1/1)
  tier large: 1
  source router: 1
decision_override_rate: 0.0 % (coverage 100.0%, 0/1)
decision_escalation_rate: UNAVAILABLE (coverage 0.0%)
jev_invocation: UNAVAILABLE (coverage 0.0%)
adapter_invocation: 2 observations (coverage 100.0%, 2/2)
adapter_agreement: 50.0 % (coverage 100.0%, 1/2)
  source:clef agreement: 1
  source:clef disagreement: 0
  source:nimble agreement: 0
  source:nimble disagreement: 1
adapter_confidence: 50.0 % (coverage 100.0%, 1/2)
  confidence 0.8-1.0: 1
adapter_latency: 10 ms (coverage 100.0%, 2/2)
  latency p50_ms: 10
  latency p95_ms: 30
adapter_invalid_choice: 0.0 % (coverage 100.0%, 0/2)
adapter_indeterminate: 0.0 % (coverage 100.0%, 0/2)
adapter_timeout_failure: 0.0 % (coverage 100.0%, 0/2)
decision_confidence: UNAVAILABLE (coverage 0.0%)
```

## 3. Safety and read-only guarantees

- Reads existing `.agent-sdlc/runs/` artifacts only. No model call, evaluation,
  retry, approval, or execution; no network call; no new service, database, or
  dependency; nothing is written or mutated.
- The aggregator stays off the production execution path: it is imported by exactly
  one file, `internal/cli/report_decisions.go`. The unwired guard
  (`internal/runmetrics/unwired_test.go`) was narrowed to allowlist that file (and
  its test) and continues to forbid every other importer — including all other
  `internal/cli` files in the run/execute/routing/approval path.
- The seven RM metrics and the existing TEL dimensions are unchanged in definition
  and order; only the additive `decisions` section is exposed.

## 4. Files changed

- `internal/cli/report_decisions.go` (new) — `runReportDecisions`.
- `internal/cli/report.go` — parse `--decisions` / `--format`.
- `internal/runmetrics/decision.go` — `RenderDecisions`.
- `internal/runmetrics/unwired_test.go` — narrow the guards to the one approved entry point.
- `internal/cli/report_decisions_test.go` (new) — CLI tests.
- Goldens regenerated (adapter-latency note wording only).

## 5. Verification

All commands run and observed green:

- `gofmt -l .` clean; `go build ./...`; `go vet ./...`.
- `go test ./internal/runmetrics/ ./internal/archtest/`.
- `go test -race ./internal/runmetrics/`.
- `go test ./internal/cli/` (full CLI suite, including the new
  `TestReportDecisions*`).
- `go test ./...` (whole module) — all packages ok.
- Manual smoke: `sop report --decisions` and `--format json` on a fixture project.

## 6. Coverage limitations and remaining gaps

- **Agreement provenance split** remains unavailable: the evidence artifact does not
  label a reference as a deterministic expectation vs a recorded SOP outcome, so
  `adapter_agreement` is over all references.
- **Routing `decision_confidence`** stays UNAVAILABLE (routing confidence is not
  persisted).
- The **provider/model** breakdown is not emitted as a separate distribution in the
  decision section; provider/model are recorded per run in `model-selection.json`, and
  the tier/source/fallback distribution is what the CLI renders.
- `docs/reference/CLI.md` (the authoritative command/flag home) lists the `report`
  flags; it has been updated to include `--decisions`/`--format`.

## 7. Deviations from the plan

- The tier percentages are surfaced as counts plus coverage (the aggregate carries
  counts and denominators); the CLI does not compute a second percentage table
  beyond what the metrics already expose. No production behavior deviates.
