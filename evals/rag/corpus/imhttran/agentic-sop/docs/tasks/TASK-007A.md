# T007A --- Scheduler Persistence and Git Validation Corrections

## Status

DONE

## Objective

Apply two focused correctness fixes discovered during
post-implementation review of T006 and T007:

1.  prevent a failed scheduler persistence operation from leaving the
    in-memory task transitioned to `READY`; and
2.  use Git's own ref validation for branch operations rather than
    maintaining an incomplete duplicate of Git's branch-name rules.

This is a corrective task only. It must not expand scheduler or Git
adapter scope.

------------------------------------------------------------------------

## Dependencies

-   T006 --- Dependency-Aware Scheduler
-   T007 --- Deterministic Git Adapter

------------------------------------------------------------------------

## Problem 1 --- Scheduler State Consistency

The scheduler currently follows the conceptual sequence:

``` text
PLANNED task
    ↓
Task.Transition(READY)
    ↓
Store.Save(task)
```

`Task.Transition` mutates the task before persistence succeeds.

If `Save` fails, the durable database remains:

``` text
PLANNED
```

while the in-memory task may remain:

``` text
READY
```

This creates two conflicting views of workflow state.

SOP's durable state must remain authoritative, and a failed scheduling
attempt must not leave an in-memory state transition that was never
persisted.

### Required Behavior

Use a copy or equivalent staged update:

``` text
Original Task: PLANNED
        │
        ▼
Copy Task
        │
        ▼
Transition Copy → READY
        │
        ▼
Persist Copy
   ┌────┴────┐
 failure   success
   │          │
   ▼          ▼
Original    Commit/update
remains     in-memory view
PLANNED     to READY
```

The exact implementation may differ, but these invariants must hold:

-   a persistence failure does not leave the original task as `READY`;
-   `READY_TASK` is not returned on persistence failure;
-   unrelated tasks are unchanged;
-   the error is returned to the caller;
-   successful persistence still results in `READY`.

Do not add rollback logic that introduces a second persistence write
merely to undo an in-memory mutation.

Prefer staging the mutation before persistence.

------------------------------------------------------------------------

## Scheduler Tests

Add or strengthen the persistence-failure test.

Given:

``` text
T001 PLANNED
```

and a store configured so:

``` text
Save(T001) → error
```

Expected:

``` text
Scheduler.Next() → error
T001.Status       → PLANNED
```

Also verify:

``` text
Outcome != READY_TASK
```

The test must inspect the original/in-memory task, not only the returned
scheduler result.

Keep the existing successful transition test proving:

``` text
PLANNED → READY
```

when persistence succeeds.

------------------------------------------------------------------------

## Problem 2 --- Git Branch Validation

T007 currently maintains a manual approximation of Git branch/ref
validation.

Conceptually:

``` go
strings.HasPrefix(...)
strings.HasSuffix(...)
strings.Contains(...)
```

This duplicates Git behavior and risks accepting branch names that Git
itself rejects as its ref-name rules evolve or cover cases omitted by
SOP.

SOP already requires the installed Git executable.

Git should therefore be the authority for validating branch names used
in Git operations.

------------------------------------------------------------------------

## Required Git Validation

Use the equivalent of:

``` bash
git check-ref-format --branch <branch>
```

through the existing deterministic Git command boundary.

Do not invoke a shell.

Continue using:

``` go
exec.CommandContext(ctx, "git", args...)
```

or the existing `run` helper.

Conceptually:

``` text
Candidate Branch
      │
      ▼
git check-ref-format --branch
      │
   ┌──┴──┐
 valid  invalid
   │      │
   ▼      ▼
continue error
```

Git validation should apply before mutating operations involving
caller-supplied branch names.

At minimum review:

-   `UpdateIntegrationBranch`
-   `BranchExists`
-   `CreateBranch`
-   `Checkout`

Do not allow a failed validation to perform repository mutations.

------------------------------------------------------------------------

## Pure Branch-Name Generation

Keep deterministic task branch generation separate from repository
mutation.

For example:

``` text
Task ID + Title
      ↓
TaskBranchName
      ↓
task/T007-deterministic-git-adapter
      ↓
Git validation
      ↓
Git operation
```

`TaskBranchName` must remain:

-   deterministic;
-   model-free;
-   network-free.

It may retain lightweight input checks needed to construct a sensible
candidate, but SOP should not maintain a second full implementation of
Git ref syntax.

If `TaskBranchName` needs authoritative validation without a repository,
use a small Git validation boundary appropriate to the existing package
design rather than recreating Git's rules manually.

Avoid turning this patch into a large interface redesign.

------------------------------------------------------------------------

## Error Handling

Invalid branch names should return clear errors.

Preserve enough context to identify the rejected branch while continuing
to wrap underlying Git/process errors where useful.

Do not interpret every `check-ref-format` failure as a missing branch.

Validation failure and branch nonexistence are different conditions.

------------------------------------------------------------------------

## Safety

This patch must preserve all T007 safety guarantees.

Do not introduce:

``` text
git reset --hard
git clean -fd
git checkout -f
git switch -C
git checkout -B
git push --force
git rebase
git commit --amend
```

Do not:

-   discard user changes;
-   automatically stash;
-   reset an existing branch;
-   modify Git configuration;
-   modify remotes;
-   bypass hooks;
-   add shell interpolation.

------------------------------------------------------------------------

## Git Validation Tests

Add tests covering Git's validation boundary.

At minimum:

### Valid Branch

``` text
task/T007-git-adapter
```

Expected:

``` text
valid
```

### Invalid Branch

Use one or more branch names rejected by:

``` bash
git check-ref-format --branch
```

Expected:

-   adapter returns an error;
-   no branch is created;
-   current branch remains unchanged.

### Existing Branch

Existing behavior must remain:

``` text
valid name + existing local branch
    ↓
ErrBranchExists
```

Do not confuse an existing branch with an invalid branch.

### Valid But Missing Branch

For an operation such as checkout:

``` text
valid branch syntax
+
branch does not exist
```

Expected:

``` text
Git operation error
```

not a validation error.

### Context Cancellation

Existing context-cancellation behavior must remain intact.

------------------------------------------------------------------------

## Regression Tests

Run the complete scheduler and Git adapter suites.

Verify that these existing behaviors remain unchanged:

``` text
Scheduler
---------
deterministic ordering
MERGED/DONE dependency requirement
single active task
blocked dependency handling
missing dependency handling
ALL_DONE
ACTIVE_TASK
WAITING_ON_DEPENDENCIES

Git Adapter
-----------
repository validation
clean/dirty detection
detached HEAD handling
fast-forward-only main update
diverged-main rejection
existing-branch rejection
dirty-tree rejection
diff inspection
commit
nothing-to-commit error
context cancellation
READY → BRANCH_CREATED only after Git success
```

------------------------------------------------------------------------

## Acceptance Criteria

T007A is complete when:

1.  scheduler persistence failure leaves the original task `PLANNED`;
2.  scheduler persistence failure returns an error;
3.  scheduler persistence failure never reports `READY_TASK`;
4.  successful scheduling still persists and produces `READY`;
5.  Git is the authoritative validator for branch names used by Git
    operations;
6.  invalid branch names are rejected before repository mutation;
7.  valid-but-missing branches are distinguished from invalid branch
    names;
8.  existing branches still produce the expected existing-branch
    behavior;
9.  deterministic task branch generation remains model-free;
10. no destructive Git operations are introduced;
11. no shell command construction is introduced;
12. scheduler and Git regression tests pass;
13. the complete repository test suite remains green.

------------------------------------------------------------------------

## Verification

Run:

``` bash
go test ./internal/scheduler/...
go test ./internal/git/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

------------------------------------------------------------------------

## Open Code Review

After local verification passes, run Open Code Review against the patch
branch.

Review specifically for:

-   scheduler mutation occurring before durable persistence;
-   attempted rollback writes after persistence failure;
-   duplicated Git ref-validation logic;
-   Git validation occurring after a mutating command;
-   shell interpolation;
-   valid missing branches being treated as invalid syntax;
-   existing branches being treated as invalid syntax;
-   accidental destructive Git behavior;
-   unnecessary interface or architecture expansion.

Fix correctness and safety findings.

Decline style-only suggestions that expand this focused patch without
improving correctness or maintainability.

------------------------------------------------------------------------

## Git

### Branch

``` text
task/T007A-scheduler-git-corrections
```

### Commit

``` text
task(T007A): fix scheduler persistence and git validation
```

### Pull Request

``` text
[Task T007A] Fix scheduler persistence and Git validation
```

------------------------------------------------------------------------

## Out of Scope

Do not add:

-   T008 test-runner functionality;
-   parallel scheduling;
-   GitHub API integration;
-   push behavior;
-   pull-request creation;
-   CI monitoring;
-   merge automation;
-   resume behavior;
-   worktrees;
-   branch cleanup;
-   scheduler priority;
-   `.agent-sdlc` migration;
-   repository rename logic.

------------------------------------------------------------------------

## Definition of Done

T007A is complete when a failed scheduler save cannot create an
in-memory state that disagrees with durable state, Git itself is
authoritative for branch-name validation, all affected regression tests
pass, and no additional workflow behavior is introduced.
