# T023 --- End-to-End Dogfood

## Status

DONE

## Objective

Exercise the whole pipeline on a small project:

```text
PRD -> plan -> tasks -> parallel selection
    -> TDD/execution -> review -> commit -> PR -> CI -> remediation -> merge
    -> handoffs -> complete
```

The real external boundaries (model, Git, GitHub) are fakes here so the pipeline
runs deterministically in CI; the live run against GitHub is documented and
opt-in.

## Dependencies

- Stages 4–21 (the components being composed)

## Scope

- `testdata/dogfood/PRD.md`: a small project PRD meeting the stage criteria.
- `internal/e2e/dogfood_test.go`: an in-process end-to-end test that composes the
  real control-plane components (planner, taskbuilder, real SQLite store,
  scheduler/completion, merge gate, CI remediation, review, parallelism, handoff)
  with fakes only at the model/Git/GitHub boundaries.
- Document the opt-in live run.

## Rules

- No network or real model in automated tests.
- The dogfood fixture has at least five tasks, one dependency chain, and two
  parallelizable tasks; it includes unit and integration tests, Docker, and
  GitHub Actions in its plan.
- One intentional failure exercises the CI remediation loop.
- Only external boundaries are faked; the control plane is the real code.

## Tests

One end-to-end test covering: PRD → plan (≥5 stages, a chain, two independent
stages); tasks persisted to SQLite with the environment dependency wired; two
parallelizable tasks selected together; a review loop that passes; a CI failure
remediated then passing; a merge gate that merges only when conditions hold; the
completion loop driving every task to DONE; a handoff capsule persisted per task.

## Acceptance Criteria

- [x] A PRD produces a plan of at least five tasks with a dependency chain and two parallelizable tasks.
- [x] Tasks are persisted and dependency-aware selection is exercised.
- [x] Review, CI remediation, and the merge gate are exercised on the path.
- [x] The completion loop drives every task to DONE.
- [x] A handoff capsule is persisted per completed task.
- [x] The automated run uses no network or real model.
- [x] `make check` passes.

## Git

Branch: `task/T023-end-to-end-dogfood`
Commit: `task(T023): add end-to-end dogfood`
PR: `[Task T023] Add end-to-end dogfood`

## Out of Scope

A live GitHub run in CI (documented, opt-in); new orchestration components.
