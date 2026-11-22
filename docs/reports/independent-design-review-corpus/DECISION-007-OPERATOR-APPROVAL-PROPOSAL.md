# Decision 7 — Operator Approval Proposal (Independent Design Review / EGM)

**Type:** Operator approval proposal (non-normative). It prepares the exact Decision 7 wording
for the operator to accept, amend, or reject.
**Status:** **PENDING OPERATOR APPROVAL — Decision 7 is not recorded.**
**Baseline:** `origin/main` @ `2c56abb9` (merge of PR #614).
**Source assessment:** [DECISION-007-THRESHOLD-COST-READINESS.md](DECISION-007-THRESHOLD-COST-READINESS.md).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §§6.1–6.2, 7.
**Decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §7.
**EP-15 evidence:** [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md),
[INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md](../INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md).

No threshold is approved, no ceiling is approved, no spending is authorized, and no model call
is made. The corpus is **NOT FROZEN** and the experiment is **NOT YET READY**.

## 1. Exact proposed operator approval

Proposed text, ready to record verbatim if the operator accepts it. It is **not recorded here**.

> Decision 7 — APPROVED
>
> **Thresholds.** The registered thresholds in the EGM assessment §§6.1–6.2 are approved
> verbatim, including the H1/H2 PASS rule (recall gain ≥25% relative and ≥3 more confirmed known
> defects, 90% interval lower bound above zero, median reproducibility ≥70%), the ≥20
> double-adjudicated-findings floor, Cohen's kappa ≥0.6, the false-positive caps, the cost and
> latency caps, the failed-run validity rules, and every INCONCLUSIVE condition.
>
> **Arm configuration.** Default 7-call configuration: A + B×3 + C, with the H1 B run sharing
> one B×3 control run. Arm D is disabled for the initial experiment. Three repetitions per plan.
> The 4-call pilot is excluded from the dataset.
>
> **Hard call ceiling.** 613 provider calls, conditional on a maximum corpus of 29 plans and the
> default 7-call configuration. The proxy must refuse calls beyond its per-run ceiling, and the
> cumulative forwarded-call count must stop at 613. The ceiling is a maximum, not a target; it
> authorizes no calls and no spending. A larger corpus, a different arm configuration, or Arm D
> requires a separate decision.
>
> **Usage accounting.** Use provider-reported token usage from `metadata.json`. Record
> `unreported_calls`. If tokens are unavailable, the cost criterion is INCONCLUSIVE. Never
> substitute output size for missing token usage.
>
> **No post-run changes.** The thresholds, arms, rubrics, corpus, ordering rules, and analysis
> methods are fixed before the first experimental call. No post-run change may be used to obtain
> a favorable result.
>
> **EP-15.** C parallel-batch latency remains **UNVERIFIED**. This approval does not claim that
> EP-15 is closed. A bounded verification proposal is prepared separately, is not run under this
> approval, and an explicit EP-15 disposition is required before Decision 9.
>
> This approval does **not** authorize any experiment call, spending, case construction,
> adjudication, corpus freeze, or execution. Decision 9 controls execution authorization.

## 2. Registered threshold matrix

All rows are registered protocol requirements; none is adjusted by this proposal.

### 2.1 Validity conditions (§6.1)

The comparison is **INCONCLUSIVE** if any holds:

| # | Condition |
| --- | --- |
| V1 | Fewer than 15 determinable known defects, or fewer than 15 plans with known defects. |
| V2 | Kappa below 0.6 on the double-adjudicated sample, or no second adjudicator and the claim would be PASS. |
| V3 | More than 10% of runs failed, or arms have unequal failure rates by more than 5 points. |
| V4 | `metadata.json` routing differs from intent; a model/provider/harness/routing override was set; or the `tokens` key does not match `routing.model`. |
| V5 | Token counts unavailable, including any call with `unreported_calls` greater than zero. |
| V6 | A provider-reported sampling parameter changed within an arm (drift recorded; reproducibility is the only guard where unreported). |
| V7 | The order, corpus, rubric, or thresholds changed after the first run. |

### 2.2 Outcome rules (§6.2, applied in order after §6.1)

| # | Rule |
| --- | --- |
| O1 | **FAIL (hard cap, point estimate):** false-positive rate > A + 10 points or > 50% absolute; token cost > 1.5× A (B) or 3.5× A (C); median latency > 1.5× A (B) or 2× A (C). |
| O2 | **PASS:** recall gain ≥25% relative and ≥3 more confirmed known defects than the comparator; 90% interval lower bound > 0; median-across-plans reproducibility ≥70%. A PASS also requires ≥20 double-adjudicated findings. |
| O3 | **FAIL (no useful effect):** upper bound of the 90% interval on the recall gain < 10% relative. |
| O4 | **INCONCLUSIVE:** everything else, including a positive point estimate whose interval includes zero. |

### 2.3 Per-hypothesis comparators and caps (§6.2)

| Hypothesis | Comparison | Comparator | FP cap | Cost cap | Latency cap |
| --- | --- | --- | --- | --- | --- |
| H1 (rubric) | B vs A | A, paired by plan | ≤ A + 10 points; ≤ 50% absolute | B ≤ 1.5× A | median B ≤ 1.5× A |
| H2 (specialized) | C vs B×3 | union of three independent B runs | ≤ A + 10 points; ≤ 50% absolute | C ≤ 3.5× A | median C ≤ 2× A |
| H3 (alternate model) | B_D / C_D vs B / C | same-request B and C under the other model | report only | report only | report only |

Hard-cap breaches are point-estimate stop rules; PASS uses the interval lower bound. A hard-cap
breach outranks a recall PASS. If tokens are unavailable, the cost hard cap is not applied and
the hypothesis is INCONCLUSIVE, not PASS.

## 3. Recommended arm configuration

| Element | Recommendation | Why |
| --- | --- | --- |
| Base configuration | **Default 7-call** (A + B×3 + C) | Registered; the H1 B run shares one B×3 control run, avoiding a redundant fourth B call. |
| H1 B run | **Shares one control run** | §4 explicitly permits 7 calls; the designation is fixed before the run, not selected after results. |
| Arm D | **Disabled** | Optional and exploratory; it adds 348–609 calls and a model-diversity claim that needs a separate decision. |
| Repetitions | **Three per plan per arm** | §5.5; the plan-level value is the mean of three. |
| Pilot | **4 calls (A and B on two plans), excluded** | §6.4; validates metadata, attribution, and token plumbing only. |
| Per-plan calls | **7** | 1 A + 3 B (control, one shared as H1 B) + 3 C. |

This configuration is the registered default; it does not change any rule. The disjoint 8-call
variant is an alternative that requires its own approval (see §9).

## 4. Hard call-ceiling calculation

`4 + (3 × 29 × 7) = 4 + 609 = 613` provider calls.

- 4 = the pilot (arms A and B on two plans).
- 3 = repetitions per plan per arm.
- 29 = the maximum corpus size in the registered target (12–15 historical, about 8 seeded,
  about 6 no-known-defect).
- 7 = calls per plan per repetition in the default configuration.

**What 613 is:** an aggregate experiment maximum, not a target and not a spending authorization.
It is a **provider-call** ceiling, not a dollar or token-spend ceiling.

**How it is enforced.** The `internal/expharness` proxy enforces a **hard per-run** call ceiling
(`--max-calls`) and refuses without forwarding beyond it. The 613 aggregate must therefore be
enforced operationally: allocate per-run ceilings whose sum is ≤ 613, track the cumulative
forwarded calls in the run manifest or the analyst log, and stop when the cumulative total
reaches 613. An aggregate-enforcing wrapper is **not implemented**; this is a recorded
operational limitation, not a threshold change.

**Caveats.** A corpus larger than 29 plans, the disjoint 8-call configuration (700 at 29 plans),
or Arm D (up to 1,309) each requires a separate ceiling decision. The final corpus size and the
exact planned call count must be recorded before execution, and must be ≤ the approved maximum.

## 5. Usage-accounting rule

- Use provider-reported token usage from `metadata.json` (per plan).
- Record `unreported_calls` for every call.
- If tokens are unavailable, or `unreported_calls` > 0, the cost criterion is **INCONCLUSIVE**
  and the hypothesis cannot PASS.
- Never substitute output size or character count for missing token usage.
- The token proxy threshold is a **stop threshold**, not a hard bound: the crossing request and
  in-flight requests can overshoot. Only the call ceiling is a hard bound.

## 6. EP-15 disposition

- C parallel-batch latency remains **UNVERIFIED**. The hardening work verified the procedure and
  instruments on a synthetic runtime; it did not measure a real provider's concurrency or
  latency.
- This approval must not claim EP-15 is closed.
- A bounded verification proposal is prepared separately (three concurrent real requests, one
  project each, under separate authorization) and is **not run** under this approval.
- An explicit EP-15 disposition is required before Decision 9. If it is not authorized, C
  latency is first measured in the full run, and a missing or invalid C timing makes H2
  INCONCLUSIVE for the latency criterion.

## 7. Remaining operational limitations

| Limitation | Effect |
| --- | --- |
| Corpus below the floor (3 strict plans / 4 defects vs 15/15) | Any run now is INCONCLUSIVE before thresholds apply. |
| Decision 6 pending (no two adjudicators) | Recall and kappa are unavailable. |
| Decision 5 controls pending (case checker, sealed manifest, construction) | No seeded/control cases; no false-positive control. |
| EP-15 UNVERIFIED | C latency may be INCONCLUSIVE. |
| EP-02 (model identity) and EP-12 (reproducibility) | Operator dispositions required. |
| No aggregate ceiling wrapper | The 613 ceiling is tracked operationally, not enforced by one proxy. |
| Token stop threshold, not a hard bound | Worst-case spend can overshoot by in-flight responses. |
| `small` model class | Do not use it; it carries an unremovable cloud fallback candidate. |

## 8. Decision 9 controls execution

This proposal, if approved, records the rules and the call maximum. It does **not** authorize
execution. Decision 9 must separately record the start condition, the approved budget, the stop
condition, the provider and model, and the EP-15 disposition. Until Decision 9 is recorded, no
call is authorized.

## 9. Alternatives requiring separate operator decisions

| Alternative | Effect | Separate decision |
| --- | --- | --- |
| Disjoint 8-call configuration | 700 calls at 29 plans | Yes |
| Arm D enabled | +348 / +522 / +609 calls | Yes |
| Corpus larger than 29 plans | Recompute the ceiling | Yes |
| Dollar/token-spend ceiling | Requires a provider price | Yes; none is registered |
| EP-15 bounded verification | Three real concurrent requests | Yes, and not under this approval |
| Per-plan or stratified sampling | Changes the approved sampling rule | Yes |
| Reconciliation / tie-breaking | Adds a rule the protocol does not define | Yes |

## 10. PENDING status and non-authorizations

- **Decision 7 is PENDING.** This proposal records nothing.
- No threshold, arm, or ceiling is approved.
- No provider spending, live inference, or experiment execution is authorized.
- No candidate is promoted; no case is constructed; no adjudicator or case checker is designated.
- The corpus is **NOT FROZEN**; the experiment is **NOT YET READY**; H1/H2 are **NOT
  ESTABLISHED**.
- No `DECISION-RECORD-007` exists.

## 11. Validation performed

- The thresholds were re-read at `2c56abb9` from §6.1–§6.2 and quoted without reinterpretation.
- The call arithmetic was recomputed: `4 + 3 × 29 × 7 = 613`; the readiness report's other
  totals were checked against §7.
- The proxy behavior was verified from `internal/expharness/proxy.go` and
  `TestProxyCeilingsRefuseWithoutForwarding`: the call ceiling is hard; the token threshold is a
  stop threshold with overshoot.
- No protocol, threshold, arm, or ceiling was changed; no live call was made.
