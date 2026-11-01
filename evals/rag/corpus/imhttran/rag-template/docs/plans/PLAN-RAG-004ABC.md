# PLAN — RAG-004A/B/C: Provider-independent embedder & generator interfaces

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `SUPERSEDED` (see `.agent-sdlc/archive/plan-rag-004abc/`). The original plan is preserved below.**

## Objective

Replace the single, oversized RAG-004 with three smaller, independently verifiable
tasks that together satisfy the original RAG-004 contract: decouple embedding and
generation from Ollama behind provider-agnostic interfaces, keep Ollama as the
default, and prove it with a stub provider — without coupling the ingestion or
retrieval pipeline to any vendor.

## Tasks

### RAG-004A — Embedder interface and Ollama embedder seam

- **Objective:** Make ingestion depend on an `Embedder` interface instead of the concrete Ollama embedder type, with Ollama still the default.
- **Scope:** In `internal/embedding`, declare `type Embedder interface { Embed(ctx context.Context, text string) ([]float64, error) }` and keep the Ollama-backed implementation behind it, so `embedding.New(client, model) Embedder` returns the interface. Change `internal/ingestion.Ingester` to hold `embedding.Embedder` rather than `*embedding.Embedder`. `internal/ollama` remains the only package that imports the vendor client for embeddings.
- **Files:** `internal/embedding/embedding.go`, `internal/ingestion/ingestion.go`, `internal/ingestion/concurrency.go`, `cmd/ingest/main.go`
- **Depends on:** none
- **Acceptance:** `internal/ingestion` compiles against the `embedding.Embedder` interface; a stub `Embedder` that performs no HTTP drives `ReplaceDocument` in a test; `internal/ollama` is the only vendor-importing package for embeddings; behavior is unchanged and `go build ./...`, `go vet ./...`, `go test ./...` pass.
- **Execution:** add a stub-embedder ingestion test; run `go build ./... && go vet ./... && go test ./...`.

### RAG-004B — Generator interface and Ollama generator seam

- **Objective:** Make generation consumers depend on a `Generator` interface, with Ollama still the default.
- **Scope:** In `internal/generation`, declare `type Generator interface { Generate(ctx context.Context, prompt string) (string, error) }` and keep the Ollama-backed implementation behind it, so `generation.New(client, model) Generator` returns the interface. Change `rag.Answer`, `rag.Rewrite`, `answerability`, and `reranking.RerankLLM` to accept `generation.Generator`.
- **Files:** `internal/generation/generation.go`, `internal/rag/answer.go`, `internal/rag/rewrite.go`, `internal/answerability/answerability.go`, `internal/reranking/llm.go`, `cmd/rag/main.go`, `cmd/eval/main.go`
- **Depends on:** RAG-004A
- **Acceptance:** those consumers compile against the `generation.Generator` interface; a stub `Generator` drives `Answer`, `Rewrite`, `IsAnswerable`, and `RerankLLM` in tests; Ollama remains the default and `go build ./...`, `go vet ./...`, `go test ./...` pass.
- **Execution:** add stub-generator tests; run `go build ./... && go vet ./... && go test ./...`.

### RAG-004C — Provider registry, config interfaces, and stub-provider pipeline test

- **Objective:** Select the embedder/generator provider (default Ollama) and return interfaces from config, with an end-to-end stub-provider test.
- **Scope:** Add a small provider registry (for example `internal/provider`) mapping `EMBED_PROVIDER` / `GEN_PROVIDER` (default `ollama`) to constructors of the `embedding.Embedder` / `generation.Generator` interfaces. Have `config.Config` expose factories that return those interfaces. `internal/ollama` and the registry remain the only vendor-aware packages.
- **Files:** `internal/provider/` (new), `internal/config/config.go`, `internal/embedding/`, `internal/generation/`, `cmd/`
- **Depends on:** RAG-004A, RAG-004B
- **Acceptance:** no package except `internal/ollama` (and the registry) imports a vendor; `config.Config` returns interface values and `cmd/*` and `internal/*` compile against interfaces; a stub in-memory provider drives ingestion + retrieval in a test; Ollama remains the default and existing tests pass unchanged.
- **Execution:** add an end-to-end stub-provider test; run `go build ./... && go vet ./... && go test ./...`.

## Notes

- **Provider-agnostic.** Interfaces live in `embedding`/`generation`; only `internal/ollama` and the new registry import a vendor. The ingestion and retrieval pipeline never references Ollama.
- **Contract preserved.** RAG-004A ∪ RAG-004B ∪ RAG-004C reproduce the original RAG-004 acceptance criteria exactly — no behavior change, no dropped criteria, no scope expansion.
- **RAG-005 deferred.** The embedding-dimension decoupling (RAG-005) is **not** in this plan; a follow-up plan will add it with `Depends on: RAG-003, RAG-004C`.
- **Human boundary.** SOP stops before commit; no task commits, pushes, or merges.
- **Out of scope.** RAG-005 and RAG-006–016; PRD non-goals; provider behavior beyond the approved scope.
