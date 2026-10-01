# PLAN — Phase 5: Bounded Model Escalation & Execution Recovery

**Status:** Implemented. Bounded escalation is shipped, off by default.
**Spec:** [../specs/RECOVERY.md](../specs/RECOVERY.md) §8 and §3 (authoritative)
**Routing:** [../specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) (class selection)
**Config:** [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — "Bounded model escalation"

## Objective

Add deterministic, bounded execution recovery: after an attempt fails a quality
gate, SOP may retry the task on the next larger model class. Models produce work
and evidence; SOP decides whether to retry, escalate, stop, or ask a human.

```text
Task → JEV evidence → Router → SMALL|MEDIUM|LARGE → Execute
     → Validation/Review → Recovery policy → RetrySame | Escalate | Human | Block
```

Builds on Phase 3 (JEV evidence), Phase 3.5 (deterministic routing), and Phase 4
(provider runtime + opt-in availability validation).

## Rollout order

```text
P5-001  recovery vocabulary (Action / Evidence / Decision / Policy)
P5-002  deterministic recovery policy + unit tests
P5-003  attempt-record persistence
P5-004  escalation execution (resolve → validate → build → guard → execute)
P5-005  bounded failure-context handoff
P5-006  human / bounded termination at the existing boundary
P5-007  CLI + report visibility
P5-008  diagnostic escalation metrics
P5-009  tests, dogfood scenarios, documentation, full validation
P5-010  classify the claimed-change verdict (hardening)
```

Each step is additive and independently reversible; the new surface defaults OFF.

## Tasks

### P5-001 — Recovery vocabulary

- **Status:** Done.
- **Scope:** `recovery.Action` (`none`, `retry_same`, `escalate`, `human`,
  `block`), `recovery.Evidence`, `recovery.Decision`, `recovery.Policy`,
  `recovery.NextClass`, the fixed `recovery.Reason*` phrases, and the
  `EvidenceFrom` adapter over `failure.Classification`.
- **Files:** `internal/recovery/decision.go`, `evidence.go`.
- **Depends on:** —
- **Acceptance:** the vocabulary is closed and `failure` is reused rather than
  duplicated (Kind/Disposition), so recovery and the lifecycle classify failures
  the same way.

### P5-002 — Deterministic recovery policy

- **Status:** Done.
- **Scope:** `recovery.Decide(policy, evidence) Decision` — safety/approval first,
  then transient retry, then continuation, then bounded escalation.
- **Files:** `internal/recovery/policy.go`, `policy_test.go`.
- **Depends on:** P5-001.
- **Acceptance:** table-driven tests cover every ladder step, the escalation
  bound, safety override, transient retry, continuation, and the fail-closed
  unclassified case; the decision is proven deterministic; the ladder is proven
  one-way.

### P5-003 — Attempt-record persistence

- **Status:** Done.
- **Scope:** versioned, non-secret `run.AttemptRecord` written to
  `.agent-sdlc/runs/<task>/attempts/NNN.json`, failing closed on an unknown version
  or result; a forgiving reader for display surfaces.
- **Files:** `internal/run/attempt.go`, `attempt_test.go`.
- **Depends on:** —
- **Acceptance:** round-trip, deterministic ordering, and fail-closed writes are
  tested; the record never replaces `routing.json`.

### P5-004 — Escalation execution

- **Status:** Done.
- **Scope:** `runAttempts` wraps the lifecycle: on an `ESCALATE` decision it
  resolves the next class (`model.Resolve`), validates the selection when
  `providers.validate` is on, builds the agent, guards `IMPLEMENT`, and re-runs the
  lifecycle — reusing the Phase 3.5 routing seam (`deps.attempt`) so the class is
  recorded as `escalation` provenance and `routing.json` is never overwritten.
- **Files:** `internal/cli/escalation.go`, `internal/cli/routing.go`,
  `internal/cli/run.go`, `internal/cli/drive.go`, `internal/cli/modelroute.go`.
- **Depends on:** P5-002, P5-003.
- **Acceptance:** the selected model always equals the executing model; a missing
  agent factory, an unresolvable class, or a failed validation stops escalation
  with an actionable message instead of substituting a model.

### P5-005 — Bounded failure-context handoff

- **Status:** Done.
- **Scope:** the escalated attempt receives the previous class/model, the failure
  stage, the deterministic reason, and bounded validation/review evidence.
- **Files:** `internal/cli/escalation.go`, `internal/cli/run.go`.
- **Depends on:** P5-004.
- **Acceptance:** the context is capped (`escalationContextMax`), so an unbounded
  log or repository history is never replayed.

### P5-006 — Human / bounded termination

- **Status:** Done.
- **Scope:** `LARGE`, an exhausted escalation budget, and a safety boundary return
  control to the existing human/block path; nothing new is invented.
- **Depends on:** P5-002, P5-004.
- **Acceptance:** the ladder ends at `small → medium → large → human/blocked` with
  no fourth execution and no wrap-around.

### P5-007 — CLI + report visibility

- **Status:** Done.
- **Scope:** `Recovery:` / `Retrying:` blocks in `sop run`, and an
  `Execution attempts:` section (initial class, per-attempt class/model/result,
  recovery action) in `sop report`.
- **Files:** `internal/cli/escalation.go`, `attempts_visibility.go`,
  `internal/cli/report.go`.
- **Depends on:** P5-003, P5-004.
- **Acceptance:** initial routing and execution attempts are distinguishable;
  nothing is rendered when escalation is off.

### P5-008 — Diagnostic escalation metrics

- **Status:** Done.
- **Scope:** attempts, escalations, per-class starts, `small→medium` /
  `medium→large` counts, and success-after-escalation, derived from the attempt
  records.
- **Depends on:** P5-003, P5-007.
- **Acceptance:** metrics are observational only and never feed routing or recovery.

### P5-009 — Tests, dogfood, documentation, full validation

- **Status:** Done.
- **Scope:** policy table tests, run-package persistence tests, model/config
  resolution tests, and injected-CLI integration tests for the three dogfood
  scenarios (SMALL succeeds; SMALL fails → MEDIUM succeeds; SMALL → MEDIUM → LARGE
  fails → existing boundary), plus no-escalation cases (disabled, zero bound,
  infrastructure error, safety boundary, manual override).
- **Acceptance:** `gofmt`, `go build`, `go vet`, `go test ./...`, the race suite,
  and `make check` all pass; escalation is OFF by default and the non-escalation
  behavior is unchanged.

### P5-010 — Classify the claimed-change verdict (hardening)

- **Status:** Done.
- **Scope:** SOP's own deterministic verdict that a mutating invocation claimed
  success while leaving the working tree unchanged (`no repository changes`, on both
  the IMPLEMENT and FIX paths) is now classified (`failure.NoChangesProduced`, an
  implementation failure) instead of being left empty, so the failure is reported
  like any other and the bounded recovery policy MAY escalate it. The disposition is
  `AUTO_FIX`, so with escalation off the lifecycle still stops at the existing
  blocked boundary.
- **Files:** `internal/failure/failure.go`, `internal/recovery/policy.go`,
  `internal/cli/run.go`.
- **Depends on:** P5-004, P5-006.
- **Acceptance:** a SMALL no-change attempt escalates to MEDIUM and is recorded as
  `escalate`; the verdict is `NO_CHANGES_PRODUCED`/`AUTO_FIX`; a failure with no
  authoritative classification still fails closed; the task still blocks when
  escalation is off.

## Definition of Done

- Recovery uses typed evidence and is deterministic.
- Routing and recovery remain separate concepts.
- Transient failures retry the same class; `SMALL → MEDIUM → LARGE` escalates;
  `LARGE` cannot escalate further; retries and escalations are bounded.
- Safety policy overrides escalation; models can never grant themselves authority.
- Every escalated provider/model is validated, and the selected model always
  equals the executing model.
- Failed-attempt context is passed forward and every attempt is persisted.
- Reports distinguish initial routing from execution attempts.
- Escalation is feature-flagged and OFF by default; behavior is unchanged when off.

## Known limitation

None outstanding. The previously-recorded limitation — a gate failure that produced
no authoritative failure classification (an agent claiming success while changing
nothing) was not escalated — is **closed** by P5-010: that verdict is now classified
(`failure.NoChangesProduced`) and participates in bounded recovery. A failure with no
authoritative classification at all still fails closed to the existing human/block
path by design (see [../specs/RECOVERY.md](../specs/RECOVERY.md) §8).

## Out of scope

Automatic provider fallback, cheapest/latency/benchmark-driven selection, provider
scoring, learned routing, automatic downgrade, unlimited retries, distributed or
remote execution, SOP Hub, MCP orchestration, and parallel competing models.
