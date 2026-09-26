# SOP

SOP — **a standard operating procedure for agentic software development.**

SOP is a local-first orchestrator for running a software development lifecycle with AI agents while keeping workflow control deterministic.

> **Agents do the work. SOP controls the process.**

The basic idea is:

> **Agents do the reasoning and implementation work. The orchestrator decides what is allowed to happen next.**

Instead of giving an AI coding agent a project and allowing it to operate without structure, SOP turns development into a controlled workflow:

```text
PRD
 ↓
Plan
 ↓
Task Breakdown
 ↓
Dependency Graph
 ↓
Pick Ready Task
 ↓
Create Branch
 ↓
Write Tests
 ↓
Implement
 ↓
Run Tests
 ↓
Review
 ↓
Fix Findings
 ↓
Pull Request
 ↓
CI
 ↓
Merge
 ↓
Next Task
```

SOP is written in Go and is designed to work with different AI harnesses and model providers.

---

# Why SOP?

Coding agents are good at:

- understanding requirements
- generating plans
- writing code
- writing tests
- reviewing code
- diagnosing failures
- fixing problems

They should not necessarily be the source of truth for workflow state.

SOP separates those responsibilities.

```text
                 SOP
                      │
             ┌────────┴────────┐
             │                 │
      Deterministic         AI Agents
         Control
             │                 │
             ▼                 ▼
        Task State          Planning
        Dependencies        Coding
        Validation          Reviewing
        Scheduling          Fixing
        Retry Limits        Reasoning
        Persistence         Documentation
        Git / CI State
```

The orchestrator owns workflow control.

Agents operate inside those boundaries.

---

# Current Status

SOP V1 is complete. A small project can start from a PRD and reach a
completed `main` branch through dependency-aware task execution, isolated
branches, test-first implementation, bounded review/fix loops, GitHub CI, a
validated merge gate, durable state, resume/recovery, environment bootstrapping,
and limited parallelism.

Implemented:

- Go CLI (`sop`)
- SQLite project state with migrations
- explicit workflow state machine and bounded retry policy
- PRD → implementation plan generation (`PLAN.md`, `.agent-sdlc/plan.json`)
- Plan → Task DAG with cycle detection and atomic persistence
- deterministic, dependency-aware scheduler
- Git adapter (branch and worktree lifecycle; structured arguments, no shell)
- test runner (build / unit / integration / lint / Docker)
- provider-independent Agent harness and command adapter
- local model providers (Ollama, OpenAI-compatible llama.cpp)
- project configuration (`.agent-sdlc/config.yaml`) with schema validation, defaults, and a generated template; the configuration selects the agent provider (the environment overrides it)
- Markdown task-file loader (`sop plan TASK.md`)
- deterministic quality gate (`PASS` / `FAIL` / `NEEDS_HUMAN`) from verification, findings, and the fix-loop budget
- command policy (`SAFE` / `REQUIRES_APPROVAL` / `DENIED`) whose project rules can only tighten the defaults
- TDD task runner (RED/GREEN with bounded retries)
- structured self-review and optional Open Code Review adapter
- commit and documentation gate
- GitHub adapter (push, pull requests, checks, merge)
- GitHub Actions workflow generation and a bounded CI remediation loop
- merge gate (tests + review + checks + mergeability)
- completion loop
- resume and recovery (`sop resume`)
- environment bootstrap tasks
- limited parallelism with Git worktrees
- task handoff capsules with optional, provider-independent compression

---

# Using SOP With Another Project

SOP is designed to operate **on other projects**.

It does not need to be copied into the repository it manages.

SOP is written in Go, but it manages projects written in any language — Go, Java, Kotlin, Python, JavaScript/TypeScript, Rust, C/C++, C#, or a mix of them. The project being managed does not need to be written in Go.

For example:

```text
~/workspace/
│
├── agentic-sdlc/             # SOP source
│
└── projects/
    ├── book-rag/             # Project being managed
    ├── medical-billing/
    └── inventory/
```

Install SOP once and run it from inside whichever project you want it to manage.

The relationship looks like this:

```text
sop                            your-project
     │                              │
     │ operates on ────────────────►│
     │                              │
     │                           PRD.md
     │                           PLAN.md
     │                           source/
     │                           .git/
     │                              │
     │                           .agent-sdlc/
     │                             state.db
     │                             plan.json
```

The target project remains a normal Git repository.

SOP adds orchestration around it.

---

# Installation

## Requirements

You currently need:

- Go
- Git
- an AI command or harness for planning

SQLite support is provided through the Go SQLite driver.

## Clone SOP

```bash
git clone https://github.com/imhttran/agentic-sdlc.git
cd agentic-sdlc
```

## Install the CLI

```bash
go install ./cmd/sop
```

Make sure your Go binary directory is on your `PATH`.

Typically:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

You can add that to your shell configuration if necessary.

For zsh:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Verify the installation:

```bash
sop version
```

You can now run `sop` from another project directory.

---

# Quick Start

Suppose you want SOP to manage:

```text
~/workspace/projects/book-rag
```

Move into that project:

```bash
cd ~/workspace/projects/book-rag
```

The basic workflow currently is:

```bash
sop init
sop plan
sop tasks
sop status
```

Conceptually:

```text
PRD.md
  │
  ▼
sop plan
  │
  ├────────→ PLAN.md
  │
  └────────→ .agent-sdlc/plan.json
                       │
                       ▼
               sop tasks
                       │
                       ▼
                 Task DAG
                       │
                       ▼
                    SQLite
                       │
                       ▼
               sop status
```

The following sections walk through this process.

---

# 1. Initialize the Project

From the target project:

```bash
sop init
```

This initializes SOP state.

Your project will contain:

```text
book-rag/
├── .git/
├── source...
│
└── .agent-sdlc/
    └── state.db
```

`state.db` contains durable workflow state for the project.

SOP treats the current working directory as the project root.

---

# 2. Create a PRD

Create:

```text
PRD.md
```

in the project root.

For example:

```markdown
# Book RAG

Build an application that allows users to upload books and ask
questions about their contents.

## Requirements

- Go backend
- book ingestion
- text chunking
- embeddings
- hybrid retrieval
- LLM question answering
- citations

## Initial Constraints

- single application
- local development
- SQLite where practical
- Ollama-compatible model support
```

The PRD is the starting point for the planning workflow.

```text
PRD
 │
 ▼
What are we building?
```

---

# 3. Configure an AI Agent

SOP does not directly depend on a specific model provider.

Instead it communicates through an Agent interface.

The current implementation provides a command-backed Agent.

Configure the command using:

```bash
export SOP_AGENT_COMMAND="your-agent-command"
```

The pre-rename name `AGENT_SDLC_AGENT_COMMAND` is still accepted for backward compatibility.

The configured command receives a JSON request through `stdin` and returns its response through `stdout`.

Conceptually:

```text
SOP
      │
      ▼
 CommandAgent
      │
      ▼
 external command
      │
      ▼
 AI harness / model
```

This keeps the core application independent of:

- Ollama
- OpenAI-compatible APIs
- Claude
- local models
- cloud models
- coding-agent harnesses

The application depends only on the Agent contract.

---

# 4. Generate the Implementation Plan

Run:

```bash
sop plan
```

The flow is:

```text
PRD.md
   │
   ▼
 Planner
   │
   ▼
 Agent
   │
   ▼
 AI Model
   │
   ▼
 JSON Response
   │
   ▼
 Plan
   │
   ▼
 Validate
   │
   ├─────────────→ PLAN.md
   │
   └─────────────→ .agent-sdlc/plan.json
```

The model does **not** directly write the plan files.

The model generates structured content.

The application:

1. parses it
2. validates it
3. converts it into a typed Plan
4. renders Markdown
5. writes the artifacts

---

# Human vs Machine Plan

Planning produces two representations:

```text
                  Plan
                 /    \
                /      \
               ▼        ▼
          PLAN.md     plan.json
           human       machine
```

## `PLAN.md`

Intended for humans.

Example:

```markdown
# Implementation Plan

## Project

Book RAG

## S001 — Application Skeleton

Create the initial Go application.

### Dependencies

None

### Deliverables

- Go application
- configuration

### Acceptance Criteria

- application starts successfully
```

## `.agent-sdlc/plan.json`

Intended for the orchestrator.

Example:

```json
{
  "project": "Book RAG",
  "summary": "Build a book question answering system.",
  "stages": [
    {
      "id": "S001",
      "title": "Application Skeleton",
      "objective": "Create the Go application.",
      "dependencies": [],
      "deliverables": ["Go application"],
      "acceptance_criteria": ["Application starts successfully"]
    }
  ]
}
```

SOP does not parse `PLAN.md` to determine executable workflow state.

Structured data is used instead.

---

# 5. Create the Task Graph

Once the plan exists, run:

```bash
sop tasks
```

SOP reads:

```text
.agent-sdlc/plan.json
```

and converts each plan stage into a task.

```text
Plan
 │
 ▼
Task Builder
 │
 ▼
Tasks
 │
 ▼
DAG Validation
 │
 ▼
SQLite
```

This operation is deterministic.

No LLM is called.

---

# Dependency Graph

Tasks contain explicit dependencies.

For example:

```text
        S001
       /    \
      ▼      ▼
    S002    S003
       \    /
        ▼  ▼
        S004
```

means:

```text
S002 depends on S001
S003 depends on S001
S004 depends on S002 and S003
```

SOP validates the graph before storing the task set.

Cycles are rejected.

For example:

```text
S001
 ↓
S002
 ↓
S003
 ↓
S001
```

is invalid.

---

# Atomic Task Creation

Task creation is transactional.

Suppose the plan contains:

```text
S001
S002
S003
S004
```

SOP will not leave:

```text
S001 ✓
S002 ✓
S003 ✗
S004 ?
```

if something fails.

Instead:

```text
BEGIN

create tasks
create dependency edges
create attempt state

success
   ↓
COMMIT

failure
   ↓
ROLLBACK
```

The task set is either persisted successfully or the database remains unchanged.

---

# Existing Task Protection

SOP does not silently overwrite existing workflow state.

If:

```text
S001
```

already exists in SQLite and a new task generation attempts to create `S001`, the operation fails.

This protects information such as:

- workflow status
- attempt count
- failure history
- blocked state
- future execution history

Updating an existing plan will eventually have its own explicit workflow.

---

# 6. Inspect Tasks

List the project's tasks:

```bash
sop status
```

Example:

```text
S001 PLANNED Application skeleton
S002 PLANNED Book ingestion
S003 PLANNED Embeddings
S004 PLANNED Retrieval
```

Inspect a specific task:

```bash
sop task S002
```

SQLite remains the durable source of truth for task execution state.

---

# CLI

Currently available commands:

```text
sop init
sop plan
sop tasks
sop status
sop task <id>
sop resume [task-id]
sop version
sop help
```

## `init`

Initialize SOP state:

```bash
sop init
```

## `plan`

Generate an implementation plan from `PRD.md`, or from a Markdown task file:

```bash
sop plan
sop plan TASK.md
```

A task file is parsed into ID, title, description, requirements, acceptance
criteria, and constraints, then normalized before the agent reasons over it.

Produces:

```text
PLAN.md
.agent-sdlc/plan.json
```

## `tasks`

Convert the machine-readable Plan into persisted tasks:

```bash
sop tasks
```

## `status`

Show project task state:

```bash
sop status
```

## `task`

Inspect a particular task:

```bash
sop task S001
```

## `resume`

Report the next legal action for interrupted work. With no id it resumes the
single in-flight task:

```bash
sop resume
sop resume S001
```

---

# Architecture

The architecture separates reasoning from control.

```text
                        CLI
                         │
                         ▼
                   Orchestrator
                         │
       ┌─────────────────┼─────────────────┐
       │                 │                 │
       ▼                 ▼                 ▼
    Planner          Scheduler         Task Runner
       │                 │                 │
       ▼                 │                 ▼
 Agent Harness            │             Git/Test
       │                  │             Review/CI
       ▼                  │
      LLM                 │
                          ▼
                     State Store
                          │
                          ▼
                        SQLite
```

Not every component shown above is implemented yet.

The architecture is being built incrementally.

---

# Core Design Rule

The central design rule is:

```text
The LLM creates content.

The application controls the process.
```

For example, during planning:

```text
LLM
 │
 │ proposes
 ▼
Plan JSON
 │
 │ validated by
 ▼
Go Application
 │
 ├── valid ──────→ continue
 │
 └── invalid ────→ stop
```

The LLM does not decide whether its own output is valid.

---

# Workflow State Machine

Tasks move through an explicit state machine.

```text
PLANNED
   ↓
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
   ↓
REVIEW
   ↓
REVIEW_PASS
   ↓
PR_OPEN
   ↓
CI_RUNNING
   ↓
CI_PASS
   ↓
MERGED
   ↓
DONE
```

Only legal state transitions are allowed.

For example:

```text
PLANNED → READY
```

is legal.

Something like:

```text
PLANNED → MERGED
```

is not.

---

# Remediation

Failures do not necessarily terminate the workflow.

Several states can move into:

```text
FIX_REQUIRED
```

For example:

```text
IMPLEMENTING ───────┐
LOCAL_TESTS_PASS ───┤
REVIEW ─────────────┼──→ FIX_REQUIRED
CI_RUNNING ─────────┘          │
                               ▼
                         IMPLEMENTING
```

Retries are bounded.

If recovery cannot continue safely, the task can eventually become:

```text
BLOCKED
```

`BLOCKED` and `DONE` are terminal states.

---

# Test-Driven Development

The intended implementation workflow follows TDD.

```text
Requirement
 ↓
Acceptance Criteria
 ↓
Write Test
 ↓
Verify RED
 ↓
Implement
 ↓
Verify GREEN
 ↓
Refactor
 ↓
Run Affected Suite
```

A task should not claim successful TDD simply because tests pass.

Where applicable, the system should verify that the new test initially failed before implementation.

If RED verification is not applicable, that should be explicitly recorded rather than fabricated.

---

# Review Pipeline

The intended review pipeline contains multiple layers.

```text
Implementation
      │
      ▼
 Local Tests
      │
      ▼
 Self Review
      │
      ▼
 Open Code Review
      │
      ▼
 Fix Findings
      │
      ▼
 Pull Request
      │
      ▼
 CI
```

Self-review focuses on:

- acceptance criteria
- correctness
- tests
- edge cases
- error handling
- architecture boundaries
- unnecessary complexity

External automated review provides an independent second pass.

Correctness findings should be fixed.

Suggestions that only add unnecessary complexity can be declined.

---

# Git Workflow

The intended task workflow uses predictable Git naming.

Branches:

```text
task/<id>-<slug>
```

Example:

```text
task/S004-hybrid-retrieval
```

Commits:

```text
task(<id>): implement <feature>
```

Example:

```text
task(S004): implement hybrid retrieval
```

Pull requests:

```text
[Task <id>] <title>
```

Example:

```text
[Task S004] Implement hybrid retrieval
```

Git automation is part of the upcoming execution stages.

---

# Scheduler

The next major component is the scheduler.

The scheduler will inspect persisted task state and determine which tasks are ready.

Example:

```text
       S001 DONE
       /       \
      ▼         ▼
S002 PLANNED  S003 PLANNED
      \         /
       ▼       ▼
         S004
```

Because `S001` is complete:

```text
S002 → READY
S003 → READY
```

But:

```text
S004 → PLANNED
```

because its dependencies have not completed.

The scheduler will answer:

> Given the current task states and dependency graph, which tasks are legally allowed to run now?

That decision remains deterministic.

---

# Future Execution

Eventually the normal workflow should become:

```bash
cd my-project

sop init
sop plan
sop tasks
sop run
```

`run` will drive the lifecycle.

The orchestration components behind it already exist (task runner, review,
commit gate, GitHub/CI adapters, CI remediation, merge gate, completion loop,
resume, parallelism); `run` is the remaining CLI wiring that composes them.

```text
                    Task DAG
                       │
                       ▼
                  Scheduler
                       │
                       ▼
                 Ready Task
                       │
                       ▼
               Create Branch
                       │
                       ▼
                 Write Tests
                       │
                       ▼
                  Verify RED
                       │
                       ▼
                  Implement
                       │
                       ▼
                 Run Tests
                       │
                       ▼
                    Review
                       │
                 ┌─────┴─────┐
                 │           │
               PASS      FIX_REQUIRED
                 │           │
                 │           └──────┐
                 ▼                  │
              Commit                │
                 │                  │
                 ▼                  │
             Pull Request           │
                 │                  │
                 ▼                  │
                  CI                │
                 │                  │
          ┌──────┴──────┐           │
          │             │           │
        PASS           FAIL ─────────┘
          │
          ▼
        Merge
          │
          ▼
        DONE
          │
          ▼
     Next Ready Task
```

---

# Controlled Parallelism

The dependency DAG eventually allows independent tasks to run concurrently.

For example:

```text
         S001
        /    \
       ▼      ▼
     S002    S003
       │      │
       └──┬───┘
          ▼
         S004
```

After `S001` completes:

```text
S002
```

and:

```text
S003
```

could potentially execute concurrently.

Parallel execution will remain bounded rather than allowing unrestricted agent spawning.

The initial target is a small configurable concurrency limit.

---

# Persistence and Recovery

Workflow state is stored in SQLite:

```text
.agent-sdlc/
└── state.db
```

This is important because an agentic workflow may run much longer than a single process.

If execution stops:

```text
Task A DONE
Task B DONE
Task C IMPLEMENTING
Task D PLANNED
```

the system should be able to restart and determine where it left off.

Future commands include:

```text
sop run
sop resume
sop retry <task>
```

The database, rather than model conversation history, is the source of truth.

---

# Development Principles

## Deterministic Control, Agentic Execution

Use AI where reasoning is useful.

Use normal software where correctness and state management matter.

Good AI responsibilities:

```text
Planning
Coding
Review
Failure Analysis
Documentation
```

Good deterministic responsibilities:

```text
State Transitions
Dependency Validation
Scheduling
Persistence
Retry Limits
Git State
CI State
Merge Gates
```

---

## Machine State Is Not Markdown

Markdown is useful for people.

Structured data is used for executable state.

```text
Human                         Machine

PRD.md
PLAN.md          ←──────→     plan.json
                              tasks
                              dependencies
                              attempts
                              SQLite
```

The orchestrator should not depend on parsing human documentation to understand its own state.

---

## Bounded Autonomy

Agents should not retry indefinitely.

For example:

```text
Attempt 1
   ↓
fail
   ↓
Attempt 2
   ↓
fail
   ↓
Attempt 3
   ↓
fail
   ↓
BLOCKED
```

The exact behavior depends on the workflow stage, but retries must have explicit limits.

---

## Recoverability

The system should be able to answer:

```text
What happened?

What state are we in?

What should happen next?
```

after a crash, process restart, agent failure, CI failure, or human interruption.

---

## Local First

The orchestrator runs locally.

The architecture should support:

- local models
- cloud models
- local Git
- local development environments
- local SQLite state
- external CI where appropriate

A cloud service should not be required simply to run the orchestrator.

---

## Provider Independence

Application components should depend on small interfaces rather than model-provider SDKs.

For example:

```go
type Agent interface {
    Generate(
        ctx context.Context,
        request Request,
    ) (Response, error)
}
```

This allows model providers and agent harnesses to change without rewriting the planning or orchestration logic.

---

# Testing

AI boundaries should be replaced with deterministic fakes in unit tests.

Tests should not require:

- a running LLM
- API keys
- network access
- cloud services

Run the test suite:

```bash
go test ./...
```

Run race detection:

```bash
go test -race ./...
```

Verify operation without CGO:

```bash
CGO_ENABLED=0 go test ./...
```

Run all project checks:

```bash
make check
```

---

# Development Roadmap

Current progress:

```text
✓ Repository / CI
✓ Domain Model
✓ Workflow State Machine
✓ SQLite Persistence
✓ Persistence Corrections
✓ CLI Foundation
✓ PRD → Plan
✓ Plan → Tasks
✓ Dependency DAG

→ Scheduler
  Git Adapter
  Test Runner
  Agent Harness
  TDD Task Runner
  Structured Self Review
  Open Code Review Integration
  Commit / Documentation Gate
  GitHub Adapter
  GitHub Actions Integration
  CI Remediation
  Merge Gate
  Completion Loop
  Resume / Recovery
  Environment Bootstrap
  Controlled Parallel Execution
  Documentation Automation
  End-to-End Dogfooding
```

The project is intentionally being built incrementally.

Sequential correctness and recovery come before parallel execution.

---

# Target End-to-End Workflow

The eventual workflow is:

```text
PRD
 ↓
PLAN
 ↓
TASK DAG
 ↓
┌─────────────────────────────────┐
│          Orchestrator           │
│                                 │
│  Pick Ready Task                │
│          ↓                      │
│  Create Branch                  │
│          ↓                      │
│  Write Tests                    │
│          ↓                      │
│  Verify RED                     │
│          ↓                      │
│  Implement                      │
│          ↓                      │
│  Verify GREEN                   │
│          ↓                      │
│  Review                         │
│          ↓                      │
│  Fix Findings                   │
│          ↓                      │
│  Commit                         │
│          ↓                      │
│  Pull Request                   │
│          ↓                      │
│  CI                             │
│          ↓                      │
│  Fix CI Failures                │
│          ↓                      │
│  Merge                          │
│          ↓                      │
│  Update State / Documentation   │
└────────────────┬────────────────┘
                 │
                 ▼
           Next Ready Task
                 │
                 ▼
               DONE
```

---

# What SOP Is Not

SOP is not intended to be an unrestricted autonomous coding agent.

It is not:

```text
Prompt
  ↓
LLM
  ↓
"Go change whatever you want"
```

Instead:

```text
             Orchestrator
                  │
          defines boundaries
                  │
                  ▼
                Agent
                  │
              performs
           specific work
                  │
                  ▼
             Validation
                  │
                  ▼
            State Change
```

The orchestrator remains responsible for deciding whether the workflow can continue.

---

# Project Goal

SOP explores a practical question:

> **How much of a real software development lifecycle can be automated with AI while keeping control, state, safety, and recovery deterministic?**

The goal is a software engineering workflow where AI agents operate inside explicit boundaries and the system can always answer:

```text
What happened?

What state are we in?

What can happen next?

What failed?

Can we retry?

Do we need a human?
```

That is the foundation for running long-lived agentic software development workflows without giving up control of the SDLC.

---

# License

Apache License 2.0
