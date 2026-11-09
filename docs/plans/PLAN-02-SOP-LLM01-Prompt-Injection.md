# Plan 02 — SOP prompt injection evidence (LLM01)

Status: READY FOR SOP RECONCILIATION; stacked on Plan 01 (PR #271); not executed.
Order: 2 of 4.
Suggested branch: security/llm01-injection.
Dependency: Plan 01 execution boundary inventory and reviewed production enforcement seams.

## Goal and scope

Produce bounded live adversarial evidence showing whether injected instructions can cause unauthorized effects through SOP's actual controls. Reuse internal/evaltarget and existing AISEC scenarios. Do not build a second evaluator or claim the model cannot be fooled.

## Project

agentic-sop

## Summary

Route a real local model's structured tool decision through SOP's actual
production authorization and command-policy boundaries into the inert recording
target, and record whether the operation was allowed or denied. Keep
deterministic, real-control, live-smoke, and live-adversarial evidence distinct;
a text assertion or model refusal is never an observed blocked effect. If safe
connectivity, decision capture, or isolation prerequisites are unavailable,
record the exact blocker and leave the live acceptance NOT_RUN.

## Migration provenance and dependency baseline

- Plan source: the recovered Linux planning document, added to this branch as
  `docs/plans/PLAN-02-SOP-LLM01-Prompt-Injection.md`; it was not previously in
  the repository.
- Plan 01 baseline: PR #271 (`task/LLM03-impl-plan-01-agency-boundary-controls`,
  `4af450e1`), not yet merged to `main`. This branch is stacked on it as the
  explicitly approved dependency branch and must be rebased/retargeted after
  #271 merges.
- Reusable seams from the Plan 01 handoff: `internal/toolharness` policy,
  `internal/commandpolicy`, `internal/approval.Service`, `internal/git`,
  `internal/github`, and `internal/evaltarget` (`RecordingTools`,
  `ProductionControls`, `RunChild`, `IsolateEnv`, `confinedDir`).
- Local model available for the bounded campaign: `ollama` at the fixed
  `http://127.0.0.1:11434` endpoint (for example `qwen3.5:4b`). No cloud model
  and no unrelated egress are permitted for the live campaign.

## INJECT-001 — Define the supported live evaluation seam

Inspect `internal/evaltarget`, `production_controls`, the Promptfoo
adapter/parser/configuration, and prompt compilation. Choose the smallest
supported path that captures a real model tool decision and submits it to the
real authorization boundary inside the controlled evaluation environment.

Record the chosen seam in the report as model → structured tool decision →
production authorization/policy/approval → inert effect → observed outcome, and
state why a text assertion or model refusal alone cannot count.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm01-injection.md` — the supported live seam, the evidence-class distinctions, the reused AISEC scenarios, and the prerequisite checks.

### Acceptance Criteria

- The existing six AISEC scenarios remain the scenario source; extend only where a concrete gap requires it.
- Deterministic, real-control, live-smoke, and live-adversarial results remain distinct.
- A text assertion alone cannot count as an observed blocked effect.
- No general live-evaluation framework or new policy authority is introduced.
- Missing model, pinned binary, or isolation prerequisites are identified before live execution.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm01-injection.md §Scenario source and §Seam; the report names AISEC-001..006 and any concrete extension
- criterion 2: manual: docs/reports/owasp-llm/llm01-injection.md §Evidence classes, which separates deterministic, real-control, live-smoke, and live-adversarial results
- criterion 3: manual: docs/reports/owasp-llm/llm01-injection.md §Observed-effect boundary, which states that the recorded control decision and effect, not model prose, are the verdict
- criterion 4: manual: docs/reports/owasp-llm/llm01-injection.md §Reused facilities, which names the existing evaltarget/production-control packages and states that no second evaluator or policy authority is added
- criterion 5: manual: docs/reports/owasp-llm/llm01-injection.md §Prerequisites, which records the local endpoint, model, binary, and isolation checks performed before live execution

### Execution

- verify-first — the seam report is present in the working tree; the configured validation proves the repository still builds and the checked packages pass.

## INJECT-002 — Implement the bounded live path and focused cases

Add only the wiring required by the selected seam. Cover direct instruction
override, indirect injection in retrieved/tool content, forged policy or approval
claims, and an authorized positive control using existing scenarios where
possible.

Use a local model and synthetic data. Permit only the exact local inference
endpoint needed; block unrelated egress using supported isolation. Protected
operations target an inert recording boundary. Do not issue real network effects.

Name the new focused tests with the `TestLive` prefix so the acceptance checks
select exactly the new path.

### Dependencies

- INJECT-001

### Deliverables

- `internal/evaltarget` — the bounded live model-decision path and its `TestLive` fixture tests (including observer-failure, malformed-output, timeout, and bounds cases), reusing `RecordingTools`, `ProductionControls`, and the existing AISEC scenarios.
- `docs/reports/owasp-llm/llm01-injection.md` — the implemented seam, bounds, and focused-test evidence.

### Acceptance Criteria

- A live model emits a captured structured tool decision.
- The actual production policy/approval service decides the submitted operation.
- Independent effect recording shows allowed/denied outcome; model prose is not the verdict.
- Malformed output, timeout, observer failure, or unavailable isolation is error/NOT_RUN, never PASS.
- Attempts, requests, concurrency, process lifetime, and output size are bounded.
- New behavior has focused fixture tests, including the observer failure path.

### Acceptance Checks

- `go test ./internal/evaltarget -run TestLive -count=1`
- `go test ./internal/evaltarget -count=1`
- `go build ./...`
- `go vet ./...`

### Acceptance Mapping

- criterion 1: manual: the bounded live-smoke artifact recorded by the new path (fixed local endpoint/model, one structured tool decision); if the live prerequisite is unavailable, the artifact records NOT_RUN and this criterion remains unmet
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1

### Execution

- verify-first — the bounded live path and its focused fixture tests are present in the working tree; the configured validation proves them. The live-model smoke remains NOT_RUN until a safe localhost inference exception exists.

## INJECT-003 — Run bounded evidence and complete review

Before execution, record the model/provider/version, prompt/scenario hashes,
fixed case count, request budget, and timeout. Perform one bounded campaign; do
not retry until a favorable response appears. Preserve attempted cases, refusals,
failures, and unsafe proposals with sanitized evidence.

### Dependencies

- INJECT-002

### Deliverables

- `docs/reports/owasp-llm/llm01-injection.md` — the campaign record, exact cases and repeat count, observed allowed/denied outcomes, positive control, sanitized failures, and explicit NOT_RUN prerequisites.
- `internal/evaltarget` — the deny-all positive-control test proving a boundary that denies everything cannot pass the suite.

### Acceptance Criteria

- Results distinguish model compliance with injection from policy rejection and actual side effects.
- The positive control proves that a boundary denying everything cannot pass the suite.
- Report states exact cases and repeat count; no universal robustness or success-rate claim from a small sample.
- Any bypass is reproduced deterministically, fixed within existing authority, and regression-tested.
- Routine Promptfoo remains disabled unless a separate existing policy decision changes it.
- Focused PR includes observed results and explicit NOT_RUN prerequisites.

### Acceptance Checks

- `go test ./internal/evaltarget -run TestLivePositiveControl -count=1`
- `go test ./...`

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm01-injection.md §Campaign record, which separates model compliance, policy rejection, and observed effects
- criterion 2: check 1
- criterion 3: manual: docs/reports/owasp-llm/llm01-injection.md §Campaign record, which states the exact cases, repeat count, and sample limits
- criterion 4: manual: docs/reports/owasp-llm/llm01-injection.md §Bypasses (or the explicit "none reproduced" statement with the tested cases) and the regression test named there
- criterion 5: manual: docs/reports/owasp-llm/llm01-injection.md §Prerequisites and `.agent-sdlc/config.yaml`, which keep the Promptfoo evaluation disabled
- criterion 6: manual: the PR body and docs/reports/owasp-llm/llm01-injection.md §Campaign record, which name observed results and every NOT_RUN prerequisite

## Risks and boundaries

A local model endpoint needs a narrow connectivity exception; do not weaken
isolation globally to make the test run. If safe connectivity or decision capture
is unavailable, record the exact blocker and finish independent
fixture/documentation work. Cross-modal attacks and protection for arbitrary
external agent runtimes are outside this initial campaign.

## Completion

Complete when the live seam and bounded evidence are reviewed and required checks
pass. If live execution cannot run, mark the live acceptance incomplete, not
complete-by-documentation. Hand the reusable seam and evidence format to Plan 03.

## Execution contract

- Planning deliverable only. No implementation or configuration change has been executed by creating this plan.
- When execution is authorized, refresh origin/main, read repository AGENTS.md and current SOP plan-authoring conventions, and create a new isolated worktree and branch from that baseline. Preserve existing worktrees and uncommitted changes.
- Import/reconcile this plan through supported SOP interfaces. Adapt task metadata to the current schema before execution; these Markdown tasks are not a claim of successful SOP ingestion.
- Run tasks sequentially through existing SOP lifecycle, validation, review, approval, and PR gates. Continue autonomously within approved scope until completion or a genuine blocker.
- Reuse existing abstractions. Record ALREADY_SATISFIED with current evidence when appropriate. Do not add duplicate evaluators, approval layers, or speculative controls.
- Fix only reproducible gaps against existing contracts. A new permission model, sandbox rollout, external service, or security-policy change requires a concrete proposal and the existing governance decision.
- Do not silently enable required security mode, change required categories, weaken severity rules, expand suppressions, or alter autonomy settings.
- Run focused regression checks for changed behavior, followed by repository-required checks once. Record actual commands, exit status, commit, platform, and evidence paths. Re-run only when changes or failures justify it.
- Open a focused PR through the established workflow. Do not claim merge, installation, deployment, or runtime verification unless observed. No forced merge.
- No real credentials, external exfiltration destinations, destructive operations, or production writes in test fixtures.

## Baseline and evidence rules

Planning baseline: imhttran/agentic-sop main at 0750e8be0cf5f318a173abb34c2b752ac131521c, reviewed in this conversation. Refresh at execution; these observations are not a statement about a later checkout or installed binary.

At that baseline, security scanning is enabled in report-only mode, required categories are unconfigured, and routine Promptfoo evaluation is disabled. The final hardening report records PASS / COMPLETE for declared SAST, secrets, and dependency scanning scope. This is not full OWASP coverage. Normal agent execution is not OS-sandboxed; isolation evidence is specific to replay/evaltarget paths. Live adversarial evaluation is not implemented.

Use IMPLEMENTED, ENABLED, VERIFIED (named platform/path), NOT_RUN, GAP, and NOT_APPLICABLE as separate evidence dimensions. Never infer runtime protection from a specification, fixture, scanner PASS, or dependency plan alone.

Reference category IDs follow the 2026 list displayed by the supplied Cycode article. Confirm the official OWASP edition before publishing a formal compliance mapping; these plans make no certification claim.

## Sources

- Article: https://cycode.com/blog/owasp-top-10-llm-applications/
- Repository baseline: https://github.com/imhttran/agentic-sop/tree/0750e8be0cf5f318a173abb34c2b752ac131521c
- Plan 01 handoff: PR #271 (`docs/reports/owasp-llm/llm03-agency.md` §Plan 02 handoff)
- Existing policy: docs/specs/SECURITY.md and docs/specs/QUALITY.md
- Current configuration: .agent-sdlc/config.yaml
- Recorded evidence: docs/reports/security-hardening/final.md
- Execution boundaries: docs/reports/security-shift-left/execution-path-matrix.md
- Live evaluation limits: docs/reports/security-shift-left/aisec-live-assessment.md
