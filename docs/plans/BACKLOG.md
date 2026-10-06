# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](../PLAN.md); this file is
the running list in between. Completed observations are preserved in
[RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md); shipped capabilities and known
limitations are catalogued in [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md).

## Agentic reliability and evaluation (AGENT-001–005)

**Status:** Complete. After the pre-Phase-6 closeout (complete), AGENT-001 through
AGENT-005 are delivered:

- **AGENT-001 Structured Run Trace** — **delivered**: a versioned observational
  `trace.json` (schema 1) per run, summarized by `sop report`.
- **AGENT-002 Progress Signals** — **delivered**: observational progress signals
  (discovery, repository mutation, verification, state transition) in `trace.json`
  (schema 2), summarized by `sop report`; they do not change lifecycle decisions.
- **AGENT-003 Evaluation Harness** — **delivered**: deterministic fixtures over
  `trace.json` under `evals/`, evaluated by `internal/eval` (see
  [EVALUATION.md](../reference/EVALUATION.md)).
- **AGENT-004 Agent Budgets** — **delivered**: the canonical execution budget
  (`internal/budget`: iteration, stale, tool-call limits), configurable via
  `SOP_OLLAMA_*` and recorded in `trace.json`.
- **AGENT-005 Replan Strategy** — **delivered**: opt-in, deterministic permission
  to change strategy once on the same class after a recoverable failure
  (`internal/recovery`; `models.replan_enabled` / `models.max_replans`), recorded in
  `trace.json` (schema 4) and asserted by `evals/replan/`.
- **Task-Scoped Discovery Budgets** (see
  [Task-Scoped Discovery Budgets](#task-scoped-discovery-budgets)) — deferred; the
  discovery allowance remains fixed (a soft threshold, not a hard budget).

Metrics to capture: run latency, token and tool-call counts, and reliability
measurements. This is distinct from the earlier, already-implemented
`PLAN-Phase-6-Interactive-Approval`; it is the reliability/evaluation workstream
that follows the closeout.


## Deferred performance architecture

**Status:** Backlog (deferred).

The pre-performance closure and baseline is complete (recorded in
[RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md) and
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md)). It deliberately did not begin
the new performance architecture, which remains deferred behind the readiness gate
([CLOSE-011](../reports/pre-performance-closure/CLOSE-011-readiness.md)):

Prompt Compiler, Response Normalizer, Context Engine, Git-SHA summary cache,
repository structural index, BM25/vector RAG, Decision Memory, Verification Cache,
Prompt Result Cache, Adaptive Routing changes, and Automatic Prompt Tuning.

## Performance telemetry gaps

**Status:** Backlog (low). `internal/perf` records stage durations and counts but
not every measurement the baselines would like. Documented gaps (recorded as
`UNAVAILABLE` rather than invented in the baselines):

- a tool-call count — `Counts` has no field (only the JEV record has `ToolCalls`);
- a per-category (`build`/`test`/`lint`) `validation_ms` breakdown for a safely
  *reused* validation;
- per-step PLAN timing (a slow plan is one number, not a profile).

Sources: `docs/history/PREJEV017-PERFORMANCE-BASELINE.md` and the PREJEV018
remaining-issues register (I4–I6).


## Local network service (team mode)

Share SOP state and control across a team on the local network.

## Small-device dashboard

A read-only view of SOP state for small devices.

## Jev adapter and Jev-vs-deterministic evaluation

An adapter for the Jev planner, and an evaluation comparing it with the
deterministic planner.

## OpenAI-style tool calling for `openai_compatible` (Phase 3)

`openai_compatible` (Phase 1 identity/configuration, Phase 2 execution) currently
runs against the shared `internal/agent` `OpenAICompatible` transport, which is
text-only: its `chatRequest` has no `tools` field and it parses no tool calls, so
the checked wrapper honestly declares the text capabilities and rejects
`IMPLEMENT`/`FIX`. The only tool-calling loop in the tree is Ollama-native
(`internal/ollamaagent`, `POST /api/chat`).

Make the generic provider a coding agent by giving the shared OpenAI-compatible
transport real tool calling:

- add `tools` / `tool_calls` to the shared request/response (one implementation,
  reused by every OpenAI-compatible identity — do not fork a second HTTP client),
  and route the tool-call loop through the existing `internal/toolharness`
  surface (`internal/ollamaagent`-equivalent controls, no parallel harness);
- declare `IMPLEMENT`/`FIX` only when the server/model establishes tool support
  (never inferred from the model name), keeping the honest capability model;
- preserve the Phase 2 endpoint precedence
  (`SOP_OPENAI_COMPATIBLE_BASE_URL` > `providers.openai_compatible.endpoint` >
  `http://127.0.0.1:8000`), locality-as-configuration, and the IMPLEMENT/FIX
  no-progress guard semantics; do not raise the iteration ceilings.

Related: `internal/agent/openai.go` (the shared transport), `internal/agent/openaicompatible.go`
(the generic identity), `internal/toolharness`, `internal/ollamaagent` (the native
tool loop to mirror), and [../specs/PROVIDERS.md](../specs/PROVIDERS.md) §2a.

## Task-Scoped Discovery Budgets

**Priority:** High  
**Status:** Backlog  
**Depends on:** Bounded productive IMPLEMENT/FIX discovery (`8f426e2`)

### Problem

IMPLEMENT/FIX currently allow productive discovery through a fixed turn threshold
before stale/no-progress semantics take over.

This prevents short premature termination, but the fixed threshold still assumes
different agents/models require roughly the same amount of discovery.

A future agent may legitimately require more discovery turns before mutation.

We should avoid provider/model-specific limits such as:

- Codex = 18 turns
- DeepSeek = 12 turns
- Nemotron = 10 turns

## Earlier multi-package discovery observations

The original P3-006/P3-009/011/014/017 evidence and successive steering fixes are
preserved in [the resolved backlog](../history/RESOLVED-BACKLOG.md#bootstrap-agent-exhausts-its-budget-on-multi-package-tasks).
Remaining candidates were a concrete file/work scope, incremental continuation,
and a stronger configured model for multi-package tasks. Task-scoped discovery
budgets above track the next refinement; current bounded discovery behavior is
owned by [AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md#9-tool-harness-bounds-and-phases).

## Autonomy `BLOCK` hardening

**Status:** Backlog (defensive). `autonomy.Decide` has no `case failure.Block`:
only `NoProgress` produces `Disposition: BLOCK` today and is handled explicitly by
the `Kind == NoProgress` branch. A future non-`NoProgress` `BLOCK` classification
would fall through to a human boundary, contradicting `BLOCK`'s documented "not a
human approval" definition. Add an explicit terminal case plus a test. Surfaced by
the pre-Phase-6 closeout; currently unreachable.


## PLAN-LIFECYCLE-FOLLOWUP

**Status:** Backlog (LOW). PLAN-001 separated plan activation from execution, added
explicit supersession (`sop plan supersede`) and historicalization
(`.agent-sdlc/archive/<plan-id>/lifecycle.json`), and made reconciliation's executed
classification authoritative (`domain.Task.Executed`). One operation remains: there is
no way to historicalize/close an ACTIVE plan **without** installing a replacement plan,
so a dangling active plan (for example one whose source file moved) can only be cleared
by superseding it with another plan.

Provide a deterministic operation to historicalize/close an active plan without
requiring a replacement plan. Requirements:

```text
model-free
no task execution
preserve task/run/approval/evaluation evidence
unfinished tasks remain historically unfinished
no fabricated PASS
no fabricated approval
clear active-plan authority safely
historical plan cannot become runnable accidentally
```

LOW priority; must not block CTX-001.


## SOP-EXTERNAL-COMPLETION

**Status:** Backlog (MEDIUM). SOP records a task's completion only when its own lifecycle
runs the task and produces a verified repository mutation (or a verified `ALREADY_SATISFIED`
claim). When the work was performed outside the SOP runtime — for example by a coding agent
that implemented and merged it directly — `sop run` cannot complete the task: the model
discovers that the work already exists, makes no mutation, and the stale guard classifies
`IMPLEMENT_NO_PROGRESS` (a spurious BLOCK). There is no model-free way to record the
completion, so the task's SOP state cannot reflect the merged work.

Provide a deterministic, model-free operation to record a task as complete against
authoritative evidence (for example a merged change plus a green validation), so SOP's task
state can reflect work performed outside its runtime without bypassing the lifecycle,
fabricating a PASS, or re-running the model.

Requirements:

```text
model-free
no task execution
requires authoritative evidence (merged change + green validation)
no fabricated PASS
no fabricated approval
does not advance dependent tasks beyond what the evidence supports
```

Surfaced when CTX-001 (Context Engine) was implemented and merged by a coding agent and
`sop run` could not record it: the model recognized the existing implementation, produced no
repository change, and the task was classified `IMPLEMENT_NO_PROGRESS` and blocked.


## Status and roadmap

See [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md) for implemented capabilities
and [its known limitations](../reference/PROJECT-STATUS.md#known-limitations-current).
The numbered implementation plans remain in [this directory](.).

## Known limitations (current)

See [the current limitations reference](../reference/PROJECT-STATUS.md#known-limitations-current).
