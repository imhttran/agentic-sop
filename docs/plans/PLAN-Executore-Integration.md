# Project

SOP Planner–Executor Integration

## Summary

Implement durable, bidirectional coordination between ChatGPT as planner and a coding agent running in Zed as executor.

The intended workflow is:

1. ChatGPT creates an SOP execution plan.
2. The user hands the plan to Zed once.
3. Zed executes authorized tasks and reports results or questions to SOP.
4. ChatGPT retrieves evidence and questions through MCP.
5. ChatGPT records decisions or requests changes through SOP.
6. Zed consumes those decisions and continues.
7. SOP retains task history, approvals, evidence, and completion state.

The user should not manually copy questions, answers, or execution results between agents after the initial handoff.

## Implementation Principles

- Reuse existing SOP CLI, MCP planner tools, durable state, and governance controls.
- Inspect current implementation before adding functionality.
- Do not create additional SPIKE tasks unless a concrete implementation blocker makes one necessary.
- Prefer deterministic behavior and minimal dependencies.
- Do not create a separate orchestration framework.
- Preserve unrelated worktree changes.
- Use isolated branches or worktrees.
- Never treat planner advice as human approval.
- Never allow model-generated decisions to bypass deterministic authorization.
- Do not claim unattended ChatGPT invocation unless a supported trigger mechanism has been verified.
- Every phase must be independently reviewable and mergeable.

## Stage 1 — Planner Communication Contract

### Objective

Complete the minimum bidirectional contract needed for planner–executor coordination.

### Tasks

- Inventory existing MCP planner and SOP task APIs.
- Reuse existing question, answer, task, approval, and evidence records.
- Define any missing task-result and planner-decision operations.
- Bind decisions to the correct plan revision, task, attempt, and question where applicable.
- Support idempotent submissions and stale-revision rejection.
- Preserve the distinction between advice, authorized execution, and human approval.
- Add focused contract and authorization tests.
- Open a focused PR.

### Acceptance Criteria

- ChatGPT can retrieve a task result or pending question through MCP.
- ChatGPT can record a decision or correction request using a supported interface.
- The executor can retrieve the recorded decision.
- Stale and duplicate submissions are handled safely.
- No model answer is treated as approval.
- Existing SOP tests pass.
- PR is reviewed and merged before operational validation.

## Stage 2 — Zed Executor Integration

### Dependencies

Stage 1 merged.

### Objective

Allow the Zed coding agent to participate in the planner workflow using existing SOP execution capabilities.

### Tasks

- Inspect existing Zed/ACP and SOP agent integration.
- Implement only missing result-publishing and decision-consumption functionality.
- Ensure the executor can report progress, questions, blockers, test results, and completion evidence.
- Ensure the executor can resume from an accepted planner decision.
- Preserve task and attempt identity.
- Handle unavailable planner decisions without busy-looping or unauthorized continuation.
- Add focused regression tests.
- Open a focused PR.

### Acceptance Criteria

- Zed can publish task results and questions to SOP.
- Zed can consume an applicable planner decision.
- Zed can continue execution without manual question/answer copying.
- An unavailable or invalid decision cannot trigger execution.
- Task history and evidence remain durable.
- PR is reviewed and merged before operational validation.

## Stage 3 — Planner Continuation

### Dependencies

Stages 1 and 2 merged.

### Objective

Enable ChatGPT to continue planning and decision-making from persisted SOP state.

### Tasks

- Provide a clear interface for pending planner work.
- Expose the evidence needed to make a decision without requiring full log copying.
- Support planner decisions such as continue, request correction, revise plan, or escalate to human.
- Distinguish supported interactive continuation from automatic event-driven invocation.
- Use an existing supported notification or invocation mechanism if available.
- Do not add a new service solely to simulate automatic ChatGPT participation.
- Add focused continuation tests.
- Open a focused PR.

### Acceptance Criteria

- ChatGPT can retrieve the current decision context through MCP.
- ChatGPT can submit a decision that the executor can consume.
- The workflow resumes from durable state after a new ChatGPT interaction.
- Any automatic notification or invocation behavior is demonstrated, not assumed.
- Unsupported unattended behavior is documented as a limitation.
- PR is reviewed and merged before operational validation.

## Stage 4 — Recovery and Hardening

### Dependencies

Stages 1–3 merged.

### Objective

Ensure safe continuation across failures, restarts, and repeated requests.

### Tasks

- Reuse existing checkpoint and attempt state.
- Handle duplicate decisions and submissions.
- Reject stale plan revisions and task attempts.
- Handle executor disconnects and planner unavailability.
- Preserve evidence across retries and restarts.
- Enforce existing approval boundaries.
- Add regression tests for observed failure modes.
- Open a focused PR.

### Acceptance Criteria

- Repeated submissions do not cause duplicate execution.
- Stale decisions cannot affect newer task attempts.
- Interrupted tasks resume from valid checkpoints.
- Approval requirements remain enforced.
- Evidence is retained and attributable to the correct attempt.
- Existing SOP verification gates pass.
- PR is reviewed and merged.

## Delivery and Validation Policy

For each stage:

1. Fetch the latest main branch.
2. Inspect existing functionality and reuse it.
3. Create an isolated implementation branch or worktree.
4. Implement the smallest complete change.
5. Run relevant automated tests.
6. Open a PR with acceptance evidence.
7. Review and merge after approval.
8. Exercise the merged capability through a real SOP workflow.
9. Record actual findings.
10. If necessary, create a focused fix branch and PR.
11. Repeat until the observed findings are resolved.

Do not create a separate exploratory test project.

Do not repeatedly test capabilities already established by earlier MCP spikes.

Do not invent passing integration evidence. Clearly identify capabilities that have not yet been exercised end-to-end.

## Final Acceptance

The implementation is complete when a real workflow demonstrates:

- One initial plan handoff from ChatGPT to Zed.
- Zed executes and reports through SOP.
- ChatGPT reads the result through MCP.
- ChatGPT makes at least one meaningful planner decision.
- Zed consumes the decision and performs the requested action.
- SOP records the decision, execution, verification, and completion evidence.
- No manual copying of intermediate messages is required.
- Human approval is requested only when required by policy.

## Final Deliverables

- Merged implementation PRs.
- Relevant automated test results.
- Real-world acceptance evidence.
- Focused fix PRs for findings, if necessary.
- Updated documentation describing the supported workflow.
- Explicit documentation of any remaining limits on unattended ChatGPT participation.
