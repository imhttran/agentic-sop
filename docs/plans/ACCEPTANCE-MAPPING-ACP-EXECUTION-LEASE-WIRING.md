# Acceptance Mapping — `PLAN-ACP-EXECUTION-LEASE-WIRING.md`

Status: **prepared and verified against the compiled plan; not approved and not part
of the plan.** Verified 2026-11-21 against `main` = `c673d442`: the 16 stage IDs
match `plan.json` and `sop status` one-for-one. The section titles below are the
**compiled task titles** shown by `sop status`; the source plan numbers the same
stages with different `### N.` headings, so the IDs are authoritative.

It is deliberately separate from the plan. The plan is not in the deterministic
rendered shape, so any plan edit forces a model-based recompile
(`internal/planner/markdown.go:351-391`), and the plan's digest is recorded in
`.agent-sdlc/plan.meta.json` (`source_sha256`). See the operator approval packet,
Decision 3, for the three integration options.

Approval status: **pending operator decision**
(`PLAN-ACP-EXECUTION-LEASE-WIRING-OPERATOR-APPROVAL-PACKET.md`).

Rules used here:

- A criterion is **met** only when the named test exists and passes, or the named
  evidence exists in the tree. Planned tests are labelled `planned` and are not
  evidence.
- Manual evidence names a file and section, never an unquoted claim.
- No stage is marked met because a different stage passed.

## Coverage matrix

| Dimension                        | Stage(s)  | Evidence today                                                                                                                                                                                                                                                                                                             |
| -------------------------------- | --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Unit-level lease correctness     | 1, 12, 14 | exists: `internal/executionlease/lease_test.go` (Acquire, heartbeat, staleness, `ReleaseStale`, recovery claim), `internal/cli/execution_lease_test.go` (off, required, advisory, unknown mode); `internal/executionlease/inherit_test.go` and `internal/cli/execution_lease_inherit_test.go` are uncommitted stage-1 work |
| ACP-vs-ACP start exclusion       | 8 (base)  | exists: `TestACPPromptConcurrentStartsCreateOneWorkItem` (`internal/cli/acp_test.go`)                                                                                                                                                                                                                                      |
| CLI-vs-ACP execution exclusion   | 6         | missing                                                                                                                                                                                                                                                                                                                    |
| MCP-vs-ACP start exclusion       | 8         | exists at the start-lock layer: `internal/cli/start_exclusion_test.go` (three tests); lease layer missing                                                                                                                                                                                                                  |
| `sop prompt` execution exclusion | 2, 7, 14  | missing                                                                                                                                                                                                                                                                                                                    |
| Concurrent state updates         | 6, 7, 13  | missing (no test asserts `state.db` is unchanged)                                                                                                                                                                                                                                                                          |
| Run artifact preservation        | 6, 7, 9   | missing (no test asserts `runs/<task-id>/` is untouched)                                                                                                                                                                                                                                                                   |
| Task graph integrity             | 6, 7, 13  | missing                                                                                                                                                                                                                                                                                                                    |
| Lease release and recovery       | 9, 10, 11 | partial: `TestRunReleasesTheLeaseOnExit`, `TestRunResumeReleasesStaleExecutionLease`, `TestRunResumeLeavesLiveExecutionLease`; no cancel/completion race, no parent-death test                                                                                                                                             |
| Error handling and regression    | 3, 5, 14  | partial: `TestExecutionLeaseRejectsAnUnknownMode`, `TestExecutionLeaseContention`, `TestStartWorkRefusesWhileATaskIsMidRun`, `TestExecutionLeaseOffByDefault`; no cross-interface off table                                                                                                                                |

---

## stage-1 — Resolve lease ownership conflict and add inheritance and acquire-or-inherit helpers

### Acceptance Mapping

- criterion 1 (ownership conflict settled in favour of marker-plus-inert-handle):
  manual: `internal/executionlease/inherit.go` and `leaseAcquireOrInherit` in
  `internal/cli/execution_lease.go`; the plan's open question must be closed in the
  plan text at the next revision.
- criterion 2 (marker identity is checked, not trusted):
  `TestExecutionLeaseInheritMatchesLiveParent`,
  `TestExecutionLeaseInheritRejectsMismatch`,
  `TestExecutionLeaseInheritRejectsUnparsableRecord`
  (`internal/executionlease/inherit_test.go`, uncommitted).
- criterion 3 (off is inert): `TestExecutionLeaseInheritIsInertUnderOff` plus the
  marker-present off case in `internal/cli/execution_lease_inherit_test.go`
  (uncommitted).
- criterion 4 (`executionLease`, `run.go` and `drive.go` unchanged):
  `git diff --stat` on those three paths shows no change from `executionLease`'s
  existing behaviour; the new code is an unused sibling.
- status: **blocked** — the change is present in the working tree but the required
  security scan failed on fingerprint drift (see the run report).

## stage-2 — Wire the execution lease into `sop prompt`

### Acceptance Mapping

- criterion 1 (prompt acquires before any metadata write):
  planned: `TestPromptHoldsExecutionLeaseForItsDuration`.
- criterion 2 (required refuses with the holder named and mutates nothing):
  planned: `TestPromptRequiredRefusesWhenLeaseHeld` (compare `state.db` bytes and
  the run directory listing before and after).
- criterion 3 (off is a strict no-op for prompt):
  planned: extend the off table (`TestExecutionLeaseOffChangesNothing`).
- status: **not implemented**.

## stage-3 — Make `startWork` consult a held lease and return `busy`

### Acceptance Mapping

- criterion 1 (required refuses under the start lock, no job dir, no child):
  planned: `TestStartWorkReturnsBusyWhileLeaseHeld` (a fake launcher that fails the
  test if called).
- criterion 2 (advisory records contention and proceeds):
  planned: same test's advisory table row.
- criterion 3 (off ignores the lease file):
  planned: `TestStartWorkIgnoresLeaseUnderOff`.
- criterion 4 (ACP maps the busy doc without change):
  planned: `TestACPMapsLeaseBusyToBusyOutcome`; current mapping is
  `acpStartWorkResult` in `internal/cli/acp.go`.
- status: **not implemented**.

## stage-4 — Launch the child with the lease identity and close the handshake window

### Acceptance Mapping

- criterion 1 (a started job is confirmed owner, or explicitly lease-pending):
  planned: `TestStartWorkWaitsForChildLeaseHandshake`,
  `TestStartWorkReportsLeasePendingOnHandshakeTimeout`.
- criterion 2 (a CLI run in the window wins the lease and the child fails fast):
  planned: a variant of stage-6's CLI-first test with the child launched first.
- criterion 3 (`started` is never presented as proof of exclusion):
  manual: the response contract in `startWork`'s doc comment and the ACP
  `SubmissionResult` mapping.
- status: **not implemented**.

## stage-5 — Record a child lease-busy failure in the job record

### Acceptance Mapping

- criterion 1 (failed child is observable with the holder):
  planned: `TestLeaseBusyChildRecordedAsFailedWithHolder` (assert `exit.json`
  carries the busy marker and `sop_work_status` names the holder).
- criterion 2 (no automatic retry or resubmission):
  manual: `internal/cli/work_cancel.go` / job exit recording; no retry path exists.
- status: **not implemented**.

## stage-6 — Proposal test 1: CLI-first, ACP-second

### Acceptance Mapping

- criterion 1 (ACP start refused as busy, holder named, no child, no job):
  planned: `TestACPPromptRefusedWhileCLIRunHoldsLease`.
- criterion 2 (the holder is the real run path, not a synthetic lease):
  planned: in-process `runSingleTask` variant against a temp project with the fake
  agent/reviewer seams.
- criterion 3 (no task mutation):
  planned: compare `state.db` bytes and the `tasks` rows before and after.
- criterion 4 (off variant green):
  planned: off row in the same table.
- status: **not implemented**; this is the first cross-entrypoint acceptance case.

## stage-7 — Proposal test 2: ACP-first, CLI-second

### Acceptance Mapping

- criterion 1 (single-task `sop run` refuses, names the prompt child, mutates nothing):
  planned: `TestRunRefusesWhileACPLStartChildHoldsLease`.
- criterion 2 (graph-mode `sop run` refuses identically):
  planned: `TestRunGraphRefusesWhileChildHoldsLease`.
- criterion 3 (no task status change):
  planned: row comparison, as in stage 6.
- status: **not implemented**.

## stage-8 — Proposal test 3: ACP vs MCP concurrent starts

### Acceptance Mapping

- criterion 1 (exactly one governed launch across ACP and MCP, both orders):
  **met at the start-lock layer**:
  `TestACPPromptAndMCPStartWorkRaceCreateOneWorkItem`,
  `TestACPPromptAndMCPStartWorkRaceDifferentWorkStillOneLaunch`,
  `TestMCPAndACPStartsExcludeEachOtherInSequence`
  (`internal/cli/start_exclusion_test.go`).
- criterion 2 (the loser's outcome is a refusal, not a silent success):
  same tests (busy / already_running / start-lock refusal).
- criterion 3 (lease-level exclusion once the child owns the lease):
  planned: extend the same tests to assert one lease record owned by the launched
  child.
- note: criterion 1 passing does not close cross-entrypoint **execution** exclusion.
- status: **partially met** (start lock only).

## stage-9 — Proposal test 4: cancel and completion race

### Acceptance Mapping

- criterion 1 (one terminal result, one exit record, one lease release):
  planned: `TestCancelDuringLeaseReleaseYieldsOneResult`.
- criterion 2 (no second lifecycle event):
  planned: same test, asserting a single `exit.json` write and a single emitted
  transition.
- status: **not implemented**.

## stage-10 — Proposal test 5: stale and live lease on resume (confirm existing)

### Acceptance Mapping

- criterion 1 (stale lease reported and released; next start succeeds):
  **met**: `TestRunResumeReleasesStaleExecutionLease`
  (`internal/cli/resume_lease_test.go`).
- criterion 2 (live lease reported and never released):
  **met**: `TestRunResumeLeavesLiveExecutionLease` (same file).
- criterion 3 (gap record cites both without claiming more):
  **met**: `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md`, “Proposal test 5”.
- status: **met** (doc-only stage).

## stage-11 — Proposal test 6: parent death leaves the child lease intact

### Acceptance Mapping

- criterion 1 (adapter/session EOF does not free a live child's lease):
  planned: `TestACPAdapterExitLeavesChildLeaseIntact` (seam level: close the input,
  then assert the lease record is still present and names the child job).
- criterion 2 (a real parent kill leaves the lease owned by the live child):
  planned: a subprocess test modelled on
  `TestLockReleasedOnProcessExit` (`internal/projectlock/lock_test.go`).
- status: **not implemented**.

## stage-12 — Proposal test 7: the lease is per project

### Acceptance Mapping

- criterion 1 (two projects acquire independently):
  planned: `TestExecutionLeaseIsPerProject` (`internal/cli/execution_lease_test.go`
  or `internal/executionlease/lease_test.go`).
- criterion 2 (releasing one leaves the other held):
  planned: same test.
- criterion 3 (the lease path is project-relative):
  manual: `leasePath` in `internal/executionlease/lease.go` joins `config.DirName`.
- status: **not implemented** (cheap; the primitive already supports it).

## stage-13 — Proposal test 8: lease-before-metadata ordering stress

### Acceptance Mapping

- criterion 1 (no deadlock between the lease and the metadata lock):
  planned: `TestExecutionLeaseBeforeMetadataLockCannotDeadlock`.
- criterion 2 (every lease-then-metadata site is documented):
  manual: `internal/cli/drive.go:83` precedes the `planflow` lock sites; add the
  `sop prompt` site in stage 2.
- status: **not implemented**.

## stage-14 — Proposal test 9: off changes nothing across all interfaces

### Acceptance Mapping

- criterion 1 (no interface creates or removes `state.exec.lock` under `off`):
  planned: `TestExecutionLeaseOffChangesNothing`, with a pre-created lease file.
- criterion 2 (prompt, run, startWork and ACP keep their current outcomes):
  planned: same table; assert the recorded outcome per interface.
- criterion 3 (existing unit guard remains):
  **met**: `TestExecutionLeaseOffByDefault` (`internal/cli/execution_lease_test.go`).
- status: **partially met**.

## stage-15 — Documentation and gap record update

### Acceptance Mapping

- criterion 1 (interface table matches the code):
  **met** for the current code (no interface acquires or checks the lease).
- criterion 2 (test-status table names the new tests and no unexplained gaps):
  **met** for tests 3 and 5; tests 1, 2, 4, 6, 7, 8, 9 are named as gaps.
- criterion 3 (no release acceptance claimed):
  **met**: the gap record states the requirement is unresolved.
- status: **met** for the pre-wiring state; must be re-checked after each stage.

## stage-16 — Rollout and rollback execution

### Acceptance Mapping

- criterion 1 (off shipped with no behaviour change):
  **met** by construction (`config.yaml` has no `execution` block; `ParseMode("")`
  is off).
- criterion 2 (advisory observations recorded per project):
  planned: operator run; record stderr contention lines.
- criterion 3 (required enabled for ACP/MCP first, then CLI):
  planned, gated on stages 1–14.
- criterion 4 (rollback verified by the off table):
  planned: run stage 14's table after setting `off`.
- status: **not started**.

## What this mapping does not do

- It does not change any acceptance criterion in the plan.
- It does not mark a stage met because another stage passed.
- It does not treat start-lock exclusion (stage 8) as execution exclusion, and it
  does not treat tests 1, 2, 4 or 6 as covered until they exist and pass with
  `execution.lease: required`.
