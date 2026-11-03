#!/usr/bin/env bash
# acp-linux-smoke.sh — end-to-end smoke of the ACP stdio endpoint.
#
# Builds `sop` from this checkout into a private temporary directory, initializes
# a disposable project, and drives `sop acp` over newline-delimited JSON-RPC the
# way an ACP client (for example Zed) does. It asserts:
#
#   initialize    the numeric protocolVersion alias is accepted and answered with
#                 the pinned version; an incompatible version is refused
#   ordering      a session method before a successful initialize is refused
#   capabilities  session/load is refused as unadvertised
#   robustness    a malformed frame is answered and the stream survives; an
#                 unknown method is refused
#   submit        session/prompt starts exactly one governed job
#   cancel        session/cancel is answered only once the persisted record says
#                 cancelled, the record keeps cancelled.json distinct from the
#                 stopped child's exit record, and the child is gone
#   notify        the id-less session/cancel notification gets no response
#   eof           a clean EOF exits 0
#
# A blocking command harness (SOP_AGENT_COMMAND) keeps the started child alive,
# so the check needs no model provider and no network. Nothing outside the
# private temporary directory is modified.
#
# Requires: go, python3.
# Usage: scripts/checks/acp-linux-smoke.sh [--keep]

set -euo pipefail

repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

keep=0
case "${1:-}" in
  "") ;;
  --keep) keep=1 ;;
  -h|--help)
    sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'
    exit 0
    ;;
  *)
    echo "usage: $0 [--keep]" >&2
    exit 2
    ;;
esac

for tool in go python3; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "acp-smoke: $tool is required" >&2
    exit 2
  fi
done

tmp="$(mktemp -d "${TMPDIR:-/tmp}/sop-acp-smoke.XXXXXX")"
cleanup() {
  if [ "$keep" = "1" ]; then
    echo "acp-smoke: kept $tmp" >&2
  else
    rm -rf "$tmp"
  fi
}
trap cleanup EXIT INT TERM

echo "acp-smoke: building sop" >&2
( cd "$repo" && go build -o "$tmp/sop" ./cmd/sop )

project="$tmp/project"
mkdir -p "$project"

# A blocking harness keeps the governed child alive until the cancel request, so
# the cancellation assertions are deterministic and provider-independent. An
# operator-supplied SOP_AGENT_COMMAND still wins.
export SOP_AGENT_COMMAND="${SOP_AGENT_COMMAND:-sleep 60}"

SOP_BIN="$tmp/sop" ACP_PROJECT="$project" python3 - <<'PY'
import json
import os
import queue
import subprocess
import sys
import threading
import time

SOP = os.environ["SOP_BIN"]
PROJECT = os.environ["ACP_PROJECT"]

results = []


def check(name, ok, detail=""):
    results.append((name, ok))
    print(("PASS " if ok else "FAIL ") + name + ((" — " + detail) if detail else ""))


def fail_now(message):
    print("FAIL " + message, file=sys.stderr)
    sys.exit(1)


init = subprocess.run([SOP, "init"], cwd=PROJECT, capture_output=True, text=True)
if init.returncode != 0:
    fail_now("sop init: " + init.stderr.strip()[:400])


class Client:
    """One `sop acp` process with a background stdout reader."""

    def __init__(self):
        self.p = subprocess.Popen(
            [SOP, "acp"],
            cwd=PROJECT,
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )
        self.frames = queue.Queue()
        threading.Thread(target=self._read, daemon=True).start()

    def _read(self):
        for line in self.p.stdout:
            self.frames.put(line)

    def send(self, obj):
        self.p.stdin.write(json.dumps(obj) + "\n")
        self.p.stdin.flush()

    def recv(self, timeout=30):
        try:
            return json.loads(self.frames.get(timeout=timeout))
        except queue.Empty:
            return None

    def recv_response(self, want_id, timeout=30):
        """Return the response with want_id, skipping outbound notifications."""
        deadline = time.time() + timeout
        while time.time() < deadline:
            msg = self.recv(timeout=max(0.1, deadline - time.time()))
            if msg is None:
                return None
            if msg.get("id") == want_id:
                return msg
        return None

    def no_response(self, timeout=0.5):
        try:
            self.frames.get(timeout=timeout)
            return False
        except queue.Empty:
            return True

    def close(self):
        try:
            self.p.stdin.close()
        except Exception:
            pass
        try:
            return self.p.wait(timeout=60)
        except subprocess.TimeoutExpired:
            self.p.kill()
            return None


client = Client()
job_id = ""
job_dir = ""

try:
    client.send({"jsonrpc": "2.0", "id": 1, "method": "initialize",
                 "params": {"protocolVersion": 1}})
    r = client.recv_response(1)
    result = (r or {}).get("result", {})
    check("initialize accepts the numeric protocolVersion alias",
          result.get("protocolVersion") in ("v1", 1),
          json.dumps(r)[:200] if r else "no response")
    check("initialize advertises the pinned protocol v1",
          result.get("protocolVersion") in ("v1", 1) and
          result.get("agentInfo", {}).get("name") == "sop",
          json.dumps(result.get("agentInfo")) if r else "")

    client.send({"jsonrpc": "2.0", "id": 2, "method": "session/new",
                 "params": {"cwd": PROJECT}})
    r = client.recv_response(2)
    session = (r or {}).get("result", {}).get("sessionId", "")
    check("session/new returns a sessionId", bool(session), session)

    client.send({"jsonrpc": "2.0", "id": 3, "method": "session/load",
                 "params": {"sessionId": session}})
    r = client.recv_response(3)
    check("session/load is refused as an unadvertised capability",
          bool(r and r.get("error", {}).get("code") == -32001),
          json.dumps(r)[:180] if r else "no response")

    client.send({"jsonrpc": "2.0", "id": 4, "method": "no/such-method",
                 "params": {}})
    r = client.recv_response(4)
    check("an unknown method is refused", bool(r and r.get("error", {}).get("code") == -32601),
          json.dumps(r)[:180] if r else "no response")

    client.p.stdin.write("this is not json\n")
    client.p.stdin.flush()
    r = client.recv()
    check("a malformed frame is answered (-32700) and the stream survives",
          bool(r and r.get("error", {}).get("code") == -32700),
          json.dumps(r)[:160] if r else "no response")

    client.send({"jsonrpc": "2.0", "method": "session/cancel",
                 "params": {"sessionId": session}})
    check("the id-less session/cancel notification gets no response",
          client.no_response())

    client.send({"jsonrpc": "2.0", "id": 5, "method": "session/prompt",
                 "params": {"sessionId": session,
                            "prompt": [{"type": "text", "text": "smoke test"}],
                            "submissionKey": "acp-smoke-1"}})
    r = client.recv_response(5)
    result = (r or {}).get("result", {})
    job_id = result.get("jobId", "")
    check("session/prompt starts exactly one governed job",
          result.get("outcome") == "started" and bool(job_id),
          json.dumps(r)[:220] if r else "no response")

    client.send({"jsonrpc": "2.0", "id": 6, "method": "session/cancel",
                 "params": {"sessionId": session}})
    r = client.recv_response(6)
    result = (r or {}).get("result", {})
    check("session/cancel reports a confirmed cancellation",
          result.get("outcome") == "cancelled" and
          result.get("stopReason") == "cancelled" and
          result.get("jobState") == "cancelled",
          json.dumps(r)[:260] if r else "no response")
    check("the cancellation carries the started job identity",
          bool(job_id) and result.get("jobId") == job_id,
          "jobId=%s" % result.get("jobId"))

    code = client.close()
    check("a clean EOF exits 0", code == 0, "exit=%s" % code)

    job_dir = os.path.join(PROJECT, ".agent-sdlc", "runs", "mcp-work", job_id)
    files = sorted(os.listdir(job_dir)) if os.path.isdir(job_dir) else []
    check("the job record and child log are persisted",
          "job.json" in files and "output.log" in files, ",".join(files))
    check("the cancellation is recorded distinctly from the stopped child's exit",
          "cancelled.json" in files and "exit.json" in files, ",".join(files))

    record = {}
    if "job.json" in files:
        with open(os.path.join(job_dir, "job.json")) as fh:
            record = json.load(fh)
    check("the job record carries its pid and work fingerprint",
          bool(record.get("pid")) and bool(record.get("fingerprint")),
          "pid=%s fingerprint=%s" % (record.get("pid"), str(record.get("fingerprint"))[:12]))

    pid = record.get("pid") or 0
    gone = False
    if pid:
        try:
            os.kill(pid, 0)
        except ProcessLookupError:
            gone = True
    check("the cancelled child is gone (no orphan)", gone, "pid=%s" % pid)

    # A fresh server must refuse an incompatible version and enforce ordering.
    other = Client()
    try:
        other.send({"jsonrpc": "2.0", "id": 1, "method": "initialize",
                    "params": {"protocolVersion": "v2"}})
        r = other.recv_response(1)
        message = (r or {}).get("error", {}).get("message", "")
        check("an incompatible protocol version is refused, naming v1",
              bool(r and r.get("error", {}).get("code") == -32000 and "v1" in message),
              json.dumps(r)[:200] if r else "no response")

        other.send({"jsonrpc": "2.0", "id": 2, "method": "session/new",
                    "params": {"cwd": PROJECT}})
        r = other.recv_response(2)
        check("session/new before a successful initialize is refused (-32002)",
              bool(r and r.get("error", {}).get("code") == -32002),
              json.dumps(r)[:160] if r else "no response")
    finally:
        other.close()
finally:
    try:
        client.close()
    except Exception:
        pass

passed = sum(1 for _, ok in results if ok)
print()
print("%d/%d checks passed" % (passed, len(results)))
if job_dir:
    print("job record: " + job_dir)
sys.exit(0 if passed == len(results) else 1)
PY

echo "acp-smoke: passed" >&2
