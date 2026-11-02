# T024 --- Local Model Providers

> Implements plan `PLAN-wrapup.md` T030 (Ollama) and T031 (llama.cpp).

## Status

DONE

## Objective

Let the harness run against local inference without a hosted provider by adding
two adapters behind the existing `agent.Agent` boundary:

```text
agent.Agent
  ├─ CommandAgent   (existing subprocess contract)
  ├─ Ollama         (Ollama /api/chat)
  └─ LlamaCpp       (OpenAI-compatible /v1/chat/completions, e.g. llama-server)
```

The provider is chosen at the composition root; the workflow never depends on a
specific one.

## Dependencies

- Stage 9 (Agent Harness Boundary, T009)
- Stage 3 (CLI composition root, T003/T005A)

## Scope

- `internal/agent/provider.go`: provider selection (`FromEnv`), shared prompt
  rendering, shared chat request types, bounded JSON POST helper, duration
  parsing.
- `internal/agent/ollama.go`: Ollama chat adapter and env constructor.
- `internal/agent/openai.go`: OpenAI-compatible adapter (llama.cpp) and env
  constructor.
- `internal/agent/provider_test.go`: table-driven tests over `httptest` servers.
- `internal/cli/cli.go`: build the agent via `agent.FromEnv`.

## Rules

- No new dependencies; local providers use only the standard library.
- The provider-agnostic `Request`/`Response` shape is unchanged; providers must
  not invent provider-specific concepts outside themselves.
- Timeouts are bounded; `stream` is always off; response size is capped.
- Missing required settings fail with a message naming the variable to set, not
  a silent fallback or a silent empty result.
- `SOP_AGENT_PROVIDER` is optional: unset keeps the historical command agent so
  existing configurations keep working.
- No secrets in committed configuration; an API key (if any) comes from the
  environment.

## Tests

Table-driven tests covering: prompt rendering includes/omits sections; base URL
normalization; timeout parsing (default, valid, invalid, non-positive); `FromEnv`
selection (default command, Ollama, llama.cpp, unknown provider); constructor
validation; Ollama success (path, model, single user message, stream off) and
failure modes (HTTP error, 200-with-error body, empty output); llama.cpp success
(path, bearer auth, model) and failure modes (HTTP error, error body, no
choices, empty output); invalid request rejected before any network call;
context cancellation; env defaults and required settings.

## Acceptance Criteria

- [x] Ollama and OpenAI-compatible (llama.cpp) adapters implement `agent.Agent`.
- [x] Providers are selected by `SOP_AGENT_PROVIDER`; unset keeps the command agent.
- [x] Base URL, model, timeout, and API key are configurable via the environment.
- [x] Requests validate before any network call; empty output is an error.
- [x] Non-2xx responses, provider error bodies, and cancellation surface clearly.
- [x] No new dependencies; automated tests use `httptest`, never the network.
- [x] `make check` passes.

## Git

Branch: `task/T024-local-model-providers`
Commit: `task(T024): add local model providers`
PR: `[Task T024] Add local model providers`

## Out of Scope

Provider capability detection and routing (plan T032, next task); hosted
providers; streaming; tool calls; structured-output enforcement.
