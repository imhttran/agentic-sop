# Decision 7 — Threshold and Cost Readiness (Independent Design Review / EGM)

**Type:** Readiness assessment (non-normative). It verifies the registered thresholds, the cost
model, and the execution-feasibility dependencies. It approves nothing and runs nothing.
**Status:** **ASSESSMENT ONLY — Decision 7 remains PENDING.**
**Baseline:** `origin/main` @ `a3e238da` (merge of PR #612).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §§4, 5.1, 5.5–5.8, 6.1–6.4, 7 — authoritative.
**Decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §7.
**Readiness:** [READINESS.md](READINESS.md).
**Workstream B:** [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md),
[INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md](../INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md).

No threshold is approved, no budget is authorized, no run is authorized, and no model call is
made. The corpus is **NOT FROZEN** and the experiment is **NOT YET READY**.

## 1. Registered requirement matrix

Labels: **R** = registered protocol requirement; **E** = registered but estimated or dependent
on unknowns; **O** = operator approval still required.

### 1.1 Arms and repetitions

| # | Requirement | Basis | Source |
| --- | --- | --- | --- |
| 1 | Every arm is `sop prompt --capability review --file <request>`, no `--cache`, JEV triage disabled; arms differ only in the request file. | R | §4 |
| 2 | Arms: A generic (1 call), B rubric (1), C specialized (3 parallel), D optional (B and C on another provider/class). | R | §4 |
| 3 | C is compared with a B×3 control (three independent B runs), union-matched. | R | §4 |
| 4 | The H1 B run may share one control run (7 calls/plan/repetition) or be disjoint (8 calls). The choice is fixed before the run; post-hoc selection makes H1 INCONCLUSIVE. | R | §4 |
| 5 | Three repetitions per plan per arm, random order; per-plan value is the mean of the three. | R | §5.5 |

### 1.2 Metrics and statistics

| # | Requirement | Basis | Source |
| --- | --- | --- | --- |
| 6 | Recall = known defects matched ÷ known defects, per run; plan value is the mean of three repetitions. | R | §5.6 |
| 7 | Precision = confirmed distinct findings ÷ all distinct findings; false-positive rate = invalid+immaterial distinct findings ÷ all distinct findings, per run and separately on the no-known-defect set. | R | §5.4, §5.6 |
| 8 | Reproducibility = share of confirmed distinct findings recurring in ≥2 of 3 repetitions; PASS gate uses the median across plans, ≥70%. Fewer than 15 contributing plans for an arm makes that arm's gate INCONCLUSIVE. | R | §5.5 |
| 9 | Unit of analysis is the plan; differences are paired by plan. | R | §5.7 |
| 10 | 90% cluster-bootstrap confidence intervals over plans, at least 10,000 resamples. | R | §5.7 |
| 11 | Agreement is Cohen's kappa on the double-adjudicated sample; kappa < 0.6 is a validity failure. | R | §5.4, §6.1 |
| 12 | A PASS additionally requires at least 20 double-adjudicated findings. | R | §6.2 |

### 1.3 Outcome and stopping rules

| # | Rule | Basis | Source |
| --- | --- | --- | --- |
| 13 | H1 PASS: recall gain ≥25% relative **and** ≥3 more confirmed known defects than A; 90% CI lower bound > 0; median reproducibility ≥70%. | R | §6.2 |
| 14 | H2 PASS: same recall rule against the B×3 control. | R | §6.2 |
| 15 | H3 (Arm D) is exploratory and cannot by itself change a recommendation; report only. | R | §5.7, §6.2 |
| 16 | FAIL (hard caps, point estimate): false-positive rate > A + 10 points or > 50% absolute; cost > 1.5× A (B) or 3.5× A (C); median latency > 1.5× A (B) or 2× A (C). | R | §6.2 |
| 17 | FAIL (no useful effect): upper bound of the 90% CI on the recall gain is below 10% relative. | R | §6.2 |
| 18 | INCONCLUSIVE: everything else, including a positive point estimate whose interval includes zero. | R | §6.2 |
| 19 | INCONCLUSIVE validity: fewer than 15 determinable defects or 15 plans with known defects; kappa < 0.6 or no second adjudicator; > 10% of runs failed or unequal arm failure rates by > 5 points; routing/override/token mismatch; corpus/order/rubric/threshold changed after the first run. | R | §6.1 |
| 20 | A hard-cap breach outranks a recall PASS; a cost criterion that is INCONCLUSIVE because tokens are unavailable is not a PASS. | R | §6.2 |

### 1.4 Cost, failure, and manifest

| # | Requirement | Basis | Source |
| --- | --- | --- | --- |
| 21 | Call budget = corpus size × arms × repetitions + 4-call pilot + optional Arm D. | R | §7 |
| 22 | Three repetitions per plan per arm. | R | §5.5, §7 |
| 23 | Token cost is measured from `metadata.json` per plan; if the provider does not report tokens, cost is INCONCLUSIVE and output size is not substituted. | R | §5.6, §7 |
| 24 | A failed call is recorded as a failed run and counted, not dropped; no retry beyond SOP's own behavior. | R | §5.5 |
| 25 | The pilot is arms A and B once on two plans (4 calls), excluded from the dataset. | R | §6.4 |
| 26 | The run manifest states the exact arm configuration, order, role holders, provider/model, environment state, and timing method before the first call. | R | §5.8 |
| 27 | The thresholds and the call/cost ceiling must be approved before the run; no threshold may change after the first run. | O | §6, §6.2; decision 7 |

## 2. Threshold implementation coverage

| Requirement | Implemented? | Evidence / gap |
| --- | --- | --- |
| Token accounting | **Yes (SOP primitive)** | `internal/perf` records `Calls`, `Input`, `Output`, `UnreportedCalls`; `internal/runmetrics` aggregates. Offline tests pass. |
| Call and token limits | **Partial (experiment harness)** | The `internal/expharness` proxy enforces a **hard call ceiling** (it refuses without forwarding). The **token threshold is a stop threshold**, checked against responses already received: the crossing request and any in-flight requests can overshoot it. The values are a Decision 7 input, not yet approved. |
| Comparative cost caps (1.5× A / 3.5× A) | **No code** | Post-hoc analyst calculation from measured tokens; no evaluator exists. |
| Latency measurement | **Partial** | SOP and the proxy record per-request duration; C parallel-batch latency is **UNVERIFIED** (EP-15). |
| Recall / precision / false-positive rate | **No EGM-specific code** | Manual adjudication plus analyst calculation; no evaluator. |
| Cohen's kappa | **No code** | Manual calculation after both adjudications. |
| 90% cluster-bootstrap CI | **No code** | Manual/analyst; no implementation. |
| Reproducibility | **No code** | Manual cross-repetition duplicate linking (§5.5). |
| Threshold / stopping-rule evaluation | **No code** | Manual, pre-registered analysis; no automated gate. |
| Failed-run rate and arm failure comparison | **No EGM-specific code** | SOP's failure classifier exists (`internal/failure`, tests pass), but the EGM failed-run rate is an analyst count. |
| Run-manifest validation | **No EGM validator** | `internal/expharness.Attribute` assembles a per-run attribution record; the EGM run manifest is an external experiment record (§5.8) with no validator. |
| Corpus size floor | **Not met** | Proposed strict corpus is 3 plans / 4 defects against 15/15. |

**Conclusion:** the thresholds are fully registered but not implemented as an evaluator. The
experiment's analysis is manually pre-registered and analyst-executed. The only hard enforced
limit is the proxy's **call ceiling**; the token threshold is a stop threshold with overshoot,
and the comparative caps are post-hoc rules.

## 3. Cost and call-budget calculations

The following are the registered counts, reproduced exactly from §7. They are **registered
requirements**, not estimates.

**Calls per plan per repetition**

| Configuration | Calls |
| --- | --- |
| A only | 1 |
| A + B (H1) | 2 |
| A + B×3 + C (default; H1 B shares a control run) | 7 |
| A + B_H1 + B×3 + C (disjoint) | 8 |
| Optional D, B + C | +4 |
| Optional D, B×3 + C | +6 |
| Optional D, B_H1 + B×3 + C | +7 |

**Totals by corpus size** (three repetitions)

| Configuration | 15 plans | 26 plans | 29 plans |
| --- | --- | --- | --- |
| A only | 45 | 78 | 87 |
| A + B | 90 | 156 | 174 |
| Default 7-call | 315 | 546 | 609 |
| Disjoint 8-call | 360 | 624 | 696 |
| Optional D, B + C | +180 | +312 | +348 |
| Optional D, B×3 + C | +270 | +468 | +522 |
| Optional D, B_H1 + B×3 + C | +315 | +546 | +609 |

**Combined totals including the 4-call pilot**

| Configuration | 15 plans | 26 plans | 29 plans |
| --- | --- | --- | --- |
| Base, default 7-call | 319 | 550 | 613 |
| Base, disjoint 8-call | 364 | 628 | 700 |
| Base + D (B+C), default | 499 | 862 | 961 |
| Base + D (B×3+C), default | 589 | 1,018 | 1,135 |
| Base + D (B_H1+B×3+C), default | 634 | 1,096 | 1,222 |
| Base + D (B+C), disjoint | 544 | 940 | 1,048 |
| Base + D (B×3+C), disjoint | 634 | 1,096 | 1,222 |
| Base + D (B_H1+B×3+C), disjoint | 679 | 1,174 | 1,309 |

**Symbolic form.** `total_calls = 4 + 3 × P × C + D`, where `P` is the number of plans, `C` is
7 (default) or 8 (disjoint), and `D` is the optional-Arm-D increment above. The range without
Arm D is **319–700 calls**; with Arm D it is **499–1,309 calls**.

**Token cost.** No dollar estimate is registered. Token cost is measured per plan from
`metadata.json`; if the provider does not report tokens, the cost criterion is INCONCLUSIVE and
output size is not substituted. A **hard call ceiling** is the provider-neutral enforceable
bound. The **token threshold is a stop threshold, not a hard bound**: the request that crosses
it and any in-flight requests can overshoot, so worst-case spend is the threshold plus the
output of in-flight requests. A dollar ceiling would require a provider price, which is not
registered.

**Human cost.** Every finding is adjudicated, and the larger of 20 findings or a random 20%
sample is double-adjudicated. The finding count is unknown before the run, so no finding count
is invented. Adjudication is the dominant human cost.

## 4. Offline verification results

Deterministic, no live provider call. Commands run in an isolated worktree at `a3e238da`:

| Check | Command | Result |
| --- | --- | --- |
| Token/metric accounting | `go test ./internal/runmetrics -count=1` | **PASS** |
| Failure classification (SOP primitive) | `go test ./internal/failure -count=1` | **PASS** |
| Token struct/aggregation | `go test ./internal/perf -count=1` | **PASS** |
| Proxy call ceiling and token threshold | `go test ./internal/expharness -run 'TestProxyCeilingsRefuseWithoutForwarding|TestProxyKeepsUpstreamHost' -count=1` | **PASS** |

**What this establishes:** the cost-accounting primitives and the absolute ceiling enforcement
work. **What it does not establish:** no fixture computes the EGM's recall, false-positive rate,
kappa, cluster-bootstrap interval, reproducibility, or stopping rules; no fixture establishes
H1/H2. No experimental finding was generated and no synthetic kappa was computed.

## 5. Missing implementation or evidence

1. **No EGM threshold/stopping-rule evaluator.** The analysis is manual; it must be
   pre-registered in the run manifest and executed by the analyst.
2. **No kappa or cluster-bootstrap implementation.** Manual, with the same pre-registration
   requirement.
3. **No EGM run-manifest validator.** The §5.8 fields are recorded by hand.
4. **C parallel-batch latency is UNVERIFIED (EP-15).** H2's latency criterion may be
   INCONCLUSIVE until a provider run measures it.
5. **Provider-attested model identity (EP-02) and reproducibility (EP-12)** remain operator
   dispositions.
6. **No approved call/cost ceiling.** The proxy can enforce one, but the value is not set.
7. **Corpus far below the floor.** 3 strict plans / 4 defects vs 15 / 15; the partials are
   outside the conservative denominator.

## 6. Operational risks

| Risk | Effect | Mitigation |
| --- | --- | --- |
| Corpus below the floor | Every recall comparison is INCONCLUSIVE before any threshold applies. | Complete the corpus first; do not run. |
| No adjudicators (Decision 6) | Findings cannot be adjudicated; kappa and recall are unavailable. | Designate two independent humans first. |
| No seeded/control cases (Decision 5) | No false-positive control; a PASS could not support a quality claim. | Authorize construction after the checker and sealed manifest exist. |
| No threshold evaluator | Manual analysis could drift from the registered rules. | Pre-register the analysis method in the manifest; have the analyst follow it. |
| Unreported tokens | Cost criterion INCONCLUSIVE. | Choose a provider that reports tokens; record `unreported_calls`. |
| EP-15 UNVERIFIED | C latency may be INCONCLUSIVE. | Authorize the bounded EP-15 check or accept the limitation. |
| Arm D optional | Adds up to 609 calls and a model-diversity claim. | Treat D as exploratory; authorize separately. |
| `small` model class | Carries an unremovable cloud fallback candidate (Workstream B). | Do not use `--model-class small`. |
| Threshold change after data | Invalidates pre-registration. | Approve thresholds before the first run; no post-run change. |

## 7. Decision options and recommendation

| Option | Description | Trade-off |
| --- | --- | --- |
| **A. Approve thresholds now, defer spend** | Record the registered thresholds, the arm configuration, the measurement rules, and a hard call ceiling; authorize spending only in Decision 9. | Pre-registers the rules; the corpus/adjudicator blockers still prevent a run. |
| B. Defer Decision 7 | Decide with the actual corpus and adjudicators in hand. | Leaves the rules and ceiling unapproved; no run is authorized either way. |
| C. Revise thresholds | e.g. a smaller corpus or budget. | Weakens power or changes the registered rules; not recommended. |

**Recommendation: Option A.** Approve the registered thresholds and a hard call ceiling now,
and keep the spend authorization in Decision 9. This separates "the rules are fixed" from "the
run is authorized," which is what pre-registration requires. It does **not** make the experiment
ready: the corpus floor, Decisions 5–6, and EP-15 remain open.

Do **not** lower any threshold to fit the current corpus. Below the floor, the correct outcome
is INCONCLUSIVE.

## 8. Exact operator approvals still required

To record Decision 7, the operator must state:

1. the approved threshold values, verbatim as the registered rules in §1;
2. the hard **call ceiling** (and a dollar ceiling only if a provider price is adopted);
3. the **measurement rule for unreported usage** (cost INCONCLUSIVE; no output-size proxy);
4. the **arm configuration** — default 7-call or disjoint 8-call — and whether Arm D is
   authorized;
5. the **"no threshold change after the first run"** statement;
6. the **EP-15 disposition** (it is a Decision 9 input, but it affects the C latency criterion).

This record approves none of them. Decision 7 remains **PENDING**, and the corpus remains
**NOT FROZEN**.

## 9. Validation performed

- All registered values were re-read at `a3e238da`; the call-budget tables are reproduced
  exactly from §7 and the thresholds from §6.1–§6.2.
- The offline commands above were run in an isolated worktree; no live provider call occurred.
- The protocol was not modified; no threshold, arm, or cost ceiling was changed.
- No candidate, case, adjudicator, or manifest was created.
