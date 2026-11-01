# PLAN — SOP Lifecycle Consistency Hardening

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** completed read-only SOP Skills Audit (2026-10-08), including the focused
RM-003 lifecycle-inconsistency investigation (this repository, `c15f4ed`).\
**Plan id:** `plan-sop-lifecycle-consistency-hardening`

## Project

SOP Lifecycle Consistency Hardening

## Summary

Make SOP's approval and plan-closure semantics **internally consistent**. Today
three different notions of "pending / unresolved approval" coexist and disagree:

- `sop approvals`, `sop approval <id>`, and `sop continue --check` derive an
  approval's applicability as `PENDING && (task == nil || !task.IsSatisfied())`, so a
  pending request on a _satisfied_ task is treated as **moot and hidden**
  (`internal/approval/approval.go:347-355`; `internal/cli/continue.go:130-146`).
- `sop plan historicalize` readiness counts **any** raw `approval.json` with
  `status: PENDING` under **every** `.agent-sdlc/runs/*` directory, with no task
  filter and no scoping to the active plan
  (`internal/planflow/historicalize.go:466-498`).
- `sop plan complete` ignores approvals entirely and gates only on
  `domain.AllSatisfied` (`internal/planflow/planflow.go`, `internal/domain/task.go:287`).

The observed consequence (RM-003): a task that failed its fix loop left a `PENDING`
`NEEDS_HUMAN` approval (`requested_at 2026-10-08T14:41`); it was closed via external
completion (`sop task complete RM-003 --external`, `14:46`), which **never resolved
the pending approval**. The task became `LOCAL_DONE` → hidden from `sop approvals`,
yet `sop plan historicalize --check` reported `unresolved approval requests: RM-003`
— a direct contradiction. `sop plan complete` then archived the plan `COMPLETE` with
that approval still `PENDING` on disk (`sop plan complete` is the weaker closer), and
the stale record (scanned globally) would block future historicalizations of
**unrelated** plans.

This plan does not reinterpret that history. It fixes the invariants for _future_
lifecycles and adds deterministic regression coverage, while proving existing
successful completion behavior is unchanged. It adds **no new dependency**, **no
provider/model-specific logic**, and **no routing, budget, retry, or approval-authority
change**.

The work proceeds through eight stages: the approval-applicability contract (LC-001,
read-only), external completion resolving pending approvals (LC-002), scoping the
historicalization approval check to the active plan (LC-003), the supported
stale-approval supersession operation that removes the plan-closure deadlock (LC-007),
order-independent graph reconciliation (LC-008), aligning the
completion/historicalization closure invariants (LC-004), the deterministic regression
suite (LC-005), and the verification / behavior-preservation audit (LC-006).

## Capabilities

### Go build/test/race toolchain — EXISTS

- **Evidence:** `.github/workflows/ci.yml` runs gofmt, `go vet ./...`, `go build ./...`, `go test ./...`, and `go test -race ./...`; the local Go toolchain builds the repository.
- **Owner:** operator-supplied development environment
- **Location:** Go executable on PATH

### Recorded approval and run artifacts — EXISTS

- **Evidence:** `.agent-sdlc/runs/<TASK-ID>/` persists `approval.json`, `approval-history.json`, `state.json`, `gate.json`, `report.json`, and `external-completion.json`; 8 `approval.json` artifacts exist today (2026-10-08), exactly one (`RM-003`) `PENDING`.
- **Owner:** SOP lifecycle (existing)
- **Location:** `.agent-sdlc/runs/<TASK-ID>/`

### SOP approval boundary and persistence — EXISTS

- **Evidence:** `internal/approval/approval.go` (`Request`, `Approval`, `Approve`, `Decline`, `view`); `internal/run/approval.go` (`approval.json`, `approval-history.json`); the CLI, MCP, and lifecycle already delegate to this one service.
- **Owner:** agentic-sop
- **Location:** `internal/approval`, `internal/run`

### Plan compile and lifecycle engine — EXISTS

- **Evidence:** `internal/planner` (deterministic `PlanFromMarkdown`), `internal/planflow` (`Complete`, `Historicalize`, `EvaluateHistoricalization`), `internal/domain` (`AllSatisfied`, `IsSatisfied`).
- **Owner:** agentic-sop
- **Location:** `internal/planner`, `internal/planflow`, `internal/domain`

### Governed parallel worktree execution — MISSING

- **Evidence:** `internal/parallel/parallel.go` exists but is wired to no CLI command; `sop orchestrate` runs a single task and is opt-in (default off). Max safe concurrency is 1.
- **Owner:** agentic-sop (backlog)
- **Gap:** no supported mechanism runs governed SOP tasks concurrently in isolated worktrees.
- **Resolution:** not relied upon by any stage; the plan runs sequentially (concurrency 1). LC-002 and LC-003 are _proposed_ as worktree-parallelizable if that capability is ever wired, but no stage requires it. No stage declares this capability under `Requires`.

## Goal

Guarantee that every SOP surface that reports, gates on, or closes an approval
derives it from **one** authoritative definition, so that:

- the operator-facing "no pending approvals" can never contradict a readiness gate;
- a task completed externally never leaves a stale `PENDING` approval;
- a stale approval on an unrelated, completed task can never block an unrelated plan;
- the two plan-closure commands (`complete`, `historicalize`) cannot disagree about
  an open human gate or missing verification.

## Non-Goals

- Reinterpreting, rewriting, or deleting any archived `RM-003` (or other) approval,
  gate, run, or archive artifact. Historical evidence is immutable here.
- Changing approval _authority_, approval _policy_, or any human boundary; this plan
  only makes existing semantics consistent and observable.
- Weakening, relaxing, or removing any quality, validation, review, or acceptance gate.
- Installing, refreshing, or removing agent skills; adding RAG skills; adding a
  `sop metrics` / `sop report --all` command; changing provider/model/routing/budget.
- Adding a third-party dependency or any provider/model/task-domain-specific logic.
- Requiring a live model call or the network.

## Basis (authoritative, from the completed audit)

- SOP Skills Audit (2026-10-08, this repository @ `c15f4ed`) — §4c lifecycle-inconsistency investigation and §5 prioritized backlog.
- `internal/approval/approval.go:347-355` — `view.Applicable` predicate.
- `internal/cli/continue.go:130-146` — `pendingApprovalTaskIDs` (Applicable-derived).
- `internal/planflow/historicalize.go:466-498` — `historicalizationUnresolvedApprovals` (global raw-artifact scan).
- `internal/planflow/planflow.go` (`Complete`) and `internal/domain/task.go:287` — `AllSatisfied`-only closure.
- `internal/cli/task.go:170-197` — external completion (`CompleteExternally`), which does not touch approvals.
- `.agent-sdlc/runs/RM-003/approval.json` — the persisted `PENDING` record (`status: PENDING`) and `external-completion.json` (`implementation_commit f4f5fa7`).
- `docs/specs/HUMAN-APPROVAL.md`, `docs/specs/PLAN-HISTORICALIZATION.md` — the semantics this plan reports on and may clarify.

## Architecture Invariant

There is exactly **one** authoritative definition of an approval's state, owned by
`internal/approval`. Every consumer (`sop approvals`, `sop approval <id>`,
`sop continue`, `sop plan historicalize` readiness, `sop plan complete`, and run
finalization) must derive "pending / unresolved / applicable" from that definition,
never from a raw artifact read that bypasses it. A pending approval whose task is
already satisfied is **stale**: it must be _resolved into history_, never merely
hidden. Closure commands must fail **closed**: an open human gate or missing
verification must block closure on every path, or be surfaced identically on every
path — never silently skipped.

## Approval Applicability Model (target)

The single authoritative classification, computed by `internal/approval`:

```text
status        := the task's head approval request (approval.json)
resolved      := status in {APPROVED, DECLINED}
stale         := status == PENDING && task != nil && task.IsSatisfied()
applicable    := status == PENDING && !stale          (a human decision can be recorded)
unresolved    := status == PENDING                    (a human gate is open for THIS task)
```

- **Operator surfaces** (`sop approvals`, `sop approval`, `sop continue`) report
  _applicable_ approvals, and may additionally surface _stale_ ones labelled as stale.
- **Closure/readiness** (`historicalize`, and `complete` once aligned) blocks on
  _unresolved_ approvals **scoped to the active plan's task IDs**.
- **External completion** transitions any pending request for the completed task to
  a _resolved-into-history_ state (not `PENDING`), so no stale record persists.

## Closure Invariants

1. **I1 — External completion resolves approvals.** `sop task complete <id> --external`
   supersedes any pending approval for `<id>` into `approval-history.json`; no
   `PENDING` head remains for a completed task.
2. **I2 — Active-plan scoping.** Readiness considers only approvals of the active
   plan's tasks; an unrelated completed task cannot block a plan.
3. **I3 — One definition.** All consumers derive from `internal/approval`; no raw
   bypass read.
4. **I4 — Aligned closers.** `plan complete` and `plan historicalize` agree on
   open-gate and missing-verification invariants; `complete` is not a silent bypass.
5. **I5 — No contradiction.** `sop approvals` and any readiness gate never disagree
   for the same task.

## Dependency Graph

```text
LC-001 (approval-applicability contract, read-only)
   |
   +----------------------+
   |                      |
LC-002 (external)      LC-003 (scope historicalization approval check)
   |                      |
   +----------+-----------+
              |
       +------+------+
       |             |
   LC-007         LC-008 (order-independent graph reconciliation)
   (supported         |
    stale-approval    |
    supersession)     |
       |             |
   LC-004 (align closure invariants: complete vs historicalize)
       |             |
       +------+------+
              |
          LC-005 (deterministic regression suite:
           stale / active / cross-plan / externally completed)
              |
          LC-006 (verification + behavior-preservation audit; docs)
```

## Parallelization

LC-002 (`internal/cli/task.go`, `internal/approval`) and LC-003
(`internal/planflow/historicalize.go`) touch disjoint files and have no ordering
between them; they _could_ run in separate worktrees once governed parallel
execution exists. It does **not** exist today (`internal/parallel` is unwired), so
this plan executes **sequentially at concurrency 1**. No stage depends on the
`Governed parallel worktree execution` capability.

## Prerequisites Requiring Human Approval

1. **Behavior-change authorization.** LC-002, LC-003, LC-004, LC-007, and LC-008 change
   production runtime behavior (approval resolution, readiness scoping, closure
   gating, a new public approval status and operator command, and order-independent
   graph persistence). They must not be implemented or activated without explicit
   human approval.
2. **Closure-semantics authorization.** LC-004 may make `sop plan complete` stricter;
   the human confirms that tightening is acceptable and that no external automation
   depends on the previously weaker behavior.
3. **Commit authorization.** No stage is committed and the plan is not closed without
   separate human approval.
4. **Doc-change authorization.** Any clarification to `docs/specs/HUMAN-APPROVAL.md`
   or `docs/specs/PLAN-HISTORICALIZATION.md` is reviewed as a spec change.

LC-001 and LC-006 are read-only documentation stages; LC-005 adds tests only. They
require no approval to author and run.

## Deferrals

- **Skills installation fixes** (Claude Code missing `/sop-cleanup`, `/sop-continue`;
  Zed stale `sop-end-to-end` real dir) — deferred; not part of this plan.
- **RAG skills** (`/rag-schema`, `/rag-test`, `/rag-ingest`) — absent; product
  decision deferred.
- **Metrics CLI** (`sop metrics` / `sop report --all`) — deferred to the RM-005
  decision; untouched here.
- **RM-003 golden-immutability addendum** — deferred; RM-003 is closed and its
  evidence is preserved, not reopened.
- **Governed parallel worktree execution** — deferred to the backlog.
- **Graphify evaluation** — remains the last, unscheduled candidate at the tail of
  the backlog; not evaluated, scheduled, or implemented by this plan.

## Safety Invariants

This plan must not:

- change approval authority, approval policy, or any human approval boundary;
- weaken, relax, or remove any quality, validation, review, or acceptance gate;
- read or write the SOP state database by hand, or bypass any approval boundary;
- rewrite, delete, or reinterpret archived `approval.json`, `approval-history.json`,
  `gate.json`, run, or archive artifacts (including RM-003's);
- add a third-party dependency or any provider/model/task-domain-specific logic;
- change routing, retry, replan, budget, verification, or commit semantics;
- require a live model call or the network to run or test.

## Rollback and Reversibility

- Each behavior stage (LC-002/LC-003/LC-004/LC-007/LC-008) is a small, self-contained
  change guarded by tests; reverting its commit restores the prior behavior exactly.
- No stage mutates persisted SOP state or historical artifacts, so a revert leaves
  run/archive evidence untouched and no migration is required.
- LC-001/LC-005/LC-006 are read-only or test-only and are trivially reversible.
- If LC-004 tightening is judged undesirable in review, the plan's stop condition is
  to leave `complete` unchanged and record the decision in LC-006 rather than
  proceeding — no partial state is left behind.

## LC-001 — Approval Applicability Contract

Fix one authoritative definition of an approval's state and specify how every SOP
consumer derives "pending / unresolved / applicable" from it; record the current
three-way divergence and the RM-003 evidence that exposed it. This is a read-only
specification stage.

### Authoritative Inputs

- `internal/approval/approval.go`, `internal/cli/approvals.go`, `internal/cli/approval.go`, `internal/cli/continue.go`
- `internal/planflow/historicalize.go`, `internal/planflow/planflow.go`, `internal/domain/task.go`, `internal/domain/approval.go`
- `internal/run/approval.go`, `internal/cli/task.go` (external completion path)
- `.agent-sdlc/runs/RM-003/approval.json`, `.agent-sdlc/runs/RM-003/external-completion.json`
- The completed Skills Audit (lifecycle-inconsistency investigation)

### Mutation Targets

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md` — the only file this task writes; no production change.

### Dependencies

None

### Requires

- Go build/test/race toolchain
- Recorded approval and run artifacts

### Deliverables

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md` — the authoritative predicate, the stale/active/cross-plan/external rules, the per-consumer derivation table with file:line evidence, and the RM-003 disposition.

### Acceptance Criteria

- Names every consumer of approval state and its current predicate, with `file:line` evidence.
- Defines the single authoritative predicate and the `resolved` / `stale` / `applicable` / `unresolved` classification.
- States invariants I1–I5 verbatim and assigns each to the stage that enforces it.
- Documents the RM-003 disposition factually: a `PENDING` approval persists after external completion and plan archival; it is neither rewritten nor deleted by this plan.
- No production file is created, modified, or deleted.

### Execution Contract

1. Read each consumer and record its approval predicate with `file:line`.
2. State the authoritative predicate and the I1–I5 invariants.
3. Produce the contract and the RM-003 disposition.
4. Finish.

Expected first action: read `internal/approval/approval.go` (`view`, `Approval`, `decide`).

### Production-Change Scope

None

## LC-002 — External Completion Resolves Pending Approvals

When `sop task complete <id> --external` records completion, resolve any pending
approval for that task into history so no `PENDING` head remains. This enforces I1
and preserves the audit trail.

### Authoritative Inputs

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
- `internal/cli/task.go` (`runTaskComplete` external path), `internal/approval/approval.go`, `internal/run/approval.go`
- `.agent-sdlc/runs/RM-003/approval.json` / `approval-history.json` (shape reference only)

### Mutation Targets

- `internal/cli/task.go`, `internal/approval/approval.go` — resolve/supersede the task's pending approval on external completion.
- Test files: `internal/cli/task_complete_test.go`, `internal/approval/approval_test.go`.

### Dependencies

LC-001

### Requires

- Go build/test/race toolchain

### Deliverables

- A resolution path invoked by external completion that moves a pending request into `approval-history.json` and leaves no `PENDING` head, plus focused tests.

### Acceptance Criteria

- After `sop task complete <id> --external`, the task's `approval.json` reports no `PENDING` status, and the superseded request is present in `approval-history.json`.
- The behavior is idempotent: a second external completion is a no-op and never duplicates history.
- External completion still requires passing validation and an advanced dependency graph exactly as before; no gate is relaxed.
- No historical artifact outside the completed task's own run directory is modified.
- `gofmt -l .` is clean and `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...` pass.

### Execution Contract

1. Add a service method that supersedes a task's pending approval into history.
2. Call it from the external-completion path only (never from ordinary run completion).
3. Add focused tests (pending → resolved; idempotent re-run; history preserved).
4. Run the validation gates.
5. Finish.

Expected first action: read `runTaskComplete` in `internal/cli/task.go`.

### Production-Change Scope

`internal/cli/task.go`, `internal/approval/approval.go` (external-completion resolution only)

### Rollback

Revert the commit; ordinary run completion and all existing artifacts are unaffected.

## LC-003 — Scope Historicalization Approval Check to the Active Plan

Make the historicalize readiness scan consider only the active plan's tasks (or
apply the `Applicable` staleness rule), so a stale approval on an unrelated completed
task cannot make a plan ineligible. This enforces I2.

### Authoritative Inputs

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
- `internal/planflow/historicalize.go` (`historicalizationUnresolvedApprovals`, `evaluateHistoricalization`)
- `internal/approval/approval.go` (the authoritative predicate)

### Mutation Targets

- `internal/planflow/historicalize.go` — scope the approval scan to the active plan's task IDs / apply the stale rule.
- Test files: `internal/planflow/historicalize_test.go`.

### Dependencies

LC-001

### Requires

- Go build/test/race toolchain

### Deliverables

- A scoped approval check in historicalization readiness, plus tests for the active, stale, and cross-plan cases.

### Acceptance Criteria

- A pending approval belonging to a task **not** in the active plan does not make the plan ineligible.
- A pending approval on an **active-plan** task still makes the plan ineligible, with the task id in the reason (unchanged behavior).
- A PENDING approval belonging to an active-plan task blocks historicalization regardless of whether that task is satisfied. A PENDING approval belonging to a task outside the active plan does not block historicalization. Resolved approvals do not block.
- Readiness is deterministic and model-free; no run/archive artifact is modified.
- `gofmt -l .` is clean and the Go build/test/race gates pass.

### Execution Contract

1. Scope the scan to the active task set (or reuse the authoritative predicate).
2. Add tests: cross-plan pending (ignored), active pending (blocks), stale pending (ignored).
3. Run the validation gates.
4. Finish.

Expected first action: read `historicalizationUnresolvedApprovals` in `internal/planflow/historicalize.go`.

### Production-Change Scope

`internal/planflow/historicalize.go` (scope of the approval scan)

### Rollback

Revert the commit; the previous (global) scan behavior is restored exactly.

## LC-007 — Supported Stale-Approval Supersession

Remove the plan-closure deadlock that LC-004 would otherwise create: a satisfied
active-plan task can carry a `PENDING` approval that no supported operation can clear
(the approval boundary refuses a decision on a satisfied task, and external
completion is rejected on an already-complete task). Provide an explicit,
operator-authorized way to supersede such a request without overloading `DECLINED`,
so the shared closure gates (LC-004) block only states an operator can actually leave.

### Authoritative Inputs

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
- `internal/domain/approval.go` (`ApprovalStatus`, `Resolved`, `ApprovalRequest`)
- `internal/approval/approval.go` (`Service`, `decide`, `View`)
- `internal/cli/approval.go` (`runApproval`, `runApprove`, `runDecline`)
- `internal/run/approval.go`, `internal/run/approval_resolution.go` (append-only history)

### Mutation Targets

- `internal/domain/approval.go` — add `ApprovalSuperseded`; extend `Resolved()`.
- `internal/approval/approval.go` — a `Supersede` operation with the authorization,
  audit, idempotency, and fail-closed rules below.
- `internal/cli/approval.go` — `sop approval supersede <task-id> --reason TEXT --by NAME`.
- Test files: `internal/domain/approval_test.go`, `internal/approval/approval_test.go`,
  `internal/cli/approval_test.go`.

### Dependencies

- LC-002
- LC-003

### Requires

- Go build/test/race toolchain
- Recorded approval and run artifacts

### Deliverables

- A supported, explicit operator operation that supersedes a stale `PENDING` approval
  on a satisfied task, records an audited reason and operator, preserves the original
  request in append-only history, and is idempotent and fail-closed.

### Acceptance Criteria

- `ApprovalSuperseded` is an explicit `ApprovalStatus`, and `Resolved()` recognizes
  `SUPERSEDED` alongside `APPROVED` and `DECLINED`.
- A supported command `sop approval supersede <task-id> --reason TEXT --by NAME`
  exists; an explicit reason and an identifiable operator are required.
- Supersession is permitted only for a `PENDING` approval whose task is satisfied; a
  pending request on an unsatisfied task is refused (it must be decided, not
  superseded), and a non-pending head is refused.
- The original request is preserved verbatim in append-only `approval-history.json`;
  the head is rewritten to `SUPERSEDED` recording the operator, reason, timestamp, and
  the request identity.
- Repeated supersession calls are idempotent: no duplicate history entry and no change
  to an already-superseded head.
- Persistence failures fail closed: a history or head write failure returns an error
  and reports no successful supersession, leaving the head `PENDING`.
- Existing `PENDING`, `APPROVED`, and `DECLINED` records remain readable with unchanged
  semantics; no existing artifact is rewritten.
- RM-003's `approval.json`, `approval-history.json`, and archive remain byte-unchanged.
- Deterministic regression tests cover success, refusal (unsatisfied, non-pending,
  missing record), repeated calls, history/head persistence errors, and evidence
  preservation — no clock, network, provider, or model dependency.
- `gofmt -l .`, `go vet ./...`, `go build ./...`, and `go test -race ./...` pass.

### Execution Contract

1. Add `ApprovalSuperseded` and extend `Resolved()`.
2. Implement `Service.Supersede`, reusing the append-only history and the LC-002
   supersede mechanics, and requiring the explicit reason and operator.
3. Wire `sop approval supersede` in the CLI.
4. Add the regression tests.
5. Run the validation gates.
6. Finish.

Expected first action: read `ApprovalStatus` and `Resolved()` in `internal/domain/approval.go`.

### Production-Change Scope

`internal/domain/approval.go`, `internal/approval/approval.go`, `internal/cli/approval.go`
(a new public approval status and operator command)

### Rollback

Revert the commit; `SUPERSEDED` and the supersede command disappear and the previous
`PENDING`/`APPROVED`/`DECLINED`-only semantics are restored exactly. No persisted
artifact is migrated, so a revert leaves run/archive evidence untouched.

## LC-008 — Order-Independent Graph Reconciliation

Make `store.ReplaceGraph` persist a reconciled graph without order-sensitive
foreign-key failures. Today `ReplaceGraph` upserts each task and its dependency edges
together, so a task whose dependency appears later in the input slice violates the
`task_dependencies.dependency_task_id` foreign key — exposed whenever reconciliation
adds a new task that an earlier-listed task depends on. `SaveTasks` already avoids this
with a two-phase rows-then-edges write; `ReplaceGraph` must do the same.

### Authoritative Inputs

- `internal/store/sqlite.go` (`ReplaceGraph`, `upsertTask`, `SaveTasks`)
- `internal/store/graph_test.go`, `internal/store/batch_test.go`

### Mutation Targets

- `internal/store/sqlite.go` — split `ReplaceGraph` into a row phase and an edge phase
  so dependencies resolve regardless of input order.
- Test files: `internal/store/graph_test.go` (and `internal/store/batch_test.go` if
  needed).

### Dependencies

- LC-003

### Requires

- Go build/test/race toolchain

### Deliverables

- `ReplaceGraph` persists all task rows before any dependency edge, so a reconciled
  graph succeeds irrespective of the order tasks appear in the input slice, with
  rollback-safe transactions and unchanged `SaveTasks` behavior.

### Acceptance Criteria

- `ReplaceGraph` writes task rows before dependency edges.
- Task ordering in the input graph does not affect success.
- Forward dependencies on newly added tasks — an earlier-listed task depending on a
  later-listed new task — persist successfully.
- Existing dependency relationships remain intact after an update.
- A failure rolls the whole transaction back, leaving the original graph unchanged.
- `SaveTasks` behavior is unchanged.
- Deterministic tests cover forward references, reversed input ordering, rollback,
  and in-place updates of an existing graph.

### Execution Contract

1. Refactor `ReplaceGraph` to a two-phase apply (rows, then edges), reusing the
   existing dependency-edge writer.
2. Keep the transaction and rollback semantics.
3. Add the regression tests.
4. Run the validation gates.
5. Finish.

Expected first action: read `ReplaceGraph` and `upsertTask` in `internal/store/sqlite.go`.

### Production-Change Scope

`internal/store/sqlite.go` (graph persistence only)

### Rollback

Revert the commit; `ReplaceGraph` returns to the interleaved single-pass behavior
exactly. No persisted artifact is migrated.

## LC-004 — Align Closure Invariants: complete vs historicalize

Ensure `sop plan complete` and `sop plan historicalize` agree on closure invariants:
`complete` must not archive a plan that has an unresolved approval or missing
verification. This enforces I3/I4/I5.

### Authoritative Inputs

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
- `internal/planflow/planflow.go` (`Complete`), `internal/planflow/historicalize.go` (`evaluateHistoricalization`), `internal/cli/plan.go` (`runPlanComplete`)
- `internal/domain/task.go` (`AllSatisfied`)

### Mutation Targets

- `internal/planflow/planflow.go` (`Complete`), `internal/cli/plan.go` — reuse readiness so `complete` fails closed on an open gate or missing verification.
- Test files: `internal/planflow/complete_test.go`, `internal/cli/plan_complete_test.go`.

### Dependencies

- LC-001
- LC-002
- LC-003
- LC-007
- LC-008

### Requires

- Go build/test/race toolchain

### Deliverables

- `plan complete` reusing the readiness invariants (or an equivalent fail-closed check), plus tests proving the tightening and proving clean-plan completion is unchanged.

### Acceptance Criteria

- `sop plan complete` refuses, with a clear reason, when the active plan has an unresolved approval or unresolved verification — matching historicalize readiness.
- A clean, fully-satisfied plan still completes exactly as before (regression), preserving task history and the `COMPLETE` disposition.
- No approval authority, quality threshold, or human boundary is changed.
- `gofmt -l .` is clean and the Go build/test/race gates pass.

### Execution Contract

1. Have `Complete` consult readiness (or a shared helper) before archiving.
2. Return an actionable refusal when a gate is open; do not archive.
3. Add tests: refusal on pending approval; refusal on missing verification; unchanged clean completion.
4. Run the validation gates.
5. Finish.

Expected first action: read `Complete` in `internal/planflow/planflow.go`.

### Production-Change Scope

`internal/planflow/planflow.go`, `internal/cli/plan.go` (closure gating only)

### Rollback

Revert the commit; `complete` returns to `AllSatisfied`-only behavior. If review deems
the tightening undesirable, stop here and record the decision in LC-006 instead.

## LC-005 — Deterministic Regression Suite for Approval States

Add table-driven, model-free regression tests covering every approval state across
the affected surfaces: active, stale, cross-plan, and externally-completed approvals.

### Authoritative Inputs

- `docs/reports/lifecycle-consistency/LC-001-approval-applicability-contract.md`
- `internal/approval/approval_test.go`, `internal/planflow/historicalize_test.go`, `internal/cli/*_test.go`

### Mutation Targets

- Test files only: `internal/approval/approval_test.go`, `internal/planflow/historicalize_test.go`, `internal/cli/task_complete_test.go`, `internal/cli/plan_complete_test.go` — no production change.

### Dependencies

- LC-002
- LC-003
- LC-004
- LC-008

### Requires

- Go build/test/race toolchain

### Deliverables

- A table-driven suite asserting each consumer's behavior for: active pending, stale (satisfied) pending, cross-plan pending, externally-completed pending, resolved, and no-approval.

### Acceptance Criteria

- A single table enumerates the states with the expected result per consumer (`approvals`, `continue`, `historicalize` readiness, `complete`).
- Tests are deterministic: no clock, network, provider, or model dependency; each runs in a temp dir.
- The suite proves I1–I5 hold: no contradiction between `approvals` and readiness for the same task; cross-plan pending is ignored; active pending blocks; external completion leaves no `PENDING` head.
- `go test -race ./...` passes; `gofmt -l .` is clean.

### Execution Contract

1. Build the state table and the expected-result matrix.
2. Implement the tests across the affected packages.
3. Run the full race suite.
4. Finish.

Expected first action: enumerate the states and consumers in a table.

### Production-Change Scope

None (tests only)

## LC-006 — Verification and Behavior-Preservation Audit

Independently verify that the hardening holds and that existing successful completion
behavior is unchanged; consolidate any spec clarification. This is a read-only audit
stage that may update documentation.

### Authoritative Inputs

- The LC-002…LC-005 changes
- `docs/specs/HUMAN-APPROVAL.md`, `docs/specs/PLAN-HISTORICALIZATION.md`
- `.agent-sdlc/runs/RM-003/` (proves historical evidence is intact and untouched)

### Mutation Targets

- `docs/reports/lifecycle-consistency/LC-006-verification.md`
- Documentation only, when needed: `docs/specs/HUMAN-APPROVAL.md`, `docs/specs/PLAN-HISTORICALIZATION.md`.

### Dependencies

LC-005

### Requires

- Go build/test/race toolchain
- Recorded approval and run artifacts

### Deliverables

- `docs/reports/lifecycle-consistency/LC-006-verification.md` — the acceptance-gate result (GO/HOLD), the behavior-preservation evidence, and any spec clarification.

### Acceptance Criteria

- Full gates pass: `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`, `go test -race ./...`, documentation link check, and `git diff --check`.
- Before/after evidence shows clean-plan completion still archives `COMPLETE` and preserves task history.
- Evidence shows RM-003's `approval.json`, `approval-history.json`, and archive are byte-unchanged by this plan.
- Every changed spec statement is consistent with implemented and tested behavior; no gate is documented as weaker than it is.
- The report states exactly which surfaces were verified and assigns a GO or HOLD verdict.

### Execution Contract

1. Run the full validation suite.
2. Diff RM-003 evidence against the pre-plan baseline to prove immutability.
3. Confirm clean-plan completion is unchanged; record the verdict.
4. Update specs only where behavior changed; keep doc links resolving.
5. Finish.

Expected first action: run the full validation suite.

### Production-Change Scope

Documentation only

## Acceptance Gate for the Plan (GO / HOLD)

The plan is **GO** — safe to close — only when **all** hold:

- I1–I5 are enforced and covered by deterministic tests (LC-005).
- `sop approvals` and readiness never disagree for the same task.
- External completion leaves no `PENDING` approval head for the completed task.
- A stale approval on an unrelated, completed task cannot block a plan.
- Clean-plan `complete` and `historicalize` behavior is unchanged (regression-proven).
- RM-003 and all other archived/historical evidence is byte-unchanged.
- `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./...`,
  `go test -race ./...`, the documentation link check, and `git diff --check` all pass.

Otherwise the verdict is **HOLD**, with the unmet criterion named and no closure
recorded.
