# Case Construction Specification — Seeded and No-Known-Defect Cases

**Type:** Construction specification (non-normative). It defines how the seeded and
no-known-defect cases would be built, sealed, and checked. It constructs no case, creates
no answer key, and designates no role holder.
**Baseline:** `origin/main` @ `5a94dfe1`.
**Protocol:** [EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../EGM-GOLDFISH-REVIEW-ASSESSMENT.md)
§5.3–§5.5, §6.4.
**Preparation plan:** [CORPUS-PREPARATION-PLAN.md](CORPUS-PREPARATION-PLAN.md) §3–§5.
**Freeze procedure:** [FREEZE-PROCEDURE.md](FREEZE-PROCEDURE.md) §3.4, §3.6.
**Decision package:** [OPERATOR-DECISION-PACKAGE.md](OPERATOR-DECISION-PACKAGE.md)
decisions 5 and 6.
**Readiness:** [READINESS.md](READINESS.md).

**Status:** zero seeded-defect cases and zero no-known-defect cases exist. This document
is the construction method only. It is not evidence that any case has been built.

## 1. Required counts and what counts

| Stratum         | Protocol target | Constructed | Independent check | Sealed |
| --------------- | --------------- | ----------- | ----------------- | ------ |
| Seeded defect   | About 8         | 0           | 0                 | 0      |
| No known defect | About 6         | 0           | 0                 | 0      |

A case counts only when it is constructed, independently checked, and recorded in the
freeze record (preparation plan §3, §4, §7). A draft, a template, or an unsealed file counts
as **0**. The seeded and no-known-defect strata are reported **separately** from the
historical stratum; whether a seeded case may count toward any floor is the unresolved
question below.

Whether a seeded defect may count toward the protocol's 15-determinable-defect floor is
**not settled by the protocol text**. §5.3 says the historical set is the recall corpus and
that the no-known-defect set is additional, but it does not say whether the seeded set is
part of the recall denominator. This specification does not resolve the question; it is
recorded as an operator question in
[READINESS.md](READINESS.md) §10.3. Until it is resolved, no seeded case is counted toward
any floor.

## 2. Roles and independence

| Role                         | Duty                                                                                                     | May not be                                                                  |
| ---------------------------- | -------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Corpus owner                 | Selects and freezes the corpus; records determinability; records the case manifest in the freeze record  | An adjudicator                                                              |
| Independent seeder           | Writes the seeded and control artifacts; plants the seeded defects; holds the sealed answer key          | An adjudicator of any case they wrote                                       |
| Independent case checker     | Re-reads each sealed case against its answer key and confirms determinability and the absence of leakage | The seeder of that case; an adjudicator of that case                        |
| Analyst                      | Holds the unblinding key; applies the within-run deduplication                                           | An adjudicator                                                              |
| First and second adjudicator | Label findings; re-adjudicate the sample                                                                 | The seeder, the corpus owner, or the author of a case they adjudicate       |

Independence constraints carried from protocol §5.4:

- A human adjudicates; a model may not be an adjudicator.
- The corpus owner, seeder, analyst, and operator may overlap with each other.
- None of them may be an adjudicator.
- The seeder of a defect may not adjudicate that defect.
- A second adjudicator must be independent of the first adjudicator and of the seeder.

The independent case checker is a **proposed control**, not one of the protocol's named
roles; the protocol names the corpus owner, seeder, analyst, and adjudicators. The checker
exists so that a case's determinability and no-leak result are confirmed by someone other
than its author.

The repository currently shows only the operator as a human Git identity, and the operator
is treated conservatively as an author or direct editor of the candidate plans. A case
written by the operator can therefore be adjudicated only by someone else. No person is
designated by this document.

## 3. Input selection

### 3.1 Seeded cases

- The base artifact is a **design document written for the case** — a plan, specification,
  or acceptance contract in the repository's own format. It is not a historical corpus file
  and is not a modified copy of one.
- It reads as a real design document with a goal, scope, non-goals, design, acceptance
  criteria, and risks. It is not a "find the bug" exercise and carries no marker text.
- Case selection spreads the five protocol classes and the three additional classes of the
  same kind. The protocol names: unstated assumption, nonexistent symbol, missing failure
  mode, unverifiable acceptance criterion, scope violation. The remaining three are chosen
  from the same family and recorded before construction.
- A case must not be recognizably derived from a historical candidate. A reviewer who has
  read this repository must not be able to map the case to a known historical defect.

### 3.2 No-known-defect cases

- A control is either (a) a real repository plan or specification at a pinned revision for
  which no defect was ever recorded, or (b) a purpose-written artifact with `seeded_defect:
none`. The choice is recorded per case so the corpus mix is transparent.
- A control is not "an unexamined document." It must be **checked** for the review question
  and the check recorded (see §5).
- "No known defect" is not "no defect." Findings on a control are adjudicated, never assumed
  invalid.

## 4. Defect insertion constraints

For each seeded case:

1. **One determinable defect.** The case states exactly one planted defect and its class.
   A distractor is not a second defect.
2. **Determinability.** The defect must be identifiable from the artifact and the permitted
   paths alone, without the sealed key, the case manifest, or any later evidence. The
   determinability rationale is written before the case is sealed.
3. **Expected finding.** Record the location (section, paragraph, or statement), a severity
   from `INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`, and the acceptable alternative
   statements. An alternative is accepted only when it identifies the same defect and a
   severity that changes the same implementation decision.
4. **No answer in the artifact.** No sentence states the defect, apologizes for it, or marks
   it. No comment, filename, commit message, or metadata names it.
5. **Plausible distractors.** Include content that is arguable but not the planted defect,
   so a reviewer must discriminate. A distractor must not be a second planted defect.
6. **Real format.** The artifact uses the repository's plan/spec structure and terminology,
   so the reviewer's difficulty is the design judgment, not format recognition.
7. **No invention.** The expected finding is a property of the constructed artifact. It is
   not invented after seeing a model's output and it is not back-filled from a historical
   report.

## 5. Control-case selection and the "no known defect" check

A control records the same fields as a seeded case plus:

- a statement of why the artifact is believed defect-free for the review question;
- the independent check used to reach that conclusion — for example a second reader who
  did not write the artifact, a specification-to-code trace, and a search of the repository
  for a later defect report, fix, erratum, or gap that names the artifact;
- the explicit distinction between "no defect found" and "no defect exists";
- the same sealing and access restrictions as a seeded case.

Accepted check evidence:

- a recorded reader who is not the seeder and not an adjudicator of that case;
- the exact commands or paths searched for a later defect record, with the result;
- for a real plan, the pinned revision and the absence of a later correction in git history.

A control whose check is "nobody has looked" is not a control.

## 6. Answer key and sealed manifest

The sealed manifest is one file held **outside the reviewer project directory**. It is never
copied into a reviewer project, a run directory that the reviewer can read, or the git
repository.

### 6.1 Answer key per case

```yaml
case:
  id: <stable sealed identifier, e.g. S-01>
  stratum: seeded # seeded | no_known_defect
  artifact_path: <path inside the exported reviewer tree>
  artifact_sha256: <sha256 of the exact bytes handed to the reviewer>
  exported_tree_sha256: <sha256 of the sorted manifest of paths and blob hashes>
  revision_pin: <synthetic commit, or real commit for a control>
  permitted_references:
    - path: <path>
      blob_sha256: <sha256>
  instruction_block_sha256: <sha256>
  seeded_defect: <class or "none">
  determinability_rationale: <one paragraph, written before sealing>
  expected_finding:
    location: <section/paragraph>
    severity: <INFO|LOW|MEDIUM|HIGH|CRITICAL>
    acceptable_alternatives:
      - location: <...>
        severity: <...>
  distractors: [<short description>]
  control_check: <for no_known_defect only; the independent check record>
  leak_checks:
    artifact_contains_answer: false
    permitted_reference_contains_answer: false
    later_evidence_in_tree: false
  authored_by: <role, not necessarily a name>
  sealed_sha256: <sha256 of this record>
```

### 6.2 Access and unblinding

- Access to the sealed manifest: the seeder, the analyst, and the corpus owner. Not the
  adjudicators, and not any reviewer.
- The adjudicators see findings with arm, repetition, model, and run ids removed, and with
  the case id replaced by a neutral plan id.
- The analyst holds the unblinding key and applies the within-run deduplication. The
  adjudicator never sees arm labels.
- The manifest hash is recorded in the freeze record; the manifest itself is not.

## 7. Revision pinning and artifact hashing

For every case and every permitted reference, record:

- the synthetic or real commit;
- the artifact blob SHA-256;
- the blob SHA-256 of every permitted reference;
- the hash of the exported reviewer tree — a SHA-256 over the sorted list of
  `path<TAB>blob_sha256` for every file in the exported tree;
- the archive SHA-256 when an archive is used;
- the instruction block text and its SHA-256.

The freeze record must let a third party rebuild the exact reviewer-visible input. A case
whose export cannot be rebuilt is not eligible.

## 8. Reviewer-visible boundary

For each run the freeze record defines:

- the exported tree at the pinned revision, with no later commit visible;
- the permitted reference paths that exist at that revision, and nothing else;
- the instruction block and its hash, and the delimiter between instruction and artifact;
- an explicit statement that later evidence, remediation plans, answer keys, and the sealed
  manifest are absent;
- the absence of `.env`, provider overrides, and unrelated repository history;
- one isolated project directory per run, containing only that run's request file;
- the provider, model class, and harness configuration, with the repository-access behavior
  recorded.

A full clone carries history. If later commits must be excluded, use an exported tree at
the pin or a shallow clone whose history ends at the pin, and record and test the method
before the first run.

## 9. Contamination prevention

| Risk                                                  | Control                                                                                                                                               |
| ----------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| Seeder also adjudicates                               | The seeder may not adjudicate any case they wrote.                                                                                                    |
| Answer key in the reviewer tree                       | The key lives outside the project; the exported tree is checked for the key hash and for the expected-finding text.                                   |
| Case recognizable as a historical candidate           | Cases are written for the case and reviewed against the historical manifest before sealing.                                                           |
| Model training data contains the case                 | Seeded cases are new text. A model whose training data postdates the case, or that has project-directory access, is recorded as a contamination risk. |
| ACP/agent project-directory access exposes the answer | The run's project directory contains only the request file; the harness repository-access behavior is recorded.                                       |
| Keyword absence treated as non-access                 | Absence of a search term in the output is not evidence that the model did not access the answer.                                                      |
| Control treated as defect-free by assumption          | A control has a recorded independent check; findings are adjudicated.                                                                                 |
| Post-hoc key editing                                  | The key hash is sealed before the first run; an edit is a new freeze and a new approval.                                                              |
| Stratum mixing inflates a historical floor            | Seeded and control results are reported separately; see §1.                                                                                           |

## 10. Construction checklist

A case may enter the freeze record only when every gate is recorded:

1. Case id assigned and sealed.
2. Stratum and class recorded.
3. Artifact written in the repository format, no markers.
4. Exactly one planted defect, or a control check record.
5. Determinability rationale written before sealing.
6. Expected finding with location, severity, and acceptable alternatives.
7. Distractors recorded.
8. Leakage checks pass: the artifact and permitted references do not contain the answer;
   no later evidence is in the exported tree.
9. Permitted references exist at the pin and their hashes match.
10. Exported tree hash and archive hash recorded.
11. Independent case checker signs the determinability and the no-leak result.
12. Sealed manifest hash recorded outside the reviewer project.
13. Access list recorded; adjudicators excluded.
14. Freeze record updated.

## 11. What this document does not do

- It does not construct a seeded or no-known-defect case.
- It does not create, write, or expose an answer key.
- It does not designate a seeder, checker, analyst, or adjudicator.
- It does not decide whether seeded cases count toward the 15-defect floor.
- It does not freeze the corpus or authorize a run.

Construction may begin only after decision 5 (independent seeder) is recorded and the
operator authorizes case construction. Until then the counts stay **0 of about 8** and
**0 of about 6**.
