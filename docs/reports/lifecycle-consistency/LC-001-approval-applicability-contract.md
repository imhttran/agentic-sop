# LC-001 — Approval Applicability Contract

**Status:** specification (read-only stage). **Enforcement:** none by this stage; invariants I1–I5 are assigned forward.

## 1. Purpose

SOP persists one authoritative, explicit human-approval request per task at a human gate (`domain.ApprovalRequest`, `internal/domain/approval.go`). Multiple consumers — the application boundary, plan historicalization readiness, the run driver, report rendering, and CLI listings — each currently decide for themselves whether such a request is "pending", "unresolved", or "applicable". These decisions have diverged three ways. LC-001 fixes exactly one authoritative definition and specifies how every consumer derives its classification from it.

This stage is a **read-only specification**. No production file is created, modified, or deleted; the only artifact is this report.

## 2. The authoritative predicate

Let `A` be the task's persisted approval request head (`internal/run/approval.go` `Run.Approval()`, backed by `.agent-sdlc/runs/<task-id>/approval.json`), and `T` the task `A.TaskID` in the task store.

**Authoritative predicate (P):**

```
resolved(A)     := A.Status == APPROVED || A.Status == DECLINED   (domain.ApprovalStatus.Resolved)
active(A)       := A.Status == PENDING
stale(A, T)     := active(A) && T != nil && T.IsSatisfied()
applicable(A,T) := active(A) && !stale(A, T)     == ( active(A) && (T == nil || !T.IsSatisfied()) )
unresolved(A,T) := active(A)                       (regardless of staleness)
```

### 2.1 Classification derived from P

| Classification | Definition                                      | Meaning                                                                                                                                    |
| -------------- | ----------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `resolved`     | `resolved(A)`                                   | A decision has been recorded (`APPROVED`/`DECLINED`). It authorizes nothing further and is archive provenance.                             |
| `stale`        | `active(A) && T.IsSatisfied()`                  | The request is still `PENDING` but its task has since reached a satisfied terminal state, so the request can no longer authorize anything. |
| `applicable`   | `active(A) && (T == nil \|\| !T.IsSatisfied())` | A decision may actually be recorded through the boundary right now.                                                                        |
| `unresolved`   | `active(A)`                                     | No decision has been recorded. Broader than `applicable`: it includes stale requests.                                                      |

`applicable ⊆ unresolved`; `stale ⊆ unresolved`; `resolved`, `stale`, and `applicable` are mutually exclusive. These are the four labels every consumer must speak in.

## 3. Stale / active / cross-plan / external rules

- **Active rule.** A request is active iff its status is `PENDING`. Only the boundary's recorded decision (`APPROVED`/`DECLINED`) makes it resolved; no consumer may infer resolution from task status, prose, or report content.
- **Stale rule.** A `PENDING` request whose task `T` is satisfied (`T.IsSatisfied()`) is `stale`. It must never be reported as `applicable`, and the boundary refuses a decision on it (`ErrApprovalNotApplicable`) and archives it into `approval-history.json`.
- **Cross-plan rule.** Approval applicability is keyed to the task (`A.TaskID`) and the task's current persisted state — never to the plan identity. Plan historicalization MUST treat a pending approval as a plan-level blocker (unresolved approval), regardless of which invocation created the request, because the request is bound to a task that is still in the plan.
- **External-completion rule.** When a task's work is satisfied by a persisted external-completion record (`external-completion.json`) rather than a run, the request remains `PENDING` in the artifact and therefore becomes `stale` once the task is satisfied. No consumer may silently rewrite or delete it; only an explicit boundary decision (or a future enforcement stage) may archive it.

## 4. Per-consumer derivation table (current divergence)

The task states a three-way divergence exists. Pinning every consumer yields exactly three distinct predicate shapes.

| #   | Consumer                                                         | Location                                                                  | Current predicate                                                        | Divergence class                    |
| --- | ---------------------------------------------------------------- | ------------------------------------------------------------------------- | ------------------------------------------------------------------------ | ----------------------------------- |
| 1   | Approval application boundary — `Applicable` projection          | `internal/approval/approval.go:355`                                       | `req.Status == PENDING && (task == nil \|\| !task.IsSatisfied())`        | **applicable** (the correct shape)  |
| 2   | Approval application boundary — decision refusal                 | `internal/approval/approval.go:281`                                       | `task != nil && task.IsSatisfied()` → refuse stale                       | aligns with (1)                     |
| 3   | Approval application boundary — request supersede                | `internal/approval/approval.go:168`                                       | `head.Status == PENDING && head.Kind == kind`                            | **active** (no staleness check)     |
| 4   | Plan historicalization readiness — unresolved approvals          | `internal/planflow/historicalize.go:483`                                  | `req.Status != PENDING → skip` (counts pending regardless of task state) | **unresolved** (no staleness check) |
| 5   | Run driver — parked human-gate print                             | `internal/cli/drive.go:689`                                               | `v.Status == PENDING` (no staleness check)                               | **unresolved**                      |
| 6   | Run driver — `currentApprovalBoundary` (APPROVAL_REQUIRED input) | `internal/cli/approval.go:387`                                            | `req.Status == PENDING`                                                  | **unresolved**                      |
| 7   | CLI listing `sop approvals`                                      | `internal/cli/approval.go` `runApprovals` (uses `view.Applicable`)        | delegates to consumer (1)                                                | **applicable**                      |
| 8   | CLI interactive decision gate list                               | `internal/cli/approval.go` `listApplicableGates` (uses `view.Applicable`) | delegates to consumer (1)                                                | **applicable**                      |
| 9   | `sop approval <task-id>` status view                             | `internal/cli/approval.go` `runApprovalStatus` (uses `view.Present`)      | presence only, no applicability claim                                    | neutral                             |
| 10  | `sop report` approval section                                    | `internal/cli/report.go:106`                                              | presence only (`ok`), renders `req.Status` verbatim                      | neutral                             |
| 11  | Run persistence — approval head/history                          | `internal/run/approval.go:48` `Run.Approval()`                            | reads `approval.json`; declares present iff `ID != ""`                   | persistence owner (defines `A`)     |
| 12  | MCP                                                              | `internal/mcp`                                                            | no approval-state consumer found (`search_files "RequiresHuman           | Approval"` → no matches)            | UNKNOWN — no consumer exists; recorded so a future MCP surface inherits P rather than reinventing it |

### 4.1 The three-way divergence

The same `PENDING` request classifies differently depending on consumer:

1. **applicable** (consumers 1–2, 7–8): requires active **and not stale**.
2. **unresolved** (consumers 4–6): requires active only; a stale `PENDING` request still blocks historicalization and still prints as a live human gate.
3. **active-only refresh** (consumer 3): treats a stale `PENDING` request as still refreshable by a re-request of the same kind.

This is the recorded divergence: a request that the boundary would refuse to decide (stale) is simultaneously (a) invisible to `sop approvals`, (b) counted as an unresolved approval blocker by plan historicalization, and (c) reported as a parked human gate by the run driver. No consumer is wrong about a fact; they disagree about which label `PENDING` implies.

### 4.2 RM-003 evidence

RM-003 is the single `PENDING` approval artifact among the 8 recorded `approval.json` artifacts (the other 7 are resolved). Its task reached a **satisfied state via external completion** (`external-completion.json`), and the plan was subsequently **archived** (`.agent-sdlc/archive/<plan-id>/lifecycle.json` present). Under P the request is therefore `stale` (active, task satisfied). It exposed the divergence because:

- consumer 1 would report `Applicable == false` (stale → not applicable);
- consumer 4 would still list RM-003 under `UnresolvedApprovals` if it were on an active plan;
- consumer 5/6 would print RM-003 as a parked human gate while reading the stale artifact.

## 5. Invariants I1–I5 (canonical — the LC plan mapping)

The five invariants below are the plan's, stated here as the **single canonical
mapping** for this plan. Every other section, and every downstream stage
(LC-002…LC-005), references these labels. LC-001 enforces none of them; each is
assigned forward to the stage that implements it. This mapping supersedes any
earlier labeling (see §8.3).

- **I1 — External completion resolves or supersedes pending approvals, preserving history.** A task closed through an external-completion record must not leave a stale `PENDING` head: its request is superseded into append-only `approval-history.json`, never rewritten or deleted in place. Enforcing stage: **LC-002**; substrate `internal/approval/approval.go` (`Request` supersede) and `internal/run/approval.go` (`AppendApprovalHistory`).
- **I2 — Approval readiness is scoped to the active plan.** Only approvals bound to a task in the active plan may block that plan; a `PENDING` request on an unrelated, satisfied task must not make an unrelated plan ineligible. Enforcing stage: **LC-003** (`internal/planflow/historicalize.go`).
- **I3 — Consumers share one approval-applicability definition.** Every consumer derives `applicable = active ∧ ¬stale` from SOP's own record (`internal/approval`); no consumer infers an approval from `BLOCKED` / `NEEDS_HUMAN` / `WAITING_FOR_HUMAN` status or prose, and none re-derives applicability from a raw artifact read. Enforcing stages: **LC-002/LC-003** (adopt P across consumers).
- **I4 — Plan completion and historicalization share closure safety gates.** `sop plan complete` and `sop plan historicalize` enforce the same closure safety invariants — no genuinely applicable pending approval and no missing required verification — while retaining their distinct operations and disposition semantics. Enforcing stage: **LC-004** (`internal/planflow/planflow.go` `Complete`, `internal/planflow/historicalize.go`).
- **I5 — Operator-facing approval status and readiness must not contradict each other.** `sop approvals` and any readiness gate never disagree for the same task. Enforcing stage: **LC-005** (regression suite asserting agreement).

The substantive facts of the earlier draft are preserved under these canonical
labels: at-most-one `PENDING` head with append-only history and "never rewrite or
delete" → **I1**; `applicable = active ∧ ¬stale` and "derive only from SOP's own
record" → **I3**; "a `PENDING` approval blocks historicalization" → **I4**.

## 6. RM-003 disposition (factual)

- A **`PENDING` approval persists after external completion and plan archival.** The `approval.json` artifact for RM-003 is still on disk and still `PENDING` after (a) its task was satisfied through an external-completion record and (b) the plan was archived.
- It is **neither rewritten nor deleted by this plan.** LC-001 is a read-only specification stage; it performs no migration, no archival, and no mutation of the stale `PENDING` request. Remediation — archiving the stale request under a rule that enforces I2/I5 — is deferred to the enforcement stage that adopts P; it is not performed here.
- Under the authoritative predicate the RM-003 request is classified **`stale`** (active, task satisfied), not `resolved` and not `applicable`.

## 7. Read-only confirmation

This stage creates exactly one file — this report — under `docs/reports/lifecycle-consistency/`. It creates, modifies, or deletes no production file. Verification: `git status`/`git diff` show only the added report; `go build ./...`, `go test ./...`, `go vet ./...`, and `gofmt -l .` are unaffected because no production source changed.

## 8. Shared closure gates and invariant reconciliation

### 8.1 Architectural decision

`sop plan complete` and `sop plan historicalize` **must enforce the same closure
safety invariants**, while keeping distinct lifecycle operations:

- **Same safety gates.** Both fail closed when the active plan has a genuinely
  _applicable_ pending approval (an open human gate whose task can still act on it)
  or missing required verification. Neither may archive a plan that readiness would
  refuse.
- **Distinct operations.** `complete` marks the active plan COMPLETE and archives it;
  `historicalize` performs the disposition-aware close with an explicit
  `COMPLETE | SUPERSEDED` selector and a readiness report. The operations differ;
  their gates must not.
- **Historical stale approvals remain auditable without blocking unrelated plans.**
  A stale `PENDING` request left by an externally completed task is preserved in
  append-only history (I1) and scoped out of unrelated plans (I2); it is never
  silently rewritten or deleted.
- **The defect this decides against.** As implemented, `complete`
  (`internal/planflow/planflow.go` `Complete`) gates only on `AllSatisfied` and
  ignores approvals and verification, while `historicalize` gates on full readiness.
  That asymmetry is why RM-003 was archived COMPLETE with a live `PENDING` approval.
  Aligning them is **LC-004**.

### 8.2 Canonical invariant mapping

| Invariant | Statement                                                                        | Enforcing stage |
| --------- | -------------------------------------------------------------------------------- | --------------- |
| I1        | External completion resolves or supersedes pending approvals, preserving history | LC-002          |
| I2        | Approval readiness is scoped to the active plan                                  | LC-003          |
| I3        | Consumers share one approval-applicability definition                            | LC-002 / LC-003 |
| I4        | Plan completion and historicalization share closure safety gates                 | LC-004          |
| I5        | Operator-facing approval status and readiness never contradict                   | LC-005          |

### 8.3 Reconciliation of the earlier §5 labels

An earlier draft of this contract labeled a _different_ set of five statements
`I1–I5`. That labeling is superseded. §5 now states the canonical mapping above, and
this section is the single authoritative reconciliation — **no second `I1–I5`
mapping remains**. The earlier statements are preserved as enforcement detail under
the canonical labels: at-most-one `PENDING` head with append-only history → **I1**;
`applicable = active ∧ ¬stale` and "derive only from SOP's own record" → **I3**;
"a `PENDING` approval blocks historicalization" → **I4**.

### 8.4 Satisfaction is not closure readiness

External completion (`external-completion.json`) establishes that a task's **work**
is satisfied; it is **not** evidence that every **required verification** has passed.
Task satisfaction and closure readiness are distinct:

- **Satisfaction:** the task reached a satisfied terminal state (`IsSatisfied`),
  possibly by an external-completion record.
- **Closure readiness:** the plan additionally has no genuinely applicable
  unresolved approval and no missing required verification.

RM-003 is the worked example: its task is satisfied (externally completed), yet under
the aligned gates its plan would not have been COMPLETE-ready, because closure
readiness is broader than task satisfaction. This plan preserves that distinction; it
never treats completion (internal or external) as proof of verification.
