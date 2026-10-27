#!/usr/bin/env bash
#
# Set up the planner MCP server for a SOP project in one step.
#
# It builds a current sop from origin/main, points the planner wrapper
# (.run/tunnel/sop-mcp-planner.sh) at PROJECT_DIR serving
# `sop mcp --planner --start-work`, checks it locally, and then reports whether
# the Claude desktop app is registered to run that wrapper. All wrapper work is
# delegated to scripts/planner-tunnel-point.sh, so there is one implementation.
#
# Usage:
#   scripts/setup-planner.sh [--no-build] [--no-start-work] [--register-claude]
#                            [--dry-run] [--checkout PATH] [PROJECT_DIR]
#
#   PROJECT_DIR        the SOP project the planner should serve (must contain
#                      .agent-sdlc). Default: the agentic-sop checkout itself.
#   --no-build         reuse the existing planner binary instead of building one
#   --no-start-work    serve the planner surface without sop_start_work
#   --register-claude  add the wrapper to the Claude desktop app's MCP servers as
#                      "sop-planner" if it is not there yet (the config is backed
#                      up first); without it the script only reports and prints
#                      the entry to add
#   --dry-run          check everything, change nothing
#   --checkout PATH    the agentic-sop checkout to use (default: the one this
#                      script is in; the installed sop-setup-planner passes it)
#
# `./install.sh --planner-setup` installs this as `sop-setup-planner` on PATH, and
# `make setup-planner [PROJECT=DIR]` runs it from the checkout.
#
# It never restarts the Claude desktop app or the ChatGPT tunnel; it prints how.

set -euo pipefail

usage() { sed -n '/^# Usage:/,/^# `\.\/install/p' "$0" | sed '$d' | sed 's/^# \{0,1\}//' >&2; exit 2; }

BUILD=1
START_WORK=1
REGISTER=0
DRY_RUN=0
CHECKOUT=""
PROJECT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --no-build) BUILD=0 ;;
    --no-start-work) START_WORK=0 ;;
    --register-claude) REGISTER=1 ;;
    --dry-run) DRY_RUN=1 ;;
    --checkout) [ $# -ge 2 ] || usage; CHECKOUT="$2"; shift ;;
    --checkout=*) CHECKOUT="${1#--checkout=}" ;;
    -h|--help) usage ;;
    -*) echo "setup-planner: unknown option $1" >&2; usage ;;
    *) [ -z "$PROJECT" ] || usage; PROJECT="$1" ;;
  esac
  shift
done

if [ -z "$CHECKOUT" ]; then
  CHECKOUT="$(cd "$(dirname "$0")" && pwd -P)"
fi
[ -d "$CHECKOUT" ] || { echo "setup-planner: checkout not found: $CHECKOUT" >&2; exit 1; }
ROOT="$(git -C "$CHECKOUT" rev-parse --show-toplevel 2>/dev/null)" || {
  echo "setup-planner: not an agentic-sop checkout: $CHECKOUT" >&2; exit 1; }
POINT="$ROOT/scripts/planner-tunnel-point.sh"
[ -x "$POINT" ] || { echo "setup-planner: missing $POINT; is $ROOT an agentic-sop checkout?" >&2; exit 1; }

PROJECT="${PROJECT:-$ROOT}"
case "$PROJECT" in
  "~") PROJECT="$HOME" ;;
  "~/"*) PROJECT="$HOME/${PROJECT#\~/}" ;;
esac
WRAPPER="$ROOT/.run/tunnel/sop-mcp-planner.sh"

# 1. Build (optionally), point, and check the wrapper.
ARGS=(--checkout "$ROOT" --no-next-steps)
[ "$BUILD" = 1 ] && ARGS+=(--build)
[ "$START_WORK" = 1 ] && ARGS+=(--start-work)
[ "$DRY_RUN" = 1 ] && ARGS+=(--dry-run)
"$POINT" "${ARGS[@]}" "$PROJECT"

# 2. The Claude desktop app runs the wrapper as a local MCP server.
case "$(uname -s)" in
  Darwin) CLAUDE_CONFIG="$HOME/Library/Application Support/Claude/claude_desktop_config.json" ;;
  *) CLAUDE_CONFIG="${XDG_CONFIG_HOME:-$HOME/.config}/Claude/claude_desktop_config.json" ;;
esac

# claude_status prints "registered", "missing", "no-config", or "invalid".
claude_status() {
  [ -f "$CLAUDE_CONFIG" ] || { echo no-config; return; }
  if command -v python3 >/dev/null 2>&1; then
    python3 - "$CLAUDE_CONFIG" "$WRAPPER" <<'PY'
import json, sys
path, wrapper = sys.argv[1], sys.argv[2]
try:
    with open(path) as f:
        cfg = json.load(f)
except Exception:
    print("invalid"); sys.exit()
servers = cfg.get("mcpServers") if isinstance(cfg, dict) else None
found = isinstance(servers, dict) and any(
    isinstance(s, dict) and s.get("command") == wrapper for s in servers.values())
print("registered" if found else "missing")
PY
  elif grep -qF "\"$WRAPPER\"" "$CLAUDE_CONFIG"; then
    echo registered
  else
    echo missing
  fi
}

# register_claude adds {"sop-planner": {"command": WRAPPER}} after a backup. It
# refuses to overwrite a different "sop-planner" entry.
register_claude() {
  command -v python3 >/dev/null 2>&1 || { echo "setup-planner: --register-claude needs python3" >&2; return 1; }
  mkdir -p "$(dirname "$CLAUDE_CONFIG")"
  if [ -f "$CLAUDE_CONFIG" ]; then
    BACKUP="$CLAUDE_CONFIG.bak-$(date +%Y%m%d-%H%M%S)"
    cp "$CLAUDE_CONFIG" "$BACKUP"
    echo "setup-planner: Claude config backed up to $BACKUP"
  fi
  python3 - "$CLAUDE_CONFIG" "$WRAPPER" <<'PY'
import json, os, sys
path, wrapper = sys.argv[1], sys.argv[2]
cfg = {}
if os.path.exists(path) and os.path.getsize(path) > 0:
    with open(path) as f:
        cfg = json.load(f)
servers = cfg.setdefault("mcpServers", {})
existing = servers.get("sop-planner")
if existing is not None and existing.get("command") != wrapper:
    sys.exit("setup-planner: an MCP server named sop-planner already runs %r; "
             "edit the Claude config by hand" % existing.get("command"))
servers["sop-planner"] = {"command": wrapper}
tmp = path + ".tmp"
with open(tmp, "w") as f:
    json.dump(cfg, f, indent=2)
    f.write("\n")
os.replace(tmp, path)
PY
  echo "setup-planner: registered sop-planner in $CLAUDE_CONFIG"
}

echo
STATUS="$(claude_status)"
case "$STATUS" in
  registered)
    echo "setup-planner: the Claude desktop app already runs this wrapper." ;;
  invalid)
    echo "setup-planner: $CLAUDE_CONFIG is not valid JSON; fix it before registering." >&2 ;;
  *)
    if [ "$REGISTER" = 1 ] && [ "$DRY_RUN" = 0 ]; then
      register_claude
    else
      [ "$REGISTER" = 1 ] && echo "setup-planner: dry-run: would register sop-planner in $CLAUDE_CONFIG"
      cat <<EOF
setup-planner: the Claude desktop app does not run this wrapper yet. Re-run with
--register-claude, or add this under "mcpServers" in
$CLAUDE_CONFIG:

  "sop-planner": { "command": "$WRAPPER" }
EOF
    fi ;;
esac

[ "$DRY_RUN" = 1 ] && { echo; echo "setup-planner: dry-run: nothing was changed"; exit 0; }

cat <<EOF

Planner ready for $(cd "$PROJECT" && pwd).

  Claude desktop: quit and reopen the app; the sop-planner tools appear in new
                  and linked chats.
  ChatGPT tunnel: it runs the same wrapper. Restart it through the verified helper:
                    scripts/planner-tunnel-candidate.sh status --checkout $ROOT
                    scripts/planner-tunnel-candidate.sh start  --checkout $ROOT
EOF
