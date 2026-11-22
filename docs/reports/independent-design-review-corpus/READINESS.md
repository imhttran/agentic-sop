# Independent Design Review Corpus — Readiness Summary

**Type:** Corpus evidence summary (non-normative). It records the Workstream A candidate
corpus, its evidence states, and whether it is ready for a corpus-owner freeze. It is not
an experiment result and does not authorize the conditional PRD.
**Baseline:** `origin/main` @ `ade6864a`, plus the restored pilot evidence commit. The
current post-Workstream B baseline is `origin/main` @ `a10edc28`, the merge of PR #574;
the later §9 reconciliation uses it. The manifest records the earlier `1b61db64` baseline
for its candidate revisions.
**Manifest:** [candidates.json](candidates.json).
**Candidate verification:** [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).
**Preparation plan:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).
**Operator decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).
**Decision record draft:** [OPERATOR-DECISION-RECORD-DRAFT.md](OPERATOR-DECISION-RECORD-DRAFT.md).
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md).
**Historical lead investigation:** [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md).
**Case construction specification:** [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md).
**Pilot evidence:** [INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md](../INDEPENDENT-DESIGN-REVIEW-PILOT-RESULTS.md).
**Workstream B evidence:** [INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md](../INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md).
**Workstream B reconciliation:** [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Feasibility report:** [INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md](../INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md).
**Plan of record:** [PLAN-Independent-Design-Review-Feasibility.md](../../plans/PLAN-Independent-Design-Review-Feasibility.md).
**Freeze status:** **NOT FROZEN.**

## 0. Readiness package

This summary is part of a five-document package:

- [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md) — revision, hash, permitted-reference,
  leakage, and discoverability checks for every manifest entry.
- [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md) — the plan for additional
  historical leads, seeded cases, no-known-defect cases, sealing, and input boundaries.
- [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md) — the nine decisions, their
  evidence, options, and consequences.
- [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) — the freeze record and post-freeze mutation
  controls, with the current freeze-readiness assessment in §8; not executed.
- [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md) — the merged Workstream B
  evidence mapped onto the protocol's execution-readiness requirements.
- This summary.

The package is evidence preparation. It does not freeze, approve, nominate, or execute.

## 1. Verified historical defect chains

Five candidates have a verified later defect or gap. None is a valid strict determinable
corpus input after reconciliation: two are held pending corpus-owner review because no
uncontaminated pre-defect artifact exists, and three are verified gaps whose
determinability is partial.

| ID          | Original artifact                                                               | Later evidence                                                              | Defect or gap                                                                                                                               | Proposed determinability                              |
| ----------- | ------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `IDR-H-001` | `docs/plans/PLAN-SOP-ACP-ZED.md` @ `e462deed`                                   | `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`                                 | The ACP acceptance matrix claims the project lock protects CLI, MCP, and ACP equally; the lock is metadata-only and not held for execution. | Pending corpus-owner review (no pre-defect artifact). |
| `IDR-H-003` | `docs/plans/PLAN-SOP-RUN-ACCEPT-001.md` @ `56ff82ac`                            | `docs/reports/PLAN-STATUS-AUDIT-2026-11-04.md`                              | Six acceptance checks fail after the later acceptance-mapping contract.                                                                     | Partially determinable.                               |
| `IDR-H-004` | `docs/plans/PLAN-04-SOP-LLM04-Supply-Chain.md` @ `e55527f4`                     | The plan status and `llm04-supply-chain.md`.                                | Historical outcomes remain PARTIAL until re-execution under the new acceptance mapping.                                                     | Partially determinable.                               |
| `IDR-H-005` | `docs/plans/PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md` @ `e5d45fc1` | `docs/reports/owasp-llm/llm02-disclosure.md` GAP-01/02/03.                  | Credential-shaped text in acceptance logs, validation failures, and JSON-escaped test output.                                               | Pending corpus-owner review (remediation commit).     |
| `IDR-H-006` | `docs/plans/PLAN-Security-Scanner-Addon.md` @ `1b611711`                        | `SECURITY-SCANNER-RESIDUAL-FINDINGS.md`, `SECURITY-DEPENDENCY-COVERAGE.md`. | Residual scanner findings and coverage gaps.                                                                                                | Partially determinable.                               |

The LLM02 candidate is held. `e5d45fc1` is the remediation commit: it adds the plan, the
disclosure report, and the GAP-01/GAP-02 fixes. It is an ancestor of the fix merge
`aa23217`, but it does not predate discovery, and no earlier plan revision exists in git.
The later disclosure report is excluded from the reviewer input.

The ACP-ZED candidate is held for the same structural reason: the plan and
`ACP-EXECUTION-EXCLUSION-GAP.md` entered git together in `79acbfc1`, so no earlier plan
revision exists. The verified defect evidence is preserved; only determinability is
pending.

The remaining three evidence-verified candidates in the feasibility report are not
corpus inputs:

- `IDR-H-002` (code-retrieval plan) has only a self-referential post-merge status line.
- `IDR-H-007` (Plan 08B) and `IDR-H-008` (Plan 09) are follow-up plans that contain the
  answer; the pre-fix Plan 08 / Plan 09 artifacts and revisions are required.

## 2. Proposed determinable defects

- **Strict proposed determinable:** 0. `IDR-H-001` is held pending corpus-owner review.
- **Proposed partially determinable:** 10. These are the three verified partial candidates
  (`IDR-H-003`, `IDR-H-004`, `IDR-H-006`) and seven proposed partial candidates
  (`IDR-E-001`–`IDR-E-004`, `IDR-E-007`–`IDR-E-009`).
- **Pending corpus-owner review:** 2 (`IDR-H-001`, `IDR-H-005`). Both have verified defect
  evidence, but no uncontaminated pre-defect artifact, so neither counts toward a floor.
- **Unverified:** 3.
- **Not determinable or not a defect:** 8.

All 17 recorded artifacts resolve at their pinned revisions and match their SHA-256 values.
Two permitted-reference lists were corrected (`IDR-H-003`, `IDR-H-006`) and two excluded
lists were cleared (`IDR-H-007`, `IDR-H-008`); the checks are recorded in
[CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).

A strict determinable defect is one a reviewer could identify from the pre-fix artifact
and its permitted repository context, without the later report. Partially determinable
defects depend on a later contract, a later report, or an interaction between documents.
No independent adjudicator has confirmed any classification.

## 3. Proposed eligible plans

Ten candidate plans have a proposed eligible defect: the ten partially determinable
candidates `IDR-H-003`, `IDR-H-004`, `IDR-H-006`, `IDR-E-001`–`IDR-E-004`,
`IDR-E-007`–`IDR-E-009`. `IDR-H-001` and `IDR-H-005` are held pending review and do not
count. This count is **proposed**, not frozen.

| Measure                                  | Count |
| ------------------------------------------------------------------ | ----- |
| Candidate manifest entries               | 23    |
| Evidence state `VERIFIED`                | 5     |
| Evidence state `PROPOSED`                | 7     |
| Evidence state `UNVERIFIED`              | 3     |
| Evidence state `EXCLUDED`                | 8     |
| Proposed inclusion `include`             | 0     |
| Proposed inclusion `include-with-review` | 10    |
| Proposed inclusion `hold`                | 5     |
| Proposed inclusion `exclude`             | 8     |
| Strict determinable defects              | 0     |
| Partially determinable defects           | 10    |
| Pending corpus-owner review (verified defect, no pre-defect input) | 2     |

The `hold` count of 5 is not the same as the two held candidates: it is the manifest's
inclusion field and comprises the 2 verified-defect holds (`IDR-H-001`, `IDR-H-005`) plus
the 3 unverified holds (`IDR-E-006`, `IDR-E-010`, `IDR-E-012`). Only the 2 verified-defect
holds are described as "held pending corpus-owner review"; the 3 unverified entries have
no identified parent artifact and count toward no floor either.

## 4. Evidence gaps and shortfall

| Floor                                                                            | Available | Shortfall |
| -------------------------------------------------------------------------------- | --------- | --------- |
| 15 plans with known defects, strict determinable only                            | 0         | 15        |
| 15 plans with known defects, if all 10 partial plans are frozen determinable     | 10        | 5         |
| 15 determinable known defects, strict only                                       | 0         | 15        |
| 15 determinable known defects, if all 10 partial defects are frozen determinable | 10        | 5         |
| 15 contributing plans per arm, strict only                                       | 0         | 15        |
| 15 contributing plans per arm, if all 10 partial plans are frozen                | 10        | 5         |
| About 8 seeded-defect cases                                                      | 0         | 8         |
| About 6 no-known-defect cases                                                    | 0         | 6         |

The partially determinable candidates do not count toward either floor under the current
conservative reading. The proposed classification amendment would let a plan count
toward the plan floor only when it has at least one independently supported, determinable
known defect. It would not count a partially determinable or hindsight-only defect toward
the determinable-defect floor. See
[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md).

The repository's defect history is largely implementation- and interaction-level. One
remediation plan (review-scope hardening) points at a pre-fix REVIEW spec that already
contained the correct behavior, so it is excluded as a design-corpus input. This is a
material limit on building a 15-plan historical design corpus from this repository alone.
The protocol's §5.3 candidate list also names `CONV-001-convergence-baseline.md`; that file
is a baseline report, not a defect-bearing plan or specification, so it is not a corpus
entry and is not in the manifest.

## 5. Seeded and no-known-defect preparation

No seeded-defect or no-known-defect case is constructed. The assessment requires about
eight seeded cases and about six no-known-defect cases, with an independent seeder and a
sealed manifest before the run. The construction method, case records, sealing, and input
boundaries are defined in
[CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md); that plan constructs nothing and
counts nothing.

This work is not started because:

- A seeder has not been designated.
- The sealed manifest and answer key must live outside the reviewer's project directory.
- Creating seeded results now would be indistinguishable from fabricated evidence.

The candidate selection method is recorded; the cases themselves are not.

## 6. Human adjudication requirements

The protocol requires:

- **Corpus owner** — selects and freezes the corpus, records determinability, and does not
  adjudicate.
- **Seeder** — creates seeded defects and the sealed manifest; does not adjudicate the
  seeded cases.
- **Analyst** — holds the unblinding key and applies the within-run merge; does not
  adjudicate.
- **First human adjudicator** — labels findings; must not be an author or direct editor
  of any corpus plan.
- **Second independent human adjudicator** — re-adjudicates the larger of 20 findings or a
  random 20% sample; must be independent of the first adjudicator and the seeder.

Repository evidence:

- `git shortlog -sne --all` shows two Tom Tran email identities and no other human
  identity. `Claude`, `Zed`, and `Eval` are agent identities.
- Co-author trailers include Tom Tran and several Claude identities; no other human
  co-author appears.
- A Git identity does not prove who wrote or directly edited a document. The operator is
  treated conservatively as an author or direct editor of the candidate plans.

**Availability is unconfirmed and insufficient.** Two independent human adjudicators who
did not author or directly edit the corpus plans are required for PASS. None is
identified. The operator may be the corpus owner, seeder, or analyst, but not an
adjudicator. The designations and the authorship checks are decisions 1 and 6 in
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).

## 7. Outstanding operator decisions

The full options, trade-offs, validity effects, and deferral consequences are in
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md). Only Decision 1 is recorded
([DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md)); the others are open.

| #   | Decision                           | Current state                                                                                    | Blocking evidence                                                            |
| --- | ---------------------------------- | ------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------- |
| 1   | Corpus owner                       | **Designated: Tom Tran (2026-11-21)**                                                            | Recorded in DECISION-RECORD-001; not an adjudicator                          |
| 2   | Historical inclusion and exclusion | **Recorded 2026-11-21** — see DECISION-RECORD-002 for the full disposition set                   | Recorded; no candidate promoted or adjudicated                               |
| 3   | Strict versus partial eligibility  | **Recorded 2026-11-21** — conservative; seeded set additional                                    | Recorded in DECISION-RECORD-003; the amendment remains pending               |
| 4   | Proposed protocol amendment        | **REJECTED (unnecessary) 2026-11-21**                                                            | Recorded in DECISION-RECORD-004; protocol unchanged                          |
| 5   | Independent seeder                 | **Recorded 2026-11-21** — seeder Tom Tran (also corpus owner); construction not authorized       | Recorded in DECISION-RECORD-005; checker, manifest, and construction open    |
| 6   | Two independent human adjudicators | **PREPARATION ONLY** — package ready; no adjudicator designated or assessed                      | 0 of 2 assessed; PASS unreachable without two independent humans             |
| 7   | Thresholds and cost limits         | **APPROVED 2026-11-21** — thresholds + 613-call ceiling; no dollar ceiling                       | Recorded in DECISION-RECORD-007; execution still requires Decision 9         |
| 8   | Corpus and instruction freeze      | **NOT FROZEN**; procedure prepared, not executed                                                 | Decisions 1–7 not recorded; corpus incomplete; instructions draft            |
| 9   | Full experiment execution          | Not authorized                                                                                   | Workstream B merged but EP-15 open; freeze and decisions 1–8 not recorded    |

Workstream B evidence is merged (PR #574), so it no longer appears as a missing report in
this table. It is recorded as a freeze dependency in
[WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md): EP-15 is UNVERIFIED,
EP-16 and EP-02/EP-09/EP-10/EP-12 are PARTIAL, and the merge resolves none of them. The
exact operator action for each decision is in
[OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).

The strict shortfall remains 15 plans and 15 determinable defects; the partial-inclusive
shortfall is 5 and 5.

## 8. Workstream A freeze readiness

**NOT READY FOR CORPUS-OWNER FREEZE.**

The manifest is complete enough to review, and nine pre-fix artifact references have been
extracted (`IDR-E-001`–`IDR-E-005`, `IDR-E-007`–`IDR-E-010`; `IDR-E-005` is excluded,
leaving seven proposed E partials plus the three verified H partials, for ten). It is not
ready to freeze because:

- No defect is strictly proposed determinable; ten are partial and two are held pending
  corpus-owner review.
- The plan and defect floors are not met under either the strict or the partial-inclusive
  reading: 0/15 strict, 10/15 partial-inclusive.
- The seeded and no-known-defect cases do not exist.
- No two independent human adjudicators are identified.
- The classification amendment is not approved.
- Workstream B evidence is merged but leaves EP-15 UNVERIFIED and several requirements
  PARTIAL; see §8.1.

### 8.1 Workstream B dependency

Workstream B is no longer pending as a missing report. PR #574 (merge `a10edc28`) merged
the execution-path verification and the `promptRunID` collision fix. That evidence records
the execution infrastructure as READY for controlled execution of arms A and B, including
concurrent calls, and gives the freeze record code-, test- and synthetic-runtime-backed
answers for configuration, isolation, timing method and evidence persistence.

It does not clear the freeze dependency, because:

- EP-15 (provider-side concurrency and parallel-batch latency) is UNVERIFIED.
- EP-02 (provider-attested model identity), EP-09 (retry accounting), EP-10 (raw reply
  persistence), EP-12 (parallel timing) and EP-16 (observed repository inspection) are
  PARTIAL.
- One unintended real cloud call occurred during the verification (EP-13 check); the
  report discloses it, and it is not experiment data.
- The two four-call pilots remain excluded; Pilot A's external timing record is invalid,
  and Pilot B's ACP-ZED runs are contaminated by the gap report in the clone.

The mapping is in [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md).

### 8.2 Full-experiment readiness verdict

**NOT YET READY.** The historical corpus is 0 strict and 10 partial, the seeded and
no-known-defect cases are not constructed, no two independent human adjudicators are
designated, the eligibility amendment is unapproved, the thresholds are unapproved, the
corpus is not frozen, and EP-15 remains an open execution dependency. Workstream B intake
is complete, but its open items do not close any of these conditions.

Workstream A can proceed to a corpus-owner review of the manifest. It must not label the
corpus `FROZEN` without the operator's decision and the corpus owner's recorded freeze. The
freeze record and post-freeze controls are specified in
[FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) and have not been executed.

## 9. Post-Workstream B reconciliation (current)

This section is later than the readiness package above and records the current state. It
does not rewrite the earlier counts; it reconciles them with the merged evidence.

**Chronology (preserved):**

1. Initial feasibility assessment (`64101f9e`).
2. Two four-call pilots (2026-11-21), excluded from the dataset.
3. Corpus verification and readiness package (PRs #569–#572).
4. Merged Workstream B verification (PR #574, merge `a10edc28`).
5. This post-Workstream B assessment.

**Current counts (unchanged):** 23 manifest entries; 5 `VERIFIED`, 7 `PROPOSED`, 3
`UNVERIFIED`, 8 `EXCLUDED`; 0 strict proposed determinable; 10 proposed partially
determinable; 2 verified-defect holds; 3 unverified holds; 8 excluded. Shortfall: 15 plans
and 15 determinable defects strict, or 5 and 5 if all 10 partial candidates are
independently frozen determinable.

**Current verdicts:**

| Measure                       | Verdict                                                  |
| ----------------------------- | -------------------------------------------------------- |
| Corpus                        | **NOT FROZEN**                                           |
| Corpus-owner freeze readiness | **NOT READY**                                            |
| Full experiment               | **NOT YET READY**                                        |
| H1/H2 outcomes                | **NOT ESTABLISHED**                                      |
| Workstream B intake           | **COMPLETE**, with EP-15 UNVERIFIED and PARTIAL items    |
| Execution path                | **READY** for controlled arms A/B, including concurrency |

**What changed since §8:** only the Workstream B intake status. No candidate was promoted,
no case was constructed, no adjudicator was named, no threshold or eligibility rule was
approved, and the corpus was not frozen.

## 10. Workstream A lead investigation and feasibility (current)

This section is later than §9 and records the lead investigation. It does not rewrite any
earlier count. It promotes nothing, constructs nothing, and approves nothing.

**Chronology (continued):**

6. Lead investigation and preparatory extraction (this revision, Workstream A).

### 10.1 Historical extraction result

All six leads in [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md) §2.2 are now
resolved against git history. The evidence is in
[HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md) and
[lead-extraction.json](lead-extraction.json).

| Lead | Result | Kind |
| --- | --- | --- |
| `IDR-L-001` | Clean pre-defect artifact extracted; the only later evidence is the artifact's own status line | Design defect, evidence-blocked |
| `IDR-L-002` | Resolves to an implementation-only fixed defect and an open residual GAP | Not a design defect |
| `IDR-L-003` | `PLAN-09…` @ `4fb3b8a3`; rebuild-scope acceptance criterion is unverifiable | Design defect, proposed strict |
| `IDR-L-004` | `REVIEW.md` @ `5fce2b6a`; two defects (review-scope contamination, missing malformed-output boundary) | Design defects, proposed strict |
| `IDR-L-005` | Backlog of production-readiness gaps; no single pre-defect design artifact | No artifact |
| `IDR-L-006` | End-to-end assessment and backlog; no single pre-defect design artifact | No artifact |

Two corrections are recorded, not applied:

- `IDR-E-005` points at `ba7736d4`, a post-fix commit that added the requirements it is
  cited as lacking. The exclusion stands as recorded; `5fce2b6a` is the correct pre-defect
  revision and is the basis for `IDR-L-004`.
- `IDR-L-002` does not become a design candidate because a later fix exists. The fixed
  defect is a missing redaction call in an outbound notification path that Plan 08 did not
  claim to cover.

### 10.2 Counts

The manifest is unchanged. The extraction is an increment, not a promotion.

| Measure | Recorded (§9) | After extraction (proposed, uncounted) |
| --- | --- | --- |
| Manifest entries | 23 | 23 |
| Strict proposed determinable | 0 | 2 plans / 3 defects (`IDR-L-003`, `IDR-L-004`) |
| Partially determinable | 10 | 10 |
| Verified-defect holds | 2 | 2 |
| Unverified holds | 3 | 3 |
| Excluded | 8 | 8 |
| Independently adjudicated determinable | 0 | **0** |
| Seeded / no-known-defect constructed | 0 / 0 | 0 / 0 |

The two proposed-strict plans and three defects are **not counted** toward any floor.
The corpus owner records the classification under decisions 2 and 3. A corpus-owner
determinability record is not an independent adjudication: adjudicator-confirmed
determinability remains **0**.

### 10.3 Does the seeded set count toward the floor?

The protocol text does not settle this, and this section does **not** resolve it.

- §5.3 lists a historical target of 12–15, a seeded set of about 8, and a no-known-defect
  set of about 6, then says: "Minimum to run: 15 determinable known defects … across at
  least 15 plans with known defects; **the no-known-defect set is additional and does not
  count toward this floor**." Naming only the no-known-defect set as additional admits a
  reading in which the **seeded** set *does* count.
- The preparation plan's gap table and [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §8 treat
  the seeded set as a **separate** floor. That is the conservative reading this package has
  applied.

Both arithmetics are reported below. Neither is used to declare the corpus ready, and no
threshold is changed. Which reading is in force was a decision for the corpus owner and
operator (decisions 2 and 3). **Resolved by Decision 3 (2026-11-21):** the conservative
reading is in force — the seeded set is additional and does not count toward either
historical minimum.

### 10.4 Scenario A — strict protocol

Only independently supported, strictly determinable defects count. Read as: the corpus
owner records a determinability judgment with a one-line basis for each defect.

| Case | Historical plans | Historical defects | + 8 seeded (if they count) | Shortfall vs 15/15 |
| --- | --- | --- | --- | --- |
| A1 — as recorded, nothing further recorded | 0 | 0 | 8 / 8 | 15 / 15 (seeded separate) or 7 / 7 |
| A2 — the two new proposals recorded strict | 2 | 3 | 10 / 11 | 13 / 12 (seeded separate) or 5 / 4 |

Under every cell, the historical strict corpus is far below 15 plans and 15 defects. The
repository cannot supply 15 strict historical design plans from its own defect history
(§4 and §10.1).

### 10.5 Scenario B — proposed amendment

[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md) is not approved. It lets a
plan count toward the plan floor only when it carries at least one **determinable** known
defect. It does not count a partial or hindsight-only defect toward either floor. Its effect
is therefore conditional on the corpus owner independently determining the defects.

| Case | Historical plans | Historical defects | + 8 seeded (if they count) | Shortfall vs 15/15 |
| --- | --- | --- | --- | --- |
| B1 — no partial candidate supported | 2 | 3 | 10 / 11 | 13 / 12 (seeded separate) or 5 / 4 |
| B2 — all 10 partial candidates independently determined and the two new proposals recorded | 12 | 13 | 20 / 21 | 3 / 2 (seeded separate) or **floor met in arithmetic only** |

The amendment's own §5 table reports 10 plans / 10 defects with a 5/5 shortfall; it
predates the two new leads, which add two plans and three defects. Even the best case is
short unless the seeded set counts, and the seeded set is **0 of about 8 constructed**, with
no designated seeder and no sealed manifest.

### 10.6 Remaining shortfall

| Floor | Scenario A (as recorded) | Scenario A2 (two proposals recorded) | Scenario B2 (amendment, all partials supported) |
| --- | --- | --- | --- |
| 15 plans with known defects, seeded separate | 15 | 13 | 3 |
| 15 determinable known defects, seeded separate | 15 | 12 | 2 |
| About 8 seeded-defect cases | 8 | 8 | 8 |
| About 6 no-known-defect cases | 6 | 6 | 6 |
| 15 contributing plans per arm | 15 | 13 | 3 |

If the seeded set counts, subtract 8 from each plan/defect shortfall, but the seeded set
is unbuilt, so the operational shortfall is unchanged today.

### 10.7 Required additional sources

- **Strict historical plans.** Scenario A2 needs 13 more plans with strictly determinable
  design defects and 12 more defects. The repository's remaining material is mostly
  implementation-level or interaction-level, and the investigation found only two plausible
  strict design artifacts in the six leads. New historical sources would have to come from
  pre-fix revisions of normative specs with a later correcting commit, or from follow-ups
  that name a pre-fix design artifact — the same test that excluded `IDR-L-002`,
  `IDR-L-005`, and `IDR-L-006`.
- **Partial determinations.** Scenario B2 needs the corpus owner to independently determine
  all 10 partial candidates and record the basis. Without that support the conservative
  reading applies and B2 collapses to B1.
- **Seeded and control cases.** Eight seeded and six control cases, constructed under
  [CASE-CONSTRUCTION-SPECIFICATION.md](CASE-CONSTRUCTION-SPECIFICATION.md), sealed outside
  the reviewer project, and independently checked.

### 10.8 Independent verification requirements

- The corpus owner records each determinability judgment and its one-line basis; the
  adjudicators do not select or freeze the corpus.
- Two independent human adjudicators, distinct from each other, from the seeder, from the
  corpus owner, and from the author of every case they adjudicate.
- A second adjudicator covers the larger of 20 findings or a random 20% sample; kappa is
  reported, and a PASS requires at least 20 double-adjudicated findings.
- The seeder is independent of the adjudicators and may not adjudicate a case they wrote.
- Each control has a recorded independent check; "no known defect" is not assumed.
- Each run's reviewer-visible tree is pinned, hashed, and free of later evidence, answer
  keys, and the sealed manifest.
- A model that could have seen the case, or that has project-directory access, is recorded
  as a contamination risk; keyword absence is not evidence of non-access.

### 10.9 Effect on statistical validity

- Below 15 determinable known defects or below 15 plans with known defects, **every recall
  comparison is INCONCLUSIVE** (assessment §6.1), before the recall rules are applied.
- Below 15 contributing plans per arm, that arm's reproducibility gate is **INCONCLUSIVE**
  (§5.5).
- A PASS additionally needs at least 20 double-adjudicated findings (§6.2); the current
  corpus would produce none.
- The plan is the unit of analysis and differences are paired by plan (§5.7). With about 15
  defects the 90% cluster-bootstrap intervals are expected to be wide and to drive an
  INCONCLUSIVE outcome; at 2, 12, or 13 plans they are wider still.
- The H1 PASS rule needs at least 25% relative recall gain, at least 3 more confirmed
  defects, a 90% interval lower bound above zero, and a median reproducibility of at least
  70% (§6.2). A 12-plan / 13-defect corpus cannot be assumed to support that claim.
- A seeded stratum that is reported separately cannot rescue a historical recall
  comparison that is below its floor; if the seeded set counts, the operator must record
  that reading before the freeze, not after seeing data.

### 10.10 Verdict (unchanged)

| Measure | Verdict |
| --- | --- |
| Corpus | **NOT FROZEN** |
| Corpus-owner freeze readiness | **NOT READY** |
| Historical strict floor | **NOT MET** (0 recorded; 2 plans / 3 defects proposed) |
| Historical partial-inclusive floor | **NOT MET** (10 / 15, or 12 / 15 under the amendment best case) |
| Seeded / no-known-defect | **NOT CONSTRUCTED** (0 / 8, 0 / 6) |
| Independent seeder | **NOT DESIGNATED** |
| Two independent human adjudicators | **NOT IDENTIFIED**; adjudicator-confirmed determinability is 0 |
| Full experiment | **NOT YET READY** |
| H1/H2 outcomes | **NOT ESTABLISHED** |

**What changed since §9:** the six historical leads are resolved, two proposed-strict design
candidates are extracted and uncounted, and the case-construction and decision-record
scaffolding is prepared. No candidate was promoted, no case was constructed, no answer key
exists, no adjudicator was named, no threshold or eligibility rule was approved, and the
corpus was not frozen. The floors are not met and were not lowered.

## 11. Post-#587 historical expansion (current)

This section is later than §10 and records two additional historical leads. It does not
rewrite any earlier count. It promotes nothing, constructs nothing, freezes nothing, and
approves nothing.

**Chronology (continued):**

7. PR #587 merged (`ebe26a11`): six leads resolved, case-construction and decision-record
   scaffolding prepared.
8. This expansion: two additional leads found by tracing commits that corrected a normative
   specification.

### 11.1 Additional extraction

The evidence is in [HISTORICAL-LEAD-INVESTIGATION.md](HISTORICAL-LEAD-INVESTIGATION.md) §8 and
[lead-extraction.json](lead-extraction.json) `additional_leads`.

| Lead | Artifact @ revision | Result |
| --- | --- | --- |
| `IDR-L-007` | `docs/specs/PLAN-HISTORICALIZATION.md` @ `fcf53888` | The archive is keyed to `.agent-sdlc/archive/<plan-id>/` with an idempotency rule, but the plan id is a basename slug; same-named or re-activated plans share one slot and a later activation overwrites the earlier archive. Fixed by PR #322. **Proposed strict.** |
| `IDR-L-008` | `docs/specs/RAG-RETRIEVAL-CONTRACT.md` @ `c6ef5f56` | `document_hash` covers the whole document while the evidence unit is a chunk, so the returned evidence is not checkable. Fixed by PR #330. The contract explicitly chose whole-document projection, so the classification is a judgment. **Proposed partial.** |

Both artifacts are clean: they contain none of the fix vocabulary, and every permitted
reference was checked to exist at the pinned revision.

### 11.2 Counts, separated by state

| Measure | Manifest recorded | PR #587 proposals | This expansion | Combined, uncounted |
| --- | --- | --- | --- | --- |
| Strict proposed plans | 0 | 2 | 1 | 3 |
| Strict proposed defects | 0 | 3 | 1 | 4 |
| Partial proposed plans | 10 | 0 | 1 | 11 |
| Partial proposed defects | 10 | 0 | 1 | 11 |
| Verified-defect holds | 2 | — | — | 2 |
| Unverified holds | 3 | — | — | 3 |
| Excluded | 8 | 1 lead | 0 | 8 entries / 9 leads |
| Independently adjudicated | 0 | 0 | 0 | **0** |

The manifest is still 23 entries. The combined column is **proposed and uncounted**; the
corpus owner records each classification under decisions 2 and 3, and a corpus-owner record
is not an independent adjudication.

### 11.3 Scenario A — strict protocol

| Case | Strict plans | Strict defects | + 8 seeded (if they count) | Shortfall vs 15/15 |
| --- | --- | --- | --- | --- |
| A0 — as recorded | 0 | 0 | 8 / 8 | 15 / 15 (seeded separate) or 7 / 7 |
| A1 — PR #587 proposals recorded | 2 | 3 | 10 / 11 | 13 / 12 (seeded separate) or 5 / 4 |
| A2 — this expansion recorded | 3 | 4 | 11 / 12 | 12 / 11 (seeded separate) or 4 / 3 |

On the evidence reviewed so far, the repository cannot supply 15 strict historical design
plans from its own defect history. The strict shortfall under the conservative reading is
**12 plans and 11 defects**.

### 11.4 Scenario B — proposed amendment

[PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md) is not approved. It counts a
plan only when it carries at least one **determinable** known defect, and it counts only
individually determinable defects. Its effect is therefore conditional on the corpus owner
independently determining every partial candidate.

| Case | Plans | Defects | + 8 seeded (if they count) | Shortfall vs 15/15 |
| --- | --- | --- | --- | --- |
| B0 — PR #587, all 10 manifest partials determined and the two strict proposals recorded | 12 | 13 | 20 / 21 | 3 / 2 (seeded separate) or floor met in arithmetic |
| B1 — this expansion, all 11 partials determined and the three strict proposals recorded | 14 | 15 | 22 / 23 | 1 plan / 0 defects (seeded separate) or floor met in arithmetic |

B1 reaches the **defect** floor (15) and misses the **plan** floor by one under the
conservative reading. That is arithmetic, not readiness: it requires the unapproved
amendment, a corpus-owner determination for each of 11 partials, eight constructed seeded
cases, six controls, two independent adjudicators, an approved threshold and budget, a
freeze, and an EP-15 disposition. None of those exists.

The question of whether the seeded set counts toward the floor (§10.3) was unresolved when
this section was written; both readings are shown. **Decision 3 (2026-11-21) records the
conservative reading: the seeded set is additional.** No threshold is changed.

### 11.5 Workstream B dependencies on the freeze

The execution-hardening evidence merged as PR #588 (`e829e5d9`); its detailed results are in
[INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md](../INDEPENDENT-DESIGN-REVIEW-WORKSTREAM-B-HARDENING.md)
and are not restated here. The freeze-relevant dependencies are:

- **EP-15 remains UNVERIFIED.** Provider concurrency and parallel-batch latency need a
  real-provider run that the operator must authorize; otherwise C-arm latency is first
  measured in the full run, as the protocol already says.
- **EP-02 and EP-12 need an operator disposition.** Model identity is client-side plus
  proxy-observed request; a server-returned `model` field is self-reported, not attested.
  Reproducibility has no sampling control; repetition carries the variance.
- **EP-16 needs a real-provider run.** The inspection instrument (the tool audit sink) is
  verified; whether a real model inspects the repository is not.
- **Instruction boundary.** SOP appends its own prose output requirement after the
  experiment's request. The wrapper is identical across arms, so it is not a fairness
  defect, but the frozen request template must be written knowing it follows.
- **Environment.** An ambient shell that exports `SOP_*` variables fails the preflight by
  design; runs use an explicit environment. A project `.env` fails it outright.
- **Model class.** The `small` class carries a built-in cloud fallback candidate that an
  overlay cannot remove, so the experiment must not use it.
- **Raw reply bodies** are hashed, not kept; the operator may choose to retain them.

These are inputs to the freeze record and to decisions 7–9. They change no corpus count,
eligibility rule, or threshold.

### 11.6 Verdict (unchanged)

| Measure | Verdict |
| --- | --- |
| Corpus | **NOT FROZEN** |
| Corpus-owner freeze readiness | **NOT READY** |
| Historical strict floor | **NOT MET** (0 recorded; 3 plans / 4 defects proposed) |
| Historical partial-inclusive floor | **NOT MET** (10 / 15 recorded; 14 / 15 if the amendment is approved and every partial is determined) |
| Seeded / no-known-defect | **NOT CONSTRUCTED** (0 / 8, 0 / 6) |
| Independent seeder | **NOT DESIGNATED** |
| Two independent human adjudicators | **NOT IDENTIFIED**; adjudicator-confirmed determinability is 0 |
| Full experiment | **NOT YET READY** |
| H1/H2 outcomes | **NOT ESTABLISHED** |

**What changed since §10:** two additional historical design defects are extracted — one
proposed strict, one proposed partial — and the amended-best-case arithmetic now reaches the
defect floor and misses the plan floor by one. No candidate was promoted, no case was
constructed, no answer key exists, no adjudicator was named, no threshold or eligibility rule
was approved, and the corpus was not frozen. The floors are not met and were not lowered.

## 12. Decision 1 recorded (current)

**Decision 1 — Corpus owner: APPROVED 2026-11-21.** Tom Tran is designated as corpus owner
for the EGM corpus and is excluded from independent adjudication. The record is
[DECISION-RECORD-001-CORPUS-OWNER.md](DECISION-RECORD-001-CORPUS-OWNER.md).

This changes no candidate classification or count. The corpus remains **NOT FROZEN**, the
experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**. Decisions 2–9 remain open; the
freeze precondition that needed a named owner is now met, and the other preconditions are
unchanged.

## 13. Decision 2 recorded (current)

**Decision 2 — Historical inclusion and exclusion: APPROVED 2026-11-21.** The approved
dispositions are 3 proposed strict plans / 4 defects and 11 proposed partial plans / 11
defects (all include with review), 2 verified-defect holds, 3 unverified holds, 1
evidence-blocked hold, 8 confirmed exclusions, and 3 excluded leads. The record is
[DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md).

This changes no candidate classification in the manifest. Approved historical disposition,
proposed determinability, final eligibility, and independent adjudication remain distinct:
no candidate is promoted, and the adjudicated count is **0**. The corpus remains **NOT
FROZEN**, the experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**. Decisions 3–9
remain open; the freeze preconditions that needed an owner and an inclusion list are now met.

## 14. Decision 3 recorded (current)

**Decision 3 — Eligibility rule: APPROVED 2026-11-21.** The rule is `conservative`: partial
and hindsight-only defects count toward neither historical minimum, the recall denominator is
the frozen determinable defect set, and the seeded set is additional. The record is
[DECISION-RECORD-003-ELIGIBILITY.md](DECISION-RECORD-003-ELIGIBILITY.md).

The numeric minimums, arms, thresholds, and cost ceilings are unchanged, and the protocol
text is not amended. The proposed strict corpus remains 3 plans / 4 defects, short by 12
plans and 11 defects; the 11 partials remain outside the denominator; the adjudicated count
is 0 and no candidate is promoted. The corpus remains **NOT FROZEN**, the experiment **NOT YET
READY**, and H1/H2 **NOT ESTABLISHED**. Decision 4 remains open; freeze preconditions 1–3 are
met and 4–7 remain open.

## 15. Decision 4 recorded (current)

**Decision 4 — Proposed protocol amendment: REJECTED (unnecessary) 2026-11-21.** Decision 3
already records the conservative rule the amendment proposed, so the amendment would change
no count. The registered protocol is byte-identical (SHA-256 `abba919e…`) and is not amended.
The record is
[DECISION-RECORD-004-PROTOCOL-AMENDMENT.md](DECISION-RECORD-004-PROTOCOL-AMENDMENT.md).

The proposed strict corpus remains 3 plans / 4 defects and the proposed partial corpus 11
plans / 11 defects; the adjudicated count is 0 and no candidate is promoted. The corpus
remains **NOT FROZEN**, the experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**.
Decision 3 remains in force; Decisions 5–9 remain open. Freeze preconditions 1–4 are met and
5–7 remain open.

## 16. Decision 5 recorded (current)

**Decision 5 — Independent seeder: APPROVED 2026-11-21.** Tom Tran is designated as seeder,
and the same person is corpus owner. Protocol §5.4 permits that overlap; it is recorded as a
limitation, not as independence. The record is
[DECISION-RECORD-005-SEEDER.md](DECISION-RECORD-005-SEEDER.md).

**Seeder designated, but case construction is not ready.** No independent case checker is
designated, no sealed-manifest location is selected, no answer key exists, and
`case_construction_authorized` is **false**. Seeded and control counts remain 0 / 8 and 0 / 6.
The proposed strict corpus remains 3 plans / 4 defects and the proposed partial corpus 11
plans / 11 defects; no candidate is promoted and the adjudicated count is 0. The corpus
remains **NOT FROZEN**, the experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**.
Decisions 6–9 remain open. Freeze preconditions 1–4 are met; precondition 5 is partially met;
preconditions 6–7 remain open.

## 17. Decision 6 preparation (current)

**Decision 6 remains PENDING.** A recruitment and qualification package is prepared in
[DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md](DECISION-006-ADJUDICATOR-QUALIFICATION-PACKAGE.md).
It contains the protocol requirement matrix, a per-candidate checklist, two blank
qualification records, a role description, a copy-ready recruitment message, a
conflict-of-interest declaration, the blinding procedure, and the double-adjudication rules.

No adjudicator is designated, contacted, or assessed. The repository shows only Tom Tran as a
human identity; he is the corpus owner and seeder and cannot adjudicate. No qualified external
human is identified. The three procedural ambiguities are **APPROVED as clarifications**
(2026-11-21) in
[DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md](DECISION-006-PROCEDURAL-CLARIFICATIONS-APPROVAL.md);
the analysis is in
[DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md](DECISION-006-ADJUDICATION-PROCEDURE-CLARIFICATION.md).
Decision 6 adjudicator designation remains PENDING. Candidate discovery and the
operator-assisted plan are recorded in
[DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md](DECISION-006-ADJUDICATOR-RECRUITMENT-SHORTLIST.md):
no verified candidate is identified from this environment, and no name is invented. The package cannot be mistaken for authorization: the corpus remains
**NOT FROZEN**, the experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**. Decision 6
stays open, and Decisions 7–9 remain open.

## 18. Decision 7 readiness (current)

**Decision 7 — APPROVED 2026-11-21.** The thresholds and a 613-call ceiling are recorded in
[DECISION-RECORD-007-THRESHOLDS.md](DECISION-RECORD-007-THRESHOLDS.md); execution remains
prohibited. Threshold and cost readiness is assessed in
[DECISION-007-THRESHOLD-COST-READINESS.md](DECISION-007-THRESHOLD-COST-READINESS.md). The
registered thresholds, the 319–700 (no Arm D) and 499–1,309 (with Arm D) call ranges, the
arm/repetition model, and the implementation gaps are recorded there. A decision-ready
proposal (default 7-call, 613-call ceiling, EP-15 UNVERIFIED) is in
[DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md](DECISION-007-OPERATOR-APPROVAL-PROPOSAL.md), and the
approval is recorded in [DECISION-RECORD-007-THRESHOLDS.md](DECISION-RECORD-007-THRESHOLDS.md).

The cost-accounting and ceiling primitives pass their offline tests. The EGM
threshold/stopping-rule, kappa, cluster-bootstrap, reproducibility, and run-manifest
evaluators are **not implemented**; the analysis is manual and must be pre-registered. No
threshold or ceiling is approved, no run is authorized, and the corpus remains **NOT FROZEN**,
the experiment **NOT YET READY**, and H1/H2 **NOT ESTABLISHED**. Decisions 8–9 remain open.
