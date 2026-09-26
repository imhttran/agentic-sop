# T026 --- Configured Agent Selection

> Follow-through on plan `PLAN-wrapup.md` T002/T008 (agent provider layer).

## Status

DONE

## Objective

Make the project configuration actually drive runtime behavior instead of being
an unused file: the configured `agent.provider` selects the agent, and the
environment overrides it.

```text
SOP_AGENT_PROVIDER (env)  ─┐
                           ├─> agent.FromConfig(provider) ─> Agent
.agent-sdlc/config.yaml  ──┘
```

## Dependencies

- T024 (local model providers)
- T025 (configuration model)

## Scope

- `internal/agent/provider.go`: add `FromConfig(provider)`; `FromEnv` delegates
  to it with a blank provider.
- `internal/cli/cli.go`: `deps.newAgent` is parameterised by provider;
  `configuredProvider` loads the project configuration.
- `internal/cli/plan.go`: pass the configured provider to the agent constructor.
- Tests in `internal/agent/provider_test.go` and `internal/cli/cli_test.go`.

## Rules

- The environment wins over configuration, so operators can override a committed
  default without editing it.
- A missing configuration file means "let the environment decide" and keeps the
  historical default (command agent); it is not an error.
- A present but invalid configuration is a hard error, never a silent fallback.
- No new behavior for commands that do not use an agent.

## Tests

`FromConfig` selects the configured provider, the environment overrides it, an
unknown configured provider errors, and a blank provider falls back to the
command agent; the CLI `plan` command passes the configured provider to the
agent constructor and fails clearly when the configuration is invalid.

## Acceptance Criteria

- [x] The configured provider selects the agent; the environment overrides it.
- [x] A missing configuration keeps the environment/default behavior.
- [x] An invalid configuration fails the command with a clear message.
- [x] `make check` passes.

## Git

Branch: `task/T026-configured-agent-selection`
Commit: `task(T026): select the agent from configuration`
PR: `[Task T026] Select the agent from configuration`

## Out of Scope

Using the remaining configuration fields (validation commands, quality policy,
human gate); the `run`/`commit`/`pr` commands.
