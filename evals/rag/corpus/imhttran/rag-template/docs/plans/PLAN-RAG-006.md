# PLAN — RAG-006: Pluggable document loader interface

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-006/`). The original plan is preserved below.**

## Objective

Introduce a loader/content-extractor seam so a document's format, not a hard-coded
`\n## ` split, decides how it becomes `[]document.Section`. Implement Markdown
(existing behaviour, moved behind the interface) and plain text; leave HTML/PDF/OCR
to later tasks.

## Why this is not a re-implementation

It wraps the existing, tested `document.ParseSections` (`internal/document/document.go:12`),
keeping its output contract and its tests unchanged. No retrieval, chunking,
embedding, or ingestion behaviour changes.

## Tasks

### RAG-006 — Pluggable document loader interface

- **Objective:** Add a loader/content-extractor interface that detects a file's
  format (by extension/magic), returns `[]document.Section`, and is dispatched by
  `cmd/ingest`; implement Markdown (existing behaviour) and plain text; leave
  HTML/PDF/OCR as separate tasks.
- **Files (expected):** a new loader file/package (for example
  `internal/document/loader.go` or `internal/loader/`); `cmd/ingest/main.go`
  (dispatch in `loadChunks`); tests alongside. `internal/document/document.go`
  is reused, not rewritten.
- **Depends on:** none
- **Acceptance:**
  - `.md` and `.txt` ingest; an unknown type returns a clear "no loader" error.
  - Existing Markdown tests pass unchanged.
  - No change to shipped defaults; no destructive migration.
- **Validation gate:** unit test per loader;
  `go build ./... && go vet ./... && go test ./...` green.
- **Execution:** `go build ./... && go vet ./... && go test ./...`
  (optionally `go test -race ./...`).
- **Scope expansion:** **none** — Markdown + plain text only; PDF/OCR remain
  RAG-007/008. Requires no PRD non-goal decision.
- **Rollback:** revert; ingestion reverts to markdown-only.

## Notes

- Provider-independent: touches no vendor, no model, no connection string.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges, or
  approves.
- **Out of scope.** RAG-007–016, PRD non-goals, and any default change.
