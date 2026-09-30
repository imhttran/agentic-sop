# Phase 3 Early-JEV Dogfood

This document records the observable behavior of the Phase 3 early-JEV
checkpoints — task triage and pre-execution — and how to reproduce it. It is a
point-in-time, non-normative historical record; the normative behavior lives in
[docs/specs/OPENJEV.md](../specs/OPENJEV.md) and
[docs/reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

It has two clearly separated parts:

1. **The deterministic fake-analyzer demonstration** (this document's main
   subject): a fully reproducible walkthrough of the early checkpoints that
   requires **no network, no live OpenJEV provider, and no LLM**. It runs entirely
   against the committed deterministic `jev.FakeAnalyzer` and the committed tests.
2. **The separate real-provider procedure** (Section 5): the procedure for
   running the early checkpoints against a live OpenJEV provider. **No
   real-provider run has been performed for P3-016**; that section documents the
   procedure only and claims no run.

Nothing here is a substitute for the tests. Where a claim is made about
behavior, it is backed by a committed test named below.

## 1. What the early checkpoints are

Phase 3 adds two **optional, disabled-by-default** JEV checkpoints that run
*before* implementation:

- **Task triage** — runs after task selection, before implementation. Purpose
  `task_triage`. Configuration key `early_jev.gates.task_triage`.
- **Pre-execution** — runs after precheck, immediately before the implement
  stage. Purpose `pre_execution`. Configuration key `early_jev.gates.pre_execution`.

Both are read-only analysis. The boundary is preserved:

```text
JEV analyzes.        (structured evidence only)
SOP decides.         (deterministic policy: autonomy.DecideEarly)
IMPLEMENT/FIX changes code.
```

An early checkpoint never transitions task state, mutates the repository,
commits, pushes, merges, approves, or writes a quality PASS. Its only persistence
is a diagnostic run artifact (Section 4). A provider failure is recorded distinctly
and is **never** a finding or a PASS.

### Disabled by default

The early layer is off unless `early_jev.enabled: true` is set, and each gate is
then additionally off unless its own `gates.*` flag is `true`. Enabling the early
layer does **not** require or imply `quality.jev.enabled`, and vice versa — they
are independent capabilities. When the gates are off, `sop run` behaves exactly
as before.

## 2. The deterministic fake-analyzer demonstration

The demonstration is driven by `internal/jev/fake.go`'s `FakeAnalyzer`, which
performs no model, network, or filesystem access and no repository mutation. Its
`Scenarios()` are the closed set the early-decision lifecycle must cover:

| Scenario                  | Result status | Evidence                                | Policy shape                          |
| ------------------------- | ------------- | --------------------------------------- | ------------------------------------- |
| `clear`                   | PASS          | `Evidence{Status: PASS}`                | continue                              |
| `ambiguous`               | INCOMPLETE    | `Evidence{Status: INCOMPLETE}`, ambiguity item | advisory / policy-evaluated    |
| `scope-concern`           | FINDINGS      | `Evidence{Status: FINDINGS}`, scope item | policy-evaluated (MEDIUM)            |
| `destructive-concern`     | FINDINGS      | `Evidence{Status: FINDINGS}`, security item | policy-evaluated (HIGH)          |
| `provider-failure`        | — (error)     | none (empty `Result`)                   | recorded as provider failure only     |
| `malformed`               | —             | unknown purpose → fails validation      | fail closed (never a PASS)            |

Builders: `NewClearFake`, `NewAmbiguousFake`, `NewScopeConcernFake`,
`NewDestructiveConcernFake`, `NewProviderFailureFake`, `NewMalformedFake`.

### 2.1 Configuration

Enable the early layer and both gates in `.agent-sdlc/config.yaml`:

```yaml
early_jev:
  enabled: true
  mode: review
  gates:
    task_triage: true
    pre_execution: true
  # fail_on names the severities that escalate to a human boundary;
  # omit it to reuse quality.fail_on (whose default is critical,high).
  fail_on:
    - critical
    - high
```

Each gate independently requires `early_jev.enabled: true`; enabling a gate
without the layer is a load-time error
(`config: early_jev.gates.task_triage requires early_jev.enabled: true`).

### 2.2 Activity lines

Each checkpoint emits stage-correct activity events, distinct from the quality
`JEV` stage:

- `[TRIAGE]` — task-triage checkpoint (`activity.StageTriage`).
- `[PRE_EXECUTION]` — pre-execution checkpoint (`activity.StagePreExecution`).
- `[AUTONOMY]` — the risk-based autonomy/disposition decision
  (`activity.StageAutonomy`).

The emitted events are a concise, secret-free `action` plus `detail`. For an
analysis that produced evidence the detail carries the finding count, the highest
severity seen, and the stated confidence, for example:

```text
[TRIAGE] analyzing JEV task analysis
[TRIAGE] completed findings: 1; severity: HIGH; confidence: 0.50; disposition: requires_human
[PRE_EXECUTION] analyzing pre-execution JEV analysis
[PRE_EXECUTION] completed findings: 0; disposition: continue
```

For a provider failure the detail is `provider failure recorded` and the action is
`unavailable` — never a finding. A disabled checkpoint emits nothing, so JEV off
adds no observability noise.

### 2.3 The persisted artifact

When a checkpoint runs, its diagnostic result is persisted best-effort into the
task's run directory under `.agent-sdlc/runs/<TASK>/`:

- `early-jev.json` — the latest early-checkpoint result (pretty-printed JSON).
- `early-jev-history.jsonl` — append-only history, one compact JSON object per
  result.

These are deliberately distinct from `jev.json` / `jev-history.jsonl` (the
quality-seam artifacts), so early and quality evidence never overwrite each other.
The artifact is versioned (`version: 1`) and records `checkpoint`
(`task_triage` / `pre_execution`), `task`, `evidence`, `provider`, `timestamp`,
`provider_failed`, `policy_decision`, and `policy_reason`. It is diagnostic
evidence only: nothing reads it back to drive a decision.

A representative `early-jev.json`:

```json
{
  "version": 1,
  "checkpoint": "task_triage",
  "task": "P3-DEMO",
  "evidence": {
    "version": 1,
    "purpose": "task_triage",
    "status": "FINDINGS",
    "severity": "HIGH",
    "confidence": 0.5,
    "items": [
      {
        "purpose": "security",
        "severity": "HIGH",
        "category": "destructive",
        "detail": "change may delete or overwrite data irreversibly"
      }
    ]
  },
  "provider": "fake",
  "timestamp": "2024-01-01T00:00:00Z",
  "provider_failed": false,
  "policy_decision": "requires_human",
  "policy_reason": "destructive change requires a human boundary"
}
```

A provider failure persists `provider_failed: true` and a zero-valued
`evidence` block; the artifact validator rejects a non-zero evidence payload
alongside `provider_failed: true` so a failure can never be read as a PASS.

## 3. Reproducing the deterministic demonstration

No network, live provider, or LLM is required. Run the committed tests that
demonstrate each flow:

```bash
# Early-triage flows (disabled, clear, blocking finding, provider failure,
# malformed) and pre-execution gate behavior — all against the fake analyzer.
go test ./internal/cli -run 'EarlyTriage|EarlyLayer|EarlyPreExecution' -v

# Graph-level triage lifecycle: disabled no-op, clear continues, escalation
# stops at the human boundary, provider failure continues.
go test ./internal/cli -run 'TestGraphRunTriage' -v

# Graph-level pre-execution lifecycle: disabled no-op, clear continues,
# escalation stops at the human boundary, invalid evidence continues, and the
# checkpoint runs after triage and before implementation.
go test ./internal/cli -run 'TestGraphRunPreExecution' -v

# Pre-execution seam unit tests: disabled no-op, absent analyzer no-op, provider
# failure recorded separately, findings recorded with evidence, no state mutation.
go test ./internal/cli -run 'TestPreExecution' -v

# The deterministic fake and its scenario builders.
go test ./internal/jev/...

# The early-artifact persistence contract.
go test ./internal/run/...
```

Committed tests backing each flow:

| Flow                                            | Test                                                          |
| ----------------------------------------------- | ------------------------------------------------------------- |
| Triage disabled → strict no-op                  | `TestEarlyTriageDisabledIsNoOp`                               |
| Triage clear → continues                        | `TestEarlyTriageClearContinues`                               |
| Triage blocking finding → escalates             | `TestEarlyTriageBlockingFindingEscalates`                     |
| Triage provider failure → continues             | `TestEarlyTriageProviderFailureContinues`                     |
| Triage malformed result → fail closed           | `TestEarlyTriageMalformedResultContinuesFailClosed`           |
| Layer enabled but no gates → no-op              | `TestEarlyLayerWithoutGatesIsNoOp`                            |
| Pre-execution gate → escalates                  | `TestEarlyPreExecutionGateEscalates`                          |
| Pre-execution disabled → no-op                  | `TestPreExecutionDisabledIsNoOp`                              |
| Pre-execution absent analyzer → no-op           | `TestPreExecutionAbsentAnalyzerIsNoOp`                        |
| Pre-execution provider failure → recorded only  | `TestPreExecutionProviderFailureRecordedSeparately`           |
| Pre-execution findings → recorded with evidence | `TestPreExecutionFindingsRecordedWithEvidence`                |
| Pre-execution seam → does not mutate state      | `TestPreExecutionSeamDoesNotMutateState`                      |
| Graph triage: disabled / clear / escalate / fail | `TestGraphRunTriageDisabledIsNoOp`, `TestGraphRunTriageClearContinues`, `TestGraphRunTriageEscalationStopsAtHumanBoundary`, `TestGraphRunTriageProviderFailureContinues` |
| Graph pre-execution: disabled / clear / escalate / invalid / ordering | `TestGraphRunPreExecutionDisabledIsNoOp`, `TestGraphRunPreExecutionClearContinues`, `TestGraphRunPreExecutionEscalationStopsAtHumanBoundary`, `TestGraphRunPreExecutionInvalidEvidenceContinues`, `TestGraphRunPreExecutionRunsAfterTriageBeforeImplementation` |

All of the above run without a network, a live OpenJEV provider, or an LLM.

## 4. What the demonstration shows

- **Disabled is unchanged.** With the gates off, triage and pre-execution are
  strict no-ops: no activity, no artifact, no disposition, no state change.
- **Clear continues.** A PASS analysis produces no escalation and the run
  continues normally.
- **Findings are policy-evaluated, not prose-matched.** Structured, typed
  evidence (purpose, severity, category) is mapped by deterministic policy
  (`autonomy.DecideEarly`) into a disposition; a destructive/security finding can
  require a human boundary, but the decision never comes from summary text.
- **Provider failure is distinct.** A failure to run is recorded as a provider
  failure with no evidence payload, never as a finding and never as a PASS, and
  the advisory default continues normal work.
- **Malformed fails closed.** Invalid evidence is rejected by fail-closed
  validation and is never accepted as a PASS.
- **No state mutation.** The seam reads task context and writes only a diagnostic
  artifact; it never transitions task state.

## 5. Real-provider procedure (NOT PERFORMED)

The procedure below describes how to run the early checkpoints against a **live
OpenJEV provider**. It is included for completeness and is separate from the
deterministic demonstration above.

> **No real-provider run has been performed for P3-016.** The early-JEV dogfood
> recorded here was performed entirely against the deterministic fake analyzer.
> Everything in this section is a procedure to be followed when a real-provider
> dogfood is actually run; no result is claimed here.

Procedure (to be executed when a live provider is available):

1. Configure a real analyzer/provider per
   [docs/specs/OPENJEV.md](../specs/OPENJEV.md) and
   [docs/reference/JEV-OPERATIONS.md](../reference/JEV-OPERATIONS.md).
2. Enable the early layer and the desired gates in `.agent-sdlc/config.yaml`
   (`early_jev.enabled: true`, `gates.task_triage` / `gates.pre_execution`).
3. Run a task through the triage/pre-execution checkpoints with a real provider.
4. Observe the `[TRIAGE]` / `[PRE_EXECUTION]` / `[AUTONOMY]` activity lines and
   the persisted `early-jev.json` / `early-jev-history.jsonl` artifacts.
5. Record the actual observed behavior as a separate, clearly-labeled real-run
   record — do not merge it into the deterministic record above.

The deterministic demonstration (Sections 2–4) does not depend on any of this and
remains reproducible with no network, provider, or LLM.

## See Also

- [docs/plans/PLAN-Phase-3-OpenJEV.md](../plans/PLAN-Phase-3-OpenJEV.md) — the
  Phase 3 plan (P3-001–P3-017), including P3-016.
- [docs/specs/OPENJEV.md](../specs/OPENJEV.md) — the normative JEV boundary and
  early-checkpoint specification.
- [docs/reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — the
  `early_jev` configuration keys and defaults.
- [docs/reference/JEV-OPERATIONS.md](../reference/JEV-OPERATIONS.md) — enabling
  JEV and provider configuration.
- [docs/history/OLLAMA-DOGFOOD.md](OLLAMA-DOGFOOD.md) — the repository's existing
  dogfood-record style.
- `internal/jev/fake.go` — the deterministic `FakeAnalyzer` and its scenarios.
- `internal/run/early_jev_artifact.go` — early-artifact persistence.
- `internal/cli/early_jev.go` — the triage/pre-execution seam and activity lines.
- `internal/cli/early_jev_report.go` — the early-checkpoint report renderer.
- `internal/activity/activity.go` — `StageTriage` / `StagePreExecution` /
  `StageAutonomy`.
