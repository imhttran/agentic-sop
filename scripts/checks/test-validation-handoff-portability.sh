#!/usr/bin/env bash
#
# Cross-agent portability tests for the MCP live-validation contract.
#
# These tests are simulated: they use a temporary checkout, a fake candidate,
# and a canary process to exercise the handoff and validator. They do not claim
# a live ChatGPT, Claude, or DeepSeek integration. They prove that the artifact
# and verifier support the Claude -> ChatGPT -> DeepSeek role transitions and
# reject invalid state without touching a process or production binary.
#
# Usage: scripts/checks/test-validation-handoff-portability.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd -P)"
HELPER="$REPO_ROOT/scripts/planner-tunnel-candidate.sh"
VALIDATOR="$REPO_ROOT/scripts/validation-handoff.py"
SCHEMA="$REPO_ROOT/docs/reference/validation-handoff.schema.json"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/sop-handoff-portability.XXXXXX")"
ROOT="$TMP/repo"
CANARY_PID=""
FAILS=0

cleanup() {
  if [ -n "$CANARY_PID" ] && kill -0 "$CANARY_PID" 2>/dev/null; then
    kill -TERM "$CANARY_PID" 2>/dev/null || true
  fi
  rm -rf "$TMP"
}
trap cleanup EXIT

fail() { printf 'FAIL: %s\n' "$*" >&2; FAILS=$((FAILS + 1)); }
pass() { printf 'ok   %s\n' "$*"; }

assert_eq() { [ "$1" = "$2" ] || fail "expected '$1' = '$2'"; }
assert_contains() {
  case "$1" in
    *"$2"*) : ;;
    *) fail "expected output to contain '$2'; got:
$1" ;;
  esac
}
assert_valid() {
  if ! python3 "$VALIDATOR" --handoff "$1" --checkout "$ROOT" --schema "$SCHEMA" --quiet; then
    fail "expected valid handoff: $1"
  fi
}
assert_invalid() {
  if python3 "$VALIDATOR" --handoff "$1" --checkout "$ROOT" --schema "$SCHEMA" >"$TMP/validator.out" 2>&1; then
    fail "expected invalid handoff: $1"
  else
    assert_contains "$(cat "$TMP/validator.out")" "INVALID"
  fi
}

json_get() {
  python3 -c 'import json,sys
d=json.load(open(sys.argv[1]))
for key in sys.argv[2].split("."):
    d = d[int(key)] if isinstance(d, list) else d[key]
print(d)' "$1" "$2"
}

sha() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}';
  else sha256sum "$1" | awk '{print $1}'; fi
}

file_mtime() {
  # GNU stat uses -c, BSD/macOS stat uses -f. Probe the GNU form first: on Linux
  # `stat -f` succeeds and prints filesystem statistics instead of an error.
  if stat -c %Y "$1" >/dev/null 2>&1; then
    stat -c %Y "$1"
  else
    stat -f %m "$1"
  fi
}

write_results() {
  local path="$1" status="$2" candidate_sha="$3" source_rev="$4" workflow_id="$5" validator="$6"
  python3 - "$path" "$status" "$candidate_sha" "$source_rev" "$workflow_id" "$validator" <<'PY'
import datetime, json, sys
path, status, candidate_sha, source_rev, workflow_id, validator = sys.argv[1:]
calls = []
failed = 0
for round_no in range(1, 11):
    for tool in ("sop_status", "sop_question_get"):
        call_status = "PASS"
        if status == "FAIL" and failed == 0 and round_no == 4 and tool == "sop_question_get":
            call_status = "FAIL"
            failed += 1
        calls.append({
            "round": round_no,
            "tool": tool,
            "arguments": {} if tool == "sop_status" else {"question_id": "q-1"},
            "status": call_status,
            "elapsed_ms": 9,
        })
now = datetime.datetime.now(datetime.timezone.utc).isoformat().replace("+00:00", "Z")
results = {
    "schema": "mcp-live-validation/validation-results/v1",
    "workflow_id": workflow_id,
    "validator": validator,
    "validator_capabilities": ["mcp_tools"],
    "candidate_sha256": candidate_sha,
    "source_revision": source_rev,
    "started_at": now,
    "finished_at": now,
    "rounds": 10,
    "concurrent": True,
    "retries": 0,
    "state_changes": False,
    "summary": {"total": len(calls), "passed": len(calls) - failed, "failed": failed},
    "calls": calls,
    "initialization_errors": [] if status == "PASS" else [{"round": 4, "error_type": "test-failure"}],
    "status": status,
}
with open(path, "w", encoding="utf-8") as fh:
    json.dump(results, fh, indent=2)
PY
}

# ---------------------------------------------------------------------------
# Fixture: a temporary git repository, a fake candidate, and a manifest.
# ---------------------------------------------------------------------------

command -v python3 >/dev/null 2>&1 || { echo "python3 is required" >&2; exit 1; }
mkdir -p "$ROOT/.run/tunnel/candidate"
git -C "$ROOT" init -q -b main
printf 'fixture\n' > "$ROOT/README.md"
git -C "$ROOT" add -A
git -C "$ROOT" -c user.name=test -c user.email=test@localhost commit -q -m fixture

CANDIDATE="$ROOT/candidate"
BASELINE="$ROOT/baseline"
printf '#!/bin/sh\nexit 0\n' > "$CANDIDATE"
printf '#!/bin/sh\nexit 0\n' > "$BASELINE"
chmod +x "$CANDIDATE" "$BASELINE"
CANDIDATE_SHA="$(sha "$CANDIDATE")"
BASELINE_SHA="$(sha "$BASELINE")"
SOURCE_REV="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
PATCH_SHA="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
WORKFLOW_ID="workflow-portability-test"
MANIFEST="$ROOT/.run/tunnel/candidate/manifest"

write_manifest() {
  local state="$1" pid="$2" command="$3"
  cat > "$MANIFEST" <<EOF
format=1
state=$state
workflow_id=$WORKFLOW_ID
candidate_path=$CANDIDATE
candidate_sha256=$CANDIDATE_SHA
candidate_version=0.0.1-test
candidate_revision=$SOURCE_REV
source_url=https://example.invalid/tunnel-client
patch=$ROOT/fix.patch
patch_sha256=$PATCH_SHA
baseline_path=$BASELINE
baseline_sha256=$BASELINE_SHA
baseline_version=0.0.0-test
profile=test-profile
candidate_log=$ROOT/candidate.log
candidate_tunnel_pid=$ROOT/tunnel.pid
process_pid=$pid
process_command=$command
wrapper_path=
wrapper_sha256=
wrapper_backup=
baseline_launch_arg=run
baseline_launch_arg=--profile
baseline_launch_arg=test-profile
candidate_launch_arg=run
candidate_launch_arg=--profile
candidate_launch_arg=test-profile
EOF
  : > "$ROOT/candidate.log"
  : > "$ROOT/tests.txt"
  printf 'go test ./...: pass\nmake test-planner-tunnel: pass\n' > "$ROOT/tests.txt"
}

run_handoff() {
  "$HELPER" handoff --checkout "$ROOT" --process-match no-such-tunnel-process "$@"
}

# ---------------------------------------------------------------------------
# Scenario A: Claude prepares and launches a candidate, then ChatGPT validates.
# ---------------------------------------------------------------------------

write_manifest prepared "" ""
if run_handoff --format json --agent claude --capabilities filesystem,process_execution \
  --local-status PASS --tests "$ROOT/tests.txt" --deployment-approval PENDING \
  --issue PORTABILITY-A --output "$ROOT/handoff-prepared.json" >"$TMP/out" 2>&1; then
  assert_valid "$ROOT/handoff-prepared.json"
  assert_eq "$(json_get "$ROOT/handoff-prepared.json" workflow.next_permitted_action)" "REQUEST_DEPLOYMENT_APPROVAL"
  assert_eq "$(json_get "$ROOT/handoff-prepared.json" environment.agent)" "claude"
  pass "scenario A: Claude prepares a candidate handoff that requests deployment approval"
else
  fail "scenario A prepared handoff failed: $(cat "$TMP/out")"
fi

# The default request is a template, not a hardcoded planner assumption.
cat > "$ROOT/request-custom.json" <<'EOF'
{
  "rounds": 5,
  "concurrent": true,
  "retries": 0,
  "state_changes": false,
  "record_initialization_errors": true,
  "total_calls": 5,
  "required_calls": [
    {"tool": "sop_status", "arguments": {}}
  ],
  "acceptance": {
    "pass": "all 5 calls succeed with no retries and no state change",
    "fail": "any call fails; record the exact error and matching debug-log lines",
    "caveat": "a clean run is supporting evidence, not proof an intermittent defect cannot recur"
  }
}
EOF
if run_handoff --format json --request "$ROOT/request-custom.json" \
  --agent claude --capabilities filesystem,process_execution \
  --local-status PASS --tests "$ROOT/tests.txt" --deployment-approval PENDING \
  --issue PORTABILITY-A --output "$ROOT/handoff-custom.json" >"$TMP/out" 2>&1; then
  assert_valid "$ROOT/handoff-custom.json"
  assert_eq "$(json_get "$ROOT/handoff-custom.json" request.rounds)" "5"
  assert_eq "$(json_get "$ROOT/handoff-custom.json" request.total_calls)" "5"
  assert_eq "$(json_get "$ROOT/handoff-custom.json" results_template.rounds)" "5"
  assert_eq "$(json_get "$ROOT/handoff-custom.json" request.required_calls.0.tool)" "sop_status"
  pass "scenario A: a custom MCP request replaces the planner template"
else
  fail "scenario A custom-request handoff failed: $(cat "$TMP/out")"
fi

sleep 60 &
CANARY_PID=$!
disown "$CANARY_PID" 2>/dev/null || true
sleep 0.2
CANARY_CMD="$(ps -p "$CANARY_PID" -o command= | sed 's/^[[:space:]]*//; s/[[:space:]]*$//')"
write_manifest candidate-running "$CANARY_PID" "$CANARY_CMD"

if run_handoff --format json --agent claude --capabilities filesystem,process_execution \
  --local-status PASS --tests "$ROOT/tests.txt" --deployment-approval APPROVED \
  --approval-reference "operator approval (test fixture)" \
  --issue PORTABILITY-A --output "$ROOT/handoff-running.json" >"$TMP/out" 2>&1; then
  assert_valid "$ROOT/handoff-running.json"
  assert_eq "$(json_get "$ROOT/handoff-running.json" workflow.next_permitted_action)" "RUN_INDEPENDENT_VALIDATION"
  pass "scenario A: Claude records the approved running candidate for independent validation"
else
  fail "scenario A running handoff failed: $(cat "$TMP/out")"
fi

WORKFLOW_ID="$(json_get "$ROOT/handoff-running.json" workflow_id)"
write_results "$ROOT/results-chatgpt.json" PASS "$CANDIDATE_SHA" "$SOURCE_REV" "$WORKFLOW_ID" chatgpt
if run_handoff --format json --agent claude --capabilities filesystem,process_execution \
  --local-status PASS --tests "$ROOT/tests.txt" --deployment-approval APPROVED \
  --approval-reference "operator approval (test fixture)" \
  --results-json "$ROOT/results-chatgpt.json" --validation-status PASS \
  --issue PORTABILITY-A --output "$ROOT/handoff-pass.json" >"$TMP/out" 2>&1; then
  assert_valid "$ROOT/handoff-pass.json"
  assert_eq "$(json_get "$ROOT/handoff-pass.json" independent_validation.validator)" "chatgpt"
  assert_eq "$(json_get "$ROOT/handoff-pass.json" independent_validation.results.candidate_sha256)" "$CANDIDATE_SHA"
  assert_eq "$(json_get "$ROOT/handoff-pass.json" workflow.next_permitted_action)" "PREPARE_PR"
  pass "scenario A: ChatGPT results are associated with the correct candidate and prepare the PR"
else
  fail "scenario A PASS handoff failed: $(cat "$TMP/out")"
fi

# ---------------------------------------------------------------------------
# Scenario B: a DeepSeek-compatible reviewer reads ChatGPT's evidence without
# rebuilding or redownloading the candidate.
# ---------------------------------------------------------------------------

CANDIDATE_MTIME_BEFORE="$(file_mtime "$CANDIDATE")"
MANIFEST_SHA_BEFORE="$(sha "$MANIFEST")"
if python3 "$VALIDATOR" --handoff "$ROOT/handoff-pass.json" --checkout "$ROOT" \
  --schema "$SCHEMA" --json >"$TMP/review.json" 2>"$TMP/review.err"; then
  assert_eq "$(json_get "$TMP/review.json" valid)" "True"
  assert_eq "$(json_get "$TMP/review.json" next_permitted_action)" "PREPARE_PR"
  assert_eq "$(json_get "$TMP/review.json" validation_status)" "PASS"
  assert_eq "$(json_get "$TMP/review.json" candidate_sha256)" "$CANDIDATE_SHA"
  CANDIDATE_MTIME_AFTER="$(file_mtime "$CANDIDATE")"
  assert_eq "$CANDIDATE_MTIME_AFTER" "$CANDIDATE_MTIME_BEFORE"
  assert_eq "$(sha "$MANIFEST")" "$MANIFEST_SHA_BEFORE"
  [ ! -d "$ROOT/.run/tunnel/build" ] || fail "reviewer created a build directory"
  pass "scenario B: a second agent resumes from evidence without rebuilding or redownloading"
else
  fail "scenario B review failed: $(cat "$TMP/review.err")"
fi

# ---------------------------------------------------------------------------
# Scenario C: invalid handoffs are rejected without touching the candidate,
# baseline, or running process.
# ---------------------------------------------------------------------------

make_variant() {
  local name="$1" expression="$2"
  python3 - "$ROOT/handoff-pass.json" "$ROOT/variant-$name.json" "$expression" <<'PY'
import json, sys
source, target, expression = sys.argv[1:]
data = json.load(open(source))
exec(expression, {"d": data})
with open(target, "w", encoding="utf-8") as fh:
    json.dump(data, fh, indent=2)
PY
}

CANDIDATE_SHA_BEFORE="$(sha "$CANDIDATE")"
BASELINE_SHA_BEFORE="$(sha "$BASELINE")"

make_variant stale-commit 'd["repository"]["commit"] = "0" * 40'
make_variant candidate-sha 'd["candidate"]["sha256"] = "1" * 64'
make_variant missing-evidence 'd["independent_validation"]["status"] = "PASS"; d["independent_validation"]["results"]["status"] = "PENDING"; d["independent_validation"]["results"]["calls"] = []; d["independent_validation"]["results"]["summary"] = {"total": 20, "passed": 0, "failed": 0}'
make_variant missing-candidate-identity 'd["independent_validation"]["results"]["candidate_sha256"] = ""'
make_variant wrong-workflow-id 'd["independent_validation"]["results"]["workflow_id"] = "other-workflow"'
make_variant unsupported-schema 'd["schema"] = "mcp-live-validation/validation-handoff/v99"'
make_variant unapproved-deployment 'd["approval"]["deployment"]["status"] = "PENDING"; d["workflow"]["status"] = "PREPARED"; d["workflow"]["next_permitted_action"] = "START_CANDIDATE"'
make_variant stale-time 'd["generated_at"] = "2000-01-01T00:00:00Z"'
make_variant wrong-source-revision 'd["independent_validation"]["results"]["source_revision"] = "c" * 40'
make_variant approval-without-reference 'd["approval"]["deployment"]["reference"] = ""'
make_variant credential-in-value 'd["local_verification"]["results"] += " Authorization: Bearer abcdefghijklmnopqrstuvwxyz0123"'

for variant in stale-commit candidate-sha missing-evidence missing-candidate-identity wrong-workflow-id unsupported-schema unapproved-deployment stale-time wrong-source-revision approval-without-reference credential-in-value; do
  assert_invalid "$ROOT/variant-$variant.json"
  assert_eq "$(sha "$CANDIDATE")" "$CANDIDATE_SHA_BEFORE"
  assert_eq "$(sha "$BASELINE")" "$BASELINE_SHA_BEFORE"
  kill -0 "$CANARY_PID" 2>/dev/null || fail "invalid handoff stopped the canary process"
done
pass "scenario C: stale, mismatched, missing-evidence, unsupported, and unapproved handoffs are rejected safely"

# A credential value is reported by path only, never echoed.
python3 "$VALIDATOR" --handoff "$ROOT/variant-credential-in-value.json" --checkout "$ROOT" \
  --schema "$SCHEMA" >"$TMP/cred.out" 2>&1 || true
assert_contains "$(cat "$TMP/cred.out")" "local_verification.results"
case "$(cat "$TMP/cred.out")" in
  *abcdefghijklmnopqrstuvwxyz0123*) fail "the validator echoed a credential value" ;;
  *) pass "scenario C: a credential pasted into a value is rejected without being echoed" ;;
esac

# The validator must be told whether to cross-check a checkout; it never prints a
# bare OK for an artifact it did not check.
if python3 "$VALIDATOR" --handoff "$ROOT/handoff-pass.json" --schema "$SCHEMA" >"$TMP/nomode.out" 2>&1; then
  fail "the validator accepted a handoff without --checkout or --artifact-only"
else
  assert_contains "$(cat "$TMP/nomode.out")" "--checkout"
  pass "scenario C: the validator requires --checkout or --artifact-only"
fi

# An approval must name the human decision behind it.
if run_handoff --format json --agent claude --capabilities filesystem,process_execution \
  --local-status PASS --tests "$ROOT/tests.txt" --deployment-approval APPROVED \
  --issue PORTABILITY-A --output "$ROOT/handoff-noref.json" >"$TMP/out" 2>&1; then
  fail "the helper recorded APPROVED without an approval reference"
else
  assert_contains "$(cat "$TMP/out")" "requires --approval-reference"
  [ ! -e "$ROOT/handoff-noref.json" ] || fail "a refused handoff was written"
  pass "scenario C: the helper refuses APPROVED without an approval reference"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

if [ "$FAILS" -ne 0 ]; then
  printf '\n%s failure(s)\n' "$FAILS" >&2
  exit 1
fi
printf '\nall validation-handoff portability tests passed (simulated agents)\n'
