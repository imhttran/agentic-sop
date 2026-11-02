# T025 --- Configuration Model

> Implements plan `PLAN-wrapup.md` T002 (and extends T003's `init`).

## Status

DONE

## Objective

Give the harness a single, human-editable configuration surface describing
policy only:

```text
.agent-sdlc/config.yaml
  project      metadata + integration branch
  agent        provider selection (command | ollama | llamacpp)
  validation   build / test / lint commands
  review       engine + delegation
  quality      require_tests, max_fix_cycles, fail_on severities
  human        approval_before_commit
```

Configuration never holds mutable task state and never holds secrets.

## Dependencies

- Stage 3 (CLI, T003/T005A)
- T024 (local model providers) for the `agent.provider` values

## Scope

- `internal/config/config.go`: schema types, `Parse`/`Load`/`LoadDir`/`Path`,
  `Default`, `Validate`, `Template`, defaults, strict decoding.
- `internal/config/config_test.go`: table-driven tests.
- `internal/cli/init.go`: generate the template when absent, never overwrite.
- `internal/cli/cli.go`: source the state directory name from `config.DirName`.
- `internal/cli/cli_test.go`: init writes and preserves configuration.

## Rules

- Policy only: no task state, no secrets. The schema has no secret fields, and
  unknown keys are rejected, so credentials cannot be committed by accident.
- Strict decoding and explicit validation: malformed YAML, unknown keys, missing
  `project.name`, unknown provider/engine/severity, a negative fix limit, and an
  unsupported version all fail with a clear message.
- Defaults are applied for omitted settings; a field whose zero value is also
  valid (`require_tests`, `approval_before_commit`) uses a pointer so an omitted
  value defaults to the safe behavior while an explicit `false` is still
  expressible.
- A missing file is `ErrNotFound` ("use defaults"), not an error.
- `init` is idempotent and never overwrites a human-edited configuration.

## Tests

Valid configuration resolves every field; omitted fields take documented
defaults (version, integration branch, provider, engine, fix cycles, fail_on,
both booleans); explicit `false` is honored; error cases (empty file, malformed
YAML, unknown key, missing name, unknown version, unknown provider/engine,
unknown severity, negative fix cycles, multiple documents); the generated
template re-parses; `LoadDir`/`ErrNotFound`; CLI `init` writes the template and
preserves an existing file.

## Acceptance Criteria

- [x] A typed configuration model covers project, agent, validation, review, quality, and human policy.
- [x] Defaults are applied and documented by the generated template.
- [x] Malformed YAML, unknown keys, unknown values, missing required fields, and unsupported versions are errors.
- [x] Configuration carries no secrets; unknown keys are rejected.
- [x] `sop init` generates the template once and never overwrites it.
- [x] `make check` passes.

## Git

Branch: `task/T025-configuration-model`
Commit: `task(T025): add configuration model`
PR: `[Task T025] Add configuration model`

## Out of Scope

Loading configuration into runtime behavior (agent selection, validation
commands, quality policy) — the next task; environment-specific overrides.
