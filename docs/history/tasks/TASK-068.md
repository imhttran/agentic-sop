# T068 --- Bound the Unmutated Run: Force a Conclusion, Retry a No-Op

> A model that neither changes the repository nor returns an outcome must not burn
> the whole budget silently — force it to conclude, and treat "changed nothing" as
> retryable, not failed.

## Status

DONE

## Objective

The phased loop is mutation-aware: once a mutation is observed, finalization waits
for the writer to stop (T064). But an invocation that *never* changes the
repository had no forcing mechanism — `finalizeEligible` required a mutation, so the
loop only *instructed* (implement-now, then a final instruction) and a model that
ignored both ran all 24 turns and ended with `termination=iteration_limit`,
`mutation_observed=false`. That surfaced twice while dogfooding (AHV2009, AHV2002):
`failed` → task BLOCKED → a manual `sop retry` for what is really a flaky
non-writing sample.

## Scope

- `internal/ollamaagent/harness.go`: `finalizeEligible` also finalizes an unmutated
  run at the late stage; a `changeIncompleteError` and the `no_change` termination
  for a phased run that ends without changing anything; `Run` maps that error to a
  **`needs_human`** outcome so SOP requeues it; the redundant late-stage
  instruction branch and its field/constant are removed.
- `internal/ollamaagent/harness_test.go`: the no-change ceiling test updated, plus a
  test that `Run` surfaces it as `needs_human`.
- `README.md`, `docs/PLAN.md`: the IMPLEMENT/FIX bullet and the Delivered list.

## Rules

- **Force a conclusion.** An unmutated run is finalized at the late stage, so the
  model can no longer read its way to the ceiling. A write during FINALIZE is still
  honoured and resumes CHANGE (T064), so a genuinely late implementation is not
  blocked.
- **A no-op is retryable, not failed.** A phased run that ends without changing the
  repository returns `needs_human` (termination `no_change`) with a "a retry may
  succeed" note. SOP requeues it (bounded by `max_attempts`) instead of blocking.
- **Nothing else changes.** The bounds, tool policy, the mutation-aware finalize
  for a stopped writer, and the `finalization_limit` diagnostic for a *mutated* run
  that will not finalize are unchanged.

## Tests

`TestImplementWithoutMutationStopsAtHardCeiling`: an unmutated run is finalized at
the late stage and ends with `termination=no_change`.
`TestRunNoChangeCeilingReturnsNeedsHuman`: the same run surfaces to SOP as a
`needs_human` outcome. The existing mutated-path tests (stopped-writer finalize,
finalization_limit, early completion) are unchanged.

## Acceptance Criteria

- [x] An unmutated run is finalized at the late stage instead of running to the
      ceiling.
- [x] A phased run that never changes the repository returns a retryable
      `needs_human` outcome, not `failed`.
- [x] A still-writing (mutated) run keeps the mutation-aware finalize from T064.
- [x] The 24-iteration ceiling and every tool/command policy are unchanged.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T068): bound the unmutated run (force a conclusion, retry a no-op)`

## Out of Scope

Raising the ceiling; changing PLAN/REVIEW/DESIGN_TESTS/DIAGNOSE_FAILURE; the
provider/harness default conflict (backlogged); SOP's lifecycle or retry policy.
