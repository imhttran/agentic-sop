# SOP --- Product Requirements Document

## 1. Product Summary

SOP is a local-first software-development orchestrator that
takes a project from product requirements to completed, tested,
reviewed, and merged implementation.

The system converts a PRD into an engineering plan and dependency-aware
tasks, executes ready tasks on isolated Git branches, follows a
test-driven development loop, performs automated review, runs local and
GitHub CI checks, fixes failures within bounded retry limits, merges
successful work to `main`, updates project documentation, and continues
until the plan is complete or human intervention is required.

The system is intended to automate the engineering workflow without
hiding its state or allowing an LLM to control critical lifecycle
decisions implicitly.

## 2. Problem

AI coding agents can implement individual changes, but completing an
entire software project requires more than code generation.

A reliable SDLC must coordinate:

- requirements;
- architecture and planning;
- task decomposition;
- task dependencies;
- Git isolation;
- test-first development;
- implementation;
- local verification;
- code review;
- CI;
- failure diagnosis;
- bounded retries;
- pull requests and merges;
- documentation;
- progress tracking.

Without an explicit orchestrator, an agent can lose state, repeat work,
merge incomplete changes, run indefinitely on failures, or attempt
dependent tasks in the wrong order.

## 3. Target User

The initial user is an experienced software engineer who wants AI agents
to perform substantial implementation work while retaining:

- visible plans and tasks;
- deterministic workflow gates;
- Git history;
- tests;
- review;
- CI;
- bounded resource usage;
- the ability to intervene at any time.

V1 is designed for a single developer and a single GitHub repository at
a time.

## 4. Primary Goal

Given a PRD and repository, the system can move the project through:

```text
PRD
 ↓
Engineering Plan
 ↓
Task Graph
 ↓
Ready Task
 ↓
Git Branch
 ↓
TDD
 ↓
Implementation
 ↓
Local Tests
 ↓
Automated Review
 ↓
Fix / Retry
 ↓
Pull Request
 ↓
GitHub CI
 ↓
Merge
 ↓
Documentation
 ↓
Next Task
```

until all tasks are complete or a task is explicitly blocked for human
intervention.

## 5. Product Principles

### Deterministic orchestration, agentic execution

LLMs may reason about requirements, code, tests, failures, and reviews.
The workflow engine owns task state, dependencies, retries, timeouts,
and merge gates.

### Tests before trust

Implementation is not complete because code was generated. Relevant
tests must pass.

### Bounded autonomy

Every retry loop must have a maximum attempt count and/or time budget.

### Git is the unit of isolation

Each task executes on its own branch and reaches `main` through an
explicit integration path.

### CI is an independent gate

Local success is not sufficient when repository CI is available.

### Human-readable state

The user should be able to inspect what is running, why it is running,
what failed, and what comes next.

### Keep concurrency small

Parallelism is allowed only for dependency-independent tasks and should
default to a conservative limit.

### Documentation is part of completion

A task may require project documentation, plan state, and lessons to be
updated before it is considered complete.

## 6. Core User Stories

- As a developer, I want to provide a PRD and receive a staged
  engineering plan.
- As a developer, I want the plan decomposed into small tasks with
  acceptance criteria and dependencies.
- As a developer, I want independent tasks identified so limited
  parallel work is possible.
- As a developer, I want each task implemented on a dedicated Git
  branch.
- As a developer, I want tests written before implementation where
  practical.
- As a developer, I want the system to verify that a new test fails
  before implementing the feature.
- As a developer, I want automated review after implementation.
- As a developer, I want review findings fixed and rechecked within
  bounded retry limits.
- As a developer, I want pull requests and GitHub CI used when
  configured.
- As a developer, I want failed CI logs inspected and actionable
  failures retried.
- As a developer, I want successful tasks merged into `main`.
- As a developer, I want the orchestrator to continue to the next
  ready task automatically.
- As a developer, I want blocked tasks surfaced instead of looping
  indefinitely.
- As a developer, I want Docker/development-environment setup created
  when the project does not yet have a runnable environment.
- As a developer, I want project documentation updated as
  implementation progresses.

## 7. Functional Requirements

### FR-1 --- Project initialization

The system shall initialize agent configuration and state for an
existing or new Git repository.

### FR-2 --- PRD ingestion

The system shall read a project PRD as the product-level source of
requirements.

### FR-3 --- Engineering plan generation

The system shall generate or update a staged `PLAN.md` from the PRD.

### FR-4 --- Task decomposition

The system shall break plan stages into tasks small enough to implement
and verify independently.

Each task shall include at minimum:

- ID;
- title;
- objective;
- acceptance criteria;
- dependencies;
- expected test scope;
- status.

### FR-5 --- Dependency graph

The system shall maintain task dependencies and determine which tasks
are ready to execute.

### FR-6 --- Controlled parallelism

The scheduler may execute dependency-independent tasks concurrently,
subject to a configurable concurrency limit. V1 shall default to no more
than two active implementation tasks.

### FR-7 --- Environment bootstrap

If required development infrastructure is missing, the system shall be
able to create an explicit environment/bootstrap task before dependent
feature tasks.

### FR-8 --- Branch creation

Each implementation task shall use an isolated Git branch.

Recommended convention:

```text
task/<task-id>-<slug>
```

### FR-9 --- TDD loop

Where practical, implementation tasks shall follow:

```text
Acceptance Criteria
 ↓
Write Test
 ↓
Verify Failure
 ↓
Implement
 ↓
Verify Pass
 ↓
Refactor
 ↓
Run Affected Suite
```

### FR-10 --- Local verification

The system shall run relevant formatting, compilation, unit tests,
integration tests, and static checks configured for the project.

### FR-11 --- Automated review

The system shall support a structured self-review/Ponytail-style review
loop and an optional external review stage such as Alibaba Open Code
Review.

### FR-12 --- Review remediation

Blocking review findings shall return the task to a fix-and-retest
state.

### FR-13 --- Commit

A task that passes required local gates shall be committed with
task-identifiable commit metadata.

### FR-14 --- Pull request

When GitHub integration is configured, the system shall push the branch
and create or update a pull request.

### FR-15 --- CI

The system shall observe required GitHub CI checks before merge.

### FR-16 --- CI diagnosis

When CI fails, the system shall retrieve available failure
information/logs and classify whether the failure is actionable by the
task agent.

### FR-17 --- Bounded retry

Test, review, and CI remediation loops shall use configurable maximum
attempts and/or wall-clock limits.

### FR-18 --- Blocked state

When retry limits are exhausted or a failure requires human input, the
task shall transition to `BLOCKED` with a recorded reason.

### FR-19 --- Merge

A task shall merge only after required tests, reviews, and CI gates
pass.

### FR-20 --- Main synchronization

After merge, subsequent tasks shall begin from an updated integration
branch.

### FR-21 --- Documentation update

The system shall update task status and relevant project documentation
after successful integration.

### FR-22 --- Completion loop

The orchestrator shall continue selecting ready tasks until all tasks
are complete or remaining work is blocked.

### FR-23 --- Resume

The workflow shall be resumable after process interruption without
relying on LLM conversational memory.

### FR-24 --- Status

The user shall be able to inspect project, task, branch, retry, review,
and CI state.

## 8. Task Lifecycle

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

Failure transitions may move a task into:

```text
FIX_REQUIRED
RETRY_WAIT
BLOCKED
```

## 9. Non-Functional Requirements

### Recoverability

Workflow state must survive process restarts.

### Observability

Each state transition and agent execution should produce useful
structured logs.

### Idempotency

Resuming an interrupted operation should not create duplicate branches,
pull requests, commits, or task records.

### Safety

The orchestrator must not merge code that has failed required gates.

### Resource control

Concurrency, retry counts, and task execution duration shall be
configurable and bounded.

### Portability

The orchestrator should run locally and interact with ordinary Git
repositories. GitHub-specific functionality should remain behind an
integration boundary.

### Simplicity

V1 should favor a single process and SQLite/local files over distributed
infrastructure.

## 10. V1 Technology Direction

The intended implementation is:

- Go CLI/application;
- SQLite for durable workflow state;
- local Git CLI for repository operations;
- GitHub integration for PRs and CI;
- pluggable coding/review agent commands;
- Docker/Compose support for project environments where useful;
- Markdown project artifacts for humans;
- machine-readable task/workflow state owned by the orchestrator.

## 11. V1 Commands

The intended CLI shape is:

```text
sop init
sop plan
sop status
sop run
sop resume
sop task <id>
sop retry <id>
```

Exact command names may change during implementation.

## 12. Human vs Machine Sources of Truth

Human-readable:

```text
PRD.md
ARCHITECTURE.md
PLAN.md
TASKS.md
docs/guides/LESSONS.md
```

Operational state:

```text
SQLite / structured state
```

Markdown must not be the only source of runtime state.

## 13. Default Guardrails

Initial defaults:

```text
max parallel implementation tasks: 2
local test/fix attempts:            3
review/fix attempts:                3
CI/fix attempts:                    3
task execution time budget:         configurable, conservative
```

The exact defaults should be configuration rather than hard-coded
policy.

## 14. V1 Success Criteria

V1 is successful when it can take a small repository with a PRD and:

1.  generate a plan;
2.  generate dependency-aware tasks;
3.  identify a ready task;
4.  create a task branch;
5.  execute a TDD implementation loop;
6.  run local tests;
7.  run structured automated review;
8.  remediate ordinary failures within limits;
9.  commit successful work;
10. create a GitHub pull request when configured;
11. observe GitHub CI;
12. remediate actionable CI failures;
13. merge passing work;
14. mark the task complete;
15. select the next task;
16. resume safely after interruption;
17. stop and report when human intervention is required.

## 15. Out of Scope for V1

- unlimited autonomous execution;
- large multi-agent swarms;
- more than a small number of parallel tasks;
- automatic production deployment;
- infrastructure provisioning across cloud providers;
- autonomous secret creation;
- bypassing branch protections;
- replacing GitHub's permission model;
- self-modifying workflow policy;
- cross-repository dependency orchestration;
- automatic acceptance of every AI review suggestion.

## 16. Future Candidates

- richer task DAG visualization;
- worktree-based parallel task isolation;
- multiple repositories;
- issue tracker synchronization;
- deployment gates;
- release generation;
- richer security scanning;
- cost/token budgets;
- agent/provider benchmarking;
- learned scheduling policies;
- automated rollback after post-merge regression.
