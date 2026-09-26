# SOP --- Architecture

## 1. Architectural Goal

The system separates **deterministic workflow control** from **agentic
reasoning**.

Agents are good at:

- interpreting requirements;
- designing tests;
- writing code;
- diagnosing failures;
- reviewing changes;
- proposing fixes.

The orchestrator is responsible for:

- durable state;
- task dependencies;
- state transitions;
- branch ownership;
- retry limits;
- concurrency;
- timeouts;
- required gates;
- merge decisions.

The core rule is:

> Agents propose and execute work; the orchestrator decides what state
> the workflow is in and what is allowed to happen next.

## 2. System Context

```text
                    Developer
                        |
                        v
                 +-------------+
                 | sop  |
                 |   Go CLI    |
                 +------+------+
                        |
        +---------------+----------------+
        |               |                |
        v               v                v
   Local Git       Agent Harness      GitHub
   Repository      / LLM Tools        PR + CI
        |               |                |
        v               v                v
 Project Files       Code/Test       Actions/Checks
 Docker/Compose       Review
```

## 3. Top-Level Components

```text
+------------------------------------------------------+
|                    CLI / API                         |
+---------------------------+--------------------------+
                            |
                            v
+------------------------------------------------------+
|                    Orchestrator                      |
| State Machine | Gates | Retry Policy | Time Budget  |
+------+----------------+----------------------+-------+
       |                |                      |
       v                v                      v
+-------------+   +-------------+      +--------------+
|   Planner   |   |  Scheduler  |      | Task Runner  |
+-------------+   +-------------+      +------+-------+
                                               |
                    +--------------------------+-------------------+
                    |              |              |               |
                    v              v              v               v
                 Git Adapter    Test Runner   Review Runner    CI Adapter
                    |              |              |               |
                    v              v              v               v
                 local git     project cmds   agent/OCR        GitHub
                    |
                    v
              Working Branch

+------------------------------------------------------+
|                  Durable State                       |
|                       SQLite                         |
+------------------------------------------------------+
```

## 4. Component Responsibilities

### CLI

Provides human entry points such as:

```text
init
plan
run
status
resume
task
retry
```

It should remain thin and delegate behavior to application services.

### Orchestrator

The central workflow controller.

Responsibilities:

- load durable project state;
- select legal state transitions;
- enforce gates;
- apply retry policy;
- stop on blocked/human-required conditions;
- coordinate planner, scheduler, task runner, review, CI, and merge.

The orchestrator should not contain code-generation intelligence.

### Planner

Transforms product intent into implementation intent.

```text
PRD
 ↓
Architecture assumptions
 ↓
PLAN
 ↓
Task DAG
```

Outputs structured tasks as well as human-readable `PLAN.md` /
`TASKS.md`.

### Scheduler

Finds tasks whose dependencies are satisfied.

```text
Task A DONE ----+
                +-> Task C READY
Task B DONE ----+
```

It also enforces concurrency limits and avoids selecting conflicting
work when detectable.

### Task Runner

Executes one task lifecycle.

Conceptually:

```text
branch
 ↓
test design
 ↓
red
 ↓
implementation
 ↓
green
 ↓
review
 ↓
fix
 ↓
commit/PR
 ↓
CI
 ↓
merge
```

### Git Adapter

Wraps local Git operations.

Responsibilities include:

- repository validation;
- current branch/status;
- create/switch branch;
- diff;
- commit;
- merge/rebase operations where policy allows;
- update from integration branch.

Git implementation should be deterministic shell/process execution
rather than LLM reasoning.

### Agent Harness Adapter

Provides a boundary around whichever coding agent/harness is used.

Example interface concept:

```text
Execute(task, context) -> result
Review(diff, criteria) -> findings
Fix(task, findings) -> result
```

The workflow should not depend permanently on one model or harness.

### Test Runner

Discovers or executes project-defined verification commands.

Possible categories:

```text
format
compile
unit
integration
lint
static analysis
docker build
```

Project configuration defines which are required.

### Review Runner

Coordinates review gates.

Potential flow:

```text
Self/Ponytail Review
        ↓
Fix
        ↓
Open Code Review
        ↓
Classify Findings
        ↓
Pass / Fix Required
```

External review is an adapter rather than hard-coded into orchestration.

### GitHub Adapter

Handles remote lifecycle operations:

- push;
- create/update PR;
- inspect checks;
- inspect workflow runs/logs;
- rerun when appropriate;
- merge when gates permit.

### Documentation Manager

Updates human-readable state:

```text
TASKS.md
PLAN.md
LESSONS.md
README/architecture docs when task requires it
```

It should not replace durable workflow state.

### State Store

SQLite is the V1 durable source of operational truth.

It stores:

- projects;
- tasks;
- dependencies;
- attempts;
- state transitions;
- branches;
- commits;
- PR identifiers;
- CI runs;
- review results;
- handoff capsules and compression metadata;
- timestamps;
- blocked reasons.

### Commit Gate

Creates the task commit only when the required tests, review, and documentation
are in place, producing a single deterministic `task(<id>): ...` commit.

### CI Generation and Remediation

Generates a GitHub Actions workflow for a project, and runs a bounded
remediation loop for ordinary CI failures. Whether a failure is actionable is
classified deterministically; only remediation itself is delegated.

### Merge Gate

Merges a pull request only when local tests, review, all GitHub checks, and
mergeability all hold. Any unmet condition blocks with a reason and never
bypasses repository protection.

### Completion Loop

Drives the plan to the end: select the next runnable task, execute it, merge,
refresh `main`, mark it `DONE`, and repeat. Handoffs are recorded best-effort and
never change task state.

### Resume

Reconciles a task's persisted status with the resources that actually exist and
reports the next legal action, recovering a lost state write without duplicating
a branch or pull request (`sop resume`).

### Environment Bootstrap

When a plan marks an environment stage, feature tasks implicitly depend on it and
stay blocked until bootstrap completes.

### Parallelism

Runs dependency-independent tasks concurrently within a bound, each in its own
Git worktree so no two tasks share a working directory.

### Handoff

Produces a small, deterministic capsule describing a completed task, with
optional provider-independent compression of bulky artifacts. Authoritative
truth remains in the state store, Git, verification results, and PR/CI state.

## 5. Task State Machine

```text
                         +------------------+
                         |     PLANNED      |
                         +--------+---------+
                                  |
                       dependencies satisfied
                                  |
                                  v
                         +------------------+
                         |      READY       |
                         +--------+---------+
                                  |
                            create branch
                                  |
                                  v
                         +------------------+
                         | BRANCH_CREATED   |
                         +--------+---------+
                                  |
                             write tests
                                  |
                                  v
                         +------------------+
                         | TESTS_WRITTEN    |
                         +--------+---------+
                                  |
                            verify failure
                                  |
                                  v
                         +------------------+
                         |  RED_VERIFIED    |
                         +--------+---------+
                                  |
                              implement
                                  |
                                  v
                         +------------------+
                         |  IMPLEMENTING    |
                         +--------+---------+
                                  |
                           local gates pass
                                  |
                                  v
                         +------------------+
                         |LOCAL_TESTS_PASS  |
                         +--------+---------+
                                  |
                               review
                                  |
                                  v
                         +------------------+
                         |     REVIEW       |
                         +--------+---------+
                                  |
                             review pass
                                  |
                                  v
                         +------------------+
                         |   REVIEW_PASS    |
                         +--------+---------+
                                  |
                              open PR
                                  |
                                  v
                         +------------------+
                         |     PR_OPEN      |
                         +--------+---------+
                                  |
                                  v
                         +------------------+
                         |    CI_RUNNING    |
                         +--------+---------+
                                  |
                              CI passes
                                  |
                                  v
                         +------------------+
                         |     CI_PASS      |
                         +--------+---------+
                                  |
                                merge
                                  |
                                  v
                         +------------------+
                         |      MERGED      |
                         +--------+---------+
                                  |
                            update state/docs
                                  |
                                  v
                         +------------------+
                         |       DONE       |
                         +------------------+
```

Any actionable failure can transition to `FIX_REQUIRED`; exhausted
retries transition to `BLOCKED`.

## 6. Retry Architecture

Retries belong to the orchestrator.

```text
Operation
   |
 Failure
   |
Classify
   +----------------+
   |                |
Actionable       Human/External
   |                |
attempt < max?      BLOCKED
   |
 Yes ----> FIX_REQUIRED -> retry
   |
 No
   v
BLOCKED
```

Each attempt should record:

- operation;
- start/end;
- result;
- failure summary;
- logs/artifacts where practical;
- agent action;
- next state.

## 7. TDD Architecture

The task runner should distinguish test creation from implementation.

```text
Task Acceptance Criteria
          |
          v
     Test Design Agent
          |
          v
       New Test
          |
          v
    Run Test (RED)
       /      \
 expected     unexpected
 failure       result
   |              |
   v              v
Implement       Diagnose
   |
   v
Run Test (GREEN)
   |
Full affected suite
```

Not every maintenance task can prove a traditional red phase. The task
should record when TDD is not applicable and why rather than fabricating
a failure.

## 8. Review Architecture

Review should be layered:

```text
Cheap deterministic gates
       |
       v
Structured self-review
       |
       v
External AI review (optional)
       |
       v
Blocking finding classification
```

Compilation/tests should happen before expensive AI review.

AI review findings are advisory until policy classifies them as
blocking. The orchestrator should never blindly implement every review
suggestion.

## 9. CI Architecture

```text
Local Pass
   |
Push Branch
   |
Open/Update PR
   |
GitHub Actions
   |
Required Checks
  / \
Pass Fail
 |    |
 |   Fetch logs
 |    |
 |   Actionable?
 |    +-> yes -> Fix -> Push -> CI again
 |    +-> no  -> BLOCKED
 |
Merge Gate
```

Recommended initial GitHub Actions:

```text
format/lint
unit tests
integration tests
docker build
optional Open Code Review
```

## 10. Environment Bootstrap

Projects may start without a runnable integration environment.

The planner should create an explicit bootstrap task when needed:

```text
T000 Development Environment
```

Possible outputs:

```text
Dockerfile
docker-compose.yml
.env.example
Makefile
scripts/test.sh
scripts/integration-test.sh
```

Feature tasks that depend on integration infrastructure remain blocked
until T000 is complete.

## 11. Parallelism

V1 default:

```text
max_parallel_tasks = 2
```

Only dependency-independent tasks may run concurrently.

A later implementation may use Git worktrees:

```text
repo/
worktrees/
  T002/
  T003/
```

This is safer for true local parallelism than repeatedly switching one
working tree between branches.

V1 may begin sequentially and introduce worktrees after the basic
lifecycle is reliable.

## 12. Data Model

Conceptual tables:

```text
projects
  id
  repo_path
  integration_branch
  status

tasks
  id
  project_id
  title
  objective
  status
  branch
  max_attempts
  created_at
  updated_at

task_dependencies
  task_id
  depends_on_task_id

attempts
  id
  task_id
  phase
  attempt_number
  status
  started_at
  ended_at
  failure_summary

reviews
  id
  task_id
  provider
  status
  findings

pull_requests
  task_id
  number
  url
  status

ci_runs
  id
  task_id
  external_id
  status
  conclusion
```

The exact schema should emerge during implementation, but workflow state
must be durable.

## 13. Configuration

Example conceptual configuration:

```yaml
project:
  integration_branch: main

scheduler:
  max_parallel_tasks: 2

retries:
  local_test: 3
  review: 3
  ci: 3

commands:
  unit_test: go test ./...
  integration_test: go test -tags=integration ./...
  lint: golangci-lint run

review:
  self_review: true
  open_code_review: optional

github:
  require_ci: true
  auto_merge: true
```

Configuration should describe policy; it should not contain mutable task
state.

## 14. Security and Permissions

The orchestrator should use the minimum permissions required.

It must not:

- bypass repository protections;
- expose secrets to agent prompts unnecessarily;
- invent credentials;
- disable tests to make CI green;
- force-push protected branches unless explicitly configured and
  appropriate.

Secrets should come from the local environment or GitHub secret
management, not project markdown.

## 15. Initial Deployment Model

V1 is a local CLI:

```text
Developer machine
  |
sop process
  |
SQLite state
  |
local repository
  |
GitHub remote
```

The projects being developed may themselves use Docker, databases, or
other services.

The orchestrator does not need Kubernetes or a server deployment for V1.

## 16. Architecture Evolution

Recommended evolution:

```text
V1  Sequential lifecycle + state machine
V2  GitHub PR/CI integration
V3  Review adapters + bounded remediation
V4  Two-task parallel scheduling/worktrees
V5  richer policies, providers, observability
```

The architecture should prove the state machine before adding
concurrency.
