# T002 — SQLite State Store

## Status

DONE

## Goal

Add durable local persistence for the agentic-sdlc domain model using SQLite.

T001 created the in-memory Task model and workflow state.

T002 must allow that state to survive process termination and restart.

The persistence layer must remain separate from the domain layer.

---

## Dependency

Depends on:

T001 — Domain Model and State Machine

T001 must be merged into `main`.

Do not redesign the T001 workflow as part of this task.

---

## Architecture

The intended boundary is:

             Domain
               ▲
               │
        TaskRepository
               │
               ▼
         SQLite Store
               │
               ▼
        .agent-sdlc/
           state.db

The domain package must not import:

- database/sql
- SQLite drivers
- persistence implementation packages

Persistence depends on the domain, not the other way around.

---

## Current Domain Model

Persist the Task model currently defined on `main`.

Task currently contains:

- ID
- Title
- Objective
- AcceptanceCriteria
- Status
- BlockedReason
- Attempt
- MaxAttempts
- DependencyIDs
- Attempts
- CreatedAt
- UpdatedAt

Attempt currently contains:

- Number
- Status
- Reason
- Output
- Duration
- Timestamp

Do not add new workflow semantics in T002.

---

## Proposed Package Structure

Create:

internal/store/

Expected structure:

internal/
├── domain/
│   └── existing T001 code
│
└── store/
    ├── repository.go
    ├── sqlite.go
    ├── migrations.go
    └── sqlite_test.go

Exact file boundaries may change if a simpler arrangement is clearer.

Do not add unnecessary abstraction.

---

## Step 1 — Prepare Branch

Update local `main`.

Create:

task/T002-sqlite-store

Verify:

- branch is based on current `main`
- current branch is `task/T002-sqlite-store`
- working tree is clean

---

## Step 2 — Select SQLite Driver

Use a SQLite driver suitable for a small local Go CLI.

Prefer a pure-Go implementation if practical so the final application
does not require CGO or a system SQLite installation.

Before adding the dependency, verify:

- maintained
- works with the project's Go version
- supports database/sql
- no unnecessary runtime services

Keep the dependency footprint small.

Run:

go mod tidy

after adding the dependency.

---

## Step 3 — Define Repository Boundary

Create:

internal/store/repository.go

Define the persistence operations required by T002.

Conceptually:

TaskRepository
├── Save(task)
├── Get(id)
└── List()

Do not add Delete unless it becomes necessary for the acceptance
criteria.

The repository should operate using domain.Task.

The interface should not expose SQL concepts.

Avoid methods such as:

ExecuteSQL()
QueryRows()
BeginTransaction()

Those belong to the SQLite implementation.

---

## Step 4 — Design SQLite Schema

Use three tables.

### tasks

Fields:

id
title
objective
acceptance_criteria
status
blocked_reason
attempt
max_attempts
created_at
updated_at

`id` is the primary key.

### task_dependencies

Fields:

task_id
dependency_task_id

Use a composite primary key:

(task_id, dependency_task_id)

The relationship represents:

Task
  |
  +---- depends on ----> Task

### task_attempts

Fields:

task_id
number
status
reason
output
duration
timestamp

Use:

(task_id, number)

as the logical identity of an attempt.

Store duration in a deterministic numeric representation such as
nanoseconds or milliseconds.

Document the choice.

---

## Step 5 — Foreign Keys

Enable SQLite foreign-key enforcement.

Relationships:

tasks
  |
  +---- task_dependencies
  |
  +---- task_attempts

A dependency should reference existing task IDs when the database model
requires that relationship.

Choose deletion behavior deliberately.

T002 does not need a public task-delete operation.

---

## Step 6 — Migration Strategy

Create:

internal/store/migrations.go

Do not introduce a large migration framework.

For V1, use a small schema-version mechanism.

For example:

schema_version

or SQLite:

PRAGMA user_version

Initialization should conceptually perform:

Open DB
   |
   v
Read schema version
   |
   +-- new DB --> create schema
   |
   +-- current --> continue
   |
   +-- unsupported --> error

The application must be able to open an existing database without
recreating or destroying its data.

---

## Step 7 — Database Initialization

Implement opening/initializing the store.

Conceptually:

Open(path)
   |
   +-- open SQLite
   |
   +-- enable foreign keys
   |
   +-- apply migrations
   |
   +-- return Store

The caller supplies the database path.

Do not hard-code the user's home directory inside the store.

This allows tests to use temporary databases.

---

## Step 8 — Write Persistence Tests First

Before implementing Save/Get/List behavior, create failing tests.

Use temporary directories/databases.

Do not write test databases into the repository.

Example:

t.TempDir()
    |
    v
state.db

Tests must clean themselves up automatically.

---

## Step 9 — Test Database Creation

Write a test proving:

Open(newPath)

creates and initializes a usable SQLite database.

Verify expected schema exists.

Then run:

go test ./internal/store/...

Confirm RED before implementing the required behavior.

---

## Step 10 — Implement Save

Implement:

Save(task)

Saving a Task must persist:

Task row
dependencies
attempt history

Treat one Task save as one logical operation.

Use a transaction:

BEGIN
  |
  +-- save/update task
  |
  +-- save dependencies
  |
  +-- save attempts
  |
COMMIT

On failure:

ROLLBACK

A partial Task must never be considered successfully saved.

---

## Step 11 — Define Save Semantics

Save should support both:

new Task
   |
   v
INSERT

and:

existing Task
   |
   v
UPDATE

Calling Save multiple times for the same Task ID must not create
duplicate Tasks.

Dependencies must not accumulate duplicate rows.

Attempts must not duplicate existing attempt numbers.

The operation should be safe to repeat with the same domain state.

---

## Step 12 — Implement Get

Implement:

Get(id)

It must reconstruct the complete domain Task:

Task
├── scalar fields
├── DependencyIDs
└── Attempts

Loading must not expose database-specific structures to callers.

If the Task does not exist, return a recognizable not-found error.

Do not return an empty Task that looks valid.

---

## Step 13 — Round-Trip Test

Create a realistic Task containing:

- normal fields
- workflow status
- blocked reason if appropriate
- multiple dependencies
- attempt count
- max attempts
- multiple Attempt records
- timestamps

Then:

Original Task
      |
      v
     Save
      |
      v
    SQLite
      |
      v
      Get
      |
      v
 Loaded Task

Verify all persisted values survive.

This is one of the primary acceptance tests for T002.

---

## Step 14 — Restart Test

Persistence must survive closing and reopening the database.

Test:

Open
  |
Save Task
  |
Close
  |
Open same state.db
  |
Get Task
  |
Verify

This proves we have durable state rather than an accidental in-memory
database.

---

## Step 15 — Implement List

Implement:

List()

Return all persisted Tasks.

Choose a deterministic order.

Prefer:

ORDER BY id

unless the application has a stronger reason for another ordering.

Test multiple Tasks.

---

## Step 16 — Dependency Round-Trip Test

Create Tasks such as:

T001
T002 depends on T001
T003 depends on T001 and T002

Save them.

Reload T003.

Verify:

DependencyIDs == [T001, T002]

Do not implement dependency scheduling in T002.

Persistence only.

---

## Step 17 — Attempt Round-Trip Test

Create a Task with multiple attempts.

Example:

Attempt #1
Status: IN_PROGRESS
Reason: implementation started

Attempt #2
Status: CI_FAIL
Reason: integration test failed

Save and reload.

Verify:

- attempt numbers
- statuses
- reasons
- output
- duration
- timestamps

survive the round trip.

---

## Step 18 — Update Existing Task Test

Test:

Save Task

modify Task

Save Task again

Get Task

Verify the latest state is returned.

Example:

READY
  |
  v
IN_PROGRESS

The database should contain one Task row representing the latest state,
not duplicate Tasks.

---

## Step 19 — Replacement Semantics

Pay special attention to child collections.

Suppose a Task originally has:

dependencies:
T001
T002

and later becomes:

dependencies:
T001

After Save + Get, T002 must not remain accidentally.

Likewise, persisted state must accurately reflect the current Task's
attempt collection.

Do not leave stale child records.

---

## Step 20 — Transaction Failure Test

Where practical, test rollback behavior.

The invariant is:

Save(Task)
   |
   +-- everything succeeds
   |       |
   |       v
   |     COMMIT
   |
   +-- anything fails
           |
           v
        ROLLBACK

Do not leave half-written task state.

Avoid creating elaborate fault-injection infrastructure solely for this
test.

Use the simplest meaningful approach.

---

## Step 21 — Unknown Domain Values

SQLite persistence must not silently convert unknown values into valid
domain values.

If invalid status data is encountered, return an error.

Example:

status = "BANANA"

must not become:

PLANNED

or:

READY

silently.

Persistence should preserve domain integrity.

---

## Step 22 — Time Handling

Store timestamps consistently.

Prefer UTC when writing timestamps.

Ensure round-trip tests account for SQLite/time serialization behavior.

Do not introduce timezone-dependent tests.

---

## Step 23 — Default Runtime Location

The store itself accepts a database path.

The intended future application location is:

<project>/.agent-sdlc/state.db

Example:

book-rag/
├── .git/
├── PRD.md
└── .agent-sdlc/
    └── state.db

Do not wire project discovery into T002.

T003 or later application code will decide which project directory to
use.

---

## Step 24 — Run Store Tests

Run:

go test ./internal/store/...

Then:

go test ./...

Fix failures before proceeding.

---

## Step 25 — Run Quality Gate

Run:

make check

Required:

format
vet
test
build

Everything must pass.

---

## Step 26 — Self Review

Review specifically for:

- SQL leaking into domain
- domain importing store
- missing transactions
- stale dependencies after update
- duplicate attempts
- duplicate tasks
- ignored SQL errors
- rows not closed
- transaction rollback mistakes
- invalid status handling
- time serialization problems
- unnecessary abstractions
- accidental scheduler/DAG logic

Fix findings.

Run:

make check

again.

---

## Step 27 — Optional OCR Review

If Open Code Review is configured locally, run OCR against the branch.

Conceptually:

main
  |
  +---- task/T002-sqlite-store
                 |
                 v
                OCR
                 |
          Ollama local model

Review findings before opening or merging the PR.

Do not automatically accept every LLM review suggestion.

Check each finding against the T002 requirements and architecture.

Fix meaningful correctness issues.

Run:

make check

after any changes.

---

## Step 28 — Scope Check

T002 must NOT implement:

- CLI commands
- scheduler
- task DAG execution
- Git operations
- GitHub integration
- agent execution
- LLM calls
- CI polling
- PR creation
- OCR adapter
- parallel task execution
- Docker orchestration
- workflow redesign

SQLite persistence is the task.

---

## Step 29 — Commit

Stage only T002 changes.

Suggested commit:

task(T002): add SQLite state store

---

## Step 30 — Push

Push:

task/T002-sqlite-store

---

## Step 31 — Pull Request

Create:

[Task T002] Add SQLite state store

Target:

main

Include:

- purpose
- schema
- repository boundary
- migration approach
- transaction behavior
- tests
- OCR results if used
- known limitations

---

## Step 32 — CI

Wait for GitHub Actions.

If CI fails:

inspect failure
   |
   v
smallest appropriate fix
   |
   v
make check
   |
   v
commit
   |
   v
push
   |
   v
CI retry

Maximum automatic remediation attempts:

3

After that, stop and mark the task blocked for investigation.

---

## Step 33 — Merge

Merge only when:

- store tests pass
- full tests pass
- `make check` passes
- OCR findings have been reviewed if OCR was run
- CI passes
- acceptance criteria are satisfied

After merge:

update local main

and verify the merge.

Then T002 is DONE.

---

# Acceptance Criteria

- [ ] SQLite dependency added
- [ ] SQLite store package exists
- [ ] domain package has no SQLite dependency
- [ ] schema initializes automatically
- [ ] schema version is tracked
- [ ] foreign keys are enabled
- [ ] Task can be saved
- [ ] Task can be loaded
- [ ] Tasks can be listed
- [ ] existing Task can be updated
- [ ] dependencies persist
- [ ] stale dependencies are removed on update
- [ ] attempts persist
- [ ] duplicate Tasks are not created
- [ ] duplicate dependency rows are not created
- [ ] Task save is transactional
- [ ] not-found behavior is explicit
- [ ] invalid stored status is rejected
- [ ] timestamps round-trip correctly
- [ ] database survives close/reopen
- [ ] temporary databases are used by tests
- [ ] `make check` passes
- [ ] GitHub CI passes
- [ ] PR merged into main

---

# Definition of Done

T002 is DONE when agentic-sdlc can persist its complete current Task
domain model to SQLite, terminate, reopen the database, and reconstruct
the same Task state.

Persistence must be transactional and must not leak SQLite concerns into
the domain layer.

The implementation must be merged into main with passing CI.

---

# Explicitly Out of Scope

Do not implement:

T003 CLI
T004 planner
T005 task DAG behavior
T006 scheduler
T007 Git adapter
T008 test runner
T009 agent harness
T010 task execution
T011 Ponytail review
T012 OCR adapter
GitHub integration
parallel execution
project discovery