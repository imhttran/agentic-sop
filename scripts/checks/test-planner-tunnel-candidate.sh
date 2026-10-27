#!/usr/bin/env bash
#
# Regression tests for scripts/planner-tunnel-candidate.sh.
#
# The suite is isolated: it creates its own git checkout, its own mock
# tunnel-client binaries, and its own processes. It never reads or writes the
# live .run/tunnel state and never touches a real tunnel-client.
#
# Usage: scripts/checks/test-planner-tunnel-candidate.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd -P)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd -P)"
CANDIDATE_SCRIPT="$REPO_ROOT/scripts/planner-tunnel-candidate.sh"
POINT_SCRIPT="$REPO_ROOT/scripts/planner-tunnel-point.sh"

TMP="$(mktemp -d "${TMPDIR:-/tmp}/sop-candidate-test.XXXXXX")"
PIDS=()
FAILS=0

cleanup() {
  local p cmd
  for p in ${PIDS[@]+"${PIDS[@]}"}; do
    [ -n "$p" ] || continue
    if kill -0 "$p" 2>/dev/null; then
      cmd="$(ps -p "$p" -o command= 2>/dev/null || true)"
      case "$cmd" in
        *"$TMP"*) kill -TERM "$p" 2>/dev/null || true ;;
      esac
    fi
  done
  sleep 0.2
  rm -rf "$TMP"
}
trap cleanup EXIT

fail() { printf 'FAIL: %s\n' "$*" >&2; FAILS=$((FAILS + 1)); }
pass() { printf 'ok   %s\n' "$*"; }

assert_contains() {
  case "$1" in
    *"$2"*) : ;;
    *) fail "expected output to contain '$2'; got:
$1" ;;
  esac
}
assert_not_contains() {
  case "$1" in
    *"$2"*) fail "expected output not to contain '$2'; got:
$1" ;;
    *) : ;;
  esac
}
assert_file() { [ -f "$1" ] || fail "expected file: $1"; }
assert_exec() { [ -x "$1" ] || fail "expected executable: $1"; }
assert_alive() { kill -0 "$1" 2>/dev/null || fail "expected PID $1 to be alive"; }
assert_dead() { ! kill -0 "$1" 2>/dev/null || fail "expected PID $1 to be dead"; }
assert_eq() { [ "$1" = "$2" ] || fail "expected '$1' = '$2'"; }

sha() {
  if command -v shasum >/dev/null 2>&1; then shasum -a 256 "$1" | awk '{print $1}';
  else sha256sum "$1" | awk '{print $1}'; fi
}

write_validation_results() {
  local path="$1" status="$2" candidate_sha="$3" source_rev="$4" workflow_id="$5" validator="$6"
  command -v python3 >/dev/null 2>&1 || { fail "python3 is required for the JSON handoff tests"; return 1; }
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
            "elapsed_ms": 12,
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

wait_for_file() {
  local f="$1" i=0
  while [ ! -s "$f" ] && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
  [ -s "$f" ] || { fail "timed out waiting for $f"; return 1; }
}
wait_dead() {
  local p="$1" i=0
  while kill -0 "$p" 2>/dev/null && [ "$i" -lt 100 ]; do sleep 0.1; i=$((i + 1)); done
  ! kill -0 "$p" 2>/dev/null
}

# ---------------------------------------------------------------------------
# Fixture: a mock tunnel-client source repo, a baseline binary, and a patch.
# ---------------------------------------------------------------------------

SRC="$TMP/tunnel-client-src"
BIN="$TMP/bin"
BASELINE="$BIN/planner-tunnel-test-baseline"
SOP_ROOT="$TMP/sop"
MATCH="planner-tunnel-test"
mkdir -p "$SRC/cmd/client" "$SRC/pkg/version" "$BIN" "$TMP/logs" "$SOP_ROOT/.run/tunnel"

cat > "$SRC/go.mod" <<'EOF'
module github.com/openai/tunnel-client

go 1.21
EOF
cat > "$SRC/pkg/version/version.go" <<'EOF'
package version

// GitSHA is injected with -ldflags in the test fixture.
var GitSHA = "dev"
EOF
cat > "$SRC/cmd/client/main.go" <<'EOF'
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/openai/tunnel-client/pkg/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("0.0.16+%s\n", version.GitSHA)
		return
	}
	logPath, pidPath := "", ""
	for i := 1; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--log.file":
			if i+1 < len(os.Args) {
				logPath = os.Args[i+1]
				i++
			}
		case "--pid.file":
			if i+1 < len(os.Args) {
				pidPath = os.Args[i+1]
				i++
			}
		}
	}
	if pidPath != "" {
		_ = os.WriteFile(pidPath, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
	}
	if logPath != "" {
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			line, _ := json.Marshal(map[string]any{
				"time":  time.Now().UTC().Format(time.RFC3339Nano),
				"level": "INFO",
				"msg":   "mock tunnel started",
			})
			_, _ = f.Write(append(line, '\n'))
			_ = f.Close()
		}
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT)
	<-ch
}
EOF

git -C "$SRC" init -q -b main
git -C "$SRC" add -A
git -C "$SRC" -c user.name=test -c user.email=test@localhost commit -q -m "baseline"
BASELINE_REV="$(git -C "$SRC" rev-parse HEAD)"

# Build the baseline from the pinned source before creating the patch commit.
(
  cd "$SRC"
  CGO_ENABLED=0 go build -ldflags "-X github.com/openai/tunnel-client/pkg/version.GitSHA=baseline" \
    -o "$BASELINE" ./cmd/client
)
chmod +x "$BASELINE"

# Create a patch commit and export it, then return the source to the baseline.
printf '\n// issue-90 candidate patch\n' >> "$SRC/pkg/version/version.go"
git -C "$SRC" add -A
git -C "$SRC" -c user.name=test -c user.email=test@localhost commit -q -m "test: candidate patch"
PATCH_DIR="$TMP/patches"
mkdir -p "$PATCH_DIR"
git -C "$SRC" format-patch -1 -q -o "$PATCH_DIR"
PATCH="$(find "$PATCH_DIR" -name '*.patch' -type f | head -n 1)"
[ -n "$PATCH" ] || { echo "fixture: could not create patch" >&2; exit 1; }
git -C "$SRC" reset -q --hard "$BASELINE_REV"

# A minimal agentic-sop checkout so the script can resolve .run/tunnel.
git -C "$SOP_ROOT" init -q -b main
printf 'fixture\n' > "$SOP_ROOT/README.md"
cat > "$SOP_ROOT/.run/tunnel/sop-mcp-planner.sh" <<EOF
#!/bin/sh
export PWD="$SOP_ROOT"
exec "$BIN/sop-fixture" mcp --planner
EOF
chmod +x "$SOP_ROOT/.run/tunnel/sop-mcp-planner.sh"
ORIGINAL_WRAPPER_SHA="$(sha "$SOP_ROOT/.run/tunnel/sop-mcp-planner.sh")"
git -C "$SOP_ROOT" add -A
git -C "$SOP_ROOT" -c user.name=test -c user.email=test@localhost commit -q -m "fixture"

MANIFEST="$SOP_ROOT/.run/tunnel/candidate/manifest"
CANDIDATE="$BIN/planner-tunnel-test-candidate"
BUILD_DIR="$SOP_ROOT/.run/tunnel/build/tunnel-client-${BASELINE_REV:0:12}-$(sha "$PATCH" | cut -c1-12)"

run_candidate() {
  "$CANDIDATE_SCRIPT" "$@" --checkout "$SOP_ROOT" --process-match "$MATCH"
}

mg() { sed -n "s/^$1=//p" "$MANIFEST" | head -n 1; }
mg_all() { sed -n "s/^$1=//p" "$MANIFEST"; }

# ---------------------------------------------------------------------------
# 1. No manifest: preflight/start fail safely.
# ---------------------------------------------------------------------------

if run_candidate preflight >"$TMP/out" 2>&1; then
  fail "preflight should fail without a manifest"
else
  assert_contains "$(cat "$TMP/out")" "no candidate manifest"
  pass "preflight refuses without a manifest"
fi
if run_candidate start >"$TMP/out" 2>&1; then
  fail "start should fail without a manifest"
else
  assert_contains "$(cat "$TMP/out")" "no candidate manifest"
  pass "start refuses without a manifest"
fi

# ---------------------------------------------------------------------------
# 2. Dry-run provisioning writes nothing.
# ---------------------------------------------------------------------------

if ! run_candidate provision --dry-run --source "$SRC" --revision "$BASELINE_REV" \
  --patch "$PATCH" --baseline "$BASELINE" --profile test-profile \
  --output "$CANDIDATE" --build-dir "$BUILD_DIR" >"$TMP/out" 2>&1; then
  fail "dry-run provision failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "dry-run"
  [ ! -e "$CANDIDATE" ] || fail "dry-run created the candidate"
  [ ! -e "$MANIFEST" ] || fail "dry-run created the manifest"
  [ ! -e "$BUILD_DIR" ] || fail "dry-run created the build directory"
  pass "dry-run provision writes nothing"
fi

# ---------------------------------------------------------------------------
# 3. Provision a real candidate while the baseline is running.
# ---------------------------------------------------------------------------

"$BASELINE" run --profile test-profile \
  --log.file "$TMP/logs/baseline.jsonl" --pid.file "$TMP/baseline.pid" \
  >"$TMP/baseline.out" 2>&1 &
BASE_PID=$!
PIDS+=("$BASE_PID")
disown "$BASE_PID" 2>/dev/null || true
wait_for_file "$TMP/baseline.pid" || exit 1
BASE_PID="$(cat "$TMP/baseline.pid")"

if ! run_candidate provision --source "$SRC" --revision "$BASELINE_REV" \
  --patch "$PATCH" --baseline "$BASELINE" --profile test-profile \
  --output "$CANDIDATE" --build-dir "$BUILD_DIR" \
  --log-file "$TMP/logs/candidate.jsonl" --pid-file "$TMP/candidate.pid" >"$TMP/out" 2>&1; then
  fail "provision failed: $(cat "$TMP/out")"
  cat "$TMP/out" >&2
  exit 1
fi
assert_file "$CANDIDATE"
assert_exec "$CANDIDATE"
assert_file "$MANIFEST"
assert_eq "$(mg candidate_path)" "$CANDIDATE"
assert_eq "$(mg candidate_sha256)" "$(sha "$CANDIDATE")"
assert_eq "$(mg baseline_sha256)" "$(sha "$BASELINE")"
assert_eq "$(mg baseline_process_pid)" "$BASE_PID"
assert_eq "$(mg profile)" "test-profile"
assert_contains "$(mg_all candidate_launch_arg)" "run"
assert_contains "$(mg_all candidate_launch_arg)" "$TMP/logs/candidate.jsonl"
assert_contains "$(mg_all candidate_launch_arg)" "$TMP/candidate.pid"
assert_not_contains "$(mg_all candidate_launch_arg)" "http-raw-unsafe"
assert_file "$(mg wrapper_backup)"
assert_eq "$(sha "$(mg wrapper_backup)")" "$ORIGINAL_WRAPPER_SHA"
pass "provision records candidate, baseline, profile, and rollback provenance"

# A second provision with the same inputs reuses the source and candidate.
if ! run_candidate provision --source "$SRC" --revision "$BASELINE_REV" \
  --patch "$PATCH" --baseline "$BASELINE" --profile test-profile \
  --output "$CANDIDATE" --build-dir "$BUILD_DIR" \
  --log-file "$TMP/logs/candidate.jsonl" --pid-file "$TMP/candidate.pid" >"$TMP/out" 2>&1; then
  fail "idempotent provision failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "reusing"
  pass "provision reuses an existing source checkout and candidate"
fi

# ---------------------------------------------------------------------------
# 4. Preflight passes, then fails on a missing or mismatched candidate.
# ---------------------------------------------------------------------------

if ! run_candidate preflight >"$TMP/out" 2>&1; then
  fail "preflight failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "preflight passed"
  pass "preflight passes for a prepared candidate"
fi

mv "$CANDIDATE" "$TMP/candidate.save"
if run_candidate preflight >"$TMP/out" 2>&1; then
  fail "preflight should fail when the candidate is missing"
else
  assert_contains "$(cat "$TMP/out")" "candidate"
  pass "preflight refuses a missing candidate"
fi
mv "$TMP/candidate.save" "$CANDIDATE"

cp "$CANDIDATE" "$TMP/candidate.save"
cp "$BASELINE" "$CANDIDATE"
MISMATCH_SHA="$(sha "$CANDIDATE")"
cp "$MANIFEST" "$TMP/manifest.save"
sed -i.bak "s/^candidate_sha256=.*/candidate_sha256=$MISMATCH_SHA/" "$MANIFEST"
if run_candidate preflight >"$TMP/out" 2>&1; then
  fail "preflight should fail on a version mismatch"
else
  assert_contains "$(cat "$TMP/out")" "version"
  pass "preflight refuses a candidate version mismatch"
fi
mv "$TMP/manifest.save" "$MANIFEST"
mv "$TMP/candidate.save" "$CANDIDATE"
rm -f "$MANIFEST.bak"

# ---------------------------------------------------------------------------
# 5. Stop is explicit and verified: it stops the captured baseline, not an
#    unrelated process.
# ---------------------------------------------------------------------------

sleep 30 &
DECOY_PID=$!
PIDS+=("$DECOY_PID")
disown "$DECOY_PID" 2>/dev/null || true
if ! run_candidate stop >"$TMP/out" 2>&1; then
  fail "stop failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "baseline stopped"
  wait_dead "$BASE_PID" || fail "baseline PID $BASE_PID is still alive"
  assert_alive "$DECOY_PID"
  assert_eq "$(mg state)" "stopped"
  pass "stop stops the verified baseline and leaves an unrelated process alone"
fi
kill -TERM "$DECOY_PID" 2>/dev/null || true
wait_dead "$DECOY_PID" || true

# ---------------------------------------------------------------------------
# 6. Start dry-run and start.
# ---------------------------------------------------------------------------

if ! run_candidate start --dry-run >"$TMP/out" 2>&1; then
  fail "start dry-run failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "dry-run: not starting"
  pass "start dry-run does not start the candidate"
fi

# The deployment gate requires a validated handoff that authorizes START_CANDIDATE.
sed -i.bak 's/^state=.*/state=prepared/' "$MANIFEST"
rm -f "$MANIFEST.bak"
printf 'go test ./...: pass\n' > "$TMP/gate-tests.txt"
if ! run_candidate handoff --format json --local-status PASS --tests "$TMP/gate-tests.txt" \
  --deployment-approval PENDING --output "$TMP/gate-unapproved.json" >"$TMP/out" 2>&1; then
  fail "unapproved gate handoff failed: $(cat "$TMP/out")"
else
  if run_candidate start --handoff "$TMP/gate-unapproved.json" >"$TMP/out" 2>&1; then
    fail "start should refuse a handoff that does not authorize START_CANDIDATE"
  else
    assert_contains "$(cat "$TMP/out")" "not START_CANDIDATE"
    assert_eq "$(mg process_pid)" ""
    pass "start refuses an unapproved handoff without launching a process"
  fi
fi

if ! run_candidate handoff --format json --local-status PASS --tests "$TMP/gate-tests.txt" \
  --deployment-approval APPROVED --approval-reference "test approval" \
  --output "$TMP/gate-approved.json" >"$TMP/out" 2>&1; then
  fail "approved gate handoff failed: $(cat "$TMP/out")"
else
  if ! run_candidate start --handoff "$TMP/gate-approved.json" >"$TMP/out" 2>&1; then
    fail "start with an approved handoff failed: $(cat "$TMP/out")"
    cat "$TMP/out" >&2
    exit 1
  fi
  CAND_PID="$(mg process_pid)"
  assert_alive "$CAND_PID"
  assert_contains "$(mg process_command)" "$CANDIDATE"
  assert_eq "$(mg state)" "candidate-running"
  assert_file "$TMP/logs/candidate.jsonl"
  pass "start launches the candidate when the handoff authorizes START_CANDIDATE"
fi

if ! run_candidate status >"$TMP/out" 2>&1; then
  fail "status failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "state:     candidate-running"
  assert_contains "$(cat "$TMP/out")" "$CANDIDATE"
  pass "status reports the active candidate"
fi

# ---------------------------------------------------------------------------
# 7. Multiple tunnel processes: refuse to stop or start.
# ---------------------------------------------------------------------------

"$BASELINE" run --profile test-profile \
  --log.file "$TMP/logs/extra.jsonl" --pid.file "$TMP/extra.pid" \
  >"$TMP/extra.out" 2>&1 &
EXTRA_PID=$!
PIDS+=("$EXTRA_PID")
disown "$EXTRA_PID" 2>/dev/null || true
wait_for_file "$TMP/extra.pid" || exit 1
EXTRA_PID="$(cat "$TMP/extra.pid")"
if run_candidate stop >"$TMP/out" 2>&1; then
  fail "stop should refuse with multiple tunnel processes"
else
  assert_contains "$(cat "$TMP/out")" "refusing to stop"
  assert_alive "$CAND_PID"
  pass "stop refuses when multiple tunnel processes are running"
fi
kill -TERM "$EXTRA_PID" 2>/dev/null || true
wait_dead "$EXTRA_PID" || fail "extra PID did not exit"

# ---------------------------------------------------------------------------
# 8. Unexpected PID/command: refuse to stop a process whose identity changed.
# ---------------------------------------------------------------------------

cp "$MANIFEST" "$TMP/manifest.save"
sed -i.bak "s/^process_command=.*/process_command=not-the-candidate/" "$MANIFEST"
if run_candidate stop >"$TMP/out" 2>&1; then
  fail "stop should refuse an identity mismatch"
else
  assert_contains "$(cat "$TMP/out")" "recorded command does not match"
  assert_alive "$CAND_PID"
  pass "stop refuses a process whose command does not match the manifest"
fi
mv "$TMP/manifest.save" "$MANIFEST"
rm -f "$MANIFEST.bak"

if ! run_candidate stop >"$TMP/out" 2>&1; then
  fail "stop failed after restoring the manifest: $(cat "$TMP/out")"
else
  wait_dead "$CAND_PID" || fail "candidate PID $CAND_PID is still alive"
  pass "stop succeeds after the manifest identity is restored"
fi

# ---------------------------------------------------------------------------
# 9. Startup failure: stop the failed candidate, leave the baseline intact.
# ---------------------------------------------------------------------------

BASELINE_SHA_BEFORE="$(sha "$BASELINE")"
cp "$CANDIDATE" "$TMP/candidate.good"
cp "$MANIFEST" "$TMP/manifest.good"
cat > "$CANDIDATE" <<'EOF'
#!/bin/sh
if [ "${1:-}" = "--version" ]; then
  echo "0.0.16+bad"
  exit 0
fi
exit 1
EOF
chmod +x "$CANDIDATE"
BAD_SHA="$(sha "$CANDIDATE")"
sed -i.bak "s/^candidate_sha256=.*/candidate_sha256=$BAD_SHA/; s/^candidate_version=.*/candidate_version=0.0.16+bad/" "$MANIFEST"
if run_candidate start --startup-timeout 2 >"$TMP/out" 2>&1; then
  fail "start should fail for a candidate that exits immediately"
else
  assert_contains "$(cat "$TMP/out")" "startup failed"
  assert_eq "$(mg state)" "failed"
  assert_eq "$(sha "$BASELINE")" "$BASELINE_SHA_BEFORE"
  case "$(run_candidate status 2>&1)" in
    *"state:     candidate-running"*) fail "failed start left a candidate running" ;;
  esac
  pass "startup failure stops the candidate and leaves the baseline intact"
fi
mv "$TMP/candidate.good" "$CANDIDATE"
mv "$TMP/manifest.good" "$MANIFEST"
rm -f "$MANIFEST.bak"

# ---------------------------------------------------------------------------
# 10. Rollback: stop candidate, restore wrapper, start the recorded baseline.
# ---------------------------------------------------------------------------

if ! run_candidate start >"$TMP/out" 2>&1; then
  fail "restart after the failure test failed: $(cat "$TMP/out")"
  exit 1
fi
CAND_PID="$(mg process_pid)"
if ! run_candidate rollback --start >"$TMP/out" 2>&1; then
  fail "rollback failed: $(cat "$TMP/out")"
  cat "$TMP/out" >&2
  exit 1
fi
wait_dead "$CAND_PID" || fail "candidate PID $CAND_PID is still alive after rollback"
assert_eq "$(mg state)" "baseline-running"
assert_eq "$(sha "$SOP_ROOT/.run/tunnel/sop-mcp-planner.sh")" "$ORIGINAL_WRAPPER_SHA"
BASELINE_NEW_PID="$(mg baseline_process_pid)"
assert_alive "$BASELINE_NEW_PID"
assert_eq "$(sha "$BASELINE")" "$BASELINE_SHA_BEFORE"
pass "rollback stops the candidate, restores the wrapper, and starts the baseline"

if ! run_candidate stop >"$TMP/out" 2>&1; then
  fail "stopping the rolled-back baseline failed: $(cat "$TMP/out")"
else
  wait_dead "$BASELINE_NEW_PID" || fail "rolled-back baseline is still alive"
  pass "the rolled-back baseline can be stopped explicitly"
fi

# ---------------------------------------------------------------------------
# 11. Handoff contract.
# ---------------------------------------------------------------------------

if ! run_candidate handoff >"$TMP/handoff.md" 2>"$TMP/err"; then
  fail "handoff failed: $(cat "$TMP/err")"
else
  HANDOFF="$(cat "$TMP/handoff.md")"
  for needle in \
    "Repository" "Candidate executable" "Active process" "Local verification" \
    "Independent validation" "Rollback" "Requested live MCP tests" \
    "sop_status" "sop_question_get" \
    "20 calls total" "mcp_initialization_required" "not a validation result"; do
    assert_contains "$HANDOFF" "$needle"
  done
  pass "handoff contains the required contract fields and test request"
fi

if ! run_candidate handoff --output "$TMP/handoff-out.md" >"$TMP/out" 2>&1; then
  fail "handoff --output failed: $(cat "$TMP/out")"
else
  assert_file "$TMP/handoff-out.md"
  if run_candidate handoff --output "$TMP/handoff-out.md" >"$TMP/out" 2>&1; then
    fail "handoff should refuse to overwrite an existing report"
  else
    assert_contains "$(cat "$TMP/out")" "output exists"
    pass "handoff refuses a silent overwrite"
  fi
fi

# ---------------------------------------------------------------------------
# 11b. Machine-readable handoff and validation status.
# ---------------------------------------------------------------------------

HANDOFF_JSON="$TMP/handoff.json"
if ! run_candidate handoff --format json --issue TEST-1 --output "$HANDOFF_JSON" >"$TMP/out" 2>&1; then
  fail "json handoff failed: $(cat "$TMP/out")"
else
  assert_file "$HANDOFF_JSON"
  JSON="$(cat "$HANDOFF_JSON")"
  for needle in \
    '"schema": "mcp-live-validation/validation-handoff/v1"' \
    '"schema_version": 1' \
    '"workflow_id"' \
    '"issue": "TEST-1"' \
    '"status": "PENDING"' \
    '"next_permitted_action"' \
    '"environment"' \
    '"local_verification"' \
    '"results_template"' \
    '"approval"' \
    '"rounds": 10' \
    '"total_calls": 20' \
    '"retries": 0' \
    '"state_changes": false' \
    '"tool": "sop_status"' \
    '"tool": "sop_question_get"' \
    '"local_tests_are_not_validation": true' \
    '"production_binary_replaced": false' \
    '"merged": false' \
    '"credentials_redacted": true'; do
    assert_contains "$JSON" "$needle"
  done
  if command -v python3 >/dev/null 2>&1; then
    printf '%s' "$JSON" | python3 -m json.tool >/dev/null || fail "json handoff is not valid JSON"
    WORKFLOW_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["workflow_id"])' "$HANDOFF_JSON")"
  elif command -v jq >/dev/null 2>&1; then
    printf '%s' "$JSON" | jq empty >/dev/null || fail "json handoff is not valid JSON"
    WORKFLOW_ID="$(jq -r .workflow_id "$HANDOFF_JSON")"
  else
    WORKFLOW_ID=""
  fi
  [ -n "$WORKFLOW_ID" ] || fail "json handoff did not record a workflow_id"
  assert_contains "$JSON" "\"next_permitted_action\": \"STOP_AND_REPORT\""
  pass "json handoff is machine-readable and records the portable contract"
fi

if run_candidate handoff --format json --validation-status PASS >"$TMP/out" 2>&1; then
  fail "PASS without --results-json should be refused"
else
  assert_contains "$(cat "$TMP/out")" "requires --results-json"
  pass "handoff refuses a PASS status without independent evidence"
fi

CANDIDATE_SHA="$(sha "$CANDIDATE")"
CANDIDATE_REV="$(mg candidate_revision)"
printf 'go test ./...: pass\nmake test-planner-tunnel: pass\n' > "$TMP/local-tests.txt"
write_validation_results "$TMP/results-pass.json" PASS "$CANDIDATE_SHA" "$CANDIDATE_REV" "$WORKFLOW_ID" chatgpt
if ! run_candidate handoff --format json --validation-status PASS \
  --results-json "$TMP/results-pass.json" --local-status PASS \
  --tests "$TMP/local-tests.txt" --deployment-approval APPROVED \
  --approval-reference "operator approval (test fixture)" \
  --output "$TMP/handoff-pass.json" >"$TMP/out" 2>&1; then
  fail "handoff PASS failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/handoff-pass.json")" '"status": "PASS"'
  assert_contains "$(cat "$TMP/handoff-pass.json")" '"next_permitted_action": "PREPARE_PR"'
  assert_contains "$(cat "$TMP/handoff-pass.json")" '"validator": "chatgpt"'
  pass "handoff records an independent PASS result and prepares the PR action"
fi

write_validation_results "$TMP/results-fail.json" FAIL "$CANDIDATE_SHA" "$CANDIDATE_REV" "$WORKFLOW_ID" chatgpt
if ! run_candidate handoff --format json --validation-status FAIL \
  --results-json "$TMP/results-fail.json" --local-status PASS \
  --tests "$TMP/local-tests.txt" --deployment-approval APPROVED \
  --approval-reference "operator approval (test fixture)" \
  --output "$TMP/handoff-fail.json" >"$TMP/out" 2>&1; then
  fail "handoff FAIL failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/handoff-fail.json")" '"status": "FAIL"'
  assert_contains "$(cat "$TMP/handoff-fail.json")" '"next_permitted_action": "DIAGNOSE_AND_PREPARE_CORRECTED_CANDIDATE"'
  assert_contains "$(cat "$TMP/handoff-fail.json")" "test-failure"
  pass "handoff records a FAIL result and its evidence"
fi

# Redaction: an evidence artifact must not carry a credential value.
cp "$MANIFEST" "$TMP/manifest.redact"
sed -i.bak 's/^process_command=.*/process_command=tunnel-client run --profile test --token supersecret-value/' "$MANIFEST"
if ! run_candidate handoff --format json --output "$TMP/handoff-redact.json" >"$TMP/out" 2>&1; then
  fail "redacted handoff failed: $(cat "$TMP/out")"
else
  assert_not_contains "$(cat "$TMP/handoff-redact.json")" "supersecret-value"
  assert_contains "$(cat "$TMP/handoff-redact.json")" "[REDACTED]"
  pass "handoff redacts credential-like command values"
fi
mv "$TMP/manifest.redact" "$MANIFEST"
rm -f "$MANIFEST.bak"

# ---------------------------------------------------------------------------
# 11c. Scenario B: failed validation, then a corrected candidate without
#      losing the original candidate or evidence.
# ---------------------------------------------------------------------------

ORIGINAL_SHA="$(sha "$CANDIDATE")"
ORIGINAL_FAIL="$(cat "$TMP/handoff-fail.json")"
printf '\n// corrected candidate patch\n' >> "$SRC/pkg/version/version.go"
git -C "$SRC" add -A
git -C "$SRC" -c user.name=test -c user.email=test@localhost commit -q -m "test: corrected candidate patch"
PATCH2_DIR="$TMP/patches2"
mkdir -p "$PATCH2_DIR"
git -C "$SRC" format-patch -1 -q -o "$PATCH2_DIR"
PATCH2="$(find "$PATCH2_DIR" -name '*.patch' -type f | head -n 1)"
[ -n "$PATCH2" ] || fail "fixture: could not create the corrected patch"
git -C "$SRC" reset -q --hard "$BASELINE_REV"
CANDIDATE2="$BIN/planner-tunnel-test-candidate-fixed"
BUILD2="$SOP_ROOT/.run/tunnel/build/tunnel-client-fixed"
if ! run_candidate provision --source "$SRC" --revision "$BASELINE_REV" \
  --patch "$PATCH2" --baseline "$BASELINE" --profile test-profile \
  --output "$CANDIDATE2" --build-dir "$BUILD2" \
  --log-file "$TMP/logs/candidate-fixed.jsonl" --pid-file "$TMP/candidate-fixed.pid" >"$TMP/out" 2>&1; then
  fail "corrected provision failed: $(cat "$TMP/out")"
else
  assert_exec "$CANDIDATE2"
  [ "$(sha "$CANDIDATE")" = "$ORIGINAL_SHA" ] || fail "corrected provision changed the original candidate"
  assert_eq "$(cat "$TMP/handoff-fail.json")" "$ORIGINAL_FAIL"
  [ "$(mg candidate_path)" = "$CANDIDATE2" ] || fail "new manifest does not point at the corrected candidate"
  [ -n "$(find "$SOP_ROOT/.run/tunnel/candidate" -name 'manifest.bak-*' -type f | head -n 1)" ] || fail "no manifest backup preserved"
  pass "failed validation keeps the original evidence and a corrected candidate is prepared"
fi

# ---------------------------------------------------------------------------
# 12. No broad process killers in the helpers.
# ---------------------------------------------------------------------------

if grep -nE '(^|[;&|()]|then|do)[[:space:]]*(pkill|killall)([[:space:]]|$)' \
  "$CANDIDATE_SCRIPT" "$POINT_SCRIPT" >"$TMP/out" 2>&1; then
  fail "helpers contain a broad process killer:
$(cat "$TMP/out")"
else
  pass "helpers contain no pkill/killall"
fi

# ---------------------------------------------------------------------------
# 13. Wrapper pointer dry-run: local check only, no live wrapper write.
# ---------------------------------------------------------------------------

POINT_CHECKOUT="$TMP/point-checkout"
POINT_PROJECT="$TMP/point-project"
MOCK_SOP="$TMP/mock-sop"
mkdir -p "$POINT_CHECKOUT" "$POINT_PROJECT/.agent-sdlc"
git -C "$POINT_CHECKOUT" init -q -b main
printf 'fixture\n' > "$POINT_CHECKOUT/README.md"
git -C "$POINT_CHECKOUT" add -A
git -C "$POINT_CHECKOUT" -c user.name=test -c user.email=test@localhost commit -q -m fixture
cat > "$MOCK_SOP" <<'EOF'
#!/bin/sh
# Consume the three MCP lines, then answer the tools/call.
cat >/dev/null
printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"[]"}],"isError":false}}'
EOF
chmod +x "$MOCK_SOP"
if ! "$POINT_SCRIPT" --checkout "$POINT_CHECKOUT" --binary "$MOCK_SOP" --dry-run "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "point dry-run failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "local check ok"
  assert_contains "$(cat "$TMP/out")" "live wrapper left unchanged"
  [ ! -e "$POINT_CHECKOUT/.run/tunnel/sop-mcp-planner.sh" ] || fail "point dry-run wrote the live wrapper"
  pass "point dry-run checks the wrapper locally and writes nothing"
fi

# ---------------------------------------------------------------------------
# 14. Wrapper pointer --start-work: the wrapper serves the flag, and a binary
#     without sop_start_work fails the local check and changes nothing.
# ---------------------------------------------------------------------------

MOCK_SOP_SW="$TMP/mock-sop-start-work"
cat > "$MOCK_SOP_SW" <<'EOF'
#!/bin/sh
# Answer the tools/call; list sop_start_work only when served with --start-work.
input="$(cat)"
printf '%s\n' '{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"no pending questions"}],"isError":false}}'
case "$input" in
  *tools/list*)
    case " $* " in
      *" --start-work "*) printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"tools":[{"name":"sop_status"},{"name":"sop_start_work"},{"name":"sop_work_status"}]}}' ;;
      *) printf '%s\n' '{"jsonrpc":"2.0","id":3,"result":{"tools":[{"name":"sop_status"}]}}' ;;
    esac ;;
esac
EOF
chmod +x "$MOCK_SOP_SW"

if ! "$POINT_SCRIPT" --checkout "$POINT_CHECKOUT" --binary "$MOCK_SOP_SW" --start-work --dry-run "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "point --start-work dry-run failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "sop_start_work is served"
  assert_contains "$(cat "$TMP/out")" "local check ok; sop_question_list"
  pass "point --start-work checks that sop_start_work is served"
fi

# The old mock answers questions but has no sop_start_work: refuse, write nothing.
if "$POINT_SCRIPT" --checkout "$POINT_CHECKOUT" --binary "$MOCK_SOP" --start-work "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "point --start-work accepted a binary without sop_start_work"
else
  assert_contains "$(cat "$TMP/out")" "does not serve sop_start_work"
  [ ! -e "$POINT_CHECKOUT/.run/tunnel/sop-mcp-planner.sh" ] || fail "a failed --start-work check wrote the live wrapper"
  pass "point --start-work rejects a binary without sop_start_work"
fi

# ---------------------------------------------------------------------------
# 15. setup-planner: defaults to the checkout, writes the --start-work wrapper,
#     and registers the Claude desktop app only when asked.
# ---------------------------------------------------------------------------

SETUP_SCRIPT="$REPO_ROOT/scripts/setup-planner.sh"
SETUP_CHECKOUT="$TMP/setup-checkout"
SETUP_HOME="$TMP/setup-home"
mkdir -p "$SETUP_CHECKOUT/scripts" "$SETUP_CHECKOUT/.agent-sdlc" "$SETUP_CHECKOUT/.run/spike" "$SETUP_HOME"
cp "$POINT_SCRIPT" "$SETUP_SCRIPT" "$SETUP_CHECKOUT/scripts/"
cp "$MOCK_SOP_SW" "$SETUP_CHECKOUT/.run/spike/sop-planner"
git -C "$SETUP_CHECKOUT" init -q -b main
git -C "$SETUP_CHECKOUT" add scripts
git -C "$SETUP_CHECKOUT" -c user.name=test -c user.email=test@localhost commit -q -m fixture
SETUP_WRAPPER="$SETUP_CHECKOUT/.run/tunnel/sop-mcp-planner.sh"
SETUP_ENV=(env HOME="$SETUP_HOME" XDG_CONFIG_HOME="$SETUP_HOME/.config")
if [ "$(uname -s)" = Darwin ]; then
  CLAUDE_CFG="$SETUP_HOME/Library/Application Support/Claude/claude_desktop_config.json"
else
  CLAUDE_CFG="$SETUP_HOME/.config/Claude/claude_desktop_config.json"
fi

# Dry run: checks, changes nothing.
if ! "${SETUP_ENV[@]}" "$SETUP_SCRIPT" --checkout "$SETUP_CHECKOUT" --no-build --dry-run >"$TMP/out" 2>&1; then
  fail "setup-planner dry-run failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "nothing was changed"
  [ ! -e "$SETUP_WRAPPER" ] || fail "setup-planner dry-run wrote the wrapper"
  pass "setup-planner dry-run changes nothing"
fi

# Default project is the checkout; without --register-claude it only reports.
if ! "${SETUP_ENV[@]}" "$SETUP_SCRIPT" --checkout "$SETUP_CHECKOUT" --no-build >"$TMP/out" 2>&1; then
  fail "setup-planner failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$SETUP_WRAPPER")" "PROJECT='$(cd "$SETUP_CHECKOUT" && pwd)'"
  assert_contains "$(cat "$SETUP_WRAPPER")" "mcp --planner --start-work"
  assert_contains "$(cat "$TMP/out")" "does not run this wrapper yet"
  case "$(cat "$TMP/out")" in *"planner-tunnel-candidate.sh status --checkout <checkout>"*) fail "setup-planner printed the point script's generic next steps" ;; esac
  [ ! -e "$CLAUDE_CFG" ] || fail "setup-planner wrote the Claude config without --register-claude"
  pass "setup-planner defaults to the checkout and serves --start-work"
fi

# --register-claude keeps other servers, backs up, and is idempotent.
mkdir -p "$(dirname "$CLAUDE_CFG")"
printf '%s\n' '{"mcpServers":{"other":{"command":"/bin/other"}},"theme":"dark"}' > "$CLAUDE_CFG"
if ! "${SETUP_ENV[@]}" "$SETUP_SCRIPT" --checkout "$SETUP_CHECKOUT" --no-build --register-claude "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "setup-planner --register-claude failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$SETUP_WRAPPER")" "PROJECT='$(cd "$POINT_PROJECT" && pwd)'"
  python3 - "$CLAUDE_CFG" "$SETUP_WRAPPER" <<'PY' || fail "Claude config not registered as expected: $(cat "$CLAUDE_CFG")"
import json, sys
cfg = json.load(open(sys.argv[1]))
assert cfg["theme"] == "dark"
assert cfg["mcpServers"]["other"] == {"command": "/bin/other"}
assert cfg["mcpServers"]["sop-planner"] == {"command": sys.argv[2]}
PY
  ls "$CLAUDE_CFG".bak-* >/dev/null 2>&1 || fail "setup-planner did not back up the Claude config"
  pass "setup-planner --register-claude adds sop-planner and keeps the rest"
fi
if ! "${SETUP_ENV[@]}" "$SETUP_SCRIPT" --checkout "$SETUP_CHECKOUT" --no-build --register-claude "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "setup-planner re-run failed: $(cat "$TMP/out")"
else
  assert_contains "$(cat "$TMP/out")" "already runs this wrapper"
  pass "setup-planner is idempotent once registered"
fi

# A different sop-planner entry is never overwritten.
printf '%s\n' '{"mcpServers":{"sop-planner":{"command":"/elsewhere/wrapper.sh"}}}' > "$CLAUDE_CFG"
if "${SETUP_ENV[@]}" "$SETUP_SCRIPT" --checkout "$SETUP_CHECKOUT" --no-build --register-claude "$POINT_PROJECT" >"$TMP/out" 2>&1; then
  fail "setup-planner overwrote a different sop-planner entry"
else
  assert_contains "$(cat "$CLAUDE_CFG")" "/elsewhere/wrapper.sh"
  pass "setup-planner refuses to overwrite a different sop-planner entry"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------

if [ "$FAILS" -ne 0 ]; then
  printf '\n%s failure(s)\n' "$FAILS" >&2
  exit 1
fi
printf '\nall candidate lifecycle tests passed\n'
