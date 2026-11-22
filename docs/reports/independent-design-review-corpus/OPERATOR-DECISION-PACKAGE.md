# Operator Decision Package — Independent Design Review

**Type:** Decision package (non-normative). It presents options and consequences. It does
**not** decide, approve, freeze, designate, or authorize anything, except Decision 1, which
the operator recorded separately in
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md).
**Baseline:** `origin/main` @ `a10edc28`, the merge of PR #574 (Workstream B). Earlier
baselines quoted below are preserved as recorded by the source documents.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json).
**Readiness:** [READINESS.md](READINESS.md).
**Verification:** [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).
**Preparation plan:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md).
**Workstream B reconciliation:** [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md).
**Conditional PRD:** [PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).
It is unapproved and is not authorized by this package.

The recommended option in each section is a recommendation, not a decision. The operator
may choose any option, including deferral. No decision below is recorded in this package
except through the separate record named above, and none is implied by a merged pull
request.

**Workstream B status at this baseline.** PR #574 (merge `a10edc28`) merged the
execution-path verification. It records the execution infrastructure as READY for
controlled execution of arms A and B, including concurrent calls, and it fixed the
same-project run-directory collision. It leaves EP-15 (provider-side concurrency and
parallel-batch latency) UNVERIFIED and EP-02/EP-09/EP-10/EP-12/EP-16 PARTIAL. The merge is
evidence, not approval of any decision in this package.

## Summary

| #   | Decision                           | Current state                                                                                    | Recommended disposition                                                                                                       | Depends on                                  | Deferral consequence                          |
| --- | ---------------------------------- | ------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------- | --------------------------------------------- |
| 1   | Corpus owner                       | **Designated: Tom Tran (2026-11-21)**                                                            | Designate one named human who is not an adjudicator                                                                           | None; needed by 2, 8                        | Corpus cannot be frozen                       |
| 2   | Historical inclusion and exclusion | **Recorded 2026-11-21** — see DECISION-RECORD-002                                                | Confirm the exclusions and holds; classify the extracted leads explicitly; keep unverified entries out of every floor         | 1                                           | Shortfall remains                             |
| 3   | Strict versus partial eligibility  | **Recorded 2026-11-21** — conservative; seeded set additional                                    | Keep the conservative reading until the corpus can meet both floors without relaxing the recall denominator                   | 2, 4                                        | Shortfall remains                             |
| 4   | Proposed protocol amendment        | **REJECTED (unnecessary) 2026-11-21**                                                            | Rejected as unnecessary under Decision 3                                                                                      | 1, 3                                        | Conservative reading applies                  |
| 5   | Independent seeder                 | **Recorded 2026-11-21** — seeder Tom Tran (also corpus owner); construction not authorized       | Designate a human independent of the adjudicators                                                                             | None; needed by 6, 8                        | No seeded or no-known-defect cases            |
| 6   | Two independent human adjudicators | None identified; operator is treated as an author                                                | Designate two external humans and record authorship checks                                                                    | 5 (seeder must differ), 1                   | PASS is impossible; best outcome INCONCLUSIVE |
| 7   | Thresholds and cost limits         | Pre-registered, unapproved                                                                       | Approve before freeze; no changes after the first run                                                                         | 2, 3 (corpus size)                          | No run authorized                             |
| 8   | Corpus and instruction freeze      | `NOT FROZEN`; instructions draft                                                                 | Authorize only after 1–7 and corpus completion                                                                                | 1–7, corpus completion, WSB limits recorded | No reproducible run                           |
| 9   | Full experiment execution          | Not authorized                                                                                   | Authorize explicitly after freeze, EP-15 disposition, and readiness checks                                                    | 8, EP-15, approved budget                   | No execution                                  |

## 1. Corpus-owner designation

**Current status.** **RECORDED 2026-11-21.** Tom Tran is designated as corpus owner; the
record is [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md).

**Verified evidence.** The protocol assigns the corpus owner the selection, determinability,
and freeze record (§5.3, §5.4). One person is designated (Tom Tran, 2026-11-21). The
repository shows only the operator as a human Git identity. The protocol permits the
operator to be the corpus owner but not an adjudicator.

**Recorded disposition.** Tom Tran was designated, with the recorded statement that he is
not an adjudicator. The options below are preserved as the analysis that preceded the
decision.

**Alternatives and trade-offs.**

- Operator as corpus owner: fastest and consistent with the protocol, but concentrates
  roles and makes external review of the freeze more important.
- An external corpus owner: stronger separation, but requires someone with repository
  access and enough context to judge determinability.
- Defer: keeps the corpus `NOT FROZEN`.

**Dependencies.** None. Decisions 2 and 8 need a named owner to record the list and the
freeze.

**Effect on experiment validity.** The freeze record needed a named owner to be treated as
pre-registered; that condition is now met.

**Consequence of deferral (no longer applicable).** Workstream A would have stayed at
`NOT READY FOR CORPUS-OWNER FREEZE`.

**Recorded operator action.** The name, role, date, and independence statement are recorded
in [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md) §1.

## 2. Historical candidate inclusion and exclusion

**Current status.** **RECORDED 2026-11-21.** The dispositions are approved in
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md).

**Verified evidence.** Twenty-three manifest entries: 5 `VERIFIED`, 7 `PROPOSED`, 3
`UNVERIFIED`, 8 `EXCLUDED`. Current disposition: 0 strict determinable, 10 proposed
partial, 2 verified-defect holds (ACP-ZED `IDR-H-001`, LLM02 `IDR-H-005`), 3 unverified
holds (`IDR-E-006`, `IDR-E-010`, `IDR-E-012`), 8 excluded. Eight evidence-backed leads are
extracted in [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md): three
proposed strict, one proposed partial, one evidence-blocked, one implementation-only, and
two with no single artifact. The manifest's `hold` inclusion value
is therefore 5, not 2; only the 2 verified-defect holds are "held pending corpus-owner
review". All 17 entries that record an artifact resolve and match their SHA-256.

**Recorded disposition.** The 8 exclusions, the 2 verified-defect holds, the 3 unverified
holds, the evidence-blocked hold, and the lead dispositions are approved in
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md). Approved: 3 proposed
strict plans / 4 defects and 11 proposed partial plans / 11 defects, all include with
review; no candidate is promoted to final eligibility. The analysis below is preserved.

**Alternatives and trade-offs.**

- Accept the held candidates: rejected on evidence; both contain the answer or lack a
  pre-defect revision.
- Apply the amendment: a plan counts only when the corpus owner independently determines
  at least one defect; a merely partial defect still counts toward neither floor.
- Expand only with historical leads: slower, but preserves the "real defect" property.
- Use seeded cases to reach the floor: faster, but changes the corpus mix and must be
  reported separately.

**Dependencies.** Decision 1 is recorded. Decision 3 determines how the 11 partial
candidates are counted.

**Effect on experiment validity.** Inclusion determines the recall denominator and the
false-positive denominator. A contaminated candidate invalidates the comparison.

**Consequence of deferral (no longer applicable).** The corpus could not be frozen and the
shortfall would remain.

**Recorded operator action.** The dispositions are recorded in
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md) §2.

## 3. Strict versus partial eligibility

**Current status.** **RECORDED 2026-11-21.** The conservative rule and the seeded-case
interpretation are recorded in [DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md).

**Verified evidence.** The conservative reading counts a partially determinable defect
toward neither floor. Under it, the proposed strict corpus is 3 plans / 4 defects (0 finally
eligible, 0 adjudicated); the 11 proposed partials are outside the denominator. The
registered protocol names only the no-known-defect set as additional, so this decision also
records that the seeded set is additional and does not count toward either historical
minimum. The proposed amendment would let a plan count toward the plan floor only when it
has at least one determinable defect. No independent adjudicator has confirmed any
classification.

**Recorded disposition.** The conservative rule was recorded, with the seeded set additional.
The analysis below is preserved.

**Alternatives and trade-offs.**

- Approve the amendment: it does not admit a partial defect as partial. It changes the
  plan-floor test to require a determinable defect, so both counts rise only if the corpus
  owner independently determines defects; the amendment's §5 shows 10/10 under that
  condition.
- Count a partial defect as determinable without independent support: rejected; it would
  dilute recall.
- Add seeded cases: raises counts without changing historical eligibility, but must be
  reported as a separate stratum.

**Dependencies.** Decisions 2 and 4. The rule cannot be applied before the candidate set
and the amendment disposition are recorded.

**Effect on experiment validity.** The amendment aligns the plan floor with the defect
floor. It cannot raise either count without an independently recorded determination, so it
does not by itself make the experiment powered. Treating a partial defect as determinable
without that support would dilute recall.

**Consequence of deferral (no longer applicable).** The strict reading would have stood
provisionally and the strict shortfall would remain 12 plans / 11 defects.

**Recorded operator action.** The rule, the seeded interpretation, the approver, and the date
are recorded in [DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md) §1.

## 4. Proposed protocol amendment

**Current status.** **REJECTED (unnecessary) 2026-11-21.** Recorded in
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md).

**Verified evidence.** [PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md)
records a proposed eligibility rule. It changes the meaning of the plan floor, not the
numeric thresholds. It is not applied. Decision 3 already records the same exclusions and
denominator, so the amendment would change no count. The assessment and the manifest keep
the conservative reading.

**Recorded disposition.** Rejected as unnecessary under Decision 3. The analysis below is
preserved.

**Alternatives and trade-offs.**

- Approve now: unblocks the plan-floor interpretation. It does not by itself raise either
  count; every counted plan still needs an independently determined defect, and the
  amendment's §5 shows 10/10 only under that condition. Approving before those
  determinations exist risks recording determinations the corpus owner cannot support.
- Reject now: preserves the strict reading and may force a larger corpus.
- Revise: for example, use a separately reported stratum.

**Dependencies.** Decisions 1 and 3. Applying it also requires the corpus owner to record
the independent support for each candidate it freezes.

**Effect on experiment validity.** The amendment cannot change the defect floor and cannot
by itself raise the plan count, so it cannot by itself make the experiment powered.

**Consequence of deferral (no longer applicable).** The conservative reading would have
applied.

**Recorded operator action.** The rejection and its rationale are recorded in
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md) §1–§2.

## 5. Independent seeder designation

**Current status.** **RECORDED 2026-11-21.** Tom Tran is designated as seeder (also corpus
owner); the case checker, sealed-manifest location, and construction authorization remain
open. See [DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md).

**Verified evidence.** One seeder is designated (Tom Tran, 2026-11-21), who is also the corpus
owner. No independent case checker is designated and no sealed-manifest location is selected.
No seeded or no-known-defect case is constructed (0 of about 8 and 0 of about 6), and
`case_construction_authorized` is false. Protocol §5.3 requires seeding by someone other than
the adjudicator and a sealed manifest before any run.

**Recorded disposition.** Tom Tran was designated as seeder, with the corpus-owner overlap
recorded as a limitation (protocol §5.4 permits it). The analysis below is preserved.

**Alternatives and trade-offs.**

- Operator as seeder: permitted if the operator is not an adjudicator, but weakens
  separation from the corpus owner.
- External seeder: stronger, but requires repository access and a handover of the sealed
  manifest.
- Defer: no seeded or no-known-defect cases.

**Dependencies.** None to designate. Decision 6 depends on the seeder being distinct from
both adjudicators; decision 8 needs the sealed manifest and access list.

**Effect on experiment validity.** Seeded and no-known-defect cases are the only control for
false positives and calibration. Without them, a PASS cannot support a quality claim.

**Consequence of deferral (no longer applicable).** The experiment would remain unpowered for
false positives.

**Recorded operator action.** The seeder, the overlap limitation, the independence
restrictions, the pending checker and manifest, and `case_construction_authorized: false` are
recorded in [DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md) §1–§5.

## 6. Two independent human adjudicators

**Current status.** Open — adjudicator designation remains pending. No adjudicator is
identified. A preparation package is in
[DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md).
**Procedural clarifications:** **APPROVED 2026-11-21** — see
[DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md).

**Verified evidence.** The repository shows only the operator as a human identity. The
operator is treated conservatively as an author or direct editor of the candidate plans. No
adjudicator is identified. The protocol requires two independent humans; a model cannot
adjudicate, and the second adjudicator re-adjudicates the larger of 20 findings or a random
20% sample.

**Recommended disposition.** Designate two external humans who did not author or directly
edit any corpus plan, and record the authorship evidence for each. If two cannot be found,
record that the best achievable outcome is INCONCLUSIVE.

**Alternatives and trade-offs.**

- One adjudicator: kappa is not independent; the outcome cannot PASS.
- Operator as adjudicator: conflicts with the authorship rule.
- AI adjudicator: not permitted by the protocol.
- Defer: no PASS.

**Dependencies.** Decisions 1 and 5. Both adjudicators must differ from the seeder of any
defect they adjudicate and from the corpus owner.

**Effect on experiment validity.** Adjudication converts findings into the labelled data the
hypotheses test. Without two independent adjudicators, the experiment cannot return PASS.

**Consequence of deferral.** Any run would be INCONCLUSIVE at best.

**Exact operator action required.** Record two names, each authorship and independence
check, the second-adjudication sampling rule, and the conflict-of-interest record.

## 7. Thresholds and cost limits

**Current status.** Open. The thresholds are pre-registered by the assessment but not
approved; no hard cost ceiling is approved. A readiness assessment is in
[DECISION-007-THRESHOLD-COST-READINESS.md](DECISION-007-THRESHOLD-COST-READINESS.md), and a
decision-ready proposal is in
[DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md).

**Verified evidence.** The assessment owns the thresholds and the call budget. The current
planning range is 319–700 calls without Arm D and 499–1,309 with Arm D across the 15–29 plan
range. No threshold is approved. No run is authorized. The Workstream B verification also
records one unintended real cloud call outside the pilot and experiment budgets (no plan or
repository content, output not kept, call count unknown); it is not experiment data and
does not change the assessment's budget, but the operator should account for it.

**Recommended disposition.** Approve the pre-registered thresholds and a hard cost ceiling
before freeze, and record that no threshold may change after the first run.

**Alternatives and trade-offs.**

- A smaller corpus: lower cost, lower power.
- A larger corpus: more power, higher cost, more adjudication.
- Optional model-diversity arms: more evidence, more cost.
- Defer: no run.

**Dependencies.** Decisions 2 and 3 determine the corpus size and therefore the call
budget. Decision 9 needs the approved ceiling.

**Effect on experiment validity.** Changing a threshold after seeing data invalidates the
pre-registration.

**Consequence of deferral.** The experiment cannot be authorized.

**Exact operator action required.** Record the approved threshold values, the hard call or
cost ceiling, the measurement rule for unreported usage, and the "no threshold change after
the first run" statement.

## 8. Corpus and instruction freeze authorization

**Current status.** Open. The corpus is `NOT FROZEN`; the instruction blocks are draft; the
freeze procedure is prepared but not executed.

**Verified evidence.** The corpus is `NOT FROZEN`. The instruction blocks are draft. The
[FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) is prepared but not executed, and its §8
freeze-readiness assessment records **NOT READY**. Workstream B is merged and its residual
limits are recorded in
[WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md).

**Recommended disposition.** Authorize the freeze only after decisions 1–7 are recorded and
the corpus, seeded cases, and no-known-defect cases are complete. Record the freeze commit
and the manifest hash.

**Alternatives and trade-offs.**

- Freeze a smaller corpus: faster, but likely INCONCLUSIVE.
- Freeze historical-only: no false-positive control.
- Defer: no run.

**Dependencies.** Decisions 1–7; a corpus that meets the floors under the rule in force;
constructed seeded and no-known-defect cases with a sealed manifest; the Workstream B
residual limits copied into the freeze record.

**Effect on experiment validity.** A freeze makes the inputs auditable. A post-freeze change
silently changes the experiment.

**Consequence of deferral.** No reproducible run.

**Exact operator action required.** Record the freeze authorization, the freeze commit, the
manifest SHA-256, the instruction-block SHA-256 values, and the date. Do not write `FROZEN`
until the record is complete.

## 9. Authorization before full experiment execution

**Current status.** Open. No full run is authorized.

**Verified evidence.** No full run. Two four-call pilots ran and are excluded from the
dataset. Workstream B is merged (PR #574): the execution path is verified for controlled
arms A and B, including concurrency, but EP-15 (provider-side concurrency and parallel-batch
latency) is UNVERIFIED, EP-16 (observed repository inspection) is PARTIAL, and EP-02,
EP-09, EP-10 and EP-12 are PARTIAL. The conditional PRD is not authorized.

**Recommended disposition.** Authorize execution only after the freeze, the EP-15
disposition, and the pilot readiness checks, with a bounded call budget and a stop
condition.

**Alternatives and trade-offs.**

- A staged run: more control, more coordination.
- A bounded pilot extension: tests plumbing, not quality.
- Defer: no execution.

**Dependencies.** Decision 8 (freeze), decision 7 (budget), a recorded EP-15 disposition
(authorized minimum concurrency check, or acceptance that C latency is first measured in
the full run), and the approved thresholds.

**Effect on experiment validity.** Execution without the freeze or without resolving the
open execution items produces results that cannot be attributed or reproduced.

**Consequence of deferral.** The experiment remains NOT YET READY.

**Exact operator action required.** Record the execution authorization with the start
condition, the bounded call budget, the stop condition, the provider and model, and the
EP-15 disposition.

## Exact approvals required

The operator is asked to record, in writing:

1. The named corpus owner and the independence statement.
2. The confirmed inclusion and exclusion list, the 2 verified-defect holds, the 3
   unverified holds, and the 6 lead dispositions.
3. The eligibility rule in force (conservative or amended).
4. The amendment decision: approve, reject, or revise.
5. The named seeder, the independence statement, and the sealed-manifest access list.
6. The two named human adjudicators, their authorship checks, and the sampling rule.
7. The approved thresholds and cost ceiling.
8. The freeze authorization, with the freeze commit and manifest hash.
9. The execution authorization, with the budget, stop condition, and EP-15 disposition.

Items 1–5 are recorded in [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md),
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md),
[DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md),
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md), and
[DECISION-RECORD-005](DECISION-RECORD-005-SEEDER.md). Items 6–9 do not yet exist in the
repository. PR #574 is merged evidence; it is not an approval of any
item above.
