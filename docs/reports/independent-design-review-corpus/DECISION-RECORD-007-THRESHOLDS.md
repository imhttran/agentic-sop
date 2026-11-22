# Decision Record 007 — Thresholds and Cost (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved Decision 7 as written in
[DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md) §1,
merged in PR #617. The decision is recorded here in the repository's written decision-record
form. The proposal text is preserved as the approved basis; this file is the decision record.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — Decision 7 |
| Operator | **Tom Tran** |
| Thresholds | **Approved verbatim** — EGM assessment §§6.1–6.2 |
| Arm configuration | **Default 7-call** — A + B×3 + C, H1 B shares one control run |
| Repetitions / pilot | **3 per plan**; **4-call pilot**, excluded |
| Arm D | **Disabled** |
| Call ceiling | **613 cumulative provider calls**, conditional on ≤29 plans |
| Usage accounting | Provider tokens; `unreported_calls` recorded; unavailable → INCONCLUSIVE |
| Pre-registration | **No post-run changes** |
| EP-15 | **UNVERIFIED**; explicit disposition required before Decision 9 |
| Execution | **NOT AUTHORIZED** — Decision 9 controls |
| Registered protocol SHA-256 | `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c` — unchanged |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `2f063756` (2026-11-21), the merge of PR #617.

**Source of the decision:** the operator's explicit approval of the PR #617 proposal §1 on
2026-11-21.

## 1. Recorded decision

> Decision 7 — APPROVED
>
> 1. **Thresholds:** Approve all registered EGM assessment §§6.1–6.2 thresholds and validity
>    rules verbatim.
> 2. **Arms:** Approve the default seven-call configuration, shared H1 B control, three
>    repetitions per plan, and four-call pilot. Arm D is disabled.
> 3. **Call ceiling:** Approve a maximum of 613 cumulative provider calls, conditional on a
>    corpus of no more than 29 plans. This is not a spending authorization.
> 4. **Usage accounting:** Approve provider-reported token usage, recording of
>    `unreported_calls`, and an INCONCLUSIVE cost criterion when token usage is unavailable.
> 5. **Pre-registration:** Approve the prohibition on changing thresholds, arms, rubrics,
>    corpus, ordering rules, or analysis methods after the first experimental call.
> 6. **EP-15:** Retain UNVERIFIED status. Require an explicit disposition before Decision 9.
>
> **Execution remains prohibited.** This approval does not authorize model calls, spending,
> case construction, adjudication, corpus freeze, or experiment execution. Decision 9 remains
> the separate execution authorization.

Recorded from the operator's explicit approval on 2026-11-21. The operator identity is Tom
Tran. No signature, time of day, or additional authorization is recorded; none was given.

## 2. Thresholds approved verbatim

The registered rules in §§6.1–6.2 are approved as written. No value is adjusted.

- **Validity (§6.1):** fewer than 15 determinable defects or 15 plans; kappa < 0.6 or no
  second adjudicator; > 10% failed runs or unequal arm failure rates by > 5 points; routing /
  override / `tokens` mismatch; unavailable tokens or any `unreported_calls` > 0; sampling
  parameter drift within an arm; order/corpus/rubric/threshold changed after the first run →
  **INCONCLUSIVE**.
- **Outcome (§6.2), in order:** hard-cap FAIL (FP > A + 10 points or > 50% absolute; cost
  > 1.5× A (B) or 3.5× A (C); median latency > 1.5× A (B) or 2× A (C)); PASS (recall gain
  ≥25% relative and ≥3 more confirmed defects, 90% interval lower bound > 0, median
  reproducibility ≥70%, and ≥20 double-adjudicated findings); FAIL no useful effect (upper
  bound < 10% relative); otherwise INCONCLUSIVE.
- **Comparators:** H1 B vs A; H2 C vs the union of three independent B runs; H3 exploratory,
  report only.

The full matrix is in
[DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md) §2.

## 3. Arm configuration

- **Default 7-call:** A + B×3 + C, with the H1 B run sharing one B×3 control run.
- **Three repetitions** per plan per arm.
- **4-call pilot** (A and B on two plans), excluded from the dataset.
- **Arm D disabled** for the initial experiment.

This is the registered default; the H1 B designation is fixed before the run, not selected
after seeing results.

## 4. Call ceiling

`4 + (3 × 29 × 7) = 4 + 609 = 613` cumulative provider calls.

- Conditional on a corpus of **no more than 29 plans** and the default 7-call configuration.
- A **maximum**, not a target, and **not a spending authorization**.
- A **provider-call** ceiling, not a dollar or token-spend ceiling.
- The `internal/expharness` proxy enforces a hard **per-run** call ceiling; the 613 aggregate
  is tracked operationally (per-run ceilings summing to ≤613, cumulative count logged, stop
  at 613). An aggregate-enforcing wrapper is not implemented — a recorded limitation.
- A larger corpus, the disjoint 8-call configuration (700 at 29 plans), or Arm D (up to
  1,309) each requires a **separate** ceiling decision.

## 5. Usage accounting

- Use provider-reported token usage from `metadata.json`.
- Record `unreported_calls` for every call.
- If tokens are unavailable, or `unreported_calls` > 0, the cost criterion is **INCONCLUSIVE**
  and the hypothesis cannot PASS.
- Never substitute output size or character count for missing token usage.
- The token proxy threshold is a **stop threshold** with overshoot; only the call ceiling is
  a hard bound.

## 6. Pre-registration

The thresholds, arms, rubrics, corpus, ordering rules, and analysis methods are fixed before
the first experimental call. No post-run change may be used to obtain a favorable result. A
changed input is a different experiment.

## 7. EP-15

- C parallel-batch latency remains **UNVERIFIED**.
- This approval does **not** claim EP-15 is closed.
- A bounded verification proposal is separate, is not run under this approval, and an
  explicit EP-15 disposition is required before Decision 9.
- If EP-15 is not authorized, C latency is first measured in the full run, and a missing or
  invalid C timing makes H2 INCONCLUSIVE for the latency criterion.

## 8. Execution remains prohibited

This decision records the rules and the call maximum. It does **not** authorize:

- any model call or provider spending;
- case construction or adjudication;
- a corpus freeze;
- experiment execution.

**Decision 9 remains the separate execution authorization.** Decisions 5 (case checker,
sealed manifest, construction) and 6 (two adjudicators) remain open, and Decision 8 (freeze)
remains pending.

## 9. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** |
| 3 | Strict versus partial eligibility | **APPROVED 2026-11-21** |
| 4 | Proposed protocol amendment | **REJECTED (unnecessary) 2026-11-21** |
| 5 | Independent seeder | **APPROVED 2026-11-21** — construction not authorized |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost limits | **APPROVED 2026-11-21** — this record |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** |

## 10. Consistency

- The registered protocol is **byte-identical**; its SHA-256 is unchanged.
- No threshold, arm, or eligibility rule was changed.
- The freeze procedure's precondition 7 ("The thresholds and cost ceiling are approved") is
  **MET** by this decision: the thresholds and a 613-call ceiling are approved; **no dollar
  ceiling is set** because no provider price is registered. Preconditions 5 (partial) and 6
  (pending) remain open, so the freeze is not authorized.
- `candidates.json` and the conditional PRD are unchanged.
- No candidate is promoted or adjudicated.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.

## 11. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
