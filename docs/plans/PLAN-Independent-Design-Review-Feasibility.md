# PLAN — Independent Design Review Experiment Feasibility

**Status:** DRAFT — coordination record. Workstream A reviewed; Workstream B pending;
not activated; no experiment or pilot inference authorized.
**Baseline:** `origin/main` @ `ade6864a`.
**Baseline note:** advances from `64101f9e` through `1b61db64` to `ade6864a` to include
PRs #567–#571 and the readiness package. Candidate revisions remain pinned to the
commits recorded in `candidates.json`.
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
  composition, routing and environment precedence (including `.env`), requested-model
  verification and provider-side identity where available, token accounting, parallel
  execution and latency, and four-call pilot readiness.
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

- The baseline is `1b61db64`, and the assessment and PRD are linked.
- Candidate paths and last-modified revisions are recorded.
- No later evidence is treated as available to the original reviewer.
- No inference is executed.

**State:** Reviewed. The candidate manifest is
[`docs/reports/independent-design-review-corpus/candidates.json`](../reports/independent-design-review-corpus/candidates.json)
with 23 entries: 5 `VERIFIED`, 7 `PROPOSED`, 3 `UNVERIFIED`, and 8 `EXCLUDED`.

### FEAS-A2 — Determinability evidence

Classify each candidate as determinable, partially determinable, or not determinable from
the document and its parent revision. Distinguish a verified later defect from a proposed
determinability judgment. Do not count an independent adjudication that has not happened.

**Acceptance criteria**

- Every determinability classification is labelled proposed or verified.
- Evidence pointers resolve to a path, commit, PR, or report.
- A remediation plan is not used as the corpus document when it would leak the answer.

**State:** Reviewed. Nine pre-fix artifact references were recorded (`E-001`–`E-005`,
`E-007`–`E-010`); `E-005` is excluded because the pre-fix spec already contained the
correct behavior, leaving seven proposed E partials; with the three verified H partials
the manifest records ten partial candidates. `E-006`, `E-011`, and `E-012` lack a single
pre-fix artifact. The manifest proposes 0 strict determinable defects, 10
partially determinable defects, 2 candidates held pending corpus-owner review, 3
unverified entries, and 8 exclusions. Co-author
trailers were examined: Tom Tran and several Claude identities, with no other human
co-author. No independent adjudicator has confirmed any classification.

### FEAS-A3 — Corpus sizing and seeded/no-known-defect plan

Compare the candidate count with the assessment's target of 26–29 plans and its floors of
15 plans with known defects and 15 determinable known defects. Record the seeded and
no-known-defect construction requirements without creating fabricated results.

**Acceptance criteria**

- The report states the verified, proposed, and missing counts separately.
- Seeded defects have an independent seeder and a sealed manifest requirement.
- No seeded defect or result is invented.

**State:** Not met. Ten plans are proposed eligible (0 strict and 10 partial). The strict
reading is 0 plans and 0 determinable defects against both 15-item floors; the
partial-inclusive reading is 10 and 10. The seeded and no-known-defect corpus is not
constructed. The proposed classification amendment is not approved.

### FEAS-A4 — Adjudication feasibility

Determine whether two independent human adjudicators are available. Record the five role
requirements, the definition of “author,” and any conflicts. Do not designate an AI agent
as a human adjudicator.

**Acceptance criteria**

- The report distinguishes confirmed availability from unconfirmed availability.
- The operator is not assumed to be an adjudicator.
- No approval or adjudication record is invented.

**State:** Blocked. Repository history shows only the operator as a human git identity;
as a conservative proxy for authorship (a git identity does not show who wrote or
directly edited the text; see report §3.2) the operator is treated as unable to
adjudicate the candidate plans. No second independent human adjudicator is identified.

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
and §§4, 7 and 8 as unverified or awaiting operator decisions. The readiness package links
the [candidate manifest](../reports/independent-design-review-corpus/candidates.json),
[candidate verification](../reports/independent-design-review-corpus/CANDIDATE-VERIFICATION.md),
[readiness summary](../reports/independent-design-review-corpus/READINESS.md),
[preparation plan](../reports/independent-design-review-corpus/CORPUS-PREPARATION-PLAN.md),
[operator decision package](../reports/independent-design-review-corpus/OPERATOR-DECISION-PACKAGE.md),
[freeze procedure](../reports/independent-design-review-corpus/FREEZE-PROCEDURE.md), and
[proposed classification amendment](../reports/independent-design-review-corpus/PROPOSED-PROTOCOL-AMENDMENT.md).

### FEAS-C2 — Validation and pull request

Run the documentation and SOP review gates, commit the plan and report, and open one PR
against `main`. Do not merge.

**Acceptance criteria**

- Documentation links resolve.
- The applicable SOP validation and review gates are recorded.
- One PR is open; no merge is performed.

**State:** In progress. The readiness package PR is pending; no merge is authorized by
this record.

## Dependencies and status

These are coordination statuses in this record, not SOP lifecycle transitions; no plan
was activated.

| Task    | Owner       | State                             | Evidence                          |
| ------- | ----------- | --------------------------------- | --------------------------------- |
| FEAS-A1 | SOP         | Reviewed                          | `candidates.json`; report §2      |
| FEAS-A2 | SOP         | Reviewed with limitations         | `candidates.json`; report §2      |
| FEAS-A3 | SOP         | Not met                           | `READINESS.md` §4                 |
| FEAS-A4 | SOP         | Blocked                           | Report §3                         |
| FEAS-A5 | SOP         | Reviewed                          | Report §§5–7                      |
| FEAS-B1 | Claude Code | Pending                           | Report §4                         |
| FEAS-C1 | SOP         | Complete with unverified sections | Readiness package; report; amendment |
| FEAS-C2 | SOP         | In progress                       | Readiness PR pending              |

## Read-only guarantee

This plan changes documentation only. It does not execute inference for the experiment,
does not run the pilot, does not change a threshold, and does not alter production code,
configuration, lifecycle gates, or approvals. Its task states are coordination statuses in
this record, not SOP lifecycle transitions.
