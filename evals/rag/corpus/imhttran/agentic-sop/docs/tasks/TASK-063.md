# T063 --- Mutation-Aware IMPLEMENT Finalization

> A threshold is not evidence that the work is done: do not withdraw IMPLEMENT's
> tools until it has actually changed the repository, so a productive
> implementation is never cut off mid-discovery.

## Status

DONE

> **Superseded in part.** A mutation-free run no longer reaches the hard ceiling: the
> repository no-progress guard stops it early with `termination=no_progress` /
> `IMPLEMENT_NO_PROGRESS` (see
> [AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) §9). The mutation-aware finalization
> described here is otherwise unchanged.

## Objective

Follow up T062. Forced finalization was working, but it fired too early: AHV2008
gathered enough context to know exactly what to change, reached the eighteen-
interaction threshold with no mutation yet, was forced into FINALIZE, and failed
with "No repository changes were made". Make finalization mutation-aware, so the
threshold only withdraws the tools once a repository change has been observed; a
threshold crossed without a change keeps the tools enabled and pushes the model to
implement.

## Scope

- `internal/ollamaagent/harness.go`: `implementState` gains `sinceMutation`,
  `implementInstructed`, and `finalInstructed`; `finalizeEligible` gates the
  FINALIZE transition on an observed mutation; the implement-now and late-stage
  final instructions; the `implementExhaustedError` diagnostic; the
  `CHANGE_CONTINUE` / `CHANGE_FINAL` trace decisions.
- `internal/ollamaagent/trace.go`: an optional `Detail` on a `TraceRecord`, so a
  decision event can carry a short reason.
- `internal/ollamaagent/harness_test.go`: the mutation-aware threshold regression,
  finalization exhaustion after a mutation, truthful non-completed outcomes, early
  mutation below the threshold, and updated forced-finalization fixtures.
- `README.md`: the IMPLEMENT bullet describes mutation-aware finalization.

## Rules

- **Finalization requires an observed mutation.** Crossing
  `implementFinalizeAfter` alone never withdraws the tools. The transition also
  requires `mutated`, plus either the completion window
  (`implementCompletionWindow`) or the late-stage point (`implementLateStageAfter`).
- **Keep the tools when nothing has changed.** Without a mutation the model keeps
  its controlled IMPLEMENT tools and is told to implement now; the permissions are
  neither broadened nor narrowed.
- **Truthful outcomes need no mutation.** `needs_human` and `failed` are accepted
  without a change; only *claiming* completion without a change is refused. No fake
  writes are encouraged.
- **One engine.** Phases, mutation, and instruction flags are invocation-scoped on
  `implementState`; nothing is persisted and SOP still sees one IMPLEMENT call. The
  hard ceiling (`maxIterationsImplement`, 24) is unchanged.
- **The agent does not own validation.** The implement-now and final instructions
  tell the model SOP runs build/test/lint/review after it returns.
- **Only IMPLEMENT changes.** PLAN keeps its discovery → synthesis phases; FIX,
  DESIGN_TESTS, DIAGNOSE_FAILURE, and REVIEW keep the generic loop.

## Tests

Deterministic, fake-Ollama tests: the AHV2008-shaped regression (threshold reached
with no mutation → not finalized, tools enabled, implement-now sent → mutation →
targeted follow-ups → FINALIZE → outcome accepted); early mutation below the
threshold succeeding; a mutation-then-refuses model stopping with
`termination=finalization_limit` and `mutation_observed=true`; a no-mutation model
stopping at the hard ceiling with `termination=iteration_limit` and
`mutation_observed=false`; `needs_human`/`failed` accepted without a mutation; the
`CHANGE_CONTINUE` decision rendered in the trace; and the existing PLAN and
capability tests unchanged.

## Acceptance Criteria

- [x] A threshold crossed without a mutation does not enter FINALIZE or disable tools.
- [x] A successful mutation makes finalization eligible; failed/denied writes do not.
- [x] The implement-now instruction is injected once the threshold is passed unmutated.
- [x] The late-stage point gives one final instruction to implement or report truthfully.
- [x] A mutation-free run stops at the hard ceiling with `mutation_observed=false`.
- [x] `needs_human` and `failed` are accepted without a repository change.
- [x] Early completion and early mutation below the threshold still succeed.
- [x] The 24-iteration ceiling and every tool/command policy are unchanged.
- [x] PLAN, FIX, DESIGN_TESTS, REVIEW keep their existing behavior.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T063): mutation-aware IMPLEMENT finalization`

## Out of Scope

Redesigning FIX; raising or removing the iteration limit; changing PLAN, the SOP
lifecycle, `ACTIVE_TASK`, or named-plan behavior; touching persisted SOP state or
the blocked dogfood tasks; broadening tool permissions; committing or pushing.
