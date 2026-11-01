# T052 --- Don't Spend the Retry Budget on a No-Progress Retry

> A requeued `needs_human` task that reproduces the *same* outcome has not
> progressed; it stays `PLANNED` without spending an attempt, so a later
> `sop run` still retries it.

## Status

DONE

## Objective

```text
needs_human, outcome changed   → requeue, spend one attempt (bounded by max_attempts)
needs_human, outcome unchanged → requeue, spend nothing (still runnable)
needs_human, budget exhausted  → BLOCKED (terminal)
```

Bounding the requeue loop (T051) kept a `needs_human` task from looping forever,
but it spent an attempt on *every* retry — including retries that changed nothing
(for example a harness that keeps asking for the same authorization). A repeat
that reproduces the previous outcome is not progress and must not consume the
budget.

## Dependencies

- T048 (requeue on needs_human), T051 (bounded requeue)

## Scope

- `internal/domain/task.go`: `RequeueWithoutSpending` returns a task to `PLANNED`
  and clears the blocked reason without touching `Attempt`; the completed-task
  refusal is factored into a shared `canRequeue`.
- `internal/run/run.go`: `Run.ReadAttempt`/`RecordAttempt` persist the previous
  attempt's outcome signature to `attempt.txt`, which — unlike `state.json` — is
  not reset by `run.New`, so it survives across runs of the same task.
- `internal/cli/drive.go`: on a `WaitingForHuman` result, compare the outcome
  signature with the recorded one; an unchanged repeat requeues without spending
  and reports `NEEDS_HUMAN (no change since the previous attempt: <reason>)`.
- Tests and docs.

## Rules

- A retry that makes progress (the outcome signature changed) spends one attempt,
  bounded by `max_attempts`; once the budget is spent the task becomes `BLOCKED`.
- A repeat that reproduces the same outcome spends nothing and leaves the task
  `PLANNED`; the same `sop run …` keeps retrying it, so a boundary the user later
  resolves is picked up automatically.
- The no-progress case still stops the current run (non-zero exit) — it does not
  loop within one invocation.
- Change detection is by outcome signature (`Decision|reasons`), not filesystem
  time; the completed-task refusal still applies.

## Tests

domain: `RequeueWithoutSpending` returns a `BLOCKED` task to `PLANNED`,
preserves the attempt count, clears the reason, and refuses a completed task.
CLI: a `needs_human` implementation that repeats the same outcome across runs
leaves the task `PLANNED` with `Attempts: 1` and prints the no-change message; a
sequence that changes the reason each run still exhausts the budget and blocks.
Verified end-to-end with a real binary.

## Acceptance Criteria

- [x] A no-progress retry does not spend the retry budget.
- [x] The task stays `PLANNED` and is retried by a later `sop run …`.
- [x] A progressing retry still spends the budget and can exhaust it to `BLOCKED`.
- [x] `make check` passes.

## Git

Branch: `task/T052-no-progress-retry`
Commit: `task(T052): skip the budget when a retry makes no progress`
PR: `[Task T052] Skip the budget when a retry makes no progress`

## Out of Scope

Backoff between requeues; a bulk `sop retry --all`; carrying the prior outcome
reason into the next attempt's context.
