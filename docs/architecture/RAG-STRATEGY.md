# RAG Retrieval Strategy — As Built

**Type:** Architecture description (non-normative).
**Authority:** current, as of 2026-11-02.
**Describes:** rag-template-rust `cmd/retrieval` at `acd1fb911ac8bdd4f425b61df16a495f4a5047c9`, and agentic-sop `internal/ragclient` and `internal/context`.
**Related:** design in [RAG-RETRIEVAL-INTEGRATION.md](RAG-RETRIEVAL-INTEGRATION.md), contract in [../specs/RAG-RETRIEVAL-CONTRACT.md](../specs/RAG-RETRIEVAL-CONTRACT.md), settings in [../specs/RAG-CONFIGURATION.md](../specs/RAG-CONFIGURATION.md), evaluation in [../specs/RAG-EVALUATION.md](../specs/RAG-EVALUATION.md).

This page records how SOP's RAG works in the code today, and what the first measured evaluation found. The design documents describe intent. Where they differ from this page, the code is what runs.

RAG has two halves. The producer, in rag-template-rust, finds evidence. SOP decides what to do with it.

## Ingestion

The ingester (`cmd/ingest`) reads one Markdown file at a time.

- It splits the file on `##` headings into sections. A file with no `##` heading is rejected with "no sections found" and is not indexed.
- It splits each section into chunks of about 50 words, with 20 words of overlap.
- It embeds each chunk with Ollama `nomic-embed-text` (768 dimensions).
- It stores each chunk in PostgreSQL with pgvector, with its source path, section heading, corpus name and content hash.
- A source's identity is its path relative to the nearest folder that contains `.git`.
- Re-ingesting an unchanged file is skipped.

## Retrieval in the producer

The retrieval-only producer (`cmd/retrieval`) answers one request per process.

1. It embeds the query with the same model.
2. It runs a vector search and a PostgreSQL full-text search, each for four times the requested `top_k` candidates.
3. It merges the two rankings with reciprocal rank fusion, with `k = 60`.
4. It returns the top `top_k` chunks as evidence, labeled `retrieval_method: hybrid`.

The corpus filter runs inside the SQL, before ranking and limits. A chunk from another corpus never takes a result slot, and rows with no corpus are never returned.

rag-template also has query rewrite, a minimum-similarity floor, small-to-big section expansion, reranking and answer generation. The retrieval-only producer turns all of these off. SOP receives raw, attributed evidence and no generated text.

## How SOP uses the evidence

RAG is off by default. It runs only when `SOP_RAG_ENABLED` is set and a producer executable is configured.

- SOP runs the producer as a local process, with no shell. It sends one JSON request on stdin and reads one JSON response from stdout, under the versioned contract `rag-retrieval/v1`. The defaults are a 3 s timeout and `top_k` 5.
- SOP validates the response strictly. An unknown schema version, evidence from a corpus outside the allowlist, oversized text or malformed JSON rejects the whole response. Nothing is partly accepted.
- `sopctx.KnowledgeItems` turns evidence into context items at priority 6, the lowest. Task, lifecycle and repository items rank above it. Items are deduplicated, labeled as untrusted retrieved evidence, and capped at 4 KB in total.
- Retrieved text never becomes an instruction, an approval or a completion verdict.

When retrieval fails:

- for an ordinary task, SOP falls back to BM25-only and records the reason in `rag.json` telemetry;
- for a task that declares required evidence, SOP blocks the task with `REQUIRED_EVIDENCE_MISSING`.

The operational fixtures in `internal/cli/rag_fixtures_test.go` cover both paths. See [../reports/rag-eval/fixtures.md](../reports/rag-eval/fixtures.md).

## Measured result (2026-11-01)

These numbers come from three identical runs of `rag-eval run` against the real producer. The runs are in `docs/reports/rag-eval/20261102T021652Z`, `20261102T021737Z` and `20261102T021822Z`. The dataset is the frozen corpus in `evals/rag` (513 of 550 documents indexed; the 37 without a `##` heading were excluded, and none of them is expected evidence). Scoring is at document level. The rollout decision is recorded separately in [../specs/RAG-SOP-004-DECISION.md](../specs/RAG-SOP-004-DECISION.md) and was still pending at the time of writing.

| Arm | What it is | recall@5, 52 historical questions |
| --- | --- | --- |
| A | SOP's BM25 today, over file paths and document class | 0.385 |
| B | BM25 plus RAG, through SOP's context assembly | 0.385 |
| C | RAG alone (diagnostic) | 0.468 |
| D | SOP's BM25 over full document text (diagnostic) | 0.635 |

Arm B equals arm A exactly. SOP ranks repository items above RAG items, so with five BM25 hits the top five are always BM25. The RAG evidence is in the context, at positions six to eight. Adding RAG cost about 300 ms at p95. Injected text in retrieved evidence was always placed below the task.

## Known weaknesses

- SOP's BM25 indexes file paths and document class, not document text. Reading the text (arm D) gave the largest gain measured.
- Neither retriever uses document authority. `repoindex` already classifies documents as current or historical, but ranking ignores it, so superseded plans and run reports compete with current specs. Most misses had their top five filled by history plans and run reports.
- The chunker only splits on `##`. It skips heading-less files and makes long chunks from large reference documents.
- Several chunks from the same file can fill RAG's top five. In the evaluation the top five held 3.9 distinct documents on average.
- RAG items rank below every BM25 hit, so RAG cannot change what lands in the top of SOP's context.

## Options under consideration

These are candidates, not decisions.

- Index document text in SOP's BM25.
- Down-rank documents that `repoindex` marks historical, in both retrievers.
- Split chunks on `###` as well, and keep at most one chunk per document in the top five.
- Reconsider RAG's context priority only after the first two changes are measured.
