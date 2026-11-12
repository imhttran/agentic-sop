# `run_command` residual analysis — `internal/toolharness/harness.go:920`

**Status:** investigation, read-only. **The residual is not accepted, and no code or
configuration is changed by this report.** It answers the operator question behind D4-5: do
the command authorization, the default-on sandbox, and the tool policy sufficiently
constrain model-controlled execution?

**Evidence base:** clean `origin/main` at `bd2e0d45`, darwin arm64, Go 1.27.1, Gitleaks
8.30.1, Semgrep 1.183.0. The isolation test suite in `internal/toolharness` was run on this
host; all 18 tests pass (§5).

**Answer in one line.** The model cannot name an arbitrary *program*, but it can run
arbitrary *code*, because `go test` and `go build` are `SAFE` and execute source the model
can write. The control that bounds that code is the sandbox, not the command policy. With
isolation on the result is contained; with isolation off the same code runs with the parent
environment, network, and credential files. **The residual is the isolation-off state, not
the `exec` call.**

## 1. The execution path, in order

`runCommandWithStatus` (`harness.go:630`) applies five controls before a process starts:

1. `SplitCommand` — tokenizes the string and rejects `;&|<>$`, backtick, newline, and the
   glob/expansion characters `*?[]{}()~`. There is no shell, so nothing is reinterpreted
   downstream.
2. `selectRoot` — picks an authorized root for the call.
3. `checkCommandIn` — the authorization gate (§2).
4. `isolate` — the OS sandbox, default-on (§3).
5. `execCaptured` — bounded timeout, process-group kill, bounded capture, and an optional
   evidence sink.

## 2. What command authorization actually guarantees

`checkCommandIn` refuses a command unless `commandpolicy.Classify` returns `SAFE`
(`harness.go:797-803`). `REQUIRES_APPROVAL` is refused, not escalated, so the harness never
runs a `REQUIRES_APPROVAL` command. It also refuses `-C`/`--git-dir`/`--work-tree`/
`--exec-path`/`--namespace`, paths that leave the authorized root, anything naming the state
database, and repository mutations inside a read-only root.

The `SAFE` set is small: read-only `git` subcommands, `go test|build|vet|fmt|list`, `gofmt`,
and `cargo` under a separate permission and classifier.

Measured classification (`commandpolicy.Classify`; the probe was removed after the run):

| argv | Class | Harness outcome |
| --- | --- | --- |
| `sh -c id`, `bash -c id`, `/bin/sh -c id` | `REQUIRES_APPROVAL` | refused |
| `python3 -c …` | `REQUIRES_APPROVAL` | refused |
| `go run ./evil.go` | `REQUIRES_APPROVAL` | refused |
| `go generate ./...` | `REQUIRES_APPROVAL` | refused |
| `git push` / `git push --force` | `REQUIRES_APPROVAL` / `DENIED` | refused |
| **`go test ./...`** | **`SAFE`** | **runs** |
| **`go test -exec ./evil ./...`** | **`SAFE`** | **runs** |
| **`go build -o ./evil ./evil.go`** | **`SAFE`** | **runs** |
| **`go vet`, `go fmt`** | **`SAFE`** | **runs** |
| **`git -c diff.external=./evil diff`** | **`SAFE`** | **runs** |
| **`git -c core.fsmonitor=./evil status`** | **`SAFE`** | **runs** |

**The gap this exposes.** `go test` compiles and runs whatever `_test.go` files exist, and
the harness exposes `write_file`, `create_file`, and `rewrite_file`. So a model can author a
package and run it. The allowlist constrains the *program*, not the *code that program
executes*. `go test -exec` and git's `-c` external-command keys (`diff.external`,
`core.fsmonitor`, `core.pager`) are a second route to the same capability; they classify
`SAFE` and are not refused. (Only the classification was measured here, not live execution
of the git `-c` route.)

This is not a defect: writing code and running the tests is the product. It does mean a
reviewer must not read "no shell + SAFE-only" as "no arbitrary code execution".

## 3. What the sandbox actually contains

`isolate` is default-on and fail-closed. `isolationDecision` returns true on a platform with
no backend, and `sandboxArgv` then errors, so the command is refused rather than run
unsandboxed (Windows B1).

With isolation in effect, the child gets:

- no outbound network (loopback only; Linux uses a private network namespace);
- no writes outside the root and temp;
- no writes under `<root>/.agent-sdlc`;
- credential locations under `$HOME` and from credential environment variables masked, plus
  Unix sockets under `$HOME` masked on Linux;
- a **scrubbed environment** — variables with credential-like names or credential-shaped
  values are dropped (`sandboxEnv`), and a throwaway `GOCACHE` is injected.

With isolation disabled by `SOP_AGENT_RUN_COMMAND_ISOLATION=off` on macOS/Linux, the same
model-authored code runs with:

- **the parent environment inherited** — `runCommandWithStatus` leaves `env` nil, and
  `execCaptured` sets `cmd.Env = env`, so the child sees every variable SOP itself holds,
  including provider API keys;
- **network access**;
- **credential files readable** — nothing masks them.

`IsolationPolicyNotice` prints a startup warning when the override is off, so the state is
not silent. It does not reduce the impact.

## 4. Assessment

| State | Arbitrary program | Arbitrary code (via `go test`) | Network | Credential env | Credential files | Assessment |
| --- | --- | --- | --- | --- | --- | --- |
| Default, macOS/Linux | denied | runs, sandboxed | denied | scrubbed | masked | Bounded. The intended capability, contained. |
| Isolation off | denied | runs, unsandboxed | allowed | **inherited** | readable | **Unbounded** relative to the model's trust level. |
| No backend (Windows) | refused | refused | — | — | — | Fail-closed. |

The command policy and the sandbox **do** constrain model execution, but not to zero, and the
constraint depends entirely on the sandbox being in effect. The finding at `harness.go:920`
is a true capability, not a false positive; the mitigations recorded in the D4 packet are
real; and the residual is narrower and sharper than "the operator can turn it off". It is:
with isolation off, `go test` is model-controlled code execution with the provider's
credentials in its environment.

## 5. Measured control evidence

`go test ./internal/toolharness/ -run 'TestRunCommandIsolation|TestIsolationPolicyNotice|TestIsolationDecision'`

18 tests, all passing on darwin arm64, including
`TestRunCommandIsolationIsOnByDefault`, `TestRunCommandIsolationDefaultsToFailClosed`,
`TestRunCommandIsolationOnFailsClosedWithoutBackend`,
`TestRunCommandIsolationDoesNotFallBackWhenBackendFails`,
`TestRunCommandIsolationDeniesNetwork`, `TestRunCommandIsolationDeniesStateDirWrite`,
`TestRunCommandIsolationDeniesCredentialRead`, `TestRunCommandIsolationDeniesCredentialHardlink`,
`TestRunCommandIsolationDeniesEnvOverrideCredentials`,
`TestRunCommandIsolationDeniesKeychainAccess`, `TestRunCommandIsolationThrowawayGoCache`,
`TestRunCommandIsolationAllowsOwnUnixSocketOnly`, `TestIsolationDecisionUnsupportedPlatforms`.

None of these covers the isolation-off environment, because that state is the one the
override deliberately permits.

## 6. Bounded mitigations (proposed, not applied)

Each is small, testable, and tightens only the residual state. None changes how a sandboxed
command runs, and none changes a `SAFE` classification.

- **M1 — scrub the child environment even when unsandboxed (recommended, cheapest).** Today
  `sandboxEnv` runs only under isolation; when isolation is off the child inherits
  everything. Apply the same credential-name/shape filter in the unsandboxed branch. Effect:
  model-authored code can no longer read provider API keys from its environment in the
  residual state. Cost: reuse one helper in the non-isolated branch, plus a test asserting an
  unsandboxed child does not see a credential-named variable. Does not address network or
  credential-file exposure.
- **M2 — refuse code-executing `SAFE` subcommands when isolation is not in effect
  (strongest bounded fix).** When `!isolationRequested()`, allow only repository inspection
  and refuse `go test`, `go build`, `gofmt`, `cargo`. Effect: "isolation off" becomes
  read-only instead of arbitrary code execution. Cost: one predicate plus a table-driven
  test. Trade-off: an operator who disables isolation explicitly loses build and test.
- **M3 — deny `git -c <key>=<value>` in the harness (cheap complement).** The agent has no
  need for it, and it is one of the two config-injection routes to an external command. Cost:
  reject `-c` alongside the existing directory overrides; test.
- **M4 — two-key override (alternative to M2).** Require a second explicit variable in
  addition to `SOP_AGENT_RUN_COMMAND_ISOLATION=off`. Reduces accidental disabling; does not
  reduce impact once both are set.

Recommended minimum: **M1 + M3**, which removes the direct credential-exfiltration path and
the config-injection route while leaving the product's capability intact. Choose **M2** if
the operator wants the override to be provably read-only.

## 7. Residual decision — still open

The D4 options remain open and this report accepts none of them:

| Option | Meaning | Note after this analysis |
| --- | --- | --- |
| R1 | Accept the bounded residual with a short expiry | Defensible only together with M1 or M2; the residual state is credential-bearing. |
| R2 | Keep Option A (report-only) | Safe fallback; no enforcement. |
| R3 | Remove or disable `run_command` | Capability loss; separate product decision. |
| R4 | Authorize a code change to the exec mechanism or policy | Now has concrete, bounded candidates (M1–M3). |
| (new) R5 | Make isolation mandatory and delete the off override | Strongest; costs operators an escape hatch. |

**This report does not select R1.** What it changes is the evidence behind the choice: the
residual is not the `exec` call and not the command policy; it is the isolation-off state,
now specified precisely enough that a bounded mitigation (M1/M2) can precede any acceptance.

## What this report does not do

- It accepts no residual and records no waiver.
- It changes no code, configuration, baseline, or enforcement setting.
- It does not demonstrate live exploitation of the git `-c` route; only classification.
- It makes no Windows claim beyond the fail-closed refusal.
