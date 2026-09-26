# T013 --- Commit and Documentation Gate

## Status

DONE

## Objective

Produce a clean, task-scoped commit only after the required conditions hold:

``` text
required tests pass
required review passes
task documentation updated
        │
        ▼
   task commit  (task(<ID>): implement <title>)
```

The gate decides *whether* a commit is allowed; the Git adapter performs it.
The gate never force-commits and never bypasses hooks.

## Dependencies

- T007 --- Git adapter (`Commit`)
- T010/T011 --- tests + review produce the preconditions

## Scope

Add `internal/commitgate`:

- `Preconditions{TestsPassed, ReviewPassed, DocsUpdated}` with a deterministic
  `unmet()` list;
- `Message(taskID, title)` → `task(<ID>): implement <title>`;
- `Gate` over a `Committer` port (`Commit(ctx, message) error`).

All required conditions must hold before the commit is attempted; otherwise the
gate returns an error and performs no commit.

## Rules

- No commit when a required condition is unmet.
- Task ID and title must be non-empty.
- The commit message is deterministic.
- No push, amend, `--no-verify`, or history rewriting.

## Tests

Use a fake committer. Cover: all preconditions met → commit with the expected
message; each unmet precondition → error and no commit; blank ID/title → error;
deterministic message format.

## Acceptance Criteria

- [x] The gate blocks a commit when required tests/review/docs are unmet.
- [x] The commit message is `task(<ID>): implement <title>`.
- [x] The gate performs no commit when blocked.
- [x] Blank task ID/title are rejected.
- [x] The Git adapter still performs the actual commit.
- [x] `make check` passes.

## Git

Branch: `task/T013-commit-gate`
Commit: `task(T013): add commit and documentation gate`
PR: `[Task T013] Add commit and documentation gate`

## Out of Scope

Push, PR, CI, merge; staging policy beyond what the caller/ Git adapter does.
