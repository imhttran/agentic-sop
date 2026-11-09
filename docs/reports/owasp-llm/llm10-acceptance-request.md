# Plan 10 — acceptance and live-delivery request

Status: **REQUEST — no acceptance is recorded, no live message is sent, and no SOP
lifecycle transition is claimed.** This document prepares the exact decisions a
designated approver must make. It does not record an approver, a timestamp, or an
outcome.

Related: [llm10-output-handling.md](llm10-output-handling.md),
[llm10-live-delivery.md](llm10-live-delivery.md), the cumulative
[evidence matrix](README.md), the
[gap register](gap-register-20261109.md), and
[PLAN-10-SOP-LLM10-Improper-Output-Handling.md](../../plans/PLAN-10-SOP-LLM10-Improper-Output-Handling.md).

## What is complete

The implementation and deterministic evidence were merged before this request. The
wrap-up re-ran the focused checks at `main` `ed9ec2dc` on Darwin arm64 (Go 1.27.1)
and preserved the original Linux `x86_64` record.

| Task | Criterion | Type | State | Evidence |
| --- | --- | --- | --- | --- |
| OUTPUT-001 | 1 — sink inventory lists origin, parser, validation/encoding, effect, owner | manual | `IMPLEMENTED` | `llm10-output-handling.md` §Sink inventory |
| OUTPUT-001 | 2 — intentional shell execution separated from interpolation | manual | `IMPLEMENTED` | §Intentional shell execution versus interpolation; `internal/toolharness`, `internal/git`, `internal/github` |
| OUTPUT-001 | 3 — terminal controls and owned-renderer fetching identified | manual | `IMPLEMENTED` | §Terminal controls and resource fetching in owned renderers |
| OUTPUT-001 | 4 — report delivered | check | `VERIFIED` | The report is in this repository |
| OUTPUT-002 | 1 — data arguments cannot become shell syntax | check | `VERIFIED` | `TestSplitCommandRejectsShellMetacharacters` |
| OUTPUT-002 | 2 — invalid structured operations cannot partially execute | check | `VERIFIED` | `TestDeniedOperationsCauseNoSideEffect` |
| OUTPUT-002 | 3 — file operations retain workspace/symlink boundaries | check | `VERIFIED` | `TestReadRejectsEscapes` and root-boundary tests |
| OUTPUT-002 | 4 — database data stays parameterized | check | `VERIFIED` | `go test ./internal/store` |
| OUTPUT-002 | 5 — owned display paths cannot execute terminal controls | check | `VERIFIED` | `go test ./internal/cli -run Terminal` |
| OUTPUT-002 | 6 — protocol data remains valid; JSON result intact | check | `VERIFIED` | `TestPromptJSONKeepsModelResultIntact` |
| OUTPUT-002 | 7 — sink-specific fixes, no destructive global escaping | manual | `IMPLEMENTED` | §Reproduced gaps and fixes; `TestPromptJSONKeepsModelResultIntact` |
| OUTPUT-003 | 1 — protocol data valid; diagnostics do not corrupt output | check | `VERIFIED` | `TestPromptJSONKeepsModelResultIntact`; ACP/MCP tests |
| OUTPUT-003 | 2 — synthetic canaries contained in tested paths | check | `VERIFIED` | Terminal and Slack control tests |
| OUTPUT-003 | 3 — required checks pass; PR records OS/client coverage | manual | `NOT_RUN` for full CI; focused checks `PASS` | This request records the exact host and path coverage; the PR triggers CI |
| OUTPUT-003 | 4 — cumulative matrix updated | check | `VERIFIED` | `git grep "Improper Output Handling" docs/reports/owasp-llm/README.md` |
| OUTPUT-003 | 5 — no full coverage inferred from plan/PR existence | manual | `IMPLEMENTED` | README §What this matrix does not claim |

## What is not complete

1. **Observed Slack delivery and rendering — `NOT_RUN`.** No authorization, no
   active webhook channel, and no channel observation exist. The HTTP 2xx
   distinction is recorded in [llm10-live-delivery.md](llm10-live-delivery.md).
2. **Manual acceptance — not recorded.** The six manual criteria above have no
   designated-approver decision record.
3. **Owning SOP lifecycle state — absent.** No checkout holds Plan 10 state. A
   merged PR is not lifecycle completion.

## Exact evidence identity

| Item | Value |
| --- | --- |
| Repository revision | `ed9ec2dc215eaa0b31a13eb49dbe2938d253c913` (`main`, 2026-11-09) |
| Wrap-up host | Darwin arm64, Go 1.27.1 |
| Original verification host | Linux `x86_64`, Go 1.27.1 (`linux/amd64`) at `24be068` + change set |
| Report | `docs/reports/owasp-llm/llm10-output-handling.md` |
| Live-test preparation | PR #313 (`f557c6fd`); this request supersedes its "exact next action" with the current blocker |
| Terminal evidence | `internal/cli/terminal.go`, `internal/cli/terminal_output_test.go` |
| Slack encoder evidence | `internal/notify/slack.go`, `internal/notify/slack_test.go` |
| Transport evidence | `internal/cli/notify_acceptance_test.go`, `internal/notify/webhook_test.go` |
| Live payload | The default `sop notify test` event in [llm10-live-delivery.md](llm10-live-delivery.md) §Exact synthetic payload (not sent) |

## Decision A — authorize one live Slack test

The approver must decide whether to authorize one synthetic test event to the
webhook bound to `#sop-alert`, during a named window, with an empty outbox and the
at-least-once caveat recorded in [llm10-live-delivery.md](llm10-live-delivery.md). The exact request text and payload
are in [llm10-live-delivery.md](llm10-live-delivery.md) §Exact authorization
request. The decision is required because:

- `notifications.channel` is `none` in every checkout checked;
- the main checkout's gitignored `.env` holds a `hooks.slack.com` webhook value,
  but a Slack incoming-webhook URL does not encode the channel name;
- no authorization record exists in the repository, PR #313, or the checked SOP
  artifacts.

The send is not performed by this request. Enabling the channel, changing the
payload style to `slack`, and running `sop notify test` all wait for the approval.

## Decision B — accept the manual criteria

The designated approver decides whether to accept the named evidence for:

- `OUTPUT-001` criteria 1–3;
- `OUTPUT-002` criterion 7;
- `OUTPUT-003` criteria 3 and 5.

Proposed evidence text (draft, not applied):

- `OUTPUT-001` criterion 1: `llm10-output-handling.md` §Sink inventory lists every
  owned sink with input origin, parser, encoding or validation, effect, and owner.
- `OUTPUT-001` criterion 2: §Intentional shell execution versus interpolation
  identifies operator-configured `sh -c` paths and confirms no model interpolation.
- `OUTPUT-001` criterion 3: §Terminal controls and resource fetching identifies the
  CLI console as the free-form owned sink and records the dashboard and outbound
  request boundaries.
- `OUTPUT-002` criterion 7: §Reproduced gaps and fixes and
  `TestPromptJSONKeepsModelResultIntact` show the terminal filter is display-only
  and persisted/JSON data is unchanged.
- `OUTPUT-003` criterion 3: the PR records Linux `x86_64` (original) and Darwin arm64
  (wrap-up) CLI/ACP/MCP scope; third-party clients and live Slack are unverified.
- `OUTPUT-003` criterion 5: README §What this matrix does not claim and the Plan
  06–09 rows keep partial and historical evidence separate.

## Lifecycle state and supported interface

Read-only checks in the wrap-up worktree at `ed9ec2dc`:

```text
sop status                       -> project is not initialized; run `sop init`
sop plan historicalize --check   -> project is not initialized; run `sop init`
sop continue --check             -> project is not initialized; run `sop init`
```

The main checkout's `plan.meta.json` has no plan identity, and the two initialized
SOP worktrees own other plans (`plan-01-sop-llm03-excessive-agency`,
`plan-04-sop-llm04-supply-chain`). No Plan 10 task, run, outcome, or approval record
exists.

Consequently:

- `sop outcome <task> --accept <criterion> --evidence <text> --by <approver>`
  cannot be used: it requires a recorded outcome and a mapped manual criterion.
- `sop plan historicalize` cannot pass readiness: it requires an active plan with
  satisfied tasks and verification evidence.

The supported path, if the operator wants a SOP acceptance record, is operator-owned:

1. Decide whether to establish an owning SOP state for this plan (for example by
   running it through `sop run` against the existing merged evidence). This may
   re-execute verify-first tasks; it is a separate governed decision.
2. Record the task outcomes through the lifecycle against the existing evidence.
3. Record one `sop outcome --accept` per manual criterion, naming the report section
   and revision.
4. Historicalize only when SOP readiness and the acceptance decisions are both
   satisfied.

This request does not perform any of those steps.

## What acceptance would and would not mean

- It would bind the named report sections and revision to the named manual criteria
  for the recorded attempt.
- It would **not** turn deterministic tests into live Slack evidence.
- It would **not** prove third-party terminal, IDE, editor, or Markdown rendering.
- It would **not** prove live Slack delivery or rendering, mention behavior, or
  auto-unfurl.
- It would **not** prove Windows behavior or a general content-sanitization/DLP
  claim.

## Bounded security claim for the verified sinks

On the verified SOP-owned sinks, SOP removes terminal control characters from
console display, escapes `&`/`<`/`>` in Slack mrkdwn so angle-bracket mentions and
masked links cannot be injected, keeps protocol output valid JSON, and retains the
existing shell, path, SQL, and authorization boundaries. This is not a claim that
all improper output handling is prevented.

## Must not

- Do not record acceptance, an approver name, or a timestamp on the operator's
  behalf.
- Do not enable the webhook channel or run `sop notify test` without Decision A.
- Do not create or edit `state.db`, `outcome.json`, or the acceptance audit files
  outside the supported command.
- Do not mark the plan complete or historicalized from this document.
- Do not treat a merged PR, a green local test, or a 2xx response as lifecycle
  completion or observed Slack delivery.

## Exact next action

The designated approver (`Tom Tran`, per `.agent-sdlc/config.yaml`) answers Decision
A (authorize one synthetic test event to the `#sop-alert` webhook and name the window)
and Decision B (accept or decline the six manual criteria with the evidence text
above). Until both records exist, Plan 10 keeps live delivery `NOT_RUN` and manual
acceptance `GAP`.
