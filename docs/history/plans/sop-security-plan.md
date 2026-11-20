# PLAN — SOP Security Shift Left

**Plan ID:** `plan-sop-security-shift-left`  
**Repository:** `https://github.com/imhttran/agentic-sop`  
**Status:** IMPLEMENTED / MERGED via PR #260 (`9cc4bf29`) on 2026-11-08. The execution evidence is preserved in `docs/reports/security-shift-left/final.md`; local SOP lifecycle state is not inferred from this repository status.  
**Scope:** Audit and improve existing SOP security readiness, early feedback, enforcement, AI-agent evaluation, and execution isolation.  
**Execution:** New, isolated Git worktree and branch based on current `origin/main`.

## 1. Objective

Move security feedback earlier in SOP's existing lifecycle, make the current controls measurable and enforceable, and improve assurance against unauthorized agent behavior. **Reuse the existing `security` add-on, command policy, validation runner, quality gate, approval boundary, and evidence infrastructure.** Do not introduce another security framework, quality gate, approval workflow, scanner abstraction, or model decision authority.

The plan is evidence-driven: inspect current source and runtime configuration first. Treat the repository state, implementations, and tests as authoritative over this document. Any suspected gap that is already closed must be marked `ALREADY_SATISFIED` with evidence, not reimplemented.

## 2. Confirmed baseline from repository documentation (reverify at execution)

- `sop security inspect` and `sop security scan` exist; security is opt-in and supports `report-only` and `required` modes.
- Semgrep CE, Gitleaks, OSV-Scanner, and cargo-audit adapters are implemented. The scanner contract includes explicit coverage, pinned-version checks, normalized findings, offline database identities and freshness, baselines, and content-based resume invalidation.
- The scanner's required-mode evidence is integrated into the **existing** quality gate. No new verdict authority is needed.
- The inspected project config had `security.enabled: true`, `security.mode: report-only`, with Gitleaks, Semgrep, OSV and local Promptfoo configured. **Do not assume this remains current or that the tools/rules are operational.**
- Promptfoo currently has a bounded local smoke-test configuration, not a live-model adversarial suite. Six AISEC scenarios have deterministic/real-control tests; the documentation reports live adversarial coverage as not implemented.
- Command policy and authorization are deterministic. Linux isolated replay has Bubblewrap protection. Distinguish isolated replay from production agent execution; do not generalize its protection to all execution paths or platforms. macOS runtime isolation claims need evidence.
- The normative security scanner specification describes scans at the existing quality seam, after implementation. Establish whether any earlier readiness checks already exist before adding behavior.

Primary references:

- `docs/specs/SECURITY.md`
- `docs/specs/SECURITY-SCANNER-ADDON.md`
- `docs/specs/EXECUTION.md`
- `docs/specs/AGENT-PROVIDER.md`
- `docs/specs/QUALITY.md`
- `docs/security-promptfoo.md`
- `docs/reference/CONFIGURATION.md`
- `docs/reference/CLI.md`
- `.agent-sdlc/config.yaml` (local/runtime configuration may differ)

## 3. Constraints and decision boundaries

1. **Use a new worktree:** Fetch `origin`, create a dedicated branch and worktree based on current `origin/main`. Do not touch, reset, clean, or overwrite other worktrees, branches, or uncommitted user changes. Follow the repository's worktree/trunk conventions.
2. **Use SOP's lifecycle:** Execute this plan under the current SOP scheduler, verification, review, acceptance, approval and PR workflow. Do not bypass existing gates. Do not fabricate `PASS` or claim validation that was not executed.
3. **Autonomy:** Proceed end-to-end within approved scope. Do not seek routine approvals. Stop only for a genuine SOP gate, security-sensitive action requiring authorization, destructive operation, scope change, or unrecoverable blocker; record the blocker and the exact next action.
4. **Security policy:** Do not install tools, download Semgrep rules/advisory databases, transmit source or secrets, enable remote model evaluation, or relax isolation without explicit authorization. Use already installed and pinned local tooling. If not available, report a readiness blocker rather than silently substitute.
5. **No speculative hardening:** Prefer a read-only finding to new code if the existing implementation already satisfies the requirement. Add only a narrowly demonstrated deficiency with a reproducible regression test.
6. **One quality gate:** Required scanning must feed the existing quality seam. Earlier preflight is about readiness and policy, not a second completion authority. Keep baseline exceptions reviewed, justified, expiring, and auditable.
7. **Preserve settings:** Do not silently convert a user's local `report-only` config to `required`. Propose the exact change and impact; apply it only if within approved repository scope and a demonstrated clean readiness baseline. Do not add mandatory checks for all unrelated projects.
8. **Time and cost bounds:** Use focused verification while implementing. Rely on the established PR review/CI process for complete verification; avoid repetitive, full-suite runs without evidence of need.
9. **Performance invariant:** Security changes must not materially slow ordinary SOP execution. Measure before/after elapsed time on comparable representative tasks (same environment, model class, workload, and warm/cold conditions); record the methodology and variance. Target **<5% incremental wall-clock overhead for ordinary tasks** as a proposed acceptance threshold, not a claimed measurement. If measurement is unavailable or noisy, report `UNVERIFIED` and do not claim the target was met.
10. **Critical-path discipline:** Reuse existing scanner resume/provenance invalidation before adding any caching. Run cheap deterministic prerequisite checks at preflight; avoid duplicate full scans. Keep Promptfoo/live AISEC adversarial evaluation outside the normal task loop (dedicated evaluation or relevant security-control changes). Preserve required-mode fail-closed behavior and complete coverage: optimization must not skip required scans, hide partial/unknown coverage, or bypass the quality gate.

## 4. Task graph

```text
SEC-LEFT-001 Baseline and gap evidence
      |
      +--> SEC-LEFT-002 Enforce existing scanner policy (conditional)
      |
      +--> SEC-LEFT-003 Early readiness/preflight (conditional)
      |
      +--> SEC-LEFT-004 Live AISEC evaluation (conditional)
      |
      +--> SEC-LEFT-005 Isolation coverage and verification (conditional)
                         |
                         v
                SEC-LEFT-006 Reconcile evidence, PR and handoff
```

Tasks 002–005 may be executed independently after Task 001; do not expand their scope beyond demonstrated gaps. Task 006 depends on all applicable tasks.

### SEC-LEFT-001 — Baseline and gap evidence

**Execution:** inspect / read-only (other than creating plan-owned evidence files in the isolated worktree).  
**Depends on:** none.

**Steps**

1. Capture branch, commit SHA, tracked/untracked state, current SOP installation/version and relevant configuration origins without exposing credentials.
2. Inspect `internal/security`, `internal/cli/security_gate.go`, `internal/quality`, `internal/commandpolicy`, `internal/evaltarget`, `internal/dqharness`, and corresponding tests/specifications. Map each control to the lifecycle stage where it actually runs.
3. Run `sop security inspect --json` from the isolated worktree, using the current supported invocation/config. Record adapter availability, version pins, language/rule coverage, source scope, excluded paths, lockfiles, local database freshness, and missing prerequisites.
4. If safe and available, execute `sop security scan --report-only` once. Record the **overall** verdict and **per-adapter** status/coverage, including `FAIL` versus `INDETERMINATE`; do not interpret the absence of findings as full coverage.
5. Check whether a baseline entry is masking actionable findings, whether its expiry and rationale are valid, and whether a specific requirement is already addressed.
6. Capture a performance baseline for representative ordinary tasks: end-to-end task wall-clock time, precheck duration, security scan duration, scan count, and whether evidence was reused. Use existing run telemetry and artifacts where possible; avoid rerunning costly work solely for benchmarking.
7. Produce `docs/reports/security-shift-left/baseline.md` containing an evidence table: control, actual implementation, when exercised, configuration state, evidence reference, gap classification (`CONFIRMED`, `UNVERIFIED`, `ALREADY_SATISFIED`), impact and minimal next action. Link available run artifacts without committing sensitive or ignored runtime reports.

**Acceptance:** A current, reproducible baseline identifies coverage and enforcement gaps without guessing. Existing timing evidence is captured or explicitly marked unavailable. Each remaining implementation task has an evidence-backed trigger or is explicitly marked not needed. No destructive operations or unapproved provisioning.

### SEC-LEFT-002 — Existing scanner policy enforcement

**Execution:** conditional implementation/config/documentation.  
**Depends on:** SEC-LEFT-001.

**Steps**

1. Verify `required` mode already integrates with the existing quality gate, including `INDETERMINATE`/partial coverage, missing adapters, stale databases, and severity handling.
2. Inspect the effective configuration used by actual SOP runs and CI/PR review—not only a sample or ignored local file. Determine where report-only enforcement is intentional.
3. Estimate required-mode impact from existing scan duration and reuse evidence; distinguish additional security work from work already performed in report-only mode. If the scanner baseline is ready, propose the smallest policy/configuration change to make agreed categories required in the appropriate scope. Avoid blanket enablement across unrelated repositories.
4. If not ready, record precise prerequisites and leave report-only in place. Do not weaken severity, required coverage, or freshness to obtain `PASS`.
5. If a confirmed enforcement bug exists, fix only that wiring and add a focused regression proving `PASS` proceeds and `FAIL`/`INDETERMINATE` cannot pass the existing gate.

**Acceptance:** The desired enforcement state and exact policy delta are documented; required-mode behavior is demonstrated where executable. No duplicate gate or automatic security finding suppression is created.

### SEC-LEFT-003 — Shift readiness checks earlier

**Execution:** conditional implementation.  
**Depends on:** SEC-LEFT-001.

**Steps**

1. Inspect current precheck, plan/task preparation, and `security inspect` capabilities. Identify only security prerequisites that can be validated deterministically **before** invoking the coding agent: configuration validity, adapter executability/version pins, rules availability, language coverage, and database presence/freshness.
2. Reuse existing inspection/configuration logic. Add the narrowest early call/diagnostic at the existing precheck seam, without a second scanner execution or lifecycle verdict. Avoid repeating inspection when the same validated configuration and content identity are already available. Avoid running full SAST/dependency scans before code exists.
3. Preserve opt-in behavior and explicit required/report-only semantics. Missing advisory tools must not block unrelated report-only tasks. In required mode, unsatisfied prerequisites must fail closed using existing failure classification or a documented existing status.
4. Add focused tests for ready, missing-tool, mismatched-version, absent rules, stale database, and disabled-security paths where applicable. Do not duplicate adapter parsing or quality policy.

**Acceptance:** Demonstrably invalid required security prerequisites are detected before IMPLEMENT, with actionable diagnostics. Ordinary tasks do not incur extra mandatory approval loops or redundant scanner runs.

### SEC-LEFT-004 — Live AISEC scenarios using existing evaluation infrastructure

**Execution:** conditional implementation/evaluation.  
**Depends on:** SEC-LEFT-001.

**Steps**

1. Reuse the existing six AISEC scenario definitions and real production authorization/command-policy interfaces. Do not build a second evaluator.
2. Assess and document how the Promptfoo adapter can exercise genuine live-model tool decisions and observe actual authorized/denied operations. A text assertion or fixture-only test is **not** equivalent to live adversarial evidence.
3. Add bounded, local-only evaluation cases for AISEC-001 through AISEC-006 if the existing infrastructure supports them safely. Include positive control AISEC-006 and negative controls for unauthorized attempts. Keep test prompts inert and do not use real credentials or risky operations.
4. Capture scenario ID, model/provider/version, local endpoint policy, test-input digest, observed tool/control outcome, expected behavior, repeat count, errors and artifact identity. Explicitly separate deterministic, real-control, live-smoke and live-adversarial evidence.
5. If no suitable live model or safe execution path exists, land only reproducible fixtures/runner integration and mark live validation `NOT_RUN` with a concrete prerequisite. Do not mislabel it `PASS`.
6. Keep evaluation advisory and downstream of lifecycle authority; model output cannot grant authorization or promote task state. Do not invoke live adversarial cases on every ordinary task; reserve them for dedicated evaluation and relevant security-control changes.

**Acceptance:** Each scenario has reproducible expected behavior, truthful evidence classification, and observed results where a local model was available. No unauthorized action becomes executable during evaluation.

### SEC-LEFT-005 — Isolation boundary verification

**Execution:** read-only verification first; conditional defect fix.  
**Depends on:** SEC-LEFT-001.

**Steps**

1. Inventory **each** execution path: normal agent tool calls, configured agent commands, verification commands, isolated replay, and evaluation-target commands. Record which paths use OS isolation, command policy only, or operator-trusted configuration.
2. Verify Linux Bubblewrap isolation using existing safe regression fixtures and runtime tests where available. Include filesystem escape and host Unix-socket denial only within disposable, isolated test environments. Do not touch user-owned secrets or services.
3. Check macOS Seatbelt test/runtime evidence separately. If only command/profile construction is tested, retain the `UNVERIFIED` runtime designation.
4. Fix only confirmed unsafe execution-path defects in the approved scope. Add focused regression evidence. Do not claim that isolated replay implies all coding-agent execution is sandboxed.
5. Document supported platforms, fallback behavior, fail-closed conditions and known limits.

**Acceptance:** Every relevant execution path has a clearly stated trust boundary and supporting evidence or an explicit unverified status. No overbroad isolation claims.

### SEC-LEFT-006 — Reconcile, review, PR and handoff

**Execution:** implementation wrap-up and SOP-governed PR.  
**Depends on:** all applicable prior tasks.

**Steps**

1. Reconcile actual changes with the SEC-LEFT-001 baseline and each task's acceptance criteria. Remove speculative work and mark no-op tasks `ALREADY_SATISFIED` only through SOP's supported evidence path.
2. Run focused verification for changed areas; record command, exit code, commit/content identity and evidence references. Compare before/after timings for representative ordinary runs using equivalent conditions, and report security scan execution/reuse counts; separate measured results from estimates. Execute the repository's existing review and PR workflow. Do not manually add redundant verification gates.
3. Open a PR against `main` from the dedicated branch. Keep the working branch/worktree intact unless existing SOP cleanup conditions explicitly authorize removal.
4. Write `docs/reports/security-shift-left/final.md`: before/after controls, findings resolved, remaining gaps, enabled enforcement vs proposed changes, live-model results or not-run prerequisites, isolation boundary matrix, **performance comparison (wall-clock, scanner cost, reuse rate, measurement limitations)**, evidence pointers, risks and decisions.
5. Report PR URL, branch/worktree path, commits, test outcomes, SOP state, open blockers, and next operator actions. Do not claim merged or deployed unless verified.

**Acceptance:** The PR and evidence make the specific shift-left impact reviewable. Established SOP lifecycle, approval and merge protections remain intact.

## 5. Success criteria (plan-level)

- A current security baseline exists, backed by actual scanner/adapter and source evidence.
- Required scanning uses the existing quality gate and does not silently pass incomplete, missing, or stale required coverage.
- Required security prerequisites are identified early enough to prevent avoidable agent attempts, without duplicating scanners or creating new approvals.
- AISEC coverage distinguishes fixtures, real-control tests, live smoke and live adversarial evidence; all six scenarios are covered or individually documented as blocked/not run.
- Isolation claims are limited to execution paths and platforms actually verified.
- No credentials, scanner rules with restricted licenses, generated secret-bearing reports, or ignored local configuration are committed.
- Routine SOP task overhead is measured against baseline, with a proposed target of **<5% additional wall-clock time** under equivalent conditions; unmeasured or inconclusive results are marked `UNVERIFIED`. Required security coverage and fail-closed enforcement take precedence over the target.
- Expensive adversarial evaluations remain outside the ordinary task critical path; identical scans are not repeated when the existing safe resume mechanism can reuse valid evidence.
- Changes are isolated in a new worktree and delivered via the standard SOP-governed PR lifecycle.

## 6. Agent handoff prompt

> Execute `PLAN-SOP-Security-Shift-Left.md` in `imhttran/agentic-sop`. Fetch `origin/main` and create a **new isolated worktree and feature branch**. Preserve all existing branches, worktrees, and uncommitted files. Use SOP's existing plan/task, verification, review, approval and PR process. Start with SEC-LEFT-001 and use its evidence to determine which subsequent tasks actually require changes. Reuse the existing security adapter, inspection, quality gate, agent evaluator, command policy and isolation paths. Do not add redundant checks or new governance frameworks, do not provision or download tools/rules/databases or transmit data without authorization, and do not silently change local security policy. Keep the ordinary execution path lightweight: reuse existing scan evidence, avoid duplicate scans, and run live AISEC evaluations outside the normal task loop. Measure before/after task overhead against a proposed <5% target without weakening required coverage. Continue autonomously through an open PR unless a genuine gate or blocker requires a decision. Provide the PR, evidence, remaining gaps and any blocked prerequisite.
