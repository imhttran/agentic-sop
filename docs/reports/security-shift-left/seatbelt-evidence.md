# SEC-LEFT-005 — macOS Seatbelt runtime verification

## Environment

- Host: `Darwin` (macOS, arm64). `bwrap` absent.
- `dqharness.DetectIsolation()` selects the `seatbelt` backend
  (`sandbox-exec`).
- Tests were run in disposable temporary directories. No user-owned secret or
  service was touched; credential checks use fake values via `t.Setenv`.

## dqharness isolated-replay path (runtime)

Command:

```sh
go test ./internal/dqharness/ -run 'Isolation|Isolated|Bubblewrap|Seatbelt' -v -count=1
```

Observed on this host:

| Test | Result |
| --- | --- |
| `TestIsolatedReplayCanonicalizationFailsBeforeLaunch` | PASS |
| `TestIsolatedReplayReplayDirWritesSucceed` | PASS |
| `TestIsolatedReplayConfinementTempRoots` | PASS |
| `TestIsolatedReplayDeniesPrivateMountWrite` | PASS |
| `TestIsolatedReplayFailsClosedWithoutIsolation` | PASS |
| `TestIsolatedReplayRunsVerificationInFreshDir` | PASS |
| `TestIsolatedReplayScrubsCredentials` | PASS |
| `TestIsolatedReplayDeniesWritesOutside` | PASS |
| `TestIsolatedReplayDeniesNetwork` | PASS |
| `TestIsolatedReplayTimesOut` | PASS |
| `TestIsolatedReplayUnixSocket` | SKIP — Linux bubblewrap backend required |
| `TestIsolatedReplayToolchainProcAndTemp` | SKIP — Linux bubblewrap backend required |
| `TestReplayIsolationRequiredEnv` | SKIP — `SOP_REPLAY_ISOLATION_REQUIRED` not set |

This is runtime confinement evidence for the **dqharness isolated-replay path**
on macOS: filesystem escape, private-mount write, network denial, credential
scrubbing, timeout, and fail-closed behavior without isolation all passed under
the Seatbelt backend.

## evaltarget isolation path (runtime)

Command:

```sh
go test ./internal/evaltarget/ -run 'Isolat' -v -count=1
```

Observed on this host:

| Test | Result |
| --- | --- |
| `TestDarwinIsolationBlocksOrdinaryWorkspace` | PASS |
| `TestDarwinIsolationBlocksNetwork` | PASS |
| `TestDarwinIsolationStripsSecrets` | PASS |
| `TestSandboxIsolatesCasesAndResets` | PASS |
| `TestIsolateEnvRemovesProductionSecrets` | PASS |
| `TestIsolationRequiredByDefault` | PASS |

Host Unix-socket denial has no evaltarget fixture on any backend; it remains
unverified for this path.

## Limits

- This evidence covers the isolated replay and evaltarget evaluation paths only.
  It does not generalize to normal agent tool calls, configured agent commands,
  or verification commands; those have no SOP-enforced OS sandbox (see
  [execution-path-matrix.md](execution-path-matrix.md)).
- Linux Bubblewrap runtime isolation was not executed on this host; see
  [bubblewrap-verification.md](bubblewrap-verification.md).
- No platform-independent claim is made.
