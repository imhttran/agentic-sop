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
  model via Ollama/llama.cpp/MLX, or an external agent command).
- SQLite is provided through the Go driver; no external database is required.

## Quick Start

Install the CLI (clone the repository and `cd` into it first):

```bash
./install.sh                  # macOS / Linux
sop version
```

```powershell
.\install.ps1                 # Windows (PowerShell)
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

For ad-hoc work, `sop prompt` runs a request through the same governed machinery:

```bash
sop prompt --capability review "Review internal/provider for architectural issues"
```

Details: [docs/specs/PROMPT-EXECUTION.md](docs/specs/PROMPT-EXECUTION.md).

## Agent Skills

SOP ships agent skills that expose the same governed path as `/sop` commands in the
coding agents that support them. Install the CLI, then link the skills into the
agent(s) you use:

```bash
./install.sh --skills zed     # the /sop* commands in Zed
./install.sh --skills claude  # the same commands in Claude Code
./install.sh --plugin claude  # the Claude Code plugin
./install.sh --all            # the CLI + every present agent's skills + the plugin
```

```text
/sop-review review internal/provider for architectural problems
/sop-plan plan a cache layer for provider model discovery
/sop-diagnose explain why the current tests are failing
/sop-test design regression tests for this behavior
/sop-implement add caching to provider model discovery
```

Each command is a thin alias over `sop prompt --capability ...`: it picks a capability
and calls SOP, which still owns routing, provider selection, validation, and approval.
`/sop-implement` is the only mutating command and runs the governed implementation
lifecycle. Guides: [docs/guides/INSTALLATION.md](docs/guides/INSTALLATION.md),
[docs/guides/ZED-SKILLS.md](docs/guides/ZED-SKILLS.md), and
[docs/guides/CLAUDE-SKILLS.md](docs/guides/CLAUDE-SKILLS.md); the plugin is
[docs/guides/CLAUDE-PLUGIN.md](docs/guides/CLAUDE-PLUGIN.md).

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
resume/recovery, environment bootstrap, and limited parallelism. Above it, model
routing (3.5), the provider runtime (4), bounded model escalation (5), unified work
items with `sop prompt` (5.4), and distribution — one installer for macOS/Linux and
Windows, a native Claude Code plugin, and CI (5.5–5.6) — are implemented; routing and
escalation are OFF by default. See [docs/plans/BACKLOG.md](docs/plans/BACKLOG.md).

## Documentation

Start at the documentation index: **[docs/README.md](docs/README.md)**.

```text
Getting Started   docs/guides/GETTING-STARTED.md · docs/guides/PROJECT-SETUP.md
Requirements      docs/PRD.md
Architecture      docs/architecture/OVERVIEW.md · docs/architecture/SOP-BOUNDARY.md
Workflow          docs/specs/WORKFLOW.md · docs/specs/TASK-LIFECYCLE.md
Execution         docs/specs/EXECUTION.md · docs/specs/WORK-ITEMS.md · docs/specs/PROMPT-EXECUTION.md
Validation        docs/specs/VALIDATION.md · docs/specs/REVIEW.md · docs/specs/QUALITY.md
Providers         docs/specs/AGENT-PROVIDER.md · docs/specs/PROVIDERS.md · docs/specs/MODEL-ROUTING.md
JEV               docs/specs/OPENJEV.md · docs/reference/JEV-OPERATIONS.md
Gates/Recovery    docs/specs/HUMAN-APPROVAL.md · docs/specs/RECOVERY.md · docs/reference/STATUS-AND-RECOVERY.md
Security          docs/specs/SECURITY.md
Installation      docs/guides/INSTALLATION.md · docs/guides/WINDOWS-INSTALLATION.md
Agent Skills      docs/guides/ZED-SKILLS.md · docs/guides/CLAUDE-SKILLS.md · docs/guides/CLAUDE-PLUGIN.md
CLI Reference     docs/reference/CLI.md
Configuration     docs/reference/CONFIGURATION.md
Performance       docs/reference/PERFORMANCE.md
Development       docs/guides/DEVELOPMENT.md
Plans             docs/plans/ · active plan: docs/plans/PLAN-Phase-3.5-Model-Routing.md
                  latest phase: docs/plans/PLAN-Phase-5.5-Distribution.md
History           docs/history/
```

## License

Apache License 2.0
