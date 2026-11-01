# PLAN — RAG-007: PDF text-layer loader

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-007/`). The original plan is preserved below.**

## Objective

Register a PDF loader so a PDF with a text layer becomes `[]document.Section`, one
section per page, carrying the 1-based page number defined by the RAG-017 contract.
A PDF with **no** text layer must fail explicitly ("no text layer; OCR required"),
never ingest silently empty.

## Why this is not a re-implementation

It implements the existing `loader.Loader` interface and registers with the existing
`loader.Registry`; `cmd/ingest` dispatch, chunking, ingestion, retrieval, and
citations are unchanged. Markdown/plain-text loaders and their tests are untouched.

## Tasks

### RAG-007 — PDF text-layer loader

- **Objective:** Add a `PDFLoader` that supports `.pdf`, extracts each page's text
  layer, and returns one `document.Section` per page with `Page` set (RAG-017);
  return an explicit error when a PDF has no extractable text layer; register it in
  `loader.DefaultRegistry()`.
- **Files (expected):** new `internal/loader/file_pdf.go` and
  `internal/loader/file_pdf_test.go`; `internal/loader/loader.go` (register the
  loader in `DefaultRegistry`); `go.mod`/`go.sum` (dependency);
  `internal/loader/testdata/` fixtures. No change to `cmd/ingest`,
  `internal/chunking`, `internal/ingestion`, `internal/retrieval`, `internal/rag`,
  or `internal/citations`.
- **PDF dependency:** `github.com/ledongthuc/pdf` (MIT, pure Go, small surface,
  direct per-page `GetPlainText`, maintained — chosen in Phase 3 of this batch). Pin
  the resolved pseudo-version; run `go mod tidy`; keep `govulncheck ./...` clean.
- **Fixtures (deterministic, committed):**
  - `internal/loader/testdata/text-layer.pdf` — a small, valid, multi-page PDF with
    a real text layer (one known sentence per page).
  - `internal/loader/testdata/no-text-layer.pdf` — a valid PDF page with an image/
    no text operators (a "scanned" stand-in).
  - a malformed/truncated `.pdf` byte blob for the error path.
- **Depends on:** none
- **External prerequisite:** RAG-017 (page provenance contract) — `LOCAL_DONE` before
  this task runs; the page fields and migration 004 are already in place.
- **Acceptance:**
  - `Supports` matches `.pdf` only; Markdown/plain-text dispatch is unchanged.
  - A text-layer PDF yields one `Section` per page, in page order, each with the
    correct `Page` (`1..N`) and the page's text; source identity is derived by the
    caller from the path, unchanged.
  - A PDF with no text layer returns an explicit error naming the file and stating
    that OCR is required — never an empty successful ingest.
  - A malformed/truncated PDF returns an error, never panics.
  - Existing loader tests and Markdown/text behaviour are unchanged.
- **Validation gate:** unit tests per fixture (text layer, no text layer, malformed);
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`); `govulncheck ./...` clean after the dependency.
- **Execution:** `go build ./... && go vet ./... && go test ./...`.
- **Rollback:** remove the loader file, its registry line, and the dependency; the
  registry falls back to the Markdown/text loaders and a `.pdf` input returns the
  existing `no loader for <path>` error.
- **Scope expansion:** **authorized** (operator, this batch).

## Notes

- Provider-independent: no model, vendor, or connection string; a pure-Go extractor.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges, or
  approves.
- **Out of scope.** OCR (RAG-008), page-citation UI beyond RAG-017's contract,
  genealogy, and any default change.
