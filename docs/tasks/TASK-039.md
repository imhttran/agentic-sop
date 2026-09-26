# T039 --- Run Report and PR Output

> Implements plan `PLAN-JEV.md` T041 (PR review output) and the informational
> half of T040 (GitHub Actions mode); complements T025 (run report).

## Status

DONE

## Objective

Publish concise, CI-friendly results from a completed run:

```text
sop report [run-id]   →  Run / Stage / Provider / Gate
                         Validation: <STATUS> <CATEGORY> <command>
                         Review findings by severity
```

The run already records everything (build/test/lint, findings, gate) in
`.agent-sdlc/runs/<id>/report.json`; `sop report` renders the short form a PR
comment needs, so a CI job can run validation and review and publish one summary.

## Dependencies

- T025 (run report artifacts), T030 (validation), T031 (review), T032 (run)

## Scope

- `internal/cli/report.go`: `sop report`, latest-run selection, concise renderer,
  findings-by-severity.
- `internal/cli/cli.go`: dispatch and help.
- `internal/cli/cli_test.go`: report tests.

## Rules

- Informational only: `sop report` never gates a merge (the merge gate owns that)
  and never mutates state.
- With no argument it reports the most recent run (by `report.json` mtime); with
  an id it reports that run. A missing run or report is a clear error.
- Output is the plan's concise shape: per-check validation lines and a finding
  count by severity, plus the final gate.

## Tests

A seeded run directory renders the summary (run id, gate, a validation line, a
severity count); with no runs at all the command errors with “no runs found”.

## Acceptance Criteria

- [x] `sop report` prints a concise summary of a run (latest by default).
- [x] Validation results and findings-by-severity are reported.
- [x] The command is informational and never merges or mutates state.
- [x] Missing runs/reports fail clearly.
- [x] `make check` passes.

## Git

Branch: `task/T039-run-report-command`
Commit: `task(T039): add report command`
PR: `[Task T039] Add report command`

## Out of Scope

Posting the summary as a PR comment (the GitHub adapter would do that); making AI
review a mandatory CI gate.
