# Operator Decision Record — Draft Entries for Decisions 1–6

**Type:** Decision-record draft (non-normative). It prepares a record entry for the first
six decisions of the [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md). It
records **no decision**, approves nothing, and nominates nobody.
**Status:** **DRAFT — NOT RECORDED.**
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
the operator records it. No name is supplied; every name is `<operator-supplied>`.

## 1. Summary

| # | Decision | Current state | Recommended option | Depends on | Approval recorded |
| --- | --- | --- | --- | --- | --- |
| 1 | Corpus owner | Not designated | One named human who is not an adjudicator; the operator may serve | — | **No** |
| 2 | Historical inclusion and exclusion | 0 strict, 10 partial, 2 verified-defect holds, 3 unverified holds, 8 excluded; six leads now extracted (2 proposed strict, 1 evidence-blocked, 1 implementation-only, 2 no artifact) | Confirm the exclusions and holds; classify the extracted leads explicitly; keep unverified entries out of every floor | 1 | **No** |
| 3 | Strict versus partial eligibility | Conservative reading applied; no rule recorded | Keep the conservative reading until the corpus can meet both floors without relaxing the recall denominator | 2, 4 | **No** |
| 4 | Proposed protocol amendment | PROPOSED, not applied | Defer until the corpus owner records independent support for each partial, then decide with the actual counts | 1, 3 | **No** |
| 5 | Independent seeder | Not designated; 0 of about 8 and 0 of about 6 cases | One named human who is not an adjudicator; record the sealed-manifest location and access list | — | **No** |
| 6 | Two independent human adjudicators | None identified; operator treated as an author | Two external humans who did not author or directly edit any corpus case; record the authorship checks | 5, 1 | **No** |

## 2. Decision 1 — Corpus owner

**Verified current state.** No corpus owner is designated. The protocol assigns the owner
the selection, determinability, and freeze record. No file in the repository names an owner
and the freeze record has no `corpus_owner` value.

**Recommended option.** Designate one named human as corpus owner with a recorded statement
that the person is not an adjudicator of any corpus case. The operator may serve, because
the protocol permits the operator to be the corpus owner but not an adjudicator.

**Dependencies.** None. Decisions 2 and 8 need a named owner to record the list and the
freeze.

**Independence constraint.** The owner may overlap with the seeder, analyst, and operator.
The owner may not be an adjudicator.

**Exact text to record (verbatim or equivalent):**

> Corpus owner: `<operator-supplied name>`. Role: selects and freezes the corpus, records
> each determinability judgment, and records the freeze. Independence: this person is not an
> adjudicator of any corpus case and is not a model. Recorded on `<date>`.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** The freeze record gains an owner, so the corpus can be treated as a
pre-registered artifact. Nothing is promoted.

**Consequence of deferral.** Workstream A stays at `NOT READY FOR CORPUS-OWNER FREEZE`.

**Explicitly not approved.** No name is proposed by this draft.

## 3. Decision 2 — Historical inclusion and exclusion

**Verified current state.** The manifest has 23 entries: 5 `VERIFIED`, 7 `PROPOSED`, 3
`UNVERIFIED`, 8 `EXCLUDED`. The proposed disposition is 0 strict, 10 partial, 2
verified-defect holds (`IDR-H-001`, `IDR-H-005`), 3 unverified holds (`IDR-E-006`,
`IDR-E-010`, `IDR-E-012`), and 8 excluded. The manifest's `hold` inclusion value is 5.

The six leads in the preparation plan are now extracted in
[HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md):

- `IDR-L-003` (`PLAN-09…` @ `4fb3b8a3`) and `IDR-L-004` (`docs/specs/REVIEW.md` @
  `5fce2b6a`) are **proposed strict** design defects — two plans, three defects. They are
  unadjudicated and not counted.
- `IDR-L-001` is extracted but its only later evidence is the artifact's own status line;
  it is evidence-blocked.
- `IDR-L-002` resolves to an implementation-only defect plus an open residual GAP; it is
  excluded as a design-corpus input.
- `IDR-L-005` and `IDR-L-006` have no single pre-defect design artifact.

The extraction also corrects a premise: `IDR-E-005` points at `ba7736d4`, which is a
post-fix revision that added the requirements. The exclusion stands as recorded.

**Recommended option.** Confirm the 8 exclusions and the 2 verified-defect holds; keep the
3 unverified entries out of every floor; classify each extracted lead explicitly — for
`IDR-L-003` and `IDR-L-004`, record **include-with-review** at most, not `include`, until
the independent support exists; keep `IDR-L-001` held; confirm `IDR-L-002`, `IDR-L-005`, and
`IDR-L-006` as excluded/no-artifact. Do not count a lead until the owner records its
classification.

**Dependencies.** Decision 1 (the owner records the list). Decision 3 determines how the 10
partial candidates and the newly extracted proposals are counted.

**Independence constraint.** The owner is not an adjudicator. The owner's determinability
judgment is a recorded judgment, not an adjudication.

**Exact text to record (per candidate ID):**

> Included: `<IDs, or "none">`.
> Included with corpus-owner review: `<IDs>`.
> Held, verified defect, no pre-defect input: `<IDs>`.
> Held, unverified: `<IDs>`.
> Excluded: `<IDs>` with one-line reason each.
> Extracted leads: `IDR-L-001` `<hold / exclude>`; `IDR-L-002` `<exclude>`; `IDR-L-003`
> `<include-with-review / hold>`; `IDR-L-004` `<include-with-review / hold>`; `IDR-L-005`
> `<exclude, no artifact>`; `IDR-L-006` `<exclude, no artifact>`.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** The recall and false-positive denominators become defined. A
contaminated or unclassified candidate stays out.

**Consequence of deferral.** The denominator stays undefined and the corpus cannot freeze.

**Explicitly not approved.** No candidate is promoted by this draft, and no count is changed.

## 4. Decision 3 — Strict versus partial eligibility

**Verified current state.** The conservative reading counts a partially determinable defect
toward neither floor. Under it the corpus is 0/15 strict and 10/15 partial-inclusive. No
eligibility rule is recorded. The two newly extracted proposals are strict by proposal only;
the owner has not recorded them.

**Recommended option.** Keep the conservative reading until the corpus can meet both floors
without relaxing the recall denominator. Do not let a partial defect enter the recall
denominator.

**Dependencies.** Decisions 2 and 4.

**Independence constraint.** The eligibility rule is applied by the corpus owner, who is not
an adjudicator.

**Exact text to record:**

> Eligibility rule in force: `conservative` — a partially determinable or hindsight-only
> defect counts toward neither the 15-plan floor nor the 15-determinable-defect floor; the
> recall denominator is the frozen determinable set.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** The freeze rule is defined, so the freeze decision is explicit
rather than implicit.

**Consequence of deferral.** The strict reading stands provisionally and the shortfall
remains 15/15.

## 5. Decision 4 — Proposed protocol amendment

**Verified current state.** [PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md)
is PROPOSED and not applied. It changes the plan-floor meaning, not the numeric thresholds.
Its own §5 shows that even with all 10 partial candidates frozen determinable the corpus
reaches 10 plans / 10 defects, a shortfall of 5 and 5.

**Recommended option.** Defer until the corpus owner records independent support for each
partial, then decide with the actual counts in hand. The amendment cannot raise either count
by itself.

**Dependencies.** Decisions 1 and 3. Applying it also requires the owner to record the
independent support for each frozen candidate.

**Exact text to record:**

> Amendment decision: `approve | reject | revise | defer`.
> If approve or revise, the exact normative wording to add to the assessment and the
> manifest fields that change: `<verbatim>`.
> If defer, the condition for reconsideration: `<condition>`.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** The plan-floor interpretation is settled. The defect floor is
unchanged either way.

**Consequence of deferral.** The conservative reading applies.

**Explicitly not approved.** The amendment is not applied and the numeric thresholds are not
changed by this draft.

## 6. Decision 5 — Independent seeder

**Verified current state.** No seeder is designated. No case is constructed: 0 of about 8
seeded and 0 of about 6 no-known-defect. No sealed manifest and no answer key exist. The
construction method is in
[CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md); it constructs
nothing.

**Recommended option.** Designate one human seeder who is independent of both adjudicators
and, preferably, of the corpus owner. Record the independence statement, the sealed-manifest
location outside the reviewer project directory, and the access list.

**Dependencies.** None to designate. Decision 6 depends on the seeder being distinct from
both adjudicators; decision 8 needs the sealed manifest and the access list.

**Independence constraint.** The seeder may overlap with the corpus owner, analyst, and
operator, but may not be an adjudicator, and may not adjudicate a case they wrote.

**Exact text to record:**

> Independent seeder: `<operator-supplied name>`. Independence: not an adjudicator of any
> corpus case; `<independence statement>`. Sealed manifest location:
> `<absolute path outside every reviewer project>`. Access list: `<roles or names>`.
> Case construction is authorized: `<yes / no>`.
>
> Approval block
> `approved: false`
> `identity: null`
> `date: null`

**Effect if recorded.** Seeded and control construction may begin under the specification.
It does not by itself create a case or an answer key.

**Consequence of deferral.** The experiment remains unpowered for false positives.

**Explicitly not approved.** No seeder is nominated and no case construction is authorized.

## 7. Decision 6 — Two independent human adjudicators

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
