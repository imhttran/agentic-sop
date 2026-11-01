# Implemented capabilities

These are shipped and covered by unit tests (and, where noted, the PostgreSQL
integration suite). They are recorded here so the code and this document agree:

- **Pluggable document loader (RAG-006)** — `internal/loader` exposes a
  `Loader` interface plus a `Registry`/`Dispatch`; Markdown and plain-text
  loaders ship today. `cmd/ingest` dispatches by loader, so a new format is added
  by registering a loader, not by editing the ingest path. Unknown formats return
  a clear `no loader for <path>` error.
- **Ingestion provenance and idempotent re-index (RAG-003)** — every chunk stores
  a content hash, embed model, dimension, chunker config, ingest time, and
  language; re-ingesting an unchanged file is a no-op (migrations 002/003).
- **Model/provider independence (RAG-004/RAG-005)** — embedder and generator are
  resolved through `internal/provider` (`EMBED_PROVIDER`/`GEN_PROVIDER`);
  `EMBED_DIM` is validated against the stored column before any write.
- **Multilingual retrieval (RAG-009)** — `CORPUS_LANGUAGE` selects the FTS
  configuration (`NormalizeFTSConfig`; unset → `english`, unsupported → `simple`)
  and is stored per document; the FTS config is a bound SQL parameter.
- **Metadata filtering (RAG-011)** — an optional parameterized `Filter` (source
  prefix with LIKE-escaping, language, ingested-date range) narrows vector search,
  keyword search, and section expansion; a zero filter is byte-identical to the
  unfiltered query.
- **Deterministic context budget (RAG-010)** — `internal/contextbudget` selects a
  subset of ranked documents whose byte-estimated size fits `CONTEXT_BUDGET`
  (higher-rank priority, round-robin fairness, oversize chunks excluded); `0`
  disables it and keeps the chunk-count behaviour.
- **Reranking hardening (RAG-012)** — a malformed, slow, or failed LLM reranker
  degrades to the fused order (bounded by `RERANK_TIMEOUT`) instead of aborting the
  request; the default stays off.
- **Structured citation validation (RAG-013)** — `internal/citations` parses,
  validates, and repairs `[source - section]` citations; `RAG_CITATION_VALIDATION`
  runs it in `cmd/rag` (off by default), reusing the parser that `cmd/eval`
  already used.
- **Structured observability (RAG-014)** — `OBSERVABILITY_FORMAT=json` emits one
  correlated `log/slog` record per run (run ID, stage timings, counts, context
  usage); the default `human` output is unchanged.
- **Security hardening (RAG-015)** — `MAX_QUESTION_BYTES`/`MAX_INPUT_BYTES` fail
  fast with actionable errors; the answer prompt separates trusted instructions
  from untrusted retrieved text (see
  [`docs/operations/prompt-injection.md`](../operations/prompt-injection.md));
  the pre-commit hook runs `govulncheck` when it is installed.

## Phase 23b capabilities

- **PDF text-layer ingestion (RAG-007)** — `internal/loader` registers a PDF
  loader that extracts one section per page from the PDF text layer; a PDF with no
  text layer (a scan) is rejected explicitly until OCR is authorized.
- **Page provenance and page-aware citations (RAG-017)** — a 1-based page number
  flows from the loader through chunking, storage (`documents.page`), retrieval,
  and citation rendering; the contract is
  [`docs/operations/page-provenance.md`](../operations/page-provenance.md).

The work still **not** implemented — OCR for scanned PDFs (RAG-008) and genealogy
entity extraction (RAG-016) — is described in
[`docs/requirements/PRD-Phase-23b.md`](../requirements/PRD-Phase-23b.md) and
[`docs/plans/PLAN-RAG-Phase-23b.md`](../plans/PLAN-RAG-Phase-23b.md).
