# SOP Backlog

Future work that is known but not yet scheduled into a numbered task. When an item
is picked up it becomes a `task(T0NN)` entry in [PLAN.md](../PLAN.md); this file is
the running list in between.

## Bootstrap resilience: run the Ollama agent as a known-good binary

The bootstrap agent can break itself while editing `internal/ollamaagent`: a small
compile error in that very package removes the agent needed to repair it, because
SOP builds and runs the harness from the same tree under edit.

Install (and run) a pinned, prebuilt known-good `sop-ollama-agent` binary — outside
the tree under edit — so a broken working copy can still be repaired by the agent.
Recovery must not depend on the tool the change is allowed to break.

## Default config cannot IMPLEMENT (provider/harness split unfinished)

AHV2008 flipped new-project defaults to `harness: tool, provider: ollama`, and
AHV2002 narrowed the native Ollama provider to text-only capabilities. Until the
`tool` harness is actually wired beneath the provider, a project on the default
config fails the up-front `guardCapability(a, IMPLEMENT)` check ("provider cannot
IMPLEMENT") because the native Ollama provider has no tools. Either wire the tool
harness end to end, or keep the default `provider: command` until then, so a fresh
`sop init` project can run a task.

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
finalizing; or use a stronger model for multi-package tasks. Related: SOP's plan
normalizer has no `execution_mode` field (`internal/planner/planner.go`), so a plan
cannot mark an already-implemented task `verify-first` to avoid the re-implement
loop.

## A fully-implemented plan cannot close through SOP

`sop run <PLAN>.md` regenerates tasks from the plan's stages and starts each at
`PLANNED` (`internal/taskbuilder/taskbuilder.go`); the plan's per-task `Status:`
line is documentation only and is not read. When every stage is already
implemented — for example a plan whose tasks were finished in a prior session —
the agent finds nothing to change, the run classifies as
`INCOMPLETE_IMPLEMENTATION` → `CONTINUE` (`internal/failure`), and after the retry
budget the task ends `BLOCKED`. Work that is already green therefore cannot be
closed out through the lifecycle.

Options: teach the planner/task builder to honour an explicit completed marker (or
an `execution_mode: verify-first` / `done` field) so an already-satisfied stage is
accepted; or provide a `sop reconcile`-style path that records an externally
finished task. Related: "Bootstrap agent exhausts its budget on multi-package
tasks" above.

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
