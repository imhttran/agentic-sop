# Goldfish Review Report — Stage 7 (Critic Review)

**Target:** `docs/requirements/PRD-MULTI-AGENT-ORCHESTRATION.md` (feature range)
**Mode:** REVIEW, Stage 7
**Reviewers:** `egm-design`, `egm-structural`, `egm-style`
**Date:** 2026-11-23

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-design` | completed |
| `egm-structural` | completed |
| `egm-style` | completed (Stage 9 pass; see `06-implementation_readiness_report.md`) |

Severity breakdown after deduplication: 3 Blocker, 10 Major, 5 Nit.

## Prioritized Findings

### `NextBatch` promotion corrupts resume — Blocker

**Lens:** `egm-design`
**Scope:** MAO-002/006, §13
**Finding:** `scheduler.Next` promotes `PLANNED → READY` and saves the staged copy;
`IsActive(READY)` treats it as in flight; `resumeActiveTask` refuses more than one
active task and `resume.ActionFor(READY)` routes to `CreateBranch`. A batch that
promoted N analysis units would leave them unrecoverable and misroute them into the
mutating lifecycle.
**Resolution:** `NextBatch` is read-only: it returns eligible tasks without
promoting or saving. `Next` keeps its promotion path. A cancelled analysis unit
therefore remains `PLANNED`/`READY`.

### The bound formula collapsed to 1 — Blocker

**Lens:** `egm-design`
**Scope:** MAO-002
**Finding:** `ExecutionPolicy.concurrency()` maps zero to 1, and
`EffectiveActiveWorkers` takes the tighter of budget and policy. Taking a minimum
with `max_concurrency` made the default 1 regardless of the new key.
**Resolution:** Removed the fourth limit. The graph bound is the existing
`EffectiveActiveWorkers`; `max_parallel_tasks` aliases `MaxConcurrency`; the
existing budget clamps to `MaxWorkers`.

### The migration erased the attribution it added — Blocker

**Lens:** `egm-design`, `egm-codebase-details`
**Scope:** MAO-003, architecture §5
**Finding:** `Store.Save` deletes and reinserts attempts from `domain.Attempt`;
`loadAttempts` selects only six columns. A later save would blank every new
attribution column.
**Resolution:** The plan now extends `domain.Attempt`, `insertAttempts`, and
`loadAttempts`, and routes the orchestration path through `AppendAttempt`.

### Requirement → stage traceability was missing — Blocker

**Lens:** `egm-structural`
**Scope:** §7/§12, plan
**Finding:** Three ID schemes had no mapping table; MAO-009 and MAO-011 appeared in
no phase; "Phase 10" collided with the BACKLOG's Dynamic Worker Action Space.
**Resolution:** Added a requirement traceability table; renamed stages to
`MAOP-001..005`; renamed the plan to `PLAN-MULTI-AGENT-ORCHESTRATION.md`; deferred
MAO-009/011 with activation triggers.

### Decision counts disagreed across documents — Major

**Lens:** `egm-structural`
**Scope:** §15, §12, plan, architecture
**Finding:** D1–D8 vs D1–D9 vs D1–D10; D3 and D4 marked resolved in one place and
open in another.
**Resolution:** D1–D10 everywhere; D3 and D4 recorded as resolved in this revision;
gates updated (MAO-B gates on D1, MAO-C on D10, MAO-D on D2/D7).

### P1 definition contradicted phase order — Major

**Lens:** `egm-structural`
**Scope:** §7 priority key, §12
**Finding:** P1 was "required before parallel mutating work", but integration and
visibility shipped after mutating execution.
**Resolution:** P1 is "required before relying on concurrent mutating results";
MAO-C (visibility) now ships before MAO-D (mutating execution); integration is
deferred to MAO-008.

### `MAO-005` contradicted "already satisfied" — Major

**Lens:** `egm-structural`
**Scope:** §3, MAO-005
**Finding:** The default-off switch was listed as "already satisfied" while
MAO-005 was a P0 requirement for it.
**Resolution:** Clarified that MAO-005 is a preservation requirement; the switch
already exists and must not regress.

### Review artifacts were claimed before they existed — Major

**Lens:** `egm-structural`, `egm-style`
**Scope:** §16, §17
**Finding:** The PRD listed eight reports that did not exist and checked a completed
review.
**Resolution:** Reports are written by this review; the definition-of-done is
checked only after they exist. The status line was calibrated.

### "Read-only work shares a snapshot" was unbacked — Major

**Lens:** `egm-design`
**Scope:** MAO-004, architecture §6
**Finding:** The sandbox root is writable and `Scope{}` is a post-dispatch filter;
a shared "read-only" snapshot is not an execution-time boundary. `EXECUTION.md` §8
says each concurrent task gets its own worktree.
**Resolution:** Non-mutating units use `toolharness.RootReadOnly`, which refuses
write tools and repository-mutating shell commands; mutating units get a worktree.
A read-only unit that runs build/test commands also gets a worktree.

### Acceptance criteria had no reachable test — Major

**Lens:** `egm-design`, `egm-zoom-out`
**Scope:** §13, plan tests
**Finding:** Stale-lease, unknown outcome, cancellation, budget exhaustion,
truncation, retry safety, and redaction had criteria but no named test.
**Resolution:** Added a cross-cutting verification table mapping each criterion to
a planned test and owning test file.

### Mid-batch budget exhaustion was undefined — Major

**Lens:** `egm-design`
**Scope:** MAO-007, §14
**Finding:** The budget check is fail-closed; with a shared usage collector, one
worker crossing the limit can affect siblings, but the PRD did not say what happens
to started units.
**Resolution:** MAO-007 now stops new selection, records every started outcome, and
leaves state resumable; a risk row and test were added.

### `sop orchestrate` had no concurrency input — Major

**Lens:** `egm-codebase-details`
**Scope:** MAOP-001
**Finding:** `sop orchestrate` accepts exactly one argument and refuses flags, so a
"concurrency requested" refusal could not be expressed there.
**Resolution:** Moved the concurrency-refusal criterion and test to
`sop run --parallel-units`; `sop orchestrate` stays `N=1`.

### `sop attempts` does not exist — Major

**Lens:** `egm-codebase-details`
**Scope:** §3 operator visibility
**Finding:** The PRD cited a non-existent subcommand.
**Resolution:** Removed the reference; the baseline now cites `sop status` and
`sop report` only.

### `MaxWorkers` was not enforced as a ceiling — Major

**Lens:** `egm-codebase-details`
**Scope:** MAO-002
**Finding:** `EffectiveActiveWorkers` is `min(MaxActiveWorkers, MaxConcurrency)`;
nothing clamps to `MaxWorkers`. The PRD claimed it could not exceed `MaxWorkers`.
**Resolution:** MAOP-002 adds the clamp to `EffectiveActiveWorkers` and a test.

### `WorkerFactory` changed a constructor with no adapter — Major

**Lens:** `egm-codebase-details`
**Scope:** MAOP-004
**Finding:** `NewCoordinator(a)` has a production caller; replacing its field would
break the build without a compatibility adapter.
**Resolution:** MAOP-004 keeps `NewCoordinator(a)` as a fixed-agent adapter and adds
the factory path; `orchestrate.go` is in the file list.

### "Permanently a safety gate" was unenforceable — Nit

**Lens:** `egm-design`
**Scope:** MAO-012
**Finding:** The phrase described intent, not an implementable rule.
**Resolution:** Changed to "for this PRD"; the enforceable rule is that the gate
does not claim benefit.

### Duplicate phase naming — Nit

**Lens:** `egm-structural`
**Scope:** Plan title, §12
**Finding:** "Phase 10" collided with the BACKLOG entry.
**Resolution:** Renamed to phases MAO-A … MAO-E and `MAOP-*` stages.

### Status vocabulary gained a fifth value — Nit

**Lens:** `egm-structural`
**Scope:** §3
**Finding:** "Unnecessary / deferred" was not one of the four declared statuses.
**Resolution:** Uses "Unnecessary"; deferral is recorded in the consequence column.

### "Byte-identical" was unachievable — Nit

**Lens:** `egm-structural`
**Scope:** §12, MAO-003/005
**Finding:** Run artifacts carry timestamps and IDs.
**Resolution:** Replaced with testable wording: no new column populated, no new
artifact, same task fields/status/attempt count.
