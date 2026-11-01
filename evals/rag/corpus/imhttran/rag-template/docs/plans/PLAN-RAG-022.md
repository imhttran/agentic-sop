# PLAN — RAG-022: RAG-021 evaluation-sweep hardening (review findings)

**Type:** Focused implementation plan (evaluation harness only; no production runtime change).
**Status:** **PROPOSED — NOT ACTIVE.** Awaiting activation.
**Derived from:** the SOP review of merge `d29dcc2` (artifact `.agent-sdlc/runs/prompts/prompt-20261009-152613/result.md`).
**Baseline:** `main` at `d8a7d7d` (contains `d29dcc2`).
**Supersedes:** none.

## Project
rag-template

## Summary
Harden `scripts/eval-model-sweep.sh` against SOP review findings 1, 2, 3, 6, 7, and 8:
quote and validate SQL identifiers together with captured-SQL regression coverage (1, 6),
validate the threshold grid and the reuse/skip-ingest combination (2, 3), document every
sweep control and destructive-operation boundary (7), and record reproducibility metadata
(8). Evaluation-only: production embedding/retrieval/ingestion defaults and the answer path
are unchanged, existing recorded evaluation results are preserved, and existing create-mode
and reuse-mode behavior for valid configurations is preserved. RAG-022A owns the sweep
script and its Go test; RAG-022B reuses both (serialized after A); RAG-022C is
documentation-only.

## RAG-022A — Quote and validate SQL identifiers, and record reproducibility metadata

Findings 1 (medium), 6 (low), and 8 (info). Today `fresh_database`, `reset_database`, and
the final drop splice `$name`/`$db_name` directly into `DROP DATABASE` / `CREATE DATABASE`
/ `TRUNCATE` statements, so safety depends entirely on `valid_db_name`. Provide one helper
that validates and quotes an identifier together, use it everywhere an identifier reaches
SQL, add a regression test that captures the emitted statements, and record the query-rewrite
setting in the results header.

### Deliverables
- `scripts/eval-model-sweep.sh` — a single validate-and-quote helper used by
  `fresh_database`, `reset_database`, and the final `DROP DATABASE`; the results header
  records the `SWEEP_QUERY_REWRITE` value.
- `cmd/eval/sweep_script_test.go` — a test that runs the create/reuse paths with a `psql`
  recording shim on `PATH` and asserts the exact database names in the emitted statements.

### Acceptance Criteria
- Every database name that reaches SQL is emitted through the shared validate-and-quote
  helper; no `DROP`/`CREATE`/`TRUNCATE` statement interpolates a raw name.
- `valid_db_name` remains the single validation authority and its behavior is unchanged.
- A regression test captures the emitted SQL and asserts the target database is exactly the
  validated name.
- The results header records `SWEEP_QUERY_REWRITE` alongside the dataset/calibration/manifest
  hashes, grid, repeats, and minimum rejection.
- Create-mode and reuse-mode validate-only plans are unchanged for valid configurations.

### Dependencies
- none

## RAG-022B — Validate the threshold grid and the reuse/skip-ingest combination

Findings 2 (medium) and 3 (medium). Reject an empty, malformed, or non-numeric `SWEEP_GRID`
before any database access, and guard the post-selection `best_floor` so a malformed grid
cannot silently proceed into `run_eval`. Refuse `SWEEP_REUSE_DB=1` together with
`SWEEP_SKIP_INGEST=1` (no separately tested safe behavior is specified).

### Deliverables
- `scripts/eval-model-sweep.sh` — grid validation before database access, a `best_floor`
  guard, and an explicit refusal of reuse + skip-ingest.
- `cmd/eval/sweep_script_test.go` — tests covering each rejection.

### Acceptance Criteria
- An empty, malformed, or non-numeric `SWEEP_GRID` fails fast (exit 2) before any database
  access, with a clear message.
- `best_floor` is asserted non-empty and numeric before the repeats loop, else exit 2.
- `SWEEP_REUSE_DB=1` with `SWEEP_SKIP_INGEST=1` is refused with a clear message.
- Existing valid grids and existing create-mode and reuse-mode behavior are unchanged.

### Dependencies
- RAG-022A

## RAG-022C — Document sweep controls and the destructive-operation boundary

Finding 7 (low), documentation only. Update `docs/operations/embedding-models.md` to
describe every sweep control and the destructive-operation boundary.

### Deliverables
- `docs/operations/embedding-models.md` — the full control list and the reuse-mode
  TRUNCATE-not-DROP boundary.

### Acceptance Criteria
- `docs/operations/embedding-models.md` documents `SWEEP_REUSE_DB`, `SWEEP_DB_NAMES`,
  `SWEEP_ALLOW_DB`, `SWEEP_KEEP_DB`, `SWEEP_KEEP_UP`, `SWEEP_VALIDATE_ONLY`,
  `SWEEP_QUERY_REWRITE`, and `SWEEP_SKIP_INGEST`, and the destructive-operation boundary
  (reuse resets with TRUNCATE, never DROP); its links resolve.
- No code or behavior is changed (documentation only).

### Dependencies
- RAG-022A
