# Plan 09 — SOP Vector and embedding weaknesses (LLM09)

Status: ADAPTED TO THE SOP PLAN SCHEMA; not executed.
Date: 2026-11-08 (America/Chicago); schema adaptation 2026-11-09 (UTC).
Series: next six plans after Plans 01–04; numbering follows category order, not mandatory execution order.
Suggested branch: security/llm09-retrieval.
Suggested repository path: docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md.

## Project

agentic-sop

## Summary

Verify retrieval isolation and lifecycle integrity for the backends SOP actually supports,
including keyword/BM25/FTS retrieval where applicable. Do not introduce vector storage
merely to match the category name. This plan owns query scope, cache scope, stale-result
handling, and context delivery. Plan 05 owns ingestion provenance; Plan 03 owns disclosure
boundaries; Plan 07 owns evidence authority. Reuse their merged implementations and
evidence, and record their actual status rather than assuming a merge.

## Goal and scope

Verify retrieval isolation and lifecycle integrity for the backends SOP actually supports, including keyword/BM25/FTS retrieval where applicable. Do not introduce vector storage merely to match the category name.

Dependency: Plan 05 supplies ingestion/provenance findings; Plan 03 supplies disclosure boundaries. Reuse merged implementations. This plan owns query scope, cache scope, stale-result handling, and context delivery.

### Dependency reconciliation

- Plan 05 (source acceptance and provenance) is merged in this baseline (PR #268, follow-up PR #274). The merged abstractions are `internal/outcomefile` (`IngestRunFor`, content identity, eligibility, source-aware gate) and `internal/memindex` (`RebuildFromDir`, drift). POISON-001/003 remain `NOT_RUN`; POISON-002 is implemented. This plan reuses the gate and does not re-author ingestion provenance.
- Plan 03 (disclosure boundaries) is merged for static tracing (PR #266). Its report already records retrieval corpus/repository/task scope and the untrusted-evidence boundary. Live extraction remains `NOT_RUN` and is not required for this plan.
- Plan 07 (evidence authority) is `NOT_RUN` (plan `PROPOSED`). Evidence authority in this baseline is the existing outcome eligibility/verification/content-hash contract and the retrieval boundary. This plan records the Plan 07 handoff; it does not invent a new authority model.
- The task IDs and every acceptance criterion from the planning document are preserved. The structured sub-sections below are the SOP schema adaptation; they do not weaken or remove planning scope.

## RETRIEVE-001 — Map retrieval boundaries

Inspect ingestion, index selection, query construction, caches, reranking, context assembly, deletion/invalidation, and fallback. Enumerate supported project/workspace/task scopes. Do not invent tenant identity for a single-user system.

Record the trace in the plan's report deliverable. Distinguish current content, historical advisory content, and authoritative execution evidence. Include keyword/BM25/FTS retrieval where applicable; identify vector/embedding evaluation surfaces as evaluation-only when no vector retriever is wired into production. Name which identity is caller-controlled and which comes from trusted project binding, and trace scope from request to datastore query to returned context.

### Dependencies

None

### Deliverables

- `docs/reports/owasp-llm/llm09-retrieval.md` — supported backend and path matrix, scope trace, cache and invalidation trace, context-delivery boundary, and explicit backend omissions.

### Acceptance Criteria

- Record which identity is caller-controlled and which comes from trusted project binding.
- Trace scope from request to datastore query to returned context.
- Distinguish current content, historical advisory content, and authoritative execution evidence.
- Deliver docs/reports/owasp-llm/llm09-retrieval.md with supported backend/path matrix.

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm09-retrieval.md §Scope and trusted binding
- criterion 2: manual: docs/reports/owasp-llm/llm09-retrieval.md §Scope trace
- criterion 3: manual: docs/reports/owasp-llm/llm09-retrieval.md §Content classes and authority
- criterion 4: manual: docs/reports/owasp-llm/llm09-retrieval.md §Backend coverage matrix

## RETRIEVE-002 — Test isolation and stale records

Create two isolated synthetic projects with overlapping document names and unique canaries. Use the actual retrieval interface for supported backends. Include a query favoring the other project's canary, forged metadata, stale/deleted content, reused cache keys, and a highly ranked malicious instruction.

Reuse the local SQLite FTS5 advisory retriever, the bounded retrieval boundary, the RAG client, and the existing outcome-file gate. Do not add a new database, tenant model, permission layer, or retrieval ranking rule. Fix only demonstrated scope or invalidation defects in the existing contracts.

While completing the isolation and stale-record evidence, also correct the RETRIEVE-001 report's stale line references and the review-identified unverified claims: complete the code-context trace, distinguish IMPLEMENTED, ENABLED, and VERIFIED for each backend, substantiate or qualify the cache-absence claim, and document the required-evidence timeout interaction. These are report corrections inside the existing RETRIEVE-001 deliverable, not new gates or scope.

The demonstrated defects at the current baseline are:

- `internal/cli/advisory_retrieval.go` `mapAdvisoryResult` checks the evidence corpus but not the evidence repository, so an injected or faulty backend can return cross-repository evidence that the mapping layer accepts.
- `internal/cli/advisory_retrieval.go` `retrieveAdvisory` builds a query with an empty repository filter when its caller passes no repository, so an unscoped read is possible at the exported advisory seam even though production derives `cfg.Project.Name`.
- `internal/memindex` `LocalRetriever` uses `SnapshotContext` for its current-source check, which does not apply the source-aware eligibility gate used by `RebuildFromDir`; a modified or deleted source artifact therefore remains retrievable until a manual rebuild.

### Dependencies

RETRIEVE-001

### Deliverables

- `internal/memindex/rebuild.go` — a read-path current-source snapshot that applies the existing `outcomefile.GateWithSourcesAndDigests` gate, so modified or deleted source artifacts are excluded even before a rebuild.
- `internal/cli/advisory_retrieval.go` — drop evidence whose repository differs from the trusted query repository, and fail closed with a bounded reason when the trusted repository is empty.
- `internal/memindex/llm09_test.go` — two-project canary tests through the real local retriever for cross-project scope, forged content metadata, stale/modified/deleted source, rebuild scope, shared document names, and a valid in-scope control.
- `internal/cli/llm09_retrieval_test.go` — cross-repository evidence rejection, empty-repository fail-closed, malicious retrieved instruction labelling, valid in-scope delivery, and the distinct optional/required failure behavior.
- `internal/ragclient/llm09_test.go` — two-corpus canary test proving a producer response from the other corpus is rejected before evidence reaches SOP.
- `internal/cli/advisory_retrieval_test.go` — update existing fake evidence to carry its trusted repository.
- `docs/reports/owasp-llm/llm09-retrieval.md` — isolation and stale-record evidence section.
- `docs/reports/owasp-llm/llm09-retrieval.md` — corrected references and completed code-context trace; separate IMPLEMENTED, ENABLED, and VERIFIED statements; substantiated cache claim; required-evidence timeout interaction.

### Acceptance Criteria

- Cross-project records cannot reach context under the current scope contract.
- User-supplied metadata cannot override trusted project binding.
- Cache keys and cache-hit validation preserve the same scope as uncached reads.
- Supported invalidation prevents obsolete records from being used as current evidence.
- Retrieved instructions remain untrusted data and cannot grant authority.
- Valid in-scope results still appear; optional failure and required-evidence failure retain their existing distinct behavior.
- Backend omissions are NOT_RUN or justified NOT_APPLICABLE, never inferred passes.

### Acceptance Checks

- `go test ./internal/memindex -run 'TestLLM09' -count=1`
- `go test ./internal/cli -run 'TestLLM09' -count=1`
- `go test ./internal/ragclient -run 'TestLLM09' -count=1`
- `go test ./internal/cli -run 'TestRAGConcurrentTasksIsolated|TestRequiredEvidenceMissingBlocks|TestRAGOptionalFailureAgentInput' -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 4
- criterion 4: check 1
- criterion 5: check 2
- criterion 6: check 2
- criterion 7: manual: docs/reports/owasp-llm/llm09-retrieval.md §Backend coverage matrix

## RETRIEVE-003 — Verify rebuild and finish coverage

Exercise supported rebuild/reindex procedures on disposable data. Repair demonstrated scope/invalidation defects without adding a new database or changing retrieval ranking solely for security optics.

Record before/after rebuild evidence, the exact commands and results, and the platform. Describe historical down-weighting in `internal/retrieval` as relevance behavior, not an authorization control. Link Plan 05 provenance evidence and the Plan 07 evidence-authority handoff.

Finish the report corrections started in RETRIEVE-002: verified references, a complete code-context trace, explicit IMPLEMENTED/ENABLED/VERIFIED dimensions, and the required-evidence timeout and cache claims. Do not add new acceptance criteria, gates, or scope.

### Dependencies

- RETRIEVE-001
- RETRIEVE-002

### Deliverables

- `docs/reports/owasp-llm/llm09-retrieval.md` — rebuild/reindex evidence, historical down-weighting explanation, criterion-to-evidence mapping, exact commands/results, platform, residual limitations, and dependency handoffs; verified references, complete code-context trace, and explicit IMPLEMENTED/ENABLED/VERIFIED dimensions.
- `internal/memindex/llm09_test.go` — rebuild scope regression coverage on disposable fixtures.

### Acceptance Criteria

- Rebuild preserves scope and does not silently import another workspace.
- Historical down-weighting is described as relevance behavior, not an authorization control.
- Focused tests and required checks pass; PR lists backend versions and exact verified paths.
- Report links Plan 05 provenance evidence and Plan 07 evidence-authority boundaries.

### Acceptance Checks

- `go test ./internal/memindex -run 'TestLLM09' -count=1`
- `go test ./internal/retrieval -run 'TestCandidatesFromIndexFullTextAndHistory' -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: manual: docs/reports/owasp-llm/llm09-retrieval.md §Verification records the exact commands, results, backend versions, platform, and base/head revisions; the PR body repeats the backend versions and verified paths
- criterion 4: manual: docs/reports/owasp-llm/llm09-retrieval.md §Dependency handoff links Plan 05 provenance evidence and the Plan 07 evidence-authority boundary

## Risks and completion

Similarity scores do not establish trust. Content-level poisoning can remain within an authorized scope; no semantic immunity is claimed. A shared tenant model or row-level-security redesign is a separate architecture decision. Complete only for tested backends and supported scopes; retain inaccessible-backend gaps.

Retrieved content cannot grant approval or replace authoritative verification. Ranking and historical down-weighting are relevance controls, not authorization controls. Optional retrieval failure and required-evidence failure keep their existing distinct behavior. Do not invent tenant identity, introduce a new database, change retrieval defaults, or expand permissions. Preserve authoritative history and user indexes; perform no destructive cleanup or production writes.

## Execution contract

Planning and schema adaptation only; no implementation, SOP ingestion, configuration change, or installation has occurred by writing this document.

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
- Earlier plans: PLAN-01-SOP-LLM03-Excessive-Agency.md; PLAN-02-SOP-LLM01-Prompt-Injection.md; PLAN-03-SOP-LLM02-Sensitive-Information-Disclosure.md; PLAN-04-SOP-LLM04-Supply-Chain.md; PLAN-05-SOP-LLM05-Data-and-Model-Poisoning.md; PLAN-07-SOP-LLM07-Misinformation.md.
- At execution, inspect current docs/specs/SECURITY.md, docs/specs/QUALITY.md, docs/specs/RAG-SECURITY-THREAT-MODEL.md, configuration, and relevant reports before relying on historical assertions.

Final handoff: changed behavior, criterion-to-evidence mapping, PR link, residual gaps, blocked criteria, and exact next action. Proposed report paths are deliverables, not claims that files already exist.
