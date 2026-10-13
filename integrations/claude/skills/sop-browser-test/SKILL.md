---
name: sop-browser-test
description: |-
  Detect UI changes, analyze browser-test coverage, and run or maintain a project's
  persistent Playwright tests.
  /sop-browser-test, /sop-browser-test --mobile, /sop-browser-test --approvals,
  /sop-browser-test --audit, /sop-browser-test --update, and combined filters.
  Use for "run the browser tests", "verify the UI", "what browser tests are missing",
  "check the mobile layout", or "update the approval tests" when the operator wants a
  project's version-controlled Playwright suite analyzed, executed, or maintained against
  the real application. It works from the project's own suite; it never fabricates a suite,
  never replaces persistent tests with ad-hoc browsing, and never replaces SOP as the
  workflow authority.
disable-model-invocation: false
---

# SOP Browser Test

Analyze, run, and maintain a project's **persistent, version-controlled** browser tests.
This skill is a thin executor and maintainer over the project's existing Playwright Test
suite: it discovers the project's own configuration and commands, detects which UI
behavior a change touches, runs the relevant tests, and updates them when that behavior
changes. It introduces no framework, no runner, and no server, and it never reports an
unrun or skipped suite as a pass.

Arguments are **instructions to this skill**, never SOP CLI flags:

- `/sop-browser-test` — run the project's existing browser tests.
- `/sop-browser-test --mobile` — run the mobile-specific tests.
- `/sop-browser-test --approvals` — run the approval-workflow tests.
- `/sop-browser-test --audit` — report browser-test coverage gaps for the current change (read-only).
- `/sop-browser-test --update` — add or update persistent tests for the confirmed UI change.
- Combined filters, for example `/sop-browser-test --mobile --approvals`.

## DISCOVER

Find the project's real setup; assume nothing and invent no option.

- Read the package manifest `scripts` (for example a `test:e2e` script), any
  `playwright.config.*`, the `tests/` tree, and the project's README or getting-started
  guide.
- Record the suite's own filters: the `projects` it declares, the tags it uses (a
  `test.describe('...', { tag: '@mobile' })` marker, or an `@approvals` tag), and the
  runner's matching flags (a project name, a tag or name `--grep`, `--ui`, `--headed`).
- Find the application's URL, startup procedure, and any test credential the suite needs,
  from the project's documentation. Never invent a URL, a port, or a secret.
- If the project is SOP-governed, its validation configuration declares the deterministic
  checks; the browser suite is additional, never a replacement for them.

If the project has **no Playwright suite**, STOP and report that plainly. Do not run
ad-hoc browser exploration in its place, and do not fabricate a suite.

## PREFLIGHT

- Confirm the project's browser-test dependencies are installed and a browser is
  available. If the runner (`@playwright/test`) or its browsers are missing, report a
  **blocker**; do not install dependencies without the operator's authorization.
- The **Playwright MCP** (the `browser_*` tools) is optional: use it to explore a screen
  or diagnose a failure, never instead of the persistent suite.
- Map `--mobile` and `--approvals` to the project's **own** filters discovered above. If
  the project declares no such filter, say so rather than guessing.
- Never expose credentials. Default to loopback; never test production without explicit
  authorization.
- If `sop` is not on `PATH`, the approval-workflow state verification is `NOT RUN`, not a
  pass: report it and fail closed rather than claiming success.

## RUN

Run the project's documented command, scoped to the request; a filter is the suite's
own, never invented. A project that ships its suite in package scripts exposes a test
script (commonly `test:e2e`); otherwise use the runner directly:

```bash
npm run test:e2e
npx playwright test --grep @mobile
npx playwright test --grep @approvals
```

- `--mobile` selects the mobile tags or projects; `--approvals` the approval tests; both
  together select the intersection.
- Keep a passing run quiet; capture the configured `screenshot` and `trace` on failure.
- Read the runner's own counts. A test that did not run — skipped, filtered out, or
  blocked by a missing dependency — is **not a pass**.

## VERIFY

- Trust the runner's result, not the UI's appearance alone.
- On failure, open the captured trace or screenshot with the Playwright MCP to identify
  the cause, then report the failing test, its assertion, and the evidence.
- For a state-changing workflow (the approval inbox is the canonical example), verify the
  **backend** state, not just the screen. Read SOP's authoritative record:

  ```bash
  sop approvals --json
  sop approval <task-id>
  ```

  Require `Approval: APPROVED` or `Approval: DECLINED` as applicable, with the recorded
  provenance. A decision SOP refuses is a conflict, never a pass.

## CHANGE ANALYSIS

Scope the analysis to the change, never the whole application.

- Start from Git: the changed and staged files, relative to the project's base or the
  active work. Fall back to the files the operator names.

  ```bash
  git status --porcelain
  git diff --name-only
  ```

- Map those files to UI behavior: routes, templates, components, and the handlers behind
  them. Trace only the workflows the changes affect.
- Compare each changed behavior with the suite: which test covers it, which assertion
  would catch a regression, and where a route, template, or control has no test.
- Classify every changed behavior as **new** (functionality with no test), **changed**
  (behavior whose test is now stale), **covered** (unchanged behavior, already tested), or
  **gap** (a regression risk with no test).

Keep it lightweight: work from the changed files and the affected workflows by default.
Reserve a full-suite coverage audit for an explicit `--audit`, or a substantial UI change
(a route, template, or workflow added, removed, or redefined).

## AUDIT (--audit)

Report the coverage picture for the current change; modify nothing.

- For each changed UI behavior, give the route, template, or component, the covering test
  (its path and title) or `NO TEST`, and the regression risk of the gap.
- Summarize new functionality, changed behavior whose tests are stale, and uncovered
  regression coverage.
- Write nothing and run nothing destructive. It is fine to run the existing suite to
  confirm the current state, and to report that run.

## UPDATE (--update)

Bring the persistent tests in step with the confirmed UI change.

- Act on the analysis: add tests for new functionality, and update the tests whose
  behavior changed. Touch only the affected files.
- Preserve valid existing assertions. Never delete or weaken a valid assertion merely to
  make a run pass; when a test is wrong, correct the behavior contract it encodes and say
  why.
- Prefer accessible roles, labels, and text, and deterministic auto-waiting assertions
  over brittle selectors, CSS paths, and fixed delays.
- Re-run the affected tests after editing and report the result. Leave unrelated tests
  untouched.
- Keep the tests evolving with the application while this skill stays stable.

## CLEANUP

The project's suite owns its own fixtures and teardown. Remove only resources this run
created; never delete or alter operator-owned state or a pre-existing server.

## REPORT

Return a concise, evidence-backed result:

- Suite: its path, the command run, and the filters applied.
- Result: passed, failed, and skipped counts, and the runner's exit status.
- Coverage: the changed behaviors analyzed, the tests added or updated, and the remaining
  gaps (`NO TEST`), with the regression risk of each.
- Evidence: the failing test and its assertion, with the screenshot or trace path.
- Backend verification: the authoritative state transition for any state-changing flow.
- Blockers: a missing runner, browser, or dependency; or a project with no browser suite.
- Limitations: what the browser could not verify, for example a real physical device.
