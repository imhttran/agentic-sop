# T010 --- TDD Task Runner

## Status

DONE

## Objective

Implement SOP's first local end-to-end task execution loop.

T010 composes the deterministic boundaries already built in T001--T009:

``` text
Task state
   ↓
Git adapter
   ↓
Agent capabilities
   ↓
Test runner
   ↓
Persisted state
```

A single `READY` task should be able to progress through:

``` text
READY
  ↓
BRANCH_CREATED
  ↓
TESTS_WRITTEN
  ↓
RED_VERIFIED
  ↓
IMPLEMENTING
  ↓
LOCAL_TESTS_PASS
```

or terminate safely as:

``` text
BLOCKED
```

after bounded unsuccessful attempts.

The task runner owns orchestration. Git, agents, and verification remain
adapters that report facts or perform narrowly authorized operations.

------------------------------------------------------------------------

## Dependencies

-   T001A --- Workflow state machine
-   T002/T002A/T002B --- durable task persistence
-   T006/T007A --- deterministic scheduler and persistence correction
-   T007 --- Git adapter
-   T008 --- configurable test runner
-   T009 --- agent capability boundary

Do not add Stage 11 review behavior in this task.

------------------------------------------------------------------------

## Core Rule

``` text
Agents do the work.
SOP controls the process.
```

The TDD runner decides:

-   which legal step runs next;
-   which agent capability may be invoked;
-   when tests run;
-   whether RED was actually observed;
-   whether GREEN was actually observed;
-   when state may transition;
-   when an attempt is recorded;
-   when retry limits are exhausted;
-   when a task becomes `BLOCKED`.

The agent must not make any of those workflow decisions.

------------------------------------------------------------------------

## Package Boundary

Add a small orchestration package, preferably:

``` text
internal/taskrunner/
```

Suggested shape:

``` text
internal/taskrunner/
├── runner.go
├── ports.go
├── prompts.go
└── runner_test.go
```

Avoid putting orchestration in:

``` text
internal/agent
internal/git
internal/testrunner
internal/store
```

Those packages are adapters/domain infrastructure, not the workflow
coordinator.

------------------------------------------------------------------------

## V1 Scope

T010 runs exactly one already-`READY` task.

It does not select the next task. T006 owns scheduling.

Conceptually:

``` go
Run(ctx, taskID)
```

or:

``` go
Run(ctx, task *domain.Task)
```

The runner should load/persist authoritative task state through a narrow
store interface rather than trusting stale caller state.

V1 remains sequential.

------------------------------------------------------------------------

## Required Ports

Define only the smallest interfaces T010 needs.

### Task Store

Conceptually:

``` go
type TaskStore interface {
    Get(id string) (*domain.Task, error)
    Save(task *domain.Task) error
}
```

Reuse the existing store contract where practical.

### Git

T010 needs only operations such as:

``` text
ValidateRepository
Status
CurrentBranch
CreateBranch
Diff
```

and deterministic:

``` text
TaskBranchName(task.ID, task.Title)
```

Do not expose the entire Git implementation merely because it exists.

### Agent

Depend on:

``` go
agent.Agent
```

using capabilities:

``` text
DESIGN_TESTS
IMPLEMENT
DIAGNOSE_FAILURE
FIX
```

`REVIEW` belongs to T011.

### Verification

Depend on a small verification interface backed by T008.

Conceptually:

``` go
Run(ctx, check) Result
RunAll(ctx, checks) SuiteResult
```

The task runner must consume T008's deterministic status. It must not
ask an LLM whether a test passed.

------------------------------------------------------------------------

## Important V1 Agent Constraint

The current T009 `Agent` contract is content-generation only and
explicitly does not own filesystem mutation.

Therefore T010 must not silently assume:

``` text
agent.Generate(...)
    ↓
repository magically changed
```

T010 needs an explicit, deterministic way to apply agent-produced work.

For V1, choose one small controlled contract and document it.

Preferred approach: introduce a task-runner-owned workspace edit/apply
boundary, for example:

``` go
type WorkApplier interface {
    Apply(ctx context.Context, work agent.Response) error
}
```

or an equivalent narrow abstraction.

The generic `agent.Agent` remains unchanged.

Tests can use a fake applier that makes deterministic file changes.

A real command-backed coding harness that directly edits the repository
is a future adapter unless it can be introduced without violating T009's
contract.

Do not parse arbitrary prose and guess file edits.

Do not let model output become a shell command.

This boundary is required so T010's tests can prove that test-writing
and implementation steps actually changed the working tree.

------------------------------------------------------------------------

## Configuration

T010 needs the project's configured verification commands from T008.

At minimum identify:

``` text
the focused/required test command used for RED/GREEN
the affected/full local suite used before LOCAL_TESTS_PASS
```

Do not infer Go commands.

A small configuration is acceptable:

``` go
type Config struct {
    TestCheck   testrunner.Check
    FinalChecks []testrunner.Check
}
```

Exact names may differ.

`TestCheck` must be configured for TDD execution.

`FinalChecks` may be the configured T008 suite:

``` text
BUILD
UNIT_TEST
INTEGRATION_TEST
LINT
DOCKER_BUILD
```

according to the project.

Do not create a large configuration framework.

------------------------------------------------------------------------

# Workflow

## 1. Load and Validate Task

Load the task from durable state.

Required initial status:

``` text
READY
```

Anything else returns an error without mutation.

Validate repository and required configuration before mutating workflow
state.

Do not create a branch if no usable TDD test command is configured.

------------------------------------------------------------------------

## 2. Create Task Branch

Generate:

``` text
task/<id>-<slug>
```

using the T007 deterministic helper.

Require a clean working tree.

Create the branch.

Only after branch creation succeeds may the task transition:

``` text
READY → BRANCH_CREATED
```

Persist the transition.

If branch creation fails:

``` text
task remains READY
```

Do not claim `BRANCH_CREATED`.

### Persistence Failure After Git Mutation

Git branch creation is an external side effect and cannot be rolled back
safely with a destructive reset.

If:

``` text
CreateBranch succeeds
Save(BRANCH_CREATED) fails
```

return an explicit consistency error.

Do not automatically delete/reset the branch.

T019 will provide broader resume/recovery behavior. T010 should make
this failure visible and safe rather than pretending atomicity across
Git + SQLite.

------------------------------------------------------------------------

## 3. Design and Write Tests

Invoke:

``` text
Capability: DESIGN_TESTS
```

with bounded context containing at least:

``` text
task ID/title
objective
acceptance criteria
relevant project/test context supplied by caller
```

The response should describe the intended tests/work.

Pass the response to the controlled work-apply boundary.

Only after the test work is successfully applied may the task
transition:

``` text
BRANCH_CREATED → TESTS_WRITTEN
```

Persist it.

A successful model response alone is not enough to claim tests were
written.

------------------------------------------------------------------------

## 4. Verify RED

Run the configured focused test check using T008.

Required RED evidence:

``` text
testrunner.FAIL
```

A normal non-zero test exit is RED.

The following are not valid RED:

``` text
ERROR
CANCELED
PASS
```

### PASS During RED

If the newly written test command returns:

``` text
PASS
```

do not transition to `RED_VERIFIED`.

This means SOP has not proven the new test catches missing behavior.

For V1, treat this as an unsuccessful TDD attempt requiring correction.

Use bounded remediation rather than faking RED.

### ERROR/CANCELED During RED

Infrastructure error or cancellation is not a meaningful failing test.

Do not call it RED.

Cancellation should return promptly without consuming workflow retries
unless the existing attempt policy clearly requires otherwise.

Infrastructure failure should be surfaced distinctly; do not ask an LLM
to reinterpret it as a test failure.

### Valid RED

Only:

``` text
FAIL
```

allows:

``` text
TESTS_WRITTEN → RED_VERIFIED
```

Persist immediately.

------------------------------------------------------------------------

## 5. Begin Implementation

After persisted `RED_VERIFIED`, transition:

``` text
RED_VERIFIED → IMPLEMENTING
```

Persist before invoking implementation work.

Invoke:

``` text
Capability: IMPLEMENT
```

with bounded context including:

``` text
task objective
acceptance criteria
test expectations/design
RED verification result
relevant project context
```

Apply the response through the controlled work boundary.

The agent does not transition state.

------------------------------------------------------------------------

## 6. Verify GREEN

Run the configured focused test again.

Expected:

``` text
PASS
```

If it passes, continue to the final affected suite.

If it returns:

``` text
FAIL
```

enter bounded diagnose/fix behavior.

If it returns:

``` text
ERROR
CANCELED
```

do not classify it as an implementation defect automatically.

------------------------------------------------------------------------

## 7. Run Final Local Suite

After focused GREEN, run the configured required local checks through
T008.

Example:

``` text
BUILD
UNIT_TEST
INTEGRATION_TEST
LINT
DOCKER_BUILD
```

T008 owns deterministic ordering/fail-fast behavior.

Only a passing required suite allows:

``` text
IMPLEMENTING → LOCAL_TESTS_PASS
```

Persist immediately.

Do not transition on partial success.

------------------------------------------------------------------------

# Bounded Fix Loop

T010 owns a small local retry loop.

When implementation/final verification returns a normal verification
`FAIL`:

``` text
FAIL
 ↓
DIAGNOSE_FAILURE
 ↓
FIX
 ↓
apply work
 ↓
run focused test
 ↓
run final suite when focused test passes
```

Use the task's existing:

``` text
Attempt
MaxAttempts
Attempts
CanRetry()
AddAttempt(...)
```

rather than inventing an unrelated retry counter.

Each meaningful implementation/fix attempt must be recorded and
persisted.

The attempt record should capture useful bounded diagnostics such as:

``` text
status
reason
verification output
duration when available
timestamp
```

Do not persist enormous raw logs without limit. A small truncation
helper is acceptable.

------------------------------------------------------------------------

## Retry Exhaustion

When the configured maximum is exhausted without reaching a passing
final suite:

``` text
task.Block(...)
```

with a meaningful reason.

Examples:

``` text
focused tests still failing after 3 attempts
required local suite failed after 3 attempts
unable to produce valid RED after bounded test-design attempts
```

Persist:

``` text
BLOCKED
```

Do not leave the task indefinitely in `IMPLEMENTING`.

Do not bypass `MaxAttempts`.

------------------------------------------------------------------------

# RED Correction Loop

RED verification deserves separate treatment from implementation
failure.

If the designed tests unexpectedly pass:

``` text
TESTS_WRITTEN
      ↓
test PASS
      ↓
DESIGN_TESTS correction
      ↓
apply corrected test work
      ↓
test again
```

Bound this loop.

Do not use `IMPLEMENT` before `RED_VERIFIED`.

Do not change production code merely to force RED.

The simplest V1 policy may share the task attempt budget if clearly
documented, but it must remain bounded and persisted.

If no valid RED can be established within the budget, block the task.

------------------------------------------------------------------------

# State/Persistence Rule

For ordinary state changes use:

``` text
copy task
 ↓
transition copy
 ↓
Save(copy)
 ↓
replace in-memory state after successful persistence
```

This follows the T007A scheduler correction and prevents failed
persistence from leaving an in-memory state that was never durable.

A helper is encouraged:

``` go
transitionAndSave(task, next)
```

with staged-copy semantics.

The helper must preserve nested fields correctly.

Do not mutate the authoritative task first and then hope `Save`
succeeds.

------------------------------------------------------------------------

# Attempt Persistence Rule

The same principle applies to attempt updates.

Prefer:

``` text
copy
 ↓
AddAttempt(copy)
 ↓
Save(copy)
 ↓
publish copy
```

If persistence fails, the caller-visible authoritative task should not
claim the attempt was durably recorded.

------------------------------------------------------------------------

# Failure Classification

T010 must preserve distinctions already established by T008.

``` text
PASS      verification passed
FAIL      project/test/build/lint failure
ERROR     runner/infrastructure failure
CANCELED  context canceled/deadline
```

Only `FAIL` should normally enter the agent diagnose/fix loop.

Do not send `ERROR` to `DIAGNOSE_FAILURE` as though it were a code
defect unless a future explicit policy permits it.

Cancellation should propagate.

------------------------------------------------------------------------

# Agent Requests

Use the T009 capabilities deliberately.

## DESIGN_TESTS

Output requirements should be explicit and suitable for the configured
work applier.

## IMPLEMENT

Called only after durable `RED_VERIFIED`.

## DIAGNOSE_FAILURE

Include a bounded representation of:

``` text
category
command
exit code
stdout
stderr
current task
```

Do not let the agent change the T008 classification.

## FIX

Include:

``` text
diagnosis
failure evidence
task acceptance criteria
relevant context
```

Apply through the controlled work boundary.

## REVIEW

Do not use in T010.

T011 owns review.

------------------------------------------------------------------------

# Branch Safety

T010 must never use:

``` text
git reset --hard
git clean
git checkout -f
git branch -D
git push --force
git rebase
```

Do not discard user changes.

The working tree is expected to become dirty after tests/implementation
are written. That is normal.

Do not call T007 operations that require a clean tree after work begins
unless the operation is explicitly safe.

T010 does not commit; T013 owns the commit/documentation gate.

------------------------------------------------------------------------

# Cancellation

Honor `context.Context` across:

``` text
Git
Agent
WorkApplier
TestRunner
```

On cancellation:

-   stop the current operation;
-   do not invent a PASS/FAIL result;
-   do not perform later workflow steps;
-   persist only state that was already legitimately completed;
-   return a context-related error.

Do not mark the task `BLOCKED` merely because the user canceled SOP.

------------------------------------------------------------------------

# Idempotency / Re-entry

Full resume is T019, so T010 does not need arbitrary crash recovery.

However, avoid obviously destructive duplicate behavior.

For V1, `Run` should require:

``` text
READY
```

and reject a task already in:

``` text
BRANCH_CREATED
TESTS_WRITTEN
RED_VERIFIED
IMPLEMENTING
LOCAL_TESTS_PASS
```

with a clear error.

Do not silently create another branch or restart work.

------------------------------------------------------------------------

# Tests

Use fakes for Git, Agent, WorkApplier, TestRunner, and Store.

No real model, network, GitHub, or remote repository is required.

Use integration-style local tests only where they materially prove
composition.

At minimum cover:

## Happy Path

``` text
READY
→ branch created
→ tests designed/applied
→ TESTS_WRITTEN
→ focused FAIL
→ RED_VERIFIED
→ IMPLEMENTING
→ implementation applied
→ focused PASS
→ final suite PASS
→ LOCAL_TESTS_PASS
```

Verify persisted transitions occur in legal order.

## Wrong Initial State

Start at `PLANNED`, `BRANCH_CREATED`, etc.

Expected:

``` text
error
no adapter mutation
```

## Branch Failure

Git create fails.

Expected:

``` text
task remains READY
agent not invoked
tests not run
```

## Branch Persistence Failure

Branch creation succeeds but `Save(BRANCH_CREATED)` fails.

Expected:

``` text
explicit consistency error
no destructive rollback
later steps not invoked
```

## Test Design Failure

`DESIGN_TESTS` returns error.

Expected:

``` text
no TESTS_WRITTEN transition
implementation not invoked
```

## Apply-Test Failure

Agent returns content but work applier fails.

Expected:

``` text
no TESTS_WRITTEN
```

## Valid RED

Focused test returns `FAIL`.

Expected:

``` text
TESTS_WRITTEN → RED_VERIFIED
```

## False RED

Focused test returns `ERROR`.

Expected:

``` text
no RED_VERIFIED
no IMPLEMENT
```

## Tests Unexpectedly Green

Focused test returns `PASS` before implementation.

Expected:

``` text
no RED_VERIFIED
bounded DESIGN_TESTS correction
```

and eventually `BLOCKED` if RED cannot be established.

## Implementation Ordering

Verify `IMPLEMENT` is never invoked before durable `RED_VERIFIED`.

## Green Focused Test

After implementation, focused test `PASS`.

Expected final suite runs.

## Implementation Failure

Focused test `FAIL`.

Expected:

``` text
DIAGNOSE_FAILURE
FIX
apply
retest
```

within attempt limit.

## Final Suite Failure

Focused test passes but final suite fails.

Expected diagnose/fix loop, not `LOCAL_TESTS_PASS`.

## Retry Success

First implementation attempt fails; second succeeds.

Verify attempts are persisted and final state is `LOCAL_TESTS_PASS`.

## Retry Exhaustion

All allowed attempts fail.

Expected:

``` text
BLOCKED
meaningful BlockedReason
attempt count == configured maximum
```

## Cancellation

Cancel during agent or verification work.

Expected:

``` text
context-related error
no later operations
not BLOCKED solely due to cancellation
```

## Persistence Failure

For each critical transition, inject a save failure.

Verify caller-visible state is not advanced before persistence succeeds.

## No Review

Verify `agent.Review` is never requested.

------------------------------------------------------------------------

# Minimal Integration Test

Add one small integration-style test using:

``` text
temporary Git repository
fake agent
fake work applier
local shell verification commands
temporary SQLite store
```

The target project can be language-neutral, for example simple files
plus shell checks.

The test should prove the components compose without a real LLM.

Do not make the integration test depend on GitHub or network access.

------------------------------------------------------------------------

# Process Cancellation Follow-up

T008 and T009 currently use `exec.CommandContext(..., "sh", "-c", ...)`.

A previously identified follow-up is process-tree cancellation for
commands that spawn children.

Do not expand T010 into a process-management refactor unless needed for
T010 correctness.

If a shared process runner is introduced while implementing T010, keep
it focused and use it consistently for both T008/T009. Otherwise track
the process-tree cancellation correction separately.

T010 tests should not falsely claim full child-process cleanup if it has
not been implemented.

------------------------------------------------------------------------

# Acceptance Criteria

T010 is complete when:

1.  an authoritative persisted `READY` task can be executed locally;
2.  a deterministic task branch is created before test work;
3.  `BRANCH_CREATED` is persisted only after successful branch creation;
4.  `DESIGN_TESTS` is invoked through the T009 agent interface;
5.  agent output is applied through an explicit controlled work
    boundary;
6.  `TESTS_WRITTEN` is persisted only after test work is actually
    applied;
7.  the configured focused test is executed through T008;
8.  only a deterministic `FAIL` can establish RED;
9.  `PASS`, `ERROR`, and `CANCELED` cannot establish RED;
10. `RED_VERIFIED` is persisted before `IMPLEMENT` is invoked;
11. implementation work is applied through the controlled work boundary;
12. focused GREEN is verified through T008;
13. the required final local suite is executed;
14. `LOCAL_TESTS_PASS` is persisted only after required verification
    passes;
15. ordinary verification failures use bounded diagnose/fix attempts;
16. retry limits use the existing task attempt model;
17. exhausted attempts result in persisted `BLOCKED`;
18. cancellation does not automatically block the task;
19. persistence failures do not falsely advance caller-visible task
    state;
20. no review, PR, CI, merge, or commit behavior is added;
21. no LLM determines test pass/fail;
22. no provider-specific logic enters the task runner;
23. tests require no network or real model;
24. the repository test suite remains green.

------------------------------------------------------------------------

# Verification

Run:

``` bash
go test ./internal/taskrunner/...
go test ./internal/agent/...
go test ./internal/testrunner/...
go test ./internal/git/...
go test ./internal/store/...
go test ./internal/domain/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

------------------------------------------------------------------------

# Open Code Review

After local verification passes, run Open Code Review against the task
branch.

Review specifically for:

-   state transitions occurring before the external action they
    represent;
-   in-memory state advancing before persistence succeeds;
-   `IMPLEMENT` running before persisted `RED_VERIFIED`;
-   treating `PASS` as valid RED;
-   treating `ERROR` or `CANCELED` as RED;
-   sending infrastructure failures into the code-fix loop;
-   unbounded retry loops;
-   a second retry counter instead of the domain attempt model;
-   blocking on user cancellation;
-   agent output being executed as shell;
-   hidden filesystem mutation inside the generic agent contract;
-   destructive Git rollback;
-   `LOCAL_TESTS_PASS` before all required checks pass;
-   T011 review behavior leaking into T010;
-   oversized orchestration abstractions or policy frameworks.

Fix correctness, safety, state-consistency, and bounded-retry findings.

Decline style suggestions that add complexity without strengthening
workflow control.

------------------------------------------------------------------------

# Git

## Branch

``` text
task/T010-tdd-task-runner
```

## Commit

``` text
task(T010): add TDD task runner
```

## Pull Request

``` text
[Task T010] Add TDD task runner
```

------------------------------------------------------------------------

# Out of Scope

Do not add:

-   scheduler selection of the next task;
-   T011 Ponytail/self review;
-   T012 Open Code Review;
-   commit creation;
-   push;
-   pull requests;
-   GitHub API;
-   CI observation;
-   merge;
-   completion loop;
-   arbitrary crash resume;
-   parallel tasks;
-   worktrees;
-   language-specific test commands;
-   provider-specific model SDKs;
-   automatic shell execution of model output;
-   destructive Git cleanup;
-   production deployment.

------------------------------------------------------------------------

# Definition of Done

T010 is complete when SOP can take one persisted `READY` task through a
genuine, observable local TDD cycle:

``` text
branch
→ tests written
→ RED proven
→ implementation
→ GREEN proven
→ required local suite passed
```

with durable legal state transitions, bounded diagnose/fix attempts,
deterministic verification, explicit application of agent-produced work,
safe failure behavior, and no review/PR/CI responsibilities leaking into
the task runner.
