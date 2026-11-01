# Determinism and Race Hardening

## ID

PREJEV012-S11

## Objective

Verify invocation isolation, deterministic lifecycle behavior, and race-sensitive shared state across the regression harness. Scope is limited to `internal/e2e/harness`, `internal/e2e/lifecycle`, and `internal/ollamaagent`. Do not discover unrelated subsystems.

This is a verification task: invocation-scoped state and the deterministic fakes are already implemented from PREJEV008. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestFakeProviderIsDeterministic`, `TestClockIsExplicit`, `TestIDsAreStable`, `TestImplementInvocationIsolation`, `TestImplementIsolationAcrossProviders`, and `TestMutationEvidenceIsInvocationScoped`, plus the whole suite run under `-race`.
- Run the focused validation and confirm it passes: `go test -race ./internal/e2e/...`.
- Run the broader validation: `go test -race ./internal/ollamaagent/...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more. The concurrency case (G5) is covered by `TestFakeProviderConcurrentUseIsRaceFree`; confirm it passes under `-race`.

## Acceptance Criteria

- canned responses are returned in order, and a call past the end yields a stable fallback rather than a panic.
- the deterministic clock and id generator advance only when asked.
- one invocation cannot observe another's recorded requests or mutation state, and invocations over the same repository see the same input state.
- concurrency-sensitive shared state is exercised under `go test -race` with no data races reported.
- the focused and broader `-race` validation commands pass.

## Constraints

- Do not commit, push, or merge; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- No sleeps, network, or live Ollama: determinism comes from explicit clocks and fakes, not timing.

## Dependencies

- PREJEV012-S6
- PREJEV012-S7
- PREJEV012-S8
- PREJEV012-S9
- PREJEV012-S10

## Execution

- implement
