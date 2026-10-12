# Batched Adapter Profile — SOP Bridge Note

**Type:** Normative note on the adapter profile
**Status:** Ratified at Gate 3 (agentic-sop `PLAN-JEV-Wave-2-Hardening`)

This note records what the *deployed* batched contract is from the SOP side,
what the SOP bridge depends on, and the one guarantee the adapter deliberately
does **not** provide. It does not change the contract: the wire shape described
in [`decision-contract.md`](decision-contract.md) is unchanged, and no
wire-level version field is added.

## 1. Relationship to SEAM-002

There are two artifacts, and they are **not** the same shape:

- **SEAM-002** (canonical, inside `agentic-sop`): a provider-neutral decision
  seam that carries **one bounded decision per call**, with a closed
  `Choice{LOW,MEDIUM,HIGH,HUMAN}`, a `contract_version`, and a `status`
  vocabulary. SOP owns its validation.
- **This batched profile** (this repository): `DecisionRequest{state, questions[],
  metadata}` → `DecisionResult{provider, model, answers{}, usage}`. It carries
  **several questions per call** against one `state`, expresses outcomes as
  `choice` / `boolean` / `score` with per-answer probabilities/confidence, and
  carries **no** `contract_version`, `status`, `diagnostics`, or `error`.

Neither artifact carries authority. Providers evaluate; SOP governs.

## 2. Profile versioning is a SOP-side concept (M3)

At Gate 3, SOP adopted **M3**: the recognized profile version is **stamped and
derived at the SOP bridge**, and the adapters add **no wire-level version
field**. This keeps the profile usable by existing, unversioned clients with no
breaking change and no adapter rewrite.

Consequences for this repository:

- The contract in `decision/` stays exactly as specified in
  [`decision-contract.md`](decision-contract.md); nothing here is versioned on the
  wire.
- Profile identity and the version stamp are assigned by SOP, which recognizes a
  profile by validating **schema, question identity, answer type, and capability
  semantics** — not by provider name alone.

## 3. What the SOP bridge depends on

The bridge (`agentic-sop/internal/decision/bridge`) translates a canonical
SEAM-002 request into one question here, and translates the answer back to a
canonical `(Choice, optional Confidence)`. It relies on these adapter-side
guarantees, each pinned by `decision/profile_compat_test.go`:

| Guarantee | Where |
|---|---|
| `state` is required and non-empty | `DecisionRequest.Validate` |
| at least one question, unique IDs | `DecisionRequest.Validate` |
| CHOICE requires non-empty `criteria` and ≥1 distinct non-empty choice | `Question.Validate` |
| closed question/answer type set `{choice, boolean, score}` | `Question/Answer.Validate` |
| confidence/probabilities/probability are finite in `[0,1]` (NaN/Inf rejected) | `Answer.Validate` |
| failures are classified by a stable `ErrorKind` usable via `errors.Is` | `ProviderError.Is` |

## 4. The guarantee the adapter does **not** provide

`DecisionResult.Validate` checks only that the answer map is **non-empty** and
that each answer is internally valid. It **cannot** assert that every requested
question is answered, because it has no request. The contract's rule that
"`Answers` MUST contain an entry for every requested question" is therefore a
**bridge obligation**, not an adapter one:

- The SOP bridge rejects a result whose answer set does not exactly match the
  questions it sent (a missing or unexpected answer fails closed).
- Partial results are never partially trusted: an answer set that does not match
  exactly is indeterminate and routes to a human.

This is asserted explicitly by
`TestResultValidationDoesNotEnforceAnswerCompleteness`, so a change to that
behavior is a deliberate decision rather than an accident.

## 5. Boundary rules (unchanged)

- Providers MUST NOT define, interpret, or enforce SOP policy.
- SOP approval thresholds MUST NOT appear in this repository; they belong to
  `agentic-sop`.
- A provider failure, or an unknown/indeterminate result, is never an approval or
  a successful execution.

## References

- [Decision contract](decision-contract.md) — the authoritative batched shape.
- agentic-sop `docs/architecture/JEV-DECISION-CONTRACT.md` — the ratified Gate 3
  decision (D1–D7) and the bridge invariants.
- agentic-sop `internal/decision/bridge` — the deterministic bridge.
