# JEV Decision Contract Alignment

**Status:** APPROVED — Gate 3 ratified 2026-10-11. §20 records the ratified
decision (D1–D7) and its constraints; §13–§19 record the accepted contract
requirements.

**Task:** JEV-202 (Decision Contract Alignment).

**Decision owner:** the operator-owned approval gate (Gate 3 of
PLAN-JEV-Wave-2-Hardening), not this document.

**What this document is:** the approved architecture decision record — an accurate
current-state comparison of the decision contracts that actually exist, the
deterministic mapping invariants the adopted mapping satisfies, the Gate 3
decision (D1–D7) with its ratified constraints, and the accepted contract
requirements. It exists so the choice is applied against written constraints
rather than ad hoc.

**What this document is not:** it does not implement a contract. Gate 3 has
selected the architecture below; the deterministic bridge, adapter-profile
documentation, and compatibility tests are the implementation work (§19), tracked
separately. JEV-202's only automated in-tree change remains the non-breaking
configuration rejection described in §12.

> **APPROVED — Gate 3 ratified 2026-10-11.** SEAM-002 is the canonical SOP
> decision boundary; Julia-1/Nimble retain their existing batched wire contract;
> M3 is adopted (SOP stamps the recognized legacy profile version at its
> deterministic bridge; adapters add no wire-level version field). Profile
> recognition validates schema, question identity, answer type, and capability
> semantics — not provider name alone. SOP owns all deterministic policy,
> escalation, and fail-closed mapping; JEV remains evidence-only; and no breaking
> change is made to existing decision APIs, DTOs, or configuration.

## 1. The three actual artifacts

A prior draft of this ADR described "two overlapping provider-neutral
contracts." That framing was inaccurate. There are **three distinct artifacts**,
and only the first two are the same seam:

1. **SEAM-002 — the canonical SOP decision contract.** The SOP-side Go surface in
   `internal/decision` (`Request`, `Result`, `Decision`, `Choice`, `Status`,
   `ContractVersion`) plus the JSON-over-stdin/stdout process DTO realized by
   `internal/decision/command`. It is specified in
   `docs/reports/decision-integration/SEAM-002-decision-contract.md`.
2. **AS-CLEF-008 — documentation of the _same_ seam.** The adapter-obligations
   text in `docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md`. It is
   **Go-typed and transport-agnostic**: it names no wire encoding, no `stdin`/
   `stdout`, no JSON DTO, and no `contract_version`, and it derives its terms from
   the same in-tree `internal/decision` types. It is therefore a _description of
   SEAM-002_, not a second contract, and it defines **no DTO**.
3. **The deployed `sop-decision-adapters` contract — a DIFFERENT shape.** The Go
   contract shipped in `sop-decision-adapters/decision`
   (`DecisionRequest{state, questions[], metadata}` →
   `DecisionResult{provider, model, answers{}, usage}`), implemented by the Julia-1
   and Nimble adapters. It is **batched** (many questions per call), carries
   **no `contract_version`**, **no `status`/`diagnostics`/`error`**, and expresses
   outcomes as `choice` / `boolean` / `score` with per-answer probabilities and
   confidence.

**Correction of the prior claim.** The deployed adapter DTO does **not**
implement SEAM-002. SEAM-002 is one bounded decision per call with a closed
`Choice{LOW,MEDIUM,HIGH,HUMAN}` and a `status` vocabulary; the deployed contract
is many questions per call with `choice`/`boolean`/`score` answers and no version
or status. `sop-decision-adapters/internal/decision` is **empty** at the audited
revision — SEAM-002 is not implemented there. The JSON DTO attributed to
"AS-CLEF-008" belongs solely to `internal/decision/command` in `agentic-sop`.

## 2. Purpose and non-goals

**Purpose.** Record the comparison, the deterministic mapping invariants for
choice/boolean/score/confidence/probability, the versioning and migration
strategy, the ownership boundaries, and the authority question, so Gate 3 can
choose one authoritative contract.

**Non-goals (explicit).**

- Do not select or adopt a contract; the choice is Gate 3.
- Do not implement any adapter change (that is gated on the Gate 3 choice).
- Do not add a JEV decision provider.
- Do not make any breaking contract or API change.
- Do not weaken deterministic approval gates or the human boundary.

## 3. Current-state comparison (three artifacts)

Both SEAM-002 and the deployed adapter contract carry **evidence only**. Neither
grants lifecycle, approval, commit, merge, scheduler, or policy authority; SOP
owns validation and interpretation.

| Aspect                   | SEAM-002 (canonical SOP)                                                                    | AS-CLEF-008 (same seam, docs)        | Deployed `sop-decision-adapters`                                      |
| ------------------------ | ------------------------------------------------------------------------------------------- | ------------------------------------ | --------------------------------------------------------------------- |
| Carrier                  | Go types `internal/decision` + JSON DTO `internal/decision/command`                         | Go-typed, transport-agnostic; no DTO | Go types `decision` (`request.go`, `result.go`)                       |
| Request keys             | `contract_version, kind, question, signals, choices, deadline_ms` (+ optional `request_id`) | `internal/decision.Request`          | `state, questions[{id,type,instructions,criteria,choices}], metadata` |
| Result keys              | `contract_version, status, kind, choice, confidence, diagnostics, error`                    | `internal/decision.Result`           | `provider, model, answers{}, usage`                                   |
| Granularity              | one bounded decision per call                                                               | one                                  | **many questions per call**                                           |
| Decision types           | closed `Choice{LOW,MEDIUM,HIGH,HUMAN}`                                                      | same                                 | `choice` / `boolean` / `score`                                        |
| Version                  | `ContractVersion = 1`                                                                       | same                                 | **no `contract_version`**                                             |
| Status / error           | `status{OK,UNSUPPORTED,ERROR,INDETERMINATE}` + 5 sentinels                                  | same                                 | **no `status`**; 4 `ErrorKind` sentinels                              |
| Confidence / probability | `*float64` in `[0,1]`; absent ⇒ indeterminate                                               | same                                 | per-answer `*float64` + `Probabilities` + `Probability` + `Score`     |
| Validation owner         | SOP (`ValidateResult`)                                                                      | SOP                                  | SOP (never the provider)                                              |
| Authority                | none; `Route` fails closed to a human                                                       | none                                 | none                                                                  |

Known minor drifts to reconcile (not blockers): the realized command request DTO
omits SEAM-002's optional `request_id`; and `ValidateResult` does not enforce the
doc's "`error` required for `UNSUPPORTED`".

## 4. Deterministic mapping invariants: choice / boolean / score / confidence / probability

Whatever Gate 3 adopts, every adapter-native shape must reduce to a canonical
`(Choice, optional Confidence)` pair, and must **fail closed** when it cannot be
mapped. The rows below are **proposed invariants, not an adopted mapping**.

| Adapter-native shape          | Canonical SEAM-002 element | Proposed treatment (NOT adopted)                                   | Fail-closed constraint                                                            |
| ----------------------------- | -------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------- |
| enumerated `choice`           | `Choice`                   | map **only** through an explicit allowed-value table               | a choice outside the table is indeterminate → HUMAN                               |
| `boolean` (+ yes-probability) | `Choice`                   | **capability-specific** policy table (e.g. a documented threshold) | a bare boolean is **not** a `Choice`; with no policy it is indeterminate → HUMAN  |
| numeric `score`               | `Confidence`               | **capability-specific** normalization into `[0,1]`                 | a raw score is **not** a probability; un-normalized/out-of-range is invalid       |
| `confidence`                  | `Confidence`               | direct                                                             | finite ∈ `[0,1]`; `NaN`/`Inf`/out-of-range invalid                                |
| `probability`                 | `Confidence`               | direct **only** when it is the probability of the chosen outcome   | a probability of a _different_ event is not confidence; ambiguous ⇒ indeterminate |

Two invariants hold for any adopted mapping:

1. **No shape becomes authority.** A boolean or score never routes as an approval
   because it is "positive"; it routes only after a documented mapping produces a
   known `Choice` and (optionally) a valid `Confidence`.
2. **Unmappable is fail-closed.** If a shape cannot be mapped, the result is
   indeterminate and deterministic policy fails closed to **HUMAN**. A provider
   failure or an unknown/indeterminate result is **never** translated into
   authorization or successful execution.

## 5. Error model

Two deliberately distinct channels, matching SEAM-002 §7:

- **In-result failures the provider observes:** `status` of `UNSUPPORTED`,
  `ERROR`, or `INDETERMINATE` (plus `error`).
- **Transport/process failures that produce no usable result:** a typed Go error
  at the SOP seam (empty output, non-zero exit, overflow, timeout, cancellation).

Rules that hold in every case: a provider failure or an unknown/indeterminate
result is never an approval, a pass, or successful execution; unknown or
uninterpretable results fail closed to a human (`decision.Route`); and disabling
the capability restores current deterministic behavior exactly (a strict no-op,
not an error).

## 6. Versioning and migration strategy

**Versioning (minimum bar for any adopted contract).**

- `contract_version` is required in both request and result; a mismatch is a
  failure.
- Additive fields are compatible (new `signals`, `diagnostics`, optional
  capability fields). Removing or re-meaning an existing field requires a version
  bump.
- The choice and status vocabularies are fixed per version; unknown values fail
  closed.
- The provider name is the selection key; a change to it is a version boundary.
- Compatibility is semantic only; no transport, encoding, or provider wire
  protocol is part of the compatibility surface.

**Migration options for existing unversioned adapter clients.** The deployed
adapter contract has **no `contract_version`**, so every current client is
"unversioned". Gate 3 chooses one of:

- **M1 — Add a required `contract_version` (default `1`) to the adapter profile.**
  Clients that omit it are treated as version `1` for a bounded transition window,
  then rejected. Lowest friction; the profile becomes self-versioning.
- **M2 — Add `contract_version` as optional, defaulting to `1`, with a deprecation
  notice.** No client breaks; the version is recorded but not enforced until a
  future boundary.
- **M3 — Introduce the version only at the SOP bridge.** Adapters stay
  unversioned; SOP stamps/derives the version at the seam. Least adapter change,
  but the adapter contract remains unversioned.

Any option is additive for the adapter; none is required by JEV-202. **Recorded
Gate 3 decision: M3** (the version is stamped/derived at the SOP bridge; see §20
D3 and §17).

## 7. Contract architecture (RATIFIED at Gate 3)

**Adopted: SEAM-002 remains the canonical contract inside SOP; the
Julia-1 and Nimble adapters retain a _separately versioned, batched adapter
profile_; SOP owns the deterministic translation and all policy enforcement;
JEV remains an evidence producer, never an execution authority.**

```text
SOP workflow (owns policy, gates, approvals)         [agentic-sop]
        |
        |  canonical decision seam: SEAM-002 (internal/decision + command DTO)
        v
   deterministic translation + fail-closed mapping   [SOP-owned]
        ^
        |  versioned batched adapter profile (choice/boolean/score)
        |
  Julia-1 / Nimble adapters (translate + normalize)   [sop-decision-adapters]

JEV (engineering analysis) ---> advisory evidence only, never authority
```

Rationale: SEAM-002 already exists on the SOP side, is validated, fail-closed,
and authority-free; the adapters are translate-and-normalize with no authority. A
single canonical SOP contract avoids a third text and drift, while the separate
versioned adapter profile preserves the adapters' batched, richer shape without a
breaking rewrite. The alternatives considered at Gate 3 were:

- **Option A** — SEAM-002 canonical; adapters conform to it.
- **Option B** — a versioned superset text with SEAM-002 as a conforming profile.
- **Option C** — keep both, with the crosswalk as the only binding text.
  Gate 3 adopted **A + a versioned adapter profile** (the adapters keep a
  distinct, versioned batched shape bridged deterministically to SEAM-002).

**Ratified at Gate 3:** the profile version is stamped at the SOP bridge (M3,
§20 D3), so the adapters themselves stay wire-unversioned and **no adapter field
is added** (§17) — which is what keeps the choice non-breaking (D6).

## 8. The authority question

> Providers evaluate. SOP governs.

The authority question Gate 3 must answer is **which contract text and version is
authoritative and who owns its evolution** — not whether authority moves to
providers. Neither contract carries authority: `internal/decision` exposes no
method or field for task/plan mutation, approval, commit, merge, policy change, or
lifecycle transition, and the adapter DTO carries only a bounded question,
numeric signals, allowed choices, and a bounded result. The deterministic SOP
policy (`internal/autonomy`; `decision.Route`; the human boundary) retains final
authority; decision evidence may add **attention** and route to a strong model or
a human, but it can never auto-approve or bypass the human boundary.

## 9. Repository ownership boundaries

- **`agentic-sop` owns:** the canonical SEAM-002 contract text and version; the
  process DTO (`internal/decision/command`); the deterministic translation and
  fail-closed mapping; all policy, gates, and approvals; and JEV (evidence only).
- **`sop-decision-adapters` owns:** its versioned batched adapter profile; the
  Julia-1 and Nimble translators/normalizers; and provider-native semantics. It
  **must not** define or enforce SOP policy, thresholds, or approval rules.
- **Neither repo** may grant a provider lifecycle, approval, commit, merge, or
  execution authority.

## 10. Compatibility and testing matrix

| Area              | `agentic-sop` obligation                                                             | `sop-decision-adapters` obligation                               |
| ----------------- | ------------------------------------------------------------------------------------ | ---------------------------------------------------------------- |
| Contract shape    | SEAM-002 request/result keys + closed vocabularies                                   | batched profile keys + `choice`/`boolean`/`score`                |
| Versioning        | `ContractVersion` enforced both directions                                           | profile version stamped at the SOP bridge (M3); no adapter field |
| Status/error      | `status` vocabulary + 5 sentinels                                                    | map `ErrorKind` → SOP status/error                               |
| Mapping           | deterministic choice/boolean/score/probability → `(Choice, Confidence)`; fail-closed | expose native shapes + per-answer probabilities/confidence       |
| Authority         | policy/gates/approvals only here                                                     | none                                                             |
| Tests             | contract validation, fail-closed routing, seam neutrality (existing)                 | adapter normalization/range tests; profile-identity tests        |
| Existing coverage | `internal/decision` + `internal/decision/command` tests                              | `internal/providers/{julia,nimble}` tests                        |

## 11. Optional `request_id` alignment (separate from mandatory changes)

SEAM-002 specifies an optional `request_id`; the realized command request DTO does
not carry it and the deployed adapter contract has no equivalent. Aligning it is
**optional and additive** and is tracked separately from any **mandatory**
contract change: adding `request_id` (optional, `omitempty`) to the command DTO and
threading a correlation id through the adapter profile is a non-breaking
enhancement, not part of the Gate 3 authority decision.

## 12. What JEV-202 changes (non-breaking)

The only automated change is configuration validation, which is backward-safe:

- `internal/config` now accepts only `deterministic` and `command` for
  `decision.provider`; the legacy `jev` value and any invented value are rejected
  through the existing default branch (`internal/config/config.go` `Validate`).
- The `Provider` field doc comment and the surrounding `DecisionConfig` comment
  were updated to match the accepted set.
- `TestDecisionProviderJevDoesNotEnableJEV` was updated to reflect rejection while
  preserving its intent, and `TestDecisionProviderRejectsUnsupported` was added.
- `docs/reference/CONFIGURATION.md` was reconciled so `decision.provider` is
  documented as `deterministic` (default) or `command`.

Explicitly **not** done here: no contract selected or adopted; no adapter bridge;
no new JEV decision provider; no breaking contract or API change; no change to
deterministic approval gates or the human boundary.

## 13. Bridge mechanics: question-ID selection and batch cardinality

The canonical seam is **one bounded decision per call** (SEAM-002); the deployed
adapter profile is **many questions per call against one `state`**. The SOP-owned
bridge (work package A) is the only component that converts between them, and it
owns every rule below. Adapters never see a canonical `kind`; they see batched
questions. SOP never sees a batched result; it sees one canonical result.

- **One canonical request → one question.** Each canonical SEAM-002 request maps
  to **exactly one** adapter `Question`. Batching across canonical requests is
  permitted only when they share an identical `state`; the minimum viable bridge
  sends **cardinality 1** and is sufficient for JEV-206.
- **Deterministic question ID.** The bridge derives the question `ID` from the
  canonical request by a documented, deterministic rule (for example `<kind>` or
  `<kind>:<hash(state)>`) — never randomly — so the answer can be attributed back
  without ambiguity. The ID MUST be non-empty and unique within a batch.
- **`state` is the bounded subject.** The adapter `state` is built from the
  canonical `question`/`subject` plus a bounded, provider-neutral rendering of
  `signals`. The adapter's rule applies: `state` is required and non-empty, so
  the bridge MUST send a non-empty `state`.
- **Only mappable questions are sent.** The bridge issues a question only for a
  canonical `kind` it can map to a question `type` _and_ back to a canonical
  `(Choice, optional Confidence)`. A kind with no mapping is **not** sent; the
  result is indeterminate → HUMAN (§18). It is never sent as a low-confidence
  guess.
- **Cardinality is preserved both directions.** Exactly the questions the bridge
  sent must be answered; a result with a different question set fails closed
  (§14).

## 14. Answer completeness: duplicate, missing, and unexpected answers

`DecisionResult.Answers` is keyed by question ID, and the deployed
`DecisionResult.Validate` checks only that the map is non-empty and that each
answer is internally valid — it does **not** assert that every requested question
is answered. That completeness guarantee is a **bridge obligation**, not an
adapter obligation, and the bridge MUST enforce it fail-closed:

| Case                                            | Adapter behavior                       | Bridge rule (fail-closed)                                                  |
| ----------------------------------------------- | -------------------------------------- | -------------------------------------------------------------------------- |
| Missing answer for a sent question              | allowed by `Validate`                  | **reject the whole result** → indeterminate → HUMAN                        |
| Unexpected/extra answer for a question not sent | allowed by `Validate`                  | **reject the whole result** (do not silently drop) → indeterminate → HUMAN |
| Duplicate question `ID` in the request          | rejected by `DecisionRequest.Validate` | never sent; request is malformed                                           |
| Duplicate choice within a CHOICE question       | rejected by `Question.Validate`        | never sent                                                                 |
| Duplicate answer key                            | impossible (map)                       | n/a                                                                        |

Rule: an answer set that does not exactly match the sent question set is **not**
partially trusted. Partial results cannot authorize continuation; the whole
result is indeterminate.

## 15. Per-question type and capability validation

- The bridge maps a canonical `kind` to exactly one adapter question `type` from
  the closed set `{choice, boolean, score}`. The mapping is a documented table
  owned by SOP; it is not inferred by the adapter.
- The answer `type` MUST equal the question `type`; a mismatch is malformed →
  indeterminate → HUMAN.
- A capability the adapter cannot serve is expressed as a **missing answer** or a
  typed provider error (§17/§18); it is never expressed as a low-confidence guess
  and never mapped to a non-HUMAN choice.
- Capability is not authority: knowing which types exist never selects a
  lifecycle transition, model class, or approval (§8).

## 16. Confidence and probability semantics (bridge-enforced)

Both sides already enforce finite `[0,1]` for `confidence`, per-choice
`probabilities`, and boolean `probability` (NaN/Inf/out-of-range rejected). The
bridge adds the semantic reductions below; each is **capability-specific** and
owned by SOP:

| Adapter answer                            | Canonical mapping (SOP-owned)                                                                       | Fail-closed rule                                                                       |
| ----------------------------------------- | --------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- |
| CHOICE + `confidence`                     | `Choice` from the allowed-value table; `Confidence` = confidence                                    | out-of-range/NaN → invalid → indeterminate → HUMAN                                     |
| CHOICE + `probabilities` only             | `Confidence` = probability **of the chosen choice**, **only if** the profile declares that semantic | otherwise indeterminate → HUMAN; probabilities of _other_ choices are never confidence |
| BOOLEAN + `probability` (yes-probability) | mapped to a `Choice` **only through a documented threshold table**                                  | with no threshold policy → indeterminate → HUMAN; a bare boolean is not a `Choice`     |
| SCORE                                     | `Confidence` = **capability-specific normalized** `[0,1]` value                                     | un-normalized/out-of-range → invalid → indeterminate → HUMAN                           |

Invariant: **no shape becomes authority.** A boolean or score never routes as an
approval because it is "positive"; it routes only after a documented mapping
produces a known `Choice` and (optionally) a valid `Confidence` (§4). Confidence
semantics are explicit per capability, never inferred from the raw number alone.

## 17. Legacy-profile identification and deprecation criteria

Under the ratified migration decision (**M3**, §6), the canonical contract
version is **stamped and derived at the SOP bridge**; adapters remain
wire-unversioned, so no adapter rewrite is required. Profile identity is:

- **Identity = the validated structural profile plus the bridge's declared
  version stamp.** The adapter reports `Provider` (and optionally `Model`) in
  `DecisionResult`; the bridge records this with its own profile-version stamp.
- **Recognition validates structure, not a name alone** (Gate 3 constraint 4).
  The bridge recognizes a profile by validating its **schema, question identity,
  answer type, and capability semantics**. A result that does not structurally
  match a known profile — or that is ambiguous or carries an unknown/absent
  `Provider` — is rejected (fail closed). The `Provider` name is a correlation
  hint, never the sole selection key.
- **Deprecation criteria.** A profile is _deprecated_ when its bridge mapping
  entry is marked deprecated: it still maps deterministically, but the bridge
  emits a deprecation diagnostic (never a policy input). A deprecated profile is
  removed at the next canonical version boundary.
- **Rejection criteria.** Unknown, absent, or ambiguous profiles are rejected
  immediately (fail closed) — the bridge does not guess a profile.

Because M3 adds no adapter field, existing unversioned clients keep working; the
bounded migration is "stamp at the bridge, reject the unknown."

## 18. Deterministic failure → HUMAN mapping

Every failure class reduces to **indeterminate → HUMAN** via `decision.Route`
(an unknown choice or an invalid confidence ⇒ `HumanTarget`). No failure ever
authorizes execution, and provider failures are never translated into
authorization or a successful decision (SEAM-002 §7; §5 here).

| Adapter-side outcome                                      | Canonical class            | Canonical status / sentinel                | Routed to                        |
| --------------------------------------------------------- | -------------------------- | ------------------------------------------ | -------------------------------- |
| `status=OK` choice + valid confidence (mapped)            | usable evidence            | `OK`                                       | policy tier (never auto-approve) |
| Missing answer / cannot serve kind                        | unsupported                | `UNSUPPORTED` → `ErrUnsupportedCapability` | HUMAN                            |
| `ErrorKind=malformed_response`, answer type/range invalid | invalid                    | `ERROR` / `ErrInvalidResult`               | HUMAN                            |
| `ErrorKind=invalid_request`                               | invalid                    | `ErrInvalidResult`                         | HUMAN                            |
| `ErrorKind=unavailable`                                   | transport/provider failure | `ErrProviderFailure`                       | HUMAN                            |
| `ErrorKind=provider_failure`                              | transport/provider failure | `ErrProviderFailure`                       | HUMAN                            |
| Unknown / ambiguous / unmappable result                   | indeterminate              | `INDETERMINATE` → `ErrIndeterminate`       | HUMAN                            |

Diagnostics are **class-only**: the canonical `error`/`diagnostics` never carry
endpoint URLs, response bodies, tokens, or credentials (consistent with the
JEV-203 fix, `jevFailClosedReason`).

## 19. Cross-repository compatibility tests (SOP, Julia-1, Nimble)

| Repo                              | Obligation                                                                                                                                                                                    | Representative tests                                    |
| --------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------- |
| `agentic-sop`                     | bridge: ID selection, cardinality, answer-completeness, type/capability mapping, confidence/probability reduction, profile identity/deprecation, class-only failure mapping, version stamping | new bridge unit tests (golden wire + fail-closed table) |
| `sop-decision-adapters` (Julia-1) | profile shape preserved; choice/boolean/score normalization; range rejection; `ErrorKind` classification                                                                                      | `internal/providers/julia/*_test.go`                    |
| `sop-decision-adapters` (Nimble)  | same                                                                                                                                                                                          | `internal/providers/nimble/*_test.go`                   |
| JEV-206 evidence                  | recorded report with command, revision, exit status, per-adapter result                                                                                                                       | `internal/jev/adapter_compat_report_test.go`            |

The JEV-206 validation **runs in `sop-decision-adapters`** (the owning repo) and
is **recorded in `agentic-sop`**. A passing JEV-202 configuration check does not
satisfy JEV-206; the report must carry the command, the repository revision, the
exit status, and the individual Julia-1 and Nimble results.

## 20. Gate 3 decision (RATIFIED 2026-10-11)

Gate 3 is **ratified**. D1–D7 are approved with the constraints below; they are
consistent with §4–§19.

- [x] **D1 — Authority.** SEAM-002 is the canonical SOP decision boundary. Julia-1
      and Nimble retain their **existing batched wire contract**, bridged
      deterministically to SEAM-002 (Option A + a versioned adapter profile, §7).
- [x] **D2 — Mapping.** SOP owns the deterministic translation, validation,
      policy, escalation, and fail-closed behavior (§4, §15–§18); JEV remains
      evidence-only.
- [x] **D3 — Adapter versioning.** Adopt **M3** (§6): SOP **stamps the recognized
      legacy profile version at its deterministic bridge**; adapters do **not** add
      a wire-level version field. Unknown or ambiguous profiles are rejected (§17).
- [x] **D4 — Status/error bridge.** Map the adapter's typed `ErrorKind` to
      canonical `status`/`error` **without exposing sensitive provider
      diagnostics** (class-only, §18).
- [x] **D5 — `request_id`.** **Deferred** (§11); optional and additive, not part of
      the Gate 3 authority decision.
- [x] **D6 — Breaking change.** **None authorized.** The bridge is an additive SOP
      component; no breaking change to existing decision APIs, DTOs, or
      configuration. `internal/decision`, the command DTO, and
      `config.DecisionConfig` are unchanged.
- [x] **D7 — ADR correction.** The corrected **three-artifact** comparison (§1)
      supersedes the prior "two overlapping contracts" framing.

**Ratified constraints (Gate 3).**

1. SEAM-002 is the canonical SOP decision boundary.
2. Julia-1/Nimble retain their existing batched wire contract.
3. M3: SOP stamps the recognized legacy profile version at its deterministic
   bridge; adapters add no wire-level version field.
4. Profile recognition validates **schema, question identity, answer type, and
   capability semantics** — **not provider name alone** (§17).
5. SOP owns all deterministic policy, escalation, and fail-closed mapping.
6. JEV remains evidence-only.
7. No breaking changes to existing decision APIs, DTOs, or configuration.

**Resolution of the D1/D3 reconciliation note.** Gate 3 resolved the earlier
M1/M2-vs-M3 flag in favor of **M3**: the profile version is a SOP-side concept
stamped at the bridge, no adapter field is added, and constraint 7 (no breaking
change) holds.

## 21. References

- docs/reports/decision-integration/SEAM-002-decision-contract.md
- docs/reports/clef-readiness/AS-CLEF-008-adapter-contract.md
- internal/decision/decision.go
- internal/decision/command/command.go
- internal/cli/decision_provider.go
- sop-decision-adapters `decision/{provider,request,result,errors}.go`
- sop-decision-adapters `internal/providers/{julia,nimble}` and `docs/specs/`
- docs/plans/PLAN-JEV-Wave-2-Hardening.md
