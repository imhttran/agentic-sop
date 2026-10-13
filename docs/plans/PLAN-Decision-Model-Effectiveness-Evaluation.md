# PLAN — Decision Model Effectiveness Evaluation

**Status:** EVALUATION COMPLETE — the corpus, deterministic baseline, advisory
comparison, recovery-question experiment, and JEV shadow-mode evaluation have run.
**Conclusion:** keep deterministic governance; do not promote the tested AI
failure-disposition configurations. Consolidated findings:
[DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md). DME-006–DME-010
(statistical reporting, safety-regression checks, performance benchmarking, results
report, decision rule) remain open as follow-ups.

**Scope:** Decide whether the local advisory decision system (`CLEF-4bit` via oMLX with a
local Bespoke-Nimble-9B backup, behind SEAM-004) measurably improves on SOP's
deterministic-only failure classification — measured on decision quality, safety,
latency, and operational cost, not on "the integration works".

## Project

agentic-sop (with `sop-decision-adapters` as the adapter under evaluation).

## Baselines

- **A — Deterministic only.** SOP's `internal/failure` classification
  (`Kind`/`Disposition`/`Confidence`) with the decision layer disabled.
- **B — Advisory.** A plus the SEAM-004 decision checkpoint (CLEF primary,
  Nimble backup, deterministic fallback), which may only **add attention**.

Both run against the **same** scenarios. The advisory recommendation and the final
governed SOP action are recorded and reported **separately**; the models and the
deterministic classifier are **never treated as ground truth**.

## Results (completed)

The evaluation ran against the frozen 61-scenario corpus (hash `9937cc46…`); the
JEV shadow-mode evaluation ran over the same corpus. Full detail, populations, and
reconciliation: [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md).

| Configuration | Correct | Exact agreement |
| --- | ---: | ---: |
| Deterministic Go | 47/61 | 77.0% |
| Go + CLEF/Nimble (change-risk) | 40/61 | 65.6% |
| JEV shadow mapping | 8/61 | 13.1% |

- **Useful escalations: 0** for both advisory configurations and for JEV.
- The DME-005 **recovery-question** experiment was marginally better than the
  change-risk question but still trailed deterministic Go (held-out 41: 30
  deterministic vs 27 change-risk vs 28 recovery), with 0 useful escalations.
- The JEV **shadow** mapping never produced `BLOCK` and regressed 39
  deterministic-correct decisions (0 improvements).

**Outcome:** keep deterministic governance; retain AI for evidence-generation and
review tasks; do not promote the tested AI failure-disposition configurations.
Outstanding follow-ups are tracked in [docs/BACKLOG.md](../BACKLOG.md) (see also §6
of the consolidated findings).

## 1. Existing telemetry coverage and limitations (Phase 1, inspected)

Recorded today (read-only inspection of `.agent-sdlc/runs/` and `internal/`):

- **Deterministic classification** — `classification.json` (`kind`, `disposition`,
  `confidence`, `reason`) and the `report.json` payload (`internal/failure`).
  Corpus: 219 run dirs; `classification.json` on **38**; `report.json` on 156.
- **Advisory evidence** — `decision-evidence.json` (`run_id`, `version`, `contract`,
  `consulted`, `outcome_class`, `choice`, `confidence`, `failure_label`,
  `baseline_action`, `final_action`, `escalated`), written only when the decision
  layer is enabled (`internal/cli/decision_evidence.go`).
- **Decision telemetry** — `sop report --decisions` over `internal/runmetrics`
  (model tier, override, escalation, JEV invocation, adapter agreement/…).
- **Failure-classification evaluation** — an existing deterministic harness
  (`internal/eval`, fixtures under `evals/<category>/*.expect.json`) asserting
  lifecycle `termination`, `progress`, `budgets`, `replans`, `context`,
  `orchestration` — **not** advisory decision quality.

Limitations / gaps (unavailable ≠ zero):

- **No historical advisory data.** `decision-evidence.json` is present on **0** of the
  219 corpus runs (the layer was off). Any A/B measurement must be **generated**, not
  read from history.
- **No serving-provider attribution.** `decision-evidence.json` records the
  consultation outcome but **not** which provider served (CLEF, Nimble, or
  deterministic fallback) nor a fallback reason. The SEAM-004 bridge is strictly
  provider-neutral, so this is currently **UNAVAILABLE** (see §4).
- **No decision-call latency.** The connector observes a latency event
  (`commandprofile` `Event{LatencyMS,…}`), but it is not persisted to a run artifact
  the evaluator can read → **UNAVAILABLE** without a small extension.
- **`internal/runmetrics` does not read `decision-evidence.json`** → the decision
  checkpoint is not part of `sop report --decisions`.
- **No labeled expected-action corpus.** `evals/` labels lifecycle outcomes, not
  "escalation was appropriate".

## 2. Evaluation corpus (Phase 2)

**Size:** 60–80 scenarios (within the requested 50–100). **Split:** ~70 % evaluation
set, ~30 % development/tuning set, fixed before any run and never re-split. Fixtures
are immutable once frozen (hashed).

**Sources:** prefer real artifacts from the corpus (the 38 `classification.json`
runs) as seeds; supplement with clearly labeled **synthetic** scenarios. Every
scenario is a small deterministic fixture (a disposable Go module + config + a
controlled command agent), modeled on the approved ETOE-005 / decision-eval
harness already used in validation.

**Categories (with target counts):**

| Category | Examples | n |
| --- | --- | --- |
| Benign impl/verification failure | typo-level incomplete change | 10 |
| Test/regression failure | failing unit test, stale test | 10 |
| Build/dependency failure | compiler error, missing dep | 8 |
| Missing/incomplete validation | validation not configured | 6 |
| Security-sensitive change | auth/token/encryption diff | 8 |
| Governance/approval boundary | destructive op, approval required | 8 |
| Ambiguous (human judgment) | contradictory acceptance | 8 |
| Provider fault injection | timeout, malformed, unavailable | 8 |

**Labeling method (independent):** each scenario gets an **expected action**
(`AUTO_FIX | CONTINUE | RETRY | REPLAN | NEEDS_HUMAN | BLOCK`) with a written
rationale derived from the scenario's *observable* properties (what actually failed,
what a human must decide), authored **without** reference to CLEF/Nimble/deterministic
outputs. Labeling is done before the model is run; labels are frozen and hashed.
Inter-labeler agreement (two independent labelers on a 20 % sample) is reported.

## 3. Measurement methodology (Phase 3)

Per scenario, record: (1) baseline deterministic action; (2) model recommendation;
(3) final SOP action; (4) expected action; (5) did the model change the final action;
(6) escalation justified; (7) false/unnecessary escalation; (8) unsafe missed
escalation; (9) provider used; (10) confidence + elapsed latency. Advisory
recommendation and final governed action are stored in separate columns.

Metrics (counts and denominators always reported with percentages):

- decision accuracy vs labels (Baseline A and Experimental B);
- useful escalation rate = justified escalations / escalations;
- false escalation rate;
- unsafe missed-escalation rate (**safety-critical**);
- % of final actions changed by B;
- provider availability + failover rate (CLEF vs Nimble vs deterministic fallback);
- added latency: median and p95 (B − A), per provider;
- evidence coverage and missing-data rate.

**Uncertainty:** Wilson score intervals for rates; a paired test (McNemar on changed
final actions) for the A/B delta; bootstrap intervals for latency. **Confidence is
reported as a model-reported number, never as a calibrated probability** (no
calibration evidence exists).

## 4. Instrumentation gaps and proposed extensions (Phase 4)

Smallest **backward-compatible, diagnostic-only** changes (to be implemented only
after approval; they do not alter governance or decision behavior):

1. **Serving-provider + fallback attribution.** Add an optional diagnostics
   passthrough: adapter output gains an optional `diagnostics` map
   (`{"served_provider","fallback"}`), `bridge.Result` gains an optional
   `Diagnostics` field that is **ignored by policy** (the connector already maps
   `canonical.Diagnostics → decision.Decision.Metadata`), and
   `decision-evidence.json` gains optional `served_provider` and `fallback`
   fields. Older outputs → **UNAVAILABLE**, never inferred.
2. **Decision-call latency.** Persist the connector's observed `LatencyMS` into
   `decision-evidence.json` as an optional field.
3. **Offline reader.** Extend `internal/runmetrics` (the existing aggregator) with
   decision-evidence dimensions (baseline vs final action, escalated, outcome class,
   provider, latency) so evaluation uses existing tooling rather than a new framework.

All three are additive and default to UNAVAILABLE for existing artifacts.

## 5. Tasks

### DME-001 — Corpus construction and labeling
- **Deliverables:** scenario fixtures + frozen, hashed labels + the split file.
- **Acceptance:** 60–80 scenarios across the categories; every label has a rationale;
  a 20 % sample double-labeled with agreement reported.
- **Deps:** none.

### DME-002 — Instrumentation extension (diagnostic-only)
- **Deliverables:** the §4 extensions + targeted tests (present/absent provider,
  latency, older artifact → UNAVAILABLE).
- **Acceptance:** governance/decision behavior unchanged; arch/neutrality tests green.
- **Deps:** none. *(Blocked on approval of §4.)*

### DME-003 — Deterministic baseline (A) execution
- **Deliverables:** a reproducible runner that executes every scenario with the
  decision layer **disabled**, capturing the deterministic action + evidence hashes.
- **Acceptance:** identical inputs → identical outputs; no model calls.
- **Deps:** DME-001.

### DME-004 — Advisory (B) execution
- **Deliverables:** the same runner with the decision layer enabled (CLEF primary,
  Nimble backup), recording recommendation, final action, provider, latency.
- **Acceptance:** CLEF→Nimble→fallback exercised; fault-injection scenarios use test
  servers / dependency injection (no changes to the real oMLX/Nimble services).
- **Deps:** DME-001, DME-002, DME-003.

### DME-005 — Measurement collection and validation
- **Deliverables:** a pure, deterministic aggregator producing the §3 table.
- **Acceptance:** counts + denominators + coverage; missing data reported
  UNAVAILABLE, never zero.
- **Deps:** DME-003, DME-004.

### DME-006 — Statistical reporting and uncertainty
- **Deliverables:** Wilson intervals, McNemar, bootstrap; a reporting template.
- **Deps:** DME-005.

### DME-007 — Safety regression checks
- **Deliverables:** assertions that no scenario's final action is **less** attentive
  than its baseline action without a justified label; unsafe-missed-escalation count
  = 0 for the `SecurityBoundary`/`DestructiveOperation`/`ApprovalRequired` categories.
- **Acceptance:** a single unsafe miss is a **STOP** condition.
- **Deps:** DME-005.

### DME-008 — Performance benchmarking
- **Deliverables:** median/p95 added latency per provider and overall; failover rate.
- **Deps:** DME-004.

### DME-009 — Results report
- **Deliverables:** `docs/reports/decision-model-effectiveness/…` with the tables,
  intervals, coverage, and an explicit statement that success ≠ agreement with the
  deterministic classifier.
- **Deps:** DME-006, DME-007, DME-008.

### DME-010 — Decision rule and recommendation
- **Deliverables:** a **retain / tune / disable** recommendation against the rule
  below, with rollback steps.
- **Deps:** DME-009.

## Decision rule (retain / tune / disable)

Retain the advisory layer only if **all** hold: (a) **no unsafe missed escalations**;
(b) it does **not** increase false escalations beyond a pre-registered threshold on
the evaluation set; (c) it changes final actions only where a label justifies it;
(d) added p95 latency is within a pre-registered budget; (e) ≥ one of useful-
escalation-rate or decision-accuracy shows a statistically supported improvement over
Baseline A. Otherwise **tune** (labels show systematic model error) or **disable**
(no measurable improvement, or any safety regression). Agreement with the
deterministic classifier alone is **not** evidence of value.

## Rollback and reproducibility

- Evaluation runs happen in **disposable** projects; no real project state or
  existing evidence is overwritten.
- Every scenario, label, config, and fixture hash is frozen; runs record the
  provider/model and artifact hashes.
- Disabling the layer (`decision.enabled: false`) restores Baseline A exactly; the
  §4 extensions are additive and inert when absent.

## Estimated effort and risks

- **Effort:** DME-001 moderate (labeling is the bulk); DME-002 small; DME-003/004
  moderate; DME-005–010 small–moderate.
- **Risks:** label subjectivity (mitigated by rationale + double-labeling);
  CLEF-4bit's schema flakiness (already handled by fail-closed normalization;
  counted as fallback, not success); latency variance from local inference
  (bootstrap + repeated runs); the provider-neutral bridge limiting attribution
  (mitigated by §4).

## Decisions requiring approval

1. Approve the **diagnostic-only instrumentation** extension (§4) — the only change
   touching the contract, and it is additive/ignored by policy.
2. Approve **running** the B/experimental set against local CLEF/Nimble (local
   inference; no cloud).
3. Confirm the **pre-registered thresholds** for false-escalation rate and p95 latency.
4. Confirm whether provider identity should be recorded in SOP evidence at all
   (provider-neutrality trade-off).
5. Confirm the **labeling authority** (who authors/freezes the expected actions).
