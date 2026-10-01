#!/usr/bin/env sh
# Install (or remove) the SOP Zed skills.
#
# Zed discovers skills as direct children of a skills root, and exposes each one as a
# slash command named after its folder:
#
#   global    ~/.agents/skills/<name>/SKILL.md     every project
#   project   <project>/.agents/skills/<name>/SKILL.md   that project only
#
# This script links the SOP skills from this checkout's skills/ tree into the selected
# scope, so the checkout stays the single source of truth. It is idempotent, changes
# only SOP-owned entries, and needs no root privileges.
#
# Usage:
#   scripts/install-zed-skills.sh [--global | --project [DIR]]
#                                 [--uninstall] [--dry-run] [--force]
#
#   --global        install into ~/.agents/skills (the default)
#   --project [DIR] install into DIR/.agents/skills (DIR defaults to the current dir)
#   --uninstall     remove the SOP skill entries from the scope
#   --dry-run       print what would happen, change nothing
#   --force         replace a SOP-named symlink that points somewhere else
#
# Zed follows symlinks, so the installed skills stay live against this checkout. A
# non-symlink entry at a SOP-owned name is never deleted: remove it yourself first.
#
# After installing, open Zed's agent message editor and type "/" to find
# /sop, /sop-prompt, /sop-plan, /sop-review, /sop-diagnose, /sop-test, /sop-implement.
set -eu

# The canonical SOP skill set. Keep in sync with internal/skill (the build fails if
# this list and the shipped skills/ tree drift).
skills="sop sop-prompt sop-plan sop-review sop-diagnose sop-test sop-implement"

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_dir=$(CDPATH= cd -- "$script_dir/.." && pwd)
source_dir="$repo_dir/skills"

usage() {
  cat <<'EOF'
Usage: scripts/install-zed-skills.sh [--global | --project [DIR]]
                                     [--uninstall] [--dry-run] [--force]

  --global         install into ~/.agents/skills (the default)
  --project [DIR]  install into DIR/.agents/skills (DIR defaults to the current dir)
  --uninstall      remove the SOP skill entries from the scope
  --dry-run        print what would happen, change nothing
  --force          replace a SOP-named symlink that points somewhere else
EOF
}

scope=global
project_dir=""
uninstall=0
dry_run=0
force=0

while [ $# -gt 0 ]; do
  case "$1" in
    --global) scope=global; shift ;;
    --project)
      scope=project
      # An optional explicit directory follows, unless the next argument is a flag.
      if [ $# -ge 2 ] && [ "${2#--}" = "$2" ]; then
        project_dir=$2
        shift
      fi
      shift
      ;;
    --project=*) scope=project; project_dir=${1#--project=}; shift ;;
    --uninstall) uninstall=1; shift ;;
    --dry-run) dry_run=1; shift ;;
    --force) force=1; shift ;;
    -h | --help) usage; exit 0 ;;
    *)
      echo "install-zed-skills: unknown argument: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

if [ "$scope" = project ]; then
  if [ -z "$project_dir" ]; then
    project_dir=$PWD
  fi
  if [ ! -d "$project_dir" ]; then
    echo "install-zed-skills: project directory does not exist: $project_dir" >&2
    exit 1
  fi
  project_dir=$(CDPATH= cd -- "$project_dir" && pwd)
  root="$project_dir/.agents/skills"
else
  root="$HOME/.agents/skills"
fi

if [ "$(id -u)" = 0 ]; then
  echo "install-zed-skills: warning: skills are per-user; root is not required" >&2
fi

# run performs an action, or prints it under --dry-run.
run() {
  if [ "$dry_run" -eq 1 ]; then
    echo "would: $*"
  else
    "$@"
  fi
}

# Report the fail-closed precondition: the skills drive the `sop` CLI, so it must be
# installed before they can do anything.
if command -v sop >/dev/null 2>&1; then
  :
else
  echo "install-zed-skills: warning: 'sop' is not on PATH; the skills will fail closed" >&2
  echo "install-zed-skills:          until it is installed (see the project README / 'make install')" >&2
fi

if [ ! -d "$source_dir" ]; then
  echo "install-zed-skills: source skills directory is missing: $source_dir" >&2
  exit 1
fi

if [ "$uninstall" -eq 0 ]; then
  run mkdir -p "$root"
fi

changed=0
skipped=0
for name in $skills; do
  src="$source_dir/$name"
  dest="$root/$name"

  if [ "$uninstall" -eq 1 ]; then
    if [ -L "$dest" ]; then
      target=$(readlink "$dest" || true)
      case "$target" in
        "$source_dir"/*)
          run rm -f "$dest"
          echo "removed $dest"
          changed=$((changed + 1))
          ;;
        *)
          echo "skip: $dest is a symlink to $target (not SOP's); leaving it" >&2
          skipped=$((skipped + 1))
          ;;
      esac
    elif [ -e "$dest" ]; then
      echo "skip: $dest exists and is not a SOP symlink; leaving it" >&2
      skipped=$((skipped + 1))
    fi
    continue
  fi

  if [ ! -f "$src/SKILL.md" ]; then
    echo "install-zed-skills: missing source skill: $src/SKILL.md" >&2
    exit 1
  fi

  if [ -L "$dest" ]; then
    target=$(readlink "$dest" || true)
    if [ "$target" = "$src" ]; then
      echo "ok: $dest"
      continue
    fi
    if [ "$force" -eq 1 ]; then
      run rm -f "$dest"
    else
      echo "skip: $dest is a symlink to $target; re-run with --force to replace it" >&2
      skipped=$((skipped + 1))
      continue
    fi
  elif [ -e "$dest" ]; then
    echo "skip: $dest exists and is not a symlink; remove it yourself, then re-run" >&2
    skipped=$((skipped + 1))
    continue
  fi

  run ln -s "$src" "$dest"
  echo "linked $dest -> $src"
  changed=$((changed + 1))
done

if [ "$uninstall" -eq 1 ]; then
  echo
  echo "done ($changed removed, $skipped skipped)."
else
  echo
  echo "done ($changed changed, $skipped skipped)."
  echo "  scope: $root"
  echo "  in Zed, type \"/\" in the agent message editor to find the SOP commands."
fi
