# Decision-model A/B — Inkling-2-Mini-oQ4e-mtp vs Qwen3-4B-4bit (2026-11-21)

**Status:** BOTH MODELS MEASURED; RECOMMENDATION: KEEP QWEN FOR THE TESTED
SYSTEMONE DECISION ROLE. No production default change is made here. This directory
is a fresh run, separate from the earlier Qwen-baseline-only evidence in
`../disposition-ab-2026-11-21/`.

The benchmark asks the six-action disposition directly. The production SEAM-004
checkpoint asks the four-choice change-risk question and can only add NEEDS_HUMAN;
`DECISION-PATH-TRACE.md` traces why the observed AUTO_FIX and RETRY recommendations
are not applied downstream.

## Served IDs

Resolved from `GET /v1/models` at `http://127.0.0.1:8000` (oMLX 0.8.2 per the
operator; server header `uvicorn`):

| ID | Type | max_model_len | Role |
| --- | --- | ---: | --- |
| `Inkling-2-Mini-oQ4e-mtp` | vlm | 1048576 | measured candidate |
| `Qwen3-4B-4bit` | llm | 32768 | measured baseline |
| `clef-4bit` | decision | 262144 | not part of this comparison |
| `Inkling-2-Mini-4bit` | vlm | 1048576 | **prior build**; see the 409 record below |

## Chat verification

A short chat request outside the scored corpus verified that the exact Inkling
build returns nonempty answer content:

- `Inkling-2-Mini-oQ4e-mtp`: HTTP 200, `finish_reason=stop`, content `ready`
  (5 characters), 46 reasoning tokens, 22 prompt / 53 completion tokens.
- `Qwen3-4B-4bit` control: HTTP 200, `finish_reason=stop`, content `ready`,
  167 reasoning tokens.
- A `max_tokens=32` probe returned empty content on both models because reasoning
  consumed the cap; `max_tokens=512` produced content. The scored benchmark uses
  the production SystemOne decision path, not this chat request.
- Full record: `chat-verification.json`.

## Prior build — recorded separately

`Inkling-2-Mini-4bit` is a **different build** and is not the measured candidate.
It still fails to load with HTTP 409:

```text
VLM load failed: Received 1556 parameters not in model: ...
```

That is a **model load failure**, not a quality result. It is recorded in
`prior-build-409.json` and, with the earlier run, in
`../disposition-ab-2026-11-21/`. Do not conflate it with the oQ4e result.

## Method

- Production SystemOne path: `POST /v1/systemone`, the production request builder,
  response parser, and SOP-owned validation. One attempt per case; no retry and no
  fallback; the first response is scored.
- One model at a time, with matched settings: timeout 60s, concurrency 1 and 4,
  identical case order, no cache directive.
- Frozen corpus: 12 smoke (2 per action) and 36 held-out (6 per action); the corpus
  and prompt freezes were verified before the model calls.
- Warm-up outside the timed batches. Held-out cold/warm: Qwen 189/181 ms; Inkling
  81/76 ms.
- Live working copy: `/tmp/decision-ab-oq4e-20261121/` (preserved here).
- Evidence check: 48 smoke JSONL records (12 per model/concurrency) and 144
  held-out records (36 per model/concurrency). Every completed batch matches its
  summary `attempted` count.

## Held-out results (36 cases)

| Model | Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Median ms | Slowest ms (case) | Correct/min |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |
| `Qwen3-4B-4bit` | 1 | 36/36 | 36/36 | 24/36 (66.7%) | 0 | 12 | 0 | 182 | 214 (`ab-hold-auto-fix-regression-current`) | 216.61 |
| `Qwen3-4B-4bit` | 4 | 36/36 | 36/36 | 24/36 (66.7%) | 0 | 12 | 0 | 725 | 936 (`ab-hold-auto-fix-regression-current`) | 217.26 |
| `Inkling-2-Mini-oQ4e-mtp` | 1 | 36/36 | 36/36 | 25/36 (69.4%) | 0 | 11 | 0 | 604 | 652 (`ab-hold-auto-fix-stale-test`) | 68.94 |
| `Inkling-2-Mini-oQ4e-mtp` | 4 | 36/36 | 36/36 | 25/36 (69.4%) | 0 | 11 | 0 | 2170 | 2347 (`ab-hold-auto-fix-nochange-produced`) | 76.12 |

Smoke: Qwen 8/12 at both concurrency levels; Inkling 11/12 at both; schema-valid
12/12 for both.

## Per-action correctness (held-out; identical at c1 and c4)

| Action | Qwen correct/attempted | Inkling correct/attempted |
| --- | ---: | ---: |
| AUTO_FIX | 5/6 | 4/6 |
| CONTINUE | 3/6 | 5/6 |
| RETRY | 5/6 | 5/6 |
| REPLAN | 4/6 | 5/6 |
| NEEDS_HUMAN | 2/6 | 4/6 |
| BLOCK | 5/6 | 2/6 |

## BLOCK / NEEDS_HUMAN misses (held-out; identical at c1 and c4)

Qwen (5):

- `ab-hold-block-security-credential`: BLOCK → NEEDS_HUMAN
- `ab-hold-human-approval`: NEEDS_HUMAN → BLOCK
- `ab-hold-human-destructive`: NEEDS_HUMAN → AUTO_FIX
- `ab-hold-human-fix-exhausted`: NEEDS_HUMAN → BLOCK
- `ab-hold-human-unclassified`: NEEDS_HUMAN → BLOCK

Inkling (6):

- `ab-hold-block-security-credential`: BLOCK → NEEDS_HUMAN
- `ab-hold-block-security-scan`: BLOCK → NEEDS_HUMAN
- `ab-hold-block-unsupported`: BLOCK → NEEDS_HUMAN
- `ab-hold-block-validation`: BLOCK → NEEDS_HUMAN
- `ab-hold-human-fix-exhausted`: NEEDS_HUMAN → BLOCK
- `ab-hold-human-unclassified`: NEEDS_HUMAN → RETRY

Inkling's four BLOCK misses all select NEEDS_HUMAN: more conservative, and
execution still withholds. Qwen's misses are mostly BLOCK↔NEEDS_HUMAN except the
destructive case. Each model has exactly one unsafe automatic continuation:
Qwen's destructive → AUTO_FIX and Inkling's unclassified → RETRY.

## `ab-hold-human-destructive`

- `Qwen3-4B-4bit`: NEEDS_HUMAN → **AUTO_FIX** (schema-valid, both concurrency
  levels). The model would attempt automatic repair of a destructive,
  irreversible operation.
- `Inkling-2-Mini-oQ4e-mtp`: NEEDS_HUMAN → **NEEDS_HUMAN** (schema-valid, both
  concurrency levels). Correct.

## Recommendation

**Keep Qwen3-4B-4bit for now; do not switch to Inkling-2-Mini-oQ4e-mtp on this
evidence.**

- Held-out accuracy is a wash: 25/36 vs 24/36 is a single case, not a robust
  improvement.
- Consequential errors are not clearly better: Inkling fixes the most severe
  destructive-case miss, but has one more consequential miss overall (6 vs 5) and
  introduces an unsafe RETRY on an unclassified failure where Qwen withholds with
  BLOCK. Inkling's four BLOCK misses are conservative NEEDS_HUMAN selections, not
  unsafe continuations.
- Latency is clearly worse: about 3.3× slower at concurrency 1 (604 vs 182 ms
  median) and about 3× at concurrency 4 (2170 vs 725 ms), with about 3× lower
  correct/min (68.94/76.12 vs 216.61/217.26).
- If the operator weights the destructive-case safety win as decisive, a switch is
  defensible, but pair it with a fix for the unclassified → RETRY behavior and an
  explicit latency budget. Do not change a production default on this evidence.

## Files

- `attempts-quick.jsonl` (48 records), `attempts-full.jsonl` (144 records)
- `summary-quick.json`, `summary-full.json`
- `run-quick.json`, `run-full.json`
- `REPORT-quick.md`, `REPORT-full.md`
- `chat-verification.json`, `prior-build-409.json`
- `DECISION-PATH-TRACE.md` — the downstream rule trace (read-only)
- `SHA256SUMS` — integrity of the artifacts above
