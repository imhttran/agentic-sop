# PLAN — SOP Autonomous Outcome Execution

**Priority:** P0  
**Status:** PROPOSED UPDATE — reconcile with active `plan-sop-autonomy` before activation  
**Branch/worktree:** `feature/autonomy-001` / `sop-autonomy-001`  
**Milestone:** AUTONOMY-001 — One Prompt → One Autonomous Execution → One Verified Outcome

## 1. Objective

Given one authorized engineering objective (for example, “Clean up unused code”), SOP should select a sound implementation path, execute it, recover within bounded limits, independently verify the result, and return one consolidated outcome. **The agent owns routine engineering decisions; SOP enforces deterministic governance; the human owns exceptional authorization decisions.** Do not require the operator to repeatedly tighten the plan or approve normal implementation choices.

## 2. Verified starting point and remaining gaps

The isolated branch at `d9f7722` already has a governed one-prompt path (`sop prompt --capability implement` → `runPromptImplement` → `runAttempts`), autonomy policy, scheduler, recovery, routing, and approvals. A real-model disposable multi-package cleanup removed four unused symbols and passed build/vet while respecting the commit boundary. Existing autonomy/recovery/approval tests pass. **Do not rebuild these capabilities or add a second orchestrator.**

Known gaps from the independent report:
- `PASS` / `LOCAL_DONE` does not guarantee independently enforced acceptance on this branch; `harden-001` has separate, unmerged experimental work. Do not silently merge it.
- No automated in-repo multi-package outcome regression; existing real-model scenario evidence is operator-run.
- Outcome reporting is fragmented; no single reliable workspace-result summary.
- The governed baseline contains incorrect code references and fixture details; preserve original evidence, add an erratum.
- Two competing plans exist: `PLAN-SOP-Autonomy.md` (activated in isolated state) and `PLAN-SOP-AUTONOMY-001.md` (broader proposal, located in main worktree at review time). Consolidate without overwriting another worktree or falsifying lifecycle history.

## 3. Autonomous Decision Contract (NEW — central requirement)

### 3.1 Authorization envelope

At execution start, record the objective, repository/worktree, allowed files and operations, protected paths, validation requirements, recovery/time/attempt budgets, external side-effect permissions, and required approval boundaries. Defaults must be conservative. Missing authorization is **not** implicit permission.

### 3.2 Decisions the agent makes without asking

Within the authorization envelope, the agent **MUST** choose and document a reasonable approach, then continue for:
- Selecting among equivalent designs and choosing the smallest viable change.
- Reconciling duplicate tasks, refining task instructions, and ordering dependent work.
- Running read-only discovery, inspecting source/docs/tests, and creating disposable fixtures.
- Editing authorized code and documentation, correcting implementation defects, and adding tests necessary to satisfy agreed acceptance criteria.
- Running approved build/test/static checks; bounded retry, recovery, and configured model fallback.
- Deferring ambiguous or unsafe cleanup candidates while completing safe candidates.
- Using parallel tasks when dependencies and write ownership are non-overlapping; otherwise executing sequentially.

The agent records *decision, alternatives considered, evidence, and reason* in a compact decision log. Do **not** request human confirmation merely because more than one valid technical approach exists.

### 3.3 Decisions SOP enforces deterministically

SOP validates the authorization envelope, task dependencies, attempt and resource budgets, approval state, protected paths, deterministic checks, acceptance evidence, and lifecycle transitions. A model-generated confidence score, recommendation, or `PASS` cannot override a deterministic block. Existing decision adapters may **rank** choices but cannot grant permissions.

### 3.4 Human-only boundaries

Stop and ask only when needed for:
- Commit, push, merge, release, publish, deployment, or external side effects not explicitly authorized by policy.
- Destructive or irreversible actions, sensitive data exposure, security/credential changes, or protected-file modifications outside the envelope.
- Material objective/scope expansion or an unresolved ambiguous product contract with incompatible outcomes.
- Exhausted recovery budgets, non-recoverable failures, or a deterministic policy conflict.

For a partial safe outcome, finish and report safe work instead of treating every uncertain candidate as a global blocker. If a stop is necessary, present **one concise decision request** with recommended option, alternatives, impact, and exact resumption action. Never bypass the boundary.

### 3.5 Anti-loop and completion rules

- One initial authorization should cover all ordinary implementation and verification work.
- Do not generate a new “tightening” plan solely because the current report identifies minor improvements; classify them as `required_for_acceptance`, `optional_followup`, or `out_of_scope`.
- If `required_for_acceptance`, resolve autonomously within the envelope and budget; if optional, record it and finish.
- A failed gate or unmet acceptance criterion is not a reason to ask the operator to write the next prompt. Diagnose, choose a bounded fix, and rerun.
- Never mark the outcome `VERIFIED` solely because a governed task is `LOCAL_DONE`.
- No endless retries or plan expansion: stop on the defined budget and give a precise HOLD reason.

## 4. Implementation work (only demonstrated gaps)

### AUTONOMY-001 — Plan reconciliation and decision authority

**Deliverables:** one authoritative plan, documented authorization envelope, decision matrix, and an auditable decision log. Reconcile the two plan documents without rewriting past SOP task state or modifying the main/hardening/preview worktrees. Preserve prior plans as historical evidence or cross-references as appropriate.

**Acceptance:** an authorized low-risk technical ambiguity is resolved without human interaction; an out-of-scope or protected action produces an explicit stop; no duplicate feature implementation.

### AUTONOMY-002 — Autonomous continuation and bounded judgment

Trace the existing prompt/graph lifecycle and identify **actual** places where routine engineering decisions trigger an avoidable human stop. Implement the smallest adapter/policy/wiring fix only where reproduced. Reuse `internal/autonomy`, CLI, scheduler, recovery, and approval machinery. Preserve retry budgets and recovery evidence.

**Acceptance:** a deterministic fixture with a reversible implementation choice, a recoverable failure, and a follow-up validation step completes without intermediate human input; exhaustion and protected actions remain blocked. If existing code already satisfies a criterion, prove it and mark `NO_CHANGE_REQUIRED`.

### AUTONOMY-003 — Independent outcome acceptance

Implement a minimal outcome-level verifier or equivalent composition of existing deterministic gates that checks **explicit objective acceptance criteria** independently of model self-report. Scope it to the approved milestone; do not import or merge `harden-001` wholesale. If reliable generic verification is not feasible, implement a narrow, truthful acceptance contract for the cleanup fixture and mark broader enforcement `PARTIAL`.

**Acceptance:** deliberately missing deliverables or inaccurate acceptance claims cannot produce an outcome status of `VERIFIED`; a correct result can. Preserve underlying task gate and original artifacts.

### AUTONOMY-004 — Automated regression and outcome report

Create an in-repo, deterministic, disposable multi-package cleanup regression with an authorized change and a deliberately protected/uncertain candidate. Use deterministic agent responses or a supported fake harness for CI; retain the existing real-model run as supplemental evidence, not a CI prerequisite. Emit one consolidated outcome summary with changed files, decisions, tests, acceptance, recovery, skipped candidates, approvals, and remaining risks. Fix baseline documentation via an erratum rather than rewriting history.

**Acceptance:** the fixture verifies safe changes, compatibility/protection, deterministic gates, and accurate reporting; failure injection produces HOLD instead of false success; no leftover generated binary or fixture pollution.

### AUTONOMY-005 — Optional UX seam (conditional)

The existing `sop prompt --capability implement` is the default single-prompt entry point. Add a new command or wrapper **only** if a reproducible usability gap prevents the outcome contract. Otherwise document the supported invocation and mark `NOT_REQUIRED`.

## 5. Dependencies and parallel execution

1. **First:** read-only reconciliation of active plan, SOP state, branch, and current behavior; finalize the authorization envelope and measurable acceptance criteria.
2. **Parallel where safe:** independent research/fixture design, acceptance verifier design, and outcome-report design. Assign non-overlapping files; do not concurrently edit shared CLI orchestration files.
3. **Then:** integrate minimal wiring changes and run deterministic scenario(s).
4. **Finally:** independent acceptance review and one outcome report.

Use existing SOP plan activation/reconciliation mechanisms. Because `plan-sop-autonomy` is already ACTIVE in the isolated worktree, do **not** activate a competing plan or implicitly archive it as complete. If reconciliation cannot be performed safely with supported lifecycle commands, preserve state and request a single governance decision.

## 6. Validation

Run `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test -count=1 ./...`, `go test -race -count=1 ./...`, and `git diff --check` as appropriate to the modified repository. Capture exact commands, results, and environmental blockers. Test three explicit outcomes:

1. **Autonomous success:** one prompt → authorized cleanup → tests → independent acceptance → `VERIFIED`, no intermediate approval.
2. **Autonomous recovery:** a transient or correctable failure → bounded fix/retry → evidence-backed completion, no operator prompt.
3. **Real authority boundary:** protected/destructive/out-of-scope change → stop with one actionable human decision, no side effect.

## 7. Outcome contract

The final status is exactly one of:
- **VERIFIED:** all mandatory criteria independently met; required deterministic checks pass.
- **PARTIAL:** safe authorized work completed, but named criteria remain unmet or unverifiable.
- **HOLD:** a genuine authority boundary, non-recoverable failure, or exhausted budget prevents continuation.

The report must include objective, scope, chosen approach, changes, verification evidence, attempts/recoveries, protected/skipped items, outstanding decisions, and optional follow-ups. Distinguish task `PASS`/`LOCAL_DONE` from verified objective completion.

## 8. Explicit non-goals and safety

No second orchestration engine; no broad hardening campaign; no speculative model/agent additions; no rewriting evidence; no changes to other worktrees; no copying `.agent-sdlc` state or secrets; no commit, push, merge, deployment, or publication without explicit authorization. Do not treat probability as authorization. Avoid additional operator checkpoints for routine authorized decisions.

## 9. Definition of done

AUTONOMY-001 is complete when a single authorized prompt drives a representative engineering outcome through decisions, bounded recovery, deterministic checks, independent acceptance, and a truthful consolidated result **without the user managing intermediate implementation choices**. A protected action must still stop safely. If a capability already exists, prove it rather than rewriting it. If a remaining issue is optional, record it without creating another mandatory tightening pass.
