# PLAN — RAG-014: Structured observability and accounting

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-014/`). The original plan is preserved below.**

## Objective

Give every `cmd/rag` invocation a correlated, machine-readable run record: a
per-run ID, per-stage timings (embed, vector, keyword, fuse, expand, judge,
generate), retrieved/filtered counts, and token/context usage. Use the standard
library `log/slog` (no external APM dependency) with an opt-in JSON mode, while
keeping the current human-readable stage prints as the default for the learning
workflow.

## Why this is not a re-implementation

The existing stage printers become one handler/mode behind a reporter; no
behaviour is removed and the default human output is unchanged. Token/context
numbers come from the RAG-010 budget (`contextbudget.EstimateAll`), not a new
estimator.

## Tasks

### RAG-014 — Structured observability and accounting

- **Objective:** Add a per-invocation run ID and a small reporter that emits
  stage timings, retrieved/filtered counts, and token/context usage; provide a
  JSON (`log/slog`) mode selected by config, with the current human-readable
  output as the default. No external APM dependency.
- **Files (expected):** a new reporter package (for example
  `internal/obs/`), `cmd/rag/main.go` (emit stage records alongside the existing
  prints), `internal/config/config.go` (log-format setting, default-safe),
  `.env.example` (document the setting); tests alongside. No change to
  `internal/contextbudget`, `internal/retrieval` query structure, migrations, or
  `internal/ingestion`.
- **Depends on:** none
- **Acceptance:**
  - Every run emits a correlated record (shared run ID) with stage timings and
    retrieved/filtered counts.
  - Token/context usage is reported per run, using the RAG-010 byte estimate.
  - The default human-readable output is unchanged for the learning workflow;
    JSON output is opt-in and introduces no default drift.
- **Validation gate:** unit test on the reporter (schema and run-ID correlation);
  a scripted `cmd/rag` run asserting the JSON schema. `go build ./... &&
  go vet ./... && go test ./...` green (optionally `go test -race ./...`). The
  scripted run needs a live database and model; if unavailable, record it as
  **NOT RUN** — never as passed.
- **Execution:** `go build ./... && go vet ./... && go test ./...` for the code
  gate; a `cmd/rag` run with the JSON mode for the schema assertion.
- **Rollback:** revert; the existing prints are restored and the default path is
  byte-identical.
- **Scope expansion:** **none** — stdlib `log/slog` only; no PRD non-goal
  decision required.

## Notes

- Provider-independent: the reporter observes the pipeline; it names no vendor,
  model, or connection string.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007–009, RAG-011, RAG-013, RAG-015, RAG-016, external
  APM/metrics backends, and any default change.
