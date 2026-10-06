# Resolved Backlog and Superseded Contracts

> **Historical, non-normative.** These observations preserve the backlog as it stood
> before this reconciliation. They describe successive fixes, including contracts
> superseded by later work; they do not define current behavior. Start at the
> [documentation index](../README.md), [current backlog](../plans/BACKLOG.md),
> [project status](../reference/PROJECT-STATUS.md), or
> [agent specification](../specs/AGENT-PROVIDER.md).

## Pre-Performance Closure and Baseline

_RESOLVED: executed._ The closure and measurement plan
[PLAN-Pre-Performance-Closure.md](plans/PLAN-Pre-Performance-Closure.md) ran to
completion: all eleven CLOSE tasks are `LOCAL_DONE`, both repositories'
deterministic gates are green, and the readiness verdict is recorded in
[CLOSE-011-readiness.md](../reports/pre-performance-closure/CLOSE-011-readiness.md)
with two explicit deferrals (representative workload measurements; the
controller-root dogfood). The new performance architecture it deferred remains in
the [current backlog](../plans/BACKLOG.md).

## Invocation-Scoped IMPLEMENT Completion Evidence

_RESOLVED: implemented._ The outer `runStages` IMPLEMENT completion gate is
invocation-scoped: it derives evidence from before/after repository snapshots plus
harness-reported changed files (the whole-tree diff is used only in verify-first
mode), so a pre-existing dirty tree does not satisfy the current invocation's
mutation requirement (`internal/cli/run.go`, `internal/cli/mutation.go`). Recorded in
[PROJECT-STATUS.md](../reference/PROJECT-STATUS.md#known-limitations-current) and
[AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md#verified-operation-level-mutation).

## Bootstrap resilience: run the Ollama agent as a known-good binary

_RESOLVED: implemented._ The bootstrap agent can break itself while editing
`internal/ollamaagent`: a small compile error in that very package removes the agent
needed to repair it, because SOP builds and runs the harness from the same tree under
edit. Recovery must not depend on the tool the change is allowed to break.

A pinned, prebuilt known-good `sop-ollama-agent` binary is therefore installed
**outside** the tree under edit (`make install-ollama-agent` →
`scripts/install/install-sop-ollama-agent.sh`, revision pinned in `scripts/agents/sop-ollama-agent.pin`),
and the command bootstrap (`scripts/agents/sop-ollama-agent.sh`) invokes that installed binary
instead of compiling candidate source; `internal/agentbin` resolves it, and
`internal/ollamaagent` reports the running revision. A broken working copy can still
be repaired. Recorded here for traceability.

## Default config cannot IMPLEMENT (provider/harness split unfinished)

_RESOLVED (verified during Phase 5.4 hardening): stale._ The Phase 5.4 composition root
(`defaultDeps().newAgent` in `internal/cli/cli.go`) resolves the effective
harness/provider pair and, for `harness: tool` + `provider: ollama`, builds the native
in-process tool agent (`ollamaagent.NativeAgentFromEnv`, wrapped with
`agent.NewChecked`) which declares every capability including `IMPLEMENT`. A fresh
`sop init` project therefore passes the up-front `guardCapability(a, IMPLEMENT)` check
and can run a task; the concern described here no longer holds. Recorded for
traceability.

## `--file` prompt input could escape the project through a symlink

_RESOLVED (Phase 5.4 hardening): `resolvePromptFile` now resolves both the project root
and the target with `filepath.EvalSymlinks` before the containment test, so a
project-local symlink pointing outside the project is rejected rather than read (the
project root is resolved too, so a project reached through a symlink is not falsely
rejected). Recorded here for traceability; see
[../specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §12._

## An IMPLEMENT prompt was rejected when the default agent lacked IMPLEMENT

_RESOLVED (Phase 5.4 hardening): the capability guard and provider validation now apply
to the FINAL executing agent (the routed selection), not to a default agent the router
replaces. With routing on, a read-only default provider with a tool-capable routed
class runs; the default agent is guarded at the routing seam only when routing selects
no class. `sop run` and `--task` follow the same rule. See
[../specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §4._

## Bootstrap agent exhausts its budget on multi-package tasks

_ADDRESSED (steering, T076): the unmutated run's steering now recurs._ The write
nudges were one-shot (at 6 and 12 interactions), so a model had up to nine
unsteered interactions (13–21) between being told to implement and the late-stage
cutoff at 22 — a one-shot instruction is easy for a small model to outrun. The
implement-now instruction is now re-stated on **every** interaction past
`implementNowAfter`, and the interactions from `implementClosingAfter` (19) to the
cutoff become **closing**: the run states that this invocation is about to end, so
the model is told the consequence while it can still write. No bound, phase, tool
availability, or finalization changes; the `→ CHANGE_CONTINUE` transition is still
recorded once. The two candidate fixes left here are not deterministic: a concrete
file/work scope would have to come from the plan, and "a stronger model for
multi-package tasks" is a routing/configuration choice (routing is off by
default). The continuation checkpoint and `execution_mode: done` (below) already
cover the other two symptoms. The original observation is kept for traceability.

_ADDRESSED (no-progress guard): a run that never changes the repository now stops early._ The
remaining observed shape — the model alternating planning with distinct read-only tool
calls and never mutating — evaded the repeat guard, which only detected _consecutive_
identical turns. A deterministic no-progress guard now treats a successful repository
mutation as the only progress signal: every non-mutating turn increments the counter
(reads — novel or repeated — searches, narration, and denied tools alike), and after
`maxNoProgressIterations` (5) the run stops with `termination=no_progress` and an
`IMPLEMENT_NO_PROGRESS` diagnostic, instead of consuming the whole budget. The guard
applies while no mutation has been observed, so a run that changed the repository is
governed by finalization as before. No ceiling was raised. See
[../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) §9. Recorded here for
traceability.

The bootstrap Ollama agent has a per-capability iteration ceiling and a late-stage
cutoff for a run that has not mutated (`IMPLEMENT`: 32 iterations,
`implementLateStageAfter` 22). On a task that spans several packages the model can
spend the whole budget on read-only discovery and be finalized having written
nothing, so SOP records `CONTINUE` and requeues the task. It converges only after a
re-run, and not at all for a task whose work already exists in the tree.

Observed dogfooding Phase 3 (2026-09-30): P3-006 issued 22 read-only tool calls and
zero writes across four attempts; P3-009/011/014/017 requeued the same way before
converging on a later run. The harness already nudges the model to write at 6/12/22
turns and the model ignores them, so raising the budget alone is unlikely to help.

Candidate fixes: give the task request a concrete file/work scope so discovery is
shorter; let a productive-but-unmutated run continue incrementally instead of
finalizing; or use a stronger model for multi-package tasks. Related: a plan can now
mark an already-implemented stage `done` (see below) so it is never re-implemented,
which removes the re-implement loop for work that already exists in the tree.

## A fully-implemented plan cannot close through SOP

_RESOLVED: `execution_mode: done` (a declared-complete stage)._ `sop run <PLAN>.md`
regenerates tasks from the plan's stages and starts each at `PLANNED`
(`internal/taskbuilder/taskbuilder.go`), reading only the stage's structured metadata.
When every stage is already implemented the agent finds nothing to change, the run
classifies as `NO_CHANGES_PRODUCED` → `AUTO_FIX` (`internal/failure`), and after the
retry budget the task ends `BLOCKED`, so work that is already green could not be
closed out through the lifecycle.

A plan stage can now declare that its work already exists:

```text
### Execution

- done
```

`done` is explicit plan metadata (like `verify-first`), never inferred from a task's
`Status:` line, title, or prose. Such a stage's task is recorded **already
satisfied** when the task graph is built, so its dependants are unblocked and a plan
whose work is already green reports as complete instead of blocking; SOP never
selects, gates, or verifies it, because the declaration is the record rather than
evidence. The run reports the declarations separately (`Declared done in the plan: N
task(s)`), so a completion is never mistaken for work the run performed. A plan
normalized by a model has the mode cleared, so only the plan document itself can
declare a stage complete, and a single `--task` run rejects it. Use `verify-first`
for work that must be _checked_; a stage with nothing to verify cannot become `done`
by implication. See [../specs/EXECUTION.md](../specs/EXECUTION.md) §6a.

## A gate failure with no authoritative classification is not escalated

_RESOLVED (Phase 5 hardening, P5-010): SOP's own deterministic verdict that a
mutating invocation claimed success while leaving the working tree unchanged is now
classified (`failure.NoChangesProduced`, an implementation failure) rather than left
empty, so the bounded recovery policy escalates it like any other classified
failure. A failure with no authoritative classification at all still fails closed to
the existing human/block path. Recorded here for traceability; see
[../specs/RECOVERY.md](../specs/RECOVERY.md) §8._

## Prompt runs are not selected by `sop report` with no argument

_RESOLVED (Phase 5.4): `latestRun` now also considers `runs/prompts/*/metadata.json`,
so `sop report` with no argument reports the newest run of either kind. Recorded here
for traceability; see [../specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §11._

## Bounded escalation does not apply to prompt runs

_RESOLVED (Phase 5 hardening): `sop prompt --capability implement` now runs the
governed implementation lifecycle through the same bounded-recovery seam a task uses,
so when `models.escalation_enabled` is on it escalates `small → medium → large`
exactly like a task, persisting one attempt per try under the prompt run. A read-only
prompt is a single bounded call with no quality gate, so escalation never applies to
it. Recorded here for traceability; see
[../specs/PROMPT-EXECUTION.md](../specs/PROMPT-EXECUTION.md) §8._


## PLAN-LIFECYCLE-FOLLOWUP — DELIVERED

_RESOLVED: `sop plan complete` (a deterministic, model-free plan-completion operation)._
PLAN-001 separated plan activation from execution and added explicit supersession
(`sop plan supersede`) and historicalization
(`.agent-sdlc/archive/<plan-id>/lifecycle.json`), but there was no way to
historicalize/close an ACTIVE plan **without** installing a replacement plan: a dangling
active plan could only be cleared by superseding it with another plan. `sop plan complete`
closes an ACTIVE plan directly.

The operation is model-free (it takes no agent and invokes no model) and executes no
task. It fails closed unless every task is satisfied, so a plan with unresolved work is
never completed. It archives the plan as `COMPLETE` with its task records preserved
verbatim (`.agent-sdlc/archive/<plan-id>/`, `lifecycle.json`), clears the task graph,
removes the machine plan, and releases the active-plan association, so the completed plan
is no longer ACTIVE and cannot be re-activated by `sop tasks` reading a stale `plan.json`.
It fabricates no PASS and no approval. POST8-001 exercised the operation end to end, and
the Phase 8 plan was closed the same way. See [../reference/CLI.md](../reference/CLI.md)
and [plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md](plans/POST8-001-PHASE-8-LIVE-PATH-INTEGRATION-GOVERNANCE.md).

## Superseded agent-provider contracts

The following text was replaced to describe the implemented completion gate,
synthesis correction allowance, terminal finalization, and bounded discovery
semantics. In particular, its automatic PASS for a model-declared no-change
completion and its mutation-only discovery guard are obsolete.

## 8. Command-Agent Protocol and Structured Outcome

A command agent MUST read JSON requests from stdin and write JSON responses to stdout, implementing the agent interface. For mutating capabilities (`IMPLEMENT`, `FIX`) it MAY return a structured execution outcome — `{"status":"completed","summary":…,"changes_expected":true|false}`, `{"status":"needs_human","reason":…}`, or `{"status":"failed","reason":…}`. SOP's reaction MUST be structural, never inferred by scanning prose:

```text
completed + changes_expected true  + non-empty change set → validate → review → gate
completed + changes_expected true  + no changes           → FAIL (claimed changes, produced none)
completed + changes_expected false + no changes           → run configured validation, then PASS
needs_human                                               → stop with NEEDS_HUMAN (requeued)
failed                                                    → FAIL (terminal)
```

Any other output MUST keep the legacy behaviour, judged by the change produced. The `claimed changes, produced none` failure is SOP's own deterministic verdict, so it MUST be classified (`NO_CHANGES_PRODUCED`) rather than left unclassified; when bounded escalation is enabled it MAY then be retried on a larger model — see [RECOVERY.md](../specs/RECOVERY.md) §8. See also [VALIDATION.md](../specs/VALIDATION.md), [QUALITY.md](../specs/QUALITY.md).

## 9. Tool-Harness Bounds and Phases

Bounds MUST be per capability. `DESIGN_TESTS`/`DIAGNOSE_FAILURE` get 12 read-only turns; `PLAN` (8) and `REVIEW` (6) run bounded read-only discovery then a tool-free **synthesis** (2 turns) that MUST produce the document, so neither can fail by exploring forever; `IMPLEMENT` (32) and `FIX` (24) share a phased loop — **discover**, **change** (first write plus targeted checks), **finalize** — so a `FIX` that wrote its repair hands back instead of running the build/tests to the ceiling. Finalization MUST be **mutation-aware**: tools are withdrawn only once a change is observed _and_ the writer has stopped; a write during finalize MUST NOT be refused and resumes change.

A phased run MUST also carry a deterministic **no-progress guard**, independent of the iteration ceiling. The only progress signal is a **successful repository mutation**: it resets the consecutive counter to zero. Every non-mutating turn increments it — a read, a search, an inspection, narration (a planning turn with no tool call and no final object), a denied tool, a repeat, and a **novel read of a file not seen before** alike. Exploration is activity, not implementation progress. After `maxNoProgressIterations` (default 5) consecutive non-mutating turns the run MUST stop early with `termination=no_progress` and a distinct `IMPLEMENT_NO_PROGRESS` / `FIX_NO_PROGRESS` diagnostic recording the observed state (mutations and changed files) — never by consuming the ceiling, and never by substituting another provider or model. Repetition of the same action is detected independently by the repetition guard. The guard applies while no mutation has been observed: once the repository has been changed the finalization lifecycle governs the run, so a productive run is unchanged.

A run that never changes the repository MUST end as a **retryable** `needs_human` (SOP requeues it), never as a hard failure: the no-progress guard stops it with `termination=no_progress` (above). A model reporting `failed` after changing nothing is likewise a retryable no-op, not a hard failure.

The two mutating ceilings are built-in defaults an operator MAY raise:
`SOP_OLLAMA_IMPLEMENT_ITERATIONS` (default 32) and `SOP_OLLAMA_FIX_ITERATIONS`
(default 24). Each MUST be a positive integer to take effect; an unset, blank,
non-numeric, or non-positive value MUST keep the default, so behavior is unchanged
when it is not set. The soft thresholds (finalize, late-stage, force-finalize) MUST
scale with the raised ceiling, so a longer loop preserves their relative steering
position instead of stranding them near the start.

