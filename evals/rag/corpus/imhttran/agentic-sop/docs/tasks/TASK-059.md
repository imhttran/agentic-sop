# T059 --- PLAN Discovery and Synthesis in the Ollama Agent

> The Ollama agent's PLAN capability now has a deterministic completion path:
> bounded read-only discovery, then a tool-free synthesis. A PLAN can no longer
> fail by exploring until it hits an iteration limit.

## Status

DONE

## Objective

PLAN runs could perform useful repository discovery but then exhaust the
iteration budget because the model kept requesting tools instead of returning
FINAL. The failure was indistinguishable from an implementation loop:

```text
Ollama agent PLAN did not complete after N iterations
```

Target:

```text
PLAN
  ↓
DISCOVERY — at most 8 turns, read-only tools
  ↓
SYNTHESIS — tools disabled, at most 2 model turns
  ↓
FINAL
```

Early FINAL during discovery still completes immediately. These are Ollama Agent
phases scoped to one invocation; no SOP lifecycle state is introduced.

## Scope

- `internal/ollamaagent/harness.go`: `Execute` dispatches PLAN to a new
  `executePlan`; the existing generic loop is renamed `executeLoop` and is
  unchanged in behavior. `executePlan` models DISCOVERY and SYNTHESIS, withdraws
  tools at the discovery limit, denies synthesis tool requests, and returns a
  phase-specific error on synthesis exhaustion.
- `internal/ollamaagent/policy.go`: PLAN's limits are the two-phase
  `planDiscoveryTurns` (8) and `planSynthesisTurns` (2), centralized with the
  other capability policy. Other capabilities' budgets are unchanged.
- `internal/ollamaagent/trace.go`: `TraceRecord` gains `phase` and `event`, and
  the trace renders PLAN's phases and the discovery-to-synthesis transition.
- `internal/ollamaagent/prompt.go`: the PLAN guidance describes bounded discovery
  and the coming synthesis.
- README and tests.

## Rules

- **Discovery is bounded and read-only.** At most eight discovery turns with the
  centralized PLAN read-only tool policy; a mutation attempt is denied and does
  not execute (it still consumes a discovery turn). Reaching the limit transitions
  to synthesis rather than failing.
- **Synthesis is tool-free.** Tools requested during synthesis are denied with a
  correction, counted against the two-turn synthesis allowance, and never
  executed; the repository is unchanged. Synthesis cannot return to discovery.
- **Early finalization is preserved.** A valid final response during discovery
  returns immediately without entering synthesis.
- **Diagnostics are phase-specific and safe.** Exhaustion reports
  `termination=synthesis_limit` with discovery and synthesis counts, never the
  generic iteration-limit message. The trace names the phase and transition and
  carries no prompts, file contents, or secrets.
- **Other capabilities are unchanged.** IMPLEMENT/FIX keep their mutation surface
  and no forced synthesis is added to them.
- **SOP remains the workflow authority.** This is an implementation-adapter change
  only; validation, review, gates, and human approval are untouched.

## Tests

Deterministic against a fake Ollama server (`internal/ollamaagent`): early final
completion; forced synthesis after the discovery limit; a synthesis tool request
is denied and not executed; successful synthesis after one denied request;
synthesis exhaustion with a phase-specific, non-generic error; mutation denial
during discovery; trace rendering of both phases, the transition, a final turn,
and the termination reason; and an AHV2006-shaped fixture that performs realistic
reads/searches then synthesizes. No live provider is required and no SOP state is
touched.

## Acceptance Criteria

- [x] PLAN distinguishes DISCOVERY from SYNTHESIS, scoped to one invocation.
- [x] Discovery permits at most eight read-only tool executions and can finish
      earlier.
- [x] Reaching the discovery limit enters SYNTHESIS instead of failing.
- [x] No repository tool executes during synthesis; synthesis is at most two turns.
- [x] Exhaustion returns `synthesis_limit`, not the generic iteration limit.
- [x] Early FINAL still completes immediately.
- [x] Other capability policies and IMPLEMENT/FIX behavior are unchanged.
- [x] The trace shows both phases, the transition, and the termination reason
      without logging secrets.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T059): PLAN discovery/synthesis and grounded IMPLEMENT outcomes`

## Out of Scope

Agent Harness V2; SOP lifecycle, validation, review, or gate changes; the plan
parser; the plan-synthesis source document; adding forced synthesis to
IMPLEMENT/FIX; raising a global iteration limit; renaming the Ollama Agent;
committing, pushing, or merging.
