# LLM06 — `run_command` sandbox: default-on measurements (OWASP gap 13)

**Status:** measured on macOS arm64 and Linux arm64. The measurements below
supersede the partial 2026-11-11 run in
[Superseded measurement](#superseded-measurement-2026-11-11) (PR #414); that
section is preserved as history, not as current fact.

**Scope:** the OS sandbox that wraps **only** the agent `run_command` tool
(`internal/toolharness.isolate`). No other execution path is covered here. The
sandbox default is unchanged: still opt-in
(`SOP_AGENT_RUN_COMMAND_ISOLATION=1` or `toolharness.Config.IsolateCommands`).
Changing the default is an operator decision; see
[PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](../../plans/PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md).

## Environment

| Item     | macOS                              | Linux                                                                |
| -------- | ---------------------------------- | -------------------------------------------------------------------- |
| Host     | Darwin 25.6.0 arm64 (macOS 26.6.2) | Debian 13 (trixie) arm64, `golang:1.27` container, privileged, uid 0 |
| Backend  | `/usr/bin/sandbox-exec` (Seatbelt) | `bubblewrap 0.12.0`                                                  |
| Go       | 1.27.1 darwin/arm64                | 1.27.4 linux/arm64                                                   |
| Revision | `333eca51` (merge of PR #436)      | same working tree, bind-mounted                                      |

The Linux host is a container because this machine has no `bwrap`. It runs
privileged so `bwrap` can create namespaces; the sandbox semantics are
bubblewrap's, but a container is not a distribution host. Treat the Linux
numbers as one bounded environment, matching the existing
[bubblewrap-verification.md](../security-shift-left/bubblewrap-verification.md)
convention.

## Execution boundary (what default-on would and would not cover)

`isolate` has exactly one caller: `runCommandWithStatus` in
`internal/toolharness/harness.go`. Every other subprocess path is separate:

| Path                                                              | OS sandbox                                      | Notes                                                                                                         |
| ----------------------------------------------------------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| Agent `run_command` (`toolharness`)                               | **opt-in (this document)**                      | One caller: `runCommandWithStatus`.                                                                           |
| Validation commands (`internal/testrunner`)                       | none                                            | `exec.CommandContext(ctx, "sh", "-c", command)`; inherits the full environment.                               |
| Security scanners (`internal/security`)                           | none                                            | Scanner argv runs directly; Promptfoo is explicitly not network-isolated.                                     |
| Replay / eval child (`internal/evaltarget`, `internal/dqharness`) | own OS isolation, always applied on those paths | Independent of `run_command`; cannot nest inside it (see [Compatibility](#compatibility-inside-the-sandbox)). |
| External agent CLI (`internal/agent`)                             | outside SOP                                     | Operator-configured command.                                                                                  |
| `git` / `gh` invocations                                          | none                                            | Process-group bounded, not sandboxed.                                                                         |

Default-on isolation therefore closes the credential-read exposure on
`run_command` only. It does not change validation commands, scanners, or the
git/gh sinks, and this document makes no claim about them.

## Compatibility inside the sandbox

Measured with the sandbox forced on and the full repository suite run through
`run_command`.

### macOS arm64

| Measurement                                     | Result                                                                                                    |
| ----------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| `go test ./...` outside the sandbox             | PASS, 86.1 s                                                                                              |
| `go test ./...` inside the sandbox              | 87.2 s and 91.1 s (two runs), exit 1; **12 test failures in 5 packages**                                  |
| `go build ./...` inside the sandbox             | exit 0, 7.8 s                                                                                             |
| `go vet ./internal/redact/` inside the sandbox  | exit 0, 1.9 s                                                                                             |
| `go test ./internal/redact/` inside the sandbox | exit 0, 2.7 s                                                                                             |
| Module cache                                    | readable read-only; `go` prints a non-fatal `writing stat cache: … operation not permitted` and continues |

The in-sandbox suite is 1–6 s slower than the unsandboxed suite, so the cold
`GOCACHE` per run is no longer a material cost at this size. It still must fit
the harness's default 120 s `CommandTimeout`; 87–93 s leaves little margin.

| Package                      | Failing test                                                                                                                                                                                    | Cause                                                                                                                                                                                                                                              |
| ---------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/toolharness`       | `TestHomeSocketsAreMasked`, `TestHomeSymlinkToDirectoryIsFollowed`                                                                                                                              | `listen unix …: bind: operation not permitted`. The profile's `(deny network*)` denies AF_UNIX **bind**, so a sandboxed process cannot create its own socket.                                                                                      |
| `internal/dqharness`         | `TestIsolatedReplayReplayDirWritesSucceed`, `TestIsolatedReplayRunsVerificationInFreshDir`, `TestIsolatedReplayScrubsCredentials`, `TestIsolatedReplayTimesOut`, `TestIsolatedReplayUnixSocket` | `sandbox-exec: sandbox_apply: Operation not permitted`. Seatbelt cannot nest.                                                                                                                                                                      |
| `internal/evaltarget`        | `TestDarwinIsolationBlocksOrdinaryWorkspace`, `TestDarwinIsolationStripsSecrets`, `TestLiveIsolationDarwin`                                                                                     | Same nesting failure.                                                                                                                                                                                                                              |
| `internal/skill`             | `TestValidationHandoffPortability`                                                                                                                                                              | `/bin/ps: Operation not permitted`. Seatbelt refuses the setuid `ps`.                                                                                                                                                                              |
| `internal/controller/config` | `TestLoadWorkspacesWithoutProjects` (package aborts)                                                                                                                                            | Environment-dependent. `sandboxEnv` dropped the operator shell's real `SOP_CONTROLLER_TOKEN` while keeping `SOP_CONTROLLER_ALLOW_NETWORK=true`, so `Load()` reached its `log.Fatal` and exited. The credential value never appeared in any output. |

### Linux arm64 (privileged container)

| Measurement                                                                      | Result         |
| -------------------------------------------------------------------------------- | -------------- |
| Isolation test subset (`Isolation`, `Bubblewrap`, `HomeSocket`, `LLM02Linux`, …) | 21 pass, 1 skip (the macOS-only keychain probe) |
| `go test ./...` inside the sandbox                                               | exit 1, 69 s   |

| Package               | Failing test                                  | Cause                                                                                                                              |
| --------------------- | --------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| `internal/evaltarget` | `TestLiveIsolationLinuxCancelsProcessGroup`   | `bwrap: Can't mount proc on /proc: Operation not permitted` at nesting depth 2. A nested sandbox that mounts `/proc` cannot start. |
| `internal/dist`       | `TestInstallerInstallsThePlannerSetupCommand` | Also fails in the same container **without** the sandbox (worktree `.git` is unreachable from the mount). Not sandbox-caused.      |
| `internal/testfast`   | `TestInstallOllamaAgentStopsWhenGateFails`    | Also fails in the same container **without** the sandbox. Not sandbox-caused.                                                      |

Nested `bwrap` without `--proc` works, so the failure is specific to mounting
`/proc` inside an existing PID namespace, not to bubblewrap in general.

## Protection matrix

`run_command`, sandbox on. Every row below was executed on this tree;
nothing here is inferred from the profile text alone.

| Property                                   | macOS (Seatbelt)                                                                               | Linux (bubblewrap)                                                                                                         | Windows     |
| ------------------------------------------ | ---------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------- | ----------- |
| Sandbox availability                       | yes                                                                                            | yes (needs unprivileged user namespaces)                                                                                   | **none**    |
| Direct credential file read under `$HOME`  | denied (EPERM) — measured                                                                      | denied (ENOENT via `--tmpfs` mask) — measured                                                                              | n/a         |
| Symlink alias to a credential              | denied — measured (SAFE command leg)                                                           | denied — measured                                                                                                          | n/a         |
| Pre-existing hardlink alias, ordinary name | **readable** — measured                                                                        | **readable** — measured (`RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)`, Option D)                                  | n/a         |
| Credential paths named by env overrides    | denied — measured                                                                              | denied — measured                                                                                                          | n/a         |
| Child environment                          | scrubbed by name and value shape — measured                                                    | scrubbed — measured                                                                                                        | n/a         |
| Parent launch-time environment             | **readable** via `kern.procargs2` — measured                                                   | not reachable (`--unshare-pid`, fresh `/proc`) — measured                                                                  | n/a         |
| Login keychain                             | denied (`com.apple.SecurityServer` mach lookup) — measured                                     | n/a                                                                                                                        | n/a         |
| Outbound network                           | denied, except host `localhost` (the profile re-allows loopback, so a host service on 127.0.0.1 stays reachable) | denied (`--unshare-net`, so even host loopback is unreachable) | n/a |
| Writes                                     | root + temp only; `.agent-sdlc` denied — measured                                              | root + temp only; `.agent-sdlc` read-only — measured                                                                       | n/a         |
| Unix sockets                               | AF_UNIX `bind` denied — measured; unix traffic falls under `network*`                           | host `/tmp`, `/run` hidden; `$HOME` masked by a bounded fail-closed scan; sockets elsewhere on the host remain connectable | n/a         |
| Nested sandbox                             | impossible — measured                                                                          | `/proc` mount fails at depth 2 — measured                                                                                  | n/a         |
| Missing backend                            | fail closed, nothing executes — measured                                                       | fail closed — measured                                                                                                     | fail closed |
| Backend present but refuses to start       | fail closed, no fallback — measured (`TestRunCommandIsolationDoesNotFallBackWhenBackendFails`) | same                                                                                                                       | n/a         |

The parent-environment result is the decisive one: **enabling the sandbox does
not stop disclosure of credentials that are in the SOP process's environment at
launch.** The child's own environment is scrubbed, but a sandboxed process walks
its ancestors with `kern.procargs2` and reads the launch-time environment. This
is the R4 known limit
([seatbelt-evidence.md](../security-shift-left/seatbelt-evidence.md#parent-environment-via-kernprocargs2-narrow-deny-attempts-owasp-gap-6)),
re-confirmed here with a synthetic canary in three ancestor processes.

## R3 reproduction (synthetic)

A temporary Go module whose test reads `$HOME/.ssh/probe`, executed through the
real harness as the SAFE command `go test -v -run TestReadHomeCredential ./...`.
The test logs the file content, so the raw command output either shows the
canary or the denial:

| Isolation | macOS | Linux |
| --- | --- | --- |
| off (shipped default) | canary in the raw output - exposure | canary in the raw output - exposure |
| on | `read credential: ... operation not permitted` | `read credential: ... no such file or directory` |

Pinned as regression tests, with different coverage per platform:

| Test | Off leg | On leg | Hardlink leg |
| --- | --- | --- | --- |
| `TestDarwinIsolationSAFECommandCredentialRead` (new) | yes, through `run_command` | yes | yes (alias readable) |
| `TestLinuxIsolationSAFECommandCredentialRead` (PR #384) | no | yes | yes (alias readable) |
| `TestLinuxIsolationOffAllowsCredentialRead` (PR #384) | yes, but as a plain unsandboxed read, not through `run_command` | n/a | n/a |

So the Linux isolation-off exposure is measured and recorded, but only the macOS
test pins the off leg through `run_command`.

## Failure behavior

| Condition                                         | Behavior                                                                                                                                        | Class           |
| ------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | --------------- |
| No `sandbox-exec` / `bwrap` on `PATH`             | `errIsolationUnavailable`; audit records `deny`; nothing executes                                                                               | **fail closed** |
| Platform with no backend (Windows)                | same                                                                                                                                            | **fail closed** |
| Backend exists but refuses at startup             | the wrapped command fails with the backend's output; no fallback                                                                                | **fail closed** |
| Unresolvable root, home, or relative env override | `errIsolationUnavailable` before exec                                                                                                           | **fail closed** |
| `$HOME` scan past 20 000 entries | `errIsolationUnavailable`; `run_command` refused | **fail closed** |
| `$HOME` scan past 4 directory levels | the scan stops silently; deeper sockets are not masked | **truncated, not fail closed** |
| `.agent-sdlc` is a symlink                        | refused (Linux)                                                                                                                                 | **fail closed** |
| Timeout / cancellation                            | command killed; Linux cancellation covered by `TestLinuxIsolationCancellationKillsChild`; child cleanup via `--die-with-parent` / process group | bounded         |

The 20 000-entry bound is an operational risk for default-on: a large home
directory would turn every `run_command` into a denial.

## What this does not cover

- Windows: no backend, so no runtime measurement is possible.
- A distribution host where `bwrap` cannot create user namespaces (for example
  Ubuntu 24.04's AppArmor default). Only the privileged container was available.
- Linux `amd64`.
- A live model. Every result here is deterministic or a direct execution
  measurement, not a live agent campaign.
- Any execution path other than `run_command`.

## Superseded measurement (2026-11-11)

Preserved as history. Abridged from PR #414: the block quote keeps the table
and the conclusion; the original "Decision for the operator" paragraph is
summarised below it.

> **Status:** partial. Measured on macOS only. The Linux socket gap is **not**
> verified: this host has no bubblewrap, so the Linux socket fix cannot be
> reproduced or tested here.
>
> | Measurement                                                                                   | Result                                                                                                         |
> | --------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
> | `go test -count=1 ./internal/run/`, outside the sandbox, fresh build cache, warm module cache | 4.4 s per run (2 runs)                                                                                         |
> | The same command inside the sandbox profile, with the build cache under `/private/tmp`        | Fails before compiling: `go: could not create module cache: stat /Users/…/go/pkg/mod: operation not permitted` |
> | Earlier attempt with the build cache under the test temp directory                            | Fails: `mkdir /var: file exists` (the `/var` symlink path is not usable by the cache setup)                    |
>
> **What this means for default-on:** a sandboxed Go command cannot use the
> module cache under `$HOME` on this machine…
>
> **Not measured:** which existing tests fail inside the sandbox across the full
> suite; behavior on a platform with no sandbox; the Linux bubblewrap socket gap
> and cold-cache overhead on Linux.

**Reconciliation.** The `could not create module cache` failure did not
reproduce at `333eca51`: with a populated module cache, `go build ./...`,
`go vet`, `go test` and the full suite all run inside the profile, and the only
module-cache symptom is a non-fatal `writing stat cache` warning. The
full-suite and unsupported-platform gaps listed above are now measured. The
Linux socket gap was closed by PR #421 and verified in the container above.
