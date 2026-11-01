# T043 --- Bootstrap Independent of the Agent

> Fixes `sop run` so preparation (init, plan compile, task creation) no longer
> depends on the configured agent.

## Status

DONE

## Objective

`sop run` with no arguments must be the whole chain, whether or not an agent is
configured:

```text
discover docs/PLAN.md → PLAN.md → docs/PRD.md → PRD.md
  → initialize state if needed
  → compile PLAN into .agent-sdlc/plan.json
  → create missing tasks
  → resume the DAG

(the agent is required only to execute — and to generate from a PRD or
 normalize an unrecognizable PLAN.md)
```

## Bug

`sop run` built the agent **before** preparing. With no agent configured it
stopped at `no agent configured: set SOP_AGENT_COMMAND` and created only
`.agent-sdlc/config.yaml` and `state.db` — no `plan.json` and no tasks. That left
preparation effectively as the separate `init`/`plan`/`tasks` steps the workflow
is meant to fold in.

## Dependencies

- T041/T042 (PLAN-first run), T025/T026 (config, provider selection)

## Scope

- `internal/cli/drive.go`: `runGraph` prepares first and builds the agent lazily;
  execution reports a missing agent clearly and only after preparation.
- `internal/planflow/planflow.go`: a distinct `errNoAgent` (actionable on its own,
  not wrapped as a validation failure) for the PRD-generation and
  unrecognizable-PLAN cases.
- Tests: planflow no-agent PRD; CLI bootstrap-without-agent.

## Rules

- Preparation is deterministic and agent-free for a PLAN.md (compiled) or an
  existing `plan.json`; it must run regardless of the agent.
- The agent is required only where it is genuinely used: executing the lifecycle,
  generating a plan from a PRD, or normalizing a PLAN.md that is not recognizable
  as a plan. Those cases fail with an actionable message.
- A missing agent does not leave half-built state; the plan and tasks are created
  first, then execution reports the agent requirement.
- Idempotent: a second `sop run` reuses the plan and task graph.

## Tests

planflow: generating from a PRD without an agent errors with a no-agent message.
CLI: `sop run` with a nil agent still compiles `docs/PLAN.md` into `plan.json`,
creates the task graph, prints the startup block, and then reports that it cannot
execute. Existing end-to-end and idempotency tests still hold.

## Acceptance Criteria

- [x] `sop run` discovers the PLAN/PRD, initializes state, compiles `plan.json`, and creates tasks with no agent configured.
- [x] The agent is required only for execution (and PRD generation / normalization), with an actionable message.
- [x] No half-built state: plan and tasks exist after a preparation-only run.
- [x] `make check` passes.

## Git

Branch: `task/T043-agent-free-bootstrap`
Commit: `task(T043): decouple bootstrap from the agent`
PR: `[Task T043] Decouple bootstrap from the agent`

## Out of Scope

Auto-detecting validation commands (configured commands remain authoritative).
