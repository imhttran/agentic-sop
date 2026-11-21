# Decision Record 005 — Independent Seeder (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved Decision 5 and designated **Tom Tran**
as seeder, with the same person serving as corpus owner. The decision is recorded here from
the operator's explicit instruction on 2026-11-21, in the repository's written
decision-record form established by Decisions 1–4. The prepared text in
[OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md) is not rewritten, and
no earlier evidence is made to imply that the decision was taken sooner.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — Decision 5, seeder designation |
| Approver | **Tom Tran** |
| Seeder | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| Role overlap | **Same person** — permitted by protocol §5.4; recorded as a limitation |
| Independent case checker | **PENDING — NOT DESIGNATED** |
| Sealed-manifest location | **PENDING — NOT SELECTED** |
| `case_construction_authorized` | **false** |
| Seeded / control cases constructed | **0 / 0** |
| Sealed answer key created | **NO** |
| Independent adjudicators | **None designated** |
| Registered protocol SHA-256 | `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c` — unchanged |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `38e20a52` (2026-11-21), the merge of PR #601.

**Source of the decision:** the operator's explicit instruction in the session of
2026-11-21, prepared from
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §5 and
[OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md) §6.

## 1. Recorded decision

> Decision 5 — APPROVED
>
> Seeder: Tom Tran. Corpus owner: Tom Tran.
>
> The same individual serves as corpus owner and seeder. This overlap is explicitly
> permitted by protocol §5.4 and is accepted as a documented limitation.
>
> Independent case checker: **not designated**.
>
> Sealed-manifest location: **not selected**.
>
> `case_construction_authorized: false`.
>
> This approval authorizes governance documentation only. It does not authorize Decision 6,
> case construction, corpus freeze, inference, or experiment execution.

Recorded from the operator's explicit instruction on 2026-11-21. The approver is Tom Tran.
No signature, time of day, or additional authorization is recorded; none was given.

## 2. Role separation and independence restrictions

The registered protocol §5.4 permits the corpus owner, seeder, analyst, and operator to
overlap, and forbids any of them from being an adjudicator. The overlap here is **valid
because the protocol permits it**, not because the two roles are independent. The seeder is
**not** independent of the corpus owner.

Recorded restrictions:

1. The seeder must not serve as an independent adjudicator.
2. The seeder may not adjudicate any case they construct.
3. Both Decision 6 adjudicators must be independent of the seeder.
4. The seeder must not influence independent adjudication outcomes.
5. The seeded answer key must remain inaccessible to reviewers and adjudicators.
6. The role overlap must remain visible in experiment governance and reporting.

## 3. Independent case checker — PENDING, NOT DESIGNATED

A separate qualified human must independently check the seeded and no-known-defect cases
under the registered protocol and
[CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md). The checker must
not be the seeder and must not adjudicate the cases they check.

No checker is designated by this record, and none is invented. Until a checker is
designated and their check is recorded, no case counts and construction is not authorized.

## 4. Sealed manifest — PENDING, NOT SELECTED

No sealed-manifest location is selected, no absolute path is invented, and no placeholder
manifest containing answers is created.

The future sealed manifest must reside outside:

- Git;
- reviewer projects;
- exported reviewer trees;
- model-accessible workspaces.

The intended access policy is limited to the authorized seeder, case checker, analyst, and
corpus owner roles. Adjudicators and reviewers must not receive the sealed answer key; the
adjudicators receive only the blinded defect list defined in the case-construction
specification.

## 5. Construction authorization

`case_construction_authorized: false`

This decision does not authorize constructing seeded cases, no-known-defect controls, or
sealed answer keys. Seeded and control counts remain **0 / 0**, and no answer key exists.
Designation alone does not satisfy the remaining case-construction prerequisites: the case
checker, the sealed-manifest location, and the explicit construction authorization.

## 6. What this decision does and does not do

Established:

- One seeder is designated: **Tom Tran**, who is also the corpus owner.
- The role overlap is recorded as a limitation, not as independence.
- The independence restrictions above are recorded.

Not established or authorized:

- No Decision 6 adjudicator is designated.
- No independent case checker is designated.
- No sealed-manifest location is selected; no answer key exists.
- No seeded or control case is constructed.
- No corpus freeze is recorded.
- No threshold, arm, cost ceiling, or protocol text changes.
- No model inference or experiment execution occurs.
- Decisions 6–9 remain open.

## 7. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** |
| 3 | Strict versus partial eligibility | **APPROVED 2026-11-21** |
| 4 | Proposed protocol amendment | **REJECTED (unnecessary) 2026-11-21** |
| 5 | Independent seeder | **APPROVED 2026-11-21** — this record |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost limits | **PENDING** |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** |

## 8. Freeze-precondition status

`FREEZE-PROCEDURE.md` §2 precondition 5 is "The seeder is designated (decision 5)." Its
**designation subcondition is met**: Tom Tran is designated.

The prerequisite is **not fully satisfied**. The independent case checker, the sealed
manifest and access list (§3.4), and the explicit construction authorization remain open, and
no case is constructed. Preconditions 1–4 are met; precondition 5 is partially met;
preconditions 6–7 remain open. The freeze is not authorized.

## 9. Consistency

- `candidates.json` is unchanged; the proposed strict corpus remains 3 plans / 4 defects and
  the proposed partial corpus 11 plans / 11 defects.
- Final candidate promotions remain **0**; independently adjudicated remain **0**.
- The registered protocol is byte-identical; its SHA-256 is unchanged.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.
- The conditional PRD is unchanged.

## 10. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
