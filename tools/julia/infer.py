#!/usr/bin/env python3
"""Repository-owned Julia-1-ONNX inference helper.

This helper is the default execution path behind the Julia adapter's ``Runner``
seam. It reads one JSON request on stdin, runs Julia-1-ONNX through ONNX Runtime,
and writes one JSON response on stdout.

Protocol
--------
stdin (one JSON object)::

    {"state": "...",
     "questions": [{"id": "risk", "type": "choice", "qtype": 0,
                    "text": "...", "options": ["LOW", "MEDIUM", "HIGH"]}]}

stdout, success (one JSON object)::

    {"model": "julia",
     "answers": {"risk": {"choice": "HIGH",
                          "probabilities": {"LOW": 0.01, "MEDIUM": 0.04, "HIGH": 0.95}}}}

stdout, failure (one JSON object, exit non-zero)::

    {"error": {"kind": "unavailable|malformed|failure", "message": "..."}}

stderr is for human diagnostics only. stdout is the machine-readable channel and
must contain exactly one JSON object.

Environment
-----------
``JULIA_MODEL_PATH``      path to the ONNX model                (required)
``JULIA_TOKENIZER_PATH``  path to tokenizer.json                (default: tokenizer.json beside the model)
``JULIA_MODEL_ID``        model identifier reported on stdout   (default: ``julia``)
``JULIA_MAX_TOKENS``      encoder context limit                 (default: 8192)

IMPORTANT — ENCODER VERIFICATION
--------------------------------
The tensor contract (``input_ids``, ``attention_mask``, ``marker_pos``,
``marker_mask``, ``qtype`` -> ``logits``) and the decoding rules are implemented
here from the documented Phase 2.1 model contract. The *encoding* of
state/question/options into ``marker_pos``/``marker_mask`` is a best-effort,
clearly-documented implementation: it MUST be re-verified against the upstream
Julia-1-ONNX export before it is trusted for live use. See tools/julia/README.md.
"""

from __future__ import annotations

import json
import math
import os
import sys

# Julia native question types.
QTYPE_CHOICE = 0
QTYPE_SCORE = 1
QTYPE_BOOLEAN = 2

# Julia's option-count limits for CHOICE and SCORE.
MIN_OPTIONS = 2
MAX_OPTIONS = 20

# Julia's fixed noul options: index 0 false, index 1 true.
BOOLEAN_OPTIONS = ["false", "true"]

DEFAULT_MODEL_ID = "julia"
DEFAULT_MAX_TOKENS = 8192

# ONNX graph tensor names (documented contract, to be re-verified against export).
GRAPH_INPUT_NAMES = ("input_ids", "attention_mask", "marker_pos", "marker_mask", "qtype")
GRAPH_LOGITS_NAME = "logits"


class HelperError(Exception):
    """A classified helper failure. ``kind`` maps onto the Go adapter's error
    classes: ``unavailable``, ``malformed``, or ``failure``."""

    def __init__(self, kind: str, message: str) -> None:
        super().__init__(message)
        self.kind = kind
        self.message = message


# --------------------------------------------------------------------------- #
# Pure functions (no numpy / onnxruntime / tokenizers) — unit-tested directly. #
# --------------------------------------------------------------------------- #


def render_prompt(state: str, text: str, options: list[str]) -> str:
    """Render the natural-language input for one Julia decision.

    Format (documented; see the module docstring caveat)::

        <state>

        <question text>

        Options:
        - <option 0>
        - <option 1>
        ...
    """
    lines = [state.strip(), "", text.strip(), "", "Options:"]
    for option in options:
        lines.append(f"- {option}")
    return "\n".join(lines)


def softmax(logits: list[float]) -> list[float]:
    """Numerically stable softmax: ``exp(x - max) / sum(exp(x - max))``."""
    if not logits:
        raise HelperError("malformed", "softmax received no logits")
    m = max(logits)
    exps = [math.exp(x - m) for x in logits]
    total = sum(exps)
    if total <= 0 or not math.isfinite(total):
        raise HelperError("malformed", "softmax produced a non-finite denominator")
    return [e / total for e in exps]


def argmax(values: list[float]) -> int:
    """Return the index of the largest value (first on ties)."""
    if not values:
        raise HelperError("malformed", "argmax received no values")
    best = 0
    for i in range(1, len(values)):
        if values[i] > values[best]:
            best = i
    return best


def expected_index(probabilities: list[float]) -> float:
    """Expected zero-based option index: ``sum(i * p_i)``.

    This is Julia's documented SCORE semantics. SCORE is *not* ``argmax``.
    """
    return sum(i * p for i, p in enumerate(probabilities))


def is_finite(value: float) -> bool:
    return isinstance(value, (int, float)) and math.isfinite(value)


def validate_logits(logits: list[float], n_options: int) -> None:
    """Validate a raw logits vector: expected length and finite values only."""
    if len(logits) != n_options:
        raise HelperError(
            "malformed",
            f"logits length {len(logits)} does not match option count {n_options}",
        )
    for i, value in enumerate(logits):
        if not is_finite(value):
            raise HelperError("malformed", f"logits[{i}] is not finite: {value!r}")


def validate_question(question: dict) -> None:
    """Validate one question's Julia-specific shape."""
    if not isinstance(question, dict):
        raise HelperError("malformed", "question is not an object")
    qid = question.get("id")
    if not isinstance(qid, str) or not qid.strip():
        raise HelperError("malformed", "question is missing a non-empty id")
    qtype = question.get("qtype")
    options = question.get("options") or []
    if not isinstance(qtype, int):
        raise HelperError("malformed", f"question {qid!r}: qtype must be an integer")
    if qtype == QTYPE_BOOLEAN:
        if options != BOOLEAN_OPTIONS:
            raise HelperError(
                "malformed",
                f"question {qid!r}: boolean (noul) options must be {BOOLEAN_OPTIONS}",
            )
        return
    if qtype in (QTYPE_CHOICE, QTYPE_SCORE):
        if not isinstance(options, list) or not all(isinstance(o, str) for o in options):
            raise HelperError("malformed", f"question {qid!r}: options must be a list of strings")
        if not (MIN_OPTIONS <= len(options) <= MAX_OPTIONS):
            raise HelperError(
                "malformed",
                f"question {qid!r}: Julia requires {MIN_OPTIONS}-{MAX_OPTIONS} options (got {len(options)})",
            )
        return
    raise HelperError("malformed", f"question {qid!r}: unknown qtype {qtype!r}")


def decode_question(question: dict, logits: list[float]) -> dict:
    """Decode one raw logits vector into the normalized per-question output.

    No confidence is fabricated: Julia's documented runtime exposes no distinct
    confidence value, so ``confidence`` is omitted.
    """
    validate_question(question)
    options = question["options"]
    qtype = question["qtype"]
    validate_logits(logits, len(options))
    probabilities = softmax(logits)

    if qtype == QTYPE_CHOICE:
        labels = {options[i]: probabilities[i] for i in range(len(options))}
        return {"choice": options[argmax(probabilities)], "probabilities": labels}
    if qtype == QTYPE_SCORE:
        return {"score": expected_index(probabilities)}
    if qtype == QTYPE_BOOLEAN:
        # option 0 is false, option 1 is true -> P(true)
        return {"probability": probabilities[1]}
    raise HelperError("malformed", f"unknown qtype {qtype!r}")


def find_marker_positions(ids: list[int], option_spans: list[list[int]]) -> tuple[list[int], list[bool]]:
    """Locate each option's token span within the full encoding.

    Returns ``(marker_pos, marker_mask)``: for each option, the index of the
    first token of its span, and whether the span was found. Missing spans are
    masked out (``marker_mask`` False) and given position -1.

    This is the isolated, replaceable point for upstream-verified encoding.
    """
    marker_pos: list[int] = []
    marker_mask: list[bool] = []
    for span in option_spans:
        position = _find_subsequence(ids, span)
        if position < 0:
            marker_pos.append(-1)
            marker_mask.append(False)
        else:
            marker_pos.append(position)
            marker_mask.append(True)
    return marker_pos, marker_mask


def _find_subsequence(haystack: list[int], needle: list[int]) -> int:
    if not needle or len(needle) > len(haystack):
        return -1
    for start in range(0, len(haystack) - len(needle) + 1):
        if haystack[start : start + len(needle)] == needle:
            return start
    return -1


# --------------------------------------------------------------------------- #
# Runtime (heavy deps imported lazily so pure tests need neither numpy nor ORT) #
# --------------------------------------------------------------------------- #


class Runtime:
    """Loaded tokenizer + ONNX session, plus the encoder."""

    def __init__(self, model_path: str, tokenizer_path: str, max_tokens: int) -> None:
        self.max_tokens = max_tokens
        try:
            import numpy as np
            import onnxruntime as ort
            from tokenizers import Tokenizer
        except ImportError as exc:  # pragma: no cover - environment dependent
            raise HelperError(
                "unavailable",
                f"inference dependencies are not installed ({exc}); run: "
                "pip install -r tools/julia/requirements.txt",
            )
        self._np = np
        self.session = ort.InferenceSession(model_path, providers=["CPUExecutionProvider"])
        self.tokenizer = Tokenizer.from_file(tokenizer_path)

    def encode(self, state: str, question: dict) -> dict:
        """Encode one question into Julia's ONNX input tensors."""
        options = question["options"]
        prompt = render_prompt(state, question["text"], options)
        encoding = self.tokenizer.encode(prompt)
        ids = list(encoding.ids)

        spans = [list(self.tokenizer.encode(f"- {option}").ids) for option in options]
        marker_pos, marker_mask = find_marker_positions(ids, spans)

        if len(ids) > self.max_tokens:
            ids = ids[: self.max_tokens]
        attention = [1] * len(ids)
        pad = self.max_tokens - len(ids)
        if pad > 0:
            ids = ids + [0] * pad
            attention = attention + [0] * pad

        clipped_pos = [min(max(p, 0), len(ids) - 1) for p in marker_pos]

        np = self._np
        return {
            "input_ids": np.array([ids], dtype=np.int64),
            "attention_mask": np.array([attention], dtype=np.int64),
            "marker_pos": np.array([clipped_pos], dtype=np.int64),
            "marker_mask": np.array([marker_mask], dtype=bool),
            "qtype": np.array([question["qtype"]], dtype=np.int64),
        }

    def logits(self, tensors: dict) -> list[float]:
        """Run ONNX inference and return the flat logits vector."""
        outputs = self.session.run(
            [GRAPH_LOGITS_NAME], {name: tensors[name] for name in GRAPH_INPUT_NAMES}
        )
        array = self._np.asarray(outputs[0])
        if array.ndim != 2 or array.shape[0] != 1:
            raise HelperError("malformed", f"unexpected logits shape {array.shape}")
        return [float(x) for x in array[0]]


# --------------------------------------------------------------------------- #
# Entry point                                                                  #
# --------------------------------------------------------------------------- #


def run(request: dict, runtime: Runtime) -> dict:
    if not isinstance(request, dict):
        raise HelperError("malformed", "request must be a JSON object")
    state = request.get("state")
    questions = request.get("questions")
    if not isinstance(state, str) or not state.strip():
        raise HelperError("malformed", "request is missing a non-empty state")
    if not isinstance(questions, list) or not questions:
        raise HelperError("malformed", "request must contain at least one question")

    answers: dict = {}
    for question in questions:
        validate_question(question)
        tensors = runtime.encode(state, question)
        logits = runtime.logits(tensors)
        answers[question["id"]] = decode_question(question, logits)

    return {"model": os.environ.get("JULIA_MODEL_ID", DEFAULT_MODEL_ID), "answers": answers}


def build_runtime() -> Runtime:
    model_path = os.environ.get("JULIA_MODEL_PATH", "").strip()
    if not model_path:
        raise HelperError("unavailable", "JULIA_MODEL_PATH is not set")
    if not os.path.isfile(model_path):
        raise HelperError("unavailable", f"model not found at {model_path}")
    tokenizer_path = os.environ.get("JULIA_TOKENIZER_PATH", "").strip()
    if not tokenizer_path:
        tokenizer_path = os.path.join(os.path.dirname(os.path.abspath(model_path)), "tokenizer.json")
    if not os.path.isfile(tokenizer_path):
        raise HelperError("unavailable", f"tokenizer not found at {tokenizer_path}")
    max_tokens = int(os.environ.get("JULIA_MAX_TOKENS", DEFAULT_MAX_TOKENS))
    return Runtime(model_path, tokenizer_path, max_tokens)


def emit(payload: dict) -> None:
    json.dump(payload, sys.stdout, separators=(",", ":"))
    sys.stdout.write("\n")
    sys.stdout.flush()


def emit_error(kind: str, message: str) -> None:
    emit({"error": {"kind": kind, "message": message}})


def main(argv: list[str] | None = None) -> int:
    try:
        raw = sys.stdin.read()
    except OSError as exc:
        emit_error("failure", f"could not read stdin: {exc}")
        return 1
    try:
        request = json.loads(raw)
    except json.JSONDecodeError as exc:
        emit_error("malformed", f"request is not valid JSON: {exc}")
        return 2
    try:
        runtime = build_runtime()
        result = run(request, runtime)
    except HelperError as exc:
        print(f"julia-infer: {exc.kind}: {exc.message}", file=sys.stderr)
        emit_error(exc.kind, exc.message)
        return 3 if exc.kind == "unavailable" else (2 if exc.kind == "malformed" else 1)
    except Exception as exc:  # pragma: no cover - unexpected runtime failure
        print(f"julia-infer: failure: {exc}", file=sys.stderr)
        emit_error("failure", str(exc))
        return 1
    emit(result)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
