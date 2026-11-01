# T006 --- Dependency-Aware Scheduler

## Status

DONE

## Objective

Implement the first deterministic scheduler for SOP.

The scheduler reads persisted tasks and their dependency graph,
determines which `PLANNED` tasks are eligible to run, and promotes the
next legal task to `READY`.

V1 runs **one task at a time**.

The scheduler must not use an LLM. Scheduling is workflow control and
therefore belongs to the deterministic application layer.

``` text
SQLite Tasks
    │
    ▼
 Scheduler
    │
    ├─ inspect task status
    ├─ inspect dependencies
    ├─ determine eligible tasks
    ├─ apply deterministic ordering
    │
    ▼
PLANNED → READY
```

------------------------------------------------------------------------

## Dependencies

-   T005 --- Plan → Tasks + Dependency DAG
-   T005A --- Rename Agentic SDLC to SOP

------------------------------------------------------------------------

## Design Rule

Preserve the project boundary:

``` text
Agents do the work.
SOP controls the process.
```

The scheduler is part of SOP's control plane.

It must not ask an agent or model which task should run next.

------------------------------------------------------------------------

## V1 Scope

V1 schedules one task at a time.

The scheduler should:

1.  load persisted tasks;
2.  inspect current workflow states;
3.  inspect dependency relationships;
4.  identify eligible `PLANNED` tasks;
5.  sort eligible tasks deterministically;
6.  select at most one task;
7.  transition the selected task from `PLANNED` to `READY`;
8.  persist the updated task;
9.  return a structured scheduling result.

Parallel scheduling is explicitly deferred to T021.

------------------------------------------------------------------------

## Eligibility

A task is eligible only when:

-   its current status is `PLANNED`;
-   every declared dependency exists;
-   every dependency satisfies the scheduler's dependency-completion
    policy;
-   transitioning the task to `READY` is legal according to the domain
    state machine.

The scheduler must use domain behavior such as:

``` go
task.ResolveDependencies(...)
task.CanTransitionTo(domain.READY)
task.Transition(domain.READY)
```

where appropriate rather than duplicating the workflow state machine.

------------------------------------------------------------------------

## Dependency Completion Policy

For V1, a downstream task should not start merely because an upstream
task has passed local tests or review.

The dependency should be considered complete only when its work is
integrated into the shared project history.

Use:

``` text
MERGED
DONE
```

as dependency-satisfying states for scheduling.

This avoids scheduling a dependent task against code that may still
exist only on another task branch.

Existing domain dependency semantics may currently accept earlier states
such as `LOCAL_TESTS_PASS`, `REVIEW_PASS`, `PR_OPEN`, or `CI_PASS`. T006
should align the scheduler/domain dependency rule with the integration
requirement above.

Do not create a second conflicting definition of dependency satisfaction
inside the scheduler.

------------------------------------------------------------------------

## Deterministic Ordering

When multiple tasks are eligible, scheduling must produce the same
result for the same persisted state.

For V1, order eligible tasks by task ID ascending unless the existing
task model contains an explicit persisted ordering field suitable for
this purpose.

Example:

``` text
T003 PLANNED
T001 PLANNED
T002 PLANNED
```

with all dependencies satisfied should select:

``` text
T001
```

Do not rely on SQLite row order, map iteration order, timestamps, or LLM
judgment.

Priority scheduling can be added later if the domain gains an explicit
priority field.

------------------------------------------------------------------------

## One Active Task

V1 runs one task at a time.

Before promoting a `PLANNED` task to `READY`, the scheduler must
determine whether another task is already active.

For V1, any task beyond `PLANNED` that has not reached a
terminal/integrated completion state should prevent the scheduler from
starting another task.

At minimum, a task already in `READY` or an execution/review/CI state
must prevent another task from being promoted.

This preserves sequential execution until controlled parallelism is
introduced in T021.

------------------------------------------------------------------------

## Scheduling Outcomes

Return explicit outcomes rather than treating every no-selection case as
an error.

The scheduler should distinguish at least:

``` text
READY_TASK
ACTIVE_TASK
WAITING_ON_DEPENDENCIES
ALL_DONE
BLOCKED
```

Suggested meaning:

### READY_TASK

A `PLANNED` task was legally promoted to `READY`.

### ACTIVE_TASK

A task is already in progress, so V1 does not schedule another task.

### WAITING_ON_DEPENDENCIES

Remaining `PLANNED` tasks exist, but none currently have all
dependencies satisfied.

### ALL_DONE

No runnable work remains because all tasks are complete.

### BLOCKED

The workflow cannot make progress because one or more relevant tasks are
terminally blocked.

The exact Go representation may be an enum-like string type plus a
result struct. Keep it small and domain-oriented.

------------------------------------------------------------------------

## Blocked Dependencies

Do not automatically transition a dependent task to `BLOCKED` merely
because one of its dependencies is blocked.

Example:

``` text
T001 BLOCKED
  ↓
T002 PLANNED
```

T002 should remain:

``` text
PLANNED
```

The scheduler may report a `BLOCKED` workflow outcome explaining that
progress cannot continue.

Terminal mutation of dependent tasks should require an explicit policy
rather than being an incidental scheduling side effect.

------------------------------------------------------------------------

## Missing Dependencies

A persisted task referencing a dependency that cannot be loaded is an
invalid workflow state.

The scheduler must not promote that task.

Return an error identifying the task and missing dependency.

Do not silently treat a missing dependency as complete or simply skip
it.

------------------------------------------------------------------------

## Proposed Package

Add:

``` text
internal/scheduler/
```

Suggested shape:

``` text
internal/scheduler/
├── scheduler.go
└── scheduler_test.go
```

Keep the scheduler independent of CLI formatting.

A reasonable boundary is:

``` go
type Scheduler struct {
    store TaskStore
}

func (s *Scheduler) Next(ctx context.Context) (Result, error)
```

The exact API may differ if a smaller interface fits the existing
repository design better.

Depend on the smallest persistence interface needed by the scheduler
rather than directly coupling scheduling logic to SQLite implementation
details.

------------------------------------------------------------------------

## Persistence

The scheduler must use the existing store abstraction and persisted task
state.

SQLite remains the source of truth.

Flow:

``` text
state.db
   │
   ▼
Load Tasks
   │
   ▼
Evaluate DAG + Status
   │
   ▼
Select Candidate
   │
   ▼
Task.Transition(READY)
   │
   ▼
Persist Task
```

Do not maintain scheduler state separately from task state.

------------------------------------------------------------------------

## Idempotency

Repeated scheduler calls against unchanged state must be safe.

Example:

``` text
call 1
T001 PLANNED → READY

call 2
T001 already READY
```

The second call must not promote T002 while V1 sequential execution is
active.

It should return `ACTIVE_TASK`.

Once T001 reaches the dependency-complete state, the next scheduler call
may select the next eligible task.

------------------------------------------------------------------------

## No LLM Boundary

Scheduler tests and runtime behavior must require:

-   no model;
-   no API key;
-   no network access;
-   no agent harness.

Scheduling must be completely deterministic.

------------------------------------------------------------------------

## CLI

Do not add `sop run` as part of T006.

The scheduler should first exist as an application component with
deterministic tests.

The future completion loop will drive it.

If a small diagnostic CLI entry point is useful during development, keep
it out of the public CLI unless separately justified.

------------------------------------------------------------------------

## Tests

Write tests before implementation.

At minimum cover:

### No Dependencies

``` text
T001 PLANNED
```

Expected:

``` text
T001 READY
```

### Linear Dependency

``` text
T001 DONE
  ↓
T002 PLANNED
```

Expected:

``` text
T002 READY
```

### Incomplete Dependency

``` text
T001 IMPLEMENTING
  ↓
T002 PLANNED
```

Expected:

``` text
T002 remains PLANNED
```

### Locally Passing But Not Integrated

``` text
T001 LOCAL_TESTS_PASS
  ↓
T002 PLANNED
```

Expected:

``` text
T002 remains PLANNED
```

### CI Pass But Not Merged

``` text
T001 CI_PASS
  ↓
T002 PLANNED
```

Expected:

``` text
T002 remains PLANNED
```

### Merged Dependency

``` text
T001 MERGED
  ↓
T002 PLANNED
```

Expected:

``` text
T002 READY
```

### Diamond Dependency

``` text
      T001 DONE
      /       \
     ▼         ▼
 T002 DONE   T003 DONE
      \       /
       ▼     ▼
        T004 PLANNED
```

Expected:

``` text
T004 READY
```

### Multiple Dependencies --- One Incomplete

``` text
T001 DONE ─────┐
               ▼
             T003 PLANNED
               ▲
T002 REVIEW ───┘
```

Expected:

``` text
T003 remains PLANNED
```

### Deterministic Ordering

Given multiple eligible tasks in arbitrary storage order:

``` text
T003 PLANNED
T001 PLANNED
T002 PLANNED
```

Expected selection:

``` text
T001
```

### Existing READY Task

``` text
T001 READY
T002 PLANNED
```

Expected:

``` text
ACTIVE_TASK
```

T002 remains `PLANNED`.

### Existing Active Task

``` text
T001 IMPLEMENTING
T002 PLANNED
```

Expected:

``` text
ACTIVE_TASK
```

### Blocked Dependency

``` text
T001 BLOCKED
  ↓
T002 PLANNED
```

Expected:

-   no task promoted;
-   T002 remains `PLANNED`;
-   scheduler reports `BLOCKED`.

### Missing Dependency

``` text
T002 depends on T999
```

where T999 does not exist.

Expected:

-   error;
-   no task promoted;
-   persisted state unchanged.

### All Done

``` text
T001 DONE
T002 DONE
```

Expected:

``` text
ALL_DONE
```

### Repeated Scheduling

After T001 is promoted to `READY`, call the scheduler again without
changing state.

Expected:

``` text
ACTIVE_TASK
```

No second task is promoted.

------------------------------------------------------------------------

## Atomicity

A failed scheduling attempt must not leave a partially updated workflow.

If persistence of the `PLANNED → READY` transition fails:

-   return an error;
-   do not report `READY_TASK`;
-   do not mutate unrelated tasks.

Use the existing persistence guarantees rather than introducing a
separate scheduler transaction system unless necessary.

------------------------------------------------------------------------

## Acceptance Criteria

T006 is complete when:

-   the scheduler loads persisted tasks;
-   only `PLANNED` tasks are considered for promotion;
-   dependencies are evaluated deterministically;
-   dependency satisfaction requires integrated upstream work (`MERGED`
    or `DONE`);
-   missing dependencies are rejected;
-   blocked dependencies do not mutate downstream tasks;
-   multiple eligible tasks are ordered deterministically;
-   V1 promotes at most one task;
-   an existing active task prevents another task from being promoted;
-   legal state transitions go through the domain state machine;
-   scheduling results distinguish ready, active, waiting, complete, and
    blocked outcomes;
-   repeated calls are safe and deterministic;
-   no LLM, agent, API key, or network access is required;
-   all scheduler tests pass;
-   the existing test suite remains green.

------------------------------------------------------------------------

## Verification

Run:

``` bash
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

Scheduler-specific tests should also be runnable directly:

``` bash
go test ./internal/scheduler/...
```

------------------------------------------------------------------------

## Open Code Review

After local tests pass, run Open Code Review against the task branch
changes.

Review specifically for:

-   scheduling a task with incomplete dependencies;
-   scheduling against unmerged upstream work;
-   nondeterministic map or database ordering;
-   duplicate task promotion;
-   accidentally enabling parallel execution;
-   bypassing the domain state machine;
-   mutating blocked dependent tasks;
-   missing-dependency handling;
-   persistence failures leaving inconsistent state;
-   scheduler logic coupled directly to SQLite;
-   unnecessary complexity.

Fix correctness issues.

Decline style suggestions that add complexity without improving
correctness or maintainability.

------------------------------------------------------------------------

## Git

### Branch

``` text
task/T006-scheduler
```

### Commit

``` text
task(T006): add dependency-aware scheduler
```

### Pull Request

``` text
[Task T006] Add dependency-aware scheduler
```

------------------------------------------------------------------------

## Out of Scope

The following are outside T006:

-   parallel task execution;
-   configurable concurrency;
-   Git branch creation;
-   test execution;
-   coding-agent execution;
-   review-agent execution;
-   pull-request creation;
-   CI monitoring;
-   retries/remediation;
-   `sop run`;
-   `sop resume`;
-   task priority configuration;
-   LLM-based task selection.

These belong to later tasks.

------------------------------------------------------------------------

## Definition of Done

T006 is complete when SOP can inspect the persisted task DAG,
deterministically select at most one legally runnable task, transition
it from `PLANNED` to `READY`, persist that transition, safely report
no-work/blocked/active outcomes, and pass all local and CI verification
without using an LLM.
