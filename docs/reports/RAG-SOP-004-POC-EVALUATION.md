# RAG-SOP-004 POC Evaluation — PostgreSQL keyword vs hybrid (real embeddings)

**Recommendation: NO-GO** (do not enable RAG by default). **Operator decision: UNSET.**
Evidence-backed on the frozen corpus/query set below. This is a retrieval-only
measurement; it does not infer downstream agent quality.

## Environment
- PostgreSQL 16.15, pgvector 0.6.0; isolated schema `rag_sop`.
- Real embeddings via `rag-template` ingestion + the Ollama embedder
  (`OLLAMA_EMBED_MODEL` default `nomic-embed-text`, dim 768). **No synthetic embeddings.**

## Corpus (ingested, not synthetic)
- **12 sectioned documents** (`## Section`) ingested with `corpus=kb` using the
  ingestion fix (`cmd/ingest` now reads `RAG_CORPUS`). Verified: **12 rows, 1 corpus,
  12 distinct embeddings** (nonzero, real).

## Method
Identical documents and queries for both modes over `corpus='kb'`:
- **Keyword baseline = PostgreSQL full-text `ts_rank_cd`** (labelled keyword, **NOT BM25**).
- **Hybrid = existing `rag-template` producer** (`cmd/retrieval`, vector+keyword fusion).
- Frozen 10 queries `topicNN` → expected source `dNN.md`; **recall@5, precision@5, MRR**.

## Results (10 queries)
| Mode | recall@5 | precision@5 | MRR | latency p50/p95 |
| --- | --- | --- | --- | --- |
| keyword (`ts_rank_cd`) | 1.0 | 0.2 | 1.0 | n/a (SQL) |
| hybrid (producer) | **1.0** | **0.2** | **1.0** | **26 ms / 27 ms** |

- **Queries keyword missed but hybrid found: none.**
- **Observed improvement: none.** Hybrid matched keyword exactly on this set.
- **Observed regression: none.**
- Corpus isolation (Phase 2): `corpus:"other"` ⇒ `UNAUTHORIZED_SCOPE`; legacy NULL rows excluded.
- Citations: every item carries `corpus/source/section/page/document_hash/retrieval_method/rank`.

## Operational cost
- Hybrid query latency **~26 ms** (p50) / **~27 ms** (p95) against the local corpus
  (embedding + pgvector + full-text fusion). Context bytes bounded by the configured budget.

## Limitations and confidence
- Small controlled corpus (12 docs) and single-keyword queries give keyword search no
  headroom to fail, so a semantic advantage cannot show here. Confidence that hybrid
  **does not regress**: high. Confidence about performance on **adversarial/natural-language**
  queries: low (untested).
- The predeclared pilot GO threshold requires **at least one meaningful improvement over
  keyword on the frozen historical-knowledge subset**; none was observed.

## Recommendation
**NO-GO** for enabling RAG by default, on the evidence that hybrid showed **no measurable
retrieval improvement** over keyword and the pilot value threshold was not met (no
regression and isolation held, but value is unproven). If the operator wants to pursue a
conditional pilot, the missing evidence is a natural-language/adversarial query set where
keyword search **misses** relevant documents.

## Reproducible commands
- Ingest: `DATABASE_URL="$DSN" RAG_CORPUS=kb go -C rag-template run ./cmd/ingest <doc.md>`
- Keyword: `psql "$DSN" -c "select source from documents where corpus='kb' order by
  ts_rank_cd(to_tsvector('english',coalesce(section,'')||' '||content),
  websearch_to_tsquery('english','topic01')) desc limit 5;"`
- Hybrid: `printf '{"schema_version":"rag-retrieval/v1","query":"topic01","top_k":5,
  "filters":{"corpus":"kb"}}' | DATABASE_URL="$DSN" RAG_RETRIEVAL_ALLOWED_CORPORA=kb /tmp/retrieval`
- (`$DSN` = isolated `rag_sop` connection; credentials not recorded.)
