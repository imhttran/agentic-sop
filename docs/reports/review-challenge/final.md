# Review Challenge & Simplification — final reconciliation

Status: implemented and verified. No new stage, gate, retry budget, approval flow,
configuration setting, or model invocation was added.

## Audit result (CRS-001)

`CRS-001-gap-matrix.md` traced the runtime review path. One actionable gap existed:

- The self-review instruction was `"Review the implementation for the change under
  review."` plus a severity/schema note. It never named acceptance criteria,
  complexity, tests, or change justification, although `docs/specs/REVIEW.md` §7
  requires them.
- `review.Request.Criteria` and `review.Request.TestSummary` existed but no
  production call site populated them. In the `sop run` path the task render
  already carries acceptance criteria, and review only runs after validation
  passes.

Everything else was already satisfied and was not changed: model findings vs.
deterministic blocking (`Report.Blocking`, `quality.BlockingFindings`,
`quality.Evaluate`), structured parsing and malformed-output fail-closed handling,
the bounded FIX → VALIDATE → REVIEW loop, invocation-scoped review isolation, the
open-code-review adapter, and no-change skip behavior.

## Changes

| File | Change |
| --- | --- |
| `internal/review/provider.go` | The REVIEW task now states a fixed challenge order (goal fit → correctness → complexity → tests → change justification), the obligations a finding must meet, the preference for no change over speculative refactoring, and the rule not to assert test pass/fail without evidence. The output schema and severity vocabulary are unchanged. |
| `docs/specs/REVIEW.md` | §7.1 records the challenge order; §7.2 records finding obligations. `docs/specs/QUALITY.md` remains authoritative for blocking and verdicts. |
| `internal/review/challenge_test.go` | Deterministic coverage for the six CRS-004 cases plus a prompt-contract test. |

Reconciliation corrections made after SOP completed:

- The first draft required every finding to name a `file` and said findings without
  one are omitted. That would suppress legitimate task-level requirement gaps, which
  contradict the plan's "when relevant" rule. Both the prompt and the spec now
  require a code path when the finding concerns specific code, and a task criterion
  otherwise.
- Added the instruction not to claim tests passed or failed when the change or the
  supplied evidence does not show it.
- Added a deterministic test that asserts the production prompt carries the
  challenge order and obligations and that criteria/test summary/diff reach the
  reviewer.

## Verification (executed)

| Command | Result |
| --- | --- |
| `gofmt -l .` | no output |
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./...` | exit 0, no failing package |
| `go test ./internal/review/ -count=1` | `ok` (focused review tests) |

Test coverage in `challenge_test.go`:

1. Clean implementation and adequate evidence → valid no-findings review, fixer not called.
2. Missing requirement → blocking finding, fix, re-validate, re-review.
3. Advisory complexity → no forced remediation.
4. Weak or missing test evidence → advisory/blocking per configured policy, no false failure claim.
5. Simplification inside a fix iteration → tests rerun and review reruns.
6. Malformed review output → bounded retry, `NEEDS_HUMAN` classification, retained
   blocking finding, fixer not called, unrelated dirty tree untouched.

The prompt-contract test asserts the five challenge questions, the no-change
preference, the test-evidence rule, and that request context reaches the reviewer.

## Behavior preserved

- SOP remains the only blocking authority. The model still returns findings only.
- Severity policy, `quality.fail_on`, `quality.max_fix_cycles`, recovery, and
  escalation are unchanged.
- A valid empty findings list still passes without invoking the implementation
  agent again; no new retry counter exists.
- Review input remains invocation-scoped; the whole working tree is never reviewed.
- The review call count is unchanged: the strengthened prompt uses the existing
  REVIEW invocation.

## Limitations

- Production call sites still pass `Task` and `Diff` only. Acceptance criteria are
  present in the task render, so goal-fit review has that context. Validation
  results are not passed as `TestSummary`; review is gated on validation passing in
  `sop run`, and `sop review`/MCP have no validation context. Plumbing a summary
  would duplicate existing context and require review-cache identity changes, so it
  was deliberately not added. The prompt instead forbids asserting unobserved
  test results.
- No model-driven evaluation was added; correctness of the reviewer's judgement
  still depends on the model, bounded by deterministic parsing, severity policy,
  and the fix loop.

## Files

- `internal/review/provider.go`
- `internal/review/challenge_test.go`
- `docs/specs/REVIEW.md`
- `docs/plans/PLAN-SOP-Review-Challenge-and-Simplification.md`
- `docs/reports/review-challenge/CRS-001-gap-matrix.md`
- `docs/reports/review-challenge/final.md`
