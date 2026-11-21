# Goldfish Review Report

**Target:** `/Users/imhttran/agentic-workspace/agentic-sop/docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` (lines 1-110)

Context files given to both reviewers: `PLAN-SOP-ACP-ZED.md`, `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`.

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-zoom-out` | completed |
| `egm-inventive-catalyst` | completed |

Severity breakdown: 0 Blocker, 7 Major, 3 Nit (after deduplication).

## Prioritized Findings (Blockers -> Major -> Nits)

### Gap is structural, not "remaining wiring": ACP/MCP start path never consults the lease - Severity: Major
* **Reviewer Lens:** `egm-inventive-catalyst` (supported by `egm-zoom-out` simplicity note)
* **Affected Scope:** lines 11-17, 74-101; PLAN-SOP-ACP-ZED.md line 291
* **Discrepancy Description**: The doc accepts "off by default" and an incremental rollout. Per lines 97-101 no interface can see another interface's running execution. The CLI holds a lease, but the ACP/MCP `startWork` path (which launches the child) never reads it. The plan requires the lock to protect "CLI, MCP, and ACP equally".
* **Rationale & Recommendation**: Add a read-only lease consult inside `startWork` under the existing start lock. Refuse with `busy` and the holder when a live lease exists. This can ship ahead of `execution.lease: required` and closes the CLI-first/ACP-second case in one step. Child-side acquisition stays a separate step. An `advisory` mode could collect contention evidence early. Risks: `startWork` gains a dependency on the lease package, and a stale or corrupt lease must be reported, not stolen. The doc should also state which option the remaining work takes (consult in `startWork` vs. delegating ownership to the child, proposal line 42).

### Window between start-lock release and child lease acquisition - Severity: Major
* **Reviewer Lens:** `egm-inventive-catalyst`
* **Affected Scope:** lines 48, 86-101; PROPOSAL §1 "Two-phase for starts", §2 (lines 46-47, 54, 60-62)
* **Discrepancy Description**: The start lock is released right after launch and the child acquires the lease as its first act. A `sop run` or a second ACP/MCP start can slip in during that gap. The decision and the ownership are separate acts.
* **Rationale & Recommendation**: Have the parent pre-acquire a pending lease (job id, no pid) under the start lock and hand it to the child by pid update. The proposal's "no pid means in-progress for a grace period" rule already supports this. It also replaces the more fragile env-marker no-nesting check. The parent must release the pending lease if the child fails to launch, the grace constant must be documented, and the pid-update handoff races with fast child exit.

### Three separate acquisition sites invite another omission - Severity: Major
* **Reviewer Lens:** `egm-inventive-catalyst`
* **Affected Scope:** lines 42-48, 97-100; PROPOSAL §5 (lines 96-101)
* **Discrepancy Description**: Acquisition would live in `sop run`, `sop prompt` and the ACP/MCP parent separately. The gap exists because `sop prompt` was skipped once already.
* **Rationale & Recommendation**: Route all executing commands through one shared entry point and add an archtest guard that fails if an execution path bypasses it. This touches the CLI execution core, which the proposal's non-goal (lines 140-141) puts out of scope until approval, so it needs sequencing. The guard must define "execution path" precisely.

### Table of CLI paths is inconsistent and omits `sop drive` - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 39-48 (table), 80-84, 82-101
* **Discrepancy Description**: Line 46 says `sop run` has "none" under execution/start lock, yet lines 80-84 say it acquires the lease when `execution.lease` is set. `internal/cli/drive.go:83` also acquires a lease (`KindRun`), and `sop drive` appears nowhere in the doc.
* **Rationale & Recommendation**: Mark the table as the default (`off`) configuration or add a "with lease on" column. Add a `sop drive` row or say it shares `sop run` behavior.

### No "remaining to close" checklist; evidence list incomplete - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 3-5, 97-110
* **Discrepancy Description**: The doc mixes gap analysis, a lease-wiring changelog and evidence. The single open item is hard to find. "Evidence to cite" omits `internal/executionlease`, `internal/cli/execution_lease.go`, `execution_lease_test.go` and `resume_lease_test.go`. The closing line promises "deterministic tests below", but only the one ACP/MCP test is listed. Proposal tests 1, 2, 4 and 6 would close the gap and are not enumerated.
* **Rationale & Recommendation**: Add a checklist (sop prompt acquisition, MCP/ACP consult or child acquisition, default flip), each paired with the proposal test number. Extend the evidence list.

### Plan says "Implemented and merged" while its acceptance criterion is unmet - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`, `egm-inventive-catalyst` (merged duplicates)
* **Affected Scope:** lines 3-4, 62-64, 97-101; PLAN-SOP-ACP-ZED.md line 4, ACP-003 criterion 3 (line 106), matrix row line 291, Definition of done lines 318-324
* **Discrepancy Description**: The plan's status line and acceptance matrix read as satisfied. This gap doc says cross-interface exclusion is unmet. Neither plan location points at the gap doc.
* **Rationale & Recommendation**: Link the plan's status line, matrix row and ACP-003 criterion 3 to this gap, or record an explicit waiver/deferral. Optionally add an archtest check that fails when an OPEN gap doc exists without a plan reference.

### "Why this is not a regression" is unevidenced and sidesteps the requirement - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 66-72
* **Discrepancy Description**: The claim that the earlier ACP design held the project metadata lock across read and start cites no file or commit. Absence of regression also does not change that the plan's line 18 requirement is unmet.
* **Rationale & Recommendation**: Cite the revision or drop the sentence. Shorten to one line: "pre-existing property; not introduced by ACP; still required by the plan".

### Heartbeat detail is an altitude drop and its failure mode is unmitigated - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`, `egm-inventive-catalyst` (merged)
* **Affected Scope:** lines 86-96
* **Discrepancy Description**: Heartbeat interval, stderr reporting and Windows pid liveness obscure what is unwired. Separately, a heartbeat failure is only logged to stderr, yet a stale heartbeat can make a live run look stale.
* **Rationale & Recommendation**: Compress to two or three sentences and link to the proposal or package docs. In the proposal, consider bounded retry plus a persisted "holder alive, heartbeat failing" state distinct from stale, with a pid-liveness check first on the same host.

### Start-lock path named inconsistently across documents - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 24, 39, 43; PROPOSAL line 28
* **Discrepancy Description**: The gap doc says `.agent-sdlc/runs/work/.lock`. The proposal says `state/runs/work/.lock`.
* **Rationale & Recommendation**: Verify against `acquireWorkLock` in `mcp_start_work.go` and align both docs.

### Evidence cites private implementation symbols - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 44, 107-108
* **Discrepancy Description**: Rows cite `internal/cli/acp.go`, `acpSubmission`, `LockTaker`, `StateReader` and `Starter`, which will rot.
* **Rationale & Recommendation**: Keep one pointer per interface and rely on tests as the durable evidence.

## Notes
* Both reviewers found no Blockers and judged the doc an honest gap record whose proposed fix reuses the existing `internal/executionlease` primitive.
* Neither reviewer read the Go sources in full. The zoom-out reviewer checked only grep hits, and the inventive-catalyst reviewer verified no code claims. The code-level claims are therefore unverified.
