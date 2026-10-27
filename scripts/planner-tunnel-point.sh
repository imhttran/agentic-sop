#!/usr/bin/env bash
#
# Point the planner MCP tunnel wrapper at a SOP project.
#
# The Secure MCP Tunnel profile runs .run/tunnel/sop-mcp-planner.sh, which starts
# `sop mcp --planner` inside one project directory. This script rewrites that
# wrapper for the project you name, optionally builds a fresh sop binary from
# origin/main first, and checks the wrapper locally with a read-only
# sop_question_list call. It never restarts the tunnel; use
# scripts/planner-tunnel-candidate.sh for the verified process lifecycle.
#
# Usage:
#   scripts/planner-tunnel-point.sh [--dry-run] [--build] [--start-work] [--no-next-steps]
#                                   [--binary PATH] [--checkout PATH] PROJECT_DIR
#
#   PROJECT_DIR     the SOP project the planner should serve (must contain .agent-sdlc)
#   --dry-run       check the project, binary, build inputs, and wrapper locally, but
#                   write nothing and leave the live wrapper unchanged
#   --build         build sop from origin/main into the binary path first
#   --start-work    serve `sop mcp --planner --start-work`, adding sop_start_work and
#                   sop_work_status; the local check then also requires sop_start_work
#   --no-next-steps do not print the tunnel restart instructions (for callers such
#                   as scripts/setup-planner.sh that print their own)
#   --binary PATH   the sop binary the wrapper runs (default: .run/spike/sop-planner)
#   --checkout PATH the agentic-sop checkout whose .run/tunnel and .run/spike to use
#                   (default: the checkout this script runs from). Use it to run the
#                   script from another worktree, for example a clean main, without
#                   switching branches in the checkout the tunnel profile points at.
#
#   Relative PROJECT_DIR and --binary paths are resolved against the checkout.
#
# Examples (back to the old ChatGPT demo, with a fresh binary from main):
#   scripts/planner-tunnel-point.sh --build .run/planner-chatgpt
#   "$TMPDIR/sop-main/scripts/planner-tunnel-point.sh" --build \
#     --checkout ~/agentic-workspace/agentic-sop .run/planner-chatgpt
#
# The new wrapper is written to a temporary file and checked first; the live
# wrapper is replaced only if the check passes. An existing wrapper is copied to
# .run/tunnel/sop-mcp-planner.sh.bak-<timestamp> before it is replaced.
#
# This script does not start, stop, or restart the tunnel. Use
# scripts/planner-tunnel-candidate.sh for the candidate lifecycle. The local
# wrapper check below is a local MCP check, not a live ChatGPT test.

set -euo pipefail

usage() { sed -n '/^# Usage:/,/^#   Relative/p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }

# Quote a value as a single-quoted shell literal, safe for any path.
shquote() { local q="'\\''"; printf "'%s'" "${1//\'/$q}"; }

TREE=""
CANDIDATE=""
cleanup() {
  [ -z "$CANDIDATE" ] || rm -f "$CANDIDATE"
  [ -z "$TREE" ] || git -C "$ROOT" worktree remove --force "$TREE" >/dev/null 2>&1 || true
}
trap cleanup EXIT

BUILD=0
DRY_RUN=0
START_WORK=0
NEXT_STEPS=1
BINARY=""
CHECKOUT=""
PROJECT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --build) BUILD=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --start-work) START_WORK=1 ;;
    --no-next-steps) NEXT_STEPS=0 ;;
    --binary) [ $# -ge 2 ] || usage; BINARY="$2"; shift ;;
    --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
    -h|--help) usage ;;
    -*) echo "point: unknown option $1" >&2; usage ;;
    *) [ -z "$PROJECT" ] || usage; PROJECT="$1" ;;
  esac
  shift
done
[ -n "$PROJECT" ] || usage

if [ -n "$CHECKOUT" ]; then
  [ -d "$CHECKOUT" ] || { echo "point: --checkout directory not found: $CHECKOUT" >&2; exit 1; }
  ROOT="$(git -C "$CHECKOUT" rev-parse --show-toplevel 2>/dev/null)" || {
    echo "point: --checkout is not a git checkout: $CHECKOUT" >&2; exit 1; }
else
  SELF_DIR="$(cd "$(dirname "$0")" && pwd -P)"
  ROOT="$(git -C "$SELF_DIR" rev-parse --show-toplevel 2>/dev/null)" || {
    echo "point: run this script from inside the agentic-sop checkout, or pass --checkout" >&2; exit 1; }
fi
cd "$ROOT"
echo "point: checkout = $ROOT"

WRAPPER="$ROOT/.run/tunnel/sop-mcp-planner.sh"
BINARY="${BINARY:-$ROOT/.run/spike/sop-planner}"
case "$BINARY" in /*) ;; *) BINARY="$ROOT/$BINARY" ;; esac

[ -d "$PROJECT" ] || { echo "point: project directory not found: $PROJECT" >&2; exit 1; }
PROJECT="$(cd "$PROJECT" && pwd)"
[ -d "$PROJECT/.agent-sdlc" ] || {
  echo "point: $PROJECT is not a SOP project (no .agent-sdlc); run sop init or sop run there first" >&2; exit 1; }

# 1. Optionally build sop from origin/main in a temporary worktree, so the
#    current checkout and its local changes are never touched.
if [ "$BUILD" = 1 ]; then
  command -v go >/dev/null 2>&1 || { echo "point: go is required for --build" >&2; exit 1; }
  if [ "$DRY_RUN" = 1 ]; then
    echo "point: dry-run: would fetch origin/main and build $(shquote "$BINARY") from a temporary worktree"
  else
    git fetch -q origin main
    TREE="$(mktemp -d "${TMPDIR:-/tmp}/sop-main.XXXXXX")"
    rmdir "$TREE"
    git worktree add -q --detach "$TREE" origin/main
    mkdir -p "$(dirname "$BINARY")"
    echo "point: building sop from origin/main ($(git rev-parse --short origin/main)) -> $BINARY"
    (cd "$TREE" && go build -o "$BINARY" ./cmd/sop)
  fi
fi
if [ "$DRY_RUN" = 1 ] && [ ! -x "$BINARY" ]; then
  echo "point: dry-run: binary does not exist yet; --build would create it" >&2
elif [ ! -x "$BINARY" ]; then
  echo "point: sop binary not found or not executable: $BINARY (use --build)" >&2
  exit 1
fi

# 2. Write the new wrapper to a temporary file with absolute, quoted paths: the
#    tunnel does not see this shell's environment. PWD is exported so the server
#    reports the same path spelling as the shell that ran sop run (needed only by
#    binaries older than the repository-identity fix; harmless otherwise).
if [ "$DRY_RUN" = 1 ]; then
  CANDIDATE="$(mktemp "${TMPDIR:-/tmp}/sop-planner-wrapper.XXXXXX")"
else
  mkdir -p "$(dirname "$WRAPPER")"
  CANDIDATE="$(mktemp "$WRAPPER.new.XXXXXX")"
fi
MCP_ARGS="mcp --planner"
[ "$START_WORK" = 1 ] && MCP_ARGS="mcp --planner --start-work"
cat > "$CANDIDATE" <<EOF
#!/bin/sh
# Planner MCP wrapper.
# Written by scripts/planner-tunnel-point.sh on $(date '+%Y-%m-%d %H:%M:%S').
PROJECT=$(shquote "$PROJECT")
cd "\$PROJECT" || exit 1
export PWD="\$PROJECT"
exec $(shquote "$BINARY") $MCP_ARGS
EOF
chmod +x "$CANDIDATE"

if [ "$DRY_RUN" = 1 ] && [ ! -x "$BINARY" ]; then
  echo "point: dry-run: wrapper would contain:"
  sed 's/^/  /' "$CANDIDATE"
  echo "point: dry-run: local MCP check skipped (binary not built yet)"
  exit 0
fi

# 3. Check the new wrapper locally with a read-only call before installing it.
#    With --start-work, also list the tools: a binary older than sop_start_work
#    rejects the flag or lacks the tool, and must not replace the live wrapper.
REQUESTS="$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"planner-tunnel-point","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"sop_question_list","arguments":{}}}')"
if [ "$START_WORK" = 1 ]; then
  REQUESTS="$REQUESTS
{\"jsonrpc\":\"2.0\",\"id\":3,\"method\":\"tools/list\"}"
fi
OUTPUT="$(printf '%s\n' "$REQUESTS" | "$CANDIDATE" 2>&1 || true)"
REPLY="$(printf '%s\n' "$OUTPUT" | grep '"id":2' | tail -1)"
[ -n "$REPLY" ] || REPLY="$(printf '%s\n' "$OUTPUT" | tail -1)"
if [ "$START_WORK" = 1 ]; then
  case "$(printf '%s\n' "$OUTPUT" | grep '"id":3' | tail -1)" in
    *'"sop_start_work"'*) echo "point: local check ok; sop_start_work is served" ;;
    *)
      echo "point: local check FAILED; $(shquote "$BINARY") does not serve sop_start_work (use --build for a current binary). The live wrapper was left unchanged. Output:" >&2
      echo "$OUTPUT" >&2
      exit 1 ;;
  esac
fi
case "$REPLY" in
  *'"id":2'*'"isError":false'*)
    TEXT="$(printf '%s' "$REPLY" | sed -n 's/.*"text":"\([^"]*\)".*/\1/p')"
    echo "point: local check ok; sop_question_list -> ${TEXT:-$REPLY}" ;;
  *)
    echo "point: local check FAILED; the live wrapper was left unchanged. The new wrapper replied:" >&2
    echo "$REPLY" >&2
    exit 1 ;;
esac

# 4. Back up the current wrapper, then atomically install the checked one.
if [ "$DRY_RUN" = 1 ]; then
  if [ -f "$WRAPPER" ]; then
    echo "point: dry-run: would back up $WRAPPER and install the checked wrapper"
  else
    echo "point: dry-run: would install the checked wrapper at $WRAPPER"
  fi
  echo "point: dry-run: live wrapper left unchanged"
  exit 0
fi
if [ -f "$WRAPPER" ]; then
  BACKUP="$WRAPPER.bak-$(date +%Y%m%d-%H%M%S)"
  cp "$WRAPPER" "$BACKUP"
  echo "point: previous wrapper saved to $BACKUP"
fi
mv -f "$CANDIDATE" "$WRAPPER"
CANDIDATE=""
echo "point: wrapper now serves $PROJECT"

[ "$NEXT_STEPS" = 1 ] || exit 0
cat <<'EOF'

Next: the wrapper is configured, but the tunnel was not restarted. Use the
candidate helper so the exact process is verified and rollback is preserved:

  scripts/planner-tunnel-candidate.sh status --checkout <checkout>
  scripts/planner-tunnel-candidate.sh start  --checkout <checkout>

If the tunnel is already running an unverified process, stop it explicitly
before starting a candidate. Do not use a broad pkill/killall; the candidate
helper refuses to signal a PID it cannot verify.
EOF
