# Proposal: post-default-on residual risks (OWASP Gap 13 / LLM02)

**Status: R2 and R4 are accepted within bounded scopes; H2S is recorded as a
bounded architectural property and is *not* a risk acceptance.** Default-on
isolation is implemented and enforced; the exposures it does not close are listed
with the evidence for each.

- **R2** is **accepted as bounded limits** (LLM02 decision packet C.4, operator
  disposition in chat, 2026-11-12), on top of the gap 5 option 1 record.
- **R4** is **accepted as a known limit** (same C.4 disposition), on top of the
  gap 6 record. None of the measured mitigation designs is implemented.
- **H2S** is recorded under Option A as a bounded architectural property, not as a
  formal residual-risk acceptance (decision memory
  `a73749c2c7fc634dc0e37b10f6b0d5f8e7e7ae8046500e9b71792ed1cc54d719`, source
  `operator:h2s-option-a-2026-11-12`).

## Tracking matrix

Owner is the accountable party: the operator holds risk decisions; the repository
owns the controls. `ACCEPTED` means the operator accepted the exposure within the
stated scope. `RECORDED` means the exposure is written down and is **not** a risk
acceptance.

| ID | Exposure | Owner | Status | Decision record | Evidence |
| --- | --- | --- | --- | --- | --- |
| **R2** | Pre-existing hardlink alias to a credential | Operator (risk); `internal/toolharness/identity.go` (control) | `ACCEPTED (BOUNDED)` | Gap 5 option 1 record, 2026-11-12; accepted as bounded limits in LLM02 packet C.4 (chat, 2026-11-12); Option D `RISK_ACCEPTED — SUBPROCESS HARDLINK ALIAS (BOUNDED)` for the Linux subprocess case | `TestLinuxIsolationPreExistingCredentialHardlink`, `TestDarwinIsolationSAFECommandCredentialRead`, `TestRunCommandIsolationDeniesCredentialHardlink` |
| **R4** | macOS parent launch-time environment (`kern.procargs2`) | Operator (risk); `internal/toolharness/isolation.go` (control) | `ACCEPTED (KNOWN LIMIT)` | Gap 6 record, PR #447; accepted as a known limit in LLM02 packet C.4 (chat, 2026-11-12) | `seatbelt-evidence.md` launch-chain options; measurement report |
| **H2S** | Host-to-sandbox Unix socket connectivity | Operator (risk); `internal/toolharness/isolation.go` profile rule | `RECORDED` — bounded architectural property, **not** a risk acceptance | H2S Option A, 2026-11-12, decision memory `a73749c2…`, source `operator:h2s-option-a-2026-11-12` | Measured on both backends; reverse direction pinned by `TestRunCommandIsolationAllowsOwnUnixSocketOnly` |
| **OVR** | Authorized operator `off` override restores policy-only execution on macOS/Linux | Operator (explicit action) | `AUTHORIZED BY POLICY` — exposure tracked here so it is never read as enforced isolation | Default-on approval, PR #441 §8 | `TestIsolationPolicyNotice`, `TestRunCommandIsolationOffKeepsExecCapturedPath`; `sop run` / `sop prompt` notice |
| **VC** | Verification coverage gaps | SOP maintainers | `PARTIAL` — Linux host and Linux `amd64` are `NOT_RUN` | none needed (evidence gap, not a risk decision) | Platform table below |
| **OPS** | Operational limits: 120 s default `CommandTimeout` against a ~90 s in-sandbox suite; 20 000-entry `$HOME` scan bound that refuses the command | SOP maintainers | `DOCUMENTED` — not a risk decision | none | Measurement report |

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

**Acceptance (2026-11-12).** The operator accepted these as **bounded limits**
(LLM02 decision packet C.4, operator disposition in chat). Option C Phase 1 is
kept and `Nlink > 1` stays rejected. No operator decision is open for R2. The macOS
in-sandbox alias measured in this verification is the same inode-alias limit, not a
new residual.

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

**Acceptance (2026-11-12).** The operator accepted this as a **known limit** (LLM02
decision packet C.4, operator disposition in chat). The sandbox is not weakened and
no mitigation design is implemented. Authorizing one of the designs (SOP
environment scrub, or launchd reparenting) would remain a separate change.

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

**Options.**

- **Option A - retain the scoped `network-bind` rule.** Record host-to-sandbox
  connectivity as a bounded architectural property. This does **not** accept a
  new credential-disclosure risk; it notes that the direction exists because a
  sandboxed build must be able to listen, which Linux already allows.
- **Option B - remove the `(allow network-bind (subpath "<writable dir>"))` rule.**
  Restores the pre-#437 behaviour, in which macOS denies AF_UNIX bind entirely.
  Legitimate workflows that would stop working: any sandboxed test or build that
  binds its own Unix socket - the repository's own `TestHomeSocketsAreMasked` and
  `TestHomeSymlinkToDirectoryIsFollowed`, local caches and dev servers that use a
  Unix socket, and the unix-socket cases in `dqharness`/`evaltarget` boundary
  suites. Those tests would have to become documented skips, and users on macOS
  would lose a capability Linux keeps.

**Recommendation: Option A.** The exposure is not model-reachable (a model can only
run sandboxed commands and cannot create a same-UID host process), the socket can
only exist where the sandbox may already write, and Option B trades that for a
documented coverage loss on real workflows while making macOS stricter than Linux
for no security gain in the credential-disclosure direction. Option A should be
recorded as a bounded property, not as an accepted risk.

**Operational consequences.**

- *Option A:* no behaviour change; macOS and Linux keep parity for build/test
  workflows; the property is written down and stays visible.
- *Option B:* two toolharness tests and the unix-socket boundary cases become
  documented skips; macOS sandboxed builds that bind a Unix socket fail; the
  profile diverges from Linux.

**Exact approval wording (Option A).**

> I approve Option A for the H2S residual: retain the scoped `(allow network-bind
> (subpath "<writable dir>"))` rule and record host-to-sandbox Unix-socket
> connectivity as a bounded architectural property of the `run_command` sandbox.
> This is not a risk acceptance. Outbound AF_UNIX and non-loopback IP access stay
> denied, and sandbox-to-host socket access stays denied.

**Disposition (recorded 2026-11-12).** The operator approved **Option A**. The
decision is recorded in decision memory
(`a73749c2c7fc634dc0e37b10f6b0d5f8e7e7ae8046500e9b71792ed1cc54d719`, source
`operator:h2s-option-a-2026-11-12`, scope `architecture`):

> Retain the scoped AF_UNIX `network-bind` rule. Host-to-sandbox Unix-socket
> connectivity is recorded as a bounded architectural property, not as a formal
> residual-risk acceptance. Sandbox-to-host connections, outbound AF_UNIX and
> non-loopback IP access stay denied.

The Seatbelt rule is unchanged, no production isolation behaviour changed, and the
regression tests that keep the reverse direction closed stay in place. To record a
different disposition later, the supported command is:

```bash
sop memory add --scope architecture \
  --source "operator:<disposition>-<date>" \
  --decision "<the decision text>" \
  --reason "<the rationale>" \
  --evidence "docs/plans/PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md#h2s---host-to-sandbox-unix-socket-direction"
```

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
