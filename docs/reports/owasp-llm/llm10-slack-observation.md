# LLM10 — Slack rendering observation sheet (OWASP gap 12)

**Status:** prepared, **not run**. Nothing is sent by this document. The operator
performs the observation in Slack and fills in the table; the result is then recorded as
`OBSERVED` with the date and workspace-independent details only, and with no screenshots
that show private data.

Background: one synthetic notification was delivered once on 2026-11-10 (see
[llm10-live-delivery.md](llm10-live-delivery.md)). The transport was confirmed; how Slack
rendered it, and whether it reached the intended channel, were not observed.

## What `sop notify test` sends

The test event is fixed in `internal/notify/dispatch.go` (`NewTestEvent`):

- outcome: `notification test event`
- next action: `no action required; this event confirms the notification channel is reachable`
- transition: `NOTIFY_TEST`, plus a fresh nonce per invocation

It contains **no markup, link, or mention-shaped text**. So it can show the channel
binding and the plain rendering, but it cannot show escaping of markup.

## Operator steps

1. Confirm the configured channel is the intended one (`sop notify status`), without
   pasting the webhook value anywhere.
2. Run once: `sop notify test`. This sends one event.
3. In Slack, check the row below and fill it in.

| Check | Expected | Observed (yes / no / n.a.) |
| --- | --- | --- |
| Message reached the intended channel | Yes | |
| Text is literal: `notification test event` and the next-action sentence appear as written | Yes | |
| No link preview or unfurl was added | Yes | |
| No mention pinged anyone | Yes | |
| Date and time shown are plausible for the run | Yes | |

## Not covered by this sheet

- **Escaped markup and mention-shaped strings.** The escaping in `internal/notify/slack.go`
  is tested in code, but a live observation of it needs an event that carries markup. No
  such path exists without a probe. That is a decision for the operator (see below).
- **Channel binding for other channels**, and any renderer outside Slack.

## Decision for the operator

Whether to add a test-only probe that sends one event with a markup canary, a link, and
a mention-shaped string, for a single live observation. Adding it changes the notify
surface, so it stays with the operator.
