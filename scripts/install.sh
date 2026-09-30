#!/usr/bin/env sh
# Install the sop CLI.
#
#   - installs the CLI with `go install ./cmd/sop`
#
# The `sop-end-to-end` agent skill is provided as a global Zed skill at
# `~/.agents/skills/sop-end-to-end`, so it is not bundled or installed here.
#
# Requires Go.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

echo "installing the sop CLI..."
( cd "$repo_dir" && go install ./cmd/sop )

echo
echo "done."
echo "  next: make sure \$(go env GOPATH)/bin is on your PATH, then run 'sop version'"
