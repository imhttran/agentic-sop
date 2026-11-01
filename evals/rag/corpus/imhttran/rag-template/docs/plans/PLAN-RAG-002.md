# PLAN — RAG-002: Ingestion reliability (batching, concurrency, retry)

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-002/`). The original plan is preserved below.**

> **RAG-002 only.** This plan carries a single work item so that
> `sop plan activate` / `sop run` compile exactly one task and can never
> re-execute RAG-001. RAG-001's scope and evidence remain in
> `docs/plans/PLAN-RAG-Gap-Closure.md`; they are referenced, not repeated as work.

## Objective

Make document ingestion reliable and fast: embed chunks concurrently instead of one
HTTP round trip per chunk, retry transient embedding failures with bounded backoff,
and insert the chunks in one batched write — while preserving the existing atomicity
guarantee. A 100+ chunk document must no longer be slow, and a single transient
Ollama error must no longer abort the whole ingest.

## Design

```text
sections -> chunks
            |
        embed chunks        (bounded worker pool + retry/backoff; order preserved)
            |
        begin transaction
            |
        DELETE FROM documents WHERE source = $1
        INSERT all chunks in one batched write
            |
        commit                       (unchanged delete-then-insert atomicity)
```

Embeddings are still produced before the transaction opens, so a failed embedding
leaves the stored document untouched (`internal/ingestion/ingestion.go:29-38`).

## Scope

- `internal/ingestion/ingestion.go` — replace the serial embedding loop
  (`ReplaceDocument`, currently `ingestion.go:49-64`) with bounded-concurrency
  embedding plus bounded retry/backoff; replace the per-row `INSERT` loop with a
  single batched insert (`CopyFrom` or a multi-row `INSERT`).
- `internal/config/config.go` — add the config surface for the worker count and the
  retry budget (**new settings only; existing defaults unchanged**).
- Tests under `internal/ingestion` (unit tests always; integration tests guarded by
  `RAG_INTEGRATION=1`).
- `docs/plans/PLAN-RAG-Gap-Closure.md` — record the outcome and evidence.

**Out of scope:** the retrieval pipeline, ranking, the embedding model, generation,
schema/migrations, provider behavior, RAG-003+, and any genealogy-specific feature.

## Constraints

- **Preserve atomicity.** Embeddings are computed before the transaction; the
  replacement stays delete-then-insert; a failed embedding leaves the stored document
  untouched (`ingestion.go:29-38`).
- **No default drift.** Existing defaults are unchanged unless a recorded evaluation
  supports a change (`docs/reference/experiments.md`; working method in `docs/plans/PLAN.md:333`).
- **Model/provider independence.** Use the existing `embedding` provider seam; do not
  bind ingestion to Ollama directly.

## Dependency

- **RAG-001 — satisfied.** Commit `70296a3` provides config validation and actionable
  failures (`internal/config/config.go`), which this task extends with new settings.
  This is the only dependency; no other task is required.

## Tasks

### RAG-002 — Ingestion reliability: batching, concurrency, retry

- **Status:** Implemented (SOP `LOCAL_DONE`). Acceptance closed by the follow-up below;
  criterion 3 (wall-clock) is recorded as a limitation, not a measured improvement.
- **Area:** Ingestion reliability (gap G-02).
- **Rationale:** `internal/ingestion/ingestion.go:49-64` embeds one chunk per HTTP
  round trip, serially, with no retry. A 100+ chunk document is slow and a single
  transient Ollama error aborts the whole ingest.
- **Scope:** Bounded-concurrency embedding (configurable worker count, default
  modest), retry with backoff on transient failures, and a single batched insert
  path (`CopyFrom` or multi-row insert) inside the existing transaction. Preserve the
  current atomicity guarantee (embeddings computed before the transaction;
  delete-then-insert) documented at `ingestion.go:29-38`.
- **Non-duplication:** Keeps `ReplaceDocument`'s transaction semantics; only the
  embedding loop and insert loop change.
- **Deps:** RAG-001 (config surface for concurrency/retry) — satisfied at `70296a3`.
- **Acceptance Criteria:**
  - [x] Embedding calls may run concurrently but results retain chunk order.
        (`mapOrdered` indexes results by input position; integration test
        `TestReplaceDocumentIntegrationRetriesTransientEmbedFailures` asserts order.)
  - [x] Transient errors retry a bounded number of times, then fail the ingest
        without leaving partial documents (integration test asserts this).
        (`retryEmbed` is bounded; `TestReplaceDocumentIntegrationRetryExhaustionPreservesDocument`
        and `TestReplaceDocumentIntegrationKeepsDocumentOnEmbedError` assert no partial write.)
  - [ ] Wall-clock time for `examples/commercial-servicing-manual.md` drops
        materially versus baseline (record before/after). **Not verifiable: no
        pre-change baseline was recorded and this environment has no embedding model
        pulled.** Limitation recorded below; no improvement is claimed.
- **Tests:** keep and extend `internal/ingestion`:
  `TestReplaceDocumentIntegration`, `TestReplaceDocumentIntegrationReplaces`,
  `TestReplaceDocumentIntegrationKeepsDocumentOnEmbedError`, plus a new test that
  injects a flaky stub embedder and asserts bounded retry with no partial write, and a
  unit test for the concurrency limiter (order preserved).
- **Validation Gate:** `go build ./...`, `go test ./...`, `go vet ./...`
  (`.agent-sdlc/config.yaml`), plus the DB integration suite with `RAG_INTEGRATION=1`
  (existing tests still pass) and the new retry/limiter tests.
- **Rollback:** Revert; no schema change. (Data is re-ingestable.)
- **Human boundary:** SOP stops at the configured human gate
  (`human.approval_before_commit: true`); no commit, push, or merge without an explicit
  operator decision.

## Follow-up — bounded acceptance closure

A bounded follow-up closed the acceptance gaps SOP's self-review flagged. No scope or
default changed (still `EMBED_WORKERS=4`, `EMBED_RETRIES=3`).

### Added tests (`internal/ingestion/integration_test.go`)

- `TestReplaceDocumentIntegrationRetriesTransientEmbedFailures` — a stub Ollama server
  fails the first two requests; with one worker the ingest retries them, stores every
  chunk **in order**, and the request count is exactly `2 + len(chunks)` (bounded).
- `TestReplaceDocumentIntegrationRetryExhaustionPreservesDocument` — a permanently
  failing embedder exhausts the bounded retries; the ingest fails naming the attempts,
  and the previously stored document is unchanged (no partial write).

### Retry classification (narrow change, `internal/ingestion/concurrency.go`)

- `retryEmbed` is strictly bounded (`retries+1` attempts) and now stops immediately when
  the context is canceled or expired, so a permanent abort cannot consume the remaining
  attempts. Unit test: `TestRetryEmbedStopsWhenContextCanceled`.
- Provider errors are not classified transient/permanent here because the shared Ollama
  client exposes no HTTP-status error type; the retry stays bounded, so a permanent
  provider error costs at most `retries+1` attempts. Adding a typed provider error is
  outside RAG-002 scope.

### Wall-clock measurement — limitation recorded

No pre-change wall-clock baseline was recorded for
`examples/commercial-servicing-manual.md` (searched `docs/`, `docs/reference/experiments.md`, and
`.agent-sdlc/runs/RAG-002/`: none), and this environment has no embedding model pulled
(`ollama` lists chat models only). A before/after comparison is therefore **not
possible**, and **no improvement is claimed**. The authorized fallback — this recorded
limitation — applies. The concurrency mechanism is itself covered by
`TestMapOrderedPreservesOrder` (peak concurrency ≤ workers, order preserved).

### Validation (follow-up pass)

| Check | Result |
|-------|--------|
| `gofmt -l .` | PASS (no files) |
| `go vet ./...` | PASS |
| `staticcheck ./...` (2026.2.1) | PASS (no findings) |
| `go build ./...` | PASS |
| `go test -count=1 ./...` | PASS |
| `go test -race -count=1 ./...` | PASS |
| `RAG_INTEGRATION=1` integration (`-p 1`) | **10 PASS / 0 FAIL** |
| `git diff --check` | PASS |

Integration breakdown (native PostgreSQL 16, `rag_db`): retrieval 5 PASS
(`TestSearchIntegrationOrdersBySimilarity`,
`TestKeywordSearchIntegrationRanksByTextRank`,
`TestSectionChunksIntegrationReturnsOrderedChunks`,
`TestSectionChunksIntegrationDoesNotStarveSections`, `TestHybridRetrieveIntegration`);
ingestion 5 PASS (`TestReplaceDocumentIntegration`,
`TestReplaceDocumentIntegrationReplaces`,
`TestReplaceDocumentIntegrationKeepsDocumentOnEmbedError`,
`TestReplaceDocumentIntegrationRetriesTransientEmbedFailures`,
`TestReplaceDocumentIntegrationRetryExhaustionPreservesDocument`).

## Validation

SOP runs the configured build/test/lint commands from `.agent-sdlc/config.yaml`:
`go build ./...`, `go test ./...`, `go vet ./...`. The database integration gate runs
with `RAG_INTEGRATION=1` against the native PostgreSQL 16 test database
(`DATABASE_URL` from the git-ignored `.env`; database `rag_db`, host `127.0.0.1:5432`).

## References

- `docs/plans/PLAN-RAG-Gap-Closure.md` — the gap audit; RAG-002 is Milestone 1.
- `internal/ingestion/ingestion.go`, `internal/config/config.go`,
  `internal/embedding/embedding.go`.
