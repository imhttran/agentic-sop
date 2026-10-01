# T074 --- A Claude Code Command-Agent Adapter

> Wire a real coding agent behind the existing `command` provider, so SOP can drive
> work the local models cannot do.

## Status

DONE

## Objective

The local Ollama models (flash and pro) could not implement the larger plan tasks
(AHV2011/AHV2012): they explore for most of the budget and only start writing once
the tools are withdrawn, or narrate instead of acting. The plan already names the
intended escape hatch — `SOP_AGENT_COMMAND="sh scripts/agents/sop-agent.sh"` → "Claude
Code / other external implementation agent" (AHV2010) — but the script did not
exist. This adds it.

## Scope

- `scripts/agents/sop-agent.sh`: reads SOP's JSON request on stdin and writes the response
  on stdout, delegating to Claude Code (`claude -p --output-format json`).
- `docs/PLAN.md`.

## Rules

- **An adapter, not an engine.** It parses the request, builds a prompt, runs the
  agent non-interactively in the repository, and emits the response SOP already
  expects. SOP keeps every workflow decision.
- **A narrow surface.** Claude runs with a read/edit plus Go-check allow-list
  (`Read,Edit,Write,Glob,Grep,LS`, `go build/test/vet`, `gofmt`, `git status/diff`);
  `git commit/push/merge/reset/clean` are not allowed, and the prompt forbids
  touching `.agent-sdlc`. The provider is generic, so the Ollama harness still works
  unchanged.
- **The existing schema.** PLAN and REVIEW emit their JSON documents; mutating
  capabilities emit the structured outcome, falling back to the agent's prose (SOP
  uses the repository diff) when no outcome line is produced.
- **No new SOP protocol.** The `SOP_JSON:` sentinel is inside the adapter only.

## Tests

Verified by hand against a scratch repository: an IMPLEMENT request edited the file
and returned `{"status":"completed",…,"changes_expected":true}`; a REVIEW request
returned the review JSON.

## Acceptance Criteria

- [x] `sh scripts/agents/sop-agent.sh` satisfies the command-provider contract.
- [x] IMPLEMENT edits the repository and returns the structured outcome.
- [x] REVIEW returns the review document.
- [x] The agent cannot commit, push, or modify `.agent-sdlc`.

## Git

Branch: `main`
Commit: `task(T074): add a Claude Code command-agent adapter`

## Out of Scope

Replacing the Ollama bootstrap harness; changing SOP's command-agent protocol;
running the agent with broad permissions.
