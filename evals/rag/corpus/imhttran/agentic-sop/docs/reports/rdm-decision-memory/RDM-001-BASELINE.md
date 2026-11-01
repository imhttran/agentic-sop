# RDM-001 baseline inventory — fresh verification

**Type:** Operator-side verification record (non-normative).
**Recorded:** 2026-10-19, during recovery of `feat/rag-decision-memory`.
**Bound to:** `7540edc9bc2b3bb5a9c6cfe4f3799021b807e085` +
`docs/plans/SOP-RAG-Decision-Memory-Plan.md`
`70e4c4f62ba0c7d2810eb446751d671bd846065bd67504c71d7ed72292559e76`.

## Why this file exists

The original `RDM-001` attempt recorded `PARTIAL` / `LOCAL_DONE` and **did not
persist a durable baseline artifact**. Its run directory contains
`report.json`, `report.md`, `trace.json`, and `implementation.md`, but
`implementation.md` records only a tool call, not the inventory. That historical
verdict is preserved unchanged and is not waived or rewritten here.

This inventory is therefore a **fresh verification performed by the operator's
agent**, clearly distinguished from the model's original claim. It is
reproducible from the committed tree and the commands listed below.

## Base and dependency identity

| Item | Value |
| --- | --- |
| Base commit | `7540edc9bc2b3bb5a9c6cfe4f3799021b807e085` (`main`) |
| Working branch | `feat/rag-decision-memory` |
| Go version | `go 1.27.1` (`go.mod`) |
| SQLite driver | `modernc.org/sqlite v1.59.0` (pure Go, no CGO) |
| PostgreSQL driver | none — no `pgx` or `lib/pq` in `go.mod` |
| SOP binary revision (recorded by the run) | `4a575deec004deb8dde627fa0c0aff7a63df5289`, `modified: true` |

## Existing RAG components

| Capability | Implementation | Evidence |
| --- | --- | --- |
| External retrieval client | `internal/ragclient` — runs a configured executable via `exec.CommandContext` | `exec.go`, `ragclient.go`, tests |
| Retrieval gate | `internal/retrievalgate` — deterministic gate and live test | `gate.go`, `gate_test.go` |
| Vector evaluation | `internal/vectoreval` | package + tests |
| Deterministic BM25 retrieval | `internal/retrieval` | `retrieval.go`, `retrieval_test.go` |
| Decision memory | `internal/decisionmemory` | package + tests |
| Prompt inclusion telemetry | `internal/cli/rag_verifier.go` writes `runs/<task>/rag.json` | `writeRAGTelemetry`, `updateRAGPromptInclusion` |

## Configuration contract

- `SOP_RAG_ENABLED` is the environment opt-in (`internal/config/rag.go`).
- The `rag:` block in `.agent-sdlc/config.yaml` is the project layer
  (`internal/config/config.go`).
- Precedence is environment → project configuration → built-in defaults.
- RAG is **disabled by default**; with an omitted block and no `SOP_RAG_*`
  variables the layer is inert and existing behavior is unchanged.
- `rag.json` is a per-run telemetry artifact, not configuration.

## SQLite / FTS5 preflight

- `modernc.org/sqlite v1.59.0` is already a direct dependency and builds without
  CGO on the supported platforms.
- FTS5 is available through that driver: `internal/memindex` creates a normal
  table plus an FTS5 virtual table and all index/search tests pass.
- No external SQLite CLI is required by the local path.

## PostgreSQL coupling

- No PostgreSQL client library is compiled into the binary.
- The existing PostgreSQL-backed capability is reached through the configured
  external retrieval executable, not an in-process connection.
- The recovery removed `internal/retrievalboundary/pgadapter.go`, which was
  unreferenced, imported only `context`, and existed solely to wrap a search
  function. No existing PostgreSQL behavior changed.
- Backend selection stays explicit: local memory is the default local path, and
  an explicitly configured PostgreSQL user is never silently switched to SQLite.

## Reproduction

```bash
git rev-parse HEAD
grep -n "SOP_RAG_ENABLED" internal/config/rag.go
grep -n "sqlite" go.mod
go test ./internal/memindex -run TestLocalIndexHasNoPostgresDependency -count=1
go test ./internal/memindex -run TestLocalIngestAndRetrieveWithoutPostgres -count=1
```

## Limits

This inventory records what the current tree supports. It does not assert that
the original `RDM-001` model claim was accurate; the durable artifact needed to
check that claim does not exist, and its `PARTIAL` verdict stands as recorded.
