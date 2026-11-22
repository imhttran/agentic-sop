# Experiment Deferral under Protocol §5.3 (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21 — the historical-corpus experiment is DEFERRED.** The operator
approved **Option B** from [HISTORICAL-CORPUS-OPERATOR-HANDOFF.md](HISTORICAL-CORPUS-OPERATOR-HANDOFF.md) §4.
The deferral is a recorded outcome, not an authorization: it approves no candidate, freezes
nothing, and substitutes no synthetic case for historical evidence.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — Option B, defer |
| Approver | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| Disposition | **DEFERRED** (protocol §5.3) |
| Historical minimum | **15 plans / 15 determinable defects — unchanged, not lowered** |
| Historical recall comparison | **INCONCLUSIVE** |
| Approved eligible candidates | **0 / 0** |
| Proposed under the recorded rulings | **5 plans / 6 defects** (shortfall **10 / 9**) |
| Corpus status | **NOT FROZEN** |
| Decision 8 (freeze) | **NOT AUTHORIZED** |
| Decision 9 (execution) | **NOT AUTHORIZED** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `bdd067ee` (2026-11-21), the merge of PR #641. PRs #640 and #641 are
merged and the registered protocol is byte-identical (SHA-256 `abba919e…`).

**Source of the decision:** the operator's explicit instruction in the session of 2026-11-21,
approving Option B deferral together with the five eligibility rulings recorded in
[DECISION-003-ELIGIBILITY-RULINGS-APPROVAL.md](DECISION-003-ELIGIBILITY-RULINGS-APPROVAL.md).

## 1. Recorded disposition

> **Historical corpus — RECORDED as below the floor.** On the evidence in this repository, the
> historical set does not reach 15 plans and 15 determinable defects under the conservative rules
> (proposed 5 / 6; approved 0 / 0). Under protocol §5.3 the run is deferred and the floor is not
> lowered. Any recall comparison would be INCONCLUSIVE. No seeded case replaces a historical case.
> This is revisited only if an independent historical evidence source is authorized.

Recorded from the operator's explicit instruction on 2026-11-21. The approver is Tom Tran. No
signature, time of day, or additional authorization is recorded; none was given.

## 2. Protocol basis (verbatim)

[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3:

> If replacements cannot reach the 15-determinable-defect floor, the run is deferred; the floor is
> not lowered.

> Minimum to run: 15 determinable known defects (the recall denominator) across at least 15 plans
> with known defects; the no-known-defect set is additional and does not count toward this floor.
> Below the floor, every recall comparison is INCONCLUSIVE.

The deferral is therefore the outcome the protocol already specifies for this evidence; this record
does not create a new rule, change a threshold, or amend the protocol. The "target 12-15" phrasing
in the same section is the recorded target range; **15 is its upper end and the minimum to run**,
not a threshold introduced here.

**Why the floor is not met.** Under the recorded conservative rulings the historical set is
**5 plans / 6 defects** (`IDR-L-003`, `IDR-L-004`, `IDR-L-007`, `GH3-001a`, `GH-013`), a shortfall
of **10 plans and 9 defects**. Approved eligible remains **0 / 0**. The permissive arithmetic
ceilings (C2 13 / 15) depend on evidence the recorded rulings exclude and are not available.

## 3. What this deferral does and does not do

Does:

- Records the historical set as below the registered floor and the experiment as **deferred**.
- Records that any historical recall comparison is **INCONCLUSIVE**.
- Leaves the **15 plans / 15 determinable defects** minimum unchanged.
- Leaves the Decision 2 dispositions and the candidate manifest untouched.

Does **not**:

- Lower the minimum, reinterpret §5.3, or amend the protocol.
- Substitute seeded or synthetic cases for historical ones, or count them toward either minimum.
- Freeze the corpus, freeze the recall denominator, or authorize instruction-block finalization.
- Authorize execution, spending, or any provider call; the approved 613-call ceiling authorizes no
  spending by itself.
- Approve any candidate, determinability finding, or adjudication.
- Establish H1 or H2, or any partial conclusion about either.
- Approve Decisions 8 or 9, or alter Decisions 1–7.
- Authorize case construction or change `case_construction_authorized: false`.

## 4. Reopening conditions

The historical set is reopened only on one of:

1. an **operator-authorized independent evidence source** (named repositories or materials, a
   data-handling rule, and the same §5.3 evidence forms);
2. a **new independent human review record** for an artifact already listed;
3. a **ruling that makes already-listed leads count**, together with their line-level human
   verification.

Until then, historical discovery in this repository stops. Broad re-sweeps have already returned no
further plan-level candidate across four passes.

## 5. Effect on the remaining decisions

| Decision | Effect of the deferral |
| --- | --- |
| 1–4 | **None** — owner, dispositions, eligibility rules, and the rejected amendment stand |
| 5 | **None** — D5-A, D5-B1–B4, D5-C remain pending; `case_construction_authorized` stays `false` |
| 6 | **None** — no adjudicator is designated; a deferred run needs none |
| 7 | **None** — thresholds, seven-call configuration, and the 613-call ceiling stand |
| 8 | **Not authorized** — the corpus cannot be frozen below the floor |
| 9 | **Not authorized** — a deferred run is not executed |

The deferral does not make the experiment *closer* to execution; it records that execution is
unavailable on the current evidence.

## 6. Mechanism

Recorded as a written operator decision in this file, the form established by
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md). It creates no SOP task approval and no
lifecycle transition. No local decision-memory entry is claimed.
