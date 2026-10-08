# PLAN --- RSH Follow-Up Hardening

**Repository:** `~/agentic-workspace/agentic-sop`\
**Scope:** `agentic-sop` only\
**Status:** PROPOSED (not activated; not implemented)\
**Basis:** the three residual governance findings recorded at Review-Governance-Hardening closure
(2026-10-08)\
**Depends on (evidence, not a task):** `docs/reports/review-hardening/RSH-005-verification.md`,
`docs/reports/review-hardening/RSH-001-review-seam-baseline.md`

## Project

RSH Follow-Up Hardening

## Summary

Close the three residual governance findings left open when the Review Scope & Output Hardening
plan was archived `COMPLETE`, without weakening any gate or approval boundary:

1. **Implicit malformed-output classification.** The run-level caller relies on the generic
   fail-closed fallback to reach `NEEDS_HUMAN` for a malformed review response; it does not check
   `review.IsMalformedOutput(error)` explicitly. Make the classification explicit and
   deterministic while preserving fail-closed behavior.
2. **Neutrality coverage gap.** The reviewer-neutrality guard covers a fixed, hard-coded file list
   that omits `internal/cli/run.go` and cannot detect a future review integration point. Extend
   the coverage to `run.go` with a maintainable, self-checking scope definition.
3. **Editor-only Markdown formatting conflicts.** An editor that rewrites emphasis markers on save
   (`*x*` to `_x_`) changes an executed task's acceptance text and manufactures a false
   `changed (executed)` reconcile conflict. Treat pure emphasis-marker variance as cosmetic
   without weakening substantive change detection.

The change is provider/model-neutral, changes no approval authority, relaxes no quality
acceptance, and adds no budget or routing authority.

## Capabilities

### Go build/test toolchain — EXISTS

- Evidence: `agentic-sop` builds and tests with the repository Go toolchain; the
  `internal/cli`, `internal/review`, `internal/archtest`, and `internal/planflow` suites
  exercise the review seam, the neutrality guard, and the reconcile comparison with fakes.
- Owner: operator-supplied development environment
- Location: Go executable available on PATH

### Existing review, neutrality, and reconcile facilities — EXISTS

- Evidence: `internal/review` (`IsMalformedOutput`, `ErrMalformedOutput`), `internal/cli/run.go`
  (the review seam), `internal/archtest` (the provider/model-neutrality guard), and
  `internal/planflow` (`compare.go` canonical comparisons, `reconcileState`) already exist and
  are tested.
- Owner: `agentic-sop`
- Location: `internal/review`, `internal/cli`, `internal/archtest`, `internal/planflow`

## Goal

Make the malformed-output classification explicit at the run-level caller, extend the
neutrality guard to `run.go` with a maintainable scope, and make pure Markdown emphasis-marker
variance cosmetic in the reconcile comparison — all without weakening any gate, approval, or
substantive-change detection.

## Non-Goals

- Do not change the allowed severity set (`INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`).
- Do not lower any severity, weaken `Report.Blocking`, or relax `quality.Evaluate`.
- Do not change the human approval boundary, autonomy policy, or approval authority.
- Do not add or change any iteration, stale, tool-call, or retry budget.
- Do not change provider/model selection or routing.
- Do not introduce provider-, model-, or domain-specific behavior.
- Do not add third-party dependencies.
- Do not reopen or modify Phase 8 / POST8-001, the convergence archive, the EV-CONTEXT-RUN plan,
  or the RSH plan and its RSH-001..RSH-005 evidence.
- Do not implement the E2 retrieval seam.

## H001 --- Explicit Malformed-Output Classification at the Run-Level Caller

Make the run-level review caller classify a malformed review response by checking
`review.IsMalformedOutput(error)` explicitly and mapping it deterministically to the existing
human boundary, instead of relying on the generic fail-closed fallback alone.

### Authoritative Inputs

- `internal/cli/run.go` (the review seam: the `provider.Review` call and its error return)
- `internal/review/provider.go` (`IsMalformedOutput`, `ErrMalformedOutput`)
- `internal/review/loop.go` (`Loop.Run` malformed-output propagation)
- `internal/cli/drive.go` (the error-to-classification path)
- `internal/failure` (`Classify`, `Disposition`, `NeedsHuman`)

### Mutation Targets

- The review-error handling in `internal/cli/run.go`.
- Focused tests under `internal/cli`.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- The run-level review caller detects the malformed-output outcome explicitly with
  `review.IsMalformedOutput` and routes it to the existing `NEEDS_HUMAN` boundary, so the
  classification does not depend on the generic fallback.
- Focused tests proving a malformed review response yields `NEEDS_HUMAN` and never a `PASS`,
  and that a retained `HIGH`/`CRITICAL` finding still blocks.

### Acceptance Criteria

- A malformed review response is classified `NEEDS_HUMAN` through an explicit
  `review.IsMalformedOutput(err)` check at the run-level caller, not only the generic
  fail-closed fallback.
- Fail-closed behavior is preserved: no malformed output yields `quality.Pass`; a retained
  `HIGH` or `CRITICAL` finding still blocks at its severity.
- The human approval boundary, the severity thresholds, and `quality.Evaluate` acceptance are
  unchanged.
- Provider/model neutrality holds and no new dependency is added.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read the review seam in `internal/cli/run.go` and the classification path in `drive.go`.
2. Implement the explicit `review.IsMalformedOutput` check at the run-level caller.
3. Add the focused tests.
4. Finish.

Expected first action: read `internal/cli/run.go`.

### Production-Change Scope

Review-error classification only. No approval-authority, severity-threshold, or
quality-acceptance change.

## H002 --- Maintainable Reviewer-Neutrality Scope

Extend the reviewer-neutrality regression guard to `internal/cli/run.go` and every future review
integration point, replacing the fixed hard-coded file list with a maintainable, self-checking
scope definition that cannot silently omit a seam file.

### Authoritative Inputs

- `internal/archtest/review_seam_neutrality_test.go` (`reviewSeamFiles`, `providerModelTokens`,
  `TestReviewSeamStaysProviderAndModelNeutral`, `TestReviewSeamNeutralityFilesExist`)
- `internal/archtest/arch_test.go`, `internal/archtest/guard_test.go` (existing guard style)
- `internal/cli/run.go`, `internal/cli/review.go`, `internal/cli/jev.go`,
  `internal/cli/reviewscope.go` (the review and change-attribution seam)

### Mutation Targets

- `internal/archtest/review_seam_neutrality_test.go` --- the scope definition and its coverage
  and completeness tests.
- Focused tests under `internal/archtest`.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- The neutrality guard covers `internal/cli/run.go` and every declared review-integration point.
- A maintainable scope definition: the covered set is derived from a documented, provider-neutral
  predicate (a production `.go` file in `internal/cli` that participates in the review or
  change-attribution seam) and asserted against the explicit reviewed list, so a new seam file
  or a removed declared file is surfaced rather than silently omitted.
- Focused tests: the guard fires on a provider token; the completeness check fails when a
  declared seam file is absent and when a discovered seam file is undeclared; the covered set
  includes `run.go`.

### Acceptance Criteria

- The coverage set includes `internal/cli/run.go`.
- A production `internal/cli` file that belongs to the review seam but is absent from the covered
  set fails the guard (no silent gap); a declared file that does not exist fails the guard.
- No provider, model, or domain name or branch is added.
- Provider/model neutrality holds and no new dependency is added.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read `internal/archtest/review_seam_neutrality_test.go` and the existing archtest style.
2. Implement the maintainable, self-checking scope covering `run.go`.
3. Add the focused tests.
4. Finish.

Expected first action: read `internal/archtest/review_seam_neutrality_test.go`.

### Production-Change Scope

Test-only. No production behavior change.

## H003 --- Editor-Only Markdown Formatting Reconciliation

Treat a pure Markdown emphasis-marker difference (`*x*` versus `_x_`) in a task's prose as
cosmetic in the reconcile comparison, so an editor-only reformat does not manufacture a
`changed (executed)` conflict, without weakening substantive change detection.

### Authoritative Inputs

- `internal/planflow/compare.go` (`canonicalProse`, `sameTaskDefinition`,
  `equivalentExecutedChange`, `sameAcceptanceCriteria`, `sameDeliverables`)
- `internal/planflow/planflow.go` (`reconcileState`)
- The planner render (`internal/planner/render.go`)

### Mutation Targets

- `internal/planflow/compare.go` (the canonical comparison of prose).
- Focused tests under `internal/planflow`.

### Dependencies

None

### Requires

- Go build/test toolchain

### Deliverables

- The canonical comparison folds a pure emphasis-marker difference (`*x*` versus `_x_`) in
  literal-free prose to one canonical form, so an editor reformat of emphasis is not a change.
- Substantive differences remain real changes: a synonym, negation, number, code identifier,
  quoted value, path, or an intra-word underscore (for example `snake_case` versus `snakecase`)
  is still reported as changed.
- Focused tests for both directions, including the negative cases.

### Acceptance Criteria

- An executed task whose definition changes ONLY by emphasis markers (`*x*` versus `_x_`) is
  reported unchanged/equivalent, not as a changed-executed conflict.
- A change inside code spans, quoted values, or backticks remains byte-exact: emphasis markers
  there are NOT folded, and a value that contains an intra-word underscore or asterisk is not
  altered.
- Substantive changes (word, negation, number, path, identifier, or code) remain real changes.
- Provider/model neutrality holds and no new dependency is added.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read `internal/planflow/compare.go` and its tests.
2. Implement the emphasis-marker normalization in the canonical comparison, guarded so
   code/quote content and intra-word delimiters are untouched.
3. Add the focused tests, including the negative cases.
4. Finish.

Expected first action: read `internal/planflow/compare.go`.

### Production-Change Scope

Reconcile comparison only. No gate threshold, severity, approval, budget, or routing change.

## H004 --- Complete H001 Acceptance Evidence and Clean Up the Review Seam

Add the focused regression tests H001 required but did not include, remove the dead activity no-op
in the run-level malformed-output path, and restore the two unrelated comments H001's edit
reformatted, without changing the H001 classification behavior.

### Authoritative Inputs

- `internal/cli/run.go` (`malformedOutputResult`, the review seam, and the reformatted comments in
  `timedValidation` and `writeModelSelectionArtifact`)
- `internal/review/provider.go` (`IsMalformedOutput`, `ErrMalformedOutput`)
- `internal/review/loop.go`
- `.agent-sdlc/runs/H001/review.json` (the review findings: missing tests, dead no-op)

### Mutation Targets

- `internal/cli/run.go`
- A new focused test file under `internal/cli`.

### Dependencies

- H001

### Requires

- Go build/test toolchain

### Deliverables

- Focused regression tests (a new test file under `internal/cli`) covering: a malformed review
  response is classified via `review.IsMalformedOutput(err)`; malformed output fails closed and
  never yields a `PASS`; an ordinary review error preserves the existing behavior; and a partial
  retained report cannot override the error-based classification.
- The dead `activity.FromContext(...)` no-op removed from `malformedOutputResult`.
- The two unrelated comments H001 reformatted restored to their prior wrapping.

### Acceptance Criteria

- A malformed review response is classified `NEEDS_HUMAN` via `review.IsMalformedOutput(err)` at
  the run-level caller, with a focused test.
- Malformed output is never a `PASS` (focused test).
- An ordinary review error keeps its existing behavior (focused test).
- A retained partial report does not override the error-based classification (focused test).
- The activity no-op is removed and behavior is unchanged.
- The unrelated comment reformatting is reverted: the `internal/cli/run.go` diff no longer
  contains the two unrelated comment changes.
- Provider/model neutrality holds and no new dependency is added.
- `gofmt`, `go vet`, `go test`, `go test -race`, `go build`, and `git diff --check` pass.

### Execution Contract

1. Read `.agent-sdlc/runs/H001/review.json` and `internal/cli/run.go`.
2. Add the focused tests.
3. Remove the no-op and revert the unrelated comments.
4. Finish.

Expected first action: read `internal/cli/run.go`.

### Production-Change Scope

Review seam only. No approval-authority, severity-threshold, quality-acceptance, budget, or
routing change.

## Regression Strategy

| #   | Scenario                                                     | Expected                                                 |
| --- | ------------------------------------------------------------ | -------------------------------------------------------- |
| 1   | Malformed review response at the run caller                  | explicit `IsMalformedOutput` → `NEEDS_HUMAN`, never PASS |
| 2   | Malformed response with a retained `HIGH`/`CRITICAL`         | still blocks at its severity                             |
| 3   | Neutrality guard scope includes `run.go`                     | covered                                                  |
| 4   | Seam file discovered but undeclared (or declared but absent) | guard fails (no silent gap)                              |
| 5   | Executed task changed only by emphasis markers               | unchanged/equivalent, not a conflict                     |
| 6   | Change inside code/quotes or an intra-word underscore        | still a real change                                      |

## Safety Invariants

This plan must not:

- change any gate threshold, severity meaning, or `quality.Evaluate` acceptance;
- bypass, weaken, or auto-resolve a human approval boundary;
- silently discard a valid finding or turn malformed output into a PASS;
- weaken the reconcile comparison's substantive-change detection;
- change commit/push authorization or provider/model selection;
- add or change any iteration, stale, tool-call, or retry budget;
- add third-party dependencies or name a provider, model, or task domain;
- modify Phase 8 / POST8-001, the convergence archive, the EV plan, or the RSH evidence.

## Dependency Graph

```text
H001 (explicit malformed-output classification)   -- independent
H002 (maintainable neutrality scope)              -- independent
H003 (editor-only Markdown reconcile fold)        -- independent
```

The three tasks are independent: they touch disjoint files (H001 `internal/cli/run.go` plus
`internal/cli` tests; H002 `internal/archtest`; H003 `internal/planflow/compare.go`). No task
depends on another, so any concurrency supported by the lifecycle is safe; the governed `sop run`
executes them sequentially in the single shared working tree.
