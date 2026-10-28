# Project State Lock

## Purpose

SOP keeps durable state in two stores that cannot share a transaction:

- JSON artifacts under `.agent-sdlc/` — `plan.json`, `plan.meta.json`, `approval.json`
  and `approval-history.json`, and the plan lifecycle records;
- the SQLite task graph in `.agent-sdlc/state.db`.

Several operations read a value, decide from it, and then write: an approval
decision reads the approval head; invalidation reads the head and the task
definition; reconciliation reads the active graph and the recorded plan. Without
mutual exclusion, a competing writer can change the input between the read and the
write.

`internal/projectlock` provides one project-scoped advisory lock that participating
writers take before they read those inputs and release only after they have written.

## What it guarantees

- **Mutual exclusion between participating writers**, across processes, for the
  whole read-check-write sequence.
- **Bounded, cancellable acquisition.** A caller whose context is done returns
  immediately; the descriptor is closed only after the background acquisition
  resolves, so cancellation cannot leak a descriptor or release a lock that was
  never held.
- **Release on process exit.** The kernel (Unix `flock`) and the OS (Windows
  `LockFileEx`) release the lock when the holder exits, so a crash does not wedge
  the project. `Release` is idempotent.
- **No silent degradation.** A platform that cannot lock returns `ErrUnsupported`
  rather than proceeding unprotected. Windows is supported natively
  (`golang.org/x/sys/windows`), so its behavior is preserved rather than weakened.

## What it does not guarantee

- **It is not a transaction across the two stores.** A process that dies between
  the SQLite write and the JSON write still leaves them disagreeing. Reconciliation
  deliberately persists the graph before the machine plan so a failed plan write
  surfaces as an error with the plan intact; that window is narrowed by the lock only
  against *participating* writers, not against a crash.
- **Participation is a convention.** A writer that does not take the lock is not
  excluded, and nothing detects one. The participants are listed below so a new
  writer can be required to join.
- **It is not a substitute for validation of inputs.** A holder must still re-check
  the state it read; the lock only makes that state stable for the duration.

## Participants

The lock is taken once per operation, never nested, and never held across a model
call or an external validation run.

| Path | State written |
| --- | --- |
| `planflow.Reconcile` | `plan.json`, `plan.meta.json`, task graph |
| `planflow.Supersede` | `plan.json`, `plan.meta.json`, task graph |
| `planflow.Complete` | plan lifecycle records, metadata |
| `planflow.Historicalize` | plan lifecycle records, metadata |
| `approval.Service.Request` | `approval.json`, history |
| `approval.Service.Approve` / `Decline` | head, history, task requeue |
| `approval.Service.Supersede` | head, history |
| `approval.Service.Invalidate` | head, history |
| `sop task complete` (external completion, via `run.ResolvePendingApproval`) | head, history |

### Not yet a participant

`planflow.Prepare`'s initial-plan path (`initialize` → `ensurePlan` → `ensureTasks`)
is not yet wired. It can invoke the planner model, and the lock must not be held
across that call, so it needs its own narrow boundary around the persistence step
rather than a wrapper at entry. Until then, plan creation is not serialized with the
other writers.
