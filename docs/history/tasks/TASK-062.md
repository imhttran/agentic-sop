# T062 --- IMPLEMENT Discovery → Change → Finalize Phases

> Make a productive IMPLEMENT return a structured outcome instead of consuming the
> whole tool budget exploring, so SOP validation can run.

## Status

DONE

## Objective

Fix the Ollama Agent IMPLEMENT loop. AHV2008 reached IMPLEMENT but never returned
control to SOP: the model kept reading/searching, hit the 24-iteration ceiling, and
failed with `iteration_limit` — SOP validation never ran. Give IMPLEMENT an explicit
completion path (discover enough context, make the change, then finalize and return)
without raising the safety ceiling or changing FIX/PLAN.

## Scope

- `internal/ollamaagent/harness.go`: `executeImplement`, the `implementPhase` model
  (`DISCOVER`, `CHANGE`, `FINALIZE`), invocation-scoped `implementState`, the
  discovery nudge and finalize transitions, `finalization_limit` termination, and
  the trace transition events.
- `internal/ollamaagent/trace.go`: the `denied` progress label.
- `internal/ollamaagent/prompt.go`: IMPLEMENT guidance names the three phases and
  that the model will be told when to finalize.
- `internal/ollamaagent/harness_test.go`: the phase tests and the AHV2008-shaped
  fixture.

## Rules

- **One engine, no new SOP state.** Phases are internal to a single IMPLEMENT
  invocation; nothing is persisted and SOP still sees one IMPLEMENT capability call.
- **Do not raise the ceiling.** `maxIterationsImplement` stays 24 as the hard
  safety bound; the normal completion mechanism is finalization, not exhaustion.
- **Mutation is observed, not claimed.** Only a successful `write_file`/`create_file`
  moves the invocation into CHANGE; a failed or denied write never counts, and the
  state is per-invocation so one run cannot leak into the next.
- **Tools are withdrawn in FINALIZE.** A tool request there is refused (audited as a
  denial) and counts against a two-turn allowance, then stops with
  `termination=finalization_limit`.
- **The agent does not own validation.** The finalize instruction tells the model SOP
  runs build/test/lint/review after it returns.

## Tests

Deterministic, fake-Ollama tests: early completion (no forced finalization), the
six-interaction discovery nudge, the mutation transition, a failed mutation and a
denied mutation not counting, forced finalization with tools withdrawn, a tool
request during finalization denied then a successful outcome, finalization
exhaustion (`finalization_limit`), invocation isolation, trace rendering of
`DISCOVER → CHANGE → FINALIZE`, and an AHV2008-shaped fixture where a mutating
IMPLEMENT finalizes and returns before the 24-iteration ceiling. PLAN and other
capabilities keep their existing tests.

## Acceptance Criteria

- [x] IMPLEMENT has explicit DISCOVER/CHANGE/FINALIZE phases scoped to one invocation.
- [x] A successful mutation moves the invocation into CHANGE; failed/denied writes do not.
- [x] Broad discovery is nudged toward implementation around six interactions.
- [x] At the finalize threshold repository tools are withdrawn and the model must
      return the structured outcome.
- [x] A model that keeps requesting tools in FINALIZE stops with
      `termination=finalization_limit`, not the 24-iteration ceiling.
- [x] The 24-iteration hard ceiling and every tool/command policy are unchanged.
- [x] PLAN, FIX, DESIGN_TESTS, REVIEW keep their existing behavior.
- [x] `gofmt -l .`, `go vet ./...`, `go test ./...`, `go test -race ./...`, and
      `go build ./...` pass.

## Git

Branch: `main`
Commit: `task(T062): IMPLEMENT discover → change → finalize phases`

## Out of Scope

Redesigning FIX; raising or removing the iteration limit; changing PLAN, the SOP
lifecycle, `ACTIVE_TASK`, or named-plan behavior; touching persisted SOP state or
the blocked dogfood tasks; committing or pushing.
