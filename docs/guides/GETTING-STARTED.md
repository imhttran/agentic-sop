# Getting Started

This guide takes you from nothing to a first `sop run`: install the CLI, install
the bundled end-to-end agent skill, and point SOP at a planning source.

SOP is a workflow authority that runs locally. It is not a library you import
into your application, and it is not copied into the projects it manages — it
operates on a project from the outside. The project-side path (state, config,
PRD, plan, tasks) lives in [PROJECT-SETUP.md](PROJECT-SETUP.md).

## Requirements

- Go, to build and install the CLI
- Git
- an AI command or harness for planning

SQLite support comes from the Go SQLite driver, so there is no separate database
service to run. How SOP selects a harness, provider, and model is described in
[../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md).

## Clone and install the CLI

```bash
git clone https://github.com/imhttran/agentic-sop.git
cd agentic-sop
go install ./cmd/sop
```

The installed binary lands in your Go binary directory, which needs to be on your
`PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Add that line to your shell configuration to make it permanent. Verify the
installation:

```bash
sop version
```

Once installed, `sop` runs from any project directory.

## Install script

```bash
./scripts/install.sh
```

This installs the `sop` CLI (via `go install ./cmd/sop`). The `sop-end-to-end`
agent skill is provided as a global Zed skill at `~/.agents/skills/sop-end-to-end`,
so an agent can drive the workflow end to end in any project. `make install` runs
the same script.

To also get the governed SOP commands in Zed's agent (`/sop`, `/sop-prompt`,
`/sop-plan`, `/sop-review`, `/sop-diagnose`, `/sop-test`, `/sop-implement`), run:

```bash
make install-skills
```

Each command is a thin alias over `sop prompt`; see
[ZED-SKILLS.md](ZED-SKILLS.md).

## One-command run

If the project has a `docs/PLAN.md` (preferred) or a `docs/PRD.md`, you can
usually just run:

```bash
sop run
```

`sop run` discovers the planning source, initializes SOP state if needed,
compiles or generates the machine plan, creates the tasks, and executes the task
graph — idempotently. Running it again resumes; completed work is never
restarted.

You can also name a specific plan or a single task:

```bash
sop run docs/PLAN-Hardening.md   # execute a specific PLAN end to end
sop run PLAN-Hardening.md        # resolves to docs/PLAN-Hardening.md
sop run --task TASK.md           # run one task through the local lifecycle
```

Several plans can coexist under `docs/`, and each carries its own identity
(source path, `source_sha256`, plan id), so SOP never mixes tasks from two plans.
The explicit, lower-level commands (`sop init`, `sop plan`, `sop tasks`,
`sop status`) stay available for debugging or step-by-step control; see
[PROJECT-SETUP.md](PROJECT-SETUP.md).

## Quick start against another project

Point SOP at the project you want it to manage and run the single command:

```bash
cd ~/workspace/projects/book-rag
sop run
```

Conceptually:

```text
docs/PLAN.md  (or docs/PRD.md)
  │
  ▼
sop run
  │
  ├────────→ .agent-sdlc/plan.json
  ├────────→ Task DAG (SQLite)
  └────────→ execute → validate → review → gate
```

Generated artifacts stay out of the project root: machine state and run reports
live under `.agent-sdlc/` (which ignores itself for Git), and a plan generated
from a PRD is written to `docs/reports/<plan-id>.md`.

## Where to go next

- [PROJECT-SETUP.md](PROJECT-SETUP.md) — the explicit setup path: `sop init`,
  PRD, agent configuration, `sop plan`, `sop tasks`, and inspection.
- [../reference/CLI.md](../reference/CLI.md) — command reference.
- [../reference/CONFIGURATION.md](../reference/CONFIGURATION.md) — configuration
  keys and precedence.
- [../specs/AGENT-PROVIDER.md](../specs/AGENT-PROVIDER.md) — harness, provider,
  and model behaviour.
