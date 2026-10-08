---
name: rag-ingest
description: |-
  Design a robust RAG ingestion pipeline: parsing and chunking, bounded embedding
  concurrency, retry/backoff classification, transactional replacement, provenance and
  idempotency, and recovery that never leaves a partial write. Read-only: it produces a
  design; apply the code through the governed /sop-implement lifecycle.
disable-model-invocation: false
---

# /rag-ingest — designing an ingestion pipeline that fails safely

Capability: `plan` (read-only). This is a RAG-domain adapter: it fixes the read-only
`plan` capability and delegates to the canonical SOP skill, which owns the contract. It
specifies the pipeline and never writes, edits, or runs it.

```bash
sop prompt --capability plan "<the operator's ingestion request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: writing or running the ingestion code MUST NOT happen here. Apply the
  change through `/sop-implement`, which runs SOP's governed implementation lifecycle.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not design or run the pipeline yourself.

The canonical `sop` skill owns the full contract and the capability map.

## Parsing and chunking

- Parse structure first (headings and sections), then chunk within a section, so a
  citation can name the section it came from.
- Chunk by a defined unit with overlap; make size and overlap configuration, not
  constants baked into the code. Handle empty, very short, and unicode input.
- Every chunk carries provenance — source, section, chunk_index — because that is what a
  citation resolves to.

## Embedding concurrency limits

- Embedding is the slow, rate-limited, failure-prone step. Bound in-flight requests with
  a fixed worker pool, never "one goroutine per chunk".
- Bound retries and total time per document so a single hung chunk cannot stall the run.
- Make concurrency and batch size settings, and record the values with the run so a
  result is reproducible.

## Retry/backoff classification

- Classify a failure before retrying it. A transient transport error or a rate limit is
  retryable with backoff; a deterministic input error (bad request, wrong dimension) or a
  parse error is not — retrying it just burns the budget.
- Use exponential backoff with jitter and a hard attempt cap, counted per chunk.
- Never turn "retried and gave up" into success. An incomplete document must be reported
  as incomplete, not as ingested.

## Transactional replacement

- Ingesting a file must be atomic: the delete of that source's old chunks and the insert
  of the new ones happen in one transaction keyed by source. Re-running the same file
  replaces it rather than appending, so the store never accumulates duplicates.
- Validate before commit: widths match the column, counts are non-zero, and the batch is
  internally consistent. Commit only a complete document; otherwise roll back.
- Keep "replace one source" and "truncate the whole table" as different operations with
  different blast radius. The second is a deliberate operator action, never a side effect
  of ingestion.

## Provenance and idempotency

- Each row records where it came from (source, section, chunk_index) and, ideally, a
  content hash and the embedding width, so a re-ingest can be proven equivalent.
- An idempotent re-ingest of unchanged input produces the same logical state. Use a
  natural key (source plus chunk_index) or a hash to detect a no-op.
- Never key idempotency on a random id or a timestamp.

## Failure recovery without partial writes

- A failed embedding leaves the previous version of the document untouched, because the
  replacement is transactional. Recovery means "retry the document", not "repair
  half-written rows".
- A crash mid-run leaves the store consistent: some sources fully replaced, none half
  replaced. Resuming re-runs the unfinished sources idempotently.
- Surface a per-document outcome — replaced, unchanged, or failed with a reason — so a
  partial run is visible instead of assumed complete.

## Example: rag-template

`rag-template` is a small Go + pgvector RAG project used here only as a reference;
nothing in this skill depends on it.

- `cmd/ingest` reads a markdown file, splits each section into overlapping chunks, embeds
  each chunk, and stores it; `internal/ingestion.ReplaceDocument` does a delete-then-insert
  replacement so re-ingesting does not duplicate rows.
- Embedding concurrency and retry policy are the project's own settings; the connection
  comes from `DATABASE_URL` and the width from `EMBED_DIM`. None are hardcoded here.

## What this skill must not do

- Do not write, run, or edit ingestion code to satisfy the request; design it and hand the
  change to `/sop-implement`.
- Do not run a destructive store operation (drop or truncate) outside a deliberate
  operator action, and never against a database you did not choose.
- Do not commit, push, or merge on the operator's behalf, and do not approve or decline on
  their behalf.
- Do not choose a provider, a model class, a model, or a connection string.
- Do not mark a document ingested until the transaction committed: a pipeline or model
  "success" claim is not verified mutation.
- Do not reimplement SOP's routing, approval, or lifecycle policy.

## Failure behavior

Report SOP's result as-is. An ingestion design is not a pass; a completed ingestion is one
only when SOP's deterministic validation confirms the committed state.
