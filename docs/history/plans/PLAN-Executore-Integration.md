# Implementation Plan

## Project

SOP Planner–Executor Integration

Durable, bidirectional coordination between ChatGPT as planner and a coding agent running in Zed as executor, built on the existing SOP CLI, MCP planner tools, durable state, and governance controls.

## Summary

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

## PE-1 — Planner Communication Contract

Complete the minimum bidirectional contract needed for planner–executor coordination, and make that contract's evidence retrievable over MCP.

Capability inventory, marked existing, missing, or unverified:

- Existing — durable question, answer, and outcome records with idempotency keys, expected-revision checks, and an append-only audit ledger (`internal/question`). Source: `docs/plans/SPIKE-004-live-evidence.md`, `docs/plans/planner-live-test-findings.md`.
- Existing — question binding to project, task, run id and run epoch, plan revision (`plan_id@source_sha256`), and repository fingerprint, with stale-binding refusal before an answer is recorded (`questionBindingCheck` in `internal/cli/question.go`).
- Existing — continuation records with lease claims, generation fencing, terminal completion, and yielded-continuation recovery, so an applied decision is consumed once (`internal/question/continuation.go`, `internal/cli/planner.go`).
- Existing — planner MCP surface `sop_status`, `sop_question_list`, `sop_question_get`, `sop_answer_submit`, deliberately excluding `sop_approve` and `sop_decline`. Source: `docs/plans/SPIKE-001-baseline-inventory.md`, `docs/plans/SOP-Planner-Connection-Spike.md`.
- Missing — a planner-scoped task-result operation. No MCP tool returns a task's persisted status, attempts, plan revision, latest run stage and gate, or evidence paths.
- Missing — a planner decision operation over a closed action vocabulary (`continue`, `correct`, `revise`, `escalate`) that validates the action and records it as advice.
- Missing — a planner-scoped decision listing that returns a task's questions, recorded answers, outcomes, and continuation state together.
- Unverified — attempt-level binding. A question records the run epoch but not the task attempt ordinal, so a decision made against an earlier attempt is refused only when the run epoch also changed.

The contract this stage establishes:

- Decision receipt and resumption is interactive. The executor discovers pending work with `sop planner status` or the decision listing tool, retrieves evidence with `sop_task_result`, applies the decision with `sop planner continue --apply`, and resumes with `sop run`. Event-driven or unattended invocation is not claimed and must never be simulated.
- Durable decision consumption and duplicate-execution protection belong to this contract, not to later hardening. A decision is consumed at most once per run epoch, a re-delivered submission returns a duplicate outcome instead of a second record, an already-applied decision is never re-executed, and the resumed stage receives the applied decision as input.
- When an action's outcome is uncertain — a continuation was claimed but not completed — recovery reconciles the durable continuation rather than re-running the action. Claim generation, lease expiry, and yielded-continuation resolution decide whether the work is still owed or already done.
- Plan revisions invalidate older decisions. A decision is recorded against the plan revision, task, and attempt it was made for; when the live plan revision differs, submission is refused as stale, the prior record is preserved, and the invalidation is scoped to decisions bound to the superseded revision. Active tasks under the new revision are unaffected.
- Planner advice is never human approval. A recorded decision is `ANSWERED` only: it never approves, declines, or resumes execution, and the planner surface exposes no approval operation.

Work for this stage:

- Inventory existing MCP planner and SOP task APIs (see the inventory above).
- Reuse existing question, answer, task, approval, and evidence records.
- Add the missing task-result and planner-decision operations.
- Bind decisions to the plan revision, task, attempt, and question.
- Support idempotent submissions and stale-revision rejection.
- Preserve the distinction between advice, authorized execution, and human approval.
- Add focused contract and authorization tests.
- Open a focused PR.

### Dependencies

None

### Acceptance Criteria

- A planner can retrieve a task result — status, attempts, plan revision, latest run stage and gate, and evidence paths — through the planner MCP surface.
- A planner can retrieve the pending question and any recorded decision for a task.
- A planner can record a `continue`, `correct`, `revise`, or `escalate` decision against that question, and the recorded decision carries the plan revision, task, and attempt it was made for.
- The executor can retrieve the recorded decision and apply it with `sop planner continue --apply`.
- A decision submitted against an older plan revision, task, attempt, or question revision is refused as stale and leaves the prior record intact.
- A re-delivered decision with the same idempotency key returns a duplicate outcome and records no second answer.
- No planner decision is treated as human approval, and the planner surface exposes no approval operation.
- Existing SOP tests pass.
- PR is reviewed and merged before operational validation.

### Acceptance Checks

- `go test ./internal/cli -run TestMCPTaskResultTool -count=1`
- `go test ./internal/cli -run TestMCPDecisionListTool -count=1`
- `go test ./internal/cli -run TestMCPDecisionSubmitTool -count=1`
- `go test ./internal/cli -run TestMCPPlannerServerSurface -count=1`
- `go test ./internal/cli -run TestMCPDecisionAttemptBindingIsStale -count=1`
- `go test ./internal/cli -run TestMCPDecisionDuplicateIsIdempotent -count=1`
- `go test ./internal/cli -run TestMCPPlannerSurfaceShape -count=1`
- `go test ./... -count=1`

### Acceptance Mapping

- criterion 1: check 1
- criterion 2: check 2
- criterion 3: check 3
- criterion 4: check 4
- criterion 5: check 5
- criterion 6: check 6
- criterion 7: check 7
- criterion 8: check 8
- criterion 9: manual: an operator reviews and merges the stage PR

## PE-2 — Zed Executor Integration

Allow the Zed coding agent to participate in the planner workflow using existing SOP execution capabilities.

Decision receipt is interactive in this stage. Zed retrieves a recorded decision with the planner surface, applies it with `sop planner continue --apply`, and resumes execution with `sop run`; the applied decision is carried into the resumed stage. Unattended or event-driven invocation is not claimed.

Work for this stage:

- Inspect existing Zed/ACP and SOP agent integration.
- Implement only missing result-publishing and decision-consumption functionality.
- Ensure the executor can report progress, questions, blockers, test results, and completion evidence.
- Ensure the executor can resume from an accepted planner decision.
- Preserve task and attempt identity.
- Handle unavailable planner decisions without busy-looping or unauthorized continuation.
- Add focused regression tests.
- Open a focused PR.

### Dependencies

- PE-1

### Acceptance Criteria

- Zed can publish task results and questions to SOP.
- Zed can retrieve a decision from the planner surface and consume an applicable planner decision.
- Zed can continue execution without manual question/answer copying, resuming with `sop planner continue --apply` and `sop run`.
- An unavailable or invalid decision cannot trigger execution.
- Task history and evidence remain durable.
- PR is reviewed and merged before operational validation.

## PE-3 — Planner Continuation

Enable ChatGPT to continue planning and decision-making from persisted SOP state.

A plan revision invalidates the decisions bound to it: a decision recorded against an earlier revision is refused when the live revision differs, and active tasks under the new revision are unaffected. Automatic notification or event-driven invocation is documented only where it is demonstrated.

Work for this stage:

- Provide a clear interface for pending planner work.
- Expose the evidence needed to make a decision without requiring full log copying.
- Support planner decisions such as continue, request correction, revise plan, or escalate to human.
- Distinguish supported interactive continuation from automatic event-driven invocation.
- Use an existing supported notification or invocation mechanism if available.
- Do not add a new service solely to simulate automatic ChatGPT participation.
- Add focused continuation tests.
- Open a focused PR.

### Dependencies

- PE-1
- PE-2

### Acceptance Criteria

- ChatGPT can retrieve the current decision context through MCP.
- ChatGPT can submit a decision that the executor can consume.
- The workflow resumes from durable state after a new ChatGPT interaction.
- A decision bound to a superseded plan revision is refused rather than applied to the current revision.
- Any automatic notification or invocation behavior is demonstrated, not assumed.
- Unsupported unattended behavior is documented as a limitation.
- PR is reviewed and merged before operational validation.

## PE-4 — Recovery and Hardening

Ensure safe continuation across failures, restarts, and repeated requests.

An uncertain action outcome is reconciled from durable continuation state — claim generation, lease expiry, and yielded-continuation resolution — instead of re-running the action, so an interrupted decision is neither lost nor executed twice.

Work for this stage:

- Reuse existing checkpoint and attempt state.
- Handle duplicate decisions and submissions.
- Reject stale plan revisions and task attempts.
- Handle executor disconnects and planner unavailability.
- Preserve evidence across retries and restarts.
- Enforce existing approval boundaries.
- Add regression tests for observed failure modes.
- Open a focused PR.

### Dependencies

- PE-1
- PE-2
- PE-3

### Acceptance Criteria

- Repeated submissions do not cause duplicate execution.
- Stale decisions cannot affect newer task attempts.
- An action whose outcome is uncertain is reconciled from durable continuation state rather than re-executed.
- Interrupted tasks resume from valid checkpoints.
- Approval requirements remain enforced.
- Evidence is retained and attributable to the correct attempt.
- Existing SOP verification gates pass.
- PR is reviewed and merged.

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
- An interrupted execution resumes from a durable checkpoint without repeating completed work.
- A stale decision is refused and a duplicate decision is consumed exactly once.

## Final Deliverables

- Merged implementation PRs.
- Relevant automated test results.
- Real-world acceptance evidence.
- Focused fix PRs for findings, if necessary.
- Updated documentation describing the supported workflow.
- Explicit documentation of any remaining limits on unattended ChatGPT participation.
