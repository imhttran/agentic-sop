# Plan 01 — SOP excessive agency coverage (LLM03)

Status: READY FOR SOP RECONCILIATION; not executed.
Order: 1 of 4.
Suggested branch: security/llm03-agency.
Dependency: none. This plan produces the boundary inventory consumed by Plans 02 and 03.

## Goal and scope

Establish where SOP actually limits model authority. Verify existing command, path,
approval, and lifecycle boundaries; close demonstrated bypasses within their current
contracts. Do not roll out a general agent sandbox in this plan.

## Project

agentic-sop

## Summary

Produce the current execution-boundary inventory, reproduce focused negative and
positive control cases against the production policy/approval boundary, repair only
demonstrated violations, and record the exact verification scope, platform, and
residual authority. Policy-only enforcement must never be described as OS
confinement, and Linux runtime evidence stays tied to its original commit.

## Migration provenance

- Linux transfer commit: `d42e63a27ec4d4cd9adef8f68d39676542864063` on
  `task/LLM03-TRANSFER-plan-01-transfer-checkpoint` (transfer PR #269; checkpoint,
  not completed implementation). Applied to `origin/main` `aa232175`.
- Recorded plan source sha256 at the Linux checkpoint:
  `992bfecd8b2abe73786ce0b92c3e7e6f312d1142937fd0ef60cf2eae74b4b325`.
- Linux SOP lifecycle at checkpoint: plan `ACTIVE`; `AGENCY-001`, `AGENCY-002`,
  `AGENCY-003` all `PLANNED`; no approval requests; last run `CONTINUE`
  (`INCOMPLETE_IMPLEMENTATION`, cause `VALIDATION_FAILED`). Nothing was complete.
- The Linux state database is not restored. This Mac plan is initialized fresh
  through supported SOP bootstrap and reconciliation; the Linux checkpoint is
  provenance only, not fabricated completed work.
- Historical Linux evidence, tied to baseline `0750e8be`: `go build ./...` PASS;
  `go vet ./...` PASS; Bubblewrap isolation suite PASS (Linux only). The Linux
  `go test ./...` run FAILED by timeout because of host storage failure; it is
  incomplete verification, not an implementation finding. It must be re-run on
  this Mac without weakening or re-timing it.
- The recovered `docs/reports/owasp-llm/llm03-agency.md` is the AGENCY-001
  inventory. It is updated against current code during execution; the Linux
  Bubblewrap result remains labelled Linux/historical.

## AGENCY-001 — Establish the current execution boundary

Priority: P1. Inspect normal tool-agent calls, configured external agent commands,
verification commands, Git/GitHub operations, planner/MCP entry points, replay, and
evaltarget. Trace each path from caller to command/effect sink and identify the
actual policy call, approval authority, credentials/environment, filesystem reach,
and OS isolation.

Reuse the recovered inventory and update it against the current checkout. Do not
rerun unrelated suites to produce it; inspect the current source and cite
`file:symbol` references. Mark any step that cannot be proven as `UNVERIFIED` with
the reason.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm03-agency.md` — the path-by-path execution matrix, the trusted-configuration/untrusted-input distinction, the policy-versus-OS-isolation distinction, the identified test coverage, and immutable source references updated against current code.

### Acceptance Criteria

- Every supported execution entry point is either traced or explicitly marked UNVERIFIED with a reason.
- Trusted configuration is distinguished from untrusted model/request data.
- Policy-only enforcement is distinguished from OS isolation.
- Existing tests and host-specific runtime evidence are identified without rerunning unrelated suites.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm03-agency.md §Path-by-path matrix; every numbered entry point carries a caller-to-sink trace with file:symbol references or an explicit UNVERIFIED reason, including the untraced MCP handler, recovery/resume, eval, and runtrace paths
- criterion 2: manual: docs/reports/owasp-llm/llm03-agency.md §Distinctions required by the task, paragraph "Trusted configuration vs untrusted model/request data"
- criterion 3: manual: docs/reports/owasp-llm/llm03-agency.md §Distinctions required by the task, paragraph "Policy-only enforcement vs OS isolation", and §Summary findings items 3 and 5
- criterion 4: manual: docs/reports/owasp-llm/llm03-agency.md §Existing tests (identified by inspection) and §Host-specific runtime evidence, which name the suites and mark the Linux-only result as historical

### Execution

- verify-first — the reviewed inventory is present in the working tree; the configured validation proves the repository still builds and the checked packages pass, so no further implementation is required.

## AGENCY-002 — Reproduce and repair contract violations

Priority: P1. Use the inventory to select focused negative cases: unauthorized
commands, destructive Git operations, repository escape and location overrides,
runtime-state mutation, and stale or mismatched authorization where the current
contract defines it. Include an authorized operation as a positive control.

Reuse current commandpolicy, approval, and tool boundaries. Fix only demonstrated
violations. If no violation exists, record ALREADY_SATISFIED rather than refactor.
Tests must exercise the production boundary, not a duplicate policy implementation.

### Dependencies

- AGENCY-001

### Deliverables

- Reproduced negative cases against the production boundary, with the evidence that demonstrates the violation, or ALREADY_SATISFIED with the existing evidence.
- Minimal fixes, only for demonstrated violations, at the existing boundary (no new approval layer, evaluator, or permission model).
- Focused regression tests that exercise the production boundary and include the authorized-operation positive control.

### Acceptance Criteria

- Denied operations have no side effect at the governed sink.
- Approved operations remain usable; tightening does not break supported verification.
- An approval cannot authorize a different operation under the existing binding contract.
- Tests exercise the production boundary, not a duplicate policy implementation.
- Any ungoverned external-agent path is reported honestly; it is not silently claimed protected.

### Acceptance Checks

- `go test ./internal/toolharness ./internal/agent ./internal/approval ./internal/git ./internal/github -count=1`
- `go build ./...`
- `go vet ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: manual: docs/reports/owasp-llm/llm03-agency.md §Summary findings item 5 and §Path-by-path matrix UNVERIFIED entries; the report must name the configured external-agent shell, MCP handler set, and recovery/resume paths as policy-only or UNVERIFIED, never as sandboxed

### Execution

- verify-first — the focused regression tests and the reviewed report are present in the working tree; the configured validation proves them, so no further implementation is required.

## AGENCY-003 — Verify, document residual authority, and open PR

Priority: P1. Run focused checks and required repository verification. Update the
coverage matrix with the exact scope of each result. Open one focused PR if changes
are justified.

### Dependencies

- AGENCY-002

### Deliverables

- `docs/reports/owasp-llm/llm03-agency.md` — verification commands, results, platform, base/head SHAs, remaining gaps, and the PR link.
- `docs/reports/owasp-llm/README.md` — add the LLM03 matrix row without replacing or rewriting the existing Plan 01-04 rows.

### Acceptance Criteria

- Report lists commands, results, platform, base/head SHAs, remaining gaps, and PR link.
- Normal agent processes are not described as sandboxed unless this is separately demonstrated.
- Any architecture-level gap has a concrete proposed follow-up and stops only dependent work.
- Handoff identifies reusable production seams for Plan 02.

### Acceptance Checks

- `go test ./...`

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm03-agency.md §Verification record with commands, exit status, platform, base/head SHAs, remaining gaps, and the PR link
- criterion 2: manual: docs/reports/owasp-llm/llm03-agency.md §Summary findings item 3 and §Host-specific runtime evidence; normal agent execution is policy-only, and only evaltarget carries OS isolation
- criterion 3: manual: docs/reports/owasp-llm/llm03-agency.md §Optional targeted follow-up and §Remaining gaps; each architecture-level gap names a concrete follow-up and does not block unrelated work
- criterion 4: manual: docs/reports/owasp-llm/llm03-agency.md §Reusable production seams for Plan 02, naming the evaltarget observer, production authorization controls, and inert effect boundary

### Execution

- verify-first — the verification record, matrix update, and reviewed inventory are present in the working tree; the configured validation proves the repository still builds and the checked packages pass.

## Risks and boundaries

Configured shell commands and repository build scripts can execute code even when
their command names look safe. Document this trust explicitly. Policy
classification must not be presented as process confinement. A general sandbox or
new authorization architecture is outside this plan.

## Completion

Complete when the inventory and focused evidence are reviewed through the existing
PR workflow, fixes are verified where needed, and residual risks are explicit. Plan
02 starts from the merged baseline, or from an explicitly approved dependency
branch if existing workflow permits it.

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
- Linux transfer: PR #269 (`task/LLM03-TRANSFER-plan-01-transfer-checkpoint`, `d42e63a2`)
- Existing policy: docs/specs/SECURITY.md and docs/specs/QUALITY.md
- Current configuration: .agent-sdlc/config.yaml
- Recorded evidence: docs/reports/security-hardening/final.md
- Execution boundaries: docs/reports/security-shift-left/execution-path-matrix.md
- Live evaluation limits: docs/reports/security-shift-left/aisec-live-assessment.md
