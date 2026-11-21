# Decision Record 002 — Historical Inclusion and Exclusion (Independent Design Review / EGM)

**Status: APPROVED 2026-11-21.** The operator approved Decision 2 of the
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) and recorded the historical
candidate dispositions below. The decision is recorded here from the operator's explicit
instruction on 2026-11-21, in the repository's written decision-record form established by
[DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md). The prepared text in
[OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md) is not rewritten, and
no earlier evidence is made to imply that the decision was taken sooner.

| Layer | State |
| --- | --- |
| Operator approval | **APPROVED 2026-11-21** — Decision 2, historical inclusion and exclusion |
| Operator identity | **Tom Tran** (the repository operator and corpus owner) |
| Scope | Independent Design Review / EGM corpus |
| Strict, approved disposition | **include with review** — 3 proposed strict plans / 4 defects |
| Partial, approved disposition | **include with review** — 11 proposed partial plans / 11 defects |
| Verified-defect holds | **2** — `IDR-H-001`, `IDR-H-005` |
| Unverified holds | **3** — `IDR-E-006`, `IDR-E-010`, `IDR-E-012` |
| Evidence-blocked hold | **1** — `IDR-L-001` |
| Confirmed exclusions | **8 entries** + **3 leads** |
| Final eligibility | **NOT DETERMINED** — this is not a promotion |
| Independently adjudicated | **0** |
| Corpus status | **NOT FROZEN** |
| Experiment status | **NOT YET READY** |
| H1 / H2 | **NOT ESTABLISHED** |

**Verified at:** `origin/main` `209b9b0f` (2026-11-21), the merge of PR #594.

**Source of the decision:** the operator's explicit instruction in the session of
2026-11-21, prepared from
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) §2 and
[OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md) §3.

## 1. Recorded decision

> Decision 2 — APPROVED
>
> Include with review — proposed strict: `IDR-L-003` (1 defect), `IDR-L-004` (2 defects),
> `IDR-L-007` (1 defect). Total: 3 proposed strict plans / 4 defects.
>
> Include with review — proposed partial: the 10 existing manifest partial candidates, plus
> `IDR-L-008` (1 defect). Total: 11 proposed partial plans / 11 defects.
>
> Verified-defect holds: `IDR-H-001`, `IDR-H-005`.
>
> Unverified holds: `IDR-E-006`, `IDR-E-010`, `IDR-E-012`.
>
> Evidence-blocked hold: `IDR-L-001`.
>
> Confirmed exclusions: `IDR-H-002`, `IDR-H-007`, `IDR-H-008`, `IDR-E-005`, `IDR-E-011`,
> `IDR-X-001`, `IDR-X-002`, `IDR-X-003`.
>
> Excluded historical leads: `IDR-L-002`, `IDR-L-005`, `IDR-L-006`.
>
> This does not freeze the corpus or adjudicate any candidate.

Recorded from the operator's explicit instruction on 2026-11-21. The operator identity is
Tom Tran. No signature, time of day, or additional authorization is recorded; none was
given.

## 2. Approved dispositions

### 2.1 Include with review — proposed strict

| ID | Artifact @ pin | Defects | Basis |
| --- | --- | --- | --- |
| `IDR-L-003` | `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` @ `4fb3b8a3` | 1 | Unverifiable rebuild-scope acceptance criterion |
| `IDR-L-004` | `docs/specs/REVIEW.md` @ `5fce2b6a` | 2 | Review-scope contamination; missing malformed-output boundary |
| `IDR-L-007` | `docs/specs/PLAN-HISTORICALIZATION.md` @ `fcf53888` | 1 | Archive slot keyed by a non-unique plan id |

**Total: 3 plans / 4 defects.** The per-plan defect counts are 1, 2, 1.

### 2.2 Include with review — proposed partial

The 10 existing manifest partial candidates are preserved without reclassification:

`IDR-H-003`, `IDR-H-004`, `IDR-H-006`, `IDR-E-001`, `IDR-E-002`, `IDR-E-003`, `IDR-E-004`,
`IDR-E-007`, `IDR-E-008`, `IDR-E-009`.

Plus `IDR-L-008` (`docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f56`, 1 defect: the
evidence hash is document-scoped while the evidence unit is a chunk).

**Total: 11 plans / 11 defects.** A partially determinable defect remains outside the
conservative recall denominator until the eligibility rule in Decision 3 says otherwise.

### 2.3 Holds and exclusions

| Group | IDs | Disposition |
| --- | --- | --- |
| Verified-defect holds | `IDR-H-001`, `IDR-H-005` | hold; the defect evidence is preserved, no pre-defect artifact exists |
| Unverified holds | `IDR-E-006`, `IDR-E-010`, `IDR-E-012` | hold; out of every floor |
| Evidence-blocked | `IDR-L-001` | hold; the only later evidence is the artifact's own status line |
| Confirmed exclusions | `IDR-H-002`, `IDR-H-007`, `IDR-H-008`, `IDR-E-005`, `IDR-E-011`, `IDR-X-001`, `IDR-X-002`, `IDR-X-003` | exclude; the recorded reasons are unchanged |
| Excluded leads | `IDR-L-002`, `IDR-L-005`, `IDR-L-006` | exclude; implementation-only or no single pre-defect artifact |

## 3. Four distinct states, kept separate

| State | Meaning here |
| --- | --- |
| **Approved historical disposition** | What this record approves: include with review, hold, or exclude. |
| **Proposed determinability** | The extraction's proposal (strict or partial). It is not a corpus-owner determination. |
| **Final eligibility** | **Not determined.** It depends on the corpus owner recording a determinability basis and on Decision 3. No candidate is promoted to final eligibility. |
| **Independent adjudication** | **0.** No finding has been adjudicated; a corpus-owner record is not an adjudication. |

The manifest's `proposed_inclusion` and `proposed_determinability` fields are unchanged. The
approved dispositions live in this record, not in the manifest. The four new leads enter the
manifest only when the corpus owner records their determinability basis.

## 4. Interpretation risks preserved

- **`IDR-L-003` — strict versus partial.** The approved disposition is *proposed strict*. A
  corpus owner who reads `RETRIEVE-003` as applying only to a workspace-local directory may
  record it partial. The basis for strict is that the criterion is not testable without
  fixing the directory layout.
- **`IDR-L-004` defect (a) — strict versus partial.** The approved disposition is *proposed
  strict* for both defects. Defect (b) (no bounded malformed-output boundary) is the stronger
  strict call; defect (a) could be read as a missing requirement rather than a defect.
- **`IDR-L-008` — partial by design.** The contract explicitly chose whole-document
  projection and lower-confidence handling, so it is not proposed strict.
- **`IDR-L-001` — evidence-blocked.** It remains held under the preparation plan §2.3, which
  rejects a document whose only defect evidence is its own later status line.

## 5. What this decision does and does not do

Established:

- The historical disposition set is recorded exactly as listed in §2.
- The 10 existing manifest partial candidates are preserved without reclassification.
- Decisions 1 and 2 are recorded.

Not established or authorized:

- No candidate is promoted to final eligibility.
- No candidate is independently adjudicated; the count is **0**.
- No seeder or adjudicator is designated.
- No seeded or no-known-defect case is constructed.
- No corpus freeze is recorded.
- No threshold, eligibility rule, protocol text, or cost ceiling changes.
- No model inference or experiment execution occurs.
- Decisions 3–9 remain open.

## 6. Decision state

| # | Decision | State |
| --- | --- | --- |
| 1 | Corpus owner | **APPROVED 2026-11-21** — Tom Tran |
| 2 | Historical inclusion and exclusion | **APPROVED 2026-11-21** — this record |
| 3 | Strict versus partial eligibility | **PENDING** |
| 4 | Proposed protocol amendment | **PENDING** |
| 5 | Independent seeder | **PENDING** |
| 6 | Two independent human adjudicators | **PENDING** |
| 7 | Thresholds and cost limits | **PENDING** |
| 8 | Corpus and instruction freeze | **PENDING** — corpus `NOT FROZEN` |
| 9 | Full experiment execution | **PENDING** |

## 7. Consistency

- The manifest remains **23 entries**; its state and inclusion counts are unchanged.
- Exactly one corpus owner is named: Tom Tran.
- No adjudicator or seeder is designated.
- The corpus remains `NOT FROZEN`; the experiment remains `NOT YET READY`; H1/H2 remain
  `NOT ESTABLISHED`.
- The conditional PRD is unchanged.

## 8. Mechanism

The decision is recorded as a written operator decision in this file, the form the decision
package asks for ("record, in writing"). It creates no SOP task approval and no lifecycle
transition; [HUMAN-APPROVAL.md](../../specs/HUMAN-APPROVAL.md) governs task approvals and is
not used here. No local decision-memory entry is claimed.
