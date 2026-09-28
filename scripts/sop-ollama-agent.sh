#!/bin/sh
# Bootstrap coding-agent harness for SOP's command provider.
#
# SOP invokes this as:
#   export SOP_AGENT_PROVIDER=command
#   export SOP_AGENT_COMMAND="sh scripts/sop-ollama-agent.sh"
#   sop run docs/PLAN-Agent-Harness-V2.md
#
# It reads the JSON agent request on stdin and writes the response on stdout,
# exactly like any other command agent. It builds and runs the Go harness from
# this repository, keeping the current directory as the repository the model may
# edit.
#
# Environment (shared with SOP's Ollama provider):
#   SOP_OLLAMA_BASE_URL   default http://127.0.0.1:11434
#   SOP_OLLAMA_MODEL      default deepseek-v4.1-flash:cloud
#   SOP_OLLAMA_TIMEOUT    default 2m
#
# It never invokes Claude or any other agent.
set -u

: "${SOP_OLLAMA_MODEL:=deepseek-v4.1-flash:cloud}"
export SOP_OLLAMA_MODEL

here=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
bin="${TMPDIR:-/tmp}/sop-ollama-agent.$$.bin"

if ! ( cd "$here" && go build -o "$bin" ./cmd/sop-ollama-agent ); then
    echo "sop-ollama-agent: build failed (is Go installed?)" >&2
    exit 1
fi

"$bin"
status=$?
rm -f "$bin"
exit "$status"
