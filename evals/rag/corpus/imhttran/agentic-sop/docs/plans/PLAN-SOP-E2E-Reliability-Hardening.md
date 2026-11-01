# PLAN — SOP E2E Reliability Hardening

**Status:** PROPOSED — derived from `PLAN-SOP-End-to-End-Reliability` (ETOE-001…010). NOT activated, NOT executed.
**Basis:** ETOE-009 scorecard and ETOE-010 final report (`docs/reports/end-to-end-reliability/`).
**Rule:** Do not declare the SOP platform production-ready. The deterministic fixture is NOT proof of real-model reliability.

## Project

SOP E2E Reliability Hardening

## Summary

Convert the end-to-end reliability assessment's observed defects and UNAVAILABLE coverage into a
scoped, prioritized hardening backlog. Ten tasks. P1: HARDEN-001b (caller-owned criterion
verification — prerequisite), HARDEN-001 (deterministic acceptance enforcement), HARDEN-002
(operator-authorised cross-repository workspace), HARDEN-003 (controller runtime parity). P2:
HARDEN-004 (successful FIX-recovery coverage), HARDEN-005 (live decision-adapter dispatch),
HARDEN-006 (explicit project-root execution safeguards), HARDEN-007 (attempt-scoped artifact
isolation), HARDEN-008 (model PLAN/tool-protocol convergence). Proposed backlog: HARDEN-009
(explicit plan-handoff disposition).

Acceptance enforcement is only legitimate if a criterion's outcome derives from an **observed**
result, not a model claim. HARDEN-001b therefore lands first: it adds operator-owned, deterministic
verifier bindings so `MET` is produced by SOP executing a trusted verifier, never by model text.

## HARDEN-001b — Caller-Owned Criterion Verification

Priority: P1. Give every required acceptance criterion a stable identifier and an operator-owned
deterministic **verifier binding**, so SOP can execute the verifier and observe the result rather
than trust a model claim. Reuse existing trusted execution mechanisms (the validated command/
validation runner) rather than adding a new execution surface.

### Dependencies

None

### Deliverables

- Task-specification extension: stable `criterion_id` per criterion + operator-owned `verify`/`expect` binding (deterministic command or named test).
- A bounded verifier runner (timeout + output cap) reusing existing trusted execution, returning observed exit status.
- Machine-readable criterion-evidence artifact recording command identity, exit status, timestamp, and artifact references.
- `internal/toolharness`/command-policy reuse so verifiers obey existing sandbox/authorization restrictions; model-authored or model-supplied verifier definitions are refused.
- `docs/specs/` + `docs/reference/` documentation of the binding and the trust model.

### Acceptance Criteria

- Every required criterion can be bound to a trusted (operator-owned) verifier; a criterion without a binding is UNAVAILABLE.
- SOP executes the verifier and observes its result; the outcome is derived from the observed exit status, not from model text.
- Model-authored claims (including `CompletionEvidence.Acceptance`) cannot establish MET.
- Missing, malformed, unsupported, or unexecutable bindings fail closed (UNAVAILABLE → not MET).
- Verification evidence records command identity, execution outcome, timestamp, and artifact references.
- Tests cover successful, failed, unavailable, and unauthorized verification.
- No regression in existing validation or approval boundaries.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If a verifier cannot be executed under the existing sandbox/authorization restrictions, it is recorded UNAVAILABLE (never MET); the change is additive and separately testable, and is reverted if it alters existing validation.

## HARDEN-001 — Deterministic acceptance-criteria enforcement

Priority: P1. Consume the trusted criterion outcomes from HARDEN-001b and make a
declared-but-unmet acceptance criterion a blocking, model-free gate outcome, **enforced by
default**. Located at the gate/quality evaluator that emits `gate.json`; the run loop writes a
machine-readable `criteria.json` (per-criterion MET/NOT_MET/UNAVAILABLE) built from the verifier
outcomes and passes it to the gate, which blocks completion (no PASS) when any required criterion
is NOT_MET/UNAVAILABLE, or when the evidence is missing/malformed.

### Dependencies

- HARDEN-001b

### Deliverables

- The gate/quality evaluator change enforcing trusted criterion outcomes by default (`internal/quality`, wired at `internal/cli/run.go`).
- The `criteria.json` writer + schema (per-criterion tri-state from HARDEN-001b verifier results).
- A regression test reproducing ETOE-007 (3 declared criteria, 0 MET) asserting the gate blocks.
- Migrated fixtures providing real verifier bindings (never fabricated MET).
- `docs/specs/` note recording the new gate semantics.

### Acceptance Criteria

- Consumes trusted criterion outcomes (never model claims); enforcement is on by default with no permissive fallback.
- A task with any required criterion NOT_MET/UNAVAILABLE cannot reach LOCAL_DONE without an explicit operator authorization record.
- Missing or malformed criterion evidence blocks; a fully-MET task is unchanged.
- Existing severity-based quality checks and human-approval boundaries remain enforced; existing FIX/human-escalation limits are unchanged.
- The ETOE-007 regression (0/3 MET) blocks; affected fixtures are migrated with real verifier bindings.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If a fully-MET task regresses, revert the package; the gate must be additive and separately testable.

## HARDEN-002 — Operator-authorised cross-repository integration workspace

Priority: P1. Provide a supported, explicit, non-production mechanism for a governed task to
inspect an allow-listed sibling repository without weakening repository-root confinement for
ordinary work. The allow-list is an **operator-controlled** configuration
(`tool_allow_roots` config key and/or `SOP_TOOL_ALLOW_ROOTS` env), default empty/OFF. It is read
**only** from operator configuration or the environment and **never** from plan, task, or model
text. Granting is audited to a machine-readable artifact.

### Dependencies

None

### Deliverables

- `internal/toolharness` support for an operator-supplied additional authorized root (config/env), default OFF; read-only access only.
- The audit artifact writer.
- Tests proving (a) default confinement is byte-for-byte unchanged, (b) an allow-listed sibling path is readable read-only and the grant is audited, (c) plan/model text can never grant access.
- `docs/reference/` documentation.

### Acceptance Criteria

- With no allow-root configured, sibling access is denied exactly as today.
- With one explicitly configured, the allow-listed path is readable read-only and the authorization is recorded.
- No plan/task/model text can add an allow-root; the mechanism never enables writes to a sibling or production state.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

Any test showing ordinary runs gaining sibling access disables the feature (fail-closed) and reverts.

## HARDEN-003 — Controller runtime parity verification

Priority: P1. Exercise `sop-controller` against a disposable SOP project and verify read-only
state display and delegated-action parity with the SOP CLI, including pending approvals, using
HARDEN-002 for reachability, with actual runtime checks.

### Dependencies

- HARDEN-002

### Deliverables

- `docs/reports/end-to-end-reliability/HARDEN-003-controller-parity-evidence.md` — build/serve transcripts, `/healthz` response, parity table, permission findings, teardown log.

### Acceptance Criteria

- `/healthz` responds; the server binds loopback by default; a gated route returns 401 without a token; state-changing routes enforce CSRF.
- Controller display and delegated actions match `sop status`/`sop task`/`sop approvals --json` for the same disposable project, including pending approvals.
- The controller mutates authoritative state only through SOP's CLI; no direct state writes observed.
- The disposable controller instance and its project are torn down after the run.

### Validation Commands

Controller: `go build ./...`; `go test ./...`; live `/healthz`/auth/CSRF probes; SOP-side `sop status`/`sop approvals --json` parity. Agentic-SOP: `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If parity fails, record the exact divergent view/action and keep the prior UNAVAILABLE marking; do not assert parity.

## HARDEN-004 — Successful FIX recovery coverage

Priority: P2. Add a fixture scenario with a deterministic FAIL → FIX → PASS recovery.

### Dependencies

None

### Deliverables

- An added fixture scenario + a run captured under `docs/reports/end-to-end-reliability/` evidence.

### Acceptance Criteria

- A run shows validation FAIL → FIX → revalidation PASS → `LOCAL_DONE`, with recorded run id and artifacts.
- The recovery is deterministic (controlled provider); no external model required.
- The failed→recovered transition is reproducible from the recorded commands.

### Validation Commands

`bash -n scripts/etoe-*-fixture-*.sh`; run the scenario in an isolated root; `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`; `scripts/checks/check-doc-links.sh`.

### Failure Behavior

If the scenario cannot reach PASS deterministically, record the bounded behavior and do not claim recovery.

## HARDEN-005 — Live decision-adapter dispatch and fallback verification

Priority: P2. Configure `decision.provider: command` with `decision.command` pointing at an adapter
fixture and verify a real dispatch with a recorded decision trace, plus contract-supported
fallback; unsupported paths stay UNAVAILABLE.

### Dependencies

- HARDEN-002

### Deliverables

- An adapter fixture + `docs/reports/end-to-end-reliability/HARDEN-005-decision-dispatch-evidence.md` — dispatch trace, normalized results, fallback outcomes.

### Acceptance Criteria

- A real SOP→adapter dispatch occurs (trace/log artifact proves it), and the normalized result is consumed by SOP policy.
- Provider failure/fallback is exercised only where the contract supports it; unsupported paths labeled UNAVAILABLE.
- Adapter output cannot override SOP policy (verified).

### Validation Commands

Agentic-SOP: `gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -race -count=1 ./...`; `git diff --check`. Adapter repo: `go test ./...` (offline).

### Failure Behavior

If no dispatch can be evidenced, keep integration UNAVAILABLE and record the failing configuration.

## HARDEN-006 — Explicit project-root execution safeguards

Priority: P2. Require explicit project targeting for mutating commands so a bare `sop run` can
never silently execute the active plan.

### Dependencies

None

### Deliverables

- `internal/cli` change requiring an explicit selector for mutating runs, with tests.
- `docs/reference/CLI.md` update.

### Acceptance Criteria

- A mutating `sop run` with no selector refuses or requires explicit confirmation in non-interactive contexts.
- Existing explicit invocations (`sop run <PLAN.md>`, `--task`, `--max-tasks`) and read-only commands are unchanged.
- A regression test reproduces the recorded accidental-run scenario and asserts it is prevented.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If any supported invocation regresses, revert; the safeguard must be conservative and separately testable.

## HARDEN-007 — Attempt-scoped artifact isolation

Priority: P2. Store retry artifacts attempt-scoped so a failed attempt's evidence survives a retry,
with latest-attempt resolution for consumers.

### Dependencies

None

### Deliverables

- `internal/run`/`internal/cli` change for attempt-scoped artifacts + tests.
- Migration note (existing single-attempt dirs remain readable).

### Acceptance Criteria

- A retried task preserves the prior attempt's `state.json`/`trace.json`/`validation.json` under an attempt-scoped path.
- `sop report`/`continue`/`approvals` resolve the latest attempt; existing flat layouts keep working.
- No evidence is overwritten across attempts.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If consumers break on the new layout, keep the flat layout and add the attempt-scoped copy alongside; never delete prior evidence.

## HARDEN-008 — Model / harness diagnostic convergence improvements

Priority: P2. Improve diagnostics distinguishing model-output failures from harness/infra
failures, with bounded, provider-neutral recovery, without raising budgets.

### Dependencies

None

### Deliverables

- Bounded, provider-neutral improvements to PLAN/tool-protocol handling + tests.
- `docs/specs/AGENT-PROVIDER.md` §9 update recording the model-vs-harness diagnostic distinction.

### Acceptance Criteria

- Malformed tool requests and PLAN synthesis failures produce a clear, actionable diagnostic (not a generic failure).
- No iteration, stale, tool-call, or retry budget is raised.
- Existing convergence behavior is preserved (regression-tested).

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `git diff --check`.

### Failure Behavior

If a change relaxes a bound or alters unrelated lifecycle behavior, revert; improvements must be additive.

## HARDEN-009 — Explicit plan-handoff disposition (proposed backlog item)

Priority: P3 (proposed backlog; do NOT execute in this plan's first pass). SOP offers only
`COMPLETE`/`SUPERSEDED` dispositions; neither expresses "assessment complete — implementation
blocked (HOLD)". Provide an explicit, honest handoff/closure disposition (or a documented
convention) so a completed-but-blocked assessment can be released without over-claiming runtime
verification.

### Dependencies

None

### Deliverables

- A design note and (if adopted) a disposition/convention change + `docs/specs/` update.

### Acceptance Criteria

- A documented mechanism (or convention) lets an assessment plan be released without implying full runtime verification.
- No existing `COMPLETE`/`SUPERSEDED` semantics change; the addition is explicit and opt-in.
- The HOLD/UNAVAILABLE findings remain externally referenced.

### Validation Commands

`gofmt -l .`; `go build ./...`; `go vet ./...`; `go test -count=1 ./...`; `git diff --check`; `scripts/checks/check-doc-links.sh`.

### Failure Behavior

If adopted and it alters existing dispositions, revert; this is a proposal until authorized.

## Dependency graph

```text
HARDEN-001b ──► HARDEN-001                 (acceptance enforcement consumes trusted criterion outcomes)
HARDEN-002  ──► HARDEN-003, HARDEN-005     (cross-repo workspace prerequisite)
HARDEN-004, HARDEN-006, HARDEN-007, HARDEN-008   (independent)
HARDEN-009  (proposed backlog; independent)
```

## Parallelizable groups

- **Group A (independent, separate worktrees):** HARDEN-001b, HARDEN-002, HARDEN-004, HARDEN-006, HARDEN-007, HARDEN-008.
- **Group B (after HARDEN-002):** HARDEN-003, HARDEN-005 (may run in parallel).
- **Group C (after HARDEN-001b):** HARDEN-001.
- **Backlog (not executed this pass):** HARDEN-009.

## Recommended execution order

1. HARDEN-001b (prerequisite — trusted criterion verification).
2. HARDEN-001 (acceptance enforcement by default) + HARDEN-002 (cross-repo workspace) in parallel.
3. HARDEN-003 and HARDEN-005 (after HARDEN-002); HARDEN-004, HARDEN-006, HARDEN-007, HARDEN-008 independently.
4. HARDEN-009 only if authorized later.
