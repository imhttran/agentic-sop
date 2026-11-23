# Review context — PRD: Native Multi-Agent Orchestration

**Target:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md`

**Purpose:** reading list for fresh-context Goldfish reviews. It records paths to
inspect, not conclusions. Reviewers must verify every claim against the source.

## Rules for reviewers

- Read-only. Do not edit, stage, commit, branch, push, open a PR, or run commands
  that mutate the repository or `.agent-sdlc`.
- Cite `file:line` or a symbol for every claim about current behavior.
- Treat the PRD as a draft, not as evidence about the repository.
- No claim that a capability is missing is valid unless the reviewer searched for
  it and records the search (package, symbol, or test).
- Report findings to the orchestrator; the orchestrator writes the report files.

## Reading list

### Canonical orchestration work already delivered

- `docs/history/plans/PHASE-9-MULTI-AGENT-ORCHESTRATION.md` — Phase 9 plan
  (ORCH-001 … ORCH-012), including the task graph and acceptance criteria.
- `docs/reports/PHASE-9-MULTI-AGENT-ADOPTION.md` — ORCH-012 adoption gate
  decision: verified, opt-in, default single-agent.
- `docs/architecture/orchestration.md` — orchestration domain model and the
  single-lifecycle-authority invariant.
- `docs/plans/README.md` and `docs/plans/BACKLOG.md` — plan inventory and known gaps.

### Execution and scheduling

- `internal/domain/task.go` — `Task`, `Dependency`, `Attempt`, the transition
  table, dependency resolution, retry budget.
- `internal/scheduler/scheduler.go` — single-slot dependency-aware scheduling and
  per-invocation recovery.
- `internal/cli/drive.go` — `driveGraph`, the serial graph driver.
- `internal/parallel/parallel.go` and `parallel_test.go` — bounded,
  dependency-independent, per-task worktree execution. Check whether any
  non-test caller exists.
- `docs/specs/EXECUTION.md` §8 — the repository's own statement of parallelism
  status.

### Orchestration package

- `internal/orchestration/`: `model.go`, `decision.go`, `plan.go`,
  `decompose.go`, `assignment.go`, `worker_adapter.go`, `coordinator.go`,
  `ownership.go`, `conflict.go`, `integrate.go`, `budget.go`, `progress.go`,
  `replacement.go`, `context_routing.go`, `orchestrate.go`, and their tests.
- `internal/cli/orchestrate.go` — the `sop orchestrate` entry point.
- `internal/config/config.go` — the `orchestration:` block and defaults.
- `internal/orchadopt/orchadopt.go` and `internal/cli/gate.go` — the ORCH-012
  adoption gate.

### Durability, isolation, recovery, governance

- `internal/executionlease/lease.go` — one active execution per project, owner
  record, heartbeat, staleness, stale release.
- `internal/projectlock/lock.go` — project metadata lock.
- `internal/store/sqlite.go` — persisted task, dependency, and attempt schema.
- `internal/run/run.go` — durable run records under `.agent-sdlc/runs/<id>/`.
- `internal/resume/resume.go` — reconciliation of persisted state with observed
  branches/PRs.
- `internal/recovery/policy.go` — deterministic failure-to-action policy.
- `internal/worktreeevidence/evidence.go` — lifecycle-evidence preservation
  before worktree removal.
- `internal/worktreecleanup/cleanup.go` — classification of dirty worktree items.
- `internal/toolharness/isolation.go`, `internal/toolharness/files.go` — tool
  and filesystem isolation.
- `internal/sensitivepath/path.go`, `internal/redact/` — credential-path denial
  and redaction.
- `internal/approval/approval.go`, `internal/mergegate/gate.go`,
  `internal/commitgate/gate.go` — human and publication gates.
- `internal/runtrace/orchestration.go`, `orchestration_compose.go`,
  `orchestration_graph.go` — provenance events and graph reconstruction.
- `internal/taskbuilder/graph.go` — cycle and unknown-dependency validation.
- `internal/agent/capabilities.go`, `internal/agent/provider.go`,
  `internal/model/` — capability, provider, locality, and routing abstractions.

### Documentation conventions

- `docs/README.md` — documentation tree and authority flow.
- `docs/specs/EXECUTION.md` §10 — reuse-before-implementation policy.
- `docs/architecture/SOP-BOUNDARY.md` — single source of truth.
- `.github/workflows/ci.yml`, `scripts/checks/pre-push.sh`,
  `scripts/checks/docs-only.sh` — documentation verification gates.
