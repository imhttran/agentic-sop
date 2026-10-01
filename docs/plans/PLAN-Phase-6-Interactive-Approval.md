# PLAN — Phase 6: Interactive Approval Workflow

**Type:** Implementation plan (normative for the work it describes; the specifications
win wherever they disagree).

**Status:** Proposed — not implemented. Phase 6 is a CLI and workflow slice over the
human approval boundary that already exists: it adds no approval state, no second state
machine, and no path that bypasses the gate. Its work items are tracked as `P6-NNN`.
Running `sop run docs/plans/PLAN-Phase-6-Interactive-Approval.md` starts the stages at
`PLANNED`; none declares `execution_mode: done` until its work exists in the tree. The
rules this plan MUST NOT break are normative in
[../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md); the rules it adds are declared
there by P6-005. See [../reference/CLI.md](../reference/CLI.md).

## Objective

Make SOP's human approval gate operable and discoverable end to end at a terminal,
without moving any authority. Today the mechanism works but a human cannot see it:
a task parks at the gate, SOP records an explicit, resolvable request, and a client can
read it (`sop approval`) or resolve it (`sop approve` / `sop decline`, or the MCP tools
`sop_approval` / `sop_approve` / `sop_decline`) — but there is **no way to see which
tasks are waiting**, a parked run does not name the command that resolves it, there is
no interactive path, and the three commands are absent from
[../reference/CLI.md](../reference/CLI.md).

Phase 6 closes those gaps and nothing else. SOP still records the gate, decides whether
it is applicable, and owns the lifecycle response; an approval still resolves only the
specific gate that raised it and still cannot manufacture completion.

## Design

```text
              sop run / sop prompt --capability implement     (UNCHANGED: stops at the gate)
                                  │
                    parks the task, records SOP's request
                                  ↓
        .agent-sdlc/runs/<task-id>/     ← the approval request head + append-only history
                                  │
        ┌─────────────────────────┼──────────────────────────┐
        │                         │                          │
  sop approvals            sop approval <id>        sop approve|decline <id>
  (enumerate; new)          (read one)              (record a decision, --by/--note)
        └─────────────────────────┴──────────────────────────┘
                                  ↓
                  approval.Service   (the single application boundary)
                                  ↓
        domain.RequeueWithoutSpending  →  the task is runnable again
                                  ↓
                 sop run   (a NEW, ordinary run; `--run` starts it explicitly)
```

The CLI and MCP stay thin clients of `approval.Service`. Nothing in this phase selects a
model, routes, validates, reviews, or transitions state except through the domain
operations that already exist. `sop run` continues to stop at the gate
([HUMAN-APPROVAL §3](../specs/HUMAN-APPROVAL.md)); the interactive work lives in the
approval commands, not in the run.

## Tasks

### P6-001 — Enumerate pending approval gates

- **Status:** Not started.
- **Scope:** add a read-only `sop approvals` that lists every task with an **applicable**
  approval request (present and `PENDING`), rendering the task id, kind, target, stage,
  and reason, and printing `no pending approvals` when there are none. It reads
  `store.List()` and asks `approval.Service.Approval` for each task, so it classifies
  nothing itself and never mutates. `--json` emits a machine-readable list with the same
  fields the single-task view projects. Add the MCP tool `sop_pending_approvals` for
  parity with the existing approval tools.
- **Files:** `internal/cli/approval.go`, `internal/cli/cli.go`, `internal/cli/mcp.go`,
  `internal/cli/approval_test.go`.
- **Depends on:** —
- **Acceptance:** `sop approvals` on a project with no gates prints the empty message and
  exits 0; with two parked tasks and one resolved request it lists exactly the two
  pending ones; a task whose request is stale (task already satisfied) is not listed as
  applicable; nothing is written.

### P6-002 — A parked run names its own gate

- **Status:** Not started.
- **Scope:** when `sop run` (graph or `--task`) or `sop prompt --capability implement`
  parks a task at a human boundary, the run MUST print the task id and the exact command
  that resolves it (`sop approval <id>`, `sop approve <id>`), and the end-of-run summary
  MUST list the gates it left behind with a pointer to `sop approvals`. This is
  presentation only: the run still stops, still does not commit, push, or merge, and the
  gate is still driven by structured state.
- **Files:** `internal/cli/drive.go`, `internal/cli/run.go`, `internal/cli/prompt.go`,
  `internal/cli/approval.go`.
- **Depends on:** P6-001
- **Acceptance:** a run that parks a task prints the resolving command for that task id;
  a run that leaves two gates lists both; a run with no gate prints no approval section;
  a test asserts the text is derived from the recorded request, not from task status or
  prose.

### P6-003 — Interactive selection at a terminal

- **Status:** Not started.
- **Scope:** `sop approve` and `sop decline` accept no `<task-id>` (and a `--select`) to
  choose among the applicable gates interactively at a TTY, then capture `--by` and
  `--note` as today. Interactivity MUST fail closed: when stdin is not a terminal, the
  command MUST print usage and the enumerated gates and exit non-zero rather than read a
  guess. No prompt text is ever interpreted as a command, a capability, or a lifecycle
  transition, and the decision still goes through `approval.Service` unchanged.
- **Files:** `internal/cli/approval.go`, `internal/cli/approval_test.go`.
- **Depends on:** P6-001
- **Acceptance:** with one applicable gate and a `y` on stdin the decision is recorded;
  with a non-TTY stdin the command fails closed and records nothing; a selection naming a
  non-applicable task is refused with the boundary's own error.

### P6-004 — Record the decision, then continue

- **Status:** Not started.
- **Scope:** add an explicit `--run` to `sop approve <id>` (and `sop approvals --run`
  after an interactive approval) that, **only after the decision is recorded**, starts the
  ordinary run for the requeued task. The decision is persisted first, so a failure to
  start the run leaves the approval recorded and the task runnable; `--run` never skips a
  gate, never commits, pushes, or merges, and never re-implements an approval the human
  did not make. Without `--run` the command's behaviour is unchanged.
- **Files:** `internal/cli/approval.go`, `internal/cli/drive.go`.
- **Depends on:** P6-003
- **Acceptance:** approving with `--run` leaves the approval recorded and starts the same
  run path `sop run` uses; a decision that is refused (stale, already resolved the other
  way) starts no run; the run that follows still stops at its own next gate.

### P6-005 — Declare the approval surface in the specification

- **Status:** Not started.
- **Scope:** extend [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) with the
  normative rules for the surface this phase adds: enumeration MUST list only applicable
  gates and MUST NOT create or resolve anything; a caller MUST NOT manufacture approval,
  and an interactive caller MUST fail closed when it cannot ask a human; `--run` MUST
  record the decision before starting the run, and starting a run MUST NOT bypass a gate;
  the CLI reference and the guide are descriptive, not normative. Link the new rules to
  the existing sections instead of restating them.
- **Files:** `docs/specs/HUMAN-APPROVAL.md`.
- **Depends on:** P6-001, P6-003, P6-004
- **Acceptance:** every rule this phase implements has one normative home in the
  specification; no rule is duplicated from §1–§5; the document still reads as the single
  owner of the human gate.

### P6-006 — Document the approval commands

- **Status:** Not started.
- **Scope:** add `sop approval`, `sop approve`, `sop decline`, and `sop approvals` to the
  command table in [../reference/CLI.md](../reference/CLI.md), and correct the same
  table's stale `sop mcp` row (it now also exposes the approval tools). Add a
  task-oriented guide, `docs/guides/APPROVALS.md`, that walks a human through seeing a
  gate, reading it, deciding it, and continuing — and states plainly that the run stops
  at the gate and that an approval is not a completion. Index the guide.
- **Files:** `docs/reference/CLI.md`, `docs/guides/APPROVALS.md`, `docs/README.md`.
- **Depends on:** P6-001, P6-003, P6-004, P6-005
- **Acceptance:** every approval command the CLI dispatches appears in the reference with
  its flags; the guide's commands run as written on a project with a parked task; the
  documentation link check passes.

### P6-007 — Render the command listing correctly

- **Status:** Not started.
- **Scope:** `sop help` renders `reconcile`, `providers`, and `eval` with a literal tab
  and misaligned columns (they were never re-indented when the approval commands were
  inserted above them). Correct the listing so every command shares one column layout,
  and add `approvals` alongside the other approval commands.
- **Files:** `internal/cli/cli.go`, `internal/cli/cli_test.go`.
- **Depends on:** P6-001
- **Acceptance:** no line of `sop help` begins with a tab; the command names and their
  descriptions align; a test asserts the listing contains no tab and that every dispatched
  command is listed.

### P6-008 — Deterministic tests for the approval surface

- **Status:** Not started.
- **Scope:** cover the surface with no terminal and no model: enumeration over an empty,
  a pending, a resolved, and a stale request; fail-closed non-TTY selection; a selected
  gate that is not applicable; `--run` recording before continuing and starting nothing
  when the decision is refused; approve/decline idempotency; a decline leaving the task
  truthful; and the guarantee that a task status or free-form prose alone produces no
  applicable gate. Reuse the existing approval fixtures rather than inventing new ones.
- **Files:** `internal/cli/approval_test.go`, `internal/approval/approval_test.go`,
  `internal/cli/mcp_test.go`.
- **Depends on:** P6-001, P6-002, P6-003, P6-004
- **Acceptance:** the cases above pass deterministically; a test proves interactivity is
  never exercised from a non-TTY; no test requires a live provider.

### P6-009 — Wire the phase and validate

- **Status:** Not started.
- **Scope:** index this plan in [../README.md](../README.md) and record it in
  [BACKLOG.md](BACKLOG.md) as started (moving the candidate out of the unplanned list),
  add the plan to the compiler contract in `internal/planner/taskshape_test.go`, and run
  the full validation for the phase.
- **Files:** `docs/README.md`, `docs/plans/BACKLOG.md`,
  `internal/planner/taskshape_test.go`.
- **Depends on:** P6-001, P6-002, P6-003, P6-004, P6-005, P6-006, P6-007, P6-008
- **Acceptance:** the plan compiles deterministically with the declared stage count;
  `gofmt -l .` is empty and `go vet`, `go build`, `go test`, and `go test -race` pass; the
  documentation link check reports no broken links; no SOP policy or execution behaviour
  outside the approval surface changed.

## Definition of Done

- A human can see every task waiting at a gate with one command, read one gate, decide
  it, and continue — without knowing any internal file path.
- A run that parks a task names the gate and the command that resolves it, and still
  stops there.
- Interactivity fails closed off a TTY and never reads a decision a human did not make.
- An approval resolves only the gate that raised it; it cannot manufacture completion and
  cannot bypass validation, review, JEV, or the quality gate.
- The decision is recorded before any follow-on run, and that run is the ordinary
  lifecycle.
- The commands are documented in the CLI reference and a guide, and the specification
  owns the rules.
- No new approval state, no second state machine, and no change to `sop run`'s stopping
  behaviour.

## Out of scope

Automatic or timed-out approval, auto-approval rules, a GUI or web approval surface,
remote or team-wide approval (that is the separate local-network-service candidate),
approving over email or a chat provider, any change to who may approve (V1 has no
identity or authorization model), a bulk `--all` approval, a second approval mechanism or
store, bypassing any gate, marketplace or plugin publishing, new providers or routing, and
any change to the meaning of `NEEDS_HUMAN`, `BLOCKED`, or `WAITING_FOR_HUMAN`.

## Validation

```bash
gofmt -l .
go vet ./...
go build ./...
go test ./...
go test -race ./...
make check
scripts/checks/check-doc-links.sh
```

Also exercised by hand on a project with a parked task: `sop approvals`, `sop approval
<id>`, an interactive `sop approve` at a TTY and the same command piped (which MUST fail
closed), `sop approve <id> --run`, and `sop decline <id>`.

## See also

- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md)
- [../specs/RECOVERY.md](../specs/RECOVERY.md) — requeue and the `needs_human`/`FAIL` split
- [../specs/QUALITY.md](../specs/QUALITY.md) — `PASS`/`FAIL`/`NEEDS_HUMAN`
- [../reference/CLI.md](../reference/CLI.md) and [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md)
- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Commit Gate, §14 Security and Permissions
- [BACKLOG.md](BACKLOG.md)
