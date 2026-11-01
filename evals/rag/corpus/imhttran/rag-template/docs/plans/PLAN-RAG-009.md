# PLAN — RAG-009: Multilingual ingestion and retrieval

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-009/`). The original plan is preserved below.**

## Objective

Make retrieval language-aware. Add a per-document `language` metadata field;
select the PostgreSQL full-text-search configuration from that language (falling
back to `simple` for unsupported languages); keep `'english'` as the default when
language is unset, so English-only corpora are byte-identical to today.

## Why this is not a re-implementation

It parameterizes the existing FTS expression (`internal/retrieval/retrieval.go`,
currently hardcoded `'english'`) and adds one additive column. RRF, section dedup,
and section expansion are untouched.

## Tasks

### RAG-009 — Multilingual ingestion and retrieval

- **Objective:** Add an additive `language` metadata column, capture a document's
  language at ingestion (explicit setting, defaulting to unset), and parameterize
  the FTS configuration used by `KeywordSearch` from that language (`simple`
  fallback; `'english'` when unset). Do not change any default.
- **Files (expected):** new `migrations/003_language.sql` (additive `language text`
  column + index if useful); `internal/retrieval/retrieval.go` (parameterized FTS
  config, parameterized query, never string-interpolated); `internal/ingestion/`
  (persist the language); `cmd/ingest/main.go` (language source, default unset);
  `internal/config/config.go` (any language setting, default-safe); tests
  alongside. No change to `internal/contextbudget`, `internal/reranking`,
  `internal/observability`, or the RAG-010/012/014 file sets.
- **Depends on:** none
- **Acceptance:**
  - A non-English document retrieves via a language-appropriate FTS
    configuration (`simple` fallback for unsupported languages).
  - English-only corpora produce byte-identical results to baseline.
  - A document with no language set uses `'english'` (the current behavior); no
    default drifts.
  - Retrieval SQL stays parameterized: the FTS configuration is a bound
    parameter, never interpolated.
- **Validation gate:** unit test for the language → FTS-config selection; a
  PostgreSQL integration test (gated by `RAG_INTEGRATION=1`) ingesting a
  non-English document and asserting language-appropriate retrieval;
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`). If PostgreSQL is unavailable, the integration gate is
  **NOT RUN** — never passed.
- **Execution:** `go build ./... && go vet ./... && go test ./...` for the code
  gate; `RAG_INTEGRATION=1 go test ./internal/retrieval/...` with a live DB for
  the integration gate.
- **Rollback:** revert to fixed `'english'`; the column is additive and needs no
  destructive migration.
- **Scope expansion:** **AUTHORIZED** — recorded in `PLAN-RAG-Gap-Closure.md`
  Amendment D.

## Notes

- Provider-independent: FTS configuration comes from PostgreSQL, not a vendor.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007/008 (PDF/OCR), RAG-011/013/015 (later in this chain),
  RAG-016, and any default change.
