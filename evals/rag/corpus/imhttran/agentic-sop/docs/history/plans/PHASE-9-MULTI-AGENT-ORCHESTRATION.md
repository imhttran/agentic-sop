# Phase 9 — Multi-Agent Orchestration

> **Document class:** plan · **Lifecycle:** active (recorded by SOP in `.agent-sdlc/plan.meta.json`) · **Authority:** current planning authority — normative for Phase 9.

This document is the Phase 9 plan for `agentic-sop`. The machine-recognizable plan
(`## Project`, `## Summary`, and the `## ORCH-00x — …` stages below) is the
authoritative planning source; the design narrative that follows is the supporting
rationale. SOP remains the sole lifecycle authority.

## Project

Agentic-SOP Phase 9 — Multi-Agent Orchestration (`agentic-sop`)

## Summary

Phase 9 evolves the deterministic single-agent execution harness into a deterministic multi-agent orchestration system while preserving the control-plane architecture established through Phases 7 and 8. Models may propose work, but the harness remains the sole lifecycle authority: it owns state, policy, permissions, work assignment, dependencies, budgets, progress, integration, verification, and termination. Multi-agent execution is provider/model-neutral, is earned by evidence through an adoption gate, and remains opt-in; single-agent execution stays the default and must remain unchanged when orchestration is disabled. Work is ordered as ORCH-001 Orchestration Domain Model, ORCH-002 Worker Assignment Contract, ORCH-003 Deterministic Work Decomposition, ORCH-004 Worker Execution Coordinator, ORCH-005 Scope & Write Ownership, ORCH-006 Conflict Detection, ORCH-007 Integration Stage, ORCH-008 Orchestration Budgets & Progress, ORCH-009 Orchestration Trace & Evaluation, ORCH-010 Context & Routing Integration, ORCH-011 End-to-End Multi-Agent Execution, and ORCH-012 Phase 9 Acceptance Gate.

## ORCH-001 — Orchestration Domain Model

Introduce a provider/model-neutral orchestration domain model representing Task to Orchestrator to Workers to Integrator to Reviewer to Verification, plus the SINGLE / SEQUENTIAL / PARALLEL execution decision. The orchestrator chooses the mode from deterministic task, dependency, and capability information. Do not assume every task requires multiple workers; single-agent execution remains a valid and efficient mode. First inspect the existing abstractions and reuse them; do not create a second competing agent abstraction. Preserve exactly one authoritative SOP lifecycle.

### Dependencies

- none

### Deliverables

- Canonical orchestration domain model (task, orchestrator, worker, integrator, reviewer, verification) adapted to existing repository conventions.
- Deterministic single / sequential / parallel decision derived from task, dependency, and capability data.
- Documentation of the single-lifecycle-authority invariant for orchestration.

### Acceptance Criteria

- The orchestration domain model depends on no specific provider or model and compiles against existing abstractions.
- The execution-mode decision is deterministic and unit-tested for single, sequential, and parallel inputs.
- No second lifecycle authority is introduced; existing domain and task invariants hold.
- `go build ./...`, `go vet ./...`, and `go test ./...` pass.

## ORCH-002 — Worker Assignment Contract

Define a canonical provider-neutral Worker contract. Conceptually an assignment receives Task, Scope, Context, AllowedTools, Budget, OutputContract, and RepositoryIdentity, and produces something equivalent to AssignmentID, Status, Findings, ProposedChanges, ChangedFiles, Evidence, Progress, and Diagnostics. Do not require these exact Go types; reuse the existing provider/agent layer wherever it can be extended cleanly.

### Dependencies

- ORCH-001

### Deliverables

- Canonical WorkAssignment and WorkResult contracts expressed through the existing agent/provider capability interface.
- Scope and repository-identity fields enforced at the contract boundary.

### Acceptance Criteria

- Assignment and result types contain no provider- or model-specific fields.
- A deterministic fake/test adapter plus at least one other adapter satisfy the contract in tests.
- Malformed or out-of-scope results fail safely rather than being reinterpreted as success.

## ORCH-003 — Deterministic Work Decomposition

Decompose work deterministically: bounded, observable, dependency-aware, budget-aware, and scope-aware, with a stable identity and explicit scope per assignment. Models must not create unlimited subagents; the harness owns decomposition bounds.

### Dependencies

- ORCH-001
- ORCH-002

### Deliverables

- A deterministic decomposition step that emits a bounded set of scoped assignments with stable identities.
- Explicit, recorded scope for every assignment.

### Acceptance Criteria

- Decomposition is bounded by an explicit, deterministic limit.
- Every emitted assignment has a stable identity and explicit scope.
- Tests prove decomposition never exceeds the bound and never emits an unscoped assignment.

## ORCH-004 — Worker Execution Coordinator

Execute assignments while the harness retains authority. For the first production version, prefer parallel reasoning/analysis plus controlled integration over multiple unrestricted agents simultaneously editing the repository; correctness and determinism outrank maximum concurrency.

### Dependencies

- ORCH-002
- ORCH-003

### Deliverables

- A coordinator that runs assignments and collects results without conferring lifecycle authority to workers.
- Worker execution routed through the existing agent/provider capability interface.

### Acceptance Criteria

- Workers cannot advance task state, approve, or commit.
- Coordinator behavior is provider-neutral and covered by deterministic tests.
- Failures produce explicit, typed outcomes rather than silent success.

## ORCH-005 — Scope & Write Ownership

Concurrent repository mutation is the highest-risk part of Phase 9. Design explicit write ownership, defaulting toward one writer/integrator rather than unrestricted parallel writes. If workers are allowed direct mutation, scopes must be disjoint and enforced; do not rely on prompt instructions for correctness.

### Dependencies

- ORCH-003
- ORCH-004

### Deliverables

- An explicit write-ownership model with a single-writer/integrator default.
- Deterministic enforcement of disjoint scopes when direct mutation is permitted.

### Acceptance Criteria

- Overlapping write scopes are detected and rejected deterministically.
- Correctness does not depend on prompt instructions such as "do not edit the same file".
- Tests cover the default single-writer path and the disjoint multi-writer path.

## ORCH-006 — Conflict Detection

Detect conflicts deterministically for overlapping files, overlapping symbols, incompatible patches, stale repository identity, dependency changes, and worker results based on outdated HEAD. Conflict must produce an explicit orchestration decision; conflicting results are never silently applied.

### Dependencies

- ORCH-005

### Deliverables

- A deterministic conflict detector covering the enumerated conflict classes.
- An explicit orchestration decision on every detected conflict.

### Acceptance Criteria

- Each conflict class has a deterministic, unit-tested detection path.
- Conflicts are surfaced as decisions, never applied silently.
- A stale repository identity (a result computed against an outdated HEAD) is rejected.

## ORCH-007 — Integration Stage

Define an explicit integration stage, distinct from worker execution: WORK to RESULTS to INTEGRATE to VERIFY. Verification stays centralized and authoritative; the integrator must not automatically trust worker claims — repository state is authoritative.

### Dependencies

- ORCH-004
- ORCH-005
- ORCH-006

### Deliverables

- An explicit integration stage and its recorded outputs.
- The centralized verification path (deterministic validation, review, quality gate) kept unchanged.

### Acceptance Criteria

- Integration is distinguishable from worker execution in state and in trace.
- Worker-reported validation is never treated as authoritative final verification.
- Existing validation ownership is unweakened and its regressions pass.

## ORCH-008 — Orchestration Budgets & Progress

AGENT-004 remains authoritative for budgets. Define orchestration-level limits such as max workers, max active workers, max assignments, and max integration attempts. Do not implement token budgets or provider-specific budgets during this phase; the total execution envelope must remain bounded. Extend AGENT-002 progress concepts to orchestration, distinguishing worker activity, worker progress, task progress, and integration progress; repeated reasoning without new evidence is not progress.

### Dependencies

- ORCH-004

### Deliverables

- Orchestration-level budgets that keep the total execution envelope bounded.
- Progress semantics for worker, task, and integration progress.

### Acceptance Criteria

- Multi-agent execution cannot multiply execution resources beyond a deterministic bound.
- No token budgets and no provider-specific budgets are introduced.
- Existing no-progress and BLOCK semantics remain intact; tests cover bounded worker replacement and replan.

## ORCH-009 — Orchestration Trace & Evaluation

Extend AGENT-001 traceability so a run makes the orchestration graph reconstructable: assignment created, worker selected, model class selected, provider/model resolved, worker started, worker completed, result accepted/rejected, integration started, integration completed, verification result, and termination. Do not log secrets or unnecessary prompt content. Extend AGENT-003 with model-free evaluation fixtures for single-worker success, parallel independent workers, worker failure, worker no-progress, worker conflict, stale worker result, integration failure, verification failure, bounded retry/replan, and successful multi-worker integration.

### Dependencies

- ORCH-004
- ORCH-007
- ORCH-008

### Deliverables

- Orchestration trace events that make the orchestration graph reconstructable.
- Model-free, deterministic evaluation fixtures for the enumerated scenarios.

### Acceptance Criteria

- A recorded run reconstructs the orchestration graph from trace alone.
- Evaluation remains model-free and deterministic; no live external provider is required.
- Trace contains no secrets and no unnecessary prompt content.

## ORCH-010 — Context & Routing Integration

Reuse the Phase 8 Context Engine; each worker receives the smallest sufficient context (global task context plus worker scope plus retrieved evidence), never the complete repository by default. The structural index and BM25 retrieval should construct worker-specific context. Adaptive routing remains capability- and evidence-driven: different assignments may legitimately use different model classes, but provider/model assignments must not be hardcoded. Decision Memory may inform but not control orchestration, and current repository evidence remains authoritative.

### Dependencies

- ORCH-004
- ORCH-008

### Deliverables

- Worker-scoped context assembly built on the Phase 8 Context Engine and retrieval.
- Capability- and evidence-driven routing for assignments.

### Acceptance Criteria

- Worker context is bounded and scoped; workers do not receive full-repository context by default.
- Routing resolves abstract capability requirements without hardcoded provider or model names.
- Stale decision memory never overrides current repository evidence; tests cover stale-memory rejection.

## ORCH-011 — End-to-End Multi-Agent Execution

Wire a bounded, opt-in end-to-end multi-agent execution path (parallel analysis/proposal plus controlled integration) that is disabled by default and preserves single-agent behavior unchanged when disabled.

### Dependencies

- ORCH-005
- ORCH-006
- ORCH-007
- ORCH-008
- ORCH-009
- ORCH-010

### Deliverables

- An opt-in multi-agent execution path gated behind explicit configuration, default off.
- A single-agent compatibility guarantee when orchestration is disabled.

### Acceptance Criteria

- With orchestration disabled, single-agent execution is unchanged: lifecycle, approval, retry, replan, budgets, and provider/model defaults are untouched.
- With orchestration enabled, the multi-agent path is reachable and produces deterministic evidence.
- No default model routing or provider behavior changes merely by introducing Phase 9.

## ORCH-012 — Phase 9 Acceptance Gate

Provide the Phase 9 evidence gate comparing the single-agent baseline against the multi-agent candidate on genuinely parallelizable tasks. A candidate must demonstrate meaningful benefit; if multi-agent execution increases failures, creates excessive conflicts, substantially increases cost/work, or produces no meaningful latency or reliability benefit, default execution remains single-agent. Multi-agent execution must be earned by evidence.

### Dependencies

- ORCH-011

### Deliverables

- A deterministic, model-free multi-agent adoption gate comparing the single-agent baseline against the multi-agent candidate.
- An explicit decision record for whether multi-agent becomes a default for any task class.

### Acceptance Criteria

- The gate measures at least task success, verification success, wall-clock time, worker count, tool calls, discovery work, repeated discovery, repository mutations, conflicts, integration attempts, replans, model escalations, and context bytes/items, without inventing unavailable measurements.
- The gate produces an explicit PASS/FAIL decision, and default execution remains single-agent unless the candidate demonstrates meaningful benefit.
- Provider neutrality, single-lifecycle authority, and existing human-boundary semantics are preserved.

## Design Narrative

The sections below are the design narrative for Phase 9. They are the supporting
rationale for the plan above, not a separate authority.
## Mission

Design and govern Phase 9 of `agentic-sop`.

Phase 9 should evolve the system from a deterministic single-agent
execution harness into a deterministic **multi-agent orchestration
system**, while preserving the control-plane architecture established
through Phases 7 and 8.

Do **not** begin implementation until the Phase 9 architecture, scope,
task graph, invariants, and acceptance gates have been reviewed and
established through SOP.

Starting baseline:

``` text
PRE_PHASE_9_BASELINE = CLEAN

Phase 7                         COMPLETE
Phase 8                         COMPLETE / CLOSED
Provider neutrality             ENFORCED
Phase 8 live-path integration   COMPLETE
POST8-001                       COMPLETE / HISTORICAL
Plan completion                 AVAILABLE
VERIFCACHE-PERSISTENCE          DEFERRED
Phase 9                         NOT STARTED
```

Repository HEAD may have advanced from the previously observed
documentation-closeout revision. Derive the actual baseline from the
repository.

## 1. Establish Baseline

Record:

``` bash
git branch --show-current
git rev-parse HEAD
git status --short
git log --oneline -15
```

Confirm:

``` text
main == origin/main
working tree clean
```

Inspect the current project status, backlog,
architecture/specifications, completed Phase 7/8 plans, and
provider-neutrality constraints.

Do not modify historical Phase 8 evidence.

## 2. Phase 9 Theme

Design Phase 9 around:

``` text
MULTI-AGENT ORCHESTRATION
```

The objective is not simply to run several LLMs at once.

The objective is:

``` text
                 SOP CONTROL PLANE
                        │
              deterministic authority
                        │
        ┌───────────────┼───────────────┐
        │               │               │
     Worker A         Worker B        Worker C
        │               │               │
        └───────────────┼───────────────┘
                        │
                    Integrator
                        │
                     Reviewer
                        │
                   Verification
```

Workers may perform specialized reasoning or repository work. The
harness remains authoritative.

## 3. Core Invariant

> Models may propose actions, implementations, analyses, and strategies.
> The harness owns state, policy, permissions, work assignment,
> dependencies, budgets, progress, integration, verification and
> termination.

Multi-agent execution must not create multiple competing lifecycle
authorities.

There is exactly one authoritative SOP lifecycle.

## 4. Phase 9 Architecture

Design a provider/model-neutral orchestration layer capable of
representing:

``` text
Task
  │
  ▼
Orchestrator
  │
  ├── Worker
  ├── Worker
  └── Worker
        │
        ▼
    Integrator
        │
        ▼
     Reviewer
        │
        ▼
   Verification
```

Do not assume every task requires multiple workers.

Single-agent execution must remain a valid and efficient execution mode.

The orchestrator should decide whether work is:

``` text
SINGLE
SEQUENTIAL
PARALLEL
```

using deterministic task/dependency/capability information where
possible.

## 5. Worker Contract

Define a canonical provider-neutral Worker contract.

Conceptually a worker receives:

``` text
WorkAssignment {
    Task
    Scope
    Context
    AllowedTools
    Budget
    OutputContract
    RepositoryIdentity
}
```

and produces something conceptually equivalent to:

``` text
WorkResult {
    AssignmentID
    Status
    Findings
    ProposedChanges
    ChangedFiles
    Evidence
    Progress
    Diagnostics
}
```

Do not require these exact Go types. First inspect existing abstractions
and reuse them wherever possible.

Do not create a second agent abstraction if the existing provider/agent
layer can be extended cleanly.

## 6. Work Decomposition

The orchestrator must not let models arbitrarily create unlimited
subagents.

Work decomposition must be:

``` text
bounded
observable
dependency-aware
budget-aware
scope-aware
```

Every assignment needs a stable identity and explicit scope.

Example:

``` text
TASK-123

worker-A
    scope: internal/context/**

worker-B
    scope: internal/retrieval/**

worker-C
    scope: tests/evals
```

The harness must know why each worker exists and what it is allowed to
touch.

## 7. Write Ownership

Concurrent repository mutation is the highest-risk part of Phase 9.

Design explicit write ownership.

Default toward:

``` text
one writer/integrator
```

rather than unrestricted parallel writes.

Preferred initial architecture:

``` text
parallel workers
        │
        ├── analysis
        ├── proposed patches
        ├── findings
        └── evidence
                │
                ▼
        deterministic integrator
                │
                ▼
          repository mutation
```

If workers are allowed direct mutation, scopes must be disjoint and
enforced.

Do not rely on prompt instructions such as "please don't edit the same
file" for correctness.

## 8. Conflict Detection

Design deterministic conflict detection for:

``` text
overlapping files
overlapping symbols
incompatible patches
stale repository identity
dependency changes
worker result based on outdated HEAD
```

Conflict must result in an explicit orchestration decision.

Do not silently apply conflicting results.

## 9. Integration

Define an explicit integration stage.

Integration must be distinguishable from worker execution.

``` text
WORK
  ↓
RESULTS
  ↓
INTEGRATE
  ↓
VERIFY
```

The integrator must not automatically trust worker claims. Repository
state is authoritative.

## 10. Verification

Verification remains centralized.

Workers may run local checks for feedback, but worker-reported
validation is not authoritative final verification.

The authoritative path remains:

``` text
integrated repository
        ↓
deterministic validation
        ↓
review
        ↓
quality gate
```

Do not weaken existing validation ownership.

## 11. Progress

Extend AGENT-002 concepts to orchestration.

Distinguish:

``` text
worker activity
worker progress
task progress
integration progress
```

Useful orchestration progress includes assignment completion,
independent findings, patch accepted/rejected, integration completed,
verification passed, and dependency resolved.

Repeated worker reasoning without new evidence is not progress.

## 12. Budgets

AGENT-004 remains authoritative.

Multi-agent execution must not accidentally multiply execution resources
without a deterministic bound.

Define orchestration-level limits such as:

``` text
max workers
max active workers
max assignments
max integration attempts
```

Do not implement token budgets during this phase unless independently
justified.

Do not create provider-specific budgets.

The total execution envelope must remain bounded.

## 13. Replanning

AGENT-005 remains authoritative.

Distinguish:

``` text
worker retry
worker replacement
task replan
orchestration replan
model escalation
```

These are not interchangeable.

A failed worker must not automatically authorize additional workers
indefinitely.

## 14. Context Engine

Reuse Phase 8 Context Engine.

Each worker should receive the smallest sufficient context for its
assignment.

``` text
global task context
        +
worker scope
        +
retrieved evidence
        =
worker context
```

Do not send the complete repository context to every worker by default.

Structural index and BM25 retrieval should help construct
worker-specific context.

## 15. Decision Memory

Decision Memory may inform orchestration but cannot control it.

Current repository evidence remains authoritative.

Memory may provide previous decomposition patterns, known architectural
decisions, previous integration failures, and successful worker
specialization, but stale memory must never override current state.

## 16. Adaptive Routing

Phase 9 should build upon the existing provider-neutral routing
architecture.

Different assignments may legitimately use different model classes.

Example only:

``` text
repository discovery      SMALL
bounded analysis          SMALL/MEDIUM
implementation            MEDIUM
complex integration       MEDIUM/LARGE
architecture review       LARGE
```

Routing must remain capability/evidence driven.

Do not hardcode provider/model assignments such as Qwen for discovery,
DeepSeek for implementation, or Nemotron for review.

Models/providers remain replaceable.

## 17. Provider Neutrality

Maintain:

> No orchestration subsystem may depend directly on a specific provider
> or model unless that dependency is isolated behind the provider/agent
> capability boundary.

The orchestration system should work conceptually with Ollama, MLX/oMLX,
llama.cpp, OpenAI-compatible providers, future Codex/Claude adapters,
and other agents.

Do not implement speculative provider integrations merely to satisfy
this list.

Use deterministic fake/test adapters for architecture tests.

## 18. Trace

Extend AGENT-001 traceability.

A run should make the orchestration graph reconstructable.

Conceptually capture:

``` text
assignment created
worker selected
model class selected
provider/model resolved
worker started
worker completed
result accepted/rejected
integration started
integration completed
verification result
termination
```

Do not log secrets or unnecessary prompt content.

## 19. Evaluation

Extend AGENT-003.

Create deterministic evaluation fixtures for at least:

``` text
single-worker success
parallel independent workers
worker failure
worker no-progress
worker conflict
stale worker result
integration failure
verification failure
bounded retry/replan
successful multi-worker integration
```

Evaluation must remain model-free.

## 20. Human Boundaries

Existing approval semantics remain authoritative.

Multi-agent execution must not increase authority.

Workers cannot approve their own work, approve another worker's work,
extend budgets, create unlimited workers, override BLOCK, override
NEEDS_HUMAN, override verification failure, or commit without existing
policy.

No new human boundary should be invented merely because multiple workers
exist.

## 21. Suggested Phase 9 Task Graph

Evaluate this proposed graph rather than blindly accepting it:

``` text
ORCH-001  Orchestration Domain Model

ORCH-002  Worker Assignment Contract
          depends on ORCH-001

ORCH-003  Deterministic Work Decomposition
          depends on ORCH-001, ORCH-002

ORCH-004  Worker Execution Coordinator
          depends on ORCH-002, ORCH-003

ORCH-005  Scope & Write Ownership
          depends on ORCH-003, ORCH-004

ORCH-006  Conflict Detection
          depends on ORCH-005

ORCH-007  Integration Stage
          depends on ORCH-004, ORCH-005, ORCH-006

ORCH-008  Orchestration Budgets & Progress
          depends on ORCH-004

ORCH-009  Orchestration Trace & Evaluation
          depends on ORCH-004, ORCH-007, ORCH-008

ORCH-010  Context & Routing Integration
          depends on ORCH-004, ORCH-008

ORCH-011  End-to-End Multi-Agent Execution
          depends on ORCH-005–ORCH-010

ORCH-012  Phase 9 Acceptance Gate
          depends on ORCH-011
```

Review the actual repository architecture and simplify or modify this
graph where justified.

Avoid unnecessary framework-building.

## 22. Important Initial Constraint

For the first production version, strongly prefer:

``` text
parallel reasoning / analysis
+
controlled integration
```

over:

``` text
multiple unrestricted agents simultaneously editing the repository
```

Correctness and determinism are more important than maximum concurrency.

## 23. Success Metrics

Define measurable comparisons against the existing single-agent
baseline.

At minimum consider:

``` text
task success
verification success
wall-clock time
worker count
tool calls
discovery work
repeated discovery
repository mutations
conflicts
integration attempts
replans
model escalations
context bytes/items
```

Do not invent unavailable measurements.

Multi-agent execution should not be considered better simply because
more agents ran.

## 24. Multi-Agent Adoption Gate

Phase 9 must include an evidence gate comparing:

``` text
single-agent baseline
        vs
multi-agent candidate
```

A candidate should demonstrate meaningful benefit on tasks that are
actually parallelizable.

If multi-agent execution increases failures, creates excessive
conflicts, substantially increases cost/work, or produces no meaningful
latency/reliability benefit, default execution should remain
single-agent.

Multi-agent execution should be earned by evidence.

## 25. Scope Exclusions

Do not include unless separately justified:

``` text
distributed execution across machines
team/network server
small-device dashboard
persistent cross-run verification cache
JEV adapter work
token budgets
provider-cost optimization
unbounded autonomous subagents
agent-to-agent free-form chat
shared mutable agent memory
controller redesign
sop-controller Phase 9 work
```

These are separate concerns.

## 26. Planning Deliverables

Before implementation, produce:

``` text
docs/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md
```

and any minimal supporting specification needed.

The plan must include:

``` text
problem statement
goals
non-goals
architecture
invariants
task graph
dependencies
acceptance criteria
evaluation strategy
migration/backward compatibility
feature gating
rollback strategy
risks
stop conditions
```

## 27. Backward Compatibility

Existing behavior must remain the baseline.

Prefer:

``` yaml
orchestration:
  enabled: false
```

or equivalent explicit opt-in for initial rollout.

With orchestration disabled, existing single-agent execution must remain
unchanged.

Do not change default model routing or provider behavior merely by
introducing Phase 9.

## 28. Governance

Phase 9 must be governed from the beginning.

After the plan has been authored and reviewed:

``` text
validate plan
activate plan
```

Do not automatically execute ORCH-001 in the same operation if SOP
supports activation separately.

Expected initial lifecycle:

``` text
Phase 9 ACTIVE

ORCH-001 PLANNED / runnable
ORCH-002–ORCH-012 PLANNED / gated

attempts = 0
```

Plan activation must not invoke a model or mutate implementation code.

## 29. Planning Gate

Before activating the plan, perform an architecture review.

Return:

``` text
PHASE_9_PLANNING_GATE = PASS
```

only if:

``` text
[ ] single lifecycle authority preserved
[ ] worker contract provider-neutral
[ ] bounded decomposition
[ ] write ownership explicit
[ ] conflict handling deterministic
[ ] integration stage explicit
[ ] verification centralized
[ ] orchestration budgets bounded
[ ] progress semantics defined
[ ] retry/replan/escalation remain distinct
[ ] Context Engine reused
[ ] provider neutrality preserved
[ ] trace design reconstructs orchestration
[ ] model-free eval strategy exists
[ ] single-agent backward compatibility preserved
[ ] multi-agent adoption requires evidence
[ ] no Phase 9 implementation has started
```

If the gate fails, fix the plan, not production implementation.

## 30. Final Output

Return:

``` text
PHASE 9 PLANNING REVIEW

BASELINE
HEAD:
main == origin/main:
Working tree:

PHASE:
Phase 9 — Multi-Agent Orchestration

ARCHITECTURE:
Orchestrator:
Worker:
Integrator:
Verification:
Lifecycle authority:

TASK GRAPH:
ORCH-001:
ORCH-002:
ORCH-003:
ORCH-004:
ORCH-005:
ORCH-006:
ORCH-007:
ORCH-008:
ORCH-009:
ORCH-010:
ORCH-011:
ORCH-012:

PROVIDER NEUTRALITY:
Status:

BACKWARD COMPATIBILITY:
Default orchestration:
Single-agent behavior:

EVALUATION:
Baseline:
Candidate:
Adoption gate:

RISKS:
CRITICAL:
HIGH:
MEDIUM:
LOW:

PLAN:
Path:
Validated:
Activated:
Model invoked during planning/activation:
Implementation started:

LIFECYCLE:
Phase 9:
ORCH-001:
ORCH-002–ORCH-012:

PHASE_9_PLANNING_GATE = PASS | FAIL

NEXT:
```

Successful stopping point:

``` text
PHASE_9_PLANNING_GATE = PASS

Phase 9 ACTIVE
ORCH-001 PLANNED / runnable
ORCH-002–ORCH-012 PLANNED / gated

No implementation started.
No model invoked merely to activate the plan.
Single-agent execution remains the default.
Multi-agent orchestration remains opt-in.

NEXT: ORCH-001 — Orchestration Domain Model

STOP.
```
