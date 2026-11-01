# PLAN — RAG-005A/B/C: Embedding-dimension correctness, procedure, and verification

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `SUPERSEDED` (see `.agent-sdlc/archive/plan-rag-005abc/`). The original plan is preserved below.**

## Objective

Deliver the original RAG-005 contract (dimension decoupling + schema guard) in three
smaller, independently verifiable tasks, reusing the existing partial implementation.
Preserve the default 768-dimensional behavior, ingestion atomicity/provenance/
idempotent re-indexing, provider-independent interfaces, and data integrity.

## Tasks

### RAG-005A — Vector typmod correctness

- **Objective:** Read the pgvector column dimension correctly and cover it with tests.
- **Scope:** Correct `vectorFromTypmod` in `internal/ingestion/schema.go` to use pgvector's `atttypmod` directly (it already equals the dimension; do not subtract a varchar header). Preserve rejection of non-vector/invalid typmods. Keep the `embeddingDimQuerier` seam and the deterministic catalog query. Update the unit table in `schema_test.go` and the assertion in `schema_integration_test.go` so `CheckEmbeddingDim(768)` returns nil against the `001` schema and a mismatch error names both dimensions for `1024`.
- **Files:** `internal/ingestion/schema.go`, `internal/ingestion/schema_test.go`, `internal/ingestion/schema_integration_test.go`
- **Depends on:** none
- **Acceptance:** `vectorFromTypmod` maps 768→768 and 1024→1024 and rejects non-vector typmods; `CheckEmbeddingDim(768)` → nil against `001`; `CheckEmbeddingDim(1024)` → actionable mismatch; `go build ./...`, `go vet ./...`, `go test ./...` pass and the integration test passes under `RAG_INTEGRATION=1`.
- **Execution:** `go build ./... && go vet ./... && go test ./...`; then `RAG_INTEGRATION=1 go test ./internal/ingestion/ -run Integration -p 1`.

### RAG-005B — Dimension mismatch and migration procedure

- **Objective:** Keep the actionable dimension-mismatch error and ship an operator-run dimension-change procedure that cannot run automatically.
- **Scope:** Place the procedure under `docs/operations/` (e.g. `docs/operations/embedding-dimension.md`) — **not** `migrations/`, so neither `make db-schema` (`migrations/*.sql`) nor the docker `docker-entrypoint-initdb.d` mount can run it. Update the guard's referenced path (`migrationFile`) and its message to point there. Document `EMBED_DIM`, dimension compatibility, required re-ingestion, and data-safety precautions.
- **Files:** `docs/operations/embedding-dimension.md` (new), `internal/ingestion/schema.go`, `README.md`
- **Depends on:** RAG-005A
- **Acceptance:** the procedure file exists under `docs/operations/` and is not matched by `migrations/*.sql`; the mismatch error names it; `make db-schema` remains idempotent and non-destructive; README documents `EMBED_DIM` and the procedure.
- **Execution:** confirm no `migrations/003_*.sql` exists; confirm `make db-schema` re-run is a no-op; `go build ./... && go test ./...`.

### RAG-005C — End-to-end verification

- **Objective:** Prove a non-768 dimension works end-to-end and 768 still works.
- **Scope:** Add an integration test that uses an isolated PostgreSQL test schema with a matching non-768 `documents.embedding` column (for example `vector(3)`) and a stub provider of that dimension, ingests, and retrieves; assert the default 768 path still works; assert a mismatched dimension fails before any destructive ingestion/replacement. Finish README/project-structure documentation. Preserve provider-independent interfaces.
- **Files:** `internal/ingestion/` (integration test), `README.md`
- **Depends on:** RAG-005A, RAG-005B
- **Acceptance:** non-768 ingest + retrieval succeed in isolation; the 768 default is unchanged; a mismatch fails before `ReplaceDocument` writes; `go build ./...`, `go vet ./...`, `go test ./...` pass and the `RAG_INTEGRATION=1` suite passes.
- **Execution:** `go build ./... && go vet ./... && go test ./...`; then `RAG_INTEGRATION=1 go test ./internal/ingestion/ -run Integration -p 1`.

## Notes

- **No destructive operation.** No automatic `ALTER TABLE … DROP COLUMN`; the existing `rag_db` and its `vector(768)` column are untouched by tests (isolated schema/database only).
- **Reuse, don't rewrite.** Fix `schema.go`'s typmod conversion; keep the guard seam, catalog query, config `EMBED_DIM`, cmd wiring, and tests.
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
- **Out of scope.** RAG-006–016; the optional vendor-import cleanup; PRD non-goals.
