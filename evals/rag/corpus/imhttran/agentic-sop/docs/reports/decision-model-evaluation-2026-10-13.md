# Decision-Model Evaluation — 2026-10-13

**Type:** Evidence report (non-normative). It records a rerun and reconciliation at a
point in time; the behavior it concerns is owned by the documents it links.
**Status:** Rerun complete and independently recomputed. No production behavior, routing,
or policy changed, and no AI layer was enabled as a governance authority.
**Sources of truth:** the frozen artifacts under [evals/decision-model/](../../evals/decision-model/),
the rerun artifacts and write-up in
[evals/decision-model/rerun-2026-10-13/README.md](../../evals/decision-model/rerun-2026-10-13/README.md),
and the DME-FU backlog entries in [plans/BACKLOG.md](../plans/BACKLOG.md).

## 1. Purpose and scope

The question: can AI-assisted decision-making improve on SOP's deterministic
failure-disposition decisions — whether a failed task should be auto-fixed, retried,
continued, replanned, blocked, or escalated to a human? Three configurations were compared
on the frozen 61-scenario Decision-Model Effectiveness (DME) corpus:

- **Deterministic Go** — SOP's `internal/failure` classification with the decision layer
  disabled. This is the execution-governance authority.
- **Go + CLEF/Nimble advisory** — the production change-risk checkpoint behind the attention
  gate (CLEF primary, Nimble backup via the adapter); evidence can only *add* attention.
- **JEV shadow** — the real JEV analyzer run over each scenario's observable failure text
  only, then mapped to a disposition through `autonomy.DecideEarly`
  (`MapJEVHypotheticalDirective`). This is a **hypothetical classifier experiment**, not
  production early-JEV behavior, and not an evaluation of JEV's code-review ability.

Scope: this corpus, these configurations, these inputs. The conclusion is an evidence-based
statement about what was measured, not a general claim about AI.

## 2. Frozen-versus-rerun results

Every count below was recomputed independently from the raw records and matched the
recorded `metrics` blocks. Denominators are stated per metric; the deterministic baseline is
byte-identical across runs, so it has one column.

| Metric (denominator) | Deterministic Go | CLEF/Nimble advisory | JEV shadow (hypothetical) |
| --- | --- | --- | --- |
| Exact agreement (of 61 paired) | 47/61 = 77.0% (both) | 40/61 → 39/61 | 8/61 → 11/61 |
| Operational agreement ("needs human?") | 60/61 = 98.4% (both) | 13/32 → 12/32 (consulted) | 31/61 → 26/61 |
| Useful escalations (of 61) | — | 0 → 0 | 0 → 1 |
| False escalations (of 61) | 0 | 19 → 20 | 18 → 26 |
| Unsafe missed escalations (of 61) | 1 | 1 → 1 | 12 → 9 |
| Final actions changed (of 61) | — | 19 → 20 | improved/worsened/agree 0/39/15 → 1/37/14 |
| Median / p95 latency | 0 / 0 ms | 3207 / 9728 ms → 347 / 411 ms | 1495 / 2218 ms → 1527 / 2492 ms |

Denominators, stated explicitly:

- The advisory layer's **exact agreement is over all 61** paired scenarios (its final
  governed action versus the frozen label); its **operational agreement is over the 32
  consulted** scenarios only. The other 29 scenarios are **not eligible** for consultation
  (their deterministic baseline already withholds), so they are never scored for advisory
  agreement — they are not missing.
- JEV's agreement metrics are over all 61 records (each is `actionable`). Its raw
  `ERROR`/`INCOMPLETE` values are **classifier outcomes** (the model returned those statuses
  in parsed JSON), not provider execution errors: no record carries a transport error, and
  every record is `ran=true, available=true`.

## 3. Findings

The measures are reported separately because they are not interchangeable.

- **Exact agreement.** The deterministic baseline (47/61) is more accurate than both AI
  configurations, in the frozen run and the rerun.
- **Operational agreement ("needs human?").** The deterministic baseline (60/61) is more
  accurate than both: the advisory layer agreed operationally on 13 of 32 consulted (rerun
  12), JEV on 31 of 61 (rerun 26).
- **False escalations.** The baseline never escalates incorrectly (0). The advisory layer
  added 19 (rerun 20) and JEV added 18 (rerun 26). This is the AI arms' clearest regression.
- **Unsafe continuations.** The baseline and the advisory layer each have one unsafe miss,
  and it is the same scenario (`dme-real-close-004`); JEV has more (12 frozen, 9 rerun).
  Every JEV unsafe miss is a `CONTINUE` directive — a directive that proceeds automatically
  where the label wanted a human boundary — so those are genuine unsafe continuations. JEV
  cannot emit `BLOCK`, so it also cannot represent a terminal stop.
- **Withholding-label mismatch.** The baseline/advisory single unsafe miss is a
  **withholding-label mismatch**, not an unsafe continuation: `dme-real-close-004` expects
  `NEEDS_HUMAN` while the deterministic layer stops at `BLOCK`. Both withhold automatic
  execution and differ only in operational responsibility (a human-approval boundary versus
  a terminal stop). The label, not the classifier, is the outlier here; see DME-FU-002.
- **Useful escalations.** Zero for the advisory layer in both runs. JEV recorded one
  "improvement" (`dme-real-close-004`, rerun only) — the same terminal scenario, where JEV's
  answer differed across runs (frozen `CONTINUE` versus rerun `NEEDS_HUMAN`) and reached the
  label only through a hypothetical mapping it cannot justify against the terminal stop.
  Treated as a coincidence, not a demonstrated capability.

*Observation* (what the data shows): the rerun differs from the frozen run on the two AI
arms but not on the deterministic baseline (byte-identical). *Interpretation* (what it
means): the difference is not attributable to any single cause from the recorded evidence;
see §4.

## 4. Reproducibility limits

- **Missing model/settings metadata.** The frozen advisory artifact records only the
  provider names `clef`/`nimble`; it does not record the model name/version/digest, sampling
  settings, backend identity, provider mix, fallback reasons, harness revision, or prompt
  hash. Whether the frozen run used the same model versions as the rerun is unknown and is
  not asserted here.
- **Changed provider mix.** The frozen advisory run mixed CLEF 23 + Nimble 9 (the 9 Nimble
  records carry `fallback_reason: "primary unavailable"`); the rerun was served entirely by
  CLEF (32) with no fallbacks. The two runs are therefore not a like-for-like configuration.
- **Outputs differed across runs on identical inputs.** The same 32 scenarios were consulted
  in both runs (the attention gate is deterministic), yet the advisory choice matched on only
  23/32 and the final governed action on 25/32; JEV's raw status matched on 41/61 and its
  derived directive on 32/61. Because the model version, digest, and sampling settings were
  not recorded, **the missing metadata prevents separating sampling variance from
  configuration changes** — this report attributes the difference to neither.
- **Latency is not directly comparable.** The advisory added-latency median fell from 3207 ms
  to 347 ms (p95 9728 → 411 ms). A ~10× shift is consistent with a cold-versus-warm local
  model and/or a changed backend or model size, but the cause cannot be determined from the
  recorded evidence. JEV latency was stable (1495/2218 → 1527/2492 ms).
- **Only the deterministic baseline is bit-reproducible** across runs (identical sha256).

## 5. Decision

**Retain deterministic Go as the execution-governance authority**, and **retain the existing
DISABLE recommendation** for the CLEF/Nimble advisory layer and the JEV shadow mapping. This
is scoped to this corpus, these configurations, and these inputs: no tested configuration
showed a useful improvement, the clearest effect is more false escalations, and the AI arms
were not reproducible run-to-run. Nothing here promotes an AI layer, and the deterministic
baseline remains the reference. The bar for any future reconsideration is recorded in
[AI-DECISION-PROMOTION-CRITERIA.md](../reference/AI-DECISION-PROMOTION-CRITERIA.md).

## 6. Follow-ups

Tracked in [plans/BACKLOG.md](../plans/BACKLOG.md):

- **DME-FU-006 — Record evaluation metadata for model-backed runs** (P2). Precedes the
  repeated-run study: without the model name/version/digest, sampling settings, backend
  identity, provider mix, fallback reasons, harness revision, and prompt hash recorded (or
  explicitly marked `UNAVAILABLE`), runs cannot be compared.
- **DME-FU-007 — Repeated-run evaluation for model-backed configurations** (P2, blocked on
  DME-FU-006). Holds the configuration fixed and measures per-scenario consistency,
  escalation behavior, and latency distributions across multiple runs.
- **DME-FU-005 — Production-fidelity JEV evaluation** (P3). Replays the real early-JEV
  checkpoint. JEV's intended evidence/code-review role is evaluated separately under
  DME-FU-004, not through this failure-disposition classifier.
- **DME-FU-004 — Evaluate JEV's code-review effectiveness on real changes** (P2, conditional).
  JEV's code-review effectiveness **remains unmeasured**: this evaluation gave JEV only
  terse observable failure text, which is outside its design use.

Metadata capture (DME-FU-006) precedes the repeated-run study (DME-FU-007); JEV's review role
(DME-FU-004/005) is kept separate from decision disposition.

## 7. Evidence

- Corpus and freeze: [corpus.json](../../evals/decision-model/corpus.json),
  [CORPUS.sha256](../../evals/decision-model/CORPUS.sha256)
  (sha256 `9937cc4631de40c78430a160ae034045c07d14d687a2cee90d54b0ad13111e77`).
- Frozen results: [baseline-A.results.json](../../evals/decision-model/baseline-A.results.json),
  [paired-A-B.results.json](../../evals/decision-model/paired-A-B.results.json),
  [jev-shadow.results.json](../../evals/decision-model/jev-shadow.results.json).
- Rerun results and reconciliation:
  [evals/decision-model/rerun-2026-10-13/README.md](../../evals/decision-model/rerun-2026-10-13/README.md)
  (with the three rerun `.results.json` artifacts beside it).
- Findings and criteria: [DECISION-MODEL-FINDINGS.md](../reference/DECISION-MODEL-FINDINGS.md),
  [AI-DECISION-PROMOTION-CRITERIA.md](../reference/AI-DECISION-PROMOTION-CRITERIA.md).
- Plan: [PLAN-Decision-Model-Effectiveness-Evaluation.md](../plans/PLAN-Decision-Model-Effectiveness-Evaluation.md).
- Reproduce the freeze check:
  `go test ./internal/deceleval/ -run TestRepoCorpusLoadsValidatesAndIsFrozen`.
- Commits: `7a07dc9` preserved the rerun artifacts and findings; `b97a171` corrected the
  rerun conclusions and added DME-FU-006/007.
