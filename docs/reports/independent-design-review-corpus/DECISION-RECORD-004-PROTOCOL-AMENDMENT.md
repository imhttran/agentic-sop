# Decision Record 004 — Proposed Protocol Amendment (Independent Design Review / EGM)

**Status: REJECTED (unnecessary) 2026-11-21.** The operator rejected the proposed corpus
eligibility amendment because Decision 3 already establishes the conservative rule. The
decision is recorded here from the operator's explicit instruction on 2026-11-21, in the
repository's written decision-record form established by
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md),
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md), and
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md). The proposal in
[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md) is preserved as historical
evidence and is not rewritten; its status line records the rejection.

| Layer | State |
| --- | --- |
| Operator decision | **REJECTED (unnecessary) 2026-11-21** |
| Approver | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| Amendment | **not applied** — the registered protocol is unchanged |
| Decision 3 | **remains in force** |
| Registered protocol SHA-256 | `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c` — unchanged |
| Eligibility rule | `conservative` (Decision 3) |
| Proposed strict / partial | 3 plans / 4 defects; 11 plans / 11 defects |
| Final eligibility / adjudication | 0 promoted / **0 adjudicated** |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `2e504cf8` (2026-11-21).

**Source of the decision:** the operator's explicit instruction in the session of
2026-11-21, prepared from
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §4,
[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md), and
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md).

## 1. Recorded decision

> Decision 4 — REJECTED (unnecessary)
>
> Approver: Tom Tran. Corpus owner: Tom Tran.
>
> The proposed eligibility amendment is rejected as unnecessary under Decision 3.
>
> The approved conservative rule already establishes that:
>
> - Partially determinable and hindsight-only defects count toward neither the
>   15-historical-plan floor nor the 15-determinable-historical-defect floor.
> - The recall denominator is the frozen determinable defect set.
> - Seeded cases are additional and count toward neither historical minimum.
>
> The proposed amendment would change no count under these rules.
>
> The registered protocol remains unchanged.

Recorded from the operator's explicit instruction on 2026-11-21. The approver is Tom Tran.
No signature, time of day, or additional authorization is recorded; none was given.

## 2. Approved rationale

The operator recorded these reasons:

1. Decision 3 already excludes partially determinable and hindsight-only defects from both
   historical minimum floors.
2. The recall denominator is the frozen determinable defect set.
3. A historical plan cannot satisfy the plan floor without an eligible determinable defect.
4. Seeded cases are additional and cannot satisfy either historical minimum.
5. The proposed amendment would change no eligible count under the approved rule.
6. Modifying the registered protocol would introduce redundant normative text without
   improving experimental validity.

## 3. Verification against the proposal and the protocol

- **The proposal's rule.** `PROPOSED-PROTOCOL-AMENDMENT.md` §2 would let a plan count toward
  the plan floor only when it has at least one determinable known defect, exclude partial and
  hindsight-only defects from both floors, and keep the recall denominator as the frozen
  determinable set.
- **Decision 3.** It records the same exclusions and the same denominator. A plan carrying
  only partial defects therefore counts toward neither floor under either text, and a plan
  with one determinable defect counts once under both. The two rules do not differ on any
  count.
- **The current corpus.** Under both, the proposed strict corpus is 3 plans / 4 defects and
  the 11 proposed partials remain outside the denominator. The strict shortfall is 12 plans /
  11 defects.
- **The registered protocol.** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
  is byte-identical; its SHA-256 is
  `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c`, unchanged before and
  after this record.
- **No conflict.** The rejection does not contradict an explicit registered requirement; it
  declines to add redundant normative text.

The proposal's own §5 and §6 figures are retained as written; they predate Decisions 2 and
3 (which approved the historical dispositions and the eligibility rule). The current counts
are those in the table above.

## 4. What this decision does and does not do

Established:

- The proposed amendment is **rejected**, not pending, approved, implemented, or deferred.
- Decision 3 remains in force; the conservative eligibility rule is unchanged.
- The registered protocol and all numeric minimums are unchanged.

Not authorized or changed:

- No repeal or weakening of Decision 3.
- No change to historical eligibility requirements.
- No change to the registered experiment.
- No replacement amendment.
- No candidate promotion or adjudication.
- No seeded or control case construction.
- No seeder or adjudicator designation.
- No corpus freeze.
- No threshold, arm, cost-ceiling, or stopping-rule change.
- No model inference or experiment execution.

## 5. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** |
| 3 | Strict versus partial eligibility | **APPROVED 2026-11-21** |
| 4 | Proposed protocol amendment | **REJECTED (unnecessary) 2026-11-21** — this record |
| 5 | Independent seeder | **PENDING** |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost limits | **PENDING** |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** |

## 6. Consistency and freeze preconditions

- `candidates.json` is unchanged; no candidate is promoted or adjudicated.
- The proposed strict corpus remains 3 plans / 4 defects and the proposed partial corpus
  11 plans / 11 defects; final promotions and adjudications remain 0.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.
- `FREEZE-PROCEDURE.md` §2 precondition 4 is "The amendment decision is recorded (decision
  4)." The decision is now recorded (rejected), so that precondition is met. Preconditions
  5–7 remain open, and the freeze is not authorized.
- The conditional PRD is unchanged.

## 7. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
