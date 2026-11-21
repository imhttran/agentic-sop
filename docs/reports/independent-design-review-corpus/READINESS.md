# Independent Design Review Corpus — Readiness Summary

**Type:** Corpus evidence summary (non-normative). It records the Workstream A candidate
corpus, its evidence states, and whether it is ready for a corpus-owner freeze. It is not
an experiment result and does not authorize the conditional PRD.
**Baseline:** `origin/main` @ `1b61db6427e73ab18cab532834504359ea41de04`.
**Manifest:** [candidates.json](candidates.json).
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Feasibility report:** [INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md](../INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md).
**Plan of record:** [PLAN-Independent-Design-Review-Feasibility.md](../../plans/PLAN-Independent-Design-Review-Feasibility.md).
**Freeze status:** **NOT FROZEN.**

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
| ---------------------------------------- | ----- |
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
| Pending corpus-owner review              | 2     |

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
sealed manifest before the run. This work is not started because:

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
adjudicator.

## 7. Outstanding operator decisions

1. **Classification amendment:** approve, reject, or revise the proposed eligibility rule
   in [PROPOSED-PROTOCOL-AMENDMENT.md](PROPOSED-PROTOCOL-AMENDMENT.md). It is not applied.
2. **Parent artifacts:** confirm the nine extracted pre-fix artifact references
   (`IDR-E-001`–`IDR-E-005`, `IDR-E-007`–`IDR-E-010`), note that `IDR-E-005` is excluded
   because its pre-fix spec already contained the correct behavior, and decide whether to
   extract the three missing parents (`IDR-E-006`, `IDR-E-011`, `IDR-E-012`).
3. **Corpus freeze:** authorize a corpus owner to freeze the eligible set. The corpus is
   **not frozen**.
4. **Exclusions:** confirm the eight excluded candidates, especially the review-scope
   plan, which points at a pre-fix spec that already contained the correct behavior.
5. **Seeded and no-known-defect cases:** designate a seeder and authorize construction of
   the sealed manifest and the about-eight/about-six cases.
6. **Adjudicators:** identify two independent human adjudicators and record the authorship
   evidence for each.
7. **Target size:** decide whether to proceed with the 10 proposed eligible plans or
   expand the historical corpus before freeze. The strict shortfall is 15 plans and 15
   determinable defects; the partial-inclusive shortfall is 5 and 5.
8. **Held candidates:** decide whether `IDR-H-001` (ACP-ZED) and `IDR-H-005` (LLM02) can
   enter the corpus at all. Both have verified defect evidence but no uncontaminated
   pre-defect artifact in git, so they are held pending corpus-owner review.

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
- Workstream B remains pending.

Workstream A can proceed to a corpus-owner review of the manifest. It must not label the
corpus `FROZEN` without the operator's decision and the corpus owner's recorded freeze.
