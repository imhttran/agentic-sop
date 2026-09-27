# T045 --- Keep Generated Artifacts Out of the Project Root

> `sop run` no longer writes `PLAN.md` or `.gitignore` to the project root.

## Status

DONE

## Objective

Stop scattering SOP output across the project root. Generated artifacts get a
home of their own:

```text
docs/reports/<plan-id>.md   generated human plan (from a PRD)
.agent-sdlc/                machine state + run reports (ignores itself for Git)
```

so a run adds **no** files to the project root.

## Problem

Running `sop run` (for example with a root `PRD.md`) created two root files:

- `PLAN.md` — the human plan generated from the PRD (written beside the PRD), and
- `.gitignore` — created by init to keep `.agent-sdlc/` out of Git.

## Dependencies

- T041/T042/T043/T044 (PLAN-first run and bootstrap)

## Scope

- `internal/planflow/planflow.go`: `writeGeneratedPlanDoc` writes to
  `docs/reports/<plan-id>.md` instead of beside the PRD (never the root).
- `internal/cli/init.go`: `ensureRuntimeIgnored` writes `.agent-sdlc/.gitignore`
  containing `*` (the directory ignores itself) instead of editing the project's
  root `.gitignore`.
- `internal/cli/drive.go`: the startup block reports `Wrote docs/reports/…`.
- Tests: assert the generated plan lands under `docs/reports/` and that no root
  `PLAN.md` / `.gitignore` is created.

## Rules

- Generated, machine-owned output never goes to the project root.
- Runtime state is Git-invisible without touching the project's own `.gitignore`:
  a `.agent-sdlc/.gitignore` with `*` ignores the whole state directory.
- The generated plan is a derived report, so it is written under `docs/reports/`
  and may be regenerated; the project's own `.gitignore` is never modified.
- Machine state and reports remain under `.agent-sdlc/`; only the generated human
  plan moves to `docs/reports/`.

## Tests

planflow: generating from a PRD writes `docs/reports/prd.md` and nothing at the
root. CLI: a PRD run writes `docs/reports/prd.md`, creates no root `PLAN.md`, and
creates no root `.gitignore`. Existing run/graph/plan tests still hold.

## Acceptance Criteria

- [x] `sop run` adds no files to the project root.
- [x] A PRD-generated plan is written under `docs/reports/`.
- [x] `.agent-sdlc/` ignores itself for Git without a root `.gitignore`.
- [x] `make check` passes.

## Git

Branch: `task/T045-artifacts-out-of-root`
Commit: `task(T045): keep generated artifacts out of the project root`
PR: `[Task T045] Keep generated artifacts out of the project root`

## Out of Scope

Making the reports directory configurable; gitignoring `docs/reports/` itself.
