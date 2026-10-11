# ETOE-004 — Decision adapter integration matrix

**Status:** decision-adapter integration is **UNAVAILABLE**. This is a read-only audit
of actual SOP wiring versus the standalone decision adapter. No runtime or lifecycle
behavior was exercised; classifications below cite concrete repository evidence.

---

## 1. Scope and method

This audit answers one question: **which decision-layer capabilities are integrated
into SOP, and which exist only in the standalone adapter surface?**

- **Integrated** = reachable through SOP's own provider/config/run path in this
  repository, using SOP code that SOP itself constructs.
- **Standalone-only** = implemented in a package that can be invoked or tested on its
  own, but for which no SOP call site constructs or dispatches it. Standalone package
  tests are **not** evidence of SOP integration.

Evidence is limited to files inspected in this repository. No SOP-to-adapter dispatch
was observed, so integration remains UNAVAILABLE regardless of standalone test results.

---

## 2. The exact integration seam

**Integration seam:** `decision.provider: command` plus `decision.command`.

The seam is a **process-level** boundary: when configured, SOP would invoke an external
executable (given as an argv in `decision.command`) that reads the SEAM-002 request DTO
on stdin and writes the SEAM-002 result DTO on stdout.

| Seam element | Value / meaning | Evidence |
| --- | --- | --- |
| `decision.provider` | Optional decision layer; accepted values include `deterministic`, `jev`, and `command`. | `docs/reference/CONFIGURATION.md` (`decision.provider` row; default `deterministic`) |
| `decision.command` | The executable argv of an external, provider-neutral decision adapter; required when `provider: command`. | `internal/config/config.go` (DecisionConfig docs and validation) |
| Validation | `config: unknown decision.provider %q (want deterministic, jev, command)` and `config: decision.provider "command" requires decision.command`. | `internal/config/config.go` |
| Config field wiring | `decision:` YAML section maps to a `DecisionConfig` (Provider + Command). | `internal/config/config.go` |
| Config parse test | Parses `provider: command` with a command argv; rejects command without argv. | `internal/config/decision_test.go` |

**Configured in the audited project?** **Not configured.** No `decision:` section with
`provider: command` and a `command:` argv was found in the audited project's
configuration in this repository; the project default is `deterministic` (the decision
layer is effectively disabled / baseline). Therefore the seam exists **in SOP's config
schema** but is **not configured in the audited project**, and no SOP-to-adapter
dispatch is configured.

> If the audited project is external to this repository, this finding is limited to the
> repository-readable configuration and the state should be treated as
> **undeterminable** for that external project. No external project path was supplied.

---

## 3. Integration status

### 3.1 Decision-adapter integration — **UNAVAILABLE**

Decision-adapter integration is marked **UNAVAILABLE** because:

1. The seam `decision.provider: command` plus `decision.command` is **not configured**
   in the audited project, and
2. **No verified SOP-to-adapter dispatch** (no SOP call site that constructs the adapter
   and routes a real decision through it at runtime, and no run trace/log/artifact
   showing one) was observed.

**Standalone adapter tests are not evidence of integration.** The `internal/decision/command`
package has its own tests that exercise the adapter in isolation; passing those tests
would demonstrate adapter-internal behavior only, not that SOP wires the adapter into its
run path. Consequently they cannot lift the UNAVAILABLE marking.

**Criterion to lift UNAVAILABLE:** both of the following must hold —

- `decision.provider: command` **and** `decision.command` are **configured** in the
  audited project, **and**
- a **verified SOP-to-adapter dispatch** exists (SOP constructs the adapter and a real
  decision is routed end-to-end, evidenced by a run trace/log/artifact).

---

## 4. Integration matrix

Legend: **INTEGRATED** = reachable through SOP; **STANDALONE-ONLY** = adapter-local only;
**UNVERIFIED** = insufficient direct evidence; **UNAVAILABLE** = not wired/not configured.

| Capability | Where it lives | Classification | Evidence |
| --- | --- | --- | --- |
| Deterministic provider (baseline rules) | `internal/decision/decision.go` (`DeterministicProvider`) | **INTEGRATED** | Provider is the default; reachable via `decision.NewProvider` (`""`/`deterministic`). |
| `jev` provider | Config accepts `jev`; `decision.NewProvider` | **UNVERIFIED** | Config allows `jev`; provider implementation is not shown. |
| Process-level `command` provider seam (`decision.provider: command` + `decision.command`) | `internal/config/config.go` (schema/validation) | **UNAVAILABLE** (not configured) | Seam exists and validates, but no `decision:`/`provider: command` config found in the audited project. |
| External process adapter | `internal/decision/command/command.go` (`command.New`, `Adapter.Decide`) | **STANDALONE-ONLY** | Adapter maps an external process to `decision.Provider`; no SOP call site constructs it. |
| LOW/MEDIUM/HIGH routing | `internal/decision/decision.go` (`Route`, `Thresholds`, `Choice`) | **INTEGRATED (policy) / adapter routing STANDALONE-ONLY** | `Route` maps a `Decision` to `SMALL_MODEL`/`STRONG_MODEL`/`HUMAN` via `route_to_strong_model`/`require_human`, failing closed on unknown/invalid results. Adapter-local routing of the external process is not observed as SOP-wired. |
| Normalized results (SEAM-002 DTO) | `internal/decision/decision.go` (`Result`, `Status`, `ValidateResult`) + `internal/decision/command/command.go` | **INTEGRATED (validation) / adapter encode+validate STANDALONE-ONLY** | SOP owns `Request.ValidateResult` (fail-closed on unknown version/status/choice, invalid confidence, etc.). The adapter builds/parses the DTO but is standalone-only. |
| Fallbacks | `decision.NewProvider` failure / fail-closed `Route` | **UNVERIFIED** | Provider selection fails clearly rather than silently falling back (`decision.NewProvider`). No SOP-level adapter fallback path was observed because the seam is unwired. |
| Failure handling (external process) | `internal/decision/command/command.go` | **STANDALONE-ONLY** | Adapter returns typed sentinels (`ErrProviderFailure`, `ErrInvalidResult`, `ErrUnsupportedCapability`, `ErrIndeterminate`); bounds output; no shell; never treats failure as approval. This is adapter-local only. |
| Standalone adapter tests | `internal/decision/command/command_test.go` | **STANDALONE-ONLY EVIDENCE** | Exercises the adapter in isolation; explicitly **not** integration evidence. |
| Config-seam tests | `internal/config/decision_test.go`, `internal/config/config_test.go` | **INTEGRATED (config layer only)** | Prove schema/validation, not SOP-to-adapter dispatch. |

---

## 5. Distinct integrated vs standalone-only capabilities

**Integrated into SOP (this repository):**

- Deterministic decision provider and its threshold-based routing policy
  (`Route`, `Thresholds`) — `internal/decision/decision.go`.
- SOP-owned fail-closed result validation (`ValidateResult`, `ValidConfidence`) —
  `internal/decision/decision.go`.
- The `decision.provider` / `decision.command` **configuration schema and validation**
  — `internal/config/config.go`, `internal/config/decision_test.go`.

**Standalone-only (implemented, but not wired into SOP):**

- The external-process decision adapter — `internal/decision/command/command.go`
  (`command.New`, `Adapter.Decide`): stdin/stdout DTO transport, bounded output,
  no-shell invocation, typed failure handling.
- The adapter's own isolation tests — `internal/decision/command/command_test.go`.

**UNAVAILABLE:**

- Real SOP-to-adapter wiring for the `command` provider (the integration seam is not
  configured and no dispatch was verified).

---

## 6. Gaps and unknowns

- No `decision:` section with `provider: command` + `command:` argv was located in the
  audited project configuration; the project remains on the `deterministic` default.
- No SOP run-time call site that constructs the `command` adapter and routes a real
  decision through it was observed, and no run trace/log/artifact evidences such a
  dispatch.
- The `jev` provider is accepted by configuration but was not shown to be implemented;
  its behavior is unverified.
- If the audited project is external to this repository, the configuration state for
  that project is **undeterminable** from repository-readable configuration alone.
- Runtime wiring exercise is out of scope for this audit (an audit/reporting task);
  this report makes no implementation proposals.

---

## 7. Evidence index

| Reference | Path |
| --- | --- |
| Integration seam config schema + validation | `internal/config/config.go` |
| Seam parse/validation test | `internal/config/decision_test.go` |
| Config defaults test | `internal/config/config_test.go` |
| Configuration reference (`decision.provider`) | `docs/reference/CONFIGURATION.md` |
| Decision provider/routing/validation contract | `internal/decision/decision.go` |
| Standalone external-process adapter | `internal/decision/command/command.go` |
| Standalone adapter tests | `internal/decision/command/command_test.go` |
| Prior decision-integration boundary notes | `docs/reports/decision-integration/SEAM-001-current-boundary.md` |
| Parent plan (binds UNAVAILABLE) | `docs/plans/PLAN-SOP-End-to-End-Reliability.md` |
| Preflight baseline (UNAVAILABLE marking) | `docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md` |
