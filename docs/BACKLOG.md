# Backlog — agentic-sop

Long-term enhancements, separate from active SOP plans and runtime state. Each item
carries ID, priority, status, problem, requirements, and acceptance criteria.

## SOP-LC-001 — Automatic Task-State Closure with Evidence Reuse

- **Priority:** P2
- **Status:** IMPLEMENTED (2026-10-12)
- **Problem:** `sop run` could not close a plan whose tasks were already implemented
  and committed: an empty-diff task still invoked the optional JEV analysis, whose
  fail-closed result stopped the run at a retry/human boundary (`LC-001 RETRY
(requeued)`), so a green tree left the plan ACTIVE. Completing the tasks required a
  separate operator command per task, each re-running the full validation suite.
- **Requirements:**
  - Skip JEV when there is no reviewable change (an empty diff with no task-owned
    change evidence), mirroring review's "no changes to review" guard.
  - Reuse valid, task-scoped validation evidence across the plan (already provided by
    the run-session verification cache).
  - Complete already-satisfied verify-first tasks without an agent call, progress the
    lifecycle automatically, and close the plan on completion.
  - Preserve deterministic acceptance, audit history, and human approval boundaries;
    never fabricate completion for insufficient evidence.
- **Acceptance:** A 12-task plan with sufficient evidence closes in one governed
  workflow without 12 redundant validation runs.
- **Evidence:** `internal/cli/run.go` gates the JEV invocation on a reviewable change;
  `internal/cli/sop_lc_001_test.go` proves one `sop run` closes a 12-task
  already-committed plan (archived `COMPLETE`), with 0 agent calls, 0 JEV invocations,
  and 1 validation run reused across the 12 tasks; a second invocation is idempotent;
  a task with insufficient evidence fails closed and does not complete.
