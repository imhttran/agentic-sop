# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](../PLAN.md); this file is
the running list in between.

## Bootstrap resilience: run the Ollama agent as a known-good binary

_RESOLVED: implemented._ The bootstrap agent can break itself while editing
`internal/ollamaagent`: a small compile error in that very package removes the agent
needed to repair it, because SOP builds and runs the harness from the same tree under
edit. Recovery must not depend on the tool the change is allowed to break.

A pinned, prebuilt known-good `sop-ollama-agent` binary is therefore installed
**outside** the tree under edit (`make install-ollama-agent` →
`scripts/install-sop-ollama-agent.sh`, revision pinned in `scripts/sop-ollama-agent.pin`),
and the command bootstrap (`scripts/sop-ollama-agent.sh`) invokes that installed binary
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

## Local network service (team mode)

Share SOP state and control across a team on the local network.

## Small-device dashboard

A read-only view of SOP state for small devices.

## Jev adapter and Jev-vs-deterministic evaluation

An adapter for the Jev planner, and an evaluation comparing it with the
deterministic planner.

## Bootstrap agent exhausts its budget on multi-package tasks

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

## Status and roadmap

SOP V1 is complete; every V1 stage is implemented and tested:

```text
Repository / CI · Domain Model · Workflow State Machine · SQLite Persistence
Persistence Corrections · CLI Foundation · PRD -> Plan · Plan -> Tasks
Dependency DAG · Scheduler · Git Adapter · Test Runner · Agent Harness
TDD Task Runner · Structured Self Review · Open Code Review Integration
Commit / Documentation Gate · GitHub Adapter · GitHub Actions Integration
CI Remediation · Merge Gate · Completion Loop · Resume / Recovery
Environment Bootstrap · Controlled Parallel Execution · Documentation Automation
End-to-End Dogfooding
```

Implemented on top of the V1 core (wrap-up work):

```text
Local model providers (Ollama, OpenAI-compatible llama.cpp)
Project configuration (.agent-sdlc/config.yaml)
Configuration-driven agent selection
Markdown task-file loader (sop plan TASK.md)
Deterministic quality gate · Command policy
Validation runner (sop validate) · Review stage (sop review)
Run lifecycle and run state (sop run)
Bounded fix loop (review -> fix -> re-validate)
Dependency-aware graph execution (sop run over the task graph)
Provider capability detection
Git workflow commands (sop commit, sop pr)
MCP server (sop mcp) · Optional decision layer (deterministic + routing)
Run report command (sop report) · Evaluation harness (sop eval)
Early JEV checkpoints (Phase 3) · Deterministic model-class routing (Phase 3.5)
Provider runtime + capability discovery (Phase 4) · Bounded model escalation (Phase 5)
Unified work items + governed `sop prompt` (Phase 5.4)
Declared-complete plan stages (`execution_mode: done`): a plan whose work already
exists records its stages as satisfied instead of re-implementing them
SOP agent skills (`/sop`, `/sop-plan`, `/sop-review`, `/sop-diagnose`, `/sop-test`,
`/sop-implement`) for Zed and Claude Code + `make install-skills` (Phase 5.4 hardening)
```

Next candidates, in the plan's build order:

```text
Interactive approval workflow
Local network service (team mode)
Small-device dashboard
Jev adapter + Jev-vs-deterministic evaluation
```

The project is intentionally built incrementally: sequential correctness and
recovery come before parallelism, and each stage is usable and tested before the
next is added.
