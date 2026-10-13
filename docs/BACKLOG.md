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

## SOP-EXEC-001 — Lightweight Execution Profiles

P2 · Proposed
Target: agentic-sop/docs/BACKLOG.md
Scope

- Add --mode=light|standard|full to sop run.
- Support the same flag in SOP skills.
- Default to standard.
- Keep mandatory targeted verification in every mode.
- Minimize preflight for low-risk tasks.
- Automatically escalate when scope or risk increases.
- Preserve Git and policy safeguards.
  Acceptance criteria
- Small tasks execute without unnecessary full-suite checks.
- Targeted tests remain mandatory where applicable.
- Failed targeted tests prevent PASS.
- No applicable tests are reported explicitly.
- Existing behavior remains backward-compatible.

## DME-FU-001 — Investigate `DecideEarly` handling of `INCOMPLETE`/`ERROR`

- **Priority:** P1
- **Status:** Proposed
- **Problem:** In the JEV shadow-mode evaluation, `autonomy.DecideEarly` mapped a
  result with no blocking finding — including `INCOMPLETE`/`ERROR` with no
  HIGH/CRITICAL items — to `CONTINUE`, while the quality gate
  (`quality.NewJEVEvidence`/`quality.Evaluate`) treats `INCOMPLETE`/`ERROR` as
  fail-closed evidence that is never a pass. The two consumers disagree on the
  safety semantics of an incomplete analysis.
- **Requirements:** Confirm whether this is reachable in the real execution path
  (the early-JEV checkpoints), whether the divergence is intentional per consumer,
  and whether `DecideEarly` should fail closed on `INCOMPLETE`/`ERROR`.
- **Acceptance:** A written finding with the production reachability path (or a
  "not reachable" determination) and a decision on fail-closed semantics; any code
  change carries targeted tests.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md) §5;
  `evals/decision-model/jev-shadow.results.json` (`dme-provider-fault-002`,
  `dme-real-as-clef-005` ERROR; `dme-real-close-001`, `dme-real-rsh-002` INCOMPLETE).

## DME-FU-002 — Review the `dme-real-close-004` label and `BLOCK` vs `NEEDS_HUMAN`

- **Priority:** P2
- **Status:** Proposed
- **Problem:** `dme-real-close-004` expects `NEEDS_HUMAN` while the deterministic
  classifier produces `BLOCK` (counted as the corpus's single unsafe miss). `BLOCK`
  and `NEEDS_HUMAN` both withhold automatic execution but carry different
  operational responsibilities (a terminal stop with no approval vs a human approval
  boundary). Whether the frozen label should be `BLOCK` or `NEEDS_HUMAN` has not been
  adjudicated.
- **Requirements:** Independently review the label against the recorded failure and
  document the intended semantics; if the label changes, re-freeze the corpus and
  record a new corpus revision (labels are otherwise frozen).
- **Acceptance:** A written adjudication with rationale; the corpus hash recorded for
  any revision; downstream metrics re-derived where the label changes.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md) §5.

## DME-FU-003 — Define evidence-based criteria for reconsidering AI failure disposition

- **Priority:** P2
- **Status:** Proposed
- **Problem:** The evaluated AI configurations produced zero useful escalations and
  more errors than deterministic Go, so no AI failure-disposition configuration is
  promoted. There is no agreed, pre-registered bar for when reconsideration would be
  justified.
- **Requirements:** Turn the plan's decision rule into concrete, pre-registered
  thresholds (unsafe misses = 0, false-escalation ceiling, latency budget, and at
  least one of accuracy or useful-escalation rate showing a supported improvement),
  and name the evidence a candidate configuration must produce first.
- **Acceptance:** A written, pre-registered acceptance bar a candidate must meet
  before it can influence production decisions.
- **Evidence:**
  [PLAN-Decision-Model-Effectiveness-Evaluation.md](plans/PLAN-Decision-Model-Effectiveness-Evaluation.md#decision-rule-retain--tune--disable).

## DME-FU-004 — Evaluate JEV's code-review effectiveness on real changes

- **Priority:** P2 · Conditional
- **Status:** Proposed (only if warranted)
- **Problem:** JEV was evaluated for failure disposition using only terse failure
  text (no diffs or repository context), which is outside its primary review use
  case. Its value as a code-quality/review mechanism therefore remains unmeasured.
- **Requirements:** If JEV's review value matters, evaluate it on real diffs and
  repository context against the quality gate's own criteria — a distinct
  experiment, not a re-run of the decision-disposition corpus. Do not promote JEV as
  a failure-disposition classifier regardless of that result.
- **Acceptance:** A scoped evaluation plan with labeled review fixtures and
  reproducible evidence, or a documented decision to defer.
- **Evidence:** [DECISION-MODEL-FINDINGS.md](reference/DECISION-MODEL-FINDINGS.md) §4, §6.
