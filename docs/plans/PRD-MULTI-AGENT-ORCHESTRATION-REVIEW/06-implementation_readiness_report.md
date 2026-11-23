# Goldfish Review Report — Stage 9 (Implementation Readiness)

**Target:** `docs/plans/PLAN-MULTI-AGENT-ORCHESTRATION.md` (detailed range)
**Mode:** REVIEW, Stage 9
**Reviewers:** `egm-implementation-readiness`, `egm-codebase-details`, `egm-style`
**Date:** 2026-11-23

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-implementation-readiness` | completed |
| `egm-codebase-details` | completed |
| `egm-style` | completed |

Severity breakdown after deduplication: 6 Blocker, 12 Major, 11 Nit.

## Prioritized Findings

### `internal/domain/selection.go` already exists — Blocker

**Lens:** `egm-implementation-readiness`, `egm-codebase-details`
**Scope:** MAOP-002 files
**Finding:** The file exists and defines `(*Task).IsBlockedSelectable`, consumed by
the scheduler. Marking it `[Create]` would collide with a live predicate.
**Resolution:** Marked `[Modify]`; the plan names the new symbol
`EligibleTasks(tasks, max)` and states its body.

### `Store.Save` would erase attribution — Blocker

**Lens:** `egm-implementation-readiness`, `egm-codebase-details`
**Scope:** MAOP-002, architecture §5
**Finding:** `replaceAttempts` deletes and reinserts from `task.Attempts`;
`loadAttempts` selects only six columns. Without extending both, the new columns
are lost on the next save or promotion.
**Resolution:** Added `domain.Attempt`, `insertAttempts`, and `loadAttempts` to the
file list; the orchestration path uses `AppendAttempt`. An interleaving test is
required.

### `N > 1` would run mutating units in one checkout — Blocker

**Lens:** `egm-implementation-readiness`
**Scope:** MAOP-002
**Finding:** `driveGraph` executes in the plan worktree; isolation arrives only in
MAOP-004. Wiring `NextBatch` at `N > 1` in MAOP-002 would run two mutating units in
one checkout.
**Resolution:** MAOP-002 wires `N > 1` only for non-mutating units; a mutating unit
request with `N > 1` is refused; mutating execution remains serial until MAOP-004.

### The predicate was under-specified — Major

**Lens:** `egm-implementation-readiness`
**Scope:** MAOP-002
**Finding:** `parallel.Select` accepts `PLANNED|READY`, `Next` requires `PLANNED`
plus `CanTransitionTo(READY)`, and `Next` has an in-flight short-circuit and a
Tier-2 recovery requeue. "One shared predicate" did not say how these compose.
**Resolution:** `EligibleTasks` covers `PLANNED|READY` with satisfied dependencies;
`Next` adds its promotion check; `NextBatch` ignores in-flight tasks and does not
run the recovery requeue; all three behaviors are tested.

### The shared agent lived in `Coordinator`, not `WorkerAdapter` — Major

**Lens:** `egm-implementation-readiness`
**Scope:** MAOP-004
**Finding:** `NewWorkerAdapter` already takes an agent per call; the shared instance
is `Coordinator.agent`. The plan pointed at the wrong file and did not define the
factory shape.
**Resolution:** MAOP-004 replaces `Coordinator.agent` with a `WorkerFactory`
(`New(ctx, unit) (agent.Agent, error)`); `NewCoordinator(a)` remains a
compatibility adapter; `orchestrate.go` is in the file list.

### "Reconcile N or prove" was an unresolved either/or — Major

**Lens:** `egm-implementation-readiness`
**Scope:** MAOP-004
**Finding:** The plan left the implementer to choose between two designs.
**Resolution:** Mutating units stay serial, so the existing single-active-task
`resumeActiveTask` is unchanged; multi-active resume is deferred.

### The graph bound had no transport or precedence — Major

**Lens:** `egm-implementation-readiness`
**Scope:** MAOP-002
**Finding:** How `N` reaches `driveGraph`, the precedence among
`max_parallel_tasks`, `max_concurrency`, and CLI flags, and whether
`orchestration.enabled` is required were unrecorded. `--max-tasks` already exists.
**Resolution:** The plan defines `--parallel-units N` on `sop run`, requires
`orchestration.enabled: true` for `N > 1`, maps `max_parallel_tasks` to
`MaxConcurrency`, and builds the bound through `EffectiveActiveWorkers`.

### Cross-cutting tests had no owning stage or file — Major

**Lens:** `egm-implementation-readiness`
**Scope:** Cross-cutting verification table
**Finding:** Seven tests were named but not attached to a stage test list or a
plausible file; one was assigned to a stage whose files could not host it.
**Resolution:** Each test is now in its stage's test list and maps to a specific
test file in the cross-cutting table.

### `sop attempts` does not exist — Major

**Lens:** `egm-codebase-details`
**Scope:** PRD operator visibility
**Finding:** The PRD cited a non-existent subcommand.
**Resolution:** Removed; baseline cites `sop status` and `sop report` only.

### `MaxWorkers` was not a bound — Major

**Lens:** `egm-codebase-details`
**Scope:** MAO-002
**Finding:** `EffectiveActiveWorkers` does not clamp to `MaxWorkers`; the PRD
claimed it did.
**Resolution:** Added the clamp and a test to MAOP-002.

### Trace truncation was attributed to the wrong package — Major

**Lens:** `egm-codebase-details`
**Scope:** MAO-007, MAOP-001
**Finding:** `internal/activity` has no cap; the bounded collector is
`runtrace.Collector`. The plan did not list `runtrace/collector.go` or `trace.go`.
**Resolution:** Both files are in MAOP-001; the wording names `runtrace.Collector`.

### `sop orchestrate` has no concurrency input — Major

**Lens:** `egm-codebase-details`
**Scope:** MAOP-001
**Finding:** `sop orchestrate` accepts one argument and refuses flags; a
"concurrency requested" refusal cannot be tested there.
**Resolution:** Moved the criterion and test to `sop run --parallel-units`.

### Decision D5 was used for two different things — Blocker

**Lens:** `egm-codebase-details`
**Scope:** MAOP-003, PRD §15
**Finding:** D5 was defined as patch integration, but the plan used it for
visibility.
**Resolution:** Added D10 (operator visibility); MAOP-003 gates on D10; D5 remains
patch integration.

### D4 resolution state differed across documents — Major

**Lens:** `egm-codebase-details`
**Scope:** PRD, plan, architecture
**Finding:** D4 was resolved in one place and open in another.
**Resolution:** D4 is marked resolved everywhere; MAO-B gates only on D1.

### Style and calibration — Major/Nit

**Lens:** `egm-style`
**Findings and resolutions:**

- Status claimed "EGM-reviewed" before the reports existed → calibrated; the
  final status reflects the completed review.
- §16 listed reports before they existed → the reports now exist; the directory is
  the evidence.
- Workflow D stated the deferred per-endpoint bound as current → calibrated.
- "Never been connected" overstated the gap → calibrated to "not connected to the
  lifecycle driver".
- "Smallest" superlatives had no comparison basis → replaced with "minimal" and a
  stated constraint.
- `archtest` was undefined → "guard test".
- "Cannot measure" → "does not measure".
- "Would convert" → "would risk".
- Duplicate file entries and duplicate out-of-scope bullets → removed.
- Decision counts D1–D9 → D1–D10.
- "Each has a named test" → "each maps to a planned test".
- Multi-active resume wording was contradictory → deferred explicitly while
  mutating units stay serial.
