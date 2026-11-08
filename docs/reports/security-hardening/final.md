# SEC-H-007 — Final report: reconcile, verify, and open the PR

Status: closing stage. The change set is reconciled against the acceptance criteria,
the focused verification suite is run and recorded, and this report records findings,
coverage, performance, and remaining gaps. No gate configuration changed. No merge or
deployment is claimed.

This stage consumes the SEC-H-001..SEC-H-006 reports as stage dependencies. It does
not re-derive their findings.

## Reconciliation record — changed or untracked file to acceptance criterion

Command: `git status --short --branch` — branch `security/hardening...origin/main`.

| File / directory (status) | Owning task | Justifying acceptance criterion | Disposition |
| --- | --- | --- | --- |
| `.agent-sdlc/security-baseline.json` (modified) | SEC-H-002/SEC-H-003 | Reviewed exact-fingerprint suppressions for the adjudicated exec findings; each entry cites the SEC-H-003 disposition and the scan artifact fingerprint. | Keep |
| `.semgrepignore` (untracked) | SEC-H-002 | Test files are in declared scope; overrides Semgrep's built-in default so SAST coverage moves from `PARTIAL` to `COMPLETE` without adding `security.exclude` scope. | Keep |
| `docs/plans/sop-security-hardening-2.md` (untracked) | plan | The plan this change set executes. | Keep |
| `docs/reports/security-hardening/baseline.md` (untracked) | SEC-H-001 | Baseline and finding triage report. | Keep |
| `docs/reports/security-hardening/exec-findings.md` (untracked) | SEC-H-002/SEC-H-003 | Coverage change and exec-finding adjudication report. | Keep |
| `docs/reports/security-hardening/required-mode-readiness.md` (untracked) | SEC-H-005 | Required-mode and quality-gate verification report. | Keep |
| `docs/reports/security-hardening/SEC-H-004-dependency-coverage-readiness.md` (untracked) | SEC-H-004 | Dependency coverage readiness report. | Keep |
| `docs/reports/security-hardening/isolation-evaluation-performance.md` (untracked) | SEC-H-006 | Isolation, evaluation, and performance report. | Keep |
| `docs/reports/security-hardening/final.md` (untracked) | SEC-H-007 | This report. | Keep |

Speculative work removed: none identified. Every changed or untracked file maps to a
task acceptance criterion or a required deliverable. No file was removed, so there is
no removal justification to record. Files outside this change set were not touched.

## Verification record — commands and observed results

All commands ran in this worktree on the SEC-H-007 host (macOS, arm64).

| # | Command | Observed result |
| --- | --- | --- |
| 1 | `gofmt -l .` | exit 0; no file listed. The change set introduced no unformatted Go file. |
| 2 | `go build ./...` | exit 0. |
| 3 | `go vet ./...` | exit 0. |
| 4 | `go test ./...` | exit 0; every package `ok` or `[no test files]`; no failure to diagnose. |
| 5 | `sop security scan --report-only` (final scan, operator-executed) | exit 0; verdict `INDETERMINATE`; coverage `UNKNOWN`; 7156 ms; findings 67, suppressed 53, blocking 0; artifact `.agent-sdlc/runs/security-20261108T173004Z/security/scan.json`. |

The final report-only scan was executed by the operator through the existing
scanner: verdict `INDETERMINATE`, coverage `UNKNOWN`, 7156 ms, findings 67,
suppressed 53, unsuppressed 14 (all MEDIUM, non-blocking), blocking 0. Artifact:
`.agent-sdlc/runs/security-20261108T173004Z/security/scan.{md,json}`. The verdict is
driven by `osv-scanner FAILED` (dependency prerequisite), not by the adjudicated
findings. Required mode is not enabled.

## Findings

- 30 reviewed, unexpired suppressions in `.agent-sdlc/security-baseline.json` (owner
  `imhttran`, expiry `2027-06-30`), plus the 23 reviewed exact-fingerprint entries added
  in SEC-H-003. Source: `baseline.md`, `exec-findings.md`.
- All HIGH `dangerous-exec-command` findings are adjudicated: three production sites
  carry trust-boundary records (T1 `internal/cli/outcome.go`, T2
  `internal/decisionadapter/providers/julia/runner.go`, T3
  `internal/dqharness/replay_exec.go`); the rest are test-only harness code. All are
  baseline-suppressed with exact fingerprints copied from the scan artifact. Source:
  `exec-findings.md`.
- 14 MEDIUM findings remain unsuppressed and advisory because `quality.fail_on` is
  `[critical, high]`. Source: `exec-findings.md`.

## Coverage

- SAST (`semgrep`): `COMPLETE` after the repository `.semgrepignore`. Source:
  `exec-findings.md`.
- Secrets (`gitleaks`): `COMPLETE`. Source: `baseline.md`.
- Dependencies (`osv-scanner`): **FAILED / UNKNOWN**. The adapter is `FAILED` because
  the offline OSV advisory database is absent (`.agent-sdlc/osv-db` missing). No
  vulnerability claim is made from OSV data. Source:
  `SEC-H-004-dependency-coverage-readiness.md`.
- No PASS security verdict is claimed. Required mode stays disabled because dependency
  coverage is incomplete; required mode does not convert incomplete coverage into a
  pass. Source: `required-mode-readiness.md`.

## Performance

Measured values are single-run measurements; variance is UNVERIFIED. Source:
`isolation-evaluation-performance.md`.

| Metric | Baseline | Measured | Status |
| --- | --- | --- | --- |
| Security scan wall-clock | 126423 ms (SEC-LEFT-001, Promptfoo enabled) | 7156 ms and 7044 ms (two final report-only runs, test-inclusive) | MEASURED, spread 112 ms |
| Scan before the test-file coverage override | 126423 ms | 5917 ms (semgrep `PARTIAL`) | MEASURED |
| Promptfoo share | 121006 ms (95.7%) | 0 (disabled) | MEASURED (adapter absent) |
| Readiness probe | n/a | 30–40 ms over 3 runs | MEASURED, spread 0.01 s |

The test-file coverage override adds ~1.2–1.8 s (743 additional Go files) while
removing the last SAST coverage gap. The fixed routine security cost is therefore
about 7.2 s per task, which is under 5% of any task longer than about 2.5 minutes.

The aggregate `<5%` overhead claim is UNVERIFIED because repeat-run variance and the
workload mix were not measured. Required coverage and fail-closed enforcement take
precedence over this target.

## Gate configuration — unchanged

SEC-H-007 changed no gate configuration, severity, coverage, or scope setting.
`.agent-sdlc/config.yaml` still has `security.mode: report-only`, no
`required_categories`, and `quality.fail_on: [critical, high]`. The single quality gate
remains the only verdict authority and blocking stays disabled. Source:
`required-mode-readiness.md`, `.agent-sdlc/config.yaml`.

## Remaining gaps

| Gap | Status | Owning layer | Notes |
| --- | --- | --- | --- |
| osv-scanner dependency adapter | PARTIAL / UNKNOWN | Security add-on dependency adapter | Offline OSV database absent. Provisioning requires explicit operator authorization: `scripts/security/fetch-osv-databases.sh --dest .agent-sdlc/osv-db`. Not run. |
| Linux Bubblewrap (`bwrap`) runtime isolation | NOT_RUN | Host OS runtime / execution environment | `bwrap` absent on this macOS host; Linux-only confinement tests skip. macOS Seatbelt results are cited instead. |
| Live-adversarial AISEC observer | NOT_RUN | evaltarget / AISEC integration owners | Requires an evaltarget-routed live path whose observer reads back a model tool decision. No fixture run is presented as live evidence. |
| Advisory Promptfoo evaluation | Disabled in the routine path | Security add-on evaluator config | `security.evaluation.enabled: false`. Not enabled in this plan. |
| 14 advisory MEDIUM findings | Advisory, non-blocking | Security review | `quality.fail_on` is `[critical, high]`; findings are listed in `exec-findings.md`. |

## Lifecycle and approval boundaries

- This stage stops at opening the PR against `main` through the existing workflow.
- The PR text carries the change summary and this verification record.
- Neither the PR text nor this report claims a merge or a deployment.
- `human.approval_before_commit: true` remains in force; commit goes through SOP.
- No file under `.agent-sdlc/` was modified except the security baseline and
  task-scoped run artifacts.
