# PLAN — RAG-005C: Isolated non-768 end-to-end verification

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-005c/`). The original plan is preserved below.**

## Objective

Deliver the original RAG-005C acceptance with one focused, self-contained
integration test: prove a 3-dimensional embedding provider drives ingestion and
retrieval end-to-end in an isolated schema, that the default 768 path is unchanged,
and that a dimension mismatch fails before any destructive replacement.

## Tasks

### RAG-005C — Isolated non-768 end-to-end ingestion + retrieval test

- **Objective:** Prove a 3-dimensional embedding provider drives ingestion and retrieval end-to-end in an isolated schema, with the 768 default unchanged and a mismatch rejected before any destructive write.
- **Scope:** Add ONE integration test in `internal/ingestion/schema_e2e_integration_test.go` (guarded by `RAG_INTEGRATION=1`) that:
  1. opens its own connection to `DATABASE_URL`, creates an isolated schema (`CREATE SCHEMA IF NOT EXISTS rag_dim_e2e`), sets `search_path` to it, and creates `documents` with `embedding vector(3)`;
  2. uses a deterministic stub `embedding.Embedder` returning a fixed 3-dimensional vector (for example `[]float64{1, 0, 0}`), builds the Ingester with `NewWithOptions(conn, stub, 1, 0)`, ingests a few chunks, and retrieves them with `retrieval.New(conn).Search`, asserting the 3-dimensional vector round-trips (cosine similarity 1) and every chunk is stored;
  3. asserts `CheckEmbeddingDim(conn, 3)` → nil in that schema and `CheckEmbeddingDim(conn, 768)` → an actionable mismatch error;
  4. asserts the default `vector(768)` path is unchanged: `CheckEmbeddingDim(testConn, 768)` → nil;
  5. asserts a mismatch is rejected before any destructive write: on the mismatched dimension the guard errors, so `ReplaceDocument` is never reached — assert a pre-seeded row in the isolated schema remains unchanged.
  It drops the isolated schema in `t.Cleanup`. It MUST NOT modify production code.
- **Files:** `internal/ingestion/schema_e2e_integration_test.go`
- **Depends on:** none
- **Acceptance:** non-768 (dimension 3) ingestion + retrieval succeed in isolation; the 768 default is unchanged; a mismatch fails before any write; `go build ./...`, `go vet ./...`, `go test ./...` pass and the `RAG_INTEGRATION=1` suite passes.
- **Execution:** `go build ./... && go vet ./... && go test ./...`; then `RAG_INTEGRATION=1 go test ./internal/ingestion/ -run E2E -p 1`.

## Notes

- Isolated schema only: the default `documents` table and the existing `rag_db` data are neither altered nor destroyed (the isolated schema is dropped in cleanup).
- Provider-independent interfaces and the existing production implementation are preserved unchanged.
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
- **Out of scope.** RAG-006–016; the optional vendor-import cleanup; PRD non-goals.
