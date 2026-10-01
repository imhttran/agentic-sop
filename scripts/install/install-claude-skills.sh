#!/usr/bin/env sh
# Thin wrapper: install the SOP skills for Claude Code.
#
# Claude Code loads skills from ~/.claude/skills (personal) or <project>/.claude/skills
# (project) and exposes each as a slash command named after its folder (or its
# frontmatter `name`). This forwards to the shared installer, which owns the
# implementation and the skill list; see scripts/install/install-skills.sh for the options.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$script_dir/install-skills.sh" claude "$@"
