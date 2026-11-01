# PLAN — RAG-005: Embedding-dimension decoupling and schema guard

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `SUPERSEDED` (see `.agent-sdlc/archive/plan-rag-005/`). The original plan is preserved below.**

## Objective

Remove the hardcoded 768-dimensional embedding assumption so a different embedding
model's dimension can be used — while keeping the default (`nomic-embed-text`, 768)
and all retrieval behavior unchanged — and make a dimension mismatch fail fast with an
actionable error instead of silently corrupting stored vectors or retrieval.

## Contract confirmed from the approved plan

- Add a documented `EMBED_DIM` setting (default 768) and validate at startup that the
  embedding dimension matches the `documents.embedding` column dimension, failing with
  a clear message.
- Changing the dimension requires an operator-run migration **plus re-ingestion**; the
  default `vector(768)` schema and behavior are unchanged.
- **Not invented / not silently changed.** The dimension change is a documented,
  operator-run procedure. This task MUST NOT auto-apply a destructive
  `ALTER TABLE … DROP COLUMN` to existing data, and MUST keep `make db-schema`
  idempotent and non-destructive.

## Tasks

### RAG-005 — Embedding-dimension decoupling and schema guard

- **Objective:** Make the embedding dimension configuration-driven with a fail-fast schema guard, keeping the 768 default and retrieval unchanged.
- **Scope:** Add `EMBED_DIM` to `internal/config` (positive int, default 768). Add a guard that reads the `documents.embedding` column dimension and, when the embedding dimension differs, fails with an actionable error naming `EMBED_DIM`, the expected/actual dimensions, and the fix (migrate + re-ingest). Wire the guard into `cmd/ingest` and `cmd/rag`. Document the dimension-change procedure (operator-run migration + re-ingest) in `README.md` and in `migrations/003_embedding_dim.sql`, which documents the procedure and MUST NOT destructively alter existing rows automatically. Retrieval SQL and the `vector(768)` default are unchanged.
- **Files:** `internal/config/config.go`, a new guard (for example `internal/ingestion/schema.go`), `cmd/ingest/main.go`, `cmd/rag/main.go`, `migrations/003_embedding_dim.sql` (documented procedure), `README.md`, and tests.
- **Depends on:** none
- **Acceptance:** A non-768 model is usable after the documented migration + re-ingest; a mismatched configured dimension fails fast with an actionable error; the default path (`nomic-embed-text`, 768) is unchanged.
- **Execution:** Add an integration test ingesting with a stubbed non-768 provider against a matching column, a startup/guard test for the mismatch, and confirm `make db-schema` is idempotent. Run `go build ./... && go vet ./... && go test ./...` and the `RAG_INTEGRATION=1` integration suite.

## Notes

- **Provider-agnostic** behavior and existing Ollama compatibility are preserved.
- **Ingestion atomicity, provenance, and idempotent re-indexing are preserved.**
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
- **Out of scope.** RAG-006–016; the optional vendor-import cleanup; PRD non-goals.
