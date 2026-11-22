# Decision 6 — Adjudicator Qualification Package (Independent Design Review / EGM)

**Type:** Preparation package (non-normative). It prepares the recruitment, qualification, and
adjudication procedure for Decision 6. It designates no adjudicator, approves no decision, and
authorizes no experiment activity.
**Status:** **PREPARATION ONLY — Decision 6 remains PENDING.**
**Baseline:** `origin/main` @ `31b479dd` (merge of PR #603).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) — authoritative.
**Decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §6.
**Readiness:** [READINESS.md](READINESS.md) §7 row 6 and §17.
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §3.10.
**Case construction:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md) §6.
**Procedure clarification:** [DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md](DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md).
**Prior decisions:** [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md),
[DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md).

No adjudicator is designated. No candidate has been contacted. No finding exists, no
adjudication has occurred, and no kappa has been computed. The corpus is **NOT FROZEN** and
the experiment is **NOT YET READY**.

## 1. Purpose and scope

Decision 6 asks the operator to designate two independent human adjudicators. This package
lets the operator identify, evaluate, and eventually designate them without inventing
evidence or weakening the protocol.

It contains:

1. a source-referenced requirement matrix;
2. a per-candidate qualification checklist and two blank records;
3. a role description and a copy-ready recruitment message;
4. a conflict-of-interest declaration;
5. the blinding and evidence-handling procedure;
6. the double-adjudication and agreement requirements;
7. the outstanding operator decisions;
8. a Decision 6 readiness checklist.

It does **not** designate anyone, contact anyone, create `DECISION-RECORD-006`, authorize
adjudication, construct cases, freeze the corpus, or run inference.

## 2. Protocol requirement matrix

Labels: **M** = mandatory in the registered protocol; **G** = existing approved governance
control (a decision record, the freeze procedure, or the case-construction specification);
**R** = recommended operational safeguard, **not** a protocol requirement.

| # | Requirement | Basis | Source |
| --- | --- | --- | --- |
| 1 | A human adjudicates. A model may not be an adjudicator; a model may serve only as a third check. | M | §5.4 |
| 2 | A PASS requires two distinct human adjudicators. | M | §5.4 |
| 3 | Adjudicators must not be authors of any plan in the corpus. Approving a plan does not make someone its author. | M | §5.4 |
| 4 | The two adjudicators must be distinct from each other. | M | §5.4 |
| 5 | Each adjudicator must be distinct from the seeder of any defect they adjudicate. | M | §5.4 |
| 6 | The corpus owner, seeder, and analyst may overlap with each other and the operator, but none may adjudicate. | M | §5.4 |
| 7 | The adjudicators remain blind; they do not see arm labels. | M | §5.4 |
| 8 | Findings are exported with arm, repetition, model, and run ids removed, shuffled per plan, and presented in that order. | M | §5.4 |
| 9 | Each finding is classified: matches known defect `<id>`; valid and not on the list (one-line justification from the document); invalid or immaterial; duplicate of finding `<n>`. | M | §5.4 |
| 10 | Judgment is against the document and the pre-registered defect list, never the reviewer's stated confidence. | M | §5.4 |
| 11 | Duplicate labels reference finding IDs; the analyst applies the within-run merge. | M | §5.4 |
| 12 | The second adjudicator independently re-adjudicates the larger of 20 findings or a random 20% sample. | M | §5.4 |
| 13 | Agreement is reported as Cohen's kappa. | M | §5.4 |
| 14 | If fewer than 20 findings exist in total, kappa is reported but the outcome cannot PASS. | M | §5.4 |
| 15 | Kappa below 0.6 on the double-adjudicated sample makes the affected comparison INCONCLUSIVE. | M | §6.1 |
| 16 | A PASS additionally requires at least 20 double-adjudicated findings. | M | §6.2 |
| 17 | Without two independent human adjudicators, the best attainable outcome is INCONCLUSIVE. | M | §5.4 |
| 18 | Adjudication is evidence work, not an approval; it creates no SOP approval record and changes no lifecycle state. | M | §5.4 |
| 19 | Findings are judged with the pre-fix artifact and permitted references; later evidence and answer materials are excluded. | M | §5.2, §5.4 |
| 20 | The freeze record names both adjudicators, their independence and authorship checks, the sampling rule, and the conflict-of-interest record. | G | Freeze §3.10 |
| 21 | Adjudicators receive the blinded defect list, never the sealed answer key. | G | Case spec §6.2 |
| 22 | The run manifest names the first and second adjudicator. | G | §5.8 |
| 23 | A conflict-of-interest record exists. | G | Freeze §3.10 |
| 24 | A qualification reviewer other than the candidate records each outcome. | R | Proposed safeguard |
| 25 | The candidate confirms availability for the adjudication workload. | R | Proposed safeguard |
| 26 | The candidate signs a confidentiality and blinding acknowledgement. | R | Proposed safeguard; the blinding itself is M (#7–#8) |
| 27 | The second-adjudication random-sample seed is recorded. | R | Proposed safeguard; §5.8 records the run-order seed, not the sample seed |
| 28 | The adjudicator is oriented on the rubric, the categories, and the defect list before labeling. | R | Proposed safeguard |

**Discrepancy check.** The Decision 6 checkpoint stated the sampling rule as "the larger of
20 findings or a random 20% sample" and the PASS floor as "at least 20 double-adjudicated
findings." Both match the protocol (§5.4, §6.2) exactly. No discrepancy was found, and the
protocol text is unchanged.

## 3. Qualification checklist and records

### 3.1 Per-candidate checklist

**Identity and role**

- [ ] Human identity confirmed. A Git identity alone does **not** prove independence.
- [ ] Relevant technical qualifications documented.
- [ ] Experience reviewing software design and architecture.
- [ ] Able to evaluate design defects against the registered rubric.
- [ ] Available for the required workload. *(R; workload TBD)*

**Independence**

- [ ] Did not author or directly edit any corpus plan or case.
- [ ] Is not Tom Tran, the designated seeder and corpus owner.
- [ ] Is not the experiment analyst.
- [ ] Is independent of the other adjudicator.
- [ ] Has not seeded any case they would adjudicate.
- [ ] Has no disqualifying conflict of interest.

**Blinding and confidentiality**

- [ ] Agrees not to access the sealed answer key.
- [ ] Agrees not to access model or experiment-arm identities.
- [ ] Agrees to use only authorized blinded materials.
- [ ] Agrees not to consult post-defect implementation evidence.
- [ ] Agrees to report accidental exposure or newly discovered conflicts.

Items 1–6 under Independence and the blinding agreement are protocol or governance controls
(M/G). The availability and signed-acknowledgement items are recommended safeguards (R).

### 3.2 Candidate record — A

| Field | Value |
| --- | --- |
| Candidate identifier | `<opaque id; no name required>` |
| Evidence reviewed | `<authorship check, independence evidence, COI declaration>` |
| Identity / role | `UNASSESSED` |
| Independence | `UNASSESSED` |
| Blinding acknowledgement | `UNASSESSED` |
| Conflict-of-interest status | `UNASSESSED` |
| Qualification status | `UNASSESSED` — `UNASSESSED` / `QUALIFIED` / `NOT QUALIFIED` / `NEEDS REVIEW` |
| Reviewer of qualification | `<name>` |
| Date | `<date>` |
| Rationale | `<short, evidence-referenced>` |

### 3.3 Candidate record — B

Same fields as record A.

**No candidate identity or qualification evidence is populated by this package.**

## 4. Recruitment role description

**Purpose.** SOP is preparing a pre-registered experiment on whether an independent design
review finds real design defects. Adjudicators turn blinded findings into labelled data.

**Responsibilities.** Classify each blinded finding against a pre-registered defect list;
record one of the four protocol categories; justify "valid and not on the list" with one line
from the document; label duplicates by finding ID. The second adjudicator re-adjudicates the
sample.

**Skills.** Software design and architecture review; the ability to distinguish a real design
defect from a style preference or an implementation slip; comfort working from a written
rubric and a document rather than from a running system.

**Independence.** The adjudicator must not have written or directly edited any corpus plan or
case, must not be the seeder or corpus owner, must not be the analyst, and must be independent
of the other adjudicator. A model cannot serve.

**Blinded boundaries.** Adjudicators see the pre-fix artifact, the permitted references, and
the blinded defect list. They do not see arm, repetition, model, or run identities, the sealed
answer key, or post-defect evidence.

**Time commitment.** TBD; not established by the protocol or this package.

**Sealed answer key.** Adjudicators never need access to it.

## 5. Copy-ready recruitment message (draft — do not send)

> Subject: Invitation to act as an independent adjudicator in a design-review study
>
> Hello `<name>`,
>
> I am preparing a small, pre-registered study on whether an independent design review finds
> real design defects. I am looking for two independent human adjudicators.
>
> The task is to read a design document and a set of blinded review findings, then classify
> each finding against a pre-registered defect list using a written rubric. You would not see
> which model or configuration produced a finding, and you would not need access to any sealed
> answer material.
>
> Independence matters: adjudicators must not have written or edited the documents under
> review, must not be involved in seeding the cases or analysing the results, and must be
> independent of each other. If you have any relationship that could affect your independent
> judgment, please say so.
>
> Could you let me know whether you have relevant software design or architecture review
> experience, whether you would be available, and whether you see any conflict of interest?
>
> Please note that the study is not yet authorized and no participation is confirmed. I am not
> sharing confidential corpus details at this stage.
>
> Thank you,
> `<operator>`

This message is a draft for operator review. It has not been sent, and no one has been
contacted.

## 6. Conflict-of-interest declaration (reusable)

> **Adjudicator conflict-of-interest declaration**
>
> Candidate identifier: `<opaque id>`  Date: `<date>`
>
> Please answer each item.
>
> 1. Did you author or directly edit any document in the corpus under review? `<yes/no; details>`
> 2. Were you involved in creating or seeding any case? `<yes/no; details>`
> 3. Are you involved in analysing the experiment or holding the unblinding key? `<yes/no; details>`
> 4. Have you had access to blinded findings, the sealed answer key, or arm/model identities? `<yes/no; details>`
> 5. Do you have any relationship, dependency, or interest that could affect your independent judgment? `<yes/no; details>`
> 6. Do you agree to disclose any new conflict that arises during the study? `<yes/no>`
>
> I confirm that the answers above are accurate to the best of my knowledge.
>
> Signature / acknowledgement: `<name or opaque id>`

Only the minimum personal information needed for qualification is collected.

## 7. Blinding and evidence-handling procedure (prepared, not executed)

1. **Create blinded findings.** Export the run's findings into the adjudication set.
2. **Remove identifiers.** Remove arm, repetition, model, run, and provider identifiers.
3. **Shuffle per plan.** Present each plan's findings in a shuffled order; record the shuffle
   seed.
4. **Authorized materials.** Provide the pre-fix artifact, the permitted references that
   existed at the pin, the blinded defect list, and the classification rubric. Do not provide
   later fixes, reports, remediation plans, the sealed answer key, or arm/model identities.
5. **Classify against the registered list.** Use the four categories in §2 #9.
6. **Valid but unmatched findings.** Record "valid and not on the list" with a one-line
   justification from the document. It counts as a true positive but is reported separately
   and is not used in the recall numerator.
7. **Invalid or immaterial findings.** Record them; they feed the false-positive rate.
8. **Duplicates.** Label by finding ID; the analyst applies the within-run merge.
9. **Second-adjudicator sampling.** Apply §5.4 as written; see §8.
10. **Agreement.** Compute and report Cohen's kappa.
11. **Disagreement resolution.** The protocol defines **no** tie-break or resolution
    procedure. The kappa floor and the INCONCLUSIVE rules govern. A resolution procedure, if
    wanted, is a separate operator decision and is not authorized here.
12. **INCONCLUSIVE conditions (adjudication-relevant).** Fewer than 20 findings in total;
    kappa below 0.6; no second adjudicator; fewer than 20 double-adjudicated findings.

No findings are generated, no adjudication is simulated, and no kappa value is calculated by
this package.

## 8. Double-adjudication and agreement requirements

**Verbatim rule (§5.4):** "A second person independently adjudicates the larger of 20 findings
or a random 20% sample, and agreement is reported (Cohen's kappa). If fewer than 20 findings
exist in total, kappa is reported but the outcome cannot PASS."

**PASS floor (§6.2):** "A PASS additionally requires at least 20 double-adjudicated findings;
below that, the outcome is INCONCLUSIVE even if the recall rules would otherwise pass."

**Validity condition (§6.1):** kappa below 0.6 on the double-adjudicated sample makes the
affected comparison INCONCLUSIVE.

**Derived sample size (for a stated universe of N findings):**

```text
k = min(N, max(20, ceil(0.20 * N)))
```

- If N < 20: all N are double-adjudicated; kappa is reported; PASS is impossible.
- If N ≥ 20: at least 20 are double-adjudicated; PASS remains possible only if kappa ≥ 0.6
  and the other validity and outcome rules hold.

**Ambiguity — sampling universe.** The protocol says "shuffled per plan" for presentation but
does not say whether the "20 findings or 20% sample" is drawn per plan or from the pooled
finding set. The phrase "if fewer than 20 findings exist in total" suggests a pooled total.
This package does **not** resolve it. The operator should record the interpretation before
the run; a record of the sample seed is a recommended safeguard.

**Ambiguity — second-adjudicator blindness.** The protocol says the second person
"independently adjudicates" the sample. This package reads that as the second adjudicator
classifying the sample without seeing the first adjudicator's labels; that reading is what
makes kappa meaningful. The protocol does not state it explicitly.

## 9. Outstanding operator decisions

| # | Decision | State |
| --- | --- | --- |
| 6 | Two independent human adjudicators | **PENDING** — name two, or record unavailability and INCONCLUSIVE |
| 6a | Sampling universe (per plan vs pooled) | **PENDING** — record before the run |
| 6b | Disagreement resolution (if any) | **PENDING** — the protocol defines none |
| 5a | Independent case checker | **PENDING** — Decision 5 |
| 5b | Sealed-manifest location | **PENDING** — Decision 5 |
| 5c | Case-construction authorization | **PENDING** — Decision 5 |
| 7 | Thresholds and cost limits | **PENDING** |
| 8 | Corpus and instruction freeze | **PENDING** |
| 9 | Full experiment execution | **PENDING** |

## 10. Decision 6 readiness checklist

- [ ] Two candidate records completed independently.
- [ ] Authorship/editing check evidence recorded for each.
- [ ] Seeder, corpus-owner, and analyst exclusion confirmed for each.
- [ ] Inter-adjudicator independence confirmed.
- [ ] Conflict-of-interest declaration and record completed.
- [ ] Blinding/confidentiality acknowledgement completed.
- [ ] Availability confirmed. *(R)*
- [ ] Sampling universe recorded.
- [ ] Random-sample seed recorded. *(R)*
- [ ] Kappa method and reporting recorded.
- [ ] Freeze record fields (§3.10) ready.
- [ ] No model is an adjudicator.

**Status: 0 of 2 candidates assessed. Decision 6 is NOT READY and NOT RECORDED.**

## 11. What this package does not do

- It does not designate, contact, or qualify any adjudicator.
- It does not create `DECISION-RECORD-006` or mark Decision 6 approved.
- It does not designate the independent case checker or select the sealed-manifest location.
- It does not construct seeded or control cases.
- It does not promote or adjudicate historical candidates.
- It does not modify the registered protocol or its hash.
- It does not change thresholds, arms, or cost ceilings.
- It does not freeze the corpus, run inference, or execute the experiment.
