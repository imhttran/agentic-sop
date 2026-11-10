# Plan 03 — SOP sensitive information disclosure (LLM02)

Status: HISTORICALIZED / COMPLETE (external completion, 2026-11-09). The original
planning status was "STATIC TRACING IMPLEMENTED; LIVE `NOT_RUN`; historical
outcomes PARTIAL (preserved). Plan 02 live-test infrastructure is an external
prerequisite." The live campaign is now OBSERVED in
[llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md); no per-criterion
acceptance is recorded, and the historical `NOT_RUN` record is preserved in that
report. See the dependency reconciliation below.
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
- DISCLOSE-003 (live extraction) depended on Plan 02's merged live-test
  infrastructure. At this planning baseline that infrastructure was not present,
  so the task was `NOT_RUN` in this workstream. The follow-up campaign later ran
  on the merged seam and is OBSERVED in
  [llm02-disclosure.md](../reports/owasp-llm/llm02-disclosure.md). No competing
  live-test implementation was created.
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

- Distinguish intentional provider authentication headers from model-visible context.
- Identify where redaction occurs before sending, before persistence, and before display.
- Record scope checks before retrieval and response validation after retrieval.
- Distinguish credentials from other confidential source content; pattern matching is not general DLP.
- Do not read or print actual secret values during assessment.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm02-disclosure.md §Provider authentication is not model content and §Data-flow trace provider row
- criterion 2: manual: docs/reports/owasp-llm/llm02-disclosure.md §Where redaction happens
- criterion 3: manual: docs/reports/owasp-llm/llm02-disclosure.md §Authorization boundaries and §Data-flow trace retrieval row; `TestSearchRejectsUnrelatedEvidence`, `TestRankerAdapterRejectsScopeItCannotHonor`
- criterion 4: manual: docs/reports/owasp-llm/llm02-disclosure.md §Residual risks and bounded claims, row "Pattern matching", and §Data-flow trace
- criterion 5: manual: docs/reports/owasp-llm/llm02-disclosure.md §Method; only synthetic canaries were used

### Execution

- verify-first — the static trace report is present in the working tree; the configured validation proves the repository still builds.

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

- Protected canaries do not appear in unauthorized model messages, persisted diagnostics, or outgoing payloads at in-scope boundaries.
- Cross-scope retrieval content is rejected before it can become model context under the current retrieval contract.
- Legitimate non-sensitive content and required authentication still work.
- Each fix reuses the shared redaction or established authorization boundary where appropriate.
- Unknown formats and arbitrary confidential text remain explicitly bounded claims.

### Acceptance Checks

- `go test ./internal/cli ./internal/agent -count=1`
- `go test ./internal/retrievalboundary ./internal/memindex ./internal/handoff ./internal/notify ./internal/acp ./internal/controller/sopclient ./internal/outcomefile -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 1
- criterion 4: manual: `internal/cli/redaction.go` `redactDiagnosticText` reuses `handoff.Redact` and `redact.ShapesValues`; docs/reports/owasp-llm/llm02-disclosure.md §Reproduced gaps and fixes
- criterion 5: manual: docs/reports/owasp-llm/llm02-disclosure.md §Residual risks and bounded claims, row "Pattern matching"

### Execution

- verify-first — the fix and its canary regressions are present in the working tree; the package tests and the configured validation prove them.

## DISCLOSE-003 — Exercise live extraction and finish review

Reuse Plan 02's bounded local evaluation path for requests to reveal synthetic
secrets or transmit protected content. Observe actual outputs and attempted
effects, not an LLM judge alone. This task remains unexecuted until the
prerequisite below exists; it adds no competing live-test implementation.

### Dependencies

- DISCLOSE-002

### Prerequisites

- Plan 02 merged live-test infrastructure: an evaltarget-routed live path whose observer reads back a model tool decision, plus the pinned binary and a reachable local model.

### Deliverables

- `docs/reports/owasp-llm/llm02-disclosure.md`

### Acceptance Criteria

- Direct extraction and an indirect injected extraction case have recorded outcomes.
- Unauthorized tool effects remain denied even if the model proposes disclosure.
- No real credential or external receiver is used.
- Report separates prompt exposure, generated disclosure, artifact leakage, and attempted exfiltration.
- Unavailable live prerequisites produce NOT_RUN and leave the relevant acceptance incomplete.
- Required verification passes and a focused PR records changed paths and residual risks.

### Acceptance Mapping

> Historical planning mapping. The live campaign is now OBSERVED; this text was
> not reconciled through SOP after historicalization and is not acceptance
> evidence. See
> [llm02-acceptance-request.md](../reports/owasp-llm/llm02-acceptance-request.md).

- criterion 1: manual: live run artifact recording direct and indirect extraction outcomes (currently NOT_RUN; Plan 02 prerequisite missing)
- criterion 2: manual: live run artifact recording denied unauthorized effects (currently NOT_RUN)
- criterion 3: manual: live harness fixture uses only synthetic canaries and no real receiver (currently NOT_RUN)
- criterion 4: manual: docs/reports/owasp-llm/llm02-disclosure.md §Live extraction separates prompt exposure, generated disclosure, artifact leakage, and attempted exfiltration (currently NOT_RUN)
- criterion 5: manual: docs/reports/owasp-llm/llm02-disclosure.md §Live extraction records NOT_RUN and the Plan 02 prerequisite
- criterion 6: manual: `sop validate` PASS and the PR diff/residual-risk summary; live acceptance remains incomplete

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
