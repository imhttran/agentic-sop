#!/usr/bin/env python3
"""Phase 4 addendum: expanded validation of the FROZEN escalation question.

Runs the frozen question on the NEWLY ADDED cases only, combines those with the
previously captured 100-case outputs, and computes all addendum metrics
(hard-case coverage, minimal pairs, sensitivity, cost, baselines, reproducibility).

Frozen question: "Does this task require escalation?" — no wording search, no
threshold tuning (readiness uses T=0.50; T=0.60 is the secondary sensitivity point).

Evaluation only. Requires a live Julia service (``JULIA_URL``).

Usage:
    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/escalation_validation2.py \
        tests/fixtures/julia/escalation-validation.json \
        --old-results /tmp/phase4.json --out /tmp/phase4b.json
"""

from __future__ import annotations

import argparse
import json
import os
import random
import sys
from datetime import datetime, timezone

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402

FROZEN_QUESTION = "Does this task require escalation?"
READINESS_THRESHOLD = 0.50
SECONDARY_THRESHOLD = 0.60
SWEEP = [0.40, 0.45, 0.50, 0.55, 0.60, 0.65, 0.70]
KEYWORDS = [
    "security",
    "authorization",
    "production",
    "migration",
    "architecture",
    "destructive",
    "credential",
]


def ask(state):
    request = {
        "state": state,
        "questions": [{"id": "q", "type": "boolean", "text": FROZEN_QUESTION}],
    }
    return http_bridge.build_output(request)["answers"]["q"]["probability"]


def metrics(rows, threshold):
    tp = fp = tn = fn = 0
    for r in rows:
        if (r["p_escalation"] >= threshold) == r["escalation"]:
            if r["escalation"]:
                tp += 1
            else:
                tn += 1
        else:
            if r["escalation"]:
                fn += 1
            else:
                fp += 1
    total = tp + fp + tn + fn
    precision = tp / (tp + fp) if (tp + fp) else None
    recall = tp / (tp + fn) if (tp + fn) else None
    specificity = tn / (tn + fp) if (tn + fp) else None
    f1 = (
        (2 * precision * recall / (precision + recall))
        if precision and recall
        else None
    )
    return {
        "threshold": threshold,
        "tp": tp,
        "fp": fp,
        "tn": tn,
        "fn": fn,
        "accuracy": (tp + tn) / total,
        "precision": precision,
        "recall": recall,
        "specificity": specificity,
        "f1": f1,
        "balanced_accuracy": ((recall or 0.0) + (specificity or 0.0)) / 2,
        "false_positive_rate": (fp / (fp + tn)) if (fp + tn) else None,
        "false_negative_rate": (fn / (fn + tp)) if (fn + tp) else None,
    }


def stat(values):
    if not values:
        return None
    v = sorted(values)
    n = len(v)
    mean = sum(v) / n
    return {
        "n": n,
        "mean": mean,
        "median": v[n // 2],
        "stddev": (sum((x - mean) ** 2 for x in v) / n) ** 0.5,
        "min": v[0],
        "max": v[-1],
        "q1": v[min(n - 1, int(0.25 * n))],
        "q3": v[min(n - 1, int(0.75 * n))],
    }


def keyword_escalation(state):
    low = state.lower()
    return any(k in low for k in KEYWORDS)


def main():
    parser = argparse.ArgumentParser(
        description="Phase 4 addendum expanded validation."
    )
    parser.add_argument("dataset")
    parser.add_argument(
        "--old-results", required=True, help="cached 100-case results JSON"
    )
    parser.add_argument("--out")
    parser.add_argument("--repro-n", type=int, default=20)
    args = parser.parse_args()

    with open(args.dataset, "r", encoding="utf-8") as handle:
        cases = json.load(handle)["cases"]
    meta = {c["id"]: c for c in cases}

    with open(args.old_results, "r", encoding="utf-8") as handle:
        old = json.load(handle)
    old_rows = old["rows"]

    new_cases = [c for c in cases if c.get("pair_id")]
    print(
        f"re-running {len(new_cases)} new cases (reusing {len(old_rows)} cached)",
        file=sys.stderr,
    )
    new_rows = [
        {"id": c["id"], "escalation": c["escalation"], "p_escalation": ask(c["state"])}
        for c in new_cases
    ]
    combined = old_rows + new_rows

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "dataset": args.dataset,
        "frozen_question": FROZEN_QUESTION,
        "counts": {
            "total": len(combined),
            "escalation": sum(1 for r in combined if r["escalation"]),
            "non_escalation": sum(1 for r in combined if not r["escalation"]),
            "new_cases": len(new_cases),
        },
        "combined_metrics": {
            str(t): metrics(combined, t)
            for t in (READINESS_THRESHOLD, SECONDARY_THRESHOLD)
        },
        "distribution": {
            "escalation": stat(
                [r["p_escalation"] for r in combined if r["escalation"]]
            ),
            "non_escalation": stat(
                [r["p_escalation"] for r in combined if not r["escalation"]]
            ),
        },
        "threshold_sensitivity": {str(t): metrics(combined, t) for t in SWEEP},
    }

    # Hard cases.
    def subset(flt):
        return [r for r in combined if flt(meta[r["id"]])]

    hn = subset(lambda c: c.get("hard_negative"))
    hp = subset(lambda c: c.get("hard_positive"))
    results["hard_negative"] = {
        "count": len(hn),
        "correctly_rejected": sum(
            1 for r in hn if r["p_escalation"] < READINESS_THRESHOLD
        ),
        "false_escalations": sum(
            1 for r in hn if r["p_escalation"] >= READINESS_THRESHOLD
        ),
    }
    results["hard_negative"]["specificity"] = (
        (results["hard_negative"]["correctly_rejected"] / len(hn)) if hn else None
    )
    results["hard_positive"] = {
        "count": len(hp),
        "correctly_escalated": sum(
            1 for r in hp if r["p_escalation"] >= READINESS_THRESHOLD
        ),
        "missed_escalations": sum(
            1 for r in hp if r["p_escalation"] < READINESS_THRESHOLD
        ),
    }
    results["hard_positive"]["recall"] = (
        (results["hard_positive"]["correctly_escalated"] / len(hp)) if hp else None
    )

    # Minimal pairs.
    pairs = {}
    for c in cases:
        if c.get("pair_id"):
            pairs.setdefault(c["pair_id"], {})[c["escalation"]] = c["id"]
    by_id = {r["id"]: r for r in combined}
    both = one = wrong = 0
    esc_side = non_side = 0
    for pid, m in pairs.items():
        a, b = by_id[m[False]], by_id[m[True]]
        a_ok = a["p_escalation"] < READINESS_THRESHOLD
        b_ok = b["p_escalation"] >= READINESS_THRESHOLD
        esc_side += b_ok
        non_side += a_ok
        if a_ok and b_ok:
            both += 1
        elif a_ok or b_ok:
            one += 1
        else:
            wrong += 1
    n_pairs = len(pairs)
    results["minimal_pairs"] = {
        "count": n_pairs,
        "both_correct": both,
        "one_correct": one,
        "both_wrong": wrong,
        "both_correct_rate": both / n_pairs if n_pairs else None,
        "escalation_side_accuracy": esc_side / n_pairs if n_pairs else None,
        "non_escalation_side_accuracy": non_side / n_pairs if n_pairs else None,
    }

    # FN / FP detail.
    results["false_negatives"] = [
        {
            "id": r["id"],
            "p": r["p_escalation"],
            "state": meta[r["id"]]["state"],
            "hard_positive": bool(meta[r["id"]].get("hard_positive")),
        }
        for r in combined
        if r["escalation"] and r["p_escalation"] < READINESS_THRESHOLD
    ]
    results["false_positives"] = [
        {
            "id": r["id"],
            "p": r["p_escalation"],
            "state": meta[r["id"]]["state"],
            "hard_negative": bool(meta[r["id"]].get("hard_negative")),
        }
        for r in combined
        if not r["escalation"] and r["p_escalation"] >= READINESS_THRESHOLD
    ]

    # Cost analysis.
    results["cost"] = {
        str(t): {
            "policyA_fn5_fp1": m["fn"] * 5 + m["fp"] * 1,
            "policyB_fn2_fp1": m["fn"] * 2 + m["fp"] * 1,
        }
        for t, m in results["threshold_sensitivity"].items()
    }

    # Baselines.
    def baseline(pred_fn):
        tp = fp = tn = fn = 0
        for r in combined:
            pred = pred_fn(meta[r["id"]])
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
        return {
            "tp": tp,
            "fp": fp,
            "tn": tn,
            "fn": fn,
            "accuracy": (tp + tn) / total,
            "balanced_accuracy": ((recall or 0.0) + (specificity or 0.0)) / 2,
            "recall": recall,
            "specificity": specificity,
            "f1": f1,
        }

    rng = random.Random(12345)
    rand_rows = []
    for r in combined:
        rand_rows.append(
            {"id": r["id"], "escalation": r["escalation"], "p_escalation": rng.random()}
        )
    keyword_rows = [
        {
            "id": r["id"],
            "escalation": r["escalation"],
            "p_escalation": 1.0 if keyword_escalation(meta[r["id"]]["state"]) else 0.0,
        }
        for r in combined
    ]
    results["baselines"] = {
        "always_escalate": baseline(lambda c: True),
        "never_escalate": baseline(lambda c: False),
        "balanced_random": metrics(rand_rows, 0.5),
        "keyword": {
            **metrics(keyword_rows, 0.5),
            "hard_negative_specificity": (
                sum(
                    1
                    for r in keyword_rows
                    if meta[r["id"]].get("hard_negative") and r["p_escalation"] < 0.5
                )
                / max(1, len(hn))
            ),
            "hard_positive_recall": (
                sum(
                    1
                    for r in keyword_rows
                    if meta[r["id"]].get("hard_positive") and r["p_escalation"] >= 0.5
                )
                / max(1, len(hp))
            ),
        },
    }

    # Reproducibility: rerun a representative sample.
    sample = []
    seen = set()
    for r in combined:
        c = meta[r["id"]]
        cat = (
            "hn"
            if c.get("hard_negative")
            else "hp"
            if c.get("hard_positive")
            else "esc"
            if c["escalation"]
            else "non"
        )
        if cat not in seen:
            sample.append(r)
            seen.add(cat)
    others = [r for r in combined if r["id"] not in {s["id"] for s in sample}]
    rng2 = random.Random(7)
    sample += rng2.sample(others, max(0, min(args.repro_n, len(others)) - len(sample)))
    repro = []
    for r in sample[: args.repro_n]:
        p2 = ask(meta[r["id"]]["state"])
        repro.append(
            {
                "id": r["id"],
                "p1": r["p_escalation"],
                "p2": p2,
                "same_label": (r["p_escalation"] >= 0.5) == (p2 >= 0.5),
                "abs_drift": abs(r["p_escalation"] - p2),
            }
        )
    results["reproducibility"] = {
        "n": len(repro),
        "exact_repeat": sum(1 for x in repro if x["abs_drift"] == 0.0),
        "changed_result": sum(1 for x in repro if not x["same_label"]),
        "max_abs_drift": max((x["abs_drift"] for x in repro), default=0.0),
        "rows": repro,
    }

    results["rows"] = combined

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
