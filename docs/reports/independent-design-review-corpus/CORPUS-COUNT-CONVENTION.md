# Corpus Count Convention — Independent Design Review (EGM)

**Type:** Presentation convention (non-normative). It fixes the arithmetic vocabulary for
reporting corpus counts. It is **not** an eligibility rule, a disposition, a promotion, a
freeze, or an amendment.
**Baseline:** `origin/main` @ `0df598fa` (after PRs #626 and #627).
**Authority it does not displace:** protocol
[§5.3](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) (the 15 / 15 floors),
[DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md) (the recorded
dispositions), [DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md) (the `conservative`
rule).
**Freeze status:** **NOT FROZEN.** Independently adjudicated: **0 / 0**.

## 1. Why this document exists

Two merged reports published different headline totals for the same corpus:

| Report | Headline | Inclusion rule it used |
| --- | --- | --- |
| [#626](HISTORICAL-CORPUS-STRICT-EVIDENCE-VERIFICATION.md) §9 | **12 plans / 14 defects** | baseline + *every* proposed increment, artifact counting, all rulings assumed favorable |
| [#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md) §9 | **4 plans / 5 defects** | baseline + only the increment that needs no open ruling |

Both are arithmetically correct. Both use the same baseline (3 / 4) and agree on every
intermediate total (6 / 7 after #613; 12 / 14 after #620). They differ **only in the
inclusion rule**, and each is silent about the other's scope. Read alone, 12 / 14 reads as
if conditional proposals were approved, and 4 / 5 hides the conditional upside. Neither
promotes a candidate; the adjudicated count is **0**.

## 2. The four published count classes

A report may publish C0, C1, C2 and C3. It must not publish **C4**. Every total names its
class.

### C0 — Official baseline

The dispositions recorded under Decision 2. None is independently adjudicated, and none is
counted toward either floor until the corpus owner records determinability (protocol §5.3).

| Class | Plans | Defects | Shortfall vs 15 / 15 | Gating decisions |
| --- | --- | --- | --- | --- |
| **C0 — official baseline** | **3** | **4** | **12 / 11** | recorded; adjudication still 0 |

Composition ([HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md) §5, §8.3):

| Lead | Artifact | Revision | Defects |
| --- | --- | --- | --- |
| `IDR-L-003` | `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` | `4fb3b8a3` | 1 |
| `IDR-L-004` | `docs/specs/REVIEW.md` | `5fce2b6a` | 2 |
| `IDR-L-007` | `docs/specs/PLAN-HISTORICALIZATION.md` | `fcf53888` | 1 |

### C1 — Independent proposal needing no open ruling

C0 plus increments whose evidence is a merged change, erratum, or recorded gap — so they do
not depend on **R-B** (bot-surfaced evidence), and whose artifact needs no **R-C** or
**R-C-packet** kind ruling, and which raise no counting-unit question.

| Class | Plans | Defects | Shortfall vs 15 / 15 | Gating decisions |
| --- | --- | --- | --- | --- |
| **C1 — conservative total** | **4** | **5** | **11 / 10** | none of R-B / R-C / R-C-packet; corpus owner must still record the lead |

The only increment in C1 is `GH3-001a` ([#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md) §4.2):
`docs/specs/PROMPT-EXECUTION.md` @ `1f9dfae8`, a spec with a genuine pre-fix revision, a
merged requirement change (`aa53c882`) and a recorded gap (`RESOLVED-BACKLOG.md`,
`d46bc167`).

### C2 — Conditional stack, artifact counting

C0 plus **every** proposed increment, each labelled with the decisions it is waiting on.
This is the maximum provisional total, not a reached total. *The gating rulings (R-B,
R-C-packet, R-C, counting unit, R-D) were recorded conservatively on 2026-11-21 in
[DECISION-003-ELIGIBILITY-RULINGS-APPROVAL.md](DECISION-003-ELIGIBILITY-RULINGS-APPROVAL.md);
no C0–C3 count changes, and C2 is no longer available under those rulings.*

| Layer | Source | Plans | Defects | Cumulative | Gating decisions |
| --- | --- | --- | --- | --- | --- |
| C0 | Decision 2 / `IDR-L` | 3 | 4 | 3 / 4 | recorded; adjudication 0 |
| #613 | `GH-001`–`GH-003` | +3 | +3 | 6 / 7 | R-B (all three); R-C-packet (`GH-003`) |
| #620 | `GH-009`–`GH-014` | +6 | +7 | 12 / 14 | R-C-packet (all six); R-B (five — `GH-013` has a merged correction) |
| #627 | `GH3-001a` | +1 | +1 | 13 / 15 | none of R-B / R-C / R-C-packet |
| **C2 — maximum provisional** | | **13** | **15** | **2 / 0** | every ruling above, all favorable, plus adjudication |

### C3 — Conditional stack, subject-cluster counting (sensitivity only)

The only cluster map that exists anywhere is #620's own, for its six leads:
`GH-009` + `GH-010` share the Gap 13 run_command-isolation cluster, and `GH-011` + `GH-012`
share the LLM03 D4 enforcement/approval cluster, so six artifacts become four clusters.
**No cluster map is defined across layers**, so C3 is published as a single sensitivity row
and never as a total.

| Class | Plans | Defects | Shortfall vs 15 / 15 |
| --- | --- | --- | --- |
| **C3 — cluster sensitivity** | **11** | **15** | **4 / 0** |

### C4 — Potential and held leads (published, never totalled)

`DISC-001` / `IDR-H-001`, `GH-004`, `GH-005`, `GH-006`, `GH2-001`–`005`, and `IDR-L-008`
(partial) sit outside every class. Their additivity is undefined: several share subjects with
stacked leads, and no counting unit has been ruled. #613's own upper-bound arithmetic (§6)
already shows they cannot close the plan floor. A report may list them; it must not add them
to C0–C3.

## 3. Reconciliation arithmetic, stated once

```
C0   3 / 4      shortfall 12 / 11     recorded dispositions, 0 adjudicated
+3   #613      GH-001..003            R-B ×3, R-C-packet ×1
=6/7
+6   #620      GH-009..014            R-C-packet ×6, R-B ×5
=12/14
+1   #627      GH3-001a               no open ruling
=13/15  C2     shortfall 2 / 0
C1 = C0 + GH3-001a = 4/5   shortfall 11 / 10
C3 = 11 / 15               shortfall 4 / 0
```

Two facts follow, and both matter more than the headline:

- **C2 meets the defect floor exactly (15) with zero slack.** One failed adjudication, one
  unfavorable R-C-packet ruling, or one dropped defect puts it below 15. "Exactly 15" is not
  a margin.
- **Plans are the binding constraint in every class.** C2 is still 2 plans short; C1 is 11
  short. No amount of defect counting closes the plan floor, because most leads are
  one-defect plans.

## 4. Additivity verification

Checked before any increment was added. Nothing below changes a count; it records why the
counts add.

1. **No path overlap.** The 13 stacked artifacts are 13 distinct paths; no path appears in
   two layers (baseline 3, #613 3, #620 6, #627 1). Verified by comparing the path sets.
2. **No duplicate defects.** Each stacked defect is a distinct defect in a distinct artifact.
   Two exclusions are deliberate: `GH-013`'s second finding (#486 thread) is `PARTIAL` and
   counted nowhere; `GH3-001b` is `PARTIAL` on a plan already counted, so it adds 0 plans and
   0 floor defects and appears only as that plan's per-plan count.
3. **One path, two records — no double count.** `docs/specs/REVIEW.md` and
   `docs/plans/PLAN-09-…` each have an **excluded** manifest entry (`IDR-E-005`, `IDR-H-008`)
   *and* a strict lead-level entry (`IDR-L-004`, `IDR-L-003`). Only the lead entry counts;
   the manifest entries are excluded and contribute 0. The baseline is not double-counted.
4. **Evidence artifacts are not counted artifacts.** `PONYTAIL-DEBT.md` appears in #627 §6 as
   *evidence* for `GH-010`; it is not a counted plan. `GH-001`'s artifact is the different
   path `PLAN-Ponytail-Complexity-Audit-Cleanup.md`.
5. **Counting unit.** Artifact counting is additive and is the convention's default. Cluster
   counting is not additive across layers and is published only as C3.
6. **R-A is settled for these leads.** #620 §3 established that each reviewed revision equals
   the merged revision, so no lead in C1–C3 is a PR-head draft fixed before merge. R-A no
   longer gates any increment.

## 5. Why 12 / 14 and 4 / 5 coexisted

- #626 answered *"what is the most we could have if every conditional proposal were admitted
  and counted by artifact?"* → 12 / 14 (C2 before #627). It added no lead.
- #627 answered *"what survives without asking the operator anything?"* → 4 / 5 (C1), because
  it verified one new lead and did not re-verify the #613/#620 layers.
- Neither report miscomputed. The defect was presentational: a corpus where the two numbers
  sit in the same folder, differ by 8 plans, and are both labelled "proposed".

## 6. Reporting rule for subsequent reports

1. State **C0 (3 / 4)** first, with "0 independently adjudicated".
2. State each increment with its layer/lead ID, its gating decisions, and its evidence type.
3. Publish **C1** and **C2** as explicitly labelled totals. Do not publish a bare
   "N plans / M defects".
4. Publish cluster counting only as **C3**, with the cluster map named. Never as a total.
5. Never describe C2 or C3 as reached, met, eligible, approved, frozen, or a floor decision.
6. Bind the word "shortfall" to a class: "C1 shortfall 11 / 10", not "shortfall".
7. Keep `candidates.json` the single manifest. Proposals and leads stay in reports; a lead
   enters the manifest only through the corpus owner's recorded decision.

## 7. What this convention does not do

It does not rule on R-B, R-C, or R-C-packet; does not change Decision 2, 3, 4, 5, or 7; does
not lower or reinterpret the 15 / 15 floors; does not promote, exclude, or adjudicate any
candidate; does not modify `candidates.json`, the registered protocol (SHA-256 `abba919e…`,
byte-identical), or any frozen or sealed artifact; does not construct a case, select a
manifest location, designate a checker or adjudicator, freeze the corpus, run inference, or
execute the experiment. Decision 6 remains **PENDING**; the corpus remains **NOT FROZEN**;
the experiment remains **NOT YET READY**; H1/H2 remain **NOT ESTABLISHED**.
