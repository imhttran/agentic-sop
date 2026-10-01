#!/usr/bin/env bash
# P35-007 link check: verify that every relative Markdown link in the documentation
# tree resolves to an existing file. Reports a broken-link count and exits non-zero
# when any link is broken.
#
# Usage: scripts/checks/check-doc-links.sh [root]
#   root defaults to the repository root (the script's grandparent directory).
#
# Coverage: docs/**/*.md (recursively) and README.md.
# Reported: "broken links: N" followed by one line per broken link.

set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# This script lives at scripts/checks/check-doc-links.sh, so the repository root is two
# levels up.
root="${1:-$(cd "$script_dir/../.." && pwd)}"

cd "$root"

# Collect the Markdown files covered by the check.
files=()
if [[ -f README.md ]]; then
  files+=("README.md")
fi
if [[ -d docs ]]; then
  while IFS= read -r f; do
    files+=("$f")
  done < <(find docs -name '*.md' -type f | sort)
fi

broken=0

for file in "${files[@]}"; do
  dir="$(dirname "$file")"
  # Extract the target of every inline Markdown link: ](target)
  while IFS= read -r link; do
    [[ -z "$link" ]] && continue
    # Strip an optional title: 'path "title"' -> 'path'
    target="${link%%[[:space:]].*}"
    # Skip external, absolute-URL, and pure-anchor links.
    case "$target" in
      http://*|https://*|mailto:*|tel:*|\#*) continue ;;
    esac
    # Drop a trailing #anchor on a file link.
    path="${target%%#*}"
    [[ -z "$path" ]] && continue
    resolved="$dir/$path"
    if [[ ! -e "$resolved" ]]; then
      echo "broken: $file -> $target"
      broken=$((broken + 1))
    fi
  done < <(grep -oE '\]\([^)]+\)' "$file" | sed -E 's/^\]\(//; s/\)$//')
done

echo "broken links: $broken"
if [[ "$broken" -ne 0 ]]; then
  exit 1
fi
