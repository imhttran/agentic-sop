#!/usr/bin/env sh
# Thin wrapper: install the SOP skills for Zed.
#
# Zed loads skills from ~/.agents/skills (global) or <project>/.agents/skills
# (project-local) and exposes each as a slash command named after its folder. This
# forwards to the shared installer, which owns the implementation and the skill list;
# see scripts/install-skills.sh for the options.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$script_dir/install-skills.sh" zed "$@"
