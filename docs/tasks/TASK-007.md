# T007 --- Deterministic Git Adapter

## Status

DONE

## Objective

Implement SOP's deterministic Git boundary.

The Git adapter provides the repository operations required by the task
workflow without allowing an LLM or coding agent to control Git state
directly.

For T007, a task that has reached `READY` must be able to obtain its own
task branch from the current integration branch and then transition to
`BRANCH_CREATED`.

``` text
READY Task
    │
    ▼
 Git Adapter
    │
    ├─ validate repository
    ├─ verify repository state
    ├─ identify integration branch
    ├─ update integration branch
    ├─ create task branch
    └─ verify checkout
    │
    ▼
BRANCH_CREATED
```

The adapter also establishes deterministic primitives for status
inspection, diff inspection, checkout, and commit operations that later
workflow stages will use.

------------------------------------------------------------------------

## Dependencies

-   T003 --- CLI Foundation
-   T006 --- Dependency-Aware Scheduler

T006 determines which task may run.

T007 begins the repository lifecycle for that selected `READY` task.

------------------------------------------------------------------------

## Design Rule

Preserve SOP's control boundary:

``` text
Agents do the work.
SOP controls the process.
```

Git operations are workflow control.

An agent may generate code or suggest a commit message in later stages,
but it must not independently decide to:

-   create branches;
-   switch branches;
-   update `main`;
-   commit;
-   merge;
-   push;
-   rewrite history.

Those operations belong to deterministic application code.

------------------------------------------------------------------------

## V1 Scope

Implement a small Git adapter around the installed `git` CLI.

T007 should support:

-   repository validation;
-   clean/dirty status;
-   current branch detection;
-   integration-branch update;
-   branch existence checks;
-   deterministic task branch naming;
-   branch creation;
-   checkout/switch;
-   diff inspection;
-   commit creation.

T007 does **not** implement GitHub, pull requests, CI, merge automation,
or parallel worktrees.

------------------------------------------------------------------------

## Git CLI Boundary

Use the local `git` executable rather than introducing a Git library
unless there is a concrete requirement that the CLI cannot satisfy.

Conceptually:

``` text
SOP
 │
 ▼
Git Adapter
 │
 ▼
git CLI
 │
 ▼
Working Repository
```

The adapter owns:

-   command construction;
-   working directory;
-   stdout/stderr capture;
-   exit-code interpretation;
-   error wrapping;
-   validation.

Other SOP components should call the adapter rather than construct
arbitrary Git commands themselves.

------------------------------------------------------------------------

## Proposed Package

Add:

``` text
internal/git/
```

Suggested shape:

``` text
internal/git/
├── git.go
├── command.go
└── git_test.go
```

A possible boundary is:

``` go
type Adapter interface {
    ValidateRepository(ctx context.Context) error
    Status(ctx context.Context) (Status, error)
    CurrentBranch(ctx context.Context) (string, error)
    UpdateIntegrationBranch(ctx context.Context, branch string) error
    BranchExists(ctx context.Context, branch string) (bool, error)
    CreateBranch(ctx context.Context, branch string) error
    Checkout(ctx context.Context, branch string) error
    Diff(ctx context.Context) (string, error)
    Commit(ctx context.Context, message string) error
}
```

The exact interface may be smaller if implementation shows some methods
should be combined.

Keep Git command execution behind one boundary.

------------------------------------------------------------------------

## Command Execution

Every Git command must execute with:

-   an explicit repository working directory;
-   the supplied `context.Context`;
-   captured stdout;
-   captured stderr;
-   deterministic arguments;
-   no interactive prompts.

Do not use:

``` text
sh -c
```

to construct Git commands from strings.

Prefer direct execution such as:

``` go
exec.CommandContext(ctx, "git", args...)
```

This avoids shell quoting problems and unnecessary command-injection
surface.

------------------------------------------------------------------------

## Repository Validation

Before mutating Git state, verify that the target directory is a Git
working tree.

A suitable check is conceptually equivalent to:

``` bash
git rev-parse --is-inside-work-tree
```

Return a clear error when:

-   Git is unavailable;
-   the directory does not exist;
-   the directory is not a Git repository;
-   Git metadata cannot be read.

Do not initialize a repository automatically.

Repository initialization is outside T007.

------------------------------------------------------------------------

## Clean / Dirty Status

The adapter must be able to determine whether the working tree contains
changes.

Use porcelain output suitable for machine parsing, for example:

``` bash
git status --porcelain
```

The result should distinguish at least:

``` text
CLEAN
DIRTY
```

Untracked files count as dirty.

Before switching/updating the integration branch or creating a task
branch, the working tree must be clean.

Do not automatically discard, stash, reset, or commit user changes.

A dirty working tree should stop the operation with a useful error.

------------------------------------------------------------------------

## Current Branch

Provide deterministic current-branch detection.

The adapter must return the branch name when attached to a branch.

A detached HEAD should be reported explicitly rather than being treated
as a normal branch.

Do not guess which branch the user intended.

------------------------------------------------------------------------

## Integration Branch

For V1, use:

``` text
main
```

as the default integration branch.

Keep the implementation structured so the integration branch can later
become project configuration.

Before creating a task branch:

``` text
validate repository
      ↓
verify clean working tree
      ↓
checkout main
      ↓
update main
      ↓
create task branch
```

The adapter should not merge arbitrary feature branches into `main`.

------------------------------------------------------------------------

## Updating `main`

Updating the integration branch should be conservative.

Preferred V1 behavior:

``` bash
git fetch origin main
git checkout main
git merge --ff-only origin/main
```

or an equivalent fast-forward-only sequence.

Requirements:

-   fetch the configured remote integration branch;
-   never create a merge commit merely to update local `main`;
-   never force-update local history;
-   fail when the update cannot be fast-forwarded;
-   preserve the user's repository state on failure.

Do not use an unrestricted:

``` bash
git pull
```

where its merge/rebase behavior may depend on user configuration.

### No Remote

Some tests and local projects may have no `origin`.

Repository inspection and local-only operations should still work.

An operation that specifically requires updating from `origin` should
return a clear error if the remote is unavailable.

Do not silently pretend the integration branch was updated.

------------------------------------------------------------------------

## Task Branch Naming

Use the existing convention:

``` text
task/<id>-<slug>
```

Example:

``` text
task/T007-git-adapter
```

Branch names must be generated deterministically from:

-   task ID;
-   task title.

Example:

``` text
ID:    T007
Title: Deterministic Git Adapter

branch:
task/T007-deterministic-git-adapter
```

### Slug Rules

Use a small deterministic slug function.

At minimum:

-   lowercase title text;
-   replace runs of unsupported separators/whitespace with `-`;
-   trim leading/trailing `-`;
-   keep task ID intact;
-   reject an empty resulting slug;
-   produce a valid Git branch name.

Do not use an LLM to create branch names.

------------------------------------------------------------------------

## Existing Branches

Before creating a task branch, check whether it already exists.

Do not silently overwrite or reset an existing branch.

For T007:

``` text
branch absent
    ↓
create branch

branch exists
    ↓
return explicit error
```

Resume/recovery behavior for an existing task branch belongs to the
later resume workflow.

Do not use:

``` bash
git checkout -B
git switch -C
```

because those commands can reset an existing branch.

------------------------------------------------------------------------

## Branch Creation

A task branch must be created from the updated integration branch.

Conceptually:

``` text
origin/main
     │
     ▼
local main
     │
     ▼
task/T007-deterministic-git-adapter
```

The operation must verify that the resulting checked-out branch is the
expected task branch.

Only after successful branch creation should the workflow be allowed to
record:

``` text
READY → BRANCH_CREATED
```

------------------------------------------------------------------------

## Workflow State Ownership

The Git adapter itself should not own task persistence.

Keep the responsibilities separate:

``` text
Orchestrator / Task Runner
       │
       ├─ calls Git Adapter
       │
       ▼
Git branch successfully created
       │
       ▼
Task.Transition(BRANCH_CREATED)
       │
       ▼
Persist Task
```

The adapter reports Git results.

The workflow layer decides whether the task state may advance.

Do not have `internal/git` import SQLite-specific code.

------------------------------------------------------------------------

## Checkout

Provide a safe branch checkout/switch operation for later workflow
stages.

Requirements:

-   validate the requested branch name;
-   fail if the working tree is dirty where switching would be unsafe;
-   capture Git errors;
-   verify the resulting branch.

Do not use force checkout.

------------------------------------------------------------------------

## Diff Inspection

Provide a deterministic way to inspect changes.

At minimum support the equivalent of:

``` bash
git diff
```

Later stages may need staged or range-based diffs, so keep the boundary
extensible without building all variants in T007.

Diff inspection is read-only.

------------------------------------------------------------------------

## Commit

Provide a deterministic commit primitive for later workflow stages.

Conceptually:

``` bash
git commit -m "<message>"
```

Requirements:

-   no shell interpolation;
-   explicit commit message;
-   return an error if there is nothing to commit;
-   return Git stdout/stderr for diagnostics where useful;
-   do not automatically use `--amend`;
-   do not bypass hooks;
-   do not force-sign commits;
-   do not push.

T007 does not decide *when* a commit is allowed. The later workflow gate
owns that decision.

------------------------------------------------------------------------

## Safety Rules

T007 must never automatically run destructive history-changing commands
such as:

``` text
git reset --hard
git clean -fd
git push --force
git branch -D
git rebase
git commit --amend
```

It must not automatically:

-   discard local changes;
-   stash local changes;
-   delete branches;
-   resolve merge conflicts;
-   modify Git configuration;
-   change remotes;
-   push credentials or secrets.

When safe progress is impossible, return an error and let the workflow
stop.

------------------------------------------------------------------------

## Errors

Git errors should include enough context to diagnose the failed
operation without requiring the caller to understand raw process
behavior.

For example:

``` text
update integration branch: git merge --ff-only origin/main: non-fast-forward
```

Avoid exposing secrets if Git output contains credential-related
information.

Use wrapped errors so callers can retain the original cause where
practical.

------------------------------------------------------------------------

## Testing Strategy

Tests must use temporary real Git repositories.

Do not mock every Git command; the value of this adapter is verifying
actual Git behavior.

Tests may create local temporary repositories and local bare
repositories to act as `origin`.

They must not require:

-   GitHub;
-   network access;
-   credentials;
-   an LLM;
-   an agent harness.

Configure test-local Git identity where commits are needed rather than
depending on the developer's global Git configuration.

------------------------------------------------------------------------

## Tests

Write tests before implementation.

At minimum cover the following.

### Repository Validation

Given a temporary Git repository:

``` text
ValidateRepository → success
```

Given a normal directory:

``` text
ValidateRepository → error
```

### Clean Repository

Given no working-tree changes:

``` text
Status → CLEAN
```

### Dirty Repository

Given modified or untracked files:

``` text
Status → DIRTY
```

### Current Branch

Given a repository on:

``` text
main
```

Expected:

``` text
CurrentBranch → main
```

### Detached HEAD

Expected:

``` text
CurrentBranch → explicit detached-HEAD error/result
```

### Deterministic Branch Name

Given:

``` text
T007
Deterministic Git Adapter
```

Expected:

``` text
task/T007-deterministic-git-adapter
```

### Create Task Branch

Given a clean repository on updated `main`:

``` text
CreateBranch(task/T007-deterministic-git-adapter)
```

Expected current branch:

``` text
task/T007-deterministic-git-adapter
```

### Existing Branch

Given the task branch already exists:

Expected:

-   explicit error;
-   existing branch is not reset;
-   repository history remains unchanged.

### Dirty Tree Before Branch Creation

Expected:

-   operation fails;
-   changes remain untouched;
-   current branch remains unchanged.

### Fast-Forward Integration Update

Use a local bare repository as `origin`.

Create a new commit on remote `main`.

Expected:

``` text
UpdateIntegrationBranch(main)
```

fast-forwards local `main` to the remote commit.

### Diverged Integration Branch

Create different local and remote commits.

Expected:

-   update fails;
-   no merge commit is created;
-   local commit is not destroyed.

### Missing Origin

Expected:

-   local read-only Git operations still work;
-   remote update returns a clear error.

### Diff

Modify a tracked file.

Expected:

``` text
Diff()
```

returns the Git diff containing the modification.

### Commit

Stage a file and commit through the adapter.

Expected:

-   commit succeeds;
-   commit message is correct;
-   new commit exists.

### Nothing to Commit

Expected:

``` text
Commit(...)
```

returns an error.

### Context Cancellation

Cancel the context for a Git operation.

Expected:

-   operation terminates;
-   cancellation is returned rather than hanging.

------------------------------------------------------------------------

## State Transition Integration

Add focused workflow-level coverage proving:

``` text
READY
  ↓
successful branch creation
  ↓
BRANCH_CREATED
```

and:

``` text
READY
  ↓
Git failure
  ↓
READY
```

A failed Git operation must not falsely advance task state.

Do not duplicate the domain transition table in Git code.

------------------------------------------------------------------------

## Acceptance Criteria

T007 is complete when:

1.  SOP can validate that a target directory is a Git working tree.
2.  SOP can determine clean versus dirty working-tree state.
3.  SOP can identify the current branch.
4.  Detached HEAD is handled explicitly.
5.  SOP can update local `main` from `origin/main` using
    fast-forward-only behavior.
6.  A non-fast-forward update fails without rewriting history.
7.  SOP can deterministically generate `task/<id>-<slug>` branch names.
8.  SOP can detect an existing task branch.
9.  SOP can create and switch to a new task branch from updated `main`.
10. Dirty user changes are never discarded or automatically stashed.
11. Existing task branches are never silently reset.
12. SOP can inspect a Git diff.
13. SOP can create a normal commit when explicitly requested.
14. Git commands execute without shell interpolation.
15. The Git adapter does not own workflow persistence.
16. A task advances from `READY` to `BRANCH_CREATED` only after branch
    creation succeeds.
17. Git failure leaves the task in `READY`.
18. Tests use temporary local repositories and require no network
    access.
19. No destructive or force Git operations are introduced.
20. The full existing test suite remains green.

------------------------------------------------------------------------

## Verification

Run:

``` bash
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

Git-adapter tests should also run directly:

``` bash
go test ./internal/git/...
```

------------------------------------------------------------------------

## Open Code Review

After local verification passes, run Open Code Review against the task
branch.

Review specifically for:

-   shell injection;
-   destructive Git commands;
-   implicit `git pull` behavior;
-   dirty-working-tree handling;
-   detached HEAD handling;
-   accidental branch reset;
-   branch-name validation;
-   non-fast-forward behavior;
-   incorrect working directory;
-   context cancellation;
-   workflow state advancing before Git success;
-   tests depending on global Git configuration;
-   tests requiring network access;
-   unnecessary abstraction.

Fix correctness and safety findings.

Decline style-only changes that add complexity without improving the Git
boundary.

------------------------------------------------------------------------

## Git

### Branch

``` text
task/T007-git-adapter
```

### Commit

``` text
task(T007): add deterministic git adapter
```

### Pull Request

``` text
[Task T007] Add deterministic Git adapter
```

------------------------------------------------------------------------

## Out of Scope

The following are outside T007:

-   GitHub API integration;
-   pull-request creation;
-   pushing task branches;
-   CI monitoring;
-   merge automation;
-   merge-conflict resolution;
-   force push;
-   rebasing;
-   worktrees;
-   parallel task execution;
-   automatic stashing;
-   branch cleanup;
-   resume/recovery of existing task branches;
-   agent-generated Git commands;
-   `sop run`.

Those belong to later workflow stages.

------------------------------------------------------------------------

## Definition of Done

T007 is complete when SOP has a tested deterministic Git boundary that
can safely validate a repository, update `main` without rewriting
history, create a new deterministic task branch for a `READY` task,
expose basic status/diff/commit operations, and allow the workflow to
advance to `BRANCH_CREATED` only after successful branch creation.
