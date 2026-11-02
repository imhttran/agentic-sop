# T035 --- Provider Capability Detection

> Implements plan `PLAN-JEV.md` T032 and PRD §9 (provider architecture).

## Status

DONE

## Objective

Represent which capabilities a provider serves and reject an unsupported
role/provider combination clearly, instead of sending it to a provider that
cannot serve it:

```text
workflow asks for capability C
        │
        v
agent.Checked(provider)  ── C supported? ── no ──> clear error naming the set
        │
        yes
        v
     provider.Generate(C)
```

## Dependencies

- Stage 9 (Agent Harness Boundary, T009), T024 (local model providers)

## Scope

- `internal/agent/capabilities.go`: `Capabilities`, `AllCapabilities`,
  `NewCapabilities`, `Supports`, `List`, `Declarer`, `CapabilitiesOf`, `Checked`,
  `NewChecked`.
- `internal/agent/command.go`, `ollama.go`, `openai.go`: each provider declares
  its capabilities.
- `internal/cli/cli.go`: the composition root wraps the resolved provider in
  `agent.NewChecked` (default deps only; tests inject their own agents).
- `internal/agent/capabilities_test.go`: model and guard tests.

## Rules

- The declared vocabulary is the existing `agent.Capability` set (PLAN,
  DESIGN_TESTS, IMPLEMENT, DIAGNOSE_FAILURE, FIX, REVIEW). The PRD's
  PLAN/CODE/REVIEW map onto these; DECISION belongs to the (future) Jev layer and
  TOOLS/STRUCTURED_OUTPUT are provider traits, not capabilities the workflow
  requests today.
- An agent that declares nothing is assumed to serve every capability, so the
  guard never changes behavior for a general-purpose provider.
- The guard validates the request first, then the capability; an unsupported one
  is an error that names both the capability and the supported set.
- Enforcement lives at the composition root, so the provider selection code and
  the request path stay unchanged.

## Tests

`Supports`/`List` (canonical order) and `AllCapabilities`; `CapabilitiesOf`
returns a declared subset and defaults to all for an undeclared agent; `Checked`
rejects an unsupported capability with a message naming it and the supported
set, delegates a supported one, reports its capabilities, and validates before
routing.

## Acceptance Criteria

- [x] A provider's served capabilities are represented and queryable.
- [x] An unsupported capability is rejected with a clear, explicit error.
- [x] General-purpose providers (declaring all) are unaffected.
- [x] The guard is applied at the composition root.
- [x] `make check` passes.

## Git

Branch: `task/T035-provider-capability-detection`
Commit: `task(T035): add provider capability detection`
PR: `[Task T035] Add provider capability detection`

## Out of Scope

Multi-provider routing/selection across providers; the Jev decision layer;
provider trait flags (tools, structured output).
