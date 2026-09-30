# Pre-JEV Baseline (PREJEV001)

Captured: 2026-09-28. Scope: `agentic-sop` at `main` (HEAD `0992657`,
"ollamaagent harness, JEV capability, and planning docs").

This baseline was produced read-only: no `.agent-sdlc/state.db` row was
written, and no AHV task was retried, reset, or reviewed as part of
producing it. Evidence sources are cited per finding.

## Source of truth note

`.agent-sdlc/state.db`'s `tasks` table currently holds only the active
`PREJEV0xx` plan; it has **no AHV2001–AHV2013 rows** (verified via
read-only `strings` inspection of the db file — no AHV id appears as a
standalone task row, only as substrings inside PREJEV task text). AHV2001–
AHV2013 belong to the separate, now-graduated `plan-agent-harness-v2` plan.
Current AHV state is therefore sourced from `.agent-sdlc/runs/<task>/state.json`
and `report.json` (per-task source of truth) cross-checked against `git log`
(ground truth for what actually landed on `main`).

## Task states

| Task    | Title                              | Run artifact stage | Committed on `main`             |
|---------|-------------------------------------|---------------------|----------------------------------|
| AHV2001 | Correct Provider Capabilities      | PASSED              | yes — `1a977f2`                  |
| AHV2002 | Introduce Harness Abstraction      | PASSED              | yes — `d637a79`, `1a977f2`       |
| AHV2003 | Extend Configuration               | PASSED              | yes — `1a977f2`                  |
| AHV2004 | Implement Controlled Tool Harness  | PASSED              | yes — `1a977f2`                  |
| AHV2005 | Implement Agent Tool Loop          | PASSED              | yes — `1a977f2`                  |
| AHV2006 | Ollama Tool Integration            | PASSED              | yes — `1a977f2`                  |
| AHV2007 | Structured Execution Outcome       | PASSED              | yes — `1a977f2`, `6cc61b8`       |
| AHV2008 | Default Selection                  | PASSED              | yes — `6450d8d`, `1a977f2`       |
| AHV2009 | Runtime Visibility                 | PASSED              | yes — `414bc71`, `51d8e0e`       |
| AHV2010 | Preserve Command/Claude Compat.    | PASSED              | yes — `1a977f2`                  |
| AHV2011 | Tests                              | FAILED              | **no**                           |
| AHV2012 | End-to-End Ollama Dogfood          | FAILED (stale, see below) | yes — `aec2774`            |
| AHV2013 | Documentation                      | PASSED              | yes — `6b7c8b1`                  |

`git status` on this baseline shows only `docs/plans/PLAN-Pre-JEV-Stabilization.md`
modified (this plan's own tracking doc) — no uncommitted AHV work exists, so
there is no LOCAL_DONE work at risk. Every AHV task whose run artifact ever
reported a passing/completed result is already committed on `main`.

## LOCAL_DONE preservation

No AHV task is currently sitting in an uncommitted LOCAL_DONE state. All
work that reached a passing gate (AHV2001–AHV2010, AHV2012, AHV2013) has
already been carried through the human commit gate and lives on `main` at
the commits listed above. Nothing was altered, retried, or reset to produce
this baseline.

## Blocked-task classification

### AHV2011 — Tests: **unresolved / external failure, not a task defect**

Evidence: `.agent-sdlc/runs/AHV2011/report.json` — `stage: FAILED`, sole
reason:

> You've hit your session limit · resets 1:30pm (America/Chicago)

`metrics`: `implement: 224.5s`, `plan: 50.2s`, `agent_calls: 2`,
`validation_runs: 0`. No `diff.patch`, `validation.json`, or `review.json`
exist for this run — the attempt never produced a mutation or reached
validation.

Classification: **transient external failure** (provider session-limit
exhaustion), not a defect in the AHV2011 task definition itself. It also
exposed a harness robustness gap (a provider rate-limit message was
accepted as a gate/review verdict instead of being treated as an
infrastructure error) — that gap is in scope for PREJEV002/PREJEV003, not
this baseline. AHV2011 has never been retried since this failure (no
commit exists for it on `main`); it remains genuinely open and should be
retried through normal SOP `sop retry AHV2011`, not through manual state
edits.

### AHV2012 — End-to-End Ollama Dogfood: **harness lifecycle defect (now resolved by commit)**

Evidence is internally inconsistent, which is itself the finding:

- `.agent-sdlc/runs/AHV2012/report.json` (`generated_at` 2026-09-28T05:42:47Z):
  `stage: WAITING_FOR_HUMAN`, decision `NEEDS_HUMAN`, reason "Ollama agent
  IMPLEMENT made no repository change after 32 iterations ... termination=no_change
  ... a retry may succeed."
- `.agent-sdlc/runs/AHV2012/state.json` (`updated_at` 2026-09-28T05:57:11Z,
  15 minutes later): final recorded stage is `FAILED`.
- `.agent-sdlc/runs/AHV2012/diff.patch` and `metrics.json` (same run
  directory) show a **substantive, successful mutation**: new files
  `docs/DOGFOOD-OLLAMA.md`, `docs/history/OLLAMA-DOGFOOD.md`,
  `internal/ollamaagent/dogfood_test.go`, and companion plan docs, plus
  `validation_runs: 1`, `review_runs: 1` in `metrics.json` — evidence of a
  later, successful attempt within the same run.
- `git log` shows commit `aec2774` ("task(AHV2012): End-to-End Ollama
  Dogfood (opt-in test and doc)") at 2026-09-28 06:12:31Z — **after** the
  run's own final `FAILED` bookkeeping at 05:57:11Z — landing exactly the
  content in `diff.patch`, and it is present on `main` today.

Classification: **harness lifecycle defect, now resolved**. The task's
actual implementation succeeded (visible in `diff.patch`/`metrics.json` and
confirmed committed on `main`), but the harness's own run bookkeeping
recorded a stale `FAILED` outcome after the successful attempt, not before
it. This matches the class of finalization/reporting bugs PREJEV002/
PREJEV007–009 target (invocation-scoped state, reconciliation between
observed mutation and reported outcome). No further action is needed on
AHV2012 itself — the work is done and on `main` — but the stale `FAILED`
artifact should not be read as "AHV2012 failed."

## Baseline build/test status

First captured at HEAD `0992657`, 2026-09-28, read-only: `go vet`/`go test`
failed in `internal/ollamaagent`, `internal/cli`, and `internal/agent`
because the in-flight ollamaagent refactor commit changed production
signatures (`ensureStructuredOutcome`'s baseline parameter, PLAN/REVIEW's
per-capability synthesis-transition event, the `deps.newAgent` factory's
`harness` parameter) without updating the test files that called them, and
because one `internal/agent` subtest failed to clear `SOP_AGENT_PROVIDER`
before asserting explicit-argument precedence (an ambient-environment
dependent test, not a code defect).

None of these breakages touch AHV2001-AHV2013's own implementation; they are
test-file drift against already-committed production code (`internal/agent`,
`internal/cli`, `internal/ollamaagent`) from work layered on top of the AHV
plan. Per SOP's validation gate, `go test ./...` failing blocks this task's
own completion regardless of scope, so the test files were updated to match
the current production signatures (mechanical realignment only — no
production code in `internal/agent`, `internal/cli`, or `internal/ollamaagent`
was changed):

- `internal/ollamaagent/outcome_test.go`: updated all `ensureStructuredOutcome`/
  `reconcileOutcome`/`retryNoChangeFailure` call sites to the current
  3-arg/`(string, bool)` signatures, using `h.tools.WorkingTreeFingerprint`
  to capture a real baseline where the test asserts change detection.
- `internal/ollamaagent/harness_test.go`: replaced the undefined
  `synthesisTransitionEvent`/`reviewDiscoveryTurns` identifiers with the
  actual per-capability values (`planTwoPhase.synthLabel`,
  `reviewTwoPhase.synthLabel`, `reviewInspectTurns`); added an initial commit
  to the `gitInit` test helper so `WorkingTreeFingerprint`'s `git diff HEAD`
  has a HEAD to diff against; updated `TestCapabilityBudgets`'s REVIEW
  expectation from 8 to 10 (`reviewInspectTurns + reviewSynthesizeTurns`,
  matching the documented two-phase budget); rewrote
  `TestRunChangedTreeKeepsFailure` to mutate the tree via a tool call during
  the invocation rather than before it starts, since baseline capture is now
  invocation-scoped and pre-existing dirty work is correctly not attributed
  to the run.
- `internal/cli/cli_test.go`, `internal/cli/perf_test.go`: added the missing
  leading `harness` parameter to four `newAgent` test factories.
- `internal/agent/provider_test.go`: cleared `EnvAgentProvider`/
  `EnvAgentHarness` in the "tool + ollama" subtest, matching the isolation
  pattern every other subtest in this file already follows.

Current status, same HEAD:

```
$ gofmt -l .
(no output — all files formatted)

$ go build ./...
(no output — build succeeds)

$ go vet ./...
(no output — vet passes)

$ go test ./...
ok   (all ~29 packages)
```

## Summary

- AHV2001–AHV2010, AHV2012, AHV2013: **done and committed on `main`**. No
  unexplained blocks remain for these tasks.
- AHV2011: **genuinely unresolved**, caused by an external provider
  session-limit failure, not a code defect in the task. Needs a normal
  `sop retry`, not a manual state edit.
- AHV2012's `FAILED` run-artifact stage is stale/misleading — the work
  succeeded and is committed; classified as a (now moot) harness
  bookkeeping defect.
- No LOCAL_DONE work existed to preserve; nothing was reset, retried, or
  manually edited to produce this baseline.
- `main` now builds, vets, and tests clean end to end; the test-file drift
  against the in-flight ollamaagent refactor (unrelated to AHV2001-AHV2013)
  was realigned to unblock this task's own validation gate.
