# OWASP LLM06 — Unbounded consumption: SOP execution limits

Status: `VERIFIED` for the merged execution controls on macOS and Linux CI; the
validation-runner output fix is on the follow-up branch; the provider
output-token cap now defaults to 16384 tokens per request, and the built-in
provider adapters reject a provider-reported `length` truncation instead of
accepting it as a complete response.
Plan: `docs/plans/PLAN-06-SOP-LLM06-Unbounded-Consumption.md` (`plan-06-sop-llm06-unbounded-consumption`).
Base commit: `360b96ef721526e58b89f7a7986849caa8a96080` (`origin/main` after the follow-up rebase).
Merged code/verification commits: `400e4544`, `c8e00b1c`, `e656a8e7`.
Follow-up code commits: `cb230d66ffad174f3040944876d6c0f7b9c7c730` (validation output bounds) and `e053d18e9252ef9d821b2648accca1967e04f0f8` (retain trailing structured evidence).
Platform: macOS `darwin/arm64` locally; Linux verified by GitHub Actions `ubuntu-26.04` CI.
Configuration identity: `.agent-sdlc/config.yaml` — harness `tool`, provider `ollama`, model `deepseek-v4.1-flash:cloud`, validation `go build ./...` / `go test ./...` / `go vet ./...`, review `self`, quality `max_fix_cycles: 3`, autonomy `balanced`, `approval_before_commit: true`.

This is an article-based OWASP taxonomy report, not a formal compliance
certification. It records what the current code and tests prove, and what they do
not.

## How to read this report

| Label | Meaning |
|---|---|
| IMPLEMENTED | The control or fix exists in the named source path. |
| ENABLED | The control is active under the current configuration. |
| VERIFIED | A named deterministic command or test produced current evidence. |
| GAP | The control does not exist, is partial, or is not enforced where it claims to be. |
| NOT_RUN | The evidence was not collected. |
| NOT_APPLICABLE | The current contract defines no such budget or reservation. |

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
fixes, and tests below were implemented and verified at the code commit above;
SOP records were reconciled through `sop task complete --external`, which re-ran
validation, and this report does not claim the model invocation completed the
tasks.

## 1. Limit inventory

Units: `s` = wall-clock seconds; `turns` = model turns; `calls` = provider or tool
calls; `bytes` = captured output; `count` = lifecycle attempts.

| Control | Unit | Owner / override authority | Default | Enforcement point | Persistence | Possible overshoot |
|---|---|---|---|---|---|---|
| IMPLEMENT iteration ceiling | turns / invocation | `internal/budget`; `SOP_OLLAMA_IMPLEMENT_ITERATIONS` | 32 | `internal/ollamaagent` phased loop | Not persisted; resets per invocation | One in-flight model turn; resets on retry/continuation |
| FIX iteration ceiling | turns / invocation | `internal/budget`; `SOP_OLLAMA_FIX_ITERATIONS` | 24 | same phased loop | Not persisted | same |
| Stale-iteration guard | turns / invocation | `internal/budget`; `SOP_OLLAMA_STALE_ITERATIONS` | 5 | phased loop; `failure` no-progress classification | Not persisted | Stops at the guard; one bounded recovery may resume from checkpoint |
| Tool-call limit | calls / invocation | `internal/budget`; `SOP_OLLAMA_TOOL_CALLS` | 80 | checked before each tool execution | Not persisted | One tool call per check point |
| Discovery allowance | turns / invocation | `internal/budget`; `SOP_OLLAMA_DISCOVERY_ITERATIONS`, clamped `[1,64]` | 12 | `discoveryTurns` / convergence enforcement | Not persisted | Soft steering only; the hard ceiling still applies |
| Capability ceilings | turns / invocation | `internal/ollamaagent/policy.go` | PLAN 11, REVIEW 10, DESIGN_TESTS 12, DIAGNOSE 12, default 12 | capability loops | Not persisted | Early completion is normal; the ceiling is a stop |
| Synthesis correction bound | corrections / invocation | `internal/ollamaagent/policy.go` | 2 | two-phase engine | Not persisted | Bounded |
| Orchestration envelope | count / run | `internal/orchestration`; `SOP_ORCH_*` | workers 8, active 4, assignments 16, integrations 4, replans 3 | pre-dispatch, worker pool, INTEGRATE, replan | In-memory envelope | One integration/replan in flight |
| Command timeout | s / command | `toolharness.Config`; `ollamaagent.Config`; controller config | 120 s (toolharness/ollamaagent); 15 m (controller) | `execCaptured`; process-group kill on cancellation/timeout | Not persisted | Direct process plus its group; macOS and Linux CI verified |
| Tool output bound | bytes / tool call | `toolharness.Config.MaxOutputBytes` | 256 KiB | `boundedCapture` and `readFileBounded` (read boundary) | Not persisted | Retained bytes are bounded; the child may write until the timeout |
| Validation command output | bytes / check | `internal/testrunner.Runner.MaxOutputBytes`; canonical context limit for prompt rendering | 10 MiB capture; 64 KiB per stream in the prompt | `limitedBuffer` at read; `boundedValidationStream` before rendering | Not persisted | Bounded at both read and prompt boundaries (follow-up) |
| Provider request timeout | s / model call | `internal/agent/provider.go`; provider-specific timeout envs | 120 s | `http.Client.Timeout` | Not persisted | A provider may continue billing after the client cancels |
| Provider response bound | bytes / model call | `internal/agent/provider.go` | 10 MiB | `io.LimitReader` in `postJSON` | Not persisted | The read is bounded; provider generation is not |
| Provider output-token bound | tokens / request | `agent.DefaultMaxOutputTokens`; `SOP_AGENT_MAX_OUTPUT_TOKENS` overrides | 16384 | request construction before dispatch: `max_tokens` / `options.num_predict` | Not persisted | A provider-reported `length` stop is rejected as incomplete (§2.9) |
| Provider retry bound | attempts / chat turn | `internal/ollamaagent/ollama.go` | 3 (initial + 2) | `chatWithFormat` before any tool executes | Not persisted | Retries multiply provider calls but are fixed and bounded |
| ACP frame bound | bytes / frame | `internal/acp` | 1 MiB | transport frame reader | Not persisted | Oversize frame is rejected |
| ACP prompt bound | bytes / submission | `internal/acp` | 256 KiB | `PromptSubmission` | Not persisted | Rejected |
| ACP cancel bound | s / cancel | `internal/acp` | 30 s (`CancelStopBound`) | cancel seam | Not persisted | Host may not acknowledge within the bound |
| ACP output excerpt | bytes / frame | `internal/acp` | 16 KiB | `MaxOutputChunkBytes` | Not persisted | Excerpt is cut, not the underlying work |
| MCP request frame | bytes / message | `internal/mcp` | 1 MiB (`MaxFrameSize`) | `readFrame` before JSON decode | Not persisted | Oversized frame is drained, answered once with a parse error, then discarded |
| Command agent call | s / call | `internal/agent/command.go`; `SOP_AGENT_COMMAND_TIMEOUT` | 120 s | `context.WithTimeout` around `sh -c`; process-group kill on timeout/cancel | Not persisted | Direct process and its group; macOS and Linux CI verified |
| Command agent output | bytes / call | `internal/agent/command.go` | 10 MiB stdout; 2 KiB stderr diagnostics | `limitedBuffer` at the write boundary | Not persisted | Retained bytes are bounded |
| Security adapter timeout | s / adapter | `internal/config/security.go` | 300 s | security runner | Not persisted | Adapter process; descendant termination is not claimed |
| Security adapter output | bytes / adapter | `internal/config/security.go` | 1 MiB | adapter capture | Not persisted | Bounded |
| Task attempt budget | count / task | `internal/domain.DefaultRetryPolicy`; `max_attempts` | 3 | `Task.Requeue` / `MaxAttempts` | `state.db` | `sop retry --force` deliberately raises the budget; operator action |
| Continuation budget | count / task | `internal/autonomy`; `SOP_AUTONOMY_MAX_CONTINUATIONS` | 4 | `Run.ConsumeContinuation` under the project lock | `continuations.txt` per task | `SOP_RUN_MAX_CONTINUES` (default 1) bounds one run; concurrent spend is serialized |
| Fix-cycle budget | cycles / task | `quality.max_fix_cycles` | 3 | quality gate | `state.json` / run artifacts | Exhaustion yields `NEEDS_HUMAN`, not a loop |
| DQ harness limits | decisions / requests / retries / elapsed / tokens | `internal/dqharness` | 180 / 360 / 180 / 90 min / 400000 | pre-request `CanRequest` / `CanRetry` | `usage.json` | Token counts are deterministic estimates (`ModelDecision`), not provider usage; in-flight requests are tracked as uncertain |
| Hipcompare budget | elapsed / tokens / retries | `internal/hipcompare` | 200000 tokens; caller time; automatic retries | `Run` before execution | result artifact | `TokensReported` distinguishes unknown from zero |
| Monetary budget | currency | **none** | — | — | — | **GAP**: no pricing or pre-dispatch reservation |

### 1.1 Per-call, per-attempt, per-job, and shared

- **Per-call**: provider timeout/response/output-token bound; tool command
  timeout/output bound; command-agent timeout/output; ACP frame/prompt/cancel;
  MCP frame; security adapter timeout/output.
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
  and none exposes a consumption parameter. Frames are bounded to 1 MiB.
- ACP prompt text and frames are bounded; the prompt cannot set a limit.
- Model tool arguments are validated by `internal/toolharness` (path roots,
  command policy, protected state DB). The model cannot set iteration, tool-call,
  output, timeout, attempt, or continuation limits.
- Provider request construction carries no model-supplied output-token value.
  `SOP_AGENT_MAX_OUTPUT_TOKENS` is operator-owned and is copied into the request
  before dispatch; when it is unset, `agent.DefaultMaxOutputTokens` (16384) is sent.

## 2. Reproduced defects and repairs

### 2.1 Command output was truncated only after full allocation

`internal/toolharness/harness.go` previously captured stdout+stderr into an
unbounded `bytes.Buffer` and called `truncate` after the process exited. Fix:
`internal/toolharness/capture.go` adds `boundedCapture`, which retains at most the
configured bytes while counting the full stream and preserving the exact
`… [truncated N bytes]` marker. `execCaptured` writes into it.

Evidence: `TestBoundedCaptureKeepsOnlyTheBound`,
`TestBoundedCaptureBoundaryCases`, `TestExecCapturedBoundsStreamedOutput`.

### 2.2 `read_file` read the whole file before truncating

`readFileBounded` now opens the file and reads at most `MaxOutputBytes+1`, so a
large file is never fully allocated. A regular file's stat size supplies the exact
truncated count; a growing or special file reports the cut without claiming an
unmeasured total.

Evidence: `TestReadFileBoundsBeforeAllocation`.

### 2.3 Missing provider usage was recorded as zero

`perf.Tokens` now carries `UnreportedCalls`; `AddUsageWithReport` records whether
the server reported usage; the Ollama and OpenAI-compatible clients decode usage
into pointers so absence is distinguishable from a measured zero. Task lines
render `tokens unknown` and `WriteTokens` has an `unreported` column.

Evidence: `TestClientsReportAbsentUsageAsUnknown`,
`TestUnreportedUsageIsNotZero`, `TestClientsReportTokenUsage`.

### 2.4 A non-positive output bound disabled the bound

`toolharness.New` normalizes a non-positive `MaxOutputBytes` to the built-in
default; `newBoundedCapture` and `readFileBounded` keep the same fallback.

Evidence: `TestBoundedCaptureBoundaryCases` (defaulted case).

### 2.5 Continuation accounting could overspend

The production path (`continueTask` in `internal/cli/drive.go`) read
`continuations.txt`, incremented it, and wrote it back without a lock. Two
concurrent runs could both spend the same continuation. `continuation.Consume`
had the same read-then-write shape.

Fix: `internal/run/run.go` adds `Run.ConsumeContinuation`, which holds the
project lock across the read, check, and write. `continuation.Consume` uses that
atomic capability when the counter provides it, and `continueTask` now calls it
directly.

Evidence: `TestConsumeContinuationDoesNotOverspendConcurrently`,
`TestContinuationLimitSurvivesRestart`,
`TestPreflightFailureKeepsContinuationBudget`.

### 2.6 Command-agent invocation had no timeout or output bound

`internal/agent/command.go` now bounds one invocation with
`SOP_AGENT_COMMAND_TIMEOUT` (default `120s`) and retains stdout/stderr through
`limitedBuffer` (10 MiB stdout, 2 KiB diagnostics). The command runs in its own
process group, and timeout/cancellation kills that group, so a shell child cannot
hold the output pipe or outlive the bound. A malformed duration fails at
configuration time.

Evidence: `TestCommandAgentTimeoutBoundsInvocation`,
`TestCommandAgentBoundsStdout`, `TestCommandAgentRejectsInvalidTimeoutEnv`,
`TestCommandAgentRespectsCancellation`.

### 2.7 MCP requests were unbounded

`internal/mcp` now reads newline-delimited frames through `readFrame`, bounded by
`MaxFrameSize` (1 MiB). An oversized frame is drained, answered once with a
JSON-RPC parse error, and the stream continues.

Evidence: `TestReadFrameBoundary`,
`TestServeRejectsOversizedFrameAndContinues`.

### 2.8 Cancellation did not reach owned descendants

The tool harness now places a command in its own process group and, on timeout or
context cancellation, kills the entire group (`internal/toolharness/proc_unix.go`,
`proc_other.go`, `execCaptured`). To make SIGINT safe, `sop run` and `sop prompt`
now use `internal/cli/execution_context.go`, which cancels on SIGINT/SIGTERM, and
the lifecycle checks that context before recording recovery or starting another
attempt. This is the complete fix for the condition that caused the earlier
process-group experiment to be reverted.

Evidence on macOS: `TestExecTimeoutKillsProcessGroup`,
`TestExecCancellationKillsProcessGroup`, `TestExecutionContextCancelsOnSignal`.
Linux was verified by GitHub Actions run `42576841956`, job `129026456948`
(`fmt, vet, build, test`, `ubuntu-26.04`, head `dfd135c`): the full `go test ./...`
passed, including the unix-tagged process-group and signal tests. No skip path is
present in those tests.

Local Linux reproduction at base revision `92c996a` (Linux `7.0.0-38-generic`
x86_64, Go `1.27.1`), with the new fixtures added on this branch:

```sh
TMPDIR=/dev/shm/llm06-tmp go test ./internal/toolharness -run 'TestExec|TestCleanupOwnedPID' -count=1
TMPDIR=/dev/shm/llm06-tmp go test ./internal/agent -run TestCommandAgentTimeoutKillsDescendant -count=1
TMPDIR=/dev/shm/llm06-tmp go test ./internal/cli -run TestExecutionContextCancelsOnSignal -count=1
```

All passed. `TestExecNormalCompletionNoDescendants`,
`TestExecTimeoutKillsGrandchild`, `TestExecCancellationKillsGrandchild`, and
`TestExecTimeoutKillsSigtermIgnoringDescendant` record the child and grandchild
PIDs and wait until `syscall.Kill(pid, 0)` reports them gone, so the evidence is
actual termination, not merely that the caller returned.
`TestCleanupOwnedPIDDoesNotSignalDifferentIdentity` proves the cleanup helper
signals a PID only when the recorded `/proc/<pid>/stat` start time still matches,
so a reused PID is not signalled. The unix contract has no graceful escalation
phase: process-group SIGKILL is the contract, and the SIGTERM-ignoring fixture
confirms that a descendant which ignores SIGTERM is still terminated. No
production-code change was required; the existing `Setpgid` plus
`terminateGroup` implementation already covered the grandchild.

### 2.9 Provider output-token bound

The single-shot providers send `max_tokens` (OpenAI-compatible) or
`options.num_predict` (Ollama), and the in-process tool loop sends
`options.num_predict`, before dispatch. The value is `SOP_AGENT_MAX_OUTPUT_TOKENS`
when set, otherwise `agent.DefaultMaxOutputTokens` (16384). The default was chosen
by the operator as the governance decision this report previously left open; it
sits far above a plan or review, including reasoning-model thinking tokens.

Evidence: `TestMaxOutputTokensDefaultsWhenUnset`,
`TestClientsHonorOperatorMaxOutputTokens`,
`TestProviderRejectsInvalidMaxOutputTokens`,
`TestOllamaClientSendsOperatorMaxOutputTokens`.

A provider that stops at that bound reports it explicitly. The adapters preserve
that report instead of discarding it:

- `internal/agent/openai.go` decodes `choices[].finish_reason` for the
  OpenAI-compatible transport shared by `llamacpp`, `mlx`, and
  `openai_compatible`.
- `internal/agent/ollama.go` decodes `done_reason` for the single-prompt Ollama
  provider.
- `internal/ollamaagent/ollama.go` decodes `done_reason` for the in-process tool
  loop and rejects the turn *before* `normalizeToolCalls`, so a partial native or
  content tool call is never dispatched.
- A `length` stop becomes `agent.TruncationError` (`internal/agent/agent.go`), a
  deterministic error: the provider boundary does not retry it. The in-process
  bridge (`internal/ollamaagent/bridge.go`) sets `Response.Truncated` and returns a
  structured `failed` outcome carrying the reason, and
  `internal/orchestration/worker_adapter.go` treats any truncated response as an
  explicit failure. Truncated output therefore cannot establish task success.
- Only the explicit `length` value is truncation. An absent reason and other
  stop reasons keep their existing behavior.
- None of the supported agent adapters streams: every request sets `stream:false`,
  so there is no streaming path in which a stop reason could be lost.

Evidence: `TestOpenAICompatibleRejectsLengthFinishReason`,
`TestOllamaRejectsLengthDoneReason`,
`TestNormalStopReasonsRemainAccepted`, `TestTruncatedToolCallIsNotDispatched`,
`TestTruncatedFinalIsNotAcceptedAsSuccess`, `TestNormalDoneReasonStillCompletes`,
`TestTruncatedPayloadFailsSafely`, `TestTruncationIsABoundedContinuation`.

### 2.10 Validation output was captured and rendered without a bound

`internal/testrunner` captured each check's stdout/stderr in an unbounded
`bytes.Buffer`, and `validationFailureContext` inserted that stream directly into
the model prompt. A streaming validation check could therefore allocate and send
unbounded output.

Fix: `execute` now captures each stream through `limitedBuffer` at a 10 MiB
read-boundary bound, retaining a bounded head and tail so trailing `go test -json`
package-completion events survive a verbose passing check. `validationFailureContext`
bounds each rendered stream to the canonical context limit
(`sopctx.DefaultLimits().MaxBytes`, 64 KiB) with a truncation marker. This is a
capture safety bound, not a configurable quota.

Evidence: `TestRunBoundsCapturedOutput`,
`TestPostgresCompletionSurvivesCaptureBound`,
`TestValidationFailureContextBoundsHugeOutput`.

## 3. Lifecycle outcomes for exhaustion and cancellation

- `failure.Classify` maps `tool-call limit reached`, `did not complete after`,
  `made no repository change after`, and `no repository progress` to
  `IncompleteImplementation`/`Continue` or `NoProgress`/one bounded `Retry`
  (`TestToolBudgetExhaustionIsNotAHumanBoundary`,
  `TestHarnessBudgetSignalsAreContinue`, `TestHarnessNoProgressIsBoundedRetry`).
- A no-progress stop is retried at most once from its persisted checkpoint; a
  repeated unchanged signature is refused (`TestUnchangedFailureNotRetried`,
  `TestExhaustionSingleBlockerReport`).
- The continuation count is persisted before the continuation is allowed and is
  now incremented atomically (`TestContinuationLimitSurvivesRestart`,
  `TestConsumeContinuationDoesNotOverspendConcurrently`).
- Resume preserves the spent count (`TestPreflightFailureKeepsContinuationBudget`).
- A signal cancels the foreground context; the lifecycle returns before recording
  recovery, and the tool harness kills the command's process group on macOS and
  Linux CI.
- Local cancellation is not evidence that a provider stopped billing.

## 3a. Post-merge reconciliation of reported residuals

| Reported residual | Disposition | Evidence / decision |
|---|---|---|
| Default provider output-token cap | Decided by the operator; implemented | Unset `SOP_AGENT_MAX_OUTPUT_TOKENS` now sends `agent.DefaultMaxOutputTokens` (16384). Test: `TestMaxOutputTokensDefaultsWhenUnset`. |
| Provider `length` truncation accepted as complete | Implemented and verified | `finish_reason`/`done_reason` `length` becomes `agent.TruncationError`; the in-process bridge marks `Response.Truncated` and returns a `failed` outcome, and the orchestration mapping refuses success. Tests: `TestOpenAICompatibleRejectsLengthFinishReason`, `TestOllamaRejectsLengthDoneReason`, `TestTruncatedToolCallIsNotDispatched`, `TestTruncatedFinalIsNotAcceptedAsSuccess`, `TestTruncatedPayloadFailsSafely`. |
| Linux descendant termination | Required criterion satisfied; local Linux evidence added | Existing process-group SIGKILL contract. Local Linux tests at base `92c996a` with the new fixtures: `TestExecTimeoutKillsGrandchild`, `TestExecCancellationKillsGrandchild`, `TestExecTimeoutKillsSigtermIgnoringDescendant`, `TestExecNormalCompletionNoDescendants`, `TestCommandAgentTimeoutKillsDescendant`, and `TestCleanupOwnedPIDDoesNotSignalDifferentIdentity` pass. Each timeout/cancellation test verifies the child and grandchild PIDs are gone; the cleanup helper verifies `/proc` start time before signalling. GitHub Actions run `42576841956`, job `129026456948`, remains corroborating CI evidence. |
| Validation-runner output bound | Required criterion implemented and verified on the follow-up branch | `limitedBuffer` bounds each capture at 10 MiB with a retained tail for trailing `go test -json` completion events; `boundedValidationStream` bounds each rendered stream to the canonical 64 KiB context limit. Tests: `TestRunBoundsCapturedOutput`, `TestPostgresCompletionSurvivesCaptureBound`, `TestValidationFailureContextBoundsHugeOutput`. |
| Snapshot restore policy | Explicitly out of scope for Plan 06's output-bound criterion | `toolharness.snapshot` is an internal undo-correctness mechanism, not model-visible output or a configured consumption limit. The allocation risk is recorded; bounding it needs a restore-completeness policy. |
| `dqharness` unknown-token handling | `NOT_APPLICABLE` for the provider-usage criterion | Production `ModelDecision` counts deterministic `retrievalboundary.EstimateTokens` for the prompt and output, not provider-reported usage; in-flight requests are recorded as uncertain. |

## 4. Acceptance mapping

### BUDGET-001 — Map limits and their authority

- Per-call/per-attempt/per-job/shared limits: §1 and §1.1.
- Elapsed/tokens/output bytes/monetary: §1; monetary is a GAP.
- Model/caller inputs: §1.2.
- Report delivered: this file.

### BUDGET-002 — Reproduce and repair limit bypasses

- Boundary cases for changed limits: §2.1, §2.2, §2.6, §2.7 (below/at/above for
  the output and MCP bounds).
- Model/caller parameters cannot raise operator limits: §1.2. The output-token
  value is operator-owned; no model-supplied value exists.
- Token counting where reported: §2.3. No execution token budget exists; step,
  tool-call, and stale bounds are the circuit breakers. A token budget is a GAP.
- Retries/continuations cannot reset an aggregate budget: no aggregate execution
  budget exists (`NOT_APPLICABLE`); per-invocation budgets reset by design and the
  attempt/continuation counts are enforced.
- Resume preserves consumed budget: `TestContinuationLimitSurvivesRestart`,
  `TestPreflightFailureKeepsContinuationBudget`.
- Concurrent accounting: `TestConsumeContinuationDoesNotOverspendConcurrently`;
  in-process `perf.Usage` remains mutex-protected
  (`TestUsageSumsCallsByModel`).
- Timeout/cancellation: direct process and process group verified on macOS and
  Linux CI (`TestExecTimeoutKillsProcessGroup`,
  `TestExecCancellationKillsProcessGroup`, `TestCommandAgentTimeoutKillsDescendant`).
- Oversized/streamed output bounded at the read boundary: §2.1, §2.2, §2.6,
  §2.7, and §2.10 (validation output).
- A normal bounded task still completes: `make check` passes; boundary tests
  include the just-below case with no truncation.

### BUDGET-003 — Verify terminal behavior and report limits

- Missing usage is unknown, not zero: §2.3. `dqharness` is an estimate-based
  evaluation path (`ModelDecision` uses `retrievalboundary.EstimateTokens`), not a
  provider-usage path, so this criterion is `NOT_APPLICABLE` there.
- No hard dollar-cap claim: §6.
- Per-call overshoot and unsupported provider behavior: §1 and §6.
- Focused and required checks pass: §5.

## 5. Verification commands and results

All commands ran in the isolated worktree on `darwin/arm64`.

| Command | Result |
|---|---|
| `go build ./...` | PASS |
| `go vet ./...` | PASS |
| `go test -count=1 ./internal/agent/... ./internal/ollamaagent/... ./internal/orchestration/... ./internal/failure/...` | PASS (truncation and bounded-classification coverage) |
| `go test -count=1 ./internal/agent/... ./internal/ollamaagent/... ./internal/mcp/... ./internal/toolharness/... ./internal/run/... ./internal/continuation/... ./internal/perf/...` | PASS |
| `go test -count=1 ./internal/cli/... ./internal/testrunner/...` | PASS |
| `go test -count=1 ./internal/failure/... ./internal/recovery/... ./internal/autonomy/... ./internal/domain/... ./internal/dqharness/... ./internal/hipcompare/...` | PASS |
| `make check` (`go fmt`, `go vet`, `go test ./...`, `go build ./...`, plugin check) | PASS |

The focused suites are the evidence for the changed read, usage, continuation,
MCP, command-agent, validation, and cancellation bounds. No live model call was
required for these checks. The merged Linux CI job `129026456948` (`ubuntu-26.04`,
run `42576841956`, head `dfd135c`) passed the full suite including the unix-tagged
process-group and signal tests.

## 6. Residual limitations and follow-up proposals

1. **Truncation is detected only where the provider reports it.** The supported
   agent adapters (`internal/agent/openai.go`, `internal/agent/ollama.go`, and the
   in-process `internal/ollamaagent/ollama.go`) reject a `length` stop. A custom
   `SOP_AGENT_COMMAND` process that exits zero with partial stdout carries no
   provider stop reason SOP can trust; the command boundary cannot infer it. A
   provider-specific stop reason other than `length` is also not guessed at: it
   needs an explicit mapping decision. The cap's interaction with provider context
   windows remains unmeasured, and there is still no execution token budget.
2. **No execution token budget.** Token usage is diagnostic; the execution circuit
   breakers are turns, tool calls, stale iterations, and lifecycle counts. A token
   budget would be a new policy control.
3. **Provider-side billing may continue** after a local timeout or cancellation;
   only a provider-side stop or verified billing report can establish that it
   stopped.
4. **Tool snapshot reads a whole file.** `toolharness.snapshot` reads a file into
   memory to support `restore_file`; a very large file is fully allocated. This is
   an internal undo-correctness mechanism, not model-visible output or a configured
   consumption limit, so it is outside Plan 06's output-bound criterion; a bound
   would need a restore-policy decision.
5. **Security adapter descendants are not claimed.** The security runner bounds
   time/output but does not use the tool-harness process-group path.
6. **`dqharness` uses token estimates, not provider usage.** Its `ModelDecision`
   counts `retrievalboundary.EstimateTokens` for the prompt and output, so the
   "missing provider usage is zero" concern does not apply to that path; in-flight
   requests are tracked as uncertain.
7. **No monetary accounting.** There is no pricing table, pre-dispatch
   reservation, or hard dollar cap. This report makes no monetary-cap claim.
8. **No graceful escalation phase exists on unix.** The subprocess contract sends
   process-group SIGKILL on timeout or cancellation; it does not send SIGTERM first.
   `TestExecTimeoutKillsSigtermIgnoringDescendant` confirms a SIGTERM-ignoring
   descendant is still terminated. This evidence is for the unix process-group
   implementation; `proc_other.go` still kills only the direct child and is not
   claimed as group termination.

## 7. PR and handoff

- Merged PR: https://github.com/imhttran/agentic-sop/pull/284 (merge commit
  `026f970c2d1040548e8578bb2d949f57e40f6a1f`).
- Merged code/verification commits: `400e4544`, `c8e00b1c`, `e656a8e7`.
- Follow-up branch: `security/llm06-reconcile`.
- Follow-up code commits: `cb230d66ffad174f3040944876d6c0f7b9c7c730` (validation
  output bounds) and `e053d18e9252ef9d821b2648accca1967e04f0f8` (retain trailing
  structured evidence).
- Follow-up PR: https://github.com/imhttran/agentic-sop/pull/289 (this PR; not merged).
