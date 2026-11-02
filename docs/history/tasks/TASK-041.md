# T041 --- PLAN-First One-Command Run

> Makes `sop run` the normal entry point: discover the planning source, prepare
> the machine plan and tasks, then execute — all idempotently.

## Status

DONE

## Objective

A developer can put a `docs/PLAN.md` and/or `docs/PRD.md` in the project and run:

```bash
sop run
```

SOP performs the equivalent of `sop init` → `sop plan` → `sop tasks` → `sop run`
safely and repeatably:

```text
discover source → ensure state → compile/generate plan.json → create tasks
                → scheduler → lifecycle → gate → continue graph
```

`init`, `plan`, and `tasks` remain explicit lower-level commands.

## Dependencies

- T025 (configuration), T026 (agent selection), T032 (run), T034 (graph execution)

## Source precedence

```text
1. an existing valid .agent-sdlc/plan.json (reused only when its source is unchanged)
2. docs/PLAN.md
3. PLAN.md
4. docs/PRD.md
5. PRD.md
```

A human PLAN is preferred over the PRD, and `plan.json` never silently overrides a
newer PLAN.

## Scope

- `internal/planner/markdown.go`: `PlanFromMarkdown` (deterministic compiler for a
  human `PLAN.md`) and `(*Planner).Compile` (compiler with an agent-normalization
  fallback).
- `internal/planflow/planflow.go`: discovery, provenance metadata, fingerprinting,
  plan rebuild/reuse, and task creation (`Prepare`).
- `internal/cli/init.go`: `ensureProjectInitialized` extracted and shared.
- `internal/cli/drive.go`: `runGraph` now bootstraps before driving; `driveGraph`
  takes an open store.
- Tests in each package plus CLI end-to-end tests.

## Rules

- `PLAN.md` is authoritative execution intent: when it exists, the PRD is not used
  to regenerate a different plan.
- `plan.json` is rebuilt only when its recorded source path or content fingerprint
  no longer matches, so a human-edited PLAN is never silently overridden and an
  unchanged PLAN is never recompiled.
- The compiler is deterministic first; the agent is consulted only to normalize a
  document the compiler cannot turn into a valid plan (and to generate a plan from
  a PRD, which only happens when no PLAN exists).
- Every step is idempotent: existing state, configuration, tasks, and an
  up-to-date plan are all reused.
- Local completion is unchanged (a passing lifecycle advances the task to `DONE`;
  no remote PR/CI/merge).

## Tests

Compiler: `RenderMarkdown` round-trips; human variants (bullets, checkboxes,
`- ` separators, `Scope`/aliases, ignored unknown sub-sections); deterministic
compile is preferred; the agent is only used as a fallback. planflow: compiles a
PLAN and creates tasks; reuses an unchanged plan; rebuilds when the source
changes; generates from a PRD; prefers PLAN over PRD; errors when no source
exists; skips when tasks already exist. CLI: no source errors and initializes
state; an existing `plan.json` is turned into tasks and executed; a `docs/PLAN.md`
is compiled, tasks created, and the graph run to done; a `docs/PRD.md` is
generated and run.

## Acceptance Criteria

- [x] `sop run` (no argument) prepares the project and executes the graph.
- [x] PLAN is preferred over PRD; the PRD never replaces an existing PLAN.
- [x] `plan.json` never silently overrides a newer PLAN (path + fingerprint).
- [x] `init`/`plan`/`tasks` remain available as lower-level commands.
- [x] `make check` passes.

## Git

Branch: `task/T041-plan-first-run`
Commit: `task(T041): make sop run the one-command workflow`
PR: `[Task T041] PLAN-first one-command run`

## Out of Scope

Auto-writing a `PLAN.md` when generating from a PRD (left to `sop plan`);
rebuilding tasks when the plan changes while tasks already exist; remote
completion.
