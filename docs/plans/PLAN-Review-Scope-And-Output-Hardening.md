# PLAN --- Review Scope & Malformed-Output Hardening

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** completed read-only EV-003 approval-boundary diagnostic (2026-10-08)\
**Depends on (evidence, not a task):** EV-003 failure, `docs/reports/ev-context-run/EV-003-retrieval-evaluation.md`

## Project

Review Scope & Malformed-Output Hardening

## Summary

Correct two defects in SOP's self-review seam that the EV-003 diagnostic proved, without
weakening any quality gate:

1. **Review-scope contamination** --- the review input is built from the *whole* working
   tree (`readDiff` = `DiffAll` minus SOP's own dirs, plus declared deliverables), so
   unrelated pre-existing uncommitted files enter every task's review and produce
   spurious findings.
2. **Malformed review-output handling** --- a single finding with an invalid/empty
   severity aborts the *entire* review (`parseReport`), which is then classified as
   `UNKNOWN` → `NEEDS_HUMAN`; a genuine high-severity finding in the same response is
   discarded, and a deterministic FAIL becomes an unclassifiable human boundary.

The change is provider/model-neutral, changes no approval authority, relaxes no quality
acceptance, and adds no budget or routing authority.

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `agentic-sop` builds and tests with the repository Go toolchain; the
  `internal/cli`, `internal/review`, `internal/git`, and `internal/planflow` suites
  exercise the review seam and the lifecycle with fakes.
- Owner: operator-supplied development environment
- Location: Go executable available on PATH

### Existing review / change-attribution facilities — EXISTS

- Evidence: `internal/review` (`parseReport`, `Report.Blocking`), `internal/git`
  (`Diff`, `DiffAll`, `DiffFiles`, `Untracked`), and the invocation-scoped change
  attribution in `internal/cli` (`invocationChanges`, `recordTaskChanges`,
  `rn.ChangedFiles()`) already exist and are tested.
- Owner: `agentic-sop`
- Location: `internal/review`, `internal/git`, `internal/cli`

## Goal

Make the review see exactly the task's own change set, and make a malformed review
response a deterministic, bounded outcome that never discards a genuine finding and
never becomes a PASS.

## Non-Goals

- Do not change the allowed severity set (`INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`).
- Do not lower any severity, weaken `Report.Blocking`, or relax `quality.Evaluate`.
- Do not change the human approval boundary, autonomy policy, or approval authority.
- Do not add or change any iteration, stale, tool-call, or retry budget.
- Do not change provider/model selection or routing.
- Do not introduce provider-, model-, or domain-specific behavior.
- Do not add third-party dependencies.
- Do not reopen or modify Phase 8 / POST8-001, the convergence archive, or the
  EV-CONTEXT-RUN plan and its EV-001..EV-003 evidence.
- Do not implement the E2 retrieval seam.

## Architecture Invariant

The change is confined to the review seam and its inputs. Policy is unchanged: the
model still only proposes findings; the harness still owns classification, severity
thresholds, and the human boundary.

```text
task change set (invocation-scoped)        declared deliverables
        |                                        |
        +------------------+---------------------+
                           v
              review input = exact task change set   <-- RSH-002 (scope)
                           |
                           v
                 REVIEW (model) -> parseReport       <-- RSH-003 (output handling)
                           |
                           v
              quality.Evaluate -> failure.Classify -> autonomy.Decide
                           |
                           v
              fix loop | NEEDS_HUMAN (unchanged boundary)
```

The invariant must hold identically for every provider and model, and must not name any
provider, model, or task domain.

## Root Causes (authoritative, from the EV-003 diagnostic)

- **Scope contamination.** `internal/cli/cli.go` `readDiff` =
  `git.New(dir).DiffAll(ctx, config.DirName, planflow.ReportsDir)` includes every
  working-tree change except SOP's own dirs. `runStages` (`internal/cli/run.go:457-467`)
  adds the task's declared report files via `DiffFiles`. The EV-003 review diff
  (`diff.patch`) therefore contained the pre-existing `docs/specs/AGENT-PROVIDER.md` and
  `docs/plans/PLAN-EV-Context-Run-Evaluation.md`, producing findings 3 and 4 that were
  **not** authored by the task (`changed-files.json` = report + test only).
- **Whole-review abort on one malformed finding.** `internal/review/provider.go:73-75`
  returns an error if *any* finding severity is invalid, discarding the entire report.
  The EV-003 final review emitted a finding with an empty severity, so the review failed
  and `runScheduledTask` (`internal/cli/drive.go:736-744`) classified it
  `UNKNOWN → NEEDS_HUMAN`. A genuine `HIGH` in the same response would have been lost.
- **Stale findings.** The review that failed the gate predated the fixes (the harness
  was added after the review), so its `HIGH` findings (1 and 2) were valid when emitted
  but obsolete against the final artifacts. The re-review that should have cleared them
  instead aborted (above).

## RSH-001 --- Baseline: Review Seam and Change Attribution

Record the exact current review-scope construction, the review call site, the
severity-rejection path, and the error-to-classification path, and anchor them with the
EV-003 evidence.

### Authoritative Inputs

- `internal/cli/cli.go` (`readDiff`), `internal/cli/run.go` (`runStages` review block),
  `internal/cli/mutation.go` (`invocationChanges`), `internal/cli/jev.go`
  (`recordTaskChanges`, `taskChangeEvidence`)
- `internal/review/provider.go` (`parseReport`), `internal/review/review.go`
  (`Severity.Valid`, `Report.Blocking`)
- `internal/cli/drive.go` (error classification), `internal/failure` (`Classify`)
- `internal/git/git.go` (`Diff`, `DiffAll`, `DiffFiles`, `Untracked`)
- `.agent-sdlc/runs/EV-003/{diff.patch,review.json,gate.json,classification.json,changed-files.json}`

### Mutation Targets

- `docs/reports/review-hardening/RSH-001-review-seam-baseline.md` --- the only file this
  task writes; no production change.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/review-hardening/RSH-001-review-seam-baseline.md` --- the exact current
  review-scope construction, the review call site (`run.go:778`), the severity-rejection
  path (`provider.go:73-75`), the error-to-classification path (`drive.go:736-744`), and
  the EV-003 evidence anchors (contaminating files, empty-severity abort).

### Acceptance Criteria

- The review diff construction is recorded with file/line anchors.
- The whole-review-abort behavior and the error→`NEEDS_HUMAN` path are recorded with
  file/line anchors.
- The EV-003 contaminating-file evidence and the malformed-severity abort are cited.
- No production change is made.

### Execution Contract

1. Read `run.go` (the review block and the `readDiff` wrapper), then `provider.go`.
2. Read `mutation.go`, `jev.go`, `git.go`, and `drive.go`.
3. Produce the baseline report.
4. Finish.

Expected first action: read `internal/cli/run.go`.

### Production-Change Scope

None

## RSH-002 --- Invocation-Scoped Review Diff

Build the review input from the task's own change set: the invocation-scoped changed
files (already computed by `invocationChanges` and recorded by `recordTaskChanges` in
`rn.ChangedFiles()`) plus the declared task deliverables --- never the whole working
tree.

### Authoritative Inputs

- `internal/cli/mutation.go` (`invocationChanges`, `snapshotRepository`)
- `internal/cli/jev.go` (`recordTaskChanges`, `taskChangeEvidence`, `taskChangedFiles`)
- `internal/git/git.go` (`Diff`, `DiffFiles`, `Untracked`)
- `internal/cli/report_deliverable.go` (`taskReportDeliverables`)

### Mutation Targets

- The review-input construction in the review seam (a scoped diff built from the task's
  recorded paths plus declared deliverables), and the git helper it uses.
- Focused tests for the scoped diff.

### Dependencies

- RSH-001

### Requires

- Go build/test toolchain

### Deliverables

- A scoped review diff: only the task's invocation-scoped changed files and declared
  deliverables, with the untracked portion filtered to the same path set.
- Focused tests for tracked, untracked, deleted, and renamed files, and for the
  fail-closed path.
- An update to the review-scope documentation (`docs/specs/REVIEW.md`).

### Acceptance Criteria

- The review input excludes unrelated pre-existing working-tree modifications.
- A mutation made *during* the task (tracked, untracked, deleted, or renamed) remains in
  scope and is reviewed.
- A task-owned untracked deliverable is included.
- If the task change set cannot be established (for example the tree is dirty but no
  task-owned change is attributable), the seam fails closed rather than reviewing the
  whole tree.
- Provider/model neutrality holds and no new dependency is added.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read RSH-001's baseline and the change-attribution helpers.
2. Implement the scoped review diff, reusing `rn.ChangedFiles()`/`DiffFiles`.
3. Add the focused tests.
4. Update `docs/specs/REVIEW.md`.
5. Finish.

Expected first action: read `docs/reports/review-hardening/RSH-001-review-seam-baseline.md`.

### Production-Change Scope

Review input only. No gate threshold, severity, approval, budget, or routing change.

## RSH-003 --- Malformed Review-Output Handling

Make an invalid review response a deterministic, bounded outcome: retain every valid
finding (so a genuine `HIGH`/`CRITICAL` is never discarded), reject malformed severity
values explicitly, and never turn malformed output into a PASS.

### Authoritative Inputs

- `internal/review/provider.go` (`parseReport`, `reviewOutputSchema`)
- `internal/review/review.go` (`Severity`, `Report.Blocking`)
- `internal/cli/drive.go` (error classification), `internal/failure` (`Classify`,
  `Disposition`, `Retryable`)
- `internal/cli/selection.go` / autonomy (`decideAutonomy`)

### Mutation Targets

- The review-output parsing boundary (`internal/review`) and, if needed, the
  error-to-classification path, so an invalid review response is a bounded, classified
  outcome rather than a discarded whole report.
- Focused tests.

### Dependencies

- RSH-001

### Requires

- Go build/test toolchain

### Deliverables

- Per-finding parsing: every finding with a valid severity is retained and blocks at its
  severity; an invalid/empty severity is never silently dropped.
- A bounded handling for invalid review output: retry the REVIEW capability up to a small
  fixed bound, then classify `NEEDS_HUMAN` with the raw response preserved --- never a
  PASS.
- Focused tests and a `docs/specs/REVIEW.md` update describing the boundary.

### Acceptance Criteria

- The allowed severity set is unchanged (`INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`).
- An invalid severity is rejected deterministically and never silently ignored.
- A valid `HIGH` or `CRITICAL` finding in a response that also contains a malformed
  finding still blocks.
- Malformed review output never yields `quality.Pass` and never a silent fix-loop exit.
- The human approval boundary is unchanged.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read RSH-001's baseline and `parseReport`.
2. Implement per-finding parsing and the bounded malformed-output handling.
3. Add the focused tests.
4. Update `docs/specs/REVIEW.md`.
5. Finish.

Expected first action: read `internal/review/provider.go`.

### Production-Change Scope

Review parsing/output handling only. No approval-authority, severity-threshold, or
quality-acceptance change.

## RSH-004 --- Regression Suite

Add deterministic regressions for the review-scope and malformed-output behaviors.

### Authoritative Inputs

- The RSH-002 and RSH-003 changes.
- Existing test patterns: `internal/cli/*_test.go`, `internal/review/*_test.go`,
  `internal/ollamaagent/convergence_regression_test.go` (scripted-fake precedent).

### Mutation Targets

- New/extended test files under `internal/cli` and `internal/review`.

### Dependencies

- RSH-002
- RSH-003

### Requires

- Go build/test toolchain

### Deliverables

- A regression covering each required scenario (see Acceptance Criteria).

### Acceptance Criteria

- **Pre-existing unrelated dirty files** are excluded from the review input.
- **Task-owned untracked deliverables** are included.
- **Unauthorized task mutations** (tracked, untracked, deleted, renamed) are in scope and
  reviewed.
- **Review findings produced before and after fix cycles** are both handled (the scope
  is recomputed per iteration against the accumulated task change set).
- **Empty and invalid severity** are rejected deterministically and never dropped.
- **Repeated malformed responses** terminate within the bounded handling, not
  unbounded, and not as a PASS.
- **Valid HIGH/CRITICAL findings** always block.
- **Retry exhaustion and human escalation** behave as today (fail-closed boundary).
- **Runtime scoped-review wiring**: an integration test proves the actual `provider.Review`
  call path receives the *scoped* diff (not `DiffAll`), and that the seam performs **no
  provider call** when a required change cannot be attributed (fail closed on a non-empty
  but unattributable tree).
- **Partial-attribution and silent exclusion**: a mutation whose path is *not* present in
  the recorded task change set (`rn.ChangedFiles()`) is either detected as in scope or the
  seam fails closed; an unauthorized task-owned mutation is never silently excluded from
  review.
- **Unauthorized mutation coverage** (tracked, untracked, deleted, renamed) is exercised as
  both an in-scope detection case and a fail-closed case.
- The scenarios use only fakes; no real provider, network, or Clef dependency.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read the RSH-002 and RSH-003 changes.
2. Add one focused test per scenario.
3. Finish.

Expected first action: read the RSH-002 test file.

### Production-Change Scope

None (test-only)

## RSH-005 --- Verification, Neutrality, and Documentation

Verify the change against the repository gates, the neutrality guard, and the lifecycle
invariants, and record the evidence.

### Authoritative Inputs

- The RSH-002..RSH-004 changes.
- `internal/archtest` (provider/model-neutrality guard).
- `docs/specs/REVIEW.md`, `docs/specs/AGENT-PROVIDER.md`.

### Mutation Targets

- `docs/reports/review-hardening/RSH-005-verification.md` --- the verification report.

### Dependencies

- RSH-004

### Requires

- Go build/test toolchain

### Deliverables

- `docs/reports/review-hardening/RSH-005-verification.md` --- the focused regressions, the
  full suite, the neutrality guard, and the safety/lifecycle checks, all recorded.

### Acceptance Criteria

- `go build ./...`, `go test ./...`, `go test -race ./...`, and `go vet ./...` pass.
- The provider/model-neutrality architecture test passes; no provider/model/domain name
  or branch is added.
- **Neutrality coverage is extended to the review and change-attribution seam in
  `internal/cli`** (which the existing `internal/archtest` guard does not cover): a check
  proves RSH-002..RSH-004 introduce no provider-, model-, or domain-specific behavior.
- Approval boundaries, fail-closed behavior, and lifecycle transitions are unchanged; no
  budget is changed.
- Documentation links pass; the specs describe the scoped review and the malformed-output
  boundary.
- No Phase 8 / POST8-001 / EV evidence is modified.

### Execution Contract

1. Read the RSH-002..RSH-004 changes.
2. Run the focused regressions and the full gates.
3. Produce the verification report.
4. Finish.

Expected first action: run the focused `internal/review` and `internal/cli` tests.

### Production-Change Scope

None

## Regression Strategy

| #   | Scenario                                            | Expected                                                                   |
| --- | --------------------------------------------------- | -------------------------------------------------------------------------- |
| 1   | Pre-existing unrelated dirty files                  | excluded from the review input                                             |
| 2   | Task-owned untracked deliverable                    | included and reviewed                                                      |
| 3   | Unauthorized task mutation during the task          | observed and in scope                                                      |
| 4   | Findings before and after a fix cycle               | scope recomputed per iteration                                             |
| 5   | Empty and invalid severity                          | rejected deterministically, retained raw                                   |
| 6   | Repeated malformed responses                        | bounded termination, never a PASS                                          |
| 7   | Valid HIGH / CRITICAL finding                       | blocks at its severity                                                     |
| 8   | Retry exhaustion / human escalation                 | unchanged fail-closed boundary                                             |
| 9   | Runtime review seam (fake provider)                 | `provider.Review` receives the scoped diff; no call when attribution fails |
| 10  | Partial attribution / path outside the recorded set | detected as in scope or fails closed; never silently excluded              |
| 11  | Review-seam neutrality (`internal/cli`)             | no provider/model/domain name or branch introduced                         |

## Lifecycle Safety

The hardening plan is authored now but **must not be activated** while EV-CONTEXT-RUN
is the active plan. The supported lifecycle facts (`internal/planflow/planflow.go`,
`internal/cli/plan.go`):

- The active plan `plan-ev-context-run-evaluation` has **unresolved work** (EV-003
  `BLOCKED`; EV-004/EV-005 `PLANNED`), so `domain.AllSatisfied` is false.
- `sop plan activate <hardening>` with a *different* source while the active plan is
  unresolved is refused with `differentPlanError` (`planflow.go:248-251`). A plain
  `sop run <hardening>` is refused the same way.
- `sop plan complete` is refused while the active plan has unresolved work.
- The only supported transition that installs a different plan with unresolved active
  work is `sop plan supersede`, which archives the active plan as **`SUPERSEDED`** ---
  that is **incorrect** for EV-CONTEXT-RUN and must not be used.

### Exact safe lifecycle transition

```text
1. Human resolves the EV-003 approval gate (see "How EV-003 Is Resumed").
2. sop run docs/plans/PLAN-EV-Context-Run-Evaluation.md
       -> EV-003 retries; then EV-004, EV-005 execute in dependency order.
3. When all EV tasks are satisfied:
       sop plan complete
       (archives EV-CONTEXT-RUN as COMPLETE, releases the active association)
   -- OR skip to 4; activating the hardening plan performs the same handoff.
4. sop plan activate docs/plans/PLAN-Review-Scope-And-Output-Hardening.md
       (reconcileState: active plan satisfied -> handOff archives EV as COMPLETE,
        then installs the hardening plan as ACTIVE)
5. sop run docs/plans/PLAN-Review-Scope-And-Output-Hardening.md
```

Activating the hardening plan is therefore **not** assumed safe; it is only safe once
EV-CONTEXT-RUN is satisfied. Until then, the hardening plan stays `PROPOSED` and inert.

### How EV-003 Is Resumed

- The pending approval is resolved with the supported approval operation:
  `sop approve EV-003` (or `sop approve EV-003 --run`). Approving a `BLOCKED` task
  resumes it through the domain's requeue operation **without spending a retry**
  (`internal/approval/approval.go:285-303`), returning EV-003 to `PLANNED`.
- `sop decline EV-003` preserves the truthful state and leaves the task `BLOCKED`
  unchanged; it does not progress EV and is not a route to completion.
- After approval, `sop run` re-runs EV-003. Because the scope/malformed-output defects
  are not yet fixed, EV-003 may block again; the human may re-inspect at the boundary.
  Committing the pre-existing user-owned changes (removing them from the working tree)
  is an operator action that reduces scope contamination for that retry, but is not part
  of this plan.

## Safety Invariants

This plan must not:

- change any gate threshold, severity meaning, or `quality.Evaluate` acceptance;
- bypass, weaken, or auto-resolve a human approval boundary;
- silently discard a valid finding or turn malformed output into a PASS;
- change commit/push authorization or provider/model selection;
- add or change any iteration, stale, tool-call, or retry budget;
- add third-party dependencies or name a provider, model, or task domain;
- modify Phase 8 / POST8-001, the convergence archive, or EV-001..EV-003 evidence.

## Unresolved Architecture Decision

RSH-003 chooses between two bounded handlings for invalid review output:

- **A. Bounded retry then `NEEDS_HUMAN`.** Re-request the REVIEW capability up to a small
  fixed bound; if still invalid, classify `NEEDS_HUMAN` with the raw response preserved.
- **B. Deterministic classification without retry.** Classify invalid output directly as
  a `NEEDS_HUMAN` disposition on the first occurrence, with the raw response preserved.

Both retain valid findings, reject malformed severities, and never PASS.

**Resolution (operator, 2026-10-08):** handle invalid review output with **Option A** ---
bounded retry of the REVIEW capability, then `NEEDS_HUMAN` with the raw response preserved ---
consistent with RSH-003's compiled deliverable and acceptance criteria. Option B is not used.

## Dependency Graph

```text
RSH-001 (review-seam baseline, read-only)
    |
    +-------------------+
    |                   |
RSH-002 (scoped diff)   RSH-003 (malformed-output handling)
    |                   |
    +---------+---------+
              |
        RSH-004 (regression suite)
              |
        RSH-005 (verification + neutrality + docs)
```
