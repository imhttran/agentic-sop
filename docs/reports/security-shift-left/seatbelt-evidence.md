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
  except loopback bind/inbound and loopback remote (host Unix sockets are denied); `deny file-write*` except the
  canonical root, TMPDIR, `/private/tmp`, `/private/var/folders`, and `/dev/null`
  (each run's `GOCACHE` is a fresh temp dir; the module cache is read-only); a final
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

Probed escapes: `launchctl submit` and `osascript do shell script` children stay
sandboxed (`.agent-sdlc` write: `Operation not permitted`).

Throwaway Go cache (follow-up): each run gets its own `GOCACHE`, removed after the
command exits, and writes into `GOMODCACHE` are denied, so one run cannot poison a
later run's caches (`TestRunCommandIsolationThrowawayGoCache`). The repository's own
`go test` for `repoindex`, `security` and `testrunner` passed under the profile
with a cold build cache in 10 s.

### Credential read denial (LLM02)

Baseline `origin/main` `823f753dfd931361b37cd9ac2b92e366d2e16ce9`, macOS arm64.
The profile also denies reading a fixed set of home-relative credential
locations. The names reuse `internal/sensitivepath` (`toolDirs` `.aws`, `.ssh`,
`.kube`, `.gnupg`; files `.netrc`, `.git-credentials`, `.npmrc`) and add only
`.config/gh`, `.config/gcloud`, `.azure`, `.docker/config.json`,
`.cargo/credentials.toml`, and `.pypirc`; no new configuration option exists.
Missing entries are skipped, and an unresolvable home directory fails closed with
`errIsolationUnavailable` so the command never runs unsandboxed. Seatbelt emits
`(deny file-read* (subpath ...))` for directories and
`(deny file-read* (literal ...))` for files, for the path and its real path
(the same `EvalSymlinks` treatment as the state dir). Because the profile is
allow-default and no `file-read` allow follows, the deny wins: the command cannot
print, copy, or hardlink a credential it cannot read.

The child environment is also scrubbed: `sandboxEnv` drops every entry whose
name matches `redact.IsSecretName` (the shared vocabulary) before adding the
per-run `GOCACHE` override, and the audit detail records `scrubbed_env=<n>`
(names only). Isolation-off runs keep the inherited environment unchanged.

Verification:

```sh
go test ./internal/toolharness/ -run 'TestSandboxHidesHomeCredentials|TestHomeCredentials|TestRunCommandIsolationDeniesCredential|TestIsolateScrubsSecretEnv' -v -count=1
```

Observed: 6 PASS.

- `TestRunCommandIsolationDeniesCredentialRead` — a sandboxed
  `cat "$HOME/.ssh/probe"` failed, the canary did not appear in the captured
  output, and an ordinary file under the same fake `HOME` still read successfully.
  The test sets `HOME` to a temp dir; the real home is never touched.
- `TestRunCommandIsolationDeniesCredentialHardlink` — Seatbelt also checks the
  link source, so `ln` never creates the alias. Measured output:
  `ln: .../.ssh/probe: Operation not permitted` (exit status 1). Because the link
  is refused, there is no alias to read; the branch that would have to prove a
  created alias is unreadable did not run on this host.
- `TestIsolateScrubsSecretEnv` — a sandboxed `env` has no `SOP_TEST_API_KEY`,
  keeps `GOFLAGS`, `HOME`, `PATH`, and the run's `GOCACHE` override, and a
  sandboxed `go version` still runs.
- `TestSandboxHidesHomeCredentials` and `TestHomeCredentialsResolveExistingEntries`
  are deterministic profile/resolution tests and do not need a backend.
  `TestHomeCredentialsFailsClosedWithoutHome` confirms an empty `HOME` returns
  `errIsolationUnavailable`.

Environment variables: scrubbed inside the sandbox (`redact.IsSecretName`);
inherited unchanged when isolation is off. In the measured shell, 15 names were
dropped (names only): `DEEPSEEK_API_KEY`, `DOCKERHUB_TOKEN`,
`FIRECRAWL_API_KEY`, `GEMINI_API_KEY`, `GITHUB_PERSONAL_ACCESS_TOKEN`,
`GOOGLE_AI_API_KEY`, `HUGGINGFACE_API_KEY`, `KITCHEN_TOKEN`,
`LLAMACPP_API_KEY`, `OLLAMA_API_KEY`, `OLMX_API_KEY`, `OPENAI_API_KEY`,
`SOP_CONTROLLER_TOKEN`, `SOP_OMLX_API_KEY`, `SSH_AUTH_SOCK`. The substring
matcher also drops harmless names such as `SSH_AUTH_SOCK` (and would drop
`TERM_SESSION_ID`); no Go toolchain variable used by `go test` matches except
`GOAUTH`, which only affects network module fetches that the sandbox denies
anyway.

Residual measured, not fixed: a hardlink alias created before the run is still
readable through `read_file`, because the file tools run in the SOP process, not
the sandbox.

Known limit: the macOS Keychain is reached through the `security` CLI and a
system service, not by file reads, so the read-deny rules do not cover it.
Measured in the sandbox: `security find-generic-password -s sop-nonexistent-probe`
returned `security: SecKeychainSearchCopyNext: The specified item could not be
found in the keychain.` (exit status 44). That reply proves the service was
reachable; it is not a denial.

Limits: Linux uses the bubblewrap backend
([bubblewrap-verification.md](bubblewrap-verification.md#run_command-opt-in-sandbox-llm03)); the
equivalent `--tmpfs <dir>` / `--ro-bind /dev/null <file>` hiding is `NOT_RUN` on
this host because `bwrap` is absent, and `--tmpfs` on a symlinked credential path
is unverified there. Seatbelt
cannot nest, so the tests skip inside another sandbox. The outbound-deny test is
only meaningful on a host with network access.
