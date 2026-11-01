# Changing the stored embedding dimension (EMBED_DIM)

This is an **operator-run procedure**. It is deliberately *not* a migration:
it lives under `docs/operations/` and is therefore **not** matched by
`migrations/*.sql`, so it is never applied automatically by `make db-schema`,
`make db-up`, or the `docker-entrypoint-initdb.d` mount in `docker-compose.yml`.
Nothing in this document is executed by anything except a human operator.

Changing the embedding dimension of `documents.embedding` is destructive to the
vectors already stored: once the column width changes, the existing vectors no
longer match the column's dimension and must be re-ingested. Plan for a
re-ingest of every corpus file.

## When to run this

The guard `ingestion.CheckEmbeddingDim` fails fast when the configured
`EMBED_DIM` does not match the stored `documents.embedding` column:

```text
EMBED_DIM (1024) does not match the documents.embedding column (vector(768)):
follow docs/operations/embedding-dimension.md to set the column to the dimension
you want, re-ingest the corpus, then set EMBED_DIM=1024 to match the column
```

That message names both dimensions and points back here. Use this procedure
when you want to change the dimension (for example, from the default 768 that
`nomic-embed-text` produces to a 1024-wide model) rather than when you want to
set `EMBED_DIM` back to the column's current width.

## Preconditions

1. Choose the target dimension `N` produced by the embedding model you intend
to use.
2. Make sure it is the dimension the model actually returns; see the model's
documentation, or embed one string and read the vector length.
3. Stop `cmd/ingest` and `cmd/rag` while the change is in progress: they read
   `documents.embedding` and will fail against a column of the wrong width.
4. Have a `psql` connection to the database, e.g.
   `postgres://rag:rag@127.0.0.1:5434/rag?sslmode=disable`.

## Step 1 — Change the column dimension

The table and index are created by `migrations/001_init.sql` as
`embedding vector(768) NOT NULL` with `documents_embedding_idx`, an HNSW index
over `vector_cosine_ops`. Change the column with pgvector's `vector(N)` type:

```sql
-- N is the target dimension, e.g. 1024.
ALTER TABLE documents
    ALTER COLUMN embedding TYPE vector(N);
```

The HNSW index is invalidated by the column type change. Rebuild it (drop and
recreate, matching `documents_embedding_idx` in `migrations/001_init.sql`):

```sql
DROP INDEX IF EXISTS documents_embedding_idx;

CREATE INDEX IF NOT EXISTS documents_embedding_idx
    ON documents USING hnsw (embedding vector_cosine_ops);
```

If the table still holds vectors, the `ALTER COLUMN` will fail rather than
silently truncate them. Delete the stale rows first when you intentionally do
not need them:

```sql
-- Only if you accept losing the stored vectors; the re-ingest below refills them.
TRUNCATE documents;
```

Then re-run the `ALTER TABLE` and `CREATE INDEX` statements above.

## Step 2 — Set EMBED_DIM

The commands read `EMBED_DIM` from the environment (see the README
Configuration table). Set it to the new dimension so the guard, the embedder,
and the ingest checks all agree with the column:

```bash
export EMBED_DIM=N     # e.g. 1024
```

and update `.env` if you keep one:

```text
EMBED_DIM=N
```

## Step 3 — Re-ingest the corpus

Every stored vector must be produced at the new dimension. Re-run ingest for
every corpus file; `cmd/ingest` replaces a source's chunks (delete-then-insert),
so re-ingesting does not duplicate rows:

```bash
make ingest                                  # examples/loan-policy.md
make ingest FILE=examples/large-loan-policy.md
make ingest FILE=examples/member-services-guide.md
make ingest FILE=examples/commercial-servicing-manual.md
```

or, for a single file:

```bash
go run ./cmd/ingest examples/loan-policy.md
```

Use the same embedding model you sized the column for.

## Step 4 — Verify

The guard is the check the commands run; run it directly if you want to confirm
before ingesting:

```bash
RAG_INTEGRATION=1 go test ./internal/ingestion/ -run IntegrationEmbeddingDim -v
```

A configured/stored mismatch returns the actionable error shown above; a match
returns `ok`. Equivalently, query the catalog:

```sql
SELECT a.atttypmod AS dimension
FROM pg_attribute AS a
JOIN pg_class AS c ON c.oid = a.attrelid
JOIN pg_namespace AS n ON n.oid = c.relnamespace
WHERE c.relname = 'documents'
  AND n.nspname = ANY (current_schemas(false))
  AND a.attname = 'embedding'
  AND NOT a.attisdropped;
```

The value must equal the `EMBED_DIM` you set in step 2. Then ingest one file and
ask a question (`make ask Q="…"`) to confirm retrieval works at the new width.

## Why this cannot run automatically

`make db-schema` applies only `migrations/*.sql`:

```make
db-schema:
	@for f in migrations/*.sql; do \
		psql "$$DATABASE_URL" -v ON_ERROR_STOP=1 -f "$$f" || exit 1; \
	done
```

`docker-compose.yml` mounts the same directory into
`/docker-entrypoint-initdb.d`, which PostgreSQL executes only when it creates a
fresh volume. This document is neither under `migrations/` nor a `*.sql` file,
so no loop and no entrypoint can pick it up. Every statement above is run by a
human against a database they chose.

That design also keeps `make db-schema` idempotent and non-destructive: the
files it does apply (`migrations/001_init.sql`, `migrations/002_ingestion_metadata.sql`)
use only `CREATE EXTENSION IF NOT EXISTS`, `CREATE TABLE IF NOT EXISTS`,
`CREATE INDEX IF NOT EXISTS`, and `ALTER TABLE … ADD COLUMN IF NOT EXISTS`,
so re-running the target against an already-migrated volume is a no-op and
executes no `DROP`, `TRUNCATE`, or unguarded `ALTER COLUMN TYPE`.
