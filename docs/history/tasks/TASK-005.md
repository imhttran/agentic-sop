# T005 — Plan to Task DAG

## Status

DONE

## Goal

Convert a validated planner.Plan into executable domain.Tasks with
dependencies, validate the resulting dependency graph, and persist the
tasks.

T005 is deterministic.

Do not call an LLM.

---

## Branch

task/T005-plan-to-task-dag

Commit:

task(T005): build task DAG from plan

PR:

[Task T005] Build task DAG from plan

---

## Architecture

planner.Plan
│
▼
Task Builder
│
▼
[]domain.Task
│
▼
DAG Validator
│
├── valid ──────→ persist
│
└── invalid ────→ error
no persistence

Desired package direction:

planner
│
▼
taskbuilder
│
├── domain
│
└── graph validation

CLI
│
▼
application operation
│
├── planner
├── taskbuilder
└── store

Do not put DAG logic in CLI.

---

## Step 1 — Define the Mapping

Each Plan Stage becomes one domain.Task.

Example:

Stage:
ID: S001
Title: Application skeleton
Objective: Create Go application
Dependencies: []
AcceptanceCriteria: - application starts

becomes:

Task:
ID: S001
Title: Application skeleton
Objective: Create Go application
DependencyIDs: []
AcceptanceCriteria: ...
Status: PLANNED
Attempt: 0
MaxAttempts: 3

Do not invent new task IDs in T005.

Preserve the stage ID.

This gives traceability:

PLAN.md S001
│
▼
Plan.Stages[S001]
│
▼
Task S001
│
▼
SQLite S001

---

## Step 2 — Acceptance Criteria Representation

The existing domain.Task currently stores:

AcceptanceCriteria string

while planner.Stage contains:

AcceptanceCriteria []string

For T005, use one deterministic serialization format.

Prefer newline-delimited criteria:

criterion one
criterion two
criterion three

Do not redesign the persistence schema yet.

Add a helper if needed so serialization behavior has one owner.

---

## Step 3 — Build Tasks

Create a deterministic builder conceptually like:

Build(plan *planner.Plan) ([]*domain.Task, error)

Responsibilities:

- require a valid Plan
- convert each Stage to Task
- preserve IDs
- preserve title
- preserve objective
- preserve acceptance criteria
- preserve dependency IDs
- initialize workflow fields

New Tasks should start:

Status = PLANNED
Attempt = 0
MaxAttempts = 3
BlockedReason = NO_REASON

CreatedAt / UpdatedAt should be initialized consistently.

---

## Step 4 — Keep Planning and Execution Separate

The Task Builder must not:

- invoke an Agent
- create Git branches
- execute tests
- transition tasks to READY
- schedule work
- open PRs
- call GitHub
- call CI

T005 answers:

"What work exists and how is it connected?"

T006 answers:

"What work is ready to run?"

---

## Step 5 — Model the Dependency Graph

Example:

S001
├────→ S002
│
└────→ S003
│
▼
S004

Meaning:

S002 depends on S001
S003 depends on S001
S004 depends on S003

The stored Task representation remains:

S002.DependencyIDs = ["S001"]
S003.DependencyIDs = ["S001"]
S004.DependencyIDs = ["S003"]

Do not add a separate graph database.

---

## Step 6 — Validate the DAG

Plan validation already catches:

- unknown dependency
- self dependency

T005 must additionally detect cycles.

Example:

S001
↓
S002
↓
S003
↓
S001

must fail.

Use normal graph traversal.

DFS or Kahn's algorithm is fine.

Prefer the simpler implementation.

---

## Step 7 — Detect Longer Cycles

Tests should include:

direct cycle:

S001 → S002 → S001

long cycle:

S001 → S002 → S003 → S001

and a valid diamond:

      S001
      /  \

S002 S003
\ /
S004

The diamond must pass.

---

## Step 8 — Deterministic Ordering

The resulting Task slice should have stable ordering.

Prefer Plan stage order.

Do not reorder tasks just because a graph algorithm visits them
differently.

This helps:

- tests
- CLI output
- debugging
- PLAN ↔ Task traceability

---

## Step 9 — Atomic Persistence

This is important.

Given:

Plan
↓
4 Tasks

we do NOT want:

T001 saved
T002 saved
T003 fails
T004 never saved

leaving half a project.

T005 should persist the generated task set atomically.

Conceptually:

BEGIN
save T001
save T002
save T003
save T004
COMMIT

on error:

ROLLBACK

---

## Step 10 — Add Batch Store Operation

Prefer adding a store operation conceptually like:

SaveTasks(tasks []*domain.Task) error

rather than having the CLI manually loop:

for task:
store.Save(task)

The store should own transaction semantics.

Existing Save(task) can remain.

SaveTasks should use one transaction.

---

## Step 11 — Dependency Insert Ordering

SQLite foreign keys mean referenced Tasks must exist before dependency
edges are inserted.

For batch persistence use two phases:

BEGIN

Phase 1:
insert/update all task rows

Phase 2:
insert dependency edges

Phase 3:
insert attempts if applicable

COMMIT

This avoids requiring Plan stage order to happen to match dependency
order.

Example:

Plan order:

S003
S001
S002

should still persist correctly if:

S003 depends on S002.

---

## Step 12 — Failure Must Roll Back

Test intentionally invalid persistence.

Verify:

SaveTasks(...)
↓
error
↓
ROLLBACK

Then:

store.List()

must show the database unchanged.

---

## Step 13 — Decide Existing Task Behavior

Do not silently destroy existing execution state.

If generated Task IDs already exist in the project database, T005
should fail for V1.

Example:

SQLite:
S001 already exists

New Plan:
S001

Result:

error: task S001 already exists

Do not overwrite:

Status
Attempts
BlockedReason
execution history

Later we can design explicit plan-update semantics.

---

## Step 14 — Application Operation

Create an application-level flow:

Plan
↓
Build Tasks
↓
Validate DAG
↓
Persist Tasks

Keep this separate from the low-level graph functions.

Conceptually:

CreateTasksFromPlan(plan)

The operation coordinates components.

It does not contain graph algorithms itself.

---

## Step 15 — CLI Boundary

Do not make:

agent-sdlc plan

automatically persist Tasks yet.

Keep:

agent-sdlc plan
↓
PLAN.md

Add a separate command:

agent-sdlc tasks

or preferably:

agent-sdlc build

However, for T005 I'd prefer:

agent-sdlc tasks

because it says exactly what this stage does.

Conceptually:

agent-sdlc tasks
↓
generate/persist Tasks from plan representation

But there is an architectural issue:

PLAN.md is human output, not machine source of truth.

Therefore do NOT parse PLAN.md.

---

## Step 16 — Preserve Machine Plan

T004 currently holds Plan only in memory and writes PLAN.md.

We now need a durable machine representation.

Add:

.agent-sdlc/plan.json

Flow becomes:

PRD.md
↓
Planner
↓
Plan
├────→ PLAN.md
│
└────→ .agent-sdlc/plan.json

PLAN.md:
human-readable

plan.json:
machine-readable

This is the correct boundary for T005.

---

## Step 17 — Update `agent-sdlc plan`

After successful plan generation:

Plan
├── RenderMarkdown()
│ ↓
│ PLAN.md
│
└── json.Marshal()
↓
.agent-sdlc/plan.json

Both outputs represent the same validated Plan.

Do not regenerate one from the other.

---

## Step 18 — Atomic Plan Output

Avoid:

PLAN.md written
plan.json fails

Prefer generating both payloads first.

Then write carefully so failure does not leave inconsistent plan
artifacts.

At minimum, write temporary files and rename them after both writes
succeed.

Do not overwrite existing PLAN.md without explicit behavior.

---

## Step 19 — `agent-sdlc tasks`

Flow:

.agent-sdlc/plan.json
│
▼
Decode
│
▼
Plan.Validate()
│
▼
Builder
│
▼
DAG Validate
│
▼
SaveTasks()
│
▼
SQLite

The command should not invoke the LLM.

---

## Step 20 — CLI Safety

`agent-sdlc tasks` should fail when:

- project is not initialized
- plan.json missing
- plan.json malformed
- Plan invalid
- graph cyclic
- Tasks already exist
- persistence fails

Usage errors:

exit 2

Runtime/data errors:

exit 1

Success:

exit 0

---

## Step 21 — Builder Tests

Test:

one stage
multiple stages
dependency preservation
acceptance criteria serialization
initial status
attempt defaults
timestamp initialization
stable ordering

No SQLite required.

---

## Step 22 — Graph Tests

Test:

single node

linear:
A → B → C

diamond:
A
/ \
B C
\ /
D

direct cycle:
A → B → A

long cycle:
A → B → C → A

self-cycle should already be rejected by Plan validation.

---

## Step 23 — Store Tests

Test:

batch insert
dependency edges
arbitrary task order
rollback
existing-ID protection
round trip

Especially verify:

Task A depends on Task B

even when A appears before B in the batch.

---

## Step 24 — CLI Integration Test

Temporary project:

PRD.md
↓
fake agent
↓
agent-sdlc plan
↓
PLAN.md
.agent-sdlc/plan.json
↓
agent-sdlc init
↓
agent-sdlc tasks
↓
SQLite
↓
agent-sdlc status

Verify expected Tasks exist.

No real model.

---

## Step 25 — Quality Gates

Run:

go test ./internal/taskbuilder/...
go test ./internal/planner/...
go test ./internal/store/...
go test ./internal/cli/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check

---

## Step 26 — Review

Self-review for:

- accidental LLM call
- parsing PLAN.md
- cycle detection errors
- partial persistence
- FK ordering bugs
- existing Task overwrite
- unstable ordering
- scheduler logic leaking into T005
- workflow transitions occurring during creation
- duplicate graph representation
- unnecessary abstractions

Then OCR review.

---

## Acceptance Criteria

- [x] Plan stages map deterministically to Tasks.
- [x] Stage IDs become Task IDs.
- [x] Dependencies are preserved.
- [x] Acceptance criteria are preserved.
- [x] New Tasks start PLANNED.
- [x] Retry defaults are initialized.
- [x] DAG cycles are rejected.
- [x] Valid diamond graphs pass.
- [x] Task order is deterministic.
- [x] Batch persistence is atomic.
- [x] Dependency insertion works regardless of input order.
- [x] Existing Tasks are not silently overwritten.
- [x] T004 persists `.agent-sdlc/plan.json`.
- [x] PLAN.md remains human-facing.
- [x] PLAN.md is never parsed for execution.
- [x] `agent-sdlc tasks` consumes plan.json.
- [x] T005 makes no LLM calls.
- [x] No scheduling behavior is introduced.
- [x] Tests require no model/network.
- [x] OCR review completed.
- [x] CI passes.
