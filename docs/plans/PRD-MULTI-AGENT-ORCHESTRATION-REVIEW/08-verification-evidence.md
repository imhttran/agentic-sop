# Verification evidence

**Date:** 2026-11-23
**Baseline:** `main` @ `2462bdc1`
**Scope:** documentation-only change. No production source, tests, configuration,
or `.agent-sdlc` state was modified by this work.

## Commands and observed results

### Documentation gates

```text
$ bash scripts/checks/test-docs-only.sh
docs-only self-check: OK

$ bash scripts/checks/pre-push.sh --base origin/main
broken links: 0
pre-push: OK (formatting; documentation links on the merge of HEAD with origin/main)
```

The pre-push gate covers formatting and the documentation link check on the merge
of the working tree with `origin/main`, matching the CI `check` job.

### Focused Go verification (baseline, unchanged by this work)

```text
$ go test ./internal/orchestration/... ./internal/scheduler/... ./internal/parallel/... \
           ./internal/store/... ./internal/domain/... ./internal/taskbuilder/...
ok  internal/orchestration
ok  internal/scheduler
ok  internal/parallel
ok  internal/store
ok  internal/domain
ok  internal/taskbuilder

$ go build ./...
(clean)
```

### Adoption gate

```text
$ go run ./cmd/sop gate orchestration
Phase 9 multi-agent adoption gate (ORCH-012)
cases: 3
metric                   baseline  candidate
task_success                    3          3
verification_success            3          3
worker_count                    3          8
tool_calls                      3          8
integration_attempts            0          3
context_bytes                 203        564
conflicts                       0          0
repository_mutations            0          0

decision: REJECT
reason: multi-agent does not demonstrate a deterministic benefit over the
single-agent baseline; default execution remains single-agent
not measured:
  - wall-clock time (deterministic fake agents have no meaningful latency)
  - provider cost and token counts (no reliable provider-independent accounting)
```

This result is recorded in the PRD as a safety/cost result, not as evidence about
the economics of concurrency.

## Repository checks used to verify claims

The following searches and reads were used to confirm the baseline statements in
the PRD and architecture. They are read-only and do not change the repository.

- Non-test callers of `internal/parallel`: none.
- Non-test callers of `ComposeOrchestrationEvents`, `WithOrchestrationCollector`,
  `EmitFromContext`, and `ReconstructOrchestrationGraph`: none.
- Non-test callers of `OwnershipDisjointMultiWriter`: none.
- `sop orchestrate` contains no `executionlease`, `run.New`, `runtrace`, or store
  call, and passes `context.Background()`.
- `internal/perf/budget.go` fails closed when `SOP_AGENT_MAX_TOTAL_TOKENS` is set
  and no `Usage` is attached to the context.
- `internal/store/sqlite.go` replaces a task's attempts on save and has no
  compare-and-set; `schemaVersion = 7`.
- `internal/executionlease/lease.go` has `KindRun` and `KindPrompt` only; the lease
  is off by default.
- `internal/toolharness/harness.go` documents the harness as safe for sequential
  use, not concurrent calls; `RootReadOnly` is enforced for write tools and
  repository-mutating shell commands.
- `internal/scheduler/scheduler.go` selects one task and is not safe for concurrent
  use; `resumeActiveTask` refuses more than one active task.
- `internal/worktreeevidence/evidence.go` archives by plan id.
- `docs/specs/EXECUTION.md` §8 records the worktree runner as Partial and
  `max_parallel_tasks` as not yet a configuration key.

## Not verified

- A multi-task concurrent run on a live provider: not attempted. It would change
  behavior and requires the operator decisions in the PRD.
- Plan activation through `sop plan activate`: not run, because it mutates project
  lifecycle state. The plan follows the documented `## Project`, `## Summary`, and
  `## MAOP-00x` grammar.
- Wall-clock and token-cost benefit: not measured. The adoption gate reports these
  dimensions as not measured.
