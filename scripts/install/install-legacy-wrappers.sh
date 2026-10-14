#!/usr/bin/env sh
# Install the OPTIONAL legacy wrappers sop-controller and sop-decision-adapter.
#
# The unified `sop` executable already provides `sop controller serve` and
# `sop decision ...`, so these wrappers are not required. Install them only for
# existing scripts or operator muscle memory that still invoke the legacy names.
# They are thin wrappers over the same shared implementations
# (internal/controller.Serve and internal/decisionadapter/cli.Run), so behavior is
# identical to the unified commands.
#
# Optional support assets (not installed here):
#   scripts/controller/       https proxy, keychain token helper, sop-agent launcher
#   integrations/controller/  Caddyfile for the HTTPS proxy
# Copy them next to your deployment when you use the controller's HTTPS convenience.
#
# Optional Julia capability: the Julia decision provider runs the repository-owned
# helper tools/julia/infer.py via JULIA_PYTHON (override with JULIA_INFERENCE_CMD).
# Model weights remain external. Without the helper and weights, Julia decisions
# report unavailable — an explicit optional capability, never a required install.
#
# Usage:
#   scripts/install/install-legacy-wrappers.sh [--dest DIR]
# Default DIR is the same bin directory the sop CLI installs into ($GOBIN, then
# $(go env GOPATH)/bin, then ~/.local/bin).

set -eu

dest=""
while [ $# -gt 0 ]; do
  case "$1" in
    --dest) shift; dest=${1:?"--dest requires a directory"} ;;
    --dest=*) dest=${1#--dest=} ;;
    -h|--help) sed -n '2,26p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "install-legacy-wrappers: unknown argument $1" >&2; exit 2 ;;
  esac
  shift
done

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
if [ ! -f "$repo_dir/go.mod" ] || [ ! -d "$repo_dir/cmd/sop-controller" ]; then
  echo "install-legacy-wrappers: run from the agentic-sop checkout" >&2
  exit 1
fi
# Confirm the shared implementations the wrappers delegate to are present.
for d in cmd/sop-controller cmd/sop-decision-adapter internal/controller internal/decisionadapter/cli; do
  if [ ! -d "$repo_dir/$d" ]; then
    echo "install-legacy-wrappers: missing $d; this is not the unified checkout" >&2
    exit 1
  fi
done

if [ -z "$dest" ]; then
  if [ -n "${GOBIN:-}" ]; then
    dest=$GOBIN
  elif [ -n "${GOPATH:-}" ]; then
    dest=$GOPATH/bin
  else
    dest=$(go env GOPATH)/bin
  fi
fi
mkdir -p "$dest"

for cmd in sop-controller sop-decision-adapter; do
  tmp="$dest/.$cmd.tmp.$$"
  ( cd "$repo_dir" && go build -o "$tmp" "./cmd/$cmd" )
  mv "$tmp" "$dest/$cmd"
  echo "installed $dest/$cmd"
done

echo "note: the unified 'sop controller serve' and 'sop decision' commands need no wrapper."
