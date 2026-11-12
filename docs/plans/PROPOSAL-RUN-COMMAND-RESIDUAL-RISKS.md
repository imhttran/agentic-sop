# Proposal: post-default-on residual risks (OWASP Gap 13 / LLM02)

**Status: R2 and R4 have recorded dispositions; H2S is `NEEDS_HUMAN`.** Nothing
here accepts, waives, transfers, or closes a risk. Default-on isolation is
implemented and enforced; the exposures it does not close are listed with the
evidence for each.

- **R2** has a recorded disposition: bounded residual exposure, gap 5 option 1,
  2026-11-12. It is *not* accepted, and no separate control is authorized.
- **R4** has a recorded disposition: known limit for the opt-in sandbox (gap 6,
  PR #447). It is *not* accepted, and none of the measured options is implemented.
- **H2S** has no recorded decision.

Related:
[PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md)
(the approved policy), [llm02-decision-packet.md](../reports/owasp-llm/llm02-decision-packet.md)
(residual rows R1-R8), and
[llm06-sandbox-default-measurements.md](../reports/owasp-llm/llm06-sandbox-default-measurements.md)
(the measurements).

## What default-on changed

With isolation enforced, the **direct** credential-read vector through SAFE
`run_command` is denied by default: a sandboxed command cannot read `$HOME`
credential paths, follow a symlink alias to them, reach a credential path named by
an environment override, or reach the macOS keychain. That is LLM02 R3's
direct-read claim, and it holds under enforcement.

Two things are explicitly **not** claimed by that:

1. The operator override `SOP_AGENT_RUN_COMMAND_ISOLATION=off` restores
   policy-only execution on macOS and Linux, by approval. Under it, the
   direct-read vector is open again; the startup notice makes that visible.
2. Three exposures survive **even with the sandbox on**. They are assessed below.

## The three residuals

### R2 - Pre-existing hardlink alias to a credential

**Threat scenario.** A credential file and an ordinary-named file share an inode.
Every sandbox credential deny is path-based, so the alias path is allowed and its
content is readable. A model-written test can read the alias, or copy it into the
repository, and the value reaches the model through ordinary command output.

**Existing controls.**

- `internal/toolharness/identity.go` (Option C Phase 1) denies file-tool access
  to aliases of protected **workspace** credentials on Linux (`VERIFIED`) and
  macOS (`VERIFIED`, attempt 3).
- The `run_command` sandbox denies credential paths and symlinks, and refuses a
  new `ln` of a protected path from inside the sandbox.
- Credential-named environment variables are scrubbed from the child.

**Current evidence.**

- `TestLinuxIsolationPreExistingCredentialHardlink` logs the residual: an alias
  created before the run is readable inside the bubblewrap sandbox.
- `TestDarwinIsolationSAFECommandCredentialRead` pins the same on macOS through
  the SAFE `go test` path.
- `TestRunCommandIsolationDeniesCredentialHardlink` pins that a *new* link is
  refused inside the sandbox.

**Remaining exposure.**

- An alias whose origin is **outside** the workspace identity registry.
- An alias created **before** the registry snapshot, or a credential created
  **after** it by a process outside the model tools.
- An origin **removed externally** before the first build.
- **Windows**: no sandbox at all, and identity verification is `UNVERIFIED`.
- `Nlink > 1` is not implemented; Option C Phase 1 covers workspace origins only.

**Disposition (recorded 2026-11-12).** The operator chose option 1 of
[PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md](PROPOSAL-LLM02-NLINK-AND-MACOS-IDENTITY.md):
Option B (macOS verification) is complete, Option A (`Nlink > 1`) is **not
approved**, and the outside-workspace, late-credential, external-removal, and
Windows cases are recorded as bounded residual exposures with no separate control
authorized. They are bounded limits, **not** an acceptance. The subprocess
hardlink alias is separately `RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)`
under Option D (Linux; [PROPOSAL-LLM02-SUBPROCESS-HARDLINK.md](PROPOSAL-LLM02-SUBPROCESS-HARDLINK.md)).

**Operator decision.** None open for R2. The macOS in-sandbox alias measured in
this verification is the same inode-alias limit, not a new residual. Naming it
explicitly next to the Linux Option D entry would be a wording change to the
Option D document, not a new decision.

### R4 - macOS parent launch-time environment (`kern.procargs2`)

**Threat scenario.** A sandboxed command walks its ancestors with
`kern.procargs2` and reads the environment SOP was launched with. On macOS that
environment is on the process's initial stack, so it is readable for the whole
run even though the child's own environment was scrubbed.

**Existing controls.**

- `sandboxEnv` drops credential-named and credential-shaped variables from the
  **child's** environment.
- The sandbox denies outbound network and credential file reads, so a leaked
  value cannot be exfiltrated over the network from inside the sandbox.
- Linux is closed by `--unshare-pid`: `/proc` is a fresh namespace, so ancestors
  are not visible.

**Current evidence.**

- [seatbelt-evidence.md](../reports/security-shift-left/seatbelt-evidence.md#parent-environment-via-kernprocargs2-narrow-deny-attempts-owasp-gap-6):
  narrow `sysctl` denies do not work; a full `(deny sysctl-read)` aborts the Go
  runtime.
- Re-confirmed with a synthetic canary in three ancestor processes
  ([llm06-sandbox-default-measurements.md](../reports/owasp-llm/llm06-sandbox-default-measurements.md)).

**Remaining exposure.** Any long-lived credential SOP itself receives through the
environment at launch. Unsetting the variable after start does **not** help:
`kern.procargs2` reads the launch-time stack, not the current environment
(measured).

**Options measured (gap 6, PR #447).**

- An **intermediate scrubbed shell** hides the direct parent, but the sandboxed
  process still reaches the SOP process above it through `kern.proc.pid` and
  `kern.procargs2`. It does not close the exposure.
- A **narrow sysctl deny** has no effect; a full `(deny sysctl-read)` aborts the Go
  runtime.
- **Scrubbing SOP's own environment** is a process-wide change, not a contained
  sandbox change, and is untested. Unsetting after start does not help either,
  because `kern.procargs2` reads the launch-time stack (measured).
- **Reparenting to launchd** would need the output and kill semantics rebuilt; untested.

**Disposition (recorded).** Gap 6 records this as a **known limit for the opt-in
sandbox**, and default-on keeps the exposure. That is a recorded limit, **not** an
acceptance, and no option is implemented.

**Operational guidance (available now, not enforced).** Do not put long-lived
secrets in SOP's environment; supply them through a file, config, or keychain that
the relevant component reads.

**Operator decision.** Formally accept the known limit, or authorize one of the
unimplemented designs (SOP environment scrub, or launchd reparenting) as its own
change.

### H2S - Host-to-sandbox Unix socket direction

**Direction matters.** This is **host -> sandbox**. The reverse - a sandboxed
command connecting to a host socket - remains **denied**, and is pinned by
`TestRunCommandIsolationAllowsOwnUnixSocketOnly`. The two directions must not be
conflated.

**Threat scenario.** The macOS profile allows AF_UNIX `bind` under a writable dir
so a sandboxed build can create its own listener. A same-user host process can
then dial that socket and exchange data with the sandboxed command.

**Existing controls.** The socket can only exist under a directory the sandbox may
already write to; outbound AF_UNIX and all non-loopback IP traffic stay denied; a
host socket is still unreachable from inside.

**Current evidence.** Measured in this verification on both backends: a host
Python client connected to a socket created by a sandboxed process, sent a byte,
and received the echo. macOS used the production Seatbelt rules; Linux used the
production bubblewrap arguments. There is no CI test for this direction.

**Remaining exposure.** An unauthorized **effect** would require a cooperating
same-UID host process to dial the socket and supply data. A model cannot create
such a process: it can only run sandboxed commands. A same-UID host process is
already inside the operator's trust boundary for that account. There is therefore
no practical new credential-disclosure path, and no model-reachable effect path.

**Recommended disposition.** Record as a bounded architectural property of the
`network-bind` rule, not a credential-disclosure risk. No mitigation is proposed;
reverting the rule would restore the socket incompatibility recorded in PR #437.

**Operator decision.** Confirm the property is recorded as bounded (no action), or
authorize reverting the `network-bind` rule and accepting the two socket-scan
tests as skips.

## Platform coverage behind these assessments

| Platform | Status | What it supports |
| --- | --- | --- |
| macOS arm64, real host | `VERIFIED` | default-on, override, credential/symlink/state/network denies, refusal paths |
| Linux arm64, privileged container | `VERIFIED` | same set under bubblewrap 0.12.0 |
| Linux, real host | `NOT_RUN` | no distribution host available through the established tools |
| Linux amd64 | `NOT_RUN` | no amd64 host or emulation in this environment |
| Windows, runtime | `VERIFIED` in CI | B1 refusal across every override and both `Config` values |

The Linux container results are **not** evidence of real-host compatibility; a
distribution host where `bwrap` cannot create user namespaces (for example Ubuntu
24.04's AppArmor default) is untested and would fail closed.
