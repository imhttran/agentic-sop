# Goldfish Review Report — Stage 6 (Comprehension Test)

**Target:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md` (full file)
**Mode:** REVIEW, Stage 6
**Date:** 2026-11-23

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-comprehension` | completed |

## Paraphrase

### What this proposes

SOP already has a complete single-process orchestration model (assignment
contracts, dependency resolution, write-ownership admission, conflict detection,
budgets, provenance types, an execution coordinator), but it is either unwired or
unsafe to run concurrently. The gap is the wiring: `sop orchestrate` does not take
the project execution lease, persists nothing, has no token accounting, shares one
harness across workers while the harness is documented as unsafe for concurrency,
and the task store does no compare-and-set. The PRD therefore specifies a minimal
extension: one leased SOP process running a bounded number of independent work
units, each with its own identity, isolation, persistence, and evidence, while
`N=1` and single-agent behavior stay unchanged and orchestration stays off by
default. It uses 13 requirements (`MAO-001` … `MAO-013`), security invariants, a
reuse-first model, phases MAO-A … MAO-E, and operator decisions D1–D10.

### How the existing system works, per this document

There is one lifecycle authority (`internal/domain`) that owns statuses and
transitions; a scheduler (`internal/scheduler`) that selects exactly one eligible
task at a time; an opt-in orchestration layer (`internal/orchestration`) that
models assignments, budgets, roles, ownership, and conflict but is not connected to
the lifecycle driver; and `internal/parallel`, a one-shot bounded worktree runner
with no production caller. `sop run` drives the task graph serially and holds a
single-owner project execution lease (`internal/executionlease`) that is off by
default; `sop orchestrate` runs one task file with a deny-all write scope, does not
take the lease, and persists nothing. Durability is SQLite (`internal/store`) with
attempts keyed `(task_id, number)` and no versioning, plus `internal/resume` and
`internal/recovery` policies. Adoption is controlled by a gate (`internal/orchadopt`,
`sop gate orchestration`) whose ADOPT branch is unreachable for a real multi-unit
decomposition.

## Findings

### Lease rule was ambiguous for `N=1` — Major

**Scope:** MAO-001
**Finding:** The requirement said "any orchestrated execution acquires the lease"
but exempted `N=1` on the "existing single-agent path"; `sop orchestrate` is not
the single-agent path.
**Resolution:** Added an explicit command × N × lease-mode table; the rule is now
unambiguous.

### `execution.lease` modes were undefined — Major

**Scope:** MAO-001
**Finding:** `off`, `advisory`, and `required` were used without definition, and no
link pointed to the config reference.
**Resolution:** Defined all three and added the matrix.

### Outcome vocabulary was a second lifecycle — Major

**Scope:** MAO-006/007/010
**Finding:** "unknown outcome" and "queued/running/completed/failed" were used
without mapping to `internal/domain` statuses.
**Resolution:** Added an attempt-outcome definition (`succeeded`, `failed`,
`cancelled`, `unknown`) distinct from task status; MAO-010 renders domain status
plus attempt outcome.

### Capacity scenario contradicted its deferred requirement — Major

**Scope:** MAO-009, §13 scenario 12, §14 risk table
**Finding:** MAO-009 deferred per-endpoint capacity, but an acceptance scenario and
a risk mitigation still asserted it.
**Resolution:** Marked the scenario inactive while MAO-009 is P2; the risk row says
the bound is deferred.

### The bound had no maximum or precedence — Major

**Scope:** MAO-002
**Finding:** `max_parallel_tasks` was described as a new key with no documented
maximum, and its relationship to `OrchestrationBudget`/`MaxConcurrency` was
unresolved.
**Resolution:** The graph bound is the existing `EffectiveActiveWorkers`;
`max_parallel_tasks` is an alias for `MaxConcurrency`, clamped to `MaxWorkers`.
No separate cap is introduced.

### "Controller" was undefined — Major

**Scope:** §6/§7/§9
**Finding:** The controller was load-bearing but not identified as a component.
**Resolution:** Terminology defines it as the existing CLI driver, scheduler, and
domain; no new component is introduced.

### "Claim" was undefined — Major

**Scope:** §1, §6, MAO-008
**Finding:** "Claim-only integration" was the v1 policy but the term was undefined.
**Resolution:** Terminology defines a claim as `orchestration.WorkResult` /
`WorkerClaim`, untrusted until verification; claim-only means no patch is applied.

### Work unit was defined after heavy use — Nit

**Scope:** §4/§6/§9
**Finding:** "Work unit" appeared in goals and requirements before its definition.
**Resolution:** Defined in the new terminology section and at first use.

### The adoption gate's inputs were undefined — Nit

**Scope:** MAO-012
**Finding:** "Candidate" and "single-agent baseline" were not explained.
**Resolution:** Named `orchadopt.Corpus` and the baseline/candidate comparison.

### EGM was undefined — Nit

**Scope:** §16/§17
**Finding:** The acronym was used without expansion or a link.
**Resolution:** Expanded in terminology and linked to the assessment.

### Version schemes were conflated — Nit

**Scope:** MAO-005
**Finding:** Config schema version and SQLite schema version were both called
"version" with no distinction.
**Resolution:** Named both explicitly and stated which artifact owns which number.

### Project-specific identifiers lacked paths — Nit

**Scope:** §3/§8/§10
**Finding:** `BuildWorkerContext`, `OwnershipSingleWriter`, `BusyError`,
`resumeActiveTask`, and locality terms were named without a path.
**Resolution:** Added paths or a terminology entry.
