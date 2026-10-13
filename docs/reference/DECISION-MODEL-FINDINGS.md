# Decision-Model Findings — DME-003/004/005 and the JEV Shadow Evaluation

**Type:** Descriptive reference
**Status:** Complete (evidence-based). No production behavior was changed.
**Evidence:** `evals/decision-model/` (frozen corpus, labels, results, hashes);
tooling in `internal/deceleval`, `cmd/deceleval-{baseline,advisory,recovery,jev}`.
**Plan:** [PLAN-Decision-Model-Effectiveness-Evaluation.md](../plans/PLAN-Decision-Model-Effectiveness-Evaluation.md).

This document consolidates what was tested, what was learned, the architectural
decisions that follow, and what remains to investigate. It does not change any
production decision path and does not treat any model as a production default.

## 1. Executive summary

**Question.** Can AI-assisted decision-making improve deterministic
failure-disposition decisions in the SOP SDLC harness — deciding whether a failed
task should be auto-fixed, retried, continued, replanned, blocked, or escalated to
a human?

**Conclusion.** The tested AI configurations did **not** demonstrate improvements
over deterministic Go for failure disposition. Every tested configuration (the
CLEF/Nimble advisory layer under two question phrasings, and a JEV shadow mapping)
produced **zero useful escalations** and **more errors** than the deterministic
classifier on the frozen corpus:

- Deterministic Go: **47/61** exact (77.0%).
- Go + CLEF/Nimble change-risk advisory: **40/61** (65.6%).
- JEV shadow mapping: **8/61** (13.1%).

This is an **evidence-based conclusion about the evaluated configurations**, not a
general claim that AI is inferior. It measures these configurations, on this
61-scenario corpus, with these inputs. JEV in particular was evaluated outside its
primary code-review use case (see §6).

## 2. Experimental methodology

The evaluation is defined in
[PLAN-Decision-Model-Effectiveness-Evaluation.md](../plans/PLAN-Decision-Model-Effectiveness-Evaluation.md).
The frozen corpus and its labeling method are described in
[evals/decision-model/README.md](../../evals/decision-model/README.md).

- **Frozen 61-scenario corpus.** Real recorded failures plus clearly labeled
  synthetic scenarios, each with an independently authored `expected` action
  (`AUTO_FIX | CONTINUE | RETRY | REPLAN | NEEDS_HUMAN | BLOCK`) and a rationale.
  Labels were frozen by hash **before** any model ran; the corpus hash is
  `9937cc4631de40c78430a160ae034045c07d14d687a2cee90d54b0ad13111e77`.
- **Deterministic baseline (Baseline A).** SOP's `internal/failure` classification
  with the decision layer disabled. Bit-reproducible (`baseline-A.results.json`).
- **CLEF/Nimble advisory (change-risk).** The production SEAM-004 checkpoint
  (`change-risk` question, LOW/MEDIUM/HIGH/HUMAN) behind the attention gate: only a
  baseline that would proceed automatically is consulted, and evidence can only
  **add** attention.
- **Recovery-question experiment (DME-005).** An evaluation-only failure-recovery
  question (`AUTO_FIX`/`CONTINUE`/`HUMAN`) routed through the real advisory
  machinery, with only `HUMAN` mapped to the adverse (escalating) choice.
- **Split.** ~1/3 `tuning` (20), ~2/3 held-out `eval` (41), fixed before any run.
  The DME-005 question was selected on tuning and the prompt frozen
  (`RECOVERY-PROMPT.sha256`) **before** the held-out run.
- **JEV shadow-mode evaluation.** The real JEV analyzer
  (`internal/jev.OllamaAnalyzer`, via `run.RunJEV`) was run over all 61 scenarios,
  each given only its observable failure text, and its result mapped to a
  disposition through the existing deterministic early-JEV policy
  `autonomy.DecideEarly`. JEV was **shadow-only**: its recommendation was recorded
  and compared, never applied.

**Metrics.** *Exact agreement* = the produced disposition equals the frozen label.
*Operational agreement* = the coarser "needs human?" question
(`disposition == NEEDS_HUMAN`) matches the label. *Useful escalation* = an added
human boundary where the label wanted one and the baseline lacked it. *False
escalation* = an added human boundary the label did not want. *Unsafe miss* = the
label wanted a human boundary and none was produced. *Latency* = observed
per-scenario model/provider latency.

**Populations and inputs differ — do not mix denominators.**
- The full-61 comparison scores each configuration on all 61 scenarios.
- The DME-005 held-out comparison scores the same 41 held-out scenarios.
- The advisory configurations only consult scenarios the attention gate marks
  eligible (21/41 in the held-out set); their *exact/operational agreement* is
  therefore over the **consulted** subset, while their *final-action* correctness is
  over all 41.
- JEV received only the observable failure text — **not** a code diff or repository
  context — so its result measures decision classification on this corpus, not its
  designed review capability.

## 3. Verified results

All figures below were reconciled against the recorded artifacts
(`baseline-A.results.json`, `paired-A-B.results.json`,
`dme005-recovery-eval.results.json`, `jev-shadow.results.json`).

### Full 61-scenario comparison

| Configuration | Correct | Exact agreement |
| --- | ---: | ---: |
| Deterministic Go | 47/61 | 77.0% |
| Go + CLEF/Nimble (change-risk) | 40/61 | 65.6% |
| JEV shadow mapping | 8/61 | 13.1% |

### DME-005 held-out comparison (41 scenarios)

| Configuration | Correct | Exact agreement |
| --- | ---: | ---: |
| Deterministic Go | 30/41 | 73.2% |
| Change-risk advisory | 27/41 | 65.9% |
| Recovery advisory | 28/41 | 68.3% |

### Escalation and safety counts

| Measure | Change-risk (61) | Recovery (41) | JEV (61) |
| --- | ---: | ---: | ---: |
| Useful escalations | 0 | 0 | 0 |
| False escalations | 19 | 11 | 18 |
| Unsafe missed escalations | 1 | 1 | 12 |

- **DME-005 useful escalations:** 0 for both AI configurations.
- **DME-005 false escalations:** 12 change-risk, 11 recovery (on the 41 held-out).
- **JEV useful improvements:** 0. **JEV false escalations:** 18.
  **JEV unsafe missed escalations:** 12.
- **JEV latency:** median 1,495 ms / p95 2,218 ms (`glm-4.7-flash:latest`, Ollama).

### Operational agreement (coarser "needs human?")

| Configuration | Operational agreement |
| --- | ---: |
| Deterministic Go | 60/61 (full), 40/41 (held-out) |
| Change-risk advisory | 9/21 (held-out consulted) |
| Recovery advisory | 10/21 (held-out consulted) |
| JEV shadow mapping | 31/61 |

### JEV directional reconciliation

JEV reported **improved 0, worsened 39, agreed with deterministic 15**. Checked
against the per-scenario records, these do not have to sum to 61 because they are
overlapping classifications:

- **agreed (15)** — JEV's derived directive equals the deterministic action. Of
  these, **8** are scenarios where both were correct.
- **worsened (39)** — the deterministic baseline was correct and JEV's directive
  differed from the label (JEV turned a correct decision into an incorrect one).
- **improved (0)** — the deterministic baseline was wrong and JEV's directive
  matched the label. JEV fixed nothing.
- **remaining (7)** — the deterministic baseline was already wrong **and** JEV's
  directive matched neither the baseline nor the label (JEV was also wrong, but
  differently). These 7 are neither improvements nor regressions relative to the
  label; they are two different incorrect answers.

15 + 0 + 39 + 7 = 61. No case is forced or dropped.

JEV raw statuses: FINDINGS 52, PASS 5, ERROR 2, INCOMPLETE 2. Derived directives:
CONTINUE 35, NEEDS_HUMAN 26. The coarse result is that JEV reduces
`DecideEarly`'s severity threshold to a two-way choice (blocking finding →
`NEEDS_HUMAN`; otherwise `CONTINUE`), which both over-escalates benign cases and
under-escalates authority-boundary cases.

## 4. Architectural findings

**Deterministic Go**
- Retains execution and governance authority; it is the sole authority for
  execution, safety, and governance.
- Produces reproducible, testable decisions (`baseline-A.results.json` is
  bit-reproducible).
- Provides explicit, inspectable safety boundaries (authority-boundary kinds →
  human; no-progress/not-configured → terminal).

**CLEF/Nimble advisory**
- Remains **experimental** for failure disposition. Two question phrasings
  (change-risk and failure-recovery) both produced 0 useful escalations and more
  errors than deterministic Go.
- Must **not** be promoted based on current evidence.
- Can be reconsidered **only** after measurable improvements and no safety
  regressions (see §7 and the plan's decision rule).

**JEV**
- Remains useful as a **code-quality / structured-review** mechanism (its designed
  purpose: findings over real changes, feeding the quality gate's severity policy).
- Should **not** be promoted as a failure-recovery classifier based on the shadow
  experiment.
- Was evaluated **without code diffs or repository context**, which limits
  conclusions about its intended review capability (see §6).

**RAG**
- No current evidence establishes that retrieval would correct the observed
  decision errors. The observed errors are classification errors, not
  knowledge-retrieval gaps.
- Defer integration into failure disposition until a concrete, evidence-identified
  gap (for which retrieval is the appropriate remedy) exists.

## 5. Important edge cases

### `dme-real-close-004`

| Field | Value |
| --- | --- |
| Expected label | `NEEDS_HUMAN` |
| Deterministic result | `BLOCK` |
| JEV result | `CONTINUE` (FINDINGS at MEDIUM severity) |
| CLEF/Nimble consultation | **ineligible** — the baseline was terminal, so the attention gate never consulted it |

`BLOCK` and `NEEDS_HUMAN` both withhold automatic execution, but they represent
different operational responsibilities: `NEEDS_HUMAN` is a human approval boundary
(a person must decide), while `BLOCK` is a terminal automation/configuration stop
(no human approval exists; the operator must change conditions and re-run). The
deterministic result (`BLOCK`) withholds execution; the label asks for a human; JEV's
`CONTINUE` would let the scenario proceed — strictly less conservative than both.

### `DecideEarly` handling of `INCOMPLETE` / `ERROR`

In the shadow run, JEV's `INCOMPLETE`/`ERROR` statuses were mapped by
`autonomy.DecideEarly`, which classifies only by finding severity. A result with no
blocking finding — including `INCOMPLETE`/`ERROR` with no HIGH/CRITICAL items —
maps to `CONTINUE`. By contrast, the quality gate
(`quality.NewJEVEvidence` / `quality.Evaluate`) treats `INCOMPLETE`/`ERROR` as
**fail-closed** evidence that is never a pass. The two paths therefore disagree on
the safety semantics of an incomplete analysis.

This is recorded as an **investigation item**, not a confirmed production
vulnerability: whether `DecideEarly`'s behavior is reachable in the real execution
path (early-JEV checkpoints), and whether the discrepancy is a defect or an
intentional difference between the two consumers, must be verified against the
production wiring before any change. No production code was modified.

## 6. Limitations

- **Corpus size is 61 scenarios** — small; a fraction of a percentage point is not
  statistically robust.
- **Labels have not undergone independent human adjudication** — the second-label
  review was rule-based/single-author (~76.9% agreement on a 13-scenario sample),
  not inter-human agreement.
- **Some scenarios are synthetic** (clearly labeled), supplemented by real recorded
  failures.
- **Model results may vary between runs** — CLEF-4bit is schema-nondeterministic and
  advisory runs are not bit-reproducible; the deterministic baseline is.
- **JEV was evaluated outside its primary review use case** (no diffs/context).
- **These experiments measure decision classification**, not overall coding-agent
  success rates.

## 7. Final decision

**Keep deterministic governance. Do not promote the tested AI failure-disposition
configurations. Retain AI for appropriate evidence-generation and review tasks.**

- Deterministic Go remains the authority for execution, policy, and safety
  boundaries.
- The CLEF/Nimble advisory layer and the JEV shadow mapping are **not** promoted to
  influence production decisions.
- AI remains appropriate for evidence generation and code-quality/review tasks
  (where JEV already operates).
- Reconsideration of AI-assisted failure disposition requires: no unsafe missed
  escalations, false escalations within a pre-registered threshold, changes only
  where labeled, bounded latency, and at least one of decision accuracy or useful
  escalation rate showing a supported improvement — the decision rule in the
  [plan](../plans/PLAN-Decision-Model-Effectiveness-Evaluation.md#decision-rule-retain--tune--disable).
