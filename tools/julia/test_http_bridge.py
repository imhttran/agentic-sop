#!/usr/bin/env python3
"""Unit tests for ``http_bridge.py``.

These tests are fully hermetic: they use only the standard library and a local
in-process mock HTTP server bound to an ephemeral port. They never contact a real
Julia service, never bind port 8011, and never load a model. Run them with::

    python3 tools/julia/test_http_bridge.py

or::

    python3 -m unittest discover -s tools/julia -p 'test_*.py'
"""

from __future__ import annotations

import io
import json
import os
import socket
import sys
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import http_bridge  # noqa: E402


class _Handler(BaseHTTPRequestHandler):
    def do_POST(self):  # noqa: N802 - http.server API
        length = int(self.headers.get("Content-Length", 0))
        raw = self.rfile.read(length)
        record = {
            "path": self.path,
            "content_type": self.headers.get("Content-Type"),
            "json": json.loads(raw.decode("utf-8")),
        }
        self.server.requests.append(record)

        status, payload, delay = self.server.responder(record)
        if delay:
            time.sleep(delay)

        data = (
            payload
            if isinstance(payload, bytes)
            else json.dumps(payload).encode("utf-8")
        )
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        try:
            self.wfile.write(data)
        except OSError:
            # The client may have already timed out and closed the socket.
            pass

    def log_message(self, *args):  # silence the default stderr logging
        pass


class _Server(ThreadingHTTPServer):
    daemon_threads = True

    def __init__(self, responder):
        super().__init__(("127.0.0.1", 0), _Handler)
        self.responder = responder
        self.requests = []


def _free_port() -> int:
    """Return a currently-unused local TCP port (for the connection-refused case)."""
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as sock:
        sock.bind(("127.0.0.1", 0))
        return sock.getsockname()[1]


class BridgeTestCase(unittest.TestCase):
    """Base case that manages mock servers and restores patched module config."""

    def setUp(self):
        self._servers = []
        self._saved = {}

    def tearDown(self):
        for server, thread in self._servers:
            server.shutdown()
            server.server_close()
            thread.join(timeout=5)
        for name, value in self._saved.items():
            setattr(http_bridge, name, value)

    def patch(self, name, value):
        if name not in self._saved:
            self._saved[name] = getattr(http_bridge, name)
        setattr(http_bridge, name, value)

    def start_server(self, responder):
        server = _Server(responder)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        self._servers.append((server, thread))
        self.patch("JULIA_URL", f"http://127.0.0.1:{server.server_address[1]}")
        return server

    def assert_bridge_error(self, kind, func, *args):
        with self.assertRaises(http_bridge.BridgeError) as ctx:
            func(*args)
        self.assertEqual(ctx.exception.kind, kind, msg=str(ctx.exception))
        return ctx.exception


def choice(question_id="risk", options=("LOW", "MEDIUM", "HIGH")):
    return {
        "id": question_id,
        "type": "choice",
        "text": "Assess the operational risk.",
        "options": list(options),
    }


def boolean(question_id="approval_required"):
    return {"id": question_id, "type": "boolean", "text": "Is approval required?"}


class ChoiceTests(BridgeTestCase):
    def test_choice_success(self):
        self.start_server(
            lambda _req: (200, {"index": 1, "probabilities": [0.1, 0.8, 0.1]}, 0)
        )
        out = http_bridge.build_output({"state": "s", "questions": [choice()]})
        self.assertEqual(out["model"], http_bridge.MODEL)
        self.assertEqual(out["answers"]["risk"]["choice"], "MEDIUM")

    def test_choice_preserves_full_probability_map(self):
        self.start_server(
            lambda _req: (200, {"index": 1, "probabilities": [0.1, 0.8, 0.1]}, 0)
        )
        out = http_bridge.build_output({"state": "s", "questions": [choice()]})
        self.assertEqual(
            out["answers"]["risk"]["probabilities"],
            {"LOW": 0.1, "MEDIUM": 0.8, "HIGH": 0.1},
        )

    def test_choice_request_shape(self):
        server = self.start_server(
            lambda _req: (200, {"index": 0, "probabilities": [1.0, 0.0, 0.0]}, 0)
        )
        http_bridge.build_output({"state": "the state", "questions": [choice()]})
        sent = server.requests[0]
        self.assertEqual(sent["path"], "/predict")
        self.assertEqual(sent["content_type"], "application/json")
        self.assertEqual(sent["json"]["state"], "the state")
        self.assertEqual(sent["json"]["type"], "choice")
        self.assertEqual(sent["json"]["question"], "Assess the operational risk.")
        self.assertEqual(sent["json"]["options"], ["LOW", "MEDIUM", "HIGH"])

    def test_choice_without_probabilities_is_allowed(self):
        self.start_server(lambda _req: (200, {"index": 2}, 0))
        out = http_bridge.build_output({"state": "s", "questions": [choice()]})
        self.assertEqual(out["answers"]["risk"], {"choice": "HIGH"})


class BooleanTests(BridgeTestCase):
    def test_boolean_translates_to_noul_and_false_true_options(self):
        server = self.start_server(
            lambda _req: (200, {"index": 1, "probabilities": [0.0, 1.0]}, 0)
        )
        http_bridge.build_output({"state": "s", "questions": [boolean()]})
        sent = server.requests[0]["json"]
        self.assertEqual(sent["type"], "noul")
        self.assertEqual(sent["options"], ["false", "true"])

    def test_boolean_probability_normalization(self):
        self.start_server(
            lambda _req: (200, {"index": 1, "probabilities": [0.0, 1.0]}, 0)
        )
        out = http_bridge.build_output({"state": "s", "questions": [boolean()]})
        self.assertEqual(out["answers"]["approval_required"], {"probability": 1.0})

    def test_boolean_probability_uses_true_index(self):
        self.start_server(
            lambda _req: (200, {"index": 0, "probabilities": [0.25, 0.75]}, 0)
        )
        out = http_bridge.build_output({"state": "s", "questions": [boolean()]})
        self.assertEqual(out["answers"]["approval_required"], {"probability": 0.75})

    def test_boolean_requires_two_probabilities(self):
        self.start_server(lambda _req: (200, {"index": 1}, 0))
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [boolean()]},
        )

    def test_boolean_probability_count_mismatch(self):
        self.start_server(lambda _req: (200, {"index": 0, "probabilities": [0.3]}, 0))
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [boolean()]},
        )


class ScoreTests(BridgeTestCase):
    def test_score_is_expected_index_not_argmax(self):
        # options length 3; expected index = 0*0 + 1*0.6 + 2*0.4 = 1.4;
        # argmax would be index 1.
        server = self.start_server(
            lambda _req: (200, {"index": 1, "probabilities": [0.0, 0.6, 0.4]}, 0)
        )
        out = http_bridge.build_output(
            {"state": "s", "questions": [choice() | {"type": "score"}]}
        )
        self.assertAlmostEqual(out["answers"]["risk"]["score"], 1.4, places=12)
        self.assertEqual(server.requests[0]["json"]["type"], "score")

    def test_score_uniform_is_middle_index(self):
        self.start_server(
            lambda _req: (
                200,
                {"index": 0, "probabilities": [0.2, 0.2, 0.2, 0.2, 0.2]},
                0,
            )
        )
        q = choice(options=("1", "2", "3", "4", "5")) | {"type": "score"}
        out = http_bridge.build_output({"state": "s", "questions": [q]})
        self.assertAlmostEqual(out["answers"]["risk"]["score"], 2.0, places=12)

    def test_score_requires_probabilities(self):
        self.start_server(lambda _req: (200, {"index": 1}, 0))
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [choice() | {"type": "score"}]},
        )


class AggregateTests(BridgeTestCase):
    def test_multiple_questions_aggregated(self):
        def responder(record):
            if record["json"]["type"] == "noul":
                return (200, {"index": 1, "probabilities": [0.05, 0.95]}, 0)
            return (200, {"index": 2, "probabilities": [0.1, 0.2, 0.7]}, 0)

        server = self.start_server(responder)
        out = http_bridge.build_output(
            {"state": "s", "questions": [choice(), boolean()]}
        )
        self.assertEqual(
            out["answers"]["risk"],
            {
                "choice": "HIGH",
                "probabilities": {"LOW": 0.1, "MEDIUM": 0.2, "HIGH": 0.7},
            },
        )
        self.assertEqual(out["answers"]["approval_required"], {"probability": 0.95})
        self.assertEqual(len(server.requests), 2)

    def test_partial_multi_question_failure_fails_closed(self):
        # The second question fails; the bridge emits no partial output.
        def responder(record):
            if record["json"]["type"] == "noul":
                return (500, {"detail": "noul boom"}, 0)
            return (200, {"index": 0, "probabilities": [1.0, 0.0, 0.0]}, 0)

        self.start_server(responder)
        self.assert_bridge_error(
            "failure",
            http_bridge.build_output,
            {"state": "s", "questions": [choice(), boolean()]},
        )


class TransportFailureTests(BridgeTestCase):
    def test_server_unavailable_connection_refused(self):
        self.patch("JULIA_URL", f"http://127.0.0.1:{_free_port()}")
        self.assert_bridge_error(
            "unavailable",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )

    def test_timeout_is_unavailable(self):
        self.patch("TIMEOUT", 0.2)
        self.start_server(
            lambda _req: (200, {"index": 0, "probabilities": [1.0, 0.0, 0.0]}, 1.0)
        )
        self.assert_bridge_error(
            "unavailable",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )

    def test_http_500_is_failure(self):
        self.start_server(lambda _req: (500, {"detail": "boom"}, 0))
        err = self.assert_bridge_error(
            "failure",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )
        self.assertIn("500", err.message)

    def test_http_400_is_failure(self):
        self.start_server(lambda _req: (400, {"detail": "bad request"}, 0))
        err = self.assert_bridge_error(
            "failure",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )
        self.assertIn("400", err.message)

    def test_malformed_json_response(self):
        self.start_server(lambda _req: (200, b"not json", 0))
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )

    def test_empty_response_is_malformed(self):
        self.start_server(lambda _req: (200, b"", 0))
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )

    def test_http_404_is_failure(self):
        self.start_server(lambda _req: (404, {"detail": "no such route"}, 0))
        err = self.assert_bridge_error(
            "failure",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )
        self.assertIn("404", err.message)

    def test_dns_failure_is_unavailable(self):
        # `.invalid` is reserved to never resolve, so this exercises the
        # name-resolution branch of the transport error handling.
        self.patch("TIMEOUT", 2.0)
        self.patch("JULIA_URL", "http://does-not-exist.invalid:8011")
        self.assert_bridge_error(
            "unavailable",
            http_bridge.build_output,
            {"state": "s", "questions": [choice()]},
        )


class ResponseValidationTests(BridgeTestCase):
    def _with_result(self, result):
        self.start_server(lambda _req: (200, result, 0))
        return {"state": "s", "questions": [choice()]}

    def test_missing_index(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"probabilities": [0.1, 0.2, 0.7]}),
        )

    def test_non_integer_index(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"index": "1", "probabilities": [0.1, 0.2, 0.7]}),
        )

    def test_boolean_index_is_rejected(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"index": True, "probabilities": [0.1, 0.2, 0.7]}),
        )

    def test_negative_index(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"index": -1, "probabilities": [0.1, 0.2, 0.7]}),
        )

    def test_index_past_end(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"index": 3, "probabilities": [0.1, 0.2, 0.7]}),
        )

    def test_probability_count_mismatch(self):
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            self._with_result({"index": 1, "probabilities": [0.5]}),
        )

    def test_response_is_not_an_object(self):
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, self._with_result([1, 2, 3])
        )


class ProbabilityValueTests(unittest.TestCase):
    def test_rejects_non_numeric(self):
        self.assert_error("x")

    def test_rejects_out_of_range(self):
        self.assert_error(1.5)

    def test_rejects_negative(self):
        self.assert_error(-0.1)

    def test_rejects_nan_and_inf(self):
        self.assert_error(float("nan"))
        self.assert_error(float("inf"))

    def test_accepts_bounds(self):
        self.assertEqual(
            http_bridge.require_number_list([0.0, 1.0], 2, "probabilities"), [0.0, 1.0]
        )

    def assert_error(self, value):
        with self.assertRaises(http_bridge.BridgeError) as ctx:
            http_bridge.require_number_list([value, 0.5], 2, "probabilities")
        self.assertEqual(ctx.exception.kind, "malformed")


class InputValidationTests(BridgeTestCase):
    def test_unsupported_type_sends_no_request(self):
        server = self.start_server(lambda _req: (200, {"index": 0}, 0))
        q = {"id": "q", "type": "ranking", "text": "t", "options": ["a", "b"]}
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": [q]}
        )
        self.assertEqual(server.requests, [])

    def test_missing_question_id(self):
        self.start_server(lambda _req: (200, {"index": 0}, 0))
        q = {"type": "choice", "text": "t", "options": ["a", "b"]}
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": [q]}
        )

    def test_question_not_an_object(self):
        self.start_server(lambda _req: (200, {"index": 0}, 0))
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": ["nope"]}
        )

    def test_duplicate_question_id(self):
        self.start_server(
            lambda _req: (200, {"index": 0, "probabilities": [1.0, 0.0, 0.0]}, 0)
        )
        self.assert_bridge_error(
            "malformed",
            http_bridge.build_output,
            {"state": "s", "questions": [choice("dup"), choice("dup")]},
        )

    def test_missing_question_text(self):
        self.start_server(lambda _req: (200, {"index": 0}, 0))
        q = {"id": "q", "type": "choice", "options": ["a", "b"]}
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": [q]}
        )

    def test_missing_options(self):
        self.start_server(lambda _req: (200, {"index": 0}, 0))
        q = {"id": "q", "type": "choice", "text": "t"}
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": [q]}
        )

    def test_questions_not_an_array(self):
        self.assert_bridge_error(
            "malformed", http_bridge.build_output, {"state": "s", "questions": "nope"}
        )

    def test_request_not_an_object(self):
        self.assert_bridge_error("malformed", http_bridge.build_output, [])


class MainTests(unittest.TestCase):
    def run_main(self, stdin_text):
        saved_in, saved_out = sys.stdin, sys.stdout
        sys.stdin = io.StringIO(stdin_text)
        sys.stdout = io.StringIO()
        try:
            code = http_bridge.main()
            out = sys.stdout.getvalue()
        finally:
            sys.stdin, sys.stdout = saved_in, saved_out
        return code, out

    def test_malformed_stdin_emits_error_envelope(self):
        code, out = self.run_main("this is not json")
        self.assertEqual(code, 1)
        envelope = json.loads(out)
        self.assertEqual(envelope["error"]["kind"], "malformed")
        self.assertIn("invalid input JSON", envelope["error"]["message"])

    def test_non_object_stdin_is_malformed(self):
        code, out = self.run_main("[1, 2, 3]")
        self.assertEqual(code, 1)
        self.assertEqual(json.loads(out)["error"]["kind"], "malformed")

    def test_error_envelope_shape(self):
        envelope = http_bridge.error_envelope("unavailable", "boom")
        self.assertEqual(
            envelope, {"error": {"kind": "unavailable", "message": "boom"}}
        )


if __name__ == "__main__":
    unittest.main()
