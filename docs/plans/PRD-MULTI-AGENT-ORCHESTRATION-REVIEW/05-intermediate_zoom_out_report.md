# Goldfish Review Report — Stage 8 (Intermediate Zoom Out)

**Target:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md` (full file)
**Mode:** REVIEW, Stage 8
**Reviewer:** `egm-zoom-out`
**Date:** 2026-11-23

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-zoom-out` | completed |

Severity breakdown: 0 Blocker, 4 Major, 6 Nit.

This pass followed the Stage 5 and Stage 7 corrections. It checked whether the
corrections reintroduced complexity.

## Prioritized Findings

### The bound rule duplicated existing code — Major

**Scope:** MAO-002 "Bound", architecture §4
**Finding:** `EffectiveActiveWorkers` already computes
`min(MaxActiveWorkers, MaxConcurrency)` floored at 1, and
`ExecutionPolicy.concurrency()` already maps zero to 1. The new rule restated it,
added a fourth limit, and used a literal cap of 8 that did not match the existing
defaults.
**Resolution:** Removed the fourth limit and the literal cap. The graph bound is
`EffectiveActiveWorkers`; `max_parallel_tasks` aliases `MaxConcurrency`; the
existing budget clamps to `MaxWorkers`.

### `RootReadOnly` already exists as an enforced boundary — Major

**Scope:** MAO-004, architecture §6
**Finding:** The PRD said a capability label is not an isolation boundary (true)
but missed that `toolharness.RootReadOnly` refuses write tools and
repository-mutating shell commands. Requiring a worktree per read-only unit was
defense-in-depth presented as the only boundary.
**Resolution:** Non-mutating units use `RootReadOnly`; mutating units use a
worktree; a read-only unit that runs build/test commands also gets a worktree.

### Patch integration was a no-op phase — Major

**Scope:** MAO-008, Phase MAO-C
**Finding:** In v1 nothing applies a patch, mutating units are serial, and the
default is single-writer, so there is no concurrent mutation to integrate. The
stale-base test had no reachable path.
**Resolution:** MAO-008 is P2/deferred with an activation trigger; Phase MAO-C is
operator visibility only.

### Several acceptance criteria had no owning test — Major

**Scope:** §13, plan tests
**Finding:** Seven criteria had no named test in the five-stage plan.
**Resolution:** Added a cross-cutting verification table; each criterion maps to a
planned test and a test file.

### Mid-batch budget exhaustion was stated four times — Nit

**Scope:** MAO-006/007, §14, architecture §7
**Finding:** The same rule was duplicated and could drift.
**Resolution:** The normative statement is in MAO-007; other sections reference it.

### The shared predicate had no home — Nit

**Scope:** MAO-002
**Finding:** "Delegate to the shared predicate" without naming its package invited
a third copy or a `parallel → scheduler` import.
**Resolution:** The predicate moves to `internal/domain` as a pure function;
`Next`, `NextBatch`, and `parallel.Select` are callers.

### Status vocabulary reappeared in the visibility requirement — Nit

**Scope:** MAO-010
**Finding:** "queued, running, completed, failed, unknown" is not a domain
vocabulary.
**Resolution:** Renders domain status plus attempt outcome.

### `workflow_id` had no owning concept — Nit

**Scope:** MAO-003, architecture §5
**Finding:** One plan per leased run makes `workflow_id` a second name for
`plan_id`, with drift risk.
**Resolution:** Derive `workflow_id` from `plan_id`; persist `work_unit_id` and
attempt attribution only.

### Deferred requirements had no stage — Nit

**Scope:** MAO-009/011, D4
**Finding:** Requirements with acceptance criteria but no stage have permanently
unreachable tests. D4 asked the operator to choose a model the architecture had
already ruled out. `--parallel N` collided with `--max-tasks`.
**Resolution:** MAO-009/011 are deferred with activation triggers; D4 is resolved;
the flag is `--parallel-units`.

### The architecture diagram still showed a shared snapshot — Nit

**Scope:** Architecture §2
**Finding:** The diagram contradicted the corrected isolation rule.
**Resolution:** Diagram updated to `RootReadOnly` for non-mutating units and a
worktree for mutating units.
