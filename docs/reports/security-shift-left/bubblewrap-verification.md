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

## run_command opt-in sandbox (LLM03)

The Linux backend for `SOP_AGENT_RUN_COMMAND_ISOLATION=1`
(`internal/toolharness/isolation.go` `bubblewrapArgs`). It matches the macOS
Seatbelt profile:

- the host is mounted read-only (`--ro-bind / /`);
- `/tmp` and `/run` are private tmpfs mounts, which hides host sockets such as
  Docker's;
- only the root is bound read-write (paths under `/tmp`, `/run` or `/var/run`
  are never bound, so host sockets stay hidden); each run's `GOCACHE` is a fresh
  dir in the private `/tmp`, and the shared module cache stays read-only;
- `<root>/.agent-sdlc` is mounted read-only after the root bind, so the mount
  point cannot be unlinked or renamed; a symlinked state dir is refused (the
  writable root would let the command replace the link), and a missing one gets
  an empty read-only tmpfs;
- `--unshare-net --unshare-pid --unshare-ipc --die-with-parent --new-session`.

It fails closed: missing `bwrap` returns `errIsolationUnavailable` and the
command does not run.

Verification (2026-11-09, `golang:1.27` container on Linux arm64, bubblewrap
0.12.0, unprivileged user):

- `go test ./internal/toolharness -run 'Isolation|Bubblewrap'`: PASS. This covers
  the root write reaching the host, the denied state-dir write, the refused
  symlinked state dir, the blocked outbound connection to 1.1.1.1:443, and the hidden
  `/tmp` Unix socket.
- The repository's own `go test` for `repoindex`, `security` and `testrunner`
  ran inside the generated view: PASS, with the module cache downloaded first.
  With an empty module cache the download fails inside the sandbox
  (`network is unreachable`), as intended.
- `touch .agent-sdlc/forged` inside the view: `Read-only file system`.
- Found while testing: binding the writable dirs skipped any path under `/tmp`,
  which also skipped a root under `/tmp`. The private tmpfs then hid the real
  root, and writes went to a throwaway copy. The root is now always bound, and
  the test checks that the write reached the host.

Limits:

- Verified in one container on arm64, not on an x86_64 host or a distribution
  that restricts unprivileged user namespaces (for example Ubuntu 24.04's
  AppArmor default). There `bwrap` cannot start, so the sandbox fails closed.
- CI does not install `bwrap`, so these runtime tests skip in CI; only
  `TestBubblewrapArgsOrder` runs there.
- The sandbox has its own network namespace, so the host's loopback is
  unreachable.

### Throwaway Go cache (follow-up)

Each sandboxed run sets `GOCACHE` to a fresh dir that is removed after the
command exits. The module cache is no longer writable: `go.sum` verifies its
contents, so sharing it read-only is safe.
`TestRunCommandIsolationThrowawayGoCache` checks that two runs get different
caches, that a run's cache is deleted afterwards, and that writing into
`GOMODCACHE` fails. The repository's own `go test` for `repoindex`, `security`
and `testrunner` passed inside the view with a cold build cache (11 s, Linux
arm64 container, bwrap 0.12.0).
