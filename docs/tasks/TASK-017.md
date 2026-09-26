# T017 --- Merge Gate

## Status

DONE

## Objective

Merge only validated work. The gate evaluates the required evidence and merges
the pull request through an explicit `Merger` port; if any condition is unmet it
blocks and never calls merge, so repository protection is never bypassed.

```text
tests pass + review pass + checks pass + mergeable?
 ├─ no  → BLOCKED (reason recorded, no merge)
 └─ yes → merge
```

## Dependencies

- T014 --- GitHub adapter (PR state, checks, merge)
- T013 --- commit gate (task commit already validated)

## Scope

Add `internal/mergegate`:

- `Input{PR, Checks, TestsPassed, ReviewPassed}`, `Outcome` (`MERGED`/`BLOCKED`),
  `Reason`, `Result{Outcome, Reason}`;
- `Merger` port (`Merge(ctx, prNumber, method)`);
- `Gate.Merge(ctx, Input)` that checks local tests, review, GitHub checks
  (all passing; pending/failing/unknown blocks), and mergeability, then merges.

## Rules

- Every required condition is evaluated deterministically; a model never decides
  whether a merge is allowed.
- Any unmet condition returns `BLOCKED` with a reason and performs **no** merge.
- Pending checks block; only passing/skipping checks count as ready.
- A non-mergeable PR (`CONFLICTING`/`UNKNOWN`) is blocked, not force-merged.
- Errors from the merger and cancellation propagate.

## Tests

Use a fake merger. Cover: all conditions hold → MERGED (merger called once with
the configured method, default `squash`); tests fail / review fails / checks
fail / checks pending / checks unknown / not mergeable → BLOCKED with the right
reason and no merge; merger error propagates; cancellation propagates.

## Acceptance Criteria

- [x] Successful PR merges to `main` only when every required condition holds.
- [x] Any unmet condition becomes BLOCKED with a recorded reason.
- [x] A blocked gate never calls merge (protection is not bypassed).
- [x] Pending or failing checks and non-mergeable PRs are blocked.
- [x] Merger errors and cancellation propagate.
- [x] `make check` passes.

## Git

Branch: `task/T017-merge-gate`
Commit: `task(T017): add merge gate`
PR: `[Task T017] Add merge gate`

## Out of Scope

Completion loop (T018), resume/recovery (T019); the merger implementation itself
(the GitHub adapter owns the `gh pr merge` call).
