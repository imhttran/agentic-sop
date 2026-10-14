#!/usr/bin/env python3
"""Unit tests for the pure functions in the Julia inference helper.

These tests import only the standard library: they never load ONNX Runtime, a
tokenizer, a model, or the network. Run them with::

    python3 tools/julia/test_infer.py

or::

    python3 -m unittest discover -s tools/julia -p 'test_*.py'
"""

from __future__ import annotations

import math
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import infer  # noqa: E402


class SoftmaxTests(unittest.TestCase):
    def test_sums_to_one(self):
        probs = infer.softmax([-3.1, 0.4, 2.9])
        self.assertAlmostEqual(sum(probs), 1.0, places=12)

    def test_uniform(self):
        self.assertEqual(infer.softmax([0.0, 0.0]), [0.5, 0.5])

    def test_large_values_are_stable(self):
        probs = infer.softmax([1000.0, 1000.0])
        self.assertEqual(probs, [0.5, 0.5])
        self.assertTrue(all(math.isfinite(p) for p in probs))

    def test_rejects_empty(self):
        with self.assertRaises(infer.HelperError) as ctx:
            infer.softmax([])
        self.assertEqual(ctx.exception.kind, "malformed")


class ArgmaxTests(unittest.TestCase):
    def test_picks_max(self):
        self.assertEqual(infer.argmax([0.1, 0.7, 0.2]), 1)

    def test_ties_pick_first(self):
        self.assertEqual(infer.argmax([0.5, 0.5]), 0)


class ExpectedIndexTests(unittest.TestCase):
    def test_weighted_index(self):
        self.assertAlmostEqual(infer.expected_index([0.2, 0.3, 0.5]), 1.3)

    def test_certain(self):
        self.assertAlmostEqual(infer.expected_index([0.0, 1.0, 0.0]), 1.0)


class ValidateLogitsTests(unittest.TestCase):
    def test_length_mismatch(self):
        with self.assertRaises(infer.HelperError) as ctx:
            infer.validate_logits([0.0, 1.0], 3)
        self.assertEqual(ctx.exception.kind, "malformed")

    def test_nan_rejected(self):
        with self.assertRaises(infer.HelperError):
            infer.validate_logits([0.0, float("nan")], 2)

    def test_inf_rejected(self):
        with self.assertRaises(infer.HelperError):
            infer.validate_logits([0.0, float("inf")], 2)

    def test_valid(self):
        infer.validate_logits([0.0, 1.0], 2)


class ValidateQuestionTests(unittest.TestCase):
    def _choice(self, n):
        return {
            "id": "q",
            "type": "choice",
            "qtype": infer.QTYPE_CHOICE,
            "text": "t",
            "options": [f"O{i}" for i in range(n)],
        }

    def test_choice_1_option_rejected(self):
        with self.assertRaises(infer.HelperError):
            infer.validate_question(self._choice(1))

    def test_choice_2_options_accepted(self):
        infer.validate_question(self._choice(2))

    def test_choice_20_options_accepted(self):
        infer.validate_question(self._choice(20))

    def test_choice_21_options_rejected(self):
        with self.assertRaises(infer.HelperError):
            infer.validate_question(self._choice(21))

    def test_score_uses_same_limits(self):
        q = self._choice(2)
        q["qtype"] = infer.QTYPE_SCORE
        infer.validate_question(q)
        q["options"] = ["only"]
        with self.assertRaises(infer.HelperError):
            infer.validate_question(q)

    def test_boolean_requires_fixed_options(self):
        q = {
            "id": "b",
            "type": "boolean",
            "qtype": infer.QTYPE_BOOLEAN,
            "text": "t",
            "options": ["false", "true"],
        }
        infer.validate_question(q)
        q["options"] = ["no", "yes"]
        with self.assertRaises(infer.HelperError):
            infer.validate_question(q)

    def test_unknown_qtype_rejected(self):
        q = {
            "id": "x",
            "type": "choice",
            "qtype": 9,
            "text": "t",
            "options": ["a", "b"],
        }
        with self.assertRaises(infer.HelperError):
            infer.validate_question(q)


class DecodeTests(unittest.TestCase):
    def test_choice(self):
        q = {
            "id": "risk",
            "qtype": infer.QTYPE_CHOICE,
            "text": "t",
            "options": ["LOW", "MEDIUM", "HIGH"],
        }
        out = infer.decode_question(q, [-3.1, 0.4, 2.9])
        self.assertEqual(out["choice"], "HIGH")
        self.assertAlmostEqual(sum(out["probabilities"].values()), 1.0, places=12)
        self.assertNotIn("confidence", out)

    def test_boolean_maps_true_probability(self):
        q = {
            "id": "approval_required",
            "qtype": infer.QTYPE_BOOLEAN,
            "text": "t",
            "options": ["false", "true"],
        }
        out = infer.decode_question(q, [0.0, 5.0])
        self.assertGreater(out["probability"], 0.99)
        self.assertNotIn("confidence", out)

    def test_score_maps_expected_index_not_argmax(self):
        q = {
            "id": "s",
            "qtype": infer.QTYPE_SCORE,
            "text": "t",
            "options": ["LOW", "MEDIUM", "HIGH"],
        }
        # Uniform logits: argmax would be index 0, but the expected index is 1.0.
        out = infer.decode_question(q, [1.0, 1.0, 1.0])
        self.assertAlmostEqual(out["score"], 1.0)

    def test_rejects_non_finite_logits(self):
        q = {
            "id": "risk",
            "qtype": infer.QTYPE_CHOICE,
            "text": "t",
            "options": ["LOW", "HIGH"],
        }
        with self.assertRaises(infer.HelperError):
            infer.decode_question(q, [float("nan"), 0.0])


class MarkerTests(unittest.TestCase):
    def test_finds_subsequence(self):
        pos, mask = infer.find_marker_positions([10, 20, 30, 40], [[20, 30], [99]])
        self.assertEqual(pos, [1, -1])
        self.assertEqual(mask, [True, False])

    def test_missing_span_masked(self):
        pos, mask = infer.find_marker_positions([1, 2, 3], [[9]])
        self.assertEqual(pos, [-1])
        self.assertEqual(mask, [False])


class RenderTests(unittest.TestCase):
    def test_includes_state_question_options(self):
        text = infer.render_prompt("STATE", "QUESTION", ["A", "B"])
        self.assertIn("STATE", text)
        self.assertIn("QUESTION", text)
        self.assertIn("- A", text)
        self.assertIn("- B", text)


if __name__ == "__main__":
    unittest.main()
