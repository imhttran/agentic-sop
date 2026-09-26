# T001A — Align Workflow State Machine

## Status

DONE

## Goal

Make the Task state machine authoritative and align its states with the
workflow architecture required by the future orchestrator.

This is domain-model correction work.

Do not add orchestration, Git, agents, planning, scheduling, or CI
integration.

---

## Branch

task/T001A-align-workflow-state-machine

---

## Step 1 — Baseline

Start from current main.

Run:

go test ./...
make check

Confirm T003 and all persistence tests pass before modifying the domain.

---

## Step 2 — Add the Missing Workflow States

Update the Task status model to support:

PLANNED
READY
BRANCH_CREATED
TESTS_WRITTEN
RED_VERIFIED
IMPLEMENTING
LOCAL_TESTS_PASS
REVIEW
REVIEW_PASS
PR_OPEN
CI_RUNNING
CI_PASS
FIX_REQUIRED
MERGED
DONE
BLOCKED

Remove or migrate the old:

IN_PROGRESS
CI_FAIL

Do not leave ambiguous duplicate concepts unless compatibility requires
a deliberate migration.

---

## Step 3 — Define the Happy Path

Encode:

PLANNED
  → READY
  → BRANCH_CREATED
  → TESTS_WRITTEN
  → RED_VERIFIED
  → IMPLEMENTING
  → LOCAL_TESTS_PASS
  → REVIEW
  → REVIEW_PASS
  → PR_OPEN
  → CI_RUNNING
  → CI_PASS
  → MERGED
  → DONE

Tests must cover every legal transition.

---

## Step 4 — Define Remediation Paths

Support:

IMPLEMENTING
  → FIX_REQUIRED

LOCAL_TESTS_PASS
  → FIX_REQUIRED

REVIEW
  → FIX_REQUIRED

CI_RUNNING
  → FIX_REQUIRED

FIX_REQUIRED
  → IMPLEMENTING

BLOCKED should remain terminal for V1.

DONE should remain terminal.

Do not create arbitrary backward transitions.

---

## Step 5 — Introduce Authoritative Transition()

Add a controlled mutation operation conceptually like:

task.Transition(nextStatus)

It must:

1. validate the transition
2. reject illegal transitions
3. mutate Status only after validation
4. update UpdatedAt
5. return an error for illegal transitions

Example:

READY
 ↓
Transition(DONE)
 ↓
error

Status remains:

READY

This is the key fix.

---

## Step 6 — Test Failed Transition Atomicity

Test:

task.Status = READY

err := task.Transition(DONE)

Expected:

err != nil
task.Status == READY

An invalid transition must never partially mutate the Task.

---

## Step 7 — Keep CanTransitionTo()

CanTransitionTo() may remain as a query:

if task.CanTransitionTo(REVIEW) {
    ...
}

But workflow code should use:

task.Transition(REVIEW)

for mutation.

Conceptually:

CanTransitionTo()
      ↓
pure question

Transition()
      ↓
authoritative mutation

---

## Step 8 — Handle BLOCKED Explicitly

Do not allow callers to create inconsistent combinations such as:

Status = READY
BlockedReason = RETRIES_EXHAUSTED

Define domain behavior for entering BLOCKED.

Prefer a controlled operation such as:

task.Block(reason)

Requirements:

reason must not be NO_REASON

Result:

Status = BLOCKED
BlockedReason = supplied reason
UpdatedAt = now

BLOCKED remains terminal in V1.

---

## Step 9 — Preserve Persistence Compatibility

The SQLite store validates persisted status values.

Update persistence validation for the new state set.

Do not redesign the schema.

Schema version should remain 1 if only textual enum values are changing
and no structural migration is required.

---

## Step 10 — Check Existing Stored-State Compatibility

Current databases may contain:

IN_PROGRESS
CI_FAIL

Decide explicitly how these are handled.

For this development-stage project, prefer a small compatibility mapping
when loading:

IN_PROGRESS → IMPLEMENTING
CI_FAIL     → FIX_REQUIRED

Do not silently spread legacy values throughout the new domain model.

Keep compatibility at the persistence boundary.

---

## Step 11 — Update CLI Tests

T003 tests currently construct Tasks using IN_PROGRESS.

Update them to use the new states.

For example:

IN_PROGRESS

becomes:

IMPLEMENTING

The CLI itself should not gain workflow logic.

It only displays persisted state.

---

## Step 12 — Keep Retry Policy Separate

Do not expand RetryPolicy in T001A.

Retry policy answers:

"May we try again?"

State machine answers:

"What state may this Task enter?"

Those remain separate concepts.

---

## Step 13 — Keep Dependency Resolution Separate

Do not redesign dependency resolution.

The future scheduler will use dependencies to decide:

PLANNED
   ↓
READY

but T001A should not implement the scheduler.

---

## Step 14 — Full Domain Tests

Add table-driven tests covering every legal transition.

Also test representative illegal transitions:

PLANNED → DONE
READY → REVIEW
BRANCH_CREATED → CI_RUNNING
TESTS_WRITTEN → DONE
REVIEW → CI_PASS
CI_RUNNING → MERGED
DONE → READY
BLOCKED → IMPLEMENTING

Illegal transitions must:

return error
preserve original state

---

## Step 15 — Persistence Round Trip

Persist Tasks in several new states:

BRANCH_CREATED
RED_VERIFIED
REVIEW
CI_RUNNING
FIX_REQUIRED
MERGED

Load them again.

Verify exact state survives:

Domain
  ↓
SQLite
  ↓
Domain

---

## Step 16 — Legacy Persistence Tests

If compatibility mapping is implemented, test:

stored "IN_PROGRESS"
        ↓
load
        ↓
IMPLEMENTING

stored "CI_FAIL"
        ↓
load
        ↓
FIX_REQUIRED

Do not write legacy states back to new records.

---

## Step 17 — Run Quality Gates

Run:

go test ./internal/domain/...
go test ./internal/store/...
go test ./internal/cli/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check

Everything must pass.

---

## Step 18 — Review

Self-review the diff for:

- direct uncontrolled status mutation
- missing happy-path states
- illegal backward transitions
- inconsistent blocked state
- persistence validation
- legacy-state handling
- CLI regressions
- unnecessary orchestrator logic
- retry/scheduler scope creep

Then run the required OCR review.

Use:

gpt-oss:120b-cloud

Focus OCR on the state machine and invariants.

---

## Step 19 — Commit / PR

Branch:

task/T001A-align-workflow-state-machine

Commit:

task(T001A): align workflow state machine

PR:

[Task T001A] Align workflow state machine

---

## Acceptance Criteria

- [ ] Full workflow state vocabulary exists.
- [ ] Happy-path transitions are explicit.
- [ ] FIX_REQUIRED remediation path exists.
- [ ] DONE is terminal.
- [ ] BLOCKED is terminal.
- [ ] Transition() validates before mutation.
- [ ] Illegal transitions leave Task unchanged.
- [ ] UpdatedAt changes on successful transition.
- [ ] Blocked Tasks require a meaningful reason.
- [ ] CanTransitionTo() remains side-effect free.
- [ ] Store understands every new status.
- [ ] Legacy IN_PROGRESS/CI_FAIL are deliberately handled.
- [ ] SQLite schema does not require redesign.
- [ ] CLI displays new statuses without owning workflow logic.
- [ ] Existing T002/T003 behavior remains intact.
- [ ] race tests pass.
- [ ] CGO-disabled tests pass.
- [ ] make check passes.
- [ ] OCR review completed.
- [ ] GitHub CI passes.

## Definition of Done

The domain now owns workflow legality:

Orchestrator
     │
     │ Transition(REVIEW)
     ▼
    Task
     │
     ├── legal? ──no──→ error
     │
     └── yes
          ↓
       mutate
          ↓
      UpdatedAt
          ↓
        Store

The future orchestrator never needs to implement:

if current == X && next == Y ...

That knowledge belongs to the Task state machine.
