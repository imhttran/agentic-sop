# T018 --- Completion Loop

## Status

DONE

## Objective

Automatically continue through the plan. Each step selects the next runnable
task, drives it to CI, and completes validated work:

```text
select next (scheduler)
  ↓
execute (READY → CI_PASS)
  ↓
merge (merge gate)
  ↓
refresh main
  ↓
mark DONE
  ↓
repeat until ALL_DONE / BLOCKED
```

The loop is control-plane: it never lets a model decide whether work is complete.

## Dependencies

- T006 --- scheduler (readiness)
- T017 --- merge gate (merge)
- T010 --- TDD task runner (the execution step)

## Scope

Add `internal/completion`:

- `Outcome` (`PROGRESSED`/`WAITING`/`ALL_DONE`/`BLOCKED`), `Result{Outcome, TaskID, Completed}`;
- ports: `Store` (`List`/`Get`/`Save`), `Executor` (`Execute` one READY task to
  `CI_PASS` or block it), `Merger` (`Merge` a CI-passed task; reports whether the
  merge happened), `Refresher` (`Refresh` the integration branch);
- `Loop.Advance` (one task lifecycle) and `Loop.Run` (until terminal).

## Rules

- Readiness comes from the scheduler; the loop never guesses.
- A task is marked `DONE` only after a confirmed merge and a main refresh.
- A blocked execution or merge becomes `BLOCKED`; the loop never bypasses.
- Execution, merge, and refresh errors propagate; cancellation stops the loop.
- `Run` terminates: every `PROGRESSED` step completes exactly one task.

## Tests

Use fakes for store/executor/merger/refresher. Cover: single task completes to
`DONE`; a multi-task dependency DAG (A; B,C←A; D←B,C) progresses to `ALL_DONE`
with all tasks `DONE`; an executor-blocked task yields `BLOCKED`; a blocked merge
yields `BLOCKED` without marking `DONE` or refreshing; refresher/executor errors
propagate; empty plan is `ALL_DONE`; cancellation propagates.

## Acceptance Criteria

- [x] A small multi-task demo project can progress until all tasks are `DONE`.
- [x] `DONE` is only set after a confirmed merge and main refresh.
- [x] Blocked execution or merge yields `BLOCKED` (no bypass).
- [x] Readiness is delegated to the scheduler (dependency-aware selection).
- [x] Errors propagate and cancellation stops the loop.
- [x] `make check` passes.

## Git

Branch: `task/T018-completion-loop`
Commit: `task(T018): add completion loop`
PR: `[Task T018] Add completion loop`

## Out of Scope

Resume/recovery (T019), parallelism (T021); the concrete executor/merger/refresh
adapters (owned by the task runner, merge gate, and Git adapter).
