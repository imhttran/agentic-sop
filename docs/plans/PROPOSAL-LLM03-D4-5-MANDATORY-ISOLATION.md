# Mandatory isolation for model-controlled execution — D4-5 remediation (OWASP gap 9, LLM03)

**Status:** proposal, read-only. **No policy is changed here.** The D4-5 residual is **not
accepted**, `security.mode` stays `report-only`, the baseline stays at 84 entries, and no
suppression is added. This packet supplies the evidence and the exact authorization a D4-5
change needs.

**Verified at:** `origin/main` `f7e06355` (2026-11-13), darwin arm64, Go 1.27.1, pinned
Gitleaks 8.30.1, Semgrep 1.183.0, OSV-Scanner 2.7.0.

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

## 4. Options compared

| Option | Mechanism | Closes model-controlled unsandboxed execution? | Compatibility | Verdict |
| --- | --- | --- | --- | --- |
| **M1** | sandbox every `run_command`, ignore the override; refuse if no backend | Yes, for `run_command` | Hosts without a backend lose `run_command` (already true without the override) | **Recommended** |
| **M2** | keep the override but only for non-model paths | Same effect today, because `run_command` is the override's only consumer | Same as M1 | Equivalent; adopt as the design rule |
| **M3** | remove `run_command` where sandboxing is unavailable | Yes, by removing the feature | Largest capability loss | Not needed: M1 already fails closed |
| **R1** | accept the residual with an expiry | **No** | None | Rejected: leaves a default-path bypass |
| **R2** | keep report-only | No | None | Not a mitigation; remains the fallback |

M1 alone is **not sufficient** because of §3.2. The git surface needs one of:

| Option | Mechanism | Completeness | Compatibility |
| --- | --- | --- | --- |
| **G2** | run every harness git invocation with execution vectors disabled (`--no-ext-diff`, `--no-textconv`, `-c core.fsmonitor=false`) | Covers external diff, textconv, and fsmonitor: the program-execution vectors reachable from `status`/`diff` | No backend needed; works on Windows and bwrap-less hosts |
| **G3** | run the git tools and internal git calls inside the sandbox | Strongest: containment rather than enumeration | Git tools fail closed wherever no backend exists — a real regression on Windows, which has working git tools today |
| **G4** | deny model writes under `.git` | Removes the injection source | Not sufficient alone: an operator-shipped config still executes, and sandboxed model code can write `.git/config` (measured) |

**Recommendation: M1 + G2, with G4 as defense in depth.** M1 closes the override path; G2
closes the repository-configuration path without a backend dependency; G4 stops the model
polluting git metadata, which also protects the operator's later git use.

G3 is the stronger boundary and the right choice if the operator accepts git tools failing
closed without a sandbox. It is not the default recommendation because it degrades Windows for
no gain over G2 on the vectors actually reachable.

## 5. Recommended architecture

```
model tool call
├── run_command ──► … ──► isolate() ──► execCaptured     (always; refuse without backend)
└── git_status / git_diff
        └──► git -c core.fsmonitor=false … --no-ext-diff --no-textconv
                        └──► execCaptured              (no program execution possible)

SOP-initiated git ──► same hardened argv
operator-configured execution ──► unchanged, outside the tool loop
```

One policy, not two: the existing `isolationDecision`/`sandboxArgv` machinery stays the only
sandbox decision and returns "sandbox" for every model-controlled command. G2 is argument
hardening in one helper, not a second classification engine.

## 6. Implementation scope

| File | Change |
| --- | --- |
| `internal/toolharness/isolation.go` | Make the sandbox mandatory: `isolationRequested` returns true (or `isolationDecision` loses the override parameter). Delete `IsolationEnv`, `UnsandboxedAckEnv`, `isOffValue`, `isAcknowledged`, `isolationOverrideOff`, `IsolationPolicyNotice`, and the `controlEnvNames` entries they fed. Keep `Config.IsolateCommands` as a deprecated always-on no-op, or remove it with its tests. |
| `internal/cli/run.go`, `internal/cli/prompt.go` | Remove the notice call sites. |
| `internal/toolharness/harness.go` | Add one `gitArgv(args ...string)` helper that prefixes `-c core.fsmonitor=false -c core.pager=cat` and adds `--no-ext-diff --no-textconv` for `diff`; use it in `git`, `execFingerprintCmd`, and `WorkingTreeChanged`. |
| `internal/toolharness/files.go` | Deny write tools under `.git/` (G4), mirroring the state-directory rule; the message should say git metadata is read-only to the model. |
| Docs | `PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md`, `PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md` (OVR row), the D4-5 packet, and the decision record. |

Smallest viable variant: keep the constants and the notice, make `isolationRequested` return
true, and have the notice say the override no longer applies. That is a one-line policy change
plus G2, at the cost of dead configuration.

## 7. Compatibility and operational impact

| Area | Impact |
| --- | --- |
| macOS Seatbelt | `run_command` unchanged. Sandboxed `git status`/`git diff` were measured working (index writes stay inside the root); G2 does not need the sandbox. |
| Linux bubblewrap | Unchanged where `bwrap` works. Where it cannot start (Ubuntu 24.04 AppArmor default), `run_command` now always fails closed; previously an operator could opt out. That is the intended trade. |
| Windows | `run_command` already refused; unchanged. **Git tools keep working** under G2, which is why G2 is preferred over G3. |
| Local LLM / Ollama | Unaffected: providers, not executors. |
| Build/test commands | `go test`/`go build` keep running model-authored code **inside** the sandbox; the throwaway `GOCACHE` and scrubbed environment are unchanged. |
| Tool approval | `commandpolicy` is untouched, so `SAFE`/`REQUIRES_APPROVAL`/`DENIED` semantics are unchanged. |
| Operator override semantics | The override stops applying to `run_command`; operators who relied on it need a host where the backend works. |
| CI and quality stage | CI does not run SOP's agent tools, so no CI change. The quality stage is unaffected. |
| Configuration | No new config key. Removing the override deprecates two environment variables, not configuration. |
| Git tooling | `diff.external`/textconv/fsmonitor stop influencing SOP's git output. Operators see plain diffs in tool results; their own shell is unaffected. |

## 8. Verification design

| Test | Asserts |
| --- | --- |
| `TestModelCommandSandboxedDespiteOverrides` | with both variables set and `Config.IsolateCommands` false, `run_command` still audits `isolate=`; fails on the current code |
| `TestModelCommandRefusedWithoutBackend` | no backend refuses even with both variables (extend the existing decision table) |
| `TestSafeBuildAndTestCannotBypassIsolation` | `go test`, `go build`, and `go vet` runs carry `isolate=` |
| `TestGitToolsIgnoreRepositoryExternalCommands` | a `.git/config` with `diff.external` does not execute; `git_diff` returns a normal diff |
| `TestGitStatusIgnoresRepositoryFsmonitor` | `core.fsmonitor` does not execute |
| `TestGitMetadataIsReadOnlyToModelTools` | `write_file`/`create_file` under `.git/` are refused; git metadata stays readable |
| `TestCredentialScrubbingRemainsActive` | the existing scrub tests stay green under mandatory isolation |
| `TestNestedSOPDoesNotInheritOverrides` | the child environment carries neither variable (today's behavior, kept) |
| `TestCommandPolicyClassesUnchanged` | the existing classifier table is unchanged |
| `TestOperatorConfiguredExecutionStillRuns` | `internal/testrunner` and the scanner adapters still execute operator commands unsandboxed, as designed |

Acceptance criteria: every model-reachable exec is either sandboxed or provably free of
command execution; the new git tests fail on `f7e06355`; `test-fast` and the scanner
integration job stay green; the scan keeps 1 blocking finding because remediation does not
remove the `exec.CommandContext` call.

**The Semgrep finding does not disappear.** Mandatory isolation mitigates behavior, not the
call site. The finding must remain visible and must not be suppressed to make a scan pass.

## 9. Remaining exposure after M1 + G2 + G4

- The sandbox still does not prevent every disclosure: pre-existing hardlink aliases (R2), the
  macOS `kern.procargs2` parent-environment read (R4), host-to-sandbox Unix sockets (H2S), and
  credentials outside the deny list.
- Model-authored code still runs sandboxed with repository read access and a writable root.
- Operator-configured executors (validation commands, scanners) remain unsandboxed by design.
- An operator-supplied `diff.external` is neutralized for SOP's git calls by G2 but still runs
  in the operator's shell.
- Git itself runs unsandboxed for the git tools under G2; its execution surface is restricted
  by flags, not containment. That is the residual if G2 misses a vector.
- Model code can still write `.git/config` (measured) unless the sandbox itself denies it; G2
  makes such config inert for SOP's git calls, but the operator's own later git use would see
  it. G4 only covers the file tools.

## 10. Relationship to D4-5 and Option C

- Today (`D4-1 = A`, report-only) nothing changes in the scan; the residual is unaccepted and
  D4-5 remains open.
- After M1 + G2 the D4-5 question changes shape: the *unsandboxed* path is closed, so R1
  ("accept the residual") becomes moot. What remains is a **sandboxed capability**, and the
  Semgrep finding is a true positive about a call site whose risk is controlled structurally.
- For Option C a Group D fingerprint entry would still be needed for the gate to pass, but its
  justification becomes "mandatory isolation + hardened git", not "accepted residual". That is
  a new authorization and is not implied by this packet.

## 11. Authorization required

> I authorize M1 + G2 + G4 as specified in
> `docs/plans/PROPOSAL-LLM03-D4-5-MANDATORY-ISOLATION.md`: `run_command` always executes inside
> a supported sandbox on macOS and Linux and is refused when no backend exists; the
> `SOP_AGENT_RUN_COMMAND_ISOLATION` override and `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK`
> acknowledgement no longer disable it, and their notice and constants may be removed; every
> harness git invocation disables external diff, textconv, fsmonitor, and pager execution; and
> model write tools may not modify paths under `.git`. This supersedes the R4 A+B override
> authorization. It does not accept the D4-5 residual, does not add a baseline entry, does not
> enable Option C, and does not change `security.mode` or `required_categories`.

If the operator prefers containment over enumeration for git, authorize **G3** instead of G2
and accept that `git_status`/`git_diff` fail closed wherever no sandbox backend exists,
including Windows.

## What this packet does not do

- Changes no code, configuration, baseline, enforcement, or decision memory.
- Records no residual acceptance and no new baseline entry.
- Modifies no historical decision; it records that the earlier residual analysis assumed the
  override was the only unsandboxed path.
