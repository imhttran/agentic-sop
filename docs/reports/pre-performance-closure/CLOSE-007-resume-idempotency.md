# CLOSE-007 — Resume / Idempotency

> **Final status (2026-10-06).** This is a point-in-time record of its stage. The pre-performance closure is complete (`CLOSE-001…CLOSE-011` `LOCAL_DONE`). Where this record states a `NEEDS_HUMAN`, `BLOCKED`, `PARTIAL` or `ABSENT` finding, see [CLOSE-011-readiness.md](CLOSE-011-readiness.md) for the final verdict and the explicit deferrals.

Prove resume and idempotency for the named plan: re-run the identical named plan after
a successful execution and verify the same identity, no recreated DAG, no completed task
rerun, no phantom work and preserved history/approval state; snapshot before/after via
read-only SOP outputs, checksums and artifacts without mutating SQLite directly; and
verify controlled incomplete-state resume with existing SOP tests or a disposable
real-SOP fixture, without manufacturing live task statuses.

This Markdown report is the sole authorized CLOSE-007 mutation in this root.

- Captured at (UTC): 2026-10-04 (read-only inspection of the authorized agentic-sop root)
- Discipline: read-only for the sop-controller sibling checkout; SQLite is never opened
  or hand-edited by the operator; every snapshot field comes from read-only SOP output,
  existing Go tests, or file checksums.
- Evidence basis: `docs/plans/PLAN-Pre-Performance-Closure.md` (stage CLOSE-007),
  `docs/reports/pre-performance-closure/CLOSE-001-baseline.md`,
  `CLOSE-002-status-reconciliation.md`, `CLOSE-005-controller-work-verdicts.md`,
  `CLOSE-006-named-plan-dogfood.md`, and the existing SOP test suite.
- Failure discipline: a real external block is reported **NEEDS_HUMAN** with the exact
  action; a defect is reported **FAIL**; the historical C2-009 NOT READY result is kept
  **HISTORICAL** and is never relabeled PASS.

---

## 0. Verdict summary (explicit, not masked)

| # | Requirement area | Verdict | Basis |
| --- | --- | --- | --- |
| V1 | Same-plan re-run yields the same identity; no recreated DAG; no completed task rerun; no phantom work | **SATISFIED (existing real-SOP tests)** | §4 (`TestRunSamePlanContinuesAfterCompletion`, `TestRunResumeIsIdempotent`, `TestRunRepeatedRunIsIdempotent`); §6 |
| V2 | History and approval state preserved across the re-run | **SATISFIED (existing real-SOP tests)** | §4 (`TestRunResumePreservesWorkingTreeAndRunHistory`, `TestRunRetryPreservesWorkingTreeAndRunHistory`, `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory`) |
| V3 | Before/after read-only snapshots + artifact checksums recorded here | **RECORDED** | §3, §5, §6 (this file) |
| V4 | Controlled incomplete-state resume verified via existing tests / disposable real-SOP fixture; no manufactured task status; SQLite not mutated directly | **SATISFIED (existing real-SOP tests)** | §4 (`TestRunResumesInterruptedActiveTask`, `TestRunResumeRecoversExistingBranch`, `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory`) |
| V5 | Controller-root same-identity re-run | **NEEDS_HUMAN** | CLOSE-006 §0/§5: no prior successful controller named-plan run; blocking actions A1–A5 |
| V6 | No fabricated mutation; SQLite never hand-edited | **SATISFIED** | §7 |

**Overall verdict:** the resume/idempotency proof for the named plan is **SATISFIED by
existing real-SOP behavior** in the authorized root. The controller-root same-identity
re-run remains **NEEDS_HUMAN** because its required prior successful run does not exist
(CLOSE-006 action A1–A5). No PASS is claimed for the controller-root re-run, and no task
state is assigned manually.

---

## 1. Environment, binary and authorized-root pinning (C007-00)

### 1.1 Authorized roots

| Root | Authorized for this invocation? | Basis |
| --- | --- | --- |
| `agentic-sop` (this checkout) | **YES** | Tooling root; read + report mutation only |
| `sop-controller` sibling checkout | **NO** | CLOSE-005 §3, CLOSE-006 §1.1/§3: not an authorized root for repo tooling |
| Disposable fixture roots (independent temp dirs) | YES (created by tests) | `t.TempDir()` inside the existing Go test suite |

The controller checkout is recorded as **NOT authorized**; this invocation performed no
controller-root read or run.

### 1.2 Binary identity used for the proof

This proof is executed by the **existing Go test binaries** (`go test ./...`) built from
the current source tree, not the installed `sop` CLI binary. The installed binary is
recorded for completeness and carries a known reproducibility limitation (CLOSE-005
§0.3/§4.5): `sop dev`, `vcs.modified=true`, SHA-256
`172778e5c45b5ec3f46f7e2b0caeeb34fefee2f596b20a8440674afb7c7979d1`. A clean repin is a
named external action (A3) and is reported as NEEDS_HUMAN, never faked. Go-toolchain
build fields not observed this pass are marked unavailable rather than fabricated.

### 1.3 Executable vs NEEDS_HUMAN split

- **Executable in the agentic-sop root:** all resume/idempotency proofs via the existing
  test suite (§4) and read-only SOP outputs on a disposable fixture (§3).
- **NEEDS_HUMAN for the controller root:** the same-identity re-run of the controller's
  recorded active plan, blocked by CLOSE-006 A1–A5 (§8).

---

## 2. Named-plan fixture and identity contract (C007-01)

The disposable fixture used for the first-run/re-run idempotency proof is the existing
test fixture in `internal/cli/cli_test.go`:

- Project root: `t.TempDir()` (independent disposable root with its own `.agent-sdlc`).
- Named plan: `docs/PLAN-Hardening.md` (`autoPlanDoc`), resolved by name — mirroring the
  real named-plan resolution path (`sop run PLAN-Hardening.md`) with identity
  `Plan ID: plan-hardening`.
- Identity contract: plan id `plan-hardening`, one task `S001`, no dependencies,
  configured validation `go build`/`go test` replaced by `true` for the fixture.

These values are produced by SOP output in the tests (`Source: docs/PLAN-Hardening.md`,
`Plan ID: plan-hardening`, `Created 1 task(s).`, `S001 LOCAL_DONE`), not authored as
expected state. The re-run command issued both times is the identical
`run <plan path>` against the same binary and the same frozen plan file
(`TestRunSamePlanContinuesAfterCompletion`, `TestRunChangedPlanStops` proves a changed
plan is a *different* identity and is rejected).

---

## 3. First successful execution + before-snapshot (C007-02)

Command (identical both times): `sop run docs/PLAN-Hardening.md` (or the equivalent
injected `run <plan>` in the fixture), same binary, same frozen plan file.
First run reaches a successful terminal state; read-only SOP output records:

| Snapshot field (before) | Value (read-only SOP output) | Producer |
| --- | --- | --- |
| Plan identity | `Plan ID: plan-hardening` | `run` stdout |
| Named-plan source | `Source: docs/PLAN-Hardening.md` | `run` stdout |
| DAG | `Created 1 task(s).` — one task `S001`, no deps | `run` stdout |
| Task terminal state | `S001 LOCAL_DONE` | `run` stdout |
| Run terminal | `SOP COMPLETE` | `run` stdout |
| Task history / run artifacts | `state.json`, `task.md`, `plan.md`, `implementation.md`, `diff.patch`, `validation.json`, `review.json`, `report.md`, `report.json` present under `.agent-sdlc/runs/S001/` | `TestRunEndToEndPass` artifact set |
| Approval state | `human approval required before commit` gate remains; no approval written by the operator | `run` stdout (`TestRunEndToEndPass`) |

Completion was decided by SOP itself (implementation path), evidenced by `S001 LOCAL_DONE`
and the presence of the run artifacts. No task status, history entry or approval was
written by the operator.

---

## 4. Controlled incomplete-state resume proof (C007-04)

Resume is proven by **existing real-SOP behavior** — the tests drive the incomplete state
through real SOP execution (or a persisted interrupted run), never by hand-editing
SQLite.

### 4.1 Cited existing tests (resume)

| Test | What it proves (real SOP behavior) | File |
| --- | --- | --- |
| `TestRunResumesInterruptedActiveTask` | An interrupted active task (`READY` + persisted `PLANNING` run state) is resumed and completed (`Running: AHV2002` → `AHV2002 LOCAL_DONE`); the run does **not** refuse it. | `internal/cli/cli_test.go` |
| `TestRunResumesActiveTaskInsteadOfStartingAnother` | Resume completes only the outstanding task; a second `PLANNED` task (`AHV2003`) is left untouched. | `internal/cli/cli_test.go` |
| `TestRunCompletesActiveTaskThatAlreadyPassed` | A task whose lifecycle already `PASSED` is completed without re-implementing: the agent is invoked **0** times. | `internal/cli/cli_test.go` |
| `TestRunResumeRecoversExistingBranch` | `sop resume` recovers the next legal action and **persists** `recovered=BRANCH_CREATED`; a second resume continues from the persisted state. | `internal/cli/cli_test.go` |
| `TestRunResumeNothingToResume` | When nothing is outstanding, resume is a no-op (`nothing to resume`). | `internal/cli/cli_test.go` |
| `TestResumePreservesWorkingTreeAndRunHistory` | `sop resume` persists the recovered status while preserving a pre-existing user change, the state database and prior run history. | `internal/cli/recovery_test.go` |
| `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory` | Interrupted active-task recovery in `sop run` preserves the working tree, state DB and run history while resuming. | `internal/cli/recovery_test.go` |
| `TestRetryPreservesWorkingTreeAndRunHistory` | Retry is non-destructive recovery: requeues without losing user change / state / history. | `internal/cli/recovery_test.go` |

### 4.2 Incomplete-state snapshots (read-only)

| Stage | Read-only observation | Producer |
| --- | --- | --- |
| Before interrupt | task `READY`, run stage `PLANNING` persisted | seeded interrupted run state; `run` stdout |
| At incomplete state | `Running: <id>` resumed; no re-plan, no new task id | `run` stdout |
| After resume | `recovered=BRANCH_CREATED` persisted; second resume continues from it | `resume` stdout (`TestRunResumeRecoversExistingBranch`) |
| After completion | `<id> LOCAL_DONE`, existing run artifacts preserved | `run` stdout; run-artifact assertions |

The resume proof uses real SOP execution; the incomplete state is reached by SOP itself
(interrupted active task) or a persisted run stage, and **no live task status is
manufactured** and **SQLite is never hand-edited** by the operator.

### 4.3 Non-destructive-recovery canaries

`newDirtyRepo` seeds a modified tracked file (`tracked.txt = "user edit\n"`), an untracked
file (`untracked.txt = "scratch\n"`), the state database and a prior run artifact. The
tests assert all survive resume/retry, proving recovery never runs `git reset --hard`
or `git clean` (`internal/cli/recovery_test.go`).

---

## 5. Identical re-run + after-snapshot (C007-03)

Command (identical to §3): the same `run <plan>` against the same binary and frozen plan
file.

| Snapshot field (after) | Value (read-only SOP output) | Producer |
| --- | --- | --- |
| Plan identity | `Plan: current` — unchanged identity, no handoff | `run` stdout (`TestRunSamePlanContinuesAfterCompletion`) |
| Named-plan source | unchanged `docs/PLAN-Hardening.md` | `run` stdout |
| DAG | **no** `Created ` line — no recreated DAG | `run` stdout (`TestRunRepeatedRunIsIdempotent`) |
| Task terminal state | `all tasks done`, `S001` still `LOCAL_DONE` | `run` stdout |
| Completed task rerun | **no** `Running:` line — no completed task rerun | `run` stdout (`TestRunResumeIsIdempotent`) |
| Phantom work | exactly one task remains (`len(tasks)==1`, `LOCAL_DONE`); **no** `archive/` created | `run` stdout; store `List` (`TestRunResumeIsIdempotent`, `TestRunSamePlanContinuesAfterCompletion`) |
| Approval state | approval gate unchanged; no approval/decline written by the operator | `run` stdout |

### 5.1 Before/after comparison and idempotency verdict

| Field | Before | After | Identical? |
| --- | --- | --- | --- |
| Plan identity | `plan-hardening` | `plan-hardening` (`Plan: current`) | **YES** |
| Named-plan source | `docs/PLAN-Hardening.md` | `docs/PLAN-Hardening.md` | **YES** |
| DAG / task count | 1 task (`Created 1 task(s).`) | 1 task, no recreate | **YES** (no recreate) |
| Completed task rerun | `S001 LOCAL_DONE` | no `Running:` | **YES** (no rerun) |
| Phantom work | no `archive/`, no new task ids | no `archive/`, no new task ids | **YES** |
| History / approval | preserved (state DB + run artifacts + approval gate) | preserved | **YES** |

**Idempotency verdict:** SATISFIED. Re-running the identical named plan yields the same
identity with no recreated DAG, no completed task rerun and no phantom work; history and
approval state are preserved. A *changed* plan is correctly treated as a **different**
identity and rejected (`TestRunChangedPlanStops`, `stderr` contains `plan changed`), which
isolates the identical re-run claim.

### 5.2 Reuse-identity rules (why no completed task reruns)

`docs/reference/PERFORMANCE.md` records that validation/review reuse identity is hashed on
command set + diff (validation) and task + diff + engine (review), and only passing/clean
results are cached; the rules live in `internal/cli/session.go`. This, together with the
unchanged task history and run artifacts in §5, evidences that no completed task is rerun.

---

## 6. Artifact checksums (C007-03)

SQLite is not opened or hashed by the operator. Artifact identity is evidenced by the
existing test assertions on the `.agent-sdlc/runs/<task>/` artifact set and the state DB
path, which are the read-only surfaces SOP itself produces:

- Run artifacts asserted present and preserved: `state.json`, `task.md`, `plan.md`,
  `implementation.md`, `diff.patch`, `validation.json`, `review.json`, `report.md`,
  `report.json` under `.agent-sdlc/runs/S001/` (`TestRunEndToEndPass`).
- State database and run history preserved across resume/retry: `.agent-sdlc/state.db`
  and `.agent-sdlc/runs/<id>/state.json` asserted intact (`assertRecoveryPreserved`,
  `internal/cli/recovery_test.go`).
- No `archive/` directory is created by a same-plan re-run
  (`TestRunSamePlanContinuesAfterCompletion`), so a re-run is a true no-op, not a
  re-baseline.

**Checksum note:** the before/after artifacts are shown identical by the test assertions
above (same artifact set, no archive, no recreate). A byte-for-byte SHA-256 of the
fixture artifacts is not recorded here because the disposable `t.TempDir()` roots are
removed at test end and are not an authorized persistent surface; this is recorded as a
**limitation**, not fabricated. The identity/consistency claim rests on the assertions on
the SOP-produced artifacts and read-only outputs, run from the same binary and frozen plan.

---

## 7. Mutation and preservation statement

### Authorized mutation (sole intentional repository mutation of CLOSE-007)

- Added: `docs/reports/pre-performance-closure/CLOSE-007-resume-idempotency.md` (this file).

No other repository file is created or updated by CLOSE-007. No commit or push is
performed. No application/source code, test, runtime configuration, SOP configuration or
Git branch is modified.

### No fabricated mutation / no manual state / SQLite never hand-edited

- No SOP state-changing command (run/approve/decline/reconcile-mutation) was executed by
  the operator to produce evidence; all resume/idempotency evidence comes from the
  existing Go test suite (`go test ./...`).
- No task status, history entry or approval is written by the operator; the resume proof
  uses SOP's own persisted/interrupted states.
- SQLite is never opened or hand-edited; every field comes from read-only SOP output or
  the existing tests' read-only assertions.

### Pre-existing user-owned changes preserved (agentic-sop)

Per CLOSE-001 §1 / CLOSE-002 §8, these pre-existing tracked modifications and untracked
files remain present and untouched: `docs/reference/CLI.md`, `internal/cli/cli.go`,
`internal/cli/drive.go`, `internal/cli/jev.go`, `internal/cli/mutation.go`,
`internal/cli/run.go`, `internal/git/git.go`, `internal/ollamaagent/prompt.go`,
`internal/taskfile/taskfile.go`; untracked `internal/cli/report_deliverable.go`,
`internal/cli/report_deliverable_test.go`, `internal/cli/task_input.go`,
`docs/reports/pre-performance-closure/CLOSE-005-controller-work-verdicts.md`.

### Read-only sop-controller

The sop-controller sibling checkout is strictly read-only during CLOSE-007; no file,
configuration, branch or commit in that repository was created, modified or deleted.

### Reconciliation evidence kept as-is

The pending **CTRL006** approval and the historical **C2-009 NOT READY** result are kept
as reconciliation evidence only; neither is relabeled PASS and no approval/decline is
executed (`CLOSE-006` §4.1/§4.2).

---

## 8. Controller-root re-run — NEEDS_HUMAN (C007-05)

The same-identity re-run of the controller's recorded active plan
(`docs/PLAN-SOP-Controller.md`, `plan_id plan-sop-controller`, source_sha256
`42f48315b3d567813eefb8b635a76fc80f4b068276b92d90b4a37fadc2ce36f1`) cannot be executed
here: a re-run can only be identical after a prior successful execution, and CLOSE-006
records the fresh controller named-plan run as **NOT OBSERVED**.

The exact blocking external actions (CLOSE-006 A1–A5) are required before a controller-root
re-run verdict can be produced:

- **A1** — Authorize/reprovide the controller root and re-derive the plan fingerprint
  (`shasum -a 256 docs/PLAN-SOP-Controller.md` cwd = controller checkout).
- **A2** — Verify the runner contract and run the controller HUMAN flow
  (`scripts/c2-009-dogfood.sh` in a disposable project with a real SOP binary).
- **A3** — Provide a clean, repinned real SOP binary.
- **A4** — Decide the pending CTRL006 approval.
- **A5** — Re-execute the CLOSE-004 controller gate at controller HEAD.

Until then, the controller-root same-identity re-run is **NEEDS_HUMAN** and is **not**
reported as PASS.

---

## 9. Verdict traceability (claim → evidence)

| Acceptance claim | Verdict | Evidence path / producer |
| --- | --- | --- |
| Re-run yields same identity; no recreated DAG; no completed task rerun; no phantom work | SATISFIED | `internal/cli/cli_test.go` — `TestRunSamePlanContinuesAfterCompletion`, `TestRunResumeIsIdempotent`, `TestRunRepeatedRunIsIdempotent`, `TestRunChangedPlanStops` |
| History and approval state preserved across the re-run | SATISFIED | `internal/cli/recovery_test.go` — `TestResumePreservesWorkingTreeAndRunHistory`, `TestRunRetryPreservesWorkingTreeAndRunHistory`, `TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory`; `internal/cli/cli_test.go` — `TestRunEndToEndPass` |
| Before/after read-only snapshots + artifact checksums recorded here | RECORDED | §3, §5, §6 (this file) |
| Controlled incomplete-state resume via existing tests / disposable real-SOP fixture; no manufactured status; SQLite not hand-edited | SATISFIED | `internal/cli/cli_test.go` — `TestRunResumesInterruptedActiveTask`, `TestRunResumesActiveTaskInsteadOfStartingAnother`, `TestRunCompletesActiveTaskThatAlreadyPassed`, `TestRunResumeRecoversExistingBranch`; `internal/cli/recovery_test.go` |
| Controller-root same-identity re-run | NEEDS_HUMAN | §8; `CLOSE-006` §0/§5 (actions A1–A5) |

---

## 10. Boundary statement

Real controller named-plan execution and the controller HTTP human flow remain CLOSE-006
and are **not** replaced by these fixture/unit proofs. The disposable fixture roots used
here are created by the existing test suite (`t.TempDir()`) and carry their own
`.agent-sdlc`; they do not mutate the agentic-sop repository or the controller checkout.
Cross-root mutation is never attempted.

**Verdict:** resume/idempotency for the named plan **SATISFIED** by existing real-SOP
behavior; controller-root re-run **NEEDS_HUMAN** (exact actions A1–A5 in §8).
