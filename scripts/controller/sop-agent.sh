#!/bin/sh
# SOP command-agent adapter for this project.
#
# Architecture (agentic-sop/README.md → "Command agent outcomes"):
#
#   agentic-sop  --(structured command-agent protocol: JSON on stdin)-->
#   sop-agent.sh --(implementation agent)-->
#   implementation agent
#
# SOP remains the workflow authority. This adapter is deliberately thin: it
# forwards the request to an implementation agent and returns the outcome. It
# contains no scheduling, dependency, retry, validation, review, or task-state
# logic — none of that belongs here.
#
# stdin (JSON): {"capability","task","input","output_requirements"}
# stdout:
#   PLAN / REVIEW                    -> the structured JSON SOP parses
#   IMPLEMENT / FIX / DESIGN_TESTS   -> a structured execution outcome (exactly):
#       {"status":"completed","summary":"…","changes_expected":true|false}
#       {"status":"needs_human","reason":"…"}
#       {"status":"failed","reason":"…"}
#   other                            -> prose
#
# The implementation agent is pluggable and is NOT part of the SOP protocol:
#   SOP_AGENT_IMPL   optional executable, given the prompt as its single
#                    argument and printing the agent output on stdout.
#                    Defaults to Claude Code.
#
set -eu

request=$(mktemp)
trap 'rm -f "$request"' EXIT
cat > "$request"

capability=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("capability",""))' "$request")

prompt=$(python3 - "$request" <<'PY'
import json, sys

r = json.load(open(sys.argv[1]))
cap = r.get("capability", "")
parts = [
    "You are the implementation agent running inside an autonomous SOP "
    "lifecycle for this repository. SOP is the workflow authority; you perform "
    "work and report what happened. Work autonomously, make reasonable "
    "decisions, and do not ask the user questions.",
    "Capability: %s" % cap,
]
if r.get("task", "").strip():
    parts.append("Task:\n" + r["task"])
if r.get("input", "").strip():
    parts.append("Input:\n" + r["input"])
if r.get("output_requirements", "").strip():
    parts.append("Output requirements:\n" + r["output_requirements"])

if cap in ("PLAN", "REVIEW"):
    parts.append(
        "Respond with ONLY the requested output, in exactly the required format "
        "(valid JSON when a JSON schema is given). Do not add commentary, "
        "explanations, or markdown code fences, and do not create or edit files.")
elif cap in ("IMPLEMENT", "FIX", "DESIGN_TESTS"):
    parts.append(
        "Perform the requested work:\n"
        "1. Make the change the task requires; edit or create files directly.\n"
        "2. Run reasonable repository-local verification commands (for example "
        "the configured build/test/lint) only when needed to complete the task.\n"
        "3. NEVER commit, push, merge, reset, or run other destructive Git "
        "operations.\n"
        "4. Do not decide workflow state; report an outcome for SOP to interpret.\n"
        "When finished, end your reply with exactly one line of this form:\n"
        'SOP_OUTCOME:{"status":"completed","summary":"<one line>","changes_expected":true}\n'
        "Use changes_expected=false for legitimate verification or read-only "
        "tasks that make no repository change (for example a baseline check). "
        "Use status \"needs_human\" (with a reason) only when you cannot continue "
        "without a human decision, and status \"failed\" (with a reason) when the "
        "work cannot be completed.")
else:
    parts.append("Return a concise, direct answer. Do not edit any files.")

print("\n\n".join(parts))
PY
)

# The implementation agent, kept behind one seam so it can be replaced without
# touching SOP semantics. $1 is the prompt; stdout is the agent's output.
run_impl() {
  if [ -n "${SOP_AGENT_IMPL:-}" ]; then
    "$SOP_AGENT_IMPL" "$1"
  else
    claude -p "$1" --dangerously-skip-permissions
  fi
}

# Non-SOP working-tree changes, so changes_expected reflects reality. SOP ignores
# its own .agent-sdlc/ output.
snapshot() {
  git status --porcelain --untracked-files=all 2>/dev/null | grep -v '\.agent-sdlc/' || true
}

# extract_first_json prints the first JSON object embedded in stdin (skipping any
# surrounding prose or code fences) and exits 0; it exits 1 when there is none. It
# replaces a greedy brace match, so a response that merely mentions braces in prose
# can never be mistaken for the document.
extract_first_json() {
  python3 -c '
import json, sys
s = sys.stdin.read()
dec = json.JSONDecoder()
i = 0
while True:
    j = s.find("{", i)
    if j == -1:
        sys.exit(1)
    try:
        obj, _ = dec.raw_decode(s[j:])
    except Exception:
        i = j + 1
        continue
    if isinstance(obj, dict):
        sys.stdout.write(json.dumps(obj))
        sys.exit(0)
    i = j + 1
'
}

# run_document returns the JSON document a document-producing capability requires.
# The agent is asked for JSON only, but a model may still answer in prose, so a
# response without a JSON object is re-requested with a corrective instruction. The
# adapter knows only that a JSON object is required; the exact shape lives in the
# request and is interpreted by SOP, so no schema is embedded here.
run_document() {
  attempt=1
  while :; do
    if [ "$attempt" -eq 1 ]; then
      ask=$1
    else
      ask="$1

Your previous response contained no JSON object. Respond with ONLY the required JSON object, with no prose, no markdown, and no code fences."
    fi
    raw=$(run_impl "$ask")
    if json=$(printf '%s' "$raw" | extract_first_json); then
      printf '%s' "$json"
      return 0
    fi
    attempt=$((attempt + 1))
    if [ "$attempt" -gt 3 ]; then
      printf '%s' "$raw"
      return 0
    fi
  done
}

case "$capability" in
  PLAN|REVIEW)
    run_document "$prompt"
    ;;

  IMPLEMENT|FIX|DESIGN_TESTS)
    before=$(snapshot)
    rc=0
    out=$(run_impl "$prompt") || rc=$?
    after=$(snapshot)
    changed=false
    if [ "$before" != "$after" ]; then changed=true; fi
    printf '%s' "$out" | python3 -c '
import sys, json

changed = sys.argv[1] == "true"
rc = int(sys.argv[2])
text = sys.stdin.read()

# Prefer the explicit, machine-readable outcome from the implementation agent.
# This is structural parsing, never inference from loose prose.
marker = "SOP_OUTCOME:"
i = text.rfind(marker)
if i != -1:
    j = text.find("{", i + len(marker))
    obj = None
    if j != -1:
        try:
            obj, _ = json.JSONDecoder().raw_decode(text[j:])
        except Exception:
            obj = None
    if isinstance(obj, dict):
        status = obj.get("status")
        summary = str(obj.get("summary") or "").strip()
        reason = str(obj.get("reason") or "").strip()
        if status == "completed":
            ce = obj.get("changes_expected")
            if not isinstance(ce, bool):
                ce = changed
            print(json.dumps({"status": "completed",
                              "summary": summary or "completed",
                              "changes_expected": ce}))
            sys.exit(0)
        if status == "needs_human":
            print(json.dumps({"status": "needs_human",
                              "reason": reason or "human decision required"}))
            sys.exit(0)
        if status == "failed":
            print(json.dumps({"status": "failed",
                              "reason": reason or "operation failed"}))
            sys.exit(0)
    # A declared outcome that is not recognizable is malformed: fail, never
    # invent success.
    print(json.dumps({"status": "failed",
                      "reason": "malformed execution outcome from implementation agent"}))
    sys.exit(0)

# No structured outcome was declared. Derive conservatively from reality (the
# exit status and the working tree), never from prose.
if rc != 0:
    print(json.dumps({"status": "failed",
                      "reason": "implementation agent exited with status %d" % rc}))
else:
    print(json.dumps({"status": "completed",
                      "summary": "implementation agent completed without a structured outcome",
                      "changes_expected": changed}))
' "$changed" "$rc"
    ;;

  *)
    run_impl "$prompt"
    ;;
esac
