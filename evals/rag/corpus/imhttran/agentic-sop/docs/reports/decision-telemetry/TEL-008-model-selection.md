# TEL-008 — Model Routing Percentage Summary

**Status:** IMPLEMENTED — `sop report --decisions` now opens with a `Model Selection`
percentage summary. Nothing is committed or pushed.

Non-normative evidence; the boundary rules remain in
[DECISION-TELEMETRY.md](../../specs/DECISION-TELEMETRY.md).

## Change

The human-readable report (`runmetrics.RenderDecisions`) now renders:

```text
Model Selection
  SMALL        0  (0.0%)
  MEDIUM       0  (0.0%)
  LARGE        1  (100.0%)
  Total        1
  Missing      0  (no recorded selection)
```

- **Denominator.** Only runs with a valid recorded model selection — the
  denominator of the already-computed `model_tier_distribution` metric
  (`ValidMeasurements`). The block rereads no artifact and duplicates no
  aggregation; the counts come from that metric's `tier` breakdown.
- **Order.** Fixed SMALL, MEDIUM, LARGE.
- **Percentages.** One decimal place (`%.1f`).
- **Zero denominator.** Renders `UNAVAILABLE`, never `0%`.
- **Missing coverage.** Reported separately (`TotalRuns - ValidMeasurements`).
- **JSON unchanged.** This is a text-renderer change only; the aggregate JSON and the
  goldens are byte-identical.

## Files changed

- `internal/runmetrics/decision.go` — `renderModelSelection` + `tierCounts`, called
  from `RenderDecisions`.
- `internal/runmetrics/decision_test.go` — `TestRenderDecisionsModelSelection`,
  `TestRenderDecisionsModelSelectionUnavailable`.
- `internal/cli/report_decisions_test.go` — asserts the block in the CLI text.
- `docs/reference/CLI.md` — notes the `Model Selection` summary.
- `docs/plans/PLAN-Decision-Telemetry-Extension.md` — TEL-008 status row.

## Verification

- `gofmt -l .` clean; `go build ./...`; `go vet ./internal/runmetrics/ ./internal/cli/`.
- `go test ./internal/runmetrics/ ./internal/cli/` — pass.
- Goldens unchanged (JSON compatibility preserved).
- Manual smoke of `sop report --decisions`.

## Remaining gaps / deviations

- A routed run whose recorded class is unknown is excluded from the tier counts but
  still counts in the denominator, so the percentages may sum slightly below 100%.
  This mirrors the metric definition; no behavior deviates from the plan.
- Provider/model is still not broken out as its own histogram in the text (the
  source/fallback breakdowns remain in the `model_tier_distribution` metric lines).
