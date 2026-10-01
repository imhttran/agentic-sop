#!/usr/bin/env sh
# Install the sop CLI.
#
# Thin wrapper: the root install.sh owns CLI installation (and delegates the agent
# skills to scripts/install-skills.sh). This file is kept so an existing workflow that
# calls scripts/install.sh keeps working; use ./install.sh directly for the documented
# interface, including --skills/--plugin.
set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
exec "$script_dir/../install.sh" "$@"
