# T028 --- Deterministic Quality Gate

> Implements plan `PLAN-wrapup.md` T024 (Quality Policy).

## Status

DONE

## Objective

Decide, deterministically, whether a task passes, fails, or needs a human:

```text
build / test / lint + unresolved findings + fix-loop budget + human flag
                          │
                          v
                 PASS | FAIL | NEEDS_HUMAN
```

Policy comes from configuration (`quality.require_tests`, `quality.fail_on`,
`quality.max_fix_cycles`); the verdict is computed in code, never by a model.

## Dependencies

- T025/T026 (configuration model and its use)

## Scope

- `internal/quality/quality.go`: `Decision`, `Result`, `Input`, `Evaluate`.
- `internal/quality/quality_test.go`: table-driven policy tests.

## Rules

- Deterministic only; no agent or network.
- Human-required takes precedence, so a requested approval is never masked by a
  pass or a fail.
- Build is always required; tests are required only when policy says so; lint is
  evaluated only when the caller marks it required.
- Only findings whose severity is named in `fail_on` block; an empty list blocks
  nothing. Severity matching reuses `review.Severity` (one authoritative enum).
- An exhausted fix loop turns a still-failing task into `NEEDS_HUMAN` instead of
  looping forever; an exhausted budget with nothing wrong still passes.
- Every result carries a reason.

## Tests

A table covers: all checks pass; build failure; test failure required vs not;
lint failure required vs not; blocking finding vs below-threshold finding; empty
`fail_on`; human-required overriding a pass; exhausted budget with a blocking
finding and with a failing check; exhausted budget with a clean pass.

## Acceptance Criteria

- [x] The gate returns `PASS`, `FAIL`, or `NEEDS_HUMAN` from the policy and inputs.
- [x] `require_tests`, `fail_on`, and `max_fix_cycles` drive the verdict.
- [x] A human requirement and an exhausted fix loop produce `NEEDS_HUMAN`.
- [x] Every result carries a reason; the gate is model-free.
- [x] `make check` passes.

## Git

Branch: `task/T028-quality-gate`
Commit: `task(T028): add deterministic quality gate`
PR: `[Task T028] Add deterministic quality gate`

## Out of Scope

Wiring the gate into a `run` lifecycle; changing the existing merge gate.
