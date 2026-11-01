# T003 — CLI Foundation

## Status

DONE

## Goal

Create the first usable `agent-sdlc` command-line interface on top of
the existing domain and SQLite store.

Commands for T003:

agent-sdlc --help
agent-sdlc version
agent-sdlc init
agent-sdlc status
agent-sdlc task <task-id>

Do not implement workflow execution yet.

---

## Starting Point

T002B is merged.

Verify:

git checkout main
git pull
go test ./...
make check

Working tree must be clean before starting.

---

## Branch

Create:

task/T003-cli-foundation

---

## Architecture

Keep command parsing separate from process startup.

Desired structure:

cmd/
└── agent-sdlc/
└── main.go

internal/
├── cli/
│ ├── cli.go
│ ├── init.go
│ ├── status.go
│ ├── task.go
│ └── cli_test.go
├── domain/
└── store/

main.go should only handle:

OS arguments
↓
cli.Run(...)
↓
exit code

Business/database behavior belongs outside main.go.

---

## CLI Contract

Use:

Run(args, stdout, stderr) int

or an equivalent testable interface.

Avoid tests that require spawning a real process for every command.

Tests should be able to do:

args
│
▼
Run(...)
│
├── stdout buffer
├── stderr buffer
└── exit code

---

## Exit Codes

Use predictable exit behavior:

0 = success
1 = runtime/application failure
2 = invalid command or arguments

Examples:

agent-sdlc banana
→ exit 2

agent-sdlc task
→ exit 2

agent-sdlc task T999
→ exit 1

agent-sdlc status before init
→ exit 1

---

## Step 1 — CLI Dispatcher

TDD first.

Write tests for:

agent-sdlc --help
agent-sdlc version
unknown command

Verify RED.

Implement the smallest dispatcher necessary.

Do not add Cobra or another CLI framework.

Use standard Go unless complexity clearly requires otherwise.

---

## Step 2 — Root Help

Implement:

agent-sdlc --help

and preferably:

agent-sdlc help

Output should identify:

init
status
task
version

Keep output deterministic so it can be tested exactly or by stable
substrings.

---

## Step 3 — Version Command

Implement:

agent-sdlc version

Keep version handling simple.

A package-level version value such as:

dev

is enough for T003.

Do not build a release/versioning system.

---

## Step 4 — Project State Location

For T003 define:

current working directory = project root

State lives at:

<cwd>/.agent-sdlc/state.db

Do not implement upward Git-root discovery yet.

Provide one small function responsible for resolving the state path.

Avoid scattering:

".agent-sdlc/state.db"

through multiple commands.

---

## Step 5 — Init Command

TDD first.

Test:

agent-sdlc init

Expected behavior:

project/
└── .agent-sdlc/
└── state.db

Flow:

init
↓
resolve cwd
↓
create .agent-sdlc
↓
store.Open(state.db)
↓
migration
↓
close
↓
success

The command must be idempotent.

Run init twice.

Existing database contents must not be deleted or reset.

---

## Step 6 — Init Preservation Test

Prove idempotency behavior.

Test sequence:

init
↓
open DB
↓
save Task
↓
close
↓
init again
↓
open DB
↓
Get(Task)
↓
Task still exists

This is more useful than merely checking that the second init returns
success.

---

## Step 7 — Status Command

Implement:

agent-sdlc status

It should:

resolve state.db
↓
open Store
↓
Store.List()
↓
format tasks
↓
stdout

Example output:

T001 DONE Domain model and state machine
T002 DONE SQLite persistence
T003 IN_PROGRESS CLI foundation

Keep formatting simple.

No:

- colors
- progress bars
- TUI
- third-party table library

Deterministic text is preferable.

---

## Step 8 — Uninitialized Project Handling

Do not allow:

agent-sdlc status

to silently create a new database.

This distinction matters:

init
↓
allowed to create state

status/task
↓
state must already exist

Before opening the store for read commands, verify that the expected
state database exists.

Return a useful error such as:

project is not initialized; run `agent-sdlc init`

Exit:

1

This prevents read commands from accidentally mutating project state.

---

## Step 9 — Task Command

Implement:

agent-sdlc task T001

Flow:

task T001
↓
Store.Get("T001")
↓
Task
↓
format
↓
stdout

Display useful persisted state:

Task: T001
Title: ...
Status: ...
Attempts: ...
Dependencies:
T000

Do not add task mutation commands yet.

---

## Step 10 — Task Error Cases

Test:

agent-sdlc task
→ missing ID
→ exit 2

agent-sdlc task T999
→ not found
→ exit 1

agent-sdlc task T001 extra
→ invalid arguments
→ exit 2

Errors should go to stderr.

Normal command output should go to stdout.

---

## Step 11 — Store Lifecycle

Every command that opens a Store must close it.

Pattern:

Open
↓
operation
↓
Close

Avoid keeping a global database connection.

T003 commands are short-lived CLI operations.

---

## Step 12 — Test Filesystem Isolation

Use:

t.TempDir()

Tests must never write:

.agent-sdlc/

into the repository being tested.

Each test should operate inside an isolated temporary project directory.

Restore working directory if a test changes it.

Prefer dependency injection for project path if that makes tests cleaner.

Do not overengineer a filesystem abstraction.

---

## Step 13 — CLI Integration Test

Add at least one test that exercises the real command flow:

temporary project
↓
init
↓
SQLite created
↓
seed Task
↓
status
↓
expected Task printed

This verifies:

CLI → Store → SQLite → CLI

without requiring Git/GitHub.

---

## Step 14 — Keep Workflow Logic Out

T003 must not introduce:

planner
scheduler
task execution
state transitions
retry loops
agents
Git branches
GitHub PRs
CI polling
OCR invocation
parallel execution

Those belong to later tasks.

The desired boundary is:

CLI
│
│ asks
▼
Application / Store
│
▼
Domain

not:

CLI
│
├── decides workflow
├── schedules tasks
├── changes state directly
└── retries operations

---

## Step 15 — Full Verification

Run:

go test ./internal/cli/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check

All must pass.

---

## Step 16 — Manual Smoke Test

From a temporary directory:

agent-sdlc --help

agent-sdlc init

agent-sdlc status

agent-sdlc task T999

agent-sdlc init

Confirm:

- help works
- database created
- empty status works
- missing task handled
- second init preserves state

---

## Step 17 — Self Review

Check specifically for:

- business logic in main.go
- read commands accidentally creating databases
- database connections not closed
- inconsistent exit codes
- stdout/stderr mixing
- filesystem writes outside test temp directories
- hard-coded working directories
- unnecessary CLI framework
- workflow logic creeping into CLI
- unrelated refactors

Keep the implementation small.

---

## Step 18 — OCR Review

OCR review is required.

Run the configured review against the T003 diff.

Use:

gpt-oss:120b-cloud

Focus review on:

- CLI boundary
- error handling
- exit codes
- filesystem safety
- SQLite lifecycle
- test isolation
- accidental state creation
- unnecessary complexity

Fix correctness issues.

Decline suggestions that add complexity without improving T003's
requirements.

After changes:

go test ./...
make check

---

## Step 19 — Commit

Suggested commit:

task(T003): add CLI foundation

---

## Step 20 — Push / PR

Push:

task/T003-cli-foundation

PR title:

[Task T003] Add CLI foundation

PR should explain:

- commands introduced
- state location
- exit-code contract
- init idempotency
- status/task read behavior
- test strategy
- what was intentionally deferred

---

## Step 21 — CI

CI must pass.

Maximum remediation attempts:

3

Each remediation:

CI failure
↓
diagnose
↓
smallest fix
↓
local tests
↓
push
↓
CI

After 3 unsuccessful attempts, stop rather than continuing blindly.

---

## Acceptance Criteria

- [x] `agent-sdlc --help` works.
- [x] `agent-sdlc version` works.
- [x] `agent-sdlc init` creates `.agent-sdlc/state.db`.
- [x] `init` is idempotent.
- [x] Re-running `init` preserves persisted tasks.
- [x] `status` reads tasks through Store.List().
- [x] `task <id>` reads through Store.Get().
- [x] status/task do not create an uninitialized database.
- [x] Missing task returns exit 1.
- [x] Invalid arguments return exit 2.
- [x] Success returns exit 0.
- [x] Errors go to stderr.
- [x] Normal output goes to stdout.
- [x] main.go remains thin.
- [x] Tests use temporary project directories.
- [x] Store connections are closed.
- [x] No workflow/orchestration logic added.
- [x] race tests pass.
- [x] CGO-disabled tests pass.
- [x] make check passes.
- [x] OCR review completed.
- [x] GitHub CI passes.

---

## Definition of Done

A user can now enter a project and run:

agent-sdlc init
↓
.agent-sdlc/state.db

agent-sdlc status
↓
persisted task summary

agent-sdlc task T001
↓
persisted task details

The CLI is now a stable entry point for later capabilities:

                 agent-sdlc
                      │
        ┌─────────────┼─────────────┐
        │             │             │
       init         status         task
        │             │             │
        └─────────────┼─────────────┘
                      ▼
                    Store
                      │
                    SQLite

Later:

                 agent-sdlc
                      │
     ┌────────────────┼────────────────┐
     ▼                ▼                ▼
    plan             run             resume
     │                │                │
     └────────────────┼────────────────┘
                      ▼
                 Orchestrator

T003 builds the entry point.

It does not build the orchestrator.
