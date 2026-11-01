# T015 --- GitHub Actions CI

## Status

DONE

## Objective

Give every pull request an independent verification environment by generating a
project CI workflow from the project's configured verification commands
(T008 `Commands`), so PR status is observable by the orchestrator (T014
`Checks`).

## Dependencies

- T008 --- configurable test runner (`Commands`)
- T014 --- GitHub adapter (observes PR status)

## Scope

Add `internal/ci`:

``` go
func Render(commands testrunner.Commands) (string, error)
```

which renders a deterministic GitHub Actions workflow (`push` + `pull_request`)
that checks out the repo and runs each configured command in order
(build, unit test, integration test, lint, docker build), as the project
defines them. No target-language commands are hard-coded.

## Rules

- Language-independent: commands come from project configuration.
- Deterministic output for a given configuration.
- A configuration with no commands is an error (nothing to verify).
- No network is required to render.

## Tests

Cover: workflow contains `on: push` and `pull_request`, `actions/checkout`, and
one step per configured command in order; deterministic (two renders equal);
unconfigured commands are omitted; all-unconfigured → error.

## Acceptance Criteria

- [x] CI workflow renders from the project's configured commands.
- [x] Each configured command becomes a step, in order.
- [x] Output is deterministic and language-independent.
- [x] An empty configuration is an error.
- [x] PR status is observable by the orchestrator via T014 `Checks`.
- [x] `make check` passes.

## Git

Branch: `task/T015-ci-workflow`
Commit: `task(T015): add ci workflow generation`
PR: `[Task T015] Add GitHub Actions CI workflow`

## Out of Scope

CI remediation (T016), merge gate (T017); language-specific setup steps.
