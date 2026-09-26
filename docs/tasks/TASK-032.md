# T032 --- Local Run Lifecycle

> Implements plan `PLAN-JEV.md` T007/T010/T011/T020-T027 and PRD §7, §8.11, via
> the `sop run` command.

## Status

DONE

## Objective

Drive one task file through the local lifecycle and leave an inspectable record:

```text
TASK.md
  → plan → implement → detect changes (Git)
  → validate (fail-fast) → review → quality gate
  → report.md / report.json        → stop at the human gate
```

## Dependencies

- T027 (task-file loader), T025/T026 (configuration), T030 (validation),
  T031 (review), T028 (quality gate)
- Stage 4/7/8 (planner, Git adapter, test runner)

## Scope

- `internal/run/run.go`: run directory + stage state (`CREATED`…`FAILED`) under
  `.agent-sdlc/runs/<id>/`, artifact writes, persisted `state.json`.
- `internal/cli/run.go`: the `sop run TASK.md` command and the run report.
- `internal/cli/cli.go`: dispatch and help.
- `internal/cli/cli_test.go`: capability-aware fake agent and run tests.

## Rules

- Git is authoritative for what changed; the agent's summary is never evidence.
  A run with no detected changes fails.
- Validation runs before review and fails fast, so uncompilable changes never
  reach review.
- The quality gate is deterministic (`quality.Evaluate`); the model only produces
  findings.
- A run never commits, pushes, or merges. A passing run stops at the human gate.
- Every stage writes an artifact so a terminated run stays inspectable.

## Tests

`internal/run`: directory/state creation, stage persistence, artifact writes,
id validation. CLI: an end-to-end pass writes every artifact and prints the
human-gate message; no changes fails; a build failure fails the gate; a blocking
finding fails the gate; a missing task file errors; bad args usage.

## Acceptance Criteria

- [x] `sop run TASK.md` drives plan → implement → validate → review → gate → report.
- [x] Run artifacts and stage state are persisted under `.agent-sdlc/runs/<id>/`.
- [x] Git is authoritative for changes; uncompilable changes skip review.
- [x] The gate is deterministic and a pass stops at the human gate (no commit).
- [x] `make check` passes.

## Git

Branch: `task/T032-local-run-lifecycle`
Commit: `task(T032): add local run lifecycle`
PR: `[Task T032] Add local run lifecycle`

## Out of Scope

The fix loop (`FIXING`) and re-validation; plan/DAG-driven execution over the
persisted task graph; commit/PR/merge steps.
