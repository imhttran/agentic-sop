# T004 — PRD to Plan

## Status

DONE

## Goal

Add a planner that reads a project PRD, asks an LLM to create an
implementation plan, validates the response, and writes PLAN.md.

T004 introduces the first agent/LLM boundary.

The application owns:

- reading PRD.md
- constructing the request
- invoking the agent
- validating output
- writing PLAN.md
- errors and exit codes

The LLM owns:

- reasoning about the PRD
- proposing implementation stages
- describing goals, dependencies, acceptance criteria, and risks

The LLM must not directly manipulate the filesystem or workflow state.

---

## Branch

task/T004-prd-to-plan

Commit:

task(T004): add PRD to plan generation

PR:

[Task T004] Add PRD to plan generation

---

## Architecture

Desired flow:

agent-sdlc plan
│
▼
Planner
│
├── Read PRD.md
│
├── Build prompt
│
├── Agent.Generate()
│
├── Validate result
│
└── Write PLAN.md
│
▼
done

Package direction:

internal/cli
│
▼
internal/planner
│
▼
internal/agent

The planner may depend on the Agent interface.

The Agent implementation must not depend on Planner.

---

## Step 1 — Define the Agent Boundary

Create a small interface.

Conceptually:

type Agent interface {
Generate(ctx context.Context, request Request) (Response, error)
}

Keep Request and Response minimal.

For T004 the request needs enough information to express:

- task/purpose
- PRD content
- output requirements

Do not expose provider-specific concepts in Planner.

Avoid types such as:

OllamaRequest
OpenAIRequest
ClaudeRequest

inside planner.

The planner should know only:

Agent

not:

Ollama
OpenAI
Claude
OCR

---

## Step 2 — Fake Agent First

Before connecting a real model, create a fake/test agent.

Architecture:

Planner
│
▼
Agent
▲
│
┌──┴─────────────┐
│ │
FakeAgent RealAgent
tests later

FakeAgent should return deterministic output.

This allows planner behavior to be tested without:

- network
- Ollama
- API keys
- model availability
- nondeterministic responses

---

## Step 3 — Define the Plan Contract

Do not let the LLM return arbitrary prose.

For T004, define a structured plan representation.

Example concept:

Plan
├── Project
├── Summary
└── Stages[]
├── ID
├── Title
├── Objective
├── Dependencies[]
├── Deliverables[]
└── AcceptanceCriteria[]

Use typed Go structures.

Do not use PLAN.md as the machine representation.

Machine:

Plan struct

Human:

PLAN.md

This distinction will matter in T005.

---

## Step 4 — Choose the Agent Output Format

Have the agent return structured JSON representing Plan.

Example:

{
"project": "Book RAG",
"summary": "...",
"stages": [
{
"id": "S001",
"title": "Application skeleton",
"objective": "...",
"dependencies": [],
"deliverables": [
"Go application"
],
"acceptance_criteria": [
"Application starts successfully"
]
}
]
}

The application parses JSON into Plan.

Do not parse Markdown produced by the LLM.

Flow:

LLM
↓
JSON
↓
json.Unmarshal
↓
Plan
↓
Validate
↓
Markdown renderer
↓
PLAN.md

---

## Step 5 — Plan Validation

Create deterministic validation.

At minimum:

- project is not empty
- summary is not empty
- at least one stage exists
- stage ID is not empty
- stage IDs are unique
- title is not empty
- objective is not empty
- acceptance criteria are not empty
- dependency IDs refer to existing stages
- stage cannot depend on itself

Do not ask another LLM whether the plan is valid.

Validation belongs to Go.

---

## Step 6 — Keep DAG Validation Limited

T004 may validate references.

Do not build the complete task DAG engine yet.

T005 owns:

Plan
↓
Task decomposition
↓
Task DAG

For T004 we only need enough validation to reject obviously invalid
planner output.

---

## Step 7 — Markdown Renderer

Create a deterministic renderer:

Plan
↓
RenderMarkdown()
↓
PLAN.md

Example:

# Implementation Plan

## Project

Book RAG

## Summary

...

## S001 — Application skeleton

Objective...

### Dependencies

None

### Deliverables

- Go application

### Acceptance Criteria

- Application starts successfully

Markdown should come from Go.

Do not ask the LLM to format PLAN.md.

This gives us repeatable output and keeps presentation separate from
reasoning.

---

## Step 8 — Planner Service

Planner should conceptually expose:

Generate(ctx, prd string) (*Plan, error)

Responsibilities:

validate PRD input
↓
build agent request
↓
agent.Generate()
↓
parse JSON
↓
validate Plan
↓
return Plan

Planner should NOT:

- write files
- open SQLite
- modify Task state
- execute Git
- call GitHub
- invoke CI

---

## Step 9 — File/Application Layer

Add the operation that handles:

PRD.md
↓
read
↓
Planner.Generate()
↓
Plan
↓
RenderMarkdown()
↓
PLAN.md

Keep filesystem behavior outside the core planner where practical.

This makes Planner easy to test with strings.

---

## Step 10 — Add CLI Command

Extend:

agent-sdlc plan

Behavior:

current directory
↓
PRD.md
↓
Planner
↓
PLAN.md

Errors:

missing PRD.md
→ exit 1

empty PRD
→ exit 1

agent failure
→ exit 1

invalid model response
→ exit 1

PLAN.md write failure
→ exit 1

bad CLI arguments
→ exit 2

---

## Step 11 — Protect Existing PLAN.md

Do not silently overwrite an existing PLAN.md.

For T004:

if PLAN.md exists
↓
return error

Do not add --force yet unless there is a clear need.

A planning agent should not destroy an existing human-edited plan.

---

## Step 12 — Agent Configuration

Do not hard-code one model provider into Planner.

Use configuration/environment at the real-agent boundary.

Conceptually:

AGENT_SDLC_AGENT_COMMAND

or equivalent configuration.

For V1, a subprocess-backed agent adapter is acceptable.

Example architecture:

Agent interface
│
▼
CommandAgent
│
▼
configured CLI / harness
│
▼
LLM

This fits the goal of working with different coding/model harnesses.

Do not build separate Ollama/OpenAI/Claude SDK integrations in T004.

---

## Step 13 — Context Cancellation

Agent.Generate should accept context.Context.

This gives future orchestration control over:

- cancellation
- timeout
- shutdown

Do not bake arbitrary sleeps into planner code.

---

## Step 14 — Prompt Contract

The planner prompt should clearly tell the model:

Input:
PRD

Task:
Create an implementation plan.

Output:
JSON only matching the supplied schema.

Requirements:

- implementation stages
- clear objectives
- explicit dependencies
- concrete deliverables
- testable acceptance criteria
- avoid implementation outside PRD scope
- prefer small stages
- preserve PRD terminology

The prompt should be a stable application asset, not scattered across
CLI code.

---

## Step 15 — TDD: Planner Happy Path

Using FakeAgent:

PRD
↓
FakeAgent returns valid JSON
↓
Planner
↓
Plan

Verify:

project
summary
stages
dependencies
acceptance criteria

No real LLM.

---

## Step 16 — TDD: Invalid Agent Output

Test:

malformed JSON
missing project
no stages
duplicate stage IDs
missing objective
missing acceptance criteria
unknown dependency
self dependency

Each should fail deterministically.

---

## Step 17 — TDD: CLI/File Flow

Using a temporary project directory:

PRD.md
│
▼
agent-sdlc plan
│
▼
FakeAgent
│
▼
PLAN.md

Verify PLAN.md exists and contains expected deterministic rendering.

Tests must not require a real model.

---

## Step 18 — Existing Plan Safety Test

Create:

PRD.md
PLAN.md

Run:

agent-sdlc plan

Verify:

- command fails
- existing PLAN.md remains byte-for-byte unchanged

---

## Step 19 — Real Agent Smoke Test

Only after deterministic tests pass:

PRD.md
↓
real configured agent
↓
JSON
↓
Plan validation
↓
PLAN.md

Use a small sample PRD.

This is a smoke/integration test, not part of normal unit tests.

Do not make GitHub CI depend on a paid/cloud LLM.

---

## Step 20 — No Automatic Persistence Yet

Do not turn generated stages into domain.Task records.

That is T005.

T004 output boundary:

PRD
↓
Plan

T005:

Plan
↓
Tasks
↓
DAG
↓
SQLite

Keeping these separate will make failures easier to understand.

---

## Step 21 — Verification

Run:

go test ./internal/planner/...
go test ./internal/agent/...
go test ./internal/cli/...
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check

---

## Step 22 — Self Review

Check for:

- provider-specific code leaking into Planner
- LLM writing files
- Markdown parsing
- weak validation
- silent PLAN.md overwrite
- nondeterministic unit tests
- network/model dependency in CI
- task/DAG logic creeping in
- direct workflow-state mutation
- oversized abstraction layers

---

## Step 23 — OCR Review

Run OCR after local tests.

Preferred review model:

gpt-oss:120b-cloud

Review specifically:

- agent/application boundary
- JSON contract
- deterministic validation
- filesystem safety
- context/error propagation
- provider coupling
- test determinism
- scope creep into T005

Fix correctness issues.

Avoid adding abstraction without a concrete T004 need.

---

## Step 24 — CI / Merge

Push branch.

Open PR:

[Task T004] Add PRD to plan generation

CI must pass.

Maximum remediation attempts:

3

Merge only after:

- tests pass
- OCR review passes
- CI passes

---

## Acceptance Criteria

- [ ] Agent interface exists.
- [ ] Planner depends only on Agent abstraction.
- [ ] Fake agent supports deterministic tests.
- [ ] Agent output is structured JSON.
- [ ] JSON maps to typed Plan.
- [ ] Plan validation is deterministic.
- [ ] Stage IDs must be unique.
- [ ] Dependencies must reference valid stages.
- [ ] Self dependencies rejected.
- [ ] Acceptance criteria required.
- [ ] Go renders PLAN.md.
- [ ] `agent-sdlc plan` reads PRD.md.
- [ ] Missing PRD fails safely.
- [ ] Empty PRD fails safely.
- [ ] Existing PLAN.md is protected.
- [ ] Invalid agent output fails safely.
- [ ] Unit tests require no real LLM.
- [ ] CI requires no LLM credentials.
- [ ] Planner does not create Tasks.
- [ ] Planner does not touch workflow state.
- [ ] OCR review completed.
- [ ] CI passes.
