# Decision 6 — Adjudication Procedure Clarification (Independent Design Review / EGM)

**Type:** Procedural-clarification proposal (non-normative). It analyses three ambiguities in
the registered adjudication procedure and recommends interpretations. It approves nothing and
authorizes no experiment activity.
**Status:** **APPROVED — PROCEDURAL CLARIFICATIONS ONLY (2026-11-21).** The three
interpretations in §§3–5 are approved in
[DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md).
Decision 6 adjudicator designation remains **PENDING**. Other operational recommendations
in this proposal are **not** approved.
**Baseline:** `origin/main` @ `f2a10fb2` (merge of PR #604).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) — authoritative.
**Qualification package (input):** [DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §3.10.
**Readiness:** [READINESS.md](READINESS.md) §17.

No adjudicator is designated. No finding exists, no adjudication has occurred, and no kappa has
been computed. The corpus is **NOT FROZEN** and the experiment is **NOT YET READY**.

## 1. Purpose and scope

The qualification package (PR #604) recorded three unresolved ambiguities in the adjudication
procedure: the sampling universe, disagreement handling, and second-adjudicator blindness.
This proposal analyses each against the exact protocol wording, recommends an interpretation
with trade-offs, and proposes operator decision wording.

It is a clarification proposal, not a decision record. It does **not** approve Decision 6,
designate or contact an adjudicator, authorize adjudication, construct cases, change the
protocol or its hash, freeze the corpus, or run inference. It does not create
`DECISION-RECORD-006`.

## 2. Exact protocol references

### 2.1 §5.4 — blinded adjudication

> Findings are exported with arm, repetition, model and run ids removed, shuffled per
> plan, and presented in that order.

> For each finding the adjudicator records one of: matches known defect `<id>`; valid
> and not on the list (needs a one-line justification from the document); invalid or
> immaterial; duplicate of finding `<n>`. Judgment is against the document and the
> pre-registered defect list, never the reviewer's stated confidence.

> **Independence.** A human adjudicates. A second person independently adjudicates the
> larger of 20 findings or a random 20% sample, and agreement is reported (Cohen's
> kappa). If fewer than 20 findings exist in total, kappa is reported but the outcome
> cannot PASS. A model may serve only as a third check, never as the sole adjudicator.

> **Minimum staffing.** A PASS requires two distinct human adjudicators who are not
> authors of any plan in the corpus. They must be distinct from each other and from the
> seeder of any defect they adjudicate. The corpus owner, seeder and analyst may overlap
> with each other and with the operator, but none of them may be an adjudicator, and the
> adjudicators remain blind.

> **Role duties.** … the adjudicators label findings; the second adjudicator
> re-adjudicates the sample.

### 2.2 §5.7 — unit of analysis

> The unit of analysis is the plan. Differences are paired by plan.

### 2.3 §6.1 — validity conditions

> kappa below 0.6 on the double-adjudicated sample, or no second adjudicator and the
> claim would be PASS;

### 2.4 §6.2 — PASS floor

> A PASS additionally requires at least 20 double-adjudicated findings; below that, the
> outcome is INCONCLUSIVE even if the recall rules would otherwise pass.

### 2.5 §5.8 — run manifest

> the full-run order; … the named holder of each role (corpus owner, seeder, analyst,
> adjudicator, second adjudicator).

### 2.6 Existing governance controls

- Freeze §3.10 requires the adjudicator identities, their independence and authorship
  checks, the second-adjudication sampling rule, and the conflict-of-interest record.
- The case-construction specification §6.2 separates the **blinded defect list** given to
  adjudicators from the **sealed answer key**.

## 3. Ambiguity 1 — sampling universe

### 3.1 What the protocol says

- Findings are shuffled **per plan** for presentation.
- The second adjudicator re-adjudicates "the larger of 20 findings or a random 20% sample."
- "If fewer than 20 findings exist **in total**, kappa is reported but the outcome cannot
  PASS."
- A PASS requires "at least 20 double-adjudicated findings."

### 3.2 What is ambiguous

The protocol does not say whether the "20 findings or 20% sample" is drawn **per plan** or
from the **pooled** set of all findings. "Shuffled per plan" describes presentation, not
necessarily sampling; "in total" describes the count, not necessarily the sampling universe.

### 3.3 Interpretations

| Option | Rule | Assessment |
| --- | --- | --- |
| **A. Pooled (recommended)** | One sample from all distinct findings; `k = min(N, max(20, ceil(0.20 × N)))` | Consistent with "in total" and the total PASS floor. |
| B. Per-plan | Sample `max(20, 20% of that plan's findings)` in every plan | Guarantees per-plan coverage but can require far more than 20 double-adjudications and conflicts with "in total". |
| C. Stratified | Allocate the pooled `k` across plans proportionally or by a minimum | A middle ground, but a new allocation rule the protocol does not state. |

### 3.4 Evidence and trade-offs

For pooled:

- "If fewer than 20 findings exist **in total**" is a total-count statement.
- §6.2's floor is a **total** of double-adjudicated findings, not a per-plan count.
- §5.7 makes the **plan** the unit of the recall analysis, but the double-adjudication is an
  agreement check, not a per-plan metric.
- One pooled `k` is the only reading under which "the larger of 20 findings or a random 20%
  sample" is a single, well-defined number.

Against pooled:

- A small plan may contribute few or no findings to the sample; per-plan kappa is not
  available. This does not affect the protocol's metrics, which use the first adjudicator's
  labels for all findings and the pooled kappa only as a validity check.

For per-plan:

- It guarantees that every plan has double-adjudicated findings, but it is not required by
  any protocol text and can inflate the workload without changing the PASS rule.

### 3.5 Recommendation

**Adopt pooled sampling** with `k = min(N, max(20, ceil(0.20 × N)))`, where `N` is the number
of distinct findings after the within-run merge. Present the sampled findings in the
protocol's per-plan shuffled order.

`ceil` is a **proposed rounding convention**, not a protocol requirement: the protocol does
not say how a fractional 20% is rounded. Recording the convention keeps the sample
reproducible.

### 3.6 Protocol compatibility

**A clarification, not an amendment.** It reads an ambiguous term consistently with "in
total"; it changes no numeric minimum, PASS/INCONCLUSIVE rule, arm, threshold, or cost
ceiling. Per-plan or stratified sampling would be an **addition** to the protocol, not a
clarification, and would require a separate operator decision.

## 4. Ambiguity 2 — disagreement handling

### 4.1 What the protocol says

- "agreement is reported (Cohen's kappa)".
- The protocol defines **no** reconciliation, tie-break, or consensus procedure.
- The metrics use the adjudicated labels; the second adjudicator re-adjudicates the sample.

### 4.2 What is ambiguous

It does not say whether a disagreement is left as two independent labels, reconciled by a
third party, or replaced by a consensus label before analysis.

### 4.3 Interpretations

| Option | Rule | Assessment |
| --- | --- | --- |
| **A. Preserve labels (recommended)** | Keep both independent label sets; compute kappa; report disagreements; no reconciliation. | Faithful to kappa and to "independently". |
| B. Reconcile before kappa | Merge to one label, then compute kappa. | **Invalid**: reconciliation destroys the independence kappa measures. |
| C. Preserve for kappa, then reconcile separately | Report kappa on independent labels; optionally produce a clearly-labelled consensus set for other uses. | Permitted if the consensus set is never used for kappa. |

### 4.4 Evidence and trade-offs

- Cohen's kappa measures agreement between **independent** raters. Reconciling first makes the
  statistic uninterpretable.
- The protocol assigns the primary labels to "the adjudicator" and the agreement check to the
  second; it does not instruct the second to replace the first's labels.
- Preserving labels leaves a disagreed finding without a single "final" label, but the
  protocol does not require one; the disagreement is reported.

### 4.5 Recommendation

**Preserve both independent label sets**; compute kappa on them; report the disagreement count
and the specific disagreements; do **not** silently reconcile. If a consensus set is wanted,
record it as a separate artifact and never use it for kappa.

### 4.6 Protocol compatibility

**A clarification, not an amendment.** It keeps the protocol's stated statistic and adds no
rule. A mandatory reconciliation procedure would be an **addition** and would require a
separate decision.

## 5. Ambiguity 3 — second-adjudicator blindness

### 5.1 What the protocol says

- "A second person **independently** adjudicates …"
- "the adjudicators remain blind";
- "The adjudicator does not see arm labels."

### 5.2 What is ambiguous

It does not explicitly say whether the second adjudicator must submit labels **before** seeing
the first adjudicator's judgments.

### 5.3 Interpretations

| Option | Rule | Assessment |
| --- | --- | --- |
| **A. Blind until both submit (recommended)** | The second submits without seeing the first's labels; the analyst joins afterwards. | Required for kappa to measure independent agreement. |
| B. Show the first's labels first | The second confirms or edits. | **Invalid for kappa**: anchoring inflates agreement. |
| C. Show labels only after a sealed submission | Same as A, with a hashed/signed submission and an audit trail. | Equivalent to A with extra assurance. |

### 5.4 Evidence and trade-offs

- "Independently" and the kappa statistic only mean something if the second does not anchor on
  the first.
- Sequential submission takes longer and requires coordination, but parallel or sequential
  blind submission is the only defensible reading.
- The analyst can hold both submissions and perform the join deterministically.

### 5.5 Recommendation

**Keep the second adjudicator blind to the first's labels until both independent submissions
are complete.** The second receives the same authorized materials (pre-fix artifact, permitted
references, blinded defect list, rubric) and no first-adjudicator labels. The analyst joins
the two submissions after both are recorded.

### 5.6 Protocol compatibility

**A clarification, not an amendment.** Showing the first's labels first would **contradict**
"independently" and invalidate kappa; it is not a permissible variant.

## 6. Protocol compatibility summary

| Interpretation | Status | Effect |
| --- | --- | --- |
| Pooled sampling with `k = min(N, max(20, ceil(0.20 × N)))` | **Clarification** | No numeric or rule change |
| Preserve independent labels; report disagreements | **Clarification** | No rule added; kappa remains valid |
| Second blind until both submit | **Clarification** | Required by "independently" |
| Per-plan or stratified sampling | **Addition** (would need a decision) | New allocation rule |
| Mandatory reconciliation | **Addition** (would need a decision) | New rule; must not feed kappa |
| Showing the first's labels before submission | **Incompatible** | Invalidates kappa |

**Conclusion:** none of the three recommended interpretations requires a formal protocol
amendment. The protocol text and its SHA-256 remain unchanged. Each is an interpretation the
operator should record before the first run so the procedure is pre-registered rather than
chosen after seeing data.

## 7. Reproducible sampling procedure (proposed, not executed)

1. The analyst applies the within-run merge (§5.4).
2. The analyst builds a finding inventory with **blinded** finding IDs; no arm, repetition,
   model, provider, or run identifiers.
3. `N` = the number of distinct findings in the inventory.
4. `k = min(N, max(20, ceil(0.20 × N)))`.
5. If `N < 20`, `k = N`: all findings are double-adjudicated, kappa is reported, and PASS is
   impossible (§5.4, §6.2).
6. Draw `k` findings without replacement using a documented deterministic method over the
   sorted blinded IDs (for example, SHA-256 counter mode keyed by an operator-recorded seed).
7. **Pre-register** the algorithm, the rounding convention, and the seed in the freeze record,
   which is committed and hashed before execution. **Record the observed** `N`, `k`, and the
   selected finding IDs after the run in the run manifest or a separate post-run evidence
   artifact, **not** in the frozen record: `N` and `k` cannot exist before the run, and adding
   them later would change the record whose hash the runs reference (freeze §4–§5).
8. Present the sampled findings in the protocol's per-plan shuffled order; record the
   presentation seed.
9. Withhold the first adjudicator's labels for the sampled findings until the second submits.
10. After both submissions, join the labels and compute Cohen's kappa.

The sample seed is a **recommended safeguard**: §5.8 records the run-order seed, not an
adjudication-sample seed, so pre-registering it in the freeze record makes the sample
reproducible without changing the protocol. The observed counts and selected IDs belong to the
post-run evidence, not the frozen record.

## 8. Evidence-handling requirements

- **Authorized to adjudicators:** the pre-fix artifact, the permitted references that existed
  at the pin, the blinded defect list, and the rubric.
- **Not authorized:** arm, repetition, model, provider, or run identities; the sealed answer
  key; later fixes, reports, or remediation plans.
- **First adjudicator's labels:** withheld from the second until both submissions are complete.
- **Analyst:** holds the unblinding key and performs the deterministic join.
- **Storage:** the sample, both label sets, and the seeds live outside every reviewer project
  and exported reviewer tree.
- **Leakage:** the reviewer tree contains no findings, labels, sample, or defect list.
- **Disagreements:** reported as-is; no silent reconciliation for kappa.

## 9. Proposed operator decision wording

The operator may record this clarification together with Decision 6. It is **not recorded
here**, and Decision 6 remains PENDING.

> Decision 6c — Adjudication procedure clarification: APPROVED
>
> Sampling universe: pooled across all distinct findings after the within-run merge;
> `k = min(N, max(20, ceil(0.20 × N)))`. If `N < 20`, all findings are double-adjudicated,
> kappa is reported, and PASS is impossible.
>
> Disagreement handling: both independent label sets are preserved; kappa is computed on them;
> disagreements are reported without reconciliation. No consensus set is used for kappa.
>
> Second-adjudicator blindness: the second adjudicator submits labels before seeing the first
> adjudicator's labels.
>
> The protocol text, numeric minimums, PASS/INCONCLUSIVE rules, arms, thresholds, and cost
> ceilings are unchanged. This records an interpretation only; it does not approve Decision 6,
> designate an adjudicator, or authorize adjudication.

## 10. PENDING status and non-authorizations

- **Decision 6 remains PENDING.** This proposal is not approved.
- No adjudicator is designated or contacted.
- No adjudication is authorized; no finding, sample, or kappa exists.
- No seeded or control case is constructed; no sealed manifest exists.
- No historical candidate is promoted or adjudicated.
- The registered protocol and its hash are unchanged.
- No threshold, arm, or cost ceiling changes.
- No corpus freeze, model inference, or experiment execution occurs.
- No `DECISION-RECORD-006` is created.

## 11. Validation performed

- Every quoted protocol passage was re-read at `origin/main` `f2a10fb2`; §§5.4, 5.7, 5.8, 6.1,
  and 6.2 are cited verbatim.
- The registered protocol is byte-identical (SHA-256 `abba919e…`).
- The qualification package and its three recorded ambiguities were read; this proposal
  analyses them without altering it.
- No findings were generated, no adjudication simulated, and no kappa computed.
