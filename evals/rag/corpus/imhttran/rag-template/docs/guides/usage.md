# Usage

## Setup

Start the database and apply the migrations (pgvector extension, `documents`
table, and the vector index):

```bash
make db-up
```

`docker compose` runs every file in `migrations/` in name order the first time
the volume is created, which only covers a fresh database. `make db-up`
therefore re-applies the migrations afterwards, and `make db-schema` does so on
demand for an existing volume. Without `make`:

```bash
docker compose up -d --wait
for f in migrations/*.sql; do
  psql 'postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable' -f "$f"
done
```

Every migration is idempotent, so re-running is safe. `make db-schema` only
globs `migrations/*.sql`, so nothing under `docs/operations/` — including the
dimension-change procedure — is applied automatically; that procedure is
deliberately outside the glob, which is what keeps `make db-schema` idempotent
and non-destructive.

### Changing the embedding dimension

`documents.embedding` is a `vector(768)` column (see `migrations/001_init.sql`)
and `EMBED_DIM` must match it. To move to a model of a different width, follow
the operator-run procedure in
[`docs/operations/embedding-dimension.md`](../operations/embedding-dimension.md):
it changes the column to `vector(n)`, rebuilds the `documents_embedding_idx` HNSW
index, sets `EMBED_DIM`, and requires **re-ingesting the corpus** so every stored
vector is produced at the new dimension. The guard `ingestion.CheckEmbeddingDim`
fails fast with the same instructions when `EMBED_DIM` and the column disagree.
This procedure is never applied by `make db-schema`, `make db-up`, or the
`docker-entrypoint-initdb.d` mount in `docker-compose.yml`, because it is not
under `migrations/*.sql`.

## Ingest

Chunk a document, embed each chunk, and store it:

```bash
go run ./cmd/ingest examples/loan-policy.md
```

The corpus is markdown; each `## section` is split into overlapping ~100-word
chunks stored with their `source`, `section`, and `chunk_index` for citations.
Output:

```text
Ingested 6 chunks from loan-policy.md.
```

Re-running replaces that file's stored chunks, so the store never accumulates
duplicates.

## Query

Ask a question:

```bash
go run ./cmd/rag "What happens if someone misses a payment?"
```

The question can also come from the `QUESTION` setting, or be typed at the prompt
when the command is run with no argument:

```bash
QUESTION="What happens if someone misses a payment?" go run ./cmd/rag
go run ./cmd/rag   # prompts: Question:
```

Output (scores and the answer vary by model and data):

```text
Question:
What happens if someone misses a payment?

Ollama created a 768-dimensional query vector

Vector retrieval:
KEPT      <score>  [loan-policy.md - Missed Payments - chunk 0] If a borrower misses a payment, ...
KEPT      <score>  [loan-policy.md - Late Fees - chunk 0] A late fee may be assessed ...
FILTERED  <score>  A delinquent loan may eventually be referred to collections ...

Hybrid retrieval:
RRF <score>  vector <score>  keyword <score>  [loan-policy.md - Missed Payments - chunk 0] ...

Expanded context:
[loan-policy.md - Missed Payments - chunk 0] ...
[loan-policy.md - Missed Payments - chunk 1] ...

Context being sent to the LLM:
Source: loan-policy.md
Section: Missed Payments
Chunk: 0
Content: If a borrower misses a payment, ...

Answer:
<the model's answer, citing [loan-policy.md - Missed Payments] where relevant>
```

Retrieval is hybrid: vector search (`TOP_K` candidates) and PostgreSQL full-text
search (`TOP_K` candidates) are combined with reciprocal rank fusion, keeping the
top `FINAL_K` fused chunks, and each matched section is then expanded to its
chunks (`EXPAND_LIMIT` cap) before the context is sent to the model.
Vector-only candidates below `MIN_SIMILARITY` are marked `FILTERED` and left out.

By default `cmd/rag` rewrites the question into a search query with the chat
model (`internal/rag`), then retrieves over both the original question and the
rewritten query. Each query runs a vector search and a PostgreSQL full-text
search, and the four rankings are fused with reciprocal rank fusion before
section deduplication and expansion. Set `QUERY_REWRITE=false` to retrieve over
the original question only. Either way the model answers the original question.

The original query is kept alongside the rewrite rather than replaced: the
rewrite is lossy, and retrieving over it alone finds fewer of the expected
documents (see the [evaluation guide](evaluation.md)). Fusing both matches the original query's
retrieval while hedging against a bad rewrite.

When `RAG_LLM_RERANK` is set, `cmd/rag` asks the chat model to reorder the fused
candidates by relevance (`reranking.RerankLLM`), keeps the top `FINAL_K`, and
expands those sections before answering — the same reranker the evaluation
compares under `EVAL_LLM_RERANK`. Otherwise it expands the fused sections
directly.

Before answering, `cmd/rag` asks the chat model whether the kept documents can
actually answer the question (`RAG_ANSWERABILITY_GATE`, on by default). If they
cannot, it replies "I do not have enough information." instead of answering.
