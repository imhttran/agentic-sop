# T029 --- Command Policy

> Implements plan `PLAN-wrapup.md` T043 (Command Policy).

## Status

DONE

## Objective

Classify a structured command before it runs:

```text
SAFE                read-only / verification, may run unattended
REQUIRES_APPROVAL   changes shared state, needs a human
DENIED              must never run unattended (force-push, ...)
```

Project policy may tighten a default class but never loosen one.

## Dependencies

- Stage 7 (Git adapter, T007) and Stage 8 (Test runner, T008) for the defaults

## Scope

- `internal/commandpolicy/commandpolicy.go`: `Class`, `Classify`, `Policy`.
- `internal/commandpolicy/commandpolicy_test.go`: table-driven classification
  and tightening tests.

## Rules

- Structured argument lists only; no shell strings, matching the repository's
  "trusted commands, structured arguments" boundary.
- Defaults: `go test/build/vet/fmt/list` and read-only git subcommands are
  `SAFE`; `git commit`/`git push` are `REQUIRES_APPROVAL`; a force-push
  (`--force`, `-f`, `--force-with-lease[=...]`) is `DENIED`.
- An unrecognized program is conservatively `REQUIRES_APPROVAL`; an empty
  command is an error.
- Policy rules match whole argument prefixes and use `max` strictness, so they
  can only make a command stricter.
- Git global options that take a value (`-C`, `-c`, `--git-dir`, ...) do not
  hide the subcommand.

## Tests

Defaults for `go` and `git` (safe, approval, denied); force-push flag variants;
`git` alone; unknown program; write-capable `go mod tidy`; empty/blank command
errors; policy tightening (deny a safe command, require approval for a safe
command, deny an approved command); a policy cannot loosen a denied command; a
prefix must match whole tokens.

## Acceptance Criteria

- [x] Commands classify as `SAFE`, `REQUIRES_APPROVAL`, or `DENIED`.
- [x] Force-push is denied; push/commit require approval; read-only commands are safe.
- [x] Project policy can only tighten the defaults.
- [x] Classification uses structured arguments, never a shell.
- [x] `make check` passes.

## Git

Branch: `task/T029-command-policy`
Commit: `task(T029): add command policy`
PR: `[Task T029] Add command policy`

## Out of Scope

Enforcing the policy inside the executors (a follow-up wiring task); an approval
workflow.
