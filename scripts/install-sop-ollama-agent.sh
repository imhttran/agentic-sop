#!/usr/bin/env sh
# Install/update the known-good sop-ollama-agent binary outside the tree under
# edit.
#
# SOP's command bootstrap (scripts/sop-ollama-agent.sh) invokes this installed
# binary instead of compiling candidate working-tree source, so a compile error
# in the candidate agent cannot remove the agent needed to repair it:
#
#   install known-good sop-ollama-agent   (this script)
#     -> SOP invokes the installed binary
#     -> the agent edits candidate source
#     -> SOP validates candidate source
#
# The binary is built from a pinned source revision recorded in
# scripts/sop-ollama-agent.pin, so the same revision yields the same binary. It is
# installed atomically and re-running is idempotent: when the installed binary
# already reports the pinned revision, nothing is rebuilt.
#
# Install layout (directory mode): the built binary is written once, under a
# revision-addressed, immutable name (sop-ollama-agent-<revision>), and the
# stable sop-ollama-agent name is published as a symlink that is replaced
# atomically. A revision-addressed file is never overwritten in place, so a
# long-lived SOP/agent process that is already executing an older revision keeps
# running that exact inode even while a newer revision is installed; only a newly
# started invocation sees the newer revision. This removes the shared-state
# hazard of replacing a running executable's path in place.
#
# Environment:
#   SOP_OLLAMA_AGENT_HOME  install directory (default $HOME/.local/share/sop/bin)
#   SOP_OLLAMA_AGENT_BIN   install to this exact path instead of a directory
#   SOP_OLLAMA_AGENT_PIN   pin file overrides scripts/sop-ollama-agent.pin
#
# Exits non-zero with an actionable message if the build or install cannot
# complete. Requires Go and Git.
set -eu

repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
pin_file=${SOP_OLLAMA_AGENT_PIN:-"$repo_dir/scripts/sop-ollama-agent.pin"}
binary_name=sop-ollama-agent

fail() {
    echo "install-sop-ollama-agent: $*" >&2
    exit 1
}

# The installer reports the stage it is in, so a run reads as progress rather
# than a silent build (and does not need `sh -x` to be followable).
stage() {
    echo "$*"
}

# --- read the pinned source revision ---
[ -f "$pin_file" ] || fail "missing pin file $pin_file"
revision=$(sed -n 's/^revision[[:space:]]*=[[:space:]]*//p' "$pin_file" | head -n 1 | tr -d '[:space:]')
[ -n "$revision" ] || fail "no 'revision = <rev>' line in $pin_file"

command -v go >/dev/null 2>&1 || fail "Go is required to build $binary_name but was not found on PATH"
command -v git >/dev/null 2>&1 || fail "Git is required to resolve revision $revision but was not found on PATH"

# The pinned revision must exist as a commit AND contain the agent's main
# package; a pin that resolves to a commit without cmd/sop-ollama-agent would
# otherwise fail deep inside the build with a less actionable message.
( cd "$repo_dir" && git cat-file -e "${revision}^{commit}" 2>/dev/null ) \
    || fail "pinned revision $revision is not present in $repo_dir; fetch it or update $pin_file"
( cd "$repo_dir" && git cat-file -e "${revision}:cmd/sop-ollama-agent/main.go" 2>/dev/null ) \
    || fail "pinned revision $revision does not contain cmd/sop-ollama-agent; update $pin_file"

# --- resolve install target ---
# explicit_bin marks the operator-owned SOP_OLLAMA_AGENT_BIN override, which is a
# single exact path installed in place rather than the directory layout below.
explicit_bin=0
if [ -n "${SOP_OLLAMA_AGENT_BIN:-}" ]; then
    install_path=$SOP_OLLAMA_AGENT_BIN
    explicit_bin=1
else
    install_dir=${SOP_OLLAMA_AGENT_HOME:-"${HOME:-}/.local/share/sop/bin"}
    [ -n "$install_dir" ] || fail "HOME is not set; set SOP_OLLAMA_AGENT_HOME or SOP_OLLAMA_AGENT_BIN"
    install_path=$install_dir/$binary_name
fi

# --- idempotency: same pinned revision means the installed binary is current ---
# Read the built binary's self-reported source revision. stdin is redirected from
# /dev/null so a binary that predates the -version flag (and would instead read a
# request from stdin) returns immediately rather than blocking.
installed_revision() {
    [ -e "$1" ] || return 1
    "$1" -version </dev/null 2>/dev/null | sed -n 's/^Source revision: //p' | head -n 1 | tr -d '[:space:]'
}

if current=$(installed_revision "$install_path"); then
    if [ "$current" = "$revision" ]; then
        echo "$binary_name at $install_path is current (revision $revision); nothing to do"
        exit 0
    fi
fi

# --- build the pinned revision from the repository ---
install_dir=$(dirname -- "$install_path")
mkdir -p "$install_dir" || fail "cannot create install directory $install_dir"

if [ "$explicit_bin" -eq 1 ]; then
    # An operator-supplied exact path is installed in place (atomic rename).
    final_path=$install_path
else
    # Revision-addressed, immutable final path; the stable name is a symlink.
    final_path=$install_dir/$binary_name-$revision
fi

# Reuse an already-built, immutable revision-addressed binary; only build when
# it is absent (or when installing to an explicit in-place path).
if [ "$explicit_bin" -eq 0 ] && [ -x "$final_path" ] && [ "$(installed_revision "$final_path")" = "$revision" ]; then
    stage "Using existing build of $binary_name (revision $revision)"
else
    stage "Building $binary_name..."
    tmp=$(mktemp "$install_dir/.${binary_name}.XXXXXX") || fail "cannot create a temporary build path in $install_dir"
    scratch=$(mktemp -d) || { rm -f "$tmp"; fail "cannot create a scratch build directory"; }
    trap 'rm -rf "$scratch"; rm -f "$tmp"' EXIT

    # Build the pinned revision by extracting it to a scratch tree, so the installed
    # binary is not built from uncommitted candidate working-tree source.
    if ! ( cd "$repo_dir" && git archive "$revision" ) | tar -x -C "$scratch"; then
        fail "cannot extract pinned revision $revision from $repo_dir"
    fi

    ldflags="-X main.version=${revision} -X main.sourceRevision=${revision}"
    if ! ( cd "$scratch" && go build -trimpath -ldflags "$ldflags" -o "$tmp" ./cmd/sop-ollama-agent ); then
        fail "building $binary_name from pinned revision $revision failed"
    fi

    chmod 0755 "$tmp" || fail "cannot mark the built binary executable"
    # Atomic publish. For the directory layout this lands on a revision-addressed
    # name that is never overwritten while a process may be running it.
    mv -f "$tmp" "$final_path" || fail "cannot install the binary to $final_path"
    trap 'rm -rf "$scratch"' EXIT
fi

# Verify the binary we are about to publish. A binary that predates the -version
# flag reports no revision, which is not a failure; when it does report one it
# must be the pinned revision, so a mislabeled build is caught before it replaces
# the stable name.
stage "Running checks..."
if ! reported=$(installed_revision "$final_path"); then
    fail "the built $binary_name is missing at $final_path"
fi
if [ -n "$reported" ] && [ "$reported" != "$revision" ]; then
    fail "the built $binary_name reports revision $reported, want $revision"
fi

stage "Installing to $install_path..."

# Publish the stable name as a symlink to the immutable, revision-addressed
# binary, replacing the symlink atomically. A process already executing the old
# target keeps its inode; a newly started invocation sees the new revision.
if [ "$explicit_bin" -eq 0 ] && [ "$final_path" != "$install_path" ]; then
    link_tmp=$(mktemp "$install_dir/.${binary_name}.link.XXXXXX") || fail "cannot create a temporary link path in $install_dir"
    rm -f "$link_tmp"
    ln -s "$(basename -- "$final_path")" "$link_tmp" || { rm -f "$link_tmp"; fail "cannot create the symlink for $install_path"; }
    mv -f "$link_tmp" "$install_path" || { rm -f "$link_tmp"; fail "cannot publish $install_path"; }
fi

echo "Installed $binary_name $revision"
"$install_path" -version </dev/null 2>/dev/null | sed 's/^/  /' || true
echo
echo "done."
