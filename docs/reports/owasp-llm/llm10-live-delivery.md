# Plan 10 — prepared live Slack delivery test

Status: **PREPARED, NOT AUTHORIZED, NOT RUN — BLOCKED ON OPERATOR.** No message was
sent by this work, and no channel was enabled. The prompt that produced this
document authorizes preparation and a request for approval, not an external send.

Related: [llm10-output-handling.md](llm10-output-handling.md) (Plan 10),
[llm10-acceptance-request.md](llm10-acceptance-request.md),
[NOTIFICATIONS.md](../../guides/NOTIFICATIONS.md), and the cumulative
[evidence matrix](README.md).

## Environment check at the wrap-up (2026-11-09T22:58Z)

Checked read-only; no configuration was changed and no state was initialized.

| Check                             | Observation                                                                                                                                                                                                                                                                           |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Repository revision               | `main` at `ed9ec2dc215eaa0b31a13eb49dbe2938d253c913`                                                                                                                                                                                                                                  |
| Host                              | Darwin arm64, Go 1.27.1                                                                                                                                                                                                                                                               |
| Active notification channel       | `notifications` block absent from `.agent-sdlc/config.yaml`; `sop notify status --json` in the main checkout reports `channel: none`, `availability: unavailable`, `webhook_configured: false`                                                                                        |
| Delivery counters (main checkout) | `delivered: 3`, `pending: 0`, `exhausted: 0` — historical records from the 2026-10-29 operator-local verification, **not** Plan 10 live evidence                                                                                                                                      |
| Webhook value                     | The main checkout's gitignored `.env` contains `SOP_NOTIFY_WEBHOOK_URL`. Only the scheme and host were inspected (`https://hooks.slack.com/…`, four path segments, no userinfo, no query). The URL value was never printed, copied into this record, or passed as a command argument. |
| Channel binding                   | A Slack incoming-webhook URL does not encode the channel name. The `#sop-alert` binding cannot be confirmed from configuration; only the operator can confirm it.                                                                                                                     |
| State in the wrap-up worktree     | No `.agent-sdlc/state.db`. `sop notify status --json` reports `project is not initialized; run sop init`. `sop init` was not run.                                                                                                                                                     |
| SOP binary                        | `sop version` reports `sop dev`                                                                                                                                                                                                                                                       |
| Authorization record              | None found in the repository, the checked SOP artifacts, or PR #313 (including its review comment).                                                                                                                                                                                   |

Read-only status snapshot from the main checkout (its own state, not Plan 10 live
evidence; the document carries no credential):

```json
{
  "active": false,
  "availability": "unavailable",
  "channel": "none",
  "delivered": 3,
  "deliveries": { "DELIVERED": 3 },
  "exhausted": 0,
  "file_configured": false,
  "pending": 0,
  "reason": "Notifications are unavailable: no channel is configured. Configure notifications.channel before relying on delivery.",
  "webhook_configured": false
}
```

No pre-send/post-send status pair exists, because no send was authorized.

Because `notifications.channel` is `none`, the `.env` webhook is not active: the
loader resolves `SOP_NOTIFY_WEBHOOK_URL` only when the channel is `webhook`
(`internal/config/notifications.go`). Enabling the channel or changing the payload
to `slack` is an operator configuration change and is **not** performed here.

## What is already verified locally (synthetic data)

Original run on Darwin arm64 at `main` `93c72857`:

```sh
go test ./internal/notify ./internal/cli -run 'Notify|Webhook|AcceptanceNotify' -count=1
```

Observed: `ok github.com/imhttran/agentic-sop/internal/notify` and
`ok github.com/imhttran/agentic-sop/internal/cli`.

Wrap-up re-run on Darwin arm64 at `main` `ed9ec2dc` (2026-11-09T22:58Z): the same
paths pass, including `TestEncodeSlackPayloadEscapesMrkdwn`,
`TestAcceptanceNotifyTestDeliversToLocalReceiver`, and the webhook
response-classification tests.

| Evidence                                                | Test                                                                                         |
| ------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| Slack payload shape (`text`, event id)                  | `TestEncodeSlackPayloadHasTextAndEventID`                                                    |
| Mrkdwn escaping (`&`, `<`, `>`)                         | `TestEncodeSlackPayloadEscapesMrkdwn`                                                        |
| Native payload unchanged                                | `TestWebhookNativePayloadUnchanged`                                                          |
| 2xx / 4xx / 5xx classification and idempotency header   | `TestWebhookSlackPayloadClassifiesResponses`, `TestWebhookSendsIdempotencyKey`               |
| No URL in the payload                                   | `TestWebhookSlackPayloadCarriesNoURL`                                                        |
| Bounded retry/backoff                                   | `internal/notify/dispatch_test.go`                                                           |
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

Steps 2 and 3 apply only when `notifications.channel` is `webhook`. The operator
must confirm which channel the bound webhook posts to. Never print, echo, or pass
the URL as a command argument; the documented `.env` procedure keeps it out of
tracked files and shell history.

No channel is enabled by default, and this work does not enable one.

## Authorization required before any send

All of the following must be true, and recorded:

- an explicit operator approval naming the **exact bound channel** (`#sop-alert`)
  and the test window;
- egress from this host to `hooks.slack.com` is permitted;
- `notifications.channel: webhook` is enabled and
  `notifications.webhook.payload: slack` is set by the operator;
- a pre-send `sop notify status --json` snapshot showing `pending: 0`
  (`configured_unverified`); `sop notify test` runs a bounded drain with the
  default limit of 50, so a non-empty outbox would also attempt other due
  deliveries, not only the synthetic test event;
- the message content is synthetic: no real secret, no customer data, no
  `@channel`/`@here`, no external or exfiltration link;
- the operator is watching the channel (or will supply a permalink/screenshot)
  to confirm rendering and absence of pings.

The Plan 10 execution contract originally said "no live Slack message". PR #313
prepared a separately authorized exception. The send is not performed by the
wrap-up; it requires the explicit approval above.

### Exact authorization request (operator-owned)

> I request authorization for **one** synthetic notification send from this
> host:
>
> - **Channel:** the Slack incoming webhook bound to `#sop-alert`
> - **Window:** `<UTC start> – <UTC end>` (to be filled in by the approver)
> - **Content:** the default `sop notify test` event shown below; no mentions, no
>   real secret, no external or exfiltration link
> - **Effect:** one POST per attempt to the configured webhook. Delivery is
>   at-least-once with bounded retry and no receiver-side deduplication guarantee:
>   a lost acknowledgement, timeout, or 5xx can produce a second visible message.
>   A retry is not a second acceptance event, but it may be a second visible
>   message.
> - **Observation:** the operator confirms the channel name, the visible text,
>   the rendering, and the absence of pings, and provides a permalink or
>   screenshot
> - **Not done before approval:** enabling `notifications.channel: webhook`,
>   changing `notifications.webhook.payload` to `slack`, or running
>   `sop notify test`
>
> Authorization reference: `<approver name and reference>`.

## Exact synthetic payload

The default test event is the only prepared live payload. It is a real lifecycle
event with a fresh 8-byte nonce, so each explicit invocation gets a distinct event
id; a retry of one delivery keeps the recorded id.

| Field       | Value                                                                                                                                        |
| ----------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Kind        | `completed`                                                                                                                                  |
| Project     | `agentic-sop`                                                                                                                                |
| Attempt     | `1`                                                                                                                                          |
| Transition  | `NOTIFY_TEST`                                                                                                                                |
| Outcome     | `notification test event`                                                                                                                    |
| Next action | `no action required; this event confirms the notification channel is reachable`                                                              |
| Event id    | fresh 64-hex SHA-256 per invocation (sample generated locally, not sent: `3059750357a62a60dc663e3cb204542b74f63aeedd9bd6d376abddcb225f1352`) |

Rendered Slack `text` (the default payload contains no `&`, `<`, or `>`, so the
mrkdwn escaping leaves it byte-identical to this template):

```text
SOP completed in agentic-sop: event <event-id> (attempt 1, NOTIFY_TEST)
outcome: notification test event
next: no action required; this event confirms the notification channel is reachable
```

This payload is not adversarial. It does not carry a mention, a masked link, a
control character, or an external URL, so an observed delivery of it **does not**
prove that those cases are neutralized in Slack. The deterministic
`TestEncodeSlackPayloadEscapesMrkdwn` covers the encoder for `<!channel>`,
`<https://…|link>`, and `&`. A separate live adversarial case would need its own
explicit authorization.

## Minimal live test (do not run without the authorization above)

```sh
sop notify status --json     # pre-send snapshot; record the exact availability
sop notify test              # enqueues one synthetic event, then runs a bounded drain
sop notify status --json     # post-send snapshot: availability and counts
```

The status counters are global across recorded deliveries, not scoped to the
current channel. A prior confirmed delivery on another channel can make
`availability` report `active` before this webhook channel has delivered. Record
the snapshot as SOP's recorded state; do not treat `active` alone as proof for the
current channel. This is a status-reporting limitation, not a Plan 10 sink repair.

The test event carries a fresh 8-byte nonce and `transition=NOTIFY_TEST`. A
confirmed delivery prints `Test event <id> delivered (confirmed by channel
webhook).`

`sop notify test` calls the bounded drain with the default limit of 50, so it can
also attempt other due deliveries. Confirm `pending: 0` in the pre-send snapshot,
or drain/isolate the outbox first, so the approval covers only the synthetic event.
A retry of the test delivery can produce a second visible message; that is the
documented at-least-once behavior.

## What each observation proves

| Observation                                        | Proves                                                                               | Does not prove                                                                      |
| -------------------------------------------------- | ------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| HTTP 2xx                                           | The endpoint accepted the POST (`AckConfirmed`)                                      | That the endpoint is the intended Slack channel, or that Slack rendered the message |
| HTTP 4xx                                           | The endpoint rejected the POST (`AckRejected`)                                       | Anything about rendering                                                            |
| HTTP 5xx / timeout                                 | `AckAmbiguous`; a retry may duplicate on a receiver that ignores the idempotency key | Delivery                                                                            |
| Slack message visible in `#sop-alert`              | End-to-end delivery and rendering for that message                                   | Future deliveries, other channels, other platforms, or adversarial payload handling |
| Post-send `delivered`/`pending`/`exhausted` counts | SOP's recorded transport state                                                       | Slack-side rendering or mention behavior                                            |

The webhook client drains and discards the response body, so Slack's own `ok`
response is not inspected. A wrong-but-valid webhook can still return 2xx. Only the
operator looking at the channel can confirm the destination and rendering.

## Evidence record to capture (no secrets)

- UTC timestamp and the approved test window;
- repository commit `ed9ec2dc` (or the commit of the sending build) and
  `sop version`;
- channel identity as confirmed by the operator (name only, never the URL);
- pre-send `sop notify status --json` (availability, counts, `webhook_configured`);
- source/binary revision and the event id from `sop notify test`;
- HTTP status class and the delivery counters after the send;
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
- Slack still interprets non-angular mrkdwn formatting and emoji shortcodes; the
  encoder neutralizes mentions and masked links, not all formatting.
- Windows terminal behavior remains `NOT_RUN`.
- Unicode bidi/format characters are bounded by the existing terminal handling,
  not removed.

## Not claimed

- No live delivery, no channel enablement, and no rendering claim until the
  authorized test above is executed and recorded.
- No claim that the default test event verifies adversarial escaping.

## Exact next action

The designated approver answers the authorization request in §Exact authorization
request, naming `#sop-alert` and the test window. The operator then enables the
`webhook` channel with the `slack` payload, runs the three commands above, captures
the evidence block, and supplies the channel observation. Until then, Plan 10 live
delivery stays `NOT_RUN`.
