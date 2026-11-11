# LLM08 — live RAG run: blocker (OWASP gap 14)

**Status:** NOT_RUN, blocked. Recorded so the next attempt starts from the real state.

## What the gap needs

A live run against a disposable PostgreSQL with pgvector and an embedding model, with
two isolated corpora, cross-corpus and unauthorized-scope cases, a deadline case, a
poisoned chunk, and a positive control (gap 14 in
[OWASP-LLM-GAP-PROMPTS.md](OWASP-LLM-GAP-PROMPTS.md)).

## What exists on this machine

- PostgreSQL 16 and 17 and pgvector, installed through Homebrew.
- The embedding model `nomic-embed-text` in the local Ollama server.
- The producer repository `rag-template` at `projects/rag-template` (commit `4f8f821`).

## Why the run cannot start

SOP's side is in place: `internal/ragclient` is the out-of-process adapter, and
`docs/architecture/RAG-RETRIEVAL-INTEGRATION.md` requires a versioned, retrieval-only
request to a "rag-template retrieval-only entrypoint". The producer does not implement
it. A search of `rag-template` (Go and Markdown) finds the phrase only in
`docs/plans/PLAN-RAG-Graph-Retrieval.md`. Its `cmd/` contains `eval`, `ingest` and `rag`,
and the architecture doc itself records this as Assumption A2.

Without that entrypoint, the client has no real endpoint to call, so any "live" case would
exercise a stub, not the producer. That would not count as evidence.

## What unblocks it

An operator decision on the producer side: implement the retrieval-only entrypoint in
`rag-template` to the versioned protocol, in that repository, under its own review. After
that, the run itself is straightforward with the tools above.

## Not changed

No retrieval default, no SOP code, and no producer code.
