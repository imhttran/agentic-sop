# Proposal: shared execution exclusion across CLI, MCP, and ACP

Status: **PROPOSAL — not approved, not implemented.** No global CLI execution
semantics are changed by this document or by the ACP change set.

Related: [`ACP-EXECUTION-EXCLUSION-GAP.md`](ACP-EXECUTION-EXCLUSION-GAP.md)
records the requirement this proposal would satisfy.

## Goal

Guarantee **exactly one active execution per project**, regardless of which
interface starts it, without giving any interface a way to bypass lifecycle,
approval, or verification gates.

“Execution” means a process that runs SOP work against a project: `sop run`, and
the governed `sop prompt` child a start launches. Plan preparation and approvals
are metadata writes and are covered by the existing `projectlock`; the two
concerns must stay distinct.

## 1. Lock ownership

Introduce one **project execution lease** at
`.agent-sdlc/state.exec.lock`, separate from `state.lock` (metadata) and from
`state/runs/work/.lock` (start serialization).

The lease file records, atomically:

```json
{ "owner": "sop", "pid": 1234, "host": "…", "kind": "run|prompt",
  "job_id": "work-…", "task_id": "ACP-003", "attempt": 2,
  "started_at": "…", "heartbeat_at": "…", "argv_digest": "…" }
```

Rules:

- **Owned by the executing process.** Whoever executes acquires the lease before
  any lifecycle work and releases it when the execution ends.
- **ACP/MCP delegate ownership to the child.** The adapter does not hold the
  lease; the launched child acquires it as its first lifecycle act and keeps it.
  The parent records which job/pid owns the lease so a client can be told who
  holds the project.
- **Two-phase for starts.** `startWork` keeps its short `O_EXCL` start lock to
  make the *decision* to launch atomic; the lease makes the *execution* exclusive.
  The start lock is released as today, immediately after launch.
- **Fixed ordering.** Execution lease → metadata lock. A process that needs both
  takes the lease first; metadata-only operations never take the lease. This
  avoids deadlock with `planflow` and `approval`.
- **No nesting.** A child started by a parent that already holds the lease
  inherits it rather than re-acquiring it (explicit env marker plus pid/job
  identity check), so ACP → child never self-contends.

## 2. Child lifetime

- The lease is bound to the child process, not to the editor session, so closing
  Zed does not silently free a still-running execution.
- The parent writes the lease owner record **before** `cmd.Start()` returns a pid
  and updates it with the pid in the same atomic write; a lease with no pid is
  treated as in-progress for a short grace period, then as stale.
- The child releases the lease in a deferred exit path and again on signal
  handlers, so normal exit, failure, and `SIGTERM` all release it.
- The lease is **not** released when the editor, MCP server, or tunnel client
  disconnects.

## 3. Cancellation

- `session/cancel` and client EOF cancel **only** adapter-owned execution, as the
  plan already requires.
- Cancellation is `SIGTERM` → bounded wait (documented constant) → `SIGKILL` to
  the owned process group.
- The lease is released only after the child is reaped and its exit recorded; an
  interrupted child keeps the lease until it is dead, so a race cannot let a new
  start in while the old one still exists.
- Cancellation is recorded distinctly from failure and completion, and partial
  evidence stays available through the existing lifecycle transitions.
- A cancellation that races normal completion yields **one** authoritative result
  (the child's recorded exit), never two lifecycle events.

## 4. Crash recovery

- A lease whose owner process is gone is **stale**, detected by pid liveness on
  the same host plus a heartbeat (bounded interval) for the cross-host case.
- Stale recovery is **explicit**, not automatic: `sop resume` (or an operator
  command) inspects the stale lease, reports the job identity and persisted state,
  and releases it. A start never silently steals a live-looking lease.
- A crashed execution never triggers automatic resubmission; reconciliation
  against persisted state is mandatory before any new start.
- The release is append-only audit evidence (who, when, why, which lease), in the
  same shape as the approval audit.

## 5. Interface changes (all gated)

| Interface | Change | Default |
| --- | --- | --- |
| CLI `sop run` / `sop prompt` | acquire the lease at execution start; report the holder on contention | behind `execution.lease` |
| MCP `sop_start_work` | child acquires the lease; busy result names the holder | behind `execution.lease` |
| ACP `session/prompt` | unchanged call path; maps a lease-busy result to the existing `busy` outcome | behind `execution.lease` |
| `sop status` / `sop resume` | report lease holder and stale leases | always read-only |

Config: `execution.lease: off | advisory | required`, default `off` until the
proposal is approved and the rollout in §7 completes. `advisory` logs contention
without blocking, so evidence can be gathered first.

## 6. Tests

Deterministic, no model, no network:

1. **CLI-first, ACP-second** — a `sop run` holds the lease; an ACP start returns
   a busy result naming the holder, starts no child, and writes no job.
2. **ACP-first, CLI-second** — an ACP-started child holds the lease; `sop run`
   refuses with the holder identity and does not mutate tasks.
3. **ACP vs MCP** — existing `TestACPPromptConcurrentStartsCreateOneWorkItem`
   extended to MCP, still exactly one governed launch.
4. **Cancel/completion race** — cancel during completion produces one exit record
   and one lease release.
5. **Crash/stale lease** — kill the owner; the lease is not stolen automatically;
   `sop resume` reports and releases it; the next start succeeds.
6. **Parent death** — killing the adapter/tunnel parent leaves the lease owned by
   the live child; disconnect does not free it.
7. **Multi-project** — leases are per project; two projects run concurrently.
8. **Ordering** — concurrent plan reconciliation and execution cannot deadlock
   (lease → metadata ordering asserted by a stress test).
9. **Rollback** — with `execution.lease: off`, every existing behaviour and test
   is unchanged.

## 7. Rollout and rollback

1. Land the lease with `off` (no behaviour change).
2. `advisory` in one project; collect contention evidence; confirm no deadlocks.
3. `required` for ACP/MCP only, then for CLI.
4. Rollback is a config change back to `off`; no state migration is required.
   Stale lease files are inert when the feature is off.

## 8. Explicit non-goals

- No waiver of any lifecycle, approval, or verification gate.
- No automatic approval, cancellation, or resubmission.
- No change to global CLI execution semantics until this proposal is approved.
