# AI Development Harness — Product Requirements Document

**Status:** Draft v1  
**Working name:** AI Development Harness  
**Primary goal:** Build a portable, provider-neutral harness for agentic software development with deterministic workflow control, specialized AI agents, automated testing, and OpenCodeReview-based independent review.

---

## 1. Problem

AI coding agents are increasingly capable of planning and implementing software, but a single agent should not own the entire development lifecycle without independent controls.

Common problems include:

- an implementation agent reviewing its own work
- inconsistent review coverage
- large prompts and unnecessary token usage
- provider lock-in
- weak separation between deterministic engineering steps and probabilistic AI decisions
- changes being accepted without sufficient tests
- difficulty reproducing agent workflows
- unclear audit trails for why a change was made
- different projects requiring different commands, languages, and validation rules

The harness should coordinate these activities while keeping the repository, Git, tests, policies, and approval gates authoritative.

---

## 2. Product Vision

Create a small, portable AI development harness that can run against different software projects and orchestrate:

```text
Requirement / Task
        |
        v
     Planner
        |
        v
Implementation Agent
        |
        v
Deterministic Validation
  | tests | lint | build |
        |
        v
OpenCodeReview
        |
        v
Independent AI Review
        |
        v
Fix / Re-review Loop
        |
        v
Quality Gate
        |
        v
Human Approval / PR
```

The harness owns workflow and policy.

AI models provide reasoning, generation, and bounded judgments.

Git and deterministic tools provide ground truth wherever possible.

---

## 3. Goals

### 3.1 Primary Goals

1. Support multiple AI coding providers.
2. Keep orchestration independent of any one LLM.
3. Integrate OpenCodeReview as an independent review stage.
4. Use Git diffs as the authoritative change set.
5. Run project-specific build, test, lint, and static-analysis commands.
6. Prevent the implementation agent from silently approving its own work.
7. Support controlled fix/re-review loops.
8. Produce machine-readable and human-readable run reports.
9. Work across multiple programming languages and repository types.
10. Run locally first, with CI integration later.
11. Make every AI capability optional behind a provider/interface boundary.
12. Preserve human approval for consequential operations.

### 3.2 Secondary Goals

- support local models through Ollama or llama.cpp
- support hosted providers
- support MCP tools
- support repository-specific skills/instructions
- support quality profiles
- support task files such as `TASK.md`
- support PR generation
- support CI/CD
- support future decision-model routing such as Jev

---

## 4. Non-Goals for V1

V1 will not:

- autonomously merge to protected branches
- deploy to production
- modify secrets
- bypass repository permissions
- replace GitHub/GitLab branch protection
- require Kubernetes
- require a vector database
- require a web UI
- require Jev
- attempt to be a full IDE
- replace existing coding agents

The harness coordinates tools and agents rather than recreating them.

---

## 5. Users

### Primary User

A software engineer using AI coding tools to implement features while retaining engineering controls.

### Future Users

- development teams
- code reviewers
- platform engineering teams
- CI/CD systems
- internal developer platforms

---

## 6. Product Principles

### 6.1 Deterministic First

Use normal engineering tools whenever the answer can be determined mechanically.

Examples:

```text
Changed files       -> git diff
Compilation         -> compiler
Tests               -> test runner
Formatting          -> formatter
Lint                 -> linter
Dependency checks   -> package tooling
Line locations      -> Git/diff metadata
```

Do not ask an LLM to infer information already available deterministically.

### 6.2 AI for Reasoning

Use AI where semantic reasoning is valuable:

- planning
- implementation
- code understanding
- architectural review
- bug detection
- security reasoning
- explaining findings

### 6.3 Independent Review

Implementation and review are separate roles.

The reviewer receives the actual repository state and diff rather than trusting an implementation summary.

### 6.4 Provider Neutrality

Core workflow code must not depend directly on:

- OpenAI
- Anthropic
- Ollama
- llama.cpp
- OpenRouter
- Hugging Face
- NVIDIA-hosted inference

Providers implement common interfaces.

### 6.5 Human Control

The harness may automate analysis and remediation, but policy determines when human approval is required.

---

## 7. Core Workflow

```text
TASK
 |
 v
Task Loader
 |
 v
Planner
 |
 v
Implementation Agent
 |
 v
Git Change Detector
 |
 +--------------------+
 |                    |
no changes         changes
 |                    |
stop                  v
               Deterministic Checks
                 |    |    |
               build test lint
                 \    |    /
                    v
              OpenCodeReview
                    |
                    v
             Review Findings
                    |
             +------+------+
             |             |
          clean         findings
             |             |
             v             v
        Quality Gate    Fix Agent
             |             |
             |          re-test
             |             |
             |          re-review
             |             |
             +------<------+
                    |
                    v
               Run Report
                    |
                    v
             Human / PR Gate
```

---

## 8. Major Components

### 8.1 Harness CLI

Initial interface:

```bash
aih init
aih plan TASK.md
aih run TASK.md
aih review
aih validate
aih status
aih report
```

Optional later commands:

```bash
aih scan
aih resume
aih doctor
aih providers
aih config
```

### 8.2 Task Loader

Accept:

- Markdown task
- plain-text prompt
- issue description
- future GitHub issue/PR
- future SOP task format

Example:

```yaml
task:
  id: T001
  title: Add health endpoint
  acceptance:
    - GET /health returns 200
    - response contains status
```

### 8.3 Planner

Produces a structured implementation plan.

Output should contain:

- affected areas
- proposed changes
- tests required
- risks
- validation commands
- assumptions

Planning does not modify the repository.

### 8.4 Implementation Agent

Executes approved plan.

Responsibilities:

- inspect repository
- edit code
- add/update tests
- run targeted checks
- report changes

It must not determine final acceptance.

### 8.5 Git Change Detector

Git is authoritative for modifications.

Capture:

```bash
git status --porcelain
git diff
git diff --cached
```

The harness records:

- modified files
- added files
- deleted files
- rename information
- diff size

### 8.6 Validation Runner

Project commands are defined in configuration.

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

For another project:

```yaml
validation:
  build:
    - ./gradlew build
  test:
    - ./gradlew test
```

The harness should not hard-code a language.

### 8.7 OpenCodeReview Adapter

OpenCodeReview is the initial specialized review engine.

Responsibilities:

- invoke OCR against repository changes
- use Delegation Mode when appropriate
- collect structured findings
- preserve severity/location/rule information
- expose findings through a provider-neutral interface

Conceptual interface:

```go
type Reviewer interface {
    Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
}
```

Possible implementations:

```text
OpenCodeReview
NativeLLMReview
StaticOnlyReview
FutureReviewEngine
```

OpenCodeReview must remain replaceable.

### 8.8 Review Agent

Review should focus on:

- correctness
- regressions
- security
- concurrency
- error handling
- API compatibility
- data integrity
- maintainability
- test coverage gaps

Style-only comments should have lower priority unless repository rules require them.

### 8.9 Fix Agent

Receives accepted findings and attempts remediation.

It must receive:

- finding
- relevant diff
- project context
- validation requirements

After fixes:

```text
fix
 -> deterministic validation
 -> fresh Git diff
 -> independent review
```

Never assume a fix is correct because an agent says it is.

### 8.10 Quality Gate

Policy engine determines whether the run can proceed.

Example:

```yaml
quality:
  max_fix_cycles: 3
  require_tests: true
  fail_on:
    - critical
    - high
```

Possible result:

```text
PASS
FAIL
NEEDS_HUMAN
```

The quality gate itself should be deterministic whenever possible.

### 8.11 Run Report

Every run produces a report containing:

- task
- plan
- provider/model
- changed files
- commands executed
- test/build results
- review findings
- fixes
- remaining findings
- token/cost data when available
- timestamps
- final gate result

Example location:

```text
.aih/runs/<run-id>/
    task.md
    plan.md
    diff.patch
    validation.json
    review.json
    report.md
```

---

## 9. Provider Architecture

```go
type Agent interface {
    Execute(ctx context.Context, req AgentRequest) (AgentResponse, error)
}

type DecisionProvider interface {
    Decide(ctx context.Context, req DecisionRequest) (Decision, error)
}

type Reviewer interface {
    Review(ctx context.Context, req ReviewRequest) (ReviewResult, error)
}
```

Potential providers:

```text
Codex
Claude Code
OpenCode
Cursor-compatible workflow
Ollama
llama.cpp
OpenRouter
Hugging Face
NVIDIA
```

Not every provider must support every capability.

---

## 10. Local Model Support

Local inference is an important target.

Example:

```text
Harness
   |
   +-- Planner ------> hosted model
   |
   +-- Coding -------> hosted/local model
   |
   +-- Review -------> OpenCodeReview
                         |
                         +--> local Qwen
```

Another configuration:

```text
Planner        Qwen local
Implementation Codex
Review          Claude
```

Provider selection should be configuration rather than code changes.

---

## 11. Configuration

Repository file:

```text
.aih/config.yaml
```

Example:

```yaml
version: 1

project:
  name: example-service

agent:
  implementation: codex
  review: open-code-review

validation:
  build:
    - go build ./...
  test:
    - go test ./...
  lint:
    - golangci-lint run

review:
  enabled: true
  engine: open-code-review
  delegation: true

quality:
  require_tests: true
  max_fix_cycles: 3
  fail_on:
    - critical
    - high

human:
  approval_before_commit: true
```

---

## 12. Safety and Repository Protection

V1 protections:

- do not push without explicit configuration/approval
- do not merge
- do not force-push
- do not modify `.git` internals
- do not expose secrets to model prompts
- redact known secret patterns
- configurable allowed commands
- command timeout
- workspace boundary enforcement
- maximum fix-loop count
- preserve original task and reports

Potential dangerous shell commands should require explicit policy.

---

## 13. Observability

Record:

- stage duration
- provider/model
- prompt/input size where available
- output tokens where available
- estimated cost where available
- validation duration
- review finding count
- fix cycles
- failure stage

Future metrics:

```text
review precision
finding acceptance rate
fix success rate
time-to-green
cost per task
tokens per task
escaped defect rate
```

---

## 14. OpenCodeReview Integration

OpenCodeReview should be treated as a tool used by the harness.

```text
Harness
  |
  +--> git diff
  |
  +--> validation
  |
  +--> OCR Adapter
         |
         +--> OpenCodeReview
                  |
                  +--> delegated AI harness/model
```

Initial integration goals:

1. detect OCR installation
2. expose installation guidance when missing
3. execute review
4. parse result
5. normalize findings
6. feed actionable findings to fix stage
7. preserve raw OCR output
8. support repository review rules
9. support Delegation Mode
10. allow OCR to be disabled

---

## 15. Phase 2 — Jev Decision Layer

Jev is an **optional decision provider** for bounded workflow judgments. It is not required for V1 and must not become a hard dependency of the harness.

The harness remains responsible for policy, thresholds, permissions, Git state, command execution, validation, and final actions.

Jev may only provide structured judgments such as:

```text
CHOICE
SCORE
YES/NO probability
confidence
reason metadata
```

Jev should not be used as the main coding or review model.

### 15.1 Initial Jev Use Cases

Approved initial use cases:

```text
task complexity classification
model routing
review escalation
finding prioritization
risk classification
whether another review pass is warranted
```

Example:

```text
                         Harness
                            |
                            v
                    DecisionProvider
                            |
                      +-----+-----+
                      |    Jev    |
                      +-----+-----+
                            |
             +--------------+--------------+
             |              |              |
             v              v              v
        Local Model    Stronger Model   Human Gate
```

### 15.2 DecisionProvider Interface

Jev must sit behind a provider-neutral abstraction:

```go
type DecisionProvider interface {
    Name() string
    Decide(ctx context.Context, req DecisionRequest) (Decision, error)
}
```

Example decision model:

```go
type Decision struct {
    Choice     string
    Confidence float64
    Metadata   map[string]any
}
```

Core workflow code must not import Jev-specific types.

### 15.3 Policy Ownership

The harness owns all policies.

For example, Jev may return:

```text
choice: stronger_model
confidence: 0.78
```

The harness decides whether `0.78` is sufficient.

Example:

```yaml
decision:
  provider: jev
  enabled: false

  thresholds:
    route_to_strong_model: 0.75
    require_human: 0.50
```

Jev never directly:

- runs shell commands
- changes files
- commits code
- pushes branches
- merges PRs
- overrides deterministic validation
- bypasses human approval
- changes policy thresholds

### 15.4 Feature Flag

Jev must be disabled by default:

```yaml
features:
  jev_decisions: false
```

When disabled, the harness uses deterministic/default routing.

This guarantees that removing Jev does not break the normal workflow.

### 15.5 Failure Behavior

If Jev:

- times out
- returns malformed output
- produces low confidence
- is unavailable
- violates its output schema

the harness falls back to deterministic policy or requires human review.

The harness must never silently fail open for high-risk decisions.

### 15.6 Auditability

Every Jev decision must record:

```text
decision type
input category
choice
confidence
threshold applied
final harness action
provider/model/version
timestamp
```

Sensitive source code or secrets should not be copied into decision logs.

### 15.7 Evaluation Requirement

Jev must not become a default workflow dependency until it demonstrates measurable value.

Compare:

```text
deterministic routing
simple LLM classification
Jev
```

Measure:

- decision accuracy
- calibration
- latency
- cost
- unnecessary escalation rate
- missed escalation rate
- workflow success rate

Promotion criteria should be documented before evaluation.

### 15.8 Architectural Rule

The guiding rule is:

```text
Jev makes bounded judgments.
The harness owns policy and control flow.
Coding models generate and reason.
Deterministic tools determine objective facts.
Humans retain authority for consequential actions.
```

## 16. Future Community / Network Mode

A future version may allow trusted developers or agents on the same local network to participate.

Potential uses:

- shared model server
- review worker
- build worker
- team dashboard
- task status

Security requirements:

- disabled by default
- local network only initially
- authenticated nodes
- encrypted transport
- explicit pairing
- least privilege
- repository allowlist
- audit log

This is not part of V1.

---

## 17. MCP

MCP can expose harness capabilities to external agents.

Potential tools:

```text
aih_plan
aih_validate
aih_review
aih_status
aih_report
```

The MCP server must call the same internal services as the CLI rather than implement separate logic.

---

## 18. API

A small API may be added after the CLI stabilizes.

Potential endpoints:

```text
POST /tasks
POST /runs
GET  /runs/{id}
POST /runs/{id}/review
POST /runs/{id}/validate
GET  /runs/{id}/report
```

CLI remains the primary V1 interface.

---

## 19. Acceptance Criteria for V1

V1 succeeds when a developer can:

1. initialize the harness in an existing repository
2. configure build/test/lint commands
3. provide a Markdown task
4. generate a plan
5. invoke an implementation agent
6. detect actual changes with Git
7. run deterministic validation
8. invoke OpenCodeReview
9. collect normalized findings
10. optionally send findings to a fix agent
11. re-run validation after fixes
12. limit automatic repair loops
13. produce a final quality result
14. inspect a complete Markdown/JSON run report
15. require human approval before commit/push
16. change AI providers without rewriting orchestration

---

## 20. Success Metrics

Initial engineering metrics:

- >95% of harness runs produce a complete audit report
- 100% of accepted changes pass configured mandatory deterministic gates
- 100% of automatic fix loops respect configured iteration limits
- provider changes require configuration rather than workflow rewrites
- no autonomous merge/push in default configuration
- review findings retain file/location/severity when provided upstream

---

## 21. Future Directions

After V1:

- Jev decision layer
- automatic model routing
- local-model benchmarking
- MCP server
- GitHub/GitLab integration
- PR creation
- CI review mode
- dashboard
- local-network team mode
- historical quality analytics
- review-rule learning with explicit approval
- parallel specialized reviewers
- architecture reviewer
- security reviewer
- database migration reviewer
- API compatibility reviewer

Each capability should remain modular and optional.
