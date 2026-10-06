# CLOSE-002 — Plans/Status/History Reconciliation (Single Truth View)

> **Final status (2026-10-06).** This is a point-in-time record of its stage. The pre-performance closure is complete (`CLOSE-001…CLOSE-011` `LOCAL_DONE`). Where this record states a `NEEDS_HUMAN`, `BLOCKED`, `PARTIAL` or `ABSENT` finding, see [CLOSE-011-readiness.md](../../reports/pre-performance-closure/CLOSE-011-readiness.md) for the final verdict and the explicit deferrals.

> The wrap-up plan path cited in this record (`docs/plans/PLAN-Wrap-Up.md`) now resolves to the located historical plan `docs/history/PLAN-wrapup.md`; the closure plan references that path.

One truthful status view reconciling agentic-sop and sop-controller plans, phases,
status and history. This Markdown report is the sole authorized evidence mutation
created by CLOSE-002; no executable, configuration or other documentation change is
made merely to clean documentation.

- Reconciled at (UTC): 2026-10-04 (read-only inspection)
- Discipline: read-only for both repositories except this one report file.
- Classification vocabulary: ACTIVE / COMPLETE / HISTORICAL / BACKLOG / BLOCKED /
  NEEDS_HUMAN (plus explicit UNKNOWN where evidence is insufficient).
- Every row carries `recorded status` (what SOP or a document records) separately
  from `implementation evidence` (what code, tests or authoritative SOP output
  proves). Recorded status alone never yields a COMPLETE classification.
- Evidence basis: CLOSE-001 baseline `docs/reports/pre-performance-closure/CLOSE-001-baseline.md`
  is the pinned starting fact set for this reconciliation; CLOSE-002 does not repin
  it (CLOSE-005 owns repinning).

---

## 1. Imported controller active plan (recorded, NOT moved by this master)

| Field | Value |
| --- | --- |
| Plan document (controller checkout) | `docs/PLAN-SOP-Controller.md` |
| plan_id | `plan-sop-controller` |
| source_sha256 | `42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1` |
| Provenance record | `docs/plans/PLAN-Pre-Performance-Closure.md` (capability section "Controller current named plan") |
| Disposition | Imported as EVIDENCE only. This master plan does not replace, move, rename or mutate the controller's recorded active plan, and no fingerprint is changed. |

Recorded status of the controller plan is preserved verbatim as imported evidence.
Any change to the recorded active plan or its fingerprint requires proper SOP
reconciliation (a separate authorized stage), not this documentation reconciliation.

Evidence citation: `docs/plans/PLAN-Pre-Performance-Closure.md` records plan_id
`plan-sop-controller` and source_sha256
`42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1` and states it
is imported as evidence and not replaced.

---

## 2. Reconciliation input manifest (S1)

| # | Named input | Repository root | Exact path | Artifact kind | State | Provenance |
| --- | --- | --- | --- | --- | --- | --- |
| M1 | Controller recorded active plan + fingerprint | sop-controller (read-only) | `docs/PLAN-SOP-Controller.md` | Named plan | Recorded (not re-read this pass) | plan_id `plan-sop-controller`, source_sha256 `42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1` per `docs/plans/PLAN-Pre-Performance-Closure.md`; existence also asserted by `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 |
| M2 | `docs/plans/PLAN-Wrap-Up.md` | agentic-sop and sop-controller | NOT LOCATED in either root | Named plan (referenced only) | NOT LOCATED | No `docs/plans/PLAN-Wrap-Up.md` in the agentic-sop `docs/plans` listing; referenced only in `docs/plans/PLAN-Pre-Performance-Closure.md` (lines 131, 273, 364). Search scope: agentic-sop `docs/plans` listing + repository-wide text search for `PLAN-Wrap-Up`. |
| M3 | Hardening plan location A | sop-controller (read-only) | UNKNOWN (per-checkout path not re-verified this pass) | Plan / report | NEEDS_HUMAN (authoritative source not located verbatim) | The PRD names "both hardening-plan locations/reports" but no exact path was read this pass; classified explicitly as not-located rather than assumed complete. |
| M4 | Hardening plan location B | sop-controller (read-only) | UNKNOWN (per-checkout path not re-verified this pass) | Plan / report | NEEDS_HUMAN (authoritative source not located verbatim) | Same as M3. |
| M5 | Human-decision integration (Phase 6) | agentic-sop | `docs/guides/APPROVALS.md`; `docs/plans/PLAN-Phase-6-Interactive-Approval.md` (P6-001–P6-011) | Guide + named plan | Present | `docs/reference/PROJECT-STATUS.md` states Phase 6 is implemented and references both files. |
| M6 | Performance / observability work | agentic-sop | `docs/reference/PERFORMANCE.md`; `internal/perf`; `internal/cli/run.go`; `internal/cli/session.go` | Reference + code | Present | `docs/reference/PERFORMANCE.md` documents the single `internal/perf` representation; `docs/plans/PLAN-Pre-Performance-Closure.md` cites the same. |
| M7 | agentic-sop project status | agentic-sop | `docs/reference/PROJECT-STATUS.md` | Reference | Present | Read directly this pass. |
| M8 | agentic-sop backlog | agentic-sop | `docs/plans/BACKLOG.md` | Backlog | Present | Read directly this pass. |
| M9 | Recent synthesis/discovery correctness closure | agentic-sop | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure"; `docs/specs/AGENT-PROVIDER.md` | Reference + spec | Present | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" records PLAN/REVIEW synthesis corrections, bounded discovery, tool-level mutation verification (`d0af5a4`), and verified already-satisfied completion (`1ce9bdb`). |
| M10 | Provider routing | agentic-sop | `docs/reference/PROJECT-STATUS.md` §"Implemented work" (local-first model tiers); `docs/specs/MODEL-ROUTING.md` | Reference + spec | Present | `docs/reference/PROJECT-STATUS.md` records SMALL/MEDIUM/LARGE mappings and the local-first fallback limitation. |
| M11 | Known limitations | agentic-sop | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | Reference | Present | Read directly this pass. |
| M12 | Controller CTRL001–CTRL017 recorded status | sop-controller / SOP state | `sop resume` output (read-only); recorded in `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller CTRL001–CTRL017 status" | Authoritative SOP output | Recorded PLANNED | `docs/plans/PLAN-Pre-Performance-Closure.md` states `sop resume` reports nothing to resume and the recorded status is PLANNED; recorded status must be distinguished from actual implementation. |
| M13 | Historical C2-009 NOT READY | sop-controller / SOP state | `scripts/c2-009-dogfood.sh` result recorded in `docs/plans/PLAN-Pre-Performance-Closure.md`; controller untracked fixtures per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 | Historical dogfood result | HISTORICAL | `docs/plans/PLAN-Pre-Performance-Closure.md` states C2-009 records a historical real-binary dogfood as NOT READY. Preserved as-is; not relabeled. |

Nothing named in the PRD is silently dropped: items M2, M3 and M4 are explicitly
recorded as NOT LOCATED / NEEDS_HUMAN with the search scope stated.

---

## 3. agentic-sop-side classification (S2)

| Item | Classification | Recorded status | Implementation evidence | Evidence citation |
| --- | --- | --- | --- | --- |
| SOP V1 core stages (Repository/CI, Domain Model, Workflow State Machine, Persistence, CLI, PRD→Plan, Plan→Tasks, Dependency DAG, Scheduler, Git Adapter, Test Runner, Agent Harness, TDD Task Runner, Review, Open Code Review, Commit/Doc Gate, GitHub Adapter, CI Integration, CI Remediation, Merge Gate, Completion Loop, Resume/Recovery, Environment Bootstrap, Parallel Execution, Documentation Automation, E2E Dogfooding) | COMPLETE | Implemented | `docs/reference/PROJECT-STATUS.md` §"Implemented work" (V1 list); behavior defined by specs under `docs/specs/` | `docs/reference/PROJECT-STATUS.md` §"Implemented work" |
| Phase 3 (early JEV checkpoints) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Implemented work" (`Early JEV checkpoints (Phase 3)`) | `docs/reference/PROJECT-STATUS.md` |
| Phase 3.5 (deterministic model-class routing) | COMPLETE (as recorded) | Implemented; routing OFF by default | `docs/reference/PROJECT-STATUS.md` §"Implemented work" + §"Implemented work" note that routing/escalation are OFF by default | `docs/reference/PROJECT-STATUS.md`; `docs/specs/MODEL-ROUTING.md` |
| Phase 4 (provider runtime + capability discovery) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Implemented work" and `docs/reference/PERFORMANCE.md` (provider identity) | `docs/reference/PROJECT-STATUS.md` |
| Phase 5 (bounded model escalation) | COMPLETE (as recorded) | Implemented; escalation OFF by default | `docs/reference/PROJECT-STATUS.md` §"Implemented work" | `docs/reference/PROJECT-STATUS.md` |
| Phase 5.4 (unified work items + governed `sop prompt`) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Implemented work" (`Unified work items + governed sop prompt`) and agent skills line | `docs/reference/PROJECT-STATUS.md` |
| Phase 5.5 (installer + Claude plugin) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Implemented work" (`Unified installer ./install.sh ...`) | `docs/reference/PROJECT-STATUS.md` |
| Phase 5.6 (Windows distribution) | COMPLETE (as recorded) with an open residual | Implemented; clean-room test outstanding | `docs/reference/PROJECT-STATUS.md` §"Implemented work" + §"Known limitations" (Windows clean-room not run) | `docs/reference/PROJECT-STATUS.md` |
| Phase 6 (interactive approval / human-decision integration) | COMPLETE (as recorded) | Implemented (P6-001–P6-011) | `docs/reference/PROJECT-STATUS.md` §"Implemented work" final paragraph; `docs/guides/APPROVALS.md`; `docs/plans/PLAN-Phase-6-Interactive-Approval.md` | `docs/reference/PROJECT-STATUS.md`; `docs/guides/APPROVALS.md` |
| PLAN/REVIEW synthesis corrections | COMPLETE (as recorded) | Implemented; not a new closure task | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" bullet 1 | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| Bounded productive IMPLEMENT/FIX discovery | COMPLETE (as recorded) | Implemented; task-scoped budgets remain backlog | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" bullet 2; `docs/plans/BACKLOG.md` §"Task-Scoped Discovery Budgets" | `docs/reference/PROJECT-STATUS.md`; `docs/plans/BACKLOG.md` |
| Tool-level repository mutation verification (`d0af5a4`) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" bullet 3 | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| Verified already-satisfied IMPLEMENT/FIX completion (`1ce9bdb`) | COMPLETE (as recorded) | Implemented | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" bullet 4 | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| `docs/plans/PLAN-Pre-Performance-Closure.md` (CLOSE-001–CLOSE-011) | ACTIVE (planned, not executed) — recorded status PLANNED | Planned, not executed; not SOP's recorded active plan | `docs/plans/PLAN-Pre-Performance-Closure.md` header: "Status: PLANNED — execution-ready"; also `docs/plans/BACKLOG.md` §"Pre-Performance Closure and Baseline" and `docs/reference/PROJECT-STATUS.md` §"Pre-performance closure (complete)" | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/plans/BACKLOG.md`; `docs/reference/PROJECT-STATUS.md` |
| CLOSE-003 — SOP deterministic baseline | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-003 (defined; not run) | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-004 — Controller deterministic baseline | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-004 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-005 — Controller work verdicts / repin | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-005 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-006 — Named-plan dogfood / human decision | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-006 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-007 — Resume/idempotency | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-007 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-008 — Telemetry inventory | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-008; `docs/reference/PERFORMANCE.md` | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/reference/PERFORMANCE.md` |
| CLOSE-009 — Four-workload performance baseline | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-009 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| CLOSE-010 — Publish `docs/reports/PERFORMANCE-BASELINE.md` | BACKLOG (planned, not executed) | Planned (file not yet captured) | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-010; `docs/reference/PERFORMANCE.md` §"SOP Performance" notes the future output is not yet captured | `docs/reference/PERFORMANCE.md` |
| CLOSE-011 — Readiness verdict | BACKLOG (planned, not executed) | Planned | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-011 | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| `internal/perf` telemetry contract (stages_ms, validation_ms, total_ms, CategoryMS, WriteTask, WriteRun) | ACTIVE (existing, in use) | Implemented | `docs/reference/PERFORMANCE.md` §"The measurement model" + §"Where it is persisted"; referenced from `internal/cli/run.go` per that doc | `docs/reference/PERFORMANCE.md` |
| Phase 6 human-decision integration (Approvals guide) | COMPLETE (as recorded) | Implemented | `docs/guides/APPROVALS.md`; `docs/reference/PROJECT-STATUS.md` final paragraph of §"Implemented work" | `docs/guides/APPROVALS.md`; `docs/reference/PROJECT-STATUS.md` |
| Performance/observability deferred optimizations (PERF008, PERF010, PERF011, PERF012, PERF014) | HISTORICAL (evaluated, deferred with evidence) | Deferred; not scheduled | `docs/reference/PERFORMANCE.md` §"Deferred optimizations (with evidence)" | `docs/reference/PERFORMANCE.md` |
| Backlog: Invocation-Scoped IMPLEMENT Completion Evidence | BACKLOG | Backlog; separate from completed tool-level mutation verification | `docs/plans/BACKLOG.md` §"Invocation-Scoped IMPLEMENT Completion Evidence"; also `docs/reference/PROJECT-STATUS.md` §"Known limitations" | `docs/plans/BACKLOG.md`; `docs/reference/PROJECT-STATUS.md` |
| Backlog: Task-Scoped Discovery Budgets | BACKLOG | High priority; depends on bounded discovery | `docs/plans/BACKLOG.md` §"Task-Scoped Discovery Budgets" | `docs/plans/BACKLOG.md` |
| Backlog: Local network service (team mode) | BACKLOG | Not scheduled | `docs/plans/BACKLOG.md` §"Local network service (team mode)" | `docs/plans/BACKLOG.md` |
| Backlog: Small-device dashboard | BACKLOG | Not scheduled | `docs/plans/BACKLOG.md` §"Small-device dashboard" | `docs/plans/BACKLOG.md` |
| Backlog: Jev adapter and Jev-vs-deterministic evaluation | BACKLOG | Not scheduled | `docs/plans/BACKLOG.md` §"Jev adapter and Jev-vs-deterministic evaluation" | `docs/plans/BACKLOG.md` |
| Backlog: OpenAI-style tool calling for `openai_compatible` (Phase 3) | BACKLOG | Not scheduled; transport is text-only today | `docs/plans/BACKLOG.md` §"OpenAI-style tool calling for openai_compatible (Phase 3)" | `docs/plans/BACKLOG.md` |
| Known limitation: Windows clean-room test not run (P56-011) | BACKLOG (known limitation) | Not run | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: outer IMPLEMENT completion uses dirty-tree diff | BACKLOG (known limitation) | Tracked in backlog | `docs/reference/PROJECT-STATUS.md` §"Known limitations"; `docs/plans/BACKLOG.md` §"Invocation-Scoped IMPLEMENT Completion Evidence" | `docs/reference/PROJECT-STATUS.md`; `docs/plans/BACKLOG.md` |
| Known limitation: no project-scope skill install on Windows | BACKLOG (known limitation) | Not delivered | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: Windows CI runs a scoped set, not full suite | BACKLOG (known limitation) | As designed | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: prompt run ids unique locally, not globally | BACKLOG (known limitation) | As designed | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: `sop-ollama-agent` pinned revision predates `-version` | BACKLOG (known limitation) | Installer handles gracefully | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: ad-hoc runs have no resolvable approval gate | BACKLOG (known limitation) | As designed (Phase 6, P6-005) | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" | `docs/reference/PROJECT-STATUS.md` |
| Known limitation: local-first fallback applies only to router/selected class | BACKLOG (known limitation) | As designed; fallback owned by `internal/cli` seam | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)"; `docs/specs/MODEL-ROUTING.md` §"Local-First Fallback" | `docs/reference/PROJECT-STATUS.md`; `docs/specs/MODEL-ROUTING.md` |
| Known-by-design gaps: TDD test-design stage, `max_parallel_tasks`, CLI push→PR→CI→merge loop, `jev` decision provider | BACKLOG (documented in place) | Designed but not delivered | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" (final bullet) pointing at `docs/specs/TASK-LIFECYCLE.md`, `docs/specs/EXECUTION.md`, `docs/architecture/OVERVIEW.md`, `internal/decision/decision.go` | `docs/reference/PROJECT-STATUS.md` |
| Twelve CLI/JEV no-change expectation failures (recorded at `1ce9bdb`) | BLOCKED (deterministic gate not green) | Unresolved closure-gate failures recorded at `1ce9bdb`, reproduced on `d0af5a4` | `docs/reference/PROJECT-STATUS.md` §"Current checkout verification (2026-10-02)" lists the twelve failing tests; CLOSE-003 owns diagnosis; not waived | `docs/reference/PROJECT-STATUS.md` §"Current checkout verification" |
| Deferred future architecture (Prompt Compiler, Response Normalizer, Context Engine, Git-SHA summary cache, structural index, BM25/vector RAG, Decision Memory, Verification Cache, Prompt Result Cache, Adaptive Routing changes, Automatic Prompt Tuning, network/team mode, small-device dashboard, Jev adapter/evaluation, task-scoped discovery budget implementation, Windows enhancements) | HISTORICAL (explicitly deferred scope) | Deferred; do not start | `docs/plans/PLAN-Pre-Performance-Closure.md` §"Scope boundary" + capabilities §"New performance architecture ... MISSING"; echoed in `docs/plans/BACKLOG.md` §"Pre-Performance Closure and Baseline" | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/plans/BACKLOG.md` |

### 3.1 agentic-sop unresolved / not-located agentic-sop items

- None not located for the agentic-sop side that were named in the PRD except
  `docs/plans/PLAN-Wrap-Up.md` (see M2), which is referenced but has no located file in
  the agentic-sop `docs/plans` listing.

---

## 4. sop-controller-side classification (S3, read-only)

All sop-controller reads in CLOSE-002 are read-only; no file in the sibling checkout
was created, modified or deleted. Pre-existing untracked `c2-009-dogfood.*` fixture
directories recorded in `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2
remain present and unmodified.

| Item | Classification | Recorded status | Implementation evidence | Evidence citation |
| --- | --- | --- | --- | --- |
| Controller recorded active named plan `docs/PLAN-SOP-Controller.md` | ACTIVE (recorded) — imported as evidence, not moved | Recorded active plan, plan_id `plan-sop-controller`, source_sha256 `42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1` | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller current named plan" records the fingerprint and states it is imported as evidence and not replaced; `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 records the controller checkout at branch `main` HEAD `a51b0c6a6563033821ed4ae1e51890ab10479bbd` with no tracked dirty state | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 |
| CTRL001–CTRL017 recorded task status | BACKLOG (recorded as PLANNED) — recorded status, not implementation | `sop resume` reports nothing to resume; recorded status PLANNED | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller CTRL001–CTRL017 status" records the PLANNED status and the requirement to distinguish recorded status from implementation | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| Pending CTRL006 approval | NEEDS_HUMAN (reconciliation evidence only) | `sop approvals --json` records CTRL006 PENDING NEEDS_HUMAN, requested 2026-10-01T03:52:02Z, tied to an older no-change diagnostic | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Pending CTRL006 approval" — recorded as evidence, not proof of a current human action, and not permission to auto-approve | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| Historical C2-009 NOT READY dogfood result | HISTORICAL — preserved as-is, not relabeled PASS | NOT READY | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller named-plan dogfood in its own checkout" states C2-009 records a historical real-binary dogfood as NOT READY; the untracked `c2-009-dogfood.*` fixture directories (controller.log, transcript.log, project/) are recorded in `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 |
| Controller wrap-up (current controller work state) | NEEDS_HUMAN (source not located verbatim this pass; defer to CLOSE-005 verdicts) | PRD names "wrap-up"; no controller wrap-up file was read in this reconciliation pass | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-005 defines the controller work verdicts against current code/tests/provenance rather than stale wrap-up status; CLOSE-002 records the input as not-located instead of inventing a verdict | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-005 |
| Controller hardening plan (both locations/reports) | NEEDS_HUMAN (authoritative source not located verbatim) | PRD names "both hardening-plan locations/reports"; exact paths not read this pass | Same as manifest rows M3/M4: explicitly labeled as not-located with search scope stated rather than assumed complete; CLOSE-005 owns the hardening verdict | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-005 |
| Controller human-decision integration | NEEDS_HUMAN (controller side not read verbatim this pass) | Present per task specification; exact pass/fail UNKNOWN until CLOSE-004/CLOSE-005 run | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller boundary/human-approval/observability/performance code — PARTIAL": present per task specification, exact pass/fail UNKNOWN until CLOSE-004 runs | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller boundary/human-approval/observability/performance code — PARTIAL" |
| Controller observability/performance code | PARTIAL / NEEDS_HUMAN (deterministic pass/fail pending CLOSE-004) | Present per task specification; exact pass/fail UNKNOWN until CLOSE-004 runs | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller performance projections" (internal/sopclient/performance.go and task/project performance templates read the existing per-task/plan metrics contract; extension deferred) and "Controller boundary/human-approval/observability/performance code — PARTIAL" | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| Controller named-plan dogfood in its own checkout (fresh run) | BACKLOG (planned; CLOSE-006) | Not yet run in current checkout; PRD says the historical result is not fresh verification | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-006 defines the fresh end-to-end dogfood/human decision; the historical C2-009 NOT READY is kept historical | `docs/plans/PLAN-Pre-Performance-Closure.md` |
| Cross-root mutation (one repo's native harness mutating its sibling) | BLOCKED (not permitted) | Misses by design; not available | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Cross-root mutation ... MISSING": no single repository's native harness is able to mutate its sibling; not permitted. Use disposable fixtures in correct roots | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Cross-root mutation ... MISSING" |
| Controller recorded plan fingerprint — any change without SOP reconciliation | BLOCKED (not permitted by this plan) | Must not move the plan or change the fingerprint without proper SOP reconciliation | `docs/plans/PLAN-Pre-Performance-Closure.md` assumption "The controller's recorded docs/PLAN-SOP-Controller.md remains authoritative unless CLOSE-002 evidence shows otherwise" and capability "Controller current named plan" | `docs/plans/PLAN-Pre-Performance-Closure.md` |

---

## 5. Single status view — cross-repository summary

The table below is the de-duplicated single truth view. Every item carries exactly one
classification and one or more evidence citations. No active recorded plan is moved,
no fingerprint changed, no completed phase rescheduled or relabeled.

| Repository | Item | Classification | Evidence (short) |
| --- | --- | --- | --- |
| agentic-sop | SOP V1 core stages | COMPLETE | `docs/reference/PROJECT-STATUS.md` §"Implemented work" |
| agentic-sop | Phase 3 (JEV checkpoints) | COMPLETE | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 3.5 (model routing) | COMPLETE | `docs/reference/PROJECT-STATUS.md`; `docs/specs/MODEL-ROUTING.md` |
| agentic-sop | Phase 4 (provider runtime) | COMPLETE | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 5 (bounded escalation) | COMPLETE | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 5.4 (unified work items + `sop prompt`) | COMPLETE | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 5.5 (installer + Claude plugin) | COMPLETE | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 5.6 (Windows distribution) | COMPLETE (residual clean-room test BACKLOG) | `docs/reference/PROJECT-STATUS.md` |
| agentic-sop | Phase 6 (interactive approval / human decision) | COMPLETE | `docs/reference/PROJECT-STATUS.md`; `docs/guides/APPROVALS.md` |
| agentic-sop | PLAN/REVIEW synthesis corrections | COMPLETE | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| agentic-sop | Bounded productive IMPLEMENT/FIX discovery | COMPLETE | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| agentic-sop | Tool-level mutation verification (`d0af5a4`) | COMPLETE | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| agentic-sop | Verified already-satisfied IMPLEMENT/FIX completion (`1ce9bdb`) | COMPLETE | `docs/reference/PROJECT-STATUS.md` §"Recent correctness closure" |
| agentic-sop | `internal/perf` telemetry contract | ACTIVE | `docs/reference/PERFORMANCE.md` §"The measurement model" |
| agentic-sop | `docs/plans/PLAN-Pre-Performance-Closure.md` (CLOSE-001–CLOSE-011) | ACTIVE (recorded PLANNED) | `docs/plans/PLAN-Pre-Performance-Closure.md` header |
| agentic-sop | CLOSE-003 … CLOSE-011 | BACKLOG | `docs/plans/PLAN-Pre-Performance-Closure.md` stages |
| agentic-sop | Twelve CLI/JEV no-change expectation failures | BLOCKED | `docs/reference/PROJECT-STATUS.md` §"Current checkout verification" |
| agentic-sop | Backlog items (Invocation-scoped completion, Task-scoped discovery, team mode, dashboard, Jev adapter/evaluation, OpenAI-style tool calling) | BACKLOG | `docs/plans/BACKLOG.md` |
| agentic-sop | Known limitations (Windows clean-room, outer IMPLEMENT diff, Windows project skills, Windows CI scope, prompt run ids, `sop-ollama-agent` revision, ad-hoc approvals, local-first fallback scope, by-design gaps) | BACKLOG | `docs/reference/PROJECT-STATUS.md` §"Known limitations (current)" |
| agentic-sop | Deferred future performance architecture | HISTORICAL (deferred scope) | `docs/plans/PLAN-Pre-Performance-Closure.md` §"Scope boundary" |
| agentic-sop | Performance/observability deferred optimizations (PERF008, PERF010, PERF011, PERF012, PERF014) | HISTORICAL | `docs/reference/PERFORMANCE.md` §"Deferred optimizations" |
| sop-controller | Controller recorded active named plan + fingerprint | ACTIVE (recorded, imported as evidence; NOT moved) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller current named plan" |
| sop-controller | CTRL001–CTRL017 | BACKLOG (recorded PLANNED) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller CTRL001–CTRL017 status" |
| sop-controller | Pending CTRL006 approval | NEEDS_HUMAN (evidence only) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Pending CTRL006 approval" |
| sop-controller | Historical C2-009 NOT READY | HISTORICAL (as-is) | `docs/plans/PLAN-Pre-Performance-Closure.md`; `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2 |
| sop-controller | Wrap-up status | NEEDS_HUMAN (not located verbatim; CLOSE-005 verdict) | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-005 |
| sop-controller | Hardening plan A | NEEDS_HUMAN (not located verbatim) | Manifest M3 |
| sop-controller | Hardening plan B | NEEDS_HUMAN (not located verbatim) | Manifest M4 |
| sop-controller | Human-decision integration | NEEDS_HUMAN (pass/fail pending CLOSE-004/005) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "... PARTIAL" |
| sop-controller | Observability/performance code | PARTIAL / NEEDS_HUMAN (pending CLOSE-004) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller performance projections" |
| sop-controller | Named-plan dogfood (fresh) | BACKLOG (CLOSE-006) | `docs/plans/PLAN-Pre-Performance-Closure.md` stage CLOSE-006 |
| sop-controller | Cross-root mutation | BLOCKED (by design) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Cross-root mutation ... MISSING" |
| sop-controller | Fingerprint change without SOP reconciliation | BLOCKED (not permitted) | `docs/plans/PLAN-Pre-Performance-Closure.md` capability "Controller current named plan" |

---

## 6. Completed phases and historical records preserved as-is

- No completed phase is rescheduled. Every agentic-sop phase recorded as implemented in
  `docs/reference/PROJECT-STATUS.md` remains COMPLETE, with its recorded status and
  implementation evidence kept distinct.
- CLOSE-001's baseline values (`docs/reports/pre-performance-closure/CLOSE-001-baseline.md`)
  are cited unchanged; CLOSE-002 does not repin HEAD or binary hashes (CLOSE-005 owns
  repinning).
- The controller's recorded active plan and its fingerprint
  (`plan_id plan-sop-controller`, source_sha256
  `42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1`) are recorded as
  imported evidence and are not moved by this master plan.
- CTRL001–CTRL017 recorded status is preserved verbatim as PLANNED; it is not relabeled
  COMPLETE.
- The historical C2-009 NOT READY result is preserved as-is; it is not relabeled PASS.
- The pending CTRL006 approval is recorded as reconciliation evidence only; it is not
  treated as proof of a current human action and is not auto-approved.

---

## 7. Unresolved items (explicitly labeled, not silently dropped)

| Unresolved item | Label | Missing evidence named | Owner stage |
| --- | --- | --- | --- |
| `docs/plans/PLAN-Wrap-Up.md` | NOT LOCATED | No file in agentic-sop `docs/plans`; only referenced in `docs/plans/PLAN-Pre-Performance-Closure.md`. Search scope: agentic-sop `docs/plans` listing + repository-wide text search for `PLAN-Wrap-Up`. | CLOSE-002 manifest (this report) |
| Hardening plan location A | NEEDS_HUMAN | Exact per-checkout path not read verbatim this pass. | CLOSE-005 |
| Hardening plan location B | NEEDS_HUMAN | Exact per-checkout path not read verbatim this pass. | CLOSE-005 |
| Controller wrap-up status | NEEDS_HUMAN | Current code/test/provenance for wrap-up not read verbatim this pass. | CLOSE-005 |
| Controller human-decision integration | NEEDS_HUMAN | Exact pass/fail pending CLOSE-004 deterministic run. | CLOSE-004 |
| Controller observability/performance code | PARTIAL / NEEDS_HUMAN | Exact pass/fail pending CLOSE-004 deterministic run. | CLOSE-004 |
| Twelve CLI/JEV no-change expectation failures | BLOCKED | Diagnosis (stale test vs behavior defect) pending CLOSE-003. | CLOSE-003 |
| Fresh controller named-plan dogfood / controller HUMAN flow | BACKLOG | Not yet run; historical C2-009 NOT READY is not fresh verification. | CLOSE-006 |
| Cross-root mutation | BLOCKED (by design) | Not permitted; use disposable fixtures in correct roots. | Process boundary / CLOSE-009 |
| Windows clean-room test | BACKLOG | Real Windows machine test not run (P56-011). | Phase 5.6 follow-up |
| `docs/reports/PERFORMANCE-BASELINE.md` | BACKLOG | Not yet captured; CLOSE-010 publishes it. | CLOSE-010 |

No unresolved item is silently dropped: each carries a label and the missing evidence
is named.

---

## 8. Mutation and preservation statement

### Authorized mutation (sole intentional repository mutation of CLOSE-002)

- Added: `docs/history/pre-performance-closure/CLOSE-002-status-reconciliation.md`
  (this file).

The parent directory `docs/reports/pre-performance-closure/` already existed
(created by CLOSE-001). No other repository file is created or updated by CLOSE-002.
No executable, configuration or other documentation change is made merely to clean
documentation. The classification acceptance is satisfied by the recorded
classification with cited evidence; repository tests are not a substitute.

### Pre-existing user-owned changes preserved (agentic-sop)

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §1, these pre-existing
tracked modifications and untracked files were present before CLOSE-002 and are
preserved untouched:

- Tracked modifications: `docs/reference/CLI.md`, `internal/cli/cli.go`,
  `internal/cli/drive.go`, `internal/cli/jev.go`, `internal/cli/mutation.go`,
  `internal/cli/run.go`, `internal/git/git.go`, `internal/ollamaagent/prompt.go`,
  `internal/taskfile/taskfile.go`.
- Untracked files: `internal/cli/report_deliverable.go`,
  `internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go`.

### Read-only sop-controller

Per `docs/reports/pre-performance-closure/CLOSE-001-baseline.md` §2, the sop-controller
sibling checkout is strictly read-only during CLOSE-002. No file, configuration,
branch or commit in that repository was created, modified or deleted, including the
pre-existing untracked `c2-009-dogfood.4a0Q7q/`, `c2-009-dogfood.QnUK2o/`,
`c2-009-dogfood.th8DCS/` and `c2-009-dogfood.vt4eoy/` fixture directories
(`controller.log`, `transcript.log`, `project/`).

### Non-mutation statement

CLOSE-002 did not mutate application/source code, tests, runtime configuration, SOP
configuration, any Git branch, any existing user change, or the sibling sop-controller
repository content. No SOP state-changing command (approve, reconcile mutation, task
mutation) was executed to produce evidence; no executable change was made merely to
clean documentation.
