# CRS-001 — Review Challenge Audit: Gap Matrix

**Task:** CRS-001 (PLAN-SOP-Review-Challenge-and-Simplification)
**Stage:** Read-only audit. No production code changed.
**Comparison basis:** `docs/specs/REVIEW.md` §7 (Self-Review Focus) and `PLAN-SOP-Review-Challenge-and-Simplification.md` §3 challenge criteria.

---

## 1. Scope and method

This stage traces the actual review lifecycle, adapters, prompt construction, parsing,
test-summary input, and quality-gate integration. It records what exists, where the
smallest change surface is, and whether the desired behavior is already present.

No production behavior is changed. The only file created by this stage is this report.

## 2. Files opened (read-only trace)

| Path | Purpose |
| --- | --- |
| `internal/review/review.go` | Schema (`Request`, `Finding`, `Report`), `Report.Blocking`, `Request.text()` |
| `internal/review/provider.go` | `reviewOutputSchema`, `AgentProvider.Review`, `parseReport`, `ErrMalformedOutput`, `reviewRetryBound` |
| `internal/review/ocr.go` | `OCRProvider.Review`, `ProviderFromEnv`, `EnvReviewCommand` |
| `internal/review/loop.go` | `Loop.Run`, `newMalformedOutputError` |
| `internal/cli/run.go` | `runRun`, `runSingleTask`, `runStages`, review call site (`provider.Review`), `reviewProvider`, `fixContext`, quality gate `quality.Evaluate` |
| `internal/cli/review.go` | `runReview` (`sop review`), `reviewProvider`, `quality.BlockingFindings` usage |
| `internal/cli/mcp.go` | Additional review call site (`provider.Review`) |
| `docs/specs/REVIEW.md` | Normative review spec, §7 |
| `docs/plans/PLAN-SOP-Review-Challenge-and-Simplification.md` | Plan and challenge criteria |

Search probes (`search_files "review.NewLoop"`, `search_files "TestSummary"`,
`search_files "review.Request{"`) located every call site.

## 3. Traced call sites

| Layer | Path | Function / symbol |
| --- | --- | --- |
| `sop run` review lifecycle | `internal/cli/run.go` | `runRun` -> `runSingleTask` / `runGraph` -> `runStages`; review call `provider.Review(ctx, review.Request{Task: reviewTask, Diff: reviewDiff})`; fix loop (`agent.Fix`, `sessionValidation`, re-`Review`) in `runStages`; `fixContext` |
| `sop review` command | `internal/cli/review.go` | `runReview` -> `reviewProvider` -> `provider.Review(context.Background(), review.Request{Task: "Review the current working changes.", Diff: diff})`; `quality.BlockingFindings(cfg.Quality.FailOn, report.Findings)` |
| Additional review call site | `internal/cli/mcp.go` | `provider.Review(ctx, review.Request{Task: "Review the current working changes.", Diff: diff})` |
| Self adapter | `internal/review/provider.go` | `AgentProvider.Review` -> `agent.Generate(Capability: agent.Review, Task: "Review the implementation for the change under review.", Input: request.text(), OutputRequirements: reviewOutputSchema)` |
| Open Code Review adapter | `internal/review/ocr.go` | `OCRProvider.Review`; `ProviderFromEnv` (`SOP_REVIEW_COMMAND`) |
| Prompt construction | `internal/review/provider.go` | `reviewOutputSchema` (const); `Request.text()` in `internal/review/review.go` joins `Task`, `Criteria`, `TestSummary`, `Diff` |
| Prompt template | not found | No review prompt template file exists. `docs/plans/BACKLOG.md` §532 mentions review/plan/diagnose/implement templates, but the runtime REVIEW prompt is the literal `Task` string plus `reviewOutputSchema`. Searched `internal/prompt`, `internal/review`, and the repository via `search_files`. |
| Structured parsing | `internal/review/provider.go` | `parseReport`, `reportWire`, `ErrMalformedSeverity` |
| Test summary producer | `internal/review/review.go` | `Request.TestSummary` field; `Request.text()` places it before `Diff` |
| Test summary producer (runtime) | `internal/cli/run.go`, `internal/cli/review.go` | Neither call site populates `Criteria` or `TestSummary`; both pass `Task` and `Diff` only |
| Quality gate integration | `internal/quality` (via `internal/cli/run.go` `runStages`) | `quality.Evaluate(cfg.Quality, quality.Input{... Unresolved: report.Findings ...})`; `quality.BlockingFindings(cfg.Quality.FailOn, report.Findings)` |
| Bounded fix loop (library) | `internal/review/loop.go` | `Loop.Run` — review, fix, re-verify (`Tester.Run`), re-review up to `maxRounds`; `newMalformedOutputError` |
| Run-level remediation | `internal/cli/run.go` | `runStages` inner loop: on `gate.Decision == quality.Fail` with `autoPolicy.AutoFix`, calls `a.Generate(Capability: agent.Fix, Input: fixContext(...))`, re-reads the diff, loops to re-validate and re-review. `fixContext` renders `# Blocking findings`. |

## 4. Gap matrix

| Requirement | Existing behavior | Gap | Minimal target | Evidence |
| --- | --- | --- | --- | --- |
| Model produces findings; SOP decides blocking | `Report.Blocking(threshold)` is the only blocking authority; `quality.BlockingFindings` and `quality.Evaluate` reuse it | None | none | `internal/review/review.go` `Report.Blocking`; `internal/cli/run.go` `runStages` (`gate = quality.Evaluate(...)`); `internal/cli/review.go` `runReview` |
| Structured findings schema (severity, path, line, message) | `reviewOutputSchema` and `reportWire` carry `severity`, `title`, `detail`, `file`, `line`, `suggestion` | No `category` field, although `docs/specs/REVIEW.md` §2 names `category` | Add `category` only if the spec is read verbatim | `internal/review/provider.go` `reviewOutputSchema`, `reportWire`; `internal/review/review.go` `Finding` |
| Self-review focus asks about acceptance criteria, correctness, tests, edge cases, error handling, architecture, **unnecessary complexity** | `AgentProvider` task is `"Review the implementation for the change under review."`; `reviewOutputSchema` says `Report only real findings; use severity CRITICAL/HIGH only for correctness, safety, or security problems.` No caller populates `Request.Criteria` | **GAP** — the prompt does not enumerate the §7 focus areas; "unnecessary complexity" is not asked for anywhere | CRS-002: extend the `AgentProvider` task string and/or `reviewOutputSchema` guidance to name the §7 focus areas, keeping schema shape and severity vocabulary | `internal/review/provider.go` `AgentProvider.Review` (`Task`, `OutputRequirements`); `docs/specs/REVIEW.md` §7 |
| Test sufficiency is asked / test summary supplied | `Request.TestSummary` and `Request.Criteria` exist; `Request.text()` places them in the review context | **GAP** — `internal/cli/run.go` passes `review.Request{Task: reviewTask, Diff: reviewDiff}` and `internal/cli/review.go` passes `review.Request{Task, Diff}`; neither populates `Criteria` or `TestSummary` | CRS-002: populate `Criteria` from `spec.AcceptanceCriteria` and `TestSummary` from the `testrunner.SuiteResult` at the review call sites | `internal/review/review.go` `Request`, `Request.text()`; `internal/cli/run.go` review call site; `internal/cli/review.go` `runReview` |
| Structured parsing | `parseReport` retains valid findings, rejects invalid severities as `*ErrMalformedSeverity`, retries up to `reviewRetryBound=3`, returns bounded `*ErrMalformedOutput` with raw response and retained findings | None | none | `internal/review/provider.go` `parseReport`, `reviewRetryBound`, `ErrMalformedOutput`; `internal/review/malformed_test.go`, `internal/review/review_test.go` |
| Open Code Review adapter | `OCRProvider.Review` sends `Request` JSON on stdin and parses stdout; `ProviderFromEnv` reads `SOP_REVIEW_COMMAND` and returns `ErrNotConfigured` when unset | None | none | `internal/review/ocr.go` `OCRProvider.Review`, `ProviderFromEnv`; `internal/review/ocr_test.go` |
| Findings become remediation | `runStages` fix loop invokes `agent.Fix` with `fixContext(...)`, which renders `# Blocking findings` with severity, location, title, detail; the diff is re-read and the loop re-validates and re-reviews. `review.Loop.Run` implements the same fix/re-verify/re-review cycle for the library API. | None | none | `internal/cli/run.go` `runStages` fix block and `fixContext`; `internal/review/loop.go` `Loop.Run`; `internal/e2e/dogfood_test.go` uses `review.NewLoop` |
| Deterministic ownership of blocking | `Report.Blocking` plus `quality.BlockingFindings`/`quality.Evaluate`; the fix-loop condition is `quality.BlockingFindings(cfg.Quality.FailOn, report.Findings) + quality.JEVBlockingFindings(...)` | None | none | `internal/review/review.go` `Report.Blocking`; `internal/quality` (`BlockingFindings`, `Evaluate`); `internal/cli/run.go` `runStages` |
| Fail-closed malformed output | `runStages` maps `review.IsMalformedOutput(err)` to `malformedOutputResult` -> `WAITING_FOR_HUMAN`, never a pass; `review.Loop.Run` returns `newMalformedOutputError` | None | none | `internal/cli/run.go` `malformedOutputResult`, `runStages`; `internal/review/loop.go` `Run`; `internal/review/malformed_test.go` |
| Review scope isolation | `runStages` builds `newReviewScope(rn.ChangedFiles(), reports, reports...)` and calls `reviewInput(...)`; the whole tree is never reviewed; the seam fails closed when no task-owned path can be established | None | none | `internal/cli/run.go` `runStages` review scope block; `internal/cli/reviewscope_runtime_test.go` |
| No changes to review | `runStages` treats `alreadySatisfied`, `legacyNoChange`, and `!changeRequired` as the no-review case; `sop review` prints `no changes to review` for an empty diff | None | none | `internal/cli/run.go` `runStages`; `internal/cli/review.go` `runReview` |

## 5. Does the current prompt already ask about unnecessary complexity and test sufficiency?

**Unnecessary complexity — GAP.** The reviewer receives exactly two instruction
strings:

1. `AgentProvider.Review` `Task`: `"Review the implementation for the change under review."`
2. `OutputRequirements = reviewOutputSchema`: `Report only real findings; use severity CRITICAL/HIGH only for correctness, safety, or security problems.`

Neither string names unnecessary complexity (nor acceptance criteria, tests, edge
cases, error handling, or architecture boundaries).

**Test sufficiency — GAP.** `Request.TestSummary` and `Request.Criteria` exist in
`internal/review/review.go`, but no production caller populates them:
`internal/cli/run.go` passes `review.Request{Task: reviewTask, Diff: reviewDiff}`,
`internal/cli/review.go` passes `review.Request{Task: "...", Diff: diff}`, and
`internal/cli/mcp.go` passes `Task` and `Diff` only.

`docs/specs/REVIEW.md` §7 requires self-review to examine acceptance criteria,
correctness, tests, edge cases, error handling, architecture boundaries, and
unnecessary complexity. The runtime prompt covers only correctness. The gap is
actionable.

## 6. Per-criterion challenge verdicts

| Criterion | Verdict | Evidence |
| --- | --- | --- |
| **Requirement fit** | **GAP** | The runtime prompt omits the §7 focus areas and does not carry `Criteria` or `TestSummary`. `docs/specs/REVIEW.md` §7; `internal/review/provider.go` `AgentProvider.Review`; `internal/cli/run.go` review call site; `internal/cli/review.go` `runReview` |
| **Correctness** | **PRESENT** | `parseReport`, `ErrMalformedSeverity`, `ErrMalformedOutput` (`internal/review/provider.go`; `malformed_test.go`, `review_test.go`); `malformedOutputResult` (`internal/cli/run.go`); `Loop.Run` (`internal/review/loop.go`); `OCRProvider.Review` (`internal/review/ocr.go`, `ocr_test.go`) |
| **Simplicity** | **PRESENT** | The smallest surface is the `AgentProvider.Review` task/schema strings in `internal/review/provider.go` and the `review.Request` literals in `internal/cli/run.go` and `internal/cli/review.go`. `Request.Criteria`/`TestSummary` already exist in `internal/review/review.go`. No schema change is required. |
| **Test evidence** | **PRESENT for existing behavior; GAP for the §7 prompt** | `malformed_test.go`, `review_test.go`, `ocr_test.go` cover parsing and adapters; `internal/cli/reviewscope_runtime_test.go` covers scope isolation; `internal/e2e/dogfood_test.go` exercises `review.NewLoop`. No test asserts the §7 focus areas because the prompt does not name them. |
| **Change justification** | **GAP** | `Request.Criteria` and `Request.TestSummary` are declared but unused by production callers; the prompt does not name unnecessary complexity. CRS-002 has both justification and a small existing surface to edit. |

## 7. ALREADY_SATISFIED record

These challenge criteria are already satisfied. No code change is made for them.

- **Model produces findings; SOP decides blocking** — `Report.Blocking`
  (`internal/review/review.go`); `quality.BlockingFindings` and `quality.Evaluate`
  used in `internal/cli/run.go` `runStages` and `internal/cli/review.go` `runReview`.
- **Structured parsing and malformed-output boundary** — `parseReport`,
  `ErrMalformedSeverity`, `ErrMalformedOutput`, `reviewRetryBound`
  (`internal/review/provider.go`) with `malformed_test.go` and `review_test.go`;
  `malformedOutputResult` (`internal/cli/run.go`); `Loop.Run`
  (`internal/review/loop.go`).
- **Open Code Review adapter** — `OCRProvider.Review` and `ProviderFromEnv`
  (`internal/review/ocr.go`) with `ocr_test.go`.
- **Findings become remediation** — `runStages` fix loop and `fixContext`
  (`internal/cli/run.go`); `Loop.Run` (`internal/review/loop.go`).
- **Deterministic blocking ownership** — `Report.Blocking` and the fix-loop
  condition using `quality.BlockingFindings` in `internal/cli/run.go`.
- **Review scope isolation and pre-existing user changes preserved** —
  `newReviewScope` / `reviewInput` in `runStages` (`internal/cli/run.go`); the
  `git status --short --branch` baseline in §9.

## 8. Smallest change surface (for CRS-002, not applied here)

| Location | Symbol | Proposed change (not applied) |
| --- | --- | --- |
| `internal/review/provider.go` | `AgentProvider.Review` `Task` / `reviewOutputSchema` | Enumerate the §7 focus areas, including unnecessary complexity and test sufficiency |
| `internal/cli/run.go` | `runStages` review call | Populate `Criteria` from `spec.AcceptanceCriteria` and `TestSummary` from the `testrunner.SuiteResult` |
| `internal/cli/review.go` | `runReview` review call | Populate `TestSummary` if a suite ran |

No other file needs to change.

## 9. Plan activation / handoff flow and repository state

**Verified plan activation/handoff mechanism.** Plan activation is performed by the
SOP lifecycle (`sop run`) itself in `internal/cli/run.go`: `runRun` ->
`runSingleTask` / `runGraph` -> `runStages`, which owns PLAN -> IMPLEMENT ->
(VALIDATE -> REVIEW -> GATE -> FIX)* and the run artifacts. Plan provenance is
recorded under `.agent-sdlc` (`planMetaName`); resumption uses `loadPriorPlan` /
`loadActivePlan` (`internal/cli/run.go`). No separate activation step bypasses SOP
verification or approval.

**Repository state (`git status --short --branch` at audit capture):**

```text
## review/challenge-simplification...origin/main
?? docs/plans/PLAN-SOP-Review-Challenge-and-Simplification.md
```

The untracked plan file is user-owned and was not modified. This stage creates only
`docs/reports/review-challenge/CRS-001-gap-matrix.md`.

## 10. Read-only guarantee

No production code was changed. `internal/review/*`, `internal/cli/run.go`,
`internal/cli/review.go`, `internal/cli/mcp.go`, and `internal/quality/*` were
inspected, not edited. Pre-existing working-tree changes were not touched. The only
file this stage writes is this report.
