# RAG-SOP-004 Phase 2 — Real PostgreSQL/pgvector Retrieval Verification

**Scope:** verify the existing `rag-template` retrieval-only producer against a real
PostgreSQL/pgvector store. Producer-level checks only; SOP-runtime BLOCKED/fallback
behavior is covered by the CLI/unit tests. No BM25-vs-RAG evaluation here.

## Environment
- PostgreSQL **16.15** (Ubuntu 24.04), extension **pgvector 0.6.0** (`vector`).
- Producer: `rag-template/cmd/retrieval` built to `/tmp/retrieval` (commit `8088aaa`).
- Isolated store: schema **`rag_sop`** in `rag_db` via
  `options='-c search_path=rag_sop,public'` — separate from `public` (production data untouched).
- Trusted allowlist: `RAG_RETRIEVAL_ALLOWED_CORPORA=kb`. Credentials are in the producer's
  environment only and are **not** recorded here.

## Test corpus (controlled)
| corpus | source | document_hash | rows |
| --- | --- | --- | --- |
| kb | docs/adr.md | hash-adr | 1 |
| kb | docs/design.md | hash-design | 1 |
| other | docs/other.md | hash-other | 1 |
| (NULL legacy) | docs/legacy.md | — | 1 |

## Commands and results
```
# authorized corpus
printf '{"schema_version":"rag-retrieval/v1","query":"retrieval fallback","top_k":5,"filters":{"corpus":"kb"}}' \
  | DATABASE_URL=<isolated> RAG_RETRIEVAL_ALLOWED_CORPORA=kb /tmp/retrieval
```
**OK** — 2 evidence items, `corpus="kb"`, deterministic `rank` 1/2, `retrieval_method="hybrid"`,
`document_hash` `hash-adr` / `hash-design`, sources `docs/adr.md` / `docs/design.md`,
section `Retrieval`, `truncated:false`, `elapsed_ms:1258`.

```
# unauthorized corpus (allowlist = kb)
... "filters":{"corpus":"other"} ...
```
**UNAUTHORIZED_SCOPE** — the `other`-corpus request is rejected; nothing returned.

```
# unsupported schema
... "schema_version":"v0" ...
```
**UNSUPPORTED_SCHEMA** — rejected.

## Isolation and provenance
- Corpus filtering is a **pre-filter**: the `other` and NULL-corpus (legacy) rows were **never
  returned** for the `kb` request — no cross-corpus or legacy leakage.
- Every returned item carries stable citation provenance: `corpus`, `source`, `section`, `page`,
  `document_hash`, `retrieval_method`, `rank`.

## Latency
- Authorized retrieval `elapsed_ms = 1258` (includes the embedding call to the provider +
  hybrid retrieval); unauthorized/schema rejections `elapsed_ms = 0`.

## Failures / limitations
- None observed for the producer path.
- **Not covered here:** SOP-runtime required-evidence BLOCKED and optional BM25 fallback
  (covered by CLI/unit tests); timeout enforcement (the producer respects `deadline_ms`, not
  separately exercised here); the BM25-vs-RAG effectiveness comparison (Phase 3).
- `rag_template` was **not** modified.
