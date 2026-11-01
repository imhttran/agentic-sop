# T021 --- Limited Parallelism

## Status

DONE

## Objective

Run dependency-independent tasks concurrently without exhausting the system or
corrupting Git state. Parallel tasks never share a working directory: each runs
in its own Git worktree.

```text
select up to max_parallel_tasks ready, mutually independent tasks
  ↓
per task: create isolated worktree (own branch)
  ↓
run concurrently (bounded)
  ↓
remove worktree
```

## Dependencies

- T006 --- scheduler (readiness)
- T007 --- Git adapter (branch lifecycle)

## Scope

Add `internal/parallel`:

- `Workspaces` port (`Create`/`Remove` an isolated directory per task) and a
  Git-worktree backed implementation;
- `Executor` port (`Execute(ctx, task, dir)`);
- deterministic `Select(tasks, max)` for dependency-independent runnable tasks;
- `Runner.Run` that provisions a worktree per task, runs up to `max` at once, and
  always removes the worktrees.

Add `AddWorktree`/`RemoveWorktree` to the Git adapter.

## Rules

- Concurrency is bounded by `max_parallel_tasks` (default 2 in wiring).
- Selected tasks are mutually independent: no selected task depends, directly or
  transitively, on another selected task.
- Every task runs in its own worktree; none share a directory.
- Selection is deterministic (ordered by task id).
- A worktree is removed even when its task fails.
- Per-task errors are collected, never merged into one another.

## Tests

`parallel`: `Select` returns at most `max`, excludes unmet and dependent tasks,
and is deterministic; `Runner` runs independent tasks concurrently (peak
concurrency observed == min(max, ready)), each with a distinct directory, and
removes every worktree on success and failure; executor errors are reported per
task; cancellation propagates and cleans up. Git: `AddWorktree`/`RemoveWorktree`
create and remove a linked worktree.

## Acceptance Criteria

- [x] Up to `max_parallel_tasks` independent tasks run concurrently.
- [x] Two dependency-independent tasks run without sharing a working directory.
- [x] Git state is not corrupted (each task uses its own worktree; all removed).
- [x] Tasks with incomplete dependencies or inter-dependencies are not co-selected.
- [x] Selection is deterministic.
- [x] `make check` passes.

## Git

Branch: `task/T021-limited-parallelism`
Commit: `task(T021): add limited parallelism`
PR: `[Task T021] Add limited parallelism`

## Out of Scope

Wiring parallel execution into the completion loop; distributed execution.
