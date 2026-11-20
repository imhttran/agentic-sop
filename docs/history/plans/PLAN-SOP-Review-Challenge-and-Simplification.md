# Plan: SOP Review Challenge and Simplification

**Plan ID:** `plan-sop-review-challenge-simplification`  
**Repository:** `imhttran/agentic-sop`  
**Status:** PROPOSED — not activated or executed  
**Proposed plan path:** `docs/plans/PLAN-SOP-Review-Challenge-and-Simplification.md`  
**Execution policy:** Existing SOP lifecycle, isolated worktree, bounded autonomy, normal PR review

## 1. Goal

Strengthen SOP's existing review phase so it critically evaluates whether implementation code **meets the task's original goal**, uses the **simplest maintainable design** consistent with repository conventions, and is backed by **meaningful test evidence**.

The reviewer should challenge assumptions and recommend a change only when it can state a concrete defect or simplification benefit. An implementation that already meets these standards should pass without manufactured changes.

**Non-goal:** Introduce a new stage, controller, approval gate, arbitrary retry loop, or mandatory refactor cycle.

## 2. Confirmed Baseline (GitHub `main`, inspected 2026-11-08)

- `docs/specs/REVIEW.md` §7 already requires self-review of acceptance criteria, correctness, tests, edge cases, error handling, architecture boundaries, and unnecessary complexity.
- `docs/specs/REVIEW.md` §3 states that models produce findings but **never decide the verdict**; §8 allows rejecting suggestions that add complexity.
- `docs/specs/QUALITY.md` owns `PASS`/`FAIL`/`NEEDS_HUMAN`, review severity policy, and the bounded fix loop. `quality.max_fix_cycles` defaults to **3**.
- `docs/specs/TASK-LIFECYCLE.md` describes acceptance criteria → tests → implement → verify → refactor → run affected suite. Its test-design/RED stage is explicitly described as future work; this plan does not implement it.
- `internal/review/review.go` defines `Request{Task, Criteria, Diff, TestSummary}`, `Finding`, `Report`, and reviewer interface.
- `internal/quality/quality.go` owns deterministic quality decisions.
- Review input is invocation-scoped; pre-existing unrelated worktree changes must be excluded. Existing validation and review reuse rules must be preserved.

**Not yet verified:** Which current prompt templates, review adapters, lifecycle call sites, or tests fully implement these documented review objectives. Task CRS-001 establishes the exact change surface. Do not assume a missing runtime behavior solely because the specification already requires it.

## 3. Required Review Behavior

For every changed task that reaches the existing review stage, the reviewer should evaluate:

1. **Requirement fit:** Does the implementation address the task's intent and every acceptance criterion? Identify unmet requirements by their existing identifier/text; do not invent requirements.
2. **Correctness and safety:** Are edge cases, failure handling, boundary conditions, security, and contracts handled appropriately?
3. **Simplicity:** Can code, duplication, layers, flags, configuration, branches, or dependencies be removed without harming correctness, clarity, performance, or future maintenance? Prefer established repository abstractions.
4. **Test evidence:** Do tests assert required behavior and meaningful failures? Are claims supported by actual validation results? Distinguish missing evidence from failed tests and from observed passes.
5. **Change justification:** For each proposed edit, explain the concrete problem, expected benefit, scope, and regression risk. No aesthetic-only churn or speculative abstractions.

### Review outcome rules

- **No justified findings:** Return the current valid structured report with no findings; reviewer narrative can state that no change is required. Do **not** add a new verdict enum or quality-gate authority.
- **Non-blocking simplification suggestion:** Use the existing finding severity policy; do not force a fix cycle and do not treat refusal of the suggestion as failure.
- **Blocking correctness/required-test finding:** Use existing review finding schema and `quality.fail_on` policy; feed remediation through the existing bounded fix loop, then revalidate and re-review.
- **Unsupported claim, malformed review result, missing review input, or provider failure:** Preserve existing fail-closed classification and recovery behavior. Do not manufacture `PASS`.
- **No task changes to review:** Preserve the existing skip behavior.

## 4. Invariants / Non-Goals

- SOP remains the sole deterministic owner of transitions, review severity policy, and gate verdicts.
- No new user approval, human decision, configuration setting, model routing override, or review engine is introduced.
- No additional always-on LLM pass or unconditional second review call; strengthen the review that already runs.
- No new retry counters or changes to `quality.max_fix_cycles`, `max_attempts`, recovery, or escalation policies.
- No mandatory refactor in a passing implementation; no changes for style alone.
- Do not mutate unrelated files, other worktrees, uncommitted changes, or protected SOP runtime state.
- Continue using invocation-scoped diffs and existing evidence provenance; never review an entire dirty working tree by fallback.
- Existing security, approval, validation, quality, PR, CI, and performance-cache constraints remain authoritative.

## 5. Tasks

### CRS-001 — Audit existing review behavior and establish the smallest change surface

**Depends on:** None  
**Scope:** Read-only code/spec inspection; no production behavior changes.

- Trace the actual `sop run` review lifecycle, `sop review` command, `self` and `open-code-review` adapters, prompt construction, structured parsing, test summaries, and quality-gate integration.
- Compare the runtime prompts and tests against `docs/specs/REVIEW.md` §7 and the challenge criteria in §3 above.
- Identify whether current prompts already ask about unnecessary complexity and test sufficiency, and where findings are turned into remediation.
- Record specific deficiencies and their evidence (file path, relevant function/test). If no actionable gap exists, document the finding and mark implementation changes already satisfied rather than changing code.

**Acceptance:** A concise evidence-backed gap matrix: requirement, existing behavior, gap (if any), minimal target. No speculative changes.

### CRS-002 — Strengthen the existing review contract and prompt only where needed

**Depends on:** CRS-001  
**Scope:** Existing review prompt/template and directly related test code; update normative specification only if behavior requirements need clarification.

- Integrate explicit **goal fit → correctness → complexity → tests → change justification** questions into the existing self-review prompt or nearest shared reviewer seam, only where CRS-001 demonstrates a gap.
- Require findings to cite a concrete affected code path and task criterion when relevant, and to explain why a suggested simplification materially helps.
- Instruct the agent to prefer **no change** over speculative refactoring when existing code is adequate.
- Preserve existing `Finding` schema, severity vocabulary, parsed-report behavior, external-engine integration, and deterministic quality ownership. Do not force a new structured response field unless audit evidence proves it necessary.
- Ensure a proposed refactor cannot silently dilute a requirement, remove an important guard, or weaken tests.

**Acceptance:** Reviewer can identify a concrete requirement defect, offer a justified complexity reduction, and return an empty valid findings list when code is adequate, without a new verdict or stage.

### CRS-003 — Preserve bounded fix/review behavior and validate no-change handling

**Depends on:** CRS-002  
**Scope:** Existing review/remediation integration and its focused regression tests; no new controller.

- Confirm that blocking findings enter the existing `FIX → VALIDATE → REVIEW` path and consume the current fix-cycle budget.
- Confirm non-blocking complexity feedback does not force a fix iteration.
- Confirm a valid empty findings report ends the review without calling the implementation agent again.
- Confirm a changed implementation is revalidated and re-reviewed with the updated invocation-scoped diff.
- Confirm malformed or unavailable reviewer output preserves existing bounded/fail-closed treatment and never results in a false pass.
- Change lifecycle code only for a demonstrated defect found during the audit, with a focused regression reproducer.

**Acceptance:** No new retries, gates, or state values; no unnecessary iteration on already-correct work; blocking findings retain unchanged quality policy.

### CRS-004 — Focused evidence and regression coverage

**Depends on:** CRS-002, CRS-003  
**Scope:** Tests covering the touched review and quality seams.

Use the repository's existing testing conventions and gates. Cover the following distinct cases with focused fixtures or tests:

1. Requirement satisfied, clean implementation, adequate tests → valid no-findings review and no fix invocation.
2. Requirement missing or behavior incorrect → justified blocking finding under configured policy.
3. Unnecessary abstraction or duplication, no correctness failure → actionable advisory finding, no forced remediation.
4. Test assertions weak or missing for a specified acceptance criterion → evidence-grounded finding without claiming tests failed when they were not run.
5. Simplification performed within a fix iteration → affected tests and review rerun according to existing rules.
6. Malformed review output and unrelated dirty worktree → existing failure/isolation protections remain intact.

Prefer deterministic tests for prompt rendering, parsing, scope, severity, and lifecycle integration. Use an existing model-driven evaluation only if already available and useful; do not make model output the sole correctness oracle or add an expensive mandatory benchmark.

**Acceptance:** Focused regression tests pass; existing configured validation gates and CI/PR workflow remain authoritative. Report the exact commands/results rather than claiming unrun checks passed.

### CRS-005 — Documentation, PR, and handoff

**Depends on:** CRS-004  
**Scope:** Documentation of the established review behavior and usual PR workflow.

- Update `docs/specs/REVIEW.md` only as necessary to reflect the verified contract. Keep `docs/specs/QUALITY.md` authoritative for blocking/verdicts.
- Update relevant documentation index or examples only if existing conventions require it; avoid documentation duplication.
- Report evidence: observed gaps, exact behavior changed, tests run, no-change behavior, remaining limitations.
- Commit changes on the isolated worktree branch, open a PR against `main`, and leave final verification to established PR checks and review. Do not auto-merge without existing governance authorization.

**Acceptance:** PR includes a concise before/after description, verified test results, and proof that no extra gate/loop was added.

## 6. Execution Instructions for the Coding Agent

1. Work in a **new isolated Git worktree and branch** based on the current `origin/main`. Preserve existing worktrees and user-owned/uncommitted changes. If a worktree already exists for this specific approved task, inspect before reusing; do not delete it.
2. Check the repository state and active SOP plan before activation. Do not supersede unresolved work or overwrite a running plan. Use the repository's supported plan handoff/reconciliation flow.
3. Execute this plan using `sop run` and established lifecycle policy; handle routine implementation choices autonomously. Do not bypass security controls or SOP verification/approval gates.
4. If CRS-001 shows the desired behavior already exists, record **ALREADY_SATISFIED** using the supported SOP lifecycle semantics and avoid redundant edits; otherwise implement only proven gaps.
5. Stop only for a genuine blocker (e.g., an authorization/security boundary or unrecoverable missing prerequisite). Report the exact reason, work completed, and supported next action. Do not loop unchanged actions.
6. Do not add approval requests, speculative remedial work, manual test checklists, or extra full test runs beyond existing SOP gates.
7. Create a PR when implementation is ready; continue only through the established PR review/verification workflow.

## 7. Definition of Done

- [ ] Reviewer explicitly evaluates requirement fit, correctness, simplicity, tests, and justification using its **existing** review invocation.
- [ ] Clean satisfactory code yields **no requested changes**.
- [ ] Justified complexity feedback does not automatically block.
- [ ] Blocking findings use existing configured severity and bounded remediation.
- [ ] Reviewer does not invent evidence, requirements, tests, or acceptance results.
- [ ] Existing review scope isolation and malformed-output protections are maintained.
- [ ] No new lifecycle stage, gate, retry budget, approval requirement, or unconditional model call.
- [ ] Focused regressions pass; test evidence and limitations are documented.
- [ ] Changes delivered in an isolated worktree and PR; no unrelated work disturbed.

## 8. Main Risk and Mitigation

**Risk:** A stronger review prompt produces more speculative suggestions, costs more tokens, or drives unnecessary fix cycles.  
**Mitigation:** Require concrete evidence and material benefit, preserve existing severity and retry policy, allow a valid zero-findings report, and compare the before/after prompt behavior with focused deterministic tests. Reject any change that adds a new model invocation just for this review.

## 9. Source References

Inspected on the repository default branch:

- https://github.com/imhttran/agentic-sop/blob/main/docs/specs/REVIEW.md
- https://github.com/imhttran/agentic-sop/blob/main/docs/specs/QUALITY.md
- https://github.com/imhttran/agentic-sop/blob/main/docs/specs/RECOVERY.md
- https://github.com/imhttran/agentic-sop/blob/main/docs/specs/TASK-LIFECYCLE.md
- https://github.com/imhttran/agentic-sop/blob/main/internal/review/review.go
- https://github.com/imhttran/agentic-sop/blob/main/internal/quality/quality.go
