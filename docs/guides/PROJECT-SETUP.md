# Project Setup

This guide covers the explicit, step-by-step path for putting a project under
SOP: initialize state, write a PRD, configure an agent, generate a plan, create
the task graph, and inspect the result. `sop run` performs these same steps
automatically (see [GETTING-STARTED.md](GETTING-STARTED.md)); use the explicit
commands to control or inspect each stage. Related references:
[../reference/CLI.md](../reference/CLI.md),
[../reference/CONFIGURATION.md](../reference/CONFIGURATION.md),
[../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md).

## SOP operates on the project, not inside it

SOP is not copied into the projects it manages. You install the `sop` CLI once
and run it from the target project's directory. SOP treats the current working
directory as the project root and keeps its own state in a `.agent-sdlc/`
directory inside that project.

## 1. Initialize the project

```bash
sop init
```

This initializes SOP state and writes a configuration template. The project then
contains `.agent-sdlc/state.db` (durable workflow state) and
`.agent-sdlc/config.yaml` (a policy template — never mutable task state, never
secrets; credentials stay in the environment).

## 2. Create a PRD

Create `PRD.md` in the project root — the starting point for the planning
workflow. It describes what you are building (for example a Book RAG app with a
Go backend, book ingestion, chunking, embeddings, hybrid retrieval, LLM question
answering, and citations); the plan then describes how. The PRD answers _what are
we building?_, the plan answers _how_.

## 3. Configure an agent

The generated `.agent-sdlc/config.yaml` describes policy: harness, provider,
model, and the validation, review, quality, human, and workflow settings. A
typical tool-harness setup:

```yaml
agent:
  harness: tool
  provider: ollama
  model: deepseek-v4.1-flash:cloud
```

Environment variables override configuration where it matters (for example
`SOP_AGENT_PROVIDER` overrides `agent.provider`). For the harness/provider/model
model, see [../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md); for the full
key list and precedence, see
[../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## 4. Generate the plan

```bash
sop plan
```

The PRD flows through the planner, the agent, and the model to a structured JSON
response, which SOP validates and turns into a typed plan: the model generates
structured content, and the application parses, validates, converts, renders
Markdown, and writes the artifacts.

Planning produces two representations of the same plan — `PLAN.md` for people
(stages with dependencies, deliverables, and acceptance criteria) and
`.agent-sdlc/plan.json` for the orchestrator. SOP does not parse `PLAN.md` to
determine executable workflow state; structured data is used instead.

A stage can declare how it executes: `### Execution / - verify-first` in
Markdown, or `"execution_mode": "verify-first"` in JSON. A verification-first
stage runs the configured validation before any agent; omitting it keeps the
ordinary implement-first behaviour.

## 5. Create the task graph

```bash
sop tasks
```

SOP reads `.agent-sdlc/plan.json`, converts each plan stage into a task, and
stores the set in SQLite. This step is deterministic — no LLM is called.

Tasks carry explicit dependencies that form a DAG (here `S002` and `S003` depend
on `S001`, and `S004` depends on both):

```text
        S001
       /    \
    S002    S003
       \    /
        S004
```

SOP validates the graph before storing the task set, and cycles are rejected.

Task creation is transactional: SOP creates tasks, dependency edges, and attempt
state in one transaction, so the set is either persisted completely or the
database is left unchanged — no half-written graph.

SOP does not silently overwrite existing workflow state. If `S001` already
exists and a new generation attempts to create it, the operation fails,
protecting status, attempt count, failure history, blocked state, and execution
history.

## 6. Inspect tasks

```bash
sop status     # list tasks, e.g. S001 PLANNED Application skeleton
sop task S002  # inspect one task
```

SQLite remains the durable source of truth for task execution state.
