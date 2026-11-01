# Embedding-model sweep results

Recorded by `scripts/eval-model-sweep.sh`. Two sets are shown: the **final
calibrated evaluation** (the RAG-021 result) and the **historical exploratory**
rows it supersedes.

- Held-out evaluation split: `evals/retrieval-vi-en-expanded.json`
  (sha256 `72d2c15b3b91966c9471357e3abbec75f71b5859b5f36120ced24949f0539451`, 39 cases)
- Calibration split: `evals/retrieval-vi-en-calibration.json`
  (sha256 `a6e4bddd6a998cdbbd2034f4bb1fa7b0f9759883a9db2e09b055cfb2f01d24a0`, 21 cases)
- Corpus manifest: `evals/corpus-vi-en.json`
  (sha256 `7833e235b90470cd49eb7428f0d08c64145690347d02d53ba39d9a962ef7789b`)
- Toolchain: **Go 1.27.2** (`govulncheck ./...` clean). Repeats: **3 per model.**

## Final calibrated evaluation (RAG-021)

Selection rule (fixed before the comparison): among floors whose **calibration**
rejection is at least the minimum (`SWEEP_MIN_REJECTION`, default `1`), choose the
highest recall@4, then the highest rejection, then the highest floor; if none
qualifies, choose the highest rejection, then the highest recall@4, then the
highest floor. A zero-rejection floor can never win on recall alone.

### Calibration thresholds (calibration split, 21 cases)

| model | floor 0.30 | 0.40 | 0.50 | 0.60 | 0.70 | selected |
| ----- | ---------- | ---- | ---- | ---- | ---- | -------- |
| nomic-embed-text | R@4 0.60 / rej 0.00 | R@4 0.60 / rej 0.00 | R@4 0.60 / rej 0.00 | R@4 0.60 / rej 0.83 | **R@4 0.60 / rej 1.00** | **0.70** |
| embeddinggemma | R@4 0.87 / rej 0.00 | R@4 0.87 / rej 0.67 | R@4 0.80 / rej 0.83 | R@4 0.60 / rej 1.00 | **R@4 0.60 / rej 1.00** | **0.70** |

`embeddinggemma`'s higher calibration recall at floor `0.30` (R@4 0.87) sits on a
**zero-rejection** floor and is correctly excluded by the joint rule; the
`0.60`/`0.70` tie breaks to the higher floor. **Both models selected floor `0.70`.**

### Held-out evaluation (evaluation split, 39 cases, floor 0.70, mean of 3 repeats)

| model | R@1 | R@4 | P@4 | evidence (before→after) | rejection | run wall (s) | client RSS (KiB) |
| ----- | --- | --- | --- | ----------------------- | --------- | ------------ | ---------------- |
| **nomic-embed-text** | **0.42** | **0.52** | 0.22 | 0.42 → **0.48** | 1.00 | 1.02 (0.97–1.05) | 35340 (32956–37428) |
| embeddinggemma | 0.27 | 0.27 | 0.22 | 0.24 → 0.27 | 1.00 | 1.34 (1.33–1.36) | 33235 (32692–33592) |

**Variance:** every metric spread is `0.0000` across the three repeats for both
models (deterministic); only run-wall and client RSS vary slightly.

**Integration tests:** 26/26 PASS against a single disposable database
(14 in `internal/retrieval`, 12 in `internal/ingestion`, run with `-count=1`).

### Recommendation

**Retain `nomic-embed-text` (768) for production.** At the jointly-selected floor
`0.70`, it beats `embeddinggemma` on recall@1 (+0.15), recall@4 (+0.25), and
evidence recall (+0.21), with equal precision (0.22) and equal rejection (1.00).
This reverses the historical exploratory row below, which was produced under the
earlier recall-only rule that allowed a zero-rejection floor to win on recall.

## Measurement labels

- `run wall (s)` — wall-clock of the whole `go run ./cmd/eval` subprocess
  (client-observed; includes the Go build, the eval client, and database work),
  **not** embedding-only latency.
- `client RSS (KiB)` — peak resident set size of that eval client process, **not**
  the embedding model server's memory.
- `embed endpoint (ms)` — opt-in (`SWEEP_EMBED_PROBE=1`) client-observed latency of
  the Ollama `/api/embed` endpoint; `n/a` when off. Server-side memory is not
  measured by this harness.

`QUERY_REWRITE=false` is the sweep's default for its eval runs
(`SWEEP_QUERY_REWRITE`, default `false`), so the embedding comparison is
deterministic and needs no chat model. This is a harness-only setting: it does
**not** change the production default (`QUERY_REWRITE` remains `true` in the
application). Set `SWEEP_QUERY_REWRITE=1` to keep the production behavior (which
then requires an available `OLLAMA_CHAT_MODEL`).

## Defect and correction (RAG-021)

The first sweep recorded **all-zero metrics**. Cause: `cmd/eval` defaults
`QUERY_REWRITE=true`, so each eval called the chat model (`qwen3.8:27b-mlx`,
absent in this environment) and exited `1`; the runner swallowed the non-zero exit
(`|| true`) and recorded empty metrics as zeros. Correction: the runner now
**fails closed** — a non-zero eval run aborts the sweep (exit 3) and prints the
eval output instead of recording zeros — and its eval runs default to
`QUERY_REWRITE=false` so the embedding comparison does not depend on a chat model.

| model | version | dim | floor | repeat | R@1 | R@4 | P@4 | evidence | rejection | run wall (s) | client RSS (KiB) |
| ----- | ------- | --- | ----- | ------ | --- | --- | --- | -------- | --------- | ------------ | ---------------- |
| nomic-embed-text | unversioned | 768 | 0.70 | 1 | 0.42 | 0.52 | 0.22 | 0.42→0.48 | 1.00 | 0.97 | 35636 |
| nomic-embed-text | unversioned | 768 | 0.70 | 2 | 0.42 | 0.52 | 0.22 | 0.42→0.48 | 1.00 | 1.05 | 32956 |
| nomic-embed-text | unversioned | 768 | 0.70 | 3 | 0.42 | 0.52 | 0.22 | 0.42→0.48 | 1.00 | 1.03 | 37428 |
| embeddinggemma | unversioned | 768 | 0.70 | 1 | 0.27 | 0.27 | 0.22 | 0.24→0.27 | 1.00 | 1.33 | 33592 |
| embeddinggemma | unversioned | 768 | 0.70 | 2 | 0.27 | 0.27 | 0.22 | 0.24→0.27 | 1.00 | 1.34 | 33420 |
| embeddinggemma | unversioned | 768 | 0.70 | 3 | 0.27 | 0.27 | 0.22 | 0.24→0.27 | 1.00 | 1.36 | 32692 |

## Historical exploratory results (superseded)

Recorded earlier with the **recall-only** floor rule (before the joint criterion
and the split). Retained for provenance only; **not** a valid operating point under
the current rule.

| model | version | dim | floor | repeat | R@1 | R@4 | P@4 | evidence | rejection | run wall (s) | client RSS (KiB) |
| ----- | ------- | --- | ----- | ------ | --- | --- | --- | -------- | --------- | ------------ | ---------------- |
| nomic-embed-text | unversioned | 768 | 0.60 | 1 | 0.48 | 0.67 | 0.28 | 0.58→0.64 | 0.67 | 1.09 | 33712 |
| nomic-embed-text | unversioned | 768 | 0.60 | 2 | 0.48 | 0.67 | 0.28 | 0.58→0.64 | 0.67 | 1.12 | 34320 |
| nomic-embed-text | unversioned | 768 | 0.60 | 3 | 0.48 | 0.67 | 0.28 | 0.58→0.64 | 0.67 | 1.08 | 33568 |
| embeddinggemma | unversioned | 768 | 0.30 | 1 | 0.67 | 0.94 | 0.28 | 0.79→0.85 | 0.00 | 1.37 | 33472 |
| embeddinggemma | unversioned | 768 | 0.30 | 2 | 0.67 | 0.94 | 0.28 | 0.79→0.85 | 0.00 | 1.41 | 32572 |
| embeddinggemma | unversioned | 768 | 0.30 | 3 | 0.67 | 0.94 | 0.28 | 0.79→0.85 | 0.00 | 1.42 | 33260 |

> The `embeddinggemma` rows at floor `0.30` have rejection `0.00` and are
> ineligible under the joint rule; they are shown only to document the earlier
> method and its confound.
