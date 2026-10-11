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
IMPLEMENT task with green validation, no observed implementation mutation, and no
verified already-satisfied proof (§8a) MUST NOT PASS: its early completion is
retryable `CONTINUE` with `IMPLEMENT_NO_CHANGES`. Both `changes_expected=true` and `changes_expected=false`
are subject to that rule. Validation still runs on the no-change path.

Verify-first and declared-done modes have their own semantics, owned by
[EXECUTION.md](EXECUTION.md#6-verify-first-execution-mode) and
[declared-complete stages](EXECUTION.md#6a-declared-complete-stages).
`needs_human`, `failed`, and legacy output continue through the existing outcome,
validation, quality, and retry paths; a model-reported failure with no observed
change is reconciled to a retryable no-op by the tool harness. See
[RECOVERY.md](RECOVERY.md), [VALIDATION.md](VALIDATION.md), and [QUALITY.md](QUALITY.md).

### Verified operation-level mutation

**Implemented / Required.** For IMPLEMENT and FIX, mutation classification MUST
identify an operation requiring observation, not prove that a mutation occurred.
`internal/toolharness.RunObservedMutation` snapshots state immediately before and
after the operation, executing the tool exactly once. Positive mutation evidence
MUST require successful execution, successful observation, and a state difference.

- Named file operations use their canonical target; commands with unknown affected
  paths use the repository tree. File creation, deletion, restoration, changed
  content, and observable type or permission changes can supply evidence.
- Snapshots use streaming hashes of complete file contents, existence, type, and
  relevant permission bits. They ignore timestamps and inode identity. Tree
  comparisons exclude Git metadata and SOP-owned `.agent-sdlc/` runtime state.
- Identical writes and unchanged formatting MUST NOT reset the stale streak,
  set `mutationObserved`, produce successful mutation paths, or enter CHANGE.
  `gofmt -l` is non-mutating; `gofmt -w` requires an actual before/after difference.
- Failed or denied operations and unavailable snapshots MUST NOT supply positive
  mutation evidence. A verified partial change left by a failed operation remains
  visible for diagnostics and reconciliation, without earning successful progress.

**Implemented.** The outer `runStages` IMPLEMENT completion gate is
invocation-scoped: it derives evidence from before/after repository snapshots plus
harness-reported changed files, so a pre-existing dirty tree does not satisfy the
current invocation's mutation requirement. The whole-tree diff is used as evidence
only in verify-first mode (`internal/cli/run.go`, `internal/cli/mutation.go`).

## 8a. Verified Already-Satisfied Completion

**Implemented.** The native Ollama IMPLEMENT/FIX harness can return an explicit
`completion: ALREADY_SATISFIED` when existing repository content satisfies the
caller-owned task verification contract. This is completion evidence, not mutation
progress, and does not enter CHANGE.

**Required.** The harness MUST verify all of the following before accepting the claim:

- The caller supplied nonempty acceptance criteria and validation commands. The
  model MUST NOT choose substitute criteria or easier checks.
- The response explicitly has `status: completed` and
  `completion: ALREADY_SATISFIED`, mapping every exact criterion once to one or more
  canonical repository files successfully and informatively read in this invocation.
- Every caller-supplied validation command actually executed successfully in this
  invocation. Command identities use parsed arguments; success comes from the real
  exit status, not model output or displayed command text.
- No verified mutation was observed, and a known invocation-start repository-tree
  fingerprint equals the fingerprint at completion. Failed or unavailable
  observation MUST fail closed.

The caller is responsible for configuring checks that prove the task's acceptance;
the harness verifies the inspection mappings and executed checks, not the semantic
meaning of arbitrary prose. Reads, narration, no-op writes, formatting no-ops, and
claimed test results alone MUST NOT qualify. Validation evidence MUST NOT reset
the stale streak or extend discovery or iteration budgets.

For example, when the caller supplied criterion `Add returns the sum` and command
`go test ./...`, a candidate outcome is:

```json
{
  "status": "completed",
  "summary": "The existing implementation passed the required check.",
  "completion": "ALREADY_SATISFIED",
  "changes_expected": false,
  "evidence": {
    "acceptance": [{"criterion": "Add returns the sum", "paths": ["sum.go"]}],
    "validation_commands": ["go test ./..."]
  }
}
```

This JSON alone has no completion authority. Only the trusted executing harness
sets `Response.VerifiedAlreadySatisfied` after verification. Parsing model or
command-provider JSON MUST NOT set that flag; the command-provider path currently
cannot establish this proof. The harness replaces claimed validation commands
with the caller-owned commands it observed passing and records
`repository_mutations: 0` without fabricating changed files.

The lifecycle MUST still run its independent validation and quality gates; a
verified claim is permission to complete without mutation, not a PASS verdict.
Reports preserve `completion: ALREADY_SATISFIED`, `repository_mutations: 0`, the
acceptance mappings, and executed validation commands. A later FIX invalidates an
earlier proof; a run that implemented changes MUST NOT be reported as a
zero-mutation completion merely because its final FIX needed no edit.

This outcome is distinct from the operator's `execution_mode: done` declaration
and the pre-agent `verify-first` path, whose rules remain in [EXECUTION.md](EXECUTION.md).

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
IMPLEMENT and FIX. Discovery credit does not affect operation-level mutation
verification (§8) or already-satisfied proof (§8a).

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
  CHANGE/finalization behavior. Discovery alone MUST NOT set mutation evidence or
  satisfy the completion requirement in §8 or already-satisfied proof in §8a.

**Required.** Once the bounded discovery window has closed (`iteration >
implementNowAfter`) with no observed mutation, the harness MUST require convergence:
as soon as a stale turn has accrued, non-mutating repository tools (`read_file`,
`list_files`, `search_files`, and non-mutating `run_command`) MUST be denied with a
correction prompt, while the mutation tools (`write_file`, `create_file`,
`delete_file`, `restore_file`, or a mutating `run_command`) remain available, so the
run converges on a mutation attempt or returns a truthful `needs_human`/`failed`
outcome. This enforcement opens strictly before the stale bound and is derived from
the existing discovery window and stale accounting; it introduces no new counter,
threshold, or budget knob.

The stale guard remains the fallback for a run that still does not converge:
continuous non-mutating activity stops by turn 17 with `IMPLEMENT_NO_PROGRESS` /
`FIX_NO_PROGRESS`, and the pre-mutation denial above fires on the same path strictly
before that bound. Attempted inspection checkpoints remain first-seen, deduplicated,
and bounded to twelve paths, with five shown in diagnostics; failed attempts may
appear there but MUST NOT earn discovery credit merely because they are checkpoint
members.

Diagnostics record the actual turn count, credited inspections, mutations, changed
files, and last action. Early `IMPLEMENT_NO_CHANGES` completion and stalled
`IMPLEMENT_NO_PROGRESS` execution MUST remain distinct.

The built-in mutating ceilings can still be overridden by positive
`SOP_OLLAMA_IMPLEMENT_ITERATIONS` and `SOP_OLLAMA_FIX_ITERATIONS` values. Unset,
blank, nonnumeric, or nonpositive values retain 32/24. The existing scaled
finalization thresholds are unchanged; discovery credit remains limited to the
first twelve model turns.

**Observational progress signals.** The harness marks the activity events it emits
with the progress it has already substantiated — a first-seen inspection
(`discovery.novel`) or a verified mutation (`mutation.verified`) — so the
structured run trace can distinguish progress from mere activity. These markers
are observation-only: they MUST NOT change the stale streak, discovery credit,
retry policy, budgets, human boundaries, or termination. See
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md) and the trace artifact
(`.agent-sdlc/runs/<id>/trace.json`).

**Proposed-future.** Task-scoped discovery budgets are tracked in
[BACKLOG.md](../plans/BACKLOG.md#task-scoped-discovery-budgets); they are not implemented.

## 10. Operator-Set Diagnostic Sinks

A failed run — or one reporting a failure or human boundary — SHOULD write a safe per-turn diagnostic trail to stderr (capability, phase, iteration, tool, request, progress, recovery) that MUST NOT contain prompts, file contents, or secrets. The command provider discards that stderr, so the operator MAY set `SOP_OLLAMA_TRACE_LOG` (trace) and `SOP_TOOL_AUDIT_LOG` (tool audit). Both MUST be operator-set sinks; neither MUST be SOP's state database.
