# T075 --- Requeue a Task Whose Retry Budget Is Spent

> `sop retry` told the operator to "raise max_attempts", but nothing could.

## Status

DONE

## Objective

Dogfooding left several tasks `BLOCKED` with an exhausted retry budget (3/3), and
no way to requeue them: `sop retry` reported *"retry budget exhausted (3/3); raise
max_attempts or start fresh"*, but `max_attempts` is fixed at task creation
(`domain.DefaultRetryPolicy`) with no config or flag, so the only escape was to
hand-edit `state.db` or discard the task graph. The message was unactionable.

## Scope

- `internal/cli/retry.go`: `sop retry <task-id> | --all [--force]`. With `--force`,
  an exhausted budget is raised before the requeue.
- `internal/cli/cli_test.go`: `TestRunRetryForceRaisesBudget`.
- `README.md`, `docs/PLAN.md`.

## Rules

- **Explicit, not automatic.** The budget still bounds automatic requeues; only an
  explicit `--force` raises it. Nothing raises it silently.
- **A raise, not a bypass.** `--force` sets `max_attempts` to `attempt +
  DefaultRetryPolicy().MaxAttempts` and then requeues, so the limit still applies.
- **Only BLOCKED tasks.** The existing guards (never a completed task, only a
  `BLOCKED` one) are unchanged.

## Tests

`TestRunRetryForceRaisesBudget`: a `BLOCKED` task with `attempt == max_attempts`
requeues under `--force`, ends `PLANNED`, and its `max_attempts` is raised. The
existing retry tests (including the un-forced exhaustion failure) are unchanged.

## Acceptance Criteria

- [x] `sop retry <id> --force` requeues a budget-exhausted `BLOCKED` task.
- [x] `max_attempts` is raised rather than bypassed.
- [x] Without `--force`, an exhausted budget still fails with an actionable message.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T075): requeue a task whose retry budget is spent`

## Out of Scope

A config-level `max_attempts`; automatic requeue past the budget; resetting state.
