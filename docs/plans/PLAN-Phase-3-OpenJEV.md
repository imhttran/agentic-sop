# PLAN --- Phase 3: Early OpenJEV Decision Layer

**Type:** Implementation plan (proposed, for review)

**Status:** Proposed. No task is implemented. This plan describes the work needed
to satisfy [../requirements/PRD-Phase-3-OpenJEV.md](../requirements/PRD-Phase-3-OpenJEV.md).
It MUST NOT override the specifications; where it and a specification disagree, the
specification wins.

## Summary

Add two **optional, disabled-by-default** JEV checkpoints that run *before* agent
implementation --- task triage (after task selection) and pre-execution (after
precheck, before execution) --- so SOP can obtain bounded early engineering
evidence without moving lifecycle authority into a model.

Phase 3 extends the existing JEV integration. It does not introduce a second
workflow engine, a parallel decision system, or a new provider stack.

## Objective

Let SOP obtain early, structured, auditable evidence about a selected task and its
proposed execution context, and evaluate that evidence with deterministic policy,
while preserving every existing behavior when the gates are disabled.

JEV must not directly change SOP task state, approve or complete tasks, bypass
validation/review/human approval, commit, open or merge PRs, retry indefinitely, or
modify the working tree.

## Architecture

```text
             reasoning / evidence
                    |
                    v
                 OpenJEV
                    |
           structured evidence
                    |
                    v
              SOP policy
                    |
        deterministic decision
                    |
                    v
            SOP lifecycle
                    |
                    v
               execution
```

Preserve:

```text
JEV analyzes.
SOP decides.
IMPLEMENT/FIX changes code.
```

## Rollout Order

```text
Phase 3A  structured early-analysis model + persistence + fake analyzer + tests
Phase 3B  task-triage integration
Phase 3C  pre-execution integration
Phase 3D  real OpenJEV provider + dogfood
Phase 3E  controller / reporting integration
```

Do not enable both new gates globally immediately.

## Tasks

Each task lists: objective, scope, dependencies, likely files/packages,
acceptance criteria, and validation.

### P3-001 --- Reconcile Phase 3 normative documentation

- **Objective:** Make the specifications authoritative for Phase 3 and remove the
  current ambiguity, without changing behavior.
- **Scope:** Update [../specs/OPENJEV.md](../specs/OPENJEV.md) to define the three
  analysis purposes, the structured evidence model, the early checkpoints (marked
  as required once enabled, disabled by default), and the analysis-result vs
  provider-failure distinction. Reconcile [../specs/QUALITY.md](../specs/QUALITY.md)
  with the autonomy architecture. Update [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md)
  for the new keys. Cross-reference rather than duplicate.
- **Depends on:** P3-002 (the evidence model must be settled first).
- **Likely files:** `docs/specs/OPENJEV.md`, `docs/specs/QUALITY.md`,
  `docs/specs/WORKFLOW.md`, `docs/specs/EXECUTION.md`,
  `docs/reference/CONFIGURATION.md`.
- **Acceptance criteria:** Docs state that JEV itself never blocks; configured SOP
  policy may block or escalate from JEV evidence; advisory JEV failure does not
  automatically block normal work; high-risk policy may fail closed or require
  human authorization. Implemented vs proposed behavior is clearly separated.
- **Validation:** `python3 ~/.agents/skills/doc-reconcile/scripts/link_check.py .`
  (0 broken links); manual review.

### P3-002 --- Define early JEV analysis purposes and structured evidence

- **Objective:** Represent an early analysis result in a structured, machine-readable
  form suitable for deterministic policy.
- **Scope:** Add an analysis purpose type (`task_triage`, `pre_execution`,
  `quality`) and a structured evidence result (purpose, bounded confidence,
  findings, summary). Reuse the existing `jev.Finding`, severity, and
  `Result.Validate` concepts; do not duplicate types. Findings must carry enough
  structure (category, severity, evidence/reason, path, line, confidence) for
  policy to evaluate.
- **Depends on:** none.
- **Likely files/packages:** `internal/jev` (`jev.go` or a new
  `evidence.go`), `internal/jev/jev_test.go`.
- **Acceptance criteria:** Evidence is structured and validated (fail closed on
  unknown purpose/severity/status); no lifecycle action is derived from summary
  text; existing `Analyzer`/`Result` remain backward compatible.
- **Validation:** `go test ./internal/jev/...`; `go vet ./internal/jev/...`.

### P3-003 --- Extend deterministic FakeAnalyzer

- **Objective:** Make the full early-decision lifecycle testable without network,
  OpenJEV, or an LLM.
- **Scope:** Extend the existing deterministic fake (`internal/jev/fake.go`) to
  return early-purpose results (clear, ambiguous, scope concern, destructive
  concern, provider failure, malformed). Keep it deterministic and side-effect free.
- **Depends on:** P3-002.
- **Likely files/packages:** `internal/jev/fake.go`, `internal/jev/jev_test.go`.
- **Acceptance criteria:** Fake covers every required test case deterministically;
  no test uses the network or a live provider.
- **Validation:** `go test ./internal/jev/...`.

### P3-004 --- Persist early JEV analysis artifacts

- **Objective:** Make early analysis auditable without creating a second source of
  truth.
- **Scope:** Persist early results using the existing run-artifact mechanism
  (alongside `jev.json`/`jev-history.jsonl`), recording checkpoint, task,
  evidence, provider, timestamp, provider-failure flag, policy decision, and
  reason. Artifacts remain diagnostic evidence; nothing reads them back to drive a
  decision. No `state.db` change.
- **Depends on:** P3-002.
- **Likely files/packages:** `internal/run/jev_artifact.go`, `internal/run/run.go`.
- **Acceptance criteria:** Artifacts are written best-effort; existing persistence
  authority is unchanged; no state migration or deletion.
- **Validation:** `go test ./internal/run/...`.

### P3-005 --- Add configuration and feature flags

- **Objective:** Add explicit configuration for the early checkpoints, off by
  default, compatible with existing installations.
- **Scope:** Add early-gate flags under the JEV configuration (namespace confirmed
  at review --- see PRD §10), validated at load time with focused errors for unknown
  keys/values, matching the current configuration policy.
- **Depends on:** none.
- **Likely files/packages:** `internal/config/config.go`, `internal/config/*_test.go`,
  `docs/reference/CONFIGURATION.md`.
- **Acceptance criteria:** Existing configs load unchanged; gates default OFF; an
  unknown mode/key fails clearly; `sop run` behavior is identical when disabled.
- **Validation:** `go test ./internal/config/...`; `go vet ./internal/config/...`.

### P3-006 --- Implement task-triage analysis seam

- **Objective:** Invoke JEV for task triage with bounded context.
- **Scope:** Build the triage request (task, acceptance criteria, dependencies,
  plan context, bounded repository context) and call the `Analyzer` at the triage
  purpose. Read-only; no side effects. No-op when disabled or absent.
- **Depends on:** P3-002, P3-003, P3-005.
- **Likely files/packages:** `internal/cli/jev.go`, `internal/cli/run.go`,
  `internal/run/jev.go`.
- **Acceptance criteria:** Triage runs only when enabled; it receives no mutation
  handle; absence/disable is a no-op; provider failure is recorded, never a finding.
- **Validation:** `go test ./internal/cli/... ./internal/run/...`.

### P3-007 --- Define deterministic triage policy mapping

- **Objective:** Map triage evidence to a deterministic disposition.
- **Scope:** Classify structured triage findings into the existing evidence/risk
  vocabulary and evaluate them with deterministic policy (reusing the autonomy
  classification approach). No prose matching; no bare confidence threshold.
- **Depends on:** P3-002, P3-006.
- **Likely files/packages:** `internal/quality` (new early policy) or a small
  early-policy file; `internal/autonomy` (reuse `failure.Kind`, `ApprovalRisk`).
- **Acceptance criteria:** Clear tasks continue; ambiguity/scope/security evidence
  is evaluated by deterministic rules; no lifecycle action comes from summary text.
- **Validation:** `go test ./internal/quality/... ./internal/autonomy/...`.

### P3-008 --- Integrate task triage into lifecycle

- **Objective:** Run the triage gate after task selection and before
  implementation, and act on its deterministic disposition.
- **Scope:** Hook the triage seam into the per-task selection path in the graph
  driver, before the lifecycle runs. Record observability (P3-012) and persist the
  result (P3-004). Disabled = no behavior change.
- **Depends on:** P3-004, P3-006, P3-007.
- **Likely files/packages:** `internal/cli/drive.go`, `internal/cli/run.go`,
  `internal/scheduler` (only if selection visibility needs it).
- **Acceptance criteria:** Normal tasks run unchanged; a policy escalation stops
  through the existing human boundary; no new task states are introduced.
- **Validation:** `go test ./internal/cli/... ./internal/scheduler/...`.

### P3-009 --- Implement pre-execution analysis seam

- **Objective:** Invoke JEV immediately before agent execution with bounded context.
- **Scope:** Build the pre-execution request (selected task, proposed execution
  context, relevant repository area) and call the `Analyzer` at the pre-execution
  purpose. Analysis only; it must not execute or mutate anything.
- **Depends on:** P3-002, P3-003, P3-005.
- **Likely files/packages:** `internal/cli/jev.go`, `internal/cli/run.go`,
  `internal/run/jev.go`.
- **Acceptance criteria:** Gate is analysis-only and read-only; no-op when
  disabled/absent; provider failure recorded separately from findings.
- **Validation:** `go test ./internal/cli/... ./internal/run/...`.

### P3-010 --- Define deterministic pre-execution policy mapping

- **Objective:** Map pre-execution evidence to a deterministic disposition.
- **Scope:** Classify structured pre-execution findings (scope expansion,
  unexpected area, destructive operation, security boundary, credential-sensitive
  work, requirement conflict, approval-sensitive operation) into the existing
  risk/kind vocabulary and evaluate with deterministic policy.
- **Depends on:** P3-002, P3-009.
- **Likely files/packages:** early-policy file; `internal/autonomy`.
- **Acceptance criteria:** Bounded normal tasks continue; destructive/security
  concerns reach a human boundary only when policy requires it; no prose matching.
- **Validation:** `go test ./internal/quality/... ./internal/autonomy/...`.

### P3-011 --- Integrate pre-execution gate

- **Objective:** Run the pre-execution gate after precheck and before
  implementation, and act on its deterministic disposition.
- **Scope:** Hook the pre-execution seam into the lifecycle immediately before the
  implement stage. Record observability and persist the result. Disabled = no
  behavior change.
- **Depends on:** P3-004, P3-009, P3-010.
- **Likely files/packages:** `internal/cli/run.go`, `internal/run/run.go`,
  `internal/scheduler`.
- **Acceptance criteria:** Normal bounded tasks continue; deterministic
  dispositions apply; no new task states.
- **Validation:** `go test ./internal/cli/... ./internal/scheduler/...`.

### P3-012 --- Add activity/report observability

- **Objective:** Expose the checkpoints via existing reporting/activity mechanisms.
- **Scope:** Add triage/pre-execution activity lines (for example `[TRIAGE]`,
  `[PRE_EXECUTION]`, `[AUTONOMY]`). Human-readable only; never used for policy.
- **Depends on:** P3-008, P3-011.
- **Likely files/packages:** `internal/activity/activity.go`, `internal/cli/activity.go`,
  `internal/cli/report.go`, `internal/cli/jev_report.go`.
- **Acceptance criteria:** Disabled JEV adds no noise; findings/confidence/severity
  are visible; report distinguishes triage/pre-execution/quality evidence.
- **Validation:** `go test ./internal/activity/... ./internal/cli/...`.

### P3-013 --- Add provider failure/fallback behavior

- **Objective:** Make the analysis-result vs provider-failure distinction explicit
  and policy-owned.
- **Scope:** Ensure a provider failure is recorded as a failure (never a finding)
  and that the configured policy fallback is deterministic: advisory fallback
  continues normal work; high-risk policy may fail closed or require authorization.
- **Depends on:** P3-006, P3-009 (with P3-007, P3-010).
- **Likely files/packages:** `internal/run/jev.go`, early-policy file,
  `internal/cli/run.go`.
- **Acceptance criteria:** Timeout/unavailable/malformed/invalid-schema/transport
  failures are classified as infrastructure failures; malformed results fail
  closed; no failure is interpreted as a finding.
- **Validation:** `go test ./internal/run/... ./internal/cli/...`.

### P3-014 --- Add security and ownership-boundary tests

- **Objective:** Prove JEV cannot mutate state or bypass gates.
- **Scope:** Tests that JEV cannot transition state, write files, commit, push,
  merge, approve, or modify persistence directly, and that the early gates cannot
  bypass validation/review/human approval.
- **Depends on:** P3-002 through P3-011.
- **Likely files/packages:** `internal/jev/boundary_test.go`,
  `internal/cli/jev_lifecycle_test.go`, plus new early-gate tests.
- **Acceptance criteria:** Ownership/boundary tests are deterministic and pass.
- **Validation:** `go test ./internal/jev/... ./internal/cli/...`;
  `go test -race ./internal/jev/... ./internal/cli/...`.

### P3-015 --- Add end-to-end regression tests

- **Objective:** Prove the early-decision lifecycle end to end with the fake
  analyzer.
- **Scope:** Cover: JEV disabled (behavior unchanged); triage no-findings →
  normal execution; structured ambiguity → policy evaluation; security-sensitive
  evidence → policy evaluation; provider failure → configured fallback; malformed
  result → fail safely; pre-execution normal → continue; scope concern →
  deterministic disposition; destructive/security concern → human boundary when
  policy requires.
- **Depends on:** P3-008, P3-011, P3-013.
- **Likely files/packages:** `internal/cli/*_test.go`, `internal/e2e`.
- **Acceptance criteria:** All flows demonstrated deterministically; no network,
  OpenJEV install, or LLM required; no manufactured success.
- **Validation:** `go test ./...`; `go test -race ./...`.

### P3-016 --- Dogfood Phase 3 with OpenJEV

- **Objective:** Exercise the real OpenJEV provider against the new checkpoints.
- **Scope:** Dogfood with JEV enabled for triage/pre-execution on a bounded task;
  record results. No automated test depends on the real provider.
- **Depends on:** P3-015.
- **Likely files/packages:** `docs/history/` (dogfood record), optional e2e fixture.
- **Acceptance criteria:** Real-provider run demonstrates the checkpoints; results
  are persisted and auditable; failure behavior observed and documented.
- **Validation:** manual dogfood run; documented outcome.

### P3-017 --- Phase 3 acceptance validation

- **Objective:** Confirm every Phase 3 acceptance criterion is met.
- **Scope:** Run the full validation suite, verify disabled behavior is
  backward compatible, verify docs distinguish implemented vs proposed behavior,
  and update the documentation index/status.
- **Depends on:** all.
- **Likely files/packages:** `docs/README.md`, `docs/requirements/PRD-Phase-3-OpenJEV.md`,
  `docs/specs/*`.
- **Acceptance criteria:** All PRD §8 criteria pass; existing tests pass.
- **Validation:** see Overall Validation.

## Dependency Graph

```text
P3-002 ─┬─ P3-001
        ├─ P3-003 ─┬─ P3-006 ─ P3-007 ─┐
        ├─ P3-004 ─┼─ P3-009 ─ P3-010 ─┤
        └─ P3-005 ─┘                   │
                                       ├─ P3-008 ─┐
                                       ├─ P3-011 ─┼─ P3-012
                                       └─ P3-013 ─┘
P3-008, P3-011, P3-013 ─ P3-015 ─ P3-016 ─ P3-017
P3-002..P3-011 ─ P3-014
```

## Overall Validation

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./...
```

Use the repository's canonical `make check` if applicable.

## Safety Invariants

Never require:

```bash
rm .agent-sdlc/state.db
git reset --hard
git clean
```

Preserve working-tree changes, task/run history, PLAN provenance,
validation/review evidence, and human approval. JEV must not manufacture
validation PASS, review PASS, CI PASS, PR creation, merge completion, or task
completion.

## Definition of Done

Phase 3 is complete when the two checkpoints are optional and disabled by default;
SOP remains the sole lifecycle authority; JEV provides structured, evidence-backed
early analysis and remains read-only; policy is deterministic and never infers a
lifecycle action from prose; provider failures are policy-owned and fail closed
where required; early evidence is persisted and auditable; existing JEV quality
analysis and recovery/reconciliation/human approval remain compatible; and
deterministic tests require no model or network.

## Out of Scope

- autonomous JEV code modification or shell execution;
- JEV-owned scheduling/retry loops;
- a second workflow engine or a parallel decision-provider architecture;
- moving JEV ownership into `sop-controller`;
- enabling either new gate globally by default.
