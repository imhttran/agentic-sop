# T020 --- Environment Bootstrap Tasks

## Status

DONE

## Objective

Handle projects whose plan needs an integration environment before feature work
can run. The planner may emit an environment stage; feature tasks then get an
implicit dependency on it, so they stay blocked until the bootstrap task is done.

```text
plan has an "environment" stage?
 ├─ no  → nothing to do
 └─ yes → every feature task depends on the environment task
          ↓
   feature tasks remain blocked until bootstrap is DONE
```

## Dependencies

- T004 --- planner (plan structure)
- T005 --- task DAG (task build + validation)

## Scope

- Add `planner.Stage.Kind` (`kind` in the JSON contract): `""`/`"feature"` or
  `"environment"`, validated deterministically.
- Add `internal/bootstrap` with `Apply(plan, tasks)` that adds the environment
  task as an implicit dependency of every other task and reports malformed plans.
- Call `bootstrap.Apply` from `taskbuilder.CreateTasksFromPlan` before DAG
  validation.

## Rules

- Detection is structural (`Kind == "environment"`), never model judgement.
- At most one environment stage; more than one is an error.
- An already-declared dependency is not duplicated.
- The bootstrap task never depends on itself; a resulting cycle is rejected by
  DAG validation.
- Only the implicit dependency edges are persisted; no schema change.

## Tests

`bootstrap`: adds the dependency to features (including transitively related
features); no environment stage is a no-op; an existing dependency is not
duplicated; multiple environment stages error; a missing environment task errors;
a self-referential environment ordering that creates a cycle is rejected.
`taskbuilder`: a plan with an environment stage yields tasks whose features
depend on it; `planner`: an unknown kind fails validation.

## Acceptance Criteria

- [x] The planner can mark an environment/bootstrap stage.
- [x] Feature tasks implicitly depend on the environment task.
- [x] Feature tasks remain blocked until bootstrap passes (DONE).
- [x] Malformed plans (multiple/missing environment tasks, cycles) are rejected.
- [x] `make check` passes.

## Git

Branch: `task/T020-environment-bootstrap`
Commit: `task(T020): add environment bootstrap tasks`
PR: `[Task T020] Add environment bootstrap tasks`

## Out of Scope

Parallelism (T021), dogfood (T023); generating the environment files themselves
(the bootstrap task is executed like any other task).
