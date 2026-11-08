# SEC-LEFT-006 — Final reconciliation, verification and handoff

Status: reconciliation and verification report. It records what was implemented,
what was removed as speculative, what was measured, and what was not run. No PR
is claimed.

## Scope

This report reconciles SEC-LEFT-001..005 against the plan's acceptance criteria,
records focused verification actually executed in this worktree, compares
before/after security cost with measured values, and states the remaining gaps.

## Reconciliation map

| Path | Task | Disposition |
| --- | --- | --- |
| `internal/cli/security_readiness.go` (new) | SEC-LEFT-003 | Read-only pre-IMPLEMENT readiness probe. Performs one repository detection per call and passes the result to selection/classification. No cache. |
| `internal/cli/security_readiness_test.go` (new) | SEC-LEFT-003 | Focused regressions: disabled is a no-op; ready required gate proceeds; missing required prerequisite fails closed through `failure.Classify`; report-only never blocks; exactly one detection per call; each call re-evaluates. |
| `internal/cli/run.go` (modified, +27) | SEC-LEFT-003 | Wires the probe at the existing pre-execution seam before IMPLEMENT. No scanner, no second verdict. |
| `.agent-sdlc/config.yaml` (modified) | SEC-LEFT-004 | `security.evaluation.enabled` set from `true` to `false` so the advisory Promptfoo evaluation leaves the routine scan path. Documented inline; reversible. |
| `docs/reports/security-shift-left/baseline.md` | SEC-LEFT-001 | Rewritten to match the real scan artifact (three unsuppressed HIGH findings, partial sast coverage, failed dependency coverage, Promptfoo runtime). |
| `docs/reports/security-shift-left/aisec-live-assessment.md` | SEC-LEFT-004 | Adapter capability assessment; live-adversarial NOT_RUN with prerequisites; no second evaluator. |
| `docs/reports/security-shift-left/execution-path-matrix.md` | SEC-LEFT-005 | Per-path trust boundary matrix with explicit unverified rows. |
| `docs/reports/security-shift-left/seatbelt-evidence.md` | SEC-LEFT-005 | macOS Seatbelt runtime results actually observed on this host. |
| `docs/reports/security-shift-left/bubblewrap-verification.md` | SEC-LEFT-005 | Linux Bubblewrap status: NOT_RUN on this host, with the existing fixtures and prerequisite. |
| `docs/plans/sop-security-plan.md` | All | Plan source executed by SOP. |

Removed as speculative during reconciliation (never referenced by any package):
`internal/security/aisec_digest.go`, `aisec_evidence.go`,
`aisec_evidence_test.go`, `aisec_runner.go`, `aisec_runner_test.go`, and
`evals/promptfoo/aisec/`. They mirrored `internal/evaltarget` scenario
definitions and added a build-tagged runner that always reported NOT_RUN, which
is precisely the duplicate evaluator the plan forbids. `internal/evaltarget`
remains the single AISEC scenario source.

No-op task: SEC-LEFT-002 is ALREADY_SATISFIED. `internal/quality/security_test.go`
already proves PASS proceeds and FAIL/INDETERMINATE fail closed through the single
quality gate. No security code was duplicated.

## Gate history for SEC-LEFT-003

SOP raised NEEDS_HUMAN after the fix-loop limit (3/3) on a defective attempt: the
model's readiness code performed two repository detections per call and kept an
unbounded process-global cache that could hide later prerequisite failures. The
operator-declined request is preserved in the task's approval history; the
corrected implementation was recorded through the supported external-completion
path, which re-ran `go build ./...`, `go test ./...`, and `go vet ./...` and
required them to pass. Evidence: `.agent-sdlc/external-completions/SEC-LEFT-003.md`.
No approval was synthesized and no declined decision was rewritten.

## Focused verification (actually executed)

| Check | Command | Result |
| --- | --- | --- |
| Build | `go build ./...` | exit 0 |
| Vet | `go vet ./...` | exit 0 (SOP validation, SEC-LEFT-003 external completion) |
| Full suite | `go test ./...` | PASS (SOP validation, same completion) |
| Readiness regressions | `go test ./internal/cli/ -run TestSecurityReadiness -count=1 -v` | PASS, 6/6 |
| Formatting | `gofmt -l internal/cli/security_readiness.go internal/cli/security_readiness_test.go` | no output |
| dqharness isolation runtime | `go test ./internal/dqharness/ -run 'Isolation|Isolated|Bubblewrap|Seatbelt' -v -count=1` | PASS on macOS Seatbelt; 3 Linux-only/env tests SKIP |
| evaltarget isolation runtime | `go test ./internal/evaltarget/ -run 'Isolat' -v -count=1` | PASS (6 tests) |
| Security scan after config change | `sop security scan --report-only` | verdict FAIL (3 unsuppressed HIGH), coverage PARTIAL, 5775 ms |
| Readiness-inspect proxy | `sop security inspect --json` ×3 | 0.04 s, 0.03 s, 0.03 s |

The two focused readiness tests fail against the rejected implementation: the
detection-count test observes two detections, and the re-evaluation test observes
a stale not-ready result after the adapter binary appears.

## Before/after controls

| Control | Before (SEC-LEFT-001 baseline) | After | Status |
| --- | --- | --- | --- |
| Quality-stage security gate | Present, report-only, single verdict | Unchanged | ALREADY_SATISFIED |
| Pre-IMPLEMENT readiness probe | None | Read-only probe at the existing pre-execution seam; fails closed only for an active required gate; no scanner, no second verdict | ENABLED |
| Required-mode integration | Present and fail-closed | Unchanged | ALREADY_SATISFIED |
| AISEC evidence | Deterministic + real-control in `evaltarget`; no live adversarial | Unchanged; live-adversarial NOT_RUN with prerequisites; no second evaluator | NOT_RUN (live) |
| Promptfoo in routine scan path | ~121 s on every quality scan | Disabled in the project config; enable temporarily for a dedicated session | CHANGED (advisory only) |
| Execution-path boundary matrix | Absent | Five paths inventoried with explicit unverified rows | DOCUMENTED |
| Linux Bubblewrap replay isolation | Claimed for replay | NOT_RUN on this host (no `bwrap`); fixtures and prerequisite recorded | NOT_RUN |
| macOS Seatbelt runtime isolation | Unevidenced | Runtime confinement verified for the dqharness replay path and evaltarget tests on this host | VERIFIED (those paths only) |

Enabled enforcement versus proposed changes: the only newly enabled enforcement
is the required-mode pre-IMPLEMENT readiness fail-closed path. The Promptfoo
config change removes an advisory adapter from routine scans; it does not change
required categories, severity handling, coverage, or the quality verdict.

## Findings resolved and remaining gaps

Resolved or bounded:

- Pre-IMPLEMENT readiness is deterministic, single-inspection, fail-closed for
  required mode, and a strict no-op when the add-on is disabled.
- Execution-path trust boundaries are stated per path; no overbroad isolation
  claim remains.
- Linux Bubblewrap was not re-verified on this host and is no longer claimed.
- macOS Seatbelt runtime evidence exists for the two isolated paths.
- The largest avoidable routine cost (advisory Promptfoo, 121 s/task) is removed.

Remaining gaps (not fixed by this plan):

| Gap | Status |
| --- | --- |
| 3 unsuppressed HIGH `dangerous-exec-command` findings (`internal/cli/outcome.go:720`, `internal/decisionadapter/providers/julia/runner.go:231`, `internal/dqharness/replay_exec.go:273`) | OPEN — pre-existing, outside this plan's scope |
| OSV offline database absent (`.agent-sdlc/osv-db`); dependency coverage FAILED | OPEN — operator provisioning decision |
| Semgrep coverage PARTIAL (743 files skipped by `.semgrepignore`; Go only) | OPEN — rule/scope decision |
| Live-model adversarial evidence | NOT_RUN — needs an evaltarget-routed live path that reads back a model tool decision |
| Linux Bubblewrap runtime verification | NOT_RUN on this host |
| evaltarget host Unix-socket denial fixture | UNVERIFIED |

## Live-model results

No live-adversarial result was produced. The pinned Promptfoo adapter runs one
inert prompt and cannot route a model tool decision through `evaltarget`
controls, so it cannot observe an allowed or denied operation. Live validation is
NOT_RUN, never PASS. Details: `aisec-live-assessment.md`.

## Isolation boundary matrix

| Path | Classification | Runtime evidence |
| --- | --- | --- |
| Normal agent tool calls | operator-trusted configuration | UNVERIFIED — SOP does not sandbox the agent process |
| Configured agent commands | command policy only | Policy exists; no OS isolation |
| Verification commands (agent-side) | operator-trusted configuration | UNVERIFIED |
| Isolated replay (dqharness) | OS isolation | macOS Seatbelt: PASS; Linux Bubblewrap: NOT_RUN here |
| Evaluation-target commands (evaltarget) | OS isolation | macOS: PASS; Linux: construction only, NOT_RUN here; Unix socket: UNVERIFIED |

Isolation claims are limited to the isolated replay and evaltarget paths on the
platforms actually exercised. They do not generalize to agent execution.

## Performance comparison

Measured on this host (macOS arm64), single runs, warm caches. Variance not
measured.

| Metric | Before | After | Source |
| --- | --- | --- | --- |
| Security scan wall-clock | 126423 ms | 5775 ms | SEC-LEFT-001 `scan.json`; post-change `sop security scan --report-only` |
| Promptfoo share | 121006 ms (95.7%) | 0 (disabled in routine path) | `metrics.adapter_duration_ms.promptfoo` |
| Semgrep | 3324 ms | included in 5775 ms | adapter metrics |
| gitleaks | 1830 ms | included in 5775 ms | adapter metrics |
| osv-scanner | 14 ms (FAILED) | included in 5775 ms | adapter metrics |
| Scans per task | 1 | 1 | run artifacts |
| Evidence reuse/resume | none | none | run artifacts |
| Pre-IMPLEMENT readiness probe | n/a | 30–40 ms per task (`sop security inspect` proxy, 3 runs) | terminal measurements |

Absolute routine security cost fell by ~120.6 s per task. Relative overhead
depends on task length: measured samples range from ~1.6% (SEC-LEFT-004:
agent 4 m 30 s + validation 1 m 25 s = 355 s) to ~14% (SEC-LEFT-005:
agent 37.8 s + validation 3.6 s = 41.4 s). The 5.8 s scan plus the ~0.04 s probe
is under 5% of any task longer than about two minutes. The proposed `<5%` target
is therefore **met for ordinary multi-minute tasks and not met for very short
tasks**; the aggregate claim is `UNVERIFIED` because variance and workload mix
were not measured. Required coverage and fail-closed enforcement take precedence
over the target.

## Commit-safety review

- No credential, token, or secret value is added. The evaluation block keeps
  model identifiers only.
- `.agent-sdlc/runs/`, `.agent-sdlc/osv-db/`, `.agent-sdlc/security-rules/`, and
  `.agent-sdlc/external-completions/` are git-ignored and not part of the commit
  set. License-restricted Semgrep rules are not copied into the repository.
- The tracked config change is limited to `security.evaluation.enabled`.
- No generated scanner report is committed.

## Evidence pointers

- Scan artifact: `.agent-sdlc/runs/SEC-LEFT-001/security/scan.{md,json}`.
- Readiness probe implementation: `internal/cli/security_readiness.go`.
- Readiness regressions: `internal/cli/security_readiness_test.go`.
- External completion: `.agent-sdlc/external-completions/SEC-LEFT-003.md`.
- Isolation evidence: `seatbelt-evidence.md`, `bubblewrap-verification.md`,
  `execution-path-matrix.md`.
- AISEC assessment: `aisec-live-assessment.md`.

## Risks and decisions

- The SEC-LEFT-003 gate was declined on the defective attempt and the corrected
  work was recorded as external completion with passing validation. This is the
  supported transition; the declined decision remains in the audit history.
- AISEC speculative code was removed after SOP completion to keep the plan's
  no-second-evaluator constraint.
- Disabling Promptfoo in the project config changes local scan composition, not
  required enforcement. Re-enable for a dedicated evaluation session.
- 006's original report claimed Linux Bubblewrap runtime PASS and macOS
  UNVERIFIED; both were corrected here from executed tests.

## Handoff

- Branch: `security/shift-left`, based on `origin/main` @ `14678012`.
- Working tree: modified/added files listed above; nothing committed.
- PR: not opened. The repository sets `human.approval_before_commit: true`;
  `sop commit --yes` and `sop pr --yes` need the operator's decision.
