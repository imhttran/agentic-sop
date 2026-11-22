# Decision 3 — Operative Eligibility Rulings (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved the five conservative eligibility rulings
prepared in [HISTORICAL-CORPUS-OPERATOR-HANDOFF.md](HISTORICAL-CORPUS-OPERATOR-HANDOFF.md) §3.
They are the operative reading of Decision 3 ([DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md))
and of protocol §5.3. This record does **not** approve any candidate, any determinability
finding, or any amendment.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — five eligibility rulings |
| Approver | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| Scope | Historical corpus eligibility rules |
| Rulings recorded | **5** (R-B, R-C-packet, R-C, counting unit, R-D) |
| Eligibility rules in force | `conservative` (Decision 3) + these five rulings |
| Candidate-level eligibility | **NOT DETERMINED** |
| Final candidate promotions | **0** |
| Independently adjudicated | **0** |
| Corpus status | **NOT FROZEN** |
| Experiment status | **DEFERRED** ([record](EXPERIMENT-DEFERRAL-APPROVAL.md)) |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `bdd067ee` (2026-11-21), the merge of PR #641. PRs #640 and #641
are merged; the registered protocol is byte-identical (SHA-256 `abba919e…`).

**Source of the decision:** the operator's explicit instruction in the session of 2026-11-21,
approving the five rulings and the Option B deferral documented in the merged operator handoff.

## 1. Recorded rulings

> **R-B — APPROVED (Option A).** A historical defect first surfaced by an automated reviewer is
> eligible only if a merged fix, erratum, reverted change or recorded gap independently documents
> it. A review finding alone is not such a record. The corpus owner records determinability at
> freeze; a model's verification is not independent human evidence. This applies Decision 3 and
> §5.3 as written and amends neither.

> **R-C-packet — APPROVED (Option A).** A historical decision or proposal packet is eligible only
> if its text, at a pre-fix revision, states a design (an action sequence, control, requirement or
> policy) that an implementer would follow. A packet that only compares options, assesses residual
> risk or requests approval is not eligible. The corpus owner rules per packet at freeze.

> **R-C — APPROVED (Option A).** Skills, guides, reference documents and operational procedures are
> not eligible as historical plans, specs or requirements, unless a document states implementable
> requirements and a merged fix, erratum or recorded gap documents the defect.

> **Counting unit — APPROVED (Option A).** The floor counts distinct artifact paths (path and
> sha256 at the pre-fix revision) that carry at least one determinable defect. A defect counts once
> however many documents restate it; later revisions of a path are the same plan. Reports show a
> subject-cluster count as a sensitivity row. The minimum of 15 plans and 15 defects is unchanged.

> **R-D — APPROVED (Option A, recorded as a rejection of the permissive reading).** A defect
> recorded only in an artifact's own later status line, with the gap still open, is **not** a
> "recorded gap" for eligibility. A self-record is not independent evidence.

The operator's approval names R-D "Option A — reject self-recorded, still-open limitations"; the
ruling above is that rejection, recorded in the operative direction.

## 2. Reconciliation with the registered protocol

Protocol §5.3 says the historical set is "plans, specs or requirements whose defect later appeared
in merged fixes, erratum, reverted change or recorded gap", each with an evidence pointer, and that
at freeze the corpus owner records whether each defect "was determinable from the document and its
parent revision, with a one-line basis".

| Ruling | Protocol basis | Effect |
| --- | --- | --- |
| R-B | The four evidence forms in §5.3 are the admitted record of a defect; a review thread is none of them | **Stricter** — excludes bot-only leads; no amendment |
| R-C-packet | "plans, specs or requirements" must state something an implementer would follow | **Stricter** — packet-level test; no amendment |
| R-C | Same clause; procedures are not plans, specs or requirements unless they state implementable requirements | **Stricter** — no amendment |
| Counting unit | §5.3 counts "plans" and records per-plan defects; a revision is not a new plan | **Stricter** — prevents revision inflation |
| R-D | A still-open self-report is not one of the four evidence forms | **Stricter** — no amendment |

**Conflict check:** none. Every ruling narrows eligibility; none changes a numeric minimum, arm,
threshold, cost ceiling, stopping rule, or the registered protocol text. The floors remain **15
plans / 15 determinable defects**, and §5.3's "target 12-15" is unchanged.

## 3. Approved rules versus candidate-level eligibility

The five rulings decide **which kinds of evidence and artifacts may count**. They do not decide
that any particular candidate counts.

- Candidate-level eligibility remains **NOT DETERMINED**. The corpus owner must still record, per
  candidate at freeze, the pre-fix revision, the evidence pointer, and a one-line determinability
  basis (protocol §5.3).
- No candidate is promoted into `candidates.json`; the file is unchanged.
- No determinability finding is approved; independent adjudication remains **0 / 0**.
- The Decision 2 dispositions are preserved as recorded.

Five candidates are proposed under these rules and remain proposals:
`IDR-L-003`, `IDR-L-004` (two defects), `IDR-L-007`, `GH3-001a`, `GH-013`.

## 4. Count impact (unchanged by this record)

| Class | Plans | Defects | Shortfall vs 15 / 15 |
| --- | ---: | ---: | --- |
| Approved eligible | **0** | **0** | **15 / 15** |
| C0 official baseline (Decision 2 dispositions) | 3 | 4 | 12 / 11 |
| C1 conservative proposals (C0 + `GH3-001a`) | 4 | 5 | 11 / 10 |
| **Proposed under the recorded rulings (S2)** | **5** | **6** | **10 / 9** |
| C3 cluster sensitivity | 11 | 15 | 4 / 0 |
| C2 conditional maximum | 13 | 15 | 2 / 0 |

S2 = C1 + `GH-013`: five distinct artifact paths and six defects (1 + 2 + 1 + 1 + 1). The C2
ceilings remain arithmetic only; the recorded rulings exclude the evidence they depend on, so C2
is **not available** under these rules. No count here is a reached, approved, or frozen count.

## 5. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** |
| 3 | Eligibility rule and these operative rulings | **APPROVED 2026-11-21** — this record |
| 4 | Proposed protocol amendment | **REJECTED (unnecessary) 2026-11-21** |
| 5 | Seeder / checker, manifest, construction | Seeder **APPROVED**; D5-A, D5-B1–B4, D5-C **PENDING** |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost ceiling | **APPROVED 2026-11-21** |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** — not authorized |

## 6. What this record does and does not do

Established:

- The five rulings above are the operative eligibility rules alongside Decision 3.
- They apply to the historical corpus only; the seeded and no-known-defect sets are unaffected.
- The registered protocol, its minimums, and Decision 7 are unchanged.

Not established or authorized:

- No candidate is promoted, approved, or entered in `candidates.json`.
- No determinability basis is recorded; no independent adjudication occurs (0).
- No historical minimum is lowered, and no synthetic or seeded case substitutes for historical
  evidence.
- No adjudicator, case checker, analyst, storage location, or key holder is designated.
- No case is constructed; no sealed manifest or answer key exists; `case_construction_authorized`
  remains **false**.
- The corpus is **NOT FROZEN**; the experiment is **NOT YET READY**; H1/H2 are **NOT ESTABLISHED**.
- No model inference or experiment execution occurs.

## 7. Mechanism

Recorded as a written operator decision in this file, the form established by
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md). It creates no SOP task approval and no
lifecycle transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and
is not used here. No local decision-memory entry is claimed.
