# PLAN — RAG-010: Deterministic token/context budget

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-010/`). The original plan is preserved below.**

## Objective

Add a deterministic, provider-neutral **context budget** so the context sent to
the model never exceeds a configured size, allocating the budget across sections
by fused rank with fairness, while the current chunk-count behaviour remains the
default (fallback). `ExpandLimit` counts *chunks* (`retrieval.go:244`), not the
size actually consumed, and expansion can exceed a nominal total when many
sections each receive at least one chunk (`docs/plans/PLAN.md:223`).

## Decision recorded (resolves `PLAN-RAG-Gap-Closure.md` Amendment C / Q-02)

The deferred token-vs-character question is resolved **for this task** as a
**provider-neutral byte-based estimate**: a chunk's estimated size is the length
of its content in bytes plus a fixed per-chunk overhead. This is an
approximation for budgeting, **not** tokenizer-accurate token counting. It keeps
the builder deterministic, model-independent, and free of new dependencies. The
estimator is a **pluggable function**, so a real tokenizer can replace it later
without changing the builder's allocation logic. No default behaviour changes:
the budget is **off** unless `CONTEXT_BUDGET` is set.

## Why this is not a re-implementation

It consumes the existing expanded result (`retrieval.PipelineResult.Expanded`,
surfaced by `HybridRetrieve`/`MultiQueryRetrieve`) and only decides which chunks
to keep. RRF fusion, section dedup, and section expansion are unchanged.

## Tasks

### RAG-010 — Deterministic context budget

- **Objective:** Add a pure, deterministic budget builder that selects a subset
  of ranked documents whose estimated size never exceeds a configured budget,
  giving higher-ranked sections priority and round-robin fairness across
  sections; make it config-driven with the current chunk-count behaviour as the
  default fallback.
- **Files:** new `internal/contextbudget/budget.go` and
  `internal/contextbudget/budget_test.go`; `internal/config/config.go` (add a
  `ContextBudget` field, its `CONTEXT_BUDGET` parse, and a default constant);
  `cmd/rag/main.go` (apply the builder to the expanded context the model answers
  from). No change to `cmd/ingest`, `internal/loader`, `internal/document`,
  `internal/chunking`, `internal/ingestion`, or any migration.
- **Depends on:** none
- **Scope expansion:** **none** — no PRD non-goal decision required.
- **Acceptance:** (deterministic and DB-free)
  - The selected context's estimated size never exceeds the configured budget.
  - With a tight budget, higher-ranked sections are included before
    lower-ranked ones.
  - Fairness: across sections, each gets a chunk before any gets a second; one
    long section cannot starve the others (mirrors the existing per-section split
    at `retrieval.go:253`).
  - Determinism: identical input yields identical output; ordering is stable.
  - A single chunk whose own estimate exceeds the budget is excluded, so the
    "never exceed" guarantee holds even for oversize chunks.
  - `CONTEXT_BUDGET` unset or `0` returns the input unchanged (current
    chunk-count behaviour; no default drift). Empty input yields empty output.
- **Validation gate:** unit tests for budget boundary, priority, fairness,
  determinism, oversize-chunk exclusion, and disabled-passthrough; plus a config
  test that `CONTEXT_BUDGET=0` is accepted and a negative value is rejected.
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`).
- **Execution:** `go build ./... && go vet ./... && go test ./...`
  (optionally `go test -race ./...`).
- **Rollback:** set `CONTEXT_BUDGET=0` (the default); behaviour reverts to the
  chunk-count limit. Revert the package to remove it entirely.

## Deterministic acceptance tests (definition)

1. **Within budget** — for a matrix of ranked inputs and budgets in
   `{1, N-1, N, large}`, the sum of estimated sizes of the selected documents is
   `<= budget`.
2. **Priority** — with a budget fitting only the top section's first chunk, the
   selected set is exactly that chunk.
3. **Fairness** — given K sections of equal rank and a budget of K chunk sizes,
   all K first chunks are selected and no section receives a second chunk before
   every section has one.
4. **Oversize exclusion** — a chunk whose estimate alone exceeds the budget is
   never selected.
5. **Determinism** — running the builder twice on the same input yields
   byte-identical slices (stable order; no map-iteration dependence).
6. **Disabled passthrough** — `budget == 0` returns the input slice unchanged
   (same length and order); empty input returns empty.

### Evidence deferred to the operator (DB-dependent; not runnable here)

The gap-closure validation gate also anticipates an **eval run** comparing
evidence recall, latency, and token usage against the pre-change baseline
(`docs/plans/PLAN.md:235`). That requires PostgreSQL + a running model and is recorded
as follow-up evidence, not a code gate. Baseline defaults are unchanged, so the
default path is byte-identical.

## Notes

- Provider-independent: no vendor, no model, no database connection string.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges, or
  approves.
- **Out of scope.** RAG-007–009 and RAG-011–016; metadata filters; retrieval query
  changes; eval-metric changes.
