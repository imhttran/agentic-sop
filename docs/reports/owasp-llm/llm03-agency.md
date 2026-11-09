# OWASP LLM03 — Agency: Current Execution Boundary

Task: AGENCY-001. Status: first-pass trace by inspection.

This report traces each supported execution entry point from its caller to its
command or effect sink. For each path it records the actual policy call, the
approval authority, credentials/environment, filesystem reach, and OS isolation.

## How to read this report

- **Traced** means the code path is followed by inspection, with `file:symbol`
  references. Line numbers drift; symbols do not, so references are file + symbol.
- **UNVERIFIED** means the step cannot be proven from inspected code. The reason
  is given inline.
- No suites were re-run to produce this report. Tests are named by inspection.
- **Policy-only** enforcement means in-process checks (name/flag/path rules). It
  is not OS isolation. Only an OS sandbox (container, namespace, seccomp, LSM,
  cgroup, bubblewrap, macOS `sandbox-exec`) is OS isolation. OS isolation was
  observed on exactly one path: the evaltarget child-command path (section 6).
  Every other traced path is policy-only.

## Distinctions required by the task

**Trusted configuration vs untrusted model/request data**

- Trusted configuration is operator environment and repository config:
  `SOP_AGENT_COMMAND` (`agent.EnvAgentCommand`), `SOP_WORKSPACE_ROOTS`
  (`ollamaagent.envWorkspaceRoots`), `SOP_TOOL_AUDIT_LOG`,
  `SOP_OLLAMA_TRACE_LOG`, `SOP_COMMAND_EVIDENCE_LOG`, `SOP_OLLAMA_*`,
  `SOP_OLLAMA_AGENT_BIN` / `SOP_OLLAMA_AGENT_HOME` (`agentbin.EnvOverride` /
  `agentbin.EnvHome`), the cargo permission (`config.CargoAgentTool`), and
  harness `Config.Roots`. `toolharness.Root` documents this: roots come from
  trusted configuration/input state and the model may only select an authorized
  root by exact canonical path (`toolharness.Harness.selectRoot`).
- Untrusted model/request data is the tool-call name, tool arguments, the
  `agent.Request` payload produced from model output, and MCP `tools/call`
  arguments. `toolharness.Harness.Run` dispatches on this data but every effect
  runs through `checkCommandIn` / `resolveIn` / the capability policy first.
  `mcp.Server.callTool` looks the requested tool name up in the fixed registered
  set (`mcp.Server.find`) and rejects an unknown name with
  `codeInvalidParams`; there is no generic filesystem or shell tool.

**Policy-only enforcement vs OS isolation**

- Policy calls observed: `commandpolicy.Classify`
  (`toolharness.Harness.checkCommandIn`), `commandpolicy.CargoClassify` (same
  function), `Harness.resolveIn` / `resolveArg` (path confinement,
  state-DB protection, read-only root), `Harness.SplitCommand` (no shell
  metacharacters), `ollamaagent.CapabilityPolicy.Allows` (tool-set gate),
  `github.validateBranchish`, and `evaltarget.confinedDir`.
- OS isolation observed: only on the `evaltarget` child path,
  `evaltarget.RunChild` → `isolation_linux.go` (`bwrap` bubblewrap) /
  `isolation_darwin.go` (`sandbox-exec`), with process-group kill
  (`sysProcAttrKill`, `terminateGroup`) and an environment allow-list
  (`evaltarget.IsolateEnv`). Every other execution sink uses plain
  `exec.CommandContext` with no namespace, container, seccomp, LSM or cgroup.

## Path-by-path matrix

Columns: Entry point | Caller → Sink | Policy call | Approval authority |
Credentials / environment | Filesystem reach | OS isolation

### 1. Normal tool-agent calls (controlled tool harness)

Entry point: `toolharness.Harness.Run` (`internal/toolharness/harness.go`). Every
tool name is in `toolharness.Tools`.

| Tool | Caller → Sink | Policy call | Approval authority | Env / creds | FS reach | OS isolation |
|---|---|---|---|---|---|---|
| read_file | `ollamaagent.executeLoop` → `Harness.Run` → `Harness.readFile` → `os.ReadFile` | `resolveArg`/`resolveIn` (escape check, `isProtectedPath`) | SOP task config + capability tool set (`CapabilityPolicy.Allows`) | Inherited process env; no credentials used | Selected authorized root only; `.agent-sdlc/state.db` refused (`stateDBRel`) | None (policy-only) |
| write_file / create_file | `Harness.writeFile` → `os.MkdirAll` + `os.WriteFile` | `resolveIn` `accessWrite` (read-only root refused, `errReadOnlyRoot`), protected-path check | Same | Env inherited; no creds | Authorized read-write root only | None |
| delete_file / restore_file / copy_file / rewrite_file | `Harness.deleteFile` / `restoreFile` / `copyFile` / `rewriteFile` → `os.Remove` / `os.WriteFile` / `os.ReadFile`+`os.WriteFile` | `resolveIn` (same rules); `copyFile` resolves source through an authorized root | Same | Same | Authorized roots only | None |
| list_files / search_files | `Harness.listFiles` / `searchFiles` → `os.ReadDir` / `filepath.WalkDir` | `resolveArg`; search skips `.git` and `stateDirRel` and `isProtectedPath` | Same | Same | Authorized roots; state DB never surfaced | None |
| run_command | `Harness.runCommand` → `runCommandWithStatus` → `SplitCommand`, `selectRoot`, `checkCommandIn`, `execCaptured` → `exec.CommandContext(argv[0], argv[1:]...)` | `SplitCommand` rejects shell metacharacters; `checkCommandIn` applies `hasDirOverride`, `namesStateDB`, `commandPathArgs`+`resolveIn`, read-only-root mutation check, then `commandpolicy.Classify` (or `CargoClassify` for cargo, gated by `CargoAgentToolEnabled`) and requires `Safe` | Only `commandpolicy.Safe` commands; the harness never authorizes Git writes | `exec.CommandContext` with no `cmd.Env` assignment → inherits the full process environment, including any credentials in it | `cmd.Dir` = selected authorized root; path args confined | None (policy-only) |
| git_status / git_diff | `Harness.dispatch` → `Harness.git(ctx, "status", "--short", "--branch")` / `"diff"` → `Harness.exec` → `exec.CommandContext` | Fixed subcommand list, allow-listed by construction | Harness code; not model-selectable | Inherited env; no creds | Repository root only | None |

Denials: `Harness.audit` records `ActionDeny`/`OutcomeDenied` for sentinel
denials (`errCommandNotAllowed`, `ErrProtectedPath`, `errUnauthorizedRoot`,
`errReadOnlyRoot`) via `isDenial`. Denials decided above the harness are recorded
through `Harness.RecordDenied`. Result content is not audited.

Callers into this harness: `ollamaagent.Harness` (a real agent loop),
`cli/preflight.go` (`PreflightAgentTool`), `cli/mutation.go`
(`RepositoryPathFingerprints`), `cli/outcome.go` (`SplitCommand`).

Gap / UNVERIFIED: whether any caller constructs a second, weaker command path is
UNVERIFIED. Inspection found the policy in `internal/toolharness` only;
`internal/ollamaagent/exec.go` states the command checks moved into
`internal/toolharness`.

### 2. Configured external agent commands

Entry point: `agent.NewCommandAgentFromEnv` reading `SOP_AGENT_COMMAND`
(`agent.EnvAgentCommand`) with legacy `AGENT_SDLC_AGENT_COMMAND`.

| Adapter | Caller → Sink | Policy call | Approval authority | Credentials / env | FS reach | OS isolation |
|---|---|---|---|---|---|---|
| `CommandAgent.Generate` (`internal/agent/command.go`) | SOP provider → `Generate` → `exec.CommandContext(ctx, "sh", "-c", a.command)` | **None on the configured command string.** The command is operator-configured, not model output. `request.Validate()` gates the payload only. | Operator environment (`SOP_AGENT_COMMAND`) | `cmd.Env` set to `os.Environ()` plus `SOP_SELECTED_CLASS` only when a class is set; the full inherited environment reaches the shell | Arbitrary: `sh -c` runs with the inherited working directory and full filesystem reach of the SOP process | None; a shell with no sandbox |
| `agentbin` (installed-binary resolution) | `agentbin.Resolve` → `Candidates` → `$SOP_OLLAMA_AGENT_BIN`, `$SOP_OLLAMA_AGENT_HOME/sop-ollama-agent`, `DefaultHome` | `isExecutableFile` (regular + executable). It never compiles working-tree source; a missing binary is `ErrNotInstalled`. | Operator environment | The resolved path only; no credentials read | Reads filesystem metadata in the configured locations; does not itself execute | Not a sink: `agentbin` resolves a path and returns it. Invocation happens elsewhere with the environment it is given. `internal/agentbin` is traced as a resolver, not an execution sink. |
| `CommandAgent` (ollama variant used in this run) | `SOP_AGENT_COMMAND` invoked `sop-ollama-agent`, which runs `ollamaagent.Run` | The inner loop re-enters `toolharness` (path 1) | Operator + harness policy | Inner harness inherits env; Ollama base URL/model from `SOP_OLLAMA_*` (`ollamaagent.ConfigFromEnv`) | Repo root from `getwd()`, plus `SOP_WORKSPACE_ROOTS` | None |
| `agent.openai` / `agent.openaicompatible` | Provider HTTP client → network sink | Request payload built from `agent.Request`; no command policy applies | Operator config (API key env) | **UNVERIFIED** — the exact credential variable names were not inspected in this pass | None locally; network reach to the configured endpoint | None (network egress unrestricted) |
| `agent.ollama` | Provider HTTP client → `SOP_OLLAMA_BASE_URL` (default `http://127.0.0.1:11434`) | None; HTTP request only | Operator config | No credential by default (local endpoint) | None locally | None |
| `agent.mlx` | Provider client | None; local model call | Operator config | **UNVERIFIED** — credential/env handling was not inspected in this pass | None locally | None |

Model influence: model output cannot choose the `SOP_AGENT_COMMAND` string,
but it does influence the `agent.Request` JSON passed on stdin and the tool
calls the inner loop executes. The inner tool calls are gated by path 1 policy.
The outer `sh -c` string is trusted configuration.

### 3. Verification commands

Entry: `internal/verify/verify.go`. Verification runs the acceptance commands via
the same controlled `run_command` path (`commandpolicy.Classify` requires
`Safe` for `go build`, `go test`, `go vet`). The targeted `go test -run`
selection is additionally guarded by `toolharness.noTestsMatched`, which appends
`noTestMatchDiagnostic` when the selection matched no tests, so a zero exit
cannot be read as a pass.

- Other verification-related sinks also exist outside the harness:
  `internal/testrunner` (`internal/testrunner/command.go`:
  `exec.CommandContext(ctx, "sh", "-c", command)`) runs a shell command
  string. It is not fed by model tool data; the command string is supplied by
  the caller. **UNVERIFIED**: whether every caller of `testrunner` passes only
  repository-verified command strings was not traced in this pass. Treat any
  caller of `testrunner` as policy-free shell execution until traced.
- `internal/testfast` and `internal/verifcache` are support packages for
  verification; their command construction was not traced line-by-line here.
- Policy call (harness path): `commandpolicy.Classify` inside `checkCommandIn`.
- Approval authority: SOP workflow decides which acceptance commands to run;
  the harness enforces the `Safe` class.
- Credentials/environment: inherited process env, no credentials.
- Filesystem reach: repository root; path args confined by `resolveIn`.
- OS isolation: none.
- Gap / UNVERIFIED: the internal call chain from scheduler to `internal/verify`
  and to `internal/testrunner` was not followed line-by-line in this pass.
  Verification is traced only to the point where it re-enters the harness
  policy (harness path) or reaches `testrunner`'s shell sink (unverified
  callers).

### 4. Git and GitHub operations

Harness Git is read-only by construction: `Harness.git` calls only `status` and
`diff`. `commandpolicy` does not classify `commit`, `push`, `merge`, `reset` or
`clean` as `Safe`, so the harness denies them (`errCommandNotAllowed`).

Write sinks live in SOP workflow code, outside the tool harness:

| Effect | Owning package / symbol | Policy | Approval authority | Credentials | FS reach | OS isolation |
|---|---|---|---|---|---|---|
| commit | `git.Adapter.Commit` (`internal/git/git.go`) → `run(ctx, dir, "commit", "-m", msg)` | None; structured args, no shell (`internal/git/command.go`: `exec.CommandContext(ctx, "git", args...)`) | SOP workflow (a stage decided to commit) | Inherited env; Git credential helper / SSH agent in env | Working directory of the adapter | None |
| branch / checkout / worktree | `git.Adapter.CreateBranch` / `Checkout` / `AddWorktree` / `RemoveWorktree` | `requireClean`, `validateBranch` (Git is the ref authority) | SOP workflow | Inherited env | Repo + worktree path | None |
| integration update (fetch + ff-only merge) | `git.Adapter.UpdateIntegrationBranch` | `requireClean`; `merge --ff-only` cannot create history | SOP workflow | Inherited env → Git remote credentials | Repo + remote | None |
| push (branch to origin) | `github.CommandClient.PushBranch` (`internal/github/command.go`) → `c.run(ctx, "git", "push", "-u", "origin", branch)` | `validateBranchish` (empty and metacharacter check); no command policy | SOP workflow that owns the PR handoff | Inherited env (`append(os.Environ(), "GH_PROMPT_DISABLED=1")`); host Git credential helper / SSH agent | Repository working directory of `CommandClient` | None |
| PR creation | `github.CommandClient.CreatePullRequest` → `c.run(ctx, "gh", "pr", "create", ...)` | Required non-empty base/head/title; structured args, no shell | SOP workflow | Inherited env; `gh` auth from host | Repository working directory | None |
| PR read / checks | `github.CommandClient.GetPullRequest` / `Checks` → `gh pr view` / `gh pr checks` | Argument guards; read-only | Workflow | Same | Same | None |
| merge | `github.CommandClient.Merge` → `c.run(ctx, "gh", "pr", "merge", ...)` | Method must be squash/merge/rebase | Workflow merge gate (`internal/mergegate` — not traced line-by-line) | Inherited env; `gh` auth | Repository working directory | None |

The package doc of `internal/git` states the rule: "Workflow control decides
when Git state may change; agents and models never call these operations
directly." The harness enforces that rule for the tool surface; the gates below
it are the approval authority for the write effects.

Gap / UNVERIFIED: `internal/commitgate` and `internal/mergegate` were not
inspected in this pass, so the exact branch-name and `gh` argument ownership of
commit and merge approvals is unproven. What is proven: the write commands run
through `internal/github/command.go`'s `execRunner` with the full inherited
environment and no OS isolation.

### 5. Planner and MCP entry points

Planner: `internal/planner.Planner.GenerateWithContext` builds an
`agent.Request` (`Capability: agent.Plan`, `Task: planTaskPrompt`,
`OutputRequirements: planOutputRequirements`) and calls `p.agent.Generate`. The
response is parsed and validated by `decodePlan` / `Plan.Validate`, a
deterministic authority; invalid plans are repaired at most `maxPlanRepairs`
times. `decodePlan` clears model-authored `AcceptanceChecks`, so a model cannot
author its own acceptance checks. The Planner writes no files and touches no
workflow state.

- Caller → sink: `Planner.GenerateWithContext` → `agent.Agent.Generate` →
  whichever adapter in section 2 is configured. It does not itself execute a
  command; its sink is the model adapter's sink.
- Policy: `Plan.Validate` on the model output; deterministic acceptance checks
  are only accepted from the operator's Markdown path.
- Approval authority: SOP planflow decides whether the planner is available.
- Model-data handling: the PRD/context and model output are untrusted; the
  planner enforces a JSON schema and validates the resulting graph. It does not
  grant any execution authority.

MCP: `internal/mcp.Server` (`mcp.go`) is a newline-delimited JSON-RPC server
that serves `initialize`, `tools/list`, `tools/call`. It exposes only the tools
passed to `mcp.NewServer`. `callTool` parses the client's `name`, resolves it
via `Server.find`, and rejects unknown names with `codeInvalidParams`. There is
no generic read/write/exec tool in the package:

- MCP `tools/*` requests are untrusted client/request data. They are gated by
  the fixed registered-tool set. The `ReadOnly` flag on `mcp.Tool` is
  descriptive only (`toolDescriptors` adds `annotations.readOnlyHint`); access is
  enforced by registration, not by that flag.
- Traced: every MCP tool either exists in the registered set (allowed) or is
  rejected. None of the registered handlers live in `internal/mcp`; they are
  supplied by the caller.
- Gap / UNVERIFIED: the concrete handler set registered by
  `internal/cli/mcp_start_work.go` was not enumerated in this pass. Whether each
  handler reaches `toolharness` policy or a dedicated workflow service is
  unproven. `internal/cli/mcp_start_work.go` also runs `exec.Command(exe, args...)`
  to launch a sub-process; its arguments were not traced.
- Untrusted-input note: MCP request data must be treated as untrusted, like model
  tool-call data. This report claims only the name gate (registered tools only);
  it does not claim that every registered handler is policy-checked.

### 6. Replay and evaltarget

`internal/evaltarget.RunChild` (`internal/evaltarget/command.go`) is a real
re-execution path and it applies stronger isolation than the tool harness:

- Confinement: `confinedDir(root, dir)` resolves symlinks and refuses a `Dir`
  outside `Root` (filesystem reach = the authorized sandbox root only).
- Environment: a nil `Env` is replaced by `evaltarget.IsolateEnv(os.Environ(), "")`,
  an allow-list that strips production secrets (credential isolation).
- Process group: `cmd.SysProcAttr = sysProcAttrKill()`, and on timeout or
  cancellation `terminateGroup(cmd.Process.Pid)` kills the whole group
  (`proc_unix.go`).
- **OS isolation: yes on Linux and macOS.** `isolation_linux.go` invokes
  `exec.CommandContext(ctx, bwrap, args...)` (bubblewrap); `isolation_darwin.go`
  invokes `exec.CommandContext(ctx, sandboxExecPath, args...)` (macOS
  `sandbox-exec`). `isolation_other.go` covers other platforms. The precise
  namespace and host binding list is in `sandbox.go` / `isolation.go` and was
  not read line-by-line in this pass; isolation presence is proven by the two
  platform files above.
- Approval authority: the SOP AISEC/eval workflow supplies the case and its
  `CommandRequest`; the model does not. `evaltarget` arguments are not read from
  model output.
- Related packages: `internal/eval` (evaluations), `internal/runtrace`
  (trace recording), `internal/recovery` and `internal/resume` (workflow
  recovery and resume), and the `cmd/deceleval-*` commands. The latter invoke
  `evaltarget` for their case execution. `internal/recovery` and `internal/resume`
  do not appear to re-execute arbitrary child commands; recovery resumes the SOP
  workflow, which then goes back through the tool harness (path 1) — this is
  reasoned from package boundaries, not traced line-by-line.
- Gap / UNVERIFIED: the exact call chain from `internal/recovery` /
  `internal/resume` / `cmd/deceleval-*` into `evaltarget.RunChild` (and whether
  any of those packages also runs a plain `exec.Command`) was not traced
  line-by-line. `internal/runtrace` and `internal/eval` were not inspected.

### 7. Entry points found during tracing beyond the named set

- `cli/preflight.go` `PreflightAgentTool` builds `toolharness.New` with
  `CargoAgentToolEnabled: decl.CargoAgentTool`. Cargo permission comes from
  `config.CargoAgentTool` (repository config, trusted).
- `cli/mutation.go` `RepositoryPathFingerprints` and
  `ollamaagent.Harness.WorkingTreeFingerprint` / `WorkingTreeChanged` /
  `ChangedSince` run fixed read-only Git commands through `Harness.exec` /
  `execFingerprintCmd`. Model cannot influence the subcommand.
- Operator-set log sinks: `SOP_TOOL_AUDIT_LOG` (`NewFileAuditor`),
  `SOP_OLLAMA_TRACE_LOG` (`Harness.FlushTrace`), `SOP_COMMAND_EVIDENCE_LOG`
  (`newEvidenceSink`). These are written outside `.agent-sdlc/state.db` by
  construction.

## Approval authority (human gate)

Human approval is a SOP lifecycle decision recorded through one boundary:
`internal/approval.Service` (`internal/approval/approval.go`). It is the
approval authority, and it is distinct from `commandpolicy.Classify`'s
`REQUIRES_APPROVAL` class:

- `commandpolicy.RequiresApproval` is a classification label only. The harness
  treats it as a denial (`checkCommandIn` accepts only `Safe`), so a
  `REQUIRES_APPROVAL` command never runs through the tool harness. No inspected
  path executes a `REQUIRES_APPROVAL` command automatically.
- `approval.Service.Request` records an explicit gate (`Kind`, `Target`,
  `Reason`, `Evidence`, `DefinitionDigest`) and returns a request id. A caller
  must not infer a gate from a `BLOCKED`/`NEEDS_HUMAN` status; only a recorded
  request is a real boundary (`approval.View.Present`).
- `approval.Service.Approve` / `Decline` record a decision. Approving resumes a
  `BLOCKED` task through `domain.Task.RequeueWithoutSpending`, the same lifecycle
  authority `sop retry` uses; it does not force a `PASS` and does not bypass
  validation, review or the quality gate.
- `approval.Service.Supersede` / `Invalidate` / `InvalidateAttested` are
  operator-authorized closures of a stale request, each requiring a non-empty
  `Reason` and `By`. They are fail-closed and ordered: the original request is
  appended to the history before the head is rewritten.
- The service is deterministic and I/O-free: persistence (`Record`, `Tasks`) and
  the clock are injected. It applies no filesystem or command policy itself. Its
  authority is over the SOP lifecycle, not over a command or effect sink.

Approval authority per traced path: the SOP workflow is the approval authority
for every command and effect sink in sections 1-6. The tool harness (section 1)
never authorizes a write Git effect; Git/GitHub write effects (section 4) are
gated by the workflow, not by `commandpolicy`. The human `approval.Service`
records the lifecycle gate that a workflow raises; it is not called from any
`exec.CommandContext` site.

## Existing tests (identified by inspection, not rerun)

No suite was rerun for this report. The following test files were identified by
search as covering the traced paths:

- Tool harness policy and denials: `internal/toolharness/harness_test.go`,
  `preventive_safety_test.go`, `readonly_command_test.go`, `roots_test.go`,
  `write_scenarios_test.go`, `mutation_test.go`, `mutation_snapshot_test.go`,
  `mutation_close004_test.go`, `cargo_test.go`, `notests_test.go`,
  `evidence_sink_test.go`, `import_sequence_test.go`.
- Tool-agent loop, capability policy, audits: `internal/ollamaagent/harness_test.go`,
  `policy_test.go`, `correction_phase_test.go`, `correction_phase_loop_test.go`,
  `cargo_wiring_test.go`, `mutation_verification_test.go`,
  `already_satisfied_test.go`, `changed_files_test.go`, `evidence_artifact_test.go`,
  `evidence_citation_test.go`, `reconcile_test.go`, `fix_lifecycle_test.go`,
  `dogfood_test.go`, `activity_test.go`.
- End-to-end tool denial: `internal/e2e/lifecycle/tool_denial_test.go`.
- Command agent: `internal/agent/command_test.go`, `provider_test.go`,
  `openaicompatible_execution_test.go`, `openaicompatible_test.go`, `mlx_test.go`.
- Installed-binary resolution: `internal/agentbin/resolve_test.go`,
  `internal/agentbin/wrapper_test.go`.
- Git adapter: `internal/git/git_test.go`, `head_test.go`,
  `untracked_binary_test.go`; `internal/git/git_test.go` exercises a real
  `git push` to a local remote.
- GitHub CLI client: `internal/github/` tests (not enumerated in this pass).
- Verification: `internal/verify/verify_test.go`; `internal/testfast/testfast_test.go`.
- MCP server: `internal/mcp/mcp_test.go` (name gate, unknown-tool rejection).
- Planner: `internal/planner/planner_test.go`, `compiler_hardening_test.go`,
  `required_evidence_test.go`, `taskshape_test.go`, `security_plan_test.go`.
- Evaltarget isolation: `internal/evaltarget/runner_test.go`,
  `production_controls_test.go`, `aisec_test.go`, `isolation_darwin_test.go`.
- Architecture guard: `internal/archtest` (tests exist per plan evidence; not
  inspected in this pass).

Paths with no test identified in this pass: the concrete MCP handler set in
`internal/cli/mcp_start_work.go`, replay (`internal/recovery`, `internal/resume`),
and `internal/eval` / `internal/runtrace`. These are also the UNVERIFIED paths,
so the missing tests are consistent with the missing trace.

## Host-specific runtime evidence

- Environment variables that shape this boundary:
  `SOP_AGENT_COMMAND` / `AGENT_SDLC_AGENT_COMMAND`, `SOP_SELECTED_CLASS`,
  `SOP_OLLAMA_BASE_URL`, `SOP_OLLAMA_MODEL`, `SOP_OLLAMA_TIMEOUT`,
  `SOP_WORKSPACE_ROOTS`, `SOP_TOOL_AUDIT_LOG`, `SOP_OLLAMA_TRACE_LOG`,
  `SOP_COMMAND_EVIDENCE_LOG`, `SOP_OLLAMA_AGENT_BIN`,
  `SOP_OLLAMA_AGENT_HOME`. Read by `agent.EnvAgentCommand`,
  `agentbin.Candidates`, and `ollamaagent.ConfigFromEnv`.
- OS isolation on the evaltarget path depends on host tooling: `bwrap`
  (bubblewrap) on Linux and `/usr/bin/sandbox-exec` on macOS. If that tooling is
  absent, isolation is not available.
- Cargo permission: project config `cargo.agent_tool` → `config.CargoAgentTool`
  → `toolharness.Config.CargoAgentToolEnabled`.
- Git and GitHub effects rely on the host's Git credential helper or SSH agent
  and on `gh` authentication. No SOP-managed credential store was observed on
  the inspected paths.
- Host-specific runtime evidence for the current attempt was not captured in
  this pass. This report does not present an execution log.

## Summary findings

1. Every tool-agent effect passes one policy implementation in
   `internal/toolharness`. Denials are sentinel-classified and audited.
2. The full process environment (and any credentials in it) is inherited by
   every `exec.CommandContext` sink on paths 1-5 and by the `sh -c` external
   agent command. Only `evaltarget.RunChild` uses an allow-list environment.
3. OS isolation was observed only on the evaltarget child path (`bwrap` /
   `sandbox-exec`). Paths 1-5 are policy-only. Agent processes are not sandboxed.
4. Git write effects and push/PR/merge are owned by `internal/git` and
   `internal/github`, gated by the SOP workflow, not by the harness. The harness
   correctly refuses destructive Git.
5. MCP is a fixed registered-tool server with a name gate; the concrete handler
   set is UNVERIFIED. The planner path funnels through the configured agent
   adapter and validates its output deterministically. The concrete
   `internal/recovery`/`internal/resume`/`cmd/deceleval-*` → `evaltarget` chain
   and `internal/eval`/`internal/runtrace` remain UNVERIFIED.

## Optional targeted follow-up (not run)

- `go test ./internal/toolharness/ ./internal/ollamaagent/ ./internal/git/`
- `go test ./internal/verify/ ./internal/mcp/ ./internal/planner/ ./internal/evaltarget/`

These are optional. This report was produced by inspection.
