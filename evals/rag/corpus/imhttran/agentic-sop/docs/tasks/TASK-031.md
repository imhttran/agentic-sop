# T031 --- Review Stage

> Implements plan `PLAN-JEV.md` T015–T019 (reviewer abstraction, OCR detection,
> adapter, quality rules) and PRD §8.7–8.8, as the `sop review` command.

## Status

DONE

## Objective

Review the current working-tree changes with a configured, replaceable engine
and report findings, with a deterministic verdict:

```text
working-tree diff
      │
      v
  review engine (self agent | open-code-review)
      │
      v
  findings  ──> blocking = severity ∈ quality.fail_on ──> exit code
```

The model produces findings; SOP decides whether they block.

## Dependencies

- Stage 11/12 (structured self-review, Open Code Review adapter, T011/T012)
- T025/T026 (configuration: `review.engine`, `quality.fail_on`)
- T028 (quality gate, for the shared `BlockingFindings`)

## Scope

- `internal/cli/review.go`: the `sop review` command and engine selection.
- `internal/cli/cli.go`: `deps.readDiff`, dispatch, help.
- `internal/cli/cli_test.go`: review tests with an injected diff and agent.
- `internal/quality/quality.go`: export `BlockingFindings` so review and the gate
  agree on what blocks.

## Rules

- Self review uses the configured agent (provider from configuration); the
  `open-code-review` engine uses `SOP_REVIEW_COMMAND` and, when it is unset,
  fails with guidance rather than silently falling back.
- The diff is read through the Git adapter (never built from model output).
- What blocks is deterministic: a finding is blocking iff its severity is named
  in `quality.fail_on`; the exit code is non-zero when blocking findings remain.
- No configuration is required: review falls back to the built-in policy; no
  changes is a clean, non-zero-free result.

## Tests

CLI: no changes to review; a blocking (HIGH) finding makes the command fail and
is listed; a below-threshold (MEDIUM) finding passes; the open-code-review engine
without `SOP_REVIEW_COMMAND` errors and names the variable; bad args usage.
Quality: `BlockingFindings` is exercised through the existing table.

## Acceptance Criteria

- [x] `sop review` reviews the working-tree diff with the configured engine.
- [x] Findings are printed with severity and location.
- [x] Blocking is determined by `quality.fail_on`, not by the model.
- [x] A missing external review command is an error, not a silent fallback.
- [x] `make check` passes.

## Git

Branch: `task/T031-review-stage`
Commit: `task(T031): add review stage command`
PR: `[Task T031] Add review stage command`

## Out of Scope

Persisting review results to a run; the fix loop; the full `run` lifecycle.
