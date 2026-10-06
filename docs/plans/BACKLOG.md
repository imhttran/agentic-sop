# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](../PLAN.md); this file is
the running list in between. Completed observations are preserved in
[RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md); shipped capabilities and known
limitations are catalogued in [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md).

## Pre-Performance Closure and Baseline

**Status:** Complete (executed). All eleven CLOSE tasks are `LOCAL_DONE` and the plan's final gate passed; the closure verdicts are under [../reports/pre-performance-closure/](../reports/pre-performance-closure/) with the published baseline at [../reports/PERFORMANCE-BASELINE.md](../reports/PERFORMANCE-BASELINE.md).

[PLAN-Pre-Performance-Closure.md](PLAN-Pre-Performance-Closure.md) was the
closure and measurement DAG for SOP and sop-controller. It verified existing
capabilities instead of reimplementing them. Performance implementation remains
deferred behind the plan's evidence-backed readiness gate (see
[CLOSE-011](../reports/pre-performance-closure/CLOSE-011-readiness.md)).

The subsequent architecture remains deferred: Prompt Compiler, Response
Normalizer, Context Engine, Git-SHA summary cache, repository structural index,
BM25/vector RAG, Decision Memory, Verification Cache, Prompt Result Cache, Adaptive
Routing changes, and Automatic Prompt Tuning. The closure plan inventories gaps
in the existing `internal/perf` contract; it does not build a competing telemetry
system or implement these candidates.

## Invocation-Scoped IMPLEMENT Completion Evidence

**Status:** Resolved (superseded). Production `runStages` is invocation-scoped:
it derives IMPLEMENT completion evidence from before/after repository snapshots
plus harness-reported changed files, and the whole-tree diff is used as evidence
only in verify-first mode (`internal/cli/run.go`, `internal/cli/mutation.go`). A
pre-existing dirty tree therefore does not satisfy the current invocation's
mutation requirement. See
[AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md#verified-operation-level-mutation).

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

## Status and roadmap

See [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md) for implemented capabilities
and [its known limitations](../reference/PROJECT-STATUS.md#known-limitations-current).
The numbered implementation plans remain in [this directory](.).

## Known limitations (current)

See [the current limitations reference](../reference/PROJECT-STATUS.md#known-limitations-current).
