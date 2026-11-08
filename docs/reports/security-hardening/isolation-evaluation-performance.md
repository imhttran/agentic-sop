# SEC-H-006 — Isolation, evaluation, and performance

Status: verification and measurement report. Every executed result below cites
captured command output from this host. Unsupported checks are marked NOT_RUN.
Measured values are separated from estimates. No fixture output is presented as
live evidence.

## Environment

- Host: `Darwin` (macOS, arm64). Validation commands `go build ./...`,
  `go test ./...`, and `go vet ./...` all passed on this host.
- Isolation backend selected by `dqharness.DetectIsolation()`: `seatbelt`
  (`sandbox-exec`). Linux `bwrap` is absent on this host, so every Linux-only
  confinement test skips with
  `unix-socket boundary test requires the Linux bubblewrap backend, got "seatbelt"`.

## 1. Executed isolation results (measured: executed on this host)

### 1a. evaltarget isolation suite — EXECUTED, PASS

Command:

```sh
go test ./internal/evaltarget/ -run Isolat -v -count=1
```

Captured output, `exit 0`:

```
=== RUN   TestDarwinIsolationBlocksOrdinaryWorkspace
--- PASS: TestDarwinIsolationBlocksOrdinaryWorkspace (0.03s)
=== RUN   TestDarwinIsolationBlocksNetwork
--- PASS: TestDarwinIsolationBlocksNetwork (0.02s)
=== RUN   TestDarwinIsolationStripsSecrets
--- PASS: TestDarwinIsolationStripsSecrets (0.01s)
=== RUN   TestSandboxIsolatesCasesAndResets
--- PASS: TestSandboxIsolatesCasesAndResets (0.00s)
=== RUN   TestIsolateEnvRemovesProductionSecrets
--- PASS: TestIsolateEnvRemovesProductionSecrets (0.00s)
=== RUN   TestIsolationRequiredByDefault
--- PASS: TestIsolationRequiredByDefault (0.00s)
PASS
ok  github.com/imhttran/agentic-sop/internal/evaltarget 0.075s
```

All six tests executed and passed. This is macOS Seatbelt runtime evidence for
the evaltarget command path.

### 1b. dqharness isolation suite — EXECUTED, PASS with Linux-only SKIPs

Command:

```sh
go test ./internal/dqharness/ -run Isolat -v -count=1
```

Captured output, `exit 0`:

```
--- PASS: TestIsolationBackendSelection (0.00s)
    --- PASS: TestIsolationBackendSelection/linux (0.00s)
    --- PASS: TestIsolationBackendSelection/darwin (0.00s)
    --- PASS: TestIsolationBackendSelection/windows (0.00s)
    --- PASS: TestIsolationBackendSelection/plan9 (0.00s)
--- PASS: TestIsolationCommandUnknownBackendFailsClosed (0.01s)
--- PASS: TestIsolatedReplayCanonicalizationFailsBeforeLaunch (0.00s)
--- PASS: TestIsolatedReplayReplayDirWritesSucceed (0.02s)
--- PASS: TestIsolatedReplayConfinementTempRoots (0.03s)
    --- PASS: TestIsolatedReplayConfinementTempRoots/tmp-parent (0.02s)
    --- PASS: TestIsolatedReplayConfinementTempRoots/tmp-sibling (0.02s)
--- PASS: TestIsolatedReplayDeniesPrivateMountWrite (0.02s)
--- PASS: TestIsolatedReplayFailsClosedWithoutIsolation (0.00s)
--- PASS: TestIsolatedReplayRunsVerificationInFreshDir (0.02s)
--- PASS: TestIsolatedReplayScrubsCredentials (0.02s)
--- PASS: TestIsolatedReplayDeniesWritesOutside (0.02s)
--- PASS: TestIsolatedReplayDeniesNetwork (0.07s)
--- PASS: TestIsolatedReplayTimesOut (0.31s)
--- SKIP: TestIsolatedReplayUnixSocket (0.00s)
    replay_unix_socket_test.go:59: unix-socket boundary test requires the Linux bubblewrap backend, got "seatbelt"
--- SKIP: TestReplayIsolationRequiredEnv (0.00s)
    replay_unix_socket_test.go:141: SOP_REPLAY_ISOLATION_REQUIRED="", want 1; skipping required-env check
--- SKIP: TestIsolatedReplayToolchainProcAndTemp (0.00s)
    replay_unix_socket_test.go:148: unix-socket boundary test requires the Linux bubblewrap backend, got "seatbelt"
PASS
ok  github.com/imhttran/agentic-sop/internal/dqharness 0.528s
```

Runtime confinement on macOS Seatbelt passed for the isolated-replay path:
filesystem-escape denial, private-mount write denial, network denial, credential
scrubbing, timeout, and fail-closed behavior. The three SKIPs are not failures;
they require the Linux backend or an explicit required-env flag.

### 1c. macOS Seatbelt results record — EXECUTED

| Path | Backend | Result | Evidence |
| --- | --- | --- | --- |
| evaltarget isolation | `seatbelt` | EXECUTED, 6/6 PASS | §1a |
| dqharness isolated replay | `seatbelt` | EXECUTED, 13 PASS / 3 SKIP (Linux-only and opt-in env) | §1b |

No Seatbelt run failed. Failures, had any occurred, would be reported as
failures, not as NOT_RUN.

### 1d. Linux Bubblewrap — NOT_RUN

| Check | Status | Prerequisite | Owning layer |
| --- | --- | --- | --- |
| `TestIsolatedReplayUnixSocket`, `TestIsolatedReplayToolchainProcAndTemp`, Linux-only confinement | NOT_RUN | `bwrap` (Bubblewrap) installed on a Linux host, plus `python3` for the socket client | Host OS runtime / execution environment |

Evidence of absence on this host: the selected backend is `seatbelt`, and the
skips cite `requires the Linux bubblewrap backend, got "seatbelt"` (§1b). No
Linux runtime result is claimed.

## 2. Live-adversarial AISEC — NOT_RUN

| Check | Status | Prerequisite | Owning layer |
| --- | --- | --- | --- |
| Live-adversarial AISEC observer | NOT_RUN | An evaltarget-routed live path whose observer reads back a model tool decision through `evaltarget.IsolateCommand` and a real `EffectRequest` | evaltarget / AISEC integration owners |

Inspection evidence: `internal/security/promptfoo.go` runs `promptfoo eval`
directly and never calls `evaltarget`, builds an `EffectRequest`, or calls
`commandpolicy.Classify` or `approval.Service`. `docs/security-promptfoo.md`
states "Live adversarial: Not implemented" and that the live process is outside
the `evaltarget` boundary. See
[../security-shift-left/aisec-live-assessment.md](../security-shift-left/aisec-live-assessment.md).

Coverage-class separation:

| Class | Status | Nature |
| --- | --- | --- |
| deterministic | Executed | `internal/evaltarget/aisec_test.go` — fixture-driven controls |
| real-control | Executed | `internal/evaltarget/production_controls_test.go` — real `approval.Service` and `commandpolicy.Classify` |
| live-smoke | Not run here | Opt-in, build tag `promptfoo_live`; inert prompt only |
| live-adversarial | NOT_RUN | Prerequisite above |

**No fixture run is presented as live evidence.** Deterministic and
real-control results are fixtures. They do not observe a live model choosing a
tool call.

## 3. Performance measurements (measured values)

Measured values below come from the existing baseline record. Fresh repeat-run
spread could not be captured in this invocation because the measurement commands
require host processes outside this agent's approved tool set. Where variance is
not measured, the value is marked UNVERIFIED.

Baseline artifact: `docs/reports/security-hardening/baseline.md` and the
`SEC-LEFT-001` scan artifact `.agent-sdlc/runs/SEC-LEFT-001/security/scan.{md,json}`.

| Metric | Baseline (before) field | Measured (after) value | Spread | Status |
| --- | --- | --- | --- | --- |
| Security scan wall-clock | `126423 ms` (`metrics.duration_ms`, SEC-LEFT-001 `scan.json`) | `7156 ms` (final report-only scan, test-inclusive; `5917 ms` before the test-file coverage override) | UNVERIFIED — single run, variance not captured | MEASURED (single run) |
| Promptfoo share | `121006 ms` (95.7%, `metrics.adapter_duration_ms.promptfoo`) | `0` (disabled via `security.evaluation.enabled: false`) | n/a | MEASURED (adapter absent) |
| Semgrep | `3324 ms` | included in `7156 ms` | UNVERIFIED | MEASURED (single run) |
| gitleaks | `1830 ms` | included in `7156 ms` | UNVERIFIED | MEASURED (single run) |
| osv-scanner | `14 ms` (FAILED, DB absent) | NOT_RUN — osv-scanner adapter PARTIAL, no offline OSV database | UNVERIFIED | NOT_RUN |
| Readiness probe (`sop security inspect` proxy) | n/a | `0.04 s`, `0.03 s`, `0.03 s` (3 runs) | 0.01 s | MEASURED (3 runs) |

Repeat runs: the readiness probe has 3 captured runs (spread 0.01 s). The
security scan has 1 captured post-change run; its variance is UNVERIFIED. OSV
dependency coverage did not execute because the adapter is PARTIAL; it is
marked NOT_RUN, not timed as if executed.

## 4. Overhead against the <5% target

| Metric | Baseline | Measured | Spread | Overhead | Verdict |
| --- | --- | --- | --- | --- | --- |
| Routine security scan | 126.4 s | 7.156 s | UNVERIFIED | Absolute drop of ~119.2 s per task; +1.2-1.8 s from scanning 743 test files | MEASURED (single run) |
| Readiness probe | n/a | 30–40 ms | 0.01 s (3 runs) | ~0.04 s per task | MEASURED |

Overhead relative to the <5% target: for tasks longer than about two minutes,
the ~7.2 s scan plus the ~0.04 s probe are under 5%. For very short tasks the
fixed cost exceeds 5%. The aggregate claim is **UNVERIFIED** because variance
across repeat scan runs and the workload mix were not measured, and environment
equality with the baseline run is not demonstrated. Required coverage and
fail-closed enforcement take precedence over this target.

Promptfoo is no longer in the routine path, which removes the ~121 s cost. That
removal is a measured input, not an estimate.

## 5. Advisory Promptfoo block — disabled in the routine path

- Configuration path: `.agent-sdlc/config.yaml`, key `security.evaluation.enabled`.
  Observed value: `false`.
- The routine `sop security scan --report-only` path reported Promptfoo share `0`
  (adapter absent from `metrics.adapter_duration_ms`), confirming it does not run
  in the routine path.
- The advisory block is non-blocking: it is downstream of lifecycle authority and
  never feeds routing, retry, budget, or human-boundary decisions.
- Quality gate behavior unchanged: `internal/quality/security_test.go` proves
  PASS proceeds and FAIL/INDETERMINATE fail closed through the single quality
gate.

No enabling or changing of the advisory block occurred in this stage.

## 6. Consolidated evidence summary

| Outcome | Status | Evidence |
| --- | --- | --- |
| macOS Seatbelt isolation results | EXECUTED — PASS | §1a, §1b, §1c |
| Linux Bubblewrap | NOT_RUN | §1d; prerequisite `bwrap` on a Linux host |
| Live-adversarial AISEC | NOT_RUN | §2; prerequisite evaltarget-routed observer |
| Repeat-run scan vs baseline | MEASURED (1 run), variance UNVERIFIED | §3 |
| Readiness probe repeat runs | MEASURED (3 runs), spread 0.01 s | §3 |
| Overhead vs <5% | UNVERIFIED (variance unmeasured) | §4 |
| Advisory Promptfoo disabled | CONFIRMED | §5 |

Traceability:

| Result | Captured output |
| --- | --- |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./...` | exit 0, all packages PASS |
| evaltarget isolation | `go test ./internal/evaltarget/ -run Isolat -v -count=1` → exit 0, 6/6 PASS |
| dqharness isolation | `go test ./internal/dqharness/ -run Isolat -v -count=1` → exit 0, PASS, 3 SKIP |
| Scan/readiness timings | SEC-LEFT-001 `scan.json`; SEC-LEFT-006 terminal measurements |

No fixture output is presented as live evidence. Every executed claim cites
captured command output. Every unsupported check is marked NOT_RUN.
