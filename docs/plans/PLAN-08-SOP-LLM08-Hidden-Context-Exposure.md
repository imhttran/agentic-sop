# Plan 08 — SOP Hidden context exposure (LLM08)

Status: PROPOSED; not executed.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm08-hidden-context-exposure.
Suggested repository path: docs/plans/PLAN-08-SOP-LLM08-Hidden-Context-Exposure.md.

## Goal and scope

Assume system/developer prompts and tool schemas can be disclosed. Verify that secrecy of those texts is not required for authorization, and that protected values are excluded from model-visible context by existing controls.

Dependency: Plan 03 owns shared redaction and disclosure fixes; Plan 02 owns bounded live evaluation. Reuse both. This plan adds hidden-context inventory and authority-independence evidence, not another DLP layer.

## Tasks

### CONTEXT-001 — Inventory model-visible hidden context

Trace prompt assembly, tool descriptions, retrieved policy text, configuration interpolation, provider diagnostics, and debug/export paths. Classify public instructions, internal non-secret policy, secrets, and user/project-confidential content. Inspect names and synthetic values; never print actual credentials.

Acceptance:
- Distinguish transport authentication headers from message contents.
- Identify controls implemented in executable policy versus instructions only.
- Record whether raw prompts are persisted or exported.
- Deliver docs/reports/owasp-llm/llm08-hidden-context.md and route shared disclosure defects to Plan 03's owner.

### CONTEXT-002 — Verify exclusion and authority independence

Place synthetic protected canaries at supported configuration and prompt boundaries. Capture actual serialized requests and relevant artifacts with a fake provider. Supply known policy/tool text to an adversarial fixture and attempt an unauthorized operation against the real policy boundary.

Acceptance:
- Protected canaries do not enter unauthorized messages or artifacts under the existing contract.
- Knowing the instructions or tool schema does not bypass action authorization.
- Untrusted retrieved policy text cannot replace trusted configuration or approval state.
- Required provider authentication and ordinary context remain functional.
- Fixes reuse existing request construction/redaction/authorization seams; successful prompt nondisclosure alone is not a passing security criterion.

### CONTEXT-003 — Record bounded extraction evidence

Where Plan 02's supported local live seam exists, use a fixed campaign with synthetic context. Record what the model reveals and which effects the policy permits. If prerequisites are missing, complete independent tests and mark live acceptance NOT_RUN.

Acceptance:
- Report distinguishes disclosure of non-secret instructions from leakage of protected values.
- No claim that prompts are universally unextractable.
- Model outputs, artifacts, and tool effects have separately recorded outcomes.
- Focused and required checks pass; PR states exact live coverage or incomplete criteria.

## Risks and completion

Redaction cannot classify all confidential prose. Public knowledge of a schema is expected; schema secrecy is not access control. Complete the deterministic boundary work with evidence; live acceptance remains incomplete until observed. Provider retention policies and enterprise DLP are outside scope.

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

