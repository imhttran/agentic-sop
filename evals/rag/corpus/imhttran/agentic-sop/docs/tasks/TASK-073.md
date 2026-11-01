# T073 --- Raise IMPLEMENT's Iteration Ceiling

> The observed failure mode is a model that explores for most of the budget and
> writes only once the tools are withdrawn; give that pattern room to finish.

## Status

DONE

## Objective

Dogfooding AHV2011/AHV2012 showed a stable pattern across two models: the model
reads for ~22 of the 24 turns, writes only after reads are withdrawn (the
late-stage finalize), and then runs out of turns one or two later:

```text
IMPLEMENT DISCOVER #1-22 → FINALIZE → create_file (last turn) → iteration_limit
```

Both models diagnosed it the same way ("extend the invocation budget to complete
the implementation"). The loop had no way to finish an explore-then-write run.

## Scope

- `internal/ollamaagent/policy.go`: `maxIterationsImplement` 24 → 32, with a comment
  explaining the explore-then-write cause. `maxIterationsFix` stays 24.
- `internal/ollamaagent/harness_test.go`: the IMPLEMENT budget case updated.
- `README.md`, `docs/PLAN.md`.

## Rules

- **A ceiling, not a target.** 32 is the hard safety bound; a run still finalizes
  early (mutated, stopped writing) or is force-finalized at the late stage.
- **Nothing else moves.** The nudge (6), implement-now (12), mutated finalize floor
  (18), late-stage finalize (22), completion window (2), and finalize allowance (2)
  are unchanged; FIX stays at 24.
- **The diagnostic is unchanged.** A run that still never concludes ends with the
  same truthful `iteration_limit` / `no_change` classifications.

## Tests

`TestCapabilityBudgets` updated (IMPLEMENT 32). The phase tests use
`maxIterationsImplement` symbolically, so they keep exercising the ceiling and the
late-stage finalize without change.

## Acceptance Criteria

- [x] IMPLEMENT's ceiling is 32; FIX stays 24.
- [x] An unmutated run is still finalized at the late stage and ends retryable.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T073): raise IMPLEMENT's iteration ceiling to 32`

## Out of Scope

Raising FIX's ceiling; changing the phase thresholds; splitting large tasks.
