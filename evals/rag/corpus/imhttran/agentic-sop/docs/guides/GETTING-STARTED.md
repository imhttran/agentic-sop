# Getting Started

This guide takes you from nothing to a first `sop run`: install the CLI, install
the bundled agent commands, and point SOP at a planning source.

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
./install.sh
```

The installed binary lands in a user-writable bin directory (`--bin-dir`, else the Go
toolchain's bin directory), which needs to be on your `PATH`:

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
```

Add that line to your shell configuration to make it permanent. Verify the
installation:

```bash
sop version
```

Once installed, `sop` runs from any project directory.

## Install agent commands

Installation options, PATH precedence, update/removal, and platform differences
are owned by [INSTALLATION.md](INSTALLATION.md). SOP ships agent skills that expose
the governed path as `/sop` commands in supported coding agents. Once the CLI works,
install the integrations for the agents you use:

```bash
./install.sh --skills zed     # the /sop* commands in Zed
./install.sh --skills claude  # the same commands in Claude Code
./install.sh --plugin claude  # the Claude Code plugin
./install.sh --all            # the CLI + every present agent's skills + the plugin
```

```text
/sop-review review internal/provider for architectural problems
/sop-plan plan a cache layer for provider model discovery
/sop-diagnose explain why the current tests are failing
/sop-test design regression tests for this behavior
/sop-implement add caching to provider model discovery
```

Each command is a thin alias over `sop prompt --capability ...`: it picks a capability
and calls SOP, which still owns routing, provider selection, validation, and approval.
`/sop-implement` is the only mutating command and runs the governed implementation
lifecycle. Guides: [INSTALLATION.md](INSTALLATION.md),
[ZED-SKILLS.md](ZED-SKILLS.md), and
[CLAUDE-SKILLS.md](CLAUDE-SKILLS.md); the plugin is
[CLAUDE-PLUGIN.md](CLAUDE-PLUGIN.md).

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
