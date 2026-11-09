# Plan 09 — SOP Vector and embedding weaknesses (LLM09)

Status: PROPOSED; not executed.
Date: 2026-11-08 (America/Chicago).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm09-vector-and-embedding-weaknesses.
Suggested repository path: docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md.

## Goal and scope

Verify retrieval isolation and lifecycle integrity for the backends SOP actually supports, including keyword/BM25/FTS retrieval where applicable. Do not introduce vector storage merely to match the category name.

Dependency: Plan 05 supplies ingestion/provenance findings; Plan 03 supplies disclosure boundaries. Reuse merged implementations. This plan owns query scope, cache scope, stale-result handling, and context delivery.

## Tasks

### RETRIEVE-001 — Map retrieval boundaries

Inspect ingestion, index selection, query construction, caches, reranking, context assembly, deletion/invalidation, and fallback. Enumerate supported project/workspace/task scopes. Do not invent tenant identity for a single-user system.

Acceptance:
- Record which identity is caller-controlled and which comes from trusted project binding.
- Trace scope from request to datastore query to returned context.
- Distinguish current content, historical advisory content, and authoritative execution evidence.
- Deliver docs/reports/owasp-llm/llm09-retrieval.md with supported backend/path matrix.

### RETRIEVE-002 — Test isolation and stale records

Create two isolated synthetic projects with overlapping document names and unique canaries. Use the actual retrieval interface for supported backends. Include a query favoring the other project's canary, forged metadata, stale/deleted content, reused cache keys, and a highly ranked malicious instruction.

Acceptance:
- Cross-project records cannot reach context under the current scope contract.
- User-supplied metadata cannot override trusted project binding.
- Cache keys and cache-hit validation preserve the same scope as uncached reads.
- Supported invalidation prevents obsolete records from being used as current evidence.
- Retrieved instructions remain untrusted data and cannot grant authority.
- Valid in-scope results still appear; optional failure and required-evidence failure retain their existing distinct behavior.
- Backend omissions are NOT_RUN or justified NOT_APPLICABLE, never inferred passes.

### RETRIEVE-003 — Verify rebuild and finish coverage

Exercise supported rebuild/reindex procedures on disposable data. Repair demonstrated scope/invalidation defects without adding a new database or changing retrieval ranking solely for security optics.

Acceptance:
- Rebuild preserves scope and does not silently import another workspace.
- Historical down-weighting is described as relevance behavior, not an authorization control.
- Focused tests and required checks pass; PR lists backend versions and exact verified paths.
- Report links Plan 05 provenance evidence and Plan 07 evidence-authority boundaries.

## Risks and completion

Similarity scores do not establish trust. Content-level poisoning can remain within an authorized scope; no semantic immunity is claimed. A shared tenant model or row-level-security redesign is a separate architecture decision. Complete only for tested backends and supported scopes; retain inaccessible-backend gaps.

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

