# T064 --- IMPLEMENT Finalization Waits for a Stopped Writer

> A mutation observed earlier is not evidence the model has finished. Finalize
> only once the model has stopped writing, and never refuse a write during
> finalization.

## Status

DONE

## Objective

Follow up T063. Mutation-aware finalization fixed the unmutated case, but its
mutated twin surfaced on AHV2009 (Runtime Visibility): the model had already
written `internal/cli/stack.go` and edited `internal/cli/drive.go`, and was
mid-way through `internal/cli/cli_test.go`, when `finalizeEligible` fired at the
late-stage threshold. IMPLEMENT entered FINALIZE, refused the write, and then
exhausted the 24-iteration ceiling:

```text
mutation_observed=true, tool_calls=22, termination=iteration_limit,
last_action="write_file path=internal/cli/cli_test.go"
validation_runs: 0
```

A run that is still mutating has not finished. Do not finalize it, and do not
refuse the write it still needs.

## Scope

- `internal/ollamaagent/harness.go`: `finalizeEligible` requires
  `sinceMutation >= implementCompletionWindow` and drops the raw late-stage
  override for mutated runs; a mutation requested during FINALIZE is honoured and
  resumes CHANGE; the phase constants and doc comments are updated to match.
- `internal/ollamaagent/harness_test.go`: two regression tests for the shapes
  above.
- `README.md`: the IMPLEMENT bullet describes the stopped-writer rule.
- `LESSONS.md`: the counter-vs-precondition lesson notes the precondition must be
  current, not stale.
- `docs/PLAN.md`: Delivered gains `T064`.

## Rules

- **Finalization requires an observed mutation *and* a stopped writer.** Crossing
  a threshold is not evidence the work is done; a model still writing keeps its
  tools.
- **A write is never denied.** A mutation requested during FINALIZE is honoured and
  the invocation resumes CHANGE — FINALIZE stops exploration, it must not truncate
  a multi-file change. Non-mutation tools (reads/searches) are still refused.
- **The ceiling is unchanged.** `maxIterationsImplement` stays 24; a run that
  genuinely needs more still stops there, truthfully (`mutation_observed=true`),
  without the self-inflicted denial.
- **One engine.** Phase and mutation state stay invocation-scoped on
  `implementState`; nothing is persisted and SOP still sees one IMPLEMENT call.
- **Only IMPLEMENT changes.** PLAN keeps its discovery → synthesis phases; FIX,
  DESIGN_TESTS, DIAGNOSE_FAILURE, and REVIEW keep the generic loop.

## Tests

Deterministic, fake-Ollama tests: a model that keeps writing a multi-file change
past the finalize threshold keeps its tools and every write executes (the AHV2009
shape); a mutation requested during FINALIZE is honoured (not denied) and the
trace shows CHANGE resumed; plus the existing mutation-aware, early-completion,
no-mutation-ceiling, and PLAN fixtures, all unchanged.

## Acceptance Criteria

- [x] A model writing past the threshold keeps its tools and is not finalized.
- [x] A mutation requested during FINALIZE is honoured and returns to CHANGE.
- [x] Non-mutation tools are still refused during FINALIZE.
- [x] A model that never mutates still stops at the hard ceiling with
      `mutation_observed=false`.
- [x] The 24-iteration ceiling and every tool/command policy are unchanged.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./internal/ollamaagent/...`,
      `go test -race ./...`, and `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T064): IMPLEMENT finalization waits for a stopped writer`

## Out of Scope

Raising or removing the iteration limit; redesigning FIX/PLAN; changing the SOP
lifecycle, `ACTIVE_TASK`, or named-plan behavior; touching persisted SOP state;
committing AHV2009's partial implementation; committing or pushing the untracked
plan documents.
