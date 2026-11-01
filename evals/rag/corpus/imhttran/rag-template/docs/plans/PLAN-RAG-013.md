# PLAN — RAG-013: Structured citations with runtime validation and repair

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-013/`). The original plan is preserved below.**

## Objective

Make `cmd/rag` answers cite trustworthy sources. Parse the model's answer into
structured citations bound to retrieved documents, validate each against the
expanded set, and on invalid or missing citations either repair (strip invented
ones) or re-prompt once. Keep the human-readable `[source - section]` rendering.

## Why this is not a re-implementation

It reuses the citation parser already proven in `cmd/eval/main.go` (the
`citationValidity`/`extractCitedClaims` logic) and does not change the prompt's
citation-format contract (`internal/rag/answer.go`).

## Tasks

### RAG-013 — Structured citations with runtime validation and repair

- **Objective:** Parse an answer's `[source - section]` citations, validate each
  against the retrieved documents, and on invalid/missing citations repair
  (strip invented ones) or re-prompt once, bounded; every rendered citation must
  resolve to a retrieved source/section.
- **Files (expected):** `internal/rag/answer.go` (validation/repair around the
  answer call), possibly a new `internal/citations/` package extracted from the
  eval parser, `cmd/rag/main.go` (report invalid/repair outcome), and
  `cmd/eval/main.go` only to share the parser; tests alongside. No change to
  `internal/contextbudget`, `internal/reranking`, `internal/observability`,
  `internal/retrieval` query structure, or migrations.
- **Depends on:** none
- **Acceptance:**
  - Every citation in a `cmd/rag` answer resolves to a retrieved source/section.
  - Invalid citations are never shown as if valid; one repair attempt is bounded.
  - Eval citation validity stays at or above baseline (89/89).
  - With validation disabled, behavior is byte-identical to baseline.
- **Validation gate:** unit tests for parse/validate/repair (including an
  invented citation and a missing citation); an eval run confirming validity and
  entailment do not regress. The eval run needs a database and a running model;
  if unavailable it is **NOT RUN** — never passed.
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`).
- **Execution:** `go build ./... && go vet ./... && go test ./...` for the code
  gate; `make eval` for the validity/entailment evidence.
- **Rollback:** return to raw model output; the parser is additive and behind a
  setting.
- **Scope expansion:** **none** — reuses existing citation machinery.

## Notes

- Provider-independent: validation is lexical over the answer and retrieved
  documents; it names no model.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007/008, RAG-015/016, changing the citation-format
  contract, and any default change.
