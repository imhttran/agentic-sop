#!/usr/bin/env python3
"""Phase 2 Julia routing-calibration experiments.

Measures whether Julia can produce a stable, discriminative LOW/MEDIUM/HIGH
routing signal under better evaluation formulations. Uses ``http_bridge.py``
(the provider's real translation layer) against a live Julia service.

This is evaluation evidence only. It changes no production policy, no provider
contract, and no Phase 1 dataset. Requires a live Julia service (``JULIA_URL``).

Usage:
    JULIA_URL=http://127.0.0.1:8011 python3 tools/julia/calibrate_routing2.py \
        tests/fixtures/julia/routing-calibration.json \
        tests/fixtures/julia/routing-validation.json \
        --out /tmp/calibration2.json
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import datetime, timezone
from itertools import permutations

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import calibrate_routing as base  # noqa: E402
import http_bridge  # noqa: E402

CLASSES = ["LOW", "MEDIUM", "HIGH"]
PERMS = [list(p) for p in permutations(CLASSES)]

FEWSHOT_EXEMPLARS = (
    "Classify each software-engineering task by how much work it needs.\n"
    "Examples:\n"
    "- LOW: fix a spelling error in the changelog.\n"
    "- LOW: update a code comment to describe current behavior.\n"
    "- LOW: make a documentation-only change to the API reference.\n"
    "- MEDIUM: add input validation to an existing request handler.\n"
    "- MEDIUM: refactor several helper functions within one package without changing behavior.\n"
    "- MEDIUM: implement a new endpoint using the established service/repository pattern.\n"
    "- HIGH: change authorization semantics so a new role can approve deployments.\n"
    "- HIGH: alter the fallback behavior used when the provider is unavailable.\n"
    "- HIGH: design the schema migration strategy for a cross-service data move.\n"
)


# --- Model access -----------------------------------------------------------


def ask(state, question, options, qtype="choice"):
    request = {
        "state": state,
        "questions": [
            {
                "id": "execution_model",
                "type": qtype,
                "text": question,
                "options": options,
            }
        ],
    }
    return http_bridge.build_output(request)["answers"]["execution_model"]


def choice_signal(state, question, options):
    """Return (selected_class, {class: probability})."""
    answer = ask(state, question, options, "choice")
    probs = {}
    for option, probability in answer.get("probabilities", {}).items():
        cls = base.class_of(option)
        if cls is not None:
            probs[cls] = probability
    return base.class_of(answer["choice"]), probs


def score_signal(state, question, options):
    return ask(state, question, options, "score")["score"]


# --- Aggregation ------------------------------------------------------------


def argmax_class(mean_probs):
    return max(CLASSES, key=lambda c: mean_probs.get(c, 0.0))


def mean_probs(signals):
    out = {c: 0.0 for c in CLASSES}
    for _, probs in signals:
        for c in CLASSES:
            out[c] += probs.get(c, 0.0)
    n = len(signals)
    return {c: out[c] / n for c in CLASSES}


def majority_class(signals):
    counts = {c: 0 for c in CLASSES}
    for selected, _ in signals:
        if selected in counts:
            counts[selected] += 1
    top = max(counts.values())
    winners = [c for c in CLASSES if counts[c] == top]
    if len(winners) == 1:
        return winners[0]
    # Tie-break by mean probability so ties are resolved by evidence, not order.
    mp = mean_probs(signals)
    return max(winners, key=lambda c: mp.get(c, 0.0))


def make_rows(cases, signals_by_case, rule):
    rows = []
    for case in cases:
        signals = signals_by_case[case["id"]]
        if rule == "majority":
            selected = majority_class(signals)
        else:
            selected = argmax_class(mean_probs(signals))
        mp = mean_probs(signals)
        others = [mp[c] for c in CLASSES if c != selected]
        rows.append(
            {
                "id": case["id"],
                "expected": case["expected"],
                "selected": selected,
                "probabilities": mp,
                "selected_probability": mp.get(selected),
                "margin": mp.get(selected, 0.0) - max(others, default=0.0),
            }
        )
    return rows


# --- Metrics ----------------------------------------------------------------


def summarize(
    rows,
    high_confidence=base.HIGH_CONFIDENCE if hasattr(base, "HIGH_CONFIDENCE") else 0.80,
):
    total = len(rows)
    confusion = {e: {p: 0 for p in CLASSES} for e in CLASSES}
    for r in rows:
        if r["expected"] in confusion and r["selected"] in confusion[r["expected"]]:
            confusion[r["expected"]][r["selected"]] += 1

    per_class = {}
    for cls in CLASSES:
        of = [r for r in rows if r["expected"] == cls]
        right = sum(1 for r in of if r["selected"] == cls)
        per_class[cls] = {
            "total": len(of),
            "recall": (right / len(of)) if of else None,
        }

    correct = sum(1 for r in rows if r["selected"] == r["expected"])
    recalls = [
        per_class[c]["recall"] for c in CLASSES if per_class[c]["recall"] is not None
    ]

    hc = [
        r["id"]
        for r in rows
        if r["selected"] != r["expected"]
        and r["selected_probability"] is not None
        and r["selected_probability"] >= high_confidence
    ]

    predicted = {c: sum(1 for r in rows if r["selected"] == c) for c in CLASSES}
    probs = [
        r["selected_probability"] for r in rows if r["selected_probability"] is not None
    ]
    margins = [r["margin"] for r in rows if r["margin"] is not None]

    return {
        "total": total,
        "correct": correct,
        "accuracy": (correct / total) if total else None,
        "macro_recall": (sum(recalls) / len(recalls)) if recalls else None,
        "per_class": per_class,
        "confusion_matrix": confusion,
        "predicted_distribution": predicted,
        "average_selected_probability": (sum(probs) / len(probs)) if probs else None,
        "average_margin": (sum(margins) / len(margins)) if margins else None,
        "high_confidence_threshold": high_confidence,
        "high_confidence_misclassifications": hc,
        "high_confidence_misclassification_count": len(hc),
    }


# --- Experiments ------------------------------------------------------------


def experiment_order_randomization(cases, question):
    """Experiment A: all 6 permutations, 1 run each; quantify position bias."""
    signals_by_case = {c["id"]: [] for c in cases}
    per_case_sensitivity = {}
    for case in cases:
        sels = []
        for perm in PERMS:
            selected, probs = choice_signal(case["state"], question, list(perm))
            signals_by_case[case["id"]].append((selected, probs))
            sels.append(selected)
        per_case_sensitivity[case["id"]] = {
            "distinct": sorted(set(s for s in sels if s)),
            "stable": len(set(sels)) == 1,
        }
    majority = make_rows(cases, signals_by_case, "majority")
    meanprob = make_rows(cases, signals_by_case, "meanprob")
    stable = sum(1 for v in per_case_sensitivity.values() if v["stable"])
    changed = sum(1 for v in per_case_sensitivity.values() if not v["stable"])
    return {
        "majority": {"metrics": summarize(majority), "rows": majority},
        "mean_probability": {"metrics": summarize(meanprob), "rows": meanprob},
        "order_sensitivity": {
            "stable_cases": stable,
            "order_changed_cases": changed,
            "stability_rate": stable / len(cases),
            "per_case": per_case_sensitivity,
        },
    }


def experiment_repeated_runs(cases, question, reps):
    """Experiment B: repeated inference in the canonical order."""
    order = ["LOW", "MEDIUM", "HIGH"]
    signals_by_case = {}
    stability = {}
    for case in cases:
        signals = [
            choice_signal(case["state"], question, list(order)) for _ in range(reps)
        ]
        signals_by_case[case["id"]] = signals
        chosen = [s for s, _ in signals]
        stability[case["id"]] = {
            "distinct": sorted(set(c for c in chosen if c)),
            "stable": len(set(chosen)) == 1,
        }
    majority = make_rows(cases, signals_by_case, "majority")
    meanprob = make_rows(cases, signals_by_case, "meanprob")
    stable = sum(1 for v in stability.values() if v["stable"])
    return {
        "reps": reps,
        "majority": {"metrics": summarize(majority), "rows": majority},
        "mean_probability": {"metrics": summarize(meanprob), "rows": meanprob},
        "stability": {
            "stable_cases": stable,
            "stability_rate": stable / len(cases),
            "per_case": stability,
        },
    }


def experiment_order_ensemble(cases, question, reps):
    """Experiment C: permutations x repetitions, aggregated after remapping."""
    signals_by_case = {c["id"]: [] for c in cases}
    for case in cases:
        for perm in PERMS:
            for _ in range(reps):
                signals_by_case[case["id"]].append(
                    choice_signal(case["state"], question, list(perm))
                )
    majority = make_rows(cases, signals_by_case, "majority")
    meanprob = make_rows(cases, signals_by_case, "meanprob")
    return {
        "permutations": 6,
        "reps": reps,
        "majority": {"metrics": summarize(majority), "rows": majority},
        "mean_probability": {"metrics": summarize(meanprob), "rows": meanprob},
    }


def experiment_fewshot(cases, framing):
    """Experiment D: few-shot exemplars + a framing."""
    signals_by_case = {}
    total_chars = 0
    for case in cases:
        state = FEWSHOT_EXEMPLARS + "\nTask: " + case["state"]
        total_chars += len(case["state"])
        if framing == "bare":
            selected, probs = choice_signal(
                state, base.CURRENT_QUESTION, ["LOW", "MEDIUM", "HIGH"]
            )
        else:  # descriptive
            selected, probs = choice_signal(
                state,
                base.CAPABILITY_QUESTION,
                [
                    base.DESCRIPTIVE["LOW"],
                    base.DESCRIPTIVE["MEDIUM"],
                    base.DESCRIPTIVE["HIGH"],
                ],
            )
        signals_by_case[case["id"]] = [(selected, probs)]
    rows = make_rows(cases, signals_by_case, "majority")
    return {
        "framing": framing,
        "exemplar_chars": len(FEWSHOT_EXEMPLARS),
        "average_case_chars": total_chars / len(cases) if cases else 0,
        "metrics": summarize(rows),
        "rows": rows,
    }


def experiment_score(cases, question, options):
    """Experiment E: raw SCORE distributions per expected class."""
    scores = {}
    for case in cases:
        scores[case["id"]] = score_signal(case["state"], question, list(options))
    by_class = {c: [] for c in CLASSES}
    for case in cases:
        by_class[case["expected"]].append(scores[case["id"]])

    def stats(values):
        if not values:
            return None
        vs = sorted(values)
        n = len(vs)

        def quantile(q):
            return vs[min(n - 1, int(q * n))]

        return {
            "n": n,
            "mean": sum(vs) / n,
            "median": vs[n // 2],
            "min": vs[0],
            "max": vs[-1],
            "q1": quantile(0.25),
            "q3": quantile(0.75),
        }

    return {
        "scores": scores,
        "by_class": {c: stats(by_class[c]) for c in CLASSES},
    }


def threshold_search(score_by_class, grid_step=0.02):
    """Phase 9: offline threshold search maximizing balanced accuracy."""
    samples = []
    for cls, values in score_by_class.items():
        for value in values:
            samples.append((value, cls))

    best = None
    grid = [i * grid_step for i in range(int(2.0 / grid_step) + 1)]
    for t1 in grid:
        for t2 in grid:
            if t2 <= t1:
                continue
            recalls = {c: 0 for c in CLASSES}
            totals = {c: 0 for c in CLASSES}
            for value, cls in samples:
                totals[cls] += 1
                if value < t1:
                    pred = "LOW"
                elif value < t2:
                    pred = "MEDIUM"
                else:
                    pred = "HIGH"
                if pred == cls:
                    recalls[cls] += 1
            recall = {
                c: (recalls[c] / totals[c]) if totals[c] else 0.0 for c in CLASSES
            }
            balanced = sum(recall.values()) / len(CLASSES)
            # Prefer thresholds that preserve all three classes (non-zero recall)
            # before maximizing balanced accuracy.
            preserved = all(recall[c] > 0.0 for c in CLASSES)
            key = (preserved, round(balanced, 6))
            if best is None or key > best[0]:
                best = (key, t1, t2, balanced, recall)
    _, t1, t2, balanced, recall = best
    return {
        "thresholds": {"t1": round(t1, 4), "t2": round(t2, 4)},
        "balanced_accuracy": balanced,
        "recall": recall,
    }


def score_rows(cases, scores, t1, t2):
    rows = []
    for case in cases:
        value = scores[case["id"]]
        selected = "LOW" if value < t1 else ("MEDIUM" if value < t2 else "HIGH")
        rows.append(
            {
                "id": case["id"],
                "expected": case["expected"],
                "selected": selected,
                "score": value,
            }
        )
    return rows


def summarize_score_rows(rows):
    total = len(rows)
    confusion = {e: {p: 0 for p in CLASSES} for e in CLASSES}
    for r in rows:
        confusion[r["expected"]][r["selected"]] += 1
    per_class = {}
    for cls in CLASSES:
        of = [r for r in rows if r["expected"] == cls]
        right = sum(1 for r in of if r["selected"] == cls)
        per_class[cls] = {"total": len(of), "recall": (right / len(of)) if of else None}
    correct = sum(1 for r in rows if r["selected"] == r["expected"])
    recalls = [per_class[c]["recall"] for c in CLASSES]
    return {
        "total": total,
        "confidence_thresholded": None,
        "accuracy": correct / total if total else None,
        "macro_recall": sum(recalls) / len(recalls),
        "per_class": per_class,
        "confusion_matrix": confusion,
        "predicted_distribution": {
            c: sum(1 for r in rows if r["selected"] == c) for c in CLASSES
        },
    }


# --- Main -------------------------------------------------------------------


def load(path):
    with open(path, "r", encoding="utf-8") as handle:
        return json.load(handle)["cases"]


def main():
    parser = argparse.ArgumentParser(
        description="Phase 2 Julia routing calibration experiments."
    )
    parser.add_argument("calibration", help="calibration dataset JSON")
    parser.add_argument("validation", help="validation dataset JSON")
    parser.add_argument("--out", help="write results here (default stdout)")
    parser.add_argument("--ensemble-reps", type=int, default=5)
    args = parser.parse_args()

    cal_cases = load(args.calibration)
    val_cases = load(args.validation)
    print(
        f"calibration={len(cal_cases)} validation={len(val_cases)} perms={len(PERMS)}",
        file=sys.stderr,
    )

    results = {
        "generated_at": datetime.now(timezone.utc).isoformat(),
        "model": http_bridge.MODEL,
        "julia_url": http_bridge.JULIA_URL,
        "calibration_dataset": args.calibration,
        "validation_dataset": args.validation,
        "calibration_counts": {
            c: sum(1 for x in cal_cases if x["expected"] == c) for c in CLASSES
        },
        "validation_counts": {
            c: sum(1 for x in val_cases if x["expected"] == c) for c in CLASSES
        },
        "experiments": {},
    }

    ex = results["experiments"]

    print("Experiment A: order randomization ...", file=sys.stderr)
    ex["A_order_randomization"] = experiment_order_randomization(
        cal_cases, base.CURRENT_QUESTION
    )

    print("Experiment B: repeated runs ...", file=sys.stderr)
    ex["B_repeated_runs"] = experiment_repeated_runs(
        cal_cases, base.CURRENT_QUESTION, 5
    )

    print("Experiment C: order ensemble ...", file=sys.stderr)
    ex["C_order_ensemble_1x6"] = experiment_order_ensemble(
        cal_cases, base.CURRENT_QUESTION, 1
    )
    ex["C_order_ensemble_3x6"] = experiment_order_ensemble(
        cal_cases, base.CURRENT_QUESTION, 3
    )
    ex["C_order_ensemble_5x6"] = experiment_order_ensemble(
        cal_cases, base.CURRENT_QUESTION, 5
    )

    print("Experiment D: few-shot ...", file=sys.stderr)
    ex["D_fewshot_bare"] = experiment_fewshot(cal_cases, "bare")
    ex["D_fewshot_descriptive"] = experiment_fewshot(cal_cases, "descriptive")

    print("Experiment E: score distributions ...", file=sys.stderr)
    score_exp = experiment_score(
        cal_cases,
        "Rate the execution severity of the described task.",
        ["LOW", "MEDIUM", "HIGH"],
    )
    ex["E_score_distributions"] = score_exp

    print("Phase 9: threshold search ...", file=sys.stderr)
    by_class = {
        c: [score_exp["scores"][x["id"]] for x in cal_cases if x["expected"] == c]
        for c in CLASSES
    }
    thresholds = threshold_search(by_class)
    ex["F_score_threshold"] = {
        "search": thresholds,
        "metrics": summarize_score_rows(
            score_rows(
                cal_cases,
                score_exp["scores"],
                thresholds["thresholds"]["t1"],
                thresholds["thresholds"]["t2"],
            )
        ),
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
