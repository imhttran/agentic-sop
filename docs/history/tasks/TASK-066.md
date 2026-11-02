# T066 --- Run FIX Through the Phased Completion Loop

> The capability that repairs a failure needs the same completion discipline as
> the one that makes the change: it must hand control back, not run the build and
> tests itself until the ceiling.

## Status

DONE

## Objective

T063/T064 fixed IMPLEMENT's completion path; the earlier brief deliberately
deferred the same treatment for FIX ("FIX may receive equivalent behavior later
after IMPLEMENT is proven"). Dogfooding AHV2009 showed it was due: after
validation failed, the fix loop ran (T065), FIX *made the repair*, and then kept
verifying — `gofmt`, `go build`, `go test`, `go vet` — until it hit the 24-turn
ceiling without ever returning its outcome:

```text
Ollama agent FIX did not complete after 24 iterations
  (termination=iteration_limit, last_action="run_command go vet ./...")
```

FIX was still running the generic `executeLoop`, which has no completion path.

## Scope

- `internal/ollamaagent/harness.go`: `Execute` routes `agent.Fix` alongside
  `agent.Implement` to the phased loop (renamed `executePhased`); the phase
  instructions are made capability-neutral ("the requested change", not "the
  implementation"); comments note that IMPLEMENT and FIX share the loop.
- `internal/ollamaagent/harness_test.go`: a FIX-phasing test, plus the instruction
  wording updated in the existing assertions.
- `README.md`: the phased-capability bullet and the per-capability bounds.

## Rules

- **One engine.** FIX reuses the same `DISCOVER → CHANGE → FINALIZE` loop as
  IMPLEMENT — no new orchestration, no second state machine.
- **FIX does not own validation.** Once it has written its repair and stopped
  mutating, the tools are withdrawn and it must return its outcome; SOP then
  re-validates and re-reviews.
- **The bounds are unchanged.** FIX keeps its 24-turn ceiling and its tool policy.
- **PLAN, REVIEW, DESIGN_TESTS, DIAGNOSE_FAILURE are unchanged** (generic loop).

## Tests

`TestFixUsesPhasedCompletion`: a FIX that writes its repair and then keeps
verifying still finalizes and returns before the ceiling, with the trace recording
the `FIX` capability. Existing IMPLEMENT phase tests updated only for the
capability-neutral instruction wording.

## Acceptance Criteria

- [x] `agent.Fix` runs the phased loop and finalizes before the ceiling.
- [x] The phase instructions read correctly for both IMPLEMENT and FIX.
- [x] IMPLEMENT's behavior and tests are unchanged apart from wording.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T066,T067): FIX shares the phased loop; persist a failed-run trace`

## Out of Scope

Changing IMPLEMENT's phase bounds; PLAN, REVIEW, DESIGN_TESTS, DIAGNOSE_FAILURE;
SOP's lifecycle or the fix loop; committing AHV2009's implementation.
