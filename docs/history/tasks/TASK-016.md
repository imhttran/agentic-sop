# T016 --- CI Remediation Loop

## Status

DONE

## Objective

Fix ordinary CI failures automatically, within a bounded loop:

``` text
CI_FAIL
  ↓
collect failure
  ↓
classify actionable?
 ├─ no  → BLOCKED
 └─ yes → remediate (diagnose/fix/local test/commit/push)
          ↓
        CI again   (bounded)
```

The loop decides whether to remediate; the actual diagnose/fix/commit/push is an
explicit `Remediator` port. Classification is deterministic — an LLM never
decides whether a CI failure is a code defect.

## Dependencies

- T014 --- GitHub adapter (checks)
- T013 --- commit gate
- T010 --- TDD task runner (remediation work)

## Scope

Add `internal/ciremediation`:

- `Failure{Checks, Logs}`, `Outcome` (`PASS`/`BLOCKED`), `Result{Outcome, Attempts}`;
- `CI` port (`Checks`, `FailureLogs`), `Remediator` port (`Attempt`);
- deterministic `Classifier` with a default (a failure with logs is actionable);
- bounded `Loop.Run` that re-checks after each attempt.

## Rules

- Retries are bounded; exhaustion is `BLOCKED`, not an unbounded loop.
- A failure that is not actionable is `BLOCKED` without remediation.
- Each remediation attempt is an explicit, recorded action (owned by the
  remediator); the loop counts attempts.
- Infrastructure errors and cancellation propagate; cancellation never blocks.

## Tests

Use fake CI/remediator. Cover: no failing checks → PASS; failing but not
actionable → BLOCKED, no remediation; remediate then pass; bounded exhaustion →
BLOCKED with attempts == max; CI/remediator errors propagate; cancellation
propagates.

## Acceptance Criteria

- [x] CI failures are collected deterministically.
- [x] Non-actionable failures become BLOCKED without remediation.
- [x] Actionable failures are remediated and CI is re-checked.
- [x] Retries are bounded and attempts are counted/recorded.
- [x] Exhaustion is BLOCKED (not a bypass or unbounded loop).
- [x] Cancellation propagates and does not block.
- [x] `make check` passes.

## Git

Branch: `task/T016-ci-remediation`
Commit: `task(T016): add ci remediation loop`
PR: `[Task T016] Add CI remediation loop`

## Out of Scope

Merge gate (T017), completion loop (T018); the remediation implementation itself.
