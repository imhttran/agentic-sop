#!/usr/bin/env sh
# Install the sop CLI and the bundled sop-end-to-end agent skill.
#
#   - installs the CLI with `go install ./cmd/sop`
#   - links this repo's .agents/sop-end-to-end skill into ~/.agents/skills so it
#     is available to Zed in every project
#
# Set SOP_SKILLS_DIR to install the skill elsewhere. Requires Go and Git.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
skills_dir=${SOP_SKILLS_DIR:-"$HOME/.agents/skills"}
skill_name=sop-end-to-end

echo "installing the sop CLI..."
( cd "$repo_dir" && go install ./cmd/sop )

echo "installing the $skill_name skill into $skills_dir..."
mkdir -p "$skills_dir"

target="$skills_dir/$skill_name"
if [ -e "$target" ] && [ ! -L "$target" ]; then
  backup="$target.bak.$(date +%Y%m%d%H%M%S)"
  mv "$target" "$backup"
  echo "backed up existing skill to $backup"
fi
ln -sfn "$repo_dir/.agents/$skill_name" "$target"
echo "linked $target -> $repo_dir/.agents/$skill_name"

echo
echo "done."
echo "  next: make sure \$(go env GOPATH)/bin is on your PATH, then run 'sop version'"
