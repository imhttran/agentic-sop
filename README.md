# SOP

**SOP — a standard operating procedure for agentic software development.** SOP is
a local-first orchestrator that runs a software development lifecycle with AI
agents while keeping workflow control deterministic.

> **Agents do the work. SOP controls the process.**

Coding agents are good at understanding requirements, planning, writing code and
tests, reviewing, and diagnosing failures — but they should not be the source of
truth for workflow state. SOP separates those responsibilities: agents reason and
implement, and the orchestrator decides what is allowed to happen next. Detailed
behavior lives in the [documentation](#documentation); the authoritative
ownership model is [docs/architecture/SOP-BOUNDARY.md](docs/architecture/SOP-BOUNDARY.md).

## Core Principles

```text
SOP owns orchestration and lifecycle state.
Agents perform bounded work.
Models are replaceable reasoning engines.
Validation provides deterministic evidence.
Review and OpenJEV provide additional quality signals.
Human approval remains explicit where policy requires it.
```

- **Deterministic control, agentic execution.** State transitions, scheduling,
  retry bounds, merge eligibility, and failure classification are deterministic
  and model-free; models only produce content.
- **The LLM creates content; the application controls the process.** A model
  never decides whether its own output is valid.
- **Machine state is not Markdown.** Executable state lives in structured data
  (`SQLite`, `plan.json`); the orchestrator never parses human docs for its state.
- **Bounded autonomy.** Retries and fix loops have explicit limits and end at a
  human gate when they must.
- **Operator-owned acceptance checks.** Deterministic checks provide objective
  completion evidence; a task's PASS alone does not verify its objective. See
  [Execution](docs/specs/EXECUTION.md).
- **Local first, provider neutral.** Runs locally with local or cloud models and
  does not require a cloud service. The smallest class is local-first: it runs a
  local model and falls back to a cloud model only when the local runtime cannot
  serve it (see [docs/specs/MODEL-ROUTING.md](docs/specs/MODEL-ROUTING.md)).
- **Evidence-bound model selection.** Model locality resolves from explicit
  configuration, authoritative runtime evidence, the narrow Ollama `:cloud`
  naming rule, or a built-in default. Operators can separately pin the plan,
  review, and JEV stages to configured model classes; see
  [model routing](docs/specs/MODEL-ROUTING.md#supporting-stage-classes).

SOP is **not** an unrestricted autonomous coding agent: the orchestrator defines
boundaries, the agent performs specific work, validation checks it, and only then
does state change. See [docs/PRD.md](docs/PRD.md) for principles and requirements.

Governance ownership is defined in [SOP Boundary](docs/architecture/SOP-BOUNDARY.md).
The [consolidation guide](docs/guides/REPOSITORY-CONSOLIDATION.md#governance-boundaries)
records advisory compatibility, evaluation, and enablement boundaries.

## Basic Architecture

```text
PRD / PLAN
    |
    v
Agentic SOP
    |
    +-- Scheduler
    +-- Agent Harness
    +-- Validation
    +-- Review
    +-- OpenJEV
    +-- Quality Policy
    +-- Recovery
    |
    v
Repository
```

SOP separates reasoning from control; every component above is implemented.
Detailed structure: [docs/architecture/OVERVIEW.md](docs/architecture/OVERVIEW.md).

## Requirements

- **Go** (see `go.mod`) and **Git**.
- An **AI command or agent harness** for planning and implementation (a local
  model via Ollama/llama.cpp/MLX, or an external agent command).
- SQLite is provided through the Go driver; no external database is required.

## Quick Start

Install the CLI (clone the repository and `cd` into it first):

```bash
./install.sh                   # macOS / Linux
.\install.ps1                  # Windows (PowerShell)
sop version
```

Then, from a project you want SOP to manage:

```bash
cd ~/workspace/projects/book-rag
sop run
```

`sop run` discovers the planning source, prepares state and tasks, and executes
the task graph — idempotently. Full setup: [docs/guides/GETTING-STARTED.md](docs/guides/GETTING-STARTED.md)
and [docs/guides/PROJECT-SETUP.md](docs/guides/PROJECT-SETUP.md). Complete command
reference: [docs/reference/CLI.md](docs/reference/CLI.md). To run SOP's model
classes on Claude, see [docs/guides/HOW-TO-SETUP-WITH-CLAUDE.md](docs/guides/HOW-TO-SETUP-WITH-CLAUDE.md).

For ad-hoc work, `sop prompt` runs a request through the same governed machinery:

```bash
sop prompt --capability review "Review internal/provider for architectural issues"
```

Details: [docs/specs/PROMPT-EXECUTION.md](docs/specs/PROMPT-EXECUTION.md).

The unified executable also provides `sop controller serve` for the dashboard and
`sop decision decide|shadow` for advisory decisions. Setup, optional requirements,
and legacy wrappers are covered in the
[consolidation guide](docs/guides/REPOSITORY-CONSOLIDATION.md).

## Agent Skills and Optional Surfaces

Install the `/sop*` agent commands with `./install.sh --skills zed`,
`./install.sh --skills claude`, or `./install.sh --all`. SOP owns routing,
validation, and approval for every command. Start with
[the command examples](docs/guides/GETTING-STARTED.md#install-agent-commands),
[installation](docs/guides/INSTALLATION.md), or
[the Claude Code plugin guide](docs/guides/CLAUDE-PLUGIN.md); the RAG skills are in
[docs/guides/RAG-SKILLS.md](docs/guides/RAG-SKILLS.md) and the provider-neutral MCP
validation skill in [docs/guides/MCP-LIVE-VALIDATION.md](docs/guides/MCP-LIVE-VALIDATION.md).

An executor publishes progress and completion evidence with `sop result publish`,
and a planner connected through `sop mcp --planner` reads it and records a bound
`continue`, `correct`, `revise`, or `escalate` decision, which the executor applies
with `sop planner continue --apply` before resuming `sop run`. With
`--start-work` the planner can start the work itself. Planner advice is never human
approval, and continuation stays interactive. See the
[CLI reference](docs/reference/CLI.md#setting-up-the-planner) and the
[planner–executor plan](docs/plans/PLAN-Executore-Integration.md).

Optional evaluation tooling sits outside the default execution path:
[`sop compare`](docs/guides/HIP-COMPARE.md) and the report-only
[Promptfoo adapter](docs/security-promptfoo.md). Neither can promote a candidate or
alter deterministic execution governance.

Background work records a durable lifecycle event delivered through one configured
channel (`none`, `file`, or `webhook`); delivery is at-least-once, a delivery failure
never changes a task's outcome, and no channel is enabled by default. See
[Notifications](docs/guides/NOTIFICATIONS.md).

## Basic Workflow

```text
PLAN → SELECT TASK → PRECHECK → EXECUTE → VALIDATE → REVIEW → JEV/QUALITY → HUMAN APPROVAL → COMPLETE
```

Detailed state transitions and lifecycle rules live in the specifications:
[docs/specs/WORKFLOW.md](docs/specs/WORKFLOW.md),
[docs/specs/TASK-LIFECYCLE.md](docs/specs/TASK-LIFECYCLE.md), and
[docs/specs/EXECUTION.md](docs/specs/EXECUTION.md).

After an accepted plan revision, `sop approval invalidate` can resolve a stale
`PENDING` approval only when SOP can prove the task-specific definition change.
It records an audited invalidation and never approves, satisfies, completes, or
waives work; see the [approval guide](docs/guides/APPROVALS.md#invalidating-stale-pending-evidence).

## Status

SOP V1 and the routing, provider, recovery, distribution, approval, agentic
reliability, and context-efficiency layers are implemented, including recent
benchmark-driven routing, recovery, and isolation hardening. Optional local
decision memory is implemented but disabled by default pending a valid held-out
benefit evaluation. See [Project Status](docs/reference/PROJECT-STATUS.md) for
delivery evidence and known limitations, [Backlog](docs/plans/BACKLOG.md) for future
work, and the [performance baseline](docs/reports/PERFORMANCE-BASELINE.md) for
measured results. Repository consolidation is complete, and its
[evidence](docs/guides/REPOSITORY-CONSOLIDATION.md) does not establish current local
machine or CI status; the legacy remotes stay archived read-only pending the
[legacy transition checklist](docs/guides/LEGACY-REPOSITORY-TRANSITION.md#readiness-checklist).

## Documentation

Start at **[docs/README.md](docs/README.md)** for the complete categorized index.

| Read about                    | Start here                                                                                   |
| ----------------------------- | -------------------------------------------------------------------------------------------- |
| Product requirements          | [PRD](docs/PRD.md)                                                                           |
| Architecture and ownership    | [Overview](docs/architecture/OVERVIEW.md), [SOP boundary](docs/architecture/SOP-BOUNDARY.md) |
| Execution and lifecycle       | [Execution](docs/specs/EXECUTION.md), [task lifecycle](docs/specs/TASK-LIFECYCLE.md)         |
| Providers and harnesses       | [Agent provider](docs/specs/AGENT-PROVIDER.md), [model routing](docs/specs/MODEL-ROUTING.md) |
| CLI and configuration         | [Commands](docs/reference/CLI.md), [configuration](docs/reference/CONFIGURATION.md)          |
| Usage and development         | [Getting started](docs/guides/GETTING-STARTED.md), [development](docs/guides/DEVELOPMENT.md) |
| Decision-model findings       | [Decision-model findings](docs/reference/DECISION-MODEL-FINDINGS.md)                         |
| Plans and historical evidence | [Plans](docs/plans/README.md), [history](docs/README.md#historical-documentation)            |

## License

Apache License 2.0
