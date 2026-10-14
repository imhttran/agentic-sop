#!/usr/bin/env python3
"""Phase 4: validate the FROZEN escalation question on a large held-out set.

Frozen question: "Does this task require escalation?" (no wording search).
No threshold fitting: it reports fixed, pre-specified operating points (0.5 and
0.6) only. Evaluation only; requires a live Julia service (``JULIA_URL``).

Usage:
    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/escalation_validation.py \
        tests/fixtures/julia/escalation-validation.json --out /tmp/phase4.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402

FROZEN_QUESTION = "Does this task require escalation?"
THRESHOLDS = [0.5, 0.6]  # fixed, pre-specified operating points (no search)


def ask_boolean(state, question):
    request = {
        "state": state,
        "questions": [{"id": "q", "type": "boolean", "text": question}],
    }
    return http_bridge.build_output(request)["answers"]["q"]["probability"]


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


def distribution(rows):
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

    return {
        "escalation": stat([r["p_escalation"] for r in rows if r["escalation"]]),
        "non_escalation": stat(
            [r["p_escalation"] for r in rows if not r["escalation"]]
        ),
    }


def main():
    parser = argparse.ArgumentParser(
        description="Phase 4 frozen escalation-question validation."
    )
    parser.add_argument("dataset", help="held-out escalation dataset JSON")
    parser.add_argument("--out", help="write results here (default stdout)")
    args = parser.parse_args()

    with open(args.dataset, "r", encoding="utf-8") as handle:
        cases = json.load(handle)["cases"]

    rows = []
    for case in cases:
        rows.append(
            {
                "id": case["id"],
                "escalation": case["escalation"],
                "p_escalation": ask_boolean(case["state"], FROZEN_QUESTION),
            }
        )

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "dataset": args.dataset,
        "frozen_question": FROZEN_QUESTION,
        "counts": {
            "escalation": sum(1 for c in cases if c["escalation"]),
            "non_escalation": sum(1 for c in cases if not c["escalation"]),
        },
        "distribution": distribution(rows),
        "operating_points": {str(t): metrics(rows, t) for t in THRESHOLDS},
        "rows": rows,
    }

    for t in THRESHOLDS:
        m = results["operating_points"][str(t)]
        print(
            f"T={t}: acc={m['accuracy']:.3f} prec={m['precision']:.3f} rec={m['recall']:.3f} "
            f"spec={m['specificity']:.3f} f1={m['f1']:.3f} balAcc={m['balanced_accuracy']:.3f}",
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
