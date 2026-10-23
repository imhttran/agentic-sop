# HIP-008 — Delivery record

- Pull request: https://github.com/imhttran/agentic-sop/pull/114
- Branch: `feat/sop-harness-improvement-pilot`
- Base: `main`
- Status: open, not merged
- Implementation commit: `91faba8a`
- Focused tests: `26ad88cc`
- Harness guard/instruction fixes: `af3bc781`, `2984f834`
- Pilot selection: `0534703e`
- Pilot evidence and adoption report: `b0c93e93`
- Harness documentation: `fb72a4c2`
- Delivery metadata: `62857b40`
- Review corrections: this commit

## Result

The bounded pilot launched **7 arm executions**: one discarded S1 baseline setup
attempt, two S1 rerun arms (one baseline failure and one candidate compile
failure), and two arms each for S2 and S3. Six appear in comparison files; the
setup attempt is preserved only as a log. This is one over the initial six-arm
pair budget but below the absolute eight-arm ceiling. No held-out pair ran.

S2 and S3 are valid execution pairs but no arm passed fixed acceptance; S1 is
invalid as a comparison. The pilot is negative for the candidate as tested and
inconclusive for candidate quality. The candidate is not included in the PR.

Token usage was not reported by the executor. The 200000-token limit is configured
but its enforcement is unproven; the 900-second time limit is enforced.

## Checks

- `go build ./...` passed.
- `go vet ./...` passed.
- `go test ./... -count=1` passed.
- `go test ./internal/hipcompare -count=1` passed.
- `go test ./internal/archtest -count=1` passed.
- GitHub checks passed for the branch head before the review corrections; affected
  checks are rerun after the correction commit.

The PR is open for review and must not be merged by the pilot runner.
