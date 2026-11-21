# Operator Decision Package — Independent Design Review

**Type:** Decision package (non-normative). It presents options and consequences. It does
**not** decide, approve, freeze, designate, or authorize anything.
**Baseline:** `origin/main` @ `ade6864a`, plus the restored pilot evidence commit.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json).
**Readiness:** [READINESS.md](READINESS.md).
**Verification:** [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).
**Preparation plan:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md).
**Conditional PRD:** [PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).
It is unapproved and is not authorized by this package.

The recommended option in each section is a recommendation, not a decision. The operator
may choose any option, including deferral.

## Summary

| # | Decision | Current state | Recommended option | Deferral consequence |
| --- | --- | --- | --- | --- |
| 1 | Corpus owner | Not designated | Designate one named human who is not an adjudicator | Corpus cannot be frozen |
| 2 | Historical inclusion and exclusion | 0 strict, 10 partial, 2 held, 8 excluded, 3 unverified | Confirm the exclusions and holds; authorize lead extraction | Denominator undefined |
| 3 | Strict versus partial eligibility | Conservative reading applied; amendment not approved | Keep the conservative reading until the corpus is powered | Freeze rule undefined |
| 4 | Proposed protocol amendment | Proposed, not applied | Defer until the corpus owner records independent support for each partial | Conservative reading stands |
| 5 | Seeder | Not designated | Designate a human independent of the adjudicators | No seeded or no-known-defect cases |
| 6 | Two human adjudicators | None identified; operator is treated as an author | Designate two external humans and record authorship checks | PASS is impossible; best outcome INCONCLUSIVE |
| 7 | Thresholds and cost limits | Pre-registered, unapproved | Approve before freeze; no changes after the first run | No run authorized |
| 8 | Corpus and instruction freeze | `NOT FROZEN`; instructions draft | Authorize only after 1-7 and corpus completion | No reproducible run |
| 9 | Full experiment execution | Not authorized | Authorize explicitly after freeze and readiness checks | No execution |

## 1. Corpus-owner designation

**Current evidence.** The protocol assigns the corpus owner the selection, determinability,
and freeze record. No person is designated. The repository shows only the operator as a
human Git identity. The protocol permits the operator to be the corpus owner but not an
adjudicator.

**Recommended option.** Designate one named human as corpus owner, with a recorded statement
that the person is not an adjudicator. The operator may serve if the operator accepts that
the role is administrative and that the operator remains unable to adjudicate.

**Alternatives and trade-offs.**

- Operator as corpus owner: fastest and consistent with the protocol, but concentrates
  roles and makes external review of the freeze more important.
- An external corpus owner: stronger separation, but requires someone with repository
  access and enough context to judge determinability.
- Defer: keeps the corpus `NOT FROZEN`.

**Effect on experiment validity.** The freeze record has no owner without this decision, so
the corpus cannot be treated as pre-registered.

**Consequence of deferral.** Workstream A stays at `NOT READY FOR CORPUS-OWNER FREEZE`.

## 2. Historical candidate inclusion and exclusion

**Current evidence.** Twenty-three manifest entries: 5 `VERIFIED`, 7 `PROPOSED`, 3
`UNVERIFIED`, 8 `EXCLUDED`. Current disposition: 0 strict determinable, 10 proposed
partial, 2 held (ACP-ZED, LLM02), 8 excluded, 3 unverified. Six evidence-backed leads are
listed in the preparation plan but are not counted.

**Recommended option.** Confirm the 8 exclusions and the 2 holds, and authorize extraction
of the six leads. Do not count a lead until it is extracted and classified.

**Alternatives and trade-offs.**

- Accept the held candidates: rejected on evidence; both contain the answer or lack a
  pre-defect revision.
- Count partials toward the plan floor: changes eligibility and requires the amendment.
- Expand only with historical leads: slower, but preserves the "real defect" property.
- Use seeded cases to reach the floor: faster, but changes the corpus mix and must be
  reported separately.

**Effect on experiment validity.** Inclusion determines the recall denominator and the
false-positive denominator. A contaminated candidate invalidates the comparison.

**Consequence of deferral.** The corpus cannot be frozen and the shortfall remains.

## 3. Strict versus partial eligibility

**Current evidence.** The conservative reading counts a partially determinable defect toward
neither floor. Under it, the corpus is 0/15 strict and 10/15 partial-inclusive. The proposed
amendment would let a plan count toward the plan floor only when it has at least one
determinable defect.

**Recommended option.** Keep the conservative reading until the corpus can meet both floors
without relaxing the recall denominator. Do not let a partial defect enter the recall
denominator.

**Alternatives and trade-offs.**

- Approve the amendment: raises the plan count only if the corpus owner independently
  supports each partial; does not raise the defect count.
- Count partials toward both floors: rejected; it would dilute recall.
- Add seeded cases: raises counts without changing historical eligibility, but must be
  reported as a separate stratum.

**Effect on experiment validity.** A relaxed rule can make the corpus look sufficient while
the detectable-defect denominator stays too small, producing an underpowered PASS.

**Consequence of deferral.** The strict reading stands and the shortfall remains 15/15.

## 4. Proposed protocol amendment

**Current evidence.**
[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md) records a proposed
eligibility rule. It changes the meaning of the plan floor, not the numeric thresholds. It
is not applied.

**Recommended option.** Defer the amendment until the corpus owner records independent
support for each partial candidate, then decide with the actual counts in hand.

**Alternatives and trade-offs.**

- Approve now: unblocks a plan-floor interpretation, but risks counting plans whose defects
  cannot enter the denominator.
- Reject now: preserves the strict reading and may force a larger corpus.
- Revise: for example, allow partials only in a separately reported stratum.

**Effect on experiment validity.** The amendment cannot change the defect floor, so it cannot
by itself make the experiment powered.

**Consequence of deferral.** The conservative reading applies.

## 5. Independent seeder designation

**Current evidence.** No seeder is designated. No seeded or no-known-defect case is
constructed. The protocol requires an independent seeder and a sealed manifest.

**Recommended option.** Designate a human seeder who is independent of the adjudicators and,
preferably, of the corpus owner. Record the independence statement.

**Alternatives and trade-offs.**

- Operator as seeder: permitted if the operator is not an adjudicator, but weakens
  separation from the corpus owner.
- External seeder: stronger, but requires repository access and a handover of the sealed
  manifest.
- Defer: no seeded or no-known-defect cases.

**Effect on experiment validity.** Seeded and no-known-defect cases are the only control for
false positives and calibration. Without them, a PASS cannot support a quality claim.

**Consequence of deferral.** The experiment remains unpowered for false positives.

## 6. Two independent human adjudicators

**Current evidence.** The repository shows only the operator as a human identity. The
operator is treated conservatively as an author or direct editor of the candidate plans. No
adjudicator is identified. The protocol requires two independent humans; a model cannot
adjudicate.

**Recommended option.** Designate two external humans who did not author or directly edit
any corpus plan, and record the authorship evidence for each. If two cannot be found, record
that the best achievable outcome is INCONCLUSIVE.

**Alternatives and trade-offs.**

- One adjudicator: kappa is not independent; the outcome cannot PASS.
- Operator as adjudicator: conflicts with the authorship rule.
- AI adjudicator: not permitted by the protocol.
- Defer: no PASS.

**Effect on experiment validity.** Adjudication converts findings into the labelled data the
hypotheses test. Without two independent adjudicators, the experiment cannot return PASS.

**Consequence of deferral.** Any run would be INCONCLUSIVE at best.

## 7. Thresholds and cost limits

**Current evidence.** The assessment owns the thresholds and the call budget. The current
planning range is 319-700 calls without Arm D and 499-1,309 with Arm D across the 15-29 plan
range. No threshold is approved. No run is authorized.

**Recommended option.** Approve the pre-registered thresholds and a hard cost ceiling before
freeze, and record that no threshold may change after the first run.

**Alternatives and trade-offs.**

- A smaller corpus: lower cost, lower power.
- A larger corpus: more power, higher cost, more adjudication.
- Optional model-diversity arms: more evidence, more cost.
- Defer: no run.

**Effect on experiment validity.** Changing a threshold after seeing data invalidates the
pre-registration.

**Consequence of deferral.** The experiment cannot be authorized.

## 8. Corpus and instruction freeze authorization

**Current evidence.** The corpus is `NOT FROZEN`. The instruction blocks are draft. The
[FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) is prepared but not executed.

**Recommended option.** Authorize the freeze only after decisions 1-7 are recorded and the
corpus, seeded cases, and no-known-defect cases are complete. Record the freeze commit and
the manifest hash.

**Alternatives and trade-offs.**

- Freeze a smaller corpus: faster, but likely INCONCLUSIVE.
- Freeze historical-only: no false-positive control.
- Defer: no run.

**Effect on experiment validity.** A freeze makes the inputs auditable. A post-freeze change
silently changes the experiment.

**Consequence of deferral.** No reproducible run.

## 9. Authorization before full experiment execution

**Current evidence.** No full run. Two four-call pilots ran and are excluded from the
dataset. Workstream B (Linux provider and execution evidence) remains pending. The
conditional PRD is not authorized.

**Recommended option.** Authorize execution only after the freeze, the Workstream B evidence,
and the pilot readiness checks, with a bounded call budget and a stop condition.

**Alternatives and trade-offs.**

- A staged run: more control, more coordination.
- A bounded pilot extension: tests plumbing, not quality.
- Defer: no execution.

**Effect on experiment validity.** Execution without the freeze or Workstream B evidence
produces results that cannot be attributed or reproduced.

**Consequence of deferral.** The experiment remains NOT YET READY.

## Exact approvals required

The operator is asked to record, in writing:

1. The named corpus owner and the independence statement.
2. The confirmed inclusion and exclusion list, and the two held candidates.
3. The eligibility rule in force (conservative or amended).
4. The amendment decision: approve, reject, or revise.
5. The named seeder and the independence statement.
6. The two named human adjudicators and their authorship checks.
7. The approved thresholds and cost ceiling.
8. The freeze authorization, with the freeze commit and manifest hash.
9. The execution authorization, with the budget and stop condition.

None of these approvals exists in the repository at this baseline.
