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
- **Local first, provider neutral.** Runs locally with local or cloud models and
  does not require a cloud service.

SOP is **not** an unrestricted autonomous coding agent: the orchestrator defines
boundaries, the agent performs specific work, validation checks it, and only then
does state change. See [docs/PRD.md](docs/PRD.md) for principles and requirements.

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
  model via Ollama/llama.cpp, or an external agent command).
- SQLite is provided through the Go driver; no external database is required.

## Quick Start

Install the CLI:

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
go install ./cmd/sop          # ensure $(go env GOPATH)/bin is on PATH
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
reference: [docs/reference/CLI.md](docs/reference/CLI.md).

## Basic Workflow

```text
PLAN
  ↓
SELECT TASK
  ↓
PRECHECK
  ↓
EXECUTE
  ↓
VALIDATE
  ↓
REVIEW
  ↓
JEV / QUALITY
  ↓
HUMAN APPROVAL
  ↓
COMPLETE
```

Detailed state transitions and lifecycle rules live in the specifications:
[docs/specs/WORKFLOW.md](docs/specs/WORKFLOW.md),
[docs/specs/TASK-LIFECYCLE.md](docs/specs/TASK-LIFECYCLE.md), and
[docs/specs/EXECUTION.md](docs/specs/EXECUTION.md).

## Status

SOP V1 is complete: dependency-aware execution, isolated branches, test-first
implementation, bounded review/fix loops, CI and merge gates, durable state,
resume/recovery, environment bootstrap, and limited parallelism. See
[docs/plans/BACKLOG.md](docs/plans/BACKLOG.md) for status and roadmap.

## Documentation

Start at the documentation index: **[docs/README.md](docs/README.md)**.

```text
Getting Started   docs/guides/GETTING-STARTED.md · docs/guides/PROJECT-SETUP.md
Requirements      docs/PRD.md
Architecture      docs/architecture/OVERVIEW.md · docs/architecture/SOP-BOUNDARY.md
Workflow          docs/specs/WORKFLOW.md
Task Lifecycle    docs/specs/TASK-LIFECYCLE.md
Execution         docs/specs/EXECUTION.md
Agent Providers   docs/specs/AGENT-PROVIDER.md
Model Routing     docs/specs/MODEL-ROUTING.md
Validation        docs/specs/VALIDATION.md
Review            docs/specs/REVIEW.md
OpenJEV           docs/specs/OPENJEV.md
Quality Gates     docs/specs/QUALITY.md
Human Approval    docs/specs/HUMAN-APPROVAL.md
Failure/Recovery  docs/specs/RECOVERY.md
Security          docs/specs/SECURITY.md
CLI Reference     docs/reference/CLI.md
Configuration     docs/reference/CONFIGURATION.md
Status/Recovery   docs/reference/STATUS-AND-RECOVERY.md
Development       docs/guides/DEVELOPMENT.md
Plans             docs/plans/ · active plan: docs/plans/PLAN-Phase-3.5-Model-Routing.md
History           docs/history/
```

## License

Apache License 2.0
