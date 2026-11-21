# Decision-model A/B — heldout (full)

- Corpus: `0296db27e415de9b7eb19d2df3508a43665969164b0cba8eb7fef1f85c99dcfa`
- Prompt: `a128f3d4d2600ef5b09623648e3e1cb590759a7a978b592adb2be7c9f680173b`
- Models requested: `Qwen3-4B-4bit`, `Inkling-2-Mini-4bit`
- **Models unavailable:** `Inkling-2-Mini-4bit`

## Observable settings

- oMLX version: not exposed by the endpoint (null)
- Model revisions: `Inkling-2-Mini-4bit` = not exposed, `Qwen3-4B-4bit` = not exposed
- Reasoning: the production SystemOne transport exposes no reasoning/thinking parameter; the endpoint's decision engine applies its own setting
- Sampling: the production SystemOne transport exposes no sampling parameters; the endpoint's decision engine applies its own settings
- Cache: no cache directive is sent; the endpoint default applies

## Cold load and warm-up (outside the timed batches)

| Model | Cold load ms | Warm-up ms |
| --- | ---: | ---: |
| `Qwen3-4B-4bit` | 185 | 186 |

## First-pass correctness and latency

| Model | Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Other errors | Median ms | Slowest ms (case) | Correct/min | Complete |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- |
| `Qwen3-4B-4bit` | 1 | 36/36 (100.0%) | 36/36 (100.0%) | 24/36 (66.7%) | 0 | 12 | 0 | 0 | 186 | 219 (`ab-hold-auto-fix-regression-current`) | 212.86 | true |
| `Qwen3-4B-4bit` | 4 | 36/36 (100.0%) | 36/36 (100.0%) | 24/36 (66.7%) | 0 | 12 | 0 | 0 | 725 | 950 (`ab-hold-auto-fix-regression-current`) | 217.46 | true |

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

## Measurement notes

- One attempt per case. The decider calls the production clef provider directly and never retries or falls back; `fallbacks` is always 0.
- `elapsed_ms` is end-to-end around the attempt. `wall_ms` is the batch wall time used for correct/min.
- Cold-load and warm-up requests are measured outside the timed batches (see `run-*.json`).
- The production SystemOne response reports input/output tokens only; time-to-first-answer-content, decode speed, and model-load duration are null.
- A field the endpoint does not report is null, never a measured zero.
- `Inkling-2-Mini-4bit` was not measured: warm-up failed: HTTP 409: {"error":{"message":"Model 'Inkling-2-Mini-4bit' is unavailable after a previous load failure: VLM load failed: Received 1556 parameters not in model: \nmodel.language_model.embed_norm.weight,\nmodel.language_model.embed_tokens.biases,\nmodel.language_model.embed_tokens.scales,\nmodel.language_model.embed_tokens.weight,\nmodel.language_model.final_input.query,\nmodel.language_model.layers.0.input_layernorm.weight,\nmodel.language_model.layers.0.mlp.experts.down_proj.global_scale,\nmodel.language_model.layers.0.mlp.experts.down_proj.scales,\nmodel.language_model.layers... (full endpoint error in run-full.json)
