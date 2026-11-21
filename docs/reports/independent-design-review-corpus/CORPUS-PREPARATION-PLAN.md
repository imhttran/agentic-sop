# Corpus Preparation Plan — Independent Design Review

**Type:** Preparation plan (non-normative). It describes how the remaining corpus would be
built and verified. It constructs nothing, freezes nothing, and authorizes nothing.
**Baseline:** `origin/main` @ `ade6864a`, plus the restored pilot evidence commit.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json).
**Readiness:** [READINESS.md](READINESS.md).
**Verification:** [CANDIDATE-VERIFICATION.md](CANDIDATE-VERIFICATION.md).

## 1. Current gap

The protocol requires **15 plans with known defects**, **15 determinable known defects**,
**about 8 seeded-defect cases**, **about 6 no-known-defect cases**, and **15 contributing
plans per arm**. It also requires at least 20 double-adjudicated findings for a PASS.

| Requirement | Available | Gap |
| --- | --- | --- |
| Plans with known, determinable defects (strict) | 0 | 15 |
| Plans with known defects, partial-inclusive upper bound | 10 | 5 |
| Determinable known defects (strict) | 0 | 15 |
| Seeded-defect cases | 0 | about 8 |
| No-known-defect cases | 0 | about 6 |
| Independent human adjudicators | 0 | 2 |
| Contributing plans per arm (strict) | 0 | 15 |

Unconstructed cases count as **0**. Nothing in this plan changes that until a case is
built, sealed, independently checked, and frozen.

## 2. Historical expansion

### 2.1 Extraction procedure

For each lead:

1. Identify the artifact that existed **before** the defect was discovered.
2. Record its commit, path, and SHA-256.
3. Read it without the later defect report, fix commit, or remediation plan.
4. Record the permitted repository paths that existed at that commit.
5. Decide whether the defect is discoverable from the artifact and those paths.
6. Record the result as `VERIFIED`, `PROPOSED`, `UNVERIFIED`, or `EXCLUDED` in the manifest.
7. Do not count the candidate until the corpus owner records the classification.

### 2.2 Lead backlog (not counted)

These are evidence-backed leads, not accepted candidates. Each requires extraction and a
corpus-owner decision before it can appear as an eligible input.

| Lead | Artifact to examine | Revision or evidence | What is missing |
| --- | --- | --- | --- |
| `IDR-L-001` | `docs/plans/PLAN-Code-Retrieval-Context-Seam.md` | Plan at `21564b27`, before the post-merge status line at `03a7d7d9` | Confirm that the implement/fix wiring gap is visible from the plan and the referenced `internal/cli/run.go`, `internal/cli/retrieve.go` at `21564b27`. The existing `IDR-H-002` revision is self-referential and does not count. |
| `IDR-L-002` | `docs/history/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md` | Pre-follow-up revisions at `4fb3b8a3`, `462ed9bb`, or `e84eb4aa`; defect later confirmed at `aa3e1d36` | Select the revision before the hidden-context confirmation and record permitted paths. `IDR-H-007` is the follow-up that contains the answer and remains excluded. |
| `IDR-L-003` | `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md` | Pre-follow-up revisions at `4fb3b8a3`, `536057ab`, or `ca4c800e`; follow-up archived at `05685308` | Select the revision before the retrieval-boundary follow-up and record permitted paths. `IDR-H-008` is the follow-up that contains the answer and remains excluded. |
| `IDR-L-004` | Review-governance design artifact | Residual findings recorded at `docs/reports/review-hardening/RSH-005-verification.md` and `RSH-001-review-seam-baseline.md` | Identify the pre-fix design artifact. `IDR-E-005` is excluded because the pre-fix `REVIEW.md` already contained the correct behavior; a code or contract artifact may be required instead. |
| `IDR-L-005` | Post-Wave-3 readiness artifact | `docs/history/plans/PLAN-Post-Wave3-Production-Readiness.md` | Identify a single pre-fix artifact. `IDR-E-011` currently has none. |
| `IDR-L-006` | End-to-end reliability artifact | `docs/history/plans/PLAN-SOP-End-to-End-Reliability.md` | Identify the pre-fix artifact. `IDR-E-012` currently has none. |

Additional leads should be taken from plans or specifications that have a later defect
report or fix commit, and from OWASP LLM follow-ups that name a pre-fix artifact. The
corpus owner must reject a lead whose only evidence is the artifact's own status line.

### 2.3 What will not be accepted

- A remediation plan that states the defect.
- A follow-up plan that records the answer.
- A document whose only defect evidence is its own later status line.
- A revision chosen only because it is before a merge, when the artifact already contains
  the fix or the disclosure.
- A "lead" counted as a plan before extraction and classification.

## 3. Seeded-defect construction

Approximately eight cases. None is constructed here.

Each case must record:

| Field | Requirement |
| --- | --- |
| Case ID | Stable, sealed identifier. |
| Base artifact | A design document written for the case, not a historical corpus file. |
| Seeded defect | One determinable defect with a stated class. |
| Determinability rationale | Why a reviewer could find it from the artifact and permitted paths. |
| Expected finding | Location, severity, and acceptable alternative statements. |
| Distractor content | Content that is plausible but not the seeded defect. |
| Answer key SHA-256 | Hash of the sealed key, recorded outside the reviewer directory. |
| Access restriction | Paths that the reviewer must not see. |
| Revision pin | Commit or content hash for the artifact and every permitted reference. |

Construction rules:

- The seeder is a designated human independent of the adjudicators.
- Seeded cases are written as design artifacts, not as "find the bug" exercises.
- The case set is not derived from the historical candidates in a way that lets a reviewer
  recognize the historical answer.
- The answer key and the case manifest are sealed and held outside the reviewer project.
- No case counts toward the eight until it is constructed, independently checked, and
  recorded in the freeze record.
- No finding is invented. The expected finding is a property of the constructed artifact.

## 4. No-known-defect construction

Approximately six cases. None is constructed here.

Each case must record the same fields as a seeded case, with `seeded_defect: none` and:

- a statement of why the artifact is believed defect-free for the review question;
- the independent check used to reach that conclusion;
- a distinction between "no defect found" and "no defect exists";
- the same sealing and access restrictions.

No-known-defect cases exist to measure false positives. A case that is merely unexamined
is not a no-known-defect case.

## 5. Reviewer-visible input boundary

The freeze record must define, per run:

- The artifact and the permitted repository paths, pinned to a commit.
- The instruction block and its hash.
- The absence of later evidence, remediation plans, answer keys, and sealed manifests.
- The absence of `.env`, provider overrides, and unrelated repository history.
- One isolated project directory per run.
- The provider, model class, and harness configuration.

A repository clone carries history. If the protocol's review input must exclude later
commits, use an exported tree at the pinned revision or a shallow clone whose history ends
at that revision. The method must be recorded and tested before the first run.

## 6. Reproducible revision pinning

For every artifact and permitted reference, record:

- the commit SHA;
- the blob SHA-256;
- the tree hash of the exported input;
- the archive SHA-256, when an export is used;
- the pinned commit used to build the run directory.

The freeze record must allow a third party to rebuild the exact reviewer-visible input.

## 7. Counts after construction

The readiness report counts only constructed, independently checked, and frozen cases.
Until then, the seeded and no-known-defect counts remain **0 of about 8** and **0 of about
6**, and the historical counts remain **0 strict, 10 partial, 2 held**.

## 8. Non-goals

This plan does not:

- create seeded or no-known-defect results;
- promote a historical lead to eligible status;
- freeze the corpus;
- approve the protocol amendment;
- designate a seeder or adjudicators;
- authorize inference or a full experiment run.
