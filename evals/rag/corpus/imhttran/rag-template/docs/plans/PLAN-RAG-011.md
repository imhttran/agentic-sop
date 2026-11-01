# PLAN — RAG-011: Metadata filtering across retrieval

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-011/`). The original plan is preserved below.**

## Objective

Add an optional, parameterized filter to the three retrieval surfaces —
`Search`, `KeywordSearch`, and `SectionChunks` — so retrieval can be restricted
to a source prefix, a language, a document type, or an ingested-date range,
applied inside SQL with parameterized predicates. The filter is threaded through
`retrieval.PipelineOptions` so `cmd/eval` can exercise it. With no filter set,
behaviour is byte-identical to the current baseline.

## Why this is not a re-implementation

It extends, and does not replace, the existing queries
(`internal/retrieval/retrieval.go`); RRF fusion, section dedup, and section
expansion are untouched. The filter is additive: an unset filter must produce
the same rows in the same order as today.

## Dependency on RAG-009

RAG-011's scope names four filter dimensions. The language dimension needs the
`documents.language` column that RAG-009 adds; that scope expansion is now
authorized (Amendment D), so this plan runs **after RAG-009 lands**. The status
per dimension:

| Dimension | Column | Status |
| --------- | ------ | ------ |
| Source prefix | `documents.source` | **available** (RAG-003) |
| Ingested date range | `documents.ingested_at` | **available** (RAG-003) |
| Language | `documents.language` | **available after RAG-009** |
| Document type | *(none)* | **out of scope** — no column; needs a separate metadata decision |

The document-type dimension is dropped from this task's scope (it has no column
and no authorized task adds one); the plan covers the source, date-range, and
language dimensions. An unset filter must produce byte-identical baseline rows.

## Tasks

### RAG-011 — Metadata filtering across retrieval

- **Objective:** Add an optional filter value to `Search`, `KeywordSearch`, and
  `SectionChunks`, constructed from configured criteria (source prefix,
  language, doc type, ingested-date range), applied as parameterized SQL
  predicates; thread it through `retrieval.PipelineOptions`.
- **Files (expected):** `internal/retrieval/retrieval.go` (filter parameter and
  predicates), `internal/retrieval/pipeline.go` (`PipelineOptions` field),
  `internal/retrieval/integration_test.go` (new integration cases),
  `cmd/eval/main.go` (thread the filter for evaluation); tests alongside. No
  change to `internal/contextbudget`, `internal/loader`, `internal/document`,
  `internal/chunking`, or `internal/ingestion`.
- **Depends on:** none
- **Acceptance:**
  - Filtering reduces candidate sets correctly in the vector, keyword, and
    expansion paths.
  - With no filter set, results are byte-identical to the pre-change baseline.
  - SQL-injection safety is preserved: values are bound as parameters and only
    predicate structure is dynamic.
  - Language and doc-type dimensions: language is in scope once RAG-009 has
    landed; document type is **out of scope** (no column); the plan covers source
    prefix, ingested-date range, and language.
- **Validation gate:** PostgreSQL integration tests (gated by
  `RAG_INTEGRATION=1`) for each in-scope filter dimension, plus a unit test that
  no-filter equals baseline. `go build ./... && go vet ./... && go test ./...`
  green (optionally `go test -race ./...`). If PostgreSQL is unavailable, the
  integration gate is recorded as **NOT RUN** — never as passed.
- **Execution:** `RAG_INTEGRATION=1 go test ./internal/retrieval/...` for the
  integration gate; `go build ./... && go vet ./... && go test ./...`
  (optionally `go test -race ./...`) otherwise.
- **Rollback:** revert; filters default to "no filter" and behaviour returns to
  the baseline.
- **Scope expansion:** **none** — this plan consumes the already-authorized
  RAG-009 `language` column; it adds no new scope of its own.

## Notes

- Provider-independent: no vendor, no model, no database connection string; the
  filter is expressed in terms of column values, not a provider.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007–009, RAG-013–016, new metadata columns, retrieval
  ranking changes, and any default change.
