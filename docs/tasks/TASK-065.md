# T065 --- Enter the Fix Loop on a Failing Validation

> A failing check is actionable in its own right. The bounded repair loop must
> run for it, not only for blocking review findings.

## Status

DONE

## Objective

The local lifecycle is documented as `(validate → review → gate → fix)*`, but the
loop only entered FIX when review left blocking findings. Review is skipped when
validation fails, so a broken build or a failing test was actionable nowhere: the
task exited as a hard failure with `fix cycles: 0` and the agent was never asked
to repair it. This surfaced while dogfooding AHV2009, where an implementation that
failed `go test ./...` went straight to FAIL/BLOCKED with no repair attempt, even
with `max_fix_cycles: 3` configured.

## Scope

- `internal/cli/run.go`: the `runStages` loop now continues on a failing validation
  (not only blocking review findings); `fixContext` carries the deterministic
  validation failure to the fix; the FIX `OutputRequirements` names failing checks
  as well as findings.
- `internal/cli/cli_test.go`: the validation-failure tests are updated to the new
  behavior, plus a new test where the fix repairs a failing check.
- `README.md`, `docs/architecture/OVERVIEW.md`, `LESSONS.md`: the fix loop is described as
  triggered by a failing check as well as by blocking findings.
- `docs/PLAN.md`: Delivered gains `T065`.

## Rules

- **A failing check is actionable.** The loop continues while `gate == FAIL` and
  either the validation failed or blocking findings remain, bounded by
  `quality.max_fix_cycles`. Review is still skipped while validation fails.
- **The fix is given the failure.** FIX receives the plan, the deterministic
  validation failure (`validationFailureContext`), the blocking findings, and the
  current diff, so it starts from the actual failure.
- **Escalation is unchanged.** Exhausting the budget still yields `NEEDS_HUMAN`
  (`quality.Evaluate` already encoded this; the loop simply never fed it a check
  failure). In graph execution a `NEEDS_HUMAN` result requeues the task for a
  later run, bounded by `max_attempts`.
- **Safety is unchanged.** The gate is still deterministic, the fix loop is still
  bounded, and `sop run` still never commits, pushes, or merges.

## Tests

`TestRunValidationFailureEntersFixLoop` (an unrepairable check failure runs the
budget and escalates to `NEEDS_HUMAN`, `fix cycles: 3/3`),
`TestRunFixLoopRepairsFailingValidation` (a fix repairs the check: PASS,
`fix cycles: 1/3`), and `TestRunGraphValidationFailureRequeues` (the graph
requeues a `NEEDS_HUMAN` on a failing check rather than blocking). The existing
fix-loop, quality-gate, and graph tests are unchanged or updated accordingly.

## Acceptance Criteria

- [x] A failing validation enters the bounded fix loop.
- [x] The fix is handed the deterministic validation failure.
- [x] Exhausting the budget yields `NEEDS_HUMAN`, never an unbounded loop.
- [x] The graph requeues (rather than blocks) a task whose repair budget ran out.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T065): enter the fix loop on a failing validation`

## Out of Scope

Changing `quality.Evaluate` or its `NEEDS_HUMAN`-on-exhaustion policy; the
verification-first fast path; the harness; committing AHV2009's partial
implementation; committing or pushing the untracked plan documents.
