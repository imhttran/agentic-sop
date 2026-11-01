# T014 --- GitHub Adapter

## Status

DONE

## Objective

Provide the remote-workflow boundary so a locally successful task can reach
`PR_OPEN` (and later CI/merge) without coupling the orchestrator to a provider.

The adapter owns remote operations only; workflow state stays with the
orchestrator. It is a thin, testable boundary: a `Client` interface with a
command-backed implementation (`gh`/`git`) and fakes for tests.

## Operations

- push a task branch;
- create/update a pull request;
- retrieve a pull request (state, url, head/base);
- inspect PR checks;
- merge a pull request.

## Dependencies

- T007 --- Git adapter

## Scope

Add `internal/github`:

- model types: `PullRequest`, `Check`, `CheckState`;
- `Client` interface: `PushBranch`, `CreatePullRequest`, `GetPullRequest`,
  `Checks`, `Merge`;
- `CommandClient` implemented over the `gh` and `git` CLIs (structured
  arguments, no shell), with an injectable command runner so parsing and
  argument construction are unit-testable without network.

## Rules

- No shell: `exec.CommandContext(ctx, "gh"|"git", args...)`.
- Commands come from trusted configuration; no untrusted data is interpolated
  into a shell.
- Deterministic parsing of `gh --json` output; malformed output is an error.
- No provider-specific SDK; `gh`/`git` are the only dependencies.
- Tests require no network.

## Tests

Inject a fake command runner. Cover: push builds the expected `git push`
arguments; PR JSON parsed into `PullRequest`; checks JSON parsed with normalized
states; merge selects the requested method; malformed JSON → error; command
failure → error with diagnostics.

## Acceptance Criteria

- [x] A `Client` interface exposes push/PR/create/get/checks/merge.
- [x] A command-backed implementation uses `gh`/`git` with structured args.
- [x] `gh --json` output is parsed deterministically.
- [x] Malformed output and command failures are errors.
- [x] Tests require no network or credentials.
- [x] `make check` passes.

## Git

Branch: `task/T014-github-adapter`
Commit: `task(T014): add github adapter`
PR: `[Task T014] Add GitHub adapter`

## Out of Scope

CI remediation policy (T016), merge gate policy (T017), completion loop (T018).
