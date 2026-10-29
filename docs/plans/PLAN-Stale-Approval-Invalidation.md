# PLAN — Invalidate Stale Approval Evidence After an Accepted Plan Revision

**Status:** IMPLEMENTED — merged in PR #141 (`bf2a562a`). APPR-001–APPR-005 reached `LOCAL_DONE` with `VERIFIED` outcomes and a passing final gate; this records committed-source status, not current operator-local plan state.
**Rule:** A human decision is SOP's authority. This plan adds one supported operator operation that invalidates a PENDING approval request whose evidence the active plan no longer contains. It never approves, satisfies, completes, or waives work, and it never weakens the blocking semantics of a valid PENDING or DECLINED request.

## Project

Invalidate Stale Approval Evidence After an Accepted Plan Revision

## Summary

When an operator reconciles a corrected plan with `sop reconcile --accept-changed TASK`, SOP deliberately preserves the executed task's lifecycle and history — but it leaves the old `PENDING` approval request in place. If that request was raised for evidence the reconciled definition no longer contains, the task is deadlocked:

- `sop approval supersede` requires a satisfied task (`internal/approval/approval.go`), so it refuses a task that reconciliation returned to `READY`.
- `sop decline` is accepted, but `approvalExecutionGate` (`internal/cli/approval.go`) blocks every later run, and `decide` refuses to approve a declined head — a one-way door.
- `sop approve` is the only path that permits execution, and it records `Approved: true` on evidence the operator has already established is void.

The repair adds a third, narrower resolution: **invalidation**. It is available only for a `PENDING` request on an unsatisfied task whose evidence is provably obsolete against the reconciled definition. It archives the original request verbatim, records who invalidated it, why, and which plan revisions and definition digests the decision spans, and leaves the task exactly as runnable as it was — so the next run must produce fresh execution and outcome evidence.

Applicability is decided deterministically, not by operator assertion:

- a request raised by this version carries a digest of the task definition it was raised against; invalidation is refused when the current definition still produces that digest;
- a legacy request carries no digest, so invalidation requires SOP's own reconciliation provenance for that task — the task must be recorded as an explicitly accepted reconciled task, reconciled strictly after the request was raised.

## APPR-001 — Invalidation vocabulary and audit record

Priority: P1. Extend the approval domain with the invalidation outcome and the durable record the operator decision must leave behind, following the existing append-only request/history model in `internal/domain/approval.go` and `internal/run/approval.go`.

### Dependencies

None

### Deliverables

- `internal/domain/approval.go`, `internal/domain/approval_test.go` — the `INVALIDATED` request status, its place in `Resolved()`, the `ApprovalInvalidation` record, and `DefinitionDigest` on the request.

### Acceptance Criteria

- `INVALIDATED` is a distinct, resolved status: it is not `APPROVED`, not `DECLINED`, and not `SUPERSEDED`, so no consumer can read it as a human acceptance.
- The invalidation record carries the operator's reason and identity, the invalidation time, the plan source fingerprints on both sides of the revision, the reconciliation time, and the definition digests on both sides of the revision.
- An invalidation record is absent from a request that was never invalidated, and its presence never sets the approval decision or lifecycle action fields.
- A request raised against a known task definition stores that definition's digest so a later invalidation can compare it.

### Acceptance Checks

- `go test ./internal/domain/...`
- `go vet ./internal/domain/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1

### Failure Behavior

If the existing `ApprovalRequest` JSON shape cannot carry an optional record without changing the encoding of requests that have none, the record is added as an omitted-when-empty field so existing artifacts round-trip byte-identically.

## APPR-002 — Definition identity and revision evidence

Priority: P1. Make "the evidence this request was raised against" observable. Expose the reconciled task definition SOP already persists, a deterministic digest of it, and the reconciliation provenance for one task, without adding a second source of truth.

### Dependencies

- APPR-001

### Deliverables

- `internal/planflow/definition.go`, `internal/planflow/definition_test.go` — a deterministic definition digest over the persisted stage, and a per-task reconciliation provenance read.

### Acceptance Criteria

- The digest is computed from the persisted active plan's stage for the task (deliverables, acceptance criteria, checks, mapping, dependencies), is stable across reads and processes, and changes when any of those fields changes.
- The digest is empty and reported absent for a task the active plan does not define.
- The reconciliation provenance reports whether a task was explicitly accepted as reconciled and when that reconciliation was recorded, or reports it absent.
- No new plan file, database table, or duplicate definition store is introduced.

### Acceptance Checks

- `go test ./internal/planflow/...`
- `go vet ./internal/planflow/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1

### Failure Behavior

If the persisted plan cannot be read, the digest and provenance report absent rather than guessing; an invalidation that cannot establish obsolescence is refused.

## APPR-003 — The invalidation operation

Priority: P1. Add the one operator operation, with every guard the issue requires, at SOP's single approval boundary in `internal/approval/approval.go`. It is deterministic and I/O-free like its siblings: revision evidence is supplied by the caller, and no task field is written.

### Dependencies

- APPR-002

### Deliverables

- `internal/approval/approval.go`, `internal/approval/approval_test.go` — `Invalidate` with its guards, audit archiving, and the invalidation record.

### Acceptance Criteria

- Invalidation requires a non-empty reason and an identifiable actor, and is refused without both.
- Invalidation applies only to a `PENDING` request; an approved, declined, superseded, or already-invalidated head is refused, so a recorded human decision is never overwritten.
- Invalidation is refused for a satisfied task, directing the operator to the existing supersede operation instead; a satisfied task is never invalidated into re-execution.
- Invalidation is refused when the current definition digest equals the digest recorded on the request, and when a legacy request has no reconciliation provenance proving the definition was accepted as changed after the request was raised.
- A permitted invalidation preserves the original request and its evidence verbatim in the append-only history before the head changes, and does not duplicate that history entry on a retry.
- The task is not written by invalidation: its status, attempt, satisfaction, and completion are unchanged, so nothing is approved, satisfied, or completed.
- The invalidation reason, actor, plan fingerprints, reconciliation time, and both definition digests are persisted on the head and remain readable after a restart.

### Acceptance Checks

- `go test ./internal/approval/...`
- `go vet ./internal/approval/...`
- `go test ./internal/run/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1
- criterion 7: check 1

### Failure Behavior

If archiving the original request fails, the operation fails closed with the head untouched and still `PENDING`, so a partial invalidation can never hide the evidence it was meant to preserve.

## APPR-004 — Operator surface and execution gating

Priority: P1. Expose invalidation as `sop approval invalidate`, wire the revision evidence the service needs, stamp new requests with their definition digest, and teach the execution gate that an invalidated head permits execution while pending and declined heads keep blocking exactly as before.

Execution guidance: before claiming completion, read every implementation and test file that this invocation will cite, and map each acceptance criterion to evidence actually read. Submit an already-satisfied result only after those reads are complete. Preserve correct existing code; do not create changes merely to satisfy progress detection.

### Dependencies

- APPR-003

### Deliverables

- `internal/cli/approval.go`, `internal/cli/approval_test.go` — the command, the wiring, request stamping, and the gate change.
- `internal/cli/approvals_test.go` — listing behavior for an invalidated request.

### Acceptance Criteria

- `sop approval invalidate <task-id> --reason R --by NAME` records the invalidation and reports the resulting boundary; a missing reason or actor is a usage error and records nothing.
- The command refuses, with an actionable message, when the evidence remains applicable, when the task is satisfied, and when the request is not pending.
- After a successful invalidation the task is still runnable and the next `sop run` executes it; invalidation alone never marks it approved, satisfied, complete, or skipped.
- A valid `PENDING` request still blocks execution, and a `DECLINED` request still blocks execution with its existing message; neither path changes.
- The approval listing distinguishes an invalidated request from a pending one and does not present it as an applicable gate.
- Newly raised requests record the definition digest they were raised against, so a future invalidation can decide applicability precisely.

### Acceptance Checks

- `go test ./internal/cli/...`
- `go vet ./internal/cli/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1
- criterion 6: check 1

### Failure Behavior

If stamping new requests would change existing request artifacts for callers that cannot supply a definition, the digest stays optional and an empty digest is treated as legacy, never as obsolescence.

## APPR-005 — Regression suite and documentation

Priority: P1. Prove the repaired deadlock end to end and document the operation, its limits, and the decision each resolution implies.

### Dependencies

- APPR-004

### Deliverables

- `internal/approval/invalidation_regression_test.go`, `internal/cli/approval_invalidation_test.go` — the reported deadlock scenarios, audit preservation, fresh verification, and refusal when evidence remains applicable.
- `docs/guides/APPROVALS.md`, `docs/specs/HUMAN-APPROVAL.md`, `docs/reference/CLI.md` — the operation, its guard conditions, and the audit record.

### Acceptance Criteria

- A regression test reproduces the reported deadlock: a reconciled, unsatisfied task with a stale pending request can be invalidated, and the next run then executes it and records a fresh outcome.
- A regression test proves the original request and its evidence survive verbatim in the append-only history, alongside the invalidation reason, actor, plan revisions, and definition digests.
- A regression test proves invalidation is refused when the current definition still yields the recorded digest, and when a legacy request has no reconciliation evidence covering it.
- A regression test proves an invalidated request leaves the task status, attempt count, and satisfaction unchanged, and that pending and declined requests still block execution exactly as before.
- Documentation states what invalidation does not do: it is not an approval, does not complete or satisfy a task, and requires re-execution and fresh verification.

### Acceptance Checks

- `go test ./internal/approval/... ./internal/cli/...`
- `go vet ./internal/approval/... ./internal/cli/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1

### Failure Behavior

If a scenario cannot be reproduced deterministically in a test, it is reported as unverified rather than asserted in documentation; the operation stays behind its guards until evidence exists.
