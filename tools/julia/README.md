# Julia-1-ONNX inference helper

`infer.py` is the **repository-owned** execution path behind the Julia adapter's
`Runner` seam. It reads one JSON request on stdin, runs Julia-1-ONNX through ONNX
Runtime, and writes one JSON response on stdout.

It replaces the "bring your own inference command" placeholder with a real,
inspectable pipeline:

```text
decision.DecisionRequest
        ↓
Julia adapter (BuildInputs)          Go
        ↓
Julia runtime request (JSON stdin)
        ↓
infer.py  ── tokenizer ──▶ ONNX tensors ──▶ Julia-1-ONNX ──▶ logits ──▶ decode
        ↓
Outputs (JSON stdout)
        ↓
Julia adapter (NormalizeOutputs)      Go
        ↓
decision.DecisionResult
```

## Setup

The default Go test suite and `go build ./...` **never** need any of this. The
helper is only needed for real inference and the opt-in live test.

```sh
python3 -m venv .venv
. .venv/bin/activate
pip install -r tools/julia/requirements.txt     # onnxruntime, tokenizers, numpy (CPU)
```

The ONNX model is **not** committed here. Download/set it up separately and point
`JULIA_MODEL_PATH` at it:

```sh
export JULIA_MODEL_PATH=/path/to/julia-1.onnx
export JULIA_TOKENIZER_PATH=/path/to/tokenizer.json   # default: tokenizer.json beside the model
```

## Protocol

stdin:

```json
{
  "state": "...",
  "questions": [
    {
      "id": "risk",
      "type": "choice",
      "qtype": 0,
      "text": "Assess deployment risk.",
      "options": ["LOW", "MEDIUM", "HIGH"]
    }
  ]
}
```

stdout (success):

```json
{
  "model": "julia",
  "answers": {
    "risk": {
      "choice": "HIGH",
      "probabilities": { "LOW": 0.01, "MEDIUM": 0.04, "HIGH": 0.95 }
    }
  }
}
```

stdout (failure, exit non-zero) — a single machine-readable envelope so the Go
adapter can classify the failure:

```json
{ "error": { "kind": "unavailable", "message": "JULIA_MODEL_PATH is not set" } }
```

`kind` is `unavailable` (missing runtime/model/config), `malformed` (bad input,
bad logits shape, non-finite values), or `failure` (unexpected runtime error).
Human diagnostics go to **stderr**; stdout is reserved for exactly one JSON
object.

## Environment

| Variable               | Required | Default                           | Meaning                                     |
| ---------------------- | -------- | --------------------------------- | ------------------------------------------- |
| `JULIA_MODEL_PATH`     | yes      | —                                 | ONNX model path.                            |
| `JULIA_TOKENIZER_PATH` | no       | `tokenizer.json` beside the model | tokenizer.json for the model's tokenizer.   |
| `JULIA_MODEL_ID`       | no       | `julia`                           | model id reported on stdout.                |
| `JULIA_MAX_TOKENS`     | no       | `8192`                            | encoder context limit (truncation/padding). |

## Semantics

Julia is a **supplied-options** decision model. Each native decision carries an
ordered option list and a question type (`qtype`):

| qtype | Meaning      | Decoding                                                                 |
| ----- | ------------ | ------------------------------------------------------------------------ |
| `0`   | choice       | `argmax` → selected label; per-option probabilities from stable softmax. |
| `1`   | score        | **expected zero-based option index** `sum(i * p_i)` — _not_ `argmax`.    |
| `2`   | noul/boolean | `P(true)` = softmax over `["false", "true"]`, index 1.                   |

- CHOICE and SCORE require **2–20** ordered options; BOOLEAN always uses exactly
  `["false", "true"]`.
- No confidence value is fabricated: Julia's documented runtime exposes no
  distinct confidence, so the helper omits it.

ONNX graph contract (input → output):

```text
input_ids       int64  [batch, tokens]
attention_mask  int64  [batch, tokens]
marker_pos      int64  [batch, options]
marker_mask     bool   [batch, options]
qtype           int64  [batch]
        ↓
logits          float  [batch, options]
```

Raw logits are validated (length, finiteness) before softmax; softmax is
numerically stable (`exp(x - max)`).

## HTTP bridge (`http_bridge.py`)

`http_bridge.py` is an **alternative** implementation of the same `CommandRunner`
JSON stdin/stdout contract. Instead of running ONNX in-process it forwards each
question to an external Julia HTTP service (`JULIA_URL`, default
`http://127.0.0.1:8011`) and normalizes the response back into `Outputs`. It is an
operator override of the inference command, **not** a new provider API:

```sh
export JULIA_INFERENCE_CMD="python3 $PWD/tools/julia/http_bridge.py"
export JULIA_URL="http://127.0.0.1:8011"
export JULIA_MODEL="SupersonicLabs/Julia-1"
```

| provider-neutral | Julia (HTTP) | response → `Outputs`                                                            |
| ---------------- | ------------ | ------------------------------------------------------------------------------- |
| `choice`         | `choice`     | `{index, probabilities}` → `{choice, probabilities}`                            |
| `boolean`        | `noul`       | `{index, probabilities}` → `{probability: P(true)}`; options `["false","true"]` |
| `score`          | `score`      | `{index, probabilities}` → `{score: Σ i·pᵢ}` (expected index, **not** argmax)   |

The `boolean → noul` translation is confined to the bridge. Configuration:
`JULIA_URL` (service root), `JULIA_HTTP_TIMEOUT` (default `30`s), and
`JULIA_MODEL_ID` / `JULIA_MODEL` (model id reported). Failures are emitted on
stdout as `{"error":{"kind":"unavailable|malformed|failure","message":…}}`
with diagnostics on **stderr**.

## ⚠️ Encoder verification status

The **tensor contract**, **qtype mapping**, **option limits**, **BOOLEAN /
SCORE semantics**, and the **decoding rules** are implemented from the documented
Julia-1-ONNX model contract.

The **encoding** of state/question/options into `marker_pos`/`marker_mask`
(`Runtime.encode` + `find_marker_positions` in `infer.py`) is a **best-effort,
clearly-isolated implementation** of that contract. It was **not** re-verified
against the upstream Julia-1-ONNX export in this environment (no upstream model
or export artifacts were available). Treat it as the single place to reconcile
with upstream before trusting live output, and update
[`../../docs/specs/julia-adapter.md`](../../docs/specs/julia-adapter.md)
accordingly.

## Tests

Pure functions (softmax, argmax, expected-index, validation, marker search,
prompt rendering) are unit-tested with the standard library only — no numpy,
ONNX Runtime, tokenizer, model, or network:

```sh
python3 tools/julia/test_infer.py
python3 tools/julia/test_http_bridge.py
```

`test_http_bridge.py` drives the HTTP bridge against a local in-process **mock**
server (never a real service, never port 8011): it asserts the exact
`boolean → noul` request payload, choice probability preservation, multi-question
aggregation, and the full failure taxonomy (unavailable, timeout, HTTP 4xx/5xx,
malformed JSON, bad index/probabilities, unsupported type, malformed stdin).

The live end-to-end paths are opt-in and SKIP when not enabled:

```sh
export JULIA_INTEGRATION_TEST=1         # Go → infer.py → ONNX
# or
export JULIA_HTTP_INTEGRATION_TEST=1    # Go → http_bridge.py → Julia HTTP
# with
export JULIA_URL=http://127.0.0.1:8011
export JULIA_MODEL_PATH=/path/to/julia-1.onnx   # only for the ONNX path
go test ./...
```
