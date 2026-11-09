# Plan 07 — SOP Misinformation (LLM07)

Status: COMPLETE for the deterministic scope; strict content binding for manual acceptance evidence implemented (Plan 07 D4 option 1); all three SOP tasks externally completed and the plan archived. Live-model and Linux verification are NOT_RUN. Evidence: `docs/reports/owasp-llm/llm07-misinformation.md`.
Date: 2026-11-08 (America/Chicago); adapted 2026-11-09.
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm07-misinformation.
Suggested repository path: docs/plans/PLAN-07-SOP-LLM07-Misinformation.md.

## Project

`imhttran/agentic-sop`. This plan hardens SOP's use of evidence in decisions: a
generated completion claim, citation, test summary, review finding, or historical
suggestion must not become a verified outcome or authorize a downstream action
unless the existing evidence resolver and lifecycle contracts accept it.

## Summary

Prevent unsupported model claims from becoming verified outcomes or authorizing downstream actions. Reuse SOP's evidence resolver, criterion mapping, approval provenance, and review workflow.

Dependency: Plan 01's authority inventory and current acceptance/review hardening. Plan 09 owns retrieval isolation. This plan owns the use of evidence in decisions, not general factual correctness of every generated sentence.

## GROUND-001 — Trace claims to authoritative decisions

Map generated completion text, proposed findings, citations, test summaries, and historical suggestions to stored outcomes and downstream actions. Identify authoritative sources and freshness requirements for each transition.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm07-misinformation.md`

### Acceptance Criteria

- Distinguish model assertions, observed tool results, accepted human evidence, and verified task status.
- Identify content/commit, task, attempt, and policy binding required by current evidence contracts.
- Deliver docs/reports/owasp-llm/llm07-misinformation.md with concrete claim-to-decision traces.

### Acceptance Checks

- `go test ./internal/outcome/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## GROUND-002 — Exercise fabricated and stale evidence

Use deterministic responses that claim tests passed when the recorded test failed or never ran; cite nonexistent evidence; reference another task/attempt; reuse stale commit evidence; and claim approval without an authoritative decision. Repair only reproduced acceptance or provenance defects.

### Dependencies

GROUND-001

### Deliverables

- `internal/toolharness/state_dir_write_test.go`
- `internal/cli/misinformation_fixtures_test.go`

### Acceptance Criteria

- Fabricated PASS text cannot override failed or missing required checks.
- Evidence from the wrong identity or invalidated content cannot satisfy the current task.
- ALREADY_SATISFIED follows the existing evidence contract rather than free-form assertions.
- Untrusted retrieved material cannot grant approval or replace current verification.
- Valid observed evidence and supported manual acceptance still advance normally.
- Regression tests invoke the real outcome/transition path, not only a prompt template.

### Acceptance Checks

- `go test ./internal/toolharness/... -run TestStateDirectoryIsWriteProtectedForModelTools`
- `go test ./internal/cli/... -run TestModelPassClaimCannotOverrideFailedAcceptanceCheck`
- `go test ./internal/cli/... -run TestModelApprovalClaimWithoutDecisionDoesNotAdvance`

### Acceptance Mapping

- criterion 1: check 2
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 3
- criterion 6: check 2

## GROUND-003 — Make reports match observed state

Repair reporting inconsistencies at existing output paths where necessary. Keep failed, blocked, cancelled, unverified, and verified outcomes distinct. Document when a source supports a claim without proving it.

### Dependencies

GROUND-002

### Deliverables

- `docs/reports/owasp-llm/llm07-misinformation.md`
- `internal/cli/outcome_stale_test.go`

### Acceptance Criteria

- User-visible result and persisted lifecycle agree for the tested paths.
- Missing checks are NOT_RUN, not implied PASS.
- Source references resolve to the recorded evidence identity where that contract exists.
- Required checks pass; PR links before/after evidence and residual factual-error risks.

### Acceptance Checks

- `go test ./internal/cli/... -run TestFailedRunDoesNotExposePreviousVerifiedOutcome`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1

## Risks and completion

Grounded text can still be wrong, and a valid source can be inaccurate. LLM judges and fluent explanations are not independent verification. Do not introduce a general fact-checking service or duplicate review gate. Complete when the existing authority chain rejects unsupported claims and reports remaining uncertainty accurately.

## Execution contract

Planning only; no implementation, SOP ingestion, configuration change, or installation has occurred. This is a task contract to adapt to the current SOP plan schema, not a compiler-validated manifest.

When authorized, refresh origin/main, read AGENTS.md and current plan-authoring conventions, and create a new isolated worktree and branch. Preserve existing worktrees, active plans, uncommitted files, and lifecycle history. Reconcile through supported SOP interfaces; never edit state.db or fabricate approvals.

Execute the tasks below sequentially through existing lifecycle and PR gates. Continue autonomously within approved scope until completion or a genuine blocker. Reuse existing abstractions and record ALREADY_SATISFIED only with current evidence. Fix reproducible defects in existing contracts; do not introduce another gate, evaluator, permission system, or service. A missing control requiring new policy gets a concrete proposal through existing governance; stop only dependent work.

Preserve report-only/required settings, configured limits, routing, and autonomy defaults. Do not enable remote evaluations, install tools, download models, expand permissions, or change security policy implicitly. Use synthetic data, disposable fixtures, fake providers, and inert effect recorders. No production writes or real exfiltration.

Run focused regression tests for changed behavior, then repository-required checks once. Repeat only after relevant changes or failures. Record exact commands, exit codes, base/head commits, configuration identity, platform, and evidence paths. Open a focused PR; follow established review and merge governance. Do not claim installation or deployment from a PR result.

## Baseline and evidence

The four preceding planning files used historical baseline 0750e8be0cf5f318a173abb34c2b752ac131521c. Their observations are context, not current runtime evidence. These six plans do not claim a fresh code audit or that preceding work has merged. Refresh the baseline and inspect current implementations before choosing files or commands.

Report IMPLEMENTED, ENABLED, and VERIFIED separately, with the exact path and platform. Use GAP, NOT_RUN, and justified NOT_APPLICABLE explicitly. A deterministic test is not live-model evidence; a scan PASS is not complete category coverage. Missing required evidence leaves the associated criterion incomplete.

Category numbers and names follow the supplied Cycode article's displayed 2026 list, accessed November 8, 2026 (America/Chicago), consistent with Plans 01–04. This is an article-based planning taxonomy, not a verified formal OWASP compliance mapping.

## Sources and handoff

- https://cycode.com/blog/owasp-top-10-llm-applications/
- Repository: https://github.com/imhttran/agentic-sop
- Earlier plans: PLAN-01-SOP-LLM03-Excessive-Agency.md; PLAN-02-SOP-LLM01-Prompt-Injection.md; PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md; PLAN-04-SOP-LLM04-Supply-Chain.md.
- At execution, inspect current docs/specs/SECURITY.md, docs/specs/QUALITY.md, configuration, and relevant reports before relying on historical assertions.

Final handoff: changed behavior, criterion-to-evidence mapping, PR link, residual gaps, blocked criteria, and exact next action. Proposed report paths are deliverables, not claims that files already exist.
