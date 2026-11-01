# T051 --- Bound the Requeue Loop

> A requeue spends one attempt; the retry budget is `max_attempts` (default 3), so
> a `needs_human` loop cannot run forever.

## Status

DONE

## Objective

```text
needs_human → requeue (attempt 1, 2, 3)
needs_human → budget spent → BLOCKED (terminal)
```

A task that keeps hitting a human boundary ends in `BLOCKED` rather than
`PLANNED` forever.

## Dependencies

- T048 (requeue on needs_human), T050 (`sop retry`)

## Scope

- `internal/domain/task.go`: `Requeue` now spends one attempt
  (`AddAttempt`) and returns `ErrRetryExhausted` once `Attempt >= MaxAttempts`
  (when `MaxAttempts > 0`).
- `internal/cli/drive.go`: a `WaitingForHuman` result requeues; on
  `ErrRetryExhausted` it blocks with `RETRIES_EXHAUSTED` and reports it.
- `internal/cli/retry.go`: `sop retry` surfaces the exhausted budget with an
  actionable message.
- Tests and docs.

## Rules

- The budget reuses the existing retry policy (`max_attempts`, default 3 from
  `domain.DefaultRetryPolicy`) and the task's `Attempt`/`Attempts` history — no new
  field or schema change.
- A refused requeue leaves the task untouched (status and attempt count
  unchanged); the check runs before the attempt is recorded.
- A completed task still cannot be requeued; the completed check precedes the
  budget check.

## Tests

domain: `Requeue` at `Attempt == MaxAttempts` returns `ErrRetryExhausted` and does
not mutate the task. CLI: `sop retry` on an exhausted task errors with “retry
budget exhausted”; driving a `needs_human` task three times requeues it, and the
fourth run blocks it (`BLOCKED`).

## Acceptance Criteria

- [x] A requeue spends one attempt against `max_attempts`.
- [x] Once the budget is spent, a `needs_human` task blocks instead of requeuing.
- [x] `sop retry` reports an exhausted budget with guidance.
- [x] A refused requeue does not mutate the task.
- [x] `make check` passes.

## Git

Branch: `task/T051-bound-requeue-loop`
Commit: `task(T051): bound the requeue loop by max_attempts`
PR: `[Task T051] Bound the requeue loop by max_attempts`

## Out of Scope

Configuring the bound separately from `max_attempts`; backoff between requeues.
