# PRD — Phase 4: Provider Runtime Abstraction & Capability Discovery

**Status:** Implemented (Phase 4 shipped; see [../plans/PLAN-Phase-4-Provider-Runtime.md](../plans/PLAN-Phase-4-Provider-Runtime.md))

## Problem

Phase 2.5 introduced model classes and Phase 3.5 made SOP select a class per task
deterministically. But SOP could not *inspect* the runtime that would serve the
selected model: it had no notion of provider identity, health, model discovery, or
capabilities, and it had no way to check that a selected model was actually
available before spending agent work on a task. Provider-specific knowledge
(Ollama, llama.cpp, the command provider) lived inside the agent layer, coupled to
execution.

There was also no support for a second local runtime (MLX/oMLX on Apple Silicon).

## Vision

Introduce a clean **provider/runtime** abstraction *underneath* the existing
model-class routing:

```text
Task → JEV evidence → deterministic SOP router → small | medium | large
     → model.Resolve() → provider/runtime abstraction
     → Ollama | llama.cpp | MLX | command → capabilities / availability
     → agent execution
```

Providers report **capabilities and availability**. Providers do **not** choose the
model class or control SOP lifecycle state.

## Goals

1. Make provider/runtime a first-class, reusable abstraction.
2. Keep Phase 3.5 routing authoritative for `small`/`medium`/`large`.
3. Let SOP inspect provider health, model availability, and typed capabilities.
4. Let the selected model be validated before execution, explicitly and opt-in.
5. Add an MLX/oMLX (Apple Silicon) provider integration.
6. Preserve existing behavior for every existing installation.
7. Keep all provider tests offline and deterministic.

## Functional Requirements

- **FR-1** A validated, closed provider identity (`ollama`, `llamacpp`, `mlx`,
  `command`); unknown names fail clearly.
- **FR-2** An explicit, deterministic provider registry (no package-level mutable
  state); duplicate registration and unknown lookups fail.
- **FR-3** A narrow, read-only provider interface: id, health, models,
  capabilities.
- **FR-4** Typed health (`healthy`/`unavailable`/`degraded`/`unknown`) that is
  informational and never reroutes.
- **FR-5** Typed, tri-state capabilities (`yes`/`no`/`unknown`); nothing
  fabricated; unknown never treated as absent.
- **FR-6** Model discovery that reports `ErrDiscoveryUnsupported` rather than an
  empty list when a provider cannot enumerate models.
- **FR-7** A read-only `ValidateSelection` check: provider exists/registered,
  reachable (when determinable), model present (when discovery is authoritative),
  can chat (when determined).
- **FR-8** An opt-in pre-execution validation hook (`providers.validate: true`),
  OFF by default.
- **FR-9** A read-only `sop providers [--models]` inspection command.
- **FR-10** Provider adapters for Ollama, llama.cpp, MLX/oMLX, and the command
  provider.
- **FR-11** Configuration of provider endpoints (`providers:`), environment-first,
  reusing existing variables.
- **FR-12** No credentials configured, persisted, or printed.

## Non-Functional Requirements

- Deterministic: registry listing, health, discovery, and validation are stable.
- Offline: no test requires a live provider (local HTTP test servers or fakes).
- Additive: with the layer off or unconfigured, existing behavior is unchanged.
- Layered: `internal/provider` does not import the agent/harness packages.

## Constraints

- Do not redesign the Phase 3.5 router; do not move class selection into provider
  code.
- Do not implement automatic provider fallback, cost/latency routing, or learned
  routing (see Out of Scope).
- Preserve the ownership model: **JEV analyzes; SOP decides; IMPLEMENT/FIX changes
  code.** Provider health/capability is evidence only.
- `sop-controller` MUST NOT invoke the provider layer to create competing
  lifecycle state.

## Success Criteria

- `sop providers` and `sop providers --models` report provider health and models.
- With `providers.validate: true`, a run fails clearly (and never substitutes)
  when the selected provider is unreachable or the selected model is absent.
- With the flag off, existing runs are unchanged.
- `go build ./...`, `go vet ./...`, `go test ./...`, and `go test -race` on the
  affected packages pass.

## Out of Scope

```text
automatic provider fallback        dynamic cheapest-provider selection
latency- or cost-based routing     benchmark-based model selection
learned routing                    automatic class escalation
distributed workers                remote SOP Hub / MCP orchestration
provider load balancing / scoring
```

## Related

- [../specs/PROVIDERS.md](../specs/PROVIDERS.md) — the normative specification.
- [../plans/PLAN-Phase-4-Provider-Runtime.md](../plans/PLAN-Phase-4-Provider-Runtime.md) — the implementation plan.
- [../specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) — Phase 3.5 routing (unchanged).
