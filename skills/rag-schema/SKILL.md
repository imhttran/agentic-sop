---
name: rag-schema
description: |-
  Inspect and evolve a pgvector (PostgreSQL + vector) RAG schema without guessing:
  read the real embedding column type, interpret its typmod correctly, reconcile it
  with the embedding dimension, and design a safe, operator-controlled migration with
  data-integrity checks. Read-only: it produces a plan; use /sop-implement to apply a
  repository change, and run the database change as an operator.
disable-model-invocation: false
---

# /rag-schema — pgvector schema inspection and safe evolution

Capability: `plan` (read-only). This is a RAG-domain adapter: it fixes the read-only
`plan` capability and delegates to the canonical SOP skill, which owns the contract. It
adds a pgvector-specific checklist and never edits the repository or the database itself.

```bash
sop prompt --capability plan "<the operator's schema request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: inspecting a schema and planning a migration MUST NOT modify the repository
  or the database. Applying either is a separate step (see below).
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not inspect or change the schema yourself.

The canonical `sop` skill owns the full contract and the capability map.

## Inspect the live schema — never assume

The stored column width is the contract, not the README. Read it from the catalog. The
queries below are PostgreSQL data, not commands this skill runs; a human or the
SOP-governed implementation step runs them against a database it chose. Use the read
only replica or a throwaway copy when the store is large or busy.

```sql
-- The rendered type, the raw typmod, nullability, and the default.
SELECT format_type(a.atttypid, a.atttypmod) AS column_type,
       a.atttypmod                          AS typmod,
       a.attnotnull                        AS not_null,
       pg_get_expr(d.adbin, d.adrelid)     AS default_expr
FROM pg_attribute AS a
JOIN pg_class     AS c ON c.oid = a.attrelid
JOIN pg_namespace AS n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef AS d ON d.adrelid = a.attrelid AND d.adnum = a.attnum
WHERE c.relname = 'documents'
  AND n.nspname = ANY (current_schemas(false))
  AND a.attname = 'embedding'
  AND NOT a.attisdropped;

-- The vector index and its operator class, which must match the query operator.
SELECT i.relname  AS index_name,
       am.amname   AS index_method,
       pg_get_indexdef(x.indexrelid) AS index_def
FROM pg_index AS x
JOIN pg_class AS i ON i.oid = x.indexrelid
JOIN pg_class AS t ON t.oid = x.indrelid
JOIN pg_am    AS am ON am.oid = i.relam
WHERE t.relname = 'documents';
```

Also read the row count and the distinct widths actually stored, so a partly migrated
table is visible rather than inferred:

```sql
SELECT count(*)                    AS rows,
       min(vector_dims(embedding)) AS min_dim,
       max(vector_dims(embedding)) AS max_dim
FROM documents;
```

## Interpret the vector typmod correctly

- `format_type(a.atttypid, a.atttypmod)` renders the human type, for example
  `vector(768)`. Trust that string for the declared width.
- pgvector encodes the dimension **directly** in `atttypmod`: for `vector(768)`,
  `atttypmod` is `768`. Do not subtract a header offset. For comparison, `varchar(n)`
  stores `n + 4`; the offset differs by type. Never hand-derive a dimension from
  `atttypmod` — render the type instead.
- The index operator class must match the distance operator the queries use:
  `vector_cosine_ops` pairs with `<=>`. A column and an index that disagree about the
  dimension or the operator are the two ways a "working" schema silently degrades.

## Embedding-dimension compatibility

- One number is authoritative per store: the column's declared width. Everything else —
  the configured dimension (for example the `EMBED_DIM` setting), the embedder's output
  width, and the query vector's width — must equal it.
- A vector of the wrong width does not "score poorly"; it fails the cast to the column
  type. A store holding more than one width is mid-migration.
- A fast-fail guard is worth having: compare the configured dimension against the column
  before embedding, and name both numbers and the fix when they disagree. In the example
  repository (`rag-template`) this guard is `ingestion.CheckEmbeddingDim`, and `EMBED_DIM`
  must match the `documents.embedding` column.

## Safe, operator-controlled migration

Changing the vector width is destructive to the stored vectors: existing rows no longer
match the column, so the whole corpus must be re-ingested at the new width.

- The **database change is an operator action**: a human runs it against a database they
  chose, with a backup and a maintenance window. It is never part of an automated
  migration loop or a test.
  - Changing the column type invalidates the vector index, which must be rebuilt.
  - Dropping or truncating rows to make the change fit destroys data: do it only
    deliberately, after a backup, and never as a side effect.
  - An `ALTER COLUMN ... TYPE vector(N)` that would silently rewrite or drop vectors must
    fail loudly instead of guessing.
- The **repository change** — a new idempotent migration file, code, or a runbook — goes
  through the governed `/sop-implement` lifecycle, not through this skill and not by
  editing files here. This skill only designs it.
- The automated migration path stays **additive and idempotent**: `CREATE EXTENSION IF
  NOT EXISTS`, `CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`, and
  `ALTER TABLE ... ADD COLUMN IF NOT EXISTS`. Re-running is a no-op, and the path never
  issues an unguarded `DROP`, `TRUNCATE`, or `ALTER COLUMN TYPE`. `rag-template` keeps its
  dimension-change procedure outside `migrations/*.sql` for exactly this reason, so its
  schema target cannot pick it up.

## Isolated PostgreSQL test schemas

Every schema-touching test must run against a throwaway database, never the application
database, so a truncation in one package cannot touch real rows.

- Prefer a per-run database or a per-run schema (`CREATE SCHEMA ...; SET search_path`)
  created from the same migration files as production, so the test proves the real DDL.
- Treat the test database as ephemeral: create, migrate, exercise, and drop it. Never
  point an integration test at the connection the application uses.
- If tests truncate a shared table, run those packages serially, not in parallel.

## Data-integrity checks

Before trusting a migrated store, check more than "the command exited 0":

- every stored vector's width equals the column's declared width (`vector_dims`);
- no embeddings are `NULL`, and none is all-zero (a zero vector scores arbitrarily);
- the vector index is valid and used: `EXPLAIN` shows a vector index scan, not a
  sequential scan, for a nearest-neighbour query;
- provenance is intact: `source`, `section`, and `chunk_index` are populated, and
  `(source, chunk_index)` has no unintended duplicates.

## Example: rag-template

`rag-template` is a small Go + pgvector RAG project used here only as a reference;
nothing in this skill depends on it.

- Schema: `migrations/001_init.sql` creates the `vector(768)` column and the
  `documents_embedding_idx` HNSW index (`vector_cosine_ops`).
- The operator-run dimension change and its integrity checks live in
  `docs/operations/embedding-dimension.md`.
- The connection comes from `DATABASE_URL` and the width from `EMBED_DIM`; tests run
  against a throwaway database. No connection string and no dimension is hardcoded here.

## What this skill must not do

- Do not edit the repository or run DDL to satisfy the request; design the change and
  hand it to `/sop-implement`, and hand the database change to the operator.
- Do not run a destructive migration (`DROP`, `TRUNCATE`, unguarded `ALTER COLUMN TYPE`)
  as an automated step, and never against a database you did not choose.
- Do not approve, decline, accept a changed task, commit, push, or merge on the
  operator's behalf.
- Do not choose a provider, a model class, a model, or a connection string.
- Do not reimplement SOP's routing, approval, or lifecycle policy.

## Failure behavior

Report SOP's result as-is. A schema change is never verified by this skill; a mutation
only becomes trusted through SOP's deterministic validation and, where required, the
human approval boundary.
