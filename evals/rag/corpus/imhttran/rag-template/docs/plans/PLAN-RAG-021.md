# PLAN — RAG-021: Expanded bilingual retrieval evaluation and embedding-model selection

**Type:** Proposed evaluation plan (planning only; makes no runtime change).
**Status:** **Historicalized — archived as `COMPLETE` (see `.agent-sdlc/archive/plan-rag-021/`). The original plan is preserved below.**

## 1. Why this is needed

RAG-018 established the method and produced a first comparison, but its dataset has
only **5 answerable cases** and the run exposed a confound: `MIN_SIMILARITY` is
**model-specific**, so models compared at one fixed threshold are measured unfairly
(`embeddinggemma` scored R@4=0.40 at 0.6 but 0.80 at 0.3, matching `nomic-embed-text`).
A defensible embedding-model decision needs a larger, calibrated, repeated study.

## 2. Model-selection criteria (defined before any comparison)

A candidate model may be proposed for promotion only if **all** hold:

1. **Recall parity or better** on the calibrated floor: `Recall@1` and `Recall@4`
   within noise of the incumbent across all four directions **and** the aggregate.
2. **Rejection parity or better**: the unanswerable/misleading-context false-positive
   rate is no worse than the incumbent at each model's calibrated floor.
3. **Dimension compatibility**: the candidate's dimension matches the schema
   (768) or a documented, operator-run migration exists (`embedding-dimension.md`).
4. **License acceptable** for the project's distribution intent (prefer OSI; a
   non-OSI model such as `embeddinggemma`'s Gemma Terms requires an explicit decision).
5. **Resource budget**: embedding latency and peak memory within an agreed budget.
6. **Reproducibility**: results stable across ≥3 repeat runs within the reported
   variance (see §5).
7. **No default drift without evidence**: promotion is recorded in
   `docs/reference/experiments.md` and is a separate, explicit operator step.

Failing any criterion → keep the incumbent (`nomic-embed-text`).

## 3. Dataset requirements

- **≥30 independently labeled answerable cases**, distributed across:
  - **EN→EN** (English query, English document) — baseline sanity.
  - **VI→VI** (Vietnamese query, Vietnamese document).
  - **VI→EN** (Vietnamese query, English document) — cross-language.
  - **EN→VI** (English query, Vietnamese document) — cross-language.
- Each case has `question`, `expected[]` (`source`, `section`, `evidence`) with
  **deterministic**, human-verified labels, plus `expected_facts` where useful.
- **≥6 unanswerable cases** and **≥3 misleading-context cases** (a distractor
  section whose text is topically close but does not answer the question), so the
  rejection/false-positive rate is measurable and not a single point.
- **Vietnamese text is preserved verbatim** and source/page provenance is kept
  (Markdown `source`+`section`; PDF `page` via RAG-017 where a paged fixture is used).
- The dataset is a committed file (`evals/retrieval-vi-en-expanded.json` or a
  successor), loaded with `EVAL_DATASET`.

## 4. Corpus and manifests

- The corpus is the committed files the dataset references; no external download.
- Add a **manifest** (`evals/corpus-vi-en.json` or a documented list) mapping each
  `source` to its file path and a content hash, so a run is reproducible: the ingest
  is described by the manifest, and drift is detected by the hash.
- Each model is ingested into its **own isolated database** (never one vector index
  with mixed models), per `docs/operations/embedding-models.md`.

## 5. Procedure

1. **Calibrate the floor per model.** For each model, sweep `MIN_SIMILARITY` over a
   small grid (e.g. 0.2–0.8) and pick the operating point (record the rule, e.g.
   "floor that maximises rejection without losing recall parity"). Report metrics at
   that operating point, not at a shared arbitrary floor.
2. **Ingest** the manifest corpus into that model's database with that model and its
   `EMBED_DIM`; **reject** any run whose dimension does not match the schema.
3. **Score** with `EVAL_DATASET=<dataset> cmd/eval`; capture `Vector retrieval`,
   `Hybrid retrieval`, `Avg Evidence Recall`, and the rejection/false-positive line.
4. **Repeat ≥3 times** (fresh ingest per repeat to include embedding nondeterminism)
   and report mean ± spread. Never credit a single-run delta.
5. **Measure resources** per model: embedding latency (time a fixed batch of
   representative chunks, e.g. via the ingest wall time or a small harness), and peak
   RSS during ingest. Record both.
6. **Record** each run (model, version, dimension, floor, corpus manifest hash,
   repeat index) in `docs/reference/experiments.md` or a dedicated results file.

## 6. Metrics

| Metric | Definition |
|--------|------------|
| Recall@1, Recall@4 | fraction of cases whose expected source/section is retrieved at K (vector and hybrid) |
| Precision@1, Precision@4 | section-based precision at K |
| Evidence recall | before/after expansion, over cases with `evidence` |
| Rejection rate | fraction of unanswerable cases correctly rejected (higher is better) |
| False-positive rate | fraction of unanswerable/misleading cases answered (lower is better) |
| Embedding latency | wall time to embed a fixed representative batch |
| Peak memory | resident memory during ingest |
| Variance | spread across ≥3 repeat runs |

## 7. Harness work required (proposed)

- **Dataset loader:** extend `cmd/eval` to report the rejection/false-positive rate
  per dataset explicitly (already printed for the single unanswerable case; make it a
  first-class metric for a set of them).
- **Latency/memory instrumentation:** a small, opt-in measurement path (not in the
  production `cmd/rag`), so embed latency and peak RSS are recorded without changing
  defaults.
- **Manifest + calibration tooling:** a script or documented commands for the floor
  sweep and the manifest-driven ingest.
- These are **evaluation-harness** changes only; no retrieval, embedding, ingestion,
  or answer-path behaviour changes.

## 8. Acceptance (for the proposed task)

- The expanded dataset (≥30 answerable + ≥6 unanswerable + ≥3 misleading) is
  committed with deterministic labels and a corpus manifest.
- Per-model metrics are reported at a calibrated floor, across ≥3 repeats, with
  variance and resource numbers.
- A model-selection decision is recorded against §2, and — if no candidate clears the
  bar — the incumbent is retained and the negative result is documented.
- No production embedding default changes as part of this task.

## 9. Risks

- **Small-corpus overfitting:** a 30-case set still has wide confidence intervals;
  report variance and avoid over-reading small deltas.
- **Threshold sensitivity:** the floor sweep is essential; skipping it invalidates the
  comparison (the RAG-018 confound).
- **Resource measurement noise:** RSS/latency vary with host load; use fixed batches
  and repeats.
- **Non-OSI candidate:** `embeddinggemma`'s license needs an explicit decision before
  any promotion.

## 10. Out of scope

- Changing the production embedding model or dimension (this plan only *measures*).
- OCR (RAG-008) and the eval capstone (RAG-016) remain frozen; genealogy untouched.
- Any code change beyond the evaluation harness.

## 11. Tasks (SOP)

### RAG-021 — Evaluation-harness validation and repeatable model comparison

- **Objective:** Add a validation test that checks the expanded bilingual dataset
  against the corpus manifest (every expected source is present in the manifest and
  every evidence string occurs in the referenced section's text), and a repeatable
  evaluation-runner script that calibrates `MIN_SIMILARITY`, runs N repeats per
  model, and records recall/precision/evidence/rejection alongside embedding latency
  and peak RSS, using one isolated database per model. No production default changes.
- **Files (expected):** `cmd/eval/main_test.go` (dataset/manifest validation test);
  new `scripts/eval-model-sweep.sh` (calibration + repeats + resource measurement);
  `docs/operations/embedding-models.md` (point at the script and the expanded
  dataset). Do **not** change `internal/retrieval`, `internal/embedding`,
  `internal/ingestion`, config defaults, or the production answer path.
- **Depends on:** none
- **Acceptance:**
  - The validation test passes for `evals/retrieval-vi-en-expanded.json` against
    `evals/corpus-vi-en.json`: every expected `source` is in the manifest, every
    referenced `section` exists in that file, and every `evidence` string is a
    substring of that section's text.
  - The runner script calibrates a floor per model, runs at least three repeats, and
    reports recall@1/recall@4, precision, evidence recall, rejection rate, embedding
    latency, and peak RSS, one isolated database per model.
  - No production embedding model or dimension changes.
- **Validation gate:** `go build ./... && go vet ./... && go test ./...` green
  (optionally `go test -race ./...`); a smoke run of the script against one model.
- **Execution:** `go build ./... && go vet ./... && go test ./...`.
- **Rollback:** remove the validation test and the script; no behaviour change
  remains (they are evaluation-only).
- **Scope expansion:** **none** — evaluation harness only.
