# JEV-206 — Adapter Compatibility Report (Julia-1 / Nimble)

**Task:** JEV-206 (Adapter Compatibility), `plan-jev-wave-2-hardening`.
**Status:** Verification complete — compatible at the bridge boundary; no adapter
wire-contract change required.
**Live inference:** none. All verification uses in-process fixtures and the
repository's captured SystemOne fixture; no network, model, or credential is used.

## 1. Evidence header

- agentic-sop revision: `a5489fbb6bbcd514bced0548decb3c5edc118f82`
- sop-decision-adapters revision: `057604ee5be77e4f9250710120d4aaf66d4b85f7`
- Go toolchain: `go1.27.1 darwin/arm64`
- Bridge under test: `agentic-sop/internal/decision/bridge` (Gate 3, M3)
- Adapter contracts under test: `sop-decision-adapters/decision` (batched
  `DecisionRequest`/`DecisionResult`) and its Julia-1 and Nimble providers

## 2. Contract-test commands and results

These run the **real** adapter code (real Julia/Nimble normalization with
deterministic in-process fixtures) in the owning repository.

| Adapter                    | Command                                                                                 | Exit status |
| -------------------------- | --------------------------------------------------------------------------------------- | ----------- |
| Julia-1                    | `go test -count=1 ./internal/providers/julia/`                                          | 0           |
| Nimble                     | `go test -count=1 ./internal/providers/nimble/`                                         | 0           |
| shared contract            | `go test -count=1 ./decision/`                                                          | 0           |
| combined (registered form) | `go test -count=1 ./decision/ ./internal/providers/julia/ ./internal/providers/nimble/` | 0           |

All three packages pass. A passing per-repository unit suite is **not** by itself
cross-repository compatibility evidence; §4 connects the two sides with real code.

## 3. Julia-1 verification

- Command: `go test -count=1 ./internal/providers/julia/`
- Exit status: 0
- Result: ok — real normalization passes for CHOICE, BOOLEAN, and SCORE
  (`TestNormalizeOutputsHappyPath`, `TestNormalizeOutputsScoreQuestion`), range
  and out-of-range rejection (`TestNormalizeOutputsBooleanOutOfRange`,
  `TestNormalizeOutputsRejections`), option-limit rejection, and typed
  `ErrorKind` classification (`TestProviderDecide*`). The adapter defines no SOP
  policy and cannot approve.

## 4. Nimble verification

- Command: `go test -count=1 ./internal/providers/nimble/`
- Exit status: 0
- Result: ok — the real production response parser passes over the captured
  fixture (`TestGoldenSystemOneResponseParses` →
  `tests/fixtures/nimble/systemone_response.json`), covering CHOICE + `noul`
  (BOOLEAN) + probabilities/confidence, plus typed error classification and the
  end-to-end HTTP transport path (`TestProviderDecideEndToEnd`). The adapter
  defines no SOP policy and cannot approve.

## 5. Cross-repository wire verification (real code, both sides)

Method — no hand-written mock contracts:

1. **Emitter (sop-decision-adapters, exit status 0).** A temporary program drives
   the **real** adapter code and writes wire documents:
   - `julia.NormalizeOutputs` (the production normalizer) for CHOICE, BOOLEAN, SCORE;
   - the **real** `nimble.Provider` with the production `HTTPTransport` against an
     `httptest` backend serving the repository's captured SystemOne fixture;
   - the real `julia.Provider.Decide` failure classification.
     Command: `go run ./tmpcompatemit <outdir>` — exit status `0`.
2. **Consumer (agentic-sop, exit status 0).** A temporary program decodes those
   exact documents with `encoding/json` **`DisallowUnknownFields`** into the real
   `bridge.Request`/`bridge.Result`, then runs the real `bridge.Build` +
   `Interpret`, `decision.Route`, and `bridge.MapError`.
   Command: `go run ./tmpcompatconsume <outdir>` — exit status `0`.

`DisallowUnknownFields` succeeded on every document: the deployed batched wire
keys (`state`, `questions[]`, `metadata`; `provider`, `model`, `answers{}`,
`usage`) are exactly the keys the bridge models — no unknown fields, no missing
required fields.

Observed canonical interpretation (real adapter output → real bridge):

| Adapter output                                    | Bridge result    | Route          |
| ------------------------------------------------- | ---------------- | -------------- |
| Julia CHOICE `high` (confidence 0.9)              | `HIGH` / 0.900   | `HUMAN`        |
| Julia BOOLEAN yes-probability 0.8 (threshold 0.5) | `HIGH` / 0.800   | `HUMAN`        |
| Julia SCORE 1 (range 0–2)                         | `MEDIUM` / 0.500 | `STRONG_MODEL` |
| Nimble CHOICE `HIGH` (confidence 0.835)           | `HIGH` / 0.835   | `HUMAN`        |

- The emitted request (`decision.DecisionRequest`) decodes losslessly into
  `bridge.Request` (`state`, `questions[].{id,type,criteria,choices}`).
- Exact question-ID / answer-set identity: each single-answer result carries the
  one requested ID; the bridge rejects any mismatch (fail closed).
- Choice/boolean/score semantics and confidence/probability handling match §16:
  an explicit valid confidence is used directly; a bare boolean is only reduced
  through the capability's threshold; a raw score is only reduced through the
  capability's normalization.
- Deterministic routing: `HIGH` → `HUMAN` (never auto-approved), `MEDIUM` →
  `STRONG_MODEL`, `LOW` → `SMALL_MODEL`; the human-approval authority is preserved.

## 6. Failure-path evidence (fail-closed)

Real Julia `Provider.Decide` failure kinds (from the emitter) and the real
`bridge.MapError` mapping:

| Situation                                | Adapter `ErrorKind`  | `bridge.MapError`             |
| ---------------------------------------- | -------------------- | ----------------------------- |
| runner error / backend down              | `provider_failure`   | `decision.ErrProviderFailure` |
| missing answer for a requested question  | `malformed_response` | `decision.ErrInvalidResult`   |
| invalid request (choice without choices) | `invalid_request`    | `decision.ErrInvalidResult`   |

A batched **multi-answer** result (real Nimble, 3 answers) is outside the bridge's
cardinality-1 contract and **fails closed**:

```
nimble_batched.json ok: fail-closed as expected:
  decision: invalid provider result: expected exactly one answer, got 3
```

No failure path produces an approval or a successful decision.

## 7. M3 profile recognition and version stamping

- The bridge recognized the Julia and Nimble profiles by **validating schema,
  question identity, answer type, and capability semantics** — the provider name
  is a correlation hint, not the sole key (Gate 3 constraint 4). Unknown/absent/
  ambiguous profiles are rejected.
- The profile **version is stamped by the bridge** (M3); the wire documents carry
  no `contract_version`, matching the deployed adapter contract. No adapter field
  was added.

## 8. Compatibility gaps and limitations

1. **Cardinality.** The bridge issues one canonical decision per call
   (cardinality 1). The deployed adapters are batched (many questions per call).
   The bridge must therefore call an adapter with exactly the questions it will
   consume; a multi-answer result is rejected (fail closed). This is a bounded,
   documented limitation, not a wire incompatibility.
2. **Nimble reachability.** Nimble's native normalization takes unexported types,
   so it is exercised in-package (golden + end-to-end) and via the real
   `Provider` over the production HTTP transport in the emitter — not by a direct
   cross-module `Normalize*` call.
3. **No wire version (M3).** Version/identity is a SOP-side concept; adapters add
   nothing. Existing unversioned clients keep working.
4. **`request_id` absent** in both the adapter profile and the command DTO
   (deferred, Gate 3 D5).
5. **`DecisionResult.Validate` does not enforce per-question answer completeness**
   — the bridge does. (Pinned by `TestResultValidationDoesNotEnforceAnswerCompleteness`.)

## 9. Scope: contract compatibility vs production runtime wiring

This task verifies **cross-repository contract compatibility**: that the real
adapter wire documents (Julia-1, Nimble) decode into, and are correctly
interpreted by, the real SOP bridge, with matching semantics and fail-closed
behavior. It does **not** exercise **production runtime wiring** — SOP actually
invoking the Julia-1/Nimble adapters as a live, configured decision provider in a
governed run. That gap remains:

- No production path yet calls `sop-decision-adapters` as the `command` decision
  provider with an M3 profile; the bridge is a committed, unit- and wire-verified
  component that is not yet wired into a live SOP run.
- No JEV decision provider is added (deliberate; Gate 3 D2/D6).
- Live-provider dogfood (JEV-207, Gate 4) and enabling the layer by default
  (Gate 5) are out of scope here.

Compatibility is therefore established at the **contract boundary**, not at the
production runtime boundary.

## 10. Conclusion and recommendation

The real committed contracts are compatible at the bridge boundary. Julia-1 and
Nimble normalize CHOICE/BOOLEAN/SCORE correctly, carry confidence/probability in
range, classify failures by a stable `ErrorKind`, and never carry SOP policy;
the real bridge decodes their wire documents losslessly, maps them to canonical
evidence, routes deterministically, and fails closed on every failure class and on
out-of-contract cardinality.

Recommendation: **no adapter wire-contract change and no new JEV decision
provider**. Proceed with the cardinality-1 bridge contract. Enabling JEV by default
remains Gate 5; live-provider dogfood remains Gate 4 (JEV-207).

## 11. References

- `docs/architecture/JEV-DECISION-CONTRACT.md` (Gate 3, ratified)
- `docs/plans/PLAN-JEV-Wave-2-Hardening.md` (JEV-206)
- sop-decision-adapters `docs/specs/decision-contract.md`, `docs/specs/bridge-profile.md`
