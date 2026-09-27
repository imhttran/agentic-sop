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
                 Git Adapter   Validation      Review          CI Adapter
                    |              |              |               |
                    v              v              v               v
                 local git   project cmds   agent/OCR        GitHub
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

Human entry points implemented today:

```text
init        create state and the configuration template
plan        generate PLAN.md from PRD.md or a task file
tasks       build and persist tasks from .agent-sdlc/plan.json
validate    run the configured build/test/lint commands
review      review the working-tree diff with the configured engine
run         run a task file through the local lifecycle
status      list persisted tasks
task        show one task
resume      report the next legal action for interrupted work
version     print the CLI version
help        show usage
```

It remains thin and delegates behavior to application services.

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

### Task Loader

Reads a single task from a Markdown file (or plain Markdown), extracting ID,
title, description, requirements, acceptance criteria, constraints, and
dependencies. Plain Markdown stays usable. `sop plan TASK.md` and
`sop run TASK.md` accept a task file; the loader normalizes it before a model
sees it.

### Plan Preparation

`sop run` prepares the project before executing: it discovers the planning source
(PLAN preferred over PRD), keeps `.agent-sdlc/plan.json` in step with it via a
recorded source path and content fingerprint, and creates tasks when none exist.
A human `PLAN.md` is compiled deterministically (with an agent-normalization
fallback); a PRD is used only to generate a plan when no PLAN exists; and a
changed document triggers a rebuild. Every step is idempotent. Generated
artifacts stay out of the project root: `.agent-sdlc/` holds machine state and
run reports (and ignores itself for Git), and a PRD-generated plan is written to
`docs/reports/<plan-id>.md`.

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
rather than LLM reasoning. Working-tree change detection includes untracked files
(minus SOP's own output), so a new file an agent creates is not missed.

### Agent Harness Adapter

Provides a boundary around whichever coding agent/harness is used.

Example interface concept:

```text
Execute(task, context) -> result
Review(diff, criteria) -> findings
Fix(task, findings) -> result
```

The workflow should not depend permanently on one model or harness.

### Agent Providers

The harness ships provider adapters behind the same boundary:

```text
command    subprocess: JSON request on stdin, response on stdout
ollama     local Ollama /api/chat
llamacpp   OpenAI-compatible /v1/chat/completions (llama.cpp llama-server)
```

The provider is selected by `agent.provider` in configuration, overridden by
`SOP_AGENT_PROVIDER`; credentials and endpoints come from the environment. Each
provider declares which capabilities it serves; `agent.Checked` wraps the
resolved provider and rejects a request for an unsupported capability with a
message naming the supported set, so an unsupported role/provider combination
fails clearly rather than being sent to a provider that cannot serve it. A run
also checks up front that the provider can `IMPLEMENT`, before any task is
attempted.

### Validation Runner

Runs the project's configured verification commands and classifies the result
deterministically. Categories:

```text
build
unit test
integration test
lint
docker build
```

`.agent-sdlc/config.yaml` defines the commands; they run in a fixed order (build,
test, lint) and fail fast, so an uncompilable change never reaches review.
`sop validate` exposes this directly.

### Quality Gate

Combines verification status, unresolved review findings, the fix-loop budget,
and human-required flags into one deterministic verdict:

```text
PASS | FAIL | NEEDS_HUMAN
```

The policy (`quality.require_tests`, `quality.fail_on`, `quality.max_fix_cycles`)
comes from configuration; the verdict is computed in code, never by a model. The
same `BlockingFindings` rule is shared with review, so review and the gate agree
on what blocks.

### Review Runner

Reviews the working-tree diff with a replaceable engine. `sop review` runs it
standalone; `sop run` runs it as a stage.

```text
working-tree diff (Git is authoritative)
        ↓
engine: self (agent) | open-code-review (external command)
        ↓
structured findings
        ↓
blocking = severity ∈ quality.fail_on  → exit code
```

The `open-code-review` engine is an adapter (`SOP_REVIEW_COMMAND`); when it is
not configured the command reports guidance rather than silently substituting
the internal reviewer. The model produces findings; the deterministic policy
decides whether they block.

### Run State and Report

`run` persists an inspectable record under `.agent-sdlc/runs/<id>/`:

```text
task.md  plan.md  implementation.md  diff.patch
validation.json  review.json  report.md  report.json  state.json
```

`state.json` records the lifecycle stage (`CREATED`…`FAILED`); the report records
the task, provider, validation results, findings, fix cycles, and the final gate.
A run that terminates for any reason stays inspectable.

### Fix Loop

When review leaves blocking findings, they are sent back to the agent with the
plan and the current diff; validation and review then run again (regression
protection). The loop is bounded by `quality.max_fix_cycles`; exhausting the
budget yields `NEEDS_HUMAN`, never an unbounded agent loop.

### Graph Execution

`sop run` with no task file drives the persisted task graph: the scheduler selects
the next ready task and the same local lifecycle runs for it, marking it `DONE` on
a passing gate or `BLOCKED` otherwise, until no runnable work remains. Local
execution has no remote PR/CI/merge, so a passing lifecycle synthesizes the
terminal transitions; the dependency rule (a dependency is complete only at
`DONE`) then holds.

### Command Policy

Before a command runs, it is classified deterministically:

```text
SAFE | REQUIRES_APPROVAL | DENIED
```

Read-only and verification commands are `SAFE`; `git commit`/`git push` require
approval; a force-push is `DENIED`. Project policy can only tighten the defaults.
Commands are structured argument lists, never a shell string.

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

### Git Workflow Commands

`sop commit` and `sop pr` make the two Git operations explicit and gated: both
require `--yes` when the human gate is on, a commit message is derived from the
task file, and a PR uses the deterministic `task/<id>-<slug>` branch. Neither
merges.

### MCP Server

`sop mcp` serves the Model Context Protocol over stdio. Its tools call the same
services as the CLI, so the workflow is not reimplemented; only registered,
project-scoped tools exist.

### Decision Layer (optional)

A bounded decision boundary: a provider proposes a choice with a confidence, and
policy (configuration thresholds) routes it to a model tier or a human. The
default provider is deterministic and the layer is disabled by default.

### Evaluation Harness

`sop eval DIR` runs a corpus through the real lifecycle and aggregates task
success, fix cycles, findings, and elapsed time, so harness changes can be
measured rather than asserted.

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

From `REVIEW_PASS` a task either completes locally at `LOCAL_DONE` (the local
lifecycle ran; no PR was opened and no CI ran) or continues through the remote
lifecycle (`PR_OPEN` … `DONE`). A dependency is satisfied by `LOCAL_DONE` (local)
or `MERGED`/`DONE` (remote). Local runs never fabricate remote states.

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

Runs are stored on the filesystem rather than in SQLite, so they stay readable
without the tool:

```text
.agent-sdlc/runs/<id>/
  task.md  plan.md  implementation.md  diff.patch
  validation.json  review.json  report.md  report.json  state.json
```

The exact schema should emerge during implementation, but workflow state
must be durable.

## 13. Configuration

`.agent-sdlc/config.yaml` is generated by `sop init`. It describes policy only:
no mutable task state, and no secrets (unknown keys are rejected, so credentials
cannot be committed by accident).

```yaml
version: 1

project:
  name: book-rag
  integration_branch: main

agent:
  provider: command # command | ollama | llamacpp

validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - go vet ./...

review:
  engine: self # self | open-code-review
  delegation: false

quality:
  require_tests: true
  max_fix_cycles: 3
  fail_on:
    - critical
    - high

human:
  approval_before_commit: true
```

Omitted fields take safe defaults; an invalid file fails with a clear message.
The environment overrides configuration where it matters (for example
`SOP_AGENT_PROVIDER` overrides `agent.provider`).

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

Delivered:

```text
V1  Sequential lifecycle + state machine       (done)
V2  GitHub PR/CI integration                   (done)
V3  Review adapters + bounded remediation      (done)
V4  Two-task parallel scheduling/worktrees     (done)
```

Delivered on top of V1 (`sop validate`, `sop review`, `sop run`):

```text
configuration model + provider selection
validation runner (config-driven, fail-fast)
review stage (self | open-code-review)
deterministic quality gate
bounded fix loop (review → fix → re-validate)
command policy
local model providers (Ollama, llama.cpp)
task-file loader
run state and run report
graph execution (sop run over the persisted task graph)
provider capability detection
Git workflow commands (sop commit, sop pr)
MCP server (sop mcp)
optional decision layer (deterministic + routing)
run report command (sop report)
evaluation harness (sop eval)
PLAN-first one-command run (sop run, planflow)
local vs remote completion (LOCAL_DONE; no synthetic remote states)
generated artifacts kept out of the project root (docs/reports; .agent-sdlc self-ignores)
```

Remaining:

```text
local network service (team mode)
small-device dashboard
Jev adapter + Jev-vs-deterministic evaluation
```
