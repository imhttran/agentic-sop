# T001 — Domain Model and State Machine

## Status

DONE

## Goal

Define the deterministic heart of the system before integrating any LLM or
external services.

When complete:

- Task and TaskStatus types are defined
- Dependency model exists
- Attempt tracking exists
- Legal state transitions are enforced
- Retry policy is defined
- Blocked reasons are enumerated
- comprehensive state-transition tests exist
- state machine can be tested without Git, GitHub, or LLM

Do not implement agent execution, Git integration, GitHub API, SQLite, CLI
commands, or Planner logic in this task. Focus on the pure domain model and
state machine.

---

## Step 1 — Create Task Branch

Create:

`task/T001-state-machine`

### Acceptance

- branch exists
- current branch is `task/T001-state-machine`
- branch is based on current `main`

---

## Step 2 — Define TaskStatus Enum

Create:

`internal/domain/status.go`

Expose:

```go
type TaskStatus string

const (
    PLANNED       TaskStatus = "PLANNED"
    READY         TaskStatus = "READY"
    IN_PROGRESS   TaskStatus = "IN_PROGRESS"
    LOCAL_TESTS_PASS TaskStatus = "LOCAL_TESTS_PASS"
    REVIEW_PASS   TaskStatus = "REVIEW_PASS"
    PR_OPEN       TaskStatus = "PR_OPEN"
    CI_PASS       TaskStatus = "CI_PASS"
    CI_FAIL       TaskStatus = "CI_FAIL"
    DONE          TaskStatus = "DONE"
    BLOCKED       TaskStatus = "BLOCKED"
)
```

### Verification

```bash
grep -A 10 "type TaskStatus" internal/domain/status.go
```

---

## Step 3 — Define BlockedReason Enum

In `internal/domain/status.go`, add:

```go
type BlockedReason string

const (
    NO_REASON                   BlockedReason = ""
    DEPENDENCIES_INCOMPLETE     BlockedReason = "DEPENDENCIES_INCOMPLETE"
    RETRIES_EXHAUSTED           BlockedReason = "RETRIES_EXHAUSTED"
    CI_FAILURE_UNACTIONABLE     BlockedReason = "CI_FAILURE_UNACTIONABLE"
    TEST_DESIGN_FAILED          BlockedReason = "TEST_DESIGN_FAILED"
    IMPLEMENTATION_TIMEOUT      BlockedReason = "IMPLEMENTATION_TIMEOUT"
    REVIEW_UNRESOLVED           BlockedReason = "REVIEW_UNRESOLVED"
)
```

### Verification

Run:

```bash
go test ./internal/domain -v
```

---

## Step 4 — Define Task Type

Create:

`internal/domain/task.go`

Expose:

```go
type Task struct {
    ID              string
    Title           string
    Objective       string
    AcceptanceCriteria string
    Status          TaskStatus
    BlockedReason   BlockedReason
    Attempt         int
    MaxAttempts     int
    DependencyIDs   []string
    CreatedAt       time.Time
    UpdatedAt       time.Time
}
```

Additional methods:

- `CanTransitionTo(newStatus TaskStatus) bool`
- `IsReady() bool`
- `IsBlocked() bool`
- `IsDone() bool`
- `CanRetry() bool`

### Verification

```bash
go test ./internal/domain -v
```

---

## Step 5 — Define State Transition Rules

In `internal/domain/task.go`, implement:

```go
func (t *Task) CanTransitionTo(newStatus TaskStatus) bool
```

**Legal transitions:**

```text
PLANNED            -> READY
READY              -> IN_PROGRESS
IN_PROGRESS        -> LOCAL_TESTS_PASS
IN_PROGRESS        -> BLOCKED (with reason)
LOCAL_TESTS_PASS   -> REVIEW_PASS
LOCAL_TESTS_PASS   -> IN_PROGRESS
REVIEW_PASS        -> PR_OPEN
PR_OPEN            -> CI_PASS
PR_OPEN            -> CI_FAIL
CI_FAIL            -> IN_PROGRESS (if retries remain)
CI_FAIL            -> BLOCKED (if retries exhausted)
CI_PASS            -> DONE
BLOCKED            -> (no transitions)
DONE               -> (no transitions)
```

All other transitions are rejected.

### Verification

```bash
go test ./internal/domain -run TestStateTransitions -v
```

---

## Step 6 — Define Dependency Model

In `internal/domain/task.go`, add:

```go
type Dependency struct {
    TaskID           string
    RequiredStatus   TaskStatus
}
```

Add method to Task:

```go
func (t *Task) ResolveDependencies(tasks map[string]*Task) (unmet []Dependency, satisfied bool)
```

A task is ready only if:

1. All dependency tasks have status >= required status
2. No circular dependencies exist

### Verification

```bash
go test ./internal/domain -run TestDependencies -v
```

---

## Step 7 — Define Attempt Tracking

In `internal/domain/task.go`, add:

```go
type Attempt struct {
    Number      int
    Status      TaskStatus
    Reason      string
    Output      string
    Duration    time.Duration
    Timestamp   time.Time
}

type Task struct {
    // existing fields...
    Attempts    []Attempt
}
```

Add methods:

- `AddAttempt(status TaskStatus, reason string) error`
- `CanRetry() bool`
- `HasRetries() bool`
- `CurrentAttempt() int`

### Verification

```bash
go test ./internal/domain -run TestAttempts -v
```

---

## Step 8 — Define Retry Policy

Create:

`internal/domain/retry.go`

```go
type RetryPolicy struct {
    MaxAttempts     int
    BackoffFactor   float64
    MaxDelay        time.Duration
}

func (p *RetryPolicy) IsRetryable(task *Task) bool
func (p *RetryPolicy) NextDelay(attempt int) time.Duration
```

Default policy:

- MaxAttempts: 3
- BackoffFactor: 2.0
- MaxDelay: 5 minutes

### Verification

```bash
go test ./internal/domain -run TestRetryPolicy -v
```

---

## Step 9 — Write Comprehensive State Transition Tests

Create:

`internal/domain/task_test.go`

Test cases (at minimum):

```go
TestTransitionPLANNED_to_READY
TestTransitionREADY_to_IN_PROGRESS
TestTransitionIN_PROGRESS_to_LOCAL_TESTS_PASS
TestTransitionIN_PROGRESS_to_BLOCKED
TestTransitionLOCAL_TESTS_PASS_to_REVIEW_PASS
TestTransitionLOCAL_TESTS_PASS_to_IN_PROGRESS
TestTransitionREVIEW_PASS_to_PR_OPEN
TestTransitionPR_OPEN_to_CI_PASS
TestTransitionPR_OPEN_to_CI_FAIL
TestTransitionCI_FAIL_to_IN_PROGRESS_with_retries
TestTransitionCI_FAIL_to_BLOCKED_without_retries
TestTransitionCI_PASS_to_DONE
TestTransitionBLOCKED_rejects_all
TestTransitionDONE_rejects_all
TestInvalidTransition_READY_to_CI_PASS_rejected
TestInvalidTransition_CI_RUNNING_to_DONE_rejected
```

Each test must verify the transition is allowed or rejected as specified.

### Verification

```bash
go test ./internal/domain -v
```

All tests must pass.

---

## Step 10 — Test Dependency Resolution

Create:

`internal/domain/dependency_test.go`

Test cases:

```go
TestDependency_all_met
TestDependency_one_missing
TestDependency_partial_completion
TestDependency_circular_detected
TestDependency_empty
```

### Verification

```bash
go test ./internal/domain -v
```

---

## Step 11 — Test Retry Logic

Add tests to `internal/domain/task_test.go`:

```go
TestRetry_task_with_retries_remaining_can_retry
TestRetry_task_exhausted_blocked
TestRetry_attempt_tracking
TestRetry_policy_enforces_max_attempts
```

### Verification

```bash
go test ./internal/domain -v
```

---

## Step 12 — Establish Local Quality Gate

Run:

```bash
make check
```

All checks must pass.

---

## Step 13 — Final Verification

Verify all pieces integrate:

```bash
go test ./internal/domain -v
go vet ./...
go fmt ./...
```

No generated artifacts or test binaries should exist.

---

## Step 14 — Review

Perform lightweight self-review:

- Domain model is pure (no Git, GitHub, LLM dependencies)
- State machine is deterministic and testable
- All legal transitions are covered
- Invalid transitions are rejected
- Dependency resolution is correct
- Retry policy is enforced
- No future features implemented
- Tests pass and cover all major paths
- Code follows Go idioms
- No boilerplate or scaffolding

---

## Step 15 — Commit

Stage T001 changes.

Commit using:

```
task(T001): define domain model and state machine
```

---

## Step 16 — Push

Push:

```
task/T001-state-machine
```

to `origin`.

---

## Step 17 — Open Pull Request

Create PR:

```
[Task T001] Domain Model and State Machine
```

Target: `main`  
Source: `task/T001-state-machine`

PR description:

- purpose
- domain types added
- state transitions defined
- test coverage for state machine
- acceptance criteria
- known limitations, if any

---

## Step 18 — Validate CI

Wait for GitHub Actions.

If CI fails:

1. inspect failing job
2. identify root cause
3. make smallest appropriate fix
4. run `make check` locally
5. commit fix
6. push
7. wait for CI again

Maximum automatic remediation attempts: 3

---

## Step 19 — Merge

Merge only when:

- local checks pass
- CI passes
- acceptance criteria are satisfied

After merge:

1. update local `main`
2. verify T001 exists on `main`
3. mark T001 DONE
4. proceed to T002

---

# Expected Final Structure

```
agentic-sdlc/
├── internal/
│   ├── app/
│   │   ├── app.go
│   │   └── app_test.go
│   └── domain/
│       ├── status.go
│       ├── task.go
│       ├── task_test.go
│       ├── retry.go
│       ├── dependency_test.go
│       └── ...
├── cmd/
│   └── agent-sdlc/
│       └── main.go
├── .github/
├── go.mod
├── Makefile
└── .gitignore
```

---

# Definition of Done

T001 is DONE only when:

- [ ] task branch was used
- [ ] TaskStatus enum defined with all states
- [ ] BlockedReason enum defined
- [ ] Task type with all required fields
- [ ] State transition rules implemented and tested
- [ ] Dependency model works
- [ ] Attempt tracking works
- [ ] Retry policy defined
- [ ] comprehensive state-transition tests exist and pass
- [ ] `make check` passes
- [ ] PR was created
- [ ] CI passes
- [ ] changes were merged into `main`

---

# Explicitly Out of Scope

Do not implement:

- SQLite or any persistence
- Git integration or branches
- GitHub API or PR creation
- Agent/LLM integration
- CLI commands beyond what exists
- Planner or task generation
- Test runner or build commands
- Concurrent execution
- Docker or environment setup

Those belong to later tasks.
