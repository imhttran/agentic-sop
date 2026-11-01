# Plan Historicalization

Status: **normative**. Owners: plan lifecycle. Command: `sop plan historicalize`
([CLI reference](../reference/CLI.md)). Skill: `/sop-historicalize`
([skills/sop-historicalize/SKILL.md](../../skills/sop-historicalize/SKILL.md)).

## 1. Purpose

A plan's work is executed against an ACTIVE task graph. When that work is done —
or when an operator explicitly disposes of it — the plan MUST leave the ACTIVE
set and become a historical record while its complete audit trail is preserved.

Historicalization is that transition. It is the general form of the plan
lifecycle's terminal step; the existing `sop plan complete` (disposition
`COMPLETE`) and `sop plan supersede` (disposition `SUPERSEDED`) remain their
operator verbs. Historicalization reuses the same archive storage
(`.agent-sdlc/archive/<plan-id>/`) and the same terminal vocabulary; it does not
introduce a second source of truth or a parallel disposition enum.

## 2. Model independence

Historicalization MUST NOT depend on a model, a provider, an agent harness, or a
specific implementation (Ollama, Codex, Nimble, Clef, Lala, …). The transition is
deterministic and testable without an LLM. An agent MAY analyze evidence and
recommend historicalization, but SOP's deterministic code validates and performs
the transition. Historicalization MUST NOT execute a task or run a model.

## 3. Invariants

- Historicalization MUST NOT silently rewrite a task outcome to make a plan
  appear complete. A preserved task keeps its exact status, blocked reason,
  attempts, and timestamps.
- A normal historicalization (disposition `COMPLETE`) MUST reject a plan with
  unresolved work: a task that is not satisfied (`PLANNED`, `READY`, any in-flight
  state, or `BLOCKED`), any pending human approval gate, or any satisfied task
  without verification evidence.
- Explicit terminal dispositions MUST be reused, not invented. The plan
  dispositions are `COMPLETE` and `SUPERSEDED`. There is no separate `FAILED`
  task status: a failed task is a terminal `BLOCKED` task carrying its classified
  `BlockedReason` (`RETRIES_EXHAUSTED`, `CI_FAILURE_UNACTIONABLE`, …), and it is
  refused exactly like any other unresolved work. `DEFERRED` and `ACCEPTED_RISK`
  are not domain concepts; adding them would be a domain-model change, not a
  historicalization change.
- The transition MUST preserve task outcomes, verification evidence, approval and
  decision history, and the plan provenance SOP already tracks (source, source
  hash, plan id, reconciliation decisions).

## 4. Readiness

Before any mutation, `EvaluateHistoricalization` (the reusable domain operation
behind `--check`) MUST produce, without mutating:

- the plan id and source;
- the current lifecycle state: `ACTIVE`, `HISTORICALIZED`, `NONE`, or
  `INCONSISTENT` (an archive record exists while the plan is still active — a
  staged transition);
- task counts by state;
- unresolved tasks, as `ID (STATUS)` (a blocked task also shows its reason);
- unresolved approvals (tasks parked at a pending gate);
- unresolved verification (satisfied tasks with no PASSED run and no
  external-completion record; a task declared `ExecutionDone` needs none);
- eligibility and, when ineligible, the reason.

Eligibility is deterministic. `COMPLETE` requires no unresolved task, no pending
approval, and no missing verification. `SUPERSEDED` is the explicit disposal path
and permits unfinished work, preserving it truthfully. An `INCONSISTENT` state is
eligible for recovery (see §7). A named plan that is not the active plan is
refused, so the wrong identity is never archived.

## 5. Operation

When eligible, historicalization MUST:

1. re-validate the current plan state (and refuse a plan that changed since
   readiness — see §8);
2. validate task terminality/dispositions and the required approvals and
   verification evidence;
3. capture the final plan status;
4. preserve task outcomes, verification evidence, approval/decision history, and
   the tracked source revisions/provenance;
5. write the historical record under `.agent-sdlc/archive/<plan-id>/`
   (`plan.json`, `plan.meta.json`, `tasks.json`, `lifecycle.json`);
6. remove the plan from ACTIVE selection (clear the task graph, release the
   active-plan association, remove the machine plan);
7. verify the resulting lifecycle state before reporting success.

## 6. Idempotency

Running historicalization against an already-historicalized plan MUST result in a
deterministic no-op reported as `ALREADY_HISTORICALIZED`. It MUST NOT duplicate
history records, evidence, summaries, or archive artifacts, and MUST NOT rewrite
the recorded disposition or timestamp.

## 7. Atomicity

The mutation is one logical state transition. The active association (the task
graph plus `.agent-sdlc/plan.json`) and the archive are separate stores, so the
transition is **staged**: archive first, then release. Each archive file is
written atomically; the task graph is cleared in a transaction. If a step fails,
SOP MUST NOT claim the plan is historicalized while required lifecycle records
remain active: the failure reports an incomplete release, and a re-run detects the
existing archive record plus the still-active plan (`INCONSISTENT`) and completes
the release without rewriting the archive.

## 8. Concurrency and stale state

Multiple SOP/controller/agent processes MAY operate on the same plan. The
readiness decision MUST NOT overwrite newer state:

- an optional expected fingerprint (from a prior readiness evaluation) is
  compared and a mismatch refuses the mutation as stale;
- regardless of the caller, the plan is re-read immediately before mutation and
  the fingerprint re-compared, so a plan changed within the readiness→mutation
  window is refused;
- blockers that a concurrent process could change without moving the task
  fingerprint (a newly raised approval gate, removed run evidence) are re-checked
  before mutation.

## 9. Skill

`/sop-historicalize` is a thin client
([skills/sop-historicalize/SKILL.md](../../skills/sop-historicalize/SKILL.md)).
It discovers the relevant active plan, runs readiness, reports blockers clearly
and stops without mutation when ineligible, invokes `sop plan historicalize` when
eligible, verifies the resulting state, reports exactly what changed, and reports
the next active/runnable plan if one exists. It MUST NOT manipulate plan files or
state to bypass the deterministic operation, and MUST NOT decide completion or
approval outcomes.

## 10. End-to-end integration

`sop run` (the `/sop-end-to-end` path) MAY, at the end of a successful run whose
active plan is fully satisfied, use historicalization as the final lifecycle
stage. It does so only as the opt-in `SOP_HISTORICALIZE_ON_COMPLETION=true`, which
is OFF by default so existing execution behavior is unchanged. The stage runs only
when readiness is eligible, so because a plan may be parked at a human approval
boundary it never historicalizes across a pending gate: historicalization
preserves the existing approval semantics and never approves, declines, or
discards a pending gate. A historicalization problem is reported and never changes
the run's outcome.
