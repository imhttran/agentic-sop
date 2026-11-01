# T030 --- Validation Runner

> Implements plan `PLAN-JEV.md` T012–T014 (validation model, configurable
> commands, validation gate) and PRD §8.6.

## Status

DONE

## Objective

Run a project's configured verification commands deterministically and
classify the result:

```text
.agent-sdlc/config.yaml validation: {build, test, lint}
        │
        v
   ordered checks (build → test → lint), fail-fast
        │
        v
   PASS | FAIL   (exit code)
```

No model decides whether a change compiles or passes.

## Dependencies

- T025 (configuration model) for the `validation` commands
- Stage 8 (Test Runner, T008) for command execution

## Scope

- `internal/validate/validate.go`: `Checks` (config → ordered `testrunner.Check`),
  `Run` (fail-fast suite), `Enabled`.
- `internal/validate/validate_test.go`: mapping and execution tests.
- `internal/cli/validate.go`: the `sop validate` command and output.
- `internal/cli/cli.go`: dispatch and help entry.
- `internal/cli/cli_test.go`: command-level tests.

## Rules

- Configuration commands are trusted project configuration and run through the
  existing test runner (like a Makefile); they are never built from untrusted
  input.
- Order is deterministic: build, then test, then lint, preserving each list's
  order and omitting blanks.
- Fail-fast: the first non-passing check stops the suite, so an uncompilable
  change is not carried forward to expensive review.
- A missing configuration is an error pointing at `sop init`; an empty
  `validation` section is not a failure.
- Exit code is non-zero when validation fails.

## Tests

`Checks` maps categories and order and omits blanks/empties; `Enabled`; `Run`
fail-fast stops after the first failure; a passing suite; diagnostics from a
failing command are surfaced. CLI: pass, fail (fail-fast, diagnostics, no lint
after a failure), missing configuration, no commands configured, and bad args.

## Acceptance Criteria

- [x] Validation commands come from configuration, not hard-coded.
- [x] Checks run in a deterministic order and fail fast.
- [x] `sop validate` exits non-zero on failure and prints per-check results.
- [x] A missing configuration and an empty command set are handled clearly.
- [x] `make check` passes.

## Git

Branch: `task/T030-validation-runner`
Commit: `task(T030): add configuration-driven validation runner`
PR: `[Task T030] Add validation runner`

## Out of Scope

Parallel independent checks; the `run` lifecycle that calls this stage; the
Git change detector and review stages.
