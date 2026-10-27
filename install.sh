#!/usr/bin/env sh
# Agentic SOP installer.
#
# Installs the `sop` CLI and, optionally, the agent integrations -- without Make and
# without root:
#
#   ./install.sh                     the sop CLI
#   ./install.sh --skills zed        + the Zed skills (/sop, /sop-plan, ...)
#   ./install.sh --skills claude     + the Claude Code skills (same commands)
#   ./install.sh --plugin claude     + prepare the Claude Code plugin
#   ./install.sh --agent claude      + the Claude Code agent wrapper (sop-agent)
#   ./install.sh --planner-setup     + sop-setup-planner, the planner MCP setup command
#   ./install.sh --all               CLI + every present agent's skills + the plugin
#                                    + the agent wrapper + sop-setup-planner
#
# The CLI is built from this checkout and written to a user-writable bin directory
# (--bin-dir, else $SOP_BIN_DIR, else $GOBIN, else `go env GOPATH`/bin). This script
# never writes outside that directory and the agent skill roots the skill installer
# owns, and it never edits a shell startup file.
#
# Installation is delegated, not duplicated:
#   * the CLI is built here (the one place that builds it);
#   * the agent skills are installed by scripts/install/install-skills.sh, which owns
#     the filesystem skill logic for every supported agent;
#   * the Claude plugin is a committed package under integrations/claude/, generated
#     from skills/ by scripts/packaging/build-claude-plugin.sh. Claude Code registers
#     plugins from within Claude Code, so this script verifies the package and prints
#     the exact official steps instead of writing into Claude's settings.
#
# Every step is idempotent, and only SOP-owned files are ever touched.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$script_dir

skills=none
plugin=none
agent=none
planner_setup=0
bin_dir=""
dry_run=0
# failed records whether any step that ran reported a problem. Every step still runs,
# so one failure never hides what the others achieved (or prints fewer instructions).
failed=0

usage() {
  cat <<'EOF'
Usage: ./install.sh [OPTIONS]

Installs the sop CLI, and optionally the agent integrations. No root is required,
and no shell startup file is modified.

Options:
  --skills <none|zed|claude|all>   link the SOP commands for those agents
                                   (default: none)
  --plugin <none|claude>           prepare the Claude Code plugin package
                                   (default: none)
  --agent <none|claude>            install the Claude Code agent wrapper as
                                   sop-agent beside sop, for SOP_AGENT_COMMAND
                                   (default: none)
  --planner-setup                  install sop-setup-planner beside sop: it sets up
                                   the planner MCP server for a project (default
                                   project: this checkout)
  --all                            --skills all --plugin claude --agent claude
                                   --planner-setup
  --bin-dir <path>                 where to install the sop binary
                                   (default: $SOP_BIN_DIR, else $GOBIN, else
                                   `go env GOPATH`/bin)
  --dry-run                        print what would happen, change nothing
  -h, --help                       show this help

Examples:
  ./install.sh
  ./install.sh --skills zed
  ./install.sh --skills claude --plugin claude
  ./install.sh --all --dry-run

Next steps after installing:
  sop version                      confirm the CLI runs
  In Zed or Claude Code, type "/" in the agent editor to find /sop, /sop-plan,
  /sop-review, /sop-diagnose, /sop-test, /sop-implement.
EOF
}

# --- Arguments ---------------------------------------------------------------

while [ $# -gt 0 ]; do
  case "$1" in
    --skills)
      [ $# -ge 2 ] || { echo "install: --skills requires a value" >&2; exit 2; }
      skills=$2
      shift 2
      ;;
    --skills=*) skills=${1#--skills=}; shift ;;
    --plugin)
      [ $# -ge 2 ] || { echo "install: --plugin requires a value" >&2; exit 2; }
      plugin=$2
      shift 2
      ;;
    --plugin=*) plugin=${1#--plugin=}; shift ;;
    --agent)
      [ $# -ge 2 ] || { echo "install: --agent requires a value" >&2; exit 2; }
      agent=$2
      shift 2
      ;;
    --agent=*) agent=${1#--agent=}; shift ;;
    --planner-setup) planner_setup=1; shift ;;
    --all) skills=all; plugin=claude; agent=claude; planner_setup=1; shift ;;
    --bin-dir)
      [ $# -ge 2 ] || { echo "install: --bin-dir requires a path" >&2; exit 2; }
      bin_dir=$2
      shift 2
      ;;
    --bin-dir=*) bin_dir=${1#--bin-dir=}; shift ;;
    --dry-run) dry_run=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *)
      echo "install: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

case "$skills" in
  none | zed | claude | all) ;;
  *)
    echo "install: unknown --skills value: $skills (want none, zed, claude, or all)" >&2
    exit 2
    ;;
esac

case "$plugin" in
  none | claude) ;;
  *)
    echo "install: unknown --plugin value: $plugin (want none or claude)" >&2
    exit 2
    ;;
esac

case "$agent" in
  none | claude) ;;
  *)
    echo "install: unknown --agent value: $agent (want none or claude)" >&2
    exit 2
    ;;
esac

if [ ! -f "$repo_dir/go.mod" ] || [ ! -d "$repo_dir/cmd/sop" ]; then
  echo "install: run this from the agentic-sop checkout (no cmd/sop under $repo_dir)" >&2
  exit 1
fi

if [ "$(id -u)" = 0 ]; then
  echo "install: warning: SOP installs per-user; root is not required" >&2
fi

run() {
  if [ "$dry_run" -eq 1 ]; then
    echo "would: $*"
  else
    "$@"
  fi
}

# --- The sop CLI -------------------------------------------------------------

# resolve_bin_dir returns the user-writable directory the sop binary is installed
# into: the explicit --bin-dir first, then the environment, then the Go toolchain's
# own bin directory (where `go install ./cmd/sop` would put it), then ~/.local/bin.
resolve_bin_dir() {
  if [ -n "$bin_dir" ]; then
    echo "$bin_dir"
    return
  fi
  if [ -n "${SOP_BIN_DIR:-}" ]; then
    echo "$SOP_BIN_DIR"
    return
  fi
  if [ -n "${GOBIN:-}" ]; then
    echo "$GOBIN"
    return
  fi
  if command -v go >/dev/null 2>&1; then
    gopath=$(go env GOPATH 2>/dev/null || true)
    if [ -n "$gopath" ]; then
      echo "$gopath/bin"
      return
    fi
  fi
  echo "$HOME/.local/bin"
}

install_cli() {
  target_dir=$1
  dest="$target_dir/sop"

  if ! command -v go >/dev/null 2>&1; then
    echo "install: the Go toolchain ('go') is required to build sop and was not found on PATH" >&2
    echo "install: install Go (https://go.dev/dl/), then re-run ./install.sh" >&2
    exit 1
  fi

  echo "Installing the sop CLI:"
  echo "  from: $repo_dir"
  echo "  to:   $dest"
  run mkdir -p "$target_dir"

  if [ "$dry_run" -eq 1 ]; then
    echo "would: (cd $repo_dir && go build -o $dest ./cmd/sop)"
    return
  fi

  # Build beside the destination and rename, so an installed sop that is currently
  # running keeps its inode and a failed build never truncates a working binary.
  tmp="$target_dir/.sop.build.$$"
  trap 'rm -f "$tmp"' EXIT INT TERM
  ( cd "$repo_dir" && go build -o "$tmp" ./cmd/sop )
  chmod 0755 "$tmp"
  mv -f "$tmp" "$dest"
  trap - EXIT INT TERM
  echo "  installed: $dest"
}

# on_path reports whether dir is one of PATH's entries.
on_path() {
  case ":${PATH:-}:" in
    *:"$1":*) return 0 ;;
    *) return 1 ;;
  esac
}

# --- The agent integrations ---------------------------------------------------

install_skills() {
  requested=$1
  [ "$requested" = none ] && return 0

  echo
  echo "Installing the SOP skills ($requested):"
  if [ "$dry_run" -eq 1 ]; then
    "$repo_dir/scripts/install/install-skills.sh" "$requested" --dry-run || failed=1
  else
    "$repo_dir/scripts/install/install-skills.sh" "$requested" || failed=1
  fi
}

plugin_dir="$repo_dir/integrations/claude"

prepare_plugin() {
  requested=$1
  [ "$requested" = none ] && return 0

  echo
  echo "Preparing the Claude Code plugin:"
  if [ ! -f "$plugin_dir/.claude-plugin/plugin.json" ]; then
    echo "install: the plugin package is missing: $plugin_dir/.claude-plugin/plugin.json" >&2
    echo "install: regenerate it with scripts/packaging/build-claude-plugin.sh" >&2
    failed=1
    return 0
  fi
  if [ ! -f "$repo_dir/.claude-plugin/marketplace.json" ]; then
    echo "install: the marketplace file is missing: $repo_dir/.claude-plugin/marketplace.json" >&2
    failed=1
    return 0
  fi
  echo "  plugin:      $plugin_dir"
  echo "  marketplace: $repo_dir/.claude-plugin/marketplace.json"

  if [ "$dry_run" -eq 0 ] && command -v claude >/dev/null 2>&1; then
    echo "  validating with the claude CLI:"
    if claude plugin validate --strict "$plugin_dir" 2>&1 | sed 's/^/    /'; then
      echo "  validation passed"
    else
      echo "  warning: 'claude plugin validate' reported a problem (see above)" >&2
      failed=1
    fi
  fi

  cat <<EOF

  Claude Code installs plugins from inside Claude Code, so run one of these:

    Load it for one session (no settings change):
      claude --plugin-dir $plugin_dir

    Register this checkout as a marketplace, then install the plugin:
      claude plugin marketplace add $repo_dir
      claude plugin install sop@agentic-sop

    The same two steps from inside a session:
      /plugin marketplace add $repo_dir
      /plugin install sop@agentic-sop

  The plugin exposes the SOP commands as /sop:sop-plan, /sop:sop-review,
  /sop:sop-diagnose, /sop:sop-test, /sop:sop-implement (and /sop:sop), and the same
  names unprefixed (/sop-review, ...) when no other skill claims them. Every one of
  them calls sop prompt, so SOP keeps owning routing, providers, and approval.
EOF
}

# install_agent copies the Claude Code agent wrapper beside sop as sop-agent, so
# SOP_AGENT_COMMAND can name a stable installed path instead of this checkout.
install_agent() {
  requested=$1
  target_dir=$2
  [ "$requested" = none ] && return 0
  src="$repo_dir/scripts/agents/sop-agent.sh"
  dest="$target_dir/sop-agent"

  echo
  echo "Installing the Claude Code agent wrapper:"
  echo "  to: $dest"
  if [ "$dry_run" -eq 1 ]; then
    echo "would: cp $src $dest"
  else
    tmp="$target_dir/.sop-agent.$$"
    cp "$src" "$tmp"
    chmod 0755 "$tmp"
    mv -f "$tmp" "$dest"
    echo "  installed: $dest"
  fi
  for tool in claude jq; do
    command -v "$tool" >/dev/null 2>&1 || echo "  note: $tool is not on PATH; sop-agent needs it"
  done
  cat <<EOF

  Point SOP at it in the managed project's .env:

    SOP_AGENT_COMMAND="$dest"

  Re-run this installer after pulling to update it.
EOF
}

# install_planner_setup writes sop-setup-planner beside sop: a small shim that
# runs this checkout's scripts/setup-planner.sh, so the command works from any
# directory and always runs the checkout's current script.
install_planner_setup() {
  [ "$planner_setup" -eq 1 ] || return 0
  target_dir=$1
  src="$repo_dir/scripts/setup-planner.sh"
  dest="$target_dir/sop-setup-planner"

  echo
  echo "Installing the planner setup command:"
  echo "  to: $dest"
  if [ "$dry_run" -eq 1 ]; then
    echo "would: write a shim that runs $src"
    return 0
  fi
  # Single-quote the checkout path for the shim, escaping embedded quotes.
  quoted_repo=$(printf '%s' "$repo_dir" | sed "s/'/'\\\\''/g")
  tmp="$target_dir/.sop-setup-planner.$$"
  {
    echo '#!/bin/sh'
    echo '# Installed by agentic-sop install.sh; re-run the installer if the checkout moves.'
    echo "exec '$quoted_repo/scripts/setup-planner.sh' --checkout '$quoted_repo' \"\$@\""
  } >"$tmp"
  chmod 0755 "$tmp"
  mv -f "$tmp" "$dest"
  echo "  installed: $dest"
  echo
  echo "  Set up the planner MCP server (default project: $repo_dir):"
  echo
  echo "    sop-setup-planner [PROJECT_DIR]"
}

# --- Report ------------------------------------------------------------------

report_path() {
  target_dir=$1
  dest="$target_dir/sop"

  if on_path "$target_dir"; then
    echo
    echo "  $target_dir is on PATH."
    if found=$(command -v sop 2>/dev/null) && [ "$found" != "$dest" ]; then
      echo "  note: 'sop' currently resolves to $found"
    fi
    return
  fi

  cat <<EOF

SOP installed to:

  $dest

Add this directory to PATH:

  export PATH="$target_dir:\$PATH"

Add that line to your shell profile (~/.zshrc, ~/.bashrc, or ~/.profile) to keep it.
EOF
}

# --- Go ----------------------------------------------------------------------

target_dir=$(resolve_bin_dir)
echo "SOP installer"
echo "  repo:     $repo_dir"
echo "  bin dir:  $target_dir"
echo "  skills:   $skills"
echo "  plugin:   $plugin"
echo "  agent:    $agent"
if [ "$planner_setup" -eq 1 ]; then
  echo "  planner:  sop-setup-planner"
fi
if [ "$dry_run" -eq 1 ]; then
  echo "  dry run:  yes (nothing will be changed)"
fi
echo

install_cli "$target_dir"
install_skills "$skills"
prepare_plugin "$plugin"
install_agent "$agent" "$target_dir"
install_planner_setup "$target_dir"
report_path "$target_dir"

if [ "$failed" -eq 0 ]; then
  echo
  echo "Done."
  exit 0
fi
echo
echo "Finished with errors (see above)." >&2
exit 1
