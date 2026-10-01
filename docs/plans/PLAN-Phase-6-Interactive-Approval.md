# PLAN — Phase 6: Interactive Approval Workflow

**Type:** Implementation plan (normative for the work it describes; the specifications
win wherever they disagree).

**Status:** Implemented. The **read surface** — `sop approvals`, the read-only
`sop reconcile --list-changed`, and the rules and documentation that go with them —
shipped (`P6-001`–`P6-004`). The **interactive surface** — a parked run naming its own
gate, choosing among the applicable gates at a terminal (fail-closed off one), the
explicit `--run` continuation, and the rules, guide, and tests that go with them —
shipped (`P6-005`–`P6-011`). Each stage declares `execution_mode: done`. The work items
are tracked as `P6-NNN`. The rules this plan MUST NOT break are normative in
[../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md); the rules it adds are declared
there by P6-003 and P6-008. See [../reference/CLI.md](../reference/CLI.md).

## Objective

Make SOP's human decision points operable and discoverable by a client — the CLI, a
controller, or MCP — without moving any authority. Today the mechanism works but a
client cannot see it: a task parks at the gate, SOP records an explicit, resolvable
request, and a client can read one (`sop approval`) or resolve it (`sop approve` /
`sop decline`, or the MCP tools) — but there was **no way to enumerate the tasks
waiting**, no way to see what a plan reconciliation would change **before** it mutates,
a parked run did not name the command that resolves it, there was no interactive path,
and the commands were absent from [../reference/CLI.md](../reference/CLI.md).

This phase closes those gaps and nothing else. Two of them are the SOP operations an
external consumer needs but could not reach:

- **enumerate the gates** — so a client can render "waiting on you" without diffing
  SOP's state or inferring a gate from a task status (P6-001);
- **list a plan's changed executed tasks** before mutating — the read-only counterpart
  of `sop reconcile --accept-changed`, which otherwise reports the change only by
  refusing (P6-002).

SOP still records the gate, decides whether it is applicable, and owns the lifecycle
response; an approval still resolves only the specific gate that raised it and still
cannot manufacture completion.

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
  (enumerate; SHIPPED)      (read one)              (record a decision, --by/--note)
        └─────────────────────────┴──────────────────────────┘
                                  ↓
                  approval.Service   (the single application boundary)
                                  ↓
        domain.RequeueWithoutSpending  →  the task is runnable again
                                  ↓
                 sop run   (a NEW, ordinary run; `--run` starts it explicitly)

  sop reconcile <PLAN.md> --list-changed     (SHIPPED: the same diff, applied nowhere)
                                  │
                                  ↓
        the executed tasks needing --accept-changed, named before anything mutates
```

The CLI and MCP stay thin clients of `approval.Service` and `planflow`. Nothing in this
phase selects a model, routes, validates, reviews, or transitions state except through
the domain operations that already exist. `sop run` continues to stop at the gate
([HUMAN-APPROVAL §3](../specs/HUMAN-APPROVAL.md)); the interactive work lives in the
approval commands, not in the run.

## Tasks

### P6-001 — Enumerate the pending approval gates

- **Status:** Done.
- **Execution:** done.
- **Scope:** a read-only `sop approvals` lists every task with an **applicable** approval
  request (present and `PENDING` and its task can still act), rendering the task id,
  kind, stage, and reason, with `no pending approvals` when there are none and `--json`
  for a machine-readable list. It reads `store.List()` and asks
  `approval.Service.Approval` for each task, so it classifies nothing itself and never
  mutates. `View.Applicable` was corrected to mean what it documents: a pending request
  whose task has already completed is stale and is reported as not applicable, matching
  the decision boundary that refuses it.
- **Files:** `internal/cli/approval.go`, `internal/cli/cli.go`,
  `internal/approval/approval.go`, `internal/cli/approvals_test.go`,
  `internal/approval/approval_test.go`.
- **Depends on:** —
- **Acceptance:** `sop approvals` prints the empty message and exits 0 with no gates;
  with a pending, a resolved, and a stale request across three tasks it lists only the
  pending one; `--json` emits an array ([] when empty); nothing is written.

### P6-002 — List a plan's changed executed tasks before mutating

- **Status:** Done.
- **Execution:** done.
- **Scope:** `sop reconcile <PLAN.md> --list-changed [--json]` reports what a
  reconciliation would change **without changing anything**: it names every changed
  executed task (whose approval `--accept-changed` would take) and every executed task
  the plan removes (which cannot be approved and must be kept or completed first).
  `planflow.Inspect` computes it from the **same diff** the applying path uses — one
  `reconcileGraph`, two dispositions — so a preview can never disagree with what
  applying does. Combining `--list-changed` with `--accept-changed` is a usage error,
  and `--json` is rejected without the listing, so a flag is never silently ignored.
- **Files:** `internal/planflow/planflow.go`, `internal/cli/reconcile.go`,
  `internal/planflow/inspect_test.go`, `internal/cli/reconcile_list_test.go`.
- **Depends on:** —
- **Acceptance:** the listing names the changed executed task and applies nothing (the
  graph, the machine plan, and the provenance are byte-identical afterwards, and a
  following reconciliation still stops); `--json` is a stable document with a non-null
  array per category; a removed executed task is reported, never offered as approvable.

### P6-003 — Declare the read surface in the specifications

- **Status:** Done.
- **Execution:** done.
- **Scope:** the normative rules for what shipped: [HUMAN-APPROVAL §6](../specs/HUMAN-APPROVAL.md)
  (the gate is exposed as data; enumeration creates, refreshes, and resolves nothing and
  infers no gate from a status, a blocked reason, attempts, or prose; a request is
  applicable only while pending **and** its task can still act; a decision goes through
  SOP's single application boundary) and [RECOVERY §7](../specs/RECOVERY.md)
  (`--list-changed` reports without writing, names both human-decision categories, and
  must be the same diff as the reconciliation).
- **Files:** `docs/specs/HUMAN-APPROVAL.md`, `docs/specs/RECOVERY.md`.
- **Depends on:** P6-001, P6-002
- **Acceptance:** every rule the shipped surface implements has one normative home in the
  specifications; nothing is restated from the sections that already own it.

### P6-004 — Document the read surface

- **Status:** Done.
- **Execution:** done.
- **Scope:** the command table in [../reference/CLI.md](../reference/CLI.md) gains
  `sop approvals`, `sop approval`, `sop approve`, and `sop decline`, the `sop reconcile`
  row gains `--list-changed [--json]`, and the stale `sop mcp` row (which predated the
  approval tools) is corrected. `sop help` is corrected too: `reconcile`, `providers`,
  and `eval` still carried a literal tab and misaligned columns from an earlier insert,
  and `approvals` is added alongside the other approval commands.
- **Files:** `docs/reference/CLI.md`, `internal/cli/cli.go`.
- **Depends on:** P6-001, P6-002, P6-003
- **Acceptance:** every approval command the CLI dispatches appears in the reference with
  its flags; no line of `sop help` begins with a tab and the columns align; the
  documentation link check passes.

### P6-005 — A parked run names its own gate

- **Status:** Done.
- **Execution:** done.
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

### P6-006 — Interactive selection at a terminal

- **Status:** Done.
- **Execution:** done.
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

### P6-007 — Record the decision, then continue

- **Status:** Done.
- **Execution:** done.
- **Scope:** add an explicit `--run` to `sop approve <id>` that, **only after the decision
  is recorded**, starts the ordinary run for the requeued task. The decision is persisted
  first, so a failure to start the run leaves the approval recorded and the task
  runnable; `--run` never skips a gate, never commits, pushes, or merges, and never
  re-implements an approval the human did not make. Without `--run` the command's
  behaviour is unchanged.
- **Files:** `internal/cli/approval.go`, `internal/cli/drive.go`.
- **Depends on:** P6-006
- **Acceptance:** approving with `--run` leaves the approval recorded and starts the same
  run path `sop run` uses; a decision that is refused (stale, already resolved the other
  way) starts no run; the run that follows still stops at its own next gate.

### P6-008 — Declare the interactive surface in the specification

- **Status:** Done.
- **Execution:** done.
- **Scope:** extend [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md) with the
  normative rules for the interactive surface: a client MUST NOT manufacture approval, an
  interactive client MUST fail closed when it cannot ask a human, and `--run` MUST record
  the decision before starting the run and MUST NOT bypass a gate. Link the existing
  sections instead of restating them.
- **Files:** `docs/specs/HUMAN-APPROVAL.md`.
- **Depends on:** P6-005, P6-006, P6-007
- **Acceptance:** every rule the interactive surface implements has one normative home;
  nothing is duplicated from §1–§6.

### P6-009 — Document the interactive surface

- **Status:** Done.
- **Execution:** done.
- **Scope:** add a task-oriented guide, `docs/guides/APPROVALS.md`, that walks a human
  through seeing a gate, reading it, deciding it, and continuing — and states plainly that
  the run stops at the gate and that an approval is not a completion. Update the
  reference rows for the new flags and index the guide.
- **Files:** `docs/guides/APPROVALS.md`, `docs/reference/CLI.md`, `docs/README.md`.
- **Depends on:** P6-005, P6-006, P6-007, P6-008
- **Acceptance:** the guide's commands run as written on a project with a parked task; the
  documentation link check passes.

### P6-010 — Deterministic tests for the surface

- **Status:** Done.
- **Execution:** done.
- **Scope:** cover the interactive surface with no terminal and no model: fail-closed
  non-TTY selection, a selected gate that is not applicable, `--run` recording before
  continuing and starting nothing when the decision is refused, and the guarantee that a
  task status or free-form prose alone produces no applicable gate. Reuse the existing
  approval fixtures.
- **Files:** `internal/cli/approval_interactive_test.go`.
- **Depends on:** P6-005, P6-006, P6-007
- **Acceptance:** the cases above pass deterministically; a test proves interactivity is
  never exercised from a non-TTY; no test requires a live provider.

### P6-011 — Wire the phase and validate

- **Status:** Done.
- **Execution:** done.
- **Scope:** index this plan and record it in [BACKLOG.md](BACKLOG.md), add the plan to the
  compiler contract in `internal/planner/taskshape_test.go`, and run the full validation
  for the phase.
- **Files:** `docs/README.md`, `docs/plans/BACKLOG.md`,
  `internal/planner/taskshape_test.go`.
- **Depends on:** P6-001, P6-002, P6-003, P6-004, P6-005, P6-006, P6-007, P6-008, P6-009, P6-010
- **Acceptance:** the plan compiles deterministically with the declared stage count;
  `gofmt -l .` is empty and `go vet`, `go build`, `go test`, and `go test -race` pass; the
  documentation link check reports no broken links; no SOP policy or execution behaviour
  outside the approval and reconcile surfaces changed.

## Definition of Done

- A client can enumerate every task waiting at a gate, and read what a plan
  reconciliation would change before it mutates anything, without diffing SOP's state or
  reading an internal file.
- A human can read one gate, decide it, and continue — without knowing any internal file
  path.
- A run that parks a task names the gate and the command that resolves it, and still
  stops there.
- Interactivity fails closed off a TTY and never reads a decision a human did not make.
- An approval resolves only the gate that raised it; it cannot manufacture completion and
  cannot bypass validation, review, JEV, or the quality gate.
- The decision is recorded before any follow-on run, and that run is the ordinary
  lifecycle.
- The commands are documented in the CLI reference, the help listing renders correctly,
  and the specification owns the rules.
- No new approval state, no second state machine, and no change to `sop run`'s stopping
  behaviour.

## Out of scope

Automatic or timed-out approval, auto-approval rules, a GUI or web approval surface,
remote or team-wide approval (that is the separate local-network-service candidate),
approving over email or a chat provider, any change to who may approve (V1 has no
identity or authorization model), a bulk `--all` approval, a second approval mechanism or
store, bypassing any gate, marketplace or plugin publishing, new providers or routing, and
any change to the meaning of `NEEDS_HUMAN`, `BLOCKED`, or `WAITING_FOR_HUMAN`. The
controller's own client work (wiring its approval controls to these operations) lives in
the `sop-controller` repository, not here.

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
<id>`, `sop reconcile <PLAN.md> --list-changed`, an interactive `sop approve` at a TTY and
the same command piped (which MUST fail closed), `sop approve <id> --run`, and `sop
decline <id>`.

## See also

- [../specs/HUMAN-APPROVAL.md](../specs/HUMAN-APPROVAL.md)
- [../specs/RECOVERY.md](../specs/RECOVERY.md) — requeue, and the reconciliation listing
- [../specs/QUALITY.md](../specs/QUALITY.md) — `PASS`/`FAIL`/`NEEDS_HUMAN`
- [../reference/CLI.md](../reference/CLI.md) and [../reference/STATUS-AND-RECOVERY.md](../reference/STATUS-AND-RECOVERY.md)
- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Commit Gate, §14 Security and Permissions
- [../architecture/SOP-BOUNDARY.md](../architecture/SOP-BOUNDARY.md) — what a consumer may do
- [BACKLOG.md](BACKLOG.md)
