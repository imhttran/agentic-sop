# T060 --- Ground IMPLEMENT/FIX Outcomes in the Working Tree

> A completed IMPLEMENT/FIX outcome's `changes_expected` is reconciled with the
> repository change the harness actually observed, so a run cannot claim a change
> it did not make (nor deny one it did).

## Status

DONE

## Objective

The command agent reports a structured outcome, but `changes_expected` is only the
model's claim. Ground it in reality: after a completed IMPLEMENT/FIX, compare the
claim with whether the repository working tree actually changed, override with the
observed truth, and surface the disagreement rather than silently correcting it.

## Scope

- `internal/ollamaagent/outcome.go` (new): recognize the same wire shape SOP parses
  (`outcomeWire`) and reconcile a completed outcome's `changes_expected` against
  the observed working-tree change. A `needs_human`/`failed` outcome and any
  non-outcome content pass through unchanged; when the tree cannot be inspected the
  claim is left as reported.
- `internal/toolharness/harness.go`: `WorkingTreeChanged` — a fixed, read-only
  `git status --porcelain` that ignores SOP's state directory and reports a failure
  to inspect as an error.
- `internal/ollamaagent/harness.go`: `Run` reconciles the final content for
  IMPLEMENT/FIX and reports a mismatch on stderr; the harness records the
  disagreement.
- Tests and README.

## Rules

- **Observation wins.** The working tree is the signal, so a change made through
  any tool (a file write or a formatting command) counts; the harness's own audit
  bookkeeping does not.
- **The claim is preserved.** The model's reported value stays visible in the
  summary note; only `changes_expected` is corrected, so SOP's outcome vocabulary
  is unchanged.
- **No groundless override.** If the working tree cannot be inspected, the model's
  claim is left untouched.
- **SOP stays the authority.** This is an adapter-side reconciliation; SOP's own
  diff check and gates are untouched.

## Tests

Reconciliation (`internal/ollamaagent`): a claimed change with a clean tree → false
plus a summary note and a stderr notice; a silent change → true; a consistent claim
→ unchanged. Observation (`internal/toolharness`): a clean tree, SOP state only,
and a new source file. All deterministic, no live provider.

## Acceptance Criteria

- [x] A completed IMPLEMENT/FIX outcome is grounded in the working tree.
- [x] A disagreement is surfaced (summary note + stderr), never silently hidden.
- [x] `needs_human`/`failed` and non-outcome content pass through unchanged.
- [x] An uninspectable tree leaves the claim untouched.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T059): PLAN discovery/synthesis and grounded IMPLEMENT outcomes`
(the two features' code is interleaved in `harness.go`/`harness_test.go`/
`policy.go`, so they landed in one commit)

## Out of Scope

SOP lifecycle, validation, or gate changes; the PLAN phases (T059); committing the
plan documents (`docs/PLAN-*.md`).
