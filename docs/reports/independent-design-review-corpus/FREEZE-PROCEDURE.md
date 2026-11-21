# Freeze Procedure — Independent Design Review Experiment

**Type:** Procedure (non-normative). It defines how a freeze would be recorded. It has not
been executed. The corpus is **NOT FROZEN**.
**Baseline:** `origin/main` @ `ade6864a`, plus the restored pilot evidence commit.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Manifest:** [candidates.json](candidates.json).
**Readiness:** [READINESS.md](READINESS.md).
**Decisions:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md).
**Preparation:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md).

## 1. Purpose

A freeze converts a proposed corpus and a proposed instruction set into a fixed,
reproducible experiment input. After the freeze, no input may change silently. A run that
uses a different input is a different experiment and must be reported as such.

A freeze is **not** an approval of the conditional PRD, and it does not authorize
inference.

## 2. Preconditions

The freeze cannot be executed until:

1. The corpus owner is designated (decision 1).
2. The inclusion and exclusion list is confirmed (decision 2).
3. The eligibility rule is recorded (decision 3).
4. The amendment decision is recorded (decision 4).
5. The seeder is designated (decision 5).
6. The two human adjudicators are designated and their authorship checks are recorded
   (decision 6).
7. The thresholds and cost ceiling are approved (decision 7).

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
