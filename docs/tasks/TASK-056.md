# T056 --- Bootstrap DeepSeek Coding-Agent Harness

> A minimal, temporary command-agent harness that lets SOP drive a plan with a
> local Ollama model (`deepseek-v4.1-flash:cloud`) instead of a hosted coding
> agent.

## Status

DONE

## Objective

SOP's `command` provider takes any harness. To execute
`docs/plans/PLAN-Agent-Harness-V2.md` without Claude, this adds the smallest useful
harness: it reads an SOP agent request, drives a bounded tool loop against Ollama,
and returns the structured response SOP expects. It implements no Agent Harness V2
work — it only exists so DeepSeek can do that work.

```text
SOP → command provider → scripts/sop-deepseek-agent.sh
    → cmd/sop-deepseek-agent → Ollama → deepseek-v4.1-flash:cloud
    → controlled repository tools → structured outcome
```

## Scope

- `internal/deepseekagent`: the harness — request parsing, the bounded tool loop,
  the Ollama `/api/chat` client, the system prompt/tool protocol, the controlled
  tools, and the command/path policy.
- `cmd/sop-deepseek-agent`: the CLI entry point (thin; wire stdin/stdout).
- `scripts/sop-deepseek-agent.sh`: builds and runs the command, keeping the
  current directory as the repository.
- Tests and docs.

## Rules

- **Adapter, not authority.** The harness never touches `.agent-sdlc` state and
  never commits, pushes, merges, resets, or rebases. SOP owns state, dependencies,
  retries, validation, review, gates, and human approval.
- **Protocol unchanged.** It reuses `agent.Request` (capability/task/input/
  output_requirements) and returns exactly what each capability expects: an
  outcome object for `IMPLEMENT`/`FIX`/`DESIGN_TESTS`/`DIAGNOSE_FAILURE`, and the
  plan/review JSON for `PLAN`/`REVIEW`. No SOP protocol changes.
- **Repository boundary.** Every path is canonicalized (including symlinks) and
  must stay inside the repository root; `.agent-sdlc` is read-only to the model.
- **Command safety in code.** `run_command` tokenizes the string itself (no shell,
  no metacharacters/globs) and admits only commands the shared
  `commandpolicy` classifies `SAFE` (plus `gofmt`); location overrides
  (`-C`/`--git-dir`/`--work-tree`) and out-of-repo absolute paths are refused.
- **Bounded.** Fixed limits on iterations, tool calls, model/API timeout, command
  timeout, and captured output; an empty model turn is re-requested a bounded
  number of times.
- **No fallback.** Every failure (Ollama down, HTTP error, timeout, malformed
  response, policy rejection, limit reached) is actionable; the harness never
  falls back to another model.

## Tests

Deterministic, with a scripted `httptest` Ollama endpoint (no live server): request
parsing; model name in the request (and the `SOP_OLLAMA_MODEL` override); tool-call
round trip feeding results back; file read/write and `create_file` refusing to
overwrite; path-escape and symlink-escape rejection; `.agent-sdlc` write refusal;
safe-command execution and destructive/overflow rejection; iteration and tool-call
bounds; empty-turn retry; completed/`changes_expected`/`needs_human`/`failed`
outcomes; malformed model and tool responses; Ollama unavailable/HTTP error; and
PLAN/REVIEW passing through unchanged. Verified once end-to-end against the real
local Ollama model.

## Acceptance Criteria

- [x] `scripts/sop-deepseek-agent.sh` runs the harness as a command agent.
- [x] The model defaults to `deepseek-v4.1-flash:cloud`, overridable by
      `SOP_OLLAMA_MODEL`/`SOP_OLLAMA_BASE_URL`.
- [x] The tool loop is bounded and enforces the repository/state/command policy in
      code.
- [x] Output matches SOP's command-agent contract for every capability.
- [x] `make check` passes; no Claude dependency.

## Git

Branch: `task/T056-deepseek-bootstrap-harness`
Commit: `task(T056): bootstrap DeepSeek coding-agent harness`
PR: `[Task T056] Bootstrap DeepSeek coding-agent harness`

## Out of Scope

Agent Harness V2 (harness/provider/model separation, capability corrections, native
config); any change to SOP's workflow engine; committing `docs/plans/PLAN-Agent-Harness-V2.md`.
