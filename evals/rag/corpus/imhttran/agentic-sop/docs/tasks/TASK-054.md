# T054 --- Verification-First Fast Path

> A plan stage may declare `execution_mode: verify-first`: SOP runs the configured
> deterministic validation before invoking any agent, and calls the implementation
> agent only when that validation fails.

## Status

DONE

## Objective

A verification-only task (for example "verify the controller/SOP boundary") used to
invoke the implementation agent before SOP ran its own validation, costing minutes
for work that required no repository change. A task can now declare that it is
verifiable without an agent:

```text
task becomes runnable
        ↓
verify-first?  ── no ──→ IMPLEMENT → validate → review → gate → fix
        ↓ yes
validate ── pass ──→ gate → complete locally (no agent invoked)
        └─ fail ──→ IMPLEMENT(context: the failure) → validate → review → gate → fix
```

## Dependencies

- T047/T050 (structured command-agent outcomes)

## Scope

- `internal/domain/execution.go`: the `ExecutionMode` vocabulary — `implement`
  (default, the zero value) and `verify-first` — with `VerifyFirst`,
  `KnownExecutionMode`, and `ParseExecutionMode` (normalization).
- `internal/planner`: `Stage.ExecutionMode` (`execution_mode` in plan.json),
  validated; the Markdown compiler reads an `### Execution` sub-section and the
  renderer emits it when set.
- `internal/taskbuilder`: the mode is carried onto the built task.
- `internal/store`: schema v3 adds `tasks.execution_mode`; existing rows default to
  the implement mode.
- `internal/taskfile`: `Spec.ExecutionMode`, read from a `## Execution` section (an
  unrecognized value is ignored, not an error, so an unrelated section is safe).
- `internal/cli`: the fast path in `runStages`, the failure context handed to the
  agent, and the record (`verified_first` in the run report).
- Tests and docs.

## Rules

- The mode is explicit plan/task metadata; it is never inferred from a title or
  prose ("Verify", "Check", "Validate").
- The zero value is the implement mode, so existing plans and tasks are unchanged.
- The fast path runs the configured validation first. A pass invokes no agent
  (neither the micro-planner, IMPLEMENT, nor FIX) and continues through the
  deterministic quality gate; a failure hands the failure to the implementation
  agent and the ordinary lifecycle (validate → review → fix) continues.
- A verify-first task with no configured check takes the ordinary implementation
  path: a fast path that verifies nothing is not a verification.
- The agent never decides pass/fail: a fast-path task passes only when its
  configured checks pass. Review, fix-cycle, retry, and human-approval semantics
  are unchanged.
- The structured command-agent outcome protocol is unchanged.

## Tests

domain: `ParseExecutionMode`/`VerifyFirst`/`KnownExecutionMode`. planner: an
unknown `execution_mode` fails validation; `### Execution` round-trips through
render/compile; an absent mode stays implement. taskbuilder: the mode reaches the
task. store: the mode round-trips and a pre-v3 database migrates to implement.
taskfile: `## Execution` parses; an unrelated `## Execution` section is ignored.
cli: a passing verify-first task runs with **zero** agent calls and records
`verified_first`; a failing one invokes the agent with `# Validation failed`
context; an ordinary task still invokes IMPLEMENT; a verify-first task with nothing
configured falls back to the implementation path. Verified end-to-end with a real
binary.

## Acceptance Criteria

- [x] A task declares verify-first through explicit plan/task metadata only.
- [x] A passing verify-first task invokes no agent and completes locally.
- [x] A failing verify-first task invokes the agent with the failure as context.
- [x] Tasks without the setting keep the existing behaviour.
- [x] Validation, review, fix-cycle, retry, and human-approval guarantees are unchanged.
- [x] `make check` passes.

## Git

Branch: `task/T054-verify-first-fast-path`
Commit: `task(T054): verification-first fast path`
PR: `[Task T054] Verification-first fast path`

## Out of Scope

Auto-detecting verification-only tasks; running an all-verify-first graph with no
agent configured; a per-task validation subset (the fast path runs the project's
configured checks).
