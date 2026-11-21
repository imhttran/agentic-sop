# Freeze Procedure — Independent Design Review Experiment

**Type:** Procedure (non-normative). It defines how a freeze would be recorded. It has not
been executed. The corpus is **NOT FROZEN**, and the freeze-readiness assessment in §8
records **NOT READY**.
**Baseline:** `origin/main` @ `a10edc28`, the merge of PR #574 (Workstream B). Earlier
baselines quoted below are preserved as recorded by the source documents.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json).
**Readiness:** [READINESS.md](READINESS.md).
**Decisions:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).
**Preparation:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).
**Workstream B reconciliation:** [WORKSTREAM-B-RECONCILIATION.md](WORKSTREAM-B-RECONCILIATION.md).

## 1. Purpose

A freeze converts a proposed corpus and a proposed instruction set into a fixed,
reproducible experiment input. After the freeze, no input may change silently. A run that
uses a different input is a different experiment and must be reported as such.

A freeze is **not** an approval of the conditional PRD, and it does not authorize
inference.

## 2. Preconditions

The freeze cannot be executed until:

1. The corpus owner is designated (decision 1). **MET 2026-11-21 — Tom Tran; see
   [DECISION-RECORD-001](DECISION-RECORD-001-CORPUS-OWNER.md).**
2. The inclusion and exclusion list is confirmed (decision 2). **MET 2026-11-21 — see
   [DECISION-RECORD-002](DECISION-RECORD-002-HISTORICAL-DISPOSITIONS.md).**
3. The eligibility rule is recorded (decision 3). **MET 2026-11-21 — conservative; see
   [DECISION-RECORD-003](DECISION-RECORD-003-ELIGIBILITY.md).**
4. The amendment decision is recorded (decision 4).
5. The seeder is designated (decision 5).
6. The two human adjudicators are designated and their authorship checks are recorded
   (decision 6).
7. The thresholds and cost ceiling are approved (decision 7).

Preconditions 1–3 are met (owner, inclusion list, and eligibility rule recorded 2026-11-21).
Preconditions 4–7 remain open.

The operator's freeze authorization is decision 8. Execution authorization is decision 9
and is separate.

## 3. Freeze record

The freeze record must contain, at minimum:

### 3.1 Candidate IDs and eligibility

- Every candidate ID included in the corpus.
- Each candidate's evidence state and determinability classification.
- The exclusion list, with reasons.
- The held list, with reasons.
- The eligibility rule in force.

### 3.2 Git revisions and artifact hashes

For every artifact and permitted reference:

- commit SHA;
- blob SHA-256;
- exported tree hash, when an export is used;
- archive SHA-256, when an archive is used.

### 3.3 Reviewer-visible input sets

For each run:

- the artifact path and revision;
- the permitted reference paths that exist at that revision;
- the instruction block and its SHA-256;
- an explicit statement that later evidence, remediation plans, answer keys, and sealed
  manifests are absent;
- the method that excludes later git history, if a full clone would expose it.

### 3.4 Seeded-case manifests and access restrictions

- Seeded case IDs and their sealed answer-key hashes.
- No-known-defect case IDs and their independent-check records.
- The location of the sealed manifest, outside the reviewer project directory.
- The list of humans with access.
- The access-control method.

### 3.5 Model and provider configuration

- Provider, harness, and model class for each arm.
- The exact model identifiers and any provider-specific settings.
- The environment allowlist, and a statement that override variables are unset.
- The token-reporting expectation and the unreported-call handling rule.

### 3.6 Review instruction blocks

- The full text or a path to the immutable copy.
- The SHA-256 of each block.
- The delimiter between instruction and artifact.
- A statement that the blocks are frozen for the run.

### 3.7 Run order and randomization seed

- The arm and plan order.
- The randomization seed.
- The method used to generate the order.

### 3.8 Evidence output locations

- The run-directory path pattern.
- The retained files and their hashes.
- The location of the sealed key and the unblinding procedure.
- The retention period and access list.

### 3.9 Cost and latency measurement

- The call budget and the cost ceiling.
- The token-accounting method.
- The wall-clock measurement method and its scope.
- The rule for a provider that does not report usage.

### 3.10 Adjudication assignments

- The first and second adjudicator identities.
- Their independence and authorship checks.
- The sampling rule for the second adjudication.
- The conflict-of-interest record.

### 3.11 Explicit operator approval

- The approval text, identity, and date.
- The approved thresholds and budget.
- The statement that no threshold may change after the first run.

## 4. Procedure

1. **Assemble.** The corpus owner assembles the candidate set and the instruction blocks.
2. **Verify.** Recompute every hash and confirm every permitted reference exists at its
   pinned revision.
3. **Seal.** The seeder seals the seeded and no-known-defect manifests outside the reviewer
   directory.
4. **Review.** The operator reviews the freeze record and the decision package.
5. **Approve.** The operator records the freeze authorization.
6. **Record.** Commit the freeze record to the repository and record its SHA-256.
7. **Tag.** Create an annotated tag for the freeze commit. Do not move or delete the tag.
8. **Announce.** Record the freeze hash in the readiness report and the run manifest.
9. **Run.** Only after a separate execution authorization.

## 5. Post-freeze mutation controls

- The freeze record's SHA-256 is quoted in every run manifest.
- The run records the freeze commit and verifies it before the first call.
- A changed artifact, instruction block, or configuration is a new experiment.
- The repository tag is immutable in practice; a moved tag is a protocol violation.
- The sealed manifest is never copied into a reviewer project directory.
- Any post-freeze correction requires a new freeze record and a new operator approval.

## 6. Record template

The following is a template, not a filled record. Values are placeholders.

```yaml
freeze_record:
  version: 1
  status: PROPOSED            # PROPOSED | FROZEN | SUPERSEDED
  baseline: <origin/main sha>
  protocol: docs/reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md
  manifest: docs/reports/independent-design-review-corpus/candidates.json
  manifest_sha256: <sha256>
  eligibility_rule: conservative   # conservative | amended
  operator_approval:
    approved: false
    identity: <name>
    date: null
    thresholds_approved: false
    budget_approved: false
  corpus_owner: null
  seeder: null
  adjudicators:
    first: null
    second: null
  candidates:
    - id: <IDR-...>
      artifact: <path>
      commit: <sha>
      artifact_sha256: <sha256>
      permitted_references: [<path>...]
      classification: <strict | partial | held | excluded>
  seeded_cases:
    count: 0
    sealed_manifest_sha256: null
    access_list: []
  no_known_defect_cases:
    count: 0
    sealed_manifest_sha256: null
    access_list: []
  instructions:
    arm_a_sha256: null
    arm_b_sha256: null
  configuration:
    providers: []
    models: []
    environment_allowlist: []
    override_variables_unset: false
  run_order:
    seed: null
    order: []
  evidence:
    output_path_pattern: null
    retention: null
  measurement:
    call_budget: null
    cost_ceiling: null
    latency_method: null
  freeze:
    commit: null
    tag: null
    record_sha256: null
```

## 7. What the freeze does not do

- It does not approve the conditional PRD.
- It does not authorize inference.
- It does not designate the corpus owner, seeder, or adjudicators.
- It does not change a protocol threshold.
- It does not convert a partial candidate into a determinable one.

## 8. Freeze-readiness assessment (current)

**Verdict: NOT READY TO FREEZE.** This assessment evaluates the current corpus against the
authoritative protocol at `origin/main` @ `a10edc28`. It promotes no candidate, constructs
no case, approves no amendment, and executes no freeze.

| #   | Readiness item                                | Evidence                                                                                                                                                  | Status                                                                                    |
| --- | --------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| 1   | Strict-determinable candidate count           | 0 strict proposed determinable; floor is 15 plans and 15 determinable defects                                                                             | **NOT MET** (15/15 shortfall)                                                             |
| 2   | Proposed partially determinable count         | 10 proposed partial (`IDR-H-003`, `IDR-H-004`, `IDR-H-006`, `IDR-E-001`–`IDR-E-004`, `IDR-E-007`–`IDR-E-009`); no independent adjudication                | **PROPOSED, NOT COUNTED**                                                                 |
| 3   | Held and excluded candidates                  | 2 verified-defect holds (`IDR-H-001`, `IDR-H-005`), 3 unverified holds (`IDR-E-006`, `IDR-E-010`, `IDR-E-012`), 8 excluded                                | **NOT MET**                                                                               |
| 4   | Historical corpus shortfall                   | 0/15 strict; 10/15 partial-inclusive; shortfall 15/15 or 5/5                                                                                              | **NOT MET**                                                                               |
| 5   | Seeded-case construction status               | 0 of about 8 constructed; method only                                                                                                                     | **NOT MET**                                                                               |
| 6   | No-known-defect control status                | 0 of about 6 constructed; method only                                                                                                                     | **NOT MET**                                                                               |
| 7   | Independent seeder availability               | None designated; no sealed manifest                                                                                                                       | **NOT MET**                                                                               |
| 8   | Independent human adjudicator availability    | None identified; operator treated as an author; PASS unreachable                                                                                          | **NOT MET**                                                                               |
| 9   | Reviewer-input boundaries                     | Protocol §5.1/§5.2 fixed; 17 artifacts hashed and 17 permitted-reference lists checked; no leakage overlap; held/excluded answer-bearing entries kept out | **PARTIAL** — defined for historical artifacts; no seeded or no-known-defect inputs exist |
| 10  | Sealed answer-key separation                  | Procedure and access-list fields defined in §3.4; no sealed manifest exists                                                                               | **NOT MET**                                                                               |
| 11  | Revision and artifact integrity               | All 17 recorded artifacts resolve and match SHA-256; every permitted reference exists; 6 entries have no artifact and are not counted                     | **MET for recorded artifacts**; the set is not frozen                                     |
| 12  | Workstream B dependencies                     | PR #574 merged: EP-01/03/04/05/06/07/08/11/13/14 VERIFIED; EP-02/09/10/12/16 PARTIAL; EP-15 UNVERIFIED                                                    | **PARTIAL**                                                                               |
| 13  | Model configuration and instruction readiness | `models.<class>` overlay requirement and EP-13 route verified; instruction blocks draft, not frozen; config overlay sha256 recorded for the pilots only   | **PARTIAL**                                                                               |
| 14  | Cost limits and execution authorization       | Thresholds and budget pre-registered in the assessment; none approved; no execution authorization                                                         | **NOT MET**                                                                               |

**Why the verdict is NOT READY.** The corpus fails both floors under either reading, the
seeded and no-known-defect controls do not exist, no independent seeder or two independent
adjudicators are designated, the eligibility amendment is unapproved, the instruction blocks
are not frozen, no threshold or cost ceiling is approved, and the Workstream B evidence has
one UNVERIFIED item and several PARTIAL items that the freeze record must either resolve or
record as limitations. The two `MET`/`PARTIAL` integrity items show that the recorded
evidence is sound enough to review; they do not make the corpus sufficient.

**What this assessment does not do.** It does not promote a held candidate, convert a
partial candidate into a determinable one, construct a seeded or no-known-defect case,
approve the eligibility amendment, approve a threshold, designate a role holder, or write a
freeze record. The freeze record template in §6 remains unfilled, and the preconditions in
§2 remain open.
