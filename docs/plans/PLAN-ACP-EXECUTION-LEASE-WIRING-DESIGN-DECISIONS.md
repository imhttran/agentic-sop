# Design decisions — `PLAN-ACP-EXECUTION-LEASE-WIRING.md`

Status: **decision record for operator review. Nothing here is approved.** These
entries resolve the plan's "Known conflict" and "Open questions" so the plan can be
approved or amended with an explicit record instead of being inferred from an
existing SOP run.

Line numbers for `internal/cli/mcp_start_work.go` refer to the working tree that
carries stage-1's uncommitted change (6 lines above `launchWorkProcess`); on
committed `main` subtract 6. The stage-1 files (`internal/executionlease/inherit.go`,
`leaseAcquireOrInherit` in `internal/cli/execution_lease.go`,
`internal/cli/execution_lease_inherit_test.go`) are **uncommitted** and are not part
of `main`.

## D1. Lease ownership and inheritance

**Current behaviour.** The lease is acquired by the process that executes:
`executionlease.Acquire` writes the owner record with `O_EXCL`
(`internal/executionlease/lease.go:168`). `sop run` acquires in-process
(`internal/cli/run.go:199`, `internal/cli/drive.go:83`). ACP/MCP parents acquire
nothing; `startWork` only takes the start lock (`mcp_start_work.go:269,696`). The
children started by `startWork` (`sop prompt …`, `sop run PLAN`) do not acquire the
lease yet. Stage-1 adds `InheritEnv` and `InheritedOwner`
(`internal/executionlease/inherit.go:20,75`) plus `leaseAcquireOrInherit`
(`internal/cli/execution_lease.go:66`), with no caller.

**Proposed resolution.** Child-as-acquirer. The ACP/MCP parent only *checks* the
lease and launches; the child acquires as its first lifecycle act. The inheritance
marker is set only when the parent already holds a lease — which the ACP/MCP flow
never does today — so inheritance remains a valid but unused path for future nested
execution. `startWork` must not set the marker.

**Alternatives and trade-offs.**
- *Parent-as-acquirer* (proposal §1's "parent writes the owner record before
  `cmd.Start()`"): requires an API addition because `Acquire` is the only writer.
  Worse, the record names the parent's pid: if the parent dies while the child runs,
  the lease names a dead pid, `State.Stale` reports it stale, and `sop resume` can
  release a lease whose execution is still alive. Rejected.
- *Re-entrant acquire* (child skips `Acquire` when an env marker is present, no
  on-disk check): two writers, and a forged/stale marker silently permits a second
  owner. Rejected.

**Tests and acceptance criteria.** `TestExecutionLeaseInheritMatchesLiveParent`,
`TestExecutionLeaseInheritRejectsMismatch`,
`TestExecutionLeaseInheritRejectsUnparsableRecord`
(`internal/executionlease/inherit_test.go`, uncommitted) and
`TestExecutionLeaseInheritIsInertUnderOff` (`internal/cli/execution_lease_inherit_test.go`,
uncommitted). Stage 6 adds the end-to-end CLI-vs-ACP case.

**Risks.** A future parent that holds the lease but forgets the marker makes the
child contend and fail closed (busy), which is safe but surprising; document it.

**Recommendation.** Adopt child-as-acquirer and keep the marker for future nesting.

## D2. Acquisition versus inherited ownership

**Current behaviour.** On a matched inheritance, `leaseAcquireOrInherit` returns a
nil `*Held`, and `Held.Release` on nil is a no-op (`lease.go:333-336`). The parent
keeps the file open, heartbeats it (`WatchHeartbeat`, `lease.go:269`), and releases
it.

**Proposed resolution.** The inherited handle is read-only: no heartbeat, no
release, no path-inode check. Document it as an observation of another process's
ownership, not a second lease.

**Alternatives and trade-offs.** Sharing the file descriptor across `exec` is not
possible; re-acquiring in the child creates two writers and a spurious `BusyError`.

**Tests and acceptance criteria.** A test asserting that when the child inherits,
the parent's record is byte-identical before and after the child's run and the file
still exists (extend the stage-1 inherit tests).

**Risks.** If the parent stops heartbeating while still alive (for example a
suspended process) and the lease is cross-host, staleness can be misreported. On the
same host, pid liveness protects it.

**Recommendation.** Keep the inert handle.

## D3. Malformed ownership records

**Current behaviour.** `readStateFile` marks a record `Unparsable` when the file
exceeds 64 KiB or the JSON is invalid or has no `kind` (`lease.go:400-427`). An
unparsable record counts as held for `Grace` (10 s) and is stale after that
(`State.Stale`, `lease.go:435-453`). `Acquire` returns `*BusyError` and never steals
(`lease.go:191`). `sop resume` releases an unparsable stale record with a zero owner
(`internal/cli/resume.go:108-114`, `ReleaseStale` `lease.go:475-528`).

**Proposed resolution.** Keep fail-closed. A start that finds any record under
`required` returns the existing `busy` outcome with the "lease record is being
written" wording; it does not attempt to parse or repair.

**Alternatives and trade-offs.** Treating an unparsable record as absent is unsafe:
a crash between `O_EXCL` create and the record write would admit a second owner in
exactly the window the lease exists to close.

**Tests and acceptance criteria.** `TestUnparsableLeaseIsHeldThenStale` and
`TestReleaseStaleReleasesAnUnparsableLease` exist; add the `startWork`-level mapping
test in stage 3.

**Risks.** A corrupt file blocks every start until `Grace` elapses and an operator
runs `sop resume`. That is the intended cost of failing closed.

**Recommendation.** Keep fail-closed.

## D4. Fail-closed behaviour

**Current behaviour.** `required` refuses: `executionLease` returns the
`*BusyError` and both run paths exit non-zero (`internal/cli/execution_lease.go:45-51`,
`run.go:200-203`, `drive.go:84-87`). `advisory` writes the contention to stderr and
proceeds. `off` touches nothing.

**Proposed resolution.** Same policy at every entry point. Under `required`, an
ACP/MCP start returns `workOutcomeBusy` naming the holder via `Owner.Describe`,
creates no job directory, and launches no child. A child that loses the race exits
non-zero and the parent records a distinct lease-busy failure (stage 5), so the loss
is observable rather than silent. Under `advisory`, contending starts proceed and
the contention is recorded.

**Alternatives and trade-offs.** Advisory-only rollout is the proposal's step 7.2
and remains available; never-fail is rejected because it defeats the requirement.

**Tests and acceptance criteria.** Stages 3, 5, 6, 7.

**Risks.** A stale lease blocks every start until `sop resume`, including CI and
editor flows. Document the operator path prominently.

**Recommendation.** As proposed.

## D5. Lease release and process termination

**Current behaviour.** Both run paths `defer lease.Release()`
(`run.go:204`, `drive.go:88`). `Release` is idempotent and removes the file only if
it still names the inode the execution created (`lease.go:333-373`). There are no
signal handlers: `SIGTERM`/`SIGKILL` skips defers, leaving a stale record that pid
liveness detects.

**Proposed resolution.** Keep deferred release as the only automatic release.
Document that a killed process leaves a stale lease that `sop resume` reports and
releases. Do not add a signal handler in the first implementation.

**Alternatives and trade-offs.** A `SIGTERM` handler that releases on the way out is
only safe after the child/agent is reaped; releasing earlier lets a second execution
start while the first still writes. The cancellation path in stage 9 must release
only after the exit is recorded.

**Tests and acceptance criteria.** `TestRunReleasesTheLeaseOnExit` exists; stage 9
covers the cancel/completion race; stage 11 covers parent death.

**Risks.** An operator who kills a run and immediately starts another is refused
until `sop resume`. That is the proposal's explicit choice (§4).

**Recommendation.** No signal-handler release initially.

## D6. Crash recovery and stale ownership

**Current behaviour.** `State.Stale`: a same-host record with a probeable pid is
stale only when the pid is dead; otherwise staleness is heartbeat age against
`HeartbeatLimit` (5 min); an unparsable record is stale after `Grace` (10 s)
(`lease.go:435-453`). Release is explicit only, through `sop resume`
(`resume.go:88-124`), and re-checks owner identity and inode before removing
(`ReleaseStale`, `lease.go:475-528`). No start steals a stale lease.

**Proposed resolution.** Keep explicit recovery. Starts may surface the stale hint
(`BusyError.Stale`) but never release. `state.exec.lock` is inert under `off`.

**Alternatives and trade-offs.** Automatic release is rejected by proposal §4.
Cross-host recovery is the weak case: a live remote run whose heartbeat stalls for
over five minutes looks stale, and `sop resume` would release it. A
`--require-local-host` or confirmation flag is the candidate mitigation and is not
implemented.

**Tests and acceptance criteria.** Proposal test 5 exists
(`TestRunResumeReleasesStaleExecutionLease`,
`TestRunResumeLeavesLiveExecutionLease`); add a test that a stale lease still
refuses a start until `sop resume` runs.

**Risks.** Two executions after a cross-host release. Decide whether shared/NFS
checkouts are in scope; if they are, require an operator flag.

**Recommendation.** Keep explicit recovery; record the cross-host limitation as a
known bound rather than an unqualified exclusion guarantee.

## D7. Cross-entrypoint execution exclusion

**Current behaviour.** Only starts are serialized. `sop run` does not read the
started-work records; `sop prompt`, MCP and ACP do not acquire or check the lease
(see `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`). `midRunTasks` sees a run only when
a task row is parked in one of eight statuses, which the local lifecycle does not
reach while executing a fresh task (`scheduler.go:181-198`,
`drive.go:1236-1261`).

**Proposed resolution.** Lease at every execution entry point and a lease check at
every start, in the fixed order lease → metadata lock. `sop prompt` acquires before
any `planflow`/approval write. ACP/MCP check under the start lock, launch, and let
the child acquire. The handshake window is closed by either a bounded wait
(proposed `workLeaseHandshake` ≈ 2 s) or an explicit `lease_pending` flag (stage 4).

**Alternatives and trade-offs.** Extending `midRunTasks` to "READY is mid-run" would
block every legitimate start on a selected task and still miss a run inside model
calls. Holding `projectlock` for the execution's lifetime would serialize metadata
and risk deadlock with `planflow`.

**Tests and acceptance criteria.** Stages 6, 7, 8 and 11 (proposal tests 1, 2, 3 and
6), plus state/artifact assertions (task rows byte-identical, no run directory).

**Risks.** A start can still report `started` before the child owns the lease; the
flag or handshake makes that visible but does not make it exclusive. Do not describe
the lease as protecting resources outside the enforcing processes.

**Recommendation.** Lease-first ordering; bounded handshake with an explicit
`lease_pending` fallback.

## D8. Interaction with the start lock

**Current behaviour.** `acquireWorkLock` creates `.start.lock` with `O_EXCL`, breaks
it after two minutes, and `startWork` releases it on return
(`mcp_start_work.go:696-715`, defer at `:340`). It serializes the decision to
launch across ACP and MCP.

**Proposed resolution.** Keep both locks and their distinct roles: the start lock
makes the decision atomic; the lease makes the execution exclusive. The lease check
happens inside the start lock. The start lock is **not** held while waiting for the
child's lease, because that would serialize unrelated job bookkeeping and can
exceed the two-minute stale threshold under load.

**Alternatives and trade-offs.** Holding the start lock until the child's lease
appears gives a stronger handshake but lengthens the critical section and risks the
stale-break path silently admitting a second start.

**Tests and acceptance criteria.** ACP-vs-ACP and ACP-vs-MCP start tests exist; add a
test that a start refused by a held lease still releases the start lock (stage 3).

**Risks.** The window between launch and child acquisition remains. It is closed by
the handshake or reported by `lease_pending`; it is not hidden.

**Recommendation.** Keep both, documented.

## D9. Disabled-mode compatibility

**Current behaviour.** `off` returns `(nil, nil)` before touching the lease file
(`internal/cli/execution_lease.go:26-28`); `ParseMode("")` is `off`
(`lease.go:65-73`); no other production code reads `state.exec.lock` except
`sop resume`, which is a reporting path. `TestExecutionLeaseOffByDefault` asserts no
file is created.

**Proposed resolution.** Every new check short-circuits on `off` before any file
read; the inheritance marker is not set; a leftover `state.exec.lock` is inert.

**Alternatives and trade-offs.** Reporting a held lease under `off` would add I/O
and a new diagnostic for no behavioural benefit; the proposal keeps `off` a strict
no-op.

**Tests and acceptance criteria.** `TestExecutionLeaseOffByDefault` exists; stage 14
extends it into a table across prompt, run, startWork and ACP with a pre-created
lease file.

**Risks.** A future call site that reads the lease before checking the mode would
regress `off`; the stage-14 table is the guard.

**Recommendation.** As proposed.
