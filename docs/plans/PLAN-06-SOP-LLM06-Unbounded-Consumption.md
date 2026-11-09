# Plan 06 — SOP Unbounded consumption (LLM06)

Status: PROPOSED; not executed.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm06-unbounded-consumption.
Suggested repository path: docs/plans/PLAN-06-SOP-LLM06-Unbounded-Consumption.md.

## Goal and scope

Verify that existing attempt, step, timeout, output, and provider limits bound an entire execution, including retries and resume. Repair bypasses without adding a second scheduler or changing budget policy.

Dependency: Plan 01's execution inventory. Reuse existing no-progress, cancellation, retry, and recovery work. No live model is needed for the core acceptance.

## Tasks

### BUDGET-001 — Map limits and their authority

Trace CLI/MCP/ACP entry points that exist, provider request construction, nested tool calls, retries, subprocesses, and resume. Record limit units, owner, default, override authority, enforcement point, persistence, and overshoot bound.

Acceptance:
- Distinguish per-call, per-attempt, per-job, and any shared limits actually supported.
- Distinguish elapsed time, tokens, output bytes, and monetary estimates.
- Identify model-controlled and caller-supplied arguments (CLI flags, MCP/ACP request fields, `SOP_TASK_INPUTS`) that can influence consumption, including output-token limits, sampling parameters, and step/retry counts.
- Deliver docs/reports/owasp-llm/llm06-consumption.md; missing token/spend accounting remains a gap, not an invented supported feature.

### BUDGET-002 — Reproduce and repair limit bypasses

Use a scripted provider, fake clock where supported, and controlled subprocess fixtures. Exercise the real scheduler/provider seams. Repair violations of existing configured bounds; do not silently choose new numeric limits.

Acceptance:
- Boundary cases just below, at, and above each changed limit match the documented contract.
- Generated or caller-supplied parameters cannot increase operator-owned limits; where a request carries an output-token or similar consumption value, the operator-owned value is enforced before dispatch, not just recorded.
- Budgets are counted in tokens where the provider reports usage, not only in requests; a chain whose step count or token use crosses its limit stops (circuit breaker) instead of continuing.
- Retries and continuations cannot reset an aggregate budget where the contract defines one.
- Resume retains relevant consumed budget; concurrent accounting cannot overspend the same reservation where supported.
- Cancellation/timeout stops owned work within the existing bound and preserves evidence.
- Oversized/streamed output is bounded at the read boundary, not only after full allocation.
- A normal bounded task still completes.

### BUDGET-003 — Verify terminal behavior and report limits

Confirm budget exhaustion produces one actionable terminal/blocking result through existing lifecycle semantics, without retrying unchanged work. Inspect usage fields for absent/partial provider accounting.

Acceptance:
- Missing token usage is unknown, not zero; estimates are labeled.
- No hard dollar-cap claim is made without enforceable pre-dispatch reservation and known pricing/usage bounds.
- Report any per-call overshoot and unsupported provider behavior.
- Focused and required checks pass; PR documents actual bounds and residual gaps.

## Risks and completion

A timeout may not stop provider-side billing; local cancellation is not a refund. Subprocess descendants and restart paths need evidence on the named OS. New global quotas or pricing services require a separate policy/design decision. Complete when existing limits are verified and demonstrated defects repaired, with missing controls explicitly retained.

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

