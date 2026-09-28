# T072 --- Bring the "Implement Now" Nudge Forward

> A model that only starts writing when told to must be told early enough to
> finish within the budget.

## Status

DONE

## Objective

Retrying AHV2011 showed a near-miss: the model read for eighteen turns, wrote its
test files only _after_ the strong "implement now" instruction at interaction 18,
spent two turns verifying, and reached the twenty-four-turn ceiling one turn before
it could return its outcome:

```text
IMPLEMENT DISCOVER #1-18  ...  → CHANGE_CONTINUE (implement now)
IMPLEMENT CHANGE #19-22   create_file x3, write_file
IMPLEMENT CHANGE #23-24   go build, go test
IMPLEMENT → FINALIZE                    (no turns left)
→ iteration_limit, mutation_observed=true, tool_calls=24
```

The strong instruction is the effective start signal for this model, but it fired
at 75% of the budget (18 of 24), leaving no room to change and return.

## Scope

- `internal/ollamaagent/harness.go`: new `implementNowAfter` (12) for the unmutated
  "implement now" nudge, decoupled from `implementFinalizeAfter` (18), which stays
  the mutated finalize floor. The nudge case uses the new constant.
- `README.md`, `docs/PLAN.md`.

## Rules

- **One lever, one purpose.** `implementNowAfter` drives only the unmutated
  "implement now" instruction; `implementFinalizeAfter` keeps driving the mutated
  finalize floor, so a multi-file change is still not cut off.
- **The nudge is not a stop.** Reads stay available after it; it only tells the
  model to start changing the repository.
- **Nothing else moves.** The soft discovery nudge (6), the late-stage finalize
  (22), the two-turn finalize allowance, and the 24-turn ceiling are unchanged.

## Tests

The existing IMPLEMENT phase tests pass unchanged: `TestImplementThresholdWithoutMutationKeepsToolsAndPushesImplementation` (the instruction still reaches the
model), `TestImplementDiscoveryNudge`, and
`TestImplementWithoutMutationStopsAtHardCeiling` (still finalized at the late stage
and ending `termination=no_change`).

## Acceptance Criteria

- [x] An unmutated run is told to implement now around interaction 12.
- [x] The mutated finalize floor (18) and the late-stage finalize (22) are unchanged.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T072): bring the implement-now nudge forward`

## Out of Scope

Raising the ceiling; changing the soft nudge or the late-stage finalize; splitting
AHV2011's scope; committing a failed attempt's partial test files.
