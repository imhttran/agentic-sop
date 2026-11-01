# T044 --- Named Plan Execution

> Adds `sop run PLAN.md` (execute a specific plan end to end) and `sop run --task
> TASK.md` (single task), with plan identity so multiple plans never mix.

## Status

DONE

## Objective

```bash
sop run                        # discover the project's normal PLAN
sop run docs/PLAN-Hardening.md  # execute a specific PLAN, end to end
sop run PLAN-Hardening.md       # resolved against docs/ when unambiguous
sop run --task TASK.md          # one task through the local lifecycle
```

The old `sop run FILE.md` meaning ("run one task") is replaced: a file argument is
now an execution PLAN, and single-task execution is explicit via `--task`.

## Dependencies

- T041/T042/T043 (PLAN-first run, honest completion, agent-free bootstrap)

## Scope

- `internal/planflow`: `Options.PlanSource` (authoritative explicit source),
  `ResolvePlanPath` (root, then `docs/`; ambiguity is an error), plan identity
  (`source`, `source_sha256`, `plan_id`) in metadata, and reconciliation that
  distinguishes same/changed/different plans.
- `internal/cli/run.go`: `parseRunArgs` and the `run [PLAN.md | --task TASK.md]`
  contract; the single-task body moves to `runSingleTask`.
- `internal/cli/drive.go`: `runGraph(planArg, …)` resolves the plan and threads it
  through `planflow.Prepare`; startup shows `Source`/`Plan ID`; a completion block
  is printed when every task is done.
- Tests and docs.

## Rules

- A named PLAN is authoritative for that execution: it is never replaced from the
  PRD, never falls back to `docs/PLAN.md`/`PLAN.md`, and never continues another
  machine plan.
- Identity is `(source, source_sha256, plan_id)`; modification time is never used.
  A plan id is the file name without extension, lowercased and dash-separated
  (`docs/PLAN-Hardening.md` → `plan-hardening`).
- Same source + same hash → reuse (`Plan: current`). Same source + changed hash,
  or a different plan while tasks exist → stop with an actionable `NEEDS_HUMAN`
  (history is never silently discarded; two plans are never mixed).
- Path resolution is unambiguous: both `./X.md` and `./docs/X.md` present is an
  error naming both candidates.
- One engine: named, discovered, and resumed plans all flow through
  `planflow.Prepare` + `driveGraph`; no second orchestration path.
- All safety properties are unchanged: configured validation, review, quality
  gates, `max_fix_cycles`, the human-approval boundary, and Git policy.

## Tests

planflow: `ResolvePlanPath` (root, docs fallback, explicit docs, absolute,
ambiguous, missing) and `planID`. CLI: named plan executes and reports `Plan ID`;
`PLAN-Hardening.md` resolves to `docs/`; ambiguous and missing plans error; a
different plan while tasks exist stops; a changed plan stops; `--task` runs the
single-task lifecycle (the existing lifecycle tests now use `--task`); repeated
named runs are idempotent; completion prints `SOP COMPLETE`.

## Acceptance Criteria

- [x] `sop run PLAN.md` executes that plan end to end (discover → init → compile → validate → tasks → graph).
- [x] `sop run PLAN-Hardening.md` resolves to `docs/PLAN-Hardening.md`; ambiguity and missing plans error clearly.
- [x] `sop run --task TASK.md` runs one task; the old `sop run FILE.md` task meaning is gone.
- [x] Plan identity (source + sha256 + plan id) prevents mixing plans or stale tasks.
- [x] `sop run` (no argument) still discovers the project plan; one engine is shared.
- [x] `make check` passes.

## Git

Branch: `task/T044-named-plan-execution`
Commit: `task(T044): add named plan execution`
PR: `[Task T044] Named plan execution`

## Out of Scope

Cross-plan reconciliation that rewrites an existing task graph (a different plan
or a changed plan while tasks exist stops with `NEEDS_HUMAN`); the remote
pull-request workflow mode.
