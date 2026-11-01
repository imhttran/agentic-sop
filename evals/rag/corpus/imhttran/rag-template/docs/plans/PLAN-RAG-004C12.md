# PLAN — RAG-004C1/C2: Provider registry + config factories, and stub-provider pipeline tests

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-004c12/`). The original plan is preserved below.**

## Objective

Complete the original RAG-004C contract with two smaller, independently verifiable
tasks: a provider registry with configuration factories (RAG-004C1), and an
end-to-end stub-provider ingestion + retrieval test (RAG-004C2). Ollama remains the
default provider and the ingestion/retrieval pipeline never imports a vendor.

## Tasks

### RAG-004C1 — Provider registry and configuration factories

- **Objective:** Select the embedder/generator provider by name (default `ollama`) and expose factories that return the provider-agnostic interfaces.
- **Scope:** Add `internal/provider` with a registry mapping `EMBED_PROVIDER` / `GEN_PROVIDER` (default `ollama`) to constructors that return `embedding.Embedder` / `generation.Generator`. Add `config.Config` factory methods (for example `Embedder()` / `Generator()`) that resolve through the registry. Only `internal/ollama` and `internal/provider` may import the vendor client; ingestion and retrieval never import it.
- **Files:** `internal/provider/provider.go` (new), `internal/provider/provider_test.go` (new), `internal/config/config.go`
- **Depends on:** none
- **Acceptance:** the registry returns an `embedding.Embedder` / `generation.Generator` by name and defaults to `ollama`; `config.Config` exposes factories returning the interfaces; only `internal/ollama` and `internal/provider` import a vendor; `go build ./...`, `go vet ./...`, `go test ./...` pass.
- **Execution:** add a registry/factory unit test (default is `ollama`; a stub name can be registered); run `go build ./... && go vet ./... && go test ./...`.

### RAG-004C2 — End-to-end stub-provider ingestion + retrieval test

- **Objective:** Prove that a stub in-memory provider drives ingestion and retrieval end-to-end, with Ollama still the default.
- **Scope:** Add a test that builds a stub in-memory `embedding.Embedder` (no HTTP), ingests a few chunks with `ingestion.ReplaceDocument` against `DATABASE_URL`, and retrieves them with `retrieval.New(conn).Search`, asserting the stub vector round-trips (cosine similarity 1) and that no vendor type is involved. Reuse the existing ingestion integration harness (`TestMain` + `applyMigrations`) and guard the test with `RAG_INTEGRATION=1`.
- **Files:** `internal/ingestion/provider_pipeline_integration_test.go` (new)
- **Depends on:** RAG-004C1
- **Acceptance:** a stub provider drives ingestion + retrieval end-to-end with no vendor type; Ollama remains the default; `go build ./...`, `go vet ./...`, `go test ./...` pass and the integration test passes under `RAG_INTEGRATION=1`.
- **Execution:** `go build ./... && go vet ./... && go test ./...`; then `RAG_INTEGRATION=1 go test ./internal/ingestion/ -run Integration -p 1`.

## Notes

- **Contract preserved.** RAG-004C1 ∪ RAG-004C2 reproduce the original RAG-004C acceptance criteria exactly — no behavior change, no dropped criteria, no scope expansion.
- **Provider-agnostic.** Interfaces live in `embedding`/`generation`; only `internal/ollama` and the new `internal/provider` import a vendor.
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
- **Out of scope.** RAG-005 and RAG-006–016; PRD non-goals; provider behavior beyond the approved scope.
