# T033 --- Bounded Fix Loop

> Implements plan `PLAN-JEV.md` T020–T023 and PRD §8.9, inside `sop run`.

## Status

DONE

## Objective

Turn blocking review findings into bounded remediation instead of an unbounded
agent loop:

```text
validate → review → gate
              │  blocking finding (severity ∈ quality.fail_on)
              ▼
            fix  →  re-detect changes → validate → review → gate
              (≤ quality.max_fix_cycles; exhaustion → NEEDS_HUMAN)
```

## Dependencies

- T028 (quality gate), T030 (validation), T031 (review), T032 (run lifecycle)

## Scope

- `internal/cli/run.go`: the validate/review/gate/fix loop, `fixContext`, and
  fix-cycle reporting (`fix-N.md`, report `.json`/`.md`).
- `internal/cli/cli_test.go`: capability-aware fix, sequenced reviews, loop tests.

## Rules

- Actionable findings are exactly the blocking ones (severity named in
  `quality.fail_on`) — the same rule the gate uses (`quality.BlockingFindings`).
- Validation runs before review and fails fast, so uncompilable changes never
  reach review or a fix.
- Each cycle re-runs validation (regression protection); a fix that removes all
  changes fails the run rather than passing on an empty diff.
- The loop is bounded by `quality.max_fix_cycles`; exhausting it yields
  `NEEDS_HUMAN`, never an unbounded loop.
- Still no commit, push, or merge; a passing run stops at the human gate.

## Tests

A blocking finding that clears after one fix passes with `fix cycles: 1/3` and a
`fix-1.md` artifact; a finding that never clears exhausts the budget and yields
`NEEDS_HUMAN` (`fix cycles: 3/3`); a persistently blocking finding yields
`NEEDS_HUMAN`; the existing run tests (pass, no-changes, validation failure,
missing file, bad args) still hold.

## Acceptance Criteria

- [x] Blocking findings are sent to a fix agent and re-validated and re-reviewed.
- [x] The loop is bounded by `quality.max_fix_cycles`; exhaustion yields `NEEDS_HUMAN`.
- [x] Validation is re-run every cycle (regression protection).
- [x] Fix cycles are recorded in the run artifacts and report.
- [x] No commit/push/merge is performed.
- [x] `make check` passes.

## Git

Branch: `task/T033-bounded-fix-loop`
Commit: `task(T033): add bounded fix loop`
PR: `[Task T033] Add bounded fix loop`

## Out of Scope

Full finding triage (ACTIONABLE/IGNORE/NEEDS_HUMAN classification beyond
blocking); plan/DAG-driven execution; parallel validation checks.
