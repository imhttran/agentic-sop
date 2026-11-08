# SEC-LEFT-005 — Linux Bubblewrap verification (NOT_RUN on this host)

## Environment

- Host used for this run: `Darwin` (macOS, arm64).
- `bwrap` is **not** installed on this host: `command -v bwrap` returns nothing.
- `dqharness.DetectIsolation()` therefore selects `seatbelt` here, and every
  Linux-only confinement fixture skips with
  `unix-socket boundary test requires the Linux bubblewrap backend, got "seatbelt"`.

## Status

Linux Bubblewrap runtime isolation is **NOT_RUN / UNVERIFIED** in this
environment. No Linux runtime result is claimed here, and no historical Linux
result is recorded elsewhere in this repository, so none is cited.

The macOS Seatbelt runtime results that did run are in
[seatbelt-evidence.md](seatbelt-evidence.md).

## Existing safe fixtures that would produce the Linux evidence

These fixtures already exist and require no new code. They run in disposable
temporary directories and never touch user-owned secrets or services:

| Test | What it checks |
| --- | --- |
| `TestIsolatedReplayFailsClosedWithoutIsolation` | With an empty PATH the replay refuses; the command never runs unsandboxed. |
| `TestIsolatedReplayRunsVerificationInFreshDir` | A fresh replay directory is used and removed. |
| `TestIsolatedReplayScrubsCredentials` | Fake `OLLAMA_API_KEY`, `SOP_CONTROLLER_TOKEN`, `SOP_OMLX_API_KEY` are absent in the child. |
| `TestIsolatedReplayDeniesWritesOutside` | Writes outside the replay directory fail; nothing escapes. |
| `TestIsolatedReplayConfinementTempRoots` | Parent and sibling writes under `/tmp` and `/dev/shm` fail. |
| `TestIsolatedReplayDeniesPrivateMountWrite` | A write under a private `/dev/shm` mount fails and does not land on the host. |
| `TestIsolatedReplayReplayDirWritesSucceed` | Writes inside the replay directory succeed. |
| `TestIsolatedReplayDeniesNetwork` | A network connection fails. |
| `TestIsolatedReplayTimesOut` | The timeout fires. |
| `TestIsolatedReplayUnixSocket` (Linux-only) | A host Unix socket is CONNECTED outside and DENIED inside; no post-isolation connection. |
| `TestIsolatedReplayToolchainProcAndTemp` (Linux-only) | `/proc` present, `TMPDIR` inside the replay directory, a real `go test` runs. |
| `TestReplayIsolationRequiredEnv` | Skips unless `SOP_REPLAY_ISOLATION_REQUIRED=1`; then a missing backend is a failure. |

## Cross-platform checks that DID run here (PASS)

These are deterministic argument/fail-closed checks, not runtime confinement:

- `TestBubblewrapArgumentsShape` — argument shape binds only the replay directory
  at `/replay` writable and refuses forbidden host binds.
- `TestBubblewrapArgumentsExcludeSeatbelt`
- `TestBubblewrapCommandCanonicalizationFailsClosed`
- `TestIsolationCommandUnknownBackendFailsClosed`
- `TestIsolationBackendSelection` (linux/darwin/windows/plan9)

## Prerequisite to close this gap

Run on a Linux host with `bwrap` installed (and `python3` for the socket client):

```sh
SOP_REPLAY_ISOLATION_REQUIRED=1 go test ./internal/dqharness/ -run 'TestIsolatedReplay|TestReplayIsolationRequiredEnv' -v
```

Until that is recorded, the Linux confinement claim stays `NOT_RUN` and must not
be reported as verified.
