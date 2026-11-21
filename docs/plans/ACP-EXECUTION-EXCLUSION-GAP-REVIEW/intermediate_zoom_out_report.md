# Goldfish Review Report

**Target:** `/Users/imhttran/agentic-workspace/agentic-sop/docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` (lines 1-110)

Stage 8 intermediate zoom-out. Stage 5 report for context: `/Users/imhttran/agentic-workspace/agentic-sop/zoom_out_report.md`. The document has not been edited since Stage 5, so most Stage 5 findings still apply.

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-zoom-out` | completed |

## Architecture verdict

The architecture holds. The document is an honest gap record: it does not claim completion, and it keeps the metadata lock, start lock and execution lease distinct. The proposed fix reuses an existing `executionlease` primitive rather than adding a scheduler. Lease ownership by the executing process fits the planned rollout (off, then advisory, then required). No Blockers.

## Prioritized Findings (Blockers -> Major -> Nits)

### Gap doc and plan are not cross-linked - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** gap doc lines 3-5 and 62-64; `PLAN-SOP-ACP-ZED.md` line 4, ACP-003 criterion 3 (line 106), acceptance-matrix row (line 291), Definition of done (lines 318-324)
* **Discrepancy Description**: The plan still says "Implemented and merged" and lists "Competing CLI/MCP/ACP starts obey the same lock" as an acceptance criterion. Nothing in the plan points to this gap. A reader of the plan alone would conclude the criterion is met. (Stage 5 finding, reaffirmed.)
* **Rationale & Recommendation**: Add a plan-side pointer, or an explicit deferral or waiver, at those three locations. This is the most valuable single mitigation.

### No remaining-work checklist; evidence list incomplete - Severity: Major
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 86-110
* **Discrepancy Description**: The closing line promises "deterministic tests below", but only one ACP/MCP test is listed. The open items are buried in changelog prose. The document also does not say which route the remaining work takes: consult the lease in `startWork`, or delegate acquisition to the child. (Stage 5 finding, reaffirmed.)
* **Rationale & Recommendation**: Add a short checklist: `sop prompt` acquires the lease; MCP and ACP consult the lease in `startWork` or delegate to the child; the default flips from off. Pair each item with the proposal test numbers (1, 2, 4 and 6 close the gap). Add `internal/executionlease`, `internal/cli/execution_lease.go` and the lease tests to the evidence list.

### Start-lock path is wrong - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 24 and 39
* **Discrepancy Description**: The doc says `.agent-sdlc/runs/work/.lock`. The code (`internal/cli/mcp_start_work.go` lines 51 and 691) uses `.start.lock` under `<stateDir>/runs/<workDirName>/`. The proposal says `state/runs/work/.lock`, so the two documents also disagree.
* **Rationale & Recommendation**: Correct both documents to the actual path and filename.

### Lock table is stale relative to lease work - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 42-48 against lines 80-84
* **Discrepancy Description**: The `sop run` row says "none" for the execution lock, yet `run.go:199` acquires the lease when the mode is not off. `drive.go:83` also acquires a lease (`KindRun`), and `sop drive` is absent from the table.
* **Rationale & Recommendation**: Label the table "with `execution.lease: off`" or add a lease-on column, and add a `sop drive` row.

### "Why this is not a regression" is unsourced - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 66-72
* **Discrepancy Description**: The claim cites no commit or file, and it does not change that the requirement is unmet.
* **Rationale & Recommendation**: Cite a revision or shorten it to one line.

### Heartbeat and resume detail is an altitude drop - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 86-96
* **Discrepancy Description**: Implementation detail sits in a gap-status record.
* **Rationale & Recommendation**: Compress it and link to the proposal or package docs.

### Gaps the comprehension and critic stages are likely to add - Severity: Nit
* **Reviewer Lens:** `egm-zoom-out`
* **Affected Scope:** lines 97-101; proposal sections 1 and 2
* **Discrepancy Description**: Anticipated additions, not verified against the code:
  * Race window: the start lock is released after launch, but the child acquires the lease only afterwards.
  * Cheapest first closure step: a read-only lease consult inside `startWork` under the start lock, returning `busy` with the holder. It fixes CLI-first then ACP-second without waiting for the `required` flip.
  * Three separate acquisition sites invite another omission like the `sop prompt` one.
  * A heartbeat failure is only logged to stderr, so a live run can look stale.
* **Rationale & Recommendation**:
  * Race window: have the parent pre-acquire a pending lease under the start lock and hand it over by pid update. The proposal's no-pid grace rule already supports this, and the grace constant must be documented.
  * Consult step: state in the doc which route the remaining work takes.
  * Acquisition sites: consider an archtest guard that fails when an execution path bypasses the shared acquire. Sequence it against the proposal's non-goal on changing CLI execution semantics.
  * Heartbeat failure: consider a "holder alive, heartbeat failing" state distinct from stale.

## Not checked

The lease package internals and tests, the ACP concurrency test, and the `midRunTasks` claims were not re-verified. If the Major items stay unaddressed, someone may read the plan as accepted while cross-interface exclusion is still unmet.
