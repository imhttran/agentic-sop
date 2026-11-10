# SEC-LEFT-005 — macOS Seatbelt runtime verification

## Environment

- Host: `Darwin` (macOS, arm64). `bwrap` absent.
- `dqharness.DetectIsolation()` selects the `seatbelt` backend
  (`sandbox-exec`).
- Baseline `93c72857`, plus the boundary-fixture changes in this branch.
- Tests were run in disposable temporary directories. No user-owned secret or
  service was touched; credential checks use fake values via `t.Setenv`.

## dqharness isolated-replay path (runtime)

Command:

```sh
go test ./internal/dqharness/ -run 'Isolation|Isolated|Bubblewrap|Seatbelt' -v -count=1
```

Observed on this host (18 PASS, 3 SKIP):

| Test                                                  | Result                                          |
| ----------------------------------------------------- | ----------------------------------------------- |
| `TestIsolationBackendSelection`                       | PASS                                            |
| `TestBubblewrapArgumentsExcludeSeatbelt`              | PASS                                            |
| `TestBubblewrapArgumentsShape`                        | PASS                                            |
| `TestSeatbeltArgumentsShape`                          | PASS                                            |
| `TestSeatbeltCommandUsesCanonicalReplayDir`           | PASS                                            |
| `TestSeatbeltCommandCanonicalizationFailsClosed`      | PASS                                            |
| `TestIsolationCommandUnknownBackendFailsClosed`       | PASS                                            |
| `TestBubblewrapCommandCanonicalizationFailsClosed`    | PASS                                            |
| `TestIsolatedReplayCanonicalizationFailsBeforeLaunch` | PASS                                            |
| `TestIsolatedReplayReplayDirWritesSucceed`            | PASS                                            |
| `TestIsolatedReplayConfinementTempRoots`              | PASS                                            |
| `TestIsolatedReplayFailsClosedWithoutIsolation`       | PASS                                            |
| `TestIsolatedReplayRunsVerificationInFreshDir`        | PASS                                            |
| `TestIsolatedReplayScrubsCredentials`                 | PASS                                            |
| `TestIsolatedReplayDeniesWritesOutside`               | PASS                                            |
| `TestIsolatedReplayDeniesNetwork`                     | PASS                                            |
| `TestIsolatedReplayTimesOut`                          | PASS                                            |
| `TestIsolatedReplayUnixSocket`                        | PASS                                            |
| `TestIsolatedReplayDeniesPrivateMountWrite`           | SKIP — bubblewrap-only; macOS has no `/dev/shm` |
| `TestIsolatedReplayToolchainProcAndTemp`              | SKIP — Linux bubblewrap backend required        |
| `TestReplayIsolationRequiredEnv`                      | SKIP — `SOP_REPLAY_ISOLATION_REQUIRED` not set  |

Two fixtures were tightened so a pass means what it says:

- `TestIsolatedReplayDeniesNetwork` now starts a loopback TCP listener, proves
  the same client connects **without** the sandbox, and then requires the
  sandboxed client to fail. The old version dialed `1.1.1.1:80`, so a host with no
  outbound network could pass for the wrong reason.
- `TestIsolatedReplayUnixSocket` now runs on Seatbelt as well as bubblewrap. With
  a real host listener outside the replay directory, the unsandboxed client
  connects (`CONNECTED`) and the sandboxed client is denied
  (`DENIED:PermissionError`), with no connection reaching the listener. The test
  uses a short `/tmp` socket path because macOS rejects Unix socket paths above
  roughly 104 bytes.
- `TestIsolatedReplayDeniesPrivateMountWrite` is now bubblewrap-only; on macOS it
  previously "passed" because `/dev/shm` does not exist, proving nothing.

This is runtime confinement evidence for the **dqharness isolated-replay path**
on macOS: filesystem escape, private-mount write (Linux only), IP network denial,
Unix-domain socket denial, credential scrubbing, timeout, and fail-closed
behavior without isolation.

## evaltarget isolation path (runtime)

Command:

```sh
go test ./internal/evaltarget/ -run 'Isolat' -v -count=1
go test ./internal/evaltarget/ -run 'TestTimeoutTerminatesChildGroup|TestCancellationTerminatesChildGroup' -v -count=1
```

Observed on this host (9 isolation PASS; 2 process-group PASS):

| Test                                         | Result |
| -------------------------------------------- | ------ |
| `TestDarwinIsolationBlocksOrdinaryWorkspace` | PASS   |
| `TestDarwinIsolationBlocksNetwork`           | PASS   |
| `TestDarwinIsolationStripsSecrets`           | PASS   |
| `TestLiveEndpointMatchesIsolationRule`       | PASS   |
| `TestLiveIsolationDarwin`                    | PASS   |
| `TestLiveIsolationUnavailable`               | PASS   |
| `TestSandboxIsolatesCasesAndResets`          | PASS   |
| `TestIsolateEnvRemovesProductionSecrets`     | PASS   |
| `TestIsolationRequiredByDefault`             | PASS   |
| `TestTimeoutTerminatesChildGroup`            | PASS   |
| `TestCancellationTerminatesChildGroup`       | PASS   |

The two process-group tests were already exercised by the Linux workstream
(PR #303); they also pass here, so the macOS timeout/cancellation path kills the
child group. The evaltarget path has no Unix-domain socket fixture; that boundary
is covered by the dqharness test above.

## Limits

- This evidence covers the isolated replay and evaltarget evaluation paths only.
  It does not generalize to normal agent tool calls, configured agent commands,
  or verification commands; those have no SOP-enforced OS sandbox (see
  [execution-path-matrix.md](execution-path-matrix.md)).
- Linux Bubblewrap runtime isolation was not executed on this host; see
  [bubblewrap-verification.md](bubblewrap-verification.md).
- No platform-independent claim is made.

## run_command opt-in sandbox (LLM03)

Implements [PROPOSAL-RUN-COMMAND-ISOLATION](../../plans/PROPOSAL-RUN-COMMAND-ISOLATION.md)
for macOS. `SOP_AGENT_RUN_COMMAND_ISOLATION=1` (or `Config.IsolateCommands`)
makes `toolharness` wrap each `run_command` in `sandbox-exec` after the command
policy check. Off by default.

- **Profile** (`internal/toolharness/isolation.go`): allow-default; `deny network*`
  except loopback bind/inbound and loopback remote; `deny file-write*` except the
  canonical root, TMPDIR, `/private/tmp`, `/private/var/folders`, the user cache
  dir (default GOCACHE), GOCACHE/GOMODCACHE, and `/dev/null`; a final
  `deny file-write*` on `<root>/.agent-sdlc` (a later deny overrides the root allow).
- **Fail closed:** a non-darwin host, missing `sandbox-exec`, or an
  uncanonicalizable root returns `errIsolationUnavailable`, audited as a denial;
  the command never runs unsandboxed.
- **Audit:** a successful isolated run records
  `isolate=seatbelt backend=<path> root=<root> profile=<sha256>` in Detail.

Verification (macOS arm64, 2026-11-09):

- `go test ./internal/toolharness -run Isolation`: PASS, including the state-dir write
  denial, the write outside the allowed dirs, the blocked outbound TCP connection to
  1.1.1.1:443, and the allowed loopback connection.
- The repository's own `go test` for `internal/toolharness`, `repoindex`, `security`
  and `testrunner` ran inside the generated profile: PASS.
- `touch .agent-sdlc/forged` under the profile: `Operation not permitted`.
- Found while testing: `(allow network* (local ip "localhost:*"))` also matches
  outbound sockets, so the profile keys only bind/inbound on the local address.

Limits: Linux has no `run_command` backend yet (NOT_RUN, fails closed). Seatbelt
cannot nest, so the tests skip inside another sandbox. The outbound-deny test is
only meaningful on a host with network access.
