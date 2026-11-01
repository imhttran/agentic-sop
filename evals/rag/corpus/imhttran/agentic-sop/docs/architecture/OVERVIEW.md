# SOP --- Architecture

> **Implemented vs planned.** This document describes the system as built. Where a
> section, diagram box, or component is designed but not yet reachable from the CLI
> it carries a **Partial** or **Planned** status note; everything else is
> implemented. The e2e dogfood test (`internal/e2e`) composes some components the
> CLI does not yet drive, faking only the external boundaries (model, Git, GitHub).

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

_(The "Time Budget" box is **planned**, not implemented: no request timeout is
enforced today, and `IMPLEMENTATION_TIMEOUT` is a reserved, unused blocked reason.)_

## 4. Component Responsibilities

Each top-level component's responsibilities are described one per section in
[`COMPONENTS.md`](COMPONENTS.md), which is the detail behind the §3 map.
Normative behavior for each concern lives in [`../specs/`](../specs/) and
[`../reference/`](../reference/); the ownership boundary is
[`SOP-BOUNDARY.md`](SOP-BOUNDARY.md).

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

A human boundary (`NEEDS_HUMAN`) is not terminal: the task is requeued to
`PLANNED` so a later run retries it. A retry that changes its outcome spends one
attempt against `max_attempts`; a retry that reproduces the same outcome is “no
progress” and spends nothing, so the task stays runnable rather than burning its
budget. A hard failure (including a claimed change with none produced) leaves the
task `BLOCKED`.

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

> **Planned.** No test-design stage runs today. The state machine reserves
> `TESTS_WRITTEN` and `RED_VERIFIED`, and the local lifecycle walks them as
> transitions, but the runner does not yet design (or verify a failing) test first.

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

> **Partial.** `sop commit` and `sop pr` are implemented (explicit and gated), and
> CI workflow generation (`internal/ci`), the bounded remediation loop
> (`internal/ciremediation`), and the merge gate (`internal/mergegate`) exist as
> components. The CLI does not yet run the push → PR → CI → merge loop for a whole
> project; the e2e dogfood test composes it.

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

> **Partial.** A worktree-based parallel runner exists (`internal/parallel`) and is
> exercised by the e2e dogfood test; `sop run` itself drives the graph one task at a
> time, and `max_parallel_tasks` is not yet a configuration key.

Only dependency-independent tasks may run concurrently, each in its own Git
worktree so no two tasks share a working directory:

```text
repo/
worktrees/
  T002/
  T003/
```

Worktrees are safer for true local parallelism than repeatedly switching one working
tree between branches. Wiring it to `sop run` (with a bound) is what remains.

## 12. Data Model

Tables the V1 store creates:

```text
tasks
  id
  title
  objective
  acceptance_criteria
  execution_mode
  status
  blocked_reason
  attempt
  max_attempts
  created_at
  updated_at

task_dependencies
  task_id
  dependency_task_id

task_attempts
  task_id
  number
  status
  reason
  output
  duration
  timestamp

handoffs
  task_id
  capsule_json
  status
  content
  references_json
  compression_error
  created_at
```

A schema version (`PRAGMA user_version`, currently 3) drives forward migrations, so
an existing database gains new columns (for example `execution_mode`) without
losing data.

Not yet tables: `projects`, `reviews`, `pull_requests`, and `ci_runs`; today review
results live in run artifacts and PR/CI state lives at the remote.

Runs are stored on the filesystem rather than in SQLite, so they stay readable
without the tool:

```text
.agent-sdlc/runs/<id>/
  task.md  plan.md  implementation.md  diff.patch
  validation.json  review.json  report.md  report.json  state.json
```

## 13. Configuration

`.agent-sdlc/config.yaml` is generated by `sop init` and describes **policy
only** — no mutable task state, and no secrets (unknown keys are rejected, so
credentials cannot be committed by accident).

The full schema, field reference, defaults, and environment precedence live in
the configuration reference: [`docs/reference/CONFIGURATION.md`](../reference/CONFIGURATION.md).
This overview deliberately does not restate the schema, so a new or changed key is
documented in exactly one place.


## 14. Security and Permissions

The orchestrator uses the minimum permissions required and never bypasses a
repository protection, exposes secrets to prompts, invents credentials, or
disables a gate to make CI green. Secrets come from the local environment or
GitHub secret management, never project markdown.

The normative rules — command policy, command execution, Git safety, state
ownership, configuration-is-policy, and the repository-safety invariants — live in
[`docs/specs/SECURITY.md`](../specs/SECURITY.md).


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
named plan execution (sop run PLAN.md; --task for a single task)
provider resolution visibility, IMPLEMENT capability guard, untracked-file detection
structured command-agent outcomes (completed / needs_human / failed)
retry and requeue (sop retry <id> / --all; a retry carries the prior outcome; a
  no-progress repeat does not spend the budget)
verification-first fast path (execution_mode: verify-first; a passing verification
  needs no agent)
bundled end-to-end skill and install script
```

Remaining:

```text
drive the remote lifecycle (push → PR → CI → merge) from the CLI
per-task time budgets / timeouts
TDD test-design stage (design and verify a failing test first)
parallel graph execution as a CLI option (max_parallel_tasks)
local network service (team mode)
small-device dashboard
Jev adapter + Jev-vs-deterministic evaluation
```
