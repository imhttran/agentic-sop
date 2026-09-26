# T002A — Correct SQLite Persistence Behavior

## Status

DONE

## Dependency

T002 must already be merged into main.

Branch from current main.

Suggested branch:

task/T002A-persistence-corrections

---

## Scope

T002A fixes:

1. List() must hydrate complete Task objects.
2. Replace CGO SQLite driver with a pure-Go SQLite driver.
3. Ensure SQLite foreign-key enforcement is active for database connections.
4. Add regression tests for all three behaviors.
5. Reduce duplicated Task hydration code where practical.

T002A must not add:

- CLI commands
- scheduler behavior
- DAG execution
- Git operations
- GitHub integration
- agents
- OCR integration
- CI orchestration
- workflow-state changes

---

## Step 1 — Verify Starting State

Update local main.

Confirm:

- T002 merge is present
- working tree is clean
- tests pass before changes

Run:

go test ./...

make check

Record any pre-existing failures before modifying code.

---

## Step 2 — Create Branch

Create:

task/T002A-persistence-corrections

Verify current branch before modifying files.

---

## Step 3 — Add Regression Test for List()

Write the failing test first.

Create at least two Tasks.

One Task should include:

- dependencies
- multiple attempts
- non-default status
- timestamps
- attempt metadata

Persist them.

Call:

List()

Verify each returned Task contains the same persisted child data.

Example expectation:

T002
├── DependencyIDs
│   └── T001
└── Attempts
    ├── #1
    └── #2

The test should fail against the current T002 implementation because
Attempts are not loaded by List().

Run:

go test ./internal/store/...

Confirm RED.

---

## Step 4 — Refactor Task Hydration

Avoid maintaining two separate implementations of domain reconstruction.

Current conceptual duplication:

Get()
  ├── scan task
  ├── load dependencies
  └── load attempts

List()
  ├── scan task
  ├── load dependencies
  └── attempts missing

Move common behavior behind internal helper functions.

Possible design:

loadTaskChildren(task)
├── loadDependencies(task.ID)
└── loadAttempts(task.ID)

Then:

Get()
  ↓
scan Task
  ↓
loadTaskChildren()

List()
  ↓
scan Task
  ↓
loadTaskChildren()

Exact helper names are not important.

The goal is one shared hydration path where practical.

Do not add an unnecessary generic repository framework.

---

## Step 5 — Implement Complete List()

List() must return complete persisted domain.Task values.

Each returned Task must contain:

- scalar fields
- DependencyIDs
- Attempts

Behavior should match Get().

List() should remain deterministically ordered.

Prefer:

ORDER BY id

Run:

go test ./internal/store/...

Confirm the new List regression test passes.

---

## Step 6 — Add List/Get Consistency Test

Persist one Task.

Load it using:

Get(id)

and also retrieve it through:

List()

Find the matching Task from List().

Verify the domain state is equivalent.

Conceptually:

       SQLite
      /      \
     v        v
   Get()    List()
     |        |
     +--- same Task ---+

This protects the repository contract from drifting again later.

---

## Step 7 — Replace SQLite Driver

Remove:

github.com/mattn/go-sqlite3

Use a maintained pure-Go SQLite implementation compatible with
database/sql.

Preferred direction:

modernc.org/sqlite

Verify current driver usage and DSN syntax from its documentation before
making the change.

Do not change repository interfaces merely because the driver changes.

Desired architecture remains:

TaskRepository
      |
      v
database/sql
      |
      v
pure-Go SQLite

Run:

go mod tidy

Verify mattn/go-sqlite3 is no longer present in go.mod/go.sum unless
required transitively for an unrelated reason.

---

## Step 8 — Preserve Store API

Existing callers should continue using:

Open(path)

Save(task)

Get(id)

List()

Close()

Do not expose driver-specific types.

T003 should not care which SQLite implementation is underneath the
store.

---

## Step 9 — Fix Foreign-Key Configuration

Do not rely on this pattern alone:

db.Exec("PRAGMA foreign_keys = ON")

because database/sql may use multiple underlying SQLite connections.

Configure foreign-key enforcement using the selected driver's supported
connection configuration or DSN mechanism.

Desired invariant:

Every connection used by this Store
          |
          v
foreign_keys = ON

Do not silently assume it is active.

---

## Step 10 — Add Foreign-Key Verification Test

Add a test proving foreign-key enforcement actually works.

For example:

Create task T002 with dependency:

T999

when T999 does not exist.

Saving should fail if the schema requires referenced dependency Tasks to
exist.

Then create T999 and verify the save succeeds.

Alternative test approaches are acceptable if they prove the same
invariant.

The test must verify behavior, not merely query:

PRAGMA foreign_keys

A behavioral constraint test is stronger.

---

## Step 11 — Check Save Semantics After Driver Change

Re-run tests covering:

- new Task save
- existing Task update
- stale dependency replacement
- stale attempt replacement
- transaction rollback
- close/reopen durability
- not-found behavior
- invalid persisted status handling

The driver migration must not alter domain behavior.

---

## Step 12 — Confirm Transaction Behavior

Verify that:

Save(task)

still behaves atomically.

Conceptually:

BEGIN
  ├── task row
  ├── dependencies
  └── attempts
COMMIT

Failure anywhere:

ROLLBACK

Pay particular attention to any driver differences involving:

- transactions
- foreign keys
- connection handling
- multi-statement schema initialization

Do not change semantics unless required for correctness.

---

## Step 13 — Migration Test

Verify existing schema initialization still works:

new database
    ↓
version 0
    ↓
create schema
    ↓
user_version = 1

Also verify:

existing version 1 DB
    ↓
Open()
    ↓
no destructive recreation

Do not introduce schema version 2 unless schema itself changes.

A driver change alone should not require a schema-version bump.

---

## Step 14 — Reopen Durability Test

Repeat:

Open
 ↓
Save
 ↓
Close
 ↓
Open same file
 ↓
Get / List
 ↓
verify full Task

Make sure Attempts survive both:

Get()

and:

List()

after reopening the database.

---

## Step 15 — Error Handling Review

Check:

- rows.Err()
- Close() calls
- transaction rollback paths
- parsing errors
- invalid status handling
- not-found handling
- dependency constraint errors

Do not swallow driver errors.

Wrap errors when added context materially helps identify the failing
operation.

Avoid excessive error wrapping.

---

## Step 16 — Run Focused Tests

Run:

go test ./internal/store/...

Then:

go test ./...

---

## Step 17 — Run Quality Gate

Run:

make check

Required:

- format
- vet
- tests
- build

All must pass.

---

## Step 18 — Self Review

Review specifically for:

- List/Get behavioral mismatch
- duplicated hydration logic
- CGO dependency remaining
- driver-specific code leaking outside store
- foreign-key enforcement assumptions
- transaction regressions
- stale child records
- missing attempts from List()
- schema-version changes without reason
- unnecessary abstraction

Fix any meaningful issues.

Run:

make check

again.

---

## Step 19 — OCR Review

If local OCR is configured:

ocr review --from main --to task/T002A-persistence-corrections

Review findings manually.

Fix correctness issues supported by code and requirements.

Do not automatically accept stylistic changes that increase complexity.

Run:

make check

after changes.

---

## Step 20 — Commit

Suggested commit:

task(T002A): correct SQLite persistence behavior

---

## Step 21 — Push

Push:

task/T002A-persistence-corrections

---

## Step 22 — Pull Request

Suggested title:

[Task T002A] Correct SQLite persistence behavior

PR description should mention:

- List now hydrates Attempts
- Get/List share common hydration behavior
- SQLite driver changed to pure Go
- foreign-key enforcement strengthened
- regression tests added
- no schema semantics changed
- no T003 work included

---

## Step 23 — CI

Wait for GitHub Actions.

If CI fails:

failure
  ↓
identify cause
  ↓
small fix
  ↓
make check
  ↓
push
  ↓
CI

Maximum remediation attempts:

3

Then stop for investigation.

---

## Step 24 — Merge Gate

Merge only if:

- List returns complete Tasks
- Get/List consistency test passes
- pure-Go SQLite driver is in use
- CGO SQLite dependency is gone
- behavioral foreign-key test passes
- full store test suite passes
- make check passes
- CI passes
- OCR findings reviewed if OCR used

---

# Acceptance Criteria

- [ ] `List()` loads Attempts
- [ ] `List()` loads dependencies
- [ ] `Get()` and `List()` return equivalent domain state
- [ ] shared hydration logic reduces duplication
- [ ] `mattn/go-sqlite3` removed
- [ ] pure-Go SQLite driver added
- [ ] Store API unchanged
- [ ] foreign keys reliably enforced
- [ ] behavioral FK regression test exists
- [ ] transaction semantics unchanged
- [ ] schema version remains valid
- [ ] close/reopen persistence still works
- [ ] existing T002 tests continue passing
- [ ] new regression tests pass
- [ ] `make check` passes
- [ ] GitHub CI passes

---

# Definition of Done

T002A is done when the persistence layer provides one consistent
repository contract:

             SQLite
                |
        +-------+-------+
        |               |
       Get             List
        |               |
        +-------+-------+
                |
          complete Task
           ├ dependencies
           └ attempts

The SQLite implementation must be pure Go, preserve the existing Store
API, and enforce database relationships reliably without leaking
SQLite-specific behavior into the domain layer.

T002A must be merged into main before T003 begins.