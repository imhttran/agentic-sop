# Testing

`go test ./...` runs the unit tests. They need no database or model: chunking,
document parsing, reranking, answerability, the eval metrics, and RRF fusion are
pure functions, and `HybridRetrieve` is driven through a fake `Searcher` that
records the arguments each stage received, so the pipeline's stage order and its
use of the similarity floor and section deduplication are pinned down.

The SQL needs PostgreSQL, so `internal/retrieval` and `internal/ingestion` have
integration tests. They connect to `DATABASE_URL`, apply
`migrations/001_init.sql` (idempotent, like `make db-up`), and cover:

- `internal/retrieval` — `Search`, `KeywordSearch`, `SectionChunks`, and the whole
  `HybridRetrieve` pipeline against real data, the part a fake cannot check,
  since column order, scan alignment, and placeholder numbering only fail against
  a real server.
- `internal/ingestion` — `ReplaceDocument`'s delete-then-insert transaction:
  re-running replaces rather than appends, the `$5::vector` cast round-trips, and
  a failed embedding leaves the stored document untouched. Embeddings come from a
  stub Ollama server, so no model is needed.

They are skipped unless `RAG_INTEGRATION=1` is set, so the pre-commit hook stays
fast and database-free. `make integration` brings the database up first:

```bash
make integration
```

Both packages use the `documents` table, and `internal/retrieval` truncates it, so
`make integration` passes `-p 1` to run them one at a time. Because the tests
truncate, run them against a throwaway database rather than the dev one;
`DATABASE_URL` is honoured by both the migration step and the tests:

```bash
psql 'postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable' -c 'CREATE DATABASE rag_test'
make integration DATABASE_URL='postgres://rag:rag@127.0.0.1:5434/rag_test?sslmode=disable'
```
