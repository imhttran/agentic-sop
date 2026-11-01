# Page provenance contract (RAG-017)

**Status:** authoritative design contract for Phase 23b. Normative for RAG-017 and
RAG-007. Additive and backward-compatible: formats without pages behave exactly as
today.

## 1. Scope

Formats that have pages (PDF) carry a **1-based page number** from the loader
through ingestion, retrieval, and citation rendering. Formats without pages
(Markdown, plain text) carry **no page** (`0` in Go, `NULL` in PostgreSQL) and are
byte-identical to the pre-RAG-017 behaviour.

## 2. Data model

| Layer | Type / artifact | Field | Unset value |
|-------|-----------------|-------|-------------|
| Extraction | `document.Section` | `Page int` (`> 0` = page) | `0` |
| Chunking | `chunking.Chunk` | `Page int` (copied from the section) | `0` |
| Schema | `migrations/004_page.sql` | `documents.page integer` (nullable) | `NULL` |
| Ingestion | SQL `INSERT` | writes `page` via `NULLIF(0)` → `NULL` | `NULL` |
| Retrieval | `retrieval.Document` | `Page int` (read `COALESCE(page, 0)`) | `0` |
| Rendering | `rag.FormatContext` | `Page: N` line **only when `Page > 0`** | omitted |
| Citations | `internal/citations` | optional `- p.N` segment | absent |

`migrations/004_page.sql` is additive and idempotent:

```sql
ALTER TABLE documents ADD COLUMN IF NOT EXISTS page integer;
```

No default, no backfill (pre-existing rows read `NULL`). No destructive change.

## 3. Flow

```
loader (PDF: one Section per page, Page = n)
  → chunking.FromSections (Chunk.Page = Section.Page)
  → ingestion INSERT (page = NULL when 0, else n)
  → retrieval (Document.Page = COALESCE(page, 0))
  → rag.FormatContext ("Page: n" only when Page > 0)
  → citations ("[source - section - p.n]" is valid iff a retrieved doc matches
     source+section AND has page n)
```

## 4. Citation form

- Legacy form `[source - section]` stays **valid**; it makes no page claim. A
  retrieved document with or without a page satisfies it.
- Page form `[source - section - p.N]` (`N` a positive integer) is valid **only if**
  a retrieved document matches the source and section **and** carries page `N`.
- An invalid page citation is treated like any other invalid citation: it is never
  rendered as valid (RAG-013 repairs it — one bounded re-prompt, then strip).
- Parsing: split the bracket on `" - "`; two segments = legacy, three segments where
  the third matches `p<digits>` = page form. Anything else is not a citation
  (unchanged).

## 5. Invariants

1. **Backward compatibility** — `Page == 0` everywhere yields output identical to
   before RAG-017 (Markdown/text, existing rows, existing answers).
2. **Source identity preserved** — the page never changes `source`; `a/doc.pdf` and
   `b/doc.pdf` stay distinct.
3. **Original text preserved** — the loader stores the page's extracted text as-is;
   the page is metadata, not a content edit.
4. **Idempotency** — the content fingerprint (RAG-003) still covers file bytes +
   chunker config, so re-ingesting an unchanged PDF is a no-op; the page is derived
   from the file, not stored in the fingerprint.
5. **Additive schema only** — migration 004 adds a nullable column and nothing else.

## 6. Out of scope

- OCR (RAG-008): a scanned PDF has no text layer and is rejected explicitly until
  OCR is authorized.
- Genealogy entity extraction: unrelated to this contract.
