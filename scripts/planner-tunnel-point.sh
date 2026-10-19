#!/usr/bin/env bash
#
# Point the planner MCP tunnel wrapper at a SOP project.
#
# The Secure MCP Tunnel profile runs .run/tunnel/sop-mcp-planner.sh, which starts
# `sop mcp --planner` inside one project directory. This script rewrites that
# wrapper for the project you name, optionally builds a fresh sop binary from
# origin/main first, and checks the wrapper locally with a read-only
# sop_question_list call. It never restarts the tunnel client; it prints the
# command to do that.
#
# Usage:
#   scripts/planner-tunnel-point.sh [--build] [--binary PATH] PROJECT_DIR
#
#   PROJECT_DIR   the SOP project the planner should serve (must contain .agent-sdlc)
#   --build       build sop from origin/main into the binary path first
#   --binary PATH the sop binary the wrapper runs (default: .run/spike/sop-planner)
#
# Example (back to the old ChatGPT demo, with a fresh binary from main):
#   scripts/planner-tunnel-point.sh --build .run/planner-chatgpt
#
# An existing wrapper is copied to .run/tunnel/sop-mcp-planner.sh.bak-<timestamp>
# before it is replaced.

set -euo pipefail

usage() { sed -n '12,18p' "$0" | sed 's/^# \{0,1\}//' >&2; exit 2; }

BUILD=0
BINARY=""
PROJECT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --build) BUILD=1 ;;
    --binary) [ $# -ge 2 ] || usage; BINARY="$2"; shift ;;
    -h|--help) usage ;;
    -*) echo "point: unknown option $1" >&2; usage ;;
    *) [ -z "$PROJECT" ] || usage; PROJECT="$1" ;;
  esac
  shift
done
[ -n "$PROJECT" ] || usage

SELF_DIR="$(cd "$(dirname "$0")" && pwd -P)"
ROOT="$(git -C "$SELF_DIR" rev-parse --show-toplevel 2>/dev/null)" || {
  echo "point: run this script from inside the agentic-sop checkout" >&2; exit 1; }
cd "$ROOT"

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
  git fetch -q origin main
  TREE="$(mktemp -d "${TMPDIR:-/tmp}/sop-main.XXXXXX")"
  rmdir "$TREE"
  git worktree add -q --detach "$TREE" origin/main
  trap 'git -C "$ROOT" worktree remove --force "$TREE" >/dev/null 2>&1 || true' EXIT
  mkdir -p "$(dirname "$BINARY")"
  echo "point: building sop from origin/main ($(git rev-parse --short origin/main)) -> $BINARY"
  (cd "$TREE" && go build -o "$BINARY" ./cmd/sop)
fi
[ -x "$BINARY" ] || { echo "point: sop binary not found or not executable: $BINARY (use --build)" >&2; exit 1; }

# 2. Back up the current wrapper, then write the new one with absolute paths: the
#    tunnel does not see this shell's environment. PWD is exported so the server
#    reports the same path spelling as the shell that ran sop run (needed only by
#    binaries older than the repository-identity fix; harmless otherwise).
mkdir -p "$(dirname "$WRAPPER")"
if [ -f "$WRAPPER" ]; then
  BACKUP="$WRAPPER.bak-$(date +%Y%m%d-%H%M%S)"
  cp "$WRAPPER" "$BACKUP"
  echo "point: previous wrapper saved to $BACKUP"
fi
cat > "$WRAPPER" <<EOF
#!/bin/sh
# Planner MCP for $PROJECT
# Written by scripts/planner-tunnel-point.sh on $(date '+%Y-%m-%d %H:%M:%S').
PROJECT="$PROJECT"
cd "\$PROJECT" || exit 1
export PWD="\$PROJECT"
exec "$BINARY" mcp --planner
EOF
chmod +x "$WRAPPER"
echo "point: wrapper now serves $PROJECT"

# 3. Check the wrapper locally with a read-only call.
REPLY="$(printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"planner-tunnel-point","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"sop_question_list","arguments":{}}}' \
  | "$WRAPPER" 2>&1 | tail -1)"
case "$REPLY" in
  *'"id":2'*'"isError":false'*)
    TEXT="$(printf '%s' "$REPLY" | sed -n 's/.*"text":"\([^"]*\)".*/\1/p')"
    echo "point: local check ok; sop_question_list -> ${TEXT:-$REPLY}" ;;
  *)
    echo "point: local check FAILED; the wrapper replied:" >&2
    echo "$REPLY" >&2
    exit 1 ;;
esac

cat <<'EOF'

Next: restart the tunnel client so it starts the new wrapper.
  1. Press Ctrl-C in the tab running tunnel-client (or: pkill -f "tunnel-client run").
  2. Start it again:
     tunnel-client run --profile sop-status-spike
EOF
