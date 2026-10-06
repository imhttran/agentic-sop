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
  does not require a cloud service. The smallest class is local-first: it runs a
  local model and falls back to a cloud model only when the local runtime cannot
  serve it (see [docs/specs/MODEL-ROUTING.md](docs/specs/MODEL-ROUTING.md)).

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

Install the `/sop*` agent commands with `./install.sh --skills zed`,
`./install.sh --skills claude`, or `./install.sh --all`. Start with
[the command examples](docs/guides/GETTING-STARTED.md#install-agent-commands),
[installation](docs/guides/INSTALLATION.md), or
[the Claude Code plugin guide](docs/guides/CLAUDE-PLUGIN.md).
SOP owns routing, validation, and approval for every command.

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

SOP V1 and the later routing, provider-runtime, recovery, prompt, distribution,
interactive-approval, and agentic reliability/evaluation layers (structured run
trace, progress signals, evaluation harness) are implemented. Detailed delivery status and
known limitations live in [PROJECT-STATUS.md](docs/reference/PROJECT-STATUS.md);
future work lives in [BACKLOG.md](docs/plans/BACKLOG.md).

The pre-performance closure and measurement plan,
[Pre-Performance Closure and Baseline](docs/plans/PLAN-Pre-Performance-Closure.md),
is **COMPLETE**: `CLOSE-001…CLOSE-011` are `LOCAL_DONE`. The published baseline is
[PERFORMANCE-BASELINE.md](docs/reports/PERFORMANCE-BASELINE.md); representative
workload measurements remain to be captured, and no new performance architecture is
started by that plan.

## Documentation

Start at **[docs/README.md](docs/README.md)** for the complete categorized index.

| Read about | Start here |
| --- | --- |
| Product requirements | [PRD](docs/PRD.md) |
| Architecture and ownership | [Overview](docs/architecture/OVERVIEW.md), [SOP boundary](docs/architecture/SOP-BOUNDARY.md) |
| Execution and lifecycle | [Execution](docs/specs/EXECUTION.md), [task lifecycle](docs/specs/TASK-LIFECYCLE.md) |
| Providers and harnesses | [Agent provider](docs/specs/AGENT-PROVIDER.md), [model routing](docs/specs/MODEL-ROUTING.md) |
| CLI and configuration | [Commands](docs/reference/CLI.md), [configuration](docs/reference/CONFIGURATION.md) |
| Usage and development | [Getting started](docs/guides/GETTING-STARTED.md), [development](docs/guides/DEVELOPMENT.md) |
| Plans and historical evidence | [Plans](docs/README.md#plans), [history](docs/README.md#historical-documentation) |

## License

Apache License 2.0
