# T055 --- Configure the Model in Config (`agent.model`)

> `.agent-sdlc/config.yaml` can name the model (`agent.model`), so provider and
> model both live in configuration; the provider's environment variable still
> overrides it.

## Status

DONE

## Objective

Selecting `ollama` or `llamacpp` required the model to come from an environment
variable (`SOP_OLLAMA_MODEL`), so a project could not fully describe its agent in
configuration. `agent.model` now carries the model, and the provider's own variable
overrides it — mirroring how `SOP_AGENT_PROVIDER` overrides `agent.provider`.

## Scope

- `internal/config`: `Agent.Model` (the `model` key), plus a commented hint in the
  generated template.
- `internal/agent`: `FromConfig(provider, model)`; `NewOllamaFromEnv(model)` /
  `NewLlamaCppFromEnv(model)` use the configured model when the environment
  variable is unset; the missing-model error names both sources.
- `internal/cli`: `deps.newAgent(provider, model)`; `configuredAgent` returns both;
  the call sites pass `cfg.Agent.Model`.
- Tests and docs.

## Rules

- Precedence mirrors the provider: provider env var > `agent.model` > (ollama: a
  clear error; llamacpp: the built-in default).
- `agent.model` is ignored by the `command` provider.
- A model name is policy, not a secret, so it belongs in the committed
  configuration; endpoints and credentials stay in the environment.

## Tests

config: `agent.model` parses and the template still parses. agent:
`NewOllamaFromEnv` uses the configured model and lets the environment variable win;
a missing model errors. cli: `sop plan` passes the configured provider *and* model
to the agent builder. Verified with a real binary: a config-only provider+model run
succeeds, and having no model anywhere fails with a message naming both sources.

## Acceptance Criteria

- [x] `agent.model` is accepted and used for ollama/llamacpp.
- [x] The provider's environment variable overrides the configured model.
- [x] The missing-model error names both `agent.model` and the variable.
- [x] `make check` passes.

## Git

Branch: `task/T055-configure-model`
Commit: `task(T055): configure the model in config`
PR: `[Task T055] Configure the model in config`

## Out of Scope

Per-provider model maps (one provider is active at a time); moving endpoints or
credentials into configuration.
