#!/usr/bin/env python3
"""HTTP bridge for the Julia adapter's CommandRunner seam.

This is an operator-supplied implementation of the *existing* CommandRunner
JSON stdin/stdout contract (see ``internal/providers/julia/runner.go`` and
``tools/julia/README.md``). It is **not** a new provider-neutral API and it
introduces no new provider surface: the Go adapter still speaks only
``Inputs``/``Outputs``.

Where ``infer.py`` runs Julia-1 ONNX in-process, this bridge forwards each
question to an external Julia HTTP service and normalizes its response back into
the runner ``Outputs`` shape:

    decision.DecisionRequest
        -> Julia adapter (BuildInputs)          Go
        -> Julia runtime request (JSON stdin)
        -> http_bridge.py  --HTTP-->  Julia service (JULIA_URL)
        -> Outputs (JSON stdout)
        -> Julia adapter (NormalizeOutputs)     Go
        -> decision.DecisionResult

Contract:

* stdin  : one JSON object ``{"state": str, "questions": [QuestionInput, ...]}``
* stdout : on success ``{"model": str, "answers": {id: QuestionOutput}}``
           on failure ``{"error": {"kind": "unavailable|malformed|failure",
                                   "message": str}}`` and a non-zero exit.
* stderr : human-readable diagnostics only. stdout carries exactly one JSON
  object.

Question-type translation (provider-neutral -> Julia-native):

* ``choice``  -> ``choice`` : ``{index, probabilities}`` -> ``{choice, probabilities}``
* ``boolean`` -> ``noul``   : fixed options ``["false", "true"]``;
                              ``{index, probabilities}`` -> ``{probability: P(true)}``
* ``score``   -> ``score``  : ``{index, probabilities}`` -> ``{score: expected index}``

SCORE is Julia's documented **expected zero-based option index** ``sum(i * p_i)``
(the same rule as ``infer.py``'s ``expected_index``); it is **not** the argmax
index and **not** the selected option's numeric value. The external runtime must
therefore return per-option ``probabilities`` for a ``score`` question; if it
does not, the bridge reports a ``malformed`` error rather than fabricating a
score. ``noul`` (and every other Julia-native term) is confined to this file.
"""

from __future__ import annotations

import json
import math
import os
import sys
import urllib.error
import urllib.request

# --- Configuration ----------------------------------------------------------
#
# Read at import so tests can point the bridge at an ephemeral mock server by
# assigning ``http_bridge.JULIA_URL``.

DEFAULT_URL = "http://127.0.0.1:8011"
DEFAULT_MODEL = "SupersonicLabs/Julia-1"
DEFAULT_TIMEOUT = 30.0

JULIA_URL = os.environ.get("JULIA_URL", DEFAULT_URL).strip() or DEFAULT_URL
JULIA_URL = JULIA_URL.rstrip("/")
MODEL = (
    os.environ.get("JULIA_MODEL_ID", "").strip()
    or os.environ.get("JULIA_MODEL", "").strip()
    or DEFAULT_MODEL
)


def _timeout_from_env(name: str, default: float) -> float:
    raw = os.environ.get(name, "").strip()
    if not raw:
        return default
    try:
        value = float(raw)
    except ValueError:
        return default
    return value if value > 0 else default


TIMEOUT = _timeout_from_env("JULIA_HTTP_TIMEOUT", DEFAULT_TIMEOUT)

# --- Julia-native vocabulary (confined to this boundary) --------------------

# Provider-neutral question type -> Julia-native qtype token for the HTTP
# service. Anything else is rejected as malformed rather than guessed.
RUNTIME_TYPES = {
    "choice": "choice",
    "boolean": "noul",
    "score": "score",
}

# Julia's fixed noul options: index 0 is false and index 1 is true, so the
# normalized boolean probability is P(true) = probabilities[1].
BOOLEAN_OPTIONS = ["false", "true"]


class BridgeError(Exception):
    """A classified bridge failure carried as the runner error envelope.

    ``kind`` is one of ``unavailable`` (transport/runtime unreachable),
    ``malformed`` (unusable input or provider response), or ``failure`` (the
    provider returned an error for a well-formed request).
    """

    def __init__(self, kind: str, message: str) -> None:
        super().__init__(message)
        self.kind = kind
        self.message = message


def error_envelope(kind: str, message: str) -> dict:
    """Return the machine-readable runner error envelope for stdout."""
    return {"error": {"kind": kind, "message": message}}


# --- Input parsing / validation --------------------------------------------


def runtime_options(question: dict) -> list:
    """Return the ordered options to send to the Julia runtime.

    BOOLEAN (noul) always uses Julia's fixed ``["false", "true"]`` options; any
    supplied options are ignored so the noul convention cannot drift. CHOICE and
    SCORE require a non-empty list of string options.
    """
    qtype = question.get("type")
    if qtype == "boolean":
        return list(BOOLEAN_OPTIONS)

    options = question.get("options")
    if not isinstance(options, list) or not options:
        raise BridgeError("malformed", "question options must be a non-empty array")
    for option in options:
        if not isinstance(option, str) or option.strip() == "":
            raise BridgeError(
                "malformed", f"question option {option!r} must be a non-empty string"
            )
    return list(options)


def question_type(question: dict) -> str:
    """Return a supported provider-neutral question type or raise malformed."""
    qtype = question.get("type")
    if qtype not in RUNTIME_TYPES:
        raise BridgeError("malformed", f"unsupported question type: {qtype!r}")
    return qtype


# --- HTTP transport ---------------------------------------------------------


def predict(state: str, question: dict, qtype: str) -> dict:
    """POST one question to the Julia HTTP service and return its JSON response."""
    text = question.get("text")
    if not isinstance(text, str) or text.strip() == "":
        raise BridgeError("malformed", "question text is required")

    payload = {
        "state": state,
        "question": text,
        "options": runtime_options(question),
        "type": RUNTIME_TYPES[qtype],
    }

    request = urllib.request.Request(
        JULIA_URL + "/predict",
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )

    try:
        with urllib.request.urlopen(request, timeout=TIMEOUT) as response:
            body = response.read()
    except urllib.error.HTTPError as exc:
        # HTTPError is a URLError subclass: it must be handled first. The
        # service answered, so this is a provider failure, not a transport one.
        detail = exc.read().decode("utf-8", errors="replace").strip()
        raise BridgeError("failure", f"Julia HTTP {exc.code}: {detail}") from exc
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        raise BridgeError("unavailable", f"Julia service unavailable: {exc}") from exc

    try:
        return json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, ValueError) as exc:
        raise BridgeError(
            "malformed", f"Julia response is not valid JSON: {exc}"
        ) from exc


# --- Response normalization -------------------------------------------------


def require_number_list(values, count: int, what: str) -> list:
    """Validate a probability vector: a list of ``count`` finite values in [0, 1]."""
    if not isinstance(values, list):
        raise BridgeError("malformed", f"Julia {what} must be an array")
    if len(values) != count:
        raise BridgeError(
            "malformed",
            f"Julia {what} has {len(values)} entries for {count} options",
        )
    out = []
    for i, value in enumerate(values):
        # bool is a subclass of int: reject it explicitly.
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise BridgeError(
                "malformed", f"Julia {what}[{i}] is not a number: {value!r}"
            )
        number = float(value)
        if not math.isfinite(number):
            raise BridgeError(
                "malformed", f"Julia {what}[{i}] is not finite: {value!r}"
            )
        if number < 0.0 or number > 1.0:
            raise BridgeError(
                "malformed", f"Julia {what}[{i}] is out of range [0, 1]: {value!r}"
            )
        out.append(number)
    return out


def require_index(result: dict, count: int) -> int:
    """Validate and return the selected option index."""
    if "index" not in result:
        raise BridgeError("malformed", "Julia response is missing index")
    index = result["index"]
    if isinstance(index, bool) or not isinstance(index, int):
        raise BridgeError(
            "malformed", f"Julia response index is not an integer: {index!r}"
        )
    if index < 0 or index >= count:
        raise BridgeError(
            "malformed",
            f"Julia returned invalid option index {index} for {count} options",
        )
    return index


def expected_index(probabilities: list) -> float:
    """Julia SCORE semantics: expected zero-based option index ``sum(i * p_i)``.

    SCORE is deliberately *not* ``argmax``: a uniform distribution over three
    options has expected index 1.0 even though ``argmax`` would be 0. This
    matches ``tools/julia/infer.py``'s ``expected_index``.
    """
    return sum(i * p for i, p in enumerate(probabilities))


def normalize(question: dict, qtype: str, result) -> dict:
    """Normalize one Julia response into the runner ``QuestionOutput`` shape."""
    if not isinstance(result, dict):
        raise BridgeError(
            "malformed", "Julia response for a question must be an object"
        )

    options = runtime_options(question)

    if qtype == "choice":
        index = require_index(result, len(options))
        answer = {"choice": options[index]}
        probabilities = result.get("probabilities")
        if probabilities is not None:
            probs = require_number_list(probabilities, len(options), "probabilities")
            answer["probabilities"] = dict(zip(options, probs))
        return answer

    if qtype == "boolean":
        probabilities = result.get("probabilities")
        if probabilities is None:
            raise BridgeError(
                "malformed", "Julia boolean response requires probabilities"
            )
        probs = require_number_list(probabilities, len(options), "probabilities")
        if "index" in result:
            require_index(result, len(options))
        return {"probability": probs[1]}

    if qtype == "score":
        probabilities = result.get("probabilities")
        if probabilities is None:
            raise BridgeError(
                "malformed",
                "Julia score response requires probabilities to compute the expected option index",
            )
        probs = require_number_list(probabilities, len(options), "probabilities")
        return {"score": expected_index(probs)}

    raise BridgeError("malformed", f"unsupported question type: {qtype!r}")


# --- Entry point ------------------------------------------------------------


def build_output(request) -> dict:
    """Run every question in a bridge request and return the runner ``Outputs``."""
    if not isinstance(request, dict):
        raise BridgeError("malformed", "bridge input must be a JSON object")

    state = request.get("state", "")
    if not isinstance(state, str):
        raise BridgeError("malformed", "state must be a string")

    questions = request.get("questions")
    if not isinstance(questions, list):
        raise BridgeError("malformed", "questions must be an array")

    answers = {}
    seen = set()
    for question in questions:
        if not isinstance(question, dict):
            raise BridgeError("malformed", "each question must be a JSON object")
        qid = question.get("id")
        if not isinstance(qid, str) or qid.strip() == "":
            raise BridgeError("malformed", "question is missing id")
        if qid in seen:
            # The provider-neutral contract already prohibits duplicate ids; the
            # bridge refuses them too, so a duplicate never silently overwrites an
            # earlier answer at the runner boundary.
            raise BridgeError("malformed", f"duplicate question id: {qid!r}")
        seen.add(qid)
        qtype = question_type(question)
        result = predict(state, question, qtype)
        answers[qid] = normalize(question, qtype, result)

    return {"model": MODEL, "answers": answers}


def emit_error(kind: str, message: str) -> int:
    """Write the machine-readable error envelope to stdout and a diagnostic to stderr."""
    json.dump(error_envelope(kind, message), sys.stdout)
    sys.stdout.write("\n")
    print(f"julia-http-bridge: {kind}: {message}", file=sys.stderr)
    return 1


def main() -> int:
    try:
        request = json.load(sys.stdin)
    except Exception as exc:  # noqa: BLE001 - any stdin failure is malformed input
        return emit_error("malformed", f"invalid input JSON: {exc}")

    try:
        output = build_output(request)
    except BridgeError as exc:
        return emit_error(exc.kind, exc.message)

    json.dump(output, sys.stdout)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
