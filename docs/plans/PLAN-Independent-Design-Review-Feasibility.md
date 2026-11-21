# PLAN — Independent Design Review Experiment Feasibility

**Status:** DRAFT — coordination record. Workstream A reviewed; Workstream B pending;
not activated; no experiment or pilot inference authorized.
**Baseline:** `origin/main` @ `64101f9e`.
**Authoritative protocol:**
[EGM-GOLDFISH-REVIEW-ASSESSMENT.md](../reports/EGM-GOLDFISH-REVIEW-ASSESSMENT.md).
**Conditional contract:**
[PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md](../requirements/PRD-SOP-INDEPENDENT-DESIGN-REVIEW.md).

## Objective

Coordinate the feasibility assessment for SOP's proposed independent design-review
experiment. This plan owns the corpus, governance, adjudication, report integration and
pull request. A separate Claude Code agent on Linux owns the provider and execution
investigation. This plan does not execute the experiment.

## Ownership and boundaries

- **Workstream A — corpus and governance (SOP-owned).** Historical defect inventory,
  original revisions and evidence, determinability, seeded and no-known-defect
  candidates, adjudication feasibility, and statistical prerequisites.
- **Workstream B — provider and execution (Claude Code-owned).** Review prompt
  composition, routing and environment precedence, served-model verification, token
  accounting, parallel execution and latency, and four-call pilot readiness.
- **Workstream C — integration (SOP-owned).** The feasibility report, validation, and PR.

Claude Code owns the technical investigation, not SOP lifecycle transitions. Workstream
B is not complete until its evidence is received and independently checked.

## Non-goals

- No experiment run, pilot inference, Arm D, or production implementation.
- No change to production code, configuration, lifecycle gates, approvals, or
  experiment thresholds.
- No installation of EGM, Superpowers, or OpenSpec.
- No claim that the experiment PASSes, FAILs, or is INCONCLUSIVE.
- No merge.

## Tasks

### FEAS-A1 — Protocol baseline and candidate inventory

Record the authoritative protocol baseline and inventory candidate historical plans,
specifications, and requirements. For each candidate record the path, revision, original
context, defect or gap, evidence pointer, proposed determinability, and author or
adjudication conflict.

**Acceptance criteria**

- The baseline is `64101f9e`, and the assessment and PRD are linked.
- Candidate paths and last-modified revisions are recorded.
- No later evidence is treated as available to the original reviewer.
- No inference is executed.

**State:** Reviewed. The inventory is in
[the feasibility report](../reports/INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md)
§2.

### FEAS-A2 — Determinability evidence

Classify each candidate as determinable, partially determinable, or not determinable from
the document and its parent revision. Distinguish a verified later defect from a proposed
determinability judgment. Do not count an independent adjudication that has not happened.

**Acceptance criteria**

- Every determinability classification is labelled proposed or verified.
- Evidence pointers resolve to a path, commit, PR, or report.
- A remediation plan is not used as the corpus document when it would leak the answer.

**State:** Reviewed. One candidate document has independent later evidence and a proposed
determinable classification; seven are partially determinable or self-referential. No
independent adjudicator has confirmed any classification.

### FEAS-A3 — Corpus sizing and seeded/no-known-defect plan

Compare the candidate count with the assessment's target of 26–29 plans and its floors of
15 plans with known defects and 15 determinable known defects. Record the seeded and
no-known-defect construction requirements without creating fabricated results.

**Acceptance criteria**

- The report states the verified, proposed, and missing counts separately.
- Seeded defects have an independent seeder and a sealed manifest requirement.
- No seeded defect or result is invented.

**State:** Not met. The seeded and no-known-defect corpus is not constructed, and the
verified historical corpus is below the assessment floor.

### FEAS-A4 — Adjudication feasibility

Determine whether two independent human adjudicators are available. Record the five role
requirements, the definition of “author,” and any conflicts. Do not designate an AI agent
as a human adjudicator.

**Acceptance criteria**

- The report distinguishes confirmed availability from unconfirmed availability.
- The operator is not assumed to be an adjudicator.
- No approval or adjudication record is invented.

**State:** Blocked. Repository history shows only the operator as a human author; the
operator authored or edited the candidate plans, so the operator cannot adjudicate them.
No second independent human adjudicator is identified.

### FEAS-A5 — Cost, risk, and pilot prerequisites

Summarize the assessment's call budget and the human-review burden. Record the risks that
would make the experiment INCONCLUSIVE and the prerequisites for the four-call pilot.

**Acceptance criteria**

- Cost figures are attributed to the assessment, not independently measured.
- The report separates inference cost from human-review cost.
- The pilot is prepared but not executed.

**State:** Reviewed. The report records the budget, the dominant INCONCLUSIVE risks, and
the unexecuted pilot prerequisites.

### FEAS-B1 — Linux provider/execution evidence intake

Receive the Linux agent's read-only report and check it against the repository
implementation and the assessment. Record agreement, disagreement, and missing evidence.

**Acceptance criteria**

- Every technical claim has a repository or measured-evidence pointer.
- A disagreement is resolved with repository evidence, not assumption.
- Workstream B remains pending when the report is unavailable.

**State:** Pending. No Linux report file, branch, or PR was available during this
inventory.

### FEAS-C1 — Integrated feasibility report

Produce `docs/reports/INDEPENDENT-DESIGN-REVIEW-EXPERIMENT-FEASIBILITY.md` with the nine
required sections and a verdict of READY, NOT YET READY, or BLOCKED.

**Acceptance criteria**

- Verified evidence is distinguished from proposed or unverified information.
- The feasibility verdict is not presented as the experiment outcome.
- The report links the assessment and PRD.

**State:** Complete with unverified sections. The report records Workstream B as pending
and §§4, 7 and 8 as unverified or awaiting operator decisions.

### FEAS-C2 — Validation and pull request

Run the documentation and SOP review gates, commit the plan and report, and open one PR
against `main`. Do not merge.

**Acceptance criteria**

- Documentation links resolve.
- The applicable SOP validation and review gates are recorded.
- One PR is open; no merge is performed.

**State:** In progress.

## Dependencies and status

These are coordination statuses in this record, not SOP lifecycle transitions; no plan
was activated.

| Task    | Owner       | State                             | Evidence                 |
| ------- | ----------- | --------------------------------- | ------------------------ |
| FEAS-A1 | SOP         | Reviewed                          | Report §2                |
| FEAS-A2 | SOP         | Reviewed with limitations         | Report §2                |
| FEAS-A3 | SOP         | Not met                           | Report §2                |
| FEAS-A4 | SOP         | Blocked                           | Report §3                |
| FEAS-A5 | SOP         | Reviewed                          | Report §§5–7             |
| FEAS-B1 | Claude Code | Pending                           | Report §4                |
| FEAS-C1 | SOP         | Complete with unverified sections | Report; §4/§7/§8 pending |
| FEAS-C2 | SOP         | In progress                       | PR pending               |

## Read-only guarantee

This plan changes documentation only. It does not execute inference for the experiment,
does not run the pilot, does not change a threshold, and does not alter production code,
configuration, lifecycle gates, or approvals. Its task states are coordination statuses in
this record, not SOP lifecycle transitions.
