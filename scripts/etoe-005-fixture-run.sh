#!/usr/bin/env bash
#
# ETOE-005 — disposable dogfood fixture runner.
#
# Executes fixture task(s) from a fixture root created by
# scripts/etoe-005-fixture-setup.sh, driving SOP with a CONTROLLED, OFFLINE agent
# (scripts/etoe-005-fixture-agent.sh via SOP_AGENT_HARNESS=command /
# SOP_AGENT_PROVIDER=command / SOP_AGENT_COMMAND). No external model service is
# used, so the baseline is deterministic.
#
# The runner ALWAYS `cd`s into the given fixture root before invoking `sop`, so
# every SOP command executes against the disposable project's own state database
# and can never discover or execute the active production plan.
#
# Usage:
#   scripts/etoe-005-fixture-run.sh [FIXTURE_ROOT] [SOP_BINARY]
#
# Environment:
#   ETOE005_FIXTURE_ROOT  overrides FIXTURE_ROOT (default: ${TMPDIR:-/tmp}/etoe-005-fixture)
#   ETOE005_SOP_BINARY    overrides SOP_BINARY    (default: sop on PATH)
#   ETOE005_RECORDS       overrides the records path (default: <root>/etoe-005-run-records.txt)
#   ETOE005_SCENARIO      all (default) | success | fail | gate
#                         Run one scenario per disposable project. `success`
#                         requires the always-failing test to be DISABLED, `fail`
#                         requires it ENABLED; the runner enforces this per mode.
#
# Evidence: for each task the script records the run id, the process exit code,
# and STRUCTURED values parsed from the run's JSON artifacts (state.json stage,
# validation.json status) plus the SOP approval listing — never a substring match
# against a raw fragment. It fails (non-zero) when an expected run directory or
# artifact is missing, so a broken run cannot masquerade as a passing baseline.

set -euo pipefail

BASE_ROOT="$(cd "${TMPDIR:-/tmp}" 2>/dev/null && pwd -P || echo /tmp)"

FIXTURE_ROOT="${1:-${ETOE005_FIXTURE_ROOT:-${BASE_ROOT}/etoe-005-fixture}}"
SOP_BINARY="${2:-${ETOE005_SOP_BINARY:-sop}}"
SCENARIO="${ETOE005_SCENARIO:-all}"

# Resolve this script's real path (follow a symlink chain) to locate the agent.
self_path="$0"
while [ -h "$self_path" ]; do
  link="$(ls -ld -- "$self_path" | sed 's/.* -> //')"
  case "$link" in
    /*) self_path="$link" ;;
    *) self_path="$(dirname "$self_path")/$link" ;;
  esac
done
SELF_DIR="$(cd "$(dirname "$self_path")" && pwd -P)"
AGENT_SCRIPT="$SELF_DIR/etoe-005-fixture-agent.sh"

_resolve() {
  local p="$1" rest=""
  while [ ! -d "$p" ]; do
    rest="/$(basename "$p")${rest}"
    p="$(dirname "$p")"
    case "$p" in /) break ;; esac
  done
  printf '%s%s' "$(cd "$p" && pwd -P)" "$rest"
}
FIXTURE_ROOT="$(_resolve "$FIXTURE_ROOT")"

case "$FIXTURE_ROOT" in
  *..*) echo "run: refusing path containing '..': $FIXTURE_ROOT" >&2; exit 1 ;;
  "$BASE_ROOT")
    echo "run: refusing to use the disposable base root itself: $FIXTURE_ROOT" >&2
    exit 1
    ;;
  "$BASE_ROOT"/*) : ;;
  *)
    echo "run: refusing fixture root outside the disposable base root ($BASE_ROOT): $FIXTURE_ROOT" >&2
    exit 1
    ;;
esac

if [ ! -d "$FIXTURE_ROOT/.agent-sdlc" ]; then
  echo "run: fixture root is not an initialized SOP project: $FIXTURE_ROOT" >&2
  echo "run: run scripts/etoe-005-fixture-setup.sh first" >&2
  exit 1
fi
command -v "$SOP_BINARY" >/dev/null 2>&1 || { echo "run: sop binary not found: $SOP_BINARY" >&2; exit 1; }
SOP_PATH="$(command -v "$SOP_BINARY")"
command -v python3 >/dev/null 2>&1 || { echo "run: python3 is required for structured evidence parsing" >&2; exit 1; }
[ -f "$AGENT_SCRIPT" ] || { echo "run: controlled agent missing: $AGENT_SCRIPT" >&2; exit 1; }

# Verify the CLI surface before use. Use `sop help` (which never executes a plan)
# from the disposable base root, so a probe can never launch a run against a real
# project: a bare `sop run` with no arguments would execute the ACTIVE plan.
help_out="$(cd "$BASE_ROOT" && "$SOP_PATH" help 2>&1 || true)"
case "$help_out" in
  *--task*) : ;;
  *) echo "run: '$SOP_PATH' does not advertise a --task run surface" >&2
     echo "run: use the source build: go build -o /tmp/sop-etoe005 ./cmd/sop" >&2
     exit 1 ;;
esac

RECORDS="${ETOE005_RECORDS:-$FIXTURE_ROOT/etoe-005-run-records.txt}"
cd "$FIXTURE_ROOT"
RUNS_DIR=".agent-sdlc/runs"

snapshot_run_dirs() { # always exits 0 so `set -e` never aborts on an empty snapshot
  if [ -d "$RUNS_DIR" ]; then ls -1 "$RUNS_DIR" 2>/dev/null | sort || true; fi
  return 0
}

new_run_id() { # prints exactly one new TASK run dir name, else fails the caller
  local before="$1" after added count
  after="$(snapshot_run_dirs)"
  # A `sop run PLAN.md` also writes a reserved "plan" run dir for plan
  # compilation; it is not a task run, so it is excluded from attribution.
  added="$(comm -13 <(printf '%s\n' "$before") <(printf '%s\n' "$after") | sed '/^$/d' | grep -vx 'plan' || true)"
  count="$(printf '%s\n' "$added" | sed '/^$/d' | wc -l | tr -d ' ')"
  if [ "$count" -ne 1 ]; then
    echo "run: EXPECTED exactly one new task run dir, got $count: ${added:-<none>}" >&2
    return 1
  fi
  printf '%s' "$added"
}

summarize() {
  local id="$1"
  python3 - "$RUNS_DIR/$id" <<'PY'
import json, os, sys
d = sys.argv[1]
def load(p):
    try:
        with open(p) as f: return json.load(f)
    except Exception: return None
state = load(os.path.join(d, "state.json")) or {}
val = load(os.path.join(d, "validation.json")) or {}
rep = load(os.path.join(d, "report.json")) or {}
print("stage=%s validation=%s gate=%s" % (
    state.get("stage", "UNAVAILABLE"),
    val.get("Status", "UNAVAILABLE"),
    rep.get("Gate") or rep.get("Decision") or "UNAVAILABLE"))
PY
}

record() { printf '%s\n' "$*" | tee -a "$RECORDS"; }

overall=0

run_task() { # run_task LABEL MODE EXPECT [RUN_ARGS...]
  local taskfile="$1" mode="$2" expect="$3" before id rc summary verdict
  shift 3
  before="$(snapshot_run_dirs)"
  set +e
  SOP_AGENT_HARNESS=command SOP_AGENT_PROVIDER=command \
    SOP_AGENT_COMMAND="bash '$AGENT_SCRIPT' $mode" \
    "$SOP_PATH" run "$@"
  rc=$?
  set -e
  if ! id="$(new_run_id "$before")"; then
    record "RESULT $taskfile | run-id UNAVAILABLE | exit=$rc | expected=$expect | verdict=MISSING-ARTIFACT"
    overall=1
    return 0
  fi
  if [ ! -f "$RUNS_DIR/$id/state.json" ]; then
    record "RESULT $taskfile | run-id $id | exit=$rc | expected=$expect | verdict=MISSING-ARTIFACT (no state.json)"
    overall=1
    return 0
  fi
  summary="$(summarize "$id")"
  verdict="$(python3 - "$expect" "$summary" "$rc" <<'PY'
import sys
expect, summary, rc = sys.argv[1], sys.argv[2], int(sys.argv[3])
kv = dict(p.split("=", 1) for p in summary.split() if "=" in p)
stage = kv.get("stage", "UNAVAILABLE")
if expect == "completed":
    ok = (rc == 0 and stage == "PASSED")
elif expect == "not-completed":
    ok = (stage != "PASSED")
elif expect == "gated":
    ok = (stage in ("WAITING_FOR_HUMAN", "FAILED"))
else:
    ok = False
print("MATCH" if ok else "MISMATCH")
PY
)"
  [ "$verdict" = "MATCH" ] || overall=1
  record "RESULT $taskfile | run-id $id | exit=$rc | expected=$expect | $summary | verdict=$verdict"
}

echo "run: fixture root = $FIXTURE_ROOT"
echo "run: sop binary   = $SOP_PATH"
echo "run: agent        = $AGENT_SCRIPT"
echo "run: scenario     = $SCENARIO"
echo "run: records      = $RECORDS"

: > "$RECORDS"
record "# ETOE-005 fixture run records (structured evidence)"
record "# binary: $SOP_PATH"
record "# agent:  $AGENT_SCRIPT (controlled, offline; SOP_AGENT_HARNESS=command)"
record "# root:   $FIXTURE_ROOT"
record "# scenario: $SCENARIO"
record "# git-baseline: $(git -C "$FIXTURE_ROOT" rev-parse --short HEAD 2>/dev/null || echo UNAVAILABLE)"

do_success() {
  if [ -e broken_test.go ]; then mv broken_test.go broken_test.go.disabled; fi
  echo "run: executing FIX-SUCCESS (mode=success)"
  run_task tasks/FIX-SUCCESS.md success completed --task tasks/FIX-SUCCESS.md
}
do_fail() {
  if [ -e broken_test.go.disabled ]; then mv -f broken_test.go.disabled broken_test.go; fi
  echo "run: executing FIX-FAIL (mode=fail; always-failing test enabled)"
  run_task tasks/FIX-FAIL.md fail not-completed --task tasks/FIX-FAIL.md
  record "# --- validation.json (FIX-FAIL) ---"
  cat "$RUNS_DIR"/$(ls -1 "$RUNS_DIR" | tail -1)/validation.json >>"$RECORDS" 2>/dev/null || record "validation.json: UNAVAILABLE"
}
do_gate() {
  echo "run: executing FIX-GATE (mode=gate; plan-based so the gate is persisted)"
  run_task docs/PLAN.md gate gated docs/PLAN.md
  record "# --- sop approvals --json (after FIX-GATE) ---"
  "$SOP_PATH" approvals --json >>"$RECORDS" 2>/dev/null || record "approvals: UNAVAILABLE"
  grep -q 'PENDING' "$RECORDS" && record "GATE-EVIDENCE: a PENDING approval is present" || { record "GATE-EVIDENCE: NO pending approval found"; overall=1; }
}

case "$SCENARIO" in
  all) do_success; do_fail; do_gate ;;
  success) do_success ;;
  fail) do_fail ;;
  gate) do_gate ;;
  *) echo "run: unknown ETOE005_SCENARIO '$SCENARIO' (want all|success|fail|gate)" >&2; exit 2 ;;
esac

record "# --- machine-readable summary (JSONL) ---"
python3 - "$RECORDS" <<'PY' >>"$RECORDS"
import json, sys, re
for line in open(sys.argv[1]):
    m = re.match(r"RESULT (\S+) \| run-id (\S+) \| exit=(\d+) \| expected=(\S+) \| (.*) \| verdict=(\S+)", line)
    if m:
        task, rid, rc, exp, summary, verdict = m.groups()
        kv = dict(p.split("=", 1) for p in summary.split() if "=" in p)
        kv.update({"task": task, "run_id": rid, "exit": int(rc), "expected": exp, "verdict": verdict})
        print(json.dumps(kv, sort_keys=True))
PY

echo "run: records written to $RECORDS"
if [ "$overall" -ne 0 ]; then
  echo "run: one or more scenarios did not MATCH their expected outcome (see $RECORDS)" >&2
  exit 1
fi
echo "run: scenario(s) MATCHED expected outcomes"
