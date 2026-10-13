---
name: sop-browser-test
description: |-
  Discover, run, and maintain a project's persistent Playwright tests.
  /sop-browser-test, /sop-browser-test --mobile, /sop-browser-test --approvals,
  /sop-browser-test --update, and combined filters (--mobile --approvals).
  Use for "run the browser tests", "verify the UI", "check the mobile layout", or
  "update the approval tests" when the operator wants a project's version-controlled
  Playwright suite executed or maintained against the real application. It runs and
  maintains the project's own suite; it never fabricates a suite, never replaces
  persistent tests with ad-hoc browsing, and never replaces SOP as the workflow authority.
disable-model-invocation: false
---

# SOP Browser Test

Run and maintain a project's **persistent, version-controlled** browser tests. This
skill is a thin executor and maintainer over the project's existing Playwright Test
suite: it discovers the project's own configuration and commands, runs the relevant
tests, and updates them when UI behavior changes. It introduces no framework, no runner,
and no server, and it never reports an unrun or skipped suite as a pass.

Arguments are **instructions to this skill**, never SOP CLI flags:

- `/sop-browser-test` — run the project's existing browser tests.
- `/sop-browser-test --mobile` — run the mobile-specific tests.
- `/sop-browser-test --approvals` — run the approval-workflow tests.
- `/sop-browser-test --update` — bring the tests up to date with changed UI behavior.
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

## UPDATE (--update)

Bring the persistent tests in step with changed UI behavior.

- Identify the affected behavior and the tests that cover it.
- Update or add only those tests. Preserve valid existing assertions; do not rewrite
  unrelated tests.
- Prefer accessible roles, labels, and text, and deterministic auto-waiting assertions
  over brittle selectors, CSS paths, and fixed delays.
- Keep the tests evolving with the application while this skill stays stable.
- Re-run the affected tests after editing, and report the result.

## CLEANUP

The project's suite owns its own fixtures and teardown. Remove only resources this run
created; never delete or alter operator-owned state or a pre-existing server.

## REPORT

Return a concise, evidence-backed result:

- Suite: its path, the command run, and the filters applied.
- Result: passed, failed, and skipped counts, and the runner's exit status.
- Evidence: the failing test and its assertion, with the screenshot or trace path.
- Backend verification: the authoritative state transition for any state-changing flow.
- Blockers: a missing runner, browser, or dependency; or a project with no browser suite.
- Limitations: what the browser could not verify, for example a real physical device.
