# T027 --- Task File Loader

> Implements plan `PLAN-wrapup.md` T006 (Task Loader).

## Status

DONE

## Objective

Let the workflow start from a hand-written Markdown task file, not only a
generated plan:

```text
TASK.md  ──parse──>  Spec{ID, Title, Description, Requirements,
                           AcceptanceCriteria, Constraints, Dependencies}
             │
             └──normalize──>  planner input
```

## Dependencies

- Stage 4 (Planner, T004)
- T026 (configured agent selection) for `sop plan`

## Scope

- `internal/taskfile/taskfile.go`: `Spec`, `Parse`, `Load`, `Render`.
- `internal/taskfile/taskfile_test.go`: parser and round-trip tests.
- `internal/cli/plan.go`: `sop plan [TASK.md]` — a file argument is parsed and
  normalized before planning; no argument keeps reading `PRD.md`.
- `internal/cli/cli.go`: help text updated.
- `internal/cli/cli_test.go`: task-file planning, missing file, too many args.

## Rules

- Plain Markdown stays usable: only an empty file is an error; any other shape
  degrades to Title + Description.
- Recognized sections are extracted case-insensitively with aliases
  (`Objective`/`Description`/`Summary`, `Rules`/`Constraints`,
  `Dependencies`/`Depends on`); unknown sections are ignored, not errors.
- Bullets (`-`, `*`, `+`, `1.`, `2)`) and checkboxes (`[ ]`, `[x]`) are stripped
  from list items.
- A leading `<id> — <title>` heading yields the ID; `## ID` / `## Title`
  sections override it.
- `sop plan` with no argument behaves exactly as before.

## Tests

A full task document parses into the expected ID, title, description,
dependencies, constraints, and checkbox-stripped criteria; plain Markdown
degrades to title + description; a title falls back to the first line;
`## ID`/`## Title` override the heading; every bullet variant is handled; empty
files error; `Load` reads a file and errors on a missing one; `Render`
round-trips and omits empty sections; the CLI plans from a task file, reports a
missing file, and rejects too many arguments.

## Acceptance Criteria

- [x] A task file's ID, title, description, requirements, acceptance criteria, and constraints are parsed.
- [x] Plain Markdown remains usable; only empty files are rejected.
- [x] `sop plan TASK.md` normalizes the file before planning; `sop plan` is unchanged.
- [x] `make check` passes.

## Git

Branch: `task/T027-task-file-loader`
Commit: `task(T027): add Markdown task-file loader`
PR: `[Task T027] Add Markdown task-file loader`

## Out of Scope

Persisting a task file as a domain task and executing it (`sop run`); task-file
front matter or schemas beyond Markdown sections.
