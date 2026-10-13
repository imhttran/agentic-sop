---
name: sop-browser-test
description: |-
  Verify a running web UI with the existing Playwright MCP.
  /sop-browser-test, /sop-browser-test --mobile, /sop-browser-test --approvals,
  /sop-browser-test --mobile --approvals.
  Use for "smoke test the app", "check the UI", "verify the mobile layout", or
  "does the approval page work" when the operator wants real browser verification of
  a running application. A thin verification layer over the Playwright MCP: it
  discovers how the target starts, drives a real browser, checks for errors, verifies
  backend state, and reports evidence. It adds no framework, no server, and no
  orchestration of its own, and it never replaces SOP as the workflow authority.
disable-model-invocation: false
---

# SOP Browser Test

Verify a running web application with the existing **Playwright MCP**. This skill is a
thin verification layer: it discovers how the target project starts, drives a real
browser, checks for errors, and reports evidence. It never plans, implements, or mutates
the target repository, and it never reimplements SOP.

The four usage patterns are **instructions to this skill**, never CLI flags:

- `/sop-browser-test` — a smoke test: the app loads and its main surfaces render.
- `/sop-browser-test --mobile` — the same on a phone-sized viewport, checking responsive layout.
- `/sop-browser-test --approvals` — the approval workflow, when the target supports it.
- `/sop-browser-test --mobile --approvals` — both together.

Scale the test to scope and risk: one surface for a narrow change, the full flow for a
workflow change. A small backend-only change does not need a browser pass.

## DISCOVER

Identify the target application and how it runs; do not invent a URL or a command.

- The target is normally the project Zed is open on. Read its own documentation and
  configuration (a getting-started guide, a README, a Makefile or task runner, a package
  manifest) to learn its URL, port, startup command, and any required test token.
- Determine the authentication the project documents (for example a `?token=` link or a
  cookie) and load it exactly as documented. Never guess a credential and never print one.
- If the target is a SOP project, read its authority state first:

  ```bash
  sop status
  ```

- Prefer an already-running local server. If a health endpoint answers, reuse it rather
  than starting a second instance.

## PREFLIGHT

- Confirm the Playwright MCP is available (the `browser_*` tools). If a check is
  obligatory and the MCP is unavailable, fail closed: report the missing MCP and do not
  report the UI as verified.
- For `--approvals`, confirm `sop` is on `PATH`; if it is not, the approval verification
  is `NOT RUN`, not a pass.
- Default to loopback. Do not test a production or shared remote host without the
  operator's explicit authorization.
- Never render, log, or transmit credentials, tokens, cookies, or secrets.

## RUN

When no healthy server is already running, start a disposable local instance using the
project's documented mechanism, in the background, bound to loopback, with any required
test token supplied from the environment. Follow the project's own guide for the exact
invocation rather than inventing one. Record the exact URL the browser will drive.

## VERIFY

Drive the real application through the Playwright MCP; do not infer rendering from code.

- Navigate, then read the accessibility snapshot and assert on visible text.
- Click, type, and submit real controls; after an action, confirm the page changed.
- With `--mobile`, set a phone viewport (about 390x844) and check the layout is usable:
  no horizontal scrolling, controls reachable, text legible.
- Check for failures: console errors, failed network requests, visible error text, and a
  blank or unchanged page after an action.
- Capture a screenshot for the evidence, and a trace when the MCP supports it.
- For any state-changing action, verify the backend state changed. The UI is not the
  authority.

## APPROVALS

Run this only when the target exposes a human approval surface (for the SOP Controller,
the `/approvals` inbox, backed by `sop approvals --json` and `sop approve`/`sop decline`).

- Read SOP's authoritative gates first:

  ```bash
  sop approvals --json
  ```

- Prepare two separate disposable approvals so Approve and Decline are exercised
  independently, each in its own disposable project or task. Create them only through
  SOP's own lifecycle (drive a disposable fixture to a genuine human gate); never
  fabricate a gate, and never approve or decline the operator's real work.
- In the browser, open the approval surface and confirm each gate shows enough context
  to decide: the target, the gate kind, SOP's reported reason, and the requested time.
- Decide one gate Approve and the other Decline, then confirm the surface reflects each
  decision (the decided gate disappears).
- Verify SOP's authoritative state for each:

  ```bash
  sop approval <task-id>
  ```

  Require `Approval: APPROVED` for the approved gate and `Approval: DECLINED` for the
  declined one, and check that the actor/note the UI sent was recorded. A decision SOP
  refuses (a stale or not-applicable gate) is a conflict, never a pass.

## CLEANUP

Remove only what the test created: stop a server the test started, and delete the
disposable projects or fixtures it created. Leave any pre-existing server and any
operator-owned resource untouched.

## REPORT

Return a concise, evidence-backed result:

- MCP: present and functional, or `UNAVAILABLE` (in which case the browser check is
  failed, never a pass).
- Target: URL, viewport(s), and whether the app was started or reused.
- Surfaces: what rendered, any console or network errors, and the screenshots.
- Approvals: each gate's decision and SOP's authoritative transition (`APPROVED` /
  `DECLINED`) with the recorded provenance.
- Defects: what failed, with exact evidence; fix a genuine defect only when the target's
  own tests cover the change.
- Cleanup: what was torn down, and what pre-existing resource was left alone.
- Limitations: anything the browser could not verify (for example a real physical device).
