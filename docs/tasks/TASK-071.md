# T071 --- A No-Change Failure Is Retryable

> An IMPLEMENT that changed nothing did not attempt the work; surface it as a
> retryable boundary instead of blocking.

## Status

DONE

## Objective

With T068 the harness's own no-change exhaustion (an unmutated run that reaches
the late stage) already returns a retryable `needs_human` outcome. But when the
*model itself* reports `failed` after changing nothing, SOP blocks the task. That
is what happened to AHV2011: the model explored for ~22 turns without writing, was
force-finalized, and returned

```text
{"status":"failed","reason":"Unable to implement AHV2011 tests within this
invocation: repository exploration consumed the available tool budget before any
test files could be written. ..."}
```

→ `AHV2011 BLOCKED`, a manual `sop retry` for what is really a flaky non-writing
sample.

## Scope

- `internal/ollamaagent/outcome.go`: `retryNoChangeFailure` rewrites a
  model-reported `failed` outcome to `needs_human` when the working tree did not
  change, keeping the model's reason and adding a short note.
- `internal/ollamaagent/harness.go`: `Run` applies it for `IMPLEMENT`/`FIX` after
  `reconcileOutcome`.
- `internal/ollamaagent/harness_test.go`: the retryable and the kept-failure cases.
- `README.md`, `LESSONS.md`, `docs/PLAN.md`.

## Rules

- **Classify a no-op, don't invent success.** Only a `failed` outcome with an
  unchanged working tree is rewritten, and only to the retryable `needs_human`
  boundary — never to `completed`, and never with a fabricated change.
- **A real failure stays a failure.** A `failed` outcome with an observed change
  (or one that cannot be inspected) is left exactly as reported, as are `completed`
  and `needs_human`.
- **Bounded.** Retries are bounded by the task's `max_attempts`; an agent that
  genuinely cannot do the work ends in `BLOCKED` after the budget, as before.
- **Only mutating capabilities.** The rule applies to `IMPLEMENT` and `FIX`; other
  capabilities are untouched.

## Tests

`TestRunNoChangeFailureIsRetryable` (a model-reported `failed` with a clean tree →
`needs_human`), `TestRunChangedTreeKeepsFailure` (a `failed` with a changed tree
stays `failed`).

## Acceptance Criteria

- [x] A model-reported `failed` IMPLEMENT/FIX with no repository change is surfaced
      as a retryable `needs_human`.
- [x] A `failed` outcome with an observed change is left as a hard failure.
- [x] The rewrite never claims success and never fabricates a change.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T071): a no-change failure is retryable`

## Out of Scope

Changing SOP's retry policy or `max_attempts`; changing the `completed` path;
rewriting a failure that did change the tree; splitting large tasks.
