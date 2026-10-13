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
- **Task-Scoped Discovery Budgets** — **delivered**: the productive-discovery allowance
  is a canonical, operator-configurable, bounded execution limit
  (`internal/budget`: `DiscoveryIterations`; `SOP_OLLAMA_DISCOVERY_ITERATIONS`), recorded
  in `trace.json` and enforced per task by the existing discovery/stale machinery. See
  [Task-Scoped Discovery Budgets](#task-scoped-discovery-budgets).

Metrics to capture: run latency, token and tool-call counts, and reliability
measurements. This is distinct from the earlier, already-implemented
`PLAN-Phase-6-Interactive-Approval`; it is the reliability/evaluation workstream
that follows the closeout.


## Post-performance architecture (Phase 8) — delivered

**Status:** Delivered (Phase 8). The pre-performance closure and baseline completed
(recorded in [RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md) and
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md)) and deferred the new performance
architecture behind the readiness gate
([CLOSE-011](../reports/pre-performance-closure/CLOSE-011-readiness.md)). That
architecture then shipped as **Phase 8 (Context & Execution Efficiency)**, which is
complete and closed. The capabilities this section once listed as deferred are now:

- **DELIVERED** — Context Engine (CTX-001), structural repository index
  (CTX-002), BM25 retrieval (CTX-003), retrieval evaluation gate (CTX-004), prompt
  compiler (CTX-005), response normalizer (CTX-006), verification cache (CTX-007),
  prompt result cache (CTX-008), decision memory (CTX-010), adaptive routing (CTX-011),
  automatic prompt tuning (CTX-012).
- **EVALUATED / REJECTED** — vector retrieval (CTX-009): a deterministic
  evaluation found no improvement over BM25, so BM25 is retained and no vector
  retrieval shipped.
- **DELIVERED BUT OPT-IN** — the verification cache (`sop validate --cache`), the
  portable cross-worktree evidence store (`SOP_VERIFY_CACHE_SHARED`, VER-002), the
  prompt result cache (`sop prompt --cache`), and decision memory on the run path
  (`context.decision_memory`); adaptive routing is live within the opt-in routing
  path.
- **DELIVERED (OPT-IN)** — the persistent cross-run verification cache, tracked
  below as VERIFCACHE-PERSISTENCE.

The Git-SHA summary cache once named here was not shipped as a distinct component; the
structural repository index (CTX-002) and the verification cache (CTX-007) carry
repository-state identity instead. Phase 8 evidence:
[history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md](../history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md);
post-Phase-8 governance:
[history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md](../history/plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md).

## Persistent cross-run verification cache (VERIFCACHE-PERSISTENCE)

**Status:** Delivered (opt-in). VER-001–VER-004 shipped the portable, cross-worktree
verification-evidence store (`internal/verifcache/store.go`, `SharedStore`) behind the
opt-in `SOP_VERIFY_CACHE_SHARED` location, plus an append-only reuse-decision record
(`verify-cache-decisions.jsonl`). It reuses the existing CTX-007 cache primitives — no
second identity or cache implementation — and a hit is returned only for exact
`Identity` equivalence; every failure mode (unreadable file, schema mismatch, corrupt
or partial entry) yields a miss, never a hit. Concurrent writers are serialized by an
advisory lock and committed with an atomic rename, so a shared store cannot lose prior
evidence or leave a truncated document. It is disabled by default, so an unconfigured
project is unchanged.

Remaining trust-boundary note (design, not a defect): cross-environment reuse assumes
the identity inputs (repository state, validation commands, toolchain) are genuinely
equivalent across the sharing worktrees. The store treats a shared location as
trusted operator input; the false-hit analysis for sharing across *heterogeneous*
environments (not just sibling worktrees of one repository) remains future work. That
is the residual item below, not the previously-described "deliberately not wired" gap,
which is closed.

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

**Priority:** High. **Status:** Delivered. **Depends on:** Bounded productive IMPLEMENT/FIX discovery (`8f426e2`).

### Problem

IMPLEMENT/FIX previously allowed productive discovery through a fixed turn threshold
before stale/no-progress semantics took over. This prevented short premature
termination, but the fixed threshold still assumed that different agents/models require
roughly the same amount of discovery before mutation.

### Resolution

The allowance is now a canonical, operator-configurable, bounded execution limit rather
than a hard-coded window — deliberately **not** a per-model table (no
`Codex = 18 / DeepSeek = 12 / Nemotron = 10` hardcoding):

- `internal/budget` carries `DiscoveryIterations` (default `12`, the historical
  window), overridable with `SOP_OLLAMA_DISCOVERY_ITERATIONS`, so an unconfigured project
  is unchanged.
- It is bounded: a malformed value keeps the base, a value below
  `MinDiscoveryIterations` is raised, a value above `MaxDiscoveryIterations` is clamped,
  and the effective window is always capped strictly below the capability's hard
  iteration ceiling.
- The capability policy (`DiscoveryWindow`) resolves it once per invocation; the
  existing discovery/stale machinery (`observeDiscovery`, `mutationConvergenceRequired`,
  `staleLimitFor`) reads that single value, so a widened allowance lets a discovery-heavy
  run keep making credited progress without ever bypassing the stale guard, the
  convergence enforcement, or the approval boundaries.
- It is recorded in `trace.json` (`budgets.discovery_iterations`), in the no-progress
  diagnostic (`discovery_allowance=`), and observed discovery in
  `progress_summary.discovery`.

Evidence: `internal/budget/budget_test.go` (bounds, clamp, base preservation),
`internal/ollamaagent/discovery_budget_test.go` (widened/narrow allowances move the
no-progress stop point but stay bounded; provider-agnostic),
`internal/ollamaagent/policy_test.go`, and the `evals/budgets/` fixtures.

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


## SOP-EXTERNAL-COMPLETION

**Status:** Delivered. `sop task complete <task-id> --external` records a task completed
outside the SOP execution as an explicit, model-free operator action backed by repository and
validation evidence (see `RECOVERY.md` §11). The description below is preserved for the
history of why it was needed.

SOP records a task's completion only when its own lifecycle
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


## Phase 10 — Dynamic Worker Action Space

**Priority:** Low
**Status:** Backlog (evidence-gated). Deliberately **not** scheduled or implemented.

Phase 10 would give every worker assignment a deterministic, harness-derived
**action space** — the bounded operations, tools, read scope, write scope, and
compatible targets the harness derives from authoritative inputs
(`WorkAssignment + RepositoryState + TaskState + Scope + Policy + WorkerCapabilities + Budget`)
— with execution-time revalidation and stale-action rejection. A model would propose
within that space; the harness would validate the proposal and grant authority. The
invariant is unchanged: a model-generated action is **data until the harness validates
it against the current worker action space**, and a worker can never gain authority by
naming a tool, operation, path, target, or provider command.

This is deferred until **real Phase 9 usage shows a measurable limitation** that the
current static orchestration cannot absorb. Revisit it when run evidence records any of:

- repeated `BLOCK` / `NO_PROGRESS` caused by static orchestration;
- manual replanning an operator performs by hand today;
- undiscovered dependencies between decomposed work units;
- another measurable orchestration limitation.

No orchestration complexity is added ahead of that evidence. Until then, static
orchestration under the existing Phase 9 boundaries remains the design. Design
inspiration only — no dependency, no code reuse: the constrained-action architecture of
`browser-use/jev-ultrafast`. Provider/model neutrality, the existing Context Engine, the
single lifecycle authority, and the single-agent defaults are all preserved.

## Phase 9 (complete)

**Status:** Complete. Phase 9 (Multi-Agent Orchestration) shipped as ORCH-001–ORCH-012;
all twelve tasks are `LOCAL_DONE`. The deterministic adoption gate
([PHASE-9-MULTI-AGENT-ADOPTION.md](../reports/PHASE-9-MULTI-AGENT-ADOPTION.md)) keeps
multi-agent execution **opt-in**, so default execution stays single-agent and Phase 9
changes no default behavior. Plan:
[PHASE-9-MULTI-AGENT-ORCHESTRATION.md](PHASE-9-MULTI-AGENT-ORCHESTRATION.md).

The post-Phase-8 governance pass (POST8-001) is complete. The remaining Phase 8
follow-up is the persistent cross-run verification cache, tracked above as
`VERIFCACHE-PERSISTENCE`.

## Status and roadmap

See [PROJECT-STATUS.md](../reference/PROJECT-STATUS.md) for implemented capabilities
and [its known limitations](../reference/PROJECT-STATUS.md#known-limitations-current).
The numbered implementation plans remain in [this directory](.).

## Known limitations (current)

See [the current limitations reference](../reference/PROJECT-STATUS.md#known-limitations-current).
