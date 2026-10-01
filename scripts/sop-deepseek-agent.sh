#!/bin/sh
# Compatibility entry point for configurations using the former agent name.
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd) || exit 1
exec sh "$here/agents/sop-ollama-agent.sh" "$@"
