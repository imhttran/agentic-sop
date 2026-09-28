# T057 --- Resume the Active Task in `sop run`

> `sop run` must continue an interrupted task instead of printing
> `no runnable task (ACTIVE_TASK)`, while `ACTIVE_TASK` still prevents starting a
> *different* task. One shared interpretation of persisted state drives both
> `sop run` and `sop resume`.

## Status

DONE

## Objective

A named plan was interrupted mid-lifecycle. The persisted state was:

```text
AHV2001 PLANNED
AHV2002 READY     (run stage: PLANNING)
AHV2003 PLANNED
```

`sop resume` correctly reported `AHV2002 CREATE_BRANCH`, but `sop run` refused:

```text
no runnable task (ACTIVE_TASK)
```

The root cause: `scheduler.Next` returns `ActiveTask` for any status past
`PLANNED` that is not terminal — including `READY`, the very status the scheduler
sets for the task it just selected. `driveGraph` had no `ActiveTask` case, so it
fell through to `default` and stopped. The resumption logic already existed in
`sop resume` (`resume.ActionFor`); `sop run` simply never consulted it.

```text
sop run
   ↓
prepare/reconcile plan
   ↓
active task in flight?
   ├── no → select next runnable task
   └── yes → inspect persisted run state → next legal action → resume same task
```

## Scope

- `internal/scheduler`: export `IsActive(status)` as the single definition of
  "occupies the execution slot".
- `internal/resume`: export `ActionFor(status)` wrapping the existing
  status→action map.
- `internal/run`: add a read-only `Load(projectDir, id)` reading `state.json`
  (unlike `New`, it must not clobber an interrupted run).
- `internal/cli`: handle `scheduler.ActiveTask` in `driveGraph` via
  `resumeActiveTask`, reuse `runScheduledTask` for the resumed lifecycle, and let
  `completeTask` continue from wherever a task already is on the local path.
- Tests and docs.

## Rules

- **One interpretation.** No second state machine: `scheduler.IsActive` decides
  which task is in flight, `resume.ActionFor` decides the next legal action — the
  same decision `sop resume` reports.
- **`ACTIVE_TASK` is preserved.** It must still prevent selecting a *different*
  task. Only the same task is resumed. Zero or two or more in-flight tasks is an
  actionable error, not a guess.
- **Local runs stay local.** From `PR_OPEN`/`CI_RUNNING`/`CI_PASS` the state
  machine only reaches a terminal through `MERGED`; a local run opens no PR and
  runs no CI, so such a task is reported precisely (task, status, stage, next
  action, `sop resume <id>`) rather than force-completed into a lie.
- **State is not reset.** No deleting `state.db`/`runs`, no status rewrites, no
  re-running `sop init`; existing interrupted state is resumable as-is.
- **Plan identity is preserved.** An active task from another plan is never
  silently resumed under a named plan.

## Tests

Deterministic (`internal/cli/cli_test.go`): a planned task with no in-flight work
is selected normally; a `READY` task with persisted stage `PLANNING` is resumed and
completed; an active task is resumed instead of starting another; a task whose
lifecycle already passed is completed without invoking the agent; ambiguous active
tasks are rejected; a remote-parked active task is refused with a precise message;
a repeated `sop run` is idempotent; and `sop resume` still reports
`AHV2002 CREATE_BRANCH` for a ready task. Verified once end-to-end with the real
binary against the reported interrupted state.

## Acceptance Criteria

- [x] `sop run` resumes an interrupted active task through the normal lifecycle.
- [x] `sop run` never starts a different task while one is in flight.
- [x] `sop resume` and `sop run` share one persisted-state → next-action rule.
- [x] Ambiguous or unresumable active state yields an actionable error, never a
      silent continue.
- [x] No state is deleted or reset; the fix is a lifecycle change.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `task/T057-resume-active-task`
Commit: `task(T057): resume the active task in sop run`
PR: `[Task T057] Resume the active task in sop run`

## Out of Scope

Agent Harness V2; Ollama/DeepSeek behavior; plan parsing; removing
`ACTIVE_TASK`; force-completing remote-parked tasks; committing, pushing, or
merging.
