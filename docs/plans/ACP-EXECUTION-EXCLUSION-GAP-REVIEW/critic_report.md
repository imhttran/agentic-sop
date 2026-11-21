# Goldfish Review Report

**Target:** `/Users/imhttran/agentic-workspace/agentic-sop/docs/plans/ACP-EXECUTION-EXCLUSION-GAP.md` (`lines 1-110`)

Finding ids are `<reviewer>-1-<n>`, where n is the reviewer's own numbering. Overlapping findings are merged and list every source id. No reviewer opened the Go source, so all claims about code, tests and behavior are unverified.

## Reviewer Status

| Reviewer | Status |
| --- | --- |
| `egm-design` | completed |
| `egm-structural` | completed |
| `egm-style` | completed |

Severity breakdown: 0 Blocker, 6 Major, 12 Nit.

## Prioritized Findings (Blockers -> Major -> Nits)

No Blockers.

### Alternatives to the execution lease are not evaluated - Severity: Major
* **Finding ids:** `egm-design-1-1`, `egm-structural-1-1`
* **Reviewer Lens:** `egm-design`, `egm-structural`
* **Affected Scope:** `lines 74-101` ("What would close it")
* **Discrepancy Description**: The document names one remedy, the project-scoped execution lease, and records no rejected alternatives. Candidates include making `sop run` and `sop prompt` consult the started-work records and start lock, holding the metadata lock for the execution lifetime, and a pid-file or supervisor. The proposal has no alternatives section either.
* **Rationale & Recommendation**: The standard's Alternatives rule requires documented rejections. Add an "Alternatives considered" section, or link one, with concrete technical or scope reasons. Do not attribute a rejection to a maintainer without a link.

### Status conflict between the gap record, the plan and the banner - Severity: Major
* **Finding ids:** `egm-design-1-2`, `egm-style-1-3`, `egm-structural-1-11`
* **Reviewer Lens:** `egm-design`, `egm-style`, `egm-structural`
* **Affected Scope:** `lines 3-5`, `62-64`, `97-101`, plus the PLAN-SOP-ACP-ZED.md header and acceptance matrix
* **Discrepancy Description**: The plan says "Implemented and merged in PR #197" and its matrix row (about line 291) still claims concurrent starts are protected equally. The gap record says release acceptance must not be declared. Nobody is named as owner of the contradiction, and the plan's definition of done depends on the matrix. The banner says the requirement is unmet but does not say that the lease partly landed behind `execution.lease` (default off). The gap record does not cite the proposal's step numbers, so "steps 1 and 2 landed" cannot be mapped to it.
* **Rationale & Recommendation**: Name an owner and a decision: amend the plan row to "known gap", or gate release on closure. Add a back-pointer from the plan. Extend the banner with one line such as "Partially mitigated behind `execution.lease` (default off); open for `sop prompt`, MCP and ACP". Cite the proposal's step numbers.

### Race windows and blast radius are asserted, not bounded - Severity: Major
* **Finding ids:** `egm-design-1-3`
* **Reviewer Lens:** `egm-design`
* **Affected Scope:** `lines 50-57` (consequences 1 and 2)
* **Discrepancy Description**: "Between task transitions it can start a second execution" gives no window estimate. The text does not say whether concurrent executions corrupt task state or evidence, nor whether either ordering was reproduced.
* **Rationale & Recommendation**: The gap's severity cannot be judged. Add a reproduction or deterministic test for both orderings and state the blast radius (double-run, worktree collision, duplicate commits).

### Landed mitigations carry undiscussed risks, and there is no Risks section - Severity: Major
* **Finding ids:** `egm-design-1-4`, `egm-structural-1-2`
* **Reviewer Lens:** `egm-design`, `egm-structural`
* **Affected Scope:** `lines 50-60`, `80-101`, whole document
* **Discrepancy Description**: There is no Risks and mitigations section and no Non-goals section (non-goals live only in proposal section 8). Heartbeat failure is reported and ignored. Staleness on Windows or another host depends on the heartbeat alone, so clock skew or a stalled write could release a live lease. The claim that `sop resume` "never releases a live lease" states no mechanism. A lease can be orphaned by SIGKILL. The default `off` leaves everything unprotected.
* **Rationale & Recommendation**: Add `## Risks and mitigations` covering false-stale release, clock skew, cross-host behavior and orphaned leases, citing the test for each. Add a short Non-goals section or link the proposal's.

### Evidence and closure criteria are missing for the landed work - Severity: Major
* **Finding ids:** `egm-design-1-5`, `egm-structural-1-4`, `egm-style-1-2`
* **Reviewer Lens:** `egm-design`, `egm-structural`, `egm-style`
* **Affected Scope:** `lines 80-101`, `103-110`
* **Discrepancy Description**: Line 101 says the gap closes when wiring is "evidenced by the deterministic tests below", but the evidence list names only the pre-lease code and the one ACP/MCP test. `internal/executionlease`, the `sop run` acquisition, the heartbeat and resume-release have no entry. The proposal lists the crash/stale case as a test to be written, yet the document presents it as working. None of the proposal's nine tests carry a status.
* **Rationale & Recommendation**: List the required tests (proposal section 6, items 1, 2, 5 and 6 at minimum) with pass or pending status. State closure criteria, for example "closed when tests 1 and 2 pass with the lease set to required". Add the lease package to the evidence list, or mark each landed claim "not independently verified here".

### Status narrative in prose with mixed tenses - Severity: Major
* **Finding ids:** `egm-structural-1-3`, `egm-style-1-1`
* **Reviewer Lens:** `egm-structural`, `egm-style`
* **Affected Scope:** `lines 80-101`
* **Discrepancy Description**: "now exists", "now wired" and "is now detectable" are changelog words relative to an unseen date. Paragraphs at lines 86-96 mix the heartbeat, `sop resume` and the failure mode. A reader cannot tell what is done and what remains without reading to the end.
* **Rationale & Recommendation**: State current behavior. Replace the prose with a table or checklist by interface (`sop run`, `sop prompt`, MCP, ACP, `sop resume`) showing wired or not wired and the default, matching the table style at lines 42-48.

### Earlier-design "not a regression" claim is unlinked and defensive - Severity: Nit
* **Finding ids:** `egm-design-1-6`, `egm-structural-1-9`, `egm-style-1-6`, `egm-style-1-5`
* **Reviewer Lens:** `egm-design`, `egm-structural`, `egm-style`
* **Affected Scope:** `lines 66-72`
* **Discrepancy Description**: The earlier bridge design is not tied to a commit or PR. "that lock" is ambiguous, and "never excluded" is stated as fact without a citation. The section rebuts a charge nobody made.
* **Rationale & Recommendation**: Link the commit or PR. Reword to "The earlier design held the metadata lock only until the child launched, so it did not exclude a running `sop run`". Consider retitling it "Baseline behavior before ACP".

### Lock path differs between documents - Severity: Nit
* **Finding ids:** `egm-design-1-7`, `egm-style-1-13`
* **Reviewer Lens:** `egm-design`, `egm-style`
* **Affected Scope:** `line 25`, `line 40`, and the proposal
* **Discrepancy Description**: The gap record gives the start lock as `.agent-sdlc/runs/work/.lock` and the proposal gives `state/runs/work/.lock`.
* **Rationale & Recommendation**: Confirm the real path and use one everywhere.

### Table notes column is mostly empty and vague - Severity: Nit
* **Finding ids:** `egm-design-1-8`, `egm-structural-1-6`, `egm-style-1-7`, `egm-style-1-8`
* **Reviewer Lens:** `egm-design`, `egm-structural`, `egm-style`
* **Affected Scope:** `lines 42-49`
* **Discrepancy Description**: Most Notes cells are blank. "probe only" and "see `internal/cli/acp.go`" are vague. "none after start" in the child row is ambiguous against line 49.
* **Rationale & Recommendation**: Fill the cells or drop the column. Name `acpSubmission` and define "probe". Use "released at launch" consistently.

### Redundant disclaimers - Severity: Nit
* **Finding ids:** `egm-structural-1-8`, `egm-style-1-4`, `egm-style-1-5`
* **Reviewer Lens:** `egm-structural`, `egm-style`
* **Affected Scope:** `lines 3-5`, `62-65`, `97-101`
* **Discrepancy Description**: "OPEN" and "unresolved" repeat each other, and the "not evidence of completion" warning appears several times.
* **Rationale & Recommendation**: Keep one actionable sentence, for example "Do not declare release acceptance on this test alone."

### Metadata block is missing a Date - Severity: Nit
* **Finding ids:** `egm-structural-1-5`
* **Reviewer Lens:** `egm-structural`
* **Affected Scope:** `lines 3-9`
* **Discrepancy Description**: Status and Plan are plain paragraphs and there is no `Date`, unlike the sibling documents.
* **Rationale & Recommendation**: Use a `Status:` and `Date:` block.

### Inconsistent reference style in the evidence list - Severity: Nit
* **Finding ids:** `egm-structural-1-7`
* **Reviewer Lens:** `egm-structural`
* **Affected Scope:** `lines 105-110`
* **Discrepancy Description**: The plan and proposal use links but the evidence entries are bare paths, with no revision pin or line anchors.
* **Rationale & Recommendation**: Pick one convention and pin a revision.

### Lock terminology is inconsistent - Severity: Nit
* **Finding ids:** `egm-style-1-12`
* **Reviewer Lens:** `egm-style`
* **Affected Scope:** `line 39` versus `42`, `69`
* **Discrepancy Description**: "project writer lock", "project metadata lock", "project lock" and "start lock" are used loosely.
* **Rationale & Recommendation**: Define each term once and use it consistently.

### Requirement quote splices two sources - Severity: Nit
* **Finding ids:** `egm-style-1-14`
* **Reviewer Lens:** `egm-style`
* **Affected Scope:** `lines 14-17`
* **Discrepancy Description**: The ellipsis joins plan line 18, the ACP-003 criteria (plan line 106) and the matrix row without marking the sources.
* **Rationale & Recommendation**: Present two quotes with their line references.

### "Requirement" heading and hedged consequences wording - Severity: Nit
* **Finding ids:** `egm-structural-1-10`, `egm-style-1-9`
* **Reviewer Lens:** `egm-structural`, `egm-style`
* **Affected Scope:** `lines 11`, `51-57`
* **Discrepancy Description**: The "Requirement" heading serves as the problem statement. "can start" and "can begin" hedge without saying whether they were observed, and line 52-53 is wordy.
* **Rationale & Recommendation**: Consider renaming the heading to match the standard. State whether each scenario is observed or untested and tighten to "is excluded only when it observes a mid-run task state".

### Lease path and heartbeat operator guidance missing - Severity: Nit
* **Finding ids:** `egm-style-1-10`, `egm-style-1-11`
* **Reviewer Lens:** `egm-style`
* **Affected Scope:** `lines 76-78`, `88-90`
* **Discrepancy Description**: The lease path `.agent-sdlc/state.exec.lock` is not given. Heartbeat failure is reported on stderr with no operator action stated.
* **Rationale & Recommendation**: Add the path. State the operator action or move the note to the operations docs.

### Italic emphasis - Severity: Nit
* **Finding ids:** `egm-style-1-15`
* **Reviewer Lens:** `egm-style`
* **Affected Scope:** `lines 109-110`
* **Discrepancy Description**: "that *is* covered" uses italics for emphasis.
* **Rationale & Recommendation**: Write "covered".
