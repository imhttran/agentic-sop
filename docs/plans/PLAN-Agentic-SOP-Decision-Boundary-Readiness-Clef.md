# PLAN --- Agentic-SOP Decision Boundary Readiness for Clef-Flash

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PLANNED\
**Prerequisite:** Pre-Performance Closure, including CLOSE-011, is
complete and verified.

## Project

Agentic-SOP Decision Boundary Readiness for Clef-Flash

## Summary

Verify and, only where necessary, harden agentic-sop so that specialized decision engines such as Clef-Flash can be consumed through a provider/model-neutral decision capability boundary. agentic-sop remains authoritative for policy, lifecycle, approval boundaries, execution authorization, and task state. The plan does not implement a Clef provider; Clef transport, /v1/systemone, response translation, provider registration, and provider-specific tests belong in sop-decision-adapters. The work proceeds through ten stages: baseline capture (AS-CLEF-001), live decision path trace (AS-CLEF-002), provider neutrality audit (AS-CLEF-003), conditional generic-contract hardening (AS-CLEF-004), failure/approval semantics verification (AS-CLEF-005), execution/decision separation verification (AS-CLEF-006), architecture guard (AS-CLEF-007), adapter-side contract definition (AS-CLEF-008), full verification (AS-CLEF-009), and final readiness decision (AS-CLEF-010). Stages run sequentially from the AS-CLEF-001 baseline; AS-CLEF-004 is NOT_REQUIRED unless AS-CLEF-003 finds a real abstraction gap.

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `go version` returned `go version go1.27.1 darwin/arm64` during compiler-hardening investigation.
- Owner: operator-supplied development environment
- Location: Go executable available on PATH

## Goal

Verify and, only where necessary, harden `agentic-sop` so that
specialized decision engines such as Clef-Flash can be consumed through
a provider/model-neutral decision capability boundary.

This plan does **not** implement a Clef provider. Clef transport,
`/v1/systemone`, response translation, provider registration, and
provider-specific tests belong in `sop-decision-adapters`.

The governing rule is:

> Providers evaluate. SOP governs.

`agentic-sop` remains authoritative for policy, lifecycle, approval
boundaries, execution authorization, and task state.

## Non-Goals

- Do not implement a Clef adapter in `agentic-sop`.
- Do not add Ollama/SystemOne HTTP transport for Clef.
- Do not make Clef, Jev, Nimble, Laya, Julia, or another model the
  default.
- Do not change SMALL/MEDIUM/LARGE execution-model defaults.
- Do not alter approval policy merely to accommodate a provider.
- Do not introduce provider-specific branches into SOP policy.
- Do not modify `sop-decision-adapters` under this plan.
- Do not promote a decision model to live authority.

## Architecture Invariant

No `agentic-sop` subsystem may depend directly on Clef-Flash, Ollama,
Jev, Nimble, Laya, Julia, or another specific decision model/provider
unless that dependency is isolated behind a provider-neutral decision
capability interface.

Desired boundary:

```text
agentic-sop
    |
    v
DecisionCapability / DecisionRequest
    |
    | external provider boundary
    v
sop-decision-adapters
    |
    v
DecisionResult
    |
    v
SOP Policy
    |
    +--> Continue
    +--> Block
    +--> Human Approval
    +--> governed routing/action
```

Provider output is evidence. SOP policy determines what that evidence
means.

---

## Planning and discovery constraint

Repository inspection and discovery performed by tasks in this plan are work to be performed by the task, not prerequisite capabilities.

The following kinds of activities MUST NOT be modeled as required capabilities that must already be VERIFIED before a task may execute:

- repository/package inspection
- decision-related package inspection
- provider-name or provider-source searches
- source-code searches
- call-path tracing
- entry-point discovery
- interface/type discovery
- request/result-shape discovery
- configuration/default discovery
- policy-boundary discovery
- adapter-boundary discovery
- test-location discovery
- report-output creation or inspection

These discovery targets may begin with status UNKNOWN. The purpose of the corresponding task is to convert that UNKNOWN into evidence-backed findings through repository inspection.

A task may require a prerequisite capability only when that capability represents an actual externally supplied runtime, permission, tool, service, artifact, or completed dependency that must exist before the task can perform its own work.

Existing completed task artifacts are evidence inputs, not capabilities requiring rediscovery.

In particular:

- AS-CLEF-001 may inspect decision-related packages and search provider names without those searches being prerequisite capabilities.
- AS-CLEF-003 may search provider names and inspect decision packages as part of its audit.
- AS-CLEF-008 may inspect the existing decision boundary, request/result types, and policy surfaces as part of defining the adapter contract.
- Required report paths are outputs of their tasks, not prerequisite capabilities.

Do not request human approval merely because one of these repository discovery targets is initially UNKNOWN. Record UNKNOWN only if the task performs the specified repository investigation and still cannot establish the fact from authoritative in-repository evidence.

---

## AS-CLEF-001 — Capture Repository Truth

Establish the exact agentic-sop baseline before any changes: record branch, HEAD revision, upstream relationship, tracked/untracked state, uncommitted changes without modifying them, Go toolchain, relevant decision-related configuration, current SMALL/MEDIUM/LARGE execution routing, and current test/build baseline; inspect and identify internal/decision, internal/provider, internal/autonomy, internal/router, internal/adaptiveroute, internal/approval, decision-memory or decision-evaluation integration points, CLI/config/bootstrap wiring that constructs decision capabilities, and architecture tests related to provider isolation; search the repository for jev, nimble, laya, julia, clef, systemone, ollama, decision provider names, and model/provider conditionals.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- Repository-baseline report identifying the authoritative decision-related packages and live wiring (evidence recorded in .agent-sdlc/runs/AS-CLEF-001/implementation.md)

### Acceptance Criteria

- Branch, HEAD revision, upstream relationship, tracked/untracked state, uncommitted changes (unmodified), Go toolchain, decision-related configuration, current SMALL/MEDIUM/LARGE execution routing, and current test/build baseline are recorded.
- internal/decision, internal/provider, internal/autonomy, internal/router, internal/adaptiveroute, internal/approval, decision-memory/decision-evaluation integration points, CLI/config/bootstrap decision-capability wiring, and provider-isolation architecture tests are identified.
- Repository searches for jev, nimble, laya, julia, clef, systemone, ollama, decision provider names, and model/provider conditionals are executed and results recorded.
- A repository-baseline report identifying the authoritative decision-related packages and live wiring is produced as evidence.
- No production changes are made; uncommitted user changes are preserved and not modified.

### Task guidance


**Objective:** Establish the exact `agentic-sop` baseline before any
changes.

Record:

- branch
- HEAD revision
- upstream relationship
- tracked/untracked state
- uncommitted changes without modifying them
- Go toolchain
- relevant decision-related configuration
- current SMALL/MEDIUM/LARGE execution routing
- current test/build baseline

Inspect and identify:

- `internal/decision`
- `internal/provider`
- `internal/autonomy`
- `internal/router`
- `internal/adaptiveroute`
- `internal/approval`
- decision-memory or decision-evaluation integration points
- CLI/config/bootstrap wiring that constructs decision capabilities
- architecture tests related to provider isolation

Search the repository for:

- `jev`
- `nimble`
- `laya`
- `julia`
- `clef`
- `systemone`
- `ollama`
- decision provider names
- model/provider conditionals

**Planning constraint:** Apply the plan-wide planning and discovery
constraint. Decision-related package inspection and provider-name or
provider-source searches are work performed by AS-CLEF-001, not
prerequisite capabilities.

**Evidence required:** A repository-baseline report identifying the
authoritative decision-related packages and live wiring.

**Production changes:** None.

---

## AS-CLEF-002 — Trace the Live Decision Path

Determine what actually happens during a governed SOP decision by tracing at least one representative path from SOP operation -> decision request -> decision capability/provider boundary -> decision result -> policy interpretation -> Continue/Block/Approval/routing, documenting request type, result type, capability/interface used, construction/injection point, default implementation if any, fallback behavior, error behavior, confidence/probability handling, human-approval boundary, and whether the path is currently active, optional, shadow-only, or inert, and explicitly answering questions 1-12 with concrete citations.

### Dependencies

- AS-CLEF-001

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-002-decision-path-trace.md — the call-path report and answers to questions 1-12.

### Acceptance Criteria

- At least one representative governed decision path is traced covering SOP operation, decision request, capability/provider boundary, decision result, policy interpretation, and Continue/Block/Approval/routing.
- All documented aspects are covered: request type, result type, capability/interface used, construction/injection point, default implementation, fallback behavior, error behavior, confidence/probability handling, human-approval boundary, and whether the path is live/optional/shadow-only/inert.
- Questions 1-12 are each explicitly answered with concrete packages, files, symbols, configuration, or call sites cited.
- The AS-CLEF-001 baseline is reused and only targeted reads/searches needed to close specific unanswered questions are performed.
- Absence of Jev, Nimble, SystemOne, Clef, Laya, or Julia from the live path, or absence of specialized decision-provider wiring, is recorded as a valid finding; discovery targets that cannot be established are recorded as UNKNOWN with the exact searches/files inspected and the missing evidence.
- BLOCKED is used only if a specific unresolved fact prevents the architecture from being determined, naming the missing evidence.
- No production changes: the report file is the only expected repository change.

### Task guidance


**Objective:** Determine what actually happens during a governed SOP
decision.

Trace at least one representative path from:

```text
SOP operation
    -> decision request
    -> decision capability/provider boundary
    -> decision result
    -> policy interpretation
    -> Continue / Block / Approval / routing
```

Document:

- request type
- result type
- capability/interface used
- construction/injection point
- default implementation, if one exists
- fallback behavior
- error behavior
- confidence/probability handling
- human-approval boundary
- whether the path is currently active, optional, shadow-only, or
  inert

Explicitly answer:

1.  Is Jev present in the live path?
2.  Is Jev a default anywhere?
3.  Is Nimble/SystemOne present in the live path?
4.  Does `agentic-sop` select a specialized decision provider itself?
5.  Is provider selection delegated outside `agentic-sop`?
6.  Are execution-model routing and decision-provider routing cleanly
    separated?
7.  What interface/type represents the decision request?
8.  What interface/type represents the decision result?
9.  Where is the capability constructed or injected?
10. What happens on provider error, timeout, unsupported capability, or
    indeterminate result?
11. Where is the human-approval boundary enforced?
12. Is the discovered path live, optional, shadow-only, or inert?

Cite concrete packages, files, symbols, configuration, or call sites for
every answer. Reuse the AS-CLEF-001 baseline; perform only targeted
reads/searches needed to close a specific unanswered question. Absence
of Jev, Nimble, SystemOne, Clef, Laya, or Julia from the live path, or
absence of specialized decision-provider wiring, is a valid finding.
Mark BLOCKED only if a specific unresolved fact prevents the
architecture from being determined, naming the missing evidence.

**Planning constraint:** The following are discovery targets of this
task, not prerequisite capabilities. Their status may legitimately be
UNKNOWN when the task begins, and none may be required to be VERIFIED
before planning or executing it:

- SOP operation entry points that trigger governed decisions
- decision request construction
- decision capability/provider boundary
- decision result handling
- policy interpretation
- human-approval enforcement point
- specialized provider presence or absence
- provider construction/injection point
- execution-routing versus decision-routing separation

The only prerequisite is the completed AS-CLEF-001 run, whose
authoritative evidence is `.agent-sdlc/runs/AS-CLEF-001/implementation.md`.
No separate AS-CLEF-001 `docs/reports` artifact is required.

The output report path below is work performed by this task, not a
prerequisite capability. If a discovery target cannot be established
after targeted inspection, record it in the report as UNKNOWN with the
exact searches/files inspected and the missing evidence. A target
starting UNKNOWN is not a reason to request human approval; request
human intervention only when an external fact or authorization that the
repository cannot establish is genuinely required.

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-002-decision-path-trace.md`
--- the call-path report and answers to questions 1--12.

**Evidence required:** A call-path report referencing concrete
packages/functions/configuration.

**Production changes:** None. The report file is the only expected
repository change.

---

## AS-CLEF-003 — Audit Provider Neutrality

Verify that the decision boundary is model/provider neutral by checking for direct dependencies on Clef, Jev, Nimble, Laya, Julia, Ollama-specific decision semantics, and SystemOne-specific decision semantics, classifying each as acceptable adapter-boundary dependency, configuration-only dependency, abstraction leak, policy leak, dead/inert code, or test-only dependency, and assessing whether the generic SOP contract expresses genuinely generic concepts (decision question/request, allowed choices, score/binary/choice semantics, probability/confidence, evidence/diagnostics, provider failure, unsupported capability) without encoding a particular provider's wire protocol.

### Dependencies

- AS-CLEF-002

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-003-provider-neutrality-audit.md — the dependency classification and contract assessment.

### Acceptance Criteria

- Direct dependencies on Clef, Jev, Nimble, Laya, Julia, Ollama-specific decision semantics, and SystemOne-specific decision semantics are identified and each classified as acceptable adapter-boundary, configuration-only, abstraction leak, policy leak, dead/inert, or test-only.
- The generic SOP contract is assessed against concepts it should express (decision question/request, allowed choices, score/binary/choice semantics where genuinely generic, probability/confidence, evidence/diagnostics, provider failure, unsupported capability) and whether it encodes a particular provider's wire protocol.
- The audit report docs/reports/clef-readiness/AS-CLEF-003-provider-neutrality-audit.md is produced with the dependency classification and contract assessment.
- Gate: if no material abstraction leak exists, the hardening task AS-CLEF-004 is marked NOT_REQUIRED.

### Task guidance


**Objective:** Verify that the decision boundary is model/provider
neutral.

Check for direct dependencies on:

- Clef
- Jev
- Nimble
- Laya
- Julia
- Ollama-specific decision semantics
- SystemOne-specific decision semantics

Classify each dependency as:

- acceptable adapter-boundary dependency
- configuration-only dependency
- abstraction leak
- policy leak
- dead/inert code
- test-only dependency

The generic SOP contract should express concepts such as:

- decision question/request
- allowed choices
- score/binary/choice semantics where genuinely generic
- probability/confidence
- evidence/diagnostics
- provider failure
- unsupported capability

It should not encode a particular provider's wire protocol.

**Planning constraint:** Apply the plan-wide planning and discovery
constraint. Provider-name or provider-source searches and decision-package
inspection are work performed by AS-CLEF-003 as part of this audit, not
prerequisite capabilities. Reuse completed task artifacts as evidence
inputs.

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-003-provider-neutrality-audit.md`
--- the dependency classification and contract assessment.

**Gate:** If no material abstraction leak exists, mark the hardening
task `NOT_REQUIRED`.

---

## AS-CLEF-004 — Harden the Generic Contract if Required

Make only the smallest provider-neutral correction required by AS-CLEF-003, running only if AS-CLEF-003 identifies a real abstraction gap; permitted changes are extracting a generic capability interface, removing provider-name checks from policy code, generalizing request/result structures, clarifying unsupported-capability behavior, isolating provider construction behind an existing boundary, and strengthening dependency direction; forbidden changes are provider-specific policy behavior such as if provider == "clef" { // SOP policy } or equivalent model-specific policy behavior.

### Dependencies

- AS-CLEF-003

### Deliverables

- Minimal provider-neutral code correction(s) removing any identified abstraction gap (only if AS-CLEF-003 finds a real gap; otherwise the task is marked NOT_REQUIRED with no code change)

### Acceptance Criteria

- The task runs only if AS-CLEF-003 identifies a real abstraction gap; otherwise it is marked NOT_REQUIRED and no production change is made.
- Only permitted smallest provider-neutral corrections are made; provider-specific policy behavior (e.g., if provider == "clef") is not introduced.
- Existing behavior remains unchanged unless the existing behavior itself violates a documented invariant (compatibility requirement).
- Tests prove old behavior remains valid and the provider-specific leak is removed.
- The generic contract does not encode a particular provider's wire protocol and provider construction is isolated behind an existing boundary where changed.

### Task guidance


**Objective:** Make only the smallest provider-neutral correction
required by AS-CLEF-003.

Run this task only if AS-CLEF-003 identifies a real abstraction gap.

Permitted changes include:

- extracting a generic capability interface
- removing provider-name checks from policy code
- generalizing request/result structures
- clarifying unsupported-capability behavior
- isolating provider construction behind an existing boundary
- strengthening dependency direction

Forbidden changes include:

```go
if provider == "clef" {
    // SOP policy
}
```

or equivalent model-specific policy behavior.

**Compatibility requirement:** Existing behavior must remain unchanged
unless the existing behavior itself violates a documented invariant.

**Evidence required:** Tests proving old behavior remains valid and the
provider-specific leak is removed.

---

## AS-CLEF-005 — Verify Failure and Approval Semantics

Prove that an external decision provider cannot bypass SOP governance by verifying behavior for provider unavailable, timeout, malformed result, empty result, unsupported decision type, invalid probability/confidence, low-confidence result, unknown/indeterminate result, and provider internal error, ensuring the decision boundary never converts unknown into approval, silently downgrades risk, bypasses required human approval, authorizes execution merely because a provider is confident, mutates task state outside the governed lifecycle, or turns provider failure into an implicit success.

### Dependencies

- AS-CLEF-004

### Deliverables

- Focused tests for each relevant governance boundary (provider unavailable, timeout, malformed/empty result, unsupported decision type, invalid probability/confidence, low-confidence, unknown/indeterminate, provider internal error)

### Acceptance Criteria

- Behavior is verified for provider unavailable, timeout, malformed result, empty result, unsupported decision type, invalid probability/confidence, low-confidence result, unknown/indeterminate result, and provider internal error.
- Tests demonstrate the decision boundary never converts unknown into approval, never silently downgrades risk, never bypasses required human approval, never authorizes execution merely because a provider is confident, never mutates task state outside the governed lifecycle, and never turns provider failure into an implicit success.
- Focused tests exist for each relevant governance boundary and pass.
- The verification consumes the AS-CLEF-004 result state (including NOT_REQUIRED) without depending on changes that were not required.

### Task guidance


**Objective:** Prove that an external decision provider cannot bypass
SOP governance.

Verify behavior for:

- provider unavailable
- timeout
- malformed result
- empty result
- unsupported decision type
- invalid probability/confidence
- low-confidence result
- unknown/indeterminate result
- provider internal error

The decision boundary must never:

- convert unknown into approval
- silently downgrade risk
- bypass required human approval
- authorize execution merely because a provider is confident
- mutate task state outside the governed lifecycle
- turn provider failure into an implicit success

**Evidence required:** Focused tests for each relevant governance
boundary.

---

## AS-CLEF-006 — Verify Execution/Decision Separation

Confirm that specialized decision providers are independent of execution-model routing by verifying that changing or adding a decision provider does not inherently change SOP_MODEL_DEFAULT_CLASS, the SMALL execution model, the MEDIUM execution model, the LARGE execution model, execution fallback policy, or local/cloud execution policy, and documenting the distinction between execution model (generates/reasons/implements work), decision provider (evaluates a constrained decision and returns evidence), and SOP policy (governs what action is permitted).

### Dependencies

- AS-CLEF-005

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-006-execution-decision-separation.md — the architecture note.

### Acceptance Criteria

- It is verified that changing or adding a decision provider does not inherently change SOP_MODEL_DEFAULT_CLASS, SMALL/MEDIUM/LARGE execution models, execution fallback policy, or local/cloud execution policy.
- The architecture note documents the distinction between execution model, decision provider, and SOP policy as specified.
- Evidence includes an architecture note plus tests/inspection showing no unintended coupling between execution-model routing and decision-provider routing.
- SMALL/MEDIUM/LARGE execution model defaults are not changed.

### Task guidance


**Objective:** Confirm that specialized decision providers are
independent of execution-model routing.

Current execution routing may include SMALL/MEDIUM/LARGE model classes.
Those classes must remain conceptually separate from the specialized
decision-provider capability.

Verify that changing or adding a decision provider does not inherently
change:

- `SOP_MODEL_DEFAULT_CLASS`
- SMALL execution model
- MEDIUM execution model
- LARGE execution model
- execution fallback policy
- local/cloud execution policy

Document the distinction:

```text
Execution model:
    generates/reasons/implements work

Decision provider:
    evaluates a constrained decision and returns evidence

SOP policy:
    governs what action is permitted
```

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-006-execution-decision-separation.md`
--- the architecture note.

**Evidence required:** Architecture note plus tests/inspection showing
no unintended coupling.

---

## AS-CLEF-007 — Add an Architecture Guard

Prevent future provider-specific leakage into agentic-sop, preferably via an automated architecture test that fails if core SOP packages acquire forbidden direct dependencies or provider-specific imports, protecting at minimum the policy/governance side from direct dependencies on known specialized providers, and maintainable as additional providers are introduced.

### Dependencies

- AS-CLEF-006

### Deliverables

- Automated architecture guard test that fails on forbidden direct dependencies or provider-specific imports into core SOP/policy packages

### Acceptance Criteria

- An automated architecture test fails if core SOP packages acquire forbidden direct dependencies or provider-specific imports.
- At minimum the policy/governance side is protected from direct dependencies on known specialized providers.
- The guard passes on the valid tree and demonstrably detects a representative forbidden dependency.
- The guard is structured to remain maintainable as additional providers are introduced.

### Task guidance


**Objective:** Prevent future provider-specific leakage into
`agentic-sop`.

Prefer an automated architecture test that fails if core SOP packages
acquire forbidden direct dependencies or provider-specific imports.

At minimum, protect the policy/governance side from direct dependencies
on known specialized providers.

The guard should be maintainable as additional providers are introduced.

**Evidence required:** Demonstrate that the guard passes on the valid
tree and would detect a representative forbidden dependency.

---

## AS-CLEF-008 — Define the Adapter-Side Contract

Produce the exact contract that sop-decision-adapters must satisfy without implementing the adapter here, documenting request fields, result fields, decision types/capabilities, probability/confidence semantics, diagnostics/evidence semantics, timeout/cancellation expectations, error categories, unsupported-capability behavior, versioning/compatibility expectations, what SOP policy consumes, and what SOP policy deliberately ignores, as the handoff to the separate Clef adapter plan.

Reuse completed AS-CLEF-001 through AS-CLEF-007 evidence. Inspect the existing decision boundary, request/result types, and policy surfaces as task work; create or verify the contract report as an output. These discovery activities and report paths are not prerequisite capabilities. Record UNKNOWN only for facts still unresolved after authoritative repository investigation, with the inspected evidence and remaining gap.

### Dependencies

- AS-CLEF-007

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md — the contract/handoff document.

### Acceptance Criteria

- The contract documents request fields, result fields, decision types/capabilities, probability/confidence semantics, diagnostics/evidence semantics, timeout/cancellation expectations, error categories, unsupported-capability behavior, versioning/compatibility expectations, what SOP policy consumes, and what SOP policy deliberately ignores.
- The document is provider-neutral and does not encode a particular provider's wire protocol.
- The contract/handoff document is committed to the appropriate agentic-sop documentation location as evidence.
- No Clef adapter is implemented in agentic-sop and sop-decision-adapters is not modified.
- Repository inspection and request/result/policy discovery are task work, completed task artifacts are evidence inputs, and the report path is an output; none is a prerequisite capability requiring rediscovery or initial VERIFIED status.

### Task guidance


**Objective:** Produce the exact contract that `sop-decision-adapters`
must satisfy without implementing the adapter here.

Document:

- request fields
- result fields
- decision types/capabilities
- probability/confidence semantics
- diagnostics/evidence semantics
- timeout/cancellation expectations
- error categories
- unsupported-capability behavior
- versioning/compatibility expectations
- what SOP policy consumes
- what SOP policy deliberately ignores

This document becomes the handoff to the separate Clef adapter plan.

**Planning constraint:** Apply the plan-wide planning and discovery
constraint. Reuse existing AS-CLEF-001 through AS-CLEF-007 evidence.
Inspection of the existing decision boundary, request/result types, and
policy surfaces is work performed by AS-CLEF-008 to define the adapter
contract, not prerequisite capabilities. The report path is an output of
this task, not a prerequisite capability.

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md`
--- the contract/handoff document.

**Evidence required:** Provider-neutral contract/handoff document
created at the required `agentic-sop` documentation location.

The document does not need to be Git-committed during this task.
Repository commit is governed separately by SOP's human commit gate.

---

## AS-CLEF-009 — Run Full Verification

Run the repository's required gates, including at minimum where applicable gofmt -l ., go vet ./..., go test -count=1 ./..., go test -race -count=1 ./..., go build ./..., and git diff --check, plus any repository-specific architecture, policy, integration, or end-to-end gates discovered during AS-CLEF-001, and record exact commands and results.

### Dependencies

- AS-CLEF-008

### Requires

- Go build/test toolchain

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-009-verification.md — the verification record.

### Acceptance Criteria

- gofmt -l ., go vet ./..., go test -count=1 ./..., go test -race -count=1 ./..., go build ./..., and git diff --check are run where applicable, with exact commands and results recorded.
- Any repository-specific architecture, policy, integration, or end-to-end gates discovered during AS-CLEF-001 are also run and recorded.
- The verification record docs/reports/clef-readiness/AS-CLEF-009-verification.md is produced.
- Gate: no task is marked complete with a failing required verification gate.

### Task guidance


Run the repository's required gates, including at minimum where
applicable:

```bash
gofmt -l .
go vet ./...
go test -count=1 ./...
go test -race -count=1 ./...
go build ./...
git diff --check
```

Also run any repository-specific architecture, policy, integration, or
end-to-end gates discovered during AS-CLEF-001.

Record exact commands and results.

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-009-verification.md`
--- the verification record.

**Gate:** No task may be marked complete with a failing required
verification gate.

---

## AS-CLEF-010 — Agentic-SOP Readiness Decision

Produce exactly one readiness result (READY, READY_WITH_NOTES, or BLOCKED) for agentic-sop and explicitly answer whether Clef-Flash can be added through the external decision-adapter boundary without introducing Clef-specific behavior into agentic-sop; if the answer is YES the plan ends and implementation moves to sop-decision-adapters, and if NO, do not start the Clef adapter implementation and instead report the smallest general architectural correction required.

### Dependencies

- AS-CLEF-009

### Deliverables

- docs/reports/clef-readiness/AS-CLEF-010-readiness-decision.md — the final readiness report.

### Acceptance Criteria

- Exactly one readiness result is produced: READY, READY_WITH_NOTES, or BLOCKED, using the stated criteria.
- The final report explicitly answers: can Clef-Flash be added through the external decision-adapter boundary without introducing Clef-specific behavior into agentic-sop?
- If the answer is YES, the plan ends and the next action is to begin Clef provider work in sop-decision-adapters; if NO, the Clef adapter implementation is not started and the smallest general architectural correction required is reported.
- The final report includes headline, repository state, AS-CLEF-001 through AS-CLEF-010 task states with one-line evidence, verified live decision architecture and default behavior, provider-neutrality finding (YES/PARTIAL/NO), Jev finding, execution/decision separation finding, governance finding, verification commands and outcomes, changes, readiness decision, and exactly one next action.
- No task is marked complete with a failing required verification gate; unrelated user changes are not overwritten and no push occurs unless the repository workflow explicitly authorizes it.

### Task guidance


Produce exactly one readiness result:

- `READY`
- `READY_WITH_NOTES`
- `BLOCKED`

### READY

Use when `agentic-sop` exposes a clean provider-neutral decision
boundary and no unresolved issue prevents Clef from being implemented
externally.

### READY_WITH_NOTES

Use when the boundary is safe but there are non-blocking observations or
deferred improvements.

### BLOCKED

Use when Clef cannot be integrated externally without provider-specific
behavior, governance changes, or an unresolved architectural defect in
`agentic-sop`.

The final report must explicitly answer:

> Can Clef-Flash be added through the external decision-adapter boundary
> without introducing Clef-specific behavior into `agentic-sop`?

If the answer is `YES`, this plan ends and implementation moves to
`sop-decision-adapters`.

If the answer is `NO`, do not start the Clef adapter implementation.
Report the smallest general architectural correction required.

**Deliverable:** `docs/reports/clef-readiness/AS-CLEF-010-readiness-decision.md`
--- the final readiness report.

---

## Execution Policy

Include the plan-wide planning and discovery constraint in every task's
planning input, including regenerated subplans. Model repository
discovery in task objectives and acceptance criteria, never in
`stage.requires`. Preserve completed evidence inputs; do not rediscover
them as prerequisite capabilities.

Execute tasks sequentially:

```text
AS-CLEF-001
    |
AS-CLEF-002
    |
AS-CLEF-003
    |
AS-CLEF-004 (only if required)
    |
AS-CLEF-005
    |
AS-CLEF-006
    |
AS-CLEF-007
    |
AS-CLEF-008
    |
AS-CLEF-009
    |
AS-CLEF-010
```

Use truthful task states:

- `PLANNED`
- `ACTIVE`
- `LOCAL_DONE`
- `BLOCKED`
- `NOT_REQUIRED`

Do not:

- skip a blocked gate
- claim completion without evidence
- overwrite unrelated user changes
- amend unrelated commits
- change execution-model defaults
- implement the Clef adapter
- modify `sop-decision-adapters`
- make a provider default
- push unless the repository workflow explicitly authorizes it

---

## Expected End State

A successful run should leave `agentic-sop` looking conceptually like:

```text
agentic-sop
    |
    +-- lifecycle
    +-- policy
    +-- approval
    +-- routing/governance
    |
    +-- provider-neutral DecisionCapability
             |
             +---- external boundary ----> sop-decision-adapters
```

There should be **no Clef-specific production behavior in
`agentic-sop`**.

The next governed phase can then operate in `sop-decision-adapters` to
implement and evaluate Clef-Flash.

---

## Final Report Format

### Headline

One-sentence outcome.

### Repository State

- branch
- HEAD
- upstream
- working tree

### Task State

Report `AS-CLEF-001` through `AS-CLEF-010` with state and one-line
evidence.

### Current Decision Architecture

State the verified live decision path and current/default implementation
behavior.

### Provider-Neutrality Finding

Exactly one:

- `YES`
- `PARTIAL`
- `NO`

### Jev Finding

State whether Jev is present, live, default, optional, test-only, or
absent.

### Execution/Decision Separation

State whether SMALL/MEDIUM/LARGE execution routing is independent from
specialized decision-provider routing.

### Governance Finding

Confirm whether provider results can or cannot bypass SOP
approval/policy boundaries.

### Verification

Record all required commands and outcomes.

### Changes

List changed files and why. If no production changes were required, say
so explicitly.

### Readiness Decision

Exactly one:

- `READY`
- `READY_WITH_NOTES`
- `BLOCKED`

### Next Action

Exactly one next action.

If `READY` or `READY_WITH_NOTES`, the next action should be to begin the
Clef provider work in `sop-decision-adapters`, not to continue adding
provider-specific behavior to `agentic-sop`.
