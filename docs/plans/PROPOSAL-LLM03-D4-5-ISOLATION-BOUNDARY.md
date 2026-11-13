# D4-5 decision packet — the `run_command` isolation boundary (OWASP gap 9, LLM03)

**Status:** R4 variants A + B are **implemented** (PR #478). The residual is still **not
accepted**, D4 enforcement stays report-only, and no baseline entry is added or suppressed.
The remaining D4-5 decision is between R1, R2 and R3 for the exposure an acknowledged
override still opens (§9). This packet records the evidence, the implemented mitigation, and
what is still open.

**Repository revision:** `origin/main` at `6efe34c8` (2026-11-13), darwin arm64, Go 1.27.1. All
measurements below were taken on that revision with the production harness and the real
sandbox backend.

**Builds on:** [PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md](PROPOSAL-RUN-COMMAND-ISOLATION-DEFAULT-ON.md)
(the approved policy), [PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md](PROPOSAL-RUN-COMMAND-RESIDUAL-RISKS.md)
(row **OVR**, and R2/R4/H2S), and
[PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md](PROPOSAL-LLM03-D4-OPERATOR-APPROVAL-PACKET.md)
(D4-5, group D).

## 1. What D4-5 is actually asking

The Semgrep finding is `exec.CommandContext(argv[0], argv[1:]...)` at
`internal/toolharness/harness.go:926`, fingerprint
`semgrep:3076a05322acb6e35b02c87144da3b8a6dfbe8eac96a7679614c4d5402973771`. It is a true
capability, not a false positive. The question is not whether that line is safe — it is
whether the **boundary around it** holds well enough to close D4-5.

The boundary has two layers: the command policy decides *which program* may run, and the
sandbox decides *what that program can reach*. §3 shows that the first layer cannot carry
the claim on its own.

## 2. Every path that can disable isolation

### 2.1 The decision function

`isolationDecision(goos, override, isolateCommands)` is the only gate:

| Platform | `IsolateCommands` | `SOP_AGENT_RUN_COMMAND_ISOLATION` | Result |
| --- | --- | --- | --- |
| darwin / linux | `true` | any value | **isolated** (`Config` wins) |
| darwin / linux | `false` | unset, `on`, `1`, `true`, `yes`, unrecognized | **isolated** (default) |
| darwin / linux | `false` | `off`, `0`, `false`, `no`, `OFF`, ` off ` | **policy-only** |
| windows / plan9 / aix | any | any value | **refused** (`run_command` never runs) |

Pinned by `TestIsolationDecisionSupportedPlatforms`,
`TestIsolationDecisionUnsupportedPlatforms` and `TestRunCommandIsolationOnWindowsRefuses`.

### 2.2 The complete inventory

| Path | Can it disable isolation? | Evidence |
| --- | --- | --- |
| `SOP_AGENT_RUN_COMMAND_ISOLATION` in SOP's environment | **Yes**, on macOS/Linux only | `isolationDecision`; `TestIsolationDecisionSupportedPlatforms` |
| `Config.IsolateCommands` | No — it only forces isolation **on** | `isolationDecision`; `DefaultConfig()` leaves it `false`, which means "defer to the environment" |
| Production callers setting `IsolateCommands` | **None** | `IsolateCommands:` appears only in a test file; production harness construction passes `CommandTimeout`, `MaxOutputBytes`, `Roots`, `CargoAgentToolEnabled`, `EvidenceSink` |
| Configuration file (`.agent-sdlc/config.yaml`) | No key exists | no `isolation` key in `internal/config` or the config file |
| CLI flag or per-task/per-call argument | None | no flag or argument reads the decision |
| Child-process environment | **It propagates.** The variable is not credential-named, so it survives `scrubbedEnvironment`, which runs only on the sandboxed path. With isolation off no scrub runs and the child inherits everything. Either way every child sees it | measured: the sandboxed/unsandboxed child printed `SOP_AGENT_RUN_COMMAND_ISOLATION=off` when set |
| Platform with no backend (Windows) | No — the override is inert and the command is refused | `isolationDecision` returns `true`; `sandboxArgv` errors |

Two consequences follow from 2.2:

- **The switch is ambient.** It is an environment variable with no committed, reviewable
  artifact. A shell profile, a wrapper script, a CI job variable, or an inherited parent
  environment can set it, and nothing in the repository records the decision.
- **It propagates.** Because the child environment keeps it, a nested SOP started from a
  model-run command inherits `off`. That is not how an operator expects a control switch to
  behave.

### 2.3 What is *not* a disable path

- The model cannot write configuration: `write_file` to `.agent-sdlc/…` is refused
  (`refusing to access SOP state`), measured.
- The model cannot change SOP's own environment: it only controls `run_command` arguments,
  and the child environment is constructed by SOP.
- The model cannot choose a non-SAFE program, so it cannot start a shell through
  `run_command` to change SOP's own environment. Code that runs *inside* a `SAFE` command can
  start further children, but it stays inside the same execution boundary (sandboxed, or
  already unsandboxed by an operator decision) and cannot change the isolation of the process
  that launched it.

## 3. What the model can execute

### 3.1 Direct execution is blocked

`checkCommandIn` runs only when `commandpolicy.Classify` returns `SAFE`. `REQUIRES_APPROVAL`
is refused, not escalated. `sh -c`, `bash -c`, `/bin/sh -c`, `python3 -c`, `go run`,
`go generate`, `git push` and `git push --force` are all denied. **The model cannot name an
arbitrary program.**

### 3.2 Execution *through* a SAFE command is not blocked

`go test`, `go test -exec`, `go build`, `go vet`, `go fmt`, `gofmt`, and (with the permission)
`cargo` are `SAFE`. `go test` compiles and runs source the model can write with `write_file`.
So the allowlist constrains the *program*, not the *code that program executes*.

### 3.3 Measured difference between the two isolation states

Same harness, same fixture: a model-authored `_test.go` that writes a marker and reads
`$HOME/.aws/credentials` (a synthetic canary in a temporary home):

| | Model code ran | Credential file read | Child saw the override |
| --- | --- | --- | --- |
| **Isolation off** (`=off`) | yes (`ran-marker`) | **yes — `D45-CANARY` returned** | `off` |
| **Isolation on** (default) | yes (`ran-marker`) | **no — `operation not permitted`** | (empty) |

This is the whole residual in one table: the sandbox does not stop execution, it contains
it. Without the sandbox the same code runs with the user's privileges, network, and
credential files. (The environment scrub from PR #474 removes credential *variables*, not
files or network.)

### 3.4 A second execution channel that does not need the override

The model can write `.git/config` (measured: `write .git/config err=<nil>`), and git executes
programs named in repository configuration. Verified harmlessly with a tracked, modified
file and `diff.external = /bin/echo`:

```
$ git diff          # with diff.external configured in the repository
f.txt …/git-blob-…/f.txt 93a9e06d… 100644 f.txt 0000000… 100644
```

That is `echo`'s argument list, not a diff: `git diff` executed the configured program. The
same mechanism is available through `core.fsmonitor` on `git status`, and through filters.
**M3 (PR #474) closed only `git -c`; repository configuration is still open.** Under
isolation this runs inside the sandbox; under the override it runs unsandboxed. It is listed
here because it is a second way to reach code execution from a `SAFE` command, and because
closing it is a separate containment decision (§7, item C).

## 4. Mandatory isolation: what it would mean

| Element | Current state |
| --- | --- |
| macOS backend | Seatbelt (`sandbox-exec`), default on, fail-closed if the binary is missing |
| Linux backend | bubblewrap (`bwrap`), default on, fail-closed if missing or if it cannot create user namespaces |
| Platform with no backend (Windows) | **Already mandatory**: every override value and both `Config` values are refused |
| Operator exception | Environment variable only, no acknowledgement, ambient and inheritable |
| Notice | One line from `sop run` and `sop prompt` when the override is honoured |

**Making isolation unconditional** (deleting the override) is coherent on macOS and Windows,
but it removes the escape hatch the operator approved for a real compatibility case: hosts
where `bwrap` cannot start (the repository already records Ubuntu 24.04's AppArmor default as
untested and fail-closed) and environments where `sandbox-exec` cannot run. On such a host
`run_command` becomes unusable, and the agent can still edit files but cannot run tests or
verification commands. That is a product regression, not a security win over the next option.

**Making isolation mandatory with an explicit exception** keeps the compatibility escape
hatch while removing *unintended* activation: the exception must be deliberate, documented,
and not inherited. That is the recommendation in §6.

## 5. Options compared

| Option | Mechanism | Closes the unsandboxed code path? | Preserves legitimate workflows? | Cost | Verdict |
| --- | --- | --- | --- | --- | --- |
| **R1** — accept the residual with an expiry | baseline entry for the finding | **No.** The override still yields unsandboxed code execution with credential-file access | Yes | Zero code; a recorded risk with a date | **Reject.** The task forbids acceptance, and an expiry does not reduce the exposure it names |
| **R2** — retain report-only enforcement | keep `security.mode: report-only` | **No.** This is D4-1, not a `run_command` control; it changes nothing about execution | Yes | Zero | **Not a D4-5 mitigation.** It can accompany any option |
| **R3** — remove `run_command` | delete the tool | Yes, by removing the capability | **No.** The agent loses test/verification execution entirely | Largest capability loss; touches prompts, tests, and workflows | **Reject as disproportionate** |
| **R4** — additional containment | gate or narrow the unsandboxed state | **Yes**, for the variants below | Yes, if chosen carefully | One predicate and tests for the recommended variant | **Recommended (§6)** |
| **Mandatory isolation, no exception** | delete the override | Yes | **No** on hosts without a working backend | Product regression on fail-closed hosts | **Reject as stated**; adopt as mandatory-with-exception |

R4 has several possible variants. They are not equivalent:

| Variant | Effect | Assessment |
| --- | --- | --- |
| **A. Confirmation-gated override** (recommended) | `off` is honoured only with a second explicit acknowledgement | Smallest change; keeps the compatibility escape hatch; stops ambient/inherited activation |
| B. Stop inheriting the control variable | nested SOP processes default back to isolation | Small, safe, complementary to A |
| C. Deny `.git/config` writes | closes the repository-config execution channel of §3.4 | Separate containment; affects legitimate `git` maintenance by the model (which has no need for it) |
| D. Refuse code-executing SAFE commands while unsandboxed | unsandboxed state becomes read-only | Makes the override useless for the compatibility case it exists for; not recommended |
| E. Config-gated switch instead of an environment variable | the exception becomes a committed, reviewable artifact | Strongest governance, but needs a tri-state config field and plumbing; larger than A |

## 6. Recommendation: R4 variant A + B

**Confirmation-gated override, plus removing the control variables from child environments.**

Rationale:

1. It targets the actual failure mode. The exposure requires isolation to be off; the current
   switch can be turned off by an ambient, inherited variable with no acknowledgement.
2. It keeps the approved compatibility escape hatch intact — an operator on a `bwrap`-less
   host can still opt out, deliberately.
3. It is small and maintainable: one predicate, one notice, one environment-filter line, and
   table-driven tests. No schema, state, or artifact change.
4. It fails in the safe direction. An unacknowledged `off` now means isolation stays **on**,
   so a stale shell profile or a leaked CI variable cannot silently weaken the boundary.

**Implemented in PR #478.** The residual is not accepted by that change, and D4 enforcement
is still report-only.

## 7. Implementation scope (implemented in PR #478)

Implemented as described below: `isolationDecision` takes the acknowledgement,
`IsolationPolicyNotice` reports an unacknowledged request, and `scrubbedEnvironment` drops
both control variables. Pinned by `TestIsolationDecisionSupportedPlatforms`,
`TestIsolationDecisionUnsupportedPlatforms`, `TestIsolationPolicyNotice`,
`TestRunCommandIsolationOffWithoutAckStaysSandboxed`, and
`TestRunCommandChildEnvironmentDropsIsolationControls`.

**A. Confirmation-gated override**

- `internal/toolharness/isolation.go`
  - add `UnsandboxedAckEnv = "SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK"`, affirmative values
    `1`, `true`, `yes` (case-insensitive, trimmed); everything else is not an acknowledgement;
  - change `isolationDecision` to take the acknowledgement and honour the off spellings only
    when it is affirmative; `Config.IsolateCommands` still forces isolation on, and a platform
    with no backend still refuses every combination;
  - `isolationRequested` passes both environment values.
- `internal/toolharness/isolation.go` — `IsolationPolicyNotice` returns
  - the existing notice when the exception is honoured;
  - a **new** notice when `off` was requested without the acknowledgement, stating that
    isolation remains in effect and naming the missing variable.
- `internal/cli/run.go`, `internal/cli/prompt.go` — no change; both already print whatever the
  notice function returns.

**B. Do not inherit the control variables**

- `internal/toolharness/isolation.go` — `scrubbedEnvironment` also drops
  `SOP_AGENT_RUN_COMMAND_ISOLATION` and `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK`, so a nested
  SOP process started by a model-run command cannot inherit an unsandboxed mode.

**Regression tests**

| Test | Asserts |
| --- | --- |
| `TestIsolationDecisionSupportedPlatforms` (extend) | every off spelling with no ack → isolated; with ack → policy-only; `IsolateCommands` still wins |
| `TestIsolationDecisionUnsupportedPlatforms` (extend) | no backend refuses every override × ack × config combination |
| `TestRunCommandIsolationOffRequiresAck` (new) | `off` alone leaves the run sandboxed (audit detail contains `isolate=`) |
| `TestRunCommandIsolationOffKeepsExecCapturedPath` (adjust) | `off` + ack keeps the current policy-only behaviour |
| `TestIsolationPolicyNotice` (extend) | the honoured notice and the unacknowledged-request notice are distinct and accurate |
| `TestRunCommandChildEnvironmentDropsIsolationControls` (new) | a child cannot see either control variable, under both isolation states |

**Out of scope for this change:** variant C (`.git/config` writes), variant E (config-gated
switch), any scanner or baseline change, and D4 enforcement. Variant C is the natural
follow-up if the operator wants the second execution channel of §3.4 closed.

## 8. Compatibility and rollback

- **Operators already using `=off`** must add the acknowledgement variable. Without it their
  commands stay sandboxed and a startup notice explains why. The change therefore cannot
  break them *unsafely*; it can surprise them, which the notice addresses.
- **Hosts without a working backend** keep the ability to opt out, by design.
- **Windows** is unchanged: already mandatory, already refusing.
- **Nested SOP invocations** stop inheriting `off`; a deliberate nested unsandboxed run must
  re-export both variables.
- **Rollback:** revert the predicate, the notice branch and the environment-filter line. No
  state, schema, or artifact changes, so no migration.

## 9. Remaining exposure after the recommendation

Even with variants A and B, the following remain, and none of them is accepted here:

- **A deliberate, acknowledged override still yields unsandboxed execution**: arbitrary code
  through `go test`, network access, and readable credential files (§3.3). That is the
  operator's explicit choice, and it is bounded by the acknowledgement, not eliminated.
- **Repository-configured git execution** (§3.4) remains available under isolation (sandboxed)
  and would be unsandboxed under an acknowledged override.
- The previously recorded sandbox residuals stay as recorded: pre-existing hardlink aliases
  (**R2**), the macOS `kern.procargs2` parent-environment read (**R4**), and host-to-sandbox
  Unix sockets (**H2S**).
- `run_command` remains refused on Windows.

## 10. Authorization required

This is a security-policy change to an approved control, so it needs explicit authorization.
The exact wording:

> I authorize R4 variants A and B in
> `docs/plans/PROPOSAL-LLM03-D4-5-ISOLATION-BOUNDARY.md`: make `run_command` isolation
> mandatory on macOS and Linux, and honour the operator override only when both
> `SOP_AGENT_RUN_COMMAND_ISOLATION=off` and `SOP_AGENT_RUN_COMMAND_UNSANDBOXED_ACK=1` are set.
> Stop inheriting either variable into model-controlled child processes, keep
> `Config.IsolateCommands` forcing isolation on, keep the no-backend refusal unchanged, and
> print a distinct notice when the override is requested without the acknowledgement. This
> authorizes the code change and its tests only. It does not accept the residual, does not
> enable D4 enforcement, and does not change any baseline entry.

If the operator prefers the exception to be a committed artifact rather than two environment
variables, the alternative is variant E (a tri-state configuration switch, reviewable in git);
it is a larger change and would need its own authorization.

To record the decision durably once made:

```bash
sop memory add --scope architecture \
  --source "operator:d4-5-isolation-boundary-<date>" \
  --decision "<the authorization text above>" \
  --reason "Close the run_command unsandboxed-execution path without accepting the residual" \
  --evidence "docs/plans/PROPOSAL-LLM03-D4-5-ISOLATION-BOUNDARY.md"
```

## 11. Evidence log

All measurements on `6efe34c8`, darwin arm64, Go 1.27.1, production harness.

| Claim | How it was established |
| --- | --- |
| Only the environment variable disables isolation | `isolationDecision`; `Config.IsolateCommands` set only in a test; no config key; no flag |
| The variable reaches children | fixture printed `iso:off` from a `go test` child under the override |
| Model code runs with the override | fixture wrote `ran-marker`; harness returned exit 0 |
| Credential file is readable only when unsandboxed | fixture read a temporary `$HOME/.aws/credentials`: `D45-CANARY` (off) vs `operation not permitted` (on) |
| The model may write `.git/config` | harness `write_file` returned no error; `.agent-sdlc/config.yaml` was refused |
| Repository config can run a program | `git diff` with `diff.external=/bin/echo` printed echo's argument list instead of a diff |
| Windows refuses regardless | `TestIsolationDecisionUnsupportedPlatforms`, `TestRunCommandIsolationOnWindowsRefuses` |
| Backends | `backendFor`: darwin→seatbelt, linux→bubblewrap, others none |

The probe code used for these measurements was temporary and is not part of this change.
