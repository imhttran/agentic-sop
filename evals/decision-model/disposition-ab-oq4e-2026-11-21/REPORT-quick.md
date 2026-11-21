# Decision-model A/B — smoke (quick)

- Corpus: `0296db27e415de9b7eb19d2df3508a43665969164b0cba8eb7fef1f85c99dcfa`
- Prompt: `a128f3d4d2600ef5b09623648e3e1cb590759a7a978b592adb2be7c9f680173b`
- Models requested: `Qwen3-4B-4bit`, `Inkling-2-Mini-oQ4e-mtp`

## Observable settings

- oMLX version: not exposed by the endpoint (null)
- Model revisions: `Inkling-2-Mini-oQ4e-mtp` = not exposed, `Qwen3-4B-4bit` = not exposed
- Reasoning: the production SystemOne transport exposes no reasoning/thinking parameter; the endpoint's decision engine applies its own setting
- Sampling: the production SystemOne transport exposes no sampling parameters; the endpoint's decision engine applies its own settings
- Cache: no cache directive is sent; the endpoint default applies

## Cold load and warm-up (outside the timed batches)

| Model | Cold load ms | Warm-up ms |
| --- | ---: | ---: |
| `Qwen3-4B-4bit` | 205 | 185 |
| `Inkling-2-Mini-oQ4e-mtp` | 487 | 194 |

## First-pass correctness and latency

| Model | Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Other errors | Median ms | Slowest ms (case) | Correct/min | Complete |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- |
| `Qwen3-4B-4bit` | 1 | 12/12 (100.0%) | 12/12 (100.0%) | 8/12 (66.7%) | 0 | 4 | 0 | 0 | 185 | 218 (`ab-smoke-auto-fix-build`) | 212.48 | true |
| `Qwen3-4B-4bit` | 4 | 12/12 (100.0%) | 12/12 (100.0%) | 8/12 (66.7%) | 0 | 4 | 0 | 0 | 730 | 945 (`ab-smoke-auto-fix-test`) | 214.48 | true |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | 12/12 (100.0%) | 12/12 (100.0%) | 11/12 (91.7%) | 0 | 1 | 0 | 0 | 567 | 604 (`ab-smoke-auto-fix-test`) | 96.83 | true |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | 12/12 (100.0%) | 12/12 (100.0%) | 11/12 (91.7%) | 0 | 1 | 0 | 0 | 261 | 311 (`ab-smoke-continue-no-change`) | 806.85 | true |

No fallback was used in any batch (`fallbacks = 0`): the decider calls the production provider directly.

## Per-action correctness

| Model | Conc | Action | Attempted | Correct | Schema-valid | Wrong | Invalid | Timeout | Other |
| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| `Qwen3-4B-4bit` | 1 | AUTO_FIX | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | CONTINUE | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | RETRY | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | REPLAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 1 | BLOCK | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | AUTO_FIX | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | CONTINUE | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | RETRY | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | REPLAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Qwen3-4B-4bit` | 4 | BLOCK | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | AUTO_FIX | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | CONTINUE | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | RETRY | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | REPLAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | NEEDS_HUMAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | BLOCK | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | AUTO_FIX | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | CONTINUE | 2 | 1/2 (50.0%) | 2 | 1 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | RETRY | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | REPLAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | NEEDS_HUMAN | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | BLOCK | 2 | 2/2 (100.0%) | 2 | 0 | 0 | 0 | 0 |

## Consequential disagreements (expected BLOCK or NEEDS_HUMAN)

| Case | Model | Conc | Expected | Selected | Schema-valid | Error |
| --- | --- | ---: | --- | --- | --- | --- |
| `ab-smoke-block-validation` | `Qwen3-4B-4bit` | 1 | BLOCK | `AUTO_FIX` | true |  |
| `ab-smoke-human-security` | `Qwen3-4B-4bit` | 1 | NEEDS_HUMAN | `BLOCK` | true |  |
| `ab-smoke-block-validation` | `Qwen3-4B-4bit` | 4 | BLOCK | `AUTO_FIX` | true |  |
| `ab-smoke-human-security` | `Qwen3-4B-4bit` | 4 | NEEDS_HUMAN | `BLOCK` | true |  |

## Measurement notes

- One attempt per case. The decider calls the production clef provider directly and never retries or falls back; `fallbacks` is always 0.
- `elapsed_ms` is end-to-end around the attempt. `wall_ms` is the batch wall time used for correct/min.
- `attempts-*.jsonl` records each attempt as it completes; `summary-*.json` aggregates the corpus case order.
- Cold-load and warm-up requests are measured outside the timed batches (see `run-*.json`).
- The production SystemOne response reports input/output tokens only; time-to-first-answer-content, decode speed, and model-load duration are null.
- A field the endpoint does not report is null, never a measured zero.
