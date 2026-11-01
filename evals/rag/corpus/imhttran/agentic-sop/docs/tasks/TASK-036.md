# T036 --- Git Workflow Commands

> Implements plan `PLAN-JEV.md` T028 (optional commit) and T029 (GitHub PR).

## Status

DONE

## Objective

Make the two remote/local Git operations explicit and gated, never automatic:

```text
sop commit [TASK.md] [--yes]   local commit, no push
sop pr     TASK.md   [--yes]   push branch + open a pull request
```

Both honour the human gate (`human.approval_before_commit`, default true): with
the gate on, running without `--yes` refuses.

## Dependencies

- Stage 7 (Git adapter), Stage 14 (GitHub adapter), T025 (configuration)

## Scope

- `internal/git/git.go`: `Adapter.Add` (structured staging; no paths stages all).
- `internal/cli/commit.go`: `sop commit`, `commitMessage`, shared `parseGateArgs`.
- `internal/cli/pr.go`: `sop pr` (branch from task file, push + create PR).
- `internal/cli/cli.go`: `deps.commit`, `deps.newGitHub`, dispatch, help.
- `internal/cli/cli_test.go`: fake GitHub client and command tests.

## Rules

- Human gate: when `human.approval_before_commit` is true, a missing `--yes` is an
  explicit error, not a silent action.
- Commit messages are derived deterministically from the task file
  (`task(<id>): <title>`), never from a model.
- The PR branch is `task/<id>-<slug>` (the Git adapter's deterministic name), and
  the base is the configured integration branch. `sop pr` requires a task file
  with an id.
- Neither command merges; neither force-pushes. The Git adapter owns staging and
  commit via structured arguments (no shell).

## Tests

Commit: refused without `--yes` when gated (and the commit never runs); message
derived from the task file; allowed without `--yes` when the gate is disabled.
Pr: pushes the deterministic branch and creates a PR against the integration
branch; refused without `--yes`; requires a task file.

## Acceptance Criteria

- [x] `sop commit` commits locally with a task-derived message; no push.
- [x] `sop pr` pushes the task branch and opens a pull request.
- [x] Both honour the human gate and require `--yes` when it is on.
- [x] Git/GitHub operations use structured arguments behind adapters, not a shell.
- [x] `make check` passes.

## Git

Branch: `task/T036-git-workflow-commands`
Commit: `task(T036): add commit and pr commands`
PR: `[Task T036] Add commit and pr commands`

## Out of Scope

Merging (the merge gate owns that); branch creation (the run lifecycle / task
runner); force-push (denied by the command policy).
