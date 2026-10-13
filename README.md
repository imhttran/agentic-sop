# SOP Controller

A local-first web dashboard for **SOP**. It gives a human a friendly
operational view of SOP projects — task state, execution progress, review
results, CI/validation state, handoff context, and failures — and exposes safe
SOP commands.

> **SOP stays the workflow authority.** The dashboard observes and commands SOP;
> it never keeps a second source of truth for task state.

## High-Level Architecture

```text
Browser
   |
   v
Go HTTP server  (html/template + HTMX + CSS)
   |
   v
SOP application/API boundary   internal/sopclient
   |
   v
SOP authoritative state        <project>/.agent-sdlc/state.db  (read-only)
                               sop CLI                        (commands)
```

Two planes, one authority:

- **`agentic-sop` / SOP** — automation and execution plane. SOP owns workflow
  state, legal state transitions, scheduling, gates, and human-approval
  boundaries.
- **`sop-controller`** — human visibility and control plane. It reads SOP's
  persisted state and delegates every action to SOP.

Read more: [docs/architecture/OVERVIEW.md](docs/architecture/OVERVIEW.md) and
[docs/architecture/SOP-BOUNDARY.md](docs/architecture/SOP-BOUNDARY.md).

## Requirements

- **Go** (1.24+). That is it at build/run time.
- The **`sop` CLI** on `PATH` (or `SOP_BIN`) for the Run/Resume/Validate/Review
  commands. Read-only views work without it.

No Node, no npm, no PostgreSQL. Server-rendered HTML plus HTMX — no SPA.

## Quick Start

```bash
go run ./cmd/sop-controller     # foreground
# → http://127.0.0.1:8080
```

By default it reads SOP state from the current directory
(`./.agent-sdlc/state.db`). Point it at other projects with
`SOP_CONTROLLER_PROJECTS=...`, or let it discover every SOP project under a
workspace root with `SOP_CONTROLLER_WORKSPACES=...`.

Full setup, including phone access: [docs/guides/GETTING-STARTED.md](docs/guides/GETTING-STARTED.md).

## Basic Run Commands

```bash
make start     # build + run in the background
make status    # is it up? (also pings /healthz)
make logs      # tail the background log
make stop      # shut it down
```

`make help` lists every target. The dashboard itself drives SOP CLI commands
(`run`, `resume`, `validate`, `review`, `report`, `retry`, `reconcile`); see
[docs/reference/CLI.md](docs/reference/CLI.md).

## Basic Workflow

The controller reflects SOP's lifecycle; it does not own it. SOP decides what is
runnable, executes tasks, runs deterministic gates, and opens a human gate only
for a genuine unresolved decision. The controller displays that state and
delegates every action back to SOP.

- Lifecycle and status vocabulary: [docs/specs/WORKFLOW.md](docs/specs/WORKFLOW.md)
- Statuses, run stages, and recovery: [docs/reference/STATUS-AND-RECOVERY.md](docs/reference/STATUS-AND-RECOVERY.md)
- Human gates: [docs/specs/HUMAN-APPROVAL.md](docs/specs/HUMAN-APPROVAL.md)

## Approvals and Mobile Access

Every human approval gate SOP reports is collected at
[`/approvals`](http://127.0.0.1:8080/approvals) — a phone-friendly inbox across
the projects the controller observes. Approve / decline controls delegate to
`sop approve` / `sop decline` (with `--by` / `--note` provenance); the controller
never approves on its own, and viewing the page starts no command. A project
whose SOP approval listing cannot be read is shown as an explicit absence, never
as "no gates". See [docs/guides/APPROVAL-AND-RECONCILE.md](docs/guides/APPROVAL-AND-RECONCILE.md).

To reach the dashboard from a phone on the same network, enable network mode
with an access token (loopback binding is the default; a non-loopback address
without it is refused):

```bash
SOP_CONTROLLER_ALLOW_NETWORK=true \
SOP_CONTROLLER_TOKEN="choose-a-token" \
SOP_CONTROLLER_ADDR=0.0.0.0:8080 \
  go run ./cmd/sop-controller
```

Network mode requires the token, and every state-changing request is `POST` with
a CSRF token. The controller never renders secrets or forwards provider
credentials to SOP. See [docs/specs/SECURITY.md](docs/specs/SECURITY.md) and
[docs/guides/GETTING-STARTED.md](docs/guides/GETTING-STARTED.md#phone--same-network-access).

The task detail view also reports SOP's recorded **model selection** (its class,
locality, and whether a fallback was used) and run usage counts when SOP persisted
them; the controller reports the selection and never chooses a model.

## Browser Tests (Playwright)

Optional, development-only end-to-end tests that drive the real dashboard in a browser
(WebKit, iPhone viewport) against disposable SOP projects. The Go build and tests never
need Node.

One-time setup:

```bash
npm install
npx playwright install webkit
```

Run:

```bash
npm run test:e2e          # the whole suite
npm run test:e2e:ui       # Playwright's interactive UI
npm run test:e2e:headed   # watch the browser
```

Each run builds the controller, allocates a free loopback port, creates disposable SOP
projects with genuine pending approvals, and removes them (and the server) afterwards —
all under the ignored `.e2e/` directory. Nothing is left behind, and no developer or
production project is touched.

On failure a screenshot is kept in `test-results/`, and a trace on the first retry.
Inspect them with `npx playwright show-trace test-results/<test>/trace.zip`.

**Limitations.** WebKit emulating an iPhone viewport is not real iPhone Safari: it does
not reproduce iOS-specific WebKit quirks, on-device rendering or touch behavior, or a
physical device's networking. Treat a green run as strong regression coverage, not as
proof that a real handset behaves identically.

## Documentation

Start at the documentation index: **[docs/README.md](docs/README.md)** — it maps
every document with a one-sentence "when to read it".
Agent routing guide: [AGENTS.md](AGENTS.md).

- **Requirements:** [docs/requirements/PRD.md](docs/requirements/PRD.md)
- **Architecture:** [docs/architecture/](docs/architecture/)
- **Specifications:** [docs/specs/](docs/specs/)
- **Reference:** [docs/reference/](docs/reference/)
- **Guides:** [docs/guides/](docs/guides/)
- **Plans:** [docs/plans/](docs/plans/) (current) and [docs/history/plans/](docs/history/plans/) (completed/superseded)
- **Evidence reports:** [docs/reports/](docs/reports/)
- **History:** [docs/history/](docs/history/)

The plan currently executing is whatever SOP records in
`.agent-sdlc/plan.meta.json` (`sop status` reports it) — not anything named in
this file. Historical artifacts under `docs/history/` are point-in-time and
non-normative.

## Status

Active and completed work is tracked by SOP, not by this README. See
[docs/plans/](docs/plans/) for current planning and [docs/history/](docs/history/)
for completed or superseded work.
