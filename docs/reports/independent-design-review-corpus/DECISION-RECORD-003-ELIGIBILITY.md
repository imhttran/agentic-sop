# Decision Record 003 — Eligibility Rule (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved Decision 3 of the
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) and recorded the conservative
eligibility rule and the seeded-case interpretation. The decision is recorded here from the
operator's explicit instruction on 2026-11-21, in the repository's written decision-record
form established by [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md) and
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md). The prepared text in
[OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md) is not rewritten, and
no earlier evidence is made to imply that the decision was taken sooner.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — Decision 3, eligibility rule |
| Approver | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| Scope | EGM corpus eligibility |
| Eligibility rule | **`conservative`** |
| Partial defects | count toward neither minimum |
| Hindsight-only defects | count toward neither minimum |
| Recall denominator | the frozen determinable defect set |
| Seeded set | **additional**; does not count toward either historical minimum |
| Seeded / control cases constructed | **0 / 0** |
| Final eligibility | **NOT DETERMINED** |
| Independently adjudicated | **0** |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `d02bc96c` (2026-11-21), the merge of PR #596.

**Source of the decision:** the operator's explicit instruction in the session of
2026-11-21, prepared from
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §3.

## 1. Recorded decision

> Decision 3 — APPROVED
>
> Approver: Tom Tran. Corpus owner: Tom Tran.
>
> **Eligibility rule: `conservative`.**
>
> A partially determinable or hindsight-only defect counts toward neither the minimum of 15
> eligible plans nor the minimum of 15 determinable defects.
>
> The recall denominator is the frozen determinable defect set.
>
> **Seeded-case interpretation.** The seeded set is additional to the historical corpus.
> Seeded cases do not count toward either minimum: 15 eligible historical plans and 15
> determinable historical defects. No seeded or control case currently exists.
>
> This interpretation is recorded explicitly so that the experiment cannot silently use
> seeded cases to satisfy the historical minimums.
>
> This decision defines eligibility rules only. It does not promote any candidate, establish
> final determinability or independent adjudication, change candidate dispositions, freeze
> the recall denominator, or approve a protocol amendment.

Recorded from the operator's explicit instruction on 2026-11-21. The approver is Tom Tran.
No signature, time of day, or additional authorization is recorded; none was given.

## 2. Reconciliation with the registered protocol

The registered protocol
([EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3) says:

> Minimum to run: 15 determinable known defects (the recall denominator) across at least 15
> plans with known defects; the no-known-defect set is additional and does not count toward
> this floor.

It names only the **no-known-defect** set as additional. It does not state whether the
**seeded** set counts. This decision resolves that ambiguity conservatively:

- The seeded set is **additional** and does **not** count toward either historical minimum.
- The numeric minimums are unchanged (15 plans, 15 defects).
- The effect is **stricter**, not looser: the historical set alone must supply 15 plans and
  15 determinable defects. The historical target in §5.3 is 12–15, so 15 is the upper end of
  the recorded target, not a new threshold.
- Seeded defects remain recordable and reportable as a **separate stratum** for
  false-positive and calibration analysis; they are simply not counted toward the
  historical minimums.
- The protocol text is **unchanged**. This record is the operative clarification for the
  corpus freeze; it does not amend the protocol and does not approve the proposed Decision 4
  amendment.

**Conflict check:** no conflict with an explicit registered requirement was found. The
protocol is silent on the seeded set's status; this decision clarifies that silence in the
conservative direction and leaves every numeric minimum, arm, threshold, cost ceiling, and
stopping rule intact.

## 3. What this decision does and does not do

Established:

- The eligibility rule in force is `conservative`.
- Partial and hindsight-only defects count toward neither historical minimum.
- The recall denominator is the frozen determinable defect set.
- The seeded and no-known-defect sets are additional to the historical corpus.

Not established or authorized:

- No candidate is promoted to final eligibility.
- No final determinability is established; the corpus owner still records each basis.
- No independent adjudication occurs; the count remains **0**.
- No candidate disposition changes; the Decision 2 set is preserved.
- The recall denominator is **not frozen**; the corpus remains `NOT FROZEN`.
- No protocol amendment is approved; Decision 4 remains open.
- No seeded or control case is constructed.
- No seeder or adjudicator is designated.
- No threshold, cost ceiling, arm, or stopping rule changes.
- No model inference or experiment execution occurs.

## 4. Counts and shortfall (unchanged)

| Measure | Current state |
| --- | --- |
| Proposed strict | **3 plans / 4 defects** |
| Proposed partial | **11 plans / 11 defects** |
| Independently adjudicated | **0** |
| Final candidate promotions | **0** |
| Constructed seeded cases | **0** |
| Constructed controls | **0** |
| Historical plan minimum | **15** |
| Historical defect minimum | **15** |
| Strict shortfall (provisional arithmetic) | **12 plans / 11 defects** |

No partial, hindsight-only, seeded, or control case is counted toward either historical
minimum. The shortfalls are provisional arithmetic, not independently verified eligibility
counts.

## 5. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** |
| 3 | Strict versus partial eligibility | **APPROVED 2026-11-21** — this record |
| 4 | Proposed protocol amendment | **PENDING** |
| 5 | Independent seeder | **PENDING** |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost limits | **PENDING** |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** |

## 6. Consistency

- `candidates.json` is unchanged; no candidate is promoted or adjudicated.
- The Decision 2 dispositions are preserved.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.
- Freeze precondition 3 (the eligibility rule is recorded) is met. Preconditions 4–7 remain
  open, and the freeze is not authorized.
- The conditional PRD is unchanged.

## 7. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
