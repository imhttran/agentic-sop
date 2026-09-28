# SOP --- Execution Plan

> Historical task records live under [`docs/tasks/`](tasks/).

## Strategy

Build the orchestrator incrementally. Do not attempt full autonomous
parallel SDLC in the first implementation.

The progression is:

```text
V1  Plan + task state
 ↓
V2  Branch + TDD + local verification
 ↓
V3  Review + PR + CI + retry + merge
 ↓
V4  Parallel scheduling + full completion loop
```

Each stage should be usable and testable before adding the next.

# Stage 0 --- Repository and Development Environment

**Status:** DONE — T000

## Goal

Create the Go project and a reproducible development environment.

## Deliverables

```text
go.mod
cmd/sop/
internal/
Dockerfile
docker-compose.yml
Makefile
.env.example
.github/workflows/ci.yml
```

Add basic commands:

```text
make test
make build
```

## Acceptance

- application builds;
- unit tests run;
- Docker image builds;
- GitHub CI runs the base test suite.

# Stage 1 --- Domain Model and State Machine

**Status:** DONE — T001, T001A

## Goal

Define the deterministic heart of the system before integrating any LLM.

## Implement

- Task;
- TaskStatus;
- Dependency;
- Attempt;
- legal state transitions;
- retry policy;
- blocked reasons.

## TDD Focus

Write state-transition tests first.

Examples:

```text
PLANNED -> READY allowed
READY -> CI_PASS rejected
CI_RUNNING -> CI_PASS allowed
retry exhausted -> BLOCKED
```

## Acceptance

The state machine can be tested without Git, GitHub, or an LLM.

# Stage 2 --- SQLite State Store

**Status:** DONE — T002, T002A, T002B

## Goal

Persist workflow state.

## Implement

- schema/migrations;
- project repository;
- task repository;
- dependencies;
- attempts;
- transition history.

## Acceptance

A process can stop and restart without losing task state.

# Stage 3 --- CLI Skeleton

**Status:** DONE — T003 (CLI renamed to `sop` in T005A)

## Goal

Expose the workflow to the user.

## Commands

```text
sop init
sop status
sop task <id>
sop retry <id>
```

`run`, `plan`, and `resume` may initially be placeholders wired
incrementally.

## Acceptance

A user can initialize a repository and inspect persisted state.

# Stage 4 --- PRD → Plan

**Status:** DONE — T004

## Goal

Introduce the first agentic boundary.

## Implement

Planner interface:

```text
CreatePlan(PRD, project context) -> structured plan
```

Generate:

```text
PLAN.md
```

Preserve a structured representation for task generation.

## Acceptance

A supplied PRD produces a staged engineering plan that can be inspected
before execution.

# Stage 5 --- Plan → Task DAG

**Status:** DONE — T005

## Goal

Turn the plan into executable units.

## Task fields

```text
ID
title
objective
acceptance criteria
dependencies
test expectations
status
```

Generate human-readable:

```text
TASKS.md
```

Persist machine state in SQLite.

## Acceptance

Ready tasks can be calculated from dependency state.

# Stage 6 --- Scheduler

**Status:** DONE — T006

## Goal

Select work deterministically.

## V1

Run one task at a time.

## Implement

- ready-task query;
- dependency checks;
- priority/order;
- no-work/completed/blocked outcomes.

## Acceptance

The scheduler never selects a task whose dependencies are incomplete.

# Stage 7 --- Git Adapter

**Status:** DONE — T007, T007A

## Goal

Make branch lifecycle deterministic.

## Implement

- repository validation;
- clean/dirty status;
- current branch;
- fetch/update integration branch;
- branch creation;
- checkout;
- diff;
- commit.

## Branch convention

```text
task/<id>-<slug>
```

## Acceptance

A ready task can obtain its own branch from current `main`.

# Stage 8 --- Test Runner

**Status:** DONE — T008

## Goal

Provide deterministic project verification.

## Implement configurable commands for:

- build;
- unit tests;
- integration tests;
- lint/static checks;
- Docker build.

Capture: - exit code; - stdout/stderr; - duration.

## Acceptance

The orchestrator can classify pass/fail without LLM interpretation.

# Stage 9 --- Agent Harness Boundary

**Status:** DONE — T009

## Goal

Avoid coupling the workflow to one coding agent.

## Define capabilities:

```text
DesignTests
Implement
DiagnoseFailure
Fix
Review
```

Start with one provider/harness.

## Acceptance

The task runner talks to an interface rather than provider-specific
logic.

# Stage 10 --- TDD Task Runner

**Status:** DONE — T010

## Goal

Complete a local task from acceptance criteria.

## Flow

```text
READY
 ↓
create branch
 ↓
design/write tests
 ↓
run test
 ↓
verify RED
 ↓
implement
 ↓
verify GREEN
 ↓
run affected suite
```

## Retry

Use bounded test/fix attempts.

## Acceptance

A small task can move from READY to LOCAL_TESTS_PASS or BLOCKED.

# Stage 11 --- Structured Self/Ponytail Review

**Status:** DONE — T011

## Goal

Review implementation before remote CI.

## Review criteria

- acceptance criteria;
- correctness;
- missing tests;
- edge cases;
- error handling;
- architecture boundaries;
- unnecessary complexity;
- security concerns;
- documentation impact.

## Flow

```text
Local Pass
 ↓
Review
 ↓
Findings?
 ├─ no -> REVIEW_PASS
 └─ yes -> Fix -> Tests -> Review again
```

## Acceptance

Review loops stop after configured limits.

# Stage 12 --- Open Code Review Adapter

**Status:** DONE — T012

## Goal

Add an independent optional review provider.

Treat Open Code Review as an adapter:

```text
ReviewProvider
  ├─ Internal/Ponytail
  └─ OpenCodeReview
```

## Acceptance

The orchestrator can run it when configured, capture findings, and apply
policy without making OCR mandatory for every project.

# Stage 13 --- Commit and Documentation Gate

**Status:** DONE — T013

## Goal

Produce a clean task commit.

Before commit: - required tests pass; - required review passes; -
task-specific docs updated.

Recommended commit:

```text
task(T012): implement <title>
```

## Acceptance

The branch contains a coherent, task-scoped commit.

# Stage 14 --- GitHub Adapter

**Status:** DONE — T014

## Goal

Integrate remote workflow.

Implement: - push branch; - create/update PR; - retrieve PR; - inspect
checks; - inspect workflow runs; - retrieve failed job information; -
merge.

## Acceptance

A locally successful task can reach `PR_OPEN`.

# Stage 15 --- GitHub Actions CI

**Status:** DONE — T015

## Goal

Give every PR an independent verification environment.

Initial workflow:

```text
checkout
setup language/runtime
unit tests
integration tests when configured
Docker build
```

## Acceptance

PR status is observable by the orchestrator.

# Stage 16 --- CI Remediation Loop

**Status:** DONE — T016

## Goal

Fix ordinary CI failures automatically.

```text
CI_FAIL
 ↓
collect failure
 ↓
classify actionable?
 ├─ no -> BLOCKED
 └─ yes
      ↓
    diagnose/fix
      ↓
    local test
      ↓
    commit/push
      ↓
    CI again
```

## Acceptance

Retries are bounded and each attempt is recorded.

# Stage 17 --- Merge Gate

**Status:** DONE — T017

## Goal

Merge only validated work.

Required conditions: - local required tests pass; - required review
passes; - required GitHub checks pass; - branch is mergeable under
repository policy.

## Acceptance

Successful PR merges to `main`; failure becomes blocked rather than
bypassing protection.

# Stage 18 --- Completion Loop

**Status:** DONE — T018

## Goal

Automatically continue through the plan.

```text
merge task
 ↓
refresh main
 ↓
mark DONE
 ↓
recompute READY tasks
 ↓
select next
```

## Acceptance

A small multi-task demo project can progress until all tasks are DONE.

# Stage 19 --- Resume and Recovery

**Status:** DONE — T019, T019A

## Goal

Recover safely from process interruption.

Test interruptions at: - after branch creation; - after test creation; -
during implementation; - after PR creation; - while CI is running.

## Acceptance

`sop resume` determines the next legal action without duplicating
remote/local resources.

# Stage 20 --- Environment Bootstrap Tasks

**Status:** DONE — T020

## Goal

Handle projects with no integration environment.

Planner may generate:

```text
T000 Development Environment
```

Possible outputs: - Dockerfile; - Compose; - test scripts; -
`.env.example`; - health checks.

## Acceptance

Feature tasks depending on environment setup remain blocked until
bootstrap passes.

# Stage 21 --- Limited Parallelism

**Status:** DONE — T021

## Goal

Run independent tasks concurrently without exhausting the system.

Start with:

```text
max_parallel_tasks = 2
```

Prefer Git worktrees for parallel local execution.

## Acceptance

Two dependency-independent tasks can run without sharing a working
directory or corrupting Git state.

# Stage 22 --- Documentation and Lessons

**Status:** DONE — T022

## Goal

Make completion leave the repository understandable.

Update as appropriate:

```text
TASKS.md
PLAN.md
LESSONS.md
README.md
ARCHITECTURE.md
```

The live task list is surfaced by `sop status` (and archived specs under
`docs/tasks/`) rather than a checked-in `TASKS.md`.

## Acceptance

The repository accurately reflects completed work and remaining work.

# Stage 23 --- End-to-End Dogfood

**Status:** DONE — T023

## Goal

Use SOP to build a small real project.

The test project should include: - a PRD; - at least 5 tasks; - one
dependency chain; - two parallelizable tasks; - unit tests; -
integration tests; - Docker; - GitHub Actions; - one intentionally
introduced failure to exercise remediation.

## Acceptance

```text
PRD -> plan -> tasks -> branches -> TDD -> review
-> PR -> CI -> fixes -> merge -> complete
```

works with bounded human intervention.

---

# Stage 24 --- JEV Plan Adoption (Wrap-up)

**Status:** DONE — T024–T032

## Goal

Adopt the forward roadmap in [`PRD-JEV.md`](PRD-JEV.md) and
[`PLAN-JEV.md`](PLAN-JEV.md) on top of the V1 core, one task at a time, each with
a spec under [`docs/tasks/`](tasks/) and tests.

## Delivered

```text
T024 local model providers (Ollama, OpenAI-compatible llama.cpp)
T025 configuration model (.agent-sdlc/config.yaml)
T026 configuration-driven agent selection
T027 Markdown task-file loader (sop plan TASK.md)
T028 deterministic quality gate (PASS/FAIL/NEEDS_HUMAN)
T029 command policy (SAFE/REQUIRES_APPROVAL/DENIED)
T030 validation runner (sop validate)
T031 review stage (sop review)
T032 local run lifecycle + run state and report (sop run)
T033 bounded fix loop (review → fix → re-validate, max_fix_cycles)
T034 dependency-aware graph execution (sop run over the task graph)
T035 provider capability detection and routing (agent.Checked)
T036 explicit Git workflow commands (sop commit, sop pr)
T037 MCP server (sop mcp)
T038 optional decision layer (deterministic + routing)
T039 run report command (sop report)
T040 evaluation harness (sop eval)
T041 PLAN-first one-command run (sop run; planflow + PLAN.md compiler)
T042 local vs remote completion (LOCAL_DONE), plan-graph validation, and reconciliation
T043 bootstrap independent of the agent (sop run prepares without an agent)
T044 named plan execution (sop run PLAN.md; --task for a single task)
T045 keep generated artifacts out of the project root (docs/reports; .agent-sdlc self-ignores)
T046 provider resolution visibility, IMPLEMENT capability guard, and untracked-file detection
T047 structured command-agent execution outcomes (completed/needs_human/failed)
T048 a needs_human outcome requeues the task (not terminal BLOCKED); a later run retries
T049 bundled sop-end-to-end skill template and install script
T050 explicit `sop retry` for BLOCKED tasks; no-change completions still validate
T051 bound the requeue loop (a requeue spends one attempt; max_attempts, default 3)
T052 a no-progress retry does not spend the budget; the task stays PLANNED and is retried
T053 `sop retry --all` for every retryable BLOCKED task; a retry carries the prior outcome
T054 verification-first fast path (execution_mode: verify-first); a passing verification needs no agent
T055 configure the model in config (agent.model); the provider env var overrides it
T056 bootstrap DeepSeek coding-agent harness (command provider → Ollama deepseek-v4.1-flash:cloud, controlled tools)
T057 resume the active task in `sop run` (one persisted-state→action rule shared with `sop resume`)
T058 make the Ollama agent capability-aware (per-capability loop bounds, tool policy, no-progress stop)
T059 two-phase PLAN (bounded read-only discovery → tool-free synthesis)
T060 ground a completed IMPLEMENT/FIX outcome's changes_expected in the working tree
T061 SOP performance measurement (stage timing + counts) and safe validation/review reuse
```

The Go module was renamed to `github.com/imhttran/agentic-sop`.

## Remaining

See [BACKLOG.md](BACKLOG.md) for future work that is not yet scheduled into a
numbered task.

---

# Suggested Build Order

```text
0  Environment              (done)
1  State machine            (done)
2  SQLite                   (done)
3  CLI                      (done)
4  Planner                  (done)
5  Task DAG                 (done)
6  Scheduler                (done)
7  Git                      (done)
8  Test runner              (done)
9  Agent adapter            (done)
10 TDD runner               (done)
11 Self review              (done)
12 Open Code Review         (done)
13 Commit/docs              (done)
14 GitHub                   (done)
15 CI                       (done)
16 CI retry                 (done)
17 Merge                    (done)
18 Completion loop          (done)
19 Resume                   (done)
19a Context handoff         (done)
20 Environment bootstrap    (done)
21 Parallelism              (done)
22 Documentation            (done)
23 Dogfood                  (done)
24+ Wrap-up (JEV)           (done: T024–T032)
```

# Definition of Done for V1

V1 is complete when a small project can start from a PRD and reach a
completed `main` branch through dependency-aware task execution,
isolated branches, test-first implementation, bounded review/fix loops,
GitHub CI, validated merge, durable state, resume, and clear
documentation.

Parallelism is useful but should not delay a reliable sequential V1.
