# T005A --- Rename Agentic SDLC to SOP

## Status

DONE

## Objective

Rename the product and CLI from **Agentic SDLC / `agent-sdlc`** to **SOP
/ `sop`** while preserving the existing architecture, behavior, workflow
state, and persisted project data.

SOP stands for the project's development philosophy:

> **A standard operating procedure for agentic software development.**

The primary project message is:

> **Agents do the work. SOP controls the process.**

This task is a product, documentation, and CLI rename. It should not
introduce unrelated architectural changes.

------------------------------------------------------------------------

## Dependencies

-   T005 --- Plan → Tasks + Dependency DAG

------------------------------------------------------------------------

## Scope

### Product Rename

Rename user-facing references from `Agentic SDLC` to `SOP`.

Use the project description:

``` text
A standard operating procedure for agentic software development.
```

Use the short description where appropriate:

``` text
Agents do the work. SOP controls the process.
```

### CLI Rename

Rename the executable from `agent-sdlc` to `sop`.

The currently supported commands should become:

``` bash
sop init
sop plan
sop tasks
sop status
sop task <id>
sop version
sop help
```

Behavior must remain equivalent to the existing commands.

Update:

-   CLI entry point
-   CLI help
-   usage output
-   version output
-   tests
-   build scripts
-   Makefile references
-   CI references
-   documentation examples

------------------------------------------------------------------------

## Internal Package Names

Do **not** rename internal packages simply for branding.

Existing domain-oriented names should remain unchanged where applicable,
including:

``` text
planner
taskbuilder
scheduler
store
domain
agent
```

The product is being renamed, not the internal architecture.

------------------------------------------------------------------------

## Runtime State Directory

Do **not** rename `.agent-sdlc/` as part of this task.

Existing projects may already contain:

``` text
.agent-sdlc/state.db
.agent-sdlc/plan.json
```

Changing this directory would introduce a persistence migration and
backward-compatibility problem.

A future task can evaluate migrating `.agent-sdlc/` to `.sop/` with an
explicit migration strategy.

For T005A, existing persisted project state must continue to work
without modification.

------------------------------------------------------------------------

## README

Create or update the README to introduce the project as:

``` text
SOP

A standard operating procedure for agentic software development.

Agents do the work. SOP controls the process.
```

The README should clearly explain that SOP itself is written in Go but
can manage projects written in other programming languages, including
Go, Java, Kotlin, Python, JavaScript/TypeScript, Rust, C/C++, C#, and
mixed-language projects.

SOP should not require the target project to be written in Go.

The README should also explain how SOP is installed once and then used
from another project.

Example:

``` text
workspace/
├── sop/                  # SOP source
└── projects/
    └── book-rag/         # project managed by SOP
```

Typical usage:

``` bash
cd ~/workspace/projects/book-rag

sop init
sop plan
sop tasks
sop status
```

Explain that SOP treats the current working directory as the target
project root.

------------------------------------------------------------------------

## Architecture Documentation

Update user-facing architecture documentation where the old product name
appears.

Preserve the existing architectural principle:

``` text
The LLM creates content.
The application controls the process.
```

The branding version can additionally use:

``` text
Agents do the work.
SOP controls the process.
```

Do not change the underlying architecture as part of the rename.

------------------------------------------------------------------------

## Repository Search

Search the repository for stale references to:

``` text
Agentic SDLC
agent-sdlc
agent_sdlc
```

Review every occurrence.

Replace user-facing and executable-name references where appropriate.

Do not blindly replace persisted paths or identifiers where doing so
could break compatibility.

------------------------------------------------------------------------

## Tests

Update existing CLI tests to use `sop` instead of `agent-sdlc`.

Existing behavioral tests should continue to pass.

Add or update tests where necessary to verify:

-   `sop help`
-   `sop version`
-   `sop init`
-   `sop plan`
-   `sop tasks`
-   `sop status`
-   `sop task <id>`
-   existing `.agent-sdlc` state remains usable
-   command behavior and exit codes remain unchanged

No LLM or network access should be required by automated tests.

------------------------------------------------------------------------

## Acceptance Criteria

The following commands work:

``` bash
sop init
sop plan
sop tasks
sop status
sop task <id>
sop version
sop help
```

The old `agent-sdlc` executable is no longer the primary CLI.

The README identifies the project as **SOP** and explains:

``` text
A standard operating procedure for agentic software development.
```

The README includes:

``` text
Agents do the work. SOP controls the process.
```

The README documents how SOP can manage another project.

The README makes clear that SOP is language-independent even though the
orchestrator itself is written in Go.

Existing `.agent-sdlc/state.db` and `.agent-sdlc/plan.json` continue to
work without migration.

Internal domain package names are not unnecessarily renamed.

Existing workflow behavior remains unchanged.

No existing task state or dependency data is modified by the rename.

All tests pass.

------------------------------------------------------------------------

## Verification

Run:

``` bash
go test ./...
go test -race ./...
CGO_ENABLED=0 go test ./...
make check
```

Search for stale naming:

``` bash
grep -R "Agentic SDLC" .
grep -R "agent-sdlc" .
```

Remaining matches must be reviewed and intentionally retained if they
represent backward-compatible runtime paths such as `.agent-sdlc/`.

------------------------------------------------------------------------

## Open Code Review

After implementation and local tests pass, run Open Code Review against
the task branch changes.

Review for:

-   accidental behavior changes
-   broken CLI references
-   stale executable names
-   broken tests
-   build/CI regressions
-   accidental `.agent-sdlc` migration
-   unnecessary internal package renaming

Fix correctness issues.

Decline style suggestions that introduce unnecessary complexity.

------------------------------------------------------------------------

## Git

### Branch

``` text
task/T005A-rename-to-sop
```

### Commit

``` text
task(T005A): rename Agentic SDLC to SOP
```

### Pull Request

``` text
[Task T005A] Rename Agentic SDLC to SOP
```

------------------------------------------------------------------------

## Out of Scope

The following are explicitly outside this task:

-   `.agent-sdlc` → `.sop` migration
-   scheduler implementation
-   Git automation
-   test-runner implementation
-   agent-harness redesign
-   workflow-state changes
-   SQLite schema changes
-   parallel task execution
-   new model-provider integrations

These should remain separate tasks.

------------------------------------------------------------------------

## Definition of Done

T005A is complete when SOP is the user-facing project and CLI name, the
README explains how to use SOP with another project, existing
`.agent-sdlc` project state remains compatible, all tests and project
checks pass, automated review has been completed, and the changes are
merged to `main`.
