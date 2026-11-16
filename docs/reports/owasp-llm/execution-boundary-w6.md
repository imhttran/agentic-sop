# Execution boundary — M1 + G3 + G4 (W6)

**Status:** `IMPLEMENTED` / `VERIFIED` on `darwin arm64` (real Seatbelt backend) and
on `ubuntu-22.04` with real bubblewrap. Merged at `1e611304` via PR #499.
**Authorization:** operator decision 2026-11-16 (option B, with mandatory
sandbox-enabled integration verification), amending
[PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md](../../plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md) §11
as recorded in [DECISION-PACKET-OWASP-LLM-W6-W8.md](../../plans/DECISION-PACKET-OWASP-LLM-W6-W8.md).

## 1. What changed

| Element | Before | After |
| --- | --- | --- |
| M1 — `run_command` | sandboxed by default, operator override (`SOP_AGENT_RUN_COMMAND_ISOLATION` + `..._UNSANDBOXED_ACK`) could disable it | **always** sandboxed; override and acknowledgement deleted |
| G3 — git | `git_status`, `git_diff`, `WorkingTreeFingerprint`, `ChangedSince`, `WorkingTreeChanged` ran unsandboxed | all run through the same `isolate()` path as `run_command` |
| G4 — `.git` | model could write `.git/config` via the file tools | every write class refuses any path whose segments include `.git`; reads stay allowed |
| W1 hardening | flags + environment scrub on git | **preserved**, now applied inside the sandbox |
| Credential scrubbing | sandbox environment scrubbed | unchanged; the git child environment is additionally stripped of git injection variables |

One helper, `execIsolated`, is the only way a model-reachable command runs. It
calls `isolate()`, and a host that cannot build a sandbox returns
`errIsolationUnavailable` — there is no fallback branch to reach.

Removed: `IsolationEnv`, `UnsandboxedAckEnv`, `isOffValue`, `isAcknowledged`,
`isolationOverrideOff`, `IsolationPolicyNotice`, `isolationDecision`, the two CLI
notice call sites, and the tests that pinned the override. `Config.IsolateCommands`
is retained as a documented no-op so construction sites keep compiling.

The retired variable names are still dropped from model-controlled child
environments: they are inert, but a nested SOP process should not inherit a stale
value.

## 2. Fail-closed behaviour

| Host | `run_command` | `git_status` / `git_diff` | fingerprint / change detection |
| --- | --- | --- | --- |
| backend can start (macOS, Linux with working bwrap) | sandboxed | sandboxed | sandboxed |
| backend missing or cannot start (Windows, CI runner, Ubuntu 24.04 AppArmor) | refused, audited `deny` | refused, audited `deny` | `errIsolationUnavailable`; `ChangedSince`/`WorkingTreeChanged` report **changed** |

The fingerprint case reports "changed" rather than erroring, so a task can never be
told it changed nothing on evidence the host cannot produce. `observedRepositoryChange`
in the agent already treats an error as "unknown", so no task fails outright.

## 3. Tests

New (`internal/toolharness/isolation_mandatory_test.go`):

| Test | Asserts |
| --- | --- |
| `TestGitToolsAreSandboxed` | `git_status` and `git_diff` audits carry `isolate=` |
| `TestSafeBuildAndTestCannotBypassIsolation` | `go build`, `go vet`, `go test` audits carry `isolate=` |
| `TestGitToolsRefusedWithoutBackend` | both git tools return `errIsolationUnavailable` and are audited `deny`; nothing runs |
| `TestFingerprintFailsClosedWithoutBackend` | fingerprint refuses; `ChangedSince`/`WorkingTreeChanged` report changed |
| `TestGitMetadataIsReadOnlyToModelTools` | write/create/rewrite/copy under `.git` refused; `.git/config` readable; `.github/...` allowed |
| `TestSandboxedGitFilterCannotReachHost` | a repository clean filter runs inside the sandbox but cannot write SOP state or read a home credential; the unsandboxed leg is the positive control |
| `TestUnsupportedPlatformHasNoBackend` | windows/plan9/aix have no backend |

Replaced/rewritten: `TestIsolationIsUnconditional` and
`TestRetiredOverrideVariablesHaveNoEffect` replace the `isolationDecision` tables.
Removed because the behaviour no longer exists: `TestIsolationPolicyNotice`,
`TestRunCommandIsolationOffKeepsExecCapturedPath`,
`TestRunCommandIsolationOffWithoutAckStaysSandboxed`,
`TestSubprocessStateWriteIsOutsideFileToolBoundary`, and the policy-only leg of
`TestRunCommandChildEnvironmentDropsIsolationControls` /
`TestRunCommandScrubsCredentialEnvironment` /
`TestDarwinIsolationSAFECommandCredentialRead`.

## 4. Skipped tests (explicit coverage statement)

A green ordinary CI job is **not** evidence of W6. On a host with no runnable
backend, measured with a temporary backend-suppression hook:

- **66** sandbox-dependent tests skip (30 in `internal/toolharness`, 36 in
  `internal/ollamaagent`; one name exists in both).
- **22** other skips remain, for unrelated reasons (platform-specific probes, live
  tags). They are reported by the integration job, not treated as failures.
- `internal/cli` needs no gating: its suite passes with no backend.

Skips are honest, not silent: `needSandbox` and `needSandboxAvailable` skip with
`no runnable sandbox backend`, and both **fail** when `SOP_SANDBOX_REQUIRED=1`.

`SandboxRunnable` deliberately checks more than "the backend binary exists": it
probes that the backend starts **and** runs one trivial command through the same
`isolate()` production path the harness uses. A host where the binary exists but
the production mount set fails therefore skips rather than failing a test that
expects execution.

## 5. Sandbox-enabled Linux verification

`scripts/checks/sandbox-integration.sh` plus the `sandbox-linux` CI job:

- Runs on `ubuntu-22.04`, which permits unprivileged user namespaces, so
  bubblewrap works **without changing the host's AppArmor policy**.
- Installs `bubblewrap`, then probes `bwrap --ro-bind / / --unshare-net true` and
  fails loudly if it cannot start.
- Runs `internal/toolharness` and `internal/ollamaagent` with
  `SOP_SANDBOX_REQUIRED=1`, so a sandbox skip becomes a failure.
- Prints `pass/fail/skip/sandbox_skip` counts, names any failing tests, and
  refuses to report OK when a sandbox skip occurred.

Coverage: `run_command` (including `go test`/`go build`/`go vet`),
`git_status`, `git_diff`, and the per-turn fingerprints all execute under real
bubblewrap containment.

**First run (2026-11-16, PR #499, run `43954680761`):** `pass=446 fail=0 skip=3
sandbox_skip=0` in 6m46s. Linux containment is `VERIFIED`. The three non-sandbox
skips are reported by the job and are unrelated to the boundary (the macOS keychain
probe and live-tagged tests).

## 6. Verification performed

```
go build ./...                                                  # ok
go vet ./...                                                    # ok
go test ./internal/toolharness/ -count=1                        # ok  45.6s
go test ./internal/ollamaagent/ -count=1                        # ok 126.3s
go test ./internal/cli/ -count=1                                # ok  65.0s
go test ./internal/archtest/ -count=1                           # ok
```

CI on PR #499: 14 of 14 checks pass, including
`sandbox-backed execution boundary (Linux)`: `pass=446 fail=0 skip=3
sandbox_skip=0` (run
[43954680761](https://github.com/imhttran/agentic-sop/actions/runs/43954680761)).

All on darwin arm64 with the real Seatbelt backend. The macOS sandbox tests that
previously ran policy-only now run contained and pass.

Scanner (pinned adapters): coverage `COMPLETE`, verdict `FAIL`,
**98 findings, 81 suppressed, 17 unsuppressed, 1 blocking**, 0 execution failures.
The single blocking finding is the acknowledged `dangerous-exec-command` call site
in `harness.go`; containment mitigates behaviour, not the call site, and it is not
suppressed. Three baseline entries became **stale** because the tests they
referenced were removed or rewritten by this change (the retired override tests);
no entry was added, edited, or removed. `security.mode` stays `report-only` and
`required_categories` stays empty (W8 deferred).

## 7. Residuals and compatibility

1. **Containment is not output trust.** A sandboxed filter still runs and can still
   transform git's reported output; that question is answered separately by the W7
   policy in [git-output-integrity.md](git-output-integrity.md).
2. **Windows.** `run_command`, `git_status`, `git_diff` and fingerprints are
   unavailable: change detection degrades to "changed". This is the trade the
   operator accepted with option B.
3. **Linux hosts without user namespaces** (Ubuntu 24.04 AppArmor default) behave
   like Windows: refusal.
4. **CI coverage.** On the ordinary runner the 66 sandbox tests skip; the
   `sandbox-linux` job is where they run. macOS arm64 is verified; Linux is
   `NOT_RUN` until that job runs.
5. G4 covers the file tools only. Sandboxed model code can still write `.git/config`
   (measured in the proposal), but it is inert for host execution because git itself
   is sandboxed.
