# LLM06 — `run_command` sandbox: default-on measurements (OWASP gap 13)

**Status:** measured on macOS arm64 and Linux arm64. The measurements below
supersede the partial 2026-11-11 run in
[Superseded measurement](#superseded-measurement-2026-11-11) (PR #414); that
section is preserved as history, not as current fact. The compatibility section
records the post-remediation re-measurement of the failures PR #437 reported.

**Scope:** the OS sandbox that wraps **only** the agent `run_command` tool
(`internal/toolharness.isolate`). No other execution path is covered here.

**Default:** isolation is **required by default** on every platform, approved by
the operator and recorded in
[PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](../../plans/PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md).
`SOP_AGENT_RUN_COMMAND_ISOLATION=off` is the operator override and prints one
startup notice; a host with no backend refuses the command (Windows B1). The
measurements below therefore also cover the default path directly.

## Environment

| Item     | macOS                              | Linux                                                                |
| -------- | ---------------------------------- | -------------------------------------------------------------------- |
| Host     | Darwin 25.6.0 arm64 (macOS 26.6.2) | Debian 13 (trixie) arm64, kernel 6.18.60-0-virt, `golang:1.27` container, privileged, uid 0 |
| Backend  | `/usr/bin/sandbox-exec` (Seatbelt) | `bubblewrap 0.12.0`; `user.max_user_namespaces=59993`                |
| Go       | 1.27.1 darwin/arm64                | 1.27.4 linux/arm64                                                   |
| Revision | `333eca51` (PR #437 baseline)      | same working tree, bind-mounted                                      |

The PR #437 baseline was measured at `333eca51`. The post-remediation rows were
measured on branch `security/llm06-isolation-compat` on top of `e4ca505d`.

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
`run_command`. PR #437 recorded the baseline; the tables below are the
post-remediation re-measurement, and every originally failing test is listed with
its disposition.

### macOS arm64

| Measurement                                      | PR #437 baseline                        | After remediation          |
| ------------------------------------------------ | --------------------------------------- | -------------------------- |
| `go test ./...` outside the sandbox              | PASS, 86.1 s                            | PASS                       |
| `go test ./...` inside the sandbox               | exit 1, 87-91 s, **12 failures / 5 pkgs** | exit 0, **87-91 s, 0 failures** |
| `go build ./...` inside the sandbox              | exit 0, 7.8 s                           | exit 0, 7.3 s              |
| `go vet ./...` inside the sandbox                | one package, exit 0, 1.9 s              | exit 0, 8.7 s              |
| `go test ./internal/redact/` inside the sandbox  | exit 0, 2.7 s                           | exit 0, 2.3 s              |
| Module cache                                     | readable read-only, non-fatal stat-cache warning | unchanged         |

### Default path (no configuration, no override)

The production default was exercised directly: `DefaultConfig()`, no
`IsolateCommands`, the override cleared, and `go test ./...` run through
`run_command`.

| Platform | Result |
| --- | --- |
| macOS arm64 | exit 0, 90.4 s, audit detail `isolate=seatbelt backend=/usr/bin/sandbox-exec`, `scrubbed_env=15` |
| Linux arm64 (container, bwrap 0.12.0) | exit 0, 66.2 s, audit detail `isolate=bubblewrap backend=/usr/bin/bwrap`, failures identical to the same container's no-sandbox baseline |

The operator override and the B1 refusal are pinned by
`TestRunCommandIsolationIsOnByDefault`,
`TestRunCommandIsolationDefaultsToFailClosed` and `TestIsolationPolicyNotice`.

The post-remediation in-sandbox runs (87.3 s and 91.3 s) are within a few seconds
of the same host's unsandboxed run (86.1 s), so the cold
`GOCACHE` per run is not a material cost at this size. It still must fit the
harness's default 120 s `CommandTimeout`; 87-91 s leaves little margin.

### macOS dispositions

| Failing test (PR #437)                                                                  | Classification      | Root cause                                                                                                                                                 | Disposition                                                                                                                                                                                                                       |
| --------------------------------------------------------------------------------------- | ------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `internal/toolharness` `TestHomeSocketsAreMasked`, `TestHomeSymlinkToDirectoryIsFollowed` | Isolation defect    | `(deny network*)` also denies AF_UNIX **bind**, so a sandboxed process cannot create its own socket. Linux allows the same operation inside its private `/tmp`. | **Fixed.** The profile re-allows `network-bind` only for socket paths under a writable dir. Outbound AF_UNIX to a host socket is still denied, pinned by `TestRunCommandIsolationAllowsOwnUnixSocketOnly`; `TestSeatbeltProfileScopesUnixSocketBind` blocks a blanket allow. |
| `internal/dqharness` `TestIsolatedReplay*` (5)                                          | Unsupported workflow | Seatbelt cannot nest: `sandbox_apply: Operation not permitted`.                                                                                            | **Classified unsupported.** `isolationBlocker` probes the real backend arguments and the tests skip with the reason. Production is unchanged and fail-closed; the parent boundary is never reused.                                  |
| `internal/evaltarget` `TestDarwinIsolation*`, `TestLiveIsolationDarwin` (3)              | Unsupported workflow | Same nesting limit.                                                                                                                                        | Same: `requireSandbox` skips when the sandbox cannot start.                                                                                                                                                                        |
| `internal/skill` `TestValidationHandoffPortability`                                     | Unsupported workflow | The handoff validator proves a candidate is running by reading the process table; Seatbelt refuses the setuid `ps`.                                         | **Classified unsupported** for that check. The whole workflow needs the process table: the fixture script no longer calls `ps`, but the validator it drives (`scripts/validation-handoff.py`) does, so the test skips when the table is unreadable. The validator's liveness check is **not** weakened.                          |
| `internal/controller/config` `TestLoadWorkspacesWithoutProjects` (package aborts)        | Test assumption     | Ambient `SOP_CONTROLLER_ALLOW_NETWORK=true` paired with a scrubbed `SOP_CONTROLLER_TOKEN` reached `Load()`'s `log.Fatal`.                                   | **Fixed** in test infrastructure: a package `TestMain` clears ambient `SOP_CONTROLLER_*`, so the tests no longer depend on the operator's shell.                                                                                    |

### Linux arm64 (privileged container)

| Measurement                        | PR #437 baseline                          | After remediation                                                    |
| ---------------------------------- | ----------------------------------------- | -------------------------------------------------------------------- |
| `go test ./...` inside the sandbox | exit 1, 69 s, 1 sandbox-caused failure    | exit 1, **0 sandbox-caused**; the 2 failures below fail identically without the sandbox |
| Touched packages, no sandbox       | -                                         | PASS                                                                 |
| Unix-socket and credential controls inside the sandbox | -                        | PASS                                                                 |

| Package               | Failing test                                  | Cause                                                                                                                         |
| --------------------- | --------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `internal/dist`       | `TestInstallerInstallsThePlannerSetupCommand` | Fails in the same container **without** the sandbox (the worktree's `.git` is unreachable from the mount). Not sandbox-caused. |
| `internal/testfast`   | `TestInstallOllamaAgentStopsWhenGateFails`    | Fails in the same container **without** the sandbox. Not sandbox-caused.                                                      |

`TestLiveIsolationLinuxCancelsProcessGroup` now skips with the reason
`bwrap: Can't mount proc on /proc: Operation not permitted`, matching the macOS
nesting class.

No real Linux host was available, so these numbers are one privileged container
on arm64 only; see [What this does not cover](#what-this-does-not-cover).

### Nested isolation: why the parent boundary is not reused

Nested sandboxing is **not** a production requirement of the `run_command`
sandbox. `evaltarget` and `dqharness` isolation are entered from `sop` CLI
commands, not from a sandboxed `run_command`, so nesting happens only when the
repository's own suite runs inside the sandbox.

Propagating the parent boundary to the child was evaluated and rejected. The
inner policy is not a subset of the outer one: `evaltarget`'s live variant needs a
Unix-socket proxy to exactly one endpoint, and `dqharness` replay confines writes
to a replay directory and hides host paths, while the `run_command` profile
allows writes across the repository root and temp. Reusing the outer boundary
would either break the inner workflow or silently widen the inner confinement.
No mechanism proves "the inherited boundary satisfies the child's policy", so
nothing is inherited and the child stays fail-closed.

Measured nesting constraint on Linux: nested `bwrap` fails only when
`--unshare-all` (user and PID namespaces) is combined with `--proc /proc`.
Nested `bwrap` without `--proc`, or without `--unshare-pid`, starts. This is what
the probe reproduces, so the skip is tied to the real condition rather than to a
guess.

### Profile observation (pre-existing, unchanged)

The `(allow network-bind network-inbound (local ip "localhost:*"))` rule does not
prevent a sandboxed process from **binding** `0.0.0.0`; inbound connections stay
loopback-only. This was already true before this change and is not altered here.
Changing it is a policy decision, not a compatibility fix.

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
| Unix sockets                               | AF_UNIX `bind` allowed only under a writable dir; outbound AF_UNIX denied, but a same-user host process can dial a created socket — measured | host `/tmp`, `/run` hidden; `$HOME` masked by a bounded fail-closed scan; sockets elsewhere on the host remain connectable | n/a         |
| Nested sandbox                             | impossible (Seatbelt cannot nest); tests skip, production fails closed — measured               | `/proc` mount fails at depth 2 — measured                                                                                  | n/a         |
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
- The nested-isolation tests themselves, when the suite runs inside the sandbox.
  They skip with an explicit reason and still run unsandboxed, so the inner
  boundaries stay covered in CI and on a developer host.
- The credential-read guarantees under `bwrap` on a real distribution host; only
  the container was available.

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
