# Plan 09B — SOP LLM09 retrieval follow-up (import scope, rebuild evidence, producer boundary)

Status: PROPOSED for SOP execution; not merged.
Date: 2026-11-09 (America/Chicago).
Series: follow-up to Plan 09 (LLM09 vector and embedding weaknesses).
Suggested branch: security/llm09-retrieval-followup.
Suggested repository path: docs/plans/PLAN-09B-SOP-LLM09-Retrieval-Followup.md.

## Project

agentic-sop

## Summary

Reconcile the remaining LLM09 retrieval-boundary evidence without re-enabling RAG,
changing security defaults, or adding a tenant/RLS architecture. The prior
`RETRIEVE-004`..`RETRIEVE-006` state referenced in an earlier session is not persisted
in this repository, any git ref, or any known agentic-sop `state.db`; this plan uses
distinct task identifiers and does not recreate or claim that missing acceptance record.

The concrete unresolved gaps are:

- The report's rebuild claim conflated two properties. `RebuildFromDir` imports every
  eligible record in the outcomes directory it is given and does not filter by
  repository; `Index.Search` applies the trusted repository at query time. Query
  filtering alone does not prove import isolation.
- Before/after rebuild evidence for modified and deleted sources is split across tests
  and lacks an explicit valid-historical positive control in the LLM09 fixture family.
- The RAG client verifies a producer response after it is returned; producer-side
  corpus isolation is not verified. The live producer and database are unavailable in
  this environment, so that evidence is `NOT_RUN`, not inferred.
- The report's authority, acceptance-mapping, and Plan 05/07 dependency statements need
  to match the current merged evidence.

## Goal and scope

Correct the rebuild import-versus-query claims, record synthetic before/after rebuild
evidence with positive controls, verify producer-side corpus isolation where the
supported backend is available (otherwise record `NOT_RUN` with the exact dependency),
and reconcile authority claims, acceptance mappings, dependency status, and the OWASP
evidence matrix.

This plan does not enable RAG, change retrieval or security defaults, add a database,
tenant model, permission layer, or new ranking rule, implement per-row digest or RAG
hash proposals, or claim semantic poisoning immunity or production vector coverage.

### Dependency reconciliation

- Plan 05 (source acceptance and provenance) is merged through PRs #268, #274, #288,
  #292, #308, and #310. Reuse `internal/outcomefile` (`IngestRunFor`, content identity,
  eligibility, `GateWithSourcesAndDigests`) and `internal/memindex` (`RebuildFromDir`).
  POISON-002 is verified; the POISON-001 trace was accepted; POISON-003 recovery is
  verified. Per-row digest and RAG chunk-hash decisions remain open.
- Plan 07 (evidence authority) is merged in PR #285 and its report records
  `IMPLEMENTED` / `ENABLED` / `VERIFIED` for deterministic evidence decisions on macOS
  arm64, with live-model and Linux `NOT_RUN`. Consume its evidence-authority boundary;
  do not add a new authority model.
- Plan 03 (disclosure boundaries) static tracing is merged in PR #266; live extraction
  remains `NOT_RUN`.
- Reuse the merged retrieval cancellation/deadline coverage from PR #298. Add new tests
  only where the gaps above require them.

## RETRIEVE-101 — Separate rebuild import scope from query scope

Measure imported index rows separately from rows returned by a scoped query. Prove that
a shared outcomes directory imports both repositories' rows while a scoped query
returns only the trusted repository, and that a workspace-local outcomes directory
imports only that workspace's rows. Correct the report so it no longer claims that a
rebuild silently fails to import another workspace.

### Dependencies

None

### Deliverables

- `internal/memindex/llm09_test.go` — `TestLLM09RebuildImportVersusQueryIsolation`, asserting `RebuildReport.Indexed` and `Index.Status.Records` for a shared directory and a workspace-local directory, plus the scoped-query result.
- `docs/reports/owasp-llm/llm09-retrieval.md` — corrected import-versus-query section and acceptance mapping.

### Acceptance Criteria

- The test measures imported rows directly, not only query results.
- A shared outcomes directory imports the cross-repository row and the scoped query does not return it.
- A workspace-local outcomes directory imports only its own row.
- The report withdraws the earlier import-isolation claim and states the supported layout.

### Acceptance Checks

- `go test ./internal/memindex -run TestLLM09RebuildImportVersusQueryIsolation -count=1`
- `go test ./internal/memindex -run TestLLM09RebuildScopeParity -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: manual: docs/reports/owasp-llm/llm09-retrieval.md §Rebuild and invalidation evidence

## RETRIEVE-102 — Record before/after rebuild evidence with positive controls

Exercise a synthetic three-record lifecycle: one record stays valid, one source is
modified, and one source is deleted. Prove that the gated read path excludes the stale
records before a rebuild, that the rebuild reports one indexed and two excluded
records, and that the valid historical record remains retrievable before and after the
rebuild.

### Dependencies

- RETRIEVE-101

### Deliverables

- `internal/memindex/llm09_test.go` — `TestLLM09RebuildModifiedAndDeletedSourceWithHistoricalControl`.
- `docs/reports/owasp-llm/llm09-retrieval.md` — before/after table, exact command, result, platform, and positive control.

### Acceptance Criteria

- Both the read-path before-rebuild state and the after-rebuild state are recorded.
- The modified and deleted sources are excluded in both states.
- The valid historical record remains retrievable as the positive control.
- The exact focused command and observed result are recorded.

### Acceptance Checks

- `go test ./internal/memindex -run TestLLM09RebuildModifiedAndDeletedSourceWithHistoricalControl -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: manual: docs/reports/owasp-llm/llm09-retrieval.md §Before/after rebuild evidence and §Verification record

## RETRIEVE-103 — Reconcile producer-side isolation and authority claims

Distinguish client-side response validation from producer-side corpus isolation. Test
producer-side isolation with two disposable corpora only if the supported live producer,
database, and embedding service are available within approved scope; otherwise record
`NOT_RUN` with the exact missing dependency. State precisely what the authority tests
demonstrate about approvals, required evidence, and authoritative state, and preserve
the untested model-influence limitation.

### Dependencies

- RETRIEVE-101

### Deliverables

- `docs/reports/owasp-llm/llm09-retrieval.md` — §RETRIEVE-103 producer-boundary and authority evidence records the client/producer split, the exact commands and observed results, the authority limits, and the `NOT_RUN` dependency record.
- `internal/ragclient/producer_acceptance_test.go` — tagged `ragacceptance` single-corpus contract and allowlist test; it does not establish cross-corpus producer isolation. No mock result is recorded as live evidence.

### Acceptance Criteria

- Client-side mixed-corpus rejection is stated separately from producer-side query scoping.
- Producer-side isolation is either verified with two disposable corpora and recorded revisions, or recorded `NOT_RUN` with the exact dependency.
- The authority section states what the tests prove and what they do not prove, including model influence.
- No production retrieval is enabled and no mock result is presented as live evidence.

### Acceptance Checks

- `go test ./internal/ragclient -run TestLLM09CrossCorpusCanaryRejected -count=1`
- `go test ./internal/prompt -run TestPromptKnowledgeBoundaryPresent -count=1`
- `go test ./internal/prompt -run TestPromptMaliciousDelimiterCannotTerminate -count=1`
- `go test ./internal/cli -run TestLLM09MaliciousInstructionIsUntrustedData -count=1`
- `go test ./internal/cli -run TestLLM09OptionalFailureAndRequiredSkipAreDistinct -count=1`
- `go test ./internal/cli -run TestRequiredEvidenceMissingBlocks -count=1`

### Acceptance Mapping

- criterion 1: manual: docs/reports/owasp-llm/llm09-retrieval.md §RETRIEVE-103 producer-boundary and authority evidence
- criterion 2: manual: docs/reports/owasp-llm/llm09-retrieval.md §RETRIEVE-103 producer-boundary and authority evidence
- criterion 3: manual: docs/reports/owasp-llm/llm09-retrieval.md §Content classes and authority
- criterion 4: manual: docs/reports/owasp-llm/llm09-retrieval.md §RETRIEVE-103 producer-boundary and authority evidence

## RETRIEVE-104 — Reconcile acceptance mappings, dependency status, and the OWASP matrix

Correct the original RETRIEVE-003 criterion mapping where it overstated import
isolation. Refresh Plan 05 and Plan 07 dependency status from their merged reports and
the OWASP evidence matrix. Update the Plan 09 row without claiming complete OWASP
coverage, semantic immunity, production vector coverage, or live producer evidence.

### Dependencies

- RETRIEVE-101
- RETRIEVE-102
- RETRIEVE-103

### Deliverables

- `docs/reports/owasp-llm/llm09-retrieval.md` — corrected criterion mapping, dependency handoff, residual limitations, and verification record.
- `docs/reports/owasp-llm/README.md` — refreshed Plan 09 row and Plan 05/07 status wording where it is inconsistent with current evidence.

### Acceptance Criteria

- The original RETRIEVE-003 criterion 1 is marked partial/corrected rather than verified.
- Plan 05 and Plan 07 are described from their current merged reports, not from the earlier proposed state.
- The OWASP matrix distinguishes local verification from `NOT_RUN` producer-side evidence.
- No complete-category, semantic-immunity, or production-vector claim is added.

### Acceptance Checks

- `go test ./internal/retrieval -run TestCandidatesFromIndexFullTextAndHistory -count=1`
- `go test ./internal/memindex -run TestLLM09RebuildImportVersusQueryIsolation -count=1`
- `go test ./internal/memindex -run TestReindexProvenanceBeforeAndAfterRebuild -count=1`

### Acceptance Mapping

- criterion 1: manual: §Criterion-to-evidence mapping
- criterion 2: manual: §Dependency handoff and docs/reports/owasp-llm/README.md
- criterion 3: manual: docs/reports/owasp-llm/README.md
- criterion 4: manual: §Limitations and docs/reports/owasp-llm/README.md

## Risks and completion

Similarity and lexical rank do not establish trust. Content-level poisoning inside an
authorized repository remains possible. A shared outcomes directory can import another
workspace's rows, but query-time scope must not return them as in-scope evidence. The
RAG protocol carries corpus identity, not repository identity. Live producer,
PostgreSQL, embedding, and live-model behavior are unavailable in this environment and
must remain `NOT_RUN` rather than be replaced by a mock.

## Execution contract

Planning only until SOP activates this plan. When activated, refresh `origin/main`, use
an isolated worktree, preserve existing worktrees and uncommitted changes, and run the
focused checks above. Do not enable RAG, change security defaults, introduce a database
or tenant model, implement undecided per-row-digest or RAG-hash proposals, or claim
semantic immunity. Open a focused PR and stop at any genuine dependency or approval
gate.

## Sources and handoff

- Repository: https://github.com/imhttran/agentic-sop
- Parent plan: `docs/plans/PLAN-09-SOP-LLM09-Vector-and-Embedding-Weaknesses.md`
- Current report: `docs/reports/owasp-llm/llm09-retrieval.md`
- Evidence matrix: `docs/reports/owasp-llm/README.md`
- Plan 05 report: `docs/reports/owasp-llm/llm05-poisoning.md`
- Plan 07 report: `docs/reports/owasp-llm/llm07-misinformation.md`
