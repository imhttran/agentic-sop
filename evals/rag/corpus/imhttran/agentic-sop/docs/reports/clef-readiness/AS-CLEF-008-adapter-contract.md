# AS-CLEF-008 — Define the Adapter-Side Contract

Status: FINAL — contract complete (documentation-only). This report is the
AS-CLEF-008 deliverable and the only repository change of this task. This note is
the handoff to the separate Clef adapter plan. It defines the exact contract that
**sop-decision-adapters** must satisfy. No Clef adapter is implemented in
agentic-sop, and sop-decision-adapters is not modified by this task.

## 0. Scope, ownership, and out-of-scope statement

This document is a **contract**, not an implementation. It records the
obligations a decision adapter must satisfy when it is built in the external
`sop-decision-adapters` project. It is provider-neutral: it names no provider,
no transport, no wire encoding, and no vendor protocol. Where the contract
refers to a decision adapter it means "any component that implements the SOP
decision boundary", regardless of whether it is rules, a model, or a remote
service.

Ownership split:

- **agentic-sop** owns the decision boundary types and the SOP policy that
  interprets decisions (`internal/decision`, `internal/autonomy`). It is the
  source of truth for the terms in this contract.
- **sop-decision-adapters** (external project) owns the adapter implementation
  that produces decisions satisfying this contract. It is **not modified** here
  and no adapter is implemented in agentic-sop.
- **AS-CLEF-008** (this document) is the handoff to the separate Clef adapter
  plan. It defines obligations; it does not create or wire an adapter.

Evidence baseline: this contract is derived from the in-tree decision boundary
and policy code located during discovery:

| Surface | Location | Role |
|---|---|---|
| Bounded decision boundary | `internal/decision/decision.go` | `Request`, `Decision`, `Choice`, `Provider`, `NewProvider`, `DeterministicProvider`, `Thresholds`, `Route`, `Target`, `validConfidence`, `knownChoice` |
| Live SOP policy engine | `internal/autonomy/autonomy.go` | `Decide`, `Decision`, `Action`, `Level`, `ApprovalRisk`, `Policy`, `humanDecision`, `boundaryRisk`, `riskOf` |
| Early-evidence policy bridge | `internal/autonomy/early.go` | `DecideEarly`, `earlyBlockingFinding`, `kindForEarlyCategory` |
| Evidence input | `internal/jev` (`jev.Evidence`, `jev.EvidenceItem`) | typed evidence consumed by `DecideEarly` |
| Live call sites | `internal/cli/early_jev.go:184`, `internal/cli/autonomy.go:22`, `internal/cli/reconcile.go:126` | policy consumers |

Status of the boundary as found: **PARTIAL (sufficient, unwired)**. `internal/decision`
defines the provider interface and result shape but has **no production caller** — its
only in-tree consumers are tests (`internal/e2e/lifecycle/early_decision_test.go`).
The live policy path is `internal/autonomy`'s pure functions. AS-CLEF-003/004
confirmed this is a provider-neutral boundary with **no material abstraction gap**
(AS-CLEF-004 **NOT_REQUIRED**), and AS-CLEF-005/007 supplied the fail-closed
behavioral suite and the architecture guard consumed below. Sections mark which
contract terms are backed by existing code (the `internal/decision` types and
`internal/autonomy` policy) and which are adapter-side obligations not yet realized
in agentic-sop.

## 1. Request fields

The adapter receives a bounded request. The authoritative request shape is
`decision.Request` (`internal/decision/decision.go`):

| Field | Type | Meaning | Adapter obligation |
|---|---|---|---|
| `UseCase` | `string` | The class of decision being requested. | Treat as an opaque, provider-neutral label. Do not require a provider-specific taxonomy. |
| `Subject` | `string` | Free text describing what is being decided. | May be read as signal; must not be the sole basis of a safety outcome. |
| `Signals` | `map[string]float64` | Numeric inputs (for example change size, file count). | Treat absent keys as zero/unknown; never invent a signal. |

Request contract rules:

- The request is **bounded context**, not an execution instruction. An adapter
  must not execute actions from a request (package rule: "Providers must never
  execute actions.", `internal/decision/decision.go`).
- An adapter must tolerate unknown/extra signal keys and absent keys without
  failing the whole request; a missing signal is unknown, not zero-confidence
  approval.
- The adapter must not require provider-specific fields outside this shape.
  Any provider-specific input is the adapter's private concern and must not
  become a required part of the SOP-visible request.

## 2. Result fields

The adapter returns a bounded decision. The authoritative result shape is
`decision.Decision` (`internal/decision/decision.go`):

| Field | Type | Meaning | Adapter obligation |
|---|---|---|---|
| `Choice` | `Choice` (`string`) | The decision outcome. | Must be one of the known choices (§3) or an explicitly unknown value; never a free-form approval. |
| `Confidence` | `float64` | Probability/confidence in `[0,1]`. | Must be a valid probability (§4); invalid values are treated as unusable and fail closed. |
| `Metadata` | `map[string]string` | Optional audit metadata. | Advisory only; never a decision input to SOP policy (§9). |

Result contract rules:

- A result is **evidence for policy**, not an action. SOP policy converts the
  result into a lifecycle action; the adapter does not.
- An adapter must return a well-formed result on success and an error on failure
  (§8); it must not encode failures as a suspicious success (`Choice`/`Confidence`
  that would be routed as an approval).
- `Metadata` must not be relied upon by policy; it exists for auditability.

## 3. Decision types / capabilities

The decision **capability** is the bounded evaluate-and-return-evidence
contract. Its authoritative interface is `decision.Provider`
(`internal/decision/decision.go`):

```go
type Provider interface {
    Name() string
    Decide(ctx context.Context, req Request) (Decision, error)
}
```

Obligations:

- `Name()` returns a stable, lower-case identifier for the adapter. Naming is
  how a provider is selected; it must not change meaning between releases
  without a version signal (§10).
- `Decide` is pure with respect to side effects: it evaluates the bounded
  request and returns a bounded result. It must not execute actions, transition
  lifecycle state, mutate persistence, or start work.
- The capability is **one decision per request**. An adapter that needs multiple
  internal steps may do so privately, but the SOP-visible contract is a single
  result.

The known choice vocabulary (`Choice`, `internal/decision/decision.go`) is:

| Choice | Meaning |
|---|---|
| `LOW` | Low-complexity / low-risk outcome. |
| `MEDIUM` | Moderate outcome. |
| `HIGH` | High-complexity / high-risk outcome. |
| `HUMAN` | Explicit human-boundary request. |

An **unknown** choice (any value outside this set) is a valid input to the
boundary but is never treated as approval; policy fails closed (§6, §9).

Supported capabilities are declared, not implied: an adapter that cannot serve
a requested `UseCase` must say so explicitly via the unsupported-capability
behavior in §7 rather than returning a low-confidence guess.

## 4. Probability / confidence semantics

Confidence is a probability in `[0,1]` (`decision.Decision.Confidence`;
`validConfidence` in `internal/decision/decision.go`).

Semantics:

- `0.0` means no confidence; `1.0` means full confidence. Values between are
  the adapter's estimate of the choice's correctness.
- A valid confidence is finite and within `[0,1]`. `NaN`, `±Inf`, negative
  values, and values above `1` are **invalid** and must never be treated as a
  confident authorization (`validConfidence`, `internal/decision/decision.go`).
- Confidence is consumed only through thresholds and fail-closed routing (§6);
  it never authorizes execution on its own. The boundary rule is explicit:
  "never authorizes execution from a provider's confidence alone"
  (`Route`, `internal/decision/decision.go`).
- The existing deterministic provider emits fixed confidences (`0.9`/`0.8`/`0.9`)
  — a concrete example that confidence need not be continuous or calibrated for
  the contract to hold. An adapter is not required to be calibrated, but it
  must not represent an uncalibrated number as certainty: a confident value for
  an uncertain decision is a contract violation in spirit (§9).

Thresholds (`decision.Thresholds`, `internal/decision/decision.go`) are **policy
configuration**, supplied outside the adapter via `route_to_strong_model` and
`require_human`. The adapter does not choose or read thresholds; it only emits a
confidence that policy thresholds interpret.

## 5. Diagnostics / evidence semantics

An adapter must be able to explain its result for auditability without that
explanation influencing policy.

- The SOP-visible diagnostic slot on the decision result is
  `Decision.Metadata map[string]string` (auditability only). The deterministic
  provider populates `Metadata["reason"]` with a short human-readable reason
  (`internal/decision/decision.go`).
- **Diagnostics are advisory.** No policy decision may be derived from
  `Metadata` text. This mirrors the repository-wide rule that evidence is typed
  and prose is provenance, codified in `internal/autonomy/early.go`: the early
  path performs "no string matching" on prose, and an item's detail is appended
  to the reason only **after** the action, risk, and human boundary are fully
  determined by typed fields.
- An adapter's evidence must be **structured and bounded**: it must not return
  free-form model prose in place of typed fields, and it must never depend on
  downstream string matching for a safety outcome.
- Evidence that an adapter cannot produce must be omitted rather than fabricated.
  Absent diagnostics are acceptable; invented evidence is a contract violation.

## 6. Timeout / cancellation expectations

The provider contract takes a `context.Context`: `Decide(ctx context.Context,
req Request) (Decision, error)` (`internal/decision/decision.go`).

Obligations:

- An adapter **must honor context cancellation and deadlines**. When `ctx` is
  done, `Decide` must return promptly with an error (§8); it must not block
  indefinitely or ignore cancellation.
- An adapter must not treat a cancelled/timed-out decision as a success. A
  deadline is not evidence; it must surface as an error, and policy fails closed
  (§9).
- An adapter must not require an unbounded amount of work to produce a decision.
  Where an adapter needs a timeout of its own, that timeout is adapter-private
  and must still respect the caller's `ctx` as the upper bound.

Status note (boundary as found): the current in-tree decision path inspects no
`context.WithTimeout` at the policy/decision boundary (AS-CLEF-002 §2.7). This
section therefore states the **adapter-side obligation** required by the
contract; it does not claim that timeout enforcement already exists in
agentic-sop. A timeout cannot become implicit success under this contract.

## 7. Error categories

Errors are the adapter's failure channel, distinct from a decision result. They
are surfaced as `error` from `Provider.Decide` and from provider construction
(`decision.NewProvider`, `internal/decision/decision.go`).

The contract recognizes these adapter-side error categories:

| Category | Meaning | Required behavior |
|---|---|---|
| **Unsupported capability** | The adapter cannot serve the requested `UseCase`/capability. | Return an explicit unsupported error; never guess. |
| **Invalid/indeterminate result** | The adapter cannot produce a valid choice or a valid `[0,1]` confidence. | Return an error, or return an explicitly unknown choice; never an approval. |
| **Cancellation / timeout** | `ctx` was cancelled or its deadline exceeded. | Return promptly with an error; never a success. |
| **Internal failure** | The adapter's own machinery failed (I/O, dependency, malformed upstream data). | Return an error; never a false success. |

Rules common to all categories:

- **Errors are never findings and never approvals.** The live seam's rule is
  that a provider error is never treated as a finding (`internal/cli/early_jev.go`);
  the same holds for the adapter contract.
- **No silent fallback inside the adapter.** The construction boundary fails
  clearly rather than silently falling back: an unsupported provider name
  returns an error (`decision.NewProvider`: "fails clearly rather than silently
  falling back", `internal/decision/decision.go`).
- **Fail closed at policy.** Any adapter error or unusable result drives SOP
  policy to its fail-closed behavior (§9), never to an automated approval.

## 8. Unsupported-capability behavior

This is the explicit adapter-side obligation for capability gaps. There is no
in-tree implementation of it in agentic-sop for adapters; the closest existing
analogue is the not-implemented provider construction path.

- An adapter that is asked for a capability it does not support **must return an
  explicit error** (the unsupported category in §7). It must not return a
  low-confidence `Choice` as a stand-in.
- Provider selection is by name and is **strict**: `decision.NewProvider` maps
  only `""`/`"deterministic"` to an implementation and returns a hard error for
  every other name (including `"jev"`, which is "recognized but not
  implemented") rather than silently substituting the deterministic provider
  (`internal/decision/decision.go`). An adapter registry must preserve this
  property: an unavailable adapter is an error, not a silent fallback to a
  different adapter.
- An unsupported capability must be distinguishable at the boundary from a
  supported decision: operators must be able to tell "no adapter can answer
  this" from "an adapter answered LOW".
- **Construction and transport are adapter-owned.** `decision.NewProvider` is the
  in-tree selector; it implements only the deterministic provider. The contract is
  semantic: an external adapter satisfies it by producing a bounded `Decision`
  through whatever provider-neutral construction/transport seam agentic-sop later
  exposes. The contract does **not** require agentic-sop to import, link, or name
  the adapter, and the adapter's wire encoding is not part of it (§10, §11).

## 9. What SOP policy consumes and what it deliberately ignores

### 9.1 What SOP policy consumes

SOP policy consumes **typed decision fields only**. Through
`decision.Route` (`internal/decision/decision.go`) policy consumes:

- `Decision.Choice` — routed as: `HUMAN` or `HIGH` → human; `MEDIUM` → strong
  model; otherwise small model.
- `Decision.Confidence` — compared against `Thresholds.RequireHuman` and
  `Thresholds.RouteToStrongModel`.

The live policy engine (`internal/autonomy`) consumes typed evidence/classification
fields — `failure.Classification` (`Kind`, `Disposition`, `Confidence`, `Reason`)
and, via `DecideEarly`, the typed `jev.Evidence` fields `Purpose`, `Status`, and
per-item `Severity`/`Category` (`internal/autonomy/early.go`). It then produces an
action from the vocabulary `AUTO_CONTINUE`, `AUTO_RETRY`, `AUTO_FIX`,
`AUTO_RECONCILE`, `TERMINAL`, `HUMAN_APPROVAL_REQUIRED` (`internal/autonomy/autonomy.go`).
That policy output is the *only* thing that permits an action; the adapter never
does.

### 9.2 What SOP policy deliberately ignores

- `Decision.Metadata` — audit-only; never a routing or safety input.
- Free-form prose (`Summary`, `Detail`, `Evidence` text) — provenance only; the
  early path performs no string matching on it (`internal/autonomy/early.go`).
- `jev.Evidence.Confidence` and other advisory confidence metadata on evidence —
  "Confidence is deliberately not a decision input here" (`DecideEarly`,
  `internal/autonomy/early.go`); evidence confidence is display-only.
- Adapter identity / vendor name — the boundary branches on typed data and
  `Policy` flags, never on provider, model, or vendor identity (AS-CLEF-003
  §1.8; `internal/autonomy`).
- `Level` as an adapter input — autonomy `Level` is config-owned policy, not an
  adapter concern.

### 9.3 Fail-closed consumption rule

Where policy cannot interpret a result — unknown `Choice`, invalid `Confidence`
— it **fails closed to a human** rather than approving
(`decision.Route`; `validConfidence`, `knownChoice` in `internal/decision/decision.go`).
An adapter cannot obtain an automated outcome by emitting an unknown or
ill-formed result.

## 10. Versioning / compatibility expectations

- **Contract version.** This document defines the adapter contract surface. An
  adapter declares which contract version it satisfies; a change to the
  SOP-visible request/result semantics (§1–§4) is a contract change and must be
  accompanied by a version signal.
- **Name stability.** `Provider.Name()` is the selection key; renaming an
  adapter is a compatibility-affecting change and must be treated as a version
  boundary (§3).
- **Additive compatibility.** Adding new `Signals` keys, new `Metadata` keys, or
  new diagnostic fields is backward-compatible. Removing or re-meaning an
  existing request/result field is not.
- **Choice vocabulary.** The known choices (`LOW`, `MEDIUM`, `HIGH`, `HUMAN`)
  are a fixed vocabulary. Adding a new choice is a contract change; an adapter
  must not invent choice values and expect them to be treated as known — unknown
  choices fail closed (§9.3).
- **Fail-closed default.** Compatibility must be preserved in the safe
  direction: when an older/newer adapter cannot be interpreted, the boundary
  must fail closed to a human, never to an approval.
- **No wire-protocol coupling.** Compatibility is defined at the semantic level
  of §1–§4 only. No transport, encoding, or provider protocol is part of the
  compatibility surface.

## 11. Handoff to the separate Clef adapter plan

This section is the explicit handoff. It states obligations; it implements
nothing.

- The adapter implementation is owned **externally** by the
  `sop-decision-adapters` project. That project is **not modified** by this task,
  and no Clef adapter is implemented in agentic-sop.
- The missing adapter capability is recorded as an **explicit external
  obligation** — a MISSING capability owned by sop-decision-adapters — not as
  existing behavior in agentic-sop.
- The separate Clef adapter plan consumes this contract and is responsible for:
  1. implementing a `Provider`-shaped adapter (`Name`/`Decide`) returning a
     bounded `Decision` (§1–§3);
  2. emitting valid `[0,1]` confidence and known choices (§3–§4);
  3. honoring context cancellation/deadlines (§6);
  4. returning explicit errors for unsupported capability and invalid results
     (§7–§8);
  5. providing advisory diagnostics only (§5).
- Provider neutrality: this contract must be satisfiable by any provider. The
  Clef adapter plan may choose its own internal transport and encoding; those are
  **not** part of this SOP-visible contract.

## 12. Acceptance-criteria mapping

| Acceptance criterion | Where satisfied |
|---|---|
| Documents request fields. | §1 (`decision.Request`: `UseCase`, `Subject`, `Signals`). |
| Documents result fields. | §2 (`decision.Decision`: `Choice`, `Confidence`, `Metadata`). |
| Documents decision types/capabilities. | §3 (`decision.Provider`; `Choice` vocabulary). |
| Documents probability/confidence semantics. | §4 (`[0,1]`, `validConfidence`, thresholds). |
| Documents diagnostics/evidence semantics. | §5 (`Metadata` advisory; typed-not-prose rule). |
| Documents timeout/cancellation expectations. | §6 (`ctx` honored; cancellation is an error). |
| Documents error categories. | §7 (unsupported, invalid, cancellation/timeout, internal). |
| Documents unsupported-capability behavior. | §8 (explicit error; strict selection; no silent fallback). |
| Documents versioning/compatibility expectations. | §10 (contract version, name stability, additive compatibility, fail-closed). |
| Documents what SOP policy consumes. | §9.1 (`Choice`, `Confidence`, typed classification/evidence). |
| Documents what SOP policy deliberately ignores. | §9.2 (`Metadata`, prose, advisory confidence, identity, `Level`). |
| Provider-neutral; no provider wire protocol. | §0 ownership/neutrality statement; §10 "No wire-protocol coupling"; every section uses SOP-boundary terms only. |
| Committed to the agentic-sop documentation location. | This file: `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md` (repository change; committed through the repo's normal workflow). |
| No Clef adapter implemented; sop-decision-adapters unmodified. | §0 and §11 (explicit external-obligation statement); no code changed by this task. |

## 13. Prior-task inputs and remaining notes

AS-CLEF-005 and AS-CLEF-007 are complete and are consumed here as inputs, not
deferred:

- **Behavioral failure/approval semantics** — proven by the AS-CLEF-005 suite
  `internal/decision/governance_test.go` (provider unavailable, timeout, malformed,
  empty, invalid confidence, unknown/indeterminate, low confidence, internal error;
  no task-state mutation). This contract's §6–§9 obligations are the adapter-side
  mirror of that verified, fail-closed behavior.
- **Architecture guard** — present as `internal/archtest/guard_test.go`
  (`TestArchitectureGuard`), which fails if a core/policy package acquires a
  forbidden provider import. It is consumed as provider-neutrality evidence (§0).

Remaining note (non-blocking; carried into AS-CLEF-010):

- **No production consumer of a non-deterministic `decision.Provider`.** Searches
  for `decision.NewProvider`/`decision.Route` over `internal` found only test hits,
  and `config.DecisionConfig.Provider` is validated but never wired to a provider
  constructor in production. The contract is therefore satisfiable by an external
  adapter, but wiring an external adapter into the live policy path is a separate,
  provider-neutral integration step — not part of this contract and not
  Clef-specific.

BLOCKED is not used: every contract section required by the PRD is defined with
cited in-tree evidence or explicitly marked as an adapter-side obligation, and the
one remaining limitation is non-blocking and named with evidence.
