# SEC-LEFT-001 — Baseline and gap evidence

Status: baseline evidence report. Read-only inspection and reporting. No tracked
behavior changed.

Revision note (2026-11-07): this report was rewritten under operator-authorized
remediation. The first generated draft stated that the security add-on was
unconfigured and that inspect/scan were `NOT_RUN`; that was incorrect. SOP's own
quality-stage scan ran during the task and its artifact is authoritative. This
revision cites that artifact and a read-only `sop security inspect --json` from
the same worktree. No scanner was re-run and no tool, rule, or database was
downloaded.

## Environment

- Worktree: `.worktrees/security-shift-left`; branch `security/shift-left`.
- Base: `origin/main` @ `1467801242d4e63492e324a982d7ef36e590e76d`.
- Scan content identity: commit `1467801242d4e63492e324a982d7ef36e590e76d`,
  tree `54cca413a37f225ae1a70e7b58fe8c9eae7d44a6`, working tree DIRTY (the plan
  source is untracked), digest `354a02188162ec72...`.
- Tracked configuration: `.agent-sdlc/config.yaml` — `security.enabled: true`,
  `mode: report-only`, `scope: workspace`, `baseline:
  .agent-sdlc/security-baseline.json`, `exclude: [.git, .agent-sdlc,
  .run/reference, internal/security/testdata, testdata]`, and pinned adapters
  gitleaks `8.30.1`, semgrep `1.183.0`, osv-scanner `2.7.0`.
- Local tooling present and matching the pins: gitleaks `8.30.1`, semgrep
  `1.183.0`, osv-scanner `2.7.0`, promptfoo `0.118.0`.
- Machine-local Semgrep rules exist under `.agent-sdlc/security-rules` (Go
  rules) and were reproduced in the isolated worktree. `.agent-sdlc/osv-db`
  does not exist on this machine.

## Control evidence

| Control | Actual implementation | When exercised | Configuration state | Evidence | Classification |
| --- | --- | --- | --- | --- | --- |
| Quality-stage security gate | `runSecurityGate` builds a `security.Policy` from `cfg.Security`, selects adapters, runs `security.NewRunner(...).Scan`, writes artifacts, and shapes `quality.SecurityEvidence` for the single quality gate. | End of a task run, at the existing quality seam. | Enabled, `report-only`. | `internal/cli/security_gate.go`; `internal/security/runner.go`; run artifact `security/scan.json` | ALREADY_SATISFIED |
| gitleaks (secrets) | `internal/security/gitleaks.go` built by `security.BuildAdapters`; explicit selection. | Quality seam, per scan. | Installed and pinned; enabled. | `security/scan.json`: `COMPLETED`, coverage `COMPLETE`, 1830 ms, no leaks. | ALREADY_SATISFIED |
| semgrep (sast) | `internal/security/semgrep.go`, coverage and language accounting; explicit selection. | Quality seam, per scan. | Installed and pinned; local Go rules configured. | `security/scan.json`: `COMPLETED`, coverage `PARTIAL`, 3324 ms, 2113 files scanned with 59 rules. | UNVERIFIED (coverage gap) |
| osv-scanner (dependencies) | `internal/security/osv.go`; offline database identity and freshness checks. | Quality seam, per scan. | Pinned; `osv_database_path: .agent-sdlc/osv-db`; database absent. | `security/scan.json`: `FAILED`, coverage `UNKNOWN`, 14 ms — "OSV database cache is unavailable: .agent-sdlc/osv-db: no such file or directory". | CONFIRMED (gap) |
| cargo-audit (dependencies) | `internal/security/cargoaudit.go`. | Not exercised: no Rust source and no adapter configured. | Not configured. | `sop security inspect`: not selected. | UNVERIFIED (inert) |
| Promptfoo evaluation | `internal/security/promptfoo.go`, appended by `BuildAdapters` from the `security.evaluation` block. | Quality seam, on every scan while evaluation is enabled. | Enabled: local profile, `qwen3.5:4b` via local Ollama, `report_only: true`, `severity_mode: none`. | `security/scan.json`: `COMPLETED`, coverage `COMPLETE`, 121006 ms. | CONFIRMED (runtime cost) |
| Baseline suppressions | `internal/security/baseline.go` (`LoadBaseline`, `Baseline.Apply`); exact fingerprint, owner, reason, `expires_at`; expired entries stay visible and stop suppressing. | Quality seam, before adapter execution. | `.agent-sdlc/security-baseline.json` present and tracked. | `security/scan.json`: 30 of 33 findings suppressed; `suppression.owner = imhttran` on each. | ALREADY_SATISFIED |
| Command policy / authorization | `internal/commandpolicy` allow-list, deterministic; wired into tool execution. | Every tool invocation. | Deterministic; no model authority. | `internal/commandpolicy/*.go`; SEC-LEFT-005 inventories per-path trust boundaries. | UNVERIFIED (per-path coverage) |
| AISEC-001..006 scenarios | `internal/evaltarget` definitions plus deterministic and real-control tests. | Evaluation tests, not the task security gate. | Live-model adversarial coverage absent. | `internal/evaltarget/*.go`; SEC-LEFT-004 owns live classification. | UNVERIFIED |
| Linux Bubblewrap isolated replay | `bwrap` protection on the isolated replay path only. | Isolated replay. | Linux only. | `internal/security/*`; SEC-LEFT-005 owns the matrix. | UNVERIFIED (scope limits) |
| macOS Seatbelt runtime isolation | No runtime isolation evidence found. | Not exercised. | Construction only, if any. | Searches of `internal/security` and `docs/specs/EXECUTION.md`. | UNVERIFIED |

## Scan result (authoritative)

From `.agent-sdlc/runs/SEC-LEFT-001/security/scan.{md,json}`:

- Verdict: `FAIL`. Coverage: `PARTIAL`. Mode: `report-only`. Scope: `workspace`.
  Duration: `126423 ms`.
- `3 unsuppressed finding(s) at a blocking severity`.
- Findings: 33 total (19 `HIGH`, 14 `MEDIUM`); 30 suppressed by the tracked
  baseline; 3 unsuppressed `HIGH` `dangerous-exec-command` findings:
  - `internal/cli/outcome.go:720`
  - `internal/decisionadapter/providers/julia/runner.go:231`
  - `internal/dqharness/replay_exec.go:273`
- Coverage by category: `sast: partial` (Go only; semgrep reported that files
  matching `.semgrepignore` were skipped, 743 files), `secrets: covered`
  (repository-wide), `dependencies: failed` (osv-scanner `FAILED`).
- Rust is declared in the Semgrep adapter configuration but was not detected in
  this repository and is not covered by the local rule set.
- Required-mode projection: `ready: false`, verdict `INDETERMINATE`, reason
  `required mode needs at least one required category`. Required mode is not
  configured on this repository.

## Read-only inspect projection vs scan result

`sop security inspect --json` (read-only, no scanner execution) reports
`enabled: true`, `mode: report-only`, languages `[go]`, `go.sum` present, and
selected adapters gitleaks `available`, semgrep `available`, osv-scanner
`unsupported` ("OSV offline database is not configured or unavailable"). Its
coverage projection is `sast: covered`, `secrets: covered`,
`dependencies: unsupported`.

The scan is authoritative: readiness projection says `sast` is covered, but the
executed scan observed `PARTIAL` coverage because `.semgrepignore` excluded
files. A readiness probe therefore cannot substitute for the required-mode
verdict; it can only fail closed before IMPLEMENT when prerequisites are
demonstrably missing.

## Baseline and suppressions

- `.agent-sdlc/security-baseline.json` is present, tracked, and active: 30
  findings were suppressed under a single owner with a recorded reason and
  expiry. Expiry is enforced mechanically on load; an expired entry remains
  visible and no longer suppresses.
- Because 3 blocking findings remain unsuppressed, the baseline is not hiding the
  control gap. Any new entry must carry a reviewed rationale and expiry.

## Timing baseline

Captured from the same run artifacts; no additional benchmark run was performed.

| Metric | Value | Source |
| --- | --- | --- |
| Security scan wall-clock | 126423 ms | `scan.json` `metrics.duration_ms` |
| Promptfoo share | 121006 ms (95.7% of scan) | `metrics.adapter_duration_ms.promptfoo` |
| Semgrep | 3324 ms | `metrics.adapter_duration_ms.semgrep` |
| gitleaks | 1830 ms | `metrics.adapter_duration_ms.gitleaks` |
| osv-scanner | 14 ms (failed) | `metrics.adapter_duration_ms["osv-scanner"]` |
| Scans per task | 1 | run artifacts |
| Evidence reuse | none (no resume) | run artifacts |
| Task wall-clock (`SEC-LEFT-001`) | 3 m 3 s including one retry | `sop report SEC-LEFT-001` |

Method and limitations: single run, warm Go build cache, one machine (macOS,
arm64), `report-only` mode, dirty working tree. Variance was not measured, so the
proposed `<5%` ordinary-task overhead target is `UNVERIFIED`; no before/after
comparison is claimed here.

Promptfoo dominates routine security cost. It is advisory (`severity_mode:
none`, `report_only: true`) and is appended to every quality-stage scan while
`security.evaluation.enabled` is true, which is outside the plan's requirement to
keep expensive evaluation off the ordinary task critical path. Moving it out of
routine scans does not weaken required coverage because evaluation is not a
required category and contributes no verdict.

Post-baseline change: `.agent-sdlc/config.yaml` now sets
`security.evaluation.enabled: false` so this cost leaves the routine scan path.
The measurements above describe the pre-change state; `final.md` records the
measured after-change scan (5775 ms).

## Informational findings

- Pre-IMPLEMENT readiness seam: none existed at baseline time. SEC-LEFT-003 adds
  one narrow read-only probe at the existing pre-execution seam; it runs no
  scanner and creates no second verdict.
- macOS Seatbelt runtime isolation: no evidence. The claim remains `UNVERIFIED`
  and is limited to the execution paths SEC-LEFT-005 inventories.

## Per-task trigger decisions

| Task | Trigger | Evidence | Disposition |
| --- | --- | --- | --- |
| SEC-LEFT-002 | Verify required-mode integration with the existing quality gate. | `internal/quality/security_test.go` already proves PASS proceeds and FAIL/INDETERMINATE fail closed; `securityGateEvidence` shapes the evidence. | `ALREADY_SATISFIED`; no code change. |
| SEC-LEFT-003 | Required prerequisites must fail closed before IMPLEMENT. | No precheck probe existed; inspect already exposes the readiness projection that the probe reuses. | `NEEDED`; implemented with focused tests. |
| SEC-LEFT-004 | Live AISEC evidence is absent and Promptfoo runs in the routine path. | `internal/evaltarget` has fixture and real-control coverage only; scan artifact shows 121 s Promptfoo cost per task. | `NEEDED`. |
| SEC-LEFT-005 | Per-path trust boundaries and macOS isolation are unverified. | Command-policy inventory absent; only Linux isolated replay is documented. | `NEEDED`. |

## Safety statement

No destructive operation was executed. No tool, rule, or advisory database was
provisioned or downloaded. No credential or secret value appears in this report.
Values not measured are marked `UNVERIFIED` rather than inferred.
