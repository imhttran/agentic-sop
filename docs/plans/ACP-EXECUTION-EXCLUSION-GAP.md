# ACP cross-interface execution exclusion — unresolved acceptance gap

Status: **OPEN — unresolved acceptance gap.** This document records a requirement
that the current change set does **not** satisfy. It is not evidence of
completion, and it must not be read as one.

Partially mitigated: the execution lease is wired for `sop run` (single-task and
graph mode) behind `execution.lease` (default off). It is not wired for
`sop prompt`, MCP or ACP. Do not declare release acceptance on the ACP start-lock
test alone.

Plan: [`PLAN-SOP-ACP-ZED.md`](PLAN-SOP-ACP-ZED.md), stage ACP-003 and the
acceptance-matrix row _“Concurrent starts — existing project lock protects CLI,
MCP, and ACP equally.”_

## Requirement

The plan requires that **one active execution per project** holds across every
interface:

> One active execution per project, including executions started through CLI or
> MCP. … Competing CLI/MCP/ACP starts obey the same lock.

## What is proven

ACP and MCP **starts** share one start lock, and concurrent ACP starts are covered
by a deterministic test:

- `startWork` takes a cross-process start lock, `.agent-sdlc/runs/mcp-work/.start.lock`,
  created with `O_EXCL` (`internal/cli/mcp_start_work.go`, `acquireWorkLock`). It
  is released when `startWork` returns, after the child is launched.
- While holding it, `startWork` re-reads the persisted started-work records and
  refuses a duplicate fingerprint (`already_running` / `already_ran`) or a
  competing running job (`busy`).
- It also refuses when a `sop run` is mid-task, detected from the persisted task
  states (`midRunTasks`).
- `TestACPPromptConcurrentStartsCreateOneWorkItem`
  (`internal/cli/acp_test.go`) races two ACP submissions through the bridge's
  `Starter` and the real start lock and asserts exactly one governed launch.

This proves **ACP-vs-ACP** start exclusion only. MCP `sop_start_work` and ACP
`session/prompt` share the same `startWork` start lock by inspection, but no test
races an MCP start against an ACP start.

## What is not proven

The project metadata lock (`.agent-sdlc/state.lock`, `internal/projectlock`) is a
**plan-metadata and approval lock**, not an execution lock:

| Path                                       | Project metadata lock                    | Execution/start lock                                                                                                                      | Notes                                                                                     |
| ------------------------------------------ | ---------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------- |
| ACP `session/prompt`                       | probe only, released immediately         | via `startWork`                                                                                                                           | `acpSubmission` in `internal/cli/acp.go`; the metadata lock is tried and released at once |
| MCP `sop_start_work`                       | none                                     | via `startWork`                                                                                                                           |                                                                                           |
| CLI `sop run` (single-task and graph mode) | only inside `planflow.Prepare`/reconcile | execution lease, when `execution.lease` is not off (`executionLease` at `run.go:199` and `drive.go:83`); does not check started-work jobs | graph mode is `runGraph`/`driveGraph` inside `sop run`                                    |
| CLI `sop prompt`                           | none                                     | **none**                                                                                                                                  |                                                                                           |
| Child `sop prompt` (started by ACP/MCP)    | none                                     | released at launch                                                                                                                        | the start lock is released once the child is launched                                     |

Consequences:

1. **CLI-first, ACP-second** — a `sop run` takes no start lock, and the local
   lifecycle persists only `READY` (at selection) and the terminal status (at
   completion). `midRunTasks` sees a run only when a task row is already parked in
   one of its eight mid-run statuses (for example an interrupted task or one
   resumed from such a status). For a fresh task it never sees the run, so an ACP
   start can begin a second execution at any point during task execution.
2. **ACP-first, CLI-second** — a `sop run` takes no execution lock and does not
   consult the started-work records, so it can begin while an ACP/MCP-started
   `sop prompt` child is still running.
3. The ACP bridge's project-lock probe cannot close either gap: it is released
   before the start, and the project lock is not held for the lifetime of any
   execution.

Neither ordering has a deterministic test. Both follow from the code paths in the
table above and correspond to proposal tests 1 and 2.

What a double execution damages today (from a code reading, not reproduced):

- **Task rows** in `.agent-sdlc/state.db`: each run loads the whole graph and
  saves full rows, so the last writer wins and an attempt counter or a `BLOCKED`
  status can be lost.
- **Run artifacts** in `.agent-sdlc/runs/<run-id>/`: the run id is the task id, so
  two runs of one task share a directory, and `runpkg.New` deletes `attempts/`.
  One run erases the other's evidence.
- **Plan metadata**: a second `sop run` can archive the plan and clear tasks while
  the first run is mid-task, leaving orphan task rows.
- **The working tree**: both agents edit the same checkout, so validation sees the
  other run's changes and evidence is attributed to the wrong run.
- **Not affected**: commits (a run does not commit) and worktrees (`AddWorktree`
  has no caller in the run path).

The ACP concurrency test therefore does **not** establish the cross-interface
exclusion requirement. That requirement remains **UNRESOLVED**, and overall
release acceptance must not be declared on the strength of that test.

## Baseline behavior before ACP

Status: **historical, not re-verified.** The code review that produced this record
checked the current code paths only; it did not reconstruct the pre-ACP design or
the bridge's earlier locking, so the claims below are recorded as context, not as
confirmed facts.

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

| Interface                              | Acquires lease                                                                                               | Checks for a held lease                                                    | Limits                                                                                                                                                                                                                                                                                                  |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `sop run` (single-task and graph mode) | when `execution.lease` is `required`; under `advisory` a busy lease is logged to stderr and the run proceeds | n/a                                                                        | heartbeat refreshes every `HeartbeatEvery` (1 minute; `HeartbeatLimit` is 5 minutes), so a healthy run longer than the limit is not mistaken for stale where pid liveness cannot be checked (Windows) or the holder is on another host; a heartbeat failure is reported on stderr and the run continues |
| `sop prompt`                           | no                                                                                                           | no                                                                         |                                                                                                                                                                                                                                                                                                         |
| MCP `sop_start_work`                   | no                                                                                                           | no                                                                         | start lock only; a start with `plan_path` launches `sop run`, whose child takes the lease when the mode is not `off`, after `startWork` has already answered `started`                                                                                                                                  |
| ACP `session/prompt`                   | no                                                                                                           | no                                                                         | start lock only                                                                                                                                                                                                                                                                                         |
| `sop resume`                           | no                                                                                                           | reports a held lease; releases a stale one in every mode, never a live one | crash/stale case: kill the owner, the lease is not stolen automatically, `sop resume` reports and releases it, the next start succeeds                                                                                                                                                                  |

The lease file is `.agent-sdlc/state.exec.lock`. An ACP/MCP **prompt** start cannot
see a running `sop run`, and a `sop run` cannot see an ACP-started prompt child. (An
MCP start with `plan_path` spawns a `sop run`, which does acquire the lease when the
mode is not `off` — see the table row above.)

Closed when proposal section 6 tests 1 and 2 pass with `execution.lease` set to
`required` across `sop run`, `sop prompt`, MCP and ACP, and test 5 (crash/stale)
passes. Test 5 passes; tests 1 and 2 remain open.

Status of the proposal's section 6 tests, from a code reading:

| Test                               | Status                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| ---------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 5 crash/stale lease                | **satisfied:** `TestRunResumeReleasesStaleExecutionLease`, `TestRunResumeLeavesLiveExecutionLease` (`internal/cli/resume_lease_test.go`) pass unchanged; recorded output below                                                                                                                                                                                                                                                                  |
| 9 rollback (`off` changes nothing) | unit level only: `TestExecutionLeaseOffByDefault`                                                                                                                                                                                                                                                                                                                                                                                               |
| 3 ACP vs MCP                       | start-lock coverage: `TestACPPromptAndMCPStartWorkRaceCreateOneWorkItem`, `TestACPPromptAndMCPStartWorkRaceDifferentWorkStillOneLaunch` and `TestMCPAndACPStartsExcludeEachOtherInSequence` (`internal/cli/start_exclusion_test.go`) race and sequence one ACP and one MCP start and assert exactly one launch; `TestACPPromptConcurrentStartsCreateOneWorkItem` remains the ACP-vs-ACP base. This is start-lock exclusion, not lease exclusion |
| 1, 2, 4, 6, 7, 8                   | none; `TestRunRefusesWhileAnotherExecutionHoldsTheProject` takes the lease directly in the test, not through an ACP-started child, so it is not test 2                                                                                                                                                                                                                                                                                          |

## Proposal test 5 (crash/stale lease) — satisfied

Proposal test 5 is satisfied by two existing tests in
`internal/cli/resume_lease_test.go`. No test was added or changed. Both tests
ran unchanged and passed:

- `TestRunResumeReleasesStaleExecutionLease` — a stale (dead pid 999999) lease is
  reported as stale on stderr and released by `sop resume`, and the next
  `executionlease.Acquire` succeeds.
- `TestRunResumeLeavesLiveExecutionLease` — a live (own pid) lease is reported as
  `held by` on stderr and not released; a later `executionlease.Acquire` returns
  `BusyError`.

Recorded output (`go test ./internal/cli/ -run <test> -v`, verbatim):

```
=== RUN   TestRunResumeReleasesStaleExecutionLease
--- PASS: TestRunResumeReleasesStaleExecutionLease (0.02s)
PASS
ok  	github.com/imhttran/agentic-sop/internal/cli	0.249s
```

```
=== RUN   TestRunResumeLeavesLiveExecutionLease
--- PASS: TestRunResumeLeavesLiveExecutionLease (0.01s)
PASS
ok  	github.com/imhttran/agentic-sop/internal/cli	0.248s
```

`go build ./...` and `go vet ./...` pass. The full suite was recorded as passing by
the run that produced this section; the focused tests named above and in
“Proposal test 3” were re-run after `internal/cli/start_exclusion_test.go` was added.

## Proposal test 3 (ACP vs MCP) — start-lock coverage

`internal/cli/start_exclusion_test.go` covers the start-lock half of proposal
test 3. Both racers are the production entry points: `acpSubmission` (what
`sop acp` installs) and `mcpStartWorkTool` (what `sop mcp` exposes), and both call
`startWork`, so the `O_EXCL` start lock and the under-lock re-read of the job
records are real. Only the launcher is faked.

- `TestACPPromptAndMCPStartWorkRaceCreateOneWorkItem` — two racers released
  together for the same work; exactly one launch and exactly one `started`; the other
  receives a start-lock refusal or a non-starting disposition.
- `TestACPPromptAndMCPStartWorkRaceDifferentWorkStillOneLaunch` — the same race with
  different work; one launch, the loser refused as `busy`.
- `TestMCPAndACPStartsExcludeEachOtherInSequence` — both orderings without racing:
  MCP starts and the ACP start is refused as `already_running`, then the reverse.

Scope limit: this proves that concurrent ACP and MCP _starts_ create one work item.
It does not establish execution exclusion, because neither interface acquires or
checks the execution lease (see the interface table). Proposal tests 1, 2, 4 and 6
remain the cross-entrypoint acceptance criteria.

## Evidence to cite

- `internal/cli/mcp_start_work.go` — `acquireWorkLock`, `startWork`,
  `listWorkJobs`, `midRunTasks`.
- `internal/cli/acp.go` — `acpSubmission` (`LockTaker`, `StateReader`, `Starter`).
- `internal/projectlock/lock.go` — the project metadata lock.
- `internal/executionlease/lease.go` — the lease (`state.exec.lock`).
- `internal/cli/execution_lease.go`, `internal/cli/run.go:199`,
  `internal/cli/drive.go:83` (`runGraph`) — lease acquisition.
- `internal/cli/resume.go` — report and release of a stale lease.
- `TestACPPromptConcurrentStartsCreateOneWorkItem` — the ACP-vs-ACP start
  exclusion that is covered.
