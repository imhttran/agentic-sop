# Operator Decision Record — Draft Entries for Decisions 1–6

**Type:** Decision-record draft (non-normative). It prepared record entries for the first six
decisions of the [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md). Decisions 1–3
were approved, Decision 4 was rejected, and Decision 5 was approved on 2026-11-21 in
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md),
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md),
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md),
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md), and
[DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md); Decision 6 remains an unrecorded
draft.
**Status:** **PARTIALLY RECORDED — Decisions 1–3 approved, Decision 4 rejected, and Decision 5 approved 2026-11-21; Decision 6 remains draft.**
**Baseline:** `origin/main` @ `5a94dfe1`.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).
**Manifest:** [candidates.json](candidates.json).
**Lead extraction:** [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md),
[lead-extraction.json](lead-extraction.json).
**Case construction:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md).
**Readiness:** [READINESS.md](READINESS.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md).

## 0. How to use this draft

Each section gives the current verified state, the recommended option, the dependencies,
the exact text the operator would record, and an approval block that is deliberately empty.
Nothing here is an approval. A merged pull request is not an approval of any item.

This draft follows the repository's decision-record convention: the status line, the
exact scope, and the effect are recorded up front, and an approval date appears only when
the operator records it. Decision 1 names Tom Tran; the remaining drafts use `<operator-supplied>`.

## 1. Summary

| # | Decision | Current state | Recommended option | Depends on | Approval recorded |
| --- | --- | --- | --- | --- | --- |
| 1 | Corpus owner | **Designated: Tom Tran (2026-11-21)** | One named human who is not an adjudicator; the operator may serve | — | **Yes** — DECISION-RECORD-001 |
| 2 | Historical inclusion and exclusion | **Recorded 2026-11-21** — see DECISION-RECORD-002 | Confirm the exclusions and holds; classify the extracted leads explicitly; keep unverified entries out of every floor | 1 | **Yes** — DECISION-RECORD-002 |
| 3 | Strict versus partial eligibility| **Recorded 2026-11-21** — conservative; seeded set additional| Keep the conservative reading until the corpus can meet both floors without relaxing the recall denominator| 2, 4       | **Yes** — DECISION-RECORD-003|
| 4 | Proposed protocol amendment| **REJECTED (unnecessary) 2026-11-21**| Rejected as unnecessary under Decision 3| 1, 3       | **Rejected** — DECISION-RECORD-004|
| 5 | Independent seeder| **Recorded 2026-11-21** — seeder Tom Tran (also corpus owner); construction not authorized| Designate a human independent of the adjudicators| None; needed by 6, 8| **Yes** — DECISION-RECORD-005|
| 6 | Two independent human adjudicators | None identified; operator treated as an author | Two external humans who did not author or directly edit any corpus case; record the authorship checks | 5, 1 | **No** |

## 2. Decision 1 — Corpus owner

**Status: APPROVED 2026-11-21.** Recorded in
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md); the preparation analysis below is
preserved.

**Verified current state (at preparation).** No corpus owner was designated. The protocol
assigns the owner the selection, determinability, and freeze record. **Recorded:** Tom Tran,
2026-11-21.

**Recorded option.** Tom Tran was designated with the recorded statement that he is not an
adjudicator of any corpus case. The protocol permits the operator to be the corpus owner but
not an adjudicator.

**Dependencies.** None. Decisions 2 and 8 need a named owner to record the list and the
freeze.

**Independence constraint.** The owner may overlap with the seeder, analyst, and operator.
The owner may not be an adjudicator.

**Recorded text.** The recorded decision, operator identity, date, and independence statement
are in [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md) §1.

**Effect of the recorded decision.** The freeze record gains an owner, so the corpus can be
treated as a pre-registered artifact. Nothing is promoted.

**Consequence of deferral (no longer applicable).** Workstream A would have stayed at
`NOT READY FOR CORPUS-OWNER FREEZE`.

**Recorded elsewhere.** The designation comes from the operator's decision in
DECISION-RECORD-001; this draft proposes no other name.

## 3. Decision 2 — Historical inclusion and exclusion

**Status: APPROVED 2026-11-21.** Recorded in
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md); the preparation
analysis below is preserved.

**Verified current state (at preparation).** The manifest has 23 entries: 5 `VERIFIED`, 7
`PROPOSED`, 3 `UNVERIFIED`, 8 `EXCLUDED`. The proposed disposition was 0 strict, 10 partial,
2 verified-defect holds (`IDR-H-001`, `IDR-H-005`), 3 unverified holds (`IDR-E-006`,
`IDR-E-010`, `IDR-E-012`), and 8 excluded. The manifest's `hold` inclusion value is 5.

The six leads in the preparation plan, plus two additional leads found after PR #587, are
now extracted in [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md):

- `IDR-L-003` (`PLAN-09…` @ `4fb3b8a3`) and `IDR-L-004` (`docs/specs/REVIEW.md` @
  `5fce2b6a`) are **proposed strict** design defects — two plans, three defects. They are
  unadjudicated and not counted.
- `IDR-L-007` (`docs/specs/PLAN-HISTORICALIZATION.md` @ `fcf53888`) is a **proposed
  strict** design defect: one archive slot is keyed to a plan id that is not unique across
  activations or source directories, so same-named or re-activated plans collide.
- `IDR-L-008` (`docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f56`) is a **proposed partial**
  design limitation: the evidence hash covers the whole document, not the returned chunk,
  so the returned evidence is not checkable.
- `IDR-L-001` is extracted but its only later evidence is the artifact's own status line;
  it is evidence-blocked.
- `IDR-L-002` resolves to an implementation-only defect plus an open residual GAP; it is
  excluded as a design-corpus input.
- `IDR-L-005` and `IDR-L-006` have no single pre-defect design artifact.

The extraction also corrects a premise: `IDR-E-005` points at `ba7736d4`, which is a
post-fix revision that added the requirements. The exclusion stands as recorded.

**Recorded option.** The 8 exclusions and 2 verified-defect holds were confirmed; the 3
unverified entries stay out of every floor; `IDR-L-003`, `IDR-L-004`, `IDR-L-007`, and
`IDR-L-008` are include with review; `IDR-L-001` is held; `IDR-L-002`, `IDR-L-005`, and
`IDR-L-006` are excluded. No candidate is promoted to final eligibility.

**Dependencies.** Decision 1 (the owner records the list). Decision 3 determines how the 10
partial candidates and the newly extracted proposals are counted.

**Independence constraint.** The owner is not an adjudicator. The owner's determinability
judgment is a recorded judgment, not an adjudication.

**Recorded text.** The exact dispositions, counts, holds, and exclusions are in
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md) §2.

**Effect if recorded.** The recall and false-positive denominators become defined. A
contaminated or unclassified candidate stays out.

**Consequence of deferral (no longer applicable).** The denominator would stay undefined and
the corpus could not freeze.

**Recorded elsewhere.** The dispositions are in DECISION-RECORD-002; no candidate is promoted
to final eligibility by this draft.

## 4. Decision 3 — Strict versus partial eligibility

**Status: APPROVED 2026-11-21.** Recorded in
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md); the preparation analysis below is
preserved.

**Verified current state (at preparation).** The conservative reading counts a partially
determinable defect toward neither floor. Under it the corpus was 0/15 strict and 10/15
partial-inclusive. No eligibility rule was recorded. The newly extracted proposals were
strict by proposal only.

**Recorded option.** The conservative rule was recorded, with the seeded set additional.

**Dependencies.** Decisions 2 and 4.

**Independence constraint.** The eligibility rule is applied by the corpus owner, who is not
an adjudicator.

**Recorded text.** The rule and the seeded-case interpretation are in
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md) §1.

**Effect of the recorded decision.** The freeze rule is defined, so the freeze decision is
explicit rather than implicit.

**Consequence of deferral (no longer applicable).** The strict reading would have stood
provisionally and the strict shortfall would have remained 12 plans / 11 defects.

## 5. Decision 4 — Proposed protocol amendment

**Status: REJECTED (unnecessary) 2026-11-21.** Recorded in
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md); the preparation analysis
below is preserved.

**Verified current state (at preparation).**
[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md) was PROPOSED and not
applied. It changes the plan-floor meaning, not the numeric thresholds. Its own §5 shows
that even with all 10 partial candidates frozen determinable the corpus reaches 10 plans /
10 defects, a shortfall of 5 and 5.

**Recorded option.** Rejected as unnecessary under Decision 3.

**Dependencies.** Decisions 1 and 3. Applying it also requires the owner to record the
independent support for each frozen candidate.

**Recorded text.** The rejection and its rationale are in
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md) §1–§2.

**Effect of the recorded decision.** The plan-floor interpretation is settled; the defect
floor and the registered protocol are unchanged.

**Consequence of deferral (no longer applicable).** The conservative reading would have
applied.

**Recorded elsewhere.** The amendment is rejected in DECISION-RECORD-004; it is not applied
and no numeric threshold changes.

## 6. Decision 5 — Independent seeder

**Status: APPROVED 2026-11-21.** Recorded in
[DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md); the preparation analysis below is
preserved.

**Verified current state (at preparation).** No seeder was designated. No case was
constructed: 0 of about 8 seeded and 0 of about 6 no-known-defect. No sealed manifest and no
answer key existed. The construction method is in
[CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md); it constructs
nothing.

**Recorded option.** Tom Tran was designated as seeder (also corpus owner). The independent
case checker, the sealed-manifest location, and construction authorization remain open.

**Dependencies.** None to designate. Decision 6 depends on the seeder being distinct from
both adjudicators; decision 8 needs the sealed manifest and the access list.

**Independence constraint.** The seeder may overlap with the corpus owner, analyst, and
operator, but may not be an adjudicator, and may not adjudicate a case they wrote.

**Recorded text.** The seeder, the overlap limitation, the independence restrictions, and
`case_construction_authorized: false` are in
[DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md) §1–§5.

**Effect of the recorded decision.** The seeder is designated. Seeded and control construction
may **not** begin: the case checker, the sealed manifest, and the explicit construction
authorization remain open.

**Consequence of deferral (no longer applicable).** The experiment would remain unpowered for
false positives.

**Recorded elsewhere.** The seeder is designated in DECISION-RECORD-005; construction is **not**
authorized.

## 7. Decision 6 — Two independent human adjudicators

**Preparation package:** [DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md).

**Procedural clarifications:** **APPROVED 2026-11-21** — [DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md); Decision 6 adjudicator designation remains PENDING.

**Verified current state.** No adjudicator is identified. The repository shows only the
operator as a human Git identity, and the operator is treated conservatively as an author or
direct editor of the candidate plans. The protocol requires two distinct humans who are not
authors of any corpus case; a model may not adjudicate. Independently adjudicator-confirmed
determinability remains **0**.

**Recommended option.** Designate two external humans who did not author or directly edit
any corpus case, and record the authorship evidence for each. If two cannot be found, record
that the best achievable outcome is INCONCLUSIVE.

**Dependencies.** Decisions 1 and 5. Both adjudicators must differ from the seeder of any
defect they adjudicate and from the corpus owner.

**Independence constraint.** The two adjudicators must be distinct from each other, from the
seeder, from the corpus owner, and from the author of every case they adjudicate. The second
adjudicator re-adjudicates the larger of 20 findings or a random 20% sample.

**Exact text to record:**

> First adjudicator: `<operator-supplied name>`. Authorship check: `<evidence>`.
> Second adjudicator: `<operator-supplied name>`. Authorship check: `<evidence>`.
> Second-adjudication sampling rule: `<larger of 20 findings or a random 20% sample>`.
> Adjudicator-visible defect list: `<blinded export derived from the sealed manifest;
> defect ids and descriptions only, with no stratum or arm/run mapping>`.
> Conflict-of-interest record: `<statement or "none">`.
> If two cannot be obtained, best attainable outcome: `INCONCLUSIVE`.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** Adjudication can convert findings into labelled data. Without two
independent adjudicators the experiment cannot return PASS.

**Consequence of deferral.** Any run would be INCONCLUSIVE at best.

**Explicitly not approved.** No adjudicator is nominated and no availability is asserted.

## 8. What this draft does not do

- It records no decision and no approval.
- It nominates no corpus owner, seeder, analyst, checker, or adjudicator.
- It does not assert that any person is available or independent.
- It does not promote, freeze, or count a candidate.
- It does not apply the amendment, change a threshold, or construct a case.
- It does not authorize inference or a run.

## 9. Empty record template

```yaml
operator_decision_record:
  version: 1
  status: DRAFT              # DRAFT | RECORDED
  decision: <1..6>
  decision_name: <name>
  recorded_identity: null
  recorded_date: null
  approved: false
  option: null               # the operator's choice, verbatim
  exact_scope: null
  dependencies: [<decisions>]
  effect_on_validity: <one line>
  deferral_consequence: <one line>
  evidence: [<paths>]
```
