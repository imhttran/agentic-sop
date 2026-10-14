# Julia (ONNX) fixtures

**Illustrative, synthetic fixtures** for the `internal/providers/julia` adapter.
They document the translation contract between the provider-neutral `decision`
package and the Julia runtime. **No live runtime ships here**: the default test
suite is fully offline and never executes Julia, Python, or ONNX.

The sample `Outputs` below was **not** captured from a real model; a genuine
golden fixture (with recorded provenance) is tracked in
[`../../docs/plans/BACKLOG.md`](../../docs/plans/BACKLOG.md).

## Julia runtime contract

The Julia adapter isolates concrete ONNX execution behind the `Runner` seam. By
default `CommandRunner` runs the repo-owned helper `tools/julia/infer.py`, which
marshals a Julia runtime request on stdin and emits `Outputs` on stdout. An
operator may override the command with `JULIA_INFERENCE_CMD`.

### Inputs (stdin)

`qtype` is Julia's native question type (`0` choice, `1` score, `2` noul/boolean);
`options` are the ordered Julia options (fixed `["false","true"]` for boolean).

```json
{
  "state": "deploying a schema migration during business hours",
  "questions": [
    {
      "id": "risk",
      "type": "choice",
      "qtype": 0,
      "text": "Assess deployment risk.",
      "options": ["LOW", "MEDIUM", "HIGH"]
    },
    {
      "id": "execution_model",
      "type": "choice",
      "qtype": 0,
      "text": "Preferred execution model",
      "options": ["SMALL", "MEDIUM", "LARGE"]
    },
    {
      "id": "approval_required",
      "type": "boolean",
      "qtype": 2,
      "text": "Is human approval required?",
      "options": ["false", "true"]
    }
  ]
}
```

### Outputs (stdout)

See [risk-evaluation.outputs.json](risk-evaluation.outputs.json). A CHOICE answer
carries a selected label and per-option probabilities; a BOOLEAN answer carries
`probability` (P(true)); a SCORE answer carries `score` (the expected zero-based
option index). Confidence is omitted: Julia exposes no distinct confidence value.

## Normalization rules

`NormalizeOutputs` requires an answer for every requested question. Per type:

- `choice` — `choice` must be non-empty and one of the question's allowed
  options; every probability key must be an allowed option; every probability
  value must be in `[0, 1]`. Probabilities are not required to sum to 1.0.
- `boolean` — `probability` must be present and in `[0, 1]`.
- `score` — `score` must be present.
- `confidence`, when present, must be in `[0, 1]`.

Julia's 2–20 option limit for CHOICE/SCORE is enforced by `BuildInputs` (and
therefore by `Decide`) with `decision.ErrInvalidRequest`. Every output rejection
wraps `decision.ErrMalformedResponse`. The result advertises `provider` =
`"julia"` and copies the model identifier verbatim.

These fixtures are documentation only; they are not executed by the default
suite. The opt-in live test is guarded by `JULIA_INTEGRATION_TEST=1` and skips
when no model is configured.
