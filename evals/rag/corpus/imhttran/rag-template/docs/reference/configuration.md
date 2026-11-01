# Configuration

All three commands read the same settings from the environment. The defaults
work with the bundled `docker-compose.yml` and a local Ollama.

`.env.example` lists every setting; copy it to `.env` and load it into your
shell (Go does not read `.env` files on its own):

```bash
cp .env.example .env
set -a; source .env; set +a
```

| Variable                  | Default                                                 | Used by        |
| ------------------------- | ------------------------------------------------------- | -------------- |
| `OLLAMA_URL`              | `http://localhost:11434`                                | all            |
| `OLLAMA_EMBED_MODEL`      | `nomic-embed-text`                                      | all            |
| `OLLAMA_CHAT_MODEL`       | `qwen3.8:27b-mlx`                                       | rag, eval      |
| `DATABASE_URL`            | `postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable` | all            |
| `EMBED_DIM`               | `768`                                                   | all            |
| `EMBED_PROVIDER`          | `ollama`                                                | all            |
| `GEN_PROVIDER`            | `ollama`                                                | all            |
| `CHUNK_SIZE`              | `50`                                                    | ingest         |
| `CHUNK_OVERLAP`           | `20`                                                    | ingest         |
| `EMBED_WORKERS`           | `4`                                                     | ingest         |
| `EMBED_RETRIES`           | `3`                                                     | ingest         |
| `CORPUS_LANGUAGE`         | _(empty — baseline `english` FTS)_                      | ingest, rag    |
| `TOP_K`                   | `4`                                                     | rag, eval      |
| `FINAL_K`                 | `3`                                                     | rag, eval      |
| `EXPAND_LIMIT`            | `20`                                                    | rag, eval      |
| `MIN_SIMILARITY`          | `0.6`                                                   | rag, eval      |
| `CONTEXT_BUDGET`          | `0` (disabled)                                          | rag            |
| `MAX_QUESTION_BYTES`      | `0` (disabled)                                          | rag            |
| `MAX_INPUT_BYTES`         | `0` (disabled)                                          | rag            |
| `REQUEST_TIMEOUT`         | `5m`                                                    | all            |
| `RAG_ANSWERABILITY_GATE`  | `true`                                                  | rag            |
| `RAG_LLM_RERANK`          | `false`                                                 | rag            |
| `RAG_CITATION_VALIDATION` | `false`                                                 | rag            |
| `RERANK_TIMEOUT`          | `0s` (disabled)                                         | rag, eval      |
| `OBSERVABILITY_FORMAT`    | `human`                                                 | rag            |
| `QUERY_REWRITE`           | `true`                                                  | rag, eval      |
| `EVAL_LEXICAL_RERANK`     | `false`                                                 | eval           |
| `EVAL_LLM_RERANK`         | `false`                                                 | eval           |
| `EVAL_ANSWERABILITY_GATE` | `false`                                                 | eval           |
| `EVAL_FACT_JUDGE`         | `false`                                                 | eval           |
| `EVAL_REWRITE_ONLY`       | `false`                                                 | eval           |
| `QUESTION`                | _(none — pass it as an argument, or type it)_           | rag            |

`all` = every command; `rag` = `cmd/rag` only; `eval` = `cmd/eval` only;
`ingest` = `cmd/ingest` only; `rag, eval` = both commands.

### Post-gap-closure settings

The settings below were added by the RAG gap-closure work (RAG-006…RAG-015).
Each defaults to the pre-change behaviour, so an unset value changes nothing:

- **`CORPUS_LANGUAGE`** (`ingest`, `rag`) — BCP-47 tag selecting the PostgreSQL
  full-text search configuration for keyword retrieval and stored per document.
  Empty (default) keeps `english`; a supported language uses its configuration;
  an unsupported one falls back to `simple` (see `internal/retrieval.NormalizeFTSConfig`).
- **`CONTEXT_BUDGET`** (`rag`) — byte-based cap on the context sent to the model.
  `0` (default) keeps the chunk-count behaviour. See
  [`internal/contextbudget`](../../internal/contextbudget).
- **`RAG_CITATION_VALIDATION`** (`rag`) — validates the model's `[source - section]`
  citations against the retrieved documents and repairs an answer whose citations
  do not resolve (one bounded re-prompt, then strip). Off by default; when off the
  answer path is unchanged. See [`internal/citations`](../../internal/citations).
- **`MAX_QUESTION_BYTES` / `MAX_INPUT_BYTES`** (`rag`) — fail-fast size limits for
  the question and the assembled retrieved context. `0` (default) disables each
  check; an oversized input errors before any model call, naming the variable.
- **`RERANK_TIMEOUT`** (`rag`, `eval`) — bounds the LLM reranker; `0s` (default)
  disables the guard. A malformed, slow, or failed reranker degrades to the fused
  order instead of failing the request.
- **`OBSERVABILITY_FORMAT`** (`rag`) — `human` (default) keeps the existing stage
  output; `json` emits one correlated `log/slog` record per run with a run ID,
  stage timings, retrieved/filtered counts, and context usage. See
  [`internal/observability`](../../internal/observability).
- **`EMBED_PROVIDER` / `GEN_PROVIDER`** (`all`) — provider-registry selectors
  (`ollama` by default). See [`internal/provider`](../../internal/provider).

`EMBED_DIM` must match the stored `documents.embedding` column. The guard
`ingestion.CheckEmbeddingDim` fails fast otherwise and points at
[`docs/operations/embedding-dimension.md`](../operations/embedding-dimension.md),
the operator-run procedure for changing the dimension; changing it also
requires re-ingesting the corpus.
