# Operator approval packet — ACP execution-lease wiring

Status: **approval packet, read-only. Nothing here is approved, applied, or
authorized.** This document requests the decisions the blocked SOP run needs before
implementation may resume. It follows the repository's decision-record convention
(`docs/plans/PROPOSAL-*-OPERATOR-DECISION-RECORD.md`).

Revision note: the first version of this packet was written while stage 1 (an
inheritance helper) was still in the working tree and blocked on a security-baseline
fingerprint. Stage 1 was reviewed and **dropped** before any commit (design record
D1), which removes that blocker. This version replaces the recovery sequence and the
baseline and reconcile decisions accordingly.

Plan (untracked working-tree document, so it is named rather than linked):
`docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING.md`
(`Status: Draft, unreviewed — authorizes no implementation`).
Design record: [`PLAN-ACP-EXECUTION-LEASE-WIRING-DESIGN-DECISIONS.md`](PLAN-ACP-EXECUTION-LEASE-WIRING-DESIGN-DECISIONS.md).
Acceptance mapping: [`ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md`](ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md).
Gap record: [`ACP-EXECUTION-EXCLUSION-GAP.md`](ACP-EXECUTION-EXCLUSION-GAP.md).
Follow-up tests: #575.

## Current state (2026-11-21, `main` = `c673d442`)

| Item               | State                                                                                                                                                      |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Plan authorization | absent — the plan still says it authorizes no implementation                                                                                               |
| Approval artifacts | none (`.agent-sdlc/approval.json` absent)                                                                                                                  |
| SOP plan           | `plan-acp-execution-lease-wiring`, `ACTIVE`, compiled from the plan **before** stage 1 was dropped                                                         |
| Tasks (persisted)  | `stage-1`, `stage-10` `BLOCKED` (RETRIES_EXHAUSTED, attempt 0/3); 14 `PLANNED`; 0 completed. `stage-1` now describes a dropped step                        |
| `execution.lease`  | absent from `.agent-sdlc/config.yaml` ⇒ `off`                                                                                                              |
| Stage-1 code       | none: the working tree has no inheritance code. `go build`, `go vet` and the `internal/cli` and `internal/executionlease` tests pass                       |
| Security baseline  | unchanged; the `internal/cli/mcp_start_work.go` entry holds `semgrep:1c1fe971…86180` (line 721), and `exec.Command(exe, args...)` is at line 721 again     |
| Security scan      | re-run 2026-11-21 after stage 1 was dropped (`sop security scan --required --scope workspace`, run id `scan-stage1-dropped`): verdict PASS, coverage COMPLETE; the `exec.Command` finding at line 721 is suppressed by the existing baseline entry (`semgrep:1c1fe971…`) |
| Plan digest        | the plan and mapping were edited after compilation, so `plan.meta.json` `source_sha256` (`6e9922ed…`) is expected to mismatch; not yet checked by SOP     |

## Decision 1 — authorize the plan

**Requested:** approve `PLAN-ACP-EXECUTION-LEASE-WIRING.md` as the implementation
authority for stages 2–16 (stage 1 is dropped), with the design decisions in the design
record (D1 child-as-acquirer, D2 resolved by D1).

What approval would mean:

- The plan's `Status:` line is replaced by an approval line naming the approver and
  date. Prose status is documentation only; SOP does not parse it (see below).
- The design record's recommendations become the accepted scope.
- Approval does **not** authorize a security-baseline change (Decision 2), a plan
  reconcile (Decision 3), or a merge (Decision 4).

Why operator input is required: SOP has no plan-level approval gate. The only
approval mechanisms are task-level `NEEDS_HUMAN` (`internal/domain/approval.go:45-55`)
and `human.approval_before_commit` (`internal/cli/commit.go:31`,
`internal/cli/pr.go:36`). `PreflightPlan` compares plan identity and digests only
(`internal/planflow/planflow.go:320-331`); nothing reads a plan's prose status.
An existing run is therefore not evidence of authorization.

## Decision 2 — security baseline (deferred; no change needed today)

**Status.** The blocker is resolved by dropping stage 1, not by a baseline edit.
Stage 1 added an import and a five-line constant above `launchWorkProcess` in
`mcp_start_work.go`, which moved the existing `exec.Command(exe, args...)` from line
721 to 727. The baseline suppression identity includes line and column
(`internal/security/baseline.go:147-148`), so the entry no longer matched. With the
change removed the call is at line 721 again. The required scan was re-run and
confirms it: verdict PASS, no unsuppressed blocking findings (18 unsuppressed MEDIUM
findings elsewhere are non-blocking).

| Fact               | Value                                                                                                                                                                                                 |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Rule               | `dangerous-exec-command` (`internal/security/.../audit/dangerous-exec-command.yaml`, severity ERROR, CWE-94, confidence LOW)                                                                          |
| Path / column      | `internal/cli/mcp_start_work.go`, column 9                                                                                                                                                            |
| Baseline entry     | `semgrep:1c1fe971…86180`, line 721                                                                                                                                                                    |
| Shifted finding    | `semgrep:bba96ea0…c8873`, line 727 (seen only while stage 1 was present)                                                                                                                              |
| Matched code       | `cmd := exec.Command(exe, args...)` — unchanged since 2026-10-27 (`git blame`)                                                                                                                        |
| Fingerprint source | semgrep OSS returns `"requires login"`; SOP computes `StableFingerprint(provider, ruleID, path, line, column)` (`internal/security/normalize.go:116`, used at `internal/security/semgrep.go:361-363`) |

Reproduction: `("semgrep","dangerous-exec-command","internal/cli/mcp_start_work.go",721,9)`
hashes to `1c1fe971…`; the same tuple with `727` hashes to `bba96ea0…`. Only the line
changed.

**Security implication of the finding:** none. `argv` is an array, not a shell string;
`exe` is `os.Executable()`; the arguments are a fixed `prompt --capability <validated>
--file <path inside the job dir>` or `run <confined plan path>`; no shell is invoked
and no untrusted text reaches `argv`. The existing baseline reason remains true.

**Recurrence risk (the real decision, due when stage 3 lands).** Stages 3, 4 and 5
edit `startWork`, which sits above `launchWorkProcess`, so the line moves again and
the same drift recurs. Decide how to handle it before stage 3 merges:

- **Option A — refresh the existing entry after each layout change.** Replace only the
  `fingerprint` value and append a dated note to `reason`; keep `rule_id`, `path`,
  `owner` and `expires_at`. Expect a refresh after stages 3–5.
- **Option B — move the launcher to its own file (recommended if stages 3–5 proceed).**
  Relocate `launchWorkProcess` to `internal/cli/launch.go` in a change of its own,
  then replace the entry with one whose `path` is the new file. Later edits to
  `startWork` no longer move the match. Cost: a code move and a baseline path change;
  the rule still fires, so this is not a suppression.
- **Option C — no change.** Accept that the required scan blocks stages 3–5.

Not requested: any new suppression, any severity or `fail_on` change, any exclusion,
or any edit to `.agent-sdlc/security-rules/`. The full fingerprint value must come
from the operator's own re-run, not from this document's elision.

## Decision 3 — reconcile the edited plan

Two separate questions.

**3a. Acceptance mapping.** The mapping document is merged and matches all 16 task
IDs; its section titles are the **compiled task titles** shown by `sop status`, which
differ from the plan's `### N.` headings. IDs are authoritative.

- **Option A — keep the mapping separate (recommended).** Record that
  `ACCEPTANCE-MAPPING-ACP-EXECUTION-LEASE-WIRING.md` is the authoritative acceptance
  reference for stages 2–16. No plan edit for it, no digest change.
- **Option B — append the mapping to the plan.** Edit the plan and reconcile (3b).
- **Option C — canonicalize the plan.** Rewrite every stage into the rendered shape
  (`## <stage-id> — <title>`) so compilation becomes deterministic and
  `### Acceptance Mapping` feeds `Stage.AcceptanceMapping`. Highest review cost.

**3b. The plan has already changed.** Dropping stage 1 edited the plan (step 1 is now a
"Dropped" stub; steps 2 and 4 and the open questions changed). The plan is not written
in the deterministic rendered shape, so `Compile` falls back to the model
(`internal/planner/markdown.go:351-391`); any plan edit therefore forces a model-based
recompile that is not guaranteed byte-stable. The "unchanged" preview is a
deterministic no-op (`internal/planflow/planflow.go:823-846`) and proves nothing about
a changed plan. Decide how to reconcile:

- **Option A — preview, then reconcile (recommended).** Run
  `sop reconcile docs/plans/PLAN-ACP-EXECUTION-LEASE-WIRING.md --list-changed` and read
  the result. If only `stage-1` changed, apply with `--accept-changed stage-1`. If any
  other executed task changes, stop and review. Never edit `plan.meta.json` or
  `plan.json` by hand.
- **Option B — revert the plan edits and record the drop only in the design record.**
  Restores the digest but leaves a `stage-1` task that SOP would try to run, so it
  needs a separate instruction to skip it. Not recommended.
- **Option C — start a new plan.** Save the revised plan under a new name and let SOP
  create a fresh plan; the persisted stage-1 and stage-10 evidence stays under
  `.agent-sdlc/runs/`. Cleanest history, but a larger change.

The reconcile preview may call the configured model (cloud, per project config), so
confirm that is acceptable before running it.

## Decision 4 — merge authorization

No merge without explicit approval. The PR that carries this packet is documentation
only and changes no production behaviour.

## Post-approval sequence (for reference; not executed)

1. Record Decisions 1–3 in a decision record (template below).
2. Preserve the blocked evidence:
   `cp -R .agent-sdlc/runs/stage-1 .agent-sdlc/runs/stage-10 .agent-sdlc/recovery-20261121/`.
3. Reconcile the edited plan per Decision 3b, with `--accept-changed` only for tasks
   that legitimately changed (`stage-1`).
4. Re-run the required security scan after each stage that edits `mcp_start_work.go`
   (the baseline matched on 2026-11-21 with stage 1 dropped). Apply a baseline change
   only if the operator authorized one.
5. `sop retry stage-10` (it was blocked by the same scan finding). Do not retry
   `stage-1`: it is dropped. Never use `sop retry --all`.
6. Continue with `sop run` under the normal lifecycle, from stage 2.

## Approval record template

```md
# Operator decision — ACP execution-lease wiring

**Status:** <APPROVED | REJECTED | DEFERRED> — decided by <name> on <YYYY-MM-DD>.

- **Decision 1 — plan:** <approved | rejected>. Scope: PLAN-ACP-EXECUTION-LEASE-WIRING
  stages <2-16 | subset>. Design decisions D1–D9: <accepted | accepted with changes:
  …>.
- **Decision 2 — security baseline:** <no change needed (scan re-run clean) | authorized:
  Option A | Option B | not authorized>. Entry: rule `dangerous-exec-command`, path
  `internal/cli/mcp_start_work.go`. Conditions: <…>.
- **Decision 3 — plan reconcile:** mapping <Option A | B | C>; reconcile <Option A | B
  | C>. `--list-changed` output reviewed: <yes | no>.
- **Decision 4 — merge:** <authorized per PR | not authorized>.
- **Expiry / review date:** <…>.
- **Evidence:** this packet, the design record, the gap record, run artifacts under
  `.agent-sdlc/runs/stage-1` (the dropped attempt) and `.agent-sdlc/runs/stage-10`.
```

## Evidence index

- `internal/executionlease/lease.go` — `Acquire`, `Held.Release`, `WatchHeartbeat`,
  `State.Stale`, `ReleaseStale`, `HeartbeatEvery`/`HeartbeatLimit`/`Grace`.
- `internal/cli/execution_lease.go` — `executionLease`.
- `internal/cli/mcp_start_work.go` — `startWork`, `acquireWorkLock`, `launchWorkProcess`.
- `internal/cli/acp.go` — `acpSubmission`, the 250 ms `projectlock` probe.
- `internal/cli/resume.go` — `reportStaleExecutionLease`.
- `internal/security/normalize.go`, `internal/security/semgrep.go`,
  `internal/security/baseline.go` — fingerprint computation and drift behaviour.
- `.agent-sdlc/runs/stage-1`, `.agent-sdlc/runs/stage-10` — `gate.json`,
  `classification.json`, `security/scan.json` (the single blocking finding while stage
  1 was present).

## Not requested

- No new security suppression, exclusion, severity change or scan weakening.
- No manual task-status edit, no `sop retry --all`, no bypass of reconciliation.
- No deletion of worktrees, run artifacts or uncommitted work.
- No release-acceptance claim for the ACP exclusion gap.
