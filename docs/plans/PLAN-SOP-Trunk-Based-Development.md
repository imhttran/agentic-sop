# Plan: SOP Trunk-Based Development

Date: 2026-11-02
Status: Draft for repository inspection and implementation planning
Target repository: https://github.com/imhttran/agentic-sop

## Objective

Keep `main` releasable through small, frequently merged PRs. Make the compliant workflow easy for coding agents while preserving SOP approvals, verification, lifecycle state, and existing worktrees.

Enforce repository rules through GitHub and execution rules through SOP.

## Evidence and assumptions

- This plan follows the agreed direction in the discussion. It does not record completed implementation or approved governance changes.
- Current repository rules, configuration keys, worktree abstractions, lifecycle transitions, and project-lock behavior have not been inspected for this plan.
- As of 2026-11-02, ruleset PullRequestRequire on the default branch already blocks deletion and force pushes, requires a PR and CodeQL, and requires all six ci.yml jobs to pass (required_status_checks, non-strict).
- Task IDs below are proposed planning identifiers. They are not existing SOP task records.
- All proposed policy settings must be mapped onto existing configuration conventions during inspection. Do not invent a parallel configuration subsystem.

## Mandatory execution worktree

The implementing agent MUST execute this plan in a **new, dedicated Git worktree and branch**. Do not implement in the primary checkout or reuse an existing task worktree.

Before editing repository files:

1. Read repository instructions and inspect active SOP plans, gates, locks, worktrees, and local changes without modifying them.
2. Fetch `origin` and record the current `origin/main` SHA. If fetching fails, stop and report the blocker instead of choosing an unverified base.
3. Create a uniquely named branch and new sibling worktree from that explicit SHA using the existing supported Git/SOP workflow.
4. Verify the new worktree path, branch, HEAD, and clean status. Report these values before implementation.
5. Initialize or register this plan through the supported SOP lifecycle. Do not copy runtime databases, approvals, locks, or verification evidence from another worktree. If an active-plan or project lock prevents registration, report the blocker without resetting or historicalizing unrelated work.
6. Perform implementation, tests, commits, and PR creation from the dedicated worktree. Preserve all existing worktrees and user changes.

Use independently mergeable PR increments. After an increment merges, reconcile its SOP state and use a new short-lived branch/worktree from refreshed trunk for the next increment when the supported lifecycle permits it. Preserve plan identity and history through supported operations; never manually rewrite state to force a transition. If SOP cannot carry one plan across increments safely, report the limitation before proceeding with that transition.

Do not remove any execution worktree until remote merge and SOP cleanup checks confirm that all user files, unpushed work, and required evidence are preserved. Creating this document does not authorize bypassing an existing approval gate or changing remote protection settings.

Acceptance: implementation evidence identifies the dedicated worktree path, branch, starting trunk SHA, and PR for each increment. No implementation changes are made in pre-existing worktrees.

## Scope

1. Inspect current controls and document gaps.
2. Define a trunk-based development policy and independently mergeable task boundaries.
3. Add SOP preflight, integration evidence, and branch-health reporting where missing.
4. Prepare and apply authorized GitHub protections.
5. Document recovery, preservation, and rollout.

Excluded: replacing SOP orchestration; bypassing approvals; automatic merge authority; bulk branch deletion; migration of active plans; third-party worktree managers (plain Git and SOP's existing worktree code — `internal/git`, `internal/cli/isolation.go` — already cover them; revisit only for a concrete gap); automatic Slack delivery setup; building a new worktree manager.

## Operating policy

| Concern | Proposed rule | Enforcement |
| --- | --- | --- |
| Trunk | `main` remains buildable and releasable | Required CI and PR review |
| New work | Start from freshly fetched `origin/main`; record its SHA | SOP preflight |
| Change boundary | One coherent, independently mergeable increment per PR | Plan authoring and review |
| Branch lifetime | Target one working day; warn after two working days | Advisory report |
| Incomplete features | Use disabled flags or backward-compatible increments | Design and review |
| Verification | Evidence identifies the actual tested revision and relevant inputs | SOP verification |
| Integration | Validate the proposed merge result | GitHub checks or merge queue |
| Cleanup | Require merge and preservation evidence | SOP cleanup |
| Exceptions | Record reason, owner, scope, and expiry or review date | Existing SOP approval mechanism |

Branch age and PR size are advisory initially. Never delete, reset, merge, or block useful recovery solely because a branch is old. Define the working-day calculation and timezone in policy; do not infer branch creation from the latest commit timestamp. Prefer persisted work-start time and label fallback measurements explicitly.

## Responsibilities

| Component | Responsibility |
| --- | --- |
| GitHub | Remote protection, required checks, configured reviews, merge authorization |
| SOP | Task boundaries, preflight, approvals, evidence, recovery, lifecycle, safe cleanup |
| Agent | Implement the assigned increment and submit it through SOP and PR gates |
| Repository owner | Approve protection and exception policy changes when required |

Local SOP checks cannot prevent a user from invoking Git directly. Remote protections must enforce merge restrictions independently of SOP.

## Implementation tasks

### TBD-001 — Inspect repository and platform capabilities

Dependencies: none.

Inspect repository instructions, existing plans, worktrees, branch state, CI workflows, verification cache keys, project identity and locks, cleanup rules, and GitHub protections. Check repository visibility, account capabilities, ruleset access, bypass actors, and merge-queue availability. Identify the review identities available for a solo-maintainer workflow; do not configure an approval requirement that no eligible reviewer can satisfy.

Deliverables:

- Current-state matrix: existing control, gap, evidence, proposed change.
- Exact code/configuration locations to reuse.
- Inventory of active worktrees and plans to preserve.
- Proposed protection settings with required check names taken from actual workflows.

Acceptance: every proposed change is tied to an observed gap; unavailable capabilities are reported as blockers or documented alternatives. No lifecycle or remote protection mutations during inspection.

### TBD-002 — Define policy and mergeable plan increments

Dependencies: TBD-001.

Add the policy to the established documentation and agent-instruction locations. Update plan guidance so each increment identifies its PR boundary, dependencies, compatibility strategy, verification, and rollback. A plan may span multiple PRs. Do not equate one task with one PR when a small coherent group of tasks is the correct boundary.

Prefer merging prerequisite increments before starting dependent branches. If stacking is necessary, record branch dependencies and retarget/revalidate after prerequisites merge. Avoid a mandatory rebase loop while agents are implementing.

Acceptance: include examples for a small fix, a feature flag, and an additive schema/API migration. Each example keeps `main` usable at every merge boundary. Document an explicit exception path using existing governance.

### TBD-003 — Enforce safe work-start preflight

Dependencies: TBD-002.

Reuse existing Git/worktree abstractions. Resolve repository identity and remote/default branch. Fetch the intended remote, record the observed trunk SHA, inspect active work and locks, and create a new isolated branch/worktree from that explicit SHA. Stop with an actionable result if fresh remote state cannot be established; any offline exception must use the documented policy.

Preserve all existing worktrees, dirty files, untracked files, and detached checkouts. Resume work in its recorded worktree rather than silently recreating it from trunk. Initialize SOP state through supported commands; do not copy another run's approvals, database, locks, or evidence.

Acceptance: focused tests cover stale local `main`, fetch failure, an existing branch/path, dirty unrelated work, resume behavior, and concurrent worktree allocation. Existing non-trunk workflows remain available under documented policy.

### TBD-004 — Bind verification and completion to repository evidence

Dependencies: TBD-003.

Reuse or extend existing evidence invalidation. Record the PR head SHA, observed base SHA, verification inputs, and merge-result identity where available. Keep local completion distinct from remote merge and plan completion using existing lifecycle semantics.

Use a supported merge queue when appropriate; otherwise require branches to be current with the target branch and run checks on the PR merge result. Inspect CI triggers before selecting either approach. Merge queues need compatible merge-group checks. A successful check of an old head or old integration result must not authorize the new result.

Acceptance: tests cover changed head, changed base/integration result, missing required checks, failed checks, and merge confirmation. Reconcile remains idempotent. Reuse already-valid evidence only under the repository's established cache rules.

### TBD-005 — Configure remote protections

Dependencies: TBD-001 and TBD-004.

As of 2026-11-02, PR-required, no-force-push, no-deletion, CodeQL, and required CI checks are already enforced on the default branch; what remains is review settings and the strict up-to-date policy decision.

Prepare a concrete settings diff before applying governance changes. Require PRs and the actual required CI checks. Configure eligible review requirements, stale-review handling, resolved conversations where appropriate, and restrictions on force pushes and deletion. Inspect bypass permissions and document any explicit emergency path. Do not grant agents administrator or bypass access.

Apply only settings authorized by the repository owner under the existing governance process. Check API responses and read back resulting configuration. Never claim settings are enforced merely because a policy document or local test exists.

Acceptance: record before/after settings and supported evidence that direct updates and unqualified merges are restricted. Use a disposable test repository or safe API inspection for negative checks; do not deliberately damage production branches. Platform or permission limitations remain explicit rollout blockers.

### TBD-006 — Add advisory branch and PR health reporting

Dependencies: TBD-002 and TBD-004.

Report work-start age, divergence from observed trunk, PR state, verification freshness, and the next action. Distinguish unavailable data from a passing result. Use existing status/report surfaces. Size warnings should account for generated files and mechanical changes.

Acceptance: reports identify aging work without deleting or modifying it. Recommendations distinguish splitting scope, updating the base, waiting for review, and abandoning work by an authorized decision. No scheduled job or outbound notification is created as part of this task.

### TBD-008 — Validate rollout and document recovery

Dependencies: TBD-006.

Run a small pilot on new work only. Record branch lifetime, time waiting for review, integration failures, setup friction, and recovery incidents. Treat these as evaluation signals rather than productivity scores. Preserve active plans and historical worktrees.

Acceptance:

- One small fix and one compatible feature increment pass the full PR lifecycle.
- Required checks pass on the applicable current integration result.
- Remote merge evidence reconciles into SOP without manufacturing approval or test evidence.
- Cleanup preserves unpushed commits, user files, and required run evidence.
- A failed setup and a stale-verification scenario produce actionable recovery guidance.

## Verification strategy

Run focused tests for modified policy, Git, state, and evidence paths. Use temporary repositories for Git operations. Run existing required project checks once per relevant revision and complete remaining verification through PR CI. Do not add tests that merely duplicate configuration text. Exercise meaningful failure paths without exhaustive repeated full-suite runs.

Evidence should include commit SHA, commands/check identities, results, timestamps, and relevant environment or version details. Never treat a draft PR, local green check, or merged code alone as proof of full plan acceptance.

## Risks and controls

| Risk | Control |
| --- | --- |
| Mandatory reviews block a solo maintainer | Confirm eligible reviewer arrangement before enabling |
| Frequent trunk updates cause redundant validation | Verify at integration boundary; retain safe cache behavior |
| Worktree state shares locks or project identity incorrectly | Inspect current semantics before adapter changes |
| CI queue or ruleset features are unavailable | Document supported alternative and remaining limitation |
| Age warnings encourage premature merges | Advisory thresholds; completion and quality gates remain authoritative |
| Automation fails after a partial mutation | Reconcile actual state; do not blindly retry |

## Rollout and rollback

Ship documentation and core enforcement in small PRs, followed by advisory reporting. Keep one plan of record with explicit increment-to-PR mapping. Adapt task boundaries after inspection rather than opening one long-lived branch for this entire plan.

Preserve evidence and existing plans. Changes to remote protections require the same governance as enabling them; operational problems do not authorize silent weakening of protections.

## Definition of done

The documented trunk workflow is enforced remotely where supported, SOP records and verifies the relevant revisions, new work uses isolated worktrees, small increments can merge independently, and safe cleanup follows remote merge confirmation. All unresolved capability gaps and policy exceptions are reported explicitly.

## References

- Repository to inspect: https://github.com/imhttran/agentic-sop
