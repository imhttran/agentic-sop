# Provider and Model Selection

## ID

PREJEV012-S8

## Objective

Verify provider and model selection: configuration, defaults, environment overrides, precedence, and Ollama selection, plus clear failure for an invalid selection. Scope is limited to `internal/agent` (`provider.go`) and `internal/config`. Do not discover unrelated subsystems.

This is a verification task: the behavior is already implemented from AHV2001 through AHV2008. Existing correct behavior must be validated, not rewritten.

## Requirements

- Follow the three-step loop: verify existing coverage first, implement only the missing coverage, then validate independently.
- Inspect existing coverage before adding anything; the behavior is already covered by `TestFromEnvSelection`, `TestFromConfigProvider`, `TestEffectiveProvider`, `TestEffectiveModel`, `TestEffectiveHarness`, `TestFromConfigModel`, `TestNewOllamaValidation`, `TestNewLlamaCppValidation`, `TestNewOllamaFromEnvRequiresModel`, `TestNewOllamaFromEnvUsesConfiguredModel`, `TestConfigDefaultsToDeepSeek`, and `TestConfigModelEnvOverride`.
- Run the focused validation and confirm it passes: `go test ./internal/agent/ -run 'Provider|Model|FromEnv|FromConfig|Effective|Ollama|LlamaCpp'`.
- Run the broader validation: `go test ./internal/agent/... ./internal/config/...`.
- If every acceptance criterion below is already covered, record the covering tests and finish with no change (`changes_expected=false`); do not manufacture a change.
- Only if a real gap is demonstrated, add the single focused test and nothing more; otherwise record the gap for PREJEV012-S12.

## Acceptance Criteria

- the effective provider resolves from environment, then configuration, then built-in default — in that precedence — and the source is reported.
- an unknown configured provider fails clearly instead of silently falling back.
- the Ollama provider requires a model, and the configured model is used unless the environment overrides it, with the environment winning.
- the unified model environment variable takes precedence over the provider-specific one.
- an invalid selection produces a focused failure.
- the focused and broader validation commands pass.

## Constraints

- Do not commit, push, or merge; never edit `.agent-sdlc/state.db`.
- Do not duplicate SOP orchestration; this task defines scope, acceptance, and validation only.
- Do not alter correct implementation behavior to force a mutation.
- Standard tests must require no live Ollama instance.

## Dependencies

- none

## Execution

- implement
