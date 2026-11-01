# T042 --- One-Command Run Hardening

> Completes the PLAN-first one-command workflow: honest completion states, plan
> graph validation, reconciliation, runtime-state gitignore, and concise output.

## Status

DONE

## Objective

Make `sop run` safe enough to be the normal entry point:

```text
local completion is LOCAL_DONE (never fabricated PR_OPEN/CI_*/MERGED)
the compiled/loaded plan is validated (ids, references, cycles, non-empty)
a changed PLAN after tasks exist stops with NEEDS_HUMAN, not a silent rebuild
runtime state does not dirty an otherwise clean source tree
startup output is concise and actionable errors name the fix
```

## Dependencies

- T041 (PLAN-first run), T032/T034 (run + graph execution), T025/T026 (config)

## Scope

- `internal/domain`: a distinct `LOCAL_DONE` terminal state; `REVIEW_PASS` may go
  to `LOCAL_DONE` or `PR_OPEN`; `LOCAL_DONE` satisfies dependencies; `Block`
  rejects completed tasks; store/scheduler recognize the state.
- `internal/planner`: `Validate` now detects dependency cycles with the path;
  `Compile` surfaces validation errors for a recognizable plan instead of asking
  the agent to replace it.
- `internal/planflow`: split into `discoverPlanningSource`/`ensurePlan`/
  `validatePlan`/`ensureTasks`/`reconcileState`; `source_sha256` provenance;
  reconciliation stop; PRD → sibling `PLAN.md`; actionable validation errors.
- `internal/cli`: honest local completion, `workflow.mode` guard, concise startup
  output, `Running:` lines, and `.agent-sdlc/` gitignored on init.
- `internal/config`: `workflow.mode` (`local` | `pull-request`).
- Docs: README, ARCHITECTURE, PLAN, LESSONS, and tests throughout.

## Rules

- Persisted state describes what actually happened: a local run records
  `LOCAL_DONE`; remote states are reserved for a real PR/CI/merge.
- The agent is never the authority: a compiled or generated plan must pass
  deterministic Go validation, and a recognizable-but-invalid plan is rejected
  with a diagnostic rather than silently normalized.
- A human PLAN is never overwritten; a generated plan is written beside the PRD
  only when no PLAN exists.
- Rebuilding tasks over existing history is not automatic: a changed source after
  tasks exist stops with `NEEDS_HUMAN` and explains why.
- SOP runtime state (`.agent-sdlc/`) is gitignored so it never counts as a source
  modification.

## Tests

Domain: `LOCAL_DONE` is terminal and satisfies dependencies; block rejects it.
Planner: cycle detection; a cyclic recognized plan errors without fallback.
planflow: cycle/missing-dependency diagnostics; PRD writes `docs/PLAN.md`;
changed-plan-with-tasks reconciliation; idempotent reuse. CLI: repeated `sop run`
is idempotent; completion persists `LOCAL_DONE` and never `DONE`/`MERGED`; a
subdirectory validation command runs as configured; startup output; actionable
no-source error.

## Acceptance Criteria

- [x] Local completion records `LOCAL_DONE`; remote states are never fabricated.
- [x] Plans are validated deterministically (ids, references, cycles, non-empty).
- [x] A changed PLAN after tasks exist stops with an actionable `NEEDS_HUMAN`.
- [x] A PRD-generated plan is written to a sibling `PLAN.md` without overwriting.
- [x] `.agent-sdlc/` is gitignored; startup output is concise.
- [x] `make check` passes.

## Git

Branch: `task/T042-one-command-hardening`
Commit: `task(T042): harden the one-command run`
PR: `[Task T042] Harden the one-command run`

## Out of Scope

The remote `pull-request` workflow mode; automatic `NEEDS_HUMAN` reconciliation
tooling beyond the diagnostic.
