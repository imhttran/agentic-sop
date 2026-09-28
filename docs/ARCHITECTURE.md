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

### CLI

Human entry points implemented today:

```text
init        create state and the configuration template
status      list persisted tasks
task        show one task
plan        generate PLAN.md from PRD.md or a task file
tasks       build and persist tasks from .agent-sdlc/plan.json
validate    run the configured build/test/lint commands
review      review the working-tree diff with the configured engine
run         run [PLAN.md | --task TASK.md]  (the normal one-command entry point)
commit      commit the current changes (gated by --yes)
pr          push a task branch and open a pull request (gated by --yes)
mcp         serve tools over the Model Context Protocol (stdio)
report      print a summary of a run
retry       requeue a BLOCKED task (<task-id> or --all)
eval        run a corpus of task files and report metrics
resume      report the next legal action for interrupted work
version     print the CLI version
help        show usage
```

It remains thin and delegates behavior to application services.

### Orchestrator

The central workflow controller. Its state, transition, gate, retry, and
stop-condition responsibilities are implemented for the **local** lifecycle.

> **Partial.** The _remote_ path (commit → PR → CI → merge) and the handoff capsule
> exist as components (`internal/commitgate`, `internal/ciremediation`,
> `internal/mergegate`, `internal/completion`, `internal/handoff`) that the e2e
> dogfood test composes; the CLI does not yet drive a project through them end to
> end.

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
sees it. An optional `## Execution` section names the task's execution mode.

### Plan Preparation

`sop run` prepares the project before executing: it discovers the planning source
(PLAN preferred over PRD), keeps `.agent-sdlc/plan.json` in step with it via a
recorded source path and content fingerprint, and creates tasks when none exist.
A human `PLAN.md` is compiled deterministically (with an agent-normalization
fallback); a PRD is used only to generate a plan when no PLAN exists; and a
changed document triggers a rebuild. Every step is idempotent. A compiled stage
carries its optional `execution_mode` onto the task. Generated
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

Outputs a structured machine plan (`.agent-sdlc/plan.json`) and its
human-readable rendering (`PLAN.md`); the task DAG is built from it
deterministically, without an agent.

### Scheduler

Finds the next task whose dependencies are satisfied.

```text
Task A DONE ----+
                +-> Task C READY
Task B DONE ----+
```

It is deterministic and part of the control plane — it never consults a model —
and selects at most one legally runnable task. V1 runs one task at a time;
parallelism is bounded separately (see Parallelism).

### Task Runner

Executes one task's **local** lifecycle (`sop run`), writing a run record under
`.agent-sdlc/runs/<id>/`:

```text
plan → implement → detect changes → validate → review → quality gate
                         ▲                                    │
                         └────────── fix (bounded) ◄──────────┘
```

The agent edits the working tree; Git is the authority on what changed. A passing
gate completes the task locally (`LOCAL_DONE`) and a human boundary stops before
any commit. A verify-first task runs `validate` before `implement` (see
[Execution Modes](#execution-modes)). The remote path — commit → PR → CI → merge —
is driven by the explicit `sop commit`/`sop pr` commands and the GitHub adapter; a
local run never synthesizes it.

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
`SOP_AGENT_PROVIDER`; the model comes from `agent.model` (overridden by the
provider's own variable, `SOP_OLLAMA_MODEL` or `SOP_LLAMACPP_MODEL`); and
credentials and endpoints come from the environment. Each
provider declares which capabilities it serves; `agent.Checked` wraps the
resolved provider and rejects a request for an unsupported capability with a
message naming the supported set, so an unsupported role/provider combination
fails clearly rather than being sent to a provider that cannot serve it. A run
also checks up front that the provider can `IMPLEMENT`, before any task is
attempted.

For mutating capabilities (`IMPLEMENT`, `FIX`) a command agent may return a
structured execution outcome — `completed` with `changes_expected`, `needs_human`,
or `failed`. SOP acts on it directly: a claimed change with none produced is a
failure, a legitimate no-change completion still runs the configured validation
before it passes, and a human boundary stops with `NEEDS_HUMAN` (the task is
requeued — bounded by `max_attempts`, default 3 — so a later run retries it; a
retry that reproduces the same outcome is “no progress” and spends no attempt,
while a retry that changes its outcome spends one; a `BLOCKED` task can be
requeued explicitly with `sop retry <task-id>` or `sop retry --all`, and a retry
is given the previous attempt's outcome as context). The outcome is never
inferred from prose.

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
A run that terminates for any reason stays inspectable. A verify-first run records
`verified_first: true` (the deterministic validation passed without invoking an
implementation agent).

### Execution Modes

A task's `execution_mode` is explicit plan/task metadata — never inferred from a
title or prose. The default, `implement` (the zero value), is the full lifecycle
below. `verify-first` runs the configured validation before any agent:

```text
verify-first → validate ── pass ──→ quality gate → complete locally (no agent)
                   │
                  fail
                   ▼
        implement (input: the failure) → validate → review → gate → fix*
```

A pass invokes no agent at all (not the micro-planner, IMPLEMENT, or FIX) and the
task passes only when its configured checks pass, so SOP stays the authority. A
failure hands the deterministic failure to the implementation agent and the
ordinary lifecycle continues, so a verification failure is not automatically
terminal. A verify-first task with nothing configured to verify falls back to the
implementation path. The mode is persisted on the task
(`tasks.execution_mode`); a pre-existing database migrates its rows to the
implement default. A run still checks up front that the provider can `IMPLEMENT`,
so a configured agent remains required even for a graph whose tasks are all
verify-first.

### Fix Loop

When review leaves blocking findings, they are sent back to the agent with the
plan and the current diff; validation and review then run again (regression
protection). The loop is bounded by `quality.max_fix_cycles`; exhausting the
budget yields `NEEDS_HUMAN`, never an unbounded agent loop.

### Graph Execution

`sop run` with no task file drives the persisted task graph: the scheduler selects
the next ready task and the same local lifecycle runs for it, until no runnable
work remains. A passing lifecycle completes the task locally at `LOCAL_DONE`; it
never fabricates the remote states (`PR_OPEN`, `CI_RUNNING`, `CI_PASS`, `MERGED`),
because no PR was opened and no CI ran. A dependency is satisfied by `LOCAL_DONE`
(local) or `MERGED`/`DONE` (remote), so the persisted state describes what
actually happened.

When the scheduler reports an in-flight (`ActiveTask`) task, `sop run` resumes
_that_ task rather than refusing to run. `scheduler.IsActive` is the single
definition of which status occupies the execution slot, and `resume.ActionFor` is
the single persisted-state → next-action rule; the same rule `sop resume` reports.
A task whose run already passed its gates is completed locally without re-invoking
the agent, a task before the local gates is run through the normal lifecycle, and a
task parked in the remote lifecycle is reported precisely (a local run can reach no
terminal from `PR_OPEN`/`CI_RUNNING`/`CI_PASS`). `ACTIVE_TASK` still prevents
selecting a _different_ task; zero or multiple in-flight tasks is an actionable
error, never a guess.

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
PLAN.md
docs/tasks/TASK-0NN.md
LESSONS.md
README / architecture docs when the task requires it
```

It should not replace durable workflow state.

### State Store

SQLite is the V1 durable source of operational truth. Four tables back it:

```text
tasks             status, blocked_reason, execution_mode, attempt/max_attempts, timestamps
task_dependencies task → dependency edges
task_attempts     one row per attempt (status, reason, output, duration, timestamp)
handoffs          completed-task capsules and compression metadata
```

Run artifacts (validation, review, diffs, reports) live on the filesystem under
`.agent-sdlc/runs/`; branches, commits, PRs, and CI state live in Git and at the
remote. Neither is duplicated into SQLite.

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
a branch or pull request (`sop resume`). The status → action decision is shared:
`resume.ActionFor` is the one interpretation, so `sop resume` (which also
reconciles observed branch/PR resources) and `sop run` (which resumes the in-flight
task) can never disagree about what comes next.

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

> **Partial.** Only the deterministic provider exists (`internal/decision`), it is
> wired through configuration but not yet consulted by the run, and no alternative
> provider or tier routing is implemented.

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
  # model: llama3.2  # ollama / llamacpp; its env var overrides it

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

workflow:
  mode: local # local | pull-request
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
