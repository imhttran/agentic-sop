# Plan 10 — SOP Improper output handling (LLM10)

Status: PROPOSED; not executed.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm10-improper-output-handling.
Suggested repository path: docs/plans/PLAN-10-SOP-LLM10-Improper-Output-Handling.md.

## Goal and scope

Treat model and tool output as untrusted at execution, persistence, and display sinks. Verify existing validation and encoding without creating a second command-policy layer.

Dependency: Plan 01 owns action authority; Plan 03 owns redaction; Plan 07 owns evidence claims. This plan owns sink-specific handling, including terminals and client rendering where SOP controls them.

## Tasks

### OUTPUT-001 — Inventory output sinks

Trace structured tool arguments, shell/subprocess construction, file/patch paths, database queries, templates, terminal output, Markdown/HTML rendering, notifications, and protocol errors. Record which sinks SOP owns and which belong to external clients.

Acceptance:
- Each sink lists input origin, parser, validation/encoding, effect, and policy owner.
- Identify intentional approved shell execution separately from accidental interpolation.
- Identify terminal control sequences and any automatic resource fetching in owned renderers.
- Deliver docs/reports/owasp-llm/llm10-output-handling.md.

### OUTPUT-002 — Repair demonstrated sink violations

Use inert execution recorders, disposable files/databases, and controlled render fixtures. Test metacharacters in data arguments, malformed structured calls, path traversal, terminal escape sequences, and remote-resource references only where a reachable sink exists.

Acceptance:
- Data arguments cannot become unintended shell syntax; approved shell behavior still follows existing authority.
- Invalid structured operations cannot partially execute.
- Supported file operations retain existing workspace/symlink boundaries.
- Database data stays parameterized at applicable sinks.
- Owned display paths cannot execute terminal controls or silently fetch attacker-selected resources contrary to their security contract.
- Legitimate formatting and valid authorized operations remain usable.
- Fixes use sink-specific encoding/validation; do not apply destructive global escaping to persisted structured data.

### OUTPUT-003 — Verify adapters and close the coverage matrix

Verify changed behavior through actual relevant CLI/MCP/ACP or notification serialization paths. Do not send live Slack messages for tests. External clients that cannot be controlled receive explicit limitations.

Acceptance:
- Protocol data remains valid; diagnostics do not corrupt machine-readable output.
- Synthetic canaries and dangerous sequences stay contained in tested paths.
- Required checks pass; PR records exact OS/client coverage.
- Update the existing cumulative OWASP report to include all ten category reports, preserving incomplete and historical evidence.
- A category is never marked fully covered merely because its plan or PR exists.

## Risks and completion

Shell-capable tools intentionally execute code; argument arrays alone do not constrain an interpreter. SOP cannot guarantee how every third-party terminal or IDE renders content. Complete for owned sinks with focused evidence; unsupported external rendering remains an explicit gap.

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

