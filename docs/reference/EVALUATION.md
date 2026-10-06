# Agentic Evaluation Reference

**Type:** Descriptive reference

The deterministic evaluation harness for Phase 7 — Agentic Reliability &
Evaluation. An evaluation compares a completed run's canonical structured trace
(`trace.json`, see [CLI.md](CLI.md)) against a fixture and reports PASS or FAIL
with actionable diagnostics.

> **Evaluations observe completed run evidence. They do not participate in
> execution or lifecycle decisions.** Nothing in the routing, lifecycle, retry,
> budget, human-boundary, or termination path reads an evaluation result.

## Contract

A **fixture** is a JSON file describing the observable properties a run's trace
must satisfy. Every property is optional: a fixture asserts only what it names,
and an unknown property is an explicit error.

```json
{
  "name": "implementation-no-progress",
  "description": "Repeated activity with no repository mutation blocks for operator intervention.",
  "expected": {
    "execution": { "capability": "implement", "provider": "ollama" },
    "progress": {
      "discovery": { "at_least": 1 },
      "repository_mutations": { "equal": 0 },
      "verification": { "equal": 0 },
      "state_transitions": { "at_least": 1 }
    },
    "termination": {
      "kind": "NO_PROGRESS",
      "disposition": "BLOCK",
      "retryable": false,
      "human_required": false,
      "diagnostic_contains": "IMPLEMENT_NO_PROGRESS"
    }
  }
}
```

Counts use `{"equal": N}`, `{"at_least": N}`, or `{"at_most": N}`. `retryable` is
derived from the recorded disposition by the authoritative failure rule, not stored
separately.

## Layout

```text
evals/
  implementation/   success.expect.json, no-progress.expect.json
  verification/     failure.expect.json
  human-boundary/   destructive.expect.json
  progress/         repeated-discovery.expect.json
```

Future categories (planning, routing, termination, …) add directories without
changing the evaluator.

## Package

`internal/eval` provides the evaluator:

- `LoadFixture` / `ParseFixture` — parse a fixture (unknown fields rejected).
- `Evaluate(trace, fixture) Result` — pure, deterministic, model-free; returns
  `Passed` and per-field `Diagnostics` (`field`, `expected`, `actual`, `message`).

The regression suite in `internal/cli` runs a real lifecycle and evaluates the real
`trace.json`; the unit tests in `internal/eval` evaluate constructed traces. No
evaluation requires a model, a provider, or network access.

## Failure → regression workflow

```text
real failure → capture trace → minimize → write a fixture → reproduce FAIL →
fix the harness → fixture PASSes → permanent regression coverage
```

Trace minimization and fixture generation are manual today; automating them is
future work (see [BACKLOG.md](../plans/BACKLOG.md)).

## Related

- [CLI.md](CLI.md) — `trace.json` and the `sop report` Trace/Progress summary.
- [PROJECT-STATUS.md](PROJECT-STATUS.md) — the Phase 7 status.
