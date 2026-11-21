# Proposed Protocol Amendment — Corpus Eligibility for Partially Determinable Plans

**Status:** **REJECTED (unnecessary) 2026-11-21** — not applied; see
[DECISION-RECORD-004](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md). The proposal and its
rationale are preserved below as historical evidence; its §5–§6 figures predate Decisions 2
and 3. The registered protocol is unchanged.
**Baseline:** `origin/main` @ `1b61db6427e73ab18cab532834504359ea41de04`.
**Authoritative protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Corpus evidence:** [READINESS.md](READINESS.md), [candidates.json](candidates.json).

## 1. Question

The assessment defines two floors:

- **15 plans with known defects.**
- **15 determinable known defects.**

It does not say how a plan whose known defect is only **partially determinable** — because
the defect becomes visible only under a later contract or later report — counts toward
either floor. The feasibility report currently takes the conservative reading: such a
plan counts toward neither floor. That reading is safe but it may be stricter than the
protocol intends.

## 2. Proposed rule

A plan counts toward the **plan floor** only when it has at least one **independently
supported, determinable known defect**.

Only **individually determinable defects** count toward the **determinable-defect floor**.

A **partially determinable** or **hindsight-only** defect does not count toward either
floor. The recall denominator is the pre-registered set of determinable known defects; it
does not include a defect that a reviewer could identify only after seeing the later fix
or report.

Proposed normative wording, for the corpus owner and operator to approve or reject:

> A plan counts toward the 15-plan floor only when it contains at least one determinable
> known defect. A defect is determinable when a reviewer with the pre-fix artifact and its
> permitted repository context could identify it without the later defect report or fix.
> Partially determinable and hindsight-only defects count toward neither floor. The recall
> denominator is the frozen set of determinable known defects.

## 3. Why this rule

- It keeps the plan floor and the defect floor aligned: a plan cannot satisfy the plan
  floor by carrying only a defect that cannot enter the recall denominator.
- It prevents a plan with several hindsight-only defects from inflating the corpus.
- It preserves the existing recall measurement: partial and hindsight-only defects remain
  out of the denominator.
- It makes the freeze decision explicit instead of leaving the partially determinable
  candidates in an unclassified state.

## 4. Does it change the protocol?

**Yes, in meaning, but not in numeric thresholds.** The assessment says “15 plans with
known defects.” The proposed rule tightens that to “15 plans, each with at least one
determinable known defect.” A plan with a known but non-determinable defect would no
longer count. That is a change to eligibility, so this document is a proposed protocol
amendment, not a clarification applied by the corpus owner.

The numeric floors (15 plans, 15 defects) and the PASS/FAIL/INCONCLUSIVE rules are
unchanged. No threshold value is changed.

## 5. Impact on the current evidence

| Reading                                                                                                                   | Plans | Determinable defects | Shortfall            |
| ------------------------------------------------------------------------------------------------------------------------- | ----- | -------------------- | -------------------- |
| Current conservative reading (partial counts toward neither floor)                                                        | 0     | 0                    | 15 plans, 15 defects |
| Proposed rule, if the corpus owner independently supports and freezes all 10 eligible partial candidates as determinable   | 10    | 10                   | 5 plans, 5 defects   |
| Proposed rule, with no partial candidate frozen                                                                            | 0     | 0                    | 15 plans, 15 defects |

The proposed rule does not by itself make the corpus sufficient, and it does not by itself
make a partial candidate determinable. The corpus owner must record the independent
support for each candidate it freezes; without that support, the strict reading applies.
Even the partial-inclusive upper bound is short of both 15-item floors. It also does not
create the seeded or no-known-defect corpus.

## 6. Authorization and next steps

This amendment is **not applied**. The assessment and the manifest keep the conservative
reading until the operator decides.

To apply it, the operator must:

1. Approve this rule, or provide a different eligibility rule.
2. Authorize the corpus owner to freeze the eligible set.
3. Record the frozen set and the excluded set in the corpus manifest.

If approved, the implementation is documentation-only:

- Add the rule to the assessment's corpus section.
- Change the manifest's proposed inclusion fields only after the corpus owner's freeze.
- Keep the recall denominator as the frozen determinable set.

If rejected, the strict reading stands: zero plans and zero determinable defects, with a
shortfall of fifteen against both floors.
