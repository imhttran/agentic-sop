# ACP cross-interface execution exclusion — unresolved acceptance gap

Status: **OPEN — unresolved acceptance gap.** This document records a requirement
that the current change set does **not** satisfy. It is not evidence of
completion, and it must not be read as one.

Partially mitigated: the execution lease is wired for `sop run` and `sop drive`
behind `execution.lease` (default off). It is not wired for `sop prompt`, MCP or
ACP. Do not declare release acceptance on the ACP/MCP start-lock test alone.

Plan: [`PLAN-SOP-ACP-ZED.md`](PLAN-SOP-ACP-ZED.md), stage ACP-003 and the
acceptance-matrix row *“Concurrent starts — existing project lock protects CLI,
MCP, and ACP equally.”*

## Requirement

The plan requires that **one active execution per project** holds across every
interface:

> One active execution per project, including executions started through CLI or
> MCP. … Competing CLI/MCP/ACP starts obey the same lock.

## What is proven

ACP and MCP **starts** are mutually exclusive, and this is covered by a
deterministic test:

- `startWork` takes a cross-process start lock, `.agent-sdlc/runs/work/.start.lock`,
  created with `O_EXCL` (`internal/cli/mcp_start_work.go`, `acquireWorkLock`).
- While holding it, `startWork` re-reads the persisted started-work records and
  refuses a duplicate fingerprint (`already_running` / `already_ran`) or a
  competing running job (`busy`).
- It also refuses when a `sop run` is mid-task, detected from the persisted task
  states (`midRunTasks`).
- `TestACPPromptConcurrentStartsCreateOneWorkItem`
  (`internal/cli/acp_test.go`) races two submissions through the real bridge and
  the real start lock and asserts exactly one governed launch.

This proves **ACP-vs-MCP** and **ACP-vs-ACP** start exclusion only.

## What is not proven

The project metadata lock (`.agent-sdlc/state.lock`, `internal/projectlock`) is a
**plan-metadata and approval lock**, not an execution lock:

| Path | Project metadata lock | Execution/start lock | Notes |
| --- | --- | --- | --- |
| ACP `session/prompt` | probe only, released immediately | via `startWork` | `acpSubmission` in `internal/cli/acp.go`; the metadata lock is tried and released at once |
| MCP `sop_start_work` | none | via `startWork` | |
| CLI `sop run` | only inside `planflow.Prepare`/reconcile | execution lease, when `execution.lease` is not off; does not check started-work jobs | |
| CLI `sop drive` | not examined | execution lease, same helper as `sop run` | |
| CLI `sop prompt` | none | **none** | |
| Child `sop prompt` (started by ACP/MCP) | none | released at launch | the start lock is released once the child is launched |

Consequences:

1. **CLI-first, ACP-second** — a `sop run` that is already executing does not
   hold a start lock, so an ACP start is excluded only if it happens to observe a
   mid-run task state. Between task transitions it can start a second execution.
2. **ACP-first, CLI-second** — a `sop run` takes no execution lock and does not
   consult the started-work records, so it can begin while an ACP/MCP-started
   `sop prompt` child is still running.
3. The ACP bridge's project-lock probe cannot close either gap: it is released
   before the start, and the project lock is not held for the lifetime of any
   execution.

Neither ordering has a deterministic test. Both follow from the code paths in the
table above and correspond to proposal tests 1 and 2.

The ACP/MCP concurrency test therefore does **not** establish the cross-interface
exclusion requirement. That requirement remains **UNRESOLVED**, and overall
release acceptance must not be declared on the strength of that test.

## Baseline behavior before ACP

Before the ACP bridge existed, CLI `sop run` and CLI `sop prompt` had the same
properties. The ACP bridge's earlier design held the project metadata lock across
its state read and start. The earlier design held the metadata lock only until the
child launched, so it did not exclude a running `sop run` either. The probe-only
form does not remove an exclusion that existed.

## What would close it

A single project-scoped **execution lease** held for the lifetime of an execution
and respected by every interface. See
[`PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`](PROPOSAL-SHARED-EXECUTION-EXCLUSION.md).
Non-goals: see the proposal, section 8.

The lease primitive exists in `internal/executionlease`: an exclusive owner
record, a heartbeat for the cross-host case, and staleness that is reported rather
than acted on. The `execution.lease` setting (`off`, `advisory`, `required`)
defaults to `off`.

| Interface | Acquires lease | Checks for a held lease | Limits |
| --- | --- | --- | --- |
| `sop run` | when `execution.lease` is `required`; contention recorded under `advisory` | n/a | heartbeat refreshes about every `HeartbeatLimit/5`, so a healthy run longer than the limit is not mistaken for stale where pid liveness cannot be checked (Windows) or the holder is on another host; a heartbeat failure is reported on stderr and the run continues |
| `sop drive` | same helper as `sop run` | n/a | same as `sop run` |
| `sop prompt` | no | no | |
| MCP `sop_start_work` | no | no | start lock only |
| ACP `session/prompt` | no | no | start lock only |
| `sop resume` | no | reports a held lease; releases a stale one in every mode, never a live one | crash/stale case: kill the owner, the lease is not stolen automatically, `sop resume` reports and releases it, the next start succeeds |

The lease file is `.agent-sdlc/state.exec.lock`. An ACP/MCP start cannot see a
running `sop run`, and a `sop run` cannot see an ACP-started child.

Closed when proposal section 6 tests 1 and 2 pass with `execution.lease` set to
`required` across `sop run`, `sop prompt`, MCP and ACP, and test 5 (crash/stale)
passes. This document does not record the status of the proposal's tests.

## Evidence to cite

- `internal/cli/mcp_start_work.go` — `acquireWorkLock`, `startWork`,
  `listWorkJobs`, `midRunTasks`.
- `internal/cli/acp.go` — `acpSubmission` (`LockTaker`, `StateReader`, `Starter`).
- `internal/projectlock/lock.go` — the project metadata lock.
- `internal/executionlease/lease.go` — the lease (`state.exec.lock`).
- `internal/cli/execution_lease.go`, `internal/cli/run.go`,
  `internal/cli/drive.go` — lease acquisition.
- `internal/cli/resume.go` — report and release of a stale lease.
- `TestACPPromptConcurrentStartsCreateOneWorkItem` — the ACP/MCP start exclusion
  that is covered.
