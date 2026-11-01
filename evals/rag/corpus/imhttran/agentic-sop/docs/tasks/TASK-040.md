# T040 --- Evaluation Harness

> Implements plan `PLAN-JEV.md` T045 (harness benchmark suite) and the mechanism
> for T046 (review evaluation).

## Status

DONE

## Objective

Run a corpus of tasks through the harness and aggregate the metrics a benchmark
needs:

```text
sop eval DIR   →  per-case gate, fix cycles, findings, duration
                  totals: passed / failed / needs_human / errored, success %
```

Task success, fix cycles, findings, and elapsed time are measured (plan T045).
Comparing review engines (plan T046) is then running the same corpus under
different configurations and comparing the metrics.

## Dependencies

- T032 (run lifecycle), T033 (fix loop), T030/T031 (validation/review)

## Scope

- `internal/eval/eval.go`: `Case`, `Outcome`, `Metrics`, `Run`, `SuccessRate`.
- `internal/eval/eval_test.go`: aggregation and cancellation tests.
- `internal/cli/eval.go`: the `sop eval DIR` command; runs each `*.md` task via
  the shared lifecycle into `eval-<name>` runs and prints a metrics table.
- `internal/cli/cli.go`: dispatch and help.
- `internal/cli/cli_test.go`: eval tests.

## Rules

- Control-plane only: `eval.Run` never calls a model; the caller supplies the
  runner (here, the run lifecycle), so the harness is measured as it actually
  behaves.
- A run error is counted as `errored`, distinct from a gate failure.
- Cancellation stops the corpus; already-run cases are still aggregated.
- Metrics are deterministic given the runner.

## Tests

Aggregation counts pass/fail/needs-human/error and sums findings, cycles, and
duration; `SuccessRate` is passed/total (0 for an empty corpus); cancellation
runs nothing. CLI: a one-case corpus passes and reports `success: 100%`; a
directory with no task files errors; a missing argument is a usage error.

## Acceptance Criteria

- [x] `sop eval DIR` runs a corpus and reports aggregate benchmark metrics.
- [x] Metrics include success, fix cycles, findings, and elapsed time.
- [x] The harness is measured through the real run lifecycle (no reimplementation).
- [x] Cancellation and empty corpora are handled.
- [x] `make check` passes.

## Git

Branch: `task/T040-evaluation-harness`
Commit: `task(T040): add evaluation harness`
PR: `[Task T040] Add evaluation harness`

## Out of Scope

A curated public benchmark corpus; token/cost capture (providers do not report
it); the Jev-vs-deterministic comparison (plan T037), which needs real runs.
