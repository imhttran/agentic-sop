# PLAN — JEV Wave 3: Adapter Integration

**Status:** APPROVED for registration (Gate W3-1). Not yet active.

## Project

agentic-sop

## Summary

Wire the ratified Gate 3 M3 bridge into production so SOP can invoke the
provider-neutral decision adapters (Julia-1 / Nimble) as an opt-in
`decision.provider: adapter`, without changing the existing `deterministic` and
`command` providers or any DTO semantics.

**Ratified architecture (Gate W3-1):**

- The adapter argv is a **dedicated `decision.adapter_command` field** (C1); the
  existing `decision.command` keeps its SEAM-002 `command`-provider meaning.
- **One bounded decision per call**: the bridge builds one question and interprets
  exactly one answer (C3).
- **SOP-owned structural profile recognition**: a profile is recognized by
  validating schema, question identity, answer type, and capability semantics; the
  provider name is a correlation hint only (C3).
- **Deterministic policy retains final authority** (`decision.Route` /
  `internal/autonomy`); adapters carry no execution or approval authority.
- **Explicit CLI invocation and exit-code contract** (C2): `<adapter-argv> decide
-file <tmp> -json -provider <p> -model <m> -timeout <d>`; `0`→ok, `2`→invalid
  request, `3`→unavailable, `1`/other→provider failure.
- **Existing adapter CLI transport is an accepted temporary divergence** (C2):
  `cmd/sop-decision-adapter` is a debugging/testing surface today; a follow-up
  adapter-repo task may add a `decide-stdin` subcommand (C7, external).
- **No breaking changes** to existing decision providers, DTOs, or configuration;
  profile data is config-driven and derived from the JEV-206 contracts, never a
  provider-name branch in policy (C3).

Approval gates (operator-owned; SOP must never cross them automatically):

- Gate W3-1 — approve and register this plan (this step).
- Gate W3-2 — implementation of JEV-209…JEV-212.
- Gate W3-3 — enable the adapter provider by default or change production policy.
- Gate W3-4 — any live adapter inference (required before JEV-213's live dogfood).

Cross-repository note: the connector lives in `agentic-sop` and invokes the
`sop-decision-adapters` executable as a subprocess over the batched profile
(`state`/`questions[]`/`metadata` → `provider`/`model`/`answers{}`/`usage`). No
`agentic-sop` package imports the adapters module; the only shared surface is the
JSON contract. Cross-repo follow-up (C7): amend the adapter CLI note or add
`decide-stdin` in `sop-decision-adapters` (tracked externally, not by this plan).

Evidence rule: each task is satisfied by native deterministic acceptance checks
that exercise production code (not test doubles); where a report is a deliverable,
a machine-readable evidence artifact recording command provenance, the repository
revision, and per-case results is required. A report that merely contains headings
is insufficient and cannot produce VERIFIED.

## Tasks

### JEV-209 — Production Adapter Connector

- **Objective:** Provide an opt-in production `decision.Provider` that translates a canonical SEAM-002 request through the M3 bridge, invokes the configured adapter executable over the batched profile, and returns canonical evidence — fail-closed and authority-free. Prerequisite: the ratified Gate 3 architecture and the committed M3 bridge (Wave 2 JEV-202/JEV-206), which are already complete.
- **Scope:** Add a provider-neutral connector package holding an injected M3 profile registry (kind -> question type / choice map / boolean threshold / score range). Per `Decide`: `bridge.Build` the profile request; write it to a private per-call temp file; invoke the adapter argv (from `decision.adapter_command`) with `-file/-json/-provider/-model/-timeout`; decode the batched result; `bridge.Interpret` into canonical `(Choice, Confidence)`; return a `decision.Decision`. Recognize the profile structurally; enforce exact answer identity; classify cancellation vs deadline distinctly; bounds on time/size; typed class-only failures. No execution or approval authority. No provider-name branch in policy.
- **Files:** internal/decision/commandprofile/connector.go, internal/decision/commandprofile/connector_test.go, internal/archtest/decision_seam_test.go
- **Depends on:** —
- **Acceptance:** the connector recognizes profiles structurally and maps real profile evidence to canonical decisions while preserving the human boundary; exact question-ID/answer-set identity is enforced; unknown/ambiguous/unsupported profiles fail closed; subprocess output and time are bounded; the private temp request file is created and removed; failures are typed, class-only, and secret-free; no breaking DTO or configuration change.
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorMapsProfileEvidence ./internal/decision/commandprofile/
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorFailsClosed ./internal/decision/commandprofile/
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorRecognizesProfileStructurally ./internal/decision/commandprofile/
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorEnforcesAnswerIdentity ./internal/decision/commandprofile/
- **Failure / rollback:** if the connector cannot preserve the human boundary or fails open, revert the package; the disabled default is a strict no-op.

### JEV-210 — Adapter Provider Configuration

- **Objective:** Add the opt-in `decision.provider: adapter` configuration value and the dedicated `decision.adapter_command` field, with validation and documentation.
- **Scope:** Add `adapter` to the accepted `decision.provider` set; add `DecisionConfig.AdapterCommand []string` (`yaml: adapter_command`); validation requires `adapter_command` when `provider: adapter`; wire `decisionProviderFromConfig` to build the connector with the config-driven profile registry; document both fields. `deterministic`, `command`, and the disabled default are unchanged.
- **Files:** internal/config/config.go, internal/config/decision_provider_test.go, internal/cli/decision_provider.go, internal/cli/decision_adapter_profile.go, docs/reference/CONFIGURATION.md
- **Depends on:** JEV-209
- **Acceptance:** `decision.provider: adapter` requires `decision.adapter_command` and is rejected when absent; existing providers and disabled behavior are unchanged; configuration does not silently imply an unsupported provider; no breaking change.
- **Acceptance check:** go test -count=1 -run TestDecisionProviderAdapterRequiresCommand ./internal/config/
- **Acceptance check:** go test -count=1 -run TestDecisionProviderRejectsUnsupported ./internal/config/
- **Failure / rollback:** unset `decision.enabled` or remove the adapter config to restore the deterministic baseline exactly (strict no-op).

### JEV-211 — Connector Security and Failure-Path Tests

- **Objective:** Prove the connector's security and failure-path behaviour with dedicated tests.
- **Scope:** Tests for bounded subprocess output/size, secret-free diagnostics and persisted reasons, `context.Canceled` vs `context.DeadlineExceeded` classification, and a fail-closed matrix (unavailable, malformed, unsupported, indeterminate, unexpected exit). Uses an injected runner/helper process; no live inference.
- **Files:** internal/decision/commandprofile/security_test.go
- **Depends on:** JEV-209
- **Acceptance:** cancellation is not misclassified as timeout; oversized output fails closed; no endpoint, body, token, or credential appears in diagnostics or state; every failure class routes to a human boundary and never to an approval.
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorDistinguishesCancelAndTimeout ./internal/decision/commandprofile/
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorBoundsSubprocess ./internal/decision/commandprofile/
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorNoSecrets ./internal/decision/commandprofile/
- **Failure / rollback:** a security failure blocks the connector; revert to the deterministic default (no-op).

### JEV-212 — Cross-Repository Contract and Compatibility Tests

- **Objective:** Validate the connector against the real adapter wire contract using the JEV-206 fixtures; independently runnable. Prerequisite: the Wave 2 JEV-206 recorded fixtures (already complete).
- **Scope:** Contract tests that feed JEV-206-derived batched wire documents (Julia-1, Nimble) through the bridge/connector and assert canonical mapping, answer identity, and fail-closed behaviour. Reuses existing fixtures; no live inference; no `agentic-sop` import of the adapters module. Independently runnable as soon as this plan is active.
- **Files:** internal/decision/commandprofile/contract_test.go, internal/decision/commandprofile/testdata/
- **Depends on:** —
- **Acceptance:** the real (recorded) adapter wire documents decode and interpret correctly; a batched multi-answer result is rejected (cardinality 1); unknown/ambiguous profiles fail closed; no adapter bypasses SOP approval rules.
- **Acceptance check:** go test -count=1 -run TestAdapterContractWireCompatibility ./internal/decision/commandprofile/
- **Failure / rollback:** a compatibility gap is documented; the connector is not enabled until resolved.

### JEV-213 — Adapter Integration and Live Dogfood

- **Objective:** Integrate the connector end-to-end and, under Gate W3-4, run a bounded live adapter dogfood in a disposable project.
- **Scope:** GATE W3-4 required before any live inference. Offline integration test first (in-process fake adapter executable). Then a disposable-project dogfood invoking Julia-1 (and/or Nimble) through the connector, recording a machine-readable, credential-free evidence artifact (command provenance, revision, provider/model, per-scenario results, exit status, governed outcome). No enabling by default (Gate W3-3).
- **Files:** internal/decision/commandprofile/integration_test.go, internal/decision/commandprofile/dogfood_evidence_test.go, docs/reports/JEV-WAVE3-ADAPTER-DOGFOOD.md, docs/reports/evidence/JEV-WAVE3-ADAPTER-DOGFOOD.json
- **Depends on:** JEV-209, JEV-210, JEV-211, JEV-212
- **Acceptance:** the offline integration test exercises the production connector; the live dogfood (only under W3-4) records provider/model/revision/per-scenario results with no credentials; a missing or incomplete artifact cannot be VERIFIED; JEV analysis remains evidence-only and adapters retain no authority.
- **Acceptance check:** go test -count=1 -run TestAdapterConnectorIntegration ./internal/decision/commandprofile/
- **Failure / rollback:** live inference is never automatic; on any unexpected provider failure the run stops and the deterministic default stands.

## Transport and invocation contract (JEV-209)

```text
<decision.adapter_command...> decide -file <tmp-request.json> -json -provider <p> -model <m> -timeout <d>
```

- The profile request comes from `bridge.Build`; the connector marshals it to
  `<tmp-request.json>` and passes it with `-file`.
- The adapter writes the batched `DecisionResult` JSON to stdout (`-json`).
- **Exit-code mapping:** `0` -> ok; `2` -> invalid request; `3` -> provider
  unavailable; `1`/other -> provider failure.
- **Temporary-file lifecycle:** private dir (`os.MkdirTemp`, mode 0700), request
  file mode 0600, whole dir removed with `defer`; never log request/response bodies.

## Security and reliability requirements

- `exec.CommandContext` with a discrete executable and argument vector — never a
  shell, never string interpolation of a path or request.
- Per-call context deadline; `WaitDelay`; bounded stdout/stderr; output beyond the
  bound is a provider failure, never a truncated success.
- `context.Canceled` and `context.DeadlineExceeded` classified distinctly; both
  fail closed.
- Structural profile recognition; exact answer identity; answer-type equality.
- Diagnostics and temp files never contain endpoints, bodies, tokens, or credentials.

## Non-goals

- No enabling the adapter provider by default (Gate W3-3).
- No change to `internal/decision` DTOs or the SEAM-002 `command` DTO.
- No adapter wire-contract change; the adapters add no version field (M3).
- No execution, approval, commit, merge, or lifecycle authority for adapters.
- No adapter-repo change here (the `decide-stdin` follow-up is external, C7).

## References

- docs/architecture/JEV-DECISION-CONTRACT.md (Gate 3, ratified)
- docs/reports/JEV-WAVE2-ADAPTER-COMPAT.md (JEV-206 cross-repository evidence)
- sop-decision-adapters docs/specs/decision-contract.md and bridge-profile.md
