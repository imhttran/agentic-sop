# T038 --- Jev Decision Layer

> Implements plan `PLAN-JEV.md` T033–T036 (decision provider interface, adapter,
> model routing, review escalation) and PRD §15.

## Status

DONE

## Objective

A bounded decision boundary where a provider proposes and policy disposes:

```text
Request ──> Provider.Decide ──> Decision{Choice, Confidence, Metadata}
                                        │
                                        v
                    Route(thresholds, decision) ──> SMALL_MODEL | STRONG_MODEL | HUMAN
```

Policy (thresholds) lives in configuration, never in the model. The default
provider is deterministic, so the Jev experiment is opt-in and comparable against
simple rules before it is trusted (plan T037).

## Dependencies

- T025/T026 (configuration), Stage 9 (agent boundary)

## Scope

- `internal/decision/decision.go`: `Choice`, `Decision`, `Request`, `Provider`,
  `NewProvider`, `DeterministicProvider`, `Thresholds`, `Target`, `Route`.
- `internal/decision/decision_test.go`: provider/routing tests.
- `internal/config/config.go`: `decision` (provider, enabled, thresholds) and
  `features` (`jev_decisions`) sections, defaults, and validation.
- `internal/config/config_test.go`: defaults and error cases.

## Rules

- A provider only produces a decision; it never executes actions (plan T034).
- Policy is outside the provider: `Route` maps a decision to a target using
  configured thresholds, and is shared by model routing (T035) and review
  escalation (T036) because the policy shape is the same.
- The layer is **disabled by default** (`decision.enabled: false`), and the
  default provider is deterministic.
- The `jev` provider is recognized by configuration but not implemented, so
  `NewProvider("jev")` fails clearly rather than silently falling back. Enabling
  Jev by default requires demonstrated value (plan T037), so it stays opt-in.
- Thresholds must be within [0,1].

## Tests

The deterministic provider classifies small/moderate/risky/large work; `NewProvider`
returns the deterministic provider for ""/"deterministic" and errors for "jev";
`Route` sends HIGH/low-confidence to a human, medium to the strong model, and a
confident low to the small model. Configuration applies decision defaults and
rejects an unknown provider or an out-of-range threshold.

## Acceptance Criteria

- [x] A `DecisionProvider` interface produces a choice with confidence and metadata.
- [x] The default provider is deterministic and explainable.
- [x] Routing/escalation policy is configuration-driven and outside the provider.
- [x] The layer is opt-in; an unimplemented provider fails clearly.
- [x] `make check` passes.

## Git

Branch: `task/T038-decision-layer`
Commit: `task(T038): add Jev decision layer`
PR: `[Task T038] Add Jev decision layer`

## Out of Scope

Wiring routing into the run lifecycle; a concrete Jev adapter (no protocol is
specified); the Jev-vs-deterministic evaluation (plan T037), which requires real
harness runs.
