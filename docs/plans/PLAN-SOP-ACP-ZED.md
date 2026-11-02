# Plan: SOP ACP integration for Zed

Date: 2026-11-01
Status: Proposed; implementation in progress (ACP-001 and ACP-002 delivered, ACP-003 through ACP-007 outstanding)
Repository: https://github.com/imhttran/agentic-sop
Suggested repository path: docs/plans/PLAN-SOP-ACP-ZED.md

## Project

SOP ACP integration for Zed

## Summary

Add `sop acp` so a user can select SOP as an external agent in Zed, submit a request, observe governed execution, inspect blockers and evidence, and cancel work safely.

SOP remains the authority for execution, lifecycle state, verification, retries, and approvals. ACP provides an editor interface. The existing CLI and MCP interfaces remain supported.

Initial release scope: a local stdio ACP endpoint in the existing SOP executable; text prompts, session/project binding, progress updates, final evidence, and actionable blockers; existing SOP provider configuration, execution boundaries, project locks, and verification gates; one active execution per project, including executions started through CLI or MCP; safe cancellation, disconnect handling, and duplicate-start prevention; Zed setup documentation and macOS/Linux acceptance evidence.

Deferred: remote ACP transport, registry publication, and automatic editor installation; restoring complete ACP conversation history after restart; editor-mediated file writes or terminal execution; forwarding arbitrary client-provided MCP servers to executors; new approval policy, automatic approval, or bypasses of existing gates; general conversational memory, attachments, and model selection UI.

## ACP-001 — Inspect boundaries and record implementation decisions

Read repository instructions and SOP boundary documentation. Inspect worktrees and active plans before changing anything. Pin a source revision for the assessment and record target Zed versions. Inspect prompt submission, background work, locking, progress events, cancellation, approvals, and notification delivery. Identify the smallest shared service API and the test seams required by the adapter. Select the protocol version and library. Record SDK licensing, maintenance, and compatibility. Define execution ownership, disconnect policy, cancellation bounds, and behavior for concurrent prompts. Load this plan through the established SOP planning workflow before implementation. Preserve active plans and user-owned changes.

### Dependencies

None

### Deliverables

- `docs/plans/ACP-001-IMPLEMENTATION-NOTE.md` — the implementation note naming actual source interfaces, protocol and dependency versions, gaps, and the chosen lifecycle semantics.
- `internal/archtest/` — the repository contract tests, including the note's deterministic section guard.

### Acceptance Criteria

- The implementation note names actual source interfaces, protocol and dependency versions, gaps, and the chosen lifecycle semantics.
- No speculative interface is treated as implemented.
- The note stays a checked repository artifact: its required decision sections are enforced by a deterministic test.

### Acceptance Checks

- `go test ./internal/archtest/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## ACP-002 — Add ACP transport and protocol negotiation

Add the `sop acp` subcommand to the existing SOP executable. Implement stdio framing, `initialize`, and `session/new` for the pinned protocol. Advertise only supported capabilities and reject unsupported input with actionable errors. Keep stdout protocol-only; route diagnostics to stderr with credential redaction. Bound input sizes and event buffering. Handle invalid JSON, unknown methods, EOF, and incompatible versions.

### Dependencies

ACP-001

### Deliverables

- `internal/acp/` — the transport package: contract constants, framing, `initialize`, `session/new`, the protocol-only writer, diagnostics, and the deterministic protocol test suite.
- `internal/cli/acp.go` — the `sop acp` subcommand wiring.

### Acceptance Criteria

- A protocol client completes initialization and session creation.
- Malformed requests cannot crash or desynchronize the process.
- Diagnostic output does not corrupt frames.
- The pinned protocol version is negotiated and an incompatible version is rejected with an actionable error.

### Acceptance Checks

- `go test ./internal/acp/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1

## ACP-003 — Bind sessions and submit governed work

Validate and canonicalize the session project directory without changing process-global cwd per session. Map session, submission, job, task, and attempt identifiers explicitly. Route text prompts through the existing governed prompt execution service. Reuse project locking across ACP, CLI, and MCP. Return a useful busy result rather than queueing conflicting work implicitly. Prevent accidental duplicate submissions within the adapter; do not treat JSON-RPC IDs as durable idempotency keys. Return job identity for reconciliation after an ambiguous disconnect. Never retry a start without checking persisted state.

### Dependencies

ACP-002

### Execution Mode

- verify-first

### Deliverables

- `internal/acp/` — session binding and governed prompt submission with explicit identifier mapping and duplicate-start prevention.
- `internal/cli/acp.go` — wiring so the served endpoint reaches the governed submission path.
- `internal/acp/` — a deterministic `TestPromptSubmission` covering one-work-item-per-submission, project isolation, the busy-lock result, and ambiguous-start reconciliation.

### Acceptance Criteria

- One accepted submission creates one work item.
- Cross-project sessions stay isolated.
- Competing CLI/MCP/ACP starts obey the same lock.
- Unsupported content cannot start partial work.
- A failed or ambiguous start is reconciled against persisted state before any new submission.

### Acceptance Checks

- `go test -run TestPromptSubmission ./internal/acp/...`
- `go test ./internal/cli/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1
- criterion 4: check 1
- criterion 5: check 1

## ACP-004 — Stream progress and evidence

Translate structured SOP progress into `session/update` notifications supported by the pinned protocol. Display task identity, current stage, verification outcome, blockers, and final evidence references. Preserve the distinction between a finished prompt turn, LOCAL_DONE, verified completion, and a task waiting for approval. Bound slow-client buffering and preserve terminal outcomes even if intermediate progress is coalesced. Use protocol-defined stop reasons; do not invent new protocol status values for SOP states.

### Dependencies

ACP-003

### Execution Mode

- verify-first

### Deliverables

- `internal/acp/` — structured progress translation into session/update frames with bounded buffering and terminal-outcome preservation.
- `internal/acp/` — a deterministic `TestProgress` covering success, verification failure, and blocked execution, plus slow-client coalescing.

### Acceptance Criteria

- Success, verification failure, and blocked execution produce accurate UI results matching persisted SOP evidence.
- A transport response never labels unfinished work complete.
- A slow client cannot grow server memory without limit, and the terminal outcome survives coalescing.

### Acceptance Checks

- `go test -run TestProgress ./internal/acp/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## ACP-005 — Expose questions and preserve approvals

Display planner questions with identifiers and the existing supported response procedure. Display approval blockers and the precise operator action needed to resolve them. Keep policy approval in the existing SOP approval interface for the initial release. Do not interpret free-form chat, an ACP permission grant, or a planner recommendation as approval. Stop at an unchanged approval blocker rather than retrying it. After external resolution, re-read authoritative state and use the existing continuation contract.

### Dependencies

ACP-004

### Deliverables

- `internal/acp/` — planner-question and approval-blocker presentation with the existing resolution procedure.
- `internal/acp/` — a deterministic `TestApproval` covering an unchanged blocker, a stale response, and external resolution requiring a fresh state read.

### Acceptance Criteria

- Pending approvals remain pending until resolved through SOP.
- Stale or unrelated responses cannot authorize work.
- Repeated prompts cannot consume retries against an unchanged approval gate.

### Acceptance Checks

- `go test -run TestApproval ./internal/acp/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## ACP-006 — Implement cancellation and disconnect recovery

Implement `session/cancel` according to the pinned protocol. Cancel only adapter-owned execution through the supported SOP mechanism; if that mechanism is missing, implement and verify it before claiming cancellation support. Stop owned subprocesses within a documented bound and preserve partial evidence through existing lifecycle transitions. Recommended initial disconnect policy: request safe cancellation of foreground work on EOF, then reconcile the final state; confirm this can be enforced under adapter termination, otherwise establish a supported supervisor or lease mechanism before release. Keep explicit cancellation distinct from failure and successful completion in SOP evidence, mapped to protocol-defined responses. On restart, expose persisted job status through the existing status path; do not require conversation-history restoration and do not restart work automatically. Preserve existing notification behavior and never emit a second lifecycle event solely because the ACP UI observed an outcome.

### Dependencies

- ACP-003
- ACP-004
- ACP-005

### Execution Mode

- verify-first

### Deliverables

- `internal/acp/` — session/cancel, EOF handling, and disconnect reconciliation over the supported SOP cancellation path.
- `internal/acp/` — a deterministic `TestCancel` covering cancel, EOF, cancellation racing completion, and restart discovery without automatic resubmission.

### Acceptance Criteria

- Cancel, EOF, process termination, and restart demonstrate no duplicate execution, false completion, abandoned lock, or unowned running child.
- Cancellation racing with normal completion produces one authoritative result.
- An explicit cancellation is recorded distinctly from failure and from completion.

### Acceptance Checks

- `go test -run TestCancel ./internal/acp/...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## ACP-007 — Integrate with Zed and release through PR review

Document adding a custom external agent in Zed and setting its executable to the installed SOP binary with the proposed `acp` argument. Verify the actual `agent_servers` schema against the supported Zed version before publishing configuration examples. Document provider environment inheritance, project selection, logs, blockers, cancellation, and recovery. Run focused adapter tests and repository-required checks once for the final change; use the established PR workflow for remaining review and verification. Run bounded real-Zed acceptance on macOS and Linux, recording versions, revision, job IDs, outcomes, and evidence paths. Open a PR with scope, compatibility, lifecycle decisions, validation evidence, and known limitations, following existing merge and release governance.

### Dependencies

- ACP-001
- ACP-002
- ACP-003
- ACP-004
- ACP-005
- ACP-006

### Deliverables

- `docs/guides/ACP-ZED.md` — the Zed setup and operations guide: `agent_servers` configuration, provider environment, project selection, logs, blockers, cancellation, and recovery.
- `internal/acp/` — the adapter package in its final, documented state.

### Acceptance Criteria

- SOP appears in Zed, a bounded request completes with evidence, all release scenarios pass, and existing CLI/MCP behavior remains compatible.
- The setup guide names the supported Zed configuration schema, provider environment inheritance, and known limitations.
- macOS acceptance is recorded as outstanding until it is executed on macOS; Linux acceptance records versions, revision, job IDs, outcomes, and evidence paths.

### Acceptance Checks

- `go test ./internal/acp/...`
- `go build ./...`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 1
- criterion 3: check 1

## Evidence and assumptions

The repository README inspected during planning documents `sop prompt`, planner integration, background work, and notifications. The ACP documentation inspected during planning labels v1 as latest and v2 as draft. Verify compatibility against the target Zed version and pin the selected protocol and dependency versions; do not implement draft behavior accidentally.

## Architecture and contracts

| Layer | Owns |
| --- | --- |
| Zed | Thread UI, user input, progress rendering |
| ACP adapter | Protocol validation, session mapping, event translation, cancellation requests |
| Shared SOP services | Work submission, project locking, persisted jobs/tasks, lifecycle, approval checks |
| Existing executors | Bounded implementation and evidence production |

Prefer shared internal services already used by CLI/MCP. Extract a small service boundary only where necessary. Do not create a second scheduler or parse human-readable CLI output. Select a maintained Go ACP library if it meets the pinned protocol and dependency requirements; otherwise document the cost of a minimal transport implementation.

Required invariants:

1. ACP messages cannot directly mutate task lifecycle records or bypass governance.
2. A UI permission response is not an SOP policy approval.
3. A prompt response or transport success is not proof of task completion.
4. Only structured, persisted SOP outcomes determine the reported execution result.
5. A disconnect or ambiguous start outcome never triggers automatic resubmission.
6. Logs and child output cannot contaminate protocol stdout or expose credentials.
7. Project identity is canonicalized and validated; sessions cannot access another project's state.
8. Existing isolation policy applies to work started from Zed.

## Acceptance matrix

| Scenario | Required evidence |
| --- | --- |
| Successful bounded task | UI outcome agrees with persisted task and verification state |
| Failed verification | Failure and actionable evidence shown; no success promotion |
| Approval required | Execution stops; gate persists until authorized SOP resolution |
| Explicit cancellation | Owned work stops within the documented bound; partial evidence retained |
| Client disconnect | Documented stop policy enforced; no orphan execution or automatic restart |
| Adapter restart | Existing job is discoverable; no duplicate submission |
| Concurrent starts | Existing project lock protects CLI, MCP, and ACP equally |
| Multiple projects | No shared cwd, task, event, or evidence leakage |
| Malformed or oversized input | Bounded error handling; protocol remains usable or exits cleanly |
| Slow client | Memory stays bounded and final outcome remains recoverable |
| Cancellation/completion race | Exactly one authoritative final execution result |
| Existing interfaces | Required CLI/MCP regression checks pass |

Use a deterministic fake executor for protocol and lifecycle edge cases. Use real model execution only for the bounded end-to-end smoke. Do not repeatedly run broad manual suites after passing required checks unless new changes or failures justify it.

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| ACP version drift | Pin versions and test the exact supported Zed releases |
| Missing reusable lifecycle APIs | Extract the minimum shared service; retain SOP as sole authority |
| Duplicate work after disconnect | Persist correlation IDs and reconcile before any new submission |
| Approval semantics confused with UI permissions | Keep policy approval in the existing SOP interface initially |
| Child processes survive editor shutdown | Verify ownership and cancellation under EOF and process termination |
| Unstructured CLI output leaks into ACP | Dedicated protocol writer; separate diagnostics and structured events |
| Zed starts with a different environment | Document provider configuration and test GUI-launched Zed |

## Rollout and rollback

Ship the adapter as an opt-in interface. Start with a disposable project and bounded work, then validate a normal project. Keep MCP available throughout rollout.

To roll back, stop or reconcile active ACP-owned jobs through SOP, remove the custom Zed agent entry, and return to CLI/MCP. Preserve task history and evidence. Do not reset lifecycle state or delete worktrees as part of rollback.

## Definition of done

- ACP-001 through ACP-007 have evidence satisfying their acceptance criteria.
- The acceptance matrix passes on the supported environments.
- SOP lifecycle, isolation, approval, verification, and retry ownership are preserved.
- Documentation names supported versions and known limitations.
- The implementation completes the established PR review and release gates.

## References

- SOP overview: https://github.com/imhttran/agentic-sop/blob/main/README.md
- SOP ownership boundary: https://github.com/imhttran/agentic-sop/blob/main/docs/architecture/SOP-BOUNDARY.md
- ACP v1 overview: https://agentclientprotocol.com/protocol/v1/overview
- Zed external agents: https://zed.dev/docs/ai/external-agents

References informed planning. Recheck version-sensitive details during ACP-001. This document is a proposed implementation plan, not evidence of implemented functionality or completed verification.
