# PLAN — RAG-012: Reranking hardening and evaluation

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-012/`). The original plan is preserved below.**

## Objective

Harden LLM reranking so a malformed or invalid reranker response degrades
deterministically to the fused order instead of aborting the request, add a
bounded latency guard, and record sweep rows comparing reranking strategies.
The default stays off (`RAG_LLM_RERANK=false`); the change makes the optional
path safe, not default-on.

## Why this is not a re-implementation

It extends `internal/reranking` (`RerankLLM`, `parseRanking`) and keeps the
existing lexical reranker and RRF baselines. The prompt and the
`[ID: n]` candidate contract are unchanged.

## Tasks

### RAG-012 — Reranking hardening and evaluation

- **Objective:** On reranker parse/validation failure, fall back to the fused
  order and log the fallback instead of returning an error; add a bounded
  latency guard so a slow reranker cannot stall the request; optionally
  evaluate a cross-encoder or stronger LLM reranker behind the provider
  interface. Do not change the default from off.
- **Files (expected):** `internal/reranking/llm.go` (fallback path and guard),
  `internal/reranking/llm_test.go` (fallback and guard tests),
  `internal/config/config.go` (guard/limit setting, default-safe), and
  `docs/reference/experiments.md` (new sweep rows); `cmd/eval/main.go` only if the sweep
  harness needs wiring. No change to `internal/contextbudget`,
  `internal/retrieval` query structure, migrations, or `internal/ingestion`.
- **Depends on:** none
- **Acceptance:**
  - A malformed reranker response degrades to the fused order; the request still
    answers (no error surfaced to the user).
  - The latency guard bounds reranker wait time and falls back to the fused
    order when exceeded.
  - New sweep rows compare off / lexical / LLM (and cross-encoder, if added) on
    recall, precision, evidence recall, and latency.
  - The default remains off; with reranking off, behaviour is byte-identical to
    baseline.
- **Validation gate:** unit tests for the fallback and the guard;
  `make sweep` rows recorded in `docs/reference/experiments.md`;
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`). The sweep requires PostgreSQL plus a running model; if
  unavailable, record the sweep as **NOT RUN** — never as passed.
- **Execution:** `go build ./... && go vet ./... && go test ./...` for the code
  gate; `make sweep` (with a live DB and model) for the evaluation rows.
- **Rollback:** the default stays off; revert the fallback/guard change
  independently without touching baselines.
- **Scope expansion:** **none** — hardening the existing optional path; adding a
  cross-encoder is optional and behind the existing provider interface, needing
  no PRD non-goal decision.

## Notes

- Provider-independent: the reranker is used through `generation.Generator`; no
  vendor, model, or connection string is hardcoded.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges,
  or approves.
- **Out of scope.** RAG-007–009, RAG-011, RAG-013–016, retrieval ranking
  changes, and any default change.
