# Decision-model A/B live evidence — 2026-11-21

**Status:** **Qwen baseline only.** The candidate (`Inkling-2-Mini-4bit`) is
unavailable because the runtime could not load it. That is a **model load
failure**, not a quality result. The replacement decision remains **open** until
the requested Inkling build completes the same frozen evaluation.

This directory preserves the live artifacts from the benchmark run recorded in
[DISPOSITION-AB-2026-11-21.md](../../../docs/reports/decision-model/DISPOSITION-AB-2026-11-21.md).
The follow-up fix for batch cancellation and evidence persistence did not change
the measurement code and did not require a re-run; these are the original live
artifacts.

## Frozen inputs

- Corpus: `evals/decision-model/disposition-ab-v1.json`
- Corpus sha256: `0296db27e415de9b7eb19d2df3508a43665969164b0cba8eb7fef1f85c99dcfa`
- Prompt sha256: `a128f3d4d2600ef5b09623648e3e1cb590759a7a978b592adb2be7c9f680173b`
- Freezes: `DISPOSITION-AB.sha256`, `DISPOSITION-AB-PROMPT.sha256`

## Run

- Endpoint: `http://127.0.0.1:8000` (server: `uvicorn`)
- Discovered: `Inkling-2-Mini-4bit` (vlm), `Qwen3-4B-4bit` (llm), `clef-4bit` (decision)
- Measured: `Qwen3-4B-4bit`
- Not measured: `Inkling-2-Mini-4bit`
- Concurrency: 1 and 4
- Timeout: 60s per attempt
- One production SystemOne attempt per case; no retry, no fallback
- Splits: 12 smoke (2 per action) and 36 held-out (6 per action)

## Candidate availability

The endpoint advertises `Inkling-2-Mini-4bit`, but every request fails at load:

```text
HTTP 409: Model 'Inkling-2-Mini-4bit' is unavailable after a previous load failure:
VLM load failed: Received 1556 parameters not in model: ...
```

The full endpoint error is in `run-quick.json` and `run-full.json` under
`model_status`. No model was substituted.

## Measured baseline (held-out, 36 cases)

| Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Median ms | Slowest ms (case) | Correct/min |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: |
| 1 | 36/36 | 36/36 | 24/36 (66.7%) | 0 | 12 | 0 | 186 | 219 (`ab-hold-auto-fix-regression-current`) | 212.86 |
| 4 | 36/36 | 36/36 | 24/36 (66.7%) | 0 | 12 | 0 | 725 | 950 (`ab-hold-auto-fix-regression-current`) | 217.46 |

Smoke (12 cases): 8/12 correct, 12/12 schema-valid, at both concurrency levels.

Per-action (held-out, both concurrency levels): AUTO_FIX 5/6, CONTINUE 3/6,
RETRY 5/6, REPLAN 4/6, NEEDS_HUMAN 2/6, BLOCK 5/6.

The consequential `BLOCK`/`NEEDS_HUMAN` disagreements are listed in
`REPORT-full.md` and the report. The most consequential is
`ab-hold-human-destructive` (NEEDS_HUMAN → AUTO_FIX).

## Files

- `attempts-*.jsonl` — one JSON object per attempted case.
- `summary-*.json` — per-model, per-concurrency metrics and disagreements.
- `run-*.json` — endpoint/model metadata, settings, case order, model status.
- `REPORT-*.md` — rendered operator report.
- `SHA256SUMS` — integrity of the files above.

## Decision

**Qwen baseline only; replacement decision open.** Do not change a model default
on this evidence. Re-run the same frozen corpus once the requested Inkling build
can serve; only then compare held-out accuracy, consequential errors, and latency.
