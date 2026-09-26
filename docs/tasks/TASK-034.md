# T034 --- Dependency-Aware Graph Execution

> Implements the plan/DAG-driven execution step: `sop run` over the persisted
> task graph (scheduler + local lifecycle).

## Status

DONE

## Objective

Drive the persisted task graph, not just a single file:

```text
sop run            → scheduler selects the next READY task
                     → local lifecycle (plan → implement → validate → review → fix)
                     → gate PASS → task DONE   |   else → task BLOCKED
                     → repeat until no runnable work
sop run TASK.md    → one task file (unchanged)
```

## Dependencies

- Stage 5/6 (task DAG, scheduler), T032 (run lifecycle), T033 (fix loop)

## Scope

- `internal/cli/run.go`: the lifecycle is extracted into a reusable
  `executeLifecycle`/`runStages`; `sop run` with no argument branches to graph
  mode, and `sop run TASK.md` uses the same code path.
- `internal/cli/drive.go`: `runGraph`, `runScheduledTask`, `specFromTask`,
  `completeTask`, `blockTask`.
- `internal/cli/cli_test.go`: graph tests.

## Rules

- Selection is the existing scheduler: one ready task at a time, in dependency
  order; no model chooses what runs next.
- A task's spec comes from the domain task (ID, title, objective, acceptance
  criteria); acceptance criteria are split on the domain's newline delimiter.
- A passing gate completes the task; anything else blocks it with a reason. Both
  are persisted through the domain state machine on a staged copy (save once,
  then publish), so a failed save cannot leave in-memory state that was never
  stored.
- **Local completion:** there is no remote PR/CI/merge in this mode, so a passing
  lifecycle advances the task through the remaining legal transitions to `DONE`
  (synthesizing the remote states). This keeps the dependency rule — a dependency
  is complete only at `DONE` — intact and lets dependents unblock. This is stated
  in the command output and docs.
- The loop is bounded by the task count; nothing commits, pushes, or merges.

## Tests

Uninitialized project and empty task set error clearly; an all-DONE graph reports
all done; a single READY task is executed, gate-passes, and is persisted as
`DONE`; a gate-failing task is persisted as `BLOCKED`; bad args (two arguments)
is a usage error.

## Acceptance Criteria

- [x] `sop run` with no argument drives the persisted graph in dependency order.
- [x] The scheduler selects the next ready task; a model never does.
- [x] A passing gate marks the task `DONE`; otherwise it is `BLOCKED`.
- [x] Task state changes are staged and persisted atomically; the loop is bounded.
- [x] `sop run TASK.md` is unchanged and reuses the same lifecycle.
- [x] `make check` passes.

## Git

Branch: `task/T034-graph-execution`
Commit: `task(T034): add dependency-aware graph execution`
PR: `[Task T034] Add dependency-aware graph execution`

## Out of Scope

True remote completion (PR/CI/merge) in graph mode; parallel graph execution.
