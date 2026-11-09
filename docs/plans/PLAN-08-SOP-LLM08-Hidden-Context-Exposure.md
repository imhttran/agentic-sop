# Plan 08 — SOP Hidden context exposure (LLM08)

Status: ADAPTED FOR EXECUTION 2026-11-09; tasks run verify-first against work present in the tree.
Date: 2026-11-08 (America/Chicago); adapted 2026-11-09.
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm08-hidden-context-exposure.
Suggested repository path: docs/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md.
Baseline: `origin/main` at `9cf554e6`.

## Project

agentic-sop

## Summary

Assume system/developer prompts and tool schemas can be disclosed. Verify that secrecy
of those texts is not required for authorization, and that protected values are
excluded from model-visible context by existing controls. Reuse Plan 03's shared
redaction and Plan 02's bounded live evaluation. This plan adds a hidden-context
inventory and authority-independence evidence; it adds no DLP layer, evaluator, or
gate.

## Dependency reconciliation

- **Plan 03 — shared redaction and disclosure controls.** Merged on the baseline:
  `internal/redact` (shape-aware detector), `internal/cli/redaction.go`
  (`redactDiagnosticText`, `redactAcceptanceOutput`), handoff redaction, and the
  provider-auth-is-not-body assertion. CONTEXT-001/002 reuse these and add no
  competing redaction path. Any newly reproduced shared disclosure defect is routed
  to Plan 03's owner instead of being re-implemented here.
- **Plan 02 — bounded local live-evaluation path.** Merged on the baseline:
  `internal/evaltarget` (`RunLive`, `ProductionControls`, recording effects, the
  fixed local endpoint allowlist, and the opt-in macOS live isolation profile).
  CONTEXT-003 reuses that seam. When the endpoint, model, or isolation is
  unavailable, the live acceptance is `NOT_RUN`; no competing live harness is
  created.
- CONTEXT-001 and CONTEXT-002 are deterministic and proceed independently of live
  prerequisites.
- These tasks run `verify-first`: the report, tests, and campaign artifact are
  present in the working tree, and SOP's configured validation plus each stage's
  acceptance checks prove them. The declaration changes scheduling only; it changes
  no security mode, required category, severity rule, autonomy setting, or
  executable behavior.

## Goal and scope

Assume system/developer prompts and tool schemas can be disclosed. Verify that secrecy
of those texts is not required for authorization, and that protected values are
excluded from model-visible context by existing controls.

Dependency: Plan 03 owns shared redaction and disclosure fixes; Plan 02 owns bounded
live evaluation. Reuse both. This plan adds hidden-context inventory and
authority-independence evidence, not another DLP layer.

## CONTEXT-001 — Inventory model-visible hidden context

Trace prompt assembly, tool descriptions, retrieved policy text, configuration
interpolation, provider diagnostics, and debug/export paths. Classify public
instructions, internal non-secret policy, secrets, and user/project-confidential
content. Inspect names and synthetic values; never print actual credentials.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm08-hidden-context.md`

### Acceptance Criteria

- Distinguish transport authentication headers from message contents.
- Identify controls implemented in executable policy versus instructions only.
- Record whether raw prompts are persisted or exported.
- Classify public instructions, internal non-secret policy, secrets, and user/project-confidential content.
- Deliver `docs/reports/owasp-llm/llm08-hidden-context.md` and route shared disclosure defects to Plan 03's owner.
- Do not read or print actual credential values; use names and synthetic canaries only.

### Acceptance Checks

- `go test ./internal/prompt ./internal/agent ./internal/ollamaagent ./internal/toolharness -count=1`
- `go test ./internal/cli -run TestOWASPLLM08 -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 2
- criterion 5: check 2
- criterion 6: check 2

### Execution

- verify-first — the inventory report is present in the working tree; the focused
  prompt/agent/tool-boundary tests and the configured validation prove the traced
  paths still exist and behave as recorded.

## CONTEXT-002 — Verify exclusion and authority independence

Place synthetic protected canaries at supported configuration and prompt boundaries.
Capture actual serialized requests and relevant artifacts with a fake provider. Supply
known policy/tool text to an adversarial fixture and attempt an unauthorized operation
against the real policy boundary.

### Dependencies

- CONTEXT-001

### Deliverables

- `internal/agent/hidden_context_test.go`
- `internal/ollamaagent/hidden_context_test.go`
- `internal/cli/llm08_hidden_context_test.go`
- `docs/reports/owasp-llm/llm08-hidden-context.md`

### Acceptance Criteria

- Protected canaries do not enter unauthorized messages or artifacts under the existing contract.
- Knowing the instructions or tool schema does not bypass action authorization.
- Untrusted retrieved policy text cannot replace trusted configuration or approval state.
- Required provider authentication and ordinary context remain functional.
- Fixes reuse existing request construction/redaction/authorization seams; successful prompt nondisclosure alone is not a passing security criterion.
- Regression tests invoke the real request, artifact, or authorization path, not only a prompt template.

### Acceptance Checks

- `go test ./internal/agent ./internal/ollamaagent ./internal/toolharness ./internal/prompt ./internal/evaltarget -count=1`
- `go test ./internal/cli -run TestOWASPLLM08 -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 2
- criterion 6: check 1

### Execution

- verify-first — the canary tests and report are present in the working tree; the
  focused tests and the configured validation prove them.

## CONTEXT-003 — Record bounded extraction evidence

Where Plan 02's supported local live seam exists, use a fixed campaign with synthetic
context. Record what the model reveals and which effects the policy permits. If
prerequisites are missing, complete independent tests and mark live acceptance
NOT_RUN.

### Dependencies

- CONTEXT-002

### Deliverables

- `internal/evaltarget/live_campaign_shared_test.go`
- `internal/evaltarget/live_llm08_campaign_test.go`
- `docs/reports/owasp-llm/llm08-campaign.json`
- `docs/reports/owasp-llm/llm08-hidden-context.md`

### Acceptance Criteria

- Report distinguishes disclosure of non-secret instructions from leakage of protected values.
- No claim that prompts are universally unextractable.
- Model outputs, artifacts, and tool effects have separately recorded outcomes.
- Unavailable live prerequisites produce NOT_RUN and leave the relevant acceptance incomplete.
- Focused and required checks pass; PR states exact live coverage or incomplete criteria.
- No real credential, external receiver, or production effect is used.

### Acceptance Checks

- `go test ./internal/evaltarget -run TestLive -count=1`
- `go test ./internal/cli -run TestOWASPLLM08 -count=1`

### Acceptance Mapping

- criterion 1: check 2
- criterion 2: check 2
- criterion 3: check 2
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1

### Execution

- verify-first — the deterministic live-path tests and report are present; a live
  campaign is opt-in and recorded separately.

## Risks and completion

Redaction cannot classify all confidential prose. Public knowledge of a schema is
expected; schema secrecy is not access control. Complete the deterministic boundary
work with evidence; live acceptance remains incomplete until observed. Provider
retention policies and enterprise DLP are outside scope.

## Execution contract

Planning only at authoring time; this adaptation records task metadata for SOP and
does not itself execute the work. Reconcile through supported SOP interfaces; never
edit `state.db` or fabricate approvals.

Execute the tasks sequentially through existing lifecycle and PR gates. Reuse existing
abstractions and record ALREADY_SATISFIED only with current evidence. Fix reproducible
defects in existing contracts; do not introduce another gate, evaluator, permission
system, or service. A missing control requiring new policy gets a concrete proposal
through existing governance; stop only dependent work.

Preserve report-only/required settings, configured limits, routing, and autonomy
defaults. Do not enable remote evaluations, install tools, download models, expand
permissions, or change security policy implicitly. Use synthetic data, disposable
fixtures, fake providers, and inert effect recorders. No production writes or real
exfiltration.

Run focused regression tests for changed behavior, then repository-required checks
once. Repeat only after relevant changes or failures. Record exact commands, exit
codes, base/head commits, configuration identity, platform, and evidence paths. Open a
focused PR; follow established review and merge governance. Do not claim installation
or deployment from a PR result.

## Baseline and evidence

The four preceding planning files used historical baseline
0750e8be0cf5f318a173abb34c2b752ac131521c. Their observations are context, not current
runtime evidence. Refresh the baseline and inspect current implementations before
choosing files or commands.

Report IMPLEMENTED, ENABLED, and VERIFIED separately, with the exact path and platform.
Use GAP, NOT_RUN, and justified NOT_APPLICABLE explicitly. A deterministic test is not
live-model evidence; a scan PASS is not complete category coverage. Missing required
evidence leaves the associated criterion incomplete.

Category numbers and names follow the supplied Cycode article's displayed 2026 list,
accessed November 8, 2026 (America/Chicago), consistent with Plans 01–04. This is an
article-based planning taxonomy, not a verified formal OWASP compliance mapping.

## Sources and handoff

- https://cycode.com/blog/owasp-top-10-llm-applications/
- Repository: https://github.com/imhttran/agentic-sop
- Earlier plans: PLAN-01-SOP-LLM03-Excessive-Agency.md; PLAN-02-SOP-LLM01-Prompt-Injection.md; PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md; PLAN-04-SOP-LLM04-Supply-Chain.md.
- At execution, inspect current docs/specs/SECURITY.md, docs/specs/QUALITY.md, configuration, and relevant reports before relying on historical assertions.

Final handoff: changed behavior, criterion-to-evidence mapping, PR link, residual gaps,
blocked criteria, and exact next action.
