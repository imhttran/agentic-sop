# PLAN — SOP Security Hardening 2

**Plan ID:** `plan-sop-security-hardening-2`
**Repository:** `imhttran/agentic-sop`
**Base:** `origin/main` @ `9cc4bf29` (PRs #259 and #260 merged)
**Execution:** isolated worktree `.worktrees/security-hardening`, branch `security/hardening`.

## 1. Objective

Close the verified gaps left by the shift-left baseline while preserving
report-only mode, the single quality gate, and the routine-overhead target.
Reuse the existing security add-on, adapters, baseline, readiness probe, quality
gate, and isolation harness. Add no framework, gate, approval flow, or download.

## 2. Verified baseline (2026-11-08, this worktree)

- `sop security scan --report-only`: verdict `FAIL`, coverage `PARTIAL`, 5.9 s.
  - gitleaks `COMPLETED` (secrets covered); semgrep `COMPLETED` / `PARTIAL`;
    osv-scanner `FAILED` (database absent); Promptfoo `disabled` in config.
  - Three unsuppressed HIGH `dangerous-exec-command` findings:
    `internal/cli/outcome.go:720`,
    `internal/decisionadapter/providers/julia/runner.go:231`,
    `internal/dqharness/replay_exec.go:273`.
  - 30 reviewed, unexpired suppressions in `.agent-sdlc/security-baseline.json`.
- Semgrep skip class: 746 files, all `*_test.go`, from Semgrep's built-in default
  `.semgrepignore`. `docs/guides/SECURITY-SCANNER-ADDON.md` states test files are
  in declared scope and coverage stays `PARTIAL` unless the repository overrides
  the default.
- `sop security inspect`: sast/secrets available, dependencies `unsupported`,
  `required_ready: false` (no required categories configured).
- macOS Seatbelt runtime isolation tests pass on this host; `bwrap` is absent, so
  Linux checks cannot run here.

## 3. Constraints

1. No tool installs and no rule or advisory-database downloads. OSV provisioning
   is a documented prerequisite that needs explicit operator authorization.
2. Keep `mode: report-only`; do not add required categories or enable blocking.
3. Do not add `security.exclude` entries to manufacture `COMPLETE` coverage.
4. Every claimed result must be executed and cited; otherwise mark `NOT_RUN` /
   `UNVERIFIED`.
5. Keep advisory Promptfoo evaluation out of routine scans; measure scan and
   readiness-probe cost.
6. Reuse existing mechanisms only; no duplicate scanner, evaluator, or gate.

## 4. Tasks

### SEC-H-001 — Baseline and finding triage

**Execution:** read-only inspection plus one report. **Depends on:** none.

1. Run `sop security inspect --json` and `sop security scan --report-only`; record
   per-adapter status, coverage, duration, scope, exclusions, and baseline state.
2. List every unsuppressed finding with severity, rule, path:line.
3. For each HIGH `dangerous-exec-command` site, record the trust boundary: where
   argv comes from, whether a shell is involved, and which existing control
   authorizes it.
4. Record the semgrep skip class, OSV absence, and required-mode projection.

**Acceptance:** `docs/reports/security-hardening/baseline.md` cites executed
evidence and gives each finding a disposition and the mechanism used.

### SEC-H-002 — SAST coverage for in-scope test files

**Execution:** implementation. **Depends on:** SEC-H-001.

1. Add an explicit repository `.semgrepignore` containing `:include .gitignore`,
   overriding Semgrep's built-in default without widening `security.exclude`.
2. Re-run the report-only scan; confirm `sast` coverage becomes `COMPLETE` and
   secrets stay `COMPLETE`.
3. List every newly surfaced finding with severity, rule, and path:line.

**Acceptance:** coverage is complete for sast and secrets with the declared
scope unchanged; new findings are enumerated; scan time is recorded.

### SEC-H-003 — Adjudicate and baseline exec findings

**Execution:** implementation (baseline only). **Depends on:** SEC-H-002.

1. For every unsuppressed HIGH `dangerous-exec-command` finding, production and
   `_test.go`, verify there is no shell interpolation, that argv is derived from
   trusted configuration or a validated/test-controlled value, and that an
   existing control (command policy, sandbox, config boundary) authorizes it.
2. Where the check above fails and a real vulnerability exists, fix that call site
   and add one focused regression; otherwise add a reviewed baseline entry with
   the exact fingerprint from the scan artifact, `rule_id`, `path`, a per-site
   rationale, owner, and an expiry date.
3. Re-run the scan and confirm zero unsuppressed HIGH findings and that no
   unrelated finding changed suppression state.

**Acceptance:** every suppression is exact, reasoned, owned, and expiring; the
unsuppressed HIGH count is zero; no severity or coverage setting was weakened.

### SEC-H-004 — Dependency coverage readiness

**Execution:** verification plus documentation. **Depends on:** SEC-H-001.

1. Verify osv-scanner fails closed when the offline database is absent, and that
   `stale_advisory_database: deny` behavior is covered by the existing tests.
2. Run the existing OSV identity/freshness tests that do not need a real database.
3. Record the supported provisioning command
   (`scripts/security/fetch-osv-databases.sh --dest .agent-sdlc/osv-db`) and that
   running it downloads external advisory data, so it requires operator
   authorization. Do not run it.
4. Record the resulting required-readiness projection.

**Acceptance:** behavior is verified with cited tests, the prerequisite and its
authorization requirement are documented, and no download occurs.

### SEC-H-005 — Required-mode and quality-gate verification

**Execution:** verification. **Depends on:** SEC-H-002, SEC-H-003, SEC-H-004.

1. Confirm the required-mode projection with `sop security scan --required`
   (simulation flag; no config change) and record verdict and exit status.
2. Rely on the existing `internal/quality` tests for the PASS/FAIL/INDETERMINATE
   mapping into the single quality gate.
3. Do not change `mode`, required categories, severity, or coverage settings.

**Acceptance:** verdict and exit status are recorded, blocking remains disabled,
and no configuration changed.

### SEC-H-006 — Isolation, evaluation, and performance

**Execution:** verification. **Depends on:** SEC-H-001.

1. Run the evaltarget and dqharness isolation tests on this host; record macOS
   Seatbelt results and mark Linux Bubblewrap `NOT_RUN` with its prerequisite.
2. Record live-adversarial AISEC as `NOT_RUN` with the evaltarget-routed
   observer prerequisite; do not present fixtures as live evidence.
3. Measure the security scan and the readiness probe (repeat runs), and compare
   with the baseline artifact; state overhead against the `<5%` target and mark
   it `UNVERIFIED` where variance is unmeasured.
4. Confirm the advisory Promptfoo block remains disabled in the routine path.

**Acceptance:** executed results are cited, unsupported checks are `NOT_RUN`, and
measured values are separated from estimates.

### SEC-H-007 — Reconcile, verify, and open the PR

**Execution:** wrap-up. **Depends on:** SEC-H-001 through SEC-H-006.

1. Reconcile the change set with each task's acceptance criteria and remove any
   speculative work.
2. Run focused verification: `gofmt -l`, `go build ./...`, `go vet ./...`,
   `go test ./...`, and one final report-only scan; record commands and results.
3. Write `docs/reports/security-hardening/final.md` with findings, coverage,
   performance, and remaining gaps.
4. Commit through SOP and open the PR against `main` with the existing workflow.

**Acceptance:** the PR and evidence are reviewable, the lifecycle and approval
boundaries remain intact, and no merge or deployment is claimed.
