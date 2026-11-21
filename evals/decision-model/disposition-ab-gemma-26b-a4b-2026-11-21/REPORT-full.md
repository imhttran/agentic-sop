# Decision-model A/B — heldout (full)

- Corpus: `0296db27e415de9b7eb19d2df3508a43665969164b0cba8eb7fef1f85c99dcfa`
- Prompt: `a128f3d4d2600ef5b09623648e3e1cb590759a7a978b592adb2be7c9f680173b`
- Models requested: `Qwen3-4B-4bit`, `gemma-4-26B-A4B-it-MLX-4bit`

## Observable settings

- oMLX version: not exposed by the endpoint (null)
- Model revisions: `Qwen3-4B-4bit` = not exposed, `gemma-4-26B-A4B-it-MLX-4bit` = not exposed
- Reasoning: the production SystemOne transport exposes no reasoning/thinking parameter; the endpoint's decision engine applies its own setting
- Sampling: the production SystemOne transport exposes no sampling parameters; the endpoint's decision engine applies its own settings
- Cache: no cache directive is sent; the endpoint default applies

## Cold load and warm-up (outside the timed batches)

| Model | Cold load ms | Warm-up ms |
| --- | ---: | ---: |
| `Qwen3-4B-4bit` | 203 | 182 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 37 | 38 |

## First-pass correctness and latency

| Model | Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Other errors | Median ms | Slowest ms (case) | Correct/min | Complete |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- |
| `Qwen3-4B-4bit` | 1 | 36/36 (100.0%) | 36/36 (100.0%) | 24/36 (66.7%) | 0 | 12 | 0 | 0 | 183 | 215 (`ab-hold-auto-fix-regression-current`) | 215.96 | true |
| `Qwen3-4B-4bit` | 4 | 36/36 (100.0%) | 36/36 (100.0%) | 24/36 (66.7%) | 0 | 12 | 0 | 0 | 716 | 761 (`ab-hold-auto-fix-nochange-produced`) | 220.66 | true |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | 36/36 (100.0%) | 36/36 (100.0%) | 25/36 (69.4%) | 0 | 11 | 0 | 0 | 242 | 273 (`ab-hold-auto-fix-regression-current`) | 170.34 | true |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | 36/36 (100.0%) | 36/36 (100.0%) | 25/36 (69.4%) | 0 | 11 | 0 | 0 | 901 | 970 (`ab-hold-block-terminal`) | 182.68 | true |

No fallback was used in any batch (`fallbacks = 0`): the decider calls the production provider directly.

## Per-action correctness

| Model | Conc | Action | Attempted | Correct | Schema-valid | Wrong | Invalid | Timeout | Other |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `Qwen3-4B-4bit` | 1 | AUTO_FIX | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | CONTINUE | 6 | 3/6 (50.0%) | 6 | 3 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | RETRY | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | REPLAN | 6 | 4/6 (66.7%) | 6 | 2 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | 6 | 2/6 (33.3%) | 6 | 4 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | BLOCK | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | AUTO_FIX | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | CONTINUE | 6 | 3/6 (50.0%) | 6 | 3 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | RETRY | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | REPLAN | 6 | 4/6 (66.7%) | 6 | 2 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | 6 | 2/6 (33.3%) | 6 | 4 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | BLOCK | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | AUTO_FIX | 6 | 3/6 (50.0%) | 6 | 3 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | CONTINUE | 6 | 4/6 (66.7%) | 6 | 2 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | RETRY | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | REPLAN | 6 | 6/6 (100.0%) | 6 | 0 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | NEEDS_HUMAN | 6 | 2/6 (33.3%) | 6 | 4 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 1 | BLOCK | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | AUTO_FIX | 6 | 3/6 (50.0%) | 6 | 3 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | CONTINUE | 6 | 4/6 (66.7%) | 6 | 2 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | RETRY | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | REPLAN | 6 | 6/6 (100.0%) | 6 | 0 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | NEEDS_HUMAN | 6 | 2/6 (33.3%) | 6 | 4 | 0 | 0 | 0 |
| `gemma-4-26B-A4B-it-MLX-4bit` | 4 | BLOCK | 6 | 5/6 (83.3%) | 6 | 1 | 0 | 0 | 0 |

## Consequential disagreements (expected BLOCK or NEEDS_HUMAN)

| Case | Model | Conc | Expected | Selected | Schema-valid | Error |
| --- | --- | ---: | --- | --- | --- | --- |
| `ab-hold-block-security-credential` | `Qwen3-4B-4bit` | 1 | BLOCK | `NEEDS_HUMAN` | true |  |
| `ab-hold-human-approval` | `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-destructive` | `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | `AUTO_FIX` | true |  |
| `ab-hold-human-fix-exhausted` | `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-unclassified` | `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-block-security-credential` | `Qwen3-4B-4bit` | 4 | BLOCK | `NEEDS_HUMAN` | true |  |
| `ab-hold-human-approval` | `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-destructive` | `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | `AUTO_FIX` | true |  |
| `ab-hold-human-fix-exhausted` | `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-unclassified` | `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-block-security-credential` | `gemma-4-26B-A4B-it-MLX-4bit` | 1 | BLOCK | `NEEDS_HUMAN` | true |  |
| `ab-hold-human-approval` | `gemma-4-26B-A4B-it-MLX-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-destructive` | `gemma-4-26B-A4B-it-MLX-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-fix-exhausted` | `gemma-4-26B-A4B-it-MLX-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-unclassified` | `gemma-4-26B-A4B-it-MLX-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-block-security-credential` | `gemma-4-26B-A4B-it-MLX-4bit` | 4 | BLOCK | `NEEDS_HUMAN` | true |  |
| `ab-hold-human-approval` | `gemma-4-26B-A4B-it-MLX-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-destructive` | `gemma-4-26B-A4B-it-MLX-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-fix-exhausted` | `gemma-4-26B-A4B-it-MLX-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-hold-human-unclassified` | `gemma-4-26B-A4B-it-MLX-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |

## Measurement notes

- One attempt per case. The decider calls the production clef provider directly and never retries or falls back; `fallbacks` is always 0.
- `elapsed_ms` is end-to-end around the attempt. `wall_ms` is the batch wall time used for correct/min.
- `attempts-*.jsonl` records each attempt as it completes; `summary-*.json` aggregates the corpus case order.
- Cold-load and warm-up requests are measured outside the timed batches (see `run-*.json`).
- The production SystemOne response reports input/output tokens only; time-to-first-answer-content, decode speed, and model-load duration are null.
- A field the endpoint does not report is null, never a measured zero.
