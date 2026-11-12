# Proposal: default-on OS sandbox for `run_command` (OWASP gap 13)

**Status: `NEEDS_HUMAN` — proposal only.** Nothing here is implemented. The
sandbox default is unchanged (opt-in). Changing it is a security-policy change
and requires the operator authorization listed at the end.

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

With the sandbox on, on macOS arm64: `go test ./...` takes 87–93 s against 86 s
unsandboxed and fails 12 tests in 5 packages — AF_UNIX `bind` denied, nested
Seatbelt impossible, setuid `ps` refused, and one env-scrub interaction. On Linux
arm64 inside a privileged container: 69 s, with the one sandbox-caused failure
being nested `bwrap` unable to mount `/proc`. `go build`, `go vet` and single
package tests all succeed.

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
3. **Unsupported platforms** (currently Windows): sub-choice for the operator —
   **B1** deny `run_command` uniformly, or **B2** keep today's policy-only
   behavior with a startup notice. B2 is recommended because
   `PLAN-Phase-5.6-Windows-Distribution.md` ships a Windows agent and B1 removes
   a documented capability there; B1 is the stricter reading of default-deny.

Option A is rejected: without an escape hatch, one unavailable backend turns a
security improvement into an availability outage. Option C is rejected as the
primary answer because the exposure stays live by default and documentation is
not mitigation — but it remains the correct answer if the operator declines to
change the default, in which case R3 is a recorded accepted residual.

## 4. Exact change required (not implemented)

**`internal/toolharness/isolation.go`**

```go
// IsolationDefaultOn is the platform default when the override is unset. A
// backend only exists on darwin and linux.
func IsolationDefaultOn() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "linux"
}

func (h *Harness) isolationRequested() bool {
	if h.cfg.IsolateCommands {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv(IsolationEnv))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return IsolationDefaultOn()
}
```

Do **not** probe for the backend here with `exec.LookPath`: a missing `bwrap`
must produce `errIsolationUnavailable` (fail closed), not "off".

**Precedence:** an explicit `Config.IsolateCommands=true` still wins, so a
programmatic opt-in cannot be turned off by the environment. The override only
applies to callers that leave the bool false, which today is the production
`ollamaagent` tool harness.

**`internal/cli`**: one startup notice when the resolved value is off on a
platform with a backend, and one on an unsupported platform under B2.

**No audit-field change.** `TestRunCommandIsolationOffKeepsExecCapturedPath`
asserts an empty audit detail when isolation is off; adding a detail would change
the audit contract. The notice covers visibility.

**Test migration** (required, and the bulk of the work):

- Tests that mean "off" must set `IsolationEnv=off` rather than unset it, e.g.
  `TestRunCommandIsolationOffKeepsExecCapturedPath`.
- Tests that exercise `run_command` through `DefaultConfig()` on Darwin/Linux
  will now sandbox; each needs an explicit `IsolateCommands` decision.
- The 12 in-sandbox incompatibilities are a separate decision: migrate the test,
  accept the failure, or change policy (for example allowing AF_UNIX `bind`,
  which is a policy change and is **not** proposed here).

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
6. `make check` passes outside the sandbox; inside it reports only the documented
   incompatibility list, or those tests are migrated.
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

## 7. Migration and rollback

Migration: none for data or configuration. Operators who need today's behavior
set `SOP_AGENT_RUN_COMMAND_ISOLATION=off`. Users on hosts where `bwrap` cannot
start will see `run_command` denials with an explicit diagnostic and can opt out.

Rollback: revert `isolationRequested`/`IsolationDefaultOn` and the notice; one
release note. No state, schema, or artifact change.

## 8. Authorization requested

The operator is asked to approve exactly this:

1. Default `run_command` isolation **on** for Darwin and Linux, off elsewhere.
2. `SOP_AGENT_RUN_COMMAND_ISOLATION` gains `on`/`off` spellings, and `off` is a
   supported operator override with a startup notice.
3. Unsupported-platform behavior: **B2** (policy-only plus notice) or **B1**
   (deny) — pick one.
4. Approval to migrate the repository's own tests for default-on, without
   changing any other execution path.
5. Whether to record the remaining residual exposure in §6 as accepted, in
   particular the macOS hardlink alias (Option D is Linux-scoped) and the macOS
   parent-environment read (R4).

Until 1–5 are recorded, this proposal stays `NEEDS_HUMAN` and the default stays
opt-in.
