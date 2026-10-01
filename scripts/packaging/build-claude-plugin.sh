#!/usr/bin/env sh
# Generate the Claude Code plugin package from the canonical SOP skill tree.
#
# skills/ is the single source of truth for every SOP command, so the plugin must not
# become a second, hand-maintained copy of it. This script mirrors skills/ into the
# plugin verbatim:
#
#   skills/sop-review/SKILL.md -> integrations/claude/skills/sop-review/SKILL.md
#
# It owns exactly two paths under integrations/claude/: skills/ (a generated mirror)
# and README.md (a generated banner). integrations/claude/.claude-plugin/plugin.json
# is hand-written metadata and is never touched.
#
# Claude Code namespaces every plugin component under the plugin name, so the plugin
# named "sop" exposes these as /sop:sop-plan, /sop:sop-review, and so on; the same
# names work unprefixed when no other skill claims them.
#
# Usage:
#   scripts/packaging/build-claude-plugin.sh          regenerate the mirror
#   scripts/packaging/build-claude-plugin.sh --check  verify the committed mirror is current
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# This script lives at scripts/packaging/build-claude-plugin.sh, so the repository root
# is two levels up.
repo_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)

src="$repo_dir/skills"
plugin_dir="$repo_dir/integrations/claude"
dst="$plugin_dir/skills"
readme="$plugin_dir/README.md"

check=0
case "${1:-}" in
  --check) check=1 ;;
  "") ;;
  *)
    echo "build-claude-plugin: unknown argument: $1" >&2
    echo "usage: scripts/packaging/build-claude-plugin.sh [--check]" >&2
    exit 2
    ;;
esac

# The only directory this script may replace. Guard it so a bad variable can never
# turn the mirror step into a delete of something else.
case "$dst" in
  "$repo_dir"/integrations/claude/skills) ;;
  *)
    echo "build-claude-plugin: refusing to write $dst" >&2
    exit 1
    ;;
esac

if [ ! -d "$src" ]; then
  echo "build-claude-plugin: the canonical skills tree is missing: $src" >&2
  exit 1
fi

stage=$(mktemp -d "${TMPDIR:-/tmp}/sop-claude-plugin.XXXXXX")
trap 'rm -rf "$stage"' EXIT INT TERM

# The mirror is the canonical tree, byte for byte.
cp -R "$src/." "$stage/skills"
cp "$script_dir/claude-plugin-README.md" "$stage/README.md"

if [ "$check" -eq 1 ]; then
  status=0
  if [ ! -d "$dst" ]; then
    echo "build-claude-plugin: $dst is missing; run scripts/packaging/build-claude-plugin.sh" >&2
    status=1
  elif ! diff -r -q "$dst" "$stage/skills" >/dev/null 2>&1; then
    echo "build-claude-plugin: $dst has drifted from skills/:" >&2
    diff -r -q "$dst" "$stage/skills" 2>&1 | sed 's/^/  /' >&2
    echo "build-claude-plugin: run scripts/packaging/build-claude-plugin.sh" >&2
    status=1
  fi
  if [ ! -f "$readme" ] || ! diff -q "$readme" "$stage/README.md" >/dev/null 2>&1; then
    echo "build-claude-plugin: $readme is out of date; run scripts/packaging/build-claude-plugin.sh" >&2
    status=1
  fi
  if [ "$status" -eq 0 ]; then
    echo "build-claude-plugin: the plugin package is current"
  fi
  exit "$status"
fi

rm -rf "$dst"
mkdir -p "$plugin_dir"
mv "$stage/skills" "$dst"
cp "$stage/README.md" "$readme"
echo "build-claude-plugin: wrote $dst"
echo "build-claude-plugin: wrote $readme"
