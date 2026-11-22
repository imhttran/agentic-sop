# Decision 6 — Procedural Clarifications Approval (Independent Design Review / EGM)

**Status: APPROVED — PROCEDURAL CLARIFICATIONS ONLY (2026-11-21).**
**Operator:** Tom Tran.
**Approval date:** **2026-11-21** (the operator's stated approval date; the record was written
on that date, not substituted from the implementation date).
**Source:** PR #606, merged as `2e16416e`, and its proposal
[DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md](DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) — unchanged.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — the three procedural clarifications only |
| Scope | sampling universe; disagreement handling; second-adjudicator blindness |
| Decision 6 — adjudicator designation | **PENDING** |
| Adjudicators designated | **0** |
| Independent case checker | **NOT DESIGNATED** |
| Sealed-manifest location | **NOT SELECTED** |
| `case_construction_authorized` | **false** |
| Registered protocol SHA-256 | `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c` — unchanged |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `2e16416e` (2026-11-21).

## 1. Recorded approval

> The operator approves the three procedural clarifications proposed in PR #606. This
> authorization applies only to recording the clarifications in governance documentation. It
> does not approve Decision 6 adjudicator designation or any experiment activity.

**Approval 1 — Pooled sampling.** The second adjudicator's sampling universe consists of all
distinct findings after the registered within-run merge. For N findings, use
`k = min(N, max(20, ceil(0.20 × N)))`. Select findings randomly from the pooled universe and
present them in the prescribed per-plan shuffled order. The sampling algorithm, rounding
convention, and random seed must be registered before execution. Observed N, k, and selected
finding IDs must be recorded after execution, not in the pre-execution freeze record.

**Approval 2 — Preserve independent adjudication labels.** Preserve both adjudicators'
original independent labels; calculate Cohen's kappa from them; report disagreements without
silently reconciling or overwriting them; do not introduce mandatory reconciliation or a new
tie-breaking mechanism; preserve the registered PASS and INCONCLUSIVE criteria.

**Approval 3 — Second-adjudicator blindness.** The second adjudicator must not see the first
adjudicator's labels before submitting independent judgments. Both adjudicators must remain
blind to prohibited experiment metadata and the sealed answer key. Comparison of judgments may
occur only after both independent submissions are complete.

The operator identity is **Tom Tran**. No signature, time of day, or additional authorization
is recorded; none was given.

## 2. Protocol compatibility

All three are **clarifications**, not amendments:

| Interpretation | Status | Effect |
| --- | --- | --- |
| Pooled sampling with `k = min(N, max(20, ceil(0.20 × N)))` | **Clarification** | No numeric or rule change |
| Preserve independent labels; report disagreements | **Clarification** | No rule added; kappa remains valid |
| Second blind until both submit | **Clarification** | Required by "independently" |

The registered protocol is **byte-identical**; its SHA-256 is unchanged. The numeric minimums,
PASS/INCONCLUSIVE rules, Cohen's kappa requirement, double-adjudication minimum, historical
eligibility rules, Decision 4's rejection, and existing candidate dispositions are unchanged.
No formal protocol amendment was required, and none is made.

## 3. Sampling reproducibility requirements

These are the approved, pre-registered requirements.

**The pre-execution record must specify:**

- **Sampling universe:** all distinct findings after the registered within-run merge.
- **Within-run merge dependency:** the merge is applied by the analyst before the inventory is
  built (§5.4).
- **Rounding convention:** `ceil` on a fractional 20%, as written in the approved formula.
- **Sampling algorithm:** the deterministic method used to draw `k` from the pooled universe.
- **Random seed:** the seed, and how it is stored and protected.
- **Required evidence fields:** the fields the post-run artifact must carry.
- **Sample identity:** how the selected findings are preserved and linked to the algorithm and
  seed.

**The post-run evidence must contain:**

- the observed total finding count `N`;
- the calculated sample size `k`;
- the selected finding IDs;
- the linkage between the selected sample and the registered algorithm and seed.

The observed `N`, `k`, and selected IDs are recorded **after** execution, in the run manifest
or a separate post-run evidence artifact — not in the pre-execution freeze record, which is
committed and hashed before execution (freeze §4–§5).

No seed was generated, no finding was selected, and no run manifest was created by this
record.

## 4. What this approval does not cover

This record approves **only** the three clarifications above. It does **not** approve:

- **Decision 6 adjudicator designation** or the complete Decision 6 decision.
- Any adjudicator, the independent case checker, or a sealed-manifest location.
- Case construction, adjudication, corpus freeze, model inference, or experiment execution.
- Any change to the protocol, thresholds, arms, cost ceilings, eligibility rules, or
  candidate dispositions.

PR #606 contains operational recommendations beyond the three approved clarifications. They
are **not** approved by this record and are flagged for a separate operator decision:

1. **The specific PRNG algorithm example** ("for example, SHA-256 counter mode"). The
   operator approved registering *a* sampling algorithm; this record does not select one.
2. **The presentation shuffle seed**, which is separate from the sampling seed. The operator
   approved registering the sampling seed; the presentation seed is not explicitly approved.
3. **Per-plan or stratified sampling** and **mandatory reconciliation / tie-breaking** — the
   proposal analyses them as additions; they are not approved.
4. **Any new storage or access control** beyond the existing protocol and freeze controls.

## 5. Outstanding Decision 6 requirements

Decision 6 remains **PENDING** because no two qualified independent human adjudicators are
designated. To complete Decision 6, the operator must still:

1. designate two independent human adjudicators;
2. record each candidate's authorship/editing check, independence, and conflict of interest;
3. confirm they are distinct from each other, the seeder (Tom Tran), the corpus owner (Tom
   Tran), and the analyst;
4. record the second-adjudication sampling rule and the blinded defect list;
5. record the conflict-of-interest record required by freeze §3.10.

The repository shows only Tom Tran as a human identity; he is the corpus owner and seeder and
cannot adjudicate. No qualified external human is identified. Without two independent human
adjudicators, the best attainable outcome is **INCONCLUSIVE** (§5.4).

## 6. Consistency

- Decisions 1–5 are unchanged; Decisions 7–9 remain pending.
- `candidates.json` is unchanged; the proposed strict corpus remains 3 plans / 4 defects and
  the proposed partial corpus 11 plans / 11 defects.
- No candidate is promoted or adjudicated; the adjudicated count remains **0**.
- No freeze precondition is marked complete by this approval. The adjudicator precondition
  (Decision 6) remains open.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.
- The conditional PRD is unchanged.

## 7. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
