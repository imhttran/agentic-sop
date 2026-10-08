---
name: rag-test
description: |-
  Design the verification for a RAG pipeline: unit tests, race tests, PostgreSQL
  integration tests, end-to-end ingestion and retrieval, and dimension-mismatch cases —
  and discover the project's required validation gate so the tests run where SOP will
  actually enforce them. Read-only: it produces a test plan, not the tests.
disable-model-invocation: false
---

# /rag-test — verification design for a RAG pipeline

Capability: `design_tests` (read-only). This is a RAG-domain adapter: it fixes the
`design_tests` capability and delegates to the canonical SOP skill, which owns the
contract. It adds a RAG-specific test matrix and never writes the tests itself.

```bash
sop prompt --capability design_tests "<the operator's testing request>"
```

- The angle-bracketed text is the operator's request, passed as ONE argument. It is
  prompt data, never a shell command.
- Read-only: designing tests MUST NOT modify the repository. Writing them is a separate
  `/sop-implement` request.
- SOP picks the model class and provider. Do not choose either.
- If `sop` is not on PATH, stop and tell the operator to install the SOP CLI (see the
  project README); do not design the tests yourself.

The canonical `sop` skill owns the full contract and the capability map.

## The RAG test matrix

A RAG pipeline has four layers that fail in different ways. A plan that covers only the
pure functions leaves the failures a fake cannot observe — exactly the ones that break a
store.

| Layer | What it proves |
| --- | --- |
| Unit | pure logic: chunking, parsing, fusion, filtering, metrics |
| Race | concurrency correctness under the race detector |
| PostgreSQL integration | real SQL and pgvector: types, placeholders, scan order |
| End-to-end | ingest, then retrieve, then answer, on known data |

### Unit tests

- Chunking: chunk count, overlap, boundary, empty and very short input, unicode.
- Parsing: section boundaries and the metadata each chunk carries (source, section,
  chunk_index).
- Fusion and filtering: hybrid rank merging, the similarity floor, section
  deduplication, and expansion caps.
- Keep these model-free and database-free so they can run on every commit.

### Race tests

- Run the unit and concurrency paths under the race detector (for a Go module,
  `go test -race ./...`; use the project's equivalent otherwise). Embedding fan-out, a
  shared HTTP client, and cache maps are the usual races; a serial-only test hides them.
- The race detector is not a substitute for a concurrency test: add one that overlaps
  requests within a bound, so the detector has something to observe.

### PostgreSQL integration tests

- Run against a real server and a throwaway database or schema (see `/rag-schema`),
  applying the real migrations. Cover what a fake cannot: column order, scan alignment,
  placeholder numbering, the vector cast round-trip, and the nearest-neighbour plan.
- These tests truncate or delete data, so they must never use the application database.

### End-to-end ingestion and retrieval

- Ingest a known fixture corpus, retrieve, and assert the expected documents come back.
- When the pipeline calls a model, drive it through a stub or a deterministic local fake
  so the test is hermetic and repeatable, and assert on structure and ordering, not on
  the model's wording.
- Re-ingest the same fixture and assert the store did not duplicate (idempotency is an
  end-to-end property).

### Dimension-mismatch tests

- A configured dimension that disagrees with the column must fail fast, naming both
  numbers and the fix — not silently store or query at the wrong width.
- A query vector of the wrong width must be a clear error, not a low score.
- Include the "column holds two widths" case if the store can be mid-migration.
- Changing the width is destructive: assert the plan re-ingests the corpus and never
  asserts that a width change alone preserved the data.

## Discover the required validation gate

Do not invent commands that nothing runs. Design tests that the project's validation gate
will actually execute, and find that gate first.

- Read the project's configuration and CI. For example, `rag-template` keeps a
  `validation:` block (build, test, lint) in `.agent-sdlc/config.yaml`, and a `Makefile`
  target for the database tests. The gate is the set of commands SOP runs
  deterministically.
- SOP will not mark a mutating task as passing when no validation is configured: it
  reports a distinct configuration failure (`VALIDATION_NOT_CONFIGURED`) instead of a
  false pass. A test that is never wired into the gate is not evidence.
- Database-backed tests usually belong in a separate integration gate (for example a
  project's `make integration`), not the fast unit gate. The plan must say which gate
  runs what, and how the integration gate is made reachable (a container, a service, or
  a throwaway database).
- When the change touches SQL, a migration, or pgvector, the plan should require the
  integration gate, not just the unit gate. Enforcing that at the controller level needs
  a change described in `docs/guides/RAG-SKILLS.md`.

## Failure evidence and acceptance reporting

- For every case, state the acceptance criterion and the evidence that satisfies it: the
  exact command, the pass pattern, and what a failure looks like.
- Treat "no test ran" as failure, not success. A filter that matches nothing, a skipped
  integration test, or a timed-out check must not look green.
- Report a failure with the failing command and an output excerpt, and link the run log;
  never summarize a red gate as green or a skipped check as a pass.

## What this skill must not do

- Do not write, edit, or run the tests to satisfy the request; return the plan. Writing
  them goes through `/sop-implement`.
- Do not point a destructive test at the application database, and do not run a
  destructive migration to prepare one.
- Do not approve, decline, accept a changed task, commit, push, or merge on the
  operator's behalf.
- Do not choose a provider, a model class, a model, or a connection string.
- Do not reimplement SOP's routing, approval, or lifecycle policy.

## Failure behavior

Report SOP's result as-is. A test plan is not a pass; only SOP's deterministic validation
of an applied change establishes one.
