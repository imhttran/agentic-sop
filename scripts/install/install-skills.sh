#!/usr/bin/env sh
# Install (or remove) the SOP agent skills for every coding agent that supports them.
#
# A skill is a folder containing a SKILL.md. Both Zed and Claude Code discover skills
# as direct children of a skills root and expose each one as a slash command named
# after its folder:
#
#   zed     global  ~/.agents/skills/<name>/SKILL.md          every project
#           project <project>/.agents/skills/<name>/SKILL.md   that project only
#   claude  global  ~/.claude/skills/<name>/SKILL.md          every project
#           project <project>/.claude/skills/<name>/SKILL.md   that project only
#
# The platforms share one canonical skill tree (this checkout's skills/), so this
# script links the same folders into whichever roots you ask for. The checkout stays
# the single source of truth and the installed skills stay live against it.
#
# It is idempotent, changes only SOP-owned entries, needs no root privileges, and
# never clobbers an entry it does not own.
#
# Usage:
#   scripts/install/install-skills.sh [TARGET] [--global | --project [DIR]]
#                                     [--uninstall] [--dry-run] [--force]
#
#   TARGET          zed | claude | all   (default: all)
#   --global        install into the target's user-scope skills root (the default)
#   --project [DIR] install into DIR's project-scope skills root (DIR defaults to
#                   the current directory)
#   --uninstall     remove the SOP skill entries from the scope
#   --dry-run       print what would happen, change nothing
#   --force         replace a SOP-named symlink that points somewhere else
#
# With `all` in the user scope, a target whose agent directory is absent is skipped;
# name the target explicitly to install it anyway. Thin wrappers live beside this
# script: scripts/install/install-zed-skills.sh and
# scripts/install/install-claude-skills.sh.
set -eu

# The canonical SOP skill set. Keep in sync with internal/skill (the build fails if
# this list and the shipped skills/ tree drift).
skills="sop sop-prompt sop-plan sop-review sop-diagnose sop-test sop-implement sop-end-to-end sop-historicalize"

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
# This script lives at scripts/install/install-skills.sh, so the repository root is two
# levels up.
repo_dir=$(CDPATH= cd -- "$script_dir/../.." && pwd)
source_dir="$repo_dir/skills"

usage() {
  cat <<'EOF'
Usage: scripts/install/install-skills.sh [TARGET] [--global | --project [DIR]]
                                         [--uninstall] [--dry-run] [--force]

  TARGET           zed | claude | all   (default: all)
  --global         install into the target's user-scope skills root (the default)
  --project [DIR]  install into DIR's project-scope skills root (DIR defaults to the
                   current directory)
  --uninstall      remove the SOP skill entries from the scope
  --dry-run        print what would happen, change nothing
  --force          replace a SOP-named symlink that points somewhere else

Skill roots:
  zed     global  ~/.agents/skills      project  <dir>/.agents/skills
  claude  global  ~/.claude/skills      project  <dir>/.claude/skills

Examples:
  scripts/install/install-skills.sh zed
  scripts/install/install-skills.sh claude --project ~/work/api
  scripts/install/install-skills.sh all --dry-run
EOF
}

target=all
scope=global
project_dir=""
uninstall=0
dry_run=0
force=0

while [ $# -gt 0 ]; do
  case "$1" in
    zed | claude | all)
      target=$1
      shift
      ;;
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
      echo "install-skills: unknown argument: $1" >&2
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
    echo "install-skills: project directory does not exist: $project_dir" >&2
    exit 1
  fi
  project_dir=$(CDPATH= cd -- "$project_dir" && pwd)
fi

if [ "$(id -u)" = 0 ]; then
  echo "install-skills: warning: skills are per-user; root is not required" >&2
fi

# Report the fail-closed precondition: the skills drive the `sop` CLI, so it must be
# installed before they can do anything.
if command -v sop >/dev/null 2>&1; then
  :
else
  echo "install-skills: warning: 'sop' is not on PATH; the skills will fail closed" >&2
  echo "install-skills:          until it is installed (see the project README / 'make install')" >&2
fi

if [ ! -d "$source_dir" ]; then
  echo "install-skills: source skills directory is missing: $source_dir" >&2
  exit 1
fi

case "$target" in
  all) targets="zed claude" ;;
  *) targets=$target ;;
esac

# agent_dir is the per-agent directory under HOME (or the project) that holds skills.
agent_dir() {
  case "$1" in
    zed) echo ".agents" ;;
    claude) echo ".claude" ;;
  esac
}

# skills_root is the skills root for a target in the selected scope.
skills_root() {
  sub=$(agent_dir "$1")
  if [ "$scope" = project ]; then
    echo "$project_dir/$sub/skills"
  else
    echo "$HOME/$sub/skills"
  fi
}

# run performs an action, or prints it under --dry-run.
run() {
  if [ "$dry_run" -eq 1 ]; then
    echo "would: $*"
  else
    "$@"
  fi
}

# install_target links (or removes) the SOP skills for one target and one root.
install_target() {
  root=$1

  if [ "$uninstall" -eq 0 ]; then
    run mkdir -p "$root"
  fi

  t_changed=0
  t_skipped=0
  for skill in $skills; do
    src="$source_dir/$skill"
    dest="$root/$skill"

    if [ "$uninstall" -eq 1 ]; then
      if [ -L "$dest" ]; then
        link_target=$(readlink "$dest" || true)
        case "$link_target" in
          "$source_dir"/*)
            run rm -f "$dest"
            echo "  removed $dest"
            t_changed=$((t_changed + 1))
            ;;
          *)
            echo "  skip: $dest is a symlink to $link_target (not SOP's); leaving it" >&2
            t_skipped=$((t_skipped + 1))
            ;;
        esac
      elif [ -e "$dest" ]; then
        echo "  skip: $dest exists and is not a SOP symlink; leaving it" >&2
        t_skipped=$((t_skipped + 1))
      fi
      continue
    fi

    if [ ! -f "$src/SKILL.md" ]; then
      echo "install-skills: missing source skill: $src/SKILL.md" >&2
      exit 1
    fi

    if [ -L "$dest" ]; then
      link_target=$(readlink "$dest" || true)
      if [ "$link_target" = "$src" ]; then
        echo "  ok: $dest"
        continue
      fi
      if [ "$force" -eq 1 ]; then
        run rm -f "$dest"
      else
        echo "  skip: $dest is a symlink to $link_target; re-run with --force to replace it" >&2
        t_skipped=$((t_skipped + 1))
        continue
      fi
    elif [ -e "$dest" ]; then
      echo "  skip: $dest exists and is not a symlink; remove it yourself, then re-run" >&2
      t_skipped=$((t_skipped + 1))
      continue
    fi

    run ln -s "$src" "$dest"
    echo "  linked $dest -> $src"
    t_changed=$((t_changed + 1))
  done

  changed=$((changed + t_changed))
  skipped=$((skipped + t_skipped))
}

changed=0
skipped=0
processed=0

for t in $targets; do
  # In the user scope, `all` installs only the agents actually present, so it never
  # creates configuration for an agent this machine does not use.
  if [ "$target" = all ] && [ "$scope" = global ]; then
    marker="$HOME/$(agent_dir "$t")"
    if [ ! -d "$marker" ]; then
      echo "skip: $t (no $marker); install that agent, or run: install-skills.sh $t" >&2
      continue
    fi
  fi

  root=$(skills_root "$t")
  processed=$((processed + 1))

  echo "SOP skills -> $t"
  echo "  scope: $scope"
  echo "  root:  $root"
  install_target "$root"
  echo
done

if [ "$processed" -eq 0 ]; then
  echo "install-skills: no supported agent environment found (looked for: $targets)" >&2
  echo "install-skills: name one explicitly, e.g. install-skills.sh zed" >&2
  exit 1
fi

if [ "$uninstall" -eq 1 ]; then
  echo "done ($changed removed, $skipped skipped)."
else
  echo "done ($changed changed, $skipped skipped)."
  echo "  in your agent, type \"/\" in the message editor to find the SOP commands."
fi
