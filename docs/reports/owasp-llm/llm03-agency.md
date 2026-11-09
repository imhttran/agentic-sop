# OWASP LLM03 — Agency: Current Execution Boundary

Task: AGENCY-001 inventory (refreshed) plus AGENCY-002 production-boundary
tests and AGENCY-003 verification record. This revision reuses the AGENCY-001
inventory, re-checks its `file:symbol` references against current source, and
records the AGENCY-003 verification results in §Verification record.

This report traces each supported execution entry point from its caller to its
command or effect sink. For each path it records the actual policy call, the
approval authority, credentials/environment, filesystem reach, and OS isolation.

## How to read this report

- **Traced** means the code path is followed by inspection, with `file:symbol`
  references. Line numbers drift; symbols do not, so references are file + symbol.
- **UNVERIFIED** means the step cannot be proven from inspected code. The reason
  is given inline.
- **Policy-only** enforcement means in-process checks (name/flag/path rules). It
  is not OS isolation. Only an OS sandbox (container, namespace, seccomp, LSM,
  cgroup, bubblewrap, macOS `sandbox-exec`) is OS isolation. OS isolation was
  observed on exactly one path: the evaltarget child-command path (entry point
  7). Every other traced path is policy-only.
- Normal agent process execution is **policy-only**. It is not sandboxed. Only
  the evaltarget child path carries OS isolation.

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
  root by exact canonical path (`toolharness.Harness.selectRoot`). Verification
  command strings are trusted project configuration too: `internal/testrunner`
  states the command string "is never built from untrusted input"
  (`testrunner.execute`).
- Untrusted model/request data is the tool-call name, tool arguments, the
  `agent.Request` payload produced from model output, and MCP `tools/call`
  arguments. `toolharness.Harness.Run` dispatches on this data but every effect
  runs through `checkCommandIn` / `resolveIn` / the capability policy first.
  `mcp.Server.callTool` looks the requested tool name up in the fixed registered
  set (`mcp.Server.find`) and rejects an unknown name with
  `codeInvalidParams`; there is no generic filesystem or shell tool in
  `internal/mcp`.
- MCP arguments are untrusted even when a registered tool accepts them. The
  registered handlers confine them where they touch the filesystem: for example
  `startWork` in `internal/cli/mcp_start_work.go` treats a client-supplied
  `plan_path` as untrusted and confines it through `planflow.ResolvePlanPath` +
  `resolvePromptFile` before `os.ReadFile`. `sop_answer_submit` and
  `sop_decision_submit` record state and never approve or resume. The MCP
  approval tools (`sop_approve`, `sop_decline`) are the human gate, not a
  capability the model gains over commands.
- The planner treats the PRD/context and model output as untrusted:
  `planner.Plan.Validate` is deterministic and `decodePlan` clears
  model-authored `AcceptanceChecks`, so the model cannot author its own
  acceptance checks.

**Policy-only enforcement vs OS isolation**

- Policy calls observed: `commandpolicy.Classify`
  (`toolharness.Harness.checkCommandIn`), `commandpolicy.CargoClassify` (same
  function), `Harness.resolveIn` / `resolveArg` (path confinement,
  state-DB protection, read-only root), `Harness.SplitCommand` (no shell
  metacharacters), `ollamaagent.CapabilityPolicy.Allows` (tool-set gate),
  `github.validateBranchish`, `mcp.Server.find` (registered-tool name gate),
  `cli.startWork` path confinement (`planflow.ResolvePlanPath`,
  `resolvePromptFile`), and `evaltarget.confinedDir`.
- OS isolation observed: only on the `evaltarget` child path,
  `evaltarget.RunChild` → `isolation_linux.go` (`bwrap` bubblewrap) /
  `isolation_darwin.go` (`sandbox-exec`), with process-group kill
  (`sysProcAttrKill`, `terminateGroup`) and an environment allow-list
  (`evaltarget.IsolateEnv`).
- Every other execution sink uses plain `exec.CommandContext`, `exec.Command`,
  or `sh -c` with no namespace, container, seccomp, LSM or cgroup. Named sinks:
  `toolharness.Harness.exec` / `runCommandWithStatus` / `execCaptured`,
  `agent.CommandAgent.Generate` (`sh -c`), `testrunner.execute` (`sh -c`),
  `git` `execRunner`, `github` `execRunner`, and
  `cli.launchWorkProcess` (`exec.Command(exe, args...)`).
- The MCP `sop_start_work` tool is a launch surface, not a sandbox boundary: it
  starts the same `sop` binary that an operator would run, so the started
  process is policy-only, exactly as the terminal command is. It does not
  approve a gate.

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
| `CommandAgent.Generate` (`internal/agent/command.go`) | SOP provider → `Generate` → `exec.CommandContext(ctx, "sh", "-c", a.command)` | **None on the configured command string.** The command is operator-configured, not model output. `request.Validate()` gates the payload only. | Operator environment (`SOP_AGENT_COMMAND`) | `cmd.Env` set to `os.Environ()` plus `SOP_SELECTED_CLASS` only when a class is set; the full inherited environment reaches the shell | Arbitrary: `sh -c` runs with the inherited working directory and full filesystem reach of the SOP process | None; a shell with no sandbox (policy-only) |
| `agentbin` (installed-binary resolution) | `agentbin.Resolve` → `Candidates` → `$SOP_OLLAMA_AGENT_BIN`, `$SOP_OLLAMA_AGENT_HOME/sop-ollama-agent`, `DefaultHome` | `isExecutableFile` (regular + executable). It never compiles working-tree source; a missing binary is `ErrNotInstalled`. | Operator environment | The resolved path only; no credentials read | Reads filesystem metadata in the configured locations; does not itself execute | Not a sink: `agentbin` resolves a path and returns it. Invocation happens elsewhere with the environment it is given. `internal/agentbin` is traced as a resolver, not an execution sink. |
| `CommandAgent` (ollama variant used in this run) | `SOP_AGENT_COMMAND` invoked `sop-ollama-agent`, which runs `ollamaagent.Run` | The inner loop re-enters `toolharness` (entry point 1) | Operator + harness policy | Inner harness inherits env; Ollama base URL/model from `SOP_OLLAMA_*` (`ollamaagent.ConfigFromEnv`) | Repo root from `getwd()`, plus `SOP_WORKSPACE_ROOTS` | None |
| `agent.openai` / `agent.openaicompatible` | Provider HTTP client → network sink | Request payload built from `agent.Request`; no command policy applies | Operator config (API key env) | **UNVERIFIED** — the exact credential variable names were not re-inspected in this pass | None locally; network reach to the configured endpoint | None (network egress unrestricted) |
| `agent.ollama` | Provider HTTP client → `SOP_OLLAMA_BASE_URL` (default `http://127.0.0.1:11434`) | None; HTTP request only | Operator config | No credential by default (local endpoint) | None locally | None |
| `agent.mlx` | Provider client | None; local model call | Operator config | **UNVERIFIED** — credential/env handling was not re-inspected in this pass | None locally | None |

Model influence: model output cannot choose the `SOP_AGENT_COMMAND` string,
but it does influence the `agent.Request` JSON passed on stdin and the tool
calls the inner loop executes. The inner tool calls are gated by entry point 1
policy. The outer `sh -c` string is trusted configuration.

### 3. Verification commands

`internal/verify` is the pure, model-free verification risk tier
(`verify.Tier`). It performs no I/O and is not a command sink. The two
verification execution paths are:

- **Configured validation** (`build`, `test`, `lint` from
  `.agent-sdlc/config.yaml`): `cli.timedValidation` / `cli.sessionValidation`
  (`internal/cli/run.go`) -> `validate.Run(ctx, dir, cfg.Validation)`
  (`internal/validate/validate.go`) -> `testrunner.New(dir).RunAll(ctx, Checks(v))`
  -> `testrunner.Runner.Run` -> `testrunner.execute`
  (`internal/testrunner/command.go`: `exec.CommandContext(ctx, "sh", "-c",
  command)`).
  - Command source: `config.Validation` is trusted repository/operator
    configuration. `validate.Checks` only maps configured build/test/
    integration/lint entries into ordered checks; it accepts no request data.
    The other `validate.Run` callers are `cli.task`, `cli.mcpValidate`
    (`internal/cli/mcp.go`), and the `sop validate` command
    (`internal/cli/validate.go`); each supplies `cfg.Validation` or an
    operator-provided `config.Validation`.
  - Policy call on the `testrunner` sink: **none in-process.** No
    `commandpolicy.Classify` runs inside `internal/testrunner`; trust is
    delegated to the caller that supplies the command string. The shell itself
    is not confined.
  - Environment: `testrunner.buildEnv(env)` starts from `os.Environ()` and
    applies overrides last, so the full inherited environment reaches the shell.
  - Filesystem reach: `cmd.Dir` (the project directory) plus whatever the shell
    can reach; no path confinement on this sink.
- **Operator-declared acceptance checks** (the plan's `### Acceptance Checks`):
  `cli.deriveOutcome` / `cli.deriveMappedOutcome` (`internal/cli/outcome.go`) ->
  `authorizeAcceptanceCommandWith` (`toolharness.SplitCommand`, dangerous-form
  rejection, `commandpolicy.Classify` requiring `Safe`, or `CargoClassify`) ->
  `runAcceptanceCheck` -> `runAcceptanceArgv` ->
  `exec.CommandContext(argv[0], argv[1:]...)`.
  - Command source: the operator-authored plan, validated into an exact argv
    vector and classified `Safe` before execution.
  - Policy call: `commandpolicy.Classify` / `CargoClassify` in
    `authorizeAcceptanceCommandWith`.
  - Isolation: argv-only, no shell; no OS isolation.
  - A `go test` check is judged from `go test -json` events, so exit 0 with no
    executed passing test is not met.
- `internal/testfast` and `internal/verifcache` are verification support
  packages; their command construction was not re-traced line-by-line here.
- Approval authority: the SOP workflow decides which validation/acceptance
  commands run. `internal/testrunner` has no approval step; its caller owns the
  choice. Acceptance checks are operator-declared plan data.
- Credentials/environment: inherited process env; no credential is added by SOP.
- OS isolation: none on either path.
- Remaining UNVERIFIED: the scheduler-to-`internal/verify`/`internal/validate`
  call chain was not followed line-by-line (package-boundary reasoning only).
  The trust statements above are per caller (`cfg.Validation` /
  operator-authored argv), not a claim that every possible caller is safe.

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
| merge | `github.CommandClient.Merge` → `c.run(ctx, "gh", "pr", "merge", ...)` | Method must be squash/merge/rebase | Workflow merge gate (`internal/mergegate`) | Inherited env; `gh` auth | Repository working directory | None |

The package doc of `internal/git` states the rule: "Workflow control decides
when Git state may change; agents and models never call these operations
directly." The harness enforces that rule for the tool surface; the gates below
it are the approval authority for the write effects.

Gap / UNVERIFIED: `internal/commitgate` and `internal/mergegate` were not
re-inspected here, so the exact branch-name and `gh` argument ownership of
commit and merge approvals remains unproven. What is proven: the write commands
run through `internal/github/command.go`'s `execRunner` with the full inherited
environment and no OS isolation.

### 5. Planner and MCP entry points

#### 5a. Planner

Planner: `internal/planner.Planner.GenerateWithContext` builds an
`agent.Request` (`Capability: agent.Plan`, `Task: planTaskPrompt`,
`OutputRequirements: planOutputRequirements`) and calls `p.agent.Generate`. The
response is parsed and validated by `decodePlan` / `Plan.Validate`, a
deterministic authority; invalid plans are repaired at most `maxPlanRepairs`
times. `decodePlan` clears model-authored `AcceptanceChecks`, so a model cannot
author its own acceptance checks. The Planner writes no files and touches no
workflow state.

- Caller → sink: `Planner.GenerateWithContext` → `agent.Agent.Generate` →
  whichever adapter in entry point 2 is configured. It does not itself execute a
  command; its sink is the model adapter's sink.
- Policy: `Plan.Validate` on the model output; deterministic acceptance checks
  are only accepted from the operator's Markdown path.
- Approval authority: SOP planflow decides whether the planner is available.
- Model-data handling: the PRD/context and model output are untrusted; the
  planner enforces a JSON schema and validates the resulting graph. It does not
  grant any execution authority.

#### 5b. MCP server and registered handler set

`internal/mcp.Server` (`mcp.go`) is a newline-delimited JSON-RPC server that
serves `initialize`, `tools/list`, `tools/call`. It exposes only the tools passed
to `mcp.NewServer`. `callTool` parses the client's `name`, resolves it via
`Server.find`, and rejects unknown names with `codeInvalidParams`. The `ReadOnly`
flag on `mcp.Tool` is descriptive only (`toolDescriptors` adds
`annotations.readOnlyHint`); access is enforced by registration, not by that
flag.

Concrete handler set, registered by `cli.runMCP` (`internal/cli/mcp.go`) and
`cli.mcpStartWorkTools` (`internal/cli/mcp_start_work.go`), in three modes plus
one opt-in surface:

- Default (`sop mcp`), from `cli.mcpTools` → `cli.mcpToolsWithMode(dir, d, false)`:
  `sop_status` (`cli.mcpStatusTool`, read-only), `sop_validate` (`cli.mcpValidate`),
  `sop_review` (`cli.mcpReview`), `sop_approval` (`cli.approvalRead`),
  `sop_approve` (`cli.approvalApprove`), `sop_decline` (`cli.approvalDecline`).
- Status-only (`sop mcp --status-only`), from
  `cli.mcpToolsWithMode(dir, d, true)`: `sop_status` only. The approval, validate,
  and review tools are never registered, so they cannot be listed or called.
- Planner (`sop mcp --planner`), from `cli.mcpPlannerTools`: `sop_status`,
  `sop_question_list`, `sop_question_get`, `sop_answer_submit`, `sop_task_result`,
  `sop_decision_list`, `sop_decision_submit`. The human approval actions are
  deliberately absent.
- Opt-in start-work (`--start-work`, appended by `cli.runMCP`), from
  `cli.mcpStartWorkTools`: `sop_start_work` (`cli.mcpStartWorkTool`) and
  `sop_work_status` (`cli.mcpWorkStatusTool`, read-only). `--start-work` is
  refused with `--status-only`.

Reach of each registered handler:

- Read-only handlers read the state database or working tree and write nothing:
  `mcpStatus`, `mcpQuestionListTool` / `mcpQuestionGetTool`, `mcpWorkStatusTool`.
- `sop_validate` runs the configured validation commands through `validate.Run`
  (entry point 3). This is a policy-only command sink; the commands are trusted
  repository config.
- `sop_review` calls the configured review provider (`reviewProvider`); its sink
  is the provider adapter in entry point 2.
- `sop_approve` / `sop_decline` call `approval.Service.Approve` / `Decline`
  through `cli.approvalTool`. They record the human gate; they never bypass
  validation, review, or the quality gate. `sop_answer_submit` and
  `sop_decision_submit` record state only.
- `sop_start_work` is the one MCP handler that launches a subprocess:
  `cli.startWork` (`internal/cli/mcp_start_work.go`) → `env.launch` →
  `cli.launchWorkProcess` → `exec.Command(exe, args...)` with
  `exe = os.Executable()`, `cmd.Dir = dir`,
  `cmd.Env = append(os.Environ(), JobEnvDir, JobEnvID, JobEnvProject)`, and a
  detached process attribute (`detachedProcAttr()`). No shell is used.
  - Argv builder: `startWork`'s `switch job.Kind` block sets
    `job.Args = []string{"prompt", "--capability", job.Capability, "--file",
    promptPath}` for prompt work, or `job.Args = []string{"run", job.Source}`
    for plan work.
  - Per-argument provenance: the verbs and flags (`prompt`, `--capability`,
    `--file`, `run`) are literal; `job.Capability` is
    `promptCapabilityName(parsePromptCapability(...))`, a validated enum;
    `promptPath` is a server-generated path under the job directory (the
    request prompt is written to that file, never passed on argv);
    `job.Source` is `planflow.SourceRel(dir, resolved)` after
    `planflow.ResolvePlanPath` + `resolvePromptFile` confine the
    client-supplied `plan_path`. Request data never appears literally in the
    child argv: the prompt becomes a file, the capability is an enum, and the
    plan path is confined before it becomes a relative source.
  - Other bounds: prompt size (`maxWorkPromptBytes`), duplicate/concurrent job
    refusal (`acquireWorkLock`, `listWorkJobs`, mid-run task check), and no
    approval action. OS isolation: none (policy-only).

Untrusted-input note: MCP `tools/*` requests are untrusted client/request data.
They are gated by the fixed registered-tool set; arguments are confined only
where the handler code confines them (for example `startWork`'s `plan_path`).
This report claims the name gate and the per-handler confinement above; it does
not claim that every handler performs a `commandpolicy` check.

### 6. Replay entry points (recovery, resume, eval, runtrace, cmd/deceleval-*)

- `internal/recovery` and `internal/resume` restore and continue the SOP
  workflow. **UNVERIFIED (package-boundary reasoning only):** the
  statement-by-statement path from either package to a command sink was not
  traced line-by-line, so they are not described as sink-free or sandboxed. A
  search of both packages found no `os/exec` import or `exec.Command` site, and
  a resumed workflow is expected to re-enter the tool harness (entry point 1)
  and the configured agent command (entry point 2); that expectation is not a
  proof.
- `internal/runtrace` records trace data. **UNVERIFIED**: it was not inspected in
  this pass, so whether it opens any process sink or writes outside the trace
  store is unproven. `internal/eval` is likewise **UNVERIFIED** for the same
  reason: neither package was inspected in this pass.
- `cmd/deceleval-*` are the case-execution commands. The recovered inventory
  states they invoke `evaltarget` for case execution; **UNVERIFIED**: the exact
  call chain from each `cmd/deceleval-*` `main` into `evaltarget.RunChild` was not
  re-traced line-by-line here, and any additional direct `exec.Command` inside
  those commands was not excluded.
- Sink in the traced case: `evaltarget.RunChild` (entry point 7).

### 7. evaltarget

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

This is the only entry point in this report that carries OS isolation. The
replay packages in entry point 6 are unproven, not sandboxed by assumption.

### 8. Entry points found during tracing beyond the named set

- `cli/preflight.go` `PreflightAgentTool` builds `toolharness.New` with
  `CargoAgentToolEnabled: decl.CargoAgentTool`. Cargo permission comes from
  `config.CargoAgentTool` (repository config, trusted).
- `cli/mutation.go` `RepositoryPathFingerprints` and
  `ollamaagent.Harness.WorkingTreeFingerprint` / `WorkingTreeChanged` /
  `ChangedSince` run fixed read-only Git commands through `Harness.exec` /
  `execFingerprintCmd`. Model cannot influence the subcommand.
- `cli.launchWorkProcess` (MCP start-work subprocess launch) is traced in entry
  point 5b; it is a policy-only `exec.Command` sink.
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
- Over MCP, the approval authority is reachable through `sop_approval`,
  `sop_approve`, and `sop_decline` (`cli.approvalTool`), which delegate to the
  same `approval.Service`. They are absent from the status-only and planner
  surfaces.

Approval authority per traced path: the SOP workflow is the approval authority
for every command and effect sink in entry points 1-7. The tool harness (entry
point 1) never authorizes a write Git effect; Git/GitHub write effects (entry
point 4) are gated by the workflow, not by `commandpolicy`. The human
`approval.Service` records the lifecycle gate that a workflow raises; it is not
called from any `exec.CommandContext` site.

## Existing tests (identified by inspection)

- Tool harness policy and denials: `internal/toolharness/harness_test.go`,
  `preventive_safety_test.go`, `readonly_command_test.go`, `roots_test.go`,
  `write_scenarios_test.go`, `mutation_test.go`, `mutation_snapshot_test.go`,
  `mutation_close004_test.go`, `cargo_test.go`, `notests_test.go`,
  `evidence_sink_test.go`, `import_sequence_test.go`.
- AGENCY-002 production-boundary regression tests:
  `internal/approval/agency002_binding_test.go` drives the production
  `approval.Service`; `internal/toolharness/agency002_contract_test.go` drives
  the production tool harness contract.
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
- Verification: `internal/verify/verify_test.go`; `internal/testfast/testfast_test.go`;
  `internal/testrunner/runner_test.go`, `enforcement_test.go`,
  `gofmt_gate_test.go`, `postgres_skip_test.go`.
- MCP server: `internal/mcp/mcp_test.go` (name gate, unknown-tool rejection);
  `internal/cli/mcp_planner_test.go`, `mcp_decision_test.go`,
  `mcp_start_work_test.go`, `mcp_status_only_test.go` exercise the registered
  handler sets and the start-work launcher with an injected fake `workLauncher`.
- Planner: `internal/planner/planner_test.go`, `compiler_hardening_test.go`,
  `required_evidence_test.go`, `taskshape_test.go`, `security_plan_test.go`;
  `internal/cli/planner_test.go`, `planner_gate_test.go`,
  `planner_contract_cli_test.go`, `planner_integration_test.go`.
- Evaltarget isolation: `internal/evaltarget/runner_test.go`,
  `production_controls_test.go`, `aisec_test.go`, `isolation_darwin_test.go`.
- Architecture guard: `internal/archtest` (tests present and passing under
  `go test ./...` in this run).

Paths with no test identified in this pass: replay (`internal/recovery`,
`internal/resume`, `internal/eval`, `internal/runtrace`) and the
`cmd/deceleval-*` → `evaltarget` chain. These are also the UNVERIFIED paths, so
the missing tests are consistent with the missing trace.

## Host-specific runtime evidence

- Environment variables that shape this boundary:
  `SOP_AGENT_COMMAND` / `AGENT_SDLC_AGENT_COMMAND`, `SOP_SELECTED_CLASS`,
  `SOP_OLLAMA_BASE_URL`, `SOP_OLLAMA_MODEL`, `SOP_OLLAMA_TIMEOUT`,
  `SOP_WORKSPACE_ROOTS`, `SOP_TOOL_AUDIT_LOG`, `SOP_OLLAMA_TRACE_LOG`,
  `SOP_COMMAND_EVIDENCE_LOG`, `SOP_OLLAMA_AGENT_BIN`,
  `SOP_OLLAMA_AGENT_HOME`, and the start-work job variables (`JobEnvDir`,
  `JobEnvID`, `JobEnvProject`). Read by `agent.EnvAgentCommand`,
  `agentbin.Candidates`, `ollamaagent.ConfigFromEnv`, and
  `cli.launchWorkProcess`.
- OS isolation on the evaltarget path depends on host tooling: `bwrap`
  (bubblewrap) on Linux and `/usr/bin/sandbox-exec` on macOS. If that tooling is
  absent, isolation is not available.
- Cargo permission: project config `cargo.agent_tool` → `config.CargoAgentTool`
  → `toolharness.Config.CargoAgentToolEnabled`.
- Git and GitHub effects rely on the host's Git credential helper or SSH agent
  and on `gh` authentication. No SOP-managed credential store was observed on
  the inspected paths.
- Linux Bubblewrap isolation suite: **Linux/historical only.** Recorded PASS
  (27.7 s, operator-observed) on the Linux source host at baseline `0750e8be`,
  with
  `SOP_REPLAY_ISOLATION_REQUIRED=1 go test ./internal/dqharness/ -run 'TestIsolatedReplay|TestReplayIsolationRequiredEnv' -count=1 -timeout 240s -v`,
  including the Linux-only `TestIsolatedReplayUnixSocket` and
  `TestIsolatedReplayToolchainProcAndTemp`. It is not evidence for the current
  Mac checkout and this report does not infer a macOS result from it.
- Current Mac host facts observed during the AGENCY-003 run: the platform was
  observed indirectly as `darwin` from the Go toolchain in the current checkout;
  `uname -srm` and `go version` were not permitted by the command allow-list, so
  the exact kernel string and Go version are not recorded. `bwrap` is a Linux
  tool and was not observed. A direct `/usr/bin/sandbox-exec` presence probe was
  not run; the evaltarget Darwin isolation code path exists
  (`internal/evaltarget/isolation_darwin.go`), but no macOS isolation-suite
  result was observed in this run. Only `internal/evaltarget` (`0.662s`) and
  `internal/dqharness` (cached) are covered by `go test ./...`; no replay
  isolation result was produced here. **The evaltarget macOS isolation presence
  on this host is NOT demonstrated by a run in this checkout.**
- Normal agent execution is not sandboxed and carries no OS isolation on this
  host. Only the evaltarget child path declares OS isolation, and that path was
  not exercised against the OS sandbox in this run.
- The historical Linux `go test ./...` run failed by timeout because of host
  storage failure; that failure is incomplete verification, not an
  implementation finding, and it is not a current-checkout result. The current
  Mac `go test ./...` result is recorded in §Verification record below.

## Summary findings

1. Every tool-agent effect passes one policy implementation in
   `internal/toolharness`. Denials are sentinel-classified and audited.
   AGENCY-002 added production-boundary regression tests
   (`internal/approval/agency002_binding_test.go`,
   `internal/toolharness/agency002_contract_test.go`) that drive the production
   `approval.Service` and harness rather than duplicates.
2. The full process environment (and any credentials in it) is inherited by
   every command sink on entry points 1-5 and by the `sh -c` external agent
   command, the `testrunner.execute` shell, and the MCP `sop_start_work` child.
   Only `evaltarget.RunChild` uses an allow-list environment.
3. OS isolation was observed only on the evaltarget child path (`bwrap` /
   `sandbox-exec`). Entry points 1-5 and the configured external-agent `sh -c`
   shell are **policy-only**. Normal agent processes are **not sandboxed**. The
   MCP `sop_start_work` child is policy-only too. No normal agent execution is
   claimed as sandboxed.
4. Git write effects and push/PR/merge are owned by `internal/git` and
   `internal/github`, gated by the SOP workflow, not by the harness. The harness
   correctly refuses destructive Git.
5. Ungoverned or policy-only external-agent paths are named here and are never
   described as sandboxed.
   - The configured external-agent shell (`agent.CommandAgent.Generate`, driven
     by `SOP_AGENT_COMMAND` / `AGENT_SDLC_AGENT_COMMAND`) runs
     `sh -c` with the inherited environment and full filesystem reach. It is
     **policy-only**: the command string is trusted operator configuration and
     no `commandpolicy.Classify` call runs on it. Model tool calls the inner
     loop makes are gated by the tool harness (entry point 1); the outer shell
     is not.
   - The MCP handler set (`internal/mcp.Server` + `cli.runMCP` registration) is a
     fixed registered-tool server with a name gate (`mcp.Server.find`); the
     concrete handler set is enumerated in entry point 5b (default, status-only,
     planner, and opt-in start-work). The handlers are **policy-only**: access is
     enforced by registration, and the `sop_start_work` child runs the same
     `sop` binary with no OS isolation. This report does not claim any MCP
     handler is sandboxed.
   - The recovery/resume paths (`internal/recovery`, `internal/resume`, and the
     `internal/eval` / `internal/runtrace` / `cmd/deceleval-*` →
     `evaltarget.RunChild` chain) are **UNVERIFIED**: those packages were not
     traced line-by-line, so none of them is described as sink-free or
     sandboxed. A search for `os/exec` found no command site in
     `internal/recovery` or `internal/resume`, but a resumed workflow is only
     expected, not proven, to re-enter the tool harness and the configured agent
     command. Only `evaltarget.RunChild` carries OS isolation.
   The planner path funnels through the configured agent adapter and validates
   its output deterministically.
6. The `internal/testrunner` shell sink is policy-free by design and trusts the
   caller that supplies the command string. Its callers are `validate.Run`
   (from `cli.timedValidation` / `cli.sessionValidation`, `cli.task`,
   `cli.mcpValidate`, and `sop validate`), and each supplies `cfg.Validation` or
   an operator-provided `config.Validation`; the acceptance path instead uses
   `internal/cli/outcome.go`'s argv-only `commandpolicy` gate. No caller builds
   the string from model or request data on the inspected paths.

## Remaining gaps

Each item names a concrete follow-up and the dependent work it stops. None of
these gaps blocks AGENCY-003 verification; the required `go build ./...`,
`go test ./...`, and `go vet ./...` all pass on this checkout.

1. `internal/recovery`, `internal/resume`, `internal/eval`, `internal/runtrace`,
   and `cmd/deceleval-*` are not traced to `evaltarget.RunChild` or to a plain
   `exec.Command`. **Proposed follow-up:** trace each `main` and each resume
   entry to its command sink, and add a focused sink-assertion test. **Stops:**
   Plan 02's live replay cases and any claim that the replay path is
   sandbox-free. It does not stop entry points 1-5, 7 or AGENCY-003.
2. `internal/commitgate` and `internal/mergegate` are not inspected, so commit
   and merge approval argument ownership is unproven. **Proposed follow-up:**
   inspect both gates and record which layer owns the branch name and `gh`
   arguments. **Stops:** only the merge-gate adversarial Plan 02 cases.
3. The scheduler-to-`internal/validate` chain was not followed line-by-line.
   The `testrunner` callers themselves are traced (`validate.Run` ←
   `cli.timedValidation`/`sessionValidation`, `cli.task`, `cli.mcpValidate`,
   `sop validate`) and pass configured command strings. **Proposed follow-up:**
   trace the scheduler entry into `internal/validate` and add a focused test
   that asserts the caller supplies `config.Validation`. **Stops:** only the
   scheduler-driven validation Plan 02 cases.
4. `agent.openai` / `agent.openaicompatible` / `agent.mlx` credential variable
   handling is UNVERIFIED. **Proposed follow-up:** read the provider
   constructors and record the exact env keys. **Stops:** only the
   provider-credential Plan 02 cases.
5. `internal/github` tests were not enumerated. **Proposed follow-up:** list
   them and map each to a `CommandClient` effect. **Stops:** only the
   GitHub-effect Plan 02 cases.
6. macOS evaltarget isolation presence was not demonstrated by a run in this
   checkout. **Proposed follow-up:** run
   `go test ./internal/dqharness/ -run 'TestIsolatedReplay|TestReplayIsolationRequiredEnv' -count=1`
   on this Mac with `SOP_REPLAY_ISOLATION_REQUIRED=1` and record the result.
   **Stops:** only the macOS replay-isolation claim. It does not change the
   finding that normal agent execution is policy-only.

## Verification record

Scope: AGENCY-003. The commands below were run once from the repository root on
this host. No command was skipped, shortened, re-timed, or weakened.

Platform: macOS (`darwin`) on Apple silicon. The command allow-list in this
run rejected `uname -srm` and `go version` as `REQUIRES_APPROVAL`, so the exact
kernel string and Go toolchain version are **not recorded**; the platform is
shown as `darwin` and the Go module builds and tests natively.

Base and head SHAs:

- Base SHA (`git rev-parse origin/main`):
  `aa2321754d90bc67ee5eced332295089c48de841`
- Head SHA (`git rev-parse security/llm03-agency`):
  `32982315dc6b9a96f40f76c5188b33b10fc85a49`

Verification working tree: the run above executed on the working tree that
produced this PR, with the AGENCY-002 regression tests and this AGENCY-003
documentation uncommitted at head `32982315`; the PR commit follows that head.

Commands and results:

| Command | Exit status | Result |
|---|---|---|
| `go build ./...` | 0 | PASS |
| `go vet ./...` | 0 | PASS |
| `go test ./...` | 0 | PASS |

`go test ./...` result: **PASS** for this Mac at the head SHA above. Every
package reported `ok`, or `[no test files]` for `cmd/*` entry points. Cached
results are reported as `(cached)` by Go; two packages rebuilt in this run:
`internal/archtest` 0.508 s, `internal/evaltarget` 0.662 s,
`internal/retrievalgate` 4.524 s, `internal/runmetrics` 0.149 s. No package
failed. This result is distinct from the historical Linux result at
`0750e8be` that failed by host storage timeout.

Focused verification scope: `go test ./...` covers the focused packages named by
the inventory, including `internal/toolharness`, `internal/agent`,
`internal/approval` (with `agency002_binding_test.go`),
`internal/toolharness` (with `agency002_contract_test.go`), `internal/git`,
`internal/github`, `internal/verify`, `internal/mcp`, `internal/planner`,
`internal/evaltarget`, `internal/commandpolicy`, and `internal/archtest`. Each
is reported `ok` in the run above. No separate focused invocation was run
beyond `go test ./...`; that single full run covers each named package and no
package is omitted.

Remaining gaps: see §Remaining gaps. Each gap there names a concrete follow-up
and the dependent work it stops. No remaining gap blocks unrelated work or the
verification above.

PR link: [#270](https://github.com/imhttran/agentic-sop/pull/270) (draft; not
merged). The implementation PR is opened by the SOP workflow after this stage;
this field records the opened draft PR.

## Reusable production seams for Plan 02

- **evaltarget observer.** `evaltarget.RunChild` plus `evaltarget.IsolateEnv`,
  `evaltarget.confinedDir`, and the per-platform `isolation_linux.go` /
  `isolation_darwin.go` / `isolation_other.go` files are the one path with OS
  isolation today. Plan 02 can observe a live path by routing cases through it.
- **Production authorization controls.** `commandpolicy.Classify` /
  `CargoClassify` and `toolharness.Harness.checkCommandIn` are the command
  authority; `toolharness.Harness.resolveIn` / `resolveArg` are the path
  authority; `mcp.Server.find` is the tool-name authority;
  `internal/approval.Service` is the human gate. Plan 02 must exercise these,
  not a duplicate policy.
- **Inert effect boundary.** `internal/testrunner.execute`,
  `agent.CommandAgent.Generate`, `git` / `github` `execRunner`, and
  `cli.launchWorkProcess` are the non-isolated effect sinks. Plan 02 negative
  cases need inert sinks (a local remote, a fake launcher, a `sh -c` no-op) so a
  denial is provable with no side effect at the governed sink.

## Optional targeted follow-up

The AGENCY-003 verification scope is the three commands in §Verification
record: `go build ./...`, `go vet ./...`, and `go test ./...`. Narrower
invocations are optional and were not needed here because the full run passed:

- `go test ./internal/toolharness/ ./internal/ollamaagent/ ./internal/git/`
- `go test ./internal/verify/ ./internal/mcp/ ./internal/planner/ ./internal/evaltarget/`
- `go test ./internal/cli/ -run 'TestMCP|TestStartWork'`
