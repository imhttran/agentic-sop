# S4V — Verify Promptfoo configuration, reporting, and normalization

## Status

VERIFY

## Objective

Run the S4 acceptance checks against the current implementation: configuration
mapping, normalized reporting, report-only behavior, pinned binary invocation,
and evidence links. This task changes no production behavior; it exists because
the S4 plan task is blocked in the model-driven implementation loop.

## Acceptance Criteria

- Config maps into the adapter and computes config/dataset hashes.
- Findings are distinct from execution errors; report-only does not block.
- Disabled evaluation is SKIPPED, not FAILED.
- The pinned Promptfoo binary runs through the adapter with a live local model.
- Evidence links resolve to the normalized report and raw JSON stays valid.

## Acceptance Checks

- go test ./internal/security/ ./internal/config/ -count=1
- go build ./...
- go vet ./...
- go test -tags=promptfoo_live ./internal/security -run TestPromptfooPinnedBinaryVersion -count=1

## Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 4
- criterion 5: check 1

## Execution

verify-first
