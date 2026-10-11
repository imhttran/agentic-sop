# ETOE-008 — Decision adapter integration or documented absence

**Status:** decision-adapter **integration** is **UNAVAILABLE** for the audited project.
This is an isolated/evidence task: the process-level decision seam
(`decision.provider: command` + `decision.command`) exists in SOP's config schema and
validation and there is now a composition-root factory (`decisionProviderFromConfig`)
that would construct the adapter — but the seam is **not configured** in the audited
project and **no verified SOP-to-adapter dispatch** was observed. Everything the
contract supports is exercised in isolation; everything it does not is labelled
UNAVAILABLE.

**Mutation statement:** the only repository artifact this task creates is this report
(`docs/reports/end-to-end-reliability/ETOE-008-adapter-evidence.md`). No orchestrator
source, no configuration, and no `.agent-sdlc/` SOP state was created or modified.

---

## 1. Scope and method

This report answers: **can SOP's decision adapter be exercised end-to-end, and where it
cannot, what is the exact absence?**

- **Integration-layer evidence** = a real SOP-to-adapter dispatch (SOP constructs the
  adapter and routes a decision through it at runtime, with a trace/log/artifact).
  **None was observed.**
- **Config-layer evidence** = the `decision.provider` / `decision.command` schema and
  validation (`internal/config/config.go`). Exercised, in isolation.
- **Policy/validation-layer evidence** = SOP-owned fail-closed `Validate` /
  `ValidateResult` / `Route` (`internal/decision/decision.go`). Exercised, in isolation.
- **Adapter-local isolation evidence** = the standalone external-process adapter
  (`internal/decision/command/command.go`) and its own tests. Isolation only; explicitly
  **not** integration evidence.

Method: read-only inspection of the decision seam and adapter package, plus isolated
contract-supported exercises (config acceptance/rejection, provider selection,
fail-closed policy validation, adapter-local failure sentinels). No attempt was made to
force an integrated dispatch that does not exist.

---

## 2. The exact integration seam

**Seam:** `decision.provider: command` plus `decision.command` (an executable argv).
When configured, SOP invokes an external, provider-neutral decision process that reads
the SEAM-002 request DTO on stdin and writes the SEAM-002 result DTO on stdout.

| Seam element | Value / meaning | Evidence |
| --- | --- | --- |
| `decision.provider` | optional; accepted values `deterministic`, `jev`, `command`; default `deterministic` | `internal/config/config.go` (`DecisionConfig`, `Validate`) |
| `decision.command` | executable argv; required when `provider: command` | `internal/config/config.go` |
| Validation errors | `config: unknown decision.provider %q (want deterministic, jev, command)`; `config: decision.provider "command" requires decision.command` | `internal/config/config.go` |
| Provider selection | `deterministic`/`""` -> in-process provider; `command` -> process adapter; any other name fails clearly | `internal/decision/decision.go` (`NewProvider`), `internal/cli/decision_provider.go` |
| Composition root | `newDecisionProvider: decisionProviderFromConfig` | `internal/cli/cli.go:177` |
| Adapter constructor + transport | `command.New`, `Adapter.Decide` | `internal/decision/command/command.go` |
| SOP-owned validation/routing | `Validate`, `ValidateResult`, `Route`, `ValidConfidence` | `internal/decision/decision.go` |

**Configured in the audited project?** **No.** There is no `decision:` section with
`provider: command` and a `command:` argv in the repository-readable project
configuration; the effective default is `deterministic`. The capability is additionally
gated by `decision.enabled`, which defaults to false (`config.Default()`), so the default
configuration constructs **no** provider at all (`internal/cli/decision_provider_test.go`,
`TestDecisionProviderOffByDefault`).

---

## 3. Integration status — UNAVAILABLE

Decision-adapter integration is **UNAVAILABLE**, because:

1. The seam `decision.provider: command` + `decision.command` is **not configured** in
   the audited project (default `deterministic`, `decision.enabled: false`), and
2. **No verified SOP-to-adapter dispatch** was observed — no run trace, log, or artifact
   shows SOP constructing the adapter and routing a real decision end-to-end.

The composition-root factory `decisionProviderFromConfig` (`internal/cli/decision_provider.go`,
wired at `internal/cli/cli.go:177`) means SOP *can* construct the adapter when the seam is
configured, but a constructible factory is **configuration-selection evidence, not
dispatch evidence**. It does not lift the UNAVAILABLE marking on its own.

**Criterion to lift UNAVAILABLE** (both must hold):

- `decision.provider: command` **and** `decision.command` are **configured** in the
  audited project, **and**
- a **verified SOP-to-adapter dispatch** exists, evidenced by a run trace/log/artifact.

**No isolation result in this report is presented as integration evidence.**

### 3.1 Discrepancy note versus ETOE-004

ETOE-004 §4 states "no SOP call site constructs it" for the external-process adapter
(`docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md`). In the
current working tree this is no longer literally true: the composition root now wires a
factory (`internal/cli/cli.go:177` -> `internal/cli/decision_provider.go`), and
`internal/cli/decision_provider_test.go` proves it constructs the command adapter when
configured. The **integration status is unchanged** (UNAVAILABLE) because the seam is
still unconfigured and no dispatch was verified, but the *reason* narrows from "no call
site" to "call site exists, seam unconfigured, no verified dispatch." This is recorded as
a finding rather than smoothed over.

---

## 4. Config-layer exercise (isolated)

Contract-supported only. Exercised by SOP's own config and provider-selection code;
labels are config-layer, **not** dispatch.

| Exercise | Expected | Evidence |
| --- | --- | --- |
| `provider: command` with a `command:` argv parses/validates | accepted | `internal/config/decision_test.go`, `internal/config/config.go` (`Validate`) |
| `provider: command` without argv | rejected: `config: decision.provider "command" requires decision.command` | `internal/config/config.go`, `internal/config/decision_test.go` |
| unknown provider | rejected: `config: unknown decision.provider %q (want deterministic, jev, command)` | `internal/config/config.go` |
| `decision.NewProvider("")` / `("deterministic")` | deterministic provider | `internal/decision/decision.go` |
| `decisionProviderFromConfig` with `command`+argv | returns the process adapter (no authority granted) | `internal/cli/decision_provider.go`, `internal/cli/decision_provider_test.go` |
| unknown provider name via factory | fails clearly; returns `(nil, error)`, never a silent fallback | `internal/cli/decision_provider_test.go` (`TestDecisionProviderFactoryFailsClosed`) |
| default config | constructs **no** provider | `internal/cli/decision_provider_test.go` (`TestDecisionProviderOffByDefault`) |

---

## 5. Policy-override and result-validation exercise (isolated)

**Acceptance:** *Decision model outputs cannot override SOP policy; any attempt to do so
is reported, not accepted.*

SOP owns interpretation: a provider returns bounded data, and `Validate` /
`ValidateResult` / `Route` decide what it means. `Route` fails closed — an unknown choice
or an invalid confidence is escalated to `HUMAN`, never converted into an approval. Only a
known choice with a finite confidence in `[0,1]` may reach a model tier.

Attempted overrides and SOP's response (all rejected / failed closed by SOP-owned code):

| Attempted override | SOP response | Evidence |
| --- | --- | --- |
| unknown `choice` (`"SKYNET"`, `""`) | `ErrInvalidResult: unknown choice`; `Route` -> `HUMAN` | `internal/decision/decision.go` (`Validate`, `Route`, `Choice.Known`) |
| unknown `contract_version` | `ErrInvalidResult: unsupported contract version` | `internal/decision/decision.go` (`ValidateResult`) |
| unknown `status` | `ErrInvalidResult: unknown status` | `internal/decision/decision.go` (`ValidateResult`, `Status.Valid`) |
| invalid `confidence` (NaN / +/-Inf / <0 / >1) | `ErrInvalidResult`; `Route` -> `HUMAN` | `internal/decision/decision.go` (`ValidConfidence`, `Route`) |
| `status: OK` carrying an error | `ErrInvalidResult: OK result carries an error` | `internal/decision/decision.go` (`ValidateResult`) |
| `status: UNSUPPORTED` | `ErrUnsupportedCapability` | `internal/decision/decision.go` |
| `status: ERROR` | `ErrProviderResult` | `internal/decision/decision.go` |
| `status: INDETERMINATE` / missing confidence | `ErrIndeterminate` (never a pass) | `internal/decision/decision.go` |
| choice outside the request's allowed set | `ErrInvalidResult: choice not in the allowed set` | `internal/decision/decision.go` (`Request.ValidateResult`) |
| `kind` mismatch | `ErrInvalidResult: kind does not match request` | `internal/decision/decision.go` |
| force-model-bypass via low confidence / HIGH choice | `Route` -> `HUMAN` (human gating not bypassable by provider output) | `internal/decision/decision.go` (`Route`) |

No case was found where SOP does not fail closed. No override was accepted; every attempt
above is **reported, not accepted**. At least one explicit not-accepted attempt: a provider
returning `choice: "SKYNET"` is rejected by `Validate`/`ValidateResult` and, in `Route`,
escalates to `HUMAN` rather than any model tier.

Deterministic results produced by `DeterministicProvider` are shown to pass through the
same SOP-owned `Validate`/`Route` path (`internal/cli/decision_provider_test.go`,
`TestDecisionProviderDeterministic`), and the cross-module fake provider fixture
(`testdata/fake-decision-provider/`) is validated by the same SOP-owned code via
`internal/cli/decision_evidence_crossmodule_test.go` — provider results never bypass
validation.

---

## 6. Provider-failure and fallback exercise (contract-supported paths only)

**Acceptance:** *Provider failure and fallback behavior is tested only where the contract
supports it, and unsupported paths are labelled UNAVAILABLE.*

**Contract-supported (adapter-local, `internal/decision/command/command.go`):**

| Induced failure | Typed sentinel / behaviour | Evidence |
| --- | --- | --- |
| external process missing / non-zero exit | `ErrProviderFailure` | `internal/decision/command/command.go` (`Decide`, `cmd.Run` error branch) |
| empty stdout | `ErrProviderFailure: returned empty output` | `internal/decision/command/command.go` |
| malformed / non-JSON stdout | `ErrInvalidResult: returned malformed output` | `internal/decision/command/command.go` |
| output larger than `maxOutputBytes` (1 MiB) | `ErrProviderFailure` (bounded, fail closed; never a truncated result) | `internal/decision/command/command.go` (`boundedWriter`, `maxOutputBytes`) |
| `status: UNSUPPORTED` | `ErrUnsupportedCapability` | `internal/decision/decision.go` via adapter `ValidateResult` |
| `status: INDETERMINATE` / missing confidence | `ErrIndeterminate` | `internal/decision/decision.go` via adapter `ValidateResult` |
| context cancel / timeout | `ErrProviderFailure` (never success) | `internal/decision/command/command.go` |

**A provider failure is never treated as approval or as a successful decision:** `Decide`
returns `(decision.Decision{}, error)` on every failure path above; the empty `Decision`
cannot pass `Validate` (unknown choice), and no caller can obtain a choice from an error
return. The adapter returns an error, never a defaulted/approving decision.

**UNAVAILABLE (contract does not support simulating these):**

- **SOP-level adapter fallback** — there is no wired fallback from a failing `command`
  provider to another provider, because the seam is unwired; reported UNAVAILABLE, not
  simulated or asserted.
- **Live external model/provider runtime** (ollama / remote model service) — UNAVAILABLE;
  no configured live provider is evidenced (default `deterministic`).
- **Runtime integrated dispatch trace** — UNAVAILABLE (see §3); cannot be produced by an
  isolation exercise.

Provider *selection* itself fails clearly rather than silently falling back:
`decision.NewProvider` and `decisionProviderFromConfig` return an error for an unknown or
misconfigured provider (`internal/decision/decision.go`, `internal/cli/decision_provider_test.go`).

---

## 7. Decision traces and model-tier distribution

**Decision traces:** no per-decision trace artifact is evidenced in the repository. The
run artifacts that do exist are `.agent-sdlc/runs/<id>/` (`state.json`, `validation.json`,
`report.json`, `approval.json`) as recorded by ETOE-006, but none of them carries a
decision-choice trace. **Decision traces are therefore UNAVAILABLE** beyond what is
readable from those general run artifacts; nothing is fabricated.

**Model-tier distribution:** **UNAVAILABLE.** The tier vocabulary exists as policy only
(`SMALL_MODEL`, `STRONG_MODEL`, `HUMAN` in `internal/decision/decision.go` via `Route`),
but no instrumented source records an actual per-decision tier distribution (ETOE-004 §4
evidence). No metric is invented.

---

## 8. Acceptance-criteria mapping

| Criterion | Status | Evidence |
| --- | --- | --- |
| Decision model outputs cannot override SOP policy; any attempt is reported, not accepted | MET (isolated) | `internal/decision/decision.go` (`Validate`, `ValidateResult`, `Route`, `ValidConfidence`); §5 catalogue |
| Missing runtime wiring is reported as UNAVAILABLE rather than assumed present | MET | §2–§3; `internal/cli/cli.go:177`, `internal/cli/decision_provider.go`; ETOE-004 §3.1 |
| Provider failure and fallback tested only where the contract supports it; unsupported paths labelled UNAVAILABLE | MET | §6; `internal/decision/command/command.go`; `internal/decision/decision.go` |

---

## 9. Evidence index (by path)

| Path | Role in this report |
| --- | --- |
| `internal/config/config.go` | decision seam schema + validation; defaults |
| `internal/config/decision_test.go` | seam parse/validation test |
| `internal/decision/decision.go` | SOP-owned policy: `Validate`, `ValidateResult`, `Route`, `NewProvider`, `DeterministicProvider` |
| `internal/decision/command/command.go` | standalone external-process adapter; typed sentinels |
| `internal/decision/command/command_test.go` | adapter-local isolation tests (not integration) |
| `internal/cli/decision_provider.go` | composition-root factory for the decision provider |
| `internal/cli/cli.go` | factory wiring (`newDecisionProvider: decisionProviderFromConfig`, line 177) |
| `internal/cli/decision_provider_test.go` | factory off-by-default, deterministic/command selection, fail-closed |
| `internal/cli/decision_evidence_crossmodule_test.go` | cross-module fake-provider validation through SOP-owned code |
| `testdata/fake-decision-provider/` | cross-module fake decision provider fixture |
| `internal/archtest/decision_seam_test.go` | seam independence guard |
| `docs/reports/end-to-end-reliability/ETOE-004-adapter-integration-matrix.md` | prior integration matrix / terminology |
| `docs/reports/end-to-end-reliability/ETOE-001-preflight-baseline.md` | preflight UNAVAILABLE marking |
| `docs/reports/end-to-end-reliability/ETOE-006-e2e-evidence.md` | run-artifact locations |

---

## 10. Verification and consistency note (ETOE-008F)

- **Capability/status claims** were re-checked against the repository paths in §9. The
  one discrepancy found (a composition-root factory now exists, contradicting ETOE-004's
  "no call site" wording) is reported in §3.1; integration remains UNAVAILABLE.
- **Isolated contract-supported exercises** (config acceptance/rejection, provider
  selection, fail-closed policy validation, adapter-local sentinels) are reproducible via
  the packages' own tests: `internal/config`, `internal/decision`,
  `internal/decision/command`, `internal/cli`.
- **Label consistency** with ETOE-004/ETOE-001 is confirmed for the UNAVAILABLE marking;
  the sole divergence is the call-site existence noted in §3.1.
- **Required validation commands** for this task were executed in the working tree:
  `go build ./...`, `go test ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`. SOP's
  configured gate commands were not changed.
- No repository source, configuration, or SOP state was modified by this stage.
