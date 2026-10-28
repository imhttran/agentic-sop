# PLAN — Reliable SOP Job Lifecycle Notifications

**Status:** PROPOSED — implements GitHub issue #138 ("Primary fix: reliable SOP job lifecycle notifications").
**Rule:** SOP owns the job lifecycle and is authoritative for these events. This plan adds a durable notification outbox and delivery path around SOP's existing lifecycle transitions. It does not enable a production channel, and it never treats a generic `BLOCKED` task as a human-action request.

## Project

Reliable SOP Job Lifecycle Notifications

## Summary

Background work can complete, fail, or stop at a human gate without telling anyone. Today the MCP start-work path (`internal/cli/mcp_start_work.go`) records a job's exit only while the MCP server process is alive: a detached child that outlives the server is reported as `ended` with no exit code and no record of what it produced. Nothing delivers a machine-readable lifecycle event, and no interface answers "are notifications actually working?".

This plan makes SOP the primary fix. SOP already owns the authoritative transitions it needs:

- terminal task completion (`LOCAL_DONE`, `DONE`) recorded by the run loop in `internal/cli/drive.go`;
- terminal task failure (`BLOCKED` with a `domain.BlockedReason`) recorded by the same loop;
- a genuine human gate, which is already an explicit `domain.ApprovalRequest` written by `recordHumanApprovalRequest` in `internal/cli/approval.go` — never inferred from `BLOCKED`/`NEEDS_HUMAN` status;
- an MCP-started background job, recorded under `.agent-sdlc/runs/mcp-work/<job-id>/`.

The fix is one durable outbox in the existing SQLite state store plus a delivery worker over a configurable channel adapter:

```text
lifecycle transition (SOP-owned)
  └─ enqueue(Event)            stable event id, INSERT OR IGNORE   → notification_events
       └─ claim delivery        per-channel row, leased             → notification_deliveries
            └─ Channel.Deliver  Confirmed | Ambiguous | Rejected
                 └─ DELIVERED | retry with bounded backoff | EXHAUSTED
```

Delivery state is separate from execution state: a delivery failure never changes a task's status or a job's outcome. Availability is reported from observed delivery evidence, never from configuration alone, so an agent can follow the handoff rule with real evidence.

## NTFY-001 — Lifecycle event model, stable identity, and configuration

Priority: P1. Define SOP's notification event as a first-class domain value, derive a stable event identity from the transition it describes, and add the operator configuration that names a channel and its bounded retry policy. Existing configuration loading and validation patterns in `internal/config` are reused; no new dependency is added.

### Dependencies

None

### Deliverables

- `internal/notify/event.go`, `internal/notify/event_test.go` — the event kinds, the event value, and `NewEvent`, which validates the payload and computes the event id.
- `internal/config/notifications.go`, `internal/config/notifications_test.go` — the `notifications` block, its defaults and validation, and the credential-presence status helpers.
- `internal/config/config.go` — `Config` wiring that resolves an omitted block to channel `none`.

### Acceptance Criteria

- Every event kind exists with the required fields, and a `waiting_for_human` event can carry both a job id and a task id when both are known.
- Event identity is deterministic: the same transition replayed yields the same event id, while a new attempt or a new job yields a different one.
- An absent `notifications` block yields channel `none`; a malformed block is rejected with an actionable configuration error.
- No configuration path renders a credential value; only presence is observable.
- Each kind enforces its payload rules when the event is constructed: a `completed` event requires the outcome and a next action; a `failed` event requires the recorded reason and a next action; a `waiting_for_human` event requires a `PENDING` approval request and carries that request's id, its reason, and a resolution command, and a non-`PENDING` request is rejected.

### Acceptance Checks

- `go test ./internal/notify/...`
- `go test ./internal/config/...`
- `go vet ./internal/notify/... ./internal/config/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 1
- criterion 5: check 1

### Failure Behavior

If the existing configuration loader cannot express a nested optional block without changing unrelated defaults, the `notifications` block is added as an independently loaded optional section with `none` as the absent default; existing configuration round-trips are preserved and covered by a regression test. The change is additive and reverted if it alters existing configuration defaults.

## NTFY-002 — Durable notification outbox with idempotent enqueue and leased claims

Priority: P1. Persist events and their per-channel delivery state in the existing SQLite state store, using unique keys for idempotency and transactional leases for safe concurrent workers. This follows the store's existing versioned migration pattern (`internal/store/sqlite.go`), which already migrates additively up to schema version 6.

### Dependencies

- NTFY-001

### Deliverables

- `internal/store/sqlite.go` — schema migration 7 adding the `notification_events` and `notification_deliveries` tables with their indexes.
- `internal/store/notifications.go`, `internal/store/notifications_test.go` — `EnqueueNotification`, `ClaimNotificationDeliveries`, `RecordDeliveryOutcome`, and the status-count reads.

### Acceptance Criteria

- Enqueuing the same event twice, concurrently or after a restart, produces one event row and one delivery row per channel.
- A second worker cannot claim a delivery another worker holds, and a stale claim becomes claimable again after the lease expires.
- Attempt count, last error, and next attempt time round-trip through the database and are readable after reopening the store.
- A delivery marked `EXHAUSTED` is never claimed again.

### Acceptance Checks

- `go test ./internal/store/...`
- `go test -race ./internal/store/...`
- `go vet ./internal/store/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 1
- criterion 4: check 1

### Failure Behavior

If the SQLite driver lacks a needed `RETURNING` capability, claims are performed as a transaction of `SELECT` then `UPDATE ... WHERE status = ? AND next_attempt_at <= ?`, verifying the affected row count; correctness is never traded for a convenience query. The migration is additive; an interrupted migration leaves the previous schema version in place, matching the existing migration contract.

## NTFY-003 — Delivery channel adapters with explicit acknowledgement semantics

Priority: P1. Deliver one event through one configured channel behind a narrow adapter interface, and classify the channel's acknowledgement honestly. Duplicate-free delivery is claimed only where the channel proves it.

### Dependencies

- NTFY-001

### Deliverables

- `internal/notify/channel.go`, `internal/notify/channel_test.go` — the `Channel` interface, the `Ack` vocabulary, the `Capabilities` value, and the idempotency contract in the package documentation.
- `internal/notify/file.go` — the local `file` channel.
- `internal/notify/webhook.go` — the `webhook` channel.

### Acceptance Criteria

- Each adapter maps its transport outcome to the documented `Ack` value, and a `Rejected` outcome is distinguishable from a transient `Ambiguous` one.
- The `file` channel confirms delivery only when the event is durably appended, and the local receiver's content is parseable as the event payload.
- The webhook adapter sends the same idempotency key on every retry of one event.
- The documented guarantee matches tested behavior: ambiguous acknowledgements are recorded as ambiguous, and no test or document claims exactly-once delivery for a channel that does not honor the key.

### Acceptance Checks

- `go test ./internal/notify/...`
- `go test -race ./internal/notify/...`
- `go vet ./internal/notify/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 1

### Failure Behavior

Network access is never required for the default path: with channel `none` nothing is delivered and status says so. If a webhook cannot be reached, the delivery is retried with backoff and eventually `EXHAUSTED`, and the execution result is untouched.

## NTFY-004 — Delivery worker with bounded backoff, restart-safe retry, and operator status

Priority: P1. Drain due deliveries with bounded exponential backoff, expose exhausted retries, and make retries happen without a long-lived daemon: the worker runs after a `sop run`/`sop prompt` process finishes and through an explicit `sop notify` command that also serves as the scheduled and post-restart trigger.

### Dependencies

- NTFY-002
- NTFY-003

### Deliverables

- `internal/notify/dispatch.go`, `internal/notify/dispatch_test.go` — `Drain` with bounded exponential backoff and the exhausted-retry transition.
- `internal/cli/notify.go`, `internal/cli/notify_test.go` — the `sop notify status`, `sop notify drain`, and `sop notify test` operator surface.
- `internal/cli/run.go`, `internal/cli/prompt.go` — the best-effort drain hook on the command completion path.

### Acceptance Criteria

- A failed delivery persists its attempt count, last error, and next attempt time, and a fresh process drains it successfully after the retry time passes — with no in-memory state carried over.
- Backoff increases per attempt and is capped by the configured maximum; the number of attempts is bounded.
- An event whose attempts are spent reports `EXHAUSTED` in status together with its last error, and is not retried again.
- A notification delivery failure leaves the corresponding task status and job outcome unchanged.
- Draining with an unavailable or unconfigured channel reports that condition explicitly instead of silently succeeding.
- The operator surface exposes one bounded drain pass, a status report, and a test event, and the drain hook never changes the exit status or output of the command that triggered it.

### Acceptance Checks

- `go test ./internal/notify/...`
- `go test ./internal/cli/...`
- `go vet ./internal/notify/... ./internal/cli/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 2
- criterion 5: check 2
- criterion 6: check 2

### Failure Behavior

If the `sop` command surface cannot run a best-effort drain without changing an existing exit status or output contract, the drain is exposed only as `sop notify drain` and documented as the scheduling trigger; the automatic hook is dropped rather than weakening existing run semantics. Retries remain durable and visible either way.

## NTFY-005 — Emit events from authoritative lifecycle transitions

Priority: P1. Emit exactly one notification event per required lifecycle transition, derived from the transition SOP actually recorded, and cover execution failures that happen before any task result exists — including a background job whose process outlives the MCP server.

### Dependencies

- NTFY-002

### Deliverables

- `internal/cli/lifecycle_notify.go`, `internal/cli/lifecycle_notify_test.go` — the emitter that turns one recorded transition into one event.
- `internal/cli/approval.go` — human-gate emission from the recorded approval request.
- `internal/cli/drive.go` — completion and failure emission at the terminal task transitions.
- `internal/cli/mcp_start_work.go` — background-job emission from the job's own process.

### Acceptance Criteria

- Each of the three required transitions produces exactly one event of the correct kind with project, task/job id, reason, and next action populated.
- A generic `BLOCKED` task with no approval request produces a `failed` event and never a `waiting_for_human` event; a task parked at an explicit pending approval request produces `waiting_for_human` with that request's identity.
- Repeated status reads, repeated drains, and a replayed run stage create no new events.
- A new execution attempt, and a job re-run, each produce their own event.
- A background job that fails before recording a task result still produces a `failed` event.

### Acceptance Checks

- `go test ./internal/cli/...`
- `go build ./...`
- `go vet ./internal/cli/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1

### Failure Behavior

If emitting from the job's own process would change how `sop run`/`sop prompt` behave when launched by an operator, emission is gated on the job environment being present and is otherwise a no-op; ordinary foreground invocations keep identical behavior and output.

## NTFY-006 — Expose availability and delivery status; no credential exposure

Priority: P1. Answer "are notifications active?" and "was this event delivered?" through SOP's status and MCP interfaces with observed evidence, so a caller can decide whether to rely on notifications before handing off background work.

### Dependencies

- NTFY-004
- NTFY-005

### Deliverables

- `internal/cli/notify.go`, `internal/cli/notify_status_test.go` — the availability value, its evidence, and the serialization-boundary redaction.
- `internal/cli/mcp.go` — the notification document on the status MCP tools.
- `internal/cli/mcp_start_work.go` — the `notifications` object in the work-status document.

### Acceptance Criteria

- Configuration alone never reports `active`: a configured channel with no confirmed delivery reports `configured_unverified`.
- A confirmed delivery moves availability to `active`; removing configuration reports `unavailable` with an actionable reason.
- Pending and exhausted delivery counts and the last error are visible through both the CLI status and the MCP status document.
- No response contains a configured credential value; only its presence is reported.
- Repeated status reads create no events and mutate no delivery state.

### Acceptance Checks

- `go test ./internal/cli/...`
- `go test ./internal/mcp/...`
- `go vet ./internal/cli/... ./internal/mcp/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 1
- criterion 5: check 1

### Failure Behavior

Status commands stay read-only: if delivery state cannot be read, the status reports `unknown` with the read error rather than failing the command or creating state.

## NTFY-007 — Agent handoff rule and documentation

Priority: P1. Add the required handoff rule wherever an agent is told how to start background work, and make the status response carry enough evidence for an agent to follow it truthfully.

### Dependencies

- NTFY-006

### Deliverables

- `skills/sop/SKILL.md` — the handoff rule in the agent instructions.
- `docs/reference/CLI.md`, `README.md` — the configuration, the delivery guarantees, the channel limitations, and the no-production-channel statement.
- `docs/guides/NOTIFICATIONS.md` — the operator runbook.
- `internal/cli/mcp_start_work.go` — the tool description and the response evidence the rule depends on.

### Acceptance Criteria

- The handoff rule appears verbatim in the agent instructions and the handoff documentation, and it matches the implemented availability semantics.
- The start-work response includes the evidence the rule depends on, and distinguishes `unavailable`, `configured_unverified`, `active`, and `unknown`.
- Documentation states delivery guarantees per channel and does not claim duplicate-free delivery where the channel does not support it.
- Documentation states that no production notification channel is enabled by this change.

### Acceptance Checks

- `go test ./internal/cli/...`
- `go build ./...`
- `go vet ./internal/cli/...`

### Acceptance Mapping

- criterion 1: check 2
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 2

### Failure Behavior

If the agent instruction files are generated from a template, the rule is added to the source template and regenerated; a hand-edit that would be overwritten is not accepted. Missing documentation fails the doc-link check rather than being silently skipped.

## NTFY-008 — Acceptance verification for the issue's scenarios

Priority: P1. Add the focused tests the issue requires, including the failure, retry, restart, concurrency, ambiguity, and non-notification cases, and run them against a local receiver.

### Dependencies

- NTFY-005
- NTFY-006
- NTFY-007

### Deliverables

- `internal/notify/lifecycle_acceptance_test.go` — lifecycle, read-only, attempt, ambiguity, and classification scenarios.
- `internal/store/notify_acceptance_test.go` — durability, replay, restart, and concurrent-worker scenarios.
- `internal/cli/notify_acceptance_test.go` — isolation, configuration and exhaustion visibility, and the local-receiver end-to-end check.

### Acceptance Criteria

- Every scenario above has a focused, named test that fails if the behavior regresses.
- The tests run without network access and without a production notification channel.
- The repository's required checks pass: `make check` and the race-enabled test run used in CI.

### Acceptance Checks

- `go test ./internal/notify/... ./internal/store/... ./internal/cli/...`
- `go test -race ./internal/notify/... ./internal/store/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 3

### Validation Commands

`make check`; `go test -race -count=1 ./...`; `bash scripts/checks/check-doc-links.sh`; `git diff --check`.

### Failure Behavior

If an existing test becomes flaky under the race detector, the new tests are isolated from it and the pre-existing failure is reported rather than masked; a scenario that cannot be verified is reported as unverified instead of being asserted in prose.
