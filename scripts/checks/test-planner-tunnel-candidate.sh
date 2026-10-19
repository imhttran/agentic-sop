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

if ! run_candidate start >"$TMP/out" 2>&1; then
  fail "start failed: $(cat "$TMP/out")"
  cat "$TMP/out" >&2
  exit 1
fi
CAND_PID="$(mg process_pid)"
assert_alive "$CAND_PID"
assert_contains "$(mg process_command)" "$CANDIDATE"
assert_eq "$(mg state)" "candidate-running"
assert_file "$TMP/logs/candidate.jsonl"
pass "start launches the candidate and records PID, command, and log"

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
    "Repository" "Candidate executable" "Active process" "Local tests" \
    "ChatGPT live MCP results" "Rollback" "Exact read-only MCP test request" \
    "sop_status({})" "sop_question_get({\"question_id\":\"q-1\"})" \
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
# Summary
# ---------------------------------------------------------------------------

if [ "$FAILS" -ne 0 ]; then
  printf '\n%s failure(s)\n' "$FAILS" >&2
  exit 1
fi
printf '\nall candidate lifecycle tests passed\n'
