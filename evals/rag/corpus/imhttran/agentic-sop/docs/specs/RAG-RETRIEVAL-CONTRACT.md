# RAG Retrieval Contract (retrieval-only)

**Type:** Normative interface specification (RAG-SOP-001 deliverable).
**Status:** APPROVED (architecture) — operator-approved 2026-10-15 for RAG-SOP-002 implementation (documentation record; not a SOP lifecycle approval).
**Plan:** [`docs/plans/PLAN-RAG-SOP-Integration.md`](../plans/PLAN-RAG-SOP-Integration.md).
**Related:** [architecture](../architecture/RAG-RETRIEVAL-INTEGRATION.md),
[threat model](RAG-SECURITY-THREAT-MODEL.md), [configuration](RAG-CONFIGURATION.md),
[evaluation](RAG-EVALUATION.md).

This is the versioned protocol between the SOP RAG client and the `rag-template`
retrieval-only entry point. It returns **raw retrieved evidence**, never a generated
answer or model judgment. It adds no model decision to the retrieval path.

## 1. Versioning

- `schema_version` is a string, currently `"rag-retrieval/v1"`. Both sides MUST reject a
  request/response whose `schema_version` they do not implement (`error_code:
  UNSUPPORTED_SCHEMA`).
- The version is bumped on any breaking change to fields, semantics, or error codes.

## 2. Request

```json
{
  "schema_version": "rag-retrieval/v1",
  "query": "architecture decision record for retrieval fallback",
  "top_k": 5,
  "filters": {
    "corpus": "sop-engineering-kb",
    "source_prefix": "imhttran/rag-template/docs/",
    "language": "english",
    "ingested_from": "2026-01-01T00:00:00Z",
    "ingested_to": null
  },
  "deadline_ms": 3000
}
```

| Field | Type | Notes |
| --- | --- | --- |
| `schema_version` | string | Required. |
| `query` | string | Required. UTF-8; validated for byte length against `SOP_RAG_MAX_QUERY_BYTES`. Never shell-interpolated. |
| `top_k` | int | 1…`SOP_RAG_MAX_RESULTS`; hard-capped by the server. |
| `filters.corpus` | string | **Required for retrieval.** The corpus identity (see §8). MUST be a member of the trusted `SOP_RAG_ALLOWED_CORPORA` allowlist; otherwise `UNAUTHORIZED_SCOPE`. |
| `filters.source_prefix` | string | Optional finer scoping; maps to `retrieval.Filter.SourcePrefix`. |
| `filters.language` | string | Optional; maps to `retrieval.Filter.Language`. |
| `filters.ingested_from` / `ingested_to` | timestamp | Optional; maps to `retrieval.Filter.IngestedFrom`/`IngestedTo`. |
| `deadline_ms` | int | Caller deadline; server MUST return by it or `TIMEOUT`. |

`filters` maps onto `rag-template` `retrieval.Filter` (`SourcePrefix`, `Language`,
`IngestedFrom`, `IngestedTo`) plus the **new `Corpus`** dimension added by the corpus
migration (§8).

## 2.1 Filter precedence — filtering happens before ranking and limits

Filtering is a **pre-filter**, not a post-filter. The `corpus` (and `source_prefix` /
`language` / date) predicate narrows the candidate rows **before** nearest-neighbour
ordering and **before** the `top_k` limit is applied. This is already how
`rag-template` composes its queries: `Retriever.SearchFiltered`,
`KeywordSearchFiltered`, and `SectionChunksFiltered` push the filter into the SQL
`WHERE` clause ahead of `ORDER BY … LIMIT $topK`, so a filter can never be bypassed by a
limit. RAG-SOP-002 MUST extend `filterFragment` with the `corpus` predicate and MUST NOT
introduce a post-filter.

Consequence: an out-of-corpus row can never consume a result slot, and an out-of-corpus
row can never be returned even when `top_k` is large.

## 3. Response

```json
{
  "schema_version": "rag-retrieval/v1",
  "status": "OK",
  "evidence": [
    {
      "id": "42",
      "corpus": "sop-engineering-kb",
      "source": "imhttran/rag-template/docs/architecture/overview.md",
      "document_hash": "sha256:…",
      "section": "Retrieval",
      "page": 0,
      "text": "…raw chunk text…",
      "retrieval_method": "hybrid",
      "score": 0.0312,
      "rank": 1
    }
  ],
  "truncated": false,
  "diagnostics": { "elapsed_ms": 87, "error_code": "" }
}
```

Field mapping from `rag-template` `retrieval.Document` and the schema:

| Contract field | Source |
| --- | --- |
| `id` | `Document.ID` |
| `corpus` | `documents.corpus` (new column; echoed from the filter) |
| `source` | `Document.Source` (validated to a relative identity, never a trusted absolute path) |
| `document_hash` | **projected** `documents.content_hash` — the existing ingestion fingerprint; RAG-SOP-002 MUST add it to the retrieval `SELECT` and to `retrieval.Document` (§5) |
| `section` | `Document.Section` |
| `page` | `Document.Page` (0 = unknown) |
| `text` | `Document.Content` |
| `retrieval_method` | the pipeline stage that produced the item: `vector` \| `keyword` \| `hybrid` (derived from the stage; see §6) |
| `score` | one of `Document.Similarity` / `KeywordScore` / `FusionScore` — never mixed |
| `rank` | 1-based, stable position in the returned ordering (see §6) |

`score` and `rank` are **advisory within the RAG corpus only**; SOP MUST NOT compare them
with BM25 scores.

## 4. Size ceilings (fail closed)

- Request: `query` ≤ `SOP_RAG_MAX_QUERY_BYTES` (default 4096); otherwise `MALFORMED`.
- Response: total JSON ≤ `SOP_RAG_MAX_RESPONSE_BYTES` (default 256 KiB); `evidence`
  ≤ `top_k`; each `text` ≤ `SOP_RAG_MAX_CHUNK_BYTES` (default 16 KiB). Oversize →
  `OVERSIZE`, and SOP drops the whole response rather than trusting a partial parse.

## 5. `content_hash` projection (D5)

`documents.content_hash` exists (`migrations/002_ingestion_metadata.sql`) and is written by
ingestion (`ingestion.ContentHash`, "fingerprint of the file's bytes combined with the …"),
but the retrieval queries do **not** currently select it and `retrieval.Document` has no
hash field. RAG-SOP-002 MUST:

1. add `content_hash` to the three retrieval `SELECT`s (`SearchFiltered`,
   `KeywordSearchFiltered`, `SectionChunksFiltered`);
2. add the field to `retrieval.Document`;
3. surface it as `document_hash` in the response.

No new hash is invented; the existing ingestion fingerprint is projected. When a row's
`content_hash` is NULL (legacy rows), the response carries `document_hash: ""` and SOP
treats the item as **lower-confidence** (it is still untrusted evidence, never an
identity for dedup against a hashed item).

## 6. `retrieval_method` and `rank`

- `retrieval_method` is the stage that produced the item: `vector` (vector-only),
  `keyword` (full-text-only), or `hybrid` (fused vector+keyword). The entry point labels
  each item from the pipeline stage it came out of; it never infers a method from a score.
- `rank` is the 1-based position **within this response's ordering**, stable for identical
  input+corpus. Ties are broken deterministically (source, section, chunk index) so rank is
  reproducible.

## 7. Errors (typed taxonomy)

| `error_code` | Meaning | SOP behaviour |
| --- | --- | --- |
| `""` / `OK` | success (possibly empty `evidence`) | use evidence |
| `EMPTY` | corpus returned no match | BM25-only; not an error |
| `TIMEOUT` | deadline exceeded | optional → BM25-only fallback + reason; required → fail closed (§9) |
| `UNAVAILABLE` | endpoint/executable missing or DB/model down | optional → fallback; required → fail closed |
| `MALFORMED` | invalid JSON / unreadable dependency | fallback; never partial-parse |
| `OVERSIZE` | ceilings exceeded | fallback; never partial-parse |
| `UNAUTHORIZED_SCOPE` | corpus not allowlisted / missing | denial; never widen scope |
| `UNSUPPORTED_SCHEMA` | unknown `schema_version` | fallback + diagnostic |
| `INTERNAL` | server-side failure | fallback; secret-free diagnostic |

The client MUST map transport errors (non-zero exit, non-JSON stdout, connection refused,
TLS error) to `UNAVAILABLE`/`MALFORMED` and MUST NOT let stdout diagnostics corrupt the
JSON (the entry point writes logs to stderr only).

## 8. Corpus identity, migration, and legacy records (D2)

- **Identity.** `corpus` is a new `text` column on `documents`, added by a
  **backward-compatible** migration (`ALTER TABLE documents ADD COLUMN IF NOT EXISTS corpus
  text;` plus an index `CREATE INDEX IF NOT EXISTS documents_corpus_idx ON documents
  (corpus);`), consistent with the existing `0NN_*.sql` idempotent migration style.
- **Enforcement.** Corpus filtering is applied in **keyword and vector retrieval**
  (`SearchFiltered`, `KeywordSearchFiltered`, and `SectionChunksFiltered`) through the same
  `Filter` mechanism, as a pre-filter (§2.1).
- **Legacy records (fail closed).** Rows written before the migration have `corpus IS NULL`.
  A request that filters on a corpus MUST NOT return NULL-corpus rows: the predicate is an
  equality match, so legacy rows are excluded until re-ingested/backfilled. NULL always
  means "not in any corpus", never "matches everything". The ingestion path MUST set
  `corpus` for new rows; a backfill is an explicit operator action.
- **Trusted allowlist.** `SOP_RAG_ALLOWED_CORPORA` is the operator-controlled allowlist; an
  empty allowlist makes retrieval impossible (fail closed). The server also validates the
  requested corpus against its own allowlist; either side rejecting yields
  `UNAUTHORIZED_SCOPE`.

## 9. Fallback and required evidence (D4)

- **Optional (default).** Any error → deterministic, observable **BM25-only** fallback with
  a recorded reason (`attempted`, `used`, `failed`, `fallback_reason`, durations, byte
  counts). Existing behaviour is unchanged.
- **Required evidence (fail closed, existing lifecycle).** A task MAY declare required
  evidence (per-task `required_evidence`: an allowlist of `corpus` and/or `source`
  identities). When it does and RAG cannot satisfy it, the task MUST NOT reach `LOCAL_DONE`;
  it is recorded **`BLOCKED`** — the existing `domain.BLOCKED` task status — under the
  existing lifecycle. No new status is invented. Recovery, if appropriate, uses the
  existing designated-approver waiver path (`sop outcome --waive`); retrieval still cannot
  authorise anything.
- **Exact blocked reason:** the pilot reuses the existing "automation/configuration block,
  not a human approval" pattern (as with `VALIDATION_NOT_CONFIGURED`/`NO_PROGRESS`). If no
  existing `BlockedReason` value fits semantically, RAG-SOP-003 adds one through the normal
  change process; RAG-SOP-001 does not invent a state.

## 10. Answer-path exclusion (initial adapter)

The retrieval-only entry point MUST NOT import or invoke answer generation, answerability,
or model reranking. Explicitly out of scope for the initial adapter:
`cmd/rag`, `internal/rag` (answer path, e.g. `answer.go`/`rewrite.go`), `internal/answerability`,
`internal/generation`, `internal/reranking`. The adapter uses `internal/retrieval` only.

## 11. What this contract is NOT

- Not an answer generator: no `generated_answer`, no answerability/verdict fields.
- Not authoritative: it cannot satisfy acceptance checks or set approval/lifecycle state.
- Not a score bridge: it never reconciles BM25 and vector numeric scores.
