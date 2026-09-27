# T050 --- Explicit Retry, and Validation for No-Change Completions

> Add a safe `sop retry` for `BLOCKED` tasks, and make a
> `changes_expected=false` completion run configured validation before PASS.

## Status

DONE

## Objective

1. **`sop retry <task-id>`** — the explicit, safe counterpart to the automatic
   requeue on a `needs_human` outcome: requeue a `BLOCKED` task to `PLANNED` so
   the next `sop run` retries it.
2. **No-change completions still validate** — a `completed` + `changes_expected:
   false` outcome with no repository change must run the configured validation
   (build/test/lint) before it can pass; review is skipped because there is
   nothing to review.

## Dependencies

- T047 (structured outcomes), T048 (requeue on needs_human), T010 (`git`)

## Scope

- `internal/cli/retry.go`: `runRetry` — refuses a completed task, acts only on a
  `BLOCKED` task, stages the change on a copy and saves once.
- `internal/cli/cli.go`: `sop retry` dispatch and help.
- `internal/cli/run.go`: drop the early PASS for a no-change completion (it now
  falls through to validation); review only runs when there is a diff.
- Tests and docs.

## Rules

- `sop retry` only requeues a `BLOCKED` task; a completed task
  (`DONE`/`LOCAL_DONE`/`MERGED`) or an already-runnable task is refused with a
  clear message. History is preserved, so the retry resumes the same task.
- A `changes_expected=false` completion and a `changed`-but-none failure are
  distinct: the former proceeds (through validation) and the latter fails.
- Deterministic validation is never skipped for a completion that claims success.

## Tests

CLI: `sop retry` requeues a `BLOCKED` task to `PLANNED`; refuses `DONE`, a
non-`BLOCKED` task, and an unknown id; missing argument is a usage error. A
no-change completion (`changes_expected=false`, empty diff) PASSes **and** runs
the configured validation (a marker file proves it). Verified end-to-end with a
real binary.

## Acceptance Criteria

- [x] `sop retry <task-id>` requeues a `BLOCKED` task; other states are refused.
- [x] A `changes_expected=false` completion runs configured validation before PASS.
- [x] Review is skipped when there is nothing to review.
- [x] `make check` passes.

## Git

Branch: `task/T050-explicit-retry-and-no-change-validation`
Commit: `task(T050): add explicit retry and validate no-change completions`
PR: `[Task T050] Explicit retry and no-change validation`

## Out of Scope

Bounding how many times a task may be requeued; a bulk `sop retry --all`.
