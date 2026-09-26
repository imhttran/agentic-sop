# T011 --- Structured Self/Ponytail Review

## Status

DONE

## Objective

Add a deterministic, bounded **review boundary** that evaluates an
implementation before remote CI, using the T009 `REVIEW` agent capability.

The review is structured: the agent returns findings, not prose, and SOP (not
the model) decides whether a finding blocks progress.

``` text
LOCAL_TESTS_PASS
      │
      ▼
   Review  ── no blocking findings ──▶ pass
      │
      └── blocking findings ──▶ Fix ──▶ re-verify tests ──▶ Review (bounded)
```

## Dependencies

- T009 --- agent capability boundary (`REVIEW`)
- T010 --- TDD task runner
- T008 --- configurable test runner

## Scope

Add `internal/review` with:

- `Severity` (`INFO`, `LOW`, `MEDIUM`, `HIGH`, `CRITICAL`);
- `Finding` (severity, title, detail, file, line, suggestion);
- `Report` (summary + findings) with a deterministic `Blocking(threshold)`;
- `Provider` interface plus an agent-backed provider (`REVIEW` capability,
  JSON findings);
- a bounded `Loop` that re-reviews after each fix and stops after a configured
  number of rounds.

Review criteria (carried to the agent): acceptance criteria, correctness,
missing tests, edge cases, error handling, architecture boundaries,
unnecessary complexity, security concerns, documentation impact.

## Rules

- Only the configured threshold decides blocking; the model does not decide
  whether it "passes".
- Findings are parsed deterministically from the agent's JSON; malformed output
  is an error, not a silent pass.
- The loop is bounded; exhaustion is returned as an error (the caller decides
  the terminal workflow state).
- The provider must not own workflow state or mutate the working tree; fixes go
  through an explicit `Fixer` port.
- No network/model is required by tests.

## Tests

Use fake providers/fixers/testers. Cover: no findings → pass; a finding below
the threshold → pass; a finding at/above the threshold → fix → re-review →
pass; malformed provider output → error; bounded exhaustion → error;
cancellation → context error; the agent is only asked for `REVIEW`.

## Acceptance Criteria

- [x] `Severity`/`Finding`/`Report` exist with a deterministic blocking rule.
- [x] An agent-backed `Provider` requests the `REVIEW` capability.
- [x] Malformed agent output is an error.
- [x] The review loop is bounded and reports exhaustion.
- [x] Fixes go through an explicit port; the provider owns no state.
- [x] Cancellation propagates.
- [x] Tests require no model/network.
- [x] `make check` passes.

## Git

Branch: `task/T011-self-review`
Commit: `task(T011): add structured review boundary`
PR: `[Task T011] Add structured self review`

## Out of Scope

OCR integration (T012), commit/PR/CI, state transitions owned by the completion
loop (T018).
