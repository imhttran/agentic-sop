# Decision Record 008 — Historical Eligibility Rulings and Corpus Deferral (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved the five conservative eligibility
rulings (R-B, R-C-packet, R-C, counting unit, R-D — each Option A) and **Option B — Defer** for
the historical corpus. The decision is recorded here from the operator's explicit instruction in
the session of 2026-11-21, in the repository's written decision-record form established by
Decisions 1–7. The proposal text in
[HISTORICAL-CORPUS-OPERATOR-HANDOFF.md](HISTORICAL-CORPUS-OPERATOR-HANDOFF.md) and the
[eligibility decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md) is preserved
as written and is not rewritten; this record is the decision.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — five eligibility rulings and deferral |
| Approver | **Tom Tran** |
| Corpus owner | **Tom Tran** |
| R-B (bot-surfaced evidence) | **Option A APPROVED** — independent historical evidence required |
| R-C-packet (decision packets) | **Option A APPROVED** — implementable design at a pre-fix revision |
| R-C (skills, guides, procedures) | **Option A APPROVED** — excluded unless the stated criteria are met |
| Counting unit | **Option A APPROVED** — distinct artifact paths, duplicate-defect controls |
| R-D (self-recorded limitations) | **Option A APPROVED** — self-record is not independent evidence |
| Evidence source | **Option B — DEFER**, no bounded new source authorized |
| Historical corpus | **BELOW THE REGISTERED FLOOR** — proposed 5 / 6; approved 0 / 0 |
| Registered minimum | **15 plans / 15 determinable defects — unchanged** |
| Approved eligible | **0 plans / 0 defects** |
| Independently adjudicated | **0** |
| `candidates.json` | **unchanged** — 23 entries, `NOT_FROZEN`, no `IDR-L` entry |
| Experiment disposition | **DEFERRED** under protocol §5.3 |
| Registered protocol SHA-256 | `abba919ecb0938475c1731b419c4bddd8b1d1fe92ed97d51109c85f14e32305c` — unchanged |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** — deferred, not authorized |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `bdd067ee800cd2085cc07fd5eb45fdd2fa0abe58` (2026-11-21), the
merge of PR #641, which was the baseline against which this record was prepared.

**Source of the decision:** the operator's explicit instruction in the session of 2026-11-21,
prepared from [HISTORICAL-CORPUS-OPERATOR-HANDOFF.md](HISTORICAL-CORPUS-OPERATOR-HANDOFF.md) §3
and §4, the [eligibility decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md)
§3–§6, and [HISTORICAL-CORPUS-FINAL-RECONCILIATION.md](HISTORICAL-CORPUS-FINAL-RECONCILIATION.md)
§6–§7.

## 1. Recorded decision

> Decision 008 — APPROVED
>
> **R-B — APPROVED (Option A).** A historical defect first surfaced by an automated reviewer is
> eligible only if a merged fix, erratum, reverted change or recorded gap independently documents
> it. A review finding alone is not such a record. The corpus owner records determinability at
> freeze; a model's verification is not independent human evidence. This applies Decision 3 and
> §5.3 as written and amends neither.
>
> **R-C-packet — APPROVED (Option A).** A historical decision or proposal packet is eligible only
> if its text, at a pre-fix revision, states a design (an action sequence, control, requirement or
> policy) that an implementer would follow. A packet that only compares options, assesses residual
> risk or requests approval is not eligible. The corpus owner rules per packet at freeze.
>
> **R-C — APPROVED (Option A).** Skills, guides, reference documents and operational procedures
> are not eligible as historical plans, specs or requirements, unless a document states
> implementable requirements and a merged fix, erratum or recorded gap documents the defect.
>
> **Counting unit — APPROVED (Option A).** The floor counts distinct artifact paths (path and
> sha256 at the pre-fix revision) that carry at least one determinable defect. A defect counts
> once however many documents restate it; later revisions of a path are the same plan. Reports
> show a subject-cluster count as a sensitivity row. The minimum of 15 plans and 15 defects is
> unchanged.
>
> **R-D — APPROVED (Option A); Option B rejected.** A defect recorded only in an artifact's own
> later, still-open status line is not a "recorded gap" for eligibility; a self-recorded
> unresolved limitation is not independent evidence.
>
> **Historical corpus — RECORDED as below the floor; experiment DEFERRED.** On the evidence in
> this repository, the historical set does not reach 15 plans and 15 determinable defects under
> the conservative rules (proposed 5 / 6; approved 0 / 0). Under protocol §5.3 the run is
> deferred and the floor is not lowered. Any recall comparison would be INCONCLUSIVE. No seeded
> case replaces a historical case. This is revisited only if an independent historical evidence
> source is authorized.
>
> This decision records eligibility rules and a disposition only. It does not promote any
> candidate, establish determinability, adjudicate, construct a case, freeze the corpus, or
> authorize execution.

Recorded from the operator's explicit instruction on 2026-11-21. The approver is Tom Tran. No
signature, time of day, or additional authorization is recorded; none was given. **Date basis:**
the repository's own timestamps are 2026-11-21 (`-0600`) for every decision commit, including this
record's, and Decisions 1–7 use 2026-11-21; an earlier draft of this record read 2026-11-22 from a
UTC/local mix-up and was corrected against that evidence.

## 2. Protocol basis

Each ruling applies §5.3 and Decision 3; none amends either. The mapping was verified against the
registered text, which admits exactly four evidence forms — "merged fixes, erratum, reverted change
or recorded gap" — and requires the owner to record determinability "from the document and its
parent revision, with a one-line basis".

| Ruling | §5.3 basis | Effect |
| --- | --- | --- |
| R-B | A review thread is none of the four admitted evidence forms | **Stricter** — excludes bot-only leads |
| R-C-packet | "plans, specs or requirements" must state something an implementer would follow | **Stricter** — packet-level test |
| R-C | Procedures are not plans, specs or requirements unless they state implementable requirements | **Stricter** |
| Counting unit | §5.3 counts "plans" and records per-plan defects; a revision is not a new plan | **Stricter** — prevents revision inflation |
| R-D | A still-open self-report is not one of the four admitted forms | **Stricter** |

The deferral rests on the same section verbatim: *"If replacements cannot reach the
15-determinable-defect floor, the run is deferred; the floor is not lowered"* and *"Below the
floor, every recall comparison is INCONCLUSIVE."* **Conflict check:** no conflict with an explicit
registered requirement; every ruling narrows eligibility, and the numeric minimums, arms,
thresholds, the 613-call ceiling, and the stopping rules are untouched.

## 3. Effect on the reconciled counts

The rulings define the counting rule; they do not promote any candidate. "Proposed" means the
conservative classification under the approved rules; "approved eligible" stays 0 until the
corpus owner records determinability for each admitted candidate at freeze.

| Classification | Plans | Defects | Shortfall vs 15 / 15 | Basis |
| --- | ---: | ---: | --- | --- |
| **Approved eligible** | **0** | **0** | **15 / 15** | no determinability recorded; 0 adjudicated |
| C0 official baseline | 3 | 4 | 12 / 11 | Decision 2 dispositions, not eligibility |
| C1 conservative proposals | 4 | 5 | 11 / 10 | C0 + `GH3-001a`; needs no R-B / R-C / R-C-packet ruling |
| **S2 — approved rules** | **5** | **6** | **10 / 9** | C1 + `GH-013` under R-C-packet Option A |
| C2 conditional maximum | 13 | 15 | 2 / 0 | not adopted; needs permissive rulings |
| C3 cluster sensitivity | 11 | 15 | 4 / 0 | sensitivity row only, never a total |

Arithmetic: S2 = 3 + 1 + 1 = 5 plans and 4 + 1 + 1 = 6 defects. R-B Option A excludes the six
T3 leads and the one T2 lead; R-C Option A contributes 0; R-D Option A contributes 0 outside S7;
the counting unit fixes 13 distinct artifact paths at C2.

**S6 (17 / 20) and S7 (15 / 20) are not adopted.** Both require permissive rulings the decision
package recommends against, and S7 has no plan slack. The registered minimum is not lowered.

## 4. S2 candidates and evidence pointers (verified, not promoted)

Each pin was re-verified against `origin/main`: the commit exists, is an ancestor of `main`, the
artifact is present at the pin, and the blob SHA-256 prefix matches the
[reconciliation register](HISTORICAL-CORPUS-FINAL-RECONCILIATION.md) §3.

| Candidate | Artifact @ pin | Blob SHA-256 | Evidence pointer | Ruling needed now |
| --- | --- | --- | --- | --- |
| `IDR-L-003` | `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` @ `4fb3b8a3` | `085a190a3430` ✓ | `05685308`; PR #363 merge `e90ecf97`; corrected LLM09 report | owner determinability |
| `IDR-L-004` | `docs/specs/REVIEW.md` @ `5fce2b6a` | `356d83a467dc` ✓ | fix commits `4223e10b`, `ba7736d4`; RSH-001/RSH-005 reports | owner determinability |
| `IDR-L-007` | `docs/specs/PLAN-HISTORICALIZATION.md` @ `fcf53888` | `a9d5f644487d` ✓ | fix `b14fcabe`; PR #322 merge `49a8f589` | owner determinability |
| `GH3-001a` | `docs/specs/PROMPT-EXECUTION.md` @ `1f9dfae8` | `991d61f0a55f` ✓ | spec change `aa53c882`; gap `d46bc167` | owner records the lead |
| `GH-013` | `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f2` | `9efbbf7c0a03` ✓ | fix `54beaf27`; PR #486 | R-C-packet admit + owner determinability |

All five satisfy the approved rules. **None is promoted:** entering a candidate in
`candidates.json` and recording its determinability basis is a separate governance step at
freeze, and the experiment is now deferred. `GH-010` remains outside S2 (its recorded gap is
confirmatory and its packet is an assessment); admitting it would make S2 6 / 7 and is not
authorized by this record.

## 5. Evidence source — Option B (defer)

No bounded search of an additional historical evidence source is authorized. The sibling
repositories (`inventory`, `medical-billing`, `portfolios`, `scheduling`) are **not** opened: no
named scope or data-handling rule was given, and `medical-billing` may contain sensitive data.

Consequences recorded:

1. No new source is searched, and no lead from a new source enters any count.
2. The floor stays at 15 plans / 15 determinable defects; it is not lowered or reinterpreted.
3. No seeded, synthetic or control case substitutes for a historical case (Decision 3).
4. The experiment is deferred under protocol §5.3; a recall comparison would be INCONCLUSIVE.
5. The workstream is reopened only on (a) an operator-authorized new independent source,
   (b) a new independent human review record, or (c) an operator ruling that makes an already
   listed lead count and needs its line-level human verification.

## 6. What this decision does and does not do

Established:

- The five eligibility rules in force are the Option A readings recorded in §1.
- The historical corpus is recorded as below the registered floor under those rules (5 / 6
  proposed; 0 / 0 approved).
- The experiment is deferred under protocol §5.3 for the historical arm.

Not established or authorized:

- No candidate is promoted to final eligibility; the count remains **0 plans / 0 defects**.
- No determinability basis is recorded; the corpus owner still records each basis at freeze.
- No independent adjudication occurs; the count remains **0**.
- `candidates.json` is not modified (23 entries, `NOT_FROZEN`).
- No additional evidence source is authorized or searched.
- No seeded, control, or synthetic case is constructed; no answer key or sealed manifest exists.
- No case checker or adjudicator is designated.
- No corpus freeze is recorded; the corpus remains `NOT FROZEN`.
- No threshold, arm, cost ceiling (including 613 calls), or protocol text changes.
- No model inference or experiment execution occurs; H1/H2 remain `NOT ESTABLISHED`.

## 7. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** — DECISION-RECORD-002 |
| 3 | Strict versus partial eligibility | **APPROVED 2026-11-21** — conservative; DECISION-RECORD-003 |
| 3a | R-B, R-C-packet, R-C, counting unit, R-D | **APPROVED 2026-11-21** — this record |
| 3b | Historical corpus disposition | **RECORDED 2026-11-21 — below the floor; DEFERRED** — this record |
| 4 | Proposed protocol amendment | **REJECTED (unnecessary) 2026-11-21** — DECISION-RECORD-004 |
| 5 | Independent seeder | **APPROVED 2026-11-21** — Tom Tran; DECISION-RECORD-005 |
| 5 | Case checker, canonicalization/storage/recovery/residual risks, construction (D5-A, B1–B4, C) | **PENDING / UNAUTHORIZED** — `case_construction_authorized: false` |
| 6 | Two independent human adjudicators | **PENDING** — 0 of 2 assessed or designated |
| 7 | Thresholds and cost limits | **APPROVED 2026-11-21** — 613-call ceiling; EP-15 UNVERIFIED |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING / DEFERRED** — not authorized |

## 8. Consistency

- The registered protocol is byte-identical; its SHA-256 is unchanged.
- `candidates.json` is unchanged: 23 entries; `include-with-review` 10, `hold` 5, `exclude` 8;
  evidence state 5 verified / 7 proposed / 3 unverified / 8 excluded; `NOT_FROZEN`; no `IDR-L`
  entry. No candidate is promoted.
- The Decision 2 dispositions and Decision 3 rule are preserved; this record applies them.
- Decisions 6, 8 and 9 and the Decision 5 governance items are unchanged and remain open.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY` and is deferred; H1/H2
  remain `NOT ESTABLISHED`.
- The proposal documents ([operator handoff](HISTORICAL-CORPUS-OPERATOR-HANDOFF.md),
  [decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md),
  [reconciliation](HISTORICAL-CORPUS-FINAL-RECONCILIATION.md)) are preserved and not rewritten;
  this record is the operative decision.

## 9. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is not
used here. No local decision-memory entry is claimed. Recording the decisions is not an
experiment step: no case is constructed, nothing is sealed, and nothing is executed.
