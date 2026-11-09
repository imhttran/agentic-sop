# Plan 03 — SOP sensitive information disclosure (LLM02)

Status: STATIC TRACING IMPLEMENTED / VERIFIED; LIVE `NOT_RUN` (Plan 02
handoff unavailable). Historical task outcomes remain PARTIAL until re-execution
under the newly recorded acceptance mapping. See the dependency reconciliation below.
Order: 3 of 4.
Suggested branch: security/llm02-disclosure.
Dependencies: Plan 01 boundary inventory; Plan 02 live seam for adversarial
acceptance. Static tracing can proceed if live execution is blocked.

## Project

agentic-sop

## Summary

Trace sensitive information from inputs to model requests, tool effects, persisted
evidence, and operator output. Close demonstrated leaks at existing boundaries.
Reuse shared redaction and retrieval scope controls. Secret scanning alone is not
outbound data protection.

## Dependency reconciliation

- DISCLOSE-001 (static data-flow tracing) proceeds independently. It does not
  require Plan 01 or Plan 02.
- DISCLOSE-002 (canary regressions and confirmed fixes) depends only on
  DISCLOSE-001 and is executed in this workstream.
- DISCLOSE-003 (live extraction) depends on Plan 02's merged live-test
  infrastructure. That infrastructure is not present in this checkout, so
  DISCLOSE-003 remains `NOT_RUN` and is intentionally not executed by this
  workstream. No competing live-test implementation is created.
- This record changes scheduling only. It does not change any security mode,
  required category, severity rule, autonomy setting, or executable behavior.
- SOP reconciliation: the executed task definitions gained an acceptance mapping
  after their first run. Each changed executed definition is accepted explicitly
  through `sop reconcile --accept-changed`; the original outcomes and attempt
  history are preserved. The mapping is authoritative for future re-execution.

## Goal and scope

Trace sensitive information from inputs to model requests, tool effects, persisted
evidence, and operator output. Close demonstrated leaks at existing boundaries.
Reuse shared redaction and retrieval scope controls. Secret scanning alone is not
outbound data protection.

## DISCLOSE-001 — Map data sources and sinks

Inspect file reads, process environment, configuration, tool stdout/stderr,
retrieval results, provider request construction, diagnostics, run artifacts,
handoffs, and enabled notification payloads. Review credential redaction call
sites, not just the shared helper. Write `docs/reports/owasp-llm/llm02-disclosure.md`
with source, transformation, authorization, sink, retention, and evidence columns.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm02-disclosure.md`

### Acceptance Criteria

- The report exists, traces the listed sources and sinks, and separates provider authentication from model-visible content.

### Acceptance Checks

- `go test ./internal/cli -run TestOWASPLLMDisclosureReportPresent -count=1`

### Acceptance Mapping

- criterion 1: check 1

### Execution

- verify-first — the static trace report is present in the working tree; the deterministic report test and the configured validation prove it.

## DISCLOSE-002 — Add canary regressions and fix confirmed leaks

Use synthetic, unique canaries in isolated fixtures for credential-shaped values,
known environment secrets, tool errors, and unauthorized retrieved records. Test
the actual request and artifact construction paths. Inspect serialized model
messages separately from transport authentication.

### Dependencies

- DISCLOSE-001

### Deliverables

- `internal/cli/redaction.go`
- `internal/cli/outcome.go`
- `internal/cli/run.go`
- `internal/cli/disclosure_redaction_test.go`
- `internal/agent/openaicompatible_execution_test.go`

### Acceptance Criteria

- The two reproduced leaks are fixed through `redactDiagnosticText`; environment secrets and credential-shaped text do not appear in check logs, report excerpts, or the model-visible validation failure context; ordinary diagnostic text survives; the provider key remains in the `Authorization` header and not the request body; existing retrieval, handoff, notification, ACP, controller, and outcome redaction tests still pass.

### Acceptance Checks

- `go test ./internal/cli -count=1`

### Acceptance Mapping

- criterion 1: check 1

### Execution

- verify-first — the fix and its canary regressions are present in the working tree; the package test and the configured validation prove them.

## DISCLOSE-003 — Exercise live extraction and finish review

Reuse Plan 02's bounded local evaluation path for requests to reveal synthetic
secrets or transmit protected content. Observe actual outputs and attempted
effects, not an LLM judge alone. This task is intentionally left unexecuted until
Plan 02's merged live-test infrastructure is available; it adds no competing
live-test implementation.

### Dependencies

- DISCLOSE-002

### Deliverables

- `docs/reports/owasp-llm/llm02-disclosure.md`

### Acceptance Criteria

- Live direct and indirect extraction are `NOT_RUN` with the missing Plan 02 prerequisite recorded; no fixture or text assertion is presented as live evidence.

### Acceptance Mapping

- criterion 1: manual: Plan 02 merged live-test infrastructure and recorded live extraction evidence (currently NOT_RUN)

## Risks and boundaries

Over-redaction can break diagnostics or valid authentication; negative and positive
fixtures are necessary. Generic detection cannot promise to protect every sensitive
string. Adding a broad DLP product, changing provider/data-residency policy,
rotating credentials, or redesigning tenant identity is outside scope and needs a
concrete follow-up.

## Completion

Complete when data flows and demonstrated fixes are reviewed, configured required
checks pass, and live acceptance is either evidenced or explicitly left incomplete.
Pass confirmed provider/artifact inventory observations to Plan 04; never include
credentials.

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
