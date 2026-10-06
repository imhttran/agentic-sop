# Components

**Type:** Descriptive architecture reference

Each top-level SOP component's responsibilities, one per section. This is the
detail behind the component map in [OVERVIEW.md](OVERVIEW.md) §3–§4. Normative
behavior lives in [`../specs/`](../specs/) and [`../reference/`](../reference/);
the ownership boundary is [SOP-BOUNDARY.md](SOP-BOUNDARY.md).

---

## CLI

The human entry points are the `sop` command surface; the authoritative list of
commands and flags is [`../reference/CLI.md`](../reference/CLI.md). The CLI stays
thin and delegates behavior to the application services below.

## Orchestrator

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

## Task Loader

Reads a single task from a Markdown file (or plain Markdown), extracting ID,
title, description, requirements, acceptance criteria, constraints, and
dependencies. Plain Markdown stays usable. `sop plan TASK.md` and
`sop run TASK.md` accept a task file; the loader normalizes it before a model
sees it. An optional `## Execution` section names the task's execution mode.

## Plan Preparation

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

## Planner

Transforms product intent into implementation intent.

```text
PRD
 ↓
Discover existing system
 ↓
Capability inventory (EXISTS / PARTIAL / MISSING / UNKNOWN)
 ↓
Gap + assumption analysis
 ↓
Synthesize executable plan
 ↓
Task DAG
```

Outputs a structured machine plan (`.agent-sdlc/plan.json`) and its
human-readable rendering (`PLAN.md`); the task DAG is built from it
deterministically, without an agent. When the work integrates with an existing
system, the plan records the capabilities it depends on — with the evidence
behind each finding and, for a capability that is missing or partial, the layer
that owns providing it. Never is a missing or unknown capability described as if
it already exists. A gap whose owner the requirements determine is handled by the
plan (recorded and scoped, or covered by a prerequisite stage); a gap with no
determined owner is a genuine product/architectural choice and stops at
`NEEDS_HUMAN` rather than being invented at implementation time.

## Scheduler

Finds the next task whose dependencies are satisfied.

```text
Task A DONE ----+
                +-> Task C READY
Task B DONE ----+
```

It is deterministic and part of the control plane — it never consults a model —
and selects at most one legally runnable task. V1 runs one task at a time;
parallelism is bounded separately (see Parallelism).

## Task Runner

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

## Git Adapter

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

## Agent Harness Adapter

Provides a boundary around whichever coding agent/harness is used.

Example interface concept:

```text
Execute(task, context) -> result
Review(diff, criteria) -> findings
Fix(task, findings) -> result
```

The workflow should not depend permanently on one model or harness.

## Agent Providers

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

For mutating capabilities (`IMPLEMENT`, `FIX`) command agents may return structured
outcomes. The protocol, deterministic completion
requirement, and current mutation-evidence limitations are owned by
[AGENT-PROVIDER.md §8](../specs/AGENT-PROVIDER.md#8-command-agent-protocol-and-structured-outcome).
The shared phased engine separates bounded discovery credit from repository
mutation; its budgets and finalization behavior are owned by
[§9](../specs/AGENT-PROVIDER.md#9-tool-harness-bounds-and-phases).
Retry/requeue semantics and prior-attempt context belong to
[RECOVERY.md](../specs/RECOVERY.md); operators use
[STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md).

## Validation Runner

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

## Quality Gate

Combines verification status, unresolved review findings, the fix-loop budget,
and human-required flags into one deterministic verdict:

```text
PASS | FAIL | NEEDS_HUMAN
```

The policy (`quality.require_tests`, `quality.fail_on`, `quality.max_fix_cycles`)
comes from configuration; the verdict is computed in code, never by a model. The
same `BlockingFindings` rule is shared with review, so review and the gate agree
on what blocks.

## Review Runner

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

## Run State and Report

`run` persists an inspectable record under `.agent-sdlc/runs/<id>/`:

```text
task.md  plan.md  implementation.md  diff.patch
validation.json  review.json  report.md  report.json  state.json  trace.json
```

`state.json` records the lifecycle stage (`CREATED`…`FAILED`); the report records
the task, provider, validation results, findings, fix cycles, and the final gate.
A run that terminates for any reason stays inspectable. A verify-first run records
`verified_first: true` (the deterministic validation passed without invoking an
implementation agent). `trace.json` is the versioned structured run trace
(schema 2): the execution identity, the observed iterations, the verification
evidence, the termination, and structured progress signals (discovery,
repository mutation, verification, state transition) that distinguish progress
from activity. It observes execution and never controls it — the signals do not
alter the stale streak, retry policy, budgets, or termination — and `sop report`
renders a concise summary from it.

## Execution Modes

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

A third mode, `done`, declares that a stage's work already exists in the tree — it
lets a plan whose work is already green close out instead of being re-implemented
and blocked. It is explicit plan metadata, never inferred from a `Status:` line or
prose, and a plan normalized by a model has it cleared, so only the plan document
itself can declare a stage complete. A stage declared done is recorded **already
satisfied** when the task graph is built: its dependants are unblocked, the plan can
complete, and SOP never selects, gates, or verifies it, because the declaration is
the record rather than evidence. A single `--task` run rejects the mode (it has no
plan to close). A stage whose work must be _verified_ is `verify-first`; one whose
work is _accepted_ is `done`.

## Fix Loop

When validation fails, or when review leaves blocking findings, the failure is
sent back to the agent with the plan, the deterministic validation failure (when
a check failed), the blocking findings, and the current diff; validation and
review then run again (regression protection). A failing build/test/lint is
actionable in its own right — review is skipped when validation fails, so without
this a broken check would never reach a fix. The loop is bounded by
`quality.max_fix_cycles`; exhausting the budget yields `NEEDS_HUMAN`, never an
unbounded agent loop. In graph execution a `NEEDS_HUMAN` result requeues the task
for a later run (bounded by `max_attempts`) rather than blocking it outright.

## Graph Execution

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

## Command Policy

Before a command runs, it is classified deterministically:

```text
SAFE | REQUIRES_APPROVAL | DENIED
```

Read-only and verification commands are `SAFE`; `git commit`/`git push` require
approval; a force-push is `DENIED`. Project policy can only tighten the defaults.
Commands are structured argument lists, never a shell string.

## GitHub Adapter

Handles remote lifecycle operations:

- push;
- create/update PR;
- inspect checks;
- inspect workflow runs/logs;
- rerun when appropriate;
- merge when gates permit.

## Documentation Manager

Updates human-readable state:

```text
PLAN.md
docs/tasks/TASK-0NN.md
docs/guides/LESSONS.md
README / architecture docs when the task requires it
```

It should not replace durable workflow state.

## State Store

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

## Commit Gate

Creates the task commit only when the required tests, review, and documentation
are in place, producing a single deterministic `task(<id>): ...` commit.

## CI Generation and Remediation

Generates a GitHub Actions workflow for a project, and runs a bounded
remediation loop for ordinary CI failures. Whether a failure is actionable is
classified deterministically; only remediation itself is delegated.

## Merge Gate

Merges a pull request only when local tests, review, all GitHub checks, and
mergeability all hold. Any unmet condition blocks with a reason and never
bypasses repository protection.

## Completion Loop

Drives the plan to the end: select the next runnable task, execute it, merge,
refresh `main`, mark it `DONE`, and repeat. Handoffs are recorded best-effort and
never change task state.

## Resume

Reconciles a task's persisted status with the resources that actually exist and
reports the next legal action, recovering a lost state write without duplicating
a branch or pull request (`sop resume`). The status → action decision is shared:
`resume.ActionFor` is the one interpretation, so `sop resume` (which also
reconciles observed branch/PR resources) and `sop run` (which resumes the in-flight
task) can never disagree about what comes next.

## Environment Bootstrap

When a plan marks an environment stage, feature tasks implicitly depend on it and
stay blocked until bootstrap completes.

## Parallelism

Runs dependency-independent tasks concurrently within a bound, each in its own
Git worktree so no two tasks share a working directory.

## Handoff

Produces a small, deterministic capsule describing a completed task, with
optional provider-independent compression of bulky artifacts. Authoritative
truth remains in the state store, Git, verification results, and PR/CI state.

## Git Workflow Commands

`sop commit` and `sop pr` make the two Git operations explicit and gated: both
require `--yes` when the human gate is on, a commit message is derived from the
task file, and a PR uses the deterministic `task/<id>-<slug>` branch. Neither
merges.

## MCP Server

`sop mcp` serves the Model Context Protocol over stdio. Its tools call the same
services as the CLI, so the workflow is not reimplemented; only registered,
project-scoped tools exist.

## Decision Layer (optional)

> **Partial.** Only the deterministic provider exists (`internal/decision`), it is
> wired through configuration but not yet consulted by the run, and no alternative
> provider or tier routing is implemented.

A bounded decision boundary: a provider proposes a choice with a confidence, and
policy (configuration thresholds) routes it to a model tier or a human. The
default provider is deterministic and the layer is disabled by default.

## Evaluation Harness

`sop eval DIR` runs a corpus through the real lifecycle and aggregates task
success, fix cycles, findings, and elapsed time, so harness changes can be
measured rather than asserted.
