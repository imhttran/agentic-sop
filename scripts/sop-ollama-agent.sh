#!/bin/sh
# Bootstrap coding-agent harness for SOP's command provider.
#
# SOP invokes this as:
#   export SOP_AGENT_PROVIDER=command
#   export SOP_AGENT_COMMAND="sh scripts/sop-ollama-agent.sh"
#   sop run docs/plans/PLAN-Agent-Harness-V2.md
#
# It reads the JSON agent request on stdin and writes the response on stdout,
# exactly like any other command agent. It runs the installed known-good
# sop-ollama-agent binary from outside the working tree under edit; it never
# compiles candidate source, so a compile error in the candidate agent cannot
# take away the agent needed to repair it:
#
#   install known-good sop-ollama-agent
#     -> SOP invokes the installed binary (this script)
#     -> the agent edits candidate source
#     -> SOP validates candidate source
#
# Install/update the binary with scripts/install-sop-ollama-agent.sh.
#
# Environment (shared with SOP's Ollama provider):
#   SOP_OLLAMA_BASE_URL   default http://127.0.0.1:11434
#   SOP_OLLAMA_MODEL      default deepseek-v4.1-flash:cloud
#   SOP_OLLAMA_TIMEOUT    default 2m
#
# Installed-binary resolution (override, then configured default), identical to
# internal/agentbin. A candidate is accepted only when it is a regular,
# executable file:
#   SOP_OLLAMA_AGENT_BIN   explicit binary to invoke (wins)
#   SOP_OLLAMA_AGENT_HOME  directory holding sop-ollama-agent
#   default                $HOME/.local/share/sop/bin/sop-ollama-agent
#
# It never invokes Claude or any other agent, and has no automatic fallback.
set -u

: "${SOP_OLLAMA_MODEL:=deepseek-v4.1-flash:cloud}"
export SOP_OLLAMA_MODEL

binary_name=sop-ollama-agent

# candidates mirrors internal/agentbin.Candidates: the override is tried first;
# setting SOP_OLLAMA_AGENT_HOME replaces (does not precede) the default directory,
# exactly as the Go-side resolver does.
candidates() {
    if [ -n "${SOP_OLLAMA_AGENT_BIN:-}" ]; then
        printf '%s\n' "$SOP_OLLAMA_AGENT_BIN"
    fi
    if [ -n "${SOP_OLLAMA_AGENT_HOME:-}" ]; then
        printf '%s\n' "$SOP_OLLAMA_AGENT_HOME/$binary_name"
        return 0
    fi
    if [ -n "${HOME:-}" ]; then
        printf '%s\n' "$HOME/.local/share/sop/bin/$binary_name"
        return 0
    fi
    return 1
}

bin=""
tried=""
while IFS= read -r candidate; do
    [ -n "$candidate" ] || continue
    tried="$tried  $candidate
"
    if [ -f "$candidate" ] && [ -x "$candidate" ]; then
        bin=$candidate
        break
    fi
done <<EOF
$(candidates)
EOF

if [ -z "$bin" ]; then
    echo "$binary_name: no installed known-good binary found; tried:" >&2
    printf '%s' "$tried" >&2
    echo "$binary_name: install one with scripts/install-sop-ollama-agent.sh (this bootstrap never compiles candidate source)" >&2
    exit 1
fi

exec "$bin" "$@"
