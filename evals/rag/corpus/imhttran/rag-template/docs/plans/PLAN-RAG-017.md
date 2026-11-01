# PLAN — RAG-017: Page provenance and page-aware citations

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-017/`). The original plan is preserved below.**

## Objective

Add an explicit, additive page-provenance contract so a chunk can be traced to the
page it came from, from extraction through ingestion, retrieval, and citation
rendering — **without** changing behaviour for formats that have no pages. This is
the contract that `PDFLoader` (RAG-007) consumes; it lands first.

## Why this is not a re-implementation

It threads one optional field (`Page`) through the existing pipeline and reuses the
RAG-013 citation parser/validator. RRF, dedup, expansion, chunking math, and the
answer prompt are otherwise untouched.

## Tasks

### RAG-017 — Page provenance and page-aware citations

- **Objective:** Add `Page` to `document.Section`, `chunking.Chunk`,
  `retrieval.Document`, and the `documents` table (nullable, additive); persist it
  in ingestion; render it in `rag.FormatContext` only when `> 0`; and extend
  `internal/citations` to accept an optional `- p.N` page segment and validate it.
- **Files (expected):** `internal/document/document.go` (+ `document_test.go`);
  `internal/chunking/chunking.go` (+ `chunking_test.go`); new
  `migrations/004_page.sql`; `internal/ingestion/ingestion.go` (+ tests);
  `internal/retrieval/retrieval.go` (+ `integration_test.go` for a page round-trip);
  `internal/rag/answer.go` (+ `answer_test.go`); `internal/citations/citations.go`
  (+ `citations_test.go`). No change to `internal/contextbudget`,
  `internal/reranking`, `internal/observability`, or the provider seams.
- **Depends on:** none
- **Acceptance:** (see `docs/operations/page-provenance.md` §5)
  - `Section.Page` → `Chunk.Page` → `documents.page` → `Document.Page` round-trips;
    unset is `0`/`NULL` throughout.
  - `migrations/004_page.sql` is additive and idempotent (`ADD COLUMN IF NOT
    EXISTS`), with no backfill and no destructive change.
  - `FormatContext` emits a `Page: N` line **only** when `Page > 0`, so Markdown and
    plain-text output is byte-identical to today.
  - `internal/citations` accepts the legacy `[source - section]` form unchanged and
    the `[source - section - p.N]` form; a page citation is valid only when a
    retrieved document matches source+section **and** has that page; an invalid page
    citation is validated/repairable exactly like any other invalid citation.
  - A re-ingest of unchanged content is still a no-op (the content fingerprint is
    unchanged).
- **Validation gate:** unit tests for propagation, `FormatContext` (no-page vs
  page), and citation parse/validate/repair (legacy, valid page, invalid page);
  a PostgreSQL integration test (gated by `RAG_INTEGRATION=1`) for the page
  round-trip and migration-004 idempotency; `go build ./... && go vet ./... &&
  go test ./...` green (optionally `go test -race ./...`).
- **Execution:** `go build ./... && go vet ./... && go test ./...`
  (optionally `go test -race ./...`).
- **Rollback:** drop `documents.page` (additive) and revert the `Page` fields, the
  `FormatContext` line, and the citation page form; the legacy form and Markdown/text
  behaviour are unaffected either way.
- **Scope expansion:** **none** — an additive field and an optional citation segment;
  no PRD non-goal is crossed.

## Notes

- Provider-independent: no model, vendor, or connection string.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges, or
  approves.
- **Out of scope.** PDF extraction (RAG-007), OCR (RAG-008), genealogy, and any
  default change.
