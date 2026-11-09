# OWASP LLM06 — Unbounded consumption: SOP execution limits

Status: VERIFIED for the changed limits; residual gaps recorded below.
Plan: `docs/plans/PLAN-06-SOP-LLM06-Unbounded-Consumption.md` (`plan-06-sop-llm06-unbounded-consumption`).
Base commit: `87d68ad93682a71a3d222f4eba438cde1fe52ec4` (`origin/main` after refresh).
Code/verification commit: `377ad4f29e80b16e8b5da5d3fe94d5c33a85276e`.
Platform: macOS `darwin/arm64`, Go `go1.27.1`.
Configuration identity: `.agent-sdlc/config.yaml` — harness `tool`, provider `ollama`, model `deepseek-v4.1-flash:cloud`, validation `go build ./...` / `go test ./...` / `go vet ./...`, review `self`, quality `max_fix_cycles: 3`, autonomy `balanced`, `approval_before_commit: true`.

This is an article-based OWASP taxonomy report, not a formal compliance
certification. It records what the current code and tests prove, and what they do
not.

## How to read this report

| Label          | Meaning                                                                           |
| -------------- | --------------------------------------------------------------------------------- |
| IMPLEMENTED    | The control or fix exists in the named source path.                               |
| ENABLED        | The control is active under the current configuration.                            |
| VERIFIED       | A named deterministic command or test produced current evidence.                  |
| GAP            | The control does not exist, is partial, or is not enforced where it claims to be. |
| NOT_RUN        | The evidence was not collected.                                                   |
| NOT_APPLICABLE | The current contract defines no such budget or reservation.                       |

`ALREADY_SATISFIED` is used only where current code and tests demonstrate the
behavior without a change. A deterministic test is not live-provider evidence. A
local timeout is not proof that provider-side billing stopped.

## Execution context

`sop run` compiled the plan and created tasks `BUDGET-001`, `BUDGET-002`, and
`BUDGET-003`. The in-process Ollama IMPLEMENT invocation produced no governed
mutation; SOP classified it `IMPLEMENT_NO_CHANGES` / `CONTINUE` twice, then
stopped at the per-run continuation limit (`SOP_RUN_MAX_CONTINUES=1` default).
The run artifacts are under `.agent-sdlc/runs/BUDGET-001/` (`report.md`,
`implementation.md`, `diff.patch`, `state.json`). The deterministic analysis,
fixes, and tests below were then implemented and verified at the code commit
above; SOP state remains authoritative and this report does not claim the model
invocation completed the tasks.

## 1. Limit inventory

Units: `s` = wall-clock seconds; `turns` = model turns; `calls` = provider or tool
calls; `bytes` = captured output; `count` = lifecycle attempts.

| Control                     | Unit                                              | Owner / override authority                                                                                                     | Default                                                        | Enforcement point                                                   | Persistence                          | Possible overshoot                                                          |
| --------------------------- | ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------- | ------------------------------------------------------------------- | ------------------------------------ | --------------------------------------------------------------------------- |
| IMPLEMENT iteration ceiling | turns / invocation                                | `internal/budget`; operator env `SOP_OLLAMA_IMPLEMENT_ITERATIONS`                                                              | 32                                                             | `internal/ollamaagent` phased loop (`policyFor`, `executePhased`)   | Not persisted; resets per invocation | One in-flight model turn; resets on retry/continuation                      |
| FIX iteration ceiling       | turns / invocation                                | `internal/budget`; `SOP_OLLAMA_FIX_ITERATIONS`                                                                                 | 24                                                             | same phased loop                                                    | Not persisted                        | same                                                                        |
| Stale-iteration guard       | turns / invocation                                | `internal/budget`; `SOP_OLLAMA_STALE_ITERATIONS`                                                                               | 5                                                              | phased loop; `failure` no-progress classification                   | Not persisted                        | Stops at the guard; one bounded recovery may resume from checkpoint         |
| Tool-call limit             | calls / invocation                                | `internal/budget`; `SOP_OLLAMA_TOOL_CALLS`                                                                                     | 80                                                             | checked before each tool execution (`executeLoop`, `executePhased`) | Not persisted                        | One tool call per check point                                               |
| Discovery allowance         | turns / invocation                                | `internal/budget`; `SOP_OLLAMA_DISCOVERY_ITERATIONS`, clamped `[1,64]`                                                         | 12                                                             | `discoveryTurns` / convergence enforcement                          | Not persisted                        | Soft steering only; the hard ceiling still applies                          |
| Capability ceilings         | turns / invocation                                | `internal/ollamaagent/policy.go`                                                                                               | PLAN 11, REVIEW 10, DESIGN_TESTS 12, DIAGNOSE 12, default 12   | capability loops                                                    | Not persisted                        | Early completion is normal; the ceiling is a stop                           |
| Synthesis correction bound  | corrections / invocation                          | `internal/ollamaagent/policy.go`                                                                                               | 2                                                              | two-phase engine                                                    | Not persisted                        | Bounded                                                                     |
| Orchestration envelope      | count / run                                       | `internal/orchestration`; `SOP_ORCH_*`                                                                                         | workers 8, active 4, assignments 16, integrations 4, replans 3 | pre-dispatch, worker pool, INTEGRATE, replan                        | In-memory envelope                   | One integration/replan in flight                                            |
| Command timeout             | s / command                                       | `toolharness.Config`; `ollamaagent.Config`; controller config                                                                  | 120 s (toolharness/ollamaagent); 15 m (controller)             | `execCaptured` / command context                                    | Not persisted                        | Direct process may take teardown time; descendants are a GAP (see §2.5)     |
| Tool output bound           | bytes / tool call                                 | `toolharness.Config.MaxOutputBytes`                                                                                            | 256 KiB                                                        | `boundedCapture` and `readFileBounded` (read boundary)              | Not persisted                        | The retained bytes are bounded; the child may still write until the timeout |
| Provider request timeout    | s / model call                                    | `internal/agent/provider.go`; `SOP_OLLAMA_TIMEOUT`, `SOP_LLAMACPP_TIMEOUT`, `SOP_MLX_TIMEOUT`, `SOP_OPENAI_COMPATIBLE_TIMEOUT` | 120 s                                                          | `http.Client.Timeout`                                               | Not persisted                        | A provider may continue billing after the client cancels                    |
| Provider response bound     | bytes / model call                                | `internal/agent/provider.go`                                                                                                   | 10 MiB                                                         | `io.LimitReader` in `postJSON`                                      | Not persisted                        | The read is bounded; provider generation is not                             |
| Provider retry bound        | attempts / chat turn                              | `internal/ollamaagent/ollama.go`                                                                                               | 3 (initial + 2)                                                | `chatWithFormat` before any tool executes                           | Not persisted                        | Retries multiply provider calls but are fixed and bounded                   |
| ACP frame bound             | bytes / frame                                     | `internal/acp`                                                                                                                 | 1 MiB                                                          | transport frame reader                                              | Not persisted                        | Oversize frame is rejected                                                  |
| ACP prompt bound            | bytes / submission                                | `internal/acp`                                                                                                                 | 256 KiB                                                        | `PromptSubmission`                                                  | Not persisted                        | Rejected                                                                    |
| ACP cancel bound            | s / cancel                                        | `internal/acp`                                                                                                                 | 30 s (`CancelStopBound`)                                       | cancel seam                                                         | Not persisted                        | Host may not acknowledge within the bound                                   |
| ACP output excerpt          | bytes / frame                                     | `internal/acp`                                                                                                                 | 16 KiB                                                         | `MaxOutputChunkBytes`                                               | Not persisted                        | Excerpt is cut, not the underlying work                                     |
| MCP request frame           | bytes / message                                   | `internal/mcp`                                                                                                                 | **none**                                                       | `json.Decoder` on the stream                                        | Not persisted                        | **GAP**: an oversized frame can allocate unboundedly                        |
| Command agent call          | s / call                                          | `internal/agent/command.go`                                                                                                    | **none**                                                       | caller context only                                                 | Not persisted                        | **GAP**: an operator command can run forever                                |
| Command agent output        | bytes / call                                      | `internal/agent/command.go`                                                                                                    | **none**                                                       | `bytes.Buffer` stdout/stderr                                        | Not persisted                        | **GAP**: full output is retained                                            |
| Security adapter timeout    | s / adapter                                       | `internal/config/security.go`                                                                                                  | 300 s                                                          | security runner                                                     | Not persisted                        | Adapter process; descendants follow the same process-group caveat           |
| Security adapter output     | bytes / adapter                                   | `internal/config/security.go`                                                                                                  | 1 MiB                                                          | adapter capture                                                     | Not persisted                        | Bounded                                                                     |
| Task attempt budget         | count / task                                      | `internal/domain.DefaultRetryPolicy`; `max_attempts`                                                                           | 3                                                              | `Task.Requeue` / `MaxAttempts`                                      | `state.db`                           | `sop retry --force` deliberately raises the budget; operator action         |
| Continuation budget         | count / task                                      | `internal/autonomy`; `SOP_AUTONOMY_MAX_CONTINUATIONS`                                                                          | 4                                                              | `continuation.Consume`                                              | `continuations.txt` per task         | `SOP_RUN_MAX_CONTINUES` (default 1) bounds one run                          |
| Fix-cycle budget            | cycles / task                                     | `quality.max_fix_cycles`                                                                                                       | 3                                                              | quality gate                                                        | `state.json` / run artifacts         | Exhaustion yields `NEEDS_HUMAN`, not a loop                                 |
| DQ harness limits           | decisions / requests / retries / elapsed / tokens | `internal/dqharness`                                                                                                           | 180 / 360 / 180 / 90 min / 400000                              | pre-request `CanRequest` / `CanRetry`                               | `usage.json`                         | Tokens only when reported; unreported per-request usage is a GAP            |
| Hipcompare budget           | elapsed / tokens / retries                        | `internal/hipcompare`                                                                                                          | 200000 tokens; caller time; automatic retries                  | `Run` before execution                                              | result artifact                      | `TokensReported` distinguishes unknown from zero                            |
| Monetary budget             | currency                                          | **none**                                                                                                                       | —                                                              | —                                                                   | —                                    | **GAP**: no pricing or pre-dispatch reservation                             |

### 1.1 Per-call, per-attempt, per-job, and shared

- **Per-call**: provider timeout/response bound; tool command timeout/output bound;
  ACP frame/prompt/cancel; security adapter timeout/output.
- **Per-attempt**: invocation iteration ceiling, stale guard, tool-call limit,
  capability ceilings, provider retry bound, fix-cycle budget.
- **Per-job/task**: `max_attempts`, `max_continuations`, `quality.max_fix_cycles`,
  orchestration envelope.
- **Shared/aggregate**: none for execution tokens or steps across attempts and
  continuations. Each attempt/continuation gets a fresh invocation budget by
  design. The evaluation harnesses (`dqharness`, `hipcompare`) do have whole-run
  request/token/time limits, but they are separate evaluation paths, not the
  normal task executor.

### 1.2 Model-controlled and caller-supplied inputs

- `SOP_TASK_INPUTS` is capped at 32 KiB and contains observation text only
  (`internal/cli/task_input.go`). It cannot set a budget.
- CLI flags are `--max-tasks`, `--model-class`, and a plan/task path; none sets a
  token, step, or byte limit.
- MCP request fields are tool names and arguments; only registered tools exist,
  and none exposes a consumption parameter. MCP frame size is a GAP.
- ACP prompt text and frames are bounded; the prompt cannot set a limit.
- Model tool arguments are validated by `internal/toolharness` (path roots,
  command policy, protected state DB). The model cannot set iteration, tool-call,
  output, timeout, attempt, or continuation limits.
- Provider request construction (`chatRequest`) carries no `max_tokens`,
  `num_predict`, or sampling parameter. There is therefore no model-supplied
  output-token value to clamp; there is also no operator-owned output-token cap at
  dispatch (GAP, §2.5).

## 2. Reproduced defects and repairs

### 2.1 Command output was truncated only after full allocation

`internal/toolharness/harness.go` previously captured stdout+stderr into an
unbounded `bytes.Buffer` and called `truncate` after the process exited. A command
that streamed output could therefore allocate its entire output before the
configured `MaxOutputBytes` was applied.

Fix: `internal/toolharness/capture.go` adds `boundedCapture`, which retains at
most the configured bytes while counting the full stream and preserving the exact
`… [truncated N bytes]` marker. `execCaptured` now writes into it.

Evidence:

- `TestBoundedCaptureKeepsOnlyTheBound` — retained bytes never exceed the bound.
- `TestBoundedCaptureBoundaryCases` — just below, at, and above the bound.
- `TestExecCapturedBoundsStreamedOutput` — a child writing 4096 bytes with a
  1024-byte bound returns only the bounded prefix plus `truncated 3072 bytes`.

### 2.2 `read_file` read the whole file before truncating

`internal/toolharness/files.go` previously called `os.ReadFile` and then
truncated. Fix: `readFileBounded` opens the file and reads at most `limit+1`
bytes, so a large file is never fully allocated. For a regular file the stat size
supplies the exact truncated count; for a growing or special file the marker
reports the cut without claiming an unmeasured total.

Evidence: `TestReadFileBoundsBeforeAllocation` — a file four times the bound is
read with a result length near the bound and `truncated 192 bytes`.

### 2.3 Missing provider usage was recorded as zero

`internal/perf.Usage` previously recorded every call with `Input`/`Output` values;
a provider that omitted usage produced a zero-token call that was indistinguishable
from a measured zero. The Ollama and OpenAI-compatible clients also decoded the
usage fields into non-pointer integers, so absence and zero were identical.

Fix: `perf.Tokens` carries `UnreportedCalls`; `AddUsageWithReport` records whether
the server reported usage; `Task.Line` renders `tokens unknown` when every call is
unreported and labels partial reporting; `WriteTokens` has an `unreported` column.
The three client paths now decode usage into pointers and call
`AddUsageWithReport` only when both counts are present.

Evidence:

- `TestClientsReportAbsentUsageAsUnknown` — both providers produce
  `UnreportedCalls: 1`, not zero tokens.
- `TestUnreportedUsageIsNotZero` — the task line says `tokens unknown` and the
  report has the unreported column.
- `TestClientsReportTokenUsage` — reported counts still render exactly as before.

### 2.4 A non-positive output bound disabled the bound

`truncate` returned the full string when its bound was `<= 0`. `toolharness.New`
now normalizes a non-positive `MaxOutputBytes` to the built-in default, so a
missing or malformed value cannot disable the bound. `newBoundedCapture` and
`readFileBounded` also keep the default as a defensive fallback.

Evidence: `TestBoundedCaptureBoundaryCases` (defaulted case).

### 2.5 Reproduced but not repaired (documented gaps)

The following are existing controls or paths that do not meet a claimed boundary.
They were not silently fixed with a new numeric policy.

1. **No output-token cap at dispatch.** `chatRequest` has no `max_tokens` /
   `num_predict`; the provider may generate unbounded tokens while the client
   reads at most 10 MiB. The byte bound is enforced; provider-side generation and
   billing are not. Adding a cap is a policy decision.
2. **Command agent has no timeout or output bound.** `internal/agent/command.go`
   runs `sh -c` with the caller context and unbounded `bytes.Buffer` stdout/stderr.
   A configured command that hangs or streams will consume without a bound.
3. **MCP frames are unbounded.** `internal/mcp` uses `json.Decoder` on the stdio
   stream with no frame size, unlike ACP's 1 MiB `MaxFrameSize`.
4. **Toolharness undo snapshot can read a whole file.** `snapshot` uses
   `os.ReadFile` to preserve a file before a mutation. That is required for
   `restore_file` correctness, but a very large file is fully allocated. A bound
   would need an explicit restore-policy decision.
5. **Descendants may outlive the command timeout.** `execCaptured` uses
   `exec.CommandContext`; the direct process is killed on timeout, but the tool
   harness does not put the command in its own process group or kill the group.
   `internal/evaltarget` does (`TestTimeoutTerminatesChildGroup`,
   `TestCancellationTerminatesChildGroup`). A tool-harness fix was prototyped and
   reverted because `sop run` does not install a signal handler: a separate
   process group could survive an unhandled SIGINT and orphan the child. The
   recommended follow-up is to add graceful signal cancellation to the CLI first,
   then reuse the evaltarget process-group pattern.
6. **Concurrent continuation accounting is not atomic across processes.**
   `RecordContinuations` is a read-modify-write of `continuations.txt`;
   `executionlease` is wired but defaults to `off`, so two concurrent runs could
   both spend the same continuation. `internal/perf.Usage` is mutex-protected
   within a process (`TestUsageSumsCallsByModel`).
7. **DQ harness does not distinguish per-request unreported tokens.** It tracks
   in-flight requests as uncertain after a crash, but `EndRequest(tokens)` records
   a zero when the provider omits usage. `hipcompare` already has
   `TokensReported` and the correct behavior.
8. **No monetary accounting.** There is no pricing table, no pre-dispatch
   reservation, and no hard dollar cap. This report makes no monetary-cap claim.

## 3. Lifecycle outcomes for exhaustion and cancellation

- A harness budget/no-progress diagnostic is classified deterministically:
  `failure.Classify` maps `tool-call limit reached`, `did not complete after`,
  `made no repository change after`, and `no repository progress` to
  `IncompleteImplementation`/`Continue` or `NoProgress`/one bounded `Retry`
  (`internal/failure/failure.go`; `TestToolBudgetExhaustionIsNotAHumanBoundary`,
  `TestHarnessBudgetSignalsAreContinue`, `TestHarnessNoProgressIsBoundedRetry`).
- A no-progress stop is retried at most once from its persisted checkpoint; a
  repeated unchanged signature is refused (`internal/recovery/noprogress.go`;
  `TestUnchangedFailureNotRetried`, `TestExhaustionSingleBlockerReport`).
- The continuation count is persisted before the continuation is allowed, so a
  restart cannot reset it (`internal/continuation/limit.go`;
  `TestContinuationLimitSurvivesRestart`).
- Resume preserves the spent count (`internal/run/run.go`;
  `TestPreflightFailureKeepsContinuationBudget`).
- Timeout/cancellation: the direct command is bounded by the context; the
  evaltarget path kills the whole child group
  (`TestTimeoutTerminatesChildGroup`, `TestCancellationTerminatesChildGroup`).
  The tool-harness descendant gap is recorded in §2.5.5.
- Local cancellation is not evidence that a provider stopped billing.

## 4. Acceptance mapping

### BUDGET-001 — Map limits and their authority

- Per-call/per-attempt/per-job/shared limits: §1 and §1.1.
- Elapsed/tokens/output bytes/monetary: §1; monetary is a GAP.
- Model/caller inputs: §1.2.
- Report delivered: this file.

### BUDGET-002 — Reproduce and repair limit bypasses

- Boundary cases for changed limits: §2.1 (below/at/above), §2.2.
- Model/caller parameters cannot raise operator limits: §1.2. The current request
  carries no output-token value, so dispatch-time clamping is NOT_APPLICABLE; the
  absence of an operator output-token cap is a GAP (§2.5.1).
- Token counting where reported: §2.3. There is no execution token budget or
  token circuit breaker; step and tool-call circuit breakers are the existing
  contract. A token budget is a GAP, not an invented feature.
- Retries/continuations cannot reset an aggregate budget: no aggregate execution
  budget exists (NOT_APPLICABLE). Per-invocation budgets reset by design; the
  attempt and continuation counts are enforced.
- Resume preserves consumed budget: `TestContinuationLimitSurvivesRestart`,
  `TestPreflightFailureKeepsContinuationBudget`.
- Concurrent accounting: in-process `perf.Usage` is mutex-protected
  (`TestUsageSumsCallsByModel`); cross-process continuation accounting is a GAP
  (§2.5.6).
- Timeout/cancellation: direct process bounded; descendant guarantee is a GAP
  (§2.5.5).
- Oversized/streamed output bounded at the read boundary: §2.1, §2.2.
- A normal bounded task still completes: `make check` passes; the boundary tests
  include the just-below case with no truncation.

### BUDGET-003 — Verify terminal behavior and report limits

- Missing usage is unknown, not zero: §2.3.
- No hard dollar-cap claim: §2.5.8.
- Per-call overshoot and unsupported provider behavior: §1 and §2.5.
- Focused and required checks pass: §5.

## 5. Verification commands and results

All commands ran in the isolated worktree on `darwin/arm64`.

| Command                                                                                                                                                                                                                                                                               | Result                                       |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------- |
| `go build ./...`                                                                                                                                                                                                                                                                      | PASS                                         |
| `go vet ./internal/toolharness/... ./internal/perf/... ./internal/agent/... ./internal/ollamaagent/...`                                                                                                                                                                               | PASS                                         |
| `go test -count=1 ./internal/toolharness/...`                                                                                                                                                                                                                                         | PASS (`ok`, 8.7 s)                           |
| `go test -count=1 ./internal/perf/... ./internal/agent/... ./internal/ollamaagent/... ./internal/failure/... ./internal/recovery/... ./internal/continuation/... ./internal/autonomy/... ./internal/run/... ./internal/domain/... ./internal/dqharness/... ./internal/hipcompare/...` | PASS (all packages `ok`; ollamaagent 43.9 s) |
| `make check` (`go fmt`, `go vet`, `go test ./...`, `go build ./...`, plugin check)                                                                                                                                                                                                    | PASS                                         |

The focused lifecycle suite is the evidence for exhaustion/terminal behavior;
the toolharness and perf/agent suites are the evidence for the changed read and
usage bounds. No live model call was required for these checks.

## 6. Residual limitations and follow-up proposals

1. Add an operator-owned output-token bound to provider requests (for example
   `num_predict` for Ollama and `max_tokens` for OpenAI-compatible) and enforce it
   before dispatch. This is a new numeric policy, so it needs an operator decision.
2. Add graceful SIGINT/SIGTERM cancellation to `sop run`/`sop prompt`, then apply
   the evaltarget process-group termination pattern to the tool harness.
3. Bound MCP input frames (reuse the ACP `MaxFrameSize` pattern) and the command
   agent's stdout/stderr.
4. Decide a restore policy for oversized snapshots (skip snapshot + refuse
   restore, or a bounded snapshot with an explicit incomplete-restore error).
5. Make continuation accounting atomic, or enable the existing execution lease
   (`execution.lease: required`) for projects that can run concurrently.
6. Extend `dqharness` with a reported/unreported token distinction equivalent to
   `hipcompare.TokensReported`.
7. If a monetary cap is required, add pricing and a pre-dispatch reservation that
   fails before the request; do not label the current byte/time bounds as a spend
   cap.
8. Provider-side billing may continue after a local timeout or cancellation; only
   a provider-side stop or verified billing report can establish that it stopped.

## 7. PR and handoff

- Branch: `security/llm06-unbounded-consumption`.
- Base: `87d68ad93682a71a3d222f4eba438cde1fe52ec4`.
- Code/verification commit: `377ad4f29e80b16e8b5da5d3fe94d5c33a85276e`.
- PR: opened from this branch; the final PR URL is recorded in the handoff
  response and the pull request itself.
