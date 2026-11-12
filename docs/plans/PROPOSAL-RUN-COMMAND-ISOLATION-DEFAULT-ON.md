# Proposal: default-on OS sandbox for `run_command` (OWASP gap 13)

**Status: `APPROVED` / `IMPLEMENTED`.** The operator authorized the policy
verbatim: "Approve macOS/Linux default-on isolation with an operator-controlled
override, subject to documented compatibility constraints. Select Windows B1
(fail closed), with its compatibility impact explicitly acknowledged."

Implemented as: isolation is required by default on every platform, the
`SOP_AGENT_RUN_COMMAND_ISOLATION` override accepts `off`/`0`/`false`/`no` with a
startup notice, and a host with no backend refuses `run_command` (B1) instead of
running it unsandboxed. `Config.IsolateCommands` remains the programmatic opt-in
and wins over the override. Verification is recorded in the
[measurement report](../reports/owasp-llm/llm06-sandbox-default-measurements.md).

Not accepted by this decision: the residual exposures in section 6. R3 moves to
`MITIGATED` for the direct-read vector only; R4 and the Option-D hardlink alias
stay open.

Related: [PROPOSAL-RUN-COMMAND-ISOLATION.md](PROPOSAL-RUN-COMMAND-ISOLATION.md)
(the original opt-in design, implemented in PRs #343, #354, #356),
[llm06-sandbox-default-measurements.md](../reports/owasp-llm/llm06-sandbox-default-measurements.md)
(the measurements this proposal rests on), and
[llm02-decision-packet.md](../reports/owasp-llm/llm02-decision-packet.md) residual
R3.

## 1. The exposure this would close

`go test` is `SAFE` (`internal/commandpolicy`), so model-written test code is
ordinary authorized work. With the sandbox off — the shipped default —
`run_command` launches that code with the SOP process's full environment and
filesystem access. A test can read `$HOME` credentials, or hardlink them into the
repository, and the result reaches the model.

Reproduced with synthetic canaries on macOS arm64 and Linux arm64: a temporary
module whose test reads `$HOME/.ssh/probe` prints the canary with isolation off
and is denied with isolation on
(`TestDarwinIsolationSAFECommandCredentialRead`,
`TestLinuxIsolationSAFECommandCredentialRead`).

## 2. Current configuration and behavior

- Opt-in. `SOP_AGENT_RUN_COMMAND_ISOLATION` in {`1`,`true`,`yes`} or
  `toolharness.Config.IsolateCommands` turns it on; anything else leaves it off.
  `internal/toolharness/isolation.go`, `(*Harness).isolationRequested`.
- One caller wraps argv: `runCommandWithStatus` in
  `internal/toolharness/harness.go`, after `checkCommandIn`.
- Backends: Seatbelt (`sandbox-exec`) on Darwin, bubblewrap (`bwrap`) on Linux,
  none elsewhere. Missing backend → `errIsolationUnavailable`, audit `deny`,
  nothing executes.
- The production tool harness (`internal/ollamaagent/tools.go`) does not set
  `IsolateCommands`; it relies on the environment variable.
- Scope: `run_command` only. Validation commands, security scanners, and git/gh
  sinks are not sandboxed and are unaffected by this proposal.

## 3. Options

|                           | A — default-on, no override                                                            | B — default-on with an override                                                | C — keep opt-in                   |
| ------------------------- | -------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | --------------------------------- |
| Security coverage         | Highest: no way to run unsandboxed                                                     | Same default; an operator can disable it                                       | Unchanged: policy-only by default |
| Compatibility             | `run_command` unusable on Windows and on any Linux host where `bwrap` cannot start     | Usable everywhere; incompatible hosts opt out                                  | No change                         |
| Cross-platform            | Windows loses `run_command` entirely                                                   | Platform-scoped default keeps Windows working (or denies it, sub-choice B1/B2) | No change                         |
| Operational failure modes | A single unavailable backend disables the agent's whole build/test loop with no remedy | None beyond a mis-set environment variable                                     | Exposure stays live               |
| Developer experience      | 12 of this repository's own tests fail; nested replay/eval cannot run                  | Same default failures, but escapable                                           | None                              |
| Migration                 | Every caller/test must assume isolation                                                | Tests that mean "off" must say `off`                                           | None                              |
| Residual risks            | Hardlink aliases; macOS parent env; env-scrub heuristic                                | Same                                                                           | Same, plus the direct read        |
| Rollback                  | Revert the default; no data change                                                     | Revert the default; no data change                                             | n/a                               |

### Compatibility evidence (from the measurement report)

Post-remediation, with the sandbox on:

- **macOS arm64:** `go test ./...` passes in 87-91 s inside the sandbox against
  86 s outside, with **zero failures**. `go build`, `go vet` and single-package
  tests succeed.
- **Linux arm64 (privileged container):** `go test ./...` has **zero
  sandbox-caused failures**. The two failures in that container also fail with
  the sandbox off, so they are container artifacts, not isolation.

PR #437 recorded 12 failures in 5 packages. Reaching zero took two fixes and two
classifications:

- **Fixed:** the macOS profile now allows AF_UNIX `bind` for socket paths under a
  writable dir, so a sandboxed build can create its own test listener. Outbound
  AF_UNIX and non-loopback IP access stay denied.
  **This is a profile change and needs its own authorization (see §8).**
- **Fixed:** the controller-config tests no longer read the operator's ambient
  `SOP_CONTROLLER_*` environment, which a scrub could split into an invalid pair.
- **Classified unsupported:** nested OS sandboxing. Seatbelt cannot nest, and
  nested bubblewrap cannot mount `/proc` under `--unshare-all`. The affected
  `evaltarget` and `dqharness` tests skip with an explicit reason; production
  stays fail-closed and never inherits the parent boundary.
- **Classified unsupported:** the handoff portability fixture needs the process
  table, which a sandbox refuses (setuid `ps`). It skips when the table is
  unreadable rather than weakening its liveness check.

Two operational limits remain: a full suite takes about 91 s against the 120 s
default `CommandTimeout`, and the `$HOME` socket scan refuses a home with more
than 20 000 entries.

### The override is not a model-reachable bypass

`SOP_AGENT_RUN_COMMAND_ISOLATION` is read from the SOP process environment at
`run_command` dispatch. A model cannot set its own parent's environment, and
`run_command` cannot set environment for SOP. So Option B's escape hatch is
operator-scoped, not agent-scoped: it does not let the model disable its own
sandbox. It must still be visible — a one-line startup notice is proposed —
because a stale or inherited variable would otherwise silently remove the
control.

### Recommended: Option B

1. **Default on where a backend exists** (Darwin and Linux).
2. **Explicit operator override**: `SOP_AGENT_RUN_COMMAND_ISOLATION=off`
   (also `0`, `false`, `no`), printed once at startup.
3. **Unsupported platforms** (currently Windows): **B1 — deny `run_command`
   uniformly.** Selected by the operator: a host with no backend fails closed.
   The compatibility impact, acknowledged with the decision: on Windows every
   `run_command` is refused with `run_command isolation unavailable`, so the
   agent's build/test loop and any validation that relies on shelling out are
   unavailable there until a Windows backend exists. Operators cannot work around
   it except by running the command outside SOP. The rejected alternative was B2
   (policy-only on Windows with a startup notice).

Option A is rejected: without an escape hatch, one unavailable backend turns a
security improvement into an availability outage. Option C is rejected as the
primary answer because the exposure stays live by default and documentation is
not mitigation — but it remains the correct answer if the operator declines to
change the default, in which case R3 is a recorded accepted residual.

## 4. Exact change implemented

**`internal/toolharness/isolation.go`**

```go
// backendFor is the single platform table: name plus the binary to exec.
func backendFor(goos string) (name, bin string) {
	switch goos {
	case "darwin":
		return "seatbelt", "sandbox-exec"
	case "linux":
		return "bubblewrap", "bwrap"
	}
	return "", ""
}

func isOffValue(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "0", "false", "no", "off":
		return true
	}
	return false
}

func isolationDecision(goos, override string, isolateCommands bool) bool {
	if !hasBackend(goos) {
		return true // refuse: the platform cannot enforce isolation
	}
	if isolateCommands {
		return true
	}
	return !isOffValue(override)
}

func (h *Harness) isolationRequested() bool {
	return isolationDecision(runtime.GOOS, os.Getenv(IsolationEnv), h.cfg.IsolateCommands)
}
```

`isolationDecision` returns true whenever the platform has no backend, **before**
either input is read. `true` sends `run_command` into `sandboxArgv`, which uses
the same `backendFor` table and returns `errIsolationUnavailable` for an empty
name, so nothing executes. The platform table therefore exists once, and a
backend-less host cannot be talked out of the refusal.

Do **not** probe for the backend here with `exec.LookPath`: a missing `bwrap` on
macOS/Linux must produce `errIsolationUnavailable` (fail closed), not "off". An
unrecognized override value also leaves the default in place.

**Precedence:** an explicit `Config.IsolateCommands=true` wins over the override,
so a programmatic opt-in cannot be turned off by the environment. The override
only applies to a caller that leaves the bool false; no production caller sets
it, so in practice the environment decides.

**`internal/cli`**: `sop run` and `sop prompt` print
`toolharness.IsolationPolicyNotice()` once at startup when the environment
disables isolation, so a stale or inherited value cannot remove the control
silently. On an unsupported platform the refusal itself is the diagnostic
(`run_command isolation unavailable: no sandbox backend on windows`); B1 has no
fallback notice because there is no policy-only mode to announce.

**No audit-field change.** `TestRunCommandIsolationOffKeepsExecCapturedPath`
asserts an empty audit detail when isolation is off; adding a detail would change
the audit contract. The notice covers visibility.

**Test migration** (done with the change):

- Tests that mean "off" set `IsolationEnv=off` rather than unset it, e.g.
  `TestRunCommandIsolationOffKeepsExecCapturedPath` and
  `TestSubprocessStateWriteIsOutsideFileToolBoundary`.
- `internal/toolharness` and `internal/ollamaagent` set the override in
  `TestMain`, so their command-policy and agent-loop tests keep their coverage on
  hosts without a backend (CI has no `bwrap`) instead of turning every command
  into a denial.
- `TestRunCommandIsolationIsOnByDefault` clears the override and pins the
  default; `TestRunCommandIsolationDefaultsToFailClosed` simulates a backend-less
  host with a stripped `PATH`; `TestIsolationDecisionUnsupportedPlatforms` covers
  the full Windows matrix; `TestRunCommandIsolationOnWindowsRefuses` proves the
  runtime refusal in the Windows CI job.
- `TestSubprocessStateWriteIsDeniedByDefault` pins that a SAFE command can no
  longer write `.agent-sdlc` under the default.
- The in-sandbox incompatibilities are settled separately: the macOS profile now
  allows AF_UNIX `bind` under a writable dir (a profile change, item 6 in §8), the
  nested-sandbox and process-table tests are classified unsupported and skip with
  a reason, and the controller-config tests no longer read ambient
  `SOP_CONTROLLER_*`.

## 5. Acceptance criteria

1. On Darwin and Linux with a working backend, `run_command` is sandboxed with no
   environment set, and the audit detail names the backend.
2. `SOP_AGENT_RUN_COMMAND_ISOLATION=off` disables it and prints one notice, for
   callers that do not set `Config.IsolateCommands`.
3. A missing or failing backend still fails closed
   (`TestRunCommandIsolationOnFailsClosedWithoutBackend`,
   `TestRunCommandIsolationDoesNotFallBackWhenBackendFails`).
4. The chosen unsupported-platform behavior is observable and documented.
5. `TestDarwinIsolationSAFECommandCredentialRead` and
   `TestLinuxIsolationSAFECommandCredentialRead` pass.
6. `make check` passes outside the sandbox, and the suite passes inside it with
   only the documented unsupported-workflow skips.
7. `gofmt`, `go vet ./...`, `go build ./...` clean.

## 6. Residual exposure that default-on does not close

- **Pre-existing hardlink alias** with an ordinary name is readable inside the
  sandbox on both platforms (measured). Option D accepts this for Linux only.
- **macOS parent launch-time environment** is readable through
  `kern.procargs2` (measured). A credential in SOP's own environment is still
  reachable from a sandboxed command. Linux is closed by `--unshare-pid`.
- **Child environment scrubbing is heuristic** (name substrings and value
  shapes), so a secret under an innocuous name survives.
- **Credentials outside `$HOME` and outside the env-override list** are not
  denied.
- **Credentials in a writable directory** can be renamed or their parent moved
  and then read.
- **Every other execution path** — validation commands, scanners, git/gh — stays
  unsandboxed.
- **Windows** has no sandbox at all.
- **Host-to-sandbox sockets.** Allowing AF_UNIX `bind` lets a same-user host
  process dial a socket a sandboxed command created, so the sandbox is no longer
  a purely one-way listener boundary in that direction. The model cannot create a
  cooperating host process, and the socket can only exist under a writable dir,
  but this is a new capability and is recorded rather than claimed as closed.

## 7. Migration and rollback

Migration: none for data or configuration. Operators who need today's behavior
set `SOP_AGENT_RUN_COMMAND_ISOLATION=off`. Users on hosts where `bwrap` cannot
start will see `run_command` denials with an explicit diagnostic and can opt out.

Rollback: revert `isolationRequested`/`IsolationDefaultOn` and the notice; one
release note. No state, schema, or artifact change.

## 8. Decision recorded

The operator's authorization, quoted verbatim:

> Default-on policy: Approve macOS/Linux default-on isolation with an
> operator-controlled override, subject to documented compatibility constraints.
> Select Windows B1 (fail closed), with its compatibility impact explicitly
> acknowledged.

What that settles, and what it does not:

| Item | Status |
| --- | --- |
| 1. Default `run_command` isolation on | **Approved; implemented** as required-by-default on every platform. |
| 2. `SOP_AGENT_RUN_COMMAND_ISOLATION=off` override with a startup notice | **Approved; implemented** (`off`, `0`, `false`, `no`; `sop run` and `sop prompt` print one notice). |
| 3. Unsupported-platform behavior | **B1 selected and enforced by platform**: on a platform with no backend (Windows) `run_command` is refused, and no override or `Config` value can change that. On macOS/Linux a missing backend binary under the override still runs policy-only, and the startup notice makes that visible. Compatibility impact acknowledged below. |
| 4. Migrating the repository's own tests | **Approved; completed** without changing any other execution path. |
| 5. Accepting the §6 residuals | **Not decided.** The decision covers the policy, not the residual risks. R4 and the Option-D hardlink alias stay open, and the host-to-sandbox socket direction stays recorded. |
| 6. The Seatbelt `network-bind` profile rule | **Submitted for approval with the change**; the operator merged it. It remains the scoped rule described in §6's final bullet. |

### Correction: the override could bypass B1 (fixed)

The first implementation of this decision honoured the operator override before
consulting the platform, so `SOP_AGENT_RUN_COMMAND_ISOLATION=off` (and the other
off spellings) skipped the sandbox decision entirely and allowed policy-only
execution on Windows. That is a B1 violation, not an authorized use of the
override: the operator approved the override as a way to accept weaker execution
*where a sandbox exists*, and approved B1 as an unconditional refusal where none
does.

The correction moves the platform check ahead of the override in a pure decision
(`isolationDecision`), and both it and `sandboxArgv` read the same `backendFor`
table, so the two cannot disagree:

| Platform | Override | `Config.IsolateCommands` | Result |
| --- | --- | --- | --- |
| no backend (Windows) | any value, unset, or unknown | `false` or `true` | **refuse** |
| `darwin`/`linux` | unset, `on`, `1`, `true`, unknown | any | sandbox |
| `darwin`/`linux` | `off`, `0`, `false`, `no` | `false` | policy-only (authorized) |
| `darwin`/`linux` | `off` | `true` | sandbox (`Config` wins) |

`IsolationPolicyNotice` is suppressed where no backend exists, so an operator is
not told the override took effect when it did not. Regression coverage:
`TestIsolationDecisionUnsupportedPlatforms`,
`TestIsolationDecisionSupportedPlatforms`,
`TestRunCommandIsolationRefusesBeforeExecution`, and the Windows runtime test
`TestRunCommandIsolationOnWindowsRefuses` executed by the `windows (install.ps1)`
CI job.

### Windows B1 compatibility impact (acknowledged)

On Windows every `run_command` is refused with `run_command isolation
unavailable: no sandbox backend on windows`. Consequences:

- The agent's build/test loop is unavailable: `go test`, `go build` and `go vet`
  cannot run through `run_command`.
- Any SOP workflow that shells out for validation is unavailable there.
- The only way to run such a command is outside SOP. There is no policy-only mode
  and no automatic fallback.
- Read-only and mutating file tools, and the fixed `git status`/`git diff` helper, are
  unaffected; only `run_command` subprocess execution is.

This is the fail-closed trade the decision selects. It is reversible in one line
(`isolationRequested`) if a Windows backend is added or B2 is preferred later.

### Remaining operator actions

- Dispose of R4 (macOS `kern.procargs2` parent-environment read) and the Option-D
  hardlink alias, plus the host-to-sandbox socket direction.
- Record this decision in decision memory if a durable ledger entry is wanted;
  this document is the in-repository record, and no decision-memory entry is
  created by the implementing change.
