# PLAN — RAG-003–005: Remaining M1–M2 (provenance, provider interfaces, dimension decoupling)

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `SUPERSEDED` (see `.agent-sdlc/archive/plan-rag-003-005/`). The original plan is preserved below.**

## Objective

Complete the remaining approved Milestone 1 and Milestone 2 tasks from
`docs/plans/PLAN-RAG-Gap-Closure.md`: RAG-003 (ingestion provenance and idempotent
re-index), RAG-004 (provider-independent embedder/generator interfaces), and RAG-005
(embedding-dimension decoupling and schema guard).

## Tasks

### RAG-003 — Ingestion provenance and idempotent re-index

- **Objective:** Give every ingested chunk provenance and make re-ingestion idempotent, so "is this document up to date?" is answerable and unchanged files are not re-embedded. `source` is `filepath.Base`, so `a/policy.md` and `b/policy.md` collide, and no content hash, embedding model, dimension, chunker config, or timestamp is stored.
- **Scope:** Add `migrations/002_ingestion_metadata.sql` with additive columns (`source_path`, `content_hash`, `embedding_model`, `embedding_dim`, `chunker_size`, `chunker_overlap`, `ingested_at`) and a `sources` table keyed by the canonical source path. Change `cmd/ingest` to use a canonical source identifier (path-relative or explicit `--source`), and skip re-embedding when `content_hash` + chunker config + model/dim are unchanged. **Do not** delete the existing `documents` columns; add only.
- **Files:** `migrations/002_ingestion_metadata.sql`, `cmd/ingest/main.go`, `internal/ingestion/ingestion.go`, `internal/config/config.go`
- **Depends on:** none
- **Acceptance:** Re-ingesting an unchanged file is a no-op (no embeddings recomputed). Changing content or chunker config re-ingests. Two files with the same basename in different directories are distinct sources. Provenance columns are populated and queryable.
- **Execution:** Add a new integration test for skip/replace/collision; `make db-schema` idempotent; `go test ./...` green.

### RAG-004 — Embedder and generator provider interfaces

- **Objective:** Decouple embedding and generation from Ollama behind interfaces, with Ollama remaining the default provider.
- **Scope:** Introduce `embedding.Embedder` and `generation.Generator` as **interfaces** (`Embed(ctx,text)` / `Generate(ctx,prompt)`) with the current Ollama structs adapted behind them, plus a tiny provider registry selected by `EMBED_PROVIDER` / `GEN_PROVIDER` env vars (default `ollama`). `internal/ollama` remains the only vendor-aware package.
- **Files:** `internal/embedding/`, `internal/generation/`, `internal/config/config.go`, `cmd/`, plus a new provider registry.
- **Depends on:** RAG-003
- **Acceptance:** No package except `internal/ollama` (and the registry) imports a vendor. `config.Config` returns interface values; `cmd/*` and `internal/*` compile against interfaces. A stub in-memory provider can drive ingestion + retrieval in tests. Ollama remains the default and existing tests pass unchanged.
- **Execution:** `go build ./...`, `go vet ./...`, `go test ./...` green; add a test wiring a fake provider end-to-end through the pipeline.

### RAG-005 — Embedding-dimension decoupling and schema guard

- **Objective:** Make the embedding dimension configuration-driven instead of a hard-coded `vector(768)`, with a fail-fast guard when the configured dimension does not match the column.
- **Scope:** Make the vector column dimension configuration-driven via a migration that reads a documented `EMBED_DIM` setting (or per-model dimension registry), and validate at startup that the retrieved/ingested dimension matches the column dimension, failing with a clear message. Document that changing dimension requires re-ingestion.
- **Files:** `migrations/` (dimension migration), `internal/config/config.go`, `internal/ingestion/`, `internal/retrieval/`
- **Depends on:** RAG-003, RAG-004
- **Acceptance:** A non-768 model is usable after a documented migration + re-ingest. Mismatched configured dimension fails fast with an actionable error. Default path (`nomic-embed-text`, 768) is unchanged.
- **Execution:** Integration test ingesting with a stubbed non-768 provider against a matching column; startup test for mismatch; `make db-schema` idempotent.

## Notes

- **Execution ordering.** The three tasks are chained for sequential execution
  (RAG-003 → RAG-004 → RAG-005) per operator instruction, so no two run
  concurrently. RAG-004 declares `RAG-003` as a dependency for ordering only; its
  scope and acceptance are unchanged.
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
  The plan is not historicalized automatically.
- **Out of scope.** RAG-006–016 and any Phase 23+ work; PRD non-goals; provider
  behavior beyond the approved scope.
