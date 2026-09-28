# T053 --- Explicit Retry: `--all` and Prior-Attempt Context

> `sop retry --all` requeues every retryable BLOCKED task at once, and a retried
> task is told why its previous attempt stopped instead of repeating the request.

## Status

DONE

## Objective

1. **`sop retry --all`** — the batch counterpart to `sop retry <task-id>`:
   requeue every `BLOCKED` task that still has budget, so recovery is one command.
   A task whose budget is spent is reported and left `BLOCKED`, never silently
   skipped.
2. **Prior-attempt context** — a retried task (automatic requeue or explicit
   `sop retry`) is given the previous attempt's outcome as context, so the agent
   can address the blocker rather than repeat the request that stopped it.

## Dependencies

- T050 (`sop retry`), T052 (no-progress requeue)

## Scope

- `internal/cli/retry.go`: `runRetry` accepts `<task-id>` or `--all`; `retryAll`
  requeues every retryable `BLOCKED` task; `requeuePersisted` is the shared
  stage-and-save.
- `internal/cli/drive.go`: `outcomeSignature` is rendered human-readably and is
  recorded on the `FAIL` path too, so a `sop retry` after a failure is given the
  real reason rather than a stale or missing note.
- `internal/cli/run.go`: the IMPLEMENT request carries a `# Previous attempt`
  section read from the recorded signature.
- Help text, tests, and docs.

## Rules

- `sop retry --all` only acts on `BLOCKED` tasks; a completed or already-runnable
  task is left untouched.
- The automatic requeue and `sop retry` are the same operation; `--all` is their
  batch form. Every requeue respects `max_attempts` (T051); an exhausted task is
  reported with guidance and left `BLOCKED`.
- The context is the recorded outcome, not a reconstruction: SOP never re-derives
  a reason from logs or prose.
- An attempt with no recorded predecessor carries no `# Previous attempt` note.

## Tests

CLI: `--all` requeues the requeueable `BLOCKED` tasks, leaves an exhausted one
`BLOCKED` with a message and an already-runnable one untouched, and reports
“no BLOCKED tasks” when there is nothing to do. A task that stops at a human
boundary and is retried receives a `# Previous attempt` section naming the prior
outcome, while the first attempt receives none. Verified end-to-end.

## Acceptance Criteria

- [x] `sop retry --all` requeues every retryable BLOCKED task.
- [x] An exhausted task is reported and left BLOCKED; a non-BLOCKED task is untouched.
- [x] A retried task is given the previous attempt's outcome as implement context.
- [x] `make check` passes.

## Git

Branch: `task/T053-explicit-retry-all-and-context`
Commit: `task(T053): add retry --all and prior-attempt context`
PR: `[Task T053] Retry --all and prior-attempt context`

## Out of Scope

Wall-clock backoff between requeues (the no-progress rule already bounds the
loop); a bulk unlock that ignores the retry budget.
