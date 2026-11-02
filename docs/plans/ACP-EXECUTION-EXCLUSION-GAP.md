# ACP cross-interface execution exclusion — unresolved acceptance gap

Status: **OPEN — unresolved acceptance gap.** This document records a requirement
that the current change set does **not** satisfy. It is not evidence of
completion, and it must not be read as one.

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

- `startWork` takes a cross-process start lock, `.agent-sdlc/runs/work/.lock`,
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

The project writer lock (`.agent-sdlc/state.lock`, `internal/projectlock`) is a
**plan-metadata and approval lock**, not an execution lock:

| Path | Project metadata lock | Execution/start lock | Notes |
| --- | --- | --- | --- |
| ACP `session/prompt` | probe only, released immediately | via `startWork` | see `internal/cli/acp.go` |
| MCP `sop_start_work` | none | via `startWork` | |
| CLI `sop run` | only inside `planflow.Prepare`/reconcile | **none** | does not check started-work jobs |
| CLI `sop prompt` | none | **none** | |
| Child `sop prompt` (started by ACP/MCP) | none | none after start | the start lock is released once the child is launched |

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

The ACP/MCP concurrency test therefore does **not** establish the cross-interface
exclusion requirement. That requirement remains **UNRESOLVED**, and overall
release acceptance must not be declared on the strength of that test.

## Why this is not a regression from the ACP work

Before the ACP bridge existed, CLI `sop run` and CLI `sop prompt` had the same
properties. The ACP bridge's earlier design held the project metadata lock across
its state read and start; that lock is released immediately after the child is
launched, so it never excluded a concurrently executing CLI run either. The
probe-only form does not remove an exclusion that existed.

## What would close it

A single project-scoped **execution lease** held for the lifetime of an execution
and respected by every interface. See
[`PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`](PROPOSAL-SHARED-EXECUTION-EXCLUSION.md).
Until that proposal is approved and implemented, this gap stays open.

## Evidence to cite

- `internal/cli/mcp_start_work.go` — `acquireWorkLock`, `startWork`,
  `listWorkJobs`, `midRunTasks`.
- `internal/cli/acp.go` — `acpSubmission` (`LockTaker`, `StateReader`, `Starter`).
- `internal/projectlock/lock.go` — the project metadata lock.
- `TestACPPromptConcurrentStartsCreateOneWorkItem` — the ACP/MCP start exclusion
  that *is* covered.
