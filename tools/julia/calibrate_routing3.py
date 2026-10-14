#!/usr/bin/env python3
"""Phase 3: binary, dual-boundary, margin, hierarchical and governance evaluation.

Asks Julia's native boolean (NOUL) capability a fixed set of binary questions and
measures whether the resulting P(true) signal builds a useful LOW/MEDIUM/HIGH
router (hierarchies H1/H2, dual-boundary, margin band) and whether narrower
governance questions separate better.

Evaluation only: no production policy, no provider/contract change, no threshold
in the bridge. Requires a live Julia service (``JULIA_URL``).

Usage:
    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/calibrate_routing3.py \
        tests/fixtures/julia/routing-calibration.json --out /tmp/phase3.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402

CLASSES = ["LOW", "MEDIUM", "HIGH"]

# Phase 2/3 primary binaries.
BINARIES = {
    "A_low": {
        "question": "Is this task simple, localized, low-risk, and suitable for the LOW execution class?",
        "true_when": "LOW",
    },
    "B_high": {
        "question": "Is this task complex or consequential enough to require the HIGH execution class?",
        "true_when": "HIGH",
    },
    "C_escalation": {
        "question": (
            "Does this task involve architecture, security, authorization, cross-system behavior, "
            "destructive operations, high ambiguity, or consequences significant enough to warrant "
            "escalation to the highest reasoning/execution tier?"
        ),
        "true_when": "HIGH",
    },
}

# Phase 11 prompt variants (fixed; used unchanged across the dataset).
PROMPT_VARIANTS = {
    "LOW_A": {
        "question": "Is this task appropriate for the LOW execution class?",
        "true_when": "LOW",
    },
    "LOW_B": {
        "question": (
            "Is this a simple, localized, low-risk task requiring little reasoning and no "
            "architectural, security, cross-system, or consequential change?"
        ),
        "true_when": "LOW",
    },
    "HIGH_A": {
        "question": "Does this task require the HIGH execution class?",
        "true_when": "HIGH",
    },
    "HIGH_B": {
        "question": (
            "Does this task involve architecture, security, authorization, cross-system behavior, "
            "destructive operations, high ambiguity, or consequential changes requiring the highest "
            "reasoning/execution tier?"
        ),
        "true_when": "HIGH",
    },
}

# Phase 19 governance probe (exploratory; ground truth = HIGH-vs-rest separation).
GOVERNANCE = {
    "G1_escalation": "Does this task require escalation?",
    "G2_architecture": "Does this task cross an architectural boundary?",
    "G3_auth_security": "Does this task modify authorization or security behavior?",
    "G4_consequences": "Could an incorrect implementation have significant operational consequences?",
}


def ask_boolean(state, question):
    """Return P(true) from Julia's native boolean (NOUL) capability."""
    request = {
        "state": state,
        "questions": [{"id": "binary", "type": "boolean", "text": question}],
    }
    return http_bridge.build_output(request)["answers"]["binary"]["probability"]


def collect(cases, question, true_when):
    rows = []
    for case in cases:
        p_true = ask_boolean(case["state"], question)
        rows.append(
            {
                "id": case["id"],
                "expected": case["expected"],
                "expected_bool": case["expected"] == true_when,
                "p_true": p_true,
                "pred": p_true >= 0.5,
                "correct": (p_true >= 0.5) == (case["expected"] == true_when),
                "margin": abs(p_true - 0.5),
            }
        )
    return rows


def stats(values):
    if not values:
        return None
    vs = sorted(values)
    n = len(vs)
    mean = sum(vs) / n
    var = sum((v - mean) ** 2 for v in vs) / n

    def q(frac):
        return vs[min(n - 1, int(frac * n))]

    return {
        "n": n,
        "mean": mean,
        "median": vs[n // 2],
        "stddev": var**0.5,
        "min": vs[0],
        "max": vs[-1],
        "q1": q(0.25),
        "q3": q(0.75),
    }


def binary_metrics(rows, threshold):
    tp = fp = tn = fn = 0
    for r in rows:
        pred = r["p_true"] >= threshold
        if r["expected_bool"] and pred:
            tp += 1
        elif r["expected_bool"]:
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
        "predicted_true": tp + fp,
        "predicted_false": tn + fn,
    }


def ptrue_by_class(rows):
    return {
        cls: stats([r["p_true"] for r in rows if r["expected"] == cls])
        for cls in CLASSES
    }


def threshold_search(rows, grid_step=0.01):
    best = None
    for i in range(int(1.0 / grid_step) + 1):
        t = i * grid_step
        m = binary_metrics(rows, t)
        key = round(m["balanced_accuracy"] or 0.0, 6)
        if best is None or key > best[0]:
            best = (key, t, m)
    return {"best_threshold": best[1], "metrics": best[2]}


def summarize_classes(rows):
    confusion = {e: {p: 0 for p in CLASSES} for e in CLASSES}
    for r in rows:
        confusion[r["expected"]][r["predicted"]] += 1
    per_class = {}
    for cls in CLASSES:
        of = [r for r in rows if r["expected"] == cls]
        right = sum(1 for r in of if r["predicted"] == cls)
        per_class[cls] = {"total": len(of), "recall": (right / len(of)) if of else None}
    correct = sum(1 for r in rows if r["predicted"] == r["expected"])
    recalls = [per_class[c]["recall"] for c in CLASSES]
    return {
        "accuracy": correct / len(rows) if rows else None,
        "macro_recall": sum(recalls) / len(recalls),
        "per_class": per_class,
        "confusion_matrix": confusion,
        "predicted_distribution": {
            c: sum(1 for r in rows if r["predicted"] == c) for c in CLASSES
        },
    }


def hierarchy(cases, p_low, p_high, low_t, high_t, first):
    rows = []
    for case in cases:
        cid = case["id"]
        low_true = p_low[cid] >= low_t
        high_true = p_high[cid] >= high_t
        if first == "LOW":
            predicted = "LOW" if low_true else ("HIGH" if high_true else "MEDIUM")
        else:
            predicted = "HIGH" if high_true else ("LOW" if low_true else "MEDIUM")
        rows.append({"id": cid, "expected": case["expected"], "predicted": predicted})
    return rows


def dual_boundary(cases, p_low, p_high, low_t, high_t):
    buckets = {"low_only": [], "high_only": [], "neither": [], "both": []}
    for case in cases:
        cid = case["id"]
        lt = p_low[cid] >= low_t
        ht = p_high[cid] >= high_t
        key = (
            "both"
            if (lt and ht)
            else ("low_only" if lt else ("high_only" if ht else "neither"))
        )
        buckets[key].append(case)

    def accuracy(resolve):
        rows = []
        for case in cases:
            cid = case["id"]
            lt = p_low[cid] >= low_t
            ht = p_high[cid] >= high_t
            if lt and ht:
                pred = "LOW" if resolve == "low_first" else "HIGH"
            elif lt:
                pred = "LOW"
            elif ht:
                pred = "HIGH"
            else:
                pred = "MEDIUM"
            rows.append({"id": cid, "expected": case["expected"], "predicted": pred})
        return summarize_classes(rows)

    conflict = len(buckets["both"])
    non_conflict = [
        c for c in cases if c["id"] not in {x["id"] for x in buckets["both"]}
    ]
    excluded_rows = []
    for case in non_conflict:
        cid = case["id"]
        pred = (
            "LOW"
            if p_low[cid] >= low_t
            else ("HIGH" if p_high[cid] >= high_t else "MEDIUM")
        )
        excluded_rows.append(
            {"id": cid, "expected": case["expected"], "predicted": pred}
        )

    return {
        "thresholds": {"low": low_t, "high": high_t},
        "counts": {k: len(v) for k, v in buckets.items()},
        "expected_by_bucket": {
            k: {c: sum(1 for x in v if x["expected"] == c) for c in CLASSES}
            for k, v in buckets.items()
        },
        "conflict_count": conflict,
        "conflict_percentage": conflict / len(cases) if cases else None,
        "accuracy_excluding_conflicts": summarize_classes(excluded_rows),
        "accuracy_conflict_low_first": accuracy("low_first"),
        "accuracy_conflict_high_first": accuracy("high_first"),
    }


def margin_analysis(cases, p_low, p_high):
    margins = {c["id"]: p_low[c["id"]] - p_high[c["id"]] for c in cases}
    by_class = {
        cls: stats([margins[c["id"]] for c in cases if c["expected"] == cls])
        for cls in CLASSES
    }

    best = None
    for i in range(0, 101):
        t = i / 100.0
        rows = []
        for case in cases:
            m = margins[case["id"]]
            pred = "LOW" if m >= t else ("HIGH" if m <= -t else "MEDIUM")
            rows.append(
                {"id": case["id"], "expected": case["expected"], "predicted": pred}
            )
        summary = summarize_classes(rows)
        key = round(summary["macro_recall"], 6)
        if best is None or key > best[0]:
            best = (key, t, summary, rows)
    return {
        "margin_definition": "P(LOW) - P(HIGH)",
        "by_class": by_class,
        "band_search": {"best_band_T": best[1], "metrics": best[2]},
    }


def evaluate_question(cases, question, true_when):
    rows = collect(cases, question, true_when)
    return {
        "question": question,
        "true_when": true_when,
        "metrics_at_0.5": binary_metrics(rows, 0.5),
        "threshold_search": threshold_search(rows),
        "p_true_by_class": ptrue_by_class(rows),
        "rows": rows,
    }


def main():
    parser = argparse.ArgumentParser(
        description="Phase 3 binary/hierarchical routing evaluation."
    )
    parser.add_argument("calibration", help="calibration dataset JSON")
    parser.add_argument("--out", help="write results here (default stdout)")
    args = parser.parse_args()

    with open(args.calibration, "r", encoding="utf-8") as handle:
        cases = json.load(handle)["cases"]

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "dataset": args.calibration,
        "counts": {c: sum(1 for x in cases if x["expected"] == c) for c in CLASSES},
        "binaries": {},
        "prompt_variants": {},
        "governance": {},
        "hierarchies": {},
    }

    raw = {}
    for name, spec in BINARIES.items():
        print(f"binary {name} ...", file=sys.stderr)
        results["binaries"][name] = evaluate_question(
            cases, spec["question"], spec["true_when"]
        )
        raw[name] = results["binaries"][name]["rows"]

    for name, spec in PROMPT_VARIANTS.items():
        print(f"variant {name} ...", file=sys.stderr)
        results["prompt_variants"][name] = evaluate_question(
            cases, spec["question"], spec["true_when"]
        )

    for name, question in GOVERNANCE.items():
        print(f"governance {name} ...", file=sys.stderr)
        results["governance"][name] = evaluate_question(cases, question, "HIGH")

    p_low = {r["id"]: r["p_true"] for r in raw["A_low"]}
    for high_name in ("B_high", "C_escalation"):
        p_high = {r["id"]: r["p_true"] for r in raw[high_name]}
        for first in ("LOW", "HIGH"):
            for label, low_t, high_t in (
                ("t0.5", 0.5, 0.5),
                (
                    "tsearch",
                    results["binaries"]["A_low"]["threshold_search"]["best_threshold"],
                    results["binaries"][high_name]["threshold_search"][
                        "best_threshold"
                    ],
                ),
            ):
                key = f"{(first)}_first_{high_name}_{label}"
                results["hierarchies"][key] = {
                    "high_question": high_name,
                    "order": first,
                    "low_threshold": low_t,
                    "high_threshold": high_t,
                    "metrics": summarize_classes(
                        hierarchy(cases, p_low, p_high, low_t, high_t, first)
                    ),
                }

    print("dual boundary ...", file=sys.stderr)
    results["dual_boundary"] = {
        "t0.5": dual_boundary(
            cases, p_low, {r["id"]: r["p_true"] for r in raw["B_high"]}, 0.5, 0.5
        ),
        "tsearch": dual_boundary(
            cases,
            p_low,
            {r["id"]: r["p_true"] for r in raw["B_high"]},
            results["binaries"]["A_low"]["threshold_search"]["best_threshold"],
            results["binaries"]["B_high"]["threshold_search"]["best_threshold"],
        ),
    }

    print("margin analysis ...", file=sys.stderr)
    results["margin"] = margin_analysis(
        cases, p_low, {r["id"]: r["p_true"] for r in raw["B_high"]}
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
