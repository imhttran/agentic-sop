#!/usr/bin/env python3
"""Focused escalation-signal study (Phase 3 follow-up).

Characterizes whether a single binary governance question is a usable
"HIGH/escalation vs rest" signal, on a larger newly authored set with a
calibration/validation split and several fixed prompt variants.

Evaluation only. Requires a live Julia service (``JULIA_URL``).

Usage:
    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/escalation_probe.py \
        tests/fixtures/julia/escalation-signal.json --out /tmp/esc.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402

QUESTIONS = {
    "Q1_escalation": {
        "text": "Does this task require escalation?",
        "true_means": "escalation",
    },
    "Q2_highest_tier": {
        "text": "Should this task be escalated to the highest reasoning and execution tier?",
        "true_means": "escalation",
    },
    "Q3_consequence_list": {
        "text": (
            "Does this task involve architecture, security, authorization, cross-system behavior, "
            "destructive operations, or significant operational consequences?"
        ),
        "true_means": "escalation",
    },
    "Q4_routine_inverted": {
        "text": "Is this task routine, localized work that does not require escalation?",
        "true_means": "routine",
    },
}


def ask_boolean(state, question):
    request = {
        "state": state,
        "questions": [{"id": "q", "type": "boolean", "text": question}],
    }
    return http_bridge.build_output(request)["answers"]["q"]["probability"]


def collect(cases, question, true_means):
    rows = []
    for case in cases:
        p_true = ask_boolean(case["state"], question)
        p_esc = p_true if true_means == "escalation" else 1.0 - p_true
        rows.append(
            {
                "id": case["id"],
                "escalation": case["escalation"],
                "p_escalation": p_esc,
                "p_true": p_true,
            }
        )
    return rows


def metrics(rows, threshold):
    tp = fp = tn = fn = 0
    for r in rows:
        pred = r["p_escalation"] >= threshold
        if r["escalation"] and pred:
            tp += 1
        elif r["escalation"]:
            fn += 1
        elif pred:
            fp += 1
        else:
            tn += 1
    total = tp + fp + tn + fn
    precision = tp / (tp + fp) if (tp + fp) else None
    recall = tp / (tp + fn) if (tp + fn) else None
    specificity = tn / (tn + fp) if (tn + fp) else None
    f1 = (
        (2 * precision * recall / (precision + recall))
        if precision and recall
        else None
    )
    balanced = ((recall or 0.0) + (specificity or 0.0)) / 2
    return {
        "threshold": threshold,
        "tp": tp,
        "fp": fp,
        "tn": tn,
        "fn": fn,
        "accuracy": (tp + tn) / total if total else None,
        "precision": precision,
        "recall": recall,
        "specificity": specificity,
        "f1": f1,
        "balanced_accuracy": balanced,
    }


def best_threshold(rows, grid_step=0.01):
    best = None
    for i in range(int(1.0 / grid_step) + 1):
        t = i * grid_step
        m = metrics(rows, t)
        key = round(m["balanced_accuracy"], 6)
        if best is None or key > best[0]:
            best = (key, t, m)
    return best[1], best[2]


def distribution(rows):
    esc = [r["p_escalation"] for r in rows if r["escalation"]]
    non = [r["p_escalation"] for r in rows if not r["escalation"]]

    def stat(v):
        if not v:
            return None
        v = sorted(v)
        n = len(v)
        return {
            "n": n,
            "mean": sum(v) / n,
            "median": v[n // 2],
            "min": v[0],
            "max": v[-1],
            "q1": v[min(n - 1, int(0.25 * n))],
            "q3": v[min(n - 1, int(0.75 * n))],
        }

    return {"escalation": stat(esc), "non_escalation": stat(non)}


def sweep(rows, thresholds):
    return [
        {
            "threshold": t,
            **{
                k: m[k]
                for k in (
                    "precision",
                    "recall",
                    "specificity",
                    "f1",
                    "balanced_accuracy",
                    "accuracy",
                )
            },
        }
        for t in thresholds
        for m in [metrics(rows, t)]
    ]


def main():
    parser = argparse.ArgumentParser(description="Escalation-signal study.")
    parser.add_argument("dataset", help="escalation dataset JSON")
    parser.add_argument("--out", help="write results here (default stdout)")
    args = parser.parse_args()

    with open(args.dataset, "r", encoding="utf-8") as handle:
        cases = json.load(handle)["cases"]
    cal = [c for c in cases if c["split"] == "calibration"]
    val = [c for c in cases if c["split"] == "validation"]

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "dataset": args.dataset,
        "counts": {
            "calibration": {
                "escalation": sum(1 for c in cal if c["escalation"]),
                "non": sum(1 for c in cal if not c["escalation"]),
            },
            "validation": {
                "escalation": sum(1 for c in val if c["escalation"]),
                "non": sum(1 for c in val if not c["escalation"]),
            },
        },
        "questions": {},
    }

    for name, spec in QUESTIONS.items():
        print(f"{name} ...", file=sys.stderr)
        cal_rows = collect(cal, spec["text"], spec["true_means"])
        val_rows = collect(val, spec["text"], spec["true_means"])
        t, cal_best = best_threshold(cal_rows)
        results["questions"][name] = {
            "text": spec["text"],
            "true_means": spec["true_means"],
            "calibration_at_0.5": metrics(cal_rows, 0.5),
            "calibration_best": cal_best,
            "validation_at_0.5": metrics(val_rows, 0.5),
            "validation_at_calibration_threshold": metrics(val_rows, t),
            "validation_threshold_used": t,
            "calibration_distribution": distribution(cal_rows),
            "validation_distribution": distribution(val_rows),
        }

    # Threshold sensitivity for the primary question on both splits.
    q1 = QUESTIONS["Q1_escalation"]
    cal_rows = collect(cal, q1["text"], q1["true_means"])
    val_rows = collect(val, q1["text"], q1["true_means"])
    grid = [0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9]
    results["q1_threshold_sensitivity"] = {
        "calibration": sweep(cal_rows, grid),
        "validation": sweep(val_rows, grid),
    }

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
