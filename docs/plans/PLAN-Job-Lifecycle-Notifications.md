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

- `internal/notify/event.go` adding `Kind` (`completed`, `failed`, `waiting_for_human`), `Event` (version, event id, kind, project, job id, task id, attempt, reason, next action, occurred-at, source transition), and `NewEvent`, which validates the event and computes its id as a SHA-256 over canonicalized identity fields so the same transition always yields the same id and a new attempt or job yields a new one.
- Payload rules: a `completed` event carries the job/task outcome and the next action; a `failed` event carries SOP's recorded reason and a next action an operator or agent can execute; a `waiting_for_human` event is produced only from a `PENDING` `domain.ApprovalRequest` and carries the approval request id, its reason, and the resolution command.
- `internal/config/notifications.go` plus `Config` wiring for a `notifications` block: channel selection (`none`, `file`, `webhook`), the channel's non-secret settings, and a bounded retry policy (max attempts, base delay, max delay, request timeout). A missing block means `none`.
- Config validation: an unknown channel, a channel enabled without its required settings, or a non-positive retry bound is a configuration error; a channel with no credentials configured is valid and reports `unavailable` at status time rather than failing the whole run.
- Redaction helpers so a channel's credential-bearing settings are never rendered: status exposes presence booleans and resolved non-secret values only.

### Acceptance Criteria

- Every event kind exists with the required fields, and a `waiting_for_human` event can carry both a job id and a task id when both are known.
- Event identity is deterministic: the same transition replayed yields the same event id, while a new attempt or a new job yields a different one.
- An absent `notifications` block yields channel `none`; a malformed block is rejected with an actionable configuration error.
- No configuration path renders a credential value; only presence is observable.

### Acceptance Checks

- `go test ./internal/notify/...`
- `go test ./internal/config/...`
- `go vet ./internal/notify/... ./internal/config/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 2
- criterion 4: check 1

### Failure Behavior

If the existing configuration loader cannot express a nested optional block without changing unrelated defaults, the `notifications` block is added as an independently loaded optional section with `none` as the absent default; existing configuration round-trips are preserved and covered by a regression test. The change is additive and reverted if it alters existing configuration defaults.

## NTFY-002 — Durable notification outbox with idempotent enqueue and leased claims

Priority: P1. Persist events and their per-channel delivery state in the existing SQLite state store, using unique keys for idempotency and transactional leases for safe concurrent workers. This follows the store's existing versioned migration pattern (`internal/store/sqlite.go`), which already migrates additively up to schema version 6.

### Dependencies

- NTFY-001

### Deliverables

- Schema migration 7 adding `notification_events` (stable event id primary key, kind, project, job id, task id, attempt, reason, next action, payload, created-at) and `notification_deliveries` (event id + channel primary key, status, attempt count, last error, next-attempt-at, delivered-at, acknowledgement classification, updated-at) with the indexes the worker needs.
- `internal/store/notifications.go`: `EnqueueNotification` (transactional `INSERT OR IGNORE`, so replay and concurrent enqueues of one event create exactly one event row), `ClaimNotificationDeliveries` (lease with a bounded takeover window so a crashed worker's claim is reclaimable), `RecordDeliveryOutcome`, and read operations for status counts.
- Delivery status vocabulary: `PENDING`, `IN_FLIGHT`, `DELIVERED`, `AMBIGUOUS`, `EXHAUSTED`, with `EXHAUSTED` reachable only after the configured maximum attempts and carrying the last error.
- Durable fields required by the issue: attempt count, last error, and next retry time are persisted per delivery and survive process restart.

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

- `internal/notify/channel.go`: `Channel` with `Name`, `Deliver(ctx, Event, idempotencyKey) (Ack, error)`, `Ack` values `Confirmed`, `Ambiguous`, and `Rejected`, and a `Capabilities` value stating whether the channel honors an idempotency key.
- A local `file` channel appending newline-delimited JSON events to an operator-configured path with an exclusive append, confirming only after the write is durably flushed. It exists so delivery can be verified end to end against a local receiver without any external service.
- A `webhook` channel performing one bounded HTTP POST per attempt with a stable `Idempotency-Key` header taken from the event identity, classifying `2xx` as `Confirmed`, `408`/`429`/`5xx`/timeout/transport error as `Ambiguous`, and other `4xx` as `Rejected`.
- A documented idempotency contract in package documentation and in the delivery-status documentation: at-least-once delivery with deduplication by stable event id; exactly-once is claimed only when the channel confirms its idempotency key, and an `AMBIGUOUS` acknowledgement is recorded as ambiguous rather than reported as either delivered or failed.
- No credential is written to logs, artifacts, or status output.

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

- `internal/notify/dispatch.go`: `Drain(ctx, Deps)`, which claims due deliveries, delivers each through the configured channel, and records the outcome — `DELIVERED` on confirmation, an incremented attempt with a bounded exponential next-attempt time on ambiguity or transient error, and `EXHAUSTED` once the configured maximum attempts is reached. Backoff is bounded by the configured maximum delay and jittered deterministically from the event id so retries of one event do not stampede.
- `sop notify status [--json]` reporting channel identity and non-secret settings, availability, per-status delivery counts, the most recent delivery error, and the next scheduled retry; `sop notify drain` running one bounded drain pass; `sop notify test` enqueuing and attempting one operator-authored test event so a channel can be verified without waiting for real work.
- Drain invocation from the completion path of `sop run` and `sop prompt`, best-effort and unable to affect their exit status, so retries resume on the next invocation after a restart.
- Delivery failures are isolated: every store write for delivery state is independent of task/job persistence, and an error in the notification path is reported as a notification error, never as a job or task failure.

### Acceptance Criteria

- A failed delivery persists its attempt count, last error, and next attempt time, and a fresh process drains it successfully after the retry time passes — with no in-memory state carried over.
- Backoff increases per attempt and is capped by the configured maximum; the number of attempts is bounded.
- An event whose attempts are spent reports `EXHAUSTED` in status together with its last error, and is not retried again.
- A notification delivery failure leaves the corresponding task status and job outcome unchanged.
- Draining with an unavailable or unconfigured channel reports that condition explicitly instead of silently succeeding.

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

### Failure Behavior

If the `sop` command surface cannot run a best-effort drain without changing an existing exit status or output contract, the drain is exposed only as `sop notify drain` and documented as the scheduling trigger; the automatic hook is dropped rather than weakening existing run semantics. Retries remain durable and visible either way.

## NTFY-005 — Emit events from authoritative lifecycle transitions

Priority: P1. Emit exactly one notification event per required lifecycle transition, derived from the transition SOP actually recorded, and cover execution failures that happen before any task result exists — including a background job whose process outlives the MCP server.

### Dependencies

- NTFY-002

### Deliverables

- Completion emission: the terminal task transitions to `LOCAL_DONE` and `DONE` enqueue a `completed` event carrying the project, task id, the job id when the task ran inside an MCP-started job, the outcome, and the next action.
- Failure emission: a task that reaches terminal `BLOCKED` enqueues a `failed` event carrying SOP's recorded `domain.BlockedReason` and blocked detail plus a next action — never a `waiting_for_human` event.
- Human-gate emission: `recordHumanApprovalRequest` in `internal/cli/approval.go` enqueues a `waiting_for_human` event only when SOP records a `PENDING` approval request, carrying the approval request id, its reason, and the recorded resolution command or next action. A `BLOCKED` or `NEEDS_HUMAN` status without an approval request emits nothing.
- Background job emission: the job's own process records its exit and enqueues the job `completed`/`failed` event, so a detached job that outlives the MCP server still notifies. Job completion reporting no longer depends on the server observing the child, and a job whose task result was never recorded still produces a `failed` event with the recorded exit code and log reference.
- Attempt identity: a new execution attempt yields its own event id, so a retried or re-run task notifies again, while re-reading state emits nothing new.

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

- An availability value derived from observed evidence rather than configuration: `unavailable` (no channel, or the channel's required capability is missing), `configured_unverified` (a channel is configured but no delivery has been confirmed), `active` (at least one delivery was confirmed on the current channel), and `unknown` (the channel could not be probed).
- Availability evidence in the response: channel name, resolved non-secret settings, whether credentials are present (boolean only), the last confirmed delivery time, pending and exhausted delivery counts, and the last delivery error.
- `sop status` gains a notifications line derived from the same service, and `sop_notify`/`sop_status` MCP tools report the same document; the read-only `sop_work_status` document gains a `notifications` object with this evidence alongside the job and task status.
- Redaction is enforced at the serialization boundary, so no status or MCP response can include a configured secret.

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

- The handoff rule, stated verbatim where agents read it, in the SOP agent instructions and the background-work handoff documentation: whenever starting background work, confirm whether notifications are active; if notifications are unavailable, say so explicitly and offer a scheduled status check; if status cannot be verified, report it as unverified; do not claim a status check is scheduled until scheduling succeeds.
- The `sop_start_work` tool description and its response document carry the notification evidence, and the response message states plainly when notifications are unavailable or unverified, so an agent cannot report success that the evidence does not support.
- Documentation in the CLI reference and README covering the configuration block, the delivery guarantees, the ambiguous-acknowledgement behavior, and the exact limitations of each channel — including that no production channel is enabled by this change.
- An operator runbook: how to configure a channel, how to verify it with `sop notify test`, how to read delivery status, and how to replay an exhausted delivery.

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

- Lifecycle tests: a completed job, a failed job, and a job waiting for a human each produce the correct event and payload, with both ids present when available.
- Read-only tests: repeated status reads and repeated drains create no new events and no new deliveries.
- Durability tests: a delivery failure persists attempt count, last error, and next attempt time, and a fresh worker process delivers after restart.
- Replay and concurrency tests: replaying an event and running two workers concurrently produce exactly one confirmed delivery per event per channel.
- Attempt tests: a new execution attempt produces its own event and notification.
- Ambiguity tests: an ambiguous acknowledgement follows the documented idempotency behavior and is reported as ambiguous, not as delivered.
- Configuration and exhaustion tests: missing configuration and unconfigured channels are reported explicitly, and exhausted retries are visible in status.
- Isolation tests: a notification failure does not alter a task's status or a job's outcome.
- Classification tests: a generic `BLOCKED` state without an approval request does not produce a human-action notification.
- A local-receiver end-to-end check driving a configured `file` channel through enqueue, drain, and status.

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
