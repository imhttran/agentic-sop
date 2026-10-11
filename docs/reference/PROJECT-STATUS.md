# Project Status

**Type:** Descriptive reference

Implemented capabilities and known limitations, extracted from the project landing
page and backlog. Future work belongs in [BACKLOG.md](../plans/BACKLOG.md); completed
fix observations belong in [RESOLVED-BACKLOG.md](../history/RESOLVED-BACKLOG.md).
These records do not override the behavior defined in [the specifications](../README.md#specifications).

## Project overview

SOP V1 is complete: dependency-aware execution, isolated branches, test-first
implementation, bounded review/fix loops, CI and merge gates, durable state,
resume/recovery, environment bootstrap, and limited parallelism. Above it, model
routing (3.5), the provider runtime (4), bounded model escalation (5), unified work
items with `sop prompt` (5.4), and distribution — one installer for macOS/Linux and
Windows, a native Claude Code plugin, and CI (5.5–5.6), and an interactive
human-decision surface (6), are implemented; routing, escalation, and replanning
are OFF by default. See [docs/plans/BACKLOG.md](../plans/BACKLOG.md).

## Implemented work

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
Two plan shapes compile deterministically (no agent): the rendered
`## <id> — <title>` form, and the `## Tasks` + `### <id> — <title>` form the phase
plans use, with explicit dependencies and a load-bearing `Execution:` field
Unified installer `./install.sh` (CLI + per-agent skills + Claude plugin) and the
native Claude Code plugin package, generated from `skills/` (Phase 5.5)
GitHub Actions CI: fmt, vet, build, test, race, doc links, installer + plugin packaging
Windows distribution: native `install.ps1` (no WSL/Bash/Make/admin, copied skills with a
`.sop-managed` ownership marker, safe update and uninstall) + a `windows-latest` CI job
(Phase 5.6; the clean-room test on a real Windows machine is still outstanding)
SOP agent skills (`/sop`, `/sop-plan`, `/sop-review`, `/sop-diagnose`, `/sop-test`,
`/sop-implement`) for Zed and Claude Code + `make install-skills` (Phase 5.4 hardening)
Recurring, closing steering for an unmutated IMPLEMENT/FIX run (T076): the
implement-now instruction is re-stated every interaction, and the turns before the
late-stage cutoff tell the model the invocation is about to end
Local-first model tiers (routing/configuration hardening): SMALL runs a local
model and switches to its configured cloud fallback (`nemotron-3-nano:30b-cloud`)
only when a read-only observation says the local runtime cannot serve it; MEDIUM is
`nemotron-3-super:cloud` and LARGE is `deepseek-v4.1-flash:cloud`
Structured run trace (`trace.json`, schema 4): a versioned, observational record
of a run's execution identity, iterations, verification, and termination, plus
structured progress signals — discovery, repository mutation, verification, and
state transition — that distinguish progress from activity (AGENT-001/AGENT-002)
Agentic evaluation harness: deterministic fixtures evaluated against a run's
`trace.json` (AGENT-003)
Canonical execution budget: explicit, configurable iteration/stale/tool-call limits
(`internal/budget`) recorded in `trace.json` (AGENT-004)
Bounded strategy replanning: an opt-in, deterministic permission to change strategy
once on the same class after a recoverable failure, recorded observationally in
`trace.json` (AGENT-005)
Validation enforcement: a task that changes the repository cannot pass without
configured validation. The gate fails closed and the failure is classified
separately as `VALIDATION_NOT_CONFIGURED` (a terminal operator-intervention block,
not a human approval). The terminal block persists that reason
(`domain.VALIDATION_NOT_CONFIGURED`) rather than an exhausted retry budget, and a
configless `sop run --task` materializes the documented default configuration
(Go build/test/vet/gofmt)
```

Candidates recorded at the time of this status summary (scheduling remains in
[the backlog](../plans/BACKLOG.md)):

```text
OpenAI-style tool calling for openai_compatible (Phase 3)
Local network service (team mode)
Small-device dashboard
Jev adapter + Jev-vs-deterministic evaluation
```

The earlier interactive-approval candidate is now **implemented**:
[PLAN-Phase-6-Interactive-Approval.md](../plans/PLAN-Phase-6-Interactive-Approval.md)
(Phase 6, `P6-001`–`P6-011`). `sop approvals` and the read-only
`sop reconcile --list-changed` — the two SOP operations an external consumer needed to
reach — landed as `P6-001`–`P6-004`; the interactive surface followed: a parked run
names the gate and the commands that resolve it, the applicable gates can be chosen at
a terminal (fail-closed off one), and `sop approve <id> --run` records the decision
before starting the ordinary run. See [../guides/APPROVALS.md](../guides/APPROVALS.md).

## Recent correctness closure

- **PLAN/REVIEW synthesis corrections are implemented.** Prohibited synthesis
  tool requests use a separate bounded correction allowance; ordinary narration
  still consumes the synthesis-attempt budget. Existing tests verify this work;
  it is not a new implementation task in the closure plan.
- **Bounded productive IMPLEMENT/FIX discovery is implemented.** Successful novel
  inspections can earn discovery credit in the initial window, separately from
  mutation progress. Task-scoped discovery budgets remain backlog work.
- **Tool-level repository mutation verification is implemented** (`d0af5a4`).
  Mutation-capable operations execute once and earn progress only on a successful,
  verified before/after state change. Same-content writes and unchanged formatting
  no longer manufacture progress.
- **Verified already-satisfied IMPLEMENT/FIX completion is implemented** (`1ce9bdb`).
  An explicit `ALREADY_SATISFIED` outcome requires caller-owned acceptance mappings,
  successful executed validation, and a known unchanged repository. Reports
  preserve zero mutations and the proof; a model claim alone has no authority.

The authoritative rules for these fixes are in
[AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md). This delivery record does not imply
that the cross-repository closure or performance baseline has passed.

## Pre-performance closure (complete)

[PLAN-Pre-Performance-Closure.md](../history/plans/PLAN-Pre-Performance-Closure.md) is
**COMPLETE**, with CLOSE-001–CLOSE-011 covering source/config capture, reconciliation,
both repositories' deterministic gates, current controller work, named-plan
dogfood, human decisions, resume/idempotency, telemetry inventory, four workloads
with three real-provider repetitions each, and the final readiness verdict.
The plan has been executed: all eleven CLOSE tasks are `LOCAL_DONE` and the plan's final gate passed. It introduces no new performance architecture and
does not start the performance phase.

### Current checkout verification (2026-10-02)

> **Superseded.** This records a point-in-time check at `1ce9bdb`. The twelve
> CLI/JEV no-change tests below now pass and the pre-performance closure is
> complete (CLOSE-001–CLOSE-011 `LOCAL_DONE`, final gate passed); see
> [BACKLOG.md](../plans/BACKLOG.md) and the
> [closure reports](../reports/pre-performance-closure/).

At `1ce9bdb` on `fix/verify-repository-mutations`, focused mutation-verification and
already-satisfied tests passed while the full suite was not green: these twelve
CLI/JEV tests failed, reproduced on predecessor `d0af5a4`:

```text
TestRunImplementNoChangesStillValidates
TestJEVNoChangeFinalInvocationReviewsAccumulatedEvidence
TestJEVTaskEvidenceIsBounded
TestJEVLegacyBootstrapSkippedWhenEvidenceExists
TestJEVLegacyBootstrapRecoversActivityEvidence
TestJEVLegacyBootstrapUnionsInvocations
TestJEVLegacyBootstrapDeduplicates
TestJEVLegacyBootstrapBounded
TestJEVLegacyBootstrapIgnoresUnrelatedDirtyFile
TestJEVLegacyBootstrapFiltersSOPOwnedPaths
TestJEVLegacyBootstrapIsIdempotent
TestJEVLegacyBootstrapDoesNotModifyRepository
```

At that revision they were unresolved closure-gate failures, not permission to
weaken mutation or completion checks. Under the completed closure the changed
expectations are reconciled and all twelve tests pass in the current deterministic
gate; this historical record asserts no baseline measurement or readiness verdict.

## Pre-JEV readiness (complete)

The PREJEV018 pre-JEV readiness gate is satisfied: the deterministic lifecycle,
harness/provider/provider-model separation, recovery/resume, the controller
boundary, and both repositories' build/test matrices are verified, and the
remaining-issues register carries no CRITICAL or HIGH issue. See
[PREJEV018-READINESS-GATE.md](../history/PREJEV018-READINESS-GATE.md) (Current
verification, 2026-10-06).

## Phase 7 — Agentic reliability and evaluation

```text
AGENT-001 Structured Run Trace     COMPLETE   trace.json (schema 4), observational
AGENT-002 Progress Signals         COMPLETE   discovery / mutation / verification / transition
AGENT-003 Evaluation Harness       COMPLETE   deterministic fixtures over trace.json
AGENT-004 Agent Budgets            COMPLETE   canonical limits (internal/budget), configurable + traced
AGENT-005 Replan Strategy          COMPLETE   opt-in bounded strategy change (internal/recovery), traced + evaluated
```

Evaluations observe completed run evidence; they do not participate in execution or
lifecycle decisions. See [EVALUATION.md](../reference/EVALUATION.md).

## Phase 8 — Context and execution efficiency

Phase 8 (Context & Execution Efficiency) is **COMPLETE** and closed, and is not
reopened. Its twelve CTX tasks shipped the deterministic, model-free context and
execution-efficiency layer:

```text
CTX-001 Context Engine               COMPLETE    canonical deterministic context (internal/context)
CTX-002 Structural Repository Index  COMPLETE    deterministic index + identity (internal/repoindex; sop index)
CTX-003 BM25 Retrieval               COMPLETE    deterministic lexical ranking (internal/retrieval; sop retrieve)
CTX-004 Retrieval Evaluation Gate    COMPLETE    PASS: BM25 beats the unranked baseline (sop gate retrieve)
CTX-005 Prompt Compiler              COMPLETE    bounded, deterministic model input (internal/prompt)
CTX-006 Response Normalizer          COMPLETE    structural, fail-closed normalization (internal/normalize)
CTX-007 Verification Cache           COMPLETE    opt-in per invocation (sop validate --cache)
CTX-008 Prompt Result Cache          COMPLETE    opt-in, read-only capabilities (sop prompt --cache)
CTX-009 Vector Retrieval Evaluation  EVALUATED   REJECTED: no improvement over BM25; not shipped
CTX-010 Decision Memory              COMPLETE    operator CLI + opt-in run context (context.decision_memory)
CTX-011 Adaptive Routing             COMPLETE    evidence-driven class selection (internal/adaptiveroute)
CTX-012 Automatic Prompt Tuning      COMPLETE    evaluation-gated promotion (internal/prompttuning)
```

Provider/model neutrality of the Phase 8 core is enforced by an architecture test
(`internal/archtest`): no Phase 8 core package imports a provider implementation, and
the context → retrieval → prompt → agent → normalize pipeline is adapter-independent.
The live-path integration is complete (POST8-001). The plan lifecycle gained a
deterministic, model-free `sop plan complete`. A persistent cross-run verification cache
remains deferred (VERIFCACHE-PERSISTENCE) and is not a Phase 8 gap. See
[BACKLOG.md](../plans/BACKLOG.md) and
[history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md](../history/plans/PHASE-8-CONTEXT-EXECUTION-EFFICIENCY.md).

## Known limitations (current)

Deliberate, understood residuals that are not scheduled work. Each is described where
its behavior is defined; this list exists so none is lost, and scheduled future work is tracked separately in [the backlog](../plans/BACKLOG.md).

- **The Windows clean-room test has not been run** (Phase 5.6, P56-011). CI runs the
  `windows` job against temporary directories on `windows-latest`; a real machine's
  `%USERPROFILE%`, PATH, Zed, and Claude Code are unverified. See
  [../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md).
- **Outer IMPLEMENT completion is invocation-scoped (resolved).** It derives evidence from before/after repository snapshots plus harness-reported changed files; the whole-tree diff is used only in verify-first mode. See [AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md#verified-operation-level-mutation).
- **No project-scope skill install on Windows.** On Unix,
  `scripts/install/install-skills.sh --project [DIR]` installs the skills into a
  project's `.agents`/`.claude` directory; `install.ps1` has no `-Project` equivalent
  yet.
- **The Windows CI job runs a scoped test set, not the full suite.** Several lifecycle
  tests execute POSIX commands (`true`/`false`/`sh`), so that job covers the build, vet,
  static PowerShell parsing, and the distribution surface under both Windows PowerShell
  and `pwsh`; the full suite and `-race` stay on Linux.
- **Prompt run ids are unique locally, not globally.** `promptRunID` is
  second-resolution plus a collision counter, so two prompts in the same second never
  share a directory, but two machines can mint the same id.
- **`sop-ollama-agent` built from the pinned revision predates the `-version` flag**, so
  `scripts/install/install-sop-ollama-agent.sh` rebuilds instead of short-circuiting on a
  revision match. The installer handles it gracefully; a pinned revision that reports
  its revision would make repeated installs a no-op.
- **Ad-hoc runs have no resolvable approval gate.** `sop run --task TASK.md` and
  `sop prompt --capability implement` park at a human boundary like a graph run, but
  their run id is not a stored task, so `sop approvals`/`sop approve`/`sop decline` do
  not apply to them. They print the boundary and how to continue instead of naming a
  command that would fail (Phase 6, P6-005).
- **The local-first fallback applies to the class the router or `--model-class`
  selects, not to the run-level default agent or an escalation attempt.**
  `applyModelRouting` (the run-level default) and `escalateTo` (Phase 5) resolve
  without an availability probe, so a `local` class used as the run-level default
  (routing off) or reached by escalation runs its local model even when the local
  runtime cannot serve it. The fallback is owned by the per-task and per-prompt
  routing seam in `internal/cli`. See
  [../specs/MODEL-ROUTING.md](../specs/MODEL-ROUTING.md) §"Local-First Fallback".
- **The full `internal/cli` suite can stall in the SQLite store on constrained
  machines.** Under the whole-package load, `store.Open`'s schema migration
  (`internal/store/sqlite.go`, `modernc.org/sqlite`) can block in `fsync` long enough
  to time out the package — `go test ./internal/cli` did not finish within 120s —
  while `internal/store` and any single test pass in under a second in isolation.
  It reproduces on an unmodified checkout and is unrelated to the validation
  enforcement; targeted `-run` selection of the affected tests completes normally.
  No test was weakened, skipped, or bypassed to work around it. The store is
  unchanged; diagnosing the `fsync` stall itself is separate, unscheduled work.
- **Capabilities designed but not delivered are documented in place, not here:** the TDD
  test-design stage ([../specs/TASK-LIFECYCLE.md](../specs/TASK-LIFECYCLE.md)),
  `max_parallel_tasks` ([../specs/EXECUTION.md](../specs/EXECUTION.md)), and the CLI-driven
  push → PR → CI → merge loop ([../architecture/OVERVIEW.md](../architecture/OVERVIEW.md));
  the `jev` decision provider is recognized but not implemented
  (`internal/decision/decision.go`).

The project is intentionally built incrementally: sequential correctness and
recovery come before parallelism, and each stage is usable and tested before the
next is added.
