# SEAM-002 — Provider-Neutral Decision Contract

Status: contract defined (documentation-only). This is the SEAM-002 deliverable and
the only repository change of this stage.

- **Repository:** `~/agentic-workspace/agentic-sop`
- **Scope:** `agentic-sop` only; `sop-decision-adapters` not touched.
- **Current HEAD:** `b743cfa93c057eb1499ce72332d6481edb4643a6`
- **Authoritative inputs:** `docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md`
  (SEAM-002) and `docs/reports/decision-integration/SEAM-001-current-boundary.md`.

This document defines the **minimum stable, provider-neutral contract** an external
decision adapter must satisfy to supply decision evidence to `agentic-sop` — without
gaining governance, policy, lifecycle, authorization, or approval authority. It
defines obligations; it implements nothing and changes no production code.

---

## 1. Purpose and scope

Define the smallest cross-module contract over which an external decision adapter can
evaluate a bounded decision and return **evidence**, so that `agentic-sop` can
validate and interpret that evidence through its own policy.

In scope: the request/result DTOs, the capability model, validation ownership,
failure semantics, cancellation/deadline ownership, the construction/configuration
boundary, the cross-module visibility decision, the autonomy-interpretation handoff,
transport independence, default/off behavior, versioning, worked examples, non-goals,
and open questions.

Out of scope: any implementation; any Clef/provider-specific payload; any change to
`internal/decision` production behavior, `internal/autonomy`, execution routing, or
the approval boundary; any decision about whether SOP reuses `autonomy.Decide` or adds
a new entry point (SEAM-004).

## 2. Governing invariant

> Providers evaluate. SOP governs.

```text
external decision provider
        │  provider-neutral result (data)
        ▼
agentic-sop boundary
        │  SOP-owned validation
        ▼
SOP-owned policy / autonomy
        │
        ├── Continue
        ├── Block
        └── Human Approval
```

Provider output is **data/evidence** until `agentic-sop` validates and interprets it.
Every lifecycle action is produced by SOP policy, never by the provider.

## 3. Dependency direction

- `external adapter → (transport) → agentic-sop seam → internal SOP types`.
- The adapter depends on **no** `agentic-sop` Go package (§11).
- `internal/decision` remains internal and stays a leaf package (SEAM-001 §5).
- `internal/autonomy` remains pure; nothing in the contract grants it new I/O.
- No dependency is introduced between decision evidence and `internal/model`
  execution routing (SEAM-001 §11).

## 4. Request contract (SOP → provider)

A single bounded request, transported as provider-neutral JSON.

| Field | Type | Required | Meaning / rule |
|---|---|---|---|
| `contract_version` | integer | yes | The contract version. An unrecognized version is a failure (fail closed). |
| `kind` | string | yes | The decision kind/capability requested. Opaque, provider-neutral label; the provider MUST NOT require a provider-specific taxonomy. |
| `question` | string | yes | Bounded free-text subject of the decision. Must not be the sole basis of a safety outcome on the SOP side; the provider treats it as signal. |
| `signals` | map<string, number> | no | Numeric inputs (for example change size, file count). An absent key is **unknown**, never zero-confidence approval. |
| `choices` | array<string> | no | When present, the allowed choice values for this request. When present, a returned `choice` MUST be a member. |
| `request_id` | string | no | Opaque correlation identifier for tracing. It carries **no** authority and MUST NOT affect policy. |
| `deadline_ms` | integer | no | Advisory hint only. SOP owns enforcement (§9); the provider must not treat its absence as permission to run unbounded. |

Request rules:

- The request is **bounded context**, not an execution instruction. It contains no
  lifecycle object, no task handle, no persistence handle, no repository mutation
  instruction, and no approval/commit/merge directive.
- The provider MUST tolerate unknown/extra request fields and absent optional fields
  without failing the whole request.

## 5. Result / evidence contract (provider → SOP)

A single bounded result, transported as provider-neutral JSON.

| Field | Type | Required | Meaning / rule |
|---|---|---|---|
| `contract_version` | integer | yes | Must match the request's version; a mismatch is a failure. |
| `status` | string | yes | One of `OK`, `UNSUPPORTED`, `ERROR`, `INDETERMINATE`. Any other value is a failure. |
| `kind` | string | no | Echo of the served kind/capability, for correlation and validation. |
| `choice` | string | conditional | The domain choice. Required when `status=OK` for a choice capability. For repository evidence, the vocabulary is the existing `decision.Choice` (`LOW`/`MEDIUM`/`HIGH`/`HUMAN`); the contract itself defines only that a choice is a member of the closed, SOP-recognized set. |
| `confidence` | number | no | Probability in `[0,1]` when the provider states one. `NaN`/`Inf`/out-of-range is invalid. **Absent** confidence means *indeterminate*, never certainty. |
| `diagnostics` | map<string, string> | no | Advisory only. SOP policy MUST NOT read it. |
| `error` | string | conditional | Bounded, human-readable text. Required when `status` is `UNSUPPORTED` or `ERROR`. |

Result rules:

- The result is **evidence**, not an action. A choice describes the domain decision
  being evaluated; it is never a governance action.
- **Distinction from a SOP policy outcome.** The contract MUST NOT carry authoritative
  values equivalent to `CONTINUE`, `BLOCK`, `APPROVE`, `REJECT`, `COMMIT`, or `MERGE`
  when those denote SOP governance actions. A provider may express a domain concept
  (for example a `HUMAN` *choice*, meaning "this looks like it needs a human"), but it
  never returns the *policy action*; SOP maps validated evidence to
  `AUTO_*`/`TERMINAL`/`HUMAN_APPROVAL_REQUIRED` itself.
- `diagnostics` is advisory; absent diagnostics are acceptable; invented evidence is a
  contract violation.
- The provider MUST NOT encode a failure as a suspicious success (a `choice`/`confidence`
  that would be interpreted as approval).

## 6. Capability representation

- The **requested capability** is named by `kind` (an opaque, provider-neutral label).
- The provider expresses **supported** capability by returning `status=OK` for that
  kind; it expresses **unsupported** capability explicitly with `status=UNSUPPORTED`.
  It MUST NOT return a low-confidence guess as a stand-in for an unsupported kind.
- Capabilities are **additive and optional**: the baseline capability is a bounded
  `choice` (with optional `confidence`). A provider may support additional
  provider-neutral operations (for example a bounded `score`, or a
  probability/confidence view) by adding fields in a version-compatible way (§16); the
  contract does not require any provider to support every capability.
- Capability advertisement (a handshake that lists a provider's supported kinds) is a
  **transport concern** left to SEAM-003; the contract requires only that an
  unsupported capability is explicit and safe, so SOP never mistakes "cannot answer"
  for "answered LOW".
- Capability is **not** a policy authority: knowing which capabilities exist never
  selects a lifecycle transition, a model class, or an approval.

## 7. Error / failure semantics

Two channels, deliberately distinct:

- **In-result failures** — failures the provider itself observes — are represented in
  the cross-module result: `status=UNSUPPORTED|ERROR|INDETERMINATE` (plus `error`).
- **Transport/process failures** — the provider produced no usable result — are
  represented as a **typed error** at the SOP seam (a Go `error`), mirroring
  `RunJEV`'s outcome/error split (`internal/run/jev.go:92,119-126`) and the existing
  subprocess adapters (`internal/agent/command.go:72-80`, `internal/review/ocr.go:62-70`).
- The SOP seam then maps the validated result (or the failure) into a **SOP-owned**
  failure classification / evidence for policy. No provider-specific error type is
  introduced into `agentic-sop`.

| Situation | Represented as | Never |
|---|---|---|
| No provider configured | strict no-op (no request, no result) | an error, a default provider, or an approval |
| Unsupported capability | result `status=UNSUPPORTED` + `error` | a low-confidence choice |
| Provider unavailable | typed transport error at the seam | a success |
| Timeout | typed error (SOP context deadline) | a success |
| Cancellation | typed error (SOP context) | a success |
| Malformed result (bad JSON / wrong version) | validation error at the seam | a pass |
| Unknown choice | validation error at the seam | an approval |
| Confidence out of range / `NaN` / `Inf` | validation error at the seam | a confident authorization |
| Missing confidence | indeterminate (fail closed when consumed) | certainty |
| Indeterminate result | result `status=INDETERMINATE` | a pass |
| Transport/process failure (crash, non-zero exit, empty output) | typed transport error at the seam | a success |

Rules that hold in every case: a provider failure is **never** translated into
authorization, approval, or successful execution; and disabling the capability
restores current deterministic behavior exactly (§14).

## 8. Validation ownership (SOP owns all of it)

`agentic-sop` validates before any evidence reaches policy. The provider is never
trusted to validate its own authority.

| Validation | Owner | Fail-closed rule |
|---|---|---|
| Known `contract_version` | SOP | unrecognized → failure |
| Known `status` | SOP | unknown → failure |
| Kind matches the request / supported | SOP | mismatch or unsupported → failure |
| Allowed-choice membership (`choices`) | SOP | non-member → failure |
| Choice is a known SOP-recognized value | SOP | unknown → failure |
| Confidence range `[0,1]`, finite | SOP | out of range / `NaN` / `Inf` → failure |
| Missing required fields per status | SOP | missing → failure |
| Malformed JSON / malformed result | SOP | → failure |
| Unexpected fields | SOP | additive fields ignored; a changed meaning is a version change (§16) |
| Provider-reported error consistency | SOP | `status!=OK` with a success payload → failure |
| Indeterminate result | SOP | treated as indeterminate (never approval) |
| Timeout / cancellation | SOP | → typed failure (never approval) |

The repository already contains the kernel of this validator — `knownChoice`
(`internal/decision/decision.go:30`) and `validConfidence` (`decision.go:127`) are
unexported and fail-closed — and the JEV seam already demonstrates validating
provider output before policy consumes it (`ev.Validate()`, `internal/cli/early_jev.go:177`).
SEAM-003 promotes/owns an exported, tested validator; SEAM-002 only fixes the
ownership and the rules.

## 9. Cancellation / deadline ownership (SOP owns it)

- SOP establishes the deadline at the invocation seam via `context`, exactly as
  `RunJEV` measures an invocation and the subprocess adapters use
  `exec.CommandContext` (SEAM-001 §13).
- On deadline expiry or cancellation, the seam terminates the provider and surfaces a
  **typed error**; a deadline is not evidence and can never become success.
- The request's `deadline_ms` is advisory; the provider must still treat the caller's
  cancellation as the upper bound.
- The contract does **not** require any particular transport mechanism for termination
  (process kill, socket close, etc.); it requires only that cancellation is
  SOP-owned and fail-closed.

## 10. Configuration / construction boundary

- Construction occurs at the composition root through the existing dependency-injection
  pattern (`deps`, `internal/cli/cli.go:48,106`; `deps.newJEVAnalyzer`, `cli.go:61`).
  The contract fixes **where** construction happens (SOP composition) and **that it is
  optional**, not which config key is used.
- Whether to reuse the inert `config.DecisionConfig{Provider, Enabled, Thresholds}`
  (`internal/config/config.go:295`) or add a new optional block is deferred to SEAM-003.
- The contract requires: with **no provider configured**, no request is issued and
  behavior is unchanged (§14).

## 11. Cross-module visibility / API decision

| Question | Answer |
|---|---|
| 1. Does the external adapter need to import an `agentic-sop` Go package? | **No.** |
| 2. If YES, the minimum public API | N/A — no public Go package is introduced. |
| 3. If NO, the transport-neutral contract and translation owner | The contract is the JSON request/result DTO (§4–§5). **`agentic-sop` owns translation** from the DTO into internal SOP types (mirroring `internal/agent/command.go` and `internal/review/ocr.go`, which parse JSON into internal types). |
| 4. Can `internal/decision` remain internal? | **Yes.** It stays internal; it is not exported or moved for provider convenience. |

Rationale (from SEAM-001 §12): no public package exists today; the module is
`internal/` + `cmd/` only; an external module cannot import `internal/decision`; and the
external adapter needs no SOP Go types because it satisfies a JSON protocol. Exporting
`internal/decision` is explicitly rejected.

## 12. Autonomy interpretation handoff (for SEAM-004)

Both options remain viable; SEAM-002 does not choose or implement either, and the
contract is designed so it **does not force** one.

- **Option A — translate validated evidence into a typed `failure.Classification` and
  reuse `autonomy.Decide`.**
  - *Coupling:* the seam depends on the `internal/failure` vocabulary.
  - *Duplication:* a deterministic mapping (validated result → `Kind`/`Disposition`).
  - *Semantic loss:* a decision is not inherently a failure; forcing it into
    `Kind`/`Disposition` can lose the "attention signal that is not a failure" case and
    does not express confidence/threshold semantics cleanly.
  - *Second policy path?* No — it reuses `autonomy.Decide`; the `DecideEarly` precedent
    (`internal/autonomy/early.go:82-88`) already translates typed evidence into a
    `failure.Classification` before calling `Decide`.
- **Option B — a dedicated `autonomy` entry point with an autonomy-owned,
  provider-neutral evidence type.**
  - *Duplication:* a second evidence type plus a second entry point.
  - *Second policy path?* Risk of one unless it delegates internally to `Decide`.
  - *Benefit:* avoids forcing a decision into a failure-shaped type.

**Currently preferred: A**, because `autonomy.Decide` already expresses the required
policy inputs and the full action vocabulary, and the `DecideEarly` pattern shows a
thin, deterministic translation into `failure.Classification` before `Decide`; B
duplicates an evidence type and risks a second policy path. (SEAM-001 §14 reached the
same provisional view.)

**How the contract avoids forcing either:** the DTO carries only a domain `choice`,
optional `confidence`, and advisory `diagnostics`. It contains no failure-shaped fields
and no autonomy-shaped fields, so the same contract is consumable by either A or B.
The final choice remains a SEAM-004 decision.

## 13. Transport independence

The contract must not depend on Clef, oMLX, Ollama, SystemOne, Nimble, a specific
executable, HTTP, shell, a particular model, or SMALL/MEDIUM/LARGE execution routing.
It is defined over the JSON DTO only, so it is implementable over an appropriate future
transport without changing SOP policy semantics. In particular, the contract does
**not** bake in `sh -c`: whether SEAM-003 uses `exec.CommandContext` with a shell or a
safe direct-executable mechanism (both precedents exist — SEAM-001 §13) is a transport
choice that must not alter the DTO or policy.

## 14. Default / off behavior

- **No external provider configured = existing SOP behavior.** No request is issued; no
  provider is required, constructed, or defaulted.
- No execution-model default changes; SMALL/MEDIUM/LARGE routing is untouched and
  independent (SEAM-001 §11).
- The capability is optional and OFF by default, mirroring the existing optional-seam
  pattern (`config.EarlyJEV.Enabled *bool` + `EarlyJEVActive()`,
  `internal/config/config.go:250,677`; `earlyGateEnabled`,
  `internal/cli/early_jev.go:69`).

## 15. Security / governance restrictions

A provider MUST NOT be given authority to: mutate task state; mutate plan state;
approve work; create approvals; authorize execution; commit; merge; change policy;
select lifecycle transitions; bypass validation; bypass human approval.

The contract enforces this **structurally**:

- It exposes no method and no field representing any of those authorities; the DTO
  carries only a bounded question, numeric signals, allowed choices, and a bounded
  domain result.
- The provider cannot return a governance action (§5): the result vocabulary is a
  domain choice, not `CONTINUE`/`BLOCK`/`APPROVE`/`REJECT`/`COMMIT`/`MERGE`.
- SOP owns validation (§8), cancellation (§9), and interpretation (§12); the human
  boundary is reached only through the existing SOP path
  (`humanBoundary`, `internal/cli/approval.go:400` → `recordHumanApprovalRequest`,
  `:410` → `internal/approval` + `runpkg.WaitingForHuman`).
- Selecting a provider is a transport/config concern; it is never a `provider == "X"`
  branch inside SOP governance code.

If the contract were to require any of those authorities, SEAM-002 would STOP. It does
not.

## 16. Versioning / compatibility considerations

- `contract_version` is required in both request and result; a mismatch is a failure.
- **Additive compatibility:** adding new `signals` keys, `diagnostics` keys, or
  capability-specific result fields is backward-compatible. Removing or re-meaning an
  existing field is not, and requires a version change.
- **Choice vocabulary:** the set of SOP-recognized choices is fixed per contract
  version. A provider must not invent choices and expect them to be treated as known —
  unknown choices fail closed (§8).
- **Status vocabulary:** the set `OK|UNSUPPORTED|ERROR|INDETERMINATE` is fixed per
  version; unknown statuses fail closed.
- **Fail-closed default:** compatibility is preserved in the safe direction — when an
  older/newer provider cannot be interpreted, the seam fails closed, never to approval.
- **No wire-protocol coupling:** compatibility is defined at the semantic level only;
  no transport, encoding, or provider protocol is part of the compatibility surface.

## 17. Example provider-neutral exchanges

All examples are provider-neutral (no Clef-specific payloads).

**Provider A — deterministic fake provider (choice + confidence).**

```json
// request
{"contract_version":1,"kind":"implementation-risk","question":"Add caching to provider discovery","signals":{"files":3,"criteria":2},"choices":["LOW","MEDIUM","HIGH","HUMAN"],"request_id":"r-1"}
// result
{"contract_version":1,"status":"OK","kind":"implementation-risk","choice":"MEDIUM","confidence":0.8,"diagnostics":{"reason":"moderate size"}}
```

**Provider B — external provider supporting choice + confidence + diagnostics.**

```json
// result
{"contract_version":1,"status":"OK","kind":"implementation-risk","choice":"HIGH","confidence":0.9,"diagnostics":{"policy":"touches-auth"}}
```

**Provider C — provider that does not support the requested capability.**

```json
// result
{"contract_version":1,"status":"UNSUPPORTED","kind":"implementation-risk","error":"kind not supported by this provider"}
```

**Provider D — unavailable / timed out.** No result is produced; the transport fails
and the SOP seam surfaces a typed error (never a success). A provider that *runs* but
cannot decide returns `status=INDETERMINATE`:

```json
// result
{"contract_version":1,"status":"INDETERMINATE","error":"insufficient signal to decide"}
```

**Invalid (fails closed at validation, never approval):**

```json
{"contract_version":1,"status":"OK","choice":"APPROVE"}                      // governance action + unknown choice -> reject
{"contract_version":1,"status":"OK","choice":"LOW","confidence":1.5}         // out-of-range confidence -> reject
{"contract_version":1,"status":"OK"}                                         // missing required choice -> reject
```

## 18. Explicit non-goals

- No Clef/provider-specific payload, endpoint, transport, or model.
- No lifecycle, approval, commit, merge, plan-mutation, or policy-action value crosses
  the boundary.
- No requirement to export or move `internal/decision`.
- No mandated transport (shell, HTTP, direct exec), and no `sh -c` semantics in the
  contract.
- No coupling between decision evidence and SMALL/MEDIUM/LARGE execution routing.
- No provider becomes required or default; no execution-model default changes.

## 19. Open questions deferred to later SEAM tasks

1. **A vs B** autonomy interpretation — final choice in SEAM-004 (§12 records the
   current preference for A).
2. **Transport mechanism** — shell vs safe direct executable in SEAM-003 (the contract
   is silent by design, §13).
3. **Capability advertisement** mechanism (handshake vs config) — SEAM-003.
4. **Config surface** — reuse `config.DecisionConfig` vs a new optional block —
   SEAM-003.
5. **Failure disposition** when a configured provider fails (continue-under-deterministic
   policy vs fail-closed-to-human) — SEAM-005; both must never become approval.
6. **Whether `request_id` is needed at all** — currently optional and non-authoritative.

---

## Architecture check

The contract is tested conceptually against four providers; all four fit the **same**
contract without changing SOP policy:

| Provider | Behavior under the contract |
|---|---|
| **A. Deterministic fake** | `status=OK`, `choice`, optional `confidence`. |
| **B. External (choice + confidence)** | `status=OK`, `choice`, `confidence`, `diagnostics`. |
| **C. Unsupported capability** | `status=UNSUPPORTED` + `error`; SOP treats it as an explicit failure, never a guess. |
| **D. Unavailable / timed out** | No result; typed transport error at the seam; never a success. |

**No provider-name branch is required.** Adding a hypothetical future provider is a
configuration/transport change: SOP policy never sees a provider identity. The seam
selects a provider by configuration/transport, validates the DTO, and interprets
evidence through the same SOP-owned policy — so no `provider == "X"` branch is added to
governance code.

**Authority check.** None of the four providers can mutate task/plan state, approve,
create approvals, authorize execution, commit, merge, change policy, select a lifecycle
transition, bypass validation, or bypass human approval: the contract carries none of
those capabilities, and SOP owns validation, cancellation, and interpretation.

## Verification and scope statement

- Plan unchanged: SEAM-002 modifies no task definition in
  `docs/plans/PLAN-Provider-Neutral-Decision-Integration-Seam.md`; later tasks are
  unchanged; no new external `Requires` and no discovery/report path modeled as a
  prerequisite capability are introduced.
- `git diff` contains **documentation only**; no production file is modified.
- No stop condition was triggered: the contract requires no lifecycle authority, does
  not export `internal/decision`, leaks no provider-specific semantics, does not couple
  decision evidence to execution routing, and requires no production change.

No commit or push was performed.
