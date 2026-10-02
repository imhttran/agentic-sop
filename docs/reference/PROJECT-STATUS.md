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
human-decision surface (6), are implemented; routing and escalation are OFF by
default. See [docs/plans/BACKLOG.md](../plans/BACKLOG.md).

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

## Known limitations (current)

Deliberate, understood residuals that are not scheduled work. Each is described where
its behavior is defined; this list exists so none is lost, and scheduled future work is tracked separately in [the backlog](../plans/BACKLOG.md).

- **The Windows clean-room test has not been run** (Phase 5.6, P56-011). CI runs the
  `windows` job against temporary directories on `windows-latest`; a real machine's
  `%USERPROFILE%`, PATH, Zed, and Claude Code are unverified. See
  [../testing/WINDOWS-CLEAN-ROOM.md](../testing/WINDOWS-CLEAN-ROOM.md).
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
- **Capabilities designed but not delivered are documented in place, not here:** the TDD
  test-design stage ([../specs/TASK-LIFECYCLE.md](../specs/TASK-LIFECYCLE.md)),
  `max_parallel_tasks` ([../specs/EXECUTION.md](../specs/EXECUTION.md)), and the CLI-driven
  push → PR → CI → merge loop ([../architecture/OVERVIEW.md](../architecture/OVERVIEW.md));
  the `jev` decision provider is recognized but not implemented
  (`internal/decision/decision.go`).

The project is intentionally built incrementally: sequential correctness and
recovery come before parallelism, and each stage is usable and tested before the
next is added.

