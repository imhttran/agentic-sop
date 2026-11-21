# Goldfish Annotated Review Report

Status: resolved (auto accept; see the resolution summary in chat). Contested: 1. Escalated: 2 (plan row, blast radius). Declined: 5. Proposal-doc lock-path edit not applied.

**Target:** `docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` (`full file`, lines 1-110)
**Source findings:** `critic_report.md` (Stage 7). Stage 5, 6 and 8 findings are out of scope for this pass, except where they overlap (noted).

Checked against the code before categorizing:
- The start lock file is `.start.lock` (`internal/cli/mcp_start_work.go:51`, `workLockFile`). The gap doc says `.lock`. The reviewers' disagreement is settled: the doc is wrong.
- The lease file is `state.exec.lock` (`internal/executionlease/lease.go:37`).
- `sop drive` acquires the lease through the same helper as `sop run` (`internal/cli/drive.go:83`). The gap doc omits it.
- Users of `internal/executionlease`: `run.go`, `drive.go`, `resume.go`, `execution_lease.go`. `prompt.go`, MCP and ACP do not use it, as the doc says.
- `PLAN-SOP-ACP-ZED.md` line 4 says "Implemented and merged in PR #197" and its matrix row (line 291) says "Existing project lock protects CLI, MCP, and ACP equally". Both are as the reviewers said.

Summary: 6 PROPOSE FIX on Majors (one split), 1 CONTEST, 2 ESCALATE, 7 PROPOSE FIX on Nits, 5 DECLINE.

## Prioritized Findings & AI Suggestions

### [1] `egm-design` `egm-structural` - Severity: Major - Alternatives to the lease are not evaluated
*   **Affected Scope:** lines 74-101
*   ***AI Suggestion:*** CONTEST
*   ***Justification:*** This file is a gap record, not a design (status banner, lines 3-5). Lines 74-78 hand the remedy to `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md`. Alternatives belong in the proposal, and the proposal has no alternatives section (sections: Goal, 1 Lock ownership, 2 Child lifetime, 3 Cancellation, 4 Crash recovery, 5 Interface changes, 6 Tests, 7 Rollout, 8 Non-goals). The reviewers' point is valid for the proposal, not for this file. Separate follow-up: add "Alternatives considered" to the proposal.
*   ***Proposed Edit:*** None in this file.

### [2a] `egm-design` `egm-style` `egm-structural` - Severity: Major - Banner does not say the lease partly landed
*   **Affected Scope:** lines 3-5
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The banner says the requirement is unmet, but lines 80-101 say part of the lease landed. A reader of the banner alone would not know that.
*   ***Proposed Edit:*** Location: line 5, after "...must not be read as one." Add a paragraph: `Partially mitigated: the execution lease is wired for sop run and sop drive behind execution.lease (default off). It is not wired for sop prompt, MCP or ACP. Do not declare release acceptance on the ACP/MCP start-lock test alone.`

### [2b] `egm-design` `egm-style` `egm-structural` - Severity: Major - Plan and gap record disagree; nobody owns the contradiction
*   **Affected Scope:** `PLAN-SOP-ACP-ZED.md` line 4 and matrix row at line 291 (outside TargetFile)
*   ***AI Suggestion:*** ESCALATE
*   ***Justification:*** The plan still says "Implemented and merged" and claims the project lock protects CLI, MCP and ACP equally. Which way to resolve it (amend the row to "known gap" or gate release on closure) is your decision, and the owner is not recorded in any file the reviewers could read. Not fixed here.
*   ***Proposed Edit:*** If you choose "known gap": in the plan, change the row to `Known gap: see ACP-EXECUTION-EXCLUSION-GAP.md` and add a back-pointer under the Status line.

### [3] `egm-design` - Severity: Major - Race windows and blast radius are asserted, not bounded
*   **Affected Scope:** lines 50-60
*   ***AI Suggestion:*** PROPOSE FIX (test status) and ESCALATE (blast radius)
*   ***Justification:*** The evidence list shows no test for either ordering, so the doc should say so. What a double execution actually damages (task state, worktrees, duplicate commits) is not recorded in any readable file; it needs your knowledge.
*   ***Proposed Edit:*** Location: after line 60. Add: `Neither ordering has a deterministic test. Both follow from the code paths in the table above and correspond to proposal tests 1 and 2.` Blast radius: left open until you supply it.

### [4] `egm-design` `egm-structural` - Severity: Major - No Risks section; landed mitigations carry undiscussed risks
*   **Affected Scope:** lines 50-60, 80-101
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** The risks are already in the text (heartbeat failure continues the run; staleness on Windows or cross-host depends on the heartbeat; default off) but are buried in prose. A new Risks section would duplicate the proposal. Put the limits in the interface table of finding [6] and link the proposal's non-goals (section 8) instead of adding a Non-goals section.
*   ***Proposed Edit:*** Covered by finding [6]'s table (a "Limits" column). Add one line under it: `Non-goals: see the proposal, section 8.`

### [5] `egm-design` `egm-structural` `egm-style` - Severity: Major - Evidence and closure criteria missing for the landed work
*   **Affected Scope:** lines 97-110
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** Line 101 refers to "deterministic tests below", but none is listed for the lease work. The evidence list omits the lease code. Test status for the proposal's nine tests is not recorded in this doc.
*   ***Proposed Edit:*** (a) Replace line 101's last sentence with: `Closed when proposal section 6 tests 1 and 2 pass with execution.lease set to required across sop run, sop prompt, MCP and ACP, and test 5 (crash/stale) passes. This document does not record the status of the proposal's tests.` (b) Add to "Evidence to cite": `internal/executionlease/lease.go` (the lease, `state.exec.lock`); `internal/cli/execution_lease.go`, `internal/cli/run.go`, `internal/cli/drive.go` (acquisition); `internal/cli/resume.go` (report and release of a stale lease).

### [6] `egm-structural` `egm-style` - Severity: Major - Status narrative in prose with mixed tenses
*   **Affected Scope:** lines 80-101
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Justification:*** A reader cannot tell what is wired without reading to the end. Overlaps Stage 6 finding 5 and Stage 5 finding 4.
*   ***Proposed Edit:*** Replace lines 86-101 with a table plus one sentence. Table (current behavior, no "now"):

    | Interface | Acquires lease | Checks for a held lease | Limits |
    | --- | --- | --- | --- |
    | `sop run` | when `execution.lease` is `required`; contention recorded under `advisory` | n/a | heartbeat refreshes about every `HeartbeatLimit/5`; a heartbeat failure is reported on stderr and the run continues |
    | `sop drive` | same helper as `sop run` | n/a | same as `sop run` |
    | `sop prompt` | no | no | |
    | MCP `sop_start_work` | no | no | start lock only |
    | ACP `session/prompt` | no | no | start lock only |
    | `sop resume` | no | reports a held lease; releases a stale one in every mode; never a live one | |

    Then: `The setting defaults to off. The lease file is .agent-sdlc/state.exec.lock. An ACP/MCP start cannot see a running sop run, and sop run cannot see an ACP-started child.` Keep lines 80-85 (what the primitive is) trimmed to that one description.

### [7] four reviewer ids - Severity: Nit - "Not a regression" claim unlinked and defensive
*   **Affected Scope:** lines 66-72
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Proposed Edit:*** Retitle `## Baseline behavior before ACP`. Reword the last sentence of the section to: `The earlier design held the metadata lock only until the child launched, so it did not exclude a running sop run either.` The commit link is not available in any file read here; add it if you have it.

### [8] `egm-design` `egm-style` - Severity: Nit - Lock path differs between documents
*   **Affected Scope:** line 25, `PROPOSAL-SHARED-EXECUTION-EXCLUSION.md` line 28
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Proposed Edit:*** Gap doc line 25: `.agent-sdlc/runs/work/.lock` -> `.agent-sdlc/runs/work/.start.lock`. The proposal's `state/runs/work/.lock` at its line 28 is also wrong (file name confirmed in code; the directory was not checked): same file-name change, outside TargetFile, needs your approval.

### [9] four reviewer ids - Severity: Nit - Table notes mostly empty and vague
*   **Affected Scope:** lines 42-48
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Proposed Edit:*** In the table: CLI `sop run` row, "Execution/start lock" `none` -> `execution lease, when execution.lease is not off`. Add a `sop drive` row (same as `sop run`). ACP row Notes: `acpSubmission in internal/cli/acp.go; the metadata lock is tried and released at once`. Child row: `none after start` -> `released at launch`.

### [10] `egm-structural` `egm-style` - Severity: Nit - Redundant disclaimers
*   **Affected Scope:** lines 3-5, 62-65, 97-101
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** For a gap record, the repeated warning is deliberate. Finding [2a] adds one actionable sentence.

### [11] `egm-structural` - Severity: Nit - Metadata block has no Date
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** The date the gap was recorded is in git history. Not worth a hand-written date that will drift.

### [12] `egm-structural` - Severity: Nit - Evidence list reference style
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** Pinning a revision adds maintenance. Finding [5] adds the missing entries in the existing style.

### [13] `egm-style` - Severity: Nit - Lock terminology inconsistent
*   **Affected Scope:** line 39
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Proposed Edit:*** Line 39: `The project writer lock` -> `The project metadata lock`, matching lines 42 and 69.

### [14] `egm-style` - Severity: Nit - Requirement quote splices two sources
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** The plan line numbers were not checked here, so a rewrite could add errors.

### [15] `egm-structural` `egm-style` - Severity: Nit - "Requirement" heading and hedged wording
*   ***AI Suggestion:*** DECLINE
*   ***Justification:*** Style preference. Finding [3] states the test status directly.

### [16] `egm-style` - Severity: Nit - Lease path and heartbeat operator guidance missing
*   **Affected Scope:** lines 76-78, 88-90
*   ***AI Suggestion:*** PROPOSE FIX (path only)
*   ***Proposed Edit:*** The lease path goes in finding [6]'s closing sentence. The heartbeat operator action is declined: it belongs in the operations docs.

### [17] `egm-style` - Severity: Nit - Italic emphasis
*   **Affected Scope:** line 110
*   ***AI Suggestion:*** PROPOSE FIX
*   ***Proposed Edit:*** `that *is* covered` -> `that is covered`.
