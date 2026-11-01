# PLAN — RAG-018: Vietnamese/English retrieval evaluation and embedding decision

**Type:** Implementation plan (normative for the work it describes).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-018/`). The original plan is preserved below.**

## Objective

Make it possible to score a **non-default** evaluation dataset without changing any
production default, so the current embedding baseline (`nomic-embed-text`) and a
multilingual candidate (`embeddinggemma`) can be compared on the same bilingual
Vietnamese/English corpus. The dataset and Vietnamese fixtures already exist
(`evals/retrieval-vi-en.json`, `examples/vi/*.md`); this task adds the harness
selection mechanism, its test, and the comparison documentation.

## Why this is not a re-implementation

It adds one optional input to `cmd/eval` (`loadCases` reads `EVAL_DATASET`, default
`evals/retrieval.json`) and reuses the existing metrics (recall@k, precision@k,
evidence recall, citation validity, latency). No retrieval, embedding, generation, or
scoring logic changes; the default path is byte-identical.

## Tasks

### RAG-018 — Dataset selection and multilingual evaluation harness

- **Objective:** Let `cmd/eval` load its cases from a path selected by the
  `EVAL_DATASET` environment variable (default `evals/retrieval.json`), so a bilingual
  dataset can be scored without editing code or changing defaults; add a unit test for
  the selection and a short operator procedure documenting how to run the per-model
  comparison on isolated databases.
- **Files (expected):** `cmd/eval/main.go` (`loadCases` honours `EVAL_DATASET`;
  a missing/unreadable/invalid dataset returns an error naming the path);
  `cmd/eval/main_test.go` (dataset selection + error path); `README.md` and
  `.env.example` (document `EVAL_DATASET`); `docs/operations/embedding-models.md`
  (new: the per-model comparison procedure and the nomic baseline). Do **not** change
  `internal/retrieval`, `internal/embedding`, `internal/ingestion`, or any default.
- **Depends on:** none
- **Acceptance:**
  - `EVAL_DATASET` unset or empty loads `evals/retrieval.json` exactly as today.
  - `EVAL_DATASET=<path>` loads that file; a missing/unreadable file, or malformed
    JSON, returns an error naming the path.
  - The bilingual dataset `evals/retrieval-vi-en.json` loads and scores (its cases
    include English→English, Vietnamese→Vietnamese, English→Vietnamese and
    Vietnamese→English retrieval, plus one unanswerable case).
  - No production default changes; the embedding model/dimension are untouched.
- **Validation gate:** unit test for dataset selection and the error path;
  `go build ./... && go vet ./... && go test ./...` green (optionally
  `go test -race ./...`); a documented `cmd/eval` run against the bilingual dataset.
- **Execution:** `go build ./... && go vet ./... && go test ./...`.
- **Rollback:** revert the `EVAL_DATASET` read; the default path and behaviour return
  unchanged.

## Notes

- Provider-independent: no model, vendor, or connection string is hardcoded; the
  dataset path is data.
- The comparison uses one **isolated database per embedding model** (embeddings from
  different models are never mixed in one vector index); see
  `docs/operations/embedding-models.md`.
- **Human boundary.** SOP stops before commit; no task commits, pushes, merges, or
  approves.
- **Out of scope.** Changing the production embedding model or dimension (RAG-018
  only *evaluates* a candidate); OCR (RAG-008); the eval capstone (RAG-016); genealogy.
