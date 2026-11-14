# Mandatory isolation for model-controlled execution — D4-5 remediation (OWASP gap 9, LLM03)

**Status:** proposal, read-only. **No policy is changed here.** The D4-5 residual is **not
accepted**, `security.mode` stays `report-only`, the baseline stays at 84 entries, and no
suppression is added. This packet supplies the evidence and the exact authorization a D4-5
change needs.

> **Correction (2026-11-13, after PR #484 merged).** The first version recommended G2
> (execution-safe git invocations). That is **incomplete**: `git status` and `git diff` run a
> configured **clean filter**, and no git flag disables filters. The #484 review raised this as
> a P1 and it was reproduced here on Git 2.50.1. The recommendation is now **M1 + G3**
> (sandbox every model-reachable git subprocess), with G4 as defense in depth. §3.4, §4–§9 and
> §11 are corrected; the original reasoning is kept so the change is auditable.

**Verified at:** `origin/main` `0344c7cf` (2026-11-13; the first analysis ran against
`f7e06355`), darwin arm64, Go 1.27.1, Git 2.50.1, pinned Gitleaks 8.30.1, Semgrep 1.183.0,
OSV-Scanner 2.7.0.

**Current scan:** 101 findings, 84 suppressed, 17 unsuppressed, **1 blocking** — the
`run_command` residual at `internal/toolharness/harness.go:926`
(`semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`).

## 1. What this packet answers

D4-5 asked whether to accept the acknowledged-unsandboxed-execution residual. The prior
analysis assumed the exposure required an operator to disable isolation. **That assumption is
wrong**, and this packet corrects it: there is a second, model-controlled, unsandboxed
execution path that needs no operator action at all.

Two questions follow:

1. Can SOP make sandboxing mandatory for model-controlled execution?
2. Can it do so without breaking operator-controlled workflows and without a second policy
   engine?

## 2. Execution-boundary trace

### 2.1 Model-reachable paths

```
model tool call
│
├── run_command ──► SplitCommand ──► selectRoot ──► checkCommandIn   (SAFE only)
│                   └──► isolationRequested()?
│                        ├── true  ──► isolate() ──► execCaptured   [sandboxed]
│                        └── false ──► execCaptured                 [UNSANDBOXED]
│
├── git_status ──► h.git("status", "--short", "--branch") ──► h.exec ──► execCaptured
├── git_diff   ──► h.git("diff")                          ──► h.exec ──► execCaptured
│                   (no isolate; `cmd.Env = nil` → SOP's full environment)
│
└── read/write/create/delete/restore/copy/rewrite/list/search  (no exec)
```

### 2.2 SOP-initiated git calls (every turn)

```
WorkingTreeFingerprint ──► execFingerprintCmd("status", "--porcelain")
                       └─► execFingerprintCmd("diff", "HEAD", "--", ...)
ChangedSince           ──► execFingerprintCmd(...)
WorkingTreeChanged     ──► exec.CommandContext(ctx, "git", "status", "--porcelain")
```

All unsandboxed, all inheriting SOP's environment. Their argv is fixed by SOP, but **git
executes programs named in repository configuration**, and the model can write that
configuration.

### 2.3 Operator-configured paths (not model-controlled)

Validation commands (`internal/testrunner`, `sh -c`), scanners (`internal/security`), and the
agent/decision/RAG command adapters. They are configured by the operator, run outside the tool
loop, and should keep their current behavior.

### 2.4 Classification is not a containment boundary

`commandpolicy` returns `SAFE` only for read-only git subcommands and `go test|build|vet|fmt|
list` (plus `gofmt`, and `cargo` under a permission). `REQUIRES_APPROVAL` and `DENIED` are
refused by the harness. That blocks *program selection*, not *code execution*: `go test` runs
model-authored source, and the git tools run git, which runs repository-configured programs.
`go generate` and `go run` are `REQUIRES_APPROVAL` and cannot be reached.

## 3. Confirmed bypasses

### 3.1 The override path (known)

`SOP_AGENT_RUN_COMMAND_ISOLATION` plus `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK` makes
`runCommandWithStatus` skip `isolate()`. Measured previously: model-authored test code then
read a credential file that the sandbox denies.

### 3.2 The git-tool path (new, and it needs no override)

Measured on `f7e06355` with the production harness:

1. The model calls `write_file` with path `.git/config` — **allowed** (`err=<nil>`; the state
   directory is refused, `.git` is not).
2. That config sets `[diff] external = ./probe.sh`.
3. The model calls `git_diff`. Output was `exit 0` with no diff: git executed the program.
4. `probe.sh` read `SOP_PROBE_CANARY` from the environment and recorded
   **`canary-value-42`**.

So a `SAFE`, fixed-argv tool became arbitrary code execution with SOP's full environment,
unsandboxed, with **no operator override and no acknowledgement**. The same vector exists on
`git status` through `core.fsmonitor`, and on every SOP-initiated fingerprint call.

The earlier D4-5 packet recorded this as "M3 covers `git -c` only"; it did not record that the
repository-configuration path is reachable in the default configuration.

### 3.3 Sandboxed model code can also write git metadata

Measured: a sandboxed `go test` fixture wrote `.git/config` successfully (`write allowed`, and
the override was present afterwards). The sandbox permits writes anywhere under the root
except `.agent-sdlc`, so a tool-level write deny (G4) cannot be the only control: the
execution surface must be neutralised or contained.

### 3.4 Clean filters execute from `status` and `diff` (measured)

A repository can define `filter.<driver>.clean` and select it with a `filter=` attribute.
Git runs the clean command whenever it reads a working-tree file to compare it, so both
`git status` and `git diff` execute it, with SOP's environment, unsandboxed.

Measured on Git 2.50.1 with a probe script that appends to a marker:

| Command | Unhardened | With `--no-ext-diff --no-textconv -c core.fsmonitor=false` |
| --- | --- | --- |
| `git status --porcelain` | filter executed | **filter still executed** |
| `git diff` | filter executed | **filter still executed** |

Git exposes no global switch to disable filter drivers: the driver name is repository-chosen,
so `-c filter.<name>.clean=` cannot be applied in advance. This is why G2 is not a boundary.

## 4. Options compared

| Option | Mechanism | Closes model-controlled unsandboxed execution? | Compatibility | Verdict |
| --- | --- | --- | --- | --- |
| **M1** | sandbox every `run_command`, ignore the override; refuse if no backend | Yes, for `run_command` | Hosts without a backend lose `run_command` (already true without the override) | **Recommended** |
| **M2** | keep the override but only for non-model paths | Same effect today, because `run_command` is the override's only consumer | Same as M1 | Equivalent; adopt as the design rule |
| **M3** | remove `run_command` where sandboxing is unavailable | Yes, by removing the feature | Largest capability loss | Not needed: M1 already fails closed |
| **R1** | accept the residual with an expiry | **No** | None | Rejected: leaves a default-path bypass |
| **R2** | keep report-only | No | None | Not a mitigation; remains the fallback |

M1 alone is **not sufficient** because of §3.2 and §3.4. The git surface needs containment:

| Vector | Unhardened | With the G2 flags |
| --- | --- | --- |
| external diff driver (`diff.external`) | executes | **blocked** |
| diff driver command (`diff.<driver>.command`) | executes | **blocked** |
| textconv (`diff.<driver>.textconv`) | executes | **blocked** |
| `GIT_EXTERNAL_DIFF` in the inherited environment | executes | **blocked** |
| fsmonitor (`core.fsmonitor`) on `status` | executes | **blocked** |
| **clean filter (`filter.<driver>.clean`)** | executes | **NOT blocked** |
| smudge filter (`filter.<driver>.smudge`) | not invoked by `status`/`diff` | not invoked |
| pager (`core.pager`, `GIT_PAGER`) | not invoked for captured (non-tty) output | not invoked |
| commit hooks on `status`/`diff` | not invoked | not invoked |
| alias replacing a builtin | not applied | not applied |

| Option | Mechanism | Completeness | Compatibility |
| --- | --- | --- | --- |
| **G2** | disable known execution mechanisms per invocation | **Incomplete**: clean filters remain, and the list is version-dependent | No backend needed |
| **G3** | run every model-reachable git subprocess inside the sandbox | **Complete by containment**: filters, drivers, and future mechanisms all run contained, with the scrubbed sandbox environment | Git tools fail closed wherever no backend exists |
| **G4** | deny model writes under `.git` | Removes the injection source from the file tools | Not sufficient alone: model code can write `.git/config` inside the sandbox (measured) |

**Recommendation: M1 + G3, with G4 as defense in depth.** M1 closes the `run_command` override
path; G3 closes the git path by containing whatever git executes; G4 keeps the model from
polluting git metadata through the file tools.

G2's flags can ride along inside the sandbox as defense in depth, but they are not the boundary:
the #484 review and the §3.4 measurement show a clean filter executes through them on both
Git 2.43.0 and 2.50.1. A boundary that depends on enumerating another project's execution
vectors will be invalidated by a future release.

The cost of G3 is explicit: `git_status` and `git_diff` fail closed where no sandbox backend
exists (Windows, and Linux hosts where `bwrap` cannot start). That is the trade for a boundary
that does not depend on git's flag semantics.

## 5. Recommended architecture

```
model tool call
├── run_command ──► … ──► isolate() ──► execCaptured     (always; refuse without backend)
└── git_status / git_diff ──► isolate() ──► execCaptured  (always; refuse without backend)

SOP-initiated git (fingerprints) ──► isolate()           (same decision)
operator-configured execution ──► unchanged, outside the tool loop
```

One policy, not two: `isolationDecision`/`sandboxArgv` stays the only sandbox decision and
returns "sandbox" for every model-reachable command. There is no second classifier, no flag
allowlist, and no per-vector maintenance.

The sandbox already provides the second half of the mitigation: an isolated child gets
`sandboxEnv`, which drops credential-named and credential-shaped variables. G3 therefore
removes both the code-execution boundary failure and the environment exposure that made the
exploit serious.

## 6. Implementation scope

| File | Change |
| --- | --- |
| `internal/toolharness/isolation.go` | Make the sandbox mandatory: `isolationRequested` returns true (or `isolationDecision` loses the override parameter). Delete `IsolationEnv`, `UnsandboxedAckEnv`, `isOffValue`, `isAcknowledged`, `isolationOverrideOff`, `IsolationPolicyNotice`, and the `controlEnvNames` entries they fed. Keep `Config.IsolateCommands` as a deprecated always-on no-op, or remove it with its tests. |
| `internal/toolharness/harness.go` | Route every model-reachable exec through `isolate()`: `runCommandWithStatus` (already), plus `git`, `execFingerprintCmd`, and `WorkingTreeChanged`. The simplest shape is one `execIsolated(ctx, dir, argv)` helper that isolates, runs, and cleans up, used by all four sites. Remove the per-vector hardening helper if G3 lands. |
| `internal/toolharness/files.go` | Deny write tools under `.git/` (G4), mirroring the state-directory rule; the message should say git metadata is read-only to the model. |
| `internal/cli/run.go`, `internal/cli/prompt.go` | Remove the notice call sites. |
| Docs | `PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md`, `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` (OVR row), the D4-5 packet, and the decision record. |

Smallest viable variant: keep the constants and the notice, make `isolationRequested` return
true, and have the notice say the override no longer applies. That is a one-line policy change
plus the isolated-exec routing, at the cost of dead configuration.

## 7. Compatibility and operational impact

| Area | Impact |
| --- | --- |
| macOS Seatbelt | Sandboxed `git status` and `git diff` were measured working here (index writes stay inside the root). |
| Linux bubblewrap | Feasibility checked in a Debian bookworm container (Git 2.39.5): the production-style argument set runs `git status` and `git diff` successfully. Requires unprivileged user namespaces on the host; where `bwrap` cannot start (Ubuntu 24.04 AppArmor default), git tools and `run_command` fail closed. Host containment (filter and environment) not measured. |
| **Windows** | No backend exists, so `git_status`/`git_diff` are now **refused** (they work today). This is the main compatibility regression and the reason G2 was attractive. It is unavoidable if the boundary is containment. |
| Local LLM / Ollama | Unaffected: providers, not executors. |
| Build/test commands | `go test`/`go build` keep running model-authored code inside the sandbox; throwaway `GOCACHE` and scrubbed environment unchanged. |
| Tool approval | `commandpolicy` is untouched, so `SAFE`/`REQUIRES_APPROVAL`/`DENIED` semantics are unchanged. |
| Operator override semantics | The override stops applying to model-controlled execution; operators who relied on it need a host where the backend works. |
| CI and quality stage | CI does not run SOP's agent tools, so no CI change. The quality stage is unaffected. |
| Configuration | No new config key. Removing the override deprecates two environment variables. |
| Git tooling | `diff.external`, textconv, fsmonitor, and filters no longer influence SOP's git output because they cannot run. Operators see plain diffs in tool results; their own shell is unaffected. |
| Performance | One sandbox exec per git call: two tools plus two to three fingerprint calls per turn. Measured git calls complete in the low milliseconds on this host. |

## 8. Verification design

| Test | Asserts |
| --- | --- |
| `TestModelCommandSandboxedDespiteOverrides` | with both variables set and `Config.IsolateCommands` false, `run_command` still audits `isolate=`; fails on the current code |
| `TestModelCommandRefusedWithoutBackend` | no backend refuses even with both variables |
| `TestSafeBuildAndTestCannotBypassIsolation` | `go test`, `go build`, `go vet` runs carry `isolate=` |
| `TestGitToolsAreSandboxed` | `git_status` and `git_diff` audits carry `isolate=`; fails on the current code |
| `TestGitToolsRefusedWithoutBackend` | no backend refuses both git tools |
| `TestGitStatusIgnoresRepositoryFsmonitor` | `core.fsmonitor` cannot reach the host (runs contained or not at all) |
| `TestGitDiffIgnoresExternalDiff` | `diff.external` and `GIT_EXTERNAL_DIFF` cannot reach the host |
| `TestGitDiffIgnoresTextconv` | `diff.<driver>.textconv` cannot reach the host |
| **`TestGitToolsIgnoreCleanFilters`** | a `filter.<driver>.clean` cannot reach the host on `status` or `diff`; the P1 regression |
| `TestSandboxedGitFilterCannotReadCredentialsOrEnv` | a filter that does run sees no credential-named environment variable and cannot read `$HOME/.ssh` (measured: `env=` empty, `ssh=DENIED`) |
| `TestGitMetadataIsReadOnlyToModelTools` | `write_file`/`create_file` under `.git/` are refused (G4) |
| `TestCredentialScrubbingRemainsActive` | the existing scrub tests stay green under mandatory isolation |
| `TestCommandPolicyClassesUnchanged` | the existing classifier table is unchanged |
| `TestOperatorConfiguredExecutionStillRuns` | `internal/testrunner` and the scanner adapters still execute operator commands unsandboxed, as designed |

Positive controls: each git test must fail on `0344c7cf` (the merged proposal's base) — the
canary must reach the host without the change. Containment tests assert the canary does **not**
reach the host, not merely that git exits non-zero.

Acceptance criteria: every model-reachable exec is sandboxed; the git tests fail on the
current code; `test-fast` and the scanner integration job stay green; the scan keeps 1 blocking
finding because remediation does not remove the `exec.CommandContext` call.

**The Semgrep finding does not disappear.** Containment mitigates behavior, not the call site.
It must remain visible and must not be suppressed to make a scan pass.

## 9. Remaining exposure after M1 + G3 + G4

- The sandbox still does not prevent every disclosure: pre-existing hardlink aliases (R2), the
  macOS `kern.procargs2` parent-environment read (R4), host-to-sandbox Unix sockets (H2S), and
  credentials outside the deny list.
- Model-authored code, including repository-configured git programs, still runs, but inside the
  sandbox with a scrubbed environment.
- Operator-configured executors (validation commands, scanners) remain unsandboxed by design.
- G4 only covers the file tools; sandboxed model code can still write `.git/config` (measured).
  Under G3 that config is inert for host-level execution but visible to the operator's later
  git use.
- G3 depends on backend availability: on Windows and on Linux hosts without working user
  namespaces, git tools and `run_command` are unavailable.
- Platform coverage: macOS measured for the bypass, the G2 gaps, and G3 containment; Linux
  bubblewrap git feasibility checked in a container but containment not measured; Windows
  untested (no backend exists, so the behaviour is refusal by construction).

## 10. Relationship to D4-5 and Option C

- Today (`D4-1 = A`, report-only) nothing changes in the scan; the residual is unaccepted and
  D4-5 remains open.
- After M1 + G3 the D4-5 question changes shape: the *unsandboxed* path is closed, so R1
  ("accept the residual") becomes moot. What remains is a **sandboxed capability**, and the
  Semgrep finding is a true positive about a call site whose risk is controlled structurally.
- For Option C a Group D fingerprint entry would still be needed for the gate to pass, but its
  justification becomes "mandatory isolation + sandboxed git", not "accepted residual". That is
  a new authorization and is not implied by this packet.

## 11. Authorization required

> I authorize M1 + G3 + G4 as specified in
> `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md`: every model-reachable execution —
> `run_command`, `git_status`, `git_diff`, and the per-turn git fingerprint calls — executes
> inside a supported sandbox on macOS and Linux and is refused when no backend exists; the
> `SOP_AGENT_RUN_COMMAND_ISOLATION` override and `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK`
> acknowledgement no longer disable it, and their notice and constants may be removed; model
> write tools may not modify paths under `.git`. This supersedes the R4 A+B override
> authorization. It does not accept the D4-5 residual, does not add a baseline entry, does not
> enable Option C, and does not change `security.mode` or `required_categories`.

This accepts the compatibility trade stated in §7: `git_status` and `git_diff` become
unavailable on Windows and on hosts without a working sandbox backend.

If the operator is unwilling to accept that trade, the alternative is **not** G2 — the §3.4
measurement shows it is not a boundary. The remaining options are R2 (stay report-only with the
bypass present) or removing the git tools on unsupported platforms.

## What this packet does not do

- Changes no code, configuration, baseline, enforcement, or decision memory.
- Records no residual acceptance and no new baseline entry.
- Modifies no historical decision; it records that the earlier residual analysis assumed the
  override was the only unsandboxed path.
