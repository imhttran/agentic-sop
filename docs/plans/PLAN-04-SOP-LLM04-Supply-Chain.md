# Plan 04 — SOP AI supply-chain coverage (LLM04)

Status: IMPLEMENTED — Mac workstream executed. SOP tasks LOCAL_DONE; final gate PASS; historical outcomes remain PARTIAL until re-execution under the newly recorded acceptance mapping. See the dependency reconciliation below.
Order: 4 of 4.
Suggested branch: security/llm04-supply-chain.
Dependency: none on Plan 03 for execution. Plan 04 consumes the current
model/tool inventory and the preceding baseline, but it does not require a
successful adversarial model response.

## Project

agentic-sop

## Summary

Establish which dependencies, executables, model services, and optional artifacts
SOP actually uses and how their identity and integrity are checked. Reuse OSV,
existing pins, provenance, and readiness checks. Add controls only for demonstrated
gaps in SOP-owned loading or execution paths.

## Dependency reconciliation

- The original "QUEUED AFTER PLAN 03" dependency is removed. Plan 04 may proceed
  independently; the baseline commit and the repository inventory are sufficient.
- Plan 03's static data-flow tracing also proceeds independently. Only Plan 03's
  live adversarial tests depend on Plan 02's merged live-test infrastructure.
- This record changes scheduling only. It does not change any security mode,
  required category, severity rule, autonomy setting, or executable behavior.
- SOP reconciliation: the executed task definitions gained an acceptance mapping
  after their first run. Each changed executed definition is accepted explicitly
  through `sop reconcile --accept-changed`; the original outcomes and attempt
  history are preserved. The mapping is authoritative for future re-execution.

## Goal and scope

Establish which dependencies, executables, model services, and optional artifacts
SOP actually uses and how their identity and integrity are checked. Reuse OSV,
existing pins, provenance, and readiness checks. Add controls only for demonstrated
gaps in SOP-owned loading or execution paths.

## SUPPLY-001 — Inventory the actual trust chain

Inspect Go/Rust manifests and lockfiles, scanner adapters, installation/provisioning
scripts, configured agent executables, model/provider selection, and optional local
model/adapters if SOP itself loads them. Distinguish directly loaded artifacts from
models managed by Ollama, oMLX, or a remote provider. Write
`docs/reports/owasp-llm/llm04-supply-chain.md` with component, owner, acquisition
source, version/digest, verification method, update process, and trust limitation.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm04-supply-chain.md`

### Acceptance Criteria

- Inventory derives from current code/configuration; no hypothetical framework entries.
- A provider model name is not described as an immutable model digest.
- Scanner coverage is separated from executable authenticity and model provenance.
- Hash matching is not represented as publisher authentication.
- NOT_APPLICABLE entries explain why SOP does not own the loading boundary.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Trust-chain inventory; source references `go.mod`, `.agent-sdlc/config.yaml`, and `scripts/security/*-manifest.json`
- criterion 2: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Trust-chain inventory row "Model providers and model files" and §SOP-owned versus provider-owned
- criterion 3: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Trust-chain inventory scanner rows and §SOP-owned versus provider-owned
- criterion 4: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Trust-chain inventory verification column and §Residual gaps and boundaries
- criterion 5: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Residual gaps and boundaries NOT_APPLICABLE rows and §Trust-chain inventory

### Execution

- verify-first — the inventory report is present in the working tree and the configured validation proves the repository still builds.

## SUPPLY-002 — Verify existing checks and repair demonstrated gaps

Inspect version checks, dependency scanner selection, lockfile scope, database
freshness/integrity, missing-prerequisite handling, and supported artifact loading
paths. Reuse current adapter contracts and readiness logic. Reproduce each gap
before fixing it.

### Dependencies

- SUPPLY-001

### Deliverables

- `internal/security/osv.go`
- `internal/security/osv_metadata_test.go`

### Acceptance Criteria

- Focused cases cover missing/mismatched supported tools, corrupt/stale advisory data, and incomplete declared dependency coverage as applicable.
- Required mode fails closed under its existing contract; report-only remains report-only.
- No network downloads or tool installs occur implicitly as a test side effect.
- Fixes are limited to existing contracts; broad signing infrastructure and new mandatory providers are excluded.
- No new vulnerability suppression is added to manufacture a clean result.

### Acceptance Checks

- `go test ./internal/security -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Verification record and §Reproduced gap and fix; no provisioning script ran and the tests use in-memory or temporary fixtures
- criterion 4: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Reproduced gap and fix and §Residual gaps and boundaries; the diff is limited to `internal/security/osv.go` and its tests
- criterion 5: manual: `.agent-sdlc/security-baseline.json` is unchanged; docs/reports/owasp-llm/llm04-supply-chain.md §Verification record records the existing 53 suppressions and no new suppression

### Execution

- verify-first — the fix and its regression cases are present in the working tree; the focused package test and the configured validation prove them.

## SUPPLY-003 — Produce a maintainable inventory and finish review

Use existing inventory/provenance formats. Keep a small versioned Markdown
inventory tied to the current commit, with a documented update command or
procedure. Do not add a new service merely to generate an AI BOM. Write the
cumulative matrix at `docs/reports/owasp-llm/README.md`.

### Dependencies

- SUPPLY-001
- SUPPLY-002

### Deliverables

- `docs/reports/owasp-llm/README.md`
- `docs/reports/owasp-llm/llm04-supply-chain.md`
- `docs/plans/README.md`

### Acceptance Criteria

- Supported checks are mapped to observed evidence; provider-owned assurances remain unverified unless sourced.
- Current configured dependency scan is reported with scope, version, database freshness, result, and exact limitations.
- New provisioning, signature requirements, or required-mode enablement are recorded as concrete policy decisions, not silently enabled.
- Focused and required checks pass; PR contains final coverage and residual-risk summary.
- A short cumulative matrix links all four reports without marking a category fully covered from partial evidence.

### Acceptance Checks

- `go test ./internal/security -count=1`

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Verification record and §Trust-chain inventory
- criterion 2: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Configured scan on this baseline
- criterion 3: manual: docs/reports/owasp-llm/llm04-supply-chain.md §Residual gaps and boundaries; `.agent-sdlc/config.yaml` still has `security.mode: report-only` and no required categories
- criterion 4: manual: `sop validate` PASS, `go test ./internal/security -count=1` PASS, and PR #265 body records final coverage and residual risks
- criterion 5: manual: docs/reports/owasp-llm/README.md cumulative matrix

### Execution

- verify-first — the matrix and inventory updates are present in the working tree.

## Risks and boundaries

Pinned versions can become vulnerable; advisory freshness and update ownership matter.
Remote model revisions may change without an immutable identifier exposed to SOP.
Downloaded artifacts require an authentic trust root for signature verification; a
checksum from the same untrusted source is insufficient. Training-data provenance
and provider internal security remain outside SOP's direct control.

## Completion

Complete when the actual inventory, supported integrity checks, verified fixes, and
residual provider assumptions are reviewed through the existing workflow. The
four-plan program closes only to the extent supported by evidence; incomplete live
tests or unresolved architecture decisions remain visible.

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
- Existing policy: docs/specs/SECURITY.md and docs/specs/QUALITY.md
- Current configuration: .agent-sdlc/config.yaml
- Recorded evidence: docs/reports/security-hardening/final.md
- Execution boundaries: docs/reports/security-shift-left/execution-path-matrix.md
- Live evaluation limits: docs/reports/security-shift-left/aisec-live-assessment.md
