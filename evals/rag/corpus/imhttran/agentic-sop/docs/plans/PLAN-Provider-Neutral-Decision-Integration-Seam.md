# PLAN --- Provider-Neutral Decision Integration Seam

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PLANNED\
**Prerequisite:** Agentic-SOP Decision Boundary Readiness for Clef (AS-CLEF-001 through AS-CLEF-010) is closed; the adapter contract is `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md`; the design analysis is `docs/reports/decision-integration/SEAM-architecture-analysis.md`.

## Project

Provider-Neutral Decision Integration Seam

## Summary

Design and implement the smallest provider-neutral construction/consumption seam that lets an external decision adapter supply decision **evidence** to the live SOP policy path without becoming a policy authority. The seam keeps the Go decision contract internal, reaches external adapters over a provider-neutral JSON process boundary (the existing `internal/agent/command.go` pattern), validates every returned result fail-closed under SOP ownership, and interprets it through the existing pure `internal/autonomy` policy. The external adapter is optional and OFF by default: with no provider configured, existing SOP behavior is unchanged. This plan implements no Clef/provider-specific behavior, keeps `internal/model` execution routing independent, and makes no provider live or default. Eight sequential stages: boundary trace (SEAM-001), contract definition (SEAM-002), seam implementation (SEAM-003), optional evidence consumption (SEAM-004), failure/governance hardening (SEAM-005), fake-external cross-module proof (SEAM-006), full verification and architecture review (SEAM-007), and the integration-seam readiness decision (SEAM-008).

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `go version` returns `go version go1.27.1 darwin/arm64`; the repository is a single Go module (`go.mod`, module `github.com/imhttran/agentic-sop`).
- Owner: operator-supplied development environment
- Location: Go executable available on `PATH`

## Goal

Make it possible for an external `sop-decision-adapters` decision adapter to supply
bounded decision evidence to the live SOP policy path, through a provider-neutral
seam, without the adapter importing SOP's internal types and without the adapter
ever gaining policy, lifecycle, approval, commit, or merge authority.

The governing invariant:

> Providers evaluate. SOP governs.

## Non-Goals

- Do not implement Clef or any provider-specific adapter in `agentic-sop`.
- Do not add Clef, oMLX, Ollama, SystemOne, Nimble, Jev, Laya, Julia, or any other
  provider-specific dependency, branch, or identity to SOP policy.
- Do not export or re-home `internal/decision` to work around Go `internal/`
  visibility.
- Do not change SMALL/MEDIUM/LARGE execution-model defaults or couple decision
  evidence to execution-model routing.
- Do not make any decision provider live, selectable, or default.
- Do not modify `sop-decision-adapters`.
- Do not weaken human approval, validation, review, or the commit gate.

## Architecture Invariant

No `agentic-sop` subsystem may depend directly on a specific decision provider, and
no external provider may influence any SOP state transition. The desired boundary:

```text
external decision adapter
        |
        | provider-neutral JSON (request/result DTO)
        v
internal/decision/command  (SOP-owned process adapter + fail-closed validation)
        |
        | validated typed evidence
        v
internal/autonomy  (pure, deterministic policy)
        |
        +--> Continue
        +--> Block
        +--> Human Approval
```

Provider output is evidence. SOP policy determines what it means. With no external
provider configured, existing behavior is unchanged.

## Planning and discovery constraint

Repository inspection and discovery performed by tasks in this plan are work to be
performed by the task, not prerequisite capabilities. The following MUST NOT be
modeled as required capabilities that must already be VERIFIED before a task may
execute: repository/package inspection; decision/policy/autonomy package
inspection; call-path tracing; interface/type discovery; request/result-shape
discovery; configuration/default discovery; error/approval discovery; test-location
discovery; report-output creation or inspection.

These discovery targets may begin with status UNKNOWN; the task converts that
UNKNOWN into evidence-backed findings through repository inspection. A task may
require a prerequisite capability only when it represents an actual externally
supplied runtime, permission, tool, service, artifact, or completed dependency.
Required report/plan paths are task outputs, not prerequisite capabilities.
Completed task artifacts are evidence inputs, not capabilities requiring
rediscovery.

---

## SEAM-001 — Capture and Trace the Current Integration Boundary

Establish the exact current integration boundary at HEAD before any change: record branch, HEAD, upstream relationship, working-tree state, Go toolchain, and the current test/build baseline; trace the live `internal/autonomy` policy path (`Decide`, `DecideEarly`, `DecidePlanChange` and its `internal/cli` call sites); trace the `internal/decision` abstraction (`Request`, `Decision`, `Choice`, `Provider`, `NewProvider`, `Thresholds`, `Route`) and confirm its production consumer status; trace where autonomy obtains evidence and where evidence becomes policy interpretation; identify the construction/bootstrap/config points (`defaultDeps`, `deps.newJEVAnalyzer`, `config.DecisionConfig`, `config.EarlyJEV`); identify existing DI/registration and process-boundary mechanisms (`deps` factories, `internal/agent/command.go`, `internal/jev.Analyzer`, `runpkg.RunJEV`); record error/failure behavior, the human-approval enforcement symbol, and the `internal/archtest` provider-isolation guards. Produce the current-state evidence record and an explicit current-state diagram.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- docs/reports/decision-integration/SEAM-001-current-boundary.md — the current-state trace, boundary inventory, and diagram (evidence record)

### Acceptance Criteria

- Branch, HEAD, upstream relationship, working-tree state, Go toolchain, and the current test/build baseline are recorded.
- The live autonomy policy path and its production call sites are identified with file/symbol evidence.
- `internal/decision` symbols and their production-consumer status are recorded with the exact searches performed.
- The evidence sources autonomy consumes and the point where evidence becomes policy are identified.
- Construction/bootstrap/config points and existing DI/registration/process-boundary mechanisms are identified.
- Error/failure behavior, the human-approval enforcement path, and the architecture guards are identified.
- An explicit current-state diagram is produced.
- No production change is made; uncommitted user changes are preserved and not modified.

### Production-change scope

None (read-only trace and report).

### Evidence requirements

Current-state record and diagram citing file/symbol locations at HEAD.

### Stop conditions

Stop and report if the live autonomy path or the human-approval boundary cannot be
established from authoritative in-repository evidence; record UNKNOWN with the
remaining gap rather than asserting.

---

## SEAM-002 — Define the Provider-Neutral Decision Contract

Define the minimum stable provider-neutral contract that an external decision adapter must satisfy, and the exact validation and interpretation rules SOP applies before policy consumes a result. Specify the request DTO (version, decision kind, bounded question, numeric signals, allowed choices), the result DTO (version, status OK/UNSUPPORTED/ERROR, choice, optional confidence in [0,1], advisory diagnostics, error text), and explicitly enumerate what MUST NOT cross the boundary (task mutation, lifecycle transitions, approval mutation, commit/merge/scheduler authority, policy-outcome authority, execution-model/class selection). Define the fail-closed validation rules (known status, known choice, finite in-range confidence, missing confidence = indeterminate) and the deterministic SOP-owned interpretation mapping from a validated result to a typed `failure.Classification` consumed by `autonomy.Decide`. The contract must be provider-neutral and must not encode any provider's wire protocol.

### Dependencies

- SEAM-001

### Deliverables

- docs/reports/decision-integration/SEAM-002-decision-contract.md — the cross-module contract, validation rules, and interpretation mapping

### Acceptance Criteria

- The request and result DTO fields, types, and required/optional status are specified.
- The known choice vocabulary and the confidence range/semantics are specified.
- The forbidden content (lifecycle/approval/commit/merge/scheduler/policy/execution-model authority) is explicitly enumerated.
- The fail-closed validation rules are specified for each malformed/indeterminate case.
- The deterministic SOP-owned interpretation mapping to a typed `failure.Classification` is specified, and it does not map any decision to an execution-model class.
- The contract is provider-neutral and contains no provider wire protocol.
- `internal/decision` is documented as remaining internal; no public Go package is introduced.

### Production-change scope

None (contract document only).

### Evidence requirements

The contract document, including the DTO shape and the validation/interpretation tables.

### Stop conditions

Stop and report if a provider-neutral contract cannot be defined without exposing
lifecycle/approval/commit authority or an execution-model class; do not broaden the
contract silently.

---

## SEAM-003 — Implement the Provider-Neutral Construction and Consumption Seam

Implement the internal seam, OFF by default: promote the fail-closed decision validator (`knownChoice`/`validConfidence`) to an exported, tested SOP-owned validator; add an in-process deterministic provider (proof, not default); add a provider-neutral process adapter (mirroring `internal/agent/command.go`) that runs a configured external command, sends the neutral JSON request on stdin, reads the neutral JSON result on stdout, honors the caller's context deadline, and returns a validated `decision.Decision` or an explicit error; add a `deps.newDecisionProvider func(cfg) (decision.Provider, error)` factory wired at the composition root; and add the configuration that selects the provider and enables the capability, defaulting to disabled. No external process is invoked when the capability is disabled.

### Dependencies

- SEAM-002

### Deliverables

- Exported fail-closed decision validator with unit tests
- In-process deterministic provider used only for proof
- Provider-neutral process adapter (`internal/decision/command`) implementing `decision.Provider`
- `deps.newDecisionProvider` factory and configuration/enablement wiring (disabled by default)

### Acceptance Criteria

- A fail-closed validator exists, is exported, and is unit-tested for unknown choice, out-of-range/NaN/Inf confidence, and missing confidence.
- The process adapter implements `decision.Provider`, sends the documented request DTO, parses only the documented result DTO, and returns an error (never a success) on malformed, unsupported, timed-out, or cancelled input.
- The process adapter honors the caller's `context` deadline and terminates the external process on cancellation.
- The factory is wired at the composition root and constructing a provider is a strict no-op when the capability is disabled or no provider is configured.
- The capability is OFF by default; no external process is invoked unless explicitly enabled.
- No Clef/provider-specific identity or branch is introduced.
- The change does not modify `internal/model` or any execution-model default.

### Production-change scope

New internal decision-seam code, one exported validator, one `deps` factory field,
and configuration wiring. No execution-model, lifecycle, approval, or policy-semantic
change.

### Evidence requirements

Unit tests for the validator and the process adapter (malformed/unsupported/timeout/
cancelled), and a test proving the disabled path is a strict no-op.

### Stop conditions

Stop and report if the process adapter cannot be implemented without importing a
provider-specific dependency or without exposing lifecycle authority in the DTO.

---

## SEAM-004 — Integrate Optional Evidence Consumption with Autonomy

Wire the optional seam into the live policy path so that, when enabled, a validated
decision result is deterministically interpreted (via the SEAM-002 mapping) into a
typed `failure.Classification` that the existing pure `autonomy.Decide` consumes.
The interpretation reuses `autonomy.Decide` by default; if a dedicated entry point is
required it must take an autonomy-owned evidence type and add no lifecycle authority.
The provider may only add attention (raise risk or require a human); it may never
reduce risk, auto-approve, authorize execution, or choose a lifecycle transition
beyond the existing policy vocabulary. When the capability is disabled or no provider
is configured, the path is a strict no-op and existing behavior is unchanged.

### Dependencies

- SEAM-003

### Deliverables

- Optional, disabled-by-default evidence-consumption call site in the live path
- Deterministic interpretation from a validated result to a typed `failure.Classification`
- Tests proving the disabled path is unchanged and the enabled path only adds attention

### Acceptance Criteria

- A validated decision result is converted to a typed `failure.Classification` and interpreted by `autonomy.Decide` (or an autonomy-owned equivalent that adds no authority).
- The interpretation is deterministic and fail-closed: unknown/invalid/indeterminate/failed results cannot produce an automated approval.
- The provider can only add attention (human boundary or higher risk); it cannot reduce risk or authorize execution.
- No decision is mapped to an execution-model class; `internal/model` routing is untouched.
- With the capability disabled or no provider configured, behavior is unchanged (proved by a test).
- `internal/autonomy` remains pure and deterministic; any new entry point takes only typed evidence.

### Production-change scope

An optional policy-seam call site and its deterministic interpretation. No change to
`internal/model`, to execution-model defaults, or to the human-approval boundary.

### Evidence requirements

Tests for disabled-equals-unchanged, enabled-adds-attention-only, and fail-closed
interpretation; a test asserting no model class is selected from decision evidence.

### Stop conditions

Stop and report if consuming decision evidence requires changing `internal/autonomy`
semantics, the human-approval boundary, or execution routing.

---

## SEAM-005 — Failure, Fallback, and Governance Hardening

Harden the seam with focused tests for every failure/fallback case in the SEAM-002/analysis contract: no provider configured; provider unavailable; timeout; cancellation; unsupported capability; malformed response; unknown choice; confidence out of range/NaN/Inf; missing confidence; indeterminate result; and provider process failure. Prove the provider cannot transition lifecycle state, create an approval, authorize a commit or merge, bypass validation/review/human approval, or mutate governance state. Prove the disabled path preserves existing behavior and that a provider failure is never an implicit success.

### Dependencies

- SEAM-004

### Deliverables

- Focused fail-closed/governance tests for each failure and fallback case
- Tests proving lifecycle/approval/commit/merge isolation

### Acceptance Criteria

- Focused tests exist and pass for each case: no provider configured; provider unavailable; timeout; cancellation; unsupported capability; malformed response; unknown choice; confidence out of range/NaN/Inf; missing confidence; indeterminate result; process failure.
- Tests prove a provider result cannot directly transition lifecycle state, create an approval, or authorize a commit/merge.
- Tests prove malformed/unknown/indeterminate evidence fails closed per the defined policy.
- Tests prove provider absence and provider failure preserve governed behavior and never become success.
- The disabled path is proven identical to existing behavior.

### Production-change scope

Tests and any bounded hardening required by the tests; no policy-semantic widening.

### Evidence requirements

The focused test suite and its passing results.

### Stop conditions

Stop and report any case that cannot be made fail-closed without broadening scope.

---

## SEAM-006 — Fake External Provider and Cross-Module Proof

Prove the cross-module boundary with a fake external decision provider that lives outside `internal/` and satisfies only the documented JSON protocol. The fixture must demonstrate that an external module/process can supply valid decision evidence through the process boundary without importing any SOP Go type and without gaining authority. Include an out-of-module fixture (for example a small test command that reads the request DTO on stdin and writes the result DTO on stdout) exercised end-to-end through the seam.

### Dependencies

- SEAM-005

### Deliverables

- An out-of-module fake external provider fixture (JSON protocol only)
- An end-to-end test proving valid evidence crosses the module/process boundary

### Acceptance Criteria

- A fake external provider outside `internal/` supplies valid evidence through the process boundary and the seam consumes it.
- The fixture imports no SOP Go type; it implements only the documented JSON protocol.
- The end-to-end test proves a valid result is validated and interpreted, and that an invalid/failed result fails closed.
- The fixture gains no lifecycle, approval, commit, or merge authority.
- Multiple providers can implement the same protocol without any change to SOP policy (demonstrated by at least two fixture behaviors, for example a passing and a failing provider).

### Production-change scope

Test-only fixtures and tests (outside `internal/` where the proof requires it).

### Evidence requirements

The fixture and the passing end-to-end test.

### Stop conditions

Stop and report if the external fixture cannot supply evidence without importing
SOP internal types or without a provider-specific change to SOP.

---

## SEAM-007 — Full Verification and Architecture Review

Run the complete repository verification gates and the architecture/provider-neutrality guards, and record exact commands and results: `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go build ./...`, `git diff --check`, plus the `internal/archtest` guard suite. Verify and record the architecture properties: no Clef-specific policy branch; no provider-name policy branch; provider results cannot transition lifecycle state, create approvals, or authorize commits/merges; malformed evidence fails closed; provider absence preserves current behavior; provider failure preserves governed behavior; execution-model routing remains independent; a fake external provider crosses the module/process boundary; multiple providers can implement the contract without modifying SOP policy.

### Dependencies

- SEAM-006

### Deliverables

- docs/reports/decision-integration/SEAM-007-verification.md — the verification and architecture-review record

### Acceptance Criteria

- `gofmt -l .`, `go vet ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, `go build ./...`, and `git diff --check` are run with exact commands and results recorded.
- The `internal/archtest` guard suite is run and recorded.
- The architecture properties are each verified and recorded with tests/inspection evidence.
- The verification record is produced.
- No task is marked complete with a failing required verification gate.

### Production-change scope

None beyond any guard/test additions required to assert the properties (no policy widening).

### Evidence requirements

The verification record with exact commands and outcomes; the guard results.

### Stop conditions

Stop and report any failing required gate; do not mark the task complete.

---

## SEAM-008 — Integration-Seam Readiness Decision

Produce exactly one readiness result (READY, READY_WITH_NOTES, or NOT_READY) for the provider-neutral decision integration seam, answering explicitly whether an external decision adapter (for example a Clef adapter) can supply decision evidence to the live SOP policy path without introducing provider-specific behavior or policy authority into `agentic-sop`. If READY or READY_WITH_NOTES, the next action is provider work in `sop-decision-adapters`; if NOT_READY, do not start provider work and instead report the smallest provider-neutral correction required.

### Dependencies

- SEAM-007

### Deliverables

- docs/reports/decision-integration/SEAM-008-readiness-decision.md — the final readiness report

### Acceptance Criteria

- Exactly one readiness result is produced: READY, READY_WITH_NOTES, or NOT_READY.
- The report explicitly answers whether an external adapter can supply evidence to the live policy path without provider-specific behavior or policy authority in `agentic-sop`.
- The report includes headline, repository state, SEAM-001..SEAM-008 task states with one-line evidence, the verified seam architecture, the contract/protocol, failure semantics, the governance boundary, verification commands and outcomes, changes, readiness decision, and exactly one next action.
- If NOT_READY, the smallest provider-neutral correction is reported and provider work is not started.
- The report confirms `internal/decision` remains internal and that SMALL/MEDIUM/LARGE defaults and execution-model routing are unchanged.

### Production-change scope

None (readiness report only).

### Evidence requirements

The readiness report with cited evidence.

### Stop conditions

Stop and report if the seam cannot be shown to preserve the invariants; classify
NOT_READY with the smallest bounded correction.

---

## Execution Policy

Include the plan-wide planning and discovery constraint in every task's planning
input. Model repository discovery in task objectives and acceptance criteria, never
in `stage.requires`. Preserve completed evidence inputs; do not rediscover them as
prerequisite capabilities.

Execute tasks sequentially:

```text
SEAM-001
    |
SEAM-002
    |
SEAM-003
    |
SEAM-004
    |
SEAM-005
    |
SEAM-006
    |
SEAM-007
    |
SEAM-008
```

Use truthful task states: `PLANNED`, `ACTIVE`, `LOCAL_DONE`, `BLOCKED`,
`NOT_REQUIRED`.

Do not: skip a blocked gate; claim completion without evidence; overwrite unrelated
user changes; amend unrelated commits; implement Clef or any provider-specific
adapter; export or re-home `internal/decision`; change execution-model defaults;
couple decision evidence to execution routing; make a provider live or default; push
unless the repository workflow explicitly authorizes it.

## Expected End State

A successful run leaves `agentic-sop` with an optional, provider-neutral decision
integration seam that:

- keeps `internal/decision` internal;
- reaches external adapters over a provider-neutral JSON process boundary;
- validates and interprets provider results fail-closed under SOP ownership;
- never lets a provider transition state, approve, authorize, or bypass a gate;
- leaves SMALL/MEDIUM/LARGE execution-model routing independent and unchanged;
- is OFF by default, so existing behavior is unchanged when no provider is
  configured.

The next governed phase can then implement a provider (for example Clef) in
`sop-decision-adapters` against the SEAM-002 contract.
