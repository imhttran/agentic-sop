# SOP planner connection spike

Date: October 13, 2026 • Status: proposed, not executed

## Project

SOP planner connection spike (agentic-sop repository).

## Summary

Prove that an actual planner client can communicate with a local SOP runner,
receive a task question, submit a bounded decision, and resume execution without
bypassing human approval. Establish whether the chosen connection supports
interactive use, unattended use, or neither.

This is a feasibility spike. Success means one small task completes two
planner-question rounds and survives a restart. It does not mean the complete
multi-PR workflow is ready.

## Scope and boundaries

Use a dedicated branch and isolated worktree based on the current intended
development baseline. Inspect active migration work before choosing that
baseline; do not modify or interrupt it. Use a disposable target repository and
fake executor for deterministic fault tests, plus one real planner-client test.
Keep all prototype changes and evidence scoped to the spike.

Include connection discovery, existing MCP status calls, minimal structured
question/decision services, durable waiting, explicit continuation, and targeted
tests. Exclude task-to-PR partitioning, parallel workers, automatic
commits/pushes/PRs, merge, deployment, dashboard development, and a
general-purpose background service.

SOP remains the state and policy authority. The planner has no human-approval
permission. Keep ordinary execution behavior unchanged unless the spike feature
is explicitly enabled. Do not turn an existing human gate or exhausted budget
into a planner question.

## Connection decision

The first target is the planner the operator intends to use. Record whether that
means a ChatGPT conversation, a Claude conversation, Claude Code, or an agent
hosted in Zed; these are different clients. A Zed agent calling SOP proves Zed
connectivity, not connectivity from the intended external planner; do not
silently substitute a Zed agent for the intended external client.

Start with SOP's existing stdio MCP surface only when the selected client
supports it. If the actual client requires a different transport, record that gap
before proposing a narrowly scoped bridge. Do not publish a local service to the
internet or invent authentication as a shortcut.

No-response and disconnected-client behavior must be explicit. An MCP server does
not itself wake a conversation. For this spike, active-client polling or explicit
check-and-resume is sufficient for interactive success. Unattended success
requires a separately demonstrated invocation mechanism and a runner that remains
alive without Zed; it is not assumed or required for the first gate.

Use a fake planner only for deterministic tests. A manual answer or fake planner
cannot satisfy the real-client connection gate. If another client succeeds while
the intended client fails, report a partial result and name both.

## Conditional gate

Stages SPIKE-003 through SPIKE-006 are conditional. Proceed only after SPIKE-002
connectivity and SPIKE-003 planner approval-tool isolation are verified. An
unsupported connection is a valid spike finding and must not be recorded as
successful integration.

## Capabilities

### SOP MCP stdio server — EXISTS

- Status: EXISTS
- Location: internal/mcp, internal/cli/mcp.go (`sop mcp`)
- Evidence: the repository ships a stdio MCP server and an existing status tool.
- Owner: SOP

### Git worktree tooling — EXISTS

- Status: EXISTS
- Location: repository development environment (`git worktree`)
- Evidence: linked worktrees are already used in this repository (for example sop-harden-001/).
- Owner: SOP

### Local SOP CLI — EXISTS

- Status: EXISTS
- Location: cmd/sop; installed on PATH
- Evidence: `sop version` and `sop status` run against this project.
- Owner: SOP

### Operator-designated external planner client — MISSING

- Status: MISSING
- Gap: the repository does not provide a planner client; the operator supplies and designates the intended external client.
- Owner: operator
- Resolution: SPIKE-002 discovery identifies the intended client and verifies access and transport; if it cannot be reached, the limitation is recorded and the outcome is PARTIAL/NO-GO.

### Operator-designated real executor client — UNKNOWN

- Status: UNKNOWN
- Gap: only exercised if a real executor is available for the optional repeat in SPIKE-006.
- Owner: operator

## Assumptions

### The intended planner client is operator-designated and external (a ChatGPT conversation, a Claude conversation, Claude Code, or a Zed-hosted agent); it is never silently substituted.

- Evidence: this plan's Connection decision section.
- Consequence: a Zed-agent-only result proves Zed connectivity and is reported as PARTIAL, not as the external-client gate.

### SOP remains the state and policy authority and the planner role holds no human-approval permission.

- Evidence: docs/architecture/SOP-BOUNDARY.md and docs/specs/HUMAN-APPROVAL.md.
- Consequence: approval-tool isolation is a required acceptance check; no planner answer may cross a human gate.

### Ordinary execution behavior is unchanged unless the spike feature is explicitly enabled.

- Evidence: .agent-sdlc/config.yaml (workflow.mode: local; human.approval_before_commit: true).
- Consequence: disabled mode preserves prior execution behavior.

## SPIKE-001 — Baseline inventory and disposable test environment

Inspect the current SOP version, branch, working tree, MCP registration, run
stages, recovery, configuration, and relevant instructions, and confirm
global-policy propagation once for participating agents. Create the disposable
target repository and an isolated worktree on a dedicated branch, and establish
run-artifact log capture, using available tools. Record the baseline commit,
candidate client/version and supported transport, an ownership diagram, and the
existing tools. Assume no compatibility.

### Requires

- Git worktree tooling
- Local SOP CLI

### Deliverables

- Recorded baseline commit and current branch/working-tree state.
- Ownership diagram and the existing tools/MCP registration.
- Disposable target repository and isolated worktree on a dedicated branch.
- Run-artifact log capture path.

### Acceptance Criteria

- Baseline commit recorded; no compatibility assumed.
- Client/version candidates and supported transport recorded.
- Disposable repository and worktree created; log capture verified.
- Global-policy propagation confirmed once for participating agents.

## SPIKE-002 — Planner-client and transport discovery

Identify the operator-intended planner client and verify access and supported
transport; do not silently substitute a Zed agent for the intended external
client. Connect the selected planner to the existing SOP MCP server and exercise
protocol initialization, tool listing, and the existing status tool against the
disposable repository, twice including after a reconnect. Record evidence. Stop
transport work if the connection is unsupported and record the exact limitation.

### Dependencies

- SPIKE-001

### Requires

- SOP MCP stdio server

### Acceptance Criteria

- A real intended client retrieves the correct repository status twice, including after a reconnect.
- Or the exact unsupported-transport limitation is recorded.
- The intended client is named; no substitution and no assumed compatibility.

## SPIKE-003 — Durable question and decision contract

Define and implement the smallest durable question/decision contract using
existing SOP services and persistence patterns, with narrowly scoped CLI/MCP
access. Keep PENDING, ANSWERED, APPLIED, REJECTED, and SUPERSEDED question states
separate from the run's WAITING_FOR_PLANNER stage. Record application before
dispatching continuation. Proceed only after SPIKE-002 connectivity is verified.

### Dependencies

- SPIKE-001
- SPIKE-002

### Requires

- SOP MCP stdio server

### Acceptance Criteria

- Question and answer are independently recorded.
- Duplicate and stale submissions are handled deterministically.
- No approval tools are exposed to the planner role.

## SPIKE-004 — WAITING_FOR_PLANNER stage and explicit continuation

Wire a structured executor question to a persisted WAITING_FOR_PLANNER stage and
explicit continuation, retaining the existing execution stage and failure
budgets. Clarification consumes a planner round, not a failure retry.

### Dependencies

- SPIKE-003

### Acceptance Criteria

- A task pauses without retrying.
- A validated planner answer resumes through normal checks.
- The loop repeats for a second question.

## SPIKE-005 — Restart, timeout, stale-state, duplicate, and authority tests

Run restart, timeout, stale-state, duplicate-delivery, and authority tests, using
a scripted executor for deterministic coverage and an injected clock for the
timeout and deadline tests.

### Dependencies

- SPIKE-004

### Acceptance Criteria

- Targeted test evidence proves no duplicate application, implicit approval, premature completion, or budget reset.
- Timeout, disconnect, and exhausted planner rounds preserve an explicit waiting/paused condition and evidence.
- Existing failure counters are unchanged by clarification; disabled mode preserves prior execution behavior.

## SPIKE-006 — Live demonstration, transcript capture, and feasibility report

Demonstrate the complete loop through the actual intended planner client and the
local runner, capturing a real-client transcript and machine evidence. The live
demonstration and transcript capture are work to perform, not a precondition.
Write the feasibility report, limitations, and recommended next phase with a
clear GO / PARTIAL / NO-GO outcome. Proceed only after SPIKE-002 connectivity and
SPIKE-003 approval-tool isolation are verified; an unsupported connection is a
valid spike finding and must not be recorded as successful integration.

### Dependencies

- SPIKE-002
- SPIKE-003
- SPIKE-005

### Acceptance Criteria

- Real-client transcript plus machine evidence, limitations, and a clear GO / PARTIAL / NO-GO outcome.
- The report identifies real versus simulated components, transport limitations, and whether a session must remain active.
- A Zed-agent-only result is reported as PARTIAL, not as the external-client gate.

## Minimal contract

| Record             | Required fields                                                                                                                                                                                |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Question           | Schema version, repository/run/task identity, question ID, plan revision, relevant repository-state fingerprint, return stage, question, options, evidence references, creation time, deadline |
| Decision           | Decision ID/idempotency key, question ID, expected revision/state, planner identity, chosen action, rationale, submission time                                                                 |
| Application result | Accepted/rejected/stale/duplicate outcome, reason, applied decision reference, continuation checkpoint                                                                                         |

Allow one outstanding blocking question per task for the spike. Keep `PENDING`, `ANSWERED`, `APPLIED`, `REJECTED`, and `SUPERSEDED` question states separate from the run's `WAITING_FOR_PLANNER` stage. Preserve rejected submissions when a corrected answer arrives.

Record application before dispatching continuation. Answer submission alone does not prove execution resumed or passed. Use the same application service for CLI and MCP access. Proposed tool names and CLI syntax should be selected after inspecting existing conventions, not assumed in this plan.

## Configuration and bounds

Introduce only what the spike exercises: opt-in enablement, external planner mode, a question-round limit, response timeout, and pause-on-timeout. Follow SOP's existing config loader and proposed `SOP_PLANNER_ENABLED` / `SOP_PLANNER_MODE` override conventions; these names are not existing functionality claims.

Suggested fixture settings: at most three question rounds, one corrected-answer retry per question, and a short controllable deadline. Use an injected clock for timeout tests. The live demo must have a finite recorded timeout and no automatic retry after expiry.

Persist effective non-secret configuration, deadlines, and counters. Restart must not silently load broader permissions from changed environment settings. Clarification consumes a planner round, not a failure retry; existing failure budgets remain intact. Credentials are never copied into test artifacts or the report.

## Demonstration fixture

Use a trivial implementation task with two genuine choices, such as adding a list-normalization helper:

1. Executor asks whether output should preserve input order or be sorted.
2. Planner selects one option and explains it. SOP records and applies the answer.
3. Executor asks whether duplicate entries should be preserved or removed.
4. Planner answers. SOP resumes, and the executor implements the selected behavior.
5. Targeted tests verify the chosen semantics; SOP reports the actual completion state.

Use a scripted executor for repeatable checkpoint tests. In the live demonstration, identify which side is scripted and which is real. If possible, repeat once with the intended real executor; otherwise report that integration as unverified. No automatic Git publication is needed.

## Acceptance checklist

- [ ] A real intended planner client lists SOP tools and reads the correct repository status.
- [ ] Two distinct questions reach that planner and its answers resume the same task through SOP.
- [ ] Restart while waiting retains the question and prevents ordinary retry/recovery from dispatching it.
- [ ] Restart after decision application resumes at most once; duplicate answer delivery has no extra effect.
- [ ] Changed repository state or plan revision rejects a stale answer without executing it.
- [ ] A human-only authorization request cannot be satisfied by the planner, regardless of the answer text.
- [ ] Timeout, disconnect, and exhausted planner rounds preserve an explicit waiting/paused condition and evidence.
- [ ] Existing failure counters are unchanged by clarification; disabled mode preserves prior execution behavior.
- [ ] Targeted tests pass for the final fixture behavior; missing or failed checks are reported truthfully.
- [ ] The report identifies real versus simulated components, transport limitations, and whether a session must remain active.

## Deliverables and disposition

Produce a short report containing the baseline commit, exact client and transport, setup needed to reproduce, question/decision timeline, checkpoint evidence, tests run, and remaining gaps. Keep required run artifacts durable and reference them from the report. Do not remove a worktree containing unpreserved evidence.

**GO:** The intended client completes the interactive loop and durability/authority checks pass. Proceed to the production first milestone, retaining its separate acceptance requirements.

**PARTIAL:** Protocol and deterministic tests pass, but the intended client or real executor is unverified, or only a substitute client works. Specify the exact integration task needed next. Interactive success alone is not unattended success.

**NO-GO for the selected connection:** Transport, persistence, or authority constraints prevent a reliable loop. Preserve findings and revise the connection design before further orchestration work.

Do not merge the prototype automatically. Decide from the evidence whether to harden reusable code or retain only the contracts and tests. This spike does not authorize unrelated repository changes or alter the larger design's human review boundary.
