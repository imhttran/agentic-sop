#!/usr/bin/env python3
"""Routing calibration runner for the Julia decision provider.

Evaluates a labeled dataset of software-engineering task descriptions against a
running Julia service, using ``http_bridge.py`` (the provider's real translation
layer) so the measured behaviour is exactly what the adapter would see. It does
NOT tune the model or define policy; it only measures agreement between the
model's selected execution class and the dataset's expected class.

Requires a live Julia service (``JULIA_URL``, default ``http://127.0.0.1:8011``).
It is NOT part of the default offline test suite.

Usage:

    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/calibrate_routing.py \
        tests/fixtures/julia/routing-calibration.json --out /tmp/calibration.json

The machine-readable results are written to stdout (or ``--out PATH``); a short
human summary goes to stderr.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402

HIGH_CONFIDENCE = 0.80

# Descriptive option wording (Phase 10 Variant B/C). Kept here so the variants are
# explicit and reproducible.
DESCRIPTIVE = {
    "LOW": "LOW — simple, localized, low-risk work requiring little reasoning",
    "MEDIUM": "MEDIUM — moderately complex implementation requiring reasoning across multiple functions or files without major architectural consequences",
    "HIGH": "HIGH — complex or consequential work involving architecture, security, cross-system behavior, high ambiguity, or significant impact",
}

CURRENT_QUESTION = "Select the execution model for the described task."
CAPABILITY_QUESTION = (
    "Determine the execution capability required to complete the described task."
)

VARIANTS = [
    {
        "name": "A_bare_choices_current_question",
        "question": CURRENT_QUESTION,
        "options": ["LOW", "MEDIUM", "HIGH"],
    },
    {
        "name": "B_descriptive_choices_current_question",
        "question": CURRENT_QUESTION,
        "options": [DESCRIPTIVE["LOW"], DESCRIPTIVE["MEDIUM"], DESCRIPTIVE["HIGH"]],
    },
    {
        "name": "C_descriptive_choices_capability_question",
        "question": CAPABILITY_QUESTION,
        "options": [DESCRIPTIVE["LOW"], DESCRIPTIVE["MEDIUM"], DESCRIPTIVE["HIGH"]],
    },
]

CLASSES = ["LOW", "MEDIUM", "HIGH"]


def class_of(option: str) -> str | None:
    """Map a supplied option label to its execution class (LOW/MEDIUM/HIGH)."""
    token = option.strip().split(" ", 1)[0].rstrip("—:-").strip().upper()
    return token if token in CLASSES else None


def evaluate_case(variant: dict, case: dict) -> dict:
    request = {
        "state": case["state"],
        "questions": [
            {
                "id": "execution_model",
                "type": "choice",
                "text": variant["question"],
                "options": variant["options"],
            }
        ],
    }
    out = http_bridge.build_output(request)
    answer = out["answers"]["execution_model"]
    probabilities = answer.get("probabilities", {})

    by_class: dict[str, float] = {}
    for option, probability in probabilities.items():
        cls = class_of(option)
        if cls is not None:
            by_class[cls] = probability

    selected = class_of(answer["choice"])
    selected_probability = by_class.get(selected)
    others = [p for c, p in by_class.items() if c != selected]
    margin = (
        None
        if selected_probability is None
        else selected_probability - max(others, default=0.0)
    )

    return {
        "id": case["id"],
        "expected": case["expected"],
        "selected": selected,
        "correct": selected == case["expected"],
        "probabilities": {c: by_class.get(c) for c in CLASSES},
        "selected_probability": selected_probability,
        "margin": margin,
        "high_confidence_misclassification": (
            selected != case["expected"]
            and selected_probability is not None
            and selected_probability >= HIGH_CONFIDENCE
        ),
    }


def metrics(rows: list[dict]) -> dict:
    total = len(rows)
    correct = sum(1 for r in rows if r["correct"])

    confusion = {expected: {pred: 0 for pred in CLASSES} for expected in CLASSES}
    for r in rows:
        if r["expected"] in confusion and r["selected"] in confusion[r["expected"]]:
            confusion[r["expected"]][r["selected"]] += 1

    per_class = {}
    for cls in CLASSES:
        of_class = [r for r in rows if r["expected"] == cls]
        right = sum(1 for r in of_class if r["correct"])
        per_class[cls] = {
            "total": len(of_class),
            "correct": right,
            "accuracy": (right / len(of_class)) if of_class else None,
        }

    predicted = {cls: sum(1 for r in rows if r["selected"] == cls) for cls in CLASSES}
    expected = {cls: sum(1 for r in rows if r["expected"] == cls) for cls in CLASSES}

    probs = [
        r["selected_probability"] for r in rows if r["selected_probability"] is not None
    ]
    margins = [r["margin"] for r in rows if r["margin"] is not None]

    high_conf_mis = [r["id"] for r in rows if r["high_confidence_misclassification"]]

    return {
        "total": total,
        "correct": correct,
        "accuracy": (correct / total) if total else None,
        "per_class": per_class,
        "confusion_matrix": confusion,
        "predicted_distribution": predicted,
        "expected_distribution": expected,
        "average_selected_probability": (sum(probs) / len(probs)) if probs else None,
        "average_margin": (sum(margins) / len(margins)) if margins else None,
        "high_confidence_threshold": HIGH_CONFIDENCE,
        "high_confidence_misclassifications": high_conf_mis,
    }


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Run the Julia routing calibration dataset."
    )
    parser.add_argument("dataset", help="path to the labeled dataset JSON")
    parser.add_argument(
        "--out", help="write machine-readable results here instead of stdout"
    )
    args = parser.parse_args()

    with open(args.dataset, "r", encoding="utf-8") as handle:
        dataset = json.load(handle)
    cases = dataset["cases"]

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "dataset": args.dataset,
        "high_confidence_threshold": HIGH_CONFIDENCE,
        "counts": {
            cls: sum(1 for c in cases if c["expected"] == cls) for cls in CLASSES
        },
        "variants": [],
    }

    for variant in VARIANTS:
        rows = [evaluate_case(variant, case) for case in cases]
        m = metrics(rows)
        results["variants"].append(
            {
                "name": variant["name"],
                "question": variant["question"],
                "options": variant["options"],
                "metrics": m,
                "rows": rows,
            }
        )
        print(
            f"variant {variant['name']}: accuracy={m['accuracy']:.3f} "
            f"(LOW={m['per_class']['LOW']['accuracy']}, "
            f"MEDIUM={m['per_class']['MEDIUM']['accuracy']}, "
            f"HIGH={m['per_class']['HIGH']['accuracy']})",
            file=sys.stderr,
        )

    payload = json.dumps(results, indent=2)
    if args.out:
        with open(args.out, "w", encoding="utf-8") as handle:
            handle.write(payload + "\n")
        print(f"wrote {args.out}", file=sys.stderr)
    else:
        print(payload)
    return 0


if __name__ == "__main__":
    sys.exit(main())
