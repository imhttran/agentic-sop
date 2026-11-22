# Historical Corpus — Operator Handoff

**Type:** Concise operator handoff (non-normative). It states where the historical-corpus workstream stands, which decisions are yours, and what each one does to the count.
It records **no decision** and promotes nothing. **Everything marked "recommended" is PROPOSED — PENDING OPERATOR APPROVAL.**
**Evidence record (authoritative, not duplicated here):** [HISTORICAL-CORPUS-FINAL-RECONCILIATION.md](HISTORICAL-CORPUS-FINAL-RECONCILIATION.md) (candidate matrix, revision register, scenarios).
**Also:** [eligibility decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md) (evidence for each option), [CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md), [gap closure](HISTORICAL-CORPUS-EVIDENCE-GAP-CLOSURE.md).
**Baseline:** `origin/main` @ `64248ffb8628ec4611fa12a7b36cb51c80cb0aeb`, 2026-11-22. **Protocol:** [§5.3–5.4](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md), SHA-256 `abba919e…` (unchanged).

## 1. Verified current status

| Check | Result (verified on `main` this pass) |
| --- | --- |
| Discovery and decision PRs | #613, #620, #626–#630, #632, #634–#636, #639 all **merged**; **no EGM PR is open** |
| Evidence investigations | complete: every lead in the inventory is verified, held, partial, excluded or blocked (reconciliation §4); the 23 revision pins were mechanically re-checked in #639 |
| New strict candidate with credible independent evidence | **none outstanding**; the last three passes (#634, #636, #639) found none |
| Duplicates and contamination | documented (reconciliation §5); no EGM or corpus file in any C2 pin tree |
| Partial / hindsight-only findings | remain **outside both floors** (Decision 3); 11 partial plans, `IDR-L-008` included |
| Promotion | **none**: `candidates.json` is unchanged since 2026-11-21 (23 entries; `include-with-review` 10, `hold` 5, `exclude` 8; `NOT_FROZEN`) |
| Independent adjudication | **0 / 0**; no adjudication record exists; no `DECISION-RECORD-006` or `-008` exists |
| Eligibility rulings (R-B, R-C-packet, R-C, unit, R-D) | **none recorded**; decision records on `main` are 001–005 and 007 |

**Workstream state: COMPLETE (evidence) — governance pending.** Nothing further can be established from evidence alone; the rest is your decisions.

## 2. Counts and the 15 / 15 shortfall

Recomputed from the records on `main`; the expected reference values were **confirmed, nothing changed**. "Shortfall" is against the registered 15 plans / 15 determinable defects.

| Classification | Plans | Defects | Shortfall | Assumptions |
| --- | ---: | ---: | --- | --- |
| **Approved eligible** | **0** | **0** | **15 / 15** | no eligibility is approved; no determinability is recorded; 0 adjudicated |
| C0 official baseline | 3 | 4 | 12 / 11 | Decision 2 **dispositions** ("include with review — proposed strict": `IDR-L-003`, `IDR-L-004`, `IDR-L-007`); **not** eligibility |
| C1 conservative proposals | 4 | 5 | 11 / 10 | C0 + `GH3-001a` (merged spec change plus recorded gap); needs no R-B / R-C / R-C-packet ruling |
| **S2 recommended scenario** | **5** | **6** | **10 / 9** | C1 + `GH-013`; R-B A, R-C-packet A, unit A, R-C A, R-D no |
| C3 cluster sensitivity | 11 | 15 | 4 / 0 | C2 counted by cluster (a sensitivity row, never a total) |
| C2 conditional maximum | 13 | 15 | 2 / 0 | every ruling permissive (R-B B, all packets admitted, artifact counting) and every adjudication succeeding; **not a corpus** |

No conditional scenario satisfies the protocol. Two further conditional routes reach 15 plans only with held leads and rulings the packages recommend against: **S6** (R-C Option C: 17 / 20) and **S7** (R-D yes plus `GH-004` admitted: 15 / 20, no plan slack); neither is a recommended outcome.
If `GH-010` is admitted (owner accepts its recorded gap and the packet) S2 becomes 6 / 7.

## 3. Decisions that need you

Each decision below is independent of the others except where noted. Evidence for every option is in the decision package §3–§6 and the gap-closure report §7.

| # | Question | Options | Recommended | Count impact | Remaining uncertainty |
| --- | --- | --- | --- | --- | --- |
| **R-B** | May a defect first surfaced by an automated reviewer count when no merged fix, erratum, reverted change or recorded gap documents it? | A: no. B: yes if a non-discoverer re-runs the check | **A** | A: caps the set at 4–5 plans. B: unlocks up to C2 | B needs a recorded reading of "recorded gap" or an amendment (Decision 4 rejected one); a bot-built denominator is circular |
| **R-C-packet** | Is a decision or proposal packet a plan or spec? | A: yes if it states a design at a pre-fix revision, no if it only compares options, assesses risk or requests approval. B: only `PLAN-*` / `docs/specs`. C: all `docs/plans/*` | **A** | +`GH-013` (S2); `GH-010`, `GH-012`, `GH-004` likely out | the owner rules packet by packet |
| **R-C** | Do skills, guides, reference docs and procedures count? | A: no. B: only with a T1 pointer. C: yes | **A** | A and B: 0. C: +4 plans (S6) | those artifacts state steps, not requirements |
| **Counting unit** | What does "15 plans" count? | A: distinct artifact path plus a same-defect rule. B: design subject. C: revision | **A** (cluster as a sensitivity row; versions of one artifact are one plan) | A: C2 13. B: C3 11 | no cross-layer cluster map exists |
| **R-D** | May a defect recorded only in an artifact's own later, still-open status line count as a "recorded gap"? | A: no. B: yes | **A** (a self-report is not independent evidence) | B: +1 plan (`IDR-L-001`), only in S7 | `IDR-L-001` is determinable but its evidence is self-recorded |
| **Corpus-owner determinability** (the step from proposed to approved) | For each candidate the owner admits: record, at freeze, a one-line basis that the defect was determinable from the document and its parent revision | per candidate | after R-B / R-C-packet / unit: **record for the S2 set** | each recorded candidate moves from "proposed" to "approved eligible"; 0 → at most 5 plans / 6 defects under S2 | the owner is the sole human author and reviewer of record; no independent check |

The individual determinability decisions needed under S2 are five: `IDR-L-003`, `IDR-L-004`, `IDR-L-007`, `GH3-001a`, `GH-013` (plus the owner's packet ruling on `GH-013`). Entering them in `candidates.json` is a governance step outside this handoff.

### Copy-ready wording (record only what you choose)

> **R-B — APPROVED (Option A).** A historical defect first surfaced by an automated reviewer is eligible only if a merged fix, erratum, reverted change or recorded gap independently documents it. A review finding alone is not such a record. The corpus owner records determinability at freeze; a model's verification is not independent human evidence. This applies Decision 3 and §5.3 as written and amends neither.

> **R-C-packet — APPROVED (Option A).** A historical decision or proposal packet is eligible only if its text, at a pre-fix revision, states a design (an action sequence, control, requirement or policy) that an implementer would follow. A packet that only compares options, assesses residual risk or requests approval is not eligible. The corpus owner rules per packet at freeze.

> **R-C — APPROVED (Option A).** Skills, guides, reference documents and operational procedures are not eligible as historical plans, specs or requirements, unless a document states implementable requirements and a merged fix, erratum or recorded gap documents the defect.

> **Counting unit — APPROVED (Option A).** The floor counts distinct artifact paths (path and sha256 at the pre-fix revision) that carry at least one determinable defect. A defect counts once however many documents restate it; later revisions of a path are the same plan. Reports show a subject-cluster count as a sensitivity row. The minimum of 15 plans and 15 defects is unchanged.

> **R-D — REJECTED.** A defect recorded only in an artifact's own later status line, with the gap still open, is not a "recorded gap" for eligibility.

> **Determinability — RECORDED for `<candidate id>`.** At the pre-fix revision `<full SHA>` the defect `<one-line description>` was determinable from the document and its parent revision because `<one-line basis>`. Evidence pointer: `<commit, PR or file>`.

## 4. Stopping condition for the historical set

**Can this repository meet 15 / 15 under the recommended conservative rules? No.** S2 is 5 plans / 6 defects: **10 plans and 9 defects short.** Only S6 or S7 reach 15 plans, and each needs several permissive rulings the packages recommend against plus every adjudication to succeed.

- **Why broad discovery will not close it:** four passes over review threads, commit history, spec amendments and held leads (#610, #613/#620, #626/#627, #634/#636) returned no further plan-level candidate. The remaining unreviewed corrective-keyword commit subjects (102 of 212) showed no sign of leads in the 110 reviewed. The recorded evidence in this repository is one author, with bot reviewers and no independent human reviewer.
- **Could an independent source supply the plans?** Possibly. The protocol text does not say plans must come from this repository. The operator's sibling repositories (`inventory`, `medical-billing`, `portfolios`, `scheduling`) are the only candidates named so far. I would **not** open them speculatively: ten strict plans with independent evidence is a large ask; they are likely also single-author with bot review; and `medical-billing` may contain sensitive data.
- **Authorization needed to expand the source:** (1) name the repositories or materials in scope; (2) rule on data handling (exclude anything containing protected or personal data by default); (3) confirm the same §5.3 evidence forms and the rulings above apply; (4) accept that each new lead needs its own pin, determinability and contamination check before it counts. Nothing is searched until you grant this.
- **Stop condition:** stop historical discovery in this repository now. Reopen only on (a) an operator-authorized new source, (b) a new independent human review record, or (c) a ruling that makes already-listed leads count and needs their line-level human verification.
- **Do not** lower the floor, substitute seeded cases for historical ones, or choose a permissive option only to reach 15.

### INCONCLUSIVE disposition (copy-ready, if you choose it)

The protocol already says what follows a missed floor (§5.3): "If replacements cannot reach the 15-determinable-defect floor, the run is deferred; the floor is not lowered", and "Below the floor, every recall comparison is INCONCLUSIVE."

> **Historical corpus — RECORDED as below the floor.** On the evidence in this repository, the historical set does not reach 15 plans and 15 determinable defects under the conservative rules (proposed 5 / 6; approved 0 / 0). Under protocol §5.3 the run is deferred and the floor is not lowered. Any recall comparison would be INCONCLUSIVE. No seeded case replaces a historical case. This is revisited only if an independent historical evidence source is authorized.

## 5. Recommendation

**Record the five rulings in §3 (they define the count and cost nothing), then record the INCONCLUSIVE / deferred disposition above unless you can name a specific independent source with human-reviewed history.** If you can, authorize that bounded source instead (§4). Do not authorize sibling repositories without a named scope and a data-handling rule.

## 6. Remaining dependencies (other decisions)

| Decision | State | What it needs |
| --- | --- | --- |
| **6** — two independent human adjudicators | **pending**; 0 of 2 assessed or designated; 7 research leads, none contacted (PR #625) | your outreach authorization, screening, conflict declarations, then designation; without two humans the best attainable outcome is INCONCLUSIVE (protocol §5.4) |
| **8** — corpus and instruction freeze | **NOT FROZEN**, not ready | decisions 1–7 recorded; a corpus that meets the floors (not met); seeded and no-known-defect sets built; instruction blocks final; the strict leads entered in `candidates.json` through governance; EP-15 verified |
| **9** — full experiment execution | **not authorized** | decisions 1–8, the budget ceiling (Decision 7, 613 calls), and Workstream B items |
| Decision 5 (case checker, manifest location, key holder) | governance pending ([closeout](DECISION-005-FINAL-CLOSEOUT.md)) | D5-A, D5-B1–B4, D5-C |

## 7. SOP lifecycle status

The owning record is [PLAN-Independent-Design-Review-Feasibility.md](../../plans/PLAN-Independent-Design-Review-Feasibility.md), a draft coordination record whose task states are coordination statuses, not SOP lifecycle transitions. This change adds a closure note there for the historical evidence workstream:
**COMPLETE (evidence) — governance pending.** No lifecycle step was run or recorded, and none was fabricated; the plan stays in `docs/plans/` because reports reference it.

## 8. Non-authorization statement

This handoff records no decision and approves no eligibility. It does not promote any candidate; change the registered protocol, Decision 3 or any threshold (including the 613-call ceiling); modify `candidates.json` or Decision 5 work; construct a seeded or control case; create a sealed manifest or answer key; designate an adjudicator or case checker;
change `case_construction_authorized`; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
