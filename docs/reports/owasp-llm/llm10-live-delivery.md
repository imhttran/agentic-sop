# Plan 10 — prepared live Slack delivery test

Status: **PREPARED, NOT RUN.** No message was sent by this work. The prompt that
produced this document authorizes preparation, not an external send.

Related: [llm10-output-handling.md](llm10-output-handling.md) (Plan 10),
[NOTIFICATIONS.md](../../guides/NOTIFICATIONS.md), and the cumulative
[evidence matrix](README.md).

## What is already verified locally (synthetic data)

Run on Darwin arm64 at `main` `93c72857`:

```sh
go test ./internal/notify ./internal/cli -run 'Notify|Webhook|AcceptanceNotify' -count=1
```

Observed: `ok github.com/imhttran/agentic-sop/internal/notify` and
`ok github.com/imhttran/agentic-sop/internal/cli`.

| Evidence | Test |
| --- | --- |
| Slack payload shape (`text`, event id) | `TestEncodeSlackPayloadHasTextAndEventID` |
| Mrkdwn escaping (`&`, `<`, `>`) | `TestEncodeSlackPayloadEscapesMrkdwn` |
| Native payload unchanged | `TestEncodeSlackPayloadUnchangedForNative` |
| 2xx / 4xx / 5xx classification and idempotency header | `TestWebhookSlackPayloadClassifiesResponses`, `TestWebhookSendsIdempotencyKey` |
| No URL in the payload | `TestWebhookPayloadCarriesNoURL` |
| Bounded retry/backoff | `internal/notify/dispatch_test.go` |
| **Real loopback HTTP transport** from `sop notify test` | `TestAcceptanceNotifyTestDeliversToLocalReceiver` (`internal/cli/notify_acceptance_test.go`) |

`NOTIFICATIONS.md` also records an operator-local Slack delivery on 2026-10-29
(`active`, `delivered=3`). That record verifies one local configuration; it does
**not** name `#sop-alert` and it is not this Plan's live evidence.

## The destination

The repository does not record `#sop-alert`. A Slack channel is bound only by the
configured incoming-webhook URL, resolved in this order:

1. `notifications.webhook.url` in `config.yaml`,
2. `SOP_NOTIFY_WEBHOOK_URL` in the process environment,
3. `SOP_NOTIFY_WEBHOOK_URL` in the project's gitignored `.env`.

The operator must confirm which channel the bound webhook posts to. Never print,
echo, or pass the URL as a command argument; the documented `.env` procedure keeps
it out of tracked files and shell history.

No channel is enabled by default, and this work does not enable one.

## Authorization required before any send

All of the following must be true, and recorded:

- an explicit operator approval naming the **exact bound channel** (`#sop-alert`)
  and the test window;
- egress from this host to `hooks.slack.com` is permitted;
- a pre-send `sop notify status --json` snapshot (`configured_unverified`, or the
  documented prior `active` state);
- the message content is synthetic: no real secret, no customer data, no
  `@channel`/`@here`, no external or exfiltration link;
- the operator is watching the channel to confirm rendering and absence of pings.

## Minimal live test (do not run without the authorization above)

```sh
sop notify status --json     # pre-send snapshot; expect configured_unverified
sop notify test              # enqueues and attempts exactly one synthetic event
sop notify status --json     # post-send snapshot: availability and counts
```

The test event carries a fresh 8-byte nonce and `transition=NOTIFY_TEST`. A
confirmed delivery prints `Test event <id> delivered (confirmed by channel
webhook).`

## What each observation proves

| Observation | Proves | Does not prove |
| --- | --- | --- |
| HTTP 2xx | The endpoint accepted the POST (`AckConfirmed`) | That the endpoint is the intended Slack channel, or that Slack rendered the message |
| HTTP 4xx | The endpoint rejected the POST (`AckRejected`) | Anything about rendering |
| HTTP 5xx / timeout | `AckAmbiguous`; a retry may duplicate on a receiver that ignores the idempotency key | Delivery |
| Slack message visible in `#sop-alert` | End-to-end delivery and rendering for that message | Future deliveries, other channels, or other platforms |
| Post-send `delivered`/`pending`/`exhausted` counts | SOP's recorded transport state | Slack-side rendering or mention behavior |

The webhook client drains and discards the response body, so Slack's own `ok`
response is not inspected. A wrong-but-valid webhook can still return 2xx. Only the
operator looking at the channel can confirm the destination and rendering.

## Evidence record to capture (no secrets)

- UTC timestamp and the approved test window;
- repository commit and `sop version`;
- channel identity as confirmed by the operator (name only, never the URL);
- pre-send `sop notify status --json` (availability, counts);
- HTTP status class and the event id;
- a `#sop-alert` permalink or screenshot showing the rendered synthetic message and
  the absence of mentions;
- the operator's name and the authorization reference.

## Constraints

- Never print or commit the webhook URL; `NOTIFICATIONS.md` documents the
  `read -rs` → `.env` path.
- Keep the body free of mention and link syntax even though mrkdwn escaping
  neutralizes `<!channel>` and `<url|text>`.
- Delivery is at-least-once; a retry is not a second acceptance event.
- Slack may auto-unfurl a bare URL; that fetch belongs to Slack, not SOP.

## Residual limits (unchanged)

- Observed Slack rendering is a point-in-time human observation, not an automated
  check.
- Third-party client rendering and auto-unfurl remain external.
- Windows terminal behavior remains `NOT_RUN`.
- Unicode bidi/format characters are bounded by the existing terminal handling,
  not removed.

## Not claimed

- No live delivery, no channel enablement, and no rendering claim until the
  authorized test above is executed and recorded.

## Exact next action

Obtain written approval for one test message to the webhook bound to `#sop-alert`,
then run the three commands above and record the evidence block. Until then, Plan
10 live delivery stays `NOT_RUN`.
