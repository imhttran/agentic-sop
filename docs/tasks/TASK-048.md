# T048 --- Requeue on NEEDS_HUMAN

> A `needs_human` outcome returns the task to `PLANNED` instead of terminal
> `BLOCKED`, so a later `sop run` retries it automatically.

## Status

DONE

## Objective

A human boundary is a pause, not a dead end:

```text
agent outcome needs_human  → run stops with NEEDS_HUMAN, task → PLANNED
later: same `sop run …`     → scheduler re-selects the task and retries it
```

Before this, any non-PASS result (including `NEEDS_HUMAN`) `Block()`ed the task,
and `BLOCKED` is terminal — so resolving the human boundary still left the task
stuck.

## Dependencies

- T047 (structured command-agent outcomes), T034 (graph execution)

## Scope

- `internal/domain/task.go`: `Task.Requeue()` — returns a non-completed task to
  `PLANNED` and clears the blocked reason (the inverse of `Block`); a completed
  task (`DONE`/`LOCAL_DONE`/`MERGED`) cannot be requeued.
- `internal/cli/drive.go`: `runScheduledTask` requeues on a `WaitingForHuman`
  result and only blocks on a failure; `requeueTask` mirrors `blockTask`.
- Tests and docs.

## Rules

- Only the human boundary requeues: a `NEEDS_HUMAN` result returns the task to
  `PLANNED` and the run stops with a non-zero exit. A `FAIL` — including a
  claimed change with none produced — still `Block()`s (terminal).
- The requeue is a documented domain operation like `Block`, not a state-machine
  `Transition`; it stages the change on a copy, saves once, then publishes, so a
  failed save cannot leave in-memory state that was never persisted.
- Completed tasks are never requeued, and attempts/history are preserved, so the
  retry resumes the same task rather than recreating it.

## Tests

domain: `Requeue` from `PLANNED`/`READY`/…/`BLOCKED` → `PLANNED` with an empty
blocked reason; refused from `DONE`/`LOCAL_DONE`/`MERGED`. CLI: a `needs_human`
implement in graph mode leaves the task `PLANNED` (not `BLOCKED`) and a second
`sop run` retries it (no “no runnable task”); a `failed` outcome still leaves the
task `BLOCKED`. Verified end-to-end with a real binary.

## Acceptance Criteria

- [x] A `needs_human` outcome requeues the task to `PLANNED`.
- [x] A later `sop run …` retries the requeued task automatically.
- [x] A `failed`/`FAIL` outcome still blocks the task (terminal).
- [x] A completed task cannot be requeued; history is preserved.
- [x] `make check` passes.

## Git

Branch: `task/T048-requeue-on-needs-human`
Commit: `task(T048): requeue on needs_human`
PR: `[Task T048] Requeue on needs_human`

## Out of Scope

A `sop retry <id>` command (no longer needed for this case); bounding how many
times a task may be requeued.
