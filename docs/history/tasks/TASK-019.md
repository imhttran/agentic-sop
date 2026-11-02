# T019 --- Resume and Recovery

## Status

DONE

## Objective

Recover safely from process interruption. `Resolve` reconciles a task's persisted
status with the resources that actually exist (branch, pull request) and reports
the next legal action, correcting a lost state write without duplicating the
branch or PR:

```text
load task + observe branch/PR
  ↓
persisted status inconsistent with reality?
 ├─ yes → recover to a consistent status (no duplicate work)
 └─ no  → verify required resources exist
  ↓
next legal action
```

## Dependencies

- T007/T007A --- Git adapter (branch existence)
- T014 --- GitHub adapter (pull request)
- T010 --- TDD task runner (the flow being resumed)

## Scope

Add `internal/resume`:

- `Observation{BranchExists, PR}`, `Observer` port;
- `Action` vocabulary (CREATE_BRANCH … MERGE/FINISH/NONE);
- `Decision{TaskID, Action, Status, Recovered}`;
- `Resumer.Resolve` (one task) and `Resumer.ResolveActive` (the single in-flight
  task), plus `Active(tasks)`.

Wire `sop resume [id]` in the CLI: with no id it resumes the single in-flight
task; observation uses the Git and GitHub adapters behind the `Observer` port.

## Rules

- Recovery is deterministic and never duplicates a branch or PR: an existing
  branch for READY/PLANNED advances to BRANCH_CREATED; an existing PR for
  REVIEW_PASS advances to PR_OPEN.
- A recorded status that requires a missing resource (BRANCH_CREATED without a
  branch, PR_OPEN without a PR) is a consistency error, not a silent re-create.
- Recovery is persisted atomically (staged copy) so a failed save changes nothing.
- No model is consulted; only persisted state and observed resources.

## Tests

Cover the `resolve` table for every status; recovery for READY+existing-branch
and REVIEW_PASS+existing-PR (persisted, no duplication); consistency errors for
BRANCH_CREATED without branch and PR_OPEN without PR; `ResolveActive` for zero,
one, and many in-flight tasks; observer error and cancellation propagate. CLI:
`sop resume` prints the action and recovers with a fake observer.

## Acceptance Criteria

- [x] `sop resume` determines the next legal action for interrupted work.
- [x] An existing branch/PR is reused, never duplicated, with the recovered state persisted.
- [x] Missing required resources surface as a consistency error.
- [x] Recovery is atomic (a failed save leaves state unchanged).
- [x] Errors propagate and cancellation stops the resolution.
- [x] `make check` passes.

## Git

Branch: `task/T019-resume-recovery`
Commit: `task(T019): add resume and recovery`
PR: `[Task T019] Add resume and recovery`

## Out of Scope

Parallelism (T021), dogfood (T023); the concrete executor/merge adapters.
