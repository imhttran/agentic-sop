# Historical Corpus — Final Reconciliation and Handoff

**Type:** Authoritative reconciliation record (non-normative). It consolidates the historical-evidence workstream into one status, one count table and one operator decision package. It records **no decision**,
promotes nothing, and changes no rule, manifest or count. **Every proposed classification and recommendation is PROPOSED — NOT APPROVED / PENDING OPERATOR APPROVAL.**
**Baseline:** `origin/main` @ `f21cc0fc5348aef1b338b32ee126e59ed5e4463c`, 2026-11-22.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md) §5.3 (SHA-256 `abba919e…`, unchanged). **Rule:** [DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md) (`conservative`).
**Count convention:** [CORPUS-COUNT-CONVENTION.md](CORPUS-COUNT-CONVENTION.md) (merged, #628). **Historical reports (preserved, not rewritten):**
[#610](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY.md), [#613](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY.md), [#620](HISTORICAL-CORPUS-GITHUB-EVIDENCE-DISCOVERY-2.md), [#626](HISTORICAL-CORPUS-STRICT-EVIDENCE-VERIFICATION.md),
[#627](HISTORICAL-CORPUS-EVIDENCE-DISCOVERY-3.md), [#618](DISC-001-H-001-EVIDENCE-VERIFICATION.md), [decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md), [gap closure](HISTORICAL-CORPUS-EVIDENCE-GAP-CLOSURE.md), [re-check](HISTORICAL-CORPUS-LINE-LEVEL-RECHECK.md).

## 1. Result

1. **Approved eligible historical count: 0 plans / 0 defects.** Decision 2 recorded *dispositions* ("include with review — proposed strict" 3 / 4) but no candidate is approved eligible, none has an owner-recorded determinability, and none is independently adjudicated (0 / 0).
2. **Every discovery PR is merged** (#613, #620, #626, #627, #628, #629, #630, #632, #634, #635, #636). **No EGM PR is open.** No operator eligibility decision (R-B, R-C-packet, R-C, counting unit, R-D) has been recorded: the decision records on `main` are 001–005 and 007.
3. **Reconciled proposed counts (unchanged):** C0 3 / 4; C1 4 / 5; C2 13 / 15 (2 plans short); C3 11 / 15 (4 short). **No new independent candidate** was found in this pass. All 23 revision pins in §3 were re-checked mechanically: each commit exists, is an ancestor of `origin/main`, and contains the named path.
4. **Under the rules the earlier packages recommend, the proposed total is 5 plans / 6 defects (10 / 9 short).** The floor is reachable only conditionally, in scenarios that need permissive rulings the packages recommend against (§6.3).
5. **Further discovery inside this repository is not justified** (§9). The smallest credible next action is operator rulings, then a decision on an evidence source or on recording the best attainable outcome as INCONCLUSIVE (§8).

## 2. State of the workstream

| PR | Status | Contribution |
| --- | --- | --- |
| #613, #620 | merged | first GitHub-evidence pass; second pass (six blocked leads verified) |
| #626, #627 | merged | line-level verification of five leads; third pass (`GH3-001`) |
| #628 | merged | count convention C0–C4 |
| #629, #630, #632 | merged | eligibility decision package and two corrections |
| #634, #635 | merged | gap-closure report and its S7 correction |
| #636 | merged | line-level re-check; `GH-004` (d) resolved; S7 defined inline |

Other agents' work (Decision 5-B tooling, Decision 6 and 7 records, count-convention and closeout worktrees) was not modified. The ignored `.agent-sdlc/` tree and all existing worktrees were preserved.

## 3. Evidence inventory and revision register

Each row was mechanically re-checked in this pass (commit exists; ancestor of `origin/main`; path present at the pin; blob SHA-256 prefix recomputed and equal to the prefix recorded in the cited report).

| ID | Artifact @ revision (full SHA) | Blob SHA-256 prefix |
| --- | --- | --- |
| IDR-L-003 | `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` @ `4fb3b8a3bff7e86e56eb7287fb6df800f6c2d737` | `085a190a3430` |
| IDR-L-004 | `docs/specs/REVIEW.md` @ `5fce2b6adcb0783ad87875550a91654a264ffcbf` | `356d83a467dc` |
| IDR-L-007 | `docs/specs/PLAN-HISTORICALIZATION.md` @ `fcf53888109500b4a12f330b88f1e19ffcad3c3b` | `a9d5f644487d` |
| GH3-001a | `docs/specs/PROMPT-EXECUTION.md` @ `1f9dfae8d94ab7a1faf603203bc6a60ea5b8e37a` | `991d61f0a55f` |
| GH-001 | `docs/plans/PLAN-Ponytail-Complexity-Audit-Cleanup.md` @ `208b4e72203f437bb0c7431c5afcbc5bbf63d4c4` | `b2f0be314373` |
| GH-002 | `docs/plans/OWASP-LLM-GAP-PROMPTS.md` @ `46f190f210bfc2b3fcb01bc498d2533a8d7f01bf` | `a36d6fa3c8c0` |
| GH-003 | `docs/plans/PROPOSAL-SEMGREP-IGNORED-COVERAGE.md` @ `62e04e19a21c7ae441e1c8a7e7fa02924983932b` | `f8248519d458` |
| GH-009 | `docs/plans/PROPOSAL-RUN-COMMAND-ISOLATION.md` @ `70ea0664bd65de90622b55a5f887c9cfaba1caec` | `7349b0b313c0` |
| GH-010 | `docs/plans/PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` @ `8495d54e26a78b28819ef062261efae6cfa610eb` | `85b07b0e8e29` |
| GH-011 | `docs/plans/PROPOSAL-LLM03-D4-ENFORCEMENT.md` @ `68015e0acd56c5b1318b7fffbe2f94705546980d` | `ac14f40b743e` |
| GH-012 | `docs/plans/PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md` @ `9aaa3a9c71653277b603043074177d5ea12cdc48` | `191a5d691c8c` |
| GH-013 | `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md` @ `de7161f27f1abec2a82112b9caf2d29c744313c8` | `9efbbf7c0a03` |
| GH-014 | `docs/plans/PROPOSAL-LLM03-W8-SCANNER-ENFORCEMENT.md` @ `e7a09f23250666ec028c2cabc341c860e8e7bc8a` | `92e8ebe9b57c` |
| GH-004 | `docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md` @ `c6307fd34d0957bd07349aa331819eb34692766f` and `022e7568555e1fc3990869d9c1ff95f4cfe15880` | `6759d860e0fa`, `8141f02df95e` |
| GH-005 | `docs/reference/CLI.md` @ `d8aa908dd3a231f749497d9a32610f2cee52d748` | `c8b4c2710fb7` |
| GH-006 | `docs/guides/PROMPTING.md` @ `cef3b8a910939f4c17664f9c9dec9fced6ee6b29` | `3ff1e8d54bb8` |
| GH2-001 | `skills/decision-ab/SKILL.md` @ `62388bab9a52d95c65259e8e6afa713bc6ab3fcb` | `42c87240e1e6` |
| GH2-002 | `skills/sop-fix-conflicts/SKILL.md` @ `52c835dd854ed7db4f3f8da46ebb2b81ede575e6` | `386524e69877` |
| IDR-L-001 | `docs/plans/PLAN-Code-Retrieval-Context-Seam.md` @ `21564b27bd9f88c288a47ba71b492b77e9468db3` | `75513c8629d8` |
| IDR-L-008 | `docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f5651cb822d501cf8b095c9f6dd54f1755a` | `20ab7c62867d` |
| IDR-H-001 | `docs/plans/PLAN-SOP-ACP-ZED.md` @ `e462deed5fdcbae5c158467431c41fc4e05eb077` | `d9d5539c79ac` |
| DISC-001 | `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` @ `ade6864a78fc743c52d37017600790ed1fd40c86` | `990ecfc7aeb1` |

`GH2-003`–`GH2-005` (`skills/sop-ci-fix`, `skills/codex-review-triage`, `skills/sop-cleanup`) are triaged only and have no verified pin in this register.

## 4. Candidate reconciliation matrix

Evidence tiers (decision package §3): **T1** merged fix, erratum or recorded gap; **T1\*** recorded gap needing an owner ruling; **T2** shipped code diverged from the document; **T3** unresolved bot finding plus deterministic check.
"V" = verified by a model by a deterministic check (line-level where stated); **no item is human-verified or adjudicated.** Source = the report that holds the evidence.

| ID | Defects | Tier | Verification (source) | Determinable without hindsight | Overlap / duplicate | Open decisions | Class | Count class |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| IDR-L-003 | 1 | T1 | V (HISTORICAL-LEAD-INVESTIGATION §3.3) | yes | manifest entry `IDR-H-008` is excluded; only the lead counts | owner determinability | STRICT (Decision 2 disposition) | C0 |
| IDR-L-004 | 2 | T1 | V (HISTORICAL-LEAD-INVESTIGATION §3.4) | yes | manifest `IDR-E-005` excluded; `IDR-E-006` f.1 shares its subject | owner determinability | STRICT (Decision 2 disposition) | C0 |
| IDR-L-007 | 1 | T1 | V (HISTORICAL-LEAD-INVESTIGATION) | yes | none | owner determinability | STRICT (Decision 2 disposition) | C0 |
| GH3-001a | 1 | T1 (merged spec change `aa53c88238443cae1298cccb706f8024b5bd9ca3`; gap `d46bc1674e6f25e6c57219b0e49e0853a94f40e5`) | V (#627 §4.2) | yes | none; `GH3-001b` is the same plan, PARTIAL | owner records the lead | STRICT_CANDIDATE | C1 |
| GH-013 | 1 | T1 (fix `54beaf27d7138bd93b4d04baf671f868de5db318`, #486) | V line-level, reproduced (#626 §6.1) | yes | D4-5 cluster | R-C-packet | STRICT_CANDIDATE | C2 |
| GH-009 | 2 | T2 | V line-level (#626 §6.5) | yes | Gap 13 cluster with GH-010 | R-B, R-C-packet | STRICT_CANDIDATE | C2 |
| GH-010 | 1 | T1\* (`PONYTAIL-DEBT.md`, confirmatory) | V line-level (#627 §6, re-check §3.1); Linux not reproduced | yes | Gap 13 cluster | owner ruling on the gap; R-C-packet (likely reject) | STRICT_CANDIDATE | C2 |
| GH-011 | 1 | T3 | V line-level (re-check §3.2) | yes | D4 cluster with GH-012 | R-B, R-C-packet | STRICT_CANDIDATE | C2 |
| GH-012 | 1 (minor) | T3 | V line-level (re-check §3.3) | yes | D4 cluster with GH-011 | R-B, R-C-packet (likely reject) | STRICT_CANDIDATE | C2 |
| GH-014 | 1 | T3 | V line-level (re-check §3.4) | yes | none | R-B, R-C-packet | STRICT_CANDIDATE | C2 |
| GH-001 | 1 | T3 (no corrective change) | V (#626 §6.3) | yes | none | R-B | STRICT_CANDIDATE | C2 |
| GH-002 | 1 | T3 | V (#626 §6.2) | yes (step 1 may mitigate) | none | R-B; prompts-document kind | STRICT_CANDIDATE | C2 |
| GH-003 | 1 (minor) | T3 | V, scratch reproduction (#626 §6.4) | yes | none | R-B, R-C-packet | STRICT_CANDIDATE | C2 |
| GH-004 | 4 (a–d) | T3 | V (#613, #620, re-check §2); PR-head pins | yes | ACP subject of DISC-001 / IDR-H-001 | R-B, R-C-packet (likely reject), R-A | conditional | C4 |
| GH-005 | 1 | T3 | V (gap closure §5.3) | n/a (accuracy defect) | none | R-C, R-B | EXCLUDED under recommended R-C | C4 |
| GH-006 | 1 | T3 | V (#620 §5.2) | n/a | none | R-C, R-B | EXCLUDED under recommended R-C | C4 |
| GH2-001 | 2 | T3 | V static (gap closure §5.4) | n/a | none | R-C, R-B | EXCLUDED under recommended R-C | C4 |
| GH2-002 | 1 | T3 | anchors only, not executed | n/a | none | R-C, R-B | EXCLUDED under recommended R-C | C4 |
| GH2-003–005 | — | T3 | triaged only | — | none | R-C | EVIDENCE_BLOCKED | C4 |
| IDR-L-001 | 1 | status line only | V (gap closure §5.1); gap persists on `main` | yes | same path as excluded `IDR-H-002` | R-D | EVIDENCE_BLOCKED | C4 |
| IDR-L-008 | 1 | T1 | Decision 2 §2.2 | partially | none | none (partial by design) | PARTIAL | outside floors |
| IDR-H-001 / DISC-001 | see #618 | — | V (#618) | yes at its pins | each other | corpus-owner rulings (#618) | held | C4 |
| 10 manifest partials (`IDR-H-003`, `-004`, `-006`, `IDR-E-001`–`004`, `-007`–`-009`) | 10 | — | pin and hash checked (CANDIDATE-VERIFICATION) | partially | none | Decision 3 | PARTIAL | outside floors |
| `IDR-E-006`, `-010`, `-012` | — | — | gap closure §5.6 | no | `-006` f.1 duplicates `IDR-L-004`; `-012` duplicates excluded `IDR-L-006` | none | EVIDENCE_BLOCKED / DUPLICATE / EXCLUDED | outside floors |
| Excluded: `IDR-H-002`, `-007`, `-008`, `IDR-E-005`, `-011`, `IDR-X-001`–`-003`, `IDR-L-002`, `-005`, `-006` | — | — | Decision 2 | — | — | none | EXCLUDED | 0 |

Distinguishing the classes the brief asked for: **verified evidence** = the "V" column; **proposed eligibility** = the Class column; **approved eligibility** = none; **excluded** and **evidence-blocked** = the last three rows and the Class column.

## 5. Duplicate and contamination analysis

- **Path and defect additivity (verified):** the 13 C2 artifact paths (3 baseline, 3 from #613, 6 from #620, 1 from #627) are distinct; no C2 path appears in two layers. The convention's two record collisions (`REVIEW.md`, `PLAN-09-…`) count once through the lead entry. `GH-004` shares a subject with `DISC-001` / `IDR-H-001` (not additive by cluster); `IDR-L-001` shares a path with the excluded `IDR-H-002`.
- **Subject clusters:** Gap 13 (`GH-009`, `GH-010`), LLM03 D4 (`GH-011`, `GH-012`), D4-5 (`GH-013`), ACP (`GH-004`, `DISC-001`, `IDR-H-001`). C3 covers the first two only.
- **Contamination:** the C2 pin trees contain no EGM or corpus-record file (#626 §4.1; the four #620 pins in the re-check). Review threads, later fix commits and these reports are outside the pin trees. **Open design question:** which subset of a pin tree a reviewer would see is a case-construction decision (not authorized).
- **Circular origin:** all nine #613 / #620 additions were first reported by one automated reviewer; only the baseline and `GH3-001a` have a non-bot origin (decision package §3.2).

## 6. Reconciled counts

### 6.1 The six requested figures, kept apart

| # | Figure | Plans / defects | Shortfall vs 15 / 15 | Basis |
| --- | --- | --- | --- | --- |
| 1 | **Official approved eligible count** | **0 / 0** | 15 / 15 | no eligibility approved; 0 adjudicated |
| — | Recorded Decision 2 dispositions (C0), not eligibility | 3 / 4 strict-proposed; 11 / 11 partial-proposed | 12 / 11 | DECISION-RECORD-002 |
| 2 | **Conservative strict proposals (C1)** | 4 / 5 | 11 / 10 | C0 + `GH3-001a` (no open ruling) |
| 3 | **Conditional maximum (C2)** | 13 / 15 | 2 / 0 | C1 plus the #613 and #620 layers; every ruling favorable |
| 4 | **Cluster-adjusted sensitivity (C3)** | 11 / 15 | 4 / 0 | C2 with the #620 clusters |
| 5 | **Newly verified independent candidates** (this pass) | 0 / 0 | — | none found |
| 6 | **Remaining shortfall** | see rows 1–4 | | |

The conditional rows never include or replace the approved row. No defect or plan is counted twice (§5).

### 6.2 Scenarios (from the decision package and gap-closure report; arithmetic re-checked)

| Scenario | Rulings | Plans / defects | Shortfall |
| --- | --- | --- | --- |
| S1 | R-B A, no packets | 4 / 5 | 11 / 10 |
| **S2 — recommended rules** | R-B A, R-C-packet A, unit A, R-C A, R-D no | **5 / 6** (+ `GH-013`) | **10 / 9** |
| S3 | R-B B, no packets | 6 / 7 | 9 / 8 |
| S4 = C2 | R-B B, all packets | 13 / 15 | 2 / 0 |
| S5 = C3 | S4 by cluster | 11 / 15 | 4 / 0 |
| S6 | S4 + R-C Option C (`GH-005`, `GH-006`, `GH2-001`, `GH2-002`) | 17 / 20 | 0 / 0, slack 2 / 5 |
| S7 | S4 + R-D yes (`IDR-L-001`) + `GH-004` admitted (4 defects) | 15 / 20 | 0 / 0, no plan slack |

S6 and S7 contain held leads, which the convention says are never totalled into C0–C3; they are shown here only as the two conditional routes to the floor. If `GH-010` is admitted (owner accepts its recorded gap and the packet), S2 becomes 6 / 7.

### 6.3 Final gap assessment

| Question | Answer |
| --- | --- |
| Satisfied by approved eligible evidence? | **No** (0 / 0). |
| Potentially satisfied pending operator decisions? | **Only in S6 or S7**, each needing permissive rulings the packages recommend against (R-B B plus R-C-packet "all admitted" plus R-C Option C, or R-D yes with approval packets admitted), every adjudication to succeed, and in S7 no plan slack. |
| Still short despite favorable decisions? | **Yes, for every combination that does not include S6 or S7's extra rulings:** C2 is 2 plans short. |
| Short under the recommended rules? | **Yes: 10 plans / 9 defects (S2).** |
| Blocked by unavailable evidence? | Partly: `IDR-L-001` (self-recorded only), `IDR-E-006`, `GH2-003`–`005`, and the absence of any human-reviewed historical source in this repository. |

## 7. Operator decision package

Evidence for each option is in the [decision package](HISTORICAL-CORPUS-ELIGIBILITY-DECISION-PACKAGE.md) §3–§6 and the [gap-closure report](HISTORICAL-CORPUS-EVIDENCE-GAP-CLOSURE.md) §7; it is not repeated. All recommendations are **PROPOSED — PENDING OPERATOR APPROVAL**; nothing is recorded.

| # | Exact question | Recommended | Effect on count | Principal risk |
| --- | --- | --- | --- | --- |
| R-B | May a defect first surfaced by an automated reviewer count when no merged fix, erratum, reverted change or recorded gap documents it? | **No** (Option A) | A: S1/S2 (4–5 plans). B: up to C2 (13) | B needs a recorded reading of "recorded gap" or an amendment (Decision 4 rejected one); circular denominator |
| R-C-packet | Is a decision or proposal packet a plan, spec or requirements artifact? | **Yes if it states a design at a pre-fix revision; no if it only compares options, assesses risk or requests approval** | +`GH-013` (S2); `GH-010`, `GH-012`, `GH-004` likely out | owner rules per packet |
| Counting unit | What does "15 plans" count? | **Distinct artifact path; a defect counts once; cluster counts as a sensitivity row only** | A: C2 13; B (cluster): C3 11 | an undefined cluster map is arbitrary |
| R-C | Do skills, guides, reference docs and procedures count? | **No**, unless they state implementable requirements and a T1 pointer exists | A: 0; C: +4 plans (S6) | excluded artifacts supply the S6 margin |
| R-D | May a defect recorded only in the artifact's own later, still-open status line count as a "recorded gap"? | **No** | +1 plan (`IDR-L-001`), only in S7 | self-record is not independent |
| R-A (for `GH-004`) | May a defect in a PR-head revision of an approval packet count? | **No** (unneeded if packets are rejected) | affects S7 only | later-revision defects depend on the earlier fix |

Copy-ready wording (each: record only if the operator chooses it; the alternative is to reject or amend):

> **R-B — APPROVED (Option A).** A historical defect first surfaced by an automated reviewer is eligible only if a merged fix, erratum, reverted change or recorded gap independently documents it. A review finding alone is not such a record. The corpus owner records determinability at freeze; a model's verification is not independent human evidence. This applies Decision 3 and §5.3 as written and amends neither.

> **R-C-packet — APPROVED (role test).** A historical decision or proposal packet is eligible as a plan, spec or requirements artifact only if its text, at a pre-fix revision, states a design (an action sequence, control, requirement or policy) that an implementer would follow. A packet that only compares options, assesses residual risk or requests approval is not eligible. The corpus owner rules per packet at freeze.

> **Counting unit — APPROVED (artifact path).** The historical floor counts distinct artifact paths (identified by path and sha256 at the pre-fix revision) that carry at least one determinable defect. A defect counts once however many documents restate it; later revisions of a path are the same plan. Reports show a subject-cluster count as a sensitivity row. The minimum of 15 plans and 15 defects is unchanged.

> **R-C — APPROVED (not eligible).** Skills, guides, reference documents and operational procedures are not eligible as historical plans, specs or requirements, unless a document states implementable requirements and a merged fix, erratum or recorded gap documents the defect. The corpus owner rules on any such document at freeze.

> **R-D — REJECTED.** A defect recorded only in an artifact's own later status line, with the gap still open, is not a "recorded gap" for eligibility.

Decisions the PRs #635 and #636 added: **R-D** (introduced by the gap-closure report) and the **R-A** question for `GH-004`; #635 and #636 otherwise corrected scenario arithmetic and added no decision.

## 8. Smallest credible next action

1. **Record R-B, R-C-packet, the counting unit and R-C** (one sitting; wording above). Result under the recommended rules: S2, 5 plans / 6 defects.
2. **Then decide the evidence source.** The recommended rules leave the historical set about 10 plans short, so the choice is between (a) authorizing a search of other human-reviewed material, for example the operator's sibling repositories (`inventory`, `medical-billing`, `portfolios`, `scheduling`; separate repositories, one possibly sensitive, independence to be re-established), and (b) recording that the best attainable outcome is INCONCLUSIVE.
3. Do not substitute seeded or synthetic cases for historical ones, and do not lower the 15 / 15 floor; both are outside this record.

## 9. Is further historical discovery justified?

**No, not inside this repository.** The #610 survey, #613/#620 review-thread sweeps, #626/#627 verification and the #634/#636 held-lead and spec-amendment sweeps have each returned no further plan-level strict candidate; the commit-subject sweep left 102 of 212 corrective-keyword subjects unreviewed, with no sign of new leads in the 110 reviewed. Leads stopped for lack of pre-fix evidence or independent record: `IDR-L-001`, `IDR-E-006`, `IDR-E-010`, `IDR-E-012`, `IDR-H-005`, `GH2-003`–`GH2-005`.
New work would be justified only if the operator authorizes a different source (§8.2) or rules R-B and R-C-packet favorably so that line-level human verification of the C2 leads becomes the next step.

## 10. Decision 8 (freeze) dependencies

From the [operator package §8](OPERATOR-DECISION-PACKAGE.md), [READINESS.md](READINESS.md) §7–§8, [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) and [DECISION-005-FINAL-CLOSEOUT.md](DECISION-005-FINAL-CLOSEOUT.md). Verified state on `main`:

| Dependency | State |
| --- | --- |
| Decisions 1–4, 7 recorded | recorded (1, 2, 3, 4 rejected, 7) |
| Decision 5 | seeder recorded; checker, canonicalization, storage, recovery, residual risks (D5-A, B1–B4, C) pending; `case_construction_authorized` false |
| Decision 6 | pending; 0 of 2 adjudicators assessed or designated |
| Historical floors | **unmet** (§6): approved 0 / 0; recommended rules 5 / 6 |
| Eligibility rulings (R-B, R-C-packet, R-C, unit, R-D) | none recorded |
| Manifest | `candidates.json` has **no** entry for any C0 or later strict lead (0 `IDR-L` ids); it holds 23 `IDR-H/E/X` entries (5 verified, 7 proposed, 3 unverified, 8 excluded). Adding the strict set needs the owner's governance process; this report does not touch it |
| Determinability record | the owner must record a one-line basis for each defect at freeze; 0 recorded |
| Seeded and no-known-defect sets | not constructed |
| Instruction blocks | draft |
| Workstream B residuals | EP-15 UNVERIFIED; EP-02/09/10/12/16 PARTIAL |
| Adjudication | 0 / 0 |

**Stale text, flagged, not rewritten:** `READINESS.md` §8 (rows 1 and 4) still reads "0 strict proposed … shortfall 15/15 or 5/5", which predates Decision 2's recording of 3 / 4. The convention and this report supersede it for reading counts; the original is preserved.

## 11. Recommended disposition of the historical discovery workstream

PROPOSED: close further in-repository discovery as **evidence-exhausted pending operator rulings**; keep all historical reports as the audit trail; treat this report as the authoritative count reference (with the convention); reopen only for an operator-authorized new source or after the rulings in §7. Record no promotion until the corpus owner's process does so.

## 12. Non-authorization statement

This report authorizes nothing and records no decision. It does not: promote any candidate; approve R-B, R-C-packet, R-C, R-D, R-A or the counting unit; change Decision 3, any threshold (including the 613-call ceiling) or the registered protocol; modify `candidates.json`, Decision 5 work or any frozen or sealed artifact;
construct a seeded or control case; create a sealed answer key; designate a case checker or adjudicator; authorize case construction; adjudicate; freeze the corpus; run model inference; or execute the experiment. The corpus remains `NOT FROZEN`, the experiment `NOT YET READY`, and H1/H2 `NOT ESTABLISHED`.
