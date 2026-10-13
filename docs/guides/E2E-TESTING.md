# End-to-End Testing

**Type:** Guide

This guide explains how to run, debug, and maintain the browser end-to-end (E2E)
tests in `tests/e2e/`. They are development-only: the Go build and the Go tests
never require Node. For the Go tests, see [DEVELOPMENT.md](DEVELOPMENT.md).

## Overview

The controller's Go tests exercise the HTTP handlers and the SOP integration in
process, against fake SOP state. They cannot tell you whether a page actually
renders in a browser, whether a mobile layout overflows, or whether clicking an
approval control really drives SOP to a recorded decision.

Playwright drives a real browser (WebKit in an iPhone viewport) against a running
controller and real disposable SOP projects, so the E2E suite covers the layer the
Go tests cannot: rendering, layout, interaction, and the end-to-end effect of a
decision on SOP's authoritative state. The two suites complement each other — the
Go tests are fast and exhaustive over the handler surface; the E2E tests are few
and deliberate over the workflows a person actually uses.

## Prerequisites

- **Node.js and npm** (developed against Node 26).
- **The `sop` CLI** on `PATH` (or `SOP_BIN`). The approval fixtures drive the real
  SOP CLI, so the suite cannot create a gate without it.
- **The Go toolchain.** The suite builds the controller from source
  (`go build ./cmd/sop-controller`) before each run.
- **The WebKit browser** for Playwright (installed once, below).

Dependencies are development-only and pinned in `package.json`
(`@playwright/test` 1.64.0). They are never needed to build or run the controller.

## Quick start

One-time setup:

```bash
npm install
npx playwright install webkit
```

Run the suite:

```bash
npm run test:e2e          # the whole suite
npm run test:e2e:ui       # Playwright's interactive UI (watch, time-travel, re-run)
npm run test:e2e:headed   # run with a visible browser
```

Run a single file, or filter by title, with the runner directly:

```bash
npx playwright test decisions.spec.ts
npx playwright test --grep "Approve"
```

## Test coverage

The suite has one project (`webkit-iphone`, the iPhone 13 viewport) and three spec
files — 13 tests:

- `tests/e2e/approvals.spec.ts` — the cross-project approvals inbox (6 tests):
  renders on a phone viewport with no horizontal overflow; a pending gate shows
  enough context to decide; the mobile form inputs and controls stay usable;
  **Approve** submits and updates authoritative SOP state (`APPROVED`); **Decline**
  submits and updates authoritative SOP state (`DECLINED`); a stale decision (a gate
  resolved out-of-band) is reported as a conflict, not a 500.
- `tests/e2e/decisions.spec.ts` — the project "Human decisions" panel (2 tests): a
  project with a pending gate shows the gate and its approve/decline controls; a
  reported-empty listing reads "none" and never the "not run" absence label.
- `tests/e2e/security.spec.ts` — authentication and CSRF (5 tests): an
  unauthenticated request is rejected (401), a wrong token is rejected (401), and
  the access token is accepted (200); a state-changing POST without a CSRF token is
  rejected (403), and with a wrong CSRF token is rejected (403).

The tests assert on accessible roles, labels, and text, and they verify SOP's
authoritative record (`sop approval <task-id>`), not just the screen.

## Test environment

Each run is self-contained; nothing is left behind and no developer or production
project is touched.

**Disposable SOP projects.** `global-setup` creates five throwaway SOP projects
under the ignored `.e2e/` directory:

| Fixture | Purpose |
| --- | --- |
| `approve` | a pending gate for the approve test |
| `decline` | a pending gate for the decline test |
| `stale` | a pending gate the stale test resolves out-of-band |
| `readonly` | a pending gate read (never decided) by the render/context tests |
| `empty` | a project with no gate, so the decisions panel reports "none" |

Each approval is a **genuine gate**: the fixture runs `sop init` and then `sop run`
with `SOP_AGENT_COMMAND` pointed at `tests/e2e/fixtures/fake-agent.sh`, a small
deterministic agent double that returns a one-stage plan and then a `needs_human`
boundary. SOP classifies it and records a real approval request. SOP state is never
mocked and no model is called.

**The controller process.** `global-setup` builds the controller, allocates a free
loopback port, and starts it in network mode with an **isolated random token** and
`SOP_CONTROLLER_ADDR=127.0.0.1:<port>`. It runs from `.e2e/`, so the repository's own
`.env` (developer credentials) is never loaded. The setup waits for `/healthz`, then
writes the base URL, token, and project ids to `.e2e/state.json`, which the tests
read. `global-teardown` stops the controller by PID and removes `.e2e/`.

**Ignored artifacts.** `node_modules/`, `test-results/`, `playwright-report/`, and
`.e2e/` are gitignored.

## Debugging

On failure, Playwright writes a screenshot to `test-results/`, and a trace on the
first retry (see `playwright.config.ts`). Inspect a trace with:

```bash
npx playwright show-trace test-results/<test-folder>/trace.zip
```

The trace shows the DOM snapshot, console, network, and time travel for every step.
For interactive exploration, `npm run test:e2e:ui` re-runs tests and lets you step
through each action; `npm run test:e2e:headed` shows the browser while it runs.

When a test fails, read the failing assertion first: the tests assert on visible text
and roles, so a failure usually means the rendered page or SOP state changed.

## Test maintenance

Add or update a regression test when browser behavior changes:

- a new or changed route, template, or control (a rendered page, a button, routing);
- a change to the approval inbox or the decisions panel;
- a bug fix whose regression the current suite would not catch.

Guidelines:

- Prefer accessible roles, labels, and text over CSS selectors and fixed delays;
  rely on Playwright's auto-waiting `expect` assertions.
- Verify backend state for a state-changing workflow, not just the screen.
- Preserve valid assertions. Never delete or weaken a test merely to make a run pass;
  when a test is wrong, correct the behavior it encodes.
- Keep the suite small and deliberate. It does not try to cover every handler — the
  Go tests do that.

## SOP skill integration

With the SOP agent skills installed, `/sop-browser-test` wraps this suite and
discovers its own configuration and commands:

- `/sop-browser-test` — run the project's existing browser tests.
- `/sop-browser-test --mobile` / `--approvals` — run the matching tests (this suite
  declares no tags, so these map to the whole suite).
- `/sop-browser-test --audit` — from the current Git change, map affected routes,
  templates, and handlers to the suite and report coverage gaps (`NO TEST`), without
  modifying anything.
- `/sop-browser-test --update` — add or update the persistent tests for a confirmed
  UI change, then re-run them.
- `/sop-end-to-end` — when a plan's work changes UI behavior, it runs the affected
  browser tests and, for confirmed behavior changes, updates them in the same run.

The skill is the entry point for an agent; this guide is the entry point for a
developer running the suite by hand.

## Limitations

- **WebKit emulation is not real iPhone Safari.** It does not reproduce iOS-specific
  WebKit quirks, on-device rendering or touch behavior, or a physical device's
  networking. A green run is strong regression coverage, not proof that a real
  handset behaves identically.
- **One run at a time.** The suite owns the fixed `.e2e/` state directory; do not run
  two suites concurrently.
- **Environment-dependent.** The suite needs the `sop` CLI, the Go toolchain, and the
  installed WebKit browser; a missing one is a blocker, not a skipped pass.
