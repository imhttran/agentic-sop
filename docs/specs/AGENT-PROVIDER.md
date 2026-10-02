# Agent Provider

**Type:** Normative specification

## Purpose

This document is the normative specification for SOP's agent boundary: the **Harness**/**Provider**/**Model** separation, harness and provider options, model requirement rules, the configuration matrix and precedence, the command-agent protocol and structured outcome, provider capability detection, tool-harness bounds and phases, and diagnostic sinks. Configuration keys are catalogued in [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## Related Specifications

- [../architecture/OVERVIEW.md](../architecture/OVERVIEW.md) — §4 Agent Harness Adapter, Agent Providers.
- [../PRD.md](../PRD.md) — §10; [EXECUTION.md](EXECUTION.md) — the up-front provider check; [RECOVERY.md](RECOVERY.md) — requeue and retry; [VALIDATION.md](VALIDATION.md), [QUALITY.md](QUALITY.md) — checks and the gate.
- [PROVIDERS.md](PROVIDERS.md) — the provider/runtime abstraction (identity, health, discovery, capabilities, opt-in validation) that sits beneath the agent boundary.
- [../README.md](../README.md) — documentation index.

## Normative Language

The terms MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are normative.

---

## 1. Harness, Provider, and Model

SOP MUST keep three concepts independent: **Harness** — the layer executing agent logic (calling the model via the provider, managing tools and tool-call loops, enforcing bounds/timeouts, handling structured outcomes, and applying safety boundaries for file access, git, and command execution); **Provider** — the raw model source, which MUST NOT be assumed to supply tool-calling, file I/O, git, or mutation, and MUST NOT be treated as a harness; and **Model** — the identifier within a provider. The workflow MUST NOT depend permanently on one model or harness.

## 2. Options

Harness MUST be `tool` (a local tool-calling harness such as `sop-ollama-agent`) or `command` (a subprocess adapter running an external command that implements the agent interface). Provider MUST be `ollama` (local text-only), `llamacpp` (OpenAI-compatible), `mlx` (an Apple-Silicon MLX / oMLX runtime behind an OpenAI-compatible boundary), `openai_compatible` (a generic OpenAI-compatible endpoint — oMLX, vLLM, LM Studio, LocalAI, ...), or `command` (an external subprocess already a full agent). `llamacpp`, `mlx`, and `openai_compatible` share one OpenAI-compatible execution transport; see [PROVIDERS.md](PROVIDERS.md) §2a.

## 3. Model Requirement Rules

For `ollama`/`llamacpp`/`mlx`/`openai_compatible` a model MUST be specified (config `agent.model` or the provider's env var); the run MUST fail clearly if neither provides one. The command provider MUST NOT require a model. The command harness MUST require `SOP_AGENT_COMMAND`; the run MUST fail if unset.

## 4. Configuration Matrix

| Harness   | Provider   | Config keys                                      | Environment variables                                                                         | Model? |
| --------- | ---------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------- | ------ |
| `tool`    | `ollama`   | `agent.harness`, `agent.provider`, `agent.model` | `SOP_OLLAMA_BASE_URL`, `SOP_OLLAMA_MODEL`, `SOP_OLLAMA_TIMEOUT`                               | Yes    |
| `tool`    | `llamacpp` | `agent.harness`, `agent.provider`, `agent.model` | `SOP_LLAMACPP_BASE_URL`, `SOP_LLAMACPP_MODEL`, `SOP_LLAMACPP_TIMEOUT`, `SOP_LLAMACPP_API_KEY` | Yes    |
| `tool`    | `mlx`      | `agent.harness`, `agent.provider`, `agent.model` | `SOP_MLX_BASE_URL`, `SOP_MLX_MODEL`, `SOP_MLX_TIMEOUT`, `SOP_MLX_API_KEY`                     | Yes    |
| `command` | `command`  | `agent.harness` (optional; defaults to command)  | `SOP_AGENT_COMMAND` (required), `SOP_AGENT_PROVIDER` (optional)                               | No     |

## 5. Configuration Precedence

Settings MUST resolve highest-first: (1) environment variables (`SOP_AGENT_PROVIDER`, `SOP_OLLAMA_MODEL`, `SOP_AGENT_COMMAND`, …); (2) `.agent-sdlc/config.yaml`; (3) hardcoded defaults (the command harness when nothing is specified). Env MUST override config; when it selects the command harness, the config's tool/provider/model settings MUST be ignored. See [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md).

## 6. Turning a Text-Only Provider into a Coding Agent

A text-only Ollama/llama.cpp endpoint alone is **not** a coding agent and cannot mutate a repository. The `tool` harness MUST add the tools, safety boundaries, and structured logic that make it one: file tools (`read_file`, `write_file`, `create_file`, `delete_file`, `restore_file`, `list_files`, `search_files`), git tools (`git_status`, `git_diff`), a bounded allow-listed `run_command`, tool-loop bounds, and structured outcomes. Layers MAY be composed (the `command` harness running a `tool` harness subprocess, which holds the provider settings). SOP MUST retain validation, review, retries, human gates, and workflow state; the harness MUST remain an implementation adapter only.

## 7. Provider-Neutral Interface and Capability Detection

The boundary MUST expose a provider-neutral interface — conceptually `Execute(task, context) -> result`, `Review(diff, criteria) -> findings`, `Fix(task, findings) -> result` — so the workflow does not depend on one model or harness. Each provider MUST declare its capabilities; a checked wrapper MUST reject, naming the supported set, an unsupported-capability request. A run MUST check up front that the provider can `IMPLEMENT` before any task and MUST reject one that cannot. See [EXECUTION.md](EXECUTION.md).

## 8. Command-Agent Protocol and Structured Outcome

**Implemented.** A command agent reads JSON requests from stdin and writes JSON
responses to stdout. For mutating capabilities (`IMPLEMENT`, `FIX`) the structured
outcome is `{"status":"completed","summary":…,"changes_expected":true|false}`,
`{"status":"needs_human","reason":…}`, or `{"status":"failed","reason":…}`.
SOP interprets the structure rather than inferring an outcome from narration.

**Required.** `changes_expected` MUST remain model-provided evidence, not authority
for waiving a deterministic change requirement. An ordinary change-requiring
IMPLEMENT task with green validation and no observed implementation mutation
MUST NOT PASS: its early completion is retryable `CONTINUE` with
`IMPLEMENT_NO_CHANGES`. Both `changes_expected=true` and `changes_expected=false`
are subject to that rule. Validation still runs on the no-change path.

Verify-first and declared-done modes have their own semantics, owned by
[EXECUTION.md](EXECUTION.md#6-verify-first-execution-mode) and
[declared-complete stages](EXECUTION.md#6a-declared-complete-stages).
`needs_human`, `failed`, and legacy output continue through the existing outcome,
validation, quality, and retry paths; a model-reported failure with no observed
change is reconciled to a retryable no-op by the tool harness. See
[RECOVERY.md](RECOVERY.md), [VALIDATION.md](VALIDATION.md), and [QUALITY.md](QUALITY.md).

**Implemented limitation.** The harness reconciles against an invocation-start
working-tree fingerprint, but successful classified mutation tools still supply
positive mutation evidence without content-change verification. The outer
`runStages` completion gate still uses a nonempty diff as its IMPLEMENT mutation
signal. Same-content writes, formatting no-ops, and dirty-tree attribution at that
outer gate therefore remain separate correctness work; this specification does
not claim they are already solved.

## 9. Tool-Harness Bounds and Phases

**Implemented.** Bounds are per capability in `internal/ollamaagent/policy.go`;
PLAN/REVIEW use `executeTwoPhase`, while IMPLEMENT/FIX use `executePhased`.

### Document-producing capabilities

PLAN permits eight discovery turns, then two ordinary synthesis attempts. REVIEW
permits eight inspection turns, with a soft wrap-up nudge after six, then two
ordinary synthesis attempts. A final document can end either phase immediately.

**Required.** Tools MUST remain unavailable during synthesis. A prohibited tool
request MUST be denied and receive the capability's correction prompt, preserving
all gathered context. The first two requests consume the separate invocation-scoped
`maxSynthesisCorrections` allowance, not the ordinary synthesis-attempt budget.
A third MUST terminate with `termination=synthesis_correction_limit`. Narration
consumes the ordinary budget and exhausts it with `termination=synthesis_limit`.
Discovery MUST NOT restart and synthesis tools MUST NOT execute during correction.
Provider-level errors and retries remain separate from those corrections.

DESIGN_TESTS and DIAGNOSE_FAILURE use a twelve-turn bounded loop; diagnostic
inspection may include allow-listed non-mutating commands.

### Mutating capabilities

IMPLEMENT (32) and FIX (24) share DISCOVER → CHANGE → FINALIZE. The first successful
governed mutation enters CHANGE. Mutation-aware finalization waits for a stopped
writer after the normal threshold; force-finalization reserves a tool-free outcome
window at iteration 28 for IMPLEMENT and 20 for FIX. FINALIZE is terminal: all tool
requests, including writes, are denied. Its allowances are three turns for
IMPLEMENT and FIX. Mutation classification itself is unchanged by discovery credit.

**Required.** Discovery evidence MUST remain separate from mutation evidence:

- Through the existing `implementNowAfter` threshold (model turn 12), a successful
  first informative inspection MAY reset the stale streak.
- Identities MUST use operation plus canonical target; search identities include
  normalized scope and query. Each identity earns credit only once per invocation.
- Failed or denied calls, repeated inspections, narration, empty reads/listings,
  and searches with no matches MUST NOT receive discovery credit.
- Every model turn, including narration and denial, counts toward that window.
  After turn 12, every non-mutating turn MUST increment the stale streak.
- Five stale turns before the first mutation MUST terminate with the existing
  `IMPLEMENT_NO_PROGRESS` or `FIX_NO_PROGRESS`, `termination=no_progress`, and
  retryable continuation disposition. Consecutive identical behavior remains
  subject to the independent repetition guard and can stop earlier.
- Successful governed mutation MUST reset the streak and retain the existing
  CHANGE/finalization behavior. Discovery MUST NOT set mutation evidence or
  satisfy the completion requirement in §8.

Continuous novel discovery with no mutation therefore stops by turn 17. Attempted
inspection checkpoints remain first-seen, deduplicated, and bounded to twelve
paths, with five shown in diagnostics; failed attempts may appear there but MUST
NOT earn discovery credit merely because they are checkpoint members.

Diagnostics record the actual turn count, credited inspections, mutations, changed
files, and last action. Early `IMPLEMENT_NO_CHANGES` completion and stalled
`IMPLEMENT_NO_PROGRESS` execution MUST remain distinct.

The built-in mutating ceilings can still be overridden by positive
`SOP_OLLAMA_IMPLEMENT_ITERATIONS` and `SOP_OLLAMA_FIX_ITERATIONS` values. Unset,
blank, nonnumeric, or nonpositive values retain 32/24. The existing scaled
finalization thresholds are unchanged; discovery credit remains limited to the
first twelve model turns.

**Proposed-future.** Task-scoped discovery budgets are tracked in
[BACKLOG.md](../plans/BACKLOG.md#task-scoped-discovery-budgets); they are not implemented.

## 10. Operator-Set Diagnostic Sinks

A failed run — or one reporting a failure or human boundary — SHOULD write a safe per-turn diagnostic trail to stderr (capability, phase, iteration, tool, request, progress, recovery) that MUST NOT contain prompts, file contents, or secrets. The command provider discards that stderr, so the operator MAY set `SOP_OLLAMA_TRACE_LOG` (trace) and `SOP_TOOL_AUDIT_LOG` (tool audit). Both MUST be operator-set sinks; neither MUST be SOP's state database.
