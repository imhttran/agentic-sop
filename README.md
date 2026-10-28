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

## Agent Skills

Install the `/sop*` agent commands with `./install.sh --skills zed`,
`./install.sh --skills claude`, or `./install.sh --all`. Start with
[the command examples](docs/guides/GETTING-STARTED.md#install-agent-commands),
[installation](docs/guides/INSTALLATION.md), or
[the Claude Code plugin guide](docs/guides/CLAUDE-PLUGIN.md).
SOP owns routing, validation, and approval for every command. Three RAG-domain
skills (`/rag-schema`, `/rag-test`, `/rag-ingest`) add a pgvector/RAG checklist on top
of the read-only capabilities, and `/sop-rag-integrate` advises whether a project needs
retrieval augmentation at all and plans a scoped integration (or recommends NO-RAG);
see [docs/guides/RAG-SKILLS.md](docs/guides/RAG-SKILLS.md).

The provider-neutral `/mcp-live-validation` skill prepares pinned MCP candidates,
produces a versioned cross-agent validation handoff, and preserves approval-gated
start, rollback, and promotion boundaries; see
[the MCP live-validation guide](docs/guides/MCP-LIVE-VALIDATION.md).

## Planner–Executor Coordination

The merged interactive planner baseline lets an executor publish progress,
blockers, test results, and completion evidence with `sop result publish`. A
planner connected through `sop mcp --planner` can retrieve task results and
questions, then record a bound `continue`, `correct`, `revise`, or `escalate`
decision. The executor applies that decision with
`sop planner continue --apply` and resumes through `sop run`.
With `sop mcp --planner --start-work`, the planner can also start work itself:
`sop_start_work` launches `sop prompt` from prompt text or `sop run` from a project
PLAN.md in the background, and reports status instead when that work is already
running, already ran, or is the active plan (see
[Starting work over MCP](docs/reference/CLI.md#starting-work-over-mcp)).
`sop-setup-planner [PROJECT_DIR]` (installed with `./install.sh --planner-setup`, or
`make setup-planner PROJECT=DIR`) sets the planner up for a project in one step; see
[Setting up the planner](docs/reference/CLI.md#setting-up-the-planner).

Planner decisions are advice, never human approval, and published results are
evidence, never automatic task completion. Resumption is currently interactive;
no unattended ChatGPT invocation or notification channel is claimed. PE-1 through
PE-4 implementation and live acceptance are merged; the merged PE-4 evidence
records all four tasks as `LOCAL_DONE`. Completing the plan remains a separate
operator-local SOP lifecycle action and is not inferred from that repository
evidence. See the
[planner–executor implementation plan](docs/plans/PLAN-Executore-Integration.md),
[PE-4 acceptance evidence](docs/reports/evidence/artifacts/pe4-acceptance/README.md),
and [CLI reference](docs/reference/CLI.md).

Optional evaluation tooling is also available outside the default execution
path: [`sop compare`](docs/guides/HIP-COMPARE.md) runs bounded
baseline/candidate comparisons, and the
[Promptfoo adapter](docs/security-promptfoo.md) performs report-only behavioral
security evaluations. Neither tool can promote a candidate or alter deterministic
execution governance.

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

SOP V1 and the subsequent routing, provider, recovery, distribution, approval,
agentic reliability, and context-efficiency layers are implemented. Optional local decision memory is implemented but disabled by default pending a valid held-out benefit evaluation. See
[Project Status](docs/reference/PROJECT-STATUS.md) for delivery evidence and known
limitations, [Backlog](docs/plans/BACKLOG.md) for future work, and the
[performance baseline](docs/reports/PERFORMANCE-BASELINE.md) for measured results
and their scope. Repository consolidation is complete; its
[evidence and compatibility boundaries](docs/guides/REPOSITORY-CONSOLIDATION.md)
do not establish current local machine or CI status. The legacy `sop-controller`
and `sop-decision-adapters` GitHub remotes are currently archived read-only,
but archival readiness is not verified; see the
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
| Plans and historical evidence | [Plans](docs/README.md#plans), [history](docs/README.md#historical-documentation)            |

## License

Apache License 2.0
