# JEV Decision Contract Alignment

**Status:** PROPOSED / NOT APPROVED — Gate 3 pending.

**Task:** JEV-202 (Decision Contract Alignment).

**Decision owner:** the operator-owned approval gate (Gate 3 of
PLAN-JEV-Wave-2-Hardening), not this document.

**What this document is:** an architecture decision *record of the options*, a
comparison of the two existing provider-neutral decision contracts, and the
compatibility mapping that any future choice must satisfy. It exists so the Gate
3 choice is made against written constraints rather than ad hoc.

**What this document is not:** it does **not** select, adopt, supersede, or
implement a contract. Nothing here is authoritative until Gate 3 accepts a
specific contract. JEV-202's only automated change is the non-breaking
configuration rejection described in §8.

> **PROPOSED / NOT APPROVED.** Reading no further: no contract choice has been
> made. The authority question in §7 is open, and any breaking or shared-contract
> adoption is explicitly Gate 3 work, deferred and not performed here.

## 1. Context

JEV is an advisory analysis component that can express bounded decisions. The
Wave-2 plan (PLAN-JEV-Wave-2-Hardening) requires that JEV decision
classification be delegated to the existing decision-adapter contract rather
than becoming a new JEV decision framework. Two overlapping provider-neutral
contracts already exist in the repository, and a single authoritative contract
has not been chosen:

- **SEAM-002 — provider-neutral decision contract.** Defined in
  docs/reports/decision-integration/SEAM-002-decision-contract.md and
  implemented as in-process Go types in `internal/decision` (`Request`,
  `Result`, `Decision`, `Choice`, `Status`, `ContractVersion`) with a
  provider-neutral process adapter in `internal/decision/command`.
- **Decision-adapter contract.** The external-facing adapter obligations in
  docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md, realized over the
  same SEAM-002 JSON DTO (a process reads the request DTO on stdin and writes the
  result DTO on stdout).

They are the **same semantic seam at two layers**: `internal/decision` is the
SOP-side Go surface; the decision-adapter contract is the wire/DTO surface and
adapter obligations that external adapters (for example the candidate Julia-1
and Nimble adapters validated in `sop-decision-adapters`) must satisfy. They
differ mainly in carrier (Go types vs JSON), not in intent.

## 2. Purpose and non-goals

**Purpose.** Record the comparison, the compatibility mapping for
choice/boolean/score/confidence/probability, the error and versioning rules, and
the authority question, so Gate 3 can choose one authoritative contract.

**Non-goals (explicit).**

- Do not select or adopt a contract; the choice is Gate 3.
- Do not add a JEV decision provider.
- Do not add an adapter bridge between the two contracts.
- Do not make any breaking contract or API change.
- Do not weaken deterministic approval gates or the human boundary.

## 3. The two contracts compared

Both contracts carry **evidence only**. Neither grants lifecycle, approval,
commit, merge, scheduler, or policy authority; SOP owns validation and
interpretation.

| Aspect | SEAM-002 (`internal/decision`) | Decision-adapter contract (AS-CLEF-008 + `internal/decision/command`) |
|---|---|---|
| Carrier | In-process Go types | JSON DTO over an external process (stdin/stdout) |
| Version signal | `decision.ContractVersion = 1`; `Result.ContractVersion` | `contract_version` in request and result JSON |
| Request shape | `Request{UseCase, Subject, Signals, Choices}` | `kind`, `question`, `signals`, `choices`, `deadline_ms` |
| Choice vocabulary | closed `Choice`: `LOW`, `MEDIUM`, `HIGH`, `HUMAN` | same closed vocabulary via `choice` |
| Confidence | `Decision.Confidence float64`; `Result.Confidence *float64` | `confidence` number in `[0,1]`; absent = indeterminate |
| Status | `Status`: `OK`, `UNSUPPORTED`, `ERROR`, `INDETERMINATE` | `status` string, same vocabulary |
| Diagnostics | `Result.Diagnostics map[string]string` (advisory) | `diagnostics` map (advisory) |
| Error channel | typed Go sentinels (see §5) | typed Go error at the seam, plus in-result `status` |
| Selection | `decision.NewProvider(name)`; `""`/`deterministic` implemented | `config.Decision.Provider` + `decision.command` argv; name is opaque |
| Validation owner | SOP (`Request.ValidateResult`) | SOP (`Request.ValidateResult`), never the provider |
| Authority | none; `Route` fails closed to a human | none; SOP policy is the only action producer |

## 4. Compatibility mapping: choice / boolean / score / confidence / probability

External adapters may natively express an outcome in different shapes. Whatever
Gate 3 adopts, every adapter-native shape must reduce to a canonical
`(Choice, optional Confidence)` pair, and must **fail closed** when it cannot be
mapped. The rows below are **proposed constraints, not an adopted mapping**.

| Adapter-native shape | Canonical SEAM-002 element | Proposed treatment (NOT adopted) | Constraint if adopted |
|---|---|---|---|
| enumerated `choice` | `Choice` (`LOW`/`MEDIUM`/`HIGH`/`HUMAN`) | map verbatim | value must be a member of the closed set; unknown choice fails closed |
| `boolean` (e.g. true/false, "needs attention"/"safe") | `Choice` | map through a documented, versioned, capability-specific table | a bare boolean is **not** a `Choice`; with no mapping it is not evidence and fails closed (never silently an approval) |
| numeric `score` (arbitrary range) | `Confidence` | normalize by a versioned, capability-specific rule before use | a raw score is **not** a probability; an un-normalized or out-of-range score is invalid |
| `confidence` | `Confidence` | map directly | finite value in `[0,1]`; `NaN`/`Inf`/out-of-range invalid |
| `probability` | `Confidence` | map directly when it is the probability of the chosen outcome | a probability of a *different* event is not confidence unless a mapping says so |

Two invariants must hold for any adopted mapping:

1. **No shape becomes authority.** A boolean or a score never routes as an
   approval merely because it is "positive"; it routes only after a documented
   mapping produces a known `Choice` and (optionally) a valid `Confidence`.
2. **Unmappable is fail-closed.** If a shape cannot be mapped for a capability,
   the result is indeterminate and policy fails closed to a human.

The candidate Julia-1 and Nimble adapters (validated separately in
`sop-decision-adapters`, JEV-206) exercise these shapes; the mapping decision is
the input JEV-206 needs from Gate 3.

## 5. Error model

Two deliberately distinct channels, matching SEAM-002 §7:

- **In-result failures** the provider observes: `status` of `UNSUPPORTED`,
  `ERROR`, or `INDETERMINATE` (plus `error`).
- **Transport/process failures** that produce no usable result: a typed Go error
  at the SOP seam.

| Situation | SEAM-002 / Go sentinel | Adapter DTO | Never becomes |
|---|---|---|---|
| No provider configured | none (strict no-op) | no request, no result | an error, a default provider, an approval |
| Unsupported capability | `ErrUnsupportedCapability` | `status=UNSUPPORTED` + `error` | a low-confidence guess |
| Provider-observed error | `ErrProviderResult` | `status=ERROR` + `error` | a success |
| Indeterminate / missing confidence | `ErrIndeterminate` | `status=INDETERMINATE` or `OK` with no confidence | certainty |
| Malformed output, wrong version, unknown status/choice, out-of-range confidence | `ErrInvalidResult` | (validation, no valid DTO) | a pass or approval |
| Transport failure, non-zero exit, empty output, output overflow, timeout, cancellation | `ErrProviderFailure` | no usable result | a success |

Rules that hold in every case: a provider failure or an unknown/indeterminate
result is **never** translated into authorization, approval, or successful
execution; unknown or uninterpretable results fail closed to a human
(`decision.Route`); and disabling the capability restores current deterministic
behavior exactly.

## 6. Versioning rules

The minimum bar for any adopted contract:

- `contract_version` is required in both request and result; a mismatch is a
  failure (`ErrInvalidResult`).
- **Additive fields are compatible** (new `signals`, `diagnostics`, or
  capability-specific optional fields). Removing or re-meaning an existing field
  is not, and requires a version bump.
- The choice vocabulary (`LOW`/`MEDIUM`/`HIGH`/`HUMAN`) and the status vocabulary
  (`OK`/`UNSUPPORTED`/`ERROR`/`INDETERMINATE`) are fixed per version; unknown
  values fail closed.
- The provider name is the selection key: it must be stable, or its change is a
  version boundary.
- Compatibility is preserved in the safe direction: an uninterpretable
  older/newer provider fails closed, never to approval.
- Compatibility is defined at the semantic level only; no transport, encoding, or
  provider wire protocol is part of the compatibility surface.

Adopting a shared or superset contract, or making any breaking change, is a
version boundary and is **Gate 3 work**, not performed by JEV-202.

## 7. The authority question

> Providers evaluate. SOP governs.

The authority question Gate 3 must answer is **which contract text and version
is authoritative and who owns its evolution** — not whether authority moves to
providers. Neither contract carries authority: `internal/decision` exposes no
method or field for task/plan mutation, approval, commit, merge, policy change,
or lifecycle transition, and the adapter DTO carries only a bounded question,
numeric signals, allowed choices, and a bounded domain result. The deterministic
SOP policy (`internal/autonomy`; `decision.Route`; the human boundary) retains
final authority; decision evidence may add **attention** and route to a strong
model or a human, but it can never auto-approve or bypass the human boundary.

Candidate authority models (**PROPOSED; none chosen**):

- **Option A — SEAM-002 (`internal/decision`) is the single canonical
  contract.** Adapters translate native shapes (boolean/score/probability) into
  it. *Authority:* SOP owns the one text and its version. *Compatibility:*
  adapters conform to SEAM-002; the mapping in §4 becomes SOP-owned. *Risk:* the
  Go-shaped vocabulary must be expressive enough for adapter-native shapes.
- **Option B — a versioned superset contract, with SEAM-002 as a conforming
  profile.** *Authority:* a shared text co-owned across the seam and adapter
  docs. *Compatibility:* needs an explicit profile/version relationship and a
  change process. *Risk:* two texts can drift; ownership must be unambiguous.
- **Option C — keep both contracts as-is, with a documented crosswalk.** *Authority:*
  split; the crosswalk is the only binding text. *Compatibility:* lowest change
  cost, but no single authoritative source. *Risk:* the crosswalk itself becomes
  the de-facto contract and can drift.

This ADR does not select among A, B, or C.

## 8. What JEV-202 changes (non-breaking)

The only automated change is configuration validation, which is backward-safe:

- `internal/config` now accepts only `deterministic` and `command` for
  `decision.provider`; the legacy `jev` value and any invented value are rejected
  through the existing default branch (`internal/config/config.go` `Validate`).
- The `Provider` field doc comment and the surrounding `DecisionConfig` comment
  were updated to match the accepted set.
- The existing test `TestDecisionProviderJevDoesNotEnableJEV` was updated to
  reflect rejection while preserving its intent (proving `provider: jev` cannot
  enable JEV), and `TestDecisionProviderRejectsUnsupported` was added.
- docs/reference/CONFIGURATION.md was reconciled so `decision.provider` is
  documented as `deterministic` (default) or `command`.

Explicitly **not** done here: no contract selected or adopted; no adapter bridge;
no new JEV decision provider; no breaking contract or API change; no change to
deterministic approval gates or the human boundary.

## 9. Open questions deferred to Gate 3

- Which contract is authoritative: A, B, or C in §7?
- Does the adopted contract need a version bump beyond `contract_version = 1`?
- Is the §4 boolean/score/probability mapping adopted as written, and who owns
  it?
- Does adopting a shared contract require any change to `internal/decision`, the
  adapter DTO, or `config.DecisionConfig`? (Any such change is breaking and is
  out of JEV-202.)

## 10. References

- docs/reports/decision-integration/SEAM-002-decision-contract.md
- docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md
- internal/decision/decision.go
- internal/decision/command/command.go
- internal/cli/decision_provider.go
- docs/plans/PLAN-JEV-Wave-2-Hardening.md
