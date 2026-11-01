# PLAN — RAG-015: Security hardening

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-015/`). The original plan is preserved below.**

## Objective

Harden the CLI against abuse and accidental leakage: bound question/document
input sizes with clear errors; delimit and label retrieved content in prompts so
injected instructions are less likely to be followed (documenting the residual
risk); confirm secrets are environment-only with a `.env` ignore check; and add a
dependency vulnerability scan (`govulncheck`) to the local hooks. Service auth and
TLS are explicitly out of scope while this remains a CLI.

## Why this is not a re-implementation

It hardens around the existing prompt (`internal/rag/answer.go`) and the existing
parameterized queries (`internal/retrieval/retrieval.go`); the tool stays read-only
with respect to the repository.

## Tasks

### RAG-015 — Security hardening

- **Objective:** Add input size limits that fail fast with actionable errors;
  separate instructions from untrusted retrieved text in the answer prompt;
  confirm secrets are env-only and `.env` is ignored; add `govulncheck` to the
  local hooks where available.
- **Files (expected):** `internal/rag/answer.go` (prompt delimiting/labelling),
  `internal/config/config.go` (input-size limits, default-safe), `cmd/rag/main.go`
  and `cmd/eval/main.go` (enforce limits with clear errors), `.gitignore` (`.env`
  ignore), local hook script (`govulncheck`); tests alongside. No change to
  `internal/contextbudget`, `internal/retrieval` query structure, `internal/reranking`,
  migrations, or `internal/ingestion`.
- **Depends on:** none
- **Acceptance:**
  - Oversized inputs fail fast with actionable errors naming the limit.
  - The prompt template clearly separates instructions from untrusted retrieved
    text; the residual prompt-injection risk is documented.
  - Secrets are environment-only and `.env` is ignored.
  - `govulncheck ./...` reports no known vulnerabilities in the dependency set.
  - Defaults and existing behavior are unchanged when the new limits are unset.
- **Validation gate:** unit tests for the limits; a documented manual
  injection case; a recorded `govulncheck` run.
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`). If `govulncheck` is unavailable, record it as
  **NOT RUN** — never passed.
- **Execution:** `go build ./... && go vet ./... && go test ./...` for the code
  gate; `govulncheck ./...` for the scan.
- **Rollback:** each sub-change is independent and revertible.
- **Scope expansion:** **none** for the listed items; service auth and TLS remain
  future work, out of scope while this is a CLI.

## Notes

- Provider-independent: hardening is local to prompts, limits, and dependencies.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007/008, RAG-016, service auth/TLS, and any default change.
