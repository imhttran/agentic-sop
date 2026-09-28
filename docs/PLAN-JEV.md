# AI Development Harness — Implementation Plan

> **Two editions of one roadmap.** This file is the **fuller** edition. Its sibling
> [`PLAN-wrapup.md`](PLAN-wrapup.md) is a **condensed** edition: both share T001–T032
> (identical numbers and the same work), then diverge — this file expands the
> decision layer (T033–T046) before the operational tasks, while the wrap-up file
> condenses it and continues straight through (for example "Command Policy" is T052
> here but T043 there). A `T0NN` reference elsewhere is only meaningful against the
> file it names: `docs/tasks/TASK-030..040` cite this file, `TASK-024..029` cite the
> wrap-up file.
>
> Neither is actively maintained. The roadmap of record is [`PLAN.md`](PLAN.md)
> (Stage 24), and the executed task sequence is `docs/tasks/TASK-0NN.md`.

**Status:** Draft v1
**Companion:** `PRD.md`

---

## 1. Implementation Strategy

Build the harness as a small modular CLI first.

Recommended initial implementation language: **Go**.

Why Go fits this project:

- single portable binary
- good process execution support
- strong concurrency primitives
- simple interfaces
- easy CLI distribution
- good fit for Git/CI tooling
- low runtime footprint

Start with one repository and one happy-path workflow before adding provider breadth.

---

## 2. Proposed Repository Layout

```text
ai-development-harness/
├── README.md
├── PRD.md
├── PLAN.md
├── go.mod
├── cmd/
│   └── aih/
│       └── main.go
├── internal/
│   ├── config/
│   ├── task/
│   ├── workflow/
│   ├── git/
│   ├── runner/
│   ├── validation/
│   ├── agent/
│   │   ├── provider.go
│   │   └── ...
│   ├── review/
│   │   ├── reviewer.go
│   │   └── ocr/
│   ├── quality/
│   ├── report/
│   ├── security/
│   └── decision/
├── pkg/
├── testdata/
├── examples/
└── .github/
    └── workflows/
```

Keep `internal/decision` as an interface/placeholder until Phase 2.

---

# Phase 0 — Foundation

## T001 — Bootstrap Repository

Create:

- Go module
- CLI entrypoint
- README
- PRD
- PLAN
- `.gitignore`
- basic Makefile or task runner
- unit-test foundation

Acceptance:

```bash
go test ./...
go build ./...
./aih --help
```

---

## T002 — Configuration Model

Implement:

```text
.aih/config.yaml
```

Support:

- project metadata
- validation commands
- agent provider
- review engine
- quality thresholds
- human gates

Requirements:

- schema validation
- defaults
- useful errors
- no secrets in committed configuration

Tests:

- valid config
- malformed YAML
- missing required fields
- defaults
- unknown version

---

## T003 — Project Initialization

Implement:

```bash
aih init
```

Behavior:

- detect repository root
- create `.aih/`
- generate config template
- create `.aih/runs/`
- do not overwrite existing config without approval

Acceptance:

```text
existing Git repo
   |
aih init
   |
.aih/config.yaml
.aih/runs/
```

---

# Phase 1 — Deterministic Core

## T004 — Command Runner

Build a controlled process executor.

Capture:

- command
- working directory
- stdout
- stderr
- exit code
- duration
- timeout

Security:

- workspace boundary
- configurable command allowlist
- timeout
- no shell interpolation unless explicitly required

---

## T005 — Git Adapter

Implement:

```go
type Git interface {
    Root(ctx context.Context) (string, error)
    Status(ctx context.Context) (Status, error)
    Diff(ctx context.Context) (Diff, error)
}
```

Capture:

- modified
- added
- deleted
- renamed
- untracked
- staged/unstaged

Git output is authoritative.

---

## T006 — Task Loader

Support:

```bash
aih plan TASK.md
aih run TASK.md
```

Task format should support:

- ID
- title
- description
- requirements
- acceptance criteria
- constraints

Keep plain Markdown usable.

---

## T007 — Run State

Create run directory:

```text
.aih/runs/<run-id>/
```

Persist:

```text
task.md
state.json
events.jsonl
```

Stages:

```text
CREATED
PLANNING
IMPLEMENTING
VALIDATING
REVIEWING
FIXING
WAITING_FOR_HUMAN
PASSED
FAILED
```

Runs must be inspectable after process termination.

---

# Phase 2 — Agent Provider Layer

## T008 — Agent Interface

Define:

```go
type Agent interface {
    Name() string
    Execute(context.Context, AgentRequest) (AgentResponse, error)
}
```

Request:

- role
- task
- repository root
- instructions
- relevant context

Response:

- summary
- structured output
- provider metadata

Avoid provider-specific types outside adapters.

---

## T009 — First Agent Adapter

Implement one provider end-to-end.

Recommended order:

```text
1. existing coding harness/provider
2. local Ollama
3. additional hosted providers
```

Do not implement five providers before the workflow works.

---

## T010 — Planner Stage

Planner must produce:

```text
summary
files/areas likely affected
implementation steps
tests
risks
assumptions
```

Persist:

```text
.aih/runs/<id>/plan.md
```

Planning must not mutate source files.

---

## T011 — Implementation Stage

Implementation agent:

- receives task + plan
- edits repository
- runs only permitted commands
- returns summary

Immediately capture Git diff afterward.

Do not use agent summary as evidence of changed files.

---

# Phase 3 — Validation Pipeline

## T012 — Validation Model

Define:

```go
type Validator interface {
    Validate(context.Context, ValidationRequest) ValidationResult
}
```

Initial validators:

- build
- test
- lint

Each result records:

```text
name
command
status
exit code
duration
stdout/stderr reference
```

---

## T013 — Configurable Validation Commands

Example:

```yaml
validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - golangci-lint run
```

Run in deterministic order initially.

Later allow parallel independent checks.

---

## T014 — Validation Gate

Policy:

```text
mandatory validation failure
        |
        v
      FAIL
```

Do not send obviously uncompilable changes through expensive AI review by default.

Optional configuration may allow review-on-failure for diagnostic use.

---

# Phase 4 — OpenCodeReview

## T015 — Reviewer Abstraction

Define:

```go
type Reviewer interface {
    Name() string
    Review(context.Context, ReviewRequest) (ReviewResult, error)
}
```

Normalized finding:

```go
type Finding struct {
    ID          string
    Severity    Severity
    File        string
    StartLine   int
    EndLine     int
    Title       string
    Description string
    Rule        string
}
```

---

## T016 — OpenCodeReview Detection

At startup/review:

- detect OCR availability
- determine compatible invocation
- capture version if available

If missing:

- fail gracefully
- show install/config guidance
- do not silently replace it with another reviewer

---

## T017 — OpenCodeReview Adapter

Implement OCR invocation.

Requirements:

- run against actual repository/diff
- support Delegation Mode where configured
- capture raw output
- normalize findings
- preserve source metadata

Persist:

```text
review-raw.*
review.json
```

---

## T018 — Delegated Review

Allow the configured AI harness to perform the semantic review while OCR manages review preparation/workflow.

Configuration:

```yaml
review:
  engine: open-code-review
  delegation: true
```

Verify:

- no duplicate LLM invocation
- findings can be normalized
- errors are observable

---

## T019 — Review Quality Rules

Initial severity:

```text
CRITICAL
HIGH
MEDIUM
LOW
INFO
```

Focus on:

- correctness
- security
- data loss
- concurrency
- resource leaks
- error handling
- compatibility
- missing tests

Avoid blocking on cosmetic findings by default.

---

# Phase 5 — Controlled Fix Loop

## T020 — Finding Triage

Classify each finding:

```text
ACTIONABLE
IGNORE
NEEDS_HUMAN
```

V1 may use deterministic rules plus reviewer metadata.

Do not automatically dismiss high-impact findings solely through the implementation agent.

---

## T021 — Fix Agent

Send actionable findings to an agent.

Provide:

- task
- plan
- finding
- relevant code
- current diff
- repository rules

After modification:

```text
capture diff
run validation
run review again
```

---

## T022 — Loop Guard

Configuration:

```yaml
quality:
  max_fix_cycles: 3
```

When exceeded:

```text
NEEDS_HUMAN
```

Never create an unbounded agent loop.

---

## T023 — Regression Protection

Every fix cycle must rerun required deterministic checks.

Minimum:

```text
fix
 |
 v
build
 |
 v
test
 |
 v
review
```

A review finding disappearing is not proof the code works.

---

# Phase 6 — Quality Gate and Reporting

## T024 — Quality Policy

Example:

```yaml
quality:
  require_tests: true
  max_fix_cycles: 3
  fail_on:
    - critical
    - high
```

Gate inputs:

- build status
- test status
- lint status
- unresolved findings
- fix-loop limit
- human-required flags

Output:

```text
PASS
FAIL
NEEDS_HUMAN
```

---

## T025 — Run Report

Generate:

```text
.aih/runs/<id>/report.md
.aih/runs/<id>/report.json
```

Include:

- task
- plan
- provider
- files changed
- diff stats
- validation results
- review findings
- fixes
- unresolved findings
- durations
- cost/token metadata where available
- final gate

---

## T026 — Status Command

Implement:

```bash
aih status
```

Show:

```text
Run: T006-...
Stage: REVIEWING
Build: PASS
Tests: PASS
Review: 2 findings
Fix cycles: 1/3
```

---

# Phase 7 — Human Approval and Git Workflow

## T027 — Human Gate

Default:

```yaml
human:
  approval_before_commit: true
```

Harness stops after PASS and presents:

- summary
- changed files
- validation
- findings
- report location

It does not commit automatically by default.

---

## T028 — Optional Commit

Later support:

```bash
aih commit
```

Only after:

- quality PASS
- human approval when configured

Generate a proposed commit message from task context, but Git operation remains explicit.

---

## T029 — GitHub PR Integration

After local workflow is stable:

```text
aih pr
```

Potential behavior:

- push approved branch
- create PR
- attach run summary
- link review report

Keep this out of the first usable release.

---

# Phase 8 — Local Models

## T030 — Ollama Provider

Implement provider adapter for local inference.

Capabilities:

- configurable base URL
- model selection
- timeout
- structured responses where supported

Example:

```yaml
providers:
  ollama:
    model: qwen3:8b
```

---

## T031 — llama.cpp Provider

Support OpenAI-compatible llama-server endpoint where practical.

Example:

```yaml
providers:
  llamacpp:
    endpoint: http://127.0.0.1:8080
    model: local
```

---

## T032 — Provider Capability Detection

Represent capabilities:

```text
PLAN
CODE
REVIEW
DECISION
TOOLS
STRUCTURED_OUTPUT
```

Routing should reject unsupported role/provider combinations clearly.

---

# Phase 9 — Jev Decision Layer

Jev is an optional post-MVP capability.

Do not begin this phase until:

- the deterministic harness is stable
- OpenCodeReview integration works
- run reports are reliable
- model/provider abstraction is working
- baseline workflow metrics exist

Jev must remain behind a feature flag and provider interface.

---

## T033 — Decision Domain Model

Define bounded decision types.

Example:

```go
type DecisionType string

const (
    DecisionTaskComplexity DecisionType = "task_complexity"
    DecisionModelRoute     DecisionType = "model_route"
    DecisionReviewEscalate DecisionType = "review_escalate"
    DecisionRiskLevel      DecisionType = "risk_level"
)

type DecisionRequest struct {
    Type    DecisionType
    Signals map[string]any
}

type Decision struct {
    Choice     string
    Confidence float64
    Metadata   map[string]any
}
```

Keep requests structured and minimal.

Do not send full repository contents unless strictly required.

---

## T034 — DecisionProvider Interface

Define:

```go
type DecisionProvider interface {
    Name() string
    Decide(context.Context, DecisionRequest) (Decision, error)
}
```

Implement a deterministic provider first:

```text
StaticDecisionProvider
```

This provides a control/baseline before Jev is introduced.

Acceptance:

- workflow runs with no Jev dependency
- decision calls can be unit tested deterministically
- provider can be swapped by configuration

---

## T035 — Jev Feature Flag

Add:

```yaml
features:
  jev_decisions: false
```

And:

```yaml
decision:
  provider: static
```

Enabling Jev requires:

```yaml
features:
  jev_decisions: true

decision:
  provider: jev
```

Default remains disabled.

---

## T036 — Jev Adapter

Implement Jev behind `DecisionProvider`.

Requirements:

- typed/validated responses
- timeout
- cancellation
- confidence normalization
- provider metadata
- clear unavailable/error states
- no direct workflow mutation

Jev returns a decision only.

The harness determines the action.

---

## T037 — Decision Policy Engine

Create deterministic policy around decision outputs.

Example:

```text
Jev:
    route = strong_model
    confidence = 0.82

Harness policy:
    threshold = 0.75

Result:
    use strong_model
```

Example configuration:

```yaml
decision:
  thresholds:
    model_route: 0.75
    review_escalation: 0.70
    human_required: 0.50
```

Do not embed threshold policy inside the Jev adapter.

---

## T038 — Task Complexity Decision

First Jev experiment:

```text
simple
medium
complex
```

Possible routing:

```text
simple  -> smaller/local model
medium  -> normal configured model
complex -> stronger configured model
```

The decision must not skip mandatory tests or review.

---

## T039 — Review Escalation Decision

Use Jev only after OpenCodeReview produces structured findings.

Possible output:

```text
normal
stronger_review
human_review
```

Inputs may include:

- severity counts
- files touched
- sensitive paths
- validation results
- diff size
- configured risk labels

Avoid sending unnecessary source text.

---

## T040 — Risk Classification

Experiment with bounded risk categories:

```text
LOW
MEDIUM
HIGH
```

Signals may include:

```text
database migration
authentication/authorization
payment logic
security-sensitive code
concurrency
public API changes
large diff
failed or flaky tests
```

Jev classification supplements deterministic rules.

It does not override them.

---

## T041 — Fail-Safe Behavior

If Jev:

```text
times out
returns invalid schema
returns unsupported choice
has confidence below threshold
is unavailable
```

then:

```text
low-risk routing -> deterministic fallback
high-risk routing -> NEEDS_HUMAN
```

Never silently downgrade a high-risk decision because Jev failed.

---

## T042 — Decision Audit Log

Add decision records to the run.

Example:

```json
{
  "type": "model_route",
  "provider": "jev",
  "choice": "strong_model",
  "confidence": 0.82,
  "threshold": 0.75,
  "action": "route_to_strong_model"
}
```

Persist with the run report.

---

## T043 — Jev Evaluation Dataset

Create a benchmark from completed real harness runs.

Label each case manually for:

```text
task complexity
risk level
review escalation
appropriate model route
```

Do not evaluate only synthetic examples.

---

## T044 — Jev Evaluation Harness

Compare:

```text
StaticDecisionProvider
Simple LLM classifier
Jev
```

Measure:

```text
accuracy
precision/recall where appropriate
confidence calibration
latency
cost
unnecessary escalation
missed escalation
workflow impact
```

---

## T045 — Jev Promotion Gate

Jev remains experimental until evaluation criteria are met.

Example gate:

```text
no meaningful increase in missed high-risk escalations
acceptable calibration
latency within configured budget
clear routing/review value over static policy
```

If the value is not measurable, keep deterministic policy as default.

---

## T046 — Optional Default Routing

Only after T045 may configuration permit:

```yaml
decision:
  provider: jev
```

as a project-selected default.

The global/default harness configuration should still support running without Jev.

---

# Phase 10 — MCP

## T047 — MCP Server

Expose:

```text
aih_plan
aih_validate
aih_review
aih_status
aih_report
```

MCP calls the same application services as CLI commands.

No duplicate workflow implementation.

---

## T048 — MCP Security

Require:

- workspace restrictions
- explicit tool capabilities
- command policy
- no arbitrary filesystem access
- no secret exposure

---

# Phase 11 — CI

## T049 — GitHub Actions Mode

Support:

```text
PR
 |
 v
build/test/lint
 |
 v
aih review
 |
 v
quality report
```

Initially informational.

Do not make AI review a mandatory merge gate until its false-positive/false-negative behavior is understood.

---

## T050 — PR Review Output

Publish concise results:

```text
Build: PASS
Tests: PASS
OCR: 2 findings
  HIGH: 0
  MEDIUM: 1
  LOW: 1
Quality: PASS
```

Link to detailed artifacts where available.

---

# Phase 12 — Hardening

## T051 — Secret Redaction

Before model calls:

- identify common secret formats
- redact configured paths/values
- prevent `.env` inclusion by default
- provide explicit allowlist for sensitive files

Test aggressively.

---

## T052 — Command Policy

Classify commands:

```text
SAFE
REQUIRES_APPROVAL
DENIED
```

Examples:

```text
go test ./...        SAFE
git diff             SAFE
git commit           REQUIRES_APPROVAL
git push             REQUIRES_APPROVAL
git push --force     DENIED
```

Project policy may tighten these defaults.

---

## T053 — Failure Recovery

Support interrupted runs.

Persist enough state to:

```bash
aih resume <run-id>
```

Never blindly replay an already completed mutation.

---

# Phase 13 — Evaluation

## T054 — Harness Benchmark Suite

Create representative tasks:

- simple bug
- API feature
- concurrency bug
- database migration
- missing validation
- security issue
- refactor

Measure:

```text
task success
test success
review findings
accepted findings
fix cycles
elapsed time
tokens
cost
```

---

## T055 — Review Evaluation

Compare:

```text
No AI review
Generic agent self-review
Independent generic reviewer
OpenCodeReview
OpenCodeReview delegation
```

Use the same task corpus.

This should determine whether OCR remains the default reviewer.

---

# Phase 14 — Optional Team / Community Mode

Do not begin until local single-user workflow is stable.

## T056 — Local Network Service

Potential API:

```text
Harness node
    |
    +-- model worker
    +-- review worker
    +-- build worker
```

Requirements:

- disabled by default
- explicit pairing
- authentication
- TLS
- repository allowlist

---

## T057 — Small-Device Dashboard

Responsive dashboard for:

- run status
- task queue
- findings
- validation
- approval requests

Design for phone/tablet-sized screens.

No code editing required initially.

---

# Recommended Build Order

Do not implement all phases simultaneously.

Build in this order:

```text
T001-T007   Foundation + deterministic core
      |
T008-T011   One working AI provider
      |
T012-T014   Validation
      |
T015-T019   OpenCodeReview
      |
T020-T023   Fix loop
      |
T024-T027   Quality + human gate
      |
      v
 FIRST USABLE RELEASE
      |
T030-T032   Local models
      |
T033-T037   Jev experiment
      |
T047-T050   MCP + CI
      |
T051-T055   Hardening + evaluation
      |
T056-T057   Team/community mode
```

---

# First Usable Release Definition

The first release is complete when this works:

```bash
aih init
aih run TASK.md
```

and produces:

```text
Task loaded
Plan generated
Implementation completed
Git diff captured
Build passed
Tests passed
Lint passed
OpenCodeReview completed
Findings processed
Fixes validated
Quality gate passed
Human approval requested
Report generated
```

with no automatic push, merge, or deployment.

---

# Immediate Next Tasks

Start with:

- [x] T001 Bootstrap Repository
- [x] T002 Configuration Model
- [x] T003 Project Initialization
- [x] T004 Command Runner
- [x] T005 Git Adapter
- [x] T006 Task Loader
- [x] T007 Run State

After T007, stop and validate the deterministic foundation before adding an AI provider.

That checkpoint is intentional: the harness should remain useful engineering software rather than becoming a collection of model calls.
