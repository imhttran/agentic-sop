# PRD --- Phase 3: Early OpenJEV Decision Layer

**Type:** Product requirements (draft, for review)

**Status:** Proposed. Phase 3 is **not implemented**. This document records what
we intend to build and why; the required _behavior_ is owned by the
[specifications](../specs/), and the _work_ is owned by
[../plans/PLAN-Phase-3-OpenJEV.md](../plans/PLAN-Phase-3-OpenJEV.md). Nothing in
this document describes current behavior.

## Purpose

Extend the existing OpenJEV (JEV) integration so SOP can obtain **bounded
engineering-analysis evidence earlier in the execution pipeline**, before agent
implementation begins. Phase 3 catches ambiguity, scope problems, missing
context, unsafe execution conditions, and potentially inappropriate task
execution before expensive agent work occurs.

## Related Documents

- [PRD-JEV.md](PRD-JEV.md) --- the JEV workstream's product requirements
  (V1 quality analysis, §15 decision layer).
- [PRD.md](../PRD.md) --- the canonical SOP product requirements.
- [../specs/OPENJEV.md](../specs/OPENJEV.md) --- the normative JEV boundary
  (authoritative for JEV behavior).
- [../specs/QUALITY.md](../specs/QUALITY.md) --- the deterministic quality gate.
- [../specs/REVIEW.md](../specs/REVIEW.md) --- the review pipeline.
- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) --- human authority.
- [../specs/WORKFLOW.md](../specs/WORKFLOW.md),
  [../specs/EXECUTION.md](../specs/EXECUTION.md),
  [../specs/RECOVERY.md](../specs/RECOVERY.md),
  [../specs/SECURITY.md](../specs/SECURITY.md) --- lifecycle, execution,
  recovery, and safety.
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) --- the
  ownership model this phase must preserve.
- [../PLAN-JEV-Implementation.md](../PLAN-JEV-Implementation.md) --- the V1 JEV
  plan (JEV001--JEV016), which is complete and is the baseline Phase 3 extends.

## 1. Problem

Today the effective JEV flow is primarily a **post-validation quality signal**:

```text
IMPLEMENT / FIX
      |
      v
VALIDATE
      |
      v
REVIEW
      |
      v
JEV (quality analysis)
      |
      v
QUALITY
      |
      v
AUTONOMY / POLICY
```

By the time JEV analyzes anything, the expensive agent work has already happened.
Ambiguity in a task, missing context, a scope mismatch between the task and the
repository it will touch, or a destructive/credential-sensitive operation that
should have required a human are all discovered _after_ implementation — when a
triage or pre-execution check could have caught them before the agent started.

## 2. Goals

Phase 3 should support this pipeline:

```text
PLAN
  |
  v
TASK SELECTION
  |
  v
JEV TASK TRIAGE             <- NEW (optional)
  |
  v
SOP POLICY
  |
  v
PRECHECK
  |
  v
JEV PRE-EXECUTION           <- NEW (optional)
  |
  v
SOP POLICY
  |
  v
IMPLEMENT / FIX
  |
  v
VALIDATE
  |
  v
REVIEW
  |
  v
JEV QUALITY ANALYSIS        <- EXISTING (unchanged)
  |
  v
QUALITY
  |
  v
AUTONOMY
  |
  v
APPROVAL / CONTINUE / RETRY / BLOCK
```

Primary goals:

1. Catch task ambiguity, missing context, scope problems, unsafe execution
   conditions, and potentially inappropriate task execution **before** expensive
   agent work.
2. Keep JEV as **evidence only**: it never decides, transitions state, or acts.
3. Keep SOP policy **deterministic** and the sole lifecycle authority.
4. Preserve every existing behavior when the new gates are disabled (they default
   OFF).
5. Make early analysis **auditable and testable** without a network, an OpenJEV
   install, or an LLM.

## 3. Non-Goals

Phase 3 does **not**:

- let JEV modify code, execute shell commands, or mutate repository or SOP state;
- replace SOP's scheduler, deterministic validation, review, quality gate,
  autonomy policy, or human approval;
- let a model control lifecycle transitions;
- introduce a generic parallel decision system or a second workflow engine;
- move JEV ownership into `sop-controller`;
- rewrite SOP in another language;
- enable either new gate globally by default.

## 4. Architecture Rule

Phase 3 must preserve the ownership model exactly:

```text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

More precisely:

```text
JEV produces structured evidence.
Quality interprets quality evidence.
Autonomy/policy determines disposition.
Approval owns explicit human authorization.
Scheduler/lifecycle owns state transitions.
```

OpenJEV MUST NOT transition task state, execute commands, modify repository files,
modify SOP persistence, approve or reject a task, bypass validation/review/human
approval, commit, push, merge, or directly instruct the scheduler to advance a
task. This is the same boundary already specified in
[../specs/OPENJEV.md](../specs/OPENJEV.md) and
[../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md); Phase 3 adds
new _purposes_, not new authority.

## 5. The Three JEV Purposes

Phase 3 must distinguish three concepts that are currently conflated under "JEV".

| Purpose                         | When it runs                                | What it produces                    | Owner of the decision             |
| ------------------------------- | ------------------------------------------- | ----------------------------------- | --------------------------------- |
| **Quality analysis** (existing) | After `VALIDATE` + `REVIEW`                 | Quality findings that feed the gate | [QUALITY.md](../specs/QUALITY.md) |
| **Task triage** (new)           | After task selection, before implementation | Bounded task-evidence findings      | SOP policy                        |
| **Pre-execution** (new)         | After precheck, before implementation       | Bounded execution-context findings  | SOP policy                        |

- **Quality analysis** already exists and must keep working unchanged.
- **Task triage** may report evidence such as task clarity, missing context,
  requirement ambiguity, dependency concerns, scope concerns, estimated risk, and
  confidence.
- **Pre-execution** may report evidence such as scope mismatch, unexpected
  repository area, security-sensitive operation, destructive-operation risk,
  requirement conflict, missing prerequisite, potential approval boundary, and
  confidence.

Neither new purpose returns lifecycle commands. Results such as `RUN`, `BLOCK`,
`APPROVE`, or `REJECT` are explicitly out of scope: JEV returns evidence that SOP
policy evaluates.

## 6. Functional Requirements

### Evidence

- **FR-P3-1 --- Structured early evidence.** Early JEV analysis MUST return a
  structured result carrying at least: the analysis purpose, a bounded confidence,
  a list of machine-readable findings (reusing the existing `Finding`, severity,
  and validation concepts), and a summary. Free-form summary text MUST NOT be
  parsed to infer a lifecycle action.
- **FR-P3-2 --- Purposes.** The result MUST name its purpose
  (`task_triage`, `pre_execution`, or `quality`) so evidence is never conflated
  across checkpoints.
- **FR-P3-3 --- Reuse, don't duplicate.** Phase 3 MUST reuse the existing JEV
  `Analyzer` boundary, `Finding`, severity, and result-validation concepts rather
  than minting a parallel evidence or provider stack. The `Analyzer` boundary
  remains the primary OpenJEV integration seam.

### Checkpoints

- **FR-P3-4 --- Task triage gate.** An optional JEV checkpoint MUST be available
  after SOP deterministically selects a runnable task and before implementation.
  It receives bounded context (task definition, acceptance criteria, dependencies,
  relevant plan context, repository context where appropriate) and returns
  evidence; SOP evaluates that evidence with deterministic policy. Normal, clear
  tasks MUST continue without human intervention.
- **FR-P3-5 --- Pre-execution gate.** A second optional checkpoint MUST be
  available immediately before agent execution, focused on whether the proposed
  execution context is consistent with the selected task. It MUST be analysis
  only: it MUST NOT execute or mutate anything.
- **FR-P3-6 --- Additive and optional.** Both gates MUST be disabled by default and
  MUST be no-ops when disabled, so existing SOP behavior is unchanged.

### Policy

- **FR-P3-7 --- Evidence drives policy.** The system MUST follow
  `JEV evidence → structured classification → deterministic SOP policy →
lifecycle action`. It MUST NOT follow `JEV prose → string matching → lifecycle
action`. Failure and approval classification already moved away from topic-word
  matching toward structured evidence; Phase 3 MUST NOT regress that.
- **FR-P3-8 --- Confidence is advisory.** If confidence is used, it MUST be
  structured and bounded (`0.0 <= confidence <= 1.0`) and MUST NOT directly control
  lifecycle state. Policy MAY combine confidence with finding category, severity,
  risk, task context, and configured thresholds. A bare rule such as
  `confidence < 0.70 → human` MUST NOT be introduced without explicit
  deterministic justification.

### Failure semantics

- **FR-P3-9 --- Analysis result vs provider failure.** The system MUST distinguish
  a JEV _analysis result_ from a JEV _infrastructure/provider failure_ (timeout,
  provider unavailable, malformed response, invalid schema, transport failure). An
  infrastructure failure MUST NOT be interpreted as a finding.
- **FR-P3-10 --- Policy owns fallback.** JEV does not decide its own failure
  policy; SOP does. For advisory early-stage analysis, a reasonable fallback is to
  record the failure and continue under deterministic SOP policy. High-risk policy
  MAY instead fail closed or require human authorization. Malformed results MUST
  fail closed (never treated as a pass), consistent with the existing boundary.

### Configuration and rollout

- **FR-P3-11 --- Compatible, off by default.** Existing installations MUST retain
  current behavior. Early JEV gates MUST default OFF. The existing `quality.jev`
  behavior MUST remain compatible. Unknown configuration keys MUST continue to
  fail clearly, per the current configuration policy. Phase 3 behavior MUST NOT be
  silently enabled.
- **FR-P3-12 --- Progressive rollout.** Rollout order: structured early-analysis
  model + persistence + fake analyzer + tests, then task triage, then
  pre-execution, then the real OpenJEV provider via dogfood, then
  controller/reporting integration. Both gates MUST NOT be enabled globally
  immediately.

### Persistence, observability, and testability

- **FR-P3-13 --- Auditable.** Early JEV results MUST be persisted so a later
  developer can determine: which checkpoint ran, which task it evaluated, what
  evidence JEV returned, which provider produced it, when it occurred, whether the
  provider failed, what SOP policy decided afterward, and why. Existing run
  artifacts/activity records MUST be reused; no second competing source of truth
  may be introduced. SQLite and existing run artifacts remain authoritative
  according to their current ownership boundaries.
- **FR-P3-14 --- Observable.** The checkpoints MUST be exposed through existing
  reporting/activity mechanisms (for example `[TRIAGE]`, `[PRE_EXECUTION]`,
  `[AUTONOMY]` lines). These human-readable messages are observability only and
  MUST NOT be used for policy.
- **FR-P3-15 --- Testable without externals.** The complete early-decision
  lifecycle MUST be testable with a deterministic fake analyzer and MUST NOT need
  network access, an OpenJEV installation, an LLM provider, or an external API.
  Tests MUST be deterministic.

## 7. Ownership and Security Invariants

Phase 3 MUST preserve every existing safety invariant, including:

- JEV cannot transition state, write files, commit, push, merge, approve, or
  modify persistence directly.
- The controller MUST NOT maintain a second source of truth for SOP task state.
- State-changing operations go through the supported SOP boundary.
- Human approval boundaries remain controlled by SOP.
- Advisory JEV failure does not automatically block normal work; high-risk policy
  may fail closed or require human authorization.

`sop-controller` remains a consumer of SOP state. It MAY eventually display triage
analysis, pre-execution analysis, JEV findings, SOP disposition, and approval
requirements, but it MUST NOT invoke JEV independently or create competing
lifecycle state.

## 8. Acceptance Criteria

Phase 3 is complete when:

1. Existing SOP behavior is unchanged when early JEV gates are disabled.
2. Task triage can run before implementation.
3. Pre-execution analysis can run before implementation.
4. Both produce structured evidence.
5. No lifecycle action is inferred from free-form JEV prose.
6. SOP remains the sole lifecycle authority.
7. Autonomy/policy remains deterministic.
8. Human approval remains explicit.
9. JEV cannot mutate repository or SOP state.
10. Provider failure has deterministic, documented fallback behavior.
11. Early-analysis evidence is persisted and auditable.
12. Existing JEV quality analysis continues working.
13. Existing tests continue passing.
14. New Phase 3 tests pass.
15. Documentation accurately distinguishes implemented behavior from proposed
    behavior.
16. `go test ./...`, `go vet ./...`, and `go build ./...` pass.
17. Race tests pass for affected concurrency-sensitive packages.

## 9. Documentation Ambiguity To Resolve

The following statements in the current documentation are ambiguous or
incompletely reconciled and must be made explicit (owned by specification
reconciliation, plan task P3-001):

- Optional JEV failures: the docs must state plainly that **JEV itself never
  blocks workflow**, that **configured SOP policy may block or escalate because of
  JEV evidence**, that **advisory JEV failure does not automatically block normal
  work**, and that **high-risk policy may fail closed or require human
  authorization**.
- [QUALITY.md](../specs/QUALITY.md) must reflect the current autonomy architecture
  rather than assuming every exhausted automation path automatically becomes
  `NEEDS_HUMAN`.
- The relationship between JEV analysis, JEV quality evidence, SOP quality policy,
  autonomy disposition, and human approval must be unambiguous.

## 10. Open Questions To Confirm At Review

These are decisions, not requirements; they are resolved during review before
implementation.

1. **Configuration shape.** Extend the existing `quality.jev` block with explicit
   early-gate flags, or introduce a separate block? The current `decision.*` block
   belongs to a _different_ capability (the partial model-routing decision layer in
   `internal/decision`), so reusing it would conflate two concepts. Recommended:
   keep early gates under `quality.jev` (for example a `gates` sub-block) unless
   review prefers a distinct namespace.
2. **Where triage hooks in graph execution.** The natural seam is the per-task
   selection path in the graph driver, after the scheduler returns a runnable task
   and before the lifecycle runs.
3. **Policy location.** Whether early-gate policy lives beside the existing quality
   JEV policy (`internal/quality`) or in a small early-policy file/package that
   reuses the same severity/risk vocabulary. The existing `internal/autonomy`
   classification vocabulary (`failure.Kind`) should be reused rather than
   extended with free-form categories.
4. **Activity stages.** Whether to add distinct activity stages (`TRIAGE`,
   `PRE_EXECUTION`) or reuse `StageJEV` with distinct actions.

## 11. Where Behavioral Requirements Live

This PRD defines **product intent**. The normative MUST/SHOULD behavior for Phase
3 will be owned by the specifications, updated rather than duplicated:

- [../specs/OPENJEV.md](../specs/OPENJEV.md) --- analysis purposes, evidence model,
  early checkpoints, failure semantics, configuration.
- [../specs/QUALITY.md](../specs/QUALITY.md) --- how early evidence interacts with
  the quality gate (it does not; early evidence is pre-execution).
- [../specs/WORKFLOW.md](../specs/WORKFLOW.md) --- where the checkpoints sit in the
  lifecycle and that they add no new task states.
- [../specs/EXECUTION.md](../specs/EXECUTION.md) --- the `sop run` seam for the
  checkpoints.
- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) --- that a
  policy-directed escalation remains a SOP human boundary.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) --- the exact
  configuration keys and defaults.

When a rule already exists in a specification, this PRD links to it rather than
restating it.
