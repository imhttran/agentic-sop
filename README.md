# SOP

SOP — **a standard operating procedure for agentic software development.**

SOP is a local-first orchestrator for running a software development lifecycle with AI agents while keeping workflow control deterministic.

> **Agents do the work. SOP controls the process.**

The basic idea is:

> **Agents do the reasoning and implementation work. The orchestrator decides what is allowed to happen next.**

Instead of giving an AI coding agent a project and allowing it to operate without structure, SOP turns development into a controlled workflow:

```text
PRD
 ↓
Plan
 ↓
Task Breakdown
 ↓
Dependency Graph
 ↓
Pick Ready Task
 ↓
Create Branch
 ↓
Write Tests
 ↓
Implement
 ↓
Run Tests
 ↓
Review
 ↓
Fix Findings
 ↓
Pull Request
 ↓
CI
 ↓
Merge
 ↓
Next Task
```

SOP is written in Go and is designed to work with different AI harnesses and model providers.

---

# Why SOP?

Coding agents are good at:

- understanding requirements
- generating plans
- writing code
- writing tests
- reviewing code
- diagnosing failures
- fixing problems

They should not necessarily be the source of truth for workflow state.

SOP separates those responsibilities.

```text
                 SOP
                      │
             ┌────────┴────────┐
             │                 │
      Deterministic         AI Agents
         Control
             │                 │
             ▼                 ▼
        Task State          Planning
        Dependencies        Coding
        Validation          Reviewing
        Scheduling          Fixing
        Retry Limits        Reasoning
        Persistence         Documentation
        Git / CI State
```

The orchestrator owns workflow control.

Agents operate inside those boundaries.

---

# Current Status

SOP V1 is complete. A small project can start from a PRD and reach a
completed `main` branch through dependency-aware task execution, isolated
branches, test-first implementation, bounded review/fix loops, GitHub CI, a
validated merge gate, durable state, resume/recovery, environment bootstrapping,
and limited parallelism.

Implemented:

- Go CLI (`sop`)
- SQLite project state with migrations
- explicit workflow state machine and bounded retry policy
- PRD → implementation plan generation (`PLAN.md`, `.agent-sdlc/plan.json`)
- Plan → Task DAG with cycle detection and atomic persistence
- deterministic, dependency-aware scheduler
- Git adapter (branch and worktree lifecycle; structured arguments, no shell)
- test runner (build / unit / integration / lint / Docker)
- provider-independent Agent harness and command adapter
- local model providers (Ollama, OpenAI-compatible llama.cpp)
- provider capability detection and routing (`agent.Checked` rejects unsupported role/provider combinations)
- explicit Git workflow commands (`sop commit`, `sop pr`) behind the human gate
- MCP server (`sop mcp`) exposing status/validate/review over stdio
- optional decision layer (deterministic provider + routing/escalation policy)
- run report command (`sop report`)
- evaluation harness (`sop eval`)
- project configuration (`.agent-sdlc/config.yaml`) with schema validation, defaults, and a generated template; the configuration selects the agent provider (the environment overrides it)
- Markdown task-file loader (`sop plan TASK.md`)
- deterministic quality gate (`PASS` / `FAIL` / `NEEDS_HUMAN`) from verification, findings, and the fix-loop budget
- command policy (`SAFE` / `REQUIRES_APPROVAL` / `DENIED`) whose project rules can only tighten the defaults
- configuration-driven validation runner (`sop validate`)
- review stage (`sop review`) over the working-tree diff, blocking on configured severities
- local run lifecycle (`sop run [PLAN.md | --task TASK.md]`) with durable run state, a bounded fix loop, a markdown/JSON run report, and dependency-aware graph execution
- TDD task runner (RED/GREEN with bounded retries)
- structured self-review and optional Open Code Review adapter
- commit and documentation gate
- GitHub adapter (push, pull requests, checks, merge)
- GitHub Actions workflow generation and a bounded CI remediation loop
- merge gate (tests + review + checks + mergeability)
- completion loop
- resume and recovery (`sop resume`)
- environment bootstrap tasks
- limited parallelism with Git worktrees
- task handoff capsules with optional, provider-independent compression

---

# Using SOP With Another Project

SOP is designed to operate **on other projects**.

It does not need to be copied into the repository it manages.

SOP is written in Go, but it manages projects written in any language — Go, Java, Kotlin, Python, JavaScript/TypeScript, Rust, C/C++, C#, or a mix of them. The project being managed does not need to be written in Go.

For example:

```text
~/workspace/
│
├── agentic-sop/              # SOP source
│
└── projects/
    ├── book-rag/             # Project being managed
    ├── medical-billing/
    └── inventory/
```

Install SOP once and run it from inside whichever project you want it to manage.

The relationship looks like this:

```text
sop                            your-project
     │                              │
     │ operates on ────────────────►│
     │                              │
     │                           PRD.md
     │                           PLAN.md
     │                           source/
     │                           .git/
     │                              │
     │                           .agent-sdlc/
     │                             state.db
     │                             plan.json
```

The target project remains a normal Git repository.

SOP adds orchestration around it.

---

# Installation

## Requirements

You currently need:

- Go
- Git
- an AI command or harness for planning

SQLite support is provided through the Go SQLite driver.

## Clone SOP

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
```

## Install the CLI

```bash
go install ./cmd/sop
```

Make sure your Go binary directory is on your `PATH`.

Typically:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

You can add that to your shell configuration if necessary.

For zsh:

```bash
echo 'export PATH="$PATH:$(go env GOPATH)/bin"' >> ~/.zshrc
source ~/.zshrc
```

Verify the installation:

```bash
sop version
```

You can now run `sop` from another project directory.

## Install the CLI and the end-to-end skill

```bash
./scripts/install.sh
```

This installs the `sop` CLI (via `go install ./cmd/sop`) and links the bundled
`sop-end-to-end` agent skill ([`.agents/sop-end-to-end`](.agents/sop-end-to-end/SKILL.md))
into `~/.agents/skills/`, so an agent can drive the workflow end to end in any
project. Set `SOP_SKILLS_DIR` to install the skill elsewhere, or run `make install`.

---

# One-Command Run

If the project has a `docs/PLAN.md` (preferred) or a `docs/PRD.md`, you can
usually just run:

```bash
sop run
```

`sop run` discovers the planning source, initializes SOP state if needed, compiles
or generates the machine plan, creates the tasks, and executes the task graph —
all idempotently. Run it again to resume; completed work is never restarted.

You can also name a specific plan or a single task:

```bash
sop run docs/PLAN-Hardening.md   # execute a specific PLAN end to end
sop run PLAN-Hardening.md        # resolves to docs/PLAN-Hardening.md
sop run --task TASK.md           # run one task through the local lifecycle
```

Several plans may coexist under `docs/`:

```text
docs/
├── PLAN.md
├── PLAN-Hardening.md
├── PLAN-Jev.md
└── PLAN-MCP.md
```

`sop run docs/PLAN-Jev.md` runs that plan independently and safely: each plan
carries its own identity (source path + `source_sha256` + plan id), so SOP never
mixes tasks from two plans. A plan is only rebuilt when its content changes, and
when it changed after tasks were created (or a different plan is requested while
another is active) SOP stops with an actionable `NEEDS_HUMAN` instead of
discarding history. The explicit steps below remain available when you want to
inspect or control each stage.

Generated artifacts stay out of the project root: machine state and run reports
live under `.agent-sdlc/` (which ignores itself for Git, so no root `.gitignore`
is needed), and a plan generated from a PRD is written to
`docs/reports/<plan-id>.md`.

---

# Quick Start

Suppose you want SOP to manage:

```text
~/workspace/projects/book-rag
```

Move into that project:

```bash
cd ~/workspace/projects/book-rag
```

The normal workflow is a single command:

```bash
cd ~/workspace/projects/book-rag
sop run
```

SOP discovers the plan, prepares state and tasks, and executes the graph.
Conceptually:

```text
docs/PLAN.md  (or docs/PRD.md)
  │
  ▼
sop run
  │
  ├────────→ .agent-sdlc/plan.json
  ├────────→ Task DAG (SQLite)
  └────────→ execute → validate → review → gate
```

The sections below then walk through the **explicit, lower-level commands**
(`sop init`, `sop plan`, `sop tasks`, `sop status`). They are useful for debugging
or when you want to inspect and control each stage:

```bash
sop init
sop plan
sop tasks
sop status
```

---

# 1. Initialize the Project (optional)

> `sop run` does this automatically. Run `sop init` only for the explicit path.

From the target project:

```bash
sop init
```

This initializes SOP state and writes a configuration template.

Your project will contain:

```text
book-rag/
├── .git/
├── source...
│
└── .agent-sdlc/
    ├── state.db
    └── config.yaml
```

`state.db` contains durable workflow state for the project.

SOP treats the current working directory as the project root.

---

# 2. Create a PRD

Create:

```text
PRD.md
```

in the project root.

For example:

```markdown
# Book RAG

Build an application that allows users to upload books and ask
questions about their contents.

## Requirements

- Go backend
- book ingestion
- text chunking
- embeddings
- hybrid retrieval
- LLM question answering
- citations

## Initial Constraints

- single application
- local development
- SQLite where practical
- Ollama-compatible model support
```

The PRD is the starting point for the planning workflow.

```text
PRD
 │
 ▼
What are we building?
```

---

# 3. Configure an AI Agent

SOP does not directly depend on a specific model provider. It communicates
through an Agent interface and ships three adapters:

| Provider   | Required            | Optional                                                                                      | Endpoint                                                                              |
| ---------- | ------------------- | --------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `command`  | `SOP_AGENT_COMMAND` | —                                                                                             | a subprocess that reads a JSON request on `stdin` and writes the response to `stdout` |
| `ollama`   | a model (below)     | `SOP_OLLAMA_BASE_URL`, `SOP_OLLAMA_TIMEOUT`                                                   | Ollama `/api/chat`                                                                    |
| `llamacpp` | —                   | `SOP_LLAMACPP_BASE_URL`, `SOP_LLAMACPP_MODEL`, `SOP_LLAMACPP_TIMEOUT`, `SOP_LLAMACPP_API_KEY` | OpenAI-compatible `/v1/chat/completions` (llama.cpp `llama-server`)                   |

Select a provider and model in `.agent-sdlc/config.yaml`:

```yaml
agent:
  provider: ollama
  model: deepseek-v4.1-flash:cloud # ollama / llamacpp; its env var overrides it
```

The environment overrides the configuration for a single run (the model too —
`SOP_OLLAMA_MODEL` beats `agent.model`):

```bash
export SOP_AGENT_PROVIDER=ollama
export SOP_OLLAMA_MODEL=deepseek-v4.1-flash:cloud
```

`ollama` needs a model from one of those two sources; with neither set the run
fails with a message naming both. The `command` provider ignores `model`.

When neither is set, the command agent is used. The pre-rename
`AGENT_SDLC_AGENT_COMMAND` is still accepted for backward compatibility.

Conceptually:

```text
SOP
      │
      ▼
    Agent
   ├── CommandAgent  → external command
   ├── Ollama        → local Ollama
   └── LlamaCpp      → OpenAI-compatible endpoint
```

This keeps the core application independent of any specific provider or harness.

### Bootstrap harness: a local Ollama coding agent

This repository ships a small, temporary command-agent harness so SOP can drive its
own plan with a local Ollama model instead of a hosted coding agent:

```bash
export SOP_AGENT_PROVIDER=command
export SOP_AGENT_COMMAND="sh scripts/sop-ollama-agent.sh"
export SOP_OLLAMA_MODEL=deepseek-v4.1-flash:cloud   # the harness default

sop run docs/PLAN-Agent-Harness-V2.md
```

`scripts/sop-ollama-agent.sh` builds `cmd/sop-ollama-agent` and runs it with the
current directory as the repository. The model gets a small set of controlled tools
(`read_file`, `write_file`, `create_file`, `list_files`, `search_files`, an
allow-listed `run_command`, `git_status`, `git_diff`) and a bounded tool loop; it
cannot escape the repository, modify `.agent-sdlc` state, or run history-changing
or destructive commands. It consumes both the JSON tool call in the model's content
and Ollama's native tool calls, so a tool-capable model works either way. SOP still
owns validation, review, retries, and human gates — the harness is an implementation
adapter only. It is expected to be superseded by
[Agent Harness V2](docs/PLAN-Agent-Harness-V2.md).

The loop is bounded per capability, not by one global number: `PLAN` gets 8 model
turns (read-only tools), `REVIEW`/`DESIGN_TESTS`/`DIAGNOSE_FAILURE` 12, and
`IMPLEMENT`/`FIX` 24 with the mutation tools. A capability that repeats a
non-progressing action is told once to conclude, then stopped early with a
diagnostic. Set `SOP_AGENT_TRACE_LOG=/path/to/trace.jsonl` to persist the safe
per-turn diagnostic trail (capability, iteration, tool, request, progress, recovery);
it never contains prompts, file contents, or secrets. The trail is also written to
stderr when a run fails.

### Command agent outcomes

So SOP can act deterministically instead of guessing from prose, a command agent
may return a **structured execution outcome** for mutating capabilities
(`IMPLEMENT`, `FIX`):

```json
{"status": "completed", "summary": "Implemented the changes.", "changes_expected": true}
{"status": "completed", "summary": "Baseline verification completed.", "changes_expected": false}
{"status": "needs_human", "reason": "Required operation needs human authorization."}
{"status": "failed", "reason": "Unable to complete the requested operation."}
```

SOP's response:

```text
completed + changes_expected true  + a non-empty change set  → validate → review → gate
completed + changes_expected true  + no changes              → FAIL (claimed changes, produced none)
completed + changes_expected false + no changes              → run configured validation, then PASS
needs_human                                                   → stop with NEEDS_HUMAN (requeued)
failed                                                        → FAIL (terminal)
```

The outcome is structural: SOP never infers `needs_human` by scanning prose for
words like “permission” or “would you like”. Any other output (plain prose, or
JSON without a recognized `status`) keeps the legacy behaviour, so existing
harnesses keep working — but they will still be judged by the change they
produced.

A `needs_human` outcome is **not terminal**: SOP requeues the task to `PLANNED`,
so the same `sop run …` retries it automatically. A retry that makes progress
(its outcome changed) spends one attempt; the budget is `max_attempts` (default
**3**), and once it is spent the task becomes `BLOCKED` instead of looping. A
repeat that reproduces the _same_ outcome is “no progress” and spends **nothing**
— the task stays `PLANNED` and a later run still retries it, so a boundary you
resolve later is picked up without a manual unlock. That case is reported as
`NEEDS_HUMAN (no change since the previous attempt: …)`. A hard `FAIL` (including
a claimed change with none produced) leaves the task `BLOCKED`; requeue it
explicitly with `sop retry <task-id>`, or requeue every `BLOCKED` task that still
has budget with `sop retry --all`. A retried task is handed the previous
attempt's outcome as context (a `# Previous attempt` section in the implement
request), so the agent can address the blocker instead of repeating the request
that stopped it.

A `changes_expected: false` completion still runs the **configured validation**
(build/test/lint) before it can pass; review is skipped because there is nothing
to review.

---

# 4. Generate the Implementation Plan (optional)

> `sop run` compiles or generates `plan.json` automatically. Use `sop plan`
> explicitly to generate a PLAN from a PRD, or `sop run docs/PLAN.md` to compile
> a human-written PLAN.

Run:

```bash
sop plan
```

The flow is:

```text
PRD.md
   │
   ▼
 Planner
   │
   ▼
 Agent
   │
   ▼
 AI Model
   │
   ▼
 JSON Response
   │
   ▼
 Plan
   │
   ▼
 Validate
   │
   ├─────────────→ PLAN.md
   │
   └─────────────→ .agent-sdlc/plan.json
```

The model does **not** directly write the plan files.

The model generates structured content.

The application:

1. parses it
2. validates it
3. converts it into a typed Plan
4. renders Markdown
5. writes the artifacts

---

# Human vs Machine Plan

Planning produces two representations:

```text
                  Plan
                 /    \
                /      \
               ▼        ▼
          PLAN.md     plan.json
           human       machine
```

## `PLAN.md`

Intended for humans.

Example:

```markdown
# Implementation Plan

## Project

Book RAG

## S001 — Application Skeleton

Create the initial Go application.

### Dependencies

None

### Deliverables

- Go application
- configuration

### Acceptance Criteria

- application starts successfully
```

A stage may also declare how it executes:

```markdown
### Execution

- verify-first
```

`verify-first` runs the configured validation before any agent (see
[Verification-first tasks](#verification-first-tasks)). Omit the section for the
ordinary implement-first behaviour.

## `.agent-sdlc/plan.json`

Intended for the orchestrator.

Example:

```json
{
  "project": "Book RAG",
  "summary": "Build a book question answering system.",
  "stages": [
    {
      "id": "S001",
      "title": "Application Skeleton",
      "objective": "Create the Go application.",
      "dependencies": [],
      "deliverables": ["Go application"],
      "acceptance_criteria": ["Application starts successfully"]
    }
  ]
}
```

SOP does not parse `PLAN.md` to determine executable workflow state.

Structured data is used instead.

A stage may add `"execution_mode": "verify-first"` to run the configured
validation before any agent (see [Verification-first
tasks](#verification-first-tasks)); it is omitted for the implement default.

---

# 5. Create the Task Graph (optional)

> `sop run` creates missing tasks automatically; use `sop tasks` only for the
> explicit path.

Once the plan exists, run:

```bash
sop tasks
```

SOP reads:

```text
.agent-sdlc/plan.json
```

and converts each plan stage into a task.

```text
Plan
 │
 ▼
Task Builder
 │
 ▼
Tasks
 │
 ▼
DAG Validation
 │
 ▼
SQLite
```

This operation is deterministic.

No LLM is called.

---

# Dependency Graph

Tasks contain explicit dependencies.

For example:

```text
        S001
       /    \
      ▼      ▼
    S002    S003
       \    /
        ▼  ▼
        S004
```

means:

```text
S002 depends on S001
S003 depends on S001
S004 depends on S002 and S003
```

SOP validates the graph before storing the task set.

Cycles are rejected.

For example:

```text
S001
 ↓
S002
 ↓
S003
 ↓
S001
```

is invalid.

---

# Atomic Task Creation

Task creation is transactional.

Suppose the plan contains:

```text
S001
S002
S003
S004
```

SOP will not leave:

```text
S001 ✓
S002 ✓
S003 ✗
S004 ?
```

if something fails.

Instead:

```text
BEGIN

create tasks
create dependency edges
create attempt state

success
   ↓
COMMIT

failure
   ↓
ROLLBACK
```

The task set is either persisted successfully or the database remains unchanged.

---

# Existing Task Protection

SOP does not silently overwrite existing workflow state.

If:

```text
S001
```

already exists in SQLite and a new task generation attempts to create `S001`, the operation fails.

This protects information such as:

- workflow status
- attempt count
- failure history
- blocked state
- future execution history

Updating an existing plan will eventually have its own explicit workflow.

---

# 6. Inspect Tasks

List the project's tasks:

```bash
sop status
```

Example:

```text
S001 PLANNED Application skeleton
S002 PLANNED Book ingestion
S003 PLANNED Embeddings
S004 PLANNED Retrieval
```

Inspect a specific task:

```bash
sop task S002
```

SQLite remains the durable source of truth for task execution state.

---

# CLI

Currently available commands:

```text
sop init
sop plan [TASK.md]
sop tasks
sop validate
sop review
sop run [PLAN.md | --task TASK.md]
sop report [run-id]
sop commit [TASK.md] [--yes]
sop pr TASK.md [--yes]
sop mcp
sop eval DIR
sop status
sop task <id>
sop resume [task-id]
sop retry <task-id> | --all
sop version
sop help
```

## `init`

Initialize SOP state and generate the configuration template:

```bash
sop init
```

Creates `.agent-sdlc/state.db` and, when absent, `.agent-sdlc/config.yaml`.
Re-running it is safe: existing state and a hand-edited configuration are never
overwritten.

## `plan`

Generate an implementation plan from `PRD.md`, or from a Markdown task file:

```bash
sop plan
sop plan TASK.md
```

A task file is parsed into ID, title, description, requirements, acceptance
criteria, and constraints, then normalized before the agent reasons over it.

Produces:

```text
PLAN.md
.agent-sdlc/plan.json
```

## `tasks`

Convert the machine-readable Plan into persisted tasks:

```bash
sop tasks
```

## `validate`

Run the configured build/test/lint commands and report a deterministic result:

```bash
sop validate
```

Commands come from `validation` in `.agent-sdlc/config.yaml`; they run in the
project directory, in order (build, test, lint), and stop at the first failure
so uncompilable changes are not carried forward. The exit code is non-zero on
failure.

## `review`

Review the current working-tree changes with the configured engine:

```bash
sop review
```

The `self` engine (default) asks the agent for structured findings; the
`open-code-review` engine runs an external command (`SOP_REVIEW_COMMAND`). The
model never decides the verdict: findings whose severity is named in
`quality.fail_on` are blocking, and the exit code is non-zero when any remain.
When there are no working-tree changes it reports “no changes to review”.

## `run`

Run a task file through the local lifecycle, or drive the persisted task graph:

```bash
sop run                        # the project's normal plan (discovered)
sop run docs/PLAN-Hardening.md # a specific plan, executed end to end
sop run --task TASK.md         # one task file
```

```text
task → plan → implement → detect changes
     → { validate → review → gate → fix }   (≤ quality.max_fix_cycles)
     → report
```

`sop run` is the **one-command workflow**: it discovers or resolves the planning
source, initializes state if needed, compiles or generates
`.agent-sdlc/plan.json`, creates the tasks, then drives the graph — each step
idempotent. A file argument names an **execution PLAN** (authoritative for that
run); `--task TASK.md` runs a single task through the local lifecycle instead.
Source precedence (when no plan is named):

```text
1. an existing valid .agent-sdlc/plan.json (reused only if its source is unchanged)
2. docs/PLAN.md
3. PLAN.md
4. docs/PRD.md
5. PRD.md
```

A human PLAN is preferred over the PRD, and `plan.json` never silently overrides
a newer PLAN: the source path and a content fingerprint are recorded, and a
changed document triggers a rebuild. `sop init`, `sop plan`, and `sop tasks`
remain the explicit lower-level commands.

Preparation does not require an agent: `sop run` compiles a `PLAN.md` and creates
tasks even when no agent is configured, then reports the missing agent only when
it needs to execute (or to generate a plan from a PRD).

Multiple plans may coexist under `docs/` (`PLAN.md`, `PLAN-Hardening.md`,
`PLAN-Jev.md`, …). Each carries its own identity (source path, `source_sha256`,
and a plan id), so `sop run docs/PLAN-Jev.md` runs that plan independently. If the
named plan's content changed after tasks were created, or a different plan is
requested while another is active, `sop run` stops with an actionable
`NEEDS_HUMAN` rather than mixing tasks from two plans.

The **scheduler** selects the next ready task and the same lifecycle runs for it;
a passing gate marks the task `LOCAL_DONE` and a failing gate marks it `BLOCKED`,
repeating until no runnable work remains. When a task is already in flight (for
example a run was interrupted), `sop run` **resumes that task** — from the same
persisted state → next-action rule that `sop resume` reports — instead of stopping
with `ACTIVE_TASK`; a task whose gates already passed is completed locally without
re-invoking the agent. `ACTIVE_TASK` still prevents starting a _different_ task: if
two tasks are in flight, or the active task cannot be safely resumed, `sop run`
stops with an actionable error naming the task and the recovery command. Local
execution has no remote PR/CI/merge, so a passing lifecycle advances the task to
`LOCAL_DONE` directly (no synthetic PR/CI/merge states; controlled by
`workflow.mode`).

Change detection includes untracked files (except SOP's own output), so a file the
agent creates counts even when no tracked file changed. The startup summary also
prints the effective provider and whether configuration or the environment
selected it, and a run rejects a provider that cannot `IMPLEMENT` before running
any task.

Every stage writes an artifact under `.agent-sdlc/runs/<id>/` (`task.md`,
`plan.md`, `implementation.md`, `diff.patch`, `fix-N.md`, `validation.json`,
`review.json`, `report.md`, `report.json`, `state.json`), so a run stays
inspectable. Git is the authority on what changed; validation fails fast before
review; a blocking finding is sent back to the agent and then validation and
review run again, up to `quality.max_fix_cycles` times — exhausting the budget
yields `NEEDS_HUMAN` rather than looping forever. The quality gate is
deterministic. `sop run` **stops at the human gate** and never commits, pushes,
or merges.

## `report`

Print a concise, informational summary of a run (the latest by default):

```bash
sop report
sop report T001
```

Shows stage, provider, gate, per-check validation, and findings by severity.

## `commit`

Commit the current changes with a message derived from a task file (no push):

```bash
sop commit TASK.md --yes
```

When `human.approval_before_commit` is on (the default), `--yes` is required.

## `pr`

Push a task branch and open a pull request:

```bash
sop pr TASK.md --yes
```

The branch is `task/<id>-<slug>` and the base is the integration branch. It never
merges.

## `mcp`

Serve tools over the Model Context Protocol on stdio:

```bash
sop mcp
```

Exposes `sop_status`, `sop_validate`, and `sop_review`, backed by the same
services as the CLI.

## `eval`

Run a corpus of task files through the lifecycle and report benchmark metrics:

```bash
sop eval corpus/
```

## `status`

Show project task state:

```bash
sop status
```

## `task`

Inspect a particular task:

```bash
sop task S001
```

## `resume`

Report the next legal action for interrupted work. With no id it resumes the
single in-flight task:

```bash
sop resume
sop resume S001
```

The same action rule drives `sop run`: when a task is in flight, `sop run` resumes
it (completing it locally if its gates already passed, running the normal lifecycle
if it is still before them) rather than refusing to continue.

## `retry`

Requeue a `BLOCKED` task to `PLANNED` so the next `sop run` retries it, or requeue
every `BLOCKED` task that still has retry budget at once:

```bash
sop retry S001
sop retry --all
```

A requeue spends one attempt against `max_attempts`; a task whose budget is spent
is reported and left `BLOCKED`. The retry resumes the same task and is given the
previous attempt's outcome as context.

---

# Configuration

`sop init` generates `.agent-sdlc/config.yaml`. Configuration describes policy
only: it never holds mutable task state and must not contain secrets. Unknown
keys are rejected, so credentials cannot be committed by accident — agent
credentials stay in the environment.

```yaml
version: 1

project:
  name: book-rag
  integration_branch: main

agent:
  provider: command # command | ollama | llamacpp

validation: # commands run by the verification stages
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
  fail_on: # severities that block a pass
    - critical
    - high

human:
  approval_before_commit: true

workflow:
  mode: local # local | pull-request
```

Omitted fields take safe defaults (documented by the generated template). An
invalid file — malformed YAML, an unknown key, an unknown provider/engine/
severity, a missing `project.name`, or an unsupported version — fails with a
clear message rather than a silent fallback.

The environment overrides configuration where it matters; for example
`SOP_AGENT_PROVIDER` overrides `agent.provider`.

---

# Architecture

The architecture separates reasoning from control.

```text
                        CLI
                         │
                         ▼
                   Orchestrator
                         │
       ┌─────────────────┼─────────────────┐
       │                 │                 │
       ▼                 ▼                 ▼
    Planner          Scheduler         Task Runner
       │                 │                 │
       ▼                 │                 ▼
 Agent Harness            │             Git/Test
       │                  │             Review/CI
       ▼                  │
      LLM                 │
                          ▼
                     State Store
                          │
                          ▼
                        SQLite
```

Every component shown above is implemented. Agents (the LLM boundary) propose
and execute work; the orchestrator decides the workflow state and what is
allowed to happen next.

---

# Core Design Rule

The central design rule is:

```text
The LLM creates content.

The application controls the process.
```

For example, during planning:

```text
LLM
 │
 │ proposes
 ▼
Plan JSON
 │
 │ validated by
 ▼
Go Application
 │
 ├── valid ──────→ continue
 │
 └── invalid ────→ stop
```

The LLM does not decide whether its own output is valid.

---

# Workflow State Machine

Tasks move through an explicit state machine.

```text
PLANNED
   ↓
READY
   ↓
BRANCH_CREATED
   ↓
TESTS_WRITTEN
   ↓
RED_VERIFIED
   ↓
IMPLEMENTING
   ↓
LOCAL_TESTS_PASS
   ↓
REVIEW
   ↓
REVIEW_PASS
   ↓
PR_OPEN
   ↓
CI_RUNNING
   ↓
CI_PASS
   ↓
MERGED
   ↓
DONE
```

Only legal state transitions are allowed.

For example:

```text
PLANNED → READY
```

is legal.

Something like:

```text
PLANNED → MERGED
```

is not.

---

# Remediation

Failures do not necessarily terminate the workflow.

Several states can move into:

```text
FIX_REQUIRED
```

For example:

```text
IMPLEMENTING ───────┐
LOCAL_TESTS_PASS ───┤
REVIEW ─────────────┼──→ FIX_REQUIRED
CI_RUNNING ─────────┘          │
                               ▼
                         IMPLEMENTING
```

Retries are bounded.

If recovery cannot continue safely, the task can eventually become:

```text
BLOCKED
```

`BLOCKED` and `DONE` are terminal states.

---

# Test-Driven Development

The intended implementation workflow follows TDD.

```text
Requirement
 ↓
Acceptance Criteria
 ↓
Write Test
 ↓
Verify RED
 ↓
Implement
 ↓
Verify GREEN
 ↓
Refactor
 ↓
Run Affected Suite
```

A task should not claim successful TDD simply because tests pass.

Where applicable, the system should verify that the new test initially failed before implementation.

If RED verification is not applicable, that should be explicitly recorded rather than fabricated.

---

# Review Pipeline

The intended review pipeline contains multiple layers.

```text
Implementation
      │
      ▼
 Local Tests
      │
      ▼
 Self Review
      │
      ▼
 Open Code Review
      │
      ▼
 Fix Findings
      │
      ▼
 Pull Request
      │
      ▼
 CI
```

Self-review focuses on:

- acceptance criteria
- correctness
- tests
- edge cases
- error handling
- architecture boundaries
- unnecessary complexity

External automated review provides an independent second pass.

Correctness findings should be fixed.

Suggestions that only add unnecessary complexity can be declined.

---

# Git Workflow

The intended task workflow uses predictable Git naming.

Branches:

```text
task/<id>-<slug>
```

Example:

```text
task/S004-hybrid-retrieval
```

Commits:

```text
task(<id>): implement <feature>
```

Example:

```text
task(S004): implement hybrid retrieval
```

Pull requests:

```text
[Task <id>] <title>
```

Example:

```text
[Task S004] Implement hybrid retrieval
```

Git automation is part of the upcoming execution stages.

---

# Scheduler

The next major component is the scheduler.

The scheduler will inspect persisted task state and determine which tasks are ready.

Example:

```text
       S001 DONE
       /       \
      ▼         ▼
S002 PLANNED  S003 PLANNED
      \         /
       ▼       ▼
         S004
```

Because `S001` is complete:

```text
S002 → READY
S003 → READY
```

But:

```text
S004 → PLANNED
```

because its dependencies have not completed.

The scheduler will answer:

> Given the current task states and dependency graph, which tasks are legally allowed to run now?

That decision remains deterministic.

---

# Run Lifecycle

The task-file lifecycle is implemented:

```bash
cd my-project

sop init
sop run docs/PLAN.md
```

`sop run` drives task → plan → implement → detect changes → validate → review →
quality gate → report, writing artifacts under `.agent-sdlc/runs/<id>/` and
stopping at the human gate without committing.

With no task file it drives the persisted task graph: the scheduler selects the
next ready task and the same lifecycle runs for it, completing each task locally at
`LOCAL_DONE` (never fabricating remote states) until no runnable work remains. A
verify-first task runs the configured validation before any agent (see
[Verification-first tasks](#verification-first-tasks)).

```text
                    Task DAG
                       │
                       ▼
                  Scheduler
                       │
                       ▼
                 Ready Task
                       │
                       ▼
               Create Branch
                       │
                       ▼
                 Write Tests
                       │
                       ▼
                  Verify RED
                       │
                       ▼
                  Implement
                       │
                       ▼
                 Run Tests
                       │
                       ▼
                    Review
                       │
                 ┌─────┴─────┐
                 │           │
               PASS      FIX_REQUIRED
                 │           │
                 │           └──────┐
                 ▼                  │
              Commit                │
                 │                  │
                 ▼                  │
             Pull Request           │
                 │                  │
                 ▼                  │
                  CI                │
                 │                  │
          ┌──────┴──────┐           │
          │             │           │
        PASS           FAIL ─────────┘
          │
          ▼
        Merge
          │
          ▼
        DONE
          │
          ▼
     Next Ready Task
```

## Verification-first tasks

A plan stage may declare `execution_mode: verify-first` (in PLAN.md, an
`### Execution` sub-section with `- verify-first`; in a task file, a
`## Execution` section). Such a task runs the configured validation **before** any
agent is invoked:

```text
verify-first
     │
     ▼
  validate ── pass ──→ quality gate → complete locally   (no agent invoked)
     │
    fail
     ▼
  implement (input: the validation failure) → validate → review → gate → fix*
```

A pass invokes no agent at all — not the micro-planner, not IMPLEMENT, not FIX —
and the task passes only because its configured checks passed. A failure hands the
deterministic failure to the implementation agent as context and the ordinary
lifecycle continues, so a verification failure is not automatically terminal. A
verify-first task with no configured check falls back to the ordinary
implementation path: a fast path that verifies nothing is not a verification.

The mode is explicit plan/task metadata, never inferred from a task's title.
Without it a task keeps the implement-first behaviour.

---

# Controlled Parallelism

The dependency DAG allows independent tasks to run concurrently, up to a
configured bound, each in its own Git worktree.

For example:

```text
         S001
        /    \
       ▼      ▼
     S002    S003
       │      │
       └──┬───┘
          ▼
         S004
```

After `S001` completes:

```text
S002
```

and:

```text
S003
```

could potentially execute concurrently.

Parallel execution will remain bounded rather than allowing unrestricted agent spawning.

The initial target is a small configurable concurrency limit.

---

# Persistence and Recovery

Workflow state is stored in SQLite:

```text
.agent-sdlc/
└── state.db
```

This is important because an agentic workflow may run much longer than a single process.

If execution stops:

```text
Task A DONE
Task B DONE
Task C IMPLEMENTING
Task D PLANNED
```

the system should be able to restart and determine where it left off.

Future commands include:

```text
sop run
sop resume
sop retry <task>
```

The database, rather than model conversation history, is the source of truth.

---

# Development Principles

## Deterministic Control, Agentic Execution

Use AI where reasoning is useful.

Use normal software where correctness and state management matter.

Good AI responsibilities:

```text
Planning
Coding
Review
Failure Analysis
Documentation
```

Good deterministic responsibilities:

```text
State Transitions
Dependency Validation
Scheduling
Persistence
Retry Limits
Git State
CI State
Merge Gates
```

---

## Machine State Is Not Markdown

Markdown is useful for people.

Structured data is used for executable state.

```text
Human                         Machine

PRD.md
PLAN.md          ←──────→     plan.json
                              tasks
                              dependencies
                              attempts
                              SQLite
```

The orchestrator should not depend on parsing human documentation to understand its own state.

---

## Bounded Autonomy

Agents should not retry indefinitely.

For example:

```text
Attempt 1
   ↓
fail
   ↓
Attempt 2
   ↓
fail
   ↓
Attempt 3
   ↓
fail
   ↓
BLOCKED
```

The exact behavior depends on the workflow stage, but retries must have explicit limits.

---

## Recoverability

The system should be able to answer:

```text
What happened?

What state are we in?

What should happen next?
```

after a crash, process restart, agent failure, CI failure, or human interruption.

---

## Local First

The orchestrator runs locally.

The architecture should support:

- local models
- cloud models
- local Git
- local development environments
- local SQLite state
- external CI where appropriate

A cloud service should not be required simply to run the orchestrator.

---

## Provider Independence

Application components should depend on small interfaces rather than model-provider SDKs.

For example:

```go
type Agent interface {
    Generate(
        ctx context.Context,
        request Request,
    ) (Response, error)
}
```

This allows model providers and agent harnesses to change without rewriting the planning or orchestration logic.

---

# Testing

AI boundaries should be replaced with deterministic fakes in unit tests.

Tests should not require:

- a running LLM
- API keys
- network access
- cloud services

Run the test suite:

```bash
go test ./...
```

Run race detection:

```bash
go test -race ./...
```

Verify operation without CGO:

```bash
CGO_ENABLED=0 go test ./...
```

Run all project checks:

```bash
make check
```

---

# Development Roadmap

V1 is complete; every V1 stage below is implemented and tested.

```text
✓ Repository / CI
✓ Domain Model
✓ Workflow State Machine
✓ SQLite Persistence
✓ Persistence Corrections
✓ CLI Foundation
✓ PRD → Plan
✓ Plan → Tasks
✓ Dependency DAG
✓ Scheduler
✓ Git Adapter
✓ Test Runner
✓ Agent Harness
✓ TDD Task Runner
✓ Structured Self Review
✓ Open Code Review Integration
✓ Commit / Documentation Gate
✓ GitHub Adapter
✓ GitHub Actions Integration
✓ CI Remediation
✓ Merge Gate
✓ Completion Loop
✓ Resume / Recovery
✓ Environment Bootstrap
✓ Controlled Parallel Execution
✓ Documentation Automation
✓ End-to-End Dogfooding
```

Implemented on top of the V1 core (wrap-up work):

```text
✓ Local model providers (Ollama, OpenAI-compatible llama.cpp)
✓ Project configuration (.agent-sdlc/config.yaml)
✓ Configuration-driven agent selection
✓ Markdown task-file loader (sop plan TASK.md)
✓ Deterministic quality gate
✓ Command policy
✓ Validation runner (sop validate)
✓ Review stage (sop review)
✓ Run lifecycle and run state (sop run)
✓ Bounded fix loop (review → fix → re-validate)
✓ Dependency-aware graph execution (sop run over the task graph)
✓ Provider capability detection
✓ Git workflow commands (sop commit, sop pr)
✓ MCP server (sop mcp)
✓ Optional decision layer (deterministic + routing)
✓ Run report command (sop report)
✓ Evaluation harness (sop eval)
```

Next candidates, in the plan's build order:

```text
  Interactive approval workflow
  Local network service (team mode)
  Small-device dashboard
  Jev adapter + Jev-vs-deterministic evaluation
```

The project is intentionally built incrementally: sequential correctness and
recovery come before parallelism, and each stage is usable and tested before the
next is added.

---

# Target End-to-End Workflow

The eventual workflow is:

```text
PRD
 ↓
PLAN
 ↓
TASK DAG
 ↓
┌─────────────────────────────────┐
│          Orchestrator           │
│                                 │
│  Pick Ready Task                │
│          ↓                      │
│  Create Branch                  │
│          ↓                      │
│  Write Tests                    │
│          ↓                      │
│  Verify RED                     │
│          ↓                      │
│  Implement                      │
│          ↓                      │
│  Verify GREEN                   │
│          ↓                      │
│  Review                         │
│          ↓                      │
│  Fix Findings                   │
│          ↓                      │
│  Commit                         │
│          ↓                      │
│  Pull Request                   │
│          ↓                      │
│  CI                             │
│          ↓                      │
│  Fix CI Failures                │
│          ↓                      │
│  Merge                          │
│          ↓                      │
│  Update State / Documentation   │
└────────────────┬────────────────┘
                 │
                 ▼
           Next Ready Task
                 │
                 ▼
               DONE
```

---

# What SOP Is Not

SOP is not intended to be an unrestricted autonomous coding agent.

It is not:

```text
Prompt
  ↓
LLM
  ↓
"Go change whatever you want"
```

Instead:

```text
             Orchestrator
                  │
          defines boundaries
                  │
                  ▼
                Agent
                  │
              performs
           specific work
                  │
                  ▼
             Validation
                  │
                  ▼
            State Change
```

The orchestrator remains responsible for deciding whether the workflow can continue.

---

# Project Goal

SOP explores a practical question:

> **How much of a real software development lifecycle can be automated with AI while keeping control, state, safety, and recovery deterministic?**

The goal is a software engineering workflow where AI agents operate inside explicit boundaries and the system can always answer:

```text
What happened?

What state are we in?

What can happen next?

What failed?

Can we retry?

Do we need a human?
```

That is the foundation for running long-lived agentic software development workflows without giving up control of the SDLC.

---

# License

Apache License 2.0
